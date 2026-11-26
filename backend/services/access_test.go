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
	for owner, want := range map[string]string{RoleEcom: "Это решение принимает администратор поиска",
		RoleProduct: "Это решение принимает категорийный и лицензионный менеджер", RoleContent: "Это решение принимает команда каталога",
		"": "Это решение принимает руководитель e-com"} {
		if got := DenyMessage(owner); got != want {
			t.Errorf("%q: %q", owner, got)
		}
	}
	if IsStaffRole(RoleBuyer) || IsStaffRole(RoleGuest) || !IsStaffRole(RoleContent) || !IsStaffRole(RoleSystem) {
		t.Error("IsStaffRole")
	}
}

// Решает владелец причины предложения (контур доводки), а не записанная роль; нет предложения — 404, кривой id — 422.
func TestProposalRole(t *testing.T) {
	s, ms := agentEnv(t, &scriptModel{})
	p := &domain.Proposal{Role: RoleEcom, Diagnosis: domain.DiagBrokenRelation, Action: domain.ActionRelationPatch,
		Status: domain.ProposalPending}
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

// Девять причин (семь контура доводки, «только по запросу» контура фида, «нет факта в базе знаний» контура подбора): каждый диагноз — ровно в одной, у каждой владелец
// из ролей сотрудников; неизвестное — ручной разбор.
func TestCauses(t *testing.T) {
	if len(Causes) != 9 {
		t.Fatalf("причин %d, ждали 9", len(Causes))
	}
	seen := map[string]string{}
	for _, c := range Causes {
		if !IsStaffRole(c.Owner) || c.OwnerLabel == "" || c.Label == "" {
			t.Errorf("причина %+v", c)
		}
		for _, d := range c.Diagnoses {
			if prev, ok := seen[d]; ok {
				t.Errorf("%s в двух причинах: %s и %s", d, prev, c.Code)
			}
			seen[d] = c.Code
		}
	}
	for _, d := range domain.Diagnoses {
		if _, ok := seen[d]; !ok {
			t.Errorf("диагноз %s без причины", d)
		}
	}
	for d, want := range map[string]string{domain.DiagAliasGap: RoleEcom, domain.DiagRankingFailure: RoleEcom,
		domain.DiagContentGap: RoleContent, domain.DiagAssortmentGap: RoleProduct, domain.DiagMissingRequired: RoleProduct,
		domain.DiagNonProduct: RoleEcom, domain.DiagOnRequestOnly: RoleContent, "MISSING_RELATION": RoleHead, "": RoleHead} {
		if got := OwnerOf(d); got != want {
			t.Errorf("владелец %q: %q, ждали %q", d, got, want)
		}
	}
}
