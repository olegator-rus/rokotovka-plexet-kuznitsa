package services

import (
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Маленький каталог в форме фида: цель ответа, голова семейства и лестница проверяются на нём, а не на
// эталонах заказчика.

func p6price(v float64) *float64 { return &v }

func p6bool(v bool) *bool { return &v }

type p6offer struct {
	id, fam, famName, vendor, cat, title, kind, org, sku, licType string
	price                                                         float64 // 0 — по запросу
	old                                                           float64
	pop                                                           float64
	famURL                                                        string
}

func p6snapshot(offers []p6offer) *domain.CatalogSnapshot {
	root := "sw"
	s := &domain.CatalogSnapshot{Version: "p6-test",
		Categories: []domain.Category{
			{ID: root, Name: "Программы"},
			{ID: "backup", Name: "Резервное копирование", ParentID: &root},
			{ID: "os", Name: "Операционные системы", ParentID: &root},
			{ID: "comm", Name: "Связь", ParentID: &root},
			{ID: "srv", Name: "Серверы", ParentID: &root},
			{ID: "av", Name: "Антивирусы", ParentID: &root},
		},
		Vendors: []domain.Vendor{
			{ID: "cyberprotect", Name: "Киберпротект", URL: "https://store.example/cyberprotect/", Slug: "cyberprotect"},
			{ID: "astralinux", Name: "Группа Астра", URL: "https://store.example/astralinux/", Slug: "astralinux"},
			{ID: "webinar", Name: "МТС Линк", URL: "https://store.example/webinar/", Slug: "webinar"},
			{ID: "kaspersky", Name: "Kaspersky", URL: "https://store.example/kaspersky/", Slug: "kaspersky"},
			{ID: "inferit", Name: "INFERIT", URL: "https://store.example/inferit/", Slug: "inferit"},
			{ID: "depo", Name: "DEPO", URL: "https://store.example/depo/", Slug: "depo"},
		},
	}
	for _, o := range offers {
		p := domain.Product{ID: o.id, SKU: o.sku, FamilyID: o.fam, FamilyName: o.famName, Title: o.title, VendorID: o.vendor,
			CategoryID: o.cat, Popularity: o.pop, OfferURL: "https://store.example/" + o.vendor + "/" + o.id + "/",
			FamilyURL: o.famURL, Available: p6bool(o.price > 0), License: domain.License{Type: o.licType, Kind: o.kind, OrgType: o.org}}
		if p.License.Type == "" {
			p.License.Type = domain.LicensePerpetual
		}
		if o.price > 0 {
			p.PriceRub = p6price(o.price)
		} else {
			p.PriceOnRequest = true
		}
		if o.old > 0 {
			p.OldPriceRub = p6price(o.old)
		}
		s.Products = append(s.Products, p)
	}
	return s
}

func p6catalog() *searchLexicon {
	b, s := domain.LicenseKindBase, domain.LicenseKindSupport
	return newSearchLexicon(p6snapshot([]p6offer{
		{id: "c1", fam: "f-std", famName: "Кибер Бэкап Стандартная редакция", vendor: "cyberprotect", cat: "backup", title: "Кибер Бэкап Стандартная", kind: b, price: 1000},
		{id: "c2", fam: "f-ext", famName: "Кибер Бэкап Расширенная редакция", vendor: "cyberprotect", cat: "backup", title: "Кибер Бэкап Расширенная, поддержка", kind: s, price: 900, pop: 0.9},
		{id: "c3", fam: "f-ext", famName: "Кибер Бэкап Расширенная редакция", vendor: "cyberprotect", cat: "backup", title: "Кибер Бэкап Расширенная", kind: b, price: 2000},
		{id: "c4", fam: "f-start", famName: "Кибер Бэкап Старт", vendor: "cyberprotect", cat: "backup", title: "Кибер Бэкап Старт", kind: b, price: 500},
		{id: "a1", fam: "f-se", famName: "Astra Linux Special Edition", vendor: "astralinux", cat: "os", title: "Сертификат техподдержки Astra Linux SE", kind: s, price: 300, pop: 0.9, famURL: "https://store.example/astralinux/astra-linux-special-edition/"},
		{id: "a2", fam: "f-se", famName: "Astra Linux Special Edition", vendor: "astralinux", cat: "os", title: "Astra Linux SE, лицензия", kind: b, price: 3000, old: 4000, famURL: "https://store.example/astralinux/astra-linux-special-edition/"},
		{id: "a3", fam: "f-se", famName: "Astra Linux Special Edition", vendor: "astralinux", cat: "os", title: "Astra Linux SE для образования", kind: b, org: "academic", price: 100, famURL: "https://store.example/astralinux/astra-linux-special-edition/"},
		{id: "a4", fam: "f-ald", famName: "ALD Pro", vendor: "astralinux", cat: "os", title: "ALD Pro", kind: b, price: 5000},
		{id: "m1", fam: "f-pro", famName: "МТС Линк Про", vendor: "webinar", cat: "comm", title: "МТС Линк Про", kind: b, price: 4500},
		{id: "m2", fam: "f-st", famName: "МТС Линк Стандарт", vendor: "webinar", cat: "comm", title: "МТС Линк Стандарт", kind: b, price: 3600},
		{id: "m3", fam: "f-opt", famName: "МТС Линк Optimum", vendor: "webinar", cat: "comm", title: "МТС Линк Optimum", price: 6600},
		{id: "k1", fam: "f-kes", famName: "Kaspersky Endpoint Security", vendor: "kaspersky", cat: "av", title: "Kaspersky Endpoint Security", kind: b, sku: "KL4863RARDR", price: 0},
		{id: "k2", fam: "f-kes", famName: "Kaspersky Endpoint Security", vendor: "kaspersky", cat: "av", title: "Kaspersky Endpoint Security, продление", kind: domain.LicenseKindRenewal, licType: domain.LicenseRenewal, price: 0},
		{id: "k3", fam: "f-ksc", famName: "Kaspersky Security Center", vendor: "kaspersky", cat: "av", title: "Kaspersky Security Center", kind: b, price: 700},
		{id: "k4", fam: "f-kav", famName: "Kaspersky Anti-Virus", vendor: "kaspersky", cat: "av", title: "Kaspersky Anti-Virus, продление", kind: domain.LicenseKindRenewal, licType: domain.LicenseRenewal, price: 800},
		{id: "s1", fam: "f-inf", famName: "Сервер INFERIT RS212", vendor: "inferit", cat: "srv", title: "Сервер INFERIT RS212", price: 100000},
		{id: "s2", fam: "f-depo", famName: "Сервер DEPO Storm", vendor: "depo", cat: "srv", title: "Сервер DEPO Storm", price: 90000},
	}))
}

func p6rc(lx *searchLexicon, vendors ...string) rankCtx {
	ent := queryEntities{vendors: vendors, vendorTokens: map[string]bool{}}
	return rankCtx{lx: lx, attrs: queryAttrs{consumed: map[int]bool{}}, ent: ent, purchased: map[string]bool{},
		purchasedQty: map[string]int{}, productRel: map[string]float64{}, textTop: map[string]bool{}}
}

func p6fams(t *testing.T, rc rankCtx, group string, ids ...string) []*familyHit {
	t.Helper()
	var fams []*familyHit
	var rej []domain.RejectedHit
	for _, id := range ids {
		fh := rc.buildFamily(id, 1, &rej)
		if fh == nil {
			t.Fatalf("семейство %s не собрано", id)
		}
		fh.group = group
		fams = append(fams, fh)
	}
	return fams
}

func p6target(t *testing.T, rc rankCtx, query, category string, fams []*familyHit) targetDecision {
	t.Helper()
	words := queryWords(strings.Fields(foldText(query)), nil)
	return selectTarget(targetInput{rc: rc, words: words, fams: fams, exactDoc: exactDocMatcher(strings.Fields(foldText(query)), nil), category: category})
}

func TestP6FamilyHeadIsBaseCommercialBuyable(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	var rej []domain.RejectedHit
	fh := rc.buildFamily("f-se", 1, &rej)
	if fh.head.ProductID != "a2" {
		t.Fatalf("голова Astra Linux SE: %s, нужна базовая лицензия a2 (не техподдержка, не академическая)", fh.head.ProductID)
	}
	for _, d := range fh.heads {
		if d.ProductID == "a3" {
			t.Fatal("академический вариант не должен быть среди вариантов без «для образования» в запросе")
		}
	}
	// Прямо просят техподдержку — она наравне с базовой (выше популярность).
	rc.wantKinds = wantedKinds([]string{"техподдержка"}, rc.attrs)
	if fh := rc.buildFamily("f-se", 1, &rej); fh.head.ProductID != "a1" {
		t.Fatalf("на «техподдержка» голова %s, нужна a1", fh.head.ProductID)
	}
	// Тип организации назван — академический вариант виден.
	rc = p6rc(lx)
	rc.attrs.org = OrgEducation
	found := false
	for _, d := range rc.buildFamily("f-se", 1, &rej).heads {
		found = found || d.ProductID == "a3"
	}
	if !found {
		t.Fatal("при «для образования» академический вариант должен остаться")
	}
}

func TestP6TargetVendorCategoryForLine(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	dec := p6target(t, rc, "кибер бэкап", "backup", p6fams(t, rc, domain.GroupMatch, "f-std", "f-ext", "f-start"))
	if dec.target == nil || dec.target.Type != out.TargetVendorCategory {
		t.Fatalf("цель: %+v, нужна vendor_category", dec.target)
	}
	if dec.target.URL != "https://store.example/cyberprotect/?filter[category][]=backup" {
		t.Fatalf("URL: %s", dec.target.URL)
	}
	if len(dec.families) != 3 {
		t.Fatalf("цель представляет %d семейств, нужно 3", len(dec.families))
	}
}

func TestP6TargetVendorForBrandOnly(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	dec := p6target(t, rc, "мтс", "", p6fams(t, rc, domain.GroupExact, "f-pro", "f-st", "f-opt"))
	if dec.target == nil || dec.target.Type != out.TargetVendor || dec.target.URL != "https://store.example/webinar/" {
		t.Fatalf("цель: %+v, нужен вендор webinar", dec.target)
	}
}

func TestP6TargetFamilyForBrandWithOneFamily(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	dec := p6target(t, rc, "astra linux", "os", p6fams(t, rc, domain.GroupMatch, "f-se", "f-ald"))
	if dec.target == nil || dec.target.Type != out.TargetFamily ||
		dec.target.URL != "https://store.example/astralinux/astra-linux-special-edition/" {
		t.Fatalf("цель: %+v, нужно семейство Astra Linux SE", dec.target)
	}
}

func TestP6TargetSKUWhenFamilyURLMissing(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	dec := p6target(t, rc, "ald pro", "", p6fams(t, rc, domain.GroupMatch, "f-ald", "f-se"))
	if dec.target == nil || dec.target.Type != out.TargetSKU || dec.coverageMiss != "f-ald" {
		t.Fatalf("цель: %+v, промах покрытия %q: нужна SKU и промах f-ald", dec.target, dec.coverageMiss)
	}
}

func TestP6TargetExactSKU(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	dec := p6target(t, rc, "KL4863RARDR", "", p6fams(t, rc, domain.GroupMatch, "f-kes"))
	if dec.target == nil || dec.target.Type != out.TargetSKU || dec.skuDoc == nil || dec.skuDoc.ProductID != "k1" {
		t.Fatalf("цель: %+v, нужен SKU k1", dec.target)
	}
}

func TestP6TargetCategoryForGenericWord(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	dec := p6target(t, rc, "сервер", "srv", p6fams(t, rc, domain.GroupMatch, "f-inf", "f-depo"))
	if dec.target == nil || dec.target.Type != out.TargetCategory || dec.target.Title != "Серверы" {
		t.Fatalf("цель: %+v, нужна категория «Серверы»", dec.target)
	}
}

func TestP6TargetServiceForNonProduct(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	rc.ent.nonProduct = true
	if dec := p6target(t, rc, "доставка", "", nil); dec.target == nil || dec.target.Type != out.TargetService {
		t.Fatalf("цель: %+v, нужна service", dec.target)
	}
}

func p6resp(rc rankCtx, fams []*familyHit) *out.SearchResponse {
	resp := &out.SearchResponse{Query: "q", Items: []out.SearchItem{}}
	for _, fh := range fams {
		resp.Items = append(resp.Items, rc.toItem(fh))
	}
	return resp
}

func TestP6LadderFallbackKeepsHardConditions(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	rc.attrs.renewal = true // продление — жёсткое условие
	fams := p6fams(t, rc, domain.GroupExact, "f-kes")
	resp := p6resp(rc, fams)
	trace := &domain.Trace{}
	step := answerStage(answerInput{rc: rc, words: []string{"kaspersky"}, fams: fams, storefront: true}, resp, trace)
	if step.Fallback+step.Alternatives == 0 {
		t.Fatal("меньше четырёх семейств и точное только по запросу — нужна добивка лестницы")
	}
	for _, it := range resp.Items {
		if it.MatchType == "" {
			t.Fatalf("карточка %s без match_type", it.ProductID)
		}
		if lx.docs[it.ProductID].LicenseType != domain.LicenseRenewal {
			t.Fatalf("в выдачу попала не-продление %s (%s): жёсткое условие ослаблено", it.ProductID, it.MatchType)
		}
	}
	if resp.Items[0].MatchType != out.MatchExact {
		t.Fatalf("первая карточка %s, нужна exact", resp.Items[0].MatchType)
	}
}

func TestP6LadderAlternativeWhenOnlyOnRequest(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	fams := p6fams(t, rc, domain.GroupExact, "f-kes")
	resp := p6resp(rc, fams)
	step := answerStage(answerInput{rc: rc, words: []string{"kaspersky", "endpoint", "security"}, fams: fams, storefront: true}, resp, &domain.Trace{})
	if step.Alternatives == 0 {
		t.Fatal("точное только по запросу — нужен блок «Можно купить сейчас»")
	}
	for _, it := range resp.Items {
		if it.MatchType == out.MatchAlternative && (it.PriceState != out.PricePriced || it.Category != resp.Items[0].Category) {
			t.Fatalf("альтернатива %s: та же категория и только с ценой", it.ProductID)
		}
	}
	if resp.Items[0].PriceState != out.PriceOnRequest || resp.Items[0].PriceFrom != nil {
		t.Fatalf("Kaspersky Endpoint Security без цены: %s %v", resp.Items[0].PriceState, resp.Items[0].PriceFrom)
	}
}

func TestP6LadderNoFallbackForServiceWhenOff(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	rc.ent.nonProduct = true
	resp := &out.SearchResponse{Items: []out.SearchItem{}}
	step := answerStage(answerInput{rc: rc, storefront: true, serviceLike: true, fallbackOff: true}, resp, &domain.Trace{})
	if len(resp.Items) != 0 || !step.FallbackOff {
		t.Fatalf("SEARCH_FALLBACK_BLOCK=off: под сервисным запросом блока нет, карточек %d", len(resp.Items))
	}
	step = answerStage(answerInput{rc: rc, storefront: true, serviceLike: true}, resp, &domain.Trace{})
	if len(resp.Items) != ladderMin || step.Fallback != ladderMin {
		t.Fatalf("по умолчанию под сервисным запросом — четыре карточки «Популярное», есть %d", len(resp.Items))
	}
	if resp.Target == nil || resp.Target.Type != out.TargetService {
		t.Fatalf("цель сервисного запроса: %+v", resp.Target)
	}
}

func TestP6CardPriceAndDiscount(t *testing.T) {
	lx := p6catalog()
	rc := p6rc(lx)
	fams := p6fams(t, rc, domain.GroupExact, "f-se")
	resp := p6resp(rc, fams)
	answerStage(answerInput{rc: rc, words: []string{"astra", "linux"}, fams: fams}, resp, &domain.Trace{})
	it := resp.Items[0]
	if it.PriceState != out.PricePriced || it.PriceFrom == nil || *it.PriceFrom != 300 {
		t.Fatalf("цена «от»: %s %v, нужна 300 (минимум доступных, академическая скрыта)", it.PriceState, it.PriceFrom)
	}
	if len(it.Badges) != 1 || it.Badges[0] != "Скидка 25%" {
		t.Fatalf("подписи: %v, нужна «Скидка 25%%»", it.Badges)
	}
	if resp.Counts == nil || resp.Counts.SKUs != 2 || resp.Counts.ByMatchType[out.MatchExact] != 1 {
		t.Fatalf("счётчики: %+v", resp.Counts)
	}
}

// Честное «нет» — шаг лестницы: пояснение остаётся сверху, ниже «Популярное в наличии»; в прогонах оценки
// (NoLog) блока нет — в метрики релевантности он не входит.
func TestP6HonestNoBecomesLadderStep(t *testing.T) {
	env := imported(t, true)
	resp := search(t, env, domain.ConfigB3, "qwzxvbn", "")
	if resp.Empty == nil {
		t.Fatalf("пояснение «Не нашли» должно остаться: %+v", resp)
	}
	if len(resp.Items) == 0 {
		t.Fatal("под честным «нет» — блок «Популярное в наличии»")
	}
	for _, it := range resp.Items {
		if it.MatchType != out.MatchFallback || it.PriceState != out.PricePriced {
			t.Fatalf("карточка %s: %s %s — нужны fallback с ценой", it.ProductID, it.MatchType, it.PriceState)
		}
	}
	if resp.Total != 0 || resp.Counts == nil || resp.Counts.ByMatchType[out.MatchFallback] != len(resp.Items) {
		t.Fatalf("Total %d (fallback не считается), счётчики %+v", resp.Total, resp.Counts)
	}
	ev, err := env.svc.SearchWith(t.Context(), "01TEST", in.SearchRequest{Query: "qwzxvbn", Config: domain.ConfigB3}, SearchOptions{NoLog: true})
	if err != nil {
		t.Fatal(err)
	}
	if len(ev.Items) != 0 {
		t.Fatalf("прогон оценки без «Популярного»: %d карточек", len(ev.Items))
	}
}

// Позиция из категории курсов — курс, даже если фид пишет «базовая» или «продление»: головой и точным совпадением
// не становится, если о курсе не просили.
func TestP6CourseCategoryIsCourse(t *testing.T) {
	root := "sw"
	snap := p6snapshot([]p6offer{
		{id: "e1", fam: "f-course", famName: "Kaspersky Express Course", vendor: "kaspersky", cat: "courses", title: "Kaspersky Express Course, продление", kind: domain.LicenseKindRenewal, licType: domain.LicenseRenewal, price: 4000},
		{id: "e2", fam: "f-sym", famName: "Kaspersky Symphony Security", vendor: "kaspersky", cat: "av", title: "Kaspersky Symphony, продление", kind: domain.LicenseKindRenewal, licType: domain.LicenseRenewal, price: 6000},
	})
	snap.Categories = append(snap.Categories, domain.Category{ID: "courses", Name: "Курсы по информационной безопасности", ParentID: &root})
	lx := newSearchLexicon(snap)
	if k := lx.kindOf(lx.docs["e1"]); k != domain.LicenseKindCourse {
		t.Fatalf("вид позиции из категории курсов: %s", k)
	}
	rc := p6rc(lx, "kaspersky")
	rc.attrs.renewal = true
	rc.wantKinds = wantedKinds([]string{"продлить"}, rc.attrs)
	fams := p6fams(t, rc, domain.GroupExact, "f-course", "f-sym")
	resp := p6resp(rc, fams)
	answerStage(answerInput{rc: rc, words: []string{"kaspersky"}, fams: fams}, resp, &domain.Trace{})
	if resp.Items[0].FamilyID != "f-sym" || resp.Items[0].MatchType != out.MatchExact {
		t.Fatalf("первой должна быть Symphony exact: %s %s", resp.Items[0].FamilyID, resp.Items[0].MatchType)
	}
	if resp.Items[1].MatchType != out.MatchRelated || !strings.Contains(resp.Items[0].LicenseLine+resp.Items[1].LicenseLine, "Продление") {
		t.Fatalf("курс без запроса — related: %+v", resp.Items[1])
	}
}

// Ступень цены фида «от 100 до 149» закрывает «на 100 пк» и «на 120 пк»; 150 — уже нет.
func TestP6TierQuantity(t *testing.T) {
	lo, hi, q := 100, 149, 100
	d := indexDoc{LicenseQuantity: &q, LicenseTierMin: &lo, LicenseTierMax: &hi}
	for n, want := range map[int]bool{100: true, 120: true, 149: true, 150: false, 99: false} {
		if quantityOK(d, n) != want {
			t.Fatalf("ступень 100–149, %d: %v", n, !want)
		}
	}
}

// Общее семейство выше редакций: «Платформа …» среди комплектов той же линейки; чужой вендор с тем же словом —
// не равный кандидат, если линейка почти целиком у одного вендора.
func TestP6TargetBaseOfLine(t *testing.T) {
	b := domain.LicenseKindBase
	lx := newSearchLexicon(p6snapshot([]p6offer{
		{id: "n1", fam: "f-k1", famName: "nanoCAD комплект Корпоративный", vendor: "kaspersky", cat: "av", title: "nanoCAD комплект Корпоративный", kind: b, price: 260000},
		{id: "n2", fam: "f-k2", famName: "nanoCAD комплект Инженерия", vendor: "kaspersky", cat: "av", title: "nanoCAD комплект Инженерия", kind: b, price: 190000},
		{id: "n3", fam: "f-pl", famName: "Платформа nanoCAD 26", vendor: "kaspersky", cat: "av", title: "Платформа nanoCAD 26", kind: b, price: 55900, famURL: "https://store.example/nanocad/"},
		{id: "n4", fam: "f-mp", famName: "ModPlus nanoCAD", vendor: "depo", cat: "av", title: "ModPlus nanoCAD", kind: b, price: 1275},
	}))
	rc := p6rc(lx)
	dec := p6target(t, rc, "nanocad", "", p6fams(t, rc, domain.GroupMatch, "f-mp", "f-k1", "f-k2", "f-pl"))
	if dec.target == nil || dec.target.Type != out.TargetFamily || dec.target.URL != "https://store.example/nanocad/" {
		t.Fatalf("цель: %+v, нужна «Платформа nanoCAD»", dec.target)
	}
}
