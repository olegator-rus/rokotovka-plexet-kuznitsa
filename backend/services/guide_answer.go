package services

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Ответ подбора (SPEC §6–§8): таблица сравнения и карточки строит код по идентификаторам; модель даёт только
// связующую фразу и выбор фактов. Проверки до показа (§7) — checkAnswer.

// offerOut — карточка оффера: всё из каталога.
func (t *guideTurn) offerOut(v guideOfferView, qty int, kind, role, note string) out.GuideOffer {
	p := v.p
	o := out.GuideOffer{OfferID: p.ID, FamilyID: p.FamilyID, Title: p.Title, FamilyTitle: p.FamilyName, SKU: p.SKU,
		Tier: v.tierLabel(), Unit: v.unitLabel(kind), Quantity: qty, PriceState: out.PriceOnRequest, OfferURL: p.OfferURL,
		Term: v.termLabel(), Role: role, Note: note}
	if v.priced() {
		price := *p.PriceRub
		o.PricePerUnit, o.PriceState = &price, out.PricePriced
		if v.inTier(qty) {
			sum := guideRound2(price * float64(v.units(qty)))
			o.Total = &sum
		}
	}
	return o
}

// claimOf — утверждение из факта: текст строит код.
func (t *guideTurn) claimOf(f *guideFact, short string) out.GuideClaim {
	kind, val := f.factValue()
	label := axisLabel(f.Axis)
	if f.AxisLabel != "" {
		label = f.AxisLabel
	}
	text := ""
	switch kind {
	case "yes":
		text = fmt.Sprintf("%s: %s — есть", short, label)
	case "no":
		text = fmt.Sprintf("%s: %s — нет по данным вендора", short, label)
	case "depends":
		text = fmt.Sprintf("%s: %s — зависит от ваших условий", short, label)
	case "value":
		text = fmt.Sprintf("%s: %s — %s", short, label, val)
	default:
		text = fmt.Sprintf("%s: %s — нет сведений", short, label)
	}
	return out.GuideClaim{FactID: f.ID, Text: text, Status: f.Status, Source: factSource(f)}
}

// factSource — источник факта без дат.
func factSource(f *guideFact) out.GuideSource {
	title := f.SourceTitle
	if title == "" {
		title = f.SourceSection
	}
	if title == "" {
		if u, err := url.Parse(f.SourceURL); err == nil {
			title = u.Host
		}
	}
	return out.GuideSource{Title: title, URL: f.SourceURL, Fragment: f.Excerpt}
}

// cellOf — ячейка таблицы из факта; нет проверенного факта — «нет сведений».
func cellOf(f *guideFact) out.GuideCell {
	if !f.verified() {
		return out.GuideCell{Kind: out.CellUnknown}
	}
	switch kind, val := f.factValue(); kind {
	case "yes":
		return out.GuideCell{Kind: out.CellYes, FactID: f.ID}
	case "no":
		return out.GuideCell{Kind: out.CellNo, FactID: f.ID}
	case "depends":
		return out.GuideCell{Kind: out.CellValue, Value: "зависит от ваших условий", FactID: f.ID}
	case "value":
		return out.GuideCell{Kind: out.CellValue, Value: val, FactID: f.ID}
	}
	return out.GuideCell{Kind: out.CellUnknown}
}

// columnOrder — порядок колонок: ранг линейки базы знаний, иначе дешевле — левее.
func (t *guideTurn) columnOrder(data *guideData) {
	rank := func(fd *guideFamData) int {
		if l, ok := t.kb.lines[fd.fid]; ok && l.Rank > 0 {
			return l.Rank
		}
		return 1000
	}
	price := func(fd *guideFamData) float64 {
		if len(fd.matched) == 0 {
			return 1e18
		}
		return offerTotal(fd.matched[0], data.qty)
	}
	sort.SliceStable(data.fams, func(i, j int) bool {
		ri, rj := rank(data.fams[i]), rank(data.fams[j])
		if ri != rj {
			return ri < rj
		}
		return price(data.fams[i]) < price(data.fams[j])
	})
}

