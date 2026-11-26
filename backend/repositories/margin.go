package repositories

import (
	"context"
	"errors"
	"strings"

	"kuznica/api/domain"

	"gorm.io/gorm"
)

// Версии политики маржи — своя цепочка config_versions с kind = margin. Каталог цепочку не
// архивирует: вес рычага не зависит от снимка каталога.

// ErrMarginVersionConflict — политика изменилась после версии, поверх которой строилась новая.
var ErrMarginVersionConflict = errors.New("margin policy version conflict")

func marginKind(db *gorm.DB) *gorm.DB {
	return db.Where("config_versions.kind = ?", domain.ConfigKindMargin)
}

// LatestMarginPolicy — последняя версия политики маржи; нет — nil без ошибки.
func (r *Repos) LatestMarginPolicy(ctx context.Context) (*domain.ConfigVersion, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var v domain.ConfigVersion
	err := r.db.WithContext(ctx).Scopes(marginKind).Order("version DESC").First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// ListMarginPolicies — все версии политики маржи, новые сверху.
func (r *Repos) ListMarginPolicies(ctx context.Context) ([]domain.ConfigVersion, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.ConfigVersion
	err := r.db.WithContext(ctx).Scopes(marginKind).Order("version DESC").Find(&rows).Error
	return rows, err
}

// GetMarginPolicy — версия политики по номеру; нет — nil без ошибки.
func (r *Repos) GetMarginPolicy(ctx context.Context, version int) (*domain.ConfigVersion, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var v domain.ConfigVersion
	err := r.db.WithContext(ctx).Scopes(marginKind).First(&v, "version = ?", version).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	return &v, err
}

// CreateMarginPolicy — новая версия политики поверх base (0 — версий ещё нет). Политика изменилась после
// base — ErrMarginVersionConflict.
func (r *Repos) CreateMarginPolicy(ctx context.Context, base int, v *domain.ConfigVersion) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	v.Kind = domain.ConfigKindMargin
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "kuznica_margin").Error; err != nil {
			return err
		}
		var latest *int
		if err := tx.Model(&domain.ConfigVersion{}).Scopes(marginKind).Select("MAX(version)").Scan(&latest).Error; err != nil {
			return err
		}
		cur := 0
		if latest != nil {
			cur = *latest
		}
		if cur != base {
			return ErrMarginVersionConflict
		}
		v.Version = base + 1
		if err := tx.Create(v).Error; err != nil {
			if errors.Is(err, gorm.ErrDuplicatedKey) || strings.Contains(err.Error(), "23505") {
				return ErrMarginVersionConflict
			}
			return err
		}
		return nil
	})
}
