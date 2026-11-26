package llm

import (
	"regexp"
	"strings"
	"unicode/utf8"
)

// Маскирование персональных данных перед внешней моделью (контур доводки, хвосты). Client.Chat прогоняет через MaskPII
// каждое сообщение и аргументы вызовов инструментов — это граница процесса: разбор намерения, агент, судья и
// классификатор наборов идут через неё, и ни один вызывающий не может её обойти, если ходит через Client.
//
// Что ищется: телефоны (RU и международные с «+»), e-mail, ИНН (10 и 12 цифр, с контрольной суммой или после слова
// «ИНН»), КПП (после слова «КПП»), ОГРН/ОГРНИП (с контрольной суммой или после слова), номера карт (13–19 цифр,
// алгоритм Луна), СНИЛС (формат 000-000-000 00 или 11 цифр с контрольной суммой), паспорт РФ (серия и номер
// «45 10 123456» / «4510 123456» или 10 цифр после слова «паспорт»). Найденное заменяется меткой: «[телефон]»,
// «[email]», «[инн]», «[кпп]», «[огрн]», «[карта]», «[снилс]», «[паспорт]». Голые числа без формата и без
// контрольной суммы не трогаются: годы, версии, артикулы и цены каталога остаются (тест по каталогу и наборам).

// Метки замены.
const (
	PIIPhone    = "[телефон]"
	PIIEmail    = "[email]"
	PIIINN      = "[инн]"
	PIIKPP      = "[кпп]"
	PIIOGRN     = "[огрн]"
	PIICard     = "[карта]"
	PIISNILS    = "[снилс]"
	PIIPassport = "[паспорт]"
)

type piiRule struct {
	re *regexp.Regexp
	// group — номер группы, которая заменяется (0 — всё совпадение).
	group int
	// label — метка; пусто — метку выбирает classify.
	label string
	// classify — проверка и метка по найденному тексту группы; "" — не ПДн, оставить.
	classify func(keyword, m string) string
}

var (
	reEmail = regexp.MustCompile(`[\pL\d._%+\-]+@[\pL\d\-]+(?:\.[\pL\d\-]+)*\.\pL{2,}`)
	// Слово-признак и номер за ним: «ИНН 7707083893», «паспорт: 4510 123456», «КПП 773601001».
	reKeyword = regexp.MustCompile(`(?i)(инн|кпп|огрнип|огрн|снилс|паспорт[а-яё]*)\s*(?:[:№#]|no\.?)?\s*(\d[\d \-]{7,18}[\dA-Z]|\d{4}[\dA-Z]{2}\d{3})`)
	// Карта: 13–19 цифр подряд или группами по 4 через пробел или дефис.
	reCard = regexp.MustCompile(`\d{4}(?:[ \-]\d{4}){2}[ \-]\d{1,7}|\d{13,19}`)
	// СНИЛС в своём формате.
	reSNILSFmt = regexp.MustCompile(`\d{3}-\d{3}-\d{3}[ \-]\d{2}`)
	// Телефон РФ: +7 / 8 / 7, код из трёх цифр (в скобках или нет), номер 3-2-2 с разделителями или без.
	rePhoneRU = regexp.MustCompile(`(?:\+7|8|7)[ \-]?(?:\(\d{3}\)|\d{3})[ \-]?\d{3}[ \-]?\d{2}[ \-]?\d{2}`)
	// Код города или оператора в скобках без префикса: «(495) 123-45-67», «(9162) 12-34-56».
	rePhoneParen = regexp.MustCompile(`\(\d{3,5}\)[ \-]?\d{1,3}[ \-]?\d{2}[ \-]?\d{2}`)
	// Мобильный без префикса, только с разделителями: «916 123-45-67».
	rePhoneMobile = regexp.MustCompile(`9\d{2}[ \-]\d{3}[ \-]\d{2}[ \-]\d{2}`)
	// Международный: «+» и код страны, дальше группы цифр (всего 8–15 цифр).
	rePhoneIntl = regexp.MustCompile(`\+\d{1,3}(?:[ \-]?\(\d{1,4}\))?(?:[ \-]?\d{1,4}){2,5}`)
	// Голые 10–15 цифр: ИНН, СНИЛС, ОГРН — только с верной контрольной суммой.
	reBareDigits = regexp.MustCompile(`\d{10,15}`)
	// Паспорт РФ: серия «45 10» или «4510», пробел, номер из шести цифр.
	rePassport = regexp.MustCompile(`\d{2} ?\d{2} \d{6}`)
)

