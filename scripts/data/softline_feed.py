#!/usr/bin/env python3
"""Фид заказчика (Yandex YML, выгрузка store.softline.ru для Mindbox) → файлы для DATA_SOURCE=generic.

    python3.13 scripts/data/softline_feed.py                       # data/softline/feed/Mindbox.yml.xml
    python3.13 scripts/data/softline_feed.py --feed путь.xml --out каталог

Пишет в data/softline/feed/ (в git не идёт): products.jsonl, vendors.jsonl, categories.jsonl, aliases.jsonl,
param_values.json (все параметры с частотами). Отчёт data/softline/FEED_STATS.md (идёт в git): сверка с
частоты параметров лицензии, что сделала таблица приведения.
Импорт: DATA_SOURCE=generic DATA_MAPPING=../data/softline/mapping.yaml go run . import

Решения:
- Семейство = категория третьего уровня по group_id оффера, имя семейства — имя этой категории. У электроники
  group_id нет: семейство — сам оффер (family_id «offer-<id>»).
- В справочник категорий идут только уровни 1–2: третий уровень — семейства, а не разделы каталога.
- Вендор = слаг vendorUrl (/kaspersky/ → kaspersky): по нему строятся URL вендора и «вендор + категория».
- Цена отдаётся как есть (price, old_price, available): правило «цена или по запросу» применяет импорт.
- Тип лицензии, тип организации и слова названия приводятся по таблице data/softline/license_map.csv — данные,
  а не код. Срок и количество — в поля license_* контракта.
- Псевдонимы вендора: слаг vendorUrl, если он не совпадает с названием; data/softline/vendor_aliases.csv (ребрендинг
  с основанием); словарь data/mock/aliases.json там, где он не привязан к позициям mock.

Только стандартная библиотека, потоковый iterparse.
"""

import argparse
import csv
import html
import json
import re
import statistics
import time
import xml.etree.ElementTree as ET
from collections import Counter, defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
SOFTLINE = ROOT / "data" / "softline"
FEED = SOFTLINE / "feed" / "Mindbox.yml.xml"
MOCK_ALIASES = ROOT / "data" / "mock" / "aliases.json"

# Значения-заглушки параметров: «нет данных».
PLACEHOLDERS = {"другое или отсутствует", "не определено", "n/a", ""}
# Параметры, которые разбираются в поля лицензии (в атрибуты тоже идут — для текста индекса).
P_TYPE, P_ORG, P_TERM, P_NODES, P_USERS = "Тип лицензии", "Тип организации", "Срок действия", "К-во узлов", "К-во пользователей до"
P_REESTR, P_VERSION, P_PRODUCT = "Номер записи в реестре отечественного ПО", "Версия", "Тип продукта"
LICENSE_PARAMS = [P_TYPE, P_ORG, P_TERM, P_NODES, P_USERS]
# Параметры без смысла для поиска: одно значение у всех или служебные.
SKIP_ATTRS = {"Компоненты пакетов", "Тип поставки", "Способ доставки"}

# Сверка с BASIS.md §2.2 (посчитано исследователем по фиду от.2026).
BASIS = {
    "offers": 136040, "vendors": 1232, "vendors_available": 661, "cat_l1": 54, "cat_l2": 497, "cat_l3": 8760,
    "families": 4413, "families_available": 1809, "software": 127915, "hardware": 8125, "available": 44601,
    "price_zero": 80269, "oldprice_gt_price": 32, "reestr": 1229, "description_median": 91,
}

SLUG_RE = re.compile(r"^https?://[^/]+/([^/?#]+)/?")
TIER_RE = re.compile(r"от\s*(\d[\d\s]*?)\s*(?:до\s*(\d[\d\s]*?))?\s*шт", re.IGNORECASE)
THOUSANDS_RE = re.compile(r"(?<=\d),(?=\d{3}(?!\d))")
MIN_PURCHASE_RE = re.compile(r"\s*•?\s*Минимальная покупка:[^•]*")
TERM_RE = re.compile(r"^(\d+(?:[.,]\d+)?)\s*(мес|дн)", re.IGNORECASE)


def num(text):
    try:
        return float((text or "").strip().replace(",", "."))
    except ValueError:
        return None


def fold(s):
    return " ".join((s or "").lower().replace("ё", "е").split())


