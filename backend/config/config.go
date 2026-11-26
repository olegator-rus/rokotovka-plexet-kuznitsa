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
	// OpenSearchURL — OPENSEARCH_URL: адрес OpenSearch. Не задан — в development http://127.0.0.1:9201,
	// в production пусто. Пусто — API стартует, поиск отвечает 503, health — search: down.
	OpenSearchURL string
	// EmbedderURL, RerankerURL — EMBEDDER_URL, RERANKER_URL: TEI-совместимый сервис моделей. Не задано —
	// в development http://127.0.0.1:8089; пусто или off — шаг пропускается (в трассе skipped).
	EmbedderURL string
	RerankerURL string
	// DataDir — DATA_DIR: каталог mock-данных для импорта; по умолчанию ../data/mock (запуск из backend/).
	DataDir string
	// LLM — модель для разбора сложных запросов (LLM_PROVIDER, LLM_BASE_URL, LLM_MODEL, LLM_FOLDER_ID,
	// LLM_API_KEY, LLM_MODE). Пока только читается и проверяется.
	LLM LLMConfig
	// Agent — агент SearchOps (AGENT_*): модель, расписание, пороги допуска (agent.go).
	Agent AgentConfig
	// Ops — администрирование, лимит частоты, дневной бюджет LLM (ops.go).
	Ops OpsConfig
}

func (c Config) IsProduction() bool { return c.AppEnv == EnvProduction }

func Load() (Config, error) {
	c := Config{
		AppEnv:         env("APP_ENV", EnvDevelopment),
		Addr:           addr(os.Getenv("PORT")),
		PublicOrigin:   env("PUBLIC_ORIGIN", ""),
		DatabaseURL:    env("DATABASE_URL", ""),
		TrustedProxies: list(os.Getenv("TRUSTED_PROXIES")),
		DataDir:        env("DATA_DIR", defaultDataDir),
	}
	var problems []string
	dev := c.AppEnv == EnvDevelopment
	c.OpenSearchURL = service("OPENSEARCH_URL", devOpenSearchURL, dev)
	c.EmbedderURL = service("EMBEDDER_URL", devModelsURL, dev)
	c.RerankerURL = service("RERANKER_URL", devModelsURL, dev)
	for _, u := range []struct{ key, val string }{
		{"OPENSEARCH_URL", c.OpenSearchURL}, {"EMBEDDER_URL", c.EmbedderURL}, {"RERANKER_URL", c.RerankerURL},
	} {
		if u.val != "" && !httpURL(u.val) {
			problems = append(problems, fmt.Sprintf("%s=%q: ожидается http(s)-адрес", u.key, u.val))
		}
	}
	llm, err := LoadLLM()
	if err != nil {
		problems = append(problems, err.Error())
	}
	c.LLM = llm
	if c.Agent, err = loadAgent(llm); err != nil {
		problems = append(problems, err.Error())
	}
	if c.Ops, err = loadOps(dev); err != nil {
		problems = append(problems, err.Error())
	}
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
		fmt.Sprintf("OpenSearch: %s", orOff(c.OpenSearchURL)),
		fmt.Sprintf("эмбеддер: %s, реранкер: %s", orOff(c.EmbedderURL), orOff(c.RerankerURL)),
		fmt.Sprintf("данные для импорта: %s", c.DataDir),
		c.LLM.summary(),
		c.Agent.summary(c.LLM),
		c.Ops.summary(),
	}
}

// Адреса по умолчанию для development: OpenSearch из deploy/docker-compose.dev.yml и нативный сервис
// моделей (make ml). В production по умолчанию пусто — адреса задаёт compose стенда.
const (
	devOpenSearchURL = "http://127.0.0.1:9201"
	devModelsURL     = "http://127.0.0.1:8089"
	defaultDataDir   = "../data/mock"
)

// service — адрес внешнего сервиса. Переменная не задана — значение по умолчанию (только в development);
// задана пустой или off — сервис выключен.
func service(key, devDefault string, dev bool) string {
	v, ok := os.LookupEnv(key)
	if !ok {
		if dev {
			return devDefault
		}
		return ""
	}
	v = strings.TrimRight(strings.TrimSpace(v), "/")
	if strings.EqualFold(v, "off") {
		return ""
	}
	return v
}

func httpURL(raw string) bool {
	u, err := url.Parse(raw)
	return err == nil && (u.Scheme == "http" || u.Scheme == "https") && u.Host != ""
}

func orOff(v string) string {
	if v == "" {
		return "выключен"
	}
	return v
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
