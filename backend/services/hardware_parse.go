package services

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"
	"unicode"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/in"
)

// Разбор запроса по оборудованию правилами: парт-номер, модель, категория, характеристики с операторами,
// цена, отрицания, количество закупки, цель совместимости. Порядок для чисел: «число + явная единица» раньше
// «голого числа»; токены, которые правила лицензий уже забрали («16 ядер», «на 10 пк»), не трогаются.

// hwCond — условие на характеристику: число (specs.<key>) или значение перечислимого атрибута.
type hwCond struct {
	Key    string  `json:"key"`
	Op     string  `json:"op"` // eq | gte | lte | range | not
	Value  float64 `json:"value"`
	Value2 float64 `json:"value2,omitempty"`
	Str    string  `json:"str,omitempty"`
	Unit   string  `json:"unit,omitempty"`
	Text   string  `json:"text"`
	Source string  `json:"source"` // query | filter
	typ    string
}

// hwMPN — найденный парт-номер.
type hwMPN struct {
	Text     string   `json:"text"`
	Norm     string   `json:"normalized"`
	Prefix   bool     `json:"prefix"`
	Products []string `json:"products"`
}

// hwPlan — разбор одного запроса и всё, что шаг hardware делает с выдачей. nil или выключен — ничего.
type hwPlan struct {
	hx *hardwareLexicon
	lx *searchLexicon
	cc *commerceCatalog

	category string
	conds    []hwCond
	priceMin *float64
	priceMax *float64
	// vendorNot — исключённые вендоры («не Dell»).
	vendorNot []string
	// quantity — количество закупки («20 ноутбуков»): не условие лицензии.
	quantity int
	mpn      *hwMPN
	// model — модель, названная без «для» («dell poweredge r740»): ищется точным полем model_id.
	model string
	// target — цель «X для Y»; compatMode — filter (только совместимые) или unknown (данных нет).
	target       string
	compatMode   string
	allowed      map[string]CompatResult
	allowUnknown bool
	// zoneCategory — категория, названная после «для» («сумка для ноутбука»): не категория запроса.
	zoneCategory bool
	// multiCat — названо несколько категорий оборудования: фильтра категории нет.
	multiCat   bool
	consumed   map[int]bool
	protected  []string
	negations  []string
	exactHits  []domain.RankedHit
	structHits []domain.RankedHit
	err        string
	ms         float64
	// indexed — характеристики, под которые в индексе есть поля specs.* (nil — неизвестно, фильтры как есть).
	// Характеристика без поля в индексе (разобрана из атрибутов фида при старте) проверяется в памяти (reject).
	indexed map[string]bool
	// relaxed — числовые условия, которым не соответствует ни одна позиция категории: сняты, выдача — похожие
	// (relaxSpecs); missing — у позиций категории этой характеристики нет вовсе.
	relaxed []hwRelaxed
}

// hwRelaxed — снятое условие характеристики и почему.
type hwRelaxed struct {
	cond    hwCond
	missing bool
}

// active — в запросе есть признак оборудования: шаг меняет фильтры, маршрут и чипы.
func (p *hwPlan) active() bool {
	return p != nil && (p.category != "" || len(p.conds) > 0 || p.mpn != nil || p.model != "" || p.target != "" ||
		p.priceMin != nil || p.priceMax != nil || len(p.vendorNot) > 0 || p.quantity > 0)
}

// exactQuery — точный запрос (парт-номер): маржа и прочие сдвиги порядка на нём выключены.
func (p *hwPlan) exactQuery() bool { return p != nil && p.mpn != nil && !p.mpn.Prefix }

var (
	reNumUnit = regexp.MustCompile(`^(\d+(?:[.,]\d+)?)(.*)$`)
	reRange   = regexp.MustCompile(`^(\d+(?:[.,]\d+)?)-(\d+(?:[.,]\d+)?)(.*)$`)
)

