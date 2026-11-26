package services

import (
	"bufio"
	"context"
	"encoding/json"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"sort"
	"strconv"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Оценка режима подбора (контур подбора): диалоги набора (data/softline/eval/guide-dialogs.jsonl, собирает
// scripts/eval/guide_dialogs.py) проходят через тот же сервис подбора, что отвечает покупателю (s.Guide), трижды.
// Автопроверки — на ответе хода, независимо от проверок до показа внутри подбора: каталог — словарь поиска,
// факты и разметка офферов — файлы базы знаний, правила лицензий — своя реализация ключевых правил. Расход — журнал
// хода (guide_turns) и llm_usage по request_id хода. Сравнение: «поиск без подбора» — POST /api/search на реплики
// диалога; «модель без проверок» — эмуляция: к показанному полным подбором добавляется то, что сняли проверки до
// показа (guide_turns.removed), — модель и код в этом варианте те же, выключены только проверки.

// Варианты (evaluations.config, ≤ 4 знаков).
const (
	GuideVariantFull    = "gd"
	GuideVariantSearch  = "srch"
	GuideVariantNoCheck = "nock"
	guideEvalSet        = "guide"
)

// guideFactOK — порог показа факта (GUIDE_MIN_FACT_STATUS по умолчанию).
var guideFactOK = map[string]bool{"verified_agent": true, "verified": true}

// GuideDialog — диалог набора.
type GuideDialog struct {
	ID       string            `json:"id"`
	Vendor   string            `json:"vendor"`
	Type     string            `json:"type"`
	Scenario string            `json:"scenario"`
	Turns    []GuideDialogTurn `json:"turns"`
	Allowed  struct {
		FamilyIDs []string `json:"family_ids"`
		FactIDs   []string `json:"fact_ids"`
		RuleIDs   []string `json:"rule_ids"`
	} `json:"allowed"`
	Forbidden struct {
		Tools         []string `json:"tools"`
		FamiliesMatch []string `json:"families_match"`
		Words         []string `json:"words"`
	} `json:"forbidden"`
	Limits struct {
		MaxQuestions       int `json:"max_questions"`
		MaxLLMCallsPerTurn int `json:"max_llm_calls_per_turn"`
		MaxTurnMs          int `json:"max_turn_ms"`
	} `json:"limits"`
	Task string `json:"task"`
	Why  string `json:"why"`
}

// GuideDialogTurn — ход: запрос, реплика, чип или правка условия, и ожидания.
type GuideDialogTurn struct {
	Query     string               `json:"query,omitempty"`
	Message   string               `json:"message,omitempty"`
	Chip      *in.GuideChip        `json:"chip,omitempty"`
	Condition *in.GuideCondition   `json:"condition,omitempty"`
	Expect    GuideTurnExpectation `json:"expect"`
}

// GuideTurnExpectation — что должно быть в ответе хода.
type GuideTurnExpectation struct {
	TypeIn          []string `json:"type_in,omitempty"`
	ClarKey         string   `json:"clar_key,omitempty"`
	MatchFamilyIn   []string `json:"match_family_in,omitempty"`
	MatchFamilyNot  []string `json:"match_family_not,omitempty"`
	RejectedRule    string   `json:"rejected_rule,omitempty"`
	ClaimsFrom      []string `json:"claims_from,omitempty"`
	ClaimsMin       int      `json:"claims_min,omitempty"`
	FirstOnRequest  bool     `json:"first_on_request,omitempty"`
	NoAlternatives  bool     `json:"no_alternatives,omitempty"`
	QuoteNotes      bool     `json:"quote_notes,omitempty"`
	RenewalOnly     bool     `json:"renewal_only,omitempty"`
	NoMatch         bool     `json:"no_match,omitempty"`
	ChangesContains string   `json:"changes_contains,omitempty"`
	CompareFamilies []string `json:"compare_families,omitempty"`
}

// ReadGuideDialogs — набор диалогов (JSONL).
func ReadGuideDialogs(path string) ([]GuideDialog, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("диалоги подбора: %w", err)
	}
	defer f.Close()
	var ds []GuideDialog
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<22)
	for n := 1; sc.Scan(); n++ {
		line := strings.TrimSpace(sc.Text())
		if line == "" {
			continue
		}
		var d GuideDialog
		if err := json.Unmarshal([]byte(line), &d); err != nil {
			return nil, fmt.Errorf("%s:%d: %w", filepath.Base(path), n, err)
		}
		if d.ID == "" || len(d.Turns) == 0 {
			return nil, fmt.Errorf("%s:%d: нет id или ходов", filepath.Base(path), n)
		}
		ds = append(ds, d)
	}
	return ds, sc.Err()
}

// guideEvalKB — то, что проверкам нужно из базы знаний: статус и семейство факта, область действия оффера.
type guideEvalKB struct {
	factStatus map[string]string
	factFamily map[string]string
	scope      map[string]string // offer_id → license_scope
	pack       map[string]int    // offer_id → мест в комплекте (sale_unit package)
}

