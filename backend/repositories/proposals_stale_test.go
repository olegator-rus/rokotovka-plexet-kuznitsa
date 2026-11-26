package repositories

import (
	"testing"

	"kuznica/api/domain"

	"github.com/google/uuid"
)

// Ревью-2 N2 и n1 на живой базе: ждущие предложения запусков до разреза → stale (pre_holdout), принятые и чужие
// запуски не трогаются; перепроверка пишет только в ждущее предложение.
func TestStaleProposalsOfRunsAndCheck(t *testing.T) {
	r := testSchemaRepos(t)
	ctx := t.Context()
	runOld, runNew := uuid.New(), uuid.New()
	mk := func(run uuid.UUID, status string) *domain.Proposal {
		p := &domain.Proposal{RunID: run, Status: status, ClusterLabel: "x"}
		if err := r.CreateProposal(ctx, p); err != nil {
			t.Fatal(err)
		}
		return p
	}
	pending, accepted, other := mk(runOld, domain.ProposalPending), mk(runOld, domain.ProposalAccepted), mk(runNew, domain.ProposalPending)
	ids, err := r.StaleProposalsOfRuns(ctx, []string{runOld.String()[:8]}, domain.ProposalStalePreHoldout, "до разреза")
	if err != nil || len(ids) != 1 || ids[0] != pending.ID.String() {
		t.Fatalf("stale: %v %v", ids, err)
	}
	for p, want := range map[*domain.Proposal]string{pending: domain.ProposalStale, accepted: domain.ProposalAccepted,
		other: domain.ProposalPending} {
		got, _ := r.GetProposal(ctx, p.ID)
		if got.Status != want {
			t.Fatalf("%s: %s, ждали %s", p.ID, got.Status, want)
		}
		if want == domain.ProposalStale && (got.DecidedReason != domain.ProposalStalePreHoldout || got.DecidedComment != "до разреза" || got.DecidedAt == nil) {
			t.Fatalf("причина: %+v", got)
		}
	}
	// Перепроверка: в stale не пишется, в ждущее — только evidence.
	ok, err := r.UpdateProposalCheck(ctx, pending.ID, domain.ProposalPending, ProposalCheck{Evidence: domain.JSONB(`{"recheck":1}`)})
	if err != nil || ok {
		t.Fatalf("stale не должно обновляться: %v %v", ok, err)
	}
	ok, err = r.UpdateProposalCheck(ctx, other.ID, domain.ProposalPending, ProposalCheck{Evidence: domain.JSONB(`{"recheck":1}`)})
	got, _ := r.GetProposal(ctx, other.ID)
	if err != nil || !ok || got.Status != domain.ProposalPending || string(got.Evidence) != `{"recheck": 1}` {
		t.Fatalf("ждущее: %v %v %s", ok, err, got.Evidence)
	}
}
