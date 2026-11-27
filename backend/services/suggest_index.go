package services

import (
	"context"
	"encoding/json"
	"math"
	"regexp"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kuznica/api/domain"

	"go.uber.org/zap"
)

// Индекс подсказок контура выдачи (RESEARCH А «Запись подсказки»). Сборка — офлайн (`go run .
// suggest-build` или в памяти при первом обращении, если сборок в базе нет): успешные запросы РАННЕГО периода
// журнала (до отложенной недели агента) и шаблоны каталога (вендор, семейство, «семейство + редакция», категория).
// Вес = соответствие префиксу 50 % (на запросе) + частота 25 % + сглаженная успешность (c + m·p0)/(n + m) 15 % +
// свежесть 10 % — стартовые, гипотеза ресерча. Фильтры: число семейств по текстовой ветке ≥ suggestHitsMin, дубли
// после нормализации, персональные данные и мусор. На нажатие — двоичный поиск по словарю в памяти.

const (
	// suggestHitsMin — подсказка, по которой текстовая ветка не находит ни одного семейства, в индекс не идёт:
	// она привела бы в пустую выдачу.
	suggestHitsMin = 1
	// suggestSmoothM — сила сглаживания успешности: запрос с одним поиском и одним заказом не обгоняет запрос с
	// сотней поисков.
	suggestSmoothM = 5.0
	// suggestFreshDays — полураспад свежести, дней.
	suggestFreshDays = 14.0
	// suggestMaxWords, suggestMaxRunes — длинные запросы журнала подсказками не становятся.
	suggestMaxWords = 6
	suggestMaxRunes = 60
	// suggestSuccessWindow — действие после поиска в той же сессии в пределах окна делает поиск успешным.
	suggestSuccessWindow = 30 * time.Minute
	// suggestNextTokens — продолжений для бейджей на подсказку.
	suggestNextTokens = 6
	// suggestReloadEvery — как часто панель проверяет, не появилась ли новая сборка.
	suggestReloadEvery = 30 * time.Second
	// suggestBuildWorkers — параллельных запросов к OpenSearch при подсчёте hits.
	suggestBuildWorkers = 8
)

// Веса подсказки (стартовые, RESEARCH А).
const (
	suggestWPrefix  = 0.50
	suggestWFreq    = 0.25
	suggestWSuccess = 0.15
	suggestWFresh   = 0.10
)

// piiRe — признаки персональных данных и реквизитов: почта, телефон и длинные номера (ИНН, счёт), формы
// организаций с названием.
var piiRe = regexp.MustCompile(`@|\d[\d\s()+-]{6,}\d|(^|\s)(ооо|оао|зао|пао|ип|инн|огрн|тел|телефон)(\s|$)`)

// nextToken — продолжение подсказки для бейджа.
type nextToken struct {
	Token string `json:"token"`
	Hits  int    `json:"hits"`
}

// dictEntry — подсказка в памяти.
type dictEntry struct {
	text, norm, source, kind, categoryID string
	weight                               float64
	hits                                 int
	categories, vendors                  []string
	next                                 []nextToken
}

// suggestDict — словарь подсказок одной сборки: ключи — суффиксы нормализованного текста с начала каждого слова,
// отсортированные; префиксный поиск — двоичный поиск диапазона.
type suggestDict struct {
	version int // сборка в базе; 0 — собран в памяти
	catalog string
	lx      *searchLexicon
	builtAt time.Time
	entries []dictEntry
	keys    []dictKey
	byNorm  map[string]int
	popular []int // по весу: пустая строка
}

type dictKey struct {
	key   string
	entry int
	pos   int // начало слова в символах
}

// dictHit — найденная подсказка и её итоговый вес.
type dictHit struct {
	entry int
	pos   int
	score float64
}

