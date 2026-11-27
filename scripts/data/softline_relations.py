#!/usr/bin/env python3
"""Связи «С этим покупают» (relations, type=complement) на семействах фида Softline.

Источник пар — таблица классов допродажи data/softline/upsell_classes.csv: по ней определяется, к какому классу
относится семейство (категория второго уровня + include/exclude по свёрнутому названию, первый подходящий класс
выигрывает — как в backend/services/upsell_classes.go). Связи строятся только ОТ семейств вне классов таблицы
(оборудование: ноутбуки, ПК, моноблоки, мини-ПК, серверы; токены электронной подписи). От семейств классов таблицы
(ОС, офис, антивирус, резервное копирование, СКЗИ…) связей нет: для них блок «Дополните решение» собирает таблица
допродажи, а связь из справочника заменила бы его в выдаче (upsell_feed.go: «блок собран по связям справочника»).

Цель каждой связи — конкретная позиция (to_product): семейство класса таблицы с ценой и в наличии, базовая лицензия
для организации, самая дешёвая из подходящих. Нет такой позиции — скрипт падает: связь на товар без цены не нужна.

Только стандартная библиотека. Запуск из корня репозитория:

  python3 -I scripts/data/softline_relations.py            # пишет data/softline/relations.csv
  python3 -I scripts/data/softline_relations.py --check    # только проверка, без записи

После правки — `cd backend && DATA_SOURCE=generic DATA_MAPPING=../data/softline/mapping.yaml go run . import-refs`.
"""

from __future__ import annotations

import argparse
import csv
import io
import json
import re
import sys
from collections import defaultdict
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
DATA = ROOT / "data" / "softline"
FEED = DATA / "feed"
OUT = DATA / "relations.csv"

# Классы якорей (вне таблицы допродажи): категории фида второго уровня и, где категория шире, — регулярное выражение
# по свёрнутому названию семейства.
ANCHORS = {
    "workstation": {"title": "Рабочее место: ноутбук, ПК, моноблок, мини-ПК", "categories": {"70824", "70187", "73180", "73062"},
                    "exclude_vendors": {"apple"}},  # macOS: ни Windows, ни Astra, ни KES для Windows сюда не предлагаются
    "server": {"title": "Сервер", "categories": {"86563", "87268"}, "include": r"сервер|server"},  # без «рабочих станций»
    "token": {"title": "Токен электронной подписи", "categories": {"687"}, "include": r"рутокен|rutoken"},
}

# Связи на ОС — только к компьютеру без предустановленной ОС (параметр фида «Операционная система»).
NO_OS = {"", "без ос", "dos", "free dos", "freedos"}

# Что предлагать к якорю: класс таблицы допродажи, точное название семейства фида, причина на карточке, стратегия
# количества. Порядок — порядок карточек.
LINKS = {
    "workstation": [
        ("os", "Microsoft Windows 11 Pro (Профессиональная)", "Операционная система, если компьютер без ОС", "per_device"),
        ("os", "Astra Linux Special Edition", "Российская ОС, если компьютер без ОС", "per_device"),
        ("office", "Microsoft Office 2024 для дома и бизнеса", "Офисный пакет для рабочего места", "per_device"),
        ("antivirus", "Kaspersky Endpoint Security для бизнеса Стандартный", "Защита рабочего места", "per_device"),
    ],
    "server": [
        ("os", "Альт Сервер 11", "Серверная операционная система", "one"),
        ("backup", "Кибер Бэкап Стандартная редакция", "Резервное копирование сервера", "one"),
        ("antivirus", "Dr.Web Server Security Suite", "Антивирус для сервера", "one"),
    ],
    "token": [
        ("crypto", "Программа КриптоПро CSP 5.0", "СКЗИ для работы с электронной подписью на токене", "per_device"),
    ],
}


def fold(s: str) -> str:
    s = (s or "").lower().replace("ё", "е")
    for a, b in (("«", '"'), ("»", '"'), ("“", '"'), ("”", '"')):
        s = s.replace(a, b)
    return " ".join(s.split())


def read_csv_commented(path: Path) -> list[dict]:
    lines = [ln for ln in path.read_text(encoding="utf-8").splitlines() if not ln.startswith("#")]
    return list(csv.DictReader(io.StringIO("\n".join(lines))))


def load_classes() -> list[dict]:
    res = []
    for r in read_csv_commented(DATA / "upsell_classes.csv"):
        res.append({
            "id": r["id"],
            "cats": set(filter(None, r["categories"].split("|"))),
            "inc": re.compile(r["include"]) if r["include"] else None,
            "exc": re.compile(r["exclude"]) if r["exclude"] else None,
        })
    return res


def class_of(classes: list[dict], name: str, cat: str) -> str:
    n = fold(name)
    for c in classes:
        if cat not in c["cats"] or (c["inc"] and not c["inc"].search(n)) or (c["exc"] and c["exc"].search(n)):
            continue
        return c["id"]
    return ""


def priced(p: dict) -> bool:
    return bool(p.get("available")) and (p.get("price") or 0) > 0


# Не лицензия: носитель, дистрибутив, документация, сертификат, техподдержка.
NOT_LICENSE = re.compile(r"media ?pack|медиапак|дистрибутив|носител|документац|сертификат|техподдержк|поддержк")
# Уточнение позиции внутри семейства (в названии семейства «КриптоПро CSP 5.0» есть и позиции версии 4.0).
TITLE_MUST = {"Программа КриптоПро CSP 5.0": r"\b5\.0\b",
              # у сервера — лицензия на сервер, а не на рабочую станцию
              "Кибер Бэкап Стандартная редакция": r"сервер"}


