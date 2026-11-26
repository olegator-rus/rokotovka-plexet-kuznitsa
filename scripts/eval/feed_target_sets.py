#!/usr/bin/env python3
"""Наборы оценки «по цели» для `go run . eval` (контур эталонов) — из исходных файлов в один формат.

Пишет data/softline/eval/{etalon,tz,neighbors,understanding}.jsonl (каталог target_sets в mapping.yaml). Строка:
  {"id", "text", "group", "vendor", "kind", "distortion", "expect": {...}, "why"}
expect (backend/domain/eval_target.go): type = family | vendor | vendor_category | category | sku | none;
group_ids, vendor_slug, category_ids, url; top (> 0 — проверка по первым N карточкам, а не по цели ответа);
exclude, alt_min, hard {license_kind, org_type, duration_months, quantity}, normalized, route_not; also — другие
допустимые цели.

Источники (менять там, затем перезапускать скрипт):
  etalon        — data/softline/eval/customer-queries.jsonl (10 эталонов заказчика);
  tz            — 7 примеров из ТЗ; ожидания — наша гипотеза по контрольному набору понимания (у заказчика цели нет);
  neighbors     — data/softline/eval/neighbor-queries.jsonl (114 соседей; вид искажения — как в
                  scripts/eval/neighbor_eval.py);
  understanding — data/softline/eval/understanding-set.jsonl (контрольный набор понимания).

Закрытый набор сюда не входит и этим скриптом не читается.

  python3 scripts/eval/feed_target_sets.py            # пишет файлы
  python3 scripts/eval/feed_target_sets.py --check    # только сверка: файлы совпадают с источниками
"""
import argparse
import json
import os
import re
import sys

sys.path.insert(0, os.path.dirname(os.path.abspath(__file__)))
import neighbor_eval as ne  # noqa: E402 — вид искажения соседей

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
OUT = os.path.join(ROOT, "data", "softline", "eval")
ETALON = os.path.join(ROOT, "data", "softline", "eval", "customer-queries.jsonl")
NEIGHBORS = os.path.join(ROOT, "data", "softline", "eval", "neighbor-queries.jsonl")
UNDERSTANDING = os.path.join(ROOT, "data", "softline", "eval", "understanding-set.jsonl")

# Примеры из ТЗ: цели заказчик не дал — ожидание наше (по набору понимания и).
TZ = [
    ("сервер", {"type": "category", "category_ids": ["86563"], "exclude": ["offer-535372", "offer-535373"]},
     "ТЗ: «сервер» — оборудование, а не ПО со словом «сервер»; в фиде это «Rack-серверы», рабочие "
     "станции DEPO Race — не серверы"),
    ("антивирь", {"type": "category", "category_ids": ["650", "3775"]},
     "жаргон «антивирус»: категория антивирусов (для организаций или для дома)"),
    ("офис", {"type": "category", "category_ids": ["1145", "222323"]},
     "жаргон «офисный пакет»: офисные пакеты Microsoft или российские"),
    ("винда", {"type": "category", "category_ids": ["445047", "2050"],
               "also": [{"type": "family", "group_ids": ["286676", "310176"]}]},
     "жаргон Windows: категория Windows (в наличии только Windows 11 Pro и Home) или само семейство"),
    ("астра линукс", {"type": "family", "group_ids": ["121231"]},
     "транслит Astra Linux → Astra Linux Special Edition (как эталон «astra linux»)"),
    ("мдаемон", {"type": "vendor", "vendor_slug": "alt-n-technologies-ltd",
                 "also": [{"type": "vendor_category", "vendor_slug": "alt-n-technologies-ltd"}]},
     "транслит MDaemon: 6 семейств вендора в разных категориях → страница вендора (правило цели, п. 6)"),
    ("Astra Linux", {"type": "family", "group_ids": ["121231"]},
     "пример ТЗ с заглавными буквами: тот же ответ, что у эталона «astra linux»"),
]

QTY = re.compile(r"\bна (\d+)\s*(пк|устройств|узл|пользовател|компьютер|раб)")


def hard_of(text):
    """Жёсткие условия, названные словами запроса: продление и число мест."""
    h = {}
    t = text.lower()
    if "продлен" in t or "продлить" in t:
        h["license_kind"] = "renewal"
    m = QTY.search(t)
    if m:
        h["quantity"] = int(m.group(1))
    return h