// newSuggestDict — словарь из строк сборки.
func newSuggestDict(rows []domain.Suggestion, lx *searchLexicon, version int, catalog string) *suggestDict {
	d := &suggestDict{version: version, catalog: catalog, lx: lx, byNorm: map[string]int{}}
	for _, r := range rows {
		if _, dup := d.byNorm[r.Norm]; dup || r.Norm == "" {
			continue
		}
		e := dictEntry{text: r.Text, norm: r.Norm, source: r.Source, kind: r.Kind, weight: r.Weight, hits: r.Hits}
		_ = json.Unmarshal(r.Categories, &e.categories)
		_ = json.Unmarshal(r.Vendors, &e.vendors)
		_ = json.Unmarshal(r.NextTokens, &e.next)
		if e.kind == suggestCategory && len(e.categories) > 0 {
			e.categoryID = e.categories[0]
		}
		if r.BuiltAt.After(d.builtAt) {
			d.builtAt = r.BuiltAt
		}
		d.byNorm[e.norm] = len(d.entries)
		d.entries = append(d.entries, e)
	}
	for i, e := range d.entries {
		runes := 0
		prev := ' '
		for b, r := range e.norm {
			if prev == ' ' {
				d.keys = append(d.keys, dictKey{key: e.norm[b:], entry: i, pos: runes})
			}
			prev = r
			runes++
		}
		d.popular = append(d.popular, i)
	}
	sort.Slice(d.keys, func(i, j int) bool { return d.keys[i].key < d.keys[j].key })
	sort.SliceStable(d.popular, func(i, j int) bool {
		a, b := d.entries[d.popular[i]], d.entries[d.popular[j]]
		if a.weight != b.weight {
			return a.weight > b.weight
		}
		return a.norm < b.norm
	})
	return d
}

// match — подсказки, у которых text начинает строку или слово, по убыванию веса (не больше limit).
func (d *suggestDict) match(text string, limit int) []dictHit {
	return d.matchW(text, limit, false)
}

// matchW — match; anyWord — совпадение с начала любого слова весит как с начала строки (дополнение словаря:
// «вин» → «windows» — в «Microsoft Windows 11 Pro» покупатель набирал начало слова Windows, а не вендора).
func (d *suggestDict) matchW(text string, limit int, anyWord bool) []dictHit {
	if d == nil || text == "" {
		return nil
	}
	best := map[int]dictHit{}
	ql := float64(utf8.RuneCountInString(text))
	for i := sort.Search(len(d.keys), func(i int) bool { return d.keys[i].key >= text }); i < len(d.keys) && strings.HasPrefix(d.keys[i].key, text); i++ {
		k := d.keys[i]
		e := d.entries[k.entry]
		m := 0.7
		if k.pos == 0 || anyWord {
			m = 1
		}
		m *= 0.8 + 0.2*math.Min(1, ql/float64(utf8.RuneCountInString(e.norm)))
		sc := suggestWPrefix*m + e.weight
		if h, ok := best[k.entry]; !ok || sc > h.score {
			best[k.entry] = dictHit{entry: k.entry, pos: k.pos, score: sc}
		}
	}
	hits := make([]dictHit, 0, len(best))
	for _, h := range best {
		hits = append(hits, h)
	}
	sort.Slice(hits, func(i, j int) bool {
		a, b := hits[i], hits[j]
		if a.score != b.score {
			return a.score > b.score
		}
		if ea, eb := d.entries[a.entry], d.entries[b.entry]; ea.hits != eb.hits {
			return ea.hits > eb.hits
		} else {
			return ea.norm < eb.norm
		}
	})
	if len(hits) > limit {
		hits = hits[:limit]
	}
	return hits
}

// --- состояние панели на граф сервисов ---

// panelState — словарь подсказок, кэш товарной секции и версия индекса.
type panelState struct {
	mu        sync.Mutex
	dict      *suggestDict
	checkedAt time.Time
	building  bool
	products  *productCache
	idx       panelIndexVersion
}

type panelIndexVersion struct {
	builtAt time.Time
	version int
}

var panelStates sync.Map

func (s *Services) panel() *panelState {
	if v, ok := panelStates.Load(s); ok {
		return v.(*panelState)
	}
	v, _ := panelStates.LoadOrStore(s, &panelState{products: newProductCache(productCacheSize, productCacheTTL)})
	return v.(*panelState)
}

// suggestDictFor — действующий словарь для снимка каталога. Сборка в базе — по номеру (новая подхватывается не
// реже suggestReloadEvery, проверка — в фоне); сборок нет или база не подключена — словарь собирается в памяти в
// фоне, до готовности — nil (секция подсказок пуста, панель работает).
func (s *Services) suggestDictFor(lx *searchLexicon, catalog string) *suggestDict {
	ps := s.panel()
	ps.mu.Lock()
	defer ps.mu.Unlock()
	d := ps.dict
	stale := d == nil || d.lx != lx || d.catalog != catalog
	if (stale || time.Since(ps.checkedAt) > suggestReloadEvery) && !ps.building {
		ps.building = true
		ps.checkedAt = time.Now()
		s.writes.Add(1)
		go func() {
			defer s.writes.Done()
			s.reloadSuggestDict(lx, catalog, d)
		}()
	}
	if stale {
		if d != nil && d.catalog == catalog {
			return d // словарь прежнего снимка того же каталога лучше пустой секции, пока идёт пересборка
		}
		return nil
	}
	return d
}

