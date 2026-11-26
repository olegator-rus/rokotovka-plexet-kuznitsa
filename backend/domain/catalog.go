package domain

import "time"

// Каталог и справочники. JSON-теги — формат источника данных:
// источник разбирается прямо в эти типы, импорт кладёт их в PostgreSQL и в индекс поиска.
// Ключи — идентификаторы источника (строки), а не UUID: по ним связаны файлы, логи и разметка.

// Типы лицензий (License.Type).
const (
	LicenseSubscription = "subscription"
	LicensePerpetual    = "perpetual"
	LicenseRenewal      = "renewal"
	LicenseUpgrade      = "upgrade"
	LicenseOEM          = "oem"
	LicenseCloud        = "cloud"
)

// Метрики лицензии (License.Metric).
const (
	MetricDevice = "device"
	MetricUser   = "user"
	MetricServer = "server"
	MetricCore   = "core"
	MetricOrg    = "org"
)

// Product — позиция каталога (уровень SKU: одна лицензия одного семейства).
type Product struct {
	ID          string `gorm:"primaryKey" json:"id"`
	SKU         string `gorm:"index" json:"sku"`
	FamilyID    string `gorm:"index" json:"family_id"`
	FamilyName  string `json:"family_name"`
	Title       string `json:"title"`
	VendorID    string `gorm:"index" json:"vendor_id"`
	CategoryID  string `gorm:"index" json:"category_id"`
	Description string `json:"description"`
	// Attributes — свободные атрибуты {"platform": "Windows, macOS"}; значения могут быть не строками.
	Attributes     JSONB    `gorm:"type:jsonb" json:"attributes"`
	License        License  `gorm:"embedded;embeddedPrefix:license_" json:"license"`
	PriceRub       *float64 `json:"price_rub"` // nil — цена по запросу
	PriceOnRequest bool     `json:"price_on_request"`
	Origin         string   `json:"origin"` // ru | foreign
	InReestr       bool     `json:"in_reestr"`
	Popularity     float64  `json:"popularity"` // 0..1
	Margin         float64  `json:"margin"`     // 0..1
	// Контур коммерции: оборудование. Нет поля в источнике — software, пустой mpn, price_mode из
	// price_on_request (catalogsource заполняет значения по умолчанию).
	Kind string `gorm:"size:16;index;not null;default:software" json:"kind"` // software | hardware
	// MPN — парт-номер производителя; "" у ПО.
	MPN string `gorm:"index" json:"mpn"`
	// Specs — числовые характеристики (ram_gb, storage_gb, …): ключи из attribute_definitions, значения — числа.
	Specs map[string]float64 `gorm:"serializer:json;type:jsonb" json:"specs,omitempty"`
	// CompatibleWith — канонические id моделей систем, с которыми совместим компонент.
	CompatibleWith []string `gorm:"serializer:json;type:jsonb" json:"compatible_with,omitempty"`
	// ModelID — каноническая модель самой системы (сервер, ноутбук); "" у компонентов и ПО.
	ModelID string `gorm:"index" json:"model_id"`
	// PriceMode — fixed | from | quote_required | customer_specific | unavailable.
	PriceMode string `gorm:"size:24" json:"price_mode"`
	// Контур фида: поля фида заказчика. В mock их нет — пустые значения, Available nil.
	// OfferURL — страница позиции на сайте магазина; FamilyURL — страница семейства (собирается с сайта отдельно).
	OfferURL  string `json:"offer_url"`
	FamilyURL string `json:"family_url"`
	// Available — в наличии по данным источника; nil — источник наличие не сообщает (mock: решает PriceOnRequest).
	Available *bool `json:"available"`
	// OldPriceRub — прежняя цена (> 0) как в источнике; скидка — только при OldPriceRub > PriceRub > 0.
	OldPriceRub *float64 `json:"old_price_rub"`
	// FeedPriceRub — цена из источника (> 0), которую нельзя показывать как действующую: позиция не в наличии
	//. У позиции с действующей ценой — nil, цена в PriceRub.
	FeedPriceRub *float64 `json:"feed_price_rub"`
	// CategoryPathIDs — id категорий от корня до категории позиции (CategoryID — последний).
	CategoryPathIDs []string `gorm:"serializer:json;type:jsonb" json:"category_path_ids,omitempty"`
	// CatalogVersion — версия снимка каталога, из которого позиция импортирована.
	CatalogVersion string    `json:"-"`
	ImportedAt     time.Time `json:"-"`
}

