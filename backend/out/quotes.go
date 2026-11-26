package out

import (
	"time"

	"kuznica/api/domain"
)

// QuoteCreated — ответ POST /api/quotes (201).
type QuoteCreated struct {
	ID        string   `json:"id"`
	Status    string   `json:"status"`
	Positions int      `json:"positions"`
	TotalRub  *float64 `json:"total_rub"` // только если известны все цены, иначе null
	OnRequest int      `json:"on_request"`
}

// Quote — запрос КП в списке бэк-офиса (GET /api/quotes).
type Quote struct {
	QuoteCreated
	CreatedAt time.Time   `json:"created_at"`
	SessionID *string     `json:"session_id"`
	ClientID  *string     `json:"client_id"`
	Comment   string      `json:"comment"`
	Items     []QuoteLine `json:"items"`
}

// QuoteLine — позиция запроса КП: что заказано и по какой цене (null — по запросу).
type QuoteLine struct {
	ProductID      string   `json:"product_id"`
	Title          string   `json:"title"`
	Quantity       int      `json:"quantity"`
	Origin         string   `json:"origin"`
	CompanionClass string   `json:"companion_class,omitempty"`
	PriceRub       *float64 `json:"price_rub"`
	PriceMode      string   `json:"price_mode"`
	// Purchase — исходный запрос и параметры закупки (контур доводки).
	Purchase *domain.QuotePurchase `json:"purchase,omitempty"`
}

// QuoteList — GET /api/quotes.
type QuoteList struct {
	Items []Quote `json:"items"`
}
