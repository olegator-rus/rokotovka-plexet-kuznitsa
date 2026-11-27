package services

import (
	"fmt"
	"sort"
	"strconv"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Лицензионные фасеты страницы результатов: вендор, тип, метрика, количество,
// срок, реестр, цена, «с ценой / по запросу», поставка. Считаются в памяти по допустимым вариантам семейств
// выдачи (familyHit.heads) — без запросов к движку, LLM и векторов. Счётчик значения — сколько семейств (без
// группировки — позиций) останется, если выбрать это значение при остальных фильтрах (без фильтра самого фасета).

// Ключи фильтров и фасетов выдачи контура выдачи.
const (
	facetVendor   = "vendor"
	facetLicType  = "license_type"
	facetMetric   = "license_metric"
	facetQuantity = "license_quantity"
	facetDuration = "duration_months"
	facetReestr   = "in_reestr"
	facetPrice    = "price_rub"
	facetHasPrice = "has_price"
	facetDelivery = "delivery"
	// facetCategory — категория (переход из панели по категории, §11.7): позиция категории или её подкатегорий.
	facetCategory = "category"
)

// Значения фасетов «цена» и «поставка».
const (
	hasPriceYes = "with_price"
	hasPriceNo  = "on_request"

	deliveryElectronic = "electronic"
	deliveryOEM        = "oem"
	deliveryPhysical   = "physical"
)

// licenseFacetKeys — ключи, которые фильтрует страница результатов (а не шаг оборудования).
var licenseFacetKeys = setOf(facetCategory, facetVendor, facetLicType, facetMetric, facetQuantity, facetDuration, facetReestr,
	facetPrice, facetHasPrice, facetDelivery)

// Порядок фасетов по виду выдачи: для ПО — закупочные параметры лицензии, для оборудования — после фасетов
// характеристик (hardware_facets.go) только общие.
var (
	softwareFacetOrder = []string{facetCategory, facetVendor, facetLicType, facetMetric, facetQuantity, facetDuration, facetReestr,
		facetPrice, facetHasPrice, facetDelivery}
	hardwareFacetOrder = []string{facetCategory, facetVendor, facetPrice, facetHasPrice, facetDelivery}
)

var licenseFacetLabels = map[string]string{
	facetCategory: "Категория", facetVendor: "Вендор", facetLicType: "Тип лицензии", facetMetric: "Метрика", facetQuantity: "Количество мест в лицензии",
	facetDuration: "Срок", facetReestr: "Реестр российского ПО", facetPrice: "Цена", facetHasPrice: "Наличие цены",
	facetDelivery: "Поставка",
}

var metricLabels = map[string]string{
	domain.MetricDevice: "На устройство", domain.MetricUser: "На пользователя", domain.MetricServer: "На сервер",
	domain.MetricCore: "На ядро", domain.MetricOrg: "На организацию",
}

var deliveryLabels = map[string]string{
	deliveryElectronic: "Электронная поставка", deliveryOEM: "OEM, с новым компьютером", deliveryPhysical: "Физическая поставка",
}

var hasPriceLabels = map[string]string{hasPriceYes: "С ценой", hasPriceNo: "По запросу"}

// chipFacet — поле чипа «как поняли запрос» → фасет, который поднимается наверх.
var chipFacet = map[string]string{
	domain.FieldLicenseQuantity: facetQuantity, domain.FieldLicenseMetric: facetMetric,
	domain.FieldLicenseType: facetLicType, domain.FieldRenewal: facetLicType, domain.FieldLicenseDuration: facetDuration,
	domain.FieldOrigin: facetReestr, fieldVendor: facetVendor, fieldPrice: facetPrice,
}

// licFilter — фильтр выдачи контура выдачи в разобранном виде.
type licFilter struct {
	key, op string
	str     string
	num     float64
	num2    float64
	isNum   bool
	boolean *bool
}

// parseLicFilters — фильтры запроса с ключами страницы результатов; остальные (характеристики) — шагу оборудования.
func parseLicFilters(fs []in.Filter) []licFilter {
	var res []licFilter
	for _, f := range fs {
		if !licenseFacetKeys[f.Key] {
			continue
		}
		lf := licFilter{key: f.Key, op: f.Op}
		switch v := f.Value.(type) {
		case string:
			lf.str = v
			if n, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64); err == nil {
				lf.num, lf.isNum = n, true
			}
			if b, err := strconv.ParseBool(v); err == nil {
				lf.boolean = &b
			}
		case float64:
			lf.num, lf.isNum, lf.str = v, true, strconv.FormatFloat(v, 'f', -1, 64)
		case bool:
			b := v
			lf.boolean, lf.str = &b, strconv.FormatBool(v)
		default:
			continue
		}
		switch v := f.Value2.(type) {
		case float64:
			lf.num2 = v
		case string:
			lf.num2, _ = strconv.ParseFloat(v, 64)
		}
		res = append(res, lf)
	}
	return res
}

