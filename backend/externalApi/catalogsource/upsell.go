package catalogsource

import (
	"context"
	"encoding/csv"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"kuznica/api/domain"
)

// Справочники допродажи контура фида — CSV рядом с mapping.yaml, по образцу словаря lexicon:
// mapping.compat_docs — совместимость с первичным документом, mapping.promos — акции магазина. Нет поля или
// файла — ErrNoFile, блок работает на связях каталога (уровень B). Строки с «#» в начале — комментарии.

// LoadCompatDocs — записи совместимости из файла mapping.compat_docs.
func (g *Generic) LoadCompatDocs(_ context.Context) ([]domain.CompatDoc, error) {
	if g.mapping.CompatDocs == "" {
		return nil, ErrNoFile
	}
	return ReadCompatDocs(g.mapping.path(g.mapping.CompatDocs))
}

// LoadPromos — акции из файла mapping.promos.
func (g *Generic) LoadPromos(_ context.Context) ([]domain.Promo, error) {
	if g.mapping.Promos == "" {
		return nil, ErrNoFile
	}
	return ReadPromos(g.mapping.path(g.mapping.Promos))
}

// LoadUpsellClasses — классы и таблица классов допродажи (mapping.upsell_classes, mapping.upsell_links).
func (g *Generic) LoadUpsellClasses(_ context.Context) ([]domain.UpsellClass, []domain.UpsellLink, error) {
	if g.mapping.UpsellClasses == "" || g.mapping.UpsellLinks == "" {
		return nil, nil, ErrNoFile
	}
	cls, err := ReadUpsellClasses(g.mapping.path(g.mapping.UpsellClasses))
	if err != nil {
		return nil, nil, err
	}
	links, err := ReadUpsellLinks(g.mapping.path(g.mapping.UpsellLinks), cls)
	if err != nil {
		return nil, nil, err
	}
	return cls, links, nil
}

// ReadUpsellClasses — классы продуктов: id, категории, регулярные выражения названия. Неверное выражение — ошибка.
func ReadUpsellClasses(path string) ([]domain.UpsellClass, error) {
	rows, err := readCSVRows(path, []string{"id", "title", "categories"})
	if err != nil {
		return nil, err
	}
	var out []domain.UpsellClass
	seen := map[string]bool{}
	for _, r := range rows {
		c := domain.UpsellClass{ID: r.get("id"), Title: r.get("title"), Categories: splitPipe(r.get("categories")),
			Include: r.get("include"), Exclude: r.get("exclude"), Source: r.get("source"), Note: r.get("note")}
		switch {
		case c.ID == "" || c.ID == domain.UpsellSelf || c.ID == domain.UpsellSelfVariant:
			return nil, fmt.Errorf("%s:%d: недопустимый id класса %q", path, r.line, c.ID)
		case seen[c.ID]:
			return nil, fmt.Errorf("%s:%d: повтор id %q", path, r.line, c.ID)
		case len(c.Categories) == 0 || c.Title == "":
			return nil, fmt.Errorf("%s:%d: у класса %q нет категорий или названия", path, r.line, c.ID)
		}
		for _, re := range []string{c.Include, c.Exclude} {
			if _, err := regexp.Compile(re); err != nil {
				return nil, fmt.Errorf("%s:%d: выражение %q: %w", path, r.line, re, err)
			}
		}
		seen[c.ID] = true
		out = append(out, c)
	}
	return out, nil
}

