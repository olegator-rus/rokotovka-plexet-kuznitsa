package services

import (
	"context"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"

	"go.uber.org/zap"
)

// Реранкер (ARCHITECTURE §2.4): кросс-энкодер bge-reranker-v2-m3 оценивает семейства-кандидаты
// относительно запроса, оценка смешивается с релевантностью слияния до порогов групп и бизнес-слоя. Только
// маршруты C и D: там запрос — описание задачи или непонятное слово, и BM25 с векторами расходятся. На A/B
// (точное название, нормализованный запрос) реранкер не нужен и стоит ~140 мс.
//
// Подобрано на tune (B3): NDCG@10 0.933 → 0.948, ухудшений 0; плато вес 0.3–0.6, взят 0.45; топ-10 семейств и
// описание до 300 символов — короче текст быстрее, но шумнее (появляются ухудшения).
const (
	// rerankTop — сколько семейств (по релевантности слияния) уходит в реранкер; остальные теряют долю веса.
	rerankTop = 10
	// rerankDescRunes — длина описания в тексте для реранкера.
	rerankDescRunes = 300
)

// Причины пропуска шага rerank в трассе.
const (
	rerankOff      = "disabled"
	rerankRoute    = "route_not_c_d"
	rerankFew      = "fewer_than_two"
	rerankFailed   = "reranker_error"
	rerankNoSignal = "zero_scores"
	// rerankAnalogs — просят замену названного продукта: аналоги из справочника, порядок — по соответствию
	// лицензии и бизнес-слою («аналог фотошопа на 20 пользователей» — первым то, что лицензируется на пользователей).
	rerankAnalogs = "reference_analogs"
)

// rerankFamilies — оценка семейств реранкером: rel = (1−w)·rel + w·score/лучший score. Сбой реранкера — выдача
// без него (шаг skipped с причиной), поиск не падает.
func (s *Services) rerankFamilies(ctx context.Context, query, route string, fams []*familyHit) domain.StepBase {
	w := s.rerankW
	switch {
	case w <= 0 || !s.reranker.Enabled():
		return skipped(domain.StepRerank, rerankOff)
	case route != domain.RouteIntent && route != domain.RouteSemantic:
		return skipped(domain.StepRerank, rerankRoute)
	case len(fams) < 2:
		return skipped(domain.StepRerank, rerankFew)
	}
	t := time.Now()
	top := append([]*familyHit(nil), fams...)
	sort.SliceStable(top, func(i, j int) bool { return top[i].rel > top[j].rel })
	var tail []*familyHit
	if len(top) > rerankTop {
		top, tail = top[:rerankTop], top[rerankTop:]
	}
	texts := make([]string, len(top))
	for i, fh := range top {
		texts[i] = rerankText(fh.head)
	}
	hits, err := s.rerankCached(ctx, query, texts)
	if err != nil {
		zap.L().Warn("поиск: реранкер недоступен — выдача без него", zap.Error(err))
		return skipped(domain.StepRerank, rerankFailed)
	}
	best := 0.0
	for _, h := range hits {
		best = max(best, h.Score)
	}
	if best <= 0 {
		st := skipped(domain.StepRerank, rerankNoSignal)
		st.Ms = msSince(t)
		return st
	}
	// Без оценки реранкера семейство теряет его долю: хвост за топ-10 и то, что реранкер не вернул.
	scored := map[*familyHit]float64{}
	for _, h := range hits {
		if h.Index >= 0 && h.Index < len(top) {
			scored[top[h.Index]] = h.Score / best
		}
	}
	for _, fh := range append(top, tail...) {
		fh.rel = (1-w)*fh.rel + w*scored[fh]
	}
	return domain.StepBase{Name: domain.StepRerank, Ms: msSince(t)}
}

// rerankText — текст семейства для реранкера: название, категория, начало описания.
func rerankText(d indexDoc) string {
	desc := d.Description
	if r := []rune(desc); len(r) > rerankDescRunes {
		desc = string(r[:rerankDescRunes])
	}
	return strings.Join([]string{d.FamilyName, d.Category, desc}, ". ")
}
