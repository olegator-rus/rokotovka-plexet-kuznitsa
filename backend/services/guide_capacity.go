package services

import (
	"fmt"
	"sort"
	"strings"
	"time"

	"kuznica/api/out"
)

// Вместимость тарифа (D9, МТС Линк вебинары): число участников против предела тарифа из базы знаний. Тариф из фида
// с пределом меньше нужного — честно «меньше нужного»; тарифы вне фида (family_id null) — только во фразе и в
// запросе КП, офферов из них нет.

// guideCapacityAxis — ось вместимости вебинарной линейки.
const guideCapacityAxis = "webinar_participants"

// capacityLine — линейка вебинаров, если подбор про вебинар ("" — нет).
func (t *guideTurn) capacityLine(fams []string, src string) string {
	if t.st.Requirements.Task != "webinar" && !strings.Contains(guideNorm(src), "вебинар") {
		return ""
	}
	for _, f := range fams {
		if l := t.kb.lines[f].Line; l != "" && t.kb.factOn(f, guideCapacityAxis, "") != nil {
			return l
		}
	}
	for l, fs := range t.kb.offFeed {
		for _, f := range fs {
			if f.Axis == guideCapacityAxis {
				return l
			}
		}
	}
	return ""
}

// capacityTurn — тарифы линейки против числа участников.
func (t *guideTurn) capacityTurn(line string) *out.GuideResponse {
	t1 := time.Now()
	st := t.st
	st.Requirements.Task = "webinar"
	need := objectCount(st.Requirements, "user")
	res := &out.GuideResponse{Claims: []out.GuideClaim{}, Offers: []out.GuideOffer{}, Changes: t.changes}
	t.meta.Fallback = guideFbNoNeed
	// Тарифы в фиде.
	var famIDs []string
	for fid, l := range t.kb.lines {
		if l.Line == line && len(t.cat.byFamily[fid]) > 0 {
			famIDs = append(famIDs, fid)
		}
	}
	sort.Strings(famIDs)
	st.CandidateFamilyIDs = famIDs
	fits := 0
	for _, fid := range famIDs {
		f := t.kb.factOn(fid, guideCapacityAxis, "")
		limit := 0
		if f.verified() {
			limit = factLimit(f)
		}
		var best *guideOfferView
		for _, p := range t.cat.byFamily[fid] {
			v := t.cat.view(p, t.kb)
			if v.priced() && (v.kind == "base" || v.kind == "") && (best == nil || *p.PriceRub < *best.p.PriceRub) {
				vv := v
				best = &vv
			}
		}
		if best == nil {
			continue
		}
		o := t.offerOut(*best, 1, "", out.OfferMatch, "")
		o.Quantity, o.Total = 1, o.PricePerUnit
		switch {
		case limit == 0:
			o.Role, o.Note = out.OfferAlternative, "Вместимость не подтверждена"
			t.prelim = true
		case need > limit:
			o.Role, o.Note = out.OfferAlternative, fmt.Sprintf("До %d участников — меньше нужного", limit)
		default:
			o.Note = fmt.Sprintf("До %d участников", limit)
			fits++
		}
		if f.verified() {
			res.Claims = append(res.Claims, t.claimOf(f, t.cat.famName[fid]))
		}
		res.Offers = append(res.Offers, o)
	}
	t.step("Вместимость тарифов", toolGetFacts, fmt.Sprintf("тарифов в каталоге: %d", len(res.Offers)), t1)
	if fits > 0 {
		res.Type = out.GuideSelection
		res.Lead = "Подходит тариф из каталога."
		res.Next = t.nextSteps(res)
		return res
	}
	// Вне фида: наименьший тариф, вмещающий нужное число, — во фразу и в запрос КП.
	var offs []*guideFact
	for _, f := range t.kb.offFeed[line] {
		if f.Axis == guideCapacityAxis && f.verified() && factLimit(f) > 0 {
			offs = append(offs, f)
		}
	}
	sort.Slice(offs, func(i, j int) bool { return factLimit(offs[i]) < factLimit(offs[j]) })
	var pick *guideFact
	for _, f := range offs {
		if factLimit(f) >= need {
			pick = f
			break
		}
	}
	res.Type = out.GuideNoData
	t.gap("no_matching_offer")
	if pick == nil {
		res.Lead = "В каталоге нет тарифа на нужное число участников — уточним в запросе КП."
		res.Next = []string{"request_quote", "change_conditions"}
		return res
	}
	name := t.offFeedName(line, pick.Variant)
	_, capText := pick.factValue()
	if strings.Contains(string(pick.Value), "depends_on_config") {
		capText += ", зависит от конфигурации"
	}
	res.Lead = fmt.Sprintf("В каталоге нет тарифа на %d участников. У вендора это %s (%s) — только по запросу, добавьте в запрос КП.",
		need, name, capText)
	t.extraNums = append(t.extraNums, capText)
	res.Docs = &out.GuideDocs{Title: factSource(pick).Title, URL: pick.SourceURL}
	note := fmt.Sprintf("%s: нужно %d участников", name, need)
	// Прежняя запись этой линейки (до смены числа участников) заменяется.
	kept := st.QuoteNotes[:0:0]
	for _, n := range st.QuoteNotes {
		if !strings.Contains(n, ": нужно ") || !strings.HasSuffix(n, " участников") {
			kept = append(kept, n)
		}
	}
	st.QuoteNotes = append(kept, note)
	res.Next = []string{"request_quote", "change_conditions"}
	return res
}

// offFeedName — название тарифа вне фида по варианту (vendor_families.json), иначе сам вариант.
func (t *guideTurn) offFeedName(line, variant string) string {
	for _, n := range t.kb.offFeedNames[line] {
		if variant != "" && strings.Contains(n, variant) {
			return strings.TrimSpace(strings.Split(n, "(")[0])
		}
	}
	return variant
}
