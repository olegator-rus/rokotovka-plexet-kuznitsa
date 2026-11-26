package services

import (
	"strings"
	"testing"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/in"
	"kuznica/api/out"
)

// Раздел строки отчёта — по доказательству: вывод агента главнее ответа поиска, ответ поиска — журнала; без
// доказательства — «ещё не разобраны».
func TestClassifyReportRow(t *testing.T) {
	q := clusterQuery{Text: "рутоен эцп", Freq: 3, Failures: 3}
	cases := []struct {
		name    string
		row     out.ReportRow
		q       clusterQuery
		probe   reportProbe
		section string
		kind    string
	}{
		{"агент: опечатка", out.ReportRow{Agent: &out.AgentVerdict{Diagnosis: domain.DiagTypo}}, q,
			reportProbe{onRequest: true, found: &out.ReportFound{Title: "x"}}, ReportNotFound, reportKindNotUnder},
		{"предложение главнее кластера", out.ReportRow{Agent: &out.AgentVerdict{Diagnosis: domain.DiagUnknown},
			Proposal: &out.ReportProposal{Diagnosis: domain.DiagOnRequestOnly}}, q, reportProbe{}, ReportNoOffer, reportKindOnRequest},
		{"агент: нет в каталоге", out.ReportRow{Agent: &out.AgentVerdict{Diagnosis: domain.DiagAssortmentGap}}, q,
			reportProbe{}, ReportNoOffer, reportKindAbsent},
		{"агент: не о товаре", out.ReportRow{Agent: &out.AgentVerdict{Diagnosis: domain.DiagNonProduct}}, q,
			reportProbe{}, ReportService, reportKindService},
		{"поиск: справка", out.ReportRow{}, q, reportProbe{service: true}, ReportService, reportKindService},
		{"поиск: первая точная только по запросу", out.ReportRow{}, q,
			reportProbe{onRequest: true, found: &out.ReportFound{Title: "Microsoft Visual Studio"}}, ReportNoOffer, reportKindOnRequest},
		{"журнал: нашли после переформулировки", out.ReportRow{},
			clusterQuery{Text: "рутоен эцп", ReformulatedTo: []reformTarget{{Text: "рутокен эцп", Count: 2, Clicked: 1}}},
			reportProbe{}, ReportNotFound, reportKindReform},
		{"переформулировали, но не нашли — не доказательство", out.ReportRow{},
			clusterQuery{Text: "x", ReformulatedTo: []reformTarget{{Text: "y", Count: 2}}}, reportProbe{}, "", reportKindPending},
		{"без доказательства", out.ReportRow{Agent: &out.AgentVerdict{Code: verdictAnswers, Label: "поиск уже отвечает"}}, q,
			reportProbe{}, "", reportKindPending},
	}
	for _, c := range cases {
		row := c.row
		got := classifyReportRow(&row, c.q, c.probe)
		if got != c.section || row.Kind != c.kind || row.Evidence == "" || row.KindLabel == "" {
			t.Errorf("%s: раздел %q, вид %q (%q), ожидалось %q, %q", c.name, got, row.Kind, row.Evidence, c.section, c.kind)
		}
	}
}

