package services

import (
	"encoding/json"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

func companionsFixture(t *testing.T) (*commerceCatalog, *searchLexicon) {
	t.Helper()
	snap := p2Snapshot(t)
	return newCommerceCatalog(snap), newSearchLexicon(snap)
}

// F16: «windows server 2025 standard на 50 пользователей» → CAL 2025 × 50 с основанием; CAL 2022 по сломанной
// связи не показан (правило вендора: версия CAL ≥ версии сервера), RDS CAL — правило есть, связи нет → в трассе.
func TestCompanionsWindowsServerCAL(t *testing.T) {
	c, lx := companionsFixture(t)
	anchor := &out.CompanionAnchor{ProductID: "ws2025-std-16core", FamilyID: "windows-server-2025", Confidence: "high"}
	r := c.companionsFor(lx, anchor, companionQuery{quantity: 50, metric: "user"})
	if len(r.required) != 1 {
		t.Fatalf("required: %+v", r.required)
	}
	req := r.required[0]
	if req.ProductID != "ws2025-cal-user" || req.SuggestedQuantity == nil || *req.SuggestedQuantity != 50 ||
		req.Class != CompanionRequired || req.Label != labelRequired || req.Source != domain.SourceVendorDocs ||
		req.SourceURL == "" || req.Reason == "" || req.Compatibility != domain.CompatVerified {
		t.Fatalf("CAL 2025: %+v", req)
	}
	reasons := map[string]string{}
	for _, x := range r.step.Rejected {
		reasons[x.ProductID] = x.Reason
	}
	if reasons["wscal-u1"] != rejectVersion {
		t.Fatalf("CAL 2022 должен быть отклонён как несовместимый: %+v", r.step.Rejected)
	}
	if reasons["rdscal25-u1"] != rejectMissingRel {
		t.Fatalf("RDS CAL без связи — missing_relation: %+v", r.step.Rejected)
	}
	for _, x := range r.step.Considered {
		if x.ProductID == "wscal-u1" && x.Shown {
			t.Fatal("CAL 2022 показан")
		}
	}
	// Без числа в запросе — количество пустое.
	r = c.companionsFor(lx, anchor, companionQuery{})
	if len(r.required) != 1 || r.required[0].SuggestedQuantity != nil {
		t.Fatalf("без количества: %+v", r.required)
	}
}

// После исправления (связь на RDS добавлена агентом) RDS CAL 2025 приходит строкой условно обязательного с
// условием remote_desktop_used.
func TestCompanionsRelationAddedByAgent(t *testing.T) {
	snap := p2Snapshot(t)
	snap.Relations = append(snap.Relations, domain.Relation{RelID: "rel-rds", FromFamily: "windows-server-2025",
		ToFamily: "rds-cal-2025", Type: domain.RelationComplement, Requirement: domain.RequirementConditional,
		Compatibility: domain.CompatVerified, QuantityStrategy: "per_user", Source: domain.SourceAgentProposal,
		ReviewStatus: domain.ReviewApproved})
	c, lx := newCommerceCatalog(snap), newSearchLexicon(snap)
	r := c.companionsFor(lx, &out.CompanionAnchor{ProductID: "ws2025-std-16core", FamilyID: "windows-server-2025"},
		companionQuery{quantity: 10, metric: "user"})
	var rds *out.Companion
	for i := range r.required {
		if r.required[i].ProductID == "rdscal25-u1" {
			rds = &r.required[i]
		}
	}
	if rds == nil || !contains(rds.Conditions, "remote_desktop_used") || *rds.SuggestedQuantity != 10 {
		t.Fatalf("RDS: %+v", r.required)
	}
}

// Сервер → память: совместимое по правилу модели, подпись «Совместимо с …»; несовместимое не показывается.
func TestCompanionsServerMemory(t *testing.T) {
	snap := p2Snapshot(t)
	bad := snap.Products[0]
	for _, p := range snap.Products {
		if p.ID == "ram-32g-rdimm" {
			bad = p
		}
	}
	bad.ID = "ram-64g-lrdimm"
	snap.Products = append(snap.Products, bad)
	snap.CompatibilityRules = append(snap.CompatibilityRules, domain.CompatibilityRule{ID: "cr-bad",
		ProductID: "ram-64g-lrdimm", TargetModel: "dell-poweredge-r740", State: domain.CompatIncompatible, Condition: "LRDIMM"})
	snap.Relations = append(snap.Relations, domain.Relation{RelID: "rel-fam", FromFamily: "dell-poweredge-r740",
		ToFamily: "dell-ram-ddr4-rdimm", Type: domain.RelationComplement, Requirement: domain.RequirementRecommended,
		Compatibility: domain.CompatVerified, Conditions: []string{condCheckCompatRule}})
	c, lx := newCommerceCatalog(snap), newSearchLexicon(snap)
	r := c.companionsFor(lx, &out.CompanionAnchor{ProductID: "dell-r740-base", FamilyID: "dell-poweredge-r740"}, companionQuery{})
	if r.block == nil || len(r.block.Items) != 1 {
		t.Fatalf("блок: %+v", r.block)
	}
	it := r.block.Items[0]
	if it.ProductID != "ram-32g-rdimm" || it.Class != CompanionCompatible || it.Compatibility != domain.CompatVerified ||
		it.Label == "" || it.Label == labelComplement {
		t.Fatalf("память: %+v", it)
	}
	if len(r.required) != 0 {
		t.Fatalf("required у сервера: %+v", r.required)
	}
	for _, x := range r.step.Considered {
		if x.ProductID == "ram-64g-lrdimm" && x.Shown {
			t.Fatal("несовместимый модуль показан")
		}
	}
}

// Якорь: категорийный запрос, аналоги, неоднозначность — без блока; связанное семейство рядом — не мешает.
func TestAnchorOf(t *testing.T) {
	c, lx := companionsFixture(t)
	fh := func(id, pid, group string, tier int) *familyHit {
		return &familyHit{id: id, group: group, tier: tier, head: indexDoc{ProductID: pid}}
	}
	ws := fh("windows-server-2025", "ws2025-std-16core", domain.GroupExact, 2)
	cal := fh("windows-server-cal-2025", "ws2025-cal-user", domain.GroupExact, 2)
	office := fh("microsoft-office-2021", "office-2021-hb", domain.GroupExact, 2)
	if a, _, _ := anchorOf(c, lx, []string{"microsoft", "windows", "server", "2025"}, []*familyHit{ws, cal}, domain.RouteNormalized, false, false, false); a == nil || a.Confidence != "high" {
		t.Fatalf("сервер + свой CAL: %+v", a)
	}
	// CAL выше сервера (совпало количество) — якорь всё равно сервер, позиция 1.
	if a, idx, _ := anchorOf(c, lx, []string{"microsoft", "windows", "server", "2025"}, []*familyHit{cal, ws}, domain.RouteNormalized, false, false, false); a == nil ||
		a.FamilyID != "windows-server-2025" || idx != 1 {
		t.Fatalf("CAL первым: %+v %d", a, idx)
	}
	if a, _, _ := anchorOf(c, lx, []string{"microsoft", "windows", "server", "2025"}, []*familyHit{ws, office}, domain.RouteNormalized, false, false, false); a != nil {
		t.Fatalf("два несвязанных названных товара: %+v", a)
	}
	if a, _, _ := anchorOf(c, lx, []string{"microsoft", "windows", "server", "2025"}, []*familyHit{fh("x", "kis-2dev-1y", domain.GroupMatch, 0)}, domain.RouteSemantic, false, false, false); a != nil {
		t.Fatal("категорийный запрос дал якорь")
	}
	if a, _, _ := anchorOf(c, lx, []string{"microsoft", "windows", "server", "2025"}, []*familyHit{ws}, domain.RouteNormalized, true, false, false); a != nil {
		t.Fatal("аналог дал якорь")
	}
	if a, _, _ := anchorOf(c, lx, []string{"microsoft", "windows", "server", "2025"}, []*familyHit{fh("windows-server-2025", "ws2025-std-16core", domain.GroupExact, 0)}, domain.RouteNormalized,
		false, false, false); a == nil || a.Confidence != "medium" {
		t.Fatalf("вендор назван, семейство одно: %+v", a)
	}
}

func TestScoreCompanions(t *testing.T) {
	pairs := []domain.CompanionPair{
		{AnchorProduct: "a1", CandidateProduct: "c1", Class: "required"},
		{AnchorProduct: "a1", CandidateProduct: "bad", Class: "incompatible"},
		{AnchorProduct: "a2", CandidateProduct: "c2", Class: "required"},
		{AnchorProduct: "a3", CandidateProduct: "c3", Class: "complement"},
		{AnchorProduct: "a4", CandidateProduct: "x", Class: "irrelevant"},
	}
	sc := ScoreCompanions(pairs, map[string][]string{"a1": {"c1", "bad"}, "a3": {"c3", "unlabeled"}}, nil)
	if sc.Anchors != 4 || sc.Covered != 2 || *sc.Coverage != 2.0/3 || sc.CompatViolations != 1 || sc.LabeledShown != 3 ||
		sc.ShownPairs != 4 || sc.RequiredAnchors != 2 || sc.RequiredHit != 1 || *sc.RequiredRecall != 0.5 || len(sc.Violations) != 1 {
		t.Fatalf("%+v", sc)
	}
	if empty := ScoreCompanions(nil, nil, nil); empty.Coverage != nil || empty.RequiredRecall != nil || empty.ViolationRate != nil {
		t.Fatalf("пустая разметка: %+v", empty)
	}
}

// Флаг допродажи выключен — выдача та же (позиции, порядок, скоры), блока и строк обязательного нет; включён —
// меняются только companions и required.
func TestCompanionsFlagOffSameItems(t *testing.T) {
	on := importedWith(t, Features{Companions: true})
	off := importedWith(t, Features{})
	blocks := 0
	for _, q := range []string{"касперский", "фотошоп", "office 2021", "антивирус на 5 устройств", "dell r740",
		"windows server 2025 на 50 пользователей", "notion"} {
		a, err := on.Search(t.Context(), "r", in.SearchRequest{Query: q, Config: "B3"})
		if err != nil {
			t.Fatal(err)
		}
		b, err := off.Search(t.Context(), "r", in.SearchRequest{Query: q, Config: "B3"})
		if err != nil {
			t.Fatal(err)
		}
		for _, st := range b.Trace.Steps {
			if _, ok := st.(CompanionsStep); ok {
				t.Fatalf("%q: шаг companions при выключенном флаге", q)
			}
		}
		if b.Companions != nil {
			t.Fatalf("%q: блок при выключенном флаге", q)
		}
		if a.Companions != nil {
			blocks++
		}
		a.Trace, b.Trace, a.Companions = nil, nil, nil
		for i := range a.Items {
			a.Items[i].Required = []out.Companion{}
		}
		ja, _ := json.Marshal(a)
		jb, _ := json.Marshal(b)
		if string(ja) != string(jb) {
			t.Fatalf("%q: выдача зависит от флага допродажи\non:  %s\noff: %s", q, ja, jb)
		}
	}
	if blocks == 0 {
		t.Fatal("ни одного блока при включённом флаге — тест ничего не проверяет")
	}
}
