package services

import (
	"slices"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
)

// Кейсы QA-обхода: B1, B2, M3–M6. Словарь — mock-каталог.

func entitiesFor(t *testing.T, query string, asTyped bool) (normResult, queryAttrs, queryEntities) {
	t.Helper()
	lx, rules, _ := mockLexicon(t)
	nr := normalizeFull(query, lx, rules, asTyped)
	a := extractAttrs(nr.tokens)
	return nr, a, findEntities(nr, a, lx, rules)
}

// B1: витрина по умолчанию — live (B3 на действующих правилах); B0–B4 принимаются как раньше.
func TestQALiveConfig(t *testing.T) {
	for raw, want := range map[string]string{"": domain.ConfigLive, "live": domain.ConfigLive, "LIVE": domain.ConfigLive,
		"b4": domain.ConfigB4, "B3": domain.ConfigB3} {
		req := in.SearchRequest{Query: "зуп", Config: raw}
		cfg, _, err := validateSearch(&req)
		if err != nil || cfg != want {
			t.Fatalf("%q → %q, %v; want %q", raw, cfg, err, want)
		}
	}
	if slices.Contains(domain.SearchConfigs, domain.ConfigLive) {
		t.Fatal("live не входит в лестницу оценки B0–B4")
	}
	req := in.SearchRequest{Query: "зуп", Config: "B9"}
	if _, _, err := validateSearch(&req); err == nil {
		t.Fatal("неизвестная конфигурация должна давать 422")
	}
}

// B2: «win» — недописанное слово: не «непонятное», не «нет в каталоге», полное слово — расширение и подсказка.
func TestQAPrefixQuery(t *testing.T) {
	nr, _, e := entitiesFor(t, "win", false)
	if nr.prefixes["win"] != "windows" || !slices.Contains(nr.expansions, "windows") {
		t.Fatalf("префикс: %v / %v", nr.prefixes, nr.expansions)
	}
	if c := nr.correction; c == nil || c.applied || c.to != "windows" {
		t.Fatalf("подсказка «Возможно, вы искали: windows»: %+v", nr.correction)
	}
	if len(e.unknown) > 0 || e.absent() || len(e.families) == 0 {
		t.Fatalf("win: unknown=%v absent=%v families=%v", e.unknown, e.absent(), e.families)
	}
	// Как введено — без достраивания; недописанным считается только последнее слово при понятных остальных.
	if nr, _, _ := entitiesFor(t, "win", true); len(nr.prefixes) > 0 {
		t.Fatalf("как введено: %v", nr.prefixes)
	}
	if nr, _, _ := entitiesFor(t, "программа для монтажа видео", false); len(nr.prefixes) > 0 {
		t.Fatalf("«видео» в непонятном запросе не достраивается: %v", nr.prefixes)
	}
	// winrar — не начало слова каталога, отсутствующий товар остаётся отсутствующим.
	if _, _, e := entitiesFor(t, "winrar", false); !e.absent() {
		t.Fatal("winrar должен остаться «нет в каталоге»")
	}
}

// M5: «акад лт» — lt транслитом, сокращение «акад» не делает запрос отсутствующим товаром; смешанный язык.
func TestQATranslitAndMixedLanguage(t *testing.T) {
	nr, _, e := entitiesFor(t, "акад лт", false)
	if !slices.Contains(nr.tokens, "lt") || e.absent() || !slices.Contains(e.families, "autodesk-autocad-lt") {
		t.Fatalf("акад лт: tokens=%v absent=%v families=%v", nr.tokens, e.absent(), e.families)
	}
	nr, _, e = entitiesFor(t, "antivirus for small business", false)
	if !slices.Contains(nr.tokens, "антивирус") || e.latinUnknown || e.absent() {
		t.Fatalf("antivirus: tokens=%v unknown=%v absent=%v", nr.tokens, e.unknown, e.absent())
	}
	for _, q := range []string{"notion", "visio", "chatgpt plus", "solidworks"} {
		if _, _, e := entitiesFor(t, q, false); !e.absent() {
			t.Fatalf("%q должен остаться «нет в каталоге»", q)
		}
	}
}

// M4: категория по словам назначения; «российские» не съедает соседнее слово и не перебивает категорию.
func TestQACategoryByPurpose(t *testing.T) {
	for q, want := range map[string]string{
		"видеоконференции российские на 50 человек":                                                           "videoconf",
		"программа чтобы сотрудники могли созваниваться показывать экран человек на 50 желательно российская": "videoconf",
		"программа для чертежей":    "cad",
		"что-нибудь для бухгалтера": "business-apps",
		"удалёнка для офиса":        "remote",
	} {
		_, a, e := entitiesFor(t, q, false)
		if e.category != want || !e.categoryByWords {
			t.Fatalf("%q: категория %q (по словам %v), want %q", q, e.category, e.categoryByWords, want)
		}
		if e.absent() {
			t.Fatalf("%q: задача названа — не «нет в каталоге»", q)
		}
		if q == "видеоконференции российские на 50 человек" && (a.origin != "ru" || !slices.Contains(e.textTokens, "видеоконференции")) {
			t.Fatalf("origin=%q textTokens=%v", a.origin, e.textTokens)
		}
	}
	// Слово «сервера» — условие (серверная лицензия), а не категория «Серверные ОС».
	if _, _, e := entitiesFor(t, "kaspersky для сервера", false); e.category == "os-server" {
		t.Fatal("«для сервера» не должно сужать выдачу до серверных ОС")
	}
}

