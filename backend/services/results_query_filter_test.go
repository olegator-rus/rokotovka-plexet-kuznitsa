package services

import (
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/out"
)

func TestQueryLicFiltersFromChips(t *testing.T) {
	chips := []out.Chip{
		{Field: fieldVendor, Value: "kaspersky"},
		{Field: domain.FieldLicenseDuration, Value: "12", Label: "Срок 1 год"},
		{Field: domain.FieldLicenseType, Value: domain.LicensePerpetual, Label: "Бессрочная"},
	}
	fs := queryLicFilters(chips, nil)
	if len(fs) != 2 || fs[0].key != facetDuration || !fs[0].isNum || fs[0].num != 12 || fs[1].key != facetLicType || fs[1].str != domain.LicensePerpetual {
		t.Fatalf("фильтры из чипов: %+v", fs)
	}
	// Фильтр панели по тому же полю важнее чипа запроса.
	if fs := queryLicFilters(chips, []licFilter{{key: facetDuration, op: "eq", str: "24"}}); len(fs) != 1 || fs[0].key != facetLicType {
		t.Fatalf("поле из панели не пропущено: %+v", fs)
	}
}

func TestQuantityFacetLabels(t *testing.T) {
	dev, usr := domain.MetricDevice, domain.MetricUser
	q := quantityMetrics{}
	q.add("10", indexDoc{LicenseMetric: &dev})
	q.add("10", indexDoc{})
	q.add("1", indexDoc{LicenseMetric: &dev})
	labels := map[string]string{}
	q.label(labels)
	if labels["10"] != "10 устройств" || labels["1"] != "1 устройство" {
		t.Fatalf("одна метрика: %v", labels)
	}
	q.add("5", indexDoc{LicenseMetric: &usr})
	q.label(labels)
	if labels["10"] != "10 мест" || labels["5"] != "5 мест" {
		t.Fatalf("разные метрики: %v", labels)
	}
}

// orgChips — чипы запроса так, как их собирает разбор: тип организации — из слов запроса (purchase_attrs.go).
func orgChips(query string) []out.Chip {
	a := extractAttrs(strings.Fields(query))
	var chips []out.Chip
	a.purchaseChips(func(field, value, label string, strict bool) {
		chips = append(chips, out.Chip{Field: field, Value: value, Label: label, Strict: strict})
	})
	return chips
}

// orgFixture — три семейства: у «astra» есть лицензии для образовательных учреждений (в выдаче они — после
// коммерческой), у «kasp» — только коммерческие, у «alt» тип организации в фиде не размечен.
func orgFixture() (rankCtx, []*familyHit) {
	docs := map[string]indexDoc{
		"astra-com":  {ProductID: "astra-com", FamilyID: "astra", VendorID: "va", LicenseType: domain.LicenseSubscription, LicenseOrgType: "commercial", PriceRub: resFp(83500)},
		"astra-edu":  {ProductID: "astra-edu", FamilyID: "astra", VendorID: "va", LicenseType: domain.LicensePerpetual, LicenseOrgType: "academic", PriceRub: resFp(43650)},
		"astra-edu2": {ProductID: "astra-edu2", FamilyID: "astra", VendorID: "va", LicenseType: domain.LicensePerpetual, LicenseOrgType: "academic", PriceRub: resFp(54300)},
		"kasp-com":   {ProductID: "kasp-com", FamilyID: "kasp", VendorID: "vk", LicenseType: domain.LicenseSubscription, LicenseOrgType: "commercial", PriceRub: resFp(9000)},
		"kasp-home":  {ProductID: "kasp-home", FamilyID: "kasp", VendorID: "vk", LicenseType: domain.LicenseSubscription, LicenseOrgType: "home", PriceRub: resFp(2000)},
		"alt-1":      {ProductID: "alt-1", FamilyID: "alt", VendorID: "vb", LicenseType: domain.LicensePerpetual, PriceRub: resFp(5000)},
	}
	lx := &searchLexicon{docs: docs, vendorName: map[string]string{"va": "Астра", "vk": "Касперский", "vb": "Альт"}}
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
	fams := []*familyHit{mk("astra", 1.0, "astra-com", "astra-edu", "astra-edu2"), mk("kasp", 0.8, "kasp-com", "kasp-home"), mk("alt", 0.6, "alt-1")}
	sortFamilies(fams)
	return rc, fams
}

