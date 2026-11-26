package services

import (
	"sort"

	"kuznica/api/out"
)

// Точный артикул в плоской выдаче: первым — сам SKU, под ним до трёх вариантов того же
// семейства, которые отличаются от него одним условием (соседняя ступень количества, другой срок, другой вид
// лицензии), остальные позиции семейства — вложенными вариантами карточки SKU, а не двадцатью карточками подряд.
// Позиция без условий лицензии (медиапак, документация) соседом SKU со ступенью не считается.

// skuNeighbourMax — сколько вариантов показывается отдельными карточками под SKU.
const skuNeighbourMax = 3

// trimSKUFamily — карточки семейства SKU: SKU, до трёх соседей, остальное — во вложенные варианты SKU.
func trimSKUFamily(lx *searchLexicon, items []out.SearchItem, sku indexDoc) []out.SearchItem {
	at := -1
	var same []out.SearchItem
	var rest []out.SearchItem
	for _, it := range items {
		switch {
		case it.ProductID == sku.ProductID:
			at = len(rest)
			rest = append(rest, it)
		case it.FamilyID == sku.FamilyID:
			same = append(same, it)
		default:
			rest = append(rest, it)
		}
	}
	if at < 0 || len(same) <= skuNeighbourMax {
		return items
	}
	type cand struct {
		it    out.SearchItem
		diff  int // сколько условий отличается
		dim   int // какое: 0 — ступень, 1 — срок, 2 — вид
		dist  int // для ступени — насколько далеко
		order int
	}
	var cs []cand
	for i, it := range same {
		d, ok := lx.docs[it.ProductID]
		if !ok {
			continue
		}
		c := cand{it: it, order: i}
		if (sku.LicenseTierMin != nil) != (d.LicenseTierMin != nil) {
			c.diff = 9 // позиция без условий лицензии (медиапак) — не сосед
		}
		if intVal(d.LicenseTierMin) != intVal(sku.LicenseTierMin) {
			c.diff++
			c.dim = 0
			c.dist = abs(intVal(d.LicenseTierMin) - intVal(sku.LicenseTierMin))
		}
		if intVal(d.LicenseDuration) != intVal(sku.LicenseDuration) {
			c.diff++
			c.dim = 1
		}
		if d.LicenseKind != sku.LicenseKind {
			c.diff++
			c.dim = 2
		}
		if d.LicenseOrgType != sku.LicenseOrgType {
			c.diff += 2
		}
		cs = append(cs, c)
	}
	sort.SliceStable(cs, func(i, j int) bool {
		if cs[i].diff != cs[j].diff {
			return cs[i].diff < cs[j].diff
		}
		if cs[i].dim != cs[j].dim {
			return cs[i].dim < cs[j].dim
		}
		if cs[i].dist != cs[j].dist {
			return cs[i].dist < cs[j].dist
		}
		return cs[i].order < cs[j].order
	})
	// Сначала по одному соседу на каждое условие, затем ближайшие.
	var shown []out.SearchItem
	used := map[string]bool{}
	dimUsed := map[int]bool{}
	for _, c := range cs {
		if len(shown) < skuNeighbourMax && c.diff == 1 && !dimUsed[c.dim] {
			dimUsed[c.dim] = true
			used[c.it.ProductID] = true
			shown = append(shown, c.it)
		}
	}
	for _, c := range cs {
		if len(shown) < skuNeighbourMax && c.diff < 9 && !used[c.it.ProductID] {
			used[c.it.ProductID] = true
			shown = append(shown, c.it)
		}
	}
	head := rest[at]
	for _, c := range cs {
		if !used[c.it.ProductID] {
			v := c.it
			v.Variants, v.MatchType, v.Explain = nil, "", ""
			head.Variants = append(head.Variants, v)
		}
	}
	for i := range shown {
		shown[i].MatchType, shown[i].Explain = out.MatchVariant, explainVariant
	}
	res := append([]out.SearchItem{}, rest[:at]...)
	res = append(res, head)
	res = append(res, shown...)
	return append(res, rest[at+1:]...)
}

func intVal(p *int) int {
	if p == nil {
		return 0
	}
	return *p
}
