package services

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
)

// Прогон правки справочника связей в песочнице (evaluate_companions): правка накладывается на
// действующие связи только в ctx (withCommerceOverride), боевые не меняются. Лестница проверок:
// структура (правка накладывается, нет дублей), находка снята и новых нет (правила лицензий и совместимости),
// блок допродажи на размеченных парах не хуже, регрессия поиска не хуже.

// companionPairLoader — источник с размеченными парами блока (MockJSON).
type companionPairLoader interface {
	LoadCompanionPairs(ctx context.Context) ([]domain.CompanionPair, error)
}

// companionPairs — пары companion_pairs.json; нет файла или источника — пусто.
func (s *Services) companionPairs(ctx context.Context) []domain.CompanionPair {
	l, ok := s.source.(companionPairLoader)
	if !ok {
		return nil
	}
	ps, err := l.LoadCompanionPairs(ctx)
	if err != nil {
		return nil
	}
	return ps
}

// relationPatchEval — результат прогона правки связей.
type relationPatchEval struct {
	FindingKey string `json:"finding_key"`
	// Resolved — находка снята; NewFindings — находки, которых до правки не было.
	Resolved       bool              `json:"resolved"`
	FindingsBefore int               `json:"findings_before"`
	FindingsAfter  int               `json:"findings_after"`
	NewFindings    []relationFinding `json:"new_findings"`
	// Block — метрики блока допродажи на companion_pairs.json до и после (nil — пар нет).
	Block *relationBlockEval `json:"block"`
	// Regression — регрессия поиска (набор regression, B4) до и после; nil — не прогонялась (Skipped — почему).
	Regression  *relationRegression `json:"regression"`
	Skipped     []string            `json:"skipped"`
	Passed      bool                `json:"passed"`
	Reasons     []string            `json:"reasons"`
	BaseVersion int                 `json:"relations_version"`
	Ms          int64               `json:"ms"`
}

// relationBlockEval — блок допродажи на размеченных парах до и после правки.
type relationBlockEval struct {
	Pairs  int            `json:"pairs"`
	Before CompanionScore `json:"before"`
	After  CompanionScore `json:"after"`
	// Enabled — блок включён флагом (FEATURE_COMPANIONS); выключен — метрики пустые у обеих сторон.
	Enabled bool `json:"enabled"`
}

// relationRegression — регрессия поиска: выдача и NDCG@10 запросов набора до и после правки.
type relationRegression struct {
	Set   string `json:"set"`
	Total int    `json:"total"`
	Worse int    `json:"worse"`
	// Better — запросов, где NDCG@10 вырос; BlockChanged — где поменялся блок допродажи или строка обязательного.
	Better       int      `json:"better"`
	BlockChanged int      `json:"block_changed"`
	WorseQueries []string `json:"worse_queries"`
}

// regressionSnap — выдача одного запроса регрессии для сравнения.
type regressionSnap struct {
	ndcg  float64
	block string
	err   bool
}

// relationSandbox — песочница прогона правок одного запуска: базовые справочники, пары и снимок регрессии
// на действующих связях (считается один раз).
type relationSandbox struct {
	s       *Services
	base    *commerceCatalog
	docs    []relationDoc
	version int
	pairs   []domain.CompanionPair
	before  *relationScan
	// regBefore — снимок регрессии на действующих связях; regQueries — запросы набора.
	regQueries []domain.EvalQuery
	regBefore  map[string]regressionSnap
	regErr     error
	cat        *evalCatalog
	maxWorse   int
}

func (s *Services) newRelationSandbox(ctx context.Context, ch *relationChain) (*relationSandbox, error) {
	c := s.commerceFor(context.Background())
	if c == nil {
		return nil, errors.New(MsgRelationsNoCatalog)
	}
	base := c.withRelations(relationsOfDocs(ch.Active))
	sb := &relationSandbox{s: s, base: base, docs: ch.Active, version: ch.Version, pairs: s.companionPairs(ctx),
		maxWorse: s.agent.opts.MaxRegressionWorse}
	sb.before = scanRelations(base, ch.Active)
	return sb, nil
}

