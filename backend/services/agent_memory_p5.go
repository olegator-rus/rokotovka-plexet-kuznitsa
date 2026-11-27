package services

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Память агента контура автономии: три слоя с записью «откуда».
//   - решения — decisions (принятые, отклонённые, откаченные предложения и снятия «не предлагать» с причиной человека);
//   - «не предлагать» — memory_suppress: отказ человека и условие возврата (после отказа неудачных поисков кластера
//     набралось не меньше, чем при отказе, и не меньше 3, или новая версия каталога — новые доказательства), текущее
//     значение условия;
//   - эпизоды — memory_episodes: итог разбора кластера и эффект принятой правки («помогло», «без эффекта»,
//     «навредило»).
// Записи создают только решения человека (отказ, снятие) и настоящие запуски агента.

// Тексты.
const (
	MsgSuppressNotFound = "Запись «не предлагать» не найдена"
	MsgSuppressReason   = "Укажите причину, почему агенту снова можно предлагать это"
	MsgSuppressRemoved  = "Запись уже снята"
)

// effectCheckMax — сколько последних принятых правок проверяет ночной запуск.
const effectCheckMax = 5

// returnFailures — порог условия возврата: после отказа неудачных поисков кластера набралось не меньше, чем человек
// видел при отказе (и не меньше 3) — проблема вернулась с той же силой. Поиски до отказа не считаются: их человек уже
// видел.
func returnFailures(at int) int { return max(at, 3) }

// queriesFailures — неудачных поисков в запросах кластера предложения.
func queriesFailures(raw domain.JSONB) int {
	var qs []clusterQuery
	_ = json.Unmarshal(raw, &qs)
	n := 0
	for _, q := range qs {
		n += q.Failures
	}
	return n
}

// suppressScope — отказ «слишком широко» не закрывает кластер: агент ищет правку уже, эта не повторяется.
func suppressScope(reason string) string {
	if reason == domain.RejectTooBroad {
		return domain.SuppressScopePatch
	}
	return domain.SuppressScopeCluster
}

// newSuppress — запись «не предлагать» по отказу человека.
func (s *Services) newSuppress(p *domain.Proposal, d *domain.Decision) *domain.MemorySuppress {
	at := queriesFailures(p.ClusterQueries)
	cv := ""
	if cs := s.catalog.Load(); cs != nil {
		cv = cs.version
	}
	terms := d.Terms
	if len(terms) == 0 {
		terms = toJSON(proposalTerms(p))
	}
	decided := d.CreatedAt
	if decided.IsZero() {
		decided = s.now().UTC()
	}
	return &domain.MemorySuppress{ClusterKey: p.ClusterKey, ClusterLabel: p.ClusterLabel, Terms: terms,
		PatchKeys: p.PatchKeys, Diagnosis: p.Diagnosis, Scope: suppressScope(d.Reason), ProposalID: p.ID,
		SourceRunID: p.RunID, Reason: d.Reason, Comment: d.Comment, DecidedAt: decided, FailuresAt: at,
		ReturnFailures: returnFailures(at), CatalogAt: cv, CurrentFailures: at, Status: domain.SuppressActive}
}

// recordSuppress — запись «не предлагать» после отказа (RejectProposal). Ошибка записи не отменяет отказ: решение
// уже в decisions, запуск восстановит запись по нему.
func (s *Services) recordSuppress(ctx context.Context, p *domain.Proposal, d *domain.Decision) {
	st := s.store()
	if st == nil {
		return
	}
	m := s.newSuppress(p, d)
	if err := st.CreateSuppress(context.WithoutCancel(ctx), m); err != nil {
		zap.L().Warn("память: запись «не предлагать» не создана", zap.String("proposal_id", p.ID.String()), zap.Error(err))
		return
	}
	zap.L().Info("memory_suppress", zap.String("proposal_id", p.ID.String()), zap.String("scope", m.Scope),
		zap.Int("return_failures", m.ReturnFailures))
}