// docDelivery — способ поставки позиции по данным каталога: атрибут delivery, OEM-лицензия, оборудование —
// физически. Данных нет — "" (в фасет не попадает, при сортировке по поставке — в конце).
func docDelivery(d indexDoc) string {
	a := strings.ToLower(d.Attributes)
	switch {
	case strings.Contains(a, "delivery: oem"), d.LicenseType == domain.LicenseOEM:
		return deliveryOEM
	case strings.Contains(a, "delivery: электрон"), strings.Contains(a, "delivery: esd"):
		return deliveryElectronic
	case docKind(d) == domain.KindHardware:
		return deliveryPhysical
	}
	return ""
}

// docPriced — у позиции есть цена (не «по запросу»): нулём «по запросу» не считается ни в фильтре, ни в сортировке.
func docPriced(d indexDoc) bool { return d.PriceRub != nil && !d.PriceOnRequest }

func numMatch(op string, x, a, b float64) bool {
	switch op {
	case "eq":
		return x == a
	case "not":
		return x != a
	case "gte":
		return x >= a
	case "lte":
		return x <= a
	case "range":
		return x >= a && x <= b
	}
	return false
}

func strMatch(op, have, want string) bool {
	if op == "not" {
		return have != want
	}
	return have == want
}

// match — позиция проходит фильтр.
func (f licFilter) match(d indexDoc) bool {
	switch f.key {
	case facetCategory:
		in := d.CategoryID == f.str || contains(d.CategoryIDs, f.str)
		return in == (f.op != "not")
	case facetVendor:
		return strMatch(f.op, d.VendorID, f.str) || f.op != "not" && strings.EqualFold(d.Vendor, f.str)
	case facetLicType:
		return strMatch(f.op, d.LicenseType, f.str)
	case facetMetric:
		m := ""
		if d.LicenseMetric != nil {
			m = *d.LicenseMetric
		}
		return strMatch(f.op, m, f.str)
	case facetQuantity:
		return d.LicenseQuantity != nil && f.isNum && numMatch(f.op, float64(*d.LicenseQuantity), f.num, f.num2)
	case facetDuration:
		months := 0
		if d.LicenseDuration != nil {
			months = *d.LicenseDuration
		}
		return f.isNum && numMatch(f.op, float64(months), f.num, f.num2)
	case facetReestr:
		return f.boolean != nil && (d.InReestr == *f.boolean) == (f.op != "not")
	case facetPrice:
		return docPriced(d) && f.isNum && numMatch(f.op, *d.PriceRub, f.num, f.num2)
	case facetHasPrice:
		v := hasPriceNo
		if docPriced(d) {
			v = hasPriceYes
		}
		return strMatch(f.op, v, f.str)
	case facetDelivery:
		return strMatch(f.op, docDelivery(d), f.str)
	}
	return true
}

// passAll — позиция проходит все фильтры, кроме фильтра по ключу skip.
func passAll(fs []licFilter, d indexDoc, skip string) bool {
	for _, f := range fs {
		if f.key != skip && !f.match(d) {
			return false
		}
	}
	return true
}

