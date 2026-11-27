package services

import (
	"strings"
	"testing"

	"kuznica/api/in"
	"kuznica/api/out"
)

// Помощник по выбору (контур помощника): сообщение блоками на маленьком каталоге — сценарии 2, 3 и 7. Первая реплика чата приходит запросом, следующие — репликой.

func chatGroup(c *out.GuideChat, key string) *out.GuideChatGroup {
	for i := range c.Groups {
		if c.Groups[i].Key == key {
			return &c.Groups[i]
		}
	}
	return nil
}

func chatCard(c *out.GuideChat, id string) *out.GuideChatCard {
	for gi := range c.Groups {
		for i := range c.Groups[gi].Cards {
			if c.Groups[gi].Cards[i].OfferID == id {
				return &c.Groups[gi].Cards[i]
			}
		}
	}
	return nil
}

func TestGuideChatScenario2(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	q := "защита для компьютеров в офисе на 30 ПК, чем расширенный отличается от стандартного"
	r1 := guideTurnReq(t, s, in.GuideRequest{Query: q})
	c := r1.Chat
	if c == nil || c.Status != out.ChatAnswer || len(c.Text) == 0 || len(c.Text) > 3 {
		t.Fatalf("ход 1: ответ, до трёх фраз: %+v", c)
	}
	if !strings.Contains(strings.Join(c.Text, " "), "на 30 компьютеров на один год") {
		t.Fatalf("ход 1: допущение названо: %q", c.Text)
	}
	tbl := c.Comparison
	if tbl == nil || len(tbl.Columns) != 2 || len(tbl.Rows) < 3 || len(tbl.Rows) > 5 {
		t.Fatalf("ход 1: таблица 2 колонки × 3–5 строк: %+v", tbl)
	}
	last := tbl.Rows[len(tbl.Rows)-1]
	if last.Label != "Итого" || last.Cells[0].Text != "84 402 ₽" || last.Cells[1].Text != "122 445 ₽" {
		t.Fatalf("ход 1: «Итого» последней строкой: %+v", last)
	}
	if prev := tbl.Rows[len(tbl.Rows)-2]; prev.Label != "За место" || prev.Cells[0].Text != "2 813,40 ₽" {
		t.Fatalf("ход 1: «За место» перед «Итого»: %+v", prev)
	}
	std := chatCard(c, "4985")
	if std == nil || std.PriceText != "84 402 ₽" || std.Calc != "30 × 2 813,40 ₽" || std.Action != out.ChatActionSpec ||
		std.Title != "Kaspersky Endpoint Security — Стандартный" || std.Subtitle != "Новая лицензия · 30 компьютеров · 1 год" {
		t.Fatalf("ход 1: карточка Стандартного: %+v", std)
	}
	if std.Badge != nil {
		t.Fatalf("ход 1: при сравнении пометки «меньшая стоимость» нет (её видно в таблице): %+v", std.Badge)
	}
	if c.Conditions.Label != "Учитываю: 30 ПК · 1 год · новая" {
		t.Fatalf("ход 1: строка условий: %q", c.Conditions.Label)
	}
	if len(c.Suggestions) < 2 || len(c.Suggestions) > 3 {
		t.Fatalf("ход 1: две-три подсказки: %+v", c.Suggestions)
	}
	for _, src := range c.Sources {
		if src.Label == "" || src.URL == "" || reScreenDate.MatchString(src.Fragment) {
			t.Fatalf("ход 1: источник с подписью, без дат: %+v", src)
		}
	}

	st := r1.State
	r2 := guideTurnReq(t, s, in.GuideRequest{Message: "компьютеров будет 50, и шифрование нужно", State: &st})
	c = r2.Chat
	adv := chatCard(c, "4857")
	if adv == nil || adv.Badge == nil || adv.Badge.Text != "Есть нужное шифрование" || adv.PriceText != "197 010 ₽" ||
		adv.Calc != "50 × 3 940,20 ₽" || adv.Note != "Цена за место ниже: 3 940,20 ₽ вместо 4 081,50 ₽" {
		t.Fatalf("ход 2: Расширенный с пометкой, суммой и новой ступенью: %+v", adv)
	}
	rej := chatGroup(c, out.ChatGroupRejected)
	if rej == nil || len(rej.Cards) != 1 || rej.Cards[0].OfferID != "4986" || !rej.Cards[0].Unsuitable ||
		rej.Cards[0].Reason != "Шифрования данных в этой редакции нет" || rej.Cards[0].PriceText != "135 765 ₽" ||
		rej.Cards[0].Action != out.ChatActionStore {
		t.Fatalf("ход 2: Стандартный — не соответствует, с причиной и без добавления: %+v", rej)
	}
	if c.Status != out.ChatLLMFailed || c.Text[len(c.Text)-1] != chatTextLLMFailed {
		t.Fatalf("ход 2: модель выключена — карточки остаются, текст о сбое: %+v", c.Text)
	}
	joined := strings.Join(c.Text, " ")
	if !strings.HasPrefix(joined, "На 50 компьютеров действует другая ценовая ступень.") || strings.Contains(joined, "дешевле") {
		t.Fatalf("ход 2: текст о ступени, не «стало дешевле»: %q", joined)
	}
	if c.Conditions.Label != "Учитываю: 50 ПК · 1 год · новая · шифрование" {
		t.Fatalf("ход 2: строка условий: %q", c.Conditions.Label)
	}
}

