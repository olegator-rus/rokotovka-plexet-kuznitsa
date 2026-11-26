package out

import "time"

// Вклад компонентов (контур доводки): лестница «точное совпадение → текст → векторы → разбор намерения →
// реранкер» и прогоны полного пути с одним выключенным компонентом на замороженном наборе.
// GET /api/evaluations/contribution, `make eval SET=contribution`.

// VectorsState — режим векторов (эмбеддер и реранкер) для /api/meta и /api/admin/vectors-mode.
type VectorsState struct {
	// Mode — on | off: идут ли запросы в эмбеддер и реранкер. Переключается без перезапуска.
	Mode string `json:"mode"`
	// Online — сервис моделей настроен (EMBEDDER_URL или RERANKER_URL).
	Online    bool       `json:"online"`
	ChangedAt *time.Time `json:"changed_at"`
	// Offline — выключены и LLM, и векторы: режим «без моделей».
	Offline bool `json:"offline"`
	// Banner — текст плашки режима; null — плашка не нужна.
	Banner *string `json:"banner"`
}

// ModelsState — ответ POST /api/admin/offline-mode: оба режима после переключения.
type ModelsState struct {
	LLM     LLMState     `json:"llm"`
	Vectors VectorsState `json:"vectors"`
}

// ContributionModes — что выключено в строке таблицы вклада.
type ContributionModes struct {
	LLM     bool `json:"llm"`
	Vectors bool `json:"vectors"`
	Rerank  bool `json:"rerank"`
}

// ContributionCost — стоимость и вызовы моделей строки, по трассам прогона.
type ContributionCost struct {
	// LLMRubPer1k — ₽ на LLM за 1 000 запросов прогона по трассам (кэш разборов — как на стенде в момент прогона).
	LLMRubPer1k float64 `json:"llm_rub_per_1k"`
	LLMCalls    int     `json:"llm_calls"`
	CacheHits   int     `json:"cache_hits"`
	// RouteDShare — доля запросов прогона на маршруте D (там зовётся модель).
	RouteDShare float64 `json:"route_d_share"`
	// ColdLLMRubPer1k — верхняя граница без кэша: доля D × стоимость разбора × 1 000; nil — LLM в строке выключен.
	ColdLLMRubPer1k *float64 `json:"cold_llm_rub_per_1k"`
	// EmbedQueries / RerankQueries — запросов, где шаг dense / rerank реально выполнился.
	EmbedQueries  int `json:"embed_queries"`
	RerankQueries int `json:"rerank_queries"`
}

// ContributionRow — строка таблицы вклада.
type ContributionRow struct {
	ID string `json:"id"`
	// Kind — ladder (лестница) | single_change (полный путь без одного компонента).
	Kind   string `json:"kind"`
	Label  string `json:"label"`
	Config string `json:"config"`
	// Off — что выключено поверх конфигурации.
	Off ContributionModes `json:"off"`
	// Against — с какой строкой сравнивались ухудшения и улучшения; "" у первой.
	Against      string           `json:"against"`
	Metrics      EvalMetrics      `json:"metrics"`
	Cost         ContributionCost `json:"cost"`
	Regressions  int              `json:"regressions"`
	Improvements int              `json:"improvements"`
	// RegressionHead / ImprovementHead — до 5 запросов для разбора.
	RegressionHead  []EvalChange `json:"regression_head"`
	ImprovementHead []EvalChange `json:"improvement_head"`
	DurationMs      int64        `json:"duration_ms"`
}

// ContributionCheck — блокирующая проверка прогона.
type ContributionCheck struct {
	ID       string `json:"id"`
	Label    string `json:"label"`
	Passed   bool   `json:"passed"`
	Value    string `json:"value"`
	Blocking bool   `json:"blocking"`
}

// ContributionDecision — предварительное решение по компоненту: полный путь против полного пути без него.
type ContributionDecision struct {
	Component    string   `json:"component"` // rerank | vectors | llm
	Label        string   `json:"label"`
	DeltaNDCG    *float64 `json:"delta_ndcg"`
	DeltaP95Ms   float64  `json:"delta_p95_ms"`
	Improvements int      `json:"improvements"`
	Regressions  int      `json:"regressions"`
	// Verdict — keep | remove | borderline.
	Verdict string `json:"verdict"`
	Text    string `json:"text"`
	// Preliminary — решение на mock-данных; окончательное — на данных задачи.
	Preliminary bool `json:"preliminary"`
}

// ContributionRun — прогон таблицы вклада.
type ContributionRun struct {
	RunID          string                 `json:"run_id"`
	Set            string                 `json:"set"`
	SetHash        string                 `json:"set_hash"`
	Queries        int                    `json:"queries"`
	Commit         string                 `json:"commit"`
	StartedAt      time.Time              `json:"started_at"`
	DurationMs     int64                  `json:"duration_ms"`
	RulesVersion   int                    `json:"rules_version"`
	CatalogVersion string                 `json:"catalog_version"`
	Rows           []ContributionRow      `json:"rows"`
	Checks         []ContributionCheck    `json:"checks"`
	Decisions      []ContributionDecision `json:"decisions"`
	// ParseCostRub — стоимость одного разбора моделью для верхней границы без кэша; ParseCostSource — measured | assumed.
	ParseCostRub    float64  `json:"parse_cost_rub"`
	ParseCostSource string   `json:"parse_cost_source"`
	Notes           []string `json:"notes"`
	Repro           string   `json:"repro"`
}

// ContributionResponse — GET /api/evaluations/contribution?set=test.
type ContributionResponse struct {
	Set string `json:"set"`
	// Run — последний записанный прогон набора; null — прогона не было.
	Run     *ContributionRun `json:"run"`
	Running *EvalJob         `json:"running"`
	LastJob *EvalJob         `json:"last_job"`
	// Modes — действующие режимы LLM и векторов (переключатель «без моделей» — у сотрудников).
	Modes ModelsState `json:"modes"`
	Notes []string    `json:"notes"`
}
