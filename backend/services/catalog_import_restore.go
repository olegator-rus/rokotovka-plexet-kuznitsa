package services

import (
	"fmt"
	"math"
	"sort"
	"strings"
	"unicode"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
)

// Восстановление недостающего в чужих данных. Правила — те же, что
// разбирают запрос покупателя (attributes.go): «10 ПК», «1 год», «бессрочная», «продление». Восстанавливается
// только пустое: значения из данных не перезаписываются. Отчёт по полю — в аудит.

// productLicenseWeights — вклад событий в популярность позиции: заказ весомее клика.
var popularityWeights = map[string]float64{"click": 1, "add_to_cart": 2, "price_request": 2, "manager_request": 2, "order": 3}

// editionWords — редакции: часть названия семейства, а не условие лицензии; в лицензию — как edition.
var editionWords = setOf("home", "pro", "professional", "standard", "enterprise", "business", "premium", "plus",
	"ultimate", "basic", "advanced", "extended", "datacenter", "foundation", "education", "academic", "корп", "проф",
	"базовый", "базовая", "стандарт", "стандартный", "стандартная", "расширенный", "расширенная", "профессиональный",
	"профессиональная", "корпоративный", "корпоративная", "премиум", "оптимальный", "максимальный", "лайт", "lite")

// licenseNoise — слова лицензии без смысла для семейства.
var licenseNoise = setOf("лицензия", "лицензии", "лицензий", "лицензию", "license", "licence", "licenses", "электронная",
	"электронный", "электронной", "ключ", "esd", "box", "коробочная", "коробка", "версия", "поставка", "подписка",
	"подписку", "бессрочная", "бессрочно", "бессрочной", "продление", "продления")

// yearLike — число похоже на год выпуска или версию («Windows Server Datacenter 2022 (лицензия …)»): в названии
// позиции это не количество мест.
func yearLike(n int) bool { return n >= 1990 && n <= 2039 }

// titleTokens — слова названия: нижний регистр, ё → е, разделители — всё, кроме букв и цифр.
func titleTokens(s string) []string {
	s = strings.ReplaceAll(strings.ToLower(s), "ё", "е")
	return strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) })
}

// licenseFromText — условия лицензии из названия или описания; consumed — слова, ушедшие в лицензию.
func licenseFromText(text string) (domain.License, map[int]bool, []string) {
	tokens := titleTokens(text)
	a := extractAttrs(tokens)
	var lic domain.License
	consumed := a.consumed
	at := func(i int) string {
		if i < 0 || i >= len(tokens) {
			return ""
		}
		return tokens[i]
	}
	if a.quantity > 0 && !yearLike(a.quantity) {
		q := a.quantity
		lic.Quantity = &q
		if a.metric != "" {
			m := a.metric
			lic.Metric = &m
		}
	}
	for i, t := range tokens {
		switch {
		// «1 сервер» без «на» правила запроса не берут (там это «для сервера»), в названии позиции — количество.
		case lic.Quantity == nil && isNumber(t) && strings.HasPrefix(at(i+1), "сервер"):
			q := atoiSafe(t)
			m := domain.MetricServer
			lic.Quantity, lic.Metric = &q, &m
			consumed[i], consumed[i+1] = true, true
		case lic.Metric == nil && (strings.HasPrefix(t, "организаци") || t == "безлимит" || t == "unlimited"):
			m := domain.MetricOrg
			lic.Metric = &m
			consumed[i] = true
		case t == "oem":
			lic.Type = domain.LicenseOEM
		case t == "saas":
			if lic.Type == "" {
				lic.Type = domain.LicenseCloud
			}
		case t == "upgrade" || (strings.HasPrefix(t, "обновлени") && at(i+1) == "с"):
			if lic.Type == "" {
				lic.Type = domain.LicenseUpgrade
			}
		}
	}
	if a.duration > 0 {
		d := a.duration
		lic.DurationMonths = &d
	}
	switch {
	case a.renewal:
		lic.Type = domain.LicenseRenewal
	case lic.Type != "":
	case a.licType != "":
		lic.Type = a.licType
	case a.duration > 0:
		lic.Type = domain.LicenseSubscription
	case hasLicenseWord(tokens):
		// «Лицензия на 2 ядра», «клиентская лицензия» без срока и подписки — бессрочная.
		lic.Type = domain.LicensePerpetual
	}
	return lic, consumed, tokens
}

