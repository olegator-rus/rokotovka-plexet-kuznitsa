package out

import "time"

// Аналитика поиска по журналам (GET /api/analytics). Доли — 0..1; nil — знаменатель пуст.

// AnalyticsResponse — плитки, воронка и проблемные запросы за период.
type AnalyticsResponse struct {
	Role     string         `json:"role"` // ecom | product | content | head
	Days     int            `json:"days"`
	From     time.Time      `json:"from"`
	To       time.Time      `json:"to"`
	Tiles    AnalyticsTiles `json:"tiles"`
	Funnel   []FunnelStep   `json:"funnel"`
	Problems []ProblemQuery `json:"problems"`
	// Sources — откуда события: live (работающий поиск) и mock_log (импорт исторических логов).
	Sources map[string]int `json:"sources"`
	// Effect — вилка эффекта (как GET /api/effect с допущениями по умолчанию); только для role=head, иначе null.
	Effect *Effect `json:"effect"`
}

// AnalyticsTiles — сводные доли.
type AnalyticsTiles struct {
	Searches int `json:"searches"`
	// EmptyRate — доля поисков без результатов.
	Empty     int      `json:"empty"`
	EmptyRate *float64 `json:"empty_rate"`
	// NoClickRate — доля непустых поисков без клика.
	NoClick     int      `json:"no_click"`
	NoClickRate *float64 `json:"no_click_rate"`
	// OrderRate — доля поисков, после которых в той же сессии был заказ.
	Orders    int      `json:"orders"`
	OrderRate *float64 `json:"order_rate"`
	// PendingProposals — предложения агента, ждущие решения (из очереди proposals).
	PendingProposals int `json:"pending_proposals"`
}

// FunnelStep — шаг воронки поиск → клик → корзина → заказ.
type FunnelStep struct {
	Step  string   `json:"step"` // searches | clicks | carts | orders
	Label string   `json:"label"`
	Count int      `json:"count"`
	Rate  *float64 `json:"rate"` // доля от поисков
}

// ProblemQuery — нормализованный запрос с неудачами.
type ProblemQuery struct {
	Query        string `json:"query"`
	Freq         int    `json:"freq"`
	Empty        int    `json:"empty"`
	NoClick      int    `json:"no_click"`
	Reformulated int    `json:"reformulated"`
	Clicks       int    `json:"clicks"`
	Orders       int    `json:"orders"`
	Failures     int    `json:"failures"`
	// Outcome — что происходит чаще всего: empty | reformulated | no_click.
	Outcome      string `json:"outcome"`
	OutcomeLabel string `json:"outcome_label"`
	Diagnosis    string `json:"diagnosis"`
	// ProposalID, ProposalStatus — предложение агента, в кластер которого входит запрос (ждущее решения — в
	// приоритете, затем самое новое); null — агент запрос не разбирал.
	ProposalID     *string `json:"proposal_id"`
	ProposalStatus *string `json:"proposal_status"`
	// ProposalDiagnosis — диагноз агента (код ALIAS_GAP…); diagnosis выше остаётся подписью журнала.
	ProposalDiagnosis *string `json:"proposal_diagnosis"`
}

// LogImportResult — импорт исторических логов.
type LogImportResult struct {
	Searches int       `json:"searches"`
	Behavior int       `json:"behavior"`
	Skipped  int       `json:"skipped"`
	From     time.Time `json:"from"`
	To       time.Time `json:"to"`
	Ms       int64     `json:"ms"`
}
