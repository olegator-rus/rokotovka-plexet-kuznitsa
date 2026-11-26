package services

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/repositories"
)

// Проблемные запросы «за период» и «сейчас». Таблица менеджера e-com строится по историческим
// журналам: «пустая выдача 36 из 36» — это старый поиск магазина за период. Рядом нужны две вещи:
//   - вывод агента — из предложения по запросу или из итога кластера последних запусков (issue_clusters):
//     «поиск уже отвечает», «закрыто: не решается правкой правил», «спрос без предложения»; «не разобрано» —
//     только если агент кластер не видел;
//   - текущая выдача — поиск без записи в журнал (NoLog) на действующих правилах, с кэшем на версию правил и
//     каталога: отдельный GET /api/analytics/current, чтобы таблица не ждала поисков.

const (
	verdictRuns     = 10               // сколько последних запусков агента смотреть
	nowCacheTTL     = 10 * time.Minute // кэш текущей выдачи
	nowDefaultLimit = 15
	nowParallel     = 4
)

// Коды вывода агента (out.AgentVerdict.Code).
const (
	verdictProposal = "proposal"
	verdictAnswers  = "answers"
	verdictClosed   = "closed"
	verdictLimit    = "limit"
	verdictStopped  = "stopped"
	verdictNoOffer  = "no_offer"
	verdictEarlier  = "earlier"
	verdictQueued   = "queued"
	verdictMostlyOK = "mostly_ok"
	verdictUnseen   = "unseen"
)

var verdictLabels = map[string]string{
	verdictProposal: "есть предложение агента",
	verdictAnswers:  "поиск уже отвечает",
	verdictClosed:   "закрыто: не решается правкой правил",
	verdictLimit:    "не доразобрано: агент упёрся в лимит",
	verdictStopped:  "не доразобрано: агент не дошёл до итога",
	verdictNoOffer:  "спрос без предложения",
	verdictEarlier:  "разобран раньше (память агента)",
	verdictQueued:   "агент видел, но не дошёл",
	verdictMostlyOK: "агент не брал: запрос чаще находит",
	verdictUnseen:   "не разобрано",
}

// memoryNotePrefix — кластер пропущен по памяти агента (решение было раньше): ищем итог в прежних запусках.
const memoryNotePrefix = "Память:"

// verdictFor — вывод агента по кластеру запуска; ok=false — кластер пропущен по памяти или не разбирался
// (ищем более содержательный итог в прежних запусках).
func verdictFor(c *repositories.RunCluster) (out.AgentVerdict, bool) {
	v := out.AgentVerdict{RunID: c.RunID.String(), ClusterID: c.ID.String(), ClusterLabel: c.Label,
		Diagnosis: c.Diagnosis, Note: shortText(c.Note, 300)}
	switch {
	case c.Status == "skipped" && strings.HasPrefix(c.Note, memoryNotePrefix):
		v.Code = verdictEarlier
		return withLabel(v), false
	case c.Status == "skipped":
		v.Code = verdictAnswers
	case c.Status == "investigated" && (c.Diagnosis == "ASSORTMENT_GAP" || c.Diagnosis == "SUBSTITUTE_AVAILABLE"):
		v.Code = verdictNoOffer
	case c.Status == "investigated" && c.Note == NoteAlreadySolved:
		v.Code = verdictAnswers
	case c.Status == "investigated" && noteIsLimit(c.Note):
		v.Code = verdictLimit
	case c.Status == "investigated" && noteIsStopped(c.Note):
		v.Code = verdictStopped
	case c.Status == "investigated":
		v.Code = verdictClosed
	default:
		v.Code = verdictQueued
		return withLabel(v), false
	}
	return withLabel(v), true
}

// noteIsLimit — кластер закрыт упором в лимит (вызовов на кластер, бюджет запуска, дневной бюджет LLM): итог
// кластера (outcome) в issue_clusters не хранится, его выдаёт заметка, которую пишет цикл агента (agent_loop.go).
func noteIsLimit(note string) bool {
	if note == "лимит вызовов инструментов" || note == "дневной бюджет LLM исчерпан" {
		return true
	}
	for _, l := range budgetLabels {
		if note == l+" запуска" {
			return true
		}
	}
	return false
}

// noteIsStopped — разбор оборвался без итога: модель не вызвала завершающий инструмент, сбой модели, отмена,
// повтор отклонённого вызова.
func noteIsStopped(note string) bool {
	return note == noteUnfinished || note == "сбой модели" || strings.HasPrefix(note, "остановлено:") ||
		strings.HasPrefix(note, noteRepeatPrefix)
}

func withLabel(v out.AgentVerdict) out.AgentVerdict {
	v.Label = verdictLabels[v.Code]
	return v
}

// clusterTexts — тексты кластера для сопоставления с запросом: подпись и запросы (foldQuery).
func clusterTexts(c *repositories.RunCluster) []string {
	texts := []string{foldQuery(c.Label)}
	var qs []struct {
		Text string `json:"text"`
	}
	_ = json.Unmarshal(c.Queries, &qs)
	for _, q := range qs {
		texts = append(texts, foldQuery(q.Text))
	}
	return texts
}

