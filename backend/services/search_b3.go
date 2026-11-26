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
	effective := cfg
	if cfg == domain.ConfigLive {
		// Витрина: на правилах версии 1 это B3, с принятыми правками агента — B4.
		effective = domain.ConfigB3
		if rules.version > 1 {
			effective = domain.ConfigB4
		}
	}
	trace := &domain.Trace{
		RequestID: requestID, Route: domain.RouteNormalized, ParseSource: domain.ParseRules, Config: cfg,
		EffectiveConfig: effective, RulesVersion: rules.version, CatalogVersion: st.version,
		Items: []domain.TraceItem{}, Rejected: []domain.RejectedHit{},
	}
	resp := &out.SearchResponse{
		RequestID: requestID, Query: req.Query, Config: cfg,
		Chips: []out.Chip{}, Items: []out.SearchItem{}, Relaxed: []out.Relaxed{}, RelatedQueries: []string{},
		Complements: []out.Complement{}, Facets: []out.Facet{},
	}

	// normalize
	t := time.Now()
	nr := normalizeFull(req.Query, lx, rules, req.AsTyped)
	nr.restoreSKUs(lx)
	resp.Normalized = nr.normalized()
	if c := nr.correction; c != nil {
		resp.Correction = &out.Correction{From: c.from, To: c.to, Confidence: c.confidence, Applied: c.applied}
	}
	normStep := domain.NormalizeStep{StepBase: domain.StepBase{Name: domain.StepNormalize, Ms: msSince(t)}, Changes: nr.changes}

	// attributes + сущности
	t = time.Now()
	attrs := extractAttrs(nr.tokens)
	attrs.applyRequest(req)
	rp := s.newResultsPlan(ctx, req)                              // страница результатов контура выдачи (results.go); флаг выключен — nil
	hw := s.hardwareParse(ctx, st, nr, &attrs, rp.hwRequest(req)) // оборудование (hardware*.go); флаг выключен — nil
	ent := findEntities(nr, attrs, lx, rules)
	ent.applyRemoved(removedSet(req.RemovedChips))
	hw.applyEntities(&ent)
	chips, found := attrs.chips(ent, lx)
	chips, found = hw.chips(chips, found)
	attrs.found = found
	attrStep := domain.AttributesStep{StepBase: domain.StepBase{Name: domain.StepAttributes, Ms: msSince(t)},
		Found: found, Unresolved: attrs.unresolved()}

	// route
	t = time.Now()
	route, signals := routeSignals(nr, attrs, ent)
	route, signals = hw.route(route, signals, ent)
	routeStep := domain.RouteStep{StepBase: domain.StepBase{Name: domain.StepRoute}, Route: route, Signals: signals}
	routeStep.Ms = msSince(t)

	// intent: только маршрут D и только если разборщик подключён
	var intentStep any = skipped(domain.StepIntent, "not_route_d")
	if ent.noise {
		intentStep = skipped(domain.StepIntent, rejectNoise)
	}
	if route == domain.RouteIntent {
		intentStep = skipped(domain.StepIntent, "parser_not_connected")
		if isSlotSearch(ctx) {
			intentStep = skipped(domain.StepIntent, solutionNested) // слот решения: модель в момент запроса не зовётся
		} else if p := s.intentParser(); p != nil {
			t = time.Now()
			// Разбор — по запросу как набран, без снятых чипов и ответа на уточнение: ключ кэша от них не
			// зависит, и пересчёт после снятия чипа идёт из кэша за 0 ₽. Снятое покупателем
			// применяется уже к итогу разбора — иначе модель возвращает условие обратно.
			rawAttrs := extractAttrs(nr.tokens)
			guess := rulesGuess(rawAttrs, findEntities(nr, rawAttrs, lx, rules))
			parsed, err := p.Parse(WithRequestID(ctx, requestID), resp.Normalized, guess, st.segment(req.ClientID))
			if err != nil {
				parsed = guess
			}
			mergeParsed(parsed, &attrs, &ent, lx)
			for _, d := range parsed.Dropped {
				if d.Field == "reference_product" && d.Reason == "not_in_catalog" {
					ent.absentName = d.Value
				}
			}
			attrs.applyRequest(req)
			ent.applyRemoved(removedSet(req.RemovedChips))
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
	if ent.categoryByWords && trace.Route == domain.RouteIntent {
		// Задача названа словами («созваниваться», «чертежи»): название категории — расширение текстового
		// поиска и смысла запроса, иначе длинная формулировка находит офисные пакеты по «50» и «российская».
		expansions = strings.TrimSpace(expansions + " " + strings.Join(ent.purposeWords, " "))
	}

	denseCh := make(chan retrieval, 1)
	filters := strictFilters(attrs, ent)
	filters = append(filters, hw.filters()...)
	go func() { denseCh <- s.denseSafe(ctx, denseText, info, filters...) }()
	bm := s.lexicalB3(ctx, text, expansions, b3MinMatch, filters)
	if bm.err == nil && len(bm.step.Top) == 0 && strings.Contains(text, " ") {
		// Все слова вместе не встретились ни в одной позиции — ослабляем до «любое слово».
		bm = s.lexicalB3(ctx, text, expansions, "1", filters)
		bm.step.Mode = "multi_match_any"
	}
	s.exactRetrieve(ctx, hw, filters)
	dn := <-denseCh
	if bm.err != nil {
		if opensearch.IsNotFound(bm.err) {
			s.index.Store(nil)
			return nil, errs.NewUnavailableError(MsgNoIndex).WithCause(bm.err)
		}
		return nil, bm.err
	}
	if len(bm.step.Top) == 0 && len(hw.exactHitsOrNil()) == 0 && route != domain.RouteIntent && !ent.noise && !hw.holdsRoute() {
		trace.Route = domain.RouteIntent
		routeStep.Route = domain.RouteIntent
		routeStep.Signals = append(routeStep.Signals, "zero_results")
	}

	// fusion
	t = time.Now()
	// Варианты под сделку или организацию, которых запрос не называет, в слияние не идут (purchase_attrs.go):
	// иначе они занимают места списков и сдвигают ранги остальных позиций.
	bmTop, dnTop := withoutHiddenOffers(lx, attrs, bm.step.Top), withoutHiddenOffers(lx, attrs, dn.step.Top)
	ranked := rrf(b3RRFK, bmTop, dnTop)
	ranked = hw.fuse(ranked, b3RRFK)
	fusionStep := domain.FusionStep{StepBase: domain.StepBase{Name: domain.StepFusion, Ms: msSince(t)}, K: b3RRFK}
	denseScore := map[string]float64{}
	for _, h := range dn.step.Top {
		denseScore[h.ProductID] = h.Score
	}

	// business: допустимость → порог → группы → скор
	t = time.Now()
	rc := s.rankContext(st, lx, req.ClientID, attrs, ent)
	rc.hw = hw
	rc.upgradeTargets = upgradeTargetSet(s.commerceFor(ctx))
	cq := s.companionQuantity(ctx, lx, attrs, ent) // «на 50 пользователей» к серверу — количество CAL (search_companion_qty.go)
	cq.apply(&rc)
	for _, h := range bmTop {
		if h.Rank <= textTopN {
			rc.textTop[lx.docs[h.ProductID].FamilyID] = true
		}
	}
	absent := ent.absent()
	minDense := denseOnlyMin
	if ent.abbrev {
		minDense = abbrevDenseMin
	}
	noLetters := ent.noise
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
		if hidden, _ := rc.offerHidden(d); hidden {
			continue // вариант под сделку или организацию, которых запрос не называет: семейство он не поднимает
		}
		if absent && denseScore[f.ProductID] < absentDenseMin {
			addRejected(&trace.Rejected, f.ProductID, rejectAbsentDense,
				fmt.Sprintf("товара нет в каталоге, аналог — только близкий по смыслу: скор %.3f < %.3f", denseScore[f.ProductID], absentDenseMin))
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
		// Шум и вопросы не о товаре: ни товаров, ни аналогов. Найденное поиском — в отказы с причиной.
		for _, f := range ranked {
			addRejected(&trace.Rejected, f.ProductID, rejectNoise, "запрос без смысла или не о товаре — товары не показываются")
		}
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
	// Метрика не та у всего названного («касперский на 50 пользователей», а Kaspersky — на устройства): честнее
	// показать названный продукт с пояснением, чем «нет в каталоге» (прогон витрины №22).
	metricRelaxed := ""
	if len(fams) == 0 && rc.attrs.metric != "" && !rc.attrs.analog && !noLetters && !ent.nonProduct && !absent &&
		(len(ent.vendors) > 0 || len(ent.families) > 0) {
		metricRelaxed = rc.attrs.metric
		rc.attrs.metricPref, rc.attrs.metric = rc.attrs.metric, ""
		seen = map[string]bool{}
		for _, f := range lx.familyOrder {
			if rel, ok := byFamily[f]; ok && rel >= relThreshold {
				addFamily(f, rel, "")
			}
		}
	}
	// Категория раньше количества: «видеоконференции на 50» — TrueConf (до 25) остаётся, офисный пакет на 50 — нет.
	fams = rc.dropOtherCategories(fams, &trace.Rejected)
	fams = rc.dropQuantityMisfits(fams, &trace.Rejected)
	rerankStep := skipped(domain.StepRerank, rerankAnalogs)
	if !(attrs.analog && len(ent.reference) > 0) {
		rerankStep = s.rerankFamilies(ctx, resp.Normalized, trace.Route, fams)
	}
	for _, fh := range fams {
		rc.assignGroup(fh)
		rc.score(fh)
	}
	cq.regroup(fams)
	sortFamilies(fams)
	fams = rc.dropTail(fams, &trace.Rejected)
	if absent && len(fams) > 5 {
		for _, fh := range fams[5:] {
			addRejected(&trace.Rejected, fh.head.ProductID, rejectAbsentTail, "товара нет в каталоге — показаны пять ближайших аналогов")
		}
		fams = fams[:5]
	}

	// Лестница: пусто, а семейство названо — аналоги из справочника.
	// Аналоги только для осмысленного запроса: есть хоть одно значимое слово, известное каталогу.
	if len(fams) == 0 && !absent && !noLetters && len(ent.families) > 0 && !attrs.analog && ent.significant > len(ent.unknown) {
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
	// Товара нет в каталоге, близкого по смыслу тоже («solidworks»): аналоги — по связям substitute от
	// семейств, которые названы вендором и категорией запроса (их называют правила или разбор маршрута D:
	// SolidWorks → САПР, ближайший вендор каталога — Autodesk → AutoCAD → КОМПАС-3D, nanoCAD). Вектор здесь не
	// решает: ближайшие по смыслу соседи у названий, которых каталог не знает, случайны.
	// Не только absent(): категория из разбора («солидворкс» → САПР) снимает признак отсутствия, а товар всё
	// равно не найден — непонятное слово осталось.
	if len(fams) == 0 && len(ent.unknown) > 0 && !ent.abbrev && !noLetters && !ent.nonProduct && !attrs.analog {
		if subs := absentSubstitutes(lx, ent); len(subs) > 0 {
			ra := rc
			ra.ent.vendors = nil // вендор — признак ближайшего продукта, а не условие для аналога
			for _, sub := range subs {
				if seen[sub] {
					continue
				}
				seen[sub] = true
				if fh := ra.buildFamily(sub, analogBaseRel, &trace.Rejected); fh != nil {
					fh.group = domain.GroupAnalog
					fams = append(fams, fh)
				}
			}
			// Порядок аналогов — по реранкеру: «solidworks» ближе к КОМПАС-3D (3D-САПР), чем к nanoCAD (DWG).
			if st := s.rerankFamilies(ctx, resp.Normalized, trace.Route, fams); !st.Skipped || rerankStep.Skipped {
				rerankStep = st
			}
			for _, fh := range fams {
				ra.score(fh)
			}
			sortFamilies(fams)
			if len(fams) > 0 && !contains(ladder, ladderAnalogs) {
				ladder = append(ladder, ladderAnalogs)
			}
		}
	}
	resp.Relaxed = relaxedMessages(rc, fams)
	if m := metricRelaxedMessage(metricRelaxed, rc.attrs.quantity, fams); m != nil {
		resp.Relaxed = append(resp.Relaxed, *m)
	}
	if len(resp.Relaxed) > 0 {
		ladder = append(ladder, ladderRelax)
	}

	exact := false
	for _, fh := range fams {
		exact = exact || fh.group != domain.GroupAnalog
	}
	switch {
	case ent.noise:
		resp.Empty = &out.Empty{Message: fmt.Sprintf("По запросу «%s» товаров нет — назовите программу или задачу", req.Query), ShowManager: true}
	case ent.nonProduct:
		resp.Empty = &out.Empty{Message: fmt.Sprintf("По запросу «%s» товаров нет — с этим вопросом поможет менеджер", req.Query), ShowManager: true}
	case absent || len(fams) == 0 && !attrs.analog || !exact && len(fams) > 0 && !attrs.analog:
		name := req.Query
		if len(ent.unknown) > 0 {
			name = strings.Join(ent.unknown, " ")
		}
		resp.Empty = &out.Empty{Message: fmt.Sprintf("Товара «%s» нет в каталоге", name), ShowManager: true, Supply: true}
	case attrs.analog && len(fams) == 0:
		resp.Empty = &out.Empty{Message: fmt.Sprintf("Аналогов по запросу «%s» в каталоге нет", req.Query), ShowManager: true}
	}
	if resp.Empty != nil {
		ladder = append(ladder, ladderEmpty)
	}

	rp.setExact(trace.Route, routeStep.Signals, hw)
	fams = rp.apply(rc, fams, resp.Chips, req.Query, &trace.Rejected)
	var marginStep *domain.MarginStep
	if rp.marginAllowed() {
		fams, marginStep = s.marginStage(ctx, cfg, rc, trace.Route, fams, opts)
	}
	rp.keep(fams)

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
				Parts: fh.parts, BM25Rank: f.BM25Rank, DenseRank: f.DenseRank, Signals: fh.signals})
		}
		addTrace(it.ProductID)
		for _, v := range it.Variants {
			addTrace(v.ProductID)
		}
	}
	trace.Rejected = dropShown(trace.Rejected, trace.Items)
	businessStep := domain.BusinessStep{StepBase: domain.StepBase{Name: domain.StepBusiness, Ms: msSince(t)}, Weights: businessWeights}
	relaxStep := domain.RelaxationStep{StepBase: domain.StepBase{Name: domain.StepRelaxation}, Ladder: ladder}
	resp.RelatedQueries = s.relatedQueries(ctx, lx, resp)
	if len(resp.Items) > 0 {
		resp.Complements = complementsOf(lx, resp.Items[0].FamilyID, map[string]bool{resp.Items[0].FamilyID: true})
	}

	// Допродажа: после выдачи, вне BM25/векторов; флаг выключен — шага и полей нет (companions.go).
	var companionsStep any
	if s.featuresFor(ctx).Companions {
		companionsStep = s.companionsStep(s.commerceFor(ctx), st.lex, fams, trace.Route, companionQuery{quantity: attrs.quantity, metric: attrs.metric, tokens: nr.tokens},
			attrs.analog, resp)
	}

	// Закупка под задачу и старшая редакция (solutions.go, upgrade.go): отдельные поля ответа, выдача та же.
	upgradeStep := s.upgradeStep(ctx, st.lex, attrs.analog, resp)
	solStep := s.solutionStep(ctx, st, solutionInput{req: req, cfg: cfg, seats: 0, origin: attrs.origin,
		vendors: hw.excludedVendors()}.withSeats(req.Query, attrs.quantity, hw.purchaseQuantity()), resp)

	trace.Steps = []any{normStep, attrStep, routeStep, intentStep, bm.step, dn.step, fusionStep, rerankStep, businessStep, relaxStep}
	hw.decorate(resp)
	rp.decorate(rc, resp, limit, exactDocMatcher(nr.tokens, hw))
	procStep := s.procurementStage(st.lex, req.Query, nr.tokens, attrs, ent, fams, resp) // контур доводки: purchase_stage.go
	if hwStep := hw.step(); hwStep != nil {
		trace.Steps = append(trace.Steps, hwStep)
	}
	if companionsStep != nil {
		trace.Steps = append(trace.Steps, companionsStep)
	}
	if upgradeStep != nil {
		trace.Steps = append(trace.Steps, upgradeStep)
	}
	if solStep != nil {
		trace.Steps = append(trace.Steps, solStep)
	}
	if marginStep != nil {
		trace.Steps = append(trace.Steps, marginStep)
	}
	trace.Steps = append(trace.Steps, procStep)
	trace.TotalMs = msSince(started)
	resp.Trace = trace
	if !opts.NoLog {
		s.saveSearch(req, resp.Normalized, resp)
	}
	stripMarginStep(resp.Trace)
	return resp, nil
}

