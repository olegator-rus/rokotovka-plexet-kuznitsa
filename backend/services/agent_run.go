package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Ход одного запуска (ARCHITECTURE §3.1): эффект принятых правок → неудачные запросы → кластеры по
// потерянному спросу → память и проверка действующим поиском → расследование кластеров моделью.

// runCtx — данные запуска, общие для всех кластеров.
type runCtx struct {
	a         *agentService
	rs        *runState
	active    *ruleSet
	sb        *sandbox
	cat       *agentCatalog
	stats     *queryStats
	failed    []clusterQuery
	clusters  []*failedCluster
	byID      map[string]*failedCluster
	rows      map[string]*domain.IssueCluster
	decisions []domain.Decision
	proposals []domain.Proposal
	// succ — успешные запросы и их векторы (find_similar_successful_queries), лениво.
	succ     []clusterQuery
	succVecs [][]float32
	succDone bool
	// memVecs — векторы терминов памяти (agent_memory.go); memVecOff — эмбеддер не ответил, векторы не берутся.
	memVecs   map[string][]float32
	memVecOff bool
	episode   map[string]any
	results   []map[string]any
	skipped   []map[string]any
	// Контур автономии: план, память («не предлагать», эпизоды), что изменилось у кластеров с наступившим условием возврата,
	// итог по видам, черновики режима «только чтение», неразобранное и версия каталога.
	plan           *runPlan
	suppress       []domain.MemorySuppress
	episodes       []domain.MemoryEpisode
	lifted         map[string]string
	outcome        out.RunOutcome
	drafts         []map[string]any
	unprocessed    []map[string]any
	memorySkipped  []map[string]any
	catalogVersion string
}

