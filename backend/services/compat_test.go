package services

import (
	"encoding/json"
	"testing"

	"kuznica/api/domain"
)

// p2Snapshot — фикстура testdata/catalog плюс то, что заложено в mock-2 для агента: CAL 2022 и сломанная связь
// Windows Server 2025 → CAL 2022 со статусом «verified», RDS CAL 2025 и правило RDS без связи.
func p2Snapshot(t *testing.T) *domain.CatalogSnapshot {
	t.Helper()
	snap, err := fixtureSource().Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	price := 3900.0
	cal22 := domain.Product{ID: "wscal-u1", FamilyID: "windows-server-cal", FamilyName: "Windows Server 2022 CAL",
		Title: "Microsoft Windows Server 2022 CAL, 1 пользователь", VendorID: "microsoft", CategoryID: "server-cal",
		Attributes: domain.JSONB(`{"version":"2022","edition":"User CAL"}`), PriceRub: &price,
		License: domain.License{Type: "perpetual", Metric: strPtr("user"), Quantity: intPtr(1)},
		Kind:    domain.KindSoftware, PriceMode: domain.PriceFixed}
	rds := cal22
	rds.ID, rds.FamilyID, rds.Title = "rdscal25-u1", "rds-cal-2025", "Microsoft RDS 2025 CAL, 1 пользователь"
	rds.Attributes = domain.JSONB(`{"version":"2025","edition":"RDS CAL"}`)
	snap.Products = append(snap.Products, cal22, rds)
	snap.Relations = append(snap.Relations, domain.Relation{RelID: "rel-broken", FromFamily: "windows-server-2025",
		ToFamily: "windows-server-cal", Type: domain.RelationComplement, Note: "Для подключения пользователей нужны CAL",
		Requirement: domain.RequirementConditional, Compatibility: domain.CompatVerified,
		Conditions: []string{"access_license_required"}, QuantityStrategy: "per_user", Source: domain.SourceCatalogRule,
		EvidenceLevel: "E1", ReviewStatus: domain.ReviewApproved})
	snap.LicenseRules = append(snap.LicenseRules, domain.LicenseRule{ID: "lr-ws-rds", VendorID: "microsoft",
		AnchorFamily: "windows-server-2025", RequiresCategory: "server-cal",
		RequiresAttrs: map[string]string{"edition": "RDS CAL", "version": ">=anchor.version"},
		Class:         domain.RequirementConditional, Conditions: []string{"remote_desktop_used"}, QuantityStrategy: "per_user",
		Reason: "RDS CAL той же версии, что сервер", Source: domain.SourceVendorDocs})
	return snap
}

func strPtr(s string) *string { return &s }
func intPtr(i int) *int       { return &i }

func TestCompatFourStates(t *testing.T) {
	c := newCommerceCatalog(p2Snapshot(t))
	cases := []struct {
		name      string
		got       CompatResult
		state     string
		basis     string
		condition bool
	}{
		{"модуль с правилом verified", c.ModelCompat("ram-32g-rdimm", "dell-poweredge-r740"), domain.CompatVerified, CompatBasisModelRule, false},
		{"без правила — unknown", c.ModelCompat("ram-32g-rdimm", "hpe-proliant-dl380-gen11"), domain.CompatUnknown, CompatBasisNone, false},
		{"сервер → память по модели якоря", c.PairCompat("dell-r740-base", "ram-32g-rdimm", nil), domain.CompatVerified, CompatBasisModelRule, false},
		{"CAL 2025 к серверу 2025", c.PairCompat("ws2025-std-16core", "ws2025-cal-user", nil), domain.CompatVerified, CompatBasisLicenseRule, false},
		{"CAL 2022 к серверу 2025: правило сильнее связи", c.PairCompat("ws2025-std-16core", "wscal-u1",
			c.relationBetween("ws2025-std-16core", "wscal-u1")), domain.CompatIncompatible, CompatBasisLicenseRule, true},
		{"связь без правила", c.PairCompat("photoshop-1u-1y", "fotomaster-pro-perp", &domain.Relation{Compatibility: domain.CompatConditional}),
			domain.CompatConditional, CompatBasisRelation, false},
		{"ничего не известно", c.PairCompat("kis-2dev-1y", "office-2021-hb", nil), domain.CompatUnknown, CompatBasisNone, false},
	}
	for _, tc := range cases {
		if tc.got.State != tc.state || tc.got.Basis != tc.basis || (tc.got.Condition != "") != tc.condition {
			t.Errorf("%s: %+v, ждали %s/%s", tc.name, tc.got, tc.state, tc.basis)
		}
	}
	if r := c.relationBetween("ws2025-std-16core", "wscal-u1"); r == nil || r.RelID != "rel-broken" {
		t.Fatalf("сломанная связь не найдена: %+v", r)
	}
	if c.PairCompat("ws2025-std-16core", "wscal-u1", nil).Allowed() {
		t.Fatal("incompatible не должен быть допустим")
	}
}

// Правило с anchor_attrs: Standard — да, другая редакция — нет; RDS: версия RDS CAL сравнивается с сервером.
func TestLicenseRulesForAnchorAttrs(t *testing.T) {
	snap := p2Snapshot(t)
	dc := snap.Products[0]
	for _, p := range snap.Products {
		if p.ID == "ws2025-std-16core" {
			dc = p
		}
	}
	dc.ID = "ws2025-dc-16core"
	raw, _ := json.Marshal(map[string]string{"edition": "Datacenter", "version": "2025"})
	dc.Attributes, dc.License.Edition = raw, "Datacenter"
	snap.Products = append(snap.Products, dc)
	c := newCommerceCatalog(snap)
	if n := len(c.licenseRulesFor("ws2025-std-16core")); n != 2 {
		t.Fatalf("Standard: правил %d, ждали 2", n)
	}
	if n := len(c.licenseRulesFor("ws2025-dc-16core")); n != 1 {
		t.Fatalf("Datacenter: правил %d, ждали 1 (только RDS без anchor_attrs)", n)
	}
	if r, _, ok := c.LicenseCompat("ws2025-std-16core", "rdscal25-u1"); !ok || r.State != domain.CompatVerified {
		t.Fatalf("RDS CAL 2025: %+v %v", r, ok)
	}
}

func TestCompareAttr(t *testing.T) {
	for _, c := range []struct {
		have, op, want string
		ok             bool
	}{{"2025", ">=", "2025", true}, {"2022", ">=", "2025", false}, {"2026", ">=", "2025", true}, {"Standard", "", "standard", true}} {
		if compareAttr(c.have, c.op, c.want) != c.ok {
			t.Errorf("%s %s %s", c.have, c.op, c.want)
		}
	}
}
