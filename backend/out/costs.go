package out

import "time"

// Costs — GET /api/costs: экономика поиска по журналам search_events и llm_usage. Непосчитанное — null
// (например, доля попаданий в кэш, пока не было ни одного разбора).
type Costs struct {
	LLMMode string       `json:"llm_mode"`
	Pricing CostsPricing `json:"pricing"`
	Summary CostsSummary `json:"summary"`
	Routes  []CostsRoute `json:"routes"`
	Calls   CostsCalls   `json:"calls"`
	// JournalHidden — открытая сводка (GET /api/costs/summary): журнал вызовов только сотруднику, calls пуст.
	JournalHidden bool `json:"journal_hidden,omitempty"`
	// Monthly — месячные сценарии 20 и 200 тыс. запросов.
	Monthly CostsMonthly `json:"monthly"`
}

// CostsMonthly — месячный расчёт: допущения и замеры, сценарии, формула.
type CostsMonthly struct {
	Formula   string          `json:"formula"`
	Params    []CostsParam    `json:"params"`
	Scenarios []CostsScenario `json:"scenarios"`
}

// CostsParam — входная величина расчёта: замер (measured), допущение (assumed) или задано в запросе (request).
type CostsParam struct {
	Key     string  `json:"key"`
	Label   string  `json:"label"`
	Value   float64 `json:"value"`
	Default float64 `json:"default"`
	Source  string  `json:"source"`
	Unit    string  `json:"unit"`
	Hint    string  `json:"hint"`
}

// CostsScenario — один месяц: строки «сервер», «онлайн-LLM», «агент», «итого», «на запрос».
type CostsScenario struct {
	Requests         int     `json:"requests"`
	LLMCalls         float64 `json:"llm_calls"`
	ServerRub        float64 `json:"server_rub"`
	LLMRub           float64 `json:"llm_rub"`
	AgentRub         float64 `json:"agent_rub"`
	TotalRub         float64 `json:"total_rub"`
	PerRequestRub    float64 `json:"per_request_rub"`
	LLMPerRequestRub float64 `json:"llm_per_request_rub"`
}

// CostsPricing — прайс, по которому посчитаны calculated_cost.
type CostsPricing struct {
	Version  string       `json:"version"`
	Date     string       `json:"date"`
	Currency string       `json:"currency"`
	Prices   []CostsPrice `json:"prices"`
}

// CostsPrice — цена модели, ₽ за 1 млн токенов.
type CostsPrice struct {
	Model  string  `json:"model"`
	Input  float64 `json:"input_rub_per_mtok"`
	Output float64 `json:"output_rub_per_mtok"`
}

// CostsSummary — плитки «Экономики».
type CostsSummary struct {
	Searches int64 `json:"searches"`
	// AvgCostRub — стоимость вызовов модели, привязанных к поискам, на один поиск.
	AvgCostRub *float64 `json:"avg_cost_rub"`
	// ShareWithoutLLM — доля поисков, где модель не вызывалась (правила или кэш).
	ShareWithoutLLM *float64 `json:"share_without_llm"`
	LLMParses       int64    `json:"llm_parses"`
	CacheHits       int64    `json:"cache_hits"`
	// CacheHitShare — попадания в кэш среди разборов маршрута D (cache / (cache + llm)).
	CacheHitShare    *float64 `json:"cache_hit_share"`
	SearchLLMCostRub float64  `json:"search_llm_cost_rub"`
	LLMCalls         int64    `json:"llm_calls"`
	LLMCostRub       float64  `json:"llm_cost_rub"`
	InputTokens      int64    `json:"input_tokens"`
	OutputTokens     int64    `json:"output_tokens"`
	AvgLLMLatencyMs  *float64 `json:"avg_llm_latency_ms"`
	// EvalCalls, EvalCostRub — вызовы модели из прогонов оценки (request_id eval-…): в стоимость запроса
	// покупателя не входят, показываются отдельно.
	EvalCalls   int64   `json:"eval_calls"`
	EvalCostRub float64 `json:"eval_cost_rub"`
	// JudgeCalls, JudgeCostRub — вызовы LLM-судьи (request_id judge-…): тоже вне стоимости запроса.
	JudgeCalls   int64   `json:"judge_calls"`
	JudgeCostRub float64 `json:"judge_cost_rub"`
	// WarmupCalls, WarmupCostRub — прогрев кэша разборов (request_id warmup-…): вне стоимости запроса.
	WarmupCalls        int64    `json:"warmup_calls"`
	WarmupCostRub      float64  `json:"warmup_cost_rub"`
	AgentRuns          int64    `json:"agent_runs"`
	AgentRunAvgCostRub *float64 `json:"agent_run_avg_cost_rub"`
}

// CostsRoute — строка таблицы по маршрутам A–D.
type CostsRoute struct {
	Route      string   `json:"route"`
	Count      int64    `json:"count"`
	Share      *float64 `json:"share"`
	AvgMs      *float64 `json:"avg_ms"`
	AvgCostRub *float64 `json:"avg_cost_rub"`
	LLMParses  int64    `json:"llm_parses"`
	CacheHits  int64    `json:"cache_hits"`
}

// CostsCalls — страница журнала вызовов модели.
type CostsCalls struct {
	Total  int64       `json:"total"`
	Limit  int         `json:"limit"`
	Offset int         `json:"offset"`
	Items  []CostsCall `json:"items"`
}

// CostsCall — один вызов модели. request_id ведёт к трассе поиска, agent_run_id — к запуску агента.
type CostsCall struct {
	ID string `json:"id"`
	// Kind — search (поиск покупателя) | eval (прогон оценки) | judge (LLM-судья) | warmup (прогрев кэша) |
	// agent (запуск агента) | other (ручной прогон).
	Kind      string    `json:"kind"`
	CreatedAt time.Time `json:"created_at"`
	RequestID *string   `json:"request_id"`
	// Query — исходный запрос поиска, к которому относится вызов; null — вызов не из поиска или поиск
	// не записан в журнал (прогон оценки, судья, агент).
	Query          *string `json:"query"`
	AgentRunID     *string `json:"agent_run_id"`
	Provider       string  `json:"provider"`
	Model          string  `json:"model"`
	InputTokens    int     `json:"input_tokens"`
	OutputTokens   int     `json:"output_tokens"`
	CachedTokens   int     `json:"cached_tokens"`
	LatencyMs      int     `json:"latency_ms"`
	CostRub        float64 `json:"cost_rub"`
	PricingVersion string  `json:"pricing_version"`
	Status         string  `json:"status"`
}
