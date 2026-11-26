#!/usr/bin/env python3
"""Генератор mock-данных PLEXeT SearchOps: каталог ПО, справочники, клиенты, наборы запросов, логи поведения.

Детерминирован: одинаковый SEED даёт побайтно одинаковые файлы. Только стандартная библиотека.

    python3.13 scripts/data/gen_mock.py            # пишет в data/mock/
    python3.13 scripts/data/gen_mock.py --out DIR  # в другой каталог (проверка детерминизма)

Данные контуров коммерции и выдачи — gen_mock_p2.py и gen_mock_p3.py. Описание данных — data/README.md.
"""

import argparse
import json
import math
import random
import re
import sys
from datetime import datetime, timedelta, timezone
from pathlib import Path

sys.dont_write_bytecode = True  # не оставлять __pycache__ рядом с данными

from defs_catalog import VENDORS, CATEGORIES, FAMILIES_1
from defs_catalog2 import FAMILIES_2
from defs_refs import ALIASES, RELATIONS, CLIENTS
from defs_queries import DEMO, EXACT, SKU_ITEMS, L
from defs_queries2 import LICENSE, ANALOG, PURPOSE, RENEWAL, ABSENT, GAP, NON_PRODUCT, NOISE
import gen_mock_p2
import gen_mock_p3
import gen_mock_p4

SEED = 0x1352905
LOG_START = datetime(2026, 4, 27, tzinfo=timezone.utc)  # 30 дней: 27.04–26.05.2026
LOG_DAYS = 30
SET_SHARES = (("tune", 0.40), ("test", 0.80), ("regression", 1.0))

ROOT = Path(__file__).resolve().parents[2]

# ------------------------------------------------------------------ русские формы
DEFAULT_UNITS = {
    "device": ("устройство", "устройства", "устройств"),
    "user": ("пользователь", "пользователя", "пользователей"),
    "server": ("сервер", "сервера", "серверов"),
    "core": ("ядро", "ядра", "ядер"),
}


def plural(n, forms):
    n10, n100 = n % 10, n % 100
    if n10 == 1 and n100 != 11:
        return forms[0]
    if 2 <= n10 <= 4 and not 12 <= n100 <= 14:
        return forms[1]
    return forms[2]


def duration(months):
    if months % 12 == 0:
        y = months // 12
        return f"{y} {plural(y, ('год', 'года', 'лет'))}"
    return f"{months} {plural(months, ('месяц', 'месяца', 'месяцев'))}"


def term_label(v):
    lt, mo = v["lt"], v["months"]
    if lt == "perpetual":
        return "бессрочная лицензия"
    if lt == "oem":
        return "OEM-версия"
    if lt == "upgrade":
        return "обновление, бессрочная лицензия"
    if lt == "renewal":
        return f"продление на {duration(mo)}" if mo else "продление"
    return duration(mo)


def license_phrase(v):
    lt, mo = v["lt"], v["months"]
    return {
        "perpetual": "бессрочная",
        "oem": "бессрочная OEM-лицензия",
        "upgrade": "бессрочная лицензия на обновление редакции",
    }.get(lt) or (f"продление подписки на {duration(mo)}" if lt == "renewal" else f"подписка на {duration(mo)}")


# ------------------------------------------------------------------ каталог
def gen_sku(vendor, pid, v, rng):
    letters = "ABCDEFGHJKLMNPRSTUVWXYZ"
    digits = "0123456789"
    pick = lambda alphabet, n: "".join(rng.choice(alphabet) for _ in range(n))  # noqa: E731
    qty = v["qty"] or 1
    if vendor == "kaspersky":
        return f"KL{rng.randint(1000, 9999)}R{pick(letters, 3)}{'E' if v['lt'] == 'renewal' else 'S'}"
    if vendor == "1c":
        return "4601546" + pick(digits, 6)
    if vendor == "drweb":
        return f"LHW-{'BK' if v['lt'] == 'renewal' else 'AK'}-{v['months'] or 12}M-{qty}-A{rng.randint(1, 4)}"
    if vendor == "eset":
        return f"NOD32-{pick(letters, 3)}-{rng.randint(1, 9)}{v['months'] or 12}-{qty}"
    if vendor == "microsoft":
        return f"{pick(letters + digits, 3)}-{pick(digits, 5)}"
    if vendor in ("basealt", "ptsecurity", "infowatch", "vmware", "postgrespro", "safib"):
        return ""  # у части позиций артикула нет — это нормально для магазина
    return f"{vendor[:3].upper()}-{pick(digits, 6)}"


