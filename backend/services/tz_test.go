package services

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"regexp"
	"strconv"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kuznica/api/externalApi/llm"
	"kuznica/api/externalApi/ocr"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
)

// Тестовое ТЗ контура ТЗ: одно и то же в DOCX, PDF с текстом и скане (TZ-CORE §9).
const tzDataDir = "../../data/softline/eval/"

func tzFile(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(tzDataDir + name)
	if err != nil {
		t.Skipf("нет тестового файла %s: %v", name, err)
	}
	return data
}

// tzRows — строки таблицы спецификации «N | …» из текста.
func tzRowsOf(text string) []string {
	var rows []string
	for _, l := range strings.Split(text, "\n") {
		if regexp.MustCompile(`^\d+ \| `).MatchString(l) {
			rows = append(rows, l)
		}
	}
	return rows
}

var tzWantRows = []string{
	"1 | Kaspersky Endpoint Security для бизнеса – Стандартный, новая лицензия на 1 год | узел | 50",
	"2 | ОС Астра Линукс Special Edition, уровень защищённости «Орёл», лицензия для рабочей станции на 12 месяцев | шт. | 50",
	"3 | Программное обеспечение резервного копирования и восстановления данных (или эквивалент), лицензия на 12 месяцев | комплект | 1",
	"4 | СКЗИ КриптоПро CSP версии 5.0, серверная лицензия, бессрочная | шт. | 2",
	"5 | Облачный офисный пакет для совместной работы (почта, диск, редакторы документов), подписка на 12 месяцев | пользователь | 50",
	"6 | Межсетевой экран нового поколения (NGFW), программный, с подпиской на обновления на 1 год | шт. | 1",
	"7 | Сервер стоечный 2U | шт. | 2",
	"8 | Работы по монтажу и пусконаладке серверного оборудования | усл. ед. | 1",
}

// DOCX и PDF дают одну и ту же таблицу: строка позиции целиком, количество не оторвано от названия (TZ-CORE §3).
func TestTZTextDocxAndPDFSameTable(t *testing.T) {
	for _, name := range []string{"TZ_Severny_veter.docx", "TZ_Severny_veter.pdf"} {
		data := tzFile(t, name)
		format := tzSniff(name, data)
		if format == "" {
			t.Fatalf("%s: формат не распознан", name)
		}
		doc, err := tzReadText(format, data)
		if err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if doc.Scan {
			t.Fatalf("%s: текстовый документ признан сканом", name)
		}
		rows := tzRowsOf(doc.Text)
		if strings.Join(rows, "\n") != strings.Join(tzWantRows, "\n") {
			t.Fatalf("%s: строки таблицы:\n%s", name, strings.Join(rows, "\n"))
		}
		for _, want := range []string{
			"резервное копирование 3 (трёх) физических серверов и 50 (пятидесяти) рабочих",
			"Иванов Пётр Сергеевич", "Монтаж серверов в стойку Заказчика",
		} {
			if !strings.Contains(tzSpaces(doc.Text), want) {
				t.Fatalf("%s: в тексте нет %q", name, want)
			}
		}
		if strings.Contains(doc.Text, "№\n") || strings.Contains(doc.Text, "котировок№") {
			t.Fatalf("%s: псевдоглиф «№» в конце строк", name)
		}
	}
}

// Скан без текстового слоя — признак скана (этап ocr), а не пустой документ.
func TestTZScanDetected(t *testing.T) {
	data := tzFile(t, "TZ_Severny_veter_scan.pdf")
	doc, err := tzReadText(tzSniff("TZ_Severny_veter_scan.pdf", data), data)
	if err != nil {
		t.Fatal(err)
	}
	if !doc.Scan || doc.Pages != 2 {
		t.Fatalf("scan=%v pages=%d", doc.Scan, doc.Pages)
	}
}

// Формат — по расширению и сигнатуре вместе.
func TestTZSniff(t *testing.T) {
	var zipNoDoc bytes.Buffer
	zw := zip.NewWriter(&zipNoDoc)
	w, _ := zw.Create("xl/workbook.xml")
	_, _ = w.Write([]byte("<x/>"))
	_ = zw.Close()
	docx := tzFile(t, "TZ_Severny_veter.docx")
	cases := []struct {
		name string
		data []byte
		want string
	}{
		{"a.pdf", []byte("%PDF-1.7\n"), tzFormatPDF},
		{"A.PDF", []byte("%PDF-1.4"), tzFormatPDF},
		{"a.pdf", []byte("PK\x03\x04"), ""},
		{"a.docx", docx, tzFormatDOCX},
		{"a.pdf", docx, ""},
		{"a.docx", zipNoDoc.Bytes(), ""},
		{"a.doc", docx, ""},
		{"a", []byte("%PDF"), ""},
		{"a.txt", []byte("%PDF"), ""},
	}
	for _, c := range cases {
		if got := tzSniff(c.name, c.data); got != c.want {
			t.Errorf("%s: %q, want %q", c.name, got, c.want)
		}
	}
}

