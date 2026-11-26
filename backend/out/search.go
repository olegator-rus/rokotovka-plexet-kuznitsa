package out

import (
	"time"

	"kuznica/api/domain"
)

// Ответы поиска и витрины. Поля correction, clarify и empty
// отсутствуют, когда им нечего сказать; массивы — всегда массивы.

// SearchResponse — POST /api/search.
type SearchResponse struct {
	RequestID      string       `json:"request_id"`
	Query          string       `json:"query"`
	Normalized     string       `json:"normalized"`
	Config         string       `json:"config"`
	Correction     *Correction  `json:"correction,omitempty"`
	Chips          []Chip       `json:"chips"`
	Clarify        *Clarify     `json:"clarify,omitempty"`
	Total          int          `json:"total"`
	Items          []SearchItem `json:"items"`
	Relaxed        []Relaxed    `json:"relaxed"`
	Empty          *Empty       `json:"empty,omitempty"`
	RelatedQueries []string     `json:"related_queries"`
	// Complements — «С этим покупают»: семейства, связанные с первой позицией выдачи (relations complement),
	// до 3; пустой массив — связей нет или конфигурация без справочников (B0–B2).
	Complements []Complement `json:"complements"`
	// Контур коммерции: поля всегда присутствуют. Companions — «Дополните решение» (null — блока нет),
	// Solution — комплексное решение (null), Facets — фасеты числовых характеристик ([]).
	Companions *CompanionBlock `json:"companions"`
	Solution   *Solution       `json:"solution"`
	// Upgrade — «Старшая редакция» рядом с выдачей; null — связи upgrade нет или флаг решений выключен.
	Upgrade *Upgrade `json:"upgrade"`
	Facets  []Facet  `json:"facets"`
	// Контур выдачи, страница результатов (§11.5, results.go): только при FEATURE_RESULTS_V3, иначе полей нет.
	Counts      *ResultCounts `json:"counts,omitempty"`
	Refinements []Refinement  `json:"refinements,omitempty"`
	Sort        *ResultSort   `json:"sort,omitempty"`
	Group       string        `json:"group,omitempty"` // family | none
	// Контур доводки (out/procurement.go): параметры закупки, ответ справки, «Вы искали» и позиции,
	// убранные из основного блока явным требованием реестра.
	Purchase          *Purchase       `json:"purchase,omitempty"`
	Help              *HelpAnswer     `json:"help,omitempty"`
	AnalogOriginal    *AnalogOriginal `json:"analog_original,omitempty"`
	ReestrUnconfirmed []SearchItem    `json:"reestr_unconfirmed,omitempty"`
	Trace             *domain.Trace   `json:"trace"`
}

// Complement — сопутствующий товар: самая популярная позиция связанного семейства.
type Complement struct {
	FamilyID       string   `json:"family_id"`
	FamilyName     string   `json:"family_name"`
	ProductID      string   `json:"product_id"`
	Title          string   `json:"title"`
	Vendor         string   `json:"vendor"`
	Category       string   `json:"category"`
	LicenseLine    string   `json:"license_line"`
	PriceRub       *float64 `json:"price_rub"`
	PriceOnRequest bool     `json:"price_on_request"`
	// Note — почему покупают вместе (пояснение связи из справочника).
	Note string `json:"note"`
}

// Correction — исправление запроса.
type Correction struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Confidence string `json:"confidence"` // high | medium
	Applied    bool   `json:"applied"`
}

// Chip — понятое условие запроса. Op и Unit — у числовых характеристик контура коммерции («Память ≥ 32 ГБ»).
type Chip struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Label  string `json:"label"`
	Strict bool   `json:"strict"`
	Op     string `json:"op,omitempty"`
	Unit   string `json:"unit,omitempty"`
}

// Clarify — уточняющий вопрос.
type Clarify struct {
	Attribute string          `json:"attribute"`
	Question  string          `json:"question"`
	Options   []ClarifyOption `json:"options"`
}

