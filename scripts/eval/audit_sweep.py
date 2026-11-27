#!/usr/bin/env python3
"""Широкий прогон качества выдачи поиска по набору запросов (только stdlib).

Набор — JSONL: {id, query, category_of_test, expected};
expected — kind (resolvable/category/absent/info) и условия match.

    python3 scripts/eval/audit_sweep.py \
        --queries data/softline/eval/audit/queries.jsonl \
        --api http://localhost:8088 --out /path/outside/git/audit-run \
        [--feed data/softline/feed/products.jsonl] [--workers 4] [--rate 100] [--compare old/results.jsonl]

Пишет в --out: raw/<id>.json (ответ API как есть), results.jsonl (вердикт по строке),
summary.json и summary.md (сводка по категориям и по видам нарушений). С --compare печатает,
какие строки починились и какие сломались относительно прошлого results.jsonl.
"""
from __future__ import annotations

import argparse
import concurrent.futures as cf
import json
import os
import re
import sys
import threading
import time
import urllib.error
import urllib.request
from collections import Counter, defaultdict

ORIGIN = "http://localhost:5177"
PRODUCT_KINDS = {"resolvable", "category", "absent"}  # info — вопрос не о покупке: только цены и даты
# Даты в служебных текстах ответа (не в названиях товаров): дд.мм, дд.мм.гггг и число с месяцем словом.
DATE_RE = re.compile(
    r"(?<![\d.])(0[1-9]|[12]\d|3[01])\.(0[1-9]|1[0-2])(\.(20)?\d{2})?(?![\d.–-])|\b\d{1,2} (январ|феврал|март|апрел|ма[яй]|июн|июл|август|сентябр|октябр|ноябр|декабр)\w*",
    re.I,
)


def load_feed(path: str) -> dict:
    feed = {}
    if not path or not os.path.exists(path):
        return feed
    with open(path, encoding="utf-8") as f:
        for line in f:
            d = json.loads(line)
            feed[str(d["id"])] = (float(d.get("price") or 0), bool(d.get("available")), d.get("category", ""))
    return feed


_pace_lock = threading.Lock()
_pace_next = [0.0]
PACE_SEC = [0.6]  # интервал между запросами; задаётся --rate (в минуту). API ограничивает /api по адресу.


def _pace() -> None:
    with _pace_lock:
        now = time.time()
        wait = _pace_next[0] - now
        _pace_next[0] = max(now, _pace_next[0]) + PACE_SEC[0]
    if wait > 0:
        time.sleep(wait)


def post(api: str, query: str, retries: int = 8) -> tuple[dict | None, str, float]:
    body = json.dumps({"query": query}).encode()
    last = ""
    for attempt in range(retries):
        _pace()
        req = urllib.request.Request(
            api.rstrip("/") + "/api/search",
            data=body,
            headers={"Content-Type": "application/json", "Origin": ORIGIN},
            method="POST",
        )
        t0 = time.time()
        try:
            with urllib.request.urlopen(req, timeout=90) as r:
                return json.loads(r.read().decode()), "", time.time() - t0
        except urllib.error.HTTPError as e:
            last = f"http {e.code}: {e.read()[:200]!r}"
            if e.code == 429:  # лимит в минуту: ждём окно и повторяем
                time.sleep(30)
                continue
            if e.code < 500:
                return None, last, time.time() - t0
        except Exception as e:  # соединение: API может перезапускаться
            last = f"{type(e).__name__}: {e}"
        time.sleep(min(2 ** attempt, 20))
    return None, last, 0.0


def rx(p: str | None, s: str) -> bool:
    return bool(p) and re.search(p, s or "", re.I) is not None


def item_matches(it: dict, m: dict) -> bool:
    if not m:
        return True
    if "product_id" in m:
        ids = {str(it.get("product_id"))} | {str(v.get("product_id")) for v in it.get("variants") or []}
        return str(m["product_id"]) in ids
    if "family_id" in m and str(it.get("family_id")) == str(m["family_id"]):
        return True
    return (
        rx(m.get("cat"), it.get("category", ""))
        or rx(m.get("vendor"), it.get("vendor", ""))
        or rx(m.get("title"), it.get("title", ""))
    )


def target_matches(t: dict | None, m: dict) -> bool | None:
    if not t or not m or "product_id" in m:
        return None
    title = t.get("title", "")
    return rx(m.get("title"), title) or rx(m.get("vendor"), title) or rx(m.get("cat"), title)


