package services

import (
	"context"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/prompts"
)

// consultantFakeModel — второй вызов (системный промпт консультанта) отвечает по очереди из replies (последний
// повторяется); вызов подбора — ошибка (подбор работает без модели).
type consultantFakeModel struct {
	replies []string
	calls   int
}

func (f *consultantFakeModel) Chat(_ context.Context, req llm.Request) (*llm.Response, error) {
	if len(req.Messages) == 0 || req.Messages[0].Content != prompts.Consultant().Text {
		return nil, context.Canceled
	}
	r := f.replies[min(f.calls, len(f.replies)-1)]
	f.calls++
	return &llm.Response{Message: llm.Message{Content: r}, Usage: llm.Usage{PromptTokens: 900, CompletionTokens: 80}}, nil
}
func (f *consultantFakeModel) Model() string { return "gpt://folder/aliceai-llm-flash/latest" }

type consultantRun struct {
	events []out.ConsultantEvent
}

func (r *consultantRun) texts() []string {
	var ts []string
	for _, e := range r.events {
		if e.Type == "message.delta" {
			ts = append(ts, e.Text)
		}
	}
	return ts
}

func (r *consultantRun) attachments() []any {
	var as []any
	for _, e := range r.events {
		if e.Type == "message.attachment" {
			as = append(as, e.Attachment)
		}
	}
	return as
}

func (r *consultantRun) errReason() string {
	for _, e := range r.events {
		if e.Type == "message.error" {
			return e.Reason
		}
	}
	return ""
}

func consult(t *testing.T, s *Services, conv, text string, spec []in.ConsultantSpecLine) *consultantRun {
	t.Helper()
	req := in.ConsultantRequest{ConversationID: conv, Text: text, Spec: spec}
	if err := s.ValidateConsultant(&req); err != nil {
		t.Fatal(err)
	}
	r := &consultantRun{}
	s.Consult(context.Background(), "rid", req, func(e out.ConsultantEvent) { r.events = append(r.events, e) })
	return r
}

func f64(v float64) *float64 { return &v }

func testSheet() *consultantSheet {
	return &consultantSheet{Kind: "workstation", Qty: 50, TermMonths: 12, Items: []consultantItem{
		{N: 1, Group: consultantFit, Title: "Kaspersky Endpoint Security — Расширенный", Unit: f64(3940.2), Total: f64(197010), UnitPrev: f64(4081.5),
			Offer: out.GuideOffer{OfferID: "4857", OfferURL: "https://store.softline.ru/kaspersky/kl4867laqfs-4857/"}},
		{N: 2, Group: consultantUnfit, Title: "Kaspersky Endpoint Security — Стандартный", Unit: f64(2715.3), Total: f64(135765),
			Reason: "Шифрования данных в этой редакции нет", Offer: out.GuideOffer{OfferID: "4986", OfferURL: "https://evil.example/x"}},
	}}
}

func TestConsultantChecks(t *testing.T) {
	sh := testSheet()
	bad := map[string]string{
		"Стоит {{total:3}}.":                      "нет в листе",
		"Всего 197 010 ₽ на год.":                 "вне вставки",
		"Это 5000 руб в год.":                     "вне вставки",
		"Скидка 10% на продление.":                "вне вставки",
		"Подробнее на store.softline.ru/specials": "адрес",
		"Смотрите https://example.com":            "адрес",
		"Было {{old:1}}.":                         "нет значения",
		"Стоит {{price:1}}.":                      "неизвестная",
		"Стоит {{ total 1 }}.":                    "формате",
	}
	for msg, want := range bad {
		err := checkRaw(msg, sh)
		if err == nil || !strings.Contains(err.Error(), want) {
			t.Errorf("%q: ждали ошибку %q, получили %v", msg, want, err)
		}
	}
	good := "На {{qty}} цена за место меняется: теперь это {{unit:1}} вместо {{unit_prev:1}}. Всего {{total:1}} на {{term}}. {{name:2}} не подойдёт."
	if err := checkRaw(good, sh); err != nil {
		t.Fatal(err)
	}
	got := substitute(good, sh)
	want := "На 50 компьютеров цена за место меняется: теперь это **3 940,20 ₽** вместо 4 081,50 ₽. Всего **197 010 ₽** на год. Kaspersky Endpoint Security — Стандартный не подойдёт."
	if got != want {
		t.Fatalf("подстановка:\n%s\nждали:\n%s", got, want)
	}
	if n := substitute("{{name:1}}", sh); n != "[Kaspersky Endpoint Security — Расширенный](https://store.softline.ru/kaspersky/kl4867laqfs-4857/)" {
		t.Fatalf("ссылка на магазин: %s", n)
	}
}

