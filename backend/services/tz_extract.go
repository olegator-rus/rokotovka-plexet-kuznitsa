package services

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"time"
	"unicode"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/out"
	"kuznica/api/prompts"

	"go.uber.org/zap"
)

// Извлечение позиций ТЗ моделью (контур ТЗ, TZ-CORE §5). Модель — та же, что разбирает запрос (Flash), ответ —
// JSON-объект, текст документа — без ПДн (tz_pii.go + llm.MaskPII в клиенте). Бюджет и квота адреса — общие с
// разбором запроса (llmguard.go), расход — строкой llm_usage. После модели код проверяет цитаты, названия,
// количества, номера и предел 50 и сам собирает плашки, purchase и filters.

const (
	tzMaxPositions = 50
	// tzChunkRunes — потолок текста документа для модели (около 12 тыс. токенов): дальше документ не читается.
	tzChunkRunes = 40_000
	// tzMaxTokens — потолок ответа на позицию; tzListMaxTokens — на перечень позиций.
	tzMaxTokens     = 1500
	tzListMaxTokens = 3000
	// tzParallel — вызовов модели извлечения одновременно на процесс.
	tzParallel = 8
	// tzCallTimeout — срок одного вызова модели (HTTP-клиент разбора режет на 30 с).
	tzCallTimeout = 30 * time.Second
	tzShortTitle  = 30
	tzMaxQuote    = 400
	tzMaxReq      = 8
)

// Виды позиции в ответе API.
const (
	tzKindProduct = "product"
	tzKindService = "service"
)

// Ошибки извлечения.
var (
	errTZModelOff = errors.New("tz: модель недоступна")
	errTZModel    = errors.New("tz: модель не ответила по схеме")
)

// tzRaw — позиция в ответе модели.
type tzRaw struct {
	Number         json.Number `json:"number"`
	Title          string      `json:"title"`
	ShortTitle     string      `json:"short_title"`
	Type           string      `json:"type"`
	Named          bool        `json:"named"`
	Vendor         string      `json:"vendor"`
	Product        string      `json:"product"`
	Edition        string      `json:"edition"`
	Quantity       *float64    `json:"quantity"`
	Unit           string      `json:"unit"`
	QuantityText   string      `json:"quantity_text"`
	Metric         string      `json:"metric"`
	DurationMonths *float64    `json:"duration_months"`
	Perpetual      bool        `json:"perpetual"`
	Deal           string      `json:"deal"`
	OrgType        string      `json:"org_type"`
	SearchQuery    string      `json:"search_query"`
	SourceQuote    string      `json:"source_quote"`
	Unverified     []string    `json:"unverified_requirements"`
	// skip — позиция сверх предела: только номер, в ответ не идёт, но считается в extracted_count.
	skip bool
}

// tzModel — модель извлечения: модель разбора запроса; nil — нет.
func (s *Services) tzModel() ChatModel {
	if m := tzModelOverride(s); m != nil {
		return m
	}
	if p, ok := s.intent.(*intentService); ok && p != nil {
		return p.primary
	}
	return nil
}

// tzModelBlocked — почему модель сейчас не зовётся ("" — можно).
func (s *Services) tzModelBlocked(ctx context.Context) string {
	switch {
	case s.tzModel() == nil:
		return "llm_not_configured"
	case s.LLMMode() != llmModeOn:
		return "llm_off"
	case s.guard().buyerExceeded(ctx):
		return fallbackBudget
	case s.guard().addrExceeded(ctx):
		return fallbackIPQuota
	}
	return ""
}

