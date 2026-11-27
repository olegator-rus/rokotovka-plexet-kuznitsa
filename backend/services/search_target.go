package services

import (
	"strings"
	"unicode"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Цель ответа: куда ведёт первый кликабельный элемент. Шесть шагов по порядку:
// совпадение с названием семейства или вендора → есть оффер с ценой и наличием → базовая лицензия выше продлений,
// техподдержки, модулей и курсов → более общее семейство выше редакций → при равенстве семейств одного вендора —
// вендор или вендор + категория второго уровня → при равенстве между вендорами — уточняющие чипы. Число офферов
// семейства не считается. Ручных исключений нет: только слова запроса, названия семейств и вендоров каталога.

// lineMin — со скольких равных семейств одного вендора это линейка: цель — вендор или вендор + категория.
const lineMin = 3

// targetDecision — выбранная цель и то, как она выбрана (шаг answer трассы).
type targetDecision struct {
	target *out.Target
	// families — семейства, которые цель представляет: они получают match_type exact.
	families map[string]bool
	// skuDoc — позиция, названная артикулом (цель sku по артикулу).
	skuDoc *indexDoc
	// covering — семейства выдачи, чьё название покрывает запрос; brandOnly — запрос — только имя вендора.
	covering  []string
	brandOnly bool
	// coverageMiss — семейство, у которого страница ещё не собрана: цель ведёт на SKU (журнал покрытия).
	coverageMiss string
	// clarify — вендоры для уточняющих чипов (равенство между вендорами, общее слово-категория).
	clarify bool
	// lead — главный продукт линейки среди равных семейств (не клиентская лицензия, стандартная редакция,
	// ближайшая версия): первая карточка, когда цель — вендор или вендор + категория. leadNote — оговорка к ней.
	lead, leadNote string
}

// targetInput — что нужно выбору цели.
type targetInput struct {
	rc       rankCtx
	words    []string // значимые слова запроса после нормализации (без служебных и атрибутов)
	fams     []*familyHit
	exactDoc func(indexDoc) bool
	category string // категория, названная словами запроса ("" — нет)
	help     *out.HelpAnswer
}

// queryWords — значимые слова запроса для сравнения с названиями: без служебных слов, атрибутов и отрицаний.
func queryWords(tokens []string, consumed map[int]bool) []string {
	var ws []string
	for i, t := range tokens {
		if consumed[i] || lexStopWords[t] {
			continue
		}
		for _, w := range wordsOf(t) {
			if !lexStopWords[w] {
				ws = append(ws, w)
			}
		}
	}
	return ws
}

// wordMatch — слово запроса есть среди слов названия: целиком или основой от 4 букв («бэкапа» — «бэкап»,
// «серверов» — «сервер»). Числа и короткие слова — только целиком.
func wordMatch(w string, names []string) bool {
	for _, n := range names {
		if n == w {
			return true
		}
		a, b := w, n
		if utf8.RuneCountInString(a) > utf8.RuneCountInString(b) {
			a, b = b, a
		}
		if utf8.RuneCountInString(a) >= 4 && strings.HasPrefix(b, a) && !hasDigit(a) {
			return true
		}
		if sw := stemWord(w); sw != w && utf8.RuneCountInString(sw) >= minStemRunes && sw == stemWord(n) {
			return true // «стандартного» — «стандартная», «бэкапом» — «бэкап»
		}
	}
	return false
}

// vendorWords — слова имени и слага вендора семейства.
func (rc rankCtx) vendorWords(fid string) []string {
	v := rc.lx.familyVendor[fid]
	ws := wordsOf(rc.lx.vendorName[v])
	ws = append(ws, wordsOf(strings.ReplaceAll(v, "-", " "))...)
	return append(ws, strings.ReplaceAll(v, "-", ""))
}

// covers — название семейства покрывает запрос: каждое слово запроса есть в названии (два соседних слова могут
// быть одним словом названия: «max patrol» — MaxPatrol). Слова, которыми назван вендор, можно не находить в
// названии, если в запросе есть и другие слова; запрос из одного имени вендора сверяется с названием буквально.
func (rc rankCtx) covers(fid string, words []string, brandOnly bool) bool {
	names := wordsOf(rc.lx.familyName[fid])
	for i, n := 0, len(names); i+1 < n; i++ {
		names = append(names, names[i]+names[i+1]) // «astralinux», «кибербэкап», «сканервс» — слитно набранное название
	}
	vendor := rc.vendorWords(fid)
	for i := 0; i < len(words); i++ {
		w := words[i]
		switch {
		case wordMatch(w, names):
		case i+1 < len(words) && wordMatch(w+words[i+1], names):
			i++
		case !brandOnly && (rc.ent.vendorTokens[w] || wordMatch(w, vendor)):
		default:
			return false
		}
	}
	return len(words) > 0
}

// brandVendor — запрос целиком называет одного вендора («мтс», «astra linux» = слаг astralinux): его id, иначе "".
func (rc rankCtx) brandVendor(words []string, fams []*familyHit) string {
	if len(words) == 0 {
		return ""
	}
	if len(rc.ent.vendors) == 1 {
		all := true
		for _, w := range words {
			all = all && rc.ent.vendorTokens[w]
		}
		if all {
			return rc.ent.vendors[0]
		}
	}
	joined := strings.Join(words, "")
	for _, fh := range fams {
		v := rc.lx.familyVendor[fh.id]
		if v != "" && (strings.ReplaceAll(v, "-", "") == joined || strings.ReplaceAll(foldText(rc.lx.vendorName[v]), " ", "") == joined) {
			return v
		}
	}
	// Каждое слово запроса — слово имени или слага вендора («мтс» — «МТС Линк (Webinar Group)»; «лаборатория
	// kaspersky» — имя «Лаборатория Касперского» и слаг kaspersky).
	for _, fh := range fams {
		v := rc.lx.familyVendor[fh.id]
		names := rc.vendorWords(fh.id)
		all := v != ""
		for _, w := range words {
			all = all && (contains(names, w) || stemWord(w) != w && wordMatch(w, names)) // «мтс линком»
		}
		if all {
			return v
		}
	}
	return ""
}

// famBuyable — у семейства есть допустимая позиция, которую можно купить сейчас.
func famBuyable(fh *familyHit) bool {
	for _, d := range fh.heads {
		if buyable(d) {
			return true
		}
	}
	return false
}

// docCat2 — категория второго уровня позиции (фид: категория позиции; глубже — второй элемент пути).
func docCat2(d indexDoc) string {
	if len(d.CategoryIDs) >= 2 {
		return d.CategoryIDs[1]
	}
	return d.CategoryID
}

// generalFamily — более общее семейство: значимые слова его названия (без имени вендора) есть в названии каждого
// другого («Kaspersky Small Office Security» общее для «… for Desktops, Mobiles and File Servers»). "" — такого нет.
func (rc rankCtx) generalFamily(ids []string) string {
	words := func(fid string) map[string]bool {
		vendor := map[string]bool{}
		for _, w := range rc.vendorWords(fid) {
			vendor[w] = true
		}
		var ws []string
		for _, w := range wordsOf(rc.lx.familyName[fid]) {
			if !vendor[w] && !lexStopWords[w] && !hasDigit(w) {
				ws = append(ws, w) // номер версии редакцией не делает: «КриптоПро CSP 5.0» общее для «CSP TLS»
			}
		}
		m := map[string]bool{}
		for _, w := range rc.withoutTypePrefix(fid, ws) {
			m[w] = true // «Программа КриптоПро CSP» в «Программах для защиты информации» — это «CSP»
		}
		return m
	}
	for _, c := range ids {
		cw := words(c)
		general := len(cw) > 0
		for _, o := range ids {
			if o == c || !general {
				continue
			}
			ow := words(o)
			if len(ow) <= len(cw) {
				general = false
				break
			}
			for w := range cw {
				if !ow[w] {
					general = false
					break
				}
			}
		}
		if general {
			return c
		}
	}
	return ""
}

// categoryQuery — запрос — общее слово-категория: категория найдена, вендор не назван, и каждое слово запроса — слово
// названия этой категории или её родителя («сервер», «офис» → office, «антивирус»).
func (rc rankCtx) categoryQuery(words []string, cat string) bool {
	if cat == "" || len(rc.ent.vendors) > 0 || len(words) == 0 {
		return false
	}
	for _, w := range words {
		if hasDigit(w) {
			return false // «windows 11»: номер называет продукт, а не категорию
		}
	}
	names := wordsOf(rc.lx.categoryName[cat])
	if root := rc.lx.categoryRoot[cat]; root != "" && root != cat {
		names = append(names, wordsOf(rc.lx.categoryName[root])...)
	}
	for _, w := range words {
		if !wordMatch(w, names) {
			return false
		}
	}
	return true
}

// selectTarget — цель ответа. nil — цели нет (пустая выдача, бессмыслица).
func selectTarget(in targetInput) targetDecision {
	rc := in.rc
	dec := targetDecision{families: map[string]bool{}}
	if rc.ent.nonProduct && gibberish(rc.ent, in.words) && in.help == nil {
		return dec // бессмыслица («asdfgh»): цели нет, даже если разбор модели назвал её «не о товаре»
	}
	if rc.ent.nonProduct || in.help != nil && in.help.Mode == HelpModeAnswer {
		t := &out.Target{Type: out.TargetService, Title: "Помощь менеджера", Reason: "запрос не о товаре"}
		if in.help != nil {
			t.Title, t.URL = in.help.Title, in.help.URL
		}
		dec.target = t
		return dec
	}
	if len(in.fams) == 0 {
		if in.category != "" && rc.categoryQuery(in.words, in.category) {
			dec.target = categoryTarget(rc, in.category, "общее слово совпадает с категорией")
		}
		return dec
	}
	byID := map[string]*familyHit{}
	for _, fh := range in.fams {
		byID[fh.id] = fh
	}

	// 0. Точный артикул.
	if in.exactDoc != nil {
		for _, fh := range in.fams {
			for _, d := range fh.heads {
				if in.exactDoc(d) {
					doc := d
					dec.skuDoc = &doc
					dec.families[fh.id] = true
					dec.target = &out.Target{Type: out.TargetSKU, URL: d.OfferURL, Title: d.Title, Reason: "точный артикул"}
					return dec
				}
			}
		}
	}

	// 1. Совпадение с названием семейства (или вендора).
	brand := rc.brandVendor(in.words, in.fams)
	dec.brandOnly = brand != ""
	coverWith := func(words []string) []string {
		var res []string
		for _, fh := range in.fams {
			// Аналоги отсутствующего товара цель не задают; преемник ушедшего с рынка («акронис» → Кибер Бэкап) —
			// задаёт: о нём и спрашивают, пояснение — в relaxed.
			if (fh.group != domain.GroupAnalog || len(rc.ent.successors) > 0) && rc.covers(fh.id, words, dec.brandOnly) {
				res = append(res, fh.id)
			}
		}
		return res
	}
	covering := coverWith(in.words)
	// Названной версии в каталоге нет («windows server 2025»): продукт — по словам без номера версии, первым —
	// ближайшая версия самого продукта, а не первое по релевантности (клиентская лицензия — допродажа).
	var wantVersions []string
	if len(covering) == 0 && !dec.brandOnly {
		var plain []string
		for _, w := range in.words {
			if isVersionWord(w) {
				wantVersions = append(wantVersions, w)
			} else {
				plain = append(plain, w)
			}
		}
		if len(wantVersions) > 0 && len(plain) > 0 {
			if c := coverWith(plain); len(c) > 0 {
				covering, in.words = c, plain
				dec.leadNote = "названной версии в каталоге нет — показана ближайшая"
			} else {
				wantVersions = nil
			}
		} else {
			wantVersions = nil
		}
	}
	if brand != "" {
		var own []string
		for _, f := range covering {
			if rc.lx.familyVendor[f] == brand {
				own = append(own, f)
			}
		}
		covering = own
	}
	// 2. Есть оффер с ценой и наличием. 3. Базовая лицензия выше продлений, поддержки, модулей и курсов.
	covering = keepIf(covering, func(f string) bool { return famBuyable(byID[f]) })
	// 3. Услуги, курсы и сертификаты техподдержки — ниже продуктов (если о них не просили). Продления — порядок
	// внутри семейства (голова), а не признак семейства: семейство из одних продлений (MaxPatrol EPP) остаётся.
	covering = keepIf(covering, func(f string) bool { return !rc.serviceFamily(f) })
	// Название начинается со слов запроса («365» — Microsoft 365 Family, а не PowerDirector 365 или CorelDRAW
	// «Подписка 365-Day»): запрос называет продукт, а не его признак.
	if !dec.brandOnly {
		covering = keepIf(covering, func(f string) bool { return rc.leads(f, in.words) })
	}
	// Слово запроса — линейка одного вендора («nanocad» — 20 семейств Нанософт и 2 — других): у него не меньше двух
	// третей покрывающих семейств — остаются только его.
	covering = keepIf(covering, func(f string) bool { return ownerShare(rc, covering, rc.lx.familyVendor[f])*3 >= 2*len(covering) })
	dec.covering = covering

	// 4. Более общее семейство выше редакций: общее по словам названия или единственная «платформа» линейки.
	single := ""
	switch {
	case len(covering) == 1:
		single = covering[0]
	case len(covering) > 1:
		single = rc.generalFamily(covering)
		if single == "" {
			single = rc.baseOfLine(covering)
		}
	}
	catQuery := rc.categoryQuery(in.words, in.category)
	if single != "" && (!catQuery || rc.namedBy(single, in.words)) {
		return familyTarget(rc, dec, byID[single], "совпадение с названием семейства")
	}
	if catQuery {
		dec.target = categoryTarget(rc, in.category, "общее слово совпадает с категорией")
		dec.clarify = true
		return dec
	}
	if len(covering) == 0 {
		if brand != "" {
			return vendorTarget(rc, dec, brand, in.fams, "у вендора нет одного явного семейства")
		}
		if !rc.relatesTo(in.fams[0].id, in.words) {
			return dec // совпадение слабое: ни одно слово запроса не из названия, вендора или категории — цели нет
		}
		return familyTarget(rc, dec, in.fams[0], "первое по релевантности")
	}
	// 5. Равенство семейств одного вендора: вендор или вендор + категория второго уровня.
	vendors, cats := map[string]bool{}, map[string]bool{}
	for _, f := range covering {
		vendors[rc.lx.familyVendor[f]] = true
		cats[docCat2(byID[f].head)] = true
	}
	lead := rc.mainOfLine(covering, byID, wantVersions)
	if len(vendors) == 1 && len(covering) < lineMin && !dec.brandOnly {
		// Два равных семейства — ещё не линейка: первое по релевантности, второе — сразу под ним.
		first, reason := covering[0], "совпадение с названием семейства, первое по релевантности"
		if lead != "" {
			first = lead
		} else if wide := rc.widestFamily(covering, byID); wide != "" {
			first, reason = wide, "совпадение с названием семейства, лицензия на больше мест"
		}
		return familyTarget(rc, dec, byID[first], reason)
	}
	if len(vendors) == 1 && lead != "" && !dec.brandOnly && !anyBuyable(covering, byID) {
		// Вся линейка только по запросу («windows server»): страница вендора с категорией купить ничего не даёт —
		// цель и первая карточка — главный продукт (: «Запросить цену» у Windows Server Standard).
		return familyTarget(rc, dec, byID[lead], "линейка только по запросу — главный продукт линейки")
	}
	if len(vendors) == 1 {
		v := rc.lx.familyVendor[covering[0]]
		for _, f := range covering {
			dec.families[f] = true
		}
		dec.lead = lead
		if len(cats) == 1 && !dec.brandOnly {
			return vendorCategoryTarget(rc, dec, v, byID[covering[0]].head)
		}
		return vendorTarget(rc, dec, v, in.fams, "у вендора несколько равных семейств")
	}
	// 6. Равенство между вендорами — первое по релевантности и уточняющие чипы.
	dec.clarify = true
	return familyTarget(rc, dec, byID[covering[0]], "несколько вендоров с похожими семействами — уточните")
}

func keepIf(xs []string, ok func(string) bool) []string {
	var res []string
	for _, x := range xs {
		if ok(x) {
			res = append(res, x)
		}
	}
	if len(res) == 0 {
		return xs // признак не различает кандидатов — шаг пропускается
	}
	return res
}

// familyTarget — цель «семейство»; страница семейства не собрана — SKU головы и промах покрытия.
func familyTarget(rc rankCtx, dec targetDecision, fh *familyHit, reason string) targetDecision {
	dec.families[fh.id] = true
	url := ""
	for _, d := range fh.heads {
		if d.FamilyURL != "" {
			url = d.FamilyURL
			break
		}
	}
	if url == "" && fh.head.OfferURL != "" {
		dec.coverageMiss = fh.id
		dec.target = &out.Target{Type: out.TargetSKU, URL: fh.head.OfferURL, Title: fh.head.Title,
			Reason: reason + "; страница семейства ещё не собрана — ведём на позицию"}
		return dec
	}
	name := rc.lx.familyName[fh.id]
	if name == "" {
		name = fh.head.Title
	}
	dec.target = &out.Target{Type: out.TargetFamily, URL: url, Title: name, Reason: reason}
	return dec
}

func vendorURLOf(fams []*familyHit, vendor string) string {
	for _, fh := range fams {
		if fh.head.VendorID == vendor && fh.head.VendorURL != "" {
			return fh.head.VendorURL
		}
	}
	return ""
}

func vendorTarget(rc rankCtx, dec targetDecision, vendor string, fams []*familyHit, reason string) targetDecision {
	for _, fh := range fams {
		if fh.head.VendorID == vendor && fh.group != domain.GroupAnalog {
			dec.families[fh.id] = true
		}
	}
	dec.target = &out.Target{Type: out.TargetVendor, URL: vendorURLOf(fams, vendor), Title: rc.lx.vendorName[vendor], Reason: reason}
	return dec
}

// vendorCategoryTarget — «вендор + категория»: <vendorUrl>?filter[category][]=<id категории второго уровня>.
func vendorCategoryTarget(rc rankCtx, dec targetDecision, vendor string, head indexDoc) targetDecision {
	cat := docCat2(head)
	url := ""
	if head.VendorURL != "" && cat != "" {
		url = head.VendorURL + "?filter[category][]=" + cat
	}
	title := rc.lx.vendorName[vendor]
	if name := rc.lx.categoryName[cat]; name != "" {
		title += " — " + lowerCategory(name)
	}
	dec.target = &out.Target{Type: out.TargetVendorCategory, URL: url, Title: title,
		Reason: "у вендора несколько равных семейств в одной категории"}
	return dec
}

func categoryTarget(rc rankCtx, cat, reason string) *out.Target {
	return &out.Target{Type: out.TargetCategory, Title: rc.lx.categoryName[cat], Reason: reason}
}

// lowerCategory — название категории внутри заголовка: первая буква строчная, если это не аббревиатура («ПО»).
func lowerCategory(s string) string {
	r := []rune(s)
	if len(r) > 1 && unicode.IsUpper(r[1]) {
		return s
	}
	return lowerFirst(s)
}

// namedBy — запрос называет семейство целиком: каждое значимое слово его названия (кроме имени вендора) есть в
// запросе. Общее слово-категория («антивирус») семейство «Softline Universe (ОС+офис+антивирус)» не называет.
func (rc rankCtx) namedBy(fid string, words []string) bool {
	vendor := rc.vendorWords(fid)
	n := 0
	for _, w := range wordsOf(rc.lx.familyName[fid]) {
		if lexStopWords[w] || contains(vendor, w) {
			continue
		}
		n++
		if !wordMatch(w, words) {
			return false
		}
	}
	return n > 0
}

// leads — значимые слова названия семейства (без имени вендора) начинаются со слов запроса (без слов вендора).
func (rc rankCtx) leads(fid string, words []string) bool {
	vendor := rc.vendorWords(fid)
	var names []string
	for _, w := range wordsOf(rc.lx.familyName[fid]) {
		if !lexStopWords[w] && !contains(vendor, w) && !lineBaseWords[w] {
			names = append(names, w)
		}
	}
	names = rc.withoutTypePrefix(fid, names)
	i := 0
	for _, w := range words {
		if rc.ent.vendorTokens[w] || wordMatch(w, vendor) {
			continue
		}
		if i >= len(names) || !wordMatch(w, names[i:i+1]) {
			return false
		}
		i++
	}
	return i > 0
}

// lineBaseWords — слова названия, которыми семейство называет себя основой линейки («Платформа nanoCAD 26»): такое
// семейство общее для редакций, комплектов и модулей той же линейки.
var lineBaseWords = setOf("платформа", "platform")

// baseOfLine — единственное семейство-основа среди покрывающих («Платформа nanoCAD» среди «nanoCAD комплект …»,
// «nanoCAD GeoniCS» …); "" — такого нет или их несколько.
func (rc rankCtx) baseOfLine(ids []string) string {
	found := ""
	for _, f := range ids {
		for _, w := range wordsOf(rc.lx.familyName[f]) {
			if lineBaseWords[w] {
				if found != "" && found != f {
					return ""
				}
				found = f
			}
		}
	}
	return found
}

// ownerShare — сколько покрывающих семейств у вендора.
func ownerShare(rc rankCtx, ids []string, vendor string) int {
	n := 0
	for _, f := range ids {
		if rc.lx.familyVendor[f] == vendor {
			n++
		}
	}
	return n
}

// withoutTypePrefix — слова названия без начального слова-типа из названия категории семейства: «Операционная
// система Microsoft Windows Server Standard» в «Операционных системах Windows» начинается с «windows server».
func (rc rankCtx) withoutTypePrefix(fid string, names []string) []string {
	cat := rc.lx.familyCat[fid]
	catWords := wordsOf(rc.lx.categoryName[cat])
	if root := rc.lx.categoryRoot[cat]; root != "" && root != cat {
		catWords = append(catWords, wordsOf(rc.lx.categoryName[root])...)
	}
	i := 0
	for i < len(names)-1 {
		typeWord := false
		for _, c := range catWords {
			if utf8.RuneCountInString(c) >= 5 && sameStem(names[i], c) {
				typeWord = true
				break
			}
		}
		if !typeWord {
			break
		}
		i++
	}
	return names[i:]
}

// accessLicenseWords — слово названия семейства, по которому это клиентская лицензия доступа к продукту (CAL), а не
// сам продукт: она — допродажа к нему и первой в линейке не встаёт.
var accessLicenseWords = setOf("cal", "cals")

// defaultEditionStems — начало слова стандартной редакции: среди равных редакций линейки она первая.
var defaultEditionStems = []string{"standard", "стандарт"}

// isVersionWord — слово запроса — номер версии или года выпуска («2025», «2019»).
func isVersionWord(w string) bool {
	if utf8.RuneCountInString(w) != 4 || !(strings.HasPrefix(w, "19") || strings.HasPrefix(w, "20")) {
		return false
	}
	for _, r := range w {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// familyVersion — номер версии семейства: из названия семейства, иначе из названия головы. 0 — не указан.
func (rc rankCtx) familyVersion(fh *familyHit) int {
	for _, s := range []string{rc.lx.familyName[fh.id], fh.head.Title} {
		for _, w := range wordsOf(s) {
			if isVersionWord(w) {
				n := 0
				for _, r := range w {
					n = n*10 + int(r-'0')
				}
				return n
			}
		}
	}
	return 0
}

// mainOfLine — главный продукт среди равных семейств одного вендора: не клиентская лицензия; если названа версия,
// которой нет, — ближайшая по номеру; иначе стандартная редакция. "" — признаки семейства не различают (порядок
// остаётся по релевантности).
func (rc rankCtx) mainOfLine(ids []string, byID map[string]*familyHit, wantVersions []string) string {
	if len(ids) < 2 && len(wantVersions) == 0 {
		return ""
	}
	decided := false
	cands := keepIf(ids, func(f string) bool {
		for _, w := range wordsOf(rc.lx.familyName[f]) {
			if accessLicenseWords[w] {
				return false
			}
		}
		return true
	})
	if len(cands) < len(ids) {
		decided = true
	}
	if len(wantVersions) > 0 {
		want := 0
		for _, r := range wantVersions[0] {
			want = want*10 + int(r-'0')
		}
		best, bestD := "", 1<<30
		for _, f := range cands {
			v := rc.familyVersion(byID[f])
			if v == 0 {
				continue
			}
			d := want - v
			if d < 0 {
				d = -d
			}
			if d < bestD {
				best, bestD = f, d
			}
		}
		if best != "" {
			return best
		}
	}
	for _, f := range cands {
		for _, w := range wordsOf(rc.lx.familyName[f]) {
			for _, st := range defaultEditionStems {
				if strings.HasPrefix(w, st) {
					return f
				}
			}
		}
	}
	if decided || len(wantVersions) > 0 {
		return cands[0]
	}
	return ""
}

// anyBuyable — у одного из семейств есть позиция, которую можно купить сейчас.
func anyBuyable(ids []string, byID map[string]*familyHit) bool {
	for _, f := range ids {
		if famBuyable(byID[f]) {
			return true
		}
	}
	return false
}

// gibberish — все значимые слова запроса неизвестны каталогу и словарю, и среди них нет слова сервисного вопроса.
func gibberish(e queryEntities, words []string) bool {
	if e.noise {
		return true
	}
	if e.significant == 0 || len(e.unknown) < e.significant {
		return false
	}
	for _, w := range words {
		if isNonProductWord(w) || howToWords[w] {
			return false
		}
	}
	return true
}

// serviceStems — начало слова названия семейства-услуги: техподдержка, сертификат поддержки, курс, сервисы.
var serviceStems = []string{"поддержк", "техподдерж", "курс", "обучени", "тренинг", "services", "сервис", "услуг", "support", "сопровожд"}

// serviceFamily — семейство — услуга, курс или сертификат техподдержки, а не продукт: все его позиции такого вида
// (support, course) или название — из слов услуги («Расширенная техническая поддержка», «Softline Expert Services»,
// «Сертификаты технической поддержки»). Если о таком виде прямо просили («техподдержка астра»), — не услуга.
func (rc rankCtx) serviceFamily(fid string) bool {
	if rc.wantKinds[domain.LicenseKindSupport] || rc.wantKinds[domain.LicenseKindCourse] {
		return false
	}
	pids := rc.lx.families[fid]
	if len(pids) > 0 {
		all := true
		for _, pid := range pids {
			k := rc.lx.kindOf(rc.lx.docs[pid])
			all = all && (k == domain.LicenseKindSupport || k == domain.LicenseKindCourse)
		}
		if all {
			return true
		}
	}
	for _, w := range wordsOf(rc.lx.familyName[fid]) {
		for _, st := range serviceStems {
			if strings.HasPrefix(w, st) {
				return true
			}
		}
	}
	return false
}

// seatsOf — сколько мест закрывает базовая лицензия головы семейства: поле количества, иначе число из названия
// позиции («подписка на 6 пользователей»). 0 — не указано.
func seatsOf(fh *familyHit) int {
	if q := fh.head.LicenseQuantity; q != nil && *q > 0 {
		return *q
	}
	return extractAttrs(strings.Fields(foldText(fh.head.Title))).quantity
}

// widestFamily — из равных семейств одного вендора то, чья лицензия закрывает больше мест: оно общее для
// остальных (Microsoft 365 Family на 6 пользователей шире Personal на одного). "" — места не различаются.
// Число офферов семейства здесь не считается.
func (rc rankCtx) widestFamily(ids []string, byID map[string]*familyHit) string {
	best, bestN, tie := "", 0, false
	for _, f := range ids {
		n := seatsOf(byID[f])
		switch {
		case n > bestN:
			best, bestN, tie = f, n, false
		case n == bestN:
			tie = true
		}
	}
	if tie || bestN <= 1 {
		return ""
	}
	return best
}

// relatesTo — хоть одно слово запроса есть в названии семейства, имени вендора или названии его категории (с
// разделом): иначе «первое по релевантности» — случайное совпадение по описанию, и целью оно не становится.
func (rc rankCtx) relatesTo(fid string, words []string) bool {
	names := wordsOf(rc.lx.familyName[fid])
	names = append(names, rc.vendorWords(fid)...)
	cat := rc.lx.familyCat[fid]
	names = append(names, wordsOf(rc.lx.categoryName[cat])...)
	if root := rc.lx.categoryRoot[cat]; root != "" && root != cat {
		names = append(names, wordsOf(rc.lx.categoryName[root])...)
	}
	for _, w := range words {
		if shopCommandWords[w] {
			continue
		}
		if wordMatch(w, names) {
			return true
		}
		for _, n := range names {
			// Составное слово: «видеоконференции» — «Конференц-связь».
			if utf8.RuneCountInString(n) >= 6 && strings.Contains(w, n) || utf8.RuneCountInString(w) >= 6 && strings.Contains(n, w) {
				return true
			}
		}
	}
	return false
}
