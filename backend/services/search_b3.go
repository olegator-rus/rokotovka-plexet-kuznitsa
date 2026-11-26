package services

import (
	"context"
	"fmt"
	"math"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Поиск B3/B4: путь запроса целиком (ARCHITECTURE §2). Трасса объясняет каждую позицию: из какого
// источника она пришла, почему стоит в своей группе, из чего сложился итоговый скор, кто и почему отсеян.

// Ступени лестницы пустой и слабой выдачи (trace relaxation.ladder).
const (
	ladderAliases  = "aliases"
	ladderSemantic = "semantic"
	ladderRelax    = "relax_soft"
	ladderAnalogs  = "analogs"
	ladderEmpty    = "empty"
)

// searchB3 — B3 (правила версии 1) и B4 (последняя принятая версия правил).
func (s *Services) searchB3(ctx context.Context, requestID string, req in.SearchRequest, cfg string, limit int,
	st *catalogState, opts SearchOptions, started time.Time) (*out.SearchResponse, error) {
	lx := st.lex
	rules := s.rulesFor(ctx, cfg, lx, opts.Rules)
	trace := &domain.Trace{
		RequestID: requestID, Route: domain.RouteNormalized, ParseSource: domain.ParseRules, Config: cfg,
		EffectiveConfig: cfg, RulesVersion: rules.version, CatalogVersion: st.version,
		Items: []domain.TraceItem{}, Rejected: []domain.RejectedHit{},
	}
	resp := &out.SearchResponse{
		RequestID: requestID, Query: req.Query, Config: cfg,
		Chips: []out.Chip{}, Items: []out.SearchItem{}, Relaxed: []out.Relaxed{}, RelatedQueries: []string{},
		Complements: []out.Complement{},
	}

	// normalize
	t := time.Now()
	nr := normalizeFull(req.Query, lx, rules, req.AsTyped)
	resp.Normalized = nr.normalized()
	if c := nr.correction; c != nil {
		resp.Correction = &out.Correction{From: c.from, To: c.to, Confidence: c.confidence, Applied: c.applied}
	}
	normStep := domain.NormalizeStep{StepBase: domain.StepBase{Name: domain.StepNormalize, Ms: msSince(t)}, Changes: nr.changes}

	// attributes + сущности
	t = time.Now()
	attrs := extractAttrs(nr.tokens)
	attrs.applyRequest(req)
	ent := findEntities(nr, attrs, lx, rules)
	ent.applyRemoved(removedSet(req.RemovedChips))
	chips, found := attrs.chips(ent, lx)
	attrs.found = found
	attrStep := domain.AttributesStep{StepBase: domain.StepBase{Name: domain.StepAttributes, Ms: msSince(t)},
		Found: found, Unresolved: attrs.unresolved()}

	// route
	t = time.Now()
	route, signals := routeSignals(nr, attrs, ent)
	routeStep := domain.RouteStep{StepBase: domain.StepBase{Name: domain.StepRoute}, Route: route, Signals: signals}
	routeStep.Ms = msSince(t)

	// intent: только маршрут D и только если разборщик подключён
	var intentStep any = skipped(domain.StepIntent, "not_route_d")
	if route == domain.RouteIntent {
		intentStep = skipped(domain.StepIntent, "parser_not_connected")
		if p := s.intentParser(); p != nil {
			t = time.Now()
			guess := rulesGuess(attrs, ent)
			parsed, err := p.Parse(WithRequestID(ctx, requestID), resp.Normalized, guess, st.segment(req.ClientID))
			if err != nil {
				parsed = guess
			}
			mergeParsed(parsed, &attrs, &ent, lx)
			step, llmInfo, cost := IntentTrace(parsed, msSince(t))
			intentStep = step
			trace.ParseSource = parsed.Source
			if parsed.Source == "" {
				trace.ParseSource = domain.ParseRules
			}
			if llmInfo != nil {
				trace.LLM = llmInfo
			}
			trace.CostRub = cost
			chips, found = attrs.chips(ent, lx)
			attrs.found = found
			attrStep.Found, attrStep.Unresolved = found, attrs.unresolved()
		}
	}
	trace.Route = route
	resp.Chips = chips
	resp.Clarify = attrs.clarify()

	info, err := s.indexInfo(ctx)
	if err != nil {
		if opensearch.IsNotFound(err) {
			return nil, errs.NewUnavailableError(MsgNoIndex).WithCause(err)
		}
		return nil, err
	}

	// Текст для поиска: без атрибутов, слов намерения и служебных слов; исходные слова алиасов — отдельно.
	var words, vendorWords []string
	for _, w := range ent.textTokens {
		switch {
		case lexStopWords[w]:
		case ent.vendorTokens[w] && len(ent.vendors) > 0 && !attrs.analog:
			vendorWords = append(vendorWords, w)
		default:
			words = append(words, w)
		}
	}
	if len(words) == 0 {
		words, vendorWords = vendorWords, nil
	}
	if attrs.renewal {
		vendorWords = append(vendorWords, "продление") // продление — фильтр; слово только усиливает
	}
	text := strings.Join(words, " ")
	if text == "" {
		text = strings.Join(ent.textTokens, " ")
	}
	if text == "" {
		text = resp.Normalized
	}
	denseText := strings.TrimSpace(strings.Join(ent.textTokens, " ") + " " + strings.Join(nr.expansions, " "))
	if denseText == "" {
		denseText = text
	}
	if attrs.origin == "ru" {
		denseText = "отечественное " + denseText
	}
	expansions := strings.TrimSpace(strings.Join(nr.expansions, " ") + " " + strings.Join(vendorWords, " "))

	denseCh := make(chan retrieval, 1)
	filters := strictFilters(attrs, ent)
	go func() { denseCh <- s.dense(ctx, denseText, info, filters...) }()
	bm := s.lexicalB3(ctx, text, expansions, b3MinMatch, filters)
	if bm.err == nil && len(bm.step.Top) == 0 && strings.Contains(text, " ") {
		// Все слова вместе не встретились ни в одной позиции — ослабляем до «любое слово».
		bm = s.lexicalB3(ctx, text, expansions, "1", filters)
		bm.step.Mode = "multi_match_any"
	}
	dn := <-denseCh
	if bm.err != nil {
		if opensearch.IsNotFound(bm.err) {
			s.index.Store(nil)
			return nil, errs.NewUnavailableError(MsgNoIndex).WithCause(bm.err)
		}
		return nil, bm.err
	}
	if len(bm.step.Top) == 0 && route != domain.RouteIntent {
		trace.Route = domain.RouteIntent
		routeStep.Route = domain.RouteIntent
		routeStep.Signals = append(routeStep.Signals, "zero_results")
	}

	// fusion
	t = time.Now()
	ranked := rrf(b3RRFK, bm.step.Top, dn.step.Top)
	fusionStep := domain.FusionStep{StepBase: domain.StepBase{Name: domain.StepFusion, Ms: msSince(t)}, K: b3RRFK}
	denseScore := map[string]float64{}
	for _, h := range dn.step.Top {
		denseScore[h.ProductID] = h.Score
	}

	// business: допустимость → порог → группы → скор
	t = time.Now()
	rc := s.rankContext(st, lx, req.ClientID, attrs, ent)
	absent := ent.absent()
	minDense := denseOnlyMin
	if ent.abbrev {
		minDense = abbrevDenseMin
	}
	noLetters := !hasLetters(resp.Normalized)
	for _, tok := range nr.tokens {
		noLetters = noLetters && !looksLikeSKU(tok)
	}
	ladder := []string{}
	if len(nr.aliasHits) > 0 {
		ladder = append(ladder, ladderAliases)
	}
	if !dn.step.Skipped && len(dn.step.Top) > 0 {
		ladder = append(ladder, ladderSemantic)
	}

	byFamily := map[string]float64{}
	best := 0.0
	for _, f := range ranked {
		best = math.Max(best, f.RRF)
	}
	fusedByID := map[string]fused{}
	for _, f := range ranked {
		d, ok := lx.docs[f.ProductID]
		if !ok {
			continue
		}
		rel := 0.0
		if best > 0 {
			rel = f.RRF / best
		}
		f.Relevance = rel
		fusedByID[f.ProductID] = f
		rc.productRel[f.ProductID] = rel
		if f.BM25Rank == nil && denseScore[f.ProductID] < minDense {
			addRejected(&trace.Rejected, f.ProductID, rejectDenseScore,
				fmt.Sprintf("найден только векторами, скор %.3f < %.3f", denseScore[f.ProductID], minDense))
			continue
		}
		if absent && denseScore[f.ProductID] < absentDenseMin {
			continue
		}
		if rel > byFamily[d.FamilyID] {
			byFamily[d.FamilyID] = rel
		}
	}

	var fams []*familyHit
	seen := map[string]bool{}
	addFamily := func(fid string, rel float64, group string) {
		if seen[fid] {
			return
		}
		seen[fid] = true
		fh := rc.buildFamily(fid, rel, &trace.Rejected)
		if fh == nil {
			return
		}
		fh.group = group
		fams = append(fams, fh)
	}
	switch {
	case noLetters || ent.nonProduct:
		// Шум и вопросы не о товаре: ни товаров, ни аналогов.
	case attrs.analog && len(ent.reference) > 0:
		// Просят замену: аналоги референса из справочника; сам референс не показывается.
		for _, ref := range ent.reference {
			for _, pid := range lx.families[ref] {
				addRejected(&trace.Rejected, pid, rejectReference, "это исходный продукт, просили замену")
			}
			seen[ref] = true
		}
		for _, ref := range ent.reference {
			for _, sub := range lx.substitutes[ref] {
				rel := byFamily[sub]
				if rel == 0 {
					rel = analogBaseRel
				}
				addFamily(sub, rel, domain.GroupAnalog)
			}
		}
		if !contains(ladder, ladderAnalogs) {
			ladder = append(ladder, ladderAnalogs)
		}
	default:
		group := ""
		if absent {
			group = domain.GroupAnalog
		}
		for _, f := range lx.familyOrder {
			rel, ok := byFamily[f]
			if !ok {
				continue
			}
			if rel < relThreshold {
				for _, pid := range lx.families[f] {
					if _, retrieved := fusedByID[pid]; retrieved {
						addRejected(&trace.Rejected, pid, rejectThreshold, fmt.Sprintf("релевантность %.2f < %.2f", rel, relThreshold))
					}
				}
				continue
			}
			addFamily(f, rel, group)
		}
	}
	fams = rc.dropQuantityMisfits(fams, &trace.Rejected)
	fams = rc.dropOtherCategories(fams, &trace.Rejected)
	for _, fh := range fams {
		rc.assignGroup(fh)
		rc.score(fh)
	}
	sortFamilies(fams)
	if absent && len(fams) > 5 {
		fams = fams[:5]
	}

	// Лестница: пусто, а семейство названо — аналоги из справочника.
	if len(fams) == 0 && !absent && !noLetters && len(ent.families) > 0 && !attrs.analog {
		for _, f := range ent.families {
			for _, sub := range lx.substitutes[f] {
				addFamily(sub, analogBaseRel, domain.GroupAnalog)
			}
		}
		for _, fh := range fams {
			rc.score(fh)
		}
		sortFamilies(fams)
		if len(fams) > 0 {
			ladder = append(ladder, ladderAnalogs)
		}
	}
	resp.Relaxed = relaxedMessages(rc, fams)
	if len(resp.Relaxed) > 0 {
		ladder = append(ladder, ladderRelax)
	}

	exact := false
	for _, fh := range fams {
		exact = exact || fh.group != domain.GroupAnalog
	}
	switch {
	case ent.nonProduct:
		resp.Empty = &out.Empty{Message: fmt.Sprintf("По запросу «%s» товаров нет — с этим вопросом поможет менеджер", req.Query), ShowManager: true}
	case absent || len(fams) == 0 && !attrs.analog || !exact && len(fams) > 0 && !attrs.analog:
		name := req.Query
		if len(ent.unknown) > 0 {
			name = strings.Join(ent.unknown, " ")
		}
		resp.Empty = &out.Empty{Message: fmt.Sprintf("Товара «%s» нет в каталоге", name), ShowManager: true}
	case attrs.analog && len(fams) == 0:
		resp.Empty = &out.Empty{Message: fmt.Sprintf("Аналогов по запросу «%s» в каталоге нет", req.Query), ShowManager: true}
	}
	if resp.Empty != nil {
		ladder = append(ladder, ladderEmpty)
	}

	resp.Total = len(fams)
	if len(fams) > limit {
		fams = fams[:limit]
	}
	for _, fh := range fams {
		it := rc.toItem(fh)
		resp.Items = append(resp.Items, it)
		addTrace := func(pid string) {
			f := fusedByID[pid]
			trace.Items = append(trace.Items, domain.TraceItem{ProductID: pid, Final: fh.final, Group: fh.group,
				Parts: fh.parts, BM25Rank: f.BM25Rank, DenseRank: f.DenseRank})
		}
		addTrace(it.ProductID)
		for _, v := range it.Variants {
			addTrace(v.ProductID)
		}
	}
	trace.Rejected = dropShown(trace.Rejected, trace.Items)
	businessStep := domain.BusinessStep{StepBase: domain.StepBase{Name: domain.StepBusiness, Ms: msSince(t)}, Weights: businessWeights}
	relaxStep := domain.RelaxationStep{StepBase: domain.StepBase{Name: domain.StepRelaxation}, Ladder: ladder}
	rerankStep := skipped(domain.StepRerank, "disabled")
	resp.RelatedQueries = s.relatedQueries(ctx, lx, resp)
	if len(resp.Items) > 0 {
		resp.Complements = complementsOf(lx, resp.Items[0].FamilyID, map[string]bool{resp.Items[0].FamilyID: true})
	}

	trace.Steps = []any{normStep, attrStep, routeStep, intentStep, bm.step, dn.step, fusionStep, rerankStep, businessStep, relaxStep}
	trace.TotalMs = msSince(started)
	resp.Trace = trace
	if !opts.NoLog {
		s.saveSearch(req, resp.Normalized, resp)
	}
	return resp, nil
}

// rankContext — контекст клиента для ранжирования.
func (s *Services) rankContext(st *catalogState, lx *searchLexicon, clientID string, attrs queryAttrs, ent queryEntities) rankCtx {
	rc := rankCtx{lx: lx, attrs: attrs, ent: ent, purchased: map[string]bool{}, purchasedQty: map[string]int{},
		boughtVendors: map[string]bool{}, productRel: map[string]float64{}, refCats: map[string]bool{}}
	for _, f := range ent.reference {
		rc.refCats[lx.familyCat[f]] = true
	}
	if clientID == "" {
		return rc
	}
	for k := range st.purchased[clientID] {
		rc.purchased[k] = true
		if strings.HasPrefix(k, "p:") {
			if d, ok := lx.docs[k[2:]]; ok {
				rc.boughtVendors[d.VendorID] = true
				if d.LicenseQuantity != nil {
					rc.purchasedQty[d.FamilyID] = *d.LicenseQuantity
				}
			}
		}
	}
	return rc
}

// relaxedMessages — мягкие условия, которым не соответствует ни одна позиция выдачи: покупателю говорится,
// что именно ослаблено. Количество — обязательное условие: оно не снимается, но если точного количества нет
// ни у одной позиции, это тоже сказано.
func relaxedMessages(rc rankCtx, fams []*familyHit) []out.Relaxed {
	res := []out.Relaxed{}
	if len(fams) == 0 {
		return res
	}
	a := rc.attrs
	some := func(pred func(indexDoc) bool) bool {
		for _, fh := range fams {
			for _, d := range fh.heads {
				if pred(d) {
					return true
				}
			}
		}
		return false
	}
	if a.licType == domain.LicensePerpetual && !some(func(d indexDoc) bool { return d.LicenseType == domain.LicensePerpetual }) {
		res = append(res, out.Relaxed{Field: domain.FieldLicenseType, Message: "Бессрочной лицензии нет, показаны подписки"})
	}
	if a.licType == domain.LicenseSubscription && !some(func(d indexDoc) bool { return d.LicenseType == domain.LicenseSubscription }) {
		res = append(res, out.Relaxed{Field: domain.FieldLicenseType, Message: "Подписки нет, показаны другие типы лицензий"})
	}
	if a.duration > 0 && !some(func(d indexDoc) bool { return d.LicenseDuration != nil && *d.LicenseDuration == a.duration }) {
		res = append(res, out.Relaxed{Field: domain.FieldLicenseDuration,
			Message: fmt.Sprintf("Лицензии на %s нет, показаны другие сроки", duration(a.duration))})
	}
	if a.quantity > 0 && !some(func(d indexDoc) bool { return quantityOK(d, a.quantity) }) {
		res = append(res, out.Relaxed{Field: domain.FieldLicenseQuantity,
			Message: fmt.Sprintf("Лицензий ровно на %s нет, показаны другие количества", quantityLabel(a.quantity, a.metric))})
	}
	return res
}

// removedSet — снятые чипы: поле → значения.
func removedSet(rs []in.RemovedChip) map[string]map[string]bool {
	m := map[string]map[string]bool{}
	for _, r := range rs {
		if m[r.Field] == nil {
			m[r.Field] = map[string]bool{}
		}
		m[r.Field][r.Value] = true
	}
	return m
}

func hasLetters(s string) bool {
	n := 0
	for _, r := range s {
		if r >= 'a' && r <= 'z' || r >= 'а' && r <= 'я' {
			n++
		}
	}
	return n >= 2
}

// segment — сегмент клиента для разбора намерения.
func (st *catalogState) segment(clientID string) string {
	for _, c := range st.clients {
		if c.ID == clientID {
			return c.Segment
		}
	}
	return ""
}

// dropShown — отказ по позиции, которая всё же показана (вариантом семейства или аналогом), из трассы убирается.
func dropShown(rejected []domain.RejectedHit, shown []domain.TraceItem) []domain.RejectedHit {
	in := map[string]bool{}
	for _, it := range shown {
		in[it.ProductID] = true
	}
	out := rejected[:0]
	for _, r := range rejected {
		if !in[r.ProductID] {
			out = append(out, r)
		}
	}
	return out
}

// strictFilters — обязательные условия, которые проверяются уже при отборе кандидатов: продление, отечественное
// ПО, названный вендор (кроме запроса аналога). Метрика проверяется в коде: у поштучных позиций она мягкая.
func strictFilters(attrs queryAttrs, ent queryEntities) []any {
	var f []any
	if attrs.renewal {
		f = append(f, map[string]any{"term": map[string]any{"license_type": domain.LicenseRenewal}})
	}
	if attrs.origin == "ru" {
		f = append(f, map[string]any{"term": map[string]any{"origin": "ru"}})
	}
	if len(ent.vendors) > 0 && !attrs.analog {
		f = append(f, map[string]any{"terms": map[string]any{"vendor_id": ent.vendors}})
	}
	return f
}

// maxComplements — сколько сопутствующих семейств показывать в блоке «С этим покупают».
const maxComplements = 3

// complementsOf — «С этим покупают» для семейства: связи complement из справочника, кроме семейств shown;
// от каждого семейства — самая популярная позиция. Семейства ниже в выдаче не исключаются: блок стоит рядом с
// первой позицией, и CAL к Windows Server нужен, даже если он есть в списке пятым.
func complementsOf(lx *searchLexicon, familyID string, shown map[string]bool) []out.Complement {
	res := []out.Complement{}
	for _, l := range lx.complements[familyID] {
		if len(res) == maxComplements {
			break
		}
		if shown[l.Family] {
			continue
		}
		var best *indexDoc
		for _, pid := range lx.families[l.Family] {
			d := lx.docs[pid]
			if best == nil || d.Popularity > best.Popularity {
				best = &d
			}
		}
		if best == nil {
			continue
		}
		res = append(res, out.Complement{
			FamilyID: l.Family, FamilyName: best.FamilyName, ProductID: best.ProductID, Title: best.Title,
			Vendor: best.Vendor, Category: best.Category, LicenseLine: licenseLine(*best), PriceRub: best.PriceRub,
			PriceOnRequest: best.PriceOnRequest, Note: l.Note,
		})
	}
	return res
}
