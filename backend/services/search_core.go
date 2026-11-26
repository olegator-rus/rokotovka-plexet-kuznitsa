package services

import (
	"context"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"time"
	"unicode"
	"unicode/utf8"

	"kuznica/api/domain"

	"go.uber.org/zap"
)

// Ядро поиска B3/B4 (ARCHITECTURE §2.1–2.7): нормализация по словарю каталога → атрибуты правилами →
// маршрут A/B/C/D → [D] разбор намерения → BM25 + векторы + RRF → допустимость → порог → группы →
// бизнес-слой → лестница пустой выдачи. Этот файл — состояние ядра, правила и сущности запроса.

// namedCoverageMin — семейство считается названным целиком, если в запросе есть такая доля его названия
// (по весу слов). Подобрано на tune.
const namedCoverageMin = 0.6

// rulesTTL — сколько верить закешированной последней версии правил (B4): агент принимает правку — через
// TTL поиск её видит.
const rulesTTL = 10 * time.Second

// searchCore — состояние ядра поиска в Services.
type searchCore struct {
	// latest — последняя версия правил из config_versions (B4).
	latest atomic.Pointer[cachedRules]
	// parser — разбор намерения на маршруте D; nil — маршрут D работает как C.
	parserMu sync.RWMutex
	parser   domain.IntentParser
	// suggest — индекс подсказок и связанных запросов; строится при первом обращении.
	suggestMu sync.Mutex
	suggest   *suggestIndex
}

type cachedRules struct {
	rules *searchRules
	lexOf *searchLexicon // правила версии 1 берутся из этого снимка каталога
	at    time.Time
}

// SetIntentParser — разборщик намерения для маршрута D (трек intent-llm). nil — маршрут D идёт на правилах.
func (s *Services) SetIntentParser(p domain.IntentParser) {
	s.core.parserMu.Lock()
	s.core.parser = p
	s.core.parserMu.Unlock()
}

func (s *Services) intentParser() domain.IntentParser {
	s.core.parserMu.RLock()
	defer s.core.parserMu.RUnlock()
	return s.core.parser
}

// SearchOptions — внутренние параметры поиска, которых нет в HTTP: песочница агента и прогон оценки.
type SearchOptions struct {
	// Rules — снимок правил вместо действующей версии (песочница агента); nil — по конфигурации.
	Rules *domain.RulesSnapshot
	// NoLog — не писать событие и трассу в журнал (прогон оценки).
	NoLog bool
	// Margin — политика маржи поверх действующей (C3, симуляция; margin.go); nil — действующая, только на live.
	Margin *MarginOverride
	// MarginProbe — снимок выдачи до политики маржи (симуляция): заполняется, порядок не меняется.
	MarginProbe *MarginProbe
}

// rulesFor — словари для конфигурации: B1–B3 — версия 1 (исходные словари каталога), B4 и live (витрина) —
// последняя принятая версия; override — снимок песочницы.
func (s *Services) rulesFor(ctx context.Context, cfg string, lx *searchLexicon, override *domain.RulesSnapshot) *searchRules {
	if override != nil {
		r, err := rulesFromSnapshot(override)
		if err == nil {
			return r
		}
		zap.L().Warn("поиск: снимок правил не разобран — исходные словари", zap.Error(err))
	}
	base := newSearchRules(1, lx.baseAliases)
	if cfg != domain.ConfigB4 && cfg != domain.ConfigLive || !s.repos.Enabled() {
		return base
	}
	if c := s.core.latest.Load(); c != nil && c.lexOf == lx && time.Since(c.at) < rulesTTL {
		return c.rules
	}
	r := base
	snap, err := s.repos.LatestRules(ctx)
	switch {
	case err != nil:
		zap.L().Warn("поиск: последняя версия правил не прочитана — исходные словари", zap.Error(err))
	case snap != nil && snap.Version > 1:
		if parsed, perr := rulesFromSnapshot(snap); perr == nil {
			r = parsed
		} else {
			zap.L().Warn("поиск: правила версии не разобраны", zap.Int("version", snap.Version), zap.Error(perr))
		}
	}
	s.core.latest.Store(&cachedRules{rules: r, lexOf: lx, at: time.Now()})
	return r
}