def margin_for(vendor, origin, v, rng):
    base = {"microsoft": 0.08, "adobe": 0.12, "autodesk": 0.11, "1c": 0.30, "kaspersky": 0.27,
            "drweb": 0.31}.get(vendor, 0.22 if origin == "ru" else 0.14)
    if v["lt"] == "renewal":
        base -= 0.02
    if v["price"] is None:
        base += 0.05
    return round(min(0.45, max(0.03, base + rng.uniform(-0.03, 0.03))), 2)


def build_catalog():
    vendors = {vid: dict(id=vid, name=name, origin=origin) for vid, name, origin in VENDORS}
    cats = {cid: dict(id=cid, name=name, parent_id=parent) for cid, name, parent in CATEGORIES}
    products, meta, skus = [], {}, set()
    for f in FAMILIES_1 + FAMILIES_2:
        ven = vendors[f["vendor"]]
        for v in f["variants"]:
            o = v["o"]
            pid = f"{f['prefix']}-{v['code']}"
            rng = random.Random(f"{SEED}:product:{pid}")
            sku = o.get("sku")
            if sku is None:
                sku = gen_sku(f["vendor"], pid, v, rng)
                while sku and sku in skus:
                    sku = gen_sku(f["vendor"], pid, v, rng)
            if sku:
                assert sku not in skus, f"duplicate sku {sku}"
                skus.add(sku)
            units = f["unit"].get(v["metric"]) or DEFAULT_UNITS.get(v["metric"])
            if o.get("t"):
                title = o["t"]
            else:
                parts = [f["tn"] + (f" {v['ed']}" if v["ed"] else "")]
                if v["qty"] is not None:
                    parts.append(f"{v['qty']} {plural(v['qty'], units)}")
                parts.append(term_label(v))
                title = ", ".join(parts)
            edition = v["ed"] or f["ed"]
            lic = license_phrase(v)
            if v["qty"] is not None:
                lic += f"; {v['qty']} {plural(v['qty'], units)}"
            if o.get("u"):
                lic += "; продаётся поштучно, количество указывается при заказе"
            desc = " ".join(x for x in (f["desc"], o.get("d"), f"Лицензия: {lic}.") if x)
            attrs = dict(f["attrs"])
            attrs.update(o.get("a", {}))
            if edition:
                attrs["edition"] = edition
            if o.get("u"):
                attrs["purchase"] = "поштучно"
            reestr = f["reestr"] if f["reestr"] is not None else ven["origin"] == "ru"
            products.append({
                "id": pid,
                "sku": sku,
                "family_id": f["id"],
                "family_name": f["name"],
                "title": title,
                "vendor_id": f["vendor"],
                "category_id": o.get("cat", f["cat"]),
                "description": desc,
                "attributes": attrs,
                "license": {
                    "type": v["lt"],
                    "metric": v["metric"],
                    "quantity": v["qty"],
                    "duration_months": v["months"],
                    "edition": edition,
                },
                "price_rub": v["price"],
                "price_on_request": v["price"] is None,
                "origin": ven["origin"],
                "in_reestr": reestr,
                "popularity": 0.0,
                "margin": margin_for(f["vendor"], ven["origin"], v, rng),
            })
            meta[pid] = {"unit": bool(o.get("u"))}
    ids = [p["id"] for p in products]
    assert len(ids) == len(set(ids)), "duplicate product id"
    return list(vendors.values()), list(cats.values()), products, meta


# ------------------------------------------------------------------ разметка
def _as_list(x):
    return x if isinstance(x, list) else [x]


def rule_matches(p, f, cats, meta):
    def fam_ok(name):
        return p["family_id"] in _as_list(f[name])
    lic = p["license"]
    checks = {
        "fam": lambda: fam_ok("fam"),
        "ids": lambda: p["id"] in f["ids"],
        "not_ids": lambda: p["id"] not in f["not_ids"],
        "cat": lambda: bool({p["category_id"], cats[p["category_id"]]["parent_id"]} & set(_as_list(f["cat"]))),
        "lt": lambda: lic["type"] in f["lt"],
        "not_lt": lambda: lic["type"] not in f["not_lt"],
        "metric": lambda: lic["metric"] in _as_list(f["metric"]),
        "qty": lambda: lic["quantity"] in _as_list(f["qty"]),
        "mo": lambda: lic["duration_months"] == f["mo"],
        "ed": lambda: f["ed"].lower() in (lic["edition"] or "").lower(),
        "origin": lambda: p["origin"] == f["origin"],
        "reestr": lambda: p["in_reestr"] == f["reestr"],
        "unit": lambda: meta[p["id"]]["unit"] == f["unit"],
    }
    for key in f:
        if key not in checks:
            raise KeyError(f"unknown judgment filter {key}")
        if not checks[key]():
            return False
    return True


