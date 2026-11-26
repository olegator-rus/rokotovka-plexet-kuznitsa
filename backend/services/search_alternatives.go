package services

import (
	"sort"
	"strings"
	"sync"
	"unicode/utf8"

	"kuznica/api/domain"
	"kuznica/api/out"
)

// «Можно купить сейчас»: точно названный продукт продаётся только по запросу —
// под ним продукты того же класса и того же назначения, которые можно купить сразу.
//
// Класс — категория второго уровня исходного продукта или соседняя категория того же раздела с тем же словом-типом
// в названии («Операционные системы Windows» ↔ «Российские операционные системы»). Назначение — редакция из
// названия семейства, названия позиции или параметра фида «Назначение»: серверная ↔ серверная, для рабочих станций ↔
// для рабочих станций. Чужое назначение не подставляется: если у исходного продукта редакция не определяется
// (графический редактор, офисный пакет), блока нет — непонятно, что предлагать вместо.
//
// Кандидат подходит, если редакция названа в имени его семейства («Альт Сервер 11») или у семейства есть обе
// редакции — для серверов и для рабочих станций (так продаются ОС и средства защиты); «серверные лицензии»
// программного комплекса без второй редакции (ALD Pro, РЕД Виртуализация) — не серверная ОС. Голова карточки —
// самая доступная базовая лицензия этой редакции в наличии с ценой. Порядок: популярность, затем ширина
// ассортимента вендора в классе (позиций в наличии; на фиде популярность у всех одна); одно семейство на вендора.

// Редакции (назначение) продукта.
const (
	editionServer      = "server"
	editionWorkstation = "workstation"
)

// explainAlternativeEdition — причина альтернативы с названным назначением (: без «замены» и «аналога»).
var explainAlternativeEdition = map[string]string{
	editionServer:      "Тот же класс решения, серверная редакция, можно купить сейчас. Совместимость с вашими приложениями нужно проверить",
	editionWorkstation: "Тот же класс решения, редакция для рабочих станций, можно купить сейчас. Совместимость с вашими приложениями нужно проверить",
}

// serverStems — слова, которые называют серверную редакцию («сервер», «серверная», «server»).
var serverStems = []string{"сервер", "server"}

// notEditionBefore — слово перед «сервер», с которым это не редакция, а метрика или зависимость: «на 1 сервер»,
// «на базе РЕД ОС Сервер», «по количеству … серверов», «под управлением».
var notEditionBefore = setOf("на", "базе", "основе", "управлением", "количеству", "количество", "одном", "одного", "файловых", "почтовых")

// dependsOn — слово, после которого в ближайших словах названа платформа-зависимость, а не редакция.
var dependsOn = setOf("базе", "основе", "управлением")

// editionsOfText — редакции, названные в тексте: серверная и/или для рабочих станций.
func editionsOfText(s string) (server, ws bool) {
	ws0 := wordsOf(s)
	for i, w := range ws0 {
		prev := ""
		if i > 0 {
			prev = ws0[i-1]
		}
		skip := notEditionBefore[prev] || hasDigit(prev)
		for j := max(0, i-3); j < i; j++ {
			skip = skip || dependsOn[ws0[j]] // «на базе РЕД ОС Сервер», «под управлением Astra Linux Server»
		}
		for _, st := range serverStems {
			if strings.HasPrefix(w, st) && !skip {
				server = true
			}
		}
		switch {
		case (w == "desktop" || w == "workstation") && !skip:
			ws = true
		case strings.HasPrefix(w, "рабоч") && i+1 < len(ws0) && strings.HasPrefix(ws0[i+1], "станц") && !skip:
			ws = true
		}
	}
	return server, ws
}

// docEditions — редакции позиции: название и параметр фида «Назначение».
func docEditions(d indexDoc) (server, ws bool) {
	server, ws = editionsOfText(d.Title)
	if v := attrTextValue(d.Attributes, "Назначение"); v != "" {
		f := foldText(v)
		server = server || strings.Contains(f, "сервер") && !strings.Contains(f, "для сервер")
		ws = ws || strings.Contains(f, "рабочая станция")
	}
	return server, ws
}

// attrTextValue — значение параметра из строки атрибутов «ключ: значение; …».
func attrTextValue(attrs, key string) string {
	for _, part := range strings.Split(attrs, "; ") {
		if v, ok := strings.CutPrefix(part, key+": "); ok {
			return v
		}
	}
	return ""
}

// famEditions — редакции семейства: названные в имени и встречающиеся у позиций.
type famEditions struct {
	nameServer, nameWS bool
	server, ws         bool
}

var famEditionsCache sync.Map // famEditionsKey → famEditions

type famEditionsKey struct {
	lx  *searchLexicon
	fid string
}

func (lx *searchLexicon) editionsOf(fid string) famEditions {
	k := famEditionsKey{lx, fid}
	if v, ok := famEditionsCache.Load(k); ok {
		return v.(famEditions)
	}
	var e famEditions
	e.nameServer, e.nameWS = editionsOfText(lx.familyName[fid])
	for _, pid := range lx.families[fid] {
		s, w := docEditions(lx.docs[pid])
		e.server, e.ws = e.server || s, e.ws || w
	}
	famEditionsCache.Store(k, e)
	return e
}

// sourceEdition — назначение исходного продукта: из имени семейства, иначе из головы карточки. "" — не определяется
// или названы обе редакции.
func sourceEdition(lx *searchLexicon, fid string, head indexDoc) string {
	e := lx.editionsOf(fid)
	s, w := e.nameServer, e.nameWS
	if !s && !w {
		s, w = docEditions(head)
	}
	switch {
	case s && !w:
		return editionServer
	case w && !s:
		return editionWorkstation
	}
	return ""
}

