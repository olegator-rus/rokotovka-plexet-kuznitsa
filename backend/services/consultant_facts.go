package services

import (
	"encoding/json"
	"math"
	"regexp"

	"kuznica/api/domain"
	"strings"

	"kuznica/api/out"
)

// Лист фактов консультанта: позиции с номерами (из хода подбора), факты с источниками, условия. Модели уходят
// только номера, названия и признаки — без цен; цены и адреса подставляет код по вставкам.

// Группы позиций.
const (
	consultantFit     = "fit"
	consultantMore    = "more"
	consultantUnfit   = "unfit"
	consultantMinList = 4
)

// consultantItem — позиция листа фактов.
type consultantItem struct {
	N          int
	Group      string
	Offer      out.GuideOffer
	Title      string
	Subtitle   string
	Unit       *float64
	Total      *float64
	Old        *float64
	UnitPrev   *float64
	Qty        int
	Reason     string
	Mark       string // пометка с проверяемым основанием («Есть нужное шифрование», «Для серверной ОС»)
	PriceState string
}

// consultantFact — факт для модели.
type consultantFact struct {
	Text   string `json:"text"`
	Source string `json:"source,omitempty"`
}

// consultantSheet — лист фактов хода.
type consultantSheet struct {
	Mode       string // selection | comparison | question | explain | page | none
	Items      []consultantItem
	Facts      []consultantFact
	Conditions string
	Assumption string
	Changed    []string
	TierItem   int  // позиция, у которой сменилась ступень цены (есть {{unit_prev}})
	Cheapest   bool // покупатель просит самый дешёвый вариант
	Question   string
	Chips      []string
	Comparison *out.GuideChatTable
	Common     []string // функции, которые есть у всех колонок сравнения (по базе знаний)
	ForceList  bool     // список даже при одной подходящей (комплект)
	NoAttach   bool     // ответ без вложений (уточнение по странице)
	ListTitle  string
	Source     *out.GuideScreenSource
	Kind       string // главный объект (workstation, server, …)
	Qty        int
	TermMonths int
	SpecTotal  *float64
	SpecCount  int
}

func (sh *consultantSheet) item(n int) *consultantItem {
	if n < 1 || n > len(sh.Items) {
		return nil
	}
	return &sh.Items[n-1]
}

func (sh *consultantSheet) count(group string) int {
	c := 0
	for _, it := range sh.Items {
		if it.Group == group {
			c++
		}
	}
	return c
}

