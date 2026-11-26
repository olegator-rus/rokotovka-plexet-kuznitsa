package services

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Уточнения (refinements, §11.5, RESEARCH Г): структурированные условия из значений
// фасетов верхних кандидатов. Один источник для бейджей панели (refinementsFor добавляет продолжения подсказок, если
// они не дублируют показанное) и чипов «+» над выдачей. Каждое уточнение — фильтр выдачи (ключи §11.5 и
// характеристики оборудования), проверенный числом результатов: в пустоту и не сужающие не предлагаются,
// названное в запросе или выбранное фильтром — тоже. Без LLM и векторов.

// maxRefinements — 4–6 на десктопе (мобильный показывает первые 4).
const maxRefinements = 6

// refinementsPerField — не больше двух значений одного поля: ряд остаётся разнообразным.
const refinementsPerField = 2

// refineFields — поля ПО в порядке важности для закупки: для широкого запроса первыми идут типы продуктов
// (категория: «для дома» / «для бизнеса»), для семейства — параметры лицензии. Характеристики оборудования
// встают перед поставкой в порядке attribute_definitions категории.
var refineFields = []string{facetCategory, facetLicType, facetQuantity, facetDuration, facetMetric, facetReestr}

// refineTail — поля после характеристик оборудования.
var refineTail = []string{facetDelivery}

// typeRefLabels — подпись бейджа типа лицензии (подписка показывается, только если сужает).
var typeRefLabels = map[string]string{
	domain.LicenseRenewal: "продление", domain.LicensePerpetual: "бессрочная", domain.LicenseSubscription: "подписка",
	domain.LicenseOEM: "OEM",
}

// metricRefLabels — подпись бейджа метрики.
var metricRefLabels = map[string]string{
	domain.MetricDevice: "на устройство", domain.MetricUser: "на пользователя", domain.MetricServer: "на сервер",
	domain.MetricCore: "на ядро",
}

// metricQtyForms — «5 устройств»: формы слова метрики для числа мест.
var metricQtyForms = map[string][3]string{
	domain.MetricDevice: {"устройство", "устройства", "устройств"},
	domain.MetricUser:   {"пользователь", "пользователя", "пользователей"},
	domain.MetricServer: {"сервер", "сервера", "серверов"},
	domain.MetricCore:   {"ядро", "ядра", "ядер"},
}

// refineTriggers — слова запроса, которые уже называют поле: бейдж того же поля не предлагается (уже применён или
// противоречит запросу). Сравнение — по началу свёрнутого слова.
var refineTriggers = map[string][]string{
	facetLicType:  {"продл", "бессроч", "подписк", "oem", "коробк"},
	facetDuration: {"год", "года", "лет", "мес"},
	facetMetric:   {"пользоват", "устройств", "ядр", "ядер", "сервер", "пк", "узл", "мест"},
	facetQuantity: {"пользоват", "устройств", "ядр", "ядер", "пк", "узл", "мест"},
	facetReestr:   {"отечеств", "реестр", "российск", "импортозамещ"},
	facetCategory: {"бизнес", "дом", "домашн", "корпоратив", "предприят"},
}

// refineCtx — что уже названо: поля фильтров и чипов (taken) и слова запроса.
type refineCtx struct {
	lx    *searchLexicon
	hx    *hardwareLexicon
	query string
	taken map[string]bool
}

// namedByWords — поля, которые называют слова запроса (число — количество мест).
func namedByWords(query string) map[string]bool {
	res := map[string]bool{}
	for _, w := range strings.Fields(foldText(query)) {
		if refNumRe.MatchString(w) {
			res[facetQuantity] = true
		}
		for field, ps := range refineTriggers {
			for _, p := range ps {
				if strings.HasPrefix(w, p) {
					res[field] = true
				}
			}
		}
	}
	return res
}

// refineValue — одно значение поля у позиции: ключ фильтра (field), значение и подпись бейджа.
type refineValue struct{ field, value, label string }

// categoryRefLabel — «Антивирусы для дома» → «для дома»; иначе название категории.
func categoryRefLabel(name string) string {
	if i := strings.Index(name, " для "); i >= 0 {
		return strings.TrimSpace(name[i+1:])
	}
	return name
}

