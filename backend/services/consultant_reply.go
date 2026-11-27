package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/prompts"
)

// Второй вызов модели: текст по листу фактов со вставками. Код подставляет вставки и проверяет текст
// (CORE.md §2): несуществующий номер, число с ₽/руб/% вне вставки, адрес сайта — один повтор, затем запасной ответ.

const (
	consultantCallTimeout = 9 * time.Second
	consultantMaxTokens   = 700
	consultantMaxMessages = 2
	consultantMaxQuick    = 3
	consultantQuickRunes  = 24
)

// consultantReply — ответ модели.
type consultantReply struct {
	Messages   []string `json:"messages"`
	Attachment struct {
		Type  string `json:"type"`
		Items []int  `json:"items"`
		Topic string `json:"topic"`
	} `json:"attachment"`
	QuickReplies []string `json:"quick_replies"`
}

// replyModel — модель второго вызова: CONSULTANT_REPLY_MODEL, иначе модель подбора.
func (t *consultantTurn) replyModel() ChatModel {
	if t.s.consultantModel != nil {
		return t.s.consultantModel
	}
	return t.g.model()
}

// selectionTurn — ход подбора: /api/guide → лист фактов → текст.
func (t *consultantTurn) selectionTurn(ctx context.Context) consultantPlan {
	t.status(consultantStepSearching)
	state := t.conv.guide
	if t.topicChanged() {
		// Другой продукт или класс продуктов без нового количества: новый подбор, условия прошлой темы не переносятся.
		state = nil
	}
	msg := t.req.Text
	if state == nil {
		msg += t.pageProductHint()
	}
	greq := in.GuideRequest{SessionID: t.req.SessionID, Message: msg, State: state}
	res, err := t.s.Guide(ctx, t.requestID, greq)
	if err != nil || res == nil {
		return t.fallback(consultantErrTurn, t.conv.sheet)
	}
	st := res.State
	t.conv.guide = &st
	t.status(consultantStepCalculating)
	sh := t.sheetFromGuide(res)
	if res.Type == out.GuideFallback && len(sh.Items) == 0 {
		return t.fallback(consultantErrTurn, nil)
	}
	t.topUp(ctx, sh)
	if len(sh.Items) > 0 {
		t.conv.sheet = sh.Items
	}
	if res.Type == out.GuideOutOfScope && len(sh.Items) == 0 && countOnlyRe.MatchString(guideNorm(t.req.Text)) {
		// «Компьютеров будет 50» без темы (разговор начат заново): не оборудование — спросить, что подбираем.
		return consultantPlan{messages: []string{"Количество учту. Подскажите, какой продукт подбираем?"}}
	}
	if res.Type == out.GuideOutOfScope && len(sh.Items) == 0 {
		// Подбор признал тему не своей: если поиск по каталогу что-то нашёл — список; иначе фраза подбора без
		// отсылок к кнопкам экрана подбора.
		if items := t.searchItemsCtx(ctx, t.req.Text, 0); len(items) > 0 {
			p := consultantPlan{messages: []string{"Вот что нашлось в каталоге по вашему запросу. Уточните, что именно нужно, — подберу точнее."}}
			if l := listAttachment(items, ""); l != nil {
				p.attachments = append(p.attachments, *l)
			}
			return p
		}
		var msgs []string
		if res.Chat != nil {
			for _, m := range res.Chat.Text {
				if l := strings.ToLower(m); !strings.Contains(l, "кнопк") && !strings.Contains(l, "справк") && !hasMoneyNumber(m) {
					msgs = append(msgs, m)
				}
			}
		}
		if len(msgs) == 0 {
			msgs = []string{"Я подбираю лицензии на программы и помогаю с вопросами по ним. Расскажите, какую задачу нужно решить."}
		}
		return consultantPlan{messages: msgs}
	}
	p := t.write(ctx, sh)
	if sh.Assumption != "" && p.errReason == "" {
		t.conv.assumed = true
	}
	return p
}

