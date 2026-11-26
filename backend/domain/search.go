package domain

// Журналы поиска и поведения. Пишутся сервисами поиска и событий;
// читают их аналитика и агент.

// Конфигурации поиска — лестница baseline.
const (
	ConfigB0 = "B0" // точное совпадение подстроки в названии
	ConfigB1 = "B1" // BM25 по мультиполям + нормализация + словари
	ConfigB2 = "B2" // B1 + векторы + RRF
	ConfigB3 = "B3" // B2 + атрибуты, намерение, ранжирование с порогом и бизнес-слоем
	ConfigB4 = "B4" // B3 + принятые правки агента
)

// SearchConfigs — все конфигурации по порядку лестницы.
var SearchConfigs = []string{ConfigB0, ConfigB1, ConfigB2, ConfigB3, ConfigB4}

// ConfigLive — витрина: пайплайн B3 на действующей (последней принятой) версии правил. В лестницу оценки не
// входит: по смыслу это B4 «сейчас», а B4 в оценке — замер конкретной версии.
const ConfigLive = "live"

// DefaultSearchConfig — конфигурация по умолчанию: витрина работает на действующих правилах.
const DefaultSearchConfig = ConfigLive

// SearchEvent — один поиск: что искали, как обработали, сколько нашли.
type SearchEvent struct {
	UUIDModel
	RequestID      string `gorm:"uniqueIndex;size:32"`
	Query          string
	Normalized     string
	Route          string `gorm:"size:1"`
	ParseSource    string `gorm:"size:8"`
	Config         string `gorm:"size:4;index"`
	ResultsCount   int    // число позиций выдачи (семейств) до обрезки по limit
	RulesVersion   int
	CatalogVersion string
	ClientID       *string `gorm:"index"`
	SessionID      *string `gorm:"index"`
	TotalMs        int
	// Source — live (поиск) | mock_log (импорт исторических логов).
	Source string `gorm:"size:16;index;default:live"`
}

// SearchTrace — трасса поиска целиком.
type SearchTrace struct {
	UUIDModel
	RequestID string `gorm:"uniqueIndex;size:32"`
	Trace     JSONB  `gorm:"type:jsonb;not null"`
}

// Типы событий поведения (POST /api/events). Событие search пишет сам поиск.
const (
	EventClick          = "click"
	EventAddToCart      = "add_to_cart"
	EventOrder          = "order"
	EventPriceRequest   = "price_request"
	EventManagerRequest = "manager_request"
	EventSuggestPick    = "suggest_pick"
	EventCorrectionUndo = "correction_undo"
	EventChipRemove     = "chip_remove"
	EventClarifyAnswer  = "clarify_answer"
)

// BehaviorEventTypes — допустимые типы событий поведения.
var BehaviorEventTypes = []string{
	EventClick, EventAddToCart, EventOrder, EventPriceRequest, EventManagerRequest,
	EventSuggestPick, EventCorrectionUndo, EventChipRemove, EventClarifyAnswer,
}

// BehaviorEvent — действие покупателя после поиска.
type BehaviorEvent struct {
	UUIDModel
	Type      string  `gorm:"size:32;index"`
	RequestID *string `gorm:"index;size:32"`
	SessionID *string `gorm:"index"`
	ClientID  *string `gorm:"index"`
	ProductID *string `gorm:"index"`
	Position  *int
	Payload   JSONB `gorm:"type:jsonb"`
	// Source — live (POST /api/events) | mock_log (импорт исторических логов).
	Source string `gorm:"size:16;index;default:live"`
}

// LLMUsage — учёт одного вызова модели (ARCHITECTURE §4).
type LLMUsage struct {
	UUIDModel
	RequestID      *string `gorm:"index;size:32"`
	AgentRunID     *string `gorm:"index"`
	Provider       string
	Model          string
	InputTokens    int
	OutputTokens   int
	CachedTokens   int
	LatencyMs      int
	PricingVersion string
	CalculatedCost float64 `gorm:"type:numeric(12,4)"` // рубли
	Status         string  `gorm:"size:16"`            // ok | error | timeout
}

// TableName — llm_usage в единственном числе, как в ARCHITECTURE §4.
func (LLMUsage) TableName() string { return "llm_usage" }

// ConfigVersion — версия словарей и правил поиска. Версия 1 — исходные словари из каталога; следующие —
// принятые правки агента.
type ConfigVersion struct {
	UUIDModel
	Version int    `gorm:"uniqueIndex"`
	Source  string // import | agent | rollback
	Comment string
	Rules   JSONB `gorm:"type:jsonb;not null"`
	// Author — system | agent+human | human; ProposalID — предложение, из которого версия; Diff — разница с
	// предыдущей версией {"add":[…],"remove":[…]}; Effect — эффект после следующего запуска агента.
	Author     string
	ProposalID *string
	Diff       JSONB `gorm:"type:jsonb"`
	Effect     JSONB `gorm:"type:jsonb"`
	// CatalogVersion — каталог, для которого собраны правила. Импорт с другим catalog_version начинает цепочку
	// версий заново: прежние помечаются Archived (не удаляются), поиск и агент берут только версии текущего каталога.
	CatalogVersion string `gorm:"index"`
	Archived       bool   `gorm:"not null;default:false"`
}
