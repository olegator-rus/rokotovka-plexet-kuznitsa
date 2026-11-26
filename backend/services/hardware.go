package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"unicode"
	"unicode/utf8"

	"kuznica/api/domain"
)

// Поиск оборудования: парт-номера, характеристики
// «число + единица», операторы и диапазоны, цена, отрицания, «X для Y» по совместимости, фасеты категории.
// Словарь строится один раз на снимок каталога; разбор запроса — правилами, без LLM. Каталог без оборудования
// (контур поиска) — словарь пуст, шаг ничего не меняет.

// hwAttr — атрибут из attribute_definitions (первое определение ключа задаёт подпись и единицу).
type hwAttr struct {
	key, label, unit, typ string
	facet                 bool
	values                []string
}

// hwUnit — ключ характеристики и множитель к её базовой единице («тб» → storage_gb ×1000).
type hwUnit struct {
	key    string
	factor float64
}

// hwEnumRef — значение перечислимого атрибута, которое названо словом запроса («ddr4» → memory_type DDR4).
type hwEnumRef struct {
	key, value string
}

// hwModel — модель системы, к которой подбирают компоненты (model_id позиций и target_model правил).
type hwModel struct {
	id, name, vendor string
	// tokens — слова названия, id и вендора; distinct — слова с цифрой («r740», «dl380», «gen10»).
	tokens   map[string]bool
	distinct map[string]bool
}

// hardwareLexicon — словарь оборудования одного снимка.
type hardwareLexicon struct {
	attrs    map[string]hwAttr
	catAttrs map[string][]string // категория → ключи атрибутов по порядку определения
	units    map[string][]hwUnit // синоним единицы (свёрнутый) → ключи
	enums    map[string][]hwEnumRef
	// mpn — нормализованный парт-номер → позиции; mpnSorted — для поиска по началу.
	mpn       map[string][]string
	mpnSorted []string
	models    []*hwModel
	modelByID map[string]*hwModel
	// hwCats — листовые категории с оборудованием; components — категории, у позиций которых есть правила
	// совместимости (компоненты: память, диски, гарантия).
	hwCats     map[string]bool
	components map[string]bool
	// catWords — слова названий категорий оборудования → категории.
	catWords map[string][]string
	catNames map[string]string
	// docAttrs — строковые атрибуты позиций оборудования (enum и bool), ключ — как в attributes.
	docAttrs map[string]map[string]string
	// products — позиции оборудования по категории (листовой).
	products map[string][]string
	// hwVendors — вендоры, у которых есть оборудование.
	hwVendors map[string]bool
}

// stdUnits — единицы, которые понимаются и без attribute_definitions: каталог без реестра (generic) всё равно
// ищется «16 гб» и «27 дюймов». Реестр каталога их дополняет и переопределяет.
var stdUnits = map[string][]hwUnit{
	"гб": {{"ram_gb", 1}, {"storage_gb", 1}}, "gb": {{"ram_gb", 1}, {"storage_gb", 1}},
	"тб": {{"storage_gb", 1000}}, "tb": {{"storage_gb", 1000}},
	"дюйм": {{"screen_inch", 1}}, "дюйма": {{"screen_inch", 1}}, "дюймов": {{"screen_inch", 1}}, `"`: {{"screen_inch", 1}},
	"гц": {{"refresh_hz", 1}}, "hz": {{"refresh_hz", 1}},
	"порт": {{"ports_count", 1}}, "порта": {{"ports_count", 1}}, "портов": {{"ports_count", 1}},
	"гбит": {{"net_gbps", 1}}, "gbps": {{"net_gbps", 1}},
	"вт": {{"power_w", 1}}, "квт": {{"power_w", 1000}}, "w": {{"power_w", 1}}, "kw": {{"power_w", 1000}},
	"u": {{"form_factor_u", 1}}, "юнит": {{"form_factor_u", 1}}, "юнита": {{"form_factor_u", 1}},
	// Полная мощность ИБП: 1 кВА ≈ 0,9 кВт (коэффициент мощности ИБП каталога: 3 000 ВА — 2 700 Вт).
	"ква": {{"power_w", 900}}, "kva": {{"power_w", 900}}, "ва": {{"power_w", 0.9}}, "va": {{"power_w", 0.9}},
}

