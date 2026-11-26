package services

import (
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
)

// understandingSnapshot — маленький каталог в духе фида заказчика: оборудование и ПО со словом «сервер»,
// антивирусы, Windows, число в названии, составное название через дефис и вендор со «случайным» слагом.
func understandingSnapshot() *domain.CatalogSnapshot {
	yes := true
	price := 100.0
	p := func(id, fam, famName, title, vendor, cat, kind string) domain.Product {
		return domain.Product{ID: id, FamilyID: fam, FamilyName: famName, Title: title, VendorID: vendor, CategoryID: cat,
			Kind: kind, Available: &yes, PriceRub: &price}
	}
	return &domain.CatalogSnapshot{
		Version: "test",
		Vendors: []domain.Vendor{{ID: "depo", Name: "DEPO"}, {ID: "inferit", Name: "Инферит"}, {ID: "drweb", Name: "Доктор Веб"},
			{ID: "kaspersky", Name: "«Лаборатория Касперского»"}, {ID: "microsoft", Name: "Microsoft"},
			{ID: "cyberlink", Name: "CyberLink"}, {ID: "echelon", Name: "НПО «Эшелон»"}, {ID: "pt", Name: "Positive Technologies"},
			{ID: "pro32", Name: "PRO32"}, {ID: "kit", Name: "Компания информационных технологий"}, {ID: "officekit", Name: "Office Kit"},
			{ID: "mdaemon", Name: "MDaemon Technologies"}},
		Categories: []domain.Category{
			{ID: "srv", Name: "Серверы и опции"}, {ID: "rack", Name: "Rack-серверы", ParentID: sp("srv")},
			{ID: "srvopt", Name: "Серверные опции", ParentID: sp("srv")},
			{ID: "net", Name: "Сеть и интернет"}, {ID: "srvsw", Name: "Серверное ПО", ParentID: sp("net")},
			{ID: "sec", Name: "Антивирусы и безопасность"}, {ID: "avorg", Name: "Антивирусы для организаций", ParentID: sp("sec")},
			{ID: "avhome", Name: "Антивирусы для дома", ParentID: sp("sec")}, {ID: "infosec", Name: "Интернет и Сеть", ParentID: sp("sec")},
			{ID: "os", Name: "Операционные системы"}, {ID: "win", Name: "Операционные системы Windows", ParentID: sp("os")},
			{ID: "office", Name: "Офисные программы"}, {ID: "suites", Name: "Офисные пакеты программ Microsoft Office", ParentID: sp("office")},
			{ID: "mm", Name: "Мультимедиа"}, {ID: "players", Name: "Плееры", ParentID: sp("mm")},
			{ID: "ot", Name: "Офисная техника"}, {ID: "shred", Name: "Шредеры", ParentID: sp("ot")},
		},
		Products: []domain.Product{
			p("r1", "r1", "Сервер INFERIT RS212", "Сервер INFERIT RS212", "inferit", "rack", domain.KindHardware),
			p("r2", "r2", "Rack-сервер DEPO Storm 3470", "Rack-сервер DEPO Storm 3470", "depo", "rack", domain.KindHardware),
			p("r3", "r3", "Рабочая станция DEPO Race ST918", "Рабочая станция DEPO Race ST918", "depo", "rack", domain.KindHardware),
			p("o1", "o1", "Радиатор процессора для сервера", "Радиатор процессора для сервера", "depo", "srvopt", domain.KindHardware),
			p("m1", "m1", "MDaemon Messaging Server", "MDaemon Messaging Server, 5 пользователей", "mdaemon", "srvsw", domain.KindSoftware),
			p("d1", "d1", "Dr.Web Desktop Security Suite", "Доктор Веб Антивирус, 1 ПК", "drweb", "avorg", domain.KindSoftware),
			p("k1", "k1", "Kaspersky Endpoint Security для бизнеса", "Kaspersky Endpoint Security", "kaspersky", "avorg", domain.KindSoftware),
			p("k2", "k2", "Kaspersky Total Security для бизнеса", "Kaspersky Total Security", "kaspersky", "avorg", domain.KindSoftware),
			p("p1", "p1", "PRO32 Total Security", "PRO32 Total Security", "pro32", "avhome", domain.KindSoftware),
			p("w1", "w1", "Microsoft Windows Server Standard 2022", "Microsoft Windows Server Standard 2022", "microsoft", "win", domain.KindSoftware),
			p("x1", "x1", "Microsoft 365 Family", "Microsoft 365 для семьи", "microsoft", "suites", domain.KindSoftware),
			p("x2", "x2", "Microsoft 365 Personal", "Microsoft 365 Персональный", "microsoft", "suites", domain.KindSoftware),
			p("x3", "x3", "Microsoft Dynamics 365", "Microsoft Dynamics 365", "microsoft", "suites", domain.KindSoftware),
			p("c1", "c1", "CyberLink PowerDVD 365", "CyberLink PowerDVD 365", "cyberlink", "players", domain.KindSoftware),
			p("e1", "e1", "Эшелон «СКАНЕР-ВС»", "НПО «Эшелон» СКАНЕР-ВС 7", "echelon", "infosec", domain.KindSoftware),
			p("t1", "t1", "MaxPatrol Endpoint Protection Platform", "Positive Technologies MaxPatrol EPP", "pt", "avorg", domain.KindSoftware),
			p("s1", "s1", "Шредер Office Kit S145", "Шредер Office Kit S145", "officekit", "shred", domain.KindHardware),
			p("s2", "s2", "Шредер Office Kit S250", "Шредер Office Kit S250", "officekit", "shred", domain.KindHardware),
			p("kt", "kt", "Система учёта КИТ", "КИТ Система учёта", "kit", "office", domain.KindSoftware),
		},
		Aliases: []domain.Alias{
			{Alias: "офис", Canonical: "office", Kind: "transliteration", Target: domain.AliasTargetTerm},
			{Alias: "сертификат", Canonical: "сертифицирован", Kind: "alias", Target: domain.AliasTargetTerm},
			{Alias: "kit", Canonical: "Компания информационных технологий", Kind: "alias", Target: domain.AliasTargetVendor, TargetID: "kit"},
			{Alias: "kaspersky", Canonical: "«Лаборатория Касперского»", Kind: "alias", Target: domain.AliasTargetVendor, TargetID: "kaspersky"},
		},
	}
}

