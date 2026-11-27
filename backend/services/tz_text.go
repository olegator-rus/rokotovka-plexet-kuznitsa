package services

import (
	"archive/zip"
	"bytes"
	"encoding/xml"
	"errors"
	"io"
	"math"
	"regexp"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/ledongthuc/pdf"
)

// Текст из файла ТЗ (контур ТЗ, TZ-CORE §3): DOCX — archive/zip и encoding/xml стандартной библиотеки, абзацы и
// таблицы; PDF с текстом — github.com/ledongthuc/pdf (BSD-3): строки собираются по координатам глифов, таблица
// склеивается построчно с разделителем колонок «1 | Kaspersky … | узел | 50», чтобы количество не отрывалось
// от названия. Признак скана — меньше tzScanCharsPerPage знаков на страницу.

// Форматы файла ТЗ.
const (
	tzFormatPDF  = "pdf"
	tzFormatDOCX = "docx"
)

// Причины отказа разбора (TZ-CORE §2).
const (
	tzReasonFormat      = "format"
	tzReasonSize        = "size"
	tzReasonDamaged     = "damaged"
	tzReasonPassword    = "password"
	tzReasonOCR         = "ocr"
	tzReasonNoPositions = "no_positions"
	tzReasonGeneral     = "general"
)

const (
	// tzScanCharsPerPage — меньше стольких знаков (без пробелов) на страницу — PDF считается сканом.
	tzScanCharsPerPage = 50
	// tzMaxTextRunes — потолок текста документа: дальше текст не читается (защита памяти и бюджета модели).
	tzMaxTextRunes = 400_000
	// tzMaxDocxXML — потолок распакованного word/document.xml (защита от zip-бомбы).
	tzMaxDocxXML = 64 << 20
	// tzMaxPDFPages — потолок страниц PDF.
	tzMaxPDFPages = 300
	// tzColumnGap — разрыв между глифами строки шире стольких пунктов — граница колонки таблицы.
	tzColumnGap = 9.0
)

// tzTextError — отказ чтения файла с причиной для операции.
type tzTextError struct{ reason string }

func (e *tzTextError) Error() string { return "tz: " + e.reason }

func tzFail(reason string) error { return &tzTextError{reason: reason} }

// tzReasonOf — причина отказа для операции: известная или general.
func tzReasonOf(err error) string {
	var te *tzTextError
	if errors.As(err, &te) {
		return te.reason
	}
	return tzReasonGeneral
}

// tzDocument — текст документа: строки (абзацы и строки таблиц) и признак скана.
type tzDocument struct {
	Text  string
	Pages int
	Scan  bool
}

// tzSniff — формат по расширению и сигнатуре; "" — не PDF и не DOCX или расширение не совпало с содержимым.
func tzSniff(name string, data []byte) string {
	ext := strings.ToLower(name)
	if i := strings.LastIndexByte(ext, '.'); i >= 0 {
		ext = ext[i+1:]
	} else {
		ext = ""
	}
	switch ext {
	case tzFormatPDF:
		if bytes.HasPrefix(data, []byte("%PDF")) {
			return tzFormatPDF
		}
	case tzFormatDOCX:
		if !bytes.HasPrefix(data, []byte("PK\x03\x04")) {
			return ""
		}
		zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
		if err != nil {
			return ""
		}
		for _, f := range zr.File {
			if f.Name == "word/document.xml" {
				return tzFormatDOCX
			}
		}
	}
	return ""
}

// tzReadText — текст файла проверенного формата.
func tzReadText(format string, data []byte) (*tzDocument, error) {
	switch format {
	case tzFormatDOCX:
		return tzDocxText(data)
	case tzFormatPDF:
		return tzPDFText(data)
	}
	return nil, tzFail(tzReasonFormat)
}

// --- DOCX ---

// tzDocxText — абзацы и строки таблиц word/document.xml по порядку документа.
func tzDocxText(data []byte) (*tzDocument, error) {
	zr, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, tzFail(tzReasonDamaged)
	}
	var doc *zip.File
	for _, f := range zr.File {
		if f.Name == "word/document.xml" {
			doc = f
		}
		if f.Name == "EncryptedPackage" {
			return nil, tzFail(tzReasonPassword)
		}
	}
	if doc == nil {
		return nil, tzFail(tzReasonDamaged)
	}
	rc, err := doc.Open()
	if err != nil {
		return nil, tzFail(tzReasonDamaged)
	}
	defer rc.Close()
	lines, err := tzDocxLines(io.LimitReader(rc, tzMaxDocxXML))
	if err != nil {
		return nil, tzFail(tzReasonDamaged)
	}
	return &tzDocument{Text: tzClip(strings.Join(lines, "\n")), Pages: 1}, nil
}

