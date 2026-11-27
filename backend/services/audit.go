package services

import (
	"context"
	"fmt"
	"math"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/in"
	"kuznica/api/pkg/errs"
)

// Аудит данных: объёмы, покрытие полей, что восстановлено
// правилами, доля запросов без результата в B0 и B3 и вывод «что меняется в плане» по ARCHITECTURE §9.
// Считается по тому же источнику, что импорт; поиск — по текущему индексу, журнал не пишется.

// AuditOptions — параметры аудита.
type AuditOptions struct {
	// Limit — сколько самых частых запросов прогнать через поиск (0 — 300).
	Limit int
	// NoSearch — без прогона поиска (индекса ещё нет).
	NoSearch bool
}

// AuditReport — итог аудита (data/AUDIT.json).
type AuditReport struct {
	GeneratedAt    time.Time                   `json:"generated_at"`
	Source         string                      `json:"source"`
	CatalogVersion string                      `json:"catalog_version"`
	Files          []catalogsource.FileInfo    `json:"files"`
	Warnings       []string                    `json:"warnings"`
	Volumes        AuditVolumes                `json:"volumes"`
	Fields         []AuditField                `json:"fields"`
	Structure      AuditStructure              `json:"structure"`
	Search         *AuditSearch                `json:"search,omitempty"`
	Plan           []AuditPlanItem             `json:"plan"`
	P2             *AuditP2                    `json:"p2"`
	Provenance     []catalogsource.FieldReport `json:"provenance,omitempty"`
	DurationMs     int64                       `json:"duration_ms"`
}

// AuditVolumes — объёмы данных.
type AuditVolumes struct {
	Products             int            `json:"products"`
	Families             int            `json:"families"`
	Vendors              int            `json:"vendors"`
	Categories           int            `json:"categories"`
	Relations            map[string]int `json:"relations"`
	Aliases              int            `json:"aliases"`
	Clients              int            `json:"clients"`
	ClientsWithPurchases int            `json:"clients_with_purchases"`
	Purchases            int            `json:"purchases"`
	Events               map[string]int `json:"events"`
	EventsFrom           *time.Time     `json:"events_from,omitempty"`
	EventsTo             *time.Time     `json:"events_to,omitempty"`
	Sessions             int            `json:"sessions"`
	QueryLogRows         int            `json:"query_log_rows"`
	DistinctQueries      int            `json:"distinct_queries"`
	QueryVolume          int            `json:"query_volume"` // сумма частот
	EvalQueries          int            `json:"eval_queries"`
}

// AuditField — покрытие поля каталога.
type AuditField struct {
	Field      string  `json:"field"`
	Label      string  `json:"label"`
	Filled     int     `json:"filled"`
	Coverage   float64 `json:"coverage"`
	FromSource int     `json:"from_source"`
	Restored   int     `json:"restored"`
	Status     string  `json:"status"`
	Note       string  `json:"note,omitempty"`
}

// AuditStructure — ответы на вопросы ARCHITECTURE §9 о структуре данных.
type AuditStructure struct {
	Flat                  bool              `json:"flat"`
	AvgVariants           float64           `json:"avg_variants"`
	SingleVariantFamilies int               `json:"single_variant_families"`
	Margin                string            `json:"margin"` // source | surrogate | partial
	PriceShare            float64           `json:"price_share"`
	OnRequestShare        float64           `json:"on_request_share"`
	SessionsReal          bool              `json:"sessions_real"`
	SyntheticSessionShare float64           `json:"synthetic_session_share"`
	ClickPositionShare    float64           `json:"click_position_share"`
	HasTime               bool              `json:"has_time"`
	EventClientShare      float64           `json:"event_client_share"`
	SearchResultsKnown    float64           `json:"search_results_known"` // доля поисков журнала с results_count
	License               map[string]string `json:"license"`              // поле → source | restored | partial | absent
}

// AuditSearch — прогон частых запросов через B0 и B3.
type AuditSearch struct {
	Queries      int            `json:"queries"`
	VolumeShare  float64        `json:"volume_share"` // доля частоты, покрытая прогоном
	LogZeroShare *float64       `json:"log_zero_share,omitempty"`
	Configs      []AuditConfig  `json:"configs"`
	Routes       map[string]int `json:"routes"` // маршруты B3
	Failed       []AuditFailed  `json:"failed"`
	DurationMs   int64          `json:"duration_ms"`
	Skipped      string         `json:"skipped,omitempty"`
}

