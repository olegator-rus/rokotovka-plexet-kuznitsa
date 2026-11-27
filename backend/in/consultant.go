package in

// ConsultantRequest — POST /api/consultant: реплика покупателя консультанту.
// Spec — текущее содержимое спецификации магазина (источник правды — фронт).
type ConsultantRequest struct {
	SessionID      string               `json:"session_id"`
	ConversationID string               `json:"conversation_id"`
	Text           string               `json:"text"`
	Page           *ConsultantPage      `json:"page"`
	Spec           []ConsultantSpecLine `json:"spec"`
}

// ConsultantPage — страница, с которой пишет покупатель: home | search | family | product | spec | other.
type ConsultantPage struct {
	Type     string  `json:"type"`
	FamilyID *string `json:"family_id"`
	OfferID  *string `json:"offer_id"`
	Query    *string `json:"query"`
}

// ConsultantSpecLine — строка спецификации магазина.
type ConsultantSpecLine struct {
	OfferID string `json:"offer_id"`
	Qty     int    `json:"qty"`
}