var (
	gteWords      = setOf("от", "from", "минимум", "больше", "более", "свыше", ">", ">=", "≥")
	lteWords      = setOf("до", "максимум", "меньше", "менее", "<", "<=", "≤")
	currencyWords = setOf("руб", "рублей", "рубля", "рубль", "р", "₽", "rub", "rur")
	thousandWords = setOf("тыс", "тысяч", "тысячи", "тысяча", "т", "к", "k")
	negWords      = setOf("не", "кроме", "без")
	osWords       = setOf("ос", "os", "операционной", "операционки", "операционная", "системы", "windows", "виндовс", "винды")
	preinstWords  = setOf("windows", "виндовс", "ос", "os", "предустановленной", "предустановленная")
)

// unitContext — слова рядом с числом, которые решают неоднозначную единицу («16 гб озу» — память).
var unitContext = []struct{ stem, key string }{
	{"озу", "ram_gb"}, {"оперативн", "ram_gb"}, {"памят", "ram_gb"}, {"ram", "ram_gb"}, {"оперативк", "ram_gb"},
	{"ssd", "storage_gb"}, {"hdd", "storage_gb"}, {"диск", "storage_gb"}, {"накопит", "storage_gb"},
	{"nvme", "storage_gb"}, {"sata", "storage_gb"}, {"storage", "storage_gb"}, {"жестк", "storage_gb"},
	{"аплинк", "net_gbps"}, {"uplink", "net_gbps"}, {"sfp", "net_gbps"}, {"диагонал", "screen_inch"},
	{"экран", "screen_inch"}, {"мощност", "power_w"}, {"ядер", "cpu_cores"},
}

func contextKey(w string) string {
	for _, c := range unitContext {
		if strings.HasPrefix(w, c.stem) {
			return c.key
		}
	}
	return ""
}

// hardwareParse — разбор запроса для шага hardware. Меняет attrs: забирает токены условий оборудования и
// переносит количество закупки из лицензионного количества. Флаг выключен или в каталоге нет оборудования — nil.
func (s *Services) hardwareParse(ctx context.Context, st *catalogState, nr normResult, attrs *queryAttrs, req in.SearchRequest) (p *hwPlan) {
	if !s.hwEnabled(ctx) || st == nil || !st.hw.hasHardware() {
		return nil
	}
	t := time.Now()
	cc := s.commerceFor(ctx)
	p = &hwPlan{hx: st.hw, lx: st.lex, cc: cc, consumed: map[int]bool{}, allowed: map[string]CompatResult{}}
	defer func() {
		if r := recover(); r != nil {
			// Ошибка разбора не ломает поиск: шаг с error, выдача как без него.
			*p = hwPlan{hx: st.hw, lx: st.lex, cc: cc, err: "разбор оборудования: сбой правил"}
		}
		p.ms = msSince(t)
	}()
	p.parse(nr, attrs)
	p.applyFilters(req.Filters)
	p.applyRemoved(req.RemovedChips)
	for i := range p.consumed {
		attrs.consumed[i] = true
	}
	return p
}

