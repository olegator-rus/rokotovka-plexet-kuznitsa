package services

import (
	"encoding/json"
	"strings"
	"testing"

	"kuznica/api/domain"
)

func TestLicenseLine(t *testing.T) {
	ptr := func(v int) *int { return &v }
	str := func(v string) *string { return &v }
	cases := []struct {
		doc  indexDoc
		want string
	}{
		{indexDoc{LicenseType: "subscription", LicenseMetric: str("device"), LicenseQuantity: ptr(10), LicenseDuration: ptr(12)}, "Подписка · 10 устройств · 1 год"},
		{indexDoc{LicenseType: "subscription", LicenseMetric: str("user"), LicenseQuantity: ptr(2), LicenseDuration: ptr(24)}, "Подписка · 2 пользователя · 2 года"},
		{indexDoc{LicenseType: "perpetual", LicenseMetric: str("user"), LicenseQuantity: ptr(21)}, "Бессрочная · 21 пользователь"},
		{indexDoc{LicenseType: "perpetual", LicenseMetric: str("org")}, "Бессрочная · на организацию"},
		{indexDoc{LicenseType: "renewal", LicenseMetric: str("core"), LicenseQuantity: ptr(16), LicenseDuration: ptr(6)}, "Продление · 16 ядер · 6 месяцев"},
		{indexDoc{LicenseType: "cloud", LicenseDuration: ptr(60)}, "Облачная · 5 лет"},
		{indexDoc{}, ""},
	}
	for _, c := range cases {
		if got := licenseLine(c.doc); got != c.want {
			t.Fatalf("%+v → %q, ожидалось %q", c.doc, got, c.want)
		}
	}
}

func TestPlural(t *testing.T) {
	forms := [3]string{"устройство", "устройства", "устройств"}
	for n, want := range map[int]string{1: "устройство", 2: "устройства", 5: "устройств", 11: "устройств", 14: "устройств",
		21: "устройство", 22: "устройства", 25: "устройств", 111: "устройств", 101: "устройство"} {
		if got := plural(n, forms); got != want {
			t.Fatalf("%d → %s, ожидалось %s", n, got, want)
		}
	}
}

// renewalLexicon — каталог, как в фиде: вектор у одной позиции семейства (представитель), продления — отдельные
// позиции. У «drweb» и «kasp» представитель — новая лицензия, у «maxp» — продление, у «nod» продлений нет.
func renewalLexicon() *searchLexicon {
	docs := map[string]indexDoc{}
	lx := &searchLexicon{docs: docs, families: map[string][]string{}}
	add := func(pid, fid, licType string) {
		docs[pid] = indexDoc{ProductID: pid, FamilyID: fid, LicenseType: licType}
		lx.families[fid] = append(lx.families[fid], pid)
	}
	add("drweb-new", "drweb", domain.LicenseSubscription)
	add("drweb-ren-1", "drweb", domain.LicenseRenewal)
	add("drweb-ren-2", "drweb", domain.LicenseRenewal)
	add("kasp-new", "kasp", domain.LicenseSubscription)
	add("kasp-ren", "kasp", domain.LicenseRenewal)
	add("maxp-ren", "maxp", domain.LicenseRenewal)
	add("nod-new", "nod", domain.LicensePerpetual)
	return lx
}

func rankedHits(ids ...string) []domain.RankedHit {
	res := make([]domain.RankedHit, len(ids))
	for i, id := range ids {
		res[i] = domain.RankedHit{ProductID: id, Rank: i + 1, Score: 0.8 - 0.01*float64(i)}
	}
	return res
}

