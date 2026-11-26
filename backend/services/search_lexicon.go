package services

import (
	"encoding/json"
	"math"
	"sort"
	"strings"
	"unicode/utf8"

	"kuznica/api/domain"
)

// Словарь каталога для нормализации и ранжирования (ARCHITECTURE §2.1, §2.5). Строится один раз на снимок
// каталога: слова названий, семейств, вендоров и категорий (по ним исправляются опечатки и раскладка),
// слова описаний (по ним слово считается известным), семейства и их лицензии (выбор варианта под условия
// запроса), связи «аналог». Поиску не нужна база: всё в памяти, каталог маленький.

// searchLexicon — словарь одного снимка каталога.
type searchLexicon struct {
	// core — слова названий, семейств, вендоров, категорий и редакций с частотой: словарь опечаток.
	core map[string]int
	// known — core + слова описаний и атрибутов: слово из этого множества не исправляется.
	known map[string]bool
	docs  map[string]indexDoc
	// families — семейство → его позиции (порядок каталога).
	families     map[string][]string
	familyOrder  []string
	familyName   map[string]string
	familyVendor map[string]string
	familyCat    map[string]string
	// familyWords — значимые слова названия семейства; idf — вес слова по числу семейств с ним.
	familyWords map[string]map[string]bool
	idf         map[string]float64
	vendorName  map[string]string
	// vendorPhrases — слова названия вендора (через пробел) → вендор.
	vendorPhrases map[string]string
	categoryName  map[string]string
	// categoryRoot — категория → корневая категория.
	categoryRoot map[string]string
	// substitutes — семейство → его аналоги из relations (substitute), в обе стороны.
	substitutes map[string][]string
	// substituteOf — то же только в направлении связи: продукт → его замена (AutoCAD → КОМПАС-3D).
	substituteOf map[string][]string
	// complements — семейство → что с ним покупают (relations complement, от from к to), с пояснением.
	complements map[string][]familyLink
	// renewals — семейство → на что оно продлевается (relations renewal, от from к to).
	renewals map[string][]string
	// baseAliases — алиасы снимка: правила версии 1.
	baseAliases []domain.Alias
	// translit — транслит слова → слово каталога другим письмом: «лт» → lt (AutoCAD LT), «antivirus» →
	// антивирус. Только однозначные ключи.
	translit map[string]string
	// categoryWords — основа значимого слова названия категории → категория (однозначно в пределах корня).
	categoryWords map[string]string
	// categoryWordText — основа → слово названия категории целиком.
	categoryWordText map[string]string
	// wordVendors — слово названия семейства → вендоры семейств с этим словом.
	wordVendors map[string]map[string]bool
	// familyCap — сколько мест закрывает самая крупная лицензия семейства; -1 — продаётся поштучно.
	familyCap map[string]int
	// consumer — домашняя линейка: категория или семейство «для дома», либо лицензии только на 1–3 устройства.
	consumer map[string]bool
	// skuCompact — артикул без разделителей и регистра → артикул как в каталоге после foldText («fqc10572» →
	// «fqc-10572»); только артикулы с разделителем и только однозначные (restoreSKUs в search_b3.go).
	skuCompact map[string]string
	// Понимание запросов (search_understanding.go): числа из названий семейств («365»), составные слова через
	// дефис («сканер-вс»), пары слов подряд в названиях, фразы вендоров, которые в названиях чаще означают чужие
	// продукты («kit»), число семейств в категории (у корня — вместе с подкатегориями).
	nameNumbers      map[string]bool
	compoundNames    map[string]bool
	familyBigrams    map[string]bool
	vendorAmbiguous  map[string]bool
	categoryFamilies map[string]int
	// vendorStemPhrases — фраза вендора по основам слов (склонения: «касперского», «эшелона»).
	vendorStemPhrases map[string]string
	// knownStems — основы кириллических слов названий (core): словоформа такого слова — известное слово.
	knownStems map[string]bool
}

// familyLink — связанное семейство и пояснение связи из справочника.
type familyLink struct {
	Family string
	Note   string
}

