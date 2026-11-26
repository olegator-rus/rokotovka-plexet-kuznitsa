package services

import (
	"context"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
)

func resIntp(v int) *int       { return &v }
func resFp(v float64) *float64 { return &v }
func resStrp(v string) *string { return &v }
func resIDs(fs []*familyHit) []string {
	ids := make([]string, len(fs))
	for i, f := range fs {
		ids[i] = f.id
	}
	return ids
}

func resEqual(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// resFixture — три семейства одной группы: «a» релевантнее всех, но с минимальной популярностью и маржей,
// «c» — наоборот; у «b» вариант с ценой по запросу и продление.
func resFixture() (rankCtx, []*familyHit) {
	docs := map[string]indexDoc{
		"a1": {ProductID: "a1", FamilyID: "a", VendorID: "va", Vendor: "A", LicenseType: domain.LicenseSubscription,
			LicenseMetric: resStrp(domain.MetricDevice), LicenseQuantity: resIntp(100), LicenseDuration: resIntp(12),
			PriceRub: resFp(50000), Popularity: 0, Margin: 0},
		"a2": {ProductID: "a2", FamilyID: "a", VendorID: "va", Vendor: "A", LicenseType: domain.LicenseRenewal,
			LicenseMetric: resStrp(domain.MetricDevice), LicenseQuantity: resIntp(100), LicenseDuration: resIntp(12),
			PriceRub: resFp(30000)},
		"b1": {ProductID: "b1", FamilyID: "b", VendorID: "vb", Vendor: "B", LicenseType: domain.LicenseSubscription,
			LicenseMetric: resStrp(domain.MetricDevice), LicenseQuantity: resIntp(100), LicenseDuration: resIntp(24),
			PriceOnRequest: true, Popularity: 0.5, Margin: 0.5},
		"c1": {ProductID: "c1", FamilyID: "c", VendorID: "vc", Vendor: "C", LicenseType: domain.LicensePerpetual,
			LicenseMetric: resStrp(domain.MetricUser), LicenseQuantity: resIntp(100), PriceRub: resFp(90000),
			InReestr: true, Popularity: 1, Margin: 1},
	}
	lx := &searchLexicon{docs: docs, vendorName: map[string]string{"va": "A", "vb": "B", "vc": "C"}}
	rc := rankCtx{lx: lx, purchased: map[string]bool{}, boughtVendors: map[string]bool{}}
	mk := func(id string, rel float64, heads ...string) *familyHit {
		fh := &familyHit{id: id, rel: rel, group: domain.GroupMatch, fits: map[string]bool{}}
		for _, h := range heads {
			fh.heads = append(fh.heads, docs[h])
			fh.fits[h] = true
		}
		fh.head, fh.headOK = fh.heads[0], true
		rc.score(fh)
		return fh
	}
	fams := []*familyHit{mk("a", 1.0, "a1", "a2"), mk("b", 0.95, "b1"), mk("c", 0.9, "c1")}
	sortFamilies(fams)
	return rc, fams
}

// AC: «По соответствию запросу» — порядок без маржи и популярности; «Рекомендуемые» их учитывают.
func TestResultsRelevanceSortIgnoresMarginAndPopularity(t *testing.T) {
	_, fams := resFixture()
	if got := resIDs(fams); !resEqual(got, []string{"c", "b", "a"}) {
		t.Fatalf("рекомендуемые учитывают популярность и маржу: %v", got)
	}
	p := &resultsPlan{sort: SortRelevance}
	p.sortFamilies(fams)
	if got := resIDs(fams); !resEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("по соответствию — только релевантность: %v", got)
	}
	// Маржа и популярность меняются — порядок по соответствию тот же.
	for _, fh := range fams {
		fh.parts.Margin, fh.parts.Popularity = 1-fh.parts.Margin, 1-fh.parts.Popularity
	}
	p.sortFamilies(fams)
	if got := resIDs(fams); !resEqual(got, []string{"a", "b", "c"}) {
		t.Fatalf("порядок не зависит от маржи: %v", got)
	}
	if p.marginAllowed() {
		t.Fatal("у сортировки по соответствию шаг маржи выключен")
	}
}

func TestResultsPriceSortOnRequestLast(t *testing.T) {
	_, fams := resFixture()
	(&resultsPlan{sort: SortPriceAsc}).sortFamilies(fams)
	if got := resIDs(fams); !resEqual(got, []string{"a", "c", "b"}) {
		t.Fatalf("дешевле: «от» 30000, 90000, по запросу в конце: %v", got)
	}
	(&resultsPlan{sort: SortPriceDesc}).sortFamilies(fams)
	if got := resIDs(fams); !resEqual(got, []string{"c", "a", "b"}) {
		t.Fatalf("дороже: по запросу всё равно в конце: %v", got)
	}
}