// reloadSuggestDict — подхватить новую сборку из базы или собрать словарь в памяти.
func (s *Services) reloadSuggestDict(lx *searchLexicon, catalog string, cur *suggestDict) {
	ps := s.panel()
	defer func() {
		ps.mu.Lock()
		ps.building = false
		ps.mu.Unlock()
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Minute)
	defer cancel()
	var next *suggestDict
	if s.repos.Enabled() {
		v, err := s.repos.LatestSuggestionsVersion(ctx)
		if err != nil {
			zap.L().Warn("подсказки: версия сборки не прочитана", zap.Error(err))
		}
		if v > 0 && cur != nil && cur.version == v && cur.lx == lx {
			return
		}
		if v > 0 {
			rows, err := s.repos.SuggestionsOf(ctx, v)
			if err == nil && len(rows) > 0 && rows[0].CatalogVersion == catalog {
				next = newSuggestDict(rows, lx, v, catalog)
			} else if err != nil {
				zap.L().Warn("подсказки: сборка не прочитана", zap.Int("version", v), zap.Error(err))
			}
		}
	}
	if next == nil {
		if cur != nil && cur.version == 0 && cur.lx == lx {
			return // уже собран в памяти для этого снимка
		}
		rows, _, err := s.BuildSuggestions(ctx)
		if err != nil {
			zap.L().Warn("подсказки: словарь не собран", zap.Error(err))
			return
		}
		next = newSuggestDict(rows, lx, 0, catalog)
	}
	ps.mu.Lock()
	ps.dict = next
	ps.mu.Unlock()
	zap.L().Info("подсказки: словарь загружен", zap.Int("version", next.version), zap.Int("entries", len(next.entries)))
}

// --- сборка ---

// SuggestBuildInfo — итог сборки для CLI и записи в журнал.
type SuggestBuildInfo struct {
	Version       int `json:"version"`
	Rows          int `json:"rows"`
	FromQueries   int `json:"from_queries"`
	FromTemplates int `json:"from_templates"`
	// FromDictionary — подсказок из словаря понимания; FromJournal — из журнала текущего каталога в базе (фид).
	FromDictionary int       `json:"from_dictionary"`
	FromJournal    int       `json:"from_journal"`
	DroppedNoHits  int       `json:"dropped_no_hits"`
	DroppedPII     int       `json:"dropped_pii"`
	DroppedNoise   int       `json:"dropped_noise"`
	DroppedRare    int       `json:"dropped_rare"`
	Duplicates     int       `json:"duplicates"`
	EarlyBefore    time.Time `json:"early_before"`
	EarlyEvents    int       `json:"early_searches"`
	Ms             float64   `json:"ms"`
}

// logQuery — запрос журнала раннего периода.
type logQuery struct {
	text           string
	searches, succ int
	last           time.Time
}

