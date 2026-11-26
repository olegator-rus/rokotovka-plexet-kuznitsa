package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"sort"
	"strconv"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Ход подбора (SPEC §5): условия из чипа и реплики → поиск семейств → офферы и факты → проверка правил → одна
// связующая фраза модели → проверки до показа → ответ. Пределы: 3 зависимых чтения, 2 вызова модели, 12 с на ход.
// Сбой или предел — ответ с условиями и пометкой, обычная выдача остаётся на экране.

const (
	guideTurnLimit     = 12 * time.Second
	guideMaxReads      = 3
	guideMaxModelCalls = 2
	guideMaxText       = 500
	guideShowOffers    = 4
)

// Тексты.
const (
	MsgGuideNoCatalog = "Каталог не загружен — подбор недоступен"
	guideNoticeFail   = "Подобрать не удалось — выдача и условия ниже, их можно изменить"
)

// guideTurn — ход подбора.
type guideTurn struct {
	g       *guideService
	cat     *guideCatalog
	kb      *guideKB
	st      *domain.GuideState
	started time.Time
	reads   int
	calls   int
	trace   []out.GuideTraceStep
	meta    out.GuideMeta
	gaps    []string
	prelim  bool
	removed []string
	changes []out.GuideChange
	mixed   bool // в реплике была посторонняя часть
}

func (t *guideTurn) ms(since time.Time) float64 {
	return math.Round(float64(time.Since(since).Microseconds())/10) / 100
}

func (t *guideTurn) step(step, tool, result string, since time.Time) {
	t.trace = append(t.trace, out.GuideTraceStep{Step: step, Tool: tool, Ms: t.ms(since), Result: result})
}

// read — ещё одно зависимое чтение; false — предел хода исчерпан.
func (t *guideTurn) read() bool {
	if t.reads >= guideMaxReads {
		return false
	}
	t.reads++
	return true
}

func (t *guideTurn) gap(code string) {
	if !containsStr(t.gaps, code) {
		t.gaps = append(t.gaps, code)
	}
}

