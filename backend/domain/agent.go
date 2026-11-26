package domain

import (
	"errors"
	"time"

	"github.com/google/uuid"
)

// Агент SearchOps (§4): кластеры неудачных запросов, запуски и
// их шаги, предложения в очередь и решения человека. Решения с отказом — память «не предлагать снова»;
// эффект принятых правок — эпизод запуска (AgentRun.Episode).

// Статусы запуска.
const (
	RunRunning = "running"
	RunDone    = "done"
	RunStopped = "stopped"
	RunFailed  = "failed"
)

// Откуда запуск.
const (
	TriggerManual   = "manual"
	TriggerSchedule = "schedule"
)

// Виды шагов ленты.
const (
	StepToolCall   = "tool_call"
	StepThought    = "thought"
	StepCheck      = "check"
	StepMemory     = "memory"
	StepDiagnosis  = "diagnosis"
	StepHypothesis = "hypothesis"
	StepProposal   = "proposal"
	StepError      = "error"
)

// Диагнозы кластера (ARCHITECTURE §3.2).
const (
	DiagAliasGap           = "ALIAS_GAP"
	DiagTypo               = "TYPO"
	DiagAssortmentGap      = "ASSORTMENT_GAP"
	DiagSubstitute         = "SUBSTITUTE_AVAILABLE"
	DiagContentGap         = "CONTENT_GAP"
	DiagRankingFailure     = "RANKING_FAILURE"
	DiagConstraintConflict = "CONSTRAINT_CONFLICT"
	DiagNonProduct         = "NON_PRODUCT"
	DiagNoise              = "NOISE"
	DiagUnknown            = "UNKNOWN"
	// DiagOnRequestOnly — товар найден, но купить нельзя: у семейства нет позиций с ценой и наличием, все «по
	// запросу» (контур фида, фид заказчика). Итог — задача каталогу (цена и наличие), а не правка поиска.
	DiagOnRequestOnly = "ON_REQUEST_ONLY"
)

// Диагнозы контура коммерции: справочник связей, детерминированные, без данных о покупках.
const (
	DiagBrokenRelation  = "BROKEN_RELATION"
	DiagMissingRequired = "MISSING_REQUIRED"
)

// Diagnoses — все коды диагнозов.
var Diagnoses = []string{DiagAliasGap, DiagTypo, DiagAssortmentGap, DiagSubstitute, DiagContentGap,
	DiagRankingFailure, DiagConstraintConflict, DiagNonProduct, DiagNoise, DiagUnknown,
	DiagBrokenRelation, DiagMissingRequired, DiagOnRequestOnly}

// Режимы запуска агента (AgentRun.Mode): search — кластеры неудачных запросов (контур поиска), commerce — справочник
// связей и правила лицензий (контур коммерции, «Коммерция»).
const (
	RunModeSearch   = "search"
	RunModeCommerce = "commerce"
)

// Статусы предложения.
const (
	ProposalPending    = "pending"
	ProposalAccepted   = "accepted"
	ProposalRejected   = "rejected"
	ProposalRolledBack = "rolled_back"
	// ProposalStale — ждало решения, но каталог сменился (импорт с другим catalog_version): правка проверялась на
	// прежнем словаре, принять её нельзя.
	ProposalStale = "stale"
)

// ProposalStaleReason — decided_reason предложений, устаревших при смене каталога.
const ProposalStaleReason = "catalog_changed"

// ProposalStalePreHoldout — decided_reason предложений из запусков до разреза журнала (ревью-2 N2): запуск видел
// отложенную неделю и тексты test, правка подобрана под них. Ставит человек командой `proposals stale`.
const ProposalStalePreHoldout = "pre_holdout"

// Кому адресовано предложение.
const (
	RoleEcom    = "ecom"
	RoleProduct = "product"
	RoleContent = "content"
)

// Действие предложения: правка правил поиска или задача человеку.
const (
	ActionRulesPatch = "rules_patch"
	ActionTicket     = "ticket"
	// ActionRelationPatch — правка справочника связей (контур коммерции): принятие создаёт версию kind=relations.
	ActionRelationPatch = "relation_patch"
)

// Статусы предложения контура коммерции: техническая корректность и экономический эффект раздельно.
const (
	TechVerified   = "verified"
	TechUnverified = "unverified"
	TechFailed     = "failed"
	EconUnknown    = "unknown"
	EconEstimated  = "estimated"
	EconMeasured   = "measured"
)

// Причины отказа человека.
const (
	RejectTooBroad     = "too_broad"
	RejectWrongProduct = "wrong_product"
	RejectNotNow       = "not_now"
	RejectOther        = "other"
)

// RejectReasons — допустимые причины отказа.
var RejectReasons = []string{RejectTooBroad, RejectWrongProduct, RejectNotNow, RejectOther}

// Ошибки гонок: решение по уже решённому предложению и правка правил поверх чужой версии.
var (
	ErrProposalDecided       = errors.New("предложение уже решено")
	ErrRulesVersionConflict  = errors.New("версия правил изменилась")
	ErrRulesVersionNotExists = errors.New("версии правил нет")
)

