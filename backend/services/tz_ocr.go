package services

import (
	"bytes"
	"context"
	"errors"
	"math"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"sync"
	"time"

	"kuznica/api/externalApi/ocr"

	"github.com/ledongthuc/pdf"
)

// Распознавание сканов ТЗ (контур ТЗ, П12-1, TZ-CORE §4). Скан — PDF, где каждая страница — картинка JPEG
// (DCTDecode). Картинки страниц достаются из файла как есть и уходят в синхронный recognizeText Yandex Vision OCR
// параллельно (асинхронный метод для многостраничного PDF не нужен). Слова с рамками собираются в строки и
// таблицу той же склейкой, что и PDF с текстом (tzJoinTables): «1 | Kaspersky … | узел | 50».

// TZImageOCR — распознавание одной картинки (externalApi/ocr.Client или фейк в тестах).
type TZImageOCR interface {
	Recognize(ctx context.Context, image []byte, mime string) ([]ocr.Word, error)
}

const (
	// tzOCRMaxPages — больше страниц скана не распознаём (цена и срок).
	tzOCRMaxPages = 30
	// tzOCRParallel — страниц одновременно.
	tzOCRParallel = 4
	// tzOCRPageTimeout — срок одной страницы.
	tzOCRPageTimeout = 40 * time.Second
	// tzOCRMaxImage — потолок картинки страницы: синхронный метод принимает до 10 МБ.
	tzOCRMaxImage = 10 << 20
)

var errTZScanImages = errors.New("tz: в скане нет картинок страниц JPEG")

// SetTZOCR — распознавание сканов; nil — нет (скан — причина ocr).
func (s *Services) SetTZOCR(o TZImageOCR) {
	if o == nil {
		s.tz().ocr = nil
		return
	}
	s.tz().ocr = &tzVision{img: o}
}

// tzVision — распознавание скана постранично.
type tzVision struct{ img TZImageOCR }

// Recognize — текст скана: строки страниц по порядку, таблица — построчно с разделителем колонок.
func (v *tzVision) Recognize(ctx context.Context, data []byte, _ int) (string, error) {
	pages, err := tzScanJPEGs(data)
	if err != nil {
		return "", err
	}
	if len(pages) > tzOCRMaxPages {
		pages = pages[:tzOCRMaxPages]
	}
	lines := make([][]tzLine, len(pages))
	errsOf := make([]error, len(pages))
	sem := make(chan struct{}, tzOCRParallel)
	var wg sync.WaitGroup
	for i, img := range pages {
		if img == nil {
			continue
		}
		wg.Add(1)
		go func() {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			pctx, cancel := context.WithTimeout(ctx, tzOCRPageTimeout)
			defer cancel()
			words, err := v.img.Recognize(pctx, img, "JPEG")
			if err != nil {
				errsOf[i] = err
				return
			}
			lines[i] = tzWordLines(words)
		}()
	}
	wg.Wait()
	var all []tzLine
	for i := range pages {
		if errsOf[i] != nil {
			return "", errsOf[i]
		}
		all = append(all, lines[i]...)
	}
	text := strings.Join(tzJoinTables(tzFixRowNumbers(all)), "\n")
	if len([]rune(strings.TrimSpace(text))) < tzScanCharsPerPage {
		return "", errors.New("tz: распознано слишком мало текста")
	}
	return tzClip(text), nil
}

// tzFixRowNumbers — номера строк таблицы после распознавания: «2:», «3.» — число; знак вместо цифры («т» вместо
// «1») в первой колонке строки из трёх и больше колонок — следующий номер по порядку.
func tzFixRowNumbers(lines []tzLine) []tzLine {
	inTable, last := false, 0
	for i := range lines {
		cs := lines[i].cells
		if len(cs) < 3 {
			continue
		}
		first := strings.TrimRight(cs[0].text, ".:;,")
		if !inTable {
			inTable = first == "№" || strings.HasPrefix(strings.ToLower(first), "п/п") || first == "N"
			continue
		}
		if n, err := strconv.Atoi(first); err == nil && n > 0 && n < 1000 {
			cs[0].text, last = first, n
			continue
		}
		if len([]rune(first)) <= 2 {
			last++
			cs[0].text = strconv.Itoa(last)
		}
	}
	return lines
}

