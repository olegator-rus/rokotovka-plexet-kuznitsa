package services

import (
	"testing"

	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/opensearch/opensearchtest"
	"kuznica/api/externalApi/tei"
	"kuznica/api/externalApi/tei/teitest"
)

// fixtureSource — маленький каталог backend/testdata/catalog.
func fixtureSource() catalogsource.MockJSON {
	return catalogsource.MockJSON{Dir: "../testdata/catalog"}
}

// testEnv — сервисы на поддельных OpenSearch и сервисе моделей, без базы.
type testEnv struct {
	svc *Services
	os  *opensearchtest.Server
	tei *teitest.Server
}

// newTestEnv — окружение; withEmbedder=false — EMBEDDER_URL не задан.
func newTestEnv(t *testing.T, withEmbedder bool) *testEnv {
	t.Helper()
	env := &testEnv{os: opensearchtest.New()}
	t.Cleanup(env.os.Close)
	opts := Options{Search: opensearch.New(env.os.URL, 0), Source: fixtureSource()}
	if withEmbedder {
		env.tei = teitest.New()
		t.Cleanup(env.tei.Close)
		opts.Embedder = tei.New(env.tei.URL, 0)
	}
	env.svc = NewServices(nil, opts)
	return env
}

// imported — окружение с импортированной фикстурой.
func imported(t *testing.T, withEmbedder bool) *testEnv {
	t.Helper()
	env := newTestEnv(t, withEmbedder)
	if _, err := env.svc.ImportCatalog(t.Context()); err != nil {
		t.Fatal(err)
	}
	return env
}