// regressionSearch — один запрос регрессии: NDCG@10 и состав блока допродажи.
func (sb *relationSandbox) regressionSearch(ctx context.Context, q domain.EvalQuery) regressionSnap {
	sr := in.SearchRequest{Query: q.Text, Config: domain.ConfigB4, Limit: 10, SessionID: evalRequestPrefix + "rel"}
	if q.ClientID != nil {
		sr.ClientID = *q.ClientID
	}
	resp, err := sb.s.SearchWith(ctx, agentRequestID(), sr, SearchOptions{NoLog: true})
	if err != nil {
		return regressionSnap{err: true}
	}
	o := evalOutcome{Total: resp.Total, EmptyMarker: resp.Empty != nil}
	for _, it := range resp.Items {
		o.Items = append(o.Items, evalItem{ProductID: it.ProductID, FamilyID: it.FamilyID, Group: it.Group})
	}
	r := evalScoreQuery(q, o, sb.cat)
	snap := regressionSnap{}
	if r.NDCG10 != nil {
		snap.ndcg = *r.NDCG10
	}
	var block []string
	for _, it := range resp.Items {
		for _, x := range it.Required {
			block = append(block, "r:"+x.ProductID)
		}
	}
	if resp.Companions != nil {
		for _, x := range resp.Companions.Items {
			block = append(block, "c:"+x.ProductID)
		}
	}
	slices.Sort(block)
	snap.block = fmt.Sprint(block)
	return snap
}

// regression — снимок набора regression на справочниках из ctx.
func (sb *relationSandbox) regression(ctx context.Context) (map[string]regressionSnap, error) {
	if sb.regQueries == nil && sb.regErr == nil {
		qs, err := sb.s.evalQueries(ctx, domain.EvalSetRegression, nil)
		if err == nil {
			sb.cat, err = sb.s.evalCatalog(ctx)
		}
		if err != nil {
			sb.regErr = err
		} else {
			sb.regQueries = qs
		}
	}
	if sb.regErr != nil {
		return nil, sb.regErr
	}
	res := make(map[string]regressionSnap, len(sb.regQueries))
	for _, q := range sb.regQueries {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		res[q.ID] = sb.regressionSearch(ctx, q)
	}
	return res, nil
}

