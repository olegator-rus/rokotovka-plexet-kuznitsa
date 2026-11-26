#!/usr/bin/env python3
"""Набор по продлениям (): продление Kaspersky на 100 узлов на 2 года, «Windows Server» по запросу →
«Можно купить сейчас», строка сравнения цены за место. Ожидания — из фида (data/softline/feed/products.jsonl).

Набор — data/softline/eval/p63-set.jsonl: id, query, note, expect.
  products_in_top    — эти позиции (id → цена) — головы карточек среди первых top, с этим match_type и ценой;
  all_exact_in_top   — первые N карточек — exact;
  family_in_top      — одно из семейств среди первых top карточек;
  first_family / first_not_family / first_category / first_price_state — первая карточка;
  alternatives       — карточки match_type=alternative: min/max, все из allowed, ни одной из forbidden, все с ценой;
  comparison_contains — подстрока comparison.text;
  no_tier_min        — ни у одной карточки (и варианта) ступень не начинается с этого числа;
  explain_all        — у каждой карточки есть explain, без «аналог» и «нет в каталоге».

  API=http://127.0.0.1:8091 python3 scripts/eval/p63_eval.py --out after
  python3 scripts/eval/p63_eval.py --before before.json --out after   # печатает «было / стало»
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
SET = os.path.join(ROOT, "data", "softline", "eval", "p63-set.jsonl")
FEED = os.path.join(ROOT, "data", "softline", "feed", "products.jsonl")
BANNED = ("аналог", "нет в каталоге")


def load_feed():
    offers = {}
    with open(FEED, encoding="utf-8") as f:
        for line in f:
            p = json.loads(line)
            offers[p["id"]] = (p["category"], p.get("license_tier_min"))
    return offers


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


def judge(exp, d, offers):
    items = d.get("items") or []
    fails = []
    first = items[0] if items else {}
    if "products_in_top" in exp:
        spec = exp["products_in_top"]
        top = items[:spec["top"]]
        heads = {it["product_id"]: it for it in top}
        for pid, price in spec["ids"].items():
            it = heads.get(pid)
            if not it:
                fails.append(f"нет {pid} в первых {spec['top']}")
            elif it.get("match_type") != spec["match_type"] or abs((it.get("price_rub") or 0) - price) > 0.01:
                fails.append(f"{pid}: {it.get('match_type')} {it.get('price_rub')}")
    if "all_exact_in_top" in exp:
        n = exp["all_exact_in_top"]
        if len(items) < n or any(it.get("match_type") != "exact" for it in items[:n]):
            fails.append(f"первые {n} не все exact")
    if "family_in_top" in exp:
        spec = exp["family_in_top"]
        if not any(it["family_id"] in spec["ids"] for it in items[:spec["top"]]):
            fails.append(f"нет {spec['ids']} в первых {spec['top']}")
    if "first_family" in exp and first.get("family_id") not in exp["first_family"]:
        fails.append(f"первая {first.get('family_id')}")
    if "first_not_family" in exp and first.get("family_id") in exp["first_not_family"]:
        fails.append(f"первая {first.get('family_id')} — клиентская лицензия")
    if "first_category" in exp and offers.get(first.get("product_id"), ("",))[0] not in exp["first_category"]:
        fails.append(f"первая из категории {offers.get(first.get('product_id'), ('?',))[0]}")
    if "first_price_state" in exp and first.get("price_state") != exp["first_price_state"]:
        fails.append(f"цена первой {first.get('price_state')}")
    if "alternatives" in exp:
        spec = exp["alternatives"]
        alts = [it for it in items if it.get("match_type") == "alternative"]
        if len(alts) < spec.get("min", 0):
            fails.append(f"альтернатив {len(alts)} < {spec['min']}")
        if "max" in spec and len(alts) > spec["max"]:
            fails.append(f"альтернатив {len(alts)} > {spec['max']}")
        for it in alts:
            if "allowed" in spec and it["family_id"] not in spec["allowed"]:
                fails.append(f"альтернатива {it['family_id']} не из серверных ОС")
            if it["family_id"] in spec.get("forbidden", []):
                fails.append(f"альтернатива {it['family_id']} запрещена")
            if it.get("price_state") != "priced":
                fails.append(f"альтернатива {it['family_id']} без цены")
    if "comparison_contains" in exp:
        text = (d.get("comparison") or {}).get("text", "")
        if exp["comparison_contains"] not in text:
            fails.append(f"сравнение: «{text}»")
    if "no_tier_min" in exp:
        for it in items:
            for x in [it] + (it.get("variants") or []):
                if x.get("matches_query", True) and offers.get(x["product_id"], ("", None))[1] == exp["no_tier_min"]:
                    fails.append(f"ступень {exp['no_tier_min']}: {x['product_id']}")
                    break
    if exp.get("explain_all"):
        for it in items:
            e = it.get("explain") or ""
            if not e or any(b in e.lower() for b in BANNED):
                fails.append(f"explain у {it['product_id']}: «{e}»")
                break
    return fails


def brief(d):
    res = []
    for it in (d.get("items") or [])[:6]:
        p = it.get("price_from")
        res.append(f"{it.get('match_type')}: {it.get('title', '')[:48]} — {('%.2f ₽' % p) if p else 'по запросу'}")
    return res


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", help="путь без расширения: пишет <out>.json")
    ap.add_argument("--before", help="отчёт прошлого прогона (.json) — печатать «было / стало»")
    args = ap.parse_args()
    offers = load_feed()
    before = {}
    if args.before:
        with open(args.before, encoding="utf-8") as f:
            before = {r["id"]: r for r in json.load(f)["rows"]}
    rows = []
    with open(SET, encoding="utf-8") as f:
        cases = [json.loads(l) for l in f if l.strip()]
    for c in cases:
        d = search(c["query"])
        fails = judge(c["expect"], d, offers)
        rows.append({"id": c["id"], "query": c["query"], "ok": not fails, "fails": fails, "cards": brief(d),
                     "comparison": (d.get("comparison") or {}).get("text")})
        was = before.get(c["id"])
        mark = "" if not was else ("было да   " if was["ok"] else "было нет  ")
        print(f"{c['id']:<14} {c['query'][:40]:<42} {mark}стало {'да ' if not fails else 'нет'} {'; '.join(fails)[:120]}")
        time.sleep(0.5)
    ok = sum(r["ok"] for r in rows)
    print(f"\n  прошло: {ok} из {len(rows)}")
    if before:
        print(f"  было: {sum(r['ok'] for r in before.values())} из {len(before)}")
    if args.out:
        with open(args.out + ".json", "w", encoding="utf-8") as f:
            json.dump({"api": API, "rows": rows}, f, ensure_ascii=False, indent=1)
        print(f"отчёт: {args.out}.json")
    return 0 if ok == len(rows) else 1


if __name__ == "__main__":
    sys.exit(main())
