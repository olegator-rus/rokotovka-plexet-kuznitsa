package services

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/prompts"

	"go.uber.org/zap"
)

// Калибровка LLM-судьи на якорях фида (`go run . judge-calib`): эталоны заказчика (etalon.jsonl) и их соседи
// (neighbors.jsonl) — запросы с известной целью. Товар целевого семейства должен получить 3 (любой вариант:
// у эталона судье показываются две основные лицензии семейства — первая и последняя по id), товар цели якоря другого
// вендора («запрещённый сосед») — не выше 1. Каппа против кликов не считается: неявные оценки почти все 2.

// judgeCalibSet — набор в judge_runs, под которым хранится калибровка (GET /api/evaluations → judge_calibration).
const judgeCalibSet = "anchors"

// Наборы якорей: файлы набора «по цели» на фиде.
var judgeCalibSets = []string{"etalon", "neighbors"}

// JudgeCalibRequest — калибровка: промпт (nil — действующий prompts.Judge()), передавать ли семейство в карточке.
type JudgeCalibRequest struct {
	Prompt   *prompts.Prompt
	NoFamily bool
	Persist  bool
	// CacheDir — кэш оценок (.judge-cache.json); пусто — без кэша.
	CacheDir string
}

// calibPair — пара калибровки.
type calibPair struct {
	judgePairIn
	expect string // target | forbidden
	etalon bool
}

// RunJudgeCalibration — пары якорей → судья → доля целей с 3 и запрещённых соседей не выше 1.
func (s *Services) RunJudgeCalibration(ctx context.Context, req JudgeCalibRequest) (*out.JudgeCalibration, error) {
	started := time.Now()
	if s.judge == nil {
		return nil, errs.NewUnavailableError(MsgJudgeNoModel)
	}
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return nil, errs.NewUnavailableError(MsgJudgeNoLex)
	}
	lx := st.lex
	var anchors []domain.TargetQuery
	for _, set := range judgeCalibSets {
		qs, err := s.TargetSetQueries(ctx, set)
		if err != nil {
			return nil, err
		}
		anchors = append(anchors, qs...)
	}
	pairs := calibPairs(anchors, lx)
	prompt := prompts.Judge()
	if req.Prompt != nil {
		prompt = *req.Prompt
	}
	res := &out.JudgeCalibration{RunID: newEvalRunID(started), Model: llmModelName(s.judge.Model()), PromptVersion: prompt.Version,
		CreatedAt: started.UTC(), WithFamily: !req.NoFamily, Misses: []out.JudgeCalibMiss{}}
	cache := loadGradeCache(req.CacheDir)
	ckey := func(p *calibPair) string {
		k := p.q.Text + "\x00" + p.pid
		if req.NoFamily {
			k += "\x00nofamily"
		}
		return cache.key(res.Model, prompt.Version, k)
	}
	order := make([]*calibPair, 0, len(pairs))
	for _, p := range pairs {
		if g, ok := cache.grades[ckey(p)]; ok {
			g := g
			p.grade = &g
			continue
		}
		order = append(order, p)
	}
	sort.Slice(order, func(i, j int) bool {
		a, b := &order[i].judgePairIn, &order[j].judgePairIn
		if a.q.ID != b.q.ID {
			return a.q.ID < b.q.ID
		}
		return judgeShuffleKey(a) < judgeShuffleKey(b)
	})
	for b := 0; b*judgeBatch < len(order); b++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		chunk := order[b*judgeBatch : min(len(order), (b+1)*judgeBatch)]
		batch := make([]*judgePairIn, len(chunk))
		for i, p := range chunk {
			batch[i] = &p.judgePairIn
		}
		rid := fmt.Sprintf("%scalib-%s-%03d", judgeRequestPrefix, res.RunID[len(res.RunID)-6:], b+1)
		cost, calls, err := s.judgeBatchWith(WithRequestID(ctx, rid), prompt.Text, lx, batch, !req.NoFamily)
		res.CostRub += cost
		res.Calls += calls
		if err != nil {
			zap.L().Warn("калибровка судьи: пачка не оценена", zap.Int("batch", b+1), zap.Error(err))
		}
		for _, p := range chunk {
			if p.grade != nil {
				cache.grades[ckey(p)] = *p.grade
				cache.dirty = true
			}
		}
	}
	if err := cache.save(); err != nil {
		zap.L().Warn("калибровка судьи: кэш не записан", zap.Error(err))
	}
	res.CostRub = round4(res.CostRub)
	scoreCalibration(res, pairs, lx)
	res.DurationMs = int(time.Since(started).Milliseconds())
	zap.L().Info("калибровка судьи завершена", zap.String("run_id", res.RunID), zap.Int("targets", res.Targets),
		zap.Int("targets_hit", res.TargetsHit), zap.Int("forbidden", res.Forbidden), zap.Int("forbidden_ok", res.ForbiddenOK),
		zap.Int("calls", res.Calls), zap.Float64("cost_rub", res.CostRub))
	if req.Persist && s.repos.Enabled() {
		row := &domain.JudgeRun{RunID: res.RunID, Set: judgeCalibSet, Model: res.Model, PromptVersion: res.PromptVersion,
			Configs: strings.Join(judgeCalibSets, ","), Pairs: len(pairs), Judged: len(pairs) - res.Unjudged, Calls: res.Calls,
			CostRub: res.CostRub, DurationMs: res.DurationMs}
		row.Agreement, _ = domain.ToJSONB(res)
		if err := s.repos.SaveJudgeRun(ctx, row); err != nil {
			return res, fmt.Errorf("калибровка судьи: результат не записан: %w", err)
		}
	}
	return res, nil
}

