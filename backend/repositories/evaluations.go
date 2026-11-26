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
