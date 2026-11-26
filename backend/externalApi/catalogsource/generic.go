package catalogsource

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
)

// Generic — данные в любом табличном виде (CSV в UTF-8 или cp1251, JSON, JSONL, XLSX) по файлу
// сопоставления mapping.yaml. Результат тот же, что у MockJSON: domain.CatalogSnapshot, журнал событий,
// наборы запросов — импорт, поиск, аналитика и агент от формата не зависят.
//
// Недостающее (атрибуты лицензий, семейства плоского каталога, вендор, популярность) восстанавливает
// Enrich — его подставляет main (services.RestoreCatalog): правила разбора живут рядом с поиском.
//
// Прочитанное кешируется до изменения любого из файлов (размер и время записи).
type Generic struct {
	mappingPath string
	mapping     *Mapping
	enrich      Enricher

	mu    sync.Mutex
	cache *genericData
}

// Enricher — восстановление недостающих полей снимка по правилам; отчёт — по полю.
type Enricher func(snap *domain.CatalogSnapshot, events []domain.LogEvent) []FieldReport

// FieldReport — откуда взялось поле каталога: из данных, восстановлено правилами, суррогат или нет.
type FieldReport struct {
	Field      string `json:"field"`
	Total      int    `json:"total"`
	FromSource int    `json:"from_source"`
	Restored   int    `json:"restored"`
	// Status — source | restored | partial | surrogate | absent.
	Status string `json:"status"`
	Note   string `json:"note,omitempty"`
}

// Статусы FieldReport.
const (
	FieldFromSource = "source"
	FieldRestored   = "restored"
	FieldPartial    = "partial"
	FieldSurrogate  = "surrogate"
	FieldAbsent     = "absent"
)

// FileInfo — что прочитано из файла: формат, строки, найденные и ненайденные колонки.
type FileInfo struct {
	Table    string   `json:"table"`
	File     string   `json:"file"`
	Detected string   `json:"detected"`
	Rows     int      `json:"rows"`
	Skipped  int      `json:"skipped"`
	Mapped   []string `json:"mapped"`  // поля контракта, для которых колонка нашлась
	Missing  []string `json:"missing"` // колонки из сопоставления, которых нет в файле
	Unused   []string `json:"unused"`  // колонки файла, не попавшие ни в одно поле
	Notes    []string `json:"notes,omitempty"`
}

// QueryLogRow — строка журнала запросов: текст, частота, контекст.
type QueryLogRow struct {
	Text         string     `json:"text"`
	Freq         int        `json:"freq"`
	TS           *time.Time `json:"ts,omitempty"`
	SessionID    string     `json:"session_id,omitempty"`
	ClientID     *string    `json:"client_id,omitempty"`
	ResultsCount *int       `json:"results_count,omitempty"`
}

// genericData — всё прочитанное за один проход.
type genericData struct {
	sig      string
	snap     *domain.CatalogSnapshot
	events   []domain.LogEvent
	hasEv    bool
	queryLog []QueryLogRow
	queries  []domain.EvalQuery
	report   []FieldReport
	files    []FileInfo
}

// NewGeneric — источник по файлу сопоставления; ошибка в mapping.yaml видна при старте.
func NewGeneric(mappingPath string, enrich Enricher) (*Generic, error) {
	m, err := LoadMapping(mappingPath)
	if err != nil {
		return nil, err
	}
	return &Generic{mappingPath: mappingPath, mapping: m, enrich: enrich}, nil
}

// Name — источник для сводки импорта.
func (g *Generic) Name() string { return "generic:" + g.mappingPath }

// Load — снимок каталога (копия среза позиций: потребители могут его менять).
func (g *Generic) Load(ctx context.Context) (*domain.CatalogSnapshot, error) {
	d, err := g.data(ctx)
	if err != nil {
		return nil, err
	}
	s := *d.snap
	s.Products = append([]domain.Product(nil), d.snap.Products...)
	s.Warnings = append([]string(nil), d.snap.Warnings...)
	return &s, nil
}

// LoadEvents — журнал событий (events и, если в нём нет поисков, журнал запросов с датами).
func (g *Generic) LoadEvents(ctx context.Context) ([]domain.LogEvent, error) {
	d, err := g.data(ctx)
	if err != nil {
		return nil, err
	}
	if !d.hasEv {
		return nil, ErrNoFile
	}
	return append([]domain.LogEvent(nil), d.events...), nil
}

// LoadQueries — наборы оценки в формате контракта: eval_queries из mapping.yaml, иначе queries.json в
// каталоге данных (туда их пишет `go run . buildsets`); нет файла — ErrNoFile.
func (g *Generic) LoadQueries(ctx context.Context) ([]domain.EvalQuery, error) {
	d, err := g.data(ctx)
	if err != nil {
		return nil, err
	}
	if d.queries == nil {
		return nil, ErrNoFile
	}
	return append([]domain.EvalQuery(nil), d.queries...), nil
}