def resolve_judgments(rules, products, cats, meta, where):
    grades = {}
    for grade, f in rules:
        f = dict(f)
        optional = f.pop("opt", False)
        hit = [p["id"] for p in products if rule_matches(p, f, cats, meta)]
        if not hit and not optional:
            raise ValueError(f"{where}: rule {grade} {f} matches no product")
        for pid in hit:
            grades[pid] = max(grades.get(pid, 0), grade)
    order = {p["id"]: i for i, p in enumerate(products)}
    return {pid: g for pid, g in sorted(grades.items(), key=lambda kv: (-kv[1], order[kv[0]])) if g > 0}


TYPE_NOTES = {
    "exact": "точное название", "sku": "артикул", "typo": "опечатка", "layout": "раскладка клавиатуры",
    "slang": "сленг или транслит", "purpose": "запрос по назначению", "license": "условия лицензии",
    "analog": "аналог (substitute, не синоним)", "absent": "товара нет в каталоге",
    "alias_gap": "сленг, которого нет в aliases.json; товар есть", "non_product": "нетоварный запрос",
    "noise": "шум", "renewal": "продление",
}


def all_clusters(products):
    by_id = {p["id"]: p for p in products}
    sku_clusters = []
    for pid, items in SKU_ITEMS:
        q = []
        for it in items:
            text = it[0]
            if text.startswith("@sku:"):
                text = by_id[text[5:]]["sku"]
                assert text, f"{pid} has no sku"
            q.append(("sku", text, it[1], {"n": it[2]} if len(it) > 2 else {}))
        sku_clusters.append(dict(id=f"sku-{pid}", j=[(3, {"ids": [pid]}), (1, {"fam": by_id[pid]["family_id"]})], q=q))
    return DEMO + EXACT + sku_clusters + LICENSE + ANALOG + PURPOSE + RENEWAL + ABSENT + GAP + NON_PRODUCT + NOISE


def primary_type(cluster):
    counts = {}
    for q in cluster["q"]:
        counts[q[0]] = counts.get(q[0], 0) + 1
    return max(counts, key=lambda t: (counts[t], -[q[0] for q in cluster["q"]].index(t)))


def assign_sets(clusters):
    groups = {}
    for c in clusters:
        if c.get("set"):
            continue
        groups.setdefault(primary_type(c), []).append(c["id"])
    result = {c["id"]: c["set"] for c in clusters if c.get("set")}
    for t in sorted(groups):
        ids = sorted(groups[t])
        random.Random(f"{SEED}:sets:{t}").shuffle(ids)
        for i, cid in enumerate(ids):
            frac = (i + 0.5) / len(ids)
            result[cid] = next(name for name, edge in SET_SHARES if frac < edge)
    return result


def build_queries(products, cats, meta):
    clusters = all_clusters(products)
    cids = [c["id"] for c in clusters]
    assert len(cids) == len(set(cids)), "duplicate cluster id"
    sets = assign_sets(clusters)
    queries, reforms, n = [], {}, 0
    for c in clusters:
        for q in c["q"]:
            qtype, text, freq = q[0], q[1], q[2]
            opts = q[3] if len(q) > 3 else {}
            n += 1
            qid = f"q-{n:04d}"
            reforms[qid] = opts.get("r", c.get("reform"))
            notes = [TYPE_NOTES[qtype]] + [x for x in (c.get("note"), opts.get("n")) if x]
            queries.append({
                "id": qid,
                "text": text,
                "type": qtype,
                "set": sets[c["id"]],
                "freq": freq,
                "client_id": opts.get("c"),
                "judgments": resolve_judgments(opts.get("j", c["j"]), products, cats, meta, qid),
                "absent": bool(c.get("absent")),
                "notes": "; ".join(notes),
                "cluster": c["id"],
                "terms": c.get("terms", []),
            })
    for qid, target in reforms.items():
        assert target is None or target in cids, f"{qid}: unknown reform cluster {target}"
    return queries, reforms


