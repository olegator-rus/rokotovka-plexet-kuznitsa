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

var proposalStatuses = []string{domain.ProposalPending, domain.ProposalAccepted, domain.ProposalRejected, domain.ProposalRolledBack}
var proposalRoles = []string{domain.RoleEcom, domain.RoleProduct, domain.RoleContent}

// Proposals — GET /api/proposals?status=&role=.
func (s *Services) Proposals(ctx context.Context, status, role string) (*out.ProposalList, error) {
	st := s.store()
	if st == nil {
		return nil, errs.NewUnavailableError(MsgNoAgentStore)
	}
	var bad []errs.FieldError
	if status != "" && !slices.Contains(proposalStatuses, status) {
		bad = append(bad, errs.FieldError{Field: "status", Code: errs.CodeInvalidFormat, Message: "Статус — pending, accepted, rejected или rolled_back"})
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
// (проверка версии от гонок), сразу в B4. Задача (ticket) → «взята в работу», правила не меняются.
func (s *Services) ApproveProposal(ctx context.Context, id string, req in.ApproveRequest) (*out.ProposalDecision, error) {
	p, err := s.loadProposal(ctx, id)
	if err != nil {
		return nil, err
	}
	if p.Status != domain.ProposalPending {
		return nil, errs.NewConflictError(MsgProposalDecided)
	}
	st := s.store()
	now := s.now().UTC()
	comment := truncateRunes(strings.TrimSpace(req.Comment), 500)
	p.Status, p.DecidedAt, p.DecidedReason, p.DecidedComment = domain.ProposalAccepted, &now, "accepted", comment
	dec := &domain.Decision{ProposalID: p.ID, Decision: domain.ProposalAccepted, Reason: "accepted", Comment: comment,
		ClusterKey: p.ClusterKey, ClusterLabel: p.ClusterLabel, Diagnosis: p.Diagnosis, PatchKeys: p.PatchKeys}
	ch := &repositories.RulesChange{Proposal: p, Decision: dec}
	if p.Action == domain.ActionRulesPatch {
		var patch RulePatch
		if err := json.Unmarshal(p.Patch, &patch); err != nil {
			return nil, errs.NewInternalError("Правка предложения не разбирается").WithCause(err)
		}
		cur, err := s.activeRules(ctx)
		if err != nil {
			return nil, err
		}
		next := cur.applyPatch(patch)
		diff, _ := json.Marshal(rulesDiff(cur, next))
		pid := p.ID.String()
		ver := cur.Version + 1
		p.ConfigVersion = &ver
		ch.Base = cur.Version
		ch.Version = &domain.ConfigVersion{Version: ver, Source: "agent", Author: "agent+human",
			Comment: proposalSummary(p, patch), ProposalID: &pid, Rules: next.rulesJSON(), Diff: diff}
	}
	if err := st.ApplyRulesChange(ctx, ch); err != nil {
		return nil, rulesError(err)
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
		CreatedAt: p.CreatedAt, DecidedAt: p.DecidedAt, ConfigVersion: p.ConfigVersion, Effect: rawOrNull(p.Effect)}
	if p.DecidedAt != nil {
		v.DecidedReason, v.DecidedComment = &p.DecidedReason, &p.DecidedComment
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
