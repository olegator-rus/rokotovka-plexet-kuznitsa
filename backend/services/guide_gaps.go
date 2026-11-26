package services

import (
	"context"
	"encoding/json"
	"sort"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Пробелы подбора для SearchOps (SPEC §12): «Вопросы, на которые подбор не ответил». Источник —
// guide_turns.gap_items; причина — «Нет факта в базе знаний» (causes.go) для missing_fact, для остальных событий —
// своя подпись. Пополнить базу или словарь предлагает SearchOps, правило публикует человек.

const (
	guideGapsMaxTurns = 5000
	guideGapsMaxRows  = 100
)

// guideGapLabels — подписи событий пробела.
var guideGapLabels = map[string]string{
	"missing_fact":          "Нет факта в базе знаний",
	"missing_compatibility": "Совместимость не подтверждена документом",
	"license_unresolved":    "Не удалось определить применимость лицензии",
	"no_matching_offer":     "Нет оффера под условия",
	"ambiguous_family":      "Неясно, какое семейство имел в виду покупатель",
	"turn_timeout":          "Ход не уложился в предел",
}

// GuideGaps — GET /api/guide/gaps?days=: вопросы, на которые подбор не ответил, сгруппированные по событию и
// тексту вопроса, частые первыми.
func (s *Services) GuideGaps(ctx context.Context, days int) (*out.GuideGapReport, error) {
	if days <= 0 || days > 90 {
		days = 7
	}
	cause := CauseOf(domain.DiagKBMissingFact)
	res := &out.GuideGapReport{Title: "Вопросы, на которые подбор не ответил", Days: days,
		Cause: out.GuideGapCause{Code: cause.Code, Label: cause.Label, Owner: cause.Owner, OwnerLabel: cause.OwnerLabel},
		Rows:  []out.GuideGapRow{}}
	if !s.repos.Enabled() {
		return res, nil
	}
	rows, err := s.repos.GuideGapItems(ctx, time.Now().Add(-time.Duration(days)*24*time.Hour), guideGapsMaxTurns)
	if err != nil {
		return nil, err
	}
	cat := s.guide().catalog()
	type key struct{ code, q string }
	agg := map[key]*out.GuideGapRow{}
	order := []key{}
	for _, raw := range rows {
		var items []guideGapItem
		if json.Unmarshal(raw, &items) != nil {
			continue
		}
		for _, it := range items {
			res.Total++
			k := key{it.Code, it.Question}
			r, ok := agg[k]
			if !ok {
				r = &out.GuideGapRow{Code: it.Code, Label: guideGapLabels[it.Code], Question: it.Question, Families: []out.GuideGapFamily{}}
				if r.Label == "" {
					r.Label = it.Code
				}
				for _, f := range it.FamilyIDs {
					if len(r.Families) >= 3 {
						break
					}
					title := ""
					if cat != nil {
						title = cat.famName[f]
					}
					r.Families = append(r.Families, out.GuideGapFamily{FamilyID: f, Title: title})
				}
				agg[k] = r
				order = append(order, k)
			}
			r.Count++
		}
	}
	for _, k := range order {
		res.Rows = append(res.Rows, *agg[k])
	}
	sort.SliceStable(res.Rows, func(i, j int) bool { return res.Rows[i].Count > res.Rows[j].Count })
	if len(res.Rows) > guideGapsMaxRows {
		res.Rows = res.Rows[:guideGapsMaxRows]
	}
	return res, nil
}