# ------------------------------------------------------------------ имитация текущего поиска магазина
STOP = set("""на для и в во с со по от до или а к у из за о об не без
лицензия лицензии лицензию лицензий лицуха купить цена стоимость шт штук год года лет месяц месяцев
пк компьютер компьютера компьютеров компов компа устройство устройства устройств пользователь пользователя
пользователей рабочих рабочее мест места место ядер ядра ядро узлов узла узел подключений сотрудников серверов
бессрочно бессрочная бессрочную бессрочка подписка подписку подписки электронная версия core cores
аналог аналоги замена заменить альтернатива чем вместо""".split())

TOKEN_RE = re.compile(r"[0-9a-zа-я]+")


def tokenize(text):
    return TOKEN_RE.findall(text.lower().replace("ё", "е"))


class LegacySearch:
    """Реконструкция поиска магазина, с которого сняты логи: AND по словам названия, вендора, категории и
    атрибутов, словарь aliases.json, исправление раскладки (как на проде), без опечаток и семантики.
    Числа и единицы лицензий игнорируются — как на проде («касперский на 10 пк» → все Kaspersky); слова
    «аналог», «замена» тоже, поэтому «аналог фотошопа» показывает сам Photoshop."""

    def __init__(self, products, vendors, cats, aliases):
        vname = {v["id"]: v["name"] for v in vendors}
        self.products = products
        self.tokens = {}
        self.title_tokens = {}
        self.skus = {p["sku"].lower(): p["id"] for p in products if p["sku"]}
        for p in products:
            cat = cats[p["category_id"]]
            parent = cats.get(cat["parent_id"]) if cat["parent_id"] else None
            text = " ".join([p["title"], p["family_name"], vname[p["vendor_id"]], cat["name"],
                             parent["name"] if parent else "", p["sku"], *map(str, p["attributes"].values())])
            self.tokens[p["id"]] = set(tokenize(text))
            self.title_tokens[p["id"]] = set(tokenize(p["title"]))
        self.aliases = sorted(((tokenize(a["alias"]), tokenize(a["canonical"])) for a in aliases),
                              key=lambda x: -len(x[0]))

    @staticmethod
    def _tok_ok(q, pset):
        if q in pset:
            return True
        return len(q) >= 6 and any(p.startswith(q[:-2]) for p in pset)

    def _slots(self, toks):
        slots, i = [], 0
        while i < len(toks):
            for alias, canon in self.aliases:
                if alias and toks[i:i + len(alias)] == alias:
                    slots.append([alias, canon])
                    i += len(alias)
                    break
            else:
                slots.append([[toks[i]]])
                i += 1
        clean = []
        for alts in slots:
            alts = [[t for t in alt if t not in STOP and not t.isdigit()] for alt in alts]
            alts = [a for a in alts if a]
            if alts:
                clean.append(alts)
        return clean

    def _run(self, text):
        if text.strip().lower() in self.skus:
            return [self.skus[text.strip().lower()]]
        slots = self._slots(tokenize(text))
        if not slots:
            return []
        hits = []
        for p in self.products:
            pset = self.tokens[p["id"]]
            if all(any(all(self._tok_ok(t, pset) for t in alt) for alt in alts) for alts in slots):
                tset = self.title_tokens[p["id"]]
                in_title = sum(any(all(self._tok_ok(t, tset) for t in alt) for alt in alts) for alts in slots)
                hits.append((-in_title, len(p["title"]), p["id"]))
        return [pid for _, _, pid in sorted(hits)]

    def search(self, text):
        res = self._run(text)
        if not res:
            swapped = L(text.lower())
            if swapped != text.lower():
                res = self._run(swapped)
        return res


# ------------------------------------------------------------------ логи поведения
P_REFORM = {"alias_gap": 0.6, "typo": 0.5, "layout": 0.4, "absent": 0.35, "purpose": 0.35, "analog": 0.35,
            "license": 0.3, "renewal": 0.3, "slang": 0.3, "exact": 0.25, "sku": 0.2, "non_product": 0.25, "noise": 0.05}
P_CLICK = {"exact": 0.72, "sku": 0.85, "slang": 0.62, "layout": 0.6, "typo": 0.55, "license": 0.55,
           "purpose": 0.45, "analog": 0.45, "renewal": 0.62, "alias_gap": 0.5, "absent": 0.2}
P_CART = {"license": 0.45, "renewal": 0.55, "sku": 0.5}
PRIORITY = {"alias_gap": 0, "typo": 1, "layout": 1, "absent": 2, "analog": 3, "purpose": 3}
CLIENT_MIX = [(None, 72), ("client-a", 6), ("client-b", 5), ("client-c", 5), ("client-d", 4), ("client-e", 4),
              ("client-f", 4)]
