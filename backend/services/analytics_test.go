package services

import (
	"testing"
	"time"

	"kuznica/api/domain"
)

func TestLogRowsAndAnalytics(t *testing.T) {
	t0 := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	at := func(sec int) time.Time { return t0.Add(time.Duration(sec) * time.Second) }
	n := func(v int) *int { return &v }
	evs := []domain.LogEvent{
		// s1: набор «вин» → «винда» до Enter (отбрасывается), пусто, переформулировка, клик, корзина, заказ.
		{TS: at(0), SessionID: "s1", Type: "search", Query: "вин", ResultsCount: n(3)},
		{TS: at(2), SessionID: "s1", Type: "search", Query: "Винда", ResultsCount: n(0)},
		{TS: at(30), SessionID: "s1", Type: "search", Query: "windows 11", ResultsCount: n(5)},
		{TS: at(40), SessionID: "s1", Type: "click", ProductID: "w11", Position: n(1)},
		{TS: at(50), SessionID: "s1", Type: "add_to_cart", ProductID: "w11"},
		{TS: at(60), SessionID: "s1", Type: "order", ProductID: "w11"},
		// s2: поиск без клика; s3: тот же запрос, пусто; клик через 2 часа к поиску не относится.
		{TS: at(100), SessionID: "s2", Type: "search", Query: "винда", ResultsCount: n(2)},
		{TS: at(200), SessionID: "s3", Type: "search", Query: "ВИНДА ", ResultsCount: n(0)},
		{TS: at(200 + 7200), SessionID: "s3", Type: "click", ProductID: "x"},
		{TS: at(300), SessionID: "s4", Type: "unknown"},
	}
	searches, behavior, skipped := logRows(evs)
	if len(searches) != 5 || len(behavior) != 4 || skipped != 1 {
		t.Fatalf("строки: %d %d %d", len(searches), len(behavior), skipped)
	}
	if searches[0].Source != domain.EventSourceMockLog || !searches[0].CreatedAt.Equal(at(0)) || searches[1].Normalized != "винда" ||
		behavior[0].RequestID == nil || *behavior[0].RequestID != searches[2].RequestID || behavior[3].RequestID != nil {
		t.Fatalf("поля: %+v / %+v", searches[1], behavior[0])
	}

	res := computeAnalytics(searches, behavior)
	tl := res.Tiles
	// Поисков 4 (набор «вин» не считается): пустых 2, непустых 2, из них без клика 1; заказ 1.
	if tl.Searches != 4 || tl.Empty != 2 || tl.NoClick != 1 || tl.Orders != 1 || *tl.EmptyRate != 0.5 ||
		*tl.NoClickRate != 0.5 || *tl.OrderRate != 0.25 || tl.PendingProposals != 0 {
		t.Fatalf("плитки: %+v", tl)
	}
	if res.Funnel[1].Count != 1 || res.Funnel[2].Count != 1 || res.Funnel[3].Count != 1 || *res.Funnel[0].Rate != 1 {
		t.Fatalf("воронка: %+v", res.Funnel)
	}
	if res.Sources[domain.EventSourceMockLog] != 4 {
		t.Fatalf("источники: %+v", res.Sources)
	}
	// «винда»: 3 поиска, 2 пустых, 1 без клика → исход «пустая выдача»; «windows 11» удачен и в таблицу не попал.
	if len(res.Problems) != 1 {
		t.Fatalf("проблемные: %+v", res.Problems)
	}
	p := res.Problems[0]
	if p.Query != "винда" || p.Freq != 3 || p.Empty != 2 || p.NoClick != 1 || p.Failures != 3 || p.Outcome != "empty" ||
		p.Diagnosis != diagnosisUnparsed {
		t.Fatalf("проблемный: %+v", p)
	}
}

func TestAnalyticsReformulation(t *testing.T) {
	t0 := time.Date(2026, 5, 12, 12, 0, 0, 0, time.UTC)
	s := "s"
	mk := func(id, q string, sec, n int) domain.SearchEvent {
		e := domain.SearchEvent{RequestID: id, Query: q, ResultsCount: n, SessionID: &s}
		e.CreatedAt = t0.Add(time.Duration(sec) * time.Second)
		return e
	}
	res := computeAnalytics([]domain.SearchEvent{mk("a", "офис", 0, 4), mk("b", "мойофис", 30, 2)}, nil)
	// Равные по неудачам и частоте — по алфавиту.
	if len(res.Problems) != 2 || res.Problems[1].Query != "офис" || res.Problems[1].Outcome != "reformulated" ||
		res.Problems[0].Outcome != "no_click" {
		t.Fatalf("переформулировка: %+v", res.Problems)
	}
	empty := computeAnalytics(nil, nil)
	if empty.Tiles.Searches != 0 || empty.Tiles.EmptyRate != nil || len(empty.Funnel) != 4 || empty.Problems == nil {
		t.Fatalf("пусто: %+v", empty)
	}
}

// Без базы аналитика отвечает нулями; неверные параметры — 422.
func TestAnalyticsWithoutDB(t *testing.T) {
	env := newTestEnv(t, false)
	res, err := env.svc.Analytics(t.Context(), 0)
	if err != nil || res.Days != 30 || res.Tiles.Searches != 0 {
		t.Fatalf("без базы: %v %+v", err, res)
	}
	if _, err := env.svc.Analytics(t.Context(), 400); err == nil {
		t.Fatal("days > 365 — ошибка")
	}
	if _, err := env.svc.ImportSearchLogs(t.Context()); err == nil {
		t.Fatal("импорт логов без базы — ошибка")
	}
}