// License — условия лицензии позиции.
type License struct {
	Type           string  `json:"type"`
	Metric         *string `json:"metric"`          // nil — без метрики
	Quantity       *int    `json:"quantity"`        // nil — без количества
	DurationMonths *int    `json:"duration_months"` // nil — бессрочная
	Edition        string  `json:"edition"`
	// Контур фида (фид заказчика), в mock пусто.
	// Kind — вид позиции по типу лицензии (LicenseKind*); "" — неизвестно. Таблица приведения — данные источника.
	Kind string `gorm:"size:16" json:"kind,omitempty"`
	// OrgType — для кого лицензия: commercial | academic | government | nonprofit | home; "" — не указано.
	OrgType string `gorm:"size:16" json:"org_type,omitempty"`
	// TierMin, TierMax — ступень цены за единицу («от 100 до 149 узлов»); Quantity у такой позиции — TierMin.
	// TierMax nil при TierMin — «от N и больше».
	TierMin *int `json:"tier_min,omitempty"`
	TierMax *int `json:"tier_max,omitempty"`
}

// Виды позиции по типу лицензии (License.Kind): базовая лицензия выше продлений, поддержки, модулей и курсов.
const (
	LicenseKindBase      = "base"      // полная версия, подписка
	LicenseKindRenewal   = "renewal"   // продление
	LicenseKindUpgrade   = "upgrade"   // обновление версии или редакции
	LicenseKindMigration = "migration" // льготный переход с другого продукта — не скидка
	LicenseKindSupport   = "support"   // техподдержка
	LicenseKindModule    = "module"    // модуль, расширение, add-on
	LicenseKindCourse    = "course"    // курс, обучение
	LicenseKindOther     = "other"
)

// LicenseKinds — все виды позиции.
var LicenseKinds = []string{LicenseKindBase, LicenseKindRenewal, LicenseKindUpgrade, LicenseKindMigration,
	LicenseKindSupport, LicenseKindModule, LicenseKindCourse, LicenseKindOther}

// Vendor — производитель.
type Vendor struct {
	ID     string `gorm:"primaryKey" json:"id"`
	Name   string `json:"name"`
	Origin string `json:"origin"` // ru | foreign
	// URL — страница вендора на сайте магазина, Slug — её последний сегмент (контур фида; в mock пусто).
	// «Вендор + категория» — URL + "?filter[category][]=" + id категории второго уровня.
	URL  string `json:"url,omitempty"`
	Slug string `json:"slug,omitempty"`
}

// Category — категория; ParentID nil — корень.
type Category struct {
	ID       string  `gorm:"primaryKey" json:"id"`
	Name     string  `json:"name"`
	ParentID *string `gorm:"index" json:"parent_id"`
}

// Типы связей между семействами (Relation.Type). Аналог — не синоним.
const (
	RelationSubstitute    = "substitute"
	RelationComplement    = "complement"
	RelationRenewal       = "renewal"
	RelationCompatibility = "compatibility"
	RelationUpgrade       = "upgrade" // старшая редакция (контур коммерции)
)

// RelationTypes — все типы связей.
var RelationTypes = []string{RelationSubstitute, RelationComplement, RelationRenewal, RelationCompatibility, RelationUpgrade}

