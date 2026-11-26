package services

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"slices"
	"sort"
	"strings"
	"time"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/externalApi/llm"
	"kuznica/api/in"
)

// Инструменты агента (ARCHITECTURE §3.3). Все детерминированы; аргументы — типизированные структуры,
// незнакомые поля и неверные значения — отказ с причиной (модель видит его и исправляется). Нет SQL,
// shell и записи в боевой индекс: каталог читается из снимка, поиск — через сервис без журнала, правка
// живёт только в песочнице до решения человека.

// agentTools — описания инструментов для модели (JSON Schema параметров).
func agentTools() []llm.Tool {
	def := func(name, desc, params string) llm.Tool {
		return llm.Tool{Type: "function", Function: llm.ToolFunction{Name: name, Description: desc, Parameters: json.RawMessage(params)}}
	}
	return []llm.Tool{
		def("get_failed_queries", "Проблемные запросы периода: частота и неудачи (пусто, переформулировка, нет клика).",
			`{"type":"object","properties":{"limit":{"type":"integer","minimum":1,"maximum":30}},"required":[]}`),
		def("get_query_cluster", "Кластер похожих неудачных запросов: запросы, во что их переформулировали, ответ действующего поиска.",
			`{"type":"object","properties":{"cluster_id":{"type":"string"}},"required":["cluster_id"]}`),
		def("search_catalog", "Поиск по каталогу в обход боевого ранжирования. expand=true — расширенный: подстроки, транслит, смысловая близость.",
			`{"type":"object","properties":{"query":{"type":"string"},"expand":{"type":"boolean"},"limit":{"type":"integer","minimum":1,"maximum":15}},"required":["query"]}`),
		def("get_active_rules", "Действующие алиасы словаря: поиск по слову или по id цели.",
			`{"type":"object","properties":{"term":{"type":"string"},"target_id":{"type":"string"}},"required":[]}`),
		def("get_search_trace", "Как действующий поиск обрабатывает запрос: нормализация, маршрут, выдача, «нет в каталоге».",
			`{"type":"object","properties":{"query":{"type":"string"}},"required":["query"]}`),
		def("find_similar_successful_queries", "Похожие запросы с кликами: переформулировки этого кластера и близкие по смыслу.",
			`{"type":"object","properties":{"query":{"type":"string"},"limit":{"type":"integer","minimum":1,"maximum":10}},"required":["query"]}`),
		def("get_product_details", "Карточка семейства или позиции: производитель, категория, описание, лицензии.",
			`{"type":"object","properties":{"family_id":{"type":"string"},"product_id":{"type":"string"}},"required":[]}`),
		def("get_product_relations", "Связи семейства: аналоги (substitute), сопутствующие, продления, совместимость.",
			`{"type":"object","properties":{"family_id":{"type":"string"}},"required":["family_id"]}`),
		def("get_previous_decisions", "Память: прошлые предложения и решения человека по кластеру или термину.",
			`{"type":"object","properties":{"cluster_id":{"type":"string"},"term":{"type":"string"}},"required":[]}`),
		def("create_sandbox_patch", "Гипотеза: правка словаря в песочнице (односторонние алиасы). Боевые правила не меняются.",
			`{"type":"object","properties":{"diagnosis":{"type":"string","enum":["ALIAS_GAP","TYPO"]},"hypothesis":{"type":"string"},`+
				`"add":{"type":"array","minItems":1,"maxItems":3,"items":{"type":"object","properties":{"alias":{"type":"string"},"canonical":{"type":"string"},`+
				`"target":{"type":"string","enum":["vendor","family","category","term"]},"target_id":{"type":"string"},"term_vendor":{"type":"string"},`+
				`"alias_kind":{"type":"string","enum":["slang","abbreviation","transliteration","alias","typo"]}},"required":["alias","canonical","target"]}},`+
				`"remove":{"type":"array","items":{"type":"object","properties":{"alias":{"type":"string"}},"required":["alias"]}}},"required":["diagnosis","hypothesis","add"]}`),
		def("run_evaluation", "Прогон правки из песочницы: целевой кластер до/после и регрессионный набор. Возвращает passed и причины.",
			`{"type":"object","properties":{"patch_id":{"type":"string"}},"required":["patch_id"]}`),
		def("submit_proposal", "Отправить прошедшую прогон правку человеку в очередь.",
			`{"type":"object","properties":{"patch_id":{"type":"string"},"diagnosis_text":{"type":"string"}},"required":["patch_id","diagnosis_text"]}`),
		def("create_assortment_ticket", "Задача человеку: SUBSTITUTE_AVAILABLE (есть аналог), ASSORTMENT_GAP (спрос без предложения), CONTENT_GAP (правка карточки), RANKING_FAILURE (товар в выдаче, но не первым — менеджеру e-com).",
			`{"type":"object","properties":{"diagnosis":{"type":"string","enum":["SUBSTITUTE_AVAILABLE","ASSORTMENT_GAP","CONTENT_GAP","RANKING_FAILURE"]},"text":{"type":"string"},`+
				`"substitute_family_ids":{"type":"array","items":{"type":"string"},"maxItems":5},"family_id":{"type":"string"}},"required":["diagnosis","text"]}`),
		def("close_cluster", "Закрыть кластер без предложения и без задачи (NOISE, NON_PRODUCT, CONSTRAINT_CONFLICT, UNKNOWN или ALIAS_GAP/TYPO после трёх отклонённых гипотез). Спрос без предложения, аналог, карточка и ранжирование — create_assortment_ticket.",
			`{"type":"object","properties":{"diagnosis":{"type":"string","enum":["ALIAS_GAP","TYPO","CONSTRAINT_CONFLICT","NON_PRODUCT","NOISE","UNKNOWN"]},"reason":{"type":"string"}},"required":["diagnosis","reason"]}`),
		def("verify_applied_change", "Эффект принятой ранее правки: сколько запросов её кластера находится сейчас.",
			`{"type":"object","properties":{"proposal_id":{"type":"string"}},"required":["proposal_id"]}`),
	}
}

