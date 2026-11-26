// Package handlers — HTTP-слой (Fiber): in/* → services → out/*. В БД мимо services не лезет.
package handlers

import (
	"kuznica/api/services"
)

type Handler struct {
	services *services.Services
	// origin — PUBLIC_ORIGIN: единственный Origin, с которого принимаются мутации.
	origin string
}

// Options — настройки HTTP-слоя из config.
type Options struct {
	PublicOrigin string
}

func NewHandlerManager(s *services.Services, opts Options) *Handler {
	return &Handler{services: s, origin: opts.PublicOrigin}
}
