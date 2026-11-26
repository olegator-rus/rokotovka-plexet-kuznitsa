package services

import (
	"context"
	"sync"
	"sync/atomic"
	"time"

	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

// Режимы работы без моделей (контур доводки). Режим LLM уже был (llmguard.go); здесь —
// режим векторов (эмбеддер и реранкер — один сервис моделей) и одна настройка «без моделей», которая выключает
// и LLM, и векторы. Переключаются без перезапуска. Для прогона вклада компонентов те же выключатели действуют на
// один поиск через контекст и не трогают режим процесса.

// Режимы векторов.
const (
	vectorsModeOn  = "on"
	vectorsModeOff = "off"
)

// reasonVectorsOff — причина пропуска шагов dense и rerank: векторы выключены настройкой (режим или прогон).
const reasonVectorsOff = "vectors_off"

// Тексты.
const (
	MsgVectorsModeInvalid = "Режим векторов — on или off"
	MsgOfflineModeInvalid = "Режим без моделей — on или off"
	bannerVectorsOff      = "Режим без векторов: поиск по тексту и правилам, реранкер выключен"
	bannerOffline         = "Режим без моделей: LLM и векторы выключены, поиск по тексту, правилам и кэшу разборов"
)

// vectorsState — режим векторов процесса.
type vectorsState struct {
	off     atomic.Bool
	changed atomic.Pointer[time.Time]
}

// vectorsBy — режим на каждый граф сервисов (в процессе он один; в тестах — свои), как evalJobsBy.
var vectorsBy sync.Map

func (s *Services) vectors() *vectorsState {
	v, _ := vectorsBy.LoadOrStore(s, &vectorsState{})
	return v.(*vectorsState)
}

// VectorsMode — действующий режим векторов: on | off.
func (s *Services) VectorsMode() string {
	if s.vectors().off.Load() {
		return vectorsModeOff
	}
	return vectorsModeOn
}

// VectorsState — режим векторов для /api/meta и /api/admin/vectors-mode.
func (s *Services) VectorsState() out.VectorsState {
	st := out.VectorsState{Mode: s.VectorsMode(), Online: s.embedder.Enabled() || s.reranker.Enabled(),
		ChangedAt: s.vectors().changed.Load(), Offline: s.VectorsMode() == vectorsModeOff && s.LLMMode() == llmModeOff}
	switch {
	case st.Offline:
		st.Banner = ptr(bannerOffline)
	case st.Mode == vectorsModeOff:
		st.Banner = ptr(bannerVectorsOff)
	}
	return st
}

// SetVectorsMode — POST /api/admin/vectors-mode: векторы и реранкер выключаются и включаются без перезапуска.
func (s *Services) SetVectorsMode(mode string) (*out.VectorsState, error) {
	if mode != vectorsModeOn && mode != vectorsModeOff {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).
			WithDetails(errs.FieldError{Field: "mode", Code: errs.CodeInvalidFormat, Message: MsgVectorsModeInvalid})
	}
	v := s.vectors()
	if prev := v.off.Swap(mode == vectorsModeOff); prev != (mode == vectorsModeOff) {
		now := s.now().UTC()
		v.changed.Store(&now)
		zap.L().Info("режим векторов переключён", zap.String("to", mode))
	}
	st := s.VectorsState()
	return &st, nil
}

// SetOfflineMode — POST /api/admin/offline-mode: одна настройка «без моделей». on — LLM и векторы выключены;
// off — векторы включены, LLM включён, если модель настроена (иначе остаётся выключенным, без ошибки).
func (s *Services) SetOfflineMode(ctx context.Context, mode string) (*out.ModelsState, error) {
	if mode != "on" && mode != "off" {
		return nil, errs.NewValidationError(errs.MsgInvalidFields).
			WithDetails(errs.FieldError{Field: "mode", Code: errs.CodeInvalidFormat, Message: MsgOfflineModeInvalid})
	}
	llmMode, vecMode := llmModeOff, vectorsModeOff
	if mode == "off" {
		vecMode = vectorsModeOn
		if s.guard().online {
			llmMode = llmModeOn
		}
	}
	if _, err := s.SetLLMMode(ctx, llmMode); err != nil {
		return nil, err
	}
	if _, err := s.SetVectorsMode(vecMode); err != nil {
		return nil, err
	}
	return &out.ModelsState{LLM: s.LLMState(ctx), Vectors: s.VectorsState()}, nil
}

// searchModes — выключатели одного поиска (прогон вклада компонентов): не меняют режим процесса.
type searchModes struct {
	noLLM, noVectors, noRerank bool
	// noModelCache — эмбеддер и реранкер без кэша (search_modelcache.go): строки таблицы вклада меряют время
	// моделей одинаково холодным, иначе строка после B2 получала бы векторы из кэша.
	noModelCache bool
}

type searchModesKey struct{}

// withSearchModes — контекст поиска с выключенными компонентами.
func withSearchModes(ctx context.Context, m searchModes) context.Context {
	return context.WithValue(ctx, searchModesKey{}, m)
}

func modesFrom(ctx context.Context) searchModes {
	m, _ := ctx.Value(searchModesKey{}).(searchModes)
	return m
}

// vectorsOff — векторная ветка выключена: режимом процесса или выключателем поиска.
func (s *Services) vectorsOff(ctx context.Context) bool {
	return s.vectors().off.Load() || modesFrom(ctx).noVectors
}

// rerankOffBy — реранкер выключен: режимом векторов (тот же сервис моделей) или выключателем поиска. Выключатель
// noVectors реранкер не трогает: строка «без векторной ветки» отделяет вклад kNN от вклада реранкера.
func (s *Services) rerankOffBy(ctx context.Context) bool {
	return s.vectors().off.Load() || modesFrom(ctx).noRerank
}

// llmOffBy — разбор моделью выключен выключателем поиска: только правила, без кэша разборов (строгая нижняя
// граница «без LLM» для таблицы вклада; режим процесса LLM_MODE=off кэш читает).
func llmOffBy(ctx context.Context) bool { return modesFrom(ctx).noLLM }

// modelCacheOff — кэш векторов и оценок реранкера выключен для этого поиска.
func modelCacheOff(ctx context.Context) bool { return modesFrom(ctx).noModelCache }
