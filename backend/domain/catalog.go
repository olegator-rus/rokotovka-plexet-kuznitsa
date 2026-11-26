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
}

// Vendor — производитель.
type Vendor struct {
	ID     string `gorm:"primaryKey" json:"id"`
	Name   string `json:"name"`
	Origin string `json:"origin"` // ru | foreign
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
)

// Relation — типизированная связь семейств.
type Relation struct {
	UUIDModel
	FromFamily string `gorm:"index" json:"from_family"`
	ToFamily   string `gorm:"index" json:"to_family"`
	Type       string `json:"type"`
	Note       string `json:"note"`
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
	// Warnings — некритичные расхождения источника (нет справочника, ссылка в никуда).
	Warnings []string
}
