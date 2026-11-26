package services

import (
	"fmt"
	"slices"
	"strconv"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Атрибуты запроса правилами (ARCHITECTURE §2.1, шаг 6): количество и метрика лицензии, срок, тип сделки,
// происхождение. Роль числа определяется по соседним словам: «11» в «винда 11» — версия, «10» в «на 10 пк» —
// количество. Метрика не подменяется: «на 30 компов» — устройства, «на 30 мест» — неясно (уточняющий вопрос).

// Поля чипов сверх условий лицензии.
const (
	fieldVendor   = "vendor"
	fieldAnalogOf = "analog_of"
	fieldCategory = "category"
)

// Значения license_metric в уточнении «Неважно».
const clarifyAny = "any"

// queryAttrs — что правила извлекли из запроса.
type queryAttrs struct {
	quantity int    // 0 — не указано
	metric   string // device | user | core | server | ""
	// metricUnresolved — количество есть, метрика неясна («на 30 мест», «5 лицензий»).
	metricUnresolved bool
	// metricWord — слово, по которому определялась метрика (для вопроса).
	metricWord string
	// serverPref — «для сервера», «на сервер»: мягкое предпочтение серверной лицензии.
	serverPref bool
	duration   int    // месяцы, 0 — не указано
	licType    string // perpetual | subscription | "" (предпочтение)
	// metricPref — метрика, которую модель домыслила по контексту («для команды» → user): не фильтр, только
	// предпочтение при выборе лицензии семейства.
	metricPref string
	renewal    bool   // продление — обязательное условие
	origin     string // ru | ""
	analog     bool   // «аналог», «замена», «вместо»: просят замену
	// Контур доводки (purchase_attrs.go): тип покупки кроме продления (extension | migration), тип организации,
	// редакция из ответа на уточнение, явное требование реестра («из реестра», «в реестре»).
	deal            string
	org             string
	edition         string
	editionAnswered bool
	reestrRequired  bool
	// consumed — индексы токенов, ушедших в атрибуты: в текстовый поиск они не идут.
	consumed map[int]bool
	found    []domain.FoundAttribute
}

// metricStems — начало слова → метрика. Пустая метрика — слово количества без ясной метрики.
var metricStems = []struct{ stem, metric string }{
	{"пк", domain.MetricDevice}, {"комп", domain.MetricDevice}, {"устройств", domain.MetricDevice},
	{"устр", domain.MetricDevice}, {"девайс", domain.MetricDevice}, {"станци", domain.MetricDevice},
	{"узл", domain.MetricDevice}, {"узел", domain.MetricDevice}, {"ноутбук", domain.MetricDevice},
	{"машин", domain.MetricDevice}, {"pc", domain.MetricDevice}, {"device", domain.MetricDevice},
	{"пользовател", domain.MetricUser}, {"юзер", domain.MetricUser}, {"сотрудник", domain.MetricUser},
	{"человек", domain.MetricUser}, {"чел", domain.MetricUser}, {"людей", domain.MetricUser},
	{"user", domain.MetricUser}, {"подключени", domain.MetricUser},
	{"ядр", domain.MetricCore}, {"ядер", domain.MetricCore}, {"core", domain.MetricCore},
	{"сервер", domain.MetricServer},
	{"мест", ""}, {"лиценз", ""}, {"рабоч", ""}, {"шт", ""}, {"копи", ""},
}

// metricOf — метрика по слову; known=false — слово не про количество.
func metricOf(w string) (string, bool) {
	for _, m := range metricStems {
		if strings.HasPrefix(w, m.stem) {
			return m.metric, true
		}
	}
	return "", false
}

// durationUnit — месяцев в единице срока; 0 — не срок.
func durationUnit(w string) int {
	switch {
	case w == "год" || w == "года" || w == "лет" || w == "годa" || strings.HasPrefix(w, "year"):
		return 12
	case strings.HasPrefix(w, "месяц") || w == "мес":
		return 1
	}
	return 0
}

var analogWords = setOf("аналог", "аналоги", "аналога", "аналогом", "замена", "замену", "заменить", "вместо",
	"альтернатива", "альтернативу", "похожее", "похожий")

func isOriginWord(w string) bool {
	return strings.HasPrefix(w, "отечествен") || strings.HasPrefix(w, "российск") || strings.HasPrefix(w, "импортозамещ") ||
		strings.HasPrefix(w, "реестр")
}

// extractAttrs — атрибуты из нормализованных токенов.
func extractAttrs(tokens []string) queryAttrs {
	a := queryAttrs{consumed: map[int]bool{}}
	n := len(tokens)
	at := func(i int) string {
		if i < 0 || i >= n {
			return ""
		}
		return tokens[i]
	}
	for i := 0; i < n; i++ {
		t := tokens[i]
		num, err := strconv.Atoi(t)
		switch {
		case err == nil && num > 0 && num < 100000:
			next := at(i + 1)
			// «рабочих мест», «рабочих станций»: метрика по второму слову.
			if strings.HasPrefix(next, "рабоч") {
				if m, ok := metricOf(at(i + 2)); ok && m != "" {
					a.setQuantity(num, m, at(i+2), i, i+1, i+2)
					a.consume(tokens, i-1, "на")
					i += 2
					continue
				}
				a.setQuantity(num, "", next+" "+at(i+2), i, i+1)
				if strings.HasPrefix(at(i+2), "мест") {
					a.consumed[i+2] = true
				}
				a.consume(tokens, i-1, "на")
				i++
				continue
			}
			if u := durationUnit(next); u > 0 {
				a.duration = num * u
				a.consumed[i], a.consumed[i+1] = true, true
				a.consume(tokens, i-1, "на")
				i++
				continue
			}
			if yearLike(num) && at(i-1) != "на" {
				continue // «windows server 2022 лицензия», «office 2019»: год выпуска, а не число мест
			}
			if m, ok := metricOf(next); ok && !(m == domain.MetricServer && at(i-1) != "на") {
				a.setQuantity(num, m, next, i, i+1)
				a.consume(tokens, i-1, "на")
				i++
				continue
			}
			if at(i-1) == "на" && next == "" {
				a.setQuantity(num, "", "", i)
				a.consumed[i-1] = true
			}
		case durationUnit(t) == 12 && t == "год" && (at(i-1) == "на" || at(i-1) == "лицензия"):
			a.duration = 12
			a.consumed[i] = true
			a.consume(tokens, i-1, "на")
		case strings.HasPrefix(t, "продл") && !isSupportWord(at(i+1)) && !isSupportWord(at(i+2)):
			// «продление поддержки hpe», «продлить гарантию»: это покупка пакета сервиса, а не продление
			// лицензии; слово остаётся в тексте поиска.
			a.renewal = true
			a.consumed[i] = true
		case strings.HasPrefix(t, "бессрочн") || t == "навсегда":
			a.licType = domain.LicensePerpetual
			a.consumed[i] = true
		case strings.HasPrefix(t, "подписк"):
			if a.licType == "" {
				a.licType = domain.LicenseSubscription
			}
			a.consumed[i] = true
		case isOriginWord(t):
			a.origin = "ru"
			a.consumed[i] = true
			a.consume(tokens, i-1, "в")
		case analogWords[t]:
			a.analog = true
			a.consumed[i] = true
			if t == "заменить" {
				a.consume(tokens, i-1, "чем")
			}
		case strings.HasPrefix(t, "сервер") && (at(i-1) == "на" || at(i-1) == "для"):
			a.serverPref = true
		}
	}
	if a.renewal && a.licType == domain.LicenseSubscription {
		a.licType = "" // «продление подписки» — это продление
	}
	extractPurchase(tokens, &a)
	return a
}

// isSupportWord — гарантия и сервисная поддержка оборудования: их «продление» — новый пакет сервиса.
func isSupportWord(w string) bool {
	for _, p := range []string{"поддерж", "гарант", "сервис", "обслуж", "support", "warranty"} {
		if strings.HasPrefix(w, p) {
			return true
		}
	}
	return false
}

// consume — служебное слово перед атрибутом («на 10 пк», «в реестре»): уходит в атрибут, только если это оно.
// Раньше слово не сверялось, и «видеоконференции российские» теряло «видеоконференции».
func (a *queryAttrs) consume(tokens []string, i int, word string) {
	if i >= 0 && i < len(tokens) && tokens[i] == word {
		a.consumed[i] = true
	}
}

func (a *queryAttrs) setQuantity(num int, metric, word string, idx ...int) {
	a.quantity = num
	a.metric = metric
	a.metricUnresolved = metric == ""
	a.metricWord = word
	for _, i := range idx {
		a.consumed[i] = true
	}
}

// applyRequest — снятые покупателем чипы и ответ на уточняющий вопрос.
func (a *queryAttrs) applyRequest(req in.SearchRequest) {
	for _, c := range req.RemovedChips {
		switch c.Field {
		case domain.FieldLicenseQuantity:
			a.quantity, a.metricUnresolved = 0, false
			if a.metric != "" && c.Value == "" {
				a.metric = ""
			}
		case domain.FieldLicenseMetric:
			a.metric, a.metricUnresolved = "", false
		case domain.FieldLicenseDuration:
			a.duration = 0
		case domain.FieldLicenseType:
			a.licType = ""
		case domain.FieldRenewal:
			a.renewal = false
		case domain.FieldOrigin:
			a.origin = ""
		}
	}
	a.applyPurchaseRequest(req)
	if v, ok := req.Clarify[domain.FieldLicenseMetric]; ok && a.metricUnresolved {
		switch v {
		case domain.MetricDevice, domain.MetricUser, domain.MetricCore, domain.MetricServer:
			a.metric, a.metricUnresolved = v, false
		case clarifyAny:
			a.metricUnresolved = false
		}
	}
}

// metricLabel — «10 устройств», «5 пользователей»; без метрики — «10 лицензий».
func quantityLabel(q int, metric string) string {
	if forms, ok := metricForms[metric]; ok {
		return fmt.Sprintf("%d %s", q, plural(q, forms))
	}
	return fmt.Sprintf("%d %s", q, plural(q, [3]string{"лицензия", "лицензии", "лицензий"}))
}

// chips — чипы «Мы поняли запрос так» и атрибуты для трассы. strict — условие учтено строго.
func (a queryAttrs) chips(ent queryEntities, lx *searchLexicon) ([]out.Chip, []domain.FoundAttribute) {
	chips := []out.Chip{}
	found := []domain.FoundAttribute{}
	add := func(field, value, label string, strict bool) {
		chips = append(chips, out.Chip{Field: field, Value: value, Label: label, Strict: strict})
		found = append(found, domain.FoundAttribute{Field: field, Value: value, Strict: strict, Source: domain.ParseRules})
	}
	if a.analog {
		for _, f := range ent.reference {
			add(fieldAnalogOf, f, "Аналог "+lx.familyName[f], true)
		}
	}
	if !a.analog {
		named := lx.matchVendors(ent.entityTokens, nil)
		for _, v := range ent.vendors {
			if ent.absentName != "" && !slices.Contains(named, v) {
				continue // «solidworks»: Autodesk — ближайший вендор для аналогов, покупатель его не называл
			}
			add(fieldVendor, v, lx.vendorName[v], true)
		}
	}
	if ent.category != "" {
		add(fieldCategory, ent.category, lx.categoryName[ent.category], false)
	}
	if a.quantity > 0 {
		add(domain.FieldLicenseQuantity, strconv.Itoa(a.quantity), quantityLabel(a.quantity, a.metric), true)
	}
	if a.metric != "" {
		labels := map[string]string{domain.MetricDevice: "На устройство", domain.MetricUser: "На пользователя",
			domain.MetricCore: "На ядро", domain.MetricServer: "На сервер"}
		add(domain.FieldLicenseMetric, a.metric, labels[a.metric], true)
	} else if a.serverPref {
		add(domain.FieldLicenseMetric, domain.MetricServer, "Серверная лицензия", false)
	}
	if a.duration > 0 {
		add(domain.FieldLicenseDuration, strconv.Itoa(a.duration), "Срок "+duration(a.duration), false)
	}
	switch a.licType {
	case domain.LicensePerpetual:
		add(domain.FieldLicenseType, a.licType, "Бессрочная", false)
	case domain.LicenseSubscription:
		add(domain.FieldLicenseType, a.licType, "Подписка", false)
	}
	if a.renewal {
		add(domain.FieldRenewal, "true", "Продление", true)
	}
	if a.origin != "" {
		add(domain.FieldOrigin, a.origin, "Отечественное ПО", true)
	}
	a.purchaseChips(add)
	return chips, found
}

// clarify — уточняющий вопрос по неразрешённой метрике; nil — спрашивать нечего.
func (a queryAttrs) clarify() *out.Clarify {
	if !a.metricUnresolved || a.quantity == 0 {
		return nil
	}
	what := quantityLabel(a.quantity, "")
	if a.metricWord != "" {
		what = fmt.Sprintf("%d %s", a.quantity, a.metricWord)
	}
	return &out.Clarify{
		Attribute: domain.FieldLicenseMetric,
		Question:  what + " — лицензия на устройство или на пользователя?",
		Options: []out.ClarifyOption{
			{Value: domain.MetricDevice, Label: "На устройство"},
			{Value: domain.MetricUser, Label: "На пользователя"},
			{Value: clarifyAny, Label: "Неважно"},
		},
	}
}

// unresolved — неразрешённые атрибуты для трассы.
func (a queryAttrs) unresolved() []string {
	if a.metricUnresolved && a.quantity > 0 {
		return []string{domain.FieldLicenseMetric}
	}
	return []string{}
}
