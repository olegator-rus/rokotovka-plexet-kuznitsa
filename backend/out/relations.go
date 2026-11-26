package out

import (
	"encoding/json"
	"time"
)

// RelationList — GET /api/relations (контур коммерции): действующий справочник связей.
type RelationList struct {
	// Version — номер действующей версии kind=relations; Materialized=false — версий ещё нет, справочник = каталог
	// (Version — номер, который получит исходная версия при первой правке).
	Version        int        `json:"version"`
	Materialized   bool       `json:"materialized"`
	CatalogVersion string     `json:"catalog_version"`
	StaleAfterDays int        `json:"stale_after_days"`
	Total          int        `json:"total"`
	Items          []Relation `json:"items"`
}

// Relation — связь в форме relations.json плюс подписи концов и пометка «проверить».
type Relation struct {
	Relation    json.RawMessage `json:"-"`
	FromName    string          `json:"from_name"`
	ToName      string          `json:"to_name"`
	NeedsReview bool            `json:"needs_review"`
}

// MarshalJSON — поля связи и подписи одним объектом.
func (r Relation) MarshalJSON() ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(r.Relation, &m); err != nil || m == nil {
		m = map[string]any{}
	}
	m["from_name"], m["to_name"], m["needs_review"] = r.FromName, r.ToName, r.NeedsReview
	return json.Marshal(m)
}

// RelationVersionList — GET /api/relations/versions.
type RelationVersionList struct {
	// Active — действующая версия; 0 — версий нет (справочник = каталог).
	Active int               `json:"active"`
	Items  []RelationVersion `json:"items"`
}

// RelationVersion — версия справочника связей: кто, откуда, разница с предыдущей.
type RelationVersion struct {
	Version        int             `json:"version"`
	CreatedAt      time.Time       `json:"created_at"`
	Author         string          `json:"author"` // system | agent+human | human
	Source         string          `json:"source"` // import | agent | rollback
	Description    string          `json:"description"`
	ProposalID     *string         `json:"proposal_id"`
	Diff           json.RawMessage `json:"diff"` // {"add":[Relation],"remove":[Relation],"update":[{"before","after"}]}
	Relations      int             `json:"relations"`
	CatalogVersion string          `json:"catalog_version"`
	Archived       bool            `json:"archived"`
}