// BuildSuggestions — строки индекса подсказок по каталогу и раннему периоду журнала (без записи в базу).
func (s *Services) BuildSuggestions(ctx context.Context) ([]domain.Suggestion, SuggestBuildInfo, error) {
	started := time.Now()
	info := SuggestBuildInfo{}
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return nil, info, errNoCatalog
	}
	lx := st.lex
	rules := s.rulesFor(ctx, domain.ConfigLive, lx, nil)
	queries, cutoff, p0 := s.earlyQueries(ctx, lx, rules, &info)
	if len(queries) == 0 {
		// Фид заказчика: файла журнала нет — успешные запросы журнала текущего каталога из базы (живые и модельный
		// поток, LogScope как у агента).
		queries, p0 = s.journalQueries(ctx, lx, rules, &info)
	}
	info.EarlyBefore = cutoff

	type cand struct {
		text, norm, source, kind, categoryID string
		lq                                   *logQuery
		pop                                  float64
		dict                                 bool
		dictFreq                             float64
	}
	var cands []cand
	seen := map[string]bool{}
	add := func(c cand) {
		c.norm = foldText(c.text)
		if c.norm == "" || utf8.RuneCountInString(c.norm) > suggestMaxRunes {
			return
		}
		if seen[c.norm] {
			info.Duplicates++
			return
		}
		seen[c.norm] = true
		cands = append(cands, c)
	}
	for _, q := range queries {
		add(cand{text: q.text, source: domain.SuggestSourceQuery, kind: suggestQuery, lq: q})
	}
	// Формы из словаря: так пишут покупатели («винда», «астра линукс», «касперский»). На одну каноническую форму —
	// одна форма ввода (не «винда, винду, винды»): совпадающая с канонической, иначе с пробелом, иначе короче; плюс
	// сама каноническая форма-термин («кибер бэкап», «microsoft 365»). Жаргон — ниже написания и ребрендинга.
	for _, dc := range dictionaryForms(s.lexicon(), lx.baseAliases) {
		add(cand{text: dc.text, source: domain.SuggestSourceDictionary, kind: suggestQuery, dict: true, dictFreq: dc.freq})
	}
	// Шаблоны каталога: короткая форма названия; вес — то, что можно купить, по ширине линейки (на фиде популярности
	// нет: у всех 0,5).
	buy, maxBuy := map[string]int{}, 1
	for _, f := range lx.familyOrder {
		for _, pid := range lx.families[f] {
			if docBuyPrice(lx.docs[pid]) != nil {
				buy[f]++
			}
		}
		maxBuy = max(maxBuy, buy[f])
	}
	breadth := func(n int) float64 {
		if n == 0 {
			return 0.05
		}
		return 0.2 + 0.6*math.Log1p(float64(n))/math.Log1p(float64(maxBuy))
	}
	for _, f := range lx.familyOrder {
		pop, editions := 0.0, map[string]bool{}
		for _, pid := range lx.families[f] {
			d := lx.docs[pid]
			pop = math.Max(pop, d.Popularity)
			if d.LicenseEdition != "" {
				editions[foldText(d.LicenseEdition)] = true
			}
		}
		if lx.docs[lx.families[f][0]].OfferURL != "" {
			pop = breadth(buy[f]) // фид: популярность у всех одинакова — ширина линейки в наличии
		}
		name := suggestDisplay(lx.familyName[f])
		add(cand{text: name, source: domain.SuggestSourceTemplate, kind: suggestQuery, pop: pop})
		if len(editions) > 1 {
			for _, ed := range sortedKeys(editions) {
				if !strings.Contains(foldText(name), ed) {
					add(cand{text: name + " " + ed, source: domain.SuggestSourceTemplate, kind: suggestQuery, pop: pop / 2})
				}
			}
		}
	}
	vendorFams := map[string]int{}
	for f, n := range buy {
		if n > 0 {
			vendorFams[lx.familyVendor[f]]++
		}
	}
	maxVF := 1
	for _, n := range vendorFams {
		maxVF = max(maxVF, n)
	}
	for _, id := range sortedKeys(lx.vendorName) {
		pop := 0.5
		if len(buy) > 0 && maxVF > 1 {
			pop = 0.2 + 0.6*math.Log1p(float64(vendorFams[id]))/math.Log1p(float64(maxVF))
		}
		add(cand{text: suggestDisplay(lx.vendorName[id]), source: domain.SuggestSourceTemplate, kind: suggestQuery, pop: pop})
	}
	for _, id := range sortedKeys(lx.categoryName) {
		add(cand{text: lx.categoryName[id], source: domain.SuggestSourceTemplate, kind: suggestCategory, categoryID: id, pop: 0.3})
	}

	// hits, категории и вендоры результатов — текстовой веткой, параллельно.
	type facts struct {
		hits       int
		cats, vend []string
	}
	res := make([]facts, len(cands))
	jobs := make(chan int)
	var wg sync.WaitGroup
	for w := 0; w < suggestBuildWorkers; w++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for i := range jobs {
				c := cands[i]
				if c.kind == suggestCategory {
					res[i] = facts{hits: categoryFamilies(lx, c.categoryID), cats: []string{c.categoryID}}
					continue
				}
				fams, err := s.textFamilies(ctx, lx, rules, c.norm)
				if err != nil {
					continue
				}
				res[i] = facts{hits: len(fams), cats: topShares(fams, lx, func(d indexDoc) string { return d.CategoryID }),
					vend: topShares(fams, lx, func(d indexDoc) string { return d.VendorID })}
			}
		}()
	}
	for i := range cands {
		jobs <- i
	}
	close(jobs)
	wg.Wait()
	if err := ctx.Err(); err != nil {
		return nil, info, err
	}

	maxN := 1
	for _, q := range queries {
		maxN = max(maxN, q.searches)
	}
	now := s.now()
	var rows []domain.Suggestion
	for i, c := range cands {
		f := res[i]
		if f.hits < suggestHitsMin {
			info.DroppedNoHits++
			continue
		}
		var freq, succ, fresh float64
		row := domain.Suggestion{Text: c.text, Norm: c.norm, Source: c.source, Kind: c.kind, Hits: f.hits,
			BuiltAt: now, CatalogVersion: st.version}
		if c.lq != nil {
			freq = math.Log1p(float64(c.lq.searches)) / math.Log1p(float64(maxN))
			succ = (float64(c.lq.succ) + suggestSmoothM*p0) / (float64(c.lq.searches) + suggestSmoothM)
			age := cutoff.Sub(c.lq.last).Hours() / 24
			fresh = math.Exp(-math.Ln2 * math.Max(0, age) / suggestFreshDays)
			row.Searches, row.Successes = c.lq.searches, c.lq.succ
			info.FromQueries++
		} else if c.dict {
			// Форма словаря: так пишут покупатели — выше шаблона каталога, ниже частых успешных запросов.
			freq, succ, fresh = c.dictFreq, p0, 0.5
			info.FromDictionary++
		} else {
			// Шаблон каталога: частота — половина популярности семейства, успешность — средняя по журналу,
			// свежесть — половина: шаблон не обгоняет запросы, которые реально ищут.
			freq, succ, fresh = c.pop/2, p0, 0.5
			info.FromTemplates++
		}
		row.Weight = round4(suggestWFreq*freq + suggestWSuccess*succ + suggestWFresh*fresh)
		row.Categories, _ = domain.ToJSONB(f.cats)
		row.Vendors, _ = domain.ToJSONB(f.vend)
		rows = append(rows, row)
	}
	// Продолжения: подсказки, которые начинаются с этой и пробела (проверены числом результатов при сборке).
	sort.Slice(rows, func(i, j int) bool { return rows[i].Norm < rows[j].Norm })
	norms := normsOf(rows)
	for i := range rows {
		var next []nextToken
		pref := rows[i].Norm + " "
		type nt struct {
			tok string
			w   float64
			h   int
		}
		var ns []nt
		seenTok := map[string]bool{}
		for j := sort.SearchStrings(norms, pref); j < len(rows) && strings.HasPrefix(rows[j].Norm, pref); j++ {
			rest := strings.Fields(strings.TrimPrefix(rows[j].Norm, pref))
			if len(rest) == 0 {
				continue
			}
			tok := rest[0]
			if len(rest) > 1 && lexStopWords[tok] {
				tok += " " + rest[1] // «для бизнеса», «на год»
				if len(rest) > 2 && hasDigit(rest[1]) {
					tok += " " + rest[2] // «на 5 пк»
				}
			}
			if seenTok[tok] {
				continue
			}
			seenTok[tok] = true
			ns = append(ns, nt{tok, rows[j].Weight, rows[j].Hits})
		}
		sort.SliceStable(ns, func(a, b int) bool { return ns[a].w > ns[b].w })
		for _, n := range ns {
			if len(next) == suggestNextTokens {
				break
			}
			next = append(next, nextToken{Token: n.tok, Hits: n.h})
		}
		rows[i].NextTokens, _ = domain.ToJSONB(next)
	}
	sort.SliceStable(rows, func(i, j int) bool {
		if rows[i].Weight != rows[j].Weight {
			return rows[i].Weight > rows[j].Weight
		}
		return rows[i].Norm < rows[j].Norm
	})
	info.Rows = len(rows)
	info.Ms = msSince(started)
	return rows, info, nil
}

