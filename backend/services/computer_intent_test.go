package services

import (
	"context"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// «подобрать комп»: жаргон «комп», «пк», «компьютер», «системный блок» — настольный компьютер (категория «Десктопы»),
// а не «Клавиатуры, мыши, комплекты» по началу слова и не «… для компьютерной техники»; с числом мест лицензии —
// по-прежнему количество. Подбор лицензий на такой запрос не открывается.

func computerSnapshot() *domain.CatalogSnapshot {
	b := domain.LicenseKindBase
	s := p6snapshot([]p6offer{
		{id: "pc1", fam: "f-pc1", famName: "ПК DEPO Neos DF8", vendor: "depo", cat: "desk", title: "ПК DEPO Neos DF8", price: 99541},
		{id: "pc2", fam: "f-pc2", famName: "ПК SL 2487", vendor: "inferit", cat: "desk", title: "ПК SL 2487", price: 42958},
		{id: "kb1", fam: "f-kb1", famName: "Oklick S290W", vendor: "inferit", cat: "kbd", title: "Клавиатура+мышь Oklick Комплект S290W", price: 1500},
		{id: "bt1", fam: "f-bt1", famName: "GoPower LR6", vendor: "inferit", cat: "bat", title: "Аккумулятор для компьютерной техники GoPower LR6", price: 100},
		{id: "ps1", fam: "f-ps1", famName: "Yealink Блок питания", vendor: "depo", cat: "psu", title: "Yealink Блок питания", price: 900},
		{id: "ks1", fam: "f-kompas", famName: "КОМПАС-3D V25", vendor: "astralinux", cat: "os", title: "АСКОН КОМПАС-3D V25", kind: b, price: 150000},
		{id: "av1", fam: "f-kes-std", famName: "Kaspersky Endpoint Security для бизнеса Стандартный", vendor: "kaspersky", cat: "av", title: "Kaspersky Endpoint Security для бизнеса Стандартный", kind: b, price: 2800},
		{id: "av2", fam: "f-kes-ext", famName: "Kaspersky Endpoint Security для бизнеса Расширенный", vendor: "kaspersky", cat: "av", title: "Kaspersky Endpoint Security для бизнеса Расширенный", kind: b, price: 4000},
		{id: "av3", fam: "f-pro32", famName: "PRO32 Endpoint Security Advanced", vendor: "depo", cat: "av", title: "PRO32 Endpoint Security Advanced версия на 1 год и 30 узлов", kind: b, price: 79500},
	})
	pc, per, acc := "pc", "periph", "acc"
	s.Categories = append(s.Categories,
		domain.Category{ID: pc, Name: "Компьютеры"},
		domain.Category{ID: "desk", Name: "Десктопы", ParentID: &pc},
		domain.Category{ID: per, Name: "Компьютерная периферия"},
		domain.Category{ID: "kbd", Name: "Клавиатуры, мыши, комплекты", ParentID: &per},
		domain.Category{ID: acc, Name: "Аксессуары к компьютерной технике"},
		domain.Category{ID: "bat", Name: "Аккумуляторы и батареи для компьютерной техники", ParentID: &acc},
		domain.Category{ID: "psu", Name: "Блоки питания", ParentID: &acc},
	)
	for i := range s.Products {
		switch s.Products[i].CategoryID {
		case "desk", "kbd", "bat", "psu":
			s.Products[i].Kind = domain.KindHardware
		}
	}
	return s
}

func computerEnv(t *testing.T) (*Services, *catalogState) {
	t.Helper()
	svc := NewServices(nil, Options{Features: Features{Hardware: true}})
	svc.SetLexicon([]domain.LexiconEntry{
		{Alias: "системный блок", Canonical: "десктоп", Kind: domain.LexiconJargon, Target: domain.AliasTargetTerm},
	})
	st := newCatalogState(computerSnapshot(), 1)
	svc.catalog.Store(st)
	return svc, st
}

func computerParse(svc *Services, st *catalogState, q string) (*hwPlan, queryEntities) {
	rules := svc.rulesFor(context.Background(), domain.ConfigB3, st.lex, nil)
	nr := normalizeFull(q, st.lex, rules, false)
	attrs := extractAttrs(nr.tokens)
	p := svc.hardwareParse(context.Background(), st, nr, &attrs, in.SearchRequest{Query: q})
	ent := findEntities(nr, attrs, st.lex, rules)
	p.applyEntities(&ent)
	return p, ent
}

func TestComputerWordsNameDesktops(t *testing.T) {
	svc, st := computerEnv(t)
	for _, q := range []string{"подобрать комп", "комп", "компьютер", "компьютеры", "купить компьютер", "пк", "системный блок", "системник"} {
		if p, _ := computerParse(svc, st, q); p == nil || p.category != "desk" {
			t.Errorf("%q: категория %q, ждали «Десктопы»", q, p.category)
		}
	}
	for q, want := range map[string]string{
		"антивирус на 20 компьютеров":             "",    // места лицензии, а не оборудование
		"kaspersky на 5 пк":                       "",    // то же
		"компас":                                  "",    // название продукта
		"клавиатура мышь комплект":                "kbd", // настоящие слова категории
		"продлить касперский на 100 пк на 2 года": "",    // «пк» с числом — места лицензии
		"kaspersky 5 пк":                          "",
	} {
		if p, _ := computerParse(svc, st, q); p != nil && (p.category != want || want == "" && p.quantity != 0) {
			t.Errorf("%q: категория %q, количество закупки %d, ждали %q", q, p.category, p.quantity, want)
		}
	}
}

func TestComputerWordNotPrefixCompleted(t *testing.T) {
	_, st := computerEnv(t)
	nr := normalizeFull("подобрать комп", st.lex, newSearchRules(1, nil), false)
	if len(nr.prefixes) != 0 || !strings.Contains(strings.Join(nr.tokens, " "), "комп") {
		t.Fatalf("«комп» дописан как начало слова: %+v %v", nr.prefixes, nr.tokens)
	}
}

func TestComputerTargetIsDesktops(t *testing.T) {
	svc, st := computerEnv(t)
	for _, q := range []string{"пк", "подобрать комп", "компьютер"} {
		p, ent := computerParse(svc, st, q)
		rc := p6rc(st.lex)
		rc.ent, rc.hw = ent, p
		fams := p6fams(t, rc, domain.GroupMatch, "f-pc2", "f-pc1")
		dec := p6target(t, rc, q, ent.category, fams)
		if dec.target == nil || dec.target.Type != out.TargetCategory || dec.target.Title != "Десктопы" {
			t.Errorf("%q: цель %+v, ждали категорию «Десктопы»", q, dec.target)
		}
	}
}

func TestGuideRouteHardwareFallsBackToSearch(t *testing.T) {
	svc, _ := computerEnv(t)
	for _, q := range []string{"подобрать комп", "подбери системный блок", "что взять: компьютер"} {
		r := svc.RouteGuide(context.Background(), q)
		if r.Mode != routeSearch || r.OpenNow || r.OpenReason != "" || r.Offer != "" || r.Reason != routeHardware {
			t.Errorf("%q: %+v, ждали обычный поиск с причиной hardware", q, r)
		}
	}
	if r := svc.RouteGuide(context.Background(), "подбери антивирус на 20 компьютеров"); r.Mode != routeGuide || !r.OpenNow {
		t.Errorf("лицензии на 20 компьютеров — подбор: %+v", r)
	}
}

func TestGuideTurnHardwareOutOfScope(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	s.features.Hardware = true
	s.catalog.Store(newCatalogState(computerSnapshot(), 1))
	r := guideTurnReq(t, s, in.GuideRequest{Query: "подобрать комп"})
	if r.Type != out.GuideOutOfScope || r.Lead != guideHardwareLead || len(r.Offers) != 0 || r.Screen != nil {
		t.Fatalf("оборудование в подборе: %+v", r)
	}
	r = guideTurnReq(t, s, in.GuideRequest{Query: "подбери антивирус на 20 компьютеров"})
	if r.Type == out.GuideOutOfScope {
		t.Fatalf("антивирус на 20 компьютеров — не оборудование: %+v", r)
	}
}

func TestGuideShortWordNotProductPrefix(t *testing.T) {
	if n := guideNameScore("КОМПАС-3D V25", []string{"комп"}); n != 0 {
		t.Fatalf("«комп» совпал с «КОМПАС»: %d", n)
	}
	if n := guideNameScore("КОМПАС-3D V25", []string{"компас"}); n != 1 {
		t.Fatalf("«компас» — название: %d", n)
	}
	if got := guideProductTokens("подобрать комп"); len(got) != 0 {
		t.Fatalf("«комп» — объект, а не слово продукта: %v", got)
	}
}

// «Изменено» — только относительно уже показанного результата: ответ на первый вопрос условие задаёт, а не меняет.
func TestGuideScreenChangedNotOnFirstResult(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	r1 := guideTurnReq(t, s, in.GuideRequest{Query: "защита для компьютеров в офисе"})
	if r1.Type != out.GuideClarification {
		t.Fatalf("ход 1: ждали вопрос, получили %+v", r1.Type)
	}
	st := r1.State
	r2 := guideTurnReq(t, s, in.GuideRequest{Chip: &in.GuideChip{Key: "count:workstation", Value: "30"}, State: &st})
	if len(r2.Offers) == 0 || r2.Screen == nil {
		t.Fatalf("ход 2: ждали карточки: %+v", r2)
	}
	if r2.Screen.Changed != nil {
		t.Fatalf("ход 2 — первый результат: «Изменено» не показывается: %+v", r2.Screen.Changed)
	}
	st = r2.State
	r3 := guideTurnReq(t, s, in.GuideRequest{Message: "компьютеров будет 50", State: &st})
	if r3.Screen == nil || r3.Screen.Changed == nil || !strings.Contains(r3.Screen.Changed.Text, "30 → 50") {
		t.Fatalf("ход 3: смена числа мест: %+v", r3.Screen)
	}
}

// «чем расширенный отличается от стандартного»: цель — линейка, где обе редакции названы, а не первое по выдаче
// семейство другого вендора.
func TestTargetEditionCompare(t *testing.T) {
	_, st := computerEnv(t)
	rc := p6rc(st.lex)
	q := "антивирус в офисе на 30 пк чем расширенный отличается от стандартного"
	rc.ent.textTokens = strings.Fields(q)
	fams := p6fams(t, rc, domain.GroupMatch, "f-pro32", "f-kes-ext", "f-kes-std")
	dec := p6target(t, rc, q, "", fams)
	if dec.target == nil || !strings.Contains(dec.target.Title, "Kaspersky Endpoint Security") {
		t.Fatalf("цель: %+v, ждали линейку Kaspersky", dec.target)
	}
	rc.ent.textTokens = strings.Fields("антивирус расширенный стандартный")
	if fh := rc.editionCompare([]string{"антивирус", "расширенный", "стандартный"}, fams); fh != nil {
		t.Fatalf("без вопроса о различиях правило не действует: %s", fh.id)
	}
}

// Ступень из одного значения без отметки ступени у источника — пакет: цена позиции — за все N мест.
func TestPackTiers(t *testing.T) {
	n30, n1, lo, hi := 30, 1, 25, 49
	ps := []domain.Product{
		{ID: "pack", License: domain.License{Quantity: &n30, TierMin: &n30, TierMax: &n30}},
		{ID: "tier", License: domain.License{Quantity: &lo, TierMin: &lo, TierMax: &hi}},
		{ID: "step", License: domain.License{Quantity: &n30, TierMin: &n30, TierMax: &n30},
			Attributes: domain.JSONB(`{"Ступень количества":"Кол-во от 30 до 30 шт."}`)},
		{ID: "one", License: domain.License{Quantity: &n1, TierMin: &n1, TierMax: &n1}},
	}
	packTiers(ps)
	if ps[0].License.TierMin != nil || ps[0].License.Quantity == nil || *ps[0].License.Quantity != 30 {
		t.Fatalf("пакет на 30: %+v", ps[0].License)
	}
	if ps[1].License.TierMin == nil || ps[2].License.TierMin == nil || ps[3].License.TierMin == nil {
		t.Fatalf("ступени и поштучная позиция не меняются: %+v %+v %+v", ps[1].License, ps[2].License, ps[3].License)
	}
	plain := []domain.Product{{ID: "pack", License: domain.License{Quantity: &n30, TierMin: &n30, TierMax: &n30}}}
	packTiers(plain)
	if plain[0].License.TierMin == nil {
		t.Fatal("источник без отметок ступеней не трогается")
	}
}