// evaluate — прогон правки: структура, находки, блок на парах, регрессия поиска.
func (sb *relationSandbox) evaluate(ctx context.Context, patch RelationPatch, findingKey string, withSearch bool) (*relationPatchEval, []relationDoc) {
	started := time.Now()
	ev := &relationPatchEval{FindingKey: findingKey, NewFindings: []relationFinding{}, Reasons: []string{}, Skipped: []string{},
		BaseVersion: sb.version, FindingsBefore: len(sb.before.Findings)}
	defer func() { ev.Ms = time.Since(started).Milliseconds() }()
	if patch.empty() {
		ev.Reasons = append(ev.Reasons, "правка пустая")
		return ev, nil
	}
	after, err := applyRelationPatch(sb.docs, patch, "")
	if err != nil {
		ev.Reasons = append(ev.Reasons, "правка не накладывается: "+err.Error())
		return ev, nil
	}
	scan := scanRelations(sb.base, after)
	ev.FindingsAfter = len(scan.Findings)
	beforeKeys := sb.before.keys()
	ev.Resolved = !scan.keys()[findingKey]
	for _, f := range scan.Findings {
		if !beforeKeys[f.Key] {
			ev.NewFindings = append(ev.NewFindings, f)
		}
	}
	if !ev.Resolved {
		ev.Reasons = append(ev.Reasons, "находка не снята: проверка справочника после правки находит её снова")
	}
	if len(ev.NewFindings) > 0 {
		ev.Reasons = append(ev.Reasons, fmt.Sprintf("правка создаёт новые проблемы: %d (%s)", len(ev.NewFindings), ev.NewFindings[0].Text))
	}
	afterCat := sb.base.withRelations(relationsOfDocs(after))
	if len(sb.pairs) > 0 {
		cur := withCommerceOverride(ctx, sb.base)
		next := withCommerceOverride(ctx, afterCat)
		b := &relationBlockEval{Pairs: len(sb.pairs), Enabled: sb.s.featuresFor(ctx).Companions,
			Before: sb.s.EvaluateCompanionPairs(cur, sb.pairs), After: sb.s.EvaluateCompanionPairs(next, sb.pairs)}
		ev.Block = b
		if b.After.CompatViolations > b.Before.CompatViolations {
			ev.Reasons = append(ev.Reasons, fmt.Sprintf("в блоке больше нарушений совместимости: %d → %d",
				b.Before.CompatViolations, b.After.CompatViolations))
		}
		if b.After.RequiredHit < b.Before.RequiredHit {
			ev.Reasons = append(ev.Reasons, fmt.Sprintf("блок реже показывает обязательное: %d → %d якорей",
				b.Before.RequiredHit, b.After.RequiredHit))
		}
	} else {
		ev.Skipped = append(ev.Skipped, "блок: нет размеченных пар companion_pairs.json")
	}
	if withSearch && sb.s.search != nil {
		if sb.regBefore == nil && sb.regErr == nil {
			sb.regBefore, sb.regErr = sb.regression(withCommerceOverride(ctx, sb.base))
		}
		var afterSnap map[string]regressionSnap
		if sb.regErr == nil {
			afterSnap, sb.regErr = sb.regression(withCommerceOverride(ctx, afterCat))
		}
		if sb.regErr != nil {
			ev.Skipped = append(ev.Skipped, "регрессия поиска не прогнана: "+shortText(sb.regErr.Error(), 120))
			if ctx.Err() != nil {
				ev.Reasons = append(ev.Reasons, "прогон прерван")
			}
		} else {
			reg := &relationRegression{Set: domain.EvalSetRegression, WorseQueries: []string{}}
			for _, q := range sb.regQueries {
				b, a := sb.regBefore[q.ID], afterSnap[q.ID]
				if b.err || a.err {
					continue
				}
				reg.Total++
				switch {
				case a.ndcg < b.ndcg-1e-9:
					reg.Worse++
					reg.WorseQueries = append(reg.WorseQueries, q.Text)
				case a.ndcg > b.ndcg+1e-9:
					reg.Better++
				}
				if a.block != b.block {
					reg.BlockChanged++
				}
			}
			ev.Regression = reg
			if reg.Worse > sb.maxWorse {
				ev.Reasons = append(ev.Reasons, fmt.Sprintf("регрессия поиска: хуже %d запросов из %d", reg.Worse, reg.Total))
			}
		}
	} else if withSearch {
		ev.Skipped = append(ev.Skipped, "регрессия поиска: движок поиска не настроен")
	}
	ev.Passed = len(ev.Reasons) == 0
	return ev, after
}

// recheckRelationPatch — перепроверка правки на сменившемся справочнике при принятии: структура и находки
// (детерминированно, без поиска).
func (s *Services) recheckRelationPatch(ctx context.Context, ch *relationChain, patch RelationPatch, key string) *relationPatchEval {
	sb, err := s.newRelationSandbox(ctx, ch)
	if err != nil {
		return &relationPatchEval{Reasons: []string{err.Error()}}
	}
	sb.pairs = nil
	ev, _ := sb.evaluate(ctx, patch, key, false)
	// На сменившемся справочнике находки может уже не быть (её сняли другой правкой) — тогда правка лишняя.
	if strings.HasPrefix(key, copurchaseKeyPrefix) {
		// Связь по совместным покупкам: находки справочника нет по построению — проверяется, что связи ещё нет.
		for _, a := range patch.Add {
			if d := hasRelation(sb.docs, a.FromFamily, a.ToFamily); d != nil {
				ev.Passed = false
				ev.Reasons = append(ev.Reasons, fmt.Sprintf("связь между %s и %s в справочнике уже есть (%s)", a.FromFamily, a.ToFamily, d.ID))
			}
		}
		return ev
	}
	if !sb.before.keys()[key] && key != "" {
		ev.Passed = false
		ev.Reasons = append(ev.Reasons, "проблемы, которую снимала правка, в действующем справочнике уже нет")
	}
	return ev
}
