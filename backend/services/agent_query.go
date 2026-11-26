package services

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"kuznica/api/domain"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Запуск на одном запросе («Разобрать» из «Запросов без ответа» или трассы) и проверка каталога
// (триггер catalog_change). Контур автономии.

// errRunBudget — бюджет запуска исчерпан посреди кластера (шаги, стоимость, время).
var errRunBudget = errors.New("бюджет запуска исчерпан")

// StopAlreadySolved — запрос «Разобрать» действующий поиск уже решает: разбирать нечего.
const StopAlreadySolved = "already_solved"

// NoteAlreadySolved — заметка кластера, когда действующий поиск уже отвечает на запрос «Разобрать».
const NoteAlreadySolved = "действующий поиск отвечает точно"

// queryLookbackDays — за сколько дней искать запрос «Разобрать» в журнале (частота, переформулировки).
const queryLookbackDays = 30

// lockCluster — кластер берёт запуск runID; занят другим запуском — его id и false.
func (a *agentService) lockCluster(runID uuid.UUID, key string) (uuid.UUID, bool) {
	a.mu.Lock()
	defer a.mu.Unlock()
	if owner, ok := a.locks[key]; ok && owner != runID {
		if _, alive := a.runs[owner]; alive {
			return owner, false
		}
	}
	a.locks[key] = runID
	return runID, true
}

// runQuery — запуск на одном запросе: запрос из журнала (если был) или новый, память, действующий поиск, план, разбор.
func (rc *runCtx) runQuery(ctx context.Context) error {
	a, s, rs := rc.a, rc.a.s, rc.rs
	if err := rc.prepare(ctx); err != nil {
		return err
	}
	rc.memoryReadStep(ctx)
	query := rs.spec.Query
	t := time.Now()
	stats, hold, err := s.agentQueryStatsSince(ctx, s.now().Add(-queryLookbackDays*24*time.Hour), a.opts.Holdout)
	if err != nil {
		return err
	}
	rc.stats = stats
	rc.episode["holdout"] = hold
	norm := foldQuery(query)
	cq, inLog := clusterQuery{Text: norm}, false
	if q := stats.byText[norm]; q != nil {
		cq, inLog = *q, true
	}
	var lx *searchLexicon
	if cs := s.catalog.Load(); cs != nil {
		lx = cs.lex
	}
	var rules *searchRules
	if lx != nil {
		rules = s.rulesFor(ctx, domain.ConfigB4, lx, rc.active.snapshot())
	}
	all := clusterFailed([]clusterQuery{cq}, nil, lx, rules)
	if len(all) == 0 {
		all = []failedCluster{{Key: norm, Label: norm, Queries: []clusterQuery{cq}}}
	}
	all[0].Label = query
	if err := rc.saveClusters(ctx, all[:1]); err != nil {
		return err
	}
	c := rc.clusters[0]
	window := agentWindowPhrase(hold, queryLookbackDays)
	title := fmt.Sprintf("Запрос «%s»: %s — %d %s, неудачных %d (пусто %d, переформулировок %d)",
		query, window, cq.Freq, plural(cq.Freq, [3]string{"поиск", "поиска", "поисков"}), cq.Failures, cq.Empty, cq.Reformulated)
	if !inLog {
		title = fmt.Sprintf("Запрос «%s»: %s его нет — разбираю как новый", query, window)
	}
	if len(c.Terms) > 0 {
		title += "; непонятные поиску слова: " + strings.Join(c.Terms, ", ")
	}
	a.toolStep(ctx, rs, clusterIDPtr(c), "get_failed_queries", map[string]any{"query": query},
		map[string]any{"in_log": inLog, "query": compactQueries([]clusterQuery{cq}), "terms": nonNil(c.Terms)}, title, true, time.Since(t))
	rc.episode["metrics"] = map[string]any{"query": query, "in_log": inLog, "freq": cq.Freq, "failures": cq.Failures}

	rc.memorySkip(ctx, c, true)
	t = time.Now()
	o, err := s.currentSearch(ctx, query, rc.active.snapshot())
	if err == nil {
		if row := rc.rows[c.ID]; row != nil {
			row.Current = toJSON([]currentOutcome{o})
		}
		var tops []string
		for _, tp := range o.Top {
			tops = append(tops, tp.Title)
		}
		st := fmt.Sprintf("Действующий поиск: %d результатов, первым — %s", o.Total, strings.Join(tops, "; "))
		if o.Empty || len(o.Top) == 0 {
			st = "Действующий поиск: пустая выдача или «нет в каталоге»"
		}
		tool := "get_search_trace"
		a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepToolCall, ClusterID: clusterIDPtr(c), Tool: &tool, OK: true,
			Title: shortText(st, 300), Args: toJSON(map[string]any{"query": query}), Result: toJSON(o),
			Ms: int(time.Since(t).Milliseconds())})
		if o.OnRequest && len(o.Top) > 0 {
			// Контур фида: найдено, но купить нельзя — это не «поиск уже отвечает»: разбор идёт, итог — задача каталогу.
			c.OnRequestFamily = o.Top[0].FamilyID
			a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepCheck, ClusterID: clusterIDPtr(c), OK: false,
				Title: fmt.Sprintf("Поиск находит «%s» первым, но купить нельзя: только по запросу — разбираю, правка поиска тут не поможет", o.Top[0].Title)})
		}
		if o.Solved && !o.OnRequest {
			a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepCheck, ClusterID: clusterIDPtr(c), OK: true,
				Title: fmt.Sprintf("Запрос решён действующим поиском: «%s» — точное совпадение первым. Разбирать нечего — возьмите запрос из «Запросов без ответа»", o.Top[0].Title)})
			rs.mu.Lock()
			rs.run.StopReason, rs.run.ClustersTotal = StopAlreadySolved, 1
			rs.mu.Unlock()
			rc.afterCluster(ctx, c, clusterResult{Outcome: "solved", Diagnosis: "", Note: NoteAlreadySolved})
			return nil
		}
	}
	cands := rc.candidates(ctx, rc.clusters)
	plan := rc.makePlan(ctx, cands, 1, nil)
	if len(plan.Clusters) == 0 {
		// Один запрос — один кластер: модель не может «не взять» запрос, который человек попросил разобрать.
		rc.replan(ctx, "запуск на одном запросе — кластер в плане обязательно", nil,
			[]planItem{{ID: c.ID, Label: c.Label, Score: cands[0].Score, Why: "запрос человека"}})
	}
	return rc.investigatePlan(ctx, rc.plan, cands, s.store())
}

