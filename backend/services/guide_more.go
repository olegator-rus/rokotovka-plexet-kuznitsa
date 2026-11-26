package services

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Контур подбора особые случаи подбора (SPEC §9) — общий запрос по ИБ и направления, сравнение между вендорами,
// цена по запросу (ориентир, «Можно купить сейчас», условия в запрос КП), вопрос о свойстве без факта, предел
// семейства по числу мест, пределы тарифа (AnyDesk: устройства, подключения, сотрудники).

var (
	reAsksFeature = regexp.MustCompile(`(поддерживает|поддержива[а-яё]*|есть\s+ли|умеет|работает\s+ли|работает\s+с|можно\s+ли|подходит\s+ли)`)
	reGeneralIS   = regexp.MustCompile(`(информационн[а-яё]*\s+безопасност|кибербезопасност|(^|[^а-яё])иб($|[^а-яё])|защит[а-яё]*\s+информац|безопасност[а-яё]*\s+(компании|офиса|бизнеса)|решени[а-яё]*\s+по\s+безопасност)`)
	reExactly     = regexp.MustCompile(`(именно|только|обязательно|нужен\s+[a-z])`)
)

// guideDirectionChipsSkip — направления, которые в чипы первого вопроса не выносятся (security_subclasses: note).
var guideDirectionChipsSkip = map[string]bool{"access": true}

// guideDirection — направление ИБ из базы знаний.
type guideDirection struct {
	ID       string
	Title    string
	Families []string
}

// directions — направления в порядке файла; нет в базе — только запасное «рабочие места».
func (kb *guideKB) directions() []guideDirection {
	if len(kb.dirOrder) == 0 {
		return []guideDirection{{ID: "endpoint", Title: "Рабочие места и серверы", Families: kb.subclass("endpoint")}}
	}
	out := []guideDirection{}
	for _, id := range kb.dirOrder {
		out = append(out, guideDirection{ID: id, Title: kb.dirTitle[id], Families: kb.subclasses[id]})
	}
	return out
}

// directionOf — направление задачи («endpoint_protection» — рабочие места).
func directionOf(task string) string {
	if task == "endpoint_protection" {
		return "endpoint"
	}
	return task
}

// guideVendorsIn — все вендоры, названные в тексте, в порядке упоминания.
func guideVendorsIn(q string) []string {
	q = guideNorm(q)
	type hit struct {
		at     int
		vendor string
	}
	var hits []hit
	seen := map[string]bool{}
	for _, v := range guideVendorStems {
		if i := strings.Index(q, v.stem); i >= 0 && !seen[v.vendor] {
			seen[v.vendor] = true
			hits = append(hits, hit{i, v.vendor})
		}
	}
	sort.Slice(hits, func(i, j int) bool { return hits[i].at < hits[j].at })
	out := []string{}
	for _, h := range hits {
		out = append(out, h.vendor)
	}
	return out
}

// askDirection — общий запрос по ИБ: «Что нужно защитить в первую очередь?» с направлениями (SPEC §9).
func (t *guideTurn) askDirection() *out.GuideResponse {
	chips := []out.GuideChip{}
	for _, d := range t.kb.directions() {
		if !guideDirectionChipsSkip[d.ID] {
			chips = append(chips, out.GuideChip{Label: strings.ToLower(d.Title[:1]) + d.Title[1:], Key: "direction", Value: d.ID})
		}
	}
	for i := range chips {
		chips[i].Label = guideLowerFirst(t.kb.dirTitle[chips[i].Value])
		if chips[i].Label == "" {
			chips[i].Label = "рабочие места и серверы"
		}
	}
	t.st.Pending = "direction"
	t.st.Questions++
	t.meta.Fallback = guideFbNoNeed
	t.step("Вопрос", "", "что защищаем", t.started)
	return &out.GuideResponse{Type: out.GuideClarification, Claims: []out.GuideClaim{}, Offers: []out.GuideOffer{},
		Clarification: &out.GuideClarify{Key: "direction", Question: "Что нужно защитить в первую очередь?", Chips: chips},
		Changes:       t.changes, Next: []string{"change_conditions", "request_quote"}}
}