// execute — запуск целиком; любой исход записывается в статус запуска.
func (a *agentService) execute(ctx context.Context, rs *runState) {
	rc := &runCtx{a: a, rs: rs, byID: map[string]*failedCluster{}, rows: map[string]*domain.IssueCluster{},
		episode: map[string]any{}, lifted: map[string]string{}}
	if cs := a.s.catalog.Load(); cs != nil {
		rc.catalogVersion = cs.version
	}
	var err error
	unit := "кластеров"
	switch {
	case rs.run.Trigger == domain.TriggerCatalogChange:
		err, unit = rc.runCatalogChange(ctx), "находок"
	case rs.run.Mode == domain.RunModeCommerce:
		err, unit = rc.runCommerce(ctx), "находок"
	case rs.spec.Query != "":
		err = rc.runQuery(ctx)
	default:
		err = rc.run(ctx)
	}
	now := a.s.now().UTC()
	// Учёт: вызовы разбора намерения внутри песочницы пишутся в llm_usage с тем же agent_run_id — ждём
	// только записи этого запуска (не общий s.writes, см. runWrites).
	if !rs.writes.wait(10 * time.Second) {
		zap.L().Warn("агент: записи учёта запуска не дождались — итог по счётчикам в памяти", zap.String("run_id", rs.run.ID.String()))
	}
	var usage *AgentUsage
	if st := a.s.store(); st != nil {
		if u, uerr := st.AgentRunUsage(context.WithoutCancel(ctx), rs.run.ID.String()); uerr == nil && u.Calls > 0 {
			usage = &u
		}
	}
	rs.mu.Lock()
	r := rs.run
	if usage != nil {
		// Итог — по llm_usage: туда же попадают разборы намерения внутри песочницы с тем же agent_run_id.
		r.InputTokens, r.OutputTokens, r.CostRub = usage.InputTokens, usage.OutputTokens, usage.CostRub
		rc.episode["llm_calls"] = usage.Calls
	}
	r.FinishedAt = &now
	r.CurrentClusterID, r.CurrentLabel = nil, ""
	// Итог по видам и бюджет (контур автономии).
	for _, res := range rc.results {
		if r.ReadOnly {
			break // в режиме «только чтение» предложений и задач нет — только черновики
		}
		switch res["outcome"] {
		case "proposal":
			rc.outcome.Proposals++
		case "ticket":
			rc.outcome.Tasks++
		}
	}
	rc.episode["outcome"] = rc.outcome
	rc.episode["unprocessed"] = nonNil(rc.unprocessed)
	rc.episode["memory_skipped"] = nonNil(rc.memorySkipped)
	if len(rc.drafts) > 0 {
		rc.episode["drafts"] = rc.drafts
	}
	exh := ""
	if _, ok := budgetLabels[r.StopReason]; ok {
		exh = r.StopReason
	}
	r.Budget = toJSON(budgetView(rs.budget, budgetUsed{Steps: r.ToolCalls, Tokens: r.InputTokens + r.OutputTokens,
		CostRub: r.CostRub, Seconds: int(now.Sub(r.StartedAt).Seconds())}, exh))
	switch {
	case rs.stop != "":
		r.Status, r.StopReason = domain.RunStopped, rs.stop
	case err != nil && errors.Is(err, context.DeadlineExceeded):
		r.Status, r.StopReason = domain.RunStopped, StopTimeout
	case err != nil:
		r.Status = domain.RunFailed
		if r.StopReason == "" {
			r.StopReason = StopError
		}
	default:
		r.Status = domain.RunDone
	}
	rc.episode["clusters"] = rc.results
	r.Episode = toJSON(rc.episode)
	if err != nil && rs.stop == "" && !errors.Is(err, context.DeadlineExceeded) {
		r.Summary = "Ошибка: " + shortText(err.Error(), 200) + ". "
	} else {
		r.Summary = ""
	}
	r.Summary += fmt.Sprintf("Разобрано %s: %d из %d, предложений в очереди: %d, вызовов инструментов: %d, %d токенов, %.2f ₽",
		unit, r.ClustersDone, r.ClustersTotal, r.ProposalsCreated, r.ToolCalls, r.InputTokens+r.OutputTokens, r.CostRub)
	if n, _ := rc.episode["clusters_limited"].(int); n > 0 {
		r.Summary += fmt.Sprintf(". Упёрлись в лимит %d вызовов на кластер: %d", agentToolCallsPerCluster, n)
	}
	if n, _ := rc.episode["clusters_unfinished"].(int); n > 0 {
		r.Summary += fmt.Sprintf(". Без итога (модель не вызвала завершающий инструмент): %d", n)
	}
	if o := rc.outcome; o.Tasks+o.SkippedMemory+o.EffectChecks+o.Rollbacks+o.Drafts > 0 {
		r.Summary += fmt.Sprintf(". Задач владельцам: %d, пропущено по памяти: %d", o.Tasks, o.SkippedMemory)
		if o.EffectChecks > 0 {
			r.Summary += fmt.Sprintf(", проверено правок: %d (помогло %d, без эффекта %d, навредило %d)", o.EffectChecks, o.Helped, o.NoEffect, o.Harmed)
		}
		if o.Rollbacks > 0 {
			r.Summary += fmt.Sprintf(", предложено откатов: %d", o.Rollbacks)
		}
		if o.Drafts > 0 {
			r.Summary += fmt.Sprintf(", черновиков (только чтение): %d", o.Drafts)
		}
	}
	if l, ok := budgetLabels[r.StopReason]; ok {
		r.Summary += fmt.Sprintf(". Остановлено: %s; неразобрано: %d", l, len(rc.unprocessed))
	}
	rs.mu.Unlock()
	if err != nil && rs.stopReason() == "" {
		a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepError, Title: "Запуск прерван: " + shortText(err.Error(), 200)})
	}
	a.saveRun(ctx, rs)
	rs.mu.Lock()
	cp := *rs.run
	rs.mu.Unlock()
	a.finish(rs, a.runView(&cp))
	zap.L().Info("агент: запуск завершён", zap.String("run_id", cp.ID.String()), zap.String("status", cp.Status),
		zap.Int("clusters", cp.ClustersDone), zap.Int("proposals", cp.ProposalsCreated), zap.Int("tool_calls", cp.ToolCalls),
		zap.Int("input_tokens", cp.InputTokens), zap.Int("output_tokens", cp.OutputTokens), zap.Float64("cost_rub", cp.CostRub))
}

// prepare — общее начало запуска по кластерам: правила, каталог, песочница, память (решения, предложения,
// «не предлагать», эпизоды).
func (rc *runCtx) prepare(ctx context.Context) error {
	s, rs := rc.a.s, rc.rs
	var err error
	if rc.active, err = s.activeRules(ctx); err != nil {
		return err
	}
	rs.mu.Lock()
	rs.run.RulesVersion = rc.active.Version
	rs.mu.Unlock()
	if rc.cat, err = s.agentCatalog(ctx); err != nil {
		return err
	}
	if rc.sb, err = s.newSandbox(ctx, "agent-"+rs.run.ID.String()[:8], rc.active, rc.a.opts); err != nil {
		return err
	}
	return rc.loadMemory(ctx)
}

// loadMemory — три слоя памяти и ждущие предложения.
func (rc *runCtx) loadMemory(ctx context.Context) error {
	st := rc.a.s.store()
	var err error
	if rc.decisions, err = st.ListDecisions(ctx); err != nil {
		return err
	}
	if rc.proposals, err = st.ListProposals(ctx, "", ""); err != nil {
		return err
	}
	if rc.suppress, err = st.ListSuppress(ctx); err != nil {
		return err
	}
	if rc.episodes, err = st.ListEpisodes(ctx, 300); err != nil {
		return err
	}
	return nil
}

