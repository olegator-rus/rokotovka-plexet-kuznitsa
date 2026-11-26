package domain

import (
	"context"
	"time"
)

// Разбор намерения на маршруте D.
// Модель возвращает структуру, код сверяет каждое значение со справочниками; запрос к движку строит
// поиск по этой структуре, а не модель.

// Виды намерения (Intent.Kind).
const (
	IntentFindProduct     = "find_product"
	IntentFindAlternative = "find_alternative"
	IntentRenewal         = "renewal"
	IntentNonProduct      = "non_product"
)

// IntentKinds — допустимые виды намерения.
var IntentKinds = []string{IntentFindProduct, IntentFindAlternative, IntentRenewal, IntentNonProduct}

// Поля условий (AttrConstraint.Field).
const (
	FieldLicenseQuantity = "license_quantity"
	FieldLicenseMetric   = "license_metric"
	FieldLicenseType     = "license_type"
	FieldLicenseDuration = "license_duration_months"
	FieldOrigin          = "origin"
	FieldRenewal         = "renewal"
)

// Intent — разобранное намерение запроса.
type Intent struct {
	Kind            string           `json:"kind"`             // find_product | find_alternative | renewal | non_product
	ReferenceFamily string           `json:"reference_family"` // id семейства-референса после сверки, "" если нет
	CategoryID      string           `json:"category_id"`
	VendorID        string           `json:"vendor_id"`
	Origin          string           `json:"origin"`      // "ru" | ""
	Constraints     []AttrConstraint `json:"constraints"` // обязательные
	Preferences     []AttrConstraint `json:"preferences"` // мягкие
	Unresolved      []string         `json:"unresolved"`
	Dropped         []DroppedField   `json:"dropped"` // отброшено сверкой со справочниками
	Source          string           `json:"source"`  // rules | cache | llm
	LLM             *LLMCallInfo     `json:"llm"`     // nil, если модель не вызывалась
	// FallbackReason — почему разбор остался на правилах (llm_off | llm_not_configured | llm_error |
	// schema_error | catalog_not_loaded); пусто — разобран моделью или взят из кэша.
	FallbackReason string `json:"fallback_reason,omitempty"`
	// PIIMasked — сколько фрагментов с персональными данными (телефон, почта, ИНН…) замаскировано в тексте запроса
	// до ключа кэша и вызова модели; 0 — не было.
	PIIMasked int `json:"pii_masked,omitempty"`
}

// AttrConstraint — условие на атрибут. Field: license_quantity | license_metric | license_type |
// license_duration_months | origin | renewal.
type AttrConstraint struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// DroppedField — значение модели, отброшенное сверкой, и причина.
type DroppedField struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Reason string `json:"reason"`
}

// LLMCallInfo — расход на разбор одного запроса (сумма по попыткам, если был повтор).
type LLMCallInfo struct {
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CachedTokens int     `json:"cached_tokens"`
	Ms           float64 `json:"ms"`
	CostRub      float64 `json:"cost_rub"`
	// Calls — сколько вызовов модели ушло на разбор (повтор при ошибке схемы, резервная модель).
	Calls int `json:"calls"`
}

// IntentParser — разбор сложного запроса. rulesGuess — то, что уже извлекли правила; парсер дополняет и
// уточняет; при любой ошибке возвращает rulesGuess с Source="rules" (ошибку — для журнала, поиск не падает).
// request_id для учёта llm_usage передаётся через контекст (services.WithRequestID).
type IntentParser interface {
	Parse(ctx context.Context, normalized string, rulesGuess Intent, clientSegment string) (Intent, error)
}

// IntentStep — шаг intent трассы: что разобрано, что отброшено, откуда разбор.
type IntentStep struct {
	StepBase
	Source  string         `json:"source,omitempty"` // rules | cache | llm
	Fields  map[string]any `json:"fields"`
	Dropped []DroppedField `json:"dropped"`
	// PIIMasked — «замаскировано: N»: фрагменты с ПДн, заменённые метками до кэша и модели (omitempty — не было).
	PIIMasked int `json:"pii_masked,omitempty"`
}

// TraceLLM — trace.llm: вызов модели в трассе поиска.
type TraceLLM struct {
	Model        string  `json:"model"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CachedTokens int     `json:"cached_tokens"`
	Ms           float64 `json:"ms"`
	CostRub      float64 `json:"cost_rub"`
	Calls        int     `json:"calls"`
}

// IntentCache — кэш разборов (intent_cache). Ключ — хэш нормализованного запроса, влияющего контекста и
// версий каталога, правил, промпта и модели: смена любой версии — промах, старые строки просто не читаются.
type IntentCache struct {
	Key            string `gorm:"primaryKey;size:64"`
	Normalized     string
	Segment        string `gorm:"size:16"`
	CatalogVersion string `gorm:"size:64"`
	RulesVersion   int
	PromptVersion  string `gorm:"size:64"`
	Model          string `gorm:"size:128"`
	Intent         JSONB  `gorm:"type:jsonb;not null"`
	Hits           int
	CreatedAt      time.Time
	LastHitAt      *time.Time
}

// TableName — intent_cache, как в ARCHITECTURE §4.
func (IntentCache) TableName() string { return "intent_cache" }