// crossVendorFamilies — по семейству на вендора внутри направления: первое по списку направления, у которого есть
// позиции под условия (KES Стандартный, Dr.Web Desktop Security Suite).
func (t *guideTurn) crossVendorFamilies(vendors []string, dir string) []string {
	pool := t.kb.subclass(dir)
	if len(pool) == 0 {
		pool = t.kb.subclass("endpoint")
	}
	kind, _ := mainObject(t.st.Requirements)
	out := []string{}
	for _, v := range vendors {
		for _, f := range pool {
			if t.cat.famVend[f] != v {
				continue
			}
			// Серверная редакция для рабочих мест — не та колонка.
			if kind == "workstation" && scopeFromTitle(strings.ToLower(t.cat.famName[f])) == "server" {
				continue
			}
			// Предел семейства меньше числа мест (KSOS до 25) — не та колонка.
			if _, n := mainObject(t.st.Requirements); n > 0 {
				if lim := t.cat.familyLimit(f, t.kb); lim > 0 && n > lim {
					continue
				}
			}
			out = append(out, f)
			break
		}
	}
	return out
}

// familyLimit — предел семейства по числу мест из фида: у семейства со ступенями без открытой верхней границы —
// наибольшая ступень (или пакет); 0 — предела нет (есть ступень «от N», или только пакеты, которые складываются).
func (c *guideCatalog) familyLimit(fid string, kb *guideKB) int {
	c.limMu.Lock()
	defer c.limMu.Unlock()
	if c.limits == nil {
		c.limits = map[string]int{}
	}
	if v, ok := c.limits[fid]; ok {
		return v
	}
	limit, tiered := 0, false
	for _, p := range c.byFamily[fid] {
		if p.License.Kind != domain.LicenseKindBase {
			continue
		}
		v := c.view(p, kb)
		switch {
		case v.unit == "node_tier" && v.tierMin > 0:
			tiered = true
			if v.tierMax == 0 {
				c.limits[fid] = 0
				return 0
			}
			limit = max(limit, v.tierMax)
		case v.unit == "pack" && v.pack > 0:
			limit = max(limit, v.pack)
		}
	}
	if !tiered {
		limit = 0
	}
	c.limits[fid] = limit
	return limit
}

// --- вопрос о свойстве (D8) ---

// featureQuestion — «кибер бэкап стандартная поддерживает ленточные библиотеки?»: ответ только фактом базы знаний;
// факта нет — «В документации вендора этого не нашли» и событие missing_fact. Никогда «да» или «нет» без факта.
func (t *guideTurn) featureQuestion(text string) *out.GuideResponse {
	t1 := time.Now()
	r := t.st.Requirements
	fams := t.cat.nameMatch(text, r.Vendor, 6)
	fams = t.cat.focusFamilies(fams, text)
	t.step("Найдены семейства", toolSearchCatalog, fmt.Sprintf("семейств: %d", len(fams)), t1)
	if len(fams) == 0 {
		return nil
	}
	fid := fams[0]
	t.st.CandidateFamilyIDs = []string{fid}
	name := t.cat.famName[fid]
	stems := featureStems(text, name)
	var best *guideFact
	bestScore := 0
	for _, f := range t.kb.byFamily[fid] {
		hay := guideNorm(f.AxisLabel + " " + f.Axis + " " + f.Excerpt)
		score := 0
		for _, s := range stems {
			if strings.Contains(hay, s) {
				score++
			}
		}
		if score > bestScore {
			best, bestScore = f, score
		}
	}
	t.step("Факты базы знаний", toolGetFacts, fmt.Sprintf("совпало слов: %d", bestScore), t1)
	res := &out.GuideResponse{Claims: []out.GuideClaim{}, Offers: []out.GuideOffer{}, Changes: t.changes,
		Next: []string{"change_conditions", "request_quote"}}
	t.meta.Fallback = guideFbNoNeed
	if best != nil && best.verified() {
		kind, _ := best.factValue()
		if kind != "unknown" {
			c := t.claimOf(best, name)
			res.Type, res.Claims = out.GuideSelection, []out.GuideClaim{c}
			res.Lead = "По данным вендора: " + strings.TrimPrefix(c.Text, name+": ") + "."
			res.Docs = &out.GuideDocs{Title: c.Source.Title, URL: c.Source.URL}
			return res
		}
	}
	t.gap("missing_fact")
	t.gapItem("missing_fact", []string{fid}, "", text)
	res.Type = out.GuideNoData
	res.Lead = "В документации вендора этого не нашли."
	res.Notice = "Можно добавить вопрос в запрос КП"
	res.Docs = t.familyDocs(fid)
	t.st.QuoteNotes = appendUnique(t.st.QuoteNotes, guideQuoteNote(text))
	return res
}