func TestOnRequestOnly(t *testing.T) {
	item := func(title, match, price string) out.SearchItem {
		return out.SearchItem{Title: title, MatchType: match, PriceState: price}
	}
	resp := func(items ...out.SearchItem) *out.SearchResponse { return &out.SearchResponse{Items: items} }
	vs := item("Microsoft Visual Studio Enterprise with MSDN", "exact", "on_request")
	if !onRequestOnly("microsoft visual studio", resp(vs, item("Visio", "exact", "priced"))) {
		t.Error("первая точная «по запросу» и это искомое — купить нельзя")
	}
	if onRequestOnly("microsoft visual studio", resp(item("Microsoft Visual Studio", "exact", "priced"), vs)) {
		t.Error("первая точная с ценой — купить можно")
	}
	if onRequestOnly("microsoft visual studio", resp(item("Microsoft Visual Studio", "alternative", "on_request"))) {
		t.Error("не точная — не «найдено»")
	}
	if onRequestOnly("ворд", resp(item("Write Brothers Word Menu", "exact", "on_request"))) {
		t.Error("слово запроса заменил поиск («ворд» → Word Menu) — не доказательство, разбирает агент")
	}
	if onRequestOnly("икс спайдер", resp(item("ИКС Стандарт", "exact", "on_request"))) {
		t.Error("не все слова запроса в названии")
	}
	withTarget := resp(item("Kaspersky Security, продление", "exact", "on_request"))
	withTarget.Target = &out.Target{Type: out.TargetVendor, Title: "Касперский"}
	if !onRequestOnly("продление касперского", withTarget) {
		t.Error("падеж: «касперского» — «касперский»")
	}
	if !onRequestOnly("photoshop", &out.SearchResponse{Items: []out.SearchItem{{Title: "Adobe Photoshop", Group: "exact", PriceOnRequest: true}}}) {
		t.Error("mock без лестницы: группа exact и price_on_request")
	}
	if onRequestOnly("x", resp()) {
		t.Error("пустая выдача")
	}
}

func TestRegressionStatusFeed(t *testing.T) {
	sets := []string{"feed_regression", "feed_tune"}
	feed := regressionStatus(sets, map[string]int{}, true, "softline-feed", 0)
	if feed.Ready || !feed.Feed || !strings.HasPrefix(feed.Message, MsgFeedNoRegression) {
		t.Fatalf("фид без наборов: %+v", feed)
	}
	ok := regressionStatus(sets, map[string]int{"feed_regression": 17, "feed_tune": 178, "closed": 60}, true,
		"softline-feed", 0)
	if !ok.Ready || ok.Queries != 195 || ok.BySet["feed_tune"] != 178 || ok.BySet["closed"] != 0 {
		t.Fatalf("фид с наборами: только наборы агента, закрытый не считается: %+v", ok)
	}
	mock := regressionStatus([]string{"regression"}, nil, false, "mock-2", 0)
	if mock.Ready || mock.Feed || strings.HasPrefix(mock.Message, MsgFeedNoRegression) {
		t.Fatalf("mock без наборов: %+v", mock)
	}
}

// ON_REQUEST_ONLY — задача каталогу только для семейства, которое нельзя купить: проверяет код по каталогу, а не
// модель. У семейства с ценой — отказ; у «по запросу» — задача владельцу каталога (content), правки поиска нет.
func TestAgentOnRequestOnlyTicket(t *testing.T) {
	model := &scriptModel{}
	s, ms := agentEnv(t, model)
	model.turns = append(model.turns,
		[]llm.ToolCall{tc("create_assortment_ticket", map[string]any{"diagnosis": "ON_REQUEST_ONLY",
			"text": "купить нельзя", "family_id": "kaspersky-internet-security"})},
		[]llm.ToolCall{tc("create_assortment_ticket", map[string]any{"diagnosis": "ON_REQUEST_ONLY",
			"text": "все позиции только по запросу — нужна цена и наличие", "family_id": "adobe-photoshop"})})
	r, err := s.StartAgentRun(t.Context(), in.AgentRunRequest{MaxClusters: 1})
	if err != nil {
		t.Fatal(err)
	}
	run := waitRun(t, s, r.RunID)
	refused := false
	for _, st := range ms.steps {
		if st.Tool != nil && *st.Tool == "create_assortment_ticket" && !st.OK && strings.Contains(st.Title, "есть позиции с ценой") {
			refused = true
		}
	}
	if !refused {
		t.Fatal("ON_REQUEST_ONLY для семейства с ценой должен быть отклонён кодом")
	}
	var ticket *domain.Proposal
	for _, p := range ms.proposals {
		if p.Diagnosis == domain.DiagOnRequestOnly {
			ticket = &p
		}
	}
	if ticket == nil || ticket.Action != domain.ActionTicket || ticket.Role != domain.RoleContent || run.ProposalsCreated != 1 {
		t.Fatalf("ожидалась задача каталогу: %+v, запуск %+v", ticket, run)
	}
	if CauseOf(domain.DiagOnRequestOnly).Code != CauseOnRequestOnly {
		t.Fatal("причина «найдено, но только по запросу»")
	}
}
