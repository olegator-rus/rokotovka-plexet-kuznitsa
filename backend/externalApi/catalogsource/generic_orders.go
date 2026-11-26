package catalogsource

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"time"

	"kuznica/api/domain"
)

// Заказы и разметка блока контура коммерции в Generic: статистика совместных покупок
// (services/commerce_copurchase.go) берёт заказы из событий order с order_id и account_id.

const (
	// RestoredOrderPrefix — номер заказа, восстановленного по событиям (в данных номера не было).
	RestoredOrderPrefix = "rst-"
	// OrderRestoreWindow — окно восстановления: события order одного покупателя в пределах окна от первого
	// события заказа — один заказ.
	OrderRestoreWindow = 30 * time.Minute
)

// Поля отчёта о происхождении заказов (FieldReport.Field): рядом с полями каталога, но с префиксом.
const (
	FieldOrderID   = "orders.order_id"
	FieldAccountID = "orders.account_id"
	FieldQuantity  = "orders.quantity"
)

// IsRestoredOrder — номер заказа восстановлен правилом, а не взят из данных.
func IsRestoredOrder(id string) bool { return strings.HasPrefix(id, RestoredOrderPrefix) }

// restoreOrders — недостающее у событий order. Нет аккаунта — client_id. Нет номера заказа — события order
// одного покупателя (аккаунт, иначе настоящая сессия) в пределах OrderRestoreWindow от первого события —
// один заказ с номером RestoredOrderPrefix+N. Гость без сессии остаётся без номера: в статистику не входит.
// События уже отсортированы по времени. Возвращает отчёт по трём полям.
func (l *genericLoader) restoreOrders() []FieldReport {
	if l.m.Events == nil {
		return nil
	}
	var total, idSrc, idRest, accSrc, accClient, qtySrc, noKey int
	type open struct {
		id    string
		start time.Time
	}
	opened := map[string]*open{}
	seq := 0
	restoredOrders := map[string]bool{}
	for i := range l.d.events {
		e := &l.d.events[i]
		if e.Type != domain.EventOrder {
			continue
		}
		total++
		if e.Quantity > 0 {
			qtySrc++
		}
		switch {
		case e.AccountID != "":
			accSrc++
		case e.ClientID != nil && *e.ClientID != "":
			e.AccountID = *e.ClientID
			accClient++
		}
		if e.OrderID != "" {
			idSrc++
			continue
		}
		key := ""
		switch {
		case e.AccountID != "":
			key = "a:" + e.AccountID
		case e.SessionID != "" && !strings.HasPrefix(e.SessionID, "row-"):
			key = "s:" + e.SessionID
		}
		if key == "" {
			noKey++
			continue
		}
		o := opened[key]
		if o == nil || e.TS.Sub(o.start) > OrderRestoreWindow {
			seq++
			o = &open{id: fmt.Sprintf("%s%05d", RestoredOrderPrefix, seq), start: e.TS}
			opened[key] = o
		}
		e.OrderID = o.id
		restoredOrders[o.id] = true
		idRest++
	}
	if total == 0 {
		return []FieldReport{
			{Field: FieldOrderID, Status: FieldAbsent, Note: "событий order в журнале нет"},
			{Field: FieldAccountID, Status: FieldAbsent},
			{Field: FieldQuantity, Status: FieldAbsent},
		}
	}
	if idRest > 0 || noKey > 0 || accClient > 0 {
		note := fmt.Sprintf("заказы: строк order %d; номер из данных %d, восстановлен %d (в %d заказов), без покупателя %d; аккаунт из данных %d, из client_id %d",
			total, idSrc, idRest, len(restoredOrders), noKey, accSrc, accClient)
		for i := range l.d.files {
			if l.d.files[i].Table == "events" {
				l.d.files[i].Notes = append(l.d.files[i].Notes, note)
			}
		}
	}
	idNote := ""
	if idRest > 0 {
		idNote = fmt.Sprintf("номера заказа нет у %d строк: события order одного аккаунта (иначе сессии) в пределах %d мин от первого — один заказ, номер %sN",
			idRest, int(OrderRestoreWindow.Minutes()), RestoredOrderPrefix)
	}
	if noKey > 0 {
		idNote = strings.TrimPrefix(idNote+fmt.Sprintf("; %d строк без аккаунта, клиента и сессии — без заказа", noKey), "; ")
	}
	accNote := ""
	if accClient > 0 {
		accNote = fmt.Sprintf("аккаунта нет у %d строк — взят client_id", accClient)
	}
	qtyNote := ""
	if qtySrc < total {
		qtyNote = "нет количества — считается 1"
	}
	return []FieldReport{
		fieldReport(FieldOrderID, total, idSrc, idRest, idNote),
		fieldReport(FieldAccountID, total, accSrc, accClient, accNote),
		fieldReport(FieldQuantity, total, qtySrc, 0, qtyNote),
	}
}

// fieldReport — статус поля по долям «из данных» и «восстановлено».
func fieldReport(field string, total, src, restored int, note string) FieldReport {
	r := FieldReport{Field: field, Total: total, FromSource: src, Restored: restored, Note: note}
	switch {
	case total > 0 && src == total:
		r.Status = FieldFromSource
	case src == 0 && restored == 0:
		r.Status = FieldAbsent
	case src == 0 && restored >= total:
		r.Status = FieldRestored
	default:
		r.Status = FieldPartial
	}
	return r
}

// companionClassWords — класс пары по-русски; контрактные значения проверяются раньше.
var companionClassWords = []struct{ stem, value string }{
	{"несовмест", "incompatible"}, {"обязат", "required"}, {"услов", "conditional"}, {"совмест", "compatible"},
	{"неизвест", "unknown"}, {"не подтв", "unknown"}, {"сопутств", "complement"}, {"дополн", "complement"},
	{"старш", "upgrade"}, {"апгрейд", "upgrade"}, {"нерелев", "irrelevant"}, {"не подход", "irrelevant"},
	{"лишн", "irrelevant"},
}

func normCompanionClass(raw string) string {
	f := fold(raw)
	if slices.Contains(domain.CompanionPairClasses, f) {
		return f
	}
	for _, w := range companionClassWords {
		if strings.Contains(f, w.stem) {
			return w.value
		}
	}
	return ""
}

// loadCompanionPairs — размеченные пары блока: концы — id или артикул позиции.
func (l *genericLoader) loadCompanionPairs(tb *table) error {
	l.d.pairs = []domain.CompanionPair{}
	for _, rec := range tb.td.rows {
		a, c := l.productID(tb.get(rec, "anchor_product")), l.productID(tb.get(rec, "candidate_product"))
		if a == "" || c == "" {
			tb.skip("позиция пары не найдена")
			continue
		}
		cls := normCompanionClass(tb.get(rec, "class"))
		if cls == "" {
			tb.skip("неизвестный класс пары")
			continue
		}
		l.d.pairs = append(l.d.pairs, domain.CompanionPair{AnchorProduct: a, CandidateProduct: c, Class: cls,
			Query: tb.get(rec, "query")})
	}
	return nil
}

// LoadCompanionPairs — размеченные пары блока допродажи (companion_pairs из mapping.yaml); нет в
// сопоставлении — ErrNoFile.
func (g *Generic) LoadCompanionPairs(ctx context.Context) ([]domain.CompanionPair, error) {
	d, err := g.data(ctx)
	if err != nil {
		return nil, err
	}
	if d.pairs == nil {
		return nil, ErrNoFile
	}
	return append([]domain.CompanionPair{}, d.pairs...), nil
}
