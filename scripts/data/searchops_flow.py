#!/usr/bin/env python3
"""Тестовый поток запросов SearchOps на фиде заказчика.

Логов поиска у заказчика нет. Поток — три слоя с пометкой происхождения:
  customer  — 10 эталонов (data/softline/eval/customer-queries.jsonl) и 7 примеров ТЗ;
  site      — 31 запрос прогона живого поиска сайта;
  synthetic — формулировки по семействам фида: опечатки, транслит, неверная раскладка, жаргон и написания
              (таблица пар research 03), склонения, «по запросу», товары вне каталога, сервисные вопросы.

Частота каждой строки — модельная (сколько сессий с этим запросом прогон трафика проиграет), не наблюдаемая.
У строки — ожидание: семейства фида, которые покупатель счёл бы ответом (по построению: эталон заказчика, правило
по вендору/названию или семейство, из которого сделана формулировка). Пустое ожидание — сервисный вопрос или товара
нет в каталоге: такой покупатель товар не выбирает. Ожидание нужно только генератору трафика (клик, если семейство
в первых карточках); поиск и агент его не видят.

    python3 scripts/data/searchops_flow.py                # → data/softline/searchops_flow.jsonl
    python3 scripts/data/searchops_flow.py --check        # только сводка, без записи

Только стандартная библиотека; детерминирован (--seed).
"""

from __future__ import annotations

import argparse
import json
import random
import re
import sys
from collections import Counter
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
FEED = ROOT / "data/softline/feed/products.jsonl"
FAMILY_URLS = ROOT / "data/softline/family_urls.jsonl"
CUSTOMER = ROOT / "data/softline/eval/customer-queries.jsonl"
OUT = ROOT / "data/softline/searchops_flow.jsonl"

# Модельные частоты (сессий за проход потока) по слоям и видам.
FREQ = {"customer": 8, "tz": 6, "site": 4, "typo": 3, "translit": 2, "layout": 2, "jargon": 3, "declension": 3,
        "on_request": 3, "absent": 2, "service": 3}

# Примеры ТЗ (контур фида, README): ожидание — правило по вендору/названию/категории.
TZ = [
    ("сервер", {"category_name": r"rack-серверы|серверы"}),
    ("антивирь", {"category_name": r"антивирус"}),
    ("офис", {"category_name": r"офисн"}),
    ("винда", {"category_name": r"windows"}),
    ("астра линукс", {"family": ["121231"]}),
    ("мдаемон", {"name": r"mdaemon"}),
    ("Astra Linux", {"family": ["121231"]}),
]

# Прогон живого поиска сайта (31 запрос). Оценка сайта — как в документе прогона; ожидание — что покупатель счёл бы
# ответом в каталоге фида.
SITE = [
    ("др веб", "провал", {"vendor": "drweb"}),
    ("dr.web", "верно", {"vendor": "drweb"}),
    ("ворд", "провал", {"category_name": r"офисн"}),
    ("мойофис", "верно", {"name": r"мойофис"}),
    ("р7 офис", "верно", {"name": r"р7-офис"}),
    ("microsoft office", "верно", {"vendor": "microsoft", "name": r"office"}),
    ("windows server 2025", "провал", {"vendor": "microsoft", "name": r"windows server"}),
    ("1с", "частично", {"name": r"^1с|1с-битрикс|1с:"}),
    ("1c бухгалтерия", "верно", {"name": r"бухгалтери"}),
    ("битрикс24", "верно", {"name": r"битрикс24"}),
    ("астра линукс", "частично", {"family": ["121231"]}),
    ("антивирус для сервера", "частично", {"category_name": r"антивирус"}),
    ("резервное копирование", "верно", {"category_name": r"резервн"}),
    ("видеоконференции", "частично", {"category_name": r"видеоконф|конференц"}),
    ("электронная подпись", "шум", {"name": r"криптопро csp|рутокен эцп|jacarta"}),
    ("криптопро", "верно", {"vendor": "kripto-pro"}),
    ("аналог microsoft office", "провал", {"category_name": r"российское офисное"}),
    ("отечественная субд", "частично", {"name": r"postgres pro|ред база данных|линтер|tantor"}),
    ("сертификат фстэк", "правдоподобно", {}),
    ("для образования", "не проверить", {}),
    ("продление касперский", "провал", {"vendor": "kaspersky", "name": r"endpoint security"}),
    ("касперский на 50 пользователей", "провал", {"vendor": "kaspersky"}),
    ("kaspersky на 3 года", "частично", {"vendor": "kaspersky"}),
    ("сервер 2u", "частично", {"category_name": r"rack-серверы|серверы"}),
    ("ноутбук 16 гб", "провал", {"category_name": r"ноутбук"}),
    ("коммутатор 24 порта", "верно", {"category_name": r"коммутатор"}),
    ("монитор 27", "верно", {"category_name": r"монитор"}),
    ("счёт", "провал", {}),
    ("доставка", "провал", {}),
    ("возврат", "провал", {}),
    ("как продлить лицензию", "провал", {}),
]

