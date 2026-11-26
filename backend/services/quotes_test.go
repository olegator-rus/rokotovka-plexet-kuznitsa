package services

import (
	"errors"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/pkg/errs"
)

// Сумма — только если известны все цены; иначе null и число позиций «по запросу».
func TestQuoteTotals(t *testing.T) {
	c := newCommerceCatalog(p2Snapshot(t))
	total, on := quoteTotals(c.products, []domain.QuoteItem{{ProductID: "ws2025-std-16core", Quantity: 1},
		{ProductID: "ws2025-cal-user", Quantity: 50}})
	if total == nil || *total != 120000+4500*50 || on != 0 {
		t.Fatalf("все цены известны: %v %d", total, on)
	}
	total, on = quoteTotals(c.products, []domain.QuoteItem{{ProductID: "ws2025-std-16core", Quantity: 1},
		{ProductID: "ram-32g-rdimm", Quantity: 4}, {ProductID: "photoshop-1u-1y", Quantity: 2}})
	if total != nil || on != 2 {
		t.Fatalf("цены по запросу: %v %d", total, on)
	}
}

func TestCreateQuoteValidation(t *testing.T) {
	svc := importedWith(t, Features{Companions: true})
	var e *errs.Error
	for name, req := range map[string]in.QuoteRequest{
		"пусто":          {},
		"нет в каталоге": {Items: []in.QuoteItem{{ProductID: "nope", Quantity: 1, Origin: "search"}}},
		"количество":     {Items: []in.QuoteItem{{ProductID: "ws2025-cal-user", Quantity: 0, Origin: "search"}}},
		"происхождение":  {Items: []in.QuoteItem{{ProductID: "ws2025-cal-user", Quantity: 1, Origin: "x"}}},
	} {
		if _, err := svc.CreateQuote(t.Context(), req); !errors.As(err, &e) || e.Type != errs.ErrTypeValidation {
			t.Errorf("%s: %v", name, err)
		}
	}
	// Верная спецификация без базы — 503, не 500.
	_, err := svc.CreateQuote(t.Context(), in.QuoteRequest{Items: []in.QuoteItem{{ProductID: "ws2025-cal-user", Quantity: 50,
		Origin: "companion", CompanionClass: "conditional_required"}}})
	if !errors.As(err, &e) || e.Type != errs.ErrTypeUnavailable {
		t.Fatalf("без базы: %v", err)
	}
	if res, err := svc.Quotes(t.Context()); err != nil || len(res.Items) != 0 {
		t.Fatalf("список без базы: %v %v", res, err)
	}
}
