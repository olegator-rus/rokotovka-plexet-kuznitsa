package services

import (
	"slices"
	"strings"
	"testing"

	"kuznica/api/domain"
)

// Смена раскладки не превращает букву в знак препинания: «кибер бэкаб» → «кибер ,"rf,» (дальше — по «rf»), «доктор
// вэб» → «доктор d",». И наоборот: латинское слово со знаком, который в русской раскладке — буква («dt,» = «веб»),
// не считается известным по одному «dt».
func TestLayoutDoesNotMakePunctuation(t *testing.T) {
	for _, c := range []struct {
		from string
		want bool
	}{{"бэкаб", true}, {"вэб", true}, {"вебка", true}, {"бух", true}, {"доктор", false}, {"касперский", false}} {
		to, ok := switchLayout(c.from)
		if !ok {
			t.Fatalf("%q не переводится", c.from)
		}
		if got := layoutPunct(c.from, to); got != c.want {
			t.Errorf("%q → %q: знак %v, ждали %v", c.from, to, got, c.want)
		}
	}
	for in, want := range map[string]bool{"dt,": true, ",e[ufknthbz": true, "dr.web": false, "office": false, "kaspersky,": true, "1c": false} {
		if got := layoutLetterPunct(in); got != want {
			t.Errorf("layoutLetterPunct(%q) = %v, ждали %v", in, got, want)
		}
	}
	snap := p6snapshot([]p6offer{
		{id: "cb", fam: "f-cb", famName: "Кибер Бэкап", vendor: "cyberprotect", cat: "backup", title: "Кибер Бэкап Стандартная", kind: "base", price: 1000},
		{id: "rf", fam: "f-rf", famName: "RF Explorer", vendor: "cyberprotect", cat: "comm", title: "RF Explorer", kind: "base", price: 1000},
		{id: "dw", fam: "f-dw", famName: "Доктор Веб Desktop", vendor: "cyberprotect", cat: "av", title: "Доктор Веб Desktop Security", kind: "base", price: 1000},
		{id: "dt", fam: "f-dt", famName: "DT Label", vendor: "cyberprotect", cat: "comm", title: "DT Label Printer", kind: "base", price: 1000},
	})
	lx := newSearchLexicon(snap)
	for in, want := range map[string]string{"кибер бэкаб": "кибер бэкаб", "доктор вэб": "доктор вэб", "ljrnjh dt,": "доктор веб"} {
		got := strings.Join(fixLayoutWith(strings.Fields(in), lx, nil, nil), " ")
		if got != want {
			t.Errorf("%q → %q, ждали %q", in, got, want)
		}
	}
}

// Голова семейства — не сборка под отдельную платформу, если платформу не назвали («астра линукс» → x86-64).
func TestHeadNotOtherPlatform(t *testing.T) {
	rc := rankCtx{}
	rc.ent.textTokens = []string{"astra", "linux"}
	elbrus := "Astra Linux Special Edition на базе п/а Эльбрус, для аппаратных платформ Эльбрус-8С"
	x86 := "Astra Linux Special Edition 1.8, х86-64, «Максимальный»"
	if !rc.otherPlatform(elbrus) || rc.otherPlatform(x86) || rc.otherPlatform("Armor Defender") {
		t.Fatal("платформа без запроса")
	}
	if less, ok := rc.headLess(indexDoc{Title: x86}, indexDoc{Title: elbrus}); !ok || !less {
		t.Fatal("x86-64 должна быть головой")
	}
	rc.ent.textTokens = []string{"astra", "linux", "эльбрус"}
	if rc.otherPlatform(elbrus) {
		t.Fatal("платформа названа — сборка под неё годится")
	}
}

// Просьба сравнить — не просьба о замене (модель отвечала find_alternative на «сравнить kes …»).
func TestQueryCompares(t *testing.T) {
	for q, want := range map[string]bool{"сравнить kes стандартный и расширенный": true, "чем расширенный отличается от стандартного": true,
		"разница между редакциями": true, "аналог фотошопа": false, "kaspersky endpoint security": false} {
		if got := queryCompares(strings.Fields(q)); got != want {
			t.Errorf("%q: %v", q, got)
		}
	}
}