func loadGuideEvalKB(dir string) (*guideEvalKB, error) {
	kb := &guideEvalKB{factStatus: map[string]string{}, factFamily: map[string]string{}, scope: map[string]string{},
		pack: map[string]int{}}
	read := func(name string, fn func(map[string]any)) error {
		f, err := os.Open(filepath.Join(dir, name))
		if err != nil {
			return err
		}
		defer f.Close()
		sc := bufio.NewScanner(f)
		sc.Buffer(make([]byte, 1<<20), 1<<22)
		for sc.Scan() {
			var m map[string]any
			if json.Unmarshal(sc.Bytes(), &m) == nil {
				fn(m)
			}
		}
		return sc.Err()
	}
	str := func(m map[string]any, k string) string { v, _ := m[k].(string); return v }
	if err := read("product_facts.jsonl", func(m map[string]any) {
		kb.factStatus[str(m, "id")], kb.factFamily[str(m, "id")] = str(m, "status"), str(m, "family_id")
	}); err != nil {
		return nil, err
	}
	if err := read("sale_units.jsonl", func(m map[string]any) {
		if str(m, "sale_unit") == "package" {
			if n, ok := m["package_size"].(float64); ok && n > 0 {
				kb.pack[str(m, "offer_id")] = int(n)
			}
		}
	}); err != nil {
		return nil, err
	}
	if err := read("offer_scope.jsonl", func(m map[string]any) {
		if s := str(m, "license_scope"); s != "" {
			kb.scope[str(m, "offer_id")] = s
		}
	}); err != nil {
		return nil, err
	}
	return kb, nil
}

// GuideEvalRequest — что прогонять.
type GuideEvalRequest struct {
	File     string
	KBDir    string
	Runs     int
	Progress func(string)
}

// GuideEvalResult — три варианта и диалоги.
type GuideEvalResult struct {
	RunID     string                                        `json:"run_id"`
	Dialogs   []GuideDialog                                 `json:"-"`
	Variants  map[string]*out.GuideEvalMetrics              `json:"variants"`
	PerDialog map[string]map[string][]out.GuideDialogResult `json:"per_dialog"` // вариант → диалог → прогоны
	// selections — selection_id подборов прогона по диалогу и прогону (для журнала хода).
	selections map[string][]string
	// removed — снятое проверками по диалогу и прогону (из журнала, FinishGuideEval).
	turnsLLM  map[string]bool
	timing    struct{ first, checked, total, totalLLM []float64 }
	reqs      map[string]domain.GuideRequirements // диалог → требования последнего хода прогона 1
	Commit    string                              `json:"commit"`
	Catalog   string                              `json:"catalog"`
	StartedAt time.Time                           `json:"-"`
}

// guideChecks — счётчики автопроверок варианта.
type guideChecks struct {
	m    *out.GuideEvalMetrics
	errs []out.GuideEvalError
}

var guideCheckLabels = map[string]string{
	"ids_exist":       "Идентификаторы офферов и фактов существуют",
	"price_catalog":   "Цена и сумма в карточке совпадают с каталогом",
	"license_rules":   "Правила лицензий не нарушены",
	"questions":       "Вопросов в диалоге не больше допустимого",
	"steps":           "Вызовов модели и времени хода не больше предела",
	"forbidden_tools": "Нет запрещённых вызовов",
	"fact_status":     "Нет фактов ниже порога показа",
	"lead_numbers":    "Во фразе нет чисел, которых нет в карточках и условиях",
	"lead_words":      "Во фразе нет запрещённых слов",
	"claims_fact":     "У каждого утверждения о свойстве есть факт",
	"foreign_match":   "Подходящие карточки — из семейств задачи",
}

func (c *guideChecks) count(key string, bad bool) {
	ch := c.m.Checks[key]
	ch.Label = guideCheckLabels[key]
	ch.Of++
	if bad {
		ch.Bad++
	}
	c.m.Checks[key] = ch
}

func newGuideMetrics(variant string, runs, dialogs int) *out.GuideEvalMetrics {
	m := &out.GuideEvalMetrics{Variant: variant, Runs: runs, Dialogs: dialogs, Checks: map[string]out.GuideCheck{},
		Errors: []out.GuideEvalError{}, Removed: map[string]int{}}
	for k, l := range guideCheckLabels {
		m.Checks[k] = out.GuideCheck{Label: l}
	}
	return m
}

// RunGuideEval — прогоны набора: полный подбор (runs раз) и поиск без подбора (один раз — он детерминирован).
func (s *Services) RunGuideEval(ctx context.Context, req GuideEvalRequest) (*GuideEvalResult, error) {
	ds, err := ReadGuideDialogs(req.File)
	if err != nil {
		return nil, err
	}
	kb, err := loadGuideEvalKB(req.KBDir)
	if err != nil {
		return nil, fmt.Errorf("база знаний: %w", err)
	}
	st := s.catalog.Load()
	if st == nil || st.lex == nil {
		return nil, errs.NewUnavailableError(MsgTargetNoCatalog)
	}
	if req.Runs <= 0 {
		req.Runs = 3
	}
	started := time.Now()
	res := &GuideEvalResult{RunID: newEvalRunID(started), Dialogs: ds, Variants: map[string]*out.GuideEvalMetrics{},
		PerDialog: map[string]map[string][]out.GuideDialogResult{}, selections: map[string][]string{},
		turnsLLM: map[string]bool{}, reqs: map[string]domain.GuideRequirements{}, Commit: buildCommit(),
		Catalog: catalogLabel(st.version), StartedAt: started}
	full := &guideChecks{m: newGuideMetrics(GuideVariantFull, req.Runs, len(ds))}
	res.Variants[GuideVariantFull] = full.m
	res.PerDialog[GuideVariantFull] = map[string][]out.GuideDialogResult{}
	tail := res.RunID[len(res.RunID)-6:]
	for run := 1; run <= req.Runs; run++ {
		for i, d := range ds {
			r := s.guideEvalDialog(ctx, d, run, i, tail, st.lex, kb, full, res)
			res.PerDialog[GuideVariantFull][d.ID] = append(res.PerDialog[GuideVariantFull][d.ID], r)
		}
		if req.Progress != nil {
			req.Progress(fmt.Sprintf("подбор: прогон %d из %d готов", run, req.Runs))
		}
	}
	srch := &guideChecks{m: newGuideMetrics(GuideVariantSearch, 1, len(ds))}
	res.Variants[GuideVariantSearch] = srch.m
	res.PerDialog[GuideVariantSearch] = map[string][]out.GuideDialogResult{}
	for _, d := range ds {
		r := s.guideEvalSearch(ctx, d, st.lex, kb, srch, res.reqs[d.ID])
		res.PerDialog[GuideVariantSearch][d.ID] = []out.GuideDialogResult{r}
	}
	for _, v := range []*guideChecks{full, srch} {
		finishGuideTask(v.m, res.PerDialog[v.m.Variant], ds)
		v.m.Errors = firstErrors(v.errs, 6)
	}
	lat := func(v []float64) out.EvalLatency { return evalLatencyOf(v) }
	full.m.MsFirst, full.m.MsChecked, full.m.MsTotal, full.m.MsTotalLLM =
		lat(res.timing.first), lat(res.timing.checked), lat(res.timing.total), lat(res.timing.totalLLM)
	zap.L().Info("оценка подбора: прогоны завершены", zap.String("run_id", res.RunID), zap.Int("dialogs", len(ds)),
		zap.Int("runs", req.Runs))
	return res, nil
}