func TestConsultantFallbackAfterRetry(t *testing.T) {
	m := &consultantFakeModel{replies: []string{`{"messages":["Стандартный стоит 84 402 ₽."],"attachment":{"type":"comparison","items":[1,2]},"quick_replies":[]}`}}
	s := guideTestServices(t, "verified", m, "on")
	r := consult(t, s, "conv-fallback", "Чем расширенный касперский отличается от стандартного, нам на 30 компьютеров", nil)
	if m.calls != 2 {
		t.Fatalf("ждали вызов и один повтор, было %d", m.calls)
	}
	if r.errReason() != consultantErrInvalid {
		t.Fatalf("причина: %q", r.errReason())
	}
	ts := r.texts()
	if len(ts) != 1 || ts[0] != ConsultantFallbackText {
		t.Fatalf("запасной ответ: %v", ts)
	}
	as := r.attachments()
	if len(as) == 0 {
		t.Fatal("карточки потеряны")
	}
	l, ok := as[0].(out.ConsultantList)
	if !ok || len(l.Groups) == 0 {
		t.Fatalf("ждали список: %#v", as[0])
	}
}

func TestConsultantRetryThenOK(t *testing.T) {
	m := &consultantFakeModel{replies: []string{
		`{"messages":["Смотрите на сайте store.softline.ru"]}`,
		`{"messages":["На {{qty}} Стандартный стоит {{total:1}}."],"attachment":{"type":"comparison","items":[1]},"quick_replies":[]}`,
	}}
	s := guideTestServices(t, "verified", m, "on")
	r := consult(t, s, "conv-retry", "Чем расширенный касперский отличается от стандартного, нам на 30 компьютеров", nil)
	if m.calls != 2 || r.errReason() != "" {
		t.Fatalf("вызовов %d, ошибка %q", m.calls, r.errReason())
	}
	if ts := r.texts(); len(ts) != 1 || !strings.Contains(ts[0], "**84 402 ₽**") {
		t.Fatalf("текст: %v", ts)
	}
}

func TestConsultantModelOff(t *testing.T) {
	s := guideTestServices(t, "verified", nil, "off")
	r := consult(t, s, "conv-off", "самый дешёвый криптопро csp для сервера", nil)
	if r.errReason() != consultantErrModel {
		t.Fatalf("причина: %q", r.errReason())
	}
	if ts := r.texts(); len(ts) != 1 || ts[0] != ConsultantFallbackText {
		t.Fatalf("запасной ответ: %v", ts)
	}
	if len(r.attachments()) == 0 {
		t.Fatal("список из каталога пропал")
	}
}

