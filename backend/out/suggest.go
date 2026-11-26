package out

// SuggestResponse — GET /api/suggest: не больше 8 строк суммарно.
type SuggestResponse struct {
	Groups []SuggestGroup `json:"groups"`
}

// SuggestGroup — группа подсказок: query | product | vendor | category.
type SuggestGroup struct {
	Type  string        `json:"type"`
	Label string        `json:"label"`
	Items []SuggestItem `json:"items"`
}

// SuggestItem — строка подсказки. Highlight — совпавшая часть [начало, конец) в символах; nil — совпало
// по n-граммам, без точного участка.
type SuggestItem struct {
	Text      string  `json:"text"`
	ID        *string `json:"id"`
	Highlight []int   `json:"highlight"`
}