// ── Проверка каталога (catalog_change) ──

var catalogChecks = []string{"rules", "suggestions", "relations"}

// runCatalogChange — новая версия каталога или справочника связей: план модели по проверкам, затем правила
// (алиасы ведут на существующие цели), подсказки (сборка того же каталога, категории и производители есть,
// подсказка находит хоть одно семейство) и связи (режим «Коммерция»: BROKEN_RELATION, MISSING_REQUIRED).
func (rc *runCtx) runCatalogChange(ctx context.Context) error {
	s, rs := rc.a.s, rc.rs
	var err error
	if rc.active, err = s.activeRules(ctx); err != nil {
		return err
	}
	if rc.cat, err = s.agentCatalog(ctx); err != nil {
		return err
	}
	if err := rc.loadMemory(ctx); err != nil {
		return err
	}
	rc.memoryReadStep(ctx)
	plan := rc.makePlan(ctx, nil, 0, catalogChecks)
	metrics := map[string]any{}
	for _, ck := range plan.Checks {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		if reason := rs.exhausted(s.now()); reason != "" {
			rc.stopBudget(ctx, reason, nil)
			return nil
		}
		switch ck {
		case "rules":
			metrics["rules"] = rc.checkRules(ctx)
		case "suggestions":
			metrics["suggestions"] = rc.checkSuggestions(ctx)
		case "relations":
			rs.mu.Lock()
			rs.run.MaxClusters = max(rs.run.MaxClusters, 1)
			rs.mu.Unlock()
			err := rc.runCommerce(ctx)
			if m, ok := rc.episode["metrics"].(map[string]any); ok {
				metrics["relations"] = m
			}
			if err != nil {
				rc.episode["metrics"] = metrics
				return err
			}
		}
	}
	rc.episode["metrics"] = metrics
	rs.mu.Lock()
	if rs.run.StopReason == StopNoFindings && (rc.outcome.Tasks > 0 || rc.outcome.Stale > 0) {
		rs.run.StopReason = ""
	}
	rs.mu.Unlock()
	return nil
}

