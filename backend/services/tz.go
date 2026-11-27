package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"regexp"
	"strings"
	"sync"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"
	"kuznica/api/prompts"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"golang.org/x/sync/singleflight"
)

// Поиск по ТЗ (контур ТЗ, TZ-CORE): POST /api/tz запускает операцию разбора в фоне, фронт опрашивает
// GET /api/tz/operations/{id} раз в секунду, готовый разбор лежит неделю под хешем содержимого файла
// (GET /api/tz/{hash}). Операции — в памяти процесса с коротким сроком, как черновик подбора (guide_state.go).

// Статусы и этапы операции.
const (
	tzStatusProcessing = "processing"
	tzStatusCompleted  = "completed"
	tzStatusError      = "error"

	tzStageReading = "reading"
	tzStageOCR     = "ocr"
)

const (
	// TZMaxFileBytes — потолок файла ТЗ (и тела POST /api/tz).
	TZMaxFileBytes = 20 << 20
	tzResultTTL    = 7 * 24 * time.Hour
	tzOpTTL        = 15 * time.Minute
	tzOpCap        = 2000
	// tzParseSlots — одновременных разборов на процесс: файл до 20 МиБ в памяти и вызов модели.
	tzParseSlots = 4
	// tzOpTimeout — срок одной операции целиком (чтение, распознавание, модель).
	tzOpTimeout = 4 * time.Minute
	// tzPerAddr — одновременных операций с одного адреса.
	tzPerAddr = 2
)

// Тексты.
const (
	MsgTZTooMany    = "Документ уже разбирается. Дождитесь окончания или отмените разбор."
	MsgTZNotFound   = "Результаты по ТЗ недоступны: ссылка устарела или неверна. Загрузите ТЗ заново"
	MsgTZOpNotFound = "Разбор ТЗ не найден: он завершён давно или отменён"
)

// tzParserVersion — версия разбора: промпт и проверки. Меняется — повторная загрузка разбирает заново.
func tzParserVersion() string { return prompts.TZ().Version + "+c7" }

var reTZHash = regexp.MustCompile(`^[0-9a-f]{64}$`)

// tzOp — операция разбора.
type tzOp struct {
	id      string
	addr    string
	status  string
	stage   string
	hash    string
	reason  string
	cancel  context.CancelFunc
	touched time.Time
}

// tzState — операции и запасное хранилище разборов (без базы) одного графа сервисов.
type tzState struct {
	mu     sync.Mutex
	ops    map[string]*tzOp
	mem    map[string]domain.TZResult // без базы: разборы в памяти процесса
	flight singleflight.Group
	sem    chan struct{}
	model  ChatModel // тесты: своя модель
	ocr    tzOCR     // распознавание сканов; nil — нет
}

var tzStates sync.Map // *Services → *tzState

func (s *Services) tz() *tzState {
	v, _ := tzStates.LoadOrStore(s, &tzState{ops: map[string]*tzOp{}, mem: map[string]domain.TZResult{},
		sem: make(chan struct{}, tzParseSlots)})
	return v.(*tzState)
}

func tzModelOverride(s *Services) ChatModel { return s.tz().model }

// SetTZModel — модель извлечения позиций в тестах.
func (s *Services) SetTZModel(m ChatModel) { s.tz().model = m }

