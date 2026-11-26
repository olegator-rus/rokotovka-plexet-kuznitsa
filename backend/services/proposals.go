package services

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"

	"github.com/google/uuid"
	"go.uber.org/zap"
)

// Очередь предложений агента: принять — новая версия правил (или «взять в работу» для
// задачи), отклонить — только с причиной; причина уходит в память агента («не предлагать снова»).

const (
	MsgProposalNotFound = "Предложение не найдено"
	MsgRejectReason     = "Укажите причину отказа: too_broad, wrong_product, not_now или other"
	MsgRejectComment    = "Для причины «другое» напишите комментарий"
)

var proposalStatuses = []string{domain.ProposalPending, domain.ProposalAccepted, domain.ProposalRejected, domain.ProposalRolledBack, domain.ProposalStale}
var proposalRoles = []string{domain.RoleEcom, domain.RoleProduct, domain.RoleContent}

// Proposals — GET /api/proposals?status=&role=.
func (s *Services) Proposals(ctx context.Context, status, role string) (*out.ProposalList, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	var bad []errs.FieldError
	if status != "" && !slices.Contains(proposalStatuses, status) {
		bad = append(bad, errs.FieldError{Field: "status", Code: errs.CodeInvalidFormat, Message: "Статус — pending, accepted, rejected, rolled_back или stale"})
	}
	if role != "" && !slices.Contains(proposalRoles, role) {
		bad = append(bad, errs.FieldError{Field: "role", Code: errs.CodeInvalidFormat, Message: "Роль — ecom, product или content"})
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidQuery).WithDetails(bad...)
	}
	rows, err := st.ListProposals(ctx, status, role)
	if err != nil {
		return nil, err
	}
	res := &out.ProposalList{Items: make([]out.Proposal, 0, len(rows))}
	for i := range rows {
		res.Items = append(res.Items, proposalView(&rows[i]))
	}
	return res, nil
}

// Proposal — GET /api/proposals/{id}.
func (s *Services) Proposal(ctx context.Context, id string) (*out.Proposal, error) {
	p, err := s.loadProposal(ctx, id)
	if err != nil {
		return nil, err
	}
	v := proposalView(p)
	return &v, nil
}

func (s *Services) loadProposal(ctx context.Context, id string) (*domain.Proposal, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	uid, err := uuid.Parse(id)
	if err != nil {
		return nil, errs.NewValidationError(errs.MsgInvalidID)
	}
	p, err := st.GetProposal(ctx, uid)
	if err != nil {
		return nil, err
	}
	if p == nil {
		return nil, errs.NewNotFoundError(MsgProposalNotFound)
	}
	return p, nil
}