// checkRules — алиасы действующих правил ведут на цели, которые есть в каталоге.
func (rc *runCtx) checkRules(ctx context.Context) map[string]any {
	t := time.Now()
	var stale []map[string]any
	for _, al := range rc.active.Aliases {
		ok := true
		switch al.Target {
		case "family":
			ok = rc.cat.families[al.TargetID] != nil
		case "vendor":
			_, ok = rc.cat.vendors[al.TargetID]
		case "category":
			_, ok = rc.cat.categories[al.TargetID]
		}
		if al.TargetID == "" {
			ok = true
		}
		if !ok {
			stale = append(stale, map[string]any{"alias": al.Alias, "canonical": al.Canonical, "target": al.Target, "target_id": al.TargetID})
		}
	}
	res := map[string]any{"checked": len(rc.active.Aliases), "stale": nonNil(stale), "rules_version": rc.active.Version}
	rc.a.toolStep(ctx, rc.rs, nil, "check_rules", map[string]any{"rules_version": rc.active.Version}, res,
		fmt.Sprintf("Проверяю правила v%d: алиасов %d, ведут на несуществующую цель — %d", rc.active.Version, len(rc.active.Aliases), len(stale)),
		len(stale) == 0, time.Since(t))
	rc.rs.mu.Lock()
	rc.outcome.Checked += len(rc.active.Aliases)
	rc.outcome.Stale += len(stale)
	rc.rs.mu.Unlock()
	if len(stale) > 0 {
		var names []string
		for _, x := range stale {
			names = append(names, fmt.Sprintf("«%v»", x["alias"]))
		}
		rc.catalogTask(ctx, domain.DiagAliasGap, "Правила после смены каталога",
			fmt.Sprintf("После смены каталога %d алиасов ведут на цели, которых больше нет: %s. Нужна правка словаря: убрать или перенаправить.",
				len(stale), shortText(strings.Join(names, ", "), 300)),
			"catalog:rules:"+rc.catalogVersion, map[string]any{"stale_aliases": stale})
	}
	return res
}

// checkSuggestions — подсказки панели: сборка того же каталога, категории и производители подсказок есть в
// каталоге, самые частые подсказки находят хоть одно семейство (текстовая ветка поиска).
func (rc *runCtx) checkSuggestions(ctx context.Context) map[string]any {
	t := time.Now()
	s := rc.a.s
	cs := s.catalog.Load()
	if cs == nil || cs.lex == nil {
		return map[string]any{"skipped": "каталог не загружен"}
	}
	d := s.suggestDictFor(cs.lex, cs.version)
	if d == nil {
		rc.a.toolStep(ctx, rc.rs, nil, "check_suggestions", map[string]any{}, map[string]any{"skipped": true},
			"Проверяю подсказки: словарь подсказок ещё собирается — проверка пропущена", true, time.Since(t))
		return map[string]any{"skipped": "словарь собирается"}
	}
	rules := s.rulesFor(ctx, domain.ConfigB4, cs.lex, rc.active.snapshot())
	const top, hitsTop = 240, 40
	var stale []map[string]any
	checked := 0
	for i, idx := range d.popular {
		if i >= top || ctx.Err() != nil {
			break
		}
		e := d.entries[idx]
		checked++
		why := ""
		for _, c := range e.categories {
			if _, ok := rc.cat.categories[c]; !ok {
				why = "категории " + c + " нет в каталоге"
			}
		}
		for _, v := range e.vendors {
			if _, ok := rc.cat.vendors[v]; !ok && why == "" {
				why = "производителя " + v + " нет в каталоге"
			}
		}
		if why == "" && i < hitsTop && e.kind != suggestCategory {
			if fams, err := s.textFamilies(ctx, cs.lex, rules, e.text); err == nil && len(fams) == 0 {
				why = "подсказка не находит ни одного семейства"
			}
		}
		if why != "" {
			stale = append(stale, map[string]any{"text": e.text, "why": why})
		}
	}
	buildStale := d.catalog != "" && d.catalog != cs.version
	res := map[string]any{"checked": checked, "stale": nonNil(stale), "build_catalog": d.catalog, "catalog": cs.version,
		"build_stale": buildStale}
	title := fmt.Sprintf("Проверяю подсказки: проверено %d, устаревших %d", checked, len(stale))
	if buildStale {
		title += fmt.Sprintf("; сборка подсказок — для каталога %s, действующий %s", d.catalog, cs.version)
	}
	rc.a.toolStep(ctx, rc.rs, nil, "check_suggestions", map[string]any{"top": top}, res, title, len(stale) == 0 && !buildStale, time.Since(t))
	rc.rs.mu.Lock()
	rc.outcome.Checked += checked
	rc.outcome.Stale += len(stale)
	rc.rs.mu.Unlock()
	if len(stale) > 0 || buildStale {
		var names []string
		for i, x := range stale {
			if i >= 8 {
				break
			}
			names = append(names, fmt.Sprintf("«%v»", x["text"]))
		}
		text := fmt.Sprintf("После смены каталога устарело подсказок: %d из %d проверенных (%s). Пересобрать индекс подсказок (go run . suggest-build).",
			len(stale), checked, shortText(strings.Join(names, ", "), 300))
		rc.catalogTask(ctx, "SUGGESTION_DEGRADED", "Подсказки после смены каталога", text,
			"catalog:suggest:"+cs.version, map[string]any{"stale_suggestions": stale, "build_stale": buildStale})
	}
	return res
}