// topUp — список в ходе подбора: не меньше четырёх позиций (вместе с «Не подойдут», как в сценарии 3); добор —
// «Ещё посмотрите» из поиска по каталогу.
func (t *consultantTurn) topUp(ctx context.Context, sh *consultantSheet) {
	if sh.attachmentKind() != "list" || len(sh.Items) >= consultantMinList {
		return
	}
	have := map[string]bool{}
	fams := map[string]bool{}
	for _, it := range sh.Items {
		have[it.Offer.OfferID] = true
		fams[it.Offer.FamilyID] = true
	}
	for _, it := range t.searchItemsCtx(ctx, t.req.Text, sh.Qty) {
		if len(sh.Items) >= consultantMinList {
			break
		}
		if have[it.Offer.OfferID] || fams[it.Offer.FamilyID] {
			continue
		}
		have[it.Offer.OfferID], fams[it.Offer.FamilyID] = true, true
		it.Group = consultantMore
		it.N = len(sh.Items) + 1
		sh.Items = append(sh.Items, it)
	}
	// Подходящие и «ещё» — выше неподходящих: переставить, сохранив номера листа.
	var head, tail []consultantItem
	for _, it := range sh.Items {
		if it.Group == consultantUnfit {
			tail = append(tail, it)
		} else {
			head = append(head, it)
		}
	}
	sh.Items = append(head, tail...)
	for i := range sh.Items {
		if sh.TierItem == sh.Items[i].N {
			sh.TierItem = i + 1
		}
		sh.Items[i].N = i + 1
	}
}

// explainTurn — объяснение без товаров (сценарий 4): модель отвечает общими словами, вложений нет.
func (t *consultantTurn) explainTurn(ctx context.Context) consultantPlan {
	sh := &consultantSheet{Mode: "explain"}
	p := t.write(ctx, sh)
	p.attachments = nil
	return p
}

// pageTurn — «расскажи про этот» на странице товара: позиция страницы — единственная в листе.
func (t *consultantTurn) pageTurn(ctx context.Context) (consultantPlan, bool) {
	pg := t.req.Page
	if pg == nil || pg.Type != "product" || pg.OfferID == nil {
		return consultantPlan{}, false
	}
	server := pageServerRe.MatchString(guideNorm(t.req.Text))
	if !consultantDeictic(t.req.Text) && !server {
		return consultantPlan{}, false
	}
	p := t.cat.products[*pg.OfferID]
	if p == nil {
		return consultantPlan{}, false
	}
	it := t.offerItem(p, 1, consultantFit)
	it.N = 1
	if it.Old != nil {
		it.Mark = "Цена по акции"
	}
	sh := &consultantSheet{Mode: "page", Items: []consultantItem{it}, Kind: "workstation", Qty: 1}
	scope := t.cat.view(p, t.kb).scope
	switch {
	case server && scope == "workstation":
		sh.Facts = append(sh.Facts, consultantFact{Text: "Позиция открытой страницы — лицензия для рабочей станции. Подтверждения, что она подходит для сервера, в данных нет."})
		sh.Question = "Ответь одним сообщением: эта карточка — лицензия для рабочей станции, подтверждения, что она подходит для сервера, нет; предложи поискать серверный вариант («Могу поискать серверный вариант»). Не утверждай, что серверный вариант есть, и не пиши, что система «не установится»."
		sh.Chips = []string{"Найди серверный"}
		sh.NoAttach = true
	case server && scope == "server":
		sh.Facts = append(sh.Facts, consultantFact{Text: "Позиция открытой страницы — серверная лицензия."})
	case server:
		sh.Facts = append(sh.Facts, consultantFact{Text: "Назначение лицензии этой позиции (рабочая станция или сервер) в данных не указано."})
	}
	t.conv.sheet = sh.Items
	return t.write(ctx, sh), true
}

var pageServerRe = regexp.MustCompile(`сервер.*(встан|подойд|подход|можно|работ|пойдет)|(встан|подойд|подход|можно|работ|пойдет).*сервер`)

var deicticRe = regexp.MustCompile(`(?i)(^|[^\p{L}])(этот|эта|это|этом|этого|эту|он|она|него|неё|нее|такой же|такую же)([^\p{L}]|$)`)

func consultantDeictic(s string) bool { return deicticRe.MatchString(s) }