// facetValueOf — значение позиции для фасета: ключ, значение для ответа, подпись; ok=false — значения нет.
func facetValueOf(lx *searchLexicon, key string, d indexDoc) (string, any, string, bool) {
	switch key {
	case facetCategory:
		if d.CategoryID == "" {
			return "", nil, "", false
		}
		name := d.Category
		if name == "" {
			name = lx.categoryName[d.CategoryID]
		}
		return d.CategoryID, d.CategoryID, name, true
	case facetVendor:
		if d.VendorID == "" {
			return "", nil, "", false
		}
		name := d.Vendor
		if name == "" {
			name = lx.vendorName[d.VendorID]
		}
		return d.VendorID, d.VendorID, name, true
	case facetLicType:
		if d.LicenseType == "" {
			return "", nil, "", false
		}
		l := licenseTypeLabels[d.LicenseType]
		if l == "" {
			l = d.LicenseType
		}
		return d.LicenseType, d.LicenseType, l, true
	case facetMetric:
		if d.LicenseMetric == nil || *d.LicenseMetric == "" {
			return "", nil, "", false
		}
		m := *d.LicenseMetric
		l := metricLabels[m]
		if l == "" {
			l = m
		}
		return m, m, l, true
	case facetQuantity:
		if d.LicenseQuantity == nil {
			return "", nil, "", false
		}
		q := *d.LicenseQuantity
		return strconv.Itoa(q), float64(q), strconv.Itoa(q), true
	case facetDuration:
		if docKind(d) == domain.KindHardware || d.LicenseType == "" {
			return "", nil, "", false
		}
		months := 0
		if d.LicenseDuration != nil {
			months = *d.LicenseDuration
		}
		l := "Бессрочно"
		if months > 0 {
			l = duration(months)
		}
		return strconv.Itoa(months), float64(months), l, true
	case facetReestr:
		if docKind(d) == domain.KindHardware {
			return "", nil, "", false
		}
		if d.InReestr {
			return "true", true, "В реестре", true
		}
		return "false", false, "Не в реестре", true
	case facetHasPrice:
		if docPriced(d) {
			return hasPriceYes, hasPriceYes, hasPriceLabels[hasPriceYes], true
		}
		return hasPriceNo, hasPriceNo, hasPriceLabels[hasPriceNo], true
	case facetDelivery:
		v := docDelivery(d)
		if v == "" {
			return "", nil, "", false
		}
		return v, v, deliveryLabels[v], true
	}
	return "", nil, "", false
}

