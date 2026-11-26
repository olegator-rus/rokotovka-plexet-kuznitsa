package services

import (
	"sort"

	"kuznica/api/out"
)

// Уточнения (refinements, §11.5; RESEARCH Г): источник «значения фасетов верхних кандидатов».
// Один и тот же для чипов «+» над выдачей и бейджей панели (трек p3-instant берёт refinementsFromDocs). Каждое
// уточнение — структурированное условие (фильтр выдачи), проверенное числом результатов: в пустоту и
// не сужающие выдачу не предлагаются. Без LLM и векторов.

// maxRefinements — 4–6 на десктопе (мобильный показывает первые 4).
const maxRefinements = 6

// refinementsPerField — не больше двух значений одного поля: ряд остаётся разнообразным.
const refinementsPerField = 2

// refineFields — поля уточнений в порядке важности для закупки лицензии.
var refineFields = []string{facetLicType, facetDuration, facetMetric, facetReestr, facetDelivery}

// refinementsFromFamilies — уточнения по допустимым вариантам семейств выдачи.
func refinementsFromFamilies(lx *searchLexicon, query string, fams []*familyHit, taken map[string]bool, limit int) []out.Refinement {
	var docs []indexDoc
	for _, fh := range fams {
		docs = append(docs, fh.heads...)
	}
	return refinementsFromDocs(lx, query, docs, taken, limit)
}

// refinementsFromDocs — уточнения по набору позиций: значение поля, которое есть не у всех позиций. taken —
// поля, уже названные в запросе или выбранные фильтром. hits — число семейств с этим значением.
func refinementsFromDocs(lx *searchLexicon, query string, docs []indexDoc, taken map[string]bool, limit int) []out.Refinement {
	res := []out.Refinement{}
	if len(docs) < 2 {
		return res
	}
	for _, key := range refineFields {
		if taken[key] {
			continue
		}
		type cand struct {
			key, label string
			variants   int
			families   map[string]bool
		}
		byVal := map[string]*cand{}
		for _, d := range docs {
			sk, _, label, ok := facetValueOf(lx, key, d)
			if !ok || key == facetReestr && sk != "true" {
				continue
			}
			c := byVal[sk]
			if c == nil {
				c = &cand{key: sk, label: label, families: map[string]bool{}}
				byVal[sk] = c
			}
			c.variants++
			c.families[d.FamilyID] = true
		}
		var cs []*cand
		for _, c := range byVal {
			if c.variants > 0 && c.variants < len(docs) {
				cs = append(cs, c)
			}
		}
		sort.Slice(cs, func(i, j int) bool {
			if len(cs[i].families) != len(cs[j].families) {
				return len(cs[i].families) > len(cs[j].families)
			}
			return cs[i].key < cs[j].key
		})
		if len(cs) > refinementsPerField {
			cs = cs[:refinementsPerField]
		}
		for _, c := range cs {
			field, value, op := key, c.key, "eq"
			label := c.label
			if key == facetReestr {
				label = "В реестре ПО РФ"
			}
			res = append(res, out.Refinement{Label: label, Query: joinQuery(query, label), Field: &field, Value: &value,
				Op: &op, Hits: len(c.families)})
		}
	}
	if len(res) > limit {
		res = res[:limit]
	}
	return res
}
