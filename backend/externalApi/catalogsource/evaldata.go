package catalogsource

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"kuznica/api/domain"
)

// Наборы запросов с разметкой и исторические логи поведения рядом с каталогом:
// queries.json и events.jsonl. Нет файла — ErrNoFile: оценка и аналитика работают без них, пусто.

// ErrNoFile — файла данных нет в каталоге источника.
var ErrNoFile = errors.New("файла данных нет")

// maxLogLine — самая длинная строка events.jsonl.
const maxLogLine = 64 << 10

// LoadQueries — все запросы queries.json. Пустой id, текст или неизвестный набор — ошибка: оценка на
// битой разметке хуже, чем её отсутствие.
func (m MockJSON) LoadQueries(_ context.Context) ([]domain.EvalQuery, error) {
	var qs []domain.EvalQuery
	if err := readJSON(filepath.Join(m.Dir, "queries.json"), &qs); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoFile
		}
		return nil, err
	}
	seen := make(map[string]bool, len(qs))
	for i, q := range qs {
		switch {
		case strings.TrimSpace(q.ID) == "":
			return nil, fmt.Errorf("queries.json: запрос №%d без id", i+1)
		case seen[q.ID]:
			return nil, fmt.Errorf("queries.json: id %q повторяется", q.ID)
		case strings.TrimSpace(q.Text) == "":
			return nil, fmt.Errorf("queries.json: %q без text", q.ID)
		}
		valid := false
		for _, s := range domain.EvalSets {
			valid = valid || s == q.Set
		}
		if !valid {
			return nil, fmt.Errorf("queries.json: %q — неизвестный набор %q", q.ID, q.Set)
		}
		seen[q.ID] = true
		if qs[i].Judgments == nil {
			qs[i].Judgments = map[string]int{}
		}
	}
	return qs, nil
}

// LoadEvents — события events.jsonl по порядку файла. Строка без ts, типа или сессии — ошибка с
// номером строки.
func (m MockJSON) LoadEvents(ctx context.Context) ([]domain.LogEvent, error) {
	f, err := os.Open(filepath.Join(m.Dir, "events.jsonl"))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoFile
		}
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), maxLogLine)
	var evs []domain.LogEvent
	for n := 1; sc.Scan(); n++ {
		if n%1000 == 0 {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
		}
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var ev domain.LogEvent
		if err := json.Unmarshal([]byte(line), &ev); err != nil {
			return nil, fmt.Errorf("events.jsonl:%d: %w", n, err)
		}
		if ev.TS.IsZero() || ev.Type == "" || ev.SessionID == "" {
			return nil, fmt.Errorf("events.jsonl:%d: нет ts, type или session_id", n)
		}
		evs = append(evs, ev)
	}
	if err := sc.Err(); err != nil {
		return nil, fmt.Errorf("events.jsonl: %w", err)
	}
	return evs, nil
}
