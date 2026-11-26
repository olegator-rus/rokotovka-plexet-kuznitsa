package services

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"math"
	"strconv"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Месячные сценарии стоимости (ARCHITECTURE §7): 20 и 200 тыс. запросов на измеренных
// долях маршрутов и кэша, стоимости разбора (llm_usage живых поисков) и запуска агента (agent_runs). Сервер
// и число запусков агента — допущения, параметры запроса.
//
//	вызовов LLM = запросы × доля маршрута D × (1 − доля попаданий в кэш)
//	онлайн-LLM  = вызовов LLM × стоимость разбора
//	агент       = запусков в месяц × стоимость запуска
//	итого       = сервер + онлайн-LLM + агент; на запрос = итого / запросы

// MonthlyParams — допущения месячного расчёта; nil — по умолчанию.
type MonthlyParams struct {
	ServerRub20k   *float64
	ServerRub200k  *float64
	AgentRunsMonth *float64
	// AgentRunsMonth200k — запусков агента в месяц при 200 тыс. запросов.
	AgentRunsMonth200k *float64
}

// Допущения по умолчанию (docs/COSTS.md): сервер 4 vCPU / 16 ГБ ≈ 7 500 ₽ при 20 тыс., 8 vCPU / 32 ГБ ≈ 15 000 ₽
// при 200 тыс.; агент — раз в день (30) при 20 тыс. и 4 раза в день (120) при 200 тыс. Когда живых поисков нет —
// доли и стоимость разбора тоже допущения.
const (
	defaultServerRub20k   = 7500
	defaultServerRub200k  = 15000
	defaultAgentRunsMonth = 30
	defaultAgentRuns200k  = 120
	assumedRouteDShare    = 0.2
	assumedCacheHitShare  = 0.3
	assumedParseCostRub   = 0.12
	assumedAgentRunRub    = 30
)

const monthlyFormula = "вызовов LLM = запросы × доля маршрута D × (1 − доля попаданий в кэш); онлайн-LLM = вызовы × " +
	"стоимость разбора; агент = запусков в месяц × стоимость запуска; итого = сервер + онлайн-LLM + агент; " +
	"на запрос = итого / запросы"

// checkMonthly — проверка допущений.
func checkMonthly(p MonthlyParams) []errs.FieldError {
	var bad []errs.FieldError
	for field, v := range map[string]*float64{"server_rub_20k": p.ServerRub20k, "server_rub_200k": p.ServerRub200k,
		"agent_runs_month": p.AgentRunsMonth, "agent_runs_month_200k": p.AgentRunsMonth200k} {
		if v != nil && (*v < 0 || *v > 1e8 || math.IsNaN(*v)) {
			bad = append(bad, errs.FieldError{Field: field, Code: errs.CodeOutOfRange, Message: "Ожидается неотрицательное число"})
		}
	}
	return bad
}

// costParam — допущение или замер для месячного расчёта.
func costParam(key, label, unit, hint string, measured *float64, assumed float64, override *float64) out.CostsParam {
	p := out.CostsParam{Key: key, Label: label, Unit: unit, Hint: hint, Default: assumed, Source: "assumed"}
	if measured != nil {
		p.Default, p.Source = *measured, "measured"
	}
	p.Value = p.Default
	if override != nil {
		p.Value, p.Source = *override, "request"
	}
	return p
}

// computeMonthly — сценарии из сводки «Экономики», маршрутов и завершённых запусков агента.
func computeMonthly(sum out.CostsSummary, routes []out.CostsRoute, runs []domain.AgentRun, p MonthlyParams) out.CostsMonthly {
	var routeD *float64
	for _, r := range routes {
		if r.Route == domain.RouteIntent && sum.Searches > 0 {
			routeD = r.Share
		}
	}
	var parseCost *float64
	if sum.LLMParses > 0 {
		parseCost = costsRatio(sum.SearchLLMCostRub, float64(sum.LLMParses), 4)
	}
	var runCost *float64
	n, total := 0, 0.0
	for _, r := range runs {
		if r.Status == domain.RunDone && r.CostRub > 0 {
			n++
			total += r.CostRub
		}
	}
	if n > 0 {
		runCost = costsRatio(total, float64(n), 2)
	}
	params := []out.CostsParam{
		costParam("route_d_share", "Доля маршрута D (сложные запросы)", "доля",
			"Замер — живые поиски по маршрутам (search_events, source = live)", routeD, assumedRouteDShare, nil),
		costParam("cache_hit_share", "Доля попаданий в кэш разборов", "доля",
			"Замер — cache / (cache + llm) среди разборов маршрута D", sum.CacheHitShare, assumedCacheHitShare, nil),
		costParam("llm_parse_cost_rub", "Стоимость одного разбора моделью", "₽",
			"Замер — расход llm_usage живых поисков / число разборов моделью (с повторами и резервом)", parseCost, assumedParseCostRub, nil),
		costParam("agent_run_cost_rub", "Стоимость одного запуска агента", "₽",
			fmt.Sprintf("Замер — средняя стоимость завершённых запусков агента (%d)", n), runCost, assumedAgentRunRub, nil),
		costParam("agent_runs_month", "Запусков агента в месяц при 20 тыс. запросов", "шт.",
			"Допущение — раз в день по расписанию", nil, defaultAgentRunsMonth, p.AgentRunsMonth),
		costParam("agent_runs_month_200k", "Запусков агента в месяц при 200 тыс. запросов", "шт.",
			"Допущение — 4 раза в день по расписанию", nil, defaultAgentRuns200k, p.AgentRunsMonth200k),
		costParam("server_rub_20k", "Сервер при 20 тыс. запросов", "₽/мес",
			"Допущение — 4 vCPU / 16 ГБ в Яндекс Облаке", nil, defaultServerRub20k, p.ServerRub20k),
		costParam("server_rub_200k", "Сервер при 200 тыс. запросов", "₽/мес",
			"Допущение — 8 vCPU / 32 ГБ в Яндекс Облаке", nil, defaultServerRub200k, p.ServerRub200k),
	}
	v := map[string]float64{}
	for _, x := range params {
		v[x.Key] = x.Value
	}
	res := out.CostsMonthly{Formula: monthlyFormula, Params: params, Scenarios: []out.CostsScenario{}}
	for _, sc := range []struct {
		requests int
		server   string
		runs     string
	}{{20000, "server_rub_20k", "agent_runs_month"}, {200000, "server_rub_200k", "agent_runs_month_200k"}} {
		calls := float64(sc.requests) * v["route_d_share"] * (1 - v["cache_hit_share"])
		llmRub := calls * v["llm_parse_cost_rub"]
		agentRub := v[sc.runs] * v["agent_run_cost_rub"]
		totalRub := v[sc.server] + llmRub + agentRub
		res.Scenarios = append(res.Scenarios, out.CostsScenario{
			Requests: sc.requests, LLMCalls: math.Round(calls), ServerRub: math.Round(v[sc.server]),
			LLMRub: math.Round(llmRub*100) / 100, AgentRub: math.Round(agentRub*100) / 100,
			TotalRub: math.Round(totalRub*100) / 100, PerRequestRub: math.Round(totalRub/float64(sc.requests)*1e4) / 1e4,
			LLMPerRequestRub: math.Round(llmRub/float64(sc.requests)*1e6) / 1e6,
		})
	}
	return res
}