// Слова «для школы», «для вуза», «для университета», «образовательная», «учебная» — условие «образовательная
// лицензия»: у семейства с таким вариантом он становится головой карточки, остальные варианты уходят.
func TestNarrowByOrgEducationWords(t *testing.T) {
	for _, q := range []string{"astra linux для школы", "astra linux для вуза", "astra linux для университета",
		"astra linux образовательная", "astra linux учебная лицензия", "астры линукс для школ"} {
		rc, fams := orgFixture()
		chips := orgChips(q)
		if queryOrg(chips) != OrgEducation {
			t.Fatalf("%q: тип организации не разобран: %+v", q, chips)
		}
		p := &resultsPlan{sort: SortRecommended, group: GroupFamily}
		var rejected []domain.RejectedHit
		res := p.apply(rc, fams, chips, q, &rejected)
		if !resEqual(resIDs(res), []string{"astra", "kasp", "alt"}) {
			t.Fatalf("%q: состав и порядок семейств не меняются: %v", q, resIDs(res))
		}
		if res[0].head.ProductID != "astra-edu" || len(res[0].heads) != 2 || res[0].heads[1].ProductID != "astra-edu2" {
			t.Fatalf("%q: голова — образовательный вариант, варианты — только образовательные: %s %d", q, res[0].head.ProductID, len(res[0].heads))
		}
		// Мягкий откат по семейству: у «kasp» и «alt» такого варианта нет — их карточки прежние.
		if res[1].head.ProductID != "kasp-com" || len(res[1].heads) != 2 || res[2].head.ProductID != "alt-1" {
			t.Fatalf("%q: семейства без образовательного варианта не трогаются: %s %s", q, res[1].head.ProductID, res[2].head.ProductID)
		}
		if !chips[len(chips)-1].Strict || chips[len(chips)-1].Field != FieldOrgType {
			t.Fatalf("%q: условие применено к первому семейству — чип strict: %+v", q, chips)
		}
		if len(rejected) != 1 || rejected[0].ProductID != "astra-com" || rejected[0].Reason != rejectOffer {
			t.Fatalf("%q: снятый вариант — в трассе: %+v", q, rejected)
		}
		if len(p.queryRelaxed) != 0 || p.counts.Families != 3 || p.counts.Variants != 5 {
			t.Fatalf("%q: relaxed=%v counts=%+v", q, p.queryRelaxed, p.counts)
		}
		if fams[0].head.ProductID != "astra-com" || len(fams[0].heads) != 3 {
			t.Fatalf("%q: исходное семейство не меняется", q)
		}
	}
}

// Контрпример: «астра линукс» без слов об образовании — условия нет, выдача та же.
func TestNarrowByOrgNotNamed(t *testing.T) {
	rc, fams := orgFixture()
	chips := orgChips("astra linux")
	if len(chips) != 0 || queryOrg(chips) != "" {
		t.Fatalf("тип организации не назван: %+v", chips)
	}
	p := &resultsPlan{sort: SortRecommended, group: GroupFamily}
	res := p.narrowByOrg(rc, fams, chips, nil)
	for i, fh := range res {
		if fh != fams[i] {
			t.Fatalf("выдача без условия не меняется: %v", resIDs(res))
		}
	}
	if res[0].head.ProductID != "astra-com" || len(res[0].heads) != 3 {
		t.Fatalf("голова прежняя: %s", res[0].head.ProductID)
	}
	// Коммерческая организация — вариант по умолчанию: отдельного условия нет.
	if queryOrg(orgChips("astra linux коммерческая")) != "" {
		t.Fatal("коммерческая лицензия не сужает выдачу")
	}
}

// Контрпример: «касперский для школы», а образовательного варианта у найденного нет — мягкий откат: выдача прежняя,
// чип остаётся нестрогим, сообщения relaxed нет.
func TestNarrowByOrgSoftFallback(t *testing.T) {
	rc, all := orgFixture()
	var fams []*familyHit
	for _, fh := range all {
		if fh.id != "astra" {
			fams = append(fams, fh)
		}
	}
	chips := orgChips("kaspersky для школы")
	if queryOrg(chips) != OrgEducation {
		t.Fatalf("тип организации не разобран: %+v", chips)
	}
	p := &resultsPlan{sort: SortRecommended, group: GroupFamily}
	var rejected []domain.RejectedHit
	res := p.apply(rc, fams, chips, "kaspersky для школы", &rejected)
	if !resEqual(resIDs(res), []string{"kasp", "alt"}) || res[0].head.ProductID != "kasp-com" || len(res[0].heads) != 2 {
		t.Fatalf("откат: выдача прежняя: %v %s", resIDs(res), res[0].head.ProductID)
	}
	if chips[0].Strict || len(rejected) != 0 || len(p.queryRelaxed) != 0 {
		t.Fatalf("откат: strict=%v rejected=%v relaxed=%v", chips[0].Strict, rejected, p.queryRelaxed)
	}
	// Образовательный вариант есть не у первого семейства: первое — прежнее, у второго головой становится он,
	// чип нестрогий (к лучшему совпадению условие не применено).
	rc, all = orgFixture()
	all[0], all[1] = all[1], all[0]
	all[0].rel, all[1].rel = 1.0, 0.8
	rc.score(all[0])
	rc.score(all[1])
	chips = orgChips("kaspersky для школы")
	res = p.narrowByOrg(rc, all, chips, nil)
	if !resEqual(resIDs(res), []string{"kasp", "astra", "alt"}) || res[0].head.ProductID != "kasp-com" || res[1].head.ProductID != "astra-edu" || chips[0].Strict {
		t.Fatalf("вариант у второго семейства: %v %s %s strict=%v", resIDs(res), res[0].head.ProductID, res[1].head.ProductID, chips[0].Strict)
	}
}

// Точный артикул: показан сам товар, условие организации варианты не подменяет.
func TestNarrowByOrgSkipsExact(t *testing.T) {
	rc, fams := orgFixture()
	p := &resultsPlan{sort: SortRecommended, group: GroupNone, exact: true}
	res := p.narrowByOrg(rc, fams, orgChips("astra-com для школы"), nil)
	if res[0] != fams[0] || res[0].head.ProductID != "astra-com" {
		t.Fatal("точный запрос не сужается")
	}
}
