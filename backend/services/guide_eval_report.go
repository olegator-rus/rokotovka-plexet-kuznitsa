package services

import (
	"fmt"
	"sort"
	"strings"

	"kuznica/api/out"
)

// FormatGuideReport — отчёт оценки подбора (data/softline/eval/results/REPORT.md). Дат нет.
func FormatGuideReport(res *GuideEvalResult, command string) string {
	var b strings.Builder
	full, srch, nock := res.Variants[GuideVariantFull], res.Variants[GuideVariantSearch], res.Variants[GuideVariantNoCheck]
	b.WriteString("# Оценка режима подбора\n\n")
	fmt.Fprintf(&b, "Каталог: %s · коммит %s · прогон …%s · диалогов %d (по вендорам %d, отрицательных %d) · прогонов %d.\n\n",
		res.Catalog, res.Commit, runTail(res.RunID), len(res.Dialogs), full.VendorOf, full.NegativeOf, full.Runs)
	if command != "" {
		fmt.Fprintf(&b, "Команда: `%s`\n\n", command)
	}
	gate := "**пройден**"
	if !full.Gate {
		gate = "**не пройден**"
	}
	fmt.Fprintf(&b, "## Гейт — %s\n\n", gate)
	b.WriteString(mdHead("условие", "полный подбор", "на чём проверено"))
	b.WriteString(mdRow([]string{"выдуманные идентификаторы офферов и фактов", fmt.Sprint(full.FabricatedIDs), checkOf(full, "ids_exist")}))
	b.WriteString(mdRow([]string{"выдуманные или неверные цены и суммы", fmt.Sprint(full.FabricatedPrices), checkOf(full, "price_catalog")}))
	b.WriteString(mdRow([]string{"нарушения правил лицензий у подходящих карточек", fmt.Sprint(full.LicenseViolation), checkOf(full, "license_rules")}))
	b.WriteString(mdRow([]string{"утверждения о свойстве без факта", fmt.Sprintf("%d из %d", full.ClaimsNoFact, full.Claims), checkOf(full, "claims_fact")}))
	b.WriteString("\n## Задачи\n\n")
	b.WriteString(mdHead("вариант", "решено 3 из 3", "по вендорам", "отрицательные", "решено по прогонам"))
	for _, m := range []*out.GuideEvalMetrics{srch, nock, full} {
		if m == nil {
			continue
		}
		all := fmt.Sprintf("%d из %d", m.TaskAll, m.Dialogs)
		if m.Runs == 1 {
			all = fmt.Sprintf("%d из %d (один прогон)", m.TaskAll, m.Dialogs)
		}
		b.WriteString(mdRow([]string{guideVariantLabel(m.Variant), all, fmt.Sprintf("%d из %d", m.VendorAll, m.VendorOf),
			fmt.Sprintf("%d из %d", m.NegativeAll, m.NegativeOf), fmt.Sprintf("%d из %d", m.TaskPasses, m.TaskTotal)}))
	}
	b.WriteString("\nЗадача решена — во всех прогонах выполнены ожидания каждого хода (тип ответа, вопрос, подходящее семейство, " +
		"блок «Не подходит», факты, «по запросу», изменения) и ни одна автопроверка не нарушена.\n\n")
	b.WriteString("## Автопроверки на ответе\n\n")
	b.WriteString(mdHead("проверка", "поиск без подбора", "без проверок (эмуляция)", "полный подбор"))
	keys := make([]string, 0, len(guideCheckLabels))
	for k := range guideCheckLabels {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		b.WriteString(mdRow([]string{guideCheckLabels[k], checkOf(srch, k), checkOf(nock, k), checkOf(full, k)}))
	}
	b.WriteString("\nЧисло — нарушений из проверенного (карточек, фактов, ходов, диалогов). У поиска проверены первые четыре карточки " +
		"и правила лицензий у первой карточки при требованиях, которые разобрал подбор; вопросов, фраз и утверждений у поиска нет.\n\n")
	if nock != nil && len(nock.Removed) > 0 {
		b.WriteString("Сняли проверки до показа (по классам, все прогоны): ")
		var parts []string
		for k, v := range nock.Removed {
			parts = append(parts, fmt.Sprintf("%s — %d", k, v))
		}
		sort.Strings(parts)
		b.WriteString(strings.Join(parts, ", ") + ".\n\n")
	}
	for _, m := range []*out.GuideEvalMetrics{nock} {
		if m != nil {
			for _, n := range m.Notes {
				b.WriteString("- " + n + "\n")
			}
		}
	}
	b.WriteString("\n## Время и расход полного подбора\n\n")
	b.WriteString(mdHead("момент", "p50, мс", "p95, мс"))
	b.WriteString(mdRow([]string{"первый вывод (офферы собраны)", fmtMs(full.MsFirst.P50), fmtMs(full.MsFirst.P95)}))
	b.WriteString(mdRow([]string{"проверенные строки", fmtMs(full.MsChecked.P50), fmtMs(full.MsChecked.P95)}))
	b.WriteString(mdRow([]string{"весь ход", fmtMs(full.MsTotal.P50), fmtMs(full.MsTotal.P95)}))
	b.WriteString(mdRow([]string{fmt.Sprintf("весь ход с вызовом модели (%d ходов)", full.LLMTurns), fmtMs(full.MsTotalLLM.P50), fmtMs(full.MsTotalLLM.P95)}))
	fmt.Fprintf(&b, "\nХодов %d; по журналу `guide_turns` — %d строк: вызовов модели %d, токенов на входе %s, на выходе %s, %s ₽. "+
		"По `llm_usage` (request_id хода): вызовов %d, %s ₽. Стоимость диалога в среднем — **%s ₽** (%d диалогов-прогонов); "+
		"диалога, где модель вызывалась (%d), — %s ₽.\n\n",
		full.Turns, full.JournalTurns, full.LLMCalls, fmtThousands(full.InputTokens), fmtThousands(full.OutputTokens),
		feedNum(full.TurnsCostRub, 4), full.UsageCalls, feedNum(full.UsageCostRub, 4), feedNum(full.DialogCostRub, 4),
		full.Dialogs*full.Runs, full.DialogsWithLLM, feedNum(full.DialogLLMRub, 4))
	b.WriteString("## Ошибки\n\n")
	for _, m := range []*out.GuideEvalMetrics{full, srch} {
		if m == nil || len(m.Errors) == 0 {
			continue
		}
		fmt.Fprintf(&b, "%s:\n\n", guideVariantLabel(m.Variant))
		for _, e := range m.Errors {
			fmt.Fprintf(&b, "- `%s`, прогон %d, ход %d — %s\n", e.Dialog, e.Run, e.Turn, e.Reason)
		}
		b.WriteString("\n")
	}
	b.WriteString("## Диалоги полного подбора\n\n")
	b.WriteString(mdHead("диалог", "тип", "прогоны", "типы ответов (прогон 1)", "вызовов модели", "причина"))
	for _, d := range res.Dialogs {
		runs := res.PerDialog[GuideVariantFull][d.ID]
		ok, llm := 0, 0
		reason := ""
		for _, r := range runs {
			if r.Pass {
				ok++
			} else if reason == "" && len(r.Fails) > 0 {
				reason = r.Fails[0]
			}
			llm += r.LLM
		}
		types := ""
		if len(runs) > 0 {
			types = strings.Join(runs[0].Types, " → ")
		}
		b.WriteString(mdRow([]string{d.ID, d.Type, fmt.Sprintf("%d из %d", ok, len(runs)), types, fmt.Sprint(llm), reason}))
	}
	return b.String()
}

func checkOf(m *out.GuideEvalMetrics, k string) string {
	if m == nil {
		return "—"
	}
	c := m.Checks[k]
	if c.Of == 0 {
		return "—"
	}
	return fmt.Sprintf("%d из %d", c.Bad, c.Of)
}

func guideVariantLabel(v string) string {
	switch v {
	case GuideVariantFull:
		return "полный подбор"
	case GuideVariantSearch:
		return "поиск без подбора"
	case GuideVariantNoCheck:
		return "модель с поиском без проверок (эмуляция)"
	}
	return v
}