// TZStart — POST /api/tz: проверка файла и запуск разбора. Отказ проверки — операция сразу в состоянии error.
func (s *Services) TZStart(ctx context.Context, fileName string, data []byte) (*out.TZUpload, error) {
	st := s.tz()
	addr := clientAddr(ctx)
	op := &tzOp{id: uuid.NewString(), addr: addr, status: tzStatusProcessing, stage: tzStageReading}
	format := ""
	switch {
	case len(data) > TZMaxFileBytes:
		op.status, op.reason = tzStatusError, tzReasonSize
	case len(data) == 0:
		op.status, op.reason = tzStatusError, tzReasonDamaged
	default:
		if format = tzSniff(fileName, data); format == "" {
			op.status, op.reason = tzStatusError, tzReasonFormat
		}
	}
	if op.status == tzStatusError {
		st.put(s.now(), op)
		return &out.TZUpload{OperationID: op.id, Status: op.status, Reason: op.reason}, nil
	}
	if addr != "" && st.active(addr, s.now()) >= tzPerAddr {
		return nil, errs.NewTooManyError(MsgTZTooMany)
	}
	octx, cancel := context.WithTimeout(context.WithoutCancel(ctx), tzOpTimeout)
	op.cancel = cancel
	st.put(s.now(), op)
	go func() {
		defer cancel()
		s.tzRun(octx, op.id, tzFileName(fileName), format, data)
	}()
	return &out.TZUpload{OperationID: op.id}, nil
}

// TZOperation — GET /api/tz/operations/{id}.
func (s *Services) TZOperation(id string) (*out.TZOperation, error) {
	op, ok := s.tz().get(id, s.now())
	if !ok {
		return nil, errs.NewNotFoundError(MsgTZOpNotFound)
	}
	return &out.TZOperation{Status: op.status, Stage: op.stage, Hash: op.hash, Reason: op.reason}, nil
}

// TZCancel — DELETE /api/tz/operations/{id}: разбор останавливается, операция забывается.
func (s *Services) TZCancel(id string) error {
	st := s.tz()
	st.mu.Lock()
	op, ok := st.ops[id]
	if ok {
		delete(st.ops, id)
	}
	st.mu.Unlock()
	if !ok {
		return errs.NewNotFoundError(MsgTZOpNotFound)
	}
	if op.cancel != nil {
		op.cancel()
	}
	return nil
}

// TZResult — GET /api/tz/{hash}: позиции без товаров; неизвестный или истёкший хеш — 404. Модель не зовётся.
func (s *Services) TZResult(ctx context.Context, hash string) (*out.TZResult, error) {
	if !reTZHash.MatchString(hash) {
		return nil, errs.NewNotFoundError(MsgTZNotFound)
	}
	row, err := s.tzLoad(ctx, hash)
	if err != nil {
		return nil, err
	}
	if row == nil {
		return nil, errs.NewNotFoundError(MsgTZNotFound)
	}
	res := &out.TZResult{Hash: row.Hash, FileName: row.FileName, ExtractedCount: row.ExtractedCount,
		ProcessedCount: row.ProcessedCount, Positions: []out.TZPosition{}}
	if len(row.Positions) > 0 {
		if err := json.Unmarshal(row.Positions, &res.Positions); err != nil {
			return nil, errs.NewInternalError("Разбор ТЗ не прочитан").WithCause(err)
		}
	}
	return res, nil
}

// tzRun — разбор в фоне: текст, распознавание скана, модель, проверки, запись под хешем.
func (s *Services) tzRun(ctx context.Context, id, fileName, format string, data []byte) {
	st := s.tz()
	defer func() {
		if r := recover(); r != nil {
			zap.L().Error("tz: разбор упал", zap.String("operation_id", id), zap.Any("panic", r))
			st.finish(id, s.now(), "", tzReasonGeneral)
		}
	}()
	select {
	case st.sem <- struct{}{}:
		defer func() { <-st.sem }()
	case <-ctx.Done():
		st.finish(id, s.now(), "", tzReasonGeneral)
		return
	}
	sum := sha256.Sum256(data)
	hash := hex.EncodeToString(sum[:])
	version := tzParserVersion()
	// Тот же файл уже разобран той же версией — модель не зовём, срок продлевается.
	if row, err := s.tzLoad(ctx, hash); err == nil && row != nil && row.ParserVersion == version {
		s.tzTouch(ctx, row, fileName)
		s.tzEvent(format, "cache", row.ProcessedCount)
		st.finish(id, s.now(), hash, "")
		return
	}
	v, err, _ := st.flight.Do(hash, func() (any, error) {
		return s.tzParse(ctx, id, hash, fileName, format, data, version)
	})
	if ctx.Err() != nil {
		return // отменено или вышел срок: результат никто не ждёт
	}
	if err != nil {
		st.finish(id, s.now(), "", tzReasonOf(err))
		return
	}
	row := v.(*domain.TZResult)
	if row.FileName != fileName {
		s.tzTouch(ctx, row, fileName)
	}
	st.finish(id, s.now(), hash, "")
}

