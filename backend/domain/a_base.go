// Package domain — предметная область магазина: типы, статусы и GORM-модели. Правила расчёта
// здесь не живут: они в services.
package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
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
	return []any{
		&Product{}, &Vendor{}, &Category{}, &Relation{}, &Alias{}, &Client{},
		&SearchEvent{}, &SearchTrace{}, &BehaviorEvent{}, &LLMUsage{}, &ConfigVersion{},
		&Evaluation{}, &JudgeRun{},
		&IntentCache{},
		&IssueCluster{}, &AgentRun{}, &AgentStep{}, &Proposal{}, &Decision{},
		&LicenseRule{}, &CompatibilityRule{}, &AttributeDefinition{}, &SolutionTemplate{}, &QuoteRequest{}, &CatalogInfo{},
		&Suggestion{},
	}
}

// JSONB — сырой JSON в колонке jsonb. В базу уходит текстом, наружу — как есть; пустое значение — null.
type JSONB json.RawMessage

// Value — запись в колонку: текст JSON (pgx сам приведёт к jsonb).
func (j JSONB) Value() (driver.Value, error) {
	if len(j) == 0 {
		return nil, nil
	}
	return string(j), nil
}

// Scan — чтение колонки jsonb.
func (j *JSONB) Scan(src any) error {
	switch v := src.(type) {
	case nil:
		*j = nil
	case []byte:
		*j = append((*j)[:0], v...)
	case string:
		*j = JSONB(v)
	default:
		return errors.New("jsonb: неожиданный тип значения")
	}
	return nil
}

func (j JSONB) MarshalJSON() ([]byte, error) {
	if len(j) == 0 {
		return []byte("null"), nil
	}
	return j, nil
}

func (j *JSONB) UnmarshalJSON(b []byte) error {
	*j = append((*j)[:0], b...)
	return nil
}

// ToJSONB — значение в JSONB.
func ToJSONB(v any) (JSONB, error) {
	b, err := json.Marshal(v)
	return b, err
}
