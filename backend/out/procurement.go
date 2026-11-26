package out

// Контур доводки, закупка и честное «нет»: поля ответа POST /api/search, необязательные.

// Purchase — извлечённые из запроса параметры закупки: карточка открывается с ними, спецификация и заявка КП
// получают их вместе с исходным запросом.
type Purchase struct {
	Query          string `json:"query"`
	Deal           string `json:"deal,omitempty"` // new | renewal | extension | migration
	DealLabel      string `json:"deal_label,omitempty"`
	Quantity       *int   `json:"quantity,omitempty"`
	Metric         string `json:"metric,omitempty"`
	DurationMonths *int   `json:"duration_months,omitempty"`
	OrgType        string `json:"org_type,omitempty"` // commercial | education | government
	OrgLabel       string `json:"org_label,omitempty"`
	Edition        string `json:"edition,omitempty"`
}

// HelpAnswer — ответ справки: mode answer — вместо товаров, hint — строка рядом с товаром («Как продлить в кабинете»).
type HelpAnswer struct {
	ID        string `json:"id"`
	Mode      string `json:"mode"` // answer | hint
	Title     string `json:"title"`
	Text      string `json:"text"`
	LinkLabel string `json:"link_label"`
	URL       string `json:"url"`
}

// Reestr — статус в реестре российского ПО: confirmed (с номером записи) | not_matching | unknown.
type Reestr struct {
	Status    string `json:"status"`
	Number    string `json:"number,omitempty"`
	Label     string `json:"label"`
	CheckedAt string `json:"checked_at,omitempty"`
}

// AnalogOriginal — «Вы искали: …»: исходный продукт запроса замены, отдельно от аналогов.
type AnalogOriginal struct {
	FamilyID     string   `json:"family_id"`
	FamilyName   string   `json:"family_name"`
	ProductID    string   `json:"product_id"`
	Title        string   `json:"title"`
	Vendor       string   `json:"vendor"`
	Category     string   `json:"category"`
	PriceFromRub *float64 `json:"price_from_rub"`
	Reestr       *Reestr  `json:"reestr"`
}