// toolTitles — подпись инструмента в ленте.
var toolTitles = map[string]string{
	"get_failed_queries": "Проблемные запросы", "get_query_cluster": "Кластер запросов", "search_catalog": "Поиск по каталогу",
	"get_active_rules": "Действующие правила", "get_search_trace": "Трасса поиска", "find_similar_successful_queries": "Похожие успешные запросы",
	"get_product_details": "Карточка товара", "get_product_relations": "Связи товара", "get_previous_decisions": "Память",
	"create_sandbox_patch": "Правка в песочнице", "run_evaluation": "Прогон", "submit_proposal": "Предложение в очередь",
	"create_assortment_ticket": "Задача человеку", "close_cluster": "Кластер закрыт", "verify_applied_change": "Эффект правки",
}

// toolError — ошибка инструмента для модели.
type toolError struct{ msg string }

func (e *toolError) Error() string { return e.msg }

func terr(format string, a ...any) error { return &toolError{msg: fmt.Sprintf(format, a...)} }

// decodeArgs — аргументы в структуру: незнакомые поля и не JSON — отказ.
func decodeArgs(raw string, dst any) error {
	if strings.TrimSpace(raw) == "" {
		raw = "{}"
	}
	dec := json.NewDecoder(bytes.NewReader([]byte(raw)))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dst); err != nil {
		return terr("аргументы не по схеме: %v", err)
	}
	return nil
}

func jsonUnmarshal(b []byte, dst any) error {
	if len(b) == 0 {
		return errors.New("пусто")
	}
	return json.Unmarshal(b, dst)
}

