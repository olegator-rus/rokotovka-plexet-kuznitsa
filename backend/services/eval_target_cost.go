package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"strconv"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/repositories"
)

// Сводка «На данных заказчика» () и стоимость запроса по формуле — только из прогонов наборов
// «по цели» (evaluations) и llm_usage:
//
//	C = p × (1 − h) × c_llm + (инфраструктура + агент + сопровождение) / N
//
// p — доля поисков с разбором моделью (вызов модели или разбор из кэша) по трассам прогона полного продукта. Метка
// маршрута D без вызова модели (её ставит «ничего не нашлось по тексту», zero_results) в p не входит: такой поиск
// модель не зовёт. h — доля разборов из кэша среди них; c_llm — llm_usage прогона / поисков с вызовом модели. Холодный
// прогон (кэш не читается) и тёплый считаются отдельно.

// Стенд и допущения расчёта (research 05): минимальная конфигурация — та, на которой поднимается стенд
// (всё в Docker, BGE-M3 на CPU).
const (
	feedRequestsMonth     = 15000
	standInfraRub         = 8218
	standInfraHint        = "4 vCPU / 16 ГБ / 50 ГБ SSD + IP в Яндекс Облаке, 720 часов"
	agentRunsMonthBasis   = 40
	assumedAgentRunRubFed = 8.40 // ₽ за запуск агента на Flash (research 05), пока замеров запусков нет
	supportRubMonth       = 0
	feedCostFormula       = "C = p × (1 − h) × c_llm + (инфраструктура + агент + сопровождение) / N"
)

// feedSetsOrder — наборы сводки по порядку; feedFourSets — наборы доли «≥4 товара» и стоимости.
var (
	feedSetsOrder = []string{domain.TargetSetEtalon, domain.TargetSetTZ, domain.TargetSetNeighbors, domain.TargetSetClosed,
		domain.TargetSetUnderstanding, domain.TargetSetFeedRegression, domain.TargetSetFeedTune}
	feedFourSets = []string{domain.TargetSetEtalon, domain.TargetSetTZ, domain.TargetSetNeighbors, domain.TargetSetClosed}
	// feedCostSets — наборы расчёта стоимости: все прогоны полного продукта на фиде, включая набор понимания (в нём
	// длинные запросы с условиями — маршрут D); стоимость не требует невиденных запросов.
	feedCostSets = []string{domain.TargetSetEtalon, domain.TargetSetTZ, domain.TargetSetNeighbors,
		domain.TargetSetUnderstanding, domain.TargetSetClosed}
	feedClosedCfgs = []string{domain.ConfigBM25, domain.ConfigB2, domain.ConfigLive}
	feedConfigs    = []out.FeedConfig{
		{ID: domain.ConfigBM25, Label: "BM25", Note: "текстовый поиск по запросу как набран"},
		{ID: domain.ConfigB0, Label: "Подстрока", Note: "точное совпадение подстроки в названии"},
		{ID: domain.ConfigB1, Label: "BM25 + нормализатор", Note: "раскладка, опечатки, транслит, словари"},
		{ID: domain.ConfigB2, Label: "Гибрид", Note: "BM25 + нормализатор + векторы BGE-M3, слияние RRF"},
		{ID: domain.ConfigB3, Label: "Полный путь, правила v1", Note: "без принятых правок агента"},
		{ID: domain.ConfigB4, Label: "Полный путь с правками агента", Note: "последняя принятая версия правил"},
		{ID: domain.ConfigLive, Label: "Полный продукт", Note: "понимание запроса, цель ответа, лестница «4 товара», модель на трудных запросах"},
	}
)

// feedRun — строка evaluations с разобранными метриками.
type feedRun struct {
	row domain.Evaluation
	m   out.EvalMetrics
}

func (r feedRun) mode() string {
	if r.m.Target != nil && r.m.Target.Cost != nil {
		return r.m.Target.Cost.Mode
	}
	return ""
}

