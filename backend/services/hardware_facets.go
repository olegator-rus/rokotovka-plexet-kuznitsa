package services

import (
	"sort"
	"strconv"

	"kuznica/api/out"
)

// Фасеты категории: по attribute_definitions категории запроса (или первой позиции
// выдачи, если это оборудование). Границы числовых — по всей категории (ползунок не прыгает), число позиций у
// значения — среди позиций категории под остальными условиями запроса (без условия самого фасета).

func (p *hwPlan) facets(resp *out.SearchResponse) []out.Facet {
	res := []out.Facet{}
	cat := p.category
	if cat == "" && len(resp.Items) > 0 {
		if d, ok := p.lx.docs[resp.Items[0].ProductID]; ok && p.hx.hwCats[d.CategoryID] {
			cat = d.CategoryID
		}
	}
	if cat == "" {
		return res
	}
	ids := p.hx.products[cat]
	for _, key := range p.hx.catAttrs[cat] {
		a, ok := p.hx.attrs[key]
		if !ok || !a.facet {
			continue
		}
		f := out.Facet{Key: key, Label: a.label, Unit: a.unit, Type: a.typ, Values: []out.FacetValue{}}
		others := p.without(key)
		counts := map[string]int{}
		var order []string
		values := map[string]any{}
		for _, id := range ids {
			d := p.lx.docs[id]
			var v any
			var sv string
			if a.typ == "number" {
				x, ok := d.Specs[key]
				if !ok {
					continue
				}
				if f.Min == nil || x < *f.Min {
					f.Min = ptrF(x)
				}
				if f.Max == nil || x > *f.Max {
					f.Max = ptrF(x)
				}
				v, sv = x, strconv.FormatFloat(x, 'f', -1, 64)
			} else {
				sv = p.hx.docAttrs[id][key]
				if sv == "" {
					continue
				}
				v = sv
				if a.typ == "bool" {
					v = sv == "true"
				}
			}
			if _, seen := values[sv]; !seen {
				values[sv] = v
				order = append(order, sv)
			}
			if r, _ := others.reject(d); r == "" {
				counts[sv]++
			}
		}
		if len(order) == 0 {
			continue
		}
		if a.typ == "number" {
			sort.Slice(order, func(i, j int) bool { return values[order[i]].(float64) < values[order[j]].(float64) })
		} else if len(a.values) > 0 {
			rank := map[string]int{}
			for i, x := range a.values {
				rank[x] = i
			}
			sort.SliceStable(order, func(i, j int) bool { return rank[order[i]] < rank[order[j]] })
		} else {
			sort.Strings(order)
		}
		for _, sv := range order {
			f.Values = append(f.Values, out.FacetValue{Value: values[sv], Count: counts[sv]})
		}
		res = append(res, f)
	}
	return res
}

// without — копия плана без условия на key (для подсчёта значений фасета).
func (p *hwPlan) without(key string) *hwPlan {
	cp := *p
	cp.conds = nil
	for _, c := range p.conds {
		if c.Key != key {
			cp.conds = append(cp.conds, c)
		}
	}
	return &cp
}

func ptrF(x float64) *float64 { return &x }
