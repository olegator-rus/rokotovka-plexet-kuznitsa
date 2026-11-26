package services

import (
	"context"
	"fmt"
	"sort"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"

	"go.uber.org/zap"
)

// CatalogSource — откуда берётся каталог для импорта. Сейчас — mock JSON (externalApi/catalogsource),
// после брифа — данные организаторов; импорт и поиск от формата источника не зависят.
type CatalogSource interface {
	Name() string
	Load(ctx context.Context) (*domain.CatalogSnapshot, error)
}

// guestClientID — покупатель без истории.
const guestClientID = "guest"

// catalogState — справочники в памяти: версия каталога и правил, клиенты и их покупки. Поиску
// хватает индекса; отсюда — только контекст клиента и /api/meta.
type catalogState struct {
	version      string
	rulesVersion int
	clients      []domain.Client
	// purchased — клиент → купленные позиции и семейства.
	purchased map[string]map[string]bool
	// productNames — id позиции → название семейства (подсказка клиента в meta).
	productNames map[string]string
	// lex — словарь каталога для нормализации и ранжирования B3 (search_lexicon.go).
	lex *searchLexicon
	// commerce — справочники контура коммерции: связи, правила лицензий и совместимости, атрибуты, шаблоны (commerce.go).
	commerce *commerceCatalog
	// hw — словарь оборудования: парт-номера, единицы, модели (hardware.go).
	hw *hardwareLexicon
}

// newCatalogState — состояние из снимка.
func newCatalogState(s *domain.CatalogSnapshot, rulesVersion int) *catalogState {
	clients := append([]domain.Client(nil), s.Clients...)
	// Гость — первым (переключатель клиента открывается на нём), остальные — по id: порядок не зависит
	// от того, прочитан каталог из файлов или из базы.
	sort.SliceStable(clients, func(i, j int) bool {
		if gi, gj := clients[i].ID == guestClientID, clients[j].ID == guestClientID; gi != gj {
			return gi
		}
		return clients[i].ID < clients[j].ID
	})
	st := &catalogState{
		version:      s.Version,
		rulesVersion: rulesVersion,
		clients:      clients,
		purchased:    make(map[string]map[string]bool, len(s.Clients)),
		productNames: make(map[string]string, len(s.Products)),
		lex:          newSearchLexicon(s),
		commerce:     newCommerceCatalog(s),
	}
	st.hw = newHardwareLexicon(s, st.lex)
	family := make(map[string]string, len(s.Products))
	for _, p := range s.Products {
		family[p.ID] = p.FamilyID
		st.productNames[p.ID] = p.FamilyName
		if st.productNames[p.ID] == "" {
			st.productNames[p.ID] = p.Title
		}
	}
	for _, c := range s.Clients {
		set := map[string]bool{}
		for _, p := range c.Purchases {
			set["p:"+p.ProductID] = true
			if f := family[p.ProductID]; f != "" {
				set["f:"+f] = true
			}
		}
		st.purchased[c.ID] = set
	}
	return st
}

// purchasedBefore — клиент покупал эту позицию или другую лицензию того же семейства.
func (st *catalogState) purchasedBefore(clientID, productID, familyID string) bool {
	if st == nil || clientID == "" {
		return false
	}
	set := st.purchased[clientID]
	return set["p:"+productID] || set["f:"+familyID]
}

// LoadCatalogState — справочники при старте: из базы, если там есть каталог, иначе из источника, и
// _meta индекса. Ошибка не роняет API: поиску справочники не обязательны, в журнале — предупреждение.
func (s *Services) LoadCatalogState(ctx context.Context) {
	s.loadCatalogState(ctx)
	if _, err := s.indexInfo(ctx); err != nil {
		zap.L().Warn("индекс поиска не прочитан — make import?", zap.Error(err))
	}
}

