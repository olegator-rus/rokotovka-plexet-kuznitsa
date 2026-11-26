package services

import (
	"fmt"
	"net/url"
	"regexp"
	"slices"
	"sort"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Вердикт запроса набора «по цели» (контур эталонов): чистые функции над ответом поиска и справочником
// позиций. Первая цель ответа — поле target, иначе первая карточка (URL семейства → оффера → вендора), как в
// scripts/eval/customer_eval.py. URL сравниваются после нормализации (targetURLKey).

const (
	storeBase      = "https://store.softline.ru"
	storeHost      = "store.softline.ru"
	targetTopCards = 10 // MRR@10 и место ожидаемой цели
	targetTopFour  = 4
	excludeDepth   = 20 // «исключённых семейств нет среди первых 20 карточек» (набор понимания)
)

// Источник первой цели ответа (TargetQueryResult.FirstSource).
const (
	firstFromTarget = "target"
	firstFromFamily = "family_url"
	firstFromOffer  = "offer_url"
	firstFromVendor = "vendor_url"
	firstNone       = "none"
)

var (
	urlIndexRe  = regexp.MustCompile(`\[\d+\]`)
	urlSchemeRe = regexp.MustCompile(`^[a-zA-Z][a-zA-Z0-9+.-]*://`)
)

// targetURLKey — каноническая форма URL магазина для сравнения с эталоном: относительный и абсолютный URL,
// регистр хоста и схемы, www, http и https у магазина, завершающий слэш, URL-кодирование (%5B%5D = []),
// filter[x][]= и filter[x][0]=, порядок параметров. Фрагмент отбрасывается. Пустое — "".
func targetURLKey(raw string) string {
	u := strings.TrimSpace(raw)
	if u == "" {
		return ""
	}
	switch {
	case strings.HasPrefix(u, "//"):
		u = "https:" + u
	case !urlSchemeRe.MatchString(u):
		if strings.HasPrefix(strings.ToLower(u), storeHost) {
			u = "https://" + u
		} else {
			if !strings.HasPrefix(u, "/") {
				u = "/" + u
			}
			u = storeBase + u
		}
	}
	p, err := url.Parse(u)
	if err != nil {
		return strings.ToLower(u)
	}
	scheme := strings.ToLower(p.Scheme)
	host := strings.TrimSuffix(strings.ToLower(p.Hostname()), ".")
	port := p.Port()
	if host == storeHost || host == "www."+storeHost {
		scheme, host, port = "https", storeHost, ""
	}
	if port != "" && !(scheme == "https" && port == "443" || scheme == "http" && port == "80") {
		host += ":" + port
	}
	path, err := url.PathUnescape(p.EscapedPath())
	if err != nil {
		path = p.Path
	}
	path = strings.TrimRight((&url.URL{Path: path}).EscapedPath(), "/")
	if path == "" {
		path = "/"
	}
	type kv struct{ k, v string }
	var params []kv
	for _, part := range strings.Split(p.RawQuery, "&") {
		if part == "" {
			continue
		}
		k, v, _ := strings.Cut(part, "=")
		if dk, err := url.QueryUnescape(k); err == nil {
			k = dk
		}
		if dv, err := url.QueryUnescape(v); err == nil {
			v = dv
		}
		params = append(params, kv{urlIndexRe.ReplaceAllString(k, "[]"), v})
	}
	sort.Slice(params, func(i, j int) bool {
		if params[i].k != params[j].k {
			return params[i].k < params[j].k
		}
		return params[i].v < params[j].v
	})
	q := make([]string, len(params))
	for i, x := range params {
		q[i] = strings.ReplaceAll(url.QueryEscape(x.k), "%5B%5D", "[]") + "=" + url.QueryEscape(x.v)
	}
	res := scheme + "://" + host + path
	if len(q) > 0 {
		res += "?" + strings.Join(q, "&")
	}
	return res
}

// sameTargetURL — два URL ведут на одну страницу магазина.
func sameTargetURL(a, b string) bool {
	ka := targetURLKey(a)
	return ka != "" && ka == targetURLKey(b)
}

// vendorPage — страница вендора на сайте.
func vendorPage(slug string) string { return storeBase + "/" + slug + "/" }

// vendorCategoryPage — вендор + категория второго уровня.
func vendorCategoryPage(slug, category string) string {
	return vendorPage(slug) + "?filter[category][]=" + category
}

// targetDoc — то, что вердикту нужно знать о позиции.
type targetDoc struct {
	Family, Vendor, Category, Kind string
	LicenseKind, OrgType           string
	Duration                       *int
	TierMin, TierMax               *int
	OfferURL                       string
}

// targetLookup — справочник позиций для вердикта (в сервисе — словарь каталога, в тестах — карта).
type targetLookup interface {
	doc(productID string) (targetDoc, bool)
	familyURL(familyID string) string
	categoriesNamed(name string) []string
}

// firstTarget — первая цель ответа: url, тип, источник, заголовок.
func firstTarget(resp *out.SearchResponse) (string, string, string, string) {
	if t := resp.Target; t != nil && (t.URL != "" || t.Type != "") {
		return t.URL, t.Type, firstFromTarget, t.Title
	}
	if len(resp.Items) == 0 {
		return "", "", firstNone, ""
	}
	it := resp.Items[0]
	switch {
	case it.FamilyURL != "":
		return it.FamilyURL, out.TargetFamily, firstFromFamily, it.Title
	case it.OfferURL != "":
		return it.OfferURL, out.TargetSKU, firstFromOffer, it.Title
	case it.VendorURL != "":
		return it.VendorURL, out.TargetVendor, firstFromVendor, it.Title
	}
	return "", "", firstNone, it.Title
}

// expectLine — ожидаемая цель коротко.
func expectLine(e domain.TargetExpect) string {
	var b strings.Builder
	b.WriteString(e.Type)
	switch e.Type {
	case domain.ExpectFamily:
		b.WriteString(" " + strings.Join(e.GroupIDs, ","))
	case domain.ExpectVendor:
		b.WriteString(" " + e.VendorSlug)
	case domain.ExpectVendorCategory:
		b.WriteString(" " + e.VendorSlug)
		if len(e.CategoryIDs) > 0 {
			b.WriteString("+" + strings.Join(e.CategoryIDs, ","))
		}
	case domain.ExpectCategory:
		b.WriteString(" " + strings.Join(e.CategoryIDs, ","))
	case domain.ExpectSKU:
		b.WriteString(" " + strings.Join(e.OfferIDs, ","))
	}
	if e.Top > 0 {
		fmt.Fprintf(&b, " (первые %d)", e.Top)
	}
	for _, a := range e.Also {
		b.WriteString(" | " + expectLine(a))
	}
	return b.String()
}

// judgeTarget — вердикт ответа на запрос набора «по цели».
func judgeTarget(q domain.TargetQuery, resp *out.SearchResponse, lk targetLookup) out.TargetQueryResult {
	r := out.TargetQueryResult{Group: q.Group, Vendor: q.Vendor, Kind: q.Kind, Distortion: q.Distortion,
		Expect: expectLine(q.Expect), ByCards: q.Expect.Top > 0}
	r.FirstURL, r.FirstType, r.FirstSource, r.FirstTitle = firstTarget(resp)
	if len(resp.Items) > 0 {
		r.FirstFamily = resp.Items[0].FamilyID
	}
	countCards(&r, resp)
	exps := append([]domain.TargetExpect{q.Expect}, q.Expect.Also...)
	for _, e := range exps {
		if expectStrict(e, resp, r, lk) {
			r.Strict = true
		}
		if rank := expectRank(e, resp, lk); rank > 0 && (r.Rank == 0 || rank < r.Rank) {
			r.Rank = rank
		}
	}
	if r.Strict {
		r.Rank = 1
	}
	r.Top4 = r.Strict || r.Rank > 0 && r.Rank <= targetTopFour
	r.Pass = r.Strict
	if !r.Strict {
		r.Fails = append(r.Fails, "target")
		r.MissReason = missReason(q.Expect, r)
	}
	e := q.Expect
	if len(e.Exclude) > 0 {
		for i, it := range resp.Items {
			if i >= excludeDepth {
				break
			}
			if slices.Contains(e.Exclude, it.FamilyID) {
				r.Pass = false
				r.Fails = append(r.Fails, "exclude")
				break
			}
		}
	}
	if e.AltMin > 0 && r.Alternatives < e.AltMin {
		r.Pass = false
		r.Fails = append(r.Fails, "alternatives")
	}
	if e.Normalized != "" && resp.Normalized != e.Normalized {
		r.Pass = false
		r.Fails = append(r.Fails, "normalized")
	}
	if e.RouteNot != "" && resp.Trace != nil && resp.Trace.Route == e.RouteNot {
		r.Pass = false
		r.Fails = append(r.Fails, "route")
	}
	if e.Hard != nil {
		r.HasHard = true
		r.Violations = hardViolations(*e.Hard, resp, lk)
		if r.Violations > 0 {
			r.Pass = false
			r.Fails = append(r.Fails, "hard")
		}
	}
	return r
}

// countCards — карточки, разные семейства и SKU, альтернативы с ценой.
func countCards(r *out.TargetQueryResult, resp *out.SearchResponse) {
	fams, skus := map[string]bool{}, map[string]bool{}
	for _, it := range resp.Items {
		if it.FamilyID != "" {
			fams[it.FamilyID] = true
		}
		if it.ProductID != "" {
			skus[it.ProductID] = true
		}
		for _, v := range it.Variants {
			if v.ProductID != "" {
				skus[v.ProductID] = true
			}
		}
		if it.MatchType == out.MatchAlternative && it.PriceState == out.PricePriced {
			r.Alternatives++
		}
	}
	r.Cards, r.Families, r.SKUs = len(resp.Items), len(fams), len(skus)
}

// expectStrict — первая цель ответа совпадает с ожидаемой (или, при Top > 0, первые N карточек подходят).
func expectStrict(e domain.TargetExpect, resp *out.SearchResponse, r out.TargetQueryResult, lk targetLookup) bool {
	if e.Top > 0 {
		return cardsMatch(e, resp, lk)
	}
	switch e.Type {
	case domain.ExpectFamily:
		for _, g := range e.GroupIDs {
			u := lk.familyURL(g)
			if e.URL != "" && len(e.GroupIDs) == 1 {
				u = e.URL
			}
			if u != "" && sameTargetURL(r.FirstURL, u) {
				return true
			}
			// Страница семейства не собрана: цель — само семейство (тип family или sku) первой карточкой.
			if lk.familyURL(g) == "" && (r.FirstType == out.TargetFamily || r.FirstType == out.TargetSKU) && r.FirstFamily == g {
				return true
			}
		}
	case domain.ExpectVendor:
		u := e.URL
		if u == "" {
			u = vendorPage(e.VendorSlug)
		}
		return sameTargetURL(r.FirstURL, u)
	case domain.ExpectVendorCategory:
		if len(e.CategoryIDs) == 0 {
			return vendorWithCategory(r.FirstURL, e.VendorSlug)
		}
		for _, c := range e.CategoryIDs {
			u := vendorCategoryPage(e.VendorSlug, c)
			if e.URL != "" && len(e.CategoryIDs) == 1 {
				u = e.URL
			}
			if sameTargetURL(r.FirstURL, u) {
				return true
			}
		}
		// Допустимые семейства линейки («kaspersky endpoint security» — любая из трёх редакций).
		for _, g := range e.GroupIDs {
			if u := lk.familyURL(g); u != "" && r.FirstType == out.TargetFamily && sameTargetURL(r.FirstURL, u) {
				return true
			}
		}
	case domain.ExpectCategory:
		if r.FirstType == out.TargetCategory && r.FirstSource == firstFromTarget {
			for _, id := range lk.categoriesNamed(r.FirstTitle) {
				if slices.Contains(e.CategoryIDs, id) {
					return true
				}
			}
			return false
		}
		// Конфигурации без поля target (BM25, B0–B2): категория первой карточки.
		if resp.Target == nil && len(resp.Items) > 0 {
			if d, ok := lk.doc(resp.Items[0].ProductID); ok {
				return slices.Contains(e.CategoryIDs, d.Category)
			}
		}
	case domain.ExpectSKU:
		for _, id := range e.OfferIDs {
			if d, ok := lk.doc(id); ok && d.OfferURL != "" && sameTargetURL(r.FirstURL, d.OfferURL) {
				return true
			}
		}
		if (resp.Target == nil || resp.Target.Type == out.TargetSKU) && len(resp.Items) > 0 {
			return slices.Contains(e.OfferIDs, resp.Items[0].ProductID)
		}
	case domain.ExpectNone:
		// Товарной цели нет: цели нет или она сервисная, и ни одна карточка не выдана за точное совпадение.
		if resp.Target != nil && resp.Target.Type != out.TargetService {
			return false
		}
		for _, it := range resp.Items {
			if it.MatchType == out.MatchExact {
				return false
			}
		}
		return true
	}
	return false
}

// vendorWithCategory — URL — страница вендора с фильтром категории (любой).
func vendorWithCategory(raw, slug string) bool {
	k := targetURLKey(raw)
	base := strings.TrimSuffix(targetURLKey(vendorPage(slug)), "/")
	return strings.HasPrefix(k, base+"?") && strings.Contains(k, "filter[category][]=")
}

// cardMatches — карточка относится к ожидаемой цели.
func cardMatches(e domain.TargetExpect, it out.SearchItem, lk targetLookup) bool {
	d, ok := lk.doc(it.ProductID)
	switch e.Type {
	case domain.ExpectFamily:
		return slices.Contains(e.GroupIDs, it.FamilyID)
	case domain.ExpectVendor:
		return ok && d.Vendor == e.VendorSlug
	case domain.ExpectVendorCategory:
		if slices.Contains(e.GroupIDs, it.FamilyID) {
			return true
		}
		return ok && d.Vendor == e.VendorSlug && (len(e.CategoryIDs) == 0 || slices.Contains(e.CategoryIDs, d.Category))
	case domain.ExpectCategory:
		return ok && slices.Contains(e.CategoryIDs, d.Category)
	case domain.ExpectSKU:
		if slices.Contains(e.OfferIDs, it.ProductID) {
			return true
		}
		return slices.ContainsFunc(it.Variants, func(v out.SearchItem) bool { return slices.Contains(e.OfferIDs, v.ProductID) })
	}
	return false
}

// cardsMatch — проверка по первым N карточкам: family/vendor/vendor_category/sku — хотя бы одна, category — все.
func cardsMatch(e domain.TargetExpect, resp *out.SearchResponse, lk targetLookup) bool {
	head := resp.Items
	if len(head) > e.Top {
		head = head[:e.Top]
	}
	if len(head) == 0 {
		return e.Type == domain.ExpectNone
	}
	if e.Type == domain.ExpectCategory {
		for _, it := range head {
			if !cardMatches(e, it, lk) {
				return false
			}
		}
		return true
	}
	return slices.ContainsFunc(head, func(it out.SearchItem) bool { return cardMatches(e, it, lk) })
}

// expectRank — место первой карточки ожидаемой цели в первых 10; 0 — нет.
func expectRank(e domain.TargetExpect, resp *out.SearchResponse, lk targetLookup) int {
	if e.Type == domain.ExpectNone {
		return 0
	}
	for i, it := range resp.Items {
		if i >= targetTopCards {
			break
		}
		if cardMatches(e, it, lk) {
			return i + 1
		}
	}
	return 0
}

// missReason — почему цель не совпала: нужное семейство первым, но ссылка не та («ссылка»), на месте N («ранг N»),
// нужного в выдаче нет («цель»).
func missReason(e domain.TargetExpect, r out.TargetQueryResult) string {
	switch {
	case r.Rank == 1:
		return "ссылка"
	case r.Rank > 1:
		return fmt.Sprintf("ранг %d", r.Rank)
	}
	return "цель"
}

// hardViolations — карточки (кроме «Популярного в наличии») в первых 10, нарушающие жёсткие условия запроса.
// Неизвестное у позиции (пустой вид лицензии, нет ступени) нарушением не считается; оборудование — тоже.
func hardViolations(h domain.HardExpect, resp *out.SearchResponse, lk targetLookup) int {
	n := 0
	for i, it := range resp.Items {
		if i >= targetTopCards {
			break
		}
		if it.MatchType == out.MatchFallback {
			continue
		}
		d, ok := lk.doc(it.ProductID)
		if !ok || d.Kind == "hardware" {
			continue
		}
		if violatesHard(h, d) {
			n++
		}
	}
	return n
}

func violatesHard(h domain.HardExpect, d targetDoc) bool {
	switch {
	case h.LicenseKind != "" && d.LicenseKind != "" && d.LicenseKind != h.LicenseKind:
		return true
	case h.OrgType != "" && d.OrgType != "" && d.OrgType != h.OrgType:
		return true
	case h.DurationMonths > 0 && d.Duration != nil && *d.Duration != h.DurationMonths:
		return true
	case h.Quantity > 0 && d.TierMin != nil && (h.Quantity < *d.TierMin || d.TierMax != nil && h.Quantity > *d.TierMax):
		return true
	}
	return false
}