# Жаргон и написания из таблицы пар research 03 (строка таблицы — основание): часть есть в словаре поиска, часть нет —
# исход решает настоящий поиск.
JARGON = [
    ("кес", 25, {"vendor": "kaspersky", "name": r"endpoint security"}),
    ("ксос", 26, {"family": ["3761"]}),
    ("ksc", 27, {"vendor": "kaspersky", "name": r"security center|центр управления"}),
    ("доктор веб", 28, {"vendor": "drweb"}),
    ("dr web", 28, {"vendor": "drweb"}),
    ("cryptopro", 29, {"vendor": "kripto-pro"}),
    ("rutoken", 30, {"name": r"рутокен"}),
    ("джакарта", 31, {"name": r"jacarta"}),
    ("кибербекап", 32, {"vendor": "cyberprotect"}),
    ("cyber backup", 32, {"vendor": "cyberprotect"}),
    ("макспатрол", 34, {"name": r"maxpatrol"}),
    ("икс спайдер", 35, {"name": r"xspider"}),
    ("снс", 37, {"name": r"secret net"}),
    ("электронный замок соболь", 38, {"name": r"соболь"}),
    ("я360", 39, {"name": r"яндекс 360"}),
    ("вк воркспейс", 40, {"name": r"vk workspace"}),
    ("о365", 41, {"vendor": "microsoft", "name": r"365"}),
    ("эксель", 42, {"category_name": r"офисн"}),
    ("трафик инспектор", 46, {"name": r"traffic inspector"}),
    ("постгрес", 47, {"name": r"postgres pro"}),
    ("нанокад", 48, {"name": r"nanocad"}),
    ("компас 3д", 49, {"name": r"компас-3d"}),
    ("энидеск", 50, {"family": ["306539"]}),
    ("рубэкап", 19, {"name": r"rubackup"}),
    ("воркспад", 20, {"name": r"workspad"}),
    ("термидеск", 18, {"name": r"termidesk"}),
    ("алд про", 16, {"name": r"ald pro"}),
    ("файнридер", 3, {"name": r"contentreader"}),
    ("нод32", 2, {"vendor": "pro32"}),
]

# Склонения (кикофф: «склонения — первым пунктом»): формы названий из фида в падежах и с глаголами покупки.
DECLENSION = [
    ("лицензия кибер бэкапа", {"vendor": "cyberprotect"}),
    ("продление касперского", {"vendor": "kaspersky"}),
    ("ключи касперского на год", {"vendor": "kaspersky"}),
    ("купить астру линукс", {"family": ["121231"]}),
    ("лицензии астры", {"family": ["121231"]}),
    ("продление доктора веба", {"vendor": "drweb"}),
    ("лицензия компаса 3d", {"name": r"компас-3d"}),
    ("рутокены эцп", {"name": r"рутокен эцп"}),
    ("лицензию на нанокад", {"name": r"nanocad"}),
    ("подписка на яндекс 360", {"name": r"яндекс 360"}),
    ("сертификаты криптопро csp", {"name": r"криптопро csp|cryptopro csp"}),
    ("антивирусом pro32", {"vendor": "pro32"}),
    ("для секрет нета", {"name": r"secret net"}),
    ("сервера ред ос", {"name": r"ред ос"}),
    ("лицензий касперского для бизнеса", {"vendor": "kaspersky"}),
]

