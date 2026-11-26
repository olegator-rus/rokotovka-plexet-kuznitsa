package catalogsource

import (
	"bufio"
	"bytes"
	"encoding/csv"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"unicode/utf8"

	"github.com/xuri/excelize/v2"
	"golang.org/x/text/encoding/charmap"
)

// Чтение таблиц любого формата в одинаковые строки: колонка → текст ячейки. Вложенный JSON
// расплющивается в "a.b"; числа и логические значения — текстом; массивы и объекты — JSON-текстом.

// record — строка таблицы источника.
type record map[string]string

// tableData — прочитанный файл: строки, заголовок (порядок колонок) и что выяснилось при чтении.
type tableData struct {
	rows    []record
	headers []string
	// detected — кодировка и разделитель CSV, лист XLSX: для сводки и аудита.
	detected string
}

// readTable — файл таблицы по сопоставлению.
func readTable(m *Mapping, t *Table) (*tableData, error) {
	path := m.path(t.File)
	format := strings.ToLower(t.Format)
	if format == "" {
		switch strings.ToLower(filepath.Ext(path)) {
		case ".csv", ".tsv", ".txt":
			format = "csv"
		case ".json":
			format = "json"
		case ".jsonl", ".ndjson":
			format = "jsonl"
		case ".xlsx", ".xlsm":
			format = "xlsx"
		default:
			return nil, fmt.Errorf("%s: формат не понят по расширению — укажите format", t.File)
		}
	}
	var (
		td  *tableData
		err error
	)
	switch format {
	case "csv":
		td, err = readCSV(path, t)
	case "json":
		td, err = readJSONTable(path, t)
	case "jsonl":
		td, err = readJSONL(path)
	case "xlsx":
		td, err = readXLSX(path, t)
	}
	if err != nil {
		return nil, fmt.Errorf("%s: %w", t.File, err)
	}
	return td, nil
}

// decodeText — UTF-8 (без BOM) или cp1251. auto: валидный UTF-8 — он, иначе cp1251.
func decodeText(raw []byte, enc string) ([]byte, string, error) {
	raw = bytes.TrimPrefix(raw, []byte("\xEF\xBB\xBF"))
	switch strings.ToLower(strings.ReplaceAll(enc, "-", "")) {
	case "utf8":
		if !utf8.Valid(raw) {
			return nil, "", errors.New("файл не в UTF-8 — укажите encoding: cp1251")
		}
		return raw, "utf-8", nil
	case "cp1251", "windows1251":
		out, err := charmap.Windows1251.NewDecoder().Bytes(raw)
		return out, "cp1251", err
	case "", "auto":
		if utf8.Valid(raw) {
			return raw, "utf-8", nil
		}
		out, err := charmap.Windows1251.NewDecoder().Bytes(raw)
		return out, "cp1251", err
	}
	return nil, "", fmt.Errorf("неизвестная кодировка %q (utf-8 | cp1251 | auto)", enc)
}

// sniffDelimiter — разделитель по первой непустой строке: чего больше вне кавычек из ; , \t |.
func sniffDelimiter(text []byte) rune {
	line := text
	for len(line) > 0 {
		i := bytes.IndexByte(line, '\n')
		first := line
		if i >= 0 {
			first, line = line[:i], line[i+1:]
		} else {
			line = nil
		}
		if len(bytes.TrimSpace(first)) == 0 {
			continue
		}
		best, bestN := ',', 0
		for _, d := range []rune{';', ',', '\t', '|'} {
			n, quoted := 0, false
			for _, r := range string(first) {
				switch {
				case r == '"':
					quoted = !quoted
				case r == d && !quoted:
					n++
				}
			}
			if n > bestN {
				best, bestN = d, n
			}
		}
		return best
	}
	return ','
}

func readCSV(path string, t *Table) (*tableData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	text, enc, err := decodeText(raw, t.Encoding)
	if err != nil {
		return nil, err
	}
	var delim rune
	switch d := t.Delimiter; d {
	case "", "auto":
		delim = sniffDelimiter(text)
	case `\t`, "tab":
		delim = '\t'
	default:
		r, size := utf8.DecodeRuneInString(d)
		if size != len(d) {
			return nil, fmt.Errorf("delimiter %q: ожидается один символ", d)
		}
		delim = r
	}
	r := csv.NewReader(bytes.NewReader(text))
	r.Comma = delim
	r.FieldsPerRecord = -1
	r.LazyQuotes = true
	var cells [][]string
	for {
		row, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		if err != nil {
			return nil, err
		}
		cells = append(cells, row)
	}
	td, err := gridToRecords(cells, t.HeaderRow)
	if err != nil {
		return nil, err
	}
	name := map[rune]string{';': ";", ',': ",", '\t': "tab", '|': "|"}[delim]
	if name == "" {
		name = string(delim)
	}
	td.detected = "csv " + enc + ", разделитель " + name
	return td, nil
}