// Сценарий 2 на каталоге фикстуры (цены как в каталоге магазина): 84 402 / 122 445 на 30; 197 010 на 50,
// 3 940,20 вместо 4 081,50.
func TestConsultantScenario2(t *testing.T) {
	m := &consultantFakeModel{replies: []string{
		`{"messages":["В Расширенном есть шифрование.","На {{qty}} Стандартный стоит {{total:1}}, Расширенный — {{total:2}}."],"attachment":{"type":"comparison","items":[1,2]},"quick_replies":[]}`,
	}}
	s := guideTestServices(t, "verified", m, "on")
	r := consult(t, s, "conv-s2", "Чем расширенный касперский отличается от стандартного, нам на 30 компьютеров", nil)
	ts := r.texts()
	if len(ts) != 2 || ts[1] != "На 30 компьютеров Стандартный стоит **84 402 ₽**, Расширенный — **122 445 ₽**." {
		t.Fatalf("ход 1: %v", ts)
	}
	var cmp *out.ConsultantComparison
	for _, a := range r.attachments() {
		if c, ok := a.(out.ConsultantComparison); ok {
			cmp = &c
		}
	}
	if cmp == nil || len(cmp.Columns) != 2 || *cmp.Columns[0].TotalPriceMinor != 8440200 || *cmp.Columns[1].TotalPriceMinor != 12244500 {
		t.Fatalf("сравнение: %#v", cmp)
	}
	m.replies = []string{`{"messages":["Тогда подходит Расширенный. На {{qty}} цена за место меняется: теперь это {{unit:1}} вместо {{unit_prev:1}}. Всего получится {{total:1}} на {{term}}."],"attachment":{"type":"card","items":[1]},"quick_replies":[]}`}
	m.calls = 0
	r = consult(t, s, "conv-s2", "Компьютеров будет 50, и шифрование нужно", nil)
	ts = r.texts()
	want := "Тогда подходит Расширенный. На 50 компьютеров цена за место меняется: теперь это **3 940,20 ₽** вместо 4 081,50 ₽. Всего получится **197 010 ₽** на год."
	if len(ts) != 1 || ts[0] != want {
		t.Fatalf("ход 2: %v (ошибка %q)", ts, r.errReason())
	}
	var card *out.ConsultantCard
	for _, a := range r.attachments() {
		if c, ok := a.(out.ConsultantCard); ok {
			card = &c
		}
	}
	if card == nil || *card.Product.TotalPriceMinor != 19701000 || *card.Product.UnitPriceMinor != 394020 || card.Product.Offer.OfferID == "" {
		t.Fatalf("карточка: %#v", card)
	}
	// «добавь» — событие спецификации с офером; повтор при той же строке в спецификации — без дубля.
	r = consult(t, s, "conv-s2", "добавь", nil)
	var ev *out.ConsultantSpecEvent
	for _, a := range r.attachments() {
		if e, ok := a.(out.ConsultantSpecEvent); ok {
			ev = &e
		}
	}
	if ev == nil || ev.Action != "added" || ev.Quantity != 50 || *ev.TotalPriceMinor != 19701000 || ev.Offer == nil {
		t.Fatalf("spec_event: %#v", ev)
	}
	r = consult(t, s, "conv-s2", "добавь", []in.ConsultantSpecLine{{OfferID: ev.OfferID, Qty: 50}})
	if len(r.attachments()) != 0 {
		t.Fatalf("повторное «добавь» создало событие: %#v", r.attachments())
	}
	// Смена темы: количество прошлого подбора (50) не переносится на КриптоПро.
	m.replies = []string{`{"messages":["Серверная лицензия стоит {{total:1}}."],"attachment":{"type":"list","items":[1]},"quick_replies":[]}`}
	r = consult(t, s, "conv-s2", "самый дешёвый криптопро csp для сервера", nil)
	for _, a := range r.attachments() {
		l, ok := a.(out.ConsultantList)
		if !ok {
			continue
		}
		for i, g := range l.Groups {
			for _, it := range g.Items {
				if it.Quantity == 50 {
					t.Fatalf("количество прошлой темы перенесено: %#v", it)
				}
			}
			if g.Kind == consultantMore && i+1 < len(l.Groups) {
				t.Fatalf("«Ещё посмотрите» не последней группой: %#v", l.Groups)
			}
		}
	}
	for _, a := range r.attachments() {
		if src, ok := a.(out.ConsultantSource); ok && strings.Contains(src.Quote, "|") {
			t.Fatalf("цитата — строка таблицы: %q", src.Quote)
		}
	}
}

// Сценарий 3 на каталоге фикстуры: 1 850 ₽ (рабочее место) — «Не подойдут» с причиной, серверная 70 000 ₽ подходит.
func TestConsultantScenario3(t *testing.T) {
	m := &consultantFakeModel{replies: []string{`{"messages":["Серверная лицензия стоит {{total:1}}."],"attachment":{"type":"list","items":[1]},"quick_replies":[]}`}}
	s := guideTestServices(t, "verified", m, "on")
	r := consult(t, s, "conv-s3", "самый дешёвый криптопро csp для сервера", nil)
	var list *out.ConsultantList
	for _, a := range r.attachments() {
		if l, ok := a.(out.ConsultantList); ok {
			list = &l
		}
	}
	if list == nil {
		t.Fatalf("нет списка: %v %q", r.texts(), r.errReason())
	}
	var fit70, unfit1850 bool
	n := 0
	for _, g := range list.Groups {
		for _, it := range g.Items {
			if g.Kind != consultantUnfit {
				n++
			}
			if g.Kind == consultantFit && it.TotalPriceMinor != nil && *it.TotalPriceMinor == 7000000 {
				fit70 = true
			}
			if g.Kind == consultantUnfit && it.TotalPriceMinor != nil && *it.TotalPriceMinor == 185000 {
				unfit1850 = it.Reason != "" && it.Compatibility == "incompatible"
			}
		}
	}
	if !fit70 || !unfit1850 {
		t.Fatalf("группы: %#v", list.Groups)
	}
	if ts := r.texts(); len(ts) != 1 || ts[0] != "Серверная лицензия стоит **70 000 ₽**." {
		t.Fatalf("текст: %v", ts)
	}
}

