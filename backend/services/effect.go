package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"
)

// Вилка эффекта для руководителя. Всё, что можно измерить, измеряется:
// объём поисков, доля проблемных, конверсия, поправка на сессии, вероятность заказа и средний чек — из
// журналов; доля спасённых — из реплея отложенной недели (holdout_week): поиски недели, которых агент не видел,
// прогнанные через B0, B3 и B4 (ревью-2 N2: не test — правка v2 подбиралась под его тексты). Вклад поиска —
// B0 → B3; вклад агента — B3 → B4, только если все принятые правки B4 сделаны запусками после разреза журнала
// (иначе 0 с причиной). Допущения можно переопределить параметрами запроса; откуда взято каждое значение — в source.
//
//	вероятность заказа спасённого = конверсия успешного поиска × понижающий коэффициент (≤ конверсии)
//	заказы/мес = поиски/мес × доля проблемных × поправка на сессии × доля спасённых × вероятность заказа спасённого
//	₽/мес      = заказы/мес × средний чек
//
// Почему понижающий коэффициент: «спасён» на реплее значит «подходящий товар в топ-10», а не «покупатель
// купил»; часть покупателей с неудачным поиском уже ушла или купила иначе. Без него базовый сценарий давал +261 заказ
// при 245 текущих — удвоение, которому никто не поверит. Коэффициент — подписанное допущение (assumed), не замер;
// вероятность заказа спасённого поиска никогда не выше измеренной конверсии успешного.
// Сценарии отличаются долей спасённых (границы 95% интервала Уилсона) и коэффициентом (×0,5 / ×1 / ×1,5, не больше 1).

// EffectRequest — параметры вилки; nil — значение по умолчанию (измеренное, если есть данные).
type EffectRequest struct {
	Days          int
	SessionFactor *float64
	OrderProb     *float64
	// RescueFactor — понижающий коэффициент для спасённого поиска (0..1].
	RescueFactor *float64
	AvgCheckRub  *float64
}

const (
	effectNote = "Сценарная оценка на тестовых данных. Фактический эффект измеряется A/B-тестом в пилоте"
	// effectFormula — формула для экрана.
	effectFormula = "заказы в месяц = поиски в месяц × доля проблемных × поправка на сессии × доля спасённых × " +
		"конверсия успешного поиска × понижающий коэффициент; рубли в месяц = заказы × средний чек"
	effectFormulaKeys = "orders_month = searches_month × problem_share × session_factor × rescued_share × " +
		"order_prob × rescue_factor; revenue_rub_month = orders_month × avg_check_rub"
	// Принятые значения, когда замера нет.
	assumedSessionFactor = 1.0
	assumedOrderProb     = 0.1
	assumedAvgCheckRub   = 15000
	// assumedRescueFactor — понижающий коэффициент: спасённый поиск заказывает в 0,3 раза реже успешного.
	// Допущение до A/B-теста в пилоте; редактируется на экране.
	assumedRescueFactor = 0.3
	// upliftWarn — прирост к текущим заказам поиска, выше которого базовый сценарий помечается как
	// неправдоподобный (предупреждение, без подрезки).
	upliftWarn = 0.5
	// effectSet — набор, на котором меряется доля спасённых: реплей отложенной недели.
	effectSet = domain.EvalSetHoldoutWeek
	monthDays = 30
)

// effectScenarios — множитель вероятности заказа и граница интервала доли спасённых.
var effectScenarios = []struct {
	id, label, rule string
	orderMul        float64
	bound           int // -1 нижняя граница, 0 точка, +1 верхняя
}{
	{"conservative", "Консервативный", "доля спасённых — нижняя граница 95% интервала, понижающий коэффициент × 0,5", 0.5, -1},
	{"base", "Базовый", "измеренные значения и допущения как есть", 1, 0},
	{"optimistic", "Оптимистичный", "доля спасённых — верхняя граница 95% интервала, понижающий коэффициент × 1,5 (не больше 1)", 1.5, 1},
}

// Effect — GET /api/effect.
func (s *Services) Effect(ctx context.Context, req EffectRequest) (*out.Effect, error) {
	return s.effect(ctx, req, nil)
}

