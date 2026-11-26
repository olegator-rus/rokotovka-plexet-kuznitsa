package services

import (
	"context"
	"crypto/sha256"
	"encoding/json"
	"errors"
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/prompts"

	"go.uber.org/zap"
)

// LLM-судья (ARCHITECTURE §6, «Разметка»): модель другого семейства, чем разбор и агент (Qwen
// против Alice AI), вслепую — без конфигурации и позиции — оценивает пары запрос–товар 0–3 на пуле топ-10
// конфигураций. Согласие с разметкой набора (точное, ±1, каппа Коэна с квадратичными весами) публикуется рядом
// с метриками, как и NDCG конфигураций глазами судьи: ему можно верить в вопросе «какая лучше», не в числах.

// Лимиты судьи: пул не больше judgeMaxPairs пар, judgeBatch пар на вызов.
const (
	judgeMaxPairs   = 600
	judgeBatch      = 10
	judgeTopK       = 10
	judgeCallLimit  = 60 * time.Second
	judgeMaxTokens  = 1000
	judgeMaxDescLen = 400
	// judgeOrderEps — разница NDCG меньше этой порядок конфигураций не задаёт (ничья).
	judgeOrderEps = 0.01
)

// judgeRequestPrefix — начало request_id вызовов судьи в llm_usage (kind=judge в /api/costs).
const judgeRequestPrefix = "judge-"

// Тексты отказов судьи.
const (
	MsgJudgeNoModel = "Модель судьи не настроена (LLM_API_KEY, LLM_JUDGE_MODEL)"
	MsgJudgeNoLex   = "Каталог не загружен — судье нечего показать"
)

// JudgeRequest — прогон судьи: набор, конфигурации пула, лимит пар.
type JudgeRequest struct {
	Set      string
	Configs  []string
	MaxPairs int
	Persist  bool
}

// judgePairIn — пара в пуле.
type judgePairIn struct {
	q     domain.EvalQuery
	pid   string
	rank  int // лучшая позиция пары среди конфигураций (только для сборки пула; судье не показывается)
	grade *int
}

// RunJudge — пул топ-10 конфигураций → судья → согласие с разметкой.
func (s *Services) RunJudge(ctx context.Context, req JudgeRequest) (*out.JudgeRun, error) {
	started := time.Now()
	if s.judge == nil {
		return nil, errs.NewUnavailableError(MsgJudgeNoModel)
	}
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return nil, errs.NewUnavailableError(MsgJudgeNoLex)
	}
	if req.MaxPairs <= 0 || req.MaxPairs > judgeMaxPairs {
		req.MaxPairs = judgeMaxPairs
	}
	run, err := s.RunEvaluation(ctx, EvalRequest{Set: req.Set, Configs: req.Configs})
	if err != nil {
		return nil, err
	}
	queries, err := s.evalQueries(ctx, run.Set, nil)
	if err != nil {
		return nil, err
	}
	pairs, depth := judgePool(run, queries, req.MaxPairs)
	res := &out.JudgeRun{JudgeAgreement: out.JudgeAgreement{
		RunID: run.RunID, Set: run.Set, SetHash: run.SetHash, Model: llmModelName(s.judge.Model()),
		PromptVersion: prompts.Judge().Version, CreatedAt: started.UTC(), Pairs: len(pairs), Depth: depth,
		ByConfig: []out.JudgeConfig{},
	}}
	for _, c := range run.Configs {
		res.Configs = append(res.Configs, c.Config)
	}

	// Вслепую: порядок пар перемешан детерминированно (хэш пары), конфигурация и позиция судье не уходят.
	order := slices.Clone(pairs)
	sort.Slice(order, func(i, j int) bool { return judgeShuffleKey(order[i]) < judgeShuffleKey(order[j]) })
	prompt := prompts.Judge()
	for b := 0; b*judgeBatch < len(order); b++ {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		batch := order[b*judgeBatch : min(len(order), (b+1)*judgeBatch)]
		rid := fmt.Sprintf("%s%s-%03d", judgeRequestPrefix, run.RunID[len(run.RunID)-6:], b+1)
		cost, calls, err := s.judgeBatch(WithRequestID(ctx, rid), prompt.Text, st.lex, batch)
		res.CostRub += cost
		res.Calls += calls
		if err != nil {
			zap.L().Warn("судья: пачка не оценена", zap.Int("batch", b+1), zap.Error(err))
		}
	}
	res.CostRub = round4(res.CostRub)

	for _, p := range pairs {
		label := p.q.Judgments[p.pid]
		res.Grades = append(res.Grades, out.JudgePair{QueryID: p.q.ID, ProductID: p.pid, Label: label, Judge: p.grade})
	}
	judgeAgreement(&res.JudgeAgreement, res.Grades)
	res.ByConfig, res.SameOrder = judgeByConfig(run, pairs, depth)
	res.DurationMs = int(time.Since(started).Milliseconds())
	zap.L().Info("судья: прогон завершён", zap.String("run_id", res.RunID), zap.Int("pairs", res.Pairs),
		zap.Int("judged", res.Judged), zap.Int("calls", res.Calls), zap.Float64("cost_rub", res.CostRub))
	if req.Persist && s.repos.Enabled() {
		row := &domain.JudgeRun{
			RunID: res.RunID, Set: res.Set, SetHash: res.SetHash, Model: res.Model, PromptVersion: res.PromptVersion,
			Configs: strings.Join(res.Configs, ","), Pairs: res.Pairs, Judged: res.Judged, Calls: res.Calls,
			CostRub: res.CostRub, DurationMs: res.DurationMs,
		}
		row.Agreement, _ = domain.ToJSONB(res.JudgeAgreement)
		row.ByConfig, _ = domain.ToJSONB(res.ByConfig)
		row.Grades, _ = domain.ToJSONB(res.Grades)
		if err := s.repos.SaveJudgeRun(ctx, row); err != nil {
			return res, fmt.Errorf("судья: результат не записан: %w", err)
		}
	}
	return res, nil
}