// loadCatalogState — справочники из базы или источника.
func (s *Services) loadCatalogState(ctx context.Context) {
	var snap *domain.CatalogSnapshot
	from := ""
	rules := 0
	if s.repos.Enabled() {
		var err error
		if snap, err = s.repos.LoadCatalog(ctx); err != nil {
			zap.L().Warn("каталог из базы не прочитан", zap.Error(err))
		}
		if rules, err = s.repos.RulesVersion(ctx); err != nil {
			zap.L().Warn("версия правил не прочитана", zap.Error(err))
		}
		from = "база"
	}
	if snap == nil && s.source != nil {
		var err error
		if snap, err = s.source.Load(ctx); err != nil {
			zap.L().Warn("каталог не загружен: в базе пусто, источник не читается — make import", zap.Error(err))
			return
		}
		from = s.source.Name()
	}
	if snap == nil {
		zap.L().Warn("каталог не загружен: нет ни базы, ни источника")
		return
	}
	if rules == 0 {
		rules = 1 // исходные словари из каталога
	}
	s.catalog.Store(newCatalogState(snap, rules))
	s.ReloadRelations(ctx) // действующая версия справочника связей (контур коммерции, relations.go)
	zap.L().Info("справочники каталога загружены", zap.String("from", from), zap.String("version", snap.Version),
		zap.Int("products", len(snap.Products)), zap.Int("clients", len(snap.Clients)), zap.Int("rules_version", rules),
		zap.Int("license_rules", len(snap.LicenseRules)), zap.Int("compatibility_rules", len(snap.CompatibilityRules)))
}

// indexMetaTTL — сколько верить закешированной _meta индекса. Импорт из CLI (make import) идёт другим
// процессом: через TTL работающий API увидит новый индекс и перечитает справочники.
const indexMetaTTL = 10 * time.Second

// cachedIndex — _meta индекса и когда она прочитана.
type cachedIndex struct {
	meta indexMeta
	at   time.Time
}

// indexInfo — _meta текущего индекса (кеш на indexMetaTTL). Сменился индекс (другое built_at) —
// справочники перечитываются в фоне.
func (s *Services) indexInfo(ctx context.Context) (*indexMeta, error) {
	cached := s.index.Load()
	if cached != nil && time.Since(cached.at) < indexMetaTTL {
		return &cached.meta, nil
	}
	var m indexMeta
	if err := s.search.Meta(ctx, s.alias, &m); err != nil {
		return nil, err
	}
	s.index.Store(&cachedIndex{meta: m, at: time.Now()})
	if cached != nil && !cached.meta.BuiltAt.Equal(m.BuiltAt) {
		zap.L().Info("индекс поиска сменился — справочники перечитываются", zap.String("version", m.CatalogVersion))
		s.writes.Add(1)
		go func() {
			defer s.writes.Done()
			ctx, cancel := context.WithTimeout(context.Background(), saveTimeout)
			defer cancel()
			s.loadCatalogState(ctx)
		}()
	}
	return &m, nil
}

// Meta — GET /api/meta.
func (s *Services) Meta() out.Meta {
	m := out.Meta{
		LLMMode:       s.LLMMode(),
		Configs:       domain.SearchConfigs,
		DefaultConfig: domain.DefaultSearchConfig,
		Clients:       []out.MetaClient{},
		LLM:           s.LLMState(context.Background()),
		Features:      out.Features(s.features),
		Synthetic:     []string{},
	}
	st := s.catalog.Load()
	if st == nil {
		return m
	}
	m.CatalogVersion = st.version
	if st.commerce != nil {
		m.Synthetic = append(m.Synthetic, st.commerce.synthetic...)
	}
	m.RulesVersion = st.rulesVersion
	for _, c := range st.clients {
		m.Clients = append(m.Clients, out.MetaClient{ID: c.ID, Name: c.Name, Segment: c.Segment, Hint: clientHint(st, c)})
	}
	return m
}

// segmentLabels — сегменты клиентов по-русски.
var segmentLabels = map[string]string{
	domain.SegmentSMB:        "Малый бизнес",
	domain.SegmentEnterprise: "Крупная компания",
	domain.SegmentGov:        "Госсектор",
	domain.SegmentIndividual: "Частное лицо",
}

// clientHint — подсказка в переключателе клиента: сегмент и последняя покупка.
func clientHint(st *catalogState, c domain.Client) string {
	seg := segmentLabels[c.Segment]
	if seg == "" {
		seg = c.Segment
	}
	if len(c.Purchases) == 0 {
		return seg + " · без истории покупок"
	}
	last := append([]domain.Purchase(nil), c.Purchases...)
	sort.SliceStable(last, func(i, j int) bool { return last[i].Date > last[j].Date })
	name := st.productNames[last[0].ProductID]
	if name == "" {
		name = last[0].ProductID
	}
	hint := fmt.Sprintf("%s · покупок: %d, последняя — %s", seg, len(c.Purchases), name)
	if d, err := time.Parse("2006-01-02", last[0].Date); err == nil {
		hint += d.Format(" (02.01.2006)")
	}
	return hint
}
