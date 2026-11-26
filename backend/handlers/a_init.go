// Package handlers — HTTP-слой (Fiber): in/* → services → out/*. В БД мимо services не лезет.
package handlers

import (
	"kuznica/api/config"
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
	// token — SHA-256 токена бэк-офиса (BACKOFFICE_TOKEN); nil — бэк-офис без входа (middleware_backoffice.go).
	token []byte
	// sessionKey — ключ HMAC сессий покупателя (session.go).
	sessionKey []byte
	// auth — вход по ролям: обязателен ли, демо-аккаунты, аккаунты по паролю, ключ cookie (auth.go).
	auth authOptions
}

// Options — настройки HTTP-слоя из config.
type Options struct {
	PublicOrigin  string
	AdminEnabled  bool
	RatePerMinute int
	// BackofficeToken — BACKOFFICE_TOKEN; пусто — бэк-офис без входа.
	BackofficeToken string
	// Auth — вход по ролям (AUTH_*, DEMO_LOGIN_BUTTONS). Нулевое значение — вход не обязателен, демо-кнопок нет
	// (так собираются обработчики в тестах прежних волн).
	Auth config.AuthConfig
	// SecureCookie — cookie входа с флагом Secure (production, HTTPS).
	SecureCookie bool
}

func NewHandlerManager(s *services.Services, opts Options) *Handler {
	key := newSessionKey(opts.BackofficeToken)
	return &Handler{services: s, origin: opts.PublicOrigin, admin: opts.AdminEnabled, rate: opts.RatePerMinute,
		token: hashToken(opts.BackofficeToken), sessionKey: key, auth: newAuthOptions(opts.Auth, key, opts.SecureCookie)}
}
