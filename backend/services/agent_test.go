package services

import (
	"context"
	"encoding/json"
	"errors"
	"sort"
	"strings"
	"sync"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/in"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"

	"github.com/google/uuid"
)

// memStore — хранилище агента в памяти: те же правила гонок, что у базы (проверка версии, статус pending).
type memStore struct {
	mu        sync.Mutex
	runs      map[uuid.UUID]domain.AgentRun
	steps     []domain.AgentStep
	clusters  map[uuid.UUID]domain.IssueCluster
	proposals map[uuid.UUID]domain.Proposal
	decisions []domain.Decision
	versions  []domain.ConfigVersion
	searches  []domain.SearchEvent
	behavior  []domain.BehaviorEvent
}

func newMemStore(rules domain.JSONB) *memStore {
	return &memStore{runs: map[uuid.UUID]domain.AgentRun{}, clusters: map[uuid.UUID]domain.IssueCluster{},
		proposals: map[uuid.UUID]domain.Proposal{}, versions: []domain.ConfigVersion{{Version: 1, Source: "import", Rules: rules}}}
}

func ensureID(m *domain.UUIDModel) {
	if m.ID == uuid.Nil {
		_ = m.BeforeCreate(nil)
	}
	if m.CreatedAt.IsZero() {
		m.CreatedAt = time.Now()
	}
}

func (m *memStore) CreateAgentRun(_ context.Context, r *domain.AgentRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ensureID(&r.UUIDModel)
	m.runs[r.ID] = *r
	return nil
}
func (m *memStore) SaveAgentRun(_ context.Context, r *domain.AgentRun) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.runs[r.ID] = *r
	return nil
}
func (m *memStore) GetAgentRun(_ context.Context, id uuid.UUID) (*domain.AgentRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r, ok := m.runs[id]
	if !ok {
		return nil, nil
	}
	return &r, nil
}
func (m *memStore) ListAgentRuns(context.Context, int) ([]domain.AgentRun, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []domain.AgentRun
	for _, r := range m.runs {
		res = append(res, r)
	}
	return res, nil
}
func (m *memStore) FailStaleAgentRuns(context.Context, string, time.Time) error { return nil }
func (m *memStore) CreateAgentStep(_ context.Context, st *domain.AgentStep) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ensureID(&st.UUIDModel)
	m.steps = append(m.steps, *st)
	return nil
}
func (m *memStore) SaveAgentStep(_ context.Context, st *domain.AgentStep) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for i := range m.steps {
		if m.steps[i].ID == st.ID {
			m.steps[i] = *st
		}
	}
	return nil
}
func (m *memStore) ListAgentSteps(_ context.Context, runID uuid.UUID, after int) ([]domain.AgentStep, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []domain.AgentStep
	for _, s := range m.steps {
		if s.RunID == runID && s.Seq > after {
			res = append(res, s)
		}
	}
	sort.Slice(res, func(i, j int) bool { return res[i].Seq < res[j].Seq })
	return res, nil
}
func (m *memStore) SaveIssueClusters(_ context.Context, rows []domain.IssueCluster) error {
	for i := range rows {
		_ = m.SaveIssueCluster(context.Background(), &rows[i])
	}
	return nil
}
func (m *memStore) SaveIssueCluster(_ context.Context, c *domain.IssueCluster) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ensureID(&c.UUIDModel)
	m.clusters[c.ID] = *c
	return nil
}
func (m *memStore) CreateProposal(_ context.Context, p *domain.Proposal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	ensureID(&p.UUIDModel)
	m.proposals[p.ID] = *p
	return nil
}
func (m *memStore) SaveProposal(_ context.Context, p *domain.Proposal) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.proposals[p.ID] = *p
	return nil
}
func (m *memStore) GetProposal(_ context.Context, id uuid.UUID) (*domain.Proposal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	p, ok := m.proposals[id]
	if !ok {
		return nil, nil
	}
	return &p, nil
}
func (m *memStore) ListProposals(_ context.Context, status, role string) ([]domain.Proposal, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	var res []domain.Proposal
	for _, p := range m.proposals {
		if (status == "" || p.Status == status) && (role == "" || p.Role == role) {
			res = append(res, p)
		}
	}
	return res, nil
}
func (m *memStore) ListDecisions(context.Context) ([]domain.Decision, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return append([]domain.Decision(nil), m.decisions...), nil
}
func (m *memStore) ListConfigVersions(context.Context) ([]domain.ConfigVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	res := append([]domain.ConfigVersion(nil), m.versions...)
	sort.Slice(res, func(i, j int) bool { return res[i].Version > res[j].Version })
	return res, nil
}
func (m *memStore) GetConfigVersion(_ context.Context, v int) (*domain.ConfigVersion, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, cv := range m.versions {
		if cv.Version == v {
			return &cv, nil
		}
	}
	return nil, nil
}
func (m *memStore) SetConfigVersionEffect(context.Context, int, domain.JSONB) error { return nil }
func (m *memStore) ApplyRulesChange(_ context.Context, ch *repositories.RulesChange) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch.Proposal != nil {
		p, ok := m.proposals[ch.Proposal.ID]
		if !ok || p.Status != domain.ProposalPending {
			return domain.ErrProposalDecided
		}
	}
	if ch.Version != nil {
		latest := 0
		for _, v := range m.versions {
			latest = max(latest, v.Version)
		}
		if latest != ch.Base {
			return domain.ErrRulesVersionConflict
		}
		ensureID(&ch.Version.UUIDModel)
		m.versions = append(m.versions, *ch.Version)
	}
	if ch.Proposal != nil {
		m.proposals[ch.Proposal.ID] = *ch.Proposal
	}
	if ch.Decision != nil && ch.Proposal != nil {
		ensureID(&ch.Decision.UUIDModel)
		m.decisions = append(m.decisions, *ch.Decision)
	}
	return nil
}
func (m *memStore) SearchEventsSince(context.Context, time.Time) ([]domain.SearchEvent, error) {
	return append([]domain.SearchEvent(nil), m.searches...), nil
}
func (m *memStore) BehaviorEventsSince(context.Context, time.Time) ([]domain.BehaviorEvent, error) {
	return append([]domain.BehaviorEvent(nil), m.behavior...), nil
}
func (m *memStore) AgentRunUsage(context.Context, string) (repositories.AgentUsage, error) {
	return repositories.AgentUsage{}, nil
}

