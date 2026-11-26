package repositories

import (
	"context"

	"kuznica/api/domain"
)

// SaveEvaluations — результаты прогона (по строке на конфигурацию) одной транзакцией.
func (r *Repos) SaveEvaluations(ctx context.Context, rows []domain.Evaluation) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	if len(rows) == 0 {
		return nil
	}
	return r.db.WithContext(ctx).Create(&rows).Error
}

// LatestEvaluations — последний прогон каждой конфигурации на наборе (без прогонов песочницы на
// снимке правил). Порядок — по конфигурации.
func (r *Repos) LatestEvaluations(ctx context.Context, set string) ([]domain.Evaluation, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.Evaluation
	err := r.db.WithContext(ctx).Raw(`SELECT DISTINCT ON (config) * FROM evaluations
		WHERE eval_set = ? AND rules_override = false ORDER BY config, created_at DESC`, set).Scan(&rows).Error
	return rows, err
}

// RecentEvaluations — прогоны наборов sets (без песочницы), новые сверху, не больше limit строк. Без per_query:
// сводке экрана хватает метрик.
func (r *Repos) RecentEvaluations(ctx context.Context, sets []string, limit int) ([]domain.Evaluation, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.Evaluation
	err := r.db.WithContext(ctx).Model(&domain.Evaluation{}).
		Select("id, created_at, run_id, eval_set, set_hash, queries, config, rules_version, rules_override, "+
			"catalog_version, commit, metrics, by_type, against, duration_ms").
		Where("eval_set IN ? AND rules_override = false", sets).
		Order("created_at DESC").Limit(limit).Find(&rows).Error
	return rows, err
}

// HasEvaluationsOfCatalog — есть ли прогоны на каталоге, версия которого начинается с prefix (mock-…).
func (r *Repos) HasEvaluationsOfCatalog(ctx context.Context, prefix string) (bool, error) {
	if !r.Enabled() {
		return false, ErrNoDatabase
	}
	var n int64
	err := r.db.WithContext(ctx).Model(&domain.Evaluation{}).Where("catalog_version LIKE ?", prefix+"%").Limit(1).Count(&n).Error
	return n > 0, err
}

// EvaluationPerQuery — результаты запросов одной конфигурации прогона.
func (r *Repos) EvaluationPerQuery(ctx context.Context, runID, config string) (domain.JSONB, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var row domain.Evaluation
	err := r.db.WithContext(ctx).Select("per_query").Where("run_id = ? AND config = ?", runID, config).First(&row).Error
	return row.PerQuery, err
}
