package services

import (
	"context"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// «Старшая редакция»: к первой позиции выдачи (точное совпадение) —
// утверждённая связь upgrade от её позиции или семейства. Карточка стоит рядом с выдачей, не в блоке дополнений,
// и только если у связи есть строки «что добавится». Порядок выдачи не меняется: это отдельное поле ответа.

// StepUpgrade — шаг трассы «Старшая редакция».
const StepUpgrade = "upgrade"

// labelUpgrade — подпись карточки.
const labelUpgrade = "Старшая редакция"

// maxUpgradeAdds — строк «что добавится» в карточке.
const maxUpgradeAdds = 2

// Причины пропуска.
const (
	upgradeNoItems   = "empty_results"
	upgradeNotExact  = "first_not_exact"
	upgradeNoRel     = "no_upgrade_relation"
	upgradeNoAdds    = "no_upgrade_adds"
	upgradeTargetBad = "target_unavailable"
)

// UpgradeStep — шаг трассы upgrade: к какой позиции, по какой связи, что предложено.
type UpgradeStep struct {
	domain.StepBase
	ForProductID string `json:"for_product_id,omitempty"`
	RelationID   string `json:"relation_id,omitempty"`
	ProductID    string `json:"product_id,omitempty"`
}

// upgradeStep — «Старшая редакция» к первой позиции выдачи. Флаг решений выключен или поиск слота — nil.
func (s *Services) upgradeStep(ctx context.Context, lx *searchLexicon, analog bool, resp *out.SearchResponse) *UpgradeStep {
	if !s.featuresFor(ctx).Solutions || isSlotSearch(ctx) {
		return nil
	}
	t := time.Now()
	step := &UpgradeStep{StepBase: domain.StepBase{Name: StepUpgrade}}
	defer func() { step.Ms = msSince(t) }()
	skip := func(reason string) *UpgradeStep {
		step.Skipped, step.Reason = true, reason
		return step
	}
	if len(resp.Items) == 0 || resp.Empty != nil {
		return skip(upgradeNoItems)
	}
	first := resp.Items[0]
	if first.Group != domain.GroupExact || analog {
		return skip(upgradeNotExact)
	}
	step.ForProductID = first.ProductID
	up, rel, reason := upgradeFor(s.commerceFor(ctx), lx, first.ProductID)
	if up == nil {
		return skip(reason)
	}
	step.RelationID, step.ProductID = relID(rel), up.ProductID
	resp.Upgrade = up
	return step
}

// upgradeFor — карточка старшей редакции к позиции: связь upgrade (утверждённая, от этой позиции или её семейства
// без уточнения), цель в продаже и не та же позиция, есть строки «что добавится».
func upgradeFor(cc *commerceCatalog, lx *searchLexicon, pid string) (*out.Upgrade, *domain.Relation, string) {
	if cc == nil || lx == nil {
		return nil, nil, upgradeNoRel
	}
	p := cc.products[pid]
	if p == nil {
		return nil, nil, upgradeNoRel
	}
	reason := upgradeNoRel
	for i := range cc.relationsFrom[p.FamilyID] {
		rel := &cc.relationsFrom[p.FamilyID][i]
		if rel.Type != domain.RelationUpgrade || rel.ReviewStatus == domain.ReviewPending || rel.ReviewStatus == domain.ReviewRejected {
			continue
		}
		if rel.FromProduct != nil && *rel.FromProduct != pid {
			continue
		}
		if len(rel.UpgradeAdds) == 0 {
			reason = upgradeNoAdds
			continue
		}
		var target *domain.Product
		if rel.ToProduct != nil {
			target = cc.products[*rel.ToProduct]
		} else if rel.ToFamily != p.FamilyID {
			for _, x := range cc.compat.byFamily[rel.ToFamily] {
				if x.PriceMode != domain.PriceUnavailable && (target == nil || x.Popularity > target.Popularity) {
					target = x
				}
			}
		}
		if target == nil || target.ID == pid || target.PriceMode == domain.PriceUnavailable {
			reason = upgradeTargetBad
			continue
		}
		d := lx.docs[target.ID]
		adds := rel.UpgradeAdds
		if len(adds) > maxUpgradeAdds {
			adds = adds[:maxUpgradeAdds]
		}
		c := out.Companion{ProductID: target.ID, FamilyID: target.FamilyID, Title: target.Title, Vendor: d.Vendor,
			Category: d.Category, LicenseLine: licenseLine(d), PriceRub: target.PriceRub, PriceMode: target.PriceMode,
			Class: domain.RelationUpgrade, Compatibility: rel.Compatibility, Reason: rel.Note,
			Conditions: append([]string{}, rel.Conditions...), EvidenceLevel: rel.EvidenceLevel, Source: rel.Source,
			SourceURL: rel.SourceURL, Label: labelUpgrade, UpgradeAdds: append([]string{}, adds...)}
		if c.PriceMode == "" {
			c.PriceMode = docPriceMode(d)
		}
		if d.ProductID == "" {
			c.LicenseLine = ""
		}
		return &out.Upgrade{ForProductID: pid, RelationID: relID(rel), Companion: c}, rel, ""
	}
	return nil, nil, reason
}

// ProductCompanions — GET /api/companions?product_id=&quantity=: компаньоны к позиции, добавленной в спецификацию
// из категорийной выдачи («К этой позиции: …»). Якорь — сама позиция (выбрал покупатель — уверенный);
// подбор тот же, что у блока «Дополните решение». Флаг допродажи выключен — пустой ответ.
func (s *Services) ProductCompanions(ctx context.Context, productID string, quantity int) (*out.ProductCompanions, error) {
	if productID == "" || len(productID) > 128 {
		return nil, errs.NewValidationError(errs.MsgInvalidQuery).WithDetails(errs.FieldError{Field: "product_id",
			Code: errs.CodeRequired, Message: "Укажите позицию"})
	}
	if quantity < 0 || quantity > 100000 {
		return nil, errs.NewValidationError(errs.MsgInvalidQuery).WithDetails(errs.FieldError{Field: "quantity",
			Code: errs.CodeOutOfRange, Message: "Количество — от 0 до 100000"})
	}
	res := &out.ProductCompanions{ProductID: productID, Required: []out.Companion{}, Items: []out.Companion{}}
	cc := s.commerceFor(ctx)
	if cc == nil || cc.products[productID] == nil {
		return nil, errs.NewNotFoundError("Позиция")
	}
	if !s.featuresFor(ctx).Companions {
		return res, nil
	}
	required, block, _ := s.CompanionsForAnchor(ctx, productID, quantity, "")
	res.Required = required
	if block != nil {
		res.Items = block.Items
	}
	if cs := s.catalog.Load(); cs != nil && s.featuresFor(ctx).Solutions {
		res.Upgrade, _, _ = upgradeFor(cc, cs.lex, productID)
	}
	return res, nil
}
