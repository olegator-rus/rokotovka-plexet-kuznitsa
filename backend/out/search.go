package out

import (
	"time"

	"kuznica/api/domain"
)

// Ответы поиска и витрины. Поля correction, clarify и empty
// отсутствуют, когда им нечего сказать; массивы — всегда массивы.

// SearchResponse — POST /api/search.
type SearchResponse struct {
	RequestID      string       `json:"request_id"`
	Query          string       `json:"query"`
	Normalized     string       `json:"normalized"`
	Config         string       `json:"config"`
	Correction     *Correction  `json:"correction,omitempty"`
	Chips          []Chip       `json:"chips"`
	Clarify        *Clarify     `json:"clarify,omitempty"`
	Total          int          `json:"total"`
	Items          []SearchItem `json:"items"`
	Relaxed        []Relaxed    `json:"relaxed"`
	Empty          *Empty       `json:"empty,omitempty"`
	RelatedQueries []string     `json:"related_queries"`
	// Complements — «С этим покупают»: семейства, связанные с первой позицией выдачи (relations complement),
	// до 3; пустой массив — связей нет или конфигурация без справочников (B0–B2).
	Complements []Complement  `json:"complements"`
	Trace       *domain.Trace `json:"trace"`
}

// Complement — сопутствующий товар: самая популярная позиция связанного семейства.
type Complement struct {
	FamilyID       string   `json:"family_id"`
	FamilyName     string   `json:"family_name"`
	ProductID      string   `json:"product_id"`
	Title          string   `json:"title"`
	Vendor         string   `json:"vendor"`
	Category       string   `json:"category"`
	LicenseLine    string   `json:"license_line"`
	PriceRub       *float64 `json:"price_rub"`
	PriceOnRequest bool     `json:"price_on_request"`
	// Note — почему покупают вместе (пояснение связи из справочника).
	Note string `json:"note"`
}

// Correction — исправление запроса.
type Correction struct {
	From       string `json:"from"`
	To         string `json:"to"`
	Confidence string `json:"confidence"` // high | medium
	Applied    bool   `json:"applied"`
}

// Chip — понятое условие запроса.
type Chip struct {
	Field  string `json:"field"`
	Value  string `json:"value"`
	Label  string `json:"label"`
	Strict bool   `json:"strict"`
}

// Clarify — уточняющий вопрос.
type Clarify struct {
	Attribute string          `json:"attribute"`
	Question  string          `json:"question"`
	Options   []ClarifyOption `json:"options"`
}

// ClarifyOption — вариант ответа на уточнение.
type ClarifyOption struct {
	Value string `json:"value"`
	Label string `json:"label"`
}

// SearchItem — позиция выдачи: лучшая лицензия семейства, остальные — в Variants.
type SearchItem struct {
	ProductID       string       `json:"product_id"`
	FamilyID        string       `json:"family_id"`
	Title           string       `json:"title"`
	Vendor          string       `json:"vendor"`
	Category        string       `json:"category"`
	LicenseLine     string       `json:"license_line"`
	PriceRub        *float64     `json:"price_rub"`
	PriceOnRequest  bool         `json:"price_on_request"`
	Group           string       `json:"group"` // exact | match | analog
	MatchesQuery    bool         `json:"matches_query"`
	PurchasedBefore bool         `json:"purchased_before"`
	Final           float64      `json:"final"`
	Variants        []SearchItem `json:"variants,omitempty"`
}

// Relaxed — ослабленное условие и объяснение покупателю.
type Relaxed struct {
	Field   string `json:"field"`
	Message string `json:"message"`
}

// Empty — точного товара нет.
type Empty struct {
	Message     string `json:"message"`
	ShowManager bool   `json:"show_manager"`
}

// Meta — GET /api/meta.
type Meta struct {
	LLMMode        string       `json:"llm_mode"`
	CatalogVersion string       `json:"catalog_version"`
	RulesVersion   int          `json:"rules_version"`
	Configs        []string     `json:"configs"`
	DefaultConfig  string       `json:"default_config"`
	Clients        []MetaClient `json:"clients"`
	// LLM — режим модели, дневной бюджет и плашка режима для шапки.
	LLM LLMState `json:"llm"`
	// AdminEnabled — доступны ли служебные методы /api/admin/* (ADMIN_ENABLED).
	AdminEnabled bool `json:"admin_enabled"`
	// RateLimitPerMin — лимит частоты запросов к /api с одного адреса; 0 — без лимита.
	RateLimitPerMin int `json:"rate_limit_per_min"`
	// Session — подписанный сервером токен сессии покупателя (handlers/session.go): фронт шлёт его в session_id
	// поиска и событий; самодельные сессии в журнал не попадают.
	Session string `json:"session"`
}

// LLMState — режим модели и бюджет (GET /api/meta → llm, POST /api/admin/llm-mode).
type LLMState struct {
	// Mode — on | off: идут ли сложные запросы (маршрут D) в модель. Переключается без перезапуска.
	Mode string `json:"mode"`
	// Online — модель настроена (ключ задан); без неё режим on недоступен.
	Online    bool       `json:"online"`
	ChangedAt *time.Time `json:"changed_at"`
	Budget    LLMBudget  `json:"budget"`
	// Banner — текст плашки в шапке: режим без LLM или исчерпан бюджет; null — плашка не нужна.
	Banner *string `json:"banner"`
}

// LLMBudget — дневной бюджет на модель (LLM_DAILY_BUDGET_RUB), сутки по Москве.
type LLMBudget struct {
	Day string `json:"day"`
	// DailyRub — потолок, ₽; null — без потолка.
	DailyRub      *float64 `json:"daily_rub"`
	SpentTodayRub float64  `json:"spent_today_rub"`
	Exceeded      bool     `json:"exceeded"`
}

// MetaClient — покупатель для переключателя «от чьего имени ищем».
type MetaClient struct {
	ID      string `json:"id"`
	Name    string `json:"name"`
	Segment string `json:"segment"`
	Hint    string `json:"hint"`
}

// ImportResult — POST /api/catalog/import и `go run . import`: что загружено и сколько это заняло.
type ImportResult struct {
	Source         string   `json:"source"`
	CatalogVersion string   `json:"catalog_version"`
	Products       int      `json:"products"`
	Families       int      `json:"families"`
	Vendors        int      `json:"vendors"`
	Categories     int      `json:"categories"`
	Relations      int      `json:"relations"`
	Aliases        int      `json:"aliases"`
	Clients        int      `json:"clients"`
	Index          string   `json:"index"`
	Vectors        bool     `json:"vectors"`
	Database       bool     `json:"database"` // записан ли каталог в PostgreSQL
	RulesVersion   int      `json:"rules_version"`
	EmbedMs        int64    `json:"embed_ms"`
	IndexMs        int64    `json:"index_ms"`
	DatabaseMs     int64    `json:"database_ms"`
	TotalMs        int64    `json:"total_ms"`
	Warnings       []string `json:"warnings"`
}