func readXLSX(path string, t *Table) (*tableData, error) {
	f, err := excelize.OpenFile(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sheet := t.Sheet
	if sheet == "" {
		sheets := f.GetSheetList()
		if len(sheets) == 0 {
			return nil, errors.New("в книге нет листов")
		}
		sheet = sheets[0]
	}
	cells, err := f.GetRows(sheet)
	if err != nil {
		return nil, fmt.Errorf("лист %q: %w", sheet, err)
	}
	td, err := gridToRecords(cells, t.HeaderRow)
	if err != nil {
		return nil, err
	}
	td.detected = "xlsx, лист " + sheet
	return td, nil
}

// gridToRecords — первая непустая строка (или headerRow) — заголовок, дальше данные; пустые строки
// пропускаются, повтор имени колонки получает суффикс " (2)".
func gridToRecords(cells [][]string, headerRow int) (*tableData, error) {
	h := -1
	if headerRow > 0 {
		h = headerRow - 1
	} else {
		for i, row := range cells {
			if !emptyRow(row) {
				h = i
				break
			}
		}
	}
	if h < 0 || h >= len(cells) {
		return nil, errors.New("нет строки заголовка")
	}
	headers := make([]string, len(cells[h]))
	seen := map[string]int{}
	for i, c := range cells[h] {
		c = strings.TrimSpace(c)
		if c == "" {
			c = "col" + strconv.Itoa(i+1)
		}
		seen[c]++
		if seen[c] > 1 {
			c = fmt.Sprintf("%s (%d)", c, seen[c])
		}
		headers[i] = c
	}
	td := &tableData{headers: headers}
	for _, row := range cells[h+1:] {
		if emptyRow(row) {
			continue
		}
		rec := make(record, len(headers))
		for i, v := range row {
			if i < len(headers) {
				rec[headers[i]] = strings.TrimSpace(v)
			}
		}
		td.rows = append(td.rows, rec)
	}
	return td, nil
}

func emptyRow(row []string) bool {
	for _, c := range row {
		if strings.TrimSpace(c) != "" {
			return false
		}
	}
	return true
}

func readJSONTable(path string, t *Table) (*tableData, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	raw = bytes.TrimPrefix(raw, []byte("\xEF\xBB\xBF"))
	dec := json.NewDecoder(bytes.NewReader(raw))
	dec.UseNumber()
	var root any
	if err := dec.Decode(&root); err != nil {
		return nil, err
	}
	node := root
	if t.JSONPath != "" {
		for _, k := range strings.Split(t.JSONPath, ".") {
			obj, ok := node.(map[string]any)
			if !ok {
				return nil, fmt.Errorf("json_path %q: %q не объект", t.JSONPath, k)
			}
			node = obj[k]
		}
	}
	list, ok := node.([]any)
	if !ok {
		// Объект { "id": {…}, … } — словарь строк: ключ становится колонкой "_key".
		obj, isObj := node.(map[string]any)
		if !isObj {
			return nil, errors.New("ожидается массив строк (укажите json_path)")
		}
		for k, v := range obj {
			if o, ok := v.(map[string]any); ok {
				o["_key"] = k
				list = append(list, o)
			}
		}
	}
	td := &tableData{detected: "json"}
	cols := map[string]bool{}
	for i, item := range list {
		obj, ok := item.(map[string]any)
		if !ok {
			return nil, fmt.Errorf("строка %d — не объект", i+1)
		}
		rec := record{}
		flatten("", obj, rec)
		for k := range rec {
			if !cols[k] {
				cols[k] = true
				td.headers = append(td.headers, k)
			}
		}
		td.rows = append(td.rows, rec)
	}
	return td, nil
}

func readJSONL(path string) (*tableData, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer func() { _ = f.Close() }()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 0, 4096), maxLogLine)
	td := &tableData{detected: "jsonl"}
	cols := map[string]bool{}
	for n := 1; sc.Scan(); n++ {
		line := bytes.TrimSpace(sc.Bytes())
		if n == 1 {
			line = bytes.TrimPrefix(line, []byte("\xEF\xBB\xBF"))
		}
		if len(line) == 0 {
			continue
		}
		dec := json.NewDecoder(bytes.NewReader(line))
		dec.UseNumber()
		var obj map[string]any
		if err := dec.Decode(&obj); err != nil {
			return nil, fmt.Errorf("строка %d: %w", n, err)
		}
		rec := record{}
		flatten("", obj, rec)
		for k := range rec {
			if !cols[k] {
				cols[k] = true
				td.headers = append(td.headers, k)
			}
		}
		td.rows = append(td.rows, rec)
	}
	return td, sc.Err()
}

// flatten — вложенные объекты в ключи через точку; массивы и объекты дополнительно лежат JSON-текстом
// под своим ключом (attributes целиком, judgments целиком).
func flatten(prefix string, v any, rec record) {
	switch x := v.(type) {
	case map[string]any:
		if prefix != "" {
			raw, _ := json.Marshal(x)
			rec[prefix] = string(raw)
			prefix += "."
		}
		for k, val := range x {
			flatten(prefix+k, val, rec)
		}
	case []any:
		raw, _ := json.Marshal(x)
		rec[prefix] = string(raw)
	case nil:
		rec[prefix] = ""
	case string:
		rec[prefix] = strings.TrimSpace(x)
	case json.Number:
		rec[prefix] = x.String()
	case bool:
		rec[prefix] = strconv.FormatBool(x)
	default:
		rec[prefix] = fmt.Sprint(x)
	}
}
