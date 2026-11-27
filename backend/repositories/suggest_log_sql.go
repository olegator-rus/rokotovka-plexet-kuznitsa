package repositories

import (
	"context"
	"encoding/json"
	"fmt"
	"time"
)

// QueryStat — запрос журнала текущего каталога для подсказок: поисков, с непустой выдачей, с кликом (успех),
// из них — модельный поток (source = traffic).
type QueryStat struct {
	Query    string `json:"query"`
	Searches int    `json:"searches"`
	NonEmpty int    `json:"non_empty"`
	Clicks   int    `json:"clicks"`
	Traffic  int    `json:"traffic"`
}

// QueryStatsScoped — все запросы журнала с момента since в границах LogScope (как вход агента на фиде): текст
// после свёртки регистра и пробелов, число поисков, непустых, с кликом и из модельного потока. Источник подсказок
// при вводе на фиде (контур эталонов): у фида файла журнала нет, журнал — в базе.
func (r *Repos) QueryStatsScoped(ctx context.Context, since time.Time, sc LogScope) ([]QueryStat, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var raw []byte
	err := r.db.WithContext(ctx).Raw(agentOutcomesCTE+`
SELECT COALESCE(json_agg(t ORDER BY t->>'query'), '[]') FROM (
	SELECT json_build_object('query', q, 'searches', COUNT(*),
		'non_empty', COUNT(*) FILTER (WHERE results_count <> 0),
		'clicks', COUNT(*) FILTER (WHERE clicked),
		'traffic', COUNT(*) FILTER (WHERE source = 'traffic')) AS t
	FROM o WHERE q <> '' GROUP BY q) x`,
		sc.args(map[string]any{"since": since, "until": openEnd})).Row().Scan(&raw)
	if err != nil {
		return nil, err
	}
	var res []QueryStat
	if err := json.Unmarshal(raw, &res); err != nil {
		return nil, fmt.Errorf("запросы журнала для подсказок: %w", err)
	}
	return res, nil
}
