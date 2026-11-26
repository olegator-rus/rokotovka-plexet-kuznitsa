package services

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	"kuznica/api/domain"
)

// rerankFamilies — экспериментальный реранкер: оценка семейств кросс-энкодером, смешивается с
// релевантностью слияния. Возвращает шаг трассы.
func (s *Services) rerankFamilies(ctx context.Context, query, route string, fams []*familyHit) domain.StepBase {
	w, _ := strconv.ParseFloat(os.Getenv("KUZNICA_RERANK_EXP"), 64)
	if w <= 0 || s.reranker == nil {
		return skipped(domain.StepRerank, "disabled")
	}
	if route != domain.RouteIntent && route != domain.RouteSemantic {
		return skipped(domain.StepRerank, "route")
	}
	if len(fams) < 2 {
		return skipped(domain.StepRerank, "few")
	}
	t := time.Now()
	texts := make([]string, len(fams))
	for i, fh := range fams {
		d := fh.head
		desc := d.Description
		if r := []rune(desc); len(r) > 300 {
			desc = string(r[:300])
		}
		texts[i] = strings.Join([]string{d.FamilyName, d.Category, desc}, ". ")
	}
	hits, err := s.reranker.Rerank(ctx, query, texts)
	if err != nil || len(hits) == 0 {
		return skipped(domain.StepRerank, "error")
	}
	best := hits[0].Score
	if best <= 0 {
		return skipped(domain.StepRerank, "zero")
	}
	for _, h := range hits {
		fh := fams[h.Index]
		fh.rel = (1-w)*fh.rel + w*h.Score/best
	}
	st := domain.StepBase{Name: domain.StepRerank}
	st.Ms = msSince(t)
	return st
}
