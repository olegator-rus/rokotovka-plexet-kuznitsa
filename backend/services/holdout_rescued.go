package services

import (
	"context"
	"encoding/json"
	"fmt"
	"math"
	"slices"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Доля спасённых на реплее отложенной недели (holdout_week) для «До / После» — GET /api/evaluations?set=holdout_week,
// поле rescued. Остаток модели эффекта контуров поиска–2: сама модель в рублях (GET /api/effect, /api/effect/commerce)
// удалена в контуре доводки, формула и сценарии — docs/EFFECT_MODEL.md. Доля спасённых — замер, не модель.

// holdoutSet — набор реплея отложенной недели.
const holdoutSet = domain.EvalSetHoldoutWeek

// agentCredit — засчитывается ли вклад агента (B3 → B4) в долю спасённых.
type agentCredit struct {
	// Counted — все принятые правки правил версии B4 сделаны запусками после разреза журнала.
	Counted bool
	// PreHoldout — среди правок B4 есть сделанные запусками до разреза журнала: B4 видел оцениваемые тексты,
	// и его числа — не независимая оценка (ни на test, ни на отложенной неделе).
	PreHoldout bool
	// Note — почему вклад агента не засчитан (или что засчитано).
	Note string
}

// honestRun — запуск агента шёл с разрезом журнала и без текстов test (эпизод holdout).
func honestRun(run *domain.AgentRun) bool {
	if run == nil {
		return false
	}
	var ep struct {
		Holdout *struct {
			Enabled     bool `json:"enabled"`
			ExcludeTest bool `json:"exclude_test"`
		} `json:"holdout"`
	}
	_ = json.Unmarshal(run.Episode, &ep)
	return ep.Holdout != nil && ep.Holdout.Enabled && ep.Holdout.ExcludeTest
}

// agentCredit — правки, вошедшие в версию правил прогона B4 на отложенной неделе (принятые, не откаченные), и
// честность их запусков. Хоть одна правка из запуска до разреза — вклад агента не засчитывается: она подобрана
// под эту же неделю и тексты test.
func (s *Services) agentCredit(ctx context.Context, evals []domain.Evaluation) agentCredit {
	var b3, b4 *domain.Evaluation
	for i := range evals {
		switch evals[i].Config {
		case domain.ConfigB3:
			b3 = &evals[i]
		case domain.ConfigB4:
			b4 = &evals[i]
		}
	}
	if b4 == nil || b3 == nil {
		return agentCredit{Note: "нет прогона B3 и B4 на отложенной неделе — вклад агента не измерен"}
	}
	st := s.store()
	if st == nil {
		return agentCredit{Note: "нет базы агента — вклад агента не засчитан"}
	}
	accepted, err := st.ListProposals(ctx, domain.ProposalAccepted, "")
	if err != nil {
		return agentCredit{Note: "предложения агента не прочитаны — вклад агента не засчитан"}
	}
	var honest, dishonest []string
	for _, p := range accepted {
		if p.ConfigVersion == nil || *p.ConfigVersion > b4.RulesVersion || *p.ConfigVersion <= b3.RulesVersion {
			continue
		}
		run, err := st.GetAgentRun(ctx, p.RunID)
		if err == nil && honestRun(run) {
			honest = append(honest, p.ID.String()[:8])
		} else {
			dishonest = append(dishonest, p.RunID.String()[:8])
		}
	}
	switch {
	case len(dishonest) > 0:
		slices.Sort(dishonest)
		return agentCredit{PreHoldout: true, Note: fmt.Sprintf("правки B4 (v%d) сделаны запусками до разреза журнала (%s): они видели "+
			"отложенную неделю и тексты test — вклад агента не засчитан (0)", b4.RulesVersion, strings.Join(slices.Compact(dishonest), ", "))}
	case len(honest) == 0:
		return agentCredit{Note: "принятых правок агента в B4 нет — вклад агента 0"}
	}
	return agentCredit{Counted: true, Note: fmt.Sprintf("правки B4 (v%d) из запусков после разреза журнала: %s",
		b4.RulesVersion, strings.Join(honest, ", "))}
}

// measureRescued — доля спасённых на реплее отложенной недели, по поискам (частота запроса — число поисков
// недели). Среди размеченных запросов, где B0 не дал релевантного в топ-10: вклад поиска — доля, где его дал
// B3; вклад агента — где B3 не дал, а B4 дал (только если credit.Counted). Прогоны B0, B3 и B4 — на одном
// наборе (set_hash). Интервал Уилсона — по числу различных запросов (поиски одного запроса не независимы).
func measureRescued(evals []domain.Evaluation, credit agentCredit) *out.EffectRescued {
	by := map[string]domain.Evaluation{}
	for _, e := range evals {
		if e.Set == "" || e.Set == holdoutSet {
			by[e.Config] = e
		}
	}
	base, ok := by[domain.ConfigB0]
	if !ok {
		return nil
	}
	top, ok := by[domain.ConfigB3]
	if !ok || top.SetHash != base.SetHash {
		return nil
	}
	agent, hasAgent := by[domain.ConfigB4]
	hasAgent = hasAgent && agent.SetHash == base.SetHash
	load := func(e domain.Evaluation) map[string]out.EvalQueryResult {
		var rows []out.EvalQueryResult
		if json.Unmarshal(e.PerQuery, &rows) != nil {
			return nil
		}
		m := make(map[string]out.EvalQueryResult, len(rows))
		for _, r := range rows {
			m[r.ID] = r
		}
		return m
	}
	b0, b3 := load(base), load(top)
	var b4 map[string]out.EvalQueryResult
	if hasAgent {
		b4 = load(agent)
	}
	r := &out.EffectRescued{Set: holdoutSet, SetHash: base.SetHash, Baseline: domain.ConfigB0, Config: domain.ConfigB3,
		RunBaseline: base.RunID, RunConfig: top.RunID, RulesVersion: top.RulesVersion, Weight: "searches",
		AgentCounted: credit.Counted && hasAgent, AgentPreHoldout: credit.PreHoldout, AgentNote: credit.Note}
	if hasAgent {
		r.RunAgent, r.AgentRulesVersion = agent.RunID, agent.RulesVersion
	}
	var searchRescued, agentRescued, rescuedQ int
	for id, q := range b0 {
		a, ok := b3[id]
		// Только размеченные запросы (MRR посчитан) с ответом обеих конфигураций, где B0 не дал релевантного.
		if q.MRR10 == nil || q.Error != "" || !ok || a.Error != "" || q.HasRelevant {
			continue
		}
		w := max(q.Freq, 1)
		r.Failed += w
		r.FailedQueries++
		switch {
		case a.HasRelevant:
			searchRescued += w
			rescuedQ++
		case r.AgentCounted:
			if x, ok := b4[id]; ok && x.Error == "" && x.HasRelevant {
				agentRescued += w
				rescuedQ++
			}
		}
	}
	if r.Failed == 0 {
		return nil
	}
	r.Rescued = searchRescued + agentRescued
	r.RescuedQueries = rescuedQ
	r.SearchShare = evalRound4(float64(searchRescued) / float64(r.Failed))
	r.AgentShare = evalRound4(float64(agentRescued) / float64(r.Failed))
	r.Share = evalRound4(float64(r.Rescued) / float64(r.Failed))
	r.CILow, r.CIHigh = wilsonP(float64(r.Rescued)/float64(r.Failed), r.FailedQueries)
	agentPart := "вклад агента B3 → B4 не засчитан"
	if r.AgentCounted {
		r.Config = domain.ConfigB4
		agentPart = fmt.Sprintf("вклад агента B3 → B4 (прогон %s)", r.RunAgent)
	}
	r.Source = fmt.Sprintf("реплей отложенной недели (holdout_week, %s): поиски, которых агент не видел; "+
		"B0 → B3 (прогоны %s, %s), %s; доля — по поискам", base.SetHash, base.RunID, top.RunID, agentPart)
	return r
}

// wilsonP — 95% интервал Уилсона для доли p на n наблюдениях.
func wilsonP(p float64, n int) (float64, float64) {
	const z = 1.96
	nn := float64(n)
	den := 1 + z*z/nn
	c := (p + z*z/(2*nn)) / den
	h := z * math.Sqrt(p*(1-p)/nn+z*z/(4*nn*nn)) / den
	return evalRound4(math.Max(0, c-h)), evalRound4(math.Min(1, c+h))
}

// round2 — округление до сотых (остался от модели эффекта: им пользуются прогон панели и suggest-bench).
func round2(v float64) float64 { return math.Round(v*100) / 100 }
