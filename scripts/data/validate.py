#!/usr/bin/env python3
"""Проверка mock-данных по формату данных (контур коммерции) и §11 (контур выдачи).

    python3.13 scripts/data/validate.py                  # проверить data/mock
    python3.13 scripts/data/validate.py --stats          # + таблицы для data/README.md
    python3.13 scripts/data/validate.py --determinism    # + два прогона генератора побайтно равны data/mock

Код возврата 0 — ошибок нет (предупреждения печатаются, но не валят проверку).
"""

import argparse
import hashlib
import json
import math
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
P2_FILES = ["license_rules.json", "compatibility_rules.json", "attribute_definitions.json", "companion_pairs.json"]
P3_FILES = ["prefixes.json"]
P4_FILES = ["help.json", "reestr.json", "showcase.json"]
DIGEST_FILES = FILES + P2_FILES + ["PLANTED.md"] + P3_FILES + P4_FILES
# Заморозка контура поиска: хэш списка (id, text, set, judgments) запросов вне p2 и первых строк events.jsonl.
P1_QUERIES_SHA = "36ad647ff56c61c3088d3fe17ea05ea464a1d095d67e2f35a5e8b093ef07de13"
P1_EVENTS_SHA = "f2608dff36743b08848ac1a917e1c3c1fdd0ef8b38127e1e63d74f57487e7b7a"
P1_EVENTS_LINES = 5121
P2_SESSION = "s2-"
# Контур выдачи (§11.1, §11.6): события намерений — третьим блоком в конце журнала; строки контуров поиска и коммерции заморожены.
P12_EVENTS_SHA = "3081fd9db1e671fa5dd8d81b1c4100d70f64ed300f0fee2033ca0a1f05a64344"
P12_EVENTS_LINES = 6403
P3_SESSION = "s3-"
P3_EVENT_TYPES = {"search_opened", "suggest_impression", "suggestion_select", "instant_product_click", "category_select",
                  "vendor_select", "refinement_apply", "refinement_remove", "search_commit", "search_abandon",
                  "variant_expand", "filter_apply", "filter_remove", "filters_reset", "sort_change", "group_change",
                  "recs_impression", "recs_click", "recs_add", "personalization_toggle", "history_clear"}
P3_OUTCOMES = {"search_commit": "commit", "suggestion_select": "suggestion", "instant_product_click": "product",
               "category_select": "category", "vendor_select": "vendor", "search_abandon": "abandon"}
PREFIX_TYPES = {"broad", "family", "sku", "layout", "typo", "no_results", "license_dupes"}
PREFIX_MIN = {"broad": 20, "family": 40, "sku": 30, "layout": 10, "typo": 10, "no_results": 10, "license_dupes": 10}
P2_QUERY_TYPES = {"p2_license": 20, "p2_mpn": 20, "p2_specs": 20, "p2_compat": 25, "p2_bundle": 15, "p2_negative": 20}
PAIR_CLASSES = ["required", "compatible", "conditional", "incompatible", "unknown", "complement", "upgrade",
                "irrelevant"]