// call — один вызов инструмента: проверка, выполнение, шаг ленты; ответ модели — JSON.
func (cr *clusterRun) call(ctx context.Context, call llm.ToolCall) string {
	a, rs := cr.rc.a, cr.rc.rs
	name := call.Function.Name
	cr.toolCalls++
	rs.mu.Lock()
	rs.run.ToolCalls++
	rs.mu.Unlock()
	started := time.Now()
	res, title, err := cr.dispatch(ctx, name, call.Function.Arguments)
	id := cr.c.ID
	tool := name
	st := &domain.AgentStep{Kind: domain.StepToolCall, ClusterID: &id, Tool: &tool, OK: err == nil,
		Ms: int(time.Since(started).Milliseconds())}
	var args any = json.RawMessage(call.Function.Arguments)
	if !json.Valid([]byte(call.Function.Arguments)) {
		args = map[string]any{"raw": shortText(call.Function.Arguments, 300)}
	}
	st.Args = toJSON(args)
	label := toolTitles[name]
	if label == "" {
		label = name
	}
	if err != nil {
		var te *toolError
		msg := "внутренняя ошибка инструмента"
		if errors.As(err, &te) {
			msg = te.msg
		}
		res = map[string]any{"error": msg}
		st.Title = label + ": отказ — " + shortText(msg, 160)
	} else {
		st.Title = label + ": " + title
	}
	st.Result = toJSON(res)
	// Шаги диагноза, гипотезы, проверки и предложения пишет сам инструмент; вызов — отдельной строкой.
	a.step(ctx, rs, st)
	for _, ps := range cr.pending {
		a.step(ctx, rs, ps)
	}
	cr.pending = nil
	return string(toJSON(res))
}

// dispatch — выполнение инструмента по имени.
func (cr *clusterRun) dispatch(ctx context.Context, name, raw string) (any, string, error) {
	switch name {
	case "get_failed_queries":
		return cr.toolFailedQueries(raw)
	case "get_query_cluster":
		return cr.toolQueryCluster(raw)
	case "search_catalog":
		return cr.toolSearchCatalog(ctx, raw)
	case "get_active_rules":
		return cr.toolActiveRules(raw)
	case "get_search_trace":
		return cr.toolSearchTrace(ctx, raw)
	case "find_similar_successful_queries":
		return cr.toolSimilarSuccessful(ctx, raw)
	case "get_product_details":
		return cr.toolProductDetails(raw)
	case "get_product_relations":
		return cr.toolProductRelations(raw)
	case "get_previous_decisions":
		return cr.toolPreviousDecisions(raw)
	case "create_sandbox_patch":
		return cr.toolCreatePatch(ctx, raw)
	case "run_evaluation":
		return cr.toolRunEvaluation(ctx, raw)
	case "submit_proposal":
		return cr.toolSubmit(ctx, raw)
	case "create_assortment_ticket":
		return cr.toolTicket(ctx, raw)
	case "close_cluster":
		return cr.toolClose(ctx, raw)
	case "verify_applied_change":
		return cr.toolVerify(ctx, raw)
	}
	return nil, "", terr("инструмента %q нет", name)
}

// noteNotFirst — семейство стоит в выдаче запроса кластера, но не первым (доказательство RANKING_FAILURE).
func (cr *clusterRun) noteNotFirst(familyID, query string) {
	if familyID == "" {
		return
	}
	if _, ok := cr.notFirst[familyID]; !ok {
		cr.notFirst[familyID] = query
	}
}

// checkText — строковый аргумент: обязательный, не длиннее n символов.
func checkText(field, v string, n int) (string, error) {
	v = strings.TrimSpace(v)
	if v == "" {
		return "", terr("поле %s пустое", field)
	}
	if utf8.RuneCountInString(v) > n {
		return "", terr("поле %s длиннее %d символов", field, n)
	}
	return v, nil
}

