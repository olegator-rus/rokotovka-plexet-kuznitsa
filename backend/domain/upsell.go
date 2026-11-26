package domain

import "time"

// Данные допродажи на фиде заказчика (контур фида): записи совместимости с первичным документом и
// акции магазина. Оба справочника — CSV рядом с mapping.yaml, читаются при старте API (переимпорт не нужен).

// Уровни основания карточки «Дополните решение».
const (
	EvidenceA = "A" // документ вендора о совместимости
	EvidenceB = "B" // каталог: тип лицензии, название SKU, экосистема вендора
	EvidenceC = "C" // та же задача, совместимость не подтверждена
)

// CompatDoc — запись совместимости «продукт × платформа» с первичным документом. Семейства каталога — по точному
// названию (переживает переимпорт). Дата документа хранится, в интерфейсе не показывается.
type CompatDoc struct {
	ID               string
	ProductVendor    string
	ProductFamilies  []string // точные family_name фида
	Product          string   // как названо в документе
	ProductVersion   string
	PlatformVendor   string
	PlatformFamilies []string
	Platform         string
	PlatformVersion  string
	Level            string // A | C
	DocKind          string // certificate | protocol | documentation | instruction | catalog
	DocTitle         string
	DocNumber        string
	DocDate          string // YYYY-MM-DD как в документе; пусто — даты нет
	Publisher        string
	URL              string
	Note             string
	// Короткие подписи для строки причины (без дат): «КриптоПро CSP 5.0 R2», «Astra Linux SE 1.7»,
	// «сертификат Ready for Astra № 29073/2025». Пусто — из полей документа.
	ProductLabel, PlatformLabel, DocLabel string
}

// Promo — акция магазина. Показывается только при подтверждённом сроке и пока он не истёк; срок в интерфейсе не
// пишется. OfferMatch — подстроки названия позиции (любая), пусто — всё семейство.
type Promo struct {
	ID          string
	Vendor      string
	Families    []string
	OfferMatch  []string
	Title       string
	Text        string
	DiscountPct *int
	Kind        string // segment_price | migration_discount | segment_discount | price_discount | sale | offer
	ValidFrom   time.Time
	ValidTo     time.Time // включительно
	URL         string
	CheckedAt   string
	Note        string
}

// ActiveOn — акция действует в этот день (по срокам со страницы акции).
func (p Promo) ActiveOn(t time.Time) bool {
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	if p.ValidTo.IsZero() || day.After(p.ValidTo) {
		return false
	}
	return p.ValidFrom.IsZero() || !day.Before(p.ValidFrom)
}

// UpsellClass — класс продукта для допродажи (контур эталонов, VENDORS.md): категории второго уровня фида и, при широкой
// категории, слова в названии семейства (Include — хотя бы одно, Exclude — ни одного; регулярные выражения по
// свёрнутому названию).
type UpsellClass struct {
	ID         string
	Title      string
	Categories []string
	Include    string
	Exclude    string
	Source     string
	Note       string
}

// UpsellLink — строка таблицы классов: класс запроса → класс дополнения с причиной на карточке. To — id класса или
// self (тип лицензии в том же семействе, Kinds) / self_variant (позиция того же семейства под другую платформу).
// RequireDoc — показывать только кандидатов с документом о совместимости. Дата записи хранится, не выводится.
type UpsellLink struct {
	From       string
	To         string
	Kinds      []string
	Max        int
	RequireDoc bool
	Reason     string
	Source     string
	AddedAt    string
}

// Особые цели строки таблицы классов.
const (
	UpsellSelf        = "self"
	UpsellSelfVariant = "self_variant"
)
