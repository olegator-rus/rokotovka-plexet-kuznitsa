package services

import (
	"fmt"
	"math/rand/v2"
	"net/url"
	"os"
	"reflect"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/repositories"

	"gorm.io/driver/postgres"
	"gorm.io/gorm"
)

// Равенство SQL-агрегации аналитики (repositories/analytics_sql.go) прежнему расчёту в памяти (buildOutcomes).
// Нужна живая база: KUZNICA_TEST_DATABASE_URL=postgres://… go test ./services -run SQL. Без переменной — пропуск
// (make test работает без живых сервисов).
//
// Синтетика пишется в отдельную схему (удаляется после теста); второй тест только читает события базы как есть
// (mock-журнал dev-базы) и сверяет оба расчёта на них.

func testDatabaseURL(t *testing.T) string {
	t.Helper()
	dsn := os.Getenv("KUZNICA_TEST_DATABASE_URL")
	if dsn == "" {
		t.Skip("KUZNICA_TEST_DATABASE_URL не задан — сверка SQL с расчётом в памяти пропущена")
	}
	return dsn
}

// openSchema — репозитории в новой схеме: search_path в адресе подключения, миграция, удаление после теста.
func openSchema(t *testing.T, dsn string) *repositories.Repos {
	t.Helper()
	base, err := gorm.Open(postgres.Open(dsn), &gorm.Config{})
	if err != nil {
		t.Fatal(err)
	}
	schema := fmt.Sprintf("itest_analytics_%d", time.Now().UnixNano())
	t.Logf("схема %s", schema)
	if err := base.Exec("CREATE SCHEMA " + schema).Error; err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_ = base.Exec("DROP SCHEMA " + schema + " CASCADE").Error
		if db, err := base.DB(); err == nil {
			_ = db.Close()
		}
	})
	u, err := url.Parse(dsn)
	if err != nil {
		t.Fatal(err)
	}
	q := u.Query()
	q.Set("search_path", schema)
	u.RawQuery = q.Encode()
	r, err := repositories.InitRepos(u.String())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	if err := r.MigrateDB(); err != nil {
		t.Fatal(err)
	}
	return r
}

// syntheticEvents — сессии с набором до Enter, переформулировками, действиями по request_id и по сессии, в окне
// и за окном, поисками без сессии и прогонами оценки. Времена поисков различны: при равных временах порядок
// в памяти зависит от порядка строк из базы.
func syntheticEvents(seed uint64, n int, t0 time.Time) ([]domain.SearchEvent, []domain.BehaviorEvent) {
	rnd := rand.New(rand.NewPCG(seed, seed^0x9e37))
	words := []string{"вин", "винда", "Windows 11", "windows", "1с", "1С ЗУП", "зуп", "ёлка", "Елка  офис", "касп",
		"касперский", "мойофис", "", "kis"}
	sessions := []string{"s1", "s2", "s3", "s4", "s5", ""}
	sources := []string{domain.EventSourceLive, domain.EventSourceMockLog, domain.EventSourceCompare, ""}
	var searches []domain.SearchEvent
	var behavior []domain.BehaviorEvent
	sec := 0
	for i := range n {
		sec += 1 + rnd.IntN(40) // шаг 1–40 с: окна 10 с и 60 с срабатывают и не срабатывают
		if rnd.IntN(25) == 0 {
			sec += 3600 // за окном атрибуции
		}
		e := domain.SearchEvent{RequestID: fmt.Sprintf("r%05d", i), Query: words[rnd.IntN(len(words))],
			ResultsCount: rnd.IntN(4) * rnd.IntN(3), Source: sources[rnd.IntN(len(sources))]}
		if rnd.IntN(30) == 0 {
			e.RequestID = fmt.Sprintf("eval-%05d", i)
		}
		if s := sessions[rnd.IntN(len(sessions))]; s != "" || rnd.IntN(2) == 0 {
			e.SessionID = &s
		}
		e.CreatedAt = t0.Add(time.Duration(sec) * time.Second)
		searches = append(searches, e)
		for range rnd.IntN(3) {
			b := domain.BehaviorEvent{Type: []string{domain.EventClick, domain.EventAddToCart, domain.EventOrder,
				"price_request"}[rnd.IntN(4)], Source: domain.EventSourceLive}
			switch rnd.IntN(4) {
			case 0: // по request_id своего поиска
				rid := e.RequestID
				b.RequestID = &rid
			case 1: // по request_id чужого (возможно, отброшенного) поиска
				rid := fmt.Sprintf("r%05d", rnd.IntN(i+1))
				b.RequestID = &rid
			}
			if rnd.IntN(4) != 0 {
				s := sessions[rnd.IntN(len(sessions))]
				b.SessionID = &s
			}
			if rnd.IntN(3) == 0 {
				pid := fmt.Sprintf("p%d", rnd.IntN(5))
				b.ProductID = &pid
			}
			b.CreatedAt = e.CreatedAt.Add(time.Duration(rnd.IntN(2400)) * time.Second)
			behavior = append(behavior, b)
		}
	}
	return searches, behavior
}