// scoreCalibration — доли и промахи по оценённым парам.
func scoreCalibration(res *out.JudgeCalibration, pairs []*calibPair, lx *searchLexicon) {
	for _, p := range pairs {
		if p.grade == nil {
			res.Unjudged++
			continue
		}
		g := min(max(*p.grade, 0), 3)
		d := lx.docs[p.pid]
		miss := false
		switch p.expect {
		case "target":
			res.Targets++
			res.TargetGrades[g]++
			if p.etalon {
				res.EtalonTargets++
			}
			if g == exactGrade {
				res.TargetsHit++
				if p.etalon {
					res.EtalonHit++
				}
			} else {
				miss = true
			}
		default:
			res.Forbidden++
			res.ForbiddenGrades[g]++
			if g <= 1 {
				res.ForbiddenOK++
			} else {
				miss = true
			}
		}
		if miss {
			res.Misses = append(res.Misses, out.JudgeCalibMiss{Query: p.q.Text, Product: d.Title, Family: d.FamilyName, Expect: p.expect, Grade: g})
		}
	}
	if res.Targets > 0 {
		res.TargetRate = evalPtr(evalRound4(float64(res.TargetsHit) / float64(res.Targets)))
	}
	if res.Forbidden > 0 {
		res.ForbiddenRate = evalPtr(evalRound4(float64(res.ForbiddenOK) / float64(res.Forbidden)))
	}
}

// calibPairs — пары калибровки. Цель якоря — семейство из ожидания (family: group_ids; vendor_category: group_ids
// или самое большое семейство вендора в категории; vendor: самое большое семейство вендора). Эталону — два артикула
// семейства (первая и последняя основная лицензия по id: «любой вариант — 3»), соседу — первая. Запрещённый сосед — первый артикул
// цели якоря другого вендора, вендор по кругу.
func calibPairs(anchors []domain.TargetQuery, lx *searchLexicon) []*calibPair {
	byFam := map[string][]string{}
	for pid, d := range lx.docs {
		if d.FamilyID != "" {
			byFam[d.FamilyID] = append(byFam[d.FamilyID], pid)
		}
	}
	// Внутри семейства — сначала основные лицензии (license_kind base или пусто), затем прочие варианты (поддержка,
	// продление, модуль): представитель продукта — его лицензия, а не сертификат поддержки.
	baseFirst := func(a, b string) int {
		ba, bb := isBaseLicense(lx.docs[a]), isBaseLicense(lx.docs[b])
		if ba != bb {
			if ba {
				return -1
			}
			return 1
		}
		return comparePID(a, b)
	}
	for _, ids := range byFam {
		slices.SortFunc(ids, baseFirst)
	}
	target := map[string]string{} // id якоря → семейство цели
	vendorTarget := map[string]string{}
	var vendors []string
	for _, a := range anchors {
		f := calibTargetFamily(a.Expect, byFam, lx)
		if f == "" {
			continue
		}
		target[a.ID] = f
		v := a.Vendor
		if v == "" {
			v = lx.docs[byFam[f][0]].VendorSlug
		}
		if _, ok := vendorTarget[v]; !ok && v != "" {
			vendorTarget[v] = f
			vendors = append(vendors, v)
		}
	}
	sort.Strings(vendors)
	var pairs []*calibPair
	for i, a := range anchors {
		f, ok := target[a.ID]
		if !ok {
			continue
		}
		q := domain.EvalQuery{ID: a.ID, Text: a.Text}
		etalon := a.Group == "etalon"
		ids := byFam[f]
		picks := []string{ids[0]}
		if etalon {
			// второй вариант той же лицензии — последний из основных по id
			last := ids[0]
			for _, pid := range ids {
				if isBaseLicense(lx.docs[pid]) {
					last = pid
				}
			}
			if last != ids[0] {
				picks = append(picks, last)
			}
		}
		for _, pid := range picks {
			pairs = append(pairs, &calibPair{judgePairIn: judgePairIn{q: q, pid: pid}, expect: "target", etalon: etalon})
		}
		own := a.Vendor
		if own == "" {
			own = lx.docs[ids[0]].VendorSlug
		}
		var others []string
		for _, v := range vendors {
			if v != own && vendorTarget[v] != f {
				others = append(others, v)
			}
		}
		if len(others) == 0 {
			continue
		}
		fv := vendorTarget[others[i%len(others)]]
		pairs = append(pairs, &calibPair{judgePairIn: judgePairIn{q: q, pid: byFam[fv][0]}, expect: "forbidden", etalon: etalon})
	}
	return pairs
}

