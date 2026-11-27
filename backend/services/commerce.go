package services

import (
	"fmt"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/pkg/errs"
)

// Контур коммерции: допродажи, оборудование, маржа. Здесь — флаги и справочники
// в памяти; логика блоков — в своих файлах (треки допродажи, оборудования, маржи).

// Features — флаги контура коммерции (config.Features). Флаг выключен — ответ и выдача как в контуре поиска.
type Features struct {
	Hardware     bool `json:"hardware"`
	Companions   bool `json:"companions"`
	MarginPolicy bool `json:"margin_policy"`
	Solutions    bool `json:"solutions"`
	// ResultsV3 — страница результатов контура выдачи (§11.5, results.go).
	ResultsV3 bool `json:"results_v3"`
	// Панель поиска контура выдачи (§11.2–§11.3): панель, товары в ней, маржа как разрешение равенства, блок рекомендаций;
	// PanelViewed — основание «недавно просмотренное» (по умолчанию выкл.).
	Panel         bool `json:"panel"`
	PanelProducts bool `json:"panel_products"`
	PanelMargin   bool `json:"panel_margin"`
	PanelRecs     bool `json:"panel_recs"`
	PanelViewed   bool `json:"panel_viewed"`
}

// commerceCatalog — справочники контура коммерции из снимка каталога. Неизменяем после сборки: импорт заменяет
// catalogState целиком.
type commerceCatalog struct {
	products map[string]*domain.Product
	// relations — все связи (с полями контура коммерции); relationsFrom — по семейству-источнику.
	relations     []domain.Relation
	relationsFrom map[string][]domain.Relation
	licenseRules  []domain.LicenseRule
	// licenseByAnchor — правила лицензий по семейству-якорю.
	licenseByAnchor map[string][]domain.LicenseRule
	compatRules     []domain.CompatibilityRule
	// attrDefs — атрибуты по категории.
	attrDefs map[string][]domain.AttributeSpec
	// synthetic — что в данных придумано (catalog.synthetic); пусто — данные не помечены.
	synthetic []string
	// compat — индекс совместимости и допродажи (compat.go).
	compat *compatIndex
}

// newCommerceCatalog — справочники контура коммерции из снимка.
func newCommerceCatalog(s *domain.CatalogSnapshot) *commerceCatalog {
	c := &commerceCatalog{
		products:        make(map[string]*domain.Product, len(s.Products)),
		relations:       s.Relations,
		relationsFrom:   map[string][]domain.Relation{},
		licenseRules:    s.LicenseRules,
		licenseByAnchor: map[string][]domain.LicenseRule{},
		compatRules:     s.CompatibilityRules,
		attrDefs:        make(map[string][]domain.AttributeSpec, len(s.AttributeDefinitions)),
		synthetic:       append([]string{}, s.Synthetic...),
	}
	for i := range s.Products {
		c.products[s.Products[i].ID] = &s.Products[i]
	}
	for _, r := range s.Relations {
		c.relationsFrom[r.FromFamily] = append(c.relationsFrom[r.FromFamily], r)
	}
	for _, lr := range s.LicenseRules {
		c.licenseByAnchor[lr.AnchorFamily] = append(c.licenseByAnchor[lr.AnchorFamily], lr)
	}
	for _, ad := range s.AttributeDefinitions {
		c.attrDefs[ad.CategoryID] = ad.Attrs
	}
	c.compat = newCompatIndex(c)
	return c
}

// docKind — вид позиции из документа индекса; индекс контура поиска поля не знает — software.
func docKind(d indexDoc) string {
	if d.Kind == "" {
		return domain.KindSoftware
	}
	return d.Kind
}

// docPriceMode — тип цены из документа индекса; нет поля — по price_on_request, как при импорте.
func docPriceMode(d indexDoc) string {
	switch {
	case d.PriceMode != "":
		return d.PriceMode
	case d.PriceOnRequest:
		return domain.PriceQuoteRequired
	}
	return domain.PriceFixed
}

// maxFilters — фасетов в одном запросе поиска.
const maxFilters = 20

// filterOps — операции фасета.
var filterOps = map[string]bool{"eq": true, "gte": true, "lte": true, "range": true, "not": true}

// validateFilters — форма фасетов запроса: ключ, операция, значение (у range — и value2). Учитывать их в выдаче
// начнёт трек оборудования; неизвестный ключ — не ошибка (справочник атрибутов меняется с каталогом).
func validateFilters(fs []in.Filter) []errs.FieldError {
	var bad []errs.FieldError
	if len(fs) > maxFilters {
		return []errs.FieldError{{Field: "filters", Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Фасетов больше %d", maxFilters)}}
	}
	for i, f := range fs {
		field := fmt.Sprintf("filters[%d]", i)
		switch {
		case strings.TrimSpace(f.Key) == "" || len(f.Key) > maxIDLen:
			bad = append(bad, errs.FieldError{Field: field + ".key", Code: errs.CodeRequired, Message: "Нужен ключ атрибута"})
		case !filterOps[f.Op]:
			bad = append(bad, errs.FieldError{Field: field + ".op", Code: errs.CodeInvalidFormat, Message: "Операция — eq, gte, lte, range или not"})
		case f.Value == nil:
			bad = append(bad, errs.FieldError{Field: field + ".value", Code: errs.CodeRequired, Message: "Нужно значение"})
		case f.Op == "range" && f.Value2 == nil:
			bad = append(bad, errs.FieldError{Field: field + ".value2", Code: errs.CodeRequired, Message: "У диапазона нужна вторая граница"})
		}
	}
	return bad
}
