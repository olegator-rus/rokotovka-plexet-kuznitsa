package out

import "kuznica/api/domain"

// Режим подбора (контур подбора). Всё, что видит покупатель, кроме связующей фразы lead, строит код по
// идентификаторам: таблица, карточки, цены, артикулы, ссылки.

// Типы ответа подбора.
const (
	GuideComparison    = "comparison"
	GuideSelection     = "selection"
	GuideClarification = "clarification"
	GuideNoData        = "no_data"
	GuideOutOfScope    = "out_of_scope"
	GuideFallback      = "fallback"
)

// GuideResponse — POST /api/guide.
type GuideResponse struct {
	Type          string              `json:"type"`
	Lead          string              `json:"lead"`
	Claims        []GuideClaim        `json:"claims"`
	Comparison    *GuideComparisonTbl `json:"comparison"`
	Offers        []GuideOffer        `json:"offers"`
	Clarification *GuideClarify       `json:"clarification"`
	Conditions    []GuideCondition    `json:"conditions"`
	Changes       []GuideChange       `json:"changes"`
	Next          []string            `json:"next"`
	Trace         []GuideTraceStep    `json:"trace"`
	State         domain.GuideState   `json:"state"`
	// Preliminary — результат предварительный: не решён вопрос, от которого зависит право использования, или
	// правило лицензирования ещё не сверено человеком.
	Preliminary bool `json:"preliminary"`
	// Notice — служебная строка блока («Помочь не удалось — выдача и условия ниже»); "" — нет.
	Notice string `json:"notice"`
	// Gaps — события пробелов для SearchOps (SPEC §12): missing_fact, license_unresolved, no_matching_offer, …
	Gaps []string `json:"gaps"`
	// Meta — время и стоимость хода (SPEC §13).
	Meta GuideMeta `json:"meta"`
	// Recommendation — одна рекомендация среди подходящих с причиной из каталога или фактов; null — нет.
	Recommendation *GuideRecommendation `json:"recommendation"`
	// Docs — ссылка на документацию вендора у ответа «нет сведений» (SPEC §9); null — нет.
	Docs *GuideDocs `json:"docs"`
	// Screen — готовые поля экрана подбора (контур экрана); null — экрана результата нет.
	Screen *GuideScreen `json:"screen"`
}

// GuideDocs — страница вендора или семейства.
type GuideDocs struct {
	Title string `json:"title"`
	URL   string `json:"url"`
}

// GuideClaim — утверждение о свойстве продукта: только проверенный факт базы знаний с источником.
type GuideClaim struct {
	FactID string      `json:"fact_id"`
	Text   string      `json:"text"`
	Source GuideSource `json:"source"`
	Status string      `json:"status"` // verified
}

// GuideSource — источник факта: страница вендора и фрагмент. Дат нет.
type GuideSource struct {
	// Label — подпись ссылки: «Документация вендора» или «Правила лицензирования {вендор}» (контур экрана).
	Label    string `json:"label,omitempty"`
	Title    string `json:"title"`
	URL      string `json:"url"`
	Fragment string `json:"fragment"`
}

// GuideComparisonTbl — таблица сравнения: колонки — семейства (редакции), строки — оси и коммерческие строки.
type GuideComparisonTbl struct {
	Columns []GuideColumn `json:"columns"`
	Rows    []GuideRow    `json:"rows"`
}

// GuideColumn — колонка таблицы.
type GuideColumn struct {
	FamilyID  string `json:"family_id"`
	Title     string `json:"title"`
	FamilyURL string `json:"family_url"`
}

// GuideRow — строка таблицы; Note — пояснение строки (почему прочерк в цене).
type GuideRow struct {
	Key   string      `json:"key"`
	Label string      `json:"label"`
	Kind  string      `json:"kind"` // feature | commercial
	Cells []GuideCell `json:"cells"`
	Note  string      `json:"note,omitempty"`
}

// Значения ячейки таблицы.
const (
	CellYes     = "yes"     // есть
	CellNo      = "no"      // нет по данным вендора
	CellValue   = "value"   // значение (число, текст, «зависит от ваших условий»)
	CellUnknown = "unknown" // нет сведений
)

// GuideCell — ячейка; FactID — факт-источник у строк о функциях; Value — число (рубли) или текст.
type GuideCell struct {
	Kind   string `json:"kind"`
	Value  any    `json:"value,omitempty"`
	FactID string `json:"fact_id,omitempty"`
}

// Роли карточки подбора.
const (
	OfferMatch       = "match"       // подходит под условия
	OfferAlternative = "alternative" // «При изменении условий»: названное отличие в Note
	OfferRejected    = "rejected"    // не подходит по правилу лицензирования (блок «Не подходит …»)
)

