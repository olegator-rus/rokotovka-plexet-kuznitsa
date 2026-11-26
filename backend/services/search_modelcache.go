package services

import (
	"container/list"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"sync"
	"time"

	"kuznica/api/externalApi/tei"

	"golang.org/x/sync/singleflight"
)

// Кэш вызовов сервиса моделей: вектор запроса и оценки реранкера зависят только от
// текста, поэтому повтор того же запроса в эмбеддер и реранкер не ходит. Одинаковые одновременные промахи
// ждут один вызов (singleflight). Ошибки не кэшируются. Размер и срок — SEARCH_MODEL_CACHE_SIZE/_TTL.

// lru — потокобезопасный LRU со сроком жизни записи.
type lru[V any] struct {
	mu    sync.Mutex
	size  int
	ttl   time.Duration
	now   func() time.Time
	order *list.List // голова — самая свежая запись
	items map[string]*list.Element
}

type lruEntry[V any] struct {
	key     string
	val     V
	expires time.Time
}

// newLRU — кэш на size записей; size <= 0 — nil (кэша нет).
func newLRU[V any](size int, ttl time.Duration, now func() time.Time) *lru[V] {
	if size <= 0 {
		return nil
	}
	if now == nil {
		now = time.Now
	}
	return &lru[V]{size: size, ttl: ttl, now: now, order: list.New(), items: map[string]*list.Element{}}
}

func (c *lru[V]) get(key string) (V, bool) {
	var zero V
	if c == nil {
		return zero, false
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	el, ok := c.items[key]
	if !ok {
		return zero, false
	}
	e := el.Value.(*lruEntry[V])
	if c.ttl > 0 && !c.now().Before(e.expires) {
		c.order.Remove(el)
		delete(c.items, key)
		return zero, false
	}
	c.order.MoveToFront(el)
	return e.val, true
}

func (c *lru[V]) put(key string, val V) {
	if c == nil {
		return
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	exp := c.now().Add(c.ttl)
	if el, ok := c.items[key]; ok {
		e := el.Value.(*lruEntry[V])
		e.val, e.expires = val, exp
		c.order.MoveToFront(el)
		return
	}
	c.items[key] = c.order.PushFront(&lruEntry[V]{key: key, val: val, expires: exp})
	for c.order.Len() > c.size {
		last := c.order.Back()
		c.order.Remove(last)
		delete(c.items, last.Value.(*lruEntry[V]).key)
	}
}

func (c *lru[V]) len() int {
	if c == nil {
		return 0
	}
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.order.Len()
}

// modelCache — кэш вектора запроса и оценок реранкера.
type modelCache struct {
	vectors *lru[[]float32]
	rerank  *lru[[]tei.RerankHit]
	flight  singleflight.Group
}

func newModelCache(size int, ttl time.Duration, now func() time.Time) *modelCache {
	if size <= 0 {
		return nil
	}
	return &modelCache{vectors: newLRU[[]float32](size, ttl, now), rerank: newLRU[[]tei.RerankHit](size, ttl, now)}
}

// queryVector — вектор текста запроса: из кэша или от эмбеддера. Ключ — текст как он уходит в эмбеддер
// (уже нормализован поиском). Срезы из кэша только читаются.
func (s *Services) queryVector(ctx context.Context, text string) ([]float32, error) {
	mc := s.models
	if mc == nil {
		vecs, err := s.embedder.Embed(ctx, []string{text}, embeddingDim)
		if err != nil {
			return nil, err
		}
		return vecs[0], nil
	}
	if v, ok := mc.vectors.get(text); ok {
		return v, nil
	}
	ch := mc.flight.DoChan("v\x00"+text, func() (any, error) {
		// Вызов общий для всех ждущих: уход первого покупателя его не отменяет, срок — как у ожидания вектора.
		fctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), queryEmbedLimit)
		defer cancel()
		vecs, err := s.embedder.Embed(fctx, []string{text}, embeddingDim)
		if err != nil {
			return nil, err
		}
		mc.vectors.put(text, vecs[0])
		return vecs[0], nil
	})
	select {
	case r := <-ch:
		if r.Err != nil {
			return nil, r.Err
		}
		return r.Val.([]float32), nil
	case <-ctx.Done():
		return nil, &tei.Error{Op: "embed", Err: ctx.Err()}
	}
}

// rerankCached — оценки реранкера: из кэша или от сервиса. Ключ — хэш запроса и текстов в их порядке.
func (s *Services) rerankCached(ctx context.Context, query string, texts []string) ([]tei.RerankHit, error) {
	mc := s.models
	if mc == nil {
		return s.reranker.Rerank(ctx, query, texts)
	}
	h := sha256.New()
	h.Write([]byte(query))
	for _, t := range texts {
		h.Write([]byte{0})
		h.Write([]byte(t))
	}
	key := hex.EncodeToString(h.Sum(nil))
	if v, ok := mc.rerank.get(key); ok {
		return v, nil
	}
	v, err, _ := mc.flight.Do("r\x00"+key, func() (any, error) {
		hits, err := s.reranker.Rerank(ctx, query, texts)
		if err != nil {
			return nil, err
		}
		mc.rerank.put(key, hits)
		return hits, nil
	})
	if err != nil {
		return nil, err
	}
	return v.([]tei.RerankHit), nil
}