// catalogTask — задача владельцу по итогам проверки каталога (не правка); повторно по тому же ключу не ставится.
func (rc *runCtx) catalogTask(ctx context.Context, diagnosis, label, text, key string, evidence map[string]any) {
	for _, p := range rc.proposals {
		var pk []string
		_ = jsonUnmarshal(p.PatchKeys, &pk)
		for _, k := range pk {
			if k == key && (p.Status == domain.ProposalPending || p.Status == domain.ProposalRejected) {
				rc.a.step(ctx, rc.rs, &domain.AgentStep{Kind: domain.StepMemoryRead, OK: true,
					Title: fmt.Sprintf("Память: задача «%s» уже есть (%s) — не повторяю", label, statusLabel(p.Status))})
				return
			}
		}
	}
	evidence["task_to_owner"] = true
	evidence["catalog_version"] = rc.catalogVersion
	p := &domain.Proposal{RunID: rc.rs.run.ID, ClusterKey: key, ClusterLabel: label, ClusterQueries: toJSON([]clusterQuery{}),
		Diagnosis: diagnosis, DiagnosisText: text, Role: OwnerOf(diagnosis), Action: domain.ActionTicket,
		PatchKeys: toJSON([]string{key}), Evidence: toJSON(evidence), BaseVersion: rc.active.Version, Status: domain.ProposalPending}
	if rc.rs.run.ReadOnly {
		rc.draft(ctx, p, "Только чтение — задача не отправлена: "+text)
		return
	}
	if err := rc.a.s.store().CreateProposal(context.WithoutCancel(ctx), p); err != nil {
		zap.L().Warn("агент: задача проверки каталога не записана", zap.Error(err))
		return
	}
	rc.proposals = append(rc.proposals, *p)
	rc.rs.mu.Lock()
	rc.rs.run.ProposalsCreated++
	rc.outcome.Tasks++
	rc.rs.mu.Unlock()
	rc.a.step(ctx, rc.rs, &domain.AgentStep{Kind: domain.StepProposal, OK: true,
		Title:  fmt.Sprintf("Задача %s: %s", ownerDative(OwnerOf(diagnosis)), shortText(text, 240)),
		Result: toJSON(map[string]any{"proposal_id": p.ID.String(), "action": p.Action, "task_to_owner": true, "role": p.Role})})
	rc.writeEpisode(ctx, &domain.MemoryEpisode{ClusterKey: key, ClusterLabel: label, Kind: "investigation", Outcome: "ticket",
		Diagnosis: diagnosis, ProposalID: &p.ID, Note: shortText(text, 400)})
}

// ownerDative — «кому задача».
func ownerDative(owner string) string {
	return map[string]string{RoleProduct: "категорийному менеджеру", RoleContent: "каталогу",
		RoleEcom: "администратору поиска", RoleHead: "руководителю e-com"}[owner]
}

// agentWindowPhrase — какую часть журнала видит агент (P8): при отложенной неделе — честные даты окна и недели,
// иначе — «за N дней». Логику окна не меняет, только описывает holdoutInfo.
func agentWindowPhrase(h holdoutInfo, days int) string {
	if !h.Enabled || h.HoldoutFrom == nil || h.HoldoutTo == nil {
		return fmt.Sprintf("в журнале за %d %s", days, plural(days, [3]string{"день", "дня", "дней"}))
	}
	hf, err1 := time.ParseInLocation("2006-01-02", *h.HoldoutFrom, mskZone)
	ht, err2 := time.ParseInLocation("2006-01-02", *h.HoldoutTo, mskZone)
	if err1 != nil || err2 != nil {
		return fmt.Sprintf("в журнале за %d %s", days, plural(days, [3]string{"день", "дня", "дней"}))
	}
	week := fmt.Sprintf("неделя %s–%s отложена для проверки и агенту не видна", hf.Format("02.01"), ht.Format("02.01"))
	from := h.SeenFrom.In(mskZone)
	var seen string
	if h.SeenTo.After(hf) {
		// Окно после отложенной недели (ночной разбор, всплеск).
		seen = "с " + from.Format("02.01")
	} else {
		seen = fmt.Sprintf("с %s по %s", from.Format("02.01"), h.SeenTo.Add(-time.Second).In(mskZone).Format("02.01"))
	}
	extra := ""
	if h.ExcludeTest && h.TestTexts > 0 {
		extra = "; запросы проверочного набора test тоже скрыты"
	}
	return fmt.Sprintf("в части журнала, которую видит агент (%s; %s%s),", seen, week, extra)
}
