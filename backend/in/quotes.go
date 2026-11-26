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
}
