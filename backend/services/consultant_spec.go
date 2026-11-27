package services

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"kuznica/api/out"
)

// Спецификация и сервисные темы консультанта: решает и формулирует код (без модели). Спецификация — стор фронта,
// присланный в запросе; ядро отдаёт spec_event, фронт применяет его к стору.

// ConsultantLinks — настройка «тема → заголовок и адрес» (CONSULTANT.md §6). Пустой адрес — фраза без ссылки.
type ConsultantLinks struct {
	Payment  string
	Delivery string
	Contacts string
}

// consultantTopic — сервисная тема.
type consultantTopic struct {
	key, title, url string
}

const consultantSpecialsURL = "https://store.softline.ru/specials/"

func (l ConsultantLinks) topic(key string) consultantTopic {
	switch key {
	case "payment":
		return consultantTopic{key, "Способы оплаты", l.Payment}
	case "delivery":
		return consultantTopic{key, "Доставка", l.Delivery}
	case "contacts":
		return consultantTopic{key, "Контакты", l.Contacts}
	case "specials":
		return consultantTopic{key, "Акции и спецпредложения", consultantSpecialsURL}
	}
	return consultantTopic{}
}

var (
	discountRe = regexp.MustCompile(`(?i)скидк|промокод|дешевле сделай|сбрось цену|уступите`)
	paymentRe  = regexp.MustCompile(`(?i)оплат|по сч[её]ту|выставить сч[её]т|выстав.* сч[её]т|рассрочк`)
	deliveryRe = regexp.MustCompile(`(?i)доставк|достав|как получить лиценз|получение лиценз|когда придет|когда придёт`)
	specialsRe = regexp.MustCompile(`(?i)акци|спецпредлож|распродаж`)
	contactsRe = regexp.MustCompile(`(?i)связаться|контакт|позвонить|телефон магазина|живой человек|с человеком|с менеджером|менеджер`)
)

// serviceTurn — оплата, доставка, акции, контакты, просьба о скидке.
func serviceTurn(text string, links ConsultantLinks) (consultantPlan, bool) {
	l := strings.ToLower(text)
	if discountRe.MatchString(l) {
		return consultantPlan{
			messages: []string{"Назначать скидки я не могу. Но могу проверить действующие акции или подобрать вариант под ваш бюджет."},
			quick:    []string{"Показать акции", "Подобрать под бюджет"},
		}, true
	}
	var key, withLink, noLink string
	switch {
	case paymentRe.MatchString(l):
		key = "payment"
		withLink = "Инструкцию по оплате, в том числе по счёту, можно посмотреть в разделе оплаты магазина. Вот ссылка."
		noLink = "Инструкция по оплате, в том числе по счёту, — в разделе «Способы оплаты» на сайте магазина."
	case deliveryRe.MatchString(l):
		key = "delivery"
		withLink = "Как получить заказ и лицензии, описано в разделе доставки магазина. Вот ссылка."
		noLink = "Как получить заказ и лицензии, описано в разделе «Доставка» на сайте магазина."
	case specialsRe.MatchString(l):
		key = "specials"
		withLink = "Действующие акции собраны в разделе акций магазина. Вот ссылка."
	case contactsRe.MatchString(l):
		key = "contacts"
		withLink = "Связаться с сотрудником магазина можно через раздел контактов. Вот ссылка."
		noLink = "Связаться с сотрудником магазина можно через раздел «Контакты» на сайте магазина."
	default:
		return consultantPlan{}, false
	}
	tp := links.topic(key)
	if tp.url == "" {
		return consultantPlan{messages: []string{noLink}}, true
	}
	return consultantPlan{messages: []string{withLink},
		attachments: []any{out.ConsultantLink{Type: "link", Topic: tp.key, Title: tp.title, URL: tp.url}}}, true
}

