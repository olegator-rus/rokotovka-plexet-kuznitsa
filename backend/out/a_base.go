// Package out — DTO ответов API.
package out

import "kuznica/api/pkg/errs"

// ErrorBody — конверт любой ошибки API: {"error":{"code","message","traceId"}} (AGENTS.md).
// Фронт разбирает его в одном месте — форма меняется только вместе с контрактом.
type ErrorBody struct {
	Error ErrorInfo `json:"error"`
}

type ErrorInfo struct {
	Code    string `json:"code"`
	Message string `json:"message"`
	TraceID string `json:"traceId"`
	// Details — пополевые ошибки валидации; нет — поля нет.
	Details []errs.FieldError `json:"details,omitempty"`
	// Caller — место создания ошибки, только при APP_ENV=development.
	Caller string `json:"caller,omitempty"`
}

// Health — GET /health и GET /api/health. status — живость процесса (всегда ok, раз ответил);
// db — готовность базы (ok | down), без причин отказа; time — время ответа сервера (RFC 3339).
type Health struct {
	Status string `json:"status"`
	DB     string `json:"db"`
	Time   string `json:"time"`
}

// OkDown — ok | down для полей готовности.
func OkDown(ok bool) string {
	if ok {
		return "ok"
	}
	return "down"
}
