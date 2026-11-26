// Package teitest — поддельный сервис моделей с API TEI для тестов. Вектор — нормированный «мешок
// слов» с хешированием в Dim измерений: тексты с общими словами ближе друг к другу, результат
// детерминирован.
package teitest

import (
	"encoding/json"
	"hash/fnv"
	"math"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"unicode"
)

// Dim — размерность векторов (как у BGE-M3).
const Dim = 1024

// Server — поддельный сервис моделей. Down — отвечать 503 на всё.
type Server struct {
	*httptest.Server
	Down       atomic.Bool
	EmbedCalls atomic.Int64
}

// New — запущенный сервис.
func New() *Server {
	s := &Server{}
	s.Server = httptest.NewServer(http.HandlerFunc(s.handle))
	return s
}

func (s *Server) handle(w http.ResponseWriter, r *http.Request) {
	if s.Down.Load() {
		w.WriteHeader(http.StatusServiceUnavailable)
		return
	}
	switch r.URL.Path {
	case "/health":
		w.WriteHeader(http.StatusOK)
	case "/embed":
		s.EmbedCalls.Add(1)
		var body struct {
			Inputs []string `json:"inputs"`
		}
		if json.NewDecoder(r.Body).Decode(&body) != nil {
			w.WriteHeader(http.StatusUnprocessableEntity)
			return
		}
		out := make([][]float32, len(body.Inputs))
		for i, t := range body.Inputs {
			out[i] = Vector(t)
		}
		_ = json.NewEncoder(w).Encode(out)
	default:
		w.WriteHeader(http.StatusNotFound)
	}
}

// Vector — детерминированный нормированный вектор текста.
func Vector(text string) []float32 {
	v := make([]float64, Dim)
	for _, w := range strings.FieldsFunc(strings.ToLower(text), func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }) {
		h := fnv.New32a()
		_, _ = h.Write([]byte(w))
		v[h.Sum32()%Dim]++
	}
	var n float64
	for _, x := range v {
		n += x * x
	}
	out := make([]float32, Dim)
	if n == 0 {
		out[0] = 1
		return out
	}
	n = math.Sqrt(n)
	for i, x := range v {
		out[i] = float32(x / n)
	}
	return out
}
