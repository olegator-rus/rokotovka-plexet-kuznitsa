package services

import (
	"math"
	"testing"

	"kuznica/api/domain"
)

func hits(ids ...string) []domain.RankedHit {
	out := make([]domain.RankedHit, len(ids))
	for i, id := range ids {
		out[i] = domain.RankedHit{ProductID: id, Rank: i + 1}
	}
	return out
}

// RRF: Σ 1/(k+rank); порядок по сумме, ранги источников сохраняются, релевантность нормирована на n/(k+1).
func TestRRF(t *testing.T) {
	got := rrf(60, hits("a", "b", "c"), hits("c", "a", "d"))
	order := []string{}
	for _, f := range got {
		order = append(order, f.ProductID)
	}
	if want := []string{"a", "c", "b", "d"}; len(order) != 4 || order[0] != want[0] || order[1] != want[1] || order[2] != want[2] || order[3] != want[3] {
		t.Fatalf("порядок %v, ожидался %v", order, want)
	}
	a := got[0]
	if math.Abs(a.RRF-(1.0/61+1.0/62)) > 1e-12 || *a.BM25Rank != 1 || *a.DenseRank != 2 {
		t.Fatalf("a: %+v", a)
	}
	if math.Abs(a.Relevance-(1.0/61+1.0/62)/(2.0/61)) > 1e-12 {
		t.Fatalf("нормировка a: %v", a.Relevance)
	}
	if b := got[2]; b.DenseRank != nil || *b.BM25Rank != 2 {
		t.Fatalf("b найден только BM25: %+v", b)
	}
	if d := got[3]; d.BM25Rank != nil || *d.DenseRank != 3 {
		t.Fatalf("d найден только dense: %+v", d)
	}
}

// Один список (dense пропущен): первое место — релевантность 1; пусто — nil.
func TestRRFSingleListAndEmpty(t *testing.T) {
	got := rrf(60, hits("x", "y"), nil)
	if len(got) != 2 || got[0].Relevance != 1 || got[1].Relevance >= 1 {
		t.Fatalf("один список: %+v", got)
	}
	if rrf(60, nil, nil) != nil {
		t.Fatal("без списков — nil")
	}
}

// Равные суммы — детерминированно: лучший ранг BM25, затем id.
func TestRRFTieBreak(t *testing.T) {
	got := rrf(60, hits("b"), hits("a"))
	if got[0].ProductID != "b" || got[1].ProductID != "a" {
		t.Fatalf("равенство решает BM25: %+v", got)
	}
}
