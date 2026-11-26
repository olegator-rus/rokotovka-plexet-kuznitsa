package services

import (
	"context"
	"errors"
	"fmt"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Панель поиска контура выдачи — GET /api/suggest. Правила: на нажатие — 0 LLM, 0 векторизаций,
// реранкер выключен (этот путь не держит ссылок на модели: счётчики meta — нули по построению); точный артикул и
// точное название не сдвигаются; пустая секция — пустой массив (фронт её скрывает). Секции: подсказки (словарь в
// памяти, микросекунды), товары (текстовый путь, свой таймаут и фоновый кэш), категории и производители, бейджи.

var errNoCatalog = errors.New("каталог не загружен")

// Режимы ответа панели (mode).
const (
	panelModePrefix  = "prefix"
	panelModeEmpty   = "empty"
	panelModeShort   = "short"
	panelModeOff     = "off"
	panelModeTooLong = "too_long"
)

const (
	panelSuggestions   = 5
	panelPopular       = 5
	panelCategoriesMax = 2
	panelVendorsMax    = 2
	panelRefinements   = 6
	// panelCatOne — доля одной категории, при которой она показывается одна; panelCatTwo — две сильные.
	panelCatOne = 0.65
	panelCatTwo = 0.3
)

// SuggestRequest — параметры панели.
type SuggestRequest struct {
	Q, IntentID, SessionID, ClientID string
	// Limit — товаров в панели (1–6; 0 — 6): мобильная раскладка просит 3.
	Limit int
}

// SuggestOptions — служебное: без записи трассы (оценка), без кэша товаров (замер промаха).
type SuggestOptions struct {
	NoLog   bool
	NoCache bool
}

// panelTrace — трасса панели (search_traces, GET /api/traces/{id}): время секций, источник, версии, кандидаты
// до свёртки, маржа (только бэк-офис — ответ витрины её не содержит).
type panelTrace struct {
	Kind               string           `json:"kind"` // suggest
	RequestID          string           `json:"request_id"`
	IntentID           string           `json:"intent_id,omitempty"`
	Query              string           `json:"query"`
	Mode               string           `json:"mode"`
	Candidates         []string         `json:"candidates"`
	Chosen             string           `json:"chosen"`
	ChosenKind         string           `json:"chosen_kind"`
	ProductsText       string           `json:"products_text"`
	Ms                 out.SuggestMs    `json:"ms"`
	ProductsMs         float64          `json:"products_search_ms"`
	Cached             bool             `json:"products_cached"`
	Partial            bool             `json:"partial"`
	SuggestionsVersion int              `json:"suggestions_version"`
	IndexVersion       int              `json:"index_version"`
	CatalogVersion     string           `json:"catalog_version"`
	Margin             string           `json:"margin"` // applied | off | short_prefix | sku_exact | policy_zero | flag_off
	MarginWeight       float64          `json:"margin_weight"`
	Shifts             []panelShift     `json:"margin_shifts"`
	Before             []panelCandidate `json:"candidates_before_fold"`
	Families           []string         `json:"families_shown"`
	Calls              map[string]int   `json:"model_calls"`
}

// SuggestPanel — GET /api/suggest: прежние groups и секции панели.
func (s *Services) SuggestPanel(ctx context.Context, requestID string, req SuggestRequest, opts SuggestOptions) (*out.SuggestResponse, error) {
	started := time.Now()
	for field, v := range map[string]string{"intent_id": req.IntentID, "session_id": req.SessionID} {
		if len(v) > maxIDLen {
			return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{Field: field,
				Code: errs.CodeOutOfRange, Message: "Слишком длинный идентификатор"})
		}
	}
	if req.Limit < 0 || req.Limit > instantSlots {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{Field: "limit",
			Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Товаров в панели — от 1 до %d", instantSlots)})
	}
	if req.Limit == 0 {
		req.Limit = instantSlots
	}
	resp, err := s.Suggest(ctx, req.Q, req.ClientID)
	if err != nil {
		return nil, err
	}
	resp.Q = req.Q
	resp.Refinements, resp.Suggestions, resp.Products = []out.Refinement{}, []out.PanelSuggestion{}, []out.PanelProduct{}
	resp.Categories, resp.Vendors = []out.PanelCategory{}, []out.PanelVendor{}
	resp.Meta.RequestID = requestID
	tr := &panelTrace{Kind: "suggest", RequestID: requestID, IntentID: req.IntentID, Query: req.Q, Candidates: []string{},
		Shifts: []panelShift{}, Before: []panelCandidate{}, Families: []string{}, Calls: map[string]int{"llm": 0, "embed": 0, "rerank": 0}}
	defer func() {
		resp.Meta.Ms.Total = msSince(started)
		tr.Mode, tr.Ms, tr.Partial, tr.Cached = resp.Mode, resp.Meta.Ms, resp.Meta.Partial, resp.Meta.ProductsCached
		tr.SuggestionsVersion, tr.IndexVersion, tr.CatalogVersion = resp.Meta.SuggestionsVersion, resp.Meta.IndexVersion, resp.Meta.CatalogVersion
		if !opts.NoLog {
			s.savePanelTrace(tr)
		}
	}()

	f := s.featuresFor(ctx)
	st := s.catalog.Load()
	q := strings.TrimSpace(foldText(req.Q))
	switch {
	case !f.Panel:
		resp.Mode = panelModeOff
		return resp, nil
	case st == nil || st.lex == nil:
		resp.Mode = panelModePrefix
		return resp, nil
	}
	lx := st.lex
	resp.Meta.CatalogVersion = st.version
	resp.Meta.IndexVersion = s.panelIndexVersion(ctx)
	t := time.Now()
	dict := s.suggestDictFor(lx, st.version)
	if dict != nil {
		resp.Meta.SuggestionsVersion = dict.version
	}
	runes := utf8.RuneCountInString(q)
	switch {
	case runes == 0:
		resp.Mode = panelModeEmpty
		if dict != nil {
			for _, i := range dict.popular {
				if len(resp.Suggestions) == panelPopular {
					break
				}
				if e := dict.entries[i]; e.kind == suggestQuery {
					resp.Suggestions = append(resp.Suggestions, out.PanelSuggestion{Text: e.text, Highlight: [][2]int{},
						Kind: e.kind, Source: e.source, Hits: e.hits})
				}
			}
		}
		resp.Meta.Ms.Suggestions = msSince(t)
		return resp, nil
	case runes < 2:
		resp.Mode = panelModeShort // 1 символ — без запроса к каталогу
		return resp, nil
	case runes > suggestMaxQuery:
		resp.Mode = panelModeTooLong
		return resp, nil
	}
	resp.Mode = panelModePrefix
	rules := s.rulesFor(ctx, domain.ConfigLive, lx, nil)
	pq := normalizePrefix(req.Q, lx, rules)
	if len(pq.cands) == 0 {
		return resp, nil
	}
	resp.Normalized = pq.cands[0].text
	for _, c := range pq.cands {
		tr.Candidates = append(tr.Candidates, c.kind+":"+c.text)
	}

	// Подсказки: набранное, алиасы законченных слов, раскладка; опечатки — только если точных кандидатов нет.
	chosen := pq.cands[0]
	var hits []dictHit
	for _, c := range pq.cands {
		if hits = dict.match(c.text, panelSuggestions*3); len(hits) > 0 {
			chosen = c
			break
		}
	}
	if len(hits) == 0 {
		if c, ok := pq.typo(lx, rules); ok {
			tr.Candidates = append(tr.Candidates, c.kind+":"+c.text)
			if hits = dict.match(c.text, panelSuggestions*3); len(hits) > 0 {
				chosen = c
			}
		}
		if len(hits) == 0 {
			for _, c := range pq.cands {
				if c.kind == prefixLayout {
					chosen = c // по набранному пусто: товары ищутся по переведённой раскладке
				}
			}
		}
	}
	tr.Chosen, tr.ChosenKind = chosen.text, chosen.kind
	if chosen.kind == prefixLayout || chosen.kind == prefixTypo {
		resp.Corrected = &out.SuggestCorrected{From: pq.cands[0].text, To: chosen.text, Kind: chosen.kind}
	}
	skuExact := pq.code != "" && len(lx.codes().exact(pq.code)) > 0

	// Товары — в фоне, пока считаются остальные секции.
	type prodRes struct {
		r      *instantResult
		cached bool
		err    error
	}
	var prodCh chan prodRes
	marginState := "flag_off"
	if f.PanelProducts {
		prodCh = make(chan prodRes, 1)
		var pct map[string]float64
		weight := 0.0
		switch {
		case !f.PanelMargin || !f.MarginPolicy:
		case codeRunes(chosen.text) < instantMarginMinRunes:
			marginState = "short_prefix"
		default:
			pol, perr := s.activeMarginPolicy(ctx)
			switch {
			case perr != nil:
				marginState = "policy_error"
			case pol.Weight <= 0 && len(pol.Categories) == 0:
				marginState = "policy_zero"
			default:
				marginState, weight, pct = "applied", pol.Weight, marginPercentiles(lx)
			}
		}
		tr.MarginWeight = weight
		key := strings.Join([]string{chosen.text, pq.code, st.version, strconv.Itoa(resp.Meta.IndexVersion), strconv.FormatFloat(weight, 'f', 4, 64)}, "|")
		tr.ProductsText = chosen.text
		cache := s.panel().products
		go func() {
			if !opts.NoCache {
				if r, ok := cache.get(key); ok {
					prodCh <- prodRes{r: r, cached: true}
					return
				}
			}
			pctx, cancel := context.WithTimeout(ctx, productsTimeout)
			defer cancel()
			fn := func(c context.Context) (*instantResult, error) {
				return s.instantProducts(c, lx, chosen.text, pq.code, pct)
			}
			var r *instantResult
			var err error
			if opts.NoCache {
				r, err = fn(pctx)
			} else {
				r, err = cache.do(pctx, key, fn)
			}
			prodCh <- prodRes{r: r, err: err}
		}()
	}

	if !skuExact {
		resp.Suggestions = panelSuggestionsOf(dict, hits, utf8.RuneCountInString(chosen.text))
	}
	resp.Vendors = panelVendors(lx, rules, chosen.text, pq)
	resp.Meta.Ms.Suggestions = msSince(t)

	var r *instantResult
	if prodCh != nil {
		tp := time.Now()
		pr := <-prodCh
		resp.Meta.Ms.Products = msSince(tp)
		switch {
		case pr.err != nil && errors.Is(pr.err, context.DeadlineExceeded):
			resp.Meta.Partial = true // не успели: ответ без товаров, кэш дописывается в фоне
		case pr.err != nil:
			resp.Meta.Partial = true
			zap.L().Warn("панель: товарная секция не посчитана", zap.String("request_id", requestID), zap.Error(pr.err))
		default:
			r, resp.Meta.ProductsCached = pr.r, pr.cached
		}
	}
	tr.Margin = marginState
	if r != nil {
		resp.Products = panelProducts(lx, r, req.Limit)
		resp.TotalHint = r.total
		tr.ProductsMs, tr.Shifts, tr.Before = r.ms, r.shifts, r.before
		if r.margin != "" && marginState == "applied" {
			tr.Margin = r.margin
		}
		for _, p := range resp.Products {
			tr.Families = append(tr.Families, p.FamilyID)
		}
		if !r.skuExact {
			resp.Categories = panelCategories(lx, r)
		}
	} else if len(hits) > 0 && dict.entries[hits[0].entry].norm == chosen.text {
		resp.TotalHint = dict.entries[hits[0].entry].hits
	}
	tf := time.Now()
	if !skuExact {
		var top *dictEntry
		if len(hits) > 0 {
			top = &dict.entries[hits[0].entry]
		}
		var docs []indexDoc
		if r != nil {
			// Только семейства выше порога панели: хвост кандидатов BM25 (мониторы на «ноутбук») бейджей не даёт.
			keep := map[string]bool{}
			for _, f := range r.families {
				keep[f.id] = true
			}
			docs = panelRefineDocs(r.docs, keep)
		}
		var shown []string
		for _, sg := range resp.Suggestions {
			shown = append(shown, sg.Text)
		}
		for _, p := range resp.Products {
			shown = append(shown, p.FamilyName)
		}
		resp.Refinements = refinementsFor(refineCtx{lx: lx, hx: st.hw, query: chosen.text}, top, docs, shown)
	}
	resp.Meta.Ms.Refinements = msSince(tf)
	return resp, nil
}

// panelSuggestionsOf — до 5 подсказок; подсказка-категория — не больше одной.
func panelSuggestionsOf(dict *suggestDict, hits []dictHit, qlen int) []out.PanelSuggestion {
	res := []out.PanelSuggestion{}
	cats := 0
	for _, h := range hits {
		if len(res) == panelSuggestions {
			break
		}
		e := dict.entries[h.entry]
		if e.kind == suggestCategory {
			if cats > 0 {
				continue
			}
			cats++
		}
		end := min(h.pos+qlen, utf8.RuneCountInString(e.text))
		res = append(res, out.PanelSuggestion{Text: e.text, Highlight: [][2]int{{h.pos, end}}, Kind: e.kind,
			Source: e.source, Hits: e.hits, CategoryID: e.categoryID, CategoryName: dict.categoryName(e.categoryID)})
	}
	return res
}

// categoryName — название категории подсказки по словарю каталога снимка.
func (d *suggestDict) categoryName(id string) string {
	if id == "" || d.lx == nil {
		return ""
	}
	return d.lx.categoryName[id]
}

// panelVendors — производитель показывается при точном совпадении: его название (или алиас) названо законченными
// словами либо ввод — начало названия (от 3 символов), а не потому, что у него больше позиций.
func panelVendors(lx *searchLexicon, rules *searchRules, text string, pq prefixQuery) []out.PanelVendor {
	ids := lx.matchVendors(strings.Fields(text), rules)
	if utf8.RuneCountInString(text) >= 3 {
		for _, id := range sortedKeys(lx.vendorName) {
			if strings.HasPrefix(foldText(lx.vendorName[id]), text) && !contains(ids, id) {
				ids = append(ids, id)
			}
		}
		if rules != nil {
			for _, p := range sortedKeys(rules.vendorPhrases) {
				if id := rules.vendorPhrases[p]; strings.HasPrefix(p, text) && lx.vendorName[id] != "" && !contains(ids, id) {
					ids = append(ids, id)
				}
			}
		}
	}
	res := []out.PanelVendor{}
	for _, id := range ids {
		if len(res) == panelVendorsMax {
			break
		}
		res = append(res, out.PanelVendor{ID: id, Name: lx.vendorName[id]})
	}
	_ = pq
	return res
}

// panelCategories — взвешенная доля категории среди семейств-кандидатов после свёртки (семейство — голос с весом
// своей релевантности: десятки лицензий одного продукта категорию не раздувают). Доля ≥ 0,65 — одна категория;
// две ≥ 0,3 и вместе ≥ 0,8 — обе; размыто — ни одной.
func panelCategories(lx *searchLexicon, r *instantResult) []out.PanelCategory {
	res := []out.PanelCategory{}
	if len(r.families) < 2 {
		return res
	}
	w, total := map[string]float64{}, 0.0
	for _, f := range r.families {
		w[f.head.CategoryID] += f.rel
		total += f.rel
	}
	if total == 0 {
		return res
	}
	ids := sortedKeys(w)
	sort.SliceStable(ids, func(i, j int) bool { return w[ids[i]] > w[ids[j]] })
	share := func(id string) float64 { return math.Round(w[id]/total*100) / 100 }
	switch {
	case share(ids[0]) >= panelCatOne:
		res = append(res, out.PanelCategory{ID: ids[0], Name: lx.categoryName[ids[0]], Share: share(ids[0])})
	case len(ids) > 1 && share(ids[1]) >= panelCatTwo && share(ids[0])+share(ids[1]) >= 0.8:
		for _, id := range ids[:2] {
			res = append(res, out.PanelCategory{ID: id, Name: lx.categoryName[id], Share: share(id)})
		}
	}
	return res
}

// Поля структурированных уточнений (ключи фильтров страницы результатов, §11.5).
const (
	refFieldType     = facetLicType
	refFieldDuration = facetDuration
	refFieldMetric   = facetMetric
	refOpEq          = "eq"
)

var refNumRe = regexp.MustCompile(`\d`)

// refinementsFor — бейджи-уточнения панели: сначала структурированные условия из
// значений фасетов кандидатов (structuredRefinements — тот же источник, что чипы над выдачей), затем продолжения
// верхней подсказки (next_tokens, проверены числом результатов при сборке) — только те, что не дублируют
// показанные подсказки и названия семейств в товарах (shown).
func refinementsFor(rc refineCtx, top *dictEntry, docs []indexDoc, shown []string) []out.Refinement {
	query := strings.TrimSpace(rc.query)
	rc.query = query
	base := query
	if top != nil && strings.HasPrefix(top.norm, query) && !refNumRe.MatchString(query) {
		base = top.text
	}
	res := structuredRefinements(rc, docs, panelRefinements) // названное — по набранному, не по верхней подсказке
	for i := range res {
		res[i].Query = joinQuery(base, res[i].Label)
	}
	if top == nil || !strings.HasPrefix(top.norm, query) || len(res) >= panelRefinements {
		return res
	}
	have := map[string]bool{}
	for _, w := range strings.Fields(foldText(query)) {
		have[w] = true
	}
	shownWords := map[string]bool{}
	for _, t := range shown {
		for _, w := range strings.Fields(foldText(t)) {
			shownWords[w] = true
		}
	}
	seen := map[string]bool{}
	for _, r := range res {
		seen[foldText(r.Label)] = true
	}
	for _, n := range top.next {
		key := foldText(n.Token)
		if len(res) == panelRefinements || n.Hits <= 0 || seen[key] {
			continue
		}
		dup := true // все значимые слова продолжения уже видны в подсказках или товарах — дубль
		for _, w := range strings.Fields(key) {
			if have[w] && !lexStopWords[w] {
				dup = true
				break
			}
			if !lexStopWords[w] && !shownWords[w] {
				dup = false
			}
		}
		if dup {
			continue
		}
		seen[key] = true
		res = append(res, out.Refinement{Label: n.Token, Query: top.text + " " + n.Token, Hits: n.Hits})
	}
	return res
}

// panelRefineDocs — кандидаты для бейджей панели: семейства выше порога. Если такое семейство одно (широкое «антивирус»
// с одним совпадением по названию), — кандидаты того же корневого раздела каталога, чтобы бейджи уточняли тип
// продукта, а не варианты одного семейства.
func panelRefineDocs(all []indexDoc, keep map[string]bool) []indexDoc {
	var docs []indexDoc
	roots := map[string]bool{}
	for _, d := range all {
		if keep[d.FamilyID] {
			docs = append(docs, d)
			roots[docRoot(d)] = true
		}
	}
	if len(keep) != 1 {
		return docs
	}
	var wide []indexDoc
	fams := map[string]bool{}
	for _, d := range all {
		if roots[docRoot(d)] {
			wide = append(wide, d)
			fams[d.FamilyID] = true
		}
	}
	if len(fams) < 2 {
		return docs
	}
	return wide
}

// docRoot — корневой раздел каталога позиции.
func docRoot(d indexDoc) string {
	if len(d.CategoryIDs) > 0 {
		return d.CategoryIDs[0]
	}
	return d.CategoryID
}

// Refinements — бейджи-уточнения для страницы результатов (§11.5): тот же источник, что у панели. docs — позиции
// выдачи (с вариантами).
func (s *Services) Refinements(query string, docs []indexDoc) []out.Refinement {
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return []out.Refinement{}
	}
	q := strings.TrimSpace(foldText(query))
	var top *dictEntry
	if d := s.suggestDictFor(st.lex, st.version); d != nil {
		if i, ok := d.byNorm[q]; ok {
			top = &d.entries[i]
		} else if hs := d.match(q, 1); len(hs) > 0 {
			top = &d.entries[hs[0].entry]
		}
	}
	return refinementsFor(refineCtx{lx: st.lex, hx: st.hw, query: q}, top, docs, nil)
}

