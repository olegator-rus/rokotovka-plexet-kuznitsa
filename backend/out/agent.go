package out

import (
	"encoding/json"
	"time"
)

// Агент SearchOps, очередь предложений, версии правил.

// AgentRun — запуск агента.
type AgentRun struct {
	ID               string     `json:"id"`
	Status           string     `json:"status"`
	Trigger          string     `json:"trigger"`
	StartedAt        time.Time  `json:"started_at"`
	FinishedAt       *time.Time `json:"finished_at"`
	Days             int        `json:"days"`
	MaxClusters      int        `json:"max_clusters"`
	ClustersTotal    int        `json:"clusters_total"`
	ClustersDone     int        `json:"clusters_done"`
	ProposalsCreated int        `json:"proposals_created"`
	ToolCalls        int        `json:"tool_calls"`
	// MaxToolCalls — лимит вызовов инструментов на запуск: лимит на кластер × выбранные кластеры.
	MaxToolCalls   int         `json:"max_tool_calls"`
	InputTokens    int         `json:"input_tokens"`
	OutputTokens   int         `json:"output_tokens"`
	CostRub        float64     `json:"cost_rub"`
	StopReason     string      `json:"stop_reason"`
	Summary        string      `json:"summary"`
	CurrentCluster *ClusterRef `json:"current_cluster"`
	Model          string      `json:"model"`
	RulesVersion   int         `json:"rules_version"`
	// Mode — search | commerce | gaps (пробелы подбора); у commerce rules_version — версия справочника связей.
	Mode    string          `json:"mode"`
	Limits  AgentLimits     `json:"limits"`
	Episode json.RawMessage `json:"episode"`
	// Контур автономии: запрос «Разобрать» (null — по периоду), режим «только чтение», вход триггера,
	// последняя редакция плана, бюджет (лимиты и расход), итог по видам, неразобранное.
	Query       *string         `json:"query"`
	ReadOnly    bool            `json:"read_only"`
	Input       json.RawMessage `json:"input"`
	Plan        json.RawMessage `json:"plan"`
	Budget      json.RawMessage `json:"budget"`
	Outcome     RunOutcome      `json:"outcome"`
	Unprocessed json.RawMessage `json:"unprocessed"`
	DurationMs  *int64          `json:"duration_ms"`
}

// RunOutcome — итог запуска для строки «N предложений, M задач, K пропущено по памяти».
type RunOutcome struct {
	Proposals     int `json:"proposals"`
	Tasks         int `json:"tasks"`
	Rollbacks     int `json:"rollbacks"`
	Drafts        int `json:"drafts"`
	SkippedMemory int `json:"skipped_memory"`
	EffectChecks  int `json:"effect_checks"`
	Helped        int `json:"helped"`
	NoEffect      int `json:"no_effect"`
	Harmed        int `json:"harmed"`
	Checked       int `json:"checked"`
	Stale         int `json:"stale"`
}

// AgentLimits — ограничители запуска (для колонки «шаг N из лимита»).
type AgentLimits struct {
	ToolCallsPerCluster int `json:"tool_calls_per_cluster"`
	Hypotheses          int `json:"hypotheses"`
	TokensPerRun        int `json:"tokens_per_run"`
	TimeoutSec          int `json:"timeout_sec"`
}

// ClusterRef — кластер одной строкой.
type ClusterRef struct {
	ID    string `json:"id"`
	Label string `json:"label"`
}

// AgentStep — шаг ленты.
type AgentStep struct {
	ID             string          `json:"id"`
	RunID          string          `json:"run_id"`
	Seq            int             `json:"seq"`
	TS             time.Time       `json:"ts"`
	Kind           string          `json:"kind"`
	ClusterID      *string         `json:"cluster_id"`
	Tool           *string         `json:"tool"`
	Title          string          `json:"title"`
	Args           json.RawMessage `json:"args"`
	Result         json.RawMessage `json:"result"`
	OK             bool            `json:"ok"`
	HypothesisNo   *int            `json:"hypothesis_no"`
	RejectedReason *string         `json:"rejected_reason"`
	Ms             int             `json:"ms"`
}

