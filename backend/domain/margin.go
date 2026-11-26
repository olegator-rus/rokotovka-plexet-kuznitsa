package domain

// Политика маржи (контур коммерции): вес маржи в бизнес-слое — версия config_versions kind=margin.

// StepMargin — шаг трассы «маржа»: что политика сделала с порядком выдачи. Только в трассе бэк-офиса
// (GET /api/traces/{id}); в ответ витрины шаг не попадает.
const StepMargin = "margin"

// Защиты политики маржи (MarginStep.Guards, simulate guards_fired).
const (
	// MarginGuardExactQuery — точный запрос (артикул, парт-номер, точное название): сдвиг по марже отключён.
	MarginGuardExactQuery = "exact_query"
	// MarginGuardExactTop1 — первая позиция точной группы (названное семейство) защищена.
	MarginGuardExactTop1 = "exact_top1"
	// MarginGuardGroupLock — переход между группами релевантности (группа, ступень, условия) запрещён.
	MarginGuardGroupLock = "group_lock"
	// MarginGuardMaxShift — сдвиг не больше двух позиций.
	MarginGuardMaxShift = "max_shift"
	// MarginGuardEpsilon — потеря NDCG@10 размеченного запроса больше ε: дорогие перестановки откачены.
	MarginGuardEpsilon = "epsilon"
	// MarginNoteUnknown — у позиции нет маржи: нейтральное значение (не защита, пометка).
	MarginNoteUnknown = "unknown_margin_neutral"
	// MarginNoteNegative — отрицательная маржа: мягкое понижение, без скрытия.
	MarginNoteNegative = "negative_margin_soft"
)

// MarginShift — позиция, которую политика маржи передвинула: from → to (1-based, по семействам выдачи).
type MarginShift struct {
	ProductID string `json:"product_id"`
	From      int    `json:"from"`
	To        int    `json:"to"`
	// Margin — нормированная маржа позиции (перцентиль в категории, 0–1; нет маржи — 0.5) — только бэк-офис.
	Margin float64 `json:"margin"`
}

// MarginStep — шаг трассы «маржа».
type MarginStep struct {
	StepBase
	PolicyVersion int     `json:"policy_version"`
	Weight        float64 `json:"weight"`
	// BaseWeight — вес контура поиска (0.05): при weight = base выдача та же, что в контуре поиска.
	BaseWeight float64 `json:"base_weight"`
	// CategoryWeights — веса по категориям поверх Weight; нет — не пишется.
	CategoryWeights map[string]float64 `json:"category_weights,omitempty"`
	Shifts          []MarginShift      `json:"shifts"`
	// Guards — защиты, которые сработали (не дали сдвинуть), и пометки (unknown/negative).
	Guards []string `json:"guards"`
	// Error — политика не прочитана: выдача по весу контура поиска, поиск не сломан.
	Error string `json:"error,omitempty"`
}

// MarginPolicyRules — содержимое версии политики маржи (config_versions.rules при kind=margin).
type MarginPolicyRules struct {
	Weight float64 `json:"weight"`
	// Categories — вес по категории позиции поверх Weight (MARGIN_OPPORTUNITY агента); нет — общий вес.
	Categories map[string]float64 `json:"categories,omitempty"`
}

// MARGIN_OPPORTUNITY агента «Коммерция» (контур коммерции): сдвиг рычага в категории без потери
// качества. Предложение — действие margin_policy; применяется на экране «Маржа» (POST /api/margin/policy с
// proposal_id), из очереди не принимается.
const (
	DiagMarginOpportunity = "MARGIN_OPPORTUNITY"
	ActionMarginPolicy    = "margin_policy"
)
