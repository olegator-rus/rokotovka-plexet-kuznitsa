package services

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/tei"
)

// Правки по нагрузочному тесту: П2 — кэш вызовов сервиса моделей,
// П3 — шум не идёт на маршрут D, П4 — разделители и склейки, аналог отсутствующего товара по связям.

// П4: разделители и склейка числа с единицей.
func TestSplitGlued(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	for q, want := range map[string]string{
		"касперский/10":   "kaspersky 10",
		"касперский,10пк": "kaspersky 10 пк",
		"10пк":            "10 пк",
		"kaspersky/10":    "kaspersky 10",
		"16ядер":          "16 ядер",
		"1с":              "1с",
		"1c ,e[ufknthbz":  "1с бухгалтерия", // запятая — буква «б» в английской раскладке
	} {
		if got := normalizeFull(q, lx, rules, false).normalized(); got != want {
			t.Errorf("%q → %q, want %q", q, got, want)
		}
	}
	if got := splitGlued([]string{"3d", "x64", "1с", "2года", "8.3"}); !slices.Equal(got, []string{"3d", "x64", "1с", "2", "года", "8.3"}) {
		t.Errorf("splitGlued: %v", got) // «3d», «x64», «1с» — не количество и не срок
	}
	for _, q := range []string{"касперский/10пк", "касперский,10пк", "10пк"} {
		_, a, _ := entitiesFor(t, q, false)
		if a.quantity != 10 || a.metric != domain.MetricDevice {
			t.Errorf("%q: количество %d, метрика %q", q, a.quantity, a.metric)
		}
	}
}