// write — второй вызов модели, подстановка, проверки, один повтор; не прошло — запасной ответ со списком.
func (t *consultantTurn) write(ctx context.Context, sh *consultantSheet) consultantPlan {
	if t.g.modelBlocked(ctx) != "" || t.replyModel() == nil {
		return t.fallback(consultantErrModel, sh.Items)
	}
	t.status(consultantStepChecking)
	user := t.modelInput(sh)
	var lastErr string
	for attempt := 0; attempt < 2; attempt++ {
		t.attempts++
		u := user
		if lastErr != "" {
			u += "\n<previous_error>" + lastErr + ". Исправь: цены, суммы, проценты, названия со ссылками и адреса — только вставками; номера — только из items.</previous_error>"
		}
		reply, reason := t.callReply(ctx, u)
		if reply == nil {
			if reason == guideFbSchema {
				lastErr = "ответ не в формате JSON по схеме"
				continue
			}
			return t.fallback(consultantErrModel, sh.Items)
		}
		msgs, err := substituteReply(splitLong(reply.Messages), sh)
		if err != nil {
			lastErr = err.Error()
			t.rejected = append(t.rejected, lastErr)
			continue
		}
		p := consultantPlan{messages: msgs}
		if !sh.NoAttach {
			p.attachments = t.attachments(sh)
		}
		p.quick = quickReplies(reply.QuickReplies, msgs)
		if len(p.quick) == 0 && hasQuestion(msgs) && len(sh.Chips) > 0 {
			p.quick = quickReplies(sh.Chips, msgs)
		}
		if sh.NoAttach && len(sh.Chips) > 0 {
			// Предложение действия («Могу поискать серверный вариант») — кнопка из настройки хода.
			p.quick = sh.Chips
		}
		return p
	}
	return t.fallback(consultantErrInvalid, sh.Items)
}

// modelInput — вход второго вызова: история, условия, реплика, лист фактов. Текст покупателя — данные в тегах.
func (t *consultantTurn) modelInput(sh *consultantSheet) string {
	b := &strings.Builder{}
	hist := make([]map[string]string, 0, len(t.conv.history))
	for _, l := range t.conv.history {
		txt, _ := maskPII(l.text)
		hist = append(hist, map[string]string{"role": l.role, "text": txt})
	}
	raw, _ := json.Marshal(hist)
	b.WriteString("<history>" + string(raw) + "</history>\n")
	q, _ := json.Marshal(t.req.Text)
	b.WriteString("<message>" + string(q) + "</message>\n")
	b.WriteString("<facts_sheet>" + sh.render() + "</facts_sheet>\n")
	return b.String()
}

// callReply — один вызов модели ответа: семафор подбора, срок, учёт расхода.
func (t *consultantTurn) callReply(ctx context.Context, user string) (*consultantReply, string) {
	m := t.replyModel()
	select {
	case guideSem <- struct{}{}:
	case <-ctx.Done():
		return nil, guideFbBusy
	case <-time.After(2 * time.Second):
		return nil, guideFbBusy
	}
	defer func() { <-guideSem }()
	cctx, cancel := context.WithTimeout(ctx, consultantCallTimeout)
	defer cancel()
	zero := 0.2
	user, _ = maskPII(user)
	req := llm.Request{
		Messages:  []llm.Message{{Role: "system", Content: prompts.Consultant().Text}, {Role: "user", Content: user}},
		MaxTokens: consultantMaxTokens, Temperature: &zero, ResponseFormat: llm.JSONObject,
	}
	started := time.Now()
	resp, err := m.Chat(cctx, req)
	rec := llmCall{Model: m.Model(), Latency: time.Since(started), Status: llmStatusOK}
	var reply *consultantReply
	reason := ""
	switch {
	case err != nil && (errors.Is(err, context.DeadlineExceeded) || errors.Is(cctx.Err(), context.DeadlineExceeded)):
		rec.Status, reason = llmStatusTimeout, guideFbTimeout
	case err != nil:
		rec.Status, reason = llmStatusError, guideFbError
	default:
		rec.InputTokens, rec.OutputTokens, rec.CachedTokens = resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.CachedTokens()
		r, perr := decodeConsultantReply(resp.Message.Content)
		if perr != nil {
			rec.Status, reason = llmStatusSchemaError, guideFbSchema
		} else {
			reply = r
		}
	}
	t.s.recordLLMUsage(ctx, rec)
	return reply, reason
}

