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
	// Source — откуда поиск: "" или live — покупатель; compare — вкладка «Один запрос» (в журнал пишется,
	// но в аналитику и стоимость запроса не входит).
	Source string `json:"source"`
	// Filters — фасеты контура коммерции; учитываться начнут с треком оборудования.
	Filters []Filter `json:"filters"`
}

// Filter — условие фасета: {"key": "ram_gb", "op": "gte", "value": 32}; range — value..value2.
type Filter struct {
	Key    string `json:"key"`
	Op     string `json:"op"` // eq | gte | lte | range | not
	Value  any    `json:"value"`
	Value2 any    `json:"value2"`
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
