package domain

import "time"

// TZResult — разбор загруженного ТЗ под хешем (контур ТЗ, TZ-CORE §7): только позиции с цитатами, без файла и
// полного текста документа. Hash — sha256 содержимого файла (hex): повторная загрузка того же файла и открытие
// ссылки модель не зовут. Срок — неделя; истёкшая запись не отдаётся и удаляется при следующей записи.
type TZResult struct {
	Hash           string `gorm:"primaryKey;size:64"`
	FileName       string `gorm:"size:255"`
	Positions      JSONB  `gorm:"type:jsonb"`
	ExtractedCount int
	ProcessedCount int
	// ParserVersion — версия разбора (промпт и проверки): при смене повторная загрузка разбирает заново.
	ParserVersion string `gorm:"size:64"`
	CreatedAt     time.Time
	ExpiresAt     time.Time `gorm:"index"`
}

// BehaviorTZUpload — событие журнала поведения: загружено ТЗ (payload — формат, число позиций, без текста).
const BehaviorTZUpload = "tz_upload"
