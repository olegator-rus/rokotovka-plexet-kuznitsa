#!/usr/bin/env python3
"""Тест склонений (контур эталонов): пара «именительный падеж — словоформа» должна давать одну и ту же цель ответа.

Набор — data/softline/eval/declension-pairs.jsonl: vendor, base, form. Цель — поле target ответа
(тип и URL после нормализации, как в customer_eval.py); у цели «категория» без URL сравнивается заголовок. Плюс
первое семейство выдачи — для справки.

  API=http://127.0.0.1:8092 python3 scripts/eval/declension_eval.py --out data/softline/eval/results/declension-after
"""
import argparse
import json
import os
import re
import sys
import time

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import customer_eval as ce  # noqa: E402

PAIRS = os.path.join(ce.ROOT, "data", "softline", "eval", "declension-pairs.jsonl")


def target_key(d):
    t = d.get("target") or {}
    url = ce.normalize_url(t.get("url") or "")
    return f"{t.get('type') or '—'} {url or (t.get('title') or '')}".strip()


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--pairs", default=PAIRS)
    ap.add_argument("--out")
    a = ap.parse_args(argv)
    rows = [json.loads(l) for l in open(a.pairs, encoding="utf-8") if l.strip()]
    cache, res = {}, []
    for r in rows:
        keys = []
        for q in (r["base"], r["form"]):
            if q not in cache:
                d = ce.search(q, "live")
                items = d.get("items") or []
                cache[q] = {"target": target_key(d), "first_family": items[0].get("family_id") if items else ""}
                time.sleep(0.1)
            keys.append(cache[q])
        same = keys[0]["target"] == keys[1]["target"] and keys[0]["target"] != "—"
        res.append({**r, "same": same, "base_target": keys[0]["target"], "form_target": keys[1]["target"]})
        print(f"{'да ' if same else 'НЕТ'} {r['vendor'][:12]:13} {r['base'][:28]:29} {r['form'][:30]:31} "
              f"{keys[0]['target'][:55]}" + ("" if same else f"  ≠  {keys[1]['target'][:55]}"))
    n = sum(r["same"] for r in res)
    print(f"\nодинаковая цель: {n} из {len(res)}")
    if a.out:
        base = re.sub(r"\.(json)$", "", a.out)
        with open(base + ".json", "w", encoding="utf-8") as f:
            json.dump({"api": ce.API, "same": n, "total": len(res), "pairs": res}, f, ensure_ascii=False, indent=1)
        print(f"отчёт: {base}.json")


if __name__ == "__main__":
    main(sys.argv[1:])
