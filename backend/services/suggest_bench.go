package services

import (
	"context"
	"math"
	"sort"

	"kuznica/api/domain"
)

// Замер панели (`go run . suggest-bench`) и проверки гейта П3-MVP-1: задержки секций на сервере, вызовы моделей,
// дубли семейств в слотах, точный артикул первым, подсказки с пустой текстовой веткой. Трассы не пишутся.

// WarmPanel — словарь подсказок синхронно (при старте API и перед замером): из последней сборки в базе или в памяти.
func (s *Services) WarmPanel(ctx context.Context) error {
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return errNoCatalog
	}
	s.reloadSuggestDict(st.lex, st.version, nil)
	if d := s.suggestDictFor(st.lex, st.version); d == nil {
		return errNoCatalog
	}
	return nil
}

// LatencyStat — перцентили, мс.
type LatencyStat struct {
	N   int     `json:"n"`
	P50 float64 `json:"p50"`
	P95 float64 `json:"p95"`
	Max float64 `json:"max"`
}

// PanelBench — итог замера.
type PanelBench struct {
	Prefixes           int                    `json:"prefixes"`
	SuggestionsVersion int                    `json:"suggestions_version"`
	DictEntries        int                    `json:"dict_entries"`
	Latency            map[string]LatencyStat `json:"latency_ms"`
	Partial            int                    `json:"partial"`
	LLMCalls           int                    `json:"llm_calls"`
	EmbedCalls         int                    `json:"embed_calls"`
	RerankCalls        int                    `json:"rerank_calls"`
	FamilyDupes        int                    `json:"family_dupes"`
	ExactChecked       int                    `json:"exact_checked"`
	ExactShifted       int                    `json:"exact_shifted"`
	SuggestionsChecked int                    `json:"suggestions_checked"`
	EmptySuggestions   int                    `json:"empty_suggestions"`
	EmptyExamples      []string               `json:"empty_examples"`
}

// BenchPanel — прогон префиксов: промах кэша товаров, затем ответ из кэша.
func (s *Services) BenchPanel(ctx context.Context, prefixes []string) (*PanelBench, error) {
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return nil, errNoCatalog
	}
	res := &PanelBench{Prefixes: len(prefixes), Latency: map[string]LatencyStat{}, EmptyExamples: []string{}}
	if d := s.suggestDictFor(st.lex, st.version); d != nil {
		res.SuggestionsVersion, res.DictEntries = d.version, len(d.entries)
	}
	series := map[string][]float64{}
	shown := map[string]bool{}
	shownCats := map[string]string{}
	codes := st.lex.codes()
	for _, p := range prefixes {
		miss, err := s.SuggestPanel(ctx, "", SuggestRequest{Q: p}, SuggestOptions{NoLog: true, NoCache: true})
		if err != nil {
			return nil, err
		}
		if miss.Mode != panelModePrefix {
			continue
		}
		m := miss.Meta
		series["suggestions"] = append(series["suggestions"], m.Ms.Suggestions)
		series["refinements"] = append(series["refinements"], m.Ms.Refinements)
		series["products_miss"] = append(series["products_miss"], m.Ms.Products)
		series["total_miss"] = append(series["total_miss"], m.Ms.Total)
		if m.Partial {
			res.Partial++
		}
		res.LLMCalls += m.LLMCalls
		res.EmbedCalls += m.EmbedCalls
		res.RerankCalls += m.RerankCalls
		// прогрев и ответ из кэша
		if _, err := s.SuggestPanel(ctx, "", SuggestRequest{Q: p}, SuggestOptions{NoLog: true}); err != nil {
			return nil, err
		}
		hit, err := s.SuggestPanel(ctx, "", SuggestRequest{Q: p}, SuggestOptions{NoLog: true})
		if err != nil {
			return nil, err
		}
		if hit.Meta.ProductsCached {
			series["products_cached"] = append(series["products_cached"], hit.Meta.Ms.Products)
			series["total_cached"] = append(series["total_cached"], hit.Meta.Ms.Total)
		}
		fams := map[string]bool{}
		for _, pr := range miss.Products {
			if fams[pr.FamilyID] {
				res.FamilyDupes++
			}
			fams[pr.FamilyID] = true
		}
		if code := normalizeMPN(p); len(code) >= 4 && hasDigit(code) {
			if pids := codes.exact(code); len(pids) > 0 {
				res.ExactChecked++
				if len(miss.Products) == 0 || !contains(pids, miss.Products[0].ProductID) {
					res.ExactShifted++
				}
			}
		}
		for _, sg := range miss.Suggestions {
			if sg.Kind == suggestCategory {
				// подсказка-категория ведёт в категорию (category_id), а не в текстовый поиск
				shownCats[sg.CategoryID] = sg.Text
				continue
			}
			shown[sg.Text] = true
		}
	}
	rules := s.rulesFor(ctx, domain.ConfigLive, st.lex, nil)
	for _, t := range sortedKeys(shown) {
		res.SuggestionsChecked++
		fams, err := s.textFamilies(ctx, st.lex, rules, foldText(t))
		if err != nil {
			return nil, err
		}
		if len(fams) == 0 {
			res.EmptySuggestions++
			if len(res.EmptyExamples) < 10 {
				res.EmptyExamples = append(res.EmptyExamples, t)
			}
		}
	}
	for _, id := range sortedKeys(shownCats) {
		res.SuggestionsChecked++
		if categoryFamilies(st.lex, id) == 0 {
			res.EmptySuggestions++
			res.EmptyExamples = append(res.EmptyExamples, shownCats[id])
		}
	}
	for k, v := range series {
		res.Latency[k] = latencyOf(v)
	}
	return res, nil
}

func latencyOf(v []float64) LatencyStat {
	if len(v) == 0 {
		return LatencyStat{}
	}
	sort.Float64s(v)
	at := func(q float64) float64 { return round2(v[int(math.Round(q*float64(len(v)-1)))]) }
	return LatencyStat{N: len(v), P50: at(0.5), P95: at(0.95), Max: round2(v[len(v)-1])}
}
