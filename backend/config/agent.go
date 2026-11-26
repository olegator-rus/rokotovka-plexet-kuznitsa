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

	// Контур автономии (автономность). ScheduleAt — время ночного разбора по Москве (AGENT_SCHEDULE_AT, «03:00»);
	// пусто — расписание по интервалу, как раньше. ScheduleDays — за сколько суток ночной разбор (AGENT_SCHEDULE_DAYS, 1).
	ScheduleAt   string
	ScheduleDays int
	// Spike — триггер «всплеск» (AGENT_SPIKE=on|off; по умолчанию — как AGENT_SCHEDULE) и его пороги AGENT_SPIKE_*.
	Spike SpikeConfig
	// CatalogTrigger — триггер «изменение каталога» (AGENT_CATALOG_TRIGGER=on|off; по умолчанию — как AGENT_SCHEDULE).
	CatalogTrigger bool
	// TriggerCheck — как часто проверять триггеры (AGENT_TRIGGER_CHECK, 1m).
	TriggerCheck time.Duration
	// ReadOnly — режим «только чтение» (AGENT_READ_ONLY): диагноз и черновик предложения без записи в очередь.
	ReadOnly bool
	// MaxParallel — запусков одновременно (AGENT_MAX_PARALLEL, 2): отдельный пул агента.
	MaxParallel int
	// Budget — бюджет запуска по периоду (AGENT_RUN_MAX_STEPS/TOKENS/RUB/DURATION); QueryDuration — срок запуска на
	// одном запросе (AGENT_QUERY_MAX_DURATION, 3m).
	Budget        BudgetConfig
	QueryDuration time.Duration
}

// SpikeConfig — пороги триггера «всплеск».
type SpikeConfig struct {
	Enabled     bool
	Window      time.Duration
	MinSearches int
	EmptyShare  float64
	ReformShare float64
	QueryFreq   int
	Cooldown    time.Duration
}

// BudgetConfig — лимиты одного запуска.
type BudgetConfig struct {
	Steps    int
	Tokens   int
	CostRub  float64
	Duration time.Duration
}

