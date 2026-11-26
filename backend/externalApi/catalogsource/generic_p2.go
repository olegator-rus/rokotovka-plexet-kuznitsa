package catalogsource

import (
	"errors"
	"fmt"
	"io/fs"
	"strings"

	"kuznica/api/domain"
)

// Поля контура коммерции в Generic: все необязательны — mapping без них читается как раньше.

// hardwareFields — kind, mpn, model_id, compatible_with, price_mode и числовые характеристики позиции.
func (l *genericLoader) hardwareFields(tb *table, rec record, p *domain.Product) {
	switch f := fold(tb.get(rec, "kind")); {
	case f == "":
	case strings.Contains(f, "hard") || strings.Contains(f, "обор") || strings.Contains(f, "желез"):
		p.Kind = domain.KindHardware
	default:
		p.Kind = domain.KindSoftware
	}
	p.MPN = tb.get(rec, "mpn")
	p.ModelID = tb.get(rec, "model_id")
	p.CompatibleWith = splitList(tb.get(rec, "compatible_with"))
	if raw := tb.get(rec, "price_mode"); raw != "" {
		p.PriceMode = normPriceMode(raw)
	}
	for _, a := range tb.t.Specs {
		col, key := splitAttr(a)
		for _, h := range tb.td.headers {
			if fold(h) != fold(col) {
				continue
			}
			if v, ok := parseNumber(rec[h]); ok {
				if p.Specs == nil {
					p.Specs = map[string]float64{}
				}
				p.Specs[key] = v
			}
		}
	}
	if p.Kind == "" && (p.MPN != "" || len(p.Specs) > 0) {
		p.Kind = domain.KindHardware
	}
}

// relationFields — поля связи контура коммерции.
func (l *genericLoader) relationFields(tb *table, rec record, r *domain.Relation) {
	r.RelID = tb.get(rec, "id")
	if v := l.productID(tb.get(rec, "from_product")); v != "" {
		r.FromProduct = &v
	}
	if v := l.productID(tb.get(rec, "to_product")); v != "" {
		r.ToProduct = &v
	}
	r.Requirement = strings.ToLower(tb.get(rec, "requirement"))
	r.Compatibility = strings.ToLower(tb.get(rec, "compatibility"))
	r.Conditions = splitList(tb.get(rec, "conditions"))
	r.QuantityStrategy = strings.ToLower(tb.get(rec, "quantity_strategy"))
	r.Source = strings.ToLower(tb.get(rec, "source"))
	r.SourceURL = tb.get(rec, "source_url")
	r.EvidenceLevel = strings.ToUpper(tb.get(rec, "evidence_level"))
	r.ReviewStatus = strings.ToLower(tb.get(rec, "review_status"))
	r.ReviewedAt = tb.get(rec, "reviewed_at")
	r.UpgradeAdds = splitList(tb.get(rec, "upgrade_adds"))
}

// loadP2 — справочники контура коммерции из JSON-файлов mapping (формат контракта) и пометка синтетических данных.
func (l *genericLoader) loadP2() error {
	snap := l.d.snap
	snap.Synthetic = l.m.Synthetic
	for _, o := range []struct {
		key, file string
		dst       any
	}{
		{"license_rules", l.m.LicenseRules, &snap.LicenseRules},
		{"compatibility_rules", l.m.CompatibilityRules, &snap.CompatibilityRules},
		{"attribute_definitions", l.m.AttributeDefinitions, &snap.AttributeDefinitions},
		{"solution_templates", l.m.SolutionTemplates, &snap.SolutionTemplates},
	} {
		if o.file == "" {
			continue
		}
		if err := readJSON(l.m.path(o.file), o.dst); err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return fmt.Errorf("%s %s: файла нет", o.key, o.file)
			}
			return err
		}
	}
	return nil
}

// normPriceMode — тип цены по-русски или по контракту.
func normPriceMode(raw string) string {
	f := fold(raw)
	switch {
	case strings.Contains(f, "quote") || strings.Contains(f, "запрос") || strings.Contains(f, "кп"):
		return domain.PriceQuoteRequired
	case strings.Contains(f, "custom") || strings.Contains(f, "индивид") || strings.Contains(f, "клиент"):
		return domain.PriceCustomerSpecific
	case strings.Contains(f, "unavail") || strings.Contains(f, "нет в нал") || strings.Contains(f, "недоступ"):
		return domain.PriceUnavailable
	case f == "from" || strings.HasPrefix(f, "от"):
		return domain.PriceFrom
	}
	return domain.PriceFixed
}

// splitList — значения ячейки через запятую, точку с запятой или «|».
func splitList(raw string) []string {
	var res []string
	for _, v := range strings.FieldsFunc(raw, func(r rune) bool { return r == ',' || r == ';' || r == '|' }) {
		if v = strings.TrimSpace(v); v != "" {
			res = append(res, v)
		}
	}
	return res
}
