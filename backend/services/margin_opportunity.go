package services

import (
	"context"
	"encoding/json"
	"fmt"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"

	"go.uber.org/zap"
)

// MARGIN_OPPORTUNITY (контур коммерции): агент в режиме «Коммерция» перебирает
// положения рычага маржи по категориям в пределах ограничений рычага (группы релевантности, ±2, точные запросы, ε) —
// тем же кодом, что симуляция POST /api/margin/simulate с полем category (marginSimulateCategory на базовой выдаче
// tune). Где сдвиг категории на одно положение вверх не меняет качество (NDCG@10 размеченных запросов не ниже,
// ни один запрос не теряет больше ε, первые места точных запросов на месте) и поднимает индекс показанной маржи, —
// предложение `margin_policy` в общую очередь. Применяется оно на экране «Маржа» (POST /api/margin/policy с
// proposal_id), а не из очереди: там видны затронутые запросы. LLM не вызывается — перебор детерминированный.

const (
	// marginOppMax — предложений по марже за запуск (лучшие по приросту индекса).
	marginOppMax = 3
	// marginOppNDCGTol — «качество не меняется»: NDCG@10 набора не ниже текущего больше чем на эту величину.
	marginOppNDCGTol = 1e-4
	// marginOppMinGain — минимальный прирост индекса показанной маржи (перцентиль 0–1), чтобы предлагать.
	marginOppMinGain = 1e-4
	// marginOppKeyPrefix — ключ находки: margin:<категория>:<вес>.
	marginOppKeyPrefix = "margin:"
	// ToolSimulateMargin — инструмент ленты агента.
	ToolSimulateMargin = "simulate_margin_policy"
)

// MsgMarginViaScreen — предложение по марже принимается на экране «Маржа».
const MsgMarginViaScreen = "Предложение по марже применяется на экране «Маржа»: откройте его в песочнице и нажмите «Применить»"

// marginOpportunity — найденная возможность: категория и следующее положение рычага.
type marginOpportunity struct {
	Key        string  `json:"key"`
	CategoryID string  `json:"category_id"`
	Category   string  `json:"category"`
	From       float64 `json:"weight_before"`
	To         float64 `json:"weight"`
	// Gain, GainRelevant — прирост индекса показанной маржи (все позиции / только релевантные).
	Gain         float64 `json:"margin_index_delta"`
	GainRelevant float64 `json:"margin_index_relevant_delta"`
	NDCGDelta    float64 `json:"ndcg_delta"`
	Queries      int     `json:"queries_in_scope"`
	Judged       int     `json:"judged_in_scope"`
	Shifted      int     `json:"queries_with_shifts"`
	Top3Changed  int     `json:"top3_changed"`
	before       out.MarginPosition
	after        out.MarginPosition
	sim          *out.MarginSimulation
}

// marginOppScan — итог перебора.
type marginOppScan struct {
	Set           string              `json:"set"`
	PolicyVersion int                 `json:"policy_version"`
	Weight        float64             `json:"weight"`
	Categories    int                 `json:"categories_checked"`
	Positions     []float64           `json:"positions"`
	Found         []marginOpportunity `json:"found"`
	// Skipped — категории без возможности: причина → сколько.
	Skipped   map[string]int `json:"skipped"`
	Synthetic bool           `json:"synthetic"`
	// Capture — ключ базовой выдачи (каталог, правила).
	SetHash        string `json:"set_hash"`
	CatalogVersion string `json:"catalog_version"`
	RulesVersion   int    `json:"rules_version"`
	Judged         int    `json:"judged"`
}

// scanMarginOpportunities — перебор категорий на базовой выдаче tune при действующей политике.
func (s *Services) scanMarginOpportunities(ctx context.Context) (*marginOppScan, error) {
	set := domain.EvalSetTune
	capture, _, err := s.marginCaptureFor(ctx, set, s.marginSimKeyFor(set))
	if err != nil {
		return nil, err
	}
	pol, _ := s.activeMarginPolicy(ctx)
	return s.marginOpportunitiesOn(capture, pol), nil
}

