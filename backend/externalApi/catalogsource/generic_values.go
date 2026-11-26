package catalogsource

import (
	"fmt"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode"
)

// Разбор значений ячеек: цены «12 990,00 ₽», даты в разных форматах, «да/нет», сроки «1 год», id-слаги.

// fold — ключ сравнения: нижний регистр, ё → е, один пробел.
func fold(s string) string {
	s = strings.ReplaceAll(strings.ToLower(s), "ё", "е")
	return strings.Join(strings.Fields(s), " ")
}

// onRequestWords — цена не числом, а «по запросу».
var onRequestWords = []string{"запрос", "договор", "уточня", "request", "call", "звоните"}

// parsePrice — рубли из «12 990,00 ₽», «12990.5», «1.234.567», «$1,299.00». onRequest — «по запросу».
// ok=false — текст не цена.
func parsePrice(s string) (value float64, onRequest, ok bool) {
	f := fold(s)
	if f == "" || f == "-" || f == "—" {
		return 0, false, false
	}
	for _, w := range onRequestWords {
		if strings.Contains(f, w) {
			return 0, true, true
		}
	}
	var b strings.Builder
	for _, r := range f {
		if unicode.IsDigit(r) || r == ',' || r == '.' || r == '-' {
			b.WriteRune(r)
		}
	}
	n := strings.Trim(b.String(), ".,")
	if n == "" {
		return 0, false, false
	}
	n = normalizeDecimal(n)
	v, err := strconv.ParseFloat(n, 64)
	if err != nil || v < 0 {
		return 0, false, false
	}
	if v == 0 {
		return 0, true, true // 0 ₽ в прайсе — почти всегда «по запросу»
	}
	return v, false, true
}

// normalizeDecimal — один десятичный разделитель «.»: последний из «,» и «.», если за ним 1–2 цифры
// (или он единственный и разделители разные); иначе разделители — тысячные.
func normalizeDecimal(n string) string {
	lastComma, lastDot := strings.LastIndex(n, ","), strings.LastIndex(n, ".")
	last := max(lastComma, lastDot)
	if last < 0 {
		return n
	}
	tail := len(n) - last - 1
	sep := n[last]
	count := strings.Count(n, string(sep))
	decimal := false
	switch {
	case lastComma >= 0 && lastDot >= 0:
		decimal = true // «1,299.00» и «1.299,00»: последний — десятичный
	case count == 1 && tail != 3:
		decimal = true // «12990,5», «12.99»
	}
	if !decimal {
		return strings.NewReplacer(",", "", ".", "").Replace(n)
	}
	head := strings.NewReplacer(",", "", ".", "").Replace(n[:last])
	return head + "." + n[last+1:]
}

// parseBool — «да/нет», true/false, 1/0, +/-, «есть»; ok=false — не понято.
func parseBool(s string) (bool, bool) {
	switch fold(s) {
	case "да", "д", "yes", "y", "true", "1", "+", "есть", "истина", "v", "✓", "x", "в реестре", "включен":
		return true, true
	case "нет", "н", "no", "n", "false", "0", "-", "—", "ложь", "не включен", "отсутствует":
		return false, true
	}
	return false, false
}

// parseNumber — первое число в тексте: «10 ПК» → 10, «0,72» → 0.72.
func parseNumber(s string) (float64, bool) {
	start := -1
	rs := []rune(s)
	for i, r := range rs {
		if unicode.IsDigit(r) {
			start = i
			break
		}
	}
	if start < 0 {
		return 0, false
	}
	end := start
	for end < len(rs) && (unicode.IsDigit(rs[end]) || rs[end] == ',' || rs[end] == '.' || rs[end] == ' ' || rs[end] == ' ') {
		end++
	}
	n := strings.NewReplacer(" ", "", " ", "").Replace(string(rs[start:end]))
	n = strings.Trim(n, ".,")
	v, err := strconv.ParseFloat(normalizeDecimal(n), 64)
	if err != nil {
		return 0, false
	}
	if start > 0 && rs[start-1] == '-' {
		v = -v
	}
	return v, true
}

func parseInt(s string) (int, bool) {
	v, ok := parseNumber(s)
	if !ok || v > math.MaxInt32 {
		return 0, false
	}
	return int(math.Round(v)), true
}