// queryEntities — что названо в запросе: вендоры, семейства, категория, референс аналога, непонятные слова.
type queryEntities struct {
	vendors  []string
	families []string
	// named — семейства, названные в запросе целиком (familyCoverage ≥ namedCoverageMin): ступень выше
	// остальных семейств того же вендора (rank.go tierOf).
	named     []string
	reference []string
	category  string
	// categoryByWords — категория найдена по словам названия категории или назначения («чертежи»), а не
	// алиасом: маршрут она не меняет (иначе запрос назначения уходит с маршрута C без реранкера).
	categoryByWords bool
	// purposeWords — слова категорий за словами назначения запроса.
	purposeWords []string
	// unknown — значимые слова, которых нет ни в каталоге, ни в словарях.
	unknown []string
	// latinUnknown — среди непонятных есть латинское слово: скорее всего, это название продукта.
	latinUnknown bool
	nonProduct   bool
	// noise — запрос не о товаре и без смысла: разметка, SQL, одни знаки или цифры (isNoiseQuery). Такой
	// запрос не идёт на маршрут D: модель за него не платится, ответ — честное «нет».
	noise bool
	// significant — значимых слов (не служебных, не атрибутов, не коды).
	significant int
	// abbrev — все непонятные слова — короткие кириллические (до 4 букв): похоже на сокращение, которого нет
	// в словаре («кис»); такой запрос не объявляется отсутствующим товаром, его пробуют найти по смыслу.
	abbrev bool
	// vendorTokens — слова запроса, которые называют вендора: вендор уже строгий фильтр, в обязательные
	// слова текстового поиска они не идут.
	vendorTokens map[string]bool
	// textTokens — слова для текстового поиска (без атрибутов и слов намерения).
	textTokens []string
	// absentName — название товара, которого нет в каталоге, как его назвал разбор маршрута D («SolidWorks»).
	absentName string
	// parsedFamily — семейство, которое назвал разбор маршрута D (правила его не нашли).
	parsedFamily string
	// entityTokens — значимые слова запроса и полные слова недописанных: по ним ищутся семейства.
	entityTokens []string
}

// nonProductStems — вопросы не о товаре: документы, доставка, оплата, заказ.
var nonProductStems = []string{"документ", "акт", "сверк", "упд", "реквизит", "счет", "счёт", "доставк", "оплат",
	"заказ", "накладн", "договор", "возврат"}

func isNonProductWord(w string) bool {
	for _, s := range nonProductStems {
		if strings.HasPrefix(w, s) && (len([]rune(w)) <= len([]rune(s))+4) {
			return true
		}
	}
	return false
}