// «продлить антивирус»: условие «продление» в векторный список не идёт — вектор один на семейство, и с этим
// условием в списке оставались только семейства, чей представитель случайно оказался продлением.
func TestDenseFiltersWithoutRenewal(t *testing.T) {
	ent := queryEntities{vendors: []string{"drweb"}}
	renew := extractAttrs(strings.Fields("продлить антивирус"))
	if !renew.renewal {
		t.Fatal("продление не разобрано")
	}
	text, _ := json.Marshal(strictFilters(renew, ent))
	dense, _ := json.Marshal(denseFilters(renew, ent, nil))
	if !strings.Contains(string(text), `"license_type":"renewal"`) || strings.Contains(string(dense), "license_type") {
		t.Fatalf("текст — с условием продления, векторы — без: %s / %s", text, dense)
	}
	if !strings.Contains(string(dense), `"vendor_id":["drweb"]`) {
		t.Fatalf("условия уровня семейства остаются: %s", dense)
	}
	if !renew.renewal {
		t.Fatal("условия запроса не меняются")
	}
	// Контрпример: «антивирус» без «продлить» — условия векторного списка те же, что у текстового.
	plain := extractAttrs(strings.Fields("антивирус"))
	text, _ = json.Marshal(strictFilters(plain, ent))
	dense, _ = json.Marshal(denseFilters(plain, ent, nil))
	if plain.renewal || string(text) != string(dense) {
		t.Fatalf("без продления условия одинаковы: %s / %s", text, dense)
	}
}

// Векторный список запроса с продлением — позиции-продления найденных семейств: ранг семейства складывается с
// текстовым у той позиции, которую нашёл текст; семейство без продлений уходит.
func TestRenewalDenseMapsFamilies(t *testing.T) {
	lx := renewalLexicon()
	bm25 := rankedHits("drweb-ren-2", "drweb-ren-1", "kasp-ren", "maxp-ren")
	got := renewalDense(lx, rankedHits("drweb-new", "nod-new", "kasp-new", "maxp-ren", "ghost"), bm25)
	want := []string{"drweb-ren-2", "kasp-ren", "maxp-ren"}
	if len(got) != len(want) {
		t.Fatalf("список: %+v", got)
	}
	for i, h := range got {
		if h.ProductID != want[i] || h.Rank != i+1 {
			t.Fatalf("место %d: %+v, ожидалось %s", i+1, h, want[i])
		}
	}
	if got[0].Score != 0.8 || got[1].Score != 0.78 {
		t.Fatalf("скор — скор представителя семейства: %+v", got)
	}
	// Текст семейство не нашёл: его представляет первое продление (или сам представитель-продление).
	got = renewalDense(lx, rankedHits("kasp-new", "maxp-ren", "drweb-ren-1"), nil)
	if len(got) != 3 || got[0].ProductID != "kasp-ren" || got[1].ProductID != "maxp-ren" || got[2].ProductID != "drweb-ren-1" {
		t.Fatalf("без текстового списка: %+v", got)
	}
	if got := renewalDense(lx, nil, bm25); got == nil || len(got) != 0 {
		t.Fatalf("пустой список остаётся пустым: %+v", got)
	}
}

// Промах «продлить антивирус»: в векторном списке под условием «продление» были две случайные позиции, и вторая из
// них (24-я по тексту) после слияния вставала первой. Со списком семейств первой остаётся найденная текстом и
// смыслом, а позиция из хвоста обоих списков — ниже.
func TestRenewalDenseFusionOrder(t *testing.T) {
	lx := renewalLexicon()
	ids := []string{"drweb-ren-1", "kasp-ren"}
	for i := 0; i < 21; i++ {
		ids = append(ids, "filler-"+string(rune('a'+i)))
	}
	bm25 := rankedHits(append(ids, "maxp-ren")...) // maxp-ren — 24-я по тексту
	old := rrf(b3RRFK, bm25, rankedHits("other-ren", "maxp-ren"))
	if old[0].ProductID != "maxp-ren" {
		t.Fatalf("причина промаха воспроизводится на списке из двух позиций: %s", old[0].ProductID)
	}
	dense := renewalDense(lx, rankedHits("drweb-new", "kasp-new", "nod-new", "maxp-ren"), bm25)
	fusedList := rrf(b3RRFK, bm25, dense)
	if fusedList[0].ProductID != "drweb-ren-1" || fusedList[1].ProductID != "kasp-ren" {
		t.Fatalf("первые — найденные текстом и смыслом: %s, %s", fusedList[0].ProductID, fusedList[1].ProductID)
	}
	for i, f := range fusedList {
		if f.ProductID == "maxp-ren" && i < 2 {
			t.Fatalf("позиция из хвоста текстового списка не первая: место %d", i+1)
		}
	}
}
