package repositories

import (
	"context"

	"kuznica/api/domain"

	"gorm.io/gorm"
)

// Индекс подсказок контура выдачи: сборка — новая версия config_versions kind = suggestions и её строки
// в suggestions в одной транзакции. Прежние сборки не удаляются: откат — пересборка или чтение версии.

func suggestionsKind(db *gorm.DB) *gorm.DB {
	return db.Where("config_versions.kind = ?", domain.ConfigKindSuggestions)
}

// SaveSuggestions — новая сборка индекса подсказок; возвращает её номер.
func (r *Repos) SaveSuggestions(ctx context.Context, v *domain.ConfigVersion, rows []domain.Suggestion) (int, error) {
	if !r.Enabled() {
		return 0, ErrNoDatabase
	}
	v.Kind = domain.ConfigKindSuggestions
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", "kuznica_suggestions").Error; err != nil {
			return err
		}
		var latest *int
		if err := tx.Model(&domain.ConfigVersion{}).Scopes(suggestionsKind).Select("MAX(version)").Scan(&latest).Error; err != nil {
			return err
		}
		v.Version = 1
		if latest != nil {
			v.Version = *latest + 1
		}
		if err := tx.Create(v).Error; err != nil {
			return err
		}
		for i := range rows {
			rows[i].IndexVersion = v.Version
		}
		if len(rows) == 0 {
			return nil
		}
		return tx.CreateInBatches(rows, 500).Error
	})
	return v.Version, err
}

// LatestSuggestionsVersion — номер последней сборки; 0 — сборок нет.
func (r *Repos) LatestSuggestionsVersion(ctx context.Context) (int, error) {
	if !r.Enabled() {
		return 0, ErrNoDatabase
	}
	var latest *int
	err := r.db.WithContext(ctx).Model(&domain.ConfigVersion{}).Scopes(suggestionsKind).Select("MAX(version)").Scan(&latest).Error
	if err != nil || latest == nil {
		return 0, err
	}
	return *latest, nil
}

// SuggestionsOf — строки сборки version.
func (r *Repos) SuggestionsOf(ctx context.Context, version int) ([]domain.Suggestion, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []domain.Suggestion
	err := r.db.WithContext(ctx).Where("index_version = ?", version).Order("weight DESC, norm").Find(&rows).Error
	return rows, err
}

// SaveTrace — трасса без события поиска: панель подсказок пишет её на каждый ответ (GET /api/traces/{id}),
// а событие search — только отправленный запрос (иначе каждое нажатие стало бы «поиском» в аналитике).
func (r *Repos) SaveTrace(ctx context.Context, tr *domain.SearchTrace) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Create(tr).Error
}