// findEntities — сущности запроса по словарю каталога.
func findEntities(nr normResult, attrs queryAttrs, lx *searchLexicon, rules *searchRules) queryEntities {
	var e queryEntities
	medium := map[string]bool{}
	if nr.correction != nil && !nr.correction.applied {
		for _, w := range strings.Fields(nr.correction.from) {
			medium[w] = true
		}
	}
	for i, t := range nr.tokens {
		if attrs.consumed[i] {
			continue
		}
		e.textTokens = append(e.textTokens, t)
		if lexStopWords[t] || isProtectedToken(t) || len(wordsOf(t)) == 0 {
			continue
		}
		if isNonProductWord(t) {
			e.nonProduct = true
		}
		e.significant++
		if !knownWord(t, lx, rules) && !medium[t] && nr.prefixes[t] == "" {
			e.unknown = append(e.unknown, t)
			for _, r := range t {
				if r >= 'a' && r <= 'z' {
					e.latinUnknown = true
					break
				}
			}
		}
	}
	e.noise = isNoiseQuery(nr, lx, rules)
	e.abbrev = len(e.unknown) == 1 && !e.latinUnknown
	for _, u := range e.unknown {
		e.abbrev = e.abbrev && utf8.RuneCountInString(u) <= 4
	}
	var entityTokens []string
	for _, t := range e.textTokens {
		if !lexStopWords[t] {
			entityTokens = append(entityTokens, t)
		}
		if p := nr.prefixes[t]; p != "" {
			entityTokens = append(entityTokens, p)
		}
	}
	e.entityTokens = entityTokens
	e.vendors = lx.matchVendors(entityTokens, rules)
	e.vendorTokens = map[string]bool{}
	for _, t := range entityTokens {
		if p := strings.Join(wordsOf(t), " "); lx.vendorPhrases[p] != "" || rules != nil && rules.vendorPhrases[p] != "" {
			e.vendorTokens[t] = true
		}
	}
	seenV := map[string]bool{}
	for _, v := range e.vendors {
		seenV[v] = true
	}
	for _, a := range nr.aliasHits {
		switch a.Target {
		case domain.AliasTargetVendor:
			if _, ok := lx.vendorName[a.TargetID]; ok && !seenV[a.TargetID] {
				seenV[a.TargetID] = true
				e.vendors = append(e.vendors, a.TargetID)
			}
		case domain.AliasTargetCategory:
			if _, ok := lx.categoryName[a.TargetID]; ok {
				e.category = a.TargetID
			}
		}
	}
	if e.category == "" {
		e.category, e.purposeWords = lx.categoryOf(entityTokens)
		e.categoryByWords = e.category != ""
	}
	e.families = lx.matchFamilies(entityTokens)
	for _, f := range e.families {
		if lx.familyCoverage(f, entityTokens) >= namedCoverageMin {
			e.named = append(e.named, f)
		}
	}
	if attrs.analog {
		e.reference = e.families
		if len(e.reference) == 0 {
			for _, v := range e.vendors {
				for _, f := range lx.familyOrder {
					if lx.familyVendor[f] == v {
						e.reference = append(e.reference, f)
					}
				}
			}
		}
	}
	return e
}

// applyRemovedEntities — снятые покупателем чипы вендора, аналога и категории.
func (e *queryEntities) applyRemoved(removed map[string]map[string]bool) {
	filter := func(xs []string, field string) []string {
		var out []string
		for _, x := range xs {
			if !removed[field][x] {
				out = append(out, x)
			}
		}
		return out
	}
	e.vendors = filter(e.vendors, fieldVendor)
	e.reference = filter(e.reference, fieldAnalogOf)
	if removed[fieldCategory][e.category] {
		e.category = ""
	}
}

// routeSignals — маршрут по сигналам ARCHITECTURE §2.2 (без LLM). Сигналы — в трассу.
func routeSignals(nr normResult, attrs queryAttrs, ent queryEntities) (string, []string) {
	if ent.noise {
		return domain.RouteNormalized, []string{signalNoise}
	}
	var signals []string
	significant := 0
	for _, t := range ent.textTokens {
		if !lexStopWords[t] {
			significant++
		}
	}
	if len(ent.unknown) > 0 {
		signals = append(signals, "unknown_words")
	}
	if attrs.analog {
		signals = append(signals, "intent_words")
	}
	if significant+len(attrs.consumed) > 5 {
		signals = append(signals, "long_query")
	}
	if ent.nonProduct {
		signals = append(signals, "non_product")
	}
	if attrs.renewal && attrs.licType == domain.LicensePerpetual {
		signals = append(signals, "conflicting_attributes")
	}
	if len(signals) > 0 {
		return domain.RouteIntent, signals
	}
	norm := map[string]bool{}
	for _, c := range nr.changes {
		if c.Kind != changeClean {
			norm[c.Kind] = true
		}
	}
	for _, k := range []string{changeLayout, changeTypo, changeAlias, changeTranslit, changePrefix} {
		if norm[k] {
			signals = append(signals, k+"_hit")
		}
	}
	found := 0
	for _, f := range attrs.found {
		if f.Field != fieldCategory || !ent.categoryByWords {
			found++
		}
	}
	if found > 0 || attrs.quantity > 0 || attrs.renewal || attrs.origin != "" || attrs.licType != "" || attrs.duration > 0 {
		signals = append(signals, "attributes")
	}
	if len(signals) > 0 {
		return domain.RouteNormalized, signals
	}
	for _, t := range ent.textTokens {
		if looksLikeSKU(t) {
			return domain.RouteExact, []string{"sku"}
		}
	}
	if len(ent.families) > 0 || len(ent.vendors) > 0 {
		return domain.RouteExact, []string{"exact_name"}
	}
	return domain.RouteSemantic, []string{"no_entities"}
}