// comparison — таблица сравнения редакций (SPEC §8).
func (t *guideTurn) comparison(data *guideData) *out.GuideResponse {
	t.columnOrder(data)
	r := t.st.Requirements
	tbl := &out.GuideComparisonTbl{}
	for _, fd := range data.fams {
		tbl.Columns = append(tbl.Columns, out.GuideColumn{FamilyID: fd.fid, Title: fd.short, FamilyURL: t.cat.famURL[fd.fid]})
	}
	// Оси — из проверенных фактов колонок; сначала те, где значения различаются.
	axes := []string{}
	for _, fd := range data.fams {
		for _, f := range t.kb.byFamily[fd.fid] {
			if f.verified() && f.applies(r.Platform) && !containsStr(axes, f.Axis) {
				axes = append(axes, f.Axis)
			}
		}
	}
	type row struct {
		r    out.GuideRow
		diff bool
	}
	var rows []row
	var claims []out.GuideClaim
	for _, axis := range axes {
		gr := out.GuideRow{Key: axis, Label: axisLabel(axis), Kind: "feature"}
		kinds := map[string]bool{}
		var yes []*guideFact
		for _, fd := range data.fams {
			f := t.kb.factOn(fd.fid, axis, r.Variant)
			c := out.GuideCell{Kind: out.CellUnknown}
			if f != nil {
				c = cellOf(f)
				if f.AxisLabel != "" {
					gr.Label = f.AxisLabel
				}
			}
			if c.Kind == out.CellUnknown {
				t.gap("missing_fact")
			}
			if c.Kind == out.CellYes {
				yes = append(yes, f)
			}
			kinds[c.Kind+fmt.Sprint(c.Value)] = true
			gr.Cells = append(gr.Cells, c)
		}
		diff := len(kinds) > 1
		if diff {
			for _, f := range yes {
				claims = append(claims, t.claimOf(f, t.shortOf(data, f.FamilyID)))
			}
		}
		rows = append(rows, row{gr, diff})
	}
	sort.SliceStable(rows, func(i, j int) bool { return rows[i].diff && !rows[j].diff })
	for i, rw := range rows {
		if i >= 8 {
			break
		}
		tbl.Rows = append(tbl.Rows, rw.r)
	}
	if len(axes) == 0 {
		t.gap("missing_fact")
	}
	tbl.Rows = append(tbl.Rows, t.commercialRows(data)...)
	res := &out.GuideResponse{Type: out.GuideComparison, Comparison: tbl, Claims: claims, Changes: t.changes}
	for _, fd := range data.fams {
		if len(fd.matched) > 0 {
			res.Offers = append(res.Offers, t.offerOut(fd.matched[0], data.qty, data.kind, out.OfferMatch, ""))
		}
	}
	if len(axes) == 0 {
		res.Notice = "В базе знаний нет проверенных сведений о функциях этих редакций — сравнение по каталогу"
	}
	res.Lead = t.compareLead(data, claims)
	res.Next = t.nextSteps(res)
	return res
}

// shortOf — подпись семейства на ходе.
func (t *guideTurn) shortOf(data *guideData, fid string) string {
	for _, fd := range data.fams {
		if fd.fid == fid {
			return fd.short
		}
	}
	return t.cat.famName[fid]
}

// compareLead — шаблонная фраза сравнения: только положительные утверждения из проверенных фактов.
func (t *guideTurn) compareLead(data *guideData, claims []out.GuideClaim) string {
	by := map[string][]string{}
	order := []string{}
	for _, c := range claims {
		f := t.kb.facts[c.FactID]
		if f == nil {
			continue
		}
		s := t.shortOf(data, f.FamilyID)
		if _, ok := by[s]; !ok {
			order = append(order, s)
		}
		label := axisLabel(f.Axis)
		if f.AxisLabel != "" {
			label = f.AxisLabel // формулировка вендора из факта, не наш пересказ
		}
		by[s] = append(by[s], guideLowerFirst(label))
	}
	if len(order) == 1 {
		feats := by[order[0]]
		if len(feats) > 3 {
			feats = feats[:3]
		}
		return fmt.Sprintf("В редакции «%s» есть %s.", order[0], joinRU(feats))
	}
	names := []string{}
	for _, fd := range data.fams {
		names = append(names, "«"+fd.short+"»")
	}
	return "Сравнение " + joinRU(names) + " по данным вендора и каталога."
}