// DataDir — каталог файлов данных (base_dir): сюда `go run . buildsets` пишет queries.json и SETS.md.
func (g *Generic) DataDir() string { return g.mapping.dir }

// queriesPath — где лежат наборы оценки.
func (m *Mapping) queriesPath() string {
	if m.EvalQueries != "" {
		return m.path(m.EvalQueries)
	}
	return m.path("queries.json")
}

// QueryLog — журнал запросов (query_log); нет — ErrNoFile.
func (g *Generic) QueryLog(ctx context.Context) ([]QueryLogRow, error) {
	if g.mapping.QueryLog == nil {
		return nil, ErrNoFile
	}
	d, err := g.data(ctx)
	if err != nil {
		return nil, err
	}
	return d.queryLog, nil
}

// Provenance — откуда взялось каждое поле каталога (после последней загрузки).
func (g *Generic) Provenance(ctx context.Context) ([]FieldReport, error) {
	d, err := g.data(ctx)
	if err != nil {
		return nil, err
	}
	return d.report, nil
}

// Files — что прочитано из каждого файла.
func (g *Generic) Files(ctx context.Context) ([]FileInfo, error) {
	d, err := g.data(ctx)
	if err != nil {
		return nil, err
	}
	return d.files, nil
}

// signature — размер и время записи всех файлов: изменился любой — перечитать.
func (g *Generic) signature() string {
	var b strings.Builder
	paths := []string{g.mappingPath}
	for _, t := range g.mapping.tables() {
		if t != nil {
			paths = append(paths, g.mapping.path(t.File))
		}
	}
	paths = append(paths, g.mapping.queriesPath())
	sort.Strings(paths)
	for _, p := range paths {
		if st, err := os.Stat(p); err == nil {
			fmt.Fprintf(&b, "%s:%d:%d;", p, st.Size(), st.ModTime().UnixNano())
		} else {
			fmt.Fprintf(&b, "%s:-;", p)
		}
	}
	return b.String()
}

func (g *Generic) data(ctx context.Context) (*genericData, error) {
	g.mu.Lock()
	defer g.mu.Unlock()
	sig := g.signature()
	if g.cache != nil && g.cache.sig == sig {
		return g.cache, nil
	}
	// mapping.yaml мог поменяться — перечитать и его.
	m, err := LoadMapping(g.mappingPath)
	if err != nil {
		return nil, err
	}
	g.mapping = m
	d, err := (&genericLoader{m: m, enrich: g.enrich}).load(ctx)
	if err != nil {
		return nil, err
	}
	d.sig = sig
	g.cache = d
	return d, nil
}

// genericLoader — один проход чтения.
type genericLoader struct {
	m      *Mapping
	enrich Enricher
	tp     *timeParser
	d      *genericData

	vendorByKey   map[string]string // id, fold(name), slug → id
	catByKey      map[string]string
	productByKey  map[string]string // id, sku → id
	familyByKey   map[string]string // id, fold(name) → family id
	categoryChild map[string]string // parent id + "\x00" + fold(name) → id
}

// table — строки таблицы с разрешёнными колонками.
type table struct {
	name string
	t    *Table
	td   *tableData
	cols map[string][]string // поле → настоящие имена колонок файла
	info *FileInfo
}

func (l *genericLoader) open(name string, t *Table) (*table, error) {
	td, err := readTable(l.m, t)
	if err != nil {
		return nil, err
	}
	tb := &table{name: name, t: t, td: td, cols: map[string][]string{},
		info: &FileInfo{Table: name, File: t.File, Detected: td.detected, Rows: len(td.rows)}}
	byFold := map[string]string{}
	for _, h := range td.headers {
		byFold[fold(h)] = h
	}
	used := map[string]bool{}
	fields := make([]string, 0, len(t.Columns))
	for f := range t.Columns {
		fields = append(fields, f)
	}
	sort.Strings(fields)
	for _, f := range fields {
		for _, c := range t.Columns[f] {
			real, ok := "", false
			if _, exact := headerSet(td.headers)[c]; exact {
				real, ok = c, true
			} else if h, found := byFold[fold(c)]; found {
				real, ok = h, true
			}
			if ok {
				tb.cols[f] = append(tb.cols[f], real)
				used[real] = true
			} else {
				tb.info.Missing = append(tb.info.Missing, f+" ← "+c)
			}
		}
		if len(tb.cols[f]) > 0 {
			tb.info.Mapped = append(tb.info.Mapped, f)
		}
	}
	for _, a := range t.Attributes {
		col, _ := splitAttr(a)
		if h, ok := byFold[fold(col)]; ok {
			used[h] = true
		} else {
			tb.info.Missing = append(tb.info.Missing, "attributes ← "+col)
		}
	}
	for _, h := range td.headers {
		if !used[h] && !strings.Contains(h, ".") {
			tb.info.Unused = append(tb.info.Unused, h)
		}
	}
	// Ключи словарей значений сравниваются свёрнутыми.
	for f, vals := range t.Values {
		folded := make(map[string]string, len(vals))
		for k, v := range vals {
			folded[fold(k)] = v
		}
		t.Values[f] = folded
	}
	return tb, nil
}

