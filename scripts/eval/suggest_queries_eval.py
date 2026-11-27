#!/usr/bin/env python3
"""Подсказки-запросы при вводе на фиде (контур эталонов): 15 префиксов → первые три подсказки-запроса и первый товар панели.

Снимок для сравнения «до / после» (колонка «Подсказки» и первая карточка товарной секции). Жёстких проверок две:
в тексте подсказок нет кавычек «» и "", и подсказки не пусты у префиксов, которые называют товар.

Запуск из корня репозитория (API по умолчанию http://127.0.0.1:8088):
  python3 scripts/eval/suggest_queries_eval.py --out data/softline/eval/results/suggest-queries
  API=http://127.0.0.1:8093 python3 scripts/eval/suggest_queries_eval.py
--out <путь без расширения> пишет <путь>.json и <путь>.md.
"""

import argparse
import json
import os
import sys
import urllib.parse
import urllib.request

API = os.environ.get("API", "http://127.0.0.1:8088").rstrip("/")
PREFIXES = ["вин", "каспер", "астр", "крип", "офи", "анти", "серв", "кибер", "1с", "dr", "мтс", "365", "ред", "альт",
            "adob"]


def suggest(q):
    url = API + "/api/suggest?" + urllib.parse.urlencode({"q": q})
    req = urllib.request.Request(url, headers={"Origin": "http://localhost:5177"})
    with urllib.request.urlopen(req, timeout=30) as r:
        return json.load(r)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", help="путь без расширения: пишутся .json и .md")
    args = ap.parse_args()
    rows, fails = [], 0
    for p in PREFIXES:
        d = suggest(p)
        sugg = [s["text"] for s in d.get("suggestions") or []][:3]
        prod = (d.get("products") or [{}])[0].get("family_name") if d.get("products") else None
        quotes = any(ch in s for s in sugg for ch in "«»\"")
        ok = bool(sugg) and not quotes
        fails += 0 if ok else 1
        rows.append({"prefix": p, "suggestions": sugg, "first_product": prod, "ok": ok,
                     "vendors": [v["name"] for v in d.get("vendors") or []]})
    lines = ["# Подсказки-запросы: 15 префиксов", "", f"API: {API}", "",
             "| Ввод | Подсказки (первые три) | Первый товар | Вендоры | Проверка |", "| --- | --- | --- | --- | --- |"]
    for r in rows:
        lines.append(f"| {r['prefix']} | {'; '.join(r['suggestions']) or '—'} | {r['first_product'] or '—'} | "
                     f"{', '.join(r['vendors']) or '—'} | {'да' if r['ok'] else 'НЕТ'} |")
    lines += ["", f"Пустых или с кавычками: {fails} из {len(rows)}"]
    print("\n".join(lines))
    if args.out:
        with open(args.out + ".json", "w", encoding="utf-8") as f:
            json.dump({"api": API, "rows": rows, "fails": fails}, f, ensure_ascii=False, indent=2)
        with open(args.out + ".md", "w", encoding="utf-8") as f:
            f.write("\n".join(lines) + "\n")
    return 1 if fails else 0


if __name__ == "__main__":
    sys.exit(main())