var (
	explainQRe   = regexp.MustCompile(`(?i)чем .*отлича|в ч[её]м разниц|разница между|что такое|что значит|что означает|зачем нужн|как работает`)
	explainCRe   = regexp.MustCompile(`(?i)продлен|новая лиценз|новой лиценз|новую лиценз|подписк|бессрочн|ступен|лицензи`)
	specShowRe   = regexp.MustCompile(`(?i)(что|чего).{0,20}(в|во) спецификац|состав спецификац|покажи спецификац|сколько итого|итого\??$|какой итог|сколько (всего|в сумме)|на какую сумму`)
	specAddRe    = regexp.MustCompile(`(?i)(^|[^\p{L}])(добавь|добавьте|добавить|добавляй|положи|положите|закинь)([^\p{L}]|$)|в спецификацию`)
	specDelRe    = regexp.MustCompile(`(?i)(^|[^\p{L}])(убери|уберите|убрать|удали|удалите|удалить|выкинь|исключи)([^\p{L}]|$)`)
	specQtyRe    = regexp.MustCompile(`(?i)(измени|поменяй|поставь|сделай|укажи).{0,25}?(\d{1,5})`)
	specWordRe   = regexp.MustCompile(`[\p{L}\d.]+`)
	specVerbs    = map[string]bool{"убери": true, "убрать": true, "удали": true, "удалить": true, "выкинь": true, "исключи": true, "добавь": true, "добавить": true, "положи": true, "из": true, "в": true, "спецификации": true, "спецификацию": true, "пожалуйста": true, "давай": true, "его": true, "ее": true, "её": true, "это": true, "этот": true, "эту": true, "тоже": true, "и": true, "мне": true, "все": true}
	specSynonyms = map[string][]string{
		"антивирус": {"антивирус", "kaspersky", "касперск", "dr.web", "eset", "endpoint security", "pro32", "small office security"},
		"защит":     {"антивирус", "kaspersky", "касперск", "dr.web", "eset", "endpoint security", "pro32"},
		"офис":      {"office", "офис", "р7", "workspace", "яндекс 360"},
		"крипто":    {"криптопро", "cryptopro", "crypto"},
		"бэкап":     {"backup", "бэкап", "резервн"},
		"резерв":    {"backup", "бэкап", "резервн"},
	}
)

// consultantExplainQuestion — вопрос о понятиях лицензирования без названия продукта (сценарий 4).
func consultantExplainQuestion(text string) bool {
	l := guideNorm(text)
	if !explainQRe.MatchString(l) || !explainCRe.MatchString(l) {
		return false
	}
	for _, v := range guideVendorStems {
		if strings.Contains(l, v.stem) {
			return false
		}
	}
	return !strings.Contains(l, "редакц")
}

// specLineView — строка спецификации с ценой каталога.
type specLineView struct {
	offerID string
	title   string
	qty     int
	total   *float64
	sub     string // подзаголовок: «Новая лицензия · 50 компьютеров · 1 год»
	match   string // текст для сопоставления (название, семейство)
	offer   *out.GuideOffer
}

func (t *consultantTurn) specLines() []specLineView {
	var res []specLineView
	for _, l := range t.req.Spec {
		p := t.cat.products[l.OfferID]
		if p == nil {
			res = append(res, specLineView{offerID: l.OfferID, title: "Позиция " + l.OfferID, qty: l.Qty})
			continue
		}
		it := t.offerItem(p, l.Qty, consultantFit)
		v := t.cat.view(p, t.kb)
		var total *float64
		if v.priced() {
			sum := guideRound2(*p.PriceRub * float64(v.units(max(l.Qty, 1))))
			total = &sum
		}
		o := it.Offer
		res = append(res, specLineView{offerID: l.OfferID, title: it.Title, sub: it.Subtitle, qty: l.Qty, total: total, offer: &o,
			match: guideNorm(it.Title + " " + p.Title + " " + p.FamilyName)})
	}
	return res
}

// specTurn — «что в спецификации», «добавь», «убери», «измени количество».
func (t *consultantTurn) specTurn() (consultantPlan, bool) {
	text := guideNorm(t.req.Text)
	switch {
	case specDelRe.MatchString(text):
		return t.specRemove(text), true
	case specShowRe.MatchString(text):
		return t.specShow(), true
	case specAddRe.MatchString(text):
		return t.specAdd(text), true
	case len(t.req.Spec) > 0 && specQtyRe.MatchString(text) && strings.Contains(text, "спецификац"):
		return t.specQty(text), true
	}
	return consultantPlan{}, false
}

