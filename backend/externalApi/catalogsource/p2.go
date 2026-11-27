package catalogsource

import (
	"fmt"
	"slices"
	"strings"

	"kuznica/api/domain"
)

// Поля и справочники контура коммерции: значения по умолчанию и проверка.
// Общие для MockJSON и Generic — обе вызывают validate.

// applyDefaults — необязательные поля контура коммерции: нет kind — software, нет price_mode — quote_required при
// price_on_request, иначе fixed; у связи нет requirement — optional, нет compatibility — unknown.
func applyDefaults(s *domain.CatalogSnapshot) {
	for i := range s.Products {
		p := &s.Products[i]
		p.Kind = strings.ToLower(strings.TrimSpace(p.Kind))
		if p.Kind == "" {
			p.Kind = domain.KindSoftware
		}
		p.MPN = strings.TrimSpace(p.MPN)
		p.PriceMode = strings.ToLower(strings.TrimSpace(p.PriceMode))
		if p.PriceMode == "" {
			p.PriceMode = domain.PriceFixed
			if p.PriceOnRequest {
				p.PriceMode = domain.PriceQuoteRequired
			}
		}
	}
	for i := range s.Relations {
		r := &s.Relations[i]
		if r.Requirement == "" {
			r.Requirement = domain.RequirementOptional
		}
		if r.Compatibility == "" {
			r.Compatibility = domain.CompatUnknown
		}
	}
}

// validateP2 — значения перечислений и уникальность id справочников контура коммерции. Неизвестный тип связи —
// ошибка: индекс строгий, а молча потерянная связь хуже отказа импорта. Ссылка правила в никуда —
// предупреждение (правило могут дописать раньше позиции).
func validateP2(s *domain.CatalogSnapshot, products map[string]bool) error {
	families := map[string]bool{}
	for _, p := range s.Products {
		families[p.FamilyID] = true
		if !slices.Contains([]string{domain.KindSoftware, domain.KindHardware}, p.Kind) {
			return fmt.Errorf("catalog.json: %q — неизвестный kind %q", p.ID, p.Kind)
		}
		if !slices.Contains(domain.PriceModes, p.PriceMode) {
			return fmt.Errorf("catalog.json: %q — неизвестный price_mode %q", p.ID, p.PriceMode)
		}
	}
	var dangling []string
	relIDs := map[string]bool{}
	for i, r := range s.Relations {
		where := fmt.Sprintf("relations.json: связь №%d", i+1)
		if r.RelID != "" {
			where = "relations.json: " + r.RelID
			if relIDs[r.RelID] {
				return fmt.Errorf("relations.json: id %q повторяется", r.RelID)
			}
			relIDs[r.RelID] = true
		}
		switch {
		case !slices.Contains(domain.RelationTypes, r.Type):
			return fmt.Errorf("%s — неизвестный type %q", where, r.Type)
		case !slices.Contains([]string{domain.RequirementConditional, domain.RequirementRecommended, domain.RequirementOptional}, r.Requirement):
			return fmt.Errorf("%s — неизвестный requirement %q", where, r.Requirement)
		case !slices.Contains([]string{domain.CompatVerified, domain.CompatConditional, domain.CompatIncompatible, domain.CompatUnknown}, r.Compatibility):
			return fmt.Errorf("%s — неизвестный compatibility %q", where, r.Compatibility)
		}
		for _, ref := range []*string{r.FromProduct, r.ToProduct} {
			if ref != nil && *ref != "" && !products[*ref] {
				dangling = append(dangling, where+" → позиция "+*ref)
			}
		}
	}
	seen := map[string]bool{}
	for _, lr := range s.LicenseRules {
		if strings.TrimSpace(lr.ID) == "" || seen["lr:"+lr.ID] {
			return fmt.Errorf("license_rules.json: пустой или повторный id %q", lr.ID)
		}
		seen["lr:"+lr.ID] = true
		if !families[lr.AnchorFamily] {
			dangling = append(dangling, "license_rules.json: "+lr.ID+" → семейство "+lr.AnchorFamily)
		}
	}
	for _, cr := range s.CompatibilityRules {
		if strings.TrimSpace(cr.ID) == "" || seen["cr:"+cr.ID] {
			return fmt.Errorf("compatibility_rules.json: пустой или повторный id %q", cr.ID)
		}
		seen["cr:"+cr.ID] = true
		if !slices.Contains([]string{domain.CompatVerified, domain.CompatConditional, domain.CompatIncompatible}, cr.State) {
			return fmt.Errorf("compatibility_rules.json: %s — неизвестный state %q", cr.ID, cr.State)
		}
		if (cr.ProductID == "") == (cr.FamilyID == "") {
			return fmt.Errorf("compatibility_rules.json: %s — нужен ровно один из product_id и family_id", cr.ID)
		}
		if (cr.ProductID != "" && !products[cr.ProductID]) || (cr.FamilyID != "" && !families[cr.FamilyID]) {
			dangling = append(dangling, "compatibility_rules.json: "+cr.ID+" → "+cr.ProductID+cr.FamilyID)
		}
	}
	for _, ad := range s.AttributeDefinitions {
		if strings.TrimSpace(ad.CategoryID) == "" || seen["ad:"+ad.CategoryID] {
			return fmt.Errorf("attribute_definitions.json: пустая или повторная category_id %q", ad.CategoryID)
		}
		seen["ad:"+ad.CategoryID] = true
	}
	if len(dangling) > 0 {
		head := dangling
		if len(head) > 5 {
			head = head[:5]
		}
		s.Warnings = append(s.Warnings, fmt.Sprintf("правила контура коммерции со ссылкой в никуда: %d (%s)", len(dangling), strings.Join(head, "; ")))
	}
	return nil
}
