package domain

import "time"

// Контур коммерции: допродажи, оборудование, маржа. Справочники приходят из
// источника каталога (JSON-теги — формат файлов data/mock), импорт кладёт их в PostgreSQL и в память.

// Виды позиции (Product.Kind).
const (
	KindSoftware = "software"
	KindHardware = "hardware"
)

// Типы цены (Product.PriceMode).
const (
	PriceFixed            = "fixed"
	PriceFrom             = "from"
	PriceQuoteRequired    = "quote_required"
	PriceCustomerSpecific = "customer_specific"
	PriceUnavailable      = "unavailable"
)

// PriceModes — допустимые типы цены.
var PriceModes = []string{PriceFixed, PriceFrom, PriceQuoteRequired, PriceCustomerSpecific, PriceUnavailable}

// Обязательность связи (Relation.Requirement, LicenseRule.Class).
const (
	RequirementConditional = "conditional_required"
	RequirementRecommended = "recommended"
	RequirementOptional    = "optional"
)

// Состояния совместимости (Relation.Compatibility, CompatibilityRule.State). Пара без правила — unknown:
// «совместимость не подтверждена», не равно «совместимо».
const (
	CompatVerified     = "verified"
	CompatConditional  = "conditional"
	CompatIncompatible = "incompatible"
	CompatUnknown      = "unknown"
)

// Источники связи и правила (Relation.Source).
const (
	SourceVendorDocs    = "vendor_documentation"
	SourceCatalogRule   = "catalog_rule"
	SourceExpert        = "expert"
	SourceCoPurchase    = "co_purchase"
	SourceAgentProposal = "agent_proposal"
)

// Статусы проверки связи (Relation.ReviewStatus).
const (
	ReviewApproved = "approved"
	ReviewPending  = "pending"
	ReviewRejected = "rejected"
)

// LicenseRule — правило вендора: что требуется якорю (license_rules.json).
type LicenseRule struct {
	ID           string `gorm:"primaryKey" json:"id"`
	VendorID     string `gorm:"index" json:"vendor_id"`
	AnchorFamily string `gorm:"index" json:"anchor_family"`
	// AnchorAttrs — уточнение якоря по атрибутам ({"edition": "Standard"}); пусто — любая позиция семейства.
	AnchorAttrs map[string]string `gorm:"serializer:json;type:jsonb" json:"anchor_attrs,omitempty"`
	// AnchorProduct — правило только для этой позиции семейства (SQL Server Standard «Server + CAL»); "" — любая.
	AnchorProduct    string `json:"anchor_product,omitempty"`
	RequiresCategory string `json:"requires_category"`
	// RequiresFamily — требуемое семейство (точнее категории: клиентские лицензии 1С); "" — по категории.
	RequiresFamily string `json:"requires_family,omitempty"`
	// RequiresAttrs — условия на требуемое ({"version": ">=anchor.version"}).
	RequiresAttrs    map[string]string `gorm:"serializer:json;type:jsonb" json:"requires_attrs,omitempty"`
	Class            string            `gorm:"size:24" json:"class"`
	Conditions       []string          `gorm:"serializer:json;type:jsonb" json:"conditions"`
	QuantityStrategy string            `gorm:"size:16" json:"quantity_strategy"`
	Reason           string            `json:"reason"`
	Source           string            `gorm:"size:24" json:"source"`
	SourceURL        string            `json:"source_url"`
}

// CompatibilityRule — проверяемое условие совместимости позиции или семейства с моделью системы.
type CompatibilityRule struct {
	ID          string `gorm:"primaryKey" json:"id"`
	ProductID   string `gorm:"index" json:"product_id"` // либо позиция, либо семейство
	FamilyID    string `gorm:"index" json:"family_id"`
	TargetModel string `gorm:"index" json:"target_model"`
	State       string `gorm:"size:16" json:"state"` // verified | conditional | incompatible
	Condition   string `json:"condition"`
	Source      string `gorm:"size:24" json:"source"`
	SourceURL   string `json:"source_url"`
	RuleVersion int    `json:"rule_version"`
	Date        string `json:"date"`
}

// AttributeDefinition — числовые и перечислимые атрибуты категории: единицы, синонимы единиц, фасеты.
type AttributeDefinition struct {
	CategoryID string          `gorm:"primaryKey" json:"category_id"`
	Attrs      []AttributeSpec `gorm:"serializer:json;type:jsonb" json:"attrs"`
}

// AttributeSpec — один атрибут категории.
type AttributeSpec struct {
	Key         string   `json:"key"` // ключ в Product.Specs: ram_gb
	Label       string   `json:"label"`
	Unit        string   `json:"unit"`
	Type        string   `json:"type"` // number | enum | bool
	Facet       bool     `json:"facet"`
	UnitAliases []string `json:"unit_aliases"`
	// UnitConvert — синоним единицы с множителем к базовой («тб» → 1000 для ГБ); Values — значения enum. Трек
	// оборудования, необязательны.
	UnitConvert map[string]float64 `json:"unit_convert,omitempty"`
	Values      []string           `json:"values,omitempty"`
}

// Статусы запроса КП (QuoteRequest.Status).
const QuoteNew = "new"

// QuoteRequest — спецификация и запрос КП (POST /api/quotes).
type QuoteRequest struct {
	UUIDModel
	SessionID *string     `gorm:"index"`
	ClientID  *string     `gorm:"index"`
	Items     []QuoteItem `gorm:"serializer:json;type:jsonb"`
	Comment   string
	Status    string `gorm:"size:16;index"`
	Positions int
	// TotalRub — сумма, только если известны все цены; nil — есть позиции по запросу.
	TotalRub  *float64 `gorm:"type:numeric(14,2)"`
	OnRequest int
}

// QuoteItem — позиция спецификации.
type QuoteItem struct {
	ProductID      string `json:"product_id"`
	Quantity       int    `json:"quantity"`
	Origin         string `json:"origin"` // search | companion | guide | tz; template — только в заявках до снятия шаблонов
	CompanionClass string `json:"companion_class,omitempty"`
	// Purchase — исходный запрос и параметры закупки (контур доводки); nil — позиция не из поиска.
	Purchase *QuotePurchase `json:"purchase,omitempty"`
}

// QuotePurchase — исходный запрос и извлечённые параметры закупки позиции спецификации.
type QuotePurchase struct {
	Query          string `json:"query"`
	Deal           string `json:"deal,omitempty"`
	Quantity       *int   `json:"quantity,omitempty"`
	Metric         string `json:"metric,omitempty"`
	DurationMonths *int   `json:"duration_months,omitempty"`
	OrgType        string `json:"org_type,omitempty"`
	Edition        string `json:"edition,omitempty"`
}

// CatalogInfo — сведения о снимке каталога, которых нет у позиций: что в данных синтетическое. Одна строка на
// версию каталога; API без файлов данных берёт отсюда плашку «Синтетические данные».
type CatalogInfo struct {
	Version    string   `gorm:"primaryKey"`
	Synthetic  []string `gorm:"serializer:json;type:jsonb"`
	ImportedAt time.Time
}

// Виды версий config_versions (ConfigVersion.Kind). Цепочки независимы: у каждого вида своя нумерация.
const (
	ConfigKindRules     = "rules"
	ConfigKindRelations = "relations"
	ConfigKindMargin    = "margin"
)
