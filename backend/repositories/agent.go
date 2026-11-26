package repositories

import (
	"context"
	"errors"
	"strings"
	"time"

	"kuznica/api/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Агент SearchOps: запуски, шаги, кластеры, предложения и решения.

// CreateAgentRun — новый запуск.
func (r *Repos) CreateAgentRun(ctx context.Context, run *domain.AgentRun) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(run).Error
}

// SaveAgentRun — запуск целиком (счётчики, статус, эпизод).
func (r *Repos) SaveAgentRun(ctx context.Context, run *domain.AgentRun) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Save(run).Error
}

// GetAgentRun — запуск по id; нет — nil без ошибки.
func (r *Repos) GetAgentRun(ctx context.Context, id uuid.UUID) (*domain.AgentRun, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var run domain.AgentRun
	err := r.db.WithContext(ctx).First(&run, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &run, err
}

// ListAgentRuns — последние запуски, новые сверху.
func (r *Repos) ListAgentRuns(ctx context.Context, limit int) ([]domain.AgentRun, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.AgentRun
	err := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// FailStaleAgentRuns — запуски, оставшиеся running после падения процесса, помечаются failed.
func (r *Repos) FailStaleAgentRuns(ctx context.Context, reason string, at time.Time) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Model(&domain.AgentRun{}).Where("status = ?", domain.RunRunning).
		Updates(map[string]any{"status": domain.RunFailed, "stop_reason": reason, "finished_at": at}).Error
}

// CreateAgentStep — шаг ленты.
func (r *Repos) CreateAgentStep(ctx context.Context, st *domain.AgentStep) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(st).Error
}

// SaveAgentStep — шаг целиком (отметка отклонённой гипотезы).
func (r *Repos) SaveAgentStep(ctx context.Context, st *domain.AgentStep) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Save(st).Error
}

// ListAgentSteps — шаги запуска с номером больше afterSeq, по порядку.
func (r *Repos) ListAgentSteps(ctx context.Context, runID uuid.UUID, afterSeq int) ([]domain.AgentStep, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.AgentStep
	err := r.db.WithContext(ctx).Where("run_id = ? AND seq > ?", runID, afterSeq).Order("seq").Find(&rows).Error
	return rows, err
}

// SaveIssueClusters — кластеры запуска (вставка или обновление).
func (r *Repos) SaveIssueClusters(ctx context.Context, rows []domain.IssueCluster) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Save(&rows).Error
}

// SaveIssueCluster — один кластер.
func (r *Repos) SaveIssueCluster(ctx context.Context, c *domain.IssueCluster) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Save(c).Error
}

// CreateProposal — предложение в очередь.
func (r *Repos) CreateProposal(ctx context.Context, p *domain.Proposal) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(p).Error
}

// SaveProposal — предложение целиком (эффект после проверки).
func (r *Repos) SaveProposal(ctx context.Context, p *domain.Proposal) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Save(p).Error
}