// lexStopWords — служебные слова: не сущности, не исправляются, не делают запрос «непонятным».
var lexStopWords = setOf(
	"и", "в", "во", "на", "для", "с", "со", "по", "от", "до", "из", "к", "о", "об", "а", "или", "не", "без",
	"под", "при", "за", "the", "for", "and", "of", "with", "как", "что", "чем", "где", "мой", "мне", "нам",
	"нужен", "нужна", "нужно", "нужны", "купить", "куплю", "цена", "стоимость", "лицензия", "лицензии",
	"лицензий", "лицензию", "программа", "программу", "программы", "версия", "версии", "шт", "штук",
	// Слова вопроса с форумов и обращений («какую лицензию выбрать», «стоит ли покупать»): не название товара,
	// не повод объявлять «товара нет» и уводить запрос в модель.
	"какой", "какая", "какое", "какую", "какие", "каких", "каким", "какими", "выбрать", "выбор", "выборе",
	"стоит", "ли", "лучше", "лучший", "посоветовать", "посоветуйте", "подскажите", "можете", "можно", "есть",
	"необходимо", "необходимы", "необходим", "приобретать", "приобрести", "использовать", "использует",
	"используете", "использовали", "правильно", "сейчас", "еще", "ещё", "кроме", "вы", "я", "мы", "его", "их",
	"это", "этот", "сколько", "бы", "же", "чтобы", "если", "что-то", "что-нибудь", "качестве", "вообще", "также", "лицензирование", "лицензирования", "лицензировать",
	// Обороты закупочной документации: «поставка неисключительных прав на использование …».
	"поставка", "поставки", "неисключительных", "неисключительные", "неисключительного", "право", "права", "прав",
	"использование", "использования", "предоставление",
	// Упаковка в названиях из прайсов дистрибьюторов: «x64 RUS», «English MLP», «WW LTU».
	"rus", "x64", "x86", "english", "eng", "mlp", "ltu", "ww",
)

// lexServiceWords — слова условий и намерений из правил атрибутов: известны, хотя в каталоге их может не быть.
var lexServiceWords = setOf(
	"пк", "комп", "компа", "компов", "компа", "компьютер", "компьютера", "компьютеров", "компьютеры",
	"устройство", "устройства", "устройств", "девайс", "девайса", "девайсов", "пользователь", "пользователя",
	"пользователей", "юзер", "юзера", "юзеров", "сотрудник", "сотрудника", "сотрудников", "человек", "ядро",
	"чел", "челов", "человека", "людей", "компы", "компам", "компах", "компами", "компу", "пользователям", "юзерам",
	"ядра", "ядер", "сервер", "сервера", "серверов", "место", "места", "мест", "рабочих", "рабочее", "рабочие",
	"станция", "станции", "станций", "узел", "узла", "узлов", "подключение", "подключения", "подключений",
	"год", "года", "лет", "месяц", "месяца", "месяцев", "бессрочно", "бессрочная", "бессрочный", "бессрочную",
	"навсегда", "подписка", "подписку", "подписки", "продлить", "продление", "продления", "продлению",
	"отечественный", "отечественная", "отечественное", "отечественные", "российский", "российская",
	"российское", "российские", "реестре", "реестр", "аналог", "аналоги", "аналога", "замена", "замену",
	"заменить", "вместо", "альтернатива", "похожее", "похожий",
)

func setOf(words ...string) map[string]bool {
	m := make(map[string]bool, len(words))
	for _, w := range words {
		m[w] = true
	}
	return m
}