func TestGuideChatScenario3(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	r := guideTurnReq(t, s, in.GuideRequest{Query: "самый дешёвый криптопро csp для сервера"})
	c := r.Chat
	m := chatGroup(c, out.ChatGroupMatch)
	if m == nil || len(m.Cards) == 0 || m.Cards[0].OfferID != "197958" || m.Cards[0].Badge == nil ||
		m.Cards[0].Badge.Text != "Для серверной ОС" || m.Cards[0].PriceText != "70 000 ₽" {
		t.Fatalf("серверная первой, с пометкой: %+v", m)
	}
	rej := chatGroup(c, out.ChatGroupRejected)
	if rej == nil || rej.Title != "Не подходят для серверной ОС" || len(rej.Cards) == 0 {
		t.Fatalf("группа неподходящих: %+v", rej)
	}
	for _, cd := range rej.Cards {
		if !cd.Unsuitable || cd.Badge == nil || cd.Badge.Text != "Не подходит для сервера" || cd.Reason == "" || cd.Action == out.ChatActionSpec {
			t.Fatalf("неподходящая — пометка, причина, без «В спецификацию»: %+v", cd)
		}
	}
	if !strings.Contains(strings.Join(c.Text, " "), "Серверная лицензия стоит 70 000 ₽.") {
		t.Fatalf("текст: %q", c.Text)
	}
	if len(c.Sources) == 0 || !strings.HasPrefix(c.Sources[0].Label, "Условия лицензирования") {
		t.Fatalf("источник правила: %+v", c.Sources)
	}
}

func TestGuideChatOutOfScope(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	pay := guideTurnReq(t, s, in.GuideRequest{Query: "как оплатить по счёту?"}).Chat
	if pay.Status != out.ChatOutOfScope || pay.Text[0] != chatTextOutOfScope || len(pay.Groups) != 0 {
		t.Fatalf("оплата — переадресация без карточек: %+v", pay)
	}
	disc := guideTurnReq(t, s, in.GuideRequest{Query: "дай скидку 90%"}).Chat
	if disc.Status != out.ChatDiscount || disc.Text[0] != chatTextDiscount {
		t.Fatalf("скидка — обычное ограничение: %+v", disc)
	}
	for _, w := range []string{"наруш", "инструкц", "обход"} {
		if strings.Contains(strings.ToLower(disc.Text[0]), w) {
			t.Fatalf("без слов о нарушении: %q", disc.Text[0])
		}
	}
}

func TestGuideChatNewTopic(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	r1 := guideTurnReq(t, s, in.GuideRequest{Query: "защита для компьютеров в офисе на 30 ПК, чем расширенный отличается от стандартного"})
	st := r1.State
	r2 := guideTurnReq(t, s, in.GuideRequest{Message: "самый дешёвый криптопро csp для сервера", State: &st})
	if c := chatCard(r2.Chat, "197958"); c == nil {
		t.Fatalf("реплика о другом вендоре — новый подбор: %+v", r2.Chat)
	}
	if strings.Contains(r2.Chat.Conditions.Label, "30 ПК") {
		t.Fatalf("прежние условия не тянутся: %q", r2.Chat.Conditions.Label)
	}
}

