package llm

import (
	"bufio"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

// Форматы ПДн, которые маскируются. Все номера вымышленные (контрольные суммы подобраны), реальных людей нет.
func TestMaskPIIFormats(t *testing.T) {
	for _, tc := range []struct{ in, want string }{
		{"перезвоните +7 (916) 123-45-67", "перезвоните " + PIIPhone},
		{"тел 8-916-123-45-67 Иван", "тел " + PIIPhone + " Иван"},
		{"89161234567", PIIPhone},
		{"+79161234567 касперский", PIIPhone + " касперский"},
		{"8 800 555 35 35", PIIPhone},
		{"(495) 123-45-67 офис", PIIPhone + " офис"},
		{"моб. 916 123-45-67", "моб. " + PIIPhone},
		{"London +44 20 7946 0958", "London " + PIIPhone},
		{"+1 (415) 555-2671", PIIPhone},
		{"+375 29 123 45 67", PIIPhone},
		{"ivan.petrov@mail.ru windows", PIIEmail + " windows"},
		{"почта закупки: zakupki@почта.рф", "почта закупки: " + PIIEmail},
		{"ИНН 7707083893 лицензии", "ИНН " + PIIINN + " лицензии"},
		{"инн:1234567890", "инн:" + PIIINN}, // после слова — без контрольной суммы
		{"7707083893", PIIINN},              // голый — по контрольной сумме
		{"500100732259", PIIINN},            // 12 цифр (ИП)
		{"КПП 773601001", "КПП " + PIIKPP},
		{"ОГРН 1027700132195", "ОГРН " + PIIOGRN},
		{"1027700132195", PIIOGRN},
		{"ОГРНИП 304500116000157", "ОГРНИП " + PIIOGRN},
		{"карта 4111 1111 1111 1111", "карта " + PIICard},
		{"4111-1111-1111-1111", PIICard},
		{"5555555555554444", PIICard},
		{"СНИЛС 112-233-445 95", "СНИЛС " + PIISNILS},
		{"11223344595", PIISNILS},
		{"паспорт 4510 123456", "паспорт " + PIIPassport},
		{"паспорта 4510123456", "паспорта " + PIIPassport},
		{"45 10 123456", PIIPassport},
		{"office 2021, звонить 8(916)1234567, почта a@b.io",
			"office 2021, звонить " + PIIPhone + ", почта " + PIIEmail},
	} {
		got, n := MaskPII(tc.in)
		if got != tc.want || n == 0 {
			t.Errorf("MaskPII(%q) = %q (%d), want %q", tc.in, got, n, tc.want)
		}
		if again, n2 := MaskPII(got); again != got || n2 != 0 {
			t.Errorf("повторное маскирование %q изменило текст: %q (%d)", got, again, n2)
		}
	}
}

// Обычные запросы и тексты каталога не трогаются: годы, версии, артикулы, цены, количества.
func TestMaskPIIKeepsCatalogText(t *testing.T) {
	for _, s := range []string{
		"windows server 2025", "fqc10572", "dr.web для бизнеса", "Dell R740 2x16 ГБ 3200",
		"office 2021 на 25 пользователей", "123 456 ₽", "1С:Предприятие 8.3", "8 ядер 16 потоков",
		"касперский 1 год 10 устройств", "SKU-1234567890", "ABC5555555555554444", "версия 7.0.1.2025",
		"RTX 4090 24 ГБ", "4601546482049", "4601546941676", "сервер на 2 процессора 512 ГБ", "1234567890", "12345678901", "10 000 000 руб",
	} {
		if got, n := MaskPII(s); got != s || n != 0 {
			t.Errorf("MaskPII(%q) = %q (%d): ложное срабатывание", s, got, n)
		}
	}
}

// Ложных срабатываний на mock-каталоге, наборах запросов, событиях и промптах нет: агент шлёт модели карточки и
// тексты запросов, разбор — справочники; маскирование не должно их портить.
func TestMaskPIINoFalsePositivesOnRepoData(t *testing.T) {
	root := filepath.Join("..", "..", "..")
	files := []string{
		"data/mock/catalog.json", "data/mock/queries.json", "data/mock/aliases.json", "data/mock/categories.json",
		"data/mock/attribute_definitions.json", "data/mock/relations.json", "data/mock/license_rules.json",
		"data/mock/compatibility_rules.json", "data/mock/showcase.json", "data/mock/help.json",
		"data/mock/prefixes.json", "data/mock/reestr.json", "data/mock/companion_pairs.json",
		"data/mock/events.jsonl",
	}
	prompts, _ := filepath.Glob(filepath.Join(root, "backend", "prompts", "*.md"))
	checked := 0
	for _, f := range files {
		checked += scanNoPII(t, filepath.Join(root, f))
	}
	for _, p := range prompts {
		checked += scanNoPII(t, p)
	}
	if checked == 0 {
		t.Skip("данные репозитория не найдены")
	}
}

func scanNoPII(t *testing.T, path string) int {
	t.Helper()
	f, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer f.Close()
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 16<<20)
	lines, hits := 0, 0
	for sc.Scan() {
		lines++
		line := sc.Text()
		if got, n := MaskPII(line); n > 0 {
			hits++
			if hits <= 5 {
				t.Errorf("%s:%d: ложное срабатывание: %q → %q", filepath.Base(path), lines, clip(line), clip(got))
			}
		}
	}
	if hits > 5 {
		t.Errorf("%s: всего строк с ложным срабатыванием: %d", filepath.Base(path), hits)
	}
	return lines
}

