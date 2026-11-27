package services

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"sync"

	"go.uber.org/zap"
)

// База знаний подбора (контур подбора, KNOWLEDGE.md): факты о свойствах семейств, правила лицензирования,
// разметка офферов и линейки. Данные кладёт трек «Знания» в data/softline/kb/ (*.json — массив или объект с
// массивами, *.jsonl — объект на строку). Тип записи определяется по полям, имена файлов не важны:
//   - факт — есть axis и family_id;
//   - правило — есть message и одно из when / forbid / require;
//   - разметка оффера — есть offer_id и одно из scope / variant / object / unit;
//   - линейка — есть family_id и line.
// В ответ покупателю идут только записи со статусом verified. Правило со статусом draft применяется как фильтр
// (осторожнее — значит безопаснее), но его текст покупатель не видит, а результат помечается предварительным.

// Статусы записей базы знаний.
const (
	kbVerified = "verified"
	kbDraft    = "draft"
)

// guideFact — факт о свойстве семейства (схема KNOWLEDGE.md).
type guideFact struct {
	ID            string            `json:"id"`
	FamilyID      string            `json:"family_id"`
	Variant       string            `json:"variant"`
	Axis          string            `json:"axis"`
	AxisLabel     string            `json:"axis_label"`
	Label         string            `json:"label"`
	Value         json.RawMessage   `json:"value"`
	Applicability map[string]string `json:"applicability"`
	SourceURL     string            `json:"source_url"`
	SourceTitle   string            `json:"source_title"`
	SourceSection string            `json:"source_section"`
	Excerpt       string            `json:"excerpt"`
	Status        string            `json:"status"`
	show          bool              // проходит порог показа
}

// guideRule — правило лицензирования (data/license-rules.json).
type guideRule struct {
	ID        string         `json:"id"`
	Vendor    string         `json:"vendor"`
	FamilyIDs []string       `json:"family_ids"`
	When      map[string]any `json:"when"`
	Forbid    map[string]any `json:"forbid"`
	Require   map[string]any `json:"require"`
	Message   string         `json:"message"`
	Note      string         `json:"note"`
	Source    string         `json:"source"`
	Status    string         `json:"status"`
	show      bool           // текст правила можно показать покупателю
}

// guideOfferMark — разметка оффера (offer_scope.jsonl, astra_variants.jsonl, sale_units.jsonl): область действия
// (workstation | server | tls_server | distribution | unknown), платформа (any | astra_alt_ks3), вариант внутри
// семейства, объект лицензии, единица продажи (per_unit_in_tier — за узел на ступени; package — за пакет на
// package_size мест). Записи одного оффера из разных файлов сливаются.
type guideOfferMark struct {
	OfferID      string `json:"offer_id"`
	FamilyID     string `json:"family_id"`
	Scope        string `json:"scope"`
	LicenseScope string `json:"license_scope"`
	Platform     string `json:"platform"`
	Variant      string `json:"variant"`
	Object       string `json:"object"`
	SaleUnit     string `json:"sale_unit"`
	PackageSize  *int   `json:"package_size"`
	LicenseType  string `json:"license_type"`
	SupportLevel string `json:"support_level"`
	OrgSize      string `json:"org_size"`
	Arch         string `json:"arch"`
	Status       string `json:"status"`
}

// merge — поля другой записи того же оффера, где у этой пусто.
func (m guideOfferMark) merge(o guideOfferMark) guideOfferMark {
	pick := func(a, b string) string {
		if a != "" {
			return a
		}
		return b
	}
	m.Scope, m.LicenseScope, m.Platform = pick(m.Scope, o.Scope), pick(m.LicenseScope, o.LicenseScope), pick(m.Platform, o.Platform)
	m.Variant, m.Object, m.SaleUnit = pick(m.Variant, o.Variant), pick(m.Object, o.Object), pick(m.SaleUnit, o.SaleUnit)
	m.LicenseType = pick(m.LicenseType, o.LicenseType)
	m.SupportLevel, m.OrgSize, m.Arch = pick(m.SupportLevel, o.SupportLevel), pick(m.OrgSize, o.OrgSize), pick(m.Arch, o.Arch)
	if m.PackageSize == nil {
		m.PackageSize = o.PackageSize
	}
	return m
}

