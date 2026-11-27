package services

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/prompts"

	"go.uber.org/zap"
)

// BuildSetsResult — собранные наборы и отчёт.
type BuildSetsResult struct {
	Queries []*DerivedQuery
	Report  *SetsReport
}

// BuildSets — наборы оценки из журнала (см. evalsets.go). Каталог должен быть загружен (LoadCatalogState):
// классификация и судья опираются на словарь каталога, пул — на индекс поиска.
func (s *Services) BuildSets(ctx context.Context, req BuildSetsRequest) (*BuildSetsResult, error) {
	started := time.Now()
	if req.MinFreq <= 0 {
		req.MinFreq = setsDefaultMinFreq
	}
	if req.MinSupport <= 0 {
		req.MinSupport = setsDefaultMinSupport
	}
	if req.ClickGrade != 1 {
		req.ClickGrade = 2
	}
	if req.JudgeCalib < 0 {
		req.JudgeCalib = 0
	}
	progress := func(f string, a ...any) {
		if req.Progress != nil {
			req.Progress(fmt.Sprintf(f, a...))
		}
	}
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return nil, errs.NewUnavailableError("Каталог не загружен — сначала импорт (make import)")
	}
	lx := st.lex
	rules := newSearchRules(1, lx.baseAliases)

	searches, behavior, err := s.setsJournal(ctx, req)
	if err != nil {
		return nil, err
	}
	lg := aggregateLog(searches, behavior)
	rep := &SetsReport{Generated: started.UTC(), MinFreq: req.MinFreq, MinSupport: req.MinSupport, ClickGrade: req.ClickGrade, Days: req.Days,
		LogSearches: lg.searches, LogActions: lg.actions, LogTexts: len(lg.queries), LogFrom: lg.from.UTC(), LogTo: lg.to.UTC(),
		Unattributed: lg.unattributed}
	progress("журнал: %d поисков, %d действий с товаром, %d различных запросов", lg.searches, lg.actions, len(lg.queries))

	h := s.agent.opts.Holdout
	if !h.Enabled {
		h = HoldoutOptions{Enabled: true}
	}
	hFrom, hTo := s.holdoutRangeAt(h, lg.to, lg.to)
	rep.HoldoutFrom, rep.HoldoutTo = hFrom.UTC(), hTo.UTC()

	// Отбор и неявная разметка.
	var logs []*setsLogQuery
	for _, q := range lg.queries {
		if q.freq >= req.MinFreq {
			logs = append(logs, q)
		} else {
			rep.BelowFreq++
		}
	}
	sort.Slice(logs, func(i, j int) bool {
		if logs[i].freq != logs[j].freq {
			return logs[i].freq > logs[j].freq
		}
		return logs[i].key < logs[j].key
	})
	qs := make([]*DerivedQuery, 0, len(logs))
	items := map[string]classifyItem{}
	ruleTypes := map[string]string{}
	first := map[string]time.Time{}
	support := map[string]map[string]int{} // id запроса → товар → поисков с действием
	for i, lq := range logs {
		q := &DerivedQuery{EvalQuery: domain.EvalQuery{ID: derivedID(i + 1), Text: lq.text, Freq: lq.freq,
			Judgments: map[string]int{}}, Sources: map[string]string{}, Terms: []string{}, Searches: lq.freq,
			FirstSeen: lq.first.UTC().Format(time.RFC3339)}
		q.EmptyShare = evalRound4(float64(lq.empty) / float64(lq.freq))
		q.ClickShare = evalRound4(float64(lq.clicked) / float64(lq.freq))
		for pid, ps := range lq.products {
			if _, ok := lx.docs[pid]; !ok {
				rep.UnknownProducts++
				continue
			}
			if g, ok := implicitGrade(ps, lq.freq, req.MinSupport, req.ClickGrade); ok {
				q.Judgments[pid] = g
				q.Sources[pid] = labelImplicit
				if support[q.ID] == nil {
					support[q.ID] = map[string]int{}
				}
				support[q.ID][pid] = ps.c1
			} else {
				rep.LowSupport++
			}
		}
		switch {
		case q.EmptyShare >= setsEmptyProblem:
			q.Problem = problemEmpty
		case lq.clicked == 0:
			q.Problem = problemNoClick
		}
		sig, _ := queryTerms(lq.text, lx, rules)
		q.Terms = append(q.Terms, sig...)
		typ, sure, unknown := ruleType(lq.text, lx, rules)
		if sure {
			ruleTypes[q.ID] = typ
		}
		reform, found := lq.topReform()
		items[q.ID] = classifyItem{ID: q.ID, Query: lq.text, UnknownWords: append([]string{}, unknown...),
			EmptyShare: q.EmptyShare, ClickShare: q.ClickShare, ReformulatedTo: reform, ReformulationFound: found}
		first[q.ID] = lq.first
		qs = append(qs, q)
	}
	if len(qs) == 0 {
		return nil, errs.NewValidationError(fmt.Sprintf("Нет запросов с частотой ≥ %d — уменьшите --min-freq", req.MinFreq))
	}
	progress("отобрано запросов: %d (частота ≥ %d), неявных оценок: %d", len(qs), req.MinFreq, countSources(qs, labelImplicit))

	// Типы.
	rep.Classify = s.classifyQueries(ctx, qs, items, ruleTypes, req.Classifier, req.ClassifyMaxRub, req.CacheDir)
	for _, q := range qs {
		q.Absent = q.Type == domain.QueryAbsent
		q.Notes = derivedNotes(q, items[q.ID])
		rep.TargetRaised += applyTargetFamily(q, support[q.ID], func(pid string) string { return lx.docs[pid].FamilyID })
		if q.TargetFamily != "" {
			rep.TargetQueries++
		}
	}
	progress("правило целевого семейства: exact/sku с целевым семейством %d, клик 2 → 3: %d", rep.TargetQueries, rep.TargetRaised)
	progress("типы: правила %d, модель %d, кэш %d, по умолчанию %d (%.2f ₽)", rep.Classify.Rules, rep.Classify.LLM,
		rep.Classify.Cache, rep.Classify.Default, rep.Classify.CostRub)

	// Кластеры и наборы.
	clusterQueries(qs, lg.queries, s.queryVectors(ctx, qs), lx, rules)
	assignSets(qs, first, hFrom)

	// Судья: пул топ-10 B0–B4, по одному товару на семейство запроса.
	var pool *out.EvalRun
	if req.JudgeMaxRub > 0 && s.judge != nil {
		progress("пул топ-10 B0–B4 и судья по семействам (лимит %.0f ₽)…", req.JudgeMaxRub)
		pool, rep.Judge, err = s.setsJudge(ctx, qs, lx, req.JudgeMaxRub, req.JudgeCalib, req.CacheDir)
		if err != nil {
			return nil, err
		}
	} else if req.JudgeMaxRub > 0 {
		rep.JudgeSkipped = MsgJudgeNoModel
	}

	sort.Slice(qs, func(i, j int) bool { return qs[i].ID < qs[j].ID })
	rep.fill(qs)
	if req.Reference != nil {
		rep.Reference = compareReference(qs, req.Reference, pool, rep.Judge, func(pid string) string { return lx.docs[pid].FamilyID })
	}
	rep.DurationMs = time.Since(started).Milliseconds()
	zap.L().Info("наборы собраны", zap.Int("queries", len(qs)), zap.Int64("ms", rep.DurationMs))
	return &BuildSetsResult{Queries: qs, Report: rep}, nil
}