// memoryReadStep — шаг «читаю память» в начале запуска: что агент помнит.
func (rc *runCtx) memoryReadStep(ctx context.Context) {
	rej, acc, active := 0, 0, 0
	for _, d := range rc.decisions {
		switch d.Decision {
		case domain.ProposalRejected:
			rej++
		case domain.ProposalAccepted:
			acc++
		}
	}
	for _, m := range rc.suppress {
		if m.Status == domain.SuppressActive {
			active++
		}
	}
	tool := "get_previous_decisions"
	rc.a.step(ctx, rc.rs, &domain.AgentStep{Kind: domain.StepMemoryRead, Tool: &tool, OK: true,
		Title: fmt.Sprintf("Читаю память: решений %d (принято %d, отклонено %d), «не предлагать» — %d действующих, эпизодов — %d",
			len(rc.decisions), acc, rej, active, len(rc.episodes)),
		Result: toJSON(map[string]any{"decisions": len(rc.decisions), "accepted": acc, "rejected": rej,
			"suppress_active": active, "suppress_total": len(rc.suppress), "episodes": len(rc.episodes)})})
}

// statsSince — начало окна журнала запуска: всплеск — окно триггера, иначе Days суток.
func (rc *runCtx) statsSince() time.Time {
	if !rc.rs.spec.Since.IsZero() {
		return rc.rs.spec.Since
	}
	return rc.a.s.now().Add(-time.Duration(rc.rs.run.Days) * 24 * time.Hour)
}

func (rc *runCtx) run(ctx context.Context) error {
	a, s, rs := rc.a, rc.a.s, rc.rs
	st := s.store()
	if err := rc.prepare(ctx); err != nil {
		return err
	}
	rc.memoryReadStep(ctx)

	// 1. Эффект ранее принятых правок (ночной разбор и разбор по периоду вручную; не всплеск).
	if rs.run.Trigger != domain.TriggerSpike {
		rc.effectCheck(ctx)
	}

	// 2. Неудачные запросы за окно.
	t := time.Now()
	var hold holdoutInfo
	var err error
	if rc.stats, hold, err = s.agentQueryStatsSince(ctx, rc.statsSince(), a.opts.Holdout); err != nil {
		return err
	}
	rc.episode["holdout"] = hold
	rc.failed = rc.stats.failed()
	if len(rs.spec.Focus) > 0 {
		// Всплеск: только запросы, на которые сработал триггер (в порядке триггера — новые первыми).
		in := map[string]bool{}
		for _, f := range rs.spec.Focus {
			in[f] = true
		}
		var kept []clusterQuery
		for _, q := range rc.failed {
			if in[q.Text] {
				kept = append(kept, q)
			}
		}
		for _, f := range rs.spec.Focus {
			if q := rc.stats.byText[f]; q != nil && q.Failures > 0 && !slices.ContainsFunc(kept, func(k clusterQuery) bool { return k.Text == f }) {
				kept = append(kept, *q)
			}
		}
		rc.failed = kept
	}
	searches, fails := 0, 0
	for _, q := range rc.stats.byText {
		searches += q.Freq
		fails += q.Failures
	}
	period := fmt.Sprintf("%d дней", rs.run.Days)
	if !rs.spec.Since.IsZero() {
		period = "окно всплеска " + shortDuration(s.now().Sub(rs.spec.Since).Round(time.Minute))
	} else if rs.run.Days == 1 {
		period = "сутки"
	}
	rc.episode["metrics"] = map[string]any{"days": rs.run.Days, "searches": searches, "failed_searches": fails,
		"distinct_queries": len(rc.stats.byText), "failed_queries": len(rc.failed), "period": period}
	top := rc.failed
	if len(top) > 10 {
		top = top[:10]
	}
	title := fmt.Sprintf("Неудачные запросы за %s: %d из %d различных, %d неудачных поисков из %d",
		period, len(rc.failed), len(rc.stats.byText), fails, searches)
	if hold.HoldoutFrom != nil && hold.HiddenEvents > 0 {
		title += fmt.Sprintf("; неделя %s–%s отложена для проверки эффекта", *hold.HoldoutFrom, *hold.HoldoutTo)
	}
	if hold.TestTexts > 0 {
		title += fmt.Sprintf("; тексты test скрыты (%d)", hold.TestTexts)
	}
	a.toolStep(ctx, rs, nil, "get_failed_queries", map[string]any{"days": rs.run.Days, "since": rc.statsSince().UTC()},
		map[string]any{"searches": searches, "failed_searches": fails, "failed_queries": len(rc.failed), "top": compactQueries(top),
			"holdout": hold}, title, true, time.Since(t))
	if len(rc.failed) == 0 {
		rs.mu.Lock()
		rs.run.StopReason = StopNoFailedQueries
		rs.mu.Unlock()
		return nil
	}

	// 3. Кластеры.
	t = time.Now()
	texts := make([]string, len(rc.failed))
	for i, q := range rc.failed {
		texts[i] = q.Text
	}
	vecs := s.embedTexts(ctx, texts)
	var lx *searchLexicon
	if cs := s.catalog.Load(); cs != nil {
		lx = cs.lex
	}
	var rules *searchRules
	if lx != nil {
		rules = s.rulesFor(ctx, domain.ConfigB4, lx, rc.active.snapshot())
	}
	all := clusterFailed(rc.failed, vecs, lx, rules)
	if len(all) > agentCandidates {
		all = all[:agentCandidates]
	}
	if err := rc.saveClusters(ctx, all); err != nil {
		return err
	}
	cands := rc.candidates(ctx, rc.clusters)
	view := []map[string]any{}
	for i, c := range cands {
		if i >= 15 {
			break
		}
		view = append(view, map[string]any{"cluster_id": c.ID, "label": c.Label, "queries": len(c.c.Queries),
			"terms": c.Terms, "lost_demand": c.c.LostDemand, "freq": c.Freq, "failures": c.Failures, "fail_share": c.FailShare,
			"new": c.New, "score": c.Score})
	}
	how := "общие непонятные слова и близость векторов BGE-M3"
	if vecs == nil {
		how = "общие непонятные слова (эмбеддер недоступен)"
	}
	a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepCheck, OK: true, Ms: int(time.Since(t).Milliseconds()),
		Title:  fmt.Sprintf("Кластеры: %d запросов → %d кластеров (%s); приоритет — частота × доля неудач × «новый»", len(rc.failed), len(all), how),
		Result: toJSON(map[string]any{"clusters": view})})

	// 4. Память и проверка действующим поиском — до плана: в план идут только кластеры, которые есть смысл разбирать.
	var open []planCandidate
	for _, c := range cands {
		if len(open) >= planMaxCandidates || ctx.Err() != nil {
			break
		}
		if rc.memorySkip(ctx, c.c, false) {
			continue
		}
		if rc.alreadySolved(ctx, c.c) {
			continue
		}
		open = append(open, c)
	}
	if len(rc.skipped) > 0 {
		var labels []string
		for _, sk := range rc.skipped {
			labels = append(labels, fmt.Sprintf("«%v»", sk["label"]))
		}
		a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepCheck, OK: true,
			Title: fmt.Sprintf("Пропущено кластеров: %d — действующий поиск понимает все слова и отвечает: %s",
				len(rc.skipped), shortText(strings.Join(labels, ", "), 300)),
			Result: toJSON(map[string]any{"skipped": rc.skipped})})
	}

	// 5. План модели и разбор по плану.
	plan := rc.makePlan(ctx, open, rs.run.MaxClusters, nil)
	return rc.investigatePlan(ctx, plan, open, st)
}