// tzParse — текст, распознавание, модель и запись разбора; ошибка — с причиной для операции.
func (s *Services) tzParse(ctx context.Context, id, hash, fileName, format string, data []byte, version string) (*domain.TZResult, error) {
	st := s.tz()
	doc, err := tzReadText(format, data)
	if err != nil {
		return nil, err
	}
	if doc.Scan {
		st.stage(id, tzStageOCR, s.now())
		text, oerr := s.tzRecognize(ctx, data, doc.Pages)
		if oerr != nil {
			zap.L().Warn("tz: распознавание не удалось", zap.String("operation_id", id), zap.Error(oerr))
			return nil, tzFail(tzReasonOCR)
		}
		doc.Text = text
		st.stage(id, tzStageReading, s.now())
	}
	positions, extracted, err := s.tzExtract(ctx, doc.Text)
	if err != nil {
		if ctx.Err() != nil {
			return nil, ctx.Err()
		}
		return nil, tzFail(tzReasonGeneral)
	}
	if len(positions) == 0 {
		return nil, tzFail(tzReasonNoPositions)
	}
	raw, err := json.Marshal(positions)
	if err != nil {
		return nil, err
	}
	now := s.now().UTC()
	row := &domain.TZResult{Hash: hash, FileName: fileName, Positions: raw, ExtractedCount: extracted,
		ProcessedCount: len(positions), ParserVersion: version, CreatedAt: now, ExpiresAt: now.Add(tzResultTTL)}
	if err := s.tzSave(ctx, row); err != nil {
		zap.L().Error("tz: разбор не записан", zap.String("operation_id", id), zap.Error(err))
		return nil, tzFail(tzReasonGeneral)
	}
	s.tzEvent(format, "parsed", len(positions))
	return row, nil
}

// --- хранение ---

func (s *Services) tzLoad(ctx context.Context, hash string) (*domain.TZResult, error) {
	now := s.now().UTC()
	if s.repos.Enabled() {
		row, err := s.repos.GetTZResult(ctx, hash, now)
		if err != nil {
			return nil, errs.NewUnavailableError("База данных недоступна").WithCause(err)
		}
		return row, nil
	}
	st := s.tz()
	st.mu.Lock()
	defer st.mu.Unlock()
	row, ok := st.mem[hash]
	if !ok || !row.ExpiresAt.After(now) {
		return nil, nil
	}
	return &row, nil
}

func (s *Services) tzSave(ctx context.Context, row *domain.TZResult) error {
	if s.repos.Enabled() {
		return s.repos.SaveTZResult(ctx, row, s.now().UTC())
	}
	st := s.tz()
	st.mu.Lock()
	defer st.mu.Unlock()
	now := s.now().UTC()
	for k, v := range st.mem {
		if !v.ExpiresAt.After(now) {
			delete(st.mem, k)
		}
	}
	st.mem[row.Hash] = *row
	return nil
}

// tzTouch — повторная загрузка: новое имя файла и срок неделя от сейчас.
func (s *Services) tzTouch(ctx context.Context, row *domain.TZResult, fileName string) {
	exp := s.now().UTC().Add(tzResultTTL)
	if s.repos.Enabled() {
		if err := s.repos.TouchTZResult(ctx, row.Hash, fileName, exp); err != nil {
			zap.L().Warn("tz: срок разбора не продлён", zap.Error(err))
		}
		return
	}
	st := s.tz()
	st.mu.Lock()
	defer st.mu.Unlock()
	if r, ok := st.mem[row.Hash]; ok {
		r.FileName, r.ExpiresAt = fileName, exp
		st.mem[row.Hash] = r
	}
}