// runTail — короткий номер прогона (без даты): последние 6 знаков run_id.
func runTail(id string) string {
	if len(id) > 6 {
		return id[len(id)-6:]
	}
	return id
}

// FeedSummary — GET /api/evaluations/feed: сводка экрана S6. Без базы — каталог и пустые блоки.
func (s *Services) FeedSummary(ctx context.Context) (*out.FeedSummary, error) {
	res := &out.FeedSummary{Closed: []out.FeedScore{}, Table: []out.FeedRow{}, Configs: feedConfigs}
	res.Catalog = feedCatalogOf(s.catalog.Load())
	if !s.repos.Enabled() {
		return res, nil
	}
	rows, err := s.repos.RecentEvaluations(ctx, feedSetsOrder, 600)
	if err != nil {
		return nil, err
	}
	if res.MockRuns, err = s.repos.HasEvaluationsOfCatalog(ctx, "mock"); err != nil {
		return nil, err
	}
	var runs []feedRun
	for _, r := range rows {
		if strings.HasPrefix(r.CatalogVersion, "mock") {
			continue // прогоны «по цели» имеют смысл только на фиде
		}
		var m out.EvalMetrics
		if len(r.Metrics) == 0 || json.Unmarshal(r.Metrics, &m) != nil || m.Target == nil {
			continue
		}
		runs = append(runs, feedRun{row: r, m: m})
	}
	latest := func(set, cfg, mode string) *feedRun {
		for i := range runs { // новые сверху
			r := &runs[i]
			if r.row.Set == set && r.row.Config == cfg && (mode == "" || r.mode() == mode) {
				return r
			}
		}
		return nil
	}
	for _, set := range feedSetsOrder {
		for _, c := range feedConfigs {
			if r := latest(set, c.ID, ""); r != nil {
				res.Table = append(res.Table, feedRow(*r))
			}
		}
	}
	if r := latest(domain.TargetSetEtalon, domain.ConfigLive, ""); r != nil {
		sc := feedScore(*r)
		res.Etalon = &sc
	}
	for _, cfg := range feedClosedCfgs {
		if r := latest(domain.TargetSetClosed, cfg, ""); r != nil {
			res.Closed = append(res.Closed, feedScore(*r))
		}
	}
	four := &out.FeedFour{Config: domain.ConfigLive, Sets: []string{}}
	for _, set := range feedFourSets {
		if r := latest(set, domain.ConfigLive, ""); r != nil {
			t := r.m.Target
			four.Sets = append(four.Sets, set)
			four.Answers += t.Answers
			four.Families4N += t.Families4N
			four.SKUs4N += t.SKUs4N
		}
	}
	if four.Answers > 0 {
		four.Families4 = evalPtr(evalRound4(float64(four.Families4N) / float64(four.Answers)))
		four.SKUs4 = evalPtr(evalRound4(float64(four.SKUs4N) / float64(four.Answers)))
		res.Four = four
	}
	res.Cost, err = s.feedCost(ctx, latest)
	if err != nil {
		return nil, err
	}
	// Диалог режима подбора (контур подбора): последняя оценка полного подбора (`go run . guide-eval`).
	if gv, runID, err := s.LatestGuideEval(ctx); err != nil {
		return nil, err
	} else if m := gv[GuideVariantFull]; m != nil {
		res.Guide = &out.FeedGuide{Dialogs: m.Dialogs, Runs: m.Runs, TaskAll: m.TaskAll, Gate: m.Gate,
			DialogCostRub: m.DialogCostRub, DialogLLMRub: m.DialogLLMRub, DialogsWithLLM: m.DialogsWithLLM,
			LLMCalls: m.LLMCalls, MsTotalP95: m.MsTotal.P95, RunTail: runTail(runID)}
	}
	return res, nil
}

