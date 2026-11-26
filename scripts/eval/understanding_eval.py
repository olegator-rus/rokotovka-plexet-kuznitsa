#!/usr/bin/env python3
"""Контрольный набор понимания запросов на фиде (контур фида): жаргон, написание, транслит, раскладка, опечатки,
коды и числа в названиях, преемники, намерение по категории и проверочные пары «сервер».

Набор — data/softline/eval/understanding-set.jsonl: query, type, expect, top.
  expect.family   — одно из семейств (group_id) среди первых top карточек;
  expect.vendor   — у одной из первых top карточек вендор из списка (слаг vendorUrl);
  expect.category — все первые top карточки (или все, если их меньше) из этих категорий второго уровня;
  expect.exclude  — этих семейств нет среди первых 20 карточек;
  expect.normalized — нормализованный запрос ровно такой; expect.route_not — маршрут не этот (D — без модели).
Вендор и категория карточки берутся из фида (data/softline/feed/products.jsonl), а не из ответа.

  API=http://127.0.0.1:8092 python3 scripts/eval/understanding_eval.py --out data/softline/eval/results/understanding-after
  python3 scripts/eval/understanding_eval.py --before before.json --out after   # печатает «было / стало»

Запросы идут с source=compare (в аналитику покупателя не входят), по одному, на 429 — пауза и повтор.
"""
import argparse
import json
import os
import sys
import time
import urllib.error
import urllib.request

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
API = os.environ.get("API", "http://127.0.0.1:8088").rstrip("/")
ORIGIN = os.environ.get("ORIGIN", "http://localhost:5177")
SET = os.path.join(ROOT, "data", "softline", "eval", "understanding-set.jsonl")
FEED = os.path.join(ROOT, "data", "softline", "feed", "products.jsonl")


def load_feed():
    fams = {}
    with open(FEED, encoding="utf-8") as f:
        for line in f:
            p = json.loads(line)
            fams.setdefault(p["family_id"], (p["vendor"], p["category"], p["family_name"]))
    return fams


def search(query):
    body = json.dumps({"query": query, "source": "compare"}).encode()
    for attempt in range(6):
        req = urllib.request.Request(API + "/api/search", data=body,
                                     headers={"Origin": ORIGIN, "Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            if e.code == 429 and attempt < 5:
                time.sleep(2 * (attempt + 1))
                continue
            raise


def judge(row, items, fams, d=None):
    exp, top = row["expect"], row.get("top", 1)
    fids = [it["family_id"] for it in items]
    head = fids[:top]
    meta = [fams.get(f, ("", "", "")) for f in head]
    ok = True
    if "family" in exp:
        ok = ok and any(f in exp["family"] for f in head)
    if "vendor" in exp:
        ok = ok and any(m[0] in exp["vendor"] for m in meta)
    if "category" in exp:
        ok = ok and bool(head) and all(m[1] in exp["category"] for m in meta)
    if "exclude" in exp:
        ok = ok and not any(f in exp["exclude"] for f in fids[:20])
    if d is not None and "normalized" in exp:
        ok = ok and d.get("normalized") == exp["normalized"]
    if d is not None and "route_not" in exp:
        ok = ok and (d.get("trace") or {}).get("route") != exp["route_not"]
    return ok


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", help="префикс файлов отчёта (.json)")
    ap.add_argument("--before", help="отчёт прошлого прогона (.json) — печатать «было / стало»")
    args = ap.parse_args()
    fams = load_feed()
    rows = [json.loads(l) for l in open(SET, encoding="utf-8") if l.strip()]
    before = {}
    if args.before:
        before = {r["query"]: r for r in json.load(open(args.before, encoding="utf-8"))["rows"]}
    res = []
    for row in rows:
        d = search(row["query"])
        items = d.get("items") or []
        ok = judge(row, items, fams, d)
        first = items[0] if items else {}
        route = (d.get("trace") or {}).get("route", "")
        res.append({"query": row["query"], "type": row["type"], "ok": ok, "route": route,
                    "normalized": d.get("normalized", ""),
                    "first": (first.get("title") or "")[:70], "first_family": first.get("family_id", ""),
                    "cards": len(items)})
        time.sleep(0.05)
    fixed = broken = 0
    for r in res:
        b = before.get(r["query"])
        was = "—" if b is None else ("да" if b["ok"] else "нет")
        now = "да" if r["ok"] else "нет"
        if b is not None and not b["ok"] and r["ok"]:
            fixed += 1
        if b is not None and b["ok"] and not r["ok"]:
            broken += 1
        print(f"{r['query'][:34]:36} {r['type']:10} было {was:3} стало {now:3} {r['route']:2} {r['first'][:60]}")
    passed = sum(r["ok"] for r in res)
    print(f"\n  прошло: {passed} из {len(res)}")
    if before:
        print(f"  было: {sum(1 for r in res if before.get(r['query'], {}).get('ok'))} из {len(res)}; "
              f"исправлено {fixed}, сломано {broken}")
    if args.out:
        with open(args.out + ".json", "w", encoding="utf-8") as f:
            json.dump({"api": API, "rows": res, "passed": passed, "total": len(res)}, f, ensure_ascii=False, indent=1)
        print(f"отчёт: {args.out}.json")
    return 0


if __name__ == "__main__":
    sys.exit(main())
