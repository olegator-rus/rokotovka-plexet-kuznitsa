package domain

import "time"

// Режим подбора (контур подбора): состояние подбора и журнал хода.

// GuideState — состояние подбора, отдельно от истории сообщений (SPEC §2). Сервер держит своё по SelectionID;
// присланное покупателем берётся, только если сервер его потерял (перезапуск), и сверяется с каталогом.
type GuideState struct {
	SelectionID  string            `json:"selection_id"`
	Intent       string            `json:"intent"` // license_selection | comparison
	Requirements GuideRequirements `json:"requirements"`
	// Origin — происхождение условия: user (ввод покупателя или чип) | default | inferred (выведено из запроса).
	Origin             map[string]string `json:"origin"`
	CandidateFamilyIDs []string          `json:"candidate_family_ids"`
	SelectedOfferIDs   []string          `json:"selected_offer_ids"`
	FactIDs            []string          `json:"fact_ids"`
	Unresolved         []string          `json:"unresolved"`
	FeedVersion        string            `json:"feed_version"`
	KBVersion          string            `json:"kb_version"`
	// Turn — номер хода; Questions — сколько вопросов задано; Skipped — сколько пропущено; Pending — условие,
	// о котором задан последний вопрос ("" — вопроса нет).
	Turn      int    `json:"turn"`
	Questions int    `json:"questions"`
	Skipped   int    `json:"skipped"`
	Pending   string `json:"pending"`
	// QuoteNotes — условия покупателя для запроса КП («нужен именно Windows, там 1С и домен»; вопрос без факта).
	QuoteNotes []string `json:"quote_notes"`
}

// GuideRequirements — условия покупателя (SPEC §2).
type GuideRequirements struct {
	Task             string        `json:"task"`
	Vendor           string        `json:"vendor"`
	Objects          []GuideObject `json:"objects"`
	Platform         string        `json:"platform"`
	Operation        string        `json:"operation"` // new | renewal
	TermMonths       int           `json:"term_months"`
	OrgType          string        `json:"org_type"` // commercial | government | academic | nonprofit
	RequiredFeatures []string      `json:"required_features"`
	Use              string        `json:"use"`     // tls_server | ""
	Variant          string        `json:"variant"` // вариант внутри семейства (уровень Astra)
	Cheapest         bool          `json:"cheapest"`
}

// GuideObject — что лицензируем и сколько: workstation | server | user | connection | device.
type GuideObject struct {
	Kind  string `json:"kind"`
	Count int    `json:"count"`
}

// GuideTurn — журнал хода подбора (SPEC §13): что выполнено, сколько стоило, сколько длилось. Текст реплик не
// пишется.
type GuideTurn struct {
	UUIDModel
	SelectionID   string  `gorm:"index;size:64"`
	SessionID     *string `gorm:"index;size:128"`
	Turn          int
	Type          string `gorm:"size:24"`
	Tools         JSONB  `gorm:"type:jsonb"` // [{step, tool, ms, result}]
	LLMCalls      int
	InputTokens   int
	OutputTokens  int
	CostRub       float64 `gorm:"type:numeric(12,4)"`
	MsFirst       int     // первые офферы собраны
	MsChecked     int     // проверки пройдены
	MsTotal       int
	Removed       JSONB  `gorm:"type:jsonb"` // удалённые проверкой утверждения
	Gaps          JSONB  `gorm:"type:jsonb"` // события пробелов
	GapItems      JSONB  `gorm:"type:jsonb"` // пробелы с вопросом и семействами — отчёт SearchOps
	Fallback      string `gorm:"size:32"`    // причина ответа без модели
	CatalogVer    string `gorm:"size:64"`
	KBVersion     string `gorm:"size:64"`
	TurnStartedAt time.Time
}