// sheetFromGuide — лист фактов из ответа подбора.
func (t *consultantTurn) sheetFromGuide(res *out.GuideResponse) *consultantSheet {
	sh := &consultantSheet{Mode: "selection"}
	st := res.State
	sh.Kind, sh.Qty = mainObject(st.Requirements)
	if sh.Kind == "" {
		sh.Kind = "workstation"
	}
	sh.TermMonths = st.Requirements.TermMonths
	sh.Cheapest = st.Requirements.Cheapest
	var conds []string
	for _, c := range res.Conditions {
		conds = append(conds, c.Label)
	}
	sh.Conditions = strings.Join(conds, " · ")
	if !t.conv.assumed && fitTermYear(res) && st.Origin["operation"] != originUser && st.Origin["term_months"] != originUser &&
		st.Requirements.Operation != "renewal" && (st.Requirements.TermMonths == 12 || st.Requirements.TermMonths == 0) {
		sh.Assumption = "Срок и тип покупки покупатель не назвал: считаем новую лицензию на один год. Скажи об этом коротко в первой фразе."
	}
	offers := map[string]out.GuideOffer{}
	for _, o := range res.Offers {
		offers[o.OfferID] = o
	}
	var change *out.GuidePriceChange
	if res.Screen != nil && res.Screen.Changed != nil {
		sh.Changed = res.Screen.Changed.Items
		change = res.Screen.Changed.PriceChange
	}
	seen := map[string]bool{}
	if c := res.Chat; c != nil {
		for _, gr := range c.Groups {
			group := consultantMore
			switch gr.Key {
			case out.ChatGroupMatch:
				group = consultantFit
			case out.ChatGroupRejected:
				group = consultantUnfit
			}
			for _, card := range gr.Cards {
				if seen[card.OfferID] {
					continue
				}
				seen[card.OfferID] = true
				it := t.itemFromCard(card, offers[card.OfferID], group)
				if change != nil && change.OfferID == card.OfferID && change.From.UnitPrice > 0 {
					prev := change.From.UnitPrice
					it.UnitPrev = &prev
				}
				it.N = len(sh.Items) + 1
				if it.UnitPrev != nil {
					sh.TierItem = it.N
				}
				sh.Items = append(sh.Items, it)
			}
		}
		if c.Question != nil {
			sh.Question = c.Question.Text
			for _, ch := range c.Question.Chips {
				sh.Chips = append(sh.Chips, ch.Label)
			}
		}
		if c.Comparison != nil && len(c.Comparison.Columns) >= 2 {
			sh.Comparison = c.Comparison
			sh.Mode = "comparison"
			sh.Source = c.Comparison.Source
			sh.Common = t.commonFeatures(c.Comparison)
			for _, label := range sh.Common {
				sh.Facts = append(sh.Facts, consultantFact{Text: label + ": есть во всех сравниваемых редакциях", Source: sourceLabel(sh.Source)})
			}
			for _, r := range c.Comparison.Rows {
				if r.Kind != "feature" {
					continue
				}
				parts := []string{}
				for i, cell := range r.Cells {
					if i < len(c.Comparison.Columns) {
						parts = append(parts, c.Comparison.Columns[i].Title+" — "+strings.ToLower(cellWord(cell)))
					}
				}
				src := ""
				if sh.Source != nil {
					src = sh.Source.Label
				}
				sh.Facts = append(sh.Facts, consultantFact{Text: r.Label + ": " + strings.Join(parts, ", "), Source: src})
			}
		}
		for _, src := range c.Sources {
			if sh.Source == nil {
				s := src
				sh.Source = &s
			}
			if src.Fragment != "" && !strings.Contains(src.Fragment, "|") {
				sh.Facts = append(sh.Facts, consultantFact{Text: src.Fragment, Source: src.Label})
			}
		}
	}
	t.narrowUse(sh)
	t.enforceServer(sh, res)
	for _, cl := range res.Claims {
		if cl.Text != "" && !hasMoneyNumber(cl.Text) {
			sh.Facts = append(sh.Facts, consultantFact{Text: cl.Text, Source: orDefault(cl.Source.Label, cl.Source.Title)})
		}
	}
	if len(sh.Items) == 0 && sh.Question != "" {
		sh.Mode = "question"
	}
	if len(sh.Items) == 0 && sh.Question == "" {
		sh.Mode = "none"
	}
	return sh
}

func cellWord(c out.GuideScreenCell) string {
	switch c.Mark {
	case out.MarkYes:
		return "есть"
	case out.MarkNo:
		return "нет"
	case out.MarkUnknown:
		return "нет сведений"
	}
	return c.Text
}

// itemFromCard — позиция из карточки помощника и оффера подбора.
func (t *consultantTurn) itemFromCard(card out.GuideChatCard, o out.GuideOffer, group string) consultantItem {
	if o.OfferID == "" {
		o = out.GuideOffer{OfferID: card.OfferID, FamilyID: card.FamilyID, Title: card.Title, OfferURL: card.OfferURL,
			Quantity: card.Quantity, PriceState: card.PriceState}
	}
	it := consultantItem{Group: group, Offer: o, Title: card.Title, Subtitle: card.Subtitle, Qty: card.Quantity,
		Total: card.Total, PriceState: card.PriceState, Reason: card.Reason}
	if o.UnitPrice != nil {
		it.Unit = o.UnitPrice
	} else {
		it.Unit = o.PricePerUnit
	}
	if card.Badge != nil && group != consultantUnfit {
		it.Mark = card.Badge.Text
	}
	if group == consultantUnfit && it.Reason == "" && card.Badge != nil {
		it.Reason = card.Badge.Text
	}
	if card.Promo {
		if p := t.cat.products[card.OfferID]; p != nil && p.OldPriceRub != nil && p.PriceRub != nil && *p.OldPriceRub > *p.PriceRub {
			old := *p.OldPriceRub
			it.Old = &old
		}
	}
	return it
}