// CostsExport — GET /api/costs/export: данные расчёта для шаблона организатора, CSV (UTF-8, запятая).
func (s *Services) CostsExport(ctx context.Context, p MonthlyParams) ([]byte, error) {
	res, err := s.Costs(ctx, CostsRequest{Limit: 1, Monthly: p})
	if err != nil {
		return nil, err
	}
	var buf bytes.Buffer
	buf.WriteString("\xEF\xBB\xBF") // BOM: Excel открывает UTF-8 без кракозябр
	w := csv.NewWriter(&buf)
	f := func(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }
	fp := func(v *float64) string {
		if v == nil {
			return ""
		}
		return f(*v)
	}
	_ = w.Write([]string{"раздел", "показатель", "ключ", "значение", "единица", "источник", "пояснение"})
	sm := res.Summary
	rows := [][]string{
		{"замер", "Живых поисков в журнале", "searches", strconv.FormatInt(sm.Searches, 10), "шт.", "measured", "search_events, source = live"},
		{"замер", "Средняя стоимость LLM на поиск", "avg_cost_rub", fp(sm.AvgCostRub), "₽", "measured", "llm_usage, привязанные к поискам"},
		{"замер", "Доля поисков без LLM", "share_without_llm", fp(sm.ShareWithoutLLM), "доля", "measured", "правила или кэш"},
		{"замер", "Разборов моделью", "llm_parses", strconv.FormatInt(sm.LLMParses, 10), "шт.", "measured", ""},
		{"замер", "Попаданий в кэш", "cache_hits", strconv.FormatInt(sm.CacheHits, 10), "шт.", "measured", ""},
		{"замер", "Средняя задержка вызова LLM", "avg_llm_latency_ms", fp(sm.AvgLLMLatencyMs), "мс", "measured", ""},
	}
	for _, r := range res.Routes {
		rows = append(rows, []string{"маршрут " + r.Route, "Доля / среднее время / средняя стоимость", "route_" + r.Route,
			fp(r.Share), "доля", "measured", fmt.Sprintf("время %s мс, стоимость %s ₽, поисков %d", fp(r.AvgMs), fp(r.AvgCostRub), r.Count)})
	}
	for _, x := range res.Monthly.Params {
		rows = append(rows, []string{"вход расчёта", x.Label, x.Key, f(x.Value), x.Unit, x.Source, x.Hint})
	}
	for _, sc := range res.Monthly.Scenarios {
		sec := fmt.Sprintf("месяц %d запросов", sc.Requests)
		rows = append(rows,
			[]string{sec, "Вызовов LLM", "llm_calls", f(sc.LLMCalls), "шт.", "calculated", ""},
			[]string{sec, "Сервер", "server_rub", f(sc.ServerRub), "₽", "calculated", ""},
			[]string{sec, "Онлайн-LLM", "llm_rub", f(sc.LLMRub), "₽", "calculated", ""},
			[]string{sec, "Агент", "agent_rub", f(sc.AgentRub), "₽", "calculated", ""},
			[]string{sec, "Итого", "total_rub", f(sc.TotalRub), "₽", "calculated", ""},
			[]string{sec, "На запрос", "per_request_rub", f(sc.PerRequestRub), "₽", "calculated", ""},
		)
	}
	for _, pr := range res.Pricing.Prices {
		rows = append(rows, []string{"прайс " + res.Pricing.Version, pr.Model, "price_" + pr.Model,
			f(pr.Input) + " / " + f(pr.Output), "₽ за 1 млн токенов (вход / выход)", "pricing", res.Pricing.Date})
	}
	rows = append(rows, []string{"формула", "Месячный расчёт", "formula", res.Monthly.Formula, "", "", ""})
	_ = w.WriteAll(rows)
	return buf.Bytes(), w.Error()
}