// tzExtract — позиции документа и полное число найденных (до обрезки до 50).
//
// Один вызов модели на позицию, параллельно (ответ на восемь позиций разом длиннее срока вызова, а массив
// объектов Flash заполняет ненадёжно): номера позиций — из таблицы спецификации, без таблицы — из короткого
// перечня, который модель составляет первым вызовом. В каждый вызов идёт весь текст документа, поэтому
// требования к позиции ниже таблицы модель видит всегда.
func (s *Services) tzExtract(ctx context.Context, text string) ([]out.TZPosition, int, error) {
	masked := tzClipRunes(tzMaskText(text), tzChunkRunes)
	if reason := s.tzModelBlocked(ctx); reason != "" {
		zap.L().Warn("tz: модель не вызвана", zap.String("reason", reason))
		return nil, 0, errTZModelOff
	}
	nums := tzSpecNumbers(masked)
	if len(nums) == 0 {
		var err error
		if nums, err = s.tzList(ctx, masked); err != nil {
			return nil, 0, err
		}
	}
	if len(nums) == 0 {
		return nil, 0, nil
	}
	work := nums
	if len(work) > tzMaxPositions {
		work = work[:tzMaxPositions] // сверх 50 модель не зовём: их число известно (extracted_count)
	}
	results := make([]*tzRaw, len(work))
	errsOf := make([]error, len(work))
	var wg sync.WaitGroup
	for i, n := range work {
		wg.Add(1)
		go func() {
			defer wg.Done()
			select {
			case tzSem <- struct{}{}:
			case <-ctx.Done():
				errsOf[i] = ctx.Err()
				return
			}
			defer func() { <-tzSem }()
			results[i], errsOf[i] = s.tzCall(ctx, masked, n)
		}()
	}
	wg.Wait()
	var raws []tzRaw
	for i := range work {
		if errsOf[i] != nil {
			return nil, 0, errsOf[i]
		}
		if results[i] != nil {
			raws = append(raws, *results[i])
		}
	}
	for _, n := range nums[len(work):] {
		raws = append(raws, tzRaw{Number: json.Number(strconv.Itoa(n)), skip: true})
	}
	positions, extracted := tzCheck(raws, masked)
	return positions, extracted, nil
}

// tzSem — вызовов модели извлечения одновременно на процесс.
var tzSem = make(chan struct{}, tzParallel)

// tzSpecNumbers — номера строк таблицы спецификации («N | наименование | …») по порядку, без повторов.
func tzSpecNumbers(text string) []int {
	var nums []int
	seen := map[int]bool{}
	for _, line := range strings.Split(text, "\n") {
		cells := strings.Split(line, " | ")
		if len(cells) < 3 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(cells[0]), "."))
		if err != nil || n <= 0 || n >= 100000 || seen[n] {
			continue
		}
		seen[n] = true
		nums = append(nums, n)
	}
	return nums
}

// tzList — номера позиций документа без таблицы: короткий перечень моделью.
func (s *Services) tzList(ctx context.Context, masked string) ([]int, error) {
	doc, _ := json.Marshal(masked)
	content, err := s.tzChat(ctx, prompts.TZList().Text, "<document>"+string(doc)+"</document>", tzListMaxTokens,
		func(c string) error { _, e := tzDecodeList(c); return e })
	if err != nil {
		return nil, err
	}
	items, _ := tzDecodeList(content)
	var nums []int
	seen := map[int]bool{}
	for i, it := range items {
		n := tzNumber(it.Number, i+1)
		if !seen[n] {
			seen[n] = true
			nums = append(nums, n)
		}
	}
	return nums, nil
}

// tzDecodeList — перечень позиций из ответа модели.
func tzDecodeList(content string) ([]struct {
	Number json.Number `json:"number"`
	Title  string      `json:"title"`
}, error) {
	var r struct {
		Items *[]struct {
			Number json.Number `json:"number"`
			Title  string      `json:"title"`
		} `json:"items"`
	}
	if err := json.Unmarshal([]byte(tzObject(content)), &r); err != nil {
		return nil, fmt.Errorf("не JSON: %w", err)
	}
	if r.Items == nil {
		return nil, errors.New("нет поля items")
	}
	return *r.Items, nil
}

// tzCall — одна позиция с номером num; nil — модель такой позиции не нашла.
func (s *Services) tzCall(ctx context.Context, masked string, num int) (*tzRaw, error) {
	doc, _ := json.Marshal(masked)
	user := "<document>" + string(doc) + "</document>\n\nЗадание: разбери позицию № " + strconv.Itoa(num) + "."
	content, err := s.tzChat(ctx, prompts.TZ().Text, user, tzMaxTokens, func(c string) error { _, e := tzDecode(c); return e })
	if err != nil {
		return nil, err
	}
	r, _ := tzDecode(content)
	if (r.Number == "" || r.Number == "null") && strings.TrimSpace(r.Title) == "" {
		return nil, nil
	}
	r.Number = json.Number(strconv.Itoa(num)) // номер задан кодом: модель отвечает про запрошенную позицию
	return &r, nil
}