func (t *consultantTurn) specShow() consultantPlan {
	t.status(consultantStepCalculating)
	lines := t.specLines()
	if len(lines) == 0 {
		return consultantPlan{messages: []string{"Сейчас спецификация пустая. Можем продолжить подбор, когда понадобится."}}
	}
	sum := &out.ConsultantSpecSummary{Type: "spec_summary", Items: []out.ConsultantSpecItem{}, Count: len(lines)}
	total, known := 0.0, true
	for _, l := range lines {
		sum.Items = append(sum.Items, out.ConsultantSpecItem{OfferID: l.offerID, Title: l.title, Quantity: l.qty, TotalPriceMinor: minor(l.total)})
		if l.total == nil {
			known = false
		} else {
			total += *l.total
		}
	}
	if m := minor(&total); m != nil {
		sum.TotalMinor = *m
	}
	var msg string
	if len(lines) == 1 {
		l := lines[0]
		msg = "Сейчас там одна позиция — " + specName(l) + qtyPhrase(l) + "."
		if l.total != nil {
			msg += " Стоимость — **" + rubRU(*l.total) + " ₽**."
		}
	} else {
		msg = fmt.Sprintf("Сейчас в спецификации %d %s", len(lines), guidePlural(len(lines), "позиция", "позиции", "позиций"))
		if known {
			msg += " на **" + rubRU(total) + " ₽**."
		} else {
			msg += ". У части позиций цена по запросу, поэтому итог неполный: **" + rubRU(total) + " ₽** за позиции с ценой."
		}
	}
	return consultantPlan{messages: []string{msg}, attachments: []any{*sum}}
}

func specName(l specLineView) string {
	if l.offer != nil {
		if u := storeURL(l.offer.OfferURL); u != "" {
			return "[" + l.title + "](" + u + ")"
		}
	}
	return l.title
}

func qtyPhrase(l specLineView) string {
	if l.sub != "" {
		return ", " + strings.ToLower(strings.ReplaceAll(l.sub, " · ", ", "))
	}
	if l.qty <= 0 {
		return ""
	}
	return fmt.Sprintf(", %d шт.", l.qty)
}

// specTarget — слова-цели реплики без глаголов и служебных слов.
func specTarget(text string) []string {
	var ws []string
	for _, w := range specWordRe.FindAllString(text, -1) {
		if specVerbs[w] || len([]rune(w)) < 3 || specDelRe.MatchString(w) || specAddRe.MatchString(w) {
			continue
		}
		ws = append(ws, w)
	}
	return ws
}

// matchesTarget — строка подходит под слова-цели (название или синоним класса).
func matchesTarget(match string, words []string) bool {
	for _, w := range words {
		for stem, syn := range specSynonyms {
			if strings.HasPrefix(w, stem) {
				for _, s := range syn {
					if strings.Contains(match, s) {
						return true
					}
				}
			}
		}
		stem := w
		if r := []rune(w); len(r) > 5 {
			stem = string(r[:len(r)-2])
		}
		if len([]rune(stem)) >= 3 && strings.Contains(match, stem) {
			return true
		}
	}
	return false
}

func (t *consultantTurn) specRemove(text string) consultantPlan {
	lines := t.specLines()
	if len(lines) == 0 {
		return consultantPlan{messages: []string{"Спецификация пустая — убирать нечего."}}
	}
	words := specTarget(text)
	var hit []specLineView
	for _, l := range lines {
		if len(words) == 0 || matchesTarget(l.match, words) {
			hit = append(hit, l)
		}
	}
	switch {
	case len(hit) == 0:
		return consultantPlan{messages: []string{"В спецификации такой позиции не нашёл. Какую позицию убрать?"}, quick: specQuick(lines)}
	case len(hit) > 1:
		names := make([]string, 0, len(hit))
		for _, l := range hit {
			names = append(names, specName(l))
		}
		return consultantPlan{messages: []string{"Подходят несколько позиций: " + strings.Join(names, ", ") + ". Какую убрать?"}, quick: specQuick(hit)}
	}
	l := hit[0]
	ev := out.ConsultantSpecEvent{Type: "spec_event", Action: "removed", OfferID: l.offerID, Title: l.title, Quantity: l.qty, TotalPriceMinor: minor(l.total), Offer: l.offer}
	return consultantPlan{messages: []string{"Убрал " + specName(l) + " из спецификации."}, attachments: []any{ev}}
}

func specQuick(lines []specLineView) []string {
	var q []string
	for _, l := range lines {
		name := l.title
		if r := []rune(name); len(r) > consultantQuickRunes {
			continue
		}
		q = append(q, name)
		if len(q) == consultantMaxQuick {
			break
		}
	}
	return q
}

