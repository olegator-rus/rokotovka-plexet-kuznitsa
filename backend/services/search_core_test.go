package services

import (
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/in"
)

// Правила нормализации, атрибутов и ранжирования B3 на словаре mock-каталога (data/mock). Кейсы — из
// queries.json (типы typo, layout, slang, license, analog, absent) и демо-сценариев F2–F5.

func mockLexicon(t *testing.T) (*searchLexicon, *searchRules, *domain.CatalogSnapshot) {
	t.Helper()
	snap, err := catalogsource.MockJSON{Dir: "../../data/mock"}.Load(t.Context())
	if err != nil {
		t.Skipf("data/mock не читается: %v", err)
	}
	lx := newSearchLexicon(snap)
	return lx, newSearchRules(1, snap.Aliases), snap
}

func TestNormalizeLayoutTypoAlias(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	for _, c := range []struct {
		in, normalized, corrected, kinds string
		applied                          bool
	}{
		{"rfcgthcrbq", "kaspersky", "касперский", "layout,alias", true},
		{"rfcgthcrbq fynbdbhec", "kaspersky антивирус", "касперский антивирус", "layout,layout,alias", true},
		{"dbylf 11", "windows 11", "винда 11", "layout,alias", true},
		{"цштвщцы 11 зкщ", "windows 11 pro", "windows 11 pro", "layout,layout", true},
		{"1c ,e[ufknthbz", "1с бухгалтерия", "1c бухгалтерия", "layout,alias", true},
		{"касперкий", "kaspersky", "касперский", "typo,alias", true},
		{"каспеский", "kaspersky", "касперский", "typo,alias", true},
		{"1с бугалтерия", "1с бухгалтерия", "1с бухгалтерия", "typo", true},
		{"teamveiwer", "teamviewer", "teamviewer", "typo", true},
		{"windows 11 prо", "windows 11 pro", "windows 11 pro", "typo", true}, // кириллическая «о»
		{"винда 11 про", "windows 11 pro", "", "alias,alias", false},         // сленг плашки не вызывает
		{"office 2021", "office 2021", "", "", false},                        // версия не исправляется
		{"KL7785RKSFS", "kl7785rksfs", "", "clean", false},                   // артикул не исправляется
	} {
		nr := normalizeFull(c.in, lx, rules, false)
		if got := nr.normalized(); got != c.normalized {
			t.Errorf("%q → %q, ожидалось %q (%+v)", c.in, got, c.normalized, nr.changes)
		}
		var kinds []string
		for _, ch := range nr.changes {
			if ch.Kind != changeClean || c.kinds == "clean" {
				kinds = append(kinds, ch.Kind)
			}
		}
		if strings.Join(kinds, ",") != c.kinds {
			t.Errorf("%q: изменения %+v, ожидались %s", c.in, nr.changes, c.kinds)
		}
		switch {
		case c.applied && (nr.correction == nil || !nr.correction.applied || nr.correction.to != c.corrected):
			t.Errorf("%q: исправление %+v, ожидалось %q", c.in, nr.correction, c.corrected)
		case !c.applied && nr.correction != nil:
			t.Errorf("%q: исправления быть не должно: %+v", c.in, nr.correction)
		}
	}
}

func TestNormalizeMediumAndAsTyped(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	// «hom» — 3 буквы: исправление не ниже medium; поиск по исходному, плашка «Возможно, вы искали».
	nr := normalizeFull("windows 11 hom", lx, rules, false)
	if nr.normalized() != "windows 11 hom" {
		t.Fatalf("medium не применяется: %q", nr.normalized())
	}
	if c := nr.correction; c != nil && (c.applied || c.confidence != confidenceMedium) {
		t.Fatalf("medium: %+v", c)
	}
	// «Искать как введено»: ни раскладки, ни опечаток; алиасы — не исправление, остаются.
	nr = normalizeFull("касперкий", lx, rules, true)
	if nr.normalized() != "касперкий" || nr.correction != nil {
		t.Fatalf("as_typed: %q %+v", nr.normalized(), nr.correction)
	}
	// Непонятное слово не «исправляется» в случайное: notion, ноушн, солидворкс остаются как есть.
	for _, q := range []string{"notion", "ноушн", "солидворкс", "chatgpt"} {
		if nr := normalizeFull(q, lx, rules, false); nr.normalized() != q || nr.correction != nil {
			t.Fatalf("%q → %q %+v", q, nr.normalized(), nr.correction)
		}
	}
}

func TestDamerau(t *testing.T) {
	for _, c := range []struct {
		a, b string
		d    int
	}{{"teamveiwer", "teamviewer", 1}, {"касперкий", "касперский", 1}, {"abc", "abc", 0}, {"abc", "xyz", 3}} {
		if got := damerau([]rune(c.a), []rune(c.b), 5); got != c.d {
			t.Fatalf("%s/%s: %d, ожидалось %d", c.a, c.b, got, c.d)
		}
	}
}

