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

func TestOrderCompletionPrefersClassedLine(t *testing.T) {
	f := newFx()
	fam := func(fid, name string, offers int, price float64) {
		for i := 0; i < offers; i++ {
			f.add(indexDoc{ProductID: fid + "-" + string(rune('a'+i%26)) + string(rune('a'+i/26)), FamilyID: fid,
				FamilyName: name, VendorID: "kaspersky", Vendor: "Kaspersky", CategoryID: "av", Title: name,
				LicenseKind: domain.LicenseKindBase, PriceRub: ptr(price + float64(i))})
		}
	}
	fam("shd", "Kaspersky Security для систем хранения данных", 30, 476)
	fam("ksos", "Kaspersky Small Office Security", 5, 991)
	fam("kes-std", "Kaspersky Endpoint Security для бизнеса Стандартный", 10, 684)
	fam("kes-ext", "Kaspersky Endpoint Security для бизнеса Расширенный", 10, 991)
	fam("kes-cloud", "Kaspersky Endpoint Security Cloud", 12, 2022)
	classOf := func(fid string) string {
		if fid == "shd" {
			return "" // узкий продукт: класса в таблице допродажи нет
		}
		return "antivirus"
	}
	r := completionFamilies(f.lx, completion{text: "kaspersky", vendor: "kaspersky"}, classOf)
	if r == nil || len(r.families) != 5 {
		t.Fatalf("семейства вендора: %+v", r)
	}
	if r.families[0].id != "kes-std" {
		t.Fatalf("первым — самая широкая линейка с классом, внутри — дешевле вход: %s", r.families[0].id)
	}
	if r.families[len(r.families)-1].id != "shd" {
		t.Fatalf("без класса — после семейств с классом, хоть позиций и больше: %s", r.families[len(r.families)-1].id)
	}
}

func TestSuggestDisplay(t *testing.T) {
	cases := map[string]string{
		"«Лаборатория Касперского»":                    "Лаборатория Касперского",
		"Программа КриптоПро CSP 5.0":                  "КриптоПро CSP 5.0",
		"Операционная система РЕД ОС":                  "РЕД ОС",
		"Эшелон «СКАНЕР-ВС»":                           "Эшелон СКАНЕР-ВС",
		"Программный комплекс «ALD Pro»":               "ALD Pro",
		"Microsoft Windows 11 Pro  (Профессиональная)": "Microsoft Windows 11 Pro",
		"Kaspersky (для дома)":                         "Kaspersky (для дома)",
	}
	for in, want := range cases {
		if got := suggestDisplay(in); got != want {
			t.Errorf("%q → %q, ждали %q", in, got, want)
		}
	}
}

func TestMergeDictHits(t *testing.T) {
	got := mergeDictHits([]dictHit{{entry: 1, score: 0.5}, {entry: 2, score: 0.4}}, []dictHit{{entry: 2, score: 0.9}, {entry: 3, score: 0.1}})
	if len(got) != 3 || got[0].entry != 2 || got[0].score != 0.9 {
		t.Fatalf("без повторов, лучший вес, по убыванию: %+v", got)
	}
}

func TestDictionaryForms(t *testing.T) {
	lex := []domain.LexiconEntry{
		{Alias: "винда", Canonical: "windows", Kind: domain.LexiconJargon, Target: "category"},
		{Alias: "винду", Canonical: "windows", Kind: domain.LexiconJargon, Target: "category"},
		{Alias: "кибербэкап", Canonical: "кибер бэкап", Kind: domain.LexiconSpelling, Target: "term"},
		{Alias: "кибер бекап", Canonical: "кибер бэкап", Kind: domain.LexiconSpelling, Target: "term"},
		{Alias: "eset", Canonical: "pro32", Kind: domain.LexiconSuccessor, Target: "term"},
		{Alias: "касперыч", Canonical: "kaspersky", Kind: domain.LexiconJargon, Target: "vendor"},
	}
	base := []domain.Alias{{Alias: "касперский", Canonical: "kaspersky", Kind: "transliteration", Target: "vendor", TargetID: "kaspersky"},
		{Alias: "kaspersky-lab", Canonical: "kaspersky", Kind: "alias", Target: "vendor", TargetID: "kaspersky"}}
	got := map[string]float64{}
	for _, f := range dictionaryForms(lex, base) {
		got[f.text] = f.freq
	}
	want := map[string]float64{"винда": 0.3, "кибер бекап": 0.6, "кибер бэкап": 0.6, "касперский": 0.6}
	if len(got) != len(want) {
		t.Fatalf("одна форма на каноническую + термин: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("%q: %v, ждали %v (всё: %v)", k, got[k], v, got)
		}
	}
}