// tzChat — вызов модели с учётом расхода; ответ не по схеме (check) — один повтор.
func (s *Services) tzChat(ctx context.Context, system, user string, maxTokens int, check func(string) error) (string, error) {
	m := s.tzModel()
	msgs := []llm.Message{{Role: "system", Content: system}, {Role: "user", Content: user}}
	for attempt := 0; attempt < 2; attempt++ {
		if err := ctx.Err(); err != nil {
			return "", err
		}
		cctx, cancel := context.WithTimeout(ctx, tzCallTimeout)
		zero := 0.0
		req := llm.Request{Messages: msgs, MaxTokens: maxTokens, Temperature: &zero, ResponseFormat: llm.JSONObject}
		if strings.HasPrefix(llmModelName(m.Model()), "qwen") {
			req.ReasoningEffort = "none"
		}
		started := time.Now()
		resp, err := m.Chat(cctx, req)
		cancel()
		call := llmCall{Model: m.Model(), Latency: time.Since(started), Status: llmStatusOK}
		var perr error
		switch {
		case err != nil && errors.Is(err, context.DeadlineExceeded):
			call.Status = llmStatusTimeout
		case err != nil:
			call.Status = llmStatusError
		default:
			call.InputTokens, call.OutputTokens, call.CachedTokens = resp.Usage.PromptTokens, resp.Usage.CompletionTokens, resp.Usage.CachedTokens()
			if perr = check(resp.Message.Content); perr != nil {
				call.Status = llmStatusSchemaError
			}
		}
		s.recordLLMUsage(ctx, call)
		if err != nil {
			zap.L().Warn("tz: вызов модели не удался", zap.String("model", llmModelName(m.Model())), zap.Error(err))
			return "", errTZModel
		}
		if perr == nil {
			return resp.Message.Content, nil
		}
		zap.L().Warn("tz: ответ модели не по схеме", zap.Int("attempt", attempt+1), zap.Error(perr))
		msgs = append(msgs,
			llm.Message{Role: "assistant", Content: intentTruncate(resp.Message.Content, 600)},
			llm.Message{Role: "user", Content: "Ответ не разобран. Верни только один JSON-объект по схеме из системного сообщения."})
	}
	return "", errTZModel
}

// tzObject — JSON-объект из ответа: лишний текст вокруг отрезается.
func tzObject(content string) string {
	c := strings.TrimSpace(content)
	if i := strings.Index(c, "{"); i > 0 {
		c = c[i:]
	}
	if j := strings.LastIndex(c, "}"); j >= 0 && j < len(c)-1 {
		c = c[:j+1]
	}
	return c
}

// tzDecode — позиция из ответа модели.
func tzDecode(content string) (tzRaw, error) {
	var r tzRaw
	dec := json.NewDecoder(strings.NewReader(tzObject(content)))
	dec.UseNumber()
	if err := dec.Decode(&r); err != nil {
		return tzRaw{}, fmt.Errorf("не JSON: %w", err)
	}
	return r, nil
}

// tzClipRunes — не длиннее n знаков.
func tzClipRunes(s string, n int) string {
	if utf8.RuneCountInString(s) <= n {
		return s
	}
	return string([]rune(s)[:n])
}

// --- проверки кода после модели ---

// tzCheck — проверенные позиции в порядке документа и полное число извлечённых (до обрезки).
func tzCheck(raws []tzRaw, text string) ([]out.TZPosition, int) {
	norm := tzNorm(text)
	rows := tzTableRows(text)
	seen := map[int]bool{}
	var res []out.TZPosition
	extracted := 0
	for i, r := range raws {
		num := tzNumber(r.Number, i+1)
		if seen[num] {
			continue // номера не повторяются: оставить первую (куски документа могли дать одну позицию дважды)
		}
		title := tzClean(r.Title, 500)
		if !r.skip && title == "" && rows[num] == "" {
			continue
		}
		seen[num] = true
		extracted++
		if r.skip || len(res) >= tzMaxPositions {
			continue
		}
		res = append(res, tzPosition(r, num, title, norm, rows, tzSection(text, num)))
	}
	return res, extracted
}

