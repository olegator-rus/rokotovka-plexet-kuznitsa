package services

import (
	"context"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Блок «Допродажи» аналитики по ролям (контур коммерции):
//   - ecom — покрытие блока (прогон лестницы p2), показы, присоединение, добавления на поиск, отказы и удаления
//     (события стенда); список якорей размеченных пар, у которых блок пуст;
//   - product — категории без утверждённых дополнений, пары из покупок без связи (находки агента), связи на проверке;
//   - content — связи без основания или уровня доказательности, оборудование без парт-номера, комплектующие без
//     правил совместимости;
//   - head — модель эффекта пакета (/api/effect/commerce), маржа на поиск (синтетическая), доля КП с дополнениями.
// Непосчитанное — value null с причиной в note. Сбой блока не ломает аналитику: причина в missing.

const commerceListMax = 50

// analyticsCommerce — блок роли.
func (s *Services) analyticsCommerce(ctx context.Context, role string, days int, since time.Time, searches int) *out.AnalyticsCommerce {
	res := &out.AnalyticsCommerce{Role: role, Title: "Допродажи",
		Note:      "Поведенческие числа — события стенда, не трафик магазина",
		Synthetic: s.marginSynthetic(), Tiles: []out.CommerceTile{}, Lists: []out.CommerceList{}, Missing: []string{}}
	cc := s.commerceFor(ctx)
	if cc == nil {
		res.Missing = append(res.Missing, "справочники контура коммерции не загружены (каталог без импорта)")
		return res
	}
	switch role {
	case "ecom":
		s.commerceEcom(ctx, res, cc, since, searches)
	case "product":
		s.commerceProduct(ctx, res, cc)
	case "content":
		commerceContent(res, cc)
	case "head":
		s.commerceHead(ctx, res, days, since, searches)
	}
	return res
}

func tile(key, label, unit, source, note string, v *float64) out.CommerceTile {
	return out.CommerceTile{Key: key, Label: label, Unit: unit, Source: source, Note: note, Value: v}
}

func countPtr(n int) *float64 { return evalPtr(float64(n)) }

func capRows(rows []out.CommerceListRow) []out.CommerceListRow {
	if len(rows) > commerceListMax {
		return rows[:commerceListMax]
	}
	if rows == nil {
		return []out.CommerceListRow{}
	}
	return rows
}

// commerceEcom — менеджер e-com.
func (s *Services) commerceEcom(ctx context.Context, res *out.AnalyticsCommerce, cc *commerceCatalog, since time.Time, searches int) {
	cov := tile("block_coverage", "Покрытие блока «Дополните решение»", "share", "offline_eval", "", nil)
	if ev, err := s.CommerceEvaluation(ctx, domain.EvalSetP2); err == nil {
		for _, c := range ev.Ladder {
			if (c.Config == "C4" || c.Config == "C2") && c.Metrics.Coverage != nil {
				cov.Value = c.Metrics.Coverage
				cov.Note = fmt.Sprintf("прогон лестницы %s, %s: доля якорей размеченных пар с непустым блоком", ev.RunID, c.Config)
			}
		}
	}
	if cov.Value == nil {
		cov.Note = "нет прогона коммерческой лестницы — make eval SET=p2"
	}
	cov.List = "empty_block"
	res.Tiles = append(res.Tiles, cov)

	// Якоря с пустым блоком — тем же кодом, что блок в поиске (CompanionsForAnchor), по размеченным парам.
	seen := map[string]bool{}
	var rows []out.CommerceListRow
	anchors := 0
	for _, p := range s.companionPairs(ctx) {
		if seen[p.AnchorProduct] || !expectsBlock[p.Class] {
			continue
		}
		seen[p.AnchorProduct] = true
		anchors++
		req, block, _ := s.CompanionsForAnchor(ctx, p.AnchorProduct, 0, "")
		if len(req) > 0 || block != nil && len(block.Items) > 0 {
			continue
		}
		title := p.AnchorProduct
		if pr := cc.products[p.AnchorProduct]; pr != nil {
			title = pr.Title
		}
		rows = append(rows, out.CommerceListRow{ID: p.AnchorProduct, Title: title, Detail: "блок пуст", Query: p.Query})
	}
	res.Lists = append(res.Lists, out.CommerceList{Key: "empty_block",
		Label: fmt.Sprintf("Якоря размеченных пар, у которых блок пуст (из %d)", anchors), Total: len(rows), Items: capRows(rows)})

	ev := map[string]int{}
	evSess := map[string]int{}
	if s.repos.Enabled() {
		if counts, err := s.repos.CommerceEventsSince(ctx, since); err == nil {
			for _, c := range counts {
				ev[c.Type], evSess[c.Type] = c.Events, c.Sessions
			}
		} else {
			res.Missing = append(res.Missing, "события блока не прочитаны: "+shortText(err.Error(), 120))
		}
	}
	const src, note = "stand_events", "события стенда"
	res.Tiles = append(res.Tiles, tile("impressions", "Показы блока (сессии)", "count", src, note, countPtr(evSess["companion_impression"])))
	attach := tile("attach_rate", "Доля присоединения", "share", src, "", nil)
	if n := evSess["companion_impression"]; n > 0 {
		attach.Value = evalPtr(evalRound4(float64(evSess["companion_add"]) / float64(n)))
		attach.Note = fmt.Sprintf("события стенда: сессии с добавлением из блока / с показом (%d / %d)", evSess["companion_add"], n)
	} else {
		attach.Note = "показов блока на стенде нет — не посчитано"
	}
	res.Tiles = append(res.Tiles, attach)
	perSearch := tile("adds_per_search", "Добавления из блока на поиск", "share", src, "", nil)
	if searches > 0 {
		perSearch.Value = evalPtr(evalRound4(float64(ev["companion_add"]) / float64(searches)))
		perSearch.Note = fmt.Sprintf("события стенда: добавлений %d на %d поисков", ev["companion_add"], searches)
	} else {
		perSearch.Note = "поисков за период нет"
	}
	res.Tiles = append(res.Tiles, perSearch,
		tile("dismissed", "Отказы «не нужно»", "count", src, note, countPtr(ev["companion_dismiss"])),
		tile("spec_removed", "Удаления позиций из спецификации", "count", src, note, countPtr(ev["spec_remove"])))
}

// complementTypes — связи, которые дают дополнение в блоке.
var complementTypes = map[string]bool{"complement": true, "compatibility": true, "renewal": true}

// commerceProduct — продукт-менеджер.
func (s *Services) commerceProduct(ctx context.Context, res *out.AnalyticsCommerce, cc *commerceCatalog) {
	famCat := map[string]string{}
	catProducts := map[string]int{}
	for _, p := range cc.products {
		famCat[p.FamilyID] = p.CategoryID
		catProducts[p.CategoryID]++
	}
	withComp := map[string]bool{}
	pending := 0
	for _, r := range cc.relations {
		if r.ReviewStatus == "pending" {
			pending++
		}
		if r.ReviewStatus != "" && r.ReviewStatus != "approved" {
			continue
		}
		if complementTypes[r.Type] || r.Requirement == "conditional_required" {
			withComp[famCat[r.FromFamily]] = true
		}
	}
	var rows []out.CommerceListRow
	for cat, n := range catProducts {
		if !withComp[cat] {
			rows = append(rows, out.CommerceListRow{ID: cat, Title: s.categoryName(cat), Detail: fmt.Sprintf("позиций: %d", n)})
		}
	}
	sort.Slice(rows, func(i, j int) bool { return rows[i].Title < rows[j].Title })
	res.Tiles = append(res.Tiles, out.CommerceTile{Key: "types_without_complements", Label: "Типы товаров без утверждённых дополнений",
		Value: countPtr(len(rows)), Unit: "count", Source: "catalog",
		Note: fmt.Sprintf("категории, ни у одного семейства которых нет утверждённой связи-дополнения (из %d)", len(catProducts)),
		List: "types_without_complements"})
	res.Lists = append(res.Lists, out.CommerceList{Key: "types_without_complements", Label: "Категории без утверждённых дополнений",
		Total: len(rows), Items: capRows(rows)})

	pairTile := out.CommerceTile{Key: "copurchase_without_relation", Label: "Пары из покупок без связи", Unit: "count",
		Source: "agent", List: "copurchase_without_relation"}
	var prow []out.CommerceListRow
	if st := s.store(); st != nil {
		if ps, err := st.ListProposals(ctx, "", ""); err == nil {
			for _, p := range ps {
				if p.Diagnosis != "MISSING_RELATION" && p.Diagnosis != "BUNDLE_OPPORTUNITY" {
					continue
				}
				prow = append(prow, out.CommerceListRow{ID: p.ID.String(), Title: p.ClusterLabel,
					Detail: fmt.Sprintf("%s · %s", p.Diagnosis, p.Status)})
			}
			pairTile.Value = countPtr(len(prow))
			pairTile.Note = "находки агента «Коммерция» по статистике покупок (MISSING_RELATION, BUNDLE_OPPORTUNITY); заказы mock синтетические"
		}
	}
	if pairTile.Value == nil {
		pairTile.Note = "очередь агента недоступна — не посчитано"
	}
	res.Tiles = append(res.Tiles, pairTile, out.CommerceTile{Key: "relations_pending", Label: "Связи на проверке",
		Value: countPtr(pending), Unit: "count", Source: "catalog", Note: "review_status = pending в действующем справочнике"})
	res.Lists = append(res.Lists, out.CommerceList{Key: "copurchase_without_relation", Label: "Находки агента по покупкам",
		Total: len(prow), Items: capRows(prow)})
}

// commerceContent — контент-менеджер.
func commerceContent(res *out.AnalyticsCommerce, cc *commerceCatalog) {
	var noBasis []out.CommerceListRow
	for _, r := range cc.relations {
		var why []string
		if r.Source == "" {
			why = append(why, "нет источника")
		}
		if r.EvidenceLevel == "" || r.EvidenceLevel == "E0" {
			why = append(why, "нет уровня доказательности")
		}
		if r.Source == "vendor_documentation" && r.SourceURL == "" {
			why = append(why, "нет ссылки на документацию")
		}
		if len(why) == 0 {
			continue
		}
		id := r.RelID
		if id == "" {
			id = fmt.Sprintf("p1:%s>%s:%s", r.FromFamily, r.ToFamily, r.Type)
		}
		noBasis = append(noBasis, out.CommerceListRow{ID: id, Title: r.FromFamily + " → " + r.ToFamily,
			Detail: r.Type + ": " + strings.Join(why, ", ")})
	}
	ruled := map[string]bool{}
	for _, cr := range cc.compatRules {
		ruled[cr.ProductID], ruled["fam:"+cr.FamilyID] = true, true
	}
	// Категории комплектующих — где у позиций есть compatible_with или правило совместимости (мониторы, ноутбуки и
	// другие самостоятельные устройства не в счёт).
	compCats := map[string]bool{}
	for id, p := range cc.products {
		if p.Kind == "hardware" && p.ModelID == "" && (len(p.CompatibleWith) > 0 || ruled[id] || ruled["fam:"+p.FamilyID]) {
			compCats[p.CategoryID] = true
		}
	}
	var noMPN, noCompat []out.CommerceListRow
	ids := make([]string, 0, len(cc.products))
	for id := range cc.products {
		ids = append(ids, id)
	}
	slices.Sort(ids)
	hw := 0
	for _, id := range ids {
		p := cc.products[id]
		if p.Kind != "hardware" {
			continue
		}
		hw++
		if strings.TrimSpace(p.MPN) == "" {
			noMPN = append(noMPN, out.CommerceListRow{ID: id, Title: p.Title, Detail: "нет парт-номера"})
		}
		if p.ModelID == "" && compCats[p.CategoryID] && !ruled[id] && !ruled["fam:"+p.FamilyID] {
			d := "нет правил совместимости"
			if len(p.CompatibleWith) == 0 {
				d += " и атрибута compatible_with"
			}
			noCompat = append(noCompat, out.CommerceListRow{ID: id, Title: p.Title, Detail: d})
		}
	}
	res.Tiles = append(res.Tiles,
		out.CommerceTile{Key: "relations_without_basis", Label: "Связи без основания или уровня доказательности",
			Value: countPtr(len(noBasis)), Unit: "count", Source: "catalog", Note: fmt.Sprintf("из %d связей справочника", len(cc.relations)),
			List: "relations_without_basis"},
		out.CommerceTile{Key: "hardware_without_mpn", Label: "Оборудование без парт-номера", Value: countPtr(len(noMPN)),
			Unit: "count", Source: "catalog", Note: fmt.Sprintf("из %d позиций оборудования", hw), List: "hardware_without_mpn"},
		out.CommerceTile{Key: "components_without_compat", Label: "Комплектующие без правил совместимости",
			Value: countPtr(len(noCompat)), Unit: "count", Source: "catalog",
			Note: "комплектующие (категории, где у позиций есть совместимость) без правила совместимости позиции или семейства",
			List: "components_without_compat"})
	res.Lists = append(res.Lists,
		out.CommerceList{Key: "relations_without_basis", Label: "Связи на заполнение основания", Total: len(noBasis), Items: capRows(noBasis)},
		out.CommerceList{Key: "hardware_without_mpn", Label: "Оборудование без парт-номера", Total: len(noMPN), Items: capRows(noMPN)},
		out.CommerceList{Key: "components_without_compat", Label: "Комплектующие без правил совместимости", Total: len(noCompat), Items: capRows(noCompat)})
}

// commerceHead — руководитель.
func (s *Services) commerceHead(ctx context.Context, res *out.AnalyticsCommerce, days int, since time.Time, searches int) {
	gp := tile("margin_per_search", "Маржа на поиск", "rub", "synthetic", "", nil)
	if s.repos.Enabled() && searches > 0 {
		if ordered, err := s.repos.OrderedProductsSince(ctx, since); err == nil {
			if cs := s.catalog.Load(); cs != nil && cs.lex != nil {
				sum, n := 0.0, 0
				for _, pc := range ordered {
					d, ok := cs.lex.docs[pc.ProductID]
					if !ok || d.PriceRub == nil || d.Margin <= 0 {
						continue
					}
					sum += *d.PriceRub * d.Margin * float64(pc.N)
					n += pc.N
				}
				gp.Value = evalPtr(float64(int(sum/float64(searches)*100+0.5)) / 100)
				gp.Note = fmt.Sprintf("валовая прибыль %d заказов периода (цена × маржа каталога, маржа синтетическая) / %d поисков; "+
					"безусловная — меряется только экспериментом", n, searches)
			}
		}
	}
	if gp.Value == nil {
		gp.Note = "нет поисков или заказов за период — не посчитано"
	}
	quotes := tile("quotes_with_companions", "Доля запросов КП с дополнениями", "share", "stand_events", "", nil)
	if s.repos.Enabled() {
		if qs, err := s.repos.QuoteStatsSince(ctx, since); err == nil && qs.Total > 0 {
			quotes.Value = evalPtr(evalRound4(float64(qs.WithCompanions) / float64(qs.Total)))
			quotes.Note = fmt.Sprintf("события стенда: КП с позицией из блока %d из %d", qs.WithCompanions, qs.Total)
		}
	}
	if quotes.Value == nil {
		quotes.Note = "запросов КП за период нет — не посчитано"
	}
	res.Tiles = append(res.Tiles, gp, quotes)
	eff, err := s.EffectCommerce(ctx, EffectCommerceRequest{Effect: EffectRequest{Days: days}})
	if err != nil {
		res.Missing = append(res.Missing, "модель эффекта не посчитана: "+shortText(err.Error(), 120))
		return
	}
	res.Effect = eff
	if base := slices.IndexFunc(eff.Scenarios, func(sc out.EffectCommerceScenario) bool { return sc.ID == "base" }); base >= 0 {
		res.Tiles = append(res.Tiles, out.CommerceTile{Key: "effect_base", Label: "Эффект пакета, базовый сценарий (ΔGP в месяц)",
			Value: evalPtr(eff.Scenarios[base].DeltaGPRub), Unit: "rub", Source: "synthetic",
			Note: "модель чувствительности, не прогноз; разложение — в effect", Link: "effect"})
	}
}