// AgentRunStarted — 202 на POST /api/agent/runs.
type AgentRunStarted struct {
	RunID string `json:"run_id"`
}

// AgentRunList — GET /api/agent/runs.
type AgentRunList struct {
	Items []AgentRun `json:"items"`
}

// AgentRunDetail — GET /api/agent/runs/{id}.
type AgentRunDetail struct {
	Run   AgentRun    `json:"run"`
	Steps []AgentStep `json:"steps"`
}

// ProposalQuery — запрос кластера с частотой.
type ProposalQuery struct {
	Text     string `json:"text"`
	Freq     int    `json:"freq"`
	Failures int    `json:"failures"`
}

// ProposalCluster — кластер в карточке предложения.
type ProposalCluster struct {
	ID         string          `json:"id"`
	Label      string          `json:"label"`
	Queries    []ProposalQuery `json:"queries"`
	LostDemand float64         `json:"lost_demand"`
}

// Proposal — предложение агента.
type Proposal struct {
	ID              string          `json:"id"`
	RunID           string          `json:"run_id"`
	Cluster         ProposalCluster `json:"cluster"`
	Diagnosis       string          `json:"diagnosis"`
	DiagnosisText   string          `json:"diagnosis_text"`
	Role            string          `json:"role"`
	Action          string          `json:"action"`
	Patch           json.RawMessage `json:"patch"`
	Evaluation      json.RawMessage `json:"evaluation"`
	Evidence        json.RawMessage `json:"evidence"`
	HypothesesTried int             `json:"hypotheses_tried"`
	BaseVersion     int             `json:"base_version"`
	Status          string          `json:"status"`
	CreatedAt       time.Time       `json:"created_at"`
	DecidedAt       *time.Time      `json:"decided_at"`
	DecidedReason   *string         `json:"decided_reason"`
	DecidedComment  *string         `json:"decided_comment"`
	// StaleReason — почему предложение устарело (status = stale): catalog_changed — сменился каталог,
	// pre_holdout — запуск до разреза журнала видел отложенную неделю и тексты test; removed_p4 — тип предложения
	// удалён в контуре доводки; пояснение — в decided_comment.
	StaleReason   *string         `json:"stale_reason"`
	ConfigVersion *int            `json:"config_version"`
	Effect        json.RawMessage `json:"effect"`
	// CheckQuery — запрос для кнопки «Проверить в поиске».
	CheckQuery string `json:"check_query"`
	// Контур коммерции: уровень доказательности E0–E5, техническая корректность verified|unverified|failed,
	// экономический эффект unknown|estimated|measured. Пусто у контура поиска.
	EvidenceLevel   string `json:"evidence_level"`
	TechnicalStatus string `json:"technical_status"`
	EconomicStatus  string `json:"economic_status"`
	// Контур доводки: причина из семи (services/causes.go) и её владелец; Diagnosis — подпричина. Role
	// равна Owner: решение принимает владелец причины.
	Cause      string `json:"cause"`
	CauseLabel string `json:"cause_label"`
	Owner      string `json:"owner"`
	OwnerLabel string `json:"owner_label"`
	// Контур автономии: триггер запуска, который предложил (manual|schedule|spike|catalog_change; "" — запуск
	// не найден); задача владельцу (action ticket); откат, предложенный агентом (action rollback).
	Trigger     string       `json:"trigger"`
	RunStarted  *time.Time   `json:"run_started_at"`
	TaskToOwner bool         `json:"task_to_owner"`
	RollbackOf  *RollbackRef `json:"rollback_of"`
}

// RollbackRef — какую принятую правку откатывает предложение агента.
type RollbackRef struct {
	ProposalID    string `json:"proposal_id"`
	ConfigVersion *int   `json:"config_version"`
	Label         string `json:"label"`
}