// feedCatalogOf — источник каталога и число офферов (без версии: в ней дата снимка).
func feedCatalogOf(st *catalogState) out.FeedCatalog {
	c := out.FeedCatalog{Source: "unknown", Label: "каталог не загружен"}
	if st == nil || st.lex == nil {
		return c
	}
	c.Offers = len(st.lex.docs)
	switch {
	case strings.HasPrefix(st.version, "mock"):
		c.Source, c.Label = "mock", "mock-каталог, до данных заказчика"
	default:
		c.Source, c.Label = "feed", "фид заказчика"
	}
	return c
}

func feedScore(r feedRun) out.FeedScore {
	t := r.m.Target
	return out.FeedScore{Set: r.row.Set, Config: r.row.Config, Matched: t.Success1N, Total: t.Judged, Success1: t.Success1,
		Top4: t.Top4, RunTail: runTail(r.row.RunID)}
}

func feedRow(r feedRun) out.FeedRow {
	row := out.FeedRow{Set: r.row.Set, Config: r.row.Config, Queries: r.row.Queries, RunTail: runTail(r.row.RunID),
		Mode: r.mode(), Metrics: *r.m.Target, P50Ms: r.m.Latency.P50, P95Ms: r.m.Latency.P95}
	row.Metrics.Breakdown = nil
	if c := r.m.Target.Cost; c != nil && c.Searches > 0 {
		rub := c.TraceRub
		if c.UsageJoined {
			rub = c.LLMRub
		}
		row.RubPerQuery = evalPtr(math.Round(rub/float64(c.Searches)*1e6) / 1e6)
	}
	return row
}

