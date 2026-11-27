package catalogsource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kuznica/api/domain"
)

func TestLoadFixture(t *testing.T) {
	snap, err := MockJSON{Dir: "../../testdata/catalog"}.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != "fixture-1" || len(snap.Products) != 12 || len(snap.Vendors) != 7 || len(snap.Categories) != 14 ||
		len(snap.Relations) != 4 || len(snap.Aliases) != 6 || len(snap.Clients) != 3 || len(snap.Warnings) != 0 {
		t.Fatalf("снимок: %+v", snap)
	}
	p := snap.Products[0]
	if p.License.Quantity == nil || *p.License.Quantity != 2 || p.License.Metric == nil || *p.License.Metric != "device" ||
		p.PriceRub == nil || *p.PriceRub != 2990 || string(p.Attributes) == "" {
		t.Fatalf("позиция: %+v", p)
	}
	if snap.Products[3].PriceRub != nil || !snap.Products[3].PriceOnRequest {
		t.Fatalf("цена по запросу: %+v", snap.Products[3])
	}
	if len(snap.Clients[1].Purchases) != 1 || snap.Clients[1].Purchases[0].ProductID != "ksos-10dev-1y" {
		t.Fatalf("покупки: %+v", snap.Clients[1])
	}
}

func write(t *testing.T, dir, name, body string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, name), []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
}

func TestLoadProblems(t *testing.T) {
	dir := t.TempDir()
	if _, err := (MockJSON{Dir: dir}).Load(t.Context()); err == nil {
		t.Fatal("без catalog.json — ошибка")
	}
	write(t, dir, "catalog.json", `{"version":"v","products":[{"id":"a","title":"A"},{"id":"a","title":"B"}]}`)
	if _, err := (MockJSON{Dir: dir}).Load(t.Context()); err == nil || !strings.Contains(err.Error(), "повторяется") {
		t.Fatalf("повтор id: %v", err)
	}
	write(t, dir, "catalog.json", `{"version":"v","products":[{"id":"a","title":""}]}`)
	if _, err := (MockJSON{Dir: dir}).Load(t.Context()); err == nil || !strings.Contains(err.Error(), "без title") {
		t.Fatalf("без названия: %v", err)
	}
	write(t, dir, "catalog.json", `{"products":[{"id":"a","title":"A","vendor_id":"x"}]}`)
	write(t, dir, "vendors.json", `[{"id":"y","name":"Y"}]`)
	snap, err := MockJSON{Dir: dir}.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	w := strings.Join(snap.Warnings, "\n")
	if snap.Products[0].FamilyID != "a" || !strings.Contains(w, "нет version") || !strings.Contains(w, "aliases.json: файла нет") ||
		!strings.Contains(w, `a → vendor "x"`) {
		t.Fatalf("предупреждения: %s / %+v", w, snap.Products[0])
	}
	write(t, dir, "aliases.json", `{не json`)
	if _, err := (MockJSON{Dir: dir}).Load(t.Context()); err == nil || !strings.Contains(err.Error(), "aliases.json") {
		t.Fatalf("битый справочник: %v", err)
	}
}