var piiRules = []piiRule{
	{re: reEmail, label: PIIEmail},
	{re: reKeyword, group: 2, classify: classifyKeyword},
	{re: reSNILSFmt, label: PIISNILS},
	{re: rePhoneRU, label: PIIPhone},
	{re: rePhoneIntl, classify: func(_, m string) string { return ifDigits(m, 8, 15, PIIPhone) }},
	{re: rePhoneParen, classify: func(_, m string) string { return ifDigits(m, 7, 12, PIIPhone) }},
	{re: rePhoneMobile, label: PIIPhone},
	{re: reCard, classify: classifyCard},
	{re: reBareDigits, classify: classifyBare},
	{re: rePassport, label: PIIPassport},
}

// MaskPII — текст с ПДн, заменёнными метками, и число замен. Повторный вызов на результате ничего не меняет.
func MaskPII(s string) (string, int) {
	if s == "" || !strings.ContainsAny(s, "0123456789@") {
		return s, 0
	}
	total := 0
	for _, r := range piiRules {
		var n int
		s, n = r.apply(s)
		total += n
	}
	return s, total
}

func (r piiRule) apply(s string) (string, int) {
	locs := r.re.FindAllStringSubmatchIndex(s, -1)
	if len(locs) == 0 {
		return s, 0
	}
	var b strings.Builder
	last, n := 0, 0
	for _, loc := range locs {
		start, end := loc[2*r.group], loc[2*r.group+1]
		if start < 0 || !bounded(s, loc[0], loc[1]) {
			continue
		}
		label := r.label
		if r.classify != nil {
			kw := ""
			if r.group > 0 {
				kw = s[loc[2]:loc[3]]
			}
			label = r.classify(kw, s[start:end])
		}
		if label == "" {
			continue
		}
		b.WriteString(s[last:start])
		b.WriteString(label)
		last = end
		n++
	}
	if n == 0 {
		return s, 0
	}
	b.WriteString(s[last:])
	return b.String(), n
}

// bounded — совпадение не продолжает слово или число: до и после — не латинская буква, не цифра и не «+».
// Кириллица рядом допустима («инн7707083893»), латиница — нет (артикулы вида fqc10572).
func bounded(s string, start, end int) bool {
	if start > 0 {
		r, _ := utf8.DecodeLastRuneInString(s[:start])
		if isASCIIAlnum(r) || (r == '+' && s[start] != '+') {
			return false
		}
	}
	if end < len(s) {
		r, _ := utf8.DecodeRuneInString(s[end:])
		if isASCIIAlnum(r) {
			return false
		}
	}
	return true
}