// weakUnits — синонимы, которые без контекста оборудования значат другое («2 г», «16 ядер» лицензии).
var weakUnits = setOf("г", "x", "×", "g", "u", "w", "in", "ge", "ядро", "ядра", "ядер", "core", "cores", "гиг", "т", "к", "k")

// enumSynonyms — слова запроса, которые называют значение перечислимого атрибута не его текстом.
var enumSynonyms = map[string]string{"жесткий": "hdd", "жесткие": "hdd", "жесткого": "hdd", "твердотельный": "ssd"}

// newHardwareLexicon — словарь оборудования из снимка; lx — словарь каталога того же снимка.
func newHardwareLexicon(s *domain.CatalogSnapshot, lx *searchLexicon) *hardwareLexicon {
	hx := &hardwareLexicon{
		attrs: map[string]hwAttr{}, catAttrs: map[string][]string{}, units: map[string][]hwUnit{},
		enums: map[string][]hwEnumRef{}, mpn: map[string][]string{}, modelByID: map[string]*hwModel{},
		hwCats: map[string]bool{}, components: map[string]bool{}, catWords: map[string][]string{},
		catNames: map[string]string{}, docAttrs: map[string]map[string]string{}, products: map[string][]string{},
		hwVendors: map[string]bool{},
	}
	for alias, us := range stdUnits {
		hx.units[alias] = append([]hwUnit(nil), us...)
	}
	for _, ad := range s.AttributeDefinitions {
		for _, a := range ad.Attrs {
			if a.Key == "" {
				continue
			}
			hx.catAttrs[ad.CategoryID] = append(hx.catAttrs[ad.CategoryID], a.Key)
			if _, ok := hx.attrs[a.Key]; !ok {
				typ := a.Type
				if typ == "" {
					typ = "number"
				}
				hx.attrs[a.Key] = hwAttr{key: a.Key, label: a.Label, unit: a.Unit, typ: typ, facet: a.Facet, values: a.Values}
			}
			for _, u := range a.UnitAliases {
				hx.addUnit(foldText(u), hwUnit{a.Key, 1})
			}
			for u, f := range a.UnitConvert {
				if f > 0 {
					hx.addUnit(foldText(u), hwUnit{a.Key, f})
				}
			}
			for _, v := range a.Values {
				fv := foldText(v)
				hx.enums[fv] = append(hx.enums[fv], hwEnumRef{a.Key, v})
			}
			if a.Type == "bool" {
				hx.enums[foldText(a.Key)] = append(hx.enums[foldText(a.Key)], hwEnumRef{a.Key, "true"})
			}
		}
	}
	cats := map[string]domain.Category{}
	for _, c := range s.Categories {
		cats[c.ID] = c
		hx.catNames[c.ID] = c.Name
	}
	vendorName := map[string]string{}
	for _, v := range s.Vendors {
		vendorName[v.ID] = v.Name
	}
	modelName := map[string]string{}
	modelVendor := map[string]string{}
	for _, p := range s.Products {
		if p.Kind != domain.KindHardware {
			continue
		}
		hx.hwCats[p.CategoryID] = true
		hx.hwVendors[p.VendorID] = true
		hx.products[p.CategoryID] = append(hx.products[p.CategoryID], p.ID)
		if n := normalizeMPN(p.MPN); len(n) >= 3 {
			hx.mpn[n] = append(hx.mpn[n], p.ID)
		}
		hx.docAttrs[p.ID] = rawAttrs(p.Attributes)
		if p.ModelID != "" && modelName[p.ModelID] == "" {
			modelName[p.ModelID] = p.FamilyName
			modelVendor[p.ModelID] = p.VendorID
		}
	}
	for n := range hx.mpn {
		hx.mpnSorted = append(hx.mpnSorted, n)
	}
	sort.Strings(hx.mpnSorted)
	byProduct := map[string]string{}
	for _, p := range s.Products {
		byProduct[p.ID] = p.CategoryID
	}
	for _, r := range s.CompatibilityRules {
		if _, ok := modelName[r.TargetModel]; !ok {
			modelName[r.TargetModel] = ""
		}
		if c := byProduct[r.ProductID]; c != "" {
			hx.components[c] = true
		}
		if r.FamilyID != "" && lx != nil {
			if c := lx.familyCat[r.FamilyID]; c != "" {
				hx.components[c] = true
			}
		}
	}
	ids := make([]string, 0, len(modelName))
	for id := range modelName {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	for _, id := range ids {
		m := &hwModel{id: id, name: modelName[id], vendor: modelVendor[id], tokens: map[string]bool{}, distinct: map[string]bool{}}
		if m.name == "" {
			m.name = modelTitle(id)
		}
		for _, w := range append(wordsOf(m.name), wordsOf(strings.ReplaceAll(id, "-", " "))...) {
			m.tokens[w] = true
			if hasDigit(w) && strings.IndexFunc(w, unicode.IsLetter) >= 0 || hasDigit(w) && utf8.RuneCountInString(w) >= 3 {
				m.distinct[w] = true
			}
		}
		if m.vendor != "" {
			for _, w := range wordsOf(m.vendor + " " + vendorName[m.vendor]) {
				m.tokens[w] = true
			}
		}
		hx.models = append(hx.models, m)
		hx.modelByID[id] = m
	}
	// Единицы и слова условий — известные слова: опечатка не превращает «порта» в «почта».
	if lx != nil && len(hx.hwCats) > 0 {
		for alias := range hx.units {
			for _, w := range wordsOf(alias) {
				lx.known[w] = true
			}
		}
		for _, c := range unitContext {
			lx.known[c.stem] = true
		}
		for _, w := range []string{"порт", "порта", "портов", "дюйм", "дюйма", "дюймов", "гц", "гб", "тб", "вт", "квт", "гбит",
			"озу", "оперативной", "оперативная", "аплинк", "ибп", "рублей", "руб", "тыс", "дешевле", "дороже",
			"закупка", "закупки", "закупить", "закупаем", "закупку"} {
			lx.known[w] = true
		}
	}
	for c := range hx.hwCats {
		for _, w := range wordsOf(hx.catNames[c]) {
			if utf8.RuneCountInString(w) >= 4 && !lexStopWords[w] {
				hx.catWords[w] = append(hx.catWords[w], c)
			}
		}
	}
	return hx
}

func (hx *hardwareLexicon) addUnit(alias string, u hwUnit) {
	if alias == "" {
		return
	}
	for i, x := range hx.units[alias] {
		if x.key == u.key {
			hx.units[alias][i] = u
			return
		}
	}
	hx.units[alias] = append(hx.units[alias], u)
}

// hasHardware — в каталоге есть оборудование: без него разбор не запускается (поведение контура поиска).
func (hx *hardwareLexicon) hasHardware() bool { return hx != nil && len(hx.hwCats) > 0 }

// modelTitle — читаемое имя модели из id: «dell-poweredge-r740» → «Dell Poweredge R740».
func modelTitle(id string) string {
	parts := strings.Split(id, "-")
	for i, p := range parts {
		if hasDigit(p) {
			parts[i] = strings.ToUpper(p)
		} else if p != "" {
			r, n := utf8.DecodeRuneInString(p)
			parts[i] = string(unicode.ToUpper(r)) + p[n:]
		}
	}
	return strings.Join(parts, " ")
}

// rawAttrs — атрибуты позиции строками (числа и bool — текстом).
func rawAttrs(raw domain.JSONB) map[string]string {
	out := map[string]string{}
	if len(raw) == 0 {
		return out
	}
	var m map[string]any
	if json.Unmarshal(raw, &m) != nil {
		return out
	}
	for k, v := range m {
		switch x := v.(type) {
		case string:
			out[k] = x
		case float64:
			out[k] = strconv.FormatFloat(x, 'f', -1, 64)
		case bool:
			out[k] = strconv.FormatBool(x)
		}
	}
	return out
}

// categoryOfWord — категории оборудования, которые называет слово запроса: синоним («ssd» → диски) или
// общее начало со словом названия категории не короче пяти букв. Из нескольких — с самым длинным общим
// началом, затем с самым коротким названием («сервер» → «Серверы», а не «Серверная память»).
func (hx *hardwareLexicon) categoryOfWord(w string) string {
	if s := hwCategorySynonym(w); s != "" {
		w = s
	}
	best, bestLen, bestWords := "", 0, 0
	for cw, cs := range hx.catWords {
		l := commonPrefix(w, cw)
		if l < 5 && !(l == utf8.RuneCountInString(w) && l >= 3 && strings.HasPrefix(cw, w)) {
			continue
		}
		for _, c := range cs {
			nw := len(wordsOf(hx.catNames[c]))
			if l > bestLen || l == bestLen && (nw < bestWords || nw == bestWords && c < best) {
				best, bestLen, bestWords = c, l, nw
			}
		}
	}
	return best
}

// hwCategoryStems — слова, которые называют категорию оборудования не её названием: основа → слово названия.
var hwCategoryStems = []struct{ stem, word string }{
	{"ssd", "диски"}, {"hdd", "диски"}, {"nvme", "диски"}, {"жестк", "диски"}, {"накопит", "диски"},
	{"ddr", "память"}, {"rdimm", "память"}, {"memory", "память"}, {"оперативк", "память"},
	{"ибп", "бесперебойного"}, {"ups", "бесперебойного"}, {"бесперебойн", "бесперебойного"},
	{"switch", "коммутаторы"}, {"свитч", "коммутаторы"}, {"laptop", "ноутбуки"}, {"notebook", "ноутбуки"},
	{"monitor", "мониторы"}, {"server", "серверы"}, {"warranty", "гарантия"},
}

func hwCategorySynonym(w string) string {
	for _, s := range hwCategoryStems {
		if strings.HasPrefix(w, s.stem) {
			return s.word
		}
	}
	return ""
}

// weakCategoryWord — слово называет категорию оборудования только вместе с другим признаком оборудования:
// «сервер 1с» и «windows server» — это ПО.
func weakCategoryWord(w string) bool {
	return strings.HasPrefix(w, "сервер") || strings.HasPrefix(w, "server") || strings.HasPrefix(w, "памят")
}

func commonPrefix(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return n
}

// mpnLookup — позиции по нормализованному парт-номеру; prefix — по началу (не короче mpnPrefixMin).
func (hx *hardwareLexicon) mpnLookup(norm string) (ids []string, prefix bool) {
	if ids := hx.mpn[norm]; len(ids) > 0 {
		return ids, false
	}
	if utf8.RuneCountInString(norm) < mpnPrefixMin {
		return nil, false
	}
	i := sort.SearchStrings(hx.mpnSorted, norm)
	for ; i < len(hx.mpnSorted) && strings.HasPrefix(hx.mpnSorted[i], norm); i++ {
		ids = append(ids, hx.mpn[hx.mpnSorted[i]]...)
	}
	if len(ids) > mpnPrefixMax {
		return nil, false
	}
	return ids, len(ids) > 0
}

// Пороги детектора парт-номера.
const (
	mpnMinLen    = 4  // смесь букв и цифр от четырёх символов
	mpnPrefixMin = 5  // неполный ввод работает как префикс с пяти символов
	mpnPrefixMax = 20 // префикс, под который подходит больше позиций, — не парт-номер
)

// latinHomoglyphs — кириллица, похожая на латиницу, в парт-номере («Р00924-В21» набран русскими Р и В).
var latinHomoglyphs = map[rune]rune{
	'а': 'a', 'в': 'b', 'е': 'e', 'к': 'k', 'м': 'm', 'н': 'h', 'о': 'o', 'р': 'p', 'с': 'c', 'т': 't',
	'у': 'y', 'х': 'x',
}

// mpnCandidate — токен как парт-номер: кириллические двойники → латиница, нормализация; "" — не похож
// (нет цифры или буквы, короче mpnMinLen, есть кириллица без двойника).
func mpnCandidate(t string) string {
	var b strings.Builder
	letters, digits := 0, 0
	for _, r := range t {
		if l, ok := latinHomoglyphs[r]; ok {
			r = l
		}
		switch {
		case r >= 'a' && r <= 'z':
			letters++
		case r >= '0' && r <= '9':
			digits++
		case r == '-' || r == '/' || r == '_' || r == '.':
		default:
			if unicode.IsLetter(r) {
				return ""
			}
		}
		b.WriteRune(r)
	}
	n := normalizeMPN(b.String())
	if letters == 0 || digits == 0 || utf8.RuneCountInString(n) < mpnMinLen {
		return ""
	}
	return n
}

// modelMatch — модель, названная словами zone: все совпавшие слова — из названия модели, среди них есть
// отличительное (с цифрой); из нескольких — с наибольшим числом совпавших слов, ничья — нет модели.
func (hx *hardwareLexicon) modelMatch(zone []string) (*hwModel, []int) {
	var best *hwModel
	var bestIdx []int
	bestScore, tie := 0, false
	for _, m := range hx.models {
		var idx []int
		distinct := false
		for i, t := range zone {
			if m.tokens[t] {
				idx = append(idx, i)
				distinct = distinct || m.distinct[t]
			}
		}
		if !distinct {
			continue
		}
		switch score := len(idx); {
		case score > bestScore:
			best, bestIdx, bestScore, tie = m, idx, score, false
		case score == bestScore:
			tie = true
		}
	}
	if tie {
		return nil, nil
	}
	return best, bestIdx
}

// specValues — значения характеристики key у позиций категории cat ("" — всё оборудование).
func (hx *hardwareLexicon) specValues(lx *searchLexicon, cat, key string) map[float64]int {
	out := map[float64]int{}
	for c, ids := range hx.products {
		if cat != "" && c != cat {
			continue
		}
		for _, id := range ids {
			if v, ok := lx.docs[id].Specs[key]; ok {
				out[v]++
			}
		}
	}
	return out
}

// hwNum — число для чипа и подписи: целое — с разрядами («100 000»), дробное — с запятой («15,6»).
func hwNum(v float64) string {
	if v == math.Trunc(v) && math.Abs(v) < 1e15 {
		s := strconv.FormatInt(int64(v), 10)
		if len(s) <= 4 {
			return s
		}
		var b strings.Builder
		for i, r := range s {
			if i > 0 && (len(s)-i)%3 == 0 {
				b.WriteRune(' ')
			}
			b.WriteRune(r)
		}
		return b.String()
	}
	return strings.ReplaceAll(strconv.FormatFloat(v, 'f', -1, 64), ".", ",")
}

// atLeastKeys — характеристики, где названное число — нижняя граница: «монитор 144 гц» — от 144 Гц.
var atLeastKeys = setOf("refresh_hz")

// fmtValue — значение для поля чипа value: без разрядов, с точкой.
func fmtValue(v float64) string { return strconv.FormatFloat(v, 'f', -1, 64) }

// opSymbol — знак оператора в подписи чипа.
func opSymbol(op string) string {
	switch op {
	case "gte":
		return "≥ "
	case "lte":
		return "≤ "
	case "not":
		return "не "
	}
	return ""
}

// hwEnabled — флаг FEATURE_HARDWARE с учётом переопределения запроса (лестница C0–C3).
func (s *Services) hwEnabled(ctx context.Context) bool { return s.featuresFor(ctx).Hardware }

// String — отладочное описание условия.
func (c hwCond) String() string {
	if c.typ != "number" {
		return fmt.Sprintf("%s %s %s", c.Key, c.Op, c.Str)
	}
	return fmt.Sprintf("%s %s %v", c.Key, c.Op, c.Value)
}