// ClarifyOption — вариант ответа на уточнение.
type ClarifyOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SearchItem — позиция выдачи: лучшая лицензия семейства, остальные — в Variants.
type SearchItem struct {
	ProductID       string   `json:"product_id"`
	FamilyID        string   `json:"family_id"`
	Title           string   `json:"title"`
	Vendor          string   `json:"vendor"`
	Category        string   `json:"category"`
	LicenseLine     string   `json:"license_line"`
	PriceRub        *float64 `json:"price_rub"`
	PriceOnRequest  bool     `json:"price_on_request"`
	Group           string   `json:"group"` // exact | match | analog
	MatchesQuery    bool     `json:"matches_query"`
	PurchasedBefore bool     `json:"purchased_before"`
	Final           float64  `json:"final"`
	// Контур коммерции: всегда присутствуют. Kind — software | hardware; MPN — парт-номер ("" у ПО); PriceMode — тип цены;
	// Compatibility — при запросе «X для Y», иначе null; Required — условно обязательное к позиции ([]).
	Kind          string             `json:"kind"`
	MPN           string             `json:"mpn"`
	PriceMode     string             `json:"price_mode"`
	Compatibility *ItemCompatibility `json:"compatibility"`
	Required      []Companion        `json:"required"`
	Variants      []SearchItem       `json:"variants,omitempty"`
	// Контур выдачи (§11.5): цена «от» по семейству (минимум по допустимым вариантам с ценой) и атрибуты карточки.
	PriceFromRub *float64   `json:"price_from_rub,omitempty"`
	Attrs        *ItemAttrs `json:"attrs,omitempty"`
	// FilterFill — семейство не найдено поиском, а дописано фильтром категории/производителя из панели (§11.8,
	// signals filter_fill): фронт показывает его отдельно — «Ещё в категории …».
	FilterFill bool `json:"filter_fill,omitempty"`
	// Контур доводки: статус реестра семейства и строка отличий аналога отсутствующего товара.
	Reestr     *Reestr `json:"reestr,omitempty"`
	AnalogNote string  `json:"analog_note,omitempty"`
}

// ItemCompatibility — совместимость позиции с моделью из запроса.
type ItemCompatibility struct {
	TargetModel string `json:"target_model"`
	State       string `json:"state"` // verified | conditional | incompatible | unknown
	Condition   string `json:"condition"`
}

// Companion — позиция блока «Дополните решение» или строки «условно обязательное».
type Companion struct {
	ProductID   string   `json:"product_id"`
	FamilyID    string   `json:"family_id"`
	Title       string   `json:"title"`
	Vendor      string   `json:"vendor"`
	Category    string   `json:"category"`
	LicenseLine string   `json:"license_line"`
	PriceRub    *float64 `json:"price_rub"`
	PriceMode   string   `json:"price_mode"`
	// Class — conditional_required | compatible | complement | upgrade | renewal; incompatible в блок не попадает.
	Class             string   `json:"class"`
	Compatibility     string   `json:"compatibility"` // verified | conditional | unknown
	Reason            string   `json:"reason"`
	Conditions        []string `json:"conditions"`
	EvidenceLevel     string   `json:"evidence_level"`
	Source            string   `json:"source"`
	SourceURL         string   `json:"source_url"`
	SuggestedQuantity *int     `json:"suggested_quantity"`
	Label             string   `json:"label"`
	UpgradeAdds       []string `json:"upgrade_adds"`
}

// CompanionBlock — «Дополните решение»: якорь и 2–4 компаньона.
type CompanionBlock struct {
	Anchor CompanionAnchor `json:"anchor"`
	Items  []Companion     `json:"items"`
}

// CompanionAnchor — к чему подобраны компаньоны.
type CompanionAnchor struct {
	ProductID  string `json:"product_id"`
	FamilyID   string `json:"family_id"`
	Confidence string `json:"confidence"` // high | medium
}

