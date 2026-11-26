package in

// MarginSimulateRequest — POST /api/margin/simulate: набор (пусто — tune) и положения рычага (пусто — все).
type MarginSimulateRequest struct {
	Set       string    `json:"set"`
	Positions []float64 `json:"positions"`
}

// MarginPolicyRequest — POST /api/margin/policy: применить положение рычага новой версией.
type MarginPolicyRequest struct {
	Weight  *float64 `json:"weight"`
	Comment string   `json:"comment"`
}

// MarginRollbackRequest — POST /api/margin/policy/rollback: новая версия = копия выбранной (0 — исходная 0.05).
type MarginRollbackRequest struct {
	ToVersion *int   `json:"to_version"`
	Comment   string `json:"comment"`
}