// render — лист фактов для модели (JSON): без цен, адресов и артикулов.
func (sh *consultantSheet) render() string {
	type item struct {
		N        int    `json:"n"`
		Group    string `json:"group"`
		Name     string `json:"name"`
		About    string `json:"about,omitempty"`
		Price    string `json:"price"`
		HasOld   bool   `json:"has_old_price,omitempty"`
		HasPrev  bool   `json:"has_unit_prev,omitempty"`
		Mark     string `json:"mark,omitempty"`
		Cheapest bool   `json:"cheapest_of_all,omitempty"`
		Reason   string `json:"reason,omitempty"`
	}
	v := struct {
		Mode       string           `json:"mode"`
		Conditions string           `json:"conditions,omitempty"`
		Assumption string           `json:"assumption,omitempty"`
		Changed    []string         `json:"changed,omitempty"`
		Items      []item           `json:"items"`
		Facts      []consultantFact `json:"facts"`
		Question   string           `json:"question_hint,omitempty"`
		Chips      []string         `json:"answer_options,omitempty"`
		Attachment string           `json:"attachment"`
		Spec       *struct {
			Count int `json:"count"`
		} `json:"spec,omitempty"`
	}{Mode: sh.Mode, Conditions: sh.Conditions, Assumption: sh.Assumption, Changed: sh.Changed, Items: []item{},
		Facts: sh.Facts, Question: sh.Question, Chips: sh.Chips, Attachment: sh.attachmentKind()}
	if v.Facts == nil {
		v.Facts = []consultantFact{}
	}
	if sh.SpecCount > 0 {
		v.Spec = &struct {
			Count int `json:"count"`
		}{sh.SpecCount}
	}
	cheapN := 0
	if sh.Cheapest {
		var best float64
		for _, it := range sh.Items {
			if it.Total != nil && (cheapN == 0 || *it.Total < best) && it.Group != consultantMore {
				cheapN, best = it.N, *it.Total
			}
		}
	}
	for _, it := range sh.Items {
		price := "known"
		if it.Total == nil && it.Unit == nil {
			price = "on_request"
		}
		v.Items = append(v.Items, item{N: it.N, Group: it.Group, Name: it.Title, About: it.Subtitle, Price: price,
			HasOld: it.Old != nil, HasPrev: it.UnitPrev != nil, Mark: it.Mark, Cheapest: it.N == cheapN, Reason: it.Reason})
	}
	raw, _ := json.Marshal(v)
	return string(raw)
}

// attachmentKind — какое вложение код приложит к ответу.
func (sh *consultantSheet) attachmentKind() string {
	switch {
	case sh.Comparison != nil:
		return "comparison"
	case len(sh.Items) == 0:
		return "none"
	case sh.ForceList:
		return "list"
	case sh.count(consultantFit) == 1:
		return "card"
	}
	return "list"
}

