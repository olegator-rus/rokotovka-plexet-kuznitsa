package services

import (
	"context"
	"fmt"
	"slices"
	"strings"

	"kuznica/api/domain"

	"github.com/google/uuid"
)

// Инструменты агента, которые меняют состояние кластера: прогон гипотезы, предложение, задача, закрытие,
// эффект принятой правки.

// emit — шаг, порождённый инструментом: в ленту сразу после строки вызова инструмента.
func (cr *clusterRun) emit(st *domain.AgentStep) { cr.pending = append(cr.pending, st) }

// rejectHypothesis — гипотеза отклонена: шаг проверки и зачёркнутая гипотеза в ленте.
func (cr *clusterRun) rejectHypothesis(ctx context.Context, sp *sandboxPatch, reason string, result map[string]any) {
	a, rs, id := cr.rc.a, cr.rc.rs, cr.c.ID
	cr.rejected++
	sp.Rejected = reason
	no := sp.No
	r := reason
	result["passed"] = false
	result["reason"] = reason
	cr.emit(&domain.AgentStep{Kind: domain.StepCheck, ClusterID: &id, OK: false, HypothesisNo: &no, RejectedReason: &r,
		Title: fmt.Sprintf("Гипотеза %d отклонена: %s", no, shortText(reason, 220)), Result: toJSON(result)})
	sp.Step.RejectedReason = &r
	sp.Step.OK = false
	// Гипотеза уже в ленте (прогон в следующем вызове) — перезапись тем же id; ещё в очереди — уйдёт с причиной.
	if sp.Step.Seq > 0 {
		a.updateStep(ctx, rs, sp.Step)
	}
	cr.tried = append(cr.tried, map[string]any{"no": no, "patch": sp.Patch, "hypothesis": sp.Hypothesis, "passed": false,
		"reason": reason, "evaluation": sp.Eval})
}