// scriptModel — модель по сценарию: ответ на каждый ход — заданные вызовы инструментов.
type scriptModel struct {
	mu    sync.Mutex
	turns [][]llm.ToolCall
	seen  [][]llm.Message
}

func (m *scriptModel) Model() string { return "aliceai-llm-flash" }
func (m *scriptModel) Chat(_ context.Context, req llm.Request) (*llm.Response, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if req.Temperature == nil || *req.Temperature != 0 {
		return nil, errors.New("temperature должна быть 0")
	}
	m.seen = append(m.seen, req.Messages)
	if len(m.turns) == 0 {
		return &llm.Response{Message: llm.Message{Role: "assistant", Content: "готово"}}, nil
	}
	calls := m.turns[0]
	m.turns = m.turns[1:]
	// Как Alice AI: id вызова = имя инструмента (дубли делает уникальными цикл).
	for i := range calls {
		calls[i].ID = calls[i].Function.Name
	}
	return &llm.Response{Message: llm.Message{Role: "assistant", ToolCalls: calls},
		Usage: llm.Usage{PromptTokens: 1000, CompletionTokens: 50}}, nil
}

func tc(name string, args any) llm.ToolCall {
	b, _ := json.Marshal(args)
	return llm.ToolCall{Type: "function", Function: llm.FunctionCall{Name: name, Arguments: string(b)}}
}

// failedLog — журнал: «кис» трижды пустой, переформулировка в название с кликом.
func failedLog() ([]domain.SearchEvent, []domain.BehaviorEvent) {
	base := time.Now().Add(-48 * time.Hour)
	var ss []domain.SearchEvent
	var bs []domain.BehaviorEvent
	for i := 0; i < 3; i++ {
		sess := uuid.NewString()
		at := base.Add(time.Duration(i) * time.Hour)
		ev := domain.SearchEvent{RequestID: "r" + sess[:8], Query: "кис", ResultsCount: 0, SessionID: &sess}
		ev.CreatedAt = at
		ev2 := domain.SearchEvent{RequestID: "s" + sess[:8], Query: "kaspersky internet security", ResultsCount: 3, SessionID: &sess}
		ev2.CreatedAt = at.Add(20 * time.Second)
		ss = append(ss, ev, ev2)
		pid, rid := "kis-2dev-1y", ev2.RequestID
		b := domain.BehaviorEvent{Type: domain.EventClick, RequestID: &rid, SessionID: &sess, ProductID: &pid}
		b.CreatedAt = at.Add(30 * time.Second)
		bs = append(bs, b)
	}
	return ss, bs
}

