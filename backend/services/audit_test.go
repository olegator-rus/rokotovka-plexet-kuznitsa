package services

import (
	"strings"
	"testing"

	"kuznica/api/externalApi/catalogsource"
)

func TestAuditFixture(t *testing.T) {
	s := NewServices(nil, Options{Source: catalogsource.MockJSON{Dir: "../testdata/catalog"}})
	r, err := s.Audit(t.Context(), AuditOptions{})
	if err != nil {
		t.Fatal(err)
	}
	if r.Volumes.Products != 12 || r.Search == nil || r.Search.Skipped == "" || len(r.Fields) != len(auditFieldLabels) {
		t.Fatalf("аудит: %+v", r)
	}
	for _, f := range r.Fields {
		if f.Field == "license_quantity" && (f.FromSource == 0 || f.Restored != 0) {
			t.Fatalf("mock: количество из данных, не правилами: %+v", f)
		}
	}
	md := FormatAuditMarkdown(r)
	for _, want := range []string{"## Что меняется в плане", "| Маржа |", "## Покрытие полей каталога", "Не прогонялся"} {
		if !strings.Contains(md, want) {
			t.Errorf("в AUDIT.md нет %q", want)
		}
	}
}

func TestAuditQueriesMergeLog(t *testing.T) {
	zero := 0
	qs := auditQueries(nil, []catalogsource.QueryLogRow{{Text: "Касперский", Freq: 5}, {Text: "касперский ", Freq: 2, ResultsCount: &zero}, {Text: "ворд", Freq: 9}})
	if len(qs) != 2 || qs[0].text != "ворд" || qs[1].freq != 7 || !qs[1].logZero {
		t.Fatalf("запросы: %+v", qs)
	}
}