// ReadUpsellLinks — таблица классов. Неизвестный класс, пустая причина или источник — ошибка.
func ReadUpsellLinks(path string, classes []domain.UpsellClass) ([]domain.UpsellLink, error) {
	rows, err := readCSVRows(path, []string{"from", "to", "reason", "source"})
	if err != nil {
		return nil, err
	}
	known := map[string]bool{domain.UpsellSelf: true, domain.UpsellSelfVariant: true}
	for _, c := range classes {
		known[c.ID] = true
	}
	var out []domain.UpsellLink
	for _, r := range rows {
		l := domain.UpsellLink{From: r.get("from"), To: r.get("to"), Kinds: splitPipe(r.get("kinds")), Max: 1,
			RequireDoc: r.get("require_doc") == "yes", Reason: r.get("reason"), Source: r.get("source"),
			AddedAt: r.get("added_at")}
		if v := r.get("max"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n < 1 || n > 4 {
				return nil, fmt.Errorf("%s:%d: max %q (1–4)", path, r.line, v)
			}
			l.Max = n
		}
		switch {
		case !known[l.From] || l.From == domain.UpsellSelf || l.From == domain.UpsellSelfVariant:
			return nil, fmt.Errorf("%s:%d: неизвестный класс запроса %q", path, r.line, l.From)
		case !known[l.To]:
			return nil, fmt.Errorf("%s:%d: неизвестный класс дополнения %q", path, r.line, l.To)
		case l.Reason == "" || l.Source == "":
			return nil, fmt.Errorf("%s:%d: у связи %s → %s нет причины или источника", path, r.line, l.From, l.To)
		case l.AddedAt != "" && !isISODate(l.AddedAt):
			return nil, fmt.Errorf("%s:%d: added_at %q (YYYY-MM-DD)", path, r.line, l.AddedAt)
		}
		out = append(out, l)
	}
	return out, nil
}

var compatLevels = map[string]bool{domain.EvidenceA: true, domain.EvidenceC: true}

var compatDocKinds = map[string]bool{"certificate": true, "protocol": true, "documentation": true,
	"instruction": true, "catalog": true}

// ReadCompatDocs — разбор CSV совместимости. Запись без первичного документа (ссылки), без семейства продукта или
// платформы, с уровнем не A/C — ошибка: связь без основания в выдачу не идёт.
func ReadCompatDocs(path string) ([]domain.CompatDoc, error) {
	rows, err := readCSVRows(path, []string{"id", "product_family", "platform_family", "level", "url", "doc_kind"})
	if err != nil {
		return nil, err
	}
	var out []domain.CompatDoc
	seen := map[string]bool{}
	for _, r := range rows {
		d := domain.CompatDoc{ID: r.get("id"), ProductVendor: r.get("product_vendor"),
			ProductFamilies: splitPipe(r.get("product_family")), Product: r.get("product"),
			ProductVersion: r.get("product_version"), PlatformVendor: r.get("platform_vendor"),
			PlatformFamilies: splitPipe(r.get("platform_family")), Platform: r.get("platform"),
			PlatformVersion: r.get("platform_version"), Level: strings.ToUpper(r.get("level")), DocKind: r.get("doc_kind"),
			DocTitle: r.get("doc_title"), DocNumber: r.get("doc_number"), DocDate: r.get("doc_date"),
			Publisher: r.get("publisher"), URL: r.get("url"), Note: r.get("note"),
			ProductLabel: r.get("product_label"), PlatformLabel: r.get("platform_label"), DocLabel: r.get("doc_label")}
		switch {
		case d.ID == "":
			return nil, fmt.Errorf("%s:%d: пустой id", path, r.line)
		case seen[d.ID]:
			return nil, fmt.Errorf("%s:%d: повтор id %q", path, r.line, d.ID)
		case len(d.ProductFamilies) == 0 || len(d.PlatformFamilies) == 0:
			return nil, fmt.Errorf("%s:%d: у записи %q нет семейства продукта или платформы", path, r.line, d.ID)
		case !compatLevels[d.Level]:
			return nil, fmt.Errorf("%s:%d: уровень %q (A | C)", path, r.line, d.Level)
		case !strings.HasPrefix(d.URL, "https://") && !strings.HasPrefix(d.URL, "http://"):
			return nil, fmt.Errorf("%s:%d: у записи %q нет ссылки на документ", path, r.line, d.ID)
		case !compatDocKinds[d.DocKind]:
			return nil, fmt.Errorf("%s:%d: вид документа %q (certificate | protocol | documentation | instruction | catalog)",
				path, r.line, d.DocKind)
		case d.DocDate != "" && !isISODate(d.DocDate):
			return nil, fmt.Errorf("%s:%d: дата документа %q (YYYY-MM-DD)", path, r.line, d.DocDate)
		}
		seen[d.ID] = true
		out = append(out, d)
	}
	return out, nil
}

