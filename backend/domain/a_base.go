// Package domain — предметная область магазина: типы, статусы и GORM-модели. Правила расчёта
// здесь не живут: они в services.
package domain

import (
	"time"

	"github.com/google/uuid"
	"gorm.io/gorm"
)

// UUIDModel — база сущностей: UUIDv7 PK (в BeforeCreate) + created_at. UUIDv7 упорядочен по времени,
// поэтому вставка не разбрасывает первичный индекс.
type UUIDModel struct {
	ID        uuid.UUID `gorm:"type:uuid;primaryKey"`
	CreatedAt time.Time
}

func (m *UUIDModel) BeforeCreate(*gorm.DB) error {
	if m.ID == uuid.Nil {
		id, err := uuid.NewV7()
		if err != nil {
			return err
		}
		m.ID = id
	}
	return nil
}

// AllModels — модели для AutoMigrate (repositories.MigrateDB). Новая модель приезжает строкой сюда,
// иначе таблицы не будет.
func AllModels() []any {
	return []any{}
}