func (tb *table) finish(l *genericLoader) { l.d.files = append(l.d.files, *tb.info) }

func headerSet(h []string) map[string]struct{} {
	m := make(map[string]struct{}, len(h))
	for _, x := range h {
		m[x] = struct{}{}
	}
	return m
}

// splitAttr — «Платформа: platform» → колонка и ключ атрибута; без двоеточия ключ = колонка.
func splitAttr(a string) (string, string) {
	if i := strings.LastIndex(a, ":"); i > 0 {
		return strings.TrimSpace(a[:i]), strings.TrimSpace(a[i+1:])
	}
	return strings.TrimSpace(a), strings.TrimSpace(a)
}

// get — значение поля строки: первая непустая колонка, затем словарь значений, затем default.
func (tb *table) get(rec record, field string) string {
	v := ""
	for _, c := range tb.cols[field] {
		if x := strings.TrimSpace(rec[c]); x != "" {
			v = x
			break
		}
	}
	if v == "" {
		v = tb.t.Defaults[field]
	}
	if v != "" {
		if mapped, ok := tb.t.Values[field][fold(v)]; ok {
			return mapped
		}
	}
	return v
}

func (tb *table) has(field string) bool { return len(tb.cols[field]) > 0 || tb.t.Defaults[field] != "" }

func (l *genericLoader) load(ctx context.Context) (*genericData, error) {
	tp, err := newTimeParser(l.m.DateFormats, l.m.Timezone)
	if err != nil {
		return nil, err
	}
	l.tp = tp
	l.d = &genericData{}
	l.vendorByKey, l.catByKey, l.productByKey = map[string]string{}, map[string]string{}, map[string]string{}
	l.familyByKey, l.categoryChild = map[string]string{}, map[string]string{}
	snap := &domain.CatalogSnapshot{Version: l.m.Version}
	if snap.Version == "" {
		snap.Version = "generic-" + Slug(strings.TrimSuffix(l.m.Catalog.File, ".xlsx"))
	}
	l.d.snap = snap

	steps := []struct {
		t    *Table
		name string
		fn   func(*table) error
	}{
		{l.m.Vendors, "vendors", l.loadVendors},
		{l.m.Categories, "categories", l.loadCategories},
		{l.m.Catalog, "catalog", l.loadCatalog},
		{l.m.Events, "events", l.loadEvents},
		{l.m.QueryLog, "query_log", l.loadQueryLog},
	}
	for _, s := range steps {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if s.t == nil {
			continue
		}
		tb, err := l.open(s.name, s.t)
		if err != nil {
			return nil, err
		}
		if err := s.fn(tb); err != nil {
			return nil, err
		}
		tb.finish(l)
	}
	sort.SliceStable(l.d.events, func(i, j int) bool { return l.d.events[i].TS.Before(l.d.events[j].TS) })

	// Восстановление — до связей и покупок: им нужны семейства и id позиций.
	if l.enrich != nil {
		l.d.report = l.enrich(snap, l.d.events)
	}
	for _, p := range snap.Products {
		l.familyByKey[p.FamilyID] = p.FamilyID
		if p.FamilyName != "" {
			if _, ok := l.familyByKey[fold(p.FamilyName)]; !ok {
				l.familyByKey[fold(p.FamilyName)] = p.FamilyID
			}
		}
	}

	later := []struct {
		t    *Table
		name string
		fn   func(*table) error
	}{
		{l.m.Relations, "relations", l.loadRelations},
		{l.m.Clients, "clients", l.loadClients},
		{l.m.Purchases, "purchases", l.loadPurchases},
		{l.m.Aliases, "aliases", l.loadAliases},
	}
	for _, s := range later {
		if s.t == nil {
			snap.Warnings = append(snap.Warnings, s.name+": нет в сопоставлении")
			continue
		}
		tb, err := l.open(s.name, s.t)
		if err != nil {
			return nil, err
		}
		if err := s.fn(tb); err != nil {
			return nil, err
		}
		tb.finish(l)
	}
	l.ensureGuest()
	if err := l.loadP2(); err != nil {
		return nil, err
	}
	qs, err := loadQueriesFile(l.m.queriesPath())
	switch {
	case errors.Is(err, ErrNoFile) && l.m.EvalQueries == "":
		// Наборов ещё нет: соберёт `go run . buildsets`.
	case errors.Is(err, ErrNoFile):
		return nil, fmt.Errorf("eval_queries %s: файла нет", l.m.EvalQueries)
	case err != nil:
		return nil, err
	default:
		if qs == nil {
			qs = []domain.EvalQuery{}
		}
		l.d.queries = qs
	}
	if err := validate(snap); err != nil {
		return nil, err
	}
	for _, f := range l.d.files {
		if len(f.Missing) > 0 {
			snap.Warnings = append(snap.Warnings, fmt.Sprintf("%s: нет колонок %s", f.File, strings.Join(f.Missing, ", ")))
		}
		if f.Skipped > 0 {
			snap.Warnings = append(snap.Warnings, fmt.Sprintf("%s: пропущено строк %d (%s)", f.File, f.Skipped, strings.Join(f.Notes, "; ")))
		}
	}
	return l.d, nil
}

