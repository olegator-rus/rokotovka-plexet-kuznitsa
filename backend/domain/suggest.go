package domain

import "time"

// Индекс подсказок контура выдачи: заранее собранные строки с весом, числом
// результатов по текстовой ветке, категориями, вендорами и продолжениями для бейджей. Версии сборок — цепочка
// config_versions с kind = suggestions; строки одной сборки помечены её номером (IndexVersion).

// ConfigKindSuggestions — вид версии config_versions: сборка индекса подсказок.
const ConfigKindSuggestions = "suggestions"

// Источники подсказки.
const (
	SuggestSourceQuery    = "successful_query"
	SuggestSourceTemplate = "catalog_template"
	// SuggestSourceDictionary — форма из словаря понимания запросов («винда», «касперский», «астра линукс»): так
	// пишут покупатели (контур эталонов).
	SuggestSourceDictionary = "dictionary"
)

// Suggestion — строка индекса подсказок.
type Suggestion struct {
	UUIDModel
	// Text — как показывается; Norm — нормализованный текст (ключ префиксного поиска и дублей).
	Text string `gorm:"not null"`
	Norm string `gorm:"not null;index"`
	// Weight — статическая часть веса (частота 25 %, сглаженная успешность 15 %, свежесть 10 %), 0–0,5; соответствие
	// префиксу (50 %) добавляется на запросе.
	Weight float64 `gorm:"not null"`
	// Hits — семейств в текстовой ветке поиска по Text: подсказка с Hits = 0 в индекс не попадает.
	Hits   int    `gorm:"not null"`
	Source string `gorm:"size:24;not null"`
	// Kind — query | category (подсказка-категория оформляется отдельно).
	Kind string `gorm:"size:16;not null;default:query"`
	// Categories, Vendors — id самых частых категорий и вендоров результатов ([]string JSON).
	Categories JSONB `gorm:"type:jsonb"`
	Vendors    JSONB `gorm:"type:jsonb"`
	// NextTokens — продолжения для бейджей: [{"token","hits"}].
	NextTokens JSONB `gorm:"type:jsonb"`
	// Searches, Successes — поисков и успешных поисков в журнале (шаблон каталога — 0).
	Searches  int `gorm:"not null;default:0"`
	Successes int `gorm:"not null;default:0"`
	BuiltAt   time.Time
	// IndexVersion — номер сборки (config_versions.version при kind = suggestions).
	IndexVersion   int    `gorm:"not null;index"`
	CatalogVersion string `gorm:"size:64"`
}
