#!/usr/bin/env python3
"""Подсказки при вводе: префикс → первое семейство или вендор (контур эталонов, гейт).

Для десяти префиксов девяти вендоров из эталонов заказчика проверяет, что первая карточка товарной секции панели
(`products[0]`) — целевое семейство эталона или семейство того же вендора, и что вендор цели есть в секции вендоров.
Цели — из data/softline/eval/customer-queries.jsonl (group_id; вендор — первый сегмент target_url).
Отдельно — раскладка и опечатка: ввод в английской раскладке и с опечаткой в законченном слове ведут туда же,
в ответе есть `corrected`.

Запуск из корня репозитория (API по умолчанию http://127.0.0.1:8088):
  python3 scripts/eval/suggest_prefix_eval.py
  API=http://127.0.0.1:8093 python3 scripts/eval/suggest_prefix_eval.py
Код выхода 1 — хотя бы один префикс не ведёт к цели.
"""

import json
import os
import sys
import urllib.parse
import urllib.request

API = os.environ.get("API", "http://127.0.0.1:8088").rstrip("/")
ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
QUERIES = os.path.join(ROOT, "data/softline/eval/customer-queries.jsonl")
VENDORS = os.path.join(ROOT, "data/softline/feed/vendors.jsonl")

# Префикс → эталон заказчика (запрос из customer-queries.jsonl). Сами префиксы — из эталонов заказчика.
PREFIXES = {
    "аст": "astra linux",
    "касп": "kaspersky small office security",
    "крип": "крипто про csp 5.0",
    "кибер": "кибер бэкап",
    "мтс": "мтс",
    "макс": "max patrol",
    "эни": "anydesk",
    "скан": "сканер-вс",
    "офис": "Microsoft Office для дома и бизнеса",
    "365": "365",
}

# Раскладка и опечатка: ввод → эталон. Опечатка исправляется в законченном слове (после пробела).
CORRECTIONS = {
    "rfcg": "kaspersky small office security",  # «касп» в английской раскладке
    "fcnhf": "astra linux",  # «астра» в английской раскладке
    "касперкий ": "kaspersky small office security",  # опечатка в законченном слове
}


def suggest(q):
    url = API + "/api/suggest?" + urllib.parse.urlencode({"q": q})
    req = urllib.request.Request(url, headers={"Origin": "http://localhost:5177"})
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)


def load_targets():
    vendor_names = {}
    with open(VENDORS, encoding="utf-8") as f:
        for line in f:
            v = json.loads(line)
            vendor_names[v["id"]] = v["name"]
    targets = {}
    with open(QUERIES, encoding="utf-8") as f:
        for line in f:
            e = json.loads(line)
            slug = e.get("vendor_slug") or e["target_url"].strip("/").split("/")[0]
            targets[e["query"]] = {"family": e.get("group_id", ""), "vendor": slug,
                                   "vendor_name": vendor_names.get(slug, slug)}
    return targets


def check(q, target):
    d = suggest(q)
    prods = d.get("products") or []
    vends = [v["id"] for v in d.get("vendors") or []]
    first = prods[0] if prods else None
    by_family = bool(first) and target["family"] != "" and first["family_id"] == target["family"]
    by_vendor = bool(first) and first["vendor"] == target["vendor_name"]
    return {"q": q, "ok": by_family or by_vendor, "how": "семейство" if by_family else ("вендор" if by_vendor else "—"),
            "first": first["family_name"] if first else None, "vendors": vends,
            "vendor_listed": target["vendor"] in vends, "corrected": d.get("corrected"),
            "together": [t["family_name"] for t in d.get("together") or []]}


def main():
    targets = load_targets()
    fails = 0
    print(f"{'ввод':<14} {'итог':<5} {'по':<9} {'первое семейство':<46} вендоры / «Вместе с этим»")
    for group in (PREFIXES, CORRECTIONS):
        for q, etalon in group.items():
            r = check(q, targets[etalon])
            fails += 0 if r["ok"] else 1
            extra = ""
            if group is CORRECTIONS:
                extra = f" corrected={r['corrected']}"
                if not r["corrected"]:
                    fails += 1
            print(f"{q!r:<14} {'да' if r['ok'] else 'НЕТ':<5} {r['how']:<9} {str(r['first'])[:46]:<46} "
                  f"{r['vendors']} / {r['together']}{extra}")
        print()
    print(f"не ведут к цели: {fails}")
    return 1 if fails else 0


if __name__ == "__main__":
    sys.exit(main())
