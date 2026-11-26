package in

// EvalRunRequest — POST /api/evaluations/run: набор и конфигурации (пусто — test и B0–B3).
type EvalRunRequest struct {
	Set     string   `json:"set"`
	Configs []string `json:"configs"`
}