func TestExtractAttrs(t *testing.T) {
	for _, c := range []struct {
		q                  string
		qty                int
		metric             string
		unresolved         bool
		dur                int
		licType            string
		renewal, analog    bool
		origin, versionTok string
	}{
		{q: "касперский на 10 пк", qty: 10, metric: domain.MetricDevice},
		{q: "1с на 5 пользователей", qty: 5, metric: domain.MetricUser},
		{q: "windows server на 16 ядер", qty: 16, metric: domain.MetricCore, versionTok: ""},
		{q: "отечественный офис на 30 компов бессрочная", qty: 30, metric: domain.MetricDevice, licType: domain.LicensePerpetual, origin: "ru"},
		{q: "р7 офис на 30 мест", qty: 30, unresolved: true},
		{q: "windows 11 pro 5 лицензий", qty: 5, unresolved: true, versionTok: "11"},
		{q: "kaspersky plus 3 устройства 2 года", qty: 3, metric: domain.MetricDevice, dur: 24},
		{q: "autocad на 3 года", dur: 36},
		{q: "акад лицензия на год", dur: 12},
		{q: "продление подписки astra linux", renewal: true},
		{q: "автокад подписка 3 года", dur: 36, licType: domain.LicenseSubscription},
		{q: "аналог photoshop на 20 пользователей", qty: 20, metric: domain.MetricUser, analog: true},
		{q: "astra linux на 10 рабочих станций", qty: 10, metric: domain.MetricDevice},
		{q: "1с рабочих мест 10", versionTok: ""},
		{q: "linux для госучреждения в реестре", origin: "ru"},
		{q: "windows 11", versionTok: "11"},
	} {
		tokens := strings.Fields(c.q)
		a := extractAttrs(tokens)
		if a.quantity != c.qty || a.metric != c.metric || a.metricUnresolved != c.unresolved || a.duration != c.dur ||
			a.licType != c.licType || a.renewal != c.renewal || a.analog != c.analog || a.origin != c.origin {
			t.Errorf("%q: %+v", c.q, a)
		}
		if c.versionTok != "" {
			for i, tok := range tokens {
				if tok == c.versionTok && a.consumed[i] {
					t.Errorf("%q: версия %s ушла в атрибуты", c.q, tok)
				}
			}
		}
	}
}

func TestAttrsClarifyAndRemovedChips(t *testing.T) {
	a := extractAttrs(strings.Fields("р7 офис на 30 мест"))
	cl := a.clarify()
	if cl == nil || cl.Attribute != domain.FieldLicenseMetric || len(cl.Options) != 3 || !strings.Contains(cl.Question, "30 мест") {
		t.Fatalf("уточнение: %+v", cl)
	}
	a.applyRequest(in.SearchRequest{Clarify: map[string]string{domain.FieldLicenseMetric: domain.MetricDevice}})
	if a.metric != domain.MetricDevice || a.clarify() != nil {
		t.Fatalf("ответ на уточнение: %+v", a)
	}
	a = extractAttrs(strings.Fields("касперский на 10 пк"))
	a.applyRequest(in.SearchRequest{RemovedChips: []in.RemovedChip{{Field: domain.FieldLicenseQuantity, Value: "10"}}})
	if a.quantity != 0 {
		t.Fatalf("снятый чип количества: %+v", a)
	}
}

func TestEntitiesAndRoute(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	parse := func(q string) (queryAttrs, queryEntities, string, []string) {
		nr := normalizeFull(q, lx, rules, false)
		a := extractAttrs(nr.tokens)
		e := findEntities(nr, a, lx, rules)
		r, sig := routeSignals(nr, a, e)
		return a, e, r, sig
	}
	_, e, route, _ := parse("касперский на 10 пк")
	if route != domain.RouteNormalized || len(e.vendors) != 1 || e.vendors[0] != "kaspersky" {
		t.Fatalf("касперский на 10 пк: %s %+v", route, e)
	}
	_, e, route, _ = parse("аналог фотошопа на 20 пользователей")
	if route != domain.RouteIntent || len(e.reference) != 1 || e.reference[0] != "adobe-photoshop" {
		t.Fatalf("аналог фотошопа: %s %+v", route, e)
	}
	_, e, route, _ = parse("notion")
	if route != domain.RouteIntent || !e.absent() {
		t.Fatalf("notion: %s %+v", route, e)
	}
	_, e, _, _ = parse("акт сверки")
	if !e.nonProduct || !e.absent() {
		t.Fatalf("акт сверки: %+v", e)
	}
	_, e, route, _ = parse("fqc-10528")
	if route != domain.RouteExact || e.absent() {
		t.Fatalf("артикул: %s %+v", route, e)
	}
	_, e, _, _ = parse("кис")
	if e.absent() || !e.abbrev {
		t.Fatalf("короткое сокращение не объявляется отсутствующим товаром: %+v", e)
	}
	_, e, route, _ = parse("программа для чертежей")
	if route != domain.RouteSemantic || e.absent() {
		t.Fatalf("назначение: %s %+v", route, e)
	}
}