func (p *hwPlan) parse(nr normResult, attrs *queryAttrs) {
	tokens := nr.tokens
	n := len(tokens)
	raw := foldText(nr.raw)
	at := func(i int) string {
		if i < 0 || i >= n {
			return ""
		}
		return tokens[i]
	}
	licensed := map[int]bool{}
	for i := range attrs.consumed {
		licensed[i] = true
	}

	// «для»: всё после него — цель совместимости, категория запроса — только до него.
	forIdx := -1
	for i, t := range tokens {
		if t == "для" || t == "for" || t == "к" {
			forIdx = i
			break
		}
	}
	head := tokens
	if forIdx >= 0 {
		head = tokens[:forIdx]
	}

	// Парт-номер: токен или два-три соседних («ksm32rd4 32hdr»), есть в словаре — целиком или началом.
	for size := 3; size >= 1 && p.mpn == nil; size-- {
		for i := 0; i+size <= n && p.mpn == nil; i++ {
			joined := strings.Join(tokens[i:i+size], "")
			norm := mpnCandidate(joined)
			if norm == "" {
				continue
			}
			ids, prefix := p.hx.mpnLookup(norm)
			if len(ids) == 0 {
				continue
			}
			p.mpn = &hwMPN{Text: strings.Join(tokens[i:i+size], " "), Norm: norm, Prefix: prefix, Products: ids}
			for j := i; j < i+size; j++ {
				p.protected = append(p.protected, tokens[j])
			}
		}
	}
	for _, t := range tokens {
		if hasDigit(t) && strings.IndexFunc(t, func(r rune) bool { return r >= 'a' && r <= 'z' }) >= 0 && !contains(p.protected, t) {
			p.protected = append(p.protected, t) // модельные обозначения: R740, i5-13400, DL380 — без исправлений
		}
	}

	// Категория: самая длинная серия соседних слов одной категории, ничья — первая. Слабые слова
	// («сервер», «память») — только с другим признаком оборудования; ПО в запросе — не оборудование.
	hasSoftware := false
	// ПО в запросе — семейство, названное словом с буквами (число «16» — не FineReader 16), не единицей и не
	// словом категории оборудования.
	var named []string
	for _, t := range tokens {
		if p.lx.compoundNames[t] {
			named = append(named, t) // «сканер-вс» — название продукта, а не категория «Сканеры»
			continue
		}
		if strings.IndexFunc(t, unicode.IsLetter) < 0 || len(p.hx.units[t]) > 0 || p.hx.categoryOfWord(t) != "" || contextKey(t) != "" {
			continue
		}
		named = append(named, t)
	}
	for _, f := range p.lx.matchFamilies(named) {
		if !p.hx.hwCats[p.lx.familyCat[f]] {
			hasSoftware = true
		}
	}
	// Вендор, у которого в каталоге нет оборудования («касперский на 50 устройств на год»): запрос о лицензиях,
	// «устройств» — метрика, а не категория «Зарядные устройства».
	for _, v := range p.lx.matchVendors(named, nil) {
		if !hasSoftware && p.lx.softwareOnlyVendor(v, p.hx.hwCats) {
			hasSoftware = true
		}
	}
	for _, t := range tokens {
		// Слово — главное в названии категории ПО («антивирус на 20 компьютеров»): количество — места лицензии.
		if !hasSoftware && !lexStopWords[t] && p.lx.softwareCategoryWord(t, p.hx.hwCats) {
			hasSoftware = true
		}
	}
	type catRun struct {
		cat         string
		start, size int
		weak        bool
		indexes     []int
	}
	var runs []catRun
	catAt := map[int]bool{}
	for i, t := range head {
		c := p.hx.categoryOfWord(t)
		if p.lx.compoundNames[t] {
			c = ""
		}
		if _, unit := metricOf(t); unit && licensed[i] {
			c = "" // «антивирус на 20 компьютеров»: число мест лицензии, а не категория «Компьютеры»
		}
		if hwComputerWords[t] && attrs.quantity > 0 && attrs.metricWord == t {
			c = "" // «продлить касперский на 100 пк»: «пк» с числом — места лицензии, а не настольные компьютеры
		}
		if c == "" {
			continue
		}
		catAt[i] = true
		if l := len(runs); l > 0 && runs[l-1].cat == c && runs[l-1].start+runs[l-1].size == i {
			runs[l-1].size++
			runs[l-1].weak = false // «серверная память»: два слова одной категории — уже не слабый признак
			continue
		}
		runs = append(runs, catRun{cat: c, start: i, size: 1, weak: weakCategoryWord(t)})
	}
	// Слово, которое ещё и единица контекста («памяти» в «ноутбук … памяти»), — только если серия длиннее.
	var bestRun *catRun
	for i := range runs {
		if bestRun == nil || runs[i].size > bestRun.size {
			bestRun = &runs[i]
		}
	}

	// Модель и цель совместимости.
	zone, zoneStart := tokens, 0
	if forIdx >= 0 {
		zone, zoneStart = tokens[forIdx+1:], forIdx+1
	}
	model, idx := p.hx.modelMatch(zone)
	if forIdx >= 0 && model == nil {
		for _, t := range zone {
			if p.hx.categoryOfWord(t) != "" {
				p.zoneCategory = true
			}
		}
	}

	evidence := p.mpn != nil || model != nil
	// Числа и единицы.
	type numTok struct {
		v, v2   float64
		isRange bool
		suffix  string
		span    int
	}
	parseNum := func(i int) (numTok, bool) {
		t := at(i)
		if m := reRange.FindStringSubmatch(t); m != nil {
			a, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
			b, _ := strconv.ParseFloat(strings.ReplaceAll(m[2], ",", "."), 64)
			return numTok{v: a, v2: b, isRange: true, suffix: m[3], span: 1}, true
		}
		m := reNumUnit.FindStringSubmatch(t)
		if m == nil {
			return numTok{}, false
		}
		v, _ := strconv.ParseFloat(strings.ReplaceAll(m[1], ",", "."), 64)
		nt := numTok{v: v, suffix: m[2], span: 1}
		if m[2] == "" && hwDigits(at(i+1)) {
			next := at(i + 1)
			switch {
			case strings.Contains(raw, t+","+next) || strings.Contains(raw, t+"."+next):
				nt.v, _ = strconv.ParseFloat(t+"."+next, 64)
				nt.span = 2
			case len(next) == 3 && strings.Contains(raw, t+" "+next):
				s := t
				for len(at(i+nt.span)) == 3 && hwDigits(at(i+nt.span)) {
					s += at(i + nt.span)
					nt.span++
				}
				nt.v, _ = strconv.ParseFloat(s, 64)
			}
		}
		return nt, true
	}

	hwCat := ""
	if bestRun != nil && !hasSoftware {
		hwCat = bestRun.cat
	}
	unitKeys := func(alias string) []hwUnit { return p.hx.units[alias] }
	chooseKey := func(cands []hwUnit, i int, v float64, op string) (hwUnit, bool) {
		if len(cands) == 1 {
			return cands[0], true
		}
		// Контекст: слова в окне ±3.
		for d := 1; d <= 3; d++ {
			for _, j := range []int{i + d, i - d} {
				w := at(j)
				k := contextKey(w)
				if k == "" || catAt[j] && p.hx.categoryOfWord(w) == hwCat {
					continue
				}
				for _, c := range cands {
					if c.key == k {
						if cw := p.hx.categoryOfWord(w); len(p.hx.enums[w]) == 0 && (cw == "" || cw != hwCat) {
							p.consumed[j] = true // «озу», «оперативной памяти» — условие, а не слово поиска
							if nx := at(j + 1); contextKey(nx) == k && len(p.hx.enums[nx]) == 0 && p.hx.categoryOfWord(nx) != hwCat {
								p.consumed[j+1] = true
							}
						}
						return c, true
					}
				}
			}
		}
		// Категория: ключи её атрибутов; несколько — по данным: у скольких позиций такое значение.
		var inCat []hwUnit
		for _, c := range cands {
			if hwCat == "" || contains(p.hx.catAttrs[hwCat], c.key) {
				inCat = append(inCat, c)
			}
		}
		if len(inCat) == 0 {
			inCat = cands
		}
		best, bestN := hwUnit{}, -1
		for _, c := range inCat {
			vals := p.hx.specValues(p.lx, hwCat, c.key)
			cnt := 0
			for x, k := range vals {
				x2 := v * c.factor
				switch op {
				case "gte":
					if x >= x2 {
						cnt += k
					}
				case "lte":
					if x <= x2 {
						cnt += k
					}
				default:
					if x == x2 {
						cnt += k
					}
				}
			}
			if cnt > bestN {
				best, bestN = c, cnt
			}
		}
		return best, bestN >= 0
	}

	var conds []hwCond
	var priceMin, priceMax *float64
	consume := func(from, to int) {
		for j := from; j < to; j++ {
			p.consumed[j] = true
		}
	}
	for i := 0; i < n; i++ {
		t := tokens[i]
		if licensed[i] || p.consumed[i] || contains(p.protected, t) && p.mpn != nil && strings.Contains(p.mpn.Text, t) {
			continue
		}
		// Отрицания: «без ОС», «не dell», «кроме hp».
		if negWords[t] && i+1 < n {
			if t == "без" && osWords[at(i+1)] {
				if v := p.enumValue("os", "без"); v != "" {
					j := i + 1
					for j < n && osWords[at(j)] {
						j++
					}
					conds = append(conds, hwCond{Key: "os", Op: "eq", Str: v, Text: strings.Join(tokens[i:j], " "), Source: "query", typ: "enum"})
					p.negations = append(p.negations, strings.Join(tokens[i:j], " "))
					consume(i, j)
					i = j - 1
					evidence = true
					continue
				}
			}
			if vs := p.lx.matchVendors([]string{at(i + 1)}, nil); len(vs) == 1 {
				p.vendorNot = append(p.vendorNot, vs[0])
				p.negations = append(p.negations, t+" "+at(i+1))
				consume(i, i+2)
				i++
				continue
			}
		}
		if t == "с" && preinstWords[at(i+1)] && hwCat != "" && contains(p.hx.catAttrs[hwCat], "os") {
			if v := p.enumValue("os", "предустанов"); v != "" {
				conds = append(conds, hwCond{Key: "os", Op: "eq", Str: v, Text: t + " " + at(i+1), Source: "query", typ: "enum"})
				consume(i, i+2)
				i++
				continue
			}
		}
		nt, ok := parseNum(i)
		if !ok {
			continue
		}
		// Оператор перед числом.
		op, opFrom, price := "eq", i, false
		prev, prev2 := at(i-1), at(i-2)
		switch {
		case prev2 == "не" && (prev == "менее" || prev == "меньше" || prev == "дешевле"):
			op, opFrom, price = "gte", i-2, prev == "дешевле"
		case prev2 == "не" && (prev == "более" || prev == "больше" || prev == "дороже"):
			op, opFrom, price = "lte", i-2, prev == "дороже"
		case prev == "дешевле":
			op, opFrom, price = "lte", i-1, true
		case prev == "дороже":
			op, opFrom, price = "gte", i-1, true
		case gteWords[prev]:
			op, opFrom = "gte", i-1
		case lteWords[prev]:
			op, opFrom = "lte", i-1
		}
		if licensed[opFrom] {
			opFrom = i
		}
		j := i + nt.span
		v, v2 := nt.v, nt.v2
		if nt.isRange {
			op = "range"
		}
		// «от 24 до 27 дюймов»
		if op == "gte" && at(j) == "до" {
			if nt2, ok2 := parseNum(j + 1); ok2 && !nt2.isRange {
				op, v2 = "range", nt2.v
				nt.suffix = nt2.suffix
				j = j + 1 + nt2.span
			}
		}
		unit := nt.suffix
		glued := unit != ""
		unitEnd := j // индекс после единицы
		if !glued {
			unit = at(j)
			unitEnd = j + 1
		}
		unit = strings.TrimSuffix(unit, ".")
		// Цена: «до 100000 рублей», «дешевле 50 тыс», «до 500 тыс руб».
		mult, currency := 1.0, currencyWords[unit]
		if thousandWords[unit] {
			mult = 1000
			if currencyWords[at(unitEnd)] {
				unitEnd++
				currency = true
			}
		}
		if currency || price || mult > 1 && op != "eq" {
			val := v * mult
			end := unitEnd
			if !currency && mult == 1 {
				end = i + nt.span // единицы нет: «дешевле 50000»
			}
			if op == "gte" {
				priceMin = &val
			} else {
				priceMax = &val
			}
			consume(opFrom, end)
			evidence = true
			i = end - 1
			continue
		}
		j = unitEnd - 1
		if cands := unitKeys(unit); len(cands) > 0 && !(weakUnits[unit] && !glued && hwCat == "" && !evidence) {
			u, ok := chooseKey(cands, i, v, op)
			if ok {
				attr := p.hx.attrs[u.key]
				if op == "eq" && atLeastKeys[u.key] {
					op = "gte"
				}
				c := hwCond{Key: u.key, Op: op, Value: v * u.factor, Unit: attr.unit, Source: "query", typ: "number"}
				if op == "range" {
					c.Value2 = v2 * u.factor
				}
				end := j + 1
				if (unit == "гбит" || unit == "гбит/с") && at(end) == "с" {
					end++
				}
				if at(end) == "и" && (at(end+1) == "выше" || at(end+1) == "более" || at(end+1) == "больше") {
					c.Op = "gte"
					end += 2
				}
				c.Text = strings.Join(tokens[opFrom:end], " ")
				conds = append(conds, c)
				consume(opFrom, end)
				evidence = true
				i = end - 1
				continue
			}
		}
		// «до 500 000» без единицы при признаке оборудования — цена.
		if op == "lte" && v >= 1000 && !nt.isRange && (hwCat != "" || evidence) && !glued {
			val := v
			priceMax = &val
			consume(opFrom, j)
			i = j - 1
			continue
		}
		// Голое число в категории: если такое значение есть ровно у одной характеристики категории.
		// После числа — незнакомое слово («2 процессора», «3 ква»): это единица, которой нет в словаре, а не
		// голое число; иначе «2» станет высотой 2U.
		nx := at(i + nt.span)
		unknownUnit := nx != "" && strings.IndexFunc(nx, unicode.IsLetter) >= 0 && !lexStopWords[nx] &&
			p.hx.categoryOfWord(nx) == "" && len(p.lx.matchVendors([]string{nx}, nil)) == 0
		if hwCat != "" && !glued && !nt.isRange && !licensed[i] && !unknownUnit {
			var keys []string
			for _, k := range p.hx.catAttrs[hwCat] {
				if p.hx.attrs[k].typ != "number" {
					continue
				}
				if p.hx.specValues(p.lx, hwCat, k)[v] > 0 {
					keys = append(keys, k)
				}
			}
			if len(keys) == 1 {
				attr := p.hx.attrs[keys[0]]
				conds = append(conds, hwCond{Key: keys[0], Op: op, Value: v, Unit: attr.unit, Text: strings.Join(tokens[opFrom:i+nt.span], " "), Source: "query", typ: "number"})
				consume(opFrom, i+nt.span)
				i += nt.span - 1
			}
		}
	}

	// Перечислимые значения словом («ddr4», «ssd», «poe»): токен не забирается — он и слово поиска.
	if hwCat != "" || evidence || len(conds) > 0 {
		seen := map[string]bool{}
		for _, c := range conds {
			seen[c.Key] = true
		}
		for i, t := range tokens {
			if licensed[i] || p.consumed[i] {
				continue
			}
			w := t
			if s := enumSynonyms[w]; s != "" {
				w = s
			}
			neg := i > 0 && negWords[tokens[i-1]] && !p.consumed[i-1]
			for _, ref := range p.hx.enums[w] {
				if seen[ref.key] || hwCat != "" && !contains(p.hx.catAttrs[hwCat], ref.key) {
					continue
				}
				seen[ref.key] = true
				if neg {
					// «без poe», «не ssd»: значение исключено; слово не идёт в поиск, иначе поднимет то, что исключили.
					conds = append(conds, hwCond{Key: ref.key, Op: "not", Str: ref.value, Text: tokens[i-1] + " " + t, Source: "query", typ: "enum"})
					p.negations = append(p.negations, tokens[i-1]+" "+t)
					consume(i-1, i+1)
				} else {
					conds = append(conds, hwCond{Key: ref.key, Op: "eq", Str: ref.value, Text: t, Source: "query", typ: "enum"})
				}
				evidence = true
			}
		}
	}

	// Несколько разных категорий («коммутатор и ибп», «сервер с памятью и дисками») — закупка нескольких вещей:
	// фильтра категории нет. Слова, ушедшие в условия («оперативной памяти»), категорией не считаются.
	distinct := map[string]bool{}
	for _, r := range runs {
		if !p.consumed[r.start] {
			distinct[r.cat] = true
		}
	}
	multiCat := len(distinct) > 1
	p.multiCat = multiCat
	if bestRun != nil && !hasSoftware && !multiCat && (!bestRun.weak || evidence || p.weakAlone(tokens, named, attrs, bestRun.start)) {
		p.category = bestRun.cat
	}

	// Модель: после «для» или рядом с категорией-компонентом — цель совместимости, иначе — модель запроса.
	if model != nil && !hasSoftware {
		if forIdx >= 0 || p.category != "" && p.hx.components[p.category] {
			p.target = model.id
			for _, k := range idx {
				p.consumed[zoneStart+k] = true
			}
			if forIdx >= 0 {
				p.consumed[forIdx] = true
			}
			p.resolveCompat()
		} else if p.category == "" || !p.hx.components[p.category] {
			p.model = model.id
		}
	}

	// Количество закупки: «20 ноутбуков» — штуки, а не лицензии на 20 устройств.
	if attrs.quantity > 0 && (p.category != "" || !hwComputerWords[attrs.metricWord] && p.hx.categoryOfWord(attrs.metricWord) != "") && !hasSoftware {
		if attrs.metric == domain.MetricCore && contains(p.hx.catAttrs[p.category], "cpu_cores") {
			conds = append(conds, hwCond{Key: "cpu_cores", Op: "eq", Value: float64(attrs.quantity), Unit: p.hx.attrs["cpu_cores"].unit,
				Text: strconv.Itoa(attrs.quantity) + " " + attrs.metricWord, Source: "query", typ: "number"})
		} else {
			p.quantity = attrs.quantity
			if p.category == "" && !multiCat {
				p.category = p.hx.categoryOfWord(attrs.metricWord)
			}
			for i, t := range tokens {
				if t == attrs.metricWord && p.hx.categoryOfWord(t) != "" {
					delete(attrs.consumed, i) // слово категории остаётся словом поиска
				}
			}
		}
		attrs.quantity, attrs.metric, attrs.metricUnresolved, attrs.metricWord = 0, "", false, ""
	}
	if p.category != "" {
		// Оборудование не продлевается как лицензия: «продление гарантии» — сервисный пакет, не renewal.
		attrs.renewal = false
	}
	p.conds = conds
	p.priceMin, p.priceMax = priceMin, priceMax
}