func decodeConsultantReply(content string) (*consultantReply, error) {
	c := strings.TrimSpace(content)
	if i := strings.Index(c, "{"); i > 0 {
		c = c[i:]
	}
	if j := strings.LastIndex(c, "}"); j >= 0 && j < len(c)-1 {
		c = c[:j+1]
	}
	var r consultantReply
	if err := json.Unmarshal([]byte(c), &r); err != nil {
		return nil, err
	}
	var msgs []string
	for _, m := range r.Messages {
		if m = strings.TrimSpace(m); m != "" {
			msgs = append(msgs, m)
		}
	}
	if len(msgs) == 0 {
		return nil, errors.New("нет сообщений")
	}
	if len(msgs) > consultantMaxMessages {
		msgs = msgs[:consultantMaxMessages]
	}
	r.Messages = msgs
	return &r, nil
}

var (
	placeholderRe = regexp.MustCompile(`\{\{\s*([a-z_]+)\s*(?::\s*(\d+))?\s*\}\}`)
	braceRe       = regexp.MustCompile(`\{\{|\}\}`)
	moneyRe       = regexp.MustCompile(`(?i)(\d[\d\s\x{00A0}\x{202F}.,]*\s*(₽|руб|р\.|%|процент|коп))|₽|(?:^|[^\p{L}])руб(?:\.|л|[^\p{L}]|$)`)
	urlRe         = regexp.MustCompile(`(?i)(https?://|www\.|\]\(|[a-z0-9-]+\.(ru|com|рф|net|org|io)(/|\b))`)
)

func hasMoneyNumber(s string) bool { return moneyRe.MatchString(s) }

// checkRaw — проверки текста модели до подстановки (CORE.md §2).
func checkRaw(m string, sh *consultantSheet) error {
	rest := placeholderRe.ReplaceAllString(m, " ")
	if braceRe.MatchString(rest) {
		return errors.New("вставка в неверном формате")
	}
	if moneyRe.MatchString(rest) {
		return errors.New("в тексте есть цена, сумма или процент вне вставки")
	}
	if urlRe.MatchString(rest) {
		return errors.New("в тексте есть адрес сайта")
	}
	for _, mm := range placeholderRe.FindAllStringSubmatch(m, -1) {
		name, num := mm[1], mm[2]
		switch name {
		case "qty", "term":
			if num != "" {
				return fmt.Errorf("вставка {{%s}} без номера", name)
			}
		case "spec_total", "spec_count":
			if sh.SpecCount == 0 {
				return fmt.Errorf("вставка {{%s}} без спецификации", name)
			}
		case "name", "unit", "total", "old", "unit_prev":
			n, _ := strconv.Atoi(num)
			it := sh.item(n)
			if it == nil {
				return fmt.Errorf("вставка {{%s:%s}} ссылается на номер, которого нет в листе фактов", name, num)
			}
			if name == "old" && it.Old == nil || name == "unit_prev" && it.UnitPrev == nil {
				return fmt.Errorf("у позиции %d нет значения для {{%s}}", n, name)
			}
		default:
			return fmt.Errorf("неизвестная вставка {{%s}}", name)
		}
	}
	return nil
}

// substituteReply — проверки и подстановка всех сообщений.
func substituteReply(msgs []string, sh *consultantSheet) ([]string, error) {
	res := make([]string, 0, len(msgs))
	for _, m := range msgs {
		if err := checkRaw(m, sh); err != nil {
			return nil, err
		}
		res = append(res, substitute(m, sh))
	}
	return res, nil
}