// AuditConfig — доля пустых выдач конфигурации.
type AuditConfig struct {
	Config       string  `json:"config"`
	Zero         int     `json:"zero"`
	ZeroShare    float64 `json:"zero_share"`
	ZeroWeighted float64 `json:"zero_weighted"`
	NoExact      int     `json:"no_exact"` // «точного нет» (empty), даже если показаны аналоги
	NoExactShare float64 `json:"no_exact_share"`
	Errors       int     `json:"errors"`
	AvgMs        float64 `json:"avg_ms"`
}

// AuditFailed — неудачный запрос.
type AuditFailed struct {
	Text    string `json:"text"`
	Freq    int    `json:"freq"`
	B0      int    `json:"b0"`
	B3      int    `json:"b3"`
	NoExact bool   `json:"no_exact"`
	LogZero bool   `json:"log_zero"`
}

// AuditPlanItem — пункт ARCHITECTURE §9 и что из него следует.
type AuditPlanItem struct {
	Item    string `json:"item"`
	Finding string `json:"finding"`
	Change  string `json:"change"`
}

// provenanceSource — источник, который знает, что восстановил (catalogsource.Generic).
type provenanceSource interface {
	Provenance(ctx context.Context) ([]catalogsource.FieldReport, error)
	Files(ctx context.Context) ([]catalogsource.FileInfo, error)
}

// queryLogSource — источник с журналом запросов.
type queryLogSource interface {
	QueryLog(ctx context.Context) ([]catalogsource.QueryLogRow, error)
}

const defaultAuditLimit = 300

// Audit — аудит данных текущего источника.
func (s *Services) Audit(ctx context.Context, opts AuditOptions) (*AuditReport, error) {
	if s.source == nil {
		return nil, errs.NewUnavailableError(MsgNoSource)
	}
	started := time.Now()
	snap, err := s.source.Load(ctx)
	if err != nil {
		return nil, errs.NewUnavailableError(MsgSourceBroken).WithCause(err)
	}
	r := &AuditReport{GeneratedAt: s.now().UTC(), Source: s.source.Name(), CatalogVersion: snap.Version,
		Warnings: append([]string{}, snap.Warnings...), Files: []catalogsource.FileInfo{}}
	if ps, ok := s.source.(provenanceSource); ok {
		if r.Provenance, err = ps.Provenance(ctx); err != nil {
			return nil, err
		}
		if r.Files, err = ps.Files(ctx); err != nil {
			return nil, err
		}
	}
	var events []domain.LogEvent
	if el, ok := s.source.(logLoader); ok {
		events, err = el.LoadEvents(ctx)
		if err != nil && !isNoFile(err) {
			return nil, err
		}
	}
	var qlog []catalogsource.QueryLogRow
	if ql, ok := s.source.(queryLogSource); ok {
		qlog, err = ql.QueryLog(ctx)
		if err != nil && !isNoFile(err) {
			return nil, err
		}
	}
	if loader, ok := s.source.(queryLoader); ok {
		if qs, err := loader.LoadQueries(ctx); err == nil {
			r.Volumes.EvalQueries = len(qs)
		}
	}
	auditVolumes(r, snap, events, qlog)
	auditFields(r, snap)
	auditStructure(r, snap, events)
	queries := auditQueries(events, qlog)
	r.Volumes.DistinctQueries = len(queries)
	for _, q := range queries {
		r.Volumes.QueryVolume += q.freq
	}
	if opts.NoSearch {
		r.Search = &AuditSearch{Skipped: "прогон поиска выключен (--no-search)"}
	} else if !s.search.Enabled() {
		r.Search = &AuditSearch{Skipped: "поиск не настроен (OPENSEARCH_URL)"}
	} else if len(queries) == 0 {
		r.Search = &AuditSearch{Skipped: "в данных нет запросов: ни журнала запросов, ни поисков в журнале событий"}
	} else {
		limit := opts.Limit
		if limit <= 0 {
			limit = defaultAuditLimit
		}
		if r.Search, err = s.auditSearch(ctx, queries, limit); err != nil {
			return nil, err
		}
	}
	r.Plan = auditPlan(r)
	r.P2 = s.auditP2(ctx, r, snap, events)
	r.DurationMs = time.Since(started).Milliseconds()
	return r, nil
}

func auditShare(n, total int) float64 {
	if total == 0 {
		return 0
	}
	return math.Round(float64(n)/float64(total)*1000) / 1000
}

