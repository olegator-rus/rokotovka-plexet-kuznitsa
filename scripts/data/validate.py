#!/usr/bin/env python3
"""Проверка mock-данных по формату данных.

    python3.13 scripts/data/validate.py                  # проверить data/mock
    python3.13 scripts/data/validate.py --stats          # + таблицы для data/README.md
    python3.13 scripts/data/validate.py --determinism    # + два прогона генератора побайтно равны data/mock

Код возврата 0 — ошибок нет (предупреждения печатаются, но не валят проверку).
"""

import argparse
import hashlib
import json
import re
import subprocess
import sys
import tempfile
from collections import Counter, defaultdict
from datetime import datetime, timedelta, timezone
from pathlib import Path

ROOT = Path(__file__).resolve().parents[2]
FILES = ["catalog.json", "vendors.json", "categories.json", "relations.json", "aliases.json", "clients.json",
         "queries.json", "events.jsonl"]
QUERY_TYPES = ["exact", "sku", "typo", "layout", "slang", "purpose", "license", "analog", "absent", "alias_gap",
               "non_product", "noise", "renewal"]
SETS = ["tune", "test", "regression", "demo"]
LICENSE_TYPES = {"subscription", "perpetual", "renewal", "upgrade", "oem", "cloud"}
METRICS = {"device", "user", "server", "core", "org", None}
EVENT_TYPES = {"search", "click", "add_to_cart", "order", "price_request", "manager_request"}
LOG_START = datetime(2026, 4, 27, tzinfo=timezone.utc)
LOG_DAYS = 30
TOKEN_RE = re.compile(r"[0-9a-zа-я]+")


def tokenize(text):
    return TOKEN_RE.findall(text.lower().replace("ё", "е"))


class Report:
    def __init__(self):
        self.errors, self.warnings = [], []

    def err(self, msg):
        self.errors.append(msg)

    def warn(self, msg):
        self.warnings.append(msg)

    def check(self, cond, msg):
        if not cond:
            self.err(msg)
        return cond


def load(d, r):
    data = {}
    for name in FILES:
        path = d / name
        if not r.check(path.exists(), f"нет файла {name}"):
            continue
        try:
            if name.endswith(".jsonl"):
                data[name] = [json.loads(line) for line in path.read_text(encoding="utf-8").splitlines() if line]
            else:
                data[name] = json.loads(path.read_text(encoding="utf-8"))
        except json.JSONDecodeError as e:
            r.err(f"{name}: невалидный JSON: {e}")
    return data


def check_refs(data, r):
    vendors = {v["id"]: v for v in data["vendors.json"]}
    r.check(len(vendors) == len(data["vendors.json"]), "vendors: повтор id")
    for v in vendors.values():
        r.check(v.get("origin") in ("ru", "foreign") and v.get("name"), f"vendor {v['id']}: name/origin")
    cats = {c["id"]: c for c in data["categories.json"]}
    r.check(len(cats) == len(data["categories.json"]), "categories: повтор id")
    for c in cats.values():
        p = c.get("parent_id")
        if p is not None:
            r.check(p in cats, f"category {c['id']}: нет родителя {p}")
            r.check(p not in cats or cats[p]["parent_id"] is None, f"category {c['id']}: больше двух уровней")
    return vendors, cats


