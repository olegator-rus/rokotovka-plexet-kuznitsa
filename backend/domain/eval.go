package domain

import (
	"encoding/json"
	"time"
)

// Оценка качества поиска: наборы запросов с разметкой, прогоны
// конфигураций и их результаты. Метрики считает services/eval*.go.

// Наборы запросов (EvalQuery.Set).
const (
	EvalSetTune       = "tune"       // подбор правил и весов
	EvalSetTest       = "test"       // замороженный: итоговые цифры, на нём ничего не настраивается
	EvalSetRegression = "regression" // проверка правок агента
	EvalSetDemo       = "demo"       // сценарии видео, в метриках test не участвуют
	EvalSetP2         = "p2"         // регрессия контура коммерции: лицензии, парт-номера, характеристики, совместимость, комплекты
	// EvalSetHoldoutWeek — реплей отложенной недели журнала (не из queries.json): поиски последних 7 дней,
	// которых агент не видел, частота = число поисков; разметка — по тексту из queries.json.
	EvalSetHoldoutWeek = "holdout_week"
)

// EvalSets — допустимые наборы.
var EvalSets = []string{EvalSetTune, EvalSetTest, EvalSetRegression, EvalSetDemo, EvalSetP2}

// Типы запросов разметки (EvalQuery.Type).
const (
	QueryExact      = "exact"
	QuerySKU        = "sku"
	QueryTypo       = "typo"
	QueryLayout     = "layout"
	QuerySlang      = "slang"
	QueryPurpose    = "purpose"
	QueryLicense    = "license"
	QueryAnalog     = "analog"
	QueryAbsent     = "absent"
	QueryAliasGap   = "alias_gap"
	QueryNonProduct = "non_product"
	QueryNoise      = "noise"
	QueryRenewal    = "renewal"
)

// EvalQuery — запрос набора с разметкой (queries.json). Judgments — оценки позиций 0–3;
// отсутствие позиции — 0.
type EvalQuery struct {
	ID        string         `json:"id"`
	Text      string         `json:"text"`
	Type      string         `json:"type"`
	Set       string         `json:"set"`
	Freq      int            `json:"freq"`
	ClientID  *string        `json:"client_id"`
	Judgments map[string]int `json:"judgments"`
	Absent    bool           `json:"absent"`
	Cluster   string         `json:"cluster,omitempty"`
	// Expect — ожидания набора p2 (контур коммерции): якорь, обязательные и несовместимые компаньоны, числовые условия.
	Expect json.RawMessage `json:"expect,omitempty" gorm:"-"`
}

// LogEvent — строка исторического журнала поведения (events.jsonl).
type LogEvent struct {
	TS           time.Time `json:"ts"`
	SessionID    string    `json:"session_id"`
	ClientID     *string   `json:"client_id"`
	Type         string    `json:"type"` // search | click | add_to_cart | order | price_request | manager_request
	Query        string    `json:"query,omitempty"`
	ResultsCount *int      `json:"results_count,omitempty"`
	ProductID    string    `json:"product_id,omitempty"`
	Position     *int      `json:"position,omitempty"`
}

// Источники записей журналов (SearchEvent.Source, BehaviorEvent.Source).
const (
	EventSourceLive    = "live"     // пишет работающий поиск и POST /api/events
	EventSourceMockLog = "mock_log" // импорт исторических логов (make import)
	EventSourceCompare = "compare"  // поиск из вкладки «Один запрос»: не входит в аналитику и стоимость
)

// RulesSnapshot — снимок словарей и правил поиска: то, что лежит в config_versions. Песочница агента
// передаёт его в прогон оценки вместо действующей версии.
type RulesSnapshot struct {
	Version int   `json:"version"`
	Rules   JSONB `json:"rules"`
}

// Evaluation — результат прогона одной конфигурации на одном наборе. Строка воспроизводимости —
// SetHash + RulesVersion + CatalogVersion + Commit.
type Evaluation struct {
	UUIDModel
	RunID          string `gorm:"index;size:40"`
	Set            string `gorm:"column:eval_set;size:16;index"`
	SetHash        string `gorm:"size:16"`
	Queries        int
	Config         string `gorm:"size:4;index"`
	RulesVersion   int
	RulesOverride  bool // прогон на снимке правил из песочницы, а не на действующей версии
	CatalogVersion string
	Commit         string `gorm:"size:40"`
	// Metrics — сводные метрики (out.EvalMetrics), ByType — те же по типам запросов.
	Metrics JSONB `gorm:"type:jsonb;not null"`
	ByType  JSONB `gorm:"type:jsonb"`
	// Against — предыдущая конфигурация прогона, с которой сравнивались ухудшения; "" у первой.
	Against string `gorm:"size:4"`
	// Regressions и Improvements — запросы, где конфигурация хуже / лучше предыдущей в прогоне.
	Regressions  JSONB `gorm:"type:jsonb"`
	Improvements JSONB `gorm:"type:jsonb"`
	// PerQuery — метрики каждого запроса (для «Один запрос» и разбора ухудшений).
	PerQuery   JSONB `gorm:"type:jsonb"`
	DurationMs int
}

// JudgeRun — прогон LLM-судьи (ARCHITECTURE §6, «Разметка»): модель другого семейства, чем разбор и агент,
// вслепую оценивает пары запрос–товар из пула топ-10 конфигураций; согласие с разметкой и оценки
// конфигураций по судье.
type JudgeRun struct {
	UUIDModel
	RunID   string `gorm:"index;size:40"`
	Set     string `gorm:"column:eval_set;size:16;index"`
	SetHash string `gorm:"size:16"`
	Model   string
	// PromptVersion — версия промпта судьи (prompts/judge.md + хэш текста).
	PromptVersion string
	Configs       string // через запятую: из чьих топ-10 собран пул
	Pairs         int    // пар в пуле
	Judged        int    // пар, по которым судья ответил
	Calls         int
	CostRub       float64 `gorm:"type:numeric(12,4)"`
	DurationMs    int
	// Agreement — out.JudgeAgreement: точное совпадение, ±1, взвешенная каппа, матрица; ByConfig — NDCG@10
	// конфигураций по разметке и по судье; Grades — оценки пар.
	Agreement JSONB `gorm:"type:jsonb;not null"`
	ByConfig  JSONB `gorm:"type:jsonb"`
	Grades    JSONB `gorm:"type:jsonb"`
}