// Битый файл с верной сигнатурой — damaged, не паника.
func TestTZDamaged(t *testing.T) {
	if _, err := tzReadText(tzFormatPDF, []byte("%PDF-1.7\nмусор")); tzReasonOf(err) != tzReasonDamaged {
		t.Fatalf("pdf: %v", err)
	}
	var b bytes.Buffer
	zw := zip.NewWriter(&b)
	w, _ := zw.Create("word/document.xml")
	_, _ = w.Write([]byte("<w:document><w:body><w:p>"))
	_ = zw.Close()
	if _, err := tzReadText(tzFormatDOCX, b.Bytes()); tzReasonOf(err) != tzReasonDamaged {
		t.Fatalf("docx: %v", err)
	}
}

// ФИО и телефон из п. 1.4 в модель не уходят.
func TestTZMaskNames(t *testing.T) {
	in := "1.4. Контактное лицо Заказчика: Иванов Пётр Сергеевич, тел. +7 (000) 000-00-00, zakupki@severny-veter.example. " +
		"Ответственный — Петрова А. В.; согласовал И.И. Сидоров; Анна Викторовна на связи. Kaspersky Endpoint Security Стандартный."
	got := tzMaskText(in)
	for _, leak := range []string{"Иванов", "Пётр", "Сергеевич", "000-00-00", "zakupki@", "Петрова", "Сидоров", "Викторовна"} {
		if strings.Contains(got, leak) {
			t.Fatalf("утечка %q: %s", leak, got)
		}
	}
	for _, keep := range []string{"Контактное лицо Заказчика", "Kaspersky Endpoint Security Стандартный", "Ответственный"} {
		if !strings.Contains(got, keep) {
			t.Fatalf("лишнее маскирование, нет %q: %s", keep, got)
		}
	}
	if s, n := tzMaskNames("Межсетевой экран нового поколения. Общие сведения. Сервер Стоечный"); n != 0 {
		t.Fatalf("ложное ФИО: %s", s)
	}
}

func tzF64(v float64) *float64 { return &v }

