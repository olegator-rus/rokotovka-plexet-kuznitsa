package services

import (
	"context"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/opensearch/opensearchtest"
)

// eventsSource — фикстура каталога со своим журналом поисков.
type eventsSource struct {
	catalogsource.MockJSON
	events []domain.LogEvent
}

func (s eventsSource) LoadEvents(context.Context) ([]domain.LogEvent, error) { return s.events, nil }

// TestSuggestMinFreq — запрос из логов, встреченный один раз, в подсказки не попадает; дважды — попадает.
func TestSuggestMinFreq(t *testing.T) {
	n := 5
	search := func(q string) domain.LogEvent { return domain.LogEvent{Type: "search", Query: q, ResultsCount: &n} }
	src := eventsSource{MockJSON: fixtureSource(), events: []domain.LogEvent{
		search("редкий запрос покупателя"),
		search("частый запрос витрины"), search("частый запрос витрины"),
	}}
	os := opensearchtest.New()
	t.Cleanup(os.Close)
	svc := NewServices(nil, Options{Search: opensearch.New(os.URL, 0), Source: src})
	if _, err := svc.ImportCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	has := func(q, want string) bool {
		resp, err := svc.Suggest(t.Context(), q, "")
		if err != nil {
			t.Fatal(err)
		}
		for _, g := range resp.Groups {
			for _, it := range g.Items {
				if g.Type == suggestQuery && it.Text == want {
					return true
				}
			}
		}
		return false
	}
	if has("редкий", "редкий запрос покупателя") {
		t.Error("разовый запрос из логов показан в подсказках")
	}
	if !has("частый", "частый запрос витрины") {
		t.Error("запрос с частотой 2 не показан в подсказках")
	}
}