func sp(s string) *string { return &s }

func testLexicon() []domain.LexiconEntry {
	return []domain.LexiconEntry{
		{Alias: "офис", Canonical: "office", Kind: domain.LexiconJargon, Target: domain.AliasTargetCategory, TargetIDs: []string{"suites", "nope"}, Basis: "ТЗ"},
		{Alias: "винда", Canonical: "windows", Kind: domain.LexiconJargon, Target: domain.AliasTargetCategory, TargetIDs: []string{"win"}, Basis: "ТЗ"},
		{Alias: "eset", Canonical: "pro32", Kind: domain.LexiconSuccessor, Target: domain.AliasTargetVendor, TargetIDs: []string{"pro32"}, Basis: "ComNews", Note: "на этом рынке — PRO32"},
		{Alias: "сертификат", Kind: domain.LexiconDrop, Basis: "фид"},
		{Alias: "acronis", Canonical: "киберпротект", Kind: domain.LexiconSuccessor, Target: domain.AliasTargetVendor, TargetIDs: []string{"cyberprotect"}, Basis: "нет"},
	}
}

func TestApplyLexicon(t *testing.T) {
	snap := understandingSnapshot()
	lx := newSearchLexicon(snap)
	r := applyLexicon(newSearchRules(1, lx.baseAliases), lx, testLexicon())
	if a := r.aliases["офис"]; a.Kind != domain.LexiconJargon || a.Target != domain.AliasTargetCategory || a.TargetID != "suites" {
		t.Fatalf("офис: запись словаря должна заменить запись импорта, получено %+v", a)
	}
	if got := r.categories["офис"]; len(got) != 1 || got[0] != "suites" {
		t.Fatalf("офис: категории %v, ожидалась только существующая suites", got)
	}
	if _, ok := r.aliases["сертификат"]; ok {
		t.Fatal("сертификат: drop должен снять запись импорта")
	}
	if _, ok := r.successors["eset"]; !ok || r.aliases["eset"].TargetID != "pro32" {
		t.Fatalf("eset: преемник не записан: %+v", r.aliases["eset"])
	}
	if _, ok := r.aliases["acronis"]; ok {
		t.Fatal("acronis: вендора нет в каталоге — записи быть не должно")
	}
	if _, ok := r.aliases["kit"]; ok {
		t.Fatal("kit: слаг, который в названиях означает чужие продукты (Office Kit), не должен оставаться алиасом вендора")
	}
	if _, ok := r.aliases["kaspersky"]; !ok {
		t.Fatal("kaspersky: однозначный слаг должен остаться")
	}
	// Запись, изменённая агентом (не совпадает с импортом), остаётся за агентом.
	agent := newSearchRules(5, []domain.Alias{{Alias: "офис", Canonical: "р7-офис", Kind: "slang", Target: domain.AliasTargetTerm}})
	if a := applyLexicon(agent, lx, testLexicon()).aliases["офис"]; a.Canonical != "р7-офис" {
		t.Fatalf("офис: правка агента перезаписана словарём: %+v", a)
	}
}

