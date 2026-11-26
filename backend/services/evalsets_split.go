package services

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"slices"
	"sort"
	"strings"
	"time"

	"kuznica/api/domain"
)

// Деление на наборы: похожие запросы делятся по кластерам, а не по строкам (ARCHITECTURE §6). Кластер —
// одинаковый нормализованный текст (раскладка, опечатки, алиасы, порядок слов), частая переформулировка
// одного в другой, одно названное семейство, общие выбранные товары, близкие векторы. Наборы tune 40 % / test 40 % / regression 20 % по числу запросов внутри
// каждого типа; кластер, все запросы которого впервые появились в отложенной неделе, — только в test
// (на прошлом его не видели ни настройка, ни агент).

const (
	// setsCosine — косинус векторов, с которого запросы — одно намерение.
	setsCosine = 0.88
	// setsProductJaccard — доля общих товаров с неявной оценкой ≥ 2, с которой запросы — одно намерение.
	setsProductJaccard = 0.5
	// clusterMaxSize — больше запросов в кластер слияниями по переформулировкам и векторам не собирается.
	clusterMaxSize = 12
	// clusterReformMin — переформулировок, с которых два запроса — один кластер.
	clusterReformMin = 2
)

// setShares — целевые доли наборов.
var setShares = []struct {
	set   string
	share float64
}{{domain.EvalSetTune, 0.4}, {domain.EvalSetTest, 0.4}, {domain.EvalSetRegression, 0.2}}

// unionFind — кластеры по индексам запросов.
type unionFind struct{ parent, size []int }

func newUnionFind(n int) *unionFind {
	u := &unionFind{parent: make([]int, n), size: make([]int, n)}
	for i := range u.parent {
		u.parent[i], u.size[i] = i, 1
	}
	return u
}

func (u *unionFind) find(i int) int {
	for u.parent[i] != i {
		u.parent[i] = u.parent[u.parent[i]]
		i = u.parent[i]
	}
	return i
}

// union — слить; capped — не сливать, если кластер станет больше clusterMaxSize.
func (u *unionFind) union(a, b int, capped bool) {
	ra, rb := u.find(a), u.find(b)
	if ra == rb || capped && u.size[ra]+u.size[rb] > clusterMaxSize {
		return
	}
	if u.size[ra] < u.size[rb] {
		ra, rb = rb, ra
	}
	u.parent[rb] = ra
	u.size[ra] += u.size[rb]
}

// clusterKey — нормализованный текст без порядка слов и служебных слов; named — семейства, названные в запросе
// целиком (через запятую, пусто — нет).
func clusterKey(text string, lx *searchLexicon, rules *searchRules) (key, named string) {
	var toks []string
	if lx != nil {
		nr := normalizeFull(text, lx, rules, false)
		toks = nr.tokens
		ent := findEntities(nr, extractAttrs(nr.tokens), lx, rules)
		fams := slices.Clone(ent.named)
		sort.Strings(fams)
		named = strings.Join(fams, ",")
	} else {
		toks = strings.Fields(foldText(text))
	}
	var keep []string
	for _, t := range toks {
		if !lexStopWords[t] {
			keep = append(keep, t)
		}
	}
	sort.Strings(keep)
	return strings.Join(keep, " "), named
}

