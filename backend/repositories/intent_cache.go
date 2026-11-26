package repositories

import (
	"context"
	"errors"
	"time"

	"kuznica/api/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetIntentCache — разбор по ключу кэша; промах — nil без ошибки. Попадание увеличивает счётчик.
func (r *Repos) GetIntentCache(ctx context.Context, key string, now time.Time) (*domain.IntentCache, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var row domain.IntentCache
	err := r.db.WithContext(ctx).Where("key = ?", key).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	if err := r.db.WithContext(ctx).Model(&domain.IntentCache{}).Where("key = ?", key).
		Updates(map[string]any{"hits": gorm.Expr("hits + 1"), "last_hit_at": now}).Error; err != nil {
		return nil, err
	}
	return &row, nil
}

// PutIntentCache — положить разбор; тот же ключ — перезапись разбора (гонка двух одинаковых запросов).
func (r *Repos) PutIntentCache(ctx context.Context, row *domain.IntentCache) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "key"}},
		DoUpdates: clause.AssignmentColumns([]string{"intent", "created_at"}),
	}).Create(row).Error
}