// suppressFor — запись «не предлагать» по отклонённому предложению; нет (отказ до контура автономии) — создаётся запуском по
// решению человека, «откуда» — то же предложение и его запуск.
func (rc *runCtx) suppressFor(ctx context.Context, proposalID string) *domain.MemorySuppress {
	for i := range rc.suppress {
		if rc.suppress[i].ProposalID.String() == proposalID {
			return &rc.suppress[i]
		}
	}
	var p *domain.Proposal
	for i := range rc.proposals {
		if rc.proposals[i].ID.String() == proposalID {
			p = &rc.proposals[i]
		}
	}
	var d *domain.Decision
	for i := range rc.decisions {
		if rc.decisions[i].ProposalID.String() == proposalID && rc.decisions[i].Decision == domain.ProposalRejected {
			d = &rc.decisions[i]
			break
		}
	}
	if p == nil || d == nil {
		return nil
	}
	m := rc.a.s.newSuppress(p, d)
	if err := rc.a.s.store().CreateSuppress(context.WithoutCancel(ctx), m); err != nil {
		zap.L().Warn("память: запись «не предлагать» не восстановлена", zap.Error(err))
		return nil
	}
	rc.suppress = append(rc.suppress, *m)
	return &rc.suppress[len(rc.suppress)-1]
}

// failuresAfter — неудачных поисков запросов кластера после момента at (в пределах окна запуска).
func (rc *runCtx) failuresAfter(ctx context.Context, c *failedCluster, at time.Time) int {
	since := maxTime(at, rc.statsSince())
	st, err := rc.a.s.windowStats(ctx, since, rc.a.s.now())
	if err != nil {
		zap.L().Warn("память: неудачи после отказа не посчитаны", zap.Error(err))
		return 0
	}
	n := 0
	for _, q := range c.Queries {
		if x := st.byText[q.Text]; x != nil {
			n += x.Empty + x.Reformulated + x.NoClick
		}
	}
	return n
}

// returnCondition — наступило ли условие возврата для кластера сейчас; cur — текущее значение (неудач после отказа).
func (rc *runCtx) returnCondition(ctx context.Context, m *domain.MemorySuppress, c *failedCluster) (bool, string, int) {
	cur := rc.failuresAfter(ctx, c, m.DecidedAt)
	if cur >= m.ReturnFailures {
		return true, fmt.Sprintf("после отказа неудачных поисков %d (при отказе было %d, порог возврата %d)", cur, m.FailuresAt, m.ReturnFailures), cur
	}
	if m.CatalogAt != "" && rc.catalogVersion != "" && m.CatalogAt != rc.catalogVersion {
		return true, fmt.Sprintf("новая версия каталога: %s (при отказе %s) — могли появиться товары", rc.catalogVersion, m.CatalogAt), cur
	}
	return false, "", cur
}

// saveSuppressCheck — текущее значение условия в запись.
func (rc *runCtx) saveSuppressCheck(ctx context.Context, m *domain.MemorySuppress, cur int) {
	now := rc.a.s.now().UTC()
	id := rc.rs.run.ID
	m.CurrentFailures, m.CheckedAt, m.CheckedRunID = cur, &now, &id
	if err := rc.a.s.store().SaveSuppress(context.WithoutCancel(ctx), m); err != nil {
		zap.L().Warn("память: проверка условия не записана", zap.Error(err))
	}
}

