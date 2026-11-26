package services

import (
	"encoding/json"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// на маленьком каталоге в форме фида: «Можно купить сейчас» по классу и назначению, продукт только под тип
// организации, главный продукт линейки (не CAL, ближайшая версия), строка сравнения цены за место, основная
// линейка вендора.

type p63offer struct {
	id, fam, famName, vendor, cat, title, kind, org, licType, purpose string
	price                                                             float64 // 0 — по запросу
	tierMin, tierMax, months                                          int
	metric                                                            string
}

func p63catalog() *searchLexicon {
	osRoot, avRoot := "os", "sec"
	cats := []domain.Category{
		{ID: osRoot, Name: "Операционные системы"},
		{ID: "win", Name: "Операционные системы Windows", ParentID: &osRoot},
		{ID: "ru", Name: "Российские операционные системы (Импортозамещение)", ParentID: &osRoot},
		{ID: "tools", Name: "Утилиты для администрирования", ParentID: &osRoot},
		{ID: avRoot, Name: "Безопасность"},
		{ID: "av", Name: "Антивирусы для организаций", ParentID: &avRoot},
		{ID: "prot", Name: "Программы для защиты информации", ParentID: &avRoot},
	}
	vendors := []domain.Vendor{{ID: "microsoft", Name: "Microsoft"}, {ID: "astralinux", Name: "Группа Астра"},
		{ID: "basealt", Name: "Базальт СПО"}, {ID: "redsoft", Name: "РЕД СОФТ"}, {ID: "kaspersky", Name: "Kaspersky"}}
	b, r, sup := domain.LicenseKindBase, domain.LicenseKindRenewal, domain.LicenseKindSupport
	offers := []p63offer{
		// Windows Server: всё по запросу; Standard — только для госучреждений.
		{id: "w1", fam: "f-std", famName: "Операционная система Microsoft Windows Server Standard", vendor: "microsoft", cat: "win", title: "Microsoft Windows Server Standard 2019 (для государственных организаций)", kind: b, org: "government"},
		{id: "w2", fam: "f-dc", famName: "Microsoft Windows Server Datacenter 2022", vendor: "microsoft", cat: "win", title: "Microsoft Windows Server Datacenter 2022", kind: b},
		{id: "w3", fam: "f-cal", famName: "Microsoft Windows Server CAL", vendor: "microsoft", cat: "win", title: "Microsoft Windows Server CAL, 1 User", kind: b},
		{id: "w4", fam: "f-cal22", famName: "Microsoft Windows Server CAL 2022", vendor: "microsoft", cat: "win", title: "Microsoft Windows Server CAL 2022", kind: b},
		// Российские ОС: у Astra SE обе редакции (по параметру «Назначение»), Альт Сервер — редакция в имени.
		{id: "a1", fam: "f-se", famName: "Astra Linux Special Edition", vendor: "astralinux", cat: "ru", title: "Astra Linux SE, Эльбрус", kind: b, price: 287300, purpose: "Сервер"},
		{id: "a2", fam: "f-se", famName: "Astra Linux Special Edition", vendor: "astralinux", cat: "ru", title: "Astra Linux SE, «Орел», серверная", kind: b, price: 20000, purpose: "Сервер, Виртуальный сервер"},
		{id: "a3", fam: "f-se", famName: "Astra Linux Special Edition", vendor: "astralinux", cat: "ru", title: "Astra Linux SE, «Орел»", kind: b, price: 9000, purpose: "Рабочая станция"},
		{id: "a4", fam: "f-se", famName: "Astra Linux Special Edition", vendor: "astralinux", cat: "ru", title: "Сертификат техподдержки Astra Linux SE", kind: sup, price: 100, purpose: "Сервер"},
		// ALD Pro: «серверные лицензии» программного комплекса без редакции для рабочих станций — не серверная ОС.
		{id: "d1", fam: "f-ald", famName: "ALD Pro", vendor: "astralinux", cat: "ru", title: "Программный комплекс ALD Pro, серверные лицензии", kind: b, price: 8500, purpose: "Сервер"},
		{id: "l1", fam: "f-alt", famName: "Альт Сервер 11", vendor: "basealt", cat: "ru", title: "BaseALT Альт Сервер 11 (бессрочная лицензия)", kind: b, price: 16680},
		{id: "r1", fam: "f-red", famName: "Операционная система РЕД ОС", vendor: "redsoft", cat: "ru", title: "РЕД ОС, Конфигурация Сервер", kind: b, price: 15000},
		{id: "r2", fam: "f-red", famName: "Операционная система РЕД ОС", vendor: "redsoft", cat: "ru", title: "РЕД ОС, Конфигурация Рабочая станция", kind: b, price: 4500},
		{id: "r3", fam: "f-virt", famName: "РЕД Виртуализация", vendor: "redsoft", cat: "ru", title: "РЕД Виртуализация, на 1 сервер", kind: b, price: 700000},
		// Другая категория того же раздела без общего слова-типа — не тот класс.
		{id: "t1", fam: "f-tool", famName: "Утилита Сервер Плюс", vendor: "redsoft", cat: "tools", title: "Утилита Сервер Плюс", kind: b, price: 500},
		// Kaspersky: продление на 2 года, ступень 100–149, за узел.
		{id: "k1", fam: "f-kstd", famName: "Kaspersky Endpoint Security для бизнеса Стандартный", vendor: "kaspersky", cat: "av", title: "KES Стандартный продление на 2 года", kind: r, licType: domain.LicenseRenewal, price: 3070.80, tierMin: 100, tierMax: 149, months: 24, metric: domain.MetricDevice},
		{id: "k1s", fam: "f-kstd", famName: "Kaspersky Endpoint Security для бизнеса Стандартный", vendor: "kaspersky", cat: "av", title: "KES Стандартный продление на 2 года", kind: r, licType: domain.LicenseRenewal, price: 4000, tierMin: 10, tierMax: 14, months: 24, metric: domain.MetricDevice},
		{id: "k2", fam: "f-kext", famName: "Kaspersky Endpoint Security для бизнеса Расширенный", vendor: "kaspersky", cat: "av", title: "KES Расширенный продление на 2 года", kind: r, licType: domain.LicenseRenewal, price: 4451.94, tierMin: 100, tierMax: 149, months: 24, metric: domain.MetricDevice},
		{id: "k3", fam: "f-kmail", famName: "Kaspersky Security для почтовых серверов", vendor: "kaspersky", cat: "av", title: "Kaspersky Security для почтовых серверов продление", kind: r, licType: domain.LicenseRenewal, price: 1814.40, tierMin: 100, tierMax: 149, months: 24, metric: domain.MetricDevice},
		{id: "k4", fam: "f-ktot", famName: "Kaspersky Total Security для бизнеса", vendor: "kaspersky", cat: "av", title: "Kaspersky Total Security для бизнеса продление", kind: r, licType: domain.LicenseRenewal, price: 7965, tierMin: 100, tierMax: 149, months: 24, metric: domain.MetricDevice},
		{id: "k5", fam: "f-ksym", famName: "Kaspersky Symphony Security", vendor: "kaspersky", cat: "prot", title: "Kaspersky Symphony Security продление", kind: r, licType: domain.LicenseRenewal, price: 6365.70, tierMin: 100, tierMax: 149, months: 24, metric: domain.MetricDevice},
	}
	s := &domain.CatalogSnapshot{Version: "p63-test", Categories: cats, Vendors: vendors}
	for _, o := range offers {
		p := domain.Product{ID: o.id, FamilyID: o.fam, FamilyName: o.famName, Title: o.title, VendorID: o.vendor,
			CategoryID: o.cat, Popularity: 0.5, Available: p6bool(o.price > 0),
			License: domain.License{Type: o.licType, Kind: o.kind, OrgType: o.org}}
		if p.License.Type == "" {
			p.License.Type = domain.LicensePerpetual
		}
		if p.License.OrgType == "" {
			p.License.OrgType = "commercial"
		}
		if o.price > 0 {
			p.PriceRub = p6price(o.price)
		} else {
			p.PriceOnRequest = true
		}
		if o.tierMin > 0 {
			mn, mx := o.tierMin, o.tierMax
			p.License.TierMin, p.License.TierMax, p.License.Quantity = &mn, &mx, &mn
		}
		if o.months > 0 {
			m := o.months
			p.License.DurationMonths = &m
		}
		if o.metric != "" {
			m := o.metric
			p.License.Metric = &m
		}
		if o.purpose != "" {
			raw, _ := json.Marshal(map[string]string{"Назначение": o.purpose})
			p.Attributes = raw
		}
		s.Products = append(s.Products, p)
	}
	return newSearchLexicon(s)
}

func TestP63EditionsOfText(t *testing.T) {
	cases := []struct {
		text       string
		server, ws bool
	}{
		{"BaseALT Альт Сервер 11", true, false},
		{"РЕД ОС, Конфигурация Рабочая станция", false, true},
		{"РЕД Виртуализация, на 1 сервер", false, false},
		{"РЕД АДМ на базе РЕД ОС Сервер", false, false},
		{"по количеству рабочих станций/файловых серверов", false, false},
		{"Kaspersky Security для почтовых серверов", false, false},
	}
	for _, c := range cases {
		if s, w := editionsOfText(c.text); s != c.server || w != c.ws {
			t.Errorf("%q: сервер %v, рабочая станция %v; нужно %v, %v", c.text, s, w, c.server, c.ws)
		}
	}
}

func TestP63OrgOnlyFamilyShownWhenNamed(t *testing.T) {
	lx := p63catalog()
	rc := p6rc(lx)
	var rej []domain.RejectedHit
	if fh := rc.buildFamily("f-std", 1, &rej); fh != nil {
		t.Fatal("не названное семейство только для госучреждений не показывается без типа организации")
	}
	rc.ent.families = []string{"f-std"}
	fh := rc.buildFamily("f-std", 1, &rej)
	if fh == nil || fh.head.ProductID != "w1" {
		t.Fatal("названный продукт, который продаётся только госучреждениям, должен остаться")
	}
	if note := rc.orgOnlyNote(fh); !strings.Contains(note, "госучреждение") {
		t.Fatalf("оговорка: %q", note)
	}
}

func p63windows(t *testing.T, rc rankCtx, query string) (*out.SearchResponse, AnswerStep) {
	t.Helper()
	rc.ent.vendors = []string{"microsoft"}
	rc.ent.families = []string{"f-std", "f-dc", "f-cal", "f-cal22"}
	fams := p6fams(t, rc, domain.GroupExact, "f-cal", "f-cal22", "f-dc", "f-std")
	resp := p6resp(rc, fams)
	resp.Group = "family"
	step := answerStage(answerInput{rc: rc, words: queryWords(strings.Fields(foldText(query)), nil), fams: fams,
		storefront: true}, resp, &domain.Trace{})
	return resp, step
}

func TestP63AlternativesSameClassAndPurpose(t *testing.T) {
	lx := p63catalog()
	resp, step := p63windows(t, p6rc(lx), "windows server")
	if resp.Items[0].FamilyID != "f-std" {
		t.Fatalf("первым — Windows Server Standard, а не %s (CAL — допродажа)", resp.Items[0].FamilyID)
	}
	if resp.Items[0].PriceState != out.PriceOnRequest || !strings.Contains(resp.Items[0].Explain, "госучреждение") {
		t.Fatalf("первая карточка: %s, %q", resp.Items[0].PriceState, resp.Items[0].Explain)
	}
	if step.Alternatives != 3 {
		t.Fatalf("альтернатив %d, нужно 3", step.Alternatives)
	}
	got := map[string]out.SearchItem{}
	for i, it := range resp.Items {
		if it.MatchType != out.MatchAlternative {
			continue
		}
		if i > 3 {
			t.Fatalf("альтернатива %s на месте %d — блок должен идти сразу под названным продуктом", it.FamilyID, i)
		}
		if it.PriceState != out.PricePriced || !strings.Contains(it.Explain, "серверная редакция") ||
			strings.Contains(strings.ToLower(it.Explain), "аналог") {
			t.Fatalf("альтернатива %s: %s, %q", it.FamilyID, it.PriceState, it.Explain)
		}
		got[it.FamilyID] = it
	}
	for _, want := range []string{"f-se", "f-alt", "f-red"} {
		if _, ok := got[want]; !ok {
			t.Fatalf("нет серверной ОС %s среди альтернатив: %v", want, got)
		}
	}
	if got["f-se"].ProductID != "a2" {
		t.Fatalf("Astra SE — самая доступная серверная базовая лицензия a2, а не %s", got["f-se"].ProductID)
	}
	if got["f-red"].ProductID != "r1" {
		t.Fatalf("РЕД ОС — конфигурация Сервер r1, а не %s", got["f-red"].ProductID)
	}
}

func TestP63MissingVersionLeadsWithNearestProduct(t *testing.T) {
	lx := p63catalog()
	resp, _ := p63windows(t, p6rc(lx), "windows server 2025")
	if resp.Items[0].FamilyID != "f-dc" {
		t.Fatalf("версии 2025 нет: первым ближайшая версия продукта (Datacenter 2022), а не %s", resp.Items[0].FamilyID)
	}
	if !strings.Contains(resp.Items[0].Explain, "ближайшая") {
		t.Fatalf("оговорка о версии: %q", resp.Items[0].Explain)
	}
}

func p63kaspersky(t *testing.T) (rankCtx, []*familyHit) {
	t.Helper()
	rc := p6rc(p63catalog(), "kaspersky")
	rc.attrs.renewal, rc.attrs.quantity, rc.attrs.duration = true, 100, 24
	rc.wantKinds = map[string]bool{domain.LicenseKindRenewal: true}
	fams := p6fams(t, rc, domain.GroupExact, "f-ksym", "f-kmail", "f-ktot", "f-kext", "f-kstd")
	return rc, fams
}

func TestP63VendorCoreLineFirst(t *testing.T) {
	rc, fams := p63kaspersky(t)
	rc.markVendorCore(fams)
	sortFamilies(fams)
	var order []string
	for _, fh := range fams {
		order = append(order, fh.id)
	}
	if order[0] != "f-kext" && order[0] != "f-kstd" || order[1] != "f-kext" && order[1] != "f-kstd" {
		t.Fatalf("первыми — редакции линейки KES: %v", order)
	}
	if order[2] != "f-ktot" || order[len(order)-1] != "f-ksym" {
		t.Fatalf("затем общий продукт основной категории, специализированный и другая категория — ниже: %v", order)
	}
	for _, fh := range fams {
		if fh.head.LicenseTierMin == nil || *fh.head.LicenseTierMin != 100 {
			t.Fatalf("%s: голова не со ступенью 100–149", fh.id)
		}
	}
}

func TestP63PriceComparisonOnePairSameConditions(t *testing.T) {
	rc, fams := p63kaspersky(t)
	var items []out.SearchItem
	for _, fh := range fams {
		it := rc.toItem(fh)
		it.MatchType = out.MatchExact
		items = append(items, it)
	}
	c := priceComparison(rc.lx, items)
	if c == nil {
		t.Fatal("нет строки сравнения")
	}
	if c.Text != "При этих условиях Стандартный на 31% дешевле, чем Расширенный, за узел" {
		t.Fatalf("текст: %q", c.Text)
	}
	if !strings.Contains(c.Conditions, "Продление") || !strings.Contains(c.Conditions, "2 года") || !strings.Contains(c.Conditions, "100–149") {
		t.Fatalf("условия: %q", c.Conditions)
	}
	for _, bad := range []string{"лучше", "выгодн", "2026"} {
		if strings.Contains(c.Text, bad) {
			t.Fatalf("в тексте %q", bad)
		}
	}
	// Без пары одной линейки (только почтовые серверы и Total) — сравнения нет.
	var other []out.SearchItem
	for _, it := range items {
		if it.FamilyID == "f-kmail" || it.FamilyID == "f-ktot" {
			other = append(other, it)
		}
	}
	if c := priceComparison(rc.lx, other); c != nil {
		t.Fatalf("сравнение разных продуктов: %q", c.Text)
	}
}

func TestP63MergeSameTitle(t *testing.T) {
	items := []out.SearchItem{
		{ProductID: "1", FamilyID: "offer-1", Vendor: "DEPO", Title: "Rack-сервер DEPO Storm 3470E2A"},
		{ProductID: "2", FamilyID: "offer-2", Vendor: "DEPO", Title: "Rack-сервер DEPO Storm 3470U1A"},
		{ProductID: "3", FamilyID: "offer-3", Vendor: "DEPO", Title: "Rack-сервер DEPO Storm 3470E2A"},
	}
	got := mergeSameTitle(items)
	if len(got) != 2 || len(got[0].Variants) != 1 || got[0].Variants[0].ProductID != "3" {
		t.Fatalf("одинаковые модели — одна карточка с вариантом: %+v", got)
	}
}

func TestP63YearIsNotQuantity(t *testing.T) {
	a := extractAttrs([]string{"windows", "server", "2022", "лицензия"})
	if a.quantity != 0 {
		t.Fatalf("2022 в «windows server 2022 лицензия» — год, а не %d мест", a.quantity)
	}
	if a := extractAttrs([]string{"на", "2000", "пользователей"}); a.quantity != 2000 {
		t.Fatalf("«на 2000 пользователей» — количество, получено %d", a.quantity)
	}
	if lic, _, _ := licenseFromText("Microsoft Windows Server Datacenter 2022 (лицензия Single OLV 2 License)"); lic.Quantity != nil && *lic.Quantity == 2022 {
		t.Fatal("в названии позиции год стал количеством")
	}
}

func TestP63GibberishHasNoServiceTarget(t *testing.T) {
	e := queryEntities{nonProduct: true, significant: 1, unknown: []string{"asdfgh"}}
	if !gibberish(e, []string{"asdfgh"}) {
		t.Fatal("asdfgh — бессмыслица")
	}
	if gibberish(queryEntities{nonProduct: true, significant: 1, unknown: []string{"упд"}}, []string{"упд"}) {
		t.Fatal("«упд» — сервисный вопрос, а не бессмыслица")
	}
	var e2 queryEntities
	e2.markHowTo([]string{"как", "продлить", "лицензию"}, queryAttrs{})
	if !e2.nonProduct {
		t.Fatal("«как продлить лицензию» без названного продукта — сервисный вопрос")
	}
	e3 := queryEntities{vendors: []string{"kaspersky"}}
	e3.markHowTo([]string{"как", "продлить", "касперский"}, queryAttrs{})
	if e3.nonProduct {
		t.Fatal("«как продлить касперский» — о товаре")
	}
}

func TestP63SKUNeighbours(t *testing.T) {
	rc, fams := p63kaspersky(t)
	var sku indexDoc
	var items []out.SearchItem
	for _, fh := range fams {
		for _, d := range fh.heads {
			items = append(items, rc.toItem(&familyHit{id: fh.id, head: d, heads: []indexDoc{d}, fits: map[string]bool{d.ProductID: true}}))
			if d.ProductID == "k1" {
				sku = d
			}
		}
	}
	// Ещё четыре позиции семейства SKU: соседи по ступени.
	for i, tier := range []int{150, 250, 500, 1000} {
		d := sku
		d.ProductID = "k1-" + strings.Repeat("x", i+1)
		mn := tier
		d.LicenseTierMin = &mn
		rc.lx.docs[d.ProductID] = d
		items = append(items, out.SearchItem{ProductID: d.ProductID, FamilyID: d.FamilyID, Title: d.Title})
	}
	got := trimSKUFamily(rc.lx, items, sku)
	same := 0
	for _, it := range got {
		if it.FamilyID == sku.FamilyID {
			same++
		}
	}
	if got[0].ProductID != "k1" && same != 4 {
		t.Fatalf("SKU и три соседа отдельными карточками: первая %s, карточек семейства %d", got[0].ProductID, same)
	}
	if same != 4 {
		t.Fatalf("карточек семейства SKU %d, нужно 4", same)
	}
}