// ProposalList — GET /api/proposals.
type ProposalList struct {
	Items []Proposal `json:"items"`
}

// ProposalDecision — ответ approve/reject.
type ProposalDecision struct {
	Proposal      Proposal `json:"proposal"`
	ConfigVersion *int     `json:"config_version,omitempty"`
}

// MemoryItem — решение человека в памяти агента.
type MemoryItem struct {
	ClusterLabel string    `json:"cluster_label"`
	ClusterKey   string    `json:"cluster_key"`
	ProposalID   string    `json:"proposal_id"`
	Diagnosis    string    `json:"diagnosis"`
	Decision     string    `json:"decision"`
	Reason       string    `json:"reason"`
	Comment      string    `json:"comment"`
	TS           time.Time `json:"ts"`
}

// MemoryList — GET /api/agent/memory.
type MemoryList struct {
	Items []MemoryItem `json:"items"`
}

// MemoryDecision — слой «Решения» (GET /api/agent/memory?layer=decisions).
type MemoryDecision struct {
	ID           string    `json:"id"`
	ProposalID   string    `json:"proposal_id"`
	RunID        *string   `json:"run_id"`
	Trigger      string    `json:"trigger"`
	ClusterLabel string    `json:"cluster_label"`
	ClusterKey   string    `json:"cluster_key"`
	Diagnosis    string    `json:"diagnosis"`
	Cause        string    `json:"cause"`
	CauseLabel   string    `json:"cause_label"`
	AgentReason  string    `json:"agent_reason"`
	Proposal     string    `json:"proposal"`
	Action       string    `json:"action"`
	Decision     string    `json:"decision"`
	Reason       string    `json:"reason"`
	ReasonLabel  string    `json:"reason_label"`
	Comment      string    `json:"comment"`
	DecidedBy    string    `json:"decided_by"`
	DecidedLabel string    `json:"decided_by_label"`
	TS           time.Time `json:"ts"`
}

// MemorySuppress — слой «Не предлагать».
type MemorySuppress struct {
	ID              string     `json:"id"`
	ClusterKey      string     `json:"cluster_key"`
	ClusterLabel    string     `json:"cluster_label"`
	Scope           string     `json:"scope"`
	PatchKeys       []string   `json:"patch_keys"`
	Diagnosis       string     `json:"diagnosis"`
	Why             string     `json:"why"`
	Reason          string     `json:"reason"`
	Comment         string     `json:"comment"`
	DecidedAt       time.Time  `json:"decided_at"`
	ProposalID      string     `json:"proposal_id"`
	RunID           string     `json:"run_id"`
	ReturnCondition string     `json:"return_condition"`
	FailuresAt      int        `json:"failures_at"`
	ReturnFailures  int        `json:"return_failures"`
	CurrentFailures int        `json:"current_failures"`
	CatalogAt       string     `json:"catalog_at"`
	CheckedAt       *time.Time `json:"checked_at"`
	CheckedRunID    *string    `json:"checked_run_id"`
	Status          string     `json:"status"`
	LiftedRunID     *string    `json:"lifted_run_id"`
	LiftedWhat      string     `json:"lifted_what"`
	RemovedReason   string     `json:"removed_reason"`
	RemovedBy       string     `json:"removed_by"`
	RemovedAt       *time.Time `json:"removed_at"`
}

// MemoryEpisode — слой «Эпизоды».
type MemoryEpisode struct {
	ID           string          `json:"id"`
	RunID        string          `json:"run_id"`
	Trigger      string          `json:"trigger"`
	Kind         string          `json:"kind"`
	ClusterKey   string          `json:"cluster_key"`
	ClusterLabel string          `json:"cluster_label"`
	Outcome      string          `json:"outcome"`
	Diagnosis    string          `json:"diagnosis"`
	Cause        string          `json:"cause"`
	CauseLabel   string          `json:"cause_label"`
	ProposalID   *string         `json:"proposal_id"`
	Effect       *string         `json:"effect"`
	Details      json.RawMessage `json:"details"`
	Note         string          `json:"note"`
	TS           time.Time       `json:"ts"`
}