// judgePool — пары запрос–товар из топ-10 конфигураций прогона, по слоям глубины: сначала все первые места,
// потом вторые и т.д., пока пул не превысит лимит. depth — последняя глубина, вошедшая целиком.
func judgePool(run *out.EvalRun, queries []domain.EvalQuery, maxPairs int) ([]*judgePairIn, int) {
	byID := map[string]domain.EvalQuery{}
	for _, q := range queries {
		byID[q.ID] = q
	}
	tops := map[string][][]string{} // запрос → топы конфигураций
	var ids []string
	for _, c := range run.Configs {
		for _, pq := range c.PerQuery {
			if _, ok := tops[pq.ID]; !ok {
				ids = append(ids, pq.ID)
			}
			tops[pq.ID] = append(tops[pq.ID], pq.Top)
		}
	}
	sort.Strings(ids)
	seen := map[string]bool{}
	var pairs []*judgePairIn
	depth := 0
	for r := 0; r < judgeTopK; r++ {
		var layer []*judgePairIn
		deeper := false
		for _, id := range ids {
			for _, top := range tops[id] {
				deeper = deeper || r < len(top)
				if r >= len(top) || seen[id+"|"+top[r]] {
					continue
				}
				seen[id+"|"+top[r]] = true
				layer = append(layer, &judgePairIn{q: byID[id], pid: top[r], rank: r + 1})
			}
		}
		if !deeper {
			break // все топы короче: дальше пар нет
		}
		if len(pairs)+len(layer) > maxPairs {
			// Неполный слой: берём сколько влезет (в согласие входят, в NDCG — нет).
			pairs = append(pairs, layer[:maxPairs-len(pairs)]...)
			break
		}
		pairs = append(pairs, layer...)
		depth = r + 1
	}
	return pairs, depth
}

func judgeShuffleKey(p *judgePairIn) string {
	sum := sha256.Sum256([]byte(p.q.ID + "|" + p.pid))
	return string(sum[:8])
}

// judgeCard — товар для судьи: то, что видит покупатель в карточке.
type judgeCard struct {
	Title       string `json:"title"`
	Vendor      string `json:"vendor"`
	Category    string `json:"category"`
	License     string `json:"license"`
	Origin      string `json:"origin"`
	Description string `json:"description"`
}

type judgeItem struct {
	ID      string    `json:"id"`
	Query   string    `json:"query"`
	Product judgeCard `json:"product"`
}