RELATION_TYPES = {"substitute", "complement", "renewal", "compatibility", "upgrade"}
REQUIREMENTS = {"conditional_required", "recommended", "optional"}
COMPAT_STATES = {"verified", "conditional", "incompatible", "unknown"}
QTY_STRATEGIES = {"per_user", "per_device", "per_server", "per_core", "per_seat", "one", "none"}
SOURCES = {"vendor_documentation", "catalog_rule", "expert", "co_purchase", "agent_proposal"}
PRICE_MODES = {"fixed", "from", "quote_required", "customer_specific", "unavailable"}
SPEC_KEYS = {"ram_gb", "storage_gb", "screen_inch", "ports_count", "refresh_hz", "power_w", "net_gbps",
             "form_factor_u", "cpu_cores"}
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
    for name in FILES + P2_FILES + P3_FILES + P4_FILES:
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
        if p.get("kind") == "hardware" and lic.get("type") is None:
            r.check(lic.get("metric") is None and lic.get("quantity") is None and lic.get("duration_months") is None,
                    f"{where}: у оборудования без лицензии заполнены поля license")
            lic = None
        if lic is None:
            pass
        elif not r.check(lic.get("type") in LICENSE_TYPES, f"{where}: license.type {lic.get('type')}"):
            pass
        else:
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
        mode = p.get("price_mode", "quote_required" if por else "fixed")
        r.check(mode in PRICE_MODES, f"{where}: price_mode {mode}")
        r.check(bool(por) == (mode == "quote_required"), f"{where}: price_on_request не согласован с price_mode")
        if mode in ("fixed", "from", "quote_required"):
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
        key = (rel.get("from_family"), rel.get("to_family"), rel.get("type"), rel.get("from_product"),
               rel.get("to_product"))
        r.check(key not in seen, f"relation {key}: повтор")
        seen.add(key)
        r.check(rel.get("from_family") in fams and rel.get("to_family") in fams, f"relation {key}: нет семейства")
        if not (rel.get("from_product") or rel.get("to_product")):
            r.check(rel.get("from_family") != rel.get("to_family"), f"relation {key}: связь на себя")
        r.check(rel.get("type") in RELATION_TYPES, f"relation {key}: type")
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
    all_queries = data["queries.json"]
    ids = Counter(q.get("id") for q in all_queries)
    r.check(all(n == 1 for n in ids.values()), "queries: повтор id")
    cluster_sets = defaultdict(set)
    for q in all_queries:
        cluster_sets[q.get("cluster")].add(q.get("set"))
    queries = [q for q in all_queries if q.get("set") != "p2"]
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
    pooled = {"wscal25-u1", "wscal25-d1", "wscal25-u5", "wscal25-d5", "rdscal25-u1", "rdscal25-d1"}  # gen_mock_p2.POOLED_P2
    sha = hashlib.sha256(json.dumps([(q["id"], q["text"], q["set"], {k: v for k, v in q["judgments"].items()
                                                                     if k not in pooled}) for q in queries],
                                    ensure_ascii=False, sort_keys=True).encode()).hexdigest()
    r.check(sha == P1_QUERIES_SHA, "queries: запросы контура поиска (id, text, set, judgments) изменились — test заморожен")
    return all_queries


