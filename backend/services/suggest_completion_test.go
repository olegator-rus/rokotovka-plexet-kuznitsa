package services

import (
	"testing"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Дополнение префикса по словарю и вес вендора в панели подсказок (контур эталонов).

func TestAliasCompletion(t *testing.T) {
	rules := newSearchRules(1, []domain.Alias{
		{Alias: "энидеск", Canonical: "anydesk", Target: "term"},
		{Alias: "эни деск", Canonical: "anydesk", Target: "term"},
		{Alias: "каспер", Canonical: "kaspersky", Target: "vendor", TargetID: "kaspersky"},
		{Alias: "касперский", Canonical: "kaspersky", Target: "vendor", TargetID: "kaspersky"},
		{Alias: "астра", Canonical: "группа астра", Target: "vendor", TargetID: "astralinux"},
		{Alias: "астра линукс", Canonical: "astra linux", Target: "term"},
		{Alias: "макспатрол", Canonical: "maxpatrol", Target: "term"},
		{Alias: "офис", Canonical: "office", Target: "category"},
	})
	cases := []struct {
		typed, text, vendor string
		ok                  bool
	}{
		{"эни", "anydesk", "", true},
		{"касп", "kaspersky", "kaspersky", true},
		{"аст", "группа астра", "astralinux", true},
		{"макс", "maxpatrol", "", true},
		{"офис", "office", "", true},
		{"ка", "", "", false},  // короче трёх символов
		{"365", "", "", false}, // цифры не дополняются
		{"zzz", "", "", false},
	}
	for _, c := range cases {
		text, vendor, ok := aliasCompletion(c.typed, rules)
		if text != c.text || vendor != c.vendor || ok != c.ok {
			t.Errorf("%q → %q/%q/%v, ждали %q/%q/%v", c.typed, text, vendor, ok, c.text, c.vendor, c.ok)
		}
	}
}

func TestRankPanelVendors(t *testing.T) {
	f := newFx()
	add := func(id, fam, vendor string) {
		f.add(indexDoc{ProductID: id, FamilyID: fam, FamilyName: fam, VendorID: vendor, Vendor: vendor, CategoryID: "sec",
			Title: fam, LicenseKind: domain.LicenseKindBase})
	}
	add("c1", "cc1", "cryptocom")
	for _, id := range []string{"k1", "k2", "k3"} {
		add(id, "kp-"+id, "kripto-pro")
	}
	vs := []out.PanelVendor{{ID: "cryptocom", Name: "Криптоком"}, {ID: "kripto-pro", Name: "КриптоПро"}}
	got := rankPanelVendors(f.lx, append([]out.PanelVendor(nil), vs...), "")
	if got[0].ID != "kripto-pro" {
		t.Fatalf("вендор с бо́льшим числом семейств в наличии — первым: %+v", got)
	}
	got = rankPanelVendors(f.lx, append([]out.PanelVendor(nil), vs...), "cryptocom")
	if got[0].ID != "cryptocom" {
		t.Fatalf("вендор дополнения словаря — первым: %+v", got)
	}
}
