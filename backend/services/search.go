package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"runtime/debug"
	"strings"
	"time"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Поиск: B0 — подстрока, BM25 — BM25 по запросу как набран (только оценка), B1 — BM25 по нормализованному запросу, B2 — BM25 + kNN + RRF (группировка по
// семействам без атрибутов и порога); B3/B4 — ядро поиска целиком (search_b3.go). Ответ и трасса —

const (
	defaultLimit    = 20
	maxLimit        = 50
	maxQueryRunes   = 300
	maxIDLen        = 128
	queryEmbedLimit = 3 * time.Second  // ожидание вектора запроса: дольше — dense пропускается
	searchLimit     = 10 * time.Second // весь поиск; OpenSearch молчит дольше — 504
	saveTimeout     = 3 * time.Second
)

// Причины пропуска шага (StepBase.Reason).
const (
	reasonNotInConfig      = "not_in_config"
	reasonNoCatalog        = "catalog_not_loaded"
	reasonEmbedderDisabled = "embedder_disabled"
	reasonEmbedderDown     = "embedder_unavailable"
	reasonNoVectors        = "index_without_vectors"
	reasonKNNFailed        = "knn_failed"
)

// Тексты отказов поиска.
const (
	MsgNoIndex = "Каталог ещё не загружен в поиск — выполните импорт (make import)"
)

// retrieval — результат одного источника кандидатов.
type retrieval struct {
	step domain.RetrievalStep
	docs map[string]indexDoc
	err  error
}

// Search — POST /api/search. requestID — номер HTTP-запроса: он же request_id ответа, трассы и события.
func (s *Services) Search(ctx context.Context, requestID string, req in.SearchRequest) (*out.SearchResponse, error) {
	return s.SearchWith(ctx, requestID, req, SearchOptions{})
}

// SearchWith — поиск с внутренними параметрами: снимок правил песочницы, без записи в журнал (оценка).
func (s *Services) SearchWith(ctx context.Context, requestID string, req in.SearchRequest, opts SearchOptions) (*out.SearchResponse, error) {
	started := time.Now()
	cfg, limit, err := validateSearch(&req)
	if err != nil {
		return nil, err
	}
	if !s.search.Enabled() {
		return nil, errs.NewUnavailableError(MsgSearchDisabled)
	}
	ctx, cancel := context.WithTimeout(ctx, searchLimit)
	defer cancel()
	st := s.catalog.Load()
	effective := cfg
	if cfg == domain.ConfigB3 || cfg == domain.ConfigB4 || cfg == domain.ConfigLive {
		if st != nil && st.lex != nil {
			return s.searchB3(ctx, requestID, req, cfg, limit, st, opts, started)
		}
		effective = domain.ConfigB2 // справочники не загружены: без словаря каталога — как B2
	}
	trace := &domain.Trace{
		RequestID: requestID, Route: domain.RouteExact, ParseSource: domain.ParseRules, Config: cfg,
		EffectiveConfig: effective, RulesVersion: 1, Items: []domain.TraceItem{}, Rejected: []domain.RejectedHit{},
	}
	if st != nil {
		trace.RulesVersion, trace.CatalogVersion = st.rulesVersion, st.version
	}

	// normalize
	t := time.Now()
	normalized, changes := normalizeQuery(req.Query)
	text := normalized
	// BM25 (оценка): запрос как набран — только регистр и пробелы, без раскладки, опечаток и словарей.
	if effective != domain.ConfigB0 && effective != domain.ConfigBM25 && st != nil && st.lex != nil {
		// B1/B2: нормализация и словари версии 1 (раскладка, опечатки, алиасы), без атрибутов и порога.
		rules := s.rulesFor(ctx, domain.ConfigB1, st.lex, opts.Rules)
		nr := normalizeFull(req.Query, st.lex, rules, req.AsTyped)
		normalized, changes = nr.normalized(), nr.changes
		text = strings.TrimSpace(normalized + " " + strings.Join(nr.expansions, " "))
	}
	normStep := domain.NormalizeStep{StepBase: domain.StepBase{Name: domain.StepNormalize}, Changes: changes}
	normStep.Ms = msSince(t)

	// Шаги следующих волн: в B0–B2 их нет по определению конфигурации, в B3/B4 они ещё не написаны.
	later := reasonNotInConfig
	if effective != cfg {
		later = reasonNoCatalog
	}
	attrStep := domain.AttributesStep{StepBase: skipped(domain.StepAttributes, later), Found: []domain.FoundAttribute{}, Unresolved: []string{}}
	routeStep := domain.RouteStep{StepBase: skipped(domain.StepRoute, later), Route: domain.RouteExact, Signals: []string{}}
	intentStep := skipped(domain.StepIntent, later)

	info, err := s.indexInfo(ctx)
	if err != nil {
		if opensearch.IsNotFound(err) {
			return nil, errs.NewUnavailableError(MsgNoIndex).WithCause(err)
		}
		return nil, err
	}

	// Кандидаты: dense считается параллельно с BM25 — вектор запроса и kNN не ждут текстового поиска.
	var denseCh chan retrieval
	if effective == domain.ConfigB2 {
		denseCh = make(chan retrieval, 1)
		go func() { denseCh <- s.denseSafe(ctx, text, info) }()
	}
	bm := s.lexical(ctx, effective, text)
	if bm.err != nil {
		if denseCh != nil {
			<-denseCh
		}
		if opensearch.IsNotFound(bm.err) {
			s.index.Store(nil)
			return nil, errs.NewUnavailableError(MsgNoIndex).WithCause(bm.err)
		}
		return nil, bm.err
	}
	dn := retrieval{step: domain.RetrievalStep{StepBase: skipped(domain.StepDense, reasonNotInConfig), Top: []domain.RankedHit{}}}
	if denseCh != nil {
		dn = <-denseCh
	}

	// fusion
	fusionStep := domain.FusionStep{StepBase: skipped(domain.StepFusion, reasonNotInConfig), K: rrfK}
	var ranked []fused
	t = time.Now()
	switch effective {
	case domain.ConfigB2:
		ranked = rrf(rrfK, bm.step.Top, dn.step.Top)
		fusionStep.StepBase = domain.StepBase{Name: domain.StepFusion, Ms: msSince(t)}
	default:
		ranked = lexicalRanking(effective, bm.step.Top)
	}
	docs := bm.docs
	for id, d := range dn.docs {
		if _, ok := docs[id]; !ok {
			docs[id] = d
		}
	}

	rerankStep := skipped(domain.StepRerank, reasonNotInConfig)
	businessStep := domain.BusinessStep{StepBase: skipped(domain.StepBusiness, later), Weights: map[string]float64{
		"relevance": 1, "popularity": 0, "margin": 0, "context": 0,
	}}
	relaxStep := domain.RelaxationStep{StepBase: skipped(domain.StepRelaxation, later), Ladder: []string{}}

	resp := &out.SearchResponse{
		RequestID: requestID, Query: req.Query, Normalized: normalized, Config: cfg,
		Chips: []out.Chip{}, Items: []out.SearchItem{}, Relaxed: []out.Relaxed{}, RelatedQueries: []string{},
		Complements: []out.Complement{}, Facets: []out.Facet{},
	}
	resp.Items, resp.Total = groupByFamily(ranked, docs, normalized, effective, limit, func(d indexDoc) bool {
		return st.purchasedBefore(req.ClientID, d.ProductID, d.FamilyID)
	})
	trace.Items = traceItems(resp.Items, ranked)
	if len(resp.Items) == 0 {
		resp.Empty = &out.Empty{Message: fmt.Sprintf("Товара «%s» нет в каталоге", req.Query), ShowManager: true}
	}
	finishCards(resp) // поля карточки контура фида и счётчики; цели и лестницы в B0–B2 нет (search_ladder.go)

	trace.Steps = []any{normStep, attrStep, routeStep, intentStep, bm.step, dn.step, fusionStep, rerankStep, businessStep, relaxStep}
	trace.TotalMs = msSince(started)
	resp.Trace = trace
	if !opts.NoLog {
		s.saveSearch(req, normalized, resp)
	}
	return resp, nil
}

// validateSearch — проверка запроса; возвращает конфигурацию и лимит со значениями по умолчанию.
func validateSearch(req *in.SearchRequest) (string, int, error) {
	var bad []errs.FieldError
	req.Query = strings.TrimSpace(req.Query)
	switch {
	case req.Query == "":
		bad = append(bad, errs.FieldError{Field: "query", Code: errs.CodeRequired, Message: "Введите запрос"})
	case utf8.RuneCountInString(req.Query) > maxQueryRunes:
		bad = append(bad, errs.FieldError{Field: "query", Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Запрос длиннее %d символов", maxQueryRunes)})
	}
	cfg := strings.ToUpper(strings.TrimSpace(req.Config))
	switch cfg {
	case "":
		cfg = domain.DefaultSearchConfig
	case strings.ToUpper(domain.ConfigLive):
		cfg = domain.ConfigLive
	}
	known := cfg == domain.ConfigLive || cfg == domain.ConfigBM25
	for _, c := range domain.SearchConfigs {
		known = known || c == cfg
	}
	if !known {
		bad = append(bad, errs.FieldError{Field: "config", Code: errs.CodeInvalidFormat, Message: "Конфигурация — live или одна из B0–B4"})
	}
	limit := req.Limit
	switch {
	case limit == 0:
		limit = defaultLimit
	case limit < 0 || limit > maxLimit:
		bad = append(bad, errs.FieldError{Field: "limit", Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Лимит — от 1 до %d", maxLimit)})
	}
	for field, v := range map[string]string{"client_id": req.ClientID, "session_id": req.SessionID} {
		if len(v) > maxIDLen {
			bad = append(bad, errs.FieldError{Field: field, Code: errs.CodeOutOfRange, Message: "Слишком длинный идентификатор"})
		}
	}
	switch req.Source = strings.ToLower(strings.TrimSpace(req.Source)); req.Source {
	case "", domain.EventSourceLive, domain.EventSourceCompare, domain.EventSourceTraffic:
	default:
		bad = append(bad, errs.FieldError{Field: "source", Code: errs.CodeInvalidFormat, Message: "Источник — live, compare или traffic"})
	}
	bad = append(bad, validateFilters(req.Filters)...)
	if len(bad) > 0 {
		return "", 0, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	return cfg, limit, nil
}

// lexical — текстовый источник: подстрока (B0) или multi_match (B1, B2).
func (s *Services) lexical(ctx context.Context, cfg, normalized string) retrieval {
	t := time.Now()
	r := retrieval{step: domain.RetrievalStep{StepBase: domain.StepBase{Name: domain.StepBM25}, Mode: "multi_match"}}
	body := bm25Query(normalized, candidatesTop)
	if cfg == domain.ConfigB0 {
		r.step.Mode = "substring"
		body = substringQuery(normalized, candidatesTop)
	}
	res, err := s.search.Search(ctx, s.alias, body)
	if err != nil {
		r.err = err
		return r
	}
	r.step.Top, r.docs, r.err = parseHits(res)
	r.step.Ms = msSince(t)
	return r
}

// lexicalB3 — текстовый источник B3 (bm25QueryB3).
func (s *Services) lexicalB3(ctx context.Context, text, expansions, minMatch string, filters []any) retrieval {
	t := time.Now()
	r := retrieval{step: domain.RetrievalStep{StepBase: domain.StepBase{Name: domain.StepBM25}, Mode: "multi_match_msm"}}
	res, err := s.search.Search(ctx, s.alias, bm25QueryB3(text, expansions, minMatch, candidatesTop, filters))
	if err != nil {
		r.err = err
		return r
	}
	r.step.Top, r.docs, r.err = parseHits(res)
	r.step.Ms = msSince(t)
	return r
}

// lexicalIntent — текстовая ветка B3 при намерении по категории (bm25IntentQuery).
func (s *Services) lexicalIntent(ctx context.Context, ci *categoryIntent, text, expansions string, filters []any) retrieval {
	t := time.Now()
	r := retrieval{step: domain.RetrievalStep{StepBase: domain.StepBase{Name: domain.StepBM25}, Mode: "multi_match_category"}}
	res, err := s.search.Search(ctx, s.alias, bm25IntentQuery(text, expansions, b3MinMatch, ci.textOptional, candidatesTop, filters))
	if err != nil {
		r.err = err
		return r
	}
	r.step.Top, r.docs, r.err = parseHits(res)
	r.step.Ms = msSince(t)
	return r
}

// dense — векторный источник: вектор запроса у сервиса моделей, затем kNN. Сбой не роняет поиск: шаг
// помечается skipped с причиной, выдача остаётся на BM25.
// denseSafe — dense в отдельной горутине: паника там не ловится recover Fiber и уронила бы весь API (ревью-2 n4),
// поэтому она превращается в пропуск dense с записью в журнал.
func (s *Services) denseSafe(ctx context.Context, normalized string, info *indexMeta, filters ...any) (r retrieval) {
	defer func() {
		if p := recover(); p != nil {
			zap.L().Error("поиск: паника в dense — пропущен", zap.Any("panic", p), zap.ByteString("stack", debug.Stack()))
			r = retrieval{step: domain.RetrievalStep{StepBase: domain.StepBase{Name: domain.StepDense, Skipped: true,
				Reason: reasonKNNFailed}, Mode: "knn", Top: []domain.RankedHit{}}}
		}
	}()
	return s.dense(ctx, normalized, info, filters...)
}

func (s *Services) dense(ctx context.Context, normalized string, info *indexMeta, filters ...any) retrieval {
	t := time.Now()
	r := retrieval{step: domain.RetrievalStep{StepBase: domain.StepBase{Name: domain.StepDense}, Mode: "knn", Top: []domain.RankedHit{}}}
	skip := func(reason string) retrieval {
		r.step.Skipped, r.step.Reason, r.step.Ms = true, reason, msSince(t)
		return r
	}
	switch {
	case s.vectorsOff(ctx):
		return skip(reasonVectorsOff)
	case !s.embedder.Enabled():
		return skip(reasonEmbedderDisabled)
	case !info.HasVectors:
		return skip(reasonNoVectors)
	}
	ectx, cancel := context.WithTimeout(ctx, queryEmbedLimit)
	vec, err := s.queryVector(ectx, normalized)
	cancel()
	if err != nil {
		zap.L().Warn("поиск: вектор запроса не получен, dense пропущен", zap.Error(err))
		return skip(reasonEmbedderDown)
	}
	res, err := s.search.Search(ctx, s.alias, knnQuery(vec, candidatesTop, filters...))
	if err == nil {
		r.step.Top, r.docs, err = parseHits(res)
	}
	if err != nil {
		zap.L().Warn("поиск: kNN не выполнен, dense пропущен", zap.Error(err))
		r.step.Top, r.docs = []domain.RankedHit{}, nil
		return skip(reasonKNNFailed)
	}
	r.step.Ms = msSince(t)
	return r
}

// parseHits — ранжированный список и документы из ответа OpenSearch.
func parseHits(res *opensearch.SearchResult) ([]domain.RankedHit, map[string]indexDoc, error) {
	top := make([]domain.RankedHit, 0, len(res.Hits.Hits))
	docs := make(map[string]indexDoc, len(res.Hits.Hits))
	for i, h := range res.Hits.Hits {
		var d indexDoc
		if err := json.Unmarshal(h.Source, &d); err != nil {
			return nil, nil, errs.NewBadGatewayError(errs.MsgBadGateway).WithCause(err)
		}
		if d.ProductID == "" {
			d.ProductID = h.ID
		}
		docs[d.ProductID] = d
		top = append(top, domain.RankedHit{ProductID: d.ProductID, Rank: i + 1, Score: round4(h.Score)})
	}
	return top, docs, nil
}

// lexicalRanking — B0 и B1 без слияния: релевантность B0 — 1 (совпало целиком), B1 — скор BM25,
// нормированный на лучший в выдаче (сырой BM25 между запросами не сравним).
func lexicalRanking(cfg string, top []domain.RankedHit) []fused {
	out := make([]fused, 0, len(top))
	best := 0.0
	for _, h := range top {
		best = math.Max(best, h.Score)
	}
	for _, h := range top {
		rank := h.Rank
		rel := 1.0
		if cfg != domain.ConfigB0 && best > 0 {
			rel = h.Score / best
		}
		out = append(out, fused{ProductID: h.ProductID, Relevance: rel, BM25Rank: &rank})
	}
	return out
}

// groupByFamily — одна позиция на семейство (лучшая по скору), остальные лицензии — variants.
// total — число семейств до обрезки по limit.
func groupByFamily(ranked []fused, docs map[string]indexDoc, normalized, cfg string, limit int,
	purchased func(indexDoc) bool) ([]out.SearchItem, int) {
	items := []out.SearchItem{}
	at := map[string]int{}
	for _, f := range ranked {
		d, ok := docs[f.ProductID]
		if !ok {
			continue
		}
		it := out.SearchItem{
			ProductID: d.ProductID, FamilyID: d.FamilyID, Title: d.Title, Vendor: d.Vendor, Category: d.Category,
			LicenseLine: licenseLine(d), PriceRub: d.PriceRub, PriceOnRequest: d.PriceOnRequest,
			Group: matchGroup(cfg, normalized, d), MatchesQuery: true, PurchasedBefore: purchased(d),
			Final: round4(f.Relevance),
			Kind:  docKind(d), MPN: d.MPNRaw, PriceMode: docPriceMode(d), Required: []out.Companion{},
			OfferURL: d.OfferURL, FamilyURL: d.FamilyURL, VendorURL: d.VendorURL, Available: d.Available,
			OldPriceRub: d.OldPriceRub, LicenseKind: d.LicenseKind,
		}
		if i, ok := at[d.FamilyID]; ok {
			items[i].Variants = append(items[i].Variants, it)
			continue
		}
		at[d.FamilyID] = len(items)
		items = append(items, it)
	}
	total := len(items)
	if len(items) > limit {
		items = items[:limit]
	}
	return items, total
}

// matchGroup — точное совпадение или подходящее. Аналоги появятся вместе со связями в выдаче.
func matchGroup(cfg, normalized string, d indexDoc) string {
	if cfg == domain.ConfigB0 || strings.Contains(foldText(d.Title), normalized) ||
		foldText(d.FamilyName) == normalized || (d.SKU != "" && foldText(d.SKU) == normalized) {
		return domain.GroupExact
	}
	return domain.GroupMatch
}

// traceItems — откуда взялась каждая позиция ответа (и её варианты): ранги источников и слагаемые скора.
func traceItems(items []out.SearchItem, ranked []fused) []domain.TraceItem {
	byID := make(map[string]fused, len(ranked))
	for _, f := range ranked {
		byID[f.ProductID] = f
	}
	res := []domain.TraceItem{}
	add := func(it out.SearchItem) {
		f := byID[it.ProductID]
		res = append(res, domain.TraceItem{
			ProductID: it.ProductID, Final: it.Final, Group: it.Group,
			Parts:    domain.ScoreParts{Relevance: it.Final},
			BM25Rank: f.BM25Rank, DenseRank: f.DenseRank,
		})
	}
	for _, it := range items {
		add(it)
		for _, v := range it.Variants {
			add(v)
		}
	}
	return res
}

// saveSearch — событие и трасса в базу, в фоне: ответ покупателю не ждёт записи. Без базы — ничего.
func (s *Services) saveSearch(req in.SearchRequest, normalized string, resp *out.SearchResponse) {
	if !s.repos.Enabled() {
		return
	}
	tr := resp.Trace
	raw, err := domain.ToJSONB(tr)
	if err == nil {
		raw, err = maskJSONStrings(raw)
	}
	if err != nil {
		zap.L().Error("поиск: трасса не сериализована", zap.String("request_id", tr.RequestID), zap.Error(err))
		return
	}
	// ПДн в журнал не пишутся: запрос и его нормальная форма — с метками вместо телефона, почты, ИНН и т. п.
	// (тот же llm.MaskPII, что перед моделью). Его читают трасса, «Запросы без ответа», журнал вызовов и агент.
	query, _ := maskPII(req.Query)
	normalized, _ = maskPII(normalized)
	ev := &domain.SearchEvent{
		RequestID: tr.RequestID, Query: query, Normalized: normalized, Route: tr.Route, ParseSource: tr.ParseSource,
		Config: tr.Config, ResultsCount: resp.Total, RulesVersion: tr.RulesVersion, CatalogVersion: tr.CatalogVersion,
		ClientID: optional(req.ClientID), SessionID: optional(req.SessionID), TotalMs: int(math.Round(tr.TotalMs)),
		Source: domain.EventSourceLive, IntentID: intentForLog(req.IntentID),
	}
	if req.Source == domain.EventSourceCompare || req.Source == domain.EventSourceTraffic {
		ev.Source = req.Source
	}
	st := &domain.SearchTrace{RequestID: tr.RequestID, Trace: raw}
	s.writes.Add(1)
	go func() {
		defer s.writes.Done()
		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		defer cancel()
		if err := s.repos.SaveSearch(ctx, ev, st); err != nil {
			zap.L().Warn("поиск: событие не записано", zap.String("request_id", ev.RequestID), zap.Error(err))
		}
	}()
}

// skipped — пропущенный шаг с причиной.
func skipped(name, reason string) domain.StepBase {
	return domain.StepBase{Name: name, Skipped: true, Reason: reason}
}

// msSince — миллисекунды с точностью до сотых.
func msSince(t time.Time) float64 {
	return math.Round(float64(time.Since(t).Microseconds())/10) / 100
}

func round4(v float64) float64 { return math.Round(v*1e4) / 1e4 }

func optional(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}