// featureStems — значимые слова вопроса без названия продукта и служебных слов.
func featureStems(text, famName string) []string {
	name := guideNorm(famName)
	out := []string{}
	for _, w := range strings.Fields(regexp.MustCompile(`[^а-яёa-z0-9 ]`).ReplaceAllString(guideNorm(text), " ")) {
		if len([]rune(w)) < 4 || strings.Contains(name, w) || reAsksFeature.MatchString(w) || guideStopWord(w) {
			continue
		}
		switch w {
		case "есть", "ли", "стандартная", "расширенная", "редакция", "версия", "кибер", "бэкап":
			continue
		}
		r := []rune(w)
		out = append(out, string(r[:min(len(r), 4)])) // «ленточные» ~ «лентах»: короткая основа
	}
	return out
}

// familyDocs — ссылка на документацию вендора: источник любого факта семейства, иначе страница семейства.
func (t *guideTurn) familyDocs(fid string) *out.GuideDocs {
	for _, f := range t.kb.byFamily[fid] {
		if f.SourceURL != "" {
			s := factSource(f)
			return &out.GuideDocs{Title: s.Title, URL: s.URL}
		}
	}
	if u := t.cat.famURL[fid]; u != "" {
		return &out.GuideDocs{Title: t.cat.famName[fid], URL: u}
	}
	return nil
}

// guideQuoteNote — реплика покупателя для запроса КП: без ПДн, до 200 символов.
func guideQuoteNote(text string) string {
	t, _ := maskPII(strings.TrimSpace(text))
	if r := []rune(t); len(r) > 200 {
		t = string(r[:200])
	}
	return t
}

// gapItem — пробел для отчёта SearchOps «Вопросы, на которые подбор не ответил».
func (t *guideTurn) gapItem(code string, fids []string, axis, question string) {
	q, _ := maskPII(guideNorm(question))
	if r := []rune(q); len(r) > 160 {
		q = string(r[:160])
	}
	t.gapItems = append(t.gapItems, guideGapItem{Code: code, FamilyIDs: fids, Axis: axis, Question: q})
}

// guideGapItem — запись пробела в guide_turns.gap_items.
type guideGapItem struct {
	Code      string   `json:"code"`
	FamilyIDs []string `json:"family_ids"`
	Axis      string   `json:"axis,omitempty"`
	Question  string   `json:"question"`
}

// --- цена по запросу (D5) ---

