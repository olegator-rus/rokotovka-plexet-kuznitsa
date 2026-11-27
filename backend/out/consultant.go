package out

// Консультант (контур консультанта): поток событий NDJSON по CONTRACT.md. Цены — целые копейки; нет цены — поля нет.

// ConsultantEvent — одно событие потока (ConversationEvent).
type ConsultantEvent struct {
	Type         string   `json:"type"` // status | message.start | message.delta | message.attachment | message.complete | message.error
	Step         string   `json:"step,omitempty"`
	ID           string   `json:"id,omitempty"`
	Text         string   `json:"text,omitempty"`
	Attachment   any      `json:"attachment,omitempty"`
	QuickReplies []string `json:"quickReplies,omitempty"`
	Reason       string   `json:"reason,omitempty"`
}

// ProductReference — позиция во вложении.
type ProductReference struct {
	OfferID           string     `json:"offerId"`
	SKU               string     `json:"sku"`
	Title             string     `json:"title"`
	Vendor            string     `json:"vendor"`
	LicenseType       string     `json:"licenseType"`
	Quantity          int        `json:"quantity,omitempty"`
	Duration          string     `json:"duration,omitempty"`
	UnitPriceMinor    *int64     `json:"unitPriceMinor,omitempty"`
	TotalPriceMinor   *int64     `json:"totalPriceMinor,omitempty"`
	OldUnitPriceMinor *int64     `json:"oldUnitPriceMinor,omitempty"`
	PriceStatus       string     `json:"priceStatus"`   // known | on_request | unknown
	Compatibility     string     `json:"compatibility"` // compatible | incompatible | unverified
	Reason            string     `json:"reason,omitempty"`
	SourceIDs         []string   `json:"sourceIds,omitempty"`
	ProductURL        string     `json:"productUrl"`
	ImageURL          string     `json:"imageUrl,omitempty"`
	Offer             GuideOffer `json:"offer"`
}

// ConsultantCard — вложение product_card.
type ConsultantCard struct {
	Type    string           `json:"type"`
	Product ProductReference `json:"product"`
}

// ConsultantListGroup — группа списка: fit | more | unfit.
type ConsultantListGroup struct {
	Kind  string             `json:"kind"`
	Title string             `json:"title,omitempty"`
	Items []ProductReference `json:"items"`
}

// ConsultantList — вложение product_list.
type ConsultantList struct {
	Type   string                `json:"type"`
	Title  string                `json:"title,omitempty"`
	Groups []ConsultantListGroup `json:"groups"`
}

// ConsultantCompareRow — строка сравнения: values '✓' | '—' | короткий текст.
type ConsultantCompareRow struct {
	Label  string   `json:"label"`
	Values []string `json:"values"`
}

// ConsultantComparison — вложение comparison; строку стоимости фронт строит из columns[].totalPriceMinor.
type ConsultantComparison struct {
	Type    string                 `json:"type"`
	Columns []ProductReference     `json:"columns"`
	Rows    []ConsultantCompareRow `json:"rows"`
}

// ConsultantSpecEvent — вложение spec_event: фронт применяет его к стору спецификации.
type ConsultantSpecEvent struct {
	Type            string      `json:"type"`
	Action          string      `json:"action"` // added | removed | updated
	OfferID         string      `json:"offerId"`
	Title           string      `json:"title"`
	Quantity        int         `json:"quantity,omitempty"`
	TotalPriceMinor *int64      `json:"totalPriceMinor,omitempty"`
	Offer           *GuideOffer `json:"offer,omitempty"`
}

// ConsultantSpecItem — строка состава спецификации.
type ConsultantSpecItem struct {
	OfferID         string `json:"offerId"`
	Title           string `json:"title"`
	Quantity        int    `json:"quantity"`
	TotalPriceMinor *int64 `json:"totalPriceMinor,omitempty"`
}

// ConsultantSpecSummary — вложение spec_summary.
type ConsultantSpecSummary struct {
	Type       string               `json:"type"`
	Items      []ConsultantSpecItem `json:"items"`
	TotalMinor int64                `json:"totalMinor"`
	Count      int                  `json:"count"`
}

// ConsultantLink — вложение link (сервисная тема).
type ConsultantLink struct {
	Type  string `json:"type"`
	Topic string `json:"topic"`
	Title string `json:"title"`
	URL   string `json:"url"`
}

// ConsultantSource — вложение source.
type ConsultantSource struct {
	Type  string `json:"type"`
	Title string `json:"title"`
	URL   string `json:"url,omitempty"`
	Quote string `json:"quote,omitempty"`
}

// ConsultantEstimateLine — строка сметы: known — посчитано; not_calculated — нет.
type ConsultantEstimateLine struct {
	Label           string `json:"label"`
	TotalPriceMinor *int64 `json:"totalPriceMinor,omitempty"`
	Status          string `json:"status"` // known | not_calculated
}

// ConsultantEstimate — вложение estimate: смета комплекта; полной суммы нет, пока есть непосчитанные строки.
type ConsultantEstimate struct {
	Type            string                   `json:"type"`
	Lines           []ConsultantEstimateLine `json:"lines"`
	KnownTotalMinor int64                    `json:"knownTotalMinor"`
	Note            string                   `json:"note"`
}
