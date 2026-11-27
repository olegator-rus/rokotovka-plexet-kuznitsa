package main

import (
	"bytes"
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"math"
	mrand "math/rand/v2"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"strings"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"kuznica/api/services"
)

// `go run . traffic` — генератор mock-трафика стенда (контур автономии). Обычный HTTP-клиент витрины: GET /api/meta
// (сессия), GET /api/suggest, POST /api/search и POST /api/events с source=traffic; гость, без входа. В базу
// напрямую не ходит и таблиц агента не касается — поэтому стартует до конфигурации и миграции API.
//
//	go run . traffic --api http://127.0.0.1:8088 --duration 1h --spikes vpn-empty@20m
//	go run . traffic --probe          # один поиск по каждой формулировке всплесков: выдача сейчас пустая?
//	go run . traffic --dry-run        # план без запросов
//	go run . traffic --flow ../data/softline/searchops_flow.jsonl --duration 15m --panel 0
//	                                  # один проход тестового потока SearchOps на фиде (traffic_flow.go)
//
// Порог лимита: --max-rpm (HTTP-запросов в минуту на весь генератор) должен быть ниже RATE_LIMIT_PER_MIN API.

const trafficUserAgent = "kuznica-traffic/1"

type trafficOpts struct {
	api, origin          string
	dataDir, spikesPath  string
	flowPath             string
	duration             time.Duration
	sessionsPerMin       float64
	maxRPM               int
	panelShare           float64
	spikes               string
	spikeLen             time.Duration
	spikeSessions        int
	repeat               time.Duration
	seed                 uint64
	dryRun, probe, quiet bool
}

func runTraffic(args []string) int {
	fs := flag.NewFlagSet("traffic", flag.ContinueOnError)
	o := trafficOpts{}
	fs.StringVar(&o.api, "api", envOr("TRAFFIC_API_URL", "http://127.0.0.1:8088"), "адрес API (без /api)")
	fs.StringVar(&o.origin, "origin", envOr("PUBLIC_ORIGIN", "http://localhost:5177"),
		"заголовок Origin изменяющих запросов (= PUBLIC_ORIGIN API)")
	fs.StringVar(&o.dataDir, "data", envOr("DATA_DIR", "../data/mock"), "каталог mock-данных (events.jsonl)")
	fs.StringVar(&o.flowPath, "flow", envOr("TRAFFIC_FLOW_FILE", ""),
		"тестовый трафик SearchOps (JSONL): один проход вместо журнала events.jsonl; --duration — окно, по которому разнесены сессии")
	fs.StringVar(&o.spikesPath, "spikes-file", envOr("TRAFFIC_SPIKES_FILE", "../data/traffic/spikes.json"),
		"описания всплесков")
	fs.DurationVar(&o.duration, "duration", 0, "сколько работать (0 — до сигнала)")
	fs.Float64Var(&o.sessionsPerMin, "rate", 2, "сессий фона в минуту (в среднем, пуассоновский поток)")
	fs.IntVar(&o.maxRPM, "max-rpm", 120, "потолок HTTP-запросов генератора в минуту (ниже RATE_LIMIT_PER_MIN)")
	fs.Float64Var(&o.panelShare, "panel", 0.5, "доля поисков через панель (suggest + события намерения)")
	fs.StringVar(&o.spikes, "spikes", "", "расписание всплесков: id@смещение через запятую, например vpn-empty@20m")
	fs.DurationVar(&o.spikeLen, "spike-len", 15*time.Minute, "длительность одного всплеска")
	fs.IntVar(&o.spikeSessions, "spike-sessions", 40, "сессий в одном всплеске")
	fs.DurationVar(&o.repeat, "repeat", 0, "повторять расписание всплесков с этим периодом (0 — один раз)")
	fs.Uint64Var(&o.seed, "seed", 0, "зерно случайности (0 — от времени)")
	fs.BoolVar(&o.dryRun, "dry-run", false, "показать план и выйти без запросов")
	fs.BoolVar(&o.probe, "probe", false, "проверить формулировки всплесков одним поиском каждая и выйти")
	fs.BoolVar(&o.quiet, "quiet", false, "без поминутной сводки в stderr")
	if err := fs.Parse(args); err != nil {
		return 2
	}
	if err := o.validate(); err != nil {
		fmt.Fprintln(os.Stderr, "traffic:", err)
		return 2
	}
	if o.flowPath != "" {
		return runFlowTraffic(o)
	}
	model, err := services.LoadTrafficModel(o.dataDir, o.spikesPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "traffic:", err)
		return 1
	}
	plan, err := services.ParseSpikePlan(o.spikes, o.spikeLen, o.spikeSessions)
	if err != nil {
		fmt.Fprintln(os.Stderr, "traffic:", err)
		return 2
	}
	for _, p := range plan {
		if _, ok := model.Spike(p.ID); !ok {
			fmt.Fprintf(os.Stderr, "traffic: всплеска %q нет в %s\n", p.ID, o.spikesPath)
			return 2
		}
	}
	if o.seed == 0 {
		o.seed = uint64(time.Now().UnixNano())
	}
	if o.dryRun {
		return printTrafficPlan(o, model, plan)
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	g := newTrafficGen(o, model)
	if o.probe {
		return g.probe(ctx)
	}
	return g.run(ctx, plan)
}