// --- справочники ---

func (l *genericLoader) addVendor(v domain.Vendor) {
	l.d.snap.Vendors = append(l.d.snap.Vendors, v)
	for _, k := range []string{v.ID, fold(v.ID), fold(v.Name), Slug(v.Name)} {
		if k != "" {
			if _, ok := l.vendorByKey[k]; !ok {
				l.vendorByKey[k] = v.ID
			}
		}
	}
}

// vendorID — id вендора по id, названию или слагу; незнакомый — новый вендор из текста.
func (l *genericLoader) vendorID(raw string) string {
	if raw == "" {
		return ""
	}
	for _, k := range []string{raw, fold(raw), Slug(raw)} {
		if id, ok := l.vendorByKey[k]; ok {
			return id
		}
	}
	id := Slug(raw)
	if id == "" {
		return ""
	}
	l.addVendor(domain.Vendor{ID: id, Name: raw})
	return id
}

func (l *genericLoader) loadVendors(tb *table) error {
	for _, rec := range tb.td.rows {
		name := tb.get(rec, "name")
		if name == "" {
			tb.skip("нет названия")
			continue
		}
		id := tb.get(rec, "id")
		if id == "" {
			id = Slug(name)
		}
		if _, dup := l.vendorByKey[id]; dup {
			tb.skip("повтор id")
			continue
		}
		l.addVendor(domain.Vendor{ID: id, Name: name, Origin: normOrigin(tb.get(rec, "origin"))})
	}
	return nil
}

func (tb *table) skip(why string) {
	tb.info.Skipped++
	for _, n := range tb.info.Notes {
		if n == why {
			return
		}
	}
	tb.info.Notes = append(tb.info.Notes, why)
}

func (l *genericLoader) addCategory(c domain.Category) {
	l.d.snap.Categories = append(l.d.snap.Categories, c)
	for _, k := range []string{c.ID, fold(c.ID), fold(c.Name)} {
		if _, ok := l.catByKey[k]; !ok {
			l.catByKey[k] = c.ID
		}
	}
	parent := ""
	if c.ParentID != nil {
		parent = *c.ParentID
	}
	l.categoryChild[parent+"\x00"+fold(c.Name)] = c.ID
}

// categoryPath — разделитель пути категорий: из сопоставления или первый встретившийся.
func (l *genericLoader) categoryPath(raw string) []string {
	seps := []string{" > ", ">", " / ", " | ", "|", " \\ ", "\\", " → ", "→"}
	if l.m.CategorySeparator != "" {
		seps = []string{l.m.CategorySeparator}
	}
	for _, s := range seps {
		if strings.Contains(raw, s) {
			var parts []string
			for _, p := range strings.Split(raw, s) {
				if p = strings.TrimSpace(p); p != "" {
					parts = append(parts, p)
				}
			}
			return parts
		}
	}
	return []string{strings.TrimSpace(raw)}
}

// categoryID — id категории по id, названию или пути «ПО > Антивирусы»; недостающие уровни создаются.
func (l *genericLoader) categoryID(raw string) string {
	if raw == "" {
		return ""
	}
	path := l.categoryPath(raw)
	if len(path) == 1 {
		for _, k := range []string{raw, fold(raw)} {
			if id, ok := l.catByKey[k]; ok {
				return id
			}
		}
	}
	parent := ""
	for i, name := range path {
		key := parent + "\x00" + fold(name)
		if id, ok := l.categoryChild[key]; ok {
			parent = id
			continue
		}
		// Корень пути может совпасть с уже известной категорией по названию.
		if i == 0 {
			if id, ok := l.catByKey[fold(name)]; ok {
				parent = id
				continue
			}
		}
		id := Slug(strings.Join(path[:i+1], " "))
		if _, taken := l.catByKey[id]; taken {
			id += "-" + strconv.Itoa(len(l.d.snap.Categories)+1)
		}
		c := domain.Category{ID: id, Name: name}
		if parent != "" {
			p := parent
			c.ParentID = &p
		}
		l.addCategory(c)
		parent = id
	}
	return parent
}