// newSearchLexicon — словарь из снимка каталога.
func newSearchLexicon(s *domain.CatalogSnapshot) *searchLexicon {
	lx := &searchLexicon{
		core: map[string]int{}, known: map[string]bool{}, docs: map[string]indexDoc{}, skuCompact: map[string]string{},
		families: map[string][]string{}, familyName: map[string]string{}, familyVendor: map[string]string{},
		familyCat: map[string]string{}, familyWords: map[string]map[string]bool{}, idf: map[string]float64{},
		vendorName: map[string]string{}, vendorPhrases: map[string]string{}, categoryName: map[string]string{},
		categoryRoot: map[string]string{},
		substitutes:  map[string][]string{}, substituteOf: map[string][]string{}, complements: map[string][]familyLink{}, renewals: map[string][]string{}, baseAliases: s.Aliases,
	}
	addCore := func(text string) {
		for _, w := range wordsOf(text) {
			lx.core[w]++
			lx.known[w] = true
		}
	}
	for _, v := range s.Vendors {
		lx.vendorName[v.ID] = v.Name
		addCore(v.Name)
		if p := strings.Join(wordsOf(v.Name), " "); p != "" {
			lx.vendorPhrases[p] = v.ID
		}
		lx.vendorPhrases[strings.Join(wordsOf(v.ID), " ")] = v.ID
	}
	cats := map[string]domain.Category{}
	var catWords []string
	for _, c := range s.Categories {
		cats[c.ID] = c
		lx.categoryName[c.ID] = c.Name
		addCore(c.Name)
		catWords = append(catWords, wordsOf(c.Name)...)
	}
	for _, c := range s.Categories {
		ids, _ := categoryChain(cats, c.ID)
		if len(ids) > 0 {
			lx.categoryRoot[c.ID] = ids[0]
		}
	}
	// generic — слово названия семейства, которое на самом деле называет категорию («антивирус» в «ESET NOD32
	// Антивирус»): по нему семейство не считается названным в запросе.
	catWordSet := map[string]bool{}
	// catPrefix6 — первые 6 букв слов категорий от 6 букв: «антивирусы» и «антивирус» совпадают. Множество, а не
	// перебор слов на каждое слово семейства: на фиде заказчика перебор был квадратичным.
	catPrefix6 := map[string]bool{}
	for _, c := range catWords {
		if utf8.RuneCountInString(c) >= 4 {
			catWordSet[c] = true
		}
		if rc := []rune(c); len(rc) >= 6 {
			catPrefix6[string(rc[:6])] = true
		}
	}
	generic := func(w string) bool {
		if catWordSet[w] {
			return true // «дома», «бизнеса»: слово названия категории, а не продукта
		}
		rw := []rune(w)
		return len(rw) >= 6 && catPrefix6[string(rw[:6])]
	}
	for _, d := range buildDocs(s) {
		lx.docs[d.ProductID] = d
		if _, ok := lx.families[d.FamilyID]; !ok {
			lx.familyOrder = append(lx.familyOrder, d.FamilyID)
			lx.familyName[d.FamilyID] = d.FamilyName
			lx.familyVendor[d.FamilyID] = d.VendorID
			lx.familyCat[d.FamilyID] = d.CategoryID
			ws := map[string]bool{}
			for _, w := range wordsOf(d.FamilyName) {
				if !lexStopWords[w] && !generic(w) {
					ws[w] = true
				}
			}
			lx.familyWords[d.FamilyID] = ws
		}
		lx.families[d.FamilyID] = append(lx.families[d.FamilyID], d.ProductID)
		lx.addSKU(d.SKU)
		addCore(d.Title + " " + d.FamilyName + " " + d.LicenseEdition)
		for _, w := range wordsOf(d.Description + " " + d.Attributes + " " + d.CategoryPath) {
			lx.known[w] = true
		}
	}
	df := map[string]int{}
	for _, ws := range lx.familyWords {
		for w := range ws {
			df[w]++
		}
	}
	n := float64(len(lx.familyWords))
	for w, c := range df {
		lx.idf[w] = math.Log(1 + n/float64(c))
	}
	lx.indexRelations(s.Relations)
	for w := range lexServiceWords {
		lx.known[w] = true
	}
	for w := range lexStopWords {
		lx.known[w] = true
	}
	lx.buildTranslit()
	lx.buildCategoryWords(s.Categories)
	lx.buildScale()
	lx.buildUnderstanding()
	for k, v := range lx.skuCompact {
		if v == "" {
			delete(lx.skuCompact, k) // два артикула с одним сжатым видом — не угадываем
		}
	}
	return lx
}