def load_license_map(path):
    """Таблица приведения: field → [(value, code, license_type, when)]."""
    table = defaultdict(list)
    with open(path, encoding="utf-8") as f:
        for row in csv.DictReader(f):
            table[row["field"]].append((row["value"], row["code"], row["license_type"], row["when"] or "base"))
    return table


def term_months(raw):
    """Срок действия → (месяцы | None, бессрочная, понятно ли значение)."""
    v = fold(raw)
    if v in PLACEHOLDERS or v == "мес.":
        return None, False, False
    if v.startswith("бессроч"):
        return None, True, True
    m = TERM_RE.match(v)
    if not m:
        return None, False, False
    x = float(m.group(1).replace(",", "."))
    if m.group(2) == "дн":
        return None, False, False  # 5–15 дней: короче месяца, в поле месяцев не кладём
    if x < 1 or x > 120:
        return None, False, False  # 0.0329 мес., 372 мес., 4106 мес. — мусор выгрузки
    return round(x), False, True


def ints(values):
    out = []
    for v in values:
        v = v.strip().replace(" ", "")
        if v.isdigit() and not v.startswith("0"):
            out.append(int(v))
    return out


class Converter:
    def __init__(self, feed, out, license_map):
        self.feed, self.out = feed, out
        self.lmap = license_map
        self.lic_type = {fold(v): (code, lt) for v, code, lt, _ in license_map["Тип лицензии"]}
        self.org_type = {fold(v): code for v, code, _, _ in license_map["Тип организации"]}
        self.name_rules = [(fold(v), code, lt, when) for v, code, lt, when in license_map["name"]]
        self.cats = {}  # id → (name, parent)
        self.params = defaultdict(Counter)
        self.s = Counter()
        self.kind_counter = Counter()
        self.kind_source = Counter()
        self.unmapped_types = Counter()
        self.term_bad = Counter()
        self.desc_len = []
        self.vendor_names = defaultdict(Counter)  # slug → имена
        self.vendor_slugs = defaultdict(set)  # имя → слаги
        self.vendor_available = set()
        self.vendor_reestr = set()
        self.families = set()
        self.families_available = set()
        self.used_cats = set()
        self.yml_date = ""

    def level(self, cid):
        n = 0
        seen = set()
        while cid in self.cats and cid not in seen:
            seen.add(cid)
            n += 1
            cid = self.cats[cid][1]
        return n

    def license_kind(self, ltype, name):
        """Вид лицензии по таблице: тип из параметра, затем правила по словам названия."""
        code, lt, source = "", "", "none"
        if not ltype and "" in self.lic_type:
            # Пустой «Тип лицензии» — базовая (VENDORS.md: пусто → базовая; у МТС Линк тип не заполнен).
            code, lt = self.lic_type[""]
            source = "empty"
        if ltype:
            hit = self.lic_type.get(fold(ltype))
            if hit:
                code, lt = hit
                source = "param"
            else:
                self.unmapped_types[ltype] += 1
                code, source = "other", "unmapped"
        fname = fold(name)
        for word, rcode, rlt, when in self.name_rules:
            if word not in fname:
                continue
            if when == "missing" and ltype:
                continue
            if when == "base" and code not in ("", "base", "other"):
                continue
            code, lt, source = rcode, rlt or lt, "name"
            break
        return code, lt, source

    def offer(self, el):
        s = self.s
        oid = el.get("id")
        gid = el.get("group_id") or ""
        available = el.get("available") == "true"
        # В фиде часть сущностей экранирована дважды («&amp;amp;»): после разбора XML остаётся «&amp;».
        name = html.unescape((el.findtext("name") or "").strip())
        vendor = html.unescape((el.findtext("vendor") or "").strip())
        vurl = (el.findtext("vendorUrl") or "").strip()
        m = SLUG_RE.match(vurl)
        slug = m.group(1).lower() if m else ""
        price = num(el.findtext("price")) or 0.0
        oldprice = num(el.findtext("oldprice")) or 0.0
        cat = (el.findtext("categoryId") or "").strip()
        desc = html.unescape((el.findtext("description") or "").strip())
        self.desc_len.append(len(desc))  # длина — как в фиде, до чистки
        # «Минимальная покупка: от N шт.» — ступень количества (она же в sales_notes и атрибутах). В описании её
        # правила восстановления читали как количество лицензии («от 1 шт.» → на 1 устройство).
        desc = MIN_PURCHASE_RE.sub("", desc).strip(" •")
        sales = (el.findtext("sales_notes") or "").strip()
        params = defaultdict(list)
        for p in el.iter("param"):
            pname, val = p.get("name") or "", (p.text or "").strip()
            self.params[pname][val] += 1
            if fold(val) not in PLACEHOLDERS:
                params[pname].append(val)
        product_type = (params.get(P_PRODUCT) or [""])[0]
        hardware = product_type == "электроника" or not gid

        s["offers"] += 1
        s["available"] += available
        s["price_zero"] += price == 0
        s["price_pos"] += price > 0
        s["available_priced"] += available and price > 0
        s["available_zero"] += available and price == 0
        s["unavailable_priced"] += (not available) and price > 0
        s["unavailable_zero"] += (not available) and price == 0
        s["oldprice_gt_price"] += oldprice > price > 0
        s["oldprice_gt_price_available"] += available and oldprice > price > 0
        s["oldprice_gt_zero_price"] += oldprice > 0 and price == 0
        s["hardware" if hardware else "software"] += 1
        s["no_group"] += not gid
        s["electronics_param"] += product_type == "электроника"
        if slug:
            self.vendor_names[slug][vendor] += 1
        self.vendor_slugs[vendor].add(slug)
        if available:
            self.vendor_available.add(slug)
        reestr = (params.get(P_REESTR) or [""])[0]
        if reestr:
            s["reestr"] += 1
            s["reestr_values"] += len(params[P_REESTR])
            self.vendor_reestr.add(slug)
        if gid:
            self.families.add(gid)
            if available:
                self.families_available.add(gid)
            if self.level(gid) != 3:
                s["group_not_l3"] += 1
        if self.level(cat) != 2:
            s["category_not_l2"] += 1
        self.used_cats.add(cat)

        # Лицензия
        ltype = (params.get(P_TYPE) or [""])[0]
        kind, lic_type, source = ("", "", "none") if hardware else self.license_kind(ltype, name)
        if not hardware:
            self.kind_counter[kind or "(нет)"] += 1
            self.kind_source[source] += 1
        months, perpetual, ok = term_months((params.get(P_TERM) or [""])[0])
        if params.get(P_TERM) and not ok:
            self.term_bad[params[P_TERM][0]] += 1
        if not lic_type and not hardware:
            if months:
                lic_type = "subscription"
            elif perpetual:
                lic_type = "perpetual"
        org = self.org_type.get(fold((params.get(P_ORG) or [""])[0]), "")
        nodes, users = ints(params.get(P_NODES, [])), ints(params.get(P_USERS, []))
        metric, qty, tier_min, tier_max = None, None, None, None
        if nodes or users:
            metric = "device" if nodes else "user"
            vals = nodes or users
            tier_min, tier_max = min(vals), max(vals)
            qty = tier_min if len(vals) > 1 else vals[0]
        t = TIER_RE.search(sales)
        if t:
            lo = int(t.group(1).replace(" ", ""))
            hi = int(t.group(2).replace(" ", "")) if t.group(2) else None
            tier_min, tier_max = lo, hi
            if qty is None or len(nodes or users) <= 1:
                qty = lo
            s["sales_notes_tier"] += 1
        edition = (params.get(P_VERSION) or [""])[0]
        if edition in ("Другое",):
            edition = ""

        attrs = {}
        for pname, vals in params.items():
            if pname in SKIP_ATTRS or pname == P_PRODUCT:
                continue
            uniq = list(dict.fromkeys(vals))
            if pname in (P_NODES, P_USERS) and len(uniq) > 3:
                attrs[pname] = f"{uniq[0]}–{uniq[-1]}"
            else:
                attrs[pname] = ", ".join(uniq)
        if sales:
            attrs["Ступень количества"] = sales
        if hardware:
            family_id, family_name = "offer-" + oid, name
        else:
            family_id, family_name = gid, self.cats.get(gid, ("", ""))[0].strip()
        row = {
            "id": oid, "sku": (el.findtext("vendorCode") or "").strip(), "title": name, "description": desc,
            "vendor": slug or vendor, "category": cat, "family_id": family_id, "family_name": family_name,
            "price": price, "old_price": oldprice, "available": available,
            "offer_url": (el.findtext("url") or "").strip(),
            "kind": "hardware" if hardware else "software",
            "mpn": (el.findtext("vendorCode") or "").strip() if hardware else "",
            "license_type": lic_type, "license_kind": kind, "license_org_type": org,
            "license_metric": metric, "license_quantity": qty, "license_tier_min": tier_min, "license_tier_max": tier_max,
            "license_duration_months": months, "license_duration": "бессрочная" if perpetual else "",
            "license_edition": edition, "in_reestr": bool(reestr), "reestr_number": reestr,
            "attrs": attrs,
        }
        return row

    def run(self):
        started = time.time()
        self.out.mkdir(parents=True, exist_ok=True)
        products = open(self.out / "products.jsonl", "w", encoding="utf-8")
        for _, el in ET.iterparse(self.feed, events=("end",)):
            tag = el.tag
            if tag == "category":
                self.cats[el.get("id")] = (html.unescape(el.text or ""), el.get("parentId"))
                el.clear()
            elif tag == "offer":
                products.write(json.dumps(self.offer(el), ensure_ascii=False) + "\n")
                el.clear()
            elif tag == "yml_catalog":
                self.yml_date = el.get("date") or ""
        products.close()
        # date — атрибут корня: iterparse отдаёт корень последним.
        self.write_refs()
        self.elapsed = time.time() - started

    def write_refs(self):
        # Категории уровней 1–2 (третий — семейства).
        levels = Counter()
        with open(self.out / "categories.jsonl", "w", encoding="utf-8") as f:
            for cid, (name, parent) in sorted(self.cats.items(), key=lambda x: int(x[0])):
                lv = self.level(cid)
                levels[lv] += 1
                if lv <= 2:
                    f.write(json.dumps({"id": cid, "name": name.strip(), "parent_id": parent or ""}, ensure_ascii=False) + "\n")
        self.levels = levels
        # Вендоры: слаг → самое частое имя; происхождение ru — если у вендора есть номер реестра отечественного ПО.
        self.vendor_rows = []
        with open(self.out / "vendors.jsonl", "w", encoding="utf-8") as f:
            for slug, names in sorted(self.vendor_names.items()):
                row = {"id": slug, "name": names.most_common(1)[0][0], "url": f"https://store.softline.ru/{slug}/",
                       "slug": slug, "origin": "ru" if slug in self.vendor_reestr else ""}
                self.vendor_rows.append(row)
                f.write(json.dumps(row, ensure_ascii=False) + "\n")
        self.write_aliases()

    def write_aliases(self):
        rows, seen = [], set()

        def add(alias, canonical, kind, target, target_id, basis):
            key = (fold(alias), target, target_id)
            if not fold(alias) or key in seen:
                return
            seen.add(key)
            rows.append({"alias": fold(alias), "canonical": canonical, "kind": kind, "target": target,
                         "target_id": target_id or "", "basis": basis})

        names = {v["id"]: v["name"] for v in self.vendor_rows}
        slug_aliases = 0
        for slug, name in names.items():
            words = re.sub(r"-\d+$", "", slug).replace("-", " ").strip()
            name_words = " ".join(re.findall(r"[\w]+", fold(name)))
            if len(words) >= 3 and words != name_words and words not in name_words.split():
                add(words, name, "alias", "vendor", slug, "слаг vendorUrl в фиде")
                slug_aliases += 1
        rebrand = 0
        with open(SOFTLINE / "vendor_aliases.csv", encoding="utf-8") as f:
            for r in csv.DictReader(f):
                if r["vendor_slug"] in names:
                    add(r["alias"], names[r["vendor_slug"]], "alias", "vendor", r["vendor_slug"], r["basis"])
                    rebrand += 1
        ported = Counter()
        for a in json.load(open(MOCK_ALIASES, encoding="utf-8")):
            t = a["target"]
            if t == "term":
                add(a["alias"], a["canonical"], a["kind"], "term", "", "словарь продукта (data/mock/aliases.json)")
                ported["term"] += 1
            elif t == "vendor" and a["target_id"] in names:
                add(a["alias"], a["canonical"], a["kind"], "vendor", a["target_id"], "словарь продукта, вендор найден по слагу")
                ported["vendor"] += 1
            elif t == "category":
                # id категорий mock в фиде нет: остаётся подстановка слова («антивирь» → «антивирус»).
                add(a["alias"], a["canonical"], a["kind"], "term", "", "словарь продукта, категория mock → слово")
                ported["category→term"] += 1
            else:
                ported["dropped_" + t] += 1
        with open(self.out / "aliases.jsonl", "w", encoding="utf-8") as f:
            for r in rows:
                f.write(json.dumps(r, ensure_ascii=False) + "\n")
        self.alias_stats = {"slug": slug_aliases, "rebrand": rebrand, **ported, "total": len(rows)}

    # --- отчёт ---

    def report(self, path):
        s = self.s
        got = {
            "offers": s["offers"], "vendors": len(self.vendor_names), "vendors_available": len(self.vendor_available),
            "cat_l1": self.levels[1], "cat_l2": self.levels[2], "cat_l3": self.levels[3],
            "families": len(self.families), "families_available": len(self.families_available),
            "software": s["software"], "hardware": s["hardware"], "available": s["available"],
            "price_zero": s["price_zero"], "oldprice_gt_price": s["oldprice_gt_price"], "reestr": s["reestr"],
            "description_median": int(statistics.median(self.desc_len)) if self.desc_len else 0,
        }
        labels = {
            "offers": "Офферов", "vendors": "Вендоров (по слагу vendorUrl)", "vendors_available": "Вендоров с доступными офферами",
            "cat_l1": "Категорий 1-го уровня", "cat_l2": "Категорий 2-го уровня", "cat_l3": "Категорий 3-го уровня",
            "families": "Семейств с офферами (group_id)", "families_available": "Семейств с доступным оффером",
            "software": "ПО", "hardware": "Электроника", "available": "`available=true`", "price_zero": "Цена 0",
            "oldprice_gt_price": "`oldprice > price > 0`", "reestr": "Номер реестра отечественного ПО",
            "description_median": "Медиана длины описания в фиде, символов",
        }
        lines = [
            "# Фид заказчика: сверка и параметры", "",
            f"Источник: `data/softline/feed/Mindbox.yml.xml` (выгрузка от {self.yml_date}), конвертер "
            f"`scripts/data/softline_feed.py`, разбор {self.elapsed:.0f} с. Версия каталога при импорте — `softline-feed`.", "",
            "## Сверка с", "",
            "| Показатель | BASIS | Посчитано | Расхождение |", "|---|---:|---:|---|",
        ]
        for k, label in labels.items():
            b, g = BASIS[k], got[k]
            diff = "совпадает" if b == g else f"**{g - b:+d}**"
            lines.append(f"| {label} | {b:,} | {g:,} | {diff} |")
        lines += [
            "", "Пояснения к расхождениям (если есть):", "",
        ]
        multi = {n: sl for n, sl in self.vendor_slugs.items() if len(sl) > 1}
        multi_names = {sl: n for sl, n in self.vendor_names.items() if len(n) > 1}
        lines.append(f"- Имя вендора с несколькими слагами: {len(multi)}; слаг с несколькими именами: {len(multi_names)}. "
                     f"Различных имён вендора в фиде: {len(self.vendor_slugs)}.")
        if multi:
            ex = "; ".join(f"{n} → {', '.join(sorted(sl))}" for n, sl in list(multi.items())[:5])
            lines.append(f"  Примеры: {ex}.")
        if multi_names:
            ex = "; ".join(f"{sl} → {', '.join(n.keys())}" for sl, n in list(multi_names.items())[:5])
            lines.append(f"  Примеры: {ex}.")
        lines += [
            f"- Офферов без `group_id`: {s['no_group']:,}; с параметром «Тип продукта: электроника»: {s['electronics_param']:,}.",
            f"- `group_id` не на третьем уровне дерева: {s['group_not_l3']}; `categoryId` не на втором уровне: {s['category_not_l2']}.",
            f"- Категорий второго уровня с офферами: {len(self.used_cats)}.",
            f"- Номер реестра: {s['reestr']:,} офферов, {s['reestr_values']:,} значений — у части офферов два номера. "
            "BASIS, судя по совпадению, считал значения параметра, а не офферы.",
            "", "## Цена и наличие", "",
            "| Состояние | Офферов | Что показываем |", "|---|---:|---|",
            f"| в наличии, цена > 0 | {s['available_priced']:,} | цена |",
            f"| в наличии, цена 0 | {s['available_zero']:,} | по запросу |",
            f"| нет в наличии, цена > 0 | {s['unavailable_priced']:,} | по запросу; цена фида хранится отдельно (`feed_price_rub`) |",
            f"| нет в наличии, цена 0 | {s['unavailable_zero']:,} | по запросу |",
            "",
            f"`oldprice > price > 0`: {s['oldprice_gt_price']}, из них в наличии {s['oldprice_gt_price_available']}. "
            f"`oldprice > 0` при цене 0: {s['oldprice_gt_zero_price']} (скидкой не считается). "
            f"Ступень количества в `sales_notes`: {s['sales_notes_tier']:,} офферов.",
            "", "## Таблица приведения лицензии", "",
            "Таблица — `data/softline/license_map.csv` (данные, не код): значение «Тип лицензии» → вид, тип контракта; "
            "слова названия — при пустом типе (`missing`) или типе base/other (`base`); «Тип организации» → код. "
            "«Миграция» — льготный переход (`migration`, тип контракта `upgrade`), не скидка.", "",
            "| Вид (`license.kind`) | Офферов ПО |", "|---|---:|",
        ]
        for k, v in self.kind_counter.most_common():
            lines.append(f"| {k} | {v:,} |")
        lines += ["", "Откуда вид: " + ", ".join(f"{k} — {v:,}" for k, v in self.kind_source.most_common())
                  + " (param — «Тип лицензии», name — слово названия, none — ни того ни другого).", ""]
        if self.unmapped_types:
            lines.append("Значения «Тип лицензии» без строки в таблице: " + ", ".join(f"«{k}» ({v})" for k, v in self.unmapped_types.most_common()) + ".")
        else:
            lines.append("Все значения «Тип лицензии» есть в таблице.")
        bad = sum(self.term_bad.values())
        lines += ["", f"«Срок действия», не попавший в месяцы: {bad:,} офферов "
                  + "(«Не определено», «мес.», дни и дробные/запредельные значения вроде «0.0329 мес.», «372 мес.», «4106.7762 мес.»). "
                  "Срок от 1 до 120 мес. округляется до целого; «бессрочная» — тип perpetual без срока.", "",
                  "Количество: «К-во узлов» → метрика device, «К-во пользователей до» → user. Несколько значений у оффера или "
                  "`sales_notes` «Кол-во от A до B шт.» — ступень цены за единицу: `license.tier_min/tier_max`, `license.quantity` = A. "
                  "Одно значение без ступени — `quantity` = N («до N»). Значения «000» и с ведущим нулём отброшены.", ""]
        lines += ["## Частоты параметров лицензии", ""]
        for pname in LICENSE_PARAMS:
            c = self.params[pname]
            lines += [f"### {pname} — {sum(c.values()):,} значений, различных {len(c)}", "",
                      "| Значение | Офферов |", "|---|---:|"]
            items = c.most_common()
            limit = 40 if pname in (P_NODES, P_USERS) else len(items)
            for v, k in items[:limit]:
                lines.append(f"| {v.replace('|', '/') or '(пусто)'} | {k:,} |")
            if len(items) > limit:
                rest = sum(k for _, k in items[limit:])
                lines.append(f"| … ещё {len(items) - limit} значений | {rest:,} |")
            lines.append("")
        a = self.alias_stats
        lines += ["## Псевдонимы", "",
                  f"Всего {a['total']}: из слага vendorUrl {a['slug']} (слаг не совпадает с названием вендора), ребрендинг с основанием "
                  f"{a['rebrand']} (`data/softline/vendor_aliases.csv`), словарь продукта: термины {a.get('term', 0)}, вендоры {a.get('vendor', 0)}, "
                  f"категории mock как слова {a.get('category→term', 0)}; отброшено (семейства mock) {a.get('dropped_family', 0)}.", ""]
        # Разряды — пробелом: «136 040». Только запятая между цифрами, текст значений не трогается.
        lines = [THOUSANDS_RE.sub(" ", line) for line in lines]
        path.write_text("\n".join(lines) + "\n", encoding="utf-8")
        with open(self.out / "param_values.json", "w", encoding="utf-8") as f:
            json.dump({k: dict(v.most_common()) for k, v in self.params.items()}, f, ensure_ascii=False, indent=1)
        return got


def main():
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--feed", type=Path, default=FEED)
    ap.add_argument("--out", type=Path, default=SOFTLINE / "feed")
    ap.add_argument("--report", type=Path, default=SOFTLINE / "FEED_STATS.md")
    args = ap.parse_args()
    conv = Converter(args.feed, args.out, load_license_map(SOFTLINE / "license_map.csv"))
    conv.run()
    got = conv.report(args.report)
    diffs = {k: (BASIS[k], v) for k, v in got.items() if BASIS[k] != v}
    print(json.dumps({"seconds": round(conv.elapsed, 1), "offers": got["offers"], "diffs": diffs,
                      "aliases": conv.alias_stats}, ensure_ascii=False))


if __name__ == "__main__":
    main()
