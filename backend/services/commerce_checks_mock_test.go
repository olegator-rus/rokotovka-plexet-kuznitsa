package services

import (
	"os"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
)

// Заложенное в data/mock/PLANTED.md (контур коммерции) находится детерминированной проверкой справочника: P2-01 —
// rel-0065 (WS 2025 → CAL 2022) против правила вендора, P2-02 — правило RDS CAL 2025 без связи, P2-05 —
// rel-0092 на снятую позицию. Нет mock-данных контура коммерции — пропуск.
func TestScanRelationsFindsPlanted(t *testing.T) {
	dir := "../../data/mock"
	if _, err := os.Stat(dir + "/license_rules.json"); err != nil {
		t.Skip("нет mock-данных контура коммерции")
	}
	snap, err := catalogsource.MockJSON{Dir: dir}.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	c := newCommerceCatalog(snap)
	scan := scanRelations(c, docsOfRelations(snap.Relations))
	got := map[string]relationFinding{}
	for _, f := range scan.Findings {
		got[f.Key] = f
		t.Logf("%s %s %s fix=%+v", f.Key, f.Reason, f.Text, f.Fix)
	}
	b := got["broken:rel-0065"]
	if b.Diagnosis != domain.DiagBrokenRelation || b.Reason != brokenLicenseRule || b.Fix == nil || len(b.Fix.Remove) != 1 {
		t.Fatalf("P2-01: %+v", b)
	}
	m := got["missing:lr-ws25-rds"]
	if m.Diagnosis != domain.DiagMissingRequired || m.Fix == nil || len(m.Fix.Add) != 1 || m.Fix.Add[0].ToFamily != "rds-cal-2025" {
		t.Fatalf("P2-02: %+v", m)
	}
	if u := got["broken:rel-0092"]; u.Reason != brokenProductUnavailable {
		t.Fatalf("P2-05: %+v", u)
	}
	// Правка-подсказка снимает свою находку и не создаёт новых.
	for _, k := range []string{"broken:rel-0065", "missing:lr-ws25-rds", "broken:rel-0092"} {
		after, err := applyRelationPatch(docsOfRelations(snap.Relations), *got[k].Fix, "")
		if err != nil {
			t.Fatal(k, err)
		}
		next := scanRelations(c, after)
		if next.keys()[k] || len(next.Findings) != len(scan.Findings)-1 {
			t.Fatalf("%s: после правки находок %d (было %d)", k, len(next.Findings), len(scan.Findings))
		}
	}
}