func countSources(qs []*DerivedQuery, src string) int {
	n := 0
	for _, q := range qs {
		for _, v := range q.Sources {
			if v == src {
				n++
			}
		}
	}
	return n
}

// derivedNotes — что о запросе известно из журнала (для человека, проверяющего набор).
func derivedNotes(q *DerivedQuery, it classifyItem) string {
	n := fmt.Sprintf("из журнала: %d поисков, пустых %.0f %%, с кликом %.0f %%; тип — %s", q.Searches,
		q.EmptyShare*100, q.ClickShare*100, q.TypeSource)
	if q.Problem != "" {
		n += "; проблемный (" + q.Problem + ")"
	}
	if it.ReformulatedTo != "" {
		n += "; переформулируют в «" + it.ReformulatedTo + "»"
	}
	return n
}

// applyTargetFamily — правило разметки exact/sku: неявная оценка 2 становится 3, если товар из целевого семейства
// (семейство с наибольшей неявной поддержкой). Равенство лидеров или нет кликов — цели нет, правило не применяется.
// Возвращает, сколько оценок повышено.
func applyTargetFamily(q *DerivedQuery, support map[string]int, family func(string) string) int {
	if !slices.Contains(exactTypes, q.Type) || len(support) == 0 {
		return 0
	}
	byFam := map[string]int{}
	for pid, n := range support {
		if f := family(pid); f != "" {
			byFam[f] += n
		}
	}
	best, top, tie := "", 0, false
	for f, n := range byFam {
		switch {
		case n > top:
			best, top, tie = f, n, false
		case n == top:
			tie = true
		}
	}
	if best == "" || tie {
		return 0
	}
	q.TargetFamily = best
	raised := 0
	for pid := range support {
		if family(pid) == best && q.Sources[pid] == labelImplicit && q.Judgments[pid] == 2 {
			q.Judgments[pid] = exactGrade
			raised++
		}
	}
	return raised
}

