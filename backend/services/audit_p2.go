package services

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
)

// Аудит контура коммерции («Правила среза»): что из данных контура коммерции есть у источника,
// что восстановлено правилами, чего нет, и какие правила среза из этого следуют. Статистика совместных покупок
// удалена в контуре доводки — аудит её больше не считает.

// AuditP2 — данные контура коммерции в источнике.
type AuditP2 struct {
	Products       int            `json:"products"`
	Hardware       int            `json:"hardware"`
	HardwareMPN    int            `json:"hardware_mpn"`
	HardwareSpecs  int            `json:"hardware_specs"`
	SpecKeys       []string       `json:"spec_keys"`
	Systems        int            `json:"systems"`    // позиции с model_id
	Components     int            `json:"components"` // позиции с compatible_with
	CompatRules    int            `json:"compatibility_rules"`
	LicenseRules   int            `json:"license_rules"`
	AttributeDefs  int            `json:"attribute_definitions"`
	RelByType      map[string]int `json:"relations_by_type"`
	RelConditional int            `json:"relations_conditional_required"`
	RelCompatKnown int            `json:"relations_compat_known"`
	RelSourced     int            `json:"relations_with_source"`
	Margin         int            `json:"margin"`
	Synthetic      []string       `json:"synthetic"`
	// Заказы: строки order журнала, с номером (из данных и восстановленные), аккаунт из client_id.
	OrderLines        int `json:"order_lines"`
	OrderIDFromSource int `json:"order_id_from_source"`
	OrderIDRestored   int `json:"order_id_restored"`
	AccountFromSource int `json:"account_from_source"`
	AccountFromClient int `json:"account_from_client"`
	// CompanionPairs — размеченных пар блока; nil — разметки нет.
	CompanionPairs *int            `json:"companion_pairs"`
	Items          []AuditP2Item   `json:"items"`
	Slice          []AuditPlanItem `json:"slice"`
}

// AuditP2Item — строка таблицы «что из контура коммерции есть в данных».
type AuditP2Item struct {
	What   string `json:"what"`
	Have   string `json:"have"`
	Origin string `json:"origin"` // из данных | восстановлено | частично | нет
	Note   string `json:"note,omitempty"`
}

// auditP2 — данные контура коммерции; события уже прочитаны аудитом.
func (s *Services) auditP2(ctx context.Context, r *AuditReport, snap *domain.CatalogSnapshot, events []domain.LogEvent) *AuditP2 {
	p := &AuditP2{Products: len(snap.Products), RelByType: map[string]int{}, SpecKeys: []string{},
		Synthetic: append([]string{}, snap.Synthetic...), CompatRules: len(snap.CompatibilityRules),
		LicenseRules: len(snap.LicenseRules), AttributeDefs: len(snap.AttributeDefinitions)}
	keys := map[string]bool{}
	for _, pr := range snap.Products {
		if pr.Kind == domain.KindHardware {
			p.Hardware++
			if pr.MPN != "" {
				p.HardwareMPN++
			}
			if len(pr.Specs) > 0 {
				p.HardwareSpecs++
			}
			for k := range pr.Specs {
				keys[k] = true
			}
		}
		if pr.ModelID != "" {
			p.Systems++
		}
		if len(pr.CompatibleWith) > 0 {
			p.Components++
		}
		if pr.Margin > 0 {
			p.Margin++
		}
	}
	for k := range keys {
		p.SpecKeys = append(p.SpecKeys, k)
	}
	sort.Strings(p.SpecKeys)
	for _, rel := range snap.Relations {
		p.RelByType[rel.Type]++
		if rel.Requirement == domain.RequirementConditional {
			p.RelConditional++
		}
		if rel.Compatibility != "" && rel.Compatibility != domain.CompatUnknown {
			p.RelCompatKnown++
		}
		if rel.Source != "" {
			p.RelSourced++
		}
	}
	for _, e := range events {
		if e.Type != domain.EventOrder {
			continue
		}
		p.OrderLines++
		if e.OrderID != "" && !catalogsource.IsRestoredOrder(e.OrderID) {
			p.OrderIDFromSource++
		} else if e.OrderID != "" {
			p.OrderIDRestored++
		}
	}
	prov := map[string]catalogsource.FieldReport{}
	for _, f := range r.Provenance {
		prov[f.Field] = f
	}
	if a, ok := prov[catalogsource.FieldAccountID]; ok {
		p.AccountFromSource, p.AccountFromClient = a.FromSource, a.Restored
	} else {
		for _, e := range events {
			if e.Type == domain.EventOrder && e.AccountID != "" {
				p.AccountFromSource++
			}
		}
	}
	if l, ok := s.source.(companionPairLoader); ok {
		if ps, err := l.LoadCompanionPairs(ctx); err == nil && len(ps) > 0 {
			n := len(ps)
			p.CompanionPairs = &n
		}
	}
	p.Items = auditP2Items(p, prov)
	p.Slice = auditP2Slice(p)
	return p
}