// indexRelations — карты связей словаря (аналоги, сопутствующие, продления) из набора связей; действующая версия
// справочника связей (контур коммерции, relations.go) пересобирает их без переимпорта.
func (lx *searchLexicon) indexRelations(rels []domain.Relation) {
	lx.substitutes, lx.substituteOf = map[string][]string{}, map[string][]string{}
	lx.complements, lx.renewals = map[string][]familyLink{}, map[string][]string{}
	seen := map[string]bool{}
	for _, r := range rels {
		if r.Type == domain.RelationComplement && r.FromFamily != r.ToFamily && !seen["c:"+r.FromFamily+">"+r.ToFamily] {
			seen["c:"+r.FromFamily+">"+r.ToFamily] = true
			lx.complements[r.FromFamily] = append(lx.complements[r.FromFamily], familyLink{Family: r.ToFamily, Note: r.Note})
		}
		if r.Type == domain.RelationRenewal && r.FromFamily != r.ToFamily && !seen["r:"+r.FromFamily+">"+r.ToFamily] {
			seen["r:"+r.FromFamily+">"+r.ToFamily] = true
			lx.renewals[r.FromFamily] = append(lx.renewals[r.FromFamily], r.ToFamily)
		}
		// Совместимость (compatibility) — не аналог: «KES для Astra Linux» не делает Astra заменой KES.
		if r.Type != domain.RelationSubstitute {
			continue
		}
		if r.FromFamily != r.ToFamily && !seen["s:"+r.FromFamily+">"+r.ToFamily] {
			seen["s:"+r.FromFamily+">"+r.ToFamily] = true
			lx.substituteOf[r.FromFamily] = append(lx.substituteOf[r.FromFamily], r.ToFamily)
		}
		for _, pair := range [][2]string{{r.FromFamily, r.ToFamily}, {r.ToFamily, r.FromFamily}} {
			key := pair[0] + ">" + pair[1]
			if seen[key] || pair[0] == pair[1] {
				continue
			}
			seen[key] = true
			lx.substitutes[pair[0]] = append(lx.substitutes[pair[0]], pair[1])
		}
	}
}

// withRelations — копия словаря с другим набором связей (остальное общее, только чтение).
func (lx *searchLexicon) withRelations(rels []domain.Relation) *searchLexicon {
	cp := *lx
	cp.indexRelations(rels)
	return &cp
}

// isKnown — слово есть в каталоге, словаре правил или служебных словах.
func (lx *searchLexicon) isKnown(w string, rules *searchRules) bool {
	if lx.known[w] {
		return true
	}
	if st := stemWord(w); st != w && lx.knownStems[st] {
		return true // словоформа слова названий: «линком», «антивируса», «бэкапом»
	}
	return rules != nil && rules.words[w]
}

// searchRules — словари одной версии правил: алиасы «как пишут → как в каталоге».
type searchRules struct {
	version int
	// aliases — ключ (слова алиаса через пробел) → алиас; maxWords — самый длинный ключ в словах.
	aliases  map[string]domain.Alias
	maxWords int
	// words — слова ключей и канонических форм алиасов: они известны и служат словарём опечаток.
	words map[string]bool
	// vendorPhrases — слова алиаса вендора и его канонической формы → вендор («dr web» → drweb).
	vendorPhrases map[string]string
	// categories — ключ записи словаря понимания с целью category → категории основной выдачи;
	// successors — записи «преемник» (search_understanding.go).
	categories map[string][]string
	successors map[string]domain.LexiconEntry
	// stemAliases — ключ алиаса по основам слов (stemPhrase) → ключ алиаса: «астры линукс», «касперскому»,
	// «кибер бэкапа» находят «астра линукс», «касперский», «кибер бэкап». "" — две записи с одной основой ведут
	// в разное. vendorStemPhrases — то же для фраз вендоров.
	stemAliases       map[string]string
	vendorStemPhrases map[string]string
}

// newSearchRules — правила из списка алиасов.
func newSearchRules(version int, aliases []domain.Alias) *searchRules {
	r := &searchRules{version: version, aliases: map[string]domain.Alias{}, words: map[string]bool{},
		vendorPhrases: map[string]string{}, stemAliases: map[string]string{}, vendorStemPhrases: map[string]string{}}
	for _, a := range aliases {
		ws := strings.Fields(foldText(a.Alias))
		if len(ws) == 0 || strings.TrimSpace(a.Canonical) == "" {
			continue
		}
		key := strings.Join(ws, " ")
		if _, dup := r.aliases[key]; dup {
			continue
		}
		r.aliases[key] = a
		r.maxWords = max(r.maxWords, len(ws))
		for _, w := range ws {
			r.words[w] = true
		}
		for _, w := range wordsOf(a.Canonical) {
			r.words[w] = true
		}
		if a.Target == domain.AliasTargetVendor && a.TargetID != "" {
			r.vendorPhrases[strings.Join(wordsOf(a.Alias), " ")] = a.TargetID
			r.vendorPhrases[strings.Join(wordsOf(a.Canonical), " ")] = a.TargetID
		}
	}
	for key, a := range r.aliases {
		addStemKey(r.stemAliases, stemPhrase(key), key, func(prev string) bool {
			b := r.aliases[prev]
			return b.Canonical == a.Canonical && b.Target == a.Target && b.TargetID == a.TargetID
		})
	}
	for p, v := range r.vendorPhrases {
		addStemKey(r.vendorStemPhrases, stemPhrase(p), v, func(prev string) bool { return prev == v })
	}
	return r
}

