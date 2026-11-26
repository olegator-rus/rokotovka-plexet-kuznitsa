package out

// Страница товара: GET /api/products/{id} — одна позиция каталога для витрины. Ответ одинаков для
// гостя, покупателя и сотрудника: маржи и популярности в нём нет ни для кого.

// ProductPage — позиция каталога со всем, что о ней известно: условия, статус реестра, характеристики, другие
// варианты семейства, совместимость, «С этим покупают» и аналоги. Массивы — всегда массивы.
type ProductPage struct {
	CatalogVersion string          `json:"catalog_version"`
	ProductID      string          `json:"product_id"`
	SKU            string          `json:"sku"`
	MPN            string          `json:"mpn"`
	Kind           string          `json:"kind"` // software | hardware
	FamilyID       string          `json:"family_id"`
	FamilyName     string          `json:"family_name"`
	Title          string          `json:"title"`
	Vendor         ProductVendor   `json:"vendor"`
	Category       ProductCategory `json:"category"`
	Description    string          `json:"description"`
	LicenseLine    string          `json:"license_line"`
	License        ProductLicense  `json:"license"`
	PriceRub       *float64        `json:"price_rub"`
	PriceOnRequest bool            `json:"price_on_request"`
	PriceMode      string          `json:"price_mode"`
	Origin         string          `json:"origin"` // ru | foreign
	// Reestr — статус в реестре российского ПО; null у оборудования.
	Reestr        *Reestr               `json:"reestr"`
	Attributes    []ProductAttr         `json:"attributes"`
	Specs         []ProductSpec         `json:"specs"` // [] у ПО
	Variants      []ProductVariant      `json:"variants"`
	Compatibility []ProductCompat       `json:"compatibility"`
	Companions    ProductPageCompanions `json:"companions"`
	Analogs       []ProductAnalog       `json:"analogs"`
}

// ProductVendor — производитель; Origin — происхождение позиции (в каталоге его задаёт производитель).
type ProductVendor struct {
	ID     string `json:"id"`
	Name   string `json:"name"`
	Origin string `json:"origin"`
}

// ProductCategory — категория позиции.
type ProductCategory struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// ProductLicense — условия лицензии: nil у metric, quantity — не заданы; nil у duration_months — бессрочная.
type ProductLicense struct {
	Type           string  `json:"type"`
	Metric         *string `json:"metric"`
	Quantity       *int    `json:"quantity"`
	DurationMonths *int    `json:"duration_months"`
	Edition        string  `json:"edition"`
}

// ProductAttr — атрибут позиции: подпись даёт сервер, значение — строкой.
type ProductAttr struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// ProductSpec — числовая характеристика оборудования с единицей.
type ProductSpec struct {
	Key   string  `json:"key"`
	Label string  `json:"label"`
	Value float64 `json:"value"`
	Unit  string  `json:"unit"`
}

// ProductVariant — другая позиция того же семейства.
type ProductVariant struct {
	ProductID      string   `json:"product_id"`
	Title          string   `json:"title"`
	LicenseLine    string   `json:"license_line"`
	PriceRub       *float64 `json:"price_rub"`
	PriceOnRequest bool     `json:"price_on_request"`
	PriceMode      string   `json:"price_mode"`
}

// ProductCompat — правило совместимости позиции с моделью системы.
type ProductCompat struct {
	TargetModel string `json:"target_model"`
	State       string `json:"state"` // verified | conditional | incompatible | unknown
	Condition   string `json:"condition"`
	SourceURL   string `json:"source_url"`
}

// ProductPageCompanions — «С этим покупают»: тот же подбор, что у GET /api/companions, без старшей редакции.
type ProductPageCompanions struct {
	Required []Companion `json:"required"`
	Items    []Companion `json:"items"`
}

// ProductAnalog — аналог по связи substitute от семейства: представитель семейства-замены.
type ProductAnalog struct {
	FamilyID       string   `json:"family_id"`
	FamilyName     string   `json:"family_name"`
	ProductID      string   `json:"product_id"`
	Title          string   `json:"title"`
	Vendor         string   `json:"vendor"`
	LicenseLine    string   `json:"license_line"`
	PriceRub       *float64 `json:"price_rub"`
	PriceOnRequest bool     `json:"price_on_request"`
	PriceMode      string   `json:"price_mode"`
	Note           string   `json:"note"`
}
