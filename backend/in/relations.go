package in

// RelationsQuery — GET /api/relations?family=&status=&type=&source= (контур коммерции): всё необязательно.
type RelationsQuery struct {
	Family string `query:"family"`
	Status string `query:"status"`
	Type   string `query:"type"`
	Source string `query:"source"`
}

// CommerceEvalQuery — GET /api/evaluations/commerce?set=p2.
type CommerceEvalQuery struct {
	Set string `query:"set"`
}
