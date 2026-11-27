package services

import (
	"context"
	"slices"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
)

// «аналог фотошопа (на 20 пользователей)», «аналог фш», «замена фотошопа»: жаргон «фотошоп»/«фш» — Adobe Photoshop
// (не «ФотоШОУ» по опечатке); аналоги — тот же класс у других вендоров, когда справочник связей замен не знает;
// «аналог» — просьба о замене, а не «Аналоговые видеокамеры».

func photoSnapshot() *domain.CatalogSnapshot {
	b := domain.LicenseKindBase
	s := p6snapshot([]p6offer{
		{id: "ps1", fam: "f-ps", famName: "Adobe Photoshop", vendor: "adobe", cat: "gfx", title: "Adobe Photoshop CC для организаций", kind: b, price: 0},
		{id: "pe1", fam: "f-pse", famName: "Adobe Photoshop Elements", vendor: "adobe", cat: "photo", title: "Adobe Photoshop Elements 2026", kind: b, price: 9000},
		{id: "pl1", fam: "f-plug", famName: "Digital Film Tools Adobe Photoshop Plug-ins", vendor: "dft", cat: "photo", title: "Digital Film Tools Adobe Photoshop Plug-ins", kind: b, price: 5000},
		{id: "fs1", fam: "f-show", famName: "ФотоШОУ PRO", vendor: "ams", cat: "photo", title: "AMS Software ФотоШОУ PRO", kind: b, price: 2000},
		{id: "ac1", fam: "f-alive", famName: "AliveColors", vendor: "akvis", cat: "gfx", title: "АКВИС AliveColors Business", kind: b, price: 3000},
		{id: "co1", fam: "f-psp", famName: "Corel PaintShop Pro 2023", vendor: "corel", cat: "gfx", title: "Corel PaintShop Pro 2023", kind: b, price: 7000},
		{id: "pc1", fam: "f-addon", famName: "PROCAD ISOMETRIC", vendor: "procad", cat: "gfx", title: "PROCAD ISOMETRIC Add-on to Photoshop", kind: b, price: 7000},
		{id: "fc1", fam: "f-fc", famName: "Rubber Monkey FilmConvert Adobe Plugin", vendor: "rubber", cat: "photo", title: "Rubber Monkey FilmConvert Adobe Plugin", kind: b, price: 13775},
		{id: "cm1", fam: "f-cam", famName: "Dahua DH-HAC-HFW1231RP", vendor: "depo", cat: "cams", title: "Аналоговая видеокамера Dahua DH-HAC-HFW1231RP", price: 4000},
		{id: "cm2", fam: "f-cam2", famName: "HiWatch DS-T203L", vendor: "inferit", cat: "cams", title: "Камера видеонаблюдения аналоговая HiWatch DS-T203L", price: 3000},
	})
	g, v := "gfxroot", "video"
	s.Categories = append(s.Categories,
		domain.Category{ID: g, Name: "Программное обеспечение для графики и дизайна"},
		domain.Category{ID: "gfx", Name: "Графические редакторы", ParentID: &g},
		domain.Category{ID: "photo", Name: "Программы для обработки цифрового фото", ParentID: &g},
		domain.Category{ID: v, Name: "Видеонаблюдение"},
		domain.Category{ID: "cams", Name: "Аналоговые видеокамеры", ParentID: &v},
	)
	for i := range s.Products {
		if s.Products[i].CategoryID == "cams" {
			s.Products[i].Kind = domain.KindHardware
		}
	}
	return s
}

func photoEnv(t *testing.T) (*Services, *catalogState) {
	t.Helper()
	svc := NewServices(nil, Options{Features: Features{Hardware: true}})
	svc.SetLexicon([]domain.LexiconEntry{
		{Alias: "фотошоп", Canonical: "photoshop", Kind: domain.LexiconJargon, Target: domain.AliasTargetTerm},
		{Alias: "фш", Canonical: "photoshop", Kind: domain.LexiconJargon, Target: domain.AliasTargetTerm},
	})
	st := newCatalogState(photoSnapshot(), 1)
	svc.catalog.Store(st)
	return svc, st
}

func photoEntities(svc *Services, st *catalogState, q string) (normResult, queryAttrs, queryEntities) {
	rules := svc.rulesFor(context.Background(), domain.ConfigB3, st.lex, nil)
	nr := normalizeFull(q, st.lex, rules, false)
	attrs := extractAttrs(nr.tokens)
	ent := findEntities(nr, attrs, st.lex, rules)
	return nr, attrs, ent
}