func (l *genericLoader) loadCategories(tb *table) error {
	type pending struct {
		id, name, parentRaw string
	}
	var rows []pending
	for _, rec := range tb.td.rows {
		if p := tb.get(rec, "path"); p != "" {
			l.categoryID(p)
			continue
		}
		name := tb.get(rec, "name")
		if name == "" {
			tb.skip("нет названия")
			continue
		}
		id := tb.get(rec, "id")
		if id == "" {
			id = Slug(name)
		}
		parent := tb.get(rec, "parent_id")
		if parent == "" {
			parent = tb.get(rec, "parent")
		}
		rows = append(rows, pending{id, name, parent})
		l.addCategory(domain.Category{ID: id, Name: name})
	}
	// Родители — вторым проходом: в файле ребёнок может стоять раньше родителя.
	cats := l.d.snap.Categories
	for _, r := range rows {
		if r.parentRaw == "" {
			continue
		}
		pid, ok := l.catByKey[r.parentRaw]
		if !ok {
			pid, ok = l.catByKey[fold(r.parentRaw)]
		}
		if !ok || pid == r.id {
			tb.skip("родитель не найден")
			continue
		}
		for i := range cats {
			if cats[i].ID == r.id {
				p := pid
				cats[i].ParentID = &p
				l.categoryChild[pid+"\x00"+fold(cats[i].Name)] = r.id
			}
		}
	}
	return nil
}

// --- каталог ---

var licenseTypeWords = []struct{ stem, value string }{
	{"продл", domain.LicenseRenewal}, {"renew", domain.LicenseRenewal},
	{"бессроч", domain.LicensePerpetual}, {"perpet", domain.LicensePerpetual}, {"постоян", domain.LicensePerpetual},
	{"подпис", domain.LicenseSubscription}, {"subscr", domain.LicenseSubscription}, {"срочн", domain.LicenseSubscription},
	{"обновл", domain.LicenseUpgrade}, {"upgrade", domain.LicenseUpgrade}, {"oem", domain.LicenseOEM},
	{"облач", domain.LicenseCloud}, {"cloud", domain.LicenseCloud}, {"saas", domain.LicenseCloud},
}

func normLicenseType(raw string) string {
	f := fold(raw)
	for _, w := range licenseTypeWords {
		if strings.Contains(f, w.stem) {
			return w.value
		}
	}
	return ""
}

var metricWords = []struct{ stem, value string }{
	{"устр", domain.MetricDevice}, {"device", domain.MetricDevice}, {"пк", domain.MetricDevice}, {"комп", domain.MetricDevice},
	{"рабоч", domain.MetricDevice}, {"узел", domain.MetricDevice}, {"польз", domain.MetricUser}, {"user", domain.MetricUser},
	{"сотруд", domain.MetricUser}, {"сервер", domain.MetricServer}, {"server", domain.MetricServer},
	{"ядр", domain.MetricCore}, {"core", domain.MetricCore}, {"орган", domain.MetricOrg}, {"org", domain.MetricOrg},
}

func normMetric(raw string) string {
	f := fold(raw)
	for _, w := range metricWords {
		if strings.Contains(f, w.stem) {
			return w.value
		}
	}
	return ""
}

// normOrigin — ru | foreign | "" (не понято).
func normOrigin(raw string) string {
	f := fold(raw)
	switch {
	case f == "":
		return ""
	case f == "ru" || strings.Contains(f, "росс") || strings.Contains(f, "отечеств") || f == "рф" || f == "russia":
		return "ru"
	case f == "foreign" || strings.Contains(f, "иностр") || strings.Contains(f, "зарубеж") || len(f) == 2:
		return "foreign"
	}
	return ""
}

