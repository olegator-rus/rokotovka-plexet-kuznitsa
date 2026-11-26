package domain

import (
	"time"

	"github.com/google/uuid"
)

// Контур автономии: триггеры запусков, план, бюджет и три слоя памяти агента.
// Все записи создают только настоящие запуски агента и решения человека (README «Главное правило»).

// Откуда запуск — контур автономии (TriggerManual и TriggerSchedule — в agent.go).
const (
	TriggerSpike         = "spike"
	TriggerCatalogChange = "catalog_change"
)

// Triggers — допустимые триггеры запуска.
var Triggers = []string{TriggerManual, TriggerSchedule, TriggerSpike, TriggerCatalogChange}

// Виды шагов контура автономии.
const (
	StepPlan        = "plan"
	StepMemoryRead  = "memory_read"
	StepMemorySkip  = "memory_skip"
	StepEffectCheck = "effect_check"
	StepBudget      = "budget"
)

// ActionRollback — предложение отката принятой правки, которое агент составил сам (rollback_proposed_by_agent):
// патч — обратная правка; принятие идёт тем же механизмом версий правил, исходное предложение → rolled_back.
const ActionRollback = "rollback"

// Эффект принятой правки (проверка в начале ночного запуска).
const (
	EffectHelped   = "helped"
	EffectNoEffect = "no_effect"
	EffectHarmed   = "harmed"
)

// Статусы записи «не предлагать».
const (
	SuppressActive  = "active"  // условия возврата не наступили — кластер пропускается
	SuppressLifted  = "lifted"  // условие наступило — агент предложил снова
	SuppressRemoved = "removed" // человек снял запись с причиной (это тоже решение)
)

// Охват записи «не предлагать».
const (
	SuppressScopeCluster = "cluster" // кластер не разбирается (отказ не из-за ширины правки)
	SuppressScopePatch   = "patch"   // «слишком широко»: кластер разбирается, эта правка не повторяется
)

// DecisionSuppressRemoved — решение человека снять запись «не предлагать».
const DecisionSuppressRemoved = "suppress_removed"

// MemorySuppress — слой «не предлагать»: что отклонено, почему и при каком условии агент вернётся.
type MemorySuppress struct {
	UUIDModel
	ClusterKey   string `gorm:"index"`
	ClusterLabel string
	Terms        JSONB  `gorm:"type:jsonb"`
	PatchKeys    JSONB  `gorm:"type:jsonb"`
	Diagnosis    string `gorm:"size:32"`
	Scope        string `gorm:"size:16"`
	// Откуда: предложение, его запуск и решение человека.
	ProposalID  uuid.UUID `gorm:"type:uuid;index"`
	SourceRunID uuid.UUID `gorm:"type:uuid"`
	Reason      string
	Comment     string
	DecidedAt   time.Time
	// Условие возврата: неудачных поисков кластера стало не меньше ReturnFailures (за то же окно, что при отказе)
	// или сменилась версия каталога (новые доказательства). FailuresAt — значение при отказе.
	FailuresAt     int
	ReturnFailures int
	CatalogAt      string `gorm:"size:64"`
	// Текущее значение условия — последней проверкой запуска.
	CurrentFailures int
	CheckedAt       *time.Time
	CheckedRunID    *uuid.UUID `gorm:"type:uuid"`
	Status          string     `gorm:"size:16;index"`
	LiftedRunID     *uuid.UUID `gorm:"type:uuid"`
	LiftedWhat      string
	RemovedReason   string
	RemovedBy       string `gorm:"size:16"`
	RemovedAt       *time.Time
}

// MemoryEpisode — слой «эпизоды»: что разобрано, чем кончилось и (после проверки) эффект.
type MemoryEpisode struct {
	UUIDModel
	RunID        uuid.UUID `gorm:"type:uuid;index"`
	Trigger      string    `gorm:"size:16"`
	ClusterKey   string    `gorm:"index"`
	ClusterLabel string
	// Kind — investigation (разбор кластера) | effect_check (проверка принятой правки).
	Kind       string     `gorm:"size:16"`
	Outcome    string     `gorm:"size:16"`
	Diagnosis  string     `gorm:"size:32"`
	ProposalID *uuid.UUID `gorm:"type:uuid;index"`
	Effect     string     `gorm:"size:16"`
	Details    JSONB      `gorm:"type:jsonb"`
	Note       string
}