// saveClusters — кластеры запуска в базу и в память запуска.
func (rc *runCtx) saveClusters(ctx context.Context, all []failedCluster) error {
	var rows []domain.IssueCluster
	for i := range all {
		c := &all[i]
		row := domain.IssueCluster{RunID: rc.rs.run.ID, Key: c.Key, Label: c.Label, Terms: toJSON(c.Terms), Queries: toJSON(c.Queries),
			Freq: c.Freq, Failures: c.Failures, LostDemand: c.LostDemand, Rank: i + 1, Status: "new"}
		if err := row.BeforeCreate(nil); err != nil {
			return err
		}
		c.ID = row.ID.String()
		rows = append(rows, row)
		rc.clusters = append(rc.clusters, c)
		rc.byID[c.ID] = c
	}
	if err := rc.a.s.store().SaveIssueClusters(ctx, rows); err != nil {
		return err
	}
	for i := range rows {
		rc.rows[rows[i].ID.String()] = &rows[i]
	}
	return nil
}

// investigatePlan — разбор кластеров плана по порядку. Кластер занят другим запуском — выпадает из плана, его место
// занимает следующий кандидат; бюджет исчерпан — остановка с причиной и списком неразобранного.
func (rc *runCtx) investigatePlan(ctx context.Context, plan *runPlan, open []planCandidate, st agentStore) error {
	a, rs := rc.a, rc.rs
	byID := map[string]planCandidate{}
	for _, c := range open {
		byID[c.ID] = c
	}
	queue := append([]planItem(nil), plan.Clusters...)
	inPlan := map[string]bool{}
	for _, c := range queue {
		inPlan[c.ID] = true
	}
	rs.mu.Lock()
	rs.run.ClustersTotal = len(queue)
	rs.mu.Unlock()
	a.saveRun(ctx, rs)
	var spent []time.Duration
	for qi := 0; qi < len(queue); qi++ {
		item := queue[qi]
		c := byID[item.ID].c
		if c == nil {
			continue
		}
		if ctx.Err() != nil {
			return ctx.Err()
		}
		now := a.s.now()
		if reason := rs.exhausted(now); reason != "" {
			rc.stopBudget(ctx, reason, queue[qi:])
			break
		}
		// Не хватит времени на следующий кластер (по среднему разобранных) — план сокращается заранее.
		if len(spent) > 0 && rs.budget.Duration > 0 {
			var sum time.Duration
			for _, d := range spent {
				sum += d
			}
			avg := sum / time.Duration(len(spent))
			left := rs.budget.Duration - now.Sub(rs.run.StartedAt)
			if left < avg {
				rc.replan(ctx, fmt.Sprintf("до лимита времени %s, кластер в среднем занимает %s — оставшиеся не начинаю",
					left.Round(time.Second), avg.Round(time.Second)), queue[qi:], nil)
				rc.stopBudget(ctx, StopTimeBudget, queue[qi:])
				break
			}
		}
		if dup := rc.sameRunProposal(ctx, c); dup != "" {
			// По похожему кластеру этот же запуск уже положил предложение (кандидаты отбирались до него).
			id := c.ID
			a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepMemorySkip, ClusterID: &id, OK: true,
				Title: fmt.Sprintf("Читаю память → кластер «%s» пропускаю: в этом запуске по похожему кластеру уже есть %s", c.Label, dup)})
			rc.replan(ctx, fmt.Sprintf("«%s» совпал с уже разобранным в этом запуске — не разбираю повторно", c.Label), []planItem{item}, nil)
			rs.mu.Lock()
			rc.outcome.SkippedMemory++
			rs.mu.Unlock()
			continue
		}
		if owner, ok := a.lockCluster(rs.run.ID, c.Key); !ok {
			// Защита от одновременных запусков на одних кластерах: кластер разбирает другой запуск.
			var add []planItem
			for _, o := range open {
				if !inPlan[o.ID] {
					add = append(add, planItem{ID: o.ID, Label: o.Label, Score: o.Score, Why: "вместо занятого кластера"})
					inPlan[o.ID] = true
					break
				}
			}
			rc.replan(ctx, fmt.Sprintf("кластер «%s» уже разбирает запуск %s — пропускаю", c.Label, owner.String()[:8]),
				[]planItem{item}, add)
			queue = append(queue, add...)
			continue
		}
		id := c.ID
		rs.mu.Lock()
		rs.run.CurrentClusterID, rs.run.CurrentLabel = &id, c.Label
		rs.mu.Unlock()
		a.saveRun(ctx, rs)
		started := time.Now()
		res := rc.investigate(ctx, c)
		spent = append(spent, time.Since(started))
		rc.afterCluster(ctx, c, res)
		if errors.Is(res.Err, errTokenLimit) || errors.Is(res.Err, errRunBudget) {
			reason := rs.exhausted(a.s.now())
			if reason == "" {
				reason = StopTokenLimit
			}
			rc.stopBudget(ctx, reason, queue[qi+1:])
			break
		}
		if errors.Is(res.Err, errBudget) {
			rs.mu.Lock()
			rs.run.StopReason = StopBudget
			rs.mu.Unlock()
			rc.markUnprocessed(queue[qi+1:], "дневной бюджет LLM исчерпан")
			break
		}
		if res.Err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if res.Err != nil {
			rs.mu.Lock()
			rs.run.StopReason = StopLLMError
			rs.mu.Unlock()
			rc.markUnprocessed(queue[qi+1:], "сбой модели")
			return res.Err
		}
	}
	rs.mu.Lock()
	rs.run.ClustersTotal = max(rs.run.ClustersTotal, rs.run.ClustersDone)
	rs.mu.Unlock()
	// Честный итог: кластер, упёршийся в лимит вызовов, разобран не до конца — запуск это показывает
	// (step_limit), а не «все кластеры разобраны».
	limited, unfinished := 0, 0
	for _, r := range rc.results {
		switch r["outcome"] {
		case "limit":
			limited++
		case "unfinished":
			unfinished++
		}
	}
	rc.episode["clusters_limited"], rc.episode["clusters_unfinished"] = limited, unfinished
	rs.mu.Lock()
	if rs.run.StopReason == "" && limited > 0 {
		rs.run.StopReason = StopStepLimit
	}
	rs.mu.Unlock()
	return nil
}

