package config

import (
	"fmt"
	"strconv"
	"strings"
)

// OpsConfig — защита публичного стенда (ARCHITECTURE §8): администрирование, лимит частоты, дневной бюджет LLM.
type OpsConfig struct {
	// AdminEnabled — ADMIN_ENABLED: служебные методы /api/admin/* (переключение LLM_MODE без перезапуска).
	// По умолчанию включено в development и выключено в production.
	AdminEnabled bool
	// RatePerMinute — RATE_LIMIT_PER_MIN: запросов к /api в минуту с одного адреса; 0 — без лимита.
	RatePerMinute int
	// DailyBudgetRub — LLM_DAILY_BUDGET_RUB: потолок расхода на модель за сутки (по Москве), ₽; 0 — без потолка.
	// Сверх него маршрут D работает на правилах и кэше, агент не запускается.
	DailyBudgetRub float64
}

// Значения по умолчанию: лимит частоты с запасом на подсказки при наборе и e2e-прогоны; бюджет — порядок
// суток разработки с прогонами судьи и агента.
const (
	defaultRatePerMinute  = 600
	defaultDailyBudgetRub = 1000
)

// loadOps — настройки защиты стенда.
func loadOps(dev bool) (OpsConfig, error) {
	o := OpsConfig{AdminEnabled: dev, RatePerMinute: defaultRatePerMinute, DailyBudgetRub: defaultDailyBudgetRub}
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
	return fmt.Sprintf("защита: администрирование %s, частота %s, бюджет LLM %s", admin, rate, budget)
}
