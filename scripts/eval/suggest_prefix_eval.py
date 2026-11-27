#!/usr/bin/env python3
"""Подсказки при вводе: префикс → первое семейство или вендор (контур эталонов, гейт).

Для десяти префиксов девяти вендоров из эталонов заказчика проверяет, что первая карточка товарной секции панели
(`products[0]`) — целевое семейство эталона или семейство того же вендора, и что вендор цели есть в секции вендоров.
Цели — из data/softline/eval/customer-queries.jsonl (group_id; вендор — первый сегмент target_url).
Отдельно — раскладка и опечатка: ввод в английской раскладке и с опечаткой в законченном слове ведут туда же,
в ответе есть `corrected`.

Задержка панели: каждый ввод запрашивается --repeats раз (по умолчанию 5); p50 и p95 — по времени ответа у клиента
(urllib, от отправки до разбора JSON) и по meta.ms.total сервера. Доля непустого «Вместе с этим» (поле together) —
по всем вводам.

Запуск из корня репозитория (API по умолчанию http://127.0.0.1:8088):
  python3 scripts/eval/suggest_prefix_eval.py
  python3 scripts/eval/suggest_prefix_eval.py --out data/softline/eval/results/suggest-report
  API=http://127.0.0.1:8093 python3 scripts/eval/suggest_prefix_eval.py
--out <путь без расширения> пишет <путь>.json и <путь>.md.
Код выхода 1 — хотя бы один префикс не ведёт к цели.
"""

import argparse
import json
import math
import os
import sys
import time
import urllib.error
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


LATENCY = []  # (ввод, мс у клиента, мс сервера)


def suggest(q, retries=6):
    url = API + "/api/suggest?" + urllib.parse.urlencode({"q": q})
    for attempt in range(retries):
        req = urllib.request.Request(url, headers={"Origin": "http://localhost:5177"})
        t = time.perf_counter()
        try:
            with urllib.request.urlopen(req, timeout=30) as r:
                d = json.load(r)
        except urllib.error.HTTPError as e:
            if e.code == 429 and attempt < retries - 1:
                time.sleep(1.0 * (attempt + 1))
                continue
            raise
        ms = (time.perf_counter() - t) * 1000
        LATENCY.append((q, ms, ((d.get("meta") or {}).get("ms") or {}).get("total")))
        time.sleep(0.06)  # не больше ~15 запросов в секунду: лимит панели — 20 в секунду с адреса
        return d


def pct(xs, p):
    xs = sorted(x for x in xs if x is not None)
    if not xs:
        return None
    k = max(0, math.ceil(p / 100 * len(xs)) - 1)
    return round(xs[k], 1)


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


def check(q, target, repeats=1):
    d = suggest(q)
    for _ in range(repeats - 1):
        suggest(q)
    prods = d.get("products") or []
    vends = [v["id"] for v in d.get("vendors") or []]
    first = prods[0] if prods else None
    by_family = bool(first) and target["family"] != "" and first["family_id"] == target["family"]
    by_vendor = bool(first) and first["vendor"] == target["vendor_name"]
    return {"q": q, "ok": by_family or by_vendor, "how": "семейство" if by_family else ("вендор" if by_vendor else "—"),
            "first": first["family_name"] if first else None, "vendors": vends,
            "vendor_listed": target["vendor"] in vends, "corrected": d.get("corrected"),
            "together": [t["family_name"] for t in d.get("together") or []]}


