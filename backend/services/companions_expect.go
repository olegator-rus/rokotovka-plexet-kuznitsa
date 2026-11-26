package services

import (
	"encoding/json"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Метрики блока по ожиданиям набора p2 (queries.json, поле expect): якорь, обязательные с количеством,
// несовместимые позиции, отсутствие блока на категорийных и отрицательных запросах. Чистая функция над ответами
// поиска; прогон — трек агента (C-лестница).

// companionExpect — часть expect, которая относится к допродаже.
type companionExpect struct {
	AnchorFamily string `json:"anchor_family"`
	Quantity     int    `json:"quantity"`
	Required     []struct {
		FamilyID  string `json:"family_id"`
		ProductID string `json:"product_id"`
		Metric    string `json:"metric"`
		Quantity  int    `json:"quantity"`
	} `json:"required"`
	Incompatible []string        `json:"incompatible"`
	NoRequired   bool            `json:"no_required"`
	Companions   json.RawMessage `json:"companions"` // присутствует и null — блока быть не должно
}

// QueryCompanionScore — метрики по запросам p2. Доли — nil при пустом знаменателе.
type QueryCompanionScore struct {
	Queries int `json:"queries"`
	// Якорь найден и совпал с ожидаемым семейством.
	AnchorExpected int      `json:"anchor_expected"`
	AnchorHit      int      `json:"anchor_hit"`
	AnchorAcc      *float64 `json:"anchor_acc"`
	// Обязательные: ожидаемый компаньон в строке обязательного (по семейству или позиции, метрике).
	RequiredExpected int      `json:"required_expected"`
	RequiredHit      int      `json:"required_hit"`
	RequiredRecall   *float64 `json:"required_recall"`
	// Количество: suggested_quantity совпало с ожидаемым (среди найденных обязательных с количеством).
	QuantityExpected int      `json:"quantity_expected"`
	QuantityHit      int      `json:"quantity_hit"`
	QuantityAcc      *float64 `json:"quantity_acc"`
	// IncompatibleShown — показаны позиции из expect.incompatible (цель 0).
	IncompatibleShown int `json:"incompatible_shown"`
	// FalseBlocks — блок или строка обязательного там, где их быть не должно (companions: null, no_required).
	NoBlockExpected int `json:"no_block_expected"`
	FalseBlocks     int `json:"false_blocks"`
}

// QueryCompanionsResult — ответ поиска на запрос набора, нужный метрике.
type QueryCompanionsResult struct {
	Query    domain.EvalQuery
	Response *out.SearchResponse
}

// ScoreCompanionQueries — метрики блока по ожиданиям expect.
func ScoreCompanionQueries(rs []QueryCompanionsResult) QueryCompanionScore {
	var sc QueryCompanionScore
	for _, r := range rs {
		if len(r.Query.Expect) == 0 || r.Response == nil {
			continue
		}
		var e companionExpect
		if json.Unmarshal(r.Query.Expect, &e) != nil {
			continue
		}
		sc.Queries++
		resp := r.Response
		var required []out.Companion
		anchorFamily := ""
		for _, it := range resp.Items {
			if len(it.Required) > 0 {
				required, anchorFamily = it.Required, it.FamilyID
				break
			}
		}
		if resp.Companions != nil {
			anchorFamily = resp.Companions.Anchor.FamilyID
		}
		if a := anchorOfTrace(resp); a != nil {
			anchorFamily = a.FamilyID
		}
		shown := map[string]bool{}
		for _, x := range required {
			shown[x.ProductID] = true
		}
		if resp.Companions != nil {
			for _, x := range resp.Companions.Items {
				shown[x.ProductID] = true
			}
		}
		if e.AnchorFamily != "" {
			sc.AnchorExpected++
			if anchorFamily == e.AnchorFamily {
				sc.AnchorHit++
			}
		}
		for _, want := range e.Required {
			sc.RequiredExpected++
			for _, got := range required {
				if want.ProductID != "" && got.ProductID != want.ProductID || want.FamilyID != "" && got.FamilyID != want.FamilyID {
					continue
				}
				if want.Metric != "" && !companionHasMetric(got, want.Metric) {
					continue
				}
				sc.RequiredHit++
				if want.Quantity > 0 {
					sc.QuantityExpected++
					if got.SuggestedQuantity != nil && *got.SuggestedQuantity == want.Quantity {
						sc.QuantityHit++
					}
				}
				break
			}
		}
		for _, id := range e.Incompatible {
			if shown[id] {
				sc.IncompatibleShown++
			}
		}
		noBlock := string(e.Companions) == "null"
		if noBlock || e.NoRequired {
			sc.NoBlockExpected++
			if noBlock && len(shown) > 0 || !noBlock && len(required) > 0 {
				sc.FalseBlocks++
			}
		}
	}
	sc.AnchorAcc = ratio(sc.AnchorHit, sc.AnchorExpected)
	sc.RequiredRecall = ratio(sc.RequiredHit, sc.RequiredExpected)
	sc.QuantityAcc = ratio(sc.QuantityHit, sc.QuantityExpected)
	return sc
}

// companionHasMetric — метрика компаньона по строке лицензии («на 1 пользователя»): позиция в каталоге не
// нужна, достаточно класса лицензии из ответа. Без строки — не проверяется.
func companionHasMetric(c out.Companion, metric string) bool {
	forms, ok := metricForms[metric]
	if !ok || c.LicenseLine == "" {
		return true
	}
	for _, f := range forms {
		if f != "" && strings.Contains(strings.ToLower(c.LicenseLine), f) {
			return true
		}
	}
	return false
}

// anchorOfTrace — якорь из шага companions трассы ответа.
func anchorOfTrace(resp *out.SearchResponse) *out.CompanionAnchor {
	if resp.Trace == nil {
		return nil
	}
	for _, st := range resp.Trace.Steps {
		if cs, ok := st.(CompanionsStep); ok {
			return cs.Anchor
		}
	}
	return nil
}
