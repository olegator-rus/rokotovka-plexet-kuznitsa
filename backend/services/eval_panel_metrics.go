package services

import (
	"math"
	"sort"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Метрики прогона панели — чистые функции над наблюдениями: что панель показала на префикс.

const (
	pevalTopSuggest  = 5 // MRR@5, Success@5 подсказок
	pevalTopProducts = 5 // MRR@5, Success@5 товаров
	pevalSlots       = 6 // слоты товаров: разнообразие семейств
)

// pevalObs — ответ панели на префикс в одном режиме.
type pevalObs struct {
	suggestions []string // тексты подсказок по порядку показа
	families    []string // семейства товаров по слотам
	products    []string // позиции-представители по слотам ("" — неизвестна, прежний ответ даёт семейства)
	ms          float64  // время сервера
	productsMs  float64  // секция товаров (панель контура выдачи), 0 — нет
	partial     bool
	llm, embed  int
	rerank      int
	err         error
}

// pevalRank — место цели (1..top) в списке; 0 — нет в первых top.
func pevalRank(list []string, target string, top int, fold func(string) string) int {
	if target == "" {
		return 0
	}
	t := fold(target)
	for i, x := range list {
		if i >= top {
			break
		}
		if fold(x) == t {
			return i + 1
		}
	}
	return 0
}

func pevalIdentity(s string) string { return s }

// pevalExactCheck — префикс — полный артикул или парт-номер позиции: проверяется, что она первая.
func pevalExactCheck(p domain.PanelPrefix) bool {
	return p.Type == domain.PrefixSKU && p.TargetProduct != nil && foldText(p.Prefix) == foldText(p.TargetQuery)
}

// scorePanelPrefix — метрики одного префикса.
func scorePanelPrefix(p domain.PanelPrefix, o pevalObs) out.PanelPrefixResult {
	r := out.PanelPrefixResult{ID: p.ID, Prefix: p.Prefix, Type: p.Type, TargetQuery: p.TargetQuery,
		Suggestions: pevalCap(o.suggestions, pevalTopSuggest), Families: pevalCap(o.families, pevalSlots), Ms: round2(o.ms)}
	if p.TargetFamily != nil {
		r.TargetFamily = *p.TargetFamily
	}
	if o.err != nil {
		r.Error = o.err.Error()
		return r
	}
	r.SuggestRank = pevalRank(o.suggestions, p.TargetQuery, pevalTopSuggest, foldText)
	r.ProductRank = pevalRank(o.families, r.TargetFamily, pevalTopProducts, pevalIdentity)
	seen := map[string]bool{}
	for i, f := range o.families {
		if i >= pevalSlots {
			break
		}
		if seen[f] {
			r.FamilyDup = true
		}
		seen[f] = true
	}
	if pevalExactCheck(p) {
		ok := false
		if len(o.families) > 0 {
			if len(o.products) > 0 && o.products[0] != "" {
				ok = o.products[0] == *p.TargetProduct
			} else {
				ok = o.families[0] == r.TargetFamily
			}
		}
		r.ExactOK = &ok
	}
	return r
}

// pevalSaved — сэкономленные нажатия по целям: длина цели (символов) и кратчайший префикс, на котором цель в
// первых 5 подсказках (0 — не нашлась).
type pevalSaved struct {
	length, found int
}

// aggregatePanel — метрики режима по префиксам. typ = "" — все префиксы; saved — по целям режима.
func aggregatePanel(prefixes []domain.PanelPrefix, rows []out.PanelPrefixResult, obs []pevalObs, saved []pevalSaved) out.PanelMetrics {
	m := out.PanelMetrics{Prefixes: len(rows), ToEmptyExamples: []string{}}
	var sMRR, pMRR float64
	var sHit, pHit, distinct int
	var cold, prod []float64
	for i, r := range rows {
		p, o := prefixes[i], obs[i]
		if r.Error != "" {
			m.Errors++
			continue
		}
		cold = append(cold, o.ms)
		if o.productsMs > 0 {
			prod = append(prod, o.productsMs)
		}
		if o.partial {
			m.Partial++
		}
		m.LLMCalls += o.llm
		m.EmbedCalls += o.embed
		m.RerankCalls += o.rerank
		if p.Type != domain.PrefixNoResults {
			m.SuggestN++
			if r.SuggestRank > 0 {
				sHit++
				sMRR += 1 / float64(r.SuggestRank)
			}
			if len(o.suggestions) == 0 {
				m.EmptySuggest++
			}
		}
		if p.TargetFamily != nil {
			m.ProductN++
			if r.ProductRank > 0 {
				pHit++
				pMRR += 1 / float64(r.ProductRank)
			}
		}
		if len(o.families) > 0 {
			m.ProductPrefixes++
			distinct += pevalDistinct(pevalCap(o.families, pevalSlots))
			if r.FamilyDup {
				m.FamilyDupPrefixes++
			}
		}
		if r.ExactOK != nil {
			m.ExactChecks++
			if !*r.ExactOK {
				m.ExactDisplaced++
			}
		}
	}
	m.SuggestMRR5, m.SuggestSuccess5 = pevalDiv(sMRR, m.SuggestN), ratio(sHit, m.SuggestN)
	m.ProductMRR5, m.ProductSuccess5 = pevalDiv(pMRR, m.ProductN), ratio(pHit, m.ProductN)
	m.EmptySuggestRate = ratio(m.EmptySuggest, m.SuggestN)
	m.FamilyDupRate = ratio(m.FamilyDupPrefixes, m.ProductPrefixes)
	m.DistinctFamilies = pevalDiv(float64(distinct), m.ProductPrefixes)
	m.P50Ms, m.P95Ms = pevalPercentile(cold, 50), pevalPercentile(cold, 95)
	m.ProductsP95Ms = pevalPercentile(prod, 95)
	if len(saved) > 0 {
		var sum, total float64
		for _, s := range saved {
			total += float64(s.length)
			if s.found > 0 {
				sum += float64(s.length - s.found)
			}
		}
		m.SavedTargets = len(saved)
		m.SavedKeystrokes = pevalDiv(sum, len(saved))
		if total > 0 {
			v := sum / total
			m.SavedShare = &v
		}
	}
	return m
}

// pevalByType — метрики по типам префиксов (без сэкономленных нажатий и задержек кэша).
func pevalByType(prefixes []domain.PanelPrefix, rows []out.PanelPrefixResult, obs []pevalObs) map[string]out.PanelMetrics {
	res := map[string]out.PanelMetrics{}
	for _, t := range domain.PrefixTypes {
		var ps []domain.PanelPrefix
		var rs []out.PanelPrefixResult
		var os []pevalObs
		for i := range prefixes {
			if prefixes[i].Type == t {
				ps, rs, os = append(ps, prefixes[i]), append(rs, rows[i]), append(os, obs[i])
			}
		}
		if len(ps) > 0 {
			res[t] = aggregatePanel(ps, rs, os, nil)
		}
	}
	return res
}

// pevalChanges — префиксы, где новый режим лучше или хуже прежнего по месту цели в подсказках и в товарах.
func pevalChanges(prefixes []domain.PanelPrefix, old, cur []out.PanelPrefixResult) []out.PanelChange {
	res := []out.PanelChange{}
	worse := func(a, b int) bool { return pevalRankScore(b) < pevalRankScore(a) }
	for i, p := range prefixes {
		if old[i].Error != "" || cur[i].Error != "" {
			continue
		}
		for _, sec := range []struct {
			name     string
			from, to int
			on       bool
		}{
			{"suggestions", old[i].SuggestRank, cur[i].SuggestRank, p.Type != domain.PrefixNoResults},
			{"products", old[i].ProductRank, cur[i].ProductRank, p.TargetFamily != nil},
		} {
			if !sec.on || sec.from == sec.to {
				continue
			}
			kind := "improvement"
			if worse(sec.from, sec.to) {
				kind = "regression"
			}
			res = append(res, out.PanelChange{ID: p.ID, Prefix: p.Prefix, Type: p.Type, Target: p.TargetQuery,
				Section: sec.name, OldRank: sec.from, NewRank: sec.to, Kind: kind})
		}
	}
	sort.SliceStable(res, func(i, j int) bool { return res[i].Kind > res[j].Kind }) // ухудшения первыми
	return res
}

// pevalRankScore — 1/место; 0 — не в первых.
func pevalRankScore(r int) float64 {
	if r <= 0 {
		return 0
	}
	return 1 / float64(r)
}

func pevalCap(xs []string, n int) []string {
	if xs == nil {
		return []string{}
	}
	if len(xs) > n {
		return xs[:n]
	}
	return xs
}

func pevalDistinct(xs []string) int {
	seen := map[string]bool{}
	for _, x := range xs {
		seen[x] = true
	}
	return len(seen)
}

func pevalDiv(sum float64, n int) *float64 {
	if n == 0 {
		return nil
	}
	v := sum / float64(n)
	return &v
}

// pevalPercentile — перцентиль по ближайшему рангу, мс с точностью до сотых; пусто — nil.
func pevalPercentile(xs []float64, p int) *float64 {
	if len(xs) == 0 {
		return nil
	}
	s := append([]float64(nil), xs...)
	sort.Float64s(s)
	i := int(math.Ceil(float64(p)/100*float64(len(s)))) - 1
	v := round2(s[max(0, min(i, len(s)-1))])
	return &v
}