// tzPosition — позиция ответа API: проверенные поля модели, плашки, purchase и filters — кодом.
func tzPosition(r tzRaw, num int, title, norm string, rows map[int]string, section string) out.TZPosition {
	// Название — строка таблицы спецификации с тем же номером (дословно, без пересказа модели); таблицы нет —
	// название модели, если оно дословно есть в тексте.
	if row := rows[num]; row != "" {
		title = row
	} else if !strings.Contains(norm, tzNorm(title)) {
		title = tzTruncate(title, 300) // в тексте нет: оставляем, но это видно по цитате
	}
	kind := tzKindProduct
	typ := strings.ToLower(strings.TrimSpace(r.Type))
	if typ == "service" {
		kind = tzKindService
	}
	short := tzClean(r.ShortTitle, 200)
	if short == "" {
		short = title
	}
	short = tzTruncate(short, tzShortTitle)
	query := tzClean(r.SearchQuery, 200)
	if !r.Named {
		query = tzStripSpecs(query) // класс товара: характеристики («2U», «64 ГБ») сужают поиск до пустой выдачи
	} else if typ == "software" {
		query = tzObjectWords(query, tzNorm(title+" "+rows[num]))
	}
	if query == "" {
		query = tzTruncate(title, 120)
	}
	quote := tzClean(r.SourceQuote, 2000)
	if quote != "" {
		quote = tzVerifyQuote(quote, norm)
	}
	p := out.TZPosition{
		ID: "p" + strconv.Itoa(num), Number: num, Title: title, ShortTitle: short, Kind: kind, Named: r.Named,
		SearchQuery: query, SourceQuote: quote, Attributes: []out.TZAttribute{}, Filters: []out.TZFilter{},
		UnverifiedRequirements: []string{},
	}
	if kind == tzKindService {
		p.Named = false
	}
	// Своё место позиции в документе: раздел требований, строка таблицы, название и цитата. По нему проверяются
	// тип покупки, организация, метрика и непроверяемые требования — модель любит дописывать их «по умолчанию».
	own := tzNorm(strings.Join([]string{section, rows[num], title, quote}, " "))
	p.UnverifiedRequirements = tzRequirements(r.Unverified, own)
	tzConditions(&p, r, typ, norm, own, tzNorm(title+" "+rows[num]))
	// ПДн в позицию не попадают ни из какого поля (страховка поверх маскирования входа).
	for _, f := range []*string{&p.Title, &p.ShortTitle, &p.SearchQuery} {
		if tzHasPII(*f) {
			*f = tzScrub(*f)
		}
	}
	if tzHasPII(p.SourceQuote) {
		p.SourceQuote = ""
	}
	return p
}

