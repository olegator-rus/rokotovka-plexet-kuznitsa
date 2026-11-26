package services

import (
	"fmt"
	"sort"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Отчёты прогона «по цели» (CLI): таблица в stdout и Markdown для data/softline/eval/report/. Дат в отчётах нет:
// прогон назван последними знаками run_id, каталог — источником и числом офферов.

// targetCfgLabel — подпись конфигурации.
func targetCfgLabel(id string) string {
	for _, c := range feedConfigs {
		if c.ID == id {
			return c.Label
		}
	}
	return id
}

func tgtShare(v *float64, n, of int) string {
	if v == nil || of == 0 {
		return "—"
	}
	return fmt.Sprintf("%d/%d (%.0f %%)", n, of, *v*100)
}

func fmtRatio(v *float64) string {
	if v == nil {
		return "—"
	}
	return feedNum(*v, 3)
}

func fmtMs(v float64) string { return fmt.Sprintf("%.0f", v) }

// fmtThousands — целое с пробелами между разрядами: 136040 → «136 040».
func fmtThousands(n int) string {
	s := fmt.Sprint(n)
	for i := len(s) - 3; i > 0; i -= 3 {
		s = s[:i] + " " + s[i:]
	}
	return s
}

// targetRubPerQuery — расход модели конфигурации на запрос: llm_usage, иначе трассы.
func targetRubPerQuery(c *out.TargetCost) string {
	if c == nil || c.Searches == 0 {
		return "—"
	}
	rub := c.TraceRub
	if c.UsageJoined {
		rub = c.LLMRub
	}
	return feedNum(rub/float64(c.Searches), 4)
}

var targetTableHead = []string{"конфигурация", "Success@1", "в первых 4", "MRR@10", "все условия", "≥4 семейств", "≥4 SKU",
	"нарушения", "p50, мс", "p95, мс", "₽/запрос", "D: метка / модель / кэш"}

func targetTableRow(c out.EvalConfigResult) []string {
	t := c.Metrics.Target
	if t == nil {
		return []string{c.Config, "—", "—", "—", "—", "—", "—", "—", "—", "—", "—", "—"}
	}
	viol := "—"
	if t.HardQueries > 0 {
		viol = fmt.Sprintf("%d из %d", t.HardViolated, t.HardQueries)
	}
	d := "—"
	if t.Cost != nil {
		d = fmt.Sprintf("%d / %d / %d", t.Cost.RouteD, t.Cost.ModelSearches, t.Cost.CacheSearches)
	}
	return []string{fmt.Sprintf("%s (%s)", targetCfgLabel(c.Config), c.Config), tgtShare(t.Success1, t.Success1N, t.Judged),
		tgtShare(t.Top4, t.Top4N, t.Judged), fmtRatio(t.MRR10), tgtShare(t.Pass, t.PassN, t.Answers),
		tgtShare(t.Families4, t.Families4N, t.Answers),
		tgtShare(t.SKUs4, t.SKUs4N, t.Answers), viol, fmtMs(c.Metrics.Latency.P50), fmtMs(c.Metrics.Latency.P95),
		targetRubPerQuery(t.Cost), d}
}

// FormatTargetTable — таблица прогона для stdout.
func FormatTargetTable(run *EvalResult) string {
	var b strings.Builder
	mode := ""
	if len(run.Configs) > 0 && run.Configs[0].Metrics.Target != nil && run.Configs[0].Metrics.Target.Cost != nil {
		mode = run.Configs[0].Metrics.Target.Cost.Mode
	}
	fmt.Fprintf(&b, "\nнабор %s: %d запросов, прогон …%s (%s)\n", run.Set, run.Queries, runTail(run.RunID), mode)
	rows := [][]string{targetTableHead}
	for _, c := range run.Configs {
		rows = append(rows, targetTableRow(c))
	}
	widths := make([]int, len(targetTableHead))
	for _, r := range rows {
		for i, c := range r {
			widths[i] = max(widths[i], len([]rune(c)))
		}
	}
	for _, r := range rows {
		for i, c := range r {
			b.WriteString(c + strings.Repeat(" ", widths[i]-len([]rune(c))+2))
		}
		b.WriteString("\n")
	}
	for _, c := range run.Configs {
		if c.Against != "" {
			fmt.Fprintf(&b, "  %s против %s: хуже %d, лучше %d\n", c.Config, c.Against, len(c.Regressions), len(c.Improvements))
		}
	}
	return b.String()
}

func mdRow(cells []string) string {
	for i, c := range cells {
		cells[i] = strings.ReplaceAll(c, "|", "\\|")
	}
	return "| " + strings.Join(cells, " | ") + " |\n"
}

func mdHead(cells ...string) string {
	return mdRow(cells) + "|" + strings.Repeat("---|", len(cells)) + "\n"
}

// FormatTargetReport — отчёт прогона в Markdown: сводная таблица, разбивки и промахи полного продукта.
func FormatTargetReport(run *EvalResult, command string, catalog out.FeedCatalog) string {
	var b strings.Builder
	mode := TargetModeWarm
	if len(run.Configs) > 0 && run.Configs[0].Metrics.Target != nil && run.Configs[0].Metrics.Target.Cost != nil {
		mode = run.Configs[0].Metrics.Target.Cost.Mode
	}
	modeName := map[string]string{TargetModeCold: "холодный (кэш разборов, векторов и реранкера не читается)",
		TargetModeWarm: "тёплый (второй проход: кэши разборов, векторов и реранкера заполнены первым)"}[mode]
	rules := 0
	for _, c := range run.Configs {
		rules = max(rules, c.RulesVersion)
	}
	fmt.Fprintf(&b, "# Оценка по цели — %s (%s)\n\n", targetSetLabel(run.Set), run.Set)
	fmt.Fprintf(&b, "Каталог: %s, %s офферов · правила v%d · коммит %s · хэш набора `%s` · %d запросов · прогон …%s · "+
		"режим: %s\n\n", catalog.Label, fmtThousands(catalog.Offers), rules, run.Commit, run.SetHash, run.Queries, runTail(run.RunID), modeName)
	if command != "" {
		fmt.Fprintf(&b, "Команда: `%s`\n\n", command)
	}
	b.WriteString("Success@1 — первая цель ответа (поле `target`, иначе первая карточка: URL семейства → оффера → вендора) " +
		"совпала с ожидаемой; URL сравниваются после нормализации (регистр хоста, схема, `www`, завершающий слэш, " +
		"URL-кодирование, `filter[category][]=` ≡ `[0]=`, порядок параметров). «В первых 4» — она же или карточка " +
		"ожидаемой цели среди первых четырёх; MRR@10 — по месту первой такой карточки. «Все условия» — Success@1 (у набора " +
		"понимания — проверка первых N карточек) и условия строки: нет исключённых семейств, есть «Можно купить сейчас», " +
		"нет нарушений жёстких условий. ≥4 семейств и SKU — доля ответов " +
		"(все карточки, включая «Популярное в наличии»). Нарушения — запросы с жёсткими условиями (продление, число мест, " +
		"тип организации, срок), где карточка в первых 10 их нарушает. D: метка / модель / кэш — поисков с меткой маршрута " +
		"D, с вызовом модели и с разбором из кэша; ₽/запрос — llm_usage прогона на запрос.\n\n")
	b.WriteString(mdHead(targetTableHead...))
	for _, c := range run.Configs {
		b.WriteString(mdRow(targetTableRow(c)))
	}
	b.WriteString("\n")
	for _, c := range run.Configs {
		if c.Against != "" {
			fmt.Fprintf(&b, "- %s против %s: хуже %d, лучше %d.\n", c.Config, c.Against, len(c.Regressions), len(c.Improvements))
		}
	}
	b.WriteString("\n")
	// Разбивки: Success@1 каждой конфигурации по виду искажения, вендору и части набора.
	for _, dim := range []struct{ key, title string }{{"group", "По частям набора"}, {"distortion", "По виду искажения"},
		{"vendor", "По вендорам"}} {
		keys := map[string]bool{}
		for _, c := range run.Configs {
			if c.Metrics.Target != nil {
				for k := range c.Metrics.Target.Breakdown[dim.key] {
					keys[k] = true
				}
			}
		}
		if len(keys) < 2 {
			continue
		}
		ks := make([]string, 0, len(keys))
		for k := range keys {
			ks = append(ks, k)
		}
		sort.Strings(ks)
		head := []string{dim.title + ": Success@1 (в первых 4)"}
		for _, c := range run.Configs {
			head = append(head, c.Config)
		}
		b.WriteString("## " + dim.title + "\n\n" + mdHead(head...))
		for _, k := range ks {
			row := []string{k}
			for _, c := range run.Configs {
				cell := "—"
				if t := c.Metrics.Target; t != nil {
					if v, ok := t.Breakdown[dim.key][k]; ok {
						cell = fmt.Sprintf("%d/%d (%d)", v.Success1, v.N, v.Top4)
					}
				}
				row = append(row, cell)
			}
			b.WriteString(mdRow(row))
		}
		b.WriteString("\n")
	}
	if len(run.Configs) == 0 {
		return b.String()
	}
	last := run.Configs[len(run.Configs)-1]
	fmt.Fprintf(&b, "## Все запросы — %s (%s)\n\n", targetCfgLabel(last.Config), last.Config)
	b.WriteString(mdHead("id", "запрос", "ожидаем", "Success@1", "в 4", "место", "первая цель", "условия", "маршрут",
		"сем. / SKU"))
	for _, q := range last.PerQuery {
		if q.Target == nil {
			b.WriteString(mdRow([]string{q.ID, q.Text, "—", "ошибка", "—", "—", q.Error, "—", "—", "—"}))
			continue
		}
		v := q.Target
		ok := "да"
		if !v.Strict {
			ok = "нет (" + v.MissReason + ")"
		}
		if v.ByCards {
			ok = "по карточкам: " + map[bool]string{true: "да", false: "нет"}[v.Strict]
		}
		rank := "—"
		if v.Rank > 0 {
			rank = fmt.Sprint(v.Rank)
		}
		first := strings.TrimSpace(v.FirstType + " " + targetURLKey(v.FirstURL))
		if v.FirstURL == "" && v.FirstTitle != "" {
			first = strings.TrimSpace(v.FirstType + " «" + v.FirstTitle + "»")
		}
		cond := "—"
		if len(v.Fails) > 0 && !(len(v.Fails) == 1 && v.Fails[0] == "target") {
			cond = strings.Join(v.Fails, ", ")
		}
		if v.Violations > 0 {
			cond += fmt.Sprintf(" (%d карт.)", v.Violations)
		}
		b.WriteString(mdRow([]string{q.ID, q.Text, v.Expect, ok, map[bool]string{true: "да", false: "нет"}[v.Top4], rank,
			first, cond, q.Route, fmt.Sprintf("%d / %d", v.Families, v.SKUs)}))
	}
	return b.String()
}

// FormatFeedReport — сводка «На данных заказчика» в Markdown (те же числа, что на экране S6).
func FormatFeedReport(sum *out.FeedSummary, header []string) string {
	var b strings.Builder
	b.WriteString("# Оценка на данных заказчика — сводка\n\n")
	for _, h := range header {
		b.WriteString(h + "\n")
	}
	if len(header) > 0 {
		b.WriteString("\n")
	}
	fmt.Fprintf(&b, "Каталог: %s, %s офферов.\n\n", sum.Catalog.Label, fmtThousands(sum.Catalog.Offers))
	b.WriteString("## Главное (экран S6)\n\n")
	if e := sum.Etalon; e != nil {
		fmt.Fprintf(&b, "- Эталоны заказчика, полный продукт: **%d из %d** (прогон …%s).\n", e.Matched, e.Total, e.RunTail)
	} else {
		b.WriteString("- Эталоны заказчика: прогон не запускался.\n")
	}
	if len(sum.Closed) == 0 {
		b.WriteString("- Закрытая часть: прогон не запускался.\n")
	}
	for _, c := range sum.Closed {
		fmt.Fprintf(&b, "- Закрытая часть, %s (%s): Success@1 %s.\n", targetCfgLabel(c.Config), c.Config,
			tgtShare(c.Success1, c.Matched, c.Total))
	}
	if f := sum.Four; f != nil {
		fmt.Fprintf(&b, "- Ответы полного продукта с ≥4 разными семействами: %s; с ≥4 SKU: %s (наборы: %s).\n",
			tgtShare(f.Families4, f.Families4N, f.Answers), tgtShare(f.SKUs4, f.SKUs4N, f.Answers), strings.Join(f.Sets, ", "))
	}
	if c := sum.Cost; c != nil {
		b.WriteString("\n## Стоимость запроса\n\n")
		fmt.Fprintf(&b, "`%s`, N = %s запросов в месяц.\n\n", c.Formula, fmtThousands(c.Requests))
		b.WriteString(mdHead("параметр", "значение", "источник", "пояснение"))
		for _, p := range c.Params {
			v := fmtThousands(int(p.Value))
			if p.Value != float64(int(p.Value)) {
				v = feedNum(p.Value, 2)
			}
			src := map[string]string{"measured": "замер", "assumed": "допущение"}[p.Source]
			b.WriteString(mdRow([]string{p.Label, v + " " + p.Unit, src, p.Hint}))
		}
		b.WriteString("\n" + mdHead("прогон", "поисков", "D по метке", "D без модели", "p", "h", "c_llm, ₽", "вызовов LLM",
			"токены вход / выход", "LLM, ₽/запрос", "постоянные, ₽/запрос", "C, ₽/запрос", "инфраструктура", "агент", "LLM",
			"p50 / p95, мс"))
		for _, r := range []*out.FeedCostRun{c.Cold, c.Warm, c.Journal} {
			if r == nil {
				continue
			}
			cl := "—"
			if r.CLLM != nil {
				cl = feedNum(*r.CLLM, 4)
			}
			pct := func(v float64, d int) string { return feedNum(v*100, d) + " %" }
			b.WriteString(mdRow([]string{map[string]string{TargetModeCold: "холодный прогон", TargetModeWarm: "тёплый прогон",
				targetModeJournal: "журнал витрины на фиде (для сравнения)"}[r.Mode],
				fmt.Sprint(r.Searches), pct(r.RouteDLabel, 1), fmt.Sprint(r.RouteDNoModel),
				pct(r.P, 1), pct(r.H, 1), cl, fmt.Sprint(r.LLMCalls),
				fmtThousands(int(r.InputTokens)) + " / " + fmtThousands(int(r.OutputTokens)), feedNum(r.LLMPerRequestRub, 4),
				feedNum(r.FixedPerRequestRub, 4), "**" + feedNum(r.PerRequestRub, 2) + "**",
				pct(r.InfraShare, 1), pct(r.AgentShare, 1), pct(r.LLMShare, 2), fmt.Sprintf("%.0f / %.0f", r.P50Ms, r.P95Ms)}))
		}
		b.WriteString("\n")
		for _, n := range c.Notes {
			b.WriteString("- " + n + "\n")
		}
	}
	b.WriteString("\n## Наборы и конфигурации\n\n")
	b.WriteString(mdHead("набор", "конфигурация", "режим", "Success@1", "в первых 4", "MRR@10", "все условия", "≥4 семейств",
		"≥4 SKU", "нарушения", "p50 / p95, мс", "₽/запрос", "прогон"))
	for _, r := range sum.Table {
		t := r.Metrics
		viol := "—"
		if t.HardQueries > 0 {
			viol = fmt.Sprintf("%d из %d", t.HardViolated, t.HardQueries)
		}
		rub := "—"
		if r.RubPerQuery != nil {
			rub = feedNum(*r.RubPerQuery, 4)
		}
		mode := map[string]string{TargetModeCold: "холодный", TargetModeWarm: "тёплый"}[r.Mode]
		b.WriteString(mdRow([]string{r.Set, fmt.Sprintf("%s (%s)", targetCfgLabel(r.Config), r.Config), mode,
			tgtShare(t.Success1, t.Success1N, t.Judged), tgtShare(t.Top4, t.Top4N, t.Judged), fmtRatio(t.MRR10),
			tgtShare(t.Pass, t.PassN, t.Answers), tgtShare(t.Families4, t.Families4N, t.Answers), tgtShare(t.SKUs4, t.SKUs4N, t.Answers), viol,
			fmt.Sprintf("%.0f / %.0f", r.P50Ms, r.P95Ms), rub, "…" + r.RunTail}))
	}
	return b.String()
}

// targetSetLabel — подпись набора.
func targetSetLabel(set string) string {
	switch set {
	case domain.TargetSetEtalon:
		return "эталоны заказчика"
	case domain.TargetSetTZ:
		return "примеры ТЗ"
	case domain.TargetSetNeighbors:
		return "соседи эталонов (настройка)"
	case domain.TargetSetClosed:
		return "закрытая часть"
	case domain.TargetSetUnderstanding:
		return "набор понимания (настройка)"
	}
	return set
}