def check_events(data, queries, by_id, clients, r):
    events = data["events.jsonl"]
    r.check(3000 <= len(events) <= 9000, f"events: {len(events)} вне 3–9 тыс.")
    end = LOG_START + timedelta(days=LOG_DAYS)
    prev_ts, days, block = "", set(), 1
    for i, e in enumerate(events):
        where = f"event #{i + 1}"
        sid = str(e.get("session_id", ""))
        pkg = 3 if sid.startswith(P3_SESSION) else 2 if sid.startswith(P2_SESSION) else 1
        want = 1 if i < P1_EVENTS_LINES else 2 if i < P12_EVENTS_LINES else 3
        r.check(pkg == want, f"{where}: событие пакета {pkg} не в своём блоке")
        p2 = pkg == 2
        if pkg != block:
            block, prev_ts = pkg, ""  # блоки контуров коммерции и 3 дописаны в конец и отсортированы отдельно
        try:
            ts = datetime.strptime(e.get("ts", ""), "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc)
        except ValueError:
            r.err(f"{where}: ts {e.get('ts')}")
            continue
        r.check(LOG_START <= ts < end, f"{where}: ts вне окна логов")
        r.check(e["ts"] >= prev_ts, f"{where}: события не отсортированы по времени")
        prev_ts = e["ts"]
        days.add(ts.date())
        allowed = EVENT_TYPES - {"search"} | P3_EVENT_TYPES if pkg == 3 else EVENT_TYPES
        r.check(e.get("type") in allowed, f"{where}: type {e.get('type')}")
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
            if p2:
                r.check(isinstance(e.get("order_id"), str) and isinstance(e.get("account_id"), str),
                        f"{where}: заказ контура коммерции без order_id/account_id")
                r.check(e.get("client_id") in (None, e.get("account_id")), f"{where}: client_id ≠ account_id")
                r.check(isinstance(e.get("quantity"), int) and e["quantity"] >= 1, f"{where}: quantity")
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


def check_popularity(all_products, all_events, r):
    for part in (False, True):
        products = [p for p in all_products if ("kind" in p) == part]
        events = [e for e in all_events if e["session_id"].startswith(P2_SESSION) == part
                  and not e["session_id"].startswith(P3_SESSION)]  # намерения контура выдачи popularity не меняют
        _check_popularity(products, events, r)


def _check_popularity(products, events, r):
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


# ------------------------------------------------------------------ контур коммерции
def fisher_greater(a, b, c, d):
    n1, k, n = a + b, a + c, a + b + c + d
    return sum(math.comb(k, x) * math.comb(n - k, n1 - x) for x in range(a, min(n1, k) + 1)) / math.comb(n, n1)


def _version_ok(rule_value, value, anchor_version):
    if rule_value.startswith(">=anchor.version"):
        return value is not None and anchor_version is not None and str(value) >= str(anchor_version)
    return str(value) in rule_value.split("|")


def check_p2(d, data, by_id, fams, cats, vendors, queries, events, r):
    products = list(by_id.values())
    p2 = [p for p in products if "kind" in p]
    hw = [p for p in p2 if p["kind"] == "hardware"]
    cat = data["catalog.json"]
    r.check(cat.get("version") == "mock-2", "catalog.version не mock-2")
    r.check(cat.get("synthetic") == ["margin", "orders"], "catalog.synthetic")
    r.check(70 <= len(hw) <= 90, f"catalog: оборудования {len(hw)} вне 70–90")
    raw = (d / "events.jsonl").read_bytes().split(b"\n")
    r.check(hashlib.sha256(b"\n".join(raw[:P1_EVENTS_LINES]) + b"\n").hexdigest() == P1_EVENTS_SHA,
            "events: строки контура поиска изменились")

    # реестр атрибутов
    adefs = {}
    for x in data["attribute_definitions.json"]:
        r.check(x.get("category_id") in cats, f"attribute_definitions: нет категории {x.get('category_id')}")
        keys = [a["key"] for a in x.get("attrs", [])]
        r.check(len(keys) == len(set(keys)), f"attribute_definitions {x.get('category_id')}: повтор ключа")
        for a in x.get("attrs", []):
            r.check(a.get("type") in ("number", "enum", "bool"), f"attribute_definitions {a.get('key')}: type")
            r.check(isinstance(a.get("label"), str) and a["label"], f"attribute_definitions {a.get('key')}: label")
            r.check(isinstance(a.get("unit_aliases"), list), f"attribute_definitions {a.get('key')}: unit_aliases")
            if a.get("type") == "number":
                r.check(a["key"] in SPEC_KEYS, f"attribute_definitions {a['key']}: числовой ключ вне specs")
        adefs[x.get("category_id")] = {a["key"]: a for a in x.get("attrs", [])}
    all_attr_keys = {k for v in adefs.values() for k in v}

    # позиции контура коммерции
    models, mpns = set(), Counter()
    for p in p2:
        where = f"product {p['id']}"
        r.check(p["kind"] in ("software", "hardware"), f"{where}: kind")
        r.check(isinstance(p.get("mpn"), str), f"{where}: mpn")
        r.check(isinstance(p.get("specs"), dict) and isinstance(p.get("compatible_with"), list), f"{where}: specs")
        if p.get("model_id"):
            models.add(p["model_id"])
        if p["kind"] == "software":
            r.check(p["mpn"] == "", f"{where}: у ПО mpn не пустой")
            continue
        r.check(bool(re.fullmatch(r"[A-Z0-9][A-Z0-9/_-]{3,}", p["mpn"])) and re.search(r"\d", p["mpn"])
                and re.search(r"[A-Z]", p["mpn"]), f"{where}: неправдоподобный mpn {p['mpn']}")
        mpns[p["mpn"]] += 1
        defs = adefs.get(p["category_id"])
        r.check(defs is not None, f"{where}: нет attribute_definitions для {p['category_id']}")
        for k, v in p["specs"].items():
            r.check(k in SPEC_KEYS and isinstance(v, (int, float)) and not isinstance(v, bool),
                    f"{where}: specs.{k} не число из списка")
            r.check(defs is None or k in defs, f"{where}: specs.{k} нет в attribute_definitions {p['category_id']}")
        if p["category_id"] in ("server-systems", "laptops"):
            r.check(bool(p.get("model_id")), f"{where}: у системы нет model_id")
    r.check(all(n == 1 for n in mpns.values()), "catalog: повтор mpn")
    for p in p2:
        for m in p["compatible_with"]:
            r.check(m in models, f"product {p['id']}: compatible_with {m} — нет такой модели")
    r.check(any(p.get("price_mode") == "quote_required" for p in hw), "catalog: нет оборудования quote_required")
    for p in products:
        if p.get("family_id") in ("windows-server-2022", "windows-server-2025", "windows-server-cal",
                                  "windows-server-cal-2025", "rds-cal-2025", "sql-server-2022"):
            r.check(p["attributes"].get("version") in ("2022", "2025"), f"product {p['id']}: нет attributes.version")

    # связи
    rels = data["relations.json"]
    rel_ids = [x.get("id") for x in rels]
    r.check(all(rel_ids) and len(rel_ids) == len(set(rel_ids)), "relations: id пустые или повторяются")
    for x in rels:
        w = f"relation {x.get('id')}"
        r.check(x.get("requirement", "optional") in REQUIREMENTS, f"{w}: requirement")
        r.check(x.get("compatibility", "unknown") in COMPAT_STATES, f"{w}: compatibility")
        r.check(x.get("quantity_strategy", "none") in QTY_STRATEGIES, f"{w}: quantity_strategy")
        r.check(x.get("source", "catalog_rule") in SOURCES, f"{w}: source")
        r.check(re.fullmatch(r"E[0-5]", x.get("evidence_level", "E0")) is not None, f"{w}: evidence_level")
        r.check(x.get("review_status", "approved") in ("approved", "pending", "rejected"), f"{w}: review_status")
        for side in ("from", "to"):
            pid = x.get(f"{side}_product")
            if pid:
                r.check(by_id.get(pid, {}).get("family_id") == x[f"{side}_family"], f"{w}: {side}_product не из семьи")
        if x.get("type") == "upgrade":
            r.check(bool(x.get("upgrade_adds")) and x.get("from_product") and x.get("to_product"),
                    f"{w}: upgrade без upgrade_adds или позиций")
    r.check(any(x["type"] == "upgrade" for x in rels), "relations: нет upgrade")

    # правила лицензирования
    lrules = {x["id"]: x for x in data["license_rules.json"]}
    for x in lrules.values():
        w = f"license_rule {x['id']}"
        r.check(x.get("vendor_id") in vendors and x.get("anchor_family") in fams, f"{w}: вендор/семейство")
        r.check(x.get("requires_category") in cats, f"{w}: requires_category")
        r.check(not x.get("requires_family") or x["requires_family"] in fams, f"{w}: requires_family")
        r.check(not x.get("anchor_product") or x["anchor_product"] in by_id, f"{w}: anchor_product")
        r.check(x.get("class") in ("conditional_required", "recommended", "optional"), f"{w}: class")
        r.check(x.get("quantity_strategy") in QTY_STRATEGIES, f"{w}: quantity_strategy")
        r.check(x.get("source") in SOURCES and x.get("reason"), f"{w}: source/reason")
        anchor_v = next((p["attributes"].get("version") for p in products if p["family_id"] == x["anchor_family"]),
                        None)
        targets = [p for p in products if p["category_id"] == x["requires_category"]
                   and (not x.get("requires_family") or p["family_id"] == x["requires_family"])
                   and all(_version_ok(v, p["attributes"].get(k if k != "edition" else "edition")
                                       or p["license"].get(k), anchor_v)
                           for k, v in x.get("requires_attrs", {}).items())]
        r.check(bool(targets), f"{w}: ни одна позиция не удовлетворяет правилу")

    # совместимость
    crules = data["compatibility_rules.json"]
    states = Counter()
    seen = set()
    for x in crules:
        w = f"compatibility_rule {x.get('id')}"
        r.check(x.get("product_id") in by_id or x.get("family_id") in fams, f"{w}: позиция/семейство")
        r.check(x.get("target_model") in models, f"{w}: target_model {x.get('target_model')}")
        r.check(x.get("state") in ("verified", "conditional", "incompatible"), f"{w}: state")
        r.check(x.get("state") != "conditional" or x.get("condition"), f"{w}: conditional без условия")
        r.check(x.get("source") in SOURCES and isinstance(x.get("rule_version"), int) and x.get("date"), f"{w}: поля")
        key = (x.get("product_id") or x.get("family_id"), x.get("target_model"))
        r.check(key not in seen, f"{w}: повтор пары")
        seen.add(key)
        states[x.get("state")] += 1
    r.check(all(states[s_] for s_ in ("verified", "conditional", "incompatible")), "compatibility_rules: не все состояния")
    cstate = {(x["product_id"], x["target_model"]): x["state"] for x in crules if x.get("product_id")}
    comps = [p for p in hw if p["category_id"] in ("server-memory", "server-storage")]
    r.check(any((p["id"], m) not in cstate for p in comps for m in models), "compatibility: нет пар unknown")

    # набор p2
    p2q = [q for q in queries if q.get("set") == "p2"]
    types = Counter(q["type"] for q in p2q)
    r.check(dict(types) == P2_QUERY_TYPES, f"queries p2: состав {dict(types)}")
    p1_texts = {q["text"].lower() for q in queries if q.get("set") != "p2"}
    texts = Counter(q["text"].lower() for q in p2q)
    for q in p2q:
        w = f"query {q['id']} «{q['text']}»"
        r.check(q["text"].lower() not in p1_texts, f"{w}: текст совпадает с запросом контура поиска")
        r.check(texts[q["text"].lower()] == 1, f"{w}: повтор текста в p2")
        r.check(isinstance(q.get("expect"), dict), f"{w}: нет expect")
        for pid, g in q["judgments"].items():
            r.check(pid in by_id and isinstance(g, int) and 1 <= g <= 3, f"{w}: разметка {pid}={g}")
        top = max(q["judgments"].values(), default=0)
        if q["type"] in ("p2_license", "p2_mpn", "p2_specs"):
            r.check(top == 3, f"{w}: нет товара с оценкой 3")
        if q["type"] == "p2_bundle":
            r.check(top >= 2, f"{w}: нет подходящего товара")
        ex = q.get("expect", {})
        for pid in ex.get("incompatible", []) + ex.get("unknown", []):
            r.check(pid in by_id and pid not in q["judgments"], f"{w}: {pid} в incompatible/unknown размечен")
        for f in ex.get("filters", []):
            r.check(f.get("key") in all_attr_keys | {"price_rub", "cpu_cores"}, f"{w}: фильтр {f.get('key')}")
            r.check(f.get("op") in ("eq", "gte", "lte", "range", "not"), f"{w}: op {f.get('op')}")
        tgt = ex.get("compat_target")
        if q["type"] == "p2_compat":
            r.check(tgt in models or tgt in fams, f"{w}: compat_target {tgt}")
            r.check(bool(q["judgments"]) or ex.get("incompatible") or ex.get("unknown") or ex.get("filters"),
                    f"{w}: пустая разметка без объяснения")
            if tgt in models:
                for pid in q["judgments"]:
                    st = cstate.get((pid, tgt), "unknown")
                    r.check(st in ("verified", "conditional"), f"{w}: размечен {pid} с совместимостью {st}")
        if ex.get("mpn"):
            r.check(any(p.get("mpn") == ex["mpn"] and q["judgments"].get(p["id"]) == 3 for p in p2),
                    f"{w}: mpn {ex['mpn']} не размечен 3")
        for req in ex.get("required", []):
            r.check(req.get("family_id") in fams or req.get("product_id") in by_id, f"{w}: required {req}")
    for p in products:
        if p.get("price_mode") == "unavailable":
            r.check(all(q["judgments"].get(p["id"], 0) <= 1 for q in p2q), f"product {p['id']}: снят, но размечен > 1")

    # пары «якорь → кандидат»
    pairs = data["companion_pairs.json"]
    r.check(130 <= len(pairs) <= 170, f"companion_pairs: {len(pairs)} вне 130–170")
    pc = Counter(x.get("class") for x in pairs)
    r.check(set(pc) == set(PAIR_CLASSES), f"companion_pairs: классы {dict(pc)}")
    for x in pairs:
        w = f"pair {x.get('anchor_product')} → {x.get('candidate_product')}"
        a, c = by_id.get(x.get("anchor_product")), by_id.get(x.get("candidate_product"))
        if not r.check(a is not None and c is not None, f"{w}: нет позиции"):
            continue
        if a.get("model_id") and a["category_id"] == "server-systems" and x["class"] in (
                "compatible", "conditional", "incompatible", "unknown"):
            st = cstate.get((c["id"], a["model_id"]), "unknown")
            want = {"compatible": "verified"}.get(x["class"], x["class"])
            r.check(st == want, f"{w}: класс {x['class']}, а правило {st}")
        if x["class"] == "required" and a["family_id"] in ("windows-server-2022", "windows-server-2025"):
            r.check(c["attributes"].get("version", "0") >= a["attributes"].get("version", "9"),
                    f"{w}: required CAL старше сервера")

    # заказы с аккаунтами и пара без связи
    fam_of = {p["id"]: p["family_id"] for p in products}
    baskets, acc = defaultdict(set), {}
    for e in events:
        if e["type"] == "order" and e.get("account_id"):
            baskets[e["order_id"]].add(fam_of[e["product_id"]])
            acc[e["order_id"]] = e["account_id"]
    single, pair, pacc = Counter(), Counter(), defaultdict(set)
    for oid, fs in baskets.items():
        fs = sorted(fs)
        single.update(fs)
        for i in range(len(fs)):
            for j in range(i + 1, len(fs)):
                pair[(fs[i], fs[j])] += 1
                pacc[(fs[i], fs[j])].add(acc[oid])
    n = len(baskets)
    r.check(any(len(fs) >= 2 for fs in baskets.values()), "events: нет многопозиционных заказов")
    nb = defaultdict(set)
    for x in rels:
        nb[x["from_family"]].add(x["to_family"])
        nb[x["to_family"]].add(x["from_family"])
    tested = {k: a for k, a in pair.items() if len(pacc[k]) >= 3}
    bonf = 0.01 / max(1, len(tested))
    unexpl = sorted((fisher_greater(a, single[k[0]] - a, single[k[1]] - a, n - single[k[0]] - single[k[1]] + a), k)
                    for k, a in tested.items() if k[1] not in nb[k[0]] and not (nb[k[0]] & nb[k[1]]))
    unexpl = [(pv, k) for pv, k in unexpl if pv < bonf]
    planted = ("dell-poweredge-r750", "ippon-innova-rt-ii")
    r.check(bool(unexpl) and unexpl[0][1] == planted, f"events: заложенная пара не первая среди значимых: {unexpl[:3]}")
    stray = [k for _, k in unexpl if not set(k) & {"ippon-innova-rt-ii"}]
    r.check(not stray, f"events: лишние значимые пары без связи: {stray}")

    # заложенное для агента
    rel_key = {(x["from_family"], x["to_family"], x["type"]) for x in rels}
    r.check(("windows-server-2025", "windows-server-cal", "complement") in rel_key, "P2-01: нет сломанной связи")
    r.check("lr-ws25-rds" in lrules and ("windows-server-2025", "rds-cal-2025", "complement") not in rel_key,
            "P2-02: пропущенное обязательное не заложено")
    planted_md = (d / "PLANTED.md")
    r.check(planted_md.exists() and all(f"P2-0{i}" in planted_md.read_text(encoding="utf-8") for i in range(1, 5)),
            "PLANTED.md: нет списка P2-01…P2-04")
    return dict(hw=len(hw), p2=len(p2), rules=len(lrules), crules=len(crules), pairs=len(pairs), orders=n,
                p2_queries=len(p2q))


# ------------------------------------------------------------------ контур выдачи (§11.1, §11.6)
def check_p3(d, data, by_id, queries, events, r):
    sys.path.insert(0, str(Path(__file__).parent))
    import gen_mock_p3 as g3
    raw = (d / "events.jsonl").read_bytes().split(b"\n")
    r.check(hashlib.sha256(b"\n".join(raw[:P12_EVENTS_LINES]) + b"\n").hexdigest() == P12_EVENTS_SHA,
            "events: строки контуров поиска и коммерции изменились")
    old = events[:P12_EVENTS_LINES]
    start, end = g3.holdout_window(old)
    hits, _, empty = g3.late_targets(old, start, end)

    # набор префиксов
    px = data["prefixes.json"]
    ids = [x.get("id") for x in px]
    r.check(all(ids) and len(ids) == len(set(ids)), "prefixes: id пустые или повторяются")
    fams = {p["family_id"] for p in by_id.values()}
    for x in px:
        w = f"prefix {x.get('id')}"
        r.check(x.get("type") in PREFIX_TYPES and x.get("set") == "panel", f"{w}: type/set")
        r.check(isinstance(x.get("prefix"), str) and x["prefix"].strip() != "", f"{w}: пустой prefix")
        r.check(isinstance(x.get("target_query"), str) and x["target_query"], f"{w}: target_query")
        fam, pid = x.get("target_family"), x.get("target_product")
        r.check(fam is None or fam in fams, f"{w}: target_family {fam}")
        r.check(pid is None or (pid in by_id and by_id[pid]["family_id"] == fam), f"{w}: target_product {pid}")
        if x.get("type") == "no_results":
            r.check(fam is None and pid is None, f"{w}: у no_results есть цель-товар")
        if x.get("source") == "log_late":
            r.check(hits[g3.fold(x["target_query"])] > 0, f"{w}: цель не из успешных поисков позднего периода")
        if x.get("source") == "log_empty":
            r.check(empty[g3.fold(x["target_query"])] > 0, f"{w}: цель не из пустых выдач позднего периода")
    types = Counter(x.get("type") for x in px)
    for t, n in PREFIX_MIN.items():
        r.check(types[t] >= n, f"prefixes: {t} — {types[t]} < {n}")

    # события намерений
    ev3 = events[P12_EVENTS_LINES:]
    intents = defaultdict(list)
    for e in ev3:
        r.check(isinstance(e.get("intent_id"), str) and e["intent_id"].startswith(g3.INTENT_PREFIX),
                f"event {e.get('ts')}: нет intent_id")
        r.check(e["type"] != "search", "events контура выдачи: поиск в блоке намерений (журнал поисков не меняется)")
        r.check(e.get("payload") is None or isinstance(e["payload"], dict), "events контура выдачи: payload не объект")
        if e["type"] in ("instant_product_click",):
            r.check(e.get("product_id") in by_id, f"event {e['ts']}: instant_product_click без товара")
        if e["type"] == "search_commit":
            p = e.get("payload") or {}
            r.check(isinstance(p.get("results_count"), int) and p.get("source") in ("enter", "suggestion"),
                    f"event {e['ts']}: search_commit без results_count/source")
        intents[e.get("intent_id")].append(e)
    outcomes = Counter()
    for iid, evs in intents.items():
        r.check(len({e["session_id"] for e in evs}) == 1, f"intent {iid}: в разных сессиях")
        r.check(evs[0]["type"] == "search_opened", f"intent {iid}: начинается не с search_opened")
        first = next((P3_OUTCOMES[e["type"]] for e in evs if e["type"] in P3_OUTCOMES), None)
        r.check(first is not None, f"intent {iid}: нет исхода")
        outcomes[first] += 1
    r.check(set(outcomes) == set(P3_OUTCOMES.values()), f"events контура выдачи: исходы {dict(outcomes)} — не все виды")
    r.check(any(e["type"] == "suggest_impression" and (e.get("payload") or {}).get("items") == 0 for e in ev3),
            "events контура выдачи: нет пустых подсказок")
    return dict(prefixes=len(px), intents=len(intents), types=types, outcomes=outcomes,
                window=(start.isoformat(), end.isoformat()))


# ------------------------------------------------------------------ контур доводки
def check_p4(data, by_id, fams, r):
    """Справка, реестр, набор витрины; варианты под сделку и организацию — только у ПО контура доводки."""
    for h in data["help.json"]:
        r.check(h.get("id") and h.get("title") and h.get("text") and h.get("url", "").startswith("/help"),
                f"help {h.get('id')}: id, title, text, url /help…")
        r.check(isinstance(h.get("triggers"), list) and h["triggers"], f"help {h.get('id')}: триггеры")
    ids = [h.get("id") for h in data["help.json"]]
    r.check(len(ids) == len(set(ids)), "help: повтор id")
    for need in ("invoice", "documents", "delivery", "return", "renewal", "activation"):
        r.check(need in ids, f"help: нет статьи {need}")
    seen = set()
    for e in data["reestr.json"]:
        f = e.get("family_id")
        r.check(f in fams and f not in seen, f"reestr {f}: нет семейства или повтор")
        seen.add(f)
        r.check(e.get("status") in ("confirmed", "not_matching"), f"reestr {f}: статус")
        if e.get("status") == "confirmed":
            r.check(bool(e.get("number")), f"reestr {f}: подтверждено без номера")
            origin = next((p["origin"] for p in by_id.values() if p["family_id"] == f), None)
            r.check(origin == "ru", f"reestr {f}: зарубежное ПО подтверждено в реестре")
    sc = data["showcase.json"]
    r.check(len(sc) == 31, f"showcase: {len(sc)} запросов, нужен 31")
    for q in sc:
        r.check(q.get("expect") in ("product", "analogs", "absent", "help"), f"showcase {q.get('id')}: класс")
        r.check(q.get("vitrine") in ("ok", "partial", "fail", "unknown"), f"showcase {q.get('id')}: оценка витрины")
    for p in by_id.values():
        a = p["attributes"]
        if "deal" in a or "org_type" in a:
            r.check(p.get("kind") == "software", f"product {p['id']}: deal/org_type вне позиций контура доводки")
            r.check(a.get("deal", "renewal") in ("renewal", "extension", "migration"), f"product {p['id']}: deal")
            r.check(a.get("org_type", "education") in ("education", "government"), f"product {p['id']}: org_type")


def digest(d):
    return {name: hashlib.sha256((d / name).read_bytes()).hexdigest() for name in DIGEST_FILES}


def check_determinism(d, r):
    gen = Path(__file__).with_name("gen_mock.py")
    with tempfile.TemporaryDirectory() as t1, tempfile.TemporaryDirectory() as t2:
        for t in (t1, t2):
            subprocess.run([sys.executable, str(gen), "--out", t], check=True, capture_output=True)
        h1, h2 = digest(Path(t1)), digest(Path(t2))
        r.check(h1 == h2, "determinism: два прогона генератора дали разные файлы")
        if d.exists():
            cur = digest(d)
            diff = [n for n in DIGEST_FILES if cur.get(n) != h1.get(n)]
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
    print_p2_stats(data, queries, events)


def print_p2_stats(data, queries, events):
    products = data["catalog.json"]["products"]
    p2 = [p for p in products if "kind" in p]
    print("\n## Контур коммерции")
    print("| Категория | Позиций | Из них quote_required |\n| --- | --- | --- |")
    for c, n in Counter(p["category_id"] for p in p2).most_common():
        q = sum(p["category_id"] == c and p["price_mode"] == "quote_required" for p in p2)
        print(f"| {c} | {n} | {q} |")
    print("price_mode: " + ", ".join(f"{k} {v}" for k, v in Counter(p["price_mode"] for p in p2).most_common()))
    print("compatibility_rules: " + ", ".join(
        f"{k} {v}" for k, v in Counter(x["state"] for x in data["compatibility_rules.json"]).most_common()))
    print(f"license_rules {len(data['license_rules.json'])}, "
          f"attribute_definitions {len(data['attribute_definitions.json'])} категорий")
    print("companion_pairs: " + ", ".join(
        f"{k} {v}" for k, v in Counter(x["class"] for x in data["companion_pairs.json"]).most_common())
          + f" (всего {len(data['companion_pairs.json'])})")
    p2q = [q for q in queries if q["set"] == "p2"]
    print("\n| Тип p2 | Запросов | freq | Пустая разметка |\n| --- | --- | --- | --- |")
    for t in P2_QUERY_TYPES:
        qs = [q for q in p2q if q["type"] == t]
        print(f"| {t} | {len(qs)} | {sum(q['freq'] for q in qs)} | {sum(not q['judgments'] for q in qs)} |")
    ev2 = [e for e in events if e["session_id"].startswith(P2_SESSION)]
    orders = defaultdict(set)
    for e in ev2:
        if e["type"] == "order":
            orders[e["order_id"]].add(e["account_id"])
    lines = Counter(e["order_id"] for e in ev2 if e["type"] == "order")
    print("\nСобытия контура коммерции: " + ", ".join(f"{k} {v}" for k, v in Counter(e["type"] for e in ev2).most_common())
          + f"; заказов {len(orders)}, многопозиционных {sum(n >= 2 for n in lines.values())}, "
          f"аккаунтов {len({a for v in orders.values() for a in v})}")


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
    p1 = [p for p in by_id.values() if "kind" not in p]
    p1_fams = {p["family_id"] for p in p1}
    r.check(50 <= len(p1_fams) <= 70, f"catalog: семейств контура поиска {len(p1_fams)} вне 50–70")
    r.check(180 <= len(p1) <= 300, f"catalog: позиций контура поиска {len(p1)} вне 180–300")
    check_p2(d, data, by_id, fams, cats, vendors, queries, events, r)
    p3 = check_p3(d, data, by_id, queries, events, r)
    check_p4(data, by_id, fams, r)
    if args.determinism:
        check_determinism(d, r)
    for w in r.warnings:
        print(f"WARN  {w}")
    for e in r.errors[:50]:
        print(f"ERROR {e}")
    if len(r.errors) > 50:
        print(f"... и ещё {len(r.errors) - 50} ошибок")
    print(f"{d}: файлов {len(data)}, семейств {len(fams)}, позиций {len(by_id)}, запросов {len(queries)}, "
          f"событий {len(events)}, префиксов {p3['prefixes']}, намерений {p3['intents']}; ошибок {len(r.errors)}, предупреждений {len(r.warnings)}"
          + ("; детерминизм проверен" if args.determinism else ""))
    if args.stats:
        print_stats(data, queries, events, reform, fams)
    return 1 if r.errors else 0


if __name__ == "__main__":
    sys.exit(main())