// rankContext — контекст клиента для ранжирования.
func (s *Services) rankContext(st *catalogState, lx *searchLexicon, clientID string, attrs queryAttrs, ent queryEntities) rankCtx {
	rc := rankCtx{lx: lx, attrs: attrs, ent: ent, purchased: map[string]bool{}, purchasedQty: map[string]int{},
		boughtVendors: map[string]bool{}, productRel: map[string]float64{}, refCats: map[string]bool{},
		business: isBusinessQuery(attrs, ent, st.segment(clientID)), textTop: map[string]bool{}}
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
			Message: fmt.Sprintf("Позиций ровно на %s нет, показаны другие количества", quantityLabel(a.quantity, a.metric))})
	}
	// Поштучные позиции с другой метрикой допускаются (rejectReason, dropQuantityMisfits) — покупателю об этом
	// говорится прямо, а не молча.
	if forms, ok := metricForms[a.metric]; ok {
		for _, fh := range fams {
			d := fh.head
			if d.LicenseMetric != nil && *d.LicenseMetric != a.metric && isPerUnit(d) {
				res = append(res, out.Relaxed{Field: domain.FieldLicenseMetric,
					Message: fmt.Sprintf("Часть позиций лицензируется не на %s, а продаётся поштучно: число мест указывается при заказе", forms[1])})
				break
			}
		}
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

// looksLikeMarkup — в запросе разметка или код («<script>…</script>», «{…}»): это не поиск товара.
func looksLikeMarkup(q string) bool {
	return strings.ContainsAny(q, "<>{}")
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

// absentSubstitutes — аналоги отсутствующего товара по связям substitute (в их направлении: от продукта к
// замене): от семейств вендора и категории запроса (одного вендора; категория — та же или её раздел). Вендор
// здесь — ближайший похожий продукт каталога, а не производитель: если покупатель или разбор назвали
// отсутствующий товар продуктом этого вендора («Autodesk Revit»), замены AutoCAD ему не аналоги — ничего.
// Без вендора или категории — тоже ничего: неясно, на что товар похож, и честнее «нет в каталоге».
func absentSubstitutes(lx *searchLexicon, ent queryEntities) []string {
	if len(ent.vendors) != 1 || ent.category == "" || len(ent.unknown) == 0 || len(ent.vendorTokens) > 0 {
		return nil
	}
	v := ent.vendors[0]
	if name := foldText(lx.vendorName[v]); name != "" && strings.Contains(foldText(ent.absentName), name) {
		return nil
	}
	root := lx.categoryRoot[ent.category]
	var out []string
	seen := map[string]bool{}
	for _, f := range lx.familyOrder {
		if lx.familyVendor[f] != v {
			continue
		}
		if c := lx.familyCat[f]; c != ent.category && (root == "" || lx.categoryRoot[c] != root) {
			continue
		}
		for _, sub := range lx.substituteOf[f] {
			if !seen[sub] && lx.familyVendor[sub] != v {
				seen[sub] = true
				out = append(out, sub)
			}
		}
	}
	return out
}

// changeSKU — артикул, набранный без дефиса или слитно («fqc10572»), восстановлен по каталогу («fqc-10572»).
const changeSKU = "sku"

// restoreSKUs — токен, который без разделителей и регистра совпадает ровно с одним артикулом каталога, заменяется
// артикулом как в каталоге: дальше его находит точное поле sku и маршрут A. Набранное как есть (as_typed) — тоже:
// это не исправление слова, а другой вид записи того же кода.
func (n *normResult) restoreSKUs(lx *searchLexicon) {
	if lx == nil || len(lx.skuCompact) == 0 {
		return
	}
	for i, t := range n.tokens {
		if !hasDigit(t) {
			continue
		}
		want, ok := lx.skuCompact[strings.ToLower(normalizeMPN(t))]
		if !ok || want == t {
			continue
		}
		n.changes = append(n.changes, domain.Change{Kind: changeSKU, From: t, To: want})
		n.tokens[i] = want
		n.protected[want] = true
	}
}
