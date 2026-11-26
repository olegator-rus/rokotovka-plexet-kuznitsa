package domain

// CompanionPair — размеченная пара «якорь → кандидат» для оценки блока допродажи (,
// data/mock/companion_pairs.json). Class — required | compatible | conditional | incompatible | unknown |
// complement | upgrade | irrelevant; Query — запрос, на котором якорь появляется (необязательно).
type CompanionPair struct {
	AnchorProduct    string `json:"anchor_product"`
	CandidateProduct string `json:"candidate_product"`
	Class            string `json:"class"`
	Query            string `json:"query,omitempty"`
}

// CompanionPairClasses — допустимые классы пар.
var CompanionPairClasses = []string{"required", "compatible", "conditional", "incompatible", "unknown", "complement",
	"upgrade", "irrelevant"}
