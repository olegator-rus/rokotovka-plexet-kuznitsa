package services

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"time"

	"kuznica/api/domain"
	"kuznica/api/out"
	"kuznica/api/pkg/errs"

	"go.uber.org/zap"
)

const (
	// embedBatch — текстов в одном вызове /embed (задача: 32–64; сервис моделей принимает до 64).
	embedBatch = 32
	// bulkBatch — документов в одном _bulk: с векторами документ ~20 КБ, пачка ~2 МБ.
	bulkBatch = 100
)

// Тексты отказов импорта.
const (
	MsgImportRunning  = "Импорт уже идёт — дождитесь его окончания"
	MsgNoSource       = "Источник каталога не настроен"
	MsgSearchDisabled = "Поисковый движок не настроен (OPENSEARCH_URL)"
	MsgSourceBroken   = "Данные каталога не прочитаны — причина в журнале API"
)

// ImportCatalog — каталог из источника в индекс OpenSearch и в PostgreSQL.
//
// Индекс строится заново: products_v{N+1}, документы с векторами (если сервис моделей отвечает); затем
// каталог заменяется в базе одной транзакцией; затем алиас products атомарно переключается на новый
// индекс, а старше предыдущего удаляются. Поиск во время импорта работает по старому индексу. Повторный
// импорт того же источника даёт то же состояние — импорт идемпотентен.
func (s *Services) ImportCatalog(ctx context.Context) (*out.ImportResult, error) {
	if !s.importMu.TryLock() {
		return nil, errs.NewConflictError(MsgImportRunning)
	}
	defer s.importMu.Unlock()
	if s.source == nil {
		return nil, errs.NewUnavailableError(MsgNoSource)
	}
	if !s.search.Enabled() {
		return nil, errs.NewUnavailableError(MsgSearchDisabled)
	}
	started := time.Now()
	snap, err := s.source.Load(ctx)
	if err != nil {
		return nil, errs.NewUnavailableError(MsgSourceBroken).WithCause(err)
	}
	res := &out.ImportResult{
		Source: s.source.Name(), CatalogVersion: snap.Version, Products: len(snap.Products),
		Vendors: len(snap.Vendors), Categories: len(snap.Categories), Relations: len(snap.Relations),
		Aliases: len(snap.Aliases), Clients: len(snap.Clients), Warnings: append([]string{}, snap.Warnings...),
	}
	families := map[string]bool{}
	for _, p := range snap.Products {
		families[p.FamilyID] = true
	}
	res.Families = len(families)
	docs := buildDocs(snap)

	t := time.Now()
	res.Vectors, err = s.embedDocs(ctx, docs)
	res.EmbedMs = time.Since(t).Milliseconds()
	if err != nil {
		res.Warnings = append(res.Warnings, "индекс без векторов: "+err.Error())
		zap.L().Warn("импорт: векторы не посчитаны, индекс строится без них", zap.Error(err))
	}

	// Индекс строится рядом с текущим, затем база, и только потом переключается алиас: API, который
	// заметит новый индекс (indexInfo), прочитает из базы уже новый каталог.
	t = time.Now()
	meta := indexMeta{CatalogVersion: snap.Version, HasVectors: res.Vectors, Products: len(docs), Source: res.Source, BuiltAt: s.now().UTC()}
	if res.Vectors {
		meta.EmbeddingDim = embeddingDim
	}
	name, existing, err := s.prepareIndex(ctx, docs, meta)
	if err != nil {
		return nil, err
	}
	res.Index = name
	res.IndexMs = time.Since(t).Milliseconds()
	drop := func(err error) (*out.ImportResult, error) {
		s.dropIndex(ctx, name)
		return nil, err
	}

	res.RulesVersion = 1
	if s.repos.Enabled() {
		t = time.Now()
		if err := s.repos.ReplaceCatalog(ctx, snap, s.now().UTC()); err != nil {
			return drop(errs.NewInternalError("Каталог не записан в базу").WithCause(err))
		}
		rules, err := domain.ToJSONB(map[string]any{"catalog_version": snap.Version, "aliases": snap.Aliases})
		if err != nil {
			return drop(errs.NewInternalError("Словари не сериализованы").WithCause(err))
		}
		// Новый catalog_version начинает цепочку версий правил заново (прежние — в архив, ждущие предложения —
		// stale); тот же — перезаписывает базовую версию цепочки, правки агента поверх неё остаются.
		ir, err := s.repos.SaveImportRules(ctx, snap.Version, rules, "исходные словари: "+res.Source)
		if err != nil {
			return drop(errs.NewInternalError("Словари не записаны в базу").WithCause(err))
		}
		res.RulesVersion = ir.Version
		if ir.NewChain {
			s.core.latest.Store(nil)
			zap.L().Info("импорт: новый каталог — версии правил начаты заново", zap.Int("version", ir.Version),
				zap.Int("stale_proposals", ir.Stale))
		}
		res.Database = true
		res.DatabaseMs = time.Since(t).Milliseconds()
	} else {
		res.Warnings = append(res.Warnings, "база не подключена: каталог только в индексе")
	}

	t = time.Now()
	if err := s.switchAlias(ctx, name, existing); err != nil {
		return drop(err)
	}
	res.IndexMs += time.Since(t).Milliseconds()
	s.index.Store(&cachedIndex{meta: meta, at: time.Now()})
	s.catalog.Store(newCatalogState(snap, res.RulesVersion))
	res.TotalMs = time.Since(started).Milliseconds()
	zap.L().Info("импорт каталога", zap.String("version", res.CatalogVersion), zap.Int("products", res.Products),
		zap.String("index", res.Index), zap.Bool("vectors", res.Vectors), zap.Int64("embed_ms", res.EmbedMs),
		zap.Int64("index_ms", res.IndexMs), zap.Int64("total_ms", res.TotalMs))
	return res, nil
}