// rulesFromSnapshot — правила из снимка config_versions ({"aliases": [...]}).
func rulesFromSnapshot(rs *domain.RulesSnapshot) (*searchRules, error) {
	var body struct {
		Aliases []domain.Alias `json:"aliases"`
	}
	if len(rs.Rules) > 0 {
		if err := json.Unmarshal(rs.Rules, &body); err != nil {
			return nil, err
		}
	}
	return newSearchRules(rs.Version, body.Aliases), nil
}

// matchFamilies — семейства, названные в запросе: вес совпавших слов названия (idf), остаются лучшие.
// Слово «11» в «windows 11» отличает Windows 11 от Windows 10: числа участвуют наравне со словами.
func (lx *searchLexicon) matchFamilies(tokens []string) []string {
	best := 0.0
	score := map[string]float64{}
	for _, f := range lx.familyOrder {
		sc := 0.0
		for _, t := range tokens {
			for _, w := range wordsOf(t) {
				if lx.familyWords[f][w] {
					sc += lx.idf[w]
				}
			}
		}
		if sc > 0 {
			score[f] = sc
			best = math.Max(best, sc)
		}
	}
	if best < 1.2 {
		return nil
	}
	var out []string
	for _, f := range lx.familyOrder {
		if score[f] >= best-1e-9 {
			out = append(out, f)
		}
	}
	return out
}

// familyCoverage — какая доля названия семейства (по весу idf значимых слов) есть в запросе: 1 — названо
// целиком («kaspersky internet security»), мало — совпало одно общее слово («офис» в «Small Office Security»).
func (lx *searchLexicon) familyCoverage(fid string, tokens []string) float64 {
	total, hit := 0.0, 0.0
	in := map[string]bool{}
	for _, t := range tokens {
		for _, w := range wordsOf(t) {
			in[w] = true
		}
	}
	for w := range lx.familyWords[fid] {
		total += lx.idf[w]
		if in[w] {
			hit += lx.idf[w]
		}
	}
	if total == 0 {
		return 0
	}
	return hit / total
}

// matchVendors — вендоры, названные в запросе: фраза названия вендора среди слов запроса.
func (lx *searchLexicon) matchVendors(tokens []string, rules *searchRules) []string {
	var words []string
	for _, t := range tokens {
		words = append(words, wordsOf(t)...)
	}
	found := map[string]bool{}
	var out []string
	for n := 3; n >= 1; n-- {
		for i := 0; i+n <= len(words); i++ {
			p := strings.Join(words[i:i+n], " ")
			id, ok := lx.vendorPhrases[p]
			if ok && lx.vendorAmbiguous[p] {
				id, ok = "", false // «kit», «universe»: в названиях чаще чужие продукты, вендором не считается
			}
			if !ok && rules != nil {
				id, ok = rules.vendorPhrases[p]
			}
			if !ok {
				id = lx.vendorOfStem(p, rules) // «касперскому», «эшелона»: фраза вендора в другом падеже
				ok = id != ""
			}
			if ok && !found[id] && lx.vendorName[id] != "" {
				found[id] = true
				out = append(out, id)
			}
		}
	}
	sort.Strings(out)
	return out
}

// isPerUnit — позиция продаётся поштучно: нужное число лицензий указывается при заказе.
func isPerUnit(d indexDoc) bool { return strings.Contains(d.Attributes, "purchase: поштучно") }

// addSKU — артикул с разделителем («FQC-10572») в словарь сжатых артикулов; повтор сжатого вида — "" (неоднозначно).
func (lx *searchLexicon) addSKU(sku string) {
	folded := foldText(strings.TrimSpace(sku))
	compact := strings.ToLower(normalizeMPN(sku))
	if compact == folded || len(compact) < 6 || strings.ContainsAny(folded, " ") {
		return
	}
	if prev, ok := lx.skuCompact[compact]; ok && prev != folded {
		lx.skuCompact[compact] = ""
		return
	}
	lx.skuCompact[compact] = folded
}
