package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"unicode"

	"kuznica/api/domain"
)

// Сверка ответа модели со справочниками каталога. Модель может назвать несуществующего вендора,
// перепутать вендора с происхождением («отечественный») или придумать количество — всё это не доходит
// до поиска, а пишется в dropped с причиной.

// Причины отбрасывания (DroppedField.Reason).
const (
	dropUnknownKind      = "unknown_kind"
	dropUnknownCategory  = "unknown_category"
	dropUnknownVendor    = "unknown_vendor"
	dropNotInCatalog     = "not_in_catalog"
	dropOriginWord       = "origin_not_vendor"
	dropReferenceVendor  = "reference_vendor"
	dropUnknownField     = "unknown_field"
	dropInvalidValue     = "invalid_value"
	dropNotInQuery       = "not_in_query"
	dropUnsupportedValue = "unsupported_value"
)

// rawIntent — ответ модели как есть (ARCHITECTURE §2.3). Значения — any: модель может прислать число
// строкой и наоборот, это не ошибка схемы.
type rawIntent struct {
	Intent           any            `json:"intent"`
	ReferenceProduct any            `json:"reference_product"`
	Category         any            `json:"category"`
	Vendor           any            `json:"vendor"`
	Origin           any            `json:"origin"`
	Constraints      map[string]any `json:"constraints"`
	Preferences      map[string]any `json:"preferences"`
	Unresolved       any            `json:"unresolved"`
}

var intentFence = regexp.MustCompile("(?s)^```[a-zA-Z]*\\s*(.*?)\\s*```$")

// decodeIntent — снять обёртку ``` и разобрать JSON. Ошибка схемы: не JSON-объект, нет строкового
// intent, constraints/preferences не объекты.
func decodeIntent(content string) (*rawIntent, error) {
	s := strings.TrimSpace(content)
	if m := intentFence.FindStringSubmatch(s); m != nil {
		s = m[1]
	}
	if strings.HasPrefix(s, "[") {
		return nil, intentSchemaErr("массив вместо объекта")
	}
	if i, j := strings.Index(s, "{"), strings.LastIndex(s, "}"); i >= 0 && j > i {
		s = s[i : j+1]
	}
	if s == "" {
		return nil, intentSchemaErr("пустой ответ")
	}
	var probe map[string]json.RawMessage
	if err := json.Unmarshal([]byte(s), &probe); err != nil {
		return nil, intentSchemaErr("не JSON-объект")
	}
	var r rawIntent
	if err := json.Unmarshal([]byte(s), &r); err != nil {
		return nil, intentSchemaErr("поля не того типа: constraints и preferences — объекты")
	}
	if k, ok := r.Intent.(string); !ok || strings.TrimSpace(k) == "" {
		return nil, intentSchemaErr("нет поля intent")
	}
	return &r, nil
}

// intentFamily — семейство продукта для сверки референса.
type intentFamily struct {
	ID, Name, VendorID, CategoryID string
	tokens                         []string
}

// intentDict — справочники для промпта и сверки, построенные из одного снимка каталога.
type intentDict struct {
	version      string
	systemPrompt string
	categories   map[string]string        // id → название
	vendors      map[string]domain.Vendor // id → вендор
	families     map[string]intentFamily  // id → семейство
	familyList   []intentFamily           // по id, для поиска по токенам
	byName       map[string]map[string]string
	aliases      map[string]domain.Alias // нормализованный алиас → алиас
}

// newIntentDict — справочники и системный промпт с подставленными категориями и вендорами.
func newIntentDict(snap *domain.CatalogSnapshot, promptText string) *intentDict {
	d := &intentDict{
		version:    snap.Version,
		categories: map[string]string{},
		vendors:    map[string]domain.Vendor{},
		families:   map[string]intentFamily{},
		byName:     map[string]map[string]string{"category": {}, "vendor": {}, "family": {}},
		aliases:    map[string]domain.Alias{},
	}
	for _, c := range snap.Categories {
		d.categories[c.ID] = c.Name
		d.byName["category"][intentNorm(c.Name)] = c.ID
	}
	for _, v := range snap.Vendors {
		d.vendors[v.ID] = v
		d.byName["vendor"][intentNorm(v.Name)] = v.ID
	}
	for _, p := range snap.Products {
		if p.FamilyID == "" {
			continue
		}
		if _, ok := d.families[p.FamilyID]; ok {
			continue
		}
		name := p.FamilyName
		if name == "" {
			name = p.Title
		}
		f := intentFamily{ID: p.FamilyID, Name: name, VendorID: p.VendorID, CategoryID: p.CategoryID, tokens: strings.Fields(intentNorm(name))}
		d.families[f.ID] = f
		d.byName["family"][intentNorm(name)] = f.ID
	}
	for _, f := range d.families {
		d.familyList = append(d.familyList, f)
	}
	sort.Slice(d.familyList, func(i, j int) bool { return d.familyList[i].ID < d.familyList[j].ID })
	for _, a := range snap.Aliases {
		d.aliases[intentNorm(a.Alias)] = a
	}

	var cats, vends []string
	for id, name := range d.categories {
		cats = append(cats, id+" — "+name)
	}
	for id, v := range d.vendors {
		vends = append(vends, id+" — "+v.Name)
	}
	sort.Strings(cats)
	sort.Strings(vends)
	d.systemPrompt = strings.NewReplacer("{{CATEGORIES}}", strings.Join(cats, "\n"), "{{VENDORS}}", strings.Join(vends, "\n")).Replace(promptText)
	return d
}