func joinRU(xs []string) string {
	switch len(xs) {
	case 0:
		return ""
	case 1:
		return xs[0]
	}
	return strings.Join(xs[:len(xs)-1], ", ") + " и " + xs[len(xs)-1]
}

// commercialRows — цена за место в год и итог: только при одинаковой единице, сроке, ступени и типе лицензии.
func (t *guideTurn) commercialRows(data *guideData) []out.GuideRow {
	var best []*guideOfferView
	for _, fd := range data.fams {
		if len(fd.matched) == 0 {
			best = append(best, nil)
			continue
		}
		v := fd.matched[0]
		best = append(best, &v)
	}
	comparable, reason := true, ""
	var unit, tier string
	term := -1
	for _, v := range best {
		switch {
		case v == nil || !v.priced():
			comparable, reason = false, "цена по запросу или позиции под условия нет"
		case v.term == 0:
			comparable, reason = false, "бессрочная лицензия — в цену за год не пересчитывается"
		case unit != "" && (v.unit != unit || v.tierLabel() != tier || v.term != term):
			comparable, reason = false, "продаются в разных единицах — сравнить цену за место нельзя"
		}
		if v != nil {
			unit, tier, term = v.unit, v.tierLabel(), v.term
		}
	}
	noun := "место"
	if data.kind == "workstation" {
		noun = "компьютер"
	}
	label := "Цена за " + noun + " в год"
	if tier != "" {
		label += ", " + tier + " мест"
	}
	perYear := out.GuideRow{Key: "price_per_seat_year", Label: label, Kind: "commercial"}
	total := out.GuideRow{Key: "total", Label: "Итого на " + countLabel(orDefault(data.kind, "workstation"), data.qty), Kind: "commercial"}
	for _, v := range best {
		if !comparable || v == nil {
			perYear.Cells = append(perYear.Cells, out.GuideCell{Kind: out.CellUnknown})
		} else {
			perYear.Cells = append(perYear.Cells, out.GuideCell{Kind: out.CellValue, Value: guideRound2(v.perSeat() * 12 / float64(v.term))})
		}
		if v != nil && v.priced() && v.inTier(data.qty) {
			total.Cells = append(total.Cells, out.GuideCell{Kind: out.CellValue, Value: guideRound2(*v.p.PriceRub * float64(v.units(data.qty)))})
		} else {
			total.Cells = append(total.Cells, out.GuideCell{Kind: out.CellValue, Value: "по запросу"})
		}
	}
	if !comparable {
		perYear.Note = reason
	}
	return []out.GuideRow{perYear, total}
}

func orDefault(s, d string) string {
	if s == "" {
		return d
	}
	return s
}