// guideLine — семейство в линейке (редакции одного продукта): kes-business → Стандартный, Расширенный, …
type guideLine struct {
	FamilyID   string `json:"family_id"`
	Line       string `json:"line"`
	Rank       int    `json:"rank"`
	Edition    string `json:"edition"`
	VendorName string `json:"vendor_name"`
}

// guideKB — база знаний в памяти; неизменяема после сборки.
type guideKB struct {
	version  string
	facts    map[string]*guideFact   // id → факт
	byFamily map[string][]*guideFact // family_id → факты
	rules    []guideRule
	marks    map[string]guideOfferMark // offer_id → разметка
	lines    map[string]guideLine      // family_id → линейка
	// subclasses — направление ИБ → семейства (security-subclasses.json: {id, title, families: [[id, название], …]}).
	subclasses map[string][]string
	dirOrder   []string          // направления в порядке файла
	dirTitle   map[string]string // направление → название
	// offFeed — факты о тарифах, которых нет в фиде (family_id null, line): только для фразы и запроса КП.
	offFeed map[string][]*guideFact
	// offFeedNames — линейка → названия тарифов вне фида (vendor_families.json, family_id null).
	offFeedNames map[string][]string
	minStatus    string
}

// factValue — значение факта: yes | no | unknown | depends или текст (число с единицей, список).
func (f *guideFact) factValue() (kind string, text string) {
	var s string
	if err := json.Unmarshal(f.Value, &s); err == nil {
		switch strings.ToLower(strings.TrimSpace(s)) {
		case "yes", "да":
			return "yes", ""
		case "no", "нет":
			return "no", ""
		case "", "unknown":
			return "unknown", ""
		case "depends":
			return "depends", ""
		}
		return "value", s
	}
	var list []any
	if err := json.Unmarshal(f.Value, &list); err == nil {
		parts := make([]string, 0, len(list))
		for _, v := range list {
			parts = append(parts, fmt.Sprint(v))
		}
		return "value", strings.Join(parts, ", ")
	}
	var num float64
	if err := json.Unmarshal(f.Value, &num); err == nil {
		return "value", trimFloat(num)
	}
	// {"value": N, "unit": "…"} или {"max": N, "unit": "…"} (README базы знаний).
	var obj struct {
		Value *float64 `json:"value"`
		Max   *float64 `json:"max"`
		Unit  string   `json:"unit"`
	}
	if err := json.Unmarshal(f.Value, &obj); err == nil {
		switch {
		case obj.Value != nil:
			return "value", strings.TrimSpace(trimFloat(*obj.Value) + " " + obj.Unit)
		case obj.Max != nil:
			return "value", strings.TrimSpace("до " + trimFloat(*obj.Max) + " " + obj.Unit)
		}
	}
	return "unknown", ""
}

func trimFloat(v float64) string {
	return strings.TrimRight(strings.TrimRight(fmt.Sprintf("%.2f", v), "0"), ".")
}

// verified — факт можно показать покупателю.
// Показ решает порог GUIDE_MIN_FACT_STATUS (setMinStatus): verified_agent — для разработки и прогонов, verified —
// для показа жюри. draft, rejected, unconfirmed не показываются никогда.
func (f *guideFact) verified() bool { return f != nil && f.show }

// statusRank — ранг статуса записи: draft 0, verified_agent 1, verified 2; прочее — -1 (не показывать).
func statusRank(s string) int {
	switch s {
	case kbDraft:
		return 0
	case "verified_agent":
		return 1
	case kbVerified:
		return 2
	}
	return -1
}