// numLabel — число без лишних нулей, с запятой.
func numLabel(x float64) string {
	return strings.ReplaceAll(strconv.FormatFloat(x, 'f', -1, 64), ".", ",")
}

// hwRefLabel — подпись характеристики: «память 16 ГБ», «накопитель 1 ТБ», «диагональ 15,6″», «без ОС».
func hwRefLabel(a hwAttr, raw string) string {
	name := strings.ToLower(a.label)
	switch a.typ {
	case "number":
		x, err := strconv.ParseFloat(raw, 64)
		if err != nil {
			return ""
		}
		unit := a.unit
		if unit == "ГБ" && x >= 1000 && int(x)%1000 == 0 {
			x, unit = x/1000, "ТБ"
		}
		switch unit {
		case "":
			return fmt.Sprintf("%s %s", name, numLabel(x))
		case "″":
			return fmt.Sprintf("%s %s″", name, numLabel(x))
		}
		return fmt.Sprintf("%s %s %s", name, numLabel(x), unit)
	case "bool":
		if raw != "true" {
			return ""
		}
		return a.label
	}
	if len(strings.Fields(raw)) > 1 {
		return raw
	}
	return name + " " + raw
}

// valuesOf — значения полей уточнений у позиции.
func (rc refineCtx) valuesOf(d indexDoc) []refineValue {
	var res []refineValue
	if d.CategoryID != "" {
		name := d.Category
		if name == "" && rc.lx != nil {
			name = rc.lx.categoryName[d.CategoryID]
		}
		if name != "" {
			res = append(res, refineValue{facetCategory, d.CategoryID, categoryRefLabel(name)})
		}
	}
	if docKind(d) == domain.KindHardware {
		if rc.hx != nil {
			attrs := rc.hx.docAttrs[d.ProductID]
			for _, key := range rc.hx.catAttrs[d.CategoryID] {
				a, ok := rc.hx.attrs[key]
				if !ok || !a.facet {
					continue
				}
				raw := ""
				if a.typ == "number" {
					x, ok := d.Specs[key]
					if !ok {
						continue
					}
					raw = strconv.FormatFloat(x, 'f', -1, 64)
				} else {
					raw = attrs[key]
				}
				if l := hwRefLabel(a, raw); raw != "" && l != "" {
					res = append(res, refineValue{key, raw, l})
				}
			}
		}
	} else {
		if l := typeRefLabels[d.LicenseType]; l != "" {
			res = append(res, refineValue{facetLicType, d.LicenseType, l})
		}
		if d.LicenseMetric != nil {
			m := *d.LicenseMetric
			if l := metricRefLabels[m]; l != "" {
				res = append(res, refineValue{facetMetric, m, l})
			}
			if forms, ok := metricQtyForms[m]; ok && d.LicenseQuantity != nil && *d.LicenseQuantity > 1 {
				q := *d.LicenseQuantity
				// Одно место — минимальная единица, не уточнение. Подпись несёт метрику: значение запоминается вместе с ней, разные метрики одного числа не сливаются.
				res = append(res, refineValue{facetQuantity, strconv.Itoa(q) + "|" + m,
					fmt.Sprintf("%d %s", q, plural(q, forms))})
			}
		}
		if d.LicenseDuration != nil && *d.LicenseDuration > 0 {
			res = append(res, refineValue{facetDuration, strconv.Itoa(*d.LicenseDuration), duration(*d.LicenseDuration)})
		}
		if d.InReestr {
			res = append(res, refineValue{facetReestr, "true", "отечественное"})
		}
	}
	if v := docDelivery(d); v != "" && docKind(d) != domain.KindHardware {
		res = append(res, refineValue{facetDelivery, v, strings.ToLower(deliveryLabels[v])})
	}
	return res
}