// enumValue — значение атрибута key из реестра, которое начинается с prefix («без» → «без ОС»).
func (p *hwPlan) enumValue(key, prefix string) string {
	for _, v := range p.hx.attrs[key].values {
		if strings.HasPrefix(foldText(v), prefix) {
			return v
		}
	}
	return ""
}

func hwDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

// resolveCompat — совместимость позиций оборудования с целью: есть правила для модели — фильтр (только
// verified и conditional), нет ни одного — unknown: обычный поиск с пометкой «не подтверждена».
func (p *hwPlan) resolveCompat() {
	has := false
	for _, ids := range p.hx.products {
		for _, id := range ids {
			r := p.cc.ModelCompat(id, p.target)
			if r.State != domain.CompatUnknown {
				has = true
			}
			p.allowed[id] = r
		}
	}
	p.compatMode = "unknown"
	if has {
		p.compatMode = "filter"
	}
}

// compatOK — позиция допустима под цель совместимости.
func (p *hwPlan) compatOK(id string) bool {
	r, ok := p.allowed[id]
	if !ok {
		r = p.cc.ModelCompat(id, p.target)
	}
	return r.State == domain.CompatVerified || r.State == domain.CompatConditional
}

// applyFilters — фасеты из запроса: ключ характеристики, цены или вендора.
func (p *hwPlan) applyFilters(fs []in.Filter) {
	for _, f := range fs {
		num := func(x any) (float64, bool) {
			switch v := x.(type) {
			case float64:
				return v, true
			case int:
				return float64(v), true
			case string:
				n, err := strconv.ParseFloat(strings.ReplaceAll(v, ",", "."), 64)
				return n, err == nil
			}
			return 0, false
		}
		switch {
		case f.Key == fieldPrice:
			if v, ok := num(f.Value); ok {
				val := v
				if f.Op == "gte" {
					p.priceMin = &val
				} else {
					p.priceMax = &val
				}
				if f.Op == "range" {
					if v2, ok := num(f.Value2); ok {
						p.priceMin, p.priceMax = &val, &v2
					}
				}
			}
		case f.Key == "vendor" || f.Key == "vendor_id":
			if s, ok := f.Value.(string); ok && f.Op == "not" {
				p.vendorNot = append(p.vendorNot, s)
			}
		default:
			a, ok := p.hx.attrs[f.Key]
			if !ok {
				continue // неизвестный ключ — не ошибка (справочник меняется с каталогом)
			}
			p.dropCond(f.Key)
			if a.typ == "number" {
				v, ok := num(f.Value)
				if !ok {
					continue
				}
				c := hwCond{Key: f.Key, Op: f.Op, Value: v, Unit: a.unit, Text: "фасет", Source: "filter", typ: "number"}
				if f.Op == "range" {
					c.Value2, _ = num(f.Value2)
				}
				p.conds = append(p.conds, c)
				continue
			}
			s := ""
			switch v := f.Value.(type) {
			case string:
				s = v
			case bool:
				s = strconv.FormatBool(v)
			case float64:
				s = strconv.FormatFloat(v, 'f', -1, 64)
			}
			op := "eq"
			if f.Op == "not" {
				op = "not"
			}
			p.conds = append(p.conds, hwCond{Key: f.Key, Op: op, Str: s, Text: "фасет", Source: "filter", typ: "enum"})
		}
	}
}