// SetsJudge — что сделал судья при сборке.
type SetsJudge struct {
	Model         string   `json:"model"`
	PromptVersion string   `json:"prompt_version"`
	PoolRunID     string   `json:"pool_run_id"`
	PoolConfigs   []string `json:"pool_configs"`
	PoolPairs     int      `json:"pool_pairs"`
	// PoolFamilies — пар «запрос — семейство» в пуле: судья оценивает одного представителя семейства, оценка
	// переносится на все артикулы семейства в этом запросе (шкала судьи — по продукту, вариант не важен).
	PoolFamilies int `json:"pool_families"`
	PoolImplicit int `json:"pool_implicit"`
	Judged       int `json:"judged"`
	Filled       int `json:"filled"`
	FilledZero   int `json:"filled_zero"`
	// Raised — неявных оценок, которые судья повысил (понижать неявную оценку судья не может).
	Raised int `json:"raised"`
	// Cached — оценок взято из кэша (.judge-cache.json): повторная сборка не платит второй раз.
	Cached    int     `json:"cached"`
	Calls     int     `json:"calls"`
	CostRub   float64 `json:"cost_rub"`
	BudgetRub float64 `json:"budget_rub"`
	Stopped   string  `json:"stopped,omitempty"`
	// VsImplicit — согласие судьи с неявной разметкой там, где у семейства есть и то и другое. На экран не идёт:
	// неявные оценки почти все 2, каппа против них вырождена.
	VsImplicit out.JudgeAgreement `json:"vs_implicit"`
	// grades — оценки судьи по паре «id запроса|товар».
	grades map[string]int
}

// setsJudgeChunk — пар в порции: внутри порции порядок перемешан (вслепую), между порциями проверяется лимит.
const setsJudgeChunk = 50

// setsPoolConfigs — из чьих топ-10 собирается пул судьи: вся лестница, включая B4.
var setsPoolConfigs = []string{domain.ConfigB0, domain.ConfigB1, domain.ConfigB2, domain.ConfigB3, domain.ConfigB4}