def etalon_rows():
    rows = []
    for i, line in enumerate(open(ETALON, encoding="utf-8"), 1):
        if not line.strip():
            continue
        r = json.loads(line)
        e = {"type": r["target_type"], "url": r["target_url"]}
        if r.get("group_id"):
            e["group_ids"] = [str(r["group_id"])]
        if r.get("vendor_slug"):
            e["vendor_slug"] = r["vendor_slug"]
        if r.get("category_id"):
            e["category_ids"] = [str(r["category_id"])]
        rows.append({"id": f"e{i:02d}", "text": r["query"], "group": "etalon", "expect": e,
                     "why": "эталон заказчика: целевой URL из файла заказчика"})
    return rows


def tz_rows():
    return [{"id": f"t{i:02d}", "text": q, "group": "tz", "expect": e, "why": why}
            for i, (q, e, why) in enumerate(TZ, 1)]


def neighbor_rows():
    rows = []
    for i, line in enumerate(open(NEIGHBORS, encoding="utf-8"), 1):
        if not line.strip():
            continue
        r = json.loads(line)
        t = r["target_type"]
        e = {"type": t}
        if t == "family":
            e["group_ids"] = [str(g) for g in r["group_ids"]]
        else:
            e["vendor_slug"] = r["vendor_slug"]
        if t == "vendor_category":
            e["category_ids"] = [str(r["category_id"])]
            alt = ne.alt_groups(r)
            if alt:
                e["group_ids"] = alt
        h = hard_of(r["query"])
        if h:
            e["hard"] = h
        why = r.get("note") or f"сосед эталона вендора {r['vendor']}: {r['kind']}"
        rows.append({"id": f"n{i:03d}", "text": r["query"], "group": "neighbor", "vendor": r["vendor"],
                     "kind": r["kind"], "distortion": ne.distortion(r["kind"]), "expect": e, "why": why})
    return rows


def understanding_rows():
    rows = []
    for i, line in enumerate(open(UNDERSTANDING, encoding="utf-8"), 1):
        if not line.strip():
            continue
        r = json.loads(line)
        x = r["expect"]
        top = int(r.get("top", 1))
        if "family" in x:
            e = {"type": "family", "group_ids": [str(g) for g in x["family"]]}
        elif "vendor" in x:
            e = {"type": "vendor", "vendor_slug": x["vendor"][0]}
            if len(x["vendor"]) > 1:
                e["also"] = [{"type": "vendor", "vendor_slug": v, "top": top} for v in x["vendor"][1:]]
        elif "category" in x:
            e = {"type": "category", "category_ids": [str(c) for c in x["category"]]}
        else:
            raise SystemExit(f"understanding: строка {i} без family/vendor/category")
        e["top"] = top  # набор понимания проверяет первые N карточек, а не цель ответа
        for k in ("exclude", "normalized", "route_not"):
            if k in x:
                e[k] = x[k]
        rows.append({"id": f"u{i:02d}", "text": r["query"], "group": "understanding", "kind": r.get("type", ""),
                     "expect": e, "why": "контрольный набор понимания (контур фида)"})
    return rows


def dump(rows):
    return "".join(json.dumps(r, ensure_ascii=False) + "\n" for r in rows)


def main(argv=None):
    ap = argparse.ArgumentParser(description="Наборы оценки «по цели» для go run . eval")
    ap.add_argument("--check", action="store_true", help="не писать, только сверить с источниками")
    a = ap.parse_args(argv)
    sets = {"etalon": etalon_rows(), "tz": tz_rows(), "neighbors": neighbor_rows(),
            "understanding": understanding_rows()}
    os.makedirs(OUT, exist_ok=True)
    stale = []
    for name, rows in sets.items():
        path = os.path.join(OUT, name + ".jsonl")
        text = dump(rows)
        cur = open(path, encoding="utf-8").read() if os.path.exists(path) else ""
        if cur != text:
            stale.append(name)
            if not a.check:
                with open(path, "w", encoding="utf-8") as f:
                    f.write(text)
        print(f"{name}: {len(rows)}{' — обновлён' if cur != text and not a.check else ''}")
    if a.check and stale:
        print("расходятся с источниками: " + ", ".join(stale))
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
