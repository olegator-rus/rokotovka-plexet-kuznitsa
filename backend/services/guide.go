package services

import (
	"sync"
	"time"
)

// Режим подбора (контур подбора): сервис подбора поверх поиска, каталога и базы знаний.

// guideService — состояние подбора процесса: база знаний, индекс каталога, подборы в памяти.
type guideService struct {
	s             *Services
	kb            guideKBState
	cat           guideCatalogCache
	fixedCatalog  *guideCatalog // тесты: каталог без загрузки
	sessions      guideSessions
	searchFn      guideSearchFunc // тесты: поиск без OpenSearch
	modelOverride ChatModel       // тесты: фейковая модель
}

// guideBy — сервис подбора на граф сервисов (как vectorsBy): Services не меняется.
var guideBy sync.Map

func (s *Services) guide() *guideService {
	if v, ok := guideBy.Load(s); ok {
		return v.(*guideService)
	}
	now := s.now
	if now == nil {
		now = time.Now
	}
	g := &guideService{s: s}
	g.sessions.now = now
	v, _ := guideBy.LoadOrStore(s, g)
	return v.(*guideService)
}