# Товары, которых в фиде нет (проверяется по названиям семейств при сборке: нашлось — строка выбрасывается).
ABSENT = [
    ("veeam backup", r"veeam"),
    ("tableau", r"tableau"),
    ("figma", r"figma"),
    ("miro доска", r"\bmiro\b"),
    ("1с зуп", r"зуп|зарплата и управление персоналом"),
    ("1с бухгалтерия 8", r"бухгалтери"),
    ("microsoft word", r"microsoft word|\bword 20"),
    ("zoom конференции", r"zoom (?!for)|zoom workplace|zoom meetings"),
]

# Сервисные вопросы магазина (не о товаре).
SERVICE = [
    "оплата по счёту", "как оплатить картой", "закрывающие документы", "статус заказа",
    "как получить лицензионный ключ", "реквизиты для оплаты", "срок доставки ключа", "возврат денег",
    "договор поставки",
]

# Вендоры, которых называют заказчик (эталоны, ТЗ, кикофф) и таблица пар research 03: популярности в фиде нет, поэтому
# опечатки, транслит и раскладка строятся по их семействам, а не по длинному хвосту каталога.
KNOWN_VENDORS = {"kaspersky", "drweb", "pro32", "astralinux", "cyberprotect", "kripto-pro", "microsoft", "ascon",
                 "nanosoft", "positive-technologies", "kod-bezopasnosti", "basealt", "redsoft", "yandex", "aktiv",
                 "anydesk", "1s-bitriks", "postgrespro", "contentai", "smartsoft", "alt-n-technologies-ltd",
                 "npo-echelon", "webinar", "vkworkspace", "safib", "peredovietechnologii", "entensys", "rosa"}

TRANSLIT = {"а": "a", "б": "b", "в": "v", "г": "g", "д": "d", "е": "e", "ё": "e", "ж": "zh", "з": "z", "и": "i",
            "й": "y", "к": "k", "л": "l", "м": "m", "н": "n", "о": "o", "п": "p", "р": "r", "с": "s", "т": "t",
            "у": "u", "ф": "f", "х": "h", "ц": "ts", "ч": "ch", "ш": "sh", "щ": "sch", "ъ": "", "ы": "y", "ь": "",
            "э": "e", "ю": "yu", "я": "ya"}
EN = "qwertyuiop[]asdfghjkl;'zxcvbnm,."
RU = "йцукенгшщзхъфывапролджэячсмитьбю"
TO_RU = dict(zip(EN, RU))
TO_EN = dict(zip(RU, EN))
CYR = re.compile(r"[а-яё]")


def load_families() -> dict[str, dict]:
    fams: dict[str, dict] = {}
    with FEED.open(encoding="utf-8") as f:
        for line in f:
            p = json.loads(line)
            fam = fams.setdefault(p["family_id"], {"id": p["family_id"], "name": p["family_name"], "vendor": p["vendor"],
                                                   "category": p["category"], "kind": p["kind"], "offers": 0, "avail": 0})
            fam["offers"] += 1
            if p.get("available") and (p.get("price") or 0) > 0:
                fam["avail"] += 1
    return fams


def load_categories() -> dict[str, str]:
    res = {}
    with (ROOT / "data/softline/feed/categories.jsonl").open(encoding="utf-8") as f:
        for line in f:
            c = json.loads(line)
            res[str(c["id"])] = c["name"]
    return res