// effect — вилка; sum — уже посчитанные суммы исходов за тот же период (аналитика роли head), nil — посчитать.
func (s *Services) effect(ctx context.Context, req EffectRequest, sum *repositories.SearchSummary) (*out.Effect, error) {
	if req.Days == 0 {
		req.Days = analyticsDefaultDays
	}
	var bad []errs.FieldError
	if req.Days < 1 || req.Days > analyticsMaxDays {
		bad = append(bad, errs.FieldError{Field: "days", Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Период — от 1 до %d дней", analyticsMaxDays)})
	}
	share := func(field string, v *float64) {
		if v != nil && (*v <= 0 || *v > 1 || math.IsNaN(*v)) {
			bad = append(bad, errs.FieldError{Field: field, Code: errs.CodeOutOfRange, Message: "Доля — больше 0 и не больше 1"})
		}
	}
	share("session_factor", req.SessionFactor)
	share("order_prob", req.OrderProb)
	share("rescue_factor", req.RescueFactor)
	if v := req.AvgCheckRub; v != nil && (*v <= 0 || *v > 1e9 || math.IsNaN(*v)) {
		bad = append(bad, errs.FieldError{Field: "avg_check_rub", Code: errs.CodeOutOfRange, Message: "Средний чек — больше 0 ₽"})
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	now := s.now().UTC()
	since := now.Add(-time.Duration(req.Days) * 24 * time.Hour)
	// Исходы поисков и заказанные товары считает база (repositories/analytics_sql.go): события в память не читаются.
	var (
		m      effectLogs
		evals  []domain.Evaluation
		credit agentCredit
	)
	if s.repos.Enabled() {
		var err error
		if sum == nil {
			if sum, _, err = s.repos.SearchOutcomesSince(ctx, since, 0); err != nil {
				return nil, err
			}
		}
		ordered, err := s.repos.OrderedProductsSince(ctx, since)
		if err != nil {
			return nil, err
		}
		if evals, err = s.repos.LatestEvaluations(ctx, effectSet); err != nil {
			return nil, err
		}
		credit = s.agentCredit(ctx, evals)
		m = effectLogsFrom(sum.Total)
		if cs := s.catalog.Load(); cs != nil && cs.lex != nil {
			for _, pc := range ordered {
				if p := cs.lex.docs[pc.ProductID].PriceRub; p != nil && *p > 0 {
					for range pc.N {
						m.orderChecks = append(m.orderChecks, *p)
					}
				}
			}
		}
	}
	res := effectFromLogs(m, measureRescued(evals, credit), req)
	res.Days, res.From, res.To = req.Days, since, now
	return res, nil
}

// effectLogs — измеренное по журналам.
type effectLogs struct {
	searches, problems, problemSessions, orders int
	clickSessions, orderSessions                int
	orderChecks                                 []float64
}

// effectLogsFrom — замер по суммам исходов (SQL-агрегат или aggregateOutcomes).
func effectLogsFrom(t repositories.SearchTotals) effectLogs {
	return effectLogs{searches: t.Searches, problems: t.Problems, problemSessions: t.ProblemSessions, orders: t.Orders,
		clickSessions: t.ClickSessions, orderSessions: t.OrderSessions}
}

// measureLogs — замер по событиям в памяти (эталон SQL-агрегации и путь тестов).
func measureLogs(searches []domain.SearchEvent, behavior []domain.BehaviorEvent, price func(string) *float64) effectLogs {
	sum, _ := aggregateOutcomes(searches, behavior, 0)
	m := effectLogsFrom(sum.Total)
	if price != nil {
		for _, b := range behavior {
			if b.Type != domain.EventOrder || b.ProductID == nil {
				continue
			}
			if p := price(*b.ProductID); p != nil && *p > 0 {
				m.orderChecks = append(m.orderChecks, *p)
			}
		}
	}
	return m
}

// agentCredit — засчитывается ли вклад агента (B3 → B4) в долю спасённых.
type agentCredit struct {
	// Counted — все принятые правки правил версии B4 сделаны запусками после разреза журнала.
	Counted bool
	// PreHoldout — среди правок B4 есть сделанные запусками до разреза журнала: B4 видел оцениваемые тексты,
	// и его числа — не независимая оценка (ни на test, ни на отложенной неделе).
	PreHoldout bool
	// Note — почему вклад агента не засчитан (или что засчитано).
	Note string
}

// honestRun — запуск агента шёл с разрезом журнала и без текстов test (эпизод holdout).
func honestRun(run *domain.AgentRun) bool {
	if run == nil {
		return false
	}
	var ep struct {
		Holdout *struct {
			Enabled     bool `json:"enabled"`
			ExcludeTest bool `json:"exclude_test"`
		} `json:"holdout"`
	}
	_ = json.Unmarshal(run.Episode, &ep)
	return ep.Holdout != nil && ep.Holdout.Enabled && ep.Holdout.ExcludeTest
}

// agentCredit — правки, вошедшие в версию правил прогона B4 на отложенной неделе (принятые, не откаченные), и
// честность их запусков. Хоть одна правка из запуска до разреза — вклад агента не засчитывается: она подобрана
// под эту же неделю и тексты test.
func (s *Services) agentCredit(ctx context.Context, evals []domain.Evaluation) agentCredit {
	var b3, b4 *domain.Evaluation
	for i := range evals {
		switch evals[i].Config {
		case domain.ConfigB3:
			b3 = &evals[i]
		case domain.ConfigB4:
			b4 = &evals[i]
		}
	}
	if b4 == nil || b3 == nil {
		return agentCredit{Note: "нет прогона B3 и B4 на отложенной неделе — вклад агента не измерен"}
	}
	st := s.store()
	if st == nil {
		return agentCredit{Note: "нет базы агента — вклад агента не засчитан"}
	}
	accepted, err := st.ListProposals(ctx, domain.ProposalAccepted, "")
	if err != nil {
		return agentCredit{Note: "предложения агента не прочитаны — вклад агента не засчитан"}
	}
	var honest, dishonest []string
	for _, p := range accepted {
		if p.ConfigVersion == nil || *p.ConfigVersion > b4.RulesVersion || *p.ConfigVersion <= b3.RulesVersion {
			continue
		}
		run, err := st.GetAgentRun(ctx, p.RunID)
		if err == nil && honestRun(run) {
			honest = append(honest, p.ID.String()[:8])
		} else {
			dishonest = append(dishonest, p.RunID.String()[:8])
		}
	}
	switch {
	case len(dishonest) > 0:
		slices.Sort(dishonest)
		return agentCredit{PreHoldout: true, Note: fmt.Sprintf("правки B4 (v%d) сделаны запусками до разреза журнала (%s): они видели "+
			"отложенную неделю и тексты test — вклад агента не засчитан (0)", b4.RulesVersion, strings.Join(slices.Compact(dishonest), ", "))}
	case len(honest) == 0:
		return agentCredit{Note: "принятых правок агента в B4 нет — вклад агента 0"}
	}
	return agentCredit{Counted: true, Note: fmt.Sprintf("правки B4 (v%d) из запусков после разреза журнала: %s",
		b4.RulesVersion, strings.Join(honest, ", "))}
}

// measureRescued — доля спасённых на реплее отложенной недели, по поискам (частота запроса — число поисков
// недели). Среди размеченных запросов, где B0 не дал релевантного в топ-10: вклад поиска — доля, где его дал
// B3; вклад агента — где B3 не дал, а B4 дал (только если credit.Counted). Прогоны B0, B3 и B4 — на одном
// наборе (set_hash). Интервал Уилсона — по числу различных запросов (поиски одного запроса не независимы).
func measureRescued(evals []domain.Evaluation, credit agentCredit) *out.EffectRescued {
	by := map[string]domain.Evaluation{}
	for _, e := range evals {
		if e.Set == "" || e.Set == effectSet {
			by[e.Config] = e
		}
	}
	base, ok := by[domain.ConfigB0]
	if !ok {
		return nil
	}
	top, ok := by[domain.ConfigB3]
	if !ok || top.SetHash != base.SetHash {
		return nil
	}
	agent, hasAgent := by[domain.ConfigB4]
	hasAgent = hasAgent && agent.SetHash == base.SetHash
	load := func(e domain.Evaluation) map[string]out.EvalQueryResult {
		var rows []out.EvalQueryResult
		if json.Unmarshal(e.PerQuery, &rows) != nil {
			return nil
		}
		m := make(map[string]out.EvalQueryResult, len(rows))
		for _, r := range rows {
			m[r.ID] = r
		}
		return m
	}
	b0, b3 := load(base), load(top)
	var b4 map[string]out.EvalQueryResult
	if hasAgent {
		b4 = load(agent)
	}
	r := &out.EffectRescued{Set: effectSet, SetHash: base.SetHash, Baseline: domain.ConfigB0, Config: domain.ConfigB3,
		RunBaseline: base.RunID, RunConfig: top.RunID, RulesVersion: top.RulesVersion, Weight: "searches",
		AgentCounted: credit.Counted && hasAgent, AgentPreHoldout: credit.PreHoldout, AgentNote: credit.Note}
	if hasAgent {
		r.RunAgent, r.AgentRulesVersion = agent.RunID, agent.RulesVersion
	}
	var searchRescued, agentRescued, rescuedQ int
	for id, q := range b0 {
		a, ok := b3[id]
		// Только размеченные запросы (MRR посчитан) с ответом обеих конфигураций, где B0 не дал релевантного.
		if q.MRR10 == nil || q.Error != "" || !ok || a.Error != "" || q.HasRelevant {
			continue
		}
		w := max(q.Freq, 1)
		r.Failed += w
		r.FailedQueries++
		switch {
		case a.HasRelevant:
			searchRescued += w
			rescuedQ++
		case r.AgentCounted:
			if x, ok := b4[id]; ok && x.Error == "" && x.HasRelevant {
				agentRescued += w
				rescuedQ++
			}
		}
	}
	if r.Failed == 0 {
		return nil
	}
	r.Rescued = searchRescued + agentRescued
	r.RescuedQueries = rescuedQ
	r.SearchShare = evalRound4(float64(searchRescued) / float64(r.Failed))
	r.AgentShare = evalRound4(float64(agentRescued) / float64(r.Failed))
	r.Share = evalRound4(float64(r.Rescued) / float64(r.Failed))
	r.CILow, r.CIHigh = wilsonP(float64(r.Rescued)/float64(r.Failed), r.FailedQueries)
	agentPart := "вклад агента B3 → B4 не засчитан"
	if r.AgentCounted {
		r.Config = domain.ConfigB4
		agentPart = fmt.Sprintf("вклад агента B3 → B4 (прогон %s)", r.RunAgent)
	}
	r.Source = fmt.Sprintf("реплей отложенной недели (holdout_week, %s): поиски, которых агент не видел; "+
		"B0 → B3 (прогоны %s, %s), %s; доля — по поискам", base.SetHash, base.RunID, top.RunID, agentPart)
	return r
}

// wilson — 95% интервал Уилсона для доли k из n.
func wilson(k, n int) (float64, float64) {
	return wilsonP(float64(k)/float64(n), n)
}

// wilsonP — 95% интервал Уилсона для доли p на n наблюдениях.
func wilsonP(p float64, n int) (float64, float64) {
	const z = 1.96
	nn := float64(n)
	den := 1 + z*z/nn
	c := (p + z*z/(2*nn)) / den
	h := z * math.Sqrt(p*(1-p)/nn+z*z/(4*nn*nn)) / den
	return evalRound4(math.Max(0, c-h)), evalRound4(math.Min(1, c+h))
}

// computeEffect — вилка из измеренного и допущений (чистая функция).
func computeEffect(searches []domain.SearchEvent, behavior []domain.BehaviorEvent, evals []domain.Evaluation,
	price func(string) *float64, req EffectRequest) *out.Effect {
	return effectFromLogs(measureLogs(searches, behavior, price), measureRescued(evals, agentCredit{}), req)
}

// effectFromLogs — вилка из замера по журналам, прогонов оценки и допущений (чистая функция).
func effectFromLogs(m effectLogs, rescued *out.EffectRescued, req EffectRequest) *out.Effect {
	days := req.Days
	if days == 0 {
		days = analyticsDefaultDays
	}
	res := &out.Effect{Note: effectNote, Formula: effectFormula, FormulaKeys: effectFormulaKeys,
		Assumptions: []out.EffectParam{}, Scenarios: []out.EffectScenario{}, Missing: []string{}, Warnings: []string{}}
	in := &res.Measured
	in.Searches, in.ProblemSearches, in.ProblemSessions, in.Orders = m.searches, m.problems, m.problemSessions, m.orders
	in.SearchesMonth = math.Round(float64(m.searches)*monthDays/float64(days)*10) / 10
	in.ProblemShare = share(m.problems, m.searches)
	in.SearchOrderRate = share(m.orders, m.searches)
	in.OrdersMonth = math.Round(float64(m.orders)*monthDays/float64(days)*10) / 10
	in.Rescued = rescued

	param := func(key, label, unit, hint string, measured *float64, assumed float64, override *float64) out.EffectParam {
		p := out.EffectParam{Key: key, Label: label, Unit: unit, Hint: hint, Default: assumed, Source: "assumed"}
		if measured != nil {
			p.Default, p.Source = *measured, "measured"
		}
		p.Value = p.Default
		if override != nil {
			p.Value, p.Source = *override, "request"
		}
		return p
	}
	sessionFactor := share(m.problemSessions, m.problems)
	orderProb := share(m.orderSessions, m.clickSessions)
	var avgCheck *float64
	if len(m.orderChecks) > 0 {
		sum := 0.0
		for _, v := range m.orderChecks {
			sum += v
		}
		avgCheck = evalPtr(math.Round(sum / float64(len(m.orderChecks))))
	}
	sf := param("session_factor", "Поправка на сессии", "доля",
		"Один покупатель повторяет неудачный поиск в одной сессии: считаем сессии, а не поиски. Замер — проблемные сессии / проблемные поиски",
		sessionFactor, assumedSessionFactor, req.SessionFactor)
	op := param("order_prob", "Конверсия успешного поиска в заказ", "доля",
		"Потолок для спасённого поиска. Замер — доля сессий с кликом из поиска, закончившихся заказом",
		orderProb, assumedOrderProb, req.OrderProb)
	rf := param("rescue_factor", "Понижающий коэффициент для спасённого поиска", "доля",
		"Допущение, не замер: «спасён» на реплее — подходящий товар в топ-10, а не покупка; часть покупателей уже ушла. "+
			"Вероятность заказа спасённого = конверсия успешного × коэффициент, не выше конверсии",
		nil, assumedRescueFactor, req.RescueFactor)
	ac := param("avg_check_rub", "Средний чек", "₽",
		"Замер — средняя цена позиций в заказах из журнала по ценам каталога",
		avgCheck, assumedAvgCheckRub, req.AvgCheckRub)
	res.Assumptions = append(res.Assumptions, sf, op, rf, ac)

	if m.searches == 0 {
		res.Missing = append(res.Missing, "нет поисков за период — нечего масштабировать")
	}
	if in.Rescued == nil {
		res.Missing = append(res.Missing, "нет прогона оценки B0 и B3 на реплее отложенной недели — make eval SET=holdout_week CONFIGS=B0,B3,B4")
	}
	if len(res.Missing) > 0 {
		return res
	}
	problemSessions := in.SearchesMonth * *in.ProblemShare * sf.Value
	for _, sc := range effectScenarios {
		rs := in.Rescued.Share
		switch sc.bound {
		case -1:
			rs = in.Rescued.CILow
		case 1:
			rs = in.Rescued.CIHigh
		}
		factor := math.Min(1, rf.Value*sc.orderMul)
		prob := op.Value * factor // не выше конверсии успешного поиска
		rescued := problemSessions * rs
		orders := rescued * prob
		scn := out.EffectScenario{
			ID: sc.id, Label: sc.label, Rule: sc.rule, RescuedShare: rs, RescueFactor: evalRound4(factor),
			OrderProb: evalRound4(prob), ProblemSessionsMonth: math.Round(problemSessions*10) / 10,
			RescuedMonth: math.Round(rescued*10) / 10, OrdersMonth: math.Round(orders*10) / 10,
			RevenueRubMonth: math.Round(orders * ac.Value),
		}
		if in.OrdersMonth > 0 {
			scn.Uplift = evalPtr(evalRound4(orders / in.OrdersMonth))
		}
		if sc.id == "base" && scn.Uplift != nil && *scn.Uplift > upliftWarn {
			res.Warnings = append(res.Warnings, fmt.Sprintf("Базовый сценарий даёт +%.0f %% к текущим заказам из поиска — "+
				"больше половины; проверьте допущения (коэффициент, конверсию)", *scn.Uplift*100))
		}
		res.Scenarios = append(res.Scenarios, scn)
	}
	return res
}
