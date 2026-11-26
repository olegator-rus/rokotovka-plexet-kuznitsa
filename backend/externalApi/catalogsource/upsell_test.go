package catalogsource

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const compatHead = "id,product_vendor,product_family,product,product_version,platform_vendor,platform_family,platform," +
	"platform_version,level,doc_kind,doc_title,doc_number,doc_date,publisher,url,note\n"

func writeTmp(t *testing.T, body string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "f.csv")
	if err := os.WriteFile(p, []byte(body), 0o600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestReadCompatDocs(t *testing.T) {
	p := writeTmp(t, "# комментарий, с запятой\n"+compatHead+
		`cd-001,kripto-pro,Программа КриптоПро CSP 5.0,КриптоПро CSP 5.0 R2,5.0.12001,astralinux,Astra Linux Special Edition,`+
		`Astra Linux SE,1.7,A,certificate,Сертификат Ready for Astra,29073/2025,2025-01-23,ООО «РусБИТех-Астра»,`+
		`https://astra.ru/upload/parser_jira/certs/cert_se17_rdy-29073.pdf,"КриптоПро CSP 5.0 R2 × Astra SE 1.7.6"`+"\n"+
		`cd-002,kaspersky,A|B,KES для Linux,12.0,redsoft,Операционная система РЕД ОС,РЕД ОС,7.3,c,documentation,Поддерживаемые ОС,,,,https://support.kaspersky.com/x,`+"\n")
	docs, err := ReadCompatDocs(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(docs) != 2 || docs[0].DocNumber != "29073/2025" || docs[0].DocDate != "2025-01-23" || docs[0].Level != "A" {
		t.Fatalf("записи: %+v", docs)
	}
	if len(docs[1].ProductFamilies) != 2 || docs[1].Level != "C" {
		t.Fatalf("несколько семейств через «|», уровень без учёта регистра: %+v", docs[1])
	}
}

func TestReadCompatDocsRejectsUnfounded(t *testing.T) {
	bad := map[string]string{
		"нет ссылки":    "cd-1,v,F,P,1,v,G,Q,1,A,certificate,T,1,,,,\n",
		"уровень B":     "cd-1,v,F,P,1,v,G,Q,1,B,certificate,T,1,,,https://x/,\n",
		"нет семейства": "cd-1,v,,P,1,v,G,Q,1,A,certificate,T,1,,,https://x/,\n",
		"вид документа": "cd-1,v,F,P,1,v,G,Q,1,A,blog,T,1,,,https://x/,\n",
		"дата":          "cd-1,v,F,P,1,v,G,Q,1,A,certificate,T,1,23.01.2025,,https://x/,\n",
		"повтор id":     "cd-1,v,F,P,1,v,G,Q,1,A,certificate,T,1,,,https://x/,\ncd-1,v,F,P,1,v,G,Q,1,A,certificate,T,1,,,https://x/,\n",
	}
	for name, row := range bad {
		if _, err := ReadCompatDocs(writeTmp(t, compatHead+row)); err == nil {
			t.Errorf("%s: ждали ошибку", name)
		}
	}
	if _, err := ReadCompatDocs(filepath.Join(t.TempDir(), "none.csv")); !errors.Is(err, ErrNoFile) {
		t.Fatalf("нет файла — ErrNoFile: %v", err)
	}
}

func TestReadPromos(t *testing.T) {
	head := "id,vendor,families,offer_match,title,text,discount_pct,kind,valid_from,valid_to,url,checked_at,note\n"
	p := writeTmp(t, head+
		`promo-1,pro32,A|B,,T,"Акция: −40% при переходе с антивируса другого производителя, условия по ссылке",40,migration_discount,2026-02-10,2026-06-28,https://store.softline.ru/specials/detail/x/,2026-05-28,`+"\n")
	ps, err := ReadPromos(p)
	if err != nil {
		t.Fatal(err)
	}
	if len(ps) != 1 || *ps[0].DiscountPct != 40 || len(ps[0].Families) != 2 || !strings.HasPrefix(ps[0].Text, "Акция:") {
		t.Fatalf("акция: %+v", ps)
	}
	if !ps[0].ActiveOn(time.Date(2026, 6, 28, 23, 0, 0, 0, time.UTC)) || ps[0].ActiveOn(time.Date(2026, 6, 29, 0, 0, 0, 0, time.UTC)) {
		t.Fatal("срок включительно по valid_to")
	}
	if _, err := ReadPromos(writeTmp(t, head+"promo-2,v,A,,T,Текст,,sale,,,https://store.softline.ru/x/,,\n")); err == nil {
		t.Fatal("акция без срока — ошибка")
	}
}

// Файлы данных заказчика в репозитории читаются без ошибок.
func TestUpsellDataFilesParse(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "data", "softline")
	if _, err := os.Stat(dir); err != nil {
		t.Skip("нет data/softline")
	}
	docs, err := ReadCompatDocs(filepath.Join(dir, "compat_docs.csv"))
	if err != nil && !errors.Is(err, ErrNoFile) {
		t.Fatalf("compat_docs.csv: %v", err)
	}
	for _, d := range docs {
		if d.Level == "A" && d.DocTitle == "" {
			t.Errorf("%s: у записи A нет названия документа", d.ID)
		}
	}
	ps, err := ReadPromos(filepath.Join(dir, "promos.csv"))
	if err != nil && !errors.Is(err, ErrNoFile) {
		t.Fatalf("promos.csv: %v", err)
	}
	for _, p := range ps {
		if p.ValidTo.IsZero() {
			t.Errorf("%s: акция без срока", p.ID)
		}
	}
}

// Таблица классов допродажи в репозитории читается и ссылается только на известные классы.
func TestUpsellClassFilesParse(t *testing.T) {
	dir := filepath.Join("..", "..", "..", "data", "softline")
	cls, err := ReadUpsellClasses(filepath.Join(dir, "upsell_classes.csv"))
	if errors.Is(err, ErrNoFile) {
		t.Skip("нет таблицы классов")
	}
	if err != nil {
		t.Fatalf("upsell_classes.csv: %v", err)
	}
	links, err := ReadUpsellLinks(filepath.Join(dir, "upsell_links.csv"), cls)
	if err != nil {
		t.Fatalf("upsell_links.csv: %v", err)
	}
	from := map[string]bool{}
	for _, l := range links {
		from[l.From] = true
	}
	for _, need := range []string{"os", "office", "antivirus", "backup", "crypto"} {
		if !from[need] {
			t.Errorf("у класса %s нет строк таблицы", need)
		}
	}
}

func TestReadUpsellLinksRejectsUnknown(t *testing.T) {
	cls := writeTmp(t, "id,title,categories\nos,ОС,1\n")
	cs, err := ReadUpsellClasses(cls)
	if err != nil {
		t.Fatal(err)
	}
	for name, row := range map[string]string{
		"неизвестный класс": "os,nope,,,,Причина,кикофф,\n",
		"нет причины":       "os,self,support,,,,кикофф,\n",
		"max":               "os,self,support,9,,Причина,кикофф,\n",
	} {
		if _, err := ReadUpsellLinks(writeTmp(t, "from,to,kinds,max,require_doc,reason,source,added_at\n"+row), cs); err == nil {
			t.Errorf("%s: ждали ошибку", name)
		}
	}
	if _, err := ReadUpsellClasses(writeTmp(t, "id,title,categories,include\nx,X,1,(\n")); err == nil {
		t.Error("неверное выражение — ошибка")
	}
}