// Допустимость и выбор лицензии: «касперский на 10 пк» — голова семейства на 10 устройств, семейства без
// такой лицензии отсеиваются, Dr.Web — другой вендор.
func TestRankLicenseFit(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	nr := normalizeFull("касперский на 10 пк", lx, rules, false)
	a := extractAttrs(nr.tokens)
	e := findEntities(nr, a, lx, rules)
	rc := rankCtx{lx: lx, attrs: a, ent: e, purchased: map[string]bool{}, purchasedQty: map[string]int{},
		boughtVendors: map[string]bool{}, productRel: map[string]float64{}, refCats: map[string]bool{}}
	var rejected []domain.RejectedHit
	if fh := rc.buildFamily("drweb-security-space", 1, &rejected); fh != nil || rejected[0].Reason != rejectVendor {
		t.Fatalf("Dr.Web при названном Kaspersky: %+v %+v", fh, rejected)
	}
	ksos := rc.buildFamily("kaspersky-small-office-security", 1, &rejected)
	if ksos == nil || *ksos.head.LicenseQuantity != 10 || !ksos.headOK || ksos.head.LicenseType == domain.LicenseRenewal {
		t.Fatalf("KSOS: голова %+v", ksos.head)
	}
	std := rc.buildFamily("kaspersky-standard", 1, &rejected)
	fams := rc.dropQuantityMisfits([]*familyHit{ksos, std}, &rejected)
	if len(fams) != 1 || fams[0].id != ksos.id {
		t.Fatalf("Standard без лицензии на 10 устройств должен уйти: %+v", fams)
	}
	rc.assignGroup(ksos)
	rc.score(ksos)
	if ksos.group != domain.GroupExact || ksos.final <= 0 || ksos.parts.Relevance != round4(0.8*ksos.rel) {
		t.Fatalf("группа и скор: %+v", ksos)
	}
}

// Продление: клиент А покупал KSOS на 10 ПК — его продление в группе exact, голова — тот же объём.
func TestRankRenewalContext(t *testing.T) {
	lx, rules, snap := mockLexicon(t)
	st := newCatalogState(snap, 1)
	nr := normalizeFull("продлить антивирус", lx, rules, false)
	a := extractAttrs(nr.tokens)
	e := findEntities(nr, a, lx, rules)
	svc := &Services{}
	rc := svc.rankContext(st, lx, "client-a", a, e)
	var rejected []domain.RejectedHit
	ksos := rc.buildFamily("kaspersky-small-office-security", 0.8, &rejected)
	drweb := rc.buildFamily("drweb-security-space", 1, &rejected)
	for _, fh := range []*familyHit{ksos, drweb} {
		rc.assignGroup(fh)
		rc.score(fh)
	}
	if ksos.head.ProductID != "ksos-10pc-ren" || ksos.group != domain.GroupExact || ksos.context != 1 {
		t.Fatalf("клиент А: %+v", ksos)
	}
	fams := []*familyHit{drweb, ksos}
	sortFamilies(fams)
	if fams[0].id != ksos.id {
		t.Fatalf("продление купленного — первым: %s", fams[0].id)
	}
	// Клиент Б без истории — контекста нет.
	rc = svc.rankContext(st, lx, "client-b", a, e)
	if b := rc.buildFamily("kaspersky-small-office-security", 0.8, &rejected); rc.contextScore(b.head) != 0 {
		t.Fatal("у клиента Б нет истории")
	}
}

func TestRelaxedMessages(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	nr := normalizeFull("kaspersky premium бессрочная", lx, rules, false)
	a := extractAttrs(nr.tokens)
	rc := rankCtx{lx: lx, attrs: a, ent: findEntities(nr, a, lx, rules), purchased: map[string]bool{},
		purchasedQty: map[string]int{}, boughtVendors: map[string]bool{}, productRel: map[string]float64{}}
	var rejected []domain.RejectedHit
	fh := rc.buildFamily("kaspersky-premium", 1, &rejected)
	rel := relaxedMessages(rc, []*familyHit{fh})
	if len(rel) != 1 || rel[0].Field != domain.FieldLicenseType || !strings.Contains(rel[0].Message, "Бессрочной лицензии нет") {
		t.Fatalf("ослабление: %+v", rel)
	}
}

func TestSuggestHelpers(t *testing.T) {
	if p := prefixAt("microsoft windows 11 pro", "win"); p != 10 {
		t.Fatalf("префикс слова: %d", p)
	}
	if p := prefixAt("windows 11", "win"); p != 0 {
		t.Fatalf("префикс строки: %d", p)
	}
	if p := prefixAt("kaspersky", "win"); p != -1 {
		t.Fatalf("нет совпадения: %d", p)
	}
	if j := jaccard(trigrams("касперски"), trigrams("касперский")); j < suggestMinNgram {
		t.Fatalf("n-граммы: %.2f", j)
	}
}