// memorySkip — память перед разбором кластера: предложение уже ждёт человека — пропуск; отказ человека (не «слишком
// широко») — пропуск, пока условия возврата не наступили (шаг memory_skip с причиной и текущим значением); условие
// наступило — агент предлагает снова и пишет, что изменилось; запись снята человеком — разбор как обычно.
// force — запуск «Разобрать» по запросу человека: память читается и показывается, но кластер не пропускается.
func (rc *runCtx) memorySkip(ctx context.Context, c *failedCluster, force bool) bool {
	hits := rc.memoryFor(ctx, c.Key, c.Terms)
	if len(hits) == 0 {
		return false
	}
	id := c.ID
	step := func(kind, title string, extra map[string]any) {
		res := map[string]any{"hits": hits}
		for k, v := range extra {
			res[k] = v
		}
		rc.a.step(ctx, rc.rs, &domain.AgentStep{Kind: kind, ClusterID: &id, OK: true, Title: title, Result: toJSON(res)})
	}
	skip := func(title string) bool {
		if row := rc.rows[c.ID]; row != nil {
			row.Status, row.Note = "skipped", title
			_ = rc.a.s.store().SaveIssueCluster(context.WithoutCancel(ctx), row)
		}
		rc.rs.mu.Lock()
		rc.outcome.SkippedMemory++
		rc.rs.mu.Unlock()
		rc.memorySkipped = append(rc.memorySkipped, map[string]any{"cluster_id": c.ID, "label": c.Label, "title": title})
		return true
	}
	for _, h := range hits {
		switch {
		case h.Kind == "proposal":
			title := fmt.Sprintf("Читаю память → группа «%s»: предложение %s уже ждёт решения человека — пропускаю", c.Label, h.Diagnosis)
			if force {
				step(domain.StepMemoryRead, title+" (запуск вручную — показываю, разбираю заново)", nil)
				continue
			}
			step(domain.StepMemorySkip, title, nil)
			return skip(title)
		case h.Status != domain.ProposalRejected:
			continue
		}
		m := rc.suppressFor(ctx, h.ProposalID)
		if m == nil {
			continue
		}
		ref := map[string]any{"suppress_id": m.ID.String(), "proposal_id": m.ProposalID.String(),
			"source_run_id": m.SourceRunID.String(), "return_failures": m.ReturnFailures}
		why := fmt.Sprintf("отклонено %s, причина «%s»%s", m.DecidedAt.In(mskZone).Format("02.01"), rejectLabels[m.Reason], commentSuffix(m.Comment))
		switch {
		case m.Status == domain.SuppressRemoved:
			step(domain.StepMemoryRead, fmt.Sprintf("Читаю память → «%s»: %s, но запись «не предлагать» снята человеком (%s) — разбираю",
				c.Label, why, shortText(m.RemovedReason, 80)), ref)
			continue
		case m.Scope == domain.SuppressScopePatch:
			step(domain.StepMemoryRead, fmt.Sprintf("Читаю память → «%s»: %s — группу разбираю, отклонённую правку не повторю", c.Label, why), ref)
			continue
		}
		met, what, cur := rc.returnCondition(ctx, m, c)
		ref["current_failures"] = cur
		if !met && m.Status == domain.SuppressLifted {
			m.Status, m.LiftedRunID, m.LiftedWhat = domain.SuppressActive, nil, ""
		}
		rc.saveSuppressCheck(ctx, m, cur)
		if met {
			if m.Status != domain.SuppressLifted {
				rid := rc.rs.run.ID
				m.Status, m.LiftedRunID, m.LiftedWhat = domain.SuppressLifted, &rid, what
				_ = rc.a.s.store().SaveSuppress(context.WithoutCancel(ctx), m)
			}
			rc.lifted[c.ID] = what
			ref["what_changed"] = what
			step(domain.StepMemoryRead, fmt.Sprintf("Читаю память → «%s»: %s; условие возврата наступило — %s. Предлагаю снова", c.Label, why, what), ref)
			continue
		}
		cond := fmt.Sprintf("после отказа неудачных поисков %d, вернусь при %d или новой версии каталога", cur, m.ReturnFailures)
		if force {
			step(domain.StepMemoryRead, fmt.Sprintf("Читаю память → «%s»: %s; условия возврата не наступили (%s). Запуск вручную — разбираю, отклонённую правку не повторю",
				c.Label, why, cond), ref)
			continue
		}
		title := fmt.Sprintf("Читаю память → группу «%s» пропускаю: %s; условия возврата не наступили (%s)", c.Label, why, cond)
		step(domain.StepMemorySkip, title, ref)
		return skip(title)
	}
	return false
}

// suppressLifted — записи, после которых отклонённую правку снова можно предлагать (снята человеком или условие
// возврата наступило в этом запуске).
func (rc *runCtx) suppressOpen(proposalID string) bool {
	for _, m := range rc.suppress {
		if m.ProposalID.String() == proposalID {
			return m.Status == domain.SuppressRemoved || m.Status == domain.SuppressLifted
		}
	}
	return false
}

// ── Эпизоды ──

// writeEpisode — эпизод запуска (memory_episode).
func (rc *runCtx) writeEpisode(ctx context.Context, e *domain.MemoryEpisode) {
	st := rc.a.s.store()
	if st == nil {
		return
	}
	e.RunID, e.Trigger = rc.rs.run.ID, rc.rs.run.Trigger
	if e.Details == nil {
		e.Details = domain.JSONB("{}")
	}
	if err := st.CreateEpisode(context.WithoutCancel(ctx), e); err != nil {
		zap.L().Warn("память: эпизод не записан", zap.Error(err))
		return
	}
	rc.episodes = append([]domain.MemoryEpisode{*e}, rc.episodes...)
}

// episodesFor — прошлые эпизоды кластера (для брифа модели: «прошлая правка без эффекта — переоцени причину»).
func (rc *runCtx) episodesFor(key string) []map[string]any {
	var res []map[string]any
	for _, e := range rc.episodes {
		if e.ClusterKey != key || e.RunID == rc.rs.run.ID {
			continue
		}
		res = append(res, map[string]any{"run_id": e.RunID.String(), "kind": e.Kind, "outcome": e.Outcome,
			"diagnosis": e.Diagnosis, "effect": e.Effect, "note": e.Note, "at": e.CreatedAt})
		if len(res) >= 5 {
			break
		}
	}
	return res
}