// firstErrors — первые n ошибок, по одной на диалог.
func firstErrors(es []out.GuideEvalError, n int) []out.GuideEvalError {
	res := []out.GuideEvalError{}
	seen := map[string]bool{}
	for _, e := range es {
		if seen[e.Dialog] || len(res) >= n {
			continue
		}
		seen[e.Dialog] = true
		res = append(res, e)
	}
	return res
}

// finishGuideTask — задачи: диалог решён во всех прогонах варианта.
func finishGuideTask(m *out.GuideEvalMetrics, per map[string][]out.GuideDialogResult, ds []GuideDialog) {
	m.TaskAll, m.TaskPasses, m.TaskTotal, m.VendorAll, m.VendorOf, m.NegativeAll, m.NegativeOf = 0, 0, 0, 0, 0, 0, 0
	for _, d := range ds {
		runs := per[d.ID]
		all := len(runs) > 0
		for _, r := range runs {
			m.TaskTotal++
			if r.Pass {
				m.TaskPasses++
			} else {
				all = false
			}
		}
		if d.Type == "negative" {
			m.NegativeOf++
			if all {
				m.NegativeAll++
			}
		} else {
			m.VendorOf++
			if all {
				m.VendorAll++
			}
		}
		if all {
			m.TaskAll++
		}
	}
	m.Gate = m.FabricatedIDs == 0 && m.FabricatedPrices == 0 && m.LicenseViolation == 0 && m.ClaimsNoFact == 0
}

// guideEvalDialog — один диалог полного подбора в одном прогоне.
func (s *Services) guideEvalDialog(ctx context.Context, d GuideDialog, run, idx int, tail string, lx *searchLexicon,
	kb *guideEvalKB, c *guideChecks, res *GuideEvalResult) out.GuideDialogResult {
	r := out.GuideDialogResult{Run: run, Pass: true, Types: []string{}}
	session := fmt.Sprintf("gev%s-r%d-%02d", tail, run, idx)
	var state *domain.GuideState
	query, questions := "", 0
	fail := func(turn int, why string) {
		r.Pass = false
		r.Fails = append(r.Fails, fmt.Sprintf("ход %d: %s", turn, why))
		c.errs = append(c.errs, out.GuideEvalError{Dialog: d.ID, Run: run, Turn: turn, Reason: why})
	}
	for ti, t := range d.Turns {
		if t.Query != "" {
			query = t.Query
		}
		req := in.GuideRequest{SessionID: session, Query: query, Message: t.Message, Chip: t.Chip, Condition: t.Condition, State: state}
		if t.Query != "" {
			req.State = nil // новый подбор
		}
		reqID := fmt.Sprintf("gv%sr%d-%02d-%d", tail, run, idx, ti+1)
		resp, err := s.Guide(ctx, reqID, req)
		r.Turns++
		if err != nil {
			fail(ti+1, "ошибка хода: "+err.Error())
			continue
		}
		state = &resp.State
		if !slices.Contains(res.selections[d.ID], resp.State.SelectionID) {
			res.selections[d.ID] = append(res.selections[d.ID], resp.State.SelectionID)
		}
		r.Selection = resp.State.SelectionID
		r.Types = append(r.Types, resp.Type)
		r.LLM += resp.Meta.LLMCalls
		r.Fallbacks = append(r.Fallbacks, resp.Meta.Fallback)
		r.CostRub += resp.Meta.CostRub
		if resp.Type == out.GuideClarification {
			questions++
		}
		res.timing.first = append(res.timing.first, resp.Meta.MsFirst)
		res.timing.checked = append(res.timing.checked, resp.Meta.MsChecked)
		res.timing.total = append(res.timing.total, resp.Meta.MsTotal)
		c.m.Turns++
		if resp.Meta.LLMCalls > 0 {
			c.m.LLMTurns++
			res.timing.totalLLM = append(res.timing.totalLLM, resp.Meta.MsTotal)
		}
		for _, why := range guideResponseChecks(d, resp, lx, kb, c) {
			fail(ti+1, why)
		}
		for _, why := range guideExpect(t.Expect, resp, lx) {
			fail(ti+1, why)
		}
		if run == 1 {
			res.reqs[d.ID] = resp.State.Requirements
		}
	}
	maxQ := d.Limits.MaxQuestions
	if maxQ == 0 {
		maxQ = 2
	}
	c.count("questions", questions > maxQ)
	if questions > maxQ {
		fail(len(d.Turns), fmt.Sprintf("вопросов %d > %d", questions, maxQ))
	}
	r.CostRub = math.Round(r.CostRub*1e4) / 1e4
	return r
}