// IssueCluster — кластер неудачных запросов одного запуска.
type IssueCluster struct {
	UUIDModel
	RunID uuid.UUID `gorm:"type:uuid;index"`
	// Key — устойчивый ключ кластера между запусками (термины или главный запрос): по нему ищется память.
	Key   string `gorm:"index"`
	Label string
	// Terms — непонятные поиску слова кластера (кандидаты в алиасы).
	Terms JSONB `gorm:"type:jsonb"`
	// Queries — [{text, freq, failures, empty, reformulated, no_click, reformulated_to}].
	Queries    JSONB `gorm:"type:jsonb"`
	Freq       int
	Failures   int
	LostDemand float64 `gorm:"type:numeric(12,2)"`
	// Current — что отвечает действующий поиск на главные запросы кластера.
	Current   JSONB  `gorm:"type:jsonb"`
	Rank      int    // место по потерянному спросу
	Status    string `gorm:"size:16"` // new | investigated | skipped
	Diagnosis string `gorm:"size:32"`
	Note      string
}

// AgentRun — запуск агента.
type AgentRun struct {
	UUIDModel
	Status           string `gorm:"size:16;index"`
	Trigger          string `gorm:"size:16"`
	Days             int
	MaxClusters      int
	StartedAt        time.Time
	FinishedAt       *time.Time
	ClustersTotal    int
	ClustersDone     int
	ProposalsCreated int
	ToolCalls        int
	InputTokens      int
	OutputTokens     int
	CostRub          float64 `gorm:"type:numeric(12,4)"`
	StopReason       string
	Summary          string
	CurrentClusterID *string
	CurrentLabel     string
	Model            string
	PromptVersion    string
	RulesVersion     int
	// Mode — search | commerce (контур коммерции); пусто у запусков контура поиска = search.
	Mode string `gorm:"size:16"`
	// Episode — эпизод запуска: метрики, эффект принятых правок (verify_applied_change), итоги кластеров.
	Episode JSONB `gorm:"type:jsonb"`
	// Контур автономии: запрос запуска «Разобрать» (пусто — по периоду), режим «только чтение», вход триггера, план
	// (последняя редакция) и бюджет (лимиты и расход).
	Query    string
	ReadOnly bool
	Input    JSONB `gorm:"type:jsonb"`
	Plan     JSONB `gorm:"type:jsonb"`
	Budget   JSONB `gorm:"type:jsonb"`
}

// AgentStep — шаг ленты запуска.
type AgentStep struct {
	UUIDModel
	RunID          uuid.UUID `gorm:"type:uuid;index:idx_agent_steps_run_seq,priority:1"`
	Seq            int       `gorm:"index:idx_agent_steps_run_seq,priority:2"`
	Kind           string    `gorm:"size:16"`
	ClusterID      *string
	Tool           *string
	Title          string
	Args           JSONB `gorm:"type:jsonb"`
	Result         JSONB `gorm:"type:jsonb"`
	OK             bool
	HypothesisNo   *int
	RejectedReason *string
	Ms             int
}

// Proposal — предложение агента в очередь человеку.
type Proposal struct {
	UUIDModel
	RunID          uuid.UUID `gorm:"type:uuid;index"`
	ClusterID      *string
	ClusterKey     string `gorm:"index"`
	ClusterLabel   string
	ClusterQueries JSONB   `gorm:"type:jsonb"`
	LostDemand     float64 `gorm:"type:numeric(12,2)"`
	Diagnosis      string  `gorm:"size:32"`
	DiagnosisText  string
	Role           string `gorm:"size:16;index"`
	Action         string `gorm:"size:16"`
	// Patch — {"add":[…],"remove":[…]}; PatchKeys — нормализованные «алиас→цель» для памяти.
	Patch     JSONB `gorm:"type:jsonb"`
	PatchKeys JSONB `gorm:"type:jsonb"`
	// Evaluation — прогон в песочнице; Evidence — что агент нашёл (аналоги, переформулировки).
	Evaluation      JSONB `gorm:"type:jsonb"`
	Evidence        JSONB `gorm:"type:jsonb"`
	HypothesesTried int
	// BaseVersion — версия правил, на которой проверялась правка.
	BaseVersion    int
	Status         string `gorm:"size:16;index"`
	DecidedAt      *time.Time
	DecidedReason  string
	DecidedComment string
	ConfigVersion  *int
	// Effect — эффект после принятия (verify_applied_change следующего запуска).
	Effect JSONB `gorm:"type:jsonb"`
	// Контур коммерции: уровень доказательности E0–E5, техническая корректность (verified|unverified|failed),
	// экономический эффект (unknown|estimated|measured). Оценка приоритета по покупкам (priority_score) удалена в
	// контуре доводки — колонку снимает repositories.migrateP4Removed.
	EvidenceLevel   string `gorm:"size:4"`
	TechnicalStatus string `gorm:"size:16"`
	EconomicStatus  string `gorm:"size:16"`
}

// Decision — решение человека по предложению. Отказы — память агента «не предлагать снова».
type Decision struct {
	UUIDModel
	ProposalID   uuid.UUID `gorm:"type:uuid;index"`
	Decision     string    `gorm:"size:16;index"` // accepted | rejected | rolled_back
	Reason       string
	Comment      string
	ClusterKey   string `gorm:"index"`
	ClusterLabel string
	Diagnosis    string
	Terms        JSONB `gorm:"type:jsonb"`
	PatchKeys    JSONB `gorm:"type:jsonb"`
	// DecidedBy — роль, принявшая решение (контур автономии, «кто» во вкладке «Решения»); пусто — до контура автономии.
	DecidedBy string `gorm:"size:16"`
}