func hasLicenseWord(tokens []string) bool {
	for _, t := range tokens {
		if strings.HasPrefix(t, "лицензи") || strings.HasPrefix(t, "license") {
			return true
		}
	}
	return false
}

func isNumber(t string) bool {
	if t == "" {
		return false
	}
	for _, r := range t {
		if !unicode.IsDigit(r) {
			return false
		}
	}
	return len(t) < 6
}

func atoiSafe(t string) int {
	n := 0
	for _, r := range t {
		n = n*10 + int(r-'0')
	}
	return n
}

// editionOf — редакция по словам названия в исходном написании: «Windows 11 Pro» → «Pro».
func editionOf(title string) string {
	for _, w := range strings.FieldsFunc(title, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		if editionWords[strings.ToLower(w)] {
			return w
		}
	}
	return ""
}

// familyKey — нормализованное название без лицензионной части: «Kaspersky Plus, 3 устройства, 1 год» →
// «kaspersky plus». Редакция остаётся: Kaspersky Plus и Premium — разные продукты.
func familyKey(title string) string {
	// Название продукта — до первой запятой: дальше в прайсах идут условия лицензии и варианты поставки
	// («, 10 ПК + 1 файловый сервер, 1 год», «, OEM-версия»). Без запятой — всё название без лицензии.
	if i := strings.Index(title, ", "); i > 0 {
		if k := tokensKey(title[:i]); k != "" {
			return k
		}
	}
	return tokensKey(title)
}

// tokensKey — слова текста без лицензионной части.
func tokensKey(title string) string {
	_, consumed, tokens := licenseFromText(title)
	var keep []string
	for i, t := range tokens {
		if (consumed[i] && isLicenseWord(t)) || licenseNoise[t] {
			continue
		}
		// Предлоги перед лицензионной частью («на 10 ПК», «для 5 устройств»).
		if (t == "на" || t == "для" || t == "до") && (i+1 >= len(tokens) || (consumed[i+1] && isLicenseWord(tokens[i+1])) || licenseNoise[tokens[i+1]]) {
			continue
		}
		keep = append(keep, t)
	}
	return strings.Join(keep, " ")
}

// isLicenseWord — слово лицензионной части: число, единица количества или срока, тип, предлог. Правила
// запроса помечают ушедшим и слово перед числом («на 10 пк»), в названии это может быть редакция.
func isLicenseWord(t string) bool {
	if isNumber(t) || durationUnit(t) > 0 || t == "на" || t == "для" || t == "до" {
		return true
	}
	if _, ok := metricOf(t); ok {
		return true
	}
	return strings.HasPrefix(t, "продл") || strings.HasPrefix(t, "бессрочн") || strings.HasPrefix(t, "подписк")
}

// familyNameFrom — название семейства: начало самого короткого названия до первой запятой или скобки
// с лицензией; если лицензия не отделена — слова ключа в исходном написании.
func familyNameFrom(titles []string, key string) string {
	sort.Slice(titles, func(i, j int) bool { return len(titles[i]) < len(titles[j]) })
	t := titles[0]
	for _, sep := range []string{", ", " — ", " - ", " (", "; "} {
		if i := strings.Index(t, sep); i > 0 && familyKey(t[:i]) == key {
			return strings.TrimSpace(t[:i])
		}
	}
	if familyKey(t) == key && !strings.ContainsAny(t, ",;") {
		return strings.TrimSpace(t)
	}
	// Слова ключа в исходном написании по порядку.
	want := strings.Fields(key)
	var out []string
	for _, w := range strings.FieldsFunc(t, func(r rune) bool { return unicode.IsSpace(r) || r == ',' || r == ';' }) {
		if len(out) == len(want) {
			break
		}
		if tt := titleTokens(w); len(tt) > 0 && tt[0] == want[len(out)] {
			out = append(out, w)
		}
	}
	if len(out) == 0 {
		return t
	}
	return strings.Join(out, " ")
}

// vendorMatcher — вендор по словам названия: имя вендора, его id, алиасы с target vendor.
type vendorMatcher struct {
	phrases []struct {
		tokens []string
		id     string
	}
}