// sameRunProposal — предложение этого запуска по кластеру, совпавшему с c (ключ, термин, транслит, вектор).
func (rc *runCtx) sameRunProposal(ctx context.Context, c *failedCluster) string {
	mine := map[string]bool{}
	for _, p := range rc.proposals {
		if p.RunID == rc.rs.run.ID && p.Status == domain.ProposalPending {
			mine[p.ID.String()] = true
		}
	}
	if len(mine) == 0 {
		return ""
	}
	for _, h := range rc.memoryFor(ctx, c.Key, c.Terms) {
		if h.Kind == "proposal" && mine[h.ProposalID] {
			return fmt.Sprintf("предложение %s по «%s»", h.Diagnosis, h.Label)
		}
	}
	return ""
}

// afterCluster — итог кластера: строка кластера, эпизод, счётчики.
func (rc *runCtx) afterCluster(ctx context.Context, c *failedCluster, res clusterResult) {
	st := rc.a.s.store()
	if row := rc.rows[c.ID]; row != nil {
		row.Status, row.Diagnosis, row.Note = "investigated", res.Diagnosis, shortText(res.Note, 500)
		if err := st.SaveIssueCluster(context.WithoutCancel(ctx), row); err != nil {
			zap.L().Warn("агент: кластер не записан", zap.Error(err))
		}
	}
	rc.results = append(rc.results, map[string]any{"cluster_id": c.ID, "label": c.Label, "diagnosis": res.Diagnosis,
		"outcome": res.Outcome, "hypotheses": res.Hypotheses, "rejected": res.Rejected, "tool_calls": res.ToolCalls,
		"proposal_id": res.ProposalID})
	var pid *uuid.UUID
	if u, err := uuid.Parse(res.ProposalID); err == nil {
		pid = &u
	}
	details := map[string]any{"hypotheses": res.Hypotheses, "rejected": res.Rejected, "tool_calls": res.ToolCalls,
		"queries": len(c.Queries), "failures": c.Failures}
	if w := rc.lifted[c.ID]; w != "" {
		details["what_changed"] = w
	}
	rc.writeEpisode(ctx, &domain.MemoryEpisode{ClusterKey: c.Key, ClusterLabel: c.Label, Kind: "investigation",
		Outcome: res.Outcome, Diagnosis: res.Diagnosis, ProposalID: pid, Details: toJSON(details), Note: shortText(res.Note, 400)})
	rc.rs.mu.Lock()
	rc.rs.run.ClustersDone++
	rc.rs.mu.Unlock()
	rc.a.saveRun(ctx, rc.rs)
}

