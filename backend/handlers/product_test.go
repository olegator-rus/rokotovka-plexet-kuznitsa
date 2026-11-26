package handlers

import (
	"encoding/json"
	"io"
	"net/http/httptest"
	"testing"

	"kuznica/api/externalApi/catalogsource"
	"kuznica/api/out"
	"kuznica/api/services"

	"github.com/gofiber/fiber/v2"
)

// productApp — вход сотрудника включён (токен задан), каталог фикстуры в памяти, без OpenSearch и базы.
func productApp(t *testing.T) *fiber.App {
	t.Helper()
	svc := services.NewServices(nil, services.Options{Source: catalogsource.MockJSON{Dir: "../testdata/catalog"}})
	svc.LoadCatalogState(t.Context())
	app := fiber.New(ServerConfig(DefaultTimeouts))
	app.Use(RequestID)
	RegisterRoutes(app, NewHandlerManager(svc, Options{PublicOrigin: testOrigin, BackofficeToken: testBackofficeToken}))
	return app
}

func getProduct(t *testing.T, app *fiber.App, id string) (int, []byte, string) {
	t.Helper()
	resp, err := app.Test(httptest.NewRequest("GET", "/api/products/"+id, nil), -1)
	if err != nil {
		t.Fatal(err)
	}
	raw, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, raw, resp.Header.Get("X-Request-Id")
}

// Страница товара открыта гостю без cookie и токена: 200 и позиция каталога.
func TestProductEndpointGuest(t *testing.T) {
	code, raw, _ := getProduct(t, productApp(t), "kis-2dev-1y")
	var p out.ProductPage
	if code != fiber.StatusOK || json.Unmarshal(raw, &p) != nil || p.ProductID != "kis-2dev-1y" || p.Title == "" ||
		p.Variants == nil || p.Attributes == nil {
		t.Fatalf("гость: %d %s", code, raw)
	}
}

// Неизвестная позиция — 404 в едином конверте с traceId = X-Request-Id; id длиннее 128 — ошибка поля id.
func TestProductEndpointErrors(t *testing.T) {
	app := productApp(t)
	code, raw, rid := getProduct(t, app, "net-takogo")
	var env out.ErrorBody
	if code != fiber.StatusNotFound || json.Unmarshal(raw, &env) != nil || env.Error.Code != "NOT_FOUND" ||
		env.Error.TraceID == "" || env.Error.TraceID != rid {
		t.Fatalf("404: %d %s", code, raw)
	}
	long := make([]byte, 129)
	for i := range long {
		long[i] = 'x'
	}
	code, raw, _ = getProduct(t, app, string(long))
	env = out.ErrorBody{}
	if code != fiber.StatusUnprocessableEntity || json.Unmarshal(raw, &env) != nil || len(env.Error.Details) != 1 ||
		env.Error.Details[0].Field != "id" {
		t.Fatalf("id > 128: %d %s", code, raw)
	}
}
