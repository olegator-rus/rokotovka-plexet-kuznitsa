package services

import (
	"encoding/json"
	"fmt"
	"math"
	"os"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/in"
	"kuznica/api/out"
)

// hwEnv — сервисы с флагом оборудования и состоянием каталога из каталога dir (без OpenSearch).
func hwEnv(t *testing.T, dir string, on bool) (*Services, *catalogState, *domain.CatalogSnapshot) {
	t.Helper()
	snap, err := catalogsource.MockJSON{Dir: dir}.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	svc := NewServices(nil, Options{Features: Features{Hardware: on}})
	st := newCatalogState(snap, 1)
	svc.catalog.Store(st)
	return svc, st, snap
}

// hwParse — та часть конвейера B3, которую меняет шаг hardware: нормализация, атрибуты, разбор, сущности.
func hwParse(svc *Services, st *catalogState, snap *domain.CatalogSnapshot, q string) (*hwPlan, queryAttrs, queryEntities) {
	rules := newSearchRules(1, snap.Aliases)
	nr := normalizeFull(q, st.lex, rules, false)
	attrs := extractAttrs(nr.tokens)
	req := in.SearchRequest{Query: q}
	p := svc.hardwareParse(nil, st, nr, &attrs, req)
	ent := findEntities(nr, attrs, st.lex, rules)
	p.applyEntities(&ent)
	return p, attrs, ent
}

func TestHardwareFixture(t *testing.T) {
	svc, st, snap := hwEnv(t, "../testdata/catalog", true)

	p, _, _ := hwParse(svc, st, snap, "sns r740 32g")
	if p.mpn == nil || p.mpn.Norm != "SNSR74032G" || p.mpn.Products[0] != "ram-32g-rdimm" {
		t.Fatalf("парт-номер из трёх токенов: %+v", p.mpn)
	}
	p, _, _ = hwParse(svc, st, snap, "SNS-R740")
	if p.mpn == nil || !p.mpn.Prefix {
		t.Fatalf("парт-номер по началу: %+v", p.mpn)
	}

	p, _, ent := hwParse(svc, st, snap, "память 32 гб для Dell R740")
	if p.target != "dell-poweredge-r740" || p.compatMode != "filter" {
		t.Fatalf("цель совместимости: %q %q", p.target, p.compatMode)
	}
	if len(p.conds) != 1 || p.conds[0].Key != "ram_gb" || p.conds[0].Value != 32 {
		t.Fatalf("условия: %+v", p.conds)
	}
	if contains(ent.vendors, "dell") {
		t.Fatal("вендор цели «для Dell R740» не должен быть строгим фильтром")
	}
	if r, _ := p.reject(st.lex.docs["ram-32g-rdimm"]); r != "" {
		t.Fatalf("совместимый модуль отклонён: %s", r)
	}
	if r, _ := p.reject(st.lex.docs["dell-r740-base"]); r == "" {
		t.Fatal("сервер без правила совместимости должен уйти в отказы")
	}
	chips, _ := p.chips(nil, nil)
	labels := []string{}
	for _, c := range chips {
		labels = append(labels, c.Label)
	}
	if !strings.Contains(strings.Join(labels, "|"), "Совместимо с Dell PowerEdge R740") {
		t.Fatalf("чипы: %v", labels)
	}

	p, _, _ = hwParse(svc, st, snap, "сервер dell r740 от 16 ядер 2u")
	if p.model != "dell-poweredge-r740" || p.target != "" {
		t.Fatalf("модель без «для»: model=%q target=%q", p.model, p.target)
	}

	// Флаг выключен — разбора нет, атрибуты не тронуты.
	off, stOff, _ := hwEnv(t, "../testdata/catalog", false)
	if p, _, _ := hwParse(off, stOff, snap, "память 32 гб для Dell R740"); p != nil {
		t.Fatal("флаг выключен — план должен быть nil")
	}
}

func TestHardwareLicenseQueriesUntouched(t *testing.T) {
	svc, st, snap := hwEnv(t, "../testdata/catalog", true)
	for _, q := range []string{"windows server 2022 на 16 ядер", "сервер 1с 8.3 x64", "1с:предприятие 8.3 на 20 рабочих мест",
		"вмваре 16 ядер", "защита от утечек информации", "касперский на 10 пк 2 года", "windows server без cal"} {
		p, attrs, _ := hwParse(svc, st, snap, q)
		if p.active() {
			t.Errorf("%q: лицензионный запрос разобран как оборудование: %+v", q, p.step())
		}
		base := extractAttrs(normalizeFull(q, st.lex, newSearchRules(1, snap.Aliases), false).tokens)
		if base.quantity != attrs.quantity || base.metric != attrs.metric {
			t.Errorf("%q: количество лицензий изменено: %d/%s → %d/%s", q, base.quantity, base.metric, attrs.quantity, attrs.metric)
		}
	}
}

