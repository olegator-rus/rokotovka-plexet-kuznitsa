package services

import (
	"slices"
	"testing"
)

// Отрицания, «или» и уточнения категорий (классы К2, К3, К5).

func negationOf(t *testing.T, q string) (vendors, families []string, masked []string) {
	t.Helper()
	lx, rules, _ := mockLexicon(t)
	nr := normalizeFull(q, lx, rules, false)
	spans := negationSpans(nr.tokens, lx, rules)
	vendors, families, _ = negatedEntities(nr.tokens, spans, lx, rules)
	return vendors, families, maskTokens(nr.tokens, spans)
}

func TestNegationExcludesVendorAndFamily(t *testing.T) {
	for q, want := range map[string]string{
		"антивирус кроме dr.web":  "drweb",
		"антивирус не касперский": "kaspersky",
		"офис без microsoft":      "microsoft",
	} {
		if vs, _, _ := negationOf(t, q); !slices.Contains(vs, want) {
			t.Errorf("%q: исключённые вендоры %v, ожидался %s", q, vs, want)
		}
	}
	if _, fs, _ := negationOf(t, "ос кроме windows"); len(fs) == 0 {
		t.Error("«ос кроме windows»: семейства Windows не исключены")
	}
}

func TestNegatedAttributeIsNotCondition(t *testing.T) {
	_, fs, masked := negationOf(t, "kaspersky без продления")
	if a := extractAttrs(masked); a.renewal {
		t.Error("«без продления» стало условием продления")
	}
	if len(fs) > 0 {
		t.Errorf("«без продления» исключило семейства %v", fs)
	}
	// Оператор числа — не исключение.
	if vs, fs, _ := negationOf(t, "коммутатор не дороже 100000"); len(vs)+len(fs) > 0 {
		t.Errorf("«не дороже» стало исключением: %v %v", vs, fs)
	}
}

func TestAlternativesKeepBothSides(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	for q, want := range map[string][]string{
		"windows или astra": {"microsoft", "astra"},
		"office или р7":     {"microsoft", "r7"},
	} {
		nr := normalizeFull(q, lx, rules, false)
		attrs := extractAttrs(nr.tokens)
		ent := findEntities(nr, attrs, lx, rules)
		ent.applyAlternatives(lx, rules)
		for _, v := range want {
			if !slices.Contains(ent.vendors, v) {
				t.Errorf("%q: вендоры %v, нет %s", q, ent.vendors, v)
			}
		}
		if slices.Contains(ent.vendors, "kaspersky") {
			t.Errorf("%q: «office» привело Kaspersky Small Office Security", q)
		}
	}
}

func TestCategoryQualifierAloneIsNotCategory(t *testing.T) {
	lx, _, _ := mockLexicon(t)
	if c, _ := lx.categoryOf([]string{"windows", "для", "дома"}); c != "" {
		t.Errorf("«windows для дома» → категория %q", c)
	}
	if c, _ := lx.categoryOf([]string{"антивирус", "для", "дома"}); c != "antivirus-home" {
		t.Errorf("«антивирус для дома» → %q", c)
	}
	if c, _ := lx.categoryOf([]string{"dr.web", "для", "бизнеса"}); c != "antivirus-business" {
		t.Errorf("«dr.web для бизнеса» → %q", c)
	}
}

func TestServerWordIsHardwareWithoutSoftware(t *testing.T) {
	svc, st, snap := hwEnv(t, "../../data/mock", true)
	for q, hw := range map[string]bool{
		"сервер": true, "сервер dell": true, "как выбрать новый сервер": true,
		"сервер 1с": false, "лицензия на сервер": false, "windows server": false, "скуль сервер 2022": false,
	} {
		p, _, _ := hwParse(svc, st, snap, q)
		if got := p != nil && p.category == "server-systems"; got != hw {
			t.Errorf("%q: категория серверов %v, ожидалось %v", q, got, hw)
		}
	}
}

func TestHardwareApproxAndUnits(t *testing.T) {
	svc, st, snap := hwEnv(t, "../../data/mock", true)
	for q, want := range map[string]float64{"ибп 3 ква": 2700, "ибп 3000 ва": 2700} {
		p, _, _ := hwParse(svc, st, snap, q)
		ok := false
		for _, c := range p.conds {
			ok = ok || c.Key == "power_w" && c.Value == want
		}
		if !ok {
			t.Errorf("%q: условия %+v, ожидалась мощность %v Вт", q, p.conds, want)
		}
	}
	if !numOK(hwCond{Key: "storage_gb", Op: "eq", Value: 1000}, 960) {
		t.Error("«1 тб» не принимает 960 ГБ")
	}
	if numOK(hwCond{Key: "storage_gb", Op: "eq", Value: 1000}, 480) {
		t.Error("«1 тб» принял 480 ГБ")
	}
	p, _, _ := hwParse(svc, st, snap, "сервер hpe 2 процессора")
	for _, c := range p.conds {
		if c.Key == "form_factor_u" {
			t.Errorf("«2 процессора» стало высотой: %+v", c)
		}
	}
}
