package services

import (
	"sort"

	"kuznica/api/domain"
)

// rrfK — константа RRF: score(d) = Σ 1/(k + rank_r(d)) (ARCHITECTURE §2.4). Стартовое значение.
const rrfK = 60

// fused — кандидат после слияния: сырой RRF, нормированная релевантность и ранги в источниках.
type fused struct {
	ProductID string
	RRF       float64
	Relevance float64 // RRF / максимум при n списках: 1.0 — первое место во всех списках
	BM25Rank  *int
	DenseRank *int
}

// rrf — слияние ранжированных списков. Релевантность нормируется на теоретический максимум
// n/(k+1), а не на лучший в выдаче: так одинаковое число значит одинаковое на разных запросах, и
// порог следующей волны можно будет настраивать. Порядок: RRF по убыванию, при равенстве — лучший
// ранг BM25, затем id (детерминированно).
func rrf(k int, bm25, dense []domain.RankedHit) []fused {
	lists := 0
	byID := map[string]*fused{}
	var order []*fused
	add := func(hits []domain.RankedHit, isBM25 bool) {
		if len(hits) == 0 {
			return
		}
		lists++
		for _, h := range hits {
			f, ok := byID[h.ProductID]
			if !ok {
				f = &fused{ProductID: h.ProductID}
				byID[h.ProductID] = f
				order = append(order, f)
			}
			rank := h.Rank
			if isBM25 {
				f.BM25Rank = &rank
			} else {
				f.DenseRank = &rank
			}
			f.RRF += 1 / float64(k+h.Rank)
		}
	}
	add(bm25, true)
	add(dense, false)
	if lists == 0 {
		return nil
	}
	ceiling := float64(lists) / float64(k+1)
	out := make([]fused, len(order))
	for i, f := range order {
		f.Relevance = f.RRF / ceiling
		out[i] = *f
	}
	sort.SliceStable(out, func(i, j int) bool {
		if out[i].RRF != out[j].RRF {
			return out[i].RRF > out[j].RRF
		}
		ri, rj := rankOr(out[i].BM25Rank), rankOr(out[j].BM25Rank)
		if ri != rj {
			return ri < rj
		}
		return out[i].ProductID < out[j].ProductID
	})
	return out
}

func rankOr(r *int) int {
	if r == nil {
		return int(^uint(0) >> 1)
	}
	return *r
}