func TestConsultantSpecAndService(t *testing.T) {
	s := guideTestServices(t, "verified", nil, "off")
	two := []in.ConsultantSpecLine{{OfferID: "4857", Qty: 50}, {OfferID: "4985", Qty: 30}}
	r := consult(t, s, "conv-spec", "убери антивирус", two)
	if len(r.attachments()) != 0 || !strings.Contains(r.texts()[0], "Какую убрать?") {
		t.Fatalf("несколько антивирусов — ждали вопрос: %v", r.texts())
	}
	r = consult(t, s, "conv-spec", "что у меня в спецификации", two[:1])
	if ts := r.texts(); !strings.Contains(ts[0], "**197 010 ₽**") {
		t.Fatalf("состав: %v", ts)
	}
	r = consult(t, s, "conv-spec", "убери антивирус", two[:1])
	if as := r.attachments(); len(as) != 1 || as[0].(out.ConsultantSpecEvent).Action != "removed" {
		t.Fatalf("удаление: %#v", as)
	}
	r = consult(t, s, "conv-spec", "сколько итого", nil)
	if ts := r.texts(); ts[0] != "Сейчас спецификация пустая. Можем продолжить подбор, когда понадобится." {
		t.Fatalf("пустая: %v", ts)
	}
	r = consult(t, s, "conv-svc", "как оплатить по счёту", nil)
	if len(r.attachments()) != 0 || strings.Contains(r.texts()[0], "http") {
		t.Fatalf("без адреса — фраза без ссылки: %v %#v", r.texts(), r.attachments())
	}
	s.consultantLinks = ConsultantLinks{Payment: "https://store.softline.ru/pay/"}
	r = consult(t, s, "conv-svc", "как оплатить по счёту", nil)
	if as := r.attachments(); len(as) != 1 || as[0].(out.ConsultantLink).Title != "Способы оплаты" {
		t.Fatalf("ссылка оплаты: %#v", as)
	}
	r = consult(t, s, "conv-svc", "дай скидку 90%", nil)
	if !strings.HasPrefix(r.texts()[0], "Назначать скидки я не могу") {
		t.Fatalf("скидка: %v", r.texts())
	}
}

// withOfficeFamilies — офисные семейства комплекта в каталоге фикстуры (цены как в каталоге магазина).
func withOfficeFamilies(s *Services) {
	g := s.guide()
	ms := guideProduct("473582", "391338", "Microsoft Office для дома и бизнеса 2024", "microsoft",
		"Microsoft Office для дома и бизнеса 2024, Электронная лицензия на 1 ПК, бессрочная", 36584, "base", nil, nil, nil)
	ya := guideProduct("500705", "422091", "Яндекс 360 для бизнеса. Тариф Основной", "yandex",
		"Яндекс 360 для бизнеса, Тариф Основной (предоставление доступа к сервисам на 1 учетную запись на 1 год)", 6588, "base", gi(12), nil, nil)
	r7 := guideProduct("267672", "222227", "«Р7-Офис» для юридических лиц", "r7", "«Р7-Офис» для юридических лиц, Профессиональный", 0, "base", gi(12), nil, nil)
	r7.PriceRub, r7.PriceOnRequest = nil, true
	for _, p := range []*domain.Product{ms, ya, r7} {
		g.fixedCatalog.products[p.ID] = p
	}
	g.fixedCatalog = newGuideCatalog("test-feed", g.fixedCatalog.products)
}