// setsJudge — прогон B0–B4 по собранным запросам (без журнала и без записи в evaluations), пул топ-10 по слоям
// глубины, по одному товару на семейство запроса, пока хватает лимита. Оценка судьи переносится на все артикулы
// семейства в запросе: заполняет пары без неявной оценки и повышает неявную, если судья дал больше (не понижает).
func (s *Services) setsJudge(ctx context.Context, qs []*DerivedQuery, lx *searchLexicon, maxRub float64, _ int, cacheDir string) (*out.EvalRun, *SetsJudge, error) {
	eqs := make([]domain.EvalQuery, len(qs))
	byID := map[string]*DerivedQuery{}
	for i, q := range qs {
		eqs[i] = q.EvalQuery
		eqs[i].Judgments = cloneGrades(q.Judgments)
		byID[q.ID] = q
	}
	run, err := s.RunEvaluation(ctx, EvalRequest{Set: "sets", Configs: setsPoolConfigs, Queries: eqs})
	if err != nil {
		return nil, nil, err
	}
	pairs, _ := judgePool(run, eqs, 1<<30)
	res := &SetsJudge{Model: llmModelName(s.judge.Model()), PoolRunID: run.RunID, PoolConfigs: setsPoolConfigs, PoolPairs: len(pairs),
		BudgetRub: maxRub, grades: map[string]int{}}
	famOf := func(pid string) string {
		if d, ok := lx.docs[pid]; ok && d.FamilyID != "" {
			return d.FamilyID
		}
		return pid
	}
	// Представитель семейства — первая пара семейства в пуле (верхние позиции раньше).
	var queue []*judgePairIn
	repOf := map[string]*judgePairIn{} // «запрос|семейство» → представитель
	for _, p := range pairs {
		if _, ok := p.q.Judgments[p.pid]; ok {
			res.PoolImplicit++
		}
		k := p.q.ID + "|" + famOf(p.pid)
		if repOf[k] == nil {
			repOf[k] = p
			queue = append(queue, p)
		}
	}
	res.PoolFamilies = len(queue)
	sys := prompts.Judge()
	res.PromptVersion = sys.Version
	cache := loadGradeCache(cacheDir)
	ckey := func(p *judgePairIn) string { return cache.key(res.Model, sys.Version, p.q.Text+"\x00"+p.pid) }
	batchNo := 0
	for c := 0; c*setsJudgeChunk < len(queue) && res.Stopped == ""; c++ {
		chunk := slices.Clone(queue[c*setsJudgeChunk : min(len(queue), (c+1)*setsJudgeChunk)])
		judgeOrder(chunk)
		var todo []*judgePairIn
		for _, p := range chunk {
			if g, ok := cache.grades[ckey(p)]; ok {
				g := g
				p.grade = &g
				res.Cached++
				continue
			}
			todo = append(todo, p)
		}
		for b := 0; b*judgeBatch < len(todo); b++ {
			if err := ctx.Err(); err != nil {
				return nil, nil, err
			}
			if res.Calls > 0 && res.CostRub+res.CostRub/float64(res.Calls) > maxRub {
				res.Stopped = fmt.Sprintf("лимит %.0f ₽: в очереди %d семейств, дальше не оценивались", maxRub, len(queue))
				break
			}
			batch := todo[b*judgeBatch : min(len(todo), (b+1)*judgeBatch)]
			batchNo++
			rid := fmt.Sprintf("%ssets-%s-%03d", judgeRequestPrefix, run.RunID[len(run.RunID)-6:], batchNo)
			cost, calls, err := s.judgeBatch(WithRequestID(ctx, rid), sys.Text, lx, batch)
			res.CostRub += cost
			res.Calls += calls
			if err != nil {
				zap.L().Warn("наборы: пачка судьи не оценена", zap.Int("batch", batchNo), zap.Error(err))
			}
			for _, p := range batch {
				if p.grade != nil {
					cache.grades[ckey(p)] = *p.grade
					cache.dirty = true
				}
			}
		}
		for _, p := range chunk {
			if p.grade != nil {
				res.Judged++
				res.grades[p.q.ID+"|"+p.pid] = *p.grade
			}
		}
	}
	if err := cache.save(); err != nil {
		zap.L().Warn("наборы: кэш судьи не записан", zap.Error(err))
	}
	res.CostRub = round4(res.CostRub)

	// Оценка семейства → все артикулы семейства в запросе: пары пула и кликнутые артикулы того же семейства.
	var vsImplicit []out.JudgePair
	apply := func(q *DerivedQuery, pid string, g int) {
		cur, labelled := q.Judgments[pid]
		src := q.Sources[pid]
		switch {
		case src == labelImplicit || src == labelImplicitJudge:
			vsImplicit = append(vsImplicit, out.JudgePair{QueryID: q.ID, ProductID: pid, Label: cur, Judge: evalPtr(g)})
			if g > cur {
				q.Judgments[pid] = g
				q.Sources[pid] = labelImplicitJudge
				res.Raised++
			}
		case labelled:
			// другой источник — не перезаписывается
		default:
			// Оценка 0 пишется явно: размеченное нерелевантное отличается от неразмеченного (метрики его не штрафуют).
			q.Sources[pid] = labelJudge
			q.Judgments[pid] = g
			res.Filled++
			if g == 0 {
				res.FilledZero++
			}
		}
	}
	famGrade := map[string]int{}
	for k, rp := range repOf {
		if rp.grade != nil {
			famGrade[k] = *rp.grade
		}
	}
	done := map[string]bool{}
	for _, p := range pairs {
		k := p.q.ID + "|" + famOf(p.pid)
		g, ok := famGrade[k]
		if !ok || done[p.q.ID+"|"+p.pid] {
			continue
		}
		done[p.q.ID+"|"+p.pid] = true
		apply(byID[p.q.ID], p.pid, g)
	}
	for _, q := range qs {
		for pid, src := range q.Sources {
			if src != labelImplicit || done[q.ID+"|"+pid] {
				continue
			}
			if g, ok := famGrade[q.ID+"|"+famOf(pid)]; ok {
				done[q.ID+"|"+pid] = true
				apply(q, pid, g)
			}
		}
	}
	judgeAgreement(&res.VsImplicit, vsImplicit)
	return run, res, nil
}

func cloneGrades(m map[string]int) map[string]int {
	c := make(map[string]int, len(m))
	for k, v := range m {
		c[k] = v
	}
	return c
}