// embedDocs — векторы документов пачками по embedBatch. Всё или ничего: при сбое любой пачки векторы
// снимаются со всех документов — половина индекса с векторами исказила бы dense-выдачу.
func (s *Services) embedDocs(ctx context.Context, docs []indexDoc) (bool, error) {
	if !s.embedder.Enabled() {
		return false, fmt.Errorf("сервис моделей не настроен (EMBEDDER_URL)")
	}
	for from := 0; from < len(docs); from += embedBatch {
		to := min(from+embedBatch, len(docs))
		texts := make([]string, 0, to-from)
		for _, d := range docs[from:to] {
			texts = append(texts, embeddingText(d))
		}
		vecs, err := s.embedder.Embed(ctx, texts, embeddingDim)
		if err != nil {
			for i := range docs {
				docs[i].Embedding = nil
			}
			return false, err
		}
		for i, v := range vecs {
			docs[from+i].Embedding = v
		}
	}
	return true, nil
}

// prepareIndex — новый индекс <алиас>_v{N+1} рядом с текущим: документы, refresh, сверка числа. Алиас
// не трогается. Сбой удаляет недостроенный индекс. existing — индексы алиаса до импорта.
func (s *Services) prepareIndex(ctx context.Context, docs []indexDoc, meta indexMeta) (string, []string, error) {
	existing, err := s.search.Indices(ctx, s.alias+"_v*")
	if err != nil {
		return "", nil, err
	}
	next := 1
	for _, name := range existing {
		next = max(next, indexVersion(s.alias, name)+1)
	}
	name := indexName(s.alias, next)
	if err := s.search.CreateIndex(ctx, name, indexBody(meta)); err != nil {
		return "", nil, err
	}
	fail := func(err error) (string, []string, error) {
		s.dropIndex(ctx, name)
		return "", nil, err
	}
	for from := 0; from < len(docs); from += bulkBatch {
		body, err := bulkBody(name, docs[from:min(from+bulkBatch, len(docs))])
		if err != nil {
			return fail(err)
		}
		if err := s.search.Bulk(ctx, body); err != nil {
			return fail(err)
		}
	}
	if err := s.search.Refresh(ctx, name); err != nil {
		return fail(err)
	}
	if n, err := s.search.Count(ctx, name); err != nil {
		return fail(err)
	} else if n != len(docs) {
		return fail(fmt.Errorf("в индексе %d документов из %d", n, len(docs)))
	}
	return name, existing, nil
}

// switchAlias — атомарно перевести алиас на новый индекс и удалить старше предыдущего (предыдущий
// остаётся для отката).
func (s *Services) switchAlias(ctx context.Context, name string, existing []string) error {
	prev, err := s.search.AliasIndices(ctx, s.alias)
	if err != nil {
		return err
	}
	if err := s.search.SwapAlias(ctx, s.alias, name, prev); err != nil {
		return err
	}
	next := indexVersion(s.alias, name)
	for _, old := range existing {
		if v := indexVersion(s.alias, old); v > 0 && v <= next-keepIndices {
			if err := s.search.DeleteIndex(ctx, old); err != nil {
				zap.L().Warn("импорт: старый индекс не удалён", zap.String("index", old), zap.Error(err))
			}
		}
	}
	return nil
}

// dropIndex — удалить недостроенный индекс; отменённый контекст импорта уборке не мешает.
func (s *Services) dropIndex(ctx context.Context, name string) {
	if err := s.search.DeleteIndex(context.WithoutCancel(ctx), name); err != nil {
		zap.L().Warn("импорт: недостроенный индекс не удалён", zap.String("index", name), zap.Error(err))
	}
}

// bulkBody — NDJSON для _bulk: index-действие с _id = id позиции.
func bulkBody(index string, docs []indexDoc) ([]byte, error) {
	var b bytes.Buffer
	enc := json.NewEncoder(&b)
	for _, d := range docs {
		if err := enc.Encode(map[string]any{"index": map[string]string{"_index": index, "_id": d.ProductID}}); err != nil {
			return nil, err
		}
		if err := enc.Encode(d); err != nil {
			return nil, err
		}
	}
	return b.Bytes(), nil
}