// setMinStatus — порог показа фактов и текстов правил; неизвестный порог — verified.
func (kb *guideKB) setMinStatus(min string) {
	r := statusRank(min)
	if r < 1 {
		r = 2
	}
	kb.minStatus = min
	for _, f := range kb.facts {
		f.show = statusRank(f.Status) >= r
	}
	for i := range kb.rules {
		kb.rules[i].show = statusRank(kb.rules[i].Status) >= r
	}
	for _, fs := range kb.offFeed {
		for _, f := range fs {
			f.show = statusRank(f.Status) >= r
		}
	}
}

// applies — факт применим к платформе запроса ("" — без условия).
func (f *guideFact) applies(platform string) bool {
	if platform == "" || f.Applicability == nil {
		return true
	}
	p := strings.ToLower(f.Applicability["platform"])
	return p == "" || p == "any" || strings.Contains(p, strings.ToLower(platform))
}

// newGuideKB — база из сырых записей (загрузчик и тесты).
func newGuideKB(records []map[string]json.RawMessage, version string) *guideKB {
	kb := &guideKB{version: version, facts: map[string]*guideFact{}, byFamily: map[string][]*guideFact{},
		marks: map[string]guideOfferMark{}, lines: map[string]guideLine{}, subclasses: map[string][]string{},
		dirTitle: map[string]string{}, offFeed: map[string][]*guideFact{}, offFeedNames: map[string][]string{}}
	for _, r := range records {
		raw, _ := json.Marshal(r)
		has := func(k string) bool { _, ok := r[k]; return ok }
		nullFamily := has("family_id") && string(r["family_id"]) == "null"
		switch {
		case nullFamily && has("axis") && has("line"):
			var f guideFact
			var ln struct {
				Line string `json:"line"`
			}
			if json.Unmarshal(raw, &f) == nil && json.Unmarshal(raw, &ln) == nil && f.ID != "" && ln.Line != "" {
				if f.AxisLabel == "" {
					f.AxisLabel = f.Label
				}
				kb.offFeed[ln.Line] = append(kb.offFeed[ln.Line], &f)
			}
		case nullFamily && has("line") && has("vendor_name"):
			var ln struct {
				Line       string `json:"line"`
				VendorName string `json:"vendor_name"`
			}
			if json.Unmarshal(raw, &ln) == nil && ln.Line != "" {
				kb.offFeedNames[ln.Line] = append(kb.offFeedNames[ln.Line], ln.VendorName)
			}
		case has("axis") && has("family_id"):
			var f guideFact
			if json.Unmarshal(raw, &f) == nil && f.ID != "" {
				if f.Status == "" {
					f.Status = kbDraft
				}
				if f.AxisLabel == "" {
					f.AxisLabel = f.Label
				}
				kb.facts[f.ID] = &f
				kb.byFamily[f.FamilyID] = append(kb.byFamily[f.FamilyID], &f)
			}
		case has("message") && (has("when") || has("forbid") || has("require")):
			var g guideRule
			if json.Unmarshal(raw, &g) == nil && g.ID != "" {
				if g.Status == "" {
					g.Status = kbDraft
				}
				if g.Status != "rejected" && g.Status != "unconfirmed" { // источник говорит другое или его нет
					kb.rules = append(kb.rules, g)
				}
			}
		case has("offer_id") && (has("scope") || has("license_scope") || has("variant") || has("object") || has("sale_unit")):
			var m guideOfferMark
			if json.Unmarshal(raw, &m) == nil && m.OfferID != "" {
				if m.Scope == "" {
					m.Scope = m.LicenseScope
				}
				kb.marks[m.OfferID] = m.merge(kb.marks[m.OfferID])
			}
		case has("id") && has("families") && has("title"):
			var sc struct {
				ID       string     `json:"id"`
				Title    string     `json:"title"`
				Families [][]string `json:"families"`
			}
			if json.Unmarshal(raw, &sc) == nil && sc.ID != "" {
				if _, ok := kb.dirTitle[sc.ID]; !ok {
					kb.dirOrder = append(kb.dirOrder, sc.ID)
				}
				kb.dirTitle[sc.ID] = sc.Title
				for _, f := range sc.Families {
					if len(f) > 0 {
						kb.subclasses[sc.ID] = append(kb.subclasses[sc.ID], f[0])
					}
				}
			}
		case has("family_id") && has("line"):
			var l guideLine
			if json.Unmarshal(raw, &l) == nil && l.FamilyID != "" {
				kb.lines[l.FamilyID] = l
			}
		}
	}
	for _, fs := range kb.byFamily {
		sort.SliceStable(fs, func(i, j int) bool { return fs[i].ID < fs[j].ID })
	}
	kb.setMinStatus(kbVerified)
	return kb
}