// loadAgent — настройки агента; модель дополняется каталогом как у LLM.
func loadAgent(l LLMConfig) (AgentConfig, error) {
	a := AgentConfig{
		Model:          l.modelURI(env("AGENT_MODEL", defaultLLMModel)),
		Interval:       24 * time.Hour,
		RegressionSets: []string{"regression", "tune"},
		ScheduleDays:   1,
		Spike: SpikeConfig{Window: time.Hour, MinSearches: 60, EmptyShare: 0.25, ReformShare: 0.12, QueryFreq: 8,
			Cooldown: 3 * time.Hour},
		TriggerCheck:  time.Minute,
		MaxParallel:   2,
		Budget:        BudgetConfig{Steps: 80, Tokens: 600_000, CostRub: 40, Duration: 25 * time.Minute},
		QueryDuration: 3 * time.Minute,
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
	if err := loadAgentP5(&a); err != nil {
		return a, err
	}
	return a, nil
}

// loadAgentP5 — триггеры, режим, пул и бюджет (контур автономии).
func loadAgentP5(a *AgentConfig) error {
	var errsList []string
	onOff := func(key string, def bool) bool {
		switch strings.ToLower(env(key, "")) {
		case "":
			return def
		case "on", "true", "1":
			return true
		case "off", "false", "0":
			return false
		}
		errsList = append(errsList, key+": ожидается on или off")
		return def
	}
	dur := func(key string, dst *time.Duration, minD time.Duration) {
		if v := env(key, ""); v != "" {
			d, err := time.ParseDuration(v)
			if err != nil || d < minD {
				errsList = append(errsList, fmt.Sprintf("%s=%q: ожидается длительность не меньше %s", key, v, minD))
				return
			}
			*dst = d
		}
	}
	intv := func(key string, dst *int, minN int) {
		if v := env(key, ""); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < minN {
				errsList = append(errsList, fmt.Sprintf("%s=%q: ожидается целое не меньше %d", key, v, minN))
				return
			}
			*dst = n
		}
	}
	share := func(key string, dst *float64) {
		if v := env(key, ""); v != "" {
			f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
			if err != nil || f <= 0 || f > 1 {
				errsList = append(errsList, fmt.Sprintf("%s=%q: ожидается доля от 0 до 1", key, v))
				return
			}
			*dst = f
		}
	}
	if v := env("AGENT_SCHEDULE_AT", ""); v != "" {
		t, err := time.Parse("15:04", v)
		if err != nil {
			errsList = append(errsList, fmt.Sprintf("AGENT_SCHEDULE_AT=%q: ожидается ЧЧ:ММ по Москве, например 03:00", v))
		} else {
			a.ScheduleAt = t.Format("15:04")
		}
	}
	intv("AGENT_SCHEDULE_DAYS", &a.ScheduleDays, 1)
	a.Spike.Enabled = onOff("AGENT_SPIKE", a.Schedule)
	dur("AGENT_SPIKE_WINDOW", &a.Spike.Window, 5*time.Minute)
	intv("AGENT_SPIKE_MIN_SEARCHES", &a.Spike.MinSearches, 1)
	share("AGENT_SPIKE_EMPTY_SHARE", &a.Spike.EmptyShare)
	share("AGENT_SPIKE_REFORM_SHARE", &a.Spike.ReformShare)
	intv("AGENT_SPIKE_QUERY_FREQ", &a.Spike.QueryFreq, 1)
	dur("AGENT_SPIKE_COOLDOWN", &a.Spike.Cooldown, time.Minute)
	a.CatalogTrigger = onOff("AGENT_CATALOG_TRIGGER", a.Schedule)
	dur("AGENT_TRIGGER_CHECK", &a.TriggerCheck, 10*time.Second)
	a.ReadOnly = onOff("AGENT_READ_ONLY", false)
	intv("AGENT_MAX_PARALLEL", &a.MaxParallel, 1)
	intv("AGENT_RUN_MAX_STEPS", &a.Budget.Steps, 1)
	intv("AGENT_RUN_MAX_TOKENS", &a.Budget.Tokens, 1000)
	if v := env("AGENT_RUN_MAX_RUB", ""); v != "" {
		f, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
		if err != nil || f <= 0 {
			errsList = append(errsList, fmt.Sprintf("AGENT_RUN_MAX_RUB=%q: ожидается положительное число", v))
		} else {
			a.Budget.CostRub = f
		}
	}
	dur("AGENT_RUN_MAX_DURATION", &a.Budget.Duration, time.Minute)
	dur("AGENT_QUERY_MAX_DURATION", &a.QueryDuration, 30*time.Second)
	if len(errsList) > 0 {
		return fmt.Errorf("%s", strings.Join(errsList, "; "))
	}
	return nil
}

// summary — строка сводки агента.
func (a AgentConfig) summary(l LLMConfig) string {
	sched := "выключено"
	if a.Schedule {
		sched = "каждые " + a.Interval.String()
		if a.ScheduleAt != "" {
			sched = "ежедневно в " + a.ScheduleAt + " МСК"
		}
	}
	if a.Spike.Enabled {
		sched += fmt.Sprintf(", всплеск: окно %s, пустых > %.0f%%, переформулировок > %.0f%%, новый запрос ≥ %d",
			a.Spike.Window, a.Spike.EmptyShare*100, a.Spike.ReformShare*100, a.Spike.QueryFreq)
	}
	if a.CatalogTrigger {
		sched += ", изменение каталога"
	}
	if a.ReadOnly {
		sched += ", только чтение"
	}
	return fmt.Sprintf("агент: модель %s, расписание %s, регрессия %s (ухудшений допустимо %d)",
		l.ShortModel(a.Model), sched, strings.Join(a.RegressionSets, "+"), a.MaxRegressionWorse)
}
