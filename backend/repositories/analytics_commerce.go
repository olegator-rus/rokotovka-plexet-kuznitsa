package repositories

import (
	"context"
	"time"
)

// Агрегаты допродажи для аналитики и модели эффекта контура коммерции: события блока
// «Дополните решение» и запросы КП. Это события стенда, не трафик магазина.

// CommerceEventCount — тип события блока: событий, сессий и поисков (request_id) с ним.
type CommerceEventCount struct {
	Type     string
	Events   int
	Sessions int
	Requests int
}

// CommerceEventsSince — события контура коммерции (companion_*, spec_remove, quote_request, template_apply) с момента since.
func (r *Repos) CommerceEventsSince(ctx context.Context, since time.Time) ([]CommerceEventCount, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var rows []CommerceEventCount
	err := r.db.WithContext(ctx).Raw(`SELECT type, COUNT(*) AS events, COUNT(DISTINCT session_id) AS sessions,
	COUNT(DISTINCT request_id) AS requests
FROM behavior_events
WHERE created_at >= ? AND type IN ('companion_impression','companion_click','companion_add','companion_dismiss',
	'spec_remove','quote_request','template_apply')
GROUP BY type ORDER BY type`, since).Scan(&rows).Error
	return rows, err
}

// QuoteStats — запросы КП периода: всего и с позициями из блока допродажи (origin = companion).
type QuoteStats struct {
	Total          int
	WithCompanions int
}

// QuoteStatsSince — запросы КП с момента since.
func (r *Repos) QuoteStatsSince(ctx context.Context, since time.Time) (QuoteStats, error) {
	var st QuoteStats
	if !r.Enabled() {
		return st, ErrNoDatabase
	}
	err := r.db.WithContext(ctx).Raw(`SELECT COUNT(*) AS total,
	COUNT(*) FILTER (WHERE EXISTS (SELECT 1 FROM jsonb_array_elements(items) it WHERE it->>'origin' = 'companion')) AS with_companions
FROM quote_requests WHERE created_at >= ?`, since).Scan(&st).Error
	return st, err
}