func qaRank(t *testing.T, query, segment string, rels map[string]float64) ([]*familyHit, []domain.RejectedHit) {
	t.Helper()
	lx, rules, _ := mockLexicon(t)
	nr := normalizeFull(query, lx, rules, false)
	a := extractAttrs(nr.tokens)
	e := findEntities(nr, a, lx, rules)
	rc := rankCtx{lx: lx, attrs: a, ent: e, purchased: map[string]bool{}, purchasedQty: map[string]int{},
		boughtVendors: map[string]bool{}, productRel: map[string]float64{}, refCats: map[string]bool{},
		business: isBusinessQuery(a, e, segment), textTop: map[string]bool{}}
	var rejected []domain.RejectedHit
	var fams []*familyHit
	for fid, rel := range rels {
		if fh := rc.buildFamily(fid, rel, &rejected); fh != nil {
			fams = append(fams, fh)
		}
	}
	fams = rc.dropOtherCategories(fams, &rejected)
	fams = rc.dropQuantityMisfits(fams, &rejected)
	for _, fh := range fams {
		rc.assignGroup(fh)
		rc.score(fh)
	}
	sortFamilies(fams)
	return rc.dropTail(fams, &rejected), rejected
}

// M3: на 1000 ПК и у клиента-организации бизнес-линейки выше домашних; «офис на 30 мест» — пакет на 30 мест
// выше поштучного Office на 1 ПК, и «офис» не делает Microsoft Office точным совпадением.
func TestQABusinessScale(t *testing.T) {
	home := map[string]float64{"kaspersky-internet-security": 1, "kaspersky-premium": 0.95, "kaspersky-endpoint-security": 0.6,
		"kaspersky-small-office-security": 0.55}
	fams, _ := qaRank(t, "касперский на 1000 пк бессрочно", "", home)
	if ids := familyIDs(fams); len(ids) < 2 || ids[0] != "kaspersky-endpoint-security" || ids[1] != "kaspersky-small-office-security" {
		t.Fatalf("на 1000 пк: %v", ids)
	}
	fams, _ = qaRank(t, "продлить антивирус", "smb", map[string]float64{"kaspersky-plus": 1, "kaspersky-small-office-security": 0.7})
	if ids := familyIDs(fams); ids[0] != "kaspersky-small-office-security" {
		t.Fatalf("клиент SMB: %v", ids)
	}
	fams, _ = qaRank(t, "продлить антивирус", "individual", map[string]float64{"kaspersky-plus": 1, "kaspersky-small-office-security": 0.7})
	if ids := familyIDs(fams); ids[0] != "kaspersky-plus" {
		t.Fatalf("частное лицо: %v", ids)
	}
	fams, _ = qaRank(t, "офис на 30 мест", "", map[string]float64{"office-2021": 1, "r7-office-professional": 0.8})
	if ids := familyIDs(fams); ids[0] != "r7-office-professional" || fams[1].group != domain.GroupMatch {
		t.Fatalf("офис на 30 мест: %v, группа Office %s", ids, fams[1].group)
	}
}

// M6: шум хвоста уходит в отказы с причиной; разметка в запросе — шум.
func TestQATailAndNoise(t *testing.T) {
	fams, rejected := qaRank(t, "винда 11 про", "", map[string]float64{"windows-11-pro": 1, "windows-10-pro": 0.9,
		"adobe-acrobat": 0.6})
	if slices.Contains(familyIDs(fams), "adobe-acrobat") {
		t.Fatalf("Acrobat Pro в хвосте «винда 11 про»: %v", familyIDs(fams))
	}
	if !slices.ContainsFunc(rejected, func(r domain.RejectedHit) bool { return r.Reason == rejectTail }) {
		t.Fatalf("отсечение хвоста без причины: %+v", rejected)
	}
	// Того же вендора не трогаем: «касперский» — и домашние, и бизнес-линейки.
	fams, _ = qaRank(t, "касперский", "", map[string]float64{"kaspersky-plus": 1, "kaspersky-endpoint-security": 0.3})
	if len(fams) != 2 {
		t.Fatalf("касперский: %v", familyIDs(fams))
	}
	for q, want := range map[string]bool{"<script>alert(1)</script>": true, "{{7*7}}": true, "касперский на 10 пк": false} {
		if looksLikeMarkup(q) != want {
			t.Fatalf("looksLikeMarkup(%q) != %v", q, want)
		}
	}
}
