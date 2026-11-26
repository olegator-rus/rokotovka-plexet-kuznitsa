package repositories

import (
	"context"
	"errors"

	"kuznica/api/domain"

	"gorm.io/gorm"
)

// Таблица вклада компонентов (контур доводки): прогон целиком одной строкой evaluations (eval_set = contribution,
// Metrics = весь ответ), как коммерческая лестница. Config — короткий код набора (колонка size:4).

const contributionEvalSet = "contribution"

// contributionCode — код набора в колонке config.
func contributionCode(set string) string {
	switch set {
	case domain.EvalSetRegression:
		return "regr"
	default:
		if len(set) > 4 {
			return set[:4]
		}
		return set
	}
}

// SaveContribution — записать прогон таблицы вклада набора set.
func (r *Repos) SaveContribution(ctx context.Context, set string, row *domain.Evaluation) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	row.Set, row.Config = contributionEvalSet, contributionCode(set)
	return r.db.WithContext(ctx).Create(row).Error
}

// LatestContribution — последний прогон таблицы вклада набора set; нет — nil.
func (r *Repos) LatestContribution(ctx context.Context, set string) (*domain.Evaluation, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var row domain.Evaluation
	err := r.db.WithContext(ctx).Where("eval_set = ? AND config = ?", contributionEvalSet, contributionCode(set)).
		Order("created_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &row, err
}