def walk_strings(obj, skip=("title", "family_name", "name", "query", "normalized", "offer_url", "family_url", "vendor_url", "url", "label", "value", "mpn", "license_line")):
    if isinstance(obj, dict):
        for k, v in obj.items():
            if k in skip or k == "trace":
                continue
            yield from walk_strings(v, skip)
    elif isinstance(obj, list):
        for v in obj:
            yield from walk_strings(v, skip)
    elif isinstance(obj, str):
        yield obj


def judge(row: dict, resp: dict | None, err: str, feed: dict) -> dict:
    exp = row.get("expected") or {}
    kind = exp.get("kind", "resolvable")
    m = exp.get("match") or {}
    hard = exp.get("hard") or {}
    v = []  # (severity, code, detail)
    res = {"id": row["id"], "query": row["query"], "category_of_test": row["category_of_test"], "kind": kind}
    if resp is None:
        res.update(violations=[["fail", "http_error", err]], verdict="fail")
        return res
    items = resp.get("items") or []
    real = [it for it in items if it.get("match_type") not in ("fallback", "alternative")]
    fb = [it for it in items if it.get("match_type") == "fallback"]
    target = resp.get("target")
    corr = resp.get("correction")
    res.update(
        target=target,
        n_items=len(items),
        n_real=len(real),
        n_fallback=len(fb),
        by_match_type=(resp.get("counts") or {}).get("by_match_type"),
        correction=corr,
        empty=(resp.get("empty") or {}).get("message"),
        chips=[(c.get("field"), c.get("value")) for c in resp.get("chips") or []],
        top=[
            [it.get("match_type"), it.get("vendor"), (it.get("title") or "")[:90], it.get("category"), it.get("price_rub"), it.get("price_from")]
            for it in items[:6]
        ],
    )
    # 1. Минимум четыре карточки и фолбэк
    if kind in PRODUCT_KINDS:
        if len(items) < 4:
            v.append(["fail", "lt4_cards", f"{len(items)} карточек"])
        if items and not real:
            v.append(["fail", "fallback_only", "выдача — только «Популярное в наличии»"])
        elif fb and kind == "resolvable":
            v.append(["warn", "fallback_added", f"{len(fb)} карточек «Популярное» добавлено"])
    if kind == "service" and (not target or target.get("type") != "service"):
        v.append(["fail", "service_not_detected", f"target={target and target.get('type')}"])
    if kind in ("nonsense", "service"):
        if real:
            v.append(["fail", "nonsense_has_results", f"{len(real)} карточек как результат: {real[0].get('title','')[:60]}"])
        if target and target.get("type") not in ("service",):
            v.append(["fail", "nonsense_has_target", f"{target.get('type')}: {target.get('title')}"])
    # 2. Релевантность первой карточки, топ-4, цели
    if kind in PRODUCT_KINDS and m and real:
        top4 = real[:4]
        ok = [item_matches(it, m) for it in top4]
        res["first_ok"] = ok[0]
        res["top4_ok"] = sum(ok)
        if not ok[0]:
            v.append(["fail", "first_irrelevant", f"{top4[0].get('vendor')} | {top4[0].get('title','')[:70]} | {top4[0].get('category')}"])
        elif sum(ok) < min(len(top4), 3) and "product_id" not in m and "family_id" not in m:
            bad = [it for it, o in zip(top4, ok) if not o][0]
            v.append(["warn", "top4_irrelevant", f"{sum(ok)}/{len(top4)} подходят; лишнее: {bad.get('title','')[:60]} | {bad.get('category')}"])
        tm = target_matches(target, m)
        if tm is False and kind != "absent":
            v.append(["warn", "target_mismatch", f"{target.get('type')}: {target.get('title')}"])
    if kind in ("resolvable", "category") and not items and not resp.get("empty"):
        v.append(["fail", "empty_silent", "ни карточек, ни сообщения"])
    # 3. Жёсткие условия на первых карточках
    head = real[:4]
    if hard and head:
        q = hard.get("quantity")
        chips = {c.get("field"): c.get("value") for c in resp.get("chips") or []}
        if q and "license_quantity" not in chips and "quantity" not in json.dumps(resp.get("purchase") or {}):
            if any(it.get("kind") == "software" for it in head):
                v.append(["warn", "quantity_not_parsed", f"нет чипа количества {q}"])
        if hard.get("license_kind"):
            lk = head[0].get("license_kind") or (head[0].get("attrs") or {}).get("license_kind")
            if lk != hard["license_kind"] and not any((vv.get("license_kind") == hard["license_kind"]) for vv in head[0].get("variants") or []):
                v.append(["fail", "license_kind_dropped", f"ждали {hard['license_kind']}, первая карточка {lk}: {head[0].get('title','')[:60]}"])
        if hard.get("perpetual"):
            a = head[0].get("attrs") or {}
            if a.get("license_type") not in ("perpetual",) and a.get("duration_months"):
                v.append(["fail", "perpetual_dropped", f"первая карточка {a.get('license_type')} {a.get('duration_months')} мес."])
        if hard.get("duration_months"):
            a = head[0].get("attrs") or {}
            if a.get("duration_months") not in (None, hard["duration_months"]):
                v.append(["warn", "duration_dropped", f"ждали {hard['duration_months']} мес., первая {a.get('duration_months')}"])
        if hard.get("org_type"):
            t = head[0].get("title", "")
            if not re.search(r"образоват|академ|учебн|школ|вуз|студен|государств|гос|некоммерч|НКО", t, re.I):
                v.append(["warn", "org_type_not_visible", f"первая: {t[:60]}"])
        pm = hard.get("price_max")
        if pm:
            over = [it for it in head if (it.get("price_from") or it.get("price_rub") or 0) > pm]
            if over:
                v.append(["fail", "price_max_dropped", f"{len(over)}/{len(head)} дороже {pm}: {over[0].get('title','')[:50]} {over[0].get('price_from')}"])
        pmin = hard.get("price_min")
        if pmin:
            under = [it for it in head if 0 < (it.get("price_from") or it.get("price_rub") or 0) < pmin]
            if under:
                v.append(["fail", "price_min_dropped", f"{len(under)}/{len(head)} дешевле {pmin}"])
    # 4. Цены против фида
    if feed:
        def chk(it, where):
            pid = str(it.get("product_id") or "")
            pr = it.get("price_rub")
            if pid in feed and pr:
                fp = feed[pid][0]
                if fp > 0 and abs(pr - fp) > 0.51:
                    ratio = pr / fp
                    v.append(["fail", "price_mismatch", f"{where}: {it.get('title','')[:60]} показано {pr}, в фиде {fp} (×{ratio:.2f})"])
        for it in items:
            chk(it, "карточка")
            for vv in it.get("variants") or []:
                chk(vv, "вариант")
            pf = it.get("price_from")
            ps = [x.get("price_rub") for x in [it] + list(it.get("variants") or []) if x.get("price_rub")]
            if pf and ps and pf > max(ps) * 1.01:
                v.append(["fail", "price_from_above_all", f"{it.get('title','')[:60]} «от {pf}» > всех цен {max(ps)}"])
        comp = resp.get("companions") or {}
        for it in comp.get("items") or []:
            chk(it, "допродажа")
        for it in resp.get("complements") or []:
            chk(it, "с этим покупают")
    # 5. Исправление запроса
    if corr:
        to = corr.get("to", "")
        if exp.get("no_correction"):
            v.append(["fail" if corr.get("applied") else "warn", "unwanted_correction", f"«{corr.get('from')}» → «{to}» applied={corr.get('applied')}"])
        elif exp.get("correction_to") and not rx(exp["correction_to"], to):
            v.append(["fail", "wrong_correction", f"«{corr.get('from')}» → «{to}», ждали ~{exp['correction_to']}"])
        if kind in ("nonsense",):
            v.append(["warn", "nonsense_corrected", f"→ «{to}»"])
    # 6. Ожидаемая страница магазина (внешние наборы): совпала цель, карточка или хотя бы вендор
    url = exp.get("expected_url")
    if url:
        nu = lambda u: (u or "").strip().lower().rstrip("/")
        want = nu(url)
        urls = {nu((target or {}).get("url"))}
        for it in real[:10]:
            urls |= {nu(it.get("offer_url")), nu(it.get("family_url")), nu(it.get("vendor_url"))}
            urls |= {nu(x.get("offer_url")) for x in it.get("variants") or []}
        seg = lambda u: u.split("store.softline.ru/")[-1].split("/")[0]
        if want in urls:
            res["url_hit"] = "exact"
        elif any(u and seg(u) == seg(want) for u in urls):
            res["url_hit"] = "vendor_or_section"
        else:
            res["url_hit"] = "miss"
            v.append(["warn", "expected_url_miss", f"ждали {url}; цель {(target or {}).get('url') or '—'}"])
    # 7. Даты в текстах
    for s in walk_strings({k: resp.get(k) for k in ("empty", "fallback_note", "relaxed", "comparison", "target", "companions", "complements", "items", "clarifications", "upgrade")}):
        if DATE_RE.search(s):
            v.append(["warn", "date_in_text", s[:100]])
            break
    res["violations"] = v
    res["verdict"] = "fail" if any(x[0] == "fail" for x in v) else ("warn" if v else "pass")
    return res


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("--queries", required=True)
    ap.add_argument("--api", default="http://localhost:8088")
    ap.add_argument("--out", required=True)
    ap.add_argument("--feed", default="data/softline/feed/products.jsonl")
    ap.add_argument("--workers", type=int, default=4)
    ap.add_argument("--compare", default="")
    ap.add_argument("--rate", type=float, default=100, help="запросов в минуту (лимит API общий с витриной)")
    ap.add_argument("--only", default="", help="id через запятую или категория теста")
    a = ap.parse_args()
    rows = [json.loads(line) for line in open(a.queries, encoding="utf-8") if line.strip()]
    if a.only:
        keys = set(a.only.split(","))
        rows = [r for r in rows if r["id"] in keys or r["category_of_test"] in keys]
    PACE_SEC[0] = 60.0 / max(a.rate, 1)
    os.makedirs(os.path.join(a.out, "raw"), exist_ok=True)
    feed = load_feed(a.feed)
    print(f"запросов {len(rows)}, фид {len(feed)} позиций", file=sys.stderr)
    results = {}

    def run(row):
        resp, err, dt = post(a.api, row["query"])
        if resp is not None:
            with open(os.path.join(a.out, "raw", row["id"] + ".json"), "w", encoding="utf-8") as f:
                json.dump(resp, f, ensure_ascii=False)
        r = judge(row, resp, err, feed)
        r["ms"] = round(dt * 1000)
        return r

    with cf.ThreadPoolExecutor(max_workers=max(1, min(a.workers, 4))) as ex:
        for n, r in enumerate(ex.map(run, rows), 1):
            results[r["id"]] = r
            if n % 50 == 0:
                print(f"  {n}/{len(rows)}", file=sys.stderr)
    with open(os.path.join(a.out, "results.jsonl"), "w", encoding="utf-8") as f:
        for row in rows:
            f.write(json.dumps(results[row["id"]], ensure_ascii=False) + "\n")
    by_cat = defaultdict(Counter)
    codes = Counter()
    for r in results.values():
        by_cat[r["category_of_test"]][r["verdict"]] += 1
        for sev, code, _ in r.get("violations", []):
            codes[(sev, code)] += 1
    summary = {
        "total": len(results),
        "verdicts": Counter(r["verdict"] for r in results.values()),
        "by_category": {k: dict(c) for k, c in sorted(by_cat.items())},
        "violations": {f"{s}:{c}": n for (s, c), n in codes.most_common()},
    }
    json.dump(summary, open(os.path.join(a.out, "summary.json"), "w"), ensure_ascii=False, indent=1)
    lines = ["| Категория | Всего | pass | warn | fail |", "|---|---:|---:|---:|---:|"]
    for k, c in sorted(by_cat.items()):
        lines.append(f"| {k} | {sum(c.values())} | {c['pass']} | {c['warn']} | {c['fail']} |")
    t = summary["verdicts"]
    lines.append(f"| **всего** | {len(results)} | {t['pass']} | {t['warn']} | {t['fail']} |")
    lines += ["", "| Нарушение | Строк |", "|---|---:|"] + [f"| {k} | {n} |" for k, n in summary["violations"].items()]
    open(os.path.join(a.out, "summary.md"), "w").write("\n".join(lines) + "\n")
    print("\n".join(lines))
    if a.compare and os.path.exists(a.compare):
        old = {json.loads(line)["id"]: json.loads(line) for line in open(a.compare, encoding="utf-8")}
        fixed, broke = [], []
        for i, r in results.items():
            o = old.get(i)
            if not o:
                continue
            if o["verdict"] == "fail" and r["verdict"] != "fail":
                fixed.append(i)
            if o["verdict"] != "fail" and r["verdict"] == "fail":
                broke.append(i)
        print(f"\nпочинилось {len(fixed)}: {' '.join(fixed)}\nсломалось {len(broke)}: {' '.join(broke)}")
    return 0


if __name__ == "__main__":
    sys.exit(main())