def check_catalog(data, vendors, cats, r):
    cat = data["catalog.json"]
    r.check(isinstance(cat.get("version"), str), "catalog.version")
    products = cat.get("products", [])
    by_id, skus, fams = {}, {}, {}
    for p in products:
        pid = p.get("id")
        where = f"product {pid}"
        r.check(pid not in by_id, f"{where}: повтор id")
        by_id[pid] = p
        for key in ("sku", "family_id", "family_name", "title", "vendor_id", "category_id", "description"):
            r.check(isinstance(p.get(key), str), f"{where}: {key} не строка")
        if p.get("sku"):
            r.check(p["sku"] not in skus, f"{where}: артикул {p['sku']} повторяется")
            skus[p["sku"]] = pid
        r.check(p.get("vendor_id") in vendors, f"{where}: нет вендора {p.get('vendor_id')}")
        r.check(p.get("category_id") in cats, f"{where}: нет категории {p.get('category_id')}")
        r.check(isinstance(p.get("attributes"), dict), f"{where}: attributes")
        lic = p.get("license", {})
        r.check(lic.get("type") in LICENSE_TYPES, f"{where}: license.type {lic.get('type')}")
        r.check(lic.get("metric") in METRICS, f"{where}: license.metric {lic.get('metric')}")
        q, mo = lic.get("quantity"), lic.get("duration_months")
        r.check(q is None or (isinstance(q, int) and q > 0), f"{where}: license.quantity")
        r.check(mo is None or (isinstance(mo, int) and mo > 0), f"{where}: license.duration_months")
        if lic.get("type") in ("perpetual", "oem", "upgrade"):
            r.check(mo is None, f"{where}: бессрочная лицензия со сроком")
        if lic.get("type") in ("subscription", "renewal"):
            r.check(mo is not None, f"{where}: подписка без срока")
        r.check(lic.get("metric") != "org" or q is None, f"{where}: metric org с количеством")
        por, price = p.get("price_on_request"), p.get("price_rub")
        r.check(isinstance(por, bool), f"{where}: price_on_request")
        r.check((price is None) == bool(por), f"{where}: price_rub/price_on_request не согласованы")
        r.check(price is None or (isinstance(price, int) and price > 0), f"{where}: price_rub")
        r.check(p.get("origin") in ("ru", "foreign"), f"{where}: origin")
        if p.get("vendor_id") in vendors:
            r.check(p.get("origin") == vendors[p["vendor_id"]]["origin"], f"{where}: origin не как у вендора")
        r.check(isinstance(p.get("in_reestr"), bool), f"{where}: in_reestr")
        r.check(not (p.get("origin") == "foreign" and p.get("in_reestr")), f"{where}: зарубежное ПО в реестре")
        for key in ("popularity", "margin"):
            val = p.get(key)
            r.check(isinstance(val, (int, float)) and 0 <= val <= 1, f"{where}: {key} вне 0..1")
        sentences = len(re.findall(r"[.!?](\s|$)", p.get("description", "")))
        if not 2 <= sentences <= 5:
            r.warn(f"{where}: в описании {sentences} предложений")
        f = fams.setdefault(p.get("family_id"), (p.get("family_name"), p.get("vendor_id")))
        r.check(f == (p.get("family_name"), p.get("vendor_id")), f"{where}: семейство с разными именем/вендором")
    return by_id, fams


def check_dicts(data, vendors, cats, fams, by_id, r):
    seen = set()
    for rel in data["relations.json"]:
        key = (rel.get("from_family"), rel.get("to_family"), rel.get("type"))
        r.check(key not in seen, f"relation {key}: повтор")
        seen.add(key)
        r.check(rel.get("from_family") in fams and rel.get("to_family") in fams, f"relation {key}: нет семейства")
        r.check(rel.get("from_family") != rel.get("to_family"), f"relation {key}: связь на себя")
        r.check(rel.get("type") in ("substitute", "complement", "renewal", "compatibility"), f"relation {key}: type")
    targets = {"vendor": vendors, "family": fams, "category": cats}
    aliases = set()
    for a in data["aliases.json"]:
        al = a.get("alias")
        r.check(al not in aliases, f"alias {al}: повтор")
        aliases.add(al)
        r.check(a.get("kind") in ("slang", "abbreviation", "transliteration", "alias"), f"alias {al}: kind")
        r.check(a.get("target") in ("vendor", "family", "category", "term"), f"alias {al}: target")
        if a.get("target") == "term":
            r.check(a.get("target_id") is None, f"alias {al}: у term не должно быть target_id")
        else:
            r.check(a.get("target_id") in targets.get(a.get("target"), {}), f"alias {al}: нет {a.get('target_id')}")
    clients = {c["id"]: c for c in data["clients.json"]}
    for need in ("guest", "client-a", "client-b"):
        r.check(need in clients, f"clients: нет {need}")
    r.check(not clients.get("guest", {}).get("purchases"), "clients: у guest есть покупки")
    r.check(not clients.get("client-b", {}).get("purchases"), "clients: у client-b есть покупки")
    ca = clients.get("client-a", {})
    r.check(ca.get("segment") == "smb", "clients: client-a не smb")
    r.check(any(by_id.get(x["product_id"], {}).get("vendor_id") == "kaspersky" for x in ca.get("purchases", [])),
            "clients: client-a не покупал Kaspersky")
    for c in clients.values():
        r.check(c.get("segment") in ("smb", "enterprise", "gov", "individual"), f"client {c['id']}: segment")
        for x in c.get("purchases", []):
            r.check(x.get("product_id") in by_id, f"client {c['id']}: нет товара {x.get('product_id')}")
            r.check(bool(re.fullmatch(r"\d{4}-\d{2}-\d{2}", x.get("date", ""))), f"client {c['id']}: дата")
    return clients