// originOf — «из данных», «восстановлено», «частично» или «нет» по числам.
func originOf(have, total, restored int) string {
	switch {
	case have == 0:
		return "нет"
	case restored == 0 && have >= total:
		return "из данных"
	case restored >= have:
		return "восстановлено"
	case restored > 0:
		return "частично восстановлено"
	}
	return "частично"
}

func auditP2Items(p *AuditP2, prov map[string]catalogsource.FieldReport) []AuditP2Item {
	var items []AuditP2Item
	add := func(what, have, origin, note string) {
		items = append(items, AuditP2Item{What: what, Have: have, Origin: origin, Note: note})
	}
	add("Оборудование (kind)", fmt.Sprintf("%d из %d позиций", p.Hardware, p.Products), map[bool]string{true: "из данных", false: "нет"}[p.Hardware > 0],
		"kind из колонки; без неё оборудованием считается позиция с парт-номером или характеристиками")
	add("Парт-номера (mpn)", fmt.Sprintf("%d из %d единиц оборудования", p.HardwareMPN, p.Hardware), originOf(p.HardwareMPN, p.Hardware, 0), "")
	add("Характеристики (specs)", fmt.Sprintf("%d из %d единиц оборудования; ключи: %s", p.HardwareSpecs, p.Hardware, orDash(strings.Join(p.SpecKeys, ", "))),
		originOf(p.HardwareSpecs, p.Hardware, 0), fmt.Sprintf("справочник атрибутов: %d категорий", p.AttributeDefs))
	add("Совместимость", fmt.Sprintf("model_id у %d систем, compatible_with у %d компонентов, правил совместимости %d", p.Systems, p.Components, p.CompatRules),
		originOf(p.Systems+p.Components+p.CompatRules, 1, 0), "пара без правила — «совместимость не подтверждена»")
	rels := make([]string, 0, len(p.RelByType))
	for t, n := range p.RelByType {
		rels = append(rels, fmt.Sprintf("%s %d", t, n))
	}
	sort.Strings(rels)
	add("Связи по типам", orDash(strings.Join(rels, ", ")), originOf(len(rels), 1, 0),
		fmt.Sprintf("условно обязательных %d, совместимость указана у %d, источник у %d", p.RelConditional, p.RelCompatKnown, p.RelSourced))
	add("Правила лицензий (license_rules)", fmt.Sprint(p.LicenseRules), originOf(p.LicenseRules, 1, 0), "")
	synthetic := slices.Contains(p.Synthetic, "margin")
	add("Маржа", fmt.Sprintf("%d из %d позиций", p.Margin, p.Products),
		map[bool]string{true: "синтетика (плашка)", false: originOf(p.Margin, p.Products, 0)}[synthetic && p.Margin > 0], "")
	idNote := prov[catalogsource.FieldOrderID].Note
	add("Заказы с номером (order_id)", fmt.Sprintf("строк order %d: номер из данных %d, восстановлен %d", p.OrderLines, p.OrderIDFromSource, p.OrderIDRestored),
		originOf(p.OrderIDFromSource+p.OrderIDRestored, p.OrderLines, p.OrderIDRestored), idNote)
	add("Аккаунт покупателя (account_id)", fmt.Sprintf("из данных %d, из client_id %d строк order", p.AccountFromSource, p.AccountFromClient),
		originOf(p.AccountFromSource+p.AccountFromClient, p.OrderLines, p.AccountFromClient), prov[catalogsource.FieldAccountID].Note)
	if p.CompanionPairs != nil {
		add("Разметка блока (companion_pairs)", fmt.Sprintf("%d пар", *p.CompanionPairs), "из данных", "")
	} else {
		add("Разметка блока (companion_pairs)", "нет разметки", "нет", "метрики блока в `make eval SET=p2` не считаются")
	}
	return items
}