func TestVendorAmbiguousPhrase(t *testing.T) {
	lx := newSearchLexicon(understandingSnapshot())
	if !lx.vendorAmbiguous["kit"] {
		t.Fatal("kit: в названиях семейств чаще Office Kit, чем продукты вендора kit")
	}
	if lx.vendorAmbiguous["kaspersky"] {
		t.Fatal("kaspersky: однозначный вендор")
	}
	if got := lx.matchVendors([]string{"office", "kit"}, nil); len(got) == 1 && got[0] == "kit" {
		t.Fatalf("office kit: вендор kit не должен находиться, получено %v", got)
	}
}

func TestHeadScoreNounAdjective(t *testing.T) {
	cases := []struct {
		name, word string
		want       int
	}{
		{"Серверы и опции", "сервер", 2},
		{"Rack-серверы", "сервер", 2},
		{"Серверное ПО", "сервер", 0},
		{"Серверные опции", "сервер", 0},
		{"Антивирусы для организаций", "антивирус", 2},
		{"Антивирусы для дома", "дома", 0},
		{"Ноутбуки", "ноутбук", 3},
		{"Оперативная память", "память", 1},
		{"Мебель, товары для офиса", "офис", 0},
	}
	for _, c := range cases {
		if got := headScore(c.name, c.word); got != c.want {
			t.Errorf("headScore(%q, %q) = %d, want %d", c.name, c.word, got, c.want)
		}
	}
	if !isAdjective("серверная") || isAdjective("сервер") || nounStem("серверов") != "сервер" {
		t.Error("прилагательное и основа существительного")
	}
}

func intentOf(t *testing.T, lx *searchLexicon, rules *searchRules, q string) *categoryIntent {
	t.Helper()
	nr := normalizeFull(q, lx, rules, false)
	attrs := extractAttrs(nr.tokens)
	ent := findEntities(nr, attrs, lx, rules)
	return findCategoryIntent(nr, ent, lx, rules, in.SearchRequest{Query: q})
}