// attachments — вложения хода по листу фактов: сравнение, карточка или список; источник — если есть.
func (t *consultantTurn) attachments(sh *consultantSheet) []any {
	var as []any
	switch sh.attachmentKind() {
	case "comparison":
		if c := comparisonAttachment(sh); c != nil {
			as = append(as, *c)
		}
	case "card":
		for _, it := range sh.Items {
			if it.Group == consultantFit {
				as = append(as, out.ConsultantCard{Type: "product_card", Product: productRef(it)})
				break
			}
		}
	case "list":
		if l := listAttachment(sh.Items, sh.ListTitle); l != nil {
			as = append(as, *l)
		}
	}
	if sh.Source != nil && (sh.Comparison != nil || sh.count(consultantUnfit) > 0) {
		quote := sh.Source.Fragment
		if strings.Contains(quote, "|") {
			quote = "" // строка таблицы вендора — не цитата для покупателя: только заголовок и адрес
		}
		as = append(as, out.ConsultantSource{Type: "source", Title: orDefault(sh.Source.Label, "Документация вендора"),
			URL: sh.Source.URL, Quote: quote})
	}
	return as
}

func comparisonAttachment(sh *consultantSheet) *out.ConsultantComparison {
	c := sh.Comparison
	byID := map[string]consultantItem{}
	for _, it := range sh.Items {
		byID[it.Offer.OfferID] = it
	}
	res := &out.ConsultantComparison{Type: "comparison", Rows: []out.ConsultantCompareRow{}}
	for _, col := range c.Columns {
		it, ok := byID[col.OfferID]
		if !ok {
			return nil
		}
		ref := productRef(it)
		ref.Title = orDefault(col.Title, ref.Title)
		res.Columns = append(res.Columns, ref)
	}
	for _, label := range sh.Common {
		row := out.ConsultantCompareRow{Label: label}
		for range c.Columns {
			row.Values = append(row.Values, "✓")
		}
		res.Rows = append(res.Rows, row)
	}
	for _, r := range c.Rows {
		if r.Kind != "feature" {
			continue
		}
		row := out.ConsultantCompareRow{Label: r.Label}
		for _, cell := range r.Cells {
			switch cell.Mark {
			case out.MarkYes:
				row.Values = append(row.Values, "✓")
			case out.MarkNo:
				row.Values = append(row.Values, "—")
			case out.MarkUnknown:
				row.Values = append(row.Values, "Нет сведений")
			default:
				row.Values = append(row.Values, cell.Text)
			}
		}
		res.Rows = append(res.Rows, row)
	}
	return res
}

// listAttachment — список с группами: подходят, «Ещё посмотрите» (добор до четырёх), «Не подойдут» с причиной.
func listAttachment(items []consultantItem, title string) *out.ConsultantList {
	if len(items) == 0 {
		return nil
	}
	groups := map[string][]out.ProductReference{}
	for _, it := range items {
		g := it.Group
		if g == "" {
			g = consultantMore
		}
		if g == consultantUnfit && it.Reason == "" {
			it.Reason = "Не подходит под условия"
		}
		groups[g] = append(groups[g], productRef(it))
	}
	l := &out.ConsultantList{Type: "product_list", Title: title, Groups: []out.ConsultantListGroup{}}
	titles := map[string]string{consultantFit: "", consultantMore: "Ещё посмотрите", consultantUnfit: "Не подойдут"}
	// Порядок групп: подходят, не подойдут (с причиной — попадают в первые четыре), ещё посмотрите (CONSULTANT.md §6.2).
	for _, g := range []string{consultantFit, consultantUnfit, consultantMore} {
		if len(groups[g]) > 0 {
			l.Groups = append(l.Groups, out.ConsultantListGroup{Kind: g, Title: titles[g], Items: groups[g]})
		}
	}
	return l
}

// minor — рубли → целые копейки; nil или 0 — нет цены.
func minor(v *float64) *int64 {
	if v == nil || *v <= 0 {
		return nil
	}
	m := int64(math.Round(*v * 100))
	return &m
}