// calibTargetFamily — семейство цели якоря; пусто — цель в каталоге не нашлась.
func calibTargetFamily(e domain.TargetExpect, byFam map[string][]string, lx *searchLexicon) string {
	for _, g := range e.GroupIDs {
		if len(byFam[g]) > 0 {
			return g
		}
	}
	if e.VendorSlug == "" {
		return ""
	}
	best, n := "", 0
	for f, ids := range byFam {
		d := lx.docs[ids[0]]
		if d.VendorSlug != e.VendorSlug {
			continue
		}
		if len(e.CategoryIDs) > 0 && !slices.ContainsFunc(e.CategoryIDs, func(c string) bool {
			return d.CategoryID == c || slices.Contains(d.CategoryIDs, c)
		}) {
			continue
		}
		if len(ids) > n || len(ids) == n && f < best {
			best, n = f, len(ids)
		}
	}
	return best
}

// isBaseLicense — основная лицензия продукта (не поддержка, не продление, не модуль).
func isBaseLicense(d indexDoc) bool { return d.LicenseKind == "base" || d.LicenseKind == "" }

// comparePID — порядок id позиций: числовые по значению, прочие по строке.
func comparePID(a, b string) int {
	if len(a) != len(b) && isDigits(a) && isDigits(b) {
		return len(a) - len(b)
	}
	return strings.Compare(a, b)
}

func isDigits(s string) bool {
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return s != ""
}

// latestCalibration — последняя калибровка судьи на якорях; nil — не было.
func (s *Services) latestCalibration(ctx context.Context) (*out.JudgeCalibration, error) {
	row, err := s.repos.LatestJudgeRun(ctx, judgeCalibSet)
	if err != nil || row == nil {
		return nil, err
	}
	var c out.JudgeCalibration
	if err := json.Unmarshal(row.Agreement, &c); err != nil {
		return nil, fmt.Errorf("judge_runs %s: %w", row.RunID, err)
	}
	c.Misses = nil
	return &c, nil
}

// FormatJudgeCalibration — сводка калибровки для stdout.
func FormatJudgeCalibration(c *out.JudgeCalibration) string {
	var b strings.Builder
	pct := func(n, of int) float64 {
		if of == 0 {
			return 0
		}
		return float64(n) * 100 / float64(of)
	}
	fmt.Fprintf(&b, "Калибровка судьи %s, промпт %s, семейство в карточке: %v\n", c.Model, c.PromptVersion, c.WithFamily)
	fmt.Fprintf(&b, "Цели (оценка 3): %d из %d (%.1f %%); эталоны заказчика: %d из %d\n", c.TargetsHit, c.Targets, pct(c.TargetsHit, c.Targets), c.EtalonHit, c.EtalonTargets)
	fmt.Fprintf(&b, "Запрещённые соседи (не выше 1): %d из %d (%.1f %%)\n", c.ForbiddenOK, c.Forbidden, pct(c.ForbiddenOK, c.Forbidden))
	fmt.Fprintf(&b, "Оценки целей 0–3: %v; запрещённых: %v; без ответа: %d\n", c.TargetGrades, c.ForbiddenGrades, c.Unjudged)
	fmt.Fprintf(&b, "Вызовов %d, %.2f ₽, %d мс, прогон %s\n", c.Calls, c.CostRub, c.DurationMs, c.RunID)
	for _, m := range c.Misses {
		fmt.Fprintf(&b, "  промах [%s] «%s» → %s (%s): %d\n", m.Expect, m.Query, m.Product, m.Family, m.Grade)
	}
	return b.String()
}