def resolve(rule: dict, fams: dict[str, dict], cats: dict[str, str]) -> list[str]:
    """Семейства фида по правилу ожидания: family — явно; vendor / name / category_name — пересечение условий."""
    if not rule:
        return []
    if "family" in rule:
        return [f for f in rule["family"] if f in fams]
    name = re.compile(rule["name"], re.I) if "name" in rule else None
    cat = re.compile(rule["category_name"], re.I) if "category_name" in rule else None
    res = []
    for f in fams.values():
        if "vendor" in rule and f["vendor"] != rule["vendor"]:
            continue
        if name and not name.search(f["name"]):
            continue
        if cat and not cat.search(cats.get(f["category"], "")):
            continue
        res.append(f["id"])
    return sorted(res, key=lambda x: (-fams[x]["avail"], -fams[x]["offers"], x))


def short_name(name: str) -> str:
    """Название семейства без версии, кавычек и префикса «Код безопасности:» — как его набирают."""
    n = re.sub(r"^код безопасности:\s*", "", name.strip(), flags=re.I)
    n = re.sub(r"[«»\"“”]", "", n)
    n = re.sub(r"(?<![\w.])(v\d+|\d+\.(\d+|x)(\.\d+)*|20\d\d)(?![\w.])", "", n, flags=re.I)
    return re.sub(r"\s+", " ", n).strip().lower()