// markUnprocessed — кластеры плана, до которых запуск не дошёл.
func (rc *runCtx) markUnprocessed(items []planItem, reason string) {
	for _, it := range items {
		rc.unprocessed = append(rc.unprocessed, map[string]any{"cluster_id": it.ID, "label": it.Label, "score": it.Score,
			"reason": reason})
	}
}

// stopBudget — бюджет запуска исчерпан: причина остановки, шаг budget со списком неразобранного.
func (rc *runCtx) stopBudget(ctx context.Context, reason string, rest []planItem) {
	rs := rc.rs
	rs.mu.Lock()
	if rs.run.StopReason == "" {
		rs.run.StopReason = reason
	}
	rs.mu.Unlock()
	rc.markUnprocessed(rest, budgetLabels[reason])
	u := rs.used(rc.a.s.now())
	var labels []string
	for _, it := range rest {
		labels = append(labels, "«"+it.Label+"»")
	}
	title := fmt.Sprintf("Бюджет запуска: исчерпан %s (шагов %d из %d, токенов %d из %d, %.2f из %.0f ₽, %d из %d с) — останавливаюсь",
		budgetLabels[reason], u.Steps, rs.budget.Steps, u.Tokens, rs.budget.Tokens, u.CostRub, rs.budget.CostRub, u.Seconds,
		int(rs.budget.Duration.Seconds()))
	if len(labels) > 0 {
		title += "; не разобраны: " + shortText(strings.Join(labels, ", "), 200)
	}
	rc.a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepBudget, OK: false, Title: title,
		Result: toJSON(map[string]any{"reason": reason, "used": u, "budget": budgetView(rs.budget, u, reason),
			"unprocessed": nonNil(rc.unprocessed)})})
}

// toolStep — шаг-вызов инструмента, который делает сам код (обязательные шаги цикла).
func (a *agentService) toolStep(ctx context.Context, rs *runState, clusterID *string, tool string, args, result any,
	title string, ok bool, d time.Duration) *domain.AgentStep {
	t := tool
	st := &domain.AgentStep{Kind: domain.StepToolCall, ClusterID: clusterID, Tool: &t, Title: title,
		Args: toJSON(args), Result: toJSON(result), OK: ok, Ms: int(d.Milliseconds())}
	a.step(ctx, rs, st)
	return st
}

// memoryHits — прошлые решения и предложения, относящиеся к кластеру (ключ или общий термин).
type memoryHit struct {
	Kind       string    `json:"kind"` // decision | proposal
	ProposalID string    `json:"proposal_id"`
	Label      string    `json:"cluster_label"`
	Diagnosis  string    `json:"diagnosis"`
	Status     string    `json:"status"`
	Reason     string    `json:"reason,omitempty"`
	Comment    string    `json:"comment,omitempty"`
	PatchKeys  []string  `json:"patch_keys,omitempty"`
	At         time.Time `json:"at"`
	// Match — как кластер совпал с записью: key | term | translit | layout | phonetic | vector (agent_memory.go).
	Match string `json:"match"`
}

