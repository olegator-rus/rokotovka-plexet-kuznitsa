package services

import (
	"context"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/in"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Консультант (контур консультанта): свободный разговор поверх ядра подбора контуров подбора–10.
// Модель выбирает и формулирует, код отвечает за числа, названия, ссылки и правила. Ход:
//   - сервисная тема, просьба о скидке, спецификация — код (без модели);
//   - объяснение без товаров — один вызов модели (текст);
//   - подбор — ход /api/guide (маршрутизация, инструменты, правила лицензий; в нём первый вызов модели) → лист
//     фактов → второй вызов модели со вставками → подстановка и проверки → вложения кодом.

const (
	consultantMaxText   = 500
	consultantMaxSpec   = 50
	consultantTTL       = 2 * time.Hour
	consultantMaxConvs  = 5000
	consultantHistory   = 8
	consultantTurnLimit = 25 * time.Second
	// ConsultantFallbackText — запасной ответ, когда текст не прошёл проверки или модель недоступна.
	ConsultantFallbackText = "Не получилось сформулировать ответ, но вот что нашлось в каталоге"
	consultantNothingText  = "Не получилось подготовить ответ. Попробуйте переформулировать вопрос"
)

// Этапы для индикатора (событие status).
const (
	consultantStepSearching   = "searching"
	consultantStepChecking    = "checking"
	consultantStepCalculating = "calculating"
)

// Причины message.error.
const (
	consultantErrModel   = "model_unavailable"
	consultantErrInvalid = "reply_rejected"
	consultantErrTurn    = "turn_failed"
)

// consultantLine — реплика истории для модели.
type consultantLine struct{ role, text string }

// consultantConv — разговор: условия подбора (состояние /api/guide), последние реплики, позиции листа фактов.
type consultantConv struct {
	mu      sync.Mutex
	guide   *domain.GuideState
	history []consultantLine
	sheet   []consultantItem
	assumed bool // допущение «новая лицензия на год» уже проговорено
	kit     *consultantKit
	seen    time.Time
}

// consultantStore — разговоры в памяти процесса с TTL (как сессии /api/guide).
type consultantStore struct {
	mu sync.Mutex
	m  map[string]*consultantConv
}

var consultantConvs = &consultantStore{m: map[string]*consultantConv{}}

func (cs *consultantStore) get(id string, now time.Time) *consultantConv {
	cs.mu.Lock()
	defer cs.mu.Unlock()
	if c, ok := cs.m[id]; ok && now.Sub(c.seen) < consultantTTL {
		c.seen = now
		return c
	}
	if len(cs.m) >= consultantMaxConvs {
		for k, c := range cs.m {
			if now.Sub(c.seen) >= consultantTTL {
				delete(cs.m, k)
			}
		}
		if len(cs.m) >= consultantMaxConvs {
			var oldK string
			var oldT time.Time
			for k, c := range cs.m {
				if oldK == "" || c.seen.Before(oldT) {
					oldK, oldT = k, c.seen
				}
			}
			delete(cs.m, oldK)
		}
	}
	c := &consultantConv{seen: now}
	cs.m[id] = c
	return c
}

func (c *consultantConv) remember(role, text string) {
	if strings.TrimSpace(text) == "" {
		return
	}
	c.history = append(c.history, consultantLine{role: role, text: text})
	if len(c.history) > consultantHistory {
		c.history = c.history[len(c.history)-consultantHistory:]
	}
}

// consultantPlan — что отдать за ход: сообщения (уже с подставленными вставками), вложения и быстрые ответы
// последнего сообщения; errReason — message.error перед запасным ответом.
type consultantPlan struct {
	messages    []string
	attachments []any
	quick       []string
	errReason   string
}

// ValidateConsultant — проверка запроса до начала потока (ошибка — обычный конверт API).
func (s *Services) ValidateConsultant(req *in.ConsultantRequest) error {
	req.Text = strings.TrimSpace(req.Text)
	req.ConversationID = strings.TrimSpace(req.ConversationID)
	var bad []errs.FieldError
	if req.Text == "" {
		bad = append(bad, errs.FieldError{Field: "text", Code: errs.CodeRequired, Message: "Нужна реплика"})
	} else if len([]rune(req.Text)) > consultantMaxText {
		bad = append(bad, errs.FieldError{Field: "text", Code: errs.CodeOutOfRange, Message: "Реплика длиннее 500 символов"})
	}
	if req.ConversationID == "" || len(req.ConversationID) > maxIDLen {
		bad = append(bad, errs.FieldError{Field: "conversation_id", Code: errs.CodeRequired, Message: "Нужен идентификатор разговора"})
	}
	if len(req.Spec) > consultantMaxSpec {
		bad = append(bad, errs.FieldError{Field: "spec", Code: errs.CodeOutOfRange, Message: "В спецификации не больше 50 строк"})
	}
	for _, l := range req.Spec {
		if l.OfferID == "" || len(l.OfferID) > maxIDLen || l.Qty < 0 || l.Qty > maxQuoteQuantity {
			bad = append(bad, errs.FieldError{Field: "spec", Code: errs.CodeInvalidFormat, Message: "Неверная строка спецификации"})
			break
		}
	}
	if req.Page != nil && len(req.Page.Type) > 16 {
		bad = append(bad, errs.FieldError{Field: "page.type", Code: errs.CodeOutOfRange, Message: "Неверный тип страницы"})
	}
	if len(bad) > 0 {
		return errs.NewValidationError(errs.MsgInvalidFields).WithDetails(bad...)
	}
	if s.guide().catalog() == nil {
		return errs.NewUnavailableError(MsgGuideNoCatalog)
	}
	return nil
}

// Consult — один ход консультанта; события отдаются через emit по порядку. Запрос уже проверен ValidateConsultant.
func (s *Services) Consult(ctx context.Context, requestID string, req in.ConsultantRequest, emit func(out.ConsultantEvent)) {
	ctx = WithRequestID(ctx, requestID)
	ctx, cancel := context.WithTimeout(ctx, consultantTurnLimit)
	defer cancel()
	conv := consultantConvs.get(req.ConversationID, time.Now())
	conv.mu.Lock()
	defer conv.mu.Unlock()
	t := &consultantTurn{s: s, g: s.guide(), req: req, conv: conv, emit: emit, requestID: requestID}
	t.cat = t.g.catalog()
	t.kb = t.g.knowledge()
	started := time.Now()
	plan := t.safeRun(ctx)
	// Журнал хода без текста реплик: тип, вызовы модели, отклонения проверками, время.
	zap.L().Info("консультант: ход", zap.String("request_id", requestID), zap.String("kind", t.kind),
		zap.Int("reply_calls", t.attempts), zap.Strings("rejected", t.rejected), zap.String("error", plan.errReason),
		zap.Int64("ms", time.Since(started).Milliseconds()))
	conv.remember("user", req.Text)
	conv.remember("assistant", strings.Join(plan.messages, " "))
	t.send(plan)
}

// consultantTurn — состояние одного хода.
type consultantTurn struct {
	s         *Services
	g         *guideService
	cat       *guideCatalog
	kb        *guideKB
	req       in.ConsultantRequest
	conv      *consultantConv
	emit      func(out.ConsultantEvent)
	requestID string
	attempts  int      // вызовов модели ответа
	rejected  []string // причины отклонения текста проверками
	kind      string   // тип хода для журнала
}

func (t *consultantTurn) status(step string) { t.emit(out.ConsultantEvent{Type: "status", Step: step}) }

// safeRun — ход без паники наружу: сбой — запасной ответ.
func (t *consultantTurn) safeRun(ctx context.Context) (plan consultantPlan) {
	defer func() {
		if r := recover(); r != nil {
			plan = t.fallback(consultantErrTurn, t.conv.sheet)
		}
	}()
	return t.run(ctx)
}

func (t *consultantTurn) run(ctx context.Context) consultantPlan {
	text := t.req.Text
	if p, ok := t.specTurn(); ok {
		t.kind = "spec"
		return p
	}
	if p, ok := serviceTurn(text, t.s.consultantLinks); ok {
		t.kind = "service"
		return p
	}
	if consultantExplainQuestion(text) {
		t.kind = "explain"
		return t.explainTurn(ctx)
	}
	if p, ok := t.kitTurn(ctx); ok {
		t.kind = "kit"
		return p
	}
	if p, ok := t.pageTurn(ctx); ok {
		t.kind = "page"
		return p
	}
	t.kind = "selection"
	return t.selectionTurn(ctx)
}

// send — события хода: каждое сообщение — пара message.start … message.complete; вложения и быстрые ответы —
// у последнего.
func (t *consultantTurn) send(p consultantPlan) {
	n := 0
	next := func() string {
		n++
		return "m" + itoa(n)
	}
	if p.errReason != "" {
		id := next()
		t.emit(out.ConsultantEvent{Type: "message.start", ID: id})
		t.emit(out.ConsultantEvent{Type: "message.error", ID: id, Reason: p.errReason})
	}
	if len(p.messages) == 0 {
		p.messages = []string{consultantNothingText}
	}
	for i, m := range p.messages {
		id := next()
		t.emit(out.ConsultantEvent{Type: "message.start", ID: id})
		t.emit(out.ConsultantEvent{Type: "message.delta", ID: id, Text: m})
		last := i == len(p.messages)-1
		var quick []string
		if last {
			for _, a := range p.attachments {
				t.emit(out.ConsultantEvent{Type: "message.attachment", ID: id, Attachment: a})
			}
			quick = p.quick
		}
		t.emit(out.ConsultantEvent{Type: "message.complete", ID: id, QuickReplies: quick})
	}
}

// fallback — запасной ответ: фраза и список того, что нашлось (карточки не теряются).
func (t *consultantTurn) fallback(reason string, items []consultantItem) consultantPlan {
	p := consultantPlan{errReason: reason, messages: []string{ConsultantFallbackText}}
	if len(items) == 0 {
		items = t.searchItems()
	}
	if l := listAttachment(items, ""); l != nil {
		p.attachments = append(p.attachments, *l)
	}
	return p
}