// onRequestBlock — у первой карточки цена по запросу: ориентир из семейства (если есть позиция с ценой) и
// «Можно купить сейчас» — покупаемые семейства того же класса (find_alternatives), с оговоркой о совместимости.
// «Нужен именно …» — альтернативы не предлагаются, условия покупателя уходят в запрос КП.
func (t *guideTurn) onRequestBlock(res *out.GuideResponse, data *guideData, text string) {
	if len(res.Offers) == 0 || res.Offers[0].Role != out.OfferMatch || res.Offers[0].PriceState != out.PriceOnRequest {
		return
	}
	first := res.Offers[0]
	res.Offers = res.Offers[:1] // искомый продукт первым и один; остальное «по запросу» — в выдаче под блоком
	res.Offers[0].Note = "Цена по запросу — добавьте в запрос КП"
	if text != "" && reExactly.MatchString(guideNorm(text)) {
		t.st.QuoteNotes = appendUnique(t.st.QuoteNotes, guideQuoteNote(text))
		res.Offers = res.Offers[:1]
		res.Lead = "Оставляю «" + first.FamilyTitle + "» в запросе КП и добавляю ваши условия."
		res.Next = []string{"request_quote", "change_conditions"}
		return
	}
	t1 := time.Now()
	if !t.read() {
		return
	}
	alts := t.cat.findAlternatives(first.FamilyID, 40)
	r := t.st.Requirements
	serverClass := scopeFromTitle(strings.ToLower(first.FamilyTitle)) == "server"
	added := 0
	for _, fid := range alts {
		if added >= 3 {
			break
		}
		views, _ := t.cat.familyOffers(fid, r, t.st.Origin["term_months"], t.kb)
		var ok []guideOfferView
		for _, v := range views {
			if serverClass && v.scope != "server" {
				continue // серверный продукт — альтернативы тоже серверные позиции
			}
			if v.priced() && len(validateLicense(v, r, t.kb, termAny)) == 0 {
				ok = append(ok, v)
			}
		}
		if len(ok) == 0 {
			continue
		}
		sortOffers(ok, data.qty, t.cat.famName[fid])
		if t.altFams == nil {
			t.altFams = map[string]bool{}
		}
		t.altFams[fid] = true
		res.Offers = append(res.Offers, t.offerOut(ok[0], data.qty, data.kind, out.OfferAlternative,
			"Можно купить сейчас: тот же класс решения; совместимость с вашими приложениями нужно проверить"))
		added++
	}
	t.step("Можно купить сейчас", toolFindAlternatives, fmt.Sprintf("альтернатив: %d", added), t1)
	res.Lead = "«" + first.FamilyTitle + "» — цена по запросу: добавьте в запрос КП."
	if added > 0 {
		res.Lead += " Ниже — что можно купить сейчас."
	}
	res.Next = []string{"request_quote", "change_conditions"}
}

// --- пределы тарифа (D7: AnyDesk) ---

// limitAxes — оси фактов-пределов варианта → вид объекта условия.
var limitAxes = map[string]string{"managed_devices": "workstation", "concurrent_connections": "connection", "licensed_users": "user"}

// limitFamily — семейство кандидатов с вариантами, у которых в базе числовые пределы ("" — нет).
func (t *guideTurn) limitFamily(fams []string) string {
	for _, f := range fams {
		for _, fact := range t.kb.byFamily[f] {
			if _, ok := limitAxes[fact.Axis]; ok && fact.Variant != "" {
				return f
			}
		}
	}
	return ""
}

// factLimit — число предела из факта ({"value": N} или {"max": N}); 0 — нет.
func factLimit(f *guideFact) int {
	var m map[string]any
	if json.Unmarshal(f.Value, &m) != nil {
		return 0
	}
	for _, k := range []string{"value", "max"} {
		if v, ok := m[k].(float64); ok {
			return int(v)
		}
	}
	return 0
}