// guideResponseChecks — автопроверки ответа хода; причины нарушений (для отчёта — пустой срез, если всё чисто).
func guideResponseChecks(d GuideDialog, resp *out.GuideResponse, lx *searchLexicon, kb *guideEvalKB, c *guideChecks) []string {
	var bad []string
	note := func(key string, ok bool, why string) {
		c.count(key, !ok)
		if !ok {
			bad = append(bad, why)
		}
	}
	allowed := d.Allowed.FamilyIDs
	for _, o := range resp.Offers {
		doc, ok := lx.docs[o.OfferID]
		exists := ok && doc.FamilyID == o.FamilyID
		note("ids_exist", exists, "оффер "+o.OfferID+" не найден в каталоге")
		if !exists {
			c.m.FabricatedIDs++
			continue
		}
		pok := guidePriceOK(o, doc)
		note("price_catalog", pok, fmt.Sprintf("цена или сумма оффера %s не совпадает с каталогом", o.OfferID))
		if !pok {
			c.m.FabricatedPrices++
		}
		if o.Role == out.OfferMatch {
			if v := guideLicenseViolation(o, doc, resp.State.Requirements, kb); v != "" {
				note("license_rules", false, "оффер "+o.OfferID+": "+v)
				c.m.LicenseViolation++
			} else {
				note("license_rules", true, "")
			}
			foreign := (len(allowed) > 0 && !slices.Contains(allowed, o.FamilyID)) || slices.Contains(d.Forbidden.FamiliesMatch, o.FamilyID)
			note("foreign_match", !foreign, "подходящий оффер "+o.OfferID+" из семейства "+o.FamilyID+" вне задачи")
		}
	}
	factCheck := func(id string) {
		st, ok := kb.factStatus[id]
		note("ids_exist", ok, "факт "+id+" не найден в базе знаний")
		if !ok {
			c.m.FabricatedIDs++
			return
		}
		note("fact_status", guideFactOK[st], "факт "+id+" ниже порога показа ("+st+")")
	}
	for _, cl := range resp.Claims {
		c.m.Claims++
		has := cl.FactID != ""
		note("claims_fact", has, "утверждение без факта: "+cl.Text)
		if !has {
			c.m.ClaimsNoFact++
			continue
		}
		factCheck(cl.FactID)
	}
	numbers := guideNumbersOf(resp)
	if cmp := resp.Comparison; cmp != nil {
		for _, row := range cmp.Rows {
			for ci, cell := range row.Cells {
				if row.Kind == "feature" && (cell.Kind == out.CellYes || cell.Kind == out.CellNo) {
					c.m.Claims++
					note("claims_fact", cell.FactID != "", "ячейка «"+row.Label+"» без факта")
					if cell.FactID == "" {
						c.m.ClaimsNoFact++
					}
				}
				if cell.FactID != "" {
					factCheck(cell.FactID)
				}
				if row.Kind == "commercial" && ci < len(cmp.Columns) {
					if v, ok := cell.Value.(float64); ok {
						pok := guideCellPriceOK(v, cmp.Columns[ci].FamilyID, resp, lx)
						note("price_catalog", pok, fmt.Sprintf("число %.2f в строке «%s» не из каталога", v, row.Label))
						if !pok {
							c.m.FabricatedPrices++
						}
					}
				}
			}
		}
	}
	tools := 0
	for _, st := range resp.Trace {
		if st.Tool == "" {
			continue
		}
		tools++
		note("forbidden_tools", !slices.Contains(d.Forbidden.Tools, st.Tool), "запрещённый вызов "+st.Tool)
	}
	if tools == 0 {
		c.count("forbidden_tools", false)
	}
	maxLLM, maxMs := d.Limits.MaxLLMCallsPerTurn, d.Limits.MaxTurnMs
	if maxMs == 0 {
		maxMs = 12000
	}
	note("steps", resp.Meta.LLMCalls <= maxLLM && resp.Meta.MsTotal <= float64(maxMs),
		fmt.Sprintf("вызовов модели %d (предел %d), ход %.0f мс (предел %d)", resp.Meta.LLMCalls, maxLLM, resp.Meta.MsTotal, maxMs))
	if lead := strings.TrimSpace(resp.Lead); lead != "" {
		var extra []string
		for _, n := range guideNumbersIn(lead) {
			if !numbers[n] {
				extra = append(extra, n)
			}
		}
		note("lead_numbers", len(extra) == 0, "во фразе числа не из карточек и условий: "+strings.Join(extra, ", "))
		low := strings.ToLower(lead)
		var words []string
		for _, w := range d.Forbidden.Words {
			if strings.Contains(low, w) {
				words = append(words, w)
			}
		}
		note("lead_words", len(words) == 0, "во фразе запрещённые слова: "+strings.Join(words, ", "))
	}
	return bad
}

// guidePriceOK — цена карточки = цена каталога; сумма = цена × целое число единиц, не больше количества.
func guidePriceOK(o out.GuideOffer, doc indexDoc) bool {
	priced := doc.PriceRub != nil && *doc.PriceRub > 0
	if o.PriceState == out.PricePriced != priced {
		return false
	}
	if !priced {
		return o.PricePerUnit == nil && o.Total == nil
	}
	if o.PricePerUnit == nil || math.Abs(*o.PricePerUnit-*doc.PriceRub) > 0.005 {
		return false
	}
	if o.Total == nil {
		return true
	}
	k := *o.Total / *doc.PriceRub
	return math.Abs(k-math.Round(k)) < 1e-6 && math.Round(k) >= 1 && math.Round(k) <= float64(max(o.Quantity, 1))
}

