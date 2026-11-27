package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"regexp"
	"slices"
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
	g        *guideService
	cat      *guideCatalog
	kb       *guideKB
	st       *domain.GuideState
	started  time.Time
	reads    int
	calls    int
	trace    []out.GuideTraceStep
	meta     out.GuideMeta
	gaps     []string
	prelim   bool
	removed  []string
	changes  []out.GuideChange
	mixed    bool // в реплике была посторонняя часть
	gapItems []guideGapItem
	text     string // реплика хода
	// altFams — семейства, найденные find_alternatives на этом ходе (проверка «из набора хода»).
	altFams map[string]bool
	query   string // строка поиска подбора
	// prevOffers — семейство → оффер, показанный на прошлом ходе (для названного отличия в changes).
	prevOffers map[string]string
	cross      bool // сравнение между вендорами
	// extraNums — тексты, числа из которых фраза хода может называть (пределы тарифов вне фида).
	extraNums []string
	// data — офферы и факты хода (после gather); before — требования до хода (экран контура экрана, «Что изменилось»).
	data   *guideData
	before screenSnapshot
	// Помощник (контур помощника): src — реплика или запрос хода (для «Показать все в поиске»); matchTotal — подходящих
	// семейств до отсечения четырёх; outKind — вид ответа вне подбора (скидка, оплата, оборудование).
	src        string
	matchTotal int
	outKind    string
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
	t.before = snapshotRequirements(sess.state)
	if req.Chip != nil && req.Chip.Key == "start_over" {
		t.before.turn = 0
	}
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
	t.screen(res)
	t.chatMessage(res)
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
	if containsStr([]string{"commercial", "government", "academic", "nonprofit", "home"}, r.OrgType) {
		st.Requirements.OrgType = r.OrgType
	}
	for _, o := range r.Objects {
		if containsStr([]string{"workstation", "server", "user", "connection", "host"}, o.Kind) && o.Count >= 0 && o.Count <= maxQuoteQuantity && len(st.Requirements.Objects) < 4 {
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
	// Чат: реплика о другом вендоре («самый дешёвый криптопро…» после Kaspersky) — новый подбор с этой реплики,
	// а не уточнение прежнего.
	if !first && req.Chip == nil && text != "" && t.newTopic(text) {
		*st = newGuideState(t.cat.version, t.kb.version)
		st.Turn, first = 1, true
		req.Query, text = text, ""
		t.before = snapshotRequirements(*st)
		t.before.turn = 0
	}
	if req.Chip != nil {
		if req.Chip.Key == "start_over" {
			*st = newGuideState(t.cat.version, t.kb.version)
			st.Turn, first = 1, true
		} else if req.Chip.Key == "direction" && strings.TrimSpace(req.Chip.Value) == "" {
			st.Requirements.Task, st.CandidateFamilyIDs = "", []string{}
			delete(st.Origin, "task")
		} else if req.Chip.Key == "direction" && t.kb.subclass(req.Chip.Value) != nil {
			st.Requirements.Task, st.Origin["task"] = req.Chip.Value, originUser
			st.CandidateFamilyIDs, st.Pending = []string{}, ""
		} else if req.Chip.Key == "family" && len(t.cat.byFamily[req.Chip.Value]) > 0 {
			st.CandidateFamilyIDs, st.Pending = []string{req.Chip.Value}, ""
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
	// «не знаю» текстом на вопрос об уровне защищённости — как кнопка «не знаю».
	if req.Chip == nil && st.Pending == "variant" && reDontKnow.MatchString(guideNorm(text)) {
		st.Unresolved = appendUnique(st.Unresolved, "variant")
		st.Skipped++
		st.Pending, text = "", ""
	}
	src := text
	t.text, t.query = text, req.Query
	t.prevOffers = map[string]string{}
	for _, id := range st.SelectedOfferIDs {
		if p := t.cat.products[id]; p != nil {
			t.prevOffers[p.FamilyID] = id
		}
	}
	if first && req.Query != "" {
		qd := extractConditions(req.Query)
		fixObjectKinds(&qd, *st)
		applyDelta(st, qd, originUser)
		if src == "" {
			src = req.Query
		}
		// Первая реплика чата приходит запросом: служебное и постороннее — так же, как у реплики.
		if text == "" {
			if r := t.serviceReply(req.Query); r != nil {
				return r
			}
			if offTopic(req.Query) && qd.empty() && !asksCompare(req.Query) {
				return t.outOfScope(req.Query)
			}
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
		// «нужно хранение на лентах» — требуемая функция по подписям фактов базы знаний у кандидатов и их линейки.
		if len(d.features) == 0 && reNeedFeature.MatchString(guideNorm(text)) && !asksCompare(text) {
			d.features = t.featuresFromKB(text, t.cat.lineFamilies(t.kb, st.CandidateFamilyIDs))
		}
		t.changes = append(t.changes, applyDelta(st, d, originUser)...)
		st.Pending = ""
	}
	t.step("Условия", "", fmt.Sprintf("условий: %d", len(conditionsOf(*st))), t0)
	t.src = src
	// Оборудование («подобрать комп», «ноутбук») подбор не ведёт: он только для лицензий на ПО. Пока задача, вендор и
	// кандидаты не названы — отказ одной фразой, без инструментов; обычная выдача остаётся на экране.
	if src != "" && st.Requirements.Task == "" && st.Requirements.Vendor == "" && len(st.CandidateFamilyIDs) == 0 &&
		t.g.s.hardwareQuery(ctx, src) {
		return t.hardwareReply()
	}

	compare := src != "" && asksCompare(src)
	r := &st.Requirements
	// Вопрос о свойстве продукта (D8): ответ только фактом.
	if src != "" && !compare && reAsksFeature.MatchString(guideNorm(src)) {
		if res := t.featureQuestion(src); res != nil {
			return res
		}
	}
	// Общий запрос по ИБ (D4): сначала «Что защищаем?».
	if r.Task == "" && r.Vendor == "" && len(r.Objects) == 0 && reGeneralIS.MatchString(guideNorm(src)) && st.Questions < 2 {
		return t.askDirection()
	}
	_, why := routeQuery(req.Query)
	// Запрос называет продукт («windows server»), а не только вендора («kaspersky»): число мест не спрашиваем.
	productNamed := req.Query != "" && why == "product_name" && guideNamesProduct(req.Query)
	askCount := len(r.Objects) == 0 && !compare && !r.Cheapest && st.Questions < 2 && st.Skipped < 2
	// Вендор назван — сначала семейства: у тарифов с пределами (AnyDesk) первый вопрос другой.
	if askCount && r.Vendor == "" && !productNamed {
		return t.askCount()
	}

	// Чтение 1: семейства.
	t1 := time.Now()
	t.read()
	var fams []string
	if vendors := guideVendorsIn(src); compare && len(vendors) >= 2 {
		// Сравнение между вендорами: по семейству на вендора внутри направления, только приведённые единицы.
		fams = t.crossVendorFamilies(vendors, directionOf(orDefault(r.Task, "endpoint")))
		if len(fams) >= 2 {
			t.cross, st.Intent = true, "comparison"
			r.Vendor = ""
			delete(st.Origin, "vendor")
		} else {
			fams = nil
		}
	}
	if compare && len(fams) == 0 {
		var allowed []string
		if r.Vendor == "" && len(st.CandidateFamilyIDs) == 0 {
			allowed = t.kb.subclass(directionOf(r.Task))
		}
		fams = t.cat.editionFamilies(t.kb, src, r.Vendor, st.CandidateFamilyIDs, allowed)
		if len(fams) < 2 && len(allowed) > 0 {
			fams = t.cat.editionFamilies(t.kb, src, r.Vendor, st.CandidateFamilyIDs, nil)
		}
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
		qtext := strings.TrimSpace(req.Query + " " + text)
		// В поиск — без числа мест: «на 3 компьютера» тянет в выдачу оборудование.
		stext := strings.TrimSpace(reGuideCountPhrase.ReplaceAllString(guideNorm(qtext), " "))
		fams = t.g.searchCatalog(ctx, t.cat, stext, r.Vendor)
		// Названный продукт должен быть среди кандидатов, даже если поиск его не поднял.
		for _, f := range t.nameMatchIfProduct(stext, r.Vendor) {
			if !containsStr(fams, f) {
				fams = append(fams, f)
			}
		}
		if r.Vendor != "" && !guideNamesProduct(qtext) {
			// Назван только вендор: сначала его линейки из базы знаний (KES, KSOS), затем выдача поиска.
			fams = append(t.vendorLines(r.Vendor), fams...)
			fams = slices.Compact(fams)
		}
		fams = t.cat.softwareOnly(fams)
		fams = t.cat.focusFamilies(fams, qtext)
		if dir := directionOf(r.Task); dir != "" && r.Vendor == "" {
			fams = t.directionFamilies(fams, dir)
		}
		if len(fams) > 8 {
			fams = fams[:8]
		}
	}
	if r.Use == "tls_server" {
		fams = t.cat.withTLSFamilies(fams, r.Vendor)
	}
	fams = t.overLimitFallback(fams)
	if len(r.RequiredFeatures) > 0 && !compare && !t.anyHasFeatures(fams) {
		// Требуемой функции нет ни у одного кандидата — она может быть в другой редакции линейки (Стандартная →
		// Расширенная).
		fams = t.cat.lineFamilies(t.kb, fams)
	}
	fams = t.cat.demoteCompanions(fams, src+" "+req.Query)
	if r.Vendor == "" && len(fams) > 0 && compare && !t.cross {
		r.Vendor = t.cat.famVend[fams[0]]
		st.Origin["vendor"] = originInferred
	}
	st.CandidateFamilyIDs = append([]string{}, fams...)
	t.step("Найдены семейства", toolSearchCatalog, fmt.Sprintf("семейств: %d", len(fams)), t1)
	if len(fams) == 0 {
		t.gap("no_matching_offer")
		return t.noData("Под эти условия в каталоге ничего не нашли — измените условия или запросите КП")
	}
	if line := t.capacityLine(fams, src); line != "" && !compare {
		t.read()
		res := t.capacityTurn(line)
		if t.mixed {
			res.Changes = append(res.Changes, out.GuideChange{Condition: "посторонняя часть", Effect: "в каталоге ПО и оборудования это не подбираю"})
		}
		res.Preliminary = t.prelim
		return res
	}
	if lf := t.limitFamily(fams); lf != "" && !compare {
		t.read()
		st.CandidateFamilyIDs = []string{lf}
		res := t.limitsTurn(lf)
		res.Preliminary = t.prelim
		return res
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
	// Продукт назван: число мест спрашиваем, только если он продаётся на рабочие места или ступенями (CSP, Кибер
	// Бэкап, KES), а не одной позицией на сервер или по запросу (Windows Server, Office на 1 ПК).
	if askCount && len(r.Objects) == 0 && t.cat.soldPerSeat(fams, t.kb) {
		return t.askCount()
	}
	if ctx.Err() != nil {
		return nil
	}

	// Чтение 2: офферы, факты, правила.
	if !t.read() {
		return t.fallback(guideFbTurn)
	}
	data := t.gather(fams)
	t.data = data
	t.meta.MsFirst = t.ms(t.started)
	var res *out.GuideResponse
	if compare && len(fams) >= 2 {
		res = t.comparison(data)
		t.neighbourEditions(res, data)
	} else {
		res = t.selection(data)
		t.onRequestBlock(res, data, text)
		t.renewalQuestion(res, data)
		t.tlsAlternative(res, data)
	}
	t.namedTierChanges(res)
	t.perpetualTerm(res)
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
		t.outKind = chatOutPrompt
	case strings.Contains(q, "скидк") || strings.Contains(q, "отправь") || strings.Contains(q, "забудь"):
		lead = "Скидки я не назначаю и ничего не отправляю без вас. Могу собрать спецификацию по ценам каталога."
		t.outKind = chatOutDiscount
	default:
		return nil
	}
	t.meta.Fallback = guideFbNoNeed
	t.step("Вне подбора", "", "отказ без инструментов", t.started)
	return &out.GuideResponse{Type: out.GuideOutOfScope, Lead: lead, Claims: []out.GuideClaim{}, Offers: []out.GuideOffer{},
		Changes: t.changes, Next: []string{"change_conditions", "request_quote"}}
}

// guideHardwareLead — ответ подбора на запрос об оборудовании.
const guideHardwareLead = "Подбор помогает выбрать лицензии на программы. Компьютеры и другое оборудование — в выдаче поиска."

// hardwareReply — запрос об оборудовании: подбор не применим (out_of_scope), без инструментов и без модели.
func (t *guideTurn) hardwareReply() *out.GuideResponse {
	t.meta.Fallback = guideFbNoNeed
	t.outKind = chatOutHardware
	t.step("Вне подбора", "", "оборудование — подбор только для лицензий", t.started)
	return &out.GuideResponse{Type: out.GuideOutOfScope, Lead: guideHardwareLead, Claims: []out.GuideClaim{},
		Offers: []out.GuideOffer{}, Changes: t.changes, Next: []string{"change_conditions"}}
}

// outOfScope — постороннее или сервисное (оплата, доставка): одна фраза.
func (t *guideTurn) outOfScope(text string) *out.GuideResponse {
	lead := "Это в каталоге ПО и оборудования не подбираю. Чем помочь с выбором лицензий?"
	t.outKind = chatOutService
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
	words := guideProductTokens(text)
	if len(words) == 0 {
		return fams
	}
	best, scores := 0, make([]int, len(fams))
	for i, f := range fams {
		scores[i] = guideNameScore(c.famName[f], words)
		best = max(best, scores[i])
	}
	if best < 1 {
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

var (
	reGuideSplit = regexp.MustCompile(`[^а-яёa-z0-9.]+`)
	// reGuideCountPhrase — «на 3 компьютера», «на 256 ip», «16 хостов»: число мест — условие, а не название продукта.
	reGuideCountPhrase = regexp.MustCompile(`(на\s+)?\d{1,6}\s*(пк|компьютер[а-яё]*|рабоч[а-яё]*(\s+мест[а-яё]*|\s+станц[а-яё]*)?|мест[а-яё]*|узл[а-яё]*|устройств[а-яё]*|машин[а-яё]*|сервер[а-яё]*|пользовател[а-яё]*|сотрудник[а-яё]*|человек|подключени[а-яё]*|лицензи[а-яё]*|шт[а-яё]*|хост[а-яё]*|ip|адрес[а-яё]*|ящик[а-яё]*|участник[а-яё]*)`)
)

// guideGenericWords — слова задачи, а не названия продукта: по ним сужать кандидатов нельзя.
var guideGenericWords = map[string]bool{"защита": true, "защиты": true, "защиту": true, "антивирус": true, "решение": true,
	"безопасности": true, "офиса": true, "офисе": true, "компании": true, "небольшой": true, "малого": true, "бизнеса": false,
	"год": true, "года": true, "лет": true, "новая": true, "продление": true, "продлить": true, "купить": true, "нужна": true,
	"нужен": true, "нужно": true, "подобрать": true, "подбери": true, "вебинара": true, "вебинар": true, "хостов": true}

// guideProductTokens — слова названия продукта из запроса: без числа мест, служебных слов и слов задачи.
func guideProductTokens(text string) []string {
	q := reGuideCountPhrase.ReplaceAllString(guideNorm(text), " ")
	out := []string{}
	for _, w := range reGuideSplit.Split(q, -1) {
		w = strings.Trim(w, ".")
		if len([]rune(w)) < 2 || guideStopWord(w) || guideGenericWords[w] {
			continue
		}
		switch w {
		case "на", "и", "в", "с", "по", "от", "до", "для", "или", "мне", "нам", "это", "какой", "какая":
			continue
		}
		out = append(out, w)
	}
	return out
}

// guideNameScore — сколько слов продукта есть в названии семейства (по началу слова названия).
func guideNameScore(name string, words []string) int {
	nameWords := reGuideSplit.Split(guideNorm(name), -1)
	score := 0
	for _, w := range words {
		r := []rune(w)
		stem := w
		if len(r) > 5 {
			stem = string(r[:len(r)-2])
		}
		// Короткое кириллическое слово — только целым словом названия: «комп» — не «КОМПАС», «ред» — не «редактор».
		short := len(r) <= 4 && isCyrillicWord(w)
		for _, nw := range nameWords {
			nw = strings.Trim(nw, ".")
			if nw != "" && (nw == stem || !short && strings.HasPrefix(nw, stem)) {
				score++
				break
			}
		}
	}
	return score
}

func guideStopWord(w string) bool {
	switch w {
	case "для", "самый", "самая", "самое", "дешевый", "дешевая", "дешевле", "подбери", "нужен", "нужна", "нужно",
		"сервера", "сервер", "компьютеров", "компьютера", "офиса", "мест", "лицензия", "лицензию", "лицензии", "как", "что", "чем":
		return true
	}
	return hwComputerWords[w] // «комп», «пк», «компьютер» — объект, а не название продукта

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
			if r.Variant == "" {
				if y := t.kb.factYesAnyVariant(fid, axis); y != nil {
					f = y // функция есть хотя бы в одной редакции внутри семейства (Сканер-ВС Enterprise)
				}
			}
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
	if r.Use == "tls_server" {
		any := false
		for _, fd := range data.fams {
			for _, v := range fd.matched {
				any = any || v.scope == "tls_server"
			}
		}
		if any {
			for _, fd := range data.fams {
				fd.matched = slices.DeleteFunc(fd.matched, func(v guideOfferView) bool { return v.scope != "tls_server" })
			}
		}
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
	items := []byte("[]")
	if len(t.gapItems) == 0 {
		// Пробел без своей записи — с вопросом хода и семействами-кандидатами.
		q := t.text
		if q == "" {
			q = t.query
		}
		for _, g := range t.gaps {
			t.gapItem(g, t.st.CandidateFamilyIDs, "", q)
		}
	}
	if len(t.gapItems) > 0 {
		items, _ = json.Marshal(t.gapItems)
	}
	row := &domain.GuideTurn{SelectionID: t.st.SelectionID, SessionID: optional(sessionID), Turn: t.st.Turn, Type: res.Type,
		Tools: tools, LLMCalls: t.meta.LLMCalls, InputTokens: t.meta.InputTokens, OutputTokens: t.meta.OutputTokens,
		CostRub: t.meta.CostRub, MsFirst: int(t.meta.MsFirst), MsChecked: int(t.meta.MsChecked), MsTotal: int(res.Meta.MsTotal),
		Removed: removed, Gaps: gaps, GapItems: items, Fallback: t.meta.Fallback, CatalogVer: t.cat.version, KBVersion: t.kb.version,
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

// namedTierChanges — смена числа мест: вместо общей фразы — названное отличие по каждому семейству
// («ступень 25–49 → 50–99, цена за место 2 813,40 → 2 715,30 ₽»; SPEC §9).
func (t *guideTurn) namedTierChanges(res *out.GuideResponse) {
	var named []out.GuideChange
	for _, o := range res.Offers {
		if o.Role != out.OfferMatch && o.Role != out.OfferAlternative {
			continue
		}
		prevID, ok := t.prevOffers[o.FamilyID]
		if !ok || prevID == o.OfferID {
			continue
		}
		prev := t.cat.products[prevID]
		if prev == nil {
			continue
		}
		pv := t.cat.view(prev, t.kb)
		if pv.tierLabel() == o.Tier || o.Tier == "" || pv.tierLabel() == "" {
			continue
		}
		eff := fmt.Sprintf("«%s»: ступень %s → %s", o.FamilyTitle, pv.tierLabel(), o.Tier)
		if pv.priced() && o.PricePerUnit != nil {
			eff += fmt.Sprintf(", цена за место %s → %s ₽", rubRU(*prev.PriceRub), rubRU(*o.PricePerUnit))
		}
		named = append(named, out.GuideChange{Condition: "число мест", Effect: eff})
	}
	if len(named) == 0 {
		return
	}
	kept := res.Changes[:0:0]
	for _, c := range res.Changes {
		if !strings.HasSuffix(c.Effect, "ступень и сумма пересчитаны") {
			kept = append(kept, c)
		}
	}
	res.Changes = append(named, kept...)
}

// perpetualTerm — срок по умолчанию не висит над бессрочными позициями: если все подходящие карточки бессрочные,
// условие «1 год» снимается (D2: серверная лицензия КриптоПро).
func (t *guideTurn) perpetualTerm(res *out.GuideResponse) {
	r := &t.st.Requirements
	if t.st.Origin["term_months"] != originDefault || r.TermMonths == 0 {
		return
	}
	n := 0
	for _, o := range res.Offers {
		if o.Role == out.OfferMatch {
			if o.Term != "бессрочная" {
				return
			}
			n++
		}
	}
	if n > 0 {
		r.TermMonths = 0
		delete(t.st.Origin, "term_months")
	}
}

// rubRU — «2 813,40».
func rubRU(v float64) string {
	s := fmt.Sprintf("%.2f", v)
	ip, fp := s[:len(s)-3], s[len(s)-2:]
	var b strings.Builder
	for i, c := range ip {
		if i > 0 && (len(ip)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(c)
	}
	if fp == "00" {
		return b.String()
	}
	return b.String() + "," + fp
}

// demoteCompanions — клиентские лицензии (CAL) — дополнение к серверу, а не сам продукт: в конец, если запрос
// не про CAL (D5 «windows server»).
func (c *guideCatalog) demoteCompanions(fams []string, q string) []string {
	if strings.Contains(guideNorm(q), "cal") {
		return fams
	}
	isCAL := func(f string) bool {
		for _, w := range strings.Fields(guideNorm(c.famName[f])) {
			if w == "cal" {
				return true
			}
		}
		return false
	}
	out := make([]string, 0, len(fams))
	for _, f := range fams {
		if !isCAL(f) {
			out = append(out, f)
		}
	}
	for _, f := range fams {
		if isCAL(f) {
			out = append(out, f)
		}
	}
	return out
}

// renewalQuestion — продление без названной редакции (D6): выдача остаётся, над ней вопрос «Какая редакция у вас
// сейчас?» с чипами семейств; ответ сужает подбор до одной редакции.
func (t *guideTurn) renewalQuestion(res *out.GuideResponse, data *guideData) {
	if t.st.Requirements.Operation != "renewal" || len(t.st.CandidateFamilyIDs) < 2 || t.st.Questions >= 2 {
		return
	}
	chips := []out.GuideChip{}
	for _, o := range res.Offers {
		if o.Role == out.OfferMatch && len(chips) < 5 {
			chips = append(chips, out.GuideChip{Label: t.shortOf(data, o.FamilyID), Key: "family", Value: o.FamilyID})
		}
	}
	if len(chips) < 2 {
		return
	}
	t.st.Pending, t.st.Questions = "family", t.st.Questions+1
	res.Clarification = &out.GuideClarify{Key: "family", Question: "Какая редакция у вас сейчас?", Chips: chips}
}

// softwareOnly — подбор лицензий: оборудование (батареи, сканеры штрихкодов) в кандидаты не идёт.
func (c *guideCatalog) softwareOnly(fams []string) []string {
	out := fams[:0:0]
	for _, f := range fams {
		ps := c.byFamily[f]
		if len(ps) > 0 && ps[0].Kind == "hardware" {
			continue
		}
		out = append(out, f)
	}
	return out
}

// reNeedFeature — реплика называет требование («нужно …», «требуется …»).
var reNeedFeature = regexp.MustCompile(`(нужн[а-яё]*|нужен|требу[а-яё]*|необходим[а-яё]*|обязательн[а-яё]*)`)

// featuresFromKB — оси фактов кандидатов, подпись которых совпадает со словами реплики (лучшее совпадение).
func (t *guideTurn) featuresFromKB(text string, fams []string) []string {
	stems := featureStems(text, "")
	if len(stems) == 0 {
		return nil
	}
	best, axes := 0, []string{}
	seen := map[string]bool{}
	for _, f := range fams {
		for _, fact := range t.kb.byFamily[f] {
			if seen[fact.Axis] || !fact.verified() {
				continue
			}
			label := guideNorm(fact.AxisLabel + " " + axisLabel(fact.Axis))
			score := 0
			for _, s := range stems {
				if strings.Contains(label, s) {
					score++
				}
			}
			if score == 0 {
				continue
			}
			seen[fact.Axis] = true
			switch {
			case score > best:
				best, axes = score, []string{fact.Axis}
			case score == best:
				axes = append(axes, fact.Axis)
			}
		}
	}
	sort.Strings(axes)
	if len(axes) > 1 {
		return nil // неоднозначно — не угадываем
	}
	return axes
}

// lineFamilies — семейства и их соседи по линейке базы знаний (редакции одного продукта), порядок сохраняется.
func (c *guideCatalog) lineFamilies(kb *guideKB, fams []string) []string {
	out := append([]string{}, fams...)
	lines := map[string]bool{}
	for _, f := range fams {
		if l := kb.lines[f].Line; l != "" {
			lines[l] = true
		}
	}
	extra := []string{}
	for f, l := range kb.lines {
		if lines[l.Line] && !containsStr(out, f) && len(c.byFamily[f]) > 0 {
			extra = append(extra, f)
		}
	}
	sort.Slice(extra, func(i, j int) bool {
		return kb.lines[extra[i]].Rank < kb.lines[extra[j]].Rank || kb.lines[extra[i]].Rank == kb.lines[extra[j]].Rank && extra[i] < extra[j]
	})
	return append(out, extra...)
}

// guideNamesProduct — в запросе есть слово продукта сверх вендора («microsoft office», а не «kaspersky»).
func guideNamesProduct(q string) bool {
	vendorWord := func(w string) bool {
		for _, v := range guideVendorStems {
			if strings.HasPrefix(w, v.stem) || strings.HasPrefix(v.stem, w) && len([]rune(w)) >= 4 {
				return true
			}
		}
		return false
	}
	for _, w := range guideProductTokens(q) {
		if !vendorWord(w) {
			return true
		}
	}
	return false
}

// overLimitFallback — все кандидаты меньше нужного числа мест (KSOS на 40): соседи по направлению того же вендора.
func (t *guideTurn) overLimitFallback(fams []string) []string {
	_, n := mainObject(t.st.Requirements)
	if n == 0 || len(fams) == 0 {
		return fams
	}
	for _, f := range fams {
		if lim := t.cat.familyLimit(f, t.kb); lim == 0 || n <= lim {
			return fams
		}
	}
	vendor := t.cat.famVend[fams[0]]
	out := append([]string{}, fams...)
	for _, d := range t.kb.directions() {
		if !slices.ContainsFunc(d.Families, func(f string) bool { return containsStr(fams, f) }) {
			continue
		}
		for _, f := range d.Families {
			// Только линейки базы знаний (KES для бизнеса), не облачные и EDR-продукты другого класса.
			if t.cat.famVend[f] == vendor && !containsStr(out, f) && t.kb.lines[f].Line != "" {
				out = append(out, f)
			}
		}
	}
	return out
}

// soldPerSeat — у первого семейства есть позиции с ценой на рабочее место, ступенью или пакетом мест.
func (c *guideCatalog) soldPerSeat(fams []string, kb *guideKB) bool {
	if len(fams) == 0 {
		return false
	}
	for _, p := range c.byFamily[fams[0]] {
		v := c.view(p, kb)
		if !v.priced() || (v.kind != "base" && v.kind != "") {
			continue
		}
		if v.tierMin > 1 || v.unit == "pack" || v.scope == "workstation" {
			return true
		}
	}
	return false
}

// anyHasFeatures — у кого-то из кандидатов есть все требуемые функции (по проверенным фактам).
func (t *guideTurn) anyHasFeatures(fams []string) bool {
	for _, f := range fams {
		ok := true
		for _, axis := range t.st.Requirements.RequiredFeatures {
			if t.kb.factYesAnyVariant(f, axis) == nil {
				ok = false
			}
		}
		if ok {
			return true
		}
	}
	return false
}

// nameMatchIfProduct — семейства по словам названия, только если назван продукт сверх вендора: «kaspersky на 50
// пк» не должен тянуть Password Manager и курсы.
func (t *guideTurn) nameMatchIfProduct(text, vendor string) []string {
	if !guideNamesProduct(text) {
		return nil
	}
	return t.cat.nameMatch(text, vendor, 8)
}

// vendorLines — семейства вендора из линеек базы знаний в порядке линейки и ранга.
func (t *guideTurn) vendorLines(vendor string) []string {
	var out []string
	for f, l := range t.kb.lines {
		if t.cat.famVend[f] == vendor && l.Line != "" && len(t.cat.byFamily[f]) > 0 {
			out = append(out, f)
		}
	}
	sort.Slice(out, func(i, j int) bool {
		a, b := t.kb.lines[out[i]], t.kb.lines[out[j]]
		if a.Line != b.Line {
			return a.Line < b.Line
		}
		if a.Rank != b.Rank {
			return a.Rank < b.Rank
		}
		return out[i] < out[j]
	})
	return out
}

// reDontKnow — «не знаю», «без понятия», «не уверен».
var reDontKnow = regexp.MustCompile(`^(не знаю|незнаю|без понятия|не уверен[а]?|хз)[.!]?$`)

// newTopic — реплика называет вендора, которого нет среди условий и кандидатов подбора: это новый вопрос.
func (t *guideTurn) newTopic(text string) bool {
	vendors := guideVendorsIn(text)
	if len(vendors) == 0 {
		return false
	}
	known := map[string]bool{}
	if v := t.st.Requirements.Vendor; v != "" {
		known[v] = true
	}
	for _, f := range t.st.CandidateFamilyIDs {
		known[t.cat.famVend[f]] = true
	}
	if len(known) == 0 {
		return false
	}
	for _, v := range vendors {
		if known[v] {
			return false
		}
	}
	return true
}