// selection — подобранная лицензия: подходящие офферы, «При изменении условий», «Не подходит …».
func (t *guideTurn) selection(data *guideData) *out.GuideResponse {
	r := t.st.Requirements
	res := &out.GuideResponse{Type: out.GuideSelection, Changes: t.changes, Claims: []out.GuideClaim{}}
	var kept []*guideFamData
	for _, fd := range data.fams {
		if fd.excluded != "" {
			res.Changes = append(res.Changes, out.GuideChange{Condition: "нужно " + strings.ToLower(axisLabel(r.RequiredFeatures[0])),
				Effect: fmt.Sprintf("«%s» убран — %s", fd.short, fd.excluded)})
			continue
		}
		kept = append(kept, fd)
	}
	var matches []out.GuideOffer
	var matchFams []*guideFamData
	for _, fd := range kept {
		if len(fd.matched) > 0 {
			matches = append(matches, t.offerOut(fd.matched[0], data.qty, data.kind, out.OfferMatch, ""))
			matchFams = append(matchFams, fd)
		}
	}
	sortByTotal(matches)
	if r.Cheapest && len(matches) > 1 {
		matches = matches[:1]
	}
	if len(matches) > guideShowOffers {
		matches = matches[:guideShowOffers]
	}
	res.Offers = append(res.Offers, matches...)
	// «При изменении условий»: исключённые по функции — с названным отличием.
	for _, fd := range data.fams {
		if fd.excluded != "" && len(fd.matched) > 0 && len(res.Offers) < guideShowOffers+2 {
			note := "Без функции «" + strings.ToLower(axisLabel(fd.exclFact.Axis)) + "»"
			if fd.exclFact.Axis == "encryption" {
				note = "Без шифрования"
			}
			res.Offers = append(res.Offers, t.offerOut(fd.matched[0], data.qty, data.kind, out.OfferAlternative, fd.short+": "+note))
		}
	}
	// «Не подходит …»: у семейства первой карточки — до двух отклонённых по праву использования, разные сроки.
	var top *guideFamData
	if len(res.Offers) > 0 {
		for _, fd := range data.fams {
			if fd.fid == res.Offers[0].FamilyID {
				top = fd
			}
		}
	} else if len(data.fams) > 0 {
		top = data.fams[0]
	}
	if top != nil {
		terms := map[int]bool{}
		for _, rj := range top.rejected {
			if len(terms) >= 2 || terms[rj.v.term] {
				continue
			}
			terms[rj.v.term] = true
			note, rid, src := rj.why.Message, "", ""
			switch rj.why.Status {
			case kbVerified:
				rid, src = rj.why.RuleID, rj.why.Source
			case kbDraft:
				note = "Не подходит под ваши условия лицензирования" // текст правила не сверен человеком
			}
			res.Offers = append(res.Offers, t.offerOut(rj.v, data.qty, data.kind, out.OfferRejected, note))
			res.Offers[len(res.Offers)-1].RuleID, res.Offers[len(res.Offers)-1].RuleSource = rid, src
		}
	}
	if len(matches) == 0 {
		res.Type = out.GuideNoData
		res.Notice = "Под эти условия подходящих позиций нет — измените условия или запросите КП"
	}
	// Утверждения: требуемые функции у подходящих — из проверенных фактов.
	for _, fd := range matchFams {
		for _, axis := range r.RequiredFeatures {
			if f := t.kb.factOn(fd.fid, axis, r.Variant); f.verified() {
				if k, _ := f.factValue(); k == "yes" {
					res.Claims = append(res.Claims, t.claimOf(f, fd.short))
				}
			}
		}
	}
	res.Lead = t.selectionLead(data, matches)
	res.Next = t.nextSteps(res)
	return res
}

func sortByTotal(os []out.GuideOffer) {
	val := func(o out.GuideOffer) float64 {
		if o.Total != nil {
			return *o.Total
		}
		if o.PricePerUnit != nil {
			return *o.PricePerUnit * float64(max(o.Quantity, 1))
		}
		return 1e18
	}
	sort.SliceStable(os, func(i, j int) bool { return val(os[i]) < val(os[j]) })
}

// selectionLead — шаблонная фраза подбора.
func (t *guideTurn) selectionLead(data *guideData, matches []out.GuideOffer) string {
	// Самое конкретное правило первым: у правила про TLS-сервер условий больше, чем у правила про сервер.
	sort.SliceStable(data.ruleHits, func(i, j int) bool {
		return len(t.kb.ruleByID(data.ruleHits[i].RuleID).When) > len(t.kb.ruleByID(data.ruleHits[j].RuleID).When)
	})
	for _, h := range data.ruleHits {
		if h.Status == kbVerified && h.Message != "" {
			return strings.TrimRight(h.Message, ".") + "."
		}
	}
	if len(matches) == 1 {
		return "Подходит «" + t.shortOf(data, matches[0].FamilyID) + "»."
	}
	if len(matches) > 1 {
		return "Подобрал варианты под ваши условия."
	}
	return ""
}

