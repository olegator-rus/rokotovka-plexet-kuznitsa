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
	cs := r.licenseKindCands(nil, "")
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
	cs := r.licenseKindCands(nil, "")
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
	cs := r.licenseKindCands(nil, "")
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

// classFixture — Astra (ОС) и Microsoft 365 (офис) с классами: антивирус (Dr.Web с акцией, KES с документом),
// СКЗИ (КриптоПро с вариантом под Astra), бэкап (два семейства Киберпротекта и Uranium), антивирус без цены.
func classFixture() (*fx, *upsellData) {
	f := astraFixture()
	add := func(id, fam, famName, vendor, cat, title string, price float64) {
		f.add(indexDoc{ProductID: id, FamilyID: fam, FamilyName: famName, VendorID: vendor, Vendor: vendor,
			CategoryID: cat, Title: title, LicenseKind: domain.LicenseKindBase, PriceRub: ptr(price)})
	}
	f.lx.categoryName["av"] = "Антивирусы для организаций"
	f.lx.categoryName["office"] = "Офисные пакеты"
	add("kes", "kesb", "Kaspersky Endpoint Security для бизнеса Стандартный", "kaspersky", "av", "KES Стандартный, 1 год", 2281)
	add("ks365", "ks365", "Kaspersky Security для Microsoft Office 365", "kaspersky", "av", "Kaspersky Security для Microsoft Office 365, 1 год", 900)
	add("drw", "drweb", "Dr.Web Desktop Security Suite", "drweb", "av", "Dr.Web Desktop Security Suite, 1 год", 1000)
	add("drw2", "drweb", "Dr.Web Desktop Security Suite", "drweb", "av", "Dr.Web Desktop Security Suite, 2 года", 1800)
	f.add(indexDoc{ProductID: "nop", FamilyID: "noprice", FamilyName: "Антивирус по запросу", VendorID: "x", Vendor: "x",
		CategoryID: "av", Title: "Антивирус по запросу", PriceOnRequest: true, PriceRub: ptr(0.0)})
	add("kbs", "kbs", "Кибер Бэкап Старт", "cyberprotect", "backup", "Кибер Бэкап Старт, подписка", 1370)
	add("ura", "ura", "Uranium Backup", "uranium", "backup", "Uranium Backup Pro", 5000)
	add("m365", "m365", "Microsoft 365 Family (для семьи)", "microsoft", "office", "Microsoft 365 Family", 11250)
	data := &upsellData{
		compat: []domain.CompatDoc{
			{ID: "cd-kes", ProductFamilies: []string{"Kaspersky Endpoint Security для бизнеса Стандартный"},
				PlatformFamilies: []string{"Astra Linux Special Edition"}, PlatformLabel: "Astra Linux SE 1.5–1.8",
				Level: domain.EvidenceA, DocKind: "documentation", DocLabel: "документация KES 12.2 для Linux", URL: "https://kes.example/"},
			{ID: "cd-csp", ProductFamilies: []string{"Программа КриптоПро CSP 5.0"}, PlatformFamilies: []string{"Astra Linux Special Edition"},
				PlatformLabel: "Astra Linux SE 1.7", Level: domain.EvidenceA, DocKind: "certificate",
				DocLabel: "сертификат Ready for Astra", DocNumber: "29073/2025", URL: "https://astra.example/cert.pdf"},
		},
		promos: []domain.Promo{{ID: "p-drweb", Vendor: "drweb", Families: []string{"Dr.Web Desktop Security Suite"},
			Text: "Акция: −50%", URL: "https://store.example/specials/drweb/", ValidTo: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)}},
		classes: []domain.UpsellClass{
			{ID: "os", Title: "ОС", Categories: []string{"os"}, Include: "linux"},
			{ID: "office", Title: "Офис", Categories: []string{"office"}},
			{ID: "antivirus", Title: "Антивирус", Categories: []string{"av"}},
			{ID: "backup", Title: "Бэкап", Categories: []string{"backup"}},
			{ID: "crypto", Title: "СКЗИ", Categories: []string{"sec"}, Include: "csp"},
		},
		links: []domain.UpsellLink{
			{From: "os", To: domain.UpsellSelf, Kinds: []string{"support"}, Max: 1, Reason: "Техподдержка к этой лицензии", Source: "кикофф"},
			{From: "os", To: "antivirus", Max: 1, Reason: "Защита рабочих мест", Source: "кикофф"},
			{From: "os", To: "crypto", Max: 1, Reason: "Криптозащита", Source: "кикофф"},
			{From: "os", To: "backup", Max: 1, Reason: "Резервное копирование инфраструктуры", Source: "кикофф"},
			{From: "office", To: "antivirus", Max: 1, Reason: "Офису нужна защита", Source: "кикофф"},
			{From: "office", To: "backup", Max: 1, Reason: "Офису нужна защита: резервное копирование", Source: "кикофф"},
		},
	}
	return f, data
}