// ReadPromos — разбор CSV акций. Без срока окончания, ссылки или текста — ошибка: акцию без подтверждённого срока
// не показываем.
func ReadPromos(path string) ([]domain.Promo, error) {
	rows, err := readCSVRows(path, []string{"id", "families", "text", "valid_to", "url"})
	if err != nil {
		return nil, err
	}
	var out []domain.Promo
	seen := map[string]bool{}
	for _, r := range rows {
		p := domain.Promo{ID: r.get("id"), Vendor: r.get("vendor"), Families: splitPipe(r.get("families")),
			OfferMatch: splitPipe(r.get("offer_match")), Title: r.get("title"), Text: r.get("text"), Kind: r.get("kind"),
			URL: r.get("url"), CheckedAt: r.get("checked_at"), Note: r.get("note")}
		var err error
		if p.ValidTo, err = parseISODate(r.get("valid_to")); err != nil || p.ValidTo.IsZero() {
			return nil, fmt.Errorf("%s:%d: у акции %q нет срока окончания (valid_to YYYY-MM-DD)", path, r.line, p.ID)
		}
		if p.ValidFrom, err = parseISODate(r.get("valid_from")); err != nil {
			return nil, fmt.Errorf("%s:%d: valid_from: %w", path, r.line, err)
		}
		if v := r.get("discount_pct"); v != "" {
			n, err := strconv.Atoi(v)
			if err != nil || n <= 0 || n >= 100 {
				return nil, fmt.Errorf("%s:%d: discount_pct %q (1–99)", path, r.line, v)
			}
			p.DiscountPct = &n
		}
		switch {
		case p.ID == "":
			return nil, fmt.Errorf("%s:%d: пустой id", path, r.line)
		case seen[p.ID]:
			return nil, fmt.Errorf("%s:%d: повтор id %q", path, r.line, p.ID)
		case len(p.Families) == 0 || p.Text == "":
			return nil, fmt.Errorf("%s:%d: у акции %q нет семейств или текста", path, r.line, p.ID)
		case !strings.HasPrefix(p.URL, "https://"):
			return nil, fmt.Errorf("%s:%d: у акции %q нет ссылки", path, r.line, p.ID)
		}
		seen[p.ID] = true
		out = append(out, p)
	}
	return out, nil
}

type csvRow struct {
	line int
	rec  []string
	col  map[string]int
}

func (r csvRow) get(name string) string {
	if i, ok := r.col[name]; ok && i < len(r.rec) {
		return strings.TrimSpace(r.rec[i])
	}
	return ""
}

// readCSVRows — строки CSV с заголовком; комментарии «#» до заголовка и между строками пропускаются.
func readCSVRows(path string, required []string) ([]csvRow, error) {
	f, err := os.Open(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, ErrNoFile
		}
		return nil, err
	}
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	r.Comment = '#'
	head, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("%s: заголовок: %w", path, err)
	}
	col := map[string]int{}
	for i, h := range head {
		col[strings.TrimSpace(strings.TrimPrefix(h, "\ufeff"))] = i
	}
	for _, need := range required {
		if _, ok := col[need]; !ok {
			return nil, fmt.Errorf("%s: нет колонки %q", path, need)
		}
	}
	var rows []csvRow
	for {
		rec, err := r.Read()
		if errors.Is(err, io.EOF) {
			break
		}
		line, _ := r.FieldPos(0)
		if err != nil {
			return nil, fmt.Errorf("%s:%d: %w", path, line, err)
		}
		if len(rec) == 0 || len(rec) == 1 && strings.TrimSpace(rec[0]) == "" {
			continue
		}
		rows = append(rows, csvRow{line: line, rec: rec, col: col})
	}
	return rows, nil
}

func splitPipe(s string) []string {
	var out []string
	for _, x := range strings.Split(s, "|") {
		if x = strings.TrimSpace(x); x != "" {
			out = append(out, x)
		}
	}
	return out
}

func isISODate(s string) bool {
	_, err := time.Parse("2006-01-02", s)
	return err == nil
}

func parseISODate(s string) (time.Time, error) {
	if s == "" {
		return time.Time{}, nil
	}
	return time.Parse("2006-01-02", s)
}
