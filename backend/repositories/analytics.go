package repositories

import (
	"context"
	"time"

	"kuznica/api/domain"

	"gorm.io/gorm"
)

// evalRequestPrefix — request_id поисков прогона оценки: в аналитику они не попадают.
const evalRequestPrefix = "eval-%"

// ReplaceMockLog — исторические логи целиком: прежние записи mock_log удаляются, новые вставляются
// (повторный импорт идемпотентен). Живые события не трогаются.
func (r *Repos) ReplaceMockLog(ctx context.Context, searches []domain.SearchEvent, behavior []domain.BehaviorEvent) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("source = ?", domain.EventSourceMockLog).Delete(&domain.SearchEvent{}).Error; err != nil {
			return err
		}
		if err := tx.Where("source = ?", domain.EventSourceMockLog).Delete(&domain.BehaviorEvent{}).Error; err != nil {
			return err
		}
		if err := createAll(tx, searches); err != nil {
			return err
		}
		return createAll(tx, behavior)
	})
}

// SearchEventsSince — поиски с момента since по времени (живые и импортированные), без прогонов оценки и
// без поисков вкладки «Один запрос» (source = compare).
func (r *Repos) SearchEventsSince(ctx context.Context, since time.Time) ([]domain.SearchEvent, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.SearchEvent
	err := r.db.WithContext(ctx).Where("created_at >= ? AND request_id NOT LIKE ? AND source <> ?", since, evalRequestPrefix, domain.EventSourceCompare).
		Order("created_at").Find(&rows).Error
	return rows, err
}

// BehaviorEventsSince — события поведения с момента since по времени.
func (r *Repos) BehaviorEventsSince(ctx context.Context, since time.Time) ([]domain.BehaviorEvent, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.BehaviorEvent
	err := r.db.WithContext(ctx).Where("created_at >= ?", since).Order("created_at, id").Find(&rows).Error
	return rows, err
}

// RunCluster — кластер запуска агента и время запуска (вывод агента у проблемного запроса).
type RunCluster struct {
	domain.IssueCluster
	RunStartedAt time.Time
}

// RecentIssueClusters — кластеры последних runs запусков агента: новые запуски первыми, внутри запуска — по
// месту в очереди.
func (r *Repos) RecentIssueClusters(ctx context.Context, runs int) ([]RunCluster, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []RunCluster
	err := r.db.WithContext(ctx).Raw(`SELECT c.*, r.started_at AS run_started_at FROM issue_clusters c
JOIN (SELECT id, started_at FROM agent_runs ORDER BY started_at DESC LIMIT ?) r ON r.id = c.run_id
ORDER BY r.started_at DESC, c.rank, c.label`, runs).Scan(&rows).Error
	return rows, err
}