def typo(word: str, r: random.Random) -> str:
    i = r.randrange(1, len(word) - 1)
    match r.randrange(3):
        case 0:
            return word[:i] + word[i + 1] + word[i] + word[i + 2:]
        case 1:
            return word[:i] + word[i + 1:]
        case _:
            return word[:i] + word[i] + word[i:]


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n\n")[0])
    ap.add_argument("--seed", type=int, default=20260528)
    ap.add_argument("--out", type=Path, default=OUT)
    ap.add_argument("--check", action="store_true", help="только сводка")
    args = ap.parse_args()
    r = random.Random(args.seed)
    fams, cats = load_families(), load_categories()
    with FAMILY_URLS.open(encoding="utf-8") as f:
        with_url = {json.loads(line)["group_id"] for line in f if line.strip()}
    rows: list[dict] = []
    seen: set[str] = set()

    def add(query: str, layer: str, kind: str, origin: str, expect: list[str], fix: str = "", freq: int | None = None):
        key = layer + "|" + kind + "|" + re.sub(r"\s+", " ", query.lower().replace("ё", "е")).strip()
        if key in seen:
            return
        seen.add(key)
        row = {"id": f"{layer[0]}-{len(rows) + 1:03d}", "query": query, "layer": layer, "kind": kind, "origin": origin,
               "freq": freq if freq is not None else FREQ[kind], "expect_families": expect[:200]}
        if fix:
            row["fix"] = fix
        rows.append(row)

    # 1. Заказчик: эталоны и примеры ТЗ.
    with CUSTOMER.open(encoding="utf-8") as f:
        for i, line in enumerate(f, 1):
            q = json.loads(line)
            if "group_id" in q:
                exp = [q["group_id"]]
            elif q.get("target_type") == "vendor_category":
                exp = resolve({"vendor": q["vendor_slug"]}, fams, cats)
                exp = [x for x in exp if fams[x]["category"] == q["category_id"]] or exp
            else:
                exp = resolve({"vendor": q["vendor_slug"]}, fams, cats)
            add(q["query"], "customer", "customer", f"эталон заказчика №{i}", exp)
    for q, rule in TZ:
        add(q, "customer", "tz", "пример из ТЗ", resolve(rule, fams, cats))

    # 2. Прогон живого сайта.
    for i, (q, verdict, rule) in enumerate(SITE, 1):
        add(q, "site", "site", f"прогон сайта №{i}, у сайта: {verdict}", resolve(rule, fams, cats))

    # 3. Синтетика из каталога. Кандидаты — семейства ПО со страницей на сайте; «можно купить» — для опечаток,
    # транслита и раскладки, «по запросу» — без доступных офферов.
    def typed(f: dict) -> bool:  # так набирают: 1–4 слова
        return 1 <= len(short_name(f["name"]).split()) <= 4

    sold = sorted((f for f in fams.values() if f["id"] in with_url and f["kind"] == "software" and f["avail"] > 0
                   and f["vendor"] in KNOWN_VENDORS and typed(f)), key=lambda f: (-f["avail"], f["id"]))
    onreq = sorted((f for f in fams.values() if f["id"] in with_url and f["kind"] == "software" and f["avail"] == 0
                    and f["offers"] >= 10 and typed(f)), key=lambda f: (-f["offers"], f["id"]))[:60]
    r.shuffle(sold)
    r.shuffle(onreq)
    n = 0
    for f in sold:
        name = short_name(f["name"])
        words = [w for w in re.findall(r"[a-zа-яё]+", name) if len(w) >= 6]
        if not words or n >= 15:
            continue
        w = max(words, key=len)
        q = name.replace(w, typo(w, r), 1)
        if q != name:
            add(q, "synthetic", "typo", f"опечатка в «{f['name']}»", [f["id"]], fix=name)
            n += 1
    n = 0
    for f in sold:
        name = short_name(f["name"])
        if not CYR.search(name) or n >= 10 or len(name) < 5:
            continue
        q = "".join(TRANSLIT.get(ch, ch) for ch in name)
        add(q, "synthetic", "translit", f"латиницей «{f['name']}»", [f["id"]], fix=name)
        n += 1
    n = 0
    for f in sold:
        name = short_name(f["name"])
        if n >= 6 or len(name) < 5 or not re.fullmatch(r"[a-zа-яё ]+", name):
            continue
        table = TO_EN if CYR.search(name) else TO_RU
        q = "".join(table.get(ch, ch) for ch in name)
        add(q, "synthetic", "layout", f"неверная раскладка «{f['name']}»", [f["id"]], fix=name)
        n += 1
    for q, row, rule in JARGON:
        add(q, "synthetic", "jargon", f"жаргон и написания, research 03, строка {row}", resolve(rule, fams, cats))
    for q, rule in DECLENSION:
        add(q, "synthetic", "declension", "склонение названия из фида", resolve(rule, fams, cats))
    n = 0
    for f in onreq:
        name = short_name(f["name"])
        if n >= 12 or len(name) < 4:
            continue
        add(name, "synthetic", "on_request", f"«{f['name']}»: в фиде только «по запросу»", [f["id"]])
        n += 1
    names = [f["name"] for f in fams.values()]
    for q, rx in ABSENT:
        hit = next((nm for nm in names if re.search(rx, nm, re.I)), None)
        if hit:
            print(f"вне каталога: «{q}» выброшен — в фиде есть «{hit}»", file=sys.stderr)
            continue
        add(q, "synthetic", "absent", "товара нет в фиде (проверено по названиям семейств)", [])
    for q in SERVICE:
        add(q, "synthetic", "service", "сервисный вопрос магазина", [])

    empty = [x for x in rows if not x["expect_families"] and x["kind"] not in ("absent", "service", "site")]
    for x in empty:
        print(f"нет ожидаемых семейств: «{x['query']}» ({x['origin']})", file=sys.stderr)
    by_layer = Counter(x["layer"] for x in rows)
    sessions = Counter()
    for x in rows:
        sessions[x["layer"]] += x["freq"]
    summary = {"queries": len(rows), "by_layer": dict(by_layer), "sessions_by_layer": dict(sessions),
               "by_kind": dict(Counter(x["kind"] for x in rows)), "seed": args.seed}
    print(json.dumps(summary, ensure_ascii=False, indent=2))
    if not args.check:
        with args.out.open("w", encoding="utf-8") as f:
            for x in rows:
                f.write(json.dumps(x, ensure_ascii=False) + "\n")
        print(f"записано: {args.out.relative_to(ROOT)}", file=sys.stderr)
    return 0


if __name__ == "__main__":
    sys.exit(main())
