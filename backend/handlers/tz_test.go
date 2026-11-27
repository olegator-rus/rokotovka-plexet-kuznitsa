package handlers

import (
	"bytes"
	"encoding/json"
	"mime/multipart"
	"net/http/httptest"
	"strings"
	"testing"

	"kuznica/api/out"

	"github.com/gofiber/fiber/v2"
)

func tzUpload(t *testing.T, app *fiber.App, name string, data []byte) *httptestResp {
	t.Helper()
	var b bytes.Buffer
	mw := multipart.NewWriter(&b)
	fw, _ := mw.CreateFormFile("file", name)
	_, _ = fw.Write(data)
	_ = mw.Close()
	req := httptest.NewRequest("POST", "/api/tz", &b)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Origin", testOrigin)
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	var body bytes.Buffer
	_, _ = body.ReadFrom(resp.Body)
	return &httptestResp{status: resp.StatusCode, body: body.Bytes()}
}

type httptestResp struct {
	status int
	body   []byte
}

// Тело больше 1 МиБ — 413 на всех маршрутах, кроме загрузки ТЗ; загрузка ТЗ принимает файл больше 1 МиБ.
func TestTZBodyLimit(t *testing.T) {
	app := newTestApp()
	big := `{"query":"` + strings.Repeat("a", MaxRequestBody) + `"}`
	req := httptest.NewRequest("POST", "/api/search", strings.NewReader(big))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Origin", testOrigin)
	resp, err := app.Test(req, 10_000)
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != fiber.StatusRequestEntityTooLarge {
		t.Fatalf("поиск с телом больше 1 МиБ: %d", resp.StatusCode)
	}
	// 2 МиБ «PDF» без текста — принят, отказ уже по содержимому (операция), а не по размеру тела.
	data := append([]byte("%PDF-1.7\n"), bytes.Repeat([]byte{' '}, 2<<20)...)
	r := tzUpload(t, app, "big.pdf", data)
	if r.status != fiber.StatusOK {
		t.Fatalf("загрузка 2 МиБ: %d %s", r.status, r.body)
	}
	var up out.TZUpload
	if err := json.Unmarshal(r.body, &up); err != nil || up.OperationID == "" {
		t.Fatalf("ответ: %s", r.body)
	}
	// Неподдерживаемый формат — операция сразу в error/format.
	r = tzUpload(t, app, "a.exe", []byte("MZ"))
	if err := json.Unmarshal(r.body, &up); err != nil || up.Status != "error" || up.Reason != "format" {
		t.Fatalf("формат: %d %s", r.status, r.body)
	}
	resp, _ = app.Test(httptest.NewRequest("GET", "/api/tz/operations/"+up.OperationID, nil))
	var op out.TZOperation
	_ = json.NewDecoder(resp.Body).Decode(&op)
	if resp.StatusCode != 200 || op.Status != "error" || op.Reason != "format" {
		t.Fatalf("операция: %d %+v", resp.StatusCode, op)
	}
}

// Маршруты ТЗ — гостевые: гость получает 404 по неизвестному хешу и операции, а не 401.
func TestTZRoutesPublic(t *testing.T) {
	app := newTestApp()
	for _, c := range []struct{ method, path string }{
		{"GET", "/api/tz/" + strings.Repeat("a", 64)},
		{"GET", "/api/tz/operations/nope"},
		{"DELETE", "/api/tz/operations/nope"},
	} {
		req := httptest.NewRequest(c.method, c.path, nil)
		req.Header.Set("Origin", testOrigin)
		resp, err := app.Test(req)
		if err != nil {
			t.Fatal(err)
		}
		if resp.StatusCode != fiber.StatusNotFound {
			t.Fatalf("%s %s: %d", c.method, c.path, resp.StatusCode)
		}
	}
	if !isPublic("GET", "/api/tz/abc") || isPublic("GET", "/api/tz/a/b") || isPublic("POST", "/api/tz/abc") ||
		!isPublic("POST", "/api/tz") || isPublic("PUT", "/api/tz/operations/x") {
		t.Fatal("tzPublic")
	}
}
