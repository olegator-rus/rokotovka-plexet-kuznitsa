package services

import (
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/repositories"

	"github.com/google/uuid"
)

func runCluster(run uuid.UUID, at time.Time, label, status, diag, note string, queries string) repositories.RunCluster {
	c := repositories.RunCluster{RunStartedAt: at}
	c.ID, c.RunID, c.Label, c.Status, c.Diagnosis, c.Note = uuid.New(), run, label, status, diag, note
	c.Queries = domain.JSONB(queries)
	return c
}

// Вывод агента у проблемного запроса (QA M2): предложение важнее кластера, новый запуск — старого,
// пропуск по памяти — только если содержательного итога нет; «не разобрано» — только если кластера не было.
func TestApplyVerdicts(t *testing.T) {
	newRun, oldRun := uuid.New(), uuid.New()
	now := time.Now()
	clusters := []repositories.RunCluster{
		runCluster(newRun, now, "продлить антивирус", "skipped", "", "действующий поиск понимает все слова и отвечает", `[{"text":"Продлить  антивирус"}]`),
		runCluster(newRun, now, "фигма", "skipped", "", "Память: похожее предложение «figma» отклонено", `[]`),
		runCluster(newRun, now, "notion", "investigated", "ASSORTMENT_GAP", "нет в каталоге", `[{"text":"ноушн"}]`),
		runCluster(newRun, now, "123", "investigated", "NON_PRODUCT", "не о покупке ПО", `[]`),
		runCluster(newRun, now, "компас", "new", "", "", `[]`),
		runCluster(oldRun, now.Add(-time.Hour), "компас", "investigated", "UNKNOWN", "ранжирование", `[]`),
		runCluster(oldRun, now.Add(-time.Hour), "продлить антивирус", "investigated", "ALIAS_GAP", "старый итог", `[]`),
		runCluster(oldRun, now.Add(-time.Hour), "удаленный доступ", "new", "", "", `[]`),
	}
	pid, st, diag := "p1", "pending", "ALIAS_GAP"
	resp := &out.AnalyticsResponse{Problems: []out.ProblemQuery{
		{Query: "продлить антивирус", Freq: 42, Failures: 42},
		{Query: "фигма", Freq: 10, Failures: 10},
		{Query: "ноушн", Freq: 6, Failures: 6},
		{Query: "123", Freq: 4, Failures: 4},
		{Query: "компас", Freq: 11, Failures: 11},
		{Query: "удаленный доступ", Freq: 11, Failures: 11},
		{Query: "зуп", Freq: 15, Failures: 15, ProposalID: &pid, ProposalStatus: &st, ProposalDiagnosis: &diag},
		{Query: "касперский", Freq: 91, Failures: 33},
		{Query: "мусор", Freq: 5, Failures: 5},
	}}
	applyVerdicts(resp, clusters)
	want := []string{verdictAnswers, verdictEarlier, verdictNoOffer, verdictClosed, verdictClosed, verdictQueued,
		verdictProposal, verdictMostlyOK, verdictUnseen}
	for i, p := range resp.Problems {
		if p.Agent.Code != want[i] || p.Agent.Label == "" {
			t.Errorf("%q: вывод %+v, ждали %s", p.Query, p.Agent, want[i])
		}
	}
	if resp.Problems[0].Agent.RunID != newRun.String() {
		t.Errorf("итог не из нового запуска: %+v", resp.Problems[0].Agent)
	}
	if resp.Problems[4].Agent.RunID != oldRun.String() {
		t.Errorf("«не дошёл» в новом запуске должен уступить разбору в старом: %+v", resp.Problems[4].Agent)
	}
	if resp.Problems[8].Diagnosis != "" || resp.Problems[0].Diagnosis != verdictLabels[verdictAnswers] {
		t.Errorf("подпись diagnosis: %q / %q", resp.Problems[8].Diagnosis, resp.Problems[0].Diagnosis)
	}
}

// QA r2 №4: упор в лимит, «уже решено» и оборванный разбор — разные подписи, не «закрыто: не решается правкой правил».
func TestVerdictByNote(t *testing.T) {
	run, now := uuid.New(), time.Now()
	cases := []struct{ note, want string }{
		{"лимит вызовов инструментов", verdictLimit},
		{"лимит шагов запуска", verdictLimit},
		{"дневной бюджет LLM исчерпан", verdictLimit},
		{NoteAlreadySolved, verdictAnswers},
		{noteUnfinished, verdictStopped},
		{noteRepeatPrefix + " «Задача владельцу» (нет доказательства)", verdictStopped},
		{"остановлено: context canceled", verdictStopped},
		{"не о покупке ПО", verdictClosed},
	}
	for _, c := range cases {
		rc := runCluster(run, now, "x", "investigated", "UNKNOWN", c.note, `[]`)
		v, ok := verdictFor(&rc)
		if !ok || v.Code != c.want || v.Label == "" || v.Note != c.note {
			t.Errorf("%q: %+v, ждали %s", c.note, v, c.want)
		}
	}
	if verdictLabels[verdictLimit] == verdictLabels[verdictClosed] || verdictLabels[verdictStopped] == verdictLabels[verdictClosed] {
		t.Error("подписи исходов должны различаться")
	}
}