func (l *genericLoader) loadCatalog(tb *table) error {
	snap := l.d.snap
	popMax := 0.0
	for n, rec := range tb.td.rows {
		title := tb.get(rec, "title")
		if title == "" {
			tb.skip("нет названия")
			continue
		}
		p := domain.Product{Title: title, SKU: tb.get(rec, "sku"), Description: tb.get(rec, "description")}
		p.ID = tb.get(rec, "id")
		if p.ID == "" {
			p.ID = p.SKU
		}
		if p.ID == "" {
			p.ID = Slug(title) + "-" + strconv.Itoa(n+1)
		}
		if _, dup := l.productByKey[p.ID]; dup {
			tb.skip("повтор id")
			continue
		}
		p.VendorID = l.vendorID(tb.get(rec, "vendor"))
		p.CategoryID = l.categoryID(tb.get(rec, "category"))
		p.FamilyName = tb.get(rec, "family_name")
		p.FamilyID = tb.get(rec, "family_id")
		if p.FamilyID == "" && p.FamilyName != "" {
			p.FamilyID = Slug(p.FamilyName)
		}
		if raw := tb.get(rec, "price_rub"); raw != "" {
			v, onReq, ok := parsePrice(raw)
			switch {
			case ok && onReq:
				p.PriceOnRequest = true
			case ok:
				p.PriceRub = &v
			}
		}
		if raw := tb.get(rec, "price_on_request"); raw != "" {
			if b, ok := parseBool(raw); ok && b {
				p.PriceOnRequest, p.PriceRub = true, nil
			}
		}
		if p.PriceRub == nil && tb.has("price_rub") && !tb.has("price_on_request") {
			p.PriceOnRequest = true // колонка цены есть, ячейка пуста — «по запросу»
		}
		lic := &p.License
		if raw := tb.get(rec, "license_type"); raw != "" {
			if lic.Type = normLicenseType(raw); lic.Type == "" {
				lic.Type = strings.ToLower(raw)
			}
		}
		if raw := tb.get(rec, "license_metric"); raw != "" {
			if m := normMetric(raw); m != "" {
				lic.Metric = &m
			}
		}
		if raw := tb.get(rec, "license_quantity"); raw != "" {
			if q, ok := parseInt(raw); ok && q > 0 {
				lic.Quantity = &q
			}
			if lic.Metric == nil {
				if m := normMetric(raw); m != "" {
					lic.Metric = &m // «10 ПК» в колонке количества
				}
			}
		}
		for _, f := range []string{"license_duration_months", "license_duration"} {
			if raw := tb.get(rec, f); raw != "" && lic.DurationMonths == nil {
				if f == "license_duration_months" {
					if v, ok := parseInt(raw); ok && v > 0 {
						lic.DurationMonths = &v
						continue
					}
				}
				if v, ok := parseDurationMonths(raw); ok && v != nil {
					lic.DurationMonths = v
				} else if ok && lic.Type == "" {
					lic.Type = domain.LicensePerpetual
				}
			}
		}
		lic.Edition = tb.get(rec, "license_edition")
		p.Origin = normOrigin(tb.get(rec, "origin"))
		if raw := tb.get(rec, "in_reestr"); raw != "" {
			p.InReestr, _ = parseBool(raw)
		}
		if raw := tb.get(rec, "popularity"); raw != "" {
			if v, ok := parseNumber(raw); ok && v > 0 {
				p.Popularity = v
				popMax = max(popMax, v)
			}
		}
		if raw := tb.get(rec, "margin"); raw != "" {
			if v, ok := parseNumber(raw); ok && v > 0 {
				if v > 1 || strings.Contains(raw, "%") {
					v /= 100
				}
				p.Margin = min(v, 1)
			}
		}
		attrs := map[string]any{}
		for _, a := range tb.t.Attributes {
			col, key := splitAttr(a)
			for _, h := range tb.td.headers {
				if fold(h) == fold(col) && strings.TrimSpace(rec[h]) != "" {
					attrs[key] = strings.TrimSpace(rec[h])
				}
			}
		}
		if lic.Edition != "" {
			if _, ok := attrs["edition"]; !ok {
				attrs["edition"] = lic.Edition
			}
		}
		if len(attrs) > 0 {
			raw, _ := json.Marshal(attrs)
			p.Attributes = domain.JSONB(raw)
		}
		l.hardwareFields(tb, rec, &p)
		snap.Products = append(snap.Products, p)
		l.productByKey[p.ID] = p.ID
		if p.SKU != "" {
			if _, ok := l.productByKey[p.SKU]; !ok {
				l.productByKey[p.SKU] = p.ID
			}
		}
	}
	// Популярность в «штуках» (продажи, просмотры) — в 0..1 от максимума.
	if popMax > 1 {
		for i := range snap.Products {
			snap.Products[i].Popularity = snap.Products[i].Popularity / popMax
		}
	}
	// Происхождение позиции без своего поля — от вендора.
	origin := map[string]string{}
	for _, v := range snap.Vendors {
		origin[v.ID] = v.Origin
	}
	for i := range snap.Products {
		if snap.Products[i].Origin == "" {
			snap.Products[i].Origin = origin[snap.Products[i].VendorID]
		}
	}
	return nil
}

// productID — id позиции по id или артикулу; "" — не найдена.
func (l *genericLoader) productID(raw string) string {
	if raw == "" {
		return ""
	}
	if id, ok := l.productByKey[raw]; ok {
		return id
	}
	return l.productByKey[strings.TrimSpace(raw)]
}

// --- журналы ---

var eventTypeWords = []struct{ stem, value string }{
	{"search", "search"}, {"поиск", "search"}, {"запрос цен", "price_request"}, {"price", "price_request"},
	{"цен", "price_request"}, {"manager", "manager_request"}, {"менедж", "manager_request"},
	{"cart", "add_to_cart"}, {"корзин", "add_to_cart"}, {"basket", "add_to_cart"},
	{"order", "order"}, {"заказ", "order"}, {"purchase", "order"}, {"покупк", "order"}, {"оплат", "order"},
	{"click", "click"}, {"клик", "click"}, {"переход", "click"}, {"view", "click"}, {"просмотр", "click"},
}

func normEventType(raw string) string {
	f := fold(raw)
	for _, w := range eventTypeWords {
		if strings.Contains(f, w.stem) {
			return w.value
		}
	}
	return ""
}

func optString(s string) *string {
	if s == "" {
		return nil
	}
	return &s
}

func optInt(s string) *int {
	if v, ok := parseInt(s); ok && s != "" {
		return &v
	}
	return nil
}