func (rc *runCtx) memoryFor(ctx context.Context, key string, terms []string) []memoryHit {
	type entry struct {
		key   string
		terms []string
		hit   memoryHit
	}
	var entries []entry
	for _, d := range rc.decisions {
		var ts, pk []string
		_ = jsonUnmarshal(d.Terms, &ts)
		_ = jsonUnmarshal(d.PatchKeys, &pk)
		entries = append(entries, entry{d.ClusterKey, ts, memoryHit{Kind: "decision", ProposalID: d.ProposalID.String(),
			Label: d.ClusterLabel, Diagnosis: d.Diagnosis, Status: d.Decision, Reason: d.Reason, Comment: d.Comment,
			PatchKeys: pk, At: d.CreatedAt}})
	}
	for _, p := range rc.proposals {
		if p.Status != domain.ProposalPending {
			continue
		}
		var pk []string
		_ = jsonUnmarshal(p.PatchKeys, &pk)
		entries = append(entries, entry{p.ClusterKey, proposalTerms(&p), memoryHit{Kind: "proposal",
			ProposalID: p.ID.String(), Label: p.ClusterLabel, Diagnosis: p.Diagnosis, Status: p.Status, PatchKeys: pk,
			At: p.CreatedAt}})
	}
	var vecs map[string][]float32
	var hits []memoryHit
	for _, e := range entries {
		m := ""
		switch {
		case e.key != "" && e.key == key:
			m = "key"
		default:
			m = termsMatch(terms, e.terms)
		}
		if m == "" && len(terms) > 0 && len(e.terms) > 0 {
			if vecs == nil {
				all := append([]string(nil), terms...)
				for _, x := range entries {
					all = append(all, x.terms...)
				}
				vecs = rc.memoryVectors(ctx, all)
			}
			if vectorMatch(vecs, terms, e.terms) {
				m = "vector"
			}
		}
		if m != "" {
			e.hit.Match = m
			hits = append(hits, e.hit)
		}
	}
	return hits
}

// rejectedPatchKeys — правки, которые человек отклонил: повторно не предлагаются.
func (rc *runCtx) rejectedPatchKeys() map[string]memoryHit {
	res := map[string]memoryHit{}
	for _, d := range rc.decisions {
		if d.Decision != domain.ProposalRejected || rc.suppressOpen(d.ProposalID.String()) {
			continue
		}
		var pk []string
		_ = jsonUnmarshal(d.PatchKeys, &pk)
		for _, k := range pk {
			res[k] = memoryHit{Kind: "decision", ProposalID: d.ProposalID.String(), Label: d.ClusterLabel, Status: d.Decision,
				Reason: d.Reason, Comment: d.Comment, At: d.CreatedAt}
		}
	}
	return res
}

// rejectLabels — причины отказа по-русски.
var rejectLabels = map[string]string{
	domain.RejectTooBroad:     "слишком широко",
	domain.RejectWrongProduct: "неверный товар",
	domain.RejectNotNow:       "не сейчас",
	domain.RejectOther:        "другое",
}

func commentSuffix(c string) string {
	if c == "" {
		return ""
	}
	return " («" + shortText(c, 80) + "»)"
}

// alreadySolved — действующий поиск (B4) уже честно находит товар по главным запросам кластера.
func (rc *runCtx) alreadySolved(ctx context.Context, c *failedCluster) bool {
	t := time.Now()
	var outs []currentOutcome
	solved := 0
	for i, q := range c.Queries {
		if i >= 2 {
			break
		}
		o, err := rc.a.s.currentSearch(ctx, q.Text, rc.active.snapshot())
		if err != nil {
			return false
		}
		// Все слова кластера поиску известны и выдача непустая — это вопрос ранжирования, а не словаря:
		// старый поиск магазина в логах падал, действующий отвечает. Такие кластеры агент не берёт.
		if !o.Solved && len(c.Terms) == 0 && !o.Empty && len(o.Top) > 0 {
			o.Solved = true
		}
		outs = append(outs, o)
		if o.Solved {
			solved++
		}
	}
	if row := rc.rows[c.ID]; row != nil {
		row.Current = toJSON(outs)
	}
	if solved < len(outs) || len(outs) == 0 {
		return false
	}
	if row := rc.rows[c.ID]; row != nil {
		row.Status, row.Note = "skipped", "действующий поиск понимает все слова и отвечает"
		_ = rc.a.s.store().SaveIssueCluster(context.WithoutCancel(ctx), row)
	}
	rc.skipped = append(rc.skipped, map[string]any{"cluster_id": c.ID, "label": c.Label, "lost_demand": c.LostDemand,
		"current_search": outs, "ms": time.Since(t).Milliseconds()})
	return true
}

