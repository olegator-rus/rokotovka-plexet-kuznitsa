package ocr

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

// Ответ Vision OCR: координаты строками, слова с номером строки разметки; ключ — в заголовке, не в ошибке.
func TestVisionRecognize(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Api-Key secret" || r.Header.Get("x-folder-id") != "f1" {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"error":"denied secret"}`))
			return
		}
		_, _ = w.Write([]byte(`{"result":{"textAnnotation":{"blocks":[{"lines":[{"words":[
			{"text":"Сервер","boundingBox":{"vertices":[{"x":"10","y":"20"},{"x":"10","y":"40"},{"x":"80","y":"40"},{"x":"80","y":"20"}]}},
			{"text":"2U","boundingBox":{"vertices":[{"x":90,"y":20},{"x":120,"y":40}]}}]}]}]}}}`))
	}))
	defer srv.Close()
	words, err := New(srv.URL, "secret", "f1", 0).Recognize(context.Background(), []byte{0xFF, 0xD8}, "JPEG")
	if err != nil || len(words) != 2 || words[0].Text != "Сервер" || words[0].X0 != 10 || words[0].Y1 != 40 ||
		words[1].X1 != 120 || words[0].Line != words[1].Line {
		t.Fatalf("%+v %v", words, err)
	}
	_, err = New(srv.URL, "secret", "other", 0).Recognize(context.Background(), nil, "JPEG")
	var e *Error
	if !errors.As(err, &e) || e.Status != 403 || strings.Contains(err.Error(), "secret") {
		t.Fatalf("ошибка: %v", err)
	}
}

func TestParseTSV(t *testing.T) {
	tsv := "level\tpage_num\tblock_num\tpar_num\tline_num\tword_num\tleft\ttop\twidth\theight\tconf\ttext\n" +
		"4\t1\t1\t1\t1\t0\t10\t20\t200\t20\t-1\t\n" +
		"5\t1\t1\t1\t1\t1\t10\t20\t60\t20\t95\tСервер\n" +
		"5\t1\t1\t1\t1\t2\t80\t20\t30\t20\t95\t2U\n" +
		"5\t1\t2\t1\t1\t1\t300\t20\t30\t20\t95\tшт.\n"
	words, err := parseTSV(tsv)
	if err != nil || len(words) != 3 || words[1].X1 != 110 || words[0].Line != words[1].Line || words[2].Line == words[0].Line {
		t.Fatalf("%+v %v", words, err)
	}
	if _, err := parseTSV("мусор"); err == nil {
		t.Fatal("не TSV")
	}
}

type failOCR struct{}

func (failOCR) Recognize(context.Context, []byte, string) ([]Word, error) {
	return nil, errors.New("нет")
}

type okOCR struct{}

func (okOCR) Recognize(context.Context, []byte, string) ([]Word, error) {
	return []Word{{Text: "да"}}, nil
}

func TestChain(t *testing.T) {
	if w, err := (Chain{failOCR{}, okOCR{}}).Recognize(context.Background(), nil, ""); err != nil || len(w) != 1 {
		t.Fatalf("%v %v", w, err)
	}
	if _, err := (Chain{}).Recognize(context.Background(), nil, ""); err == nil {
		t.Fatal("пустая цепочка")
	}
}
