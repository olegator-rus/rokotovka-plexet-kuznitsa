package services

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/tei"
)

// названное семейство выше вендора, типизированные связи (renewal, compatibility).

func rankFor(t *testing.T, query string, rels map[string]float64) []*familyHit {
	t.Helper()
	lx, rules, _ := mockLexicon(t)
	nr := normalizeFull(query, lx, rules, false)
	a := extractAttrs(nr.tokens)
	e := findEntities(nr, a, lx, rules)
	rc := rankCtx{lx: lx, attrs: a, ent: e, purchased: map[string]bool{}, purchasedQty: map[string]int{},
		boughtVendors: map[string]bool{}, productRel: map[string]float64{}, refCats: map[string]bool{}}
	var rejected []domain.RejectedHit
	var fams []*familyHit
	for fid, rel := range rels {
		fh := rc.buildFamily(fid, rel, &rejected)
		if fh == nil {
			t.Fatalf("%q: семейство %s не собрано: %+v", query, fid, rejected)
		}
		rc.assignGroup(fh)
		rc.score(fh)
		fams = append(fams, fh)
	}
	sortFamilies(fams)
	return fams
}

func familyIDs(fams []*familyHit) []string {
	var ids []string
	for _, fh := range fams {
		ids = append(ids, fh.id)
	}
	return ids
}

// «kaspersky internet security продление»: агент нашёл RANKING_FAILURE — первым стоял Kaspersky Standard (выше
// скор), хотя семейство названо целиком. Теперь первым KIS, за ним Plus — продление KIS по справочнику.
func TestNamedFamilyAboveVendor(t *testing.T) {
	rels := map[string]float64{"kaspersky-standard": 1, "kaspersky-internet-security": 0.9, "kaspersky-plus": 0.85,
		"kaspersky-premium": 0.95}
	fams := rankFor(t, "kaspersky internet security продление", rels)
	got := familyIDs(fams)
	if got[0] != "kaspersky-internet-security" || got[1] != "kaspersky-plus" {
		t.Fatalf("порядок %v: первым KIS, вторым Plus (путь продления)", got)
	}
	if fams[0].group != domain.GroupExact || fams[0].head.LicenseType != domain.LicenseRenewal {
		t.Fatalf("KIS: %+v", fams[0])
	}
	if s := fams[0].signals; len(s) != 1 || s[0] != signalNamedFamily {
		t.Fatalf("сигнал в трассу: %v", s)
	}
	if s := fams[1].signals; len(s) != 1 || s[0] != signalRenewalPath {
		t.Fatalf("сигнал продления: %v", s)
	}
	// Без «продление» путь продления не поднимает Plus: остальные — по скору.
	got = familyIDs(rankFor(t, "kaspersky internet security", rels))
	if got[0] != "kaspersky-internet-security" || got[1] != "kaspersky-standard" {
		t.Fatalf("без продления: %v", got)
	}
}

// Назван только вендор или одно общее слово — ступеней нет, порядок по скору, как раньше.
func TestNamedFamilyNeedsFullName(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	for _, c := range []struct {
		q     string
		named []string
	}{
		{"касперский продление", nil},
		{"офис бессрочно", nil},
		{"kaspersky internet security", []string{"kaspersky-internet-security"}},
		{"kaspersky plus продление", []string{"kaspersky-plus"}},
		// Версии и редакции не путаются: названа 2021 — 2024 не названа, Pro — не Home.
		{"office 2021", []string{"office-2021"}},
		{"windows 11 pro", []string{"windows-11-pro"}},
		{"autocad lt", []string{"autodesk-autocad-lt"}},
	} {
		nr := normalizeFull(c.q, lx, rules, false)
		a := extractAttrs(nr.tokens)
		e := findEntities(nr, a, lx, rules)
		if len(e.named) != len(c.named) || len(c.named) > 0 && e.named[0] != c.named[0] {
			t.Errorf("%q: названы %v, ожидалось %v (семейства %v)", c.q, e.named, c.named, e.families)
		}
	}
	got := familyIDs(rankFor(t, "касперский продление", map[string]float64{"kaspersky-standard": 1,
		"kaspersky-internet-security": 0.9}))
	if got[0] != "kaspersky-standard" {
		t.Fatalf("вендор назван, семейство нет — по скору: %v", got)
	}
}

// Совместимость — не аналог; продление — отдельная связь от from к to.
func TestTypedRelations(t *testing.T) {
	lx, _, _ := mockLexicon(t)
	if contains(lx.substitutes["kaspersky-endpoint-security"], "astra-linux-se") {
		t.Fatal("compatibility KES → Astra не должна быть аналогом")
	}
	if !contains(lx.renewals["kaspersky-internet-security"], "kaspersky-plus") {
		t.Fatalf("renewal KIS → Plus: %v", lx.renewals)
	}
	if len(lx.renewals["kaspersky-plus"]) != 0 {
		t.Fatal("renewal односторонний")
	}
}

// Реранкер: только маршруты C/D; оценка смешивается с релевантностью; сбой — выдача без него.
func TestRerankFamilies(t *testing.T) {
	lx, _, _ := mockLexicon(t)
	fail := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail {
			http.Error(w, "down", http.StatusServiceUnavailable)
			return
		}
		var body struct{ Texts []string }
		_ = json.NewDecoder(r.Body).Decode(&body)
		var out []tei.RerankHit
		for i, tx := range body.Texts {
			sc := 0.1
			if strings.HasPrefix(tx, "КОМПАС-3D") {
				sc = 0.9
			}
			out = append(out, tei.RerankHit{Index: i, Score: sc})
		}
		_ = json.NewEncoder(w).Encode(out)
	}))
	defer srv.Close()
	mk := func() []*familyHit {
		return []*familyHit{
			{id: "autodesk-autocad", rel: 1, head: lx.docs[lx.families["autodesk-autocad"][0]]},
			{id: "kompas-3d", rel: 0.6, head: lx.docs[lx.families["kompas-3d"][0]]},
		}
	}
	s := &Services{reranker: tei.New(srv.URL, time.Second), rerankW: 0.45}
	fams := mk()
	if st := s.rerankFamilies(t.Context(), "чертежи по гост", domain.RouteNormalized, fams); !st.Skipped || fams[0].rel != 1 {
		t.Fatalf("маршрут B — без реранкера: %+v", st)
	}
	st := s.rerankFamilies(t.Context(), "чертежи по гост", domain.RouteIntent, fams)
	if st.Skipped || fams[1].rel <= fams[0].rel {
		t.Fatalf("КОМПАС должен обойти AutoCAD: %+v %.3f %.3f", st, fams[0].rel, fams[1].rel)
	}
	fail = true
	fams = mk()
	if st := s.rerankFamilies(t.Context(), "чертежи по гост", domain.RouteIntent, fams); !st.Skipped ||
		st.Reason != rerankFailed || fams[0].rel != 1 || fams[1].rel != 0.6 {
		t.Fatalf("сбой реранкера — порядок прежний: %+v", st)
	}
	s.rerankW = 0
	if st := s.rerankFamilies(t.Context(), "x", domain.RouteIntent, mk()); st.Reason != rerankOff {
		t.Fatalf("вес 0 — выключен: %+v", st)
	}
}