// ── Проверка эффекта принятых правок ──

// inversePatch — обратная правка: убрать добавленные алиасы (вернуть прежние значения из предыдущей версии),
// вернуть удалённые.
func (rc *runCtx) inversePatch(ctx context.Context, p *domain.Proposal, patch RulePatch) RulePatch {
	inv := RulePatch{Add: []PatchAlias{}, Remove: []PatchAlias{}}
	var prev *ruleSet
	if p.ConfigVersion != nil && *p.ConfigVersion > 1 {
		if v, err := rc.a.s.store().GetConfigVersion(ctx, *p.ConfigVersion-1); err == nil && v != nil {
			prev, _ = parseRules(v)
		}
	}
	for _, a := range patch.Add {
		if prev != nil {
			if old := prev.find(a.Alias); old != nil {
				inv.Add = append(inv.Add, PatchAlias{Kind: "alias", Alias: old.Alias, Canonical: old.Canonical, Target: old.Target,
					TargetID: old.TargetID, AliasKind: old.Kind})
				continue
			}
		}
		if rc.active.find(a.Alias) != nil {
			inv.Remove = append(inv.Remove, PatchAlias{Alias: a.Alias})
		}
	}
	for _, r := range patch.Remove {
		if prev != nil {
			if old := prev.find(r.Alias); old != nil {
				inv.Add = append(inv.Add, PatchAlias{Kind: "alias", Alias: old.Alias, Canonical: old.Canonical, Target: old.Target,
					TargetID: old.TargetID, AliasKind: old.Kind})
			}
		}
	}
	return inv
}

// effectVerdict — итог проверки: «навредило» — без правки регрессионный набор лучше (правка ухудшила запросы) или
// целевые запросы находятся хуже, чем до правки; «помогло» — целевые запросы находятся лучше, чем до правки, и
// вреда нет; иначе «без эффекта».
func effectVerdict(beforeFound, nowFound int, inv *patchEval) string {
	if inv != nil && (inv.Regression.Better > 0 || inv.Target.After.Found > inv.Target.Before.Found) || nowFound < beforeFound {
		return domain.EffectHarmed
	}
	if nowFound > beforeFound {
		return domain.EffectHelped
	}
	return domain.EffectNoEffect
}

var effectLabels = map[string]string{domain.EffectHelped: "помогло", domain.EffectNoEffect: "без эффекта", domain.EffectHarmed: "навредило"}

