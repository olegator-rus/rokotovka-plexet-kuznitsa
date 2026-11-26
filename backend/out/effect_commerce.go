package out

import "time"

// EffectCommerce — GET /api/effect/commerce: модель эффекта контура коммерции. Модель
// чувствительности, не прогноз: три непересекающихся механизма и вычеты, тождество ΔGP = ΔO·G₀ + O₀·ΔG + ΔO·ΔG.
type EffectCommerce struct {
	Days  int       `json:"days"`
	From  time.Time `json:"from"`
	To    time.Time `json:"to"`
	Title string    `json:"title"`
	Note  string    `json:"note"`
	// Synthetic — во входах есть синтетические данные (маржа, заказы); SyntheticKeys — какие входы.
	Synthetic     bool     `json:"synthetic"`
	SyntheticKeys []string `json:"synthetic_keys"`
	Formula       []string `json:"formula"`
	Identity      string   `json:"identity"`
	// Inputs — входы модели: у каждого источник.
	Inputs    []EffectInput            `json:"inputs"`
	Scenarios []EffectCommerceScenario `json:"scenarios"`
	// Pilot — что пилот различит за 4–8 недель.
	Pilot    string   `json:"pilot"`
	Missing  []string `json:"missing"`
	Warnings []string `json:"warnings"`
}

// EffectInput — вход модели.
type EffectInput struct {
	Key   string   `json:"key"`
	Label string   `json:"label"`
	Value *float64 `json:"value"` // nil — не посчитано (причина в missing)
	// Default — значение без параметра запроса (замер или допущение).
	Default *float64 `json:"default"`
	Unit    string   `json:"unit"`
	// Source — measured (замер стенда) | synthetic (синтетические данные mock) | assumed (допущение) |
	// literature (литература, ссылка в source_url) | request (задано в запросе).
	Source    string `json:"source"`
	SourceRef string `json:"source_ref"`
	SourceURL string `json:"source_url,omitempty"`
	// Scenarios — множитель или значение по сценариям, если вход между ними меняется.
	Scenarios map[string]float64 `json:"scenarios,omitempty"`
	Hint      string             `json:"hint"`
}

// EffectTerm — член разложения ΔGP: рубли валовой прибыли в месяц; nil — не посчитан.
type EffectTerm struct {
	Key     string   `json:"key"` // rescued | upsell | mix | anchor_loss | returns
	Label   string   `json:"label"`
	Kind    string   `json:"kind"` // mechanism | deduction
	GPRub   *float64 `json:"gp_rub_month"`
	Orders  *float64 `json:"orders_month,omitempty"`
	Formula string   `json:"formula"`
}

// EffectIdentity — проверка тождества ΔGP = ΔO·G₀ + O₀·ΔG + ΔO·ΔG.
type EffectIdentity struct {
	O0      float64 `json:"o0"`
	G0      float64 `json:"g0"`
	DeltaO  float64 `json:"delta_o"`
	DeltaG  float64 `json:"delta_g"`
	DOxG0   float64 `json:"do_g0"`
	O0xDG   float64 `json:"o0_dg"`
	DOxDG   float64 `json:"do_dg"`
	Sum     float64 `json:"sum"`
	Direct  float64 `json:"direct"` // (O₀+ΔO)(G₀+ΔG) − O₀G₀
	Terms   float64 `json:"terms"`  // сумма членов разложения
	OK      bool    `json:"ok"`
	Epsilon float64 `json:"epsilon"`
}

// EffectCommerceScenario — сценарий.
type EffectCommerceScenario struct {
	ID    string       `json:"id"`
	Label string       `json:"label"`
	Rule  string       `json:"rule"`
	Terms []EffectTerm `json:"terms"`
	// DeltaGPRub — сумма посчитанных членов; Complete — все члены посчитаны.
	DeltaGPRub float64         `json:"delta_gp_rub_month"`
	Complete   bool            `json:"complete"`
	Check      *EffectIdentity `json:"check"`
	// Uplift — ΔGP к текущей валовой прибыли поиска O₀·G₀; nil — её нет.
	Uplift *float64 `json:"uplift"`
}
