package in

// MarginSimulateRequest — POST /api/margin/simulate: набор (пусто — tune) и положения рычага (пусто — все).
type MarginSimulateRequest struct {
	Set       string    `json:"set"`
	Positions []float64 `json:"positions"`
	// Category — положения рычага только для позиций этой категории, остальные — по действующей политике
	// (так воспроизводится предложение MARGIN_OPPORTUNITY); пусто — общий вес.
	Category string `json:"category"`
}

// MarginPolicyRequest — POST /api/margin/policy: применить положение рычага новой версией.
type MarginPolicyRequest struct {
	// Weight — общий вес; nil — прежний (тогда нужен Categories).
	Weight *float64 `json:"weight"`
	// Categories — веса по категориям; nil — прежние, {} — снять все.
	Categories map[string]float64 `json:"categories"`
	Comment    string             `json:"comment"`
}

// MarginRollbackRequest — POST /api/margin/policy/rollback: новая версия = копия выбранной (0 — исходная 0.05).
type MarginRollbackRequest struct {
	ToVersion *int   `json:"to_version"`
	Comment   string `json:"comment"`
}
