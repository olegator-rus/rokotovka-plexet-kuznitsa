//go:build integration

package services

import (
	"context"
	"os"
	"testing"
	"time"

	"kuznica/api/externalApi/opensearch"
	"kuznica/api/externalApi/tei"
)

// Живой тест на настоящем OpenSearch (и сервисе моделей, если задан):
//
//	OPENSEARCH_URL=http://127.0.0.1:9201 EMBEDDER_URL=http://127.0.0.1:8089 go test -tags integration ./services/ -run Live
//
// Индекс — свой алиас itest_products, индекс разработки products не трогается; после теста удаляется.
func TestLiveImportAndSearch(t *testing.T) {
	url := os.Getenv("OPENSEARCH_URL")
	if url == "" {
		t.Skip("OPENSEARCH_URL не задан")
	}
	osc := opensearch.New(url, 30*time.Second)
	svc := NewServices(nil, Options{Search: osc, Embedder: tei.New(os.Getenv("EMBEDDER_URL"), 2*time.Minute),
		Source: fixtureSource(), IndexAlias: "itest_products"})
	t.Cleanup(func() {
		names, _ := osc.Indices(context.Background(), "itest_products_v*")
		for _, n := range names {
			_ = osc.DeleteIndex(context.Background(), n)
		}
	})
	res, err := svc.ImportCatalog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("импорт: %d позиций, векторы %v, embed %d мс, индекс %d мс", res.Products, res.Vectors, res.EmbedMs, res.IndexMs)

	env := &testEnv{svc: svc}
	// B0: буквальная подстрока, ё и регистр не мешают.
	if r := search(t, env, "B0", "kaspersky internet", ""); r.Total != 1 || len(r.Items[0].Variants) != 1 {
		t.Fatalf("B0: %+v", r.Items)
	}
	// B1: морфология (title.ru), алиасы и артикул.
	for q, family := range map[string]string{
		"касперский":  "kaspersky-internet-security",
		"KL4541RAKFS": "kaspersky-small-office-security",
		"бухгалтерии": "1c-accounting",
		"фотошоп":     "adobe-photoshop",
		"Microsoft Office 2021 для дома и бизнеса": "microsoft-office-2021",
	} {
		r := search(t, env, "B1", q, "")
		if len(r.Items) == 0 || r.Items[0].FamilyID != family {
			t.Fatalf("B1 %q: %+v", q, r.Items)
		}
	}
	r := search(t, env, "B2", "программа для ретуши фотографий", "")
	_, by := stepsByName(t, r.Trace)
	t.Logf("B2: dense skipped=%v reason=%v, первая позиция %s, %.1f мс", by["dense"]["skipped"], by["dense"]["reason"],
		r.Items[0].FamilyID, r.Trace.TotalMs)
	if res.Vectors && by["dense"]["skipped"] == true {
		t.Fatalf("векторы есть, а dense пропущен: %v", by["dense"])
	}
}