func (cr *clusterRun) toolRunEvaluation(ctx context.Context, raw string) (any, string, error) {
	var args struct {
		PatchID string `json:"patch_id"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	sp := cr.patches[strings.TrimSpace(args.PatchID)]
	if sp == nil {
		return nil, "", terr("правки %q нет: сначала create_sandbox_patch", args.PatchID)
	}
	if sp.Rejected != "" {
		return nil, "", terr("гипотеза %d уже отклонена: %s", sp.No, sp.Rejected)
	}
	if sp.Eval != nil {
		return map[string]any{"patch_id": sp.ID, "evaluation": sp.Eval}, "прогон уже был", nil
	}
	eval := cr.rc.sb.evaluate
	if h := cr.rc.a.evalHook; h != nil {
		eval = h
	}
	ev, err := eval(ctx, cr.c, sp.Patch)
	if err != nil {
		return nil, "", terr("прогон прерван: %v", err)
	}
	sp.Eval = ev
	// Ожидаемое правкой семейство в выдаче, но не первым: ранжирование, а не словарь.
	var expected map[string]bool
	if cr.rc.sb != nil {
		expected = cr.rc.sb.expectedFamilies(sp.Patch)
	}
	ranking := map[string]string{}
	for _, q := range ev.Target.Queries {
		for _, top := range [][]string{q.BeforeTop, q.AfterTop} {
			for i, f := range top {
				if i > 0 && f != top[0] {
					cr.noteNotFirst(f, q.Text)
				}
			}
		}
		if q.BeforeFound && !q.AfterFound {
			for i, f := range q.AfterTop {
				if i > 0 && expected[f] {
					ranking[f] = q.Text
				}
			}
		}
	}
	title := fmt.Sprintf("гипотеза %d: целевой кластер %d/%d → %d/%d, регрессия: хуже %d из %d",
		sp.No, ev.Target.Before.Found, ev.Target.Before.Total, ev.Target.After.Found, ev.Target.After.Total,
		ev.Regression.Worse, ev.Regression.Total)
	res := map[string]any{"patch_id": sp.ID, "hypothesis_no": sp.No, "evaluation": ev}
	if !ev.Passed {
		cr.rejectHypothesis(ctx, sp, strings.Join(ev.Reasons, "; "), map[string]any{"evaluation": ev})
		res["hypotheses_left"] = agentMaxHypotheses - cr.hypotheses
		switch {
		case len(ranking) > 0:
			for f, q := range ranking {
				res["hint"] = fmt.Sprintf("по запросу «%s» нужное семейство %s в выдаче, но не первым — это похоже на RANKING_FAILURE: "+
					"задача менеджеру e-com (create_assortment_ticket, diagnosis=RANKING_FAILURE, family_id=%s) или следующая гипотеза", q, f, f)
				break
			}
		case cr.hypotheses >= agentMaxHypotheses:
			res["hint"] = "гипотезы исчерпаны: закрой кластер (close_cluster) с причиной «не решается правкой правил»"
		default:
			res["hint"] = "сформулируй следующую гипотезу уже, учитывая причины отказа"
		}
		return res, title + " — отклонена", nil
	}
	id, no := cr.c.ID, sp.No
	cr.emit(&domain.AgentStep{Kind: domain.StepCheck, ClusterID: &id, OK: true, HypothesisNo: &no,
		Title: fmt.Sprintf("Гипотеза %d прошла прогон: находилось %d из %d, стало %d из %d; регрессия — хуже %d из %d",
			no, ev.Target.Before.Found, ev.Target.Before.Total, ev.Target.After.Found, ev.Target.After.Total, ev.Regression.Worse, ev.Regression.Total),
		Result: toJSON(map[string]any{"evaluation": ev, "passed": true})})
	cr.tried = append(cr.tried, map[string]any{"no": no, "patch": sp.Patch, "hypothesis": sp.Hypothesis, "passed": true, "evaluation": ev})
	res["next"] = "submit_proposal"
	return res, title + " — прошла", nil
}

// newProposal — общая часть предложения: кластер, доказательства, база правил.
func (cr *clusterRun) newProposal(diagnosis, text, role, action string) *domain.Proposal {
	c := cr.c
	id := c.ID
	cr.evidence["hypotheses"] = cr.tried
	cr.evidence["memory"] = nonNil(cr.memory)
	if row := cr.rc.rows[c.ID]; row != nil && len(row.Current) > 0 {
		cr.evidence["current_search"] = row.Current
	}
	var reform []map[string]any
	for _, q := range c.Queries {
		for _, r := range q.ReformulatedTo {
			reform = append(reform, map[string]any{"from": q.Text, "to": r.Text, "count": r.Count, "clicked": r.Clicked})
		}
	}
	cr.evidence["reformulations"] = nonNil(reform)
	return &domain.Proposal{RunID: cr.rc.rs.run.ID, ClusterID: &id, ClusterKey: c.Key, ClusterLabel: c.Label,
		ClusterQueries: toJSON(c.Queries), LostDemand: c.LostDemand, Diagnosis: diagnosis, DiagnosisText: text,
		Role: role, Action: action, HypothesesTried: cr.hypotheses, BaseVersion: cr.rc.active.Version,
		Status: domain.ProposalPending, Evidence: toJSON(cr.evidence)}
}

// saveProposal — предложение в очередь, шаг ленты, счётчик запуска.
func (cr *clusterRun) saveProposal(ctx context.Context, p *domain.Proposal, title string) error {
	if err := cr.rc.a.s.store().CreateProposal(context.WithoutCancel(ctx), p); err != nil {
		return err
	}
	cr.rc.proposals = append(cr.rc.proposals, *p)
	_, rs, id := cr.rc.a, cr.rc.rs, cr.c.ID
	cr.emit(&domain.AgentStep{Kind: domain.StepProposal, ClusterID: &id, OK: true, Title: title,
		Result: toJSON(map[string]any{"proposal_id": p.ID.String(), "diagnosis": p.Diagnosis, "action": p.Action, "role": p.Role})})
	rs.mu.Lock()
	rs.run.ProposalsCreated++
	rs.mu.Unlock()
	cr.result.ProposalID = p.ID.String()
	return nil
}

func (cr *clusterRun) toolSubmit(ctx context.Context, raw string) (any, string, error) {
	var args struct {
		PatchID       string `json:"patch_id"`
		DiagnosisText string `json:"diagnosis_text"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	text, err := checkText("diagnosis_text", args.DiagnosisText, 600)
	if err != nil {
		return nil, "", err
	}
	sp := cr.patches[strings.TrimSpace(args.PatchID)]
	switch {
	case sp == nil:
		return nil, "", terr("правки %q нет", args.PatchID)
	case sp.Eval == nil:
		return nil, "", terr("правка %s не прогонялась: сначала run_evaluation", sp.ID)
	case !sp.Eval.Passed:
		return nil, "", terr("правка %s не прошла прогон и в очередь не идёт", sp.ID)
	}
	p := cr.newProposal(sp.Diagnosis, text, domain.RoleEcom, domain.ActionRulesPatch)
	p.Patch = toJSON(sp.Patch)
	p.PatchKeys = toJSON(patchKeys(sp.Patch))
	p.Evaluation = toJSON(sp.Eval)
	ev := sp.Eval
	title := fmt.Sprintf("Предложение в очередь: %s; целевой кластер: находилось %d из %d, стало %d из %d; регрессия: ухудшилось %d",
		patchLine(sp.Patch), ev.Target.Before.Found, ev.Target.Before.Total, ev.Target.After.Found, ev.Target.After.Total, ev.Regression.Worse)
	if err := cr.saveProposal(ctx, p, title); err != nil {
		return nil, "", err
	}
	cr.finish("proposal", sp.Diagnosis, text)
	return map[string]any{"proposal_id": p.ID.String(), "status": p.Status}, "предложение " + p.ID.String()[:8] + " ждёт решения человека", nil
}

func (cr *clusterRun) toolTicket(ctx context.Context, raw string) (any, string, error) {
	var args struct {
		Diagnosis   string   `json:"diagnosis"`
		Text        string   `json:"text"`
		Substitutes []string `json:"substitute_family_ids"`
		FamilyID    string   `json:"family_id"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	text, err := checkText("text", args.Text, 600)
	if err != nil {
		return nil, "", err
	}
	cat := cr.rc.cat
	var subs []map[string]any
	for _, f := range args.Substitutes {
		fam := cat.families[strings.TrimSpace(f)]
		if fam == nil {
			return nil, "", terr("семейства-аналога %q нет в каталоге", f)
		}
		subs = append(subs, map[string]any{"family_id": fam.ID, "name": fam.Name, "vendor": cat.vendorName(fam.VendorID)})
	}
	role := domain.RoleProduct
	switch args.Diagnosis {
	case domain.DiagAssortmentGap:
		if !cr.expanded {
			return nil, "", terr("ASSORTMENT_GAP ставится только после расширенного поиска: search_catalog с expand=true")
		}
		if len(subs) > 0 {
			return nil, "", terr("аналоги указаны — это SUBSTITUTE_AVAILABLE, а не ASSORTMENT_GAP")
		}
	case domain.DiagSubstitute:
		if !cr.searched {
			return nil, "", terr("сначала найди аналог в каталоге (search_catalog)")
		}
		if len(subs) == 0 {
			return nil, "", terr("для SUBSTITUTE_AVAILABLE укажи substitute_family_ids")
		}
	case domain.DiagContentGap:
		if cat.families[strings.TrimSpace(args.FamilyID)] == nil {
			return nil, "", terr("для CONTENT_GAP укажи family_id карточки из каталога")
		}
		// Карточка виновата, только если покупатели этого кластера всё же приходят к семейству — после
		// переформулировки или через похожие успешные запросы. Иначе («счёт на оплату» → 1С по подстроке)
		// это не карточка, а NON_PRODUCT или спрос без предложения.
		if !cr.foundBy[strings.TrimSpace(args.FamilyID)] {
			return nil, "", terr("CONTENT_GAP — только если покупатели кластера находят семейство %s после переформулировки "+
				"(reformulated_to) или через похожие успешные запросы (find_similar_successful_queries); иначе это NON_PRODUCT "+
				"(запрос не о товаре) или ASSORTMENT_GAP", strings.TrimSpace(args.FamilyID))
		}
		role = domain.RoleContent
		cr.evidence["family_id"] = strings.TrimSpace(args.FamilyID)
	case domain.DiagRankingFailure:
		fid := strings.TrimSpace(args.FamilyID)
		if cat.families[fid] == nil {
			return nil, "", terr("для RANKING_FAILURE укажи family_id семейства из каталога, которое стоит низко")
		}
		q, ok := cr.notFirst[fid]
		if !ok {
			return nil, "", terr("RANKING_FAILURE — только если семейство %s видно в выдаче запроса кластера, но не первым: "+
				"сначала get_search_trace или прогон гипотезы", fid)
		}
		role = domain.RoleEcom
		cr.evidence["family_id"] = fid
		cr.evidence["ranking_query"] = q
	default:
		return nil, "", terr("diagnosis — SUBSTITUTE_AVAILABLE, ASSORTMENT_GAP, CONTENT_GAP или RANKING_FAILURE")
	}
	// Память: такая же задача по кластеру уже отклонена человеком.
	for _, h := range cr.memory {
		if h.Status == domain.ProposalRejected && h.Diagnosis == args.Diagnosis {
			return nil, "", terr("человек уже отклонил задачу %s по этому кластеру (%s) — не предлагаю повторно", args.Diagnosis, rejectLabels[h.Reason])
		}
	}
	id := cr.c.ID
	cr.diagnosis = args.Diagnosis
	cr.emit(&domain.AgentStep{Kind: domain.StepDiagnosis, ClusterID: &id, OK: true,
		Title:  fmt.Sprintf("Диагноз: %s — %s", args.Diagnosis, diagnosisLabels[args.Diagnosis]),
		Result: toJSON(map[string]any{"diagnosis": args.Diagnosis, "reason": text, "substitutes": nonNil(subs)})})
	cr.evidence["substitutes"] = nonNil(subs)
	cr.evidence["expanded_search"] = cr.expanded
	p := cr.newProposal(args.Diagnosis, text, role, domain.ActionTicket)
	p.PatchKeys = toJSON([]string{"ticket:" + args.Diagnosis + ":" + cr.c.Key})
	who := map[string]string{domain.RoleProduct: "продукт-менеджеру", domain.RoleContent: "контент-менеджеру", domain.RoleEcom: "менеджеру e-com"}[role]
	title := fmt.Sprintf("Задача %s: %s — «%s»", who, args.Diagnosis, cr.c.Label)
	if len(subs) > 0 {
		var names []string
		for _, s := range subs {
			names = append(names, fmt.Sprint(s["name"]))
		}
		title += ", аналоги: " + strings.Join(names, ", ")
	}
	if err := cr.saveProposal(ctx, p, title); err != nil {
		return nil, "", err
	}
	cr.finish("ticket", args.Diagnosis, text)
	return map[string]any{"proposal_id": p.ID.String(), "status": p.Status, "role": role}, title, nil
}

var closeDiagnoses = []string{domain.DiagAliasGap, domain.DiagTypo, domain.DiagConstraintConflict,
	domain.DiagNonProduct, domain.DiagNoise, domain.DiagUnknown}

// ticketDiagnoses — диагнозы, итог которых — задача человеку, а не закрытие кластера.
var ticketDiagnoses = []string{domain.DiagAssortmentGap, domain.DiagSubstitute, domain.DiagContentGap, domain.DiagRankingFailure}

func (cr *clusterRun) toolClose(ctx context.Context, raw string) (any, string, error) {
	var args struct {
		Diagnosis string `json:"diagnosis"`
		Reason    string `json:"reason"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	reason, err := checkText("reason", args.Reason, 400)
	if err != nil {
		return nil, "", err
	}
	// Модель путает инструменты: «закрыть» кластер с диагнозом задачи. Раньше это был отказ, и после двух
	// отказов модель сдавалась на UNKNOWN (winrar, запуск 01a20fe5). Теперь — задача с тем же текстом: её
	// проверки (расширенный поиск для ASSORTMENT_GAP, аналоги для SUBSTITUTE_AVAILABLE) делает toolTicket.
	if slices.Contains(ticketDiagnoses, args.Diagnosis) {
		ticket := map[string]any{"diagnosis": args.Diagnosis, "text": reason}
		if args.Diagnosis == domain.DiagRankingFailure && len(cr.notFirst) == 1 {
			for f := range cr.notFirst {
				ticket["family_id"] = f
			}
		}
		return cr.toolTicket(ctx, string(toJSON(ticket)))
	}
	if !slices.Contains(closeDiagnoses, args.Diagnosis) {
		return nil, "", terr("diagnosis для закрытия — %s; для %s — create_assortment_ticket",
			strings.Join(closeDiagnoses, ", "), strings.Join(ticketDiagnoses, ", "))
	}
	// UNKNOWN при доказанном отсутствии: расширенный поиск ничего не нашёл, правки не прошли — это спрос без
	// предложения. Один раз отказываем с подсказкой; повторное UNKNOWN принимается (модель могла увидеть аналог).
	if args.Diagnosis == domain.DiagUnknown && cr.expanded && !cr.expandedFound && !cr.unknownHinted && !cr.anyPassed() {
		cr.unknownHinted = true
		return nil, "", terr("расширенный поиск по каталогу (expand=true) не нашёл ни товара, ни семейств — это ASSORTMENT_GAP, " +
			"а не UNKNOWN: create_assortment_ticket с diagnosis=ASSORTMENT_GAP (или SUBSTITUTE_AVAILABLE, если нашёл аналог)")
	}
	// Правка прошла прогон — её нельзя молча выбросить.
	for _, p := range cr.patches {
		if p.Eval != nil && p.Eval.Passed {
			return nil, "", terr("гипотеза %d прошла прогон — отправь её (submit_proposal)", p.No)
		}
	}
	id := cr.c.ID
	if cr.diagnosis != args.Diagnosis {
		cr.emit(&domain.AgentStep{Kind: domain.StepDiagnosis, ClusterID: &id, OK: true,
			Title:  fmt.Sprintf("Диагноз: %s — %s", args.Diagnosis, diagnosisLabels[args.Diagnosis]),
			Result: toJSON(map[string]any{"diagnosis": args.Diagnosis, "reason": reason})})
	}
	cr.finish("closed", args.Diagnosis, reason)
	return map[string]any{"closed": true, "diagnosis": args.Diagnosis}, args.Diagnosis + " — " + shortText(reason, 120), nil
}

// anyPassed — хоть одна гипотеза прошла прогон.
func (cr *clusterRun) anyPassed() bool {
	for _, p := range cr.patches {
		if p.Eval != nil && p.Eval.Passed {
			return true
		}
	}
	return false
}

func (cr *clusterRun) toolVerify(ctx context.Context, raw string) (any, string, error) {
	var args struct {
		ProposalID string `json:"proposal_id"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	uid, err := uuid.Parse(strings.TrimSpace(args.ProposalID))
	if err != nil {
		return nil, "", terr("proposal_id — не идентификатор")
	}
	p, err := cr.rc.a.s.store().GetProposal(ctx, uid)
	if err != nil {
		return nil, "", err
	}
	if p == nil || p.Status != domain.ProposalAccepted || p.Action != domain.ActionRulesPatch {
		return nil, "", terr("принятой правки %s нет", args.ProposalID)
	}
	eff, err := cr.rc.verifyProposal(ctx, p)
	if err != nil {
		return nil, "", terr("эффект не посчитан: %v", err)
	}
	return eff, fmt.Sprint(eff["summary"]), nil
}