// loadGuideKB — все *.json и *.jsonl каталога; нет каталога — пустая база (подбор работает на каталоге и
// правилах кода, без строк о функциях).
func loadGuideKB(dir string) (*guideKB, error) {
	entries, err := os.ReadDir(dir)
	if os.IsNotExist(err) {
		return newGuideKB(nil, "none"), nil
	}
	if err != nil {
		return nil, err
	}
	names := []string{}
	for _, e := range entries {
		n := e.Name()
		if !e.IsDir() && (strings.HasSuffix(n, ".json") || strings.HasSuffix(n, ".jsonl")) {
			names = append(names, n)
		}
	}
	sort.Strings(names)
	h := sha256.New()
	var records []map[string]json.RawMessage
	for _, n := range names {
		raw, err := os.ReadFile(filepath.Join(dir, n))
		if err != nil {
			return nil, err
		}
		h.Write([]byte(n))
		h.Write(raw)
		recs, err := kbRecords(n, raw)
		if err != nil {
			return nil, fmt.Errorf("%s: %w", n, err)
		}
		records = append(records, recs...)
	}
	return newGuideKB(records, "kb-"+hex.EncodeToString(h.Sum(nil))[:12]), nil
}

// kbRecords — объекты файла: JSONL построчно; JSON — массив объектов или объект, в котором массивы объектов
// (records, facts, rules, …) или сам объект — запись.
func kbRecords(name string, raw []byte) ([]map[string]json.RawMessage, error) {
	var out []map[string]json.RawMessage
	if strings.HasSuffix(name, ".jsonl") {
		sc := bufio.NewScanner(bytes.NewReader(raw))
		sc.Buffer(make([]byte, 1<<20), 1<<22)
		for sc.Scan() {
			line := bytes.TrimSpace(sc.Bytes())
			if len(line) == 0 || line[0] != '{' {
				continue
			}
			var m map[string]json.RawMessage
			if err := json.Unmarshal(line, &m); err != nil {
				return nil, err
			}
			out = append(out, m)
		}
		return out, sc.Err()
	}
	trimmed := bytes.TrimSpace(raw)
	if len(trimmed) > 0 && trimmed[0] == '[' {
		err := json.Unmarshal(trimmed, &out)
		return out, err
	}
	var obj map[string]json.RawMessage
	if err := json.Unmarshal(trimmed, &obj); err != nil {
		return nil, err
	}
	nested := false
	keys := make([]string, 0, len(obj))
	for k := range obj {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	for _, k := range keys {
		var arr []map[string]json.RawMessage
		if len(obj[k]) > 0 && obj[k][0] == '[' && json.Unmarshal(obj[k], &arr) == nil && len(arr) > 0 {
			out = append(out, arr...)
			nested = true
		}
	}
	if !nested {
		out = append(out, obj)
	}
	return out, nil
}

// guideKBState — база знаний процесса: загружается при первом подборе, перечитывается SetGuideKBDir.
type guideKBState struct {
	mu        sync.Mutex
	dir       string
	kb        *guideKB
	minStatus string // порог показа (GUIDE_MIN_FACT_STATUS)
}

// dataDirSource — источник каталога с каталогом файлов (generic: base_dir фида).
type dataDirSource interface{ DataDir() string }

// guideKBDir — data/softline/kb: соседний с base_dir фида каталог kb.
func (s *Services) guideKBDir() string {
	if d, ok := s.source.(dataDirSource); ok && d.DataDir() != "" {
		return filepath.Join(filepath.Dir(filepath.Clean(d.DataDir())), "kb")
	}
	return ""
}

// knowledge — база знаний подбора; ошибка чтения — пустая база и предупреждение.
func (g *guideService) knowledge() *guideKB {
	g.kb.mu.Lock()
	defer g.kb.mu.Unlock()
	if g.kb.kb != nil {
		return g.kb.kb
	}
	dir := g.kb.dir
	if dir == "" {
		dir = g.s.guideKBDir()
	}
	kb, err := loadGuideKB(dir)
	if err != nil || dir == "" {
		if err != nil {
			zap.L().Warn("подбор: база знаний не прочитана — без строк о функциях", zap.String("dir", dir), zap.Error(err))
		}
		kb = newGuideKB(nil, "none")
	}
	kb.setMinStatus(g.kb.minStatus)
	verified := 0
	for _, f := range kb.facts {
		if f.verified() {
			verified++
		}
	}
	zap.L().Info("подбор: база знаний", zap.String("version", kb.version), zap.Int("facts", len(kb.facts)),
		zap.String("min_status", kb.minStatus), zap.Int("shown", verified), zap.Int("rules", len(kb.rules)), zap.Int("offer_marks", len(kb.marks)),
		zap.Int("lines", len(kb.lines)))
	g.kb.kb = kb
	return kb
}

// ReloadGuideKB — перечитать базу знаний подбора (после выгрузки трека «Знания»); dir "" — data/softline/kb.
func (s *Services) ReloadGuideKB(dir string) {
	g := s.guide()
	g.kb.mu.Lock()
	g.kb.dir, g.kb.kb = dir, nil
	g.kb.mu.Unlock()
	g.knowledge()
}

// endpointFallback — семейства направления «Рабочие места и серверы», пока трек «Знания» не выгрузил
// security-subclasses.json в базу (тот же список, data/security-subclasses.json контура подбора).
var endpointFallback = []string{"3761", "3953", "3935", "419620", "4052", "420044", "4069", "4092", "329406"}

// subclass — семейства направления; нет в базе — запасной список для endpoint.
func (kb *guideKB) subclass(id string) []string {
	if fs := kb.subclasses[id]; len(fs) > 0 {
		return fs
	}
	if id == "endpoint" {
		return endpointFallback
	}
	return nil
}

// SetGuideMinFactStatus — порог показа фактов подбора (GUIDE_MIN_FACT_STATUS: verified_agent | verified).
func (s *Services) SetGuideMinFactStatus(min string) {
	g := s.guide()
	g.kb.mu.Lock()
	g.kb.minStatus = min
	if g.kb.kb != nil {
		g.kb.kb.setMinStatus(min)
	}
	g.kb.mu.Unlock()
}

// ruleByID — правило по идентификатору (пустое — нет).
func (kb *guideKB) ruleByID(id string) guideRule {
	for _, r := range kb.rules {
		if r.ID == id {
			return r
		}
	}
	return guideRule{}
}

// factYesAnyVariant — проверенный факт «есть» по оси у любого варианта семейства; nil — нет.
func (kb *guideKB) factYesAnyVariant(fid, axis string) *guideFact {
	for _, f := range kb.byFamily[fid] {
		if f.Axis == axis && f.verified() {
			if k, _ := f.factValue(); k == "yes" {
				return f
			}
		}
	}
	return nil
}