// effectCheck — в начале ночного запуска: каждая принятая правка (последние effectCheckMax) — тот же запрос сейчас и
// регрессия «с правкой против без неё» в песочнице. Итог — шаг effect_check, эффект в предложение и версию,
// эпизод; «навредило» — предложение отката (rollback_proposed_by_agent); «без эффекта» — эпизод, причина кластера
// переоценивается на следующем разборе (эпизод попадает в бриф модели).
func (rc *runCtx) effectCheck(ctx context.Context) {
	var effects []map[string]any
	n := 0
	for i := range rc.proposals {
		p := &rc.proposals[i]
		if p.Status != domain.ProposalAccepted || p.Action != domain.ActionRulesPatch || n >= effectCheckMax {
			continue
		}
		if ctx.Err() != nil || rc.rs.exhausted(rc.a.s.now()) != "" {
			break
		}
		n++
		t := time.Now()
		eff, err := rc.verifyProposal(ctx, p)
		if err != nil {
			continue
		}
		var patch RulePatch
		_ = jsonUnmarshal(p.Patch, &patch)
		var ev patchEval
		_ = jsonUnmarshal(p.Evaluation, &ev)
		var qs []clusterQuery
		_ = jsonUnmarshal(p.ClusterQueries, &qs)
		inv := rc.inversePatch(ctx, p, patch)
		var iev *patchEval
		if len(inv.Add)+len(inv.Remove) > 0 {
			eval := rc.a.evalHook
			if eval == nil {
				eval = rc.sb.evaluate
			}
			iev, err = eval(ctx, &failedCluster{Key: p.ClusterKey, Label: p.ClusterLabel, Queries: qs}, inv)
			if err != nil {
				iev = nil
			}
		}
		nowFound := 0
		if c, ok := eff["now"].(evalCount); ok {
			nowFound = c.Found
		}
		verdict := effectVerdict(ev.Target.Before.Found, nowFound, iev)
		regTotal, harmedN := 0, 0
		var harmedQ []string
		if iev != nil {
			regTotal, harmedN = iev.Regression.Total, iev.Regression.Better
			for _, q := range iev.Regression.BetterQueries {
				harmedQ = append(harmedQ, q.Text)
			}
		}
		eff["verdict"], eff["verdict_label"] = verdict, effectLabels[verdict]
		eff["regression"] = map[string]any{"total": regTotal, "harmed": harmedN, "harmed_queries": nonNil(harmedQ)}
		if iev != nil {
			eff["without_patch"] = iev
		}
		saved := rc.saveEffect(ctx, p, eff)
		date := ""
		if p.DecidedAt != nil {
			date = " от " + p.DecidedAt.In(mskZone).Format("02.01")
		}
		target := fmt.Sprintf("целевых запросов находится %d из %d (до правки %d)", nowFound, len(qs), ev.Target.Before.Found)
		if nowFound > ev.Target.Before.Found {
			target = fmt.Sprintf("запрос спасён: находится %d из %d (до правки %d)", nowFound, len(qs), ev.Target.Before.Found)
		}
		title := fmt.Sprintf("Проверяю эффект правки%s (%s): %s, регрессий %d из %d — %s", date, patchLine(patch), target,
			harmedN, regTotal, effectLabels[verdict])
		pid := p.ID.String()
		tool := "verify_applied_change"
		rc.a.step(ctx, rc.rs, &domain.AgentStep{Kind: domain.StepEffectCheck, Tool: &tool, OK: verdict != domain.EffectHarmed,
			Title: shortText(title, 300), Args: toJSON(map[string]any{"proposal_id": pid}), Result: saved,
			Ms: int(time.Since(t).Milliseconds())})
		pidU := p.ID
		rc.writeEpisode(ctx, &domain.MemoryEpisode{ClusterKey: p.ClusterKey, ClusterLabel: p.ClusterLabel, Kind: "effect_check",
			Outcome: verdict, Diagnosis: p.Diagnosis, ProposalID: &pidU, Effect: verdict,
			Details: toJSON(map[string]any{"target_before": ev.Target.Before.Found, "target_now": nowFound, "target_total": len(qs),
				"regression_total": regTotal, "regression_harmed": harmedN, "harmed_queries": nonNil(harmedQ),
				"config_version": p.ConfigVersion}),
			Note: shortText(title, 400)})
		rc.rs.mu.Lock()
		rc.outcome.EffectChecks++
		switch verdict {
		case domain.EffectHelped:
			rc.outcome.Helped++
		case domain.EffectNoEffect:
			rc.outcome.NoEffect++
		case domain.EffectHarmed:
			rc.outcome.Harmed++
		}
		rc.rs.mu.Unlock()
		effects = append(effects, eff)
		if verdict == domain.EffectHarmed && iev != nil {
			rc.proposeRollback(ctx, p, patch, inv, iev, eff, title)
		}
	}
	if len(effects) > 0 {
		rc.episode["applied_effects"] = effects
	}
}

// saveEffect — эффект в предложение и версию правил.
func (rc *runCtx) saveEffect(ctx context.Context, p *domain.Proposal, eff map[string]any) domain.JSONB {
	j := toJSON(eff)
	if st := rc.a.s.store(); st != nil {
		p.Effect = j
		if err := st.SaveProposal(context.WithoutCancel(ctx), p); err != nil {
			zap.L().Warn("агент: эффект не записан в предложение", zap.Error(err))
		}
		if p.ConfigVersion != nil {
			_ = st.SetConfigVersionEffect(context.WithoutCancel(ctx), *p.ConfigVersion, j)
		}
	}
	return j
}