// tzConditions — количество, срок, тип покупки и организация: плашки, purchase и filters.
func tzConditions(p *out.TZPosition, r tzRaw, typ, norm, own, head string) {
	qty := 0
	if r.Quantity != nil && *r.Quantity >= 1 && *r.Quantity <= maxQuoteQuantity && *r.Quantity == float64(int(*r.Quantity)) {
		qty = int(*r.Quantity)
	}
	qtyText := tzClean(r.QuantityText, 200)
	if qtyText != "" && !strings.Contains(norm, tzNorm(qtyText)) {
		qtyText = "" // составное количество — только дословное из документа
	}
	if nums := reDigits.FindAllString(qtyText, -1); len(nums) < 2 {
		// Не составное («50 (пятидесяти) узлов»): одно число — это и есть количество.
		if len(nums) == 1 && qty == 0 {
			if n, err := strconv.Atoi(nums[0]); err == nil && n >= 1 && n <= maxQuoteQuantity {
				qty = n
			}
		}
		qtyText = ""
	}
	unit := tzClean(r.Unit, 40)
	months := 0
	if r.DurationMonths != nil && *r.DurationMonths >= 1 && *r.DurationMonths <= maxQuoteDuration && *r.DurationMonths == float64(int(*r.DurationMonths)) {
		months = int(*r.DurationMonths)
	}
	software := typ == "software"
	metric := ""
	if software {
		// Метрика — по единице из документа («узел», «пользователь»), иначе — если её слово есть в названии
		// позиции («лицензия для рабочей станции», «серверная лицензия»). «шт.» сама метрику не задаёт.
		metric = tzMetricOfUnit(unit)
		if m := strings.ToLower(strings.TrimSpace(r.Metric)); metric == "" && tzMetricNamed(m, head) {
			metric = m
		}
	}
	deal := ""
	switch d := strings.ToLower(strings.TrimSpace(r.Deal)); {
	case !software:
	case d == DealRenewal && strings.Contains(own, "продл") && !strings.Contains(own, "не продл"):
		deal = d
	case d == DealNew && reNewLicense.MatchString(own):
		deal = d
	}
	org := ""
	if o := strings.ToLower(strings.TrimSpace(r.OrgType)); software && quoteOrgs[o] && tzOrgNamed(o, own) {
		org = o
	}
	edition := ""
	if software {
		edition = tzEdition(r.Edition, own)
	}

	switch {
	case qtyText != "":
		p.Attributes = append(p.Attributes, out.TZAttribute{Key: "quantity", Label: "Количество", Value: qtyText})
		qty = 0 // составное: одним числом в расчёт не идёт
	case qty > 0:
		p.Attributes = append(p.Attributes, out.TZAttribute{Key: "quantity", Label: "Количество", Value: tzQuantityLabel(qty, unit)})
	}
	switch {
	case months > 0:
		p.Attributes = append(p.Attributes, out.TZAttribute{Key: "duration", Label: "Срок", Value: duration(months)})
	case software && r.Perpetual:
		p.Attributes = append(p.Attributes, out.TZAttribute{Key: "duration", Label: "Срок", Value: "Бессрочная"})
	}
	if deal != "" {
		p.Attributes = append(p.Attributes, out.TZAttribute{Key: "deal", Label: "Тип", Value: dealLabels[deal]})
	}
	if org != "" {
		p.Attributes = append(p.Attributes, out.TZAttribute{Key: "org_type", Label: "Организация", Value: orgLabels[org]})
	}

	if kind := p.Kind; kind == tzKindService {
		// Работы и услуги: условия показываются, в поиск и расчёт не идут; поиск по позиции не даёт точного
		// совпадения (tz_search.go: tzServiceResult).
		p.Filters = append(p.Filters, out.TZFilter{Key: tzFilterKind, Op: "eq", Value: tzKindService})
		if qty > 0 {
			p.Purchase = &domain.QuotePurchase{Query: p.SearchQuery, Quantity: &qty}
		}
		return
	}
	pur := &domain.QuotePurchase{Query: p.SearchQuery, Deal: deal, Metric: metric, OrgType: org, Edition: edition}
	if qty > 0 {
		q := qty
		pur.Quantity = &q
	}
	if months > 0 {
		m := months
		pur.DurationMonths = &m
	}
	p.Purchase = pur
	if !software {
		return // оборудование: фильтров лицензии нет, количество — в purchase для спецификации
	}
	if qty > 0 && metric != "" {
		p.Filters = append(p.Filters, out.TZFilter{Key: facetQuantity, Op: "eq", Value: strconv.Itoa(qty)},
			out.TZFilter{Key: facetMetric, Op: "eq", Value: metric})
	}
	if months > 0 {
		p.Filters = append(p.Filters, out.TZFilter{Key: facetDuration, Op: "eq", Value: strconv.Itoa(months)})
	}
	if edition != "" {
		// Редакция — дополнение к ключам facets_license: поиск с source = tz берёт её условием запроса (tz_search.go).
		p.Filters = append(p.Filters, out.TZFilter{Key: tzFilterEdition, Op: "eq", Value: edition})
	}
}