// guideCellPriceOK — число строки «цена/итого» таблицы: цена или сумма карточки ответа либо цена оффера семейства
// колонки, умноженная или делённая на целое (единицы, пакет).
func guideCellPriceOK(v float64, family string, resp *out.GuideResponse, lx *searchLexicon) bool {
	near := func(a, b float64) bool { return math.Abs(a-b) <= 0.01+1e-9*math.Abs(b) }
	for _, o := range resp.Offers {
		if o.PricePerUnit != nil && near(v, *o.PricePerUnit) || o.Total != nil && near(v, *o.Total) {
			return true
		}
	}
	for _, id := range lx.families[family] {
		p := lx.docs[id].PriceRub
		if p == nil || *p <= 0 {
			continue
		}
		for k := 1.0; k <= 1000; k++ {
			if near(v, *p*k) || near(v, guideRound2(*p/k)) || near(v, *p/k) {
				return true
			}
		}
	}
	return false
}

// guideLicenseViolation — независимая проверка правил лицензий у подходящей карточки (license_rules.json, verified):
// рабочее место CSP не в серверной ОС; TLS-сервер — только лицензия tls_server; Linux-рабочие станции Кибер Бэкап —
// не Стандартная; новая лицензия — не продление, обновление и миграция; количество — в ступени; тип организации.
func guideLicenseViolation(o out.GuideOffer, doc indexDoc, r domain.GuideRequirements, kb *guideEvalKB) string {
	hasObj := func(k string) bool {
		return slices.ContainsFunc(r.Objects, func(x domain.GuideObject) bool { return x.Kind == k })
	}
	count := func() int {
		n := 0
		for _, x := range r.Objects {
			n = max(n, x.Count)
		}
		return n
	}
	scope := kb.scope[o.OfferID]
	switch {
	case doc.FamilyID == "22268" && hasObj("server") && !hasObj("workstation") && scope == "workstation":
		return "cryptopro_workstation_not_server: лицензия рабочего места для сервера"
	case r.Use == "tls_server" && (doc.FamilyID == "22268" || doc.FamilyID == "385280") && scope != "" && scope != "tls_server":
		return "cryptopro_tls_separate: для TLS-сервера не TLS-лицензия"
	case doc.FamilyID == "173614" && strings.EqualFold(r.Platform, "linux") && hasObj("workstation"):
		return "cyberbackup_linux_workstation_advanced: Linux-рабочие станции — только Расширенная"
	case r.Operation == "new" && slices.Contains([]string{domain.LicenseKindRenewal, domain.LicenseKindUpgrade, domain.LicenseKindMigration}, doc.LicenseKind):
		return "new_excludes_renewal: новая лицензия, а карточка — " + doc.LicenseKind
	case r.Operation == "renewal" && doc.LicenseKind != "" && doc.LicenseKind != domain.LicenseKindRenewal:
		return "продление, а карточка — " + doc.LicenseKind
	case r.OrgType != "" && doc.LicenseOrgType != "" && doc.LicenseOrgType != r.OrgType:
		return "тип организации " + doc.LicenseOrgType + " ≠ " + r.OrgType
	}
	if size := kb.pack[o.OfferID]; size > 0 && o.Total != nil && doc.PriceRub != nil && *doc.PriceRub > 0 {
		// Комплект на N мест: число комплектов = ⌈мест / N⌉.
		if n := count(); n > 0 && math.Round(*o.Total / *doc.PriceRub) != math.Ceil(float64(n)/float64(size)) {
			return fmt.Sprintf("quantity_in_tier: комплектов %.0f, а нужно %.0f на %d мест", math.Round(*o.Total / *doc.PriceRub),
				math.Ceil(float64(n)/float64(size)), n)
		}
		return ""
	}
	if n := count(); n > 0 && o.Total != nil && doc.LicenseTierMin != nil {
		if n < *doc.LicenseTierMin || doc.LicenseTierMax != nil && n > *doc.LicenseTierMax {
			return fmt.Sprintf("quantity_in_tier: %d вне ступени", n)
		}
	}
	return ""
}

var guideNumRe = regexp.MustCompile(`\d[\d\s\x{00a0}\x{202f}]*(?:[.,]\d+)?`)

// guideNumbersIn — числа текста в каноническом виде: без пробелов в разрядах, запятая → точка, без хвостовых нулей.
func guideNumbersIn(s string) []string {
	var res []string
	for _, m := range guideNumRe.FindAllString(s, -1) {
		m = strings.Map(func(r rune) rune {
			if r == ' ' || r == ' ' || r == ' ' {
				return -1
			}
			return r
		}, m)
		m = strings.ReplaceAll(m, ",", ".")
		if f, err := strconv.ParseFloat(m, 64); err == nil {
			res = append(res, strconv.FormatFloat(f, 'f', -1, 64))
		}
	}
	return res
}