// editionFits — семейство продаёт эту редакцию как редакцию: она в имени или у семейства есть обе.
func editionFits(e famEditions, edition string) bool {
	if edition == editionServer {
		return e.nameServer || e.server && e.ws && !e.nameWS
	}
	return e.nameWS || e.server && e.ws && !e.nameServer
}

// docHasEdition — позиция этой редакции (для семейства с редакцией в имени — любая позиция).
func docHasEdition(e famEditions, d indexDoc, edition string) bool {
	if edition == editionServer && e.nameServer || edition == editionWorkstation && e.nameWS {
		return true
	}
	s, w := docEditions(d)
	if edition == editionServer {
		return s && !w
	}
	return w && !s
}

// sameClass — категория кандидата того же класса: та же или соседняя в том же разделе с общим словом-типом.
func (lx *searchLexicon) sameClass(src, cand string) bool {
	if src == "" || cand == "" {
		return false
	}
	if src == cand {
		return true
	}
	if lx.categoryRoot[src] == "" || lx.categoryRoot[src] != lx.categoryRoot[cand] {
		return false
	}
	for _, a := range wordsOf(lx.categoryName[src]) {
		if utf8.RuneCountInString(a) < 5 || lexStopWords[a] {
			continue
		}
		for _, b := range wordsOf(lx.categoryName[cand]) {
			if sameStem(a, b) {
				return true
			}
		}
	}
	return false
}

// sameStem — одно слово в разных формах («операционные» — «операционная»): общее начало не короче пяти букв и
// отличие только в окончании (не больше двух букв у более короткого).
func sameStem(a, b string) bool {
	ra, rb := []rune(a), []rune(b)
	n := 0
	for n < len(ra) && n < len(rb) && ra[n] == rb[n] {
		n++
	}
	return n >= 5 && n >= min(len(ra), len(rb))-2
}

// purposeAlternatives — до n семейств того же класса и назначения, что и src, с базовой лицензией этой редакции
// в наличии с ценой. Вендор запроса здесь не условие (альтернатива — по определению другой продукт); жёсткие
// условия запроса (тип лицензии, организация, количество) остаются — buildFamily.
func purposeAlternatives(rc rankCtx, src out.SearchItem, shown map[string]bool, n int) []out.SearchItem {
	d, ok := rc.lx.docs[src.ProductID]
	if !ok || n <= 0 {
		return nil
	}
	edition := sourceEdition(rc.lx, src.FamilyID, d)
	if edition == "" {
		return nil
	}
	srcCat := docCat2(d)
	free := rc
	free.ent.vendors = nil
	type cand struct {
		item   out.SearchItem
		vendor string
		pop    float64
		named  bool
		offers int
		pos    int
	}
	var cs []cand
	breadth := map[string]int{}
	var sink []domain.RejectedHit
	for pos, fid := range rc.lx.familyOrder {
		if !rc.lx.sameClass(srcCat, famCat2(rc.lx, fid)) {
			continue
		}
		// Ширина ассортимента вендора в классе: позиции в наличии с ценой во всех его семействах класса.
		for _, pid := range rc.lx.families[fid] {
			if buyable(rc.lx.docs[pid]) {
				breadth[rc.lx.familyVendor[fid]]++
			}
		}
		if shown[fid] {
			continue
		}
		e := rc.lx.editionsOf(fid)
		if !editionFits(e, edition) {
			continue
		}
		sink = sink[:0]
		fh := free.buildFamily(fid, 0, &sink)
		if fh == nil {
			continue
		}
		var head *indexDoc
		offers := 0
		for i, x := range fh.heads {
			if !fh.fits[x.ProductID] || !buyable(x) || kindRank(rc.lx.kindOf(x), rc.wantKinds) > 1 || !docHasEdition(e, x, edition) {
				continue
			}
			offers++
			if head == nil || *x.PriceRub < *head.PriceRub {
				head = &fh.heads[i] // цена «от»: самая доступная позиция этой редакции
			}
		}
		if head == nil {
			continue
		}
		fh.group = domain.GroupAnalog
		it := free.cardOf(fh, *head, nil)
		it.MatchType, it.Explain = out.MatchAlternative, explainAlternativeEdition[edition]
		v := rc.lx.familyVendor[fid]
		cs = append(cs, cand{it, v, head.Popularity, e.nameServer && edition == editionServer || e.nameWS && edition == editionWorkstation, offers, pos})
	}
	sort.SliceStable(cs, func(i, j int) bool {
		a, b := cs[i], cs[j]
		if a.pop != b.pop {
			return a.pop > b.pop
		}
		if breadth[a.vendor] != breadth[b.vendor] {
			return breadth[a.vendor] > breadth[b.vendor]
		}
		if a.vendor != b.vendor {
			return a.pos < b.pos
		}
		if a.named != b.named {
			return a.named // у вендора — семейство, названное этой редакцией («Альт Сервер 11»)
		}
		if a.offers != b.offers {
			return a.offers > b.offers
		}
		return a.pos < b.pos
	})
	var res []out.SearchItem
	used := map[string]bool{}
	for _, c := range cs {
		if len(res) >= n {
			break
		}
		if used[c.vendor] {
			continue
		}
		used[c.vendor] = true
		shown[c.item.FamilyID] = true
		res = append(res, c.item)
	}
	return res
}
