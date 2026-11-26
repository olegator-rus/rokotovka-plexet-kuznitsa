package out

import "time"

// CommerceEval — GET /api/evaluations/commerce?set=p2 (контур коммерции): коммерческая лестница C0–C3 и
// прогоны с одним изменённым компонентом. Метрики, которые не посчитаны (компонент не готов), — null; почему — в Notes.
type CommerceEval struct {
	RunID         string           `json:"run_id"`
	Set           string           `json:"set"`
	SetHash       string           `json:"set_hash"`
	Queries       int              `json:"queries"`
	Pairs         int              `json:"pairs"`
	Commit        string           `json:"commit"`
	StartedAt     time.Time        `json:"started_at"`
	DurationMs    int64            `json:"duration_ms"`
	Repro         string           `json:"repro"`
	Synthetic     bool             `json:"synthetic"`
	MarginWeight  float64          `json:"margin_weight"`
	Ready         CommerceReady    `json:"ready"`
	Ladder        []CommerceConfig `json:"ladder"`
	SingleChanges []CommerceConfig `json:"single_changes"`
	Notes         []string         `json:"notes"`
}

// CommerceReady — какие компоненты контура коммерции дали шаги в трассе (готовы к сравнению).
type CommerceReady struct {
	Hardware   bool `json:"hardware"`
	Companions bool `json:"companions"`
	Margin     bool `json:"margin"`
}

// CommerceConfig — одна конфигурация лестницы.
type CommerceConfig struct {
	Config string `json:"config"` // C0 | C1 | C2 | C3 | C0+companions | C3-hardware …
	// Base и Changed — у прогона с одним изменением: от какой конфигурации и что изменено (+companions, -hardware).
	Base           string   `json:"base,omitempty"`
	Changed        string   `json:"changed,omitempty"`
	Against        string   `json:"against,omitempty"`
	Features       Features `json:"features"`
	RulesVersion   int      `json:"rules_version"`
	CatalogVersion string   `json:"catalog_version"`
	// RelationsVersion — версия справочника связей прогона (C0–C3 — связи каталога, C4 — действующая);
	// MarginCategories — веса маржи по категориям (только C4).
	RelationsVersion int                `json:"relations_version"`
	MarginCategories map[string]float64 `json:"margin_categories,omitempty"`
	Metrics          CommerceMetrics    `json:"metrics"`
	// Block — метрики блока на companion_pairs.json целиком (нарушения — список пар); nil — блок выключен.
	Block any `json:"block"`
	// Regressions — запросы, где основная выдача хуже предыдущей ступени лестницы (до 10).
	Regressions []EvalChange `json:"regressions"`
	DurationMs  int64        `json:"duration_ms"`
}

// CommerceMetrics — метрики конфигурации.
type CommerceMetrics struct {
	CompatViolations    *int     `json:"compat_violations"`
	RequiredRecall      *float64 `json:"required_recall"`
	Coverage            *float64 `json:"coverage"`
	BlockPrecisionJudge *float64 `json:"block_precision_judge"`
	MPNTop1             *float64 `json:"mpn_top1"`
	UnitsParseAcc       *float64 `json:"units_parse_acc"`
	ExactTop1           *float64 `json:"exact_top1"`
	NDCG10              *float64 `json:"ndcg10"`
	MarginIndex         *float64 `json:"margin_index"`
	Errors              int      `json:"errors"`
}
