package repositories

import (
	"context"
	"errors"
	"time"

	"kuznica/api/domain"
)

// TraceRow — сохранённая трасса поиска и строка журнала поиска к ней.
type TraceRow struct {
	RequestID  string
	Query      string
	Normalized string
	Config     string
	Route      string
	Source     string
	ClientID   *string
	CreatedAt  time.Time
	Trace      domain.JSONB
}

// ErrNotFound — записи нет.
var ErrNotFound = errors.New("repositories: запись не найдена")

// TraceByRequestID — трасса из search_traces и исходный запрос из search_events.
func (r *Repos) TraceByRequestID(ctx context.Context, requestID string) (*TraceRow, error) {
	if !r.Enabled() {
		return nil, ErrNoDatabase
	}
	var row TraceRow
	res := r.db.WithContext(ctx).Raw(`
SELECT t.request_id, coalesce(e.query, '') AS query, coalesce(e.normalized, '') AS normalized,
       coalesce(e.config, '') AS config, coalesce(e.route, '') AS route, coalesce(e.source, '') AS source,
       e.client_id, t.created_at, t.trace
FROM search_traces t LEFT JOIN search_events e ON e.request_id = t.request_id
WHERE t.request_id = ? LIMIT 1`, requestID).Scan(&row)
	if res.Error != nil {
		return nil, res.Error
	}
	if res.RowsAffected == 0 {
		return nil, ErrNotFound
	}
	return &row, nil
}

// QueriesByRequestID — исходные запросы поисков по их request_id (для журнала вызовов модели).
func (r *Repos) QueriesByRequestID(ctx context.Context, ids []string) (map[string]string, error) {
	res := map[string]string{}
	if !r.Enabled() {
		return res, ErrNoDatabase
	}
	if len(ids) == 0 {
		return res, nil
	}
	var rows []struct {
		RequestID string
		Query     string
	}
	err := r.db.WithContext(ctx).Model(&domain.SearchEvent{}).Select("request_id, query").
		Where("request_id IN ?", ids).Scan(&rows).Error
	for _, row := range rows {
		res[row.RequestID] = row.Query
	}
	return res, err
}