// substitute — вставки → значения из листа фактов (цены полужирным, названия — ссылкой на магазин).
func substitute(m string, sh *consultantSheet) string {
	return placeholderRe.ReplaceAllStringFunc(m, func(s string) string {
		mm := placeholderRe.FindStringSubmatch(s)
		name := mm[1]
		n, _ := strconv.Atoi(mm[2])
		it := sh.item(n)
		switch name {
		case "name":
			if u := storeURL(it.Offer.OfferURL); u != "" {
				return "[" + it.Title + "](" + u + ")"
			}
			return it.Title
		case "unit":
			if it.Unit == nil {
				return "цена по запросу"
			}
			return "**" + rubRU(*it.Unit) + " ₽**"
		case "total":
			if it.Total == nil {
				if it.Unit == nil {
					return "цена по запросу"
				}
				return "**" + rubRU(*it.Unit) + " ₽**"
			}
			return "**" + rubRU(*it.Total) + " ₽**"
		case "old":
			return rubRU(*it.Old) + " ₽"
		case "unit_prev":
			return rubRU(*it.UnitPrev) + " ₽"
		case "qty":
			if sh.Qty <= 0 {
				return "нужное количество"
			}
			return fmt.Sprintf("%d %s", sh.Qty, objectNoun(sh.Kind, sh.Qty))
		case "term":
			switch {
			case sh.TermMonths == 0 && len(sh.Items) > 0 && strings.Contains(strings.ToLower(sh.Items[0].Subtitle), "бессроч"):
				return "бессрочно"
			case sh.TermMonths <= 0 || sh.TermMonths == 12:
				return "год"
			}
			return termWords(sh.TermMonths)
		case "spec_total":
			if sh.SpecTotal == nil {
				return "сумма уточняется"
			}
			return "**" + rubRU(*sh.SpecTotal) + " ₽**"
		case "spec_count":
			return fmt.Sprintf("%d %s", sh.SpecCount, guidePlural(sh.SpecCount, "позиция", "позиции", "позиций"))
		}
		return s
	})
}

func hasQuestion(msgs []string) bool {
	for _, m := range msgs {
		if strings.Contains(m, "?") {
			return true
		}
	}
	return false
}

// quickReplies — до трёх, до 24 знаков, только если в тексте есть вопрос; без цен и адресов.
func quickReplies(qs []string, msgs []string) []string {
	if !hasQuestion(msgs) {
		return nil
	}
	var res []string
	for _, q := range qs {
		q = strings.TrimSpace(q)
		if q == "" || len([]rune(q)) > consultantQuickRunes || moneyRe.MatchString(q) || urlRe.MatchString(q) ||
			placeholderRe.MatchString(q) || containsStr(res, q) {
			continue
		}
		res = append(res, q)
		if len(res) == consultantMaxQuick {
			break
		}
	}
	return res
}

// offerItem — позиция листа по офферу каталога (цены — как в подборе).
func (t *consultantTurn) offerItem(p *domain.Product, qty int, group string) consultantItem {
	st := domain.GuideState{}
	if t.conv.guide != nil {
		st = *t.conv.guide
	}
	gt := &guideTurn{g: t.g, cat: t.cat, kb: t.kb, st: &st}
	v := t.cat.view(p, t.kb)
	q := max(qty, 1)
	o := gt.offerOut(v, q, gt.screenKind(), out.OfferMatch, "")
	gt.decorateOffer(&o)
	it := consultantItem{Group: group, Offer: o, Qty: q, Total: o.Total, PriceState: o.PriceState}
	if o.Display != nil {
		it.Title, it.Subtitle = o.Display.Title, o.Display.Subtitle
	}
	if it.Title == "" {
		it.Title = suggestDisplay(p.FamilyName)
	}
	it.Unit = o.UnitPrice
	if it.Unit == nil {
		it.Unit = o.PricePerUnit
	}
	if p.OldPriceRub != nil && p.PriceRub != nil && *p.OldPriceRub > *p.PriceRub {
		old := *p.OldPriceRub
		it.Old = &old
	}
	return it
}

// searchItems — запасной список: поиск по каталогу по реплике покупателя.
func (t *consultantTurn) searchItems() []consultantItem {
	ctx, cancel := context.WithTimeout(context.Background(), 4*time.Second)
	defer cancel()
	return t.searchItemsCtx(ctx, t.req.Text, 0)
}

func (t *consultantTurn) searchItemsCtx(ctx context.Context, text string, qty int) []consultantItem {
	if t.cat == nil {
		return nil
	}
	fams := t.g.searchCatalog(ctx, t.cat, text, "")
	var items []consultantItem
	for _, fid := range fams {
		var best *domain.Product
		for _, p := range t.cat.byFamily[fid] {
			v := t.cat.view(p, t.kb)
			if !v.priced() || !consultantNewPurchase(p) || p.Kind == "hardware" {
				continue
			}
			if qty > 0 && !v.inTier(qty) {
				continue
			}
			if best == nil || *p.PriceRub < *best.PriceRub {
				best = p
			}
		}
		if best == nil {
			continue
		}
		it := t.offerItem(best, qty, consultantMore)
		it.N = len(items) + 1
		items = append(items, it)
		if len(items) >= 6 {
			break
		}
	}
	return items
}

