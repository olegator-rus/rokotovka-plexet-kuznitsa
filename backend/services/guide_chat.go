package services

import (
	"fmt"
	"regexp"
	"slices"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Помощник по выбору (контур помощника): сообщение помощника блоками —
// до трёх фраз, карточки с полной стоимостью под условия покупателя, таблица при сравнении, источники, вопрос и
// подсказки следующего шага. Собирается после хода и экрана контура экрана из тех же проверенных полей: модель выбирает
// идентификаторы, названия, цены, суммы, пометки и применимость — код. Фронт только показывает.

// Тексты помощника (CHAT.md §8).
const (
	chatMaxSentences = 3
	chatShowCards    = 4

	chatTextOutOfScope = "Помощник подбирает товары и лицензии. Условия оплаты — в разделе магазина."
	chatTextHardware   = "Помощник подбирает лицензии на программы. Компьютеры и серверы — в выдаче поиска слева."
	chatTextDiscount   = "Не могу изменить цену товара. Могу показать действующие акции."
	chatTextNoPrompt   = "Этого показать не могу. Помощник подбирает товары и лицензии."
	chatTextLLMFailed  = "Товары найдены, но не удалось подготовить пояснение."
	chatTextFailed     = "Не удалось получить товары."
	chatTextNoOffers   = "Не нашёл лицензию под эти условия."
	chatTextNoData     = "Нет сведений в документации."
	chatTextPromo      = "На часть лицензий действует акция."
	chatTextAssumeBoth = "Считаю новую лицензию на один год."
	chatTextAssumeTerm = "Рассчитываю на один год."
	chatTextPrelim     = "Результат предварительный: часть условий лицензирования ещё сверяется."

	chatGroupOther      = "Другие варианты"
	chatGroupNotServer  = "Не подходят для серверной ОС"
	chatGroupNotWS      = "Только для серверной ОС"
	chatGroupNotFeature = "Не соответствуют требованию"
	chatGroupNotRules   = "Не подходят под условия"
)

// Виды ответа вне подбора (для текста помощника).
const (
	chatOutService  = "service"  // оплата, доставка, счета, постороннее
	chatOutDiscount = "discount" // скидка, «отправь», «забудь правила»
	chatOutPrompt   = "prompt"   // просьба показать инструкции
	chatOutHardware = "hardware" // оборудование
)

// chatModelFailures — модель была нужна (реплика в подборе) и не ответила: текст заменяется сообщением о сбое.
var chatModelFailures = map[string]bool{guideFbNoModel: true, guideFbOff: true, guideFbBudget: true, guideFbIPQuota: true,
	guideFbBusy: true, guideFbError: true, guideFbTimeout: true, guideFbSchema: true}

// reChatSentence — граница фразы: точка, «!» или «?» и пробел перед заглавной или кавычкой.
var reChatSentence = regexp.MustCompile(`([.!?])\s+([«"A-ZА-ЯЁ0-9])`)

// chatSentences — фразы текста ("" — нет).
func chatSentences(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	parts := strings.Split(reChatSentence.ReplaceAllString(s, "$1\x00$2"), "\x00")
	var out []string
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}

// chatMessage — сообщение помощника к готовому ответу хода (после screen).
func (t *guideTurn) chatMessage(res *out.GuideResponse) {
	if res == nil {
		return
	}
	c := &out.GuideChat{Text: []string{}, Groups: []out.GuideChatGroup{}, Sources: []out.GuideScreenSource{},
		Complements: []out.GuideChatComplement{}, Suggestions: []out.GuideChatSuggestion{}, Links: []out.GuideChatLink{}}
	c.Conditions = t.chatConditions()
	res.Chat = c
	switch res.Type {
	case out.GuideOutOfScope:
		t.chatOutOfScope(c)
		return
	case out.GuideFallback:
		c.Status = out.ChatFailed
		c.Text = []string{chatTextFailed}
		return
	}
	c.Groups = t.chatGroups(res)
	cards := 0
	for _, g := range c.Groups {
		cards += len(g.Cards)
		for _, cd := range g.Cards {
			c.HasPromo = c.HasPromo || cd.Promo
		}
	}
	if res.Type == out.GuideComparison {
		c.Comparison = t.chatTable(res)
	}
	c.Sources = t.chatSources(res)
	if q := res.Clarification; q != nil && (q.Required || cards == 0) {
		c.Question = chatQuestion(q)
	}
	if t.matchTotal > chatShowCards && t.src != "" && t.st.Turn == 1 {
		c.More = &out.GuideChatMore{Label: fmt.Sprintf("Показать все %d %s в поиске", t.matchTotal,
			guidePlural(t.matchTotal, "вариант", "варианта", "вариантов")), Query: t.src, Count: t.matchTotal}
	}
	c.Links = t.chatLinks(res)
	switch {
	case res.Type == out.GuideNoData && res.Docs != nil:
		c.Status = out.ChatNoData
		c.Text = orSentences(res.Lead, chatTextNoData)
	case res.Type == out.GuideNoData || (cards == 0 && c.Question == nil):
		c.Status = out.ChatNoOffers
		c.Text = []string{chatTextNoOffers}
		if s := chatSentences(res.Lead); len(s) > 0 {
			c.Text = s
		}
	case c.Question != nil && cards == 0:
		c.Status = out.ChatQuestion
		c.Text = chatSentences(res.Lead)
	default:
		c.Status = out.ChatAnswer
		c.Text = t.chatText(res, c)
		if t.text != "" && res.Type == out.GuideSelection && chatModelFailures[t.meta.Fallback] {
			// Фраза модели не готова: остаются фразы кода (ступень, цена серверной), затем сообщение о сбое.
			c.Status = out.ChatLLMFailed
			kept := []string{}
			for _, x := range c.Text {
				if strings.HasPrefix(x, "На ") && strings.Contains(x, "ценовая ступень") {
					kept = append(kept, x)
				}
			}
			c.Text = append(kept, chatTextLLMFailed)
		}
		if c.Question != nil {
			c.Status = out.ChatQuestion
		}
	}
	if res.Preliminary && cards > 0 && c.Question == nil {
		c.Warning = chatTextPrelim
	}
	if len(c.Text) > chatMaxSentences {
		c.Text = c.Text[:chatMaxSentences]
	}
	if c.Question == nil {
		c.Suggestions = t.chatSuggestions(res, c)
	}
}

func orSentences(s, def string) []string {
	if x := chatSentences(s); len(x) > 0 {
		return x
	}
	return []string{def}
}

// chatOutOfScope — не его тема, скидка, оборудование: короткая переадресация без карточек.
func (t *guideTurn) chatOutOfScope(c *out.GuideChat) {
	c.Status = out.ChatOutOfScope
	switch t.outKind {
	case chatOutDiscount:
		c.Status = out.ChatDiscount
		c.Text = []string{chatTextDiscount}
		c.Suggestions = []out.GuideChatSuggestion{{Label: "Показать акции", Message: "покажи акции"}}
	case chatOutPrompt:
		c.Text = []string{chatTextNoPrompt}
	case chatOutHardware:
		c.Text = []string{chatTextHardware}
		if t.src != "" {
			c.More = &out.GuideChatMore{Label: "Показать в поиске", Query: t.src}
		}
	default:
		c.Text = []string{chatTextOutOfScope}
		c.Links = []out.GuideChatLink{{Kind: "payment", Label: "Перейти к условиям оплаты", URL: "/help"}}
	}
}

// --- строка условий ---

// chatConditions — «Учитываю: 20 ПК · 1 год · новая · шифрование»: число, срок, тип покупки, функции, назначение.
func (t *guideTurn) chatConditions() out.GuideChatConditions {
	r := t.st.Requirements
	items := []string{}
	for _, o := range r.Objects {
		if o.Count > 0 {
			items = append(items, countLabel(o.Kind, o.Count))
		} else if o.Kind == "server" {
			items = append(items, "для сервера")
		}
	}
	if r.Use == "tls_server" {
		items = append(items, "TLS-сервер")
	}
	if r.Variant != "" {
		items = append(items, "«"+r.Variant+"»")
	}
	if r.TermMonths > 0 {
		items = append(items, termWords(r.TermMonths))
	}
	if r.Operation == "renewal" {
		items = append(items, "продление")
	} else if r.Operation == domain.LicenseKindMigration {
		items = append(items, "переход")
	} else {
		items = append(items, "новая")
	}
	for _, f := range r.RequiredFeatures {
		items = append(items, featureNoun(f, t.kb))
	}
	if r.OrgType != "" && r.OrgType != defaultOrg {
		items = append(items, orgLabel(r.OrgType))
	}
	label := ""
	if t.st.Turn > 0 && (len(r.Objects) > 0 || r.Vendor != "" || r.Task != "" || len(t.st.CandidateFamilyIDs) > 0) {
		label = "Учитываю: " + strings.Join(items, " · ")
	}
	return out.GuideChatConditions{Label: label, Items: items}
}

// --- карточки ---

// chatGroups — подходящие (без заголовка) · «Другие варианты» · «Не подходят …». Неподходящее не прячется.
func (t *guideTurn) chatGroups(res *out.GuideResponse) []out.GuideChatGroup {
	var match, other, rejected []out.GuideChatCard
	rejTitle := ""
	featureBasis := res.Screen != nil && res.Screen.Result != nil && res.Screen.Result.Basis == out.BasisRequiredFeature
	for i := range res.Offers {
		o := &res.Offers[i]
		cd := t.chatCard(res, o)
		code := ""
		if o.Reason != nil {
			code = o.Reason.Code
		}
		switch {
		case o.Role == out.OfferMatch && featureBasis && (code == "feature_unknown" || code == "missing_feature"):
			other = append(other, cd)
		case o.Role == out.OfferMatch:
			match = append(match, cd)
		case o.Role == out.OfferAlternative && code == "tls_server_only":
			match = append(match, cd)
		case o.Role == out.OfferAlternative && code == "missing_feature":
			cd.Unsuitable, cd.Action = true, out.ChatActionStore
			cd.Badge = &out.GuideReason{Code: "not_required", Text: "Не соответствует требованию"}
			cd.Reason = missingFeatureLine(o.Reason.Text)
			rejected = append(rejected, cd)
			rejTitle = orDefault(rejTitle, chatGroupNotFeature)
		case o.Role == out.OfferAlternative:
			other = append(other, cd)
		case o.Role == out.OfferRejected:
			cd.Unsuitable, cd.Action = true, out.ChatActionStore
			switch code {
			case "not_for_server_os":
				cd.Badge = &out.GuideReason{Code: "unsuitable", Text: "Не подходит для сервера"}
				cd.Reason = "Лицензия для рабочей станции"
				rejTitle = orDefault(rejTitle, chatGroupNotServer)
			case "server_only":
				cd.Badge = &out.GuideReason{Code: "unsuitable", Text: "Только для серверной ОС"}
				rejTitle = orDefault(rejTitle, chatGroupNotWS)
			default:
				cd.Badge = &out.GuideReason{Code: "unsuitable", Text: "Не подходит"}
				rejTitle = orDefault(rejTitle, chatGroupNotRules)
			}
			rejected = append(rejected, cd)
		}
	}
	t.chatBadges(res, match)
	groups := []out.GuideChatGroup{}
	if len(match) > 0 {
		groups = append(groups, out.GuideChatGroup{Key: out.ChatGroupMatch, Title: "", Cards: match})
	}
	if len(other) > 0 {
		groups = append(groups, out.GuideChatGroup{Key: out.ChatGroupOther, Title: chatGroupOther, Cards: other})
	}
	if len(rejected) > 0 {
		groups = append(groups, out.GuideChatGroup{Key: out.ChatGroupRejected, Title: rejTitle, Cards: rejected})
	}
	return groups
}

// missingFeatureLine — «Не подходит: нет шифрования данных» → «Шифрования данных в этой редакции нет».
func missingFeatureLine(s string) string {
	if f, ok := strings.CutPrefix(s, "Не подходит: нет "); ok {
		return guideUpperFirst(f) + " в этой редакции нет"
	}
	return s
}

// chatCard — карточка из оффера хода: полная стоимость, расчёт, причина, акция, действие.
func (t *guideTurn) chatCard(res *out.GuideResponse, o *out.GuideOffer) out.GuideChatCard {
	cd := out.GuideChatCard{OfferID: o.OfferID, FamilyID: o.FamilyID, Title: o.FamilyTitle, PriceState: o.PriceState,
		OfferURL: o.OfferURL, Quantity: max(o.Quantity, 1), Total: o.Total, Details: []out.GuideDetail{}, Action: out.ChatActionSpec}
	if d := o.Display; d != nil {
		cd.Title, cd.Subtitle = orDefault(d.Title, o.FamilyTitle), d.Subtitle
		cd.Details = append(cd.Details, d.Details...)
	}
	if p := t.cat.products[o.OfferID]; p != nil {
		if v := t.cat.view(p, t.kb); v.variant != "" && !strings.Contains(cd.Title, variantLabel(v.variant)) {
			cd.Title += " — «" + variantLabel(v.variant) + "»"
		}
	}
	switch {
	case o.PriceState == out.PriceOnRequest || (o.Total == nil && o.UnitPrice == nil):
		cd.PriceText, cd.Action, cd.Total = screenOnRequest, out.ChatActionQuote, nil
	case o.Total != nil:
		cd.PriceText = rubRU(*o.Total) + " ₽"
	default:
		cd.PriceText = o.PriceText // количество вне ступени или неизвестно: цена за единицу, без выдуманного итога
	}
	if cd.PriceState != out.PriceOnRequest && o.UnitPrice != nil {
		if pk := o.Package; o.SaleUnitKind == "package" && pk != nil {
			if pk.Packs > 1 {
				cd.Calc = fmt.Sprintf("%d × %s ₽ · %s", pk.Packs, rubRU(*o.UnitPrice), o.SaleUnit)
			} else {
				cd.Calc = guideUpperFirst(o.SaleUnit)
			}
			if pk.Spare > 0 {
				cd.Calc += fmt.Sprintf(" · %d %s в запасе", pk.Spare, guidePlural(pk.Spare, "место", "места", "мест"))
			}
		} else if o.PayQuantity > 1 && o.PriceLine != "" {
			cd.Calc = o.PriceLine
		}
	}
	if o.Reason != nil && o.Role != out.OfferMatch {
		cd.Reason = o.Reason.Text
	} else if o.Reason != nil && o.Reason.Code != "" {
		cd.Reason = o.Reason.Text
	}
	if p := t.cat.products[o.OfferID]; p != nil && p.OldPriceRub != nil && p.PriceRub != nil && *p.OldPriceRub > *p.PriceRub && *p.PriceRub > 0 {
		cd.Promo = true
		cd.OldPriceText = rubRU(*p.OldPriceRub) + " ₽ " + saleUnitPhrase(o.SaleUnit)
	}
	if res.Screen != nil && res.Screen.Changed != nil && res.Screen.Changed.PriceChange != nil && res.Screen.Changed.PriceChange.OfferID == o.OfferID {
		pc := res.Screen.Changed.PriceChange
		word := "ниже"
		if pc.To.UnitPrice > pc.From.UnitPrice {
			word = "выше"
		}
		cd.Note = fmt.Sprintf("Цена за место %s: %s вместо %s", word, pc.To.Text, pc.From.Text)
	}
	if o.Role == out.OfferMatch && o.Note != "" && strings.HasPrefix(o.Note, "Новая лицензия") {
		cd.Note = o.Note
	}
	return cd
}

// chatBadges — одна пометка на подходящую карточку при проверяемом основании (CHAT.md §5); «Рекомендуем» нет.
func (t *guideTurn) chatBadges(res *out.GuideResponse, cards []out.GuideChatCard) {
	if len(cards) == 0 {
		return
	}
	kind := t.screenKind()
	var featureOffer, featureText string
	if res.Screen != nil && res.Screen.Result != nil && res.Screen.Result.Basis == out.BasisRequiredFeature {
		featureOffer = res.Screen.Result.OfferID
		featureText = "Есть нужная функция"
		if r := t.st.Requirements; len(r.RequiredFeatures) == 1 && r.RequiredFeatures[0] == "encryption" {
			featureText = "Есть нужное шифрование"
		}
	}
	for i := range cards {
		cd := &cards[i]
		p := t.cat.products[cd.OfferID]
		if p == nil {
			continue
		}
		v := t.cat.view(p, t.kb)
		switch {
		case cd.OfferID == featureOffer:
			cd.Badge = &out.GuideReason{Code: "required_feature", Text: featureText}
		case kind == "server" && v.scope == "server":
			cd.Badge = &out.GuideReason{Code: "server_os", Text: "Для серверной ОС"}
		case v.kind == domain.LicenseKindMigration:
			cd.Badge = &out.GuideReason{Code: "migration", Text: "Доступна лицензия для перехода"}
		}
	}
	// Меньшая стоимость — при одинаковых количестве и сроке, среди подходящих с итогом.
	priced, terms, unknown := 0, map[int]bool{}, false
	best := -1
	for i, cd := range cards {
		if cd.Unsuitable {
			continue
		}
		if cd.Total == nil {
			unknown = true
			continue
		}
		if p := t.cat.products[cd.OfferID]; p != nil {
			terms[t.cat.view(p, t.kb).term] = true
		}
		priced++
		if best < 0 || *cd.Total < *cards[best].Total {
			best = i
		}
	}
	if priced >= 2 && len(terms) == 1 && best >= 0 && cards[best].Badge == nil && featureOffer == "" && res.Type != out.GuideComparison &&
		res.Clarification == nil {
		text := "Меньшая стоимость среди найденных"
		if unknown {
			text = "Меньшая стоимость среди вариантов с известной ценой"
		}
		cards[best].Badge = &out.GuideReason{Code: "lowest_total", Text: text}
		if best > 0 {
			b := cards[best]
			copy(cards[1:best+1], cards[0:best])
			cards[0] = b
		}
	}
}

// --- таблица ---

// chatTable — две редакции, три-пять строк: о чём спросили, различия, затем «За место» и «Итого».
func (t *guideTurn) chatTable(res *out.GuideResponse) *out.GuideChatTable {
	if res.Screen == nil || res.Screen.Comparison == nil || len(res.Screen.Comparison.Columns) < 2 {
		return nil
	}
	sc := res.Screen.Comparison
	cols := sc.Columns[:2]
	all := append(append([]out.GuideScreenRow{}, sc.Rows...), sc.MoreRows...)
	slices.SortStableFunc(all, func(a, b out.GuideScreenRow) int { return a.Priority - b.Priority })
	tbl := &out.GuideChatTable{Title: sc.Title, Columns: cols, Rows: []out.GuideChatRow{}, Source: t.relabelSource(sc.Source, res)}
	features := 0
	for _, r := range all {
		if r.Kind != "feature" || features >= 3 || len(r.Cells) < 2 {
			continue
		}
		if r.Cells[0].Mark == out.MarkUnknown && r.Cells[1].Mark == out.MarkUnknown {
			continue
		}
		tbl.Rows = append(tbl.Rows, out.GuideChatRow{Key: r.Key, Label: r.Label, Kind: r.Kind, Cells: r.Cells[:2]})
		features++
	}
	if features == 0 {
		return nil
	}
	for _, r := range all {
		if r.Key == "unit_price" && len(r.Cells) >= 2 {
			tbl.Rows = append(tbl.Rows, out.GuideChatRow{Key: r.Key, Label: "За место", Kind: "commercial", Cells: r.Cells[:2]})
			break
		}
	}
	total := out.GuideChatRow{Key: "total", Label: "Итого", Kind: "commercial", Cells: []out.GuideScreenCell{}}
	for _, col := range cols {
		cell := out.GuideScreenCell{Mark: out.MarkUnknown, Text: "Нет сведений"}
		for _, o := range res.Offers {
			if o.OfferID == col.OfferID {
				if o.Total != nil {
					cell = out.GuideScreenCell{Mark: out.MarkValue, Text: rubRU(*o.Total) + " ₽"}
				} else {
					cell = out.GuideScreenCell{Mark: out.MarkValue, Text: screenOnRequest}
				}
			}
		}
		total.Cells = append(total.Cells, cell)
	}
	tbl.Rows = append(tbl.Rows, total)
	return tbl
}

// --- источники ---

// relabelSource — «Документация вендора» → «Документация Kaspersky»; правила — «Условия лицензирования {вендор}».
func (t *guideTurn) relabelSource(s *out.GuideScreenSource, res *out.GuideResponse) *out.GuideScreenSource {
	if s == nil {
		return nil
	}
	c := *s
	if v, ok := strings.CutPrefix(c.Label, "Правила лицензирования "); ok {
		c.Label = "Условия лицензирования " + v
		return &c
	}
	if c.Label == screenSourceLabel || c.Label == "" {
		if v := t.chatVendor(res); v != "" {
			c.Label = "Документация " + v
		} else {
			c.Label = screenSourceLabel
		}
	}
	return &c
}

// chatVendor — вендор подходящих карточек, если он один и у него есть имя на витрине ("" — разные или нет имени).
func (t *guideTurn) chatVendor(res *out.GuideResponse) string {
	vend := ""
	for _, o := range res.Offers {
		if o.Role == out.OfferRejected {
			continue
		}
		v := t.cat.famVend[o.FamilyID]
		if vend != "" && v != vend {
			return ""
		}
		vend = v
	}
	if vend == "" && t.st.Requirements.Vendor != "" {
		vend = t.st.Requirements.Vendor
	}
	if l := vendorLabel(vend); l != vend {
		return l
	}
	return ""
}

// chatSources — один источник на документ: таблица, основание результата, утверждения, документация «нет сведений».
func (t *guideTurn) chatSources(res *out.GuideResponse) []out.GuideScreenSource {
	srcs := []out.GuideScreenSource{}
	seen := map[string]bool{}
	add := func(s *out.GuideScreenSource) {
		if s == nil || s.URL == "" || seen[s.URL] {
			return
		}
		seen[s.URL] = true
		srcs = append(srcs, *s)
	}
	if sc := res.Screen; sc != nil {
		if res.Type == out.GuideComparison && sc.Comparison != nil {
			add(t.relabelSource(sc.Comparison.Source, res))
		}
		if sc.Result != nil {
			add(t.relabelSource(sc.Result.Source, res))
		}
	}
	for _, c := range res.Claims {
		add(t.relabelSource(&out.GuideScreenSource{Label: c.Source.Label, Fragment: c.Source.Fragment, URL: c.Source.URL}, res))
	}
	if res.Docs != nil {
		add(t.relabelSource(&out.GuideScreenSource{Label: screenSourceLabel, URL: res.Docs.URL}, res))
	}
	if len(srcs) > 2 {
		srcs = srcs[:2]
	}
	return srcs
}

// --- вопрос и подсказки ---

// chatQuestion — вопрос с двумя-четырьмя кнопками; «другое» не нужно — ответить можно текстом.
func chatQuestion(q *out.GuideClarify) *out.GuideChatQuestion {
	chips := []out.GuideChip{}
	for _, ch := range q.Chips {
		if ch.Value == "" && ch.Key != "feature" {
			continue
		}
		if len(chips) >= 4 {
			break
		}
		chips = append(chips, ch)
	}
	return &out.GuideChatQuestion{Key: q.Key, Text: q.Question, Chips: chips}
}

// chatCountSteps — следующее «круглое» число мест для подсказки «Рассчитать на 50 ПК».
var chatCountSteps = []int{10, 20, 30, 50, 100, 200, 500}

func nextCount(n int) int {
	for _, s := range chatCountSteps {
		if s > n {
			return s
		}
	}
	return 0
}

// chatSuggestions — две-три подсказки следующего шага по условиям и ответу.
func (t *guideTurn) chatSuggestions(res *out.GuideResponse, c *out.GuideChat) []out.GuideChatSuggestion {
	r := t.st.Requirements
	var sg []out.GuideChatSuggestion
	add := func(s out.GuideChatSuggestion) {
		if len(sg) < 3 {
			sg = append(sg, s)
		}
	}
	if c.Status != out.ChatAnswer && c.Status != out.ChatLLMFailed {
		if c.Status == out.ChatNoOffers || c.Status == out.ChatNoData {
			if r.Operation != "renewal" && len(r.Objects) > 0 {
				add(out.GuideChatSuggestion{Label: "Без срока", Message: "любой срок", Chip: &out.GuideChip{Key: "term_months", Value: ""}})
			}
		}
		return nonNilSuggestions(sg)
	}
	// Функция, которая разводит варианты (не требуемая): «Нужно шифрование».
	if q := res.Clarification; q != nil && q.Key == "feature" && !q.Required {
		for _, ch := range q.Chips {
			if ch.Value != "" {
				label := guideUpperFirst(ch.Label)
				add(out.GuideChatSuggestion{Label: label, Message: strings.ToLower(ch.Label), Chip: &out.GuideChip{Key: ch.Key, Value: ch.Value}})
				break
			}
		}
	} else if c.Comparison != nil && len(r.RequiredFeatures) == 0 {
		for _, row := range c.Comparison.Rows {
			if row.Kind == "feature" && len(row.Cells) == 2 && row.Cells[0].Mark != row.Cells[1].Mark &&
				(row.Cells[0].Mark == out.MarkYes || row.Cells[1].Mark == out.MarkYes) {
				noun := featureNoun(row.Key, t.kb)
				if _, ok := featureShort[row.Key]; !ok && row.Key != "encryption" {
					continue
				}
				add(out.GuideChatSuggestion{Label: "Нужно " + noun, Message: "нужно " + noun})
				break
			}
		}
	}
	kind, n := mainObject(r)
	if kind == "server" && r.Use == "" && t.hasTLSCard(res) {
		add(out.GuideChatSuggestion{Label: "Сервер для TLS", Message: "сервер для TLS", Chip: &out.GuideChip{Key: "use", Value: "tls_server"}})
	}
	if kind == "server" && n <= 1 {
		add(out.GuideChatSuggestion{Label: "На 3 сервера", Message: "на 3 сервера"})
	}
	if nc := nextCount(max(n, 1)); n > 0 && nc > 0 && kind != "server" {
		label := fmt.Sprintf("Рассчитать на %s", countLabel(kind, nc))
		add(out.GuideChatSuggestion{Label: label, Message: "на " + objectCountText(kind, nc)})
	}
	if len(r.RequiredFeatures) > 0 && r.TermMonths == 12 {
		add(out.GuideChatSuggestion{Label: "На 2 года", Message: "на 2 года", Chip: &out.GuideChip{Key: "term_months", Value: "24"}})
	}
	if r.Operation != "renewal" && r.Operation != domain.LicenseKindMigration && kind != "server" {
		add(out.GuideChatSuggestion{Label: "Это продление", Message: "это продление", Chip: &out.GuideChip{Key: "operation", Value: "renewal"}})
	}
	return nonNilSuggestions(sg)
}

func nonNilSuggestions(s []out.GuideChatSuggestion) []out.GuideChatSuggestion {
	if s == nil {
		return []out.GuideChatSuggestion{}
	}
	return s
}

func (t *guideTurn) hasTLSCard(res *out.GuideResponse) bool {
	for _, o := range res.Offers {
		if o.Reason != nil && o.Reason.Code == "tls_server_only" {
			return true
		}
	}
	return false
}

// --- текст ---

// chatText — до трёх фраз: смена ступени, связующая фраза хода (проверенная), допущение по сроку и типу покупки.
func (t *guideTurn) chatText(res *out.GuideResponse, c *out.GuideChat) []string {
	r := t.st.Requirements
	var s []string
	if res.Screen != nil && res.Screen.Changed != nil && res.Screen.Changed.PriceChange != nil {
		kind, n := mainObject(r)
		if n > 0 {
			s = append(s, fmt.Sprintf("На %s действует другая ценовая ступень.", objectCountText(orDefault(kind, "workstation"), n)))
		}
	}
	for _, x := range chatSentences(res.Lead) {
		if !strings.HasSuffix(x, ".") && !strings.HasSuffix(x, "!") && !strings.HasSuffix(x, "?") {
			x += "."
		}
		s = append(s, x)
	}
	if t.serverCheapest(res) != "" {
		s = append(s, t.serverCheapest(res))
	}
	kind, n := mainObject(r)
	opDefault := t.st.Origin["operation"] != originUser
	termDefault := t.st.Origin["term_months"] != originUser && r.TermMonths == 12
	switch {
	case t.before.shown:
		// условия уже названы в прошлом ответе и видны в строке «Учитываю»
	case res.Type == out.GuideComparison && n > 0 && r.TermMonths == 12:
		s = append(s, fmt.Sprintf("Стоимость посчитана на %s на один год.", objectCountText(orDefault(kind, "workstation"), n)))
	case opDefault && termDefault && r.Operation != "renewal":
		s = append(s, chatTextAssumeBoth)
	case termDefault:
		s = append(s, chatTextAssumeTerm)
	}
	if c.HasPromo && len(s) < chatMaxSentences {
		s = append(s, chatTextPromo)
	}
	return s
}

// serverCheapest — «Серверная лицензия стоит 70 000 ₽.»: просьба о самой дешёвой для сервера, одна подходящая.
func (t *guideTurn) serverCheapest(res *out.GuideResponse) string {
	r := t.st.Requirements
	kind, _ := mainObject(r)
	if !r.Cheapest || kind != "server" {
		return ""
	}
	for _, o := range res.Offers {
		if o.Role == out.OfferMatch && o.Total != nil {
			if p := t.cat.products[o.OfferID]; p != nil && t.cat.view(p, t.kb).scope == "server" {
				return "Серверная лицензия стоит " + rubRU(*o.Total) + " ₽."
			}
			return ""
		}
	}
	return ""
}

// chatLinks — ссылки под ответом. Конфигуратора Astra в каталоге нет (ссылки в фиде нет) — его не показываем.
func (t *guideTurn) chatLinks(res *out.GuideResponse) []out.GuideChatLink {
	_ = res
	return []out.GuideChatLink{}
}

// --- соседние редакции при сравнении ---

// neighbourEditions — сравнение двух редакций: ниже таблицы ещё до двух карточек «Другие варианты» — соседние
// редакции той же линейки (Расширенный Плюс), затем другие продукты того же вендора из направления задачи, с ценой
// на те же условия и названным отличием. Подходящими они не становятся (роль alternative).
func (t *guideTurn) neighbourEditions(res *out.GuideResponse, data *guideData) {
	if res == nil || t.cross || len(res.Offers) < 2 || len(res.Offers) >= chatShowCards {
		return
	}
	r := t.st.Requirements
	shown := map[string]bool{}
	var fams []string
	for _, o := range res.Offers {
		shown[o.FamilyID] = true
		fams = append(fams, o.FamilyID)
	}
	vendor := t.cat.famVend[fams[0]]
	var cands []string
	for _, f := range t.cat.lineFamilies(t.kb, fams) {
		if !shown[f] {
			cands = append(cands, f)
		}
	}
	line := len(cands)
	for _, f := range t.kb.subclass(directionOf(orDefault(r.Task, "endpoint"))) {
		if !shown[f] && !containsStr(cands, f) && t.cat.famVend[f] == vendor {
			cands = append(cands, f)
		}
	}
	for i, fid := range cands {
		if len(res.Offers) >= chatShowCards {
			break
		}
		if lim := t.cat.familyLimit(fid, t.kb); lim > 0 && data.qty > lim {
			continue
		}
		views, _ := t.cat.familyOffers(fid, r, t.st.Origin["term_months"], t.kb)
		var ok []guideOfferView
		for _, v := range views {
			if v.priced() && len(validateLicense(v, r, t.kb, termAny)) == 0 && v.term == 12 {
				ok = append(ok, v)
			}
		}
		if len(ok) == 0 {
			continue
		}
		sortOffers(ok, data.qty, t.cat.famName[fid])
		note := "Другая редакция линейки"
		if i >= line {
			note = "Другой продукт " + orDefault(vendorLabel(vendor), "вендора")
		}
		if len(t.kb.factsFor([]string{fid}, r.Platform)) == 0 {
			note += "; состав функций в документации не подтверждён"
		}
		if t.altFams == nil {
			t.altFams = map[string]bool{}
		}
		t.altFams[fid] = true
		res.Offers = append(res.Offers, t.offerOut(ok[0], data.qty, data.kind, out.OfferAlternative, note))
	}
}