func newVendorMatcher(s *domain.CatalogSnapshot) *vendorMatcher {
	vm := &vendorMatcher{}
	add := func(text, id string) {
		if t := titleTokens(text); len(t) > 0 && len([]rune(strings.Join(t, ""))) >= 2 {
			vm.phrases = append(vm.phrases, struct {
				tokens []string
				id     string
			}{t, id})
		}
	}
	for _, v := range s.Vendors {
		add(v.Name, v.ID)
		add(strings.ReplaceAll(v.ID, "-", " "), v.ID)
	}
	for _, a := range s.Aliases {
		if a.Target == domain.AliasTargetVendor && a.TargetID != "" {
			add(a.Alias, a.TargetID)
			add(a.Canonical, a.TargetID)
		}
	}
	// Длинные фразы первыми: «Лаборатория Касперского» раньше «Касперского».
	sort.SliceStable(vm.phrases, func(i, j int) bool { return len(vm.phrases[i].tokens) > len(vm.phrases[j].tokens) })
	return vm
}

// match — вендор, чья фраза стоит в названии раньше всех.
func (vm *vendorMatcher) match(title string) string {
	tokens := titleTokens(title)
	best, bestPos := "", math.MaxInt
	for _, p := range vm.phrases {
		for i := 0; i+len(p.tokens) <= len(tokens) && i < bestPos; i++ {
			ok := true
			for j, w := range p.tokens {
				if tokens[i+j] != w {
					ok = false
					break
				}
			}
			if ok {
				best, bestPos = p.id, i
				break
			}
		}
	}
	return best
}

