package services

import (
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Контур доводки, закупка и честное «нет» (purchase_*.go, help_index.go, analogs_reestr.go, showcase_eval.go).

func TestExtractPurchase(t *testing.T) {
	cases := []struct {
		q                string
		deal, org        string
		renewal, reestr  bool
		quantity, months int
	}{
		{q: "продлить касперский на 100 пк на 2 года", renewal: true, quantity: 100, months: 24},
		{q: "расширение лицензии kaspersky на 20 узлов", deal: DealExtension, quantity: 20},
		{q: "миграция на касперский", deal: DealMigration},
		{q: "антивирус для школы", org: OrgEducation},
		{q: "замена офиса для госкомпании", org: OrgGovernment},
		{q: "аналог microsoft office из реестра", reestr: true},
	}
	for _, c := range cases {
		a := extractAttrs(strings.Fields(c.q))
		if a.deal != c.deal || a.org != c.org || a.renewal != c.renewal || a.reestrRequired != c.reestr ||
			a.quantity != c.quantity || a.duration != c.months {
			t.Errorf("%q: deal=%q org=%q renewal=%v reestr=%v qty=%d months=%d", c.q, a.deal, a.org, a.renewal,
				a.reestrRequired, a.quantity, a.duration)
		}
	}
	// Слова организации остаются в тексте поиска, слова сделки — нет.
	toks := strings.Fields("расширение для образования")
	a := extractAttrs(toks)
	if !a.consumed[0] || a.consumed[2] {
		t.Errorf("consumed: %v", a.consumed)
	}
}

func TestPurchaseRequestAndChips(t *testing.T) {
	a := extractAttrs(strings.Fields("миграция для образования"))
	a.applyRequest(in.SearchRequest{Clarify: map[string]string{FieldEdition: "Стандартный"}})
	chips, _ := a.chips(queryEntities{}, &searchLexicon{})
	got := map[string]string{}
	for _, c := range chips {
		got[c.Field] = c.Label
	}
	if got[FieldPurchaseType] != "Миграция" || got[FieldOrgType] != "Образовательное учреждение" ||
		got[FieldEdition] != "Редакция «Стандартный»" {
		t.Fatalf("chips: %v", got)
	}
	a.applyRequest(in.SearchRequest{RemovedChips: []in.RemovedChip{{Field: FieldPurchaseType}, {Field: FieldOrgType}}})
	if a.deal != "" || a.org != "" {
		t.Fatalf("снятые чипы: deal=%q org=%q", a.deal, a.org)
	}
	b := extractAttrs(strings.Fields("продлить касперский"))
	b.applyRequest(in.SearchRequest{Clarify: map[string]string{FieldEdition: clarifyAny}})
	if b.edition != "" || !b.editionAnswered {
		t.Fatalf("«Не знаю»: edition=%q answered=%v", b.edition, b.editionAnswered)
	}
}

func p4doc(id, attrs, lt string) indexDoc {
	return indexDoc{ProductID: id, FamilyID: "kes", Attributes: attrs, LicenseType: lt}
}

func TestOfferHidden(t *testing.T) {
	edu := p4doc("edu", "edition: Стандартный; org_type: education; purchase: поштучно", "subscription")
	mig := p4doc("mig", "deal: migration; purchase: поштучно", "subscription")
	ren := p4doc("ren", "deal: renewal; purchase: поштучно", "renewal")
	pack := p4doc("pack", "edition: Стандартный", "renewal")
	rc := func(q string) rankCtx { return rankCtx{attrs: extractAttrs(strings.Fields(q))} }
	check := func(q string, d indexDoc, want bool) {
		t.Helper()
		if got, _ := rc(q).offerHidden(d); got != want {
			t.Errorf("%q %s: hidden=%v, want %v", q, d.ProductID, got, want)
		}
	}
	check("kaspersky endpoint security", edu, true)
	check("kaspersky для образования", edu, false)
	check("kaspersky endpoint security", mig, true)
	check("миграция на kaspersky", mig, false)
	check("продление kaspersky", ren, true) // без количества и срока — готовая фасовка
	check("продлить kaspersky на 100 пк", ren, false)
	check("продлить kaspersky на 2 года", ren, false)
	check("kaspersky", pack, false)
	if s := rc("миграция для образования").purchaseFit(mig); s != 5 {
		t.Errorf("purchaseFit migration = %d", s)
	}
}

func TestWithoutHiddenOffers(t *testing.T) {
	lx := &searchLexicon{docs: map[string]indexDoc{
		"a":   p4doc("a", "", "subscription"),
		"mig": p4doc("mig", "deal: migration", "subscription"),
		"b":   p4doc("b", "", "subscription"),
	}}
	hits := []domain.RankedHit{{ProductID: "a", Rank: 1}, {ProductID: "mig", Rank: 2}, {ProductID: "b", Rank: 3}}
	got := withoutHiddenOffers(lx, extractAttrs([]string{"kaspersky"}), hits)
	if len(got) != 2 || got[1].ProductID != "b" || got[1].Rank != 2 {
		t.Fatalf("got %+v", got)
	}
	same := withoutHiddenOffers(lx, extractAttrs([]string{"миграция"}), hits)
	if len(same) != 3 {
		t.Fatalf("миграция названа: %+v", same)
	}
}

func TestEditionClarify(t *testing.T) {
	std := indexDoc{ProductID: "s", LicenseEdition: "Стандартный"}
	ext := indexDoc{ProductID: "e", LicenseEdition: "Расширенный"}
	fh := &familyHit{group: domain.GroupExact, heads: []indexDoc{std, ext}, fits: map[string]bool{"s": true, "e": true}}
	a := extractAttrs(strings.Fields("продлить касперский на 100 пк"))
	c := editionClarify(a, []*familyHit{fh}, strings.Fields("продлить kaspersky на 100 пк"))
	if c == nil || c.Attribute != FieldEdition || c.Question != "Какая редакция у вас сейчас: Стандартный или Расширенный?" ||
		len(c.Options) != 3 {
		t.Fatalf("clarify: %+v", c)
	}
	if editionClarify(a, []*familyHit{fh}, strings.Fields("продлить kaspersky стандартный")) != nil {
		t.Error("редакция названа в запросе — вопроса нет")
	}
	if editionClarify(extractAttrs([]string{"kaspersky"}), []*familyHit{fh}, []string{"kaspersky"}) != nil {
		t.Error("без типа покупки — вопроса нет")
	}
}

var testHelp = []domain.HelpArticle{
	{ID: "invoice", Title: "Счёт", Triggers: []string{"счет", "счёт", "оплатить по счету"}},
	{ID: "renewal", Title: "Продление", Triggers: []string{"как продлить", "продлить лицензию"}, Mixed: DealRenewal},
}

func TestMatchHelp(t *testing.T) {
	if a := matchHelp(testHelp, []string{"счет"}); a == nil || a.ID != "invoice" {
		t.Fatalf("счет: %+v", a)
	}
	if a := matchHelp(testHelp, strings.Fields("как продлить лицензию")); a == nil || a.ID != "renewal" {
		t.Fatalf("как продлить лицензию: %+v", a)
	}
	if a := matchHelp(testHelp, strings.Fields("счетчик электроэнергии")); a != nil {
		t.Fatalf("счетчик — не счёт: %+v", a)
	}
}

func TestHelpForModes(t *testing.T) {
	s := &Services{}
	s.procurement.once.Do(func() { s.procurement.help = testHelp })
	if h := s.helpFor([]string{"счет"}, queryAttrs{}, queryEntities{nonProduct: true}); h == nil || h.Mode != HelpModeAnswer {
		t.Fatalf("счёт: %+v", h)
	}
	ren := extractAttrs(strings.Fields("продлить kaspersky"))
	if h := s.helpFor(strings.Fields("продлить kaspersky"), ren, queryEntities{vendors: []string{"kaspersky"}}); h == nil ||
		h.Mode != HelpModeHint {
		t.Fatalf("смешанный: %+v", h)
	}
	if h := s.helpFor(strings.Fields("как продлить kaspersky"), ren, queryEntities{vendors: []string{"kaspersky"}}); h == nil ||
		h.Mode != HelpModeHint {
		t.Fatalf("товар назван — не ответ справки: %+v", h)
	}
	resp := &out.SearchResponse{Items: []out.SearchItem{{ProductID: "x"}}, Total: 1}
	applyHelpAnswer(resp)
	if len(resp.Items) != 0 || resp.Total != 0 {
		t.Fatalf("ответ справки: товаров нет — %+v", resp.Items)
	}
}

func TestReestrStates(t *testing.T) {
	s := &Services{}
	s.procurement.once.Do(func() {
		s.procurement.reestr = map[string]domain.ReestrEntry{"myoffice": {FamilyID: "myoffice", Status: domain.ReestrConfirmed, Number: "1083"}}
	})
	lx := &searchLexicon{families: map[string][]string{"myoffice": {"m"}, "r7": {"r"}, "office": {"o"}},
		docs: map[string]indexDoc{"m": {Origin: "ru"}, "r": {Origin: "ru"}, "o": {Origin: "foreign"}}}
	for fam, want := range map[string]string{"myoffice": domain.ReestrConfirmed, "r7": domain.ReestrUnknown,
		"office": domain.ReestrNotMatching} {
		if got := s.reestrOf(lx, fam); got.Status != want {
			t.Errorf("%s: %s, want %s", fam, got.Status, want)
		}
	}
	if got := s.reestrOf(lx, "myoffice").Label; got != "В реестре российского ПО, № 1083" {
		t.Errorf("label %q", got)
	}
	resp := &out.SearchResponse{Items: []out.SearchItem{{FamilyID: "myoffice"}, {FamilyID: "r7"}}, Total: 2}
	s.applyReestr(lx, resp, true)
	if len(resp.Items) != 1 || resp.Items[0].FamilyID != "myoffice" || len(resp.ReestrUnconfirmed) != 1 || resp.Total != 1 {
		t.Fatalf("из реестра: items %+v, unconfirmed %+v, total %d", resp.Items, resp.ReestrUnconfirmed, resp.Total)
	}
}

func TestShowcaseClass(t *testing.T) {
	help := &out.HelpAnswer{Mode: HelpModeAnswer}
	cases := []struct {
		r    out.SearchResponse
		want string
	}{
		{out.SearchResponse{Help: help}, ShowcaseHelp},
		{out.SearchResponse{Empty: &out.Empty{}, Items: []out.SearchItem{{Group: domain.GroupAnalog}}}, ShowcaseAbsent},
		{out.SearchResponse{Items: []out.SearchItem{{Group: domain.GroupAnalog}}}, ShowcaseAnalogs},
		{out.SearchResponse{Items: []out.SearchItem{{Group: domain.GroupMatch}}}, ShowcaseProduct},
		{out.SearchResponse{}, ShowcaseEmpty},
	}
	for i, c := range cases {
		if got := ShowcaseClass(&c.r); got != c.want {
			t.Errorf("#%d: %s, want %s", i, got, c.want)
		}
	}
}

func TestQuotePurchase(t *testing.T) {
	q, d := 100, 24
	p, bad := quotePurchase("items[0]", &in.QuotePurchase{Query: " продлить касперский ", Deal: DealRenewal,
		Quantity: &q, DurationMonths: &d, Edition: "Стандартный"})
	if len(bad) > 0 || p.Query != "продлить касперский" || p.Deal != DealRenewal || *p.Quantity != 100 {
		t.Fatalf("p=%+v bad=%v", p, bad)
	}
	if _, bad := quotePurchase("items[0]", &in.QuotePurchase{Deal: "steal"}); len(bad) != 1 {
		t.Fatalf("неизвестный тип покупки: %v", bad)
	}
	if p, bad := quotePurchase("items[0]", nil); p != nil || bad != nil {
		t.Fatal("нет параметров — nil")
	}
}

func TestMetricRelaxedMessage(t *testing.T) {
	dev := domain.MetricDevice
	fams := []*familyHit{{head: indexDoc{LicenseMetric: &dev}}}
	m := metricRelaxedMessage(domain.MetricUser, 50, fams)
	if m == nil || !strings.Contains(m.Message, "50 пользователей — это 50 устройств") {
		t.Fatalf("%+v", m)
	}
	if metricRelaxedMessage("", 50, fams) != nil {
		t.Fatal("метрика не снималась — сообщения нет")
	}
}