// marginOpportunitiesOn — чистый перебор на снятой выдаче: по каждой категории из выдачи размеченных запросов —
// следующее положение вверх от её действующего веса.
func (s *Services) marginOpportunitiesOn(capture *marginCapture, pol activeMargin) *marginOppScan {
	scan := &marginOppScan{Set: capture.set, PolicyVersion: pol.Version, Weight: pol.Weight, Positions: MarginPositions(),
		Skipped: map[string]int{}, Synthetic: s.marginSynthetic(), SetHash: capture.setHash, CatalogVersion: capture.catalog,
		RulesVersion: capture.rules, Found: []marginOpportunity{}}
	cats := map[string]bool{}
	for _, q := range capture.queries {
		if len(q.q.Judgments) == 0 {
			continue
		}
		scan.Judged++
		for _, c := range q.in.cands {
			if c.cat != "" {
				cats[c.cat] = true
			}
		}
	}
	ids := make([]string, 0, len(cats))
	for c := range cats {
		ids = append(ids, c)
	}
	sort.Strings(ids)
	scan.Categories = len(ids)
	for _, cat := range ids {
		cur := pol.weights().of(marginCand{cat: cat})
		next, ok := nextMarginPosition(cur)
		if !ok {
			scan.Skipped["at_max"]++
			continue
		}
		sim := s.marginSimulateCategory(capture, pol, cat, nil)
		var before, after *out.MarginPosition
		for i := range sim.Items {
			switch {
			case sameWeight(sim.Items[i].Weight, cur):
				before = &sim.Items[i]
			case sameWeight(sim.Items[i].Weight, next):
				after = &sim.Items[i]
			}
		}
		if before == nil || after == nil {
			scan.Skipped["no_position"]++
			continue
		}
		reason := marginOppReject(*before, *after, sim.Scope)
		if reason != "" {
			scan.Skipped[reason]++
			continue
		}
		scan.Found = append(scan.Found, marginOpportunity{Key: marginOppKey(cat, next), CategoryID: cat,
			Category: s.categoryName(cat), From: cur, To: next, Gain: evalRound4(after.MarginIndex - before.MarginIndex),
			GainRelevant: evalRound4(after.MarginIndexRelevant - before.MarginIndexRelevant),
			NDCGDelta:    evalRound4(after.NDCG10 - before.NDCG10), Queries: sim.Scope.QueriesInScope,
			Judged: sim.Scope.JudgedInScope, Shifted: after.QueriesWithShifts, Top3Changed: after.Top3Changed,
			before: *before, after: *after, sim: sim})
	}
	sort.SliceStable(scan.Found, func(i, j int) bool {
		a, b := scan.Found[i], scan.Found[j]
		if a.Gain != b.Gain {
			return a.Gain > b.Gain
		}
		return a.CategoryID < b.CategoryID
	})
	return scan
}

// marginOppReject — почему сдвиг категории на одно положение не предлагается ("" — предлагается).
func marginOppReject(before, after out.MarginPosition, scope *out.MarginScope) string {
	switch {
	case scope == nil || scope.JudgedInScope == 0:
		return "no_judged_queries"
	case !after.Available:
		return "beyond_stop"
	case after.ShareOverEps > 0:
		return "over_epsilon"
	case after.NDCG10 < before.NDCG10-marginOppNDCGTol:
		return "quality_drop"
	case after.ExactTop1Changed > 0:
		return "exact_top1"
	case after.QueriesWithShifts <= before.QueriesWithShifts:
		return "no_shift"
	case after.MarginIndex-before.MarginIndex < marginOppMinGain:
		return "no_margin_gain"
	}
	return ""
}

// nextMarginPosition — следующее положение рычага вверх.
func nextMarginPosition(w float64) (float64, bool) {
	for _, p := range marginPositions {
		if p > w+1e-9 {
			return p, true
		}
	}
	return 0, false
}

func marginOppKey(cat string, w float64) string {
	return fmt.Sprintf("%s%s:%.2f", marginOppKeyPrefix, cat, w)
}

// marginOppPatch — patch предложения margin_policy: что применить и как воспроизвести.
type marginOppPatch struct {
	CategoryID    string  `json:"category_id"`
	Category      string  `json:"category"`
	WeightBefore  float64 `json:"weight_before"`
	Weight        float64 `json:"weight"`
	GlobalWeight  float64 `json:"global_weight"`
	PolicyVersion int     `json:"policy_version"`
	// Simulate — тело POST /api/margin/simulate, которое даёт те же числа.
	Simulate in.MarginSimulateRequest `json:"simulate"`
	// Apply — тело POST /api/margin/policy для экрана «Маржа» (proposal_id подставляет фронт).
	Apply map[string]any `json:"apply"`
}