// verifyProposal — эффект принятой правки: сейчас находится столько-то запросов кластера.
func (rc *runCtx) verifyProposal(ctx context.Context, p *domain.Proposal) (map[string]any, error) {
	var qs []clusterQuery
	if err := jsonUnmarshal(p.ClusterQueries, &qs); err != nil || len(qs) == 0 {
		return nil, errors.New("нет запросов кластера")
	}
	var patch RulePatch
	_ = jsonUnmarshal(p.Patch, &patch)
	var ev patchEval
	_ = jsonUnmarshal(p.Evaluation, &ev)
	expected := rc.sb.expectedFamilies(patch)
	found, total := 0, 0
	var now []map[string]any
	for _, q := range qs {
		eq, judged := rc.sb.byText[foldQuery(q.Text)]
		if !judged {
			eq = domain.EvalQuery{ID: "verify", Text: q.Text, Freq: q.Freq, Judgments: map[string]int{}}
		}
		r := rc.sb.searchOne(ctx, eq, rc.active.snapshot())
		top := rc.sb.topFamilies(r)
		ok := topExpected(top, expected)
		if judged {
			ok = rc.sb.topRelevant(eq, r)
		}
		total++
		if ok {
			found++
		}
		now = append(now, map[string]any{"text": q.Text, "found": ok, "top": top})
	}
	holds := found >= ev.Target.After.Found
	summary := fmt.Sprintf("до правки находилось %d из %d, прогон обещал %d, сейчас %d", ev.Target.Before.Found, total, ev.Target.After.Found, found)
	eff := map[string]any{"proposal_id": p.ID.String(), "cluster_label": p.ClusterLabel, "config_version": p.ConfigVersion,
		"before": ev.Target.Before, "promised": ev.Target.After, "now": evalCount{Found: found, Total: total},
		"holds": holds, "summary": summary, "rules_version": rc.active.Version, "checked_at": rc.a.s.now().UTC(), "queries": now}
	if st := rc.a.s.store(); st != nil {
		p.Effect = toJSON(eff)
		if err := st.SaveProposal(context.WithoutCancel(ctx), p); err != nil {
			zap.L().Warn("агент: эффект не записан в предложение", zap.Error(err))
		}
		if p.ConfigVersion != nil {
			_ = st.SetConfigVersionEffect(context.WithoutCancel(ctx), *p.ConfigVersion, p.Effect)
		}
	}
	return eff, nil
}

func derefInt(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}

// compactQueries — запросы для результата инструмента.
func compactQueries(qs []clusterQuery) []map[string]any {
	res := make([]map[string]any, 0, len(qs))
	for _, q := range qs {
		res = append(res, map[string]any{"text": q.Text, "freq": q.Freq, "failures": q.Failures, "empty": q.Empty,
			"reformulated": q.Reformulated, "no_click": q.NoClick, "fail_sessions": q.FailSessions})
	}
	return res
}

// successful — запросы с кликами (доля ≥ 30 %, не меньше двух), по убыванию кликов.
func (rc *runCtx) successful(ctx context.Context) ([]clusterQuery, [][]float32) {
	if rc.succDone {
		return rc.succ, rc.succVecs
	}
	rc.succDone = true
	for _, q := range rc.stats.byText {
		// Нераспознанные остатки неверной раскладки как «успешный запрос» агенту не показываем.
		if q.Clicks >= 2 && float64(q.Clicks) >= 0.3*float64(q.Freq) && readableQuery(q.Text) {
			rc.succ = append(rc.succ, *q)
		}
	}
	sort.Slice(rc.succ, func(i, j int) bool {
		if rc.succ[i].Clicks != rc.succ[j].Clicks {
			return rc.succ[i].Clicks > rc.succ[j].Clicks
		}
		return rc.succ[i].Text < rc.succ[j].Text
	})
	texts := make([]string, len(rc.succ))
	for i, q := range rc.succ {
		texts[i] = q.Text
	}
	rc.succVecs = rc.a.s.embedTexts(ctx, texts)
	return rc.succ, rc.succVecs
}

// topClickedFamily — семейство, по которому чаще кликали после запроса.
func (rc *runCtx) topClickedFamily(text string) string {
	best, bestN := "", 0
	for pid, n := range rc.stats.clickedProduct[text] {
		f := rc.cat.familyOfProduct(pid)
		if n > bestN || n == bestN && f < best {
			best, bestN = f, n
		}
	}
	return best
}

func clusterIDPtr(c *failedCluster) *string { id := c.ID; return &id }

// clusterBrief — кластер для модели: только данные.
func (rc *runCtx) clusterBrief(c *failedCluster, memory []memoryHit) map[string]any {
	var current any
	if row := rc.rows[c.ID]; row != nil && len(row.Current) > 0 {
		current = row.Current
	}
	return map[string]any{
		"cluster": map[string]any{"cluster_id": c.ID, "label": c.Label, "lost_demand": c.LostDemand, "freq": c.Freq,
			"failures": c.Failures, "unknown_terms": c.Terms, "queries": c.Queries},
		"current_search": current,
		"memory":         memory,
		"past_episodes":  nonNil(rc.episodesFor(c.Key)),
		"what_changed":   rc.lifted[c.ID],
		"rules_version":  rc.active.Version,
		"limits":         map[string]any{"tool_calls": agentToolCallsPerCluster, "hypotheses": agentMaxHypotheses},
	}
}
