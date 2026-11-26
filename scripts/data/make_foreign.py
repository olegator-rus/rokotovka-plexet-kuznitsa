#!/usr/bin/env python3
"""«Чужой» набор данных из data/mock — для сквозной проверки DATA_SOURCE=generic.

    python3.13 scripts/data/make_foreign.py                 # пишет data/foreign/ и data/foreign/mapping.yaml
    python3.13 scripts/data/make_foreign.py --out <папка>    # в другой каталог

Тот же магазин, но в виде, в каком данные обычно приходят: прайс в XLSX с шапкой над таблицей, справочники и
журналы в CSV в cp1251 с «;», журнал действий в TSV с BOM и московским временем, другие имена колонок,
вендор и категория — названиями (категория — путём «Родитель > Категория»), цены «12 990,00 ₽» и
«по запросу», «да/нет». Каталог плоский: нет семейств, условий лицензии, популярности и происхождения
позиции; из описаний убрана фраза «Лицензия: …» — всё это импорт должен восстановить сам.
Коды позиций совпадают с id mock, поэтому разметка data/mock/queries.json годится для оценки.

Часть контура коммерции: в прайсе — тип товара, парт-номер, модель, «совместим с», маржа в процентах и
две характеристики; в журнале — номер заказа, контрагент и количество (номера и контрагента нет у заказов
контура поиска — их импорт восстанавливает по клиенту и окну); правила совместимости — JSON контракта; разметка
пар блока — CSV с классами по-русски. Правил лицензий, атрибутов, шаблонов решений и полей связей контура коммерции
нет — аудит должен показать их отсутствие и правила среза.

Только стандартная библиотека; XLSX собирается вручную (zip + XML, строки inline).
"""

import argparse
import csv
import io
import json
import re
import zipfile
from datetime import datetime, timedelta, timezone
from pathlib import Path
from xml.sax.saxutils import escape

ROOT = Path(__file__).resolve().parents[2]
MOCK = ROOT / "data" / "mock"
MSK = timezone(timedelta(hours=3))

ORIGIN = {"ru": "Россия", "foreign": "США"}
SEGMENT = {"smb": "Малый бизнес", "enterprise": "Крупный бизнес", "gov": "Госсектор", "individual": "Физлицо"}
RELATION = {"substitute": "Аналог", "complement": "Сопутствующий", "renewal": "Продление", "compatibility": "Совместимость",
            "upgrade": "Старшая редакция"}
KIND = {"software": "ПО", "hardware": "Оборудование"}
PAIR_CLASS = {"required": "обязательный", "compatible": "совместимый", "conditional": "условно совместимый",
              "incompatible": "несовместимый", "unknown": "неизвестно", "complement": "сопутствующий",
              "upgrade": "старшая редакция", "irrelevant": "нерелевантно"}
ACTION = {"search": "Поиск", "click": "Переход в карточку", "add_to_cart": "Добавление в корзину", "order": "Заказ оформлен",
          "price_request": "Запрос цены", "manager_request": "Запрос менеджеру"}


def load(name):
    with open(MOCK / name, encoding="utf-8") as f:
        return json.load(f)


def price_text(p):
    if p["price_on_request"] or p["price_rub"] is None:
        return "по запросу"
    whole, frac = divmod(round(p["price_rub"] * 100), 100)
    return f"{whole:,}".replace(",", " ") + f",{frac:02d} ₽"


def num_text(v):
    """Число по-русски: «7,1», целое — без дробной части; нет — пусто."""
    if v is None:
        return ""
    v = round(float(v), 2)
    return str(int(v)) if v == int(v) else str(v).replace(".", ",")


def strip_license(desc):
    return re.sub(r"\s*Лицензия:[^.]*\.?", "", desc or "").strip()


def col_name(i):
    s = ""
    i += 1
    while i:
        i, r = divmod(i - 1, 26)
        s = chr(65 + r) + s
    return s