CLIENT_MIX_RENEWAL = [(None, 40), ("client-a", 15), ("client-c", 10), ("client-d", 10), ("client-e", 15),
                      ("client-f", 10)]


def weighted(rng, items):
    total = sum(w for _, w in items)
    x = rng.random() * total
    for item, w in items:
        x -= w
        if x < 0:
            return item
    return items[-1][0]


def build_events(queries, reforms, products, legacy, clients):
    rng = random.Random(f"{SEED}:events")
    by_id = {p["id"]: p for p in products}
    owned = {c["id"]: {by_id[pid]["family_id"] for pid, _ in c["purchases"]} for c in clients}
    info = {}
    for q in queries:
        res = legacy.search(q["text"])
        pos = {pid: i + 1 for i, pid in enumerate(res)}
        good = [(pid, g, pos[pid]) for pid, g in q["judgments"].items() if g >= 2 and pid in pos]
        info[q["id"]] = {"res": res, "good": good, "fail": not good}
    by_cluster = {}
    for q in queries:
        by_cluster.setdefault(q["cluster"], []).append(q)
    budget = {q["id"]: q["freq"] for q in queries}

    def pick_next(cur, chain):
        used = {x["id"] for x in chain}
        target = reforms[cur["id"]]
        pools = ([by_cluster[target]] if target else []) + [by_cluster[cur["cluster"]]]
        for pool in pools:
            cand = [(x, budget[x["id"]]) for x in pool if x["id"] not in used and budget[x["id"]] > 0
                    and x["client_id"] in (None, chain[0]["client_id"])]
            if cand:
                return weighted(rng, cand)
        return None

    sessions = []
    for q in sorted(queries, key=lambda x: (PRIORITY.get(x["type"], 4), x["id"])):
        while budget[q["id"]] > 0:
            budget[q["id"]] -= 1
            chain, cur = [q], q
            while len(chain) < 3 and info[cur["id"]]["fail"] and rng.random() < P_REFORM[cur["type"]]:
                nxt = pick_next(cur, chain)
                if nxt is None:
                    break
                budget[nxt["id"]] -= 1
                chain.append(nxt)
                cur = nxt
            sessions.append(chain)

    day_w = [(d, 1.0 if (LOG_START + timedelta(days=d)).weekday() < 5 else 0.35) for d in range(LOG_DAYS)]
    hour_w = [(h, w) for h, w in zip(range(5, 18), (1, 4, 7, 9, 9, 6, 7, 9, 8, 7, 5, 3, 1))]  # UTC; МСК = +3
    events = []
    for chain in sessions:
        first = chain[0]
        if first["client_id"]:
            client = first["client_id"]
        else:
            client = weighted(rng, CLIENT_MIX_RENEWAL if first["type"] == "renewal" else CLIENT_MIX)
        t = LOG_START + timedelta(days=weighted(rng, day_w), hours=weighted(rng, hour_w),
                                  seconds=rng.randrange(3600))
        sid = f"s-{rng.getrandbits(48):012x}"
        base = {"session_id": sid, "client_id": client}

        def emit(kind, **kw):
            events.append({"ts": t.strftime("%Y-%m-%dT%H:%M:%SZ"), **base, "type": kind, **kw})

        for i, q in enumerate(chain):
            inf = info[q["id"]]
            emit("search", query=q["text"], results_count=len(inf["res"]))
            if not inf["fail"]:
                p = P_CLICK.get(q["type"], 0.0)
                if min(x[2] for x in inf["good"]) > 10:
                    p *= 0.5
                if rng.random() < p:
                    cand = [((pid, ps), (3 if g == 3 else 1) * (5 if client and by_id[pid]["family_id"] in owned[client]
                                                                 else 1) / (1 + 0.15 * ps))
                            for pid, g, ps in inf["good"]]
                    pid, ps = weighted(rng, cand)
                    t += timedelta(seconds=rng.randint(4, 40))
                    emit("click", product_id=pid, position=ps)
                    if by_id[pid]["price_on_request"]:
                        if rng.random() < 0.35:
                            t += timedelta(seconds=rng.randint(20, 200))
                            emit("price_request", product_id=pid)
                    elif rng.random() < P_CART.get(q["type"], 0.35):
                        t += timedelta(seconds=rng.randint(15, 180))
                        emit("add_to_cart", product_id=pid)
                        if client != "client-b" and rng.random() < 0.55:
                            t += timedelta(seconds=rng.randint(60, 900))
                            emit("order", product_id=pid)
            elif i == len(chain) - 1 and ((q["type"] == "absent" and rng.random() < 0.12)
                                           or (q["type"] == "alias_gap" and rng.random() < 0.05)):
                t += timedelta(seconds=rng.randint(10, 60))
                emit("manager_request", query=q["text"])
            if i < len(chain) - 1:
                t += timedelta(seconds=rng.randint(6, 50))
    events.sort(key=lambda e: (e["ts"], e["session_id"]))
    return events