// TestLoadFixtureP2 — поля и справочники контура коммерции и значения по умолчанию.
func TestLoadFixtureP2(t *testing.T) {
	snap, err := MockJSON{Dir: "../../testdata/catalog"}.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	byID := map[string]domain.Product{}
	for _, p := range snap.Products {
		byID[p.ID] = p
	}
	if p := byID["kis-2dev-1y"]; p.Kind != domain.KindSoftware || p.MPN != "" || p.PriceMode != domain.PriceFixed {
		t.Fatalf("ПО без полей контура коммерции: kind=%q mpn=%q price_mode=%q", p.Kind, p.MPN, p.PriceMode)
	}
	if p := byID["photoshop-1u-1y"]; p.PriceMode != domain.PriceQuoteRequired {
		t.Fatalf("price_on_request → quote_required: %q", p.PriceMode)
	}
	srv := byID["dell-r740-base"]
	if srv.Kind != domain.KindHardware || srv.MPN != "PER740-BASE-01" || srv.ModelID != "dell-poweredge-r740" ||
		srv.PriceMode != domain.PriceFrom || srv.Specs["ram_gb"] != 32 || srv.Specs["form_factor_u"] != 2 {
		t.Fatalf("сервер: %+v", srv)
	}
	ram := byID["ram-32g-rdimm"]
	if ram.PriceMode != domain.PriceQuoteRequired || len(ram.CompatibleWith) != 1 || ram.CompatibleWith[0] != "dell-poweredge-r740" {
		t.Fatalf("память: %+v", ram)
	}
	rels := map[string]domain.Relation{}
	for _, r := range snap.Relations {
		rels[r.RelID] = r
	}
	if r := rels[""]; r.Requirement != domain.RequirementOptional || r.Compatibility != domain.CompatUnknown {
		t.Fatalf("связь контура поиска — значения по умолчанию: %+v", r)
	}
	cal := rels["rel-p2-0001"]
	if cal.Requirement != domain.RequirementConditional || cal.Compatibility != domain.CompatConditional ||
		len(cal.Conditions) != 2 || cal.QuantityStrategy != "per_user" || cal.EvidenceLevel != "E2" || cal.FromProduct != nil {
		t.Fatalf("связь контура коммерции: %+v", cal)
	}
	if r := rels["rel-p2-0002"]; r.ToProduct == nil || *r.ToProduct != "ram-32g-rdimm" {
		t.Fatalf("уточнение до позиции: %+v", r)
	}
	if len(snap.LicenseRules) != 1 || snap.LicenseRules[0].AnchorAttrs["edition"] != "Standard" ||
		len(snap.CompatibilityRules) != 1 || snap.CompatibilityRules[0].State != domain.CompatVerified ||
		len(snap.AttributeDefinitions) != 2 ||
		len(snap.Synthetic) != 1 || snap.Synthetic[0] != "margin" {
		t.Fatalf("справочники контура коммерции: %+v %+v %+v %v", snap.LicenseRules, snap.CompatibilityRules,
			snap.AttributeDefinitions, snap.Synthetic)
	}
}

// TestLoadP2Optional — каталог контура поиска без файлов контура коммерции читается: справочники пусты, без предупреждений о них.
func TestLoadP2Optional(t *testing.T) {
	dir := t.TempDir()
	write(t, dir, "catalog.json", `{"version":"v","products":[{"id":"a","title":"A"}]}`)
	snap, err := MockJSON{Dir: dir}.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snap.LicenseRules != nil || snap.CompatibilityRules != nil || snap.AttributeDefinitions != nil ||
		snap.Synthetic != nil {
		t.Fatalf("справочники контура коммерции не пусты: %+v", snap)
	}
	for _, w := range snap.Warnings {
		if strings.Contains(w, "rules") || strings.Contains(w, "attribute") {
			t.Fatalf("предупреждение о необязательном файле: %q", w)
		}
	}
}

// TestLoadP2Invalid — неизвестные значения перечислений контура коммерции — ошибка загрузки.
func TestLoadP2Invalid(t *testing.T) {
	for name, files := range map[string][2]string{
		"kind":       {`{"version":"v","products":[{"id":"a","title":"A","kind":"robot"}]}`, `[]`},
		"price_mode": {`{"version":"v","products":[{"id":"a","title":"A","price_mode":"free"}]}`, `[]`},
		"type":       {`{"version":"v","products":[{"id":"a","title":"A"}]}`, `[{"from_family":"a","to_family":"a","type":"bundle"}]`},
		"requirement": {`{"version":"v","products":[{"id":"a","title":"A"}]}`,
			`[{"from_family":"a","to_family":"a","type":"upgrade","requirement":"must"}]`},
	} {
		dir := t.TempDir()
		write(t, dir, "catalog.json", files[0])
		write(t, dir, "relations.json", files[1])
		if _, err := (MockJSON{Dir: dir}).Load(t.Context()); err == nil || !strings.Contains(err.Error(), name) {
			t.Errorf("%s: ожидалась ошибка, получено %v", name, err)
		}
	}
}