// Solution — комплексное решение по шаблону. Seats — число мест из запроса (нет — 1);
// TotalRub — сумма по выбранным позициям, только если известны все цены, иначе null и OnRequest.
type Solution struct {
	TemplateID string         `json:"template_id"`
	Title      string         `json:"title"`
	Seats      int            `json:"seats"`
	Source     string         `json:"source"`
	Slots      []SolutionSlot `json:"slots"`
	TotalRub   *float64       `json:"total_rub"`
	OnRequest  int            `json:"on_request"`
}

// SolutionSlot — слот решения: подобранная позиция и альтернативы («заменить»). Quantity — сколько штук выбранной
// позиции заказать (фасовка на N мест — ⌈N / мест в позиции⌉); Quantities — то же для каждой альтернативы.
type SolutionSlot struct {
	ID           string         `json:"id"`
	Label        string         `json:"label"`
	CategoryID   string         `json:"category_id"`
	Query        string         `json:"query"`
	Strategy     string         `json:"quantity_strategy"`
	Quantity     int            `json:"quantity"`
	Item         *SearchItem    `json:"item"`
	Alternatives []SearchItem   `json:"alternatives"`
	Quantities   map[string]int `json:"quantities"`
	// Active — слот входит в решение; условный слот («ОС, если ноутбук без ОС») активен, только когда выбранная
	// позиция слота Condition.Slot — из Condition.ProductIDs (при замене позиции фронт сверяет по этому списку).
	Active    bool           `json:"active"`
	Condition *SlotCondition `json:"condition"`
}

// SlotCondition — условие слота на позицию другого слота.
type SlotCondition struct {
	Slot       string   `json:"slot"`
	Key        string   `json:"key"`
	Value      string   `json:"value"`
	ProductIDs []string `json:"product_ids"`
}

// Upgrade — «Старшая редакция» к первой позиции выдачи: связь upgrade, 1–2 строки «что добавится».
type Upgrade struct {
	ForProductID string `json:"for_product_id"`
	RelationID   string `json:"relation_id"`
	Companion
}

// Facet — фасет числовой или перечислимой характеристики.
type Facet struct {
	Key    string       `json:"key"`
	Label  string       `json:"label"`
	Unit   string       `json:"unit"`
	Type   string       `json:"type"` // number | enum | bool
	Min    *float64     `json:"min"`
	Max    *float64     `json:"max"`
	Values []FacetValue `json:"values"`
}

// FacetValue — значение фасета и сколько позиций выдачи с ним.
type FacetValue struct {
	Value any `json:"value"`
	Count int `json:"count"`
	// Label — подпись значения (лицензионные фасеты §11.5: «Продление», «12 мес»); "" — подпись = значение.
	Label string `json:"label,omitempty"`
}

// Relaxed — ослабленное условие и объяснение покупателю.
type Relaxed struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Empty — точного товара нет.
type Empty struct {
	Message     string `json:"message"`
	ShowManager bool   `json:"show_manager"`
	// Supply — товара нет в каталоге: кнопка «Запросить поставку» (событие supply_request с исходным запросом).
	Supply bool `json:"supply,omitempty"`
}

// Meta — GET /api/meta.
type Meta struct {
	LLMMode        string       `json:"llm_mode"`
	CatalogVersion string       `json:"catalog_version"`
	RulesVersion   int          `json:"rules_version"`
	Configs        []string     `json:"configs"`
	DefaultConfig  string       `json:"default_config"`
	Clients        []MetaClient `json:"clients"`
	// LLM — режим модели, дневной бюджет и плашка режима для шапки.
	LLM LLMState `json:"llm"`
	// Vectors — режим векторов (эмбеддер и реранкер) и плашка режима «без моделей» (контур доводки).
	Vectors VectorsState `json:"vectors"`
	// AdminEnabled — доступны ли служебные методы /api/admin/* (ADMIN_ENABLED).
	AdminEnabled bool `json:"admin_enabled"`
	// RateLimitPerMin — лимит частоты запросов к /api с одного адреса; 0 — без лимита.
	RateLimitPerMin int `json:"rate_limit_per_min"`
	// Session — подписанный сервером токен сессии покупателя (handlers/session.go): фронт шлёт его в session_id
	// поиска и событий; самодельные сессии в журнал не попадают.
	Session string `json:"session"`
	// Viewer — кто смотрит сайт: роль, имя, клиент покупателя из сессии.
	Viewer Viewer `json:"viewer"`
	// Features — флаги контура коммерции (FEATURE_*): фронт не показывает блоки выключенных возможностей.
	Features Features `json:"features"`
	// Synthetic — что в данных придумано (catalog.synthetic) → плашка «Синтетические данные»; [] — нет.
	Synthetic []string `json:"synthetic"`
}