func TestHwNum(t *testing.T) {
	for v, want := range map[float64]string{100000: "100 000", 15.6: "15,6", 32: "32", 2000: "2000"} {
		if got := hwNum(v); got != want {
			t.Errorf("hwNum(%v) = %q, want %q", v, got, want)
		}
	}
}

// TestHardwareParseP2 — точность разбора на наборе p2 (data/mock, поле expect): парт-номер, количество, цель
// совместимости, фильтры. Отчёт: go test ./services/ -run HardwareParseP2 -v.
func TestHardwareParseP2(t *testing.T) {
	const dir = "../../data/mock"
	if _, err := os.Stat(dir + "/attribute_definitions.json"); err != nil {
		t.Skip("data/mock контура коммерции нет")
	}
	svc, st, snap := hwEnv(t, dir, true)
	qs, err := catalogsource.MockJSON{Dir: dir}.LoadQueries(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	type expectT struct {
		MPN          *string          `json:"mpn"`
		MPNPrefix    *string          `json:"mpn_prefix"`
		Quantity     *int             `json:"quantity"`
		CompatTarget *json.RawMessage `json:"compat_target"`
		Filters      *[]struct {
			Key    string  `json:"key"`
			Op     string  `json:"op"`
			Value  any     `json:"value"`
			Value2 float64 `json:"value2"`
		} `json:"filters"`
	}
	stats := map[string][2]int{}
	hit := func(kind string, ok bool, q string, detail string) {
		s := stats[kind]
		s[1]++
		if ok {
			s[0]++
		} else {
			t.Logf("  %-7s ✗ %q: %s", kind, q, detail)
		}
		stats[kind] = s
	}
	// Запросы контура поиска (test, tune, regression, demo) шаг hardware не трогает.
	p1, p1Active := 0, 0
	for _, q := range qs {
		if q.Set == domain.EvalSetP2 {
			continue
		}
		p1++
		if p, _, _ := hwParse(svc, st, snap, q.Text); p.active() {
			p1Active++
			t.Logf("  контур поиска ✗ %q разобран как оборудование: %+v", q.Text, p.step())
		}
	}
	t.Logf("контур поиска: запросов %d, из них разобрано как оборудование %d", p1, p1Active)
	if p1Active > 0 {
		t.Errorf("шаг hardware сработал на %d запросах контура поиска", p1Active)
	}
	for _, q := range qs {
		if q.Set != domain.EvalSetP2 || len(q.Expect) == 0 || !strings.HasPrefix(q.Type, "p2_") || q.Type == "p2_bundle" {
			continue
		}
		var e expectT
		if json.Unmarshal(q.Expect, &e) != nil {
			continue
		}
		p, attrs, _ := hwParse(svc, st, snap, q.Text)
		if e.MPN != nil {
			ok := p.mpn != nil && p.mpn.Norm == normalizeMPN(*e.MPN)
			hit("mpn", ok, q.Text, fmt.Sprintf("%+v", p.mpn))
		}
		if e.MPNPrefix != nil {
			ok := p.mpn != nil && strings.HasPrefix(p.mpn.Norm, normalizeMPN(*e.MPNPrefix))
			hit("mpn", ok, q.Text, fmt.Sprintf("%+v", p.mpn))
		}
		if e.Quantity != nil && q.Type != "p2_license" {
			got := attrs.quantity // лицензии («антивирус на 50 пк»)
			if p != nil && p.quantity > 0 {
				got = p.quantity // закупка оборудования («20 ноутбуков»)
				if attrs.quantity != 0 {
					got = -1 // количество не должно остаться и лицензионным
				}
			}
			hit("quantity", got == *e.Quantity, q.Text, fmt.Sprintf("got %d", got))
		}
		if e.CompatTarget != nil {
			var want *string
			_ = json.Unmarshal(*e.CompatTarget, &want)
			got := ""
			if p != nil {
				got = p.target
			}
			ok := want == nil && got == "" || want != nil && got == *want
			if want != nil && st.hw.modelByID[*want] == nil {
				continue // цель — ПО («для windows server 2025»): совместимость лицензий — трек допродажи
			}
			hit("compat", ok, q.Text, "got "+got)
		}
		if e.Filters != nil && q.Type != "p2_license" {
			want := map[string]bool{}
			for _, f := range *e.Filters {
				want[filterKey(f.Key, f.Op, f.Value, f.Value2)] = true
			}
			got := map[string]bool{}
			if p != nil {
				for _, c := range p.conds {
					var v any = c.Value
					if c.typ != "number" {
						v = c.Str
					}
					got[filterKey(c.Key, c.Op, v, c.Value2)] = true
				}
				if p.priceMax != nil {
					got[filterKey("price_rub", "lte", *p.priceMax, 0)] = true
				}
				if p.priceMin != nil {
					got[filterKey("price_rub", "gte", *p.priceMin, 0)] = true
				}
			}
			ok := len(want) == len(got)
			for k := range want {
				ok = ok && got[k]
			}
			hit("filters", ok, q.Text, fmt.Sprintf("want %v got %v", keys(want), keys(got)))
		}
	}
	total := [2]int{}
	for k, s := range stats {
		t.Logf("%-9s %d/%d", k, s[0], s[1])
		total[0] += s[0]
		total[1] += s[1]
	}
	t.Logf("units_parse_acc %d/%d = %.3f", total[0], total[1], float64(total[0])/math.Max(1, float64(total[1])))
	if total[1] > 0 && float64(total[0])/float64(total[1]) < 0.85 {
		t.Errorf("точность разбора ниже 0,85")
	}
}

func filterKey(key, op string, v any, v2 float64) string {
	switch x := v.(type) {
	case float64:
		if op == "range" {
			return fmt.Sprintf("%s %s %g-%g", key, op, x, v2)
		}
		return fmt.Sprintf("%s %s %g", key, op, x)
	case bool:
		return fmt.Sprintf("%s %s %v", key, op, x)
	}
	return fmt.Sprintf("%s %s %v", key, op, strings.ToLower(fmt.Sprint(v)))
}

func keys(m map[string]bool) []string {
	var out []string
	for k := range m {
		out = append(out, k)
	}
	return out
}

func TestHardwareFacetsAndFilters(t *testing.T) {
	svc, st, snap := hwEnv(t, "../testdata/catalog", true)
	rules := newSearchRules(1, snap.Aliases)
	nr := normalizeFull("сервер dell 2u", st.lex, rules, false)
	attrs := extractAttrs(nr.tokens)
	req := in.SearchRequest{Query: "сервер dell 2u", Filters: []in.Filter{{Key: "ram_gb", Op: "gte", Value: float64(64)}, {Key: "nope", Op: "eq", Value: "x"}}}
	p := svc.hardwareParse(nil, st, nr, &attrs, req)
	if p.category != "servers" {
		t.Fatalf("категория: %q", p.category)
	}
	var ram *hwCond
	for i := range p.conds {
		if p.conds[i].Key == "ram_gb" {
			ram = &p.conds[i]
		}
	}
	if ram == nil || ram.Op != "gte" || ram.Value != 64 || ram.Source != "filter" {
		t.Fatalf("фасет из запроса: %+v", p.conds)
	}
	if r, _ := p.reject(st.lex.docs["dell-r740-base"]); r != rejectSpec {
		t.Fatalf("сервер с 32 ГБ при фасете ≥ 64: %q", r)
	}
	resp := &out.SearchResponse{Query: req.Query}
	p.decorate(resp)
	keys := map[string]out.Facet{}
	for _, f := range resp.Facets {
		keys[f.Key] = f
	}
	f, ok := keys["ram_gb"]
	if !ok || f.Min == nil || *f.Min != 32 || len(f.Values) != 1 || f.Values[0].Count != 1 {
		t.Fatalf("фасет памяти считается без своего условия: %+v", resp.Facets)
	}
	if _, ok := keys["form_factor_u"]; !ok {
		t.Fatalf("фасеты категории: %+v", resp.Facets)
	}
	// Снятый чип убирает условие.
	nr = normalizeFull("память 32 гб для Dell R740", st.lex, rules, false)
	attrs = extractAttrs(nr.tokens)
	p = svc.hardwareParse(nil, st, nr, &attrs, in.SearchRequest{RemovedChips: []in.RemovedChip{{Field: "ram_gb", Value: "32"}, {Field: fieldCompat}}})
	if len(p.conds) != 0 || p.target != "" {
		t.Fatalf("снятые чипы: conds=%+v target=%q", p.conds, p.target)
	}
}