// structuredRefinements — уточнения по набору позиций (кандидаты панели или допустимые варианты семейств выдачи).
// Значение сужает: при нескольких семействах — есть не у всех семейств или не больше чем у половины вариантов, при
// одном семействе — не у всех вариантов.
// hits — число семейств со значением. Порядок — по кругу полей (сначала по одному значению каждого поля).
func structuredRefinements(rc refineCtx, docs []indexDoc, limit int) []out.Refinement {
	res := []out.Refinement{}
	if len(docs) == 0 || limit <= 0 {
		return res
	}
	type cand struct {
		v        refineValue
		families map[string]bool
		variants int
	}
	byField := map[string]map[string]*cand{}
	fams := map[string]bool{}
	var hwOrder []string
	seenHW := map[string]bool{}
	for _, d := range docs {
		fams[d.FamilyID] = true
		for _, v := range rc.valuesOf(d) {
			if byField[v.field] == nil {
				byField[v.field] = map[string]*cand{}
				if rc.hx != nil {
					if _, ok := rc.hx.attrs[v.field]; ok && !seenHW[v.field] {
						seenHW[v.field] = true
						hwOrder = append(hwOrder, v.field)
					}
				}
			}
			c := byField[v.field][v.value]
			if c == nil {
				c = &cand{v: v, families: map[string]bool{}}
				byField[v.field][v.value] = c
			}
			c.families[d.FamilyID] = true
			c.variants++
		}
	}
	words := namedByWords(rc.query)
	qwords := map[string]bool{}
	for _, w := range strings.Fields(foldText(rc.query)) {
		qwords[w] = true
	}
	order := append(append(append([]string{}, refineFields...), hwOrder...), refineTail...)
	perField := make([][]*cand, len(order))
	for i, field := range order {
		if rc.taken[field] || words[field] || field == facetMetric && rc.taken[facetQuantity] {
			continue
		}
		var cs []*cand
		for _, c := range byField[field] {
			// Сужает семейства или заметно сужает варианты (продление у всех семейств, но это треть позиций).
			narrows := len(c.families) < len(fams) || field != facetCategory && 2*c.variants <= len(docs)
			if len(fams) == 1 {
				narrows = c.variants < len(docs)
			}
			// Тип продукта из одного семейства на широкой выдаче — хвост поиска, а не тип.
			if field == facetCategory && len(fams) > 3 && len(c.families) < 2 {
				narrows = false
			}
			if !narrows || len(c.families) == 0 || c.v.label == "" {
				continue
			}
			named := false
			for _, w := range strings.Fields(foldText(c.v.label)) {
				if qwords[w] && !lexStopWords[w] {
					named = true
				}
			}
			if !named {
				cs = append(cs, c)
			}
		}
		sort.Slice(cs, func(a, b int) bool {
			if len(cs[a].families) != len(cs[b].families) {
				return len(cs[a].families) > len(cs[b].families)
			}
			if cs[a].variants != cs[b].variants {
				return cs[a].variants > cs[b].variants
			}
			return cs[a].v.value < cs[b].v.value
		})
		if len(cs) > refinementsPerField {
			cs = cs[:refinementsPerField]
		}
		perField[i] = cs
	}
	seen := map[string]bool{}
	for round := 0; round < refinementsPerField && len(res) < limit; round++ {
		for i := range order {
			if round >= len(perField[i]) || len(res) == limit {
				continue
			}
			c := perField[i][round]
			key := foldText(c.v.label)
			if seen[key] {
				continue
			}
			seen[key] = true
			field, value, op := c.v.field, c.v.value, refOpEq
			if field == facetQuantity {
				value, _, _ = strings.Cut(value, "|")
			}
			res = append(res, out.Refinement{Label: c.v.label, Query: joinQuery(rc.query, c.v.label), Field: &field,
				Value: &value, Op: &op, Hits: len(c.families)})
		}
	}
	return res
}

// refinementsFromFamilies — уточнения по допустимым вариантам семейств выдачи.
func refinementsFromFamilies(lx *searchLexicon, hx *hardwareLexicon, query string, fams []*familyHit, taken map[string]bool, limit int) []out.Refinement {
	var docs []indexDoc
	for _, fh := range fams {
		docs = append(docs, fh.heads...)
	}
	return structuredRefinements(refineCtx{lx: lx, hx: hx, query: query, taken: taken}, docs, limit)
}