def render_md(rows, s):
    yn = lambda b: "да" if b else "НЕТ"
    out = ["# Подсказки при вводе: префиксы эталонов, раскладка и опечатка", "",
           "Требования 8 и К3. Скрипт `scripts/eval/suggest_prefix_eval.py`, метод `GET /api/suggest`. Цель префикса — "
           "первая карточка товарной секции панели: целевое семейство эталона или семейство того же вендора. Раскладка и "
           "опечатка — то же, плюс в ответе есть `corrected`.", "",
           "## Итог", "",
           "| Показатель | Значение |", "| --- | --- |",
           f"| Префиксы ведут к цели | {s['prefix_ok']}/{s['prefix_total']} (по семейству — {s['prefix_by_family']}, "
           f"по вендору — {s['prefix_by_vendor']}) |",
           f"| Вендор цели в секции вендоров (префиксы) | {s['prefix_vendor_listed']}/{s['prefix_total']} |",
           f"| Раскладка и опечатка ведут к цели и исправлены | {s['corr_ok']}/{s['corr_total']} |",
           f"| Непустое «Вместе с этим» | {s['together_nonempty']}/{s['inputs']} |",
           f"| Задержка панели у клиента, p50 / p95, мс | {s['client_p50']} / {s['client_p95']} "
           f"({s['samples']} запросов, по {s['repeats']} на ввод) |",
           f"| Задержка панели на сервере (meta.ms.total), p50 / p95, мс | {s['server_p50']} / {s['server_p95']} |",
           f"| Первый запрос каждого ввода у клиента, p50 / p95, мс | {s['first_p50']} / {s['first_p95']} |",
           "", "## По вводам", "",
           "| Ввод | Эталон | К цели | По | Первое семейство | Вендор в секции | corrected | «Вместе с этим» |",
           "| --- | --- | --- | --- | --- | --- | --- | --- |"]
    for r in rows:
        out.append(f"| `{r['q']}` | {r['etalon']} | {yn(r['ok'])} | {r['how']} | {r['first'] or '—'} | "
                   f"{yn(r['vendor_listed'])} | {r['corrected'] or '—'} | {'; '.join(r['together']) or 'пусто'} |")
    out += ["", "Задержка у клиента — локальный стенд, один поток; в неё входит сеть до 127.0.0.1. Префиксы — из плана "
            "контура эталонов; набор открытый, команда его видела.", ""]
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", help="путь без расширения: пишутся .json и .md")
    ap.add_argument("--repeats", type=int, default=5, help="запросов на каждый ввод для замера задержки")
    args = ap.parse_args()
    targets = load_targets()
    fails = 0
    rows = []
    print(f"{'ввод':<14} {'итог':<5} {'по':<9} {'первое семейство':<46} вендоры / «Вместе с этим»")
    for group in (PREFIXES, CORRECTIONS):
        for q, etalon in group.items():
            r = check(q, targets[etalon], args.repeats)
            r["etalon"], r["kind"] = etalon, "prefix" if group is PREFIXES else "correction"
            fails += 0 if r["ok"] else 1
            extra = ""
            if group is CORRECTIONS:
                extra = f" corrected={r['corrected']}"
                if not r["corrected"]:
                    fails += 1
            rows.append(r)
            print(f"{q!r:<14} {'да' if r['ok'] else 'НЕТ':<5} {r['how']:<9} {str(r['first'])[:46]:<46} "
                  f"{r['vendors']} / {r['together']}{extra}")
        print()
    print(f"не ведут к цели: {fails}")
    pre = [r for r in rows if r["kind"] == "prefix"]
    cor = [r for r in rows if r["kind"] == "correction"]
    firsts, seen = [], set()
    for q, ms, _ in LATENCY:
        if q not in seen:
            seen.add(q)
            firsts.append(ms)
    summary = {
        "prefix_total": len(pre), "prefix_ok": sum(r["ok"] for r in pre),
        "prefix_by_family": sum(r["how"] == "семейство" for r in pre),
        "prefix_by_vendor": sum(r["how"] == "вендор" for r in pre),
        "prefix_vendor_listed": sum(r["vendor_listed"] for r in pre),
        "corr_total": len(cor), "corr_ok": sum(bool(r["ok"] and r["corrected"]) for r in cor),
        "inputs": len(rows), "together_nonempty": sum(bool(r["together"]) for r in rows),
        "repeats": args.repeats, "samples": len(LATENCY),
        "client_p50": pct([x[1] for x in LATENCY], 50), "client_p95": pct([x[1] for x in LATENCY], 95),
        "server_p50": pct([x[2] for x in LATENCY], 50), "server_p95": pct([x[2] for x in LATENCY], 95),
        "first_p50": pct(firsts, 50), "first_p95": pct(firsts, 95), "failures": fails,
    }
    print(json.dumps(summary, ensure_ascii=False))
    if args.out:
        with open(args.out + ".json", "w", encoding="utf-8") as f:
            json.dump({"api": API, "summary": summary, "rows": rows,
                       "latency_ms": [{"q": q, "client": round(c, 2), "server": sv} for q, c, sv in LATENCY]},
                      f, ensure_ascii=False, indent=1)
        with open(args.out + ".md", "w", encoding="utf-8") as f:
            f.write(render_md(rows, summary))
    return 1 if fails else 0


if __name__ == "__main__":
    sys.exit(main())