// GuideOffer — карточка подобранной лицензии: всё из каталога. PricePerUnit — цена за единицу продажи; Total —
// на заданное количество (nil — цена по запросу или количество вне ступени).
type GuideOffer struct {
	OfferID      string   `json:"offer_id"`
	FamilyID     string   `json:"family_id"`
	Title        string   `json:"title"`
	FamilyTitle  string   `json:"family_title"`
	SKU          string   `json:"sku"`
	Tier         string   `json:"tier"`
	Unit         string   `json:"unit"`
	Quantity     int      `json:"quantity"`
	PricePerUnit *float64 `json:"price_per_unit"`
	Total        *float64 `json:"total"`
	PriceState   string   `json:"price_state"` // priced | on_request
	OfferURL     string   `json:"offer_url"`
	Term         string   `json:"term"`
	Role         string   `json:"role"`
	Note         string   `json:"note,omitempty"`
	// PackagePrice, PackageSize — у позиции, которая продаётся пакетом на N мест: цена пакета из каталога и число
	// мест; тогда PricePerUnit — цена за место (PackagePrice / PackageSize), Total — пакеты × PackagePrice.
	PackagePrice *float64 `json:"package_price,omitempty"`
	PackageSize  int      `json:"package_size,omitempty"`
	// RuleID — нарушенное правило у rejected; RuleSource — его источник.
	RuleID     string `json:"rule_id,omitempty"`
	RuleSource string `json:"rule_source,omitempty"`

	// Поля экрана контура экрана: готовые тексты и числа, фронт не считает.
	Display      *GuideOfferDisplay `json:"display,omitempty"`
	SaleUnit     string             `json:"sale_unit,omitempty"`
	SaleUnitKind string             `json:"sale_unit_kind,omitempty"` // per_object | package | license
	// UnitPrice — цена одной единицы продажи из каталога (за компьютер на ступени, за пакет, за сервер).
	UnitPrice *float64 `json:"unit_price"`
	// PayQuantity — сколько единиц продажи к оплате (компьютеров или пакетов).
	PayQuantity int             `json:"pay_quantity"`
	PriceLine   string          `json:"price_line"`
	PriceText   string          `json:"price_text"`
	Package     *GuideOfferPack `json:"package"`
	Reason      *GuideReason    `json:"reason"`
}

// GuideOfferDisplay — заголовок и подзаголовок лицензии (UI 6.1) и раскрытие «Сведения о лицензии».
type GuideOfferDisplay struct {
	Title    string        `json:"title"`
	Subtitle string        `json:"subtitle"`
	Details  []GuideDetail `json:"details"`
}

// GuideDetail — строка раскрытия.
type GuideDetail struct {
	Label string `json:"label"`
	Value string `json:"value"`
}

// GuideOfferPack — пакет на N мест под задачу: сколько пакетов, мест всего и в запасе.
type GuideOfferPack struct {
	Size  int    `json:"size"`
	Packs int    `json:"packs"`
	Seats int    `json:"seats"`
	Spare int    `json:"spare"`
	Text  string `json:"text"`
}

// GuideReason — код причины и готовый текст («Не подходит для серверной ОС»).
type GuideReason struct {
	Code string `json:"code"`
	Text string `json:"text"`
}

// Основания рекомендации (UI 6.2).
const (
	BasisOnlyValid       = "only_valid"
	BasisRequiredFeature = "required_feature"
	BasisLowestTotal     = "lowest_total"
	BasisVendorNamed     = "vendor_named"
	BasisNone            = "none"
)

// GuideScreen — экран подбора контура экрана: всё, что фронт показывает, кроме полей карточек offers[].
type GuideScreen struct {
	Title           string            `json:"title"`
	Task            string            `json:"task"`
	Result          *GuideResult      `json:"result"`
	SelectedOfferID string            `json:"selected_offer_id"`
	Options         []string          `json:"options"`
	Comparison      *GuideScreenTable `json:"comparison"`
	TaskCost        *GuideTaskCost    `json:"task_cost"`
	Changed         *GuideChanged     `json:"changed"`
	Others          []GuideOther      `json:"others"`
	Summary         *GuideSummary     `json:"summary"`
}

// GuideResult — блок результата: основание, заголовок по основанию, позиция и одна фраза причины.
type GuideResult struct {
	Basis   string             `json:"basis"`
	Heading string             `json:"heading"`
	OfferID string             `json:"offer_id"`
	Reason  string             `json:"reason"`
	Source  *GuideScreenSource `json:"source"`
}