// Guide — POST /api/guide: один ход подбора.
func (s *Services) Guide(ctx context.Context, requestID string, req in.GuideRequest) (*out.GuideResponse, error) {
	req.Query, req.Message = strings.TrimSpace(req.Query), strings.TrimSpace(req.Message)
	var bad []errs.FieldError
	if len([]rune(req.Query)) > guideMaxText {
		bad = append(bad, errs.FieldError{Field: "query", Code: errs.CodeOutOfRange, Message: "Запрос длиннее 500 символов"})
	}
	if len([]rune(req.Message)) > guideMaxText {
		bad = append(bad, errs.FieldError{Field: "message", Code: errs.CodeOutOfRange, Message: "Реплика длиннее 500 символов"})
	}
	if req.Chip != nil && (len(req.Chip.Key) > 64 || len(req.Chip.Value) > 128) {
		bad = append(bad, errs.FieldError{Field: "chip", Code: errs.CodeOutOfRange, Message: "Слишком длинный чип"})
	}
	if req.Condition != nil && req.Chip == nil {
		// Правка чипа условия — тот же путь, что чип: null снимает условие.
		v := ""
		if req.Condition.Value != nil {
			v = *req.Condition.Value
		}
		req.Chip = &in.GuideChip{Key: req.Condition.Key, Value: v}
	}
	if req.Query == "" && req.Message == "" && req.Chip == nil {
		bad = append(bad, errs.FieldError{Field: "query", Code: errs.CodeRequired, Message: "Нужен запрос, реплика или чип"})
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	g := s.guide()
	cat := g.catalog()
	if cat == nil {
		return nil, errs.NewUnavailableError(MsgGuideNoCatalog)
	}
	kb := g.knowledge()
	ctx = WithRequestID(ctx, requestID)
	ctx, cancel := context.WithTimeout(ctx, guideTurnLimit)
	defer cancel()

	sess := g.session(req.State, cat, kb)
	mu := g.sessions.turnLock(sess.state.SelectionID)
	mu.Lock()
	defer mu.Unlock()
	t := &guideTurn{g: g, cat: cat, kb: kb, st: &sess.state, started: time.Now()}
	t.meta.TurnID = requestID
	res := t.safeRun(ctx, req)
	res.Trace, res.Gaps = t.trace, nonNilStrings(t.gaps)
	res.Meta = t.meta
	res.Meta.MsTotal = t.ms(t.started)
	res.Meta.Removed = len(t.removed)
	res.Conditions = conditionsOf(*t.st)
	res.State = *t.st
	if res.Changes == nil {
		res.Changes = []out.GuideChange{}
	}
	g.sessions.put(sess)
	g.saveTurn(ctx, req.SessionID, t, res)
	return res, nil
}

// session — подбор сервера по selection_id; потерян — присланное состояние после сверки с каталогом; нет — новый.
func (g *guideService) session(st *domain.GuideState, cat *guideCatalog, kb *guideKB) *guideSession {
	if st != nil && st.SelectionID != "" && len(st.SelectionID) <= maxIDLen {
		if s, ok := g.sessions.get(st.SelectionID); ok {
			return s
		}
		clean := sanitizeGuideState(*st, cat)
		clean.FeedVersion, clean.KBVersion = cat.version, kb.version
		return &guideSession{state: clean}
	}
	return &guideSession{state: newGuideState(cat.version, kb.version)}
}

// sanitizeGuideState — присланное состояние: только известные значения и существующие идентификаторы.
func sanitizeGuideState(in domain.GuideState, cat *guideCatalog) domain.GuideState {
	st := newGuideState(cat.version, "")
	st.SelectionID = in.SelectionID
	r := in.Requirements
	if r.Operation == "new" || r.Operation == "renewal" {
		st.Requirements.Operation = r.Operation
	}
	if r.TermMonths > 0 && r.TermMonths <= 120 {
		st.Requirements.TermMonths = r.TermMonths
	}
	if containsStr([]string{"commercial", "government", "academic", "nonprofit"}, r.OrgType) {
		st.Requirements.OrgType = r.OrgType
	}
	for _, o := range r.Objects {
		if containsStr([]string{"workstation", "server", "user", "connection"}, o.Kind) && o.Count >= 0 && o.Count <= maxQuoteQuantity && len(st.Requirements.Objects) < 4 {
			st.Requirements.Objects = append(st.Requirements.Objects, o)
		}
	}
	for _, f := range r.RequiredFeatures {
		if len(f) <= 40 && len(st.Requirements.RequiredFeatures) < 8 {
			st.Requirements.RequiredFeatures = append(st.Requirements.RequiredFeatures, f)
		}
	}
	if len(r.Vendor) <= 64 {
		st.Requirements.Vendor = r.Vendor
	}
	if r.Use == "tls_server" {
		st.Requirements.Use = r.Use
	}
	if containsStr([]string{"Орел", "Орёл", "Воронеж", "Смоленск"}, r.Variant) {
		st.Requirements.Variant = r.Variant
	}
	if len(r.Platform) <= 32 {
		st.Requirements.Platform = r.Platform
	}
	st.Requirements.Task, st.Requirements.Cheapest = r.Task, r.Cheapest
	for k, v := range in.Origin {
		if len(st.Origin) < 32 && len(k) <= 48 && (v == originUser || v == originDefault || v == originInferred) {
			st.Origin[k] = v
		}
	}
	for _, f := range in.CandidateFamilyIDs {
		if len(cat.byFamily[f]) > 0 && len(st.CandidateFamilyIDs) < 8 {
			st.CandidateFamilyIDs = append(st.CandidateFamilyIDs, f)
		}
	}
	st.Turn, st.Questions, st.Skipped = min(max(in.Turn, 0), 100), min(max(in.Questions, 0), 10), min(max(in.Skipped, 0), 10)
	if strings.HasPrefix(in.Pending, "count:") && len(in.Pending) < 32 {
		st.Pending = in.Pending
	}
	return st
}

// safeRun — ход с отказом: паника или предел времени — ответ с условиями и пометкой.
func (t *guideTurn) safeRun(ctx context.Context, req in.GuideRequest) (res *out.GuideResponse) {
	defer func() {
		if p := recover(); p != nil {
			zap.L().Error("подбор: ход упал — отказ к выдаче", zap.Any("panic", p))
			res = t.fallback(guideFbError)
		}
	}()
	res = t.run(ctx, req)
	if res == nil || ctx.Err() != nil && res.Type != out.GuideFallback {
		if ctx.Err() != nil {
			t.gap("turn_timeout")
			return t.fallback(guideFbTurn)
		}
	}
	return res
}

// fallback — отказ: блок сообщает, что помочь не удалось; условия и выдача остаются.
func (t *guideTurn) fallback(reason string) *out.GuideResponse {
	t.meta.Fallback = reason
	return &out.GuideResponse{Type: out.GuideFallback, Notice: guideNoticeFail, Claims: []out.GuideClaim{},
		Offers: []out.GuideOffer{}, Changes: t.changes, Next: []string{"change_conditions", "request_quote"}}
}

// run — ход.
func (t *guideTurn) run(ctx context.Context, req in.GuideRequest) *out.GuideResponse {
	st := t.st
	st.Turn++
	first := st.Turn == 1
	text := req.Message
	t0 := time.Now()
	if req.Chip != nil {
		if req.Chip.Key == "start_over" {
			*st = newGuideState(t.cat.version, t.kb.version)
			st.Turn, first = 1, true
		} else if req.Chip.Key == "variant" && req.Chip.Value == "unknown" {
			st.Unresolved = appendUnique(st.Unresolved, "variant")
			st.Skipped++
			st.Pending = ""
		} else if d, ok := chipDelta(*st, &guideChipIn{Key: req.Chip.Key, Value: req.Chip.Value}); ok {
			if st.Pending != "" && strings.TrimSpace(req.Chip.Value) == "" {
				st.Skipped++
			}
			t.changes = append(t.changes, applyDelta(st, d, originUser)...)
			st.Pending = ""
		}
	}
	src := text
	if first && req.Query != "" {
		qd := extractConditions(req.Query)
		fixObjectKinds(&qd, *st)
		applyDelta(st, qd, originUser)
		if src == "" {
			src = req.Query
		}
	}
	if text != "" {
		if r := t.serviceReply(text); r != nil {
			return r
		}
		d := extractConditions(text)
		fixObjectKinds(&d, *st)
		if offTopic(text) {
			if d.empty() && !asksCompare(text) {
				return t.outOfScope(text)
			}
			t.mixed = true
		}
		t.changes = append(t.changes, applyDelta(st, d, originUser)...)
		st.Pending = ""
	}
	t.step("Условия", "", fmt.Sprintf("условий: %d", len(conditionsOf(*st))), t0)

	compare := src != "" && asksCompare(src)
	r := &st.Requirements
	if len(r.Objects) == 0 && !compare && !r.Cheapest && st.Questions < 2 && st.Skipped < 2 {
		return t.askCount()
	}

	// Чтение 1: семейства.
	t1 := time.Now()
	t.read()
	var fams []string
	if compare {
		fams = t.cat.editionFamilies(t.kb, src, r.Vendor, st.CandidateFamilyIDs)
		if len(fams) >= 2 {
			st.Intent = "comparison"
		} else {
			fams = nil
		}
	}
	if len(fams) == 0 && len(st.CandidateFamilyIDs) > 0 {
		fams = st.CandidateFamilyIDs
	}
	if len(fams) == 0 {
		fams = t.g.searchCatalog(ctx, t.cat, strings.TrimSpace(req.Query+" "+text), r.Vendor)
		fams = t.cat.focusFamilies(fams, strings.TrimSpace(req.Query+" "+text))
		if r.Task == "endpoint_protection" && r.Vendor == "" {
			fams = t.directionFamilies(fams, "endpoint")
		}
		if len(fams) > 8 {
			fams = fams[:8]
		}
	}
	if r.Use == "tls_server" {
		fams = t.cat.withTLSFamilies(fams, r.Vendor)
	}
	if r.Vendor == "" && len(fams) > 0 && compare {
		r.Vendor = t.cat.famVend[fams[0]]
		st.Origin["vendor"] = originInferred
	}
	st.CandidateFamilyIDs = append([]string{}, fams...)
	t.step("Найдены семейства", toolSearchCatalog, fmt.Sprintf("семейств: %d", len(fams)), t1)
	if len(fams) == 0 {
		t.gap("no_matching_offer")
		return t.noData("Под эти условия в каталоге ничего не нашли — измените условия или запросите КП")
	}
	if vf := t.variantFamily(fams); vf != "" && !compare {
		t.read()
		st.CandidateFamilyIDs = []string{vf}
		res := t.variantTurn(vf)
		t.meta.MsFirst = t.ms(t.started)
		res.Preliminary = t.prelim
		if t.meta.Fallback == "" {
			t.meta.Fallback = guideFbNoNeed
		}
		return res
	}
	if ctx.Err() != nil {
		return nil
	}

	// Чтение 2: офферы, факты, правила.
	if !t.read() {
		return t.fallback(guideFbTurn)
	}
	data := t.gather(fams)
	t.meta.MsFirst = t.ms(t.started)
	var res *out.GuideResponse
	if compare && len(fams) >= 2 {
		res = t.comparison(data)
	} else {
		res = t.selection(data)
	}
	if ctx.Err() != nil {
		return nil
	}
	// Модель: связующая фраза и выбор фактов — только на реплику покупателя.
	// Сравнение — фраза кода из формулировок вендора в фактах (модель пересказывает «патч-менеджмент» своими словами).
	if text != "" && res.Type == out.GuideSelection {
		t.modelLead(ctx, text, res, data)
	} else if t.meta.Fallback == "" {
		t.meta.Fallback = guideFbNoNeed
	}
	t4 := time.Now()
	t.checkAnswer(res, data)
	t.step("Проверки до показа", "", fmt.Sprintf("удалено утверждений: %d", len(t.removed)), t4)
	t.meta.MsChecked = t.ms(t.started)
	if t.mixed {
		res.Changes = append(res.Changes, out.GuideChange{Condition: "посторонняя часть", Effect: "в каталоге ПО и оборудования это не подбираю"})
	}
	st.SelectedOfferIDs, st.FactIDs = []string{}, []string{}
	for _, o := range res.Offers {
		if o.Role == out.OfferMatch {
			st.SelectedOfferIDs = append(st.SelectedOfferIDs, o.OfferID)
		}
	}
	for _, c := range res.Claims {
		st.FactIDs = append(st.FactIDs, c.FactID)
	}
	res.Preliminary = t.prelim
	return res
}

// fixObjectKinds — число без вида («будет 50») относится к главному объекту подбора.
func fixObjectKinds(d *guideDelta, st domain.GuideState) {
	for i := range d.objects {
		if d.objects[i].Kind == "" {
			k, _ := mainObject(st.Requirements)
			if k == "" {
				k = "workstation"
			}
			d.objects[i].Kind = k
		}
	}
}

// askCount — вопрос «сколько» с чипами (стадия 2 порядка выяснения, SPEC §3).
func (t *guideTurn) askCount() *out.GuideResponse {
	st := t.st
	q := "Сколько рабочих мест нужно?"
	if st.Requirements.Task == "endpoint_protection" {
		q = "Сколько компьютеров нужно защитить?"
	}
	chips := []out.GuideChip{}
	for _, n := range []string{"10", "30", "50", "100"} {
		chips = append(chips, out.GuideChip{Label: n, Key: "count:workstation", Value: n})
	}
	chips = append(chips, out.GuideChip{Label: "другое", Key: "count:workstation", Value: ""})
	st.Pending = "count:workstation"
	st.Questions++
	t.meta.Fallback = guideFbNoNeed
	t.step("Вопрос", "", "сколько мест", t.started)
	return &out.GuideResponse{Type: out.GuideClarification, Claims: []out.GuideClaim{}, Offers: []out.GuideOffer{},
		Clarification: &out.GuideClarify{Key: "count:workstation", Question: q, Chips: chips},
		Changes:       t.changes, Next: []string{"change_conditions", "request_quote"}}
}

// serviceReply — попытка сломать или служебная просьба (SPEC §10, D10): ответ кодом, без инструментов.
func (t *guideTurn) serviceReply(text string) *out.GuideResponse {
	q := guideNorm(text)
	lead := ""
	switch {
	case strings.Contains(q, "инструкц") || strings.Contains(q, "промпт") || strings.Contains(q, "system prompt"):
		lead = "Этого показать не могу. Чем помочь с подбором?"
	case strings.Contains(q, "скидк") || strings.Contains(q, "отправь") || strings.Contains(q, "забудь"):
		lead = "Скидки я не назначаю и ничего не отправляю без вас. Могу собрать спецификацию по ценам каталога."
	default:
		return nil
	}
	t.meta.Fallback = guideFbNoNeed
	t.step("Вне подбора", "", "отказ без инструментов", t.started)
	return &out.GuideResponse{Type: out.GuideOutOfScope, Lead: lead, Claims: []out.GuideClaim{}, Offers: []out.GuideOffer{},
		Changes: t.changes, Next: []string{"change_conditions", "request_quote"}}
}

// outOfScope — постороннее или сервисное (оплата, доставка): одна фраза.
func (t *guideTurn) outOfScope(text string) *out.GuideResponse {
	lead := "Это в каталоге ПО и оборудования не подбираю. Чем помочь с выбором лицензий?"
	q := guideNorm(text)
	if strings.Contains(q, "оплат") || strings.Contains(q, "доставк") || strings.Contains(q, "счет") || strings.Contains(q, "возврат") {
		lead = "Оплату, доставку и счета подбор не обсуждает — ответ в справке магазина над выдачей."
	}
	t.meta.Fallback = guideFbNoNeed
	t.step("Вне подбора", "", "без инструментов", t.started)
	return &out.GuideResponse{Type: out.GuideOutOfScope, Lead: lead, Claims: []out.GuideClaim{}, Offers: []out.GuideOffer{},
		Changes: t.changes, Next: []string{"change_conditions", "request_quote"}}
}

// noData — нет данных под условия.
func (t *guideTurn) noData(notice string) *out.GuideResponse {
	if t.meta.Fallback == "" {
		t.meta.Fallback = guideFbNoNeed
	}
	return &out.GuideResponse{Type: out.GuideNoData, Lead: "", Notice: notice, Claims: []out.GuideClaim{}, Offers: []out.GuideOffer{},
		Changes: t.changes, Next: []string{"change_conditions", "request_quote"}}
}

// focusFamilies — запрос называет продукт («криптопро csp»): оставить семейства с наибольшим числом слов запроса в
// названии; иначе порядок поиска как есть.
func (c *guideCatalog) focusFamilies(fams []string, text string) []string {
	words := []string{}
	for _, w := range strings.Fields(guideNorm(text)) {
		if len([]rune(w)) >= 3 && !guideStopWord(w) {
			words = append(words, stemRU(w))
		}
	}
	best, scores := 0, make([]int, len(fams))
	for i, f := range fams {
		nameWords := strings.Fields(guideNorm(c.famName[f]))
		for _, w := range words {
			for _, nw := range nameWords {
				if strings.HasPrefix(nw, w) {
					scores[i]++
					break
				}
			}
		}
		best = max(best, scores[i])
	}
	if best < 2 {
		return fams
	}
	out := []string{}
	for i, f := range fams {
		if scores[i] == best {
			out = append(out, f)
		}
	}
	return out
}

func guideStopWord(w string) bool {
	switch w {
	case "для", "самый", "самая", "самое", "дешевый", "дешевая", "дешевле", "подбери", "нужен", "нужна", "нужно",
		"сервера", "сервер", "компьютеров", "компьютера", "офиса", "мест", "лицензия", "лицензию", "лицензии", "как", "что", "чем":
		return true
	}
	return false
}

// withTLSFamilies — для TLS-сервера к кандидатам добавляются семейства вендора с TLS в названии.
func (c *guideCatalog) withTLSFamilies(fams []string, vendor string) []string {
	if vendor == "" && len(fams) > 0 {
		vendor = c.famVend[fams[0]]
	}
	if vendor == "" {
		return fams
	}
	extra := []string{}
	for f, n := range c.famName {
		if c.famVend[f] == vendor && strings.Contains(strings.ToLower(n), "tls") && !containsStr(fams, f) {
			extra = append(extra, f)
		}
	}
	sort.Strings(extra)
	return append(extra, fams...)
}

// --- данные хода ---

// guideFamData — семейство на ходе: офферы под условия, отклонённые правилами права использования, факты.
type guideFamData struct {
	fid      string
	short    string
	matched  []guideOfferView
	rejected []guideRejected
	excluded string // причина исключения по требуемой функции ("" — не исключено)
	exclFact *guideFact
}

type guideRejected struct {
	v   guideOfferView
	why guideViolation
}

// guideData — данные хода.
type guideData struct {
	fams     []*guideFamData
	qty      int
	kind     string
	ruleHits []guideViolation // проверенные правила базы знаний, отсекшие офферы
}

// gather — get_family_offers + validate_license + get_facts по семействам.
func (t *guideTurn) gather(fams []string) *guideData {
	t2 := time.Now()
	r := t.st.Requirements
	kind, n := mainObject(r)
	qty := max(n, 1)
	data := &guideData{qty: qty, kind: kind}
	shorts := t.cat.shortTitles(fams)
	offersSeen, matchedN, rejectedN := 0, 0, 0
	for _, fid := range fams {
		fd := &guideFamData{fid: fid, short: shorts[fid]}
		views, policy := t.cat.familyOffers(fid, r, t.st.Origin["term_months"], t.kb)
		offersSeen += len(views)
		match := func(policy guideTermPolicy) {
			fd.matched, fd.rejected = nil, nil
			for _, v := range views {
				bad := validateLicense(v, r, t.kb, policy)
				if len(bad) == 0 {
					fd.matched = append(fd.matched, v)
					continue
				}
				if why, ok := rightOfUse(bad); ok {
					fd.rejected = append(fd.rejected, guideRejected{v: v, why: why})
				}
			}
		}
		match(policy)
		if len(fd.matched) == 0 && policy != termAny && t.st.Origin["term_months"] != originUser {
			match(termAny)
		}
		sortOffers(fd.matched, qty, t.cat.famName[fid])
		if r.Use == "tls_server" {
			sort.SliceStable(fd.matched, func(i, j int) bool {
				return fd.matched[i].scope == "tls_server" && fd.matched[j].scope != "tls_server"
			})
		}
		ver := versionToken(t.cat.famName[fid])
		sort.SliceStable(fd.rejected, func(i, j int) bool {
			a, b := offerTotal(fd.rejected[i].v, qty), offerTotal(fd.rejected[j].v, qty)
			if a != b {
				return a < b
			}
			return ver != "" && strings.Contains(fd.rejected[i].v.p.Title, ver) && !strings.Contains(fd.rejected[j].v.p.Title, ver)
		})
		for _, rj := range fd.rejected {
			if rj.why.Status == kbDraft {
				t.prelim = true
			}
			if rj.why.Status == kbVerified {
				data.ruleHits = append(data.ruleHits, rj.why)
			}
		}
		for _, v := range fd.matched {
			if v.scope == "" && (kind == "server" || r.Use != "") && !v.marked {
				t.gap("license_unresolved")
			}
		}
		matchedN += len(fd.matched)
		rejectedN += len(fd.rejected)
		for _, axis := range r.RequiredFeatures {
			f := t.kb.factOn(fid, axis, r.Variant)
			switch {
			case f.verified():
				if k, _ := f.factValue(); k == "no" {
					fd.excluded, fd.exclFact = "в нём нет функции «"+strings.ToLower(axisLabel(axis))+"»", f
					if axis == "encryption" {
						fd.excluded = "в нём нет шифрования"
					}
				}
			default:
				t.prelim = true
				t.gap("missing_fact")
			}
		}
		data.fams = append(data.fams, fd)
	}
	t.step("Офферы семейств", toolGetFamilyOffers, fmt.Sprintf("позиций: %d", offersSeen), t2)
	t.step("Правила лицензирования", toolValidateLicense, fmt.Sprintf("подходят: %d, не подходят по праву использования: %d", matchedN, rejectedN), t2)
	facts := t.kb.factsFor(fams, r.Platform)
	verified := 0
	for _, f := range facts {
		if f.verified() {
			verified++
		}
	}
	t.step("Факты базы знаний", toolGetFacts, fmt.Sprintf("проверенных фактов: %d", verified), t2)
	if matchedN == 0 {
		t.gap("no_matching_offer")
	}
	return data
}

// rightOfUse — нарушение, которое покупателю стоит показать блоком «Не подходит …»: область действия или правило
// базы знаний. Операция, срок, ступень — просто другие позиции, их не показываем.
func rightOfUse(bad []guideViolation) (guideViolation, bool) {
	var hit *guideViolation
	for i := range bad {
		switch bad[i].RuleID {
		case ruleCodeScope:
			if hit == nil {
				hit = &bad[i]
			}
		case ruleCodeOperation, ruleCodeOrg, ruleCodeTerm, ruleCodeTier, ruleCodeVariant, ruleCodeUnit:
			return guideViolation{}, false
		default:
			hit = &bad[i] // правило базы знаний важнее правила кода: у него источник
		}
	}
	if hit == nil {
		return guideViolation{}, false
	}
	return *hit, true
}

// offerTotal — сумма на qty (без цены — бесконечность: в конец списка).
func offerTotal(v guideOfferView, qty int) float64 {
	if !v.priced() {
		return math.Inf(1)
	}
	return *v.p.PriceRub * float64(v.units(qty))
}

// sortOffers — дешевле на заданное количество выше; при равенстве — позиция той версии, что в названии семейства.
func sortOffers(vs []guideOfferView, qty int, famName string) {
	ver := versionToken(famName)
	sort.SliceStable(vs, func(i, j int) bool {
		a, b := offerTotal(vs[i], qty), offerTotal(vs[j], qty)
		if a != b {
			return a < b
		}
		ai, bi := ver != "" && strings.Contains(vs[i].p.Title, ver), ver != "" && strings.Contains(vs[j].p.Title, ver)
		if ai != bi {
			return ai
		}
		return vs[i].p.ID < vs[j].p.ID
	})
}

// versionToken — номер версии в названии («КриптоПро CSP 5.0» → «5.0»).
func versionToken(name string) string {
	for _, w := range strings.Fields(name) {
		if strings.Contains(w, ".") {
			if _, err := strconv.ParseFloat(w, 64); err == nil {
				return w
			}
		}
	}
	return ""
}

// shortTitles — короткие подписи колонок: название без общего начала («Стандартный», «Расширенный»).
func (c *guideCatalog) shortTitles(fams []string) map[string]string {
	out := map[string]string{}
	if len(fams) < 2 {
		for _, f := range fams {
			out[f] = c.famName[f]
		}
		return out
	}
	words := make([][]string, len(fams))
	for i, f := range fams {
		words[i] = strings.Fields(c.famName[f])
	}
	common := len(words[0])
	for _, w := range words[1:] {
		k := 0
		for k < common && k < len(w) && strings.EqualFold(w[k], words[0][k]) {
			k++
		}
		common = k
	}
	for i, f := range fams {
		rest := strings.Join(words[i][min(common, len(words[i])):], " ")
		if rest == "" || common == 0 {
			rest = c.famName[f]
		}
		out[f] = rest
	}
	return out
}

// --- сохранение журнала хода ---

// saveTurn — журнал хода (SPEC §13): в лог и в guide_turns, если есть база. Текст реплик не пишется.
func (g *guideService) saveTurn(ctx context.Context, sessionID string, t *guideTurn, res *out.GuideResponse) {
	zap.L().Info("подбор: ход", zap.String("selection_id", t.st.SelectionID), zap.Int("turn", t.st.Turn),
		zap.String("type", res.Type), zap.Int("llm_calls", t.meta.LLMCalls), zap.Int("in", t.meta.InputTokens),
		zap.Int("out", t.meta.OutputTokens), zap.Float64("cost_rub", t.meta.CostRub), zap.Float64("ms", res.Meta.MsTotal),
		zap.String("fallback", t.meta.Fallback), zap.Strings("gaps", t.gaps), zap.Int("removed", len(t.removed)))
	if !g.s.repos.Enabled() {
		return
	}
	tools, _ := json.Marshal(t.trace)
	removed, _ := json.Marshal(nonNilStrings(t.removed))
	gaps, _ := json.Marshal(nonNilStrings(t.gaps))
	row := &domain.GuideTurn{SelectionID: t.st.SelectionID, SessionID: optional(sessionID), Turn: t.st.Turn, Type: res.Type,
		Tools: tools, LLMCalls: t.meta.LLMCalls, InputTokens: t.meta.InputTokens, OutputTokens: t.meta.OutputTokens,
		CostRub: t.meta.CostRub, MsFirst: int(t.meta.MsFirst), MsChecked: int(t.meta.MsChecked), MsTotal: int(res.Meta.MsTotal),
		Removed: removed, Gaps: gaps, Fallback: t.meta.Fallback, CatalogVer: t.cat.version, KBVersion: t.kb.version,
		TurnStartedAt: t.started}
	g.s.writes.Add(1)
	go func() {
		defer g.s.writes.Done()
		wctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), saveTimeout)
		defer cancel()
		if err := g.s.repos.SaveGuideTurn(wctx, row); err != nil {
			zap.L().Warn("подбор: журнал хода не записан", zap.Error(err))
		}
	}()
}

func nonNilStrings(ss []string) []string {
	if ss == nil {
		return []string{}
	}
	return ss
}

// directionFamilies — общий запрос «защита компьютеров»: только семейства направления (антивирус, шлюз и
// резервное копирование не сравниваются как взаимозаменяемые): сначала найденные поиском, затем остальные.
func (t *guideTurn) directionFamilies(fams []string, dir string) []string {
	allowed := t.kb.subclass(dir)
	if len(allowed) == 0 {
		return fams
	}
	out := []string{}
	for _, f := range fams {
		if containsStr(allowed, f) {
			out = append(out, f)
		}
	}
	for _, f := range allowed {
		if len(t.cat.byFamily[f]) > 0 && !containsStr(out, f) {
			out = append(out, f)
		}
	}
	return out
}
