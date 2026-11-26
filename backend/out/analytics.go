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
	// Commerce — блок «Допродажи» для роли (контур коммерции); nil — блок не собран.
	Commerce *AnalyticsCommerce `json:"commerce"`
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
	// Agent — вывод агента по запросу: из предложения или итога кластера последних запусков.
	Agent AgentVerdict `json:"agent"`
}

// AgentVerdict — вывод агента у проблемного запроса.
type AgentVerdict struct {
	// Code: proposal (есть предложение) | answers (действующий поиск уже отвечает) | closed (разобран, правкой
	// правил не решается) | no_offer (спрос без предложения в каталоге) | earlier (разобран в прежних запусках) |
	// queued (агент видел кластер, но до него не дошёл) | mostly_ok (агент не брал: запрос чаще заканчивается
	// кликом) | unseen (агент кластер не видел).
	Code  string `json:"code"`
	Label string `json:"label"`
	// Note — пояснение агента (итог кластера); Diagnosis — код диагноза, если был.
	Note      string `json:"note,omitempty"`
	Diagnosis string `json:"diagnosis,omitempty"`
	// RunID, ClusterID, ClusterLabel — где агент это решил (лента запуска).
	RunID        string `json:"run_id,omitempty"`
	ClusterID    string `json:"cluster_id,omitempty"`
	ClusterLabel string `json:"cluster_label,omitempty"`
}

// ProblemsNowResponse — GET /api/analytics/current: что отвечает действующий поиск на проблемные запросы
// сейчас (поиск без записи в журнал, действующие правила).
type ProblemsNowResponse struct {
	Days           int          `json:"days"`
	RulesVersion   int          `json:"rules_version"`
	CatalogVersion string       `json:"catalog_version"`
	CheckedAt      time.Time    `json:"checked_at"`
	Items          []ProblemNow `json:"items"`
}

// ProblemNow — текущая выдача по одному проблемному запросу.
type ProblemNow struct {
	Query string `json:"query"`
	Total int    `json:"total"`
	// Exact — первым стоит точный товар (группа exact); NotInCatalog — поиск сказал «нет в каталоге»
	// (показаны аналоги или ничего).
	Exact        bool   `json:"exact"`
	NotInCatalog bool   `json:"not_in_catalog"`
	TopTitle     string `json:"top_title"`
	Route        string `json:"route"`
	// Cached — ответ из кэша проверки (10 мин, та же версия правил и каталога).
	Cached bool   `json:"cached"`
	Error  string `json:"error,omitempty"`
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

// AnalyticsCommerce — блок «Допродажи» аналитики для роли (контур коммерции). Поведенческие числа —
// события стенда, не трафик магазина; маржа и заказы mock — синтетические.
type AnalyticsCommerce struct {
	Role      string `json:"role"`
	Title     string `json:"title"`
	Note      string `json:"note"`
	Synthetic bool   `json:"synthetic"`
	// Tiles — плитки роли; у каждой источник и ссылка на список (List — ключ из Lists, Link — экран).
	Tiles []CommerceTile `json:"tiles"`
	Lists []CommerceList `json:"lists"`
	// Effect — модель эффекта пакета (как GET /api/effect/commerce по умолчанию); только role=head.
	Effect  *EffectCommerce `json:"effect"`
	Missing []string        `json:"missing"`
}

// CommerceTile — плитка блока «Допродажи». Value — доля или число; nil — не посчитано (причина в Note).
type CommerceTile struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Value *float64 `json:"value"`
	Unit  string   `json:"unit"` // share | count | rub
	// Source — stand_events (события стенда) | offline_eval (прогон лестницы) | catalog (справочники) |
	// agent (очередь агента) | synthetic (синтетические данные).
	Source string `json:"source"`
	Note   string `json:"note"`
	List   string `json:"list,omitempty"`
	Link   string `json:"link,omitempty"`
}

// CommerceList — список за плиткой (до 50 строк, Total — всего).
type CommerceList struct {
	Key   string            `json:"key"`
	Label string            `json:"label"`
	Total int               `json:"total"`
	Items []CommerceListRow `json:"items"`
}

// CommerceListRow — строка списка.
type CommerceListRow struct {
	ID     string `json:"id"`
	Title  string `json:"title"`
	Detail string `json:"detail"`
	Query  string `json:"query,omitempty"`
}
