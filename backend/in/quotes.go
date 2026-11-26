package in

// QuoteRequest — POST /api/quotes: спецификация и запрос КП.
type QuoteRequest struct {
	SessionID string      `json:"session_id"`
	ClientID  string      `json:"client_id"`
	Items     []QuoteItem `json:"items"`
	Comment   string      `json:"comment"`
}

// QuoteItem — позиция спецификации; Origin — search | companion | template.
type QuoteItem struct {
	ProductID      string `json:"product_id"`
	Quantity       int    `json:"quantity"`
	Origin         string `json:"origin"`
	CompanionClass string `json:"companion_class"`
	// Purchase — параметры закупки из запроса, с которого позиция пришла в спецификацию (контур доводки, §12.6).
	Purchase *QuotePurchase `json:"purchase"`
}

// QuotePurchase — исходный запрос и извлечённые параметры: тип покупки, количество, срок, организация, редакция.
type QuotePurchase struct {
	Query          string `json:"query"`
	Deal           string `json:"deal"`
	Quantity       *int   `json:"quantity"`
	Metric         string `json:"metric"`
	DurationMonths *int   `json:"duration_months"`
	OrgType        string `json:"org_type"`
	Edition        string `json:"edition"`
}