func (f *fx) runAt(anchorPID string, data *upsellData, now time.Time) (*upsellRun, *UpsellStep) {
	r, step := f.run(anchorPID, data)
	r.now = now
	return r, step
}

func TestUpsellByClassAstra(t *testing.T) {
	f, data := classFixture()
	r, step := f.runAt("a-orel-ws", data, time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC))
	if cls := r.ix.classOf("astra"); cls != "os" {
		t.Fatalf("класс Astra — os: %q", cls)
	}
	items := r.assemble()
	if step.Class != "os" || len(items) != 4 {
		t.Fatalf("Astra → техподдержка, антивирус, СКЗИ, бэкап: %+v", items)
	}
	want := []struct{ fam, level string }{{"astra", "B"}, {"kesb", "A"}, {"csp", "A"}, {"kb", "B"}}
	for i, w := range want {
		if items[i].FamilyID != w.fam || items[i].EvidenceLevel != w.level || items[i].Reason == "" {
			t.Fatalf("карточка %d: ждали %s/%s, есть %s/%s (%s)", i, w.fam, w.level, items[i].FamilyID, items[i].EvidenceLevel, items[i].Reason)
		}
	}
	// Антивирус: вендор с документом (KES) выше Dr.Web с акцией без документа; причина — из таблицы + документ.
	if !strings.HasPrefix(items[1].Reason, "Защита рабочих мест. Совместимо с Astra Linux SE 1.5–1.8") ||
		items[1].Evidence.Kind != evKindCompat {
		t.Fatalf("антивирус с документом: %+v", items[1])
	}
	// Бэкап без документа — уровень B, вид «логика класса».
	if items[3].Evidence.Kind != evKindClass || !strings.HasPrefix(items[3].Reason, "Резервное копирование") {
		t.Fatalf("бэкап по логике класса: %+v", items[3])
	}
	// СКЗИ: документ A и позиция под Astra.
	if items[2].ProductID != "csp-astra" || items[2].Evidence.DocNumber != "29073/2025" ||
		!strings.Contains(items[2].Reason, "Совместимо с Astra Linux SE 1.7") {
		t.Fatalf("СКЗИ с документом: %+v", items[2])
	}
	vendors := map[string]bool{}
	for _, it := range items {
		if it.PriceRub == nil {
			t.Fatalf("карточка без цены: %+v", it)
		}
		if it.FamilyID != "astra" {
			if vendors[it.Vendor] {
				t.Fatalf("два семейства одного вендора: %s", it.Vendor)
			}
			vendors[it.Vendor] = true
		}
	}
}