// agentEnv — сервисы на фикстуре каталога, хранилище в памяти, модель по сценарию.
func agentEnv(t *testing.T, model *scriptModel) (*Services, *memStore) {
	t.Helper()
	env := imported(t, false)
	s := env.svc
	rules, _ := json.Marshal(map[string]any{"aliases": []ruleAlias{{Alias: "винда", Canonical: "windows", Kind: "slang", Target: "term"}}})
	ms := newMemStore(rules)
	ms.searches, ms.behavior = failedLog()
	s.agentStoreOverride = ms
	s.agent = newAgentService(s, model, AgentOptions{})
	return s, ms
}

func waitRun(t *testing.T, s *Services, id string) *domain.AgentRun {
	t.Helper()
	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		s.agent.mu.Lock()
		busy := s.agent.active != nil
		s.agent.mu.Unlock()
		if !busy {
			run, err := s.loadRun(t.Context(), id)
			if err != nil {
				t.Fatal(err)
			}
			return run
		}
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatal("запуск не закончился")
	return nil
}

func evalResult(beforeFound, afterFound, regWorse int) *patchEval {
	ev := &patchEval{}
	ev.Target.Before, ev.Target.After = evalCount{Found: beforeFound, Total: 1}, evalCount{Found: afterFound, Total: 1}
	ev.Regression.Total, ev.Regression.Worse = 50, regWorse
	sb := &sandbox{}
	ev.Passed, ev.Reasons = sb.admit(ev)
	return ev
}

