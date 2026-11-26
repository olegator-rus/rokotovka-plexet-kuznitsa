package services

import (
	"context"
	"time"
)

// dbPingTimeout — сколько health ждёт ответа базы.
const dbPingTimeout = time.Second

// DBReady — база подключена и отвечает. Причина отказа наружу не идёт: health отдаёт только ok/down.
func (s *Services) DBReady(ctx context.Context) bool {
	if !s.repos.Enabled() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, dbPingTimeout)
	defer cancel()
	return s.repos.Ping(ctx) == nil
}

// Now — текущее время по часам сервиса (в health — время ответа).
func (s *Services) Now() time.Time { return s.now() }