func (o trafficOpts) validate() error {
	u, err := url.Parse(o.api)
	switch {
	case err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "":
		return fmt.Errorf("--api: ожидается http(s)://хост[:порт]")
	case o.origin == "":
		return fmt.Errorf("--origin пуст: изменяющие запросы API примет только с Origin = PUBLIC_ORIGIN")
	case o.sessionsPerMin <= 0 || o.sessionsPerMin > 60:
		return fmt.Errorf("--rate: от 0 до 60 сессий в минуту")
	case o.maxRPM <= 0 || o.maxRPM > 1000:
		return fmt.Errorf("--max-rpm: от 1 до 1000")
	case o.panelShare < 0 || o.panelShare > 1:
		return fmt.Errorf("--panel: доля от 0 до 1")
	case o.spikeSessions < 0 || o.spikeSessions > 1000:
		return fmt.Errorf("--spike-sessions: от 0 до 1000")
	case o.spikeLen <= 0:
		return fmt.Errorf("--spike-len: положительная длительность")
	case o.repeat < 0:
		return fmt.Errorf("--repeat: неотрицательная длительность")
	}
	return nil
}

func envOr(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func printTrafficPlan(o trafficOpts, m *services.TrafficModel, plan []services.SpikePlan) int {
	r := mrand.New(mrand.NewPCG(o.seed, 1))
	out := map[string]any{
		"api": o.api, "background_sessions": len(m.Sessions), "background_searches": m.Searches,
		"rate_sessions_per_min": o.sessionsPerMin, "max_rpm": o.maxRPM, "panel_share": o.panelShare,
		"duration": o.duration.String(), "repeat": o.repeat.String(), "seed": o.seed,
	}
	var spikes []map[string]any
	for _, p := range plan {
		s, _ := m.Spike(p.ID)
		spikes = append(spikes, map[string]any{"id": p.ID, "kind": s.Kind, "label": s.Label, "at": p.At.String(),
			"len": p.Len.String(), "sessions": p.Sessions, "example": s.SpikeSession(r)})
	}
	out["spikes"] = spikes
	out["example_background"] = m.PickBackground(r)
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
	return 0
}

// ── исполнитель ──

type trafficGen struct {
	o      trafficOpts
	m      *services.TrafficModel
	client *http.Client

	tick <-chan time.Time // ровный темп запросов: не больше maxRPM в минуту
	mu   sync.Mutex
	r    *mrand.Rand

	st trafficStats
	// flowLayer — строка потока → слой (сводка по слоям).
	flowLayer map[string]string
}

type spikeCounters struct{ sessions, searches, empty atomic.Int64 }

// flowCounters — исходы модельного покупателя потока по слою.
type flowCounters struct{ sessions, found, onRequest, miss, reformulated, reformFound atomic.Int64 }

type trafficStats struct {
	sessions, spikeSessions, searches, empty, suggests, events, skippedActions atomic.Int64
	errors, limited                                                            atomic.Int64
	bySpike                                                                    sync.Map // id → *spikeCounters
	byLayer                                                                    sync.Map // слой потока → *flowCounters
}

func newTrafficGen(o trafficOpts, m *services.TrafficModel) *trafficGen {
	return &trafficGen{o: o, m: m, client: &http.Client{Timeout: 30 * time.Second},
		tick: time.Tick(time.Minute / time.Duration(o.maxRPM)), r: mrand.New(mrand.NewPCG(o.seed, 2))}
}

// childRand — свой генератор для горутины сессии (общий *rand.Rand не потокобезопасен).
func (g *trafficGen) childRand() *mrand.Rand {
	g.mu.Lock()
	defer g.mu.Unlock()
	return mrand.New(mrand.NewPCG(g.r.Uint64(), g.r.Uint64()))
}

func (g *trafficGen) run(ctx context.Context, plan []services.SpikePlan) int {
	if g.o.duration > 0 {
		var cancel context.CancelFunc
		ctx, cancel = context.WithTimeout(ctx, g.o.duration)
		defer cancel()
	}
	started := time.Now()
	fmt.Fprintf(os.Stderr, "traffic: старт %s → %s, фон %.1f сессий/мин, потолок %d запросов/мин, всплесков в плане %d\n",
		started.Format(time.RFC3339), g.o.api, g.o.sessionsPerMin, g.o.maxRPM, len(plan))
	var wg sync.WaitGroup
	wg.Add(1)
	go func() { defer wg.Done(); g.background(ctx, &wg) }()
	wg.Add(1)
	go func() { defer wg.Done(); g.spikes(ctx, &wg, plan) }()
	if !g.o.quiet {
		go g.report(ctx, started)
	}
	<-ctx.Done()
	wg.Wait()
	g.summary(started)
	return 0
}

// background — пуассоновский поток сессий фона.
func (g *trafficGen) background(ctx context.Context, wg *sync.WaitGroup) {
	r := g.childRand()
	mean := float64(time.Minute) / g.o.sessionsPerMin
	for {
		wait := time.Duration(r.ExpFloat64() * mean)
		if !sleepCtx(ctx, wait) {
			return
		}
		ses := g.m.PickBackground(r)
		wg.Add(1)
		go func() { defer wg.Done(); g.session(ctx, ses) }()
	}
}

// spikes — расписание всплесков (с повтором раз в repeat).
func (g *trafficGen) spikes(ctx context.Context, wg *sync.WaitGroup, plan []services.SpikePlan) {
	if len(plan) == 0 {
		return
	}
	start := time.Now()
	for cycle := 0; ; cycle++ {
		base := start.Add(time.Duration(cycle) * g.o.repeat)
		for _, p := range plan {
			wg.Add(1)
			go func() { defer wg.Done(); g.spike(ctx, wg, p, base.Add(p.At)) }()
		}
		if g.o.repeat <= 0 {
			return
		}
		if !sleepCtx(ctx, time.Until(start.Add(time.Duration(cycle+1)*g.o.repeat))) {
			return
		}
	}
}

func (g *trafficGen) spike(ctx context.Context, wg *sync.WaitGroup, p services.SpikePlan, at time.Time) {
	if !sleepCtx(ctx, time.Until(at)) {
		return
	}
	s, _ := g.m.Spike(p.ID)
	r := g.childRand()
	fmt.Fprintf(os.Stderr, "traffic: всплеск %s (%s) — %d сессий за %s\n", s.ID, s.Kind, p.Sessions, p.Len)
	begin := time.Now()
	for _, off := range services.SpikeOffsets(r, p) {
		if !sleepCtx(ctx, time.Until(begin.Add(off))) {
			return
		}
		ses := s.SpikeSession(r)
		wg.Add(1)
		go func() { defer wg.Done(); g.session(ctx, ses) }()
	}
}

type searchHit struct {
	requestID string
	intentID  string
	top       string         // product_id первой позиции
	positions map[string]int // product_id (представитель и варианты семейства) → позиция семейства, с 1
	cards     []services.FlowCard
}

// session — сценарий одной сессии: сессия от сервера, затем шаги с паузами.
func (g *trafficGen) session(ctx context.Context, ses services.TrafficSession) {
	r := g.childRand()
	token, err := g.meta(ctx)
	if err != nil {
		g.fail(err)
		return
	}
	g.st.sessions.Add(1)
	var sc *spikeCounters
	if ses.Spike != "" {
		g.st.spikeSessions.Add(1)
		v, _ := g.st.bySpike.LoadOrStore(ses.Spike, &spikeCounters{})
		sc = v.(*spikeCounters)
		sc.sessions.Add(1)
	}
	var last *searchHit
	for _, st := range ses.Steps {
		if !sleepCtx(ctx, st.Gap) {
			return
		}
		if st.Kind == services.TrafficSearch {
			hit, total, err := g.search(ctx, r, token, st.Query)
			if err != nil {
				g.fail(err)
				last = nil
				continue
			}
			last = hit
			if st.Flow {
				g.flowAct(ctx, r, token, ses.Flow, st, hit)
				continue
			}
			if sc != nil {
				sc.searches.Add(1)
				if total == 0 {
					sc.empty.Add(1)
				}
			}
			continue
		}
		pid, pos := "", 0
		if last != nil {
			if st.ClickTop && last.top != "" {
				pid, pos = last.top, 1
			} else if p, ok := last.positions[st.ProductID]; ok {
				pid, pos = st.ProductID, p
			}
		}
		if pos == 0 {
			g.st.skippedActions.Add(1) // товара журнала в текущей выдаче нет — покупатель ушёл без действия
			continue
		}
		ev := map[string]any{"type": st.Kind, "request_id": last.requestID, "session_id": token, "product_id": pid,
			"position": pos, "source": "traffic"}
		if last.intentID != "" {
			ev["intent_id"] = last.intentID
		}
		if err := g.do(ctx, http.MethodPost, "/api/events", ev, nil); err != nil {
			g.fail(err)
			continue
		}
		g.st.events.Add(1)
	}
}

type searchResp struct {
	RequestID string `json:"request_id"`
	Total     int    `json:"total"`
	Items     []struct {
		ProductID      string `json:"product_id"`
		FamilyID       string `json:"family_id"`
		PriceOnRequest bool   `json:"price_on_request"`
		PriceState     string `json:"price_state"`
		Variants       []struct {
			ProductID string `json:"product_id"`
		} `json:"variants"`
	} `json:"items"`
}

// search — поиск; с долей panelShare — через панель: search_opened, подсказки на префиксах, показ панели,
// поиск и search_commit с числом результатов (как SearchPanel.vue).
func (g *trafficGen) search(ctx context.Context, r *mrand.Rand, token, q string) (*searchHit, int, error) {
	intent := ""
	prefixes := services.PanelPrefixes(q)
	if len(prefixes) > 0 && r.Float64() < g.o.panelShare {
		intent = "tr-" + randHex(8)
		g.intentEvent(ctx, token, intent, "search_opened", map[string]any{"state": "empty"})
		for _, p := range prefixes {
			if !sleepCtx(ctx, time.Duration(300+r.IntN(900))*time.Millisecond) {
				return nil, 0, ctx.Err()
			}
			items, sections, err := g.suggest(ctx, token, intent, p)
			if err != nil {
				g.fail(err)
				continue
			}
			g.intentEvent(ctx, token, intent, "suggest_impression",
				map[string]any{"prefix_len": len([]rune(p)), "sections": sections, "items": items})
		}
	}
	body := map[string]any{"query": q, "session_id": token, "source": "traffic"}
	if intent != "" {
		body["intent_id"] = intent
	}
	var resp searchResp
	if err := g.do(ctx, http.MethodPost, "/api/search", body, &resp); err != nil {
		return nil, 0, err
	}
	g.st.searches.Add(1)
	if resp.Total == 0 {
		g.st.empty.Add(1)
	}
	hit := &searchHit{requestID: resp.RequestID, intentID: intent, positions: map[string]int{}}
	for i, it := range resp.Items {
		if i == 0 {
			hit.top = it.ProductID
		}
		if _, ok := hit.positions[it.ProductID]; !ok && it.ProductID != "" {
			hit.positions[it.ProductID] = i + 1
		}
		onReq := it.PriceState == "on_request" || it.PriceState == "" && it.PriceOnRequest
		hit.cards = append(hit.cards, services.FlowCard{FamilyID: it.FamilyID, ProductID: it.ProductID, OnRequest: onReq})
		for _, v := range it.Variants {
			if _, ok := hit.positions[v.ProductID]; !ok && v.ProductID != "" {
				hit.positions[v.ProductID] = i + 1
			}
		}
	}
	if intent != "" {
		g.intentEvent(ctx, token, intent, "search_commit", map[string]any{"outcome": "commit", "source": "enter",
			"prefix_len": len([]rune(q)), "results_count": resp.Total})
	}
	return hit, resp.Total, nil
}

func (g *trafficGen) intentEvent(ctx context.Context, token, intent, typ string, payload map[string]any) {
	ev := map[string]any{"type": typ, "session_id": token, "intent_id": intent, "payload": payload, "source": "traffic"}
	if err := g.do(ctx, http.MethodPost, "/api/events", ev, nil); err != nil {
		g.fail(err)
		return
	}
	g.st.events.Add(1)
}

// suggest — GET /api/suggest; число строк и непустые секции — для suggest_impression.
func (g *trafficGen) suggest(ctx context.Context, token, intent, prefix string) (int, []string, error) {
	var resp struct {
		Refinements []json.RawMessage `json:"refinements"`
		Suggestions []json.RawMessage `json:"suggestions"`
		Categories  []json.RawMessage `json:"categories"`
		Vendors     []json.RawMessage `json:"vendors"`
		Products    []json.RawMessage `json:"products"`
	}
	q := url.Values{"q": {prefix}, "intent_id": {intent}, "session_id": {token}}
	if err := g.do(ctx, http.MethodGet, "/api/suggest?"+q.Encode(), nil, &resp); err != nil {
		return 0, nil, err
	}
	g.st.suggests.Add(1)
	sections := []string{}
	for _, s := range []struct {
		name string
		n    int
	}{{"refinements", len(resp.Refinements)}, {"suggestions", len(resp.Suggestions)},
		{"categories", len(resp.Categories)}, {"vendors", len(resp.Vendors)}, {"products", len(resp.Products)}} {
		if s.n > 0 {
			sections = append(sections, s.name)
		}
	}
	return len(resp.Suggestions) + len(resp.Categories) + len(resp.Vendors) + len(resp.Products), sections, nil
}

// meta — GET /api/meta: подписанная сессия покупателя от сервера (самодельную API пишет как «без сессии»).
func (g *trafficGen) meta(ctx context.Context) (string, error) {
	var resp struct {
		Session string `json:"session"`
	}
	if err := g.do(ctx, http.MethodGet, "/api/meta", nil, &resp); err != nil {
		return "", err
	}
	return resp.Session, nil
}

// errLimited — API ответило 429: генератор ждёт Retry-After и пропускает шаг.
type errLimited struct{ wait time.Duration }

func (e errLimited) Error() string { return "429: лимит частоты API" }

// do — запрос с соблюдением потолка частоты; 429 — ожидание Retry-After (не больше минуты) и ошибка шага.
func (g *trafficGen) do(ctx context.Context, method, path string, body, dst any) error {
	select {
	case <-ctx.Done():
		return ctx.Err()
	case <-g.tick:
	}
	var rd io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			return err
		}
		rd = bytes.NewReader(raw)
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimRight(g.o.api, "/")+path, rd)
	if err != nil {
		return err
	}
	req.Header.Set("User-Agent", trafficUserAgent)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Origin", g.o.origin)
	}
	resp, err := g.client.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusTooManyRequests {
		wait := 5 * time.Second
		if s, err := time.ParseDuration(resp.Header.Get("Retry-After") + "s"); err == nil && s > 0 {
			wait = min(s, time.Minute)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		sleepCtx(ctx, wait)
		return errLimited{wait}
	}
	if resp.StatusCode >= 300 {
		msg, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("%s %s: %d %s", method, strings.SplitN(path, "?", 2)[0], resp.StatusCode, bytes.TrimSpace(msg))
	}
	if dst == nil {
		_, _ = io.Copy(io.Discard, resp.Body)
		return nil
	}
	return json.NewDecoder(resp.Body).Decode(dst)
}

