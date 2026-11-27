package services

import (
	"context"
	"sort"
	"strings"

	"kuznica/api/domain"
	"kuznica/api/in"
)

// Семь инструментов подбора (SPEC §4) поверх готового поиска и каталога. Инструменты исполняет код в
// фиксированной последовательности (CHECKS.md, отсечение); модель инструменты не выбирает. Инструментов,
// меняющих цену, назначающих скидку или отправляющих КП, нет.

// Имена инструментов (журнал хода).
const (
	toolSearchCatalog    = "search_catalog"
	toolGetFamilyOffers  = "get_family_offers"
	toolGetFacts         = "get_facts"
	toolValidateLicense  = "validate_license"
	toolFindAlternatives = "find_alternatives"
	toolGetRelated       = "get_related"
	toolUpdateSpec       = "update_spec"
)

// guideSearchFunc — поиск семейств (тесты подставляют свой).
type guideSearchFunc func(ctx context.Context, text string) ([]string, error)

// searchCatalog — search_catalog: семейства из обычного поиска (без LLM-разбора и без журнала). Без поиска —
// совпадение слов названия семейства в каталоге.
func (g *guideService) searchCatalog(ctx context.Context, cat *guideCatalog, text, vendor string) []string {
	var fams []string
	if g.searchFn != nil {
		fams, _ = g.searchFn(ctx, text)
	} else if g.s.search.Enabled() {
		// Подбору нужны семейства, а не тонкий порядок выдачи: без модели разбора и без реранкера (векторы нужны —
		// без них «криптопро csp для сервера» не находит CSP 5.0).
		sctx := withSearchModes(ctx, searchModes{noLLM: true, noRerank: true})
		res, err := g.s.SearchWith(sctx, "guide", in.SearchRequest{Query: text, Limit: 12}, SearchOptions{NoLog: true})
		if err == nil && res != nil {
			seen := map[string]bool{}
			for _, it := range res.Items {
				if it.FamilyID != "" && !seen[it.FamilyID] {
					seen[it.FamilyID] = true
					fams = append(fams, it.FamilyID)
				}
			}
		}
	}
	if len(fams) == 0 {
		fams = cat.nameMatch(text, vendor, 8)
	}
	if vendor != "" {
		kept := fams[:0:0]
		for _, f := range fams {
			if cat.famVend[f] == vendor {
				kept = append(kept, f)
			}
		}
		if len(kept) > 0 {
			fams = kept
		}
	}
	return fams
}

// nameMatch — семейства, в названии которых больше всего слов запроса (запасной путь без поиска).
func (c *guideCatalog) nameMatch(text, vendor string, limit int) []string {
	words := []string{}
	for _, w := range strings.Fields(guideNorm(text)) {
		if len([]rune(w)) >= 3 && !hwComputerWords[w] {
			words = append(words, stemRU(w))
		}
	}
	type sc struct {
		id    string
		score int
	}
	var all []sc
	for fid, name := range c.famName {
		if vendor != "" && c.famVend[fid] != vendor {
			continue
		}
		n := guideNorm(name)
		score := 0
		for _, w := range words {
			if strings.Contains(n, w) {
				score++
			}
		}
		if score > 0 {
			all = append(all, sc{fid, score*1000 - len(n)})
		}
	}
	sort.Slice(all, func(i, j int) bool {
		if all[i].score != all[j].score {
			return all[i].score > all[j].score
		}
		return all[i].id < all[j].id
	})
	out := []string{}
	for i := 0; i < len(all) && i < limit; i++ {
		out = append(out, all[i].id)
	}
	return out
}

// stemRU — грубая основа слова: без окончания (2 последние буквы у длинных слов).
func stemRU(w string) string {
	r := []rune(w)
	if len(r) > 6 {
		return string(r[:len(r)-2])
	}
	return w
}

// guideEditionStems — слова редакций и как их узнать в названии семейства.
var guideEditionStems = []struct{ ask, name string }{
	{"стандарт", "стандарт"}, {"расширен", "расширен"}, {"total", "total"}, {"тотал", "total"},
	{"плюс", "плюс"}, {"базов", "базов"}, {"корпоратив", "корпоратив"}, {"премиум", "премиум"},
	{"small office", "small office"}, {"ksos", "small office"}, {"cloud", "cloud"}, {"облачн", "cloud"},
}

