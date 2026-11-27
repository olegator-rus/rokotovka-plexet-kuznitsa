package services

import (
	"context"
	"encoding/json"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Счёт пары «конфигурация против предыдущей» для GET /api/evaluations: сколько запросов стало лучше, хуже и осталось
// без изменений. Списки regressions и improvements полные, но пересекаются: запрос, у которого NDCG вырос, а условие
// лицензии нарушилось, есть в обоих. Складывать длины списков нельзя — здесь каждый сравненный запрос входит ровно
// в одну группу, и сумма трёх групп равна числу сравненных.

// evalPairQuery — то из сохранённого результата запроса, от чего зависит, сравнивался ли он с предыдущей конфигурацией.
type evalPairQuery struct {
	ID     string          `json:"id"`
	Error  string          `json:"error"`
	Target json.RawMessage `json:"target"`
}

func (q evalPairQuery) hasTarget() bool { return len(q.Target) > 0 && string(q.Target) != "null" }

// evalComparedIDs — id запросов, которые сравнивались в паре: есть у обеих конфигураций и без ошибки поиска у обеих;
// у наборов «по цели» (byTarget) — ещё и с вердиктом у обеих. Те же условия, что в evalCompare и TargetChanges.
func evalComparedIDs(prev, cur []evalPairQuery, byTarget bool) map[string]struct{} {
	before := make(map[string]evalPairQuery, len(prev))
	for _, p := range prev {
		before[p.ID] = p
	}
	ids := make(map[string]struct{}, len(cur))
	for _, c := range cur {
		p, ok := before[c.ID]
		if !ok || p.Error != "" || c.Error != "" || (byTarget && (!p.hasTarget() || !c.hasTarget())) {
			continue
		}
		ids[c.ID] = struct{}{}
	}
	return ids
}

// evalChangeCounts — счёт пары по сравненным запросам и спискам изменений. Запрос из обоих списков (одно стало
// лучше, другое хуже) считается ухудшением: правка допустима, только если ухудшений нет. Список называет запрос,
// которого среди сравненных нет (строки разных прогонов), — nil: непосчитанное не показывается.
func evalChangeCounts(compared map[string]struct{}, regs, imps []out.EvalChange) *out.EvalChangeCounts {
	worse := make(map[string]struct{}, len(regs))
	for _, r := range regs {
		if _, ok := compared[r.QueryID]; !ok {
			return nil
		}
		worse[r.QueryID] = struct{}{}
	}
	better := make(map[string]struct{}, len(imps))
	for _, r := range imps {
		if _, ok := compared[r.QueryID]; !ok {
			return nil
		}
		if _, mixed := worse[r.QueryID]; !mixed {
			better[r.QueryID] = struct{}{}
		}
	}
	return &out.EvalChangeCounts{ComparedN: len(compared), ImprovedN: len(better), RegressedN: len(worse),
		UnchangedN: len(compared) - len(better) - len(worse)}
}

// evalRowChangeCounts — счёт пары из сохранённых строк: запросы конфигурации (cur) и предыдущей в том же прогоне
// (prev), списки изменений строки. Запросы не разбираются — nil.
func evalRowChangeCounts(set string, prev, cur domain.JSONB, regs, imps []out.EvalChange) *out.EvalChangeCounts {
	var p, c []evalPairQuery
	if len(prev) == 0 || len(cur) == 0 || json.Unmarshal(prev, &p) != nil || json.Unmarshal(cur, &c) != nil {
		return nil
	}
	return evalChangeCounts(evalComparedIDs(p, c, domain.IsTargetSet(set)), regs, imps)
}

// evalPairCounts — счёт пары для строки r последнего прогона. Предыдущая конфигурация — из того же прогона: среди
// rows, а если её последний прогон другой — из базы. У первой конфигурации прогона и без запросов предыдущей — nil.
func (s *Services) evalPairCounts(ctx context.Context, r domain.Evaluation, rows []domain.Evaluation, v out.EvaluationView) *out.EvalChangeCounts {
	if r.Against == "" {
		return nil
	}
	var prev domain.JSONB
	for _, o := range rows {
		if o.RunID == r.RunID && o.Config == r.Against {
			prev = o.PerQuery
		}
	}
	if len(prev) == 0 {
		raw, err := s.repos.EvaluationPerQuery(ctx, r.RunID, r.Against)
		if err != nil {
			return nil
		}
		prev = raw
	}
	return evalRowChangeCounts(r.Set, prev, r.PerQuery, v.Regressions, v.Improvements)
}
