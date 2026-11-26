package out

import "time"

// PanelEvaluation — GET /api/evaluations?set=panel: последний прогон набора
// префиксов по прежним подсказкам и по панели контура выдачи, плюс учёт намерений по событиям стенда.
type PanelEvaluation struct {
	Set string `json:"set"` // panel
	// Run — последний прогон; nil — прогона не было (make eval SET=panel).
	Run *PanelEvalRun `json:"run"`
	// Intents — две конверсии, пустые выдачи и подсказки за 30 дней по событиям стенда; nil — без базы.
	Intents *IntentAnalytics `json:"intents"`
	// IntentsDays — период учёта намерений, дней.
	IntentsDays int `json:"intents_days"`
}

// PanelEvalRun — прогон набора префиксов.
type PanelEvalRun struct {
	RunID          string            `json:"run_id"`
	Set            string            `json:"set"`
	SetHash        string            `json:"set_hash"`
	Prefixes       int               `json:"prefixes"`
	Commit         string            `json:"commit"`
	CatalogVersion string            `json:"catalog_version"`
	StartedAt      time.Time         `json:"started_at"`
	DurationMs     int64             `json:"duration_ms"`
	Split          PanelSplit        `json:"split"`
	Modes          []PanelModeResult `json:"modes"`
	// Changes — префиксы, где панель контура выдачи лучше или хуже прежних подсказок (ухудшения рядом с улучшениями).
	Changes  []PanelChange `json:"changes"`
	Repro    string        `json:"repro"`
	Warnings []string      `json:"warnings"`
}

// PanelSplit — деление журнала: индекс подсказок строится на раннем периоде, цели набора — из позднего
// (отложенная неделя агента).
type PanelSplit struct {
	LateFrom    *time.Time `json:"late_from"`
	LateTo      *time.Time `json:"late_to"`
	LateTargets int        `json:"late_targets"` // префиксов с целью из позднего периода
	Note        string     `json:"note"`
}

// PanelModeResult — один режим прогона: old (прежние подсказки контура коммерции, флаг панели выключен) | new (панель).
type PanelModeResult struct {
	Mode      string `json:"mode"`
	Label     string `json:"label"`
	Available bool   `json:"available"`
	// Reason — почему режим не прогнан (Available = false).
	Reason    string                  `json:"reason,omitempty"`
	Metrics   PanelMetrics            `json:"metrics"`
	ByType    map[string]PanelMetrics `json:"by_type"`
	PerPrefix []PanelPrefixResult     `json:"per_prefix"`
}

// PanelMetrics — метрики режима. Доли — 0..1; nil — не посчитано (нет знаменателя или режим не прогнан).
type PanelMetrics struct {
	Prefixes int `json:"prefixes"`
	Errors   int `json:"errors"`
	// Подсказки: место целевого запроса среди первых 5 (префиксы с целью-запросом, кроме no_results).
	SuggestN        int      `json:"suggest_n"`
	SuggestMRR5     *float64 `json:"suggest_mrr5"`
	SuggestSuccess5 *float64 `json:"suggest_success5"`
	// Товары: место целевого семейства среди первых 5 товаров панели (префиксы с целевым семейством).
	ProductN        int      `json:"product_n"`
	ProductMRR5     *float64 `json:"product_mrr5"`
	ProductSuccess5 *float64 `json:"product_success5"`
	// Сэкономленные нажатия: по целям позднего периода — длина цели минус кратчайший префикс, на котором цель
	// в первых 5 подсказках (не нашлась — 0). SavedShare — сэкономлено / длина целей.
	SavedTargets    int      `json:"saved_targets"`
	SavedKeystrokes *float64 `json:"saved_keystrokes"`
	SavedShare      *float64 `json:"saved_share"`
	// Пустые подсказки: префиксов (кроме no_results) без единой подсказки.
	EmptySuggest     int      `json:"empty_suggest"`
	EmptySuggestRate *float64 `json:"empty_suggest_rate"`
	// Разнообразие семейств в 6 слотах товаров: префиксов с двумя вариантами одного семейства.
	ProductPrefixes   int      `json:"product_prefixes"` // префиксов с непустой секцией товаров
	FamilyDupPrefixes int      `json:"family_dup_prefixes"`
	FamilyDupRate     *float64 `json:"family_dup_rate"`
	DistinctFamilies  *float64 `json:"distinct_families"` // среднее различных семейств в слотах
	// Точные не вытеснены: префикс = полный артикул или парт-номер → первый товар — эта позиция (семейство).
	ExactChecks    int `json:"exact_checks"`
	ExactDisplaced int `json:"exact_displaced"`
	// Подсказки в пустую выдачу: различные показанные подсказки, у которых текстовая ветка поиска (B1) — 0.
	SuggestionsChecked int      `json:"suggestions_checked"`
	SuggestionsToEmpty int      `json:"suggestions_to_empty"`
	ToEmptyExamples    []string `json:"to_empty_examples"`
	// Задержка сервера (в процессе, мс): первый проход (промах кэша) и повтор (из кэша).
	P50Ms       *float64 `json:"p50_ms"`
	P95Ms       *float64 `json:"p95_ms"`
	P50MsCached *float64 `json:"p50_ms_cached"`
	P95MsCached *float64 `json:"p95_ms_cached"`
	// ProductsP95Ms — p95 секции товаров по meta.ms.products (только панель контура выдачи).
	ProductsP95Ms *float64 `json:"products_p95_ms"`
	Partial       int      `json:"partial"` // ответов без товаров: секция не уложилась в таймаут
	LLMCalls      int      `json:"llm_calls"`
	EmbedCalls    int      `json:"embed_calls"`
	RerankCalls   int      `json:"rerank_calls"`
}

// PanelPrefixResult — один префикс в режиме.
type PanelPrefixResult struct {
	ID           string   `json:"id"`
	Prefix       string   `json:"prefix"`
	Type         string   `json:"type"`
	TargetQuery  string   `json:"target_query"`
	TargetFamily string   `json:"target_family,omitempty"`
	Suggestions  []string `json:"suggestions"`
	Families     []string `json:"families"`
	SuggestRank  int      `json:"suggest_rank"` // 1..5; 0 — не в первых 5
	ProductRank  int      `json:"product_rank"`
	FamilyDup    bool     `json:"family_dup"`
	ExactOK      *bool    `json:"exact_ok"` // nil — не проверка точного
	Ms           float64  `json:"ms"`
	Error        string   `json:"error,omitempty"`
}

// PanelChange — префикс, где панель контура выдачи лучше или хуже прежних подсказок.
type PanelChange struct {
	ID      string `json:"id"`
	Prefix  string `json:"prefix"`
	Type    string `json:"type"`
	Target  string `json:"target"`
	Section string `json:"section"` // suggestions | products
	OldRank int    `json:"old_rank"`
	NewRank int    `json:"new_rank"`
	Kind    string `json:"kind"` // improvement | regression
}
