package domain

// Трасса поиска: отдаётся в ответе и сохраняется в search_traces.
// Шаги идут в фиксированном порядке; пропущенный шаг остаётся в списке со skipped: true. Массивы шагов
// никогда не null: пустой список — пустой массив.

// Имена шагов трассы по порядку.
const (
	StepNormalize  = "normalize"
	StepAttributes = "attributes"
	StepRoute      = "route"
	StepIntent     = "intent"
	StepBM25       = "bm25"
	StepDense      = "dense"
	StepFusion     = "fusion"
	StepRerank     = "rerank"
	StepBusiness   = "business"
	StepRelaxation = "relaxation"
)

// Маршруты запроса (ARCHITECTURE §2.2).
const (
	RouteExact      = "A"
	RouteNormalized = "B"
	RouteSemantic   = "C"
	RouteIntent     = "D"
)

// Источник разбора запроса.
const (
	ParseRules = "rules"
	ParseCache = "cache"
	ParseLLM   = "llm"
)

// Группы выдачи.
const (
	GroupExact  = "exact"
	GroupMatch  = "match"
	GroupAnalog = "analog"
)

// Trace — трасса одного поиска.
type Trace struct {
	RequestID   string `json:"request_id"`
	Route       string `json:"route"`
	ParseSource string `json:"parse_source"`
	Config      string `json:"config"`
	// EffectiveConfig — конфигурация, которая реально отработала: пока B3 и B4 не реализованы, они
	// работают как B2, и трасса говорит об этом прямо.
	EffectiveConfig string        `json:"effective_config"`
	RulesVersion    int           `json:"rules_version"`
	CatalogVersion  string        `json:"catalog_version"`
	TotalMs         float64       `json:"total_ms"`
	CostRub         float64       `json:"cost_rub"`
	Steps           []any         `json:"steps"`
	Items           []TraceItem   `json:"items"`
	Rejected        []RejectedHit `json:"rejected"`
	LLM             any           `json:"llm"`
}

// StepBase — общие поля шага. Reason — почему шаг пропущен или чем закончился сбой («пропущен» и «упал» в трассе различимы).
type StepBase struct {
	Name    string  `json:"name"`
	Ms      float64 `json:"ms"`
	Skipped bool    `json:"skipped,omitempty"`
	Reason  string  `json:"reason,omitempty"`
}

// Change — одно изменение запроса на шаге нормализации.
type Change struct {
	Kind string `json:"kind"` // layout | typo | alias | clean
	From string `json:"from"`
	To   string `json:"to"`
}

// NormalizeStep — шаг normalize.
type NormalizeStep struct {
	StepBase
	Changes []Change `json:"changes"`
}

// FoundAttribute — атрибут, извлечённый из запроса.
type FoundAttribute struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Strict bool   `json:"strict"`
	Source string `json:"source"`
}

// AttributesStep — шаг attributes.
type AttributesStep struct {
	StepBase
	Found      []FoundAttribute `json:"found"`
	Unresolved []string         `json:"unresolved"`
}

// RouteStep — шаг route.
type RouteStep struct {
	StepBase
	Route   string   `json:"route"`
	Signals []string `json:"signals"`
}

// RankedHit — позиция в списке кандидатов одного источника.
type RankedHit struct {
	ProductID string  `json:"product_id"`
	Rank      int     `json:"rank"`
	Score     float64 `json:"score"`
}

// RetrievalStep — шаги bm25 и dense. Mode (дополнение к контракту) — каким запросом отобраны
// кандидаты: substring (B0) или multi_match (B1+), knn для dense.
type RetrievalStep struct {
	StepBase
	Mode string      `json:"mode,omitempty"`
	Top  []RankedHit `json:"top"`
}

// FusionStep — шаг fusion (RRF).
type FusionStep struct {
	StepBase
	K int `json:"k"`
}

// BusinessStep — шаг business.
type BusinessStep struct {
	StepBase
	Weights map[string]float64 `json:"weights"`
}

// RelaxationStep — шаг relaxation.
type RelaxationStep struct {
	StepBase
	Ladder []string `json:"ladder"`
}

// ScoreParts — слагаемые итогового скора.
type ScoreParts struct {
	Relevance  float64 `json:"relevance"`
	Popularity float64 `json:"popularity"`
	Margin     float64 `json:"margin"`
	Context    float64 `json:"context"`
}

// TraceItem — позиция выдачи и откуда она взялась. Ранг nil — источник позицию не нашёл.
type TraceItem struct {
	ProductID string     `json:"product_id"`
	Final     float64    `json:"final"`
	Group     string     `json:"group"`
	Parts     ScoreParts `json:"parts"`
	BM25Rank  *int       `json:"bm25_rank"`
	DenseRank *int       `json:"dense_rank"`
}

// RejectedHit — кандидат, исключённый из выдачи, и причина.
type RejectedHit struct {
	ProductID string `json:"product_id"`
	Reason    string `json:"reason"`
	Detail    string `json:"detail,omitempty"`
}
