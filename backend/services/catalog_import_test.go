package services

import (
	"slices"
	"strings"
	"testing"
)

// Импорт фикстуры: счётчики, векторы, индекс products_v1 за алиасом products, документы целиком.
func TestImportFixture(t *testing.T) {
	env := newTestEnv(t, true)
	res, err := env.svc.ImportCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.CatalogVersion != "fixture-1" || res.Products != 12 || res.Families != 11 || res.Vendors != 7 || res.Categories != 14 ||
		res.Relations != 4 || res.Aliases != 6 || res.Clients != 3 || res.Index != "products_v1" || !res.Vectors || res.Database {
		t.Fatalf("сводка: %+v", res)
	}
	if !slices.Contains(res.Warnings, "база не подключена: каталог только в индексе") {
		t.Fatalf("предупреждения: %v", res.Warnings)
	}
	if env.os.Alias("products") != "products_v1" {
		t.Fatalf("алиас: %q", env.os.Alias("products"))
	}
	docs := env.os.Docs("products")
	if len(docs) != 12 {
		t.Fatalf("документов %d", len(docs))
	}
	kis := docs["kis-2dev-1y"]
	if emb, _ := kis["embedding"].([]any); len(emb) != 1024 || kis["vendor"] != "Kaspersky" || kis["license_quantity"].(float64) != 2 {
		t.Fatalf("документ: %v", kis)
	}
	// Векторы пачками по 32: 12 позиций — один вызов.
	if n := env.tei.EmbedCalls.Load(); n != 1 {
		t.Fatalf("вызовов /embed %d", n)
	}
	if m := env.svc.Meta(); m.CatalogVersion != "fixture-1" || len(m.Clients) != 3 || m.RulesVersion != 1 {
		t.Fatalf("meta после импорта: %+v", m)
	}
}

// Повторный импорт идемпотентен: новый индекс с теми же документами, алиас переключён, старше
// предыдущего индексы удалены.
func TestImportIdempotent(t *testing.T) {
	env := imported(t, true)
	first := env.os.Docs("products")
	for i := 0; i < 2; i++ {
		if _, err := env.svc.ImportCatalog(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
	if env.os.Alias("products") != "products_v3" {
		t.Fatalf("алиас: %s", env.os.Alias("products"))
	}
	if got := env.os.IndexNames(); !slices.Equal(got, []string{"products_v2", "products_v3"}) {
		t.Fatalf("индексы: %v (храним текущий и предыдущий)", got)
	}
	again := env.os.Docs("products")
	if len(again) != len(first) || again["kis-2dev-1y"]["title"] != first["kis-2dev-1y"]["title"] {
		t.Fatal("повторный импорт изменил документы")
	}
}

// Сервис моделей не настроен или лежит — индекс строится без векторов, с предупреждением.
func TestImportWithoutVectors(t *testing.T) {
	env := newTestEnv(t, false)
	res, err := env.svc.ImportCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if res.Vectors || !strings.Contains(strings.Join(res.Warnings, ";"), "индекс без векторов") {
		t.Fatalf("без эмбеддера: %+v", res)
	}
	if _, ok := env.os.Docs("products")["kis-2dev-1y"]["embedding"]; ok {
		t.Fatal("вектор в документе без эмбеддера")
	}

	env = newTestEnv(t, true)
	env.tei.Down.Store(true)
	res, err = env.svc.ImportCatalog(t.Context())
	if err != nil || res.Vectors {
		t.Fatalf("эмбеддер лежит: %+v, %v", res, err)
	}
}

// Без OpenSearch или источника импорт честно отказывает.
func TestImportUnavailable(t *testing.T) {
	if _, err := NewServices(nil, Options{Source: fixtureSource()}).ImportCatalog(t.Context()); err == nil ||
		!strings.Contains(err.Error(), MsgSearchDisabled) {
		t.Fatalf("без OpenSearch: %v", err)
	}
	env := newTestEnv(t, false)
	env.svc.source = nil
	if _, err := env.svc.ImportCatalog(t.Context()); err == nil || !strings.Contains(err.Error(), MsgNoSource) {
		t.Fatalf("без источника: %v", err)
	}
}

// Импорт другим процессом (make import при запущенном API): по истечении кеша _meta API видит новый
// индекс — векторы появились, справочники перечитаны.
func TestImportFromAnotherProcess(t *testing.T) {
	api := newTestEnv(t, true)
	cli := NewServices(nil, Options{Search: api.svc.search, Source: fixtureSource()}) // без эмбеддера
	if _, err := cli.ImportCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	_, by := stepsByName(t, search(t, api, "B2", "касперский", "").Trace)
	if by["dense"]["reason"] != reasonNoVectors || api.svc.Meta().CatalogVersion != "" {
		t.Fatalf("до импорта с векторами: %v, %+v", by["dense"], api.svc.Meta())
	}
	cli.embedder = api.svc.embedder
	if _, err := cli.ImportCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	cached := api.svc.index.Load()
	api.svc.index.Store(&cachedIndex{meta: cached.meta}) // кеш истёк
	_, by = stepsByName(t, search(t, api, "B2", "касперский", "").Trace)
	api.svc.Wait()
	if by["dense"]["skipped"] == true || api.svc.Meta().CatalogVersion != "fixture-1" {
		t.Fatalf("после импорта с векторами: %v, %+v", by["dense"], api.svc.Meta())
	}
}
