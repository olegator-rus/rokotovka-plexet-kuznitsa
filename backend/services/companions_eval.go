package services

import (
	"context"
	"sort"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Метрики блока допродажи для оценки (C-лестница трека агента). Чистые функции над
// размеченными парами companion_pairs.json: показанное блоком сравнивается с разметкой.

// CompanionScore — метрики блока на наборе пар. Доли — nil, если знаменатель 0 (непосчитанное не показывается).
type CompanionScore struct {
	// Anchors — якорей в разметке; Covered — из них с непустым блоком (required ∪ items).
	Anchors int `json:"anchors"`
	Covered int `json:"covered"`
	// Coverage — доля якорей с ожидаемым блоком (есть пара required/compatible/conditional/complement), у которых
	// блок непуст.
	Coverage *float64 `json:"coverage"`
	// ShownPairs — показанных пар «якорь → позиция блока»; LabeledShown — из них размеченных.
	ShownPairs   int `json:"shown_pairs"`
	LabeledShown int `json:"labeled_shown"`
	// CompatViolations — показанных пар с разметкой incompatible (блокирующая, цель 0); ViolationRate — их доля
	// среди размеченных показанных.
	CompatViolations int      `json:"compat_violations"`
	ViolationRate    *float64 `json:"compat_violation_rate"`
	// RequiredRecall — доля якорей с обязательным компаньоном в разметке, у которых блок показал хотя бы один
	// из них.
	RequiredAnchors int      `json:"required_anchors"`
	RequiredHit     int      `json:"required_hit"`
	RequiredRecall  *float64 `json:"required_recall"`
	// Violations — пары-нарушения (для списка «проверенных пар» в До / После).
	Violations []domain.CompanionPair `json:"violations"`
}

// expectsBlock — классы пар, при которых у якоря ожидается непустой блок.
var expectsBlock = map[string]bool{"required": true, "compatible": true, "conditional": true, "complement": true}

func ratio(n, d int) *float64 {
	if d == 0 {
		return nil
	}
	v := float64(n) / float64(d)
	return &v
}

// ScoreCompanions — метрики по разметке pairs и показанному shown (якорь → id позиций блока и строки
// обязательного). Якорь без записи в shown — блок пуст. familyOf (может быть nil) — семейство позиции: блок
// показывает одну позицию семейства, и обязательное засчитывается, если показана позиция того же семейства,
// что размеченная (CAL на 1 пользователя вместо размеченной CAL на 5).
func ScoreCompanions(pairs []domain.CompanionPair, shown map[string][]string, familyOf func(string) string) CompanionScore {
	label := map[[2]string]string{}
	anchors := map[string]bool{}
	expect := map[string]bool{}
	required := map[string]map[string]bool{}
	for _, p := range pairs {
		label[[2]string{p.AnchorProduct, p.CandidateProduct}] = p.Class
		anchors[p.AnchorProduct] = true
		if expectsBlock[p.Class] {
			expect[p.AnchorProduct] = true
		}
		if p.Class == "required" {
			if required[p.AnchorProduct] == nil {
				required[p.AnchorProduct] = map[string]bool{}
			}
			required[p.AnchorProduct][p.CandidateProduct] = true
			if familyOf != nil {
				required[p.AnchorProduct]["f:"+familyOf(p.CandidateProduct)] = true
			}
		}
	}
	sc := CompanionScore{Anchors: len(anchors), RequiredAnchors: len(required), Violations: []domain.CompanionPair{}}
	ids := make([]string, 0, len(anchors))
	for a := range anchors {
		ids = append(ids, a)
	}
	sort.Strings(ids)
	for _, a := range ids {
		items := shown[a]
		if expect[a] && len(items) > 0 {
			sc.Covered++
		}
		hit := false
		for _, c := range items {
			sc.ShownPairs++
			cls, ok := label[[2]string{a, c}]
			if !ok {
				hit = hit || familyOf != nil && required[a]["f:"+familyOf(c)]
				continue
			}
			sc.LabeledShown++
			if cls == "incompatible" {
				sc.CompatViolations++
				sc.Violations = append(sc.Violations, domain.CompanionPair{AnchorProduct: a, CandidateProduct: c, Class: cls})
			}
			hit = hit || required[a][c] || familyOf != nil && required[a]["f:"+familyOf(c)]
		}
		if hit {
			sc.RequiredHit++
		}
	}
	sc.Coverage = ratio(sc.Covered, len(expect))
	sc.ViolationRate = ratio(sc.CompatViolations, sc.LabeledShown)
	sc.RequiredRecall = ratio(sc.RequiredHit, sc.RequiredAnchors)
	return sc
}

// shownIDs — id позиций строки обязательного и блока.
func shownIDs(required []out.Companion, block *out.CompanionBlock) []string {
	ids := []string{}
	for _, x := range required {
		ids = append(ids, x.ProductID)
	}
	if block != nil {
		for _, x := range block.Items {
			ids = append(ids, x.ProductID)
		}
	}
	return ids
}

// CompanionsForAnchor — подбор к заданному якорю без поиска (якорь считается уверенным): для оценки блока на
// парах и для агента (evaluate_companions). ctx — песочница справочников агента (commerceFor).
func (s *Services) CompanionsForAnchor(ctx context.Context, anchorID string, quantity int, metric string) (
	required []out.Companion, block *out.CompanionBlock, step CompanionsStep) {
	cc := s.commerceFor(ctx)
	cs := s.catalog.Load()
	if cc == nil || cs == nil || cc.products[anchorID] == nil {
		return []out.Companion{}, nil, CompanionsStep{StepBase: domain.StepBase{Name: StepCompanions, Skipped: true,
			Reason: "якоря нет в каталоге"}, Considered: []CompanionConsidered{}, Rejected: []domain.RejectedHit{}}
	}
	p := cc.products[anchorID]
	r := cc.companionsFor(cs.lex, &out.CompanionAnchor{ProductID: anchorID, FamilyID: p.FamilyID, Confidence: "high"},
		companionQuery{quantity: quantity, metric: metric})
	return r.required, r.block, r.step
}

// EvaluateCompanionPairs — метрики блока на размеченных парах: подбор к каждому якорю разметки (флаг
// допродажи из ctx учитывается: выключен — блок пуст у всех якорей, как в C0/C1).
func (s *Services) EvaluateCompanionPairs(ctx context.Context, pairs []domain.CompanionPair) CompanionScore {
	shown := map[string][]string{}
	if s.featuresFor(ctx).Companions {
		for _, p := range pairs {
			if _, done := shown[p.AnchorProduct]; done {
				continue
			}
			req, block, _ := s.CompanionsForAnchor(ctx, p.AnchorProduct, 0, "")
			shown[p.AnchorProduct] = shownIDs(req, block)
		}
	}
	var familyOf func(string) string
	if cc := s.commerceFor(ctx); cc != nil {
		familyOf = func(pid string) string {
			if p := cc.products[pid]; p != nil {
				return p.FamilyID
			}
			return ""
		}
	}
	return ScoreCompanions(pairs, shown, familyOf)
}