func (p *hwPlan) dropCond(key string) {
	out := p.conds[:0]
	for _, c := range p.conds {
		if c.Key != key {
			out = append(out, c)
		}
	}
	p.conds = out
}

// applyRemoved — снятые покупателем чипы оборудования.
func (p *hwPlan) applyRemoved(rs []in.RemovedChip) {
	for _, r := range rs {
		switch r.Field {
		case fieldMPN:
			p.mpn = nil
		case fieldCompat:
			p.target, p.compatMode, p.allowed = "", "", map[string]CompatResult{}
		case fieldModel:
			p.model = ""
		case fieldPrice:
			p.priceMin, p.priceMax = nil, nil
		case fieldQuantity:
			p.quantity = 0
		case fieldVendorNot:
			out := p.vendorNot[:0]
			for _, v := range p.vendorNot {
				if v != r.Value {
					out = append(out, v)
				}
			}
			p.vendorNot = out
		case fieldCategory:
			if r.Value == p.category {
				p.category = ""
			}
		default:
			p.dropCond(r.Field)
		}
	}
}

// softwareContext — слова, с которыми «сервер» — это лицензия или ПО, а не железо.
var softwareContext = []string{"лиценз", "ос", "операцион", "cal", "кал", "субд", "1с", "windows", "винд", "linux",
	"линукс", "sql", "терминал", "rds", "почтов", "антивирус", "защит", "бэкап", "резерв", "программ", "софт",
	"виртуал", "гипервиз", "ядр", "клиент", "доступ"}

