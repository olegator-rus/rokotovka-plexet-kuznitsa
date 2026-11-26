package services

import (
	"strings"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Допродажа на фиде (upsell_feed.go): связи из типа лицензии с проверкой редакции, совместимость с документом,
// варианты SKU под платформу, экосистема, акции. Каталог — синтетический, в форме фида.

type fx struct{ lx *searchLexicon }

func newFx() *fx {
	return &fx{lx: &searchLexicon{docs: map[string]indexDoc{}, families: map[string][]string{},
		familyName: map[string]string{}, familyVendor: map[string]string{}, familyCat: map[string]string{},
		categoryName: map[string]string{"os": "Российские операционные системы", "sec": "Программы для защиты информации",
			"backup": "Программное обеспечение для резервного копирования", "vdi": "Виртуализация", "ecm": "СЭД"},
		vendorName: map[string]string{}}}
}

func (f *fx) add(d indexDoc) {
	if d.PriceRub == nil {
		d.PriceRub = ptr(1000.0)
	}
	if d.Available == nil {
		d.Available = ptr(true)
	}
	if d.OfferURL == "" {
		d.OfferURL = "https://store.example/" + d.ProductID + "/"
	}
	if _, ok := f.lx.familyName[d.FamilyID]; !ok {
		f.lx.familyOrder = append(f.lx.familyOrder, d.FamilyID)
	}
	f.lx.docs[d.ProductID] = d
	f.lx.families[d.FamilyID] = append(f.lx.families[d.FamilyID], d.ProductID)
	f.lx.familyName[d.FamilyID] = d.FamilyName
	f.lx.familyVendor[d.FamilyID] = d.VendorID
	f.lx.familyCat[d.FamilyID] = d.CategoryID
	f.lx.vendorName[d.VendorID] = d.Vendor
}

func (f *fx) run(anchorPID string, data *upsellData) (*upsellRun, *UpsellStep) {
	if data == nil {
		data = &upsellData{}
	}
	ix := buildUpsellIndex(f.lx, data)
	step := &UpsellStep{Considered: []UpsellConsidered{}, Rejected: []domain.RejectedHit{}}
	return &upsellRun{ix: ix, lx: f.lx, anchor: f.lx.docs[anchorPID], shown: map[string]bool{}, step: step,
		rejected: map[string]bool{}}, step
}

// astraFixture — ОС с редакциями «Орел»/«Воронеж» и назначением; КриптоПро с вариантом под платформу; бэкап «для
// рабочей станции Linux»; САПР «для Linux»; продукты вендора ОС в других категориях.
func astraFixture() *fx {
	f := newFx()
	astra := func(id, kind, ed, purpose string, price float64, title string) {
		f.add(indexDoc{ProductID: id, FamilyID: "astra", FamilyName: "Astra Linux Special Edition", VendorID: "astralinux",
			Vendor: "Группа Астра", CategoryID: "os", Title: title, LicenseKind: kind, LicenseEdition: ed,
			LicenseOrgType: "commercial", PriceRub: ptr(price), FamilyURL: "https://store.example/astralinux/astra/",
			VendorURL: "https://store.example/astralinux/", Description: "Платформа: Linux",
			Attributes: "Версия: " + ed + "; Назначение: " + purpose + "; Совместимая архитектура/платформа: x86-64"})
	}
	astra("a-orel-ws", domain.LicenseKindBase, "Орел", "Рабочая станция", 3100, "Astra Linux SE, «Орел», для рабочей станции")
	astra("a-vor-ws", domain.LicenseKindBase, "Воронеж", "Рабочая станция", 5100, "Astra Linux SE, «Воронеж», для рабочей станции")
	astra("s-vor-ws", domain.LicenseKindSupport, "Воронеж", "Рабочая станция", 900, "Техподдержка Astra Linux SE, «Воронеж», рабочая станция")
	astra("s-orel-srv", domain.LicenseKindSupport, "Орел", "Сервер", 1500, "Техподдержка Astra Linux SE, «Орел», сервер")
	astra("s-orel-ws", domain.LicenseKindSupport, "Орел", "Рабочая станция", 1200, "Техподдержка Astra Linux SE, «Орел», рабочая станция")
	f.add(indexDoc{ProductID: "csp-astra", FamilyID: "csp", FamilyName: "Программа КриптоПро CSP 5.0", VendorID: "kripto-pro",
		Vendor: "КРИПТО-ПРО", CategoryID: "sec", Title: "КриптоПро CSP для Astra Linux SE, Лицензия на рабочем месте",
		LicenseKind: domain.LicenseKindBase, PriceRub: ptr(7500.0)})
	f.add(indexDoc{ProductID: "csp-win", FamilyID: "csp", FamilyName: "Программа КриптоПро CSP 5.0", VendorID: "kripto-pro",
		Vendor: "КРИПТО-ПРО", CategoryID: "sec", Title: "КриптоПро CSP 5.0 на одном рабочем месте",
		LicenseKind: domain.LicenseKindBase, PriceRub: ptr(2700.0)})
	f.add(indexDoc{ProductID: "kb-linux", FamilyID: "kb", FamilyName: "Кибер Бэкап Расширенная редакция", VendorID: "cyberprotect",
		Vendor: "Киберпротект", CategoryID: "backup", LicenseOrgType: "commercial", LicenseKind: domain.LicenseKindUpgrade,
		Title: "Кибер Бэкап Расширенная редакция (переходы), для рабочей станции Linux –", PriceRub: ptr(9321.0)})
	f.add(indexDoc{ProductID: "cad-linux", FamilyID: "cad", FamilyName: "САПР 26", VendorID: "cad", Vendor: "САПР-Софт",
		CategoryID: "ecm", Title: "САПР 26 для Linux (подписка)", LicenseKind: domain.LicenseKindBase})
	f.add(indexDoc{ProductID: "astrakhan", FamilyID: "cat", FamilyName: "Каталог цен", VendorID: "aros", Vendor: "АРОС",
		CategoryID: "ecm", Title: "Каталог цен для Астраханской области", LicenseKind: domain.LicenseKindBase})
	f.add(indexDoc{ProductID: "td", FamilyID: "termidesk", FamilyName: "Termidesk", VendorID: "astralinux",
		Vendor: "Группа Астра", CategoryID: "vdi", Title: "Termidesk, лицензия", LicenseKind: domain.LicenseKindBase})
	f.add(indexDoc{ProductID: "ald", FamilyID: "ald", FamilyName: "ALD Pro", VendorID: "astralinux",
		Vendor: "Группа Астра", CategoryID: "os", Title: "ALD Pro, лицензия", LicenseKind: domain.LicenseKindBase})
	return f
}

func TestUpsellLicenseKindChecksEdition(t *testing.T) {
	f := astraFixture()
	r, step := f.run("a-orel-ws", nil)
	cs := r.licenseKindCands()
	if len(cs) != 1 || cs[0].pid != "s-orel-ws" {
		t.Fatalf("техподдержка к «Орлу» для рабочей станции: %+v", cs)
	}
	if cs[0].level != domain.EvidenceB || cs[0].kind != evKindLicense || !strings.HasPrefix(cs[0].reason, "Техподдержка к этой лицензии") {
		t.Fatalf("основание и причина: %+v", cs[0])
	}
	if !strings.Contains(cs[0].reason, "«Орел»") || !strings.Contains(cs[0].reason, "рабочая станция") {
		t.Fatalf("в причине нет редакции и назначения: %q", cs[0].reason)
	}
	reasons := map[string]string{}
	for _, rj := range step.Rejected {
		reasons[rj.ProductID] = rj.Reason
	}
	if reasons["s-vor-ws"] != upRejectEdition || reasons["s-orel-srv"] != upRejectPurpose {
		t.Fatalf("отказы по редакции и назначению: %v", reasons)
	}
}

func TestUpsellLicenseKindTitleEdition(t *testing.T) {
	// Редакция только в названии («Стандартная редакция» / «расширенная Редакция»), семейство одно.
	f := newFx()
	add := func(id, kind, title string) {
		f.add(indexDoc{ProductID: id, FamilyID: "ecm", FamilyName: "ЕСМ ТЕЗИС", VendorID: "h", Vendor: "H",
			CategoryID: "ecm", Title: title, LicenseKind: kind})
	}
	add("base-std", domain.LicenseKindBase, "ЕСМ ТЕЗИС (лицензии), 10 подключений. Стандартная Редакция")
	add("sup-ext", domain.LicenseKindSupport, "ЕСМ ТЕЗИС, техподдержка (расширенная Редакция), 10 подключений")
	add("sup-std", domain.LicenseKindSupport, "ЕСМ ТЕЗИС, техподдержка, Стандартная редакция, 10 подключений")
	r, step := f.run("base-std", nil)
	cs := r.licenseKindCands()
	if len(cs) != 1 || cs[0].pid != "sup-std" {
		t.Fatalf("к «Стандартной» — техподдержка «Стандартной», не «Расширенной»: %+v", cs)
	}
	if len(step.Rejected) != 1 || step.Rejected[0].ProductID != "sup-ext" || step.Rejected[0].Reason != upRejectEdition {
		t.Fatalf("отказ «Расширенной»: %+v", step.Rejected)
	}
}

func TestUpsellCompatDocLevels(t *testing.T) {
	f := astraFixture()
	data := &upsellData{compat: []domain.CompatDoc{
		{ID: "cd-1", ProductFamilies: []string{"Программа КриптоПро CSP 5.0"}, Product: "КриптоПро CSP 5.0 R2",
			ProductVersion: "5.0.12001", PlatformFamilies: []string{"Astra Linux Special Edition"},
			Platform: "Astra Linux SE", PlatformVersion: "1.7", Level: domain.EvidenceA, DocKind: "certificate",
			DocTitle: "Сертификат Ready for Astra", DocNumber: "29073/2025", DocDate: "2025-01-23",
			URL: "https://astra.example/cert.pdf"},
		{ID: "cd-2", ProductFamilies: []string{"Кибер Бэкап Расширенная редакция"},
			PlatformFamilies: []string{"Astra Linux Special Edition"}, Platform: "Astra Linux SE", Level: domain.EvidenceC,
			DocKind: "documentation", DocTitle: "Системные требования", URL: "https://backup.example/req"},
		{ID: "cd-3", ProductFamilies: []string{"Нет такого семейства"}, PlatformFamilies: []string{"Astra Linux Special Edition"},
			Level: domain.EvidenceA, DocKind: "certificate", URL: "https://x.example/"},
	}}
	r, _ := f.run("a-orel-ws", data)
	cs := r.compatCands()
	if len(cs) != 2 {
		t.Fatalf("две записи с семействами каталога: %+v", cs)
	}
	a := cs[0]
	if a.level != domain.EvidenceA || a.pid != "csp-astra" || a.docNo != "29073/2025" || a.evURL != "https://astra.example/cert.pdf" {
		t.Fatalf("A: позиция под платформу, номер и ссылка: %+v", a)
	}
	if a.reason != "Совместимо с Astra Linux SE 1.7 — Сертификат Ready for Astra № 29073/2025" {
		t.Fatalf("причина A без даты: %q", a.reason)
	}
	if strings.Contains(a.reason, "2025-01-23") || strings.Contains(a.reason, "24.07") {
		t.Fatal("дата документа в интерфейсе")
	}
	if cs[1].level != domain.EvidenceC || cs[1].compat != domain.CompatUnknown || !strings.Contains(cs[1].reason, "Проверьте совместимость") {
		t.Fatalf("C: проверьте совместимость: %+v", cs[1])
	}
	// Обратное направление: якорь — продукт, карточка — платформа.
	r2, _ := f.run("csp-win", data)
	back := r2.compatCands()
	if len(back) != 1 || back[0].fid != "astra" || !strings.HasPrefix(back[0].reason, "Платформа для КриптоПро CSP 5.0 R2 5.0.12001: Astra Linux SE 1.7") {
		t.Fatalf("от продукта к платформе: %+v", back)
	}
}

func TestUpsellPlatformVariants(t *testing.T) {
	f := astraFixture()
	r, _ := f.run("a-orel-ws", nil)
	cs := r.platformCands()
	got := map[string]string{}
	var order []string
	for _, c := range cs {
		got[c.fid] = c.reason
		order = append(order, c.fid)
	}
	if got["csp"] != "Вариант для Astra Linux SE" {
		t.Fatalf("КриптоПро — вариант с именем платформы: %v", got)
	}
	if got["kb"] != "Вариант для рабочей станции Linux" {
		t.Fatalf("Кибер Бэкап — вариант для Linux: %v", got)
	}
	if _, ok := got["cat"]; ok {
		t.Fatal("«Астраханской» — не Astra")
	}
	if len(order) != 2 || order[0] != "csp" || order[1] != "kb" {
		t.Fatalf("порядок: имя платформы, затем бэкап; «для Linux» у прикладной программы — нет: %v", order)
	}
	if _, ok := got["termidesk"]; ok {
		t.Fatal("свой вендор — экосистема, не вариант под платформу")
	}
}

func TestUpsellEcosystemAndAssemble(t *testing.T) {
	f := astraFixture()
	data := &upsellData{compat: []domain.CompatDoc{{ID: "cd-1", ProductFamilies: []string{"Программа КриптоПро CSP 5.0"},
		PlatformFamilies: []string{"Astra Linux Special Edition"}, Platform: "Astra Linux SE", PlatformVersion: "1.7",
		Level: domain.EvidenceA, DocKind: "certificate", DocTitle: "Сертификат Ready for Astra", DocNumber: "29073/2025",
		URL: "https://astra.example/cert.pdf"}}}
	r, step := f.run("a-orel-ws", data)
	eco := r.ecosystemCands()
	if len(eco) != 1 || eco[0].fid != "termidesk" || eco[0].reason != "Из экосистемы «Группа Астра»" {
		t.Fatalf("экосистема — другие категории вендора (ALD Pro в той же категории — нет): %+v", eco)
	}
	items := r.assemble()
	byFam := map[string]out.Companion{}
	for _, it := range items {
		byFam[it.FamilyID] = it
		if it.Evidence == nil || it.Reason == "" || it.EvidenceLevel == "" {
			t.Fatalf("у карточки нет причины или основания: %+v", it)
		}
	}
	if byFam["csp"].EvidenceLevel != domain.EvidenceA || byFam["csp"].Evidence.DocNumber != "29073/2025" {
		t.Fatalf("КриптоПро — уровень A (документ сильнее варианта под платформу): %+v", byFam["csp"])
	}
	if byFam["astra"].ProductID != "s-orel-ws" || byFam["kb"].EvidenceLevel != domain.EvidenceB {
		t.Fatalf("техподдержка и Кибер Бэкап: %+v", items)
	}
	if len(items) > upsellMax {
		t.Fatalf("карточек больше %d", upsellMax)
	}
	if len(step.Considered) == 0 {
		t.Fatal("трасса без рассмотренных")
	}
}

func TestUpsellPromos(t *testing.T) {
	f := astraFixture()
	now := time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC)
	data := &upsellData{promos: []domain.Promo{
		{ID: "p-on", Vendor: "astralinux", Families: []string{"Astra Linux Special Edition"}, Text: "Акция: …",
			URL: "https://store.example/specials/a/", ValidFrom: now.AddDate(0, -1, 0), ValidTo: now.AddDate(0, 1, 0)},
		{ID: "p-off", Vendor: "astralinux", Families: []string{"Astra Linux Special Edition"}, Text: "Акция: старая",
			URL: "https://store.example/specials/b/", ValidTo: now.AddDate(0, 0, -1)},
		{ID: "p-offer", Families: []string{"Программа КриптоПро CSP 5.0"}, OfferMatch: []string{"astra"}, Text: "Акция: x",
			URL: "https://store.example/specials/c/", ValidTo: now.AddDate(0, 1, 0)},
		{ID: "p-vendor", Vendor: "other", Families: []string{"Termidesk"}, Text: "Акция: чужой вендор",
			URL: "https://store.example/specials/d/", ValidTo: now.AddDate(0, 1, 0)},
	}}
	ix := buildUpsellIndex(f.lx, data)
	if ps := ix.promosFor("astra", []string{"x"}, now); len(ps) != 1 || ps[0].ID != "p-on" {
		t.Fatalf("истёкшая акция не показывается: %+v", ps)
	}
	if ps := ix.promosFor("csp", []string{"КриптоПро CSP 5.0"}, now); len(ps) != 0 {
		t.Fatalf("offer_match не совпал — акции нет: %+v", ps)
	}
	if ps := ix.promosFor("csp", []string{"КриптоПро CSP для Astra Linux SE"}, now); len(ps) != 1 {
		t.Fatalf("offer_match совпал: %+v", ps)
	}
	if ps := ix.promosFor("termidesk", []string{"Termidesk"}, now); len(ps) != 0 {
		t.Fatalf("вендор акции не совпал — не показывается: %+v", ps)
	}
}

