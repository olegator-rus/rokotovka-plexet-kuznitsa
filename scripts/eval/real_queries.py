#!/usr/bin/env python3
"""Прогон реальных и собственных запросов через живой /api/search.

Разметка — data/real-queries/labels.py (want: семейства или "cat:<категория>" в топ-3).
Запросы идут с source=compare: в журнал пишутся, но в аналитику и стоимость покупателя не входят.

  python3 scripts/eval/real_queries.py [--only own|forum|batch2] [--grep подстрока] [--out файл.json]
"""
import argparse
import json
import os
import sys
import time
import urllib.request
from concurrent.futures import ThreadPoolExecutor

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
sys.path.insert(0, os.path.join(ROOT, "data", "real-queries"))
import labels  # noqa: E402

API = os.environ.get("API", "http://127.0.0.1:8088")
ORIGIN = os.environ.get("ORIGIN", "http://localhost:5177")


def catalog_categories():
    with open(os.path.join(ROOT, "data", "mock", "catalog.json"), encoding="utf-8") as f:
        c = json.load(f)
    return {p["family_id"]: p["category_id"] for p in c["products"]}


FAM_CAT = catalog_categories()


def search(text):
    body = json.dumps({"query": text, "source": "compare"}).encode()
    req = urllib.request.Request(API + "/api/search", data=body, method="POST",
                                 headers={"Content-Type": "application/json", "Origin": ORIGIN})
    t = time.time()
    try:
        with urllib.request.urlopen(req, timeout=40) as r:
            d = json.load(r)
    except urllib.error.HTTPError as e:
        return {"http": e.code, "error": e.read().decode()[:200], "ms": round((time.time() - t) * 1000)}
    except Exception as e:  # noqa: BLE001
        return {"http": 0, "error": str(e)[:200], "ms": round((time.time() - t) * 1000)}
    d["ms"] = round((time.time() - t) * 1000)
    return d


def hit(fam, want):
    if not fam:
        return False
    for w in want:
        if w.startswith("cat:"):
            if FAM_CAT.get(fam) == w[4:]:
                return True
        elif fam == w:
            return True
    return False


def run_one(lab):
    d = search(lab["q"])
    items = d.get("items") or []
    fams = [it.get("family_id") or it.get("product_id") for it in items]
    tr = d.get("trace") or {}
    res = {
        **lab,
        "http": d.get("http", 200),
        "error": d.get("error"),
        "ms": d.get("ms"),
        "route": tr.get("route"),
        "parse": tr.get("parse_source"),
        "cost": tr.get("cost_rub"),
        "total": d.get("total"),
        "top": [{"fam": it.get("family_id"), "title": it.get("title", "")[:80], "group": it.get("group")} for it in items[:5]],
        "chips": [c.get("label") for c in d.get("chips") or []],
        "relaxed": [r.get("message") for r in d.get("relaxed") or []],
        "empty": (d.get("empty") or {}).get("message"),
        "normalized": d.get("normalized"),
    }
    if lab["want"]:
        res["hit1"] = hit(fams[0] if fams else None, lab["want"])
        res["hit3"] = any(hit(f, lab["want"]) for f in fams[:3])
    return res


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--only")
    ap.add_argument("--grep")
    ap.add_argument("--out", default=os.path.join(ROOT, "data", "real-queries", "last-run.json"))
    ap.add_argument("--workers", type=int, default=4)
    a = ap.parse_args()
    labs = [x for x in labels.L if (not a.only or x["src"] == a.only) and (not a.grep or a.grep.lower() in x["q"].lower())]
    with ThreadPoolExecutor(a.workers) as ex:
        res = list(ex.map(run_one, labs))
    with open(a.out, "w", encoding="utf-8") as f:
        json.dump(res, f, ensure_ascii=False, indent=1)
    scored = [r for r in res if "hit1" in r]
    by = {}
    for r in scored:
        s = by.setdefault(r["src"], [0, 0, 0])
        s[0] += 1
        s[1] += r["hit1"]
        s[2] += r["hit3"]
    errs = [r for r in res if r["http"] != 200]
    print(f"запросов {len(res)}, размечено {len(scored)}, ошибок HTTP {len(errs)}")
    for src, (n, h1, h3) in sorted(by.items()):
        print(f"  {src:7} n={n:3}  top1 {h1 / n:6.1%}  top3 {h3 / n:6.1%}")
    n = len(scored)
    if n:
        print(f"  {'всего':7} n={n:3}  top1 {sum(r['hit1'] for r in scored) / n:6.1%}  top3 {sum(r['hit3'] for r in scored) / n:6.1%}")
    ms = sorted(r["ms"] for r in res if r.get("ms") is not None)
    if ms:
        print(f"  задержка p50 {ms[len(ms) // 2]} мс, p95 {ms[int(len(ms) * 0.95) - 1]} мс")
    print("\nпромахи топ-3:")
    for r in res:
        if r.get("hit3") is False or r["http"] != 200:
            top = "; ".join(f"{t['fam']}" for t in r["top"][:3]) or (r["empty"] or r.get("error") or "—")
            print(f"  [{r['src']}] {r['q'][:70]!r} route={r['route']} → {top}")


if __name__ == "__main__":
    main()