func (cr *clusterRun) toolFailedQueries(raw string) (any, string, error) {
	var args struct {
		Limit int `json:"limit"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	if args.Limit <= 0 || args.Limit > 30 {
		args.Limit = 15
	}
	qs := cr.rc.failed
	if len(qs) > args.Limit {
		qs = qs[:args.Limit]
	}
	return map[string]any{"queries": compactQueries(qs), "total_failed": len(cr.rc.failed)},
		fmt.Sprintf("%d запросов из %d неудачных", len(qs), len(cr.rc.failed)), nil
}

func (cr *clusterRun) toolQueryCluster(raw string) (any, string, error) {
	var args struct {
		ClusterID string `json:"cluster_id"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	c := cr.rc.byID[strings.TrimSpace(args.ClusterID)]
	if c == nil {
		return nil, "", terr("кластера %q в этом запуске нет", args.ClusterID)
	}
	return cr.rc.clusterBrief(c, nonNil(cr.rc.memoryFor(c.Key, c.Terms))), fmt.Sprintf("«%s», %d запросов", c.Label, len(c.Queries)), nil
}

func (cr *clusterRun) toolSearchCatalog(ctx context.Context, raw string) (any, string, error) {
	var args struct {
		Query  string `json:"query"`
		Expand bool   `json:"expand"`
		Limit  int    `json:"limit"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	q, err := checkText("query", args.Query, 100)
	if err != nil {
		return nil, "", err
	}
	if args.Limit <= 0 || args.Limit > 15 {
		args.Limit = 8
	}
	hits := cr.rc.cat.search(ctx, cr.rc.a.s, q, args.Expand, args.Limit)
	cr.searched = true
	if args.Expand {
		cr.expanded = true
		cr.expandedFound = cr.expandedFound || len(hits) > 0
	}
	mode := "обычный"
	if args.Expand {
		mode = "расширенный"
	}
	title := fmt.Sprintf("«%s» (%s) — найдено семейств: %d", shortText(q, 40), mode, len(hits))
	if len(hits) > 0 {
		title += ", первое — " + hits[0].Name
	}
	return map[string]any{"query": q, "expand": args.Expand, "families": nonNil(hits)}, title, nil
}

func (cr *clusterRun) toolActiveRules(raw string) (any, string, error) {
	var args struct {
		Term     string `json:"term"`
		TargetID string `json:"target_id"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	term, target := foldText(strings.TrimSpace(args.Term)), strings.TrimSpace(args.TargetID)
	var hits []ruleAlias
	for _, r := range cr.rc.active.Aliases {
		if term != "" && (strings.Contains(foldText(r.Alias), term) || strings.Contains(foldText(r.Canonical), term)) ||
			target != "" && r.TargetID == target {
			hits = append(hits, r)
		}
		if len(hits) >= 30 {
			break
		}
	}
	return map[string]any{"rules_version": cr.rc.active.Version, "aliases_total": len(cr.rc.active.Aliases), "matches": nonNil(hits)},
		fmt.Sprintf("версия %d, алиасов %d, совпало %d", cr.rc.active.Version, len(cr.rc.active.Aliases), len(hits)), nil
}

func (cr *clusterRun) toolSearchTrace(ctx context.Context, raw string) (any, string, error) {
	var args struct {
		Query string `json:"query"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	q, err := checkText("query", args.Query, 120)
	if err != nil {
		return nil, "", err
	}
	resp, err := cr.rc.a.s.SearchWith(ctx, agentRequestID(), in.SearchRequest{Query: q, Config: domain.ConfigB4, Limit: 5, SessionID: "agent"},
		SearchOptions{Rules: cr.rc.active.snapshot(), NoLog: true})
	if err != nil {
		return nil, "", terr("поиск не ответил: %v", err)
	}
	res := map[string]any{"query": q, "normalized": resp.Normalized, "total": resp.Total}
	if resp.Correction != nil {
		res["correction"] = resp.Correction
	}
	if resp.Empty != nil {
		res["empty"] = resp.Empty.Message
	}
	var items []map[string]any
	for i, it := range resp.Items {
		items = append(items, map[string]any{"family_id": it.FamilyID, "title": it.Title, "vendor": it.Vendor, "group": it.Group})
		if i > 0 && it.FamilyID != resp.Items[0].FamilyID {
			cr.noteNotFirst(it.FamilyID, q)
		}
	}
	res["items"] = nonNil(items)
	if tr := resp.Trace; tr != nil {
		res["route"], res["parse_source"] = tr.Route, tr.ParseSource
		var steps []map[string]any
		_ = json.Unmarshal(toJSON(tr.Steps), &steps)
		for _, s := range steps {
			switch s["name"] {
			case "normalize", "route", "attributes", "intent":
				delete(s, "ms")
				res["step_"+fmt.Sprint(s["name"])] = s
			}
		}
		if len(tr.Rejected) > 0 {
			rej := tr.Rejected
			if len(rej) > 5 {
				rej = rej[:5]
			}
			res["rejected"] = rej
		}
	}
	title := fmt.Sprintf("«%s»: маршрут %v, результатов %d", shortText(q, 40), res["route"], resp.Total)
	if resp.Empty != nil {
		title += ", «нет в каталоге»"
	} else if len(resp.Items) > 0 {
		title += fmt.Sprintf(", первым — %s (%s)", resp.Items[0].Title, resp.Items[0].Group)
	}
	return res, title, nil
}

func (cr *clusterRun) toolSimilarSuccessful(ctx context.Context, raw string) (any, string, error) {
	var args struct {
		Query string `json:"query"`
		Limit int    `json:"limit"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	q, err := checkText("query", args.Query, 120)
	if err != nil {
		return nil, "", err
	}
	if args.Limit <= 0 || args.Limit > 10 {
		args.Limit = 6
	}
	type hit struct {
		Text       string  `json:"text"`
		Freq       int     `json:"freq"`
		ClickRate  float64 `json:"click_rate"`
		Family     string  `json:"clicked_family"`
		Source     string  `json:"source"`
		Similarity float64 `json:"similarity,omitempty"`
	}
	seen := map[string]bool{}
	var hits []hit
	// Переформулировки запросов кластера, после которых покупатель кликнул, — прямое доказательство.
	for _, cq := range cr.c.Queries {
		for _, r := range cq.ReformulatedTo {
			if r.Clicked == 0 || seen[r.Text] {
				continue
			}
			seen[r.Text] = true
			h := hit{Text: r.Text, Source: "reformulation", Family: cr.rc.cat.familyOfProduct(r.ClickedOn)}
			if s := cr.rc.stats.byText[r.Text]; s != nil {
				h.Freq, h.ClickRate = s.Freq, evalRound4(float64(s.Clicks)/float64(max(1, s.Freq)))
			}
			hits = append(hits, h)
		}
	}
	succ, vecs := cr.rc.successful(ctx)
	if qv := cr.rc.a.s.embedTexts(ctx, []string{q}); len(qv) == 1 && len(vecs) == len(succ) {
		type sc struct {
			i int
			c float64
		}
		var scored []sc
		for i, v := range vecs {
			scored = append(scored, sc{i, cosine(qv[0], v)})
		}
		sort.Slice(scored, func(i, j int) bool { return scored[i].c > scored[j].c })
		for _, s := range scored {
			if len(hits) >= args.Limit || s.c < 0.5 {
				break
			}
			sq := succ[s.i]
			if seen[sq.Text] {
				continue
			}
			seen[sq.Text] = true
			hits = append(hits, hit{Text: sq.Text, Freq: sq.Freq, ClickRate: evalRound4(float64(sq.Clicks) / float64(max(1, sq.Freq))),
				Family: cr.rc.topClickedFamily(sq.Text), Source: "similar", Similarity: evalRound4(s.c)})
		}
	}
	if len(hits) > args.Limit {
		hits = hits[:args.Limit]
	}
	for _, h := range hits {
		if h.Family != "" {
			cr.foundBy[h.Family] = true
		}
	}
	title := fmt.Sprintf("найдено %d", len(hits))
	if len(hits) > 0 {
		title += fmt.Sprintf(", например «%s» → %s", shortText(hits[0].Text, 40), hits[0].Family)
	}
	return map[string]any{"query": q, "queries": nonNil(hits)}, title, nil
}

func (cr *clusterRun) toolProductDetails(raw string) (any, string, error) {
	var args struct {
		FamilyID  string `json:"family_id"`
		ProductID string `json:"product_id"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	fid := strings.TrimSpace(args.FamilyID)
	if fid == "" && args.ProductID != "" {
		fid = cr.rc.cat.familyOfProduct(strings.TrimSpace(args.ProductID))
	}
	f := cr.rc.cat.families[fid]
	if f == nil {
		return nil, "", terr("семейства или позиции %q нет в каталоге", firstNonEmpty(args.FamilyID, args.ProductID))
	}
	cr.evidence["checked_family"] = fid
	return cr.rc.cat.details(f), f.Name + " (" + cr.rc.cat.vendorName(f.VendorID) + ")", nil
}

func (cr *clusterRun) toolProductRelations(raw string) (any, string, error) {
	var args struct {
		FamilyID string `json:"family_id"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	f := cr.rc.cat.families[strings.TrimSpace(args.FamilyID)]
	if f == nil {
		return nil, "", terr("семейства %q нет в каталоге", args.FamilyID)
	}
	rels := cr.rc.cat.relationsOf(f.ID)
	return map[string]any{"family_id": f.ID, "relations": nonNil(rels)}, fmt.Sprintf("%s: связей %d", f.Name, len(rels)), nil
}

func (cr *clusterRun) toolPreviousDecisions(raw string) (any, string, error) {
	var args struct {
		ClusterID string `json:"cluster_id"`
		Term      string `json:"term"`
	}
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	key, terms := cr.c.Key, cr.c.Terms
	if c := cr.rc.byID[strings.TrimSpace(args.ClusterID)]; c != nil {
		key, terms = c.Key, c.Terms
	}
	if t := foldText(strings.TrimSpace(args.Term)); t != "" {
		key, terms = t, []string{t}
	}
	hits := cr.rc.memoryFor(key, terms)
	return map[string]any{"items": nonNil(hits)}, fmt.Sprintf("решений найдено: %d", len(hits)), nil
}

func firstNonEmpty(vs ...string) string {
	for _, v := range vs {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}

// patchArgs — аргументы create_sandbox_patch.
type patchArgs struct {
	Diagnosis  string `json:"diagnosis"`
	Hypothesis string `json:"hypothesis"`
	Add        []struct {
		Alias      string `json:"alias"`
		Canonical  string `json:"canonical"`
		Target     string `json:"target"`
		TargetID   string `json:"target_id"`
		TermVendor string `json:"term_vendor"`
		AliasKind  string `json:"alias_kind"`
	} `json:"add"`
	Remove []struct {
		Alias string `json:"alias"`
	} `json:"remove"`
}

var aliasKinds = []string{"", "slang", "abbreviation", "transliteration", "alias", "typo"}

func (cr *clusterRun) toolCreatePatch(ctx context.Context, raw string) (any, string, error) {
	var args patchArgs
	if err := decodeArgs(raw, &args); err != nil {
		return nil, "", err
	}
	if args.Diagnosis != domain.DiagAliasGap && args.Diagnosis != domain.DiagTypo {
		return nil, "", terr("правка словаря — только для ALIAS_GAP или TYPO; для аналогов и пробелов ассортимента — create_assortment_ticket")
	}
	hyp, err := checkText("hypothesis", args.Hypothesis, 300)
	if err != nil {
		return nil, "", err
	}
	if cr.hypotheses >= agentMaxHypotheses {
		return nil, "", terr("лимит гипотез (%d) исчерпан: закрой кластер (close_cluster) или оформи задачу", agentMaxHypotheses)
	}
	if len(args.Add) == 0 || len(args.Add) > 3 {
		return nil, "", terr("в правке от 1 до 3 алиасов")
	}
	cat, active := cr.rc.cat, cr.rc.active
	var patch RulePatch
	var vendorProblems []string
	for _, a := range args.Add {
		alias, err := checkText("alias", strings.ToLower(a.Alias), 40)
		if err != nil {
			return nil, "", err
		}
		if utf8.RuneCountInString(alias) < 2 || len(strings.Fields(alias)) > 4 {
			return nil, "", terr("алиас «%s»: от 2 символов и не больше 4 слов", alias)
		}
		canonical, err := checkText("canonical", a.Canonical, 80)
		if err != nil {
			return nil, "", err
		}
		if !slices.Contains(aliasKinds, a.AliasKind) {
			return nil, "", terr("alias_kind — slang, abbreviation, transliteration, alias или typo")
		}
		// Цель и производитель цели — из каталога; canonical обязан называть цель (поиск подставляет
		// именно canonical, а не target_id).
		var targetVendor string
		switch a.Target {
		case domain.AliasTargetVendor, domain.AliasTargetFamily, domain.AliasTargetCategory:
			v, err := cat.checkCanonical(a.Target, a.TargetID, canonical)
			if err != nil {
				return nil, "", err
			}
			targetVendor = v
		case domain.AliasTargetTerm:
			if a.TargetID != "" {
				return nil, "", terr("для target=term поле target_id должно быть пустым")
			}
			if missing := cat.unknownWords(canonical); len(missing) > 0 {
				return nil, "", terr("канонической формы «%s» нет в каталоге (нет слов: %s) — алиас вёл бы в никуда", canonical, strings.Join(missing, ", "))
			}
			targetVendor = cat.termVendor(canonical)
		default:
			return nil, "", terr("target — vendor, family, category или term")
		}
		if ex := active.find(alias); ex != nil {
			if aliasKey(ex.Canonical) == aliasKey(canonical) {
				return nil, "", terr("алиас «%s» → «%s» уже действует", alias, ex.Canonical)
			}
			return nil, "", terr("алиас «%s» уже ведёт в «%s»; чтобы заменить, добавь его в remove", alias, ex.Canonical)
		}
		// Синонимы между разными производителями запрещены (аналог ≠ синоним). Чей продукт называют словом,
		// определяет код по данным — производитель в самом слове, клики после переформулировок запросов
		// кластера, поиск слова по каталогу; term_vendor модели может только добавить отказ, но не разрешить.
		if targetVendor != "" {
			evidence := cr.termVendors(ctx, alias)
			termVendor, known := cat.resolveVendor(a.TermVendor)
			aliasVendor := cat.vendorOfText(alias)
			switch {
			case aliasVendor != "" && aliasVendor != targetVendor:
				vendorProblems = append(vendorProblems, fmt.Sprintf("слово «%s» называет производителя %s, а цель — %s: синоним между разными производителями",
					alias, cat.vendorName(aliasVendor), cat.vendorName(targetVendor)))
			case len(evidence) == 0:
				vendorProblems = append(vendorProblems, fmt.Sprintf("нет данных, что «%s» — продукт %s: ни кликов после переформулировок, ни совпадения в каталоге "+
					"(слова модели не доказательство) — синоним между разными производителями не исключён", alias, cat.vendorName(targetVendor)))
			case evidence[targetVendor] == "":
				vendorProblems = append(vendorProblems, fmt.Sprintf("«%s» по данным — продукт %s, а цель — %s: синоним между разными производителями",
					alias, cat.vendorNames(evidence), cat.vendorName(targetVendor)))
			case strings.TrimSpace(a.TermVendor) != "" && known && termVendor != targetVendor:
				vendorProblems = append(vendorProblems, fmt.Sprintf("«%s» — продукт %s, а цель — %s: синоним между разными производителями",
					alias, cat.vendorName(termVendor), cat.vendorName(targetVendor)))
			}
			cr.evidence["term_vendor"] = map[string]any{"alias": alias, "target_vendor": targetVendor, "evidence": evidence}
		}
		patch.Add = append(patch.Add, PatchAlias{Kind: "alias", Alias: alias, Canonical: canonical, Target: a.Target,
			TargetID: a.TargetID, AliasKind: a.AliasKind})
	}
	for _, r := range args.Remove {
		if active.find(r.Alias) == nil {
			return nil, "", terr("алиаса «%s» в действующих правилах нет", r.Alias)
		}
		patch.Remove = append(patch.Remove, PatchAlias{Kind: "alias", Alias: strings.ToLower(strings.TrimSpace(r.Alias))})
	}
	for _, p := range cr.patches {
		if slices.Equal(patchKeys(p.Patch), patchKeys(patch)) {
			return nil, "", terr("такая правка уже проверялась в гипотезе %d", p.No)
		}
	}

	cr.hypotheses++
	sp := &sandboxPatch{ID: fmt.Sprintf("p%d", cr.hypotheses), No: cr.hypotheses, Diagnosis: args.Diagnosis, Hypothesis: hyp, Patch: patch}
	cr.patches[sp.ID] = sp
	id := cr.c.ID
	if cr.diagnosis != args.Diagnosis {
		cr.diagnosis = args.Diagnosis
		cr.emit(&domain.AgentStep{Kind: domain.StepDiagnosis, ClusterID: &id, OK: true,
			Title:  fmt.Sprintf("Диагноз: %s — %s", args.Diagnosis, diagnosisLabels[args.Diagnosis]),
			Result: toJSON(map[string]any{"diagnosis": args.Diagnosis, "reason": hyp})})
	}
	no := sp.No
	sp.Step = &domain.AgentStep{Kind: domain.StepHypothesis, ClusterID: &id, OK: true, HypothesisNo: &no,
		Title: fmt.Sprintf("Гипотеза %d: %s", no, patchLine(patch)), Args: toJSON(patch),
		Result: toJSON(map[string]any{"patch_id": sp.ID, "hypothesis": hyp})}
	cr.emit(sp.Step)

	// Проверки кодом до прогона: память человека и запрет синонимов между производителями.
	rejectedKeys := cr.rc.rejectedPatchKeys()
	for _, k := range patchKeys(patch) {
		if h, ok := rejectedKeys[k]; ok {
			reason := fmt.Sprintf("человек уже отклонил такую правку %s (причина: %s%s)", h.At.Format("02.01"), rejectLabels[h.Reason], commentSuffix(h.Comment))
			cr.rejectHypothesis(ctx, sp, reason, map[string]any{"memory": h})
			return map[string]any{"patch_id": sp.ID, "hypothesis_no": no, "rejected": true, "reason": reason,
				"hypotheses_left": agentMaxHypotheses - cr.hypotheses}, "отклонена памятью: " + reason, nil
		}
	}
	if len(vendorProblems) > 0 {
		reason := "синоним между разными производителями запрещён: " + strings.Join(vendorProblems, "; ")
		cr.rejectHypothesis(ctx, sp, reason, map[string]any{"rule": "cross_vendor"})
		return map[string]any{"patch_id": sp.ID, "hypothesis_no": no, "rejected": true, "reason": reason,
			"hint":            "если товара нет, а есть аналог — create_assortment_ticket с SUBSTITUTE_AVAILABLE",
			"hypotheses_left": agentMaxHypotheses - cr.hypotheses}, "отклонена кодом: " + reason, nil
	}
	return map[string]any{"patch_id": sp.ID, "hypothesis_no": no, "patch": patch, "next": "run_evaluation"},
		fmt.Sprintf("гипотеза %d (%s) готова к прогону", no, sp.ID), nil
}

// diagnosisLabels — диагнозы словами.
var diagnosisLabels = map[string]string{
	domain.DiagAliasGap:           "неизвестный термин: сленг, сокращение или транслит существующего товара",
	domain.DiagTypo:               "опечатка, которую поиск не исправил",
	domain.DiagAssortmentGap:      "товара нет и аналога нет — спрос без предложения",
	domain.DiagSubstitute:         "товара нет, но есть аналог",
	domain.DiagContentGap:         "в карточке нет слов, которыми ищут",
	domain.DiagRankingFailure:     "товар найден, но стоит низко",
	domain.DiagConstraintConflict: "такой лицензии не существует",
	domain.DiagNonProduct:         "запрос не о товаре",
	domain.DiagNoise:              "мусор",
	domain.DiagUnknown:            "недостаточно доказательств",
}

// patchLine — правка одной строкой.
func patchLine(p RulePatch) string {
	var parts []string
	for _, a := range p.Add {
		t := a.Target
		if a.TargetID != "" {
			t += ":" + a.TargetID
		}
		parts = append(parts, fmt.Sprintf("алиас «%s» → «%s» (%s)", a.Alias, a.Canonical, t))
	}
	for _, a := range p.Remove {
		parts = append(parts, fmt.Sprintf("убрать «%s»", a.Alias))
	}
	return strings.Join(parts, ", ")
}
