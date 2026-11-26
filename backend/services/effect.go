package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Вилка эффекта для руководителя. Всё, что можно измерить, измеряется:
// объём поисков, доля проблемных, конверсия, поправка на сессии, вероятность заказа и средний чек — из
// журналов; доля спасённых — из прогона оценки B0 vs B4 на замороженном наборе test. Допущения можно
// переопределить параметрами запроса; откуда взято каждое значение — в source.
//
//	заказы/мес = поиски/мес × доля проблемных × поправка на сессии × доля спасённых × вероятность заказа
//	₽/мес      = заказы/мес × средний чек
//
// Сценарии отличаются долей спасённых (границы 95% интервала Уилсона) и вероятностью заказа (×0,5 / ×1 / ×1,5).

// EffectRequest — параметры вилки; nil — значение по умолчанию (измеренное, если есть данные).
type EffectRequest struct {
	Days          int
	SessionFactor *float64
	OrderProb     *float64
	AvgCheckRub   *float64
}

const (
	effectNote = "Сценарная оценка на тестовых данных. Фактический эффект измеряется A/B-тестом в пилоте"
	// effectFormula — формула для экрана.
	effectFormula = "заказы в месяц = поиски в месяц × доля проблемных × поправка на сессии × доля спасённых × " +
		"вероятность заказа; рубли в месяц = заказы × средний чек"
	effectFormulaKeys = "orders_month = searches_month × problem_share × session_factor × rescued_share × order_prob; " +
		"revenue_rub_month = orders_month × avg_check_rub"
	// Принятые значения, когда замера нет.
	assumedSessionFactor = 1.0
	assumedOrderProb     = 0.1
	assumedAvgCheckRub   = 15000
	// effectSet — набор, на котором меряется доля спасённых: замороженный test.
	effectSet = domain.EvalSetTest
	monthDays = 30
)

// effectScenarios — множитель вероятности заказа и граница интервала доли спасённых.
var effectScenarios = []struct {
	id, label, rule string
	orderMul        float64
	bound           int // -1 нижняя граница, 0 точка, +1 верхняя
}{
	{"conservative", "Консервативный", "доля спасённых — нижняя граница 95% интервала, вероятность заказа × 0,5", 0.5, -1},
	{"base", "Базовый", "измеренные значения и допущения как есть", 1, 0},
	{"optimistic", "Оптимистичный", "доля спасённых — верхняя граница 95% интервала, вероятность заказа × 1,5 (не больше 1)", 1.5, 1},
}