func TestUpsellDiscountFromVariants(t *testing.T) {
	resp := &out.SearchResponse{Items: []out.SearchItem{{ProductID: "head", PriceRub: ptr(104300.0), Available: ptr(true),
		MatchesQuery: true, PriceFrom: ptr(3100.0), Variants: []out.SearchItem{
			{ProductID: "orel", PriceRub: ptr(3100.0), OldPriceRub: ptr(4200.0), Available: ptr(true), MatchesQuery: true}}}}}
	discountFromVariants(resp)
	if len(resp.Items[0].Badges) != 1 || resp.Items[0].Badges[0] != "Скидка 26%" {
		t.Fatalf("бейдж по позиции цены «от»: %v", resp.Items[0].Badges)
	}
	if got := anchorOffer(resp.Items[0], "астра линукс"); got != "orel" {
		t.Fatalf("позиция якоря — цена «от»: %s", got)
	}
}

func TestUpsellAnchorNeedsNamedTarget(t *testing.T) {
	items := []out.SearchItem{{MatchType: out.MatchRelated}, {MatchType: out.MatchExact}}
	if i, c, _ := upsellAnchor(&out.SearchResponse{Target: &out.Target{Type: out.TargetFamily}, Items: items}, false); i != 1 || c != "high" {
		t.Fatalf("семейство: якорь — первая точная карточка, high: %d %s", i, c)
	}
	if i, _, _ := upsellAnchor(&out.SearchResponse{Target: &out.Target{Type: out.TargetCategory}, Items: items}, false); i != -1 {
		t.Fatal("категория — без якоря")
	}
	if i, _, _ := upsellAnchor(&out.SearchResponse{Target: &out.Target{Type: out.TargetFamily}, Items: items}, true); i != -1 {
		t.Fatal("аналог — без якоря")
	}
}