var sentenceEnd = regexp.MustCompile(`[.!?…](\s+)`)

// splitLong — одно сообщение из четырёх и более предложений делится на два пузыря (объяснение и цены).
func splitLong(msgs []string) []string {
	if len(msgs) != 1 {
		return msgs
	}
	m := msgs[0]
	locs := sentenceEnd.FindAllStringSubmatchIndex(m, -1)
	if len(locs) < 3 {
		return msgs
	}
	// граница — после половины предложений (не больше четырёх в первом)
	k := min((len(locs)+1)/2, 4) - 1
	cut := locs[k][2]
	a, b := strings.TrimSpace(m[:cut]), strings.TrimSpace(m[cut:])
	if a == "" || b == "" {
		return msgs
	}
	return []string{a, b}
}

// consultantNewPurchase — позиция для новой покупки: продления, обновления, переходы и поддержка в добор не идут.
func consultantNewPurchase(p *domain.Product) bool {
	switch p.License.Kind {
	case domain.LicenseKindRenewal, domain.LicenseKindUpgrade, domain.LicenseKindMigration, domain.LicenseKindSupport, domain.LicenseKindCourse:
		return false
	}
	l := strings.ToLower(p.Title)
	return !strings.Contains(l, "продлен") && !strings.Contains(l, "обновлен") && !strings.Contains(l, "техподдерж")
}

var countOnlyRe = regexp.MustCompile(`\d+.{0,20}(компьютер|пк|рабочих мест|мест|пользовател|сотрудник)|(компьютер|пк|рабочих мест|пользовател|сотрудник).{0,20}\d+`)

// consultantClasses — классы продуктов для смены темы.
var consultantClasses = map[string][]string{
	"antivirus": {"антивирус", "kaspersky", "касперск", "dr.web", "eset", "endpoint", "pro32", "защит"},
	"office":    {"офис", "office", "р7", "workspace", "яндекс 360"},
	"crypto":    {"криптопро", "cryptopro", "крипто про", "csp", "эцп", "подпис"},
	"backup":    {"backup", "бэкап", "резервн"},
	"os":        {"astra", "астра", "linux", "линукс", "операционн", "windows"},
}

func classesOf(text string) map[string]bool {
	l := guideNorm(text)
	res := map[string]bool{}
	for c, ws := range consultantClasses {
		for _, w := range ws {
			if strings.Contains(l, w) {
				res[c] = true
				break
			}
		}
	}
	return res
}

var digitRe = regexp.MustCompile(`\d`)

// topicChanged — реплика называет другого вендора или другой класс продуктов, чем позиции прошлого листа, и не
// называет количество.
func (t *consultantTurn) topicChanged() bool {
	if t.conv.guide == nil || len(t.conv.sheet) == 0 || digitRe.MatchString(t.req.Text) {
		return false
	}
	var prev strings.Builder
	for _, it := range t.conv.sheet {
		if it.Group != consultantMore {
			prev.WriteString(it.Title + " " + it.Offer.Title + " ")
		}
	}
	prevText := guideNorm(prev.String())
	vendors := guideVendorsIn(t.req.Text)
	for _, v := range vendors {
		known := false
		for _, st := range guideVendorStems {
			if st.vendor == v && strings.Contains(prevText, st.stem) {
				known = true
			}
		}
		if !known {
			return true
		}
	}
	now, before := classesOf(t.req.Text), classesOf(prevText)
	if len(now) == 0 || len(before) == 0 {
		return false
	}
	for c := range now {
		if before[c] {
			return false
		}
	}
	return true
}

// pageProductHint — подбор с нуля на странице товара без названия продукта в реплике («найди серверный»):
// к реплике добавляется название семейства страницы.
func (t *consultantTurn) pageProductHint() string {
	pg := t.req.Page
	if pg == nil || (pg.Type != "product" && pg.Type != "family") || len(guideVendorsIn(t.req.Text)) > 0 {
		return ""
	}
	fid := ""
	if pg.OfferID != nil {
		if p := t.cat.products[*pg.OfferID]; p != nil {
			fid = p.FamilyID
		}
	}
	if fid == "" && pg.FamilyID != nil {
		fid = *pg.FamilyID
	}
	if name := suggestDisplay(t.cat.famName[fid]); name != "" {
		return " " + name
	}
	return ""
}
