#!/usr/bin/env python3
"""Блок «Дополните решение» на эталонах заказчика и примерах ТЗ (требования 5 и К2).

Для каждого запроса делает POST /api/search с source=compare (в аналитику и стоимость не входит) и проверяет блок
companions по правилам сборки (backend/services/upsell_classes.go, upsell_feed.go):
  - число карточек и не больше 4;
  - у каждой карточки есть reason и evidence.level;
  - только с ценой (price_mode=fixed, price_rub > 0);
  - одно семейство на вендора: у вендора не больше одного семейства, кроме семейства якоря;
  - доля уровня A там, где для пары «якорь × семейство карточки» есть документ уровня A в data/softline/compat_docs.csv;
  - нет карточки из семейства якоря, кроме техподдержки и продления (class=renewal).
Запросы: data/softline/eval/customer-queries.jsonl (10 эталонов) и data/softline/eval/tz.jsonl (7 примеров ТЗ).

Запуск из корня репозитория:
  python3 scripts/eval/upsell_eval.py --out data/softline/eval/results/upsell-report
  API=http://127.0.0.1:8093 python3 scripts/eval/upsell_eval.py
--out <путь без расширения> пишет <путь>.json и <путь>.md. Код выхода 1 — нарушено хотя бы одно жёсткое правило
(reason/основание, цена, ≤4, одно семейство на вендора).
"""

import argparse
import csv
import json
import os
import sys
import time
import urllib.error
import urllib.request

API = os.environ.get("API", "http://127.0.0.1:8088").rstrip("/")
ORIGIN = "http://localhost:5177"
ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
ETALONS = os.path.join(ROOT, "data/softline/eval/customer-queries.jsonl")
TZ = os.path.join(ROOT, "data/softline/eval/tz.jsonl")
PRODUCTS = os.path.join(ROOT, "data/softline/feed/products.jsonl")
COMPAT = os.path.join(ROOT, "data/softline/compat_docs.csv")
UPSELL_MAX = 4
SAME_FAMILY_OK = {"renewal"}  # техподдержка и продление


def post_json(path, body, retries=6):
    data = json.dumps(body).encode("utf-8")
    for attempt in range(retries):
        req = urllib.request.Request(API + path, data=data, method="POST",
                                     headers={"Origin": ORIGIN, "Content-Type": "application/json"})
        try:
            with urllib.request.urlopen(req, timeout=60) as r:
                return json.load(r)
        except urllib.error.HTTPError as e:
            if e.code == 429 and attempt < retries - 1:
                wait = e.headers.get("Retry-After")
                time.sleep(float(wait) if wait and wait.replace(".", "", 1).isdigit() else 2 * (attempt + 1))
                continue
            raise


def load_queries():
    qs = []
    with open(ETALONS, encoding="utf-8") as f:
        for i, line in enumerate(f, 1):
            if line.strip():
                qs.append({"set": "etalon", "id": f"e{i:02d}", "query": json.loads(line)["query"]})
    with open(TZ, encoding="utf-8") as f:
        for line in f:
            if line.strip():
                e = json.loads(line)
                qs.append({"set": "tz", "id": e["id"], "query": e["text"]})
    return qs


def load_families():
    fam = {}
    with open(PRODUCTS, encoding="utf-8") as f:
        for line in f:
            p = json.loads(line)
            if p.get("family_id"):
                fam[p["family_id"]] = (p.get("family_name") or "", p.get("vendor") or "")
    return fam


def load_doc_pairs():
    """Пары имён семейств (неупорядоченные), для которых есть документ уровня A."""
    lines = [l for l in open(COMPAT, encoding="utf-8") if not l.startswith("#")]
    pairs = {}
    for r in csv.DictReader(lines):
        prods = [x.strip() for x in r["product_family"].split("|") if x.strip()]
        plats = [x.strip() for x in r["platform_family"].split("|") if x.strip()]
        for a in prods:
            for b in plats:
                key = frozenset((a, b))
                if r["level"] == "A" or key not in pairs:
                    pairs[key] = r["level"]
    return pairs


