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
	MaxToolCalls   int             `json:"max_tool_calls"`
	InputTokens    int             `json:"input_tokens"`
	OutputTokens   int             `json:"output_tokens"`
	CostRub        float64         `json:"cost_rub"`
	StopReason     string          `json:"stop_reason"`
	Summary        string          `json:"summary"`
	CurrentCluster *ClusterRef     `json:"current_cluster"`
	Model          string          `json:"model"`
	RulesVersion   int             `json:"rules_version"`
	Limits         AgentLimits     `json:"limits"`
	Episode        json.RawMessage `json:"episode"`
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
	// pre_holdout — запуск до разреза журнала видел отложенную неделю и тексты test; пояснение — в decided_comment.
	StaleReason   *string         `json:"stale_reason"`
	ConfigVersion *int            `json:"config_version"`
	Effect        json.RawMessage `json:"effect"`
	// CheckQuery — запрос для кнопки «Проверить в поиске».
	CheckQuery string `json:"check_query"`
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