func auditVolumes(r *AuditReport, snap *domain.CatalogSnapshot, events []domain.LogEvent, qlog []catalogsource.QueryLogRow) {
	v := &r.Volumes
	v.Products, v.Vendors, v.Categories, v.Aliases, v.Clients = len(snap.Products), len(snap.Vendors), len(snap.Categories),
		len(snap.Aliases), len(snap.Clients)
	fam := map[string]bool{}
	for _, p := range snap.Products {
		fam[p.FamilyID] = true
	}
	v.Families = len(fam)
	v.Relations = map[string]int{}
	for _, rel := range snap.Relations {
		v.Relations[rel.Type]++
	}
	for _, c := range snap.Clients {
		if len(c.Purchases) > 0 {
			v.ClientsWithPurchases++
			v.Purchases += len(c.Purchases)
		}
	}
	v.Events = map[string]int{}
	sessions := map[string]bool{}
	for i, e := range events {
		v.Events[e.Type]++
		sessions[e.SessionID] = true
		if i == 0 || e.TS.Before(*v.EventsFrom) {
			t := e.TS
			v.EventsFrom = &t
		}
		if i == 0 || e.TS.After(*v.EventsTo) {
			t := e.TS
			v.EventsTo = &t
		}
	}
	v.Sessions = len(sessions)
	v.QueryLogRows = len(qlog)
}

// auditFieldLabels — поля каталога в порядке таблицы.
var auditFieldLabels = []struct{ field, label string }{
	{"sku", "Артикул"}, {"description", "Описание"}, {"vendor", "Вендор"}, {"category", "Категория"},
	{"family", "Семейство"}, {"price", "Цена"}, {"license_type", "Тип лицензии"}, {"license_metric", "Метрика лицензии"},
	{"license_quantity", "Количество"}, {"license_duration", "Срок"}, {"license_edition", "Редакция"},
	{"attributes", "Атрибуты (характеристики)"}, {"origin", "Происхождение ru/foreign"}, {"in_reestr", "В реестре ПО"},
	{"popularity", "Популярность"}, {"margin", "Маржа"},
}

func filled(p domain.Product, field string) bool {
	switch field {
	case "sku":
		return p.SKU != ""
	case "description":
		return p.Description != ""
	case "vendor":
		return p.VendorID != ""
	case "category":
		return p.CategoryID != ""
	case "family":
		return p.FamilyID != "" && p.FamilyID != p.ID
	case "price":
		return p.PriceRub != nil
	case "license_type":
		return p.License.Type != ""
	case "license_metric":
		return p.License.Metric != nil
	case "license_quantity":
		return p.License.Quantity != nil
	case "license_duration":
		// Срок известен, если он есть или тип лицензии бессрочный.
		return p.License.DurationMonths != nil || p.License.Type == domain.LicensePerpetual
	case "license_edition":
		return p.License.Edition != ""
	case "attributes":
		return len(p.Attributes) > 2 && string(p.Attributes) != "null"
	case "origin":
		return p.Origin != ""
	case "in_reestr":
		return p.InReestr
	case "popularity":
		return p.Popularity > 0
	case "margin":
		return p.Margin > 0
	}
	return false
}

func auditFields(r *AuditReport, snap *domain.CatalogSnapshot) {
	prov := map[string]catalogsource.FieldReport{}
	for _, p := range r.Provenance {
		prov[p.Field] = p
	}
	n := len(snap.Products)
	for _, fl := range auditFieldLabels {
		f := AuditField{Field: fl.field, Label: fl.label}
		for _, p := range snap.Products {
			if filled(p, fl.field) {
				f.Filled++
			}
		}
		f.Coverage = auditShare(f.Filled, n)
		f.FromSource = f.Filled
		if pr, ok := prov[fl.field]; ok {
			// Из данных — сколько было до восстановления; остальное заполненное — правилами.
			f.FromSource = min(pr.FromSource, f.Filled)
			f.Restored = f.Filled - f.FromSource
			f.Note = pr.Note
		}
		switch {
		case fl.field == "margin" && f.Filled == 0:
			f.Status = catalogsource.FieldSurrogate
			if f.Note == "" {
				f.Note = "маржи нет в данных: суррогат 0 — вклад маржи в итоговое ранжирование одинаков у всех и на порядок не влияет"
			}
		case f.Filled == 0:
			f.Status = catalogsource.FieldAbsent
		case f.Restored == 0 && f.Filled == n:
			f.Status = catalogsource.FieldFromSource
		case f.FromSource == 0 && f.Restored > 0:
			f.Status = catalogsource.FieldRestored
		default:
			f.Status = catalogsource.FieldPartial
		}
		r.Fields = append(r.Fields, f)
	}
}