// checkSQLEqualsMemory — суммы, проблемные запросы и ответ аналитики из SQL равны расчёту в памяти.
func checkSQLEqualsMemory(t *testing.T, r *repositories.Repos, since time.Time) {
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
	const limit = 100000
	wantSum, wantProblems := aggregateOutcomes(searches, behavior, limit)
	gotSum, gotProblems, err := r.SearchOutcomesSince(ctx, since, limit)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotSum, wantSum) {
		t.Fatalf("суммы:\nSQL    %+v\nпамять %+v", gotSum, wantSum)
	}
	if len(wantProblems) == 0 && len(gotProblems) == 0 {
		return
	}
	if !reflect.DeepEqual(gotProblems, wantProblems) {
		for i := range min(len(gotProblems), len(wantProblems)) {
			if gotProblems[i] != wantProblems[i] {
				t.Fatalf("проблемный запрос %d из %d/%d:\nSQL    %+v\nпамять %+v", i, len(gotProblems), len(wantProblems),
					gotProblems[i], wantProblems[i])
			}
		}
		t.Fatalf("проблемных запросов: SQL %d, память %d", len(gotProblems), len(wantProblems))
	}
	_, gotTop, err := r.SearchOutcomesSince(ctx, since, problemsLimit)
	if err != nil {
		t.Fatal(err)
	}
	want := computeAnalytics(searches, behavior)
	if got := analyticsFrom(gotSum, gotTop); !reflect.DeepEqual(got, want) {
		t.Fatalf("ответ аналитики:\nSQL    %+v\nпамять %+v", got, want)
	}
	// Вилка эффекта: замер по журналам из сумм SQL равен замеру в памяти (без цен).
	m := measureLogs(searches, behavior, nil)
	if got := effectLogsFrom(gotSum.Total); !reflect.DeepEqual(got, m) {
		t.Fatalf("замер эффекта: SQL %+v, память %+v", got, m)
	}
	t.Logf("поисков %d, действий %d, проблемных запросов %d", len(searches), len(behavior), len(gotProblems))
}

func TestAnalyticsSQLEqualsMemorySynthetic(t *testing.T) {
	dsn := testDatabaseURL(t)
	t0 := time.Date(2026, 5, 3, 9, 0, 0, 0, time.UTC)
	for seed := uint64(1); seed <= 5; seed++ {
		r := openSchema(t, dsn) // схема на каждый набор: ReplaceMockLog стирает только mock_log
		searches, behavior := syntheticEvents(seed, 600, t0)
		if err := r.ReplaceMockLog(t.Context(), searches, behavior); err != nil {
			t.Fatal(err)
		}
		for _, since := range []time.Time{t0.Add(-time.Hour), t0.Add(3 * time.Hour)} {
			t.Run(fmt.Sprintf("seed%d/%s", seed, since.Format("15:04")), func(t *testing.T) { checkSQLEqualsMemory(t, r, since) })
		}
	}
}

// На событиях базы как они есть (dev: mock-журнал и живые поиски) — только чтение.
func TestAnalyticsSQLEqualsMemoryOnDatabase(t *testing.T) {
	r, err := repositories.InitRepos(testDatabaseURL(t))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = r.Close() })
	for _, days := range []int{30, 365} {
		checkSQLEqualsMemory(t, r, time.Now().Add(-time.Duration(days)*24*time.Hour))
	}
}
