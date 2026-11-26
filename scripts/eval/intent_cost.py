#!/usr/bin/env python3
"""Входные токены разбора маршрута D на вызов (стоимость системного промпта разбора, intent.go).

Запросы — по строке в файле (data/softline/eval/intent-cost-queries.txt). Для каждого — маршрут, вызывалась ли
модель (trace.llm), входные и выходные токены, стоимость, итог разбора (категория, вендор, референс). Кэш разборов
не обходится: запросы подобраны новые, а после смены промпта ключ кэша меняется сам — оба прогона идут в модель.
Ответ из кэша (вызова не было) в средние не входит и помечен.

  API=http://127.0.0.1:8092 python3 scripts/eval/intent_cost.py --out data/softline/eval/results/intent-cost-after
"""
import argparse
import json
import os
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import customer_eval as ce  # noqa: E402

QUERIES = os.path.join(ce.ROOT, "data", "softline", "eval", "intent-cost-queries.txt")


def main(argv=None):
    ap = argparse.ArgumentParser()
    ap.add_argument("--queries", default=QUERIES)
    ap.add_argument("--out")
    a = ap.parse_args(argv)
    rows = []
    for q in [l.strip() for l in open(a.queries, encoding="utf-8") if l.strip()]:
        d = ce.search(q, "live")
        tr = d.get("trace") or {}
        llm = tr.get("llm") or {}
        steps = {s.get("name"): s for s in tr.get("steps") or []}
        it = steps.get("intent") or {}
        parsed = it.get("fields") or {}
        rows.append({"query": q, "route": tr.get("route"), "parse_source": it.get("source") or tr.get("parse_source"),
                     "input_tokens": llm.get("input_tokens"), "output_tokens": llm.get("output_tokens"),
                     "cost_rub": tr.get("cost_rub"), "category": parsed.get("category_id"),
                     "vendor": parsed.get("vendor_id"), "reference": parsed.get("reference_family"),
                     "kind": parsed.get("kind"), "first_family": ((d.get("items") or [{}])[0]).get("family_id")})
        r = rows[-1]
        print(f"{r['route'] or '—'} {str(r['parse_source']):6} in {str(r['input_tokens']):>6} out {str(r['output_tokens']):>4} "
              f"{str(r['cost_rub']):>7} ₽  кат {str(r['category'])[:8]:8} вендор {str(r['vendor'])[:14]:14} {q[:50]}")
    called = [r for r in rows if r["input_tokens"]]
    if called:
        avg_in = sum(r["input_tokens"] for r in called) / len(called)
        avg_cost = sum(r["cost_rub"] or 0 for r in called) / len(called)
        print(f"\nвызовов модели: {len(called)} из {len(rows)}; входных токенов в среднем {avg_in:.0f}; "
              f"стоимость в среднем {avg_cost:.3f} ₽")
    if a.out:
        with open(a.out + ".json", "w", encoding="utf-8") as f:
            json.dump({"api": ce.API, "rows": rows}, f, ensure_ascii=False, indent=1)
        print(f"отчёт: {a.out}.json")


if __name__ == "__main__":
    main(sys.argv[1:])