def apply_popularity(products, events):
    weight = {"order": 5, "add_to_cart": 2, "price_request": 2, "click": 1}
    score = {}
    for e in events:
        if e["type"] in weight:
            score[e["product_id"]] = score.get(e["product_id"], 0) + weight[e["type"]]
    top = max(score.values())
    for p in products:
        s = score.get(p["id"], 0)
        p["popularity"] = round(0.02 + 0.98 * math.log1p(s) / math.log1p(top), 3) if s else 0.01


# ------------------------------------------------------------------ запись
def write_json(path, data):
    path.write_text(json.dumps(data, ensure_ascii=False, indent=2) + "\n", encoding="utf-8")


def main():
    ap = argparse.ArgumentParser(description=__doc__.splitlines()[0])
    ap.add_argument("--out", default=str(ROOT / "data" / "mock"))
    args = ap.parse_args()
    out = Path(args.out)
    out.mkdir(parents=True, exist_ok=True)

    vendors, cats_list, products, meta = build_catalog()
    cats = {c["id"]: c for c in cats_list}
    aliases = [dict(alias=a, canonical=c, kind=k, target=t, target_id=tid) for a, c, k, t, tid in ALIASES]
    relations = [dict(from_family=a, to_family=b, type=t, note=n) for a, b, t, n in RELATIONS]
    clients = [dict(id=cid, name=name, segment=seg, purchases=[dict(product_id=p, date=d) for p, d in pur])
               for cid, name, seg, pur in CLIENTS]
    queries, reforms = build_queries(products, cats, meta)
    legacy = LegacySearch(products, vendors, cats, aliases)
    events = build_events(queries, reforms, products, legacy, [
        {"id": c["id"], "purchases": [(x["product_id"], x["date"]) for x in c["purchases"]]} for c in clients])
    apply_popularity(products, events)
    # Контур коммерции дописывается поверх готового контура поиска (данные контура поиска не меняются, см. gen_mock_p2.py).
    meta2, relations, extra, events2, planted = gen_mock_p2.extend(
        SEED, LOG_START, LOG_DAYS, weighted, products, vendors, cats_list, relations, clients, queries, events,
        aliases, LegacySearch)
    # Контур выдачи: набор префиксов панели и события намерений — отдельным блоком в конце журнала (gen_mock_p3.py).
    prefixes, events3 = gen_mock_p3.extend(SEED, LOG_START, LOG_DAYS, products, queries, events + events2)
    # Контур доводки: варианты под тип покупки и организации, реестр, справка, набор витрины — в конец (gen_mock_p4.py).
    relations, extra4 = gen_mock_p4.extend(products, vendors, relations, aliases, gen_mock_p2._rel)
    extra.update(extra4)

    write_json(out / "catalog.json", {**meta2, "products": products})
    write_json(out / "vendors.json", vendors)
    write_json(out / "categories.json", cats_list)
    write_json(out / "relations.json", relations)
    write_json(out / "aliases.json", aliases)
    write_json(out / "clients.json", clients)
    write_json(out / "queries.json", queries)
    for name, data in extra.items():
        write_json(out / name, data)
    (out / "PLANTED.md").write_text(planted, encoding="utf-8")
    write_json(out / "prefixes.json", prefixes)
    with open(out / "events.jsonl", "w", encoding="utf-8") as fh:
        for e in events + events2 + events3:
            fh.write(json.dumps(e, ensure_ascii=False, separators=(",", ":")) + "\n")
    events = events + events2

    fams = {p["family_id"] for p in products}
    searches = sum(e["type"] == "search" for e in events)
    zero = sum(e["type"] == "search" and e["results_count"] == 0 for e in events)
    print(f"out: {out}")
    print(f"families {len(fams)}, products {len(products)}, vendors {len(vendors)}, categories {len(cats_list)}, "
          f"relations {len(relations)}, aliases {len(aliases)}, clients {len(clients)}")
    print(f"queries {len(queries)}, events {len(events)} (search {searches}, zero-result {zero / searches:.1%})")


if __name__ == "__main__":
    main()