// judgeBatch — один вызов судьи на пачку пар (с одним повтором при сбое или неполном ответе).
func (s *Services) judgeBatch(ctx context.Context, system string, lx *searchLexicon, batch []*judgePairIn) (float64, int, error) {
	items := make([]judgeItem, 0, len(batch))
	byID := map[string]*judgePairIn{}
	for i, p := range batch {
		d, ok := lx.docs[p.pid]
		if !ok {
			continue
		}
		origin := "зарубежное ПО"
		if d.Origin == "ru" {
			origin = "отечественное ПО"
		}
		id := fmt.Sprintf("p%d", i+1)
		byID[id] = p
		items = append(items, judgeItem{ID: id, Query: p.q.Text, Product: judgeCard{
			Title: d.Title, Vendor: d.Vendor, Category: d.CategoryPath, License: licenseLine(d), Origin: origin,
			Description: judgeTruncate(d.Description, judgeMaxDescLen),
		}})
	}
	if len(items) == 0 {
		return 0, 0, nil
	}
	raw, err := json.Marshal(map[string]any{"pairs": items})
	if err != nil {
		return 0, 0, err
	}
	msgs := []llm.Message{{Role: "system", Content: system}, {Role: "user", Content: "<pairs>" + string(raw) + "</pairs>"}}
	zero := 0.0
	var cost float64
	var lastErr error
	for attempt := 0; attempt < 2; attempt++ {
		cctx, cancel := context.WithTimeout(ctx, judgeCallLimit)
		t := time.Now()
		resp, err := s.judge.Chat(cctx, llm.Request{Messages: msgs, MaxTokens: judgeMaxTokens, Temperature: &zero,
			ResponseFormat: llm.JSONObject, ReasoningEffort: "none"})
		cancel()
		call := llmCall{Model: s.judge.Model(), Latency: time.Since(t), Status: llmStatusOK}
		if resp != nil {
			call.InputTokens, call.OutputTokens, call.CachedTokens = resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.CachedTokens()
		}
		var missing int
		if err == nil {
			missing, err = applyJudgeGrades(resp.Message.Content, byID)
			if err != nil {
				call.Status = llmStatusSchemaError
			}
		} else {
			call.Status = llmStatusError
			if errors.Is(err, context.DeadlineExceeded) {
				call.Status = llmStatusTimeout
			}
		}
		cost += s.recordLLMUsage(ctx, call)
		if err == nil && missing == 0 {
			return cost, attempt + 1, nil
		}
		if err == nil {
			err = fmt.Errorf("судья оценил не все пары: нет %d", missing)
		}
		lastErr = err
	}
	return cost, 2, lastErr
}

// applyJudgeGrades — оценки из ответа судьи; missing — пары без оценки.
func applyJudgeGrades(content string, byID map[string]*judgePairIn) (int, error) {
	content = strings.TrimSpace(content)
	content = strings.TrimPrefix(strings.TrimPrefix(content, "```json"), "```")
	content = strings.TrimSpace(strings.TrimSuffix(content, "```"))
	// Qwen изредка портит имя ключа («/grades»): берётся ключ, похожий на grades, иначе единственный массив.
	var top map[string]json.RawMessage
	if err := json.Unmarshal([]byte(content), &top); err != nil {
		return len(byID), fmt.Errorf("судья: ответ не JSON: %w", err)
	}
	var arr json.RawMessage
	for k, v := range top {
		if strings.Contains(strings.ToLower(k), "grades") {
			arr = v
			break
		}
		if len(top) == 1 {
			arr = v
		}
	}
	var grades []struct {
		ID    string `json:"id"`
		Grade any    `json:"grade"`
	}
	if len(arr) == 0 || json.Unmarshal(arr, &grades) != nil {
		return len(byID), errors.New("судья: в ответе нет массива grades")
	}
	for _, g := range grades {
		p, ok := byID[g.ID]
		if !ok || p.grade != nil {
			continue
		}
		var v int
		switch x := g.Grade.(type) {
		case float64:
			v = int(x)
			if float64(v) != x {
				continue
			}
		case string:
			if _, err := fmt.Sscanf(x, "%d", &v); err != nil {
				continue
			}
		default:
			continue
		}
		if v < 0 || v > 3 {
			continue
		}
		p.grade = &v
	}
	missing := 0
	for _, p := range byID {
		if p.grade == nil {
			missing++
		}
	}
	return missing, nil
}

// judgeAgreement — точное совпадение, ±1, взвешенная каппа и матрица по оценённым парам.
func judgeAgreement(a *out.JudgeAgreement, grades []out.JudgePair) {
	var o [4][4]float64
	n := 0.0
	exact, within := 0, 0
	for _, g := range grades {
		if g.Judge == nil {
			continue
		}
		l, j := min(max(g.Label, 0), 3), *g.Judge
		a.Confusion[l][j]++
		o[l][j]++
		n++
		if l == j {
			exact++
		}
		if l-j <= 1 && j-l <= 1 {
			within++
		}
	}
	a.Judged = int(n)
	if n == 0 {
		return
	}
	a.Exact = evalPtr(evalRound4(float64(exact) / n))
	a.Within1 = evalPtr(evalRound4(float64(within) / n))
	a.Kappa = quadraticKappa(o)
}

