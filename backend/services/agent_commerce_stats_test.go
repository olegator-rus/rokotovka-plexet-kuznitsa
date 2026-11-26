package services

import (
	"context"
	"encoding/json"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/externalApi/llm"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/opensearch/opensearchtest"
	"kuznica/api/externalApi/tei"
	"kuznica/api/externalApi/tei/teitest"
	"kuznica/api/in"
)

// judgeFake — судья: отвечает JSON, считает вызовы.
type judgeFake struct {
	mu    sync.Mutex
	calls int
	reply string
}

func (j *judgeFake) Model() string { return "qwen3.6-35b-a3b" }
func (j *judgeFake) Chat(_ context.Context, req llm.Request) (*llm.Response, error) {
	j.mu.Lock()
	defer j.mu.Unlock()
	j.calls++
	return &llm.Response{Message: llm.Message{Role: "assistant", Content: j.reply}, Usage: llm.Usage{PromptTokens: 500, CompletionTokens: 30}}, nil
}

// mockCommerceEnv — агент на mock-данных контура коммерции; находки справочника P2-01/02/04 уже ждут решения (как демо).
func mockCommerceEnv(t *testing.T) (*Services, *judgeFake) {
	t.Helper()
	dir := "../../data/mock"
	if _, err := os.Stat(dir + "/license_rules.json"); err != nil {
		t.Skip("нет mock-данных контура коммерции")
	}
	osrv, models := opensearchtest.New(), teitest.New()
	t.Cleanup(osrv.Close)
	t.Cleanup(models.Close)
	s := NewServices(nil, Options{Search: opensearch.New(osrv.URL, 0), Embedder: tei.New(models.URL, 0),
		Source: catalogsource.MockJSON{Dir: dir}, Features: Features{Hardware: true, Companions: true, MarginPolicy: true}})
	if _, err := s.ImportCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	rules, _ := json.Marshal(map[string]any{"aliases": []ruleAlias{}})
	ms := newMemStore(rules)
	s.agentStoreOverride = &memRelStore{memStore: ms}
	s.agent = newAgentService(s, &scriptModel{}, AgentOptions{})
	s.llm = newLLMGuard(llmModeOn, true, 0, time.Now, nil)
	j := &judgeFake{reply: `{"objection": false, "reason": "ИБП к серверу — обычное дополнение"}`}
	s.judge = j
	ch, err := s.relationChain(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	sb, err := s.newRelationSandbox(t.Context(), ch)
	if err != nil {
		t.Fatal(err)
	}
	for _, f := range sb.before.Findings {
		p := &domain.Proposal{ClusterKey: f.Key, Diagnosis: f.Diagnosis, Action: domain.ActionRelationPatch, Status: domain.ProposalPending}
		if err := ms.CreateProposal(t.Context(), p); err != nil {
			t.Fatal(err)
		}
	}
	return s, j
}

func TestCommerceCopurchaseRun(t *testing.T) {
	if testing.Short() {
		t.Skip("долгий: импорт mock и регрессия песочницы")
	}
	s, j := mockCommerceEnv(t)
	started, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{Mode: "commerce", MaxClusters: 5})
	if err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, s, started.RunID)
	detail, err := s.AgentRun(t.Context(), started.RunID)
	if err != nil {
		t.Fatal(err)
	}
	rejectedHyp := 0
	for _, st := range detail.Steps {
		t.Logf("%s %s ok=%v %s", st.Kind, strDeref(st.Tool), st.OK, st.Title)
		if st.Kind == domain.StepHypothesis && st.RejectedReason != nil {
			rejectedHyp++
		}
	}
	mine := 0
	all, _ := s.Proposals(t.Context(), domain.ProposalPending, "")
	for _, p := range all.Items {
		if p.Diagnosis == domain.DiagMissingRelation || p.Diagnosis == domain.DiagBundleOpportunity {
			mine++
		}
	}
	if run.Status != domain.RunDone || mine != 2 {
		t.Fatalf("запуск: %s %s, предложений %d: %s", run.Status, run.StopReason, run.ProposalsCreated, run.Summary)
	}
	if rejectedHyp == 0 {
		t.Fatal("ни одна гипотеза не отклонена на ступени")
	}
	list, _ := s.Proposals(t.Context(), domain.ProposalPending, "")
	var rel, bundle *string
	for i, p := range list.Items {
		switch p.Diagnosis {
		case domain.DiagMissingRelation:
			id := p.ID
			rel = &id
			if i > 1 || p.PriorityScore == nil || p.EvidenceLevel != "E4" || !strings.Contains(string(mustJSON(p.Patch)), "ippon-innova-rt-ii") {
				t.Fatalf("связь: позиция %d %+v", i, p)
			}
		case domain.DiagBundleOpportunity:
			id := p.ID
			bundle = &id
			if p.Action != domain.ActionBundle {
				t.Fatalf("комплект: %+v", p)
			}
		}
	}
	if rel == nil || bundle == nil {
		t.Fatalf("нужны MISSING_RELATION и BUNDLE_OPPORTUNITY: %+v", list.Items)
	}
	if j.calls < 2 {
		t.Fatalf("судья вызван %d раз", j.calls)
	}
	// Человек отклоняет комплект — повторный запуск его не предлагает, связь тоже (ждёт решения).
	if _, err := s.RejectProposal(t.Context(), *bundle, in.RejectRequest{Reason: "not_now"}); err != nil {
		t.Fatal(err)
	}
	started2, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{Mode: "commerce", MaxClusters: 5})
	if err != nil {
		t.Fatal(err)
	}
	run2 := waitRun(t, s, started2.RunID)
	again, _ := s.Proposals(t.Context(), "", "")
	n2 := 0
	for _, p := range again.Items {
		if p.RunID == started2.RunID && (p.Diagnosis == domain.DiagMissingRelation || p.Diagnosis == domain.DiagBundleOpportunity) {
			n2++
		}
	}
	if n2 != 0 {
		t.Fatalf("повторный запуск предложил снова: %s", run2.Summary)
	}
	// Принятие связи — версия справочника; следующий запуск видит пару связанной.
	res, err := s.ApproveProposal(t.Context(), *rel, in.ApproveRequest{})
	if err != nil || res.ConfigVersion == nil {
		t.Fatalf("принятие: %+v %v", res, err)
	}
}

func strDeref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

func mustJSON(v any) []byte {
	b, _ := json.Marshal(v)
	return b
}