func isASCIIAlnum(r rune) bool {
	return r < utf8.RuneSelf && (r >= '0' && r <= '9' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z')
}

func digitsOf(s string) string {
	var b strings.Builder
	for i := 0; i < len(s); i++ {
		if s[i] >= '0' && s[i] <= '9' {
			b.WriteByte(s[i])
		}
	}
	return b.String()
}

func ifDigits(m string, lo, hi int, label string) string {
	if n := len(digitsOf(m)); n >= lo && n <= hi {
		return label
	}
	return ""
}

// classifyKeyword — номер после слова-признака: длина должна подходить к документу, контрольная сумма не нужна.
func classifyKeyword(keyword, m string) string {
	kw := strings.ToLower(keyword)
	d := digitsOf(m)
	switch {
	case kw == "инн" && (len(d) == 10 || len(d) == 12):
		return PIIINN
	case kw == "кпп" && len(m) == 9:
		return PIIKPP
	case (kw == "огрн" && len(d) == 13) || (kw == "огрнип" && len(d) == 15):
		return PIIOGRN
	case kw == "снилс" && len(d) == 11:
		return PIISNILS
	case strings.HasPrefix(kw, "паспорт") && len(d) == 10:
		return PIIPassport
	}
	return ""
}

// classifyCard — номер карты по алгоритму Луна; первая цифра 2–6 (платёжные системы); группы — только по четыре
// цифры. Голые 13–15 цифр картой не считаются: это штрихкоды EAN-13 в артикулах каталога (4601546…) и ОГРН —
// их разбирает classifyBare.
func classifyCard(_, m string) string {
	d := digitsOf(m)
	if len(d) < 13 || len(d) > 19 || d[0] < '2' || d[0] > '6' {
		return ""
	}
	if d == m && len(d) < 16 {
		return ""
	}
	if luhnOK(d) {
		return PIICard
	}
	return ""
}

// classifyBare — голые 10–15 цифр: ИНН, СНИЛС, ОГРН по контрольной сумме; иначе не ПДн (артикул, штрихкод).
func classifyBare(_, d string) string {
	switch len(d) {
	case 10, 12:
		if innOK(d) {
			return PIIINN
		}
	case 11:
		if snilsOK(d) {
			return PIISNILS
		}
	case 13:
		if ogrnOK(d) {
			return PIIOGRN
		}
	case 15:
		if ogrnipOK(d) {
			return PIIOGRN
		}
	}
	return ""
}

func dig(d string, i int) int { return int(d[i] - '0') }

func innOK(d string) bool {
	check := func(n int, w []int) bool {
		sum := 0
		for i, k := range w {
			sum += k * dig(d, i)
		}
		return sum%11%10 == dig(d, n)
	}
	switch len(d) {
	case 10:
		return check(9, []int{2, 4, 10, 3, 5, 9, 4, 6, 8})
	case 12:
		return check(10, []int{7, 2, 4, 10, 3, 5, 9, 4, 6, 8}) && check(11, []int{3, 7, 2, 4, 10, 3, 5, 9, 4, 6, 8})
	}
	return false
}

func snilsOK(d string) bool {
	if len(d) != 11 {
		return false
	}
	sum := 0
	for i := 0; i < 9; i++ {
		sum += dig(d, i) * (9 - i)
	}
	ctl := sum % 101
	if ctl == 100 {
		ctl = 0
	}
	return ctl == dig(d, 9)*10+dig(d, 10)
}

// modTail — остаток от деления числа из первых n цифр на m (по цифре, без переполнения).
func modTail(d string, n, m int) int {
	r := 0
	for i := 0; i < n; i++ {
		r = (r*10 + dig(d, i)) % m
	}
	return r
}

// ogrnOK — ОГРН юрлица: первая цифра 1 или 5 (признак записи), сумма по модулю 11. Первая цифра отсекает штрихкоды
// EAN-13 российских товаров (начинаются с 46).
func ogrnOK(d string) bool {
	return len(d) == 13 && (d[0] == '1' || d[0] == '5') && modTail(d, 12, 11)%10 == dig(d, 12)
}

// ogrnipOK — ОГРНИП: первая цифра 3, сумма по модулю 13.
func ogrnipOK(d string) bool {
	return len(d) == 15 && d[0] == '3' && modTail(d, 14, 13)%10 == dig(d, 14)
}

func luhnOK(d string) bool {
	sum, alt := 0, false
	for i := len(d) - 1; i >= 0; i-- {
		n := dig(d, i)
		if alt {
			n *= 2
			if n > 9 {
				n -= 9
			}
		}
		sum += n
		alt = !alt
	}
	return sum%10 == 0
}

// maskMessages — копия сообщений с замаскированным текстом и аргументами вызовов; исходный срез не меняется
// (агент держит историю диалога у себя). Второе значение — число замен.
func maskMessages(msgs []Message) ([]Message, int) {
	out := make([]Message, len(msgs))
	total := 0
	for i, m := range msgs {
		var n int
		m.Content, n = MaskPII(m.Content)
		total += n
		if len(m.ToolCalls) > 0 {
			calls := make([]ToolCall, len(m.ToolCalls))
			for j, c := range m.ToolCalls {
				c.Function.Arguments, n = MaskPII(c.Function.Arguments)
				total += n
				calls[j] = c
			}
			m.ToolCalls = calls
		}
		out[i] = m
	}
	return out, total
}