// runMarginOpportunity — этап агента «Коммерция»: перебор рычага маржи по категориям, предложения в очередь.
// Ошибка этапа не прерывает запуск: шаг с ok=false.
func (cr *commerceRun) runMarginOpportunity(ctx context.Context) {
	rc := cr.rc
	a, s, rs := rc.a, rc.a.s, rc.rs
	if !s.features.MarginPolicy {
		a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepThought, OK: true,
			Title: "Маржа: политика выключена (FEATURE_MARGIN_POLICY=off) — рычаг не перебираю"})
		return
	}
	t := time.Now()
	scan, err := s.scanMarginOpportunities(ctx)
	args := map[string]any{"set": domain.EvalSetTune, "scope": "category", "step": "одно положение вверх",
		"positions": MarginPositions()}
	if err != nil {
		a.toolStep(ctx, rs, nil, ToolSimulateMargin, args, map[string]any{"error": err.Error()},
			"Маржа: симуляция рычага не выполнена — "+shortText(err.Error(), 160), false, time.Since(t))
		rc.episode["margin_opportunity"] = map[string]any{"error": err.Error()}
		return
	}
	found := make([]map[string]any, 0, len(scan.Found))
	for _, o := range scan.Found {
		found = append(found, map[string]any{"key": o.Key, "category": o.Category, "weight_before": o.From, "weight": o.To,
			"margin_index_delta": o.Gain, "ndcg_delta": o.NDCGDelta, "queries_in_scope": o.Queries})
	}
	a.toolStep(ctx, rs, nil, ToolSimulateMargin, args,
		map[string]any{"policy_version": scan.PolicyVersion, "weight": scan.Weight, "categories_checked": scan.Categories,
			"judged": scan.Judged, "found": found, "skipped": scan.Skipped, "set_hash": scan.SetHash, "synthetic": scan.Synthetic},
		fmt.Sprintf("Маржа: перебираю рычаг по %d категориям на %d размеченных запросах tune — без потери качества "+
			"можно сдвинуть: %d", scan.Categories, scan.Judged, len(scan.Found)), true, time.Since(t))
	metrics := map[string]any{"categories_checked": scan.Categories, "found": len(scan.Found), "skipped": scan.Skipped,
		"policy_version": scan.PolicyVersion}
	created := 0
	for _, o := range scan.Found {
		if created >= marginOppMax || ctx.Err() != nil {
			break
		}
		if cr.marginMemorySkip(ctx, o) {
			continue
		}
		if err := cr.proposeMargin(ctx, scan, o); err != nil {
			key := o.Key
			a.step(ctx, rs, &domain.AgentStep{Kind: domain.StepError, ClusterID: &key,
				Title: "Маржа: предложение не записано — " + shortText(err.Error(), 160)})
			continue
		}
		created++
	}
	metrics["proposals"] = created
	rc.episode["margin_opportunity"] = metrics
}

// marginMemorySkip — по категории уже ждёт предложение или человек отклонил это положение.
func (cr *commerceRun) marginMemorySkip(ctx context.Context, o marginOpportunity) bool {
	rc := cr.rc
	catPrefix := marginOppKeyPrefix + o.CategoryID + ":"
	var title string
	for _, p := range rc.proposals {
		if p.Status == domain.ProposalPending && strings.HasPrefix(p.ClusterKey, catPrefix) {
			title = fmt.Sprintf("Память: по марже в категории «%s» уже есть предложение, оно ждёт решения — пропускаю", o.Category)
		}
	}
	for _, d := range rc.decisions {
		if title == "" && d.ClusterKey == o.Key && d.Decision == domain.ProposalRejected {
			title = fmt.Sprintf("Память: вес %s в категории «%s» человек отклонил %s, причина: %s%s — не предлагаю",
				marginWeightLabel(o.To), o.Category, d.CreatedAt.Format("02.01"), rejectLabels[d.Reason], commentSuffix(d.Comment))
		}
	}
	if title == "" {
		return false
	}
	key := o.Key
	rc.a.step(ctx, rc.rs, &domain.AgentStep{Kind: domain.StepMemory, ClusterID: &key, OK: true, Title: title})
	return true
}

