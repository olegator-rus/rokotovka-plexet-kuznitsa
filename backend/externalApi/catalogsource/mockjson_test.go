package catalogsource

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestLoadFixture(t *testing.T) {
	snap, err := MockJSON{Dir: "../../testdata/catalog"}.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if snap.Version != "fixture-1" || len(snap.Products) != 8 || len(snap.Vendors) != 6 || len(snap.Categories) != 8 ||
		len(snap.Relations) != 2 || len(snap.Aliases) != 6 || len(snap.Clients) != 3 || len(snap.Warnings) != 0 {
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