// GuideScreenSource — текстовая ссылка на источник с дословным фрагментом. Дат нет.
type GuideScreenSource struct {
	Label    string `json:"label"`
	Fragment string `json:"fragment"`
	URL      string `json:"url"`
}

// GuideScreenTable — сравнение на 4–6 строк (UI 6.3).
type GuideScreenTable struct {
	Title        string             `json:"title"`
	Columns      []GuideScreenCol   `json:"columns"`
	Rows         []GuideScreenRow   `json:"rows"`
	MoreRows     []GuideScreenRow   `json:"more_rows"`
	HiddenNoData int                `json:"hidden_no_data"`
	HiddenLabel  string             `json:"hidden_label"`
	VendorsNote  string             `json:"vendors_note"`
	Difference   string             `json:"difference"`
	Source       *GuideScreenSource `json:"source"`
}

// GuideScreenCol — колонка сравнения.
type GuideScreenCol struct {
	OfferID  string `json:"offer_id"`
	FamilyID string `json:"family_id"`
	Title    string `json:"title"`
}

// GuideScreenRow — строка сравнения: priority по UI 6.3, asked — функция из условий покупателя.
type GuideScreenRow struct {
	Key      string            `json:"key"`
	Label    string            `json:"label"`
	Priority int               `json:"priority"`
	Asked    bool              `json:"asked"`
	Kind     string            `json:"kind"` // feature | commercial
	Cells    []GuideScreenCell `json:"cells"`
}

// Значки ячейки экрана.
const (
	MarkYes         = "yes"
	MarkNo          = "no"
	MarkUnknown     = "unknown"
	MarkConditional = "conditional"
	MarkValue       = "value"
)

// GuideScreenCell — ячейка: mark и готовый текст («Есть», «Нет», «Нет сведений», «С условиями», «2 715,30 ₽»).
type GuideScreenCell struct {
	Mark   string `json:"mark"`
	Text   string `json:"text"`
	FactID string `json:"fact_id,omitempty"`
}

// GuideTaskCost — полная стоимость одной задачи у нескольких вендоров (сценарий 4).
type GuideTaskCost struct {
	Title    string             `json:"title"`
	Columns  []string           `json:"columns"`
	Rows     []GuideTaskCostRow `json:"rows"`
	Note     string             `json:"note"`
	Cheapest string             `json:"cheapest"`
}

// GuideTaskCostRow — строка: как продаётся и сколько стоит вся задача.
type GuideTaskCostRow struct {
	OfferID   string   `json:"offer_id"`
	Title     string   `json:"title"`
	HowSold   string   `json:"how_sold"`
	Total     *float64 `json:"total"`
	TotalText string   `json:"total_text"`
	Note      string   `json:"note"`
}

// GuideChanged — «Что изменилось» и смена ступени цены.
type GuideChanged struct {
	Text        string            `json:"text"`
	Items       []string          `json:"items"`
	PriceChange *GuidePriceChange `json:"price_change"`
}

// GuidePriceChange — прежняя и новая ступень цены выбранного семейства.
type GuidePriceChange struct {
	OfferID string         `json:"offer_id"`
	Label   string         `json:"label"`
	From    GuidePriceTier `json:"from"`
	To      GuidePriceTier `json:"to"`
}

// GuidePriceTier — ступень и цена за единицу.
type GuidePriceTier struct {
	Tier      string  `json:"tier"`
	UnitPrice float64 `json:"unit_price"`
	Text      string  `json:"text"`
}

// GuideOther — строка «При изменении условий»: неподходящая или альтернатива с причиной.
type GuideOther struct {
	OfferID   string       `json:"offer_id"`
	Title     string       `json:"title"`
	Subtitle  string       `json:"subtitle"`
	PriceText string       `json:"price_text"`
	Reason    *GuideReason `json:"reason"`
}

// GuideSummary — закреплённая итоговая строка: сумма один раз, расчёт, одна кнопка.
type GuideSummary struct {
	OfferID   string   `json:"offer_id"`
	Label     string   `json:"label"`
	Total     *float64 `json:"total"`
	TotalText string   `json:"total_text"`
	Formula   string   `json:"formula"`
	Action    string   `json:"action"` // add_to_spec | request_quote | choose
	Hint      string   `json:"hint"`
}

// GuideClarify — один вопрос с чипами и полем для текста.
type GuideClarify struct {
	Key      string      `json:"key"`
	Question string      `json:"question"`
	Chips    []GuideChip `json:"chips"`
	// Required — вопрос о праве использования: без ответа результат предварительный.
	Required bool `json:"required"`
}

