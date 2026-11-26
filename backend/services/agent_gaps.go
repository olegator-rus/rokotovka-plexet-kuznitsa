package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Пробелы подбора в потоке агента SearchOps. Подбор пишет событие missing_fact, когда в базе
// знаний нет факта для ответа (guide_turns.gap_items). Код запуска превращает такие вопросы в кластеры с причиной
// kb_missing (диагноз KB_MISSING_FACT) и задачей владельцу причины — без модели: причину ставит событие, а не агент
// (модели этот диагноз не доступен). Память — как у задач агента: открытая или принятая задача по тому же вопросу —
// повторно не создаётся, отклонённая человеком — тоже. Запускается в разборе по периоду (после проверки эффекта) и
// отдельным запуском mode=gaps («Создать задачу каталогу» в отчёте SearchOps).

const (
	// gapMissingFact — событие подбора «в базе знаний нет факта».
	gapMissingFact = "missing_fact"
	// agentGapTickets — задач по пробелам за один запуск.
	agentGapTickets = 10
)

// gapKey — ключ кластера пробела: нормализованный вопрос.
func gapKey(question string) string { return "kb:" + foldQuery(question) }

// gapPatchKey — ключ памяти задачи по пробелу.
func gapPatchKey(question string) string {
	return "ticket:" + domain.DiagKBMissingFact + ":" + gapKey(question)
}

// runGaps — запуск mode=gaps: только пробелы подбора, без модели.
func (rc *runCtx) runGaps(ctx context.Context) error {
	var err error
	if rc.active, err = rc.a.s.activeRules(ctx); err != nil {
		return err
	}
	rc.rs.mu.Lock()
	rc.rs.run.RulesVersion = rc.active.Version
	rc.rs.mu.Unlock()
	if err := rc.loadMemory(ctx); err != nil {
		return err
	}
	rc.memoryReadStep(ctx)
	return rc.gapsStep(ctx)
}

// gapsStep — вопросы, на которые подбор не ответил из-за отсутствия факта: кластеры kb_missing и задачи владельцу.
func (rc *runCtx) gapsStep(ctx context.Context) error {
	a, s, rs := rc.a, rc.a.s, rc.rs
	days := rs.run.Days
	if days < 1 {
		days = 7
	}
	t := time.Now()
	rep, err := s.GuideGaps(ctx, days)
	if err != nil {
		return err
	}
	var gaps []out.GuideGapRow
	for _, r := range rep.Rows {
		if r.Code == gapMissingFact {
			gaps = append(gaps, r)
		}
	}
	tool := "get_guide_gaps"
	a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepToolCall, Tool: &tool, OK: true, Ms: int(time.Since(t).Milliseconds()),
		Title: fmt.Sprintf("Пробелы подбора за %d дн.: вопросов без ответа — %d, из них без факта в базе знаний — %d", days,
			len(rep.Rows), len(gaps)),
		Args:   toJSON(map[string]any{"days": days}),
		Result: toJSON(map[string]any{"total": rep.Total, "rows": len(rep.Rows), "missing_fact": len(gaps)})})
	open := map[string]string{}
	for _, p := range rc.proposals {
		if p.Diagnosis != domain.DiagKBMissingFact || p.Status == domain.ProposalStale {
			continue
		}
		var keys []string
		_ = jsonUnmarshal(p.PatchKeys, &keys)
		for _, k := range keys {
			open[k] = p.Status
		}
	}
	for _, d := range rc.decisions {
		if d.Diagnosis != domain.DiagKBMissingFact || d.Decision != domain.ProposalRejected {
			continue
		}
		var keys []string
		_ = jsonUnmarshal(d.PatchKeys, &keys)
		for _, k := range keys {
			open[k] = domain.ProposalRejected
		}
	}
	made := 0
	for _, g := range gaps {
		if ctx.Err() != nil {
			return ctx.Err()
		}
		pk := gapPatchKey(g.Question)
		if st, ok := open[pk]; ok {
			label := map[string]string{domain.ProposalRejected: "человек отклонил задачу", domain.ProposalAccepted: "задача уже принята",
				domain.ProposalPending: "задача уже ждёт владельца"}[st]
			if label == "" {
				label = "задача уже есть"
			}
			a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepMemorySkip, OK: true,
				Title: fmt.Sprintf("Читаю память → «%s»: %s — не создаю повторно", shortText(g.Question, 80), label)})
			rs.mu.Lock()
			rc.outcome.SkippedMemory++
			rs.mu.Unlock()
			continue
		}
		if made >= agentGapTickets {
			rc.unprocessed = append(rc.unprocessed, map[string]any{"label": g.Question, "reason": "лимит задач по пробелам за запуск"})
			continue
		}
		if err := rc.gapTicket(ctx, g, pk); err != nil {
			return err
		}
		open[pk] = domain.ProposalPending
		made++
	}
	return nil
}