func auditStructure(r *AuditReport, snap *domain.CatalogSnapshot, events []domain.LogEvent) {
	st := &r.Structure
	n := len(snap.Products)
	for _, p := range r.Provenance {
		if p.Field == "family" && p.FromSource == 0 {
			st.Flat = true
		}
	}
	size := map[string]int{}
	priced, onReq := 0, 0
	for _, p := range snap.Products {
		size[p.FamilyID]++
		if p.PriceRub != nil {
			priced++
		}
		if p.PriceOnRequest {
			onReq++
		}
	}
	for _, c := range size {
		if c == 1 {
			st.SingleVariantFamilies++
		}
	}
	if len(size) > 0 {
		st.AvgVariants = math.Round(float64(n)/float64(len(size))*100) / 100
	}
	if len(r.Provenance) == 0 && len(size) == n {
		st.Flat = true // источник без отчёта, у каждой позиции своё семейство
	}
	st.PriceShare, st.OnRequestShare = auditShare(priced, n), auditShare(onReq, n)
	st.License = map[string]string{}
	for _, f := range r.Fields {
		switch f.Field {
		case "margin":
			st.Margin = f.Status
		case "license_type", "license_metric", "license_quantity", "license_duration", "license_edition":
			st.License[f.Field] = f.Status
		}
	}
	synthetic, clicks, withPos, withClient, searches, withCount := 0, 0, 0, 0, 0, 0
	for _, e := range events {
		if strings.HasPrefix(e.SessionID, "syn-") || strings.HasPrefix(e.SessionID, "row-") {
			synthetic++
		}
		if e.Type == "click" {
			clicks++
			if e.Position != nil {
				withPos++
			}
		}
		if e.Type == "search" {
			searches++
			if e.ResultsCount != nil {
				withCount++
			}
		}
		if e.ClientID != nil {
			withClient++
		}
	}
	st.HasTime = len(events) > 0
	st.SyntheticSessionShare = auditShare(synthetic, len(events))
	st.SessionsReal = len(events) > 0 && synthetic == 0
	st.ClickPositionShare = auditShare(withPos, clicks)
	st.EventClientShare = auditShare(withClient, len(events))
	st.SearchResultsKnown = auditShare(withCount, searches)
}

// auditQuery — запрос для прогона: текст, частота, был ли он пустым по журналу.
type auditQuery struct {
	text    string
	freq    int
	logZero bool
	logKnow bool
}

// auditQueries — запросы журнала запросов (или поисков журнала событий) по убыванию частоты.
func auditQueries(events []domain.LogEvent, qlog []catalogsource.QueryLogRow) []auditQuery {
	agg := map[string]*auditQuery{}
	add := func(text string, freq int, rc *int) {
		k := foldQuery(text)
		if k == "" {
			return
		}
		q, ok := agg[k]
		if !ok {
			q = &auditQuery{text: strings.TrimSpace(text)}
			agg[k] = q
		}
		q.freq += max(freq, 1)
		if rc != nil {
			q.logKnow = true
			q.logZero = q.logZero || *rc == 0
		}
	}
	if len(qlog) > 0 {
		for _, r := range qlog {
			add(r.Text, r.Freq, r.ResultsCount)
		}
		// Частота — из журнала запросов, «было ли пусто» — ещё и из поисков журнала событий.
		for _, e := range events {
			if q, ok := agg[foldQuery(e.Query)]; ok && e.Type == "search" && e.ResultsCount != nil {
				q.logKnow = true
				q.logZero = q.logZero || *e.ResultsCount == 0
			}
		}
	} else {
		for _, e := range events {
			if e.Type == "search" {
				add(e.Query, 1, e.ResultsCount)
			}
		}
	}
	out := make([]auditQuery, 0, len(agg))
	for _, q := range agg {
		out = append(out, *q)
	}
	sort.Slice(out, func(i, j int) bool {
		if out[i].freq != out[j].freq {
			return out[i].freq > out[j].freq
		}
		return out[i].text < out[j].text
	})
	return out
}