// tzWordLines — слова страницы → строки сверху вниз. Слова сначала собираются в отрезки по строкам разметки
// распознавания (строка внутри одной колонки), отрезки с близкой серединой по высоте — в одну строку страницы;
// разрыв между отрезками шире высоты слова — граница колонки. Координаты переводятся в «пункты» (высота слова ≈
// кегль) и переворачиваются, чтобы склейка таблицы работала как у PDF с текстом.
func tzWordLines(words []ocr.Word) []tzLine {
	if len(words) == 0 {
		return nil
	}
	hs := make([]float64, 0, len(words))
	for _, w := range words {
		hs = append(hs, w.Y1-w.Y0)
	}
	sort.Float64s(hs)
	h := math.Max(hs[len(hs)/2], 1) // медианная высота слова
	type seg struct {
		x0, x1, y float64
		words     []ocr.Word
	}
	byLine := map[int]*seg{}
	var segs []*seg
	for i, w := range words {
		id := w.Line
		if id == 0 {
			id = -(i + 1) // без разметки строк — каждое слово отдельно
		}
		sg, ok := byLine[id]
		if !ok {
			sg = &seg{x0: w.X0, x1: w.X1}
			byLine[id] = sg
			segs = append(segs, sg)
		}
		sg.words = append(sg.words, w)
		sg.x0, sg.x1 = math.Min(sg.x0, w.X0), math.Max(sg.x1, w.X1)
	}
	for _, sg := range segs {
		sort.SliceStable(sg.words, func(a, b int) bool { return sg.words[a].X0 < sg.words[b].X0 })
		ys := make([]float64, len(sg.words))
		for i, w := range sg.words {
			ys[i] = (w.Y0 + w.Y1) / 2
		}
		sort.Float64s(ys)
		sg.y = ys[len(ys)/2]
	}
	sort.SliceStable(segs, func(i, j int) bool { return segs[i].y < segs[j].y })
	scale := tzColumnGap / h
	var lines []tzLine
	for i := 0; i < len(segs); {
		j := i + 1
		for j < len(segs) && segs[j].y-segs[i].y <= h*0.6 {
			j++
		}
		row := append([]*seg(nil), segs[i:j]...)
		sort.SliceStable(row, func(a, b int) bool { return row[a].x0 < row[b].x0 })
		// Слова строки слева направо; граница колонки — разрыв шире двух высот слова или «|» (линия таблицы,
		// распознанная как знак).
		var ws []ocr.Word
		for _, sg := range row {
			ws = append(ws, sg.words...)
		}
		sort.SliceStable(ws, func(a, b int) bool { return ws[a].X0 < ws[b].X0 })
		var cells []tzCell
		brk := true
		end := 0.0
		for _, w := range ws {
			t := strings.Trim(w.Text, "|")
			if t == "" {
				brk = true
				continue
			}
			if !brk && w.X0-end <= h*2 {
				last := &cells[len(cells)-1]
				last.text += " " + t
				last.x1 = w.X1 * scale
			} else {
				cells = append(cells, tzCell{x0: w.X0 * scale, x1: w.X1 * scale, text: t})
			}
			brk, end = false, w.X1
		}
		if len(cells) == 0 {
			i = j
			continue
		}
		lines = append(lines, tzLine{y: -segs[i].y * scale, cells: cells})
		i = j
	}
	return lines
}