def catalog_text(products, vendors, cats):
    parts = []
    for p in products:
        parts += [p["title"], p["family_name"], p["description"], p["sku"], *map(str, p["attributes"].values())]
    parts += [v["name"] for v in vendors.values()] + [c["name"] for c in cats.values()]
    return " ".join(parts).lower().replace("ё", "е")


def check_queries(data, by_id, clients, vendors, cats, r):
    queries = data["queries.json"]
    ids = Counter(q.get("id") for q in queries)
    r.check(all(n == 1 for n in ids.values()), "queries: повтор id")
    cluster_sets = defaultdict(set)
    for q in queries:
        where = f"query {q.get('id')} «{q.get('text')}»"
        r.check(isinstance(q.get("text"), str) and q["text"].strip(), f"{where}: text")
        r.check(q.get("type") in QUERY_TYPES, f"{where}: type")
        r.check(q.get("set") in SETS, f"{where}: set")
        r.check(isinstance(q.get("freq"), int) and q["freq"] > 0, f"{where}: freq")
        r.check(q.get("client_id") is None or q["client_id"] in clients, f"{where}: client_id")
        j = q.get("judgments", {})
        for pid, g in j.items():
            r.check(pid in by_id, f"{where}: в разметке нет товара {pid}")
            r.check(isinstance(g, int) and 0 <= g <= 3, f"{where}: оценка {g}")
        r.check(q.get("absent") == (q.get("type") == "absent"), f"{where}: absent не согласован с type")
        top = max(j.values(), default=0)
        if q.get("type") in ("non_product", "noise"):
            r.check(not j, f"{where}: у нетоварного запроса есть разметка")
        elif q.get("type") == "absent":
            r.check(top <= 2, f"{where}: у absent есть точный товар (3)")
        elif q.get("type") in ("purpose", "analog"):
            r.check(top >= 2, f"{where}: нет подходящего товара")
        else:
            r.check(top == 3, f"{where}: нет товара с оценкой 3")
        cluster_sets[q.get("cluster")].add(q.get("set"))
    for cl, s in cluster_sets.items():
        r.check(len(s) == 1, f"cluster {cl}: в нескольких наборах {sorted(s)}")

    types = Counter(q["type"] for q in queries)
    for t in QUERY_TYPES:
        r.check(types[t] > 0, f"queries: нет типа {t}")
    r.check(300 <= len(queries) <= 400, f"queries: {len(queries)} вне 300–400")
    demo = sum(q["set"] == "demo" for q in queries)
    r.check(10 <= demo <= 15, f"queries: demo {demo} вне 10–15")
    absent_cl = {q["cluster"] for q in queries if q["type"] == "absent"}
    gap_cl = {q["cluster"] for q in queries if q["type"] == "alias_gap"}
    r.check(6 <= len(absent_cl) <= 10, f"queries: absent-кластеров {len(absent_cl)} вне 6–10")
    r.check(5 <= len(gap_cl) <= 8, f"queries: alias_gap-кластеров {len(gap_cl)} вне 5–8")

    alias_strings = {a["alias"].lower() for a in data["aliases.json"]}
    alias_tokens = {t for a in alias_strings for t in tokenize(a)}
    ctext = catalog_text(list(by_id.values()), vendors, cats)
    ctokens = set(tokenize(ctext))
    for cl in sorted(gap_cl):
        qs = [q for q in queries if q["cluster"] == cl]
        terms = qs[0].get("terms") or []
        r.check(bool(terms), f"alias_gap {cl}: не задан term")
        for term in terms:
            t = term.lower()
            r.check(t not in alias_strings and t not in alias_tokens, f"alias_gap {cl}: «{term}» есть в aliases.json")
            r.check(t not in ctokens, f"alias_gap {cl}: «{term}» есть словом в каталоге")
            prefixed = sorted(x for x in ctokens if x.startswith(t) and x != t)
            if prefixed:
                r.warn(f"alias_gap {cl}: «{term}» — префикс слов каталога {prefixed[:3]} (ловушка для n-gram)")
        for q in qs:
            r.check(any(t.lower() in q["text"].lower() for t in terms), f"alias_gap {cl}: «{q['text']}» без term")
            r.check(max(q["judgments"].values(), default=0) == 3, f"alias_gap {cl}: «{q['text']}» без товара")
    for cl in sorted(absent_cl):
        terms = next(q for q in queries if q["cluster"] == cl).get("terms") or []
        r.check(bool(terms), f"absent {cl}: не задан term")
        for term in terms:
            r.check(term.lower() not in ctext, f"absent {cl}: «{term}» встречается в каталоге")
    return queries