// tzEvent — факт загрузки в журнал поведения: формат, разобран или взят готовым, число позиций. Без текста и имени файла.
func (s *Services) tzEvent(format, mode string, positions int) {
	if !s.repos.Enabled() {
		return
	}
	payload, _ := json.Marshal(map[string]any{"format": format, "mode": mode, "positions": positions})
	ev := &domain.BehaviorEvent{Type: domain.BehaviorTZUpload, Payload: payload, Source: domain.EventSourceTZ}
	s.writes.Add(1)
	go func() {
		defer s.writes.Done()
		ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
		defer cancel()
		if err := s.repos.SaveBehaviorEvent(ctx, ev); err != nil {
			zap.L().Warn("tz: событие загрузки не записано", zap.Error(err))
		}
	}()
}

// --- операции в памяти ---

func (st *tzState) put(now time.Time, op *tzOp) {
	st.mu.Lock()
	defer st.mu.Unlock()
	op.touched = now
	if len(st.ops) >= tzOpCap {
		oldest, oldestAt := "", now
		for k, v := range st.ops {
			if now.Sub(v.touched) > tzOpTTL {
				delete(st.ops, k)
				continue
			}
			if v.touched.Before(oldestAt) {
				oldest, oldestAt = k, v.touched
			}
		}
		if len(st.ops) >= tzOpCap && oldest != "" {
			if o := st.ops[oldest]; o.cancel != nil {
				o.cancel()
			}
			delete(st.ops, oldest)
		}
	}
	st.ops[op.id] = op
}

// get — копия операции (без гонки с фоновым разбором).
func (st *tzState) get(id string, now time.Time) (tzOp, bool) {
	st.mu.Lock()
	defer st.mu.Unlock()
	op, ok := st.ops[id]
	if !ok || now.Sub(op.touched) > tzOpTTL {
		return tzOp{}, false
	}
	return *op, true
}

// active — операций в работе с адреса.
func (st *tzState) active(addr string, now time.Time) int {
	st.mu.Lock()
	defer st.mu.Unlock()
	n := 0
	for _, op := range st.ops {
		if op.addr == addr && op.status == tzStatusProcessing && now.Sub(op.touched) <= tzOpTTL {
			n++
		}
	}
	return n
}

func (st *tzState) stage(id, stage string, now time.Time) {
	st.mu.Lock()
	defer st.mu.Unlock()
	if op, ok := st.ops[id]; ok {
		op.stage, op.touched = stage, now
	}
}

// finish — итог операции: hash — готово, иначе ошибка с причиной.
func (st *tzState) finish(id string, now time.Time, hash, reason string) {
	st.mu.Lock()
	defer st.mu.Unlock()
	op, ok := st.ops[id]
	if !ok {
		return // отменена
	}
	op.touched, op.stage = now, ""
	if hash != "" {
		op.status, op.hash = tzStatusCompleted, hash
		return
	}
	if reason == "" {
		reason = tzReasonGeneral
	}
	op.status, op.reason = tzStatusError, reason
}

// tzFileName — имя файла для заголовка: без пути, без управляющих знаков, до 255 знаков.
func tzFileName(name string) string {
	if i := strings.LastIndexAny(name, "/\\"); i >= 0 {
		name = name[i+1:]
	}
	name = tzClean(name, 255)
	if name == "" {
		return "document"
	}
	return name
}

var errTZNoOCR = errors.New("tz: распознавание сканов не подключено")

// tzOCR — распознавание скана PDF: текст страниц по порядку.
type tzOCR interface {
	Recognize(ctx context.Context, pdf []byte, pages int) (string, error)
}

// tzRecognize — текст скана; распознавания нет — ошибка (причина ocr).
func (s *Services) tzRecognize(ctx context.Context, data []byte, pages int) (string, error) {
	o := s.tz().ocr
	if o == nil {
		return "", errTZNoOCR
	}
	return o.Recognize(ctx, data, pages)
}