// editionFamilies — семейства редакций, названных в вопросе «чем отличается A от B»: из линейки базы знаний, а
// без неё — среди семейств вендора с общим началом названия (KES Стандартный и KES Расширенный, а не «Kaspersky
// Стандартный для Linux»). Пусто — редакции не названы или не найдены.
//
// allowed — направление задачи без вендора («защита для компьютеров … чем расширенный отличается от стандартного»):
// только семейства направления, чтобы редакции резервного копирования не сравнивались вместо антивируса.
func (c *guideCatalog) editionFamilies(kb *guideKB, text, vendor string, candidates, allowed []string) []string {
	q := guideNorm(text)
	var asked []string
	for _, e := range guideEditionStems {
		if i := strings.Index(q, e.ask); i >= 0 && !containsStr(asked, e.name) {
			asked = append(asked, e.name)
		}
	}
	if len(asked) < 2 {
		return nil
	}
	if vendor == "" && len(candidates) > 0 {
		vendor = c.famVend[candidates[0]]
	}
	pool := map[string][]string{}
	for fid, name := range c.famName {
		if vendor != "" && c.famVend[fid] != vendor {
			continue
		}
		if len(allowed) > 0 && !containsStr(allowed, fid) {
			continue
		}
		n := guideNorm(name)
		for _, a := range asked {
			if !strings.Contains(n, a) {
				continue
			}
			// Редакция, о которой не спрашивали («Расширенный Плюс» при вопросе о Расширенном), — не та.
			extra := false
			for _, e := range guideEditionStems {
				if e.name != a && !containsStr(asked, e.name) && strings.Contains(n, e.name) {
					extra = true
				}
			}
			if !extra {
				pool[a] = append(pool[a], fid)
			}
		}
	}
	for _, a := range asked {
		if len(pool[a]) == 0 {
			return nil
		}
		sort.Strings(pool[a])
	}
	// Предпочесть семейства одной линейки базы знаний.
	if kb != nil && len(kb.lines) > 0 {
		lineOf := func(f string) string { return kb.lines[f].Line }
		for _, first := range pool[asked[0]] {
			line := lineOf(first)
			if line == "" {
				continue
			}
			pick := []string{first}
			for _, a := range asked[1:] {
				for _, f := range pool[a] {
					if lineOf(f) == line && !containsStr(pick, f) {
						pick = append(pick, f)
						break
					}
				}
			}
			if len(pick) == len(asked) {
				return pick
			}
		}
	}
	// Иначе — пара с самым длинным общим началом названия; при равенстве — больше базовых позиций.
	best, bestScore := []string(nil), -1
	for _, a0 := range pool[asked[0]] {
		pick := []string{a0}
		score := 0
		for _, a := range asked[1:] {
			bf, bs := "", -1
			for _, f := range pool[a] {
				if containsStr(pick, f) {
					continue
				}
				s := guideCommonPrefix(guideNorm(c.famName[a0]), guideNorm(c.famName[f]))*100 + c.baseOffers(f)
				if s > bs {
					bf, bs = f, s
				}
			}
			if bf == "" {
				pick = nil
				break
			}
			pick = append(pick, bf)
			score += bs
		}
		score += c.baseOffers(a0)
		if pick != nil && score > bestScore {
			best, bestScore = pick, score
		}
	}
	return best
}

// baseOffers — число базовых позиций семейства с ценой (до 99: вес при равенстве).
func (c *guideCatalog) baseOffers(fid string) int {
	n := 0
	for _, p := range c.byFamily[fid] {
		if p.License.Kind == domain.LicenseKindBase && priceKnown(p) {
			n++
		}
	}
	return min(n, 99)
}

func guideCommonPrefix(a, b string) int {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return n
}

// familyOffers — get_family_offers: позиции семейства с видом под операцию; срок — названный, а если
// покупатель срок не называл и в семействе нет позиций на 12 месяцев — любой.
func (c *guideCatalog) familyOffers(fid string, r domain.GuideRequirements, termOrigin string, kb *guideKB) ([]guideOfferView, guideTermPolicy) {
	var views []guideOfferView
	for _, p := range c.byFamily[fid] {
		v := c.view(p, kb)
		switch v.kind {
		case domain.LicenseKindSupport, domain.LicenseKindCourse, domain.LicenseKindOther, domain.LicenseKindModule:
			continue
		}
		if !v.priced() && !p.PriceOnRequest {
			continue // нет в наличии: вчерашнюю цену не показываем
		}
		if guideDistribution(p.Title) {
			continue // дистрибутив, установочный комплект — не право использования
		}
		views = append(views, v)
	}
	if termOrigin == originUser {
		return views, termExact
	}
	return views, termDefault
}

// factsFor — get_facts: факты семейств (все статусы; показ фильтрует проверка).
func (kb *guideKB) factsFor(fids []string, platform string) []*guideFact {
	var out []*guideFact
	for _, f := range fids {
		for _, fact := range kb.byFamily[f] {
			if fact.applies(platform) {
				out = append(out, fact)
			}
		}
	}
	return out
}

// factOn — факт оси у семейства (проверенный важнее черновика); nil — нет.
func (kb *guideKB) factOn(fid, axis, variant string) *guideFact {
	var draft *guideFact
	for _, f := range kb.byFamily[fid] {
		if f.Axis != axis || (variant != "" && f.Variant != "" && f.Variant != variant) {
			continue
		}
		if f.verified() {
			return f
		}
		if draft == nil {
			draft = f
		}
	}
	return draft
}

// findAlternatives — find_alternatives: покупаемые семейства той же категории (для цены по запросу).
func (c *guideCatalog) findAlternatives(fid string, limit int) []string {
	ps := c.byFamily[fid]
	if len(ps) == 0 {
		return nil
	}
	same := func(level func(p *domain.Product) string) []string {
		want := level(ps[0])
		var out []string
		if want == "" {
			return nil
		}
		for f, list := range c.byFamily {
			if f == fid || len(list) == 0 || level(list[0]) != want || c.famVend[f] == c.famVend[fid] {
				continue
			}
			if c.baseOffers(f) > 0 {
				out = append(out, f)
			}
		}
		sort.Strings(out)
		return out
	}
	out := same(func(p *domain.Product) string { return p.CategoryID })
	if len(out) == 0 {
		// Категория вся «по запросу» — тот же раздел уровнем выше.
		out = same(func(p *domain.Product) string {
			if n := len(p.CategoryPathIDs); n >= 2 {
				return p.CategoryPathIDs[n-2]
			}
			return ""
		})
	}
	if len(out) > limit {
		out = out[:limit]
	}
	return out
}

// guideDistribution — позиция-дистрибутив по названию.
func guideDistribution(title string) bool {
	t := strings.ToLower(title)
	return strings.Contains(t, "дистрибутив") || strings.Contains(t, "media pack") || strings.Contains(t, "установочный комплект") ||
		strings.Contains(t, "формуляр")
}