// guideNumbersOf — числа, которые фраза может назвать: карточки, условия, таблица, утверждения, изменения.
func guideNumbersOf(resp *out.GuideResponse) map[string]bool {
	set := map[string]bool{}
	add := func(s string) {
		for _, n := range guideNumbersIn(s) {
			set[n] = true
		}
	}
	addF := func(f *float64) {
		if f != nil {
			set[strconv.FormatFloat(math.Round(*f*100)/100, 'f', -1, 64)] = true
			set[strconv.FormatFloat(math.Round(*f), 'f', -1, 64)] = true
		}
	}
	for _, o := range resp.Offers {
		add(o.Title + " " + o.FamilyTitle + " " + o.Tier + " " + o.Unit + " " + o.Term + " " + o.Note + " " + o.SKU)
		set[strconv.Itoa(o.Quantity)] = true
		addF(o.PricePerUnit)
		addF(o.Total)
	}
	for _, c := range resp.Conditions {
		add(c.Label + " " + c.Value)
	}
	for _, c := range resp.Claims {
		add(c.Text)
	}
	for _, c := range resp.Changes {
		add(c.Condition + " " + c.Effect)
	}
	if cmp := resp.Comparison; cmp != nil {
		for _, col := range cmp.Columns {
			add(col.Title)
		}
		for _, row := range cmp.Rows {
			add(row.Label + " " + row.Note)
			for _, c := range row.Cells {
				add(fmt.Sprint(c.Value))
			}
		}
	}
	for _, x := range resp.State.Requirements.Objects {
		set[strconv.Itoa(x.Count)] = true
	}
	if m := resp.State.Requirements.TermMonths; m > 0 {
		set[strconv.Itoa(m)] = true
		if m%12 == 0 {
			set[strconv.Itoa(m/12)] = true
		}
	}
	return set
}

// guideExpect — ожидания хода; причины несовпадения.
func guideExpect(e GuideTurnExpectation, resp *out.GuideResponse, lx *searchLexicon) []string {
	var bad []string
	match := []out.GuideOffer{}
	for _, o := range resp.Offers {
		if o.Role == out.OfferMatch {
			match = append(match, o)
		}
	}
	if len(e.TypeIn) > 0 && !slices.Contains(e.TypeIn, resp.Type) {
		bad = append(bad, "тип ответа "+resp.Type+", ждали "+strings.Join(e.TypeIn, "|"))
	}
	if e.ClarKey != "" && (resp.Clarification == nil || resp.Clarification.Key != e.ClarKey) {
		bad = append(bad, "нет вопроса "+e.ClarKey)
	}
	if len(e.MatchFamilyIn) > 0 {
		if len(match) == 0 {
			bad = append(bad, "нет подходящих карточек")
		} else if !slices.Contains(e.MatchFamilyIn, match[0].FamilyID) {
			bad = append(bad, "первая подходящая — семейство "+match[0].FamilyID+", ждали "+strings.Join(e.MatchFamilyIn, "|"))
		}
	}
	for _, o := range match {
		if slices.Contains(e.MatchFamilyNot, o.FamilyID) {
			bad = append(bad, "подходящим осталось семейство "+o.FamilyID)
			break
		}
	}
	if e.RejectedRule != "" && !slices.ContainsFunc(resp.Offers, func(o out.GuideOffer) bool {
		return o.Role == out.OfferRejected && o.RuleID == e.RejectedRule
	}) {
		bad = append(bad, "нет блока «Не подходит» по правилу "+e.RejectedRule)
	}
	for _, c := range resp.Claims {
		if len(e.ClaimsFrom) > 0 && !slices.Contains(e.ClaimsFrom, c.FactID) {
			bad = append(bad, "утверждение не из ожидаемых фактов: "+c.FactID)
		}
	}
	if len(resp.Claims) < e.ClaimsMin {
		bad = append(bad, fmt.Sprintf("утверждений %d < %d", len(resp.Claims), e.ClaimsMin))
	}
	if e.FirstOnRequest && (len(resp.Offers) == 0 || resp.Offers[0].PriceState != out.PriceOnRequest) {
		bad = append(bad, "первая карточка не «по запросу»")
	}
	if e.NoAlternatives && slices.ContainsFunc(resp.Offers, func(o out.GuideOffer) bool { return o.Role == out.OfferAlternative }) {
		bad = append(bad, "остались альтернативы после «нужен именно»")
	}
	if e.QuoteNotes && len(resp.State.QuoteNotes) == 0 {
		bad = append(bad, "условия покупателя не попали в запрос КП")
	}
	if e.RenewalOnly {
		for _, o := range match {
			if lx.docs[o.OfferID].LicenseKind != domain.LicenseKindRenewal {
				bad = append(bad, "среди подходящих не продление: "+o.OfferID)
				break
			}
		}
	}
	if e.NoMatch && len(match) > 0 {
		bad = append(bad, fmt.Sprintf("подходящих карточек %d, ждали 0", len(match)))
	}
	if e.ChangesContains != "" && !slices.ContainsFunc(resp.Changes, func(c out.GuideChange) bool {
		return strings.Contains(c.Condition+" "+c.Effect, e.ChangesContains)
	}) {
		bad = append(bad, "в изменениях нет «"+e.ChangesContains+"»")
	}
	if len(e.CompareFamilies) > 0 {
		var cols []string
		if resp.Comparison != nil {
			for _, c := range resp.Comparison.Columns {
				cols = append(cols, c.FamilyID)
			}
		}
		for _, f := range e.CompareFamilies {
			if !slices.Contains(cols, f) {
				bad = append(bad, "в сравнении нет семейства "+f)
				break
			}
		}
	}
	return bad
}

