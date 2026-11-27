package services

import (
	"fmt"
	"math"
	"slices"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Отчёт о собранных наборах (SETS.md) и сверка с разметкой по построению (mock): совпадение неявной оценки,
// оценки судьи и итоговой разметки с эталоном, точность типов, целостность кластеров, лестница B0–B3 по обеим
// разметкам.

// SetsReport — что собрано и из чего.
type SetsReport struct {
	Generated  time.Time `json:"generated"`
	MinFreq    int       `json:"min_freq"`
	MinSupport int       `json:"min_support"`
	ClickGrade int       `json:"click_grade"`
	Days       int       `json:"days"`

	LogSearches  int       `json:"log_searches"`
	LogActions   int       `json:"log_actions"`
	LogTexts     int       `json:"log_texts"`
	LogFrom      time.Time `json:"log_from"`
	LogTo        time.Time `json:"log_to"`
	Unattributed int       `json:"unattributed_actions"`
	BelowFreq    int       `json:"below_min_freq"`
	HoldoutFrom  time.Time `json:"holdout_from"`
	HoldoutTo    time.Time `json:"holdout_to"`
	// UnknownProducts — действия с товарами, которых нет в каталоге; LowSupport — пар ниже минимальной поддержки.
	UnknownProducts int `json:"unknown_products"`
	LowSupport      int `json:"low_support"`
	// TargetQueries — exact/sku-запросов с целевым семейством; TargetRaised — неявных оценок 2 → 3 по правилу цели.
	TargetQueries int `json:"target_queries"`
	TargetRaised  int `json:"target_raised"`

	Queries       int                       `json:"queries"`
	Clusters      int                       `json:"clusters"`
	Freq          int                       `json:"freq"`
	BySet         map[string]int            `json:"by_set"`
	ClustersBySet map[string]int            `json:"clusters_by_set"`
	ByType        map[string]map[string]int `json:"by_type"` // тип → набор → запросов
	FreqByType    map[string]int            `json:"freq_by_type"`
	Labels        map[string]int            `json:"labels"`   // implicit | judge → оценок > 0
	Grades        map[string][4]int         `json:"grades"`   // источник → распределение 0–3
	Problems      map[string]int            `json:"problems"` // empty | no_click
	Unlabeled     int                       `json:"unlabeled_queries"`
	HoldoutOnly   int                       `json:"holdout_only_queries"`

	Classify     classifyStats `json:"classify"`
	Judge        *SetsJudge    `json:"judge,omitempty"`
	JudgeSkipped string        `json:"judge_skipped,omitempty"`
	Reference    *SetsRefCheck `json:"reference,omitempty"`
	DurationMs   int64         `json:"duration_ms"`
}

// fill — сводка по запросам.
func (r *SetsReport) fill(qs []*DerivedQuery) {
	r.Queries = len(qs)
	r.BySet, r.ClustersBySet, r.ByType, r.FreqByType = map[string]int{}, map[string]int{}, map[string]map[string]int{}, map[string]int{}
	r.Labels, r.Grades, r.Problems = map[string]int{}, map[string][4]int{}, map[string]int{}
	clusters := map[string]string{}
	for _, q := range qs {
		r.Freq += q.Freq
		r.BySet[q.Set]++
		clusters[q.Cluster] = q.Set
		if r.ByType[q.Type] == nil {
			r.ByType[q.Type] = map[string]int{}
		}
		r.ByType[q.Type][q.Set]++
		r.FreqByType[q.Type] += q.Freq
		if q.Problem != "" {
			r.Problems[q.Problem]++
		}
		positive := false
		for _, g := range q.Judgments {
			positive = positive || g > 0
		}
		if !positive {
			r.Unlabeled++
		}
		if !r.HoldoutFrom.IsZero() {
			if f, err := time.Parse(time.RFC3339, q.FirstSeen); err == nil && !f.Before(r.HoldoutFrom) {
				r.HoldoutOnly++
			}
		}
		for pid, src := range q.Sources {
			g := q.Judgments[pid]
			d := r.Grades[src]
			d[min(max(g, 0), 3)]++
			r.Grades[src] = d
			if g > 0 {
				r.Labels[src]++
			}
		}
	}
	r.Clusters = len(clusters)
	for _, set := range clusters {
		r.ClustersBySet[set]++
	}
}

// SetsRefCheck — сверка с разметкой по построению.
type SetsRefCheck struct {
	Matched     int `json:"matched"`      // запросов набора, чей текст есть в эталоне
	RefQueries  int `json:"ref_queries"`  // запросов в эталоне
	RefCovered  int `json:"ref_covered"`  // из них попали в набор
	TypeCorrect int `json:"type_correct"` // тип совпал с эталонным
	// TypeConfusion — «эталон → наш» для несовпавших, с числом.
	TypeConfusion map[string]int `json:"type_confusion"`
	// ImplicitVsRef, JudgeVsRef, FinalVsRef — согласие с эталоном: неявной оценки, судьи и итоговой разметки
	// (неявная + судья) на парах пула топ-10.
	ImplicitVsRef out.JudgeAgreement `json:"implicit_vs_ref"`
	JudgeVsRef    out.JudgeAgreement `json:"judge_vs_ref"`
	FinalVsRef    out.JudgeAgreement `json:"final_vs_ref"`
	// ClustersWhole — эталонных кластеров (≥2 запроса в наборе), целиком попавших в один наш набор.
	ClustersMulti int `json:"clusters_multi"`
	ClustersWhole int `json:"clusters_whole"`
	// Ladder — NDCG@10 B0–B3 на нашем test по нашей и по эталонной разметке.
	Ladder    []out.JudgeConfig `json:"ladder"`
	LadderN   int               `json:"ladder_queries"`
	SameOrder bool              `json:"same_order"`
}

// compareReference — сверка собранных наборов с разметкой по построению по тексту запроса.
func compareReference(qs []*DerivedQuery, ref []domain.EvalQuery, pool *out.EvalRun, judge *SetsJudge, family func(string) string) *SetsRefCheck {
	byText := map[string]domain.EvalQuery{}
	for _, q := range ref {
		k := foldQuery(q.Text)
		if old, ok := byText[k]; !ok || old.ClientID != nil && q.ClientID == nil {
			byText[k] = q
		}
	}
	c := &SetsRefCheck{RefQueries: len(byText), TypeConfusion: map[string]int{}, Ladder: []out.JudgeConfig{}}
	matched := map[string]domain.EvalQuery{}
	var implicit, judged, final []out.JudgePair
	clusterSets := map[string]map[string]bool{}
	clusterN := map[string]int{}
	for _, q := range qs {
		r, ok := byText[foldQuery(q.Text)]
		if !ok {
			continue
		}
		matched[q.ID] = r
		c.Matched++
		if q.Type == r.Type {
			c.TypeCorrect++
		} else {
			c.TypeConfusion[r.Type+" → "+q.Type]++
		}
		if r.Cluster != "" {
			if clusterSets[r.Cluster] == nil {
				clusterSets[r.Cluster] = map[string]bool{}
			}
			clusterSets[r.Cluster][q.Set] = true
			clusterN[r.Cluster]++
		}
		for pid, src := range q.Sources {
			if src == labelImplicit {
				g := q.Judgments[pid]
				implicit = append(implicit, out.JudgePair{QueryID: q.ID, ProductID: pid, Label: r.Judgments[pid], Judge: &g})
			}
		}
		if judge != nil {
			for k, g := range judge.grades {
				if qid, pid, _ := strings.Cut(k, "|"); qid == q.ID {
					g := g
					judged = append(judged, out.JudgePair{QueryID: q.ID, ProductID: pid, Label: r.Judgments[pid], Judge: &g})
				}
			}
		}
	}
	c.RefCovered = c.Matched
	for cl, sets := range clusterSets {
		if clusterN[cl] >= 2 {
			c.ClustersMulti++
			if len(sets) == 1 {
				c.ClustersWhole++
			}
		}
	}
	derived := map[string]*DerivedQuery{}
	for _, q := range qs {
		derived[q.ID] = q
	}
	if pool != nil {
		seen := map[string]bool{}
		for _, cfg := range pool.Configs {
			for _, pq := range cfg.PerQuery {
				r, ok := matched[pq.ID]
				if !ok {
					continue
				}
				q := derived[pq.ID]
				for _, pid := range pq.Top {
					if seen[pq.ID+"|"+pid] {
						continue
					}
					seen[pq.ID+"|"+pid] = true
					if _, labeled := q.Sources[pid]; !labeled {
						continue
					}
					g := q.Judgments[pid]
					final = append(final, out.JudgePair{QueryID: pq.ID, ProductID: pid, Label: r.Judgments[pid], Judge: &g})
				}
			}
		}
		c.Ladder, c.LadderN = refLadder(pool, derived, matched, family)
		c.SameOrder = sameOrder(c.Ladder)
	}
	judgeAgreement(&c.ImplicitVsRef, implicit)
	judgeAgreement(&c.JudgeVsRef, judged)
	judgeAgreement(&c.FinalVsRef, final)
	return c
}

// refLadder — NDCG@10 конфигураций пула на нашем test: NDCGJudge — по нашей разметке, NDCGLabels — по эталону.
// Идеал — как в оценке (eval_metrics.go): лучшая оценка каждого семейства.
func refLadder(pool *out.EvalRun, derived map[string]*DerivedQuery, matched map[string]domain.EvalQuery, family func(string) string) ([]out.JudgeConfig, int) {
	ndcg := func(top []string, m map[string]int) (float64, bool) {
		fam := map[string]int{}
		for pid, v := range m {
			f := family(pid)
			if f == "" {
				f = pid
			}
			fam[f] = max(fam[f], v)
		}
		var ideal, got []int
		for _, v := range fam {
			ideal = append(ideal, v)
		}
		for _, pid := range top[:min(evalTopK, len(top))] {
			got = append(got, m[pid])
		}
		v := evalNDCG(got, ideal, evalTopK)
		if v == nil {
			return 0, false
		}
		return *v, true
	}
	res := []out.JudgeConfig{}
	n := 0
	for _, cfg := range pool.Configs {
		var sd, sr float64
		var nd, nr int
		for _, pq := range cfg.PerQuery {
			q, r := derived[pq.ID], matched[pq.ID]
			if q == nil || q.Set != domain.EvalSetTest || r.ID == "" {
				continue
			}
			// Обе разметки должны быть определены, иначе сравниваются разные запросы.
			vd, okd := ndcg(pq.Top, q.Judgments)
			vr, okr := ndcg(pq.Top, r.Judgments)
			if !okd || !okr {
				continue
			}
			sd, sr = sd+vd, sr+vr
			nd++
			nr++
		}
		jc := out.JudgeConfig{Config: cfg.Config}
		if nd > 0 {
			jc.NDCGJudge = evalPtr(evalRound4(sd / float64(nd)))
			jc.NDCGLabels = evalPtr(evalRound4(sr / float64(nr)))
		}
		n = max(n, nd)
		res = append(res, jc)
	}
	return res, n
}

// FormatSetsMarkdown — SETS.md: сколько, типы, наборы, источники разметки, сверка.
func FormatSetsMarkdown(r *SetsReport, source string) string {
	var b strings.Builder
	p := func(f string, a ...any) { fmt.Fprintf(&b, f, a...) }
	p("# Наборы оценки из журнала — %s\n\n", source)
	p("Собрано `go run . buildsets` %s. поля `type_source`, `judgment_sources`,\n", r.Generated.Format("2006-01-02 15:04 UTC"))
	p("`problem`, `searches`, `empty_share`, `click_share`, `first_seen` — происхождение записи.\n\n")
	p("## Журнал\n\n")
	p("| Что | Сколько |\n| --- | --- |\n")
	p("| Период | %s — %s |\n", r.LogFrom.Format("2006-01-02"), r.LogTo.Format("2006-01-02"))
	if r.Days > 0 {
		p("| Окно (--days) | %d дн. |\n", r.Days)
	}
	p("| Поисков | %d |\n| Различных запросов | %d |\n| Действий с товаром после поиска | %d |\n", r.LogSearches, r.LogTexts, r.LogActions)
	p("| Действий без поиска (не размечают) | %d |\n| Запросов с частотой < %d (не вошли) | %d |\n", r.Unattributed, r.MinFreq, r.BelowFreq)
	p("| Товаров не из каталога (отброшены) | %d |\n| Пар ниже поддержки %d | %d |\n", r.UnknownProducts, r.MinSupport, r.LowSupport)
	p("| Отложенная неделя (как у агента) | %s — %s |\n\n", r.HoldoutFrom.In(mskZone).Format("2006-01-02"), r.HoldoutTo.Add(-time.Second).In(mskZone).Format("2006-01-02"))

	p("## Наборы\n\n")
	p("Запросов %d в %d кластерах, частота %d. Деление по кластерам (нормализованный текст, переформулировки ≥ %d,\n", r.Queries, r.Clusters, r.Freq, clusterReformMin)
	p("общие выбранные товары ≥ %.0f %%, векторы cos ≥ %.2f), внутри типа 40 / 40 / 20 %% запросов; кластеры, впервые появившиеся в отложенной неделе (%d запр.), — только в test.\n", setsProductJaccard*100, setsCosine, r.HoldoutOnly)
	p("Тексты test исключаются из входа агента автоматически (AGENT_EXCLUDE_TEST), реплей недели — `make eval SET=holdout_week`.\n\n")
	sets := []string{domain.EvalSetTune, domain.EvalSetTest, domain.EvalSetRegression}
	p("| Тип | tune | test | regression | Всего | Поисков |\n| --- | --- | --- | --- | --- | --- |\n")
	types := make([]string, 0, len(r.ByType))
	for t := range r.ByType {
		types = append(types, t)
	}
	sort.Slice(types, func(i, j int) bool { return r.FreqByType[types[i]] > r.FreqByType[types[j]] })
	for _, t := range types {
		tot := 0
		p("| %s", t)
		for _, s := range sets {
			p(" | %d", r.ByType[t][s])
			tot += r.ByType[t][s]
		}
		p(" | %d | %d |\n", tot, r.FreqByType[t])
	}
	p("| **всего** | %d | %d | %d | %d | %d |\n\n", r.BySet[sets[0]], r.BySet[sets[1]], r.BySet[sets[2]], r.Queries, r.Freq)
	p("Кластеров: tune %d, test %d, regression %d. Проблемных запросов: пустая выдача ≥ %.0f %% — %d, без кликов — %d.\n\n",
		r.ClustersBySet[sets[0]], r.ClustersBySet[sets[1]], r.ClustersBySet[sets[2]], setsEmptyProblem*100, r.Problems[problemEmpty], r.Problems[problemNoClick])

	p("## Типы запросов\n\n")
	cs := r.Classify
	p("Правила (нормализатор, атрибуты, сущности поиска): %d; модель: %d; кэш: %d; по умолчанию: %d.", cs.Rules, cs.LLM, cs.Cache, cs.Default)
	if cs.Calls > 0 {
		p(" Модель %s, промпт %s, %d вызовов, %.2f ₽.", cs.Model, cs.PromptVersion, cs.Calls, cs.CostRub)
	}
	p("\n\n## Разметка\n\n")
	p("Неявная (по журналу): заказ 3, корзина или запрос цены 2; клик без корзины — %d, если после него сессия не искала\n", r.ClickGrade)
	p("сразу другое (за %d с), иначе 1; частый клик (≥ %.0f %% поисков запроса с поправкой на позицию: вес √pos, не больше %.0f;\n", int(reformulateWindow.Seconds()), setsClickShare2*100, setsPosBiasCap)
	p("не меньше двух) — 2. Минимальная поддержка — %d поиск(а).\n", r.MinSupport)
	p("Правило цели (exact/sku): неявная оценка 2 становится 3, если товар из целевого семейства — семейства с наибольшей\n")
	p("неявной поддержкой; при равенстве лидеров цели нет. Целевое семейство найдено у %d запросов, повышено оценок: %d.\n", r.TargetQueries, r.TargetRaised)
	p("Остальное — LLM-судья вслепую на пуле топ-10 B0–B4, по одному товару на семейство запроса; оценка семейства\n")
	p("переносится на все его артикулы в запросе. Судья может повысить неявную оценку (источник implicit+judge), но не понизить.\n\n")
	p("| Источник | 0 | 1 | 2 | 3 |\n| --- | --- | --- | --- | --- |\n")
	for _, src := range []string{labelImplicit, labelImplicitJudge, labelJudge} {
		g := r.Grades[src]
		p("| %s | %d | %d | %d | %d |\n", src, g[0], g[1], g[2], g[3])
	}
	p("\nЗапросов без единой оценки > 0: %d (пустые `judgments` — для absent/non_product/noise это ответ «нет»).\n\n", r.Unlabeled)
	if j := r.Judge; j != nil {
		p("Судья %s (%s): пул %d пар (из них с неявной оценкой %d), семейств в пуле %d, оценено %d (из кэша %d), добито пар %d (из них 0 — %d), повышено неявных %d, %d вызовов, %.2f ₽ из %.0f ₽.",
			j.Model, j.PromptVersion, j.PoolPairs, j.PoolImplicit, j.PoolFamilies, j.Judged, j.Cached, j.Filled, j.FilledZero, j.Raised, j.Calls, j.CostRub, j.BudgetRub)
		if j.Stopped != "" {
			p(" Остановлен: %s.", j.Stopped)
		}
		p("\n\nСогласие судьи с кликами не публикуется: неявные оценки почти все 2, каппа против них вырождена. Калибровка судьи —\n")
		p("на эталонах заказчика и их соседях (`go run . judge-calib`).\n\n")
	} else if r.JudgeSkipped != "" {
		p("Судья не вызывался: %s.\n\n", r.JudgeSkipped)
	}
	if c := r.Reference; c != nil {
		p("## Сверка с разметкой по построению\n\n")
		p("Совпало по тексту %d запросов из %d эталонных. Тип совпал у %d (%.1f %%).\n\n", c.Matched, c.RefQueries, c.TypeCorrect, pct(c.TypeCorrect, c.Matched))
		p("| Что сравнивается с эталоном | Пар | Точно | ±1 | Каппа (квадр.) |\n| --- | --- | --- | --- | --- |\n")
		for _, row := range []struct {
			name string
			a    out.JudgeAgreement
		}{{"неявная разметка", c.ImplicitVsRef}, {"судья", c.JudgeVsRef}, {"итог на пуле топ-10 (неявная + судья)", c.FinalVsRef}} {
			p("| %s | %d | %s | %s | %s |\n", row.name, row.a.Judged, fmtShare(row.a.Exact), fmtShare(row.a.Within1), fmtNum(row.a.Kappa))
		}
		p("\nКластеры эталона, у которых в наборе ≥ 2 запроса: %d, из них целиком в одном нашем наборе — %d.\n\n", c.ClustersMulti, c.ClustersWhole)
		if len(c.Ladder) > 0 {
			p("Лестница на нашем test (%d запросов с обеими разметками), NDCG@10:\n\n| Конфигурация | наша разметка | эталон |\n| --- | --- | --- |\n", c.LadderN)
			for _, l := range c.Ladder {
				p("| %s | %s | %s |\n", l.Config, fmtNum(l.NDCGJudge), fmtNum(l.NDCGLabels))
			}
			order := "тот же"
			if !c.SameOrder {
				order = "другой"
			}
			p("\nПорядок конфигураций по нашей разметке — %s, что по эталону.\n\n", order)
		}
		if len(c.TypeConfusion) > 0 {
			type kv struct {
				k string
				n int
			}
			var xs []kv
			for k, n := range c.TypeConfusion {
				xs = append(xs, kv{k, n})
			}
			sort.Slice(xs, func(i, j int) bool { return xs[i].n > xs[j].n || xs[i].n == xs[j].n && xs[i].k < xs[j].k })
			p("Частые расхождения типов (эталон → наш): ")
			var parts []string
			for _, x := range xs[:min(10, len(xs))] {
				parts = append(parts, fmt.Sprintf("%s %d", x.k, x.n))
			}
			p("%s.\n\n", strings.Join(parts, "; "))
		}
	}
	p("Время сборки: %.1f с.\n", float64(r.DurationMs)/1000)
	return b.String()
}

func agreementLine(a out.JudgeAgreement) string {
	return fmt.Sprintf("точно %s, ±1 %s, каппа %s", fmtShare(a.Exact), fmtShare(a.Within1), fmtNum(a.Kappa))
}

func fmtShare(v *float64) string {
	if v == nil {
		return "—"
	}
	return fmt.Sprintf("%.1f %%", *v*100)
}

func fmtNum(v *float64) string {
	if v == nil || math.IsNaN(*v) {
		return "—"
	}
	return fmt.Sprintf("%.3f", *v)
}

func pct(n, d int) float64 {
	if d == 0 {
		return 0
	}
	return float64(n) * 100 / float64(d)
}

// SetsQueriesJSON — запросы в порядке id (для записи queries.json).
func SetsQueriesJSON(qs []*DerivedQuery) []*DerivedQuery {
	res := slices.Clone(qs)
	sort.Slice(res, func(i, j int) bool { return res[i].ID < res[j].ID })
	return res
}