func TestUpsellByClassOfficeNamesAnchor(t *testing.T) {
	f, data := classFixture()
	r, _ := f.runAt("m365", data, time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC))
	items := r.assemble()
	if len(items) != 2 || items[0].FamilyID != "ks365" {
		t.Fatalf("Microsoft 365 → Kaspersky Security для Microsoft Office 365 первым: %+v", items)
	}
	if !strings.Contains(items[0].Reason, "Вариант для Microsoft Office 365") || items[0].EvidenceLevel != domain.EvidenceB {
		t.Fatalf("причина и уровень: %+v", items[0])
	}
	if items[1].FamilyID != "kb" && items[1].FamilyID != "kbs" && items[1].FamilyID != "ura" {
		t.Fatalf("второе — бэкап: %+v", items[1])
	}
}

func TestUpsellClassPromoLiftsAndExpires(t *testing.T) {
	f, data := classFixture()
	// Бэкап: документов нет ни у кого; акция Uranium поднимает его, после срока — порядок без неё.
	data.promos = append(data.promos, domain.Promo{ID: "p-ura", Families: []string{"Uranium Backup"}, Text: "Акция",
		URL: "https://store.example/specials/u/", ValidTo: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)})
	r, _ := f.runAt("a-orel-ws", data, time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC))
	cs := r.classCands("os", data.links[3])
	if len(cs) < 2 || cs[0].fid != "ura" {
		t.Fatalf("действующая акция поднимает карточку: %+v", cs)
	}
	r2, _ := f.runAt("a-orel-ws", data, time.Date(2027, 1, 15, 12, 0, 0, 0, time.UTC))
	cs = r2.classCands("os", data.links[3])
	if len(cs) < 2 || cs[0].fid == "ura" {
		t.Fatalf("истёкшая акция не поднимает: %+v", cs)
	}
	// Антивирус: вендор с документом (KES) выше Dr.Web с акцией без документа; семейства без цены нет.
	cs = r2.classCands("os", data.links[1])
	for _, c := range cs {
		if c.fid == "noprice" {
			t.Fatal("семейство без цены в кандидатах")
		}
	}
	if len(cs) == 0 || cs[0].fid != "kesb" {
		t.Fatalf("KES с документом первым: %+v", cs)
	}
}

func TestAnchorNameTerms(t *testing.T) {
	if got := anchorNameTerms("Microsoft 365 Family (для семьи)"); len(got) != 1 || got[0] != "365" {
		t.Fatalf("числа имени: %v", got)
	}
	if got := anchorNameTerms("Astra Linux Special Edition"); len(got) != 1 || got[0] != "astra linux" {
		t.Fatalf("два слова: %v", got)
	}
	if namesAnchor("Kaspersky Security для Microsoft Office 365", []string{"2024"}) != "" {
		t.Fatal("Office 365 — не Office 2024")
	}
}

func TestUpsellClassTieBreakByDocOrder(t *testing.T) {
	f, data := classFixture()
	// Оба с документом A и оба с акцией — решает порядок записей compat_docs (KES раньше Dr.Web), а не ширина линейки.
	data.compat = append(data.compat, domain.CompatDoc{ID: "cd-drw", ProductFamilies: []string{"Dr.Web Desktop Security Suite"},
		PlatformFamilies: []string{"Astra Linux Special Edition"}, Level: domain.EvidenceA, DocKind: "documentation",
		URL: "https://drweb.example/"})
	data.promos = append(data.promos, domain.Promo{ID: "p-kes", Families: []string{"Kaspersky Endpoint Security для бизнеса Стандартный"},
		Text: "Акция", URL: "https://store.example/specials/k/", ValidTo: time.Date(2026, 7, 2, 0, 0, 0, 0, time.UTC)})
	r, _ := f.runAt("a-orel-ws", data, time.Date(2026, 5, 28, 12, 0, 0, 0, time.UTC))
	cs := r.classCands("os", data.links[1])
	if len(cs) < 2 || cs[0].fid != "kesb" || cs[1].fid != "drweb" {
		t.Fatalf("ничья по ярусу и акции — порядок документов: %+v", cs)
	}
}