// productRef — позиция для фронта: цены в копейках, ссылка только store.softline.ru.
func productRef(it consultantItem) out.ProductReference {
	o := it.Offer
	ref := out.ProductReference{OfferID: o.OfferID, SKU: o.SKU, Title: orDefault(it.Title, o.Title),
		Vendor: vendorOfTitle(orDefault(it.Title, o.Title)), LicenseType: licenseTypeOf(it.Subtitle), Quantity: it.Qty,
		Duration: o.Term, UnitPriceMinor: minor(it.Unit), TotalPriceMinor: minor(it.Total), OldUnitPriceMinor: minor(it.Old),
		Reason: orDefault(it.Reason, it.Mark), ProductURL: storeURL(o.OfferURL), Offer: o}
	if ref.Duration == "" {
		ref.Duration = durationOf(it.Subtitle)
	}
	switch {
	case ref.UnitPriceMinor != nil || ref.TotalPriceMinor != nil:
		ref.PriceStatus = "known"
	case it.PriceState == out.PriceOnRequest:
		ref.PriceStatus = "on_request"
	default:
		ref.PriceStatus = "unknown"
	}
	switch it.Group {
	case consultantFit:
		ref.Compatibility = "compatible"
	case consultantUnfit:
		ref.Compatibility = "incompatible"
	default:
		ref.Compatibility = "unverified"
	}
	return ref
}

// storeURL — только адреса store.softline.ru.
func storeURL(u string) string {
	if strings.HasPrefix(u, "https://store.softline.ru/") {
		return u
	}
	return ""
}

// licenseTypeOf — первая часть подзаголовка карточки («Новая лицензия», «Серверная лицензия», «Продление»).
func licenseTypeOf(sub string) string {
	first, _, _ := strings.Cut(sub, " · ")
	if first != "" && !strings.ContainsAny(first, "0123456789") {
		return first // «Новая лицензия», «Серверная лицензия», «Рабочее место», «Для TLS-сервера»
	}
	return "Новая лицензия"
}

func durationOf(sub string) string {
	for _, p := range strings.Split(sub, " · ") {
		l := strings.ToLower(p)
		if strings.Contains(l, "год") || strings.Contains(l, "лет") || strings.Contains(l, "мес") || strings.Contains(l, "бессроч") {
			return p
		}
	}
	return ""
}

func vendorOfTitle(title string) string {
	l := guideNorm(title)
	for _, v := range guideVendorStems {
		if strings.Contains(l, v.stem) {
			return vendorLabel(v.vendor)
		}
	}
	first, _, _ := strings.Cut(title, " ")
	return first
}

func sourceLabel(s *out.GuideScreenSource) string {
	if s == nil {
		return ""
	}
	return s.Label
}

// commonFeatures — оси базы знаний со значением «есть» у всех семейств сравнения, которых нет в строках таблицы.
func (t *consultantTurn) commonFeatures(c *out.GuideChatTable) []string {
	if t.kb == nil {
		return nil
	}
	inTable := map[string]bool{}
	for _, r := range c.Rows {
		inTable[r.Label] = true
	}
	count := map[string]int{}
	label := map[string]string{}
	var order []string
	for _, col := range c.Columns {
		seen := map[string]bool{}
		for _, f := range t.kb.byFamily[col.FamilyID] {
			if !f.show || seen[f.Axis] || f.Variant != "" {
				continue
			}
			if k, _ := f.factValue(); k != "yes" {
				continue
			}
			seen[f.Axis] = true
			if count[f.Axis] == 0 {
				order = append(order, f.Axis)
				label[f.Axis] = orDefault(f.Label, axisLabel(f.Axis))
			}
			count[f.Axis]++
		}
	}
	var res []string
	for _, a := range order {
		if count[a] == len(c.Columns) && !inTable[label[a]] && len(res) < 2 {
			res = append(res, label[a])
		}
	}
	return res
}