func (l *genericLoader) loadEvents(tb *table) error {
	l.d.hasEv = true
	synthetic := 0
	unknownProduct := 0
	for n, rec := range tb.td.rows {
		ts, ok := l.tp.parse(tb.get(rec, "ts"))
		if !ok {
			tb.skip("дата не разобрана")
			continue
		}
		typ := normEventType(tb.get(rec, "type"))
		if typ == "" {
			tb.skip("неизвестный тип события")
			continue
		}
		ev := domain.LogEvent{TS: ts, Type: typ, SessionID: tb.get(rec, "session_id"),
			ClientID: optString(tb.get(rec, "client_id")), Query: tb.get(rec, "query"),
			ResultsCount: optInt(tb.get(rec, "results_count")), Position: optInt(tb.get(rec, "position"))}
		if raw := tb.get(rec, "product"); raw != "" {
			if ev.ProductID = l.productID(raw); ev.ProductID == "" {
				unknownProduct++
				ev.ProductID = raw
			}
		}
		if typ == "search" && ev.Query == "" {
			tb.skip("поиск без текста")
			continue
		}
		if ev.SessionID == "" {
			ev.SessionID = syntheticSession(ev.ClientID, ts, n)
			synthetic++
		}
		l.d.events = append(l.d.events, ev)
	}
	if synthetic > 0 {
		tb.info.Notes = append(tb.info.Notes, fmt.Sprintf("сессии синтезированы у %d событий (клиент + час)", synthetic))
	}
	if unknownProduct > 0 {
		tb.info.Notes = append(tb.info.Notes, fmt.Sprintf("позиция не найдена в каталоге у %d событий", unknownProduct))
	}
	return nil
}

// syntheticSession — сессии нет: клиент и час события (гость — строка файла: без связи действий).
func syntheticSession(client *string, ts time.Time, row int) string {
	if client == nil {
		return "row-" + strconv.Itoa(row+1)
	}
	return "syn-" + *client + "-" + ts.UTC().Format("2006010215")
}

func (l *genericLoader) loadQueryLog(tb *table) error {
	hasSearch := false
	for _, e := range l.d.events {
		if e.Type == "search" {
			hasSearch = true
			break
		}
	}
	added := 0
	for n, rec := range tb.td.rows {
		text := tb.get(rec, "text")
		if text == "" {
			tb.skip("нет текста")
			continue
		}
		row := QueryLogRow{Text: text, Freq: 1, SessionID: tb.get(rec, "session_id"),
			ClientID: optString(tb.get(rec, "client_id")), ResultsCount: optInt(tb.get(rec, "results_count"))}
		if f, ok := parseInt(tb.get(rec, "freq")); ok && f > 0 {
			row.Freq = f
		}
		if ts, ok := l.tp.parse(tb.get(rec, "ts")); ok {
			row.TS = &ts
		}
		l.d.queryLog = append(l.d.queryLog, row)
		// Журнал запросов с датами — поиски в журнал событий, если там поисков нет.
		if !hasSearch && row.TS != nil {
			s := row.SessionID
			if s == "" {
				s = syntheticSession(row.ClientID, *row.TS, n)
			}
			l.d.events = append(l.d.events, domain.LogEvent{TS: *row.TS, SessionID: s, ClientID: row.ClientID,
				Type: "search", Query: text, ResultsCount: row.ResultsCount})
			l.d.hasEv = true
			added++
		}
	}
	if added > 0 {
		tb.info.Notes = append(tb.info.Notes, fmt.Sprintf("%d запросов с датами добавлены в журнал событий как поиски", added))
	}
	return nil
}

// --- связи, клиенты, алиасы ---

var relationWords = []struct{ stem, value string }{
	{"аналог", domain.RelationSubstitute}, {"замен", domain.RelationSubstitute}, {"substit", domain.RelationSubstitute},
	{"альтерн", domain.RelationSubstitute}, {"сопутств", domain.RelationComplement}, {"complem", domain.RelationComplement},
	{"вместе", domain.RelationComplement}, {"доп", domain.RelationComplement}, {"продл", domain.RelationRenewal},
	{"renew", domain.RelationRenewal}, {"совмест", domain.RelationCompatibility}, {"compat", domain.RelationCompatibility},
	{"upgrad", domain.RelationUpgrade}, {"апгрейд", domain.RelationUpgrade}, {"старш", domain.RelationUpgrade},
}

func normRelation(raw string) string {
	f := fold(raw)
	for _, w := range relationWords {
		if strings.Contains(f, w.stem) {
			return w.value
		}
	}
	return ""
}

// familyRef — семейство по id семейства, id или артикулу позиции, названию семейства.
func (l *genericLoader) familyRef(raw string) string {
	if raw == "" {
		return ""
	}
	if id, ok := l.familyByKey[raw]; ok {
		return id
	}
	if pid := l.productID(raw); pid != "" {
		for _, p := range l.d.snap.Products {
			if p.ID == pid {
				return p.FamilyID
			}
		}
	}
	if id, ok := l.familyByKey[fold(raw)]; ok {
		return id
	}
	return l.familyByKey[Slug(raw)]
}