// feedCost — стоимость запроса по последним холодному и тёплому прогонам полного продукта на наборах сводки.
func (s *Services) feedCost(ctx context.Context, latest func(set, cfg, mode string) *feedRun) (*out.FeedCost, error) {
	agentRub, agentHint, agentSrc := assumedAgentRunRubFed, "12 шагов на Flash (research 05): завершённых запусков с расходом нет", "assumed"
	if st := s.store(); st != nil {
		list, err := st.ListAgentRuns(ctx, 50)
		if err != nil {
			return nil, err
		}
		n, sum := 0, 0.0
		for _, r := range list {
			if r.Status == domain.RunDone && r.CostRub > 0 {
				n++
				sum += r.CostRub
			}
		}
		if n > 0 {
			agentRub = math.Round(sum/float64(n)*100) / 100
			agentHint, agentSrc = fmt.Sprintf("средняя стоимость завершённых запусков агента (%d) из agent_runs", n), "measured"
		}
	}
	fc := &out.FeedCost{Formula: feedCostFormula, Requests: feedRequestsMonth, Notes: []string{}}
	param := func(key, label, unit, hint, source string, v float64) out.CostsParam {
		return out.CostsParam{Key: key, Label: label, Unit: unit, Hint: hint, Default: v, Value: v, Source: source}
	}
	fc.Params = []out.CostsParam{
		param("requests", "Запросов в месяц (N)", "шт.", "Середина диапазона заказчика 10–20 тыс.", "assumed", feedRequestsMonth),
		param("infra_rub", "Инфраструктура стенда", "₽/мес", standInfraHint, "assumed", standInfraRub),
		param("agent_run_rub", "Стоимость запуска агента SearchOps", "₽", agentHint, agentSrc, agentRub),
		param("agent_runs_month", "Запусков агента в месяц", "шт.", ": ежедневно и около 10 по всплеску и изменению каталога", "assumed", agentRunsMonthBasis),
		param("support_rub", "Сопровождение", "₽/мес", "В расчёт не включено: часы и ставка без источника", "assumed", supportRubMonth),
	}
	agentMonth := agentRub * agentRunsMonthBasis
	// Журнал витрины и трафика на этом каталоге — для сравнения и как источник c_llm, если в прогонах модель не
	// вызывалась (доля p при этом из прогона, LLM на запрос = p × (1 − h) × c_llm).
	var journalC *float64
	if st := s.catalog.Load(); st != nil && st.version != "" {
		t, err := s.repos.CatalogRouteCosts(ctx, st.version)
		if err != nil {
			return nil, err
		}
		if t.Searches > 0 {
			jr := journalCostRun(t)
			journalC = jr.CLLM
			applyFeedCost(jr, jr.CLLM, standInfraRub, agentMonth, supportRubMonth, feedRequestsMonth)
			fc.Journal = jr
		}
	}
	var coldC *float64
	for _, mode := range []string{TargetModeCold, TargetModeWarm} {
		cr, err := s.feedCostRun(ctx, latest, mode)
		if err != nil {
			return nil, err
		}
		if cr == nil {
			continue
		}
		cl := cr.CLLM
		if cl == nil {
			cl = coldC // в тёплом прогоне модель могла не вызываться: c_llm — из холодного
		}
		if cl == nil {
			cl = journalC // в прогонах модель не вызывалась: c_llm — из журнала на этом каталоге
		}
		applyFeedCost(cr, cl, standInfraRub, agentMonth, supportRubMonth, feedRequestsMonth)
		cr.CLLM = cl
		if mode == TargetModeCold {
			coldC = cl
			fc.Cold = cr
		} else {
			fc.Warm = cr
		}
	}
	if fc.Cold == nil && fc.Warm == nil {
		return nil, nil
	}
	names := map[string]string{TargetModeCold: "Холодный прогон", TargetModeWarm: "Тёплый прогон", targetModeJournal: "Журнал витрины на фиде"}
	for _, cr := range []*out.FeedCostRun{fc.Cold, fc.Warm, fc.Journal} {
		if cr == nil {
			continue
		}
		d := int(math.Round(cr.RouteDLabel * float64(cr.Searches)))
		fc.Notes = append(fc.Notes, fmt.Sprintf("%s: маршрут D по метке — %d из %d поисков, из них %d без вызова модели "+
			"(ничего не нашлось по тексту) — в p не входят; с разбором моделью (вызов или кэш) — %s %%, из них из кэша — %s %%.",
			names[cr.Mode], d, cr.Searches, cr.RouteDNoModel, feedNum(cr.P*100, 1), feedNum(cr.H*100, 1)))
	}
	if fc.Journal != nil && fc.Journal.CLLM != nil {
		j := fc.Journal
		perCall := 0.0
		if j.LLMCalls > 0 {
			perCall = float64(j.InputTokens) / float64(j.LLMCalls)
		}
		fc.Notes = append(fc.Notes, fmt.Sprintf("c_llm — %s ₽ за поиск с вызовом модели по llm_usage поисков на этом каталоге: "+
			"%d вызовов, токенов на входе %s, на выходе %s (около %s входных на вызов — в системном промпте разбора справочник "+
			"категорий и вендоров каталога). В прогоне, где модель вызывалась, c_llm — его собственный.",
			feedNum(*j.CLLM, 4), j.LLMCalls, fmtThousands(int(j.InputTokens)), fmtThousands(int(j.OutputTokens)),
			fmtThousands(int(math.Round(perCall)))))
	}
	return fc, nil
}

// targetModeJournal — строка расчёта по журналу витрины (FeedCostRun.Mode).
const targetModeJournal = "journal"

// journalCostRun — замеры журнала витрины и трафика каталога.
func journalCostRun(t repositories.CatalogRouteTotals) *out.FeedCostRun {
	n := float64(t.Searches)
	cr := &out.FeedCostRun{Mode: targetModeJournal, Sets: []string{}, Searches: int(t.Searches),
		RouteDLabel: evalRound4(float64(t.RouteD) / n), RouteDNoModel: int(t.DNoModel),
		LLMCalls: t.Calls, InputTokens: t.InputTokens, OutputTokens: t.OutputTokens, LLMRub: evalRound4(t.CostRub),
		P50Ms: t.P50Ms, P95Ms: t.P95Ms}
	parsed := t.LLM + t.Cache
	cr.P = evalRound4(float64(parsed) / n)
	if parsed > 0 {
		cr.H = evalRound4(float64(t.Cache) / float64(parsed))
	}
	if t.LLM > 0 {
		cr.CLLM = evalPtr(math.Round(t.CostRub/float64(t.LLM)*1e4) / 1e4)
	}
	return cr
}