// guideEvalSearch — вариант «поиск без подбора»: каждая реплика диалога — отдельный поиск витрины; чипов и правок
// условий у поиска нет. Проверяется то, что поиск умеет: первая карточка, продления, «по запросу», отсутствие точных
// карточек; вопрос, сравнение, утверждения, изменения и запрос КП — «нет в поиске».
func (s *Services) guideEvalSearch(ctx context.Context, d GuideDialog, lx *searchLexicon, kb *guideEvalKB, c *guideChecks,
	reqs domain.GuideRequirements) out.GuideDialogResult {
	r := out.GuideDialogResult{Run: 1, Pass: true, Types: []string{}}
	fail := func(turn int, why string) {
		r.Pass = false
		r.Fails = append(r.Fails, fmt.Sprintf("ход %d: %s", turn, why))
		c.errs = append(c.errs, out.GuideEvalError{Dialog: d.ID, Run: 1, Turn: turn, Reason: why})
	}
	for ti, t := range d.Turns {
		text := t.Query
		if text == "" {
			text = t.Message
		}
		e := t.Expect
		unsupported := e.ClarKey != "" || e.RejectedRule != "" || len(e.ClaimsFrom) > 0 || e.ClaimsMin > 0 ||
			e.NoAlternatives || e.QuoteNotes || e.ChangesContains != "" || len(e.CompareFamilies) > 0 || len(e.TypeIn) > 0
		if text == "" {
			if !reflectEmpty(e) {
				fail(ti+1, "чипа и правки условия в поиске нет")
			}
			continue
		}
		resp, err := s.SearchWith(ctx, fmt.Sprintf("gvs-%s-%d", d.ID, ti+1), in.SearchRequest{Query: text, Config: domain.ConfigLive,
			Limit: targetSearchLimit}, SearchOptions{NoLog: true, Storefront: true})
		r.Turns++
		if err != nil {
			fail(ti+1, "ошибка поиска: "+err.Error())
			continue
		}
		c.m.Turns++
		if unsupported {
			fail(ti+1, "нет в поиске: вопрос, сравнение, факты, «не подходит» или запрос КП")
		}
		var first *out.SearchItem
		if len(resp.Items) > 0 {
			first = &resp.Items[0]
		}
		for i, it := range resp.Items {
			if i >= 4 {
				break
			}
			doc, ok := lx.docs[it.ProductID]
			c.count("ids_exist", !ok)
			if !ok {
				c.m.FabricatedIDs++
				continue
			}
			pok := doc.PriceRub == nil && it.PriceRub == nil || doc.PriceRub != nil && it.PriceRub != nil && math.Abs(*doc.PriceRub-*it.PriceRub) < 0.005
			c.count("price_catalog", !pok)
			if !pok {
				c.m.FabricatedPrices++
			}
		}
		if first != nil && reqs.Operation != "" {
			doc := lx.docs[first.ProductID]
			o := out.GuideOffer{OfferID: first.ProductID, FamilyID: first.FamilyID}
			v := guideLicenseViolation(o, doc, reqs, kb)
			c.count("license_rules", v != "")
			if v != "" {
				c.m.LicenseViolation++
				fail(ti+1, "первая карточка поиска: "+v)
			}
		}
		if len(e.MatchFamilyIn) > 0 && (first == nil || !slices.Contains(e.MatchFamilyIn, first.FamilyID)) {
			fam := ""
			if first != nil {
				fam = first.FamilyID
			}
			fail(ti+1, "первая карточка поиска — семейство "+fam)
		}
		if first != nil && slices.Contains(e.MatchFamilyNot, first.FamilyID) {
			fail(ti+1, "первая карточка поиска — исключаемое семейство "+first.FamilyID)
		}
		if e.FirstOnRequest && (first == nil || first.PriceState != out.PriceOnRequest) {
			fail(ti+1, "первая карточка поиска не «по запросу»")
		}
		if e.RenewalOnly {
			for _, it := range resp.Items {
				if it.MatchType == out.MatchExact && lx.docs[it.ProductID].LicenseKind != domain.LicenseKindRenewal {
					fail(ti+1, "среди точных карточек поиска не продление")
					break
				}
			}
		}
		if e.NoMatch && slices.ContainsFunc(resp.Items, func(it out.SearchItem) bool { return it.MatchType == out.MatchExact }) {
			fail(ti+1, "поиск выдал точные карточки на отрицательный запрос")
		}
	}
	return r
}

func reflectEmpty(e GuideTurnExpectation) bool {
	raw, _ := json.Marshal(e)
	return string(raw) == "{}"
}