// licenseFacets — фасеты по семействам выдачи до фильтров. perVariant — счёт позиций (без группировки), иначе
// семейств. named — фасеты, названные в запросе (подъём наверх). hardware — выдача оборудования (другой порядок).
func licenseFacets(lx *searchLexicon, fams []*familyHit, fs []licFilter, named map[string]bool, hardware, perVariant bool) []out.Facet {
	order := softwareFacetOrder
	if hardware {
		order = hardwareFacetOrder
	}
	active := map[string]bool{}
	for _, f := range fs {
		active[f.key] = true
	}
	var res []out.Facet
	for _, key := range order {
		f := out.Facet{Key: key, Label: licenseFacetLabels[key], Type: "enum", Values: []out.FacetValue{}}
		if key == facetPrice {
			f.Type, f.Unit = "number", "₽"
			for _, fh := range fams {
				for _, d := range fh.heads {
					if !docPriced(d) || !passAll(fs, d, key) {
						continue
					}
					if f.Min == nil || *d.PriceRub < *f.Min {
						f.Min = ptrF(*d.PriceRub)
					}
					if f.Max == nil || *d.PriceRub > *f.Max {
						f.Max = ptrF(*d.PriceRub)
					}
				}
			}
			if f.Min != nil && (*f.Max > *f.Min || active[key]) {
				res = append(res, f)
			}
			continue
		}
		if key == facetReestr {
			f.Type = "bool"
		}
		counts := map[string]int{}
		qtyMetrics := quantityMetrics{}
		values := map[string]any{}
		labels := map[string]string{}
		for _, fh := range fams {
			seen := map[string]bool{}
			for _, d := range fh.heads {
				if !passAll(fs, d, key) {
					continue
				}
				sk, v, l, ok := facetValueOf(lx, key, d)
				if !ok || (!perVariant && seen[sk]) {
					continue
				}
				seen[sk] = true
				counts[sk]++
				values[sk], labels[sk] = v, l
				if key == facetQuantity {
					qtyMetrics.add(sk, d)
				}
			}
		}
		if key == facetQuantity {
			qtyMetrics.label(labels)
		}
		if len(counts) == 0 || len(counts) < 2 && !active[key] {
			continue // одно значение у всех — выбирать не из чего
		}
		keys := make([]string, 0, len(counts))
		for k := range counts {
			keys = append(keys, k)
		}
		sortFacetKeys(key, keys, counts, labels)
		for _, k := range keys {
			f.Values = append(f.Values, out.FacetValue{Value: values[k], Count: counts[k], Label: labels[k]})
		}
		res = append(res, f)
	}
	// Подъём фасетов, названных в запросе, с сохранением порядка категории внутри групп.
	sort.SliceStable(res, func(i, j int) bool { return named[res[i].Key] && !named[res[j].Key] })
	return res
}

// sortFacetKeys — числа по возрастанию (бессрочная — после сроков), вендоры по числу семейств, остальное по подписи.
func sortFacetKeys(key string, keys []string, counts map[string]int, labels map[string]string) {
	sort.Slice(keys, func(i, j int) bool {
		switch key {
		case facetQuantity, facetDuration:
			a, _ := strconv.Atoi(keys[i])
			b, _ := strconv.Atoi(keys[j])
			if key == facetDuration && (a == 0) != (b == 0) {
				return b == 0
			}
			return a < b
		case facetVendor, facetCategory:
			if counts[keys[i]] != counts[keys[j]] {
				return counts[keys[i]] > counts[keys[j]]
			}
		case facetHasPrice, facetReestr:
			return keys[i] > keys[j] // with_price / true — первыми
		}
		return labels[keys[i]] < labels[keys[j]]
	})
}

// filterLabel — «Срок: 1 год» для трассы и отказов.
func filterLabel(f licFilter) string {
	return fmt.Sprintf("%s %s %s", f.key, f.op, f.str)
}

// quantityMetrics — метрики позиций для каждого значения фасета «Количество»: число без единицы непонятно
// (места? сотрудники? штуки?), поэтому подпись — «10 устройств», «5 пользователей»; метрики разные или
// не указаны — «10 мест».
type quantityMetrics map[string]map[string]bool

func (q quantityMetrics) add(key string, d indexDoc) {
	m := ""
	if d.LicenseMetric != nil {
		m = *d.LicenseMetric
	}
	if q[key] == nil {
		q[key] = map[string]bool{}
	}
	q[key][m] = true
}

func (q quantityMetrics) label(labels map[string]string) {
	// Одна метрика на весь фасет (позиции без метрики не мешают) — «10 устройств»; метрики разные — у всех
	// значений «10 мест», чтобы рядом не стояли «3 устройства» и «4 места». Выбор в «Метрике» уточняет подписи.
	metric := ""
	for _, ms := range q {
		for m := range ms {
			switch {
			case m == "":
			case metric == "":
				metric = m
			case metric != m:
				metric = "-"
			}
		}
	}
	if _, ok := metricForms[metric]; !ok || metric == "" {
		metric = ""
	}
	for k := range q {
		n, err := strconv.Atoi(k)
		if err != nil {
			continue
		}
		if metric != "" {
			labels[k] = quantityLabel(n, metric)
		} else {
			labels[k] = fmt.Sprintf("%d %s", n, plural(n, [3]string{"место", "места", "мест"}))
		}
	}
}
