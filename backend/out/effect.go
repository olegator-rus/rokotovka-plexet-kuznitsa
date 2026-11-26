package out

import "time"

// Effect — GET /api/effect (и поле effect в GET /api/analytics?role=head): вилка эффекта внедрения для
// руководителя. Три сценария в заказах и рублях в месяц из измеренного (журналы поиска,
// прогон оценки B0 vs B4 на test) и допущений с явными подписями. Непосчитанное — null.
type Effect struct {
	Days int       `json:"days"`
	From time.Time `json:"from"`
	To   time.Time `json:"to"`
	// Note — постоянная подпись под блоком.
	Note string `json:"note"`
	// Formula — формула по-русски; FormulaKeys — та же формула ключами measured/assumptions.
	Formula     string        `json:"formula"`
	FormulaKeys string        `json:"formula_keys"`
	Measured    EffectInputs  `json:"measured"`
	Assumptions []EffectParam `json:"assumptions"`
	// Scenarios — conservative | base | optimistic; пусто, если не хватает измерений (нет поисков или прогона).
	Scenarios []EffectScenario `json:"scenarios"`
	// Missing — чего не хватает для расчёта (по-русски); пусто — всё посчитано.
	Missing []string `json:"missing"`
}

// EffectInputs — измеренное: журналы поиска за период и прогон оценки.
type EffectInputs struct {
	Searches int `json:"searches"`
	// SearchesMonth — поиски, приведённые к 30 дням.
	SearchesMonth float64 `json:"searches_month"`
	// ProblemSearches — пустая выдача или выдача без клика.
	ProblemSearches int      `json:"problem_searches"`
	ProblemShare    *float64 `json:"problem_share"`
	// ProblemSessions — сессии, в которых был хотя бы один проблемный поиск.
	ProblemSessions int `json:"problem_sessions"`
	Orders          int `json:"orders"`
	// SearchOrderRate — конверсия поиска в заказ (заказы / поиски).
	SearchOrderRate *float64 `json:"search_order_rate"`
	// Rescued — доля спасённых: среди запросов test, где B0 не дал релевантного в топ-10, — доля, где его дал B4.
	Rescued *EffectRescued `json:"rescued"`
}

// EffectRescued — доля спасённых запросов по прогону оценки.
type EffectRescued struct {
	Set      string `json:"set"`
	Baseline string `json:"baseline"` // B0
	Config   string `json:"config"`   // B4 (или B3, если B4 не прогонялся)
	// RunBaseline, RunConfig — прогоны (evaluations.run_id), из которых взяты результаты.
	RunBaseline  string  `json:"run_baseline"`
	RunConfig    string  `json:"run_config"`
	RulesVersion int     `json:"rules_version"`
	Failed       int     `json:"failed"`  // запросов, где B0 не дал релевантного в топ-10
	Rescued      int     `json:"rescued"` // из них B4 дал
	Share        float64 `json:"share"`
	// CILow, CIHigh — 95% интервал Уилсона: из него берутся консервативный и оптимистичный сценарии.
	CILow  float64 `json:"ci_low"`
	CIHigh float64 `json:"ci_high"`
}

// EffectParam — допущение: значение, откуда оно и подпись.
type EffectParam struct {
	Key   string  `json:"key"`   // session_factor | order_prob | avg_check_rub
	Label string  `json:"label"` // подпись поля
	Value float64 `json:"value"`
	// Default — значение по умолчанию (измеренное или принятое), Source — откуда значение: measured (из
	// журналов), assumed (принято без замера), request (задано в запросе).
	Default float64 `json:"default"`
	Source  string  `json:"source"`
	Unit    string  `json:"unit"`
	Hint    string  `json:"hint"`
}

// EffectScenario — один сценарий вилки.
type EffectScenario struct {
	ID    string `json:"id"` // conservative | base | optimistic
	Label string `json:"label"`
	// Как сценарий отличается от базового.
	Rule string `json:"rule"`
	// RescuedShare, OrderProb — значения, подставленные в формулу.
	RescuedShare float64 `json:"rescued_share"`
	OrderProb    float64 `json:"order_prob"`
	// ProblemSessionsMonth — проблемные поиски в месяц с поправкой на сессии.
	ProblemSessionsMonth float64 `json:"problem_sessions_month"`
	RescuedMonth         float64 `json:"rescued_month"`
	OrdersMonth          float64 `json:"orders_month"`
	RevenueRubMonth      float64 `json:"revenue_rub_month"`
}