// looksLikeSKU — артикул: буквы и цифры с дефисом или длинный числовой код.
func looksLikeSKU(t string) bool {
	if len(t) >= 8 && strings.Trim(t, "0123456789") == "" {
		return true
	}
	return len(t) >= 6 && hasDigit(t) && strings.ContainsAny(t, "-") || len(t) >= 9 && hasDigit(t) && strings.ContainsAny(t, "abcdefghijklmnopqrstuvwxyz")
}

// rulesGuess — то, что правила знают о запросе, в форме domain.Intent для разборщика маршрута D.
func rulesGuess(attrs queryAttrs, ent queryEntities) domain.Intent {
	in := domain.Intent{Kind: domain.IntentFindProduct, Source: domain.ParseRules, Origin: attrs.origin,
		CategoryID: ent.category, Constraints: []domain.AttrConstraint{}, Preferences: []domain.AttrConstraint{},
		Unresolved: attrs.unresolved()}
	switch {
	case ent.nonProduct:
		in.Kind = domain.IntentNonProduct
	case attrs.analog:
		in.Kind = domain.IntentFindAlternative
	case attrs.renewal:
		in.Kind = domain.IntentRenewal
	}
	if len(ent.reference) > 0 {
		in.ReferenceFamily = ent.reference[0]
	}
	if len(ent.vendors) == 1 {
		in.VendorID = ent.vendors[0]
	}
	add := func(strict bool, field, value string) {
		c := domain.AttrConstraint{Field: field, Value: value}
		if strict {
			in.Constraints = append(in.Constraints, c)
		} else {
			in.Preferences = append(in.Preferences, c)
		}
	}
	if attrs.quantity > 0 {
		add(true, domain.FieldLicenseQuantity, itoa(attrs.quantity))
	}
	if attrs.metric != "" {
		add(true, domain.FieldLicenseMetric, attrs.metric)
	}
	if attrs.renewal {
		add(true, domain.FieldRenewal, "true")
	}
	if attrs.origin != "" {
		add(true, domain.FieldOrigin, attrs.origin)
	}
	if attrs.licType != "" {
		add(false, domain.FieldLicenseType, attrs.licType)
	}
	if attrs.duration > 0 {
		add(false, domain.FieldLicenseDuration, itoa(attrs.duration))
	}
	return in
}

// mergeParsed — разбор маршрута D поверх правил: модель дополняет, явно извлечённое правилами остаётся.
func mergeParsed(in domain.Intent, attrs *queryAttrs, ent *queryEntities, lx *searchLexicon) {
	if in.Kind == domain.IntentFindAlternative {
		attrs.analog = true
	}
	if in.Kind == domain.IntentRenewal {
		attrs.renewal = true
	}
	if in.Kind == domain.IntentNonProduct {
		ent.nonProduct = true
	}
	if f := in.ReferenceFamily; f != "" && lx.families[f] != nil {
		if attrs.analog {
			if len(ent.reference) == 0 {
				ent.reference = []string{f}
			}
		} else if len(ent.families) == 0 {
			ent.families = []string{f}
			ent.parsedFamily = f
		}
		ent.unknown = nil
	}
	if v := in.VendorID; v != "" && lx.vendorName[v] != "" && len(ent.vendors) == 0 && !attrs.analog {
		ent.vendors = []string{v}
	}
	if c := in.CategoryID; c != "" && lx.categoryName[c] != "" && ent.category == "" {
		ent.category = c
	}
	if in.Origin == "ru" {
		attrs.origin = "ru"
	}
	// Мягкие условия (домыслены моделью, а не названы покупателем): метрика — только предпочтение при выборе
	// лицензии семейства, происхождение — не фильтр. Тип и срок лицензии мягкие и так.
	for _, c := range in.Preferences {
		switch c.Field {
		case domain.FieldLicenseMetric:
			if attrs.metric == "" && metricForms[c.Value][0] != "" {
				attrs.metricPref = c.Value
			}
		case domain.FieldLicenseType, domain.FieldLicenseDuration, domain.FieldLicenseQuantity:
			mergeConstraint(c, attrs)
		}
	}
	for _, c := range in.Constraints {
		mergeConstraint(c, attrs)
	}
}

