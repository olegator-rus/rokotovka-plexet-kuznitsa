package config

import (
	"fmt"
	"strconv"
	"strings"
)

// OpsConfig — защита публичного стенда (docs/ARCHITECTURE.md, «Модель угроз»): администрирование, лимит частоты, дневной бюджет LLM.
type OpsConfig struct {
	// AdminEnabled — ADMIN_ENABLED: служебные методы /api/admin/* (переключение LLM_MODE без перезапуска).
	// По умолчанию включено в development и выключено в production.
	AdminEnabled bool
	// RatePerMinute — RATE_LIMIT_PER_MIN: запросов к /api в минуту с одного адреса; 0 — без лимита.
	RatePerMinute int
	// DailyBudgetRub — LLM_DAILY_BUDGET_RUB: потолок расхода на модель за сутки (по Москве), ₽; 0 — без потолка.
	// Сверх него маршрут D работает на правилах и кэше, агент не запускается.
	DailyBudgetRub float64
	// AgentReserveRub — AGENT_BUDGET_RESERVE_RUB: часть дневного бюджета, которую разборы покупателей не тратят —
	// она остаётся агенту: маршрут D уходит на правила при расходе ≥ бюджет − резерв, агент — при расходе ≥ бюджет.
	AgentReserveRub float64
	// PerIPDailyRub — LLM_PER_IP_DAILY_RUB: потолок расхода на разборы маршрута D (промахи кэша) с одного адреса
	// за сутки, ₽; сверх — правила и кэш с причиной ip_quota. 0 — без квоты.
	PerIPDailyRub float64
	// BackofficeToken — BACKOFFICE_TOKEN: токен бэк-офиса. Задан — изменяющие методы (кроме поиска и событий) и
	// чтение аналитики, трасс, журналов агента и экономики требуют Authorization: Bearer <токен> или basic-auth с
	// паролем = токен (так его шлёт браузер после входа на Caddy). Пусто — без входа (только development).
	BackofficeToken string
}

// Значения по умолчанию: лимит частоты с запасом на подсказки при наборе и e2e-прогоны; бюджет — порядок
// суток разработки с прогонами судьи и агента.
const (
	defaultRatePerMinute   = 600
	defaultDailyBudgetRub  = 1000
	defaultAgentReserveRub = 100
	defaultPerIPDailyRub   = 20
	// minBackofficeToken — короче токен перебирается; openssl rand -hex 24 даёт 48 символов.
	minBackofficeToken = 24
)

// loadOps — настройки защиты стенда.
func loadOps(dev bool) (OpsConfig, error) {
	o := OpsConfig{AdminEnabled: dev, RatePerMinute: defaultRatePerMinute, DailyBudgetRub: defaultDailyBudgetRub,
		AgentReserveRub: defaultAgentReserveRub, PerIPDailyRub: defaultPerIPDailyRub,
		BackofficeToken: env("BACKOFFICE_TOKEN", "")}
	var problems []string
	if v := env("ADMIN_ENABLED", ""); v != "" {
		switch strings.ToLower(v) {
		case "on", "true", "1":
			o.AdminEnabled = true
		case "off", "false", "0":
			o.AdminEnabled = false
		default:
			problems = append(problems, fmt.Sprintf("ADMIN_ENABLED=%q: ожидается on или off", v))
		}
	}
	if v := env("RATE_LIMIT_PER_MIN", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			problems = append(problems, fmt.Sprintf("RATE_LIMIT_PER_MIN=%q: ожидается неотрицательное число", v))
		}
		o.RatePerMinute = n
	}
	if v := env("LLM_DAILY_BUDGET_RUB", ""); v != "" {
		f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
		if err != nil || f < 0 {
			problems = append(problems, fmt.Sprintf("LLM_DAILY_BUDGET_RUB=%q: ожидается неотрицательное число рублей", v))
		}
		o.DailyBudgetRub = f
	}
	rub := func(key string, dst *float64) {
		if v := env(key, ""); v != "" {
			f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
			if err != nil || f < 0 {
				problems = append(problems, fmt.Sprintf("%s=%q: ожидается неотрицательное число рублей", key, v))
			}
			*dst = f
		}
	}
	rub("AGENT_BUDGET_RESERVE_RUB", &o.AgentReserveRub)
	rub("LLM_PER_IP_DAILY_RUB", &o.PerIPDailyRub)
	if o.DailyBudgetRub > 0 && o.AgentReserveRub >= o.DailyBudgetRub {
		problems = append(problems, fmt.Sprintf("AGENT_BUDGET_RESERVE_RUB=%.0f: резерв агента должен быть меньше LLM_DAILY_BUDGET_RUB=%.0f",
			o.AgentReserveRub, o.DailyBudgetRub))
	}
	switch {
	case o.BackofficeToken == "" && !dev:
		problems = append(problems, "BACKOFFICE_TOKEN не задан: в production бэк-офис без входа не запускается")
	case o.BackofficeToken != "" && len(o.BackofficeToken) < minBackofficeToken:
		problems = append(problems, fmt.Sprintf("BACKOFFICE_TOKEN короче %d символов", minBackofficeToken))
	}
	if len(problems) > 0 {
		return o, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return o, nil
}

// summary — строка сводки защиты стенда.
func (o OpsConfig) summary() string {
	admin := "выключено"
	if o.AdminEnabled {
		admin = "включено"
	}
	rate := "без лимита"
	if o.RatePerMinute > 0 {
		rate = fmt.Sprintf("%d/мин с адреса", o.RatePerMinute)
	}
	budget := "без потолка"
	if o.DailyBudgetRub > 0 {
		budget = fmt.Sprintf("%.0f ₽/сутки", o.DailyBudgetRub)
	}
	if o.DailyBudgetRub > 0 {
		budget += fmt.Sprintf(" (резерв агента %.0f ₽)", o.AgentReserveRub)
	}
	quota := "без квоты"
	if o.PerIPDailyRub > 0 {
		quota = fmt.Sprintf("%.0f ₽/сутки", o.PerIPDailyRub)
	}
	backoffice := "без входа"
	if o.BackofficeToken != "" {
		backoffice = "по токену"
	}
	return fmt.Sprintf("защита: администрирование %s, частота %s, бюджет LLM %s, квота LLM на адрес %s, бэк-офис %s",
		admin, rate, budget, quota, backoffice)
}
