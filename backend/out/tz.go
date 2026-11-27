package out

import "kuznica/api/domain"

// TZUpload — ответ POST /api/tz (TZ-CORE §2). Status и Reason — только при отказе на проверке до разбора
// (формат, размер): операция уже в состоянии error с той же причиной.
type TZUpload struct {
	OperationID string `json:"operation_id"`
	Status      string `json:"status,omitempty"`
	Reason      string `json:"reason,omitempty"`
}

// TZOperation — GET /api/tz/operations/{id}: processing + stage | completed + hash | error + reason.
type TZOperation struct {
	Status string `json:"status"`
	Stage  string `json:"stage,omitempty"`
	Hash   string `json:"hash,omitempty"`
	Reason string `json:"reason,omitempty"`
}

// TZResult — GET /api/tz/{hash}: позиции документа без товаров.
type TZResult struct {
	Hash           string       `json:"hash"`
	FileName       string       `json:"file_name"`
	ExtractedCount int          `json:"extracted_count"`
	ProcessedCount int          `json:"processed_count"`
	Positions      []TZPosition `json:"positions"`
}

// TZPosition — позиция ТЗ. Purchase — в формате шага закупки (уходит в SpecLine.purchase и QuoteItem.purchase);
// Filters — в формате POST /api/search.
type TZPosition struct {
	ID                     string                `json:"id"`
	Number                 int                   `json:"number"`
	Title                  string                `json:"title"`
	ShortTitle             string                `json:"short_title"`
	Kind                   string                `json:"kind"` // product | service
	Named                  bool                  `json:"named"`
	Attributes             []TZAttribute         `json:"attributes"`
	Purchase               *domain.QuotePurchase `json:"purchase"`
	SearchQuery            string                `json:"search_query"`
	Filters                []TZFilter            `json:"filters"`
	SourceQuote            string                `json:"source_quote"`
	UnverifiedRequirements []string              `json:"unverified_requirements"`
}

// TZAttribute — плашка условия позиции.
type TZAttribute struct {
	Key   string `json:"key"`
	Label string `json:"label"`
	Value string `json:"value"`
}

// TZFilter — фильтр поиска по позиции (ключи services/facets_license.go).
type TZFilter struct {
	Key   string `json:"key"`
	Op    string `json:"op"`
	Value string `json:"value"`
}
