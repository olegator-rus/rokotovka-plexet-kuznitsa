package services

import (
	"testing"

	"kuznica/api/domain"
)

// Матрица прав: руководитель и служебный доступ — всё; роль — только своё; гость и покупатель — ничего.
func TestCanDecide(t *testing.T) {
	owners := []string{RoleEcom, RoleProduct, RoleContent}
	for _, actor := range []string{RoleGuest, RoleBuyer, RoleEcom, RoleProduct, RoleContent, RoleHead, RoleSystem, ""} {
		for _, owner := range owners {
			want := actor == RoleHead || actor == RoleSystem || actor == owner
			if got := CanDecide(actor, owner); got != want {
				t.Errorf("%q решает за %q: %v, ждали %v", actor, owner, got, want)
			}
		}
	}
	for owner, want := range map[string]string{RoleEcom: "Это решение принимает менеджер e-com",
		RoleProduct: "Это решение принимает продукт-менеджер", RoleContent: "Это решение принимает контент-менеджер",
		"": "Это решение принимает руководитель e-com"} {
		if got := DenyMessage(owner); got != want {
			t.Errorf("%q: %q", owner, got)
		}
	}
	if IsStaffRole(RoleBuyer) || IsStaffRole(RoleGuest) || !IsStaffRole(RoleContent) || !IsStaffRole(RoleSystem) {
		t.Error("IsStaffRole")
	}
}

// Роль предложения читается из очереди; нет предложения — 404, кривой id — 422.
func TestProposalRole(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	p := &domain.Proposal{Role: RoleProduct, Action: domain.ActionRelationPatch, Status: domain.ProposalPending}
	if err := ms.CreateProposal(t.Context(), p); err != nil {
		t.Fatal(err)
	}
	if role, err := s.ProposalRole(t.Context(), p.ID.String()); err != nil || role != RoleProduct {
		t.Fatalf("%q %v", role, err)
	}
	if _, err := s.ProposalRole(t.Context(), "not-a-uuid"); err == nil {
		t.Error("кривой id")
	}
	if _, err := s.ProposalRole(t.Context(), "01a21227-db17-77c8-a4fb-dbae24688465"); err == nil {
		t.Error("нет предложения")
	}
}
