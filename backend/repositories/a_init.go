// Package repositories — доступ к PostgreSQL через GORM. Сервисы ходят в базу только отсюда.
package repositories

import (
	"context"
	"log"
	"os"
	"time"

	"kuznica/api/domain"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	gormlogger "gorm.io/gorm/logger"
)

// Repos — доступ к БД. Репозитории добавляются полями и заполняются в newRepos.
//
// Выключенное состояние — nil *Repos: метод с указательным получателем вызывается и на nil,
// поэтому вызывающему достаточно Enabled(). Правило пакета: любой метод Repos начинается с
// проверки Enabled(), иначе он разыменует nil и уронит процесс.
type Repos struct {
	db *gorm.DB
}

// newRepos — репозитории поверх db (база или транзакция).
func newRepos(db *gorm.DB) *Repos {
	return &Repos{db: db}
}

// Transaction — fn в одной транзакции: tx — те же репозитории поверх неё. Ошибка или паника fn
// откатывает всё.
func (r *Repos) Transaction(ctx context.Context, fn func(tx *Repos) error) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	return r.db.WithContext(ctx).Transaction(func(db *gorm.DB) error { return fn(newRepos(db)) })
}

// ErrNoDatabase — обращение к БД, когда база выключена. Сервис, которому нужна запись, обязан
// спросить Enabled() раньше.
var ErrNoDatabase = errs.NewUnavailableError("Сервис запущен без базы данных")

// Enabled — включены ли репозитории.
func (r *Repos) Enabled() bool { return r != nil && r.db != nil }

// gormLogger — ParameterizedQueries: значения в лог не подставляются, иначе ошибка БД уносила
// бы в лог данные покупателей. IgnoreRecordNotFoundError: промах First — штатный ответ «такого нет».
var gormLogger = gormlogger.New(log.New(os.Stdout, "\r\n", log.LstdFlags), gormlogger.Config{
	SlowThreshold:             200 * time.Millisecond,
	LogLevel:                  gormlogger.Warn,
	Colorful:                  true,
	ParameterizedQueries:      true,
	IgnoreRecordNotFoundError: true,
})

// InitRepos — подключение к БД. Пустой dsn → выключенные репозитории (nil) и WARN: каркас
// поднимается без базы. Заданный, но нерабочий dsn — ошибка: это опечатка в конфиге или не поднятый
// `make db`, и молча работать без базы тогда хуже, чем упасть.
func InitRepos(dsn string) (*Repos, error) {
	if dsn == "" {
		zap.L().Warn("DATABASE_URL пуст — работаем без базы: миграция не выполняется")
		return nil, nil
	}
	// DisableForeignKeyConstraintWhenMigrating: никаких FK в БД — все связи резолвим в коде.
	db, err := gorm.Open(postgres.Open(dsn), &gorm.Config{
		DisableForeignKeyConstraintWhenMigrating: true,
		Logger:                                   gormLogger,
	})
	if err != nil {
		return nil, err
	}
	sqlDB, err := db.DB()
	if err != nil {
		return nil, err
	}
	sqlDB.SetMaxOpenConns(10)
	sqlDB.SetConnMaxLifetime(10 * time.Minute)
	zap.L().Info("база подключена")
	return newRepos(db), nil
}

// Ping — база отвечает. Для health: выключенная база — ErrNoDatabase.
func (r *Repos) Ping(ctx context.Context) error {
	if !r.Enabled() {
		return ErrNoDatabase
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.PingContext(ctx)
}

// Close — закрыть пул соединений.
func (r *Repos) Close() error {
	if !r.Enabled() {
		return nil
	}
	sqlDB, err := r.db.DB()
	if err != nil {
		return err
	}
	return sqlDB.Close()
}

// migrateLock — имя advisory-замка миграции.
const migrateLock = "kuznica_migrate"

// MigrateDB — ЕДИНСТВЕННАЯ точка миграции схемы. Одна транзакция под pg_advisory_xact_lock: процессы,
// стартовавшие одновременно, не мигрируют наперегонки. Без базы мигрировать нечего — это не отказ.
func (r *Repos) MigrateDB() error {
	if !r.Enabled() {
		return nil
	}
	models := domain.AllModels()
	if len(models) == 0 {
		return nil
	}
	return r.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Exec("SELECT pg_advisory_xact_lock(hashtext(?))", migrateLock).Error; err != nil {
			return err
		}
		if err := tx.AutoMigrate(models...); err != nil {
			return err
		}
		for _, q := range extraMigrations {
			if err := tx.Exec(q).Error; err != nil {
				return err
			}
		}
		return migrateP4Removed(tx)
	})
}

