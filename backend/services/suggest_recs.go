package services

import (
	"context"
	"fmt"
	"sort"
	"strings"

	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Блок рекомендаций панели — GET /api/suggest/recommendations (RESEARCH В). Считается на сессию, вне
// бюджета нажатия. Лестница оснований по убыванию доверия: позиции спецификации → точный SKU во вводе → недавно
// просмотренный (FEATURE_PANEL_VIEWED, по умолчанию выкл.) → популярное в уверенной категории. Для первых
// трёх — только проверенные связи контура коммерции (подбор блока «Дополните решение»), совместимость не unknown и не
// incompatible. Заголовок называет основание; без оснований — null (блок скрыт).

const (
	recsMax     = 3
	recsSpecMax = 10
)

// Основания блока.
const (
	recsBasisSpec     = "spec"
	recsBasisSKU      = "exact_sku"
	recsBasisViewed   = "viewed"
	recsBasisCategory = "category_popular"
)

// RecsRequest — параметры блока. Q — текущий ввод (основание «точный SKU»; аддитивно к §11.3).
type RecsRequest struct {
	SessionID, IntentID string
	Spec                []string
	Viewed, Category, Q string
	// Exact — точный SKU, выбранный панелью (product_id, §11.7); иначе основание ищется по артикулу в Q.
	Exact string
}

// SuggestRecommendations — блок рекомендаций; nil — оснований нет или флаг выключен.
func (s *Services) SuggestRecommendations(ctx context.Context, req RecsRequest) (*out.PanelRecommendations, error) {
	if len(req.Spec) > recsSpecMax {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{Field: "spec",
			Code: errs.CodeOutOfRange, Message: fmt.Sprintf("В спецификации — не больше %d позиций", recsSpecMax)})
	}
	for field, v := range map[string]string{"session_id": req.SessionID, "intent_id": req.IntentID, "viewed": req.Viewed,
		"category": req.Category, "exact": req.Exact} {
		if len(v) > maxIDLen {
			return nil, errs.NewValidationError(errs.MsgInvalidFields).WithDetails(errs.FieldError{Field: field,
				Code: errs.CodeOutOfRange, Message: "Слишком длинный идентификатор"})
		}
	}
	f := s.featuresFor(ctx)
	st := s.catalog.Load()
	cc := s.commerceFor(ctx)
	if !f.Panel || !f.PanelRecs || st == nil || st.lex == nil {
		return nil, nil
	}
	lx := st.lex
	if f.Companions && cc != nil {
		var spec []string
		inSpec := map[string]bool{}
		for _, pid := range req.Spec {
			if pid = strings.TrimSpace(pid); pid != "" && cc.products[pid] != nil && !inSpec[pid] {
				spec = append(spec, pid)
				inSpec[pid] = true
				inSpec["f:"+cc.products[pid].FamilyID] = true
			}
		}
		if len(spec) > 0 {
			if items := s.verifiedCompanions(ctx, spec, inSpec); len(items) > 0 {
				names := make([]string, 0, len(spec))
				for _, pid := range spec {
					names = append(names, cc.products[pid].FamilyName)
				}
				return &out.PanelRecommendations{Basis: recsBasisSpec, Title: "К вашей спецификации",
					Why: "Проверенные дополнения к позициям спецификации: " + strings.Join(uniqueStrings(names), ", "), Items: items}, nil
			}
		}
		var exact []string
		if _, ok := lx.docs[req.Exact]; ok {
			exact = []string{req.Exact}
		} else if code := normalizeMPN(strings.ReplaceAll(foldText(req.Q), " ", "")); len(code) >= 4 && hasDigit(code) {
			exact = lx.codes().exact(code)
		}
		if len(exact) > 0 {
			p := lx.docs[exact[0]]
			if items := s.verifiedCompanions(ctx, exact[:1], map[string]bool{exact[0]: true, "f:" + p.FamilyID: true}); len(items) > 0 {
				return &out.PanelRecommendations{Basis: recsBasisSKU, Title: "Дополните " + p.FamilyName,
					Why: "Введён точный артикул; позиции подобраны по проверенным связям и совместимости", Items: items}, nil
			}
		}
		if f.PanelViewed && req.Viewed != "" && cc.products[req.Viewed] != nil {
			p := cc.products[req.Viewed]
			if items := s.verifiedCompanions(ctx, []string{req.Viewed}, map[string]bool{req.Viewed: true, "f:" + p.FamilyID: true}); len(items) > 0 {
				return &out.PanelRecommendations{Basis: recsBasisViewed, Title: "К просмотренному: " + p.FamilyName,
					Why: "Вы смотрели эту позицию в этой сессии; дополнения — по проверенным связям", Items: items}, nil
			}
		}
	}
	if name := lx.categoryName[req.Category]; name != "" {
		if items := popularInCategory(lx, req.Category); len(items) > 0 {
			return &out.PanelRecommendations{Basis: recsBasisCategory, Title: "Популярное в категории " + name,
				Why: "Самые популярные позиции категории запроса; совместимость с вашим заказом не проверялась", Items: items}, nil
		}
	}
	return nil, nil
}

// verifiedCompanions — дополнения к якорям по правилам контура коммерции: совместимость verified или conditional, без
// позиций и семейств из exclude, по одному на семейство, не больше трёх.
func (s *Services) verifiedCompanions(ctx context.Context, anchors []string, exclude map[string]bool) []out.Companion {
	res := []out.Companion{}
	seen := map[string]bool{}
	for _, a := range anchors {
		required, block, _ := s.CompanionsForAnchor(ctx, a, 0, "")
		cands := append([]out.Companion{}, required...)
		if block != nil {
			cands = append(cands, block.Items...)
		}
		for _, c := range cands {
			if len(res) == recsMax {
				return res
			}
			if c.Compatibility == "unknown" || c.Compatibility == "incompatible" || c.Compatibility == "" {
				continue
			}
			if exclude[c.ProductID] || exclude["f:"+c.FamilyID] || seen[c.FamilyID] {
				continue
			}
			seen[c.FamilyID] = true
			res = append(res, c)
		}
	}
	return res
}

// popularInCategory — самые популярные позиции категории (и подкатегорий), по одной на семейство.
func popularInCategory(lx *searchLexicon, cat string) []out.Companion {
	var docs []indexDoc
	for _, d := range lx.docs {
		if d.CategoryID == cat || contains(d.CategoryIDs, cat) {
			docs = append(docs, d)
		}
	}
	sort.Slice(docs, func(i, j int) bool {
		if docs[i].Popularity != docs[j].Popularity {
			return docs[i].Popularity > docs[j].Popularity
		}
		return docs[i].ProductID < docs[j].ProductID
	})
	res := []out.Companion{}
	seen := map[string]bool{}
	for _, d := range docs {
		if len(res) == recsMax {
			break
		}
		if seen[d.FamilyID] {
			continue
		}
		seen[d.FamilyID] = true
		res = append(res, out.Companion{ProductID: d.ProductID, FamilyID: d.FamilyID, Title: d.Title, Vendor: d.Vendor,
			Category: d.Category, LicenseLine: licenseLine(d), PriceRub: d.PriceRub, PriceMode: docPriceMode(d),
			Class: "popular", Compatibility: "unknown", Reason: "Популярно в категории", Conditions: []string{},
			EvidenceLevel: "popularity", Source: "catalog:popularity", UpgradeAdds: []string{}})
	}
	return res
}

func uniqueStrings(xs []string) []string {
	var out []string
	for _, x := range xs {
		if !contains(out, x) {
			out = append(out, x)
		}
	}
	return out
}