func (g *trafficGen) fail(err error) {
	if err == nil || err == context.Canceled || err == context.DeadlineExceeded {
		return
	}
	if _, ok := err.(errLimited); ok {
		g.st.limited.Add(1)
		return
	}
	if g.st.errors.Add(1) <= 20 {
		fmt.Fprintln(os.Stderr, "traffic: ошибка:", err)
	}
}

// report — поминутная сводка в stderr.
func (g *trafficGen) report(ctx context.Context, started time.Time) {
	t := time.NewTicker(time.Minute)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			fmt.Fprintf(os.Stderr, "traffic: %s — сессий %d (всплеск %d), поисков %d, пустых %d, событий %d, подсказок %d, ошибок %d, 429: %d\n",
				time.Since(started).Round(time.Second), g.st.sessions.Load(), g.st.spikeSessions.Load(), g.st.searches.Load(),
				g.st.empty.Load(), g.st.events.Load(), g.st.suggests.Load(), g.st.errors.Load(), g.st.limited.Load())
		}
	}
}

// summary — итог прогона JSON-ом в stdout.
func (g *trafficGen) summary(started time.Time) {
	searches, empty := g.st.searches.Load(), g.st.empty.Load()
	out := map[string]any{
		"started_at": started.UTC().Format(time.RFC3339), "duration_s": int(time.Since(started).Seconds()),
		"api": g.o.api, "seed": g.o.seed, "sessions": g.st.sessions.Load(), "spike_sessions": g.st.spikeSessions.Load(),
		"searches": searches, "empty": empty, "empty_share": ratio(empty, searches), "events": g.st.events.Load(),
		"suggests": g.st.suggests.Load(), "skipped_actions": g.st.skippedActions.Load(), "errors": g.st.errors.Load(),
		"rate_limited": g.st.limited.Load(),
	}
	spikes := map[string]any{}
	g.st.bySpike.Range(func(k, v any) bool {
		c := v.(*spikeCounters)
		spikes[k.(string)] = map[string]any{"sessions": c.sessions.Load(), "searches": c.searches.Load(),
			"empty": c.empty.Load(), "empty_share": ratio(c.empty.Load(), c.searches.Load())}
		return true
	})
	out["by_spike"] = spikes
	if g.flowLayer != nil {
		out["flow_by_layer"] = g.flowSummary()
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(out)
}

// probe — один поиск по каждой формулировке всплесков (и переформулировке): пустая ли выдача сейчас. Поиски
// пишутся в журнал как source=traffic.
func (g *trafficGen) probe(ctx context.Context) int {
	token, err := g.meta(ctx)
	if err != nil {
		fmt.Fprintln(os.Stderr, "traffic:", err)
		return 1
	}
	type row struct {
		Spike string `json:"spike"`
		Kind  string `json:"kind"`
		Query string `json:"query"`
		Role  string `json:"role"`
		Total int    `json:"total"`
		OK    bool   `json:"ok"`
	}
	var rows []row
	bad := 0
	for _, s := range g.m.Spikes {
		check := func(q, role string, wantEmpty bool) {
			var resp searchResp
			if err := g.do(ctx, http.MethodPost, "/api/search", map[string]any{"query": q, "session_id": token,
				"source": "traffic"}, &resp); err != nil {
				fmt.Fprintln(os.Stderr, "traffic:", err)
				bad++
				return
			}
			ok := (resp.Total == 0) == wantEmpty
			if s.Kind == services.SpikeReform && role == "fail" {
				ok = true // неудача reform — переформулировка без клика; пустота необязательна
			}
			if !ok {
				bad++
			}
			rows = append(rows, row{Spike: s.ID, Kind: s.Kind, Query: q, Role: role, Total: resp.Total, OK: ok})
		}
		for _, q := range s.Queries {
			check(q, "fail", true)
		}
		if s.Fix != "" {
			check(s.Fix, "fix", false)
		}
	}
	enc := json.NewEncoder(os.Stdout)
	enc.SetIndent("", "  ")
	_ = enc.Encode(map[string]any{"rows": rows, "mismatches": bad})
	if bad > 0 {
		return 1
	}
	return 0
}

func ratio(n, d int64) any {
	if d == 0 {
		return nil
	}
	return math.Round(float64(n)/float64(d)*10000) / 10000
}

func randHex(n int) string {
	b := make([]byte, n)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// sleepCtx — пауза; false — контекст завершён.
func sleepCtx(ctx context.Context, d time.Duration) bool {
	if d <= 0 {
		return ctx.Err() == nil
	}
	t := time.NewTimer(d)
	defer t.Stop()
	select {
	case <-ctx.Done():
		return false
	case <-t.C:
		return true
	}
}