// RestoreCatalog — восстановить пустые поля снимка правилами; отчёт по полям для аудита. Подставляется в
// catalogsource.Generic из main.
func RestoreCatalog(s *domain.CatalogSnapshot, events []domain.LogEvent) []catalogsource.FieldReport {
	n := len(s.Products)
	reports := map[string]*catalogsource.FieldReport{}
	field := func(name string) *catalogsource.FieldReport {
		r, ok := reports[name]
		if !ok {
			r = &catalogsource.FieldReport{Field: name, Total: n}
			reports[name] = r
		}
		return r
	}
	for _, f := range []string{"sku", "description", "vendor", "category", "family", "price", "license_type", "license_metric",
		"license_quantity", "license_duration", "license_edition", "origin", "in_reestr", "popularity", "margin"} {
		field(f)
	}

	// Что было в данных — до восстановления.
	flat := true
	for _, p := range s.Products {
		count := func(name string, present bool) {
			if present {
				field(name).FromSource++
			}
		}
		count("sku", p.SKU != "")
		count("description", p.Description != "")
		count("vendor", p.VendorID != "")
		count("category", p.CategoryID != "")
		count("price", p.PriceRub != nil)
		count("license_type", p.License.Type != "")
		count("license_metric", p.License.Metric != nil)
		count("license_quantity", p.License.Quantity != nil)
		count("license_duration", p.License.DurationMonths != nil)
		count("license_edition", p.License.Edition != "")
		count("origin", p.Origin != "")
		count("in_reestr", p.InReestr)
		count("popularity", p.Popularity > 0)
		count("margin", p.Margin > 0)
		if p.FamilyID != "" {
			field("family").FromSource++
			flat = false
		}
	}

	// Вендор из названия по справочнику вендоров и алиасам.
	vm := newVendorMatcher(s)
	for i := range s.Products {
		p := &s.Products[i]
		if p.VendorID == "" {
			if id := vm.match(p.Title); id != "" {
				p.VendorID = id
				field("vendor").Restored++
			}
		}
	}

	// Условия лицензии: сначала название, недостающее — из описания.
	for i := range s.Products {
		p := &s.Products[i]
		l := &p.License
		if l.Type != "" && l.Metric != nil && l.Quantity != nil {
			continue
		}
		for _, text := range []string{p.Title, p.Description} {
			if text == "" {
				continue
			}
			got, _, _ := licenseFromText(text)
			// Из описания — только количество, срок и тип: метрику («для организаций») описание часто
			// упоминает не про лицензию.
			if text != p.Title {
				got.Metric = nil
			}
			if l.Type == "" && got.Type != "" {
				l.Type = got.Type
				field("license_type").Restored++
			}
			if l.Metric == nil && got.Metric != nil {
				l.Metric = got.Metric
				field("license_metric").Restored++
			}
			if l.Quantity == nil && got.Quantity != nil {
				l.Quantity = got.Quantity
				field("license_quantity").Restored++
			}
			if l.DurationMonths == nil && got.DurationMonths != nil {
				l.DurationMonths = got.DurationMonths
				field("license_duration").Restored++
			}
		}
		// Бессрочная без явного слова: тип известен, срока нет — так и остаётся (nil = бессрочная).
		if l.Edition == "" {
			if e := editionOf(p.Title); e != "" {
				l.Edition = e
				field("license_edition").Restored++
			}
		}
	}

	// Семейства: плоский каталог группируется по названию без лицензионной части.
	if flat {
		groups := map[string][]int{}
		for i, p := range s.Products {
			k := familyKey(p.Title)
			if k == "" {
				k = strings.ToLower(p.ID)
			}
			// Один ключ у разных вендоров — разные семейства.
			groups[p.VendorID+"\x00"+k] = append(groups[p.VendorID+"\x00"+k], i)
		}
		used := map[string]bool{}
		keys := make([]string, 0, len(groups))
		for k := range groups {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, gk := range keys {
			idx := groups[gk]
			key := gk[strings.IndexByte(gk, 0)+1:]
			titles := make([]string, 0, len(idx))
			for _, i := range idx {
				titles = append(titles, s.Products[i].Title)
			}
			name := familyNameFrom(titles, key)
			id := catalogsource.Slug(key)
			if id == "" {
				id = "family"
			}
			for base, k := id, 2; used[id]; k++ {
				id = fmt.Sprintf("%s-%d", base, k)
			}
			used[id] = true
			for _, i := range idx {
				s.Products[i].FamilyID, s.Products[i].FamilyName = id, name
				field("family").Restored++
			}
		}
	} else {
		for i := range s.Products {
			p := &s.Products[i]
			if p.FamilyName == "" && p.FamilyID != "" {
				p.FamilyName = familyNameFrom([]string{p.Title}, familyKey(p.Title))
			}
		}
	}
	// Категория и вендор позиции без них — от соседей по семейству.
	byFamily := map[string]*domain.Product{}
	for i := range s.Products {
		p := &s.Products[i]
		if f, ok := byFamily[p.FamilyID]; !ok || (f.CategoryID == "" && p.CategoryID != "") {
			byFamily[p.FamilyID] = p
		}
	}
	for i := range s.Products {
		p := &s.Products[i]
		mate := byFamily[p.FamilyID]
		if mate == nil {
			continue
		}
		if p.CategoryID == "" && mate.CategoryID != "" {
			p.CategoryID = mate.CategoryID
			field("category").Restored++
		}
		if p.VendorID == "" && mate.VendorID != "" {
			p.VendorID = mate.VendorID
			field("vendor").Restored++
		}
	}
	// Происхождение — от вендора.
	origin := map[string]string{}
	for _, v := range s.Vendors {
		origin[v.ID] = v.Origin
	}
	for i := range s.Products {
		if p := &s.Products[i]; p.Origin == "" && origin[p.VendorID] != "" {
			p.Origin = origin[p.VendorID]
			field("origin").Restored++
		}
	}

	// Популярность — из событий, если в данных её нет.
	pop := field("popularity")
	if pop.FromSource == 0 && len(events) > 0 {
		score := map[string]float64{}
		top := 0.0
		for _, e := range events {
			if w := popularityWeights[e.Type]; w > 0 && e.ProductID != "" {
				score[e.ProductID] += w
				top = max(top, score[e.ProductID])
			}
		}
		if top > 0 {
			for i := range s.Products {
				if v := score[s.Products[i].ID]; v > 0 {
					s.Products[i].Popularity = math.Round(math.Log1p(v)/math.Log1p(top)*1000) / 1000
					pop.Restored++
				}
			}
			pop.Note = "из кликов, корзин и заказов журнала событий (вес 1/2/3, логарифм от максимума)"
		}
	}

	out := make([]catalogsource.FieldReport, 0, len(reports))
	for _, r := range reports {
		switch {
		case r.FromSource == r.Total && r.Total > 0:
			r.Status = catalogsource.FieldFromSource
		case r.FromSource == 0 && r.Restored == 0:
			r.Status = catalogsource.FieldAbsent
		case r.FromSource == 0 && r.Restored >= r.Total:
			r.Status = catalogsource.FieldRestored
		default:
			r.Status = catalogsource.FieldPartial
		}
		out = append(out, *r)
	}
	if m := reports["margin"]; m.FromSource == 0 {
		for i := range out {
			if out[i].Field == "margin" {
				out[i].Status = catalogsource.FieldSurrogate
				out[i].Note = "маржи нет в данных: суррогат 0 у всех позиций — вклад маржи в бизнес-слое одинаков и на порядок не влияет"
			}
		}
	}
	for i := range out {
		if out[i].Field == "family" && flat {
			out[i].Note = "каталог плоский: семейства собраны по названию без лицензионной части"
		}
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Field < out[j].Field })
	return out
}
