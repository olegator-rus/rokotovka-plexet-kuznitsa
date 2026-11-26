package services

import (
	"context"
	"errors"
	"sync/atomic"

	"kuznica/api/domain"
	"kuznica/api/externalApi/catalogsource"

	"go.uber.org/zap"
)

// Справочники допродажи контура фида: совместимость с первичным документом и акции магазина.
// Читаются при старте API из файлов рядом с mapping.yaml (как словарь понимания) — переимпорт каталога не нужен.
// Битый файл не роняет поиск: предупреждение в журнале, блок работает на связях каталога (уровень B).

// upsellState — справочники допродажи в памяти; меняются целиком.
type upsellState struct {
	data atomic.Pointer[upsellData]
}

type upsellData struct {
	compat []domain.CompatDoc
	promos []domain.Promo
}

// upsellLoader — источник каталога со справочниками допродажи (generic: mapping.compat_docs, mapping.promos).
type upsellLoader interface {
	LoadCompatDocs(ctx context.Context) ([]domain.CompatDoc, error)
	LoadPromos(ctx context.Context) ([]domain.Promo, error)
}

// LoadUpsellData — справочники допродажи из источника каталога. Нет файла — справочник пуст.
func (s *Services) LoadUpsellData(ctx context.Context) {
	l, ok := s.source.(upsellLoader)
	if !ok {
		return
	}
	compat, err := l.LoadCompatDocs(ctx)
	switch {
	case errors.Is(err, catalogsource.ErrNoFile):
		compat = nil
	case err != nil:
		zap.L().Warn("совместимость с документом не прочитана — блок допродажи без уровня A", zap.Error(err))
		compat = nil
	}
	promos, err := l.LoadPromos(ctx)
	switch {
	case errors.Is(err, catalogsource.ErrNoFile):
		promos = nil
	case err != nil:
		zap.L().Warn("акции не прочитаны — карточки без акций", zap.Error(err))
		promos = nil
	}
	s.SetUpsellData(compat, promos)
	zap.L().Info("справочники допродажи загружены", zap.Int("compat_docs", len(compat)), zap.Int("promos", len(promos)))
}

// SetUpsellData — справочники допродажи (тесты и старт).
func (s *Services) SetUpsellData(compat []domain.CompatDoc, promos []domain.Promo) {
	s.upsell.data.Store(&upsellData{compat: append([]domain.CompatDoc(nil), compat...),
		promos: append([]domain.Promo(nil), promos...)})
	upsellIndexes.Range(func(k, _ any) bool { upsellIndexes.Delete(k); return true })
}

func (s *Services) upsellData() *upsellData {
	if d := s.upsell.data.Load(); d != nil {
		return d
	}
	return &upsellData{}
}

// upsellBlockTitle — заголовок блока: настройка, по умолчанию «Дополните решение».
func (s *Services) upsellBlockTitle() string {
	if s.upsellTitle != "" {
		return s.upsellTitle
	}
	return defaultUpsellTitle
}

const defaultUpsellTitle = "Дополните решение"
