package out

// SuggestResponse — GET /api/suggest (§11.2). Groups — прежний ответ (не больше
// 8 строк суммарно) для старых потребителей; остальные поля — панель контура выдачи, присутствуют всегда.
type SuggestResponse struct {
	Groups []SuggestGroup `json:"groups"`

	Q           string            `json:"q"`
	Normalized  string            `json:"normalized"`
	Mode        string            `json:"mode"` // prefix | empty | short | off
	Corrected   *SuggestCorrected `json:"corrected"`
	Refinements []Refinement      `json:"refinements"`
	Suggestions []PanelSuggestion `json:"suggestions"`
	Categories  []PanelCategory   `json:"categories"`
	Vendors     []PanelVendor     `json:"vendors"`
	Products    []PanelProduct    `json:"products"`
	TotalHint   int               `json:"total_hint"`
	// Together — строка «Вместе с этим» (контур эталонов): одно-два дополнения из таблицы классов для первого семейства
	// Products; [] — класса нет или дополнений с ценой нет.
	Together []PanelTogether `json:"together"`
	Meta     SuggestMeta     `json:"meta"`
}

// PanelTogether — дополнение в строке «Вместе с этим»: к какому семейству (AnchorFamilyID), что предлагаем, причина
// из таблицы классов, уровень основания A|B, вид (class_logic | compat_document), ссылка на документ (SourceURL, у A).
type PanelTogether struct {
	AnchorFamilyID string   `json:"anchor_family_id"`
	FamilyID       string   `json:"family_id"`
	FamilyName     string   `json:"family_name"`
	ProductID      string   `json:"product_id"`
	Title          string   `json:"title"`
	Vendor         string   `json:"vendor"`
	PriceRub       *float64 `json:"price_rub"`
	Reason         string   `json:"reason"`
	Level          string   `json:"level"`
	Kind           string   `json:"kind"`
	Label          string   `json:"label"`
	OfferURL       string   `json:"offer_url,omitempty"`
	FamilyURL      string   `json:"family_url,omitempty"`
	SourceURL      string   `json:"source_url,omitempty"`
}

// SuggestGroup — группа подсказок: query | product | vendor | category.
type SuggestGroup struct {
	Type  string        `json:"type"`
	Label string        `json:"label"`
	Items []SuggestItem `json:"items"`
}

// SuggestItem — строка подсказки. Highlight — совпавшая часть [начало, конец) в символах; nil — совпало
// по n-граммам, без точного участка.
type SuggestItem struct {
	Text      string  `json:"text"`
	ID        *string `json:"id"`
	Highlight []int   `json:"highlight"`
}

// SuggestCorrected — по чему подобраны секции, если ввод исправлен (строку покупателя фронт не переписывает).
type SuggestCorrected struct {
	From string `json:"from"`
	To   string `json:"to"`
	Kind string `json:"kind"` // layout | typo
}

// Refinement — бейдж-уточнение (§11.2, §11.5): query — текст запроса с уточнением; field/value/op — структурированное
// условие (фильтр страницы результатов), null — уточнение дописывает текст. Hits — семейств с уточнением.
type Refinement struct {
	Label string  `json:"label"`
	Query string  `json:"query"`
	Field *string `json:"field"`
	Value *string `json:"value"`
	Op    *string `json:"op"`
	Hits  int     `json:"hits"`
}

// PanelSuggestion — подсказка-интент. Highlight — набранная часть [[начало, конец)] в символах.
type PanelSuggestion struct {
	Text      string   `json:"text"`
	Highlight [][2]int `json:"highlight"`
	Kind      string   `json:"kind"`   // query | category
	Source    string   `json:"source"` // successful_query | catalog_template | popular
	Hits      int      `json:"hits"`
	// CategoryID, CategoryName — у kind = category: категория, в которую ведёт подсказка (§11.7).
	CategoryID   string `json:"category_id,omitempty"`
	CategoryName string `json:"category_name,omitempty"`
}

// PanelCategory — категория запроса при уверенности; Share — взвешенная доля среди семейств-кандидатов.
type PanelCategory struct {
	ID    string  `json:"id"`
	Name  string  `json:"name"`
	Share float64 `json:"share"`
}

// PanelVendor — производитель, названный во вводе.
type PanelVendor struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// PanelProduct — семейство в правой колонке панели; представитель — самый релевантный вариант.
type PanelProduct struct {
	FamilyID      string   `json:"family_id"`
	FamilyName    string   `json:"family_name"`
	ProductID     string   `json:"product_id"`
	Title         string   `json:"title"`
	Kind          string   `json:"kind"` // software | hardware
	Vendor        string   `json:"vendor"`
	Category      string   `json:"category"`
	MPN           string   `json:"mpn"`
	LicenseLine   string   `json:"license_line"`
	SpecsLine     string   `json:"specs_line"`
	PriceFromRub  *float64 `json:"price_from_rub"`
	PriceMode     string   `json:"price_mode"`
	VariantsCount int      `json:"variants_count"`
	Exact         bool     `json:"exact"`
	SKUExact      bool     `json:"sku_exact"`
	// DeliveryLine — способ поставки оборудования из данных каталога (сроков поставки в данных нет); ImageURL — фото
	// (в данных нет — null). §11.7.
	DeliveryLine *string `json:"delivery_line"`
	ImageURL     *string `json:"image_url"`
}

// SuggestMeta — время секций, признак частичного ответа, версии и счётчики вызовов моделей (на панели всегда 0).
type SuggestMeta struct {
	Ms             SuggestMs `json:"ms"`
	Partial        bool      `json:"partial"`
	ProductsCached bool      `json:"products_cached"`
	LLMCalls       int       `json:"llm_calls"`
	EmbedCalls     int       `json:"embed_calls"`
	RerankCalls    int       `json:"rerank_calls"`
	IndexVersion   int       `json:"index_version"`
	CatalogVersion string    `json:"catalog_version"`
	// SuggestionsVersion — сборка индекса подсказок (config_versions kind = suggestions); 0 — собран в памяти.
	SuggestionsVersion int    `json:"suggestions_version"`
	RequestID          string `json:"request_id"`
}

// SuggestMs — миллисекунды секций на сервере.
type SuggestMs struct {
	Total       float64 `json:"total"`
	Suggestions float64 `json:"suggestions"`
	Products    float64 `json:"products"`
	Refinements float64 `json:"refinements"`
}

// PanelRecommendations — GET /api/suggest/recommendations (§11.3); блока нет — ответ null.
type PanelRecommendations struct {
	Basis string      `json:"basis"` // spec | exact_sku | viewed | category_popular
	Title string      `json:"title"`
	Why   string      `json:"why"`
	Items []Companion `json:"items"`
}