func normsOf(rows []domain.Suggestion) []string {
	ns := make([]string, len(rows))
	for i, r := range rows {
		ns[i] = r.Norm
	}
	return ns
}

// SuggestBuild — `go run . suggest-build`: сборка и новая версия config_versions kind = suggestions.
func (s *Services) SuggestBuild(ctx context.Context) (SuggestBuildInfo, error) {
	rows, info, err := s.BuildSuggestions(ctx)
	if err != nil {
		return info, err
	}
	rules, _ := domain.ToJSONB(map[string]any{"rows": info.Rows, "weights": map[string]float64{
		"prefix": suggestWPrefix, "frequency": suggestWFreq, "success": suggestWSuccess, "freshness": suggestWFresh},
		"smooth_m": suggestSmoothM, "hits_min": suggestHitsMin, "early_before": info.EarlyBefore,
		"from_queries": info.FromQueries, "from_templates": info.FromTemplates, "dropped_no_hits": info.DroppedNoHits,
		"dropped_pii": info.DroppedPII, "dropped_noise": info.DroppedNoise, "dropped_rare": info.DroppedRare})
	st := s.catalog.Load()
	v := &domain.ConfigVersion{Source: "import", Author: "system", Rules: rules, CatalogVersion: st.version,
		Comment: "Индекс подсказок: ранний период журнала и шаблоны каталога"}
	info.Version, err = s.repos.SaveSuggestions(ctx, v, rows)
	return info, err
}