def check_events(data, queries, by_id, clients, r):
    events = data["events.jsonl"]
    r.check(3000 <= len(events) <= 6000, f"events: {len(events)} вне 3–6 тыс.")
    end = LOG_START + timedelta(days=LOG_DAYS)
    prev_ts, days = "", set()
    for i, e in enumerate(events):
        where = f"event #{i + 1}"
        try:
            ts = datetime.strptime(e.get("ts", ""), "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc)
        except ValueError:
            r.err(f"{where}: ts {e.get('ts')}")
            continue
        r.check(LOG_START <= ts < end, f"{where}: ts вне окна логов")
        r.check(e["ts"] >= prev_ts, f"{where}: события не отсортированы по времени")
        prev_ts = e["ts"]
        days.add(ts.date())
        r.check(e.get("type") in EVENT_TYPES, f"{where}: type {e.get('type')}")
        r.check(isinstance(e.get("session_id"), str) and e["session_id"], f"{where}: session_id")
        r.check(e.get("client_id") is None or e["client_id"] in clients, f"{where}: client_id")
        if e["type"] in ("search", "manager_request"):
            r.check(isinstance(e.get("query"), str), f"{where}: query")
        if e["type"] == "search":
            r.check(isinstance(e.get("results_count"), int) and e["results_count"] >= 0, f"{where}: results_count")
        if e["type"] in ("click", "add_to_cart", "order", "price_request"):
            r.check(e.get("product_id") in by_id, f"{where}: product_id")
        if e["type"] == "click":
            r.check(isinstance(e.get("position"), int) and e["position"] >= 1, f"{where}: position")
        if e["type"] == "order":
            r.check(e.get("client_id") != "client-b", f"{where}: заказ у client-b (у него нет истории)")
    r.check(len(days) == LOG_DAYS, f"events: покрыто {len(days)} дней из {LOG_DAYS}")

    freq = Counter()
    for q in queries:
        freq[q["text"]] += q["freq"]
    seen = Counter(e["query"] for e in events if e["type"] == "search")
    for text in sorted(set(freq) | set(seen)):
        r.check(freq[text] == seen[text], f"events: «{text}» — freq {freq[text]}, поисков {seen[text]}")

    by_type = {q["text"]: q["type"] for q in queries if q["type"] in ("absent", "alias_gap")}
    sessions = defaultdict(list)
    for e in events:
        sessions[e["session_id"]].append(e)
    reform = bad_click = 0
    for evs in sessions.values():
        last_search = None
        for e in evs:
            if e["type"] == "search":
                if e["query"] in by_type:
                    r.check(e["results_count"] == 0, f"events: {by_type[e['query']]} «{e['query']}» с результатами")
                if last_search and not last_search.get("_clicked"):
                    gap = (datetime.fromisoformat(e["ts"].replace("Z", "+00:00"))
                           - datetime.fromisoformat(last_search["ts"].replace("Z", "+00:00"))).total_seconds()
                    reform += gap < 60
                last_search = dict(e)
            elif e["type"] == "click" and last_search:
                last_search["_clicked"] = True
                bad_click += last_search["query"] in by_type
    r.check(bad_click == 0, f"events: {bad_click} кликов после absent/alias_gap")
    r.check(reform > 0, "events: нет переформулировок за <60 с")
    r.check(any(e["type"] == "order" for e in events), "events: нет заказов")
    return events, reform


def check_popularity(products, events, r):
    w = {"order": 5, "add_to_cart": 2, "price_request": 2, "click": 1}
    score = Counter()
    for e in events:
        if e["type"] in w:
            score[e["product_id"]] += w[e["type"]]
    ranked = sorted(products, key=lambda p: score[p["id"]])
    for a, b in zip(ranked, ranked[1:]):
        if score[b["id"]] > score[a["id"]]:
            r.check(b["popularity"] > a["popularity"],
                    f"popularity: {b['id']} ({score[b['id']]}) не выше {a['id']} ({score[a['id']]})")
    ordered = {e["product_id"] for e in events if e["type"] == "order"}
    if ordered:
        lo = min(p["popularity"] for p in products if p["id"] in ordered)
        hi = max((p["popularity"] for p in products if not score[p["id"]]), default=0)
        r.check(lo > hi, "popularity: товар с заказами ниже товара без событий")


def digest(d):
    return {name: hashlib.sha256((d / name).read_bytes()).hexdigest() for name in FILES}


def check_determinism(d, r):
    gen = Path(__file__).with_name("gen_mock.py")
    with tempfile.TemporaryDirectory() as t1, tempfile.TemporaryDirectory() as t2:
        for t in (t1, t2):
            subprocess.run([sys.executable, str(gen), "--out", t], check=True, capture_output=True)
        h1, h2 = digest(Path(t1)), digest(Path(t2))
        r.check(h1 == h2, "determinism: два прогона генератора дали разные файлы")
        if d.exists():
            cur = digest(d)
            diff = [n for n in FILES if cur.get(n) != h1.get(n)]
            r.check(not diff, f"determinism: {d} отличается от свежей генерации: {diff}")
    return h1


def print_stats(data, queries, events, reform, fams):
    products = data["catalog.json"]["products"]
    print("\n## Состав")
    print("| Что | Сколько |\n| --- | --- |")
    rows = [("Семейств продуктов", len(fams)), ("Позиций (лицензий)", len(products)),
            ("Из них «цена по запросу»", sum(p["price_on_request"] for p in products)),
            ("Отечественных позиций (origin=ru)", sum(p["origin"] == "ru" for p in products)),
            ("Вендоров", len(data["vendors.json"])), ("Категорий (2 уровня)", len(data["categories.json"])),
            ("Связей семейств", len(data["relations.json"])), ("Алиасов в словаре", len(data["aliases.json"])),
            ("Клиентов", len(data["clients.json"])), ("Запросов", len(queries)),
            ("Кластеров запросов", len({q["cluster"] for q in queries})),
            ("Событий", len(events))]
    for k, v in rows:
        print(f"| {k} | {v} |")
    lt = Counter(p["license"]["type"] for p in products)
    print("\nТипы лицензий: " + ", ".join(f"{k} {v}" for k, v in lt.most_common()))
    rel = Counter(x["type"] for x in data["relations.json"])
    print("Связи: " + ", ".join(f"{k} {v}" for k, v in rel.most_common()))

    print("\n## Запросы: тип × набор (число запросов / сумма freq)")
    print("| Тип | " + " | ".join(SETS) + " | Всего | freq |\n| --- |" + " --- |" * (len(SETS) + 2))
    for t in QUERY_TYPES:
        qs = [q for q in queries if q["type"] == t]
        cells = [str(sum(q["set"] == s for q in qs)) for s in SETS]
        print(f"| {t} | " + " | ".join(cells) + f" | {len(qs)} | {sum(q['freq'] for q in qs)} |")
    tot = [str(sum(q["set"] == s for q in queries)) for s in SETS]
    print("| **всего** | " + " | ".join(tot) + f" | {len(queries)} | {sum(q['freq'] for q in queries)} |")
    cl = defaultdict(set)
    for q in queries:
        cl[q["set"]].add(q["cluster"])
    print("Кластеров по наборам: " + ", ".join(f"{s} {len(cl[s])}" for s in SETS))

    print("\n## События")
    et = Counter(e["type"] for e in events)
    print("| Тип | Сколько |\n| --- | --- |")
    for t in ("search", "click", "add_to_cart", "order", "price_request", "manager_request"):
        print(f"| {t} | {et[t]} |")
    searches = [e for e in events if e["type"] == "search"]
    zero = sum(e["results_count"] == 0 for e in searches)
    sess = defaultdict(list)
    for e in events:
        sess[e["session_id"]].append(e)
    with_order = sum(any(e["type"] == "order" for e in v) for v in sess.values())
    qtype = {q["text"]: q["type"] for q in queries}
    clicked_after = Counter()
    for v in sess.values():
        cur = None
        for e in v:
            if e["type"] == "search":
                cur = id(e)
            elif e["type"] == "click" and cur is not None:
                clicked_after[cur] = 1
    print("\n| Тип запроса | Поисков | Пустая выдача | С кликом |\n| --- | --- | --- | --- |")
    for t in QUERY_TYPES:
        ss = [e for e in searches if qtype[e["query"]] == t]
        if ss:
            z = sum(e["results_count"] == 0 for e in ss) / len(ss)
            c = sum(clicked_after[id(e)] for e in ss) / len(ss)
            print(f"| {t} | {len(ss)} | {z:.0%} | {c:.0%} |")
    print(f"\nСессий {len(sess)}; поисков с пустой выдачей {zero} ({zero / len(searches):.1%}); "
          f"переформулировок за <60 с без клика {reform}; сессий с заказом {with_order} "
          f"({with_order / len(sess):.1%}).")


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--dir", default=str(ROOT / "data" / "mock"))
    ap.add_argument("--stats", action="store_true")
    ap.add_argument("--determinism", action="store_true")
    args = ap.parse_args()
    d = Path(args.dir)
    r = Report()
    data = load(d, r)
    if r.errors:
        print("\n".join(r.errors))
        return 1
    vendors, cats = check_refs(data, r)
    by_id, fams = check_catalog(data, vendors, cats, r)
    clients = check_dicts(data, vendors, cats, fams, by_id, r)
    queries = check_queries(data, by_id, clients, vendors, cats, r)
    events, reform = check_events(data, queries, by_id, clients, r)
    check_popularity(data["catalog.json"]["products"], events, r)
    r.check(50 <= len(fams) <= 70, f"catalog: семейств {len(fams)} вне 50–70")
    r.check(180 <= len(by_id) <= 300, f"catalog: позиций {len(by_id)} вне 180–300")
    if args.determinism:
        check_determinism(d, r)
    for w in r.warnings:
        print(f"WARN  {w}")
    for e in r.errors[:50]:
        print(f"ERROR {e}")
    if len(r.errors) > 50:
        print(f"... и ещё {len(r.errors) - 50} ошибок")
    print(f"{d}: файлов {len(data)}, семейств {len(fams)}, позиций {len(by_id)}, запросов {len(queries)}, "
          f"событий {len(events)}; ошибок {len(r.errors)}, предупреждений {len(r.warnings)}"
          + ("; детерминизм проверен" if args.determinism else ""))
    if args.stats:
        print_stats(data, queries, events, reform, fams)
    return 1 if r.errors else 0


if __name__ == "__main__":
    sys.exit(main())