// nextSteps — следующий шаг: в спецификацию, если есть подходящая позиция с ценой; запрос КП — всегда.
func (t *guideTurn) nextSteps(res *out.GuideResponse) []string {
	next := []string{}
	for _, o := range res.Offers {
		if o.Role == out.OfferMatch && o.PriceState == out.PricePriced {
			next = append(next, "add_to_spec")
			break
		}
	}
	return append(next, "change_conditions", "request_quote")
}

// --- модель ---

// modelLead — один вызов модели: фраза и выбор фактов; условия, которых правила не увидели, — в состояние и
// повторный сбор данных (третье чтение), фраза тогда шаблонная.
func (t *guideTurn) modelLead(ctx context.Context, text string, res *out.GuideResponse, data *guideData) {
	if reason := t.g.modelBlocked(ctx); reason != "" {
		t.meta.Fallback = reason
		return
	}
	if t.calls >= guideMaxModelCalls {
		return
	}
	in := guideModelInput{Query: text}
	for _, c := range conditionsOf(*t.st) {
		in.Conditions = append(in.Conditions, c.Label)
	}
	// В модель — только факты различающихся осей и требуемых функций (короче вход — быстрее ответ).
	axes := map[string]bool{}
	for _, c := range res.Claims {
		if f := t.kb.facts[c.FactID]; f != nil {
			axes[f.Axis] = true
		}
	}
	for _, a := range t.st.Requirements.RequiredFeatures {
		axes[a] = true
	}
	for _, fd := range data.fams {
		in.Families = append(in.Families, map[string]string{"family_id": fd.fid, "title": fd.short})
		for _, f := range t.kb.byFamily[fd.fid] {
			if f.verified() && f.applies(t.st.Requirements.Platform) && axes[f.Axis] && len(in.Facts) < 12 {
				k, v := f.factValue()
				if k == "value" {
					k = v
				}
				in.Facts = append(in.Facts, map[string]string{"fact_id": f.ID, "family_id": f.FamilyID, "axis": f.Axis, "value": k})
			}
		}
	}
	for _, o := range res.Offers {
		if o.Role == out.OfferMatch {
			in.Offers = append(in.Offers, map[string]string{"offer_id": o.OfferID, "family_id": o.FamilyID})
		}
	}
	for _, c := range res.Changes {
		in.Changes = append(in.Changes, c.Effect)
	}
	if len(in.Facts) == 0 {
		// Без проверенных фактов фраза модели всё равно не пройдёт проверку — вызов не нужен.
		t.meta.Fallback = guideFbNoNeed
		return
	}
	t3 := time.Now()
	reply, call, reason := t.g.callModel(ctx, in.render())
	t.calls++
	t.meta.LLMCalls++
	t.meta.InputTokens += call.in
	t.meta.OutputTokens += call.out
	t.meta.CostRub = guideRound4(t.meta.CostRub + call.cost)
	t.step("Связующая фраза", "llm", "", t3)
	if reply == nil {
		t.meta.Fallback = reason
		return
	}
	// Условия, которые модель увидела, а правила — нет.
	if d := deltaFromSet(reply.Set); !d.empty() {
		before := fmt.Sprint(t.st.Requirements)
		ch := applyDelta(t.st, d, originUser)
		if fmt.Sprint(t.st.Requirements) != before && t.read() {
			t.changes = append(t.changes, ch...)
			nd := t.gather(t.st.CandidateFamilyIDs)
			*data = *nd
			var fresh *out.GuideResponse
			if res.Type == out.GuideComparison {
				fresh = t.comparison(data)
			} else {
				fresh = t.selection(data)
			}
			*res = *fresh
			t.removed = append(t.removed, "lead: условия изменены моделью — фраза шаблонная")
			return
		}
	}
	shown := map[string]bool{}
	for _, fd := range data.fams {
		shown[fd.fid] = true
	}
	var claims []out.GuideClaim
	for _, id := range reply.Claims {
		f := t.kb.facts[id]
		if f == nil || !f.verified() || !shown[f.FamilyID] || !f.applies(t.st.Requirements.Platform) {
			t.removed = append(t.removed, "claim:"+id)
			continue
		}
		claims = append(claims, t.claimOf(f, t.shortOf(data, f.FamilyID)))
	}
	for _, id := range append(append([]string{}, reply.Fam...), reply.Off...) {
		if !shown[id] && t.cat.products[id] == nil {
			t.removed = append(t.removed, "id:"+id)
		}
	}
	lead := strings.TrimSpace(reply.Lead)
	if why := checkLead(lead, t.allowedNumbers(res), true); why != "" {
		t.removed = append(t.removed, "lead: "+why)
		return
	}
	if len(claims) == 0 {
		t.removed = append(t.removed, "lead: без фактов")
		return
	}
	res.Lead = lead
	if len(claims) > 0 {
		res.Claims = claims
	}
	t.meta.Fallback = ""
}

