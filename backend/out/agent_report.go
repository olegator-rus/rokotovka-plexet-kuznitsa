package out

import "time"

// SearchOpsReport — GET /api/agent/report: первый экран SearchOps, отчёт для маркетолога (контур фида).
// Неудачные запросы текущего каталога по четырём разделам; строка раскрывается в ленту шагов агента (run_id и
// cluster_id в agent). Потерянная выручка в рублях не считается; дат в отчёте нет.
type SearchOpsReport struct {
	CatalogVersion string          `json:"catalog_version"`
	RulesVersion   int             `json:"rules_version"`
	Days           int             `json:"days"`
	Stream         ReportStream    `json:"stream"`
	Totals         ReportTotals    `json:"totals"`
	Sections       []ReportSection `json:"sections"`
	// Pending — неудачные запросы без доказанной причины: агент их ещё не разбирал, а по ответу поиска и журналу
	// раздел не определяется. Отдельно от разделов, с кнопкой «Разобрать».
	Pending   []ReportRow `json:"pending"`
	CheckedAt time.Time   `json:"checked_at"`
	// Truncated — неудачных запросов больше, чем проверено действующим поиском (ReportProbeLimit).
	Truncated bool `json:"truncated,omitempty"`
}

// ReportStream — откуда поиски журнала: слои тестового потока и живые поиски; частоты — модельные.
type ReportStream struct {
	Label     string        `json:"label"`
	FreqLabel string        `json:"freq_label"`
	Layers    []ReportLayer `json:"layers"`
}

// ReportLayer — слой потока: запросов в потоке, различных запросов в журнале и поисков.
type ReportLayer struct {
	Code     string `json:"code"`
	Label    string `json:"label"`
	Queries  int    `json:"queries"`
	Searches int    `json:"searches"`
}

// ReportTotals — итог журнала текущего каталога за период.
type ReportTotals struct {
	Searches       int `json:"searches"`
	FailedSearches int `json:"failed_searches"`
	Queries        int `json:"queries"`
	FailedQueries  int `json:"failed_queries"`
}

// ReportSection — раздел отчёта.
type ReportSection struct {
	Code     string      `json:"code"`
	Title    string      `json:"title"`
	Hint     string      `json:"hint"`
	Rows     []ReportRow `json:"rows"`
	Searches int         `json:"searches"`
	Failures int         `json:"failures"`
}

// ReportRow — строка раздела: запрос (или слово — в разделе «слова клиента»).
type ReportRow struct {
	Query        string `json:"query"`
	Freq         int    `json:"freq"`
	Failures     int    `json:"failures"`
	Empty        int    `json:"empty"`
	NoClick      int    `json:"no_click"`
	Reformulated int    `json:"reformulated"`
	// Layers — происхождение: customer | site | synthetic (тестовый поток) | live (живой поиск).
	Layers []string `json:"layers"`
	Origin string   `json:"origin,omitempty"`
	// Kind — почему строка в разделе (подпричина), KindLabel — словами; Evidence — доказательство одной строкой.
	Kind      string `json:"kind"`
	KindLabel string `json:"kind_label"`
	Evidence  string `json:"evidence"`
	// Found — что сейчас первым в выдаче действующего поиска (без записи в журнал).
	Found *ReportFound `json:"found,omitempty"`
	// Words — непонятные поиску слова запроса; Examples — у слова: запросы, в которых оно встретилось.
	Words    []string `json:"words,omitempty"`
	Examples []string `json:"examples,omitempty"`
	// Agent — вывод агента и где он принят (run_id, cluster_id — раскрытие в ленту шагов); nil — агент не разбирал.
	Agent *AgentVerdict `json:"agent,omitempty"`
	// Proposal — предложение или задача агента по этому запросу.
	Proposal *ReportProposal `json:"proposal,omitempty"`
}

// ReportFound — первая карточка действующего поиска.
type ReportFound struct {
	FamilyID   string `json:"family_id"`
	Title      string `json:"title"`
	MatchType  string `json:"match_type,omitempty"`
	PriceState string `json:"price_state,omitempty"`
	Target     string `json:"target,omitempty"`
}

// ReportProposal — предложение агента по строке.
type ReportProposal struct {
	ID        string `json:"id"`
	Status    string `json:"status"`
	Action    string `json:"action"`
	Diagnosis string `json:"diagnosis"`
	RunID     string `json:"run_id"`
	ClusterID string `json:"cluster_id,omitempty"`
}