// Effect — GET /api/effect.
func (s *Services) Effect(ctx context.Context, req EffectRequest) (*out.Effect, error) {
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
	if v := req.AvgCheckRub; v != nil && (*v <= 0 || *v > 1e9 || math.IsNaN(*v)) {
		bad = append(bad, errs.FieldError{Field: "avg_check_rub", Code: errs.CodeOutOfRange, Message: "Средний чек — больше 0 ₽"})
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	now := s.now().UTC()
	since := now.Add(-time.Duration(req.Days) * 24 * time.Hour)
	var (
		searches []domain.SearchEvent
		behavior []domain.BehaviorEvent
		evals    []domain.Evaluation
	)
	if s.repos.Enabled() {
		var err error
		if searches, err = s.repos.SearchEventsSince(ctx, since); err != nil {
			return nil, err
		}
		if behavior, err = s.repos.BehaviorEventsSince(ctx, since); err != nil {
			return nil, err
		}
		if evals, err = s.repos.LatestEvaluations(ctx, effectSet); err != nil {
			return nil, err
		}
	}
	var price func(string) *float64
	if cs := s.catalog.Load(); cs != nil && cs.lex != nil {
		price = func(pid string) *float64 { return cs.lex.docs[pid].PriceRub }
	}
	res := computeEffect(searches, behavior, evals, price, req)
	res.Days, res.From, res.To = req.Days, since, now
	return res, nil
}

// effectLogs — измеренное по журналам.
type effectLogs struct {
	searches, problems, problemSessions, orders int
	clickSessions, orderSessions                int
	orderChecks                                 []float64
}

func measureLogs(searches []domain.SearchEvent, behavior []domain.BehaviorEvent, price func(string) *float64) effectLogs {
	var m effectLogs
	problemSess := map[string]bool{}
	clickSess := map[string]bool{}
	orderSess := map[string]bool{}
	for _, o := range buildOutcomes(searches, behavior) {
		m.searches++
		key := sessionKey(o.ev.SessionID, o.ev.RequestID)
		if o.ev.ResultsCount == 0 || !o.clicked {
			m.problems++
			problemSess[key] = true
		}
		if o.clicked {
			clickSess[key] = true
			if o.ordered {
				orderSess[key] = true
			}
		}
		if o.ordered {
			m.orders++
		}
	}
	m.problemSessions, m.clickSessions, m.orderSessions = len(problemSess), len(clickSess), len(orderSess)
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

// measureRescued — доля спасённых: test, B0 против B4 (или B3, если B4 не прогонялся).
func measureRescued(evals []domain.Evaluation) *out.EffectRescued {
	by := map[string]domain.Evaluation{}
	for _, e := range evals {
		by[e.Config] = e
	}
	base, ok := by[domain.ConfigB0]
	if !ok {
		return nil
	}
	cfg := domain.ConfigB4
	top, ok := by[cfg]
	if !ok {
		cfg = domain.ConfigB3
		if top, ok = by[cfg]; !ok {
			return nil
		}
	}
	var b0, bx []out.EvalQueryResult
	if json.Unmarshal(base.PerQuery, &b0) != nil || json.Unmarshal(top.PerQuery, &bx) != nil {
		return nil
	}
	after := map[string]out.EvalQueryResult{}
	for _, r := range bx {
		after[r.ID] = r
	}
	r := &out.EffectRescued{Set: effectSet, Baseline: domain.ConfigB0, Config: cfg, RunBaseline: base.RunID,
		RunConfig: top.RunID, RulesVersion: top.RulesVersion}
	for _, q := range b0 {
		// Только запросы с разметкой релевантного (MRR посчитан) и с ответом обеих конфигураций.
		a, ok := after[q.ID]
		if q.MRR10 == nil || q.Error != "" || !ok || a.Error != "" || q.HasRelevant {
			continue
		}
		r.Failed++
		if a.HasRelevant {
			r.Rescued++
		}
	}
	if r.Failed == 0 {
		return nil
	}
	r.Share = evalRound4(float64(r.Rescued) / float64(r.Failed))
	r.CILow, r.CIHigh = wilson(r.Rescued, r.Failed)
	return r
}

// wilson — 95% интервал Уилсона для доли k из n.
func wilson(k, n int) (float64, float64) {
	const z = 1.96
	p, nn := float64(k)/float64(n), float64(n)
	den := 1 + z*z/nn
	c := (p + z*z/(2*nn)) / den
	h := z * math.Sqrt(p*(1-p)/nn+z*z/(4*nn*nn)) / den
	return evalRound4(math.Max(0, c-h)), evalRound4(math.Min(1, c+h))
}

// computeEffect — вилка из измеренного и допущений (чистая функция).
func computeEffect(searches []domain.SearchEvent, behavior []domain.BehaviorEvent, evals []domain.Evaluation,
	price func(string) *float64, req EffectRequest) *out.Effect {
	days := req.Days
	if days == 0 {
		days = analyticsDefaultDays
	}
	m := measureLogs(searches, behavior, price)
	res := &out.Effect{Note: effectNote, Formula: effectFormula, FormulaKeys: effectFormulaKeys,
		Assumptions: []out.EffectParam{}, Scenarios: []out.EffectScenario{}, Missing: []string{}}
	in := &res.Measured
	in.Searches, in.ProblemSearches, in.ProblemSessions, in.Orders = m.searches, m.problems, m.problemSessions, m.orders
	in.SearchesMonth = math.Round(float64(m.searches)*monthDays/float64(days)*10) / 10
	in.ProblemShare = share(m.problems, m.searches)
	in.SearchOrderRate = share(m.orders, m.searches)
	in.Rescued = measureRescued(evals)

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
	op := param("order_prob", "Вероятность заказа спасённого поиска", "доля",
		"Спасённый поиск ведёт себя как успешный. Замер — доля сессий с кликом из поиска, закончившихся заказом",
		orderProb, assumedOrderProb, req.OrderProb)
	ac := param("avg_check_rub", "Средний чек", "₽",
		"Замер — средняя цена позиций в заказах из журнала по ценам каталога",
		avgCheck, assumedAvgCheckRub, req.AvgCheckRub)
	res.Assumptions = append(res.Assumptions, sf, op, ac)

	if m.searches == 0 {
		res.Missing = append(res.Missing, "нет поисков за период — нечего масштабировать")
	}
	if in.Rescued == nil {
		res.Missing = append(res.Missing, "нет прогона оценки B0 и B4 (или B3) на наборе test — make eval SET=test CONFIGS=B0,B1,B2,B3,B4")
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
		prob := math.Min(1, op.Value*sc.orderMul)
		rescued := problemSessions * rs
		orders := rescued * prob
		res.Scenarios = append(res.Scenarios, out.EffectScenario{
			ID: sc.id, Label: sc.label, Rule: sc.rule, RescuedShare: rs, OrderProb: evalRound4(prob),
			ProblemSessionsMonth: math.Round(problemSessions*10) / 10, RescuedMonth: math.Round(rescued*10) / 10,
			OrdersMonth: math.Round(orders*10) / 10, RevenueRubMonth: math.Round(orders * ac.Value),
		})
	}
	return res
}