// ApproveProposal — POST /api/proposals/{id}/approve. Правка правил → версия N+1 поверх действующей
// (проверка версии от гонок), сразу в B4. Если правила сменились после прогона агента (base_version ≠
// действующей), правка сначала перепрогоняется на действующих правилах (цель + регрессия): не прошла —
// 409 с причинами, статус не меняется; прошла — применяется, новый прогон записывается в предложение.
// Задача (ticket) → «взята в работу», правила не меняются.
func (s *Services) ApproveProposal(ctx context.Context, id string, req in.ApproveRequest) (*out.ProposalDecision, error) {
	p, err := s.loadProposal(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Status != domain.ProposalPending {
		return nil, errs.NewConflictError(MsgProposalDecided)
	}
	st := s.store()
	comment := truncateRunes(strings.TrimSpace(req.Comment), 500)
	if p.Action == domain.ActionRelationPatch {
		return s.approveRelationPatch(ctx, p, comment)
	}
	var version *domain.ConfigVersion
	base, rechecked := 0, false
	if p.Action == domain.ActionRulesPatch {
		var patch RulePatch
		if err := json.Unmarshal(p.Patch, &patch); err != nil {
			return nil, errs.NewInternalError("Правка предложения не разбирается").WithCause(err)
		}
		cur, err := s.activeRules(ctx)
		if err != nil {
			return nil, err
		}
		if p.BaseVersion != cur.Version {
			if err := s.recheckProposal(ctx, p, patch, cur); err != nil {
				return nil, err
			}
			rechecked = true
		}
		next := cur.applyPatch(patch)
		diff, _ := json.Marshal(rulesDiff(cur, next))
		pid := p.ID.String()
		ver := cur.Version + 1
		p.ConfigVersion = &ver
		base = cur.Version
		version = &domain.ConfigVersion{Version: ver, Source: "agent", Author: "agent+human",
			Comment: proposalSummary(p, patch), ProposalID: &pid, Rules: next.rulesJSON(), Diff: diff}
	}
	now := s.now().UTC()
	p.Status, p.DecidedAt, p.DecidedReason, p.DecidedComment = domain.ProposalAccepted, &now, "accepted", comment
	dec := &domain.Decision{ProposalID: p.ID, Decision: domain.ProposalAccepted, Reason: "accepted", Comment: comment,
		ClusterKey: p.ClusterKey, ClusterLabel: p.ClusterLabel, Diagnosis: p.Diagnosis, PatchKeys: p.PatchKeys}
	ch := &repositories.RulesChange{Proposal: p, Decision: dec, Base: base, Version: version}
	if err := st.ApplyRulesChange(ctx, ch); err != nil {
		return nil, rulesError(err)
	}
	if rechecked {
		// Решение уже записано; новый прогон и база — отдельной записью (ApplyRulesChange меняет только статус).
		bv := p.BaseVersion
		if _, err := st.UpdateProposalCheck(context.WithoutCancel(ctx), p.ID, domain.ProposalAccepted,
			repositories.ProposalCheck{Evidence: p.Evidence, Evaluation: p.Evaluation, BaseVersion: &bv}); err != nil {
			zap.L().Warn("перепроверка: новый прогон не записан в принятое предложение", zap.Error(err))
		}
	}
	res := &out.ProposalDecision{Proposal: proposalView(p), ConfigVersion: p.ConfigVersion}
	if p.ConfigVersion != nil {
		s.rulesChanged(*p.ConfigVersion)
		zap.L().Info("предложение принято: новая версия правил", zap.String("proposal_id", id), zap.Int("version", *p.ConfigVersion))
	} else {
		zap.L().Info("предложение взято в работу", zap.String("proposal_id", id))
	}
	return res, nil
}

// MsgRecheckFailed — правка не прошла перепрогон на сменившихся правилах.
const MsgRecheckFailed = "Правила изменились после проверки агента (v%d → v%d), и на действующих правилах правка не прошла прогон: %s. Предложение осталось в очереди"

// recheckProposal — перепрогон правки на действующих правилах (M2). Результат пишется в предложение:
// прошла — новая evaluation и base_version (прежний прогон — в evidence.initial_evaluation); не прошла —
// evidence.recheck и 409, статус не меняется.
func (s *Services) recheckProposal(ctx context.Context, p *domain.Proposal, patch RulePatch, cur *ruleSet) error {
	var qs []clusterQuery
	_ = json.Unmarshal(p.ClusterQueries, &qs)
	cluster := &failedCluster{Key: p.ClusterKey, Label: p.ClusterLabel, Queries: qs}
	eval := s.agent.evalHook
	if eval == nil {
		sb, err := s.newSandbox(ctx, "recheck-"+p.ID.String()[:8], cur, s.agent.opts)
		if err != nil {
			return err
		}
		eval = sb.evaluate
	}
	ev, err := eval(ctx, cluster, patch)
	if err != nil {
		return errs.NewUnavailableError("Перепроверка правки не завершилась — повторите").WithCause(err)
	}
	ev.RulesVersion = cur.Version
	var evidence map[string]any
	_ = json.Unmarshal(p.Evidence, &evidence)
	if evidence == nil {
		evidence = map[string]any{}
	}
	recheck := map[string]any{"from_version": p.BaseVersion, "to_version": cur.Version, "passed": ev.Passed,
		"reasons": ev.Reasons, "evaluation": ev, "checked_at": s.now().UTC()}
	if !ev.Passed {
		evidence["recheck"] = recheck
		p.Evidence = toJSON(evidence)
		// Только evidence и только пока предложение ждёт: за время перепрогона его могли отклонить или перевести
		// в stale — тогда копия p устарела и не пишется (ревью-2 n1).
		ok, err := s.store().UpdateProposalCheck(context.WithoutCancel(ctx), p.ID, domain.ProposalPending,
			repositories.ProposalCheck{Evidence: p.Evidence})
		if err != nil {
			zap.L().Warn("перепроверка: результат не записан в предложение", zap.Error(err))
		} else if !ok {
			zap.L().Info("перепроверка: предложение уже не ждёт решения — результат не записан", zap.String("proposal_id", p.ID.String()))
		}
		zap.L().Info("предложение не прошло перепроверку", zap.String("proposal_id", p.ID.String()),
			zap.Int("base", p.BaseVersion), zap.Int("active", cur.Version))
		return errs.NewConflictError(fmt.Sprintf(MsgRecheckFailed, p.BaseVersion, cur.Version, strings.Join(ev.Reasons, "; ")))
	}
	evidence["initial_evaluation"] = json.RawMessage(p.Evaluation)
	evidence["recheck"] = recheck
	p.Evidence = toJSON(evidence)
	p.Evaluation = toJSON(ev)
	p.BaseVersion = cur.Version
	return nil
}

// RejectProposal — POST /api/proposals/{id}/reject: причина обязательна, уходит в память агента.
func (s *Services) RejectProposal(ctx context.Context, id string, req in.RejectRequest) (*out.ProposalDecision, error) {
	reason := strings.TrimSpace(req.Reason)
	comment := truncateRunes(strings.TrimSpace(req.Comment), 500)
	if !slices.Contains(domain.RejectReasons, reason) {
		return nil, errs.NewValidationError(MsgRejectReason).WithDetails(errs.FieldError{Field: "reason", Code: errs.CodeInvalidFormat, Message: MsgRejectReason})
	}
	if reason == domain.RejectOther && comment == "" {
		return nil, errs.NewValidationError(MsgRejectComment).WithDetails(errs.FieldError{Field: "comment", Code: errs.CodeInvalidFormat, Message: MsgRejectComment})
	}
	p, err := s.loadProposal(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Status != domain.ProposalPending {
		return nil, errs.NewConflictError(MsgProposalDecided)
	}
	now := s.now().UTC()
	p.Status, p.DecidedAt, p.DecidedReason, p.DecidedComment = domain.ProposalRejected, &now, reason, comment
	terms := domain.JSONB(nil)
	if ev := proposalTerms(p); len(ev) > 0 {
		terms, _ = domain.ToJSONB(ev)
	}
	dec := &domain.Decision{ProposalID: p.ID, Decision: domain.ProposalRejected, Reason: reason, Comment: comment,
		ClusterKey: p.ClusterKey, ClusterLabel: p.ClusterLabel, Diagnosis: p.Diagnosis, Terms: terms, PatchKeys: p.PatchKeys}
	if err := s.store().ApplyRulesChange(ctx, &repositories.RulesChange{Proposal: p, Decision: dec}); err != nil {
		return nil, rulesError(err)
	}
	zap.L().Info("предложение отклонено", zap.String("proposal_id", id), zap.String("reason", reason))
	return &out.ProposalDecision{Proposal: proposalView(p)}, nil
}

// AgentMemory — GET /api/agent/memory: решения человека с причинами.
func (s *Services) AgentMemory(ctx context.Context) (*out.MemoryList, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	ds, err := st.ListDecisions(ctx)
	if err != nil {
		return nil, err
	}
	res := &out.MemoryList{Items: make([]out.MemoryItem, 0, len(ds))}
	for _, d := range ds {
		res.Items = append(res.Items, out.MemoryItem{ClusterLabel: d.ClusterLabel, ClusterKey: d.ClusterKey,
			ProposalID: d.ProposalID.String(), Diagnosis: d.Diagnosis, Decision: d.Decision, Reason: d.Reason,
			Comment: d.Comment, TS: d.CreatedAt})
	}
	return res, nil
}

// proposalSummary — описание версии правил одной строкой.
func proposalSummary(p *domain.Proposal, patch RulePatch) string {
	var parts []string
	for _, a := range patch.Add {
		parts = append(parts, fmt.Sprintf("«%s» → «%s»", a.Alias, a.Canonical))
	}
	for _, a := range patch.Remove {
		parts = append(parts, fmt.Sprintf("убран «%s»", a.Alias))
	}
	return fmt.Sprintf("%s: %s (кластер «%s»)", p.Diagnosis, strings.Join(parts, ", "), p.ClusterLabel)
}

// proposalTerms — термины кластера предложения (из доказательств).
func proposalTerms(p *domain.Proposal) []string {
	var ev struct {
		Terms []string `json:"terms"`
	}
	_ = json.Unmarshal(p.Evidence, &ev)
	return ev.Terms
}

// proposalView — предложение в форме ответа API.
func proposalView(p *domain.Proposal) out.Proposal {
	v := out.Proposal{ID: p.ID.String(), RunID: p.RunID.String(), Diagnosis: p.Diagnosis, DiagnosisText: p.DiagnosisText,
		Role: p.Role, Action: p.Action, Patch: rawOrNull(p.Patch), Evaluation: rawOrNull(p.Evaluation),
		Evidence: rawOrNull(p.Evidence), HypothesesTried: p.HypothesesTried, BaseVersion: p.BaseVersion, Status: p.Status,
		CreatedAt: p.CreatedAt, DecidedAt: p.DecidedAt, ConfigVersion: p.ConfigVersion, Effect: rawOrNull(p.Effect),
		EvidenceLevel: p.EvidenceLevel, TechnicalStatus: p.TechnicalStatus, EconomicStatus: p.EconomicStatus,
		PriorityScore: p.PriorityScore}
	if p.DecidedAt != nil {
		v.DecidedReason, v.DecidedComment = &p.DecidedReason, &p.DecidedComment
	}
	if p.Status == domain.ProposalStale {
		reason := p.DecidedReason
		if reason == "" {
			reason = domain.ProposalStaleReason
		}
		v.StaleReason = &reason
	}
	v.Cluster = out.ProposalCluster{Label: p.ClusterLabel, LostDemand: p.LostDemand, Queries: []out.ProposalQuery{}}
	if p.ClusterID != nil {
		v.Cluster.ID = *p.ClusterID
	}
	var qs []clusterQuery
	_ = json.Unmarshal(p.ClusterQueries, &qs)
	for _, q := range qs {
		v.Cluster.Queries = append(v.Cluster.Queries, out.ProposalQuery{Text: q.Text, Freq: q.Freq, Failures: q.Failures})
	}
	if len(v.Cluster.Queries) > 0 {
		v.CheckQuery = v.Cluster.Queries[0].Text
	}
	return v
}

// StaleProposalsOfRuns — `go run . proposals stale --run <id> … --reason "<текст>"`: ждущие предложения запусков
// до разреза журнала → stale (pre_holdout). Id запуска — полный или префикс не короче 8 знаков.
func (s *Services) StaleProposalsOfRuns(ctx context.Context, runs []string, comment string) ([]string, error) {
	var prefixes []string
	for _, r := range runs {
		r = strings.ToLower(strings.TrimSpace(r))
		if len(r) < 8 || strings.Trim(r, "0123456789abcdef-") != "" {
			return nil, errs.NewValidationError(fmt.Sprintf("id запуска %q: полный uuid или первые 8+ знаков", r))
		}
		prefixes = append(prefixes, r)
	}
	if len(prefixes) == 0 {
		return nil, errs.NewValidationError("нужен хотя бы один --run")
	}
	if strings.TrimSpace(comment) == "" {
		return nil, errs.NewValidationError("нужна причина --reason")
	}
	if !s.repos.Enabled() {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	return s.repos.StaleProposalsOfRuns(ctx, prefixes, domain.ProposalStalePreHoldout, strings.TrimSpace(comment))
}