// applyVerdicts — вывод агента у каждого проблемного запроса. Предложение (linkProposals) важнее итога кластера;
// среди кластеров — новый запуск важнее старого, а содержательный итог (разобран или пропущен, потому что поиск
// отвечает) — пропуска по памяти и «не дошёл».
func applyVerdicts(resp *out.AnalyticsResponse, clusters []repositories.RunCluster) {
	best := map[string]out.AgentVerdict{}
	final := map[string]bool{}
	for i := range clusters {
		c := &clusters[i]
		v, ok := verdictFor(c)
		for _, t := range clusterTexts(c) {
			if t == "" || final[t] {
				continue
			}
			if _, seen := best[t]; seen && !ok {
				continue
			}
			best[t], final[t] = v, ok
		}
	}
	for i := range resp.Problems {
		p := &resp.Problems[i]
		switch v, ok := best[p.Query]; {
		case p.ProposalID != nil:
			p.Agent = withLabel(out.AgentVerdict{Code: verdictProposal, Diagnosis: derefStr(p.ProposalDiagnosis)})
		case ok:
			p.Agent = v
		case p.Failures*2 < p.Freq:
			// Агент берёт в разбор неудачные запросы; этот чаще заканчивается кликом — его кластера нет.
			p.Agent = withLabel(out.AgentVerdict{Code: verdictMostlyOK,
				Note: fmt.Sprintf("неудачных %d из %d поисков за период — агент разбирает запросы, которые чаще не находят", p.Failures, p.Freq)})
		default:
			p.Agent = withLabel(out.AgentVerdict{Code: verdictUnseen})
		}
		if p.Agent.Code != verdictUnseen {
			p.Diagnosis = p.Agent.Label
		}
		if d := p.Agent.Diagnosis; d != "" {
			c := CauseOf(d)
			p.Cause, p.CauseLabel, p.Owner, p.OwnerLabel = c.Code, c.Label, c.Owner, c.OwnerLabel
		}
	}
}

// agentVerdicts — итоги кластеров последних запусков в ответ аналитики; без базы — у всех «не разобрано».
func (s *Services) agentVerdicts(ctx context.Context, resp *out.AnalyticsResponse) error {
	var clusters []repositories.RunCluster
	if s.repos.Enabled() {
		var err error
		if clusters, err = s.repos.RecentIssueClusters(ctx, verdictRuns); err != nil {
			return err
		}
	}
	applyVerdicts(resp, clusters)
	return nil
}

// nowCache — текущая выдача по ключу «версия правил | каталог | запрос».
var nowCache = struct {
	sync.Mutex
	m map[string]nowEntry
}{m: map[string]nowEntry{}}

type nowEntry struct {
	at   time.Time
	item out.ProblemNow
}

// ProblemsNow — GET /api/analytics/current?days=&limit=: текущая выдача по первым limit проблемным запросам
// периода (как в таблице). Поиск — действующие правила (B4), без записи в журнал.
func (s *Services) ProblemsNow(ctx context.Context, days, limit int) (*out.ProblemsNowResponse, error) {
	if days == 0 {
		days = analyticsDefaultDays
	}
	if limit == 0 {
		limit = nowDefaultLimit
	}
	var bad []errs.FieldError
	if days < 1 || days > analyticsMaxDays {
		bad = append(bad, errs.FieldError{Field: "days", Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Период — от 1 до %d дней", analyticsMaxDays)})
	}
	if limit < 1 || limit > problemsLimit {
		bad = append(bad, errs.FieldError{Field: "limit", Code: errs.CodeOutOfRange, Message: fmt.Sprintf("Число запросов — от 1 до %d", problemsLimit)})
	}
	if len(bad) > 0 {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	now := s.now().UTC()
	res := &out.ProblemsNowResponse{Days: days, CheckedAt: now, Items: []out.ProblemNow{}, RulesVersion: 1}
	if !s.repos.Enabled() {
		return res, nil
	}
	_, problems, err := s.repos.SearchOutcomesSince(ctx, now.Add(-time.Duration(days)*24*time.Hour), limit)
	if err != nil {
		return nil, err
	}
	snap, err := s.repos.LatestRules(ctx)
	if err != nil {
		return nil, err
	}
	if snap != nil {
		res.RulesVersion = snap.Version
	}
	if cs := s.catalog.Load(); cs != nil {
		res.CatalogVersion = cs.version
	}
	res.Items = make([]out.ProblemNow, len(problems))
	sem := make(chan struct{}, nowParallel)
	var wg sync.WaitGroup
	for i, p := range problems {
		key := fmt.Sprintf("%d|%s|%s", res.RulesVersion, res.CatalogVersion, p.Query)
		nowCache.Lock()
		e, ok := nowCache.m[key]
		nowCache.Unlock()
		if ok && now.Sub(e.at) < nowCacheTTL {
			e.item.Cached = true
			res.Items[i] = e.item
			continue
		}
		wg.Add(1)
		go func(i int, q, key string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			item := s.problemNow(ctx, q, snap)
			res.Items[i] = item
			if item.Error == "" {
				nowCache.Lock()
				nowCache.m[key] = nowEntry{at: time.Now(), item: item}
				nowCache.Unlock()
			}
		}(i, p.Query, key)
	}
	wg.Wait()
	return res, nil
}

// problemNow — один запрос через действующий поиск без записи в журнал.
func (s *Services) problemNow(ctx context.Context, q string, rules *domain.RulesSnapshot) out.ProblemNow {
	o, err := s.currentSearch(ctx, q, rules)
	item := out.ProblemNow{Query: q}
	if err != nil {
		item.Error = "поиск не ответил"
		return item
	}
	item.Total, item.NotInCatalog, item.Exact, item.Route = o.Total, o.Empty, o.Solved, o.Route
	if len(o.Top) > 0 {
		item.TopTitle = o.Top[0].Title
	}
	return item
}