// Проверки кода после модели (TZ-CORE §5): цитата дословно, название из таблицы, количество, номера, предел 50.
func TestTZCheck(t *testing.T) {
	text := strings.Join(append([]string{"№ | Наименование | Ед. изм. | Кол-во"}, tzWantRows...), "\n") +
		"\n3.1. Позиция 1. Антивирусная защита.\nЛицензия должна обеспечивать защиту 50 (пятидесяти)\nузлов. Тип лицензии – новая. Лицензия для коммерческой организации.\n" +
		"3.3. Позиция 3. Резервное копирование.\nПрограммное обеспечение, включённое в Единый реестр российских программ. Резервное копирование 3 (трёх) физических серверов и 50 (пятидесяти) рабочих станций.\n" +
		"3.4. Позиция 4. КриптоПро CSP версии 5.0. Лицензия на сервере, бессрочная.\n"
	raws := []tzRaw{
		{Number: "1", Title: "Kaspersky Endpoint Security для бизнеса – Стандартный, новая лицензия на 1 год", Type: "software", Named: true,
			Quantity: f64(50), Unit: "узел", Metric: "device", DurationMonths: f64(12), Deal: "new", OrgType: "commercial",
			Edition: "Стандартный", SearchQuery: "Kaspersky Endpoint Security Стандартный",
			SourceQuote: "Лицензия должна обеспечивать защиту 50 (пятидесяти) узлов…", Unverified: []string{"сертификат ФСТЭК России"}},
		{Number: "1", Title: "дубль", Type: "software"},
		{Number: "3", Title: "ПО резервного копирования (пересказ)", Type: "software", Quantity: f64(1), Unit: "комплект",
			QuantityText: "3 (трёх) физических серверов и 50 (пятидесяти) рабочих станций", DurationMonths: f64(12), Deal: "new",
			SourceQuote: "ПО должно копировать серверы", Unverified: []string{"включение в Единый реестр российских программ", "видеоконференции"}},
		{Number: "4", Title: "СКЗИ КриптоПро CSP версии 5.0, серверная лицензия, бессрочная", Type: "software", Named: true,
			Quantity: f64(-2), Unit: "шт.", Metric: "server", Perpetual: true, SearchQuery: "КриптоПро CSP серверная"},
		{Number: "7", Title: "Сервер стоечный 2U", Type: "hardware", Quantity: f64(2), Unit: "шт.", Metric: "device", SearchQuery: "стоечный сервер 2U"},
		{Number: "8", Title: "Работы по монтажу и пусконаладке серверного оборудования", Type: "service", Named: true, Quantity: f64(1), Unit: "усл. ед."},
	}
	ps, extracted := tzCheck(raws, text)
	if extracted != 5 || len(ps) != 5 {
		t.Fatalf("extracted=%d len=%d", extracted, len(ps))
	}
	p1 := ps[0]
	if p1.ID != "p1" || p1.Kind != tzKindProduct || !p1.Named || p1.Purchase == nil || *p1.Purchase.Quantity != 50 ||
		*p1.Purchase.DurationMonths != 12 || p1.Purchase.Deal != DealNew || p1.Purchase.Metric != "device" || p1.Purchase.OrgType != OrgCommercial {
		t.Fatalf("p1 = %+v purchase=%+v", p1, p1.Purchase)
	}
	if p1.SourceQuote != "Лицензия должна обеспечивать защиту 50 (пятидесяти) узлов" {
		t.Fatalf("цитата через перенос строки и с многоточием: %q", p1.SourceQuote)
	}
	if len(p1.UnverifiedRequirements) != 0 {
		t.Fatalf("требование не из текста позиции осталось: %v", p1.UnverifiedRequirements)
	}
	wantAttrs := "quantity=50 узлов;duration=1 год;deal=Новая лицензия;org_type=Коммерческая организация"
	if got := attrsString(p1.Attributes); got != wantAttrs {
		t.Fatalf("плашки p1: %s", got)
	}
	if got := filtersString(p1.Filters); got != "license_quantity=50;license_metric=device;duration_months=12;edition=Стандартный" {
		t.Fatalf("фильтры p1: %s", got)
	}
	p3 := ps[1]
	if p3.Title != tzWantRows[2][len("3 | "):strings.Index(tzWantRows[2], " | комплект")] {
		t.Fatalf("название не из таблицы: %q", p3.Title)
	}
	if p3.SourceQuote != "" {
		t.Fatalf("пересказ вместо цитаты: %q", p3.SourceQuote)
	}
	if got := attrsString(p3.Attributes); got != "quantity=3 (трёх) физических серверов и 50 (пятидесяти) рабочих станций;duration=1 год" {
		t.Fatalf("количество позиции 3 — из раздела требований: %s", got)
	}
	if p3.Purchase.Quantity != nil || p3.Purchase.Deal != "" {
		t.Fatalf("составное количество и тип покупки без «новая» в тексте: %+v", p3.Purchase)
	}
	if strings.Join(p3.UnverifiedRequirements, ";") != "включение в Единый реестр российских программ" {
		t.Fatalf("не проверено p3: %v", p3.UnverifiedRequirements)
	}
	p4 := ps[2]
	if p4.Purchase.Quantity != nil || attrsString(p4.Attributes) != "duration=Бессрочная" || p4.Purchase.Metric != "server" {
		t.Fatalf("отрицательное количество не убрано: %+v %s", p4.Purchase, attrsString(p4.Attributes))
	}
	p7 := ps[3]
	if p7.SearchQuery != "стоечный сервер" || len(p7.Filters) != 0 || *p7.Purchase.Quantity != 2 || p7.Purchase.Metric != "" {
		t.Fatalf("оборудование: %+v %+v", p7, p7.Purchase)
	}
	p8 := ps[4]
	if p8.Kind != tzKindService || p8.Named || filtersString(p8.Filters) != "kind=service" {
		t.Fatalf("работы: %+v", p8)
	}

	// Больше 50 позиций: первые 50, extracted_count — полное число.
	var many []tzRaw
	for i := 1; i <= 57; i++ {
		many = append(many, tzRaw{Number: json.Number(strconv.Itoa(i)), Title: "Позиция " + strconv.Itoa(i), Type: "software"})
	}
	ps, extracted = tzCheck(many, "")
	if len(ps) != 50 || extracted != 57 || ps[49].Number != 50 {
		t.Fatalf("предел 50: len=%d extracted=%d", len(ps), extracted)
	}
}