// Сценарий 1: вопрос, офисные решения на 30 мест (Office 2024 — 36 584 за компьютер, 1 097 520 на 30), «добавь» с
// предложением защиты.
func TestConsultantScenario1Kit(t *testing.T) {
	m := &consultantFakeModel{replies: []string{
		`{"messages":["Давайте соберём. Обычно для офиса нужны офисный пакет, антивирус и резервное копирование. Сколько у вас рабочих мест и что из этого уже есть?"],"attachment":{"type":"none","items":[]},"quick_replies":[]}`,
	}}
	s := guideTestServices(t, "verified", m, "on")
	withOfficeFamilies(s)
	r := consult(t, s, "conv-s1", "Подобрать ПО для офиса", nil)
	if ts := r.texts(); len(ts) != 1 || !strings.Contains(ts[0], "Сколько у вас рабочих мест") || len(r.attachments()) != 0 {
		t.Fatalf("вопрос комплекта: %v %#v", ts, r.attachments())
	}
	m.replies = []string{`{"messages":["Начнём с офисного пакета. {{name:1}} можно купить один раз: {{unit:1}} за компьютер, на {{qty}} выйдет {{total:1}}.","Есть и варианты по подписке. Что для вас важнее?"],"attachment":{"type":"list","items":[1,2,3]},"quick_replies":["Привычный Office","Важнее бюджет","Сравнить варианты"]}`}
	m.calls = 0
	r = consult(t, s, "conv-s1", "30, ничего нет", nil)
	ts := r.texts()
	if len(ts) != 2 || !strings.Contains(ts[0], "**36 584 ₽** за компьютер, на 30 компьютеров выйдет **1 097 520 ₽**") {
		t.Fatalf("офисный пакет: %v (%q)", ts, r.errReason())
	}
	var list *out.ConsultantList
	for _, a := range r.attachments() {
		if l, ok := a.(out.ConsultantList); ok {
			list = &l
		}
	}
	if list == nil || list.Title != "Офисные решения на 30 мест" || len(list.Groups[0].Items) != 3 {
		t.Fatalf("список офисных решений: %#v", list)
	}
	if last := list.Groups[0].Items[2]; last.PriceStatus != "on_request" || last.TotalPriceMinor != nil {
		t.Fatalf("Р7 — цена по запросу: %#v", last)
	}
	r = consult(t, s, "conv-s1", "Давай Microsoft, добавь", nil)
	ts = r.texts()
	if len(ts) != 2 || !strings.HasPrefix(ts[0], "Добавил Microsoft") || !strings.Contains(ts[1], "Антивирус нужен только на рабочие места") {
		t.Fatalf("добавление и допродажа: %v", ts)
	}
	ev, ok := r.attachments()[0].(out.ConsultantSpecEvent)
	if !ok || ev.Quantity != 30 || *ev.TotalPriceMinor != 109752000 {
		t.Fatalf("событие: %#v", r.attachments())
	}
}

// Сценарий 8: смета комплекта — посчитана только защита, остальное «не рассчитано», полной суммы нет.
func TestConsultantScenario8Estimate(t *testing.T) {
	m := &consultantFakeModel{replies: []string{
		`{"messages":["Для нового офиса я бы начал с трёх вещей: офисного пакета, защиты компьютеров и резервного копирования. Всё покупаете с нуля или какие-то лицензии уже есть?"],"attachment":{"type":"none","items":[]},"quick_replies":[]}`,
	}}
	s := guideTestServices(t, "verified", m, "on")
	r := consult(t, s, "conv-s8", "открываем офис на 30 человек", nil)
	if ts := r.texts(); len(ts) != 1 || !strings.Contains(ts[0], "с нуля") {
		t.Fatalf("вопрос: %v", ts)
	}
	m.replies = []string{`{"messages":["Из антивирусов я бы начал с {{name:1}} — на {{qty}} годовая лицензия стоит {{total:1}}.","Где будут храниться рабочие документы и нужен ли вам Microsoft Office?"],"attachment":{"type":"list","items":[1]},"quick_replies":["Нужен Microsoft Office","Рассмотрим альтернативы"]}`}
	m.calls = 0
	r = consult(t, s, "conv-s8", "всё с нуля", nil)
	if r.errReason() != "" || len(r.texts()) != 2 {
		t.Fatalf("ход 2: %v %q", r.texts(), r.errReason())
	}
	var est *out.ConsultantEstimate
	for _, a := range r.attachments() {
		if e, ok := a.(out.ConsultantEstimate); ok {
			est = &e
		}
	}
	if est == nil || len(est.Lines) != 3 {
		t.Fatalf("смета: %#v", est)
	}
	known, notCalc := 0, 0
	for _, l := range est.Lines {
		if l.Status == "known" && l.TotalPriceMinor != nil {
			known++
		}
		if l.Status == "not_calculated" && l.TotalPriceMinor == nil {
			notCalc++
		}
	}
	if known != 1 || notCalc != 2 || !strings.Contains(est.Note, "Полная стоимость пока неизвестна") || est.KnownTotalMinor <= 0 {
		t.Fatalf("строки сметы: %#v", est)
	}
}