func TestCategoryIntent(t *testing.T) {
	lx := newSearchLexicon(understandingSnapshot())
	rules := applyLexicon(newSearchRules(1, lx.baseAliases), lx, testLexicon())

	ci := intentOf(t, lx, rules, "сервер")
	if ci == nil || len(ci.cats) != 1 || ci.cats[0] != "rack" {
		t.Fatalf("сервер: ожидалась подкатегория rack (Rack-серверы) — она точнее корня, получено %+v", ci)
	}
	if !ci.keeps(lx.docs["r1"]) || !ci.keeps(lx.docs["r2"]) {
		t.Error("сервер: «Сервер INFERIT» и «Rack-сервер DEPO» — серверы")
	}
	if ci.keeps(lx.docs["r3"]) || ci.keeps(lx.docs["o1"]) {
		t.Error("сервер: рабочая станция и радиатор из той же категории — не серверы")
	}
	other := map[string]bool{}
	for _, m := range ci.other {
		other[m.id] = true
	}
	if !other["srvsw"] {
		t.Errorf("сервер: «Серверное ПО» — другой смысл, уточнения %+v", ci.other)
	}

	if ci := intentOf(t, lx, rules, "антивирус"); ci == nil || len(ci.cats) != 2 {
		t.Fatalf("антивирус: ожидались две подкатегории антивирусов (без корня), получено %+v", ci)
	}
	if ci := intentOf(t, lx, rules, "антивирус для сервера"); ci == nil || ci.cats[0] != "avorg" && ci.cats[0] != "avhome" {
		t.Fatalf("антивирус для сервера: главное слово до «для» — антивирус, получено %+v", ci)
	}
	if ci := intentOf(t, lx, rules, "почтовый сервер"); ci != nil {
		t.Fatalf("почтовый сервер: «почтовый» не из названия серверов — намерения оборудования нет, получено %+v", ci)
	}
	if ci := intentOf(t, lx, rules, "винда"); ci == nil || ci.source != "lexicon" || ci.cats[0] != "win" {
		t.Fatalf("винда: категория из словаря, получено %+v", ci)
	}
	if ci := intentOf(t, lx, rules, "kaspersky endpoint security"); ci != nil {
		t.Fatalf("название продукта — не намерение по категории, получено %+v", ci)
	}
	req := in.SearchRequest{Query: "сервер", Filters: []in.Filter{{Key: facetCategory, Op: "eq", Value: "srvsw"}}}
	nr := normalizeFull("сервер", lx, rules, false)
	if ci := findCategoryIntent(nr, findEntities(nr, extractAttrs(nr.tokens), lx, rules), lx, rules, req); ci != nil {
		t.Fatal("явный фильтр категории — намерения нет")
	}
}

func TestNumbersCodesAndJoins(t *testing.T) {
	lx := newSearchLexicon(understandingSnapshot())
	rules := newSearchRules(1, lx.baseAliases)

	nr := normalizeFull("365", lx, rules, false)
	if isNoiseQuery(nr, lx, rules) {
		t.Fatal("365: число из названий семейств — не шум")
	}
	if isNoiseQuery(normalizeFull("777", lx, rules, false), lx, rules) == false {
		t.Fatal("777: число, которого нет в названиях, — шум")
	}
	if v := lx.numberOwner([]string{"365"}); v != "microsoft" {
		t.Fatalf("365: владелец числа — microsoft (3 семейства против 1), получено %q", v)
	}
	if got := normalizeFull("max patrol", lx, rules, false).normalized(); got != "maxpatrol" {
		t.Fatalf("max patrol → %q, ожидалось maxpatrol", got)
	}
	if got := normalizeFull("total security", lx, rules, false).normalized(); got != "total security" {
		t.Fatalf("total security склеено: %q", got)
	}
	if !lx.compoundNames["сканер-вс"] {
		t.Fatal("сканер-вс — составное название из семейства")
	}
	if c, _ := lx.categoryOf([]string{"сканер-вс"}); c != "" {
		t.Fatalf("сканер-вс: категория по части названия %q", c)
	}
	if got := normalizeFull("kaspersky totl security", lx, rules, false).normalized(); got != "kaspersky total security" {
		t.Fatalf("totl → total по соседнему слову: %q", got)
	}
}

func TestSuccessorAndAliasReplacement(t *testing.T) {
	lx := newSearchLexicon(understandingSnapshot())
	rules := applyLexicon(newSearchRules(1, lx.baseAliases), lx, testLexicon())
	nr := normalizeFull("eset", lx, rules, false)
	if len(nr.successors) != 1 || nr.successors[0].note == "" {
		t.Fatalf("eset: преемник с пояснением, получено %+v", nr.successors)
	}
	ent := findEntities(nr, extractAttrs(nr.tokens), lx, rules)
	if len(ent.successors) != 1 || len(ent.vendors) != 1 || ent.vendors[0] != "pro32" {
		t.Fatalf("eset: вендор преемника PRO32, получено %+v / %v", ent.successors, ent.vendors)
	}
	if got := normalizeFull("kaspersky", lx, rules, false).normalized(); got != "kaspersky" {
		t.Fatalf("kaspersky → %q: слаг вендора, который так и пишется в названиях, не заменяется именем с кавычками", got)
	}
}