def write_xlsx(path, sheet, rows):
    """Минимальная книга: один лист, все ячейки — inline-строки."""
    out = io.StringIO()
    out.write('<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n'
              '<worksheet xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main"><sheetData>')
    for r, row in enumerate(rows, 1):
        out.write(f'<row r="{r}">')
        for c, v in enumerate(row):
            if v is None or v == "":
                continue
            out.write(f'<c r="{col_name(c)}{r}" t="inlineStr"><is><t xml:space="preserve">{escape(str(v))}</t></is></c>')
        out.write("</row>")
    out.write("</sheetData></worksheet>")
    files = {
        "[Content_Types].xml": '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n'
        '<Types xmlns="http://schemas.openxmlformats.org/package/2006/content-types">'
        '<Default Extension="rels" ContentType="application/vnd.openxmlformats-package.relationships+xml"/>'
        '<Default Extension="xml" ContentType="application/xml"/>'
        '<Override PartName="/xl/workbook.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.sheet.main+xml"/>'
        '<Override PartName="/xl/worksheets/sheet1.xml" ContentType="application/vnd.openxmlformats-officedocument.spreadsheetml.worksheet+xml"/>'
        "</Types>",
        "_rels/.rels": '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n'
        '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/officeDocument" Target="xl/workbook.xml"/>'
        "</Relationships>",
        "xl/workbook.xml": '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n'
        '<workbook xmlns="http://schemas.openxmlformats.org/spreadsheetml/2006/main" '
        'xmlns:r="http://schemas.openxmlformats.org/officeDocument/2006/relationships">'
        f'<sheets><sheet name="{escape(sheet)}" sheetId="1" r:id="rId1"/></sheets></workbook>',
        "xl/_rels/workbook.xml.rels": '<?xml version="1.0" encoding="UTF-8" standalone="yes"?>\n'
        '<Relationships xmlns="http://schemas.openxmlformats.org/package/2006/relationships">'
        '<Relationship Id="rId1" Type="http://schemas.openxmlformats.org/officeDocument/2006/relationships/worksheet" Target="worksheets/sheet1.xml"/>'
        "</Relationships>",
        "xl/worksheets/sheet1.xml": out.getvalue(),
    }
    with zipfile.ZipFile(path, "w", zipfile.ZIP_DEFLATED) as z:
        for name in sorted(files):
            info = zipfile.ZipInfo(name, date_time=(2026, 5, 27, 0, 0, 0))  # детерминированный архив
            info.compress_type = zipfile.ZIP_DEFLATED
            z.writestr(info, files[name])


def write_csv(path, header, rows, encoding="cp1251", delimiter=";", bom=False):
    buf = io.StringIO()
    w = csv.writer(buf, delimiter=delimiter, lineterminator="\r\n")
    w.writerow(header)
    w.writerows(rows)
    data = buf.getvalue().encode(encoding, errors="replace")
    if bom:
        data = b"\xef\xbb\xbf" + data
    path.write_bytes(data)


