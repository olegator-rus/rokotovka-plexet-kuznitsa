package repositories

import (
	"context"
	"time"

	"kuznica/api/domain"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

// insertBatch — строк в одном INSERT при импорте.
const insertBatch = 200

// ReplaceCatalog — снимок каталога целиком в одной транзакции: прежние строки удаляются, новые
// вставляются. Повторный импорт того же снимка даёт то же состояние (идемпотентно).
func (r *Repos) ReplaceCatalog(ctx context.Context, s *domain.CatalogSnapshot, importedAt time.Time) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	for i := range s.Products {
		s.Products[i].CatalogVersion = s.Version
		s.Products[i].ImportedAt = importedAt
	}
	return r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		for _, m := range []any{&domain.Product{}, &domain.Vendor{}, &domain.Category{}, &domain.Relation{},
			&domain.Alias{}, &domain.Client{}} {
			if err := tx.Where("1 = 1").Delete(m).Error; err != nil {
				return err
			}
		}
		for _, insert := range []func() error{
			func() error { return createAll(tx, s.Products) },
			func() error { return createAll(tx, s.Vendors) },
			func() error { return createAll(tx, s.Categories) },
			func() error { return createAll(tx, s.Relations) },
			func() error { return createAll(tx, s.Aliases) },
			func() error { return createAll(tx, s.Clients) },
		} {
			if err := insert(); err != nil {
				return err
			}
		}
		return nil
	})
}

// createAll — вставка среза пачками; пустой срез — ничего.
func createAll[T any](tx *gorm.DB, rows []T) error {
	if len(rows) == 0 {
		return nil
	}
	return tx.CreateInBatches(rows, insertBatch).Error
}

// LoadCatalog — снимок каталога из базы: API, запущенный без файлов данных, берёт справочники отсюда.
// Каталог пуст — nil без ошибки.
func (r *Repos) LoadCatalog(ctx context.Context) (*domain.CatalogSnapshot, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	db := r.db.WithContext(ctx)
	s := &domain.CatalogSnapshot{}
	if err := db.Order("id").Find(&s.Products).Error; err != nil {
		return nil, err
	}
	if len(s.Products) == 0 {
		return nil, nil
	}
	s.Version = s.Products[0].CatalogVersion
	for _, q := range []struct {
		dst   any
		order string
	}{
		{&s.Vendors, "id"}, {&s.Categories, "id"}, {&s.Relations, "created_at"}, {&s.Aliases, "created_at"}, {&s.Clients, "id"},
	} {
		if err := db.Order(q.order).Find(q.dst).Error; err != nil {
			return nil, err
		}
	}
	return s, nil
}

// SaveBaseRules — версия правил 1: исходные словари из каталога. Повторный импорт её перезаписывает;
// версии агента (2+) не трогает.
func (r *Repos) SaveBaseRules(ctx context.Context, rules domain.JSONB, comment string) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	v := domain.ConfigVersion{Version: 1, Source: "import", Comment: comment, Rules: rules}
	return r.db.WithContext(ctx).Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "version"}},
		DoUpdates: clause.AssignmentColumns([]string{"source", "comment", "rules"}),
	}).Create(&v).Error
}

// RulesVersion — текущая (последняя) версия правил; правил нет — 0.
func (r *Repos) RulesVersion(ctx context.Context) (int, error) {
	if !r.Enabled() {
		return 0, ErrNoDatabase
	}
	var v *int
	err := r.db.WithContext(ctx).Model(&domain.ConfigVersion{}).Select("MAX(version)").Scan(&v).Error
	if err != nil || v == nil {
		return 0, err
	}
	return *v, nil
}
