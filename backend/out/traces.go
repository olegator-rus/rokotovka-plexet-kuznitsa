package out

import (
	"time"

	"kuznica/api/domain"
)

// TraceView — GET /api/traces/{request_id}: сохранённая трасса поиска и исходный запрос.
type TraceView struct {
	RequestID  string `json:"request_id"`
	Query      string `json:"query"`
	Normalized string `json:"normalized"`
	Config     string `json:"config"`
	Route      string `json:"route"`
	// Source — live (поиск покупателя) | compare (вкладка «Один запрос»); "" — строки журнала поиска нет.
	Source    string       `json:"source"`
	ClientID  *string      `json:"client_id"`
	CreatedAt time.Time    `json:"created_at"`
	Trace     domain.JSONB `json:"trace"`
}