MAPPING = """\
# Сопоставление для «чужого» набора (scripts/data/make_foreign.py): прайс XLSX + CSV cp1251 + TSV.
# Пояснения к каждому ключу — data/mapping.example.yaml.
version: foreign-1
category_separator: " > "
timezone: Europe/Moscow

catalog:
  file: Номенклатура.xlsx
  sheet: Прайс
  header_row: 3                 # над таблицей — шапка прайса
  columns:
    id: Код товара
    sku: Артикул
    title: Наименование
    description: Описание
    vendor: Производитель       # название, не id: вендор находится по справочнику
    category: Раздел каталога   # путь «Родитель > Категория»
    price_rub: Цена, руб.
    in_reestr: Реестр Минцифры
    kind: Тип товара
    mpn: Парт-номер
    model_id: Модель
    compatible_with: Совместим с
    margin: Маржа, %
  attributes: ["Платформа: platform", "Язык: language", "Поставка: delivery"]
  specs: ["Память, ГБ: ram_gb", "Мощность, Вт: power_w"]
  values:
    kind: {ПО: software, Оборудование: hardware}

synthetic: [margin, orders]     # маржа и заказы в mock придуманы
compatibility_rules: compatibility_rules.json

vendors:
  file: vendors.csv
  columns: {name: Наименование, origin: Страна}
  values:
    origin: {Россия: ru, США: foreign}

relations:
  file: links.csv
  columns: {from: Код товара, to: Связанный товар, type: Вид связи, note: Комментарий}

clients:
  file: clients.csv
  columns: {id: Код клиента, name: Контрагент, segment: Сегмент}

purchases:
  file: purchases.csv
  columns: {client_id: Код клиента, product: Код товара, date: Дата}

query_log:
  file: queries_freq.csv
  columns: {text: Поисковая фраза, freq: Частота в месяц}

events:
  file: actions.tsv
  columns:
    ts: Дата и время
    session_id: Сессия
    client_id: Клиент
    type: Действие
    query: Поисковая фраза
    results_count: Найдено
    product: Артикул            # события ссылаются на артикул, а не на код
    position: Позиция
    order_id: Номер заказа      # нет — заказ восстанавливается по клиенту и окну 30 мин
    account_id: Контрагент      # нет — берётся клиент
    quantity: Кол-во

companion_pairs:
  file: pairs.csv
  columns: {anchor_product: Товар, candidate_product: Предлагаемый товар, class: Оценка, query: Запрос}

eval_queries: ../mock/queries.json
"""


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--out", default=str(ROOT / "data" / "foreign"))
    args = ap.parse_args()
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)

    catalog = load("catalog.json")["products"]
    vendors = {v["id"]: v for v in load("vendors.json")}
    cats = {c["id"]: c for c in load("categories.json")}
    by_id = {p["id"]: p for p in catalog}

    def cat_path(cid):
        parts = []
        while cid:
            parts.append(cats[cid]["name"])
            cid = cats[cid]["parent_id"]
        return " > ".join(reversed(parts))

    head = ["Код товара", "Артикул", "Наименование", "Производитель", "Раздел каталога", "Описание", "Цена, руб.",
            "Реестр Минцифры", "Платформа", "Язык", "Поставка", "Тип товара", "Парт-номер", "Модель", "Совместим с", "Маржа, %",
            "Память, ГБ", "Мощность, Вт"]
    rows = [["Прайс-лист интернет-магазина ПО на.2026"], [], head]
    for p in catalog:
        a = p.get("attributes") or {}
        rows.append([p["id"], p["sku"], p["title"], vendors[p["vendor_id"]]["name"], cat_path(p["category_id"]),
                     strip_license(p["description"]), price_text(p), "да" if p["in_reestr"] else "нет",
                     a.get("platform", ""), a.get("language", ""), a.get("delivery", ""), KIND[p.get("kind") or "software"],
                     p.get("mpn") or "", p.get("model_id") or "", ", ".join(p.get("compatible_with") or []),
                     num_text(p["margin"] * 100) if p.get("margin") else "",
                     num_text((p.get("specs") or {}).get("ram_gb")), num_text((p.get("specs") or {}).get("power_w"))])
    write_xlsx(out / "Номенклатура.xlsx", "Прайс", rows)

    write_csv(out / "vendors.csv", ["Наименование", "Страна"],
              [[v["name"], ORIGIN[v["origin"]]] for v in vendors.values()])

    # Связи — по коду первой позиции семейства.
    first = {}
    for p in catalog:
        first.setdefault(p["family_id"], p["id"])
    write_csv(out / "links.csv", ["Код товара", "Связанный товар", "Вид связи", "Комментарий"],
              [[first[r["from_family"]], first[r["to_family"]], RELATION[r["type"]], r.get("note", "")]
               for r in load("relations.json") if r["from_family"] in first and r["to_family"] in first])

    clients = load("clients.json")
    write_csv(out / "clients.csv", ["Код клиента", "Контрагент", "Сегмент"],
              [[c["id"], c["name"], SEGMENT[c["segment"]]] for c in clients])
    write_csv(out / "purchases.csv", ["Код клиента", "Код товара", "Дата"],
              [[c["id"], pu["product_id"], datetime.strptime(pu["date"], "%Y-%m-%d").strftime("%d.%m.%Y")]
               for c in clients for pu in c["purchases"]])

    write_csv(out / "queries_freq.csv", ["Поисковая фраза", "Частота в месяц"],
              [[q["text"], q["freq"]] for q in load("queries.json")])

    ev_rows = []
    with open(MOCK / "events.jsonl", encoding="utf-8") as f:
        for line in f:
            if not line.strip():
                continue
            e = json.loads(line)
            if e["type"] not in ACTION:
                continue  # события витрины контуров коммерции–3 (подсказки, блок допродажи) — не журнал магазина
            ts = datetime.fromisoformat(e["ts"].replace("Z", "+00:00")).astimezone(MSK)
            pid = e.get("product_id") or ""
            ev_rows.append([ts.strftime("%d.%m.%Y %H:%M:%S"), e["session_id"], e.get("client_id") or "", ACTION[e["type"]],
                            e.get("query") or "", "" if e.get("results_count") is None else e["results_count"],
                            (by_id[pid]["sku"] or pid) if pid in by_id else pid,
                            "" if e.get("position") is None else e["position"], e.get("order_id") or "",
                            e.get("account_id") or "", e.get("quantity") or ""])
    write_csv(out / "actions.tsv", ["Дата и время", "Сессия", "Клиент", "Действие", "Поисковая фраза", "Найдено", "Артикул",
                                    "Позиция", "Номер заказа", "Контрагент", "Кол-во"], ev_rows, encoding="utf-8", delimiter="\t",
              bom=True)

    (out / "compatibility_rules.json").write_text(json.dumps(load("compatibility_rules.json"), ensure_ascii=False, indent=1),
                                                  encoding="utf-8")
    pairs = load("companion_pairs.json")
    write_csv(out / "pairs.csv", ["Товар", "Предлагаемый товар", "Оценка", "Запрос"],
              [[by_id[x["anchor_product"]]["sku"] or x["anchor_product"], x["candidate_product"], PAIR_CLASS[x["class"]],
                x.get("query") or ""] for x in pairs], encoding="utf-8", delimiter=",")

    mapping = MAPPING
    if out.resolve() != (ROOT / "data" / "foreign").resolve():
        mapping = mapping.replace("../mock/queries.json", str(MOCK / "queries.json"))
    (out / "mapping.yaml").write_text(mapping, encoding="utf-8")
    print(f"{out}: позиций {len(catalog)}, событий {len(ev_rows)}, запросов {len(load('queries.json'))}")


if __name__ == "__main__":
    main()
