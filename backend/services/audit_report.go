package services

import (
	"fmt"
	"sort"
	"strings"

	"kuznica/api/externalApi/catalogsource"
)

// auditStatusLabels — статус поля по-русски.
var auditStatusLabels = map[string]string{
	catalogsource.FieldFromSource: "из данных",
	catalogsource.FieldRestored:   "восстановлено правилами",
	catalogsource.FieldPartial:    "частично",
	catalogsource.FieldSurrogate:  "суррогат",
	catalogsource.FieldAbsent:     "нет",
}

// mdCell — текст для ячейки таблицы Markdown.
func mdCell(s string) string {
	return strings.NewReplacer("|", "\\|", "\n", " ").Replace(s)
}

// FormatAuditMarkdown — data/AUDIT.md.
func FormatAuditMarkdown(r *AuditReport) string {
	var b strings.Builder
	w := func(format string, a ...any) { fmt.Fprintf(&b, format, a...) }
	w("# Аудит данных\n\n")
	w("Источник: `%s`, версия каталога `%s`, %s UTC, %d мс.\n\n", r.Source, r.CatalogVersion,
		r.GeneratedAt.Format("2006-01-02 15:04"), r.DurationMs)

	w("## Что меняется в плане (ARCHITECTURE §9)\n\n| Вопрос | Что в данных | Что меняем |\n|---|---|---|\n")
	for _, p := range r.Plan {
		w("| %s | %s | %s |\n", mdCell(p.Item), mdCell(p.Finding), mdCell(p.Change))
	}

	if p := r.P2; p != nil {
		w("\n## Контур коммерции: что есть в данных\n\n| Что | Есть | Откуда | Примечание |\n|---|---|---|---|\n")
		for _, it := range p.Items {
			w("| %s | %s | %s | %s |\n", mdCell(it.What), mdCell(it.Have), mdCell(it.Origin), mdCell(orDash(it.Note)))
		}
		w("\n## Правила среза контура коммерции\n\n| Направление | Что в данных | Что следует |\n|---|---|---|\n")
		for _, x := range p.Slice {
			w("| %s | %s | %s |\n", mdCell(x.Item), mdCell(x.Finding), mdCell(x.Change))
		}
	}

	v := r.Volumes
	w("\n## Объёмы\n\n| Что | Сколько |\n|---|---|\n")
	w("| Позиции | %d |\n| Семейства | %d |\n| Вендоры | %d |\n| Категории | %d |\n", v.Products, v.Families, v.Vendors, v.Categories)
	rels := make([]string, 0, len(v.Relations))
	for t, n := range v.Relations {
		rels = append(rels, fmt.Sprintf("%s %d", t, n))
	}
	sort.Strings(rels)
	w("| Связи | %s |\n| Алиасы | %d |\n", orDash(strings.Join(rels, ", ")), v.Aliases)
	w("| Клиенты (с покупками) | %d (%d), покупок %d |\n", v.Clients, v.ClientsWithPurchases, v.Purchases)
	evs := make([]string, 0, len(v.Events))
	for t, n := range v.Events {
		evs = append(evs, fmt.Sprintf("%s %d", t, n))
	}
	sort.Strings(evs)
	w("| События | %s |\n", orDash(strings.Join(evs, ", ")))
	if v.EventsFrom != nil {
		w("| Период событий | %s — %s, сессий %d |\n", v.EventsFrom.Format("02.01.2006"), v.EventsTo.Format("02.01.2006"), v.Sessions)
	}
	w("| Журнал запросов | %d строк |\n| Уникальных запросов | %d, суммарная частота %d |\n| Размеченных запросов (eval) | %d |\n",
		v.QueryLogRows, v.DistinctQueries, v.QueryVolume, v.EvalQueries)

	w("\n## Покрытие полей каталога\n\n| Поле | Заполнено | Из данных | Правилами | Статус | Примечание |\n|---|---|---|---|---|---|\n")
	for _, f := range r.Fields {
		w("| %s | %s (%d) | %d | %d | %s | %s |\n", f.Label, auditPct(f.Coverage), f.Filled, f.FromSource, f.Restored,
			auditStatusLabels[f.Status], mdCell(f.Note))
	}

	st := r.Structure
	w("\n## Структура\n\n")
	w("- Каталог: %s; в среднем %.1f позиций на семейство, одиночных семейств %d.\n", map[bool]string{true: "плоский (семейства собраны правилами)", false: "с семействами"}[st.Flat],
		st.AvgVariants, st.SingleVariantFamilies)
	w("- Маржа: %s. Цена у %s позиций, «по запросу» у %s.\n", auditStatusLabels[st.Margin], auditPct(st.PriceShare), auditPct(st.OnRequestShare))
	if st.HasTime {
		w("- Сессии: %s; позиция показа у %s кликов; клиент у %s событий; число результатов у %s поисков журнала.\n",
			map[bool]string{true: "из данных", false: fmt.Sprintf("синтезированы у %s событий", auditPct(st.SyntheticSessionShare))}[st.SessionsReal],
			auditPct(st.ClickPositionShare), auditPct(st.EventClientShare), auditPct(st.SearchResultsKnown))
	} else {
		w("- Журнала событий нет: сессий, времени и позиций показов нет.\n")
	}
	lic := make([]string, 0, len(st.License))
	for _, f := range r.Fields {
		if s, ok := st.License[f.Field]; ok {
			lic = append(lic, f.Label+" — "+auditStatusLabels[s])
		}
	}
	w("- Атрибуты лицензий: %s.\n", strings.Join(lic, "; "))

	if s := r.Search; s != nil {
		w("\n## Поиск по частым запросам\n\n")
		if s.Skipped != "" {
			w("Не прогонялся: %s.\n", s.Skipped)
		} else {
			w("%d самых частых запросов (%s суммарной частоты), без журнала и без LLM, %d мс.", s.Queries, auditPct(s.VolumeShare), s.DurationMs)
			if s.LogZeroShare != nil {
				w(" По журналу магазина без результата — %s частоты.", auditPct(*s.LogZeroShare))
			}
			w("\n\n| Конфигурация | Без результата | По частоте | Точного нет | Ошибок | Среднее, мс |\n|---|---|---|---|---|---|\n")
			for _, c := range s.Configs {
				w("| %s | %d (%s) | %s | %d (%s) | %d | %.1f |\n", c.Config, c.Zero, auditPct(c.ZeroShare), auditPct(c.ZeroWeighted),
					c.NoExact, auditPct(c.NoExactShare), c.Errors, c.AvgMs)
			}
			routes := make([]string, 0, len(s.Routes))
			for k, n := range s.Routes {
				routes = append(routes, fmt.Sprintf("%s %d", k, n))
			}
			sort.Strings(routes)
			w("\nМаршруты B3: %s.\n", orDash(strings.Join(routes, ", ")))
			w("\n### Топ неудачных запросов\n\n| Запрос | Частота | B0 | B3 | Точного нет | Пусто в журнале |\n|---|---|---|---|---|---|\n")
			for _, f := range s.Failed {
				w("| %s | %d | %d | %d | %s | %s |\n", mdCell(f.Text), f.Freq, f.B0, f.B3, yesNo(f.NoExact), yesNo(f.LogZero))
			}
			if len(s.Failed) == 0 {
				w("| — | | | | | |\n")
			}
		}
	}

	if len(r.Files) > 0 {
		w("\n## Файлы\n\n| Таблица | Файл | Формат | Строк | Пропущено | Не найдены колонки | Не использованы |\n|---|---|---|---|---|---|---|\n")
		for _, f := range r.Files {
			skipped := fmt.Sprint(f.Skipped)
			if len(f.Notes) > 0 {
				skipped += " (" + strings.Join(f.Notes, "; ") + ")"
			}
			w("| %s | %s | %s | %d | %s | %s | %s |\n", f.Table, mdCell(f.File), mdCell(f.Detected), f.Rows, mdCell(skipped),
				mdCell(orDash(strings.Join(f.Missing, ", "))), mdCell(orDash(strings.Join(f.Unused, ", "))))
		}
	}
	if len(r.Warnings) > 0 {
		w("\n## Предупреждения источника\n\n")
		for _, x := range r.Warnings {
			w("- %s\n", x)
		}
	}
	return b.String()
}

func orDash(s string) string {
	if s == "" {
		return "—"
	}
	return s
}

func yesNo(b bool) string {
	if b {
		return "да"
	}
	return "нет"
}