// tzDocxLines — разбор WordprocessingML: абзац — строка; строка таблицы — ячейки через « | » (абзацы ячейки —
// через пробел); элемент нумерованного или маркированного списка — с «– » в начале.
func tzDocxLines(r io.Reader) ([]string, error) {
	const ns = "http://schemas.openxmlformats.org/wordprocessingml/2006/main"
	dec := xml.NewDecoder(r)
	var (
		lines []string
		para  strings.Builder // текущий абзац
		list  bool            // у абзаца есть numPr
		depth int             // вложенность таблиц
		rows  [][]string      // по уровню таблицы: ячейки текущей строки
		cell  []strings.Builder
		inT   bool
	)
	flushPara := func() {
		t := strings.TrimSpace(tzSpaces(para.String()))
		para.Reset()
		if t == "" {
			list = false
			return
		}
		if list {
			t = "– " + t
		}
		list = false
		if depth > 0 {
			c := &cell[depth-1]
			if c.Len() > 0 {
				c.WriteByte(' ')
			}
			c.WriteString(t)
			return
		}
		lines = append(lines, t)
	}
	for {
		tok, err := dec.Token()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		switch t := tok.(type) {
		case xml.StartElement:
			if t.Name.Space != ns {
				continue
			}
			switch t.Name.Local {
			case "tbl":
				depth++
				rows = append(rows, nil)
				cell = append(cell, strings.Builder{})
			case "tr":
				if depth > 0 {
					rows[depth-1] = []string{}
				}
			case "tc":
				if depth > 0 {
					cell[depth-1].Reset()
				}
			case "numPr":
				list = true
			case "t":
				inT = true
			case "tab":
				para.WriteByte(' ')
			case "br", "cr":
				para.WriteByte(' ')
			}
		case xml.EndElement:
			if t.Name.Space != ns {
				continue
			}
			switch t.Name.Local {
			case "t":
				inT = false
			case "p":
				flushPara()
			case "tc":
				if depth > 0 {
					rows[depth-1] = append(rows[depth-1], strings.TrimSpace(cell[depth-1].String()))
					cell[depth-1].Reset()
				}
			case "tr":
				if depth > 0 {
					row := strings.Join(rows[depth-1], " | ")
					if strings.Trim(row, " |") != "" {
						if depth == 1 {
							lines = append(lines, row)
						} else {
							c := &cell[depth-2]
							if c.Len() > 0 {
								c.WriteByte(' ')
							}
							c.WriteString(row)
						}
					}
					rows[depth-1] = nil
				}
			case "tbl":
				if depth > 0 {
					depth--
					rows = rows[:depth]
					cell = cell[:depth]
				}
			}
		case xml.CharData:
			if inT {
				para.Write(t)
			}
		}
	}
	return lines, nil
}

// --- PDF ---

// tzPDFText — текст PDF по строкам; страниц с текстом меньше порога — скан.
func tzPDFText(data []byte) (doc *tzDocument, err error) {
	defer func() {
		// Библиотека на повреждённом файле может паниковать: это отказ разбора, а не падение процесса.
		if r := recover(); r != nil {
			doc, err = nil, tzFail(tzReasonDamaged)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "encrypt") || strings.Contains(strings.ToLower(err.Error()), "password") {
			return nil, tzFail(tzReasonPassword)
		}
		return nil, tzFail(tzReasonDamaged)
	}
	n := r.NumPage()
	if n <= 0 {
		return nil, tzFail(tzReasonDamaged)
	}
	if n > tzMaxPDFPages {
		n = tzMaxPDFPages
	}
	var all []tzLine
	chars := 0
	for i := 1; i <= n; i++ {
		p := r.Page(i)
		if p.V.IsNull() {
			continue
		}
		lines := tzPageLines(tzGlyphs(p))
		for _, l := range lines {
			chars += l.chars()
		}
		all = append(all, lines...)
	}
	text := strings.Join(tzJoinTables(all), "\n")
	return &tzDocument{Text: tzClip(text), Pages: n, Scan: chars < tzScanCharsPerPage*n}, nil
}

// tzCell — фрагмент строки между разрывами колонок.
type tzCell struct {
	x0, x1 float64
	text   string
}

// tzLine — строка страницы: фрагменты слева направо.
type tzLine struct {
	y     float64
	cells []tzCell
}

func (l tzLine) chars() int {
	n := 0
	for _, c := range l.cells {
		for _, r := range c.text {
			if !unicode.IsSpace(r) {
				n++
			}
		}
	}
	return n
}

func (l tzLine) String() string {
	parts := make([]string, 0, len(l.cells))
	for _, c := range l.cells {
		parts = append(parts, c.text)
	}
	return strings.Join(parts, " ")
}