// proposeRollback — агент сам предлагает откатить навредившую правку: обратная правка тем же механизмом версий,
// доказательства — прогон «с правкой / без правки». Применяет человек. Повторно не предлагается, пока прежнее
// предложение отката ждёт решения или отклонено (память).
func (rc *runCtx) proposeRollback(ctx context.Context, p *domain.Proposal, patch, inv RulePatch, iev *patchEval,
	eff map[string]any, checkTitle string) {
	key := "rollback:" + p.ID.String()
	for _, q := range rc.proposals {
		var pk []string
		_ = jsonUnmarshal(q.PatchKeys, &pk)
		if slices.Contains(pk, key) && (q.Status == domain.ProposalPending || q.Status == domain.ProposalRejected && !rc.suppressOpen(q.ID.String())) {
			rc.a.step(ctx, rc.rs, &domain.AgentStep{Kind: domain.StepMemoryRead, OK: true,
				Title:  fmt.Sprintf("Память: откат правки «%s» уже предлагался (%s) — не повторяю", p.ClusterLabel, statusLabel(q.Status)),
				Result: toJSON(map[string]any{"proposal_id": q.ID.String(), "rollback_of": p.ID.String()})})
			return
		}
	}
	date := ""
	if p.DecidedAt != nil {
		date = " от " + p.DecidedAt.In(mskZone).Format("02.01")
	}
	what := ""
	if len(iev.Regression.BetterQueries) > 0 {
		b := iev.Regression.BetterQueries[0]
		what = fmt.Sprintf(": запрос «%s» с правкой хуже, чем без неё", b.Text)
	} else if iev.Target.After.Found > iev.Target.Before.Found {
		what = fmt.Sprintf(": без правки целевых запросов находится больше (%d против %d)", iev.Target.After.Found, iev.Target.Before.Found)
	}
	text := fmt.Sprintf("Правка%s (%s, версия %d) навредила%s. Предлагаю откатить её обратной правкой — новая версия правил тем же механизмом, что ручной откат.",
		date, patchLine(patch), derefInt(p.ConfigVersion), what)
	id := ""
	if p.ClusterID != nil {
		id = *p.ClusterID
	}
	evidence := map[string]any{"rollback_proposed_by_agent": true, "rollback_of": p.ID.String(), "config_version": p.ConfigVersion,
		"original_patch": patch, "effect": eff, "check": checkTitle, "terms": proposalTerms(p)}
	rb := &domain.Proposal{RunID: rc.rs.run.ID, ClusterKey: p.ClusterKey, ClusterLabel: p.ClusterLabel,
		ClusterQueries: p.ClusterQueries, LostDemand: p.LostDemand, Diagnosis: p.Diagnosis, DiagnosisText: text,
		Role: OwnerOf(p.Diagnosis), Action: domain.ActionRollback, Patch: toJSON(inv), PatchKeys: toJSON([]string{key}),
		Evaluation: toJSON(iev), Evidence: toJSON(evidence), BaseVersion: rc.active.Version, Status: domain.ProposalPending,
		EvidenceLevel: p.EvidenceLevel, TechnicalStatus: domain.TechVerified, EconomicStatus: domain.EconUnknown}
	if id != "" {
		rb.ClusterID = &id
	}
	title := "Предлагаю откат: " + text
	if rc.rs.run.ReadOnly {
		rc.draft(ctx, rb, "Только чтение — откат не отправлен в очередь: "+text)
		return
	}
	if err := rc.a.s.store().CreateProposal(context.WithoutCancel(ctx), rb); err != nil {
		zap.L().Warn("агент: предложение отката не записано", zap.Error(err))
		return
	}
	rc.proposals = append(rc.proposals, *rb)
	rc.rs.mu.Lock()
	rc.rs.run.ProposalsCreated++
	rc.outcome.Rollbacks++
	rc.rs.mu.Unlock()
	rc.a.step(ctx, rc.rs, &domain.AgentStep{Kind: domain.StepProposal, OK: true, Title: shortText(title, 300),
		Result: toJSON(map[string]any{"proposal_id": rb.ID.String(), "action": rb.Action, "rollback_of": p.ID.String(),
			"rollback_proposed_by_agent": true, "role": rb.Role})})
	zap.L().Info("rollback_proposed_by_agent", zap.String("proposal_id", rb.ID.String()), zap.String("rollback_of", p.ID.String()))
}

// draft — режим «только чтение»: предложение сформировано, но в очередь не пишется (черновик в эпизоде и шаге).
func (rc *runCtx) draft(ctx context.Context, p *domain.Proposal, title string) {
	rc.a.step(ctx, rc.rs, rc.draftStep(p, title))
}

// draftStep — черновик в счётчики запуска и шаг ленты (без записи в очередь).
func (rc *runCtx) draftStep(p *domain.Proposal, title string) *domain.AgentStep {
	d := map[string]any{"diagnosis": p.Diagnosis, "cause": CauseOf(p.Diagnosis).Code, "owner": OwnerOf(p.Diagnosis),
		"action": p.Action, "text": p.DiagnosisText, "patch": json.RawMessage(nonEmptyJSON(p.Patch)),
		"evaluation": json.RawMessage(nonEmptyJSON(p.Evaluation)), "cluster_label": p.ClusterLabel}
	rc.rs.mu.Lock()
	rc.drafts = append(rc.drafts, d)
	rc.outcome.Drafts++
	rc.rs.mu.Unlock()
	return &domain.AgentStep{Kind: domain.StepProposal, ClusterID: p.ClusterID, OK: true, Title: shortText(title, 300),
		Result: toJSON(map[string]any{"draft": d, "read_only": true})}
}

