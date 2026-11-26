package services

import (
	"errors"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/out"
)

func TestPanelScorePrefix(t *testing.T) {
	fam, pid := "kaspersky-plus", "kplus-1y"
	p := domain.PanelPrefix{ID: "px-1", Prefix: "касп", TargetQuery: "Касперский Плюс", TargetFamily: &fam,
		TargetProduct: &pid, Type: domain.PrefixFamily}
	r := scorePanelPrefix(p, pevalObs{suggestions: []string{"касперский", "касперский  плюс"},
		families: []string{"kis", fam, "kis"}, products: []string{"kis-1", pid, "kis-2"}})
	if r.SuggestRank != 2 || r.ProductRank != 2 || !r.FamilyDup || r.ExactOK != nil {
		t.Fatalf("ранги %+v", r)
	}
	if r := scorePanelPrefix(p, pevalObs{suggestions: []string{"a", "b", "c", "d", "e", "касперский плюс"}}); r.SuggestRank != 0 {
		t.Fatalf("цель шестой — вне @5, ранг %d", r.SuggestRank)
	}
}

func TestPanelExactCheck(t *testing.T) {
	fam, pid := "srv", "srv-r750"
	p := domain.PanelPrefix{ID: "px-2", Prefix: "R750-XS", TargetQuery: "r750-xs", TargetFamily: &fam, TargetProduct: &pid,
		Type: domain.PrefixSKU}
	if r := scorePanelPrefix(p, pevalObs{families: []string{fam}, products: []string{pid}}); r.ExactOK == nil || !*r.ExactOK {
		t.Fatal("точный первым — не вытеснен")
	}
	if r := scorePanelPrefix(p, pevalObs{families: []string{"other", fam}, products: []string{"x", pid}}); *r.ExactOK {
		t.Fatal("точный вторым — вытеснен")
	}
	// прежние подсказки дают только семейства
	if r := scorePanelPrefix(p, pevalObs{families: []string{fam}}); !*r.ExactOK {
		t.Fatal("семейство первым без позиции — не вытеснен")
	}
	p.Prefix = "R750"
	if r := scorePanelPrefix(p, pevalObs{}); r.ExactOK != nil {
		t.Fatal("неполный артикул — не проверка точного")
	}
}

func TestAggregatePanel(t *testing.T) {
	f1, f2 := "f1", "f2"
	ps := []domain.PanelPrefix{
		{ID: "a", Prefix: "ab", TargetQuery: "abc", TargetFamily: &f1, Type: domain.PrefixFamily},
		{ID: "b", Prefix: "xy", TargetQuery: "xyz", TargetFamily: &f2, Type: domain.PrefixBroad},
		{ID: "c", Prefix: "no", TargetQuery: "nothing", Type: domain.PrefixNoResults},
		{ID: "d", Prefix: "er", TargetQuery: "err", Type: domain.PrefixFamily},
	}
	obs := []pevalObs{
		{suggestions: []string{"abc"}, families: []string{f1, f1}, ms: 2},
		{suggestions: []string{"x", "xyz"}, families: []string{"f9", f2}, ms: 4},
		{ms: 10},
		{err: errors.New("сбой")},
	}
	rows := make([]out.PanelPrefixResult, len(ps))
	for i := range ps {
		rows[i] = scorePanelPrefix(ps[i], obs[i])
	}
	m := aggregatePanel(ps, rows, obs, []pevalSaved{{length: 10, found: 3}, {length: 10}})
	eq := func(name string, got *float64, want float64) {
		t.Helper()
		if got == nil || *got < want-1e-9 || *got > want+1e-9 {
			t.Fatalf("%s = %v, want %v", name, got, want)
		}
	}
	if m.Errors != 1 || m.SuggestN != 2 || m.ProductN != 2 || m.EmptySuggest != 0 || m.FamilyDupPrefixes != 1 {
		t.Fatalf("счётчики %+v", m)
	}
	eq("MRR подсказок", m.SuggestMRR5, 0.75)
	eq("S@5 подсказок", m.SuggestSuccess5, 1)
	eq("MRR товаров", m.ProductMRR5, 0.75)
	eq("сэкономлено", m.SavedKeystrokes, 3.5)
	eq("доля", m.SavedShare, 0.35)
	eq("p50", m.P50Ms, 4)
	eq("p95", m.P95Ms, 10)
	eq("дубли", m.FamilyDupRate, 0.5)
}

func TestPanelChanges(t *testing.T) {
	f := "f"
	ps := []domain.PanelPrefix{{ID: "a", TargetFamily: &f, Type: domain.PrefixFamily}, {ID: "b", Type: domain.PrefixBroad}}
	old := []out.PanelPrefixResult{{SuggestRank: 1, ProductRank: 0}, {SuggestRank: 0}}
	cur := []out.PanelPrefixResult{{SuggestRank: 3, ProductRank: 2}, {SuggestRank: 0}}
	ch := pevalChanges(ps, old, cur)
	if len(ch) != 2 || ch[0].Kind != "regression" || ch[0].Section != "suggestions" || ch[1].Kind != "improvement" {
		t.Fatalf("изменения %+v", ch)
	}
}

func TestPanelRunFromRows(t *testing.T) {
	run := &out.PanelEvalRun{RunID: "r1", Set: domain.EvalSetPanel, SetHash: "h", Prefixes: 2, Warnings: []string{},
		Modes: []out.PanelModeResult{
			{Mode: panelEvalOld, Available: true, Metrics: out.PanelMetrics{SuggestN: 2}, ByType: map[string]out.PanelMetrics{}},
			{Mode: panelEvalNew, Reason: "нет", ByType: map[string]out.PanelMetrics{}},
		},
		Changes: []out.PanelChange{{ID: "a", Kind: "regression"}}}
	back, err := panelRunFromRows(panelEvalRows(run))
	if err != nil {
		t.Fatal(err)
	}
	if len(back.Modes) != 2 || back.Modes[0].Metrics.SuggestN != 2 || back.Modes[1].Available || back.Modes[1].Reason != "нет" ||
		len(back.Changes) != 1 {
		t.Fatalf("прогон из строк %+v", back)
	}
	if r, _ := panelRunFromRows(nil); r != nil {
		t.Fatal("без строк — nil")
	}
}