// panelIndexVersion — номер физического индекса за алиасом (products_vN): ключ кэша и meta. Перечитывается, когда
// меняется built_at индекса.
func (s *Services) panelIndexVersion(ctx context.Context) int {
	info, err := s.indexInfo(ctx)
	if err != nil {
		return 0
	}
	ps := s.panel()
	ps.mu.Lock()
	cur := ps.idx
	ps.mu.Unlock()
	if cur.builtAt.Equal(info.BuiltAt) && cur.version > 0 {
		return cur.version
	}
	names, err := s.search.AliasIndices(ctx, s.alias)
	if err != nil || len(names) == 0 {
		return 0
	}
	v := 0
	for _, n := range names {
		if i := strings.LastIndex(n, "_v"); i >= 0 {
			if x, err := strconv.Atoi(n[i+2:]); err == nil && x > v {
				v = x
			}
		}
	}
	ps.mu.Lock()
	ps.idx = panelIndexVersion{builtAt: info.BuiltAt, version: v}
	ps.mu.Unlock()
	return v
}

// savePanelTrace — трасса панели в search_traces в фоне (без события search: нажатие — не поиск).
func (s *Services) savePanelTrace(tr *panelTrace) {
	if !s.repos.Enabled() || tr.RequestID == "" {
		return
	}
	raw, err := domain.ToJSONB(tr)
	if err != nil {
		return
	}
	s.writes.Add(1)
	go func() {
		defer s.writes.Done()
		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		defer cancel()
		if err := s.repos.SaveTrace(ctx, &domain.SearchTrace{RequestID: tr.RequestID, Trace: raw}); err != nil {
			zap.L().Warn("панель: трасса не записана", zap.String("request_id", tr.RequestID), zap.Error(err))
		}
	}()
}