// Названный продукт не уходит в отказы по количеству: «алд про на 50 пользователей» — ALD Pro без числа мест в
// фиде остаётся, хотя чужое «… Pro» поштучно закрывает 50.
func TestNamedFamilyKeptOnQuantity(t *testing.T) {
	one, user := 1, domain.MetricUser
	ald := &familyHit{id: "ald", head: indexDoc{ProductID: "a1"}, heads: []indexDoc{{ProductID: "a1"}}}
	dim := &familyHit{id: "dim", head: indexDoc{ProductID: "d1", LicenseQuantity: &one, LicenseMetric: &user, LicenseType: "per_unit"}}
	dim.heads = []indexDoc{dim.head}
	rc := rankCtx{}
	rc.attrs.quantity, rc.attrs.metric = 50, domain.MetricUser
	rc.ent.named = []string{"ald"}
	var rej []domain.RejectedHit
	got := rc.dropQuantityMisfits([]*familyHit{ald, dim}, &rej)
	if len(got) == 0 || got[0].id != "ald" {
		t.Fatalf("названный ALD Pro ушёл в отказы: %v", rej)
	}
}

// «astra linux» — семейство с обоими словами запроса, а не «Astra Pro» с редким весом одного «astra».
func TestPreferQueryWords(t *testing.T) {
	snap := p6snapshot([]p6offer{
		{id: "ap", fam: "f-ap", famName: "Astra Pro", vendor: "inferit", cat: "os", title: "Astra Pro базовая", kind: "base", price: 1000},
		{id: "se", fam: "f-se", famName: "Astra Linux Special Edition", vendor: "astralinux", cat: "os", title: "Astra Linux Special Edition 1.8", kind: "base", price: 1000},
	})
	lx := newSearchLexicon(snap)
	toks := []string{"astra", "linux"}
	if got := lx.preferQueryWords([]string{"f-ap"}, []string{"f-ap", "f-se"}, toks); !slices.Equal(got, []string{"f-se"}) {
		t.Fatalf("названное: %v", got)
	}
	if got := lx.preferQueryWords([]string{"f-ap"}, []string{"f-ap", "f-se"}, []string{"astra", "pro"}); !slices.Equal(got, []string{"f-ap"}) {
		t.Fatalf("«astra pro»: %v", got)
	}
}

// Назван программный продукт без признаков оборудования — сканеры штрихкодов по слову «сканер» уходят.
func TestDropHardwareForNamedSoftware(t *testing.T) {
	snap := p6snapshot([]p6offer{
		{id: "sv", fam: "f-sv", famName: "Эшелон «СКАНЕР-ВС»", vendor: "inferit", cat: "comm", title: "СКАНЕР-ВС 7", kind: "base", price: 690000},
		{id: "hw", fam: "f-hw", famName: "Сканер штрихкодов Honeywell", vendor: "depo", cat: "srv", title: "Сканер штрихкодов Honeywell Voyager", price: 19881},
	})
	for i := range snap.Products {
		if snap.Products[i].ID == "hw" {
			snap.Products[i].Kind = domain.KindHardware
		}
	}
	lx := newSearchLexicon(snap)
	rc := p6rc(lx)
	fams := p6fams(t, rc, "", "f-sv", "f-hw")
	ent := queryEntities{families: []string{"f-sv", "f-hw"}, entityTokens: []string{"сканер-вс"}}
	var rej []domain.RejectedHit
	got := dropHardwareForNamedSoftware(lx, ent, fams, &rej)
	if len(got) != 1 || got[0].id != "f-sv" {
		t.Fatalf("осталось %d семейств, ждали только СКАНЕР-ВС", len(got))
	}
}

// Продление: среди найденного нет позиции названного — у названного продления нет (FastReport ≠ Astra Linux).
func TestNamedHit(t *testing.T) {
	snap := p6snapshot([]p6offer{
		{id: "se", fam: "f-se", famName: "Astra Linux Special Edition", vendor: "astralinux", cat: "os", title: "Astra Linux SE", kind: "base", price: 1000},
		{id: "fr", fam: "f-fr", famName: "FastReport VCL", vendor: "inferit", cat: "os", title: "FastReport VCL продление", kind: "renewal", price: 1000},
	})
	lx := newSearchLexicon(snap)
	ent := queryEntities{families: []string{"f-se"}}
	other := retrieval{step: domain.RetrievalStep{Top: []domain.RankedHit{{ProductID: "fr"}}}}
	if namedHit(lx, ent, other) {
		t.Fatal("FastReport — не названный продукт")
	}
	own := retrieval{step: domain.RetrievalStep{Top: []domain.RankedHit{{ProductID: "se"}}}}
	if !namedHit(lx, ent, other, own) {
		t.Fatal("Astra Linux найден")
	}
}