// MemoryLayer — GET /api/agent/memory?layer=…: одна из трёх вкладок.
type MemoryLayer struct {
	Layer     string           `json:"layer"`
	Decisions []MemoryDecision `json:"decisions,omitempty"`
	Suppress  []MemorySuppress `json:"suppress,omitempty"`
	Episodes  []MemoryEpisode  `json:"episodes,omitempty"`
}

// AgentSettings — GET /api/agent/settings: режим, расписание, пороги триггеров, пул и бюджет.
type AgentSettings struct {
	ReadOnly       bool              `json:"read_only"`
	ModeLabel      string            `json:"mode_label"`
	Schedule       AgentSchedule     `json:"schedule"`
	Spike          AgentSpike        `json:"spike"`
	CatalogTrigger bool              `json:"catalog_trigger"`
	MaxParallel    int               `json:"max_parallel"`
	Running        int               `json:"running"`
	Budget         AgentBudgetLimits `json:"budget"`
	QueryBudget    AgentBudgetLimits `json:"query_budget"`
	Line           string            `json:"line"`
	// Regression — наборы, на которых агент проверяет правку (контур фида): на фиде без своих наборов правка не допускается.
	Regression AgentRegression `json:"regression"`
}

// AgentRegression — наборы регрессии агента на текущем каталоге.
type AgentRegression struct {
	Sets           []string       `json:"sets"`
	BySet          map[string]int `json:"by_set"`
	Queries        int            `json:"queries"`
	MaxWorse       int            `json:"max_worse"`
	Feed           bool           `json:"feed"`
	CatalogVersion string         `json:"catalog_version,omitempty"`
	Ready          bool           `json:"ready"`
	Message        string         `json:"message"`
}

// AgentSchedule — расписание ночного разбора.
type AgentSchedule struct {
	Enabled  bool       `json:"enabled"`
	At       string     `json:"at"`
	Interval string     `json:"interval"`
	Days     int        `json:"days"`
	NextAt   *time.Time `json:"next_at"`
}

// AgentSpike — пороги триггера «всплеск».
type AgentSpike struct {
	Enabled     bool    `json:"enabled"`
	Window      string  `json:"window"`
	MinSearches int     `json:"min_searches"`
	EmptyShare  float64 `json:"empty_share"`
	ReformShare float64 `json:"reform_share"`
	QueryFreq   int     `json:"query_freq"`
	Cooldown    string  `json:"cooldown"`
}

// AgentBudgetLimits — лимиты одного запуска.
type AgentBudgetLimits struct {
	Steps       int     `json:"steps"`
	Tokens      int     `json:"tokens"`
	CostRub     float64 `json:"cost_rub"`
	DurationSec int     `json:"duration_sec"`
}

// ConfigVersion — версия правил.
type ConfigVersion struct {
	Version     int             `json:"version"`
	CreatedAt   time.Time       `json:"created_at"`
	Author      string          `json:"author"`
	Source      string          `json:"source"`
	Description string          `json:"description"`
	ProposalID  *string         `json:"proposal_id"`
	Aliases     int             `json:"aliases"`
	Diff        json.RawMessage `json:"diff"`
	Effect      json.RawMessage `json:"effect"`
	// CatalogVersion — каталог, для которого собраны правила; Archived — версия прежнего каталога: после импорта
	// другого каталога цепочка начинается заново, откат к архивной версии — 409.
	CatalogVersion string `json:"catalog_version"`
	Archived       bool   `json:"archived"`
}

// ConfigList — GET /api/configs.
type ConfigList struct {
	Active int             `json:"active"`
	Items  []ConfigVersion `json:"items"`
}

// ConfigRollback — POST /api/configs/rollback.
type ConfigRollback struct {
	Version      int      `json:"version"`
	RolledBackTo int      `json:"rolled_back_to"`
	Proposals    []string `json:"rolled_back_proposals"`
}