// parseDurationMonths — срок: «12» (месяцы), «1 год», «2 года», «6 мес», «бессрочно» (nil, true).
func parseDurationMonths(s string) (*int, bool) {
	f := fold(s)
	if f == "" {
		return nil, false
	}
	if strings.Contains(f, "бессроч") || strings.Contains(f, "perpetual") || strings.Contains(f, "навсегда") {
		return nil, true
	}
	n, ok := parseInt(f)
	if !ok {
		if strings.Contains(f, "год") || strings.Contains(f, "year") {
			n, ok = 1, true
		} else {
			return nil, false
		}
	}
	switch {
	case strings.Contains(f, "год") || strings.Contains(f, "лет") || strings.Contains(f, "year"):
		n *= 12
	case strings.Contains(f, "дн") || strings.Contains(f, "day"):
		n = max(1, n/30)
	}
	if n <= 0 {
		return nil, false
	}
	return &n, true
}

// baseDateFormats — встроенные форматы дат; mapping.date_formats добавляются впереди.
var baseDateFormats = []string{
	time.RFC3339Nano, "2006-01-02T15:04:05", "2006-01-02T15:04", "2006-01-02 15:04:05", "2006-01-02 15:04",
	"2006-01-02", "02.01.2006 15:04:05", "02.01.2006 15:04", "02.01.2006", "02.01.06 15:04", "02.01.06",
	"02/01/2006 15:04:05", "02/01/2006 15:04", "02/01/2006", "2006/01/02 15:04:05", "2006/01/02",
	"2006-01-02 15:04:05.000", "2006-01-02 15:04:05-07", "2006-01-02 15:04:05-07:00",
}

// timeParser — даты по форматам в поясе по умолчанию; числа — unix-секунды, миллисекунды или серийная
// дата Excel.
type timeParser struct {
	formats []string
	loc     *time.Location
}

func newTimeParser(extra []string, tz string) (*timeParser, error) {
	if tz == "" {
		tz = "Europe/Moscow"
	}
	loc, err := time.LoadLocation(tz)
	if err != nil {
		// В минимальном образе без tzdata Москва — фиксированный UTC+3.
		if tz != "Europe/Moscow" {
			return nil, fmt.Errorf("timezone %q: %w", tz, err)
		}
		loc = time.FixedZone("MSK", 3*3600)
	}
	return &timeParser{formats: append(append([]string{}, extra...), baseDateFormats...), loc: loc}, nil
}

func (p *timeParser) parse(s string) (time.Time, bool) {
	s = strings.TrimSpace(s)
	if s == "" {
		return time.Time{}, false
	}
	if v, err := strconv.ParseFloat(strings.Replace(s, ",", ".", 1), 64); err == nil {
		switch {
		case v > 1e11: // миллисекунды
			return time.UnixMilli(int64(v)).UTC(), true
		case v > 1e8: // секунды
			return time.Unix(int64(v), 0).UTC(), true
		case v > 20000 && v < 80000: // серийная дата Excel (1954–2119)
			base := time.Date(1899, 12, 30, 0, 0, 0, 0, p.loc)
			return base.Add(time.Duration(v * 24 * float64(time.Hour))).Round(time.Second).UTC(), true
		}
		return time.Time{}, false
	}
	for _, f := range p.formats {
		if t, err := time.ParseInLocation(f, s, p.loc); err == nil {
			return t.UTC(), true
		}
	}
	return time.Time{}, false
}

// translit — латиница для id: «МойОфис» → «moyofis».
var translit = map[rune]string{
	'а': "a", 'б': "b", 'в': "v", 'г': "g", 'д': "d", 'е': "e", 'ё': "e", 'ж': "zh", 'з': "z", 'и': "i",
	'й': "y", 'к': "k", 'л': "l", 'м': "m", 'н': "n", 'о': "o", 'п': "p", 'р': "r", 'с': "s", 'т': "t",
	'у': "u", 'ф': "f", 'х': "h", 'ц': "ts", 'ч': "ch", 'ш': "sh", 'щ': "sch", 'ъ': "", 'ы': "y", 'ь': "",
	'э': "e", 'ю': "yu", 'я': "ya",
}

// Slug — id из названия: латиница, цифры и дефисы, не длиннее 80 символов.
func Slug(s string) string {
	var b strings.Builder
	dash := false
	for _, r := range strings.ToLower(s) {
		var piece string
		switch {
		case r < 128 && (unicode.IsLetter(r) || unicode.IsDigit(r)):
			piece = string(r)
		case translit[r] != "" || r == 'ъ' || r == 'ь':
			piece = translit[r]
		case r == '+':
			piece = "plus"
		default:
			if b.Len() > 0 {
				dash = true
			}
			continue
		}
		if piece == "" {
			continue
		}
		if dash {
			b.WriteByte('-')
			dash = false
		}
		b.WriteString(piece)
	}
	out := b.String()
	if len(out) > 80 {
		out = strings.TrimRight(out[:80], "-")
	}
	return out
}