// Relation — типизированная связь семейств. Поля контура коммерции необязательны: в источнике контура поиска
// их нет, значения по умолчанию — requirement optional, compatibility unknown.
type Relation struct {
	UUIDModel
	// RelID — id связи в источнике ("rel-0001"); "" у связей контура поиска. Первичный ключ таблицы — UUID.
	RelID      string `gorm:"column:rel_id;index" json:"id"`
	FromFamily string `gorm:"index" json:"from_family"`
	ToFamily   string `gorm:"index" json:"to_family"`
	// FromProduct, ToProduct — уточнение до позиции; nil — вся семья.
	FromProduct *string `json:"from_product"`
	ToProduct   *string `json:"to_product"`
	Type        string  `json:"type"`
	// Note — одна строка «почему вместе» (= reason компаньона).
	Note             string   `json:"note"`
	Requirement      string   `gorm:"size:24" json:"requirement"`   // conditional_required | recommended | optional
	Compatibility    string   `gorm:"size:16" json:"compatibility"` // verified | conditional | incompatible | unknown
	Conditions       []string `gorm:"serializer:json;type:jsonb" json:"conditions"`
	QuantityStrategy string   `gorm:"size:16" json:"quantity_strategy"` // per_user | per_device | … | one | none
	Source           string   `gorm:"size:24" json:"source"`            // vendor_documentation | catalog_rule | expert | co_purchase | agent_proposal
	SourceURL        string   `json:"source_url"`
	EvidenceLevel    string   `gorm:"size:4" json:"evidence_level"` // E0..E5
	ReviewStatus     string   `gorm:"size:16" json:"review_status"` // approved | pending | rejected
	ReviewedAt       string   `json:"reviewed_at"`                  // дата YYYY-MM-DD
	// UpgradeAdds — что добавится при переходе на старшую редакцию (только type=upgrade).
	UpgradeAdds []string `gorm:"serializer:json;type:jsonb" json:"upgrade_adds"`
}

// Цели алиаса (Alias.Target).
const (
	AliasTargetVendor   = "vendor"
	AliasTargetFamily   = "family"
	AliasTargetCategory = "category"
	AliasTargetTerm     = "term"
)

// Alias — «как пишут → как в каталоге», только в одну сторону.
type Alias struct {
	UUIDModel
	Alias     string `gorm:"index" json:"alias"`
	Canonical string `json:"canonical"`
	Kind      string `json:"kind"`   // slang | abbreviation | transliteration | alias
	Target    string `json:"target"` // vendor | family | category | term
	TargetID  string `json:"target_id"`
}

// Сегменты клиентов (Client.Segment).
const (
	SegmentSMB        = "smb"
	SegmentEnterprise = "enterprise"
	SegmentGov        = "gov"
	SegmentIndividual = "individual"
)

// Client — покупатель (mock): сегмент и история покупок для контекста выдачи.
type Client struct {
	ID        string     `gorm:"primaryKey" json:"id"`
	Name      string     `json:"name"`
	Segment   string     `json:"segment"`
	Purchases []Purchase `gorm:"serializer:json;type:jsonb" json:"purchases"`
}

// Purchase — покупка клиента.
type Purchase struct {
	ProductID string `json:"product_id"`
	Date      string `json:"date"`
}

// CatalogSnapshot — каталог целиком из одного источника: единица импорта.
type CatalogSnapshot struct {
	Version    string
	Products   []Product
	Vendors    []Vendor
	Categories []Category
	Relations  []Relation
	Aliases    []Alias
	Clients    []Client
	// Контур коммерции: справочники необязательны — нет файла, пустой срез.
	LicenseRules         []LicenseRule
	CompatibilityRules   []CompatibilityRule
	AttributeDefinitions []AttributeDefinition
	SolutionTemplates    []SolutionTemplate
	// Synthetic — что в данных придумано (catalog.synthetic: "margin", "orders") → плашка «Синтетические данные».
	Synthetic []string
	// Warnings — некритичные расхождения источника (нет справочника, ссылка в никуда).
	Warnings []string
}