// gapTicket — кластер kb_missing из одного вопроса и задача владельцу причины.
func (rc *runCtx) gapTicket(ctx context.Context, g out.GuideGapRow, patchKey string) error {
	a, rs, st := rc.a, rc.rs, rc.a.s.store()
	var names, ids []string
	for _, f := range g.Families {
		ids = append(ids, f.FamilyID)
		if f.Title != "" {
			names = append(names, "«"+f.Title+"»")
		}
	}
	queries := []clusterQuery{{Text: g.Question, Freq: g.Count, Failures: g.Count, FailSessions: g.Count}}
	row := domain.IssueCluster{RunID: rs.run.ID, Key: gapKey(g.Question), Label: shortText(g.Question, 120),
		Terms: toJSON([]string{}), Queries: toJSON(queries), Freq: g.Count, Failures: g.Count, LostDemand: float64(g.Count),
		Rank: len(rc.rows) + 1, Status: "investigated", Diagnosis: domain.DiagKBMissingFact,
		Note: "подбор не ответил: в базе знаний нет факта"}
	if err := row.BeforeCreate(nil); err != nil {
		return err
	}
	if err := st.SaveIssueClusters(ctx, []domain.IssueCluster{row}); err != nil {
		return err
	}
	rc.rows[row.ID.String()] = &row
	id := row.ID.String()
	about := strings.Join(names, ", ")
	if about == "" {
		about = "продукт из вопроса"
	}
	cause := CauseOf(domain.DiagKBMissingFact)
	text := fmt.Sprintf("Подбор не ответил на вопрос «%s» (%d %s): в базе знаний нет факта о %s. Нужно добавить факт с "+
		"источником — тогда подбор ответит на такие вопросы.", shortText(g.Question, 160), g.Count,
		pluralRu(g.Count, "раз", "раза", "раз"), about)
	a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepThought, ClusterID: &id, OK: true,
		Title: fmt.Sprintf("Кластер «%s»: вопрос подбору, %d %s; событие подбора — %s", shortText(g.Question, 80), g.Count,
			pluralRu(g.Count, "раз", "раза", "раз"), g.Label),
		Result: toJSON(map[string]any{"cluster_label": row.Label, "families": g.Families, "event": g.Code})})
	a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepDiagnosis, ClusterID: &id, OK: true,
		Title:  fmt.Sprintf("Диагноз: %s — %s (ставит событие подбора, не модель)", domain.DiagKBMissingFact, cause.Label),
		Result: toJSON(map[string]any{"diagnosis": domain.DiagKBMissingFact, "cause": cause.Code, "families": ids})})
	p := &domain.Proposal{RunID: rs.run.ID, ClusterID: &id, ClusterKey: row.Key, ClusterLabel: row.Label,
		ClusterQueries: toJSON(queries), LostDemand: float64(g.Count), Diagnosis: domain.DiagKBMissingFact, DiagnosisText: text,
		Role: cause.Owner, Action: domain.ActionTicket, BaseVersion: rc.active.Version, Status: domain.ProposalPending,
		PatchKeys: toJSON([]string{patchKey}),
		Evidence: toJSON(map[string]any{"source": "guide_turns.gap_items", "event": g.Code, "count": g.Count,
			"families": g.Families, "task_to_owner": true})}
	title := fmt.Sprintf("Задача категорийному менеджеру: %s — «%s»", domain.DiagKBMissingFact, shortText(g.Question, 80))
	if rs.run.ReadOnly {
		p.ClusterID = &id
		a.step(ctx, rs, rc.draftStep(p, "Только чтение — не отправлено в очередь: "+title))
	} else {
		if err := st.CreateProposal(context.WithoutCancel(ctx), p); err != nil {
			return err
		}
		rc.proposals = append(rc.proposals, *p)
		a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepProposal, ClusterID: &id, OK: true, Title: title,
			Result: toJSON(map[string]any{"proposal_id": p.ID.String(), "diagnosis": p.Diagnosis, "action": p.Action, "role": p.Role})})
	}
	rs.mu.Lock()
	if !rs.run.ReadOnly {
		rs.run.ProposalsCreated++
	}
	rs.run.ClustersDone++
	rs.run.ClustersTotal++
	rs.mu.Unlock()
	rc.results = append(rc.results, map[string]any{"cluster_id": id, "label": row.Label, "diagnosis": domain.DiagKBMissingFact,
		"outcome": "ticket", "tool_calls": 0, "proposal_id": p.ID.String()})
	return nil
}
