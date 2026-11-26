package out

import "time"

// Оценка качества: прогоны конфигураций на наборах запросов (GET /api/evaluations, `go run . eval`).
// Доли — 0..1; nil — метрику не на чем считать (в наборе нет запросов нужного типа).

// EvalMetrics — метрики конфигурации на наборе или на одном типе запросов.
type EvalMetrics struct {
	Queries int `json:"queries"`
	// EmptyRate — доля пустых выдач; EmptyRateWeighted — она же, взвешенная по freq.
	EmptyRate         *float64 `json:"empty_rate"`
	EmptyRateWeighted *float64 `json:"empty_rate_weighted"`
	// NoRelevantTop10 — выдача есть, а релевантного (оценка ≥2) в топ-10 нет; среди запросов, у которых
	// релевантное в разметке есть.
	NoRelevantTop10 *float64 `json:"no_relevant_top10"`
	// RelevantTop10 — есть релевантное (≥2) в топ-10; среди запросов, у которых оно есть в разметке.
	RelevantTop10 *float64 `json:"relevant_top10"`
	NDCG10        *float64 `json:"ndcg10"`
	MRR10         *float64 `json:"mrr10"`
	// Recall50 — доля релевантных (≥2) семейств разметки, попавших в топ-50.
	Recall50 *float64 `json:"recall50"`
	// ExactAccuracy — exact/sku: на 1-м месте позиция с оценкой 3.
	ExactAccuracy *float64 `json:"exact_accuracy"`
	// RescuedRate — проблемные типы (typo/layout/slang/alias_gap/license/analog): релевантное в топ-10.
	RescuedRate *float64 `json:"rescued_rate"`
	// FalseRescueRate — absent/noise: выдача непустая, без «нет в каталоге» и без пометки аналога.
	FalseRescueRate *float64 `json:"false_rescue_rate"`
	// ViolationRate — license/renewal: в топ-10 есть позиция нужного продукта с не той метрикой,
	// количеством или типом лицензии; ViolationItems — сколько таких позиций всего.
	ViolationRate  *float64 `json:"violation_rate"`
	ViolationItems int      `json:"violation_items"`
	// Errors — запросы, на которых поиск вернул ошибку (в метрики не входят).
	Errors  int                    `json:"errors"`
	Latency EvalLatency            `json:"latency"`
	ByRoute map[string]EvalLatency `json:"by_route,omitempty"`
}

// EvalLatency — время ответа сервиса поиска, мс.
type EvalLatency struct {
	Count int     `json:"count"`
	P50   float64 `json:"p50"`
	P95   float64 `json:"p95"`
}

// EvalQueryResult — один запрос в одной конфигурации.
type EvalQueryResult struct {
	ID    string `json:"id"`
	Text  string `json:"text"`
	Type  string `json:"type"`
	Freq  int    `json:"freq"`
	Route string `json:"route,omitempty"`
	// Top — product_id первых 10 позиций выдачи, Grades — их оценки по разметке.
	Top         []string `json:"top"`
	Grades      []int    `json:"grades"`
	Total       int      `json:"total"`
	Empty       bool     `json:"empty"`
	NDCG10      *float64 `json:"ndcg10"`
	MRR10       *float64 `json:"mrr10"`
	Recall50    *float64 `json:"recall50"`
	HasRelevant bool     `json:"has_relevant"`
	ExactHit    *bool    `json:"exact_hit,omitempty"`
	Rescued     *bool    `json:"rescued,omitempty"`
	FalseRescue *bool    `json:"false_rescue,omitempty"`
	Violations  int      `json:"violations"`
	Ms          float64  `json:"ms"`
	Error       string   `json:"error,omitempty"`
}