func clip(s string) string {
	if r := []rune(s); len(r) > 160 {
		return string(r[:160]) + "…"
	}
	return s
}

// Тело запроса к модели (подменный сервер) не содержит телефона, почты, ИНН и карты — ни в тексте сообщений,
// ни в аргументах вызовов инструментов; исходные сообщения вызывающего не меняются; число замен — в ответе.
func TestChatMasksPIIInRequestBody(t *testing.T) {
	var body string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		raw, _ := io.ReadAll(r.Body)
		body = string(raw)
		_, _ = w.Write([]byte(`{"choices":[{"finish_reason":"stop","message":{"role":"assistant","content":"{}"}}]}`))
	}))
	defer srv.Close()

	msgs := []Message{
		{Role: "system", Content: "Справочник: Windows Server 2025, артикул fqc10572"},
		{Role: "user", Content: `<query>"касперский 8-916-123-45-67 ivan.petrov@mail.ru ИНН 7707083893"</query>`},
		{Role: "assistant", ToolCalls: []ToolCall{{ID: "c1", Type: "function",
			Function: FunctionCall{Name: "search_catalog", Arguments: `{"query":"+7 916 123 45 67 карта 4111 1111 1111 1111"}`}}}},
		{Role: "tool", ToolCallID: "c1", Content: `{"queries":[{"q":"windows 8 916 1234567","n":3}]}`},
	}
	orig := msgs[1].Content
	resp, err := New(srv.URL, "m", "", "k", time.Second).Chat(context.Background(), Request{Messages: msgs})
	if err != nil {
		t.Fatal(err)
	}
	var decoded struct {
		Messages []Message `json:"messages"`
	}
	if err := json.Unmarshal([]byte(body), &decoded); err != nil {
		t.Fatal(err)
	}
	var sent strings.Builder
	for _, m := range decoded.Messages {
		sent.WriteString(m.Content + "\n")
		for _, c := range m.ToolCalls {
			sent.WriteString(c.Function.Arguments + "\n")
		}
	}
	text := sent.String()
	for _, leak := range []string{"916", "123-45-67", "ivan.petrov", "@mail.ru", "7707083893", "4111"} {
		if strings.Contains(text, leak) {
			t.Errorf("в теле запроса к модели осталось %q:\n%s", leak, text)
		}
	}
	for _, keep := range []string{"Windows Server 2025", "fqc10572", "касперский", PIIPhone, PIIEmail, PIIINN, PIICard} {
		if !strings.Contains(text, keep) {
			t.Errorf("в теле нет %q:\n%s", keep, text)
		}
	}
	if resp.PIIMasked != 6 {
		t.Errorf("PIIMasked = %d, want 6", resp.PIIMasked)
	}
	if msgs[1].Content != orig || !strings.Contains(msgs[2].ToolCalls[0].Function.Arguments, "4111") {
		t.Error("исходные сообщения вызывающего изменены")
	}
}
