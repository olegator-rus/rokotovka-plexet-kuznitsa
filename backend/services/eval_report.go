package services

import (
	"fmt"
	"slices"
	"sort"
	"strings"

	"kuznica/api/out"
)

// FormatEvalTable — прогон текстом для stdout и отчётов: строки — метрики, столбцы — конфигурации
// (в скобках — изменение к предыдущему столбцу), ниже — NDCG@10 по типам, время по маршрутам,
// ухудшения рядом с улучшениями и строка воспроизводимости.
func FormatEvalTable(run *out.EvalRun) string {
	if run == nil {
		return ""
	}
	var b strings.Builder
	fmt.Fprintf(&b, "Прогон %s · %s\n\n", run.RunID, run.Repro)
	head := []string{"Метрика"}
	for _, c := range run.Configs {
		head = append(head, c.Config)
	}
	rows := [][]string{head, evalSeparator(len(head))}
	type metric struct {
		name   string
		get    func(out.EvalMetrics) *float64
		better int // +1 — больше лучше, −1 — меньше лучше
		pct    bool
	}
	metrics := []metric{
		{"Пустые выдачи", func(m out.EvalMetrics) *float64 { return m.EmptyRate }, -1, true},
		{"Пустые (по частоте)", func(m out.EvalMetrics) *float64 { return m.EmptyRateWeighted }, -1, true},
		{"Нет релевантного в топ-10", func(m out.EvalMetrics) *float64 { return m.NoRelevantTop10 }, -1, true},
		{"NDCG@10", func(m out.EvalMetrics) *float64 { return m.NDCG10 }, 1, false},
		{"MRR@10", func(m out.EvalMetrics) *float64 { return m.MRR10 }, 1, false},
		{"Recall@50 (размеч.)", func(m out.EvalMetrics) *float64 { return m.Recall50 }, 1, false},
		{"Точные запросы: 1-е место", func(m out.EvalMetrics) *float64 { return m.ExactAccuracy }, 1, true},
		{"Спасённые проблемные", func(m out.EvalMetrics) *float64 { return m.RescuedRate }, 1, true},
		{"Ложное спасение", func(m out.EvalMetrics) *float64 { return m.FalseRescueRate }, -1, true},
		{"Нарушения условий", func(m out.EvalMetrics) *float64 { return m.ViolationRate }, -1, true},
	}
	for _, m := range metrics {
		row := []string{m.name}
		var prev *float64
		for i, c := range run.Configs {
			v := m.get(c.Metrics)
			row = append(row, evalCell(v, prev, i > 0, m.pct))
			prev = v
		}
		rows = append(rows, row)
	}
	lat := []string{"Время P50 / P95, мс"}
	for _, c := range run.Configs {
		lat = append(lat, fmt.Sprintf("%.1f / %.1f", c.Metrics.Latency.P50, c.Metrics.Latency.P95))
	}
	regs := []string{"Ухудшений / улучшений"}
	for i, c := range run.Configs {
		if i == 0 {
			regs = append(regs, "—")
			continue
		}
		regs = append(regs, fmt.Sprintf("%d / %d", len(c.Regressions), len(c.Improvements)))
	}
	errsRow := []string{"Ошибок поиска"}
	for _, c := range run.Configs {
		errsRow = append(errsRow, fmt.Sprint(c.Metrics.Errors))
	}
	rows = append(rows, lat, regs, errsRow)
	evalWriteTable(&b, rows)

	// NDCG@10 по типам запросов.
	types := map[string]int{}
	for _, c := range run.Configs {
		for t, m := range c.ByType {
			types[t] = max(types[t], m.Queries)
		}
	}
	if len(types) > 0 {
		names := make([]string, 0, len(types))
		for t := range types {
			names = append(names, t)
		}
		sort.Strings(names)
		b.WriteString("\nNDCG@10 по типам (в скобках — есть релевантное в топ-10)\n\n")
		trows := [][]string{append([]string{"Тип (запросов)"}, head[1:]...), evalSeparator(len(head))}
		for _, t := range names {
			row := []string{fmt.Sprintf("%s (%d)", t, types[t])}
			for _, c := range run.Configs {
				m := c.ByType[t]
				cell := "—"
				if m.NDCG10 != nil {
					cell = fmt.Sprintf("%.3f", *m.NDCG10)
				}
				if m.RelevantTop10 != nil {
					cell += fmt.Sprintf(" (%.0f%%)", *m.RelevantTop10*100)
				}
				row = append(row, cell)
			}
			trows = append(trows, row)
		}
		evalWriteTable(&b, trows)
	}

	// Время по маршрутам.
	routes := map[string]bool{}
	for _, c := range run.Configs {
		for r := range c.Metrics.ByRoute {
			routes[r] = true
		}
	}
	if len(routes) > 1 {
		names := make([]string, 0, len(routes))
		for r := range routes {
			names = append(names, r)
		}
		slices.Sort(names)
		b.WriteString("\nВремя по маршрутам, P50 / P95 мс (запросов)\n\n")
		rrows := [][]string{append([]string{"Маршрут"}, head[1:]...), evalSeparator(len(head))}
		for _, r := range names {
			row := []string{r}
			for _, c := range run.Configs {
				l, ok := c.Metrics.ByRoute[r]
				if !ok {
					row = append(row, "—")
					continue
				}
				row = append(row, fmt.Sprintf("%.1f / %.1f (%d)", l.P50, l.P95, l.Count))
			}
			rrows = append(rrows, row)
		}
		evalWriteTable(&b, rrows)
	}

	// Ухудшения рядом с улучшениями.
	for i, c := range run.Configs {
		if i == 0 {
			continue
		}
		fmt.Fprintf(&b, "\n%s против %s: ухудшений %d, улучшений %d\n", c.Config, c.Against, len(c.Regressions), len(c.Improvements))
		for _, ch := range evalHead5(c.Regressions) {
			fmt.Fprintf(&b, "  − %s [%s, freq %d] %q: %s, ΔNDCG %+.3f\n", ch.QueryID, ch.Type, ch.Freq, ch.Text, strings.Join(ch.Reasons, ", "), ch.NDCGDelta)
		}
		for _, ch := range evalHead5(c.Improvements) {
			fmt.Fprintf(&b, "  + %s [%s, freq %d] %q: %s, ΔNDCG %+.3f\n", ch.QueryID, ch.Type, ch.Freq, ch.Text, strings.Join(ch.Reasons, ", "), ch.NDCGDelta)
		}
		for _, w := range c.Warnings {
			fmt.Fprintf(&b, "  ! %s\n", w)
		}
	}
	if run.File != "" {
		fmt.Fprintf(&b, "\nJSON: %s\n", run.File)
	}
	return b.String()
}

func evalHead5(list []out.EvalChange) []out.EvalChange {
	if len(list) > 5 {
		return list[:5]
	}
	return list
}

func evalSeparator(n int) []string {
	s := make([]string, n)
	for i := range s {
		s[i] = "---"
	}
	return s
}

// evalCell — значение и изменение к предыдущему столбцу.
func evalCell(v, prev *float64, withDelta, pct bool) string {
	if v == nil {
		return "—"
	}
	scale, unit, digits := 1.0, "", 3
	if pct {
		scale, unit, digits = 100, "%", 1
	}
	s := fmt.Sprintf("%.*f%s", digits, *v*scale, unit)
	if withDelta && prev != nil {
		s += fmt.Sprintf(" (%+.*f)", digits, (*v-*prev)*scale)
	}
	return s
}

// evalWriteTable — markdown-таблица: читается и в терминале, и в отчётах.
func evalWriteTable(b *strings.Builder, rows [][]string) {
	for _, r := range rows {
		b.WriteString("| " + strings.Join(r, " | ") + " |\n")
	}
}
