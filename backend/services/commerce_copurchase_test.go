package services

import (
	"math"
	"os"
	"slices"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
)

// Точный тест и поправка — на числах, посчитанных независимо (scipy.stats.fisher_exact, alternative="greater").
func TestFisherAndBH(t *testing.T) {
	cases := []struct {
		k, a, b, n int
		want       float64
	}{
		{9, 13, 11, 179, 8.89e-11}, // PLANTED P2-03 по заказам
		{6, 9, 8, 41, 4.48e-4},     // та же пара по аккаунтам видимого периода
		{0, 5, 5, 10, 1},
	}
	for _, c := range cases {
		got := fisherGreater(c.k, c.a, c.b, c.n)
		if math.Abs(got-c.want)/c.want > 0.01 {
			t.Fatalf("fisher(%d,%d,%d,%d) = %g, want %g", c.k, c.a, c.b, c.n, got, c.want)
		}
	}
	q := bhAdjust([]float64{0.01, 0.04, 0.03, 0.005})
	want := []float64{0.02, 0.04, 0.04, 0.02}
	for i := range q {
		if math.Abs(q[i]-want[i]) > 1e-12 {
			t.Fatalf("bh: %v", q)
		}
	}
}

// mockCopurchase — каталог и заказы data/mock; нет файлов контура коммерции — пропуск.
func mockCopurchase(t *testing.T) (*commerceCatalog, []relationDoc, *copurchaseScan) {
	t.Helper()
	dir := "../../data/mock"
	if _, err := os.Stat(dir + "/license_rules.json"); err != nil {
		t.Skip("нет mock-данных контура коммерции")
	}
	src := catalogsource.MockJSON{Dir: dir}
	snap, err := src.Load(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	evs, err := src.LoadEvents(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	c := newCommerceCatalog(snap)
	docs := docsOfRelations(snap.Relations)
	bs, _ := ordersOf(evs, c)
	if len(bs) == 0 {
		t.Skip("в events.jsonl нет заказов с аккаунтами")
	}
	s := &Services{now: time.Now}
	from, to := s.orderWindow(bs)
	return c, docs, scanCopurchase(c, docs, bs, from, to)
}

// P2-03 (R750 → ИБП) — кандидат и проходит структуру, совместимость, лицензии, маржу и повтор; шум PLANTED.md —
// не кандидат или отклонён структурой.
func TestCopurchaseFindsPlanted(t *testing.T) {
	c, docs, sc := mockCopurchase(t)
	t.Logf("заказов %d, аккаунтов %d, поздний: %d/%d, тестов %d, значимых %d, шум %v", sc.Orders, sc.Accounts,
		sc.LateOrders, sc.LateAccounts, sc.Tests, sc.Significant, sc.NoiseCounts)
	var p203 *copurchaseCandidate
	for i, cand := range sc.Candidates {
		ok, why := sc.structureCheck(c, docs, cand.Anchor, cand.Companion)
		t.Logf("кандидат %s p=%s q=%s приоритет %.0f структура %v %s", cand.Key, fmtP(cand.Stat.P), fmtP(*cand.Stat.Q),
			cand.Priority, ok, why)
		if cand.Anchor == "dell-poweredge-r750" && cand.Companion == "ippon-innova-rt-ii" {
			p203 = &sc.Candidates[i]
			if !ok {
				t.Fatalf("P2-03 не прошла структуру: %s", why)
			}
		} else if ok {
			t.Fatalf("кандидат %s прошёл структуру — ожидался только P2-03", cand.Key)
		}
	}
	if p203 == nil {
		t.Fatal("P2-03 не найдена")
	}
	if i := slices.IndexFunc(sc.Candidates, func(x copurchaseCandidate) bool { return x.Key == p203.Key }); i != 0 {
		t.Fatalf("P2-03 должна быть первой по оценке приоритета, а она %d-я", i+1)
	}
	passing, state, cond := compatSummary(c.pairSKUCompat(p203.Anchor, p203.Companion))
	if len(passing) != 2 || state != domain.CompatConditional {
		t.Fatalf("совместимость ИБП: %v %s %s", passing, state, cond)
	}
	if ok, why := sc.lateRepeat(p203.Anchor, p203.Companion); !ok {
		t.Fatalf("P2-03 не повторилась: %v", why)
	}
	if ok, _, _ := c.licenseCheck(p203.Anchor, passing); !ok {
		t.Fatal("лицензии")
	}
	// Шум: N-01 (один аккаунт), N-02 (заменители) — не кандидаты.
	for _, n := range [][2]string{{"dlink-dgs-1210", "hp-e24-g5"}, {"hp-probook-450-g10", "lenovo-thinkpad-e14-gen5"}} {
		if slices.ContainsFunc(sc.Candidates, func(x copurchaseCandidate) bool {
			return pairKey(x.Anchor, x.Companion) == pairKey(n[0], n[1])
		}) {
			t.Fatalf("шум %v попал в кандидаты", n)
		}
	}
	// Блок совместимости ИБП к серверу: Smart Power Pro II 1200 Вт < 1400 Вт у R750 с Gold 6338.
	for _, it := range c.pairSKUCompat("dell-poweredge-r750", "ippon-smart-power-pro-ii") {
		if it.State != domain.CompatIncompatible {
			t.Fatalf("ИБП 1200 Вт к R750: %+v", it)
		}
	}
}