// Контур доводки: обратная миграция удалённого из кода. Каждая — идемпотентна.
const (
	// StaleReasonRemovedP4 — decided_reason предложений удалённых типов.
	StaleReasonRemovedP4  = "removed_p4"
	staleCommentRemovedP4 = "удалено в контуре доводки"
)

// removedP4Diagnoses — диагнозы агента, удалённые в контуре доводки: статистика совместных покупок и предложения по марже.
var removedP4Diagnoses = []string{"MISSING_RELATION", "BUNDLE_OPPORTUNITY", "MARGIN_OPPORTUNITY"}

// removedP4Templates — шаблоны решений, удалённые из data/mock в контуре доводки (остались «Рабочее место» и «Сервер под 1С»).
var removedP4Templates = []string{"tmpl-remote-office"}

func migrateP4Removed(tx *gorm.DB) error {
	// Ждущие и отклонённые предложения удалённых типов → stale с причиной «удалено в контуре доводки»: их больше нельзя
	// принять, а память агента по ним не нужна. Принятые не трогаются — это история версий.
	if err := tx.Exec(`UPDATE proposals SET status = 'stale', decided_reason = ?, decided_comment = ?,
		decided_at = COALESCE(decided_at, now()) WHERE diagnosis IN ? AND status IN ('pending', 'rejected')`,
		StaleReasonRemovedP4, staleCommentRemovedP4, removedP4Diagnoses).Error; err != nil {
		return err
	}
	// proposals.priority_score — оценка приоритета по совместным покупкам; поле снято с модели,
	// колонка снимается здесь явно (AutoMigrate колонки не удаляет).
	if tx.Migrator().HasColumn("proposals", "priority_score") {
		if err := tx.Exec(`ALTER TABLE proposals DROP COLUMN IF EXISTS priority_score`).Error; err != nil {
			return err
		}
	}
	// Удалённый шаблон решения: строка справочника до следующего импорта (импорт data/mock его уже не создаст).
	return tx.Exec(`DELETE FROM solution_templates WHERE id IN ?`, removedP4Templates).Error
}

// extraMigrations — то, что не выражается тегами GORM: индексы по времени для агрегатов аналитики за период
// (analytics_sql.go) и заполнение новых колонок у строк, созданных до них. Каждая — идемпотентна.
var extraMigrations = []string{
	`CREATE INDEX IF NOT EXISTS idx_search_events_created_at ON search_events (created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_behavior_events_created_at ON behavior_events (created_at)`,
	`CREATE INDEX IF NOT EXISTS idx_llm_usage_created_at ON llm_usage (created_at)`,
	// Версии правил до цепочек по каталогам (M1): каталог — из снимка правил.
	`UPDATE config_versions SET catalog_version = rules->>'catalog_version'
		WHERE COALESCE(catalog_version, '') = '' AND rules->>'catalog_version' IS NOT NULL`,
	// Контур коммерции: цепочки версий по виду (rules | relations | margin) — уникальна пара (kind, version), прежний
	// уникальный индекс по одному version снимается. Строки до kind — правила поиска.
	`UPDATE config_versions SET kind = 'rules' WHERE COALESCE(kind, '') = ''`,
	`DROP INDEX IF EXISTS idx_config_versions_version`,
}