// reSpecToken — характеристика в запросе: число с единицей («2U», «64ГБ», «10 Гбит/с» — по словам).
var reSpecToken = regexp.MustCompile(`(?i)^\d+([.,]\d+)?(u|гб|тб|мб|gb|tb|гбит/с|гц|ghz|вт|w)?$`)

// tzStripSpecs — запрос класса товара без характеристик, единиц измерения и общих слов («ПО», «программное
// обеспечение», «данных»): поиск без модели ищет их буквально («восстановление данных» → терминалы сбора данных).
func tzStripSpecs(q string) string {
	var keep []string
	for _, w := range strings.Fields(q) {
		lw := strings.ToLower(w)
		if reSpecToken.MatchString(w) || reSpecUnit[lw] || tzGenericWords[lw] {
			continue
		}
		keep = append(keep, w)
	}
	for len(keep) > 0 && (keep[len(keep)-1] == "и" || keep[len(keep)-1] == "для") {
		keep = keep[:len(keep)-1]
	}
	if len(keep) == 0 {
		return q
	}
	return strings.Join(keep, " ")
}

var tzGenericWords = map[string]bool{"по": true, "программное": true, "обеспечение": true, "система": true,
	"системы": true, "решение": true, "программа": true, "программный": true, "комплекс": true, "данных": true}

var reSpecUnit = map[string]bool{"гб": true, "тб": true, "гбит/с": true, "мбит/с": true, "gb": true, "tb": true}

// tzObjectWords — объект лицензии из названия позиции, если запрос модели его потерял: «для рабочей станции»,
// «серверная». Без него у семейств с вариантами под сервер и рабочую станцию (Astra Linux) голова выбирается
// по случаю; слова названия ищутся текстом и выбирают вариант.
func tzObjectWords(query, head string) string {
	q := strings.ToLower(query)
	switch {
	case strings.Contains(head, "рабочей станци") && !strings.Contains(q, "рабоч"):
		return query + " для рабочей станции"
	case strings.Contains(head, "серверн") && !strings.Contains(q, "сервер"):
		return query + " серверная"
	}
	return query
}

// tzFilterEdition — ключ фильтра редакции позиции ТЗ.
const tzFilterEdition = "edition"

var reQuoted = regexp.MustCompile(`[«"“]([^»"”]{1,40})[»"”]`)

// tzEdition — редакция: одно название («уровень защищённости «Орёл»» → «Орёл»), которое есть в тексте позиции;
// версия («версия 5.0») редакцией не считается.
func tzEdition(raw, own string) string {
	e := tzClean(raw, 64)
	if m := reQuoted.FindStringSubmatch(e); m != nil {
		e = m[1]
	}
	e = strings.Trim(e, " «»\"“”'.,")
	if e == "" || reDigits.MatchString(e) && !regexp.MustCompile(`\p{L}{3,}`).MatchString(strings.TrimPrefix(strings.ToLower(e), "версия")) {
		return ""
	}
	if !strings.Contains(own, tzNorm(e)) {
		return ""
	}
	return tzTruncate(e, 64)
}

var reDigits = regexp.MustCompile(`\d+`)

// reNewLicense — «новая лицензия», «тип лицензии – новая», «новые лицензии» (не «нового поколения»).
var reNewLicense = regexp.MustCompile(`(^|[^\p{L}])нов(ая|ые|ую|ых)([^\p{L}]|$)`)

// tzMetricNamed — слово метрики m есть в названии позиции.
func tzMetricNamed(m, head string) bool {
	if m == "" {
		return false
	}
	for _, w := range strings.Fields(head) {
		if tzMetricOfUnit(strings.Trim(w, ".,;:()«»\"")) == m {
			return true
		}
	}
	return false
}

// tzOrgNamed — тип организации прямо назван в тексте позиции.
func tzOrgNamed(o, own string) bool {
	switch o {
	case OrgCommercial:
		return strings.Contains(own, "коммерческ")
	case OrgEducation:
		return strings.Contains(own, "образоват") || strings.Contains(own, "учебн")
	case OrgGovernment:
		return strings.Contains(own, "государствен") || strings.Contains(own, "бюджетн") || strings.Contains(own, "казенн")
	}
	return false
}