// quadraticKappa — каппа Коэна с квадратичными весами для шкалы 0–3; nil — не определена (одна категория).
func quadraticKappa(o [4][4]float64) *float64 {
	var rows, cols [4]float64
	n := 0.0
	for i := range 4 {
		for j := range 4 {
			rows[i] += o[i][j]
			cols[j] += o[i][j]
			n += o[i][j]
		}
	}
	if n == 0 {
		return nil
	}
	num, den := 0.0, 0.0
	for i := range 4 {
		for j := range 4 {
			w := float64((i-j)*(i-j)) / 9
			num += w * o[i][j]
			den += w * rows[i] * cols[j] / n
		}
	}
	if den == 0 {
		return nil
	}
	return evalPtr(evalRound4(1 - num/den))
}

// judgeByConfig — NDCG@depth каждой конфигурации на пуле: по разметке и по судье, с одинаковым идеалом
// (лучшие оценки пула запроса). Запросы без оценённых пар в пуле не входят.
func judgeByConfig(run *out.EvalRun, pairs []*judgePairIn, depth int) ([]out.JudgeConfig, bool) {
	res := []out.JudgeConfig{}
	if depth == 0 {
		return res, false
	}
	type grades struct{ label, judge map[string]int }
	pool := map[string]*grades{}
	for _, p := range pairs {
		if p.grade == nil || p.rank > depth {
			continue
		}
		g := pool[p.q.ID]
		if g == nil {
			g = &grades{label: map[string]int{}, judge: map[string]int{}}
			pool[p.q.ID] = g
		}
		g.label[p.pid] = p.q.Judgments[p.pid]
		g.judge[p.pid] = *p.grade
	}
	ideal := func(m map[string]int) float64 {
		var vs []int
		for _, v := range m {
			vs = append(vs, v)
		}
		sort.Sort(sort.Reverse(sort.IntSlice(vs)))
		return judgeDCG(vs[:min(depth, len(vs))])
	}
	ndcg := func(top []string, m map[string]int) (float64, bool) {
		id := ideal(m)
		if id == 0 {
			return 0, false
		}
		var vs []int
		for _, pid := range top[:min(depth, len(top))] {
			vs = append(vs, m[pid])
		}
		return judgeDCG(vs) / id, true
	}
	for _, c := range run.Configs {
		var sl, sj float64
		var nl, nj int
		for _, pq := range c.PerQuery {
			g := pool[pq.ID]
			if g == nil {
				continue
			}
			if v, ok := ndcg(pq.Top, g.label); ok {
				sl += v
				nl++
			}
			if v, ok := ndcg(pq.Top, g.judge); ok {
				sj += v
				nj++
			}
		}
		jc := out.JudgeConfig{Config: c.Config}
		if nl > 0 {
			jc.NDCGLabels = evalPtr(evalRound4(sl / float64(nl)))
		}
		if nj > 0 {
			jc.NDCGJudge = evalPtr(evalRound4(sj / float64(nj)))
		}
		res = append(res, jc)
	}
	return res, sameOrder(res)
}

// sameOrder — конфигурации упорядочены по судье так же, как по разметке.
func sameOrder(cs []out.JudgeConfig) bool {
	val := func(p *float64) float64 {
		if p == nil {
			return math.Inf(-1)
		}
		return *p
	}
	for i := range cs {
		for j := i + 1; j < len(cs); j++ {
			dl := val(cs[i].NDCGLabels) - val(cs[j].NDCGLabels)
			dj := val(cs[i].NDCGJudge) - val(cs[j].NDCGJudge)
			if math.Abs(dl) < judgeOrderEps || math.Abs(dj) < judgeOrderEps {
				continue // разница меньше шума — не порядок
			}
			if dl*dj < 0 {
				return false
			}
		}
	}
	return true
}

func judgeDCG(gs []int) float64 {
	s := 0.0
	for i, g := range gs {
		s += (math.Pow(2, float64(g)) - 1) / math.Log2(float64(i+2))
	}
	return s
}

func judgeTruncate(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n]) + "…"
}

// latestJudge — последний прогон судьи на наборе для /api/evaluations; nil — не было.
func (s *Services) latestJudge(ctx context.Context, set string) (*out.JudgeAgreement, error) {
	row, err := s.repos.LatestJudgeRun(ctx, set)
	if err != nil || row == nil {
		return nil, err
	}
	var a out.JudgeAgreement
	if err := json.Unmarshal(row.Agreement, &a); err != nil {
		return nil, fmt.Errorf("judge_runs %s: %w", row.RunID, err)
	}
	return &a, nil
}
