package config

import (
	"fmt"
	"strconv"
	"strings"
	"time"
)

// AgentConfig — агент SearchOps (AGENT_*): модель, расписание и пороги допуска правки.
type AgentConfig struct {
	// Model — модель агента (AGENT_MODEL, по умолчанию aliceai-llm-flash: нативные tool_calls), полный URI.
	Model string
	// Schedule — запуск по расписанию (AGENT_SCHEDULE=on), по умолчанию выключен; Interval — AGENT_SCHEDULE_INTERVAL.
	Schedule bool
	Interval time.Duration
	// MaxRegressionWorse — сколько запросов регрессионного набора может ухудшиться (AGENT_MAX_REGRESSION_WORSE, 0).
	MaxRegressionWorse int
	// RegressionSets — наборы для проверки регрессии (AGENT_REGRESSION_SETS, regression,tune); test не трогается.
	RegressionSets []string
}

// loadAgent — настройки агента; модель дополняется каталогом как у LLM.
func loadAgent(l LLMConfig) (AgentConfig, error) {
	a := AgentConfig{
		Model:          l.modelURI(env("AGENT_MODEL", defaultLLMModel)),
		Interval:       24 * time.Hour,
		RegressionSets: []string{"regression", "tune"},
	}
	switch strings.ToLower(env("AGENT_SCHEDULE", "off")) {
	case "on", "true", "1":
		a.Schedule = true
	case "off", "false", "0":
	default:
		return a, fmt.Errorf("AGENT_SCHEDULE: ожидается on или off")
	}
	if v := env("AGENT_SCHEDULE_INTERVAL", ""); v != "" {
		d, err := time.ParseDuration(v)
		if err != nil || d < time.Minute {
			return a, fmt.Errorf("AGENT_SCHEDULE_INTERVAL=%q: ожидается длительность не меньше 1m, например 24h", v)
		}
		a.Interval = d
	}
	if v := env("AGENT_MAX_REGRESSION_WORSE", ""); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 0 {
			return a, fmt.Errorf("AGENT_MAX_REGRESSION_WORSE=%q: ожидается неотрицательное число", v)
		}
		a.MaxRegressionWorse = n
	}
	if v := env("AGENT_REGRESSION_SETS", ""); v != "" {
		a.RegressionSets = nil
		for _, s := range strings.Split(v, ",") {
			s = strings.TrimSpace(strings.ToLower(s))
			if s == "test" {
				return a, fmt.Errorf("AGENT_REGRESSION_SETS: набор test замороженный, на нём агент не проверяет правки")
			}
			if s != "" {
				a.RegressionSets = append(a.RegressionSets, s)
			}
		}
	}
	return a, nil
}

// summary — строка сводки агента.
func (a AgentConfig) summary(l LLMConfig) string {
	sched := "выключено"
	if a.Schedule {
		sched = "каждые " + a.Interval.String()
	}
	return fmt.Sprintf("агент: модель %s, расписание %s, регрессия %s (ухудшений допустимо %d)",
		l.ShortModel(a.Model), sched, strings.Join(a.RegressionSets, "+"), a.MaxRegressionWorse)
}
