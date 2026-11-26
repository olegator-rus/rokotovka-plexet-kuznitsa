package services

import (
	"context"
	"time"

	"kuznica/api/domain"
	"kuznica/api/repositories"

	"github.com/google/uuid"
)

// agentStore — хранилище агента, предложений и версий правил. В бою — *repositories.Repos, в тестах —
// память (agent_test.go): цикл, допуск правки, гонка версий и память проверяются без базы.
type agentStore interface {
	CreateAgentRun(ctx context.Context, run *domain.AgentRun) error
	SaveAgentRun(ctx context.Context, run *domain.AgentRun) error
	GetAgentRun(ctx context.Context, id uuid.UUID) (*domain.AgentRun, error)
	ListAgentRuns(ctx context.Context, limit int) ([]domain.AgentRun, error)
	FailStaleAgentRuns(ctx context.Context, reason string, at time.Time) error
	CreateAgentStep(ctx context.Context, st *domain.AgentStep) error
	SaveAgentStep(ctx context.Context, st *domain.AgentStep) error
	ListAgentSteps(ctx context.Context, runID uuid.UUID, afterSeq int) ([]domain.AgentStep, error)
	SaveIssueClusters(ctx context.Context, rows []domain.IssueCluster) error
	SaveIssueCluster(ctx context.Context, c *domain.IssueCluster) error
	CreateProposal(ctx context.Context, p *domain.Proposal) error
	SaveProposal(ctx context.Context, p *domain.Proposal) error
	GetProposal(ctx context.Context, id uuid.UUID) (*domain.Proposal, error)
	ListProposals(ctx context.Context, status, role string) ([]domain.Proposal, error)
	ListDecisions(ctx context.Context) ([]domain.Decision, error)
	ListConfigVersions(ctx context.Context) ([]domain.ConfigVersion, error)
	GetConfigVersion(ctx context.Context, version int) (*domain.ConfigVersion, error)
	SetConfigVersionEffect(ctx context.Context, version int, effect domain.JSONB) error
	ApplyRulesChange(ctx context.Context, ch *repositories.RulesChange) error
	SearchEventsSince(ctx context.Context, since time.Time) ([]domain.SearchEvent, error)
	BehaviorEventsSince(ctx context.Context, since time.Time) ([]domain.BehaviorEvent, error)
	AgentRunUsage(ctx context.Context, runID string) (repositories.AgentUsage, error)
}

// AgentUsage — расход модели запуском по llm_usage.
type AgentUsage = repositories.AgentUsage

// store — хранилище агента; nil — без базы (агент и очередь недоступны).
func (s *Services) store() agentStore {
	if s.agentStoreOverride != nil {
		return s.agentStoreOverride
	}
	if !s.repos.Enabled() {
		return nil
	}
	return s.repos
}