// specAdd — «добавь»: позиция из последнего листа фактов (по названию, иначе единственная подходящая).
func (t *consultantTurn) specAdd(text string) (plan consultantPlan) {
	sheet := t.conv.sheet
	if len(sheet) == 0 {
		return consultantPlan{messages: []string{"Пока нечего добавить: сначала подберём позицию. Что ищете?"}}
	}
	words := specTarget(text)
	var hit []consultantItem
	if len(words) > 0 {
		for _, it := range sheet {
			if it.Group != consultantUnfit && matchesTarget(guideNorm(it.Title+" "+it.Offer.Title+" "+it.Offer.FamilyTitle), words) {
				hit = append(hit, it)
			}
		}
	}
	if len(hit) == 0 {
		for _, it := range sheet {
			if it.Group == consultantFit {
				hit = append(hit, it)
			}
		}
	}
	if len(hit) != 1 {
		var names []string
		for _, it := range hit {
			names = append(names, it.Title)
		}
		msg := "Какую позицию добавить?"
		if len(names) > 1 {
			msg = "Какую позицию добавить: " + strings.Join(names, " или ") + "?"
		}
		var quick []string
		for _, n := range names {
			if len([]rune(n)) <= consultantQuickRunes {
				quick = append(quick, n)
			}
		}
		return consultantPlan{messages: []string{msg}, quick: quick}
	}
	it := hit[0]
	if it.Group == consultantUnfit {
		return consultantPlan{messages: []string{"Эту позицию не добавляю: " + strings.ToLower(it.Reason) + "."}}
	}
	qty := max(it.Qty, 1)
	o := it.Offer
	o.Quantity = qty
	for _, l := range t.req.Spec {
		if l.OfferID == o.OfferID {
			if l.Qty == qty {
				return consultantPlan{messages: []string{"Эта позиция уже в спецификации."}}
			}
			ev := out.ConsultantSpecEvent{Type: "spec_event", Action: "updated", OfferID: o.OfferID, Title: it.Title, Quantity: qty, TotalPriceMinor: minor(it.Total), Offer: &o}
			return consultantPlan{messages: []string{fmt.Sprintf("Эта позиция уже в спецификации — поменял количество на %d.", qty)}, attachments: []any{ev}}
		}
	}
	ev := out.ConsultantSpecEvent{Type: "spec_event", Action: "added", OfferID: o.OfferID, Title: it.Title, Quantity: qty, TotalPriceMinor: minor(it.Total), Offer: &o}
	defer func() { t.kitAfterAdd(&plan, it) }()
	msg := "Добавил " + it.Title
	if it.Qty > 0 {
		kind := "workstation"
		if t.conv.guide != nil {
			if k, _ := mainObject(t.conv.guide.Requirements); k != "" {
				kind = k
			}
		}
		msg += fmt.Sprintf(" на %d %s.", qty, objectNoun(kind, qty))
	}
	if it.Total != nil {
		msg += " Стоимость — **" + rubRU(*it.Total) + " ₽**."
	}
	return consultantPlan{messages: []string{strings.TrimSpace(msg)}, attachments: []any{ev}}
}

// specQty — «измени количество … на N».
func (t *consultantTurn) specQty(text string) consultantPlan {
	m := specQtyRe.FindStringSubmatch(text)
	n, _ := strconv.Atoi(m[2])
	if n <= 0 || n > maxQuoteQuantity {
		return consultantPlan{messages: []string{"Количество должно быть от 1 до 100 000."}}
	}
	lines := t.specLines()
	words := specTarget(specQtyRe.ReplaceAllString(text, ""))
	var hit []specLineView
	for _, l := range lines {
		if len(lines) == 1 || matchesTarget(l.match, words) {
			hit = append(hit, l)
		}
	}
	if len(hit) != 1 {
		return consultantPlan{messages: []string{"У какой позиции поменять количество?"}, quick: specQuick(lines)}
	}
	l := hit[0]
	p := t.cat.products[l.offerID]
	if p == nil {
		return consultantPlan{messages: []string{"Этой позиции больше нет в каталоге."}}
	}
	it := t.offerItem(p, n, consultantFit)
	o := it.Offer
	ev := out.ConsultantSpecEvent{Type: "spec_event", Action: "updated", OfferID: l.offerID, Title: l.title, Quantity: n, TotalPriceMinor: minor(it.Total), Offer: &o}
	msg := fmt.Sprintf("Поменял количество: %s, %d шт.", specName(l), n)
	if it.Total != nil {
		msg += " Стоимость — **" + rubRU(*it.Total) + " ₽**."
	}
	if v := t.cat.view(p, t.kb); !v.inTier(n) {
		msg += " Это количество не попадает в ступень цены этой позиции — проверьте её в спецификации."
	}
	return consultantPlan{messages: []string{msg}, attachments: []any{ev}}
}