// mergeConstraint — обязательное условие разбора поверх правил: заполняет только пустое.
func mergeConstraint(c domain.AttrConstraint, attrs *queryAttrs) {
	switch c.Field {
	case domain.FieldLicenseQuantity:
		if n := atoi(c.Value); n > 0 && attrs.quantity == 0 {
			attrs.quantity = n
			attrs.metricUnresolved = attrs.metric == ""
		}
	case domain.FieldLicenseMetric:
		if attrs.metric == "" && metricForms[c.Value][0] != "" {
			attrs.metric, attrs.metricUnresolved = c.Value, false
		}
	case domain.FieldLicenseType:
		if attrs.licType == "" && (c.Value == domain.LicensePerpetual || c.Value == domain.LicenseSubscription) {
			attrs.licType = c.Value
		}
		if c.Value == domain.LicenseRenewal {
			attrs.renewal = true
		}
	case domain.FieldLicenseDuration:
		if n := atoi(c.Value); n > 0 && attrs.duration == 0 {
			attrs.duration = n
		}
	case domain.FieldOrigin:
		if c.Value == "ru" {
			attrs.origin = "ru"
		}
	case domain.FieldRenewal:
		attrs.renewal = attrs.renewal || c.Value == "true"
	}
}

func itoa(n int) string { return strconv.Itoa(n) }

func atoi(s string) int {
	n, err := strconv.Atoi(strings.TrimSpace(s))
	if err != nil {
		return 0
	}
	return n
}

// absent — точного товара, судя по словам запроса, в каталоге нет: есть латинское название, которого каталог
// не знает, или непонятных слов больше половины значимых (и это не похоже на сокращение).
func (e queryEntities) absent() bool {
	if e.nonProduct {
		return true
	}
	if len(e.unknown) == 0 {
		return false
	}
	if e.latinUnknown {
		return true
	}
	if e.category != "" {
		return false // названа задача («созваниваться», «чертежи»): ищем по категории, а не объявляем отсутствие
	}
	return !e.abbrev && len(e.unknown)*2 > e.significant
}

// signalNoise — сигнал маршрута для шума: разбор правилами, без модели.
const signalNoise = "noise"

// sqlLike — похоже на SQL или инъекцию: «' OR 1=1 --», «SELECT * FROM products», «DROP TABLE».
var sqlLike = regexp.MustCompile(`(?i)\b(select|insert|update|delete|drop|union|alter|truncate)\b.*\b(from|into|table|where|set|select|all)\b` +
	`|'\s*(or|and)\b|\b\d+\s*=\s*\d+\b|--\s*$|;\s*--|/\*`)

// isNoiseQuery — запрос не о товаре по форме: разметка или код, SQL, меньше двух букв
// без единого слова каталога и артикула («!!!», «%%%», «№1», «123»). «1с», «r7» — слова каталога, не шум.
func isNoiseQuery(nr normResult, lx *searchLexicon, rules *searchRules) bool {
	if looksLikeMarkup(nr.raw) || sqlLike.MatchString(nr.raw) {
		return true
	}
	if hasLetters(nr.normalized()) {
		return false
	}
	for _, t := range nr.tokens {
		if looksLikeSKU(t) || lx != nil && strings.IndexFunc(t, unicode.IsLetter) >= 0 && knownWord(t, lx, rules) {
			return false
		}
	}
	return true
}