// FinishGuideEval — расход по журналу хода и llm_usage, эмуляция «без проверок», запись в evaluations.
func (s *Services) FinishGuideEval(ctx context.Context, res *GuideEvalResult, persist bool) error {
	full := res.Variants[GuideVariantFull]
	nock := &out.GuideEvalMetrics{}
	if s.repos.Enabled() {
		var ids []string
		for _, v := range res.selections {
			ids = append(ids, v...)
		}
		sort.Strings(ids)
		rows, err := s.repos.GuideTurnsOf(ctx, ids)
		if err != nil {
			return fmt.Errorf("журнал хода: %w", err)
		}
		removedBy := map[string][]string{} // selection_id → снятое
		for _, r := range rows {
			full.JournalTurns++
			full.LLMCalls += r.LLMCalls
			full.InputTokens += r.InputTokens
			full.OutputTokens += r.OutputTokens
			full.TurnsCostRub += r.CostRub
			var rem []string
			_ = json.Unmarshal(r.Removed, &rem)
			removedBy[r.SelectionID] = append(removedBy[r.SelectionID], rem...)
		}
		full.TurnsCostRub = math.Round(full.TurnsCostRub*1e4) / 1e4
		tail := res.RunID[len(res.RunID)-6:]
		for run := 1; run <= full.Runs; run++ {
			t, err := s.repos.LLMUsageByPrefix(ctx, fmt.Sprintf("gv%sr%d-", tail, run))
			if err != nil {
				return err
			}
			full.UsageCalls += t.Calls
			full.UsageCostRub += t.CostRub
		}
		full.UsageCostRub = math.Round(full.UsageCostRub*1e4) / 1e4
		// «Без проверок»: к показанному добавляется снятое проверками того же хода.
		*nock = *full
		nock.Variant, nock.Checks, nock.Errors, nock.Removed = GuideVariantNoCheck, map[string]out.GuideCheck{}, []out.GuideEvalError{}, map[string]int{}
		for k, v := range full.Checks {
			nock.Checks[k] = v
		}
		per := map[string][]out.GuideDialogResult{}
		for _, d := range res.Dialogs {
			for _, r := range res.PerDialog[GuideVariantFull][d.ID] {
				rr := r
				rr.Fails = append([]string(nil), r.Fails...)
				for _, item := range removedBy[r.Selection] {
					cls, why := guideRemovedClass(item)
					nock.Removed[cls]++
					full.Removed[cls]++
					switch cls {
					case "claim":
						nock.ClaimsNoFact++
					case "id", "offer":
						nock.FabricatedIDs++
					case "license":
						nock.LicenseViolation++
					}
					rr.Pass = false
					rr.Fails = append(rr.Fails, "без проверок показано бы: "+why)
					nock.Errors = append(nock.Errors, out.GuideEvalError{Dialog: d.ID, Run: r.Run, Reason: why})
				}
				per[d.ID] = append(per[d.ID], rr)
			}
		}
		res.PerDialog[GuideVariantNoCheck] = per
		finishGuideTask(nock, per, res.Dialogs)
		nock.Errors = firstErrors(nock.Errors, 6)
		nock.Notes = []string{"Эмуляция: тот же ход подбора, к показанному добавлено то, что сняли проверки до показа " +
			"(guide_turns.removed). Карточки, цены и артикулы и в этом варианте строит код: выдуманной цены модель " +
			"дать не может, поэтому «цены» здесь не отличаются от полного подбора."}
		res.Variants[GuideVariantNoCheck] = nock
	}
	dialogs := len(res.Dialogs) * full.Runs
	if dialogs > 0 {
		cost := full.UsageCostRub
		if cost == 0 {
			cost = full.TurnsCostRub
		}
		full.DialogCostRub = math.Round(cost/float64(dialogs)*1e4) / 1e4
		withLLM := 0
		for _, d := range res.Dialogs {
			for _, r := range res.PerDialog[GuideVariantFull][d.ID] {
				if r.LLM > 0 {
					withLLM++
				}
			}
		}
		full.DialogsWithLLM = withLLM
		if withLLM > 0 {
			full.DialogLLMRub = math.Round(cost/float64(withLLM)*1e4) / 1e4
		}
	}
	full.Gate = full.FabricatedIDs == 0 && full.FabricatedPrices == 0 && full.LicenseViolation == 0 && full.ClaimsNoFact == 0
	if !persist || !s.repos.Enabled() {
		return nil
	}
	var rows []domain.Evaluation
	for _, v := range []string{GuideVariantSearch, GuideVariantNoCheck, GuideVariantFull} {
		m := res.Variants[v]
		if m == nil {
			continue
		}
		em := out.EvalMetrics{Queries: len(res.Dialogs), Guide: m, Latency: m.MsTotal}
		var pq []out.EvalQueryResult
		for _, d := range res.Dialogs {
			pq = append(pq, out.EvalQueryResult{ID: d.ID, Text: d.Scenario, Type: d.Type, Top: []string{}, Grades: []int{},
				Guide: res.PerDialog[v][d.ID]})
		}
		row := domain.Evaluation{RunID: res.RunID, Set: guideEvalSet, SetHash: "", Queries: len(res.Dialogs), Config: v,
			CatalogVersion: s.catalogVersion(), Commit: res.Commit}
		row.Metrics, _ = domain.ToJSONB(em)
		row.PerQuery, _ = domain.ToJSONB(pq)
		row.ByType, _ = domain.ToJSONB(map[string]any{})
		row.Regressions, _ = domain.ToJSONB([]any{})
		row.Improvements, _ = domain.ToJSONB([]any{})
		rows = append(rows, row)
	}
	return s.repos.SaveEvaluations(ctx, rows)
}

func (s *Services) catalogVersion() string {
	if st := s.catalog.Load(); st != nil {
		return st.version
	}
	return ""
}

// guideRemovedClass — класс снятого проверкой (guide_answer.go checkAnswer) и подпись.
func guideRemovedClass(item string) (string, string) {
	switch {
	case strings.HasPrefix(item, "claim:"), strings.HasPrefix(item, "cell:"):
		return "claim", "утверждение без годного факта (" + item + ")"
	case strings.HasPrefix(item, "id:"):
		return "id", "выдуманный идентификатор факта (" + item + ")"
	case strings.HasPrefix(item, "offer:") && strings.Count(item, ":") >= 2:
		return "license", "карточка с нарушением правила (" + item + ")"
	case strings.HasPrefix(item, "offer:"):
		return "offer", "карточка не из каталога или не из семейств хода (" + item + ")"
	case strings.HasPrefix(item, "cheapest:"):
		return "cheapest", "«самый дешёвый» не пересчитан среди подходящих (" + item + ")"
	case strings.HasPrefix(item, "lead"):
		return "lead", "фраза модели не прошла проверку (" + item + ")"
	}
	return "other", item
}

// LatestGuideEval — последняя оценка подбора: вариант → метрики (для сводки S6 и отчёта).
func (s *Services) LatestGuideEval(ctx context.Context) (map[string]*out.GuideEvalMetrics, string, error) {
	res := map[string]*out.GuideEvalMetrics{}
	if !s.repos.Enabled() {
		return res, "", nil
	}
	rows, err := s.repos.LatestEvaluations(ctx, guideEvalSet)
	if err != nil {
		return nil, "", err
	}
	runID := ""
	for _, r := range rows {
		var m out.EvalMetrics
		if json.Unmarshal(r.Metrics, &m) == nil && m.Guide != nil {
			res[r.Config] = m.Guide
			if r.Config == GuideVariantFull {
				runID = r.RunID
			}
		}
	}
	return res, runID, nil
}