// intentNorm — нижний регистр, ё → е, всё кроме букв и цифр — пробел.
func intentNorm(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "ё", "е")
	return strings.Join(strings.FieldsFunc(s, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }), " ")
}

// intentOriginWords — «происхождение», которое модель путает с вендором.
var intentOriginWords = map[string]bool{
	"отечественный": true, "отечественное": true, "отечественная": true, "российский": true, "российское": true,
	"российская": true, "русский": true, "ru": true, "russia": true, "russian": true, "domestic": true,
	"россия": true, "рф": true,
}

// resolve — id по id, названию или алиасу справочника kind (category | vendor | family).
func (d *intentDict) resolve(kind, v string) string {
	n := intentNorm(v)
	if n == "" {
		return ""
	}
	switch kind {
	case "category":
		if _, ok := d.categories[v]; ok {
			return v
		}
	case "vendor":
		if _, ok := d.vendors[v]; ok {
			return v
		}
	case "family":
		if _, ok := d.families[v]; ok {
			return v
		}
	}
	if id := d.byName[kind][n]; id != "" {
		return id
	}
	if a, ok := d.aliases[n]; ok && a.Target == kind && a.TargetID != "" {
		return a.TargetID
	}
	return ""
}

// resolveFamily — семейство-референс: id, название, алиас, затем по токенам (все слова референса есть в
// названии семейства; из нескольких — самое короткое название). «Photoshop» → adobe-photoshop.
func (d *intentDict) resolveFamily(ref string) string {
	if id := d.resolve("family", ref); id != "" {
		return id
	}
	toks := strings.Fields(intentNorm(ref))
	// Слова-алиасы внутри референса: «фотошоп cc» → adobe-photoshop.
	for _, t := range toks {
		if a, ok := d.aliases[t]; ok && a.Target == "family" && a.TargetID != "" {
			if _, known := d.families[a.TargetID]; known {
				return a.TargetID
			}
		}
	}
	if len(toks) == 0 {
		return ""
	}
	best, bestLen := "", 0
	for _, f := range d.familyList {
		if !intentContainsAll(f.tokens, toks) {
			continue
		}
		if best == "" || len(f.tokens) < bestLen {
			best, bestLen = f.ID, len(f.tokens)
		}
	}
	return best
}

