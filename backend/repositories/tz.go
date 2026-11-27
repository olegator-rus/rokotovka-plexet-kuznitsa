package repositories

import (
	"context"
	"errors"
	"time"

	"kuznica/api/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// GetTZResult — разбор ТЗ по хешу, если он не истёк к now; иначе nil без ошибки.
func (r *Repos) GetTZResult(ctx context.Context, hash string, now time.Time) (*domain.TZResult, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var row domain.TZResult
	err := r.db.WithContext(ctx).Where("hash = ? AND expires_at > ?", hash, now).First(&row).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &row, nil
}

// SaveTZResult — записать разбор (тот же хеш — перезапись) и удалить истёкшие к now.
func (r *Repos) SaveTZResult(ctx context.Context, row *domain.TZResult, now time.Time) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Where("expires_at <= ?", now).Delete(&domain.TZResult{}).Error; err != nil {
			return err
		}
		return tx.Clauses(clause.OnConflict{
			Columns: []clause.Column{{Name: "hash"}},
			DoUpdates: clause.AssignmentColumns([]string{"file_name", "positions", "extracted_count", "processed_count",
				"parser_version", "created_at", "expires_at"}),
		}).Create(row).Error
	})
}

// TouchTZResult — повторная загрузка того же файла: имя файла и новый срок, разбор прежний.
func (r *Repos) TouchTZResult(ctx context.Context, hash, fileName string, expires time.Time) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Model(&domain.TZResult{}).Where("hash = ?", hash).
		Updates(map[string]any{"file_name": fileName, "expires_at": expires}).Error
}