// GetProposal — предложение по id; нет — nil без ошибки.
func (r *Repos) GetProposal(ctx context.Context, id uuid.UUID) (*domain.Proposal, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var p domain.Proposal
	err := r.db.WithContext(ctx).First(&p, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &p, err
}

// ListProposals — предложения с фильтром по статусу и роли (пусто — все), по убыванию потерь.
func (r *Repos) ListProposals(ctx context.Context, status, role string) ([]domain.Proposal, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	q := r.db.WithContext(ctx).Model(&domain.Proposal{})
	if status != "" {
		q = q.Where("status = ?", status)
	}
	if role != "" {
		q = q.Where("role = ?", role)
	}
	var rows []domain.Proposal
	err := q.Order("lost_demand DESC, created_at DESC").Limit(500).Find(&rows).Error
	return rows, err
}

// ListDecisions — решения человека, новые сверху.
func (r *Repos) ListDecisions(ctx context.Context) ([]domain.Decision, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.Decision
	err := r.db.WithContext(ctx).Order("created_at DESC").Limit(500).Find(&rows).Error
	return rows, err
}

// ListConfigVersions — все версии правил, новые сверху.
func (r *Repos) ListConfigVersions(ctx context.Context) ([]domain.ConfigVersion, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.ConfigVersion
	err := r.db.WithContext(ctx).Order("version DESC").Find(&rows).Error
	return rows, err
}

// GetConfigVersion — версия правил по номеру; нет — nil без ошибки.
func (r *Repos) GetConfigVersion(ctx context.Context, version int) (*domain.ConfigVersion, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var v domain.ConfigVersion
	err := r.db.WithContext(ctx).First(&v, "version = ?", version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &v, err
}

// SetConfigVersionEffect — эффект версии после проверки следующим запуском.
func (r *Repos) SetConfigVersionEffect(ctx context.Context, version int, effect domain.JSONB) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Model(&domain.ConfigVersion{}).Where("version = ?", version).
		Update("effect", effect).Error
}

// RulesChange — одна атомарная смена правил: новая версия поверх base (оптимистичная блокировка), решение
// по предложению и смена статусов предложений.
type RulesChange struct {
	// Base — версия, поверх которой построена новая; последняя версия в базе должна быть ровно ею.
	Base    int
	Version *domain.ConfigVersion
	// Proposal — решаемое предложение: должно быть в статусе pending; nil — откат без предложения.
	Proposal *domain.Proposal
	Decision *domain.Decision
	// RollBackAbove — откат: принятые предложения с версией выше этой становятся rolled_back (0 — не трогать).
	RollBackAbove int
	// RolledBack — решения rolled_back по откаченным предложениям (заполняется внутри).
	RolledBack []domain.Decision
}

// ApplyRulesChange — смена правил и решение одной транзакцией. Гонки: предложение уже решено —
// domain.ErrProposalDecided; правила изменились после base — domain.ErrRulesVersionConflict.
func (r *Repos) ApplyRulesChange(ctx context.Context, ch *RulesChange) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		// Один писатель правил за раз: замок транзакции, затем проверка версии.
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "kuznica_rules").Error; err != nil {
			return err
		}
		if ch.Proposal != nil {
			res := tx.Model(&domain.Proposal{}).Where("id = ? AND status = ?", ch.Proposal.ID, domain.ProposalPending).
				Updates(map[string]any{"status": ch.Proposal.Status, "decided_at": ch.Proposal.DecidedAt,
					"decided_reason": ch.Proposal.DecidedReason, "decided_comment": ch.Proposal.DecidedComment,
					"config_version": ch.Proposal.ConfigVersion})
			if res.Error != nil {
				return res.Error
			}
			if res.RowsAffected != 1 {
				return domain.ErrProposalDecided
			}
		}
		if ch.Version != nil {
			var latest *int
			if err := tx.Model(&domain.ConfigVersion{}).Select("MAX(version)").Scan(&latest).Error; err != nil {
				return err
			}
			if latest == nil || *latest != ch.Base {
				return domain.ErrRulesVersionConflict
			}
			if err := tx.Create(ch.Version).Error; err != nil {
				if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "23505") {
					return domain.ErrRulesVersionConflict
				}
				return err
			}
		}
		if ch.RollBackAbove > 0 {
			var undone []domain.Proposal
			if err := tx.Where("status = ? AND config_version > ?", domain.ProposalAccepted, ch.RollBackAbove).
				Find(&undone).Error; err != nil {
				return err
			}
			for i := range undone {
				p := &undone[i]
				if err := tx.Model(p).Update("status", domain.ProposalRolledBack).Error; err != nil {
					return err
				}
				d := domain.Decision{ProposalID: p.ID, Decision: domain.ProposalRolledBack, Reason: "rollback",
					ClusterKey: p.ClusterKey, ClusterLabel: p.ClusterLabel, Diagnosis: p.Diagnosis, PatchKeys: p.PatchKeys}
				if ch.Decision != nil {
					d.Comment = ch.Decision.Comment
				}
				if err := tx.Create(&d).Error; err != nil {
					return err
				}
				ch.RolledBack = append(ch.RolledBack, d)
			}
		}
		if ch.Decision != nil && ch.Proposal != nil {
			return tx.Create(ch.Decision).Error
		}
		return nil
	})
}

// AgentUsage — расход модели одного запуска агента (все строки llm_usage с его agent_run_id).
type AgentUsage struct {
	Calls        int
	InputTokens  int
	OutputTokens int
	CostRub      float64
}

// AgentRunUsage — сумма llm_usage по запуску агента.
func (r *Repos) AgentRunUsage(ctx context.Context, runID string) (AgentUsage, error) {
	var u AgentUsage
	if !r.Enabled() {
		return u, ErrNoDatabase
	}
	err := r.db.WithContext(ctx).Model(&domain.LLMUsage{}).Where("agent_run_id = ?", runID).
		Select("COUNT(*) AS calls, COALESCE(SUM(input_tokens),0) AS input_tokens, COALESCE(SUM(output_tokens),0) AS output_tokens, " +
			"COALESCE(SUM(calculated_cost),0) AS cost_rub").Scan(&u).Error
	return u, err
}
