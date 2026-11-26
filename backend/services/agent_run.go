package services

import (
	"context"
	"errors"
	"fmt"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"

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
}

// execute — запуск целиком; любой исход записывается в статус запуска.
func (a *agentService) execute(ctx context.Context, rs *runState) {
	rc := &runCtx{a: a, rs: rs, byID: map[string]*failedCluster{}, rows: map[string]*domain.IssueCluster{},
		episode: map[string]any{}}
	err := rc.run(ctx)
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
	r.Summary += fmt.Sprintf("Разобрано кластеров: %d из %d, предложений в очереди: %d, вызовов инструментов: %d, %d токенов, %.2f ₽",
		r.ClustersDone, r.ClustersTotal, r.ProposalsCreated, r.ToolCalls, r.InputTokens+r.OutputTokens, r.CostRub)
	if n, _ := rc.episode["clusters_limited"].(int); n > 0 {
		r.Summary += fmt.Sprintf(". Упёрлись в лимит %d вызовов на кластер: %d", agentToolCallsPerCluster, n)
	}
	if n, _ := rc.episode["clusters_unfinished"].(int); n > 0 {
		r.Summary += fmt.Sprintf(". Без итога (модель не вызвала завершающий инструмент): %d", n)
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

func (rc *runCtx) run(ctx context.Context) error {
	a, s, rs := rc.a, rc.a.s, rc.rs
	st := s.store()
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
	if rc.sb, err = s.newSandbox(ctx, "agent-"+rs.run.ID.String()[:8], rc.active, a.opts); err != nil {
		return err
	}
	if rc.decisions, err = st.ListDecisions(ctx); err != nil {
		return err
	}
	if rc.proposals, err = st.ListProposals(ctx, "", ""); err != nil {
		return err
	}

	// 1. Эффект ранее принятых правок (память: эпизоды).
	rc.verifyAccepted(ctx)

	// 2. Неудачные запросы за период.
	t := time.Now()
	var hold holdoutInfo
	if rc.stats, hold, err = s.agentQueryStats(ctx, rs.run.Days, a.opts.Holdout); err != nil {
		return err
	}
	rc.episode["holdout"] = hold
	rc.failed = rc.stats.failed()
	searches, fails := 0, 0
	for _, q := range rc.stats.byText {
		searches += q.Freq
		fails += q.Failures
	}
	rc.episode["metrics"] = map[string]any{"days": rs.run.Days, "searches": searches, "failed_searches": fails,
		"distinct_queries": len(rc.stats.byText), "failed_queries": len(rc.failed)}
	top := rc.failed
	if len(top) > 10 {
		top = top[:10]
	}
	title := fmt.Sprintf("Неудачные запросы за %d дней: %d из %d различных, %d неудачных поисков из %d",
		rs.run.Days, len(rc.failed), len(rc.stats.byText), fails, searches)
	if hold.HoldoutFrom != nil {
		title += fmt.Sprintf("; неделя %s–%s отложена для проверки эффекта", *hold.HoldoutFrom, *hold.HoldoutTo)
	}
	if hold.TestTexts > 0 {
		title += fmt.Sprintf("; тексты test скрыты (%d)", hold.TestTexts)
	}
	a.toolStep(ctx, rs, nil, "get_failed_queries", map[string]any{"days": rs.run.Days},
		map[string]any{"searches": searches, "failed_searches": fails, "failed_queries": len(rc.failed), "top": compactQueries(top),
			"holdout": hold}, title, true, time.Since(t))
	if len(rc.failed) == 0 {
		rs.mu.Lock()
		rs.run.StopReason = StopNoFailedQueries
		rs.mu.Unlock()
		return nil
	}

	// 3. Кластеры по потерянному спросу.
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
	var rows []domain.IssueCluster
	for i := range all {
		c := &all[i]
		row := domain.IssueCluster{RunID: rs.run.ID, Key: c.Key, Label: c.Label, Terms: toJSON(c.Terms), Queries: toJSON(c.Queries),
			Freq: c.Freq, Failures: c.Failures, LostDemand: c.LostDemand, Rank: i + 1, Status: "new"}
		if err := row.BeforeCreate(nil); err != nil {
			return err
		}
		c.ID = row.ID.String()
		rows = append(rows, row)
		rc.clusters = append(rc.clusters, c)
		rc.byID[c.ID] = c
	}
	if err := st.SaveIssueClusters(ctx, rows); err != nil {
		return err
	}
	for i := range rows {
		rc.rows[rows[i].ID.String()] = &rows[i]
	}
	view := []map[string]any{}
	for i, c := range rc.clusters {
		if i >= 15 {
			break
		}
		view = append(view, map[string]any{"cluster_id": c.ID, "label": c.Label, "queries": len(c.Queries),
			"terms": c.Terms, "lost_demand": c.LostDemand})
	}
	how := "общие непонятные слова и близость векторов BGE-M3"
	if vecs == nil {
		how = "общие непонятные слова (эмбеддер недоступен)"
	}
	a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepCheck, OK: true, Ms: int(time.Since(t).Milliseconds()),
		Title:  fmt.Sprintf("Кластеры: %d запросов → %d кластеров (%s), по убыванию потерянного спроса", len(rc.failed), len(all), how),
		Result: toJSON(map[string]any{"clusters": view})})

	// 4. Выбор: память и проверка действующим поиском.
	var picked []*failedCluster
	for _, c := range rc.clusters {
		if len(picked) >= rs.run.MaxClusters || ctx.Err() != nil {
			break
		}
		if skip := rc.memorySkip(ctx, c); skip {
			continue
		}
		if rc.alreadySolved(ctx, c) {
			continue
		}
		picked = append(picked, c)
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
	var pickedView []map[string]any
	for _, c := range picked {
		pickedView = append(pickedView, map[string]any{"cluster_id": c.ID, "label": c.Label, "lost_demand": c.LostDemand, "terms": c.Terms})
	}
	a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepThought, OK: true,
		Title:  fmt.Sprintf("Беру в разбор кластеров: %d (по убыванию потерянного спроса)", len(picked)),
		Result: toJSON(map[string]any{"clusters": nonNil(pickedView)})})
	rs.mu.Lock()
	rs.run.ClustersTotal = len(picked)
	rs.mu.Unlock()
	a.saveRun(ctx, rs)

	// 5. Расследование кластеров.
	for _, c := range picked {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if rs.tokens() >= agentTokensPerRun {
			rs.mu.Lock()
			rs.run.StopReason = StopTokenLimit
			rs.mu.Unlock()
			break
		}
		id := c.ID
		rs.mu.Lock()
		rs.run.CurrentClusterID, rs.run.CurrentLabel = &id, c.Label
		rs.mu.Unlock()
		a.saveRun(ctx, rs)
		res := rc.investigate(ctx, c)
		if row := rc.rows[c.ID]; row != nil {
			row.Status, row.Diagnosis, row.Note = "investigated", res.Diagnosis, shortText(res.Note, 500)
			if err := st.SaveIssueCluster(context.WithoutCancel(ctx), row); err != nil {
				zap.L().Warn("агент: кластер не записан", zap.Error(err))
			}
		}
		rc.results = append(rc.results, map[string]any{"cluster_id": c.ID, "label": c.Label, "diagnosis": res.Diagnosis,
			"outcome": res.Outcome, "hypotheses": res.Hypotheses, "rejected": res.Rejected, "tool_calls": res.ToolCalls,
			"proposal_id": res.ProposalID})
		rs.mu.Lock()
		rs.run.ClustersDone++
		rs.mu.Unlock()
		a.saveRun(ctx, rs)
		if errors.Is(res.Err, errTokenLimit) {
			rs.mu.Lock()
			rs.run.StopReason = StopTokenLimit
			rs.mu.Unlock()
			break
		}
		if errors.Is(res.Err, errBudget) {
			rs.mu.Lock()
			rs.run.StopReason = StopBudget
			rs.mu.Unlock()
			break
		}
		if res.Err != nil && ctx.Err() != nil {
			return ctx.Err()
		}
		if res.Err != nil {
			rs.mu.Lock()
			rs.run.StopReason = StopLLMError
			rs.mu.Unlock()
			return res.Err
		}
	}
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
		if d.Decision != domain.ProposalRejected {
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

// memorySkip — кластер не разбирается: предложение уже ждёт человека или человек отклонил его не из-за
// ширины правки (тогда ту же проблему не предлагаем снова). Отказ «слишком широко» не закрывает кластер:
// агент ищет правку уже, но отклонённую правку не повторит (код не пропустит).
func (rc *runCtx) memorySkip(ctx context.Context, c *failedCluster) bool {
	hits := rc.memoryFor(ctx, c.Key, c.Terms)
	if len(hits) == 0 {
		return false
	}
	id := c.ID
	for _, h := range hits {
		var title string
		switch {
		case h.Kind == "proposal":
			title = fmt.Sprintf("Память: по кластеру «%s» уже есть предложение %s, оно ждёт решения — пропускаю", c.Label, h.Diagnosis)
		case h.Status == domain.ProposalRejected && h.Reason != domain.RejectTooBroad:
			title = fmt.Sprintf("Память: похожее предложение «%s» отклонено %s, причина: %s%s — не предлагаю",
				h.Label, h.At.Format("02.01"), rejectLabels[h.Reason], commentSuffix(h.Comment))
		default:
			continue
		}
		rc.a.step(ctx, rc.rs, &domain.AgentStep{Kind: domain.StepMemory, ClusterID: &id, OK: true, Title: title,
			Result: toJSON(map[string]any{"hits": hits})})
		if row := rc.rows[c.ID]; row != nil {
			row.Status, row.Note = "skipped", title
			_ = rc.a.s.store().SaveIssueCluster(context.WithoutCancel(ctx), row)
		}
		return true
	}
	return false
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

// verifyAccepted — verify_applied_change по всем принятым правкам: целевой кластер на действующих правилах
// сейчас против цифр прогона при принятии. Эффект — в предложение, версию правил и эпизод запуска.
func (rc *runCtx) verifyAccepted(ctx context.Context) {
	var effects []map[string]any
	for i := range rc.proposals {
		p := &rc.proposals[i]
		if p.Status != domain.ProposalAccepted || p.Action != domain.ActionRulesPatch {
			continue
		}
		t := time.Now()
		eff, err := rc.verifyProposal(ctx, p)
		if err != nil {
			continue
		}
		effects = append(effects, eff)
		ok, _ := eff["holds"].(bool)
		rc.a.toolStep(ctx, rc.rs, nil, "verify_applied_change", map[string]any{"proposal_id": p.ID.String()}, eff,
			fmt.Sprintf("Эффект принятой правки «%s» (v%d): %s", p.ClusterLabel, derefInt(p.ConfigVersion), eff["summary"]), ok, time.Since(t))
	}
	if len(effects) > 0 {
		rc.episode["applied_effects"] = effects
	}
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
		"rules_version":  rc.active.Version,
		"limits":         map[string]any{"tool_calls": agentToolCallsPerCluster, "hypotheses": agentMaxHypotheses},
	}
}