func nonEmptyJSON(j domain.JSONB) []byte {
	if len(j) == 0 {
		return []byte("null")
	}
	return j
}

// ── API памяти ──

// AgentMemoryLayer — GET /api/agent/memory?layer=decisions|suppress|episodes.
func (s *Services) AgentMemoryLayer(ctx context.Context, layer string) (*out.MemoryLayer, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	runs := s.runTriggers(ctx)
	res := &out.MemoryLayer{Layer: layer}
	switch layer {
	case "decisions":
		ds, err := st.ListDecisions(ctx)
		if err != nil {
			return nil, err
		}
		props, err := st.ListProposals(ctx, "", "")
		if err != nil {
			return nil, err
		}
		byID := map[uuid.UUID]*domain.Proposal{}
		for i := range props {
			byID[props[i].ID] = &props[i]
		}
		res.Decisions = []out.MemoryDecision{}
		for _, d := range ds {
			c := CauseOf(d.Diagnosis)
			v := out.MemoryDecision{ID: d.ID.String(), ProposalID: d.ProposalID.String(), ClusterLabel: d.ClusterLabel,
				ClusterKey: d.ClusterKey, Diagnosis: d.Diagnosis, Cause: c.Code, CauseLabel: c.Label, Decision: d.Decision,
				Reason: d.Reason, ReasonLabel: decisionReasonLabel(d), Comment: d.Comment, DecidedBy: d.DecidedBy,
				DecidedLabel: RoleLabel(d.DecidedBy), TS: d.CreatedAt}
			if d.DecidedBy == "" {
				v.DecidedLabel = ""
			}
			if p := byID[d.ProposalID]; p != nil {
				rid := p.RunID.String()
				v.RunID, v.Trigger, v.AgentReason, v.Action = &rid, runs[p.RunID], p.DiagnosisText, p.Action
				var patch RulePatch
				if p.Action == domain.ActionRulesPatch || p.Action == domain.ActionRollback {
					_ = json.Unmarshal(p.Patch, &patch)
					v.Proposal = patchLine(patch)
				} else {
					v.Proposal = "задача: " + CauseOf(p.Diagnosis).Label
				}
			}
			res.Decisions = append(res.Decisions, v)
		}
	case "suppress":
		ms, err := st.ListSuppress(ctx)
		if err != nil {
			return nil, err
		}
		res.Suppress = []out.MemorySuppress{}
		for _, m := range ms {
			res.Suppress = append(res.Suppress, suppressView(&m))
		}
	case "episodes":
		es, err := st.ListEpisodes(ctx, 300)
		if err != nil {
			return nil, err
		}
		res.Episodes = []out.MemoryEpisode{}
		for _, e := range es {
			c := CauseOf(e.Diagnosis)
			v := out.MemoryEpisode{ID: e.ID.String(), RunID: e.RunID.String(), Trigger: e.Trigger, Kind: e.Kind,
				ClusterKey: e.ClusterKey, ClusterLabel: e.ClusterLabel, Outcome: e.Outcome, Diagnosis: e.Diagnosis,
				Cause: c.Code, CauseLabel: c.Label, Details: rawOrNull(e.Details), Note: e.Note, TS: e.CreatedAt}
			if e.Diagnosis == "" {
				v.Cause, v.CauseLabel = "", ""
			}
			if e.ProposalID != nil {
				pid := e.ProposalID.String()
				v.ProposalID = &pid
			}
			if e.Effect != "" {
				eff := e.Effect
				v.Effect = &eff
			}
			res.Episodes = append(res.Episodes, v)
		}
	default:
		return nil, errs.NewValidationError(errs.MsgInvalidQuery).WithDetails(errs.FieldError{Field: "layer",
			Code: errs.CodeInvalidFormat, Message: "Слой — decisions, suppress или episodes"})
	}
	return res, nil
}

// decisionReasonLabel — причина решения по-русски.
func decisionReasonLabel(d domain.Decision) string {
	switch d.Decision {
	case domain.ProposalRejected:
		return rejectLabels[d.Reason]
	case domain.ProposalAccepted:
		return "принято"
	case domain.ProposalRolledBack:
		if d.Reason == "agent_rollback" {
			return "откачено по предложению агента"
		}
		return "откачено"
	case domain.DecisionSuppressRemoved:
		return "снята запись «не предлагать»"
	}
	return d.Reason
}

// runTriggers — запуск → триггер (для «откуда» в памяти и очереди).
func (s *Services) runTriggers(ctx context.Context) map[uuid.UUID]string {
	res := map[uuid.UUID]string{}
	if st := s.store(); st != nil {
		if rows, err := st.ListAgentRunsByTrigger(ctx, "", 500); err == nil {
			for _, r := range rows {
				res[r.ID] = r.Trigger
			}
		}
	}
	return res
}

