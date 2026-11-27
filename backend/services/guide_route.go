package services

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Три состояния строки поиска (SPEC §1) и извлечение условий правилами. Режим выбирается по смыслу: вопрос и
// «подбери» — подбор; общий запрос, категория или число мест без полных условий — поиск с предложением; артикул,
// название продукта, полные условия — обычный поиск. Модель здесь не вызывается: неоднозначное — «поиск +
// предложение», покупатель сам решает, нажать ли «Помочь выбрать».

// Режимы строки.
const (
	routeSearch      = "search"
	routeSearchOffer = "search_offer"
	routeGuide       = "guide"
)

// GuideOfferText — строка-предложение над выдачей.
const GuideOfferText = "Могу помочь выбрать редакцию и посчитать лицензии."

var (
	reGuideAsk = regexp.MustCompile(`(чем\s+.*отлича|отличается|отличия|разниц|сравни|сравнить|срав[а-яё]*|что\s+(взять|выбрать|купить|лучше)|как(ой|ую|ое|ие)\s+(взять|выбрать)|подбери|подобрать|помоги\s+(выбрать|подобрать)|самы[йе]\s+дешев|подешевле|что\s+подойд|что\s+нужно\s+для)`)
	reSKU      = regexp.MustCompile(`^[a-z]{1,4}[0-9][a-z0-9\-]{4,}$`)
	reCount    = regexp.MustCompile(`(\d{1,6})\s*(-?х|-?ти|-?и)?\s*(пк|комп(?:ьютер[а-яёa-z]*|ов|а|ы|ам|ах)?|рабоч[а-яёa-z]*\s+(мест[а-яёa-z]*|станц[а-яёa-z]*)|рабоч[а-яёa-z]*|мест[а-яёa-z]*|узл[а-яёa-z]*|устройств[а-яёa-z]*|машин[а-яёa-z]*|сервер[а-яёa-z]*|пользовател[а-яёa-z]*|сотрудник[а-яёa-z]*|человек|подключени[а-яёa-z]*|лицензи[а-яёa-z]*|шт[а-яёa-z]*|хост[а-яёa-z]*|ip)`)
	reCountRev = regexp.MustCompile(`(пк|комп(?:ьютер[а-яёa-z]*|ов|а|ы|ам|ах)?|мест[а-яёa-z]*|узл[а-яёa-z]*|сервер[а-яёa-z]*|пользовател[а-яёa-z]*)\s+(будет\s+)?(\d{1,6})`)
	reBudet    = regexp.MustCompile(`будет\s+(\d{1,6})`)
	// «вообще на 300.» — число без объекта в конце фразы: относится к главному объекту подбора.
	reBareCount = regexp.MustCompile(`на\s+(\d{1,6})\s*(?:$|[,.;!?])`)
	// «серверов будет три» — число словом после объекта.
	reCountWordRev = regexp.MustCompile(`(сервер[а-яёa-z]*|комп(?:ьютер[а-яёa-z]*|ов|а|ы|ам|ах)?|пк|станци[а-яёa-z]*)\s+(будет\s+|нужно\s+)?(один|одна|два|две|три|четыре|пять|десять)(?:$|[^а-яё])`)
	reTermYear     = regexp.MustCompile(`на\s+(\d{1,2}|один|одн[а-яёa-z]*|два|две|три)\s+(год|года|лет)`)
	reTermMon      = regexp.MustCompile(`на\s+(\d{1,2})\s+мес`)
	reGeneral      = regexp.MustCompile(`(защит|антивирус|безопасност|для\s+офиса|решени|резервн|бэкап|удал[её]нн[а-яёa-z]*\s+доступ|офисн[а-яёa-z]*\s+пакет|операционн|электронн[а-яёa-z]*\s+подпис|шифрован|видеоконференц|вебинар)`)
	reOffTopic     = regexp.MustCompile(`(оплат|доставк|счет([^а-яё]|$)|счёт|счета|выставит|возврат|скидк|промокод|корм|погод|анекдот|инструкци|системн[а-яёa-z]*\s+промпт)`)
)