func (l *genericLoader) loadRelations(tb *table) error {
	seen := map[string]bool{}
	for _, rec := range tb.td.rows {
		from, to := l.familyRef(tb.get(rec, "from")), l.familyRef(tb.get(rec, "to"))
		if from == "" || to == "" {
			tb.skip("семейство не найдено")
			continue
		}
		typ := normRelation(tb.get(rec, "type"))
		if typ == "" {
			typ = domain.RelationSubstitute
			if tb.has("type") {
				tb.skip("неизвестный тип связи")
				continue
			}
		}
		key := from + "\x00" + to + "\x00" + typ
		if from == to || seen[key] {
			continue
		}
		seen[key] = true
		rel := domain.Relation{FromFamily: from, ToFamily: to, Type: typ, Note: tb.get(rec, "note")}
		l.relationFields(tb, rec, &rel)
		l.d.snap.Relations = append(l.d.snap.Relations, rel)
	}
	return nil
}

func normSegment(raw string) string {
	f := fold(raw)
	switch {
	case f == "":
		return ""
	case strings.Contains(f, "smb") || strings.Contains(f, "мал") || strings.Contains(f, "сред") || strings.Contains(f, "мсб"):
		return domain.SegmentSMB
	case strings.Contains(f, "enter") || strings.Contains(f, "корп") || strings.Contains(f, "круп"):
		return domain.SegmentEnterprise
	case strings.Contains(f, "gov") || strings.Contains(f, "гос") || strings.Contains(f, "бюдж"):
		return domain.SegmentGov
	case strings.Contains(f, "indiv") || strings.Contains(f, "физ") || strings.Contains(f, "част") || strings.Contains(f, "b2c"):
		return domain.SegmentIndividual
	}
	return ""
}

func (l *genericLoader) clientIndex() map[string]int {
	idx := make(map[string]int, len(l.d.snap.Clients))
	for i, c := range l.d.snap.Clients {
		idx[c.ID] = i
	}
	return idx
}

func (l *genericLoader) loadClients(tb *table) error {
	idx := l.clientIndex()
	for _, rec := range tb.td.rows {
		id := tb.get(rec, "id")
		if id == "" {
			tb.skip("нет id")
			continue
		}
		if _, dup := idx[id]; dup {
			tb.skip("повтор id")
			continue
		}
		name := tb.get(rec, "name")
		if name == "" {
			name = id
		}
		idx[id] = len(l.d.snap.Clients)
		l.d.snap.Clients = append(l.d.snap.Clients, domain.Client{ID: id, Name: name, Segment: normSegment(tb.get(rec, "segment")),
			Purchases: []domain.Purchase{}})
	}
	return nil
}

func (l *genericLoader) loadPurchases(tb *table) error {
	idx := l.clientIndex()
	for _, rec := range tb.td.rows {
		cid, pid := tb.get(rec, "client_id"), l.productID(tb.get(rec, "product"))
		if pid == "" {
			tb.skip("позиция не найдена")
			continue
		}
		i, ok := idx[cid]
		if !ok {
			i = len(l.d.snap.Clients)
			idx[cid] = i
			l.d.snap.Clients = append(l.d.snap.Clients, domain.Client{ID: cid, Name: cid})
		}
		date := tb.get(rec, "date")
		if ts, ok := l.tp.parse(date); ok {
			date = ts.Format("2006-01-02")
		}
		l.d.snap.Clients[i].Purchases = append(l.d.snap.Clients[i].Purchases, domain.Purchase{ProductID: pid, Date: date})
	}
	return nil
}

// ensureGuest — гость без истории обязателен: на нём открывается переключатель клиента.
func (l *genericLoader) ensureGuest() {
	for _, c := range l.d.snap.Clients {
		if c.ID == "guest" {
			return
		}
	}
	l.d.snap.Clients = append(l.d.snap.Clients, domain.Client{ID: "guest", Name: "Гость", Segment: domain.SegmentIndividual,
		Purchases: []domain.Purchase{}})
}

func (l *genericLoader) loadAliases(tb *table) error {
	for _, rec := range tb.td.rows {
		a := domain.Alias{Alias: strings.ToLower(tb.get(rec, "alias")), Canonical: strings.ToLower(tb.get(rec, "canonical")),
			Kind: tb.get(rec, "kind"), Target: tb.get(rec, "target"), TargetID: tb.get(rec, "target_id")}
		if a.Alias == "" || a.Canonical == "" {
			tb.skip("пустой алиас")
			continue
		}
		if a.Kind == "" {
			a.Kind = "alias"
		}
		if a.Target == "" {
			a.Target = domain.AliasTargetTerm
		}
		l.d.snap.Aliases = append(l.d.snap.Aliases, a)
	}
	return nil
}