// fitTermYear — у подходящих позиций срок «1 год»: только тогда допущение про год имеет смысл.
func fitTermYear(res *out.GuideResponse) bool {
	if res.Chat == nil {
		return false
	}
	for _, g := range res.Chat.Groups {
		if g.Key != out.ChatGroupMatch {
			continue
		}
		for _, c := range g.Cards {
			if strings.Contains(c.Subtitle, "1 год") {
				return true
			}
		}
	}
	return false
}

var signatureRe = regexp.MustCompile(`(?i)подпис|эцп|(^|[^\p{L}])эп([^\p{L}]|$)`)

// narrowUse — ответ «электронная подпись» на вопрос о назначении: лицензия TLS-сервера уходит из подходящих в
// «Ещё посмотрите» (требование «TLS» покупатель не называл).
func (t *consultantTurn) narrowUse(sh *consultantSheet) {
	l := guideNorm(t.req.Text)
	if !signatureRe.MatchString(l) || strings.Contains(l, "tls") || sh.count(consultantFit) < 2 {
		return
	}
	for i := range sh.Items {
		it := &sh.Items[i]
		if it.Group == consultantFit && strings.Contains(strings.ToLower(it.Title+" "+it.Subtitle), "tls") {
			it.Group = consultantMore
			it.Mark = ""
			it.Reason = "Для TLS-сервера"
		}
	}
}

var serverWantRe = regexp.MustCompile(`сервер`)
var serverNotRe = regexp.MustCompile(`не (для|на) сервер|без сервер`)

// enforceServer — покупатель просит серверный вариант, а среди подходящих — лицензии рабочей станции (подбор
// Astra ведёт по уровням защищённости и назначение не сверяет): такие позиции уходят в «Не подойдут» с причиной,
// подходящими становятся серверные позиции тех же семейств и уровней.
func (t *consultantTurn) enforceServer(sh *consultantSheet, res *out.GuideResponse) {
	l := guideNorm(t.req.Text)
	wantServer := hasObject(res.State.Requirements, "server") || (serverWantRe.MatchString(l) && !serverNotRe.MatchString(l))
	if !wantServer {
		return
	}
	type key struct{ fam, variant string }
	var moved []key
	for i := range sh.Items {
		it := &sh.Items[i]
		p := t.cat.products[it.Offer.OfferID]
		if p == nil || it.Group == consultantUnfit {
			continue
		}
		v := t.cat.view(p, t.kb)
		if v.scope != "workstation" {
			continue
		}
		it.Group, it.Mark, it.Reason = consultantUnfit, "", "Лицензия для рабочей станции — не для сервера"
		moved = append(moved, key{p.FamilyID, v.variant})
	}
	if len(moved) == 0 || sh.count(consultantFit) > 0 {
		return
	}
	org := orDefault(res.State.Requirements.OrgType, "commercial")
	var add []consultantItem
	seen := map[key]bool{}
	for _, k := range moved {
		if seen[k] {
			continue
		}
		seen[k] = true
		var best *domain.Product
		var bestKey []float64
		for _, p := range t.cat.byFamily[k.fam] {
			v := t.cat.view(p, t.kb)
			if v.scope != "server" || v.variant != k.variant || !v.priced() || !consultantNewPurchase(p) || (v.org != "" && v.org != org) {
				continue
			}
			term := 2.0
			switch v.term {
			case 12:
				term = 0
			case 0:
				term = 1
			}
			kk := []float64{term, *p.PriceRub}
			if best == nil || lessKey(kk, bestKey) {
				best, bestKey = p, kk
			}
		}
		if best != nil {
			it := t.offerItem(best, max(sh.Qty, 1), consultantFit)
			it.Mark = "Серверная лицензия"
			if k.variant != "" && !strings.Contains(it.Title, "«") {
				it.Title += " — «" + variantLabel(k.variant) + "»"
			}
			add = append(add, it)
		}
	}
	if len(add) == 0 {
		return
	}
	sh.Items = append(add, sh.Items...)
	sh.TierItem = 0
	for i := range sh.Items {
		sh.Items[i].N = i + 1
	}
}
