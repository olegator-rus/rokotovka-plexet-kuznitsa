#!/usr/bin/env python3
"""Оценка на запросах-«соседях» эталонов (контур эталонов): 114 запросов по девяти вендорам из эталонов.

Набор — data/softline/eval/neighbor-queries.jsonl: vendor, query, target_type (family | vendor |
vendor_category), kind (вид формулировки), group_ids, vendor_slug, category_id, note.

Сравнение строгое: первая цель ответа (поле target, иначе первый товар — как в customer_eval.py) совпадает с
ожидаемой по URL после нормализации:
  family          — URL семейства одного из group_ids (data/softline/family_urls.jsonl); если URL семейства
                    не собран — тип цели family и первый товар из этого семейства;
  vendor          — https://store.softline.ru/<vendor_slug>/;
  vendor_category — https://store.softline.ru/<vendor_slug>/?filter[category][]=<category_id>; номера семейств,
                    названные в note как допустимые («допустимы и семейства 3953, …»), тоже засчитываются.
Отдельно — «в первых 4»: строгое совпадение или нужное семейство среди первых четырёх карточек (для вендора и
вендора + категории — карточка этого вендора, той же категории второго уровня).

Отчёт: по вендорам, по виду искажения (склонение, транслит, опечатка, раскладка, атрибут, написание, название) и
по исходному виду формулировки; список промахов с первой целью и местом ожидаемого семейства.

  API=http://127.0.0.1:8092 python3 scripts/eval/neighbor_eval.py --out data/softline/eval/results/neighbors-after
  python3 scripts/eval/neighbor_eval.py --before data/softline/eval/results/neighbors-before.json --out …
"""
import argparse
import json
import os
import re
import sys
import time
from collections import OrderedDict, defaultdict

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import customer_eval as ce  # noqa: E402

ROOT = ce.ROOT
QUERIES = os.path.join(ROOT, "data", "softline", "eval", "neighbor-queries.jsonl")
FAMILY_URLS = os.path.join(ROOT, "data", "softline", "family_urls.jsonl")
FEED = os.path.join(ROOT, "data", "softline", "feed", "products.jsonl")

# Вид формулировки → вид искажения отчёта. Составной вид («склонение + транслит») — по первому слову.
DISTORTION = {
    "склонение": "склонение",
    "транслит": "транслит", "английское написание": "транслит", "английское название": "транслит",
    "смешанное написание": "транслит", "смешанные буквы": "транслит", "слитно + транслит": "транслит",
    "опечатка": "опечатка", "е вместо э": "опечатка",
    "раскладка": "раскладка",
    "атрибут": "атрибут", "атрибут лицензии": "атрибут", "атрибут количества": "атрибут", "версия": "атрибут",
    "редакция": "атрибут", "уточнение": "атрибут",
    "слитно": "написание", "раздельно": "написание", "дефис": "написание", "без дефиса": "написание",
    "сокращение": "написание", "жаргон": "написание", "прежнее название": "написание", "преемник": "написание",
    "смешанное": "написание",
}
DISTORTION_ORDER = ["склонение", "транслит", "опечатка", "раскладка", "атрибут", "написание", "название"]


def distortion(kind):
    k = kind.split(" + ")[0] if kind not in DISTORTION else kind
    return DISTORTION.get(k, "название")


def load_family_urls():
    urls = {}
    if os.path.exists(FAMILY_URLS):
        with open(FAMILY_URLS, encoding="utf-8") as f:
            for line in f:
                r = json.loads(line)
                if r.get("family_url"):
                    urls[str(r["group_id"])] = r["family_url"]
    return urls


def load_feed_meta():
    """group_id → (слаг вендора, категория второго уровня)."""
    meta = {}
    with open(FEED, encoding="utf-8") as f:
        for line in f:
            p = json.loads(line)
            meta.setdefault(str(p["family_id"]), (p["vendor"], str(p["category"])))
    return meta


def expected(row, fam_urls):
    t = row["target_type"]
    if t == "family":
        return [fam_urls.get(g, "") for g in row["group_ids"]]
    base = f"{ce.STORE}/{row['vendor_slug']}/"
    if t == "vendor":
        return [base]
    return [f"{base}?filter[category][]={row['category_id']}"]


def alt_groups(row):
    """Семейства, названные в note как допустимые (только у vendor_category)."""
    if row["target_type"] != "vendor_category":
        return []
    note = row.get("note") or ""
    if "допустим" not in note:
        return []
    return re.findall(r"\b\d{3,}\b", note)


