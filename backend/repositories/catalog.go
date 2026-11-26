package repositories

import (
	"context"
	"errors"
	"time"

	"kuznica/api/domain"

	"gorm.io/gorm"
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

// rulesLock — имя advisory-замка записи правил (тот же, что у ApplyRulesChange).
const rulesLock = "kuznica_rules"

// ImportRules — итог записи исходных словарей импортом.
type ImportRules struct {
	Version int // действующая версия правил после импорта
	// NewChain — каталог сменился: прежние версии архивированы, цепочка начата заново.
	NewChain bool
	// Stale — ждавшие решения предложения, ставшие stale.
	Stale int
}

// SaveImportRules — исходные словари каталога catalogVersion (ревью M1). Одна транзакция под замком правил:
//   - версий нет — версия 1;
//   - тот же каталог — базовая версия текущей цепочки (последняя import) перезаписывается, версии агента и
//     отката поверх неё остаются: повторный импорт того же снимка идемпотентен;
//   - другой каталог — все прежние версии помечаются archived (история не стирается), версия N+1 = новые
//     исходные словари (source import), ждущие решения предложения → stale: они проверялись на прежнем словаре.
func (r *Repos) SaveImportRules(ctx context.Context, catalogVersion string, rules domain.JSONB, comment string) (ImportRules, error) {
	var res ImportRules
	if !r.Enabled() {
		return res, ErrNoDatabase
	}
	err := r.db.WithContext(ctx).Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", rulesLock).Error; err != nil {
			return err
		}
		var latest domain.ConfigVersion
		err := tx.Order("version DESC").First(&latest).Error
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			v := domain.ConfigVersion{Version: 1, Source: "import", Author: "system", Comment: comment, Rules: rules,
				CatalogVersion: catalogVersion}
			res.Version = 1
			return tx.Create(&v).Error
		case err != nil:
			return err
		}
		if !latest.Archived && latest.CatalogVersion == catalogVersion {
			res.Version = latest.Version
			return tx.Exec(`UPDATE config_versions SET rules = ?, comment = ? WHERE version = (
				SELECT MAX(version) FROM config_versions WHERE source = 'import' AND NOT archived AND catalog_version = ?)`,
				rules, comment, catalogVersion).Error
		}
		if err := tx.Model(&domain.ConfigVersion{}).Where("NOT archived").Update("archived", true).Error; err != nil {
			return err
		}
		v := domain.ConfigVersion{Version: latest.Version + 1, Source: "import", Author: "system", Comment: comment,
			Rules: rules, CatalogVersion: catalogVersion}
		if err := tx.Create(&v).Error; err != nil {
			return err
		}
		res.Version, res.NewChain = v.Version, true
		st := tx.Model(&domain.Proposal{}).Where("status = ?", domain.ProposalPending).
			Updates(map[string]any{"status": domain.ProposalStale, "decided_at": time.Now().UTC(),
				"decided_reason": domain.ProposalStaleReason})
		res.Stale = int(st.RowsAffected)
		return st.Error
	})
	return res, err
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