// earlyQueries — успешные запросы журнала до начала отложенной недели (как у агента: AGENT_HOLDOUT_*), с числом
// поисков, успехов и последним появлением; p0 — средняя успешность. Запрос показывается так, как его понял поиск:
// неверная раскладка исправляется; мусор и персональные данные отбрасываются; разовые — тоже.
func (s *Services) earlyQueries(ctx context.Context, lx *searchLexicon, rules *searchRules, info *SuggestBuildInfo) ([]*logQuery, time.Time, float64) {
	el, ok := s.source.(eventLoader)
	if !ok {
		return nil, s.now(), 0.5
	}
	evs, err := el.LoadEvents(ctx)
	if err != nil || len(evs) == 0 {
		if err != nil {
			zap.L().Warn("подсказки: журнал не прочитан — только каталог", zap.Error(err))
		}
		return nil, s.now(), 0.5
	}
	sort.SliceStable(evs, func(i, j int) bool { return evs[i].TS.Before(evs[j].TS) })
	last := evs[len(evs)-1].TS
	cutoff := last.Add(time.Nanosecond)
	if h := HoldoutFromEnv(); h.Enabled {
		cutoff, _ = s.holdoutRangeAt(h, last, last)
	}
	// Успех поиска — действие (клик, корзина, заказ, запрос цены) в той же сессии до следующего поиска.
	type open struct {
		key string
		at  time.Time
		ok  bool
	}
	bySession := map[string]*open{}
	byText := map[string]*logQuery{}
	var order []string
	total, succ := 0, 0
	flush := func(o *open) {
		if o == nil {
			return
		}
		if o.ok {
			byText[o.key].succ++
			succ++
		}
	}
	for _, e := range evs {
		if !e.TS.Before(cutoff) {
			break
		}
		switch e.Type {
		case "search":
			flush(bySession[e.SessionID])
			delete(bySession, e.SessionID)
			if e.Query == "" || e.ResultsCount == nil || *e.ResultsCount == 0 {
				continue
			}
			info.EarlyEvents++
			text, ok := suggestText(e.Query, lx, rules)
			if !ok {
				continue
			}
			q := byText[text]
			if q == nil {
				q = &logQuery{text: text}
				byText[text] = q
				order = append(order, text)
			}
			q.searches++
			q.last = e.TS
			total++
			bySession[e.SessionID] = &open{key: text, at: e.TS}
		default:
			if o := bySession[e.SessionID]; o != nil && e.TS.Sub(o.at) <= suggestSuccessWindow {
				o.ok = true
			}
		}
	}
	for _, o := range bySession {
		flush(o)
	}
	p0 := 0.5
	if total > 0 {
		p0 = float64(succ) / float64(total)
	}
	order = mergeDeclensions(order, byText)
	var res []*logQuery
	for _, t := range order {
		q := byText[t]
		switch {
		case piiRe.MatchString(t) && len(lx.codes().exact(normalizeMPN(t))) == 0:
			// длинные номера — признак телефона или реквизитов, кроме артикулов каталога («65297615BA01A12»)
			info.DroppedPII++
		case !readableQuery(t) || !hasLetters(t) || looksLikeMarkup(t) || len(strings.Fields(t)) > suggestMaxWords:
			info.DroppedNoise++
		case q.searches < suggestMinFreq:
			info.DroppedRare++
		default:
			res = append(res, q)
		}
	}
	return res, cutoff, p0
}

// suggestText — текст подсказки из запроса журнала: свёртка, исправление раскладки (как понял поиск); ok=false —
// длинный.
func suggestText(q string, lx *searchLexicon, rules *searchRules) (string, bool) {
	text := foldText(q)
	if utf8.RuneCountInString(text) > suggestMaxRunes {
		return "", false
	}
	nr := normalizeFull(text, lx, rules, false)
	for _, c := range nr.changes {
		// Раскладка и опечатка исправляются: «dr.web enterprisse», «опрационная система» подсказкой не становятся.
		if c.Kind == changeLayout || c.Kind == changeTypo {
			return foldText(nr.corrected), true
		}
	}
	return text, true
}