// GuideChip — чип: нажатие отправляет {chip: {key, value}}; value "" у «другое» — ввести текстом.
type GuideChip struct {
	Label string `json:"label"`
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GuideCondition — условие подбора на экране: чип «50 ПК ×».
type GuideCondition struct {
	Key    string `json:"key"`
	Label  string `json:"label"`
	Value  string `json:"value"`
	Origin string `json:"origin"` // user | default | inferred
}

// GuideChange — что изменилось от условия: «Стандартный убран — в нём нет шифрования».
type GuideChange struct {
	Condition string `json:"condition"`
	Effect    string `json:"effect"`
}

// GuideTraceStep — шаг журнала хода («Как подобрано»): только выполненные операции, без рассуждений модели.
type GuideTraceStep struct {
	Step   string  `json:"step"`
	Tool   string  `json:"tool,omitempty"`
	Ms     float64 `json:"ms"`
	Result string  `json:"result,omitempty"`
}

// GuideMeta — телеметрия хода для экрана и отчёта.
type GuideMeta struct {
	TurnID       string  `json:"turn_id"`
	LLMCalls     int     `json:"llm_calls"`
	InputTokens  int     `json:"input_tokens"`
	OutputTokens int     `json:"output_tokens"`
	CostRub      float64 `json:"cost_rub"`
	MsFirst      float64 `json:"ms_first"`
	MsChecked    float64 `json:"ms_checked"`
	MsTotal      float64 `json:"ms_total"`
	Fallback     string  `json:"fallback,omitempty"`
	Removed      int     `json:"removed"`
	// PIIMasked — сколько фрагментов ПДн замаскировано в тексте, ушедшем в модель.
	PIIMasked int `json:"pii_masked"`
}

// GuideSpec — черновик спецификации подбора (POST /api/guide/spec): цена перечитана из каталога.
type GuideSpec struct {
	Version    int             `json:"version"`
	Lines      []GuideSpecLine `json:"lines"`
	TotalRub   *float64        `json:"total_rub"`
	OnRequest  int             `json:"on_request"`
	Unresolved []string        `json:"unresolved"`
	Duplicate  bool            `json:"duplicate"`
}

// GuideSpecLine — строка черновика.
type GuideSpecLine struct {
	OfferID      string   `json:"offer_id"`
	FamilyID     string   `json:"family_id"`
	Title        string   `json:"title"`
	SKU          string   `json:"sku"`
	Tier         string   `json:"tier"`
	Quantity     int      `json:"quantity"`
	PricePerUnit *float64 `json:"price_per_unit"`
	Total        *float64 `json:"total"`
	PriceState   string   `json:"price_state"`
	OfferURL     string   `json:"offer_url"`
	PackagePrice *float64 `json:"package_price,omitempty"`
	PackageSize  int      `json:"package_size,omitempty"`
}

// GuideRoute — POST /api/guide/route: состояние строки (SPEC §1). Offer — текст строки-предложения над выдачей
// ("" при mode=search).
type GuideRoute struct {
	Mode   string `json:"mode"` // search | search_offer | guide
	Reason string `json:"reason"`
	Offer  string `json:"offer"`
	// OpenNow — раскрыть подбор сразу (UI §2); OpenReason — ask | compare | use_limit | "".
	OpenNow    bool   `json:"open_now"`
	OpenReason string `json:"open_reason"`
}

// GuideGapReport — GET /api/guide/gaps: «Вопросы, на которые подбор не ответил» для отчёта SearchOps.
type GuideGapReport struct {
	Title string        `json:"title"`
	Days  int           `json:"days"`
	Cause GuideGapCause `json:"cause"`
	Total int           `json:"total"`
	Rows  []GuideGapRow `json:"rows"`
}

// GuideGapCause — причина диагностики SearchOps для пробелов подбора.
type GuideGapCause struct {
	Code       string `json:"code"`
	Label      string `json:"label"`
	Owner      string `json:"owner"`
	OwnerLabel string `json:"owner_label"`
}

// GuideGapRow — событие пробела и вопрос покупателя (ПДн замаскированы).
type GuideGapRow struct {
	Code     string           `json:"code"`
	Label    string           `json:"label"`
	Question string           `json:"question"`
	Families []GuideGapFamily `json:"families"`
	Count    int              `json:"count"`
}

// GuideGapFamily — семейство, о котором спрашивали.
type GuideGapFamily struct {
	FamilyID string `json:"family_id"`
	Title    string `json:"title"`
}

// GuideRecommendation — рекомендация подбора: оффер и причина («дешевле всех за место среди подходящих: …»).
type GuideRecommendation struct {
	OfferID string `json:"offer_id"`
	Reason  string `json:"reason"`
	// Basis — на чём основана: price_per_seat (каталог) | feature (факт) | named (названа покупателем).
	Basis string `json:"basis"`
}