// marginOppLabel — подпись в очереди.
func marginOppLabel(o marginOpportunity) string {
	return fmt.Sprintf("Маржа: «%s» — рычаг %s → %s", o.Category, marginWeightLabel(o.From), marginWeightLabel(o.To))
}

// proposeMargin — предложение margin_policy с числами симуляции и запросом для их воспроизведения.
func (cr *commerceRun) proposeMargin(ctx context.Context, scan *marginOppScan, o marginOpportunity) error {
	rc := cr.rc
	s := rc.a.s
	key, label := o.Key, marginOppLabel(o)
	sim := in.MarginSimulateRequest{Set: scan.Set, Category: o.CategoryID, Positions: []float64{o.From, o.To}}
	patch := marginOppPatch{CategoryID: o.CategoryID, Category: o.Category, WeightBefore: o.From, Weight: o.To,
		GlobalWeight: scan.Weight, PolicyVersion: scan.PolicyVersion, Simulate: sim,
		Apply: map[string]any{"categories": map[string]float64{o.CategoryID: o.To}}}
	text := fmt.Sprintf("В категории «%s» рычаг маржи можно сдвинуть на одно положение (%s → %s): NDCG@10 на %d "+
		"размеченных запросах tune %s, ни один запрос не теряет больше ε, первые места точных запросов на месте; индекс "+
		"показанной маржи %s (по релевантным %s). Затронуто запросов: %d из %d с позициями категории.",
		o.Category, marginWeightLabel(o.From), marginWeightLabel(o.To), scan.Judged, ndcgChangeText(o.NDCGDelta),
		ruSigned4(o.Gain), ruSigned4(o.GainRelevant), o.Shifted, o.Queries)
	affected := o.after.Affected
	if len(affected) > 10 {
		affected = affected[:10]
	}
	eval := map[string]any{"set": scan.Set, "set_hash": scan.SetHash, "catalog_version": scan.CatalogVersion,
		"rules_version": scan.RulesVersion, "policy_version": scan.PolicyVersion, "judged": o.sim.Judged,
		"base_ndcg10": o.sim.BaseNDCG10, "epsilon": o.sim.Epsilon, "scope": o.sim.Scope,
		"before": marginPositionBrief(o.before), "after": marginPositionBrief(o.after),
		"margin_index_delta": o.Gain, "margin_index_relevant_delta": o.GainRelevant, "ndcg_delta": o.NDCGDelta,
		"affected": affected, "simulate_request": sim, "passed": true, "synthetic": scan.Synthetic,
		// commit — сборка, снявшая базовую выдачу: числа воспроизводятся тем же кодом на том же каталоге и правилах.
		"commit": buildCommit()}
	evidence := map[string]any{"finding_key": key, "finding_text": text, "method": "перебор положений рычага по " +
		"категориям на базовой выдаче tune (marginSimulateCategory), одно положение вверх от действующего; ограничения " +
		"рычага те же, что в живом поиске", "scan": map[string]any{"categories_checked": scan.Categories,
		"found": len(scan.Found), "skipped": scan.Skipped}, "evidence_level": "E1",
		"technical_note": "качество проверено кодом на размеченном наборе tune: NDCG@10, доля запросов с потерей > ε, " +
			"точные запросы",
		"economic_note": "индекс показанной маржи — перцентиль маржи в категории по показам первой десятки, не рубли; " +
			"маржа в данных синтетическая; эффект в деньгах — только A/B-тестом",
		"synthetic": scan.Synthetic}
	q := ""
	if len(affected) > 0 {
		q = affected[0].Query
	}
	p := &domain.Proposal{RunID: rc.rs.run.ID, ClusterID: &key, ClusterKey: key, ClusterLabel: label,
		ClusterQueries: toJSON([]clusterQuery{{Text: q}}), Diagnosis: domain.DiagMarginOpportunity, DiagnosisText: text,
		Role: domain.RoleEcom, Action: domain.ActionMarginPolicy, Patch: toJSON(patch), PatchKeys: toJSON([]string{key}),
		Evaluation: toJSON(eval), Evidence: toJSON(evidence), HypothesesTried: 1, BaseVersion: scan.PolicyVersion,
		Status: domain.ProposalPending, EvidenceLevel: "E1", TechnicalStatus: domain.TechVerified,
		EconomicStatus: domain.EconEstimated}
	if err := s.store().CreateProposal(context.WithoutCancel(ctx), p); err != nil {
		return err
	}
	rc.rs.mu.Lock()
	rc.rs.run.ProposalsCreated++
	rc.rs.mu.Unlock()
	rc.results = append(rc.results, map[string]any{"cluster_id": key, "label": label, "diagnosis": domain.DiagMarginOpportunity,
		"outcome": "proposal", "hypotheses": 1, "rejected": 0, "tool_calls": 0, "proposal_id": p.ID.String()})
	rc.a.step(ctx, rc.rs, &domain.AgentStep{Kind: domain.StepProposal, ClusterID: &key, OK: true,
		Title:  fmt.Sprintf("Готовлю предложение: %s — в очередь (%s, открыть в песочнице «Маржа»)", label, domain.DiagMarginOpportunity),
		Result: toJSON(map[string]any{"proposal_id": p.ID.String(), "patch": patch})})
	return nil
}