// tzMetricOfUnit — метрика по единице из документа; "" — неясно («шт.», «комплект»).
func tzMetricOfUnit(unit string) string {
	u := strings.ToLower(unit)
	for _, ms := range metricStems {
		if ms.metric != "" && strings.HasPrefix(u, ms.stem) {
			return ms.metric
		}
	}
	return ""
}

// tzUnitForms — формы единиц для плашки количества: «50 узлов», «2 шт.».
var tzUnitForms = []struct {
	stem  string
	forms [3]string
}{
	{"узел", [3]string{"узел", "узла", "узлов"}}, {"узл", [3]string{"узел", "узла", "узлов"}},
	{"пользовател", [3]string{"пользователь", "пользователя", "пользователей"}},
	{"устройств", [3]string{"устройство", "устройства", "устройств"}},
	{"рабоч", [3]string{"рабочая станция", "рабочие станции", "рабочих станций"}},
	{"сервер", [3]string{"сервер", "сервера", "серверов"}},
	{"ядр", [3]string{"ядро", "ядра", "ядер"}}, {"ядер", [3]string{"ядро", "ядра", "ядер"}},
	{"лиценз", [3]string{"лицензия", "лицензии", "лицензий"}},
	{"комплект", [3]string{"комплект", "комплекта", "комплектов"}},
	{"мест", [3]string{"место", "места", "мест"}},
	{"компьютер", [3]string{"компьютер", "компьютера", "компьютеров"}},
}

// tzQuantityLabel — «50 узлов»; единица без известных форм — как в документе («2 шт.», «1 усл. ед.»).
func tzQuantityLabel(qty int, unit string) string {
	u := strings.ToLower(strings.TrimSpace(unit))
	if u == "" {
		return strconv.Itoa(qty)
	}
	for _, f := range tzUnitForms {
		if strings.HasPrefix(u, f.stem) {
			return strconv.Itoa(qty) + " " + plural(qty, f.forms)
		}
	}
	return strconv.Itoa(qty) + " " + strings.TrimSpace(unit)
}

// tzRequirements — непроверяемые требования: короткие, без повторов и ПДн, не больше tzMaxReq, и только те, что
// есть в тексте самой позиции (own — нормализованный): не меньше половины значимых слов требования — в нём.
func tzRequirements(in []string, own string) []string {
	res := []string{}
	seen := map[string]bool{}
	for _, r := range in {
		r = tzClean(r, 200)
		k := strings.ToLower(r)
		if r == "" || seen[k] || tzHasPII(r) || !tzUnverifiable(k) || !tzGrounded(r, own) {
			continue
		}
		seen[k] = true
		res = append(res, tzTruncate(r, 120))
		if len(res) >= tzMaxReq {
			break
		}
	}
	return res
}

// tzUnverifiableStems — о чём каталог магазина не знает (TZ-CORE §5): сертификаты, реестр, происхождение,
// гарантия, совместимость с инфраструктурой заказчика, техническая поддержка. Функции и характеристики товара
// («видеоконференции», «64 ГБ») сюда не входят: их сверяет покупатель по карточке.
var tzUnverifiableStems = []string{"сертифик", "фстэк", "фсб", "реестр", "российск", "отечествен", "происхожд",
	"страна", "гаранти", "совместим", "поддержк", "импортозамещ", "гост"}

// tzUnverifiable — требование из тех, что нельзя проверить по каталогу (k — в нижнем регистре).
func tzUnverifiable(k string) bool {
	for _, st := range tzUnverifiableStems {
		if strings.Contains(k, st) {
			return true
		}
	}
	return false
}

// tzGrounded — не меньше половины значимых слов (от 4 букв, по первым пяти) есть в тексте позиции.
func tzGrounded(r, own string) bool {
	total, found := 0, 0
	for _, w := range strings.FieldsFunc(tzNorm(r), func(c rune) bool { return !unicode.IsLetter(c) && !unicode.IsDigit(c) }) {
		rs := []rune(w)
		if len(rs) < 4 {
			continue
		}
		total++
		if strings.Contains(own, string(rs[:min(5, len(rs))])) {
			found++
		}
	}
	return total > 0 && found*2 >= total
}

