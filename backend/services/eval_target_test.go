package services

import (
	"os"
	"path/filepath"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// fakeTargetLookup — справочник вердикта из карт.
type fakeTargetLookup struct {
	docs   map[string]targetDoc
	famURL map[string]string
	cats   map[string][]string
}

func (f fakeTargetLookup) doc(id string) (targetDoc, bool) { d, ok := f.docs[id]; return d, ok }
func (f fakeTargetLookup) familyURL(id string) string      { return f.famURL[id] }
func (f fakeTargetLookup) categoriesNamed(n string) []string {
	return f.cats[n]
}

func newFakeLookup() fakeTargetLookup {
	renewal, base := "renewal", "base"
	tmin, tmax := 100, 149
	return fakeTargetLookup{
		docs: map[string]targetDoc{
			"p1": {Family: "121231", Vendor: "astralinux", Category: "222320", LicenseKind: base},
			"p2": {Family: "287015", Vendor: "astralinux", Category: "222320", LicenseKind: base},
			"p3": {Family: "3953", Vendor: "kaspersky", Category: "650", LicenseKind: renewal, TierMin: &tmin, TierMax: &tmax},
			"p4": {Family: "3935", Vendor: "kaspersky", Category: "650", LicenseKind: base},
			"p5": {Family: "447243", Vendor: "webinar", Category: "7388", OfferURL: "https://store.softline.ru/webinar/x-5/"},
			"s1": {Family: "offer-s1", Vendor: "depo", Category: "86563", Kind: "hardware"},
		},
		famURL: map[string]string{
			"121231": "https://store.softline.ru/astralinux/astra-linux-special-edition/",
			"3953":   "https://store.softline.ru/kaspersky/kes-standard/",
		},
		cats: map[string][]string{"Rack-серверы": {"86563"}, "Антивирусы для организаций": {"650"}},
	}
}

func items(ids ...string) []out.SearchItem {
	lk := newFakeLookup()
	var res []out.SearchItem
	for _, id := range ids {
		d := lk.docs[id]
		res = append(res, out.SearchItem{ProductID: id, FamilyID: d.Family, FamilyURL: lk.famURL[d.Family],
			OfferURL: d.OfferURL, MatchType: out.MatchExact})
	}
	return res
}

func TestTargetURLKey(t *testing.T) {
	same := [][2]string{
		{"/astralinux/astra-linux-special-edition/", "https://STORE.softline.ru/astralinux/astra-linux-special-edition"},
		{"/cyberprotect/?filter[category][]=1033", "https://store.softline.ru/cyberprotect/?filter%5Bcategory%5D%5B0%5D=1033"},
		{"http://www.store.softline.ru/webinar/", "store.softline.ru/webinar"},
		{"/a/?y=2&x=1", "/a?x=1&y=2#frag"},
	}
	for _, p := range same {
		if !sameTargetURL(p[0], p[1]) {
			t.Errorf("%q и %q — одна страница: %q != %q", p[0], p[1], targetURLKey(p[0]), targetURLKey(p[1]))
		}
	}
	if sameTargetURL("/webinar/", "/webinar/?filter[category][]=7388") || sameTargetURL("", "") {
		t.Fatal("разные страницы и пустые URL не совпадают")
	}
}

func TestJudgeTargetTypes(t *testing.T) {
	lk := newFakeLookup()
	cases := []struct {
		name   string
		expect domain.TargetExpect
		resp   *out.SearchResponse
		strict bool
		rank   int
	}{
		{"семейство по цели ответа", domain.TargetExpect{Type: domain.ExpectFamily, GroupIDs: []string{"121231"}},
			&out.SearchResponse{Target: &out.Target{Type: "family", URL: "https://store.softline.ru/astralinux/astra-linux-special-edition/"},
				Items: items("p1", "p2")}, true, 1},
		{"семейство без цели — URL первой карточки", domain.TargetExpect{Type: domain.ExpectFamily, GroupIDs: []string{"121231"}},
			&out.SearchResponse{Items: items("p2", "p1")}, false, 2},
		{"страница семейства не собрана — само семейство первым", domain.TargetExpect{Type: domain.ExpectFamily, GroupIDs: []string{"447243"}},
			&out.SearchResponse{Target: &out.Target{Type: "sku", URL: "https://store.softline.ru/webinar/x-5/"}, Items: items("p5")}, true, 1},
		{"вендор", domain.TargetExpect{Type: domain.ExpectVendor, VendorSlug: "webinar"},
			&out.SearchResponse{Target: &out.Target{Type: "vendor", URL: "https://store.softline.ru/webinar"}, Items: items("p5")}, true, 1},
		{"вендор + категория, [0] вместо []", domain.TargetExpect{Type: domain.ExpectVendorCategory, VendorSlug: "kaspersky", CategoryIDs: []string{"650"}},
			&out.SearchResponse{Target: &out.Target{Type: "vendor_category", URL: "/kaspersky/?filter[category][0]=650"}, Items: items("p3")}, true, 1},
		{"вендор + категория: допустимое семейство линейки", domain.TargetExpect{Type: domain.ExpectVendorCategory, VendorSlug: "kaspersky",
			CategoryIDs: []string{"650"}, GroupIDs: []string{"3953"}},
			&out.SearchResponse{Target: &out.Target{Type: "family", URL: "https://store.softline.ru/kaspersky/kes-standard/"}, Items: items("p3")}, true, 1},
		{"категория по названию в цели", domain.TargetExpect{Type: domain.ExpectCategory, CategoryIDs: []string{"86563"}},
			&out.SearchResponse{Target: &out.Target{Type: "category", Title: "Rack-серверы"}, Items: items("s1")}, true, 1},
		{"категория, цель — другая категория", domain.TargetExpect{Type: domain.ExpectCategory, CategoryIDs: []string{"86563"}},
			&out.SearchResponse{Target: &out.Target{Type: "category", Title: "Антивирусы для организаций"}, Items: items("p4")}, false, 0},
		{"категория без поля target — категория первой карточки", domain.TargetExpect{Type: domain.ExpectCategory, CategoryIDs: []string{"86563"}},
			&out.SearchResponse{Items: items("s1")}, true, 1},
		{"SKU", domain.TargetExpect{Type: domain.ExpectSKU, OfferIDs: []string{"p5"}},
			&out.SearchResponse{Target: &out.Target{Type: "sku", URL: "https://store.softline.ru/webinar/x-5"}, Items: items("p5")}, true, 1},
		{"бессмыслица: цели нет, точных карточек нет", domain.TargetExpect{Type: domain.ExpectNone},
			&out.SearchResponse{Items: []out.SearchItem{{ProductID: "p4", MatchType: out.MatchFallback}}}, true, 1},
		{"бессмыслица: BM25 выдал точное совпадение", domain.TargetExpect{Type: domain.ExpectNone},
			&out.SearchResponse{Items: items("p4")}, false, 0},
		{"Also: допустима и другая цель", domain.TargetExpect{Type: domain.ExpectCategory, CategoryIDs: []string{"445047"},
			Also: []domain.TargetExpect{{Type: domain.ExpectFamily, GroupIDs: []string{"121231"}}}},
			&out.SearchResponse{Target: &out.Target{Type: "family", URL: "/astralinux/astra-linux-special-edition/"}, Items: items("p1")}, true, 1},
	}
	for _, c := range cases {
		v := judgeTarget(domain.TargetQuery{ID: "q", Text: c.name, Expect: c.expect}, c.resp, lk)
		if v.Strict != c.strict || v.Rank != c.rank {
			t.Errorf("%s: strict %v rank %d, ждали %v %d (%+v)", c.name, v.Strict, v.Rank, c.strict, c.rank, v)
		}
	}
}

func TestJudgeTargetExtrasAndHard(t *testing.T) {
	lk := newFakeLookup()
	resp := &out.SearchResponse{Target: &out.Target{Type: "family", URL: "https://store.softline.ru/kaspersky/kes-standard/"},
		Items: items("p3", "p4")}
	q := domain.TargetQuery{ID: "h", Text: "продлить касперский на 120 пк", Expect: domain.TargetExpect{Type: domain.ExpectFamily,
		GroupIDs: []string{"3953"}, Hard: &domain.HardExpect{LicenseKind: "renewal", Quantity: 120}}}
	v := judgeTarget(q, resp, lk)
	// p3 — продление 100–149: подходит; p4 — базовая лицензия: нарушение.
	if !v.Strict || v.Violations != 1 || v.Pass || !v.HasHard {
		t.Fatalf("жёсткие условия: %+v", v)
	}
	q.Expect.Hard = &domain.HardExpect{Quantity: 200}
	if v := judgeTarget(q, resp, lk); v.Violations != 1 {
		t.Fatalf("200 мест не покрывает ступень 100–149: %+v", v)
	}
	// Альтернативы с ценой и исключённое семейство.
	resp.Items = append(resp.Items, out.SearchItem{ProductID: "p2", FamilyID: "287015", MatchType: out.MatchAlternative, PriceState: out.PricePriced})
	q.Expect.Hard, q.Expect.AltMin, q.Expect.Exclude = nil, 2, []string{"287015"}
	v = judgeTarget(q, resp, lk)
	if v.Alternatives != 1 || v.Pass || len(v.Fails) != 2 {
		t.Fatalf("альтернатив 1 из 2 и исключённое семейство: %+v", v)
	}
	// По первым N карточкам (набор понимания): категория — все N из неё.
	q2 := domain.TargetQuery{ID: "c", Text: "антивирус", Expect: domain.TargetExpect{Type: domain.ExpectCategory, CategoryIDs: []string{"650"}, Top: 2}}
	if v := judgeTarget(q2, &out.SearchResponse{Items: items("p3", "p4", "p1")}, lk); !v.Strict || !v.ByCards {
		t.Fatalf("первые 2 — антивирусы: %+v", v)
	}
	if v := judgeTarget(q2, &out.SearchResponse{Items: items("p3", "p1")}, lk); v.Strict {
		t.Fatalf("вторая карточка — не антивирус: %+v", v)
	}
}

func TestTargetAggregateAndChanges(t *testing.T) {
	mk := func(id string, strict, top4 bool, rank, fams int, route, parse string, llm int, zero bool) out.EvalQueryResult {
		return out.EvalQueryResult{ID: id, Text: id, Type: "neighbor", Route: route, Ms: float64(10 * (rank + 1)),
			Target: &out.TargetQueryResult{Strict: strict, Top4: top4, Rank: rank, Pass: strict, Families: fams, SKUs: fams * 2,
				Cards: fams, Vendor: "kaspersky", Distortion: "склонение", ParseSource: parse, LLMCalls: llm, ZeroResults: zero, CostRub: 0.1 * float64(llm)}}
	}
	rs := []out.EvalQueryResult{
		mk("a", true, true, 1, 5, "B", "rules", 0, false),
		mk("b", false, true, 3, 4, "D", "llm", 1, false),
		mk("c", false, false, 0, 2, "D", "rules", 0, true),
		mk("d", true, true, 1, 1, "D", "cache", 0, false),
		{ID: "e", Error: "сбой"},
	}
	m := targetAggregate(rs, TargetModeCold)
	tm := m.Target
	if m.Errors != 1 || tm.Answers != 4 || tm.Judged != 4 || tm.Success1N != 2 || *tm.Success1 != 0.5 || tm.Top4N != 3 {
		t.Fatalf("сводка: %+v", tm)
	}
	// MRR: (1 + 1/3 + 0 + 1) / 4.
	if *tm.MRR10 != evalRound4((1+1.0/3+1)/4) || tm.Families4N != 2 || tm.SKUs4N != 3 {
		t.Fatalf("MRR и ≥4: %+v", tm)
	}
	c := tm.Cost
	if c.RouteD != 3 || c.ModelSearches != 1 || c.CacheSearches != 1 || c.RouteDNoModel != 1 || c.Searches != 4 || c.Mode != TargetModeCold {
		t.Fatalf("стоимость: %+v", c)
	}
	if cell := tm.Breakdown["vendor"]["kaspersky"]; cell.N != 4 || cell.Success1 != 2 {
		t.Fatalf("разбивка: %+v", tm.Breakdown)
	}
	cur := []out.EvalQueryResult{mk("a", false, true, 2, 5, "B", "rules", 0, false), mk("b", true, true, 1, 4, "D", "llm", 1, false)}
	worse, better := TargetChanges(rs, cur, "B2")
	if len(worse) != 1 || worse[0].QueryID != "a" || worse[0].Reasons[0] != "lost_target" || len(better) != 1 || better[0].QueryID != "b" {
		t.Fatalf("изменения: хуже %+v, лучше %+v", worse, better)
	}
}

func TestApplyFeedCost(t *testing.T) {
	// p = 0,2, h = 0,5, c_llm = 0,2 ₽: LLM 0,02 ₽ на запрос; постоянные (8 218 + 336 + 0) / 15 000 = 0,5703 ₽.
	cr := &out.FeedCostRun{P: 0.2, H: 0.5}
	applyFeedCost(cr, evalPtr(0.2), 8218, 336, 0, 15000)
	if cr.LLMPerRequestRub != 0.02 || cr.FixedPerRequestRub != 0.5703 || cr.PerRequestRub != 0.5903 {
		t.Fatalf("стоимость: %+v", cr)
	}
	if s := cr.InfraShare + cr.AgentShare + cr.LLMShare + cr.SupportShare; s < 0.999 || s > 1.001 {
		t.Fatalf("доли в сумме 1: %+v", cr)
	}
	// Без замера c_llm (тёплый прогон без вызовов) — LLM 0.
	cr = &out.FeedCostRun{P: 0.2, H: 1}
	applyFeedCost(cr, nil, 8218, 0, 0, 15000)
	if cr.LLMPerRequestRub != 0 || cr.LLMShare != 0 || cr.InfraShare != 1 {
		t.Fatalf("без LLM: %+v", cr)
	}
}

func TestReadTargetQueries(t *testing.T) {
	dir := t.TempDir()
	good := filepath.Join(dir, "ok.jsonl")
	if err := os.WriteFile(good, []byte(`{"id":"a","text":"астра","expect":{"type":"vendor","vendor_slug":"astralinux"}}
# комментарий

{"id":"b","text":"ывапр","expect":{"type":"none"}}
`), 0o644); err != nil {
		t.Fatal(err)
	}
	qs, err := ReadTargetQueries(good)
	if err != nil || len(qs) != 2 {
		t.Fatalf("набор: %v %+v", err, qs)
	}
	for name, body := range map[string]string{
		"тип":    `{"id":"a","text":"x","expect":{"type":"page"}}`,
		"повтор": `{"id":"a","text":"x","expect":{"type":"none"}}` + "\n" + `{"id":"a","text":"y","expect":{"type":"none"}}`,
		"family": `{"id":"a","text":"x","expect":{"type":"family"}}`,
	} {
		p := filepath.Join(dir, name+".jsonl")
		_ = os.WriteFile(p, []byte(body), 0o644)
		if _, err := ReadTargetQueries(p); err == nil {
			t.Errorf("%s: ждали ошибку", name)
		}
	}
	s := &Services{}
	if _, err := s.TargetSetQueries(t.Context(), "closed"); !isStatus(err, 403) {
		t.Fatalf("закрытый набор агенту не отдаётся: %v", err)
	}
}

// Открытые наборы в data/softline/eval читаются и согласованы с источниками по числу строк.
func TestTargetSetFiles(t *testing.T) {
	dir := filepath.Join("..", "..", "data", "softline", "eval")
	want := map[string]int{"etalon.jsonl": 10, "tz.jsonl": 7, "neighbors.jsonl": 114}
	for f, n := range want {
		qs, err := ReadTargetQueries(filepath.Join(dir, f))
		if err != nil {
			t.Fatalf("%s: %v", f, err)
		}
		if len(qs) != n {
			t.Errorf("%s: %d строк, ждали %d", f, len(qs), n)
		}
	}
}