func ndcgChangeText(d float64) string {
	if d > -marginOppNDCGTol && d < marginOppNDCGTol {
		return "не меняется"
	}
	return ruSigned4(d)
}

// ruSigned4 — число со знаком и десятичной запятой: +0,0002.
func ruSigned4(v float64) string { return strings.Replace(fmt.Sprintf("%+.4f", v), ".", ",", 1) }

// marginPositionBrief — показатели положения без списка затронутых (он отдельно).
func marginPositionBrief(p out.MarginPosition) out.MarginPosition {
	p.Affected = nil
	return p
}

// marginProposalFor — предложение, которое применяется на экране «Маржа»: ждёт решения, действие margin_policy,
// и новая настройка ставит его категории предложенный вес.
func (s *Services) marginProposalFor(ctx context.Context, id string, cur, next activeMargin) (*domain.Proposal, error) {
	p, err := s.loadProposal(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Action != domain.ActionMarginPolicy {
		return nil, errs.NewValidationError(MsgMarginProposal)
	}
	if p.Status != domain.ProposalPending {
		return nil, errs.NewConflictError(MsgProposalDecided)
	}
	var patch marginOppPatch
	if err := json.Unmarshal(p.Patch, &patch); err != nil {
		return nil, errs.NewInternalError("Правка предложения не разбирается").WithCause(err)
	}
	if !sameWeight(next.weights().of(marginCand{cat: patch.CategoryID}), patch.Weight) || !sameWeight(next.Weight, cur.Weight) {
		return nil, errs.NewValidationError(MsgMarginProposalChange)
	}
	return p, nil
}

// acceptMarginProposal — предложение принято применением на экране «Маржа»: статус, версия политики, решение
// (память). Версия уже записана — сбой здесь только в журнал.
func (s *Services) acceptMarginProposal(ctx context.Context, p *domain.Proposal, version int) {
	now := s.now().UTC()
	v := version
	p.Status, p.DecidedAt, p.DecidedReason, p.DecidedComment, p.ConfigVersion = domain.ProposalAccepted, &now, "accepted",
		"применено на экране «Маржа»", &v
	dec := &domain.Decision{ProposalID: p.ID, Decision: domain.ProposalAccepted, Reason: "accepted", Comment: p.DecidedComment,
		ClusterKey: p.ClusterKey, ClusterLabel: p.ClusterLabel, Diagnosis: p.Diagnosis, PatchKeys: p.PatchKeys}
	if err := s.store().ApplyRulesChange(context.WithoutCancel(ctx), &repositories.RulesChange{Proposal: p, Decision: dec}); err != nil {
		zap.L().Warn("маржа: версия записана, предложение не отмечено принятым", zap.String("proposal_id", p.ID.String()),
			zap.Int("version", version), zap.Error(err))
		return
	}
	zap.L().Info("маржа: предложение агента применено", zap.String("proposal_id", p.ID.String()), zap.Int("version", version))
}