// feedCostRun — замеры одного режима: сумма последних прогонов полного продукта на наборах сводки.
func (s *Services) feedCostRun(ctx context.Context, latest func(set, cfg, mode string) *feedRun, mode string) (*out.FeedCostRun, error) {
	cr := &out.FeedCostRun{Mode: mode, Sets: []string{}}
	var sum out.TargetCost
	var ms []float64
	for _, set := range feedCostSets {
		r := latest(set, domain.ConfigLive, mode)
		if r == nil {
			continue
		}
		c := r.m.Target.Cost
		cr.Sets = append(cr.Sets, set)
		sum.Searches += c.Searches
		sum.RouteD += c.RouteD
		sum.RouteDNoModel += c.RouteDNoModel
		sum.ModelSearches += c.ModelSearches
		sum.CacheSearches += c.CacheSearches
		sum.LLMCalls += c.LLMCalls
		sum.InputTokens += c.InputTokens
		sum.OutputTokens += c.OutputTokens
		if c.UsageJoined {
			sum.LLMRub += c.LLMRub
		} else {
			sum.LLMRub += c.TraceRub
		}
		raw, err := s.repos.EvaluationPerQuery(ctx, r.row.RunID, r.row.Config)
		if err != nil {
			return nil, err
		}
		var pq []out.EvalQueryResult
		if json.Unmarshal(raw, &pq) == nil {
			for _, q := range pq {
				if q.Error == "" {
					ms = append(ms, q.Ms)
				}
			}
		}
	}
	if sum.Searches == 0 {
		return nil, nil
	}
	n := float64(sum.Searches)
	cr.Searches = sum.Searches
	cr.RouteDLabel = evalRound4(float64(sum.RouteD) / n)
	cr.RouteDNoModel = sum.RouteDNoModel
	parsed := sum.ModelSearches + sum.CacheSearches
	cr.P = evalRound4(float64(parsed) / n)
	if parsed > 0 {
		cr.H = evalRound4(float64(sum.CacheSearches) / float64(parsed))
	}
	if sum.ModelSearches > 0 {
		cr.CLLM = evalPtr(math.Round(sum.LLMRub/float64(sum.ModelSearches)*1e4) / 1e4)
	}
	cr.LLMCalls, cr.InputTokens, cr.OutputTokens = sum.LLMCalls, sum.InputTokens, sum.OutputTokens
	cr.LLMRub = evalRound4(sum.LLMRub)
	lat := evalLatencyOf(ms)
	cr.P50Ms, cr.P95Ms = lat.P50, lat.P95
	return cr, nil
}

// applyFeedCost — C = p × (1 − h) × c_llm + (инфраструктура + агент + сопровождение) / N и доли слагаемых.
func applyFeedCost(cr *out.FeedCostRun, cllm *float64, infra, agent, support float64, n int) {
	llm := 0.0
	if cllm != nil {
		llm = cr.P * (1 - cr.H) * *cllm
	}
	fixed := (infra + agent + support) / float64(n)
	c := llm + fixed
	cr.LLMPerRequestRub = math.Round(llm*1e6) / 1e6
	cr.FixedPerRequestRub = math.Round(fixed*1e4) / 1e4
	cr.PerRequestRub = math.Round(c*1e4) / 1e4
	cr.MonthRub = math.Round(c*float64(n)*100) / 100
	if c > 0 {
		cr.InfraShare = evalRound4(infra / float64(n) / c)
		cr.AgentShare = evalRound4(agent / float64(n) / c)
		cr.SupportShare = evalRound4(support / float64(n) / c)
		cr.LLMShare = evalRound4(llm / c)
	}
}

// feedNum — число с запятой: 1.7502 → «1,7502».
func feedNum(v float64, digits int) string {
	return strings.Replace(strconv.FormatFloat(v, 'f', digits, 64), ".", ",", 1)
}