// limitsTurn — тариф по трём пределам: сначала вопрос об одновременных подключениях, затем младший тариф, который
// проходит все названные пределы. Сотрудники, устройства и подключения не смешиваются.
func (t *guideTurn) limitsTurn(fid string) *out.GuideResponse {
	t1 := time.Now()
	r := t.st.Requirements
	res := &out.GuideResponse{Claims: []out.GuideClaim{}, Offers: []out.GuideOffer{}, Changes: t.changes}
	t.meta.Fallback = guideFbNoNeed
	if objectCount(r, "connection") == 0 && t.st.Questions < 2 && t.st.Skipped < 2 {
		chips := []out.GuideChip{}
		for _, n := range []string{"1", "2", "5"} {
			chips = append(chips, out.GuideChip{Label: n, Key: "count:connection", Value: n})
		}
		chips = append(chips, out.GuideChip{Label: "другое", Key: "count:connection", Value: ""})
		t.st.Pending, t.st.Questions = "count:connection", t.st.Questions+1
		res.Type = out.GuideClarification
		res.Clarification = &out.GuideClarify{Key: "count:connection", Question: "Сколько подключений должно работать одновременно?", Chips: chips}
		res.Next = []string{"change_conditions", "request_quote"}
		t.step("Вопрос", "", "одновременные подключения", t1)
		return res
	}
	// Варианты по фактам: предел каждой оси.
	limits := map[string]map[string]*guideFact{} // вариант → ось → факт
	for _, f := range t.kb.byFamily[fid] {
		if _, ok := limitAxes[f.Axis]; ok && f.Variant != "" {
			if limits[f.Variant] == nil {
				limits[f.Variant] = map[string]*guideFact{}
			}
			limits[f.Variant][f.Axis] = f
		}
	}
	// Базовая лицензия варианта — позиция, название которой кончается на «, <вариант>».
	base := map[string]guideOfferView{}
	for _, p := range t.cat.byFamily[fid] {
		v := t.cat.view(p, t.kb)
		for vr := range limits {
			if strings.HasSuffix(strings.TrimSpace(p.Title), ", "+vr) && v.priced() {
				// Основная лицензия тарифа — самое короткое название (без Custom Namespace, дополнений).
				if cur, ok := base[vr]; !ok || len(p.Title) < len(cur.p.Title) || len(p.Title) == len(cur.p.Title) && p.ID < cur.p.ID {
					base[vr] = v
				}
			}
		}
	}
	variants := []string{}
	for vr := range base {
		variants = append(variants, vr)
	}
	sort.Slice(variants, func(i, j int) bool { return *base[variants[i]].p.PriceRub < *base[variants[j]].p.PriceRub })
	tbl := &out.GuideComparisonTbl{}
	for _, vr := range variants {
		tbl.Columns = append(tbl.Columns, out.GuideColumn{FamilyID: fid, Title: vr, FamilyURL: t.cat.famURL[fid]})
	}
	chosen := ""
	for _, axis := range []string{"managed_devices", "concurrent_connections", "licensed_users"} {
		row := out.GuideRow{Key: axis, Label: axisLabel(axis), Kind: "feature"}
		for _, vr := range variants {
			f := limits[vr][axis]
			if f == nil {
				row.Cells = append(row.Cells, out.GuideCell{Kind: out.CellUnknown})
				continue
			}
			if f.AxisLabel != "" {
				row.Label = f.AxisLabel
			}
			c := cellOf(f)
			if n := factLimit(f); n > 0 && f.verified() {
				c = out.GuideCell{Kind: out.CellValue, Value: n, FactID: f.ID} // число — единица в подписи строки
			}
			row.Cells = append(row.Cells, c)
		}
		tbl.Rows = append(tbl.Rows, row)
	}
	for _, vr := range variants {
		fits := true
		for axis, kind := range limitAxes {
			need := objectCount(r, kind)
			if need == 0 {
				continue
			}
			f := limits[vr][axis]
			if f == nil || !f.verified() || factLimit(f) == 0 {
				t.prelim = true // предел не подтверждён — не отсекаем, но помечаем
				continue
			}
			if factLimit(f) < need {
				fits = false
			}
		}
		if fits {
			chosen = vr
			break
		}
	}
	t.step("Пределы тарифов", toolGetFacts, fmt.Sprintf("тарифов: %d", len(variants)), t1)
	res.Comparison = tbl
	res.Type = out.GuideComparison
	if chosen == "" {
		t.gap("no_matching_offer")
		res.Notice = "Ни один тариф не проходит все пределы — нужны дополнения, уточним в запросе КП"
		res.Lead = "Сравнил пределы тарифов по данным вендора."
	} else {
		res.Type = out.GuideSelection
		res.Offers = append(res.Offers, t.offerOut(base[chosen], 1, "", out.OfferMatch, "Тариф "+chosen))
		res.Lead = "Подходит тариф " + chosen + ": проверены управляемые устройства, одновременные подключения и сотрудники."
		for _, f := range limits[chosen] {
			if f.verified() {
				res.Claims = append(res.Claims, t.claimOf(f, chosen))
			}
		}
		sort.Slice(res.Claims, func(i, j int) bool { return res.Claims[i].FactID < res.Claims[j].FactID })
	}
	res.Next = t.nextSteps(res)
	return res
}