// numWords — числительные словами для условий.
var numWords = map[string]int{"один": 1, "одна": 1, "одно": 1, "два": 2, "две": 2, "три": 3, "четыре": 4, "пять": 5,
	"двух": 2, "трех": 3, "четырех": 4, "пяти": 5, "десяти": 10,
	"десять": 10, "двадцать": 20, "тридцать": 30, "сорок": 40, "пятьдесят": 50, "сто": 100}

// guideVendorStems — начало слова → вендор каталога.
var guideVendorStems = []struct{ stem, vendor string }{
	{"касперск", "kaspersky"}, {"kaspersky", "kaspersky"}, {"kes", "kaspersky"},
	{"криптопро", "kripto-pro"}, {"крипто про", "kripto-pro"}, {"cryptopro", "kripto-pro"}, {"crypto pro", "kripto-pro"},
	{"astra", "astralinux"}, {"астра", "astralinux"},
	{"dr.web", "drweb"}, {"drweb", "drweb"}, {"доктор веб", "drweb"}, {"доктор web", "drweb"},
	{"кибер бэкап", "cyberprotect"}, {"кибербэкап", "cyberprotect"}, {"cyber backup", "cyberprotect"},
	{"pro32", "pro32"}, {"anydesk", "anydesk"}, {"эни деск", "anydesk"},
	{"microsoft", "microsoft"}, {"майкрософт", "microsoft"}, {"мтс линк", "webinar"},
}

// guideFeatureStems — начало слова → ось базы знаний (требуемая функция).
var guideFeatureStems = []struct{ stem, axis string }{
	{"шифрован", "encryption"}, {"патч", "patch_management"}, {"обновлени", "patch_management"},
	{"контрол аномал", "adaptive_anomaly"}, {"аномал", "adaptive_anomaly"}, {"почтов", "mail_server"},
	{"edr", "edr"},
}

// guideNorm — нижний регистр, ё → е, одинарные пробелы.
func guideNorm(s string) string {
	s = strings.ToLower(strings.ReplaceAll(strings.ReplaceAll(s, "ё", "е"), "Ё", "е"))
	return strings.Join(strings.Fields(s), " ")
}

// routeHardware — причина маршрута: запрос об оборудовании («подобрать комп», «ноутбук»). Подбор — только для
// лицензий на ПО: обычная выдача, без строки-предложения и без раскрытия.
const routeHardware = "hardware"

// RouteGuide — POST /api/guide/route: состояние строки по правилам (SPEC §1).
func (s *Services) RouteGuide(ctx context.Context, query string) out.GuideRoute {
	mode, reason := routeQuery(query)
	if mode != routeSearch && s.hardwareQuery(ctx, query) {
		return out.GuideRoute{Mode: routeSearch, Reason: routeHardware}
	}
	r := out.GuideRoute{Mode: mode, Reason: reason}
	switch {
	case mode == routeGuide && asksCompare(query):
		r.OpenNow, r.OpenReason = true, "compare"
	case mode == routeGuide:
		r.OpenNow, r.OpenReason = true, "ask"
	case mode != routeSearch || reason == "product_name":
		// Ограничение применения у названного продукта («криптопро для сервера») — подбор сразу (UI §2).
		if d := extractConditions(query); d.vendor != "" && (d.objKind == "server" || d.use != "") && len(d.objects) == 0 {
			r.Mode, r.OpenNow, r.OpenReason = routeGuide, true, "use_limit"
		}
	}
	if r.Mode != routeSearch {
		r.Offer = GuideOfferText
	}
	return r
}

// hardwareQuery — запрос называет категорию оборудования и не называет ПО: разбор оборудования поиска
// (hardware_parse.go) на текущем каталоге. «антивирус на 20 компьютеров» — не оборудование (места лицензии),
// «подобрать комп», «системный блок», «ноутбук» — оборудование. Каталога или оборудования в нём нет — false.
func (s *Services) hardwareQuery(ctx context.Context, query string) bool {
	if s == nil {
		return false
	}
	st := s.catalog.Load()
	if st == nil || st.lex == nil || !st.hw.hasHardware() {
		return false
	}
	if ctx == nil {
		ctx = context.Background()
	}
	rules := s.rulesFor(ctx, domain.ConfigB3, st.lex, nil)
	nr := normalizeFull(query, st.lex, rules, false)
	attrs := extractAttrs(nr.tokens)
	p := s.hardwareParse(ctx, st, nr, &attrs, in.SearchRequest{Query: query})
	return p != nil && p.category != ""
}