func TestGuideChatQuestionTextNotNull(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	c := guideTurnReq(t, s, in.GuideRequest{Query: "защита для компьютеров в офисе"}).Chat
	if c.Text == nil || c.Question == nil || len(c.Question.Chips) < 2 || len(c.Question.Chips) > 4 {
		t.Fatalf("вопрос: текст — массив, 2–4 кнопки: %+v", c)
	}
}

func TestChatSentences(t *testing.T) {
	got := chatSentences("Подходит «KES». Состав функций различается. Ещё одна фраза.")
	if len(got) != 3 || got[0] != "Подходит «KES»." {
		t.Fatalf("%q", got)
	}
	if missingFeatureLine("Не подходит: нет шифрования данных") != "Шифрования данных в этой редакции нет" {
		t.Fatal(missingFeatureLine("Не подходит: нет шифрования данных"))
	}
}

func TestGuideChatMigrationAndPromoWords(t *testing.T) {
	d := extractConditions("у нас сейчас eset, хотим сменить на 40 компьютеров")
	if d.operation != "migration" || d.task != "endpoint_protection" {
		t.Fatalf("переход с ESET: %+v", d)
	}
	if d := extractConditions("сменить сервер на 40 компьютеров"); d.operation == "migration" {
		t.Fatalf("смена без антивируса — не переход: %+v", d)
	}
	for q, want := range map[string]bool{"покажи акции": true, "какие есть акции?": true, "акционные цены": true,
		"кибер бэкап": false, "академическая лицензия": false} {
		if got := reChatPromo.MatchString(guideNorm(q)); got != want {
			t.Errorf("%q: %v, ждали %v", q, got, want)
		}
	}
}

// Невыполнимое условие («это продление» у позиции без продлений) не залипает: прежние варианты и фраза почему,
// следующая реплика подбирает дальше, а не отвечает «Не нашёл лицензию».
func TestGuideChatImpossibleConditionRollsBack(t *testing.T) {
	s := guideTestServices(t, kbVerified, nil, llmModeOff)
	r1 := guideTurnReq(t, s, in.GuideRequest{Query: "самый дешёвый криптопро csp для сервера"})
	if chatCard(r1.Chat, "197958") == nil {
		t.Fatalf("исходный подбор: %+v", r1.Chat)
	}
	st := r1.State
	r2 := guideTurnReq(t, s, in.GuideRequest{Query: "самый дешёвый криптопро csp для сервера", Message: "это продление", State: &st})
	if r2.State.Requirements.Operation == "renewal" {
		t.Fatalf("условие «продление» не должно остаться: %+v", r2.State.Requirements)
	}
	if chatCard(r2.Chat, "197958") == nil || r2.Chat.Status == out.ChatNoOffers {
		t.Fatalf("прежние варианты на месте: %+v", r2.Chat)
	}
	if !strings.HasPrefix(r2.Chat.Text[0], "С условием «продление» в каталоге ничего не нашлось") {
		t.Fatalf("фраза отката: %q", r2.Chat.Text)
	}
	st = r2.State
	r3 := guideTurnReq(t, s, in.GuideRequest{Query: "самый дешёвый криптопро csp для сервера", Message: "на 3 сервера", State: &st})
	if r3.Chat.Status == out.ChatNoOffers || !strings.Contains(r3.Chat.Conditions.Label, "3 сервер") {
		t.Fatalf("следующая реплика подбирает дальше: %q %+v", r3.Chat.Conditions.Label, r3.Chat.Text)
	}
}

func TestGuideCountSlangKomp(t *testing.T) {
	d := extractConditions("на 10 компов есть что-то?")
	if len(d.objects) == 0 || d.objects[0].Count != 10 || d.objects[0].Kind != "workstation" {
		t.Fatalf("«10 компов» — 10 рабочих мест: %+v", d.objects)
	}
	if !asksCompare("Сравинть хочу") {
		t.Fatal("«сравинть» — просьба сравнить")
	}
}
