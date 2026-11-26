package services

import (
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/opensearch/opensearchtest"
	"kuznica/api/externalApi/tei"
	"kuznica/api/externalApi/tei/teitest"
	"kuznica/api/in"
	"kuznica/api/pkg/errs"
)

// importedWith — фикстура, импортированная в сервис с заданными флагами контура коммерции.
func importedWith(t *testing.T, f Features) *Services {
	t.Helper()
	osrv, models := opensearchtest.New(), teitest.New()
	t.Cleanup(osrv.Close)
	t.Cleanup(models.Close)
	svc := NewServices(nil, Options{Search: opensearch.New(osrv.URL, 0), Embedder: tei.New(models.URL, 0),
		Source: fixtureSource(), Features: f})
	if _, err := svc.ImportCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	return svc
}

// Поля контура коммерции в ответе поиска присутствуют всегда, и в пустом ответе, и в ответе с позициями.
func TestSearchP2FieldsAlwaysPresent(t *testing.T) {
	env := imported(t, true)
	for _, c := range []struct{ cfg, query string }{{"B0", "notion"}, {"B1", "kaspersky"}, {"B3", "kaspersky"}, {"live", "photoshop"}} {
		raw, err := json.Marshal(search(t, env, c.cfg, c.query, ""))
		if err != nil {
			t.Fatal(err)
		}
		for _, want := range []string{`"companions":null`, `"solution":null`, `"facets":[]`, `"complements":[`} {
			if !strings.Contains(string(raw), want) {
				t.Fatalf("%s %q: нет %s в %s", c.cfg, c.query, want, raw)
			}
		}
		var resp struct {
			Items []map[string]json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(raw, &resp); err != nil {
			t.Fatal(err)
		}
		// Поддельный кластер понимает только простые запросы B0–B2: позиции проверяются на B1, поля ответа — везде.
		if c.cfg == "B1" && len(resp.Items) == 0 {
			t.Fatalf("%s %q: пустая выдача", c.cfg, c.query)
		}
		for _, it := range resp.Items {
			if string(it["required"]) != "[]" || string(it["compatibility"]) != "null" || string(it["kind"]) != `"software"` ||
				string(it["mpn"]) != `""` || !slices.Contains([]string{`"fixed"`, `"quote_required"`}, string(it["price_mode"])) {
				t.Fatalf("%s %q: поля позиции %s", c.cfg, c.query, raw)
			}
		}
	}
}

// Оборудование: вид, парт-номер и тип цены доходят до позиции ответа из индекса.
func TestSearchHardwareItem(t *testing.T) {
	env := imported(t, true)
	resp := search(t, env, "B1", "Dell PowerEdge R740", "")
	if len(resp.Items) == 0 || resp.Items[0].ProductID != "dell-r740-base" {
		t.Fatalf("выдача: %+v", resp.Items)
	}
	if it := resp.Items[0]; it.Kind != domain.KindHardware || it.MPN != "PER740-BASE-01" || it.PriceMode != domain.PriceFrom {
		t.Fatalf("сервер: %+v", it)
	}
	doc := env.os.Docs("products")["ram-32g-rdimm"]
	if doc["kind"] != "hardware" || doc["mpn_raw"] != "SNS-R740-32G" || doc["mpn_normalized"] != "SNSR74032G" ||
		doc["price_mode"] != "quote_required" || doc["specs"].(map[string]any)["ram_gb"].(float64) != 32 {
		t.Fatalf("документ памяти: %v", doc)
	}
	if sw := env.os.Docs("products")["kis-2dev-1y"]; sw["mpn_raw"] != nil || sw["specs"] != nil || sw["kind"] != "software" {
		t.Fatalf("документ ПО: %v", sw)
	}
}

// Флаги выключены — выдача та же, что с включёнными: логики контура коммерции ещё нет, и включение не должно менять выдачу
// контура поиска (флаг выключен — ответ идентичен контуру поиска).
func TestSearchFeaturesOffSameResults(t *testing.T) {
	on := importedWith(t, Features{Hardware: true, Companions: true, MarginPolicy: true, Solutions: true})
	off := importedWith(t, Features{})
	for _, cfg := range []string{"B0", "B1", "B2", "B3", "live"} {
		for _, q := range []string{"касперский", "фотошоп", "office 2021", "антивирус на 5 устройств", "dell r740", "notion"} {
			a, err := on.Search(t.Context(), "r", in.SearchRequest{Query: q, Config: cfg})
			if err != nil {
				t.Fatal(err)
			}
			b, err := off.Search(t.Context(), "r", in.SearchRequest{Query: q, Config: cfg})
			if err != nil {
				t.Fatal(err)
			}
			a.Trace, b.Trace = nil, nil
			ja, _ := json.Marshal(a)
			jb, _ := json.Marshal(b)
			if string(ja) != string(jb) {
				t.Fatalf("%s %q: выдача зависит от флагов\non:  %s\noff: %s", cfg, q, ja, jb)
			}
		}
	}
}

// Фасеты запроса: форма проверяется, логика — у трека оборудования.
func TestSearchFiltersValidation(t *testing.T) {
	env := imported(t, false)
	two := 64
	for name, f := range map[string]in.Filter{
		"key":    {Op: "eq", Value: 32},
		"op":     {Key: "ram_gb", Op: "between", Value: 32},
		"value":  {Key: "ram_gb", Op: "gte"},
		"value2": {Key: "ram_gb", Op: "range", Value: 16},
	} {
		_, err := env.svc.Search(t.Context(), "r", in.SearchRequest{Query: "dell", Filters: []in.Filter{f}})
		var e *errs.Error
		if !errors.As(err, &e) || e.Type != errs.ErrTypeValidation || len(e.Details) != 1 || !strings.HasSuffix(e.Details[0].Field, name) {
			t.Fatalf("%s: %v", name, err)
		}
	}
	if _, err := env.svc.Search(t.Context(), "r", in.SearchRequest{Query: "dell",
		Filters: []in.Filter{{Key: "ram_gb", Op: "range", Value: 16, Value2: two}}}); err != nil {
		t.Fatalf("верный фасет: %v", err)
	}
}

// /api/meta: флаги и пометка синтетических данных.
func TestMetaP2(t *testing.T) {
	svc := importedWith(t, Features{Hardware: true, Companions: true})
	m := svc.Meta()
	if !m.Features.Hardware || !m.Features.Companions || m.Features.MarginPolicy || m.Features.Solutions ||
		len(m.Synthetic) != 1 || m.Synthetic[0] != "margin" {
		t.Fatalf("meta: %+v / %v", m.Features, m.Synthetic)
	}
	st := svc.catalog.Load()
	c := st.commerce
	if len(c.licenseByAnchor["windows-server-2025"]) != 1 || len(c.relationsFrom["windows-server-2025"]) != 1 ||
		len(c.attrDefs["servers"]) != 3 || len(c.compatRules) != 1 || len(c.solutions) != 1 || c.products["ram-32g-rdimm"] == nil {
		t.Fatalf("справочники контура коммерции в памяти: %+v", c)
	}
	if raw, _ := json.Marshal(NewServices(nil, Options{}).Meta()); !strings.Contains(string(raw), `"synthetic":[]`) {
		t.Fatalf("meta без каталога: %s", raw)
	}
}

// События контура коммерции: тип принимается, обязательные поля проверяются; события контура поиска — как раньше.
func TestRecordEventP2(t *testing.T) {
	svc := NewServices(nil, Options{})
	check := func(req in.EventRequest, wantFields ...string) {
		t.Helper()
		err := svc.RecordEvent(t.Context(), req)
		var e *errs.Error
		if !errors.As(err, &e) {
			t.Fatalf("%s: %v", req.Type, err)
		}
		if len(wantFields) == 0 {
			// Проверка пройдена — без базы событие некуда записать.
			if e.Type != errs.ErrTypeUnavailable {
				t.Fatalf("%s: ожидалось 503 без базы, получено %v", req.Type, err)
			}
			return
		}
		var got []string
		for _, d := range e.Details {
			got = append(got, d.Field)
		}
		if e.Type != errs.ErrTypeValidation || !slices.Equal(got, wantFields) {
			t.Fatalf("%s: поля %v, ожидались %v", req.Type, got, wantFields)
		}
	}
	check(in.EventRequest{Type: "companion_add", ProductID: "ws2025-cal-user",
		Payload: json.RawMessage(`{"class":"conditional_required","quantity":50}`)})
	check(in.EventRequest{Type: "companion_add", Payload: json.RawMessage(`{"class":"complement"}`)}, "product_id", "payload.quantity")
	check(in.EventRequest{Type: "relation_decision", Payload: json.RawMessage(`{"relation_id":"rel-0001","decision":"accept"}`)})
	check(in.EventRequest{Type: "facet_apply"}, "payload.key", "payload.value")
	check(in.EventRequest{Type: "click", ProductID: "kis-2dev-1y"})
	for _, typ := range []string{"companion_impression", "companion_click", "companion_dismiss", "spec_remove", "template_apply",
		"quote_request", "margin_simulate", "margin_apply"} {
		if !slices.Contains(domain.BehaviorEventTypes, typ) || len(domain.EventRequiredFields[typ]) == 0 {
			t.Fatalf("тип %s не заведён", typ)
		}
	}
}

func TestNormalizeMPN(t *testing.T) {
	for in, want := range map[string]string{
		"SNS-R740-32G": "SNSR74032G", " sns r740/32g ": "SNSR74032G", "J9775A#ABB": "J9775A#ABB", "": "",
		"P00924-B21": "P00924B21",
	} {
		if got := normalizeMPN(in); got != want {
			t.Errorf("%q → %q, ожидалось %q", in, got, want)
		}
	}
}
