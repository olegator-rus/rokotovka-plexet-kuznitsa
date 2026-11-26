package services

import (
	"strconv"
	"testing"

	"kuznica/api/domain"
)

// TestEmbedTargets — маленький каталог эмбеддится целиком, большой — по одной позиции на семейство: в наличии
// с ценой, затем базовая лицензия.
func TestEmbedTargets(t *testing.T) {
	small := []indexDoc{{ProductID: "a", FamilyID: "f"}, {ProductID: "b", FamilyID: "f"}}
	if got, scope := embedTargets(small); scope != EmbedScopeAll || len(got) != 2 {
		t.Fatalf("маленький каталог: %v %s", got, scope)
	}
	yes, price := true, 10.0
	docs := make([]indexDoc, 0, embedAllMax+3)
	docs = append(docs,
		indexDoc{ProductID: "support", FamilyID: "astra", LicenseKind: domain.LicenseKindSupport},
		indexDoc{ProductID: "base-avail", FamilyID: "astra", LicenseKind: domain.LicenseKindBase, Available: &yes, PriceRub: &price},
		indexDoc{ProductID: "base", FamilyID: "astra", LicenseKind: domain.LicenseKindBase},
	)
	for i := 0; len(docs) <= embedAllMax; i++ {
		docs = append(docs, indexDoc{ProductID: "x" + strconv.Itoa(i), FamilyID: "fam-" + strconv.Itoa(i/2)})
	}
	got, scope := embedTargets(docs)
	if scope != EmbedScopeFamily {
		t.Fatalf("большой каталог: охват %s", scope)
	}
	if docs[got[0]].ProductID != "base-avail" {
		t.Errorf("семейство представляет позиция в наличии с ценой, а не %s", docs[got[0]].ProductID)
	}
	families := map[string]bool{}
	for _, i := range got {
		if families[docs[i].FamilyID] {
			t.Fatalf("семейство %s дважды", docs[i].FamilyID)
		}
		families[docs[i].FamilyID] = true
	}
}

// TestQuantityTier — ступень цены за единицу фида закрывает количество внутри ступени.
func TestQuantityTier(t *testing.T) {
	q, lo, hi := 100, 100, 149
	dev := domain.MetricDevice
	d := indexDoc{LicenseQuantity: &q, LicenseMetric: &dev, LicenseTierMin: &lo, LicenseTierMax: &hi}
	for n, want := range map[int]bool{100: true, 120: true, 149: true, 150: false, 99: false} {
		if got := quantityOK(d, n); got != want {
			t.Errorf("%d узлов в ступени 100–149: %v", n, got)
		}
	}
}