func intentContainsAll(have, want []string) bool {
	set := make(map[string]bool, len(have))
	for _, h := range have {
		set[h] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

// validate — ответ модели после сверки со справочниками и с текстом запроса.
func (d *intentDict) validate(r rawIntent, normalized string) domain.Intent {
	in := domain.Intent{Constraints: []domain.AttrConstraint{}, Preferences: []domain.AttrConstraint{},
		Unresolved: []string{}, Dropped: []domain.DroppedField{}}
	drop := func(field, value, reason string) {
		in.Dropped = append(in.Dropped, domain.DroppedField{Field: field, Value: value, Reason: reason})
	}

	kind := strings.ToLower(strings.TrimSpace(intentStr(r.Intent)))
	in.Kind = domain.IntentFindProduct
	if intentContains(domain.IntentKinds, kind) {
		in.Kind = kind
	} else {
		drop("intent", kind, dropUnknownKind)
	}

	var ref intentFamily
	if v := intentStr(r.ReferenceProduct); v != "" {
		if id := d.resolveFamily(v); id != "" {
			in.ReferenceFamily = id
			ref = d.families[id]
		} else {
			drop("reference_product", v, dropNotInCatalog)
		}
	}
	if v := intentStr(r.Category); v != "" {
		if id := d.resolve("category", v); id != "" {
			in.CategoryID = id
		} else {
			drop("category", v, dropUnknownCategory)
		}
	}
	if in.CategoryID == "" && in.Kind == domain.IntentFindAlternative && ref.CategoryID != "" {
		in.CategoryID = ref.CategoryID
	}
	if v := intentStr(r.Vendor); v != "" {
		switch id := d.resolve("vendor", v); {
		case intentOriginWords[intentNorm(v)]:
			in.Origin = "ru"
			drop("vendor", v, dropOriginWord)
		case id == "":
			drop("vendor", v, dropUnknownVendor)
		case in.Kind == domain.IntentFindAlternative && id == ref.VendorID:
			drop("vendor", v, dropReferenceVendor) // аналог ищут у других вендоров
		default:
			in.VendorID = id
		}
	}
	if v := strings.ToLower(intentStr(r.Origin)); v != "" {
		switch {
		case v == "ru" || intentOriginWords[intentNorm(v)]:
			in.Origin = "ru"
		default:
			drop("origin", v, dropUnsupportedValue)
		}
	}

	seen := map[string]bool{}
	for _, part := range []struct {
		src  map[string]any
		dst  *[]domain.AttrConstraint
		name string
	}{{r.Constraints, &in.Constraints, "constraints"}, {r.Preferences, &in.Preferences, "preferences"}} {
		keys := make([]string, 0, len(part.src))
		for k := range part.src {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			v := part.src[k]
			if v == nil {
				continue
			}
			field := intentField(k)
			val, reason := intentAttr(field, v, normalized)
			switch {
			case field == "":
				drop(part.name+"."+k, intentStr(v), dropUnknownField)
			case reason == attrSkip:
				// null, пустое или «false» — условия нет
			case reason != "":
				drop(field, intentStr(v), reason)
			case seen[field]:
				// уже есть в обязательных — мягкое условие то же поле не дублирует
			default:
				seen[field] = true
				if field == domain.FieldOrigin {
					in.Origin = "ru"
				}
				*part.dst = append(*part.dst, domain.AttrConstraint{Field: field, Value: val})
			}
		}
	}

	for _, u := range intentStrs(r.Unresolved) {
		if len(in.Unresolved) >= 8 {
			break
		}
		if u = intentTruncate(strings.TrimSpace(u), 80); u != "" {
			in.Unresolved = append(in.Unresolved, u)
		}
	}
	return in
}

// intentField — имя поля условия; "" — поле неизвестно.
func intentField(k string) string {
	switch strings.ToLower(strings.TrimSpace(k)) {
	case "license_quantity", "quantity":
		return domain.FieldLicenseQuantity
	case "license_metric", "metric":
		return domain.FieldLicenseMetric
	case "license_type", "type":
		return domain.FieldLicenseType
	case "license_duration_months", "license_duration", "duration_months", "duration":
		return domain.FieldLicenseDuration
	case "origin":
		return domain.FieldOrigin
	case "renewal":
		return domain.FieldRenewal
	}
	return ""
}

var (
	intentMetricSynonyms = map[string]string{
		"device": domain.MetricDevice, "devices": domain.MetricDevice, "pc": domain.MetricDevice, "computer": domain.MetricDevice,
		"user": domain.MetricUser, "users": domain.MetricUser, "server": domain.MetricServer, "servers": domain.MetricServer,
		"core": domain.MetricCore, "cores": domain.MetricCore, "org": domain.MetricOrg, "organization": domain.MetricOrg,
	}
	intentLicenseTypes = map[string]string{
		"subscription": domain.LicenseSubscription, "perpetual": domain.LicensePerpetual, "renewal": domain.LicenseRenewal,
		"upgrade": domain.LicenseUpgrade, "oem": domain.LicenseOEM, "cloud": domain.LicenseCloud,
	}
	intentDurationWords = []string{"год", "лет", "мес", "year", "month"}
)

// attrSkip — значение пустое: условия нет, отбрасывать нечего.
const attrSkip = "skip"

// intentAttr — значение условия в каноническом виде; reason != "" — отброшено.
func intentAttr(field string, v any, normalized string) (string, string) {
	s := strings.ToLower(strings.TrimSpace(intentStr(v)))
	if s == "" || s == "null" {
		return "", attrSkip
	}
	switch field {
	case domain.FieldLicenseQuantity:
		n, ok := intentInt(v)
		if !ok || n < 1 || n > 100000 {
			return "", dropInvalidValue
		}
		if !intentNumberInQuery(normalized, n) {
			return "", dropNotInQuery // количество, которого нет в запросе, — выдумка модели
		}
		return strconv.Itoa(n), ""
	case domain.FieldLicenseMetric:
		if m, ok := intentMetricSynonyms[s]; ok {
			return m, ""
		}
		return "", dropInvalidValue
	case domain.FieldLicenseType:
		if t, ok := intentLicenseTypes[s]; ok {
			return t, ""
		}
		return "", dropInvalidValue
	case domain.FieldLicenseDuration:
		n, ok := intentInt(v)
		if !ok || n < 1 || n > 120 {
			return "", dropInvalidValue
		}
		for _, w := range intentDurationWords {
			if strings.Contains(normalized, w) {
				return strconv.Itoa(n), ""
			}
		}
		return "", dropNotInQuery
	case domain.FieldOrigin:
		if s == "ru" || intentOriginWords[intentNorm(s)] {
			return "ru", ""
		}
		return "", dropUnsupportedValue
	case domain.FieldRenewal:
		if b, ok := v.(bool); ok {
			if b {
				return "true", ""
			}
			return "", attrSkip
		}
		if s == "true" || s == "yes" || s == "да" {
			return "true", ""
		}
		return "", attrSkip
	}
	return "", dropUnknownField
}

var intentDigits = regexp.MustCompile(`\d+`)

// intentNumberInQuery — число n написано в запросе цифрами.
func intentNumberInQuery(q string, n int) bool {
	for _, m := range intentDigits.FindAllString(q, -1) {
		if v, err := strconv.Atoi(m); err == nil && v == n {
			return true
		}
	}
	return false
}

func intentStr(v any) string {
	switch x := v.(type) {
	case nil:
		return ""
	case string:
		return strings.TrimSpace(x)
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case bool:
		return strconv.FormatBool(x)
	default:
		b, _ := json.Marshal(x)
		return string(b)
	}
}

func intentStrs(v any) []string {
	switch x := v.(type) {
	case []any:
		out := make([]string, 0, len(x))
		for _, e := range x {
			if s := intentStr(e); s != "" {
				out = append(out, s)
			}
		}
		return out
	case string:
		if x != "" {
			return []string{x}
		}
	}
	return nil
}

func intentInt(v any) (int, bool) {
	switch x := v.(type) {
	case float64:
		if x != float64(int(x)) {
			return 0, false
		}
		return int(x), true
	case string:
		n, err := strconv.Atoi(strings.TrimSpace(x))
		return n, err == nil
	}
	return 0, false
}

func intentContains(list []string, v string) bool {
	for _, x := range list {
		if x == v {
			return true
		}
	}
	return false
}

// mergeIntent — разбор правил дополняется разбором модели. Правила детерминированы и сверены по
// построению, поэтому их поля приоритетнее: модель только заполняет пустое. Вид намерения — от модели
// (правила его не определяют надёжно), если правила не увидели продление.
func mergeIntent(rules, m domain.Intent) domain.Intent {
	out := rules
	out.Kind = m.Kind
	if rules.Kind == domain.IntentRenewal || m.Kind == "" {
		out.Kind = rules.Kind
	}
	if out.Kind == "" {
		out.Kind = domain.IntentFindProduct
	}
	pick := func(r, l string) string {
		if r != "" {
			return r
		}
		return l
	}
	out.ReferenceFamily = pick(rules.ReferenceFamily, m.ReferenceFamily)
	out.CategoryID = pick(rules.CategoryID, m.CategoryID)
	out.VendorID = pick(rules.VendorID, m.VendorID)
	out.Origin = pick(rules.Origin, m.Origin)

	has := map[string]bool{}
	out.Constraints = append([]domain.AttrConstraint{}, rules.Constraints...)
	out.Preferences = append([]domain.AttrConstraint{}, rules.Preferences...)
	for _, c := range out.Constraints {
		has[c.Field] = true
	}
	for _, c := range out.Preferences {
		has[c.Field] = true
	}
	for _, c := range m.Constraints {
		if !has[c.Field] {
			has[c.Field] = true
			out.Constraints = append(out.Constraints, c)
		}
	}
	for _, c := range m.Preferences {
		if !has[c.Field] {
			has[c.Field] = true
			out.Preferences = append(out.Preferences, c)
		}
	}
	if out.Origin == "ru" && !has[domain.FieldOrigin] {
		out.Constraints = append(out.Constraints, domain.AttrConstraint{Field: domain.FieldOrigin, Value: "ru"})
	}

	// Неразрешённое: что не определили правила, но определила модель, — снимается.
	out.Unresolved = []string{}
	seen := map[string]bool{}
	for _, u := range append(append([]string{}, rules.Unresolved...), m.Unresolved...) {
		if seen[u] || has[u] {
			continue
		}
		seen[u] = true
		out.Unresolved = append(out.Unresolved, u)
	}
	out.Dropped = append(append([]domain.DroppedField{}, rules.Dropped...), m.Dropped...)
	return out
}

// String — короткое описание разбора для отладки тестов (без текста запроса).
func intentSummary(in domain.Intent) string {
	return fmt.Sprintf("%s ref=%s cat=%s vendor=%s origin=%s c=%v p=%v drop=%v src=%s",
		in.Kind, in.ReferenceFamily, in.CategoryID, in.VendorID, in.Origin, in.Constraints, in.Preferences, in.Dropped, in.Source)
}
