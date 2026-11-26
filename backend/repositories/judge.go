package repositories

import (
	"context"
	"errors"

	"kuznica/api/domain"

	"gorm.io/gorm"
)

// JudgeRequestPrefix — префикс request_id вызовов LLM-судьи в llm_usage (services.judgeRequestPrefix).
const JudgeRequestPrefix = "judge-"

// SaveJudgeRun — результат прогона судьи.
func (r *Repos) SaveJudgeRun(ctx context.Context, row *domain.JudgeRun) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(row).Error
}

// LatestJudgeRun — последний прогон судьи на наборе; nil — прогонов не было.
func (r *Repos) LatestJudgeRun(ctx context.Context, set string) (*domain.JudgeRun, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var row domain.JudgeRun
	err := r.db.WithContext(ctx).Where("eval_set = ?", set).Order("created_at DESC").First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}
