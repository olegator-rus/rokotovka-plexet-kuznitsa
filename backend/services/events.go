package services

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/pkg/errs"
)

// События поведения покупателя (POST /api/events): клик, корзина, заказ, запрос цены и т. п. Событие
// search пишет сам поиск.

const (
	maxPayload  = 4 << 10
	maxPosition = 10000
)

// RecordEvent — проверить и записать событие. Без базы — 503: событие некуда положить.
func (s *Services) RecordEvent(ctx context.Context, req in.EventRequest) error {
	var bad []errs.FieldError
	if !slices.Contains(domain.BehaviorEventTypes, req.Type) {
		bad = append(bad, errs.FieldError{Field: "type", Code: errs.CodeInvalidFormat, Message: "Неизвестный тип события"})
	}
	for field, v := range map[string]string{"request_id": req.RequestID, "session_id": req.SessionID,
		"client_id": req.ClientID, "product_id": req.ProductID} {
		if len(v) > maxIDLen {
			bad = append(bad, errs.FieldError{Field: field, Code: errs.CodeOutOfRange, Message: "Слишком длинный идентификатор"})
		}
	}
	if req.Position != nil && (*req.Position < 0 || *req.Position > maxPosition) {
		bad = append(bad, errs.FieldError{Field: "position", Code: errs.CodeOutOfRange, Message: "Позиция вне диапазона"})
	}
	payload := bytes.TrimSpace(req.Payload)
	if bytes.Equal(payload, []byte("null")) {
		payload = nil
	}
	if len(payload) > 0 {
		var obj map[string]any
		switch {
		case len(payload) > maxPayload:
			bad = append(bad, errs.FieldError{Field: "payload", Code: errs.CodeOutOfRange, Message: "payload больше 4 КБ"})
		case json.Unmarshal(payload, &obj) != nil:
			bad = append(bad, errs.FieldError{Field: "payload", Code: errs.CodeInvalidFormat, Message: "payload — JSON-объект"})
		}
	}
	if len(bad) > 0 {
		return errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	if !s.repos.Enabled() {
		return errs.NewUnavailableError("Сервис запущен без базы данных: событие не записано")
	}
	ev := &domain.BehaviorEvent{
		Type: req.Type, RequestID: optional(req.RequestID), SessionID: optional(req.SessionID),
		ClientID: optional(req.ClientID), ProductID: optional(req.ProductID), Position: req.Position,
	}
	if len(payload) > 0 {
		ev.Payload = domain.JSONB(payload)
	}
	return s.repos.SaveBehaviorEvent(ctx, ev)
}
