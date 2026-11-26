package main

import (
	"context"
	"encoding/json"
	"fmt"
	mrand "math/rand/v2"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"sync"
	"syscall"
	"time"

	"kuznica/api/services"
)

// `go run . traffic --flow <файл>` — один проход тестового потока SearchOps на фиде (services/traffic_flow.go):
// каждая строка — Freq сессий, сессии в случайном порядке разнесены по окну --duration (по умолчанию 15m). Сессия —
// обычный клиент витрины: GET /api/meta, POST /api/search (source=traffic), затем действие модельного покупателя по
// настоящей выдаче (POST /api/events: click или price_request) или переформулировка правильным названием. В базу и
// таблицы агента генератор не пишет.

const flowDefaultSpread = 15 * time.Minute

func runFlowTraffic(o trafficOpts) int {
	rows, err := services.LoadFlow(o.flowPath)
	if err != nil {
		fmt.Fprintln(os.Stderr, "traffic:", err)
		return 1
	}
	if o.seed == 0 {
		o.seed = uint64(time.Now().UnixNano())
	}
	spread := o.duration
	if spread <= 0 {
		spread = flowDefaultSpread
	}
	sessions := services.FlowSessions(rows, mrand.New(mrand.NewPCG(o.seed, 3)))
	layers := map[string]string{}
	queries, byLayer := map[string]int{}, map[string]int{}
	for _, r := range rows {
		layers[r.ID] = r.Layer
		queries[r.Layer]++
		byLayer[r.Layer] += r.Freq
	}
	if o.dryRun {
		enc := json.NewEncoder(os.Stdout)
		enc.SetIndent("", "  ")
		_ = enc.Encode(map[string]any{"flow": o.flowPath, "queries": len(rows), "queries_by_layer": queries,
			"sessions": len(sessions), "sessions_by_layer": byLayer, "spread": spread.String(), "max_rpm": o.maxRPM,
			"seed": o.seed, "example": sessions[0]})
		return 0
	}
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()
	// Запас на хвост: последняя сессия стартует в конце окна и может переформулировать запрос через 45 с.
	ctx, cancel := context.WithTimeout(ctx, spread+5*time.Minute)
	defer cancel()
	g := newTrafficGen(o, nil)
	g.flowLayer = layers
	started := time.Now()
	fmt.Fprintf(os.Stderr, "traffic: поток %s — %d запросов, %d сессий за %s → %s, потолок %d запросов/мин\n",
		o.flowPath, len(rows), len(sessions), spread, o.api, o.maxRPM)
	if !o.quiet {
		go g.report(ctx, started)
	}
	offsets := services.SpikeOffsets(g.childRand(), services.SpikePlan{Len: spread, Sessions: len(sessions)})
	var wg sync.WaitGroup
	for i, ses := range sessions {
		if !sleepCtx(ctx, time.Until(started.Add(offsets[i]))) {
			break
		}
		wg.Add(1)
		go func() { defer wg.Done(); g.session(ctx, ses) }()
	}
	wg.Wait()
	g.summary(started)
	return 0
}

// flowAct — действие модельного покупателя после поиска строки потока.
func (g *trafficGen) flowAct(ctx context.Context, r *mrand.Rand, token, flowID string, st services.TrafficStep, hit *searchHit) {
	fc := g.flowCounters(flowID)
	fc.sessions.Add(1)
	ch := services.FlowChoose(hit.cards, st.Expect)
	if ch.Action == "" {
		fc.miss.Add(1)
		ok, gap := services.FlowReform(st, r)
		if !ok || !sleepCtx(ctx, gap) {
			return
		}
		hit2, _, err := g.search(ctx, r, token, st.Fix)
		if err != nil {
			g.fail(err)
			return
		}
		fc.reformulated.Add(1)
		if ch = services.FlowChoose(hit2.cards, st.Expect); ch.Action == "" {
			return
		}
		fc.reformFound.Add(1)
		hit = hit2
	} else if ch.Action == "click" {
		fc.found.Add(1)
	} else {
		fc.onRequest.Add(1)
	}
	if !sleepCtx(ctx, time.Duration(3+r.IntN(8))*time.Second) {
		return
	}
	ev := map[string]any{"type": ch.Action, "request_id": hit.requestID, "session_id": token, "product_id": ch.ProductID,
		"position": ch.Position, "source": "traffic"}
	if hit.intentID != "" {
		ev["intent_id"] = hit.intentID
	}
	if err := g.do(ctx, http.MethodPost, "/api/events", ev, nil); err != nil {
		g.fail(err)
		return
	}
	g.st.events.Add(1)
}

func (g *trafficGen) flowCounters(flowID string) *flowCounters {
	layer := g.flowLayer[flowID]
	if layer == "" {
		layer = "?"
	}
	v, _ := g.st.byLayer.LoadOrStore(layer, &flowCounters{})
	return v.(*flowCounters)
}

// flowSummary — исходы модельного покупателя по слоям (в итог прогона).
func (g *trafficGen) flowSummary() map[string]any {
	res := map[string]any{}
	var keys []string
	g.st.byLayer.Range(func(k, _ any) bool { keys = append(keys, k.(string)); return true })
	sort.Strings(keys)
	for _, k := range keys {
		v, _ := g.st.byLayer.Load(k)
		c := v.(*flowCounters)
		res[k] = map[string]any{"sessions": c.sessions.Load(), "found": c.found.Load(), "on_request": c.onRequest.Load(),
			"miss": c.miss.Load(), "reformulated": c.reformulated.Load(), "found_after_reform": c.reformFound.Load()}
	}
	return res
}