// routeQuery — режим и причина.
func routeQuery(query string) (string, string) {
	q := guideNorm(query)
	if q == "" {
		return routeSearch, "empty"
	}
	if reOffTopic.MatchString(q) && !reGuideAsk.MatchString(q) {
		return routeSearch, "service_or_off_topic"
	}
	if reGuideAsk.MatchString(q) {
		return routeGuide, "question"
	}
	words := strings.Fields(q)
	if len(words) == 1 && reSKU.MatchString(words[0]) {
		return routeSearch, "sku"
	}
	d := extractConditions(q)
	hasCount := len(d.objects) > 0
	if hasCount && d.operation == "renewal" && d.term > 0 {
		return routeSearch, "full_conditions"
	}
	if hasCount {
		return routeSearchOffer, "count_without_conditions"
	}
	if d.vendor == "" && reGeneral.MatchString(q) {
		return routeSearchOffer, "general_request"
	}
	return routeSearch, "product_name"
}

// extractConditions — условия из текста правилами: число и вид объектов, продление, срок, вендор, функции,
// «для сервера», TLS, «самый дешёвый», уровень Astra.
func extractConditions(q string) guideDelta {
	q = guideNorm(q)
	d := guideDelta{fromText: true}
	addObj := func(kind string, n int) {
		if n <= 0 || n > maxQuoteQuantity {
			return
		}
		for i := range d.objects {
			if d.objects[i].Kind == kind {
				d.objects[i].Count = n
				return
			}
		}
		d.objects = append(d.objects, domain.GuideObject{Kind: kind, Count: n})
	}
	for _, m := range reCount.FindAllStringSubmatch(q, -1) {
		n, _ := strconv.Atoi(m[1])
		addObj(objectKind(m[3]), n)
	}
	for _, m := range reCountRev.FindAllStringSubmatch(q, -1) {
		n, _ := strconv.Atoi(m[3])
		if len(d.objects) == 0 {
			addObj(objectKind(m[1]), n)
		}
	}
	// Числительные словами: «два сервера», «три сервера».
	fields := strings.Fields(q)
	for i := 0; i+1 < len(fields); i++ {
		if n, ok := numWords[fields[i]]; ok {
			if k := objectKind(fields[i+1]); k != "" {
				addObj(k, n)
			}
		}
	}
	for _, m := range reCountWordRev.FindAllStringSubmatch(q, -1) {
		if n, ok := numWords[m[3]]; ok {
			addObj(objectKind(m[1]), n)
		}
	}
	if len(d.objects) == 0 {
		if m := reBareCount.FindStringSubmatch(q); m != nil {
			n, _ := strconv.Atoi(m[1])
			d.objects = append(d.objects, domain.GuideObject{Kind: "", Count: n}) // вид — из состояния
		}
	}
	if len(d.objects) == 0 {
		if m := reBudet.FindStringSubmatch(q); m != nil {
			n, _ := strconv.Atoi(m[1])
			d.objects = append(d.objects, domain.GuideObject{Kind: "", Count: n}) // вид — из состояния
		}
	}
	if strings.Contains(q, "продлен") || strings.Contains(q, "продлит") || strings.Contains(q, "продлить") {
		d.operation = "renewal"
	}
	if m := reTermYear.FindStringSubmatch(q); m != nil {
		n, err := strconv.Atoi(m[1])
		if err != nil {
			n = numWords[m[1]]
			if strings.HasPrefix(m[1], "одн") {
				n = 1
			}
		}
		if n > 0 {
			d.term = n * 12
		}
	} else if m := reTermMon.FindStringSubmatch(q); m != nil {
		d.term, _ = strconv.Atoi(m[1])
	}
	for _, v := range guideVendorStems {
		if strings.Contains(q, v.stem) {
			d.vendor = v.vendor
			break
		}
	}
	for _, f := range guideFeatureStems {
		if strings.Contains(q, f.stem) && !strings.Contains(q, "без "+f.stem) {
			if !containsStr(d.features, f.axis) && featureNeeded(q, f.stem) {
				d.features = append(d.features, f.axis)
			}
		}
	}
	if strings.Contains(q, "tls") {
		d.use = "tls_server"
		d.objKind = "server"
	} else if regexp.MustCompile(`(для|на)\s+(сервер|серверн)`).MatchString(q) || strings.Contains(q, "серверн") {
		d.objKind = "server"
	}
	if strings.Contains(q, "самый дешев") || strings.Contains(q, "самая дешев") || strings.Contains(q, "подешевле") || strings.Contains(q, "дешевле всего") {
		d.cheapest = true
	}
	if v := variantFromTitle(q); v != "" {
		d.variant = v
	}
	switch {
	case strings.Contains(q, "государствен") || strings.Contains(q, "бюджетн"):
		d.org = "government"
	case strings.Contains(q, "образован") || strings.Contains(q, "школ") || strings.Contains(q, "вуз"):
		d.org = "academic"
	}
	if strings.Contains(q, "антивир") || strings.Contains(q, "защит") && !reGeneralIS.MatchString(q) {
		d.task = "endpoint_protection" // «подбери антивирь», «защита для компьютеров»
	}
	// Переход с другого антивируса (контур помощника, сценарий 5): «у нас eset, хотим сменить», «перейти с другого антивируса».
	if reMigrateFrom.MatchString(q) && (reOtherAV.MatchString(q) || strings.Contains(q, "антивир")) {
		d.operation, d.task = domain.LicenseKindMigration, "endpoint_protection"
	}
	return d
}