// textFamilies — семейства текстовой ветки поиска (BM25 B3 без векторов, с теми же minimum_should_match) по тексту.
func (s *Services) textFamilies(ctx context.Context, lx *searchLexicon, rules *searchRules, text string) ([]string, error) {
	nr := normalizeFull(text, lx, rules, false)
	norm := nr.normalized()
	if norm == "" {
		return nil, nil
	}
	res, err := s.search.Search(ctx, s.alias, bm25QueryB3(norm, strings.Join(nr.expansions, " "), b3MinMatch, candidatesTop, nil))
	if err != nil {
		return nil, err
	}
	top, docs, err := parseHits(res)
	if err != nil {
		return nil, err
	}
	var fams []string
	for _, h := range top {
		if f := docs[h.ProductID].FamilyID; f != "" && !contains(fams, f) {
			fams = append(fams, f)
		}
	}
	return fams, nil
}

// categoryFamilies — семейств в категории (с подкатегориями — по category_ids позиций).
func categoryFamilies(lx *searchLexicon, cat string) int {
	n := 0
	for _, f := range lx.familyOrder {
		for _, pid := range lx.families[f] {
			d := lx.docs[pid]
			if d.CategoryID == cat || contains(d.CategoryIDs, cat) {
				n++
				break
			}
		}
	}
	return n
}

// topShares — до двух значений key, у которых доля среди семейств ≥ 0,3 (семейство — один голос).
func topShares(fams []string, lx *searchLexicon, key func(indexDoc) string) []string {
	cnt := map[string]int{}
	for _, f := range fams {
		if pids := lx.families[f]; len(pids) > 0 {
			if k := key(lx.docs[pids[0]]); k != "" {
				cnt[k]++
			}
		}
	}
	ks := sortedKeys(cnt)
	sort.SliceStable(ks, func(i, j int) bool { return cnt[ks[i]] > cnt[ks[j]] })
	var out []string
	for _, k := range ks {
		if len(out) == 2 || float64(cnt[k]) < 0.3*float64(len(fams)) {
			break
		}
		out = append(out, k)
	}
	return out
}

func sortedKeys[V any](m map[string]V) []string {
	ks := make([]string, 0, len(m))
	for k := range m {
		ks = append(ks, k)
	}
	sort.Strings(ks)
	return ks
}

// journalQueries — успешные запросы журнала текущего каталога в базе (фид: файла журнала нет): живые и модельный
// поток в границах LogScope агента; успех — клик по выдаче; разовые (меньше suggestMinFreq поисков) отбрасываются,
// как и мусор и персональные данные. p0 — средняя успешность.
func (s *Services) journalQueries(ctx context.Context, lx *searchLexicon, rules *searchRules, info *SuggestBuildInfo) ([]*logQuery, float64) {
	if !s.repos.Enabled() {
		return nil, 0.5
	}
	stats, err := s.repos.QueryStatsScoped(ctx, time.Time{}, s.agentLogScope(ctx))
	if err != nil {
		zap.L().Warn("подсказки: журнал в базе не прочитан — только словарь и каталог", zap.Error(err))
		return nil, 0.5
	}
	byText := map[string]*logQuery{}
	var order []string
	total, succ := 0, 0
	now := s.now()
	for _, st := range stats {
		total += st.Searches
		succ += st.Clicks
		if st.NonEmpty == 0 || st.Clicks == 0 {
			continue
		}
		text, ok := suggestText(st.Query, lx, rules)
		if !ok {
			continue
		}
		q := byText[text]
		if q == nil {
			q = &logQuery{text: text, last: now}
			byText[text] = q
			order = append(order, text)
		}
		q.searches += st.Searches
		q.succ += st.Clicks
		info.EarlyEvents += st.Searches
	}
	p0 := 0.5
	if total > 0 {
		p0 = float64(succ) / float64(total)
	}
	order = mergeDeclensions(order, byText)
	var res []*logQuery
	for _, t := range order {
		q := byText[t]
		switch {
		case piiRe.MatchString(t) && len(lx.codes().exact(normalizeMPN(t))) == 0:
			info.DroppedPII++
		case !readableQuery(t) || !hasLetters(t) || looksLikeMarkup(t) || len(strings.Fields(t)) > suggestMaxWords:
			info.DroppedNoise++
		case q.searches < suggestMinFreq:
			info.DroppedRare++
		default:
			res = append(res, q)
			info.FromJournal++
		}
	}
	return res, p0
}