// deltaFromSet — условия из поля set ответа модели (только известные ключи и значения).
func deltaFromSet(set map[string]any) guideDelta {
	var d guideDelta
	num := func(v any) int {
		switch x := v.(type) {
		case float64:
			return int(x)
		case string:
			n, _ := strconv.Atoi(x)
			return n
		}
		return 0
	}
	kind := "workstation"
	if k, ok := set["object"].(string); ok && containsStr([]string{"workstation", "server", "user"}, k) {
		kind = k
	}
	if n := num(set["count"]); n > 0 && n <= maxQuoteQuantity {
		d.objects = []domain.GuideObject{{Kind: kind, Count: n}}
	}
	if op, ok := set["operation"].(string); ok && (op == "new" || op == "renewal") {
		d.operation = op
	}
	if n := num(set["term_months"]); n > 0 && n <= 120 {
		d.term = n
	}
	if fs, ok := set["features"].([]any); ok {
		for _, f := range fs {
			if s, ok := f.(string); ok && len(s) <= 40 {
				if _, known := axisLabels[s]; known {
					d.features = append(d.features, s)
				}
			}
		}
	}
	return d
}

// --- проверки до показа (SPEC §7) ---

var (
	reLeadForbidden = regexp.MustCompile(`(лучш|самы[йеяо]|гарантир|полностью соответству|надежнее|надёжнее)`)
	reLeadNegation  = regexp.MustCompile(`(^|[^а-яё])(нет|без|не\s+(поддерж|включ|умеет|име|содерж))([^а-яё]|$)`)
	reLeadNumber    = regexp.MustCompile(`\d+`)
	reLeadSKU       = regexp.MustCompile(`[A-Za-z]{2,}\d{3,}[A-Za-z0-9]*`)
)

// checkLead — "" — фраза годится; иначе причина. model — фраза модели (ещё и без отрицаний функций).
func checkLead(lead string, allowed map[string]bool, model bool) string {
	low := strings.ToLower(lead)
	switch {
	case lead == "":
		return "пусто"
	case len(strings.Fields(lead)) > guideLeadMaxWords:
		return "длиннее 60 слов"
	case reLeadForbidden.MatchString(low):
		return "запретное слово"
	case strings.Contains(lead, "₽") || strings.Contains(low, "руб"):
		return "цена во фразе"
	case reLeadSKU.MatchString(lead):
		return "артикул во фразе"
	case model && reLeadNegation.MatchString(low):
		return "отрицание функции"
	}
	for _, n := range reLeadNumber.FindAllString(lead, -1) {
		if !allowed[n] {
			return "число не из данных: " + n
		}
	}
	return ""
}

// allowedNumbers — числа, которые фраза может назвать: из условий, ступеней, названий.
func (t *guideTurn) allowedNumbers(res *out.GuideResponse) map[string]bool {
	m := map[string]bool{}
	add := func(s string) {
		for _, n := range reLeadNumber.FindAllString(s, -1) {
			m[n] = true
		}
	}
	for _, c := range conditionsOf(*t.st) {
		add(c.Label)
	}
	for _, o := range res.Offers {
		add(o.Tier)
		add(o.FamilyTitle)
	}
	if res.Comparison != nil {
		for _, c := range res.Comparison.Columns {
			add(c.Title)
		}
	}
	return m
}