func TestPhotoshopJargon(t *testing.T) {
	svc, st := photoEnv(t)
	for _, q := range []string{"фотошоп", "фотошопа", "фш"} {
		nr, _, ent := photoEntities(svc, st, q)
		if got := strings.Join(nr.tokens, " "); got != "photoshop" || nr.correction != nil && nr.correction.applied {
			t.Errorf("%q: %q (исправление %+v), ждали photoshop", q, got, nr.correction)
		}
		if !slices.Contains(ent.families, "f-ps") || slices.Contains(ent.families, "f-show") {
			t.Errorf("%q: семейства %v, ждали Adobe Photoshop", q, ent.families)
		}
	}
}

func TestAnalogOfPhotoshopIsClass(t *testing.T) {
	svc, st := photoEnv(t)
	for _, q := range []string{"аналог фотошопа", "аналог фш", "замена фотошопа", "альтернатива фотошопу", "аналог фотошопа на 20 пользователей"} {
		_, attrs, ent := photoEntities(svc, st, q)
		if !attrs.analog || !slices.Equal(ent.reference, []string{"f-ps"}) {
			t.Errorf("%q: аналог %v, референс %v — ждали только Adobe Photoshop", q, attrs.analog, ent.reference)
			continue
		}
		if !confidentReference(st.lex, ent) {
			t.Errorf("%q: референс назван продуктом", q)
		}
		got := classAnalogs(st.lex, ent.reference, ent.families, ent.entityTokens)
		for _, want := range []string{"f-alive", "f-psp", "f-show"} {
			if !slices.Contains(got, want) {
				t.Errorf("%q: нет аналога %s в %v", q, want, got)
			}
		}
		for _, bad := range []string{"f-ps", "f-pse", "f-plug", "f-addon", "f-fc", "f-cam", "f-cam2"} {
			if slices.Contains(got, bad) {
				t.Errorf("%q: %s — не аналог (тот же вендор, дополнение или другой класс): %v", q, bad, got)
			}
		}
		if ent.category == "cams" || ent.intent != nil && slices.Contains(ent.intent.cats, "cams") {
			t.Errorf("%q: «аналог» понят как «Аналоговые видеокамеры»", q)
		}
	}
	_, attrs, _ := photoEntities(svc, st, "аналог фотошопа на 20 пользователей")
	if attrs.quantity != 20 || attrs.metric != domain.MetricUser {
		t.Errorf("«на 20 пользователей» — условие количества: %d %q", attrs.quantity, attrs.metric)
	}
	// Референс угадан по одному общему слову — класс не строится.
	_, _, ent := photoEntities(svc, st, "аналог plug-ins")
	if confidentReference(st.lex, ent) && len(ent.reference) > 0 && ent.reference[0] != "f-plug" {
		t.Errorf("слабый референс: %v", ent.reference)
	}
}

func TestAnalogTargetIsReferenceCategory(t *testing.T) {
	svc, st := photoEnv(t)
	_, attrs, ent := photoEntities(svc, st, "замена фотошопа")
	rc := p6rc(st.lex)
	rc.attrs, rc.ent = attrs, ent
	fams := p6fams(t, rc, domain.GroupAnalog, "f-alive", "f-psp", "f-show")
	dec := p6target(t, rc, "замена photoshop", "gfxroot", fams)
	if dec.target == nil || dec.target.Type != out.TargetCategory || dec.target.Title != "Графические редакторы" {
		t.Fatalf("цель: %+v, ждали категорию названного продукта", dec.target)
	}
}

func TestAnalogWordIsNotAnalogCamera(t *testing.T) {
	svc, st := photoEnv(t)
	if c := st.hw.categoryOfWord("аналог"); c != "" {
		t.Fatalf("«аналог» → категория оборудования %q", c)
	}
	for _, q := range []string{"аналоговые видеокамеры", "аналоговая камера"} {
		rules := svc.rulesFor(context.Background(), domain.ConfigB3, st.lex, nil)
		nr := normalizeFull(q, st.lex, rules, false)
		attrs := extractAttrs(nr.tokens)
		p := svc.hardwareParse(context.Background(), st, nr, &attrs, in.SearchRequest{Query: q})
		if p == nil || p.category != "cams" {
			t.Errorf("%q: категория %v, ждали «Аналоговые видеокамеры»", q, p)
		}
	}
}

// Реранкер аналогов по классу видит названный продукт и его класс, а не «на 20 пользователей».
func TestClassAnalogQuery(t *testing.T) {
	_, st := photoEnv(t)
	if got := classAnalogQuery(st.lex, []string{"f-ps"}); got != "аналог Adobe Photoshop. Графические редакторы" {
		t.Fatalf("запрос реранкера: %q", got)
	}
}
