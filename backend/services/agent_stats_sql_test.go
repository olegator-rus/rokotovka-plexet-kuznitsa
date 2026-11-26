package services

import (
	"fmt"
	"reflect"
	"sort"
	"testing"
	"time"

	"kuznica/api/repositories"
)

// Равенство SQL-входа агента (repositories.AgentQueryStatsBetween) прежнему расчёту в памяти (computeQueryStats).
// Нужна живая база: KUZNICA_TEST_DATABASE_URL=postgres://… go test ./services -run SQL. Без переменной — пропуск.

// memOnlyStore — хранилище без SQL-агрегата: agentQueryStats идёт по ветке в памяти.
type memOnlyStore struct{ agentStore }

// comparableStats — исходы в виде для сравнения: без служебного множества сессий; товар успешной
// переформулировки при нескольких кликах в памяти зависит от порядка обхода map — сравнивается только при одном.
func comparableStats(st *queryStats) map[string]clusterQuery {
	res := map[string]clusterQuery{}
	for k, q := range st.byText {
		c := *q
		c.failSess = nil
		c.ReformulatedTo = append([]reformTarget(nil), q.ReformulatedTo...)
		for i := range c.ReformulatedTo {
			if c.ReformulatedTo[i].Clicked > 1 {
				c.ReformulatedTo[i].ClickedOn = ""
			}
		}
		if len(c.ReformulatedTo) == 0 {
			c.ReformulatedTo = nil
		}
		res[k] = c
	}
	return res
}

func checkStatsEqual(t *testing.T, got, want *queryStats) {
	t.Helper()
	g, w := comparableStats(got), comparableStats(want)
	if len(g) != len(w) {
		t.Fatalf("запросов: SQL %d, память %d", len(g), len(w))
	}
	keys := make([]string, 0, len(w))
	for k := range w {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		if !reflect.DeepEqual(g[k], w[k]) {
			t.Fatalf("запрос %q:\nSQL    %+v\nпамять %+v", k, g[k], w[k])
		}
	}
	gp, wp := got.clickedProduct, want.clickedProduct
	if len(gp) == 0 && len(wp) == 0 {
		return
	}
	if !reflect.DeepEqual(gp, wp) {
		t.Fatalf("товары кликов:\nSQL    %v\nпамять %v", gp, wp)
	}
}

// checkAgentStatsSQL — агрегат [since, until) равен computeQueryStats на тех же событиях.
func checkAgentStatsSQL(t *testing.T, r *repositories.Repos, since, until time.Time) int {
	t.Helper()
	ctx := t.Context()
	searches, err := r.SearchEventsSince(ctx, since)
	if err != nil {
		t.Fatal(err)
	}
	behavior, err := r.BehaviorEventsSince(ctx, since)
	if err != nil {
		t.Fatal(err)
	}
	ks := searches[:0:0]
	for _, e := range searches {
		if e.CreatedAt.Before(until) {
			ks = append(ks, e)
		}
	}
	kb := behavior[:0:0]
	for _, b := range behavior {
		if b.CreatedAt.Before(until) {
			kb = append(kb, b)
		}
	}
	want := computeQueryStats(ks, kb)
	agg, err := r.AgentQueryStatsBetween(ctx, since, until)
	if err != nil {
		t.Fatal(err)
	}
	checkStatsEqual(t, queryStatsFromSQL(agg), want)
	return len(ks)
}

func TestAgentStatsSQLEqualsMemorySynthetic(t *testing.T) {
	dsn := testDatabaseURL(t)
	t0 := time.Date(2026, 5, 3, 9, 0, 0, 0, time.UTC)
	for seed := uint64(1); seed <= 5; seed++ {
		r := openSchema(t, dsn)
		searches, behavior := syntheticEvents(seed, 600, t0)
		if err := r.ReplaceMockLog(t.Context(), searches, behavior); err != nil {
			t.Fatal(err)
		}
		for _, w := range []struct{ since, until time.Time }{
			{t0.Add(-time.Hour), time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)},
			{t0.Add(-time.Hour), t0.Add(2 * time.Hour)},
			{t0.Add(time.Hour), t0.Add(4 * time.Hour)},
		} {
			t.Run(fmt.Sprintf("seed%d/%s-%s", seed, w.since.Format("15:04"), w.until.Format("15:04")), func(t *testing.T) {
				checkAgentStatsSQL(t, r, w.since, w.until)
			})
		}
		// Целиком через agentQueryStats: граница окна, скрытые поиски, исключение test — как в ветке в памяти.
		s := NewServices(r, Options{Source: fixtureSource()})
		s.now = func() time.Time { return t0.Add(48 * time.Hour) }
		for _, h := range []HoldoutOptions{
			{},
			{Enabled: true, ExcludeTest: true, From: t0.Add(2 * time.Hour), Days: 1},
			{Enabled: true, ExcludeTest: true},
		} {
			gotSt, gotInfo, err := s.agentQueryStats(t.Context(), 30, h)
			if err != nil {
				t.Fatal(err)
			}
			s.agentStoreOverride = memOnlyStore{r}
			wantSt, wantInfo, err := s.agentQueryStats(t.Context(), 30, h)
			s.agentStoreOverride = nil
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(gotInfo, wantInfo) {
				t.Fatalf("seed %d, %+v: сведения о разрезе:\nSQL    %+v\nпамять %+v", seed, h, gotInfo, wantInfo)
			}
			checkStatsEqual(t, gotSt, wantSt)
		}
	}
}

// На событиях базы как они есть (dev: mock-журнал и живые поиски) — только чтение.
func TestAgentStatsSQLEqualsMemoryOnDatabase(t *testing.T) {
	r, err := repositories.InitRepos(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	now := time.Now()
	for _, days := range []int{30, 365} {
		since := now.Add(-time.Duration(days) * 24 * time.Hour)
		n := checkAgentStatsSQL(t, r, since, time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC))
		t.Logf("%d дней: поисков %d", days, n)
	}
	s := NewServices(r, Options{Source: fixtureSource()})
	s.now = func() time.Time { return now }
	h := HoldoutOptions{Enabled: true, ExcludeTest: true}
	gotSt, gotInfo, err := s.agentQueryStats(t.Context(), 30, h)
	if err != nil {
		t.Fatal(err)
	}
	s.agentStoreOverride = memOnlyStore{r}
	wantSt, wantInfo, err := s.agentQueryStats(t.Context(), 30, h)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotInfo, wantInfo) {
		t.Fatalf("сведения о разрезе:\nSQL    %+v\nпамять %+v", gotInfo, wantInfo)
	}
	checkStatsEqual(t, gotSt, wantSt)
	t.Logf("разрез %+v; запросов %d, неудачных %d", gotInfo, len(gotSt.byText), len(gotSt.failed()))
}