def judge(row, d, fam_urls, feed):
    url, ttype, _ = ce.first_target(d) if d.get("_http") == 200 else ("", "", "")
    items = d.get("items") or []
    fids = [str(it.get("family_id") or "") for it in items]
    first_fid = fids[0] if fids else ""
    exp = expected(row, fam_urls)
    strict = any(e and ce.same_url(url, e) for e in exp)
    groups = [str(g) for g in (row.get("group_ids") or [])] + alt_groups(row)
    if not strict and row["target_type"] == "family":
        # URL семейства не собран: цель — семейство и первый товар из него.
        missing = any(not fam_urls.get(g) for g in groups)
        strict = missing and ttype == "family" and first_fid in groups
    if not strict and groups and row["target_type"] == "vendor_category":
        strict = ttype == "family" and any(ce.same_url(url, fam_urls.get(g, "")) for g in groups if fam_urls.get(g))
    top4 = strict
    rank = None
    if groups:
        for i, f in enumerate(fids, 1):
            if f in groups:
                rank = i
                break
        top4 = top4 or (rank is not None and rank <= 4)
    if row["target_type"] in ("vendor", "vendor_category") and not top4:
        for f in fids[:4]:
            v, c = feed.get(f, ("", ""))
            if v == row["vendor_slug"] and (row["target_type"] == "vendor" or c == str(row["category_id"])):
                top4 = True
                break
    return {"strict": strict, "top4": top4, "rank": rank, "first_url": url, "first_type": ttype,
            "first_family": first_fid, "first_title": (items[0].get("title") if items else "") or "",
            "route": (d.get("trace") or {}).get("route"), "normalized": d.get("normalized"),
            "cards": len(items), "http": d.get("_http"), "ms": d.get("_ms")}


def share(rows, key):
    n = sum(1 for r in rows if r[key])
    return n, len(rows)


def fmt(n, of):
    return f"{n}/{of} ({n / of:.0%})" if of else "—"


def breakdown(results, field):
    groups = OrderedDict()
    for r in results:
        groups.setdefault(r[field], []).append(r)
    return groups


def summary_table(results, before, field, order=None):
    groups = breakdown(results, field)
    keys = order or list(groups)
    lines = []
    for k in keys:
        rows = groups.get(k)
        if not rows:
            continue
        s, n = share(rows, "strict")
        t, _ = share(rows, "top4")
        line = f"{k:24} строго {fmt(s, n):13} в первых 4 {fmt(t, n):13}"
        if before:
            b = [before[r["query"]] for r in rows if r["query"] in before]
            if b:
                bs, bn = share(b, "strict")
                bt, _ = share(b, "top4")
                line += f"  было: строго {fmt(bs, bn)}, в первых 4 {fmt(bt, bn)}"
        lines.append(line)
    return lines


def markdown(run, before):
    res = run["results"]
    s, n = share(res, "strict")
    t, _ = share(res, "top4")
    out = ["# Оценка на запросах-соседях эталонов (контур эталонов)", "",
           f"API: `{run['api']}` · каталог: **{run['catalog_version'] or 'неизвестен'}** · "
           f"версия правил: {run['rules_version'] or '—'}", "",
           f"Команда: `{run['command']}`", "",
           "Строго — первая цель ответа (поле `target`, иначе первый товар) совпадает с ожидаемой по URL после "
           "нормализации. «В первых 4» — строго или ожидаемое семейство (для вендора — карточка вендора) среди "
           "первых четырёх карточек.", "",
           f"**Итого: строго {fmt(s, n)}, в первых 4 {fmt(t, n)}.**", ""]
    for title, field, order in (("По вендорам", "vendor", None), ("По виду искажения", "distortion", DISTORTION_ORDER),
                                ("По виду формулировки", "kind", None)):
        out += [f"## {title}", "", "| группа | строго | в первых 4 |" + (" было строго | было в первых 4 |" if before else ""),
                "|---|---|---|" + ("---|---|" if before else "")]
        groups = breakdown(res, field)
        for k in order or list(groups):
            rows = groups.get(k)
            if not rows:
                continue
            gs, gn = share(rows, "strict")
            gt, _ = share(rows, "top4")
            cells = [k, fmt(gs, gn), fmt(gt, gn)]
            if before:
                b = [before[r["query"]] for r in rows if r["query"] in before]
                bs, bn = share(b, "strict")
                bt, _ = share(b, "top4")
                cells += [fmt(bs, bn), fmt(bt, bn)]
            out.append("| " + " | ".join(cells) + " |")
        out.append("")
    out += ["## Все запросы", "", "| вендор | запрос | вид | ожидаем | строго | в 4 | место | первая цель | маршрут |",
            "|---|---|---|---|---|---|---|---|---|"]
    for r in res:
        exp = ",".join(r["group_ids"]) if r["group_ids"] else r["target_type"]
        out.append("| " + " | ".join(str(c).replace("|", "\\|") for c in [
            r["vendor"], r["query"], r["kind"], exp, "да" if r["strict"] else "нет", "да" if r["top4"] else "нет",
            r["rank"] or "—", f"{r['first_type'] or '—'} {ce.normalize_url(r['first_url']) or ''}".strip(),
            r["route"] or "—"]) + " |")
    return "\n".join(out) + "\n"


