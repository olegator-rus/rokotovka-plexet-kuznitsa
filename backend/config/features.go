package config

import (
	"fmt"
	"strings"
)

// Features — флаги контура коммерции. Флаг выключен — ответ и выдача
// идентичны контуру поиска. Значения: on | off (true/false, 1/0); не задано — по умолчанию.
type Features struct {
	// Hardware — FEATURE_HARDWARE: парт-номера, числовые характеристики, совместимость (по умолчанию on).
	Hardware bool
	// Companions — FEATURE_COMPANIONS: блок «Дополните решение» и условно обязательное (on).
	Companions bool
	// MarginPolicy — FEATURE_MARGIN_POLICY: политика маржи в ранжировании (on).
	MarginPolicy bool
	// Solutions — FEATURE_SOLUTIONS: закупка под задачу (шаблоны) и старшая редакция (on).
	Solutions bool
	// ResultsV3 — FEATURE_RESULTS_V3: страница результатов контура выдачи — лицензионные фасеты, сортировки,
	// группировка, уточнения (по умолчанию on).
	ResultsV3 bool
	// Панель поиска контура выдачи: FEATURE_PANEL, FEATURE_PANEL_PRODUCTS, FEATURE_PANEL_MARGIN,
	// FEATURE_PANEL_RECS (on); FEATURE_PANEL_VIEWED — основание «недавно просмотренное» в рекомендациях (off).
	Panel, PanelProducts, PanelMargin, PanelRecs, PanelViewed bool
}

// loadFeatures — флаги из окружения.
func loadFeatures() (Features, error) {
	f := Features{Hardware: true, Companions: true, MarginPolicy: true, Solutions: true, ResultsV3: true,
		Panel: true, PanelProducts: true, PanelMargin: true, PanelRecs: true}
	var problems []string
	for _, x := range []struct {
		key string
		dst *bool
	}{
		{"FEATURE_HARDWARE", &f.Hardware}, {"FEATURE_COMPANIONS", &f.Companions},
		{"FEATURE_MARGIN_POLICY", &f.MarginPolicy}, {"FEATURE_SOLUTIONS", &f.Solutions},
		{"FEATURE_RESULTS_V3", &f.ResultsV3},
		{"FEATURE_PANEL", &f.Panel}, {"FEATURE_PANEL_PRODUCTS", &f.PanelProducts},
		{"FEATURE_PANEL_MARGIN", &f.PanelMargin}, {"FEATURE_PANEL_RECS", &f.PanelRecs},
		{"FEATURE_PANEL_VIEWED", &f.PanelViewed},
	} {
		v := env(x.key, "")
		switch strings.ToLower(v) {
		case "":
		case "on", "true", "1":
			*x.dst = true
		case "off", "false", "0":
			*x.dst = false
		default:
			problems = append(problems, fmt.Sprintf("%s=%q: ожидается on или off", x.key, v))
		}
	}
	if len(problems) > 0 {
		return Features{}, fmt.Errorf("%s", strings.Join(problems, "; "))
	}
	return f, nil
}

func (f Features) summary() string {
	onOff := func(b bool) string {
		if b {
			return "on"
		}
		return "off"
	}
	return fmt.Sprintf("контур коммерции: оборудование %s, допродажа %s, политика маржи %s, решения %s; страница результатов п3 %s; панель п3 %s (товары %s, маржа %s, рекомендации %s, просмотренное %s)",
		onOff(f.Hardware), onOff(f.Companions), onOff(f.MarginPolicy), onOff(f.Solutions), onOff(f.ResultsV3),
		onOff(f.Panel), onOff(f.PanelProducts), onOff(f.PanelMargin), onOff(f.PanelRecs), onOff(f.PanelViewed))
}