// auditSearch — частые запросы через B0 и B3 без журнала; LLM — как настроено в процессе (CLI выключает).
func (s *Services) auditSearch(ctx context.Context, queries []auditQuery, limit int) (*AuditSearch, error) {
	started := time.Now()
	total := 0
	for _, q := range queries {
		total += q.freq
	}
	if len(queries) > limit {
		queries = queries[:limit]
	}
	res := &AuditSearch{Queries: len(queries), Routes: map[string]int{}}
	covered, logKnown, logZero := 0, 0, 0
	for _, q := range queries {
		covered += q.freq
		if q.logKnow {
			logKnown += q.freq
			if q.logZero {
				logZero += q.freq
			}
		}
	}
	res.VolumeShare = auditShare(covered, total)
	if logKnown > 0 {
		v := auditShare(logZero, logKnown)
		res.LogZeroShare = &v
	}
	counts := map[string][]int{}
	noExact := make([]bool, len(queries))
	for _, cfg := range []string{domain.ConfigB0, domain.ConfigB3} {
		ac := AuditConfig{Config: cfg}
		var ms float64
		zeroW := 0
		counts[cfg] = make([]int, len(queries))
		for i, q := range queries {
			if err := ctx.Err(); err != nil {
				return nil, err
			}
			t := time.Now()
			resp, err := s.SearchWith(ctx, fmt.Sprintf("audit-%s-%d", cfg, i), in.SearchRequest{Query: q.text, Config: cfg, Limit: 10,
				SessionID: "audit"}, SearchOptions{NoLog: true})
			ms += float64(time.Since(t).Microseconds()) / 1000
			if err != nil {
				ac.Errors++
				counts[cfg][i] = -1
				continue
			}
			counts[cfg][i] = resp.Total
			if resp.Total == 0 {
				ac.Zero++
				zeroW += q.freq
			}
			if resp.Empty != nil || resp.Total == 0 {
				ac.NoExact++
				if cfg == domain.ConfigB3 {
					noExact[i] = true
				}
			}
			if cfg == domain.ConfigB3 && resp.Trace != nil {
				res.Routes[resp.Trace.Route]++
			}
		}
		ac.ZeroShare, ac.ZeroWeighted, ac.NoExactShare = auditShare(ac.Zero, len(queries)), auditShare(zeroW, covered), auditShare(ac.NoExact, len(queries))
		if len(queries) > 0 {
			ac.AvgMs = math.Round(ms/float64(len(queries))*10) / 10
		}
		res.Configs = append(res.Configs, ac)
	}
	for i, q := range queries {
		if counts[domain.ConfigB3][i] == 0 || noExact[i] || (q.logZero && counts[domain.ConfigB0][i] == 0) {
			res.Failed = append(res.Failed, AuditFailed{Text: q.text, Freq: q.freq, B0: counts[domain.ConfigB0][i],
				B3: counts[domain.ConfigB3][i], NoExact: noExact[i], LogZero: q.logZero})
		}
	}
	if len(res.Failed) > 25 {
		res.Failed = res.Failed[:25]
	}
	res.DurationMs = time.Since(started).Milliseconds()
	return res, nil
}

func auditPct(v float64) string { return fmt.Sprintf("%.0f%%", v*100) }