// Цикл на модели по сценарию: первая гипотеза (широкий алиас на производителя) отклонена прогоном,
// вторая прошла и ушла в очередь; принятие создаёт версию 2; отказ по другому кластеру — память.
func TestAgentLoopHypothesisRevision(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	calls := 0
	s.agent.evalHook = func(_ context.Context, _ *failedCluster, p RulePatch) (*patchEval, error) {
		calls++
		if p.Add[0].Target == domain.AliasTargetVendor {
			return evalResult(0, 0, 4), nil // слишком широко
		}
		return evalResult(0, 1, 0), nil
	}
	model.turns = [][]llm.ToolCall{
		{tc("get_search_trace", map[string]any{"query": "кис"}), tc("search_catalog", map[string]any{"query": "kaspersky internet security", "expand": true})},
		{tc("create_sandbox_patch", map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "кис — это Касперский",
			"add": []map[string]any{{"alias": "кис", "canonical": "kaspersky", "target": "vendor", "target_id": "kaspersky", "term_vendor": "Kaspersky"}}})},
		{tc("run_evaluation", map[string]any{"patch_id": "p1"})},
		{tc("create_sandbox_patch", map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "кис — это Kaspersky Internet Security",
			"add": []map[string]any{{"alias": "кис", "canonical": "kaspersky internet security", "target": "family",
				"target_id": "kaspersky-internet-security", "term_vendor": "Касперский"}}})},
		{tc("run_evaluation", map[string]any{"patch_id": "p2"})},
		{tc("submit_proposal", map[string]any{"patch_id": "p2", "diagnosis_text": "кис — сокращение Kaspersky Internet Security"})},
	}
	started, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, s, started.RunID)
	if run.Status != domain.RunDone || run.ProposalsCreated != 1 || run.ClustersDone != 1 || calls != 2 {
		t.Fatalf("запуск: %+v, прогонов %d", run, calls)
	}
	var rejected, hyp2 bool
	for _, st := range ms.steps {
		if st.Kind == domain.StepHypothesis && st.HypothesisNo != nil && *st.HypothesisNo == 1 && st.RejectedReason != nil &&
			strings.Contains(*st.RejectedReason, "слишком широко") {
			rejected = true
		}
		if st.Kind == domain.StepCheck && st.HypothesisNo != nil && *st.HypothesisNo == 2 && st.OK {
			hyp2 = true
		}
	}
	if !rejected || !hyp2 {
		t.Fatalf("гипотеза 1 должна быть отклонена прогоном, 2 — пройти: %+v", ms.steps)
	}
	// Дубли id вызовов (id = имя инструмента) сделаны уникальными.
	for _, msgs := range model.seen {
		ids := map[string]bool{}
		for _, m := range msgs {
			for _, c := range m.ToolCalls {
				if ids[c.ID] {
					t.Fatalf("повтор id вызова %s", c.ID)
				}
				ids[c.ID] = true
			}
		}
	}
	// Данные кластера — в сообщении пользователя JSON-ом, не в системном промпте.
	if !strings.Contains(model.seen[0][1].Content, `"кис"`) || strings.Contains(model.seen[0][0].Content, "кис") {
		t.Fatal("тексты запросов должны идти данными в JSON")
	}

	list, err := s.Proposals(t.Context(), domain.ProposalPending, "")
	if err != nil || len(list.Items) != 1 {
		t.Fatalf("очередь: %+v %v", list, err)
	}
	p := list.Items[0]
	if p.HypothesesTried != 2 || p.Action != domain.ActionRulesPatch || p.CheckQuery != "кис" {
		t.Fatalf("предложение: %+v", p)
	}
	res, err := s.ApproveProposal(t.Context(), p.ID, in.ApproveRequest{})
	if err != nil || res.ConfigVersion == nil || *res.ConfigVersion != 2 {
		t.Fatalf("принятие: %+v %v", res, err)
	}
	active, _ := s.activeRules(t.Context())
	if active.Version != 2 || active.find("кис") == nil || active.find("винда") == nil {
		t.Fatalf("версия 2 должна содержать старые и новый алиасы: %+v", active)
	}
	// Повторное решение — 409.
	if _, err := s.ApproveProposal(t.Context(), p.ID, in.ApproveRequest{}); !isConflict(err) {
		t.Fatalf("повторное принятие: %v", err)
	}
}

func isConflict(err error) bool {
	var e *errs.Error
	return errors.As(err, &e) && e.HTTPStatus() == 409
}

// Память: отклонённая человеком правка в следующем запуске отклоняется кодом, без прогона.
func TestAgentMemoryBlocksRejectedPatch(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	evals := 0
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) {
		evals++
		return evalResult(0, 1, 0), nil
	}
	patch := map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "кис — KIS",
		"add": []map[string]any{{"alias": "кис", "canonical": "kaspersky internet security", "target": "family",
			"target_id": "kaspersky-internet-security", "term_vendor": "Kaspersky"}}}
	model.turns = [][]llm.ToolCall{
		{tc("create_sandbox_patch", patch)}, {tc("run_evaluation", map[string]any{"patch_id": "p1"})},
		{tc("submit_proposal", map[string]any{"patch_id": "p1", "diagnosis_text": "сокращение"})},
	}
	r1, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, s, r1.RunID)
	list, _ := s.Proposals(t.Context(), domain.ProposalPending, "")
	if len(list.Items) != 1 {
		t.Fatalf("очередь: %+v", list)
	}
	if _, err := s.RejectProposal(t.Context(), list.Items[0].ID, in.RejectRequest{}); err == nil {
		t.Fatal("отказ без причины должен отклоняться")
	}
	if _, err := s.RejectProposal(t.Context(), list.Items[0].ID, in.RejectRequest{Reason: "too_broad", Comment: "кис — ещё и KSOS"}); err != nil {
		t.Fatal(err)
	}
	// Второй запуск: отказ «слишком широко» не закрывает кластер, но ту же правку код отклоняет памятью.
	model.turns = [][]llm.ToolCall{
		{tc("create_sandbox_patch", patch)},
		{tc("close_cluster", map[string]any{"diagnosis": "ALIAS_GAP", "reason": "единственная правка отклонена человеком"})},
	}
	r2, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, s, r2.RunID)
	if run.ProposalsCreated != 0 || evals != 1 {
		t.Fatalf("отклонённое не должно вернуться: %+v, прогонов %d", run, evals)
	}
	found := false
	for _, st := range ms.steps {
		if st.RunID.String() == r2.RunID && st.Kind == domain.StepCheck && st.RejectedReason != nil &&
			strings.Contains(*st.RejectedReason, "человек уже отклонил") {
			found = true
		}
	}
	if !found {
		t.Fatal("нет шага с отказом по памяти")
	}
	// Отказ по другой причине закрывает кластер целиком: третий запуск его не берёт.
	ms.decisions[0].Reason = domain.RejectWrongProduct
	model.turns = nil
	r3, _ := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	run = waitRun(t, s, r3.RunID)
	if run.ClustersTotal != 0 {
		t.Fatalf("кластер с отказом «неверный товар» не должен браться: %+v", run)
	}
}

