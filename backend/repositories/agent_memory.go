package repositories

import (
	"context"
	"errors"

	"kuznica/api/domain"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// Память агента контура автономии: «не предлагать» и эпизоды; решения — таблица decisions (agent.go).

// ListAgentRunsByTrigger — последние запуски триггера (пусто — все), новые сверху.
func (r *Repos) ListAgentRunsByTrigger(ctx context.Context, trigger string, limit int) ([]domain.AgentRun, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	q := r.db.WithContext(ctx).Order("created_at DESC").Limit(limit)
	if trigger != "" {
		q = q.Where("trigger = ?", trigger)
	}
	var rows []domain.AgentRun
	return rows, q.Find(&rows).Error
}

// CreateSuppress — запись «не предлагать».
func (r *Repos) CreateSuppress(ctx context.Context, m *domain.MemorySuppress) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(m).Error
}

// SaveSuppress — запись «не предлагать» целиком (текущее значение условия, снятие).
func (r *Repos) SaveSuppress(ctx context.Context, m *domain.MemorySuppress) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Save(m).Error
}

// GetSuppress — запись по id; нет — nil без ошибки.
func (r *Repos) GetSuppress(ctx context.Context, id uuid.UUID) (*domain.MemorySuppress, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var m domain.MemorySuppress
	err := r.db.WithContext(ctx).First(&m, "id = ?", id).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &m, err
}

// ListSuppress — записи «не предлагать», новые сверху.
func (r *Repos) ListSuppress(ctx context.Context) ([]domain.MemorySuppress, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.MemorySuppress
	return rows, r.db.WithContext(ctx).Order("created_at DESC").Limit(500).Find(&rows).Error
}

// CreateEpisode — эпизод.
func (r *Repos) CreateEpisode(ctx context.Context, e *domain.MemoryEpisode) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(e).Error
}

// ListEpisodes — эпизоды, новые сверху.
func (r *Repos) ListEpisodes(ctx context.Context, limit int) ([]domain.MemoryEpisode, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.MemoryEpisode
	return rows, r.db.WithContext(ctx).Order("created_at DESC").Limit(limit).Find(&rows).Error
}

// CreateDecision — решение человека вне очереди (снятие записи «не предлагать»).
func (r *Repos) CreateDecision(ctx context.Context, d *domain.Decision) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(d).Error
}

// ClusterKeysSeenBefore — ключи кластеров, которые уже были в запусках до runID (признак «новый» в приоритете).
func (r *Repos) ClusterKeysSeenBefore(ctx context.Context, runID uuid.UUID, keys []string) (map[string]bool, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	res := map[string]bool{}
	if len(keys) == 0 {
		return res, nil
	}
	var seen []string
	err := r.db.WithContext(ctx).Model(&domain.IssueCluster{}).Where("run_id <> ? AND key IN ?", runID, keys).
		Distinct("key").Pluck("key", &seen).Error
	for _, k := range seen {
		res[k] = true
	}
	return res, err
}