// EvalChange — запрос, по которому конфигурация хуже (или лучше) предыдущей в лестнице прогона.
type EvalChange struct {
	QueryID string `json:"query_id"`
	Text    string `json:"text"`
	Type    string `json:"type"`
	Freq    int    `json:"freq"`
	// Against — с какой конфигурацией сравнивали.
	Against   string   `json:"against"`
	NDCGFrom  *float64 `json:"ndcg_from"`
	NDCGTo    *float64 `json:"ndcg_to"`
	NDCGDelta float64  `json:"ndcg_delta"`
	// Reasons — что изменилось: ndcg | lost_relevant | became_empty | lost_exact | new_violation |
	// false_rescue (и обратные для улучшений: gained_relevant | no_longer_empty | gained_exact |
	// fixed_violation | false_rescue_fixed).
	Reasons []string `json:"reasons"`
}

// EvalConfigResult — одна конфигурация в прогоне.
type EvalConfigResult struct {
	Config         string                 `json:"config"`
	RulesVersion   int                    `json:"rules_version"`
	RulesOverride  bool                   `json:"rules_override"`
	CatalogVersion string                 `json:"catalog_version"`
	Metrics        EvalMetrics            `json:"metrics"`
	ByType         map[string]EvalMetrics `json:"by_type"`
	// Against — предыдущая конфигурация прогона, с которой сравнивались ухудшения; "" у первой.
	Against      string            `json:"against,omitempty"`
	Regressions  []EvalChange      `json:"regressions"`
	Improvements []EvalChange      `json:"improvements"`
	PerQuery     []EvalQueryResult `json:"per_query,omitempty"`
	DurationMs   int64             `json:"duration_ms"`
	Warnings     []string          `json:"warnings,omitempty"`
}

// EvalRun — прогон: набор, конфигурации, строка воспроизводимости.
type EvalRun struct {
	RunID      string    `json:"run_id"`
	Set        string    `json:"set"`
	SetHash    string    `json:"set_hash"`
	Queries    int       `json:"queries"`
	Commit     string    `json:"commit"`
	StartedAt  time.Time `json:"started_at"`
	DurationMs int64     `json:"duration_ms"`
	// Repro — строка воспроизводимости: набор@хэш · правила vN · каталог · коммит.
	Repro   string             `json:"repro"`
	Configs []EvalConfigResult `json:"configs"`
	// File — куда записан JSON прогона (только CLI).
	File string `json:"file,omitempty"`
}

// EvaluationsResponse — GET /api/evaluations: последний прогон каждой конфигурации на наборе.
type EvaluationsResponse struct {
	Set     string           `json:"set"`
	Configs []EvaluationView `json:"configs"`
	Running *EvalJob         `json:"running"`
	LastJob *EvalJob         `json:"last_job"`
}

// EvaluationView — сохранённый результат конфигурации.
type EvaluationView struct {
	RunID          string                 `json:"run_id"`
	Set            string                 `json:"set"`
	SetHash        string                 `json:"set_hash"`
	Queries        int                    `json:"queries"`
	Config         string                 `json:"config"`
	RulesVersion   int                    `json:"rules_version"`
	RulesOverride  bool                   `json:"rules_override"`
	CatalogVersion string                 `json:"catalog_version"`
	Commit         string                 `json:"commit"`
	Repro          string                 `json:"repro"`
	CreatedAt      time.Time              `json:"created_at"`
	DurationMs     int                    `json:"duration_ms"`
	Metrics        EvalMetrics            `json:"metrics"`
	ByType         map[string]EvalMetrics `json:"by_type"`
	Against        string                 `json:"against,omitempty"`
	Regressions    []EvalChange           `json:"regressions"`
	Improvements   []EvalChange           `json:"improvements"`
}

// EvalJob — асинхронный прогон (POST /api/evaluations/run).
type EvalJob struct {
	ID         string     `json:"id"`
	Set        string     `json:"set"`
	Configs    []string   `json:"configs"`
	Status     string     `json:"status"` // running | done | failed
	StartedAt  time.Time  `json:"started_at"`
	FinishedAt *time.Time `json:"finished_at,omitempty"`
	RunID      string     `json:"run_id,omitempty"`
	Error      string     `json:"error,omitempty"`
}