// weakAlone — слабое слово («сервер», «серверы dell», «отечественный сервер») без ПО рядом: запрос про железо.
// «сервер 1с», «лицензия на сервер», «windows server» — ПО: там рядом семейство, слово ПО или метрика лицензии.
func (p *hwPlan) weakAlone(tokens, named []string, attrs *queryAttrs, at int) bool {
	if attrs != nil && attrs.consumed[at] {
		return false // «на сервер» — метрика лицензии
	}
	if !strings.HasPrefix(tokens[at], "сервер") && !strings.HasPrefix(tokens[at], "server") {
		return false // «память» без признака — и оперативка ноутбука, и модуль сервера: решает разбор дальше
	}
	for _, t := range tokens {
		for _, w := range softwareContext {
			if t == w || utf8.RuneCountInString(w) > 2 && strings.HasPrefix(t, w) {
				return false
			}
		}
	}
	if c, _ := p.lx.categoryOf(named); c != "" && !p.hx.hwCats[c] {
		return false
	}
	// Незнакомое слово рядом («скуль сервер 2022», «эскуэль сервер») — скорее название программы, чем железо.
	for _, t := range named {
		if utf8.RuneCountInString(t) > 2 && !lexStopWords[t] && !lexServiceWords[t] && !plainAdjective(t) && !knownWord(t, p.lx, nil) {
			return false
		}
	}
	return true
}

// plainAdjective — общие слова оценки («новый», «мощный», «недорогой»): не название программы.
func plainAdjective(t string) bool {
	for _, st := range []string{"нов", "мощн", "недорог", "дешев", "дешёв", "хорош", "надежн", "надёжн", "быстр", "простой", "бу", "б/у", "современ"} {
		if strings.HasPrefix(t, st) {
			return true
		}
	}
	return false
}