// auditPlan — «что меняется в плане» по пунктам ARCHITECTURE §9.
func auditPlan(r *AuditReport) []AuditPlanItem {
	st := r.Structure
	field := map[string]AuditField{}
	for _, f := range r.Fields {
		field[f.Field] = f
	}
	var plan []AuditPlanItem
	add := func(item, finding, change string) {
		plan = append(plan, AuditPlanItem{Item: item, Finding: finding, Change: change})
	}
	if st.Flat {
		add("Плоский каталог или семейства",
			fmt.Sprintf("каталог плоский; семейства собраны правилами по названию: %d семейств на %d позиций (в среднем %.1f, одиночных %d)",
				r.Volumes.Families, r.Volumes.Products, st.AvgVariants, st.SingleVariantFamilies),
			"группировка выдачи по семействам остаётся; глазами проверить крупнейшие группы и одиночки, при ошибках — колонка family в mapping.yaml")
	} else {
		add("Плоский каталог или семейства",
			fmt.Sprintf("семейства есть в данных: %d семейств, в среднем %.1f позиций", r.Volumes.Families, st.AvgVariants),
			"индекс по позициям с группировкой по семействам — без изменений")
	}
	switch st.Margin {
	case catalogsource.FieldSurrogate, catalogsource.FieldAbsent:
		add("Маржа", "маржи в данных нет — суррогат 0 у всех позиций",
			"вес маржи (0.05) фактически выключен; в демо говорить «маржа не передана»; запросить у организаторов или считать от цены закупки")
	case catalogsource.FieldPartial:
		add("Маржа", fmt.Sprintf("маржа у %s позиций", auditPct(field["margin"].Coverage)),
			"остальным — 0: итоговое ранжирование занижает их; рассмотреть медиану категории вместо 0")
	default:
		add("Маржа", "маржа есть у всех позиций", "вес маржи в итоговом ранжировании работает как задумано")
	}
	switch {
	case !st.HasTime:
		add("Сессии и время", "журнала событий нет", "воронка «поиск → клик → заказ» и уходы не считаются; аналитика — только по журналу запросов")
	case st.SessionsReal:
		add("Сессии и время", fmt.Sprintf("сессии в данных, период %s — %s", r.Volumes.EventsFrom.Format("02.01.2006"),
			r.Volumes.EventsTo.Format("02.01.2006")), "атрибуция действий к поиску, переформулировки за 60 с и holdout-неделя агента — без изменений")
	default:
		add("Сессии и время", fmt.Sprintf("сессий нет у %s событий — синтезированы (клиент + час)", auditPct(st.SyntheticSessionShare)),
			"переформулировки и уходы после поиска — оценка снизу; в аналитике пометить «приблизительно»")
	}
	if r.Volumes.Events["click"] == 0 {
		add("Позиции показов", "кликов в журнале нет", "неявная разметка — только по заказам и корзинам; CTR по позициям не строится")
	} else if st.ClickPositionShare < 0.5 {
		add("Позиции показов", fmt.Sprintf("позиция есть у %s кликов", auditPct(st.ClickPositionShare)),
			"неявная разметка без поправки на позицию; MRR по журналу не считать")
	} else {
		add("Позиции показов", fmt.Sprintf("позиция есть у %s кликов", auditPct(st.ClickPositionShare)), "поправка на позицию в неявной разметке включается")
	}
	var lic []string
	for _, f := range []string{"license_type", "license_metric", "license_quantity", "license_duration"} {
		fl := field[f]
		lic = append(lic, fmt.Sprintf("%s %s (из данных %d, правилами %d)", fl.Label, auditPct(fl.Coverage), fl.FromSource, fl.Restored))
	}
	licChange := "фильтры количества, метрики и срока работают на данных"
	if field["license_quantity"].Restored > 0 || field["license_type"].Restored > 0 {
		licChange = "фильтры лицензий работают на восстановленных правилами значениях — проверить выборку и добавить шаблоны названий, если покрытие ниже 80%"
	}
	if field["license_quantity"].Coverage < 0.3 {
		licChange = "условий лицензии почти нет — строгие чипы количества и срока в B3 не сработают; показывать их мягкими"
	}
	add("Атрибуты лицензий", strings.Join(lic, "; "), licChange)
	add("Цены и клиенты", fmt.Sprintf("цена у %s позиций, «по запросу» у %s; клиентов %d, с покупками %d; клиент в событиях у %s",
		auditPct(st.PriceShare), auditPct(st.OnRequestShare), r.Volumes.Clients, r.Volumes.ClientsWithPurchases, auditPct(st.EventClientShare)),
		"персонализация по истории покупок работает только для клиентов с покупками; гость — без контекста")
	if s := r.Search; s != nil && s.Skipped == "" && len(s.Configs) == 2 {
		b0, b3 := s.Configs[0], s.Configs[1]
		finding := fmt.Sprintf("без результата: B0 %s (по частоте %s), B3 %s (по частоте %s) на %d частых запросах (%s объёма)",
			auditPct(b0.ZeroShare), auditPct(b0.ZeroWeighted), auditPct(b3.ZeroShare), auditPct(b3.ZeroWeighted), s.Queries, auditPct(s.VolumeShare))
		if s.LogZeroShare != nil {
			finding += fmt.Sprintf("; по журналу магазина — %s", auditPct(*s.LogZeroShare))
		}
		add("Доля запросов без результата", finding, "это базовая точка «до/после» для питча; неудачные запросы B3 — первые кандидаты агенту")
		d := s.Routes["D"]
		add("Доля запросов, которым нужна LLM", fmt.Sprintf("маршрут D у %d из %d запросов B3 (%s)", d, s.Queries, auditPct(auditShare(d, s.Queries))),
			"пересчитать стоимость запроса в «Экономике» с этой долей")
	}
	return plan
}