// tzScanJPEGs — картинка JPEG каждой страницы (самая большая на странице); у страницы без JPEG — nil. Библиотека
// PDF не отдаёт поток DCTDecode как есть, поэтому байты картинки берутся из файла по объекту: размеры и длина
// потока — из словаря страницы через библиотеку, сам поток — по номеру объекта в файле.
func tzScanJPEGs(data []byte) (pages [][]byte, err error) {
	defer func() {
		if r := recover(); r != nil {
			pages, err = nil, tzFail(tzReasonDamaged)
		}
	}()
	r, err := pdf.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, tzFail(tzReasonDamaged)
	}
	streams := tzRawDCTStreams(data)
	n := min(r.NumPage(), tzOCRMaxPages)
	found := 0
	for i := 1; i <= n; i++ {
		p := r.Page(i)
		xo := p.Resources().Key("XObject")
		var best []byte
		bestArea := 0.0
		for _, k := range xo.Keys() {
			x := xo.Key(k)
			if x.Key("Subtype").Name() != "Image" || !tzIsDCT(x.Key("Filter")) {
				continue
			}
			area := x.Key("Width").Float64() * x.Key("Height").Float64()
			length := x.Key("Length").Int64()
			img := streams.match(length, x.Key("Width").Int64(), x.Key("Height").Int64())
			if img != nil && area > bestArea && len(img) <= tzOCRMaxImage {
				best, bestArea = img, area
			}
		}
		if best != nil {
			found++
		}
		pages = append(pages, best)
	}
	if found == 0 {
		return nil, errTZScanImages
	}
	return pages, nil
}

func tzIsDCT(f pdf.Value) bool {
	switch f.Kind() {
	case pdf.Name:
		return f.Name() == "DCTDecode"
	case pdf.Array:
		return f.Len() == 1 && f.Index(0).Name() == "DCTDecode"
	}
	return false
}

// tzDCT — поток JPEG из файла: размеры из словаря и байты.
type tzDCT struct {
	w, h int64
	data []byte
}

type tzDCTs []tzDCT

// match — поток с той же длиной (или те же размеры, если длина в словаре косвенная).
func (ds tzDCTs) match(length, w, h int64) []byte {
	for _, d := range ds {
		if int64(len(d.data)) == length && (d.w == 0 || d.w == w) {
			return d.data
		}
	}
	for _, d := range ds {
		if d.w == w && d.h == h {
			return d.data
		}
	}
	return nil
}

var (
	reObjHead = regexp.MustCompile(`(\d+)\s+\d+\s+obj\b`)
	reLength  = regexp.MustCompile(`/Length\s+(\d+)(\s+\d+\s+R)?`)
	reWidth   = regexp.MustCompile(`/Width\s+(\d+)`)
	reHeight  = regexp.MustCompile(`/Height\s+(\d+)`)
)

// tzRawDCTStreams — потоки DCTDecode в байтах файла: словарь объекта до «stream», байты — по /Length, при
// косвенной длине — до «endstream», с обрезкой до конца JPEG (FF D9).
func tzRawDCTStreams(data []byte) tzDCTs {
	var res tzDCTs
	heads := reObjHead.FindAllIndex(data, -1)
	for i, hd := range heads {
		end := len(data)
		if i+1 < len(heads) {
			end = heads[i+1][0]
		}
		obj := data[hd[1]:end]
		si := bytes.Index(obj, []byte("stream"))
		if si < 0 {
			continue
		}
		dict := obj[:si]
		if !bytes.Contains(dict, []byte("/DCTDecode")) || !bytes.Contains(dict, []byte("/Image")) {
			continue
		}
		start := si + len("stream")
		if start < len(obj) && obj[start] == '\r' {
			start++
		}
		if start < len(obj) && obj[start] == '\n' {
			start++
		}
		body := obj[start:]
		var img []byte
		if m := reLength.FindSubmatch(dict); m != nil && len(m[2]) == 0 {
			if n, err := strconv.Atoi(string(m[1])); err == nil && n > 0 && n <= len(body) {
				img = body[:n]
			}
		}
		if img == nil {
			if e := bytes.Index(body, []byte("endstream")); e > 0 {
				img = body[:e]
				if k := bytes.LastIndex(img, []byte{0xFF, 0xD9}); k > 0 {
					img = img[:k+2]
				}
			}
		}
		if len(img) < 4 || img[0] != 0xFF || img[1] != 0xD8 {
			continue
		}
		d := tzDCT{data: img}
		if m := reWidth.FindSubmatch(dict); m != nil {
			d.w, _ = strconv.ParseInt(string(m[1]), 10, 64)
		}
		if m := reHeight.FindSubmatch(dict); m != nil {
			d.h, _ = strconv.ParseInt(string(m[1]), 10, 64)
		}
		res = append(res, d)
	}
	return res
}
