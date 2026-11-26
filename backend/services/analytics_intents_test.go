package services

import (
	"encoding/json"
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/pkg/errs"
)

// intentLog — намерения всех исходов: отправка с кликом, подсказка без действия, товар из панели, категория с
// кликом, отказ, открытое; пустая выдача по отправленному и пустая подсказка.
func intentLog(t0 time.Time) ([]domain.BehaviorEvent, map[string]int) {
	var evs []domain.BehaviorEvent
	at := t0
	add := func(intent, typ, src string, payload string) {
		at = at.Add(time.Second)
		e := domain.BehaviorEvent{Type: typ, Source: src}
		if intent != "" {
			e.IntentID = ptr(intent)
		}
		if payload != "" {
			e.Payload = domain.JSONB(payload)
		}
		e.CreatedAt = at
		evs = append(evs, e)
	}
	add("i1", domain.EventSearchOpened, "mock_log", `{"state":"empty"}`)
	add("i1", domain.EventSuggestImpression, "mock_log", `{"prefix_len":3,"items":5}`)
	add("i1", domain.EventSearchCommit, "mock_log", `{"source":"enter","results_count":4}`)
	add("i1", domain.EventClick, "mock_log", "")
	add("i2", domain.EventSuggestImpression, "live", `{"prefix_len":4,"items":0}`)
	add("i2", domain.EventSuggestionSelect, "live", `{"prefix_len":4}`)
	commit := domain.BehaviorEvent{Type: domain.EventSearchCommit, IntentID: ptr("i2"), RequestID: ptr("r-empty"),
		Payload: domain.JSONB(`{"source":"suggestion"}`), Source: "live"}
	at = at.Add(time.Second)
	commit.CreatedAt = at
	evs = append(evs, commit)
	add("i3", domain.EventInstantProductClick, "live", `{"family_id":"f"}`)
	add("i3", domain.EventSearchAbandon, "live", `{"prefix_len":4}`) // после исхода — не меняет его
	add("i4", domain.EventCategorySelect, "live", `{"value":"c"}`)
	add("i4", domain.EventClick, "live", "")
	add("i5", domain.EventSearchAbandon, "live", `{"prefix_len":2}`)
	add("i5", domain.EventSuggestImpression, "live", `{"prefix_len":1,"items":0}`) // 1 символ — не считается
	add("i6", domain.EventSearchOpened, "live", "")
	add("i7", domain.EventSearchCommit, "live", `{"outcome":"vendor","source":"panel"}`) // исход из payload
	add("i8", domain.EventSearchCommit, "live", `{"outcome":"commit"}`)                  // число — по поиску намерения
	add("", domain.EventClick, "live", "")                                               // вне намерения
	return evs, map[string]int{"r-empty": 0}
}

func TestIntentAnalytics(t *testing.T) {
	evs, results := intentLog(time.Date(2026, 5, 22, 10, 0, 0, 0, time.UTC))
	res := intentsFrom(aggregateIntents(evs, results, map[string]int{"i8": 3}))
	if res.Intents != 7 || res.Open != 1 {
		t.Fatalf("намерений %d, открытых %d", res.Intents, res.Open)
	}
	want := map[string][2]int{"commit": {2, 1}, "suggestion": {1, 0}, "product": {1, 1}, "category": {1, 1}, "vendor": {1, 0}, "abandon": {1, 0}}
	for _, o := range res.Outcomes {
		if w := want[o.Outcome]; o.Count != w[0] || o.Converted != w[1] {
			t.Fatalf("исход %s: %d/%d, want %v", o.Outcome, o.Count, o.Converted, w)
		}
	}
	if r := res.ConversionLegacy; r.Num != 1 || r.Den != 3 {
		t.Fatalf("прежняя конверсия %+v", r)
	}
	if r := res.ConversionUnified; r.Num != 3 || r.Den != 7 {
		t.Fatalf("единая конверсия %+v", r)
	}
	if r := res.EmptyResults; r.Num != 1 || r.Den != 3 {
		t.Fatalf("пустые выдачи %+v", r)
	}
	if r := res.EmptySuggestions; r.Num != 1 || r.Den != 2 {
		t.Fatalf("пустые подсказки %+v", r)
	}
	if !reflect.DeepEqual(res.Sources, map[string]int{"live": 7, "mock_log": 1}) {
		t.Fatalf("источники %v", res.Sources)
	}
}