// checkAnswer — проверки до показа: идентификаторы, факты, фраза, правила, «самый дешёвый», ступень.
func (t *guideTurn) checkAnswer(res *out.GuideResponse, data *guideData) {
	shown := map[string]bool{}
	for _, fd := range data.fams {
		shown[fd.fid] = true
	}
	// Утверждения.
	kept := res.Claims[:0:0]
	for _, c := range res.Claims {
		f := t.kb.facts[c.FactID]
		if f == nil || !f.verified() || !shown[f.FamilyID] || !f.applies(t.st.Requirements.Platform) {
			t.removed = append(t.removed, "claim:"+c.FactID)
			continue
		}
		kept = append(kept, c)
	}
	res.Claims = kept
	// Ячейки таблицы с фактом — только проверенные.
	if res.Comparison != nil {
		for i := range res.Comparison.Rows {
			for j, c := range res.Comparison.Rows[i].Cells {
				if c.FactID != "" && !t.kb.facts[c.FactID].verified() {
					res.Comparison.Rows[i].Cells[j] = out.GuideCell{Kind: out.CellUnknown}
					t.removed = append(t.removed, "cell:"+c.FactID)
				}
			}
		}
	}
	// Офферы: существуют, из найденных на ходе семейств, подходящие проходят validate_license.
	r := t.st.Requirements
	ok := res.Offers[:0:0]
	for _, o := range res.Offers {
		p := t.cat.products[o.OfferID]
		if p == nil || !shown[p.FamilyID] {
			t.removed = append(t.removed, "offer:"+o.OfferID)
			continue
		}
		if o.Role == out.OfferMatch {
			if bad := validateLicense(t.cat.view(p, t.kb), r, t.kb, termAny); len(bad) > 0 {
				t.removed = append(t.removed, "offer:"+o.OfferID+":"+bad[0].RuleID)
				continue
			}
			if o.Total == nil && o.PriceState == out.PricePriced {
				t.st.Unresolved = appendUnique(t.st.Unresolved, "Количество вне ступени: "+o.OfferID)
			}
		}
		ok = append(ok, o)
	}
	res.Offers = ok
	// «Самый дешёвый» — первая подходящая карточка дешевле всех подходящих позиций хода.
	if r.Cheapest {
		var first *out.GuideOffer
		for i := range res.Offers {
			if res.Offers[i].Role == out.OfferMatch {
				first = &res.Offers[i]
				break
			}
		}
		if first != nil && first.PricePerUnit != nil {
			for _, fd := range data.fams {
				if fd.excluded != "" {
					continue
				}
				for _, v := range fd.matched {
					if v.priced() && offerTotal(v, data.qty) < *first.PricePerUnit*float64(max(first.Quantity, 1))-0.005 {
						t.removed = append(t.removed, "cheapest:"+first.OfferID)
						*first = t.offerOut(v, data.qty, data.kind, out.OfferMatch, "")
					}
				}
			}
		}
	}
	// Фраза.
	if res.Lead != "" {
		if why := checkLead(res.Lead, t.allowedNumbers(res), false); why != "" {
			t.removed = append(t.removed, "lead: "+why)
			res.Lead = ""
		}
	}
	if res.Lead == "" && res.Type == out.GuideComparison {
		res.Lead = "Сравнение редакций по данным вендора и каталога."
	}
	if res.Claims == nil {
		res.Claims = []out.GuideClaim{}
	}
	if res.Offers == nil {
		res.Offers = []out.GuideOffer{}
	}
	res.Next = t.nextSteps(res)
}

func appendUnique(xs []string, x string) []string {
	if containsStr(xs, x) {
		return xs
	}
	return append(xs, x)
}

func guideRound4(v float64) float64 { return float64(int64(v*1e4+0.5)) / 1e4 }

// guideLowerFirst — первая буква строчная («Патч-менеджмент» → «патч-менеджмент»); аббревиатуры не трогаются.
func guideLowerFirst(s string) string {
	r := []rune(s)
	if len(r) > 1 && strings.ToUpper(string(r[1])) == string(r[1]) {
		return s
	}
	if len(r) > 0 {
		r[0] = []rune(strings.ToLower(string(r[0])))[0]
	}
	return string(r)
}