// reMigrateFrom — просьба сменить продукт; reOtherAV — антивирус, которого нет в каталоге или другого вендора.
var (
	reMigrateFrom = regexp.MustCompile(`(смен[а-яё]*|перей[тд][а-яё]*|переход[а-яё]*|миграц[а-яё]*|заменить|уйти с)`)
	reOtherAV     = regexp.MustCompile(`(eset|nod ?32|есет|avast|аваст|symantec|norton|mcafee|trend ?micro|sophos|bitdefender|другого антивир)`)
)

// featureNeeded — функция названа как требование, а не в вопросе «чем отличается».
func featureNeeded(q, stem string) bool {
	return regexp.MustCompile(`(нужн[а-яёa-z]*|нужен|требу[а-яёa-z]*|необходим[а-яёa-z]*|с\s+|важн[а-яёa-z]*|обязательн[а-яёa-z]*)[^.,;]{0,30}` + regexp.QuoteMeta(stem) +
		`|` + regexp.QuoteMeta(stem) + `[^.,;]{0,20}(нужн[а-яёa-z]*|нужен|требу[а-яёa-z]*|необходим[а-яёa-z]*|обязательн[а-яёa-z]*)`).MatchString(q)
}

// objectKind — вид объекта по слову.
func objectKind(w string) string {
	switch {
	case strings.HasPrefix(w, "сервер"):
		return "server"
	case strings.HasPrefix(w, "пользовател") || strings.HasPrefix(w, "сотрудник") || strings.HasPrefix(w, "человек") ||
		strings.HasPrefix(w, "админ") || strings.HasPrefix(w, "участник"):
		return "user"
	case strings.HasPrefix(w, "подключени"):
		return "connection"
	case strings.HasPrefix(w, "хост") || w == "ip":
		return "host"
	case strings.HasPrefix(w, "пк") || strings.HasPrefix(w, "комп") || strings.HasPrefix(w, "рабоч") || strings.HasPrefix(w, "станц") ||
		strings.HasPrefix(w, "мест") || strings.HasPrefix(w, "узл") || strings.HasPrefix(w, "устройств") ||
		strings.HasPrefix(w, "машин") || strings.HasPrefix(w, "лицензи") || strings.HasPrefix(w, "шт"):
		return "workstation"
	}
	return ""
}

func containsStr(xs []string, x string) bool {
	for _, v := range xs {
		if v == x {
			return true
		}
	}
	return false
}

// offTopic — посторонняя часть запроса (корм, погода) или сервисная (оплата, доставка).
func offTopic(q string) bool { return reOffTopic.MatchString(guideNorm(q)) }

// asksCompare — запрос о различиях редакций.
func asksCompare(q string) bool {
	q = guideNorm(q)
	return regexp.MustCompile(`(чем\s+.*отлича|отличается|отличия|разниц|сравни|сравнить|срав[а-яё]*|что\s+лучше)`).MatchString(q)
}