func TestIntentEventValidation(t *testing.T) {
	svc := NewServices(nil, Options{})
	for name, req := range map[string]in.EventRequest{
		"нет intent_id":   {Type: domain.EventSearchOpened},
		"длинный":         {Type: domain.EventSearchOpened, IntentID: strings.Repeat("x", maxIntentIDLen+1)},
		"символы":         {Type: domain.EventSearchAbandon, IntentID: "it 1"},
		"без товара":      {Type: domain.EventInstantProductClick, IntentID: "it-1"},
		"неизвестный тип": {Type: "keypress", IntentID: "it-1"},
	} {
		var e *errs.Error
		if err := svc.RecordEvent(t.Context(), req); !errors.As(err, &e) || e.Type != errs.ErrTypeValidation {
			t.Errorf("%s: ждали отказ проверки, получили %v", name, err)
		}
	}
	// Верное событие контура выдачи без базы — 503: проверку прошло, записать некуда.
	var e *errs.Error
	err := svc.RecordEvent(t.Context(), in.EventRequest{Type: domain.EventSearchCommit, IntentID: "it-1_a",
		Payload: json.RawMessage(`{"source":"enter"}`)})
	if !errors.As(err, &e) || e.Type != errs.ErrTypeUnavailable {
		t.Fatalf("верное событие: %v", err)
	}
	// События фронта панели: intent_id верхнего уровня, поля — в payload.
	for _, req := range []in.EventRequest{
		{Type: domain.EventSearchOpened, IntentID: "it-abc", Payload: json.RawMessage(`{"state":"empty"}`)},
		{Type: domain.EventSuggestImpression, IntentID: "it-abc", Payload: json.RawMessage(
			`{"prefix_len":4,"sections":["suggestions"],"items":5,"server_ms":3.1,"render_ms":12,"partial":false}`)},
		{Type: domain.EventSuggestionSelect, IntentID: "it-abc", Payload: json.RawMessage(`{"position":1,"prefix_len":4,"kind":"query"}`)},
		{Type: domain.EventInstantProductClick, IntentID: "it-abc", ProductID: "kis-2dev-1y",
			Payload: json.RawMessage(`{"family_id":"kis","position":1,"prefix_len":4,"exact":false}`)},
		{Type: domain.EventSearchCommit, IntentID: "it-abc", Payload: json.RawMessage(`{"outcome":"product","prefix_len":4}`)},
		{Type: domain.EventRefinementApply, IntentID: "it-abc", Payload: json.RawMessage(`{"field":"vendor","value":"kaspersky","place":"panel"}`)},
		{Type: domain.EventRecsAdd, IntentID: "it-abc", ProductID: "kis-2dev-1y", Payload: json.RawMessage(`{"basis":"spec","class":"required"}`)},
		{Type: domain.EventHistoryClear, IntentID: "it-abc", Payload: json.RawMessage(`{"value":"all"}`)},
		{Type: domain.EventSearchAbandon, IntentID: "it-abc", Payload: json.RawMessage(`{"prefix_len":2}`)},
	} {
		if err := svc.RecordEvent(t.Context(), req); !errors.As(err, &e) || e.Type != errs.ErrTypeUnavailable {
			t.Fatalf("%s от фронта: %v", req.Type, err)
		}
	}
	if bad := checkIntentID(domain.EventClick, ""); bad != nil {
		t.Fatalf("click без intent_id: %v", bad)
	}
}

// Равенство SQL-агрегации намерений (repositories.IntentsSince) эталону в Go. Нужна живая база:
// KUZNICA_TEST_DATABASE_URL=postgres://… go test ./services -run SQLIntents.
func TestSQLIntentsMatchMemory(t *testing.T) {
	r := openSchema(t, testDatabaseURL(t))
	t0 := time.Now().UTC().Add(-time.Hour).Truncate(time.Second)
	evs, _ := intentLog(t0)
	se := domain.SearchEvent{RequestID: "r-empty", Query: "x", ResultsCount: 0, Source: "live"}
	se.CreatedAt = t0
	si := domain.SearchEvent{RequestID: "r-i8", Query: "y", ResultsCount: 3, Source: "live", IntentID: ptr("i8")}
	si.CreatedAt = t0.Add(time.Minute)
	if err := r.ReplaceMockLog(t.Context(), []domain.SearchEvent{se, si}, evs); err != nil {
		t.Fatal(err)
	}
	got, err := r.IntentsSince(t.Context(), t0.Add(-time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	want := aggregateIntents(evs, map[string]int{"r-empty": 0}, map[string]int{"i8": 3})
	if !reflect.DeepEqual(intentsFrom(got), intentsFrom(want)) {
		t.Fatalf("SQL %+v\nGo  %+v", intentsFrom(got), intentsFrom(want))
	}
}
