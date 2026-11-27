package services

import (
	"encoding/json"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Правки по разбору набора оценки на фиде: основная линейка вендора из базы знаний, полное название семейства,
// имя вендора в подстановке алиаса, преемник в запросе со словом категории, «о365», характеристики оборудования из
// атрибутов и похожие вместо пустой выдачи, дополнение к лицензии не головой семейства.

// «астра» — только вендор: первой идёт основная линейка из базы знаний (Astra Linux SE), а не семейство, которое
// выше по тексту. Категория этого не различает: оба семейства в одной.
func TestKnownLineFirstForVendorOnly(t *testing.T) {
	rc := p6rc(p63catalog(), "astralinux")
	rc.vendorOnly = true
	order := func(lines map[string]bool) []string {
		rc.vendorLines = lines
		fams := p6fams(t, rc, domain.GroupExact, "f-ald", "f-se")
		for _, fh := range fams {
			fh.final = 0.5
		}
		fams[0].final = 0.9 // ALD Pro выше по тексту
		rc.markVendorCore(fams)
		sortFamilies(fams)
		var ids []string
		for _, fh := range fams {
			ids = append(ids, fh.id)
		}
		return ids
	}
	if got := order(nil); got[0] != "f-ald" {
		t.Fatalf("без базы знаний порядок по тексту: %v", got)
	}
	if got := order(map[string]bool{"f-se": true}); got[0] != "f-se" {
		t.Fatalf("основная линейка из базы знаний должна быть первой: %v", got)
	}
	// Запрос не «только вендор» и без условий покупки — порядок не трогается.
	rc.vendorOnly = false
	if got := order(map[string]bool{"f-se": true}); got[0] != "f-ald" {
		t.Fatalf("без признака «только вендор» линейка не поднимается: %v", got)
	}
}

// «Кибер Хранилище» и «Кибер Инфраструктура»: отличающее слово совпало со словом категории, значимое слово у обоих
// одно — «кибер». На «кибер инфраструктура» целиком назван только второй.
func TestFullNamedByWholeTitle(t *testing.T) {
	root := "soft"
	yes, price := true, 100.0
	s := &domain.CatalogSnapshot{Version: "t",
		Vendors: []domain.Vendor{{ID: "cyberprotect", Name: "Киберпротект"}},
		Categories: []domain.Category{{ID: root, Name: "Программы"},
			{ID: "nas", Name: "Сетевые хранилища", ParentID: &root}, {ID: "cloud", Name: "Облачная инфраструктура", ParentID: &root},
			{ID: "prot", Name: "Защита информации", ParentID: &root}},
		Products: []domain.Product{
			{ID: "s1", FamilyID: "f-store", FamilyName: "Кибер Хранилище", Title: "Киберпротект Кибер Хранилище, 1 ТБ",
				VendorID: "cyberprotect", CategoryID: "prot", Available: &yes, PriceRub: &price},
			{ID: "i1", FamilyID: "f-infra", FamilyName: "Кибер Инфраструктура", Title: "Киберпротект Кибер Инфраструктура, 1 CPU",
				VendorID: "cyberprotect", CategoryID: "prot"},
		}}
	lx := newSearchLexicon(s)
	if len(lx.familyWords["f-store"]) != 1 || len(lx.familyWords["f-infra"]) != 1 {
		t.Fatalf("предпосылка: значимое слово одно — «кибер»: %v %v", lx.familyWords["f-store"], lx.familyWords["f-infra"])
	}
	rc := p6rc(lx)
	rc.ent.named = []string{"f-infra", "f-store"}
	rc.ent.entityTokens = []string{"кибер", "инфраструктура"}
	if tier, _ := rc.tierOf("f-infra"); tier != 3 {
		t.Fatalf("«Кибер Инфраструктура» названа целиком: ступень %d", tier)
	}
	if tier, _ := rc.tierOf("f-store"); tier != 2 {
		t.Fatalf("«Кибер Хранилище» названо частично: ступень %d", tier)
	}
	// Значимые слова различаются — прежнее правило, полное название не вмешивается.
	if rc.sameSignificant("f-infra", "f-store") != true {
		t.Fatal("значимые слова совпадают")
	}
}

// «астра» → «группа астра»: оба слова подстановки — имя вендора, а «acronis» → «кибер бэкап» — слова продукта.
func TestAliasVendorWordsAreVendorTokens(t *testing.T) {
	lx := &searchLexicon{vendorName: map[string]string{"astralinux": "Группа Астра", "cyberprotect": "Киберпротект"}}
	e := queryEntities{entityTokens: []string{"группа", "астра"}, vendorTokens: map[string]bool{"астра": true}}
	e.markAliasVendorWords([]domain.Alias{{Alias: "астра", Canonical: "группа астра", Target: domain.AliasTargetVendor, TargetID: "astralinux"}}, lx)
	if !e.vendorTokens["группа"] || !e.vendorTokens["астра"] {
		t.Fatalf("«группа астра» — имя вендора: %v", e.vendorTokens)
	}
	e = queryEntities{entityTokens: []string{"кибер", "бэкап"}, vendorTokens: map[string]bool{}}
	e.markAliasVendorWords([]domain.Alias{{Alias: "acronis", Canonical: "кибер бэкап", Target: domain.AliasTargetVendor, TargetID: "cyberprotect"}}, lx)
	if len(e.vendorTokens) != 0 {
		t.Fatalf("слова продукта в подстановке именем вендора не становятся: %v", e.vendorTokens)
	}
}

// Преемник подставляется и в запросе со словом другой категории («zoom конференции»); запись словаря «о365» не
// превращается в номер «0365».
func TestSuccessorAndDigitAliasInQuery(t *testing.T) {
	lx := newSearchLexicon(understandingSnapshot())
	entries := append(testLexicon(),
		domain.LexiconEntry{Alias: "zoom", Canonical: "microsoft 365", Kind: domain.LexiconSuccessor, Target: domain.AliasTargetCategory,
			TargetIDs: []string{"suites"}, Basis: "тест", Note: "Zoom в каталоге нет"},
		domain.LexiconEntry{Alias: "о365", Canonical: "microsoft 365", Kind: domain.LexiconSpelling, Target: domain.AliasTargetTerm, Basis: "тест"})
	rules := applyLexicon(newSearchRules(1, lx.baseAliases), lx, entries)

	nr := normalizeFull("zoom антивирусы", lx, rules, false)
	if got := strings.Join(nr.tokens, " "); !strings.HasPrefix(got, "microsoft 365") || len(nr.successors) != 1 {
		t.Fatalf("преемник не подставлен рядом со словом другой категории: %q, %+v", got, nr.successors)
	}
	// Запись-категория без признака преемника по-прежнему уступает месту («антивирус для офиса»).
	nr = normalizeFull("антивирус для офиса", lx, rules, false)
	if strings.Contains(strings.Join(nr.tokens, " "), "office") {
		t.Fatalf("«для офиса» — место, а не офисный пакет: %v", nr.tokens)
	}
	nr = normalizeFull("о365", lx, rules, false)
	if got := strings.Join(nr.tokens, " "); got != "microsoft 365" {
		t.Fatalf("о365 → microsoft 365, получено %q", got)
	}
	nr = normalizeFull("office 2о21", lx, rules, false)
	if !strings.Contains(strings.Join(nr.tokens, " "), "2021") {
		t.Fatalf("«2о21» — по-прежнему номер версии: %v", nr.tokens)
	}
}

func TestSpecsFromFeedAttributes(t *testing.T) {
	got := specsFromAttributes(map[string]string{
		"Объем установленной ОЗУ": "16 ГБ", "Макс. объем памяти": "4096", "Объем установленного SSD": "512 ГБ",
		"Объем установленного HDD": "1 ТБ", "Диагональ экрана": "15.6", "Высота Rackmount": "2U", "Объем установленного eMMC": "не установлен",
	})
	want := map[string]float64{"ram_gb": 16, "storage_gb": 1512, "screen_inch": 15.6, "form_factor_u": 2}
	if len(got) != len(want) {
		t.Fatalf("характеристики: %v", got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("%s = %v, ожидалось %v (все: %v)", k, got[k], v, got)
		}
	}
	// Готовые характеристики из данных не перезаписываются; у ПО правила не работают.
	raw, _ := json.Marshal(map[string]string{"Объем установленной ОЗУ": "8 ГБ"})
	if s := docSpecs(domain.Product{Kind: domain.KindHardware, Attributes: raw, Specs: map[string]float64{"ram_gb": 32}}); s["ram_gb"] != 32 {
		t.Fatalf("specs из данных перезаписаны: %v", s)
	}
	if s := docSpecs(domain.Product{Kind: domain.KindSoftware, Attributes: raw}); len(s) != 0 {
		t.Fatalf("у ПО характеристик оборудования нет: %v", s)
	}
}

// hwFeedSnapshot — оборудование в форме фида: характеристики только строками атрибутов, без specs и реестра.
func hwFeedSnapshot() *domain.CatalogSnapshot {
	root := "hw"
	yes, price := true, 1000.0
	attrs := func(m map[string]string) domain.JSONB { b, _ := json.Marshal(m); return b }
	p := func(id, cat, title string, a map[string]string) domain.Product {
		return domain.Product{ID: id, FamilyID: id, FamilyName: title, Title: title, VendorID: "v", CategoryID: cat,
			Kind: domain.KindHardware, Available: &yes, PriceRub: &price, Attributes: attrs(a)}
	}
	return &domain.CatalogSnapshot{Version: "hw-feed", Vendors: []domain.Vendor{{ID: "v", Name: "Вендор"}},
		Categories: []domain.Category{{ID: root, Name: "Компьютеры и сеть"}, {ID: "nb", Name: "Ноутбуки", ParentID: &root},
			{ID: "sw", Name: "Коммутаторы", ParentID: &root}},
		Products: []domain.Product{
			p("n8", "nb", "Ноутбук Альфа 8", map[string]string{"Объем установленной ОЗУ": "8 ГБ"}),
			p("n16", "nb", "Ноутбук Бета 16", map[string]string{"Объем установленной ОЗУ": "16 ГБ"}),
			p("n32", "nb", "Ноутбук Гамма 32", map[string]string{"Объем установленной ОЗУ": "32 ГБ"}),
			p("s1", "sw", "Коммутатор Дельта SG-1", map[string]string{"Уровень коммутатора": "Layer 2"}),
			p("s2", "sw", "Коммутатор Эпсилон SG-2", map[string]string{"Уровень коммутатора": "Layer 3"}),
		}}
}

func hwFeedPlan(t *testing.T, q string) (*hwPlan, *catalogState) {
	t.Helper()
	snap := hwFeedSnapshot()
	svc := NewServices(nil, Options{Features: Features{Hardware: true}})
	st := newCatalogState(snap, 1)
	svc.catalog.Store(st)
	rules := newSearchRules(1, snap.Aliases)
	nr := normalizeFull(q, st.lex, rules, false)
	attrs := extractAttrs(nr.tokens)
	p := svc.hardwareParse(nil, st, nr, &attrs, in.SearchRequest{Query: q})
	ent := findEntities(nr, attrs, st.lex, rules)
	p.applyEntities(&ent)
	return p, st
}

func TestHardwareFeedSpecsAndRelaxation(t *testing.T) {
	// «ноутбук 16 гб»: ОЗУ разобрана из атрибута — подходит одна позиция, условие не снимается.
	p, st := hwFeedPlan(t, "ноутбук 16 гб")
	if p.category != "nb" || len(p.conds) != 1 || p.conds[0].Key != "ram_gb" {
		t.Fatalf("разбор: категория %q, условия %+v", p.category, p.conds)
	}
	p.setIndexed(&indexMeta{}) // в индексе нет полей specs.* — условие держится в памяти
	if f := p.filters(); strings.Contains(mustJSON(t, f), "specs.ram_gb") {
		t.Fatalf("фильтр по полю, которого нет в индексе, обнулил бы выдачу: %s", mustJSON(t, f))
	}
	if p.relaxSpecs() {
		t.Fatal("позиция с 16 ГБ есть — снимать нечего")
	}
	if r, _ := p.reject(st.lex.docs["n16"]); r != "" {
		t.Fatalf("ноутбук с 16 ГБ отклонён: %s", r)
	}
	if r, _ := p.reject(st.lex.docs["n8"]); r == "" {
		t.Fatal("ноутбук с 8 ГБ должен уйти в отказы")
	}

	// «ноутбук 24 гб»: такого нет — похожие, ближайшие по значению первыми, с пояснением.
	p, _ = hwFeedPlan(t, "ноутбук 24 гб")
	if !p.relaxSpecs() || len(p.conds) != 0 || len(p.relaxed) != 1 || p.relaxed[0].missing {
		t.Fatalf("условие должно сняться как «нет такого значения»: conds=%+v relaxed=%+v", p.conds, p.relaxed)
	}
	hits := p.structuredHits()
	if len(hits) != 3 || hits[0].ProductID != "n16" && hits[0].ProductID != "n32" {
		t.Fatalf("похожие — по близости ОЗУ: %+v", hits)
	}
	msgs := p.relaxedMessages()
	if len(msgs) != 1 || !strings.Contains(msgs[0].Message, "показаны похожие") {
		t.Fatalf("пояснение: %+v", msgs)
	}

	// «коммутатор 24 порта»: числа портов в данных нет ни у одной позиции — похожие и честное пояснение.
	p, _ = hwFeedPlan(t, "коммутатор 24 порта")
	if p.category != "sw" || !p.relaxSpecs() || !p.relaxed[0].missing {
		t.Fatalf("категория %q, relaxed=%+v", p.category, p.relaxed)
	}
	if hits := p.structuredHits(); len(hits) != 2 {
		t.Fatalf("все коммутаторы категории: %+v", hits)
	}
	if msgs := p.relaxedMessages(); len(msgs) != 1 || !strings.Contains(msgs[0].Message, "не проверить") {
		t.Fatalf("пояснение: %+v", msgs)
	}
	chips := p.relaxedChips(p.condChipsFor())
	for _, c := range chips {
		if c.Field == "ports_count" && (c.Strict || !strings.HasPrefix(c.Label, "Порты")) {
			t.Fatalf("чип снятого условия: %+v", c)
		}
	}
}

// condChipsFor — чипы снятых условий, как их строит hwPlan.chips до снятия.
func (p *hwPlan) condChipsFor() []out.Chip {
	var cs []out.Chip
	for _, r := range p.relaxed {
		cs = append(cs, p.condChip(r.cond))
	}
	return cs
}

func mustJSON(t *testing.T, v any) string {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// Дополнение к лицензии и переход с другого продукта фид помечает базовыми: головой семейства — новая лицензия.
func TestAddOnIsNotFamilyHead(t *testing.T) {
	if !addOnTitle("XSpider (дополнительный хост к лицензии), на 4 хоста") || !addOnTitle("XSpider Pro (на дополнительный IP), к лицензии на 4 IP") {
		t.Fatal("дополнения не распознаны")
	}
	if addOnTitle("Secret Net Studio 8 (дополнительная защита), лицензия на 1 год") {
		t.Fatal("«дополнительная защита» — редакция, а не дополнение")
	}
	if !migrationTitle("XSpider Pro (лицензия перехода с Xspider на XSpider Pro)") || migrationTitle("Переход на новую версию") {
		t.Fatal("переход с другого продукта")
	}
	price, yes := 100.0, true
	base := indexDoc{ProductID: "b", Title: "XSpider Pro (лицензия Pentest), на 4 IP", LicenseKind: domain.LicenseKindBase, PriceRub: &price, Available: &yes}
	add := indexDoc{ProductID: "a", Title: "XSpider (дополнительный хост к лицензии), на 4 хоста", LicenseKind: domain.LicenseKindBase, PriceRub: &price, Available: &yes}
	rc := p6rc(&searchLexicon{})
	if less, ok := rc.headLess(base, add); !ok || !less {
		t.Fatal("новая лицензия должна быть головой раньше дополнения")
	}
	rc.ent.textTokens = []string{"дополнительный", "хост", "xspider"}
	if _, ok := rc.headLess(base, add); ok {
		t.Fatal("дополнение названо в запросе — по этому признаку варианты равны")
	}
}
