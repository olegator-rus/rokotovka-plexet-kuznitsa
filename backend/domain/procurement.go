package domain

// Контур доводки, закупка и честное «нет»: справка магазина, статус реестра, набор витрины, события.

// HelpArticle — статья справки (data/mock/help.json): триггеры — слова и фразы запроса; Mixed — вид закупки
// (renewal), при котором статья показывается строкой рядом с товаром.
type HelpArticle struct {
	ID        string   `json:"id"`
	Title     string   `json:"title"`
	Text      string   `json:"text"`
	LinkLabel string   `json:"link_label"`
	URL       string   `json:"url"`
	Triggers  []string `json:"triggers"`
	Mixed     string   `json:"mixed,omitempty"`
}

// ReestrEntry — статус семейства в реестре российского ПО (data/mock/reestr.json).
type ReestrEntry struct {
	FamilyID  string `json:"family_id"`
	Status    string `json:"status"` // confirmed | not_matching
	Number    string `json:"number"`
	CheckedAt string `json:"checked_at"`
	Source    string `json:"source"`
}

// Статусы реестра в ответе.
const (
	ReestrConfirmed   = "confirmed"
	ReestrNotMatching = "not_matching"
	ReestrUnknown     = "unknown"
)

// ShowcaseQuery — запрос прогона витрины, перенесённый на mock-каталог (data/mock/showcase.json).
type ShowcaseQuery struct {
	ID      string `json:"id"`
	Query   string `json:"query"`
	Vitrine string `json:"vitrine"` // ok | partial | fail | unknown — оценка витрины
	Expect  string `json:"expect"`  // product | analogs | absent | help
	Note    string `json:"note"`
}

// События контура доводки.
const (
	EventHelpAnswerShown     = "help_answer_shown"
	EventHelpClick           = "help_click"
	EventSupplyRequest       = "supply_request"
	EventAnalogOriginalShown = "analog_original_shown"
)

func init() {
	BehaviorEventTypes = append(BehaviorEventTypes, EventHelpAnswerShown, EventHelpClick, EventSupplyRequest,
		EventAnalogOriginalShown)
	EventRequiredFields[EventHelpAnswerShown] = []string{"help_id"}
	EventRequiredFields[EventHelpClick] = []string{"help_id"}
	EventRequiredFields[EventSupplyRequest] = []string{"query"}
	EventRequiredFields[EventAnalogOriginalShown] = []string{"family_id"}
}