// suggestLeadWords — служебные слова в начале названия семейства, которые покупатель не набирает.
var suggestLeadWords = []string{"операционная система ", "программный комплекс ", "программа ", "пк "}

// suggestDisplay — текст подсказки из названия: без кавычек «» и "", без «Программа», «Операционная система» в
// начале, пробелы схлопнуты: «Программа КриптоПро CSP 5.0» → «КриптоПро CSP 5.0», «"Лаборатория Касперского"» →
// «Лаборатория Касперского».
func suggestDisplay(name string) string {
	// Пояснение в скобках в конце («Microsoft Windows 11 Pro (Профессиональная)») покупатель не набирает.
	if i := strings.LastIndex(name, " ("); i > 0 && strings.HasSuffix(strings.TrimSpace(name), ")") && i > len(name)/3 {
		name = name[:i]
	}
	r := strings.NewReplacer("«", "", "»", "", "\"", "", "“", "", "”", "", "„", "")
	t := strings.Join(strings.Fields(r.Replace(name)), " ")
	low := strings.ToLower(t)
	for _, w := range suggestLeadWords {
		if strings.HasPrefix(low, w) && len(t) > len(w)+2 {
			t = t[len(w):]
			low = low[len(w):]
		}
	}
	return strings.TrimSpace(t)
}

// dictForm — подсказка из словаря и её частота-заменитель (написание 0,6, жаргон 0,3).
type dictForm struct {
	text string
	freq float64
}

// dictionaryForms — формы ввода из словаря понимания (lexicon.csv) и импортные транслитерации вендоров
// («касперский»): по одной на каноническую форму, плюс каноническая форма-термин. Порядок детерминирован.
func dictionaryForms(lex []domain.LexiconEntry, base []domain.Alias) []dictForm {
	type group struct {
		canon  string
		forms  []string
		plain  []string // не жаргон: «касперский» раньше «касперыч»
		jargon bool
		term   bool
	}
	groups := map[string]*group{}
	var order []string
	addForm := func(canon, alias string, jargon, term bool) {
		canon, alias = strings.Join(strings.Fields(foldText(canon)), " "), strings.TrimSpace(alias)
		if canon == "" || alias == "" {
			return
		}
		g := groups[canon]
		if g == nil {
			g = &group{canon: canon, jargon: true}
			groups[canon] = g
			order = append(order, canon)
		}
		g.forms = append(g.forms, alias)
		if !jargon {
			g.plain = append(g.plain, alias)
		}
		g.jargon = g.jargon && jargon
		g.term = g.term || term
	}
	for _, e := range lex {
		if e.Kind == domain.LexiconDrop || e.Kind == domain.LexiconSuccessor {
			continue
		}
		addForm(e.Canonical, e.Alias, e.Kind == domain.LexiconJargon, e.Target == domain.AliasTargetTerm)
	}
	for _, a := range base {
		// Импортные алиасы — только транслитерации вендоров кириллицей («касперский»), не слаги.
		if a.Target == domain.AliasTargetVendor && a.Kind == "transliteration" && hasCyrillic(a.Alias) {
			addForm(a.Canonical, a.Alias, false, false)
		}
	}
	var res []dictForm
	for _, c := range order {
		g := groups[c]
		freq := 0.6
		if g.jargon {
			freq = 0.3
		}
		best := ""
		forms := g.forms
		if len(g.plain) > 0 {
			forms = g.plain
		}
		for _, f := range forms {
			ff := foldText(f)
			switch {
			case best == "":
				best = f
			case ff == c && foldText(best) != c:
				best = f
			case foldText(best) == c:
			case strings.Contains(ff, " ") != strings.Contains(foldText(best), " "):
				if strings.Contains(ff, " ") {
					best = f
				}
			case len(ff) < len(foldText(best)) || len(ff) == len(foldText(best)) && ff < foldText(best):
				best = f
			}
		}
		res = append(res, dictForm{text: best, freq: freq})
		if g.term && foldText(best) != c {
			res = append(res, dictForm{text: c, freq: 0.6})
		}
	}
	return res
}

func hasCyrillic(s string) bool {
	for _, r := range s {
		if r >= 'а' && r <= 'я' || r >= 'А' && r <= 'Я' || r == 'ё' || r == 'Ё' {
			return true
		}
	}
	return false
}
