package services

import (
	"context"
	"strings"
	"testing"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// Панель контура выдачи: режим префикса, словарь подсказок, маржа как разрешение равенства,
// доли категорий, бейджи, флаги и рекомендации.

func TestNormalizePrefixCandidates(t *testing.T) {
	lx, rules, _ := mockLexicon(t)
	// Раскладка — дополнительный кандидат, набранное остаётся первым.
	pq := normalizePrefix("лфызукылн", lx, rules)
	if pq.cands[0].kind != prefixTyped || pq.cands[0].text != "лфызукылн" {
		t.Fatalf("первым — как набрано: %+v", pq.cands)
	}
	found := false
	for _, c := range pq.cands {
		found = found || c.kind == prefixLayout && c.text == "kaspersky"
	}
	if !found {
		t.Fatalf("нет кандидата раскладки: %+v", pq.cands)
	}
	// Недописанное последнее слово в другой раскладке — тоже кандидат («лфы» → kas…).
	pq = normalizePrefix("лфыз", lx, rules)
	if len(pq.cands) < 2 || pq.cands[1].text != "kasp" {
		t.Fatalf("префикс в другой раскладке: %+v", pq.cands)
	}
	// Опечатка — не в последнем (недописанном) слове.
	pq = normalizePrefix("касперкий", lx, rules)
	if _, ok := pq.typo(lx, rules); ok {
		t.Fatal("опечатка в недописанном слове не исправляется")
	}
	pq = normalizePrefix("касперкий ", lx, rules)
	c, ok := pq.typo(lx, rules)
	if !ok || c.text != "касперский" {
		t.Fatalf("опечатка в законченном слове: %+v %v", c, ok)
	}
	// Код: бережная канонизация.
	if pq = normalizePrefix("kw9-00664", lx, rules); pq.code != "KW900664" {
		t.Fatalf("код: %q", pq.code)
	}
	if pids := lx.codes().exact(pq.code); len(pids) != 1 || pids[0] != "w11h-esd" {
		t.Fatalf("точный артикул: %v", pids)
	}
	if pids := lx.codes().prefix("KW9", 10); len(pids) < 2 {
		t.Fatalf("префикс артикула: %v", pids)
	}
}

func dictRows(rows ...domain.Suggestion) []domain.Suggestion {
	for i := range rows {
		rows[i].Norm = foldText(rows[i].Text)
		if rows[i].Kind == "" {
			rows[i].Kind = suggestQuery
		}
		rows[i].Hits = max(rows[i].Hits, 1)
	}
	return rows
}

func TestSuggestDictMatch(t *testing.T) {
	d := newSuggestDict(dictRows(
		domain.Suggestion{Text: "kaspersky endpoint security", Weight: 0.1},
		domain.Suggestion{Text: "kaspersky", Weight: 0.3},
		domain.Suggestion{Text: "Kaspersky Plus", Weight: 0.2},
		domain.Suggestion{Text: "kaspersky", Weight: 0.05}, // дубль после нормализации
	), nil, 1, "v")
	if len(d.entries) != 3 {
		t.Fatalf("дубли: %d", len(d.entries))
	}
	hs := d.match("kasp", 10)
	if len(hs) != 3 || d.entries[hs[0].entry].text != "kaspersky" {
		t.Fatalf("порядок по весу: %+v", hs)
	}
	// Начало слова внутри строки — тоже совпадение, но ниже начала строки.
	hs = d.match("endp", 10)
	if len(hs) != 1 || hs[0].pos != 10 {
		t.Fatalf("начало слова: %+v", hs)
	}
	if hs := d.match("spers", 10); len(hs) != 0 {
		t.Fatalf("середина слова не совпадает: %+v", hs)
	}
	var nd *suggestDict
	if nd.match("k", 1) != nil {
		t.Fatal("пустой словарь")
	}
}

func TestMarginTieBreak(t *testing.T) {
	fs := []instantFamily{
		{id: "exact", rel: 1, exact: true, margin: 0},
		{id: "a", rel: 0.95, margin: 0.1},
		{id: "b", rel: 0.93, margin: 0.9}, // почти равен a и с большей маржой — поднимается на одну
		{id: "c", rel: 0.92, margin: 0.99},
		{id: "d", rel: 0.5, margin: 1}, // далеко от лучшего — не двигается
		{id: "e", rel: 0.49, margin: 0},
	}
	got, shifts := marginTieBreak(append([]instantFamily(nil), fs...))
	order := []string{}
	for _, f := range got {
		order = append(order, f.id)
	}
	if strings.Join(order, ",") != "exact,b,a,c,d,e" {
		t.Fatalf("порядок: %v", order)
	}
	for _, s := range shifts {
		if d := s.From - s.To; d > 1 || d < -1 {
			t.Fatalf("сдвиг больше одной позиции: %+v", s)
		}
	}
}

func TestPanelCategoriesShares(t *testing.T) {
	lx := &searchLexicon{categoryName: map[string]string{"av": "Антивирусы", "os": "ОС"}}
	fam := func(cat string, rel float64) instantFamily {
		return instantFamily{head: indexDoc{CategoryID: cat}, rel: rel}
	}
	one := panelCategories(lx, &instantResult{families: []instantFamily{fam("av", 1), fam("av", 1), fam("os", 0.5)}})
	if len(one) != 1 || one[0].ID != "av" || one[0].Share != 0.8 {
		t.Fatalf("одна уверенная: %+v", one)
	}
	two := panelCategories(lx, &instantResult{families: []instantFamily{fam("av", 1), fam("os", 1)}})
	if len(two) != 2 {
		t.Fatalf("две сильные: %+v", two)
	}
	blurred := panelCategories(lx, &instantResult{families: []instantFamily{fam("av", 1), fam("os", 1), fam("x", 1), fam("y", 1)}})
	if len(blurred) != 0 {
		t.Fatalf("размыто — пусто: %+v", blurred)
	}
}

func TestRefinementsFor(t *testing.T) {
	m12, m24 := 12, 24
	q3, q5 := 3, 5
	dev := domain.MetricDevice
	lx := &searchLexicon{categoryName: map[string]string{"home": "Антивирусы для дома", "biz": "Антивирусная защита для бизнеса"}}
	docs := []indexDoc{
		{FamilyID: "a", CategoryID: "home", LicenseType: domain.LicenseSubscription, LicenseDuration: &m12, LicenseMetric: &dev, LicenseQuantity: &q3},
		{FamilyID: "a", CategoryID: "home", LicenseType: domain.LicenseRenewal, LicenseDuration: &m24, LicenseMetric: &dev, LicenseQuantity: &q5},
		{FamilyID: "b", CategoryID: "home", LicenseType: domain.LicenseSubscription, LicenseDuration: &m12, LicenseMetric: &dev, LicenseQuantity: &q5},
		{FamilyID: "c", CategoryID: "biz", LicenseType: domain.LicenseSubscription, LicenseDuration: &m12, LicenseMetric: &dev, LicenseQuantity: &q5, InReestr: true},
	}
	top := &dictEntry{text: "kaspersky", norm: "kaspersky", next: []nextToken{
		{Token: "premium", Hits: 1}, {Token: "на 5 пк", Hits: 3}, {Token: "пусто", Hits: 0}}}
	res := refinementsFor(refineCtx{lx: lx, query: "kasp"}, top, docs, []string{"kaspersky premium"})
	labels := map[string]out.Refinement{}
	for i, r := range res {
		labels[r.Label] = r
		if r.Field == nil && i < len(res)-1 && res[i+1].Field != nil {
			t.Fatalf("продолжения подсказки — после структурированных: %+v", res)
		}
	}
	if r, ok := labels["для бизнеса"]; !ok || *r.Field != facetCategory || *r.Value != "biz" || r.Hits != 1 {
		t.Fatalf("категория — «для бизнеса»: %+v", res)
	}
	if r, ok := labels["продление"]; !ok || *r.Field != refFieldType || *r.Value != domain.LicenseRenewal || r.Hits != 1 {
		t.Fatalf("структурированный бейдж: %+v", res)
	}
	if r, ok := labels["3 устройства"]; !ok || *r.Field != facetQuantity || *r.Value != "3" {
		t.Fatalf("число мест с метрикой: %+v", res)
	}
	if r, ok := labels["отечественное"]; !ok || *r.Field != facetReestr || *r.Value != "true" {
		t.Fatalf("реестр: %+v", res)
	}
	if _, ok := labels["premium"]; ok {
		t.Fatal("продолжение, которое уже есть в подсказках, показано")
	}
	if _, ok := labels["пусто"]; ok {
		t.Fatal("бейдж без результатов показан")
	}
	// Метрика, подписка, 5 устройств и 1 год есть у всех семейств — ничего не уточняют.
	for _, l := range []string{"на устройство", "подписка", "5 устройств", "1 год"} {
		if _, ok := labels[l]; ok {
			t.Fatalf("значение у всех семейств — не бейдж: %s", l)
		}
	}
	if len(res) > panelRefinements {
		t.Fatalf("больше шести: %d", len(res))
	}
	// Названное в запросе (и противоречащее ему) не предлагается.
	for _, r := range refinementsFor(refineCtx{lx: lx, query: "kaspersky продление на 3 устройства"}, nil, docs, nil) {
		if *r.Field == refFieldType || *r.Field == facetQuantity || *r.Field == facetMetric {
			t.Fatalf("названное в запросе предложено: %+v", r)
		}
	}
}

// panelEnv — фикстура с импортированным каталогом и словарём подсказок без сборки (словарь кладётся напрямую).
func panelEnv(t *testing.T) *testEnv {
	t.Helper()
	env := imported(t, false)
	env.svc.features = Features{Hardware: true, Companions: true, MarginPolicy: true, Solutions: true, ResultsV3: true,
		Panel: true, PanelProducts: true, PanelMargin: true, PanelRecs: true}
	st := env.svc.catalog.Load()
	ps := env.svc.panel()
	ps.mu.Lock()
	ps.dict = newSuggestDict(dictRows(
		domain.Suggestion{Text: "kaspersky internet security", Weight: 0.3, Source: domain.SuggestSourceTemplate},
		domain.Suggestion{Text: "kaspersky", Weight: 0.2, Source: domain.SuggestSourceQuery},
		domain.Suggestion{Text: "windows server", Weight: 0.1, Source: domain.SuggestSourceQuery},
	), st.lex, 7, st.version)
	ps.checkedAt = time.Now().Add(time.Hour)
	ps.mu.Unlock()
	return env
}

func TestSuggestPanelModesAndFlags(t *testing.T) {
	env := panelEnv(t)
	ctx := t.Context()
	get := func(ctx context.Context, q string) *out.SuggestResponse {
		t.Helper()
		r, err := env.svc.SuggestPanel(ctx, "01TEST", SuggestRequest{Q: q}, SuggestOptions{NoLog: true})
		if err != nil {
			t.Fatal(err)
		}
		return r
	}
	r := get(ctx, "kasp")
	if r.Mode != panelModePrefix || len(r.Suggestions) == 0 || r.Suggestions[0].Text != "kaspersky internet security" {
		t.Fatalf("подсказки: %+v", r.Suggestions)
	}
	if h := r.Suggestions[0].Highlight; len(h) != 1 || h[0] != [2]int{0, 4} {
		t.Fatalf("выделение: %+v", h)
	}
	if r.Meta.LLMCalls+r.Meta.EmbedCalls+r.Meta.RerankCalls != 0 || r.Meta.SuggestionsVersion != 7 {
		t.Fatalf("meta: %+v", r.Meta)
	}
	if len(r.Groups) == 0 {
		t.Fatal("прежние groups пропали")
	}
	// Точный артикул: один товар, без подсказок и бейджей.
	r = get(ctx, "KL1939RDBFS")
	if len(r.Products) != 1 || !r.Products[0].SKUExact || r.Products[0].ProductID != "kis-2dev-1y" || len(r.Suggestions) != 0 {
		t.Fatalf("точный артикул: %+v", r.Products)
	}
	if r = get(ctx, ""); r.Mode != panelModeEmpty || len(r.Suggestions) == 0 || len(r.Products) != 0 {
		t.Fatalf("пустая строка: %+v", r)
	}
	if r = get(ctx, "k"); r.Mode != panelModeShort || len(r.Suggestions)+len(r.Products) != 0 {
		t.Fatalf("1 символ: %+v", r)
	}
	// Флаг панели выключен — поведение контура коммерции: только groups, новые секции пустые.
	off := WithFeatures(ctx, Features{Hardware: true, Companions: true, MarginPolicy: true, Solutions: true, ResultsV3: true})
	r = get(off, "kasp")
	if r.Mode != panelModeOff || len(r.Suggestions)+len(r.Products)+len(r.Refinements) != 0 || len(r.Groups) == 0 {
		t.Fatalf("флаг выключен: %+v", r)
	}
	// Товарная секция выключена — подсказки остаются.
	noProd := WithFeatures(ctx, Features{Panel: true, PanelMargin: true, PanelRecs: true, Companions: true, MarginPolicy: true})
	if r = get(noProd, "KL1939RDBFS"); len(r.Products) != 0 {
		t.Fatalf("товары при выключенном флаге: %+v", r.Products)
	}
	if _, err := env.svc.SuggestPanel(ctx, "", SuggestRequest{Q: "x", Limit: 7}, SuggestOptions{NoLog: true}); err == nil {
		t.Fatal("лимит больше шести принят")
	}
}

func TestSuggestRecommendationsLadder(t *testing.T) {
	env := panelEnv(t)
	ctx := t.Context()
	// Без оснований — null.
	if r, err := env.svc.SuggestRecommendations(ctx, RecsRequest{}); err != nil || r != nil {
		t.Fatalf("без оснований: %+v %v", r, err)
	}
	// Спецификация: сервер — проверенные дополнения, не позиции самой спецификации.
	r, err := env.svc.SuggestRecommendations(ctx, RecsRequest{Spec: []string{"ws2025-std-16core"}})
	if err != nil {
		t.Fatal(err)
	}
	if r != nil {
		if r.Basis != recsBasisSpec || r.Title != "К вашей спецификации" || len(r.Items) > recsMax {
			t.Fatalf("спецификация: %+v", r)
		}
		for _, it := range r.Items {
			if it.FamilyID == "windows-server-2025" || it.Compatibility == "unknown" || it.Compatibility == "incompatible" {
				t.Fatalf("непроверенное или из спецификации: %+v", it)
			}
		}
	}
	// Популярное в категории — под своим заголовком.
	st := env.svc.catalog.Load()
	cat := st.lex.docs["kis-2dev-1y"].CategoryID
	r, err = env.svc.SuggestRecommendations(ctx, RecsRequest{Category: cat})
	if err != nil || r == nil || r.Basis != recsBasisCategory || r.Title != "Вам может быть интересно" {
		t.Fatalf("категория: %+v %v", r, err)
	}
	// Флаг рекомендаций выключен — null.
	off := WithFeatures(ctx, Features{Panel: true, Companions: true})
	if r, _ := env.svc.SuggestRecommendations(off, RecsRequest{Category: cat}); r != nil {
		t.Fatalf("флаг выключен: %+v", r)
	}
}
