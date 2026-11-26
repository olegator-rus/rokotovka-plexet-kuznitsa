package in

import "encoding/json"

// SearchRequest — POST /api/search. removed_chips, clarify и as_typed
// принимаются по контракту; учитываться начнут вместе с атрибутами и исправлениями.
type SearchRequest struct {
	Query        string            `json:"query"`
	ClientID     string            `json:"client_id"`
	SessionID    string            `json:"session_id"`
	Config       string            `json:"config"`
	AsTyped      bool              `json:"as_typed"`
	RemovedChips []RemovedChip     `json:"removed_chips"`
	Clarify      map[string]string `json:"clarify"`
	Limit        int               `json:"limit"`
}

// RemovedChip — условие, которое покупатель снял с запроса.
type RemovedChip struct {
	Field string `json:"field"`
	Value string `json:"value"`
}

// EventRequest — POST /api/events.
type EventRequest struct {
	Type      string          `json:"type"`
	RequestID string          `json:"request_id"`
	SessionID string          `json:"session_id"`
	ClientID  string          `json:"client_id"`
	ProductID string          `json:"product_id"`
	Position  *int            `json:"position"`
	Payload   json.RawMessage `json:"payload"`
}