def judge(resp, fam, doc_pairs):
    block = resp.get("companions")
    if not block:
        return {"block": False, "cards": 0}
    anchor_fid = (block.get("anchor") or {}).get("family_id", "")
    anchor_name = fam.get(anchor_fid, ("", ""))[0]
    items = block.get("items") or []
    cards, vendor_fams = [], {}
    for it in items:
        ev = it.get("evidence") or {}
        fid = it.get("family_id", "")
        name = fam.get(fid, ("", ""))[0]
        doc = doc_pairs.get(frozenset((anchor_name, name))) if fid != anchor_fid else None
        price = it.get("price_rub")
        cards.append({
            "title": it.get("title"), "family_id": fid, "family": name, "vendor": it.get("vendor"),
            "class": it.get("class"), "label": it.get("label"), "price_rub": price,
            "level": ev.get("level") or it.get("evidence_level"), "kind": ev.get("kind"),
            "has_reason": bool((it.get("reason") or "").strip()),
            "has_level": bool(ev.get("level")),
            "priced": it.get("price_mode") == "fixed" and isinstance(price, (int, float)) and price > 0,
            "same_family": fid == anchor_fid, "doc_a": doc == "A",
        })
        if fid != anchor_fid:
            vendor_fams.setdefault(it.get("vendor"), set()).add(fid)
    doc_cards = [c for c in cards if c["doc_a"]]
    bad_same = [c for c in cards if c["same_family"] and c["class"] not in SAME_FAMILY_OK]
    return {
        "block": True, "anchor_family_id": anchor_fid, "anchor_family": anchor_name,
        "anchor_confidence": (block.get("anchor") or {}).get("confidence"),
        "cards": len(cards),
        "all_reason_and_level": all(c["has_reason"] and c["has_level"] for c in cards),
        "all_priced": all(c["priced"] for c in cards),
        "one_family_per_vendor": all(len(v) <= 1 for v in vendor_fams.values()),
        "le_4": len(cards) <= UPSELL_MAX,
        "doc_a_cards": len(doc_cards), "doc_a_shown_a": sum(1 for c in doc_cards if c["level"] == "A"),
        "same_family_other": [c["label"] for c in bad_same],
        "levels": {lv: sum(1 for c in cards if c["level"] == lv) for lv in sorted({c["level"] for c in cards})},
        "items": cards,
    }


def summarize(rows):
    with_block = [r for r in rows if r["block"]]
    cards = [c for r in with_block for c in r["items"]]
    doc_cards = [c for c in cards if c["doc_a"]]
    levels = {}
    for c in cards:
        levels[c["level"]] = levels.get(c["level"], 0) + 1
    return {
        "queries": len(rows), "with_block": len(with_block), "cards_total": len(cards),
        "cards_per_block": {str(n): sum(1 for r in with_block if r["cards"] == n) for n in range(0, UPSELL_MAX + 2)
                            if any(r["cards"] == n for r in with_block)},
        "all_reason_and_level": sum(1 for r in with_block if r["all_reason_and_level"]),
        "all_priced": sum(1 for r in with_block if r["all_priced"]),
        "one_family_per_vendor": sum(1 for r in with_block if r["one_family_per_vendor"]),
        "le_4": sum(1 for r in with_block if r["le_4"]),
        "no_same_family_other": sum(1 for r in with_block if not r["same_family_other"]),
        "same_family_other_labels": sorted({l for r in with_block for l in r["same_family_other"]}),
        "doc_a_cards": len(doc_cards), "doc_a_shown_a": sum(1 for c in doc_cards if c["level"] == "A"),
        "levels": dict(sorted(levels.items(), key=lambda x: str(x[0]))),
        "same_family_cards": sum(1 for c in cards if c["same_family"]),
        "kinds": {k: sum(1 for c in cards if c["kind"] == k) for k in sorted({str(c["kind"]) for c in cards})},
        "classes": {k: sum(1 for c in cards if c["class"] == k) for k in sorted({str(c["class"]) for c in cards})},
    }


def frac(a, b):
    return f"{a}/{b}" if b else "—"