def pick_offer(offers: list[dict], must: str = "") -> dict | None:
    """Позиция-цель: с ценой и в наличии, базовая лицензия для организации (не продление, не поддержка, не носитель),
    меньшая фасовка, срок 1 год или бессрочно, затем дешевле."""
    def ok(p: dict) -> bool:
        t = fold(p["title"])
        return (priced(p) and p.get("license_kind") in ("base", None, "") and not NOT_LICENSE.search(t)
                and (not must or re.search(must, t))
                and p.get("license_org_type") in ("commercial", None, ""))
    cands = [p for p in offers if ok(p)]
    # Меньшая фасовка (у лицензий на узлы — нижняя ступень), затем срок 1 год или бессрочно, затем цена.
    return min(cands, key=lambda p: (p.get("license_quantity") or 1, p.get("license_duration_months") not in (None, 12),
                                     p["price"], p["id"])) if cands else None


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__.split("\n")[0])
    ap.add_argument("--check", action="store_true", help="только проверка, без записи")
    args = ap.parse_args()

    classes = load_classes()
    fam_offers: dict[str, list[dict]] = defaultdict(list)
    fam_name: dict[str, str] = {}
    fam_cat: dict[str, str] = {}
    with (FEED / "products.jsonl").open(encoding="utf-8") as f:
        for line in f:
            p = json.loads(line)
            fid = p["family_id"]
            fam_offers[fid].append(p)
            fam_name.setdefault(fid, p.get("family_name") or p["title"])
            fam_cat.setdefault(fid, p["category"])
    by_name: dict[str, list[str]] = defaultdict(list)
    for fid, n in fam_name.items():
        by_name[fold(n)].append(fid)

    # Цели: семейство по точному названию, класс таблицы допродажи, позиция с ценой.
    targets: dict[tuple[str, str], tuple[str, dict]] = {}
    problems = []
    for anchor, links in LINKS.items():
        for cls, name, _, _ in links:
            fids = [f for f in by_name.get(fold(name), []) if any(priced(p) for p in fam_offers[f])]
            if len(fids) != 1:
                problems.append(f"{anchor}: семейство «{name}» — найдено с ценой: {len(fids)}")
                continue
            fid = fids[0]
            got = class_of(classes, fam_name[fid], fam_cat[fid])
            if got != cls:
                problems.append(f"{anchor}: «{name}» в таблице допродажи — класс «{got}», ожидался «{cls}»")
                continue
            offer = pick_offer(fam_offers[fid], TITLE_MUST.get(name, ""))
            if offer is None:
                problems.append(f"{anchor}: у «{name}» нет базовой позиции с ценой")
                continue
            targets[(anchor, name)] = (fid, offer)

    # Якоря: семейства категорий якоря вне классов таблицы, хотя бы одна позиция с ценой.
    anchors: dict[str, list[str]] = defaultdict(list)
    for fid, offers in fam_offers.items():
        if not any(priced(p) for p in offers):
            continue
        for a, spec in ANCHORS.items():
            if fam_cat[fid] not in spec["categories"]:
                continue
            if spec.get("include") and not re.search(spec["include"], fold(fam_name[fid])):
                continue
            if class_of(classes, fam_name[fid], fam_cat[fid]):
                continue  # семейство класса таблицы: блок собирает допродажа
            if offers[0].get("vendor") in spec.get("exclude_vendors", ()):
                continue
            anchors[a].append(fid)

    for a in ANCHORS:
        if not anchors[a]:
            problems.append(f"якорей класса {a} нет")
    if problems:
        print("ошибки:\n  " + "\n  ".join(problems), file=sys.stderr)
        return 1

    rows = []
    for a in ANCHORS:
        for fid in sorted(anchors[a], key=lambda x: (fam_name[x], x)):
            os_attr = fold(next((p.get("attrs", {}).get("Операционная система", "") for p in fam_offers[fid] if priced(p)), ""))
            for cls, name, note, strategy in LINKS[a]:
                if a == "workstation" and cls == "os" and os_attr not in NO_OS:
                    continue  # ОС уже установлена
                tfid, offer = targets[(a, name)]
                rows.append({
                    "id": f"rel-sl-{len(rows) + 1:04d}", "from": fid, "to": tfid, "to_product": offer["id"],
                    "type": "complement", "note": note, "requirement": "recommended", "compatibility": "unknown",
                    "quantity_strategy": strategy, "source": "catalog_rule", "evidence_level": "B",
                    "review_status": "approved", "anchor_class": a, "target_class": cls,
                })

    print(f"якорей: " + ", ".join(f"{a} {len(anchors[a])}" for a in ANCHORS) + f"; связей: {len(rows)}")
    for (a, name), (tfid, offer) in targets.items():
        print(f"  {a} → {name} ({tfid}): {offer['title'][:80]} — {offer['price']:g} ₽")
    if args.check:
        return 0
    with OUT.open("w", encoding="utf-8", newline="") as f:
        w = csv.DictWriter(f, fieldnames=list(rows[0].keys()), lineterminator="\n")
        w.writeheader()
        w.writerows(rows)
    print(f"записано: {OUT.relative_to(ROOT)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