// Синоним между производителями отклоняется кодом до прогона.
func TestAgentCrossVendorRejected(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	s.agent.evalHook = func(context.Context, *failedCluster, RulePatch) (*patchEval, error) {
		t.Fatal("прогон не должен вызываться")
		return nil, nil
	}
	model.turns = [][]llm.ToolCall{
		{tc("create_sandbox_patch", map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "acronis — это МойОфис",
			"add": []map[string]any{{"alias": "кис", "canonical": "мойофис", "target": "family", "target_id": "myoffice-standard", "term_vendor": "Acronis"}}})},
		{tc("create_sandbox_patch", map[string]any{"diagnosis": "ALIAS_GAP", "hypothesis": "kaspersky = adobe",
			"add": []map[string]any{{"alias": "кис", "canonical": "photoshop", "target": "family", "target_id": "adobe-photoshop", "term_vendor": "Kaspersky"}}})},
		{tc("close_cluster", map[string]any{"diagnosis": "UNKNOWN", "reason": "нет правки"})},
	}
	r, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	waitRun(t, s, r.RunID)
	n := 0
	for _, st := range ms.steps {
		if st.Kind == domain.StepCheck && st.RejectedReason != nil && strings.Contains(*st.RejectedReason, "разными производителями") {
			n++
		}
	}
	if n != 2 {
		t.Fatalf("ожидалось 2 отказа cross-vendor, есть %d", n)
	}
}

// Гонка версий: правка поверх устаревшей версии — 409, правила не меняются.
func TestRulesVersionRace(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	mk := func(alias string) string {
		p := &domain.Proposal{Status: domain.ProposalPending, Action: domain.ActionRulesPatch, ClusterLabel: alias,
			Patch: toJSON(RulePatch{Add: []PatchAlias{{Kind: "alias", Alias: alias, Canonical: "kaspersky", Target: "vendor", TargetID: "kaspersky"}}})}
		_ = ms.CreateProposal(t.Context(), p)
		return p.ID.String()
	}
	a, b := mk("кис"), mk("ксос")
	// Писатель прочитал версию 1, другой успел создать версию 2.
	stale := &repositories.RulesChange{Base: 1, Version: &domain.ConfigVersion{Version: 2}}
	if _, err := s.ApproveProposal(t.Context(), a, in.ApproveRequest{}); err != nil {
		t.Fatal(err)
	}
	if err := ms.ApplyRulesChange(t.Context(), stale); !errors.Is(err, domain.ErrRulesVersionConflict) {
		t.Fatalf("устаревшая база: %v", err)
	}
	if !isConflict(rulesError(domain.ErrRulesVersionConflict)) {
		t.Fatal("конфликт версии — 409")
	}
	// Следующее принятие строится поверх действующей версии: v3 содержит оба алиаса.
	res, err := s.ApproveProposal(t.Context(), b, in.ApproveRequest{})
	if err != nil || *res.ConfigVersion != 3 {
		t.Fatalf("второе принятие: %+v %v", res, err)
	}
	active, _ := s.activeRules(t.Context())
	if active.find("кис") == nil || active.find("ксос") == nil {
		t.Fatal("версия 3 должна содержать обе правки")
	}
	// Откат к 1 — новая версия 4, принятые предложения — «откачено» (в памяти — через ApplyRulesChange базы).
	rb, err := s.RollbackConfig(t.Context(), in.RollbackRequest{ToVersion: 1})
	if err != nil || rb.Version != 4 {
		t.Fatalf("откат: %+v %v", rb, err)
	}
	active, _ = s.activeRules(t.Context())
	if active.find("кис") != nil || active.find("винда") == nil {
		t.Fatal("после отката к 1 — исходные правила")
	}
	cfgs, _ := s.Configs(t.Context())
	if cfgs.Active != 4 || len(cfgs.Items) != 4 {
		t.Fatalf("версии: %+v", cfgs)
	}
}