// suppressView — запись «не предлагать» в форме API.
func suppressView(m *domain.MemorySuppress) out.MemorySuppress {
	var pk []string
	_ = json.Unmarshal(m.PatchKeys, &pk)
	why := rejectLabels[m.Reason]
	if m.Comment != "" {
		why += ": " + m.Comment
	}
	cond := fmt.Sprintf("после отказа наберётся не меньше %d неудачных поисков в группе (при отказе было %d) или выйдет новая версия каталога", m.ReturnFailures, m.FailuresAt)
	if m.Scope == domain.SuppressScopePatch {
		cond = "группа разбирается; эта правка не повторяется, пока человек не снимет запись"
	}
	v := out.MemorySuppress{ID: m.ID.String(), ClusterKey: m.ClusterKey, ClusterLabel: m.ClusterLabel, Scope: m.Scope,
		PatchKeys: nonNil(pk), Diagnosis: m.Diagnosis, Why: why, Reason: m.Reason, Comment: m.Comment, DecidedAt: m.DecidedAt,
		ProposalID: m.ProposalID.String(), RunID: m.SourceRunID.String(), ReturnCondition: cond, FailuresAt: m.FailuresAt,
		ReturnFailures: m.ReturnFailures, CurrentFailures: m.CurrentFailures, CatalogAt: m.CatalogAt, CheckedAt: m.CheckedAt,
		Status: m.Status, LiftedWhat: m.LiftedWhat, RemovedReason: m.RemovedReason, RemovedBy: m.RemovedBy, RemovedAt: m.RemovedAt}
	if m.CheckedRunID != nil {
		id := m.CheckedRunID.String()
		v.CheckedRunID = &id
	}
	if m.LiftedRunID != nil {
		id := m.LiftedRunID.String()
		v.LiftedRunID = &id
	}
	return v
}

// SuppressOwner — кто вправе снять запись: владелец причины её предложения.
func (s *Services) SuppressOwner(ctx context.Context, id string) (string, error) {
	m, err := s.loadSuppress(ctx, id)
	if err != nil {
		return "", err
	}
	return OwnerOf(m.Diagnosis), nil
}

func (s *Services) loadSuppress(ctx context.Context, id string) (*domain.MemorySuppress, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.NewValidationError(errs.MsgInvalidID)
	}
	m, err := st.GetSuppress(ctx, uid)
	if err != nil {
		return nil, err
	}
	if m == nil {
		return nil, errs.NewNotFoundError(MsgSuppressNotFound)
	}
	return m, nil
}

// RemoveSuppress — POST /api/agent/memory/suppress/{id}/remove {"reason"}: человек снимает запись «не предлагать»
// с причиной; это тоже решение (decisions: suppress_removed).
func (s *Services) RemoveSuppress(ctx context.Context, id string, req in.SuppressRemoveRequest, actor string) (*out.MemorySuppress, error) {
	reason := truncateRunes(strings.TrimSpace(req.Reason), 300)
	if reason == "" {
		return nil, errs.NewValidationError(MsgSuppressReason).WithDetails(errs.FieldError{Field: "reason",
			Code: errs.CodeInvalidFormat, Message: MsgSuppressReason})
	}
	m, err := s.loadSuppress(ctx, id)
	if err != nil {
		return nil, err
	}
	if m.Status == domain.SuppressRemoved {
		return nil, errs.NewConflictError(MsgSuppressRemoved)
	}
	now := s.now().UTC()
	m.Status, m.RemovedReason, m.RemovedBy, m.RemovedAt = domain.SuppressRemoved, reason, actor, &now
	st := s.store()
	if err := st.SaveSuppress(ctx, m); err != nil {
		return nil, err
	}
	d := &domain.Decision{ProposalID: m.ProposalID, Decision: domain.DecisionSuppressRemoved, Reason: "suppress_removed",
		Comment: reason, ClusterKey: m.ClusterKey, ClusterLabel: m.ClusterLabel, Diagnosis: m.Diagnosis, Terms: m.Terms,
		PatchKeys: m.PatchKeys, DecidedBy: actor}
	if err := st.CreateDecision(ctx, d); err != nil {
		zap.L().Warn("память: решение о снятии не записано", zap.Error(err))
	}
	zap.L().Info("memory_decision", zap.String("decision", domain.DecisionSuppressRemoved), zap.String("suppress_id", id))
	v := suppressView(m)
	return &v, nil
}
