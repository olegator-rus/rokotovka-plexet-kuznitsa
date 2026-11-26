package services

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/repositories"

	"go.uber.org/zap"
)

// Прогрев кэша разборов (бэклог «предразбор частотных запросов»): частотные запросы из журнала
// поиска и наборов прогоняются через поиск без журнала — разборы маршрута D ложатся в intent_cache, и на
// питче повтор запроса стоит 0 ₽. Вызовы модели пишутся в llm_usage с request_id warmup-… (вне стоимости
// запроса покупателя); дневной бюджет LLM соблюдается, как у живого поиска.

// WarmupRequest — `go run . warmup --limit 100 --configs B3,B4 --days 90`.
type WarmupRequest struct {
	Limit   int
	Configs []string
	Days    int
}

// WarmupResult — сводка прогрева.
type WarmupResult struct {
	Queries   int      `json:"queries"`
	Configs   []string `json:"configs"`
	Searches  int      `json:"searches"`
	RouteD    int      `json:"route_d"`
	LLM       int      `json:"llm"`   // разобрано моделью сейчас
	Cache     int      `json:"cache"` // уже было в кэше
	Fallback  int      `json:"fallback"`
	CostRub   float64  `json:"cost_rub"`
	Errors    int      `json:"errors"`
	DurationS float64  `json:"duration_s"`
}

const (
	defaultWarmupLimit = 100
	defaultWarmupDays  = 90
)

// Warmup — прогрев кэша разборов частотными запросами.
func (s *Services) Warmup(ctx context.Context, req WarmupRequest) (*WarmupResult, error) {
	if req.Limit <= 0 {
		req.Limit = defaultWarmupLimit
	}
	if req.Days <= 0 {
		req.Days = defaultWarmupDays
	}
	if len(req.Configs) == 0 {
		req.Configs = []string{domain.ConfigB3, domain.ConfigB4}
	}
	started := time.Now()
	freq := map[string]int{}
	text := map[string]string{}
	add := func(q string, n int) {
		k := foldQuery(q)
		if k == "" {
			return
		}
		if _, ok := text[k]; !ok {
			text[k] = strings.TrimSpace(q)
		}
		freq[k] += n
	}
	if s.repos.Enabled() {
		evs, err := s.repos.SearchEventsSince(ctx, s.now().Add(-time.Duration(req.Days)*24*time.Hour))
		if err != nil {
			return nil, err
		}
		for _, e := range evs {
			add(e.Query, 1)
		}
	}
	if loader, ok := s.source.(queryLoader); ok {
		if qs, err := loader.LoadQueries(ctx); err == nil {
			for _, q := range qs {
				add(q.Text, max(q.Freq, 1))
			}
		}
	}
	keys := make([]string, 0, len(freq))
	for k := range freq {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		if freq[keys[i]] != freq[keys[j]] {
			return freq[keys[i]] > freq[keys[j]]
		}
		return keys[i] < keys[j]
	})
	if len(keys) > req.Limit {
		keys = keys[:req.Limit]
	}
	res := &WarmupResult{Queries: len(keys), Configs: req.Configs}
	for _, k := range keys {
		for _, cfg := range req.Configs {
			if err := ctx.Err(); err != nil {
				return res, err
			}
			resp, err := s.SearchWith(ctx, warmupRequestID(), in.SearchRequest{Query: text[k], Config: cfg, Limit: 10, SessionID: "warmup"},
				SearchOptions{NoLog: true})
			res.Searches++
			if err != nil {
				res.Errors++
				continue
			}
			tr := resp.Trace
			if tr == nil || tr.Route != domain.RouteIntent {
				continue
			}
			res.RouteD++
			res.CostRub += tr.CostRub
			switch tr.ParseSource {
			case domain.ParseLLM:
				res.LLM++
			case domain.ParseCache:
				res.Cache++
			default:
				res.Fallback++
			}
		}
	}
	res.CostRub = evalRound4(res.CostRub)
	res.DurationS = float64(time.Since(started).Milliseconds()) / 1000
	zap.L().Info("прогрев кэша разборов", zap.Int("queries", res.Queries), zap.Int("route_d", res.RouteD),
		zap.Int("llm", res.LLM), zap.Int("cache", res.Cache), zap.Int("fallback", res.Fallback), zap.Float64("cost_rub", res.CostRub))
	return res, nil
}

func warmupRequestID() string {
	b := make([]byte, 8)
	_, _ = rand.Read(b)
	return repositories.WarmupRequestPrefix + hex.EncodeToString(b)
}

// String — сводка для stdout.
func (r *WarmupResult) String() string {
	return fmt.Sprintf("Прогрев: %d запросов × %s = %d поисков; маршрут D: %d (моделью %d, из кэша %d, на правилах %d); %.4f ₽; ошибок %d; %.1f с\n",
		r.Queries, strings.Join(r.Configs, ","), r.Searches, r.RouteD, r.LLM, r.Cache, r.Fallback, r.CostRub, r.Errors, r.DurationS)
}
