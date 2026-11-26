package in

import "kuznica/api/domain"

// GuideRequest — POST /api/guide: ход режима подбора. Query — строка поиска, с которой начат подбор;
// Message — реплика покупателя в блоке подбора; Chip — нажатый чип (вопроса или условия); State — состояние подбора
// из прошлого ответа (сервер держит своё; присланное используется, только если сервер его потерял).
type GuideRequest struct {
	SessionID string             `json:"session_id"`
	Query     string             `json:"query"`
	Message   string             `json:"message"`
	Chip      *GuideChip         `json:"chip"`
	State     *domain.GuideState `json:"state"`
	// Condition — правка чипа условия: value null — снять условие (вернуть значение по умолчанию), иначе изменить.
	Condition *GuideCondition `json:"condition"`
}

// GuideChip — чип: key — условие (count, object, operation, term_months, org_type, vendor, feature, use, variant,
// platform) или действие (start_over); value — новое значение; пусто — убрать условие (вернуть значение по умолчанию).
type GuideChip struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// GuideCondition — правка условия подбора с экрана.
type GuideCondition struct {
	Key   string  `json:"key"`
	Value *string `json:"value"`
}

// GuideSpecRequest — POST /api/guide/spec: строка подбора в черновик спецификации (SPEC §4, update_spec).
// Version — версия черновика, которую видел покупатель (0 — первый раз); OpKey — ключ операции: повтор с тем же
// ключом не дублирует строку.
type GuideSpecRequest struct {
	SessionID   string `json:"session_id"`
	SelectionID string `json:"selection_id"`
	Version     int    `json:"version"`
	OfferID     string `json:"offer_id"`
	Quantity    int    `json:"quantity"`
	OpKey       string `json:"op_key"`
}

// GuideRouteRequest — POST /api/guide/route: какое из трёх состояний строки у запроса (SPEC §1).
type GuideRouteRequest struct {
	Query string `json:"query"`
}