def render_md(rows, total, by_set):
    out = ["# Блок «Дополните решение»: эталоны и примеры ТЗ", "",
           "Требования 5 и К2. Скрипт `scripts/eval/upsell_eval.py`, поиск с `source=compare`. Правила проверки — "
           "в шапке скрипта. Знаменатель у долей — ответы, где блок есть.", "",
           "## Итог", "",
           "| Показатель | Всего | Эталоны | ТЗ |", "| --- | --- | --- | --- |"]
    e, t = by_set["etalon"], by_set["tz"]

    def line(name, key):
        out.append(f"| {name} | {frac(total[key], total['with_block'])} | {frac(e[key], e['with_block'])} | "
                   f"{frac(t[key], t['with_block'])} |")
    out.append(f"| Ответов с блоком | {total['with_block']}/{total['queries']} | {e['with_block']}/{e['queries']} | "
               f"{t['with_block']}/{t['queries']} |")
    out.append(f"| Карточек всего | {total['cards_total']} | {e['cards_total']} | {t['cards_total']} |")
    line("У всех карточек есть причина и уровень основания", "all_reason_and_level")
    line("Все карточки с ценой", "all_priced")
    line("Одно семейство на вендора", "one_family_per_vendor")
    line("Не больше 4 карточек", "le_4")
    line("Нет карточки из семейства якоря, кроме техподдержки и продления", "no_same_family_other")
    out.append(f"| Уровень A там, где есть документ A (карточек) | {frac(total['doc_a_shown_a'], total['doc_a_cards'])} | "
               f"{frac(e['doc_a_shown_a'], e['doc_a_cards'])} | {frac(t['doc_a_shown_a'], t['doc_a_cards'])} |")
    out += ["", f"Уровни оснований по карточкам: "
            + ", ".join(f"{k} — {v}" for k, v in total["levels"].items()) + ".",
            "Виды оснований: " + ", ".join(f"{k} — {v}" for k, v in total["kinds"].items())
            + "; классы карточек: " + ", ".join(f"{k} — {v}" for k, v in total["classes"].items()) + ".",
            f"Карточек из семейства якоря: {total['same_family_cards']}; из них не техподдержка и не продление: "
            + (", ".join(total["same_family_other_labels"]) or "нет") + ".",
            f"Распределение числа карточек в блоке: "
            + ", ".join(f"{k} — {v}" for k, v in total["cards_per_block"].items()) + ".", "",
            "У цели-категории («антивирь», «офис», «винда») якорь блока — первое семейство выдачи, кроме "
            "«Популярного» и альтернатив (`TestUpsellAnchorNeedsNamedTarget`); блока нет, если у якоря нет ни класса "
            "в таблице, ни типа лицензии, ни документа (оборудование на «сервер»).", "",
            "## По запросам", "",
            "| Набор | Запрос | Якорь | Карточек | Причина и основание | Цена | Вендор | ≤4 | A по документу | "
            "Своё семейство не продление | Карточки (уровень, вид) |",
            "| --- | --- | --- | --- | --- | --- | --- | --- | --- | --- | --- |"]
    yn = lambda b: "да" if b else "НЕТ"
    for r in rows:
        if not r["block"]:
            out.append(f"| {r['set']} | {r['query']} | блока нет | 0 | — | — | — | — | — | — | — |")
            continue
        cards = "; ".join(f"{c['label'] or c['class']}: {c['family'] or c['title'][:40]} ({c['level']}, {c['kind']})"
                          for c in r["items"])
        out.append(f"| {r['set']} | {r['query']} | {r['anchor_family']} | {r['cards']} | "
                   f"{yn(r['all_reason_and_level'])} | {yn(r['all_priced'])} | {yn(r['one_family_per_vendor'])} | "
                   f"{yn(r['le_4'])} | {frac(r['doc_a_shown_a'], r['doc_a_cards'])} | "
                   f"{', '.join(r['same_family_other']) or 'нет'} | {cards} |")
    out += ["", "Точность самого подбора (нужна ли покупателю карточка) не размечалась: скрипт проверяет правила "
            "сборки блока и основания, а не полезность.", ""]
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", help="путь без расширения: пишутся .json и .md")
    args = ap.parse_args()
    fam, doc_pairs = load_families(), load_doc_pairs()
    rows = []
    for q in load_queries():
        resp = post_json("/api/search", {"query": q["query"], "source": "compare"})
        r = {**q, **judge(resp, fam, doc_pairs)}
        rows.append(r)
        print(f"{q['set']:<6} {q['query'][:36]:<36} блок={'да' if r['block'] else 'нет'} карточек={r['cards']}"
              + (f" цена={r['all_priced']} вендор={r['one_family_per_vendor']} A={r['doc_a_shown_a']}/{r['doc_a_cards']}"
                 f" своё={r['same_family_other']}" if r["block"] else ""))
    total = summarize(rows)
    by_set = {s: summarize([r for r in rows if r["set"] == s]) for s in ("etalon", "tz")}
    hard_fail = sum(1 for r in rows if r["block"] and not (
        r["all_reason_and_level"] and r["all_priced"] and r["one_family_per_vendor"] and r["le_4"]))
    print(json.dumps(total, ensure_ascii=False))
    if args.out:
        with open(args.out + ".json", "w", encoding="utf-8") as f:
            json.dump({"api": API, "summary": total, "by_set": by_set, "rows": rows}, f, ensure_ascii=False, indent=1)
        with open(args.out + ".md", "w", encoding="utf-8") as f:
            f.write(render_md(rows, total, by_set))
    return 1 if hard_fail else 0


if __name__ == "__main__":
    sys.exit(main())