// tzPageLines — глифы страницы → строки сверху вниз; в строке разрыв шире tzColumnGap делит фрагменты.
func tzPageLines(glyphs []pdf.Text) []tzLine {
	gs := make([]pdf.Text, 0, len(glyphs))
	for _, g := range glyphs {
		if g.S != "" {
			gs = append(gs, g)
		}
	}
	sort.SliceStable(gs, func(i, j int) bool { return gs[i].Y > gs[j].Y })
	var lines []tzLine
	for i := 0; i < len(gs); {
		y := gs[i].Y
		tol := math.Max(gs[i].FontSize*0.35, 1.5)
		j := i
		for j < len(gs) && y-gs[j].Y <= tol {
			j++
		}
		row := append([]pdf.Text(nil), gs[i:j]...)
		sort.SliceStable(row, func(a, b int) bool { return row[a].X < row[b].X })
		var cells []tzCell
		var cur strings.Builder
		var c tzCell
		end := math.Inf(-1)
		for k, g := range row {
			if k > 0 && g.X-end > tzColumnGap {
				c.text = strings.TrimSpace(tzSpaces(cur.String()))
				if c.text != "" {
					cells = append(cells, c)
				}
				cur.Reset()
				c = tzCell{x0: g.X}
			} else if k > 0 && g.X-end > g.FontSize*0.15 && !strings.HasSuffix(cur.String(), " ") && g.S != " " {
				cur.WriteByte(' ') // пробела глифом нет, но буквы разнесены
			}
			if k == 0 {
				c.x0 = g.X
			}
			cur.WriteString(g.S)
			end = math.Max(end, g.X+g.W)
			c.x1 = end
		}
		c.text = strings.TrimSpace(tzSpaces(cur.String()))
		if c.text != "" {
			cells = append(cells, c)
		}
		if len(cells) > 0 {
			lines = append(lines, tzLine{y: y, cells: cells})
		}
		i = j
	}
	return lines
}

var reRowNumber = regexp.MustCompile(`^\d{1,3}\.?$`)

// tzJoinTables — строки страницы в текст; таблица (строка-заголовок из трёх и больше колонок) склеивается: строка
// с номером в первой колонке открывает строку таблицы, строки без номера дописываются в свои колонки.
func tzJoinTables(lines []tzLine) []string {
	var res []string
	for i := 0; i < len(lines); i++ {
		l := lines[i]
		if len(l.cells) < 3 {
			res = append(res, l.String())
			continue
		}
		// Заголовок таблицы: границы колонок — по началам его фрагментов.
		cols := make([]float64, len(l.cells))
		for k, c := range l.cells {
			cols[k] = c.x0
		}
		res = append(res, strings.Join(cellTexts(l.cells), " | "))
		var row []string
		flush := func() {
			if row != nil {
				res = append(res, strings.Join(row, " | "))
				row = nil
			}
		}
		j := i + 1
		for ; j < len(lines); j++ {
			ln := lines[j]
			numbered := reRowNumber.MatchString(ln.cells[0].text) && len(ln.cells) >= 2
			if numbered && len(ln.cells) == len(cols) {
				// Заголовок обычно выровнен по центру: начала колонок точнее берутся у строки данных.
				for k, c := range ln.cells {
					cols[k] = c.x0
				}
			}
			slots := make([]string, len(cols))
			for _, c := range ln.cells {
				k := tzColumnOf(cols, c)
				if numbered && c == ln.cells[0] {
					k = 0
				}
				if slots[k] != "" {
					slots[k] += " "
				}
				slots[k] += c.text
			}
			first := slots[0]
			switch {
			case numbered:
				flush()
				row = slots
			case first == "" && row != nil && ln.cells[0].x0 >= cols[1]-tzColumnGap:
				for k := range slots {
					if slots[k] == "" {
						continue
					}
					if row[k] != "" {
						row[k] += " "
					}
					row[k] += slots[k]
				}
			default:
				goto done
			}
		}
	done:
		flush()
		i = j - 1
	}
	return res
}

func cellTexts(cs []tzCell) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.text
	}
	return out
}

// tzColumnOf — колонка фрагмента: последняя, чьё начало левее начала фрагмента (с допуском на выравнивание).
func tzColumnOf(cols []float64, c tzCell) int {
	k := 0
	for i, x := range cols {
		if c.x0 >= x-tzColumnGap*2 {
			k = i
		}
	}
	return k
}

// --- общее ---

var reSpaces = regexp.MustCompile(`[\s\x{00A0}\x{2007}\x{202F}]+`)

// tzSpaces — пробелы любого вида → один пробел.
func tzSpaces(s string) string { return reSpaces.ReplaceAllString(s, " ") }

// tzClip — текст не длиннее tzMaxTextRunes знаков, без битых последовательностей UTF-8.
func tzClip(s string) string {
	s = strings.ToValidUTF8(s, "")
	if utf8.RuneCountInString(s) <= tzMaxTextRunes {
		return s
	}
	r := []rune(s)
	return string(r[:tzMaxTextRunes])
}