func TestResultsFiltersFacetsAndRefinements(t *testing.T) {
	rc, fams := resFixture()
	// Фильтр «продление»: у «a» остаётся вариант-продление и он становится представителем; «b», «c» уходят.
	p := &resultsPlan{sort: SortRecommended, group: GroupFamily,
		filters: parseLicFilters([]in.Filter{{Key: facetLicType, Op: "eq", Value: domain.LicenseRenewal}})}
	var rejected []domain.RejectedHit
	res := p.apply(rc, fams, nil, "антивирус", &rejected)
	if len(res) != 1 || res[0].head.ProductID != "a2" || p.counts.Families != 1 || p.counts.Variants != 1 {
		t.Fatalf("фильтр по варианту: %+v counts=%+v", resIDs(res), p.counts)
	}
	for _, fh := range fams {
		if fh.id == "a" && (len(fh.heads) != 2 || fh.head.ProductID != "a1") {
			t.Fatal("исходное семейство не меняется (фасеты считаются по нему)")
		}
	}
	// Фасет типа лицензии считается без своего фильтра: видны все три значения.
	var types map[string]int
	for _, f := range p.facets {
		if f.Key == facetLicType {
			types = map[string]int{}
			for _, v := range f.Values {
				types[v.Value.(string)] = v.Count
			}
		}
	}
	if types[domain.LicenseRenewal] != 1 || types[domain.LicenseSubscription] != 2 || types[domain.LicensePerpetual] != 1 {
		t.Fatalf("счётчики фасета типа: %v", types)
	}
	// Без фильтров — уточнения сужают выдачу и не ведут в пустоту; поле типа уже выбрано — его нет.
	p2 := &resultsPlan{sort: SortRecommended, group: GroupFamily}
	p2.apply(rc, fams, nil, "антивирус", &rejected)
	if len(p2.refinements) == 0 {
		t.Fatal("уточнения есть")
	}
	for _, r := range p2.refinements {
		if r.Hits == 0 || r.Field == nil {
			t.Fatalf("уточнение без проверки числом: %+v", r)
		}
	}
	for _, r := range p.refinements {
		if *r.Field == facetLicType {
			t.Fatalf("выбранное поле не предлагается: %+v", r)
		}
	}
}

func TestResultsFlattenAndPriceFrom(t *testing.T) {
	rc, fams := resFixture()
	p := &resultsPlan{sort: SortRecommended, group: GroupNone}
	p.apply(rc, fams, nil, "q", &[]domain.RejectedHit{})
	p.keep(fams)
	flat := p.flatten(rc, 20, nil)
	if len(flat) != 4 || p.counts.Variants != 4 {
		t.Fatalf("без группировки — все позиции: %d", len(flat))
	}
	for _, it := range flat {
		if len(it.Variants) != 0 {
			t.Fatal("без вложенных вариантов")
		}
	}
	// Точный артикул первым.
	p.exact = true
	exact := func(d indexDoc) bool { return d.ProductID == "a2" }
	if flat = p.flatten(rc, 20, exact); flat[0].ProductID != "a2" {
		t.Fatalf("точный артикул первым: %s", flat[0].ProductID)
	}
	for _, fh := range fams {
		if fh.id == "a" && (familyPrice(fh) == nil || *familyPrice(fh) != 30000) {
			t.Fatal("цена «от» — минимум по вариантам")
		}
		if fh.id == "b" && familyPrice(fh) != nil {
			t.Fatal("по запросу — не ноль")
		}
	}
}

func TestResultsPlanOffWithoutFlag(t *testing.T) {
	s := &Services{features: Features{Hardware: true}}
	if s.newResultsPlan(context.Background(), in.SearchRequest{}) != nil {
		t.Fatal("флаг выключен — плана нет, ответ контура коммерции")
	}
	var p *resultsPlan
	req := in.SearchRequest{Filters: []in.Filter{{Key: "ram_gb", Op: "gte", Value: 32.0}}}
	if got := p.hwRequest(req); len(got.Filters) != 1 || !p.marginAllowed() {
		t.Fatal("nil-план ничего не меняет")
	}
	s.features.ResultsV3 = true
	p = s.newResultsPlan(context.Background(), in.SearchRequest{Sort: "price_asc", Group: "none",
		Filters: []in.Filter{{Key: "ram_gb", Op: "gte", Value: 32.0}, {Key: facetLicType, Op: "eq", Value: "renewal"}}})
	if p == nil || p.sort != SortPriceAsc || p.group != GroupNone || len(p.filters) != 1 {
		t.Fatalf("план: %+v", p)
	}
	if got := p.hwRequest(req); len(got.Filters) != 1 || got.Filters[0].Key != "ram_gb" {
		t.Fatalf("оборудованию — только его фильтры: %+v", got.Filters)
	}
}

// Переход из панели (§11.7): фильтры category (с подкатегориями) и vendor по id.
func TestResultsCategoryAndVendorFilters(t *testing.T) {
	d := indexDoc{ProductID: "x", CategoryID: "antivirus-home", CategoryIDs: []string{"security", "antivirus-home"}, VendorID: "kaspersky"}
	cases := []struct {
		f    in.Filter
		want bool
	}{
		{in.Filter{Key: "category", Op: "eq", Value: "antivirus-home"}, true},
		{in.Filter{Key: "category", Op: "eq", Value: "security"}, true},
		{in.Filter{Key: "category", Op: "eq", Value: "office"}, false},
		{in.Filter{Key: "category", Op: "not", Value: "security"}, false},
		{in.Filter{Key: "vendor", Op: "eq", Value: "kaspersky"}, true},
		{in.Filter{Key: "vendor", Op: "eq", Value: "eset"}, false},
	}
	for _, c := range cases {
		fs := parseLicFilters([]in.Filter{c.f})
		if len(fs) != 1 || fs[0].match(d) != c.want {
			t.Fatalf("%+v: ожидалось %v", c.f, c.want)
		}
	}
}
