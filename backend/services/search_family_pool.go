package services

import (
	"context"

	"kuznica/api/domain"
)

// Глубина кандидатов по семействам. На фиде семейство — это сотни офферов (Kaspersky Symphony,
// Dr.Web), и первые 50 мест BM25 занимают два-три таких семейства: «продлить касперский на 100 пк на 2 года» не
// доставал KES вовсе, хотя у него есть продление ровно под эти условия. Вектор здесь не помогает — он один на
// семейство и в kNN с фильтром продления не попадает.
//
// Решение — дедупликация по семейству до слияния, только для запроса «вендор + условия покупки» (там текст —
// одно имя вендора и офферы по нему равны; при названном продукте BM25 различает семейства сам, и ранги не
// трогаются — иначе меняются цели «365» и «1с на 5 пользователей»): если в первых местах мало разных семейств, текстовый запрос
// повторяется со сворачиванием по family_id (лучший оффер семейства), и ранги BM25 в слиянии становятся рангами
// семейств — как у векторного списка, где одно семейство и так одно место. Остальные офферы исходного списка
// идут следом: их релевантность остаётся для выбора варианта внутри семейства, места других семейств они больше не
// занимают. Какой вариант станет головой, по-прежнему решает buildFamily по всем офферам семейства.

// familyPoolMinShare — если разных семейств в полном списке меньше этой доли мест, запрос повторяется по семействам.
const familyPoolMinShare = 0.5

// familyDominated — список полон, а разных семейств в нём меньше familyPoolMinShare мест.
func familyDominated(r retrieval) bool {
	if len(r.step.Top) < candidatesTop {
		return false
	}
	fams := map[string]bool{}
	for _, h := range r.step.Top {
		if d, ok := r.docs[h.ProductID]; ok {
			fams[d.FamilyID] = true
		}
	}
	return float64(len(fams)) < familyPoolMinShare*float64(len(r.step.Top))
}

// familyDeep — текстовый список с рангами по семействам: сначала лучший оффер каждого семейства (запрос со
// сворачиванием), затем остальные офферы исходного списка. Сбой повторного запроса — исходный список.
func (s *Services) familyDeep(ctx context.Context, q map[string]any, r retrieval) retrieval {
	if r.err != nil || !familyDominated(r) {
		return r
	}
	q["collapse"] = map[string]any{"field": "family_id"}
	res, err := s.search.Search(ctx, s.alias, q)
	if err != nil {
		return r
	}
	top, docs, err := parseHits(res)
	if err != nil || len(top) == 0 {
		return r
	}
	seen := map[string]bool{}
	merged := make([]domain.RankedHit, 0, len(top)+len(r.step.Top))
	add := func(h domain.RankedHit) {
		if seen[h.ProductID] {
			return
		}
		seen[h.ProductID] = true
		h.Rank = len(merged) + 1
		merged = append(merged, h)
	}
	for _, h := range top {
		add(h)
	}
	for _, h := range r.step.Top {
		add(h)
	}
	for id, d := range docs {
		if _, ok := r.docs[id]; !ok {
			r.docs[id] = d
		}
	}
	r.step.Top = merged
	r.step.Mode += "+families"
	return r
}
