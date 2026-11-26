package services

import (
	"context"
	"time"
)

// pingTimeout — сколько health ждёт ответа зависимости.
const pingTimeout = time.Second

// DBReady — база подключена и отвечает. Причина отказа наружу не идёт: health отдаёт только ok/down.
func (s *Services) DBReady(ctx context.Context) bool {
	if !s.repos.Enabled() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	return s.repos.Ping(ctx) == nil
}

// SearchReady — OpenSearch настроен и кластер не красный.
func (s *Services) SearchReady(ctx context.Context) bool {
	if !s.search.Enabled() {
		return false
	}
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	return s.search.Ping(ctx) == nil
}

// EmbedderStatus — сервис векторов: ok | down | off (не настроен).
func (s *Services) EmbedderStatus(ctx context.Context) string {
	if !s.embedder.Enabled() {
		return "off"
	}
	ctx, cancel := context.WithTimeout(ctx, pingTimeout)
	defer cancel()
	if s.embedder.Health(ctx) != nil {
		return "down"
	}
	return "ok"
}

// Now — текущее время по часам сервиса (в health — время ответа).
func (s *Services) Now() time.Time { return s.now() }
