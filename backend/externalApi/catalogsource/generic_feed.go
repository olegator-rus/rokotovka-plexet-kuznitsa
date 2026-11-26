package catalogsource

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"slices"
	"sort"
	"strings"

	"kuznica/api/domain"
)

// Поля контура фида в Generic (фид заказчика): наличие, прежняя цена, страницы позиции и семейства, вид и ступень
// лицензии, путь категорий. Все необязательны — mapping без них читается как раньше.

// attrCol — колонка файла, которая идёт в свободный атрибут позиции.
type attrCol struct{ header, key string }

// attributeColumns — колонки атрибутов каталога, разрешённые один раз на таблицу: «Платформа: platform»,
// «Платформа» или «attrs.*» — все колонки с префиксом «attrs.», ключ — остаток имени.
func attributeColumns(tb *table) []attrCol {
	var out []attrCol
	for _, a := range tb.t.Attributes {
		if prefix, ok := strings.CutSuffix(strings.TrimSpace(a), "*"); ok {
			for _, h := range tb.td.headers {
				if rest, found := strings.CutPrefix(h, prefix); found && rest != "" {
					out = append(out, attrCol{header: h, key: rest})
				}
			}
			continue
		}
		col, key := splitAttr(a)
		for _, h := range tb.td.headers {
			if fold(h) == fold(col) {
				out = append(out, attrCol{header: h, key: key})
			}
		}
	}
	// Порядок ключей не важен (атрибуты — объект), но одинаковый порядок упрощает сверку отчётов.
	sort.SliceStable(out, func(i, j int) bool { return out[i].key < out[j].key })
	return out
}

// feedFields — наличие, прежняя цена и страницы позиции. Правило цены: действующая цена — только
// у позиции в наличии с ценой больше нуля; цена позиции не в наличии хранится отдельно и как цена не отдаётся.
func (l *genericLoader) feedFields(tb *table, rec record, p *domain.Product) {
	p.OfferURL = tb.get(rec, "offer_url")
	p.FamilyURL = tb.get(rec, "family_url")
	if raw := tb.get(rec, "old_price_rub"); raw != "" {
		if v, onReq, ok := parsePrice(raw); ok && !onReq && v > 0 {
			p.OldPriceRub = &v
		}
	}
	if raw := tb.get(rec, "available"); raw != "" {
		if avail, ok := parseBool(raw); ok {
			p.Available = &avail
			if !avail && p.PriceRub != nil {
				p.FeedPriceRub, p.PriceRub, p.PriceOnRequest = p.PriceRub, nil, true
			}
		}
	}
}

// licenseFields — вид позиции, тип организации и ступень количества лицензии.
func (l *genericLoader) licenseFields(tb *table, rec record, lic *domain.License) {
	if k := strings.ToLower(tb.get(rec, "license_kind")); k != "" {
		if !slices.Contains(domain.LicenseKinds, k) {
			k = domain.LicenseKindOther
		}
		lic.Kind = k
	}
	lic.OrgType = strings.ToLower(tb.get(rec, "license_org_type"))
	if v, ok := parseInt(tb.get(rec, "license_tier_min")); ok && v > 0 {
		lic.TierMin = &v
	}
	if v, ok := parseInt(tb.get(rec, "license_tier_max")); ok && v > 0 {
		lic.TierMax = &v
	}
}

// fillCategoryPaths — id категорий от корня до категории позиции (для URL «вендор + категория» и фасетов уровня).
func (l *genericLoader) fillCategoryPaths() {
	parent := make(map[string]string, len(l.d.snap.Categories))
	for _, c := range l.d.snap.Categories {
		if c.ParentID != nil {
			parent[c.ID] = *c.ParentID
		} else {
			parent[c.ID] = ""
		}
	}
	cache := map[string][]string{}
	for i := range l.d.snap.Products {
		p := &l.d.snap.Products[i]
		if p.CategoryID == "" {
			continue
		}
		if path, ok := cache[p.CategoryID]; ok {
			p.CategoryPathIDs = path
			continue
		}
		var path []string
		seen := map[string]bool{}
		for id := p.CategoryID; id != "" && !seen[id]; id = parent[id] {
			if _, known := parent[id]; !known {
				break
			}
			seen[id] = true
			path = append([]string{id}, path...)
		}
		cache[p.CategoryID] = path
		p.CategoryPathIDs = path
	}
}

// loadFamilyURLs — страницы семейств (family_urls): family_id → url. Файла нет — предупреждение, не ошибка:
// таблицу собирает отдельный сборщик, импорт без неё работает.
func (l *genericLoader) loadFamilyURLs() error {
	t := l.m.FamilyURLs
	if t == nil {
		return nil
	}
	if _, err := os.Stat(l.m.path(t.File)); errors.Is(err, fs.ErrNotExist) {
		l.d.snap.Warnings = append(l.d.snap.Warnings, "family_urls: файла "+t.File+" нет — URL семейств пустые")
		return nil
	}
	tb, err := l.open("family_urls", t)
	if err != nil {
		// Сборщик дописывает файл построчно: недописанная строка не должна ронять импорт каталога.
		l.d.snap.Warnings = append(l.d.snap.Warnings, "family_urls: файл не прочитан ("+err.Error()+") — URL семейств пустые")
		return nil
	}
	urls := make(map[string]string, len(tb.td.rows))
	for _, rec := range tb.td.rows {
		id, url := tb.get(rec, "family_id"), tb.get(rec, "url")
		if id == "" || url == "" {
			tb.skip("нет family_id или url")
			continue
		}
		urls[id] = url
	}
	families, hit := map[string]bool{}, map[string]bool{}
	for i := range l.d.snap.Products {
		p := &l.d.snap.Products[i]
		families[p.FamilyID] = true
		if u, ok := urls[p.FamilyID]; ok && p.FamilyURL == "" {
			p.FamilyURL = u
			hit[p.FamilyID] = true
		}
	}
	tb.info.Notes = append(tb.info.Notes, fmt.Sprintf("URL есть у %d семейств из %d", len(hit), len(families)))
	tb.finish(l)
	return nil
}