func TestAdmit(t *testing.T) {
	if ev := evalResult(0, 1, 0); !ev.Passed {
		t.Fatalf("улучшение без регрессии должно проходить: %v", ev.Reasons)
	}
	if ev := evalResult(1, 1, 0); ev.Passed || !strings.Contains(ev.Reasons[0], "не улучшился") {
		t.Fatalf("без улучшения: %v", ev.Reasons)
	}
	if ev := evalResult(0, 1, 3); ev.Passed || !strings.Contains(strings.Join(ev.Reasons, ";"), "слишком широко: 3") {
		t.Fatalf("регрессия: %v", ev.Reasons)
	}
}

func TestRulePatchDiff(t *testing.T) {
	base := &ruleSet{Version: 1, Aliases: []ruleAlias{{Alias: "винда", Canonical: "windows", Target: "term"}, {Alias: "ворд", Canonical: "word", Target: "term"}}}
	p := RulePatch{Add: []PatchAlias{{Alias: "Кис", Canonical: "kaspersky internet security", Target: "family", TargetID: "kis"}},
		Remove: []PatchAlias{{Alias: "ворд"}}}
	next := base.applyPatch(p)
	if next.find("кис") == nil || next.find("ворд") != nil || next.find("винда") == nil {
		t.Fatalf("правка: %+v", next.Aliases)
	}
	d := rulesDiff(base, next)
	if len(d.Add) != 1 || len(d.Remove) != 1 || d.Remove[0].Alias != "ворд" {
		t.Fatalf("разница: %+v", d)
	}
	if k := patchKeys(p); len(k) != 2 || k[1] != "кис→family:kis" {
		t.Fatalf("ключи: %v", k)
	}
}

func TestTextToolCalls(t *testing.T) {
	got := textToolCalls("Посмотрю трассу.\n```\n{\"name\":\"get_search_trace\",\"parameters\":{\"query\":\"кис\"}}\n```")
	if len(got) != 1 || got[0].Function.Name != "get_search_trace" || !strings.Contains(got[0].Function.Arguments, "кис") {
		t.Fatalf("вызов текстом: %+v", got)
	}
	if len(textToolCalls(`{"name":"rm -rf","parameters":{}}`)) != 0 {
		t.Fatal("неизвестный инструмент не вызывается")
	}
}

func TestComputeQueryStatsAndClusters(t *testing.T) {
	ss, bs := failedLog()
	st := computeQueryStats(ss, bs)
	q := st.byText["кис"]
	if q == nil || q.Freq != 3 || q.Empty != 3 || q.Failures != 3 || len(q.ReformulatedTo) != 1 || q.ReformulatedTo[0].Clicked != 3 {
		t.Fatalf("исходы: %+v", q)
	}
	if ok := st.byText["kaspersky internet security"]; ok.Failures != 0 || ok.Clicks != 3 {
		t.Fatalf("успешный: %+v", ok)
	}
	f := st.failed()
	cl := clusterFailed(append(f, clusterQuery{Text: "кис 3 пк", Freq: 2, Failures: 2}), nil, nil, nil)
	if len(f) != 1 || len(cl) != 2 {
		t.Fatalf("без словаря кластеры по словам не склеиваются: %+v", cl)
	}
}

func TestInferredTextCalls(t *testing.T) {
	got := textToolCalls("Проверим поиск. ```json {\"query\":\"кис\"} ``` и каталог ```json {\"expand\":true,\"query\":\"кис\"}```")
	if len(got) != 2 || got[0].Function.Name != "get_search_trace" || got[1].Function.Name != "search_catalog" {
		t.Fatalf("вызовы по аргументам: %+v", got)
	}
}