func TestForPhraseWords(t *testing.T) {
	terms := []platformTerm{{"astra linux", 2}, {"linux", 1}}
	cases := []struct {
		title, phrase string
		w             int
	}{
		{"КриптоПро CSP для Astra Linux SE, Лицензия", "Astra Linux SE", 2},
		{"Кибер Бэкап (переходы), для рабочей станции Linux –", "рабочей станции Linux", 1},
		{"Каталог для Астраханской области", "", 0},
		{"Модуль для Linuxoid", "", 0},
	}
	for _, c := range cases {
		low := strings.ToLower(c.title)
		if p, w := forPhrase(c.title, low, terms); p != c.phrase || w != c.w {
			t.Errorf("%q → %q/%d, ждали %q/%d", c.title, p, w, c.phrase, c.w)
		}
	}
}

func TestUpsellLicenseKindVersionAndPlatform(t *testing.T) {
	f := newFx()
	add := func(id, kind, title string) {
		f.add(indexDoc{ProductID: id, FamilyID: "csp", FamilyName: "Программа КриптоПро CSP 5.0", VendorID: "kp",
			Vendor: "КРИПТО-ПРО", CategoryID: "sec", Title: title, LicenseKind: kind})
	}
	add("base5", domain.LicenseKindBase, "КриптоПро CSP версии 5.0 (Номер ЕРРП: 4332), Лицензия на рабочем месте")
	add("sup-astra", domain.LicenseKindSupport, "КриптоПро CSP для Astra Linux SE, Сертификат на техподдержку версии 5.0")
	add("mod4", domain.LicenseKindModule, "КриптоПро CSP версии 4.0 Lic, Лицензия на право использования")
	add("sup5", domain.LicenseKindSupport, "КриптоПро CSP, Сертификат на годовую техподдержку СКЗИ версии 5.0")
	r, step := f.run("base5", nil)
	cs := r.licenseKindCands()
	if len(cs) != 1 || cs[0].pid != "sup5" {
		t.Fatalf("к CSP 5.0 — техподдержка 5.0 без платформы: %+v", cs)
	}
	reasons := map[string]string{}
	for _, rj := range step.Rejected {
		reasons[rj.ProductID] = rj.Reason
	}
	if reasons["sup-astra"] != upRejectPlatform || reasons["mod4"] != upRejectVersion {
		t.Fatalf("отказы по платформе и версии: %v", reasons)
	}
}
