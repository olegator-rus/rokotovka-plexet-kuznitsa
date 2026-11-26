package services

import (
	"context"
	"fmt"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Набор витрины: 31 запрос прогона store.softline.ru, перенесённый на mock-каталог,
// с ожидаемым классом ответа — товар, аналоги, честное «нет» или справка. `make eval SET=showcase`: доля верных
// классов на витрине (live: B3 на действующих правилах), журнал не пишется.

// Классы ответа.
const (
	ShowcaseProduct = "product"
	ShowcaseAnalogs = "analogs"
	ShowcaseAbsent  = "absent"
	ShowcaseHelp    = "help"
	ShowcaseEmpty   = "empty"
)

// MsgShowcaseNoSet — набора витрины нет в источнике.
const MsgShowcaseNoSet = "Набора витрины нет: data/mock/showcase.json (python3 scripts/data/gen_mock.py)"

// ShowcaseRow — один запрос прогона.
type ShowcaseRow struct {
	ID      string   `json:"id"`
	Query   string   `json:"query"`
	Vitrine string   `json:"vitrine"`
	Expect  string   `json:"expect"`
	Got     string   `json:"got"`
	OK      bool     `json:"ok"`
	Route   string   `json:"route"`
	Top     []string `json:"top"`
	Note    string   `json:"note,omitempty"`
	Error   string   `json:"error,omitempty"`
	Ms      float64  `json:"ms"`
}

// ShowcaseResult — прогон набора витрины.
type ShowcaseResult struct {
	RunID      string            `json:"run_id"`
	Config     string            `json:"config"`
	StartedAt  time.Time         `json:"started_at"`
	Queries    int               `json:"queries"`
	Correct    int               `json:"correct"`
	Share      float64           `json:"share"`
	ByClass    map[string][2]int `json:"by_class"`   // класс → [верно, всего]
	VitrineOK  int               `json:"vitrine_ok"` // верных у витрины (оценка прогона)
	Rows       []ShowcaseRow     `json:"rows"`
	DurationMs int64             `json:"duration_ms"`
}

type showcaseLoader interface {
	LoadShowcase(ctx context.Context) ([]domain.ShowcaseQuery, error)
}

// ShowcaseClass — класс ответа поиска: справка без товаров, честное «нет» (с аналогами или без), только аналоги,
// товар; пустая выдача без сообщения — empty (ошибка класса в любом наборе).
func ShowcaseClass(r *out.SearchResponse) string {
	switch {
	case r.Help != nil && r.Help.Mode == HelpModeAnswer && len(r.Items) == 0:
		return ShowcaseHelp
	case r.Empty != nil:
		return ShowcaseAbsent
	case len(r.Items) == 0:
		return ShowcaseEmpty
	}
	for _, it := range r.Items {
		if it.Group != domain.GroupAnalog {
			return ShowcaseProduct
		}
	}
	return ShowcaseAnalogs
}

// RunShowcase — прогон набора витрины на конфигурации витрины (live).
func (s *Services) RunShowcase(ctx context.Context) (*ShowcaseResult, error) {
	l, ok := s.source.(showcaseLoader)
	if !ok {
		return nil, errs.NewUnavailableError(MsgShowcaseNoSet)
	}
	qs, err := l.LoadShowcase(ctx)
	if err != nil {
		return nil, errs.NewUnavailableError(MsgShowcaseNoSet).WithCause(err)
	}
	started := time.Now()
	run := &ShowcaseResult{RunID: newEvalRunID(started), Config: domain.ConfigLive, StartedAt: started.UTC(),
		Queries: len(qs), ByClass: map[string][2]int{}, Rows: make([]ShowcaseRow, 0, len(qs))}
	for _, q := range qs {
		row := ShowcaseRow{ID: q.ID, Query: q.Query, Vitrine: q.Vitrine, Expect: q.Expect, Note: q.Note, Top: []string{}}
		t := time.Now()
		resp, err := s.SearchWith(ctx, evalRequestID(run.RunID, "SC", q.ID), in.SearchRequest{Query: q.Query,
			Config: domain.ConfigLive, Limit: 10, SessionID: evalRequestPrefix + run.RunID}, SearchOptions{NoLog: true})
		row.Ms = float64(time.Since(t).Microseconds()) / 1000
		if err != nil {
			row.Error, row.Got = err.Error(), "error"
		} else {
			row.Got = ShowcaseClass(resp)
			if resp.Trace != nil {
				row.Route = resp.Trace.Route
			}
			for i, it := range resp.Items {
				if i == 3 {
					break
				}
				row.Top = append(row.Top, it.FamilyID)
			}
		}
		row.OK = row.Got == q.Expect
		c := run.ByClass[q.Expect]
		c[1]++
		if row.OK {
			c[0]++
			run.Correct++
		}
		run.ByClass[q.Expect] = c
		if q.Vitrine == "ok" {
			run.VitrineOK++
		}
		run.Rows = append(run.Rows, row)
	}
	if run.Queries > 0 {
		run.Share = float64(run.Correct) / float64(run.Queries)
	}
	run.DurationMs = time.Since(started).Milliseconds()
	return run, nil
}

// FormatShowcaseTable — таблица прогона для stdout.
func FormatShowcaseTable(r *ShowcaseResult) string {
	var b strings.Builder
	fmt.Fprintf(&b, "Прогон %s · набор showcase (%d запр., прогон витрины на mock-каталоге) · %s\n\n",
		r.RunID, r.Queries, r.Config)
	b.WriteString("| # | Запрос | Витрина | Ждём | Получили | Верно | Маршрут | Первые семейства |\n| --- | --- | --- | --- | --- | --- | --- | --- |\n")
	for _, row := range r.Rows {
		mark := "да"
		if !row.OK {
			mark = "НЕТ"
		}
		fmt.Fprintf(&b, "| %s | %s | %s | %s | %s | %s | %s | %s |\n", row.ID, row.Query, row.Vitrine, row.Expect,
			row.Got, mark, row.Route, strings.Join(row.Top, ", "))
	}
	fmt.Fprintf(&b, "\nВерный класс: %d из %d (%.1f%%); у витрины «верно» в таблице прогона — %d из %d.\n", r.Correct,
		r.Queries, r.Share*100, r.VitrineOK, r.Queries)
	for _, cls := range []string{ShowcaseProduct, ShowcaseAnalogs, ShowcaseAbsent, ShowcaseHelp} {
		if c, ok := r.ByClass[cls]; ok {
			fmt.Fprintf(&b, "  %s: %d из %d\n", cls, c[0], c[1])
		}
	}
	return b.String()
}