def main(argv=None):
    ap = argparse.ArgumentParser(description="Оценка на запросах-соседях эталонов")
    ap.add_argument("--queries", default=QUERIES)
    ap.add_argument("--out", help="путь без расширения: <out>.json и <out>.md")
    ap.add_argument("--before", help="прошлый отчёт .json — колонки «было»")
    ap.add_argument("--pause", type=float, default=0.15)
    a = ap.parse_args(argv)
    fam_urls, feed = load_family_urls(), load_feed_meta()
    rows = [json.loads(l) for l in open(a.queries, encoding="utf-8") if l.strip()]
    before = {}
    if a.before:
        before = {r["query"]: r for r in json.load(open(a.before, encoding="utf-8"))["results"]}
    m = ce.meta()
    results = []
    for row in rows:
        d = ce.search(row["query"], "live")
        j = judge(row, d, fam_urls, feed)
        results.append({"vendor": row["vendor"], "query": row["query"], "kind": row["kind"],
                        "distortion": distortion(row["kind"]), "target_type": row["target_type"],
                        "group_ids": row.get("group_ids") or [], "expected": expected(row, fam_urls), **j})
        time.sleep(a.pause)
    run = {"api": ce.API, "catalog_version": m.get("catalog_version"), "rules_version": m.get("rules_version"),
           "command": " ".join(["python3", os.path.relpath(os.path.abspath(__file__), os.getcwd()), *(argv or [])]),
           "results": results}
    s, n = share(results, "strict")
    t, _ = share(results, "top4")
    print("По вендорам:")
    for line in summary_table(results, before, "vendor"):
        print("  " + line)
    print("По виду искажения:")
    for line in summary_table(results, before, "distortion", DISTORTION_ORDER):
        print("  " + line)
    print("\nПромахи (строго):")
    for r in results:
        if not r["strict"]:
            print(f"  {r['vendor'][:12]:13} {r['query'][:34]:35} ждём {(','.join(r['group_ids']) or r['target_type'])[:16]:17}"
                  f" в4 {'да ' if r['top4'] else 'нет'} место {str(r['rank'] or '—'):3} → {r['first_type'] or '—'} "
                  f"{ce.normalize_url(r['first_url'])[:70]}")
    print(f"\nИтого: строго {fmt(s, n)}, в первых 4 {fmt(t, n)}")
    if before:
        b = list(before.values())
        bs, bn = share(b, "strict")
        bt, _ = share(b, "top4")
        fixed = sum(1 for r in results if r["strict"] and not before.get(r["query"], {}).get("strict", True))
        broken = sum(1 for r in results if not r["strict"] and before.get(r["query"], {}).get("strict", False))
        print(f"Было: строго {fmt(bs, bn)}, в первых 4 {fmt(bt, bn)}; исправлено {fixed}, сломано {broken}")
    if a.out:
        base = re.sub(r"\.(json|md)$", "", a.out)
        os.makedirs(os.path.dirname(os.path.abspath(base)), exist_ok=True)
        with open(base + ".json", "w", encoding="utf-8") as f:
            json.dump(run, f, ensure_ascii=False, indent=1)
        with open(base + ".md", "w", encoding="utf-8") as f:
            f.write(markdown(run, before))
        print(f"отчёт: {base}.json, {base}.md")


if __name__ == "__main__":
    main(sys.argv[1:])
