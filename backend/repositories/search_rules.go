package repositories

import (
	"context"
	"errors"

	"kuznica/api/domain"

	"gorm.io/gorm"
)

// LatestRules — последняя версия словарей и правил поиска (config_versions). Правил нет — nil без ошибки.
func (r *Repos) LatestRules(ctx context.Context) (*domain.RulesSnapshot, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var v domain.ConfigVersion
	err := r.db.WithContext(ctx).Scopes(rulesKind).Order("version DESC").First(&v).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &domain.RulesSnapshot{Version: v.Version, Rules: v.Rules}, nil
}

// rulesKind — только цепочка правил поиска (config_versions.kind = rules): версии связей и политики маржи
// (контур коммерции) нумеруются отдельно и в правила поиска не попадают.
func rulesKind(db *gorm.DB) *gorm.DB {
	return db.Where("config_versions.kind = ?", domain.ConfigKindRules)
}