// reSection — начало раздела требований к позиции: «Позиция 3.», «Позиция № 3».
var reSection = regexp.MustCompile(`(?i)позици[яи]\s*№?\s*(\d+)`)

// tzSection — раздел требований к позиции num: от «Позиция num» до следующей «Позиция …», не длиннее 4000
// знаков; раздела нет — "".
func tzSection(text string, num int) string {
	locs := reSection.FindAllStringSubmatchIndex(text, -1)
	for i, l := range locs {
		if n, err := strconv.Atoi(text[l[2]:l[3]]); err != nil || n != num {
			continue
		}
		end := len(text)
		for _, nx := range locs[i+1:] {
			if m, err := strconv.Atoi(text[nx[2]:nx[3]]); err == nil && m != num {
				end = nx[0]
				break
			}
		}
		return tzClipRunes(text[l[0]:end], 4000)
	}
	return ""
}

// tzVerifyQuote — цитата, если она дословно есть в тексте (с точностью до пробелов, регистра, вида тире и кавычек;
// многоточие по краям — пересказ границы, отрезается); иначе "".
func tzVerifyQuote(q, norm string) string {
	q = strings.TrimSpace(q)
	for _, cut := range []string{"…", "..."} {
		q = strings.TrimSpace(strings.TrimSuffix(strings.TrimPrefix(q, cut), cut))
	}
	if utf8.RuneCountInString(q) < 8 || !strings.Contains(norm, tzNorm(q)) {
		return ""
	}
	return tzTruncate(q, tzMaxQuote)
}

// tzNorm — форма для сравнения «дословно»: регистр, ё, тире, кавычки, разделитель колонок и пробелы.
func tzNorm(s string) string {
	s = strings.ToLower(s)
	s = tzNormReplacer.Replace(s)
	return strings.TrimSpace(tzSpaces(s))
}

var tzNormReplacer = strings.NewReplacer(
	"ё", "е", "–", "-", "—", "-", "−", "-", "‑", "-", "«", "\"", "»", "\"", "“", "\"", "”", "\"", "„", "\"",
	"’", "'", "‘", "'", "­", "", " | ", " ", "|", " ",
)

// tzTableRows — наименования из строк таблицы «N | наименование | …» по номеру.
func tzTableRows(text string) map[int]string {
	rows := map[int]string{}
	for _, line := range strings.Split(text, "\n") {
		cells := strings.Split(line, " | ")
		if len(cells) < 2 {
			continue
		}
		n, err := strconv.Atoi(strings.TrimSuffix(strings.TrimSpace(cells[0]), "."))
		if err != nil || n <= 0 {
			continue
		}
		if _, ok := rows[n]; !ok {
			rows[n] = strings.TrimSpace(cells[1])
		}
	}
	return rows
}

// tzNumber — номер позиции из ответа модели; нет или мусор — порядковый.
func tzNumber(n json.Number, fallback int) int {
	if v, err := n.Int64(); err == nil && v > 0 && v < 100000 {
		return int(v)
	}
	if f, err := n.Float64(); err == nil && f >= 1 && f < 100000 {
		return int(f)
	}
	return fallback
}

// tzClean — строка без лишних пробелов и управляющих знаков, не длиннее max знаков.
func tzClean(s string, max int) string {
	s = strings.Map(func(r rune) rune {
		if r < 0x20 {
			return ' '
		}
		return r
	}, s)
	return tzTruncate(strings.TrimSpace(tzSpaces(s)), max)
}

// tzTruncate — не длиннее max знаков; обрезанное — с многоточием.
func tzTruncate(s string, max int) string {
	if utf8.RuneCountInString(s) <= max {
		return s
	}
	r := []rune(s)
	return strings.TrimSpace(string(r[:max-1])) + "…"
}

// tzScrub — строка без ПДн: метки вместо найденного.
func tzScrub(s string) string {
	s, _ = tzMaskNames(s)
	s, _ = maskPII(s)
	return s
}
