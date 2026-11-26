package repositories

import (
	"context"

	"kuznica/api/domain"

	"gorm.io/gorm"
)

// SaveSearch — событие поиска и его трасса в одной транзакции.
func (r *Repos) SaveSearch(ctx context.Context, ev *domain.SearchEvent, tr *domain.SearchTrace) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Create(ev).Error; err != nil {
			return err
		}
		return tx.Create(tr).Error
	})
}

// SaveBehaviorEvent — событие поведения покупателя.
func (r *Repos) SaveBehaviorEvent(ctx context.Context, ev *domain.BehaviorEvent) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(ev).Error
}