func a0(r in.SearchRequest) bool { return r.TZ != nil }

func attrsString(as []out.TZAttribute) string {
	var p []string
	for _, a := range as {
		p = append(p, a.Key+"="+a.Value)
	}
	return strings.Join(p, ";")
}

func filtersString(fs []out.TZFilter) string {
	var p []string
	for _, f := range fs {
		p = append(p, f.Key+"="+f.Value)
	}
	return strings.Join(p, ";")
}

// tzFakeModel — модель извлечения: позиция по номеру из задания; считает вызовы и запоминает входы.
type tzFakeModel struct {
	calls atomic.Int32
	mu    sync.Mutex
	seen  []string
	fail  bool
}

func (m *tzFakeModel) Model() string { return "aliceai-llm-flash" }

func (m *tzFakeModel) Chat(_ context.Context, req llm.Request) (*llm.Response, error) {
	m.calls.Add(1)
	user := req.Messages[len(req.Messages)-1].Content
	m.mu.Lock()
	m.seen = append(m.seen, user)
	m.mu.Unlock()
	if m.fail {
		return nil, errors.New("модель недоступна")
	}
	n := 0
	if mm := regexp.MustCompile(`позицию № (\d+)`).FindStringSubmatch(user); mm != nil {
		n, _ = strconv.Atoi(mm[1])
	}
	body, _ := json.Marshal(map[string]any{"number": n, "title": "позиция", "type": "software", "quantity": 1, "unit": "шт.",
		"search_query": "товар " + strconv.Itoa(n), "source_quote": "", "unverified_requirements": []string{}})
	return &llm.Response{Message: llm.Message{Role: "assistant", Content: string(body)}, Usage: llm.Usage{PromptTokens: 100, CompletionTokens: 50}}, nil
}

func tzTestServices(model ChatModel, now *time.Time) *Services {
	return NewServices(nil, Options{LLMMode: "on", IntentModel: model, Now: func() time.Time { return *now }})
}