// clusterQueries — кластер каждого запроса (DerivedQuery.Cluster). vecs — векторы текстов (nil — без них).
func clusterQueries(qs []*DerivedQuery, logq map[string]*setsLogQuery, vecs [][]float32, lx *searchLexicon, rules *searchRules) {
	u := newUnionFind(len(qs))
	byKey, byNamed := map[string]int{}, map[string]int{}
	byText := map[string]int{}
	for i, q := range qs {
		byText[foldQuery(q.Text)] = i
		k, named := clusterKey(q.Text, lx, rules)
		if j, ok := byKey[k]; ok && k != "" {
			u.union(i, j, false)
		} else {
			byKey[k] = i
		}
		// Одно и то же названное семейство («office 2021» и «office 2021 профессиональный плюс»).
		if j, ok := byNamed[named]; ok && named != "" {
			u.union(i, j, true)
		} else if named != "" {
			byNamed[named] = i
		}
	}
	for i, q := range qs {
		lq := logq[foldQuery(q.Text)]
		if lq == nil {
			continue
		}
		for to, n := range lq.reform {
			if j, ok := byText[to]; ok && n >= clusterReformMin {
				u.union(i, j, true)
			}
		}
	}
	// Общие товары: покупатели двух запросов выбирают одно и то же.
	byProduct := map[string][]int{}
	for i, q := range qs {
		for pid, g := range q.Judgments {
			if g >= 2 {
				byProduct[pid] = append(byProduct[pid], i)
			}
		}
	}
	tried := map[[2]int]bool{}
	for _, idx := range byProduct {
		for a := 0; a < len(idx); a++ {
			for b := a + 1; b < len(idx); b++ {
				k := [2]int{idx[a], idx[b]}
				if tried[k] {
					continue
				}
				tried[k] = true
				if productJaccard(qs[idx[a]].Judgments, qs[idx[b]].Judgments) >= setsProductJaccard {
					u.union(idx[a], idx[b], true)
				}
			}
		}
	}
	if len(vecs) == len(qs) {
		for i := range qs {
			for j := i + 1; j < len(qs); j++ {
				if cosine(vecs[i], vecs[j]) >= setsCosine {
					u.union(i, j, true)
				}
			}
		}
	}
	members := map[int][]string{}
	for i, q := range qs {
		r := u.find(i)
		members[r] = append(members[r], foldQuery(q.Text))
	}
	ids := map[int]string{}
	for r, m := range members {
		sort.Strings(m)
		sum := sha256.Sum256([]byte(strings.Join(m, "\x00")))
		ids[r] = "c-" + hex.EncodeToString(sum[:4])
	}
	for i, q := range qs {
		q.Cluster = ids[u.find(i)]
	}
}

// assignSets — наборы по кластерам: внутри типа (тип самого частого запроса кластера) кластеры по хэшу id
// раздаются набору с наибольшим недобором до целевой доли. holdoutFrom — начало отложенной недели.
func assignSets(qs []*DerivedQuery, first map[string]time.Time, holdoutFrom time.Time) {
	type cl struct {
		id      string
		qs      []*DerivedQuery
		typ     string
		holdout bool
	}
	byID := map[string]*cl{}
	var order []*cl
	for _, q := range qs {
		c := byID[q.Cluster]
		if c == nil {
			c = &cl{id: q.Cluster, holdout: true}
			byID[q.Cluster] = c
			order = append(order, c)
		}
		c.qs = append(c.qs, q)
		if f, ok := first[q.ID]; !ok || holdoutFrom.IsZero() || f.Before(holdoutFrom) {
			c.holdout = false
		}
	}
	byType := map[string][]*cl{}
	for _, c := range order {
		top := c.qs[0]
		for _, q := range c.qs {
			if q.Freq > top.Freq || q.Freq == top.Freq && q.ID < top.ID {
				top = q
			}
		}
		c.typ = top.Type
		byType[c.typ] = append(byType[c.typ], c)
	}
	types := make([]string, 0, len(byType))
	for t := range byType {
		types = append(types, t)
	}
	sort.Strings(types)
	for _, t := range types {
		cs := byType[t]
		sort.Slice(cs, func(i, j int) bool { return clusterHash(cs[i].id) < clusterHash(cs[j].id) })
		got := map[string]int{}
		total := 0
		put := func(c *cl, set string) {
			for _, q := range c.qs {
				q.Set = set
			}
			got[set] += len(c.qs)
			total += len(c.qs)
		}
		for _, c := range cs {
			if c.holdout {
				put(c, domain.EvalSetTest)
			}
		}
		for _, c := range cs {
			if c.holdout {
				continue
			}
			best, deficit := setShares[0].set, -1e9
			after := float64(total + len(c.qs))
			for _, sh := range setShares {
				d := sh.share*after - float64(got[sh.set])
				if d > deficit {
					best, deficit = sh.set, d
				}
			}
			put(c, best)
		}
	}
}

func clusterHash(id string) string {
	sum := sha256.Sum256([]byte("sets|" + id))
	return hex.EncodeToString(sum[:8])
}

// queryVectors — векторы нормализованных текстов; эмбеддер не настроен — nil.
func (s *Services) queryVectors(ctx context.Context, qs []*DerivedQuery) [][]float32 {
	texts := make([]string, len(qs))
	for i, q := range qs {
		texts[i] = foldQuery(q.Text)
	}
	return s.embedTexts(ctx, texts)
}

// productJaccard — доля общих товаров с оценкой ≥ 2.
func productJaccard(a, b map[string]int) float64 {
	inter, union := 0, 0
	for p, g := range a {
		if g < 2 {
			continue
		}
		union++
		if b[p] >= 2 {
			inter++
		}
	}
	for p, g := range b {
		if g >= 2 && a[p] < 2 {
			union++
		}
	}
	if union == 0 {
		return 0
	}
	return float64(inter) / float64(union)
}