// П3: разметка, SQL, одни знаки и цифры — шум: маршрут не D, сигнал noise. Слова каталога из одной буквы
// с цифрой («1с») и обычные запросы — не шум.
func TestNoiseQueryNotRouteD(t *testing.T) {
	for _, q := range []string{"' OR 1=1 --", "SELECT * FROM products", "<script>alert(1)</script>", "{}", "!!!", "%%%",
		"---", `\`, "№1", "123", "drop table users;--"} {
		nr, a, e := entitiesFor(t, q, false)
		route, signals := routeSignals(nr, a, e)
		if !e.noise || route == domain.RouteIntent || !slices.Equal(signals, []string{signalNoise}) {
			t.Errorf("%q: noise=%v route=%s signals=%v", q, e.noise, route, signals)
		}
	}
	for _, q := range []string{"1с", "касперский на 10 пк", "солидворкс", "c++ ide", "1С:Предприятие", "windows 11",
		"KL1939RDBFS", "select антивирус", "асдф"} {
		if _, _, e := entitiesFor(t, q, false); e.noise {
			t.Errorf("%q — не шум", q)
		}
	}
}

// П4: товара нет — аналоги по связям substitute от продукта вендора и категории, которые назвал разбор.
func TestAbsentSubstitutes(t *testing.T) {
	lx, _, _ := mockLexicon(t)
	ent := queryEntities{vendors: []string{"autodesk"}, category: "cad-design", unknown: []string{"solidworks"},
		absentName: "SolidWorks"}
	subs := absentSubstitutes(lx, ent)
	if !slices.Contains(subs, "kompas-3d") || !slices.Contains(subs, "nanocad") || slices.Contains(subs, "autodesk-autocad") {
		t.Fatalf("solidworks: %v", subs)
	}
	// Отсутствующий продукт того же вендора («Autodesk Revit»): замены AutoCAD ему не аналоги.
	ent.absentName = "Autodesk Revit"
	if subs := absentSubstitutes(lx, ent); len(subs) != 0 {
		t.Fatalf("revit: %v", subs)
	}
	// Без вендора или категории — честное «нет в каталоге».
	for _, e := range []queryEntities{
		{category: "cad-design", unknown: []string{"x"}},
		{vendors: []string{"autodesk"}, unknown: []string{"x"}},
	} {
		if subs := absentSubstitutes(lx, e); len(subs) != 0 {
			t.Fatalf("%+v: %v", e, subs)
		}
	}
	// Связь в обратную сторону (Кибер Бэкап → Veeam) аналогом не считается.
	if subs := absentSubstitutes(lx, queryEntities{vendors: []string{"cyberprotect"}, category: "backup",
		unknown: []string{"acronis"}}); len(subs) != 0 {
		t.Fatalf("acronis: %v", subs)
	}
}

func TestLRU(t *testing.T) {
	now := time.Unix(0, 0)
	c := newLRU[int](2, time.Minute, func() time.Time { return now })
	c.put("a", 1)
	c.put("b", 2)
	c.get("a")
	c.put("c", 3) // вытесняет b — давно не читали
	if _, ok := c.get("b"); ok {
		t.Fatal("b должен быть вытеснен")
	}
	if v, ok := c.get("a"); !ok || v != 1 {
		t.Fatal("a потерян")
	}
	now = now.Add(2 * time.Minute)
	if _, ok := c.get("a"); ok || c.len() != 1 {
		t.Fatalf("срок истёк, len=%d", c.len())
	}
	if newLRU[int](0, time.Minute, nil) != nil {
		t.Fatal("размер 0 — без кэша")
	}
}

// П2: повтор и одновременные одинаковые запросы ходят в эмбеддер и реранкер один раз; ошибка не кэшируется.
func TestModelCache(t *testing.T) {
	var embeds, reranks atomic.Int32
	var fail atomic.Bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fail.Load() {
			http.Error(w, "down", http.StatusInternalServerError)
			return
		}
		switch r.URL.Path {
		case "/embed":
			embeds.Add(1)
			time.Sleep(20 * time.Millisecond)
			w.Write([]byte("[[" + strings.Repeat("0.1,", embeddingDim-1) + "0.1]]"))
		case "/rerank":
			reranks.Add(1)
			w.Write([]byte(`[{"index":1,"score":0.9},{"index":0,"score":0.1}]`))
		}
	}))
	defer srv.Close()
	c := tei.New(srv.URL, 5*time.Second)
	s := &Services{embedder: c, reranker: c, models: newModelCache(16, time.Hour, nil)}
	ctx := context.Background()

	var wg sync.WaitGroup
	for range 10 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			if v, err := s.queryVector(ctx, "антивирус"); err != nil || len(v) != embeddingDim {
				t.Errorf("вектор: %v", err)
			}
		}()
	}
	wg.Wait()
	if _, err := s.queryVector(ctx, "антивирус"); err != nil || embeds.Load() != 1 {
		t.Fatalf("эмбеддер вызван %d раз, want 1 (%v)", embeds.Load(), err)
	}
	for range 3 {
		if hits, err := s.rerankCached(ctx, "q", []string{"a", "b"}); err != nil || len(hits) != 2 || hits[0].Index != 1 {
			t.Fatalf("реранк: %v %v", hits, err)
		}
	}
	if _, err := s.rerankCached(ctx, "q", []string{"b", "a"}); err != nil || reranks.Load() != 2 {
		t.Fatalf("другой порядок текстов — другой ключ: вызовов %d", reranks.Load())
	}

	fail.Store(true)
	if _, err := s.queryVector(ctx, "новый"); err == nil {
		t.Fatal("ошибка эмбеддера должна вернуться")
	}
	fail.Store(false)
	if _, err := s.queryVector(ctx, "новый"); err != nil || embeds.Load() != 2 {
		t.Fatalf("ошибка закэширована: вызовов %d, %v", embeds.Load(), err)
	}

	// Ожидание прерывается сроком вызывающего, общий вызов доживает и кладёт вектор в кэш.
	short, cancel := context.WithTimeout(ctx, time.Millisecond)
	defer cancel()
	var te *tei.Error
	if _, err := s.queryVector(short, "медленный"); err == nil || !errors.As(err, &te) {
		t.Fatalf("истёкший срок: %v", err)
	}
}