// Features — флаги контура коммерции.
type Features struct {
	Hardware     bool `json:"hardware"`
	Companions   bool `json:"companions"`
	MarginPolicy bool `json:"margin_policy"`
	Solutions    bool `json:"solutions"`
	// ResultsV3 — страница результатов контура выдачи (§11.5): фасеты лицензий, сортировки, группировка.
	ResultsV3 bool `json:"results_v3"`
	// Панель поиска контура выдачи (§11.2–§11.3): панель, товары в ней, маржа как разрешение равенства, блок рекомендаций;
	// PanelViewed — основание «недавно просмотренное» (по умолчанию выкл.).
	Panel         bool `json:"panel"`
	PanelProducts bool `json:"panel_products"`
	PanelMargin   bool `json:"panel_margin"`
	PanelRecs     bool `json:"panel_recs"`
	PanelViewed   bool `json:"panel_viewed"`
}

// LLMState — режим модели и бюджет (GET /api/meta → llm, POST /api/admin/llm-mode).
type LLMState struct {
	// Mode — on | off: идут ли сложные запросы (маршрут D) в модель. Переключается без перезапуска.
	Mode string `json:"mode"`
	// Online — модель настроена (ключ задан); без неё режим on недоступен.
	Online    bool       `json:"online"`
	ChangedAt *time.Time `json:"changed_at"`
	Budget    LLMBudget  `json:"budget"`
	// Banner — текст плашки в шапке: режим без LLM или исчерпан бюджет; null — плашка не нужна.
	Banner *string `json:"banner"`
}

// LLMBudget — дневной бюджет на модель (LLM_DAILY_BUDGET_RUB), сутки по Москве.
type LLMBudget struct {
	Day string `json:"day"`
	// DailyRub — потолок, ₽; null — без потолка.
	DailyRub      *float64 `json:"daily_rub"`
	SpentTodayRub float64  `json:"spent_today_rub"`
	Exceeded      bool     `json:"exceeded"`
}

// MetaClient — покупатель для переключателя «от чьего имени ищем».
type MetaClient struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Segment string `json:"segment"`
	Hint    string `json:"hint"`
}

// ImportResult — POST /api/catalog/import и `go run . import`: что загружено и сколько это заняло.
type ImportResult struct {
	Source         string   `json:"source"`
	CatalogVersion string   `json:"catalog_version"`
	Products       int      `json:"products"`
	Families       int      `json:"families"`
	Vendors        int      `json:"vendors"`
	Categories     int      `json:"categories"`
	Relations      int      `json:"relations"`
	Aliases        int      `json:"aliases"`
	Clients        int      `json:"clients"`
	Index          string   `json:"index"`
	Vectors        bool     `json:"vectors"`
	Database       bool     `json:"database"` // записан ли каталог в PostgreSQL
	RulesVersion   int      `json:"rules_version"`
	EmbedMs        int64    `json:"embed_ms"`
	IndexMs        int64    `json:"index_ms"`
	DatabaseMs     int64    `json:"database_ms"`
	TotalMs        int64    `json:"total_ms"`
	Warnings       []string `json:"warnings"`
}

// ProductCompanions — GET /api/companions: «К этой позиции» в спецификации.
type ProductCompanions struct {
	ProductID string      `json:"product_id"`
	Required  []Companion `json:"required"`
	Items     []Companion `json:"items"`
	Upgrade   *Upgrade    `json:"upgrade"`
}
