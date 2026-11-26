package out

import "time"

// MarginGuards — защиты рычага маржи (нередактируемые).
type MarginGuards struct {
	ExactQuery    bool `json:"exact_query"`
	ExactTop1     bool `json:"exact_top1"`
	GroupLock     bool `json:"group_lock"`
	Compatibility bool `json:"compatibility"`
	MaxShift      int  `json:"max_shift"`
}

// MarginVersion — версия политики маржи (config_versions kind=margin); версия 0 — исходная, вес контура поиска.
type MarginVersion struct {
	Version   int        `json:"version"`
	Weight    float64    `json:"weight"`
	Source    string     `json:"source"` // default | human | rollback | agent
	Author    string     `json:"author"`
	Comment   string     `json:"comment"`
	CreatedAt *time.Time `json:"created_at"`
}

// MarginPolicy — GET /api/margin/policy.
type MarginPolicy struct {
	Version    int          `json:"version"`
	Weight     float64      `json:"weight"`
	BaseWeight float64      `json:"base_weight"`
	Positions  []float64    `json:"positions"`
	Guards     MarginGuards `json:"guards"`
	Epsilon    float64      `json:"epsilon"`
	MaxShift   int          `json:"max_shift"`
	// StopWeight — упор рычага по последней симуляции на tune этого каталога; nil — симуляции ещё не было.
	StopWeight *float64 `json:"stop_weight"`
	Synthetic  bool     `json:"synthetic"`
	// Enabled — флаг FEATURE_MARGIN_POLICY: выключен — политика в поиске не применяется.
	Enabled  bool            `json:"enabled"`
	Versions []MarginVersion `json:"versions"`
}

// MarginApplied — POST /api/margin/policy и /rollback.
type MarginApplied struct {
	Version       int     `json:"version"`
	Weight        float64 `json:"weight"`
	VersionBefore int     `json:"version_before"`
	WeightBefore  float64 `json:"weight_before"`
	RolledBackTo  *int    `json:"rolled_back_to,omitempty"`
}

// MarginItemRef — позиция выдачи в «было / стало».
type MarginItemRef struct {
	ProductID string `json:"product_id"`
	Title     string `json:"title"`
	Grade     *int   `json:"grade"` // оценка разметки; nil — не размечена
	// Margin — нормированная маржа (перцентиль в категории, 0–1; нет маржи — 0.5).
	Margin float64 `json:"margin"`
	// Moved — сдвиг политикой: +1 — поднялась на позицию, −2 — опустилась на две; 0 — на месте.
	Moved int `json:"moved"`
}

// MarginAffected — запрос, у которого изменилась выдача.
type MarginAffected struct {
	QueryID     string          `json:"query_id"`
	Query       string          `json:"query"`
	Type        string          `json:"type"`
	NDCGBefore  *float64        `json:"ndcg_before"`
	NDCGAfter   *float64        `json:"ndcg_after"`
	NDCGDelta   float64         `json:"ndcg_delta"`
	Top1Changed bool            `json:"top1_changed"`
	Top3Changed bool            `json:"top3_changed"`
	Guards      []string        `json:"guards"`
	Before      []MarginItemRef `json:"before"`
	After       []MarginItemRef `json:"after"`
}

// MarginPosition — показатели одного положения рычага.
type MarginPosition struct {
	Weight float64 `json:"weight"`
	// NDCG10 — среднее NDCG@10 после политики (с бюджетом ε); NDCGDeltaMean — среднее изменение к контуру поиска.
	NDCG10        float64 `json:"ndcg10"`
	NDCGDeltaMean float64 `json:"ndcg_delta_mean"`
	// NDCGDeltaP10 — худший дециль изменения NDCG@10 (10-й перцентиль).
	NDCGDeltaP10 float64 `json:"ndcg_delta_p10"`
	// ShareOverEps — доля размеченных запросов, где политика без бюджета ε (как в живом поиске) теряет больше ε.
	ShareOverEps float64 `json:"share_over_eps"`
	// MarginIndex — индекс показанной маржи: нормированная маржа топ-10 с весом позиции 1/log2(i+1), среднее по
	// запросам; MarginIndexRelevant — то же только по релевантным позициям (оценка ≥ 2).
	MarginIndex         float64 `json:"margin_index"`
	MarginIndexRelevant float64 `json:"margin_index_relevant"`
	Top1Changed         int     `json:"top1_changed"`
	Top3Changed         int     `json:"top3_changed"`
	// ExactTop1Changed — точные запросы (exact, sku), у которых сменился первый результат; цель — 0.
	ExactTop1Changed int `json:"exact_top1_changed"`
	// GuardsFired — сколько раз защиты не дали переставить (по видам и всего).
	GuardsFired       int              `json:"guards_fired"`
	GuardsByKind      map[string]int   `json:"guards_by_kind"`
	EpsilonRollbacks  int              `json:"epsilon_rollbacks"`
	Available         bool             `json:"available"`
	Affected          []MarginAffected `json:"affected"`
	QueriesWithShifts int              `json:"queries_with_shifts"`
}

// MarginSimulation — POST /api/margin/simulate.
type MarginSimulation struct {
	Set     string  `json:"set"`
	SetHash string  `json:"set_hash"`
	Queries int     `json:"queries"`
	Judged  int     `json:"judged"`
	Epsilon float64 `json:"epsilon"`
	// BaseNDCG10 — NDCG@10 контура поиска (вес 0.05) на этом наборе.
	BaseNDCG10    float64          `json:"base_ndcg10"`
	BaseWeight    float64          `json:"base_weight"`
	CurrentWeight float64          `json:"current_weight"`
	PolicyVersion int              `json:"policy_version"`
	Items         []MarginPosition `json:"items"`
	// StopWeight — упор рычага: последнее положение вверх от 0.05, где share_over_eps = 0.
	StopWeight float64 `json:"stop_weight"`
	// MinWeight — то же вниз от 0.05.
	MinWeight      float64 `json:"min_weight"`
	CatalogVersion string  `json:"catalog_version"`
	RulesVersion   int     `json:"rules_version"`
	Synthetic      bool    `json:"synthetic"`
	Cached         bool    `json:"cached"`
	DurationMs     int64   `json:"duration_ms"`
	Note           string  `json:"note"`
}

// MarginSimStatus — GET /api/margin/simulate/status: прогон базовой выдачи идёт (прогресс).
type MarginSimStatus struct {
	Running bool `json:"running"`
	Done    int  `json:"done"`
	Total   int  `json:"total"`
}