// auditP2Slice — какие правила среза контура коммерции следуют из данных («Правила среза»).
func auditP2Slice(p *AuditP2) []AuditPlanItem {
	var plan []AuditPlanItem
	add := func(item, finding, change string) {
		plan = append(plan, AuditPlanItem{Item: item, Finding: finding, Change: change})
	}
	switch {
	case p.Hardware == 0:
		add("Оборудование", "оборудования в данных нет", "FEATURE_HARDWARE=off; сценарии оборудования — в список несделанного, в видео не показывать")
	case p.HardwareSpecs*10 < p.Hardware*3 && p.HardwareMPN > 0:
		add("Оборудование", fmt.Sprintf("характеристики у %d из %d, парт-номера у %d", p.HardwareSpecs, p.Hardware, p.HardwareMPN),
			"нет атрибутов оборудования — остаётся поиск по парт-номерам; фасеты и разбор единиц не обещать")
	case p.HardwareMPN == 0:
		add("Оборудование", fmt.Sprintf("парт-номеров нет у %d единиц оборудования", p.Hardware),
			"точность по парт-номеру (p2_mpn) не считается; поиск оборудования — по названию и характеристикам")
	default:
		add("Оборудование", fmt.Sprintf("парт-номера у %d, характеристики у %d из %d", p.HardwareMPN, p.HardwareSpecs, p.Hardware),
			"поиск по парт-номерам, фасеты и разбор единиц — как на mock")
	}
	if p.CompatRules == 0 && p.Components == 0 && p.RelCompatKnown == 0 {
		add("Совместимость", "ни правил совместимости, ни compatible_with, ни совместимости у связей",
			"фильтр по совместимости режется первым; блок показывает «совместимость не подтверждена», не «совместимо»")
	} else {
		add("Совместимость", fmt.Sprintf("правил %d, компонентов с compatible_with %d, связей с совместимостью %d", p.CompatRules, p.Components, p.RelCompatKnown),
			"проверки совместимости остаются (не режутся); пары без правила — «не подтверждена»")
	}
	if p.LicenseRules == 0 && p.RelConditional == 0 {
		add("Условно обязательные компаньоны", "правил лицензий и условно обязательных связей нет",
			"не режутся: дописать правила вендоров под ключевые семейства (Windows Server + CAL, 1С) вручную; MISSING_REQUIRED без них не находится")
	} else {
		add("Условно обязательные компаньоны", fmt.Sprintf("правил лицензий %d, условно обязательных связей %d", p.LicenseRules, p.RelConditional),
			"не режутся; агент проверяет BROKEN_RELATION и MISSING_REQUIRED")
	}
	if p.RelByType[domain.RelationUpgrade] == 0 {
		add("Апселл", "связей upgrade нет", "«Старшая редакция» режется (в списке несделанного)")
	}
	switch {
	case p.Margin == 0:
		add("Маржа", "маржи в данных нет", "рычаг показывается на синтетике с плашкой (synthetic: [margin] в mapping.yaml)")
	case slices.Contains(p.Synthetic, "margin"):
		add("Маржа", fmt.Sprintf("маржа у %d из %d позиций, помечена синтетической", p.Margin, p.Products), "рычаг — с плашкой «Синтетические данные»")
	default:
		add("Маржа", fmt.Sprintf("маржа у %d из %d позиций", p.Margin, p.Products), "рычаг маржи с ограничениями — на данных заказчика")
	}
	if p.CompanionPairs == nil {
		add("Разметка блока", "companion_pairs нет", "метрики блока в `make eval SET=p2` — «нет разметки»; разметить пары ключевых якорей или говорить только о проверках совместимости")
	}
	return plan
}