func tzWait(t *testing.T, s *Services, id string) *out.TZOperation {
	t.Helper()
	for i := 0; i < 200; i++ {
		op, err := s.TZOperation(id)
		if err != nil {
			t.Fatal(err)
		}
		if op.Status != tzStatusProcessing {
			return op
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Fatal("операция не завершилась")
	return nil
}

// Полный ход: загрузка, опрос, разбор под хешем; повторная загрузка и GET по хешу модель не зовут; ФИО и телефон
// в модель не уходят; истёкший и неизвестный хеш — 404.
func TestTZFlow(t *testing.T) {
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	m := &tzFakeModel{}
	s := tzTestServices(m, &now)
	data := tzFile(t, "TZ_Severny_veter.docx")
	up, err := s.TZStart(context.Background(), "C:\\docs\\TZ_Severny_veter.docx", data)
	if err != nil || up.OperationID == "" || up.Status != "" {
		t.Fatalf("start: %+v %v", up, err)
	}
	op := tzWait(t, s, up.OperationID)
	if op.Status != tzStatusCompleted || len(op.Hash) != 64 {
		t.Fatalf("op = %+v", op)
	}
	if m.calls.Load() != 8 {
		t.Fatalf("вызовов модели %d, ждали 8 (по позиции)", m.calls.Load())
	}
	for _, in := range m.seen {
		for _, leak := range []string{"Иванов", "Пётр Сергеевич", "000-00-00", "zakupki@"} {
			if strings.Contains(in, leak) {
				t.Fatalf("в модель ушло %q", leak)
			}
		}
	}
	res, err := s.TZResult(context.Background(), op.Hash)
	if err != nil {
		t.Fatal(err)
	}
	if res.FileName != "TZ_Severny_veter.docx" || res.ExtractedCount != 8 || res.ProcessedCount != 8 || len(res.Positions) != 8 ||
		res.Positions[0].Title != tzWantRows[0][4:strings.Index(tzWantRows[0], " | узел")] {
		t.Fatalf("res = %+v", res)
	}
	// Повторная загрузка того же файла — без модели.
	up2, _ := s.TZStart(context.Background(), "copy.docx", data)
	if op2 := tzWait(t, s, up2.OperationID); op2.Hash != op.Hash || m.calls.Load() != 8 {
		t.Fatalf("повтор: %+v, вызовов %d", op2, m.calls.Load())
	}
	if res, _ := s.TZResult(context.Background(), op.Hash); res.FileName != "copy.docx" || m.calls.Load() != 8 {
		t.Fatalf("GET по хешу: %+v", res)
	}
	// Неизвестный, кривой и истёкший хеш — 404.
	for _, h := range []string{strings.Repeat("0", 64), "../etc", strings.ToUpper(op.Hash)} {
		if _, err := s.TZResult(context.Background(), h); tzStatus(err) != 404 {
			t.Fatalf("%s: %v", h, err)
		}
	}
	now = now.Add(tzResultTTL + time.Minute)
	if _, err := s.TZResult(context.Background(), op.Hash); tzStatus(err) != 404 {
		t.Fatalf("истёкший хеш: %v", err)
	}
	if _, err := s.TZOperation(up.OperationID); tzStatus(err) != 404 {
		t.Fatalf("просроченная операция: %v", err)
	}
}

// Отказы до разбора — операция сразу в error с причиной; модель недоступна — general; скан без распознавания — ocr.
func TestTZErrors(t *testing.T) {
	now := time.Date(2026, 5, 29, 10, 0, 0, 0, time.UTC)
	m := &tzFakeModel{fail: true}
	s := tzTestServices(m, &now)
	for _, c := range []struct {
		name   string
		data   []byte
		reason string
	}{
		{"a.txt", []byte("hello"), tzReasonFormat},
		{"a.pdf", []byte("PK\x03\x04"), tzReasonFormat},
		{"a.pdf", make([]byte, TZMaxFileBytes+1), tzReasonSize},
		{"a.pdf", nil, tzReasonDamaged},
	} {
		up, err := s.TZStart(context.Background(), c.name, c.data)
		if err != nil || up.Status != tzStatusError || up.Reason != c.reason {
			t.Fatalf("%s: %+v %v", c.name, up, err)
		}
		if op, _ := s.TZOperation(up.OperationID); op.Status != tzStatusError || op.Reason != c.reason {
			t.Fatalf("%s: op %+v", c.name, op)
		}
	}
	up, _ := s.TZStart(context.Background(), "t.pdf", tzFile(t, "TZ_Severny_veter.pdf"))
	if op := tzWait(t, s, up.OperationID); op.Status != tzStatusError || op.Reason != tzReasonGeneral {
		t.Fatalf("модель упала: %+v", op)
	}
	up, _ = s.TZStart(context.Background(), "scan.pdf", tzFile(t, "TZ_Severny_veter_scan.pdf"))
	if op := tzWait(t, s, up.OperationID); op.Status != tzStatusError || op.Reason != tzReasonOCR {
		t.Fatalf("скан: %+v", op)
	}
	// Отмена: операция забыта.
	up, _ = s.TZStart(context.Background(), "t.docx", tzFile(t, "TZ_Severny_veter.docx"))
	if err := s.TZCancel(up.OperationID); err != nil {
		t.Fatal(err)
	}
	if _, err := s.TZOperation(up.OperationID); tzStatus(err) != 404 {
		t.Fatalf("после отмены: %v", err)
	}
}

// source = tz: количество, метрика, срок и редакция из фильтров — условия запроса; модель выключена.
func TestTZSearchConditions(t *testing.T) {
	req := in.SearchRequest{Query: "q", Source: "tz", Filters: []in.Filter{
		{Key: facetQuantity, Op: "eq", Value: "50"}, {Key: facetMetric, Op: "eq", Value: "device"},
		{Key: facetDuration, Op: "eq", Value: float64(12)}, {Key: tzFilterEdition, Op: "eq", Value: "Орёл"},
		{Key: facetVendor, Op: "eq", Value: "kaspersky"},
	}}
	ctx, got := tzSearch(context.Background(), req)
	if !llmOffBy(ctx) {
		t.Fatal("модель не выключена")
	}
	if got.TZ == nil || *got.TZ != (in.TZConditions{Quantity: 50, Metric: "device", DurationMonths: 12, Edition: "Орел"}) {
		t.Fatalf("условия: %+v", got.TZ)
	}
	if len(got.Filters) != 1 || got.Filters[0].Key != facetVendor || !a0(got) {
		t.Fatalf("фильтры: %+v", got.Filters)
	}
	var a queryAttrs
	a.applyTZ(got.TZ)
	if a.quantity != 50 || a.metric != "" || a.metricPref != "device" || a.duration != 12 || a.edition != "Орел" || !a.tzSoftQty {
		t.Fatalf("attrs: %+v", a)
	}
	if _, live := tzSearch(context.Background(), in.SearchRequest{Query: "q", Filters: req.Filters}); live.TZ != nil || len(live.Filters) != 5 {
		t.Fatal("живой поиск не должен меняться")
	}
	// Работы: товар каталога не точное совпадение.
	resp := &out.SearchResponse{Items: []out.SearchItem{{MatchType: out.MatchExact, Group: "exact",
		Variants: []out.SearchItem{{MatchType: out.MatchExact, Group: "exact"}}}}}
	svc := in.SearchRequest{Source: "tz", Filters: []in.Filter{{Key: tzFilterKind, Op: "eq", Value: "service"}}}
	tzServiceResult(svc, resp)
	if it := resp.Items[0]; it.MatchType != out.MatchRelated || it.Group != "match" || it.Variants[0].MatchType != out.MatchRelated {
		t.Fatalf("работы: %+v", it)
	}
	// Равные варианты: без ограничения по размеру организации, затем дешевле.
	p := func(v float64) *float64 { return &v }
	a = queryAttrs{tzSearch: true}
	priv := indexDoc{Title: "ALSE «Орел»", PriceRub: p(14550), Attributes: "Размер организации: Крупный бизнес"}
	std := indexDoc{Title: "ALSE «Орел»", PriceRub: p(6150), Attributes: "Размер организации: Крупный бизнес"}
	smb := indexDoc{Title: "ALSE «Орел»", PriceRub: p(5100), Attributes: "Размер организации: Малый и средний бизнес"}
	if _, ok := a.tzHeadLess(std, indexDoc{Title: "ALSE серверная", PriceRub: p(1400)}); ok {
		t.Fatal("разные названия — не по цене")
	}
	if l, ok := a.tzHeadLess(std, priv); !ok || !l {
		t.Fatal("дешевле — выше")
	}
	if l, ok := a.tzHeadLess(smb, std); !ok || l {
		t.Fatal("акция для МСП — не головой")
	}
	if _, ok := (queryAttrs{}).tzHeadLess(std, priv); ok {
		t.Fatal("живой поиск: порядок прежний")
	}
}

func tzStatus(err error) int {
	var e *errs.Error
	if errors.As(err, &e) {
		return e.HTTPStatus()
	}
	return 0
}

// Картинки страниц скана достаются из файла как JPEG: по одной на страницу.
func TestTZScanJPEGs(t *testing.T) {
	pages, err := tzScanJPEGs(tzFile(t, "TZ_Severny_veter_scan.pdf"))
	if err != nil || len(pages) != 2 {
		t.Fatalf("pages=%d err=%v", len(pages), err)
	}
	for i, p := range pages {
		if len(p) < 100_000 || !bytes.HasPrefix(p, []byte{0xFF, 0xD8, 0xFF}) || !bytes.HasSuffix(p, []byte{0xFF, 0xD9}) {
			t.Fatalf("страница %d: %d байт", i+1, len(p))
		}
	}
	if _, err := tzScanJPEGs(tzFile(t, "TZ_Severny_veter.pdf")); err == nil {
		t.Fatal("в текстовом PDF нашлись картинки страниц")
	}
}

// Скан через локальный tesseract (если установлен): та же таблица на восемь строк, что у DOCX и PDF.
func TestTZScanTesseract(t *testing.T) {
	tess := ocr.NewTesseract()
	if tess == nil {
		t.Skip("tesseract с языком rus не установлен")
	}
	text, err := (&tzVision{img: tess}).Recognize(context.Background(), tzFile(t, "TZ_Severny_veter_scan.pdf"), 2)
	if err != nil {
		t.Fatal(err)
	}
	rows := tzRowsOf(text)
	if len(rows) != len(tzWantRows) {
		t.Fatalf("строк таблицы %d:\n%s", len(rows), strings.Join(rows, "\n"))
	}
	for i, r := range rows {
		if got, want := tzNorm(strings.TrimRight(r, ":")), tzNorm(tzWantRows[i]); got != want {
			t.Errorf("строка %d:\n%s\n%s", i+1, got, want)
		}
	}
}
