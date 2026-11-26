// Package handlers — HTTP-слой (Fiber): in/* → services → out/*. В БД мимо services не лезет.
package handlers

import (
	"kuznica/api/services"
)

type Handler struct {
	services *services.Services
	// origin — PUBLIC_ORIGIN: единственный Origin, с которого принимаются мутации.
	origin string
	// admin — служебные методы /api/admin/* (ADMIN_ENABLED).
	admin bool
	// rate — лимит запросов к /api в минуту с адреса (RATE_LIMIT_PER_MIN); 0 — без лимита.
	rate int
}

// Options — настройки HTTP-слоя из config.
type Options struct {
	PublicOrigin  string
	AdminEnabled  bool
	RatePerMinute int
}

func NewHandlerManager(s *services.Services, opts Options) *Handler {
	return &Handler{services: s, origin: opts.PublicOrigin, admin: opts.AdminEnabled, rate: opts.RatePerMinute}
}
