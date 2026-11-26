package services

import (
	"encoding/json"
	"errors"
	"strings"
	"testing"

	"kuznica/api/in"
	"kuznica/api/pkg/errs"
)

func TestRecordEventValidation(t *testing.T) {
	svc := NewServices(nil, Options{})
	pos := -1
	for name, req := range map[string]in.EventRequest{
		"тип":      {Type: "search"},
		"позиция":  {Type: "click", Position: &pos},
		"payload":  {Type: "click", Payload: json.RawMessage(`[1,2]`)},
		"длина id": {Type: "click", ProductID: strings.Repeat("x", maxIDLen+1)},
	} {
		var e *errs.Error
		if err := svc.RecordEvent(t.Context(), req); !errors.As(err, &e) || e.Type != errs.ErrTypeValidation {
			t.Fatalf("%s: %v", name, err)
		}
	}
	// Верное событие без базы — 503: записать некуда.
	var e *errs.Error
	err := svc.RecordEvent(t.Context(), in.EventRequest{Type: "click", ProductID: "kis-2dev-1y", Payload: json.RawMessage(`{"a":1}`)})
	if !errors.As(err, &e) || e.Type != errs.ErrTypeUnavailable {
		t.Fatalf("без базы: %v", err)
	}
}

// Подсказка клиента: сегмент и последняя покупка (название семейства и дата).
func TestMetaClientHints(t *testing.T) {
	env := imported(t, false)
	hints := map[string]string{}
	var order []string
	for _, c := range env.svc.Meta().Clients {
		hints[c.ID] = c.Hint
		order = append(order, c.ID)
	}
	if strings.Join(order, ",") != "guest,client-a,client-b" {
		t.Fatalf("порядок клиентов: %v", order)
	}
	if hints["client-a"] != "Малый бизнес · покупок: 1, последняя — Kaspersky Small Office Security (22.05.2025)" ||
		hints["guest"] != "Частное лицо · без истории покупок" {
		t.Fatalf("подсказки: %v", hints)
	}
	m := NewServices(nil, Options{}).Meta()
	if m.CatalogVersion != "" || m.Clients == nil || len(m.Configs) != 5 || m.DefaultConfig != "B3" || m.LLMMode != "off" {
		t.Fatalf("meta без каталога: %+v", m)
	}
}
