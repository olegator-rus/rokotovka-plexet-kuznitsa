// Package config — настройки сервиса из окружения. ЕДИНСТВЕННОЕ место, где API читает
// переменные: остальные пакеты получают готовые значения, а неверная конфигурация видна при
// старте одной сводкой.
//
// Секретов в логи не попадает: Summary печатает только, задано ли значение.
package config

import (
	"errors"
	"fmt"
	"net"
	"net/netip"
	"net/url"
	"os"
	"strings"
)

// Значения APP_ENV.
const (
	EnvDevelopment = "development"
	EnvProduction  = "production"
)

// devOrigin — dev-сервер Vite: в разработке фронт и /api на одном origin через прокси Vite.
const devOrigin = "http://localhost:5177"

// defaultAddr — адрес API по умолчанию: только loopback.
const defaultAddr = "127.0.0.1:8088"

type Config struct {
	AppEnv string // development | production
	Addr   string // PORT: адрес HTTP-сервера, 127.0.0.1:8088 по умолчанию
	// PublicOrigin — PUBLIC_ORIGIN: origin магазина; по нему проверяется Origin на мутациях.
	// В development по умолчанию — dev-сервер Vite, в production обязателен.
	PublicOrigin string
	// DatabaseURL — в development пусто допустимо: API стартует без базы (WARN, health db: down).
	// В production обязателен.
	DatabaseURL string
	// TrustedProxies — TRUSTED_PROXIES: IP или CIDR обратного прокси (Caddy) через запятую. Только от
	// них API берёт адрес клиента из X-Real-IP. Пусто — заголовок игнорируется. В production обязателен.
	TrustedProxies []string
}

func (c Config) IsProduction() bool { return c.AppEnv == EnvProduction }

func Load() (Config, error) {
	c := Config{
		AppEnv:         env("APP_ENV", EnvDevelopment),
		Addr:           addr(os.Getenv("PORT")),
		PublicOrigin:   env("PUBLIC_ORIGIN", ""),
		DatabaseURL:    env("DATABASE_URL", ""),
		TrustedProxies: list(os.Getenv("TRUSTED_PROXIES")),
	}
	var problems []string
	for _, a := range c.TrustedProxies {
		if !validProxy(a) {
			problems = append(problems, fmt.Sprintf("TRUSTED_PROXIES: %q — не IP и не CIDR", a))
		}
	}
	switch c.AppEnv {
	case EnvDevelopment:
		if c.PublicOrigin == "" {
			c.PublicOrigin = devOrigin
		}
	case EnvProduction:
		if c.PublicOrigin == "" {
			problems = append(problems, "PUBLIC_ORIGIN не задан")
		}
		if c.DatabaseURL == "" {
			problems = append(problems, "DATABASE_URL не задан")
		}
		if len(c.TrustedProxies) == 0 {
			problems = append(problems, "TRUSTED_PROXIES не задан (адрес или подсеть Caddy)")
		}
	default:
		problems = append(problems, fmt.Sprintf("APP_ENV=%q: ожидается development или production", c.AppEnv))
	}
	if c.PublicOrigin != "" {
		if o, err := origin(c.PublicOrigin); err != nil {
			problems = append(problems, err.Error())
		} else {
			c.PublicOrigin = o
		}
	}
	if len(problems) > 0 {
		return Config{}, errors.New(strings.Join(problems, "; "))
	}
	return c, nil
}

// Summary — сводка для лога старта, без значений секретов.
func (c Config) Summary() []string {
	return []string{
		fmt.Sprintf("HTTP %s, APP_ENV=%s, origin %s", c.Addr, c.AppEnv, c.PublicOrigin),
		fmt.Sprintf("база: %s", set(c.DatabaseURL)),
		fmt.Sprintf("доверенные прокси (X-Real-IP): %s", proxies(c.TrustedProxies)),
	}
}

// origin — схема и хост без пути: «https://example.ru/» → «https://example.ru».
func origin(raw string) (string, error) {
	u, err := url.Parse(raw)
	if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || (u.Path != "" && u.Path != "/") {
		return "", fmt.Errorf("PUBLIC_ORIGIN=%q: ожидается схема и хост, например https://example.ru", raw)
	}
	return u.Scheme + "://" + u.Host, nil
}

func addr(p string) string {
	p = strings.TrimSpace(p)
	switch {
	case p == "":
		return defaultAddr
	case strings.Contains(p, ":"):
		return p
	}
	return "127.0.0.1:" + p
}

func env(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func set(v string) string {
	if v == "" {
		return "не задан"
	}
	return "задан"
}

// list — значения через запятую без пустых.
func list(raw string) []string {
	var out []string
	for _, v := range strings.Split(raw, ",") {
		if v = strings.TrimSpace(v); v != "" {
			out = append(out, v)
		}
	}
	return out
}

// validProxy — IP или CIDR. Fiber на неверный адрес только пишет предупреждение и молча не доверяет
// прокси, поэтому опечатку ловим при старте.
func validProxy(v string) bool {
	if _, err := netip.ParsePrefix(v); err == nil {
		return true
	}
	return net.ParseIP(v) != nil
}

func proxies(p []string) string {
	if len(p) == 0 {
		return "нет — адрес соединения"
	}
	return strings.Join(p, ", ")
}
