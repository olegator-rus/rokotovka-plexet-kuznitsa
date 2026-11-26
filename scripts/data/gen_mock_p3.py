"""Контур выдачи поверх готовых контуров поиска и коммерции: набор префиксов панели и события поисковых намерений.

Вызывается из gen_mock.py последним. Данные контуров поиска и коммерции не меняются: события намерений дописываются в конец
events.jsonl отдельным отсортированным по времени блоком (сессии s3-), поисков (type=search) в блоке нет — журнал
поисков, отложенная неделя агента и наборы оценки остаются прежними.

prefixes.json — что человек набирает в строке и чего ждёт:
  * цели из ПОЗДНЕГО периода журнала (отложенная неделя агента: последние 7 дней по МСК до последнего поиска) —
    успешные поиски с кликом; индекс подсказок строится на раннем периоде, проверка — на позднем;
  * артикулы и парт-номера каталога, семейства с дублями лицензий;
  * префиксы запросов, которые в позднем периоде дали пустую выдачу.
"""

import random
import re
from collections import Counter, defaultdict
from datetime import datetime, timedelta, timezone

SESSION_PREFIX = "s3-"
INTENT_PREFIX = "it-"
MSK = timezone(timedelta(hours=3))
HOLDOUT_DAYS = 7
ATTRIBUTION = timedelta(minutes=30)

N_TARGETS = 60       # целей из позднего периода
N_LAYOUT = 15
N_TYPO = 15
N_SKU = 20
N_EMPTY = 15
N_DUPES = 12
N_INTENTS = 360
OUTCOMES = (("commit", 34), ("suggestion", 24), ("product", 18), ("category", 7), ("vendor", 5), ("abandon", 12))

EN = "qwertyuiop[]asdfghjkl;'zxcvbnm,."
RU = "йцукенгшщзхъфывапролджэячсмитьбю"
TO_RU = dict(zip(EN, RU))
TO_EN = dict(zip(RU, EN))


def fold(text):
    return re.sub(r"\s+", " ", text.lower().replace("ё", "е")).strip()


def switch_layout(text):
    """Набрано не в той раскладке: кириллица → латинские клавиши и наоборот (по первой букве)."""
    for ch in text:
        if ch in TO_EN:
            return "".join(TO_EN.get(c, c) for c in text)
        if ch in TO_RU:
            return "".join(TO_RU.get(c, c) for c in text)
    return None


def ts(e):
    return datetime.strptime(e["ts"], "%Y-%m-%dT%H:%M:%SZ").replace(tzinfo=timezone.utc)


def holdout_window(events):
    """Отложенная неделя агента — как services.holdoutRangeAt: от последнего поиска журнала, полночь по МСК."""
    last = max(ts(e) for e in events if e["type"] == "search").astimezone(MSK)
    to = datetime(last.year, last.month, last.day, tzinfo=MSK) + timedelta(days=1)
    return (to - timedelta(days=HOLDOUT_DAYS)).astimezone(timezone.utc), to.astimezone(timezone.utc)


def late_targets(events, start, end):
    """Успешные поиски позднего периода: поиск с результатами и кликом той же сессии до следующего поиска."""
    sessions = defaultdict(list)
    for e in events:
        sessions[e["session_id"]].append(e)
    hits, clicks, empty = Counter(), defaultdict(Counter), Counter()
    for evs in sessions.values():
        cur, hit = None, False
        for e in evs:
            if e["type"] == "search":
                cur, hit = (e if start <= ts(e) < end else None), False
                if cur is not None and cur["results_count"] == 0:
                    empty[fold(cur["query"])] += 1
            elif e["type"] == "click" and cur is not None and ts(e) - ts(cur) <= ATTRIBUTION:
                q = fold(cur["query"])
                if not hit:
                    hits[q] += 1
                    hit = True
                clicks[q][e["product_id"]] += 1
    return hits, clicks, empty


def word_prefix(text, extra):
    """Первое слово целиком плюс extra символов следующего (или первое слово, обрезанное до ≥4 символов)."""
    words = text.split(" ")
    if len(words) > 1 and extra > 0:
        return words[0] + " " + words[1][:extra]
    return words[0][:max(4, len(words[0]) - 1)] if len(words[0]) > 4 else words[0]


def typo(word, rng):
    """Перестановка соседних букв с 3-го символа: «kaspersky» → «kaspresky»."""
    i = rng.randrange(2, len(word) - 1)
    return word[:i] + word[i + 1] + word[i] + word[i + 2:]


def build_prefixes(events, queries, products, rng):
    start, end = holdout_window(events)
    hits, clicks, empty = late_targets(events, start, end)
    by_id = {p["id"]: p for p in products}
    qtype = {fold(q["text"]): q["type"] for q in queries}
    items, seen = [], set()

    def add(prefix, target, typ, family=None, product=None, source="log_late", freq=None):
        key = (prefix, typ, target)
        if not prefix or key in seen:
            return
        seen.add(key)
        it = dict(id="", prefix=prefix, target_query=target, target_family=family, target_product=product, type=typ,
                  set="panel", source=source)
        if freq is not None:
            it["freq"] = freq
        items.append(it)

    targets = sorted(hits.items(), key=lambda kv: (-kv[1], kv[0]))[:N_TARGETS]
    broad = {}
    for q, n in targets:
        pid = sorted(clicks[q].items(), key=lambda kv: (-kv[1], kv[0]))[0][0]
        fam = by_id[pid]["family_id"]
        typ = "sku" if qtype.get(q) in ("sku", "p2_mpn") else "family"
        add(word_prefix(q, 2) if typ == "family" else q[:max(5, len(q) * 3 // 5)], q, typ, fam, pid, freq=n)
        b = q[:3]
        if len(b) == 3 and " " not in b and b not in broad:
            broad[b] = (q, fam, pid, n)
    for b, (q, fam, pid, n) in sorted(broad.items(), key=lambda kv: (-kv[1][3], kv[0])):
        add(b, q, "broad", fam, pid, freq=n)
    layouts = 0
    for q, n in targets:
        p = word_prefix(q, 0)
        sw = switch_layout(p)
        if layouts < N_LAYOUT and sw and len(p) >= 4 and qtype.get(q) not in ("sku", "p2_mpn"):
            pid = sorted(clicks[q].items(), key=lambda kv: (-kv[1], kv[0]))[0][0]
            add(sw, q, "layout", by_id[pid]["family_id"], pid, freq=n)
            layouts += 1
    typos = 0
    for q, n in targets:
        words = q.split(" ")
        if typos < N_TYPO and len(words) > 1 and len(words[0]) >= 5 and words[0].isalpha():
            pid = sorted(clicks[q].items(), key=lambda kv: (-kv[1], kv[0]))[0][0]
            add(typo(words[0], rng) + " " + words[1][:2], q, "typo", by_id[pid]["family_id"], pid, freq=n)
            typos += 1

    coded = [p for p in products if p.get("mpn") or p.get("sku")]
    coded.sort(key=lambda p: (-p["popularity"], p["id"]))
    for p in coded[:N_SKU]:
        code = p.get("mpn") or p["sku"]
        add(code, code, "sku", p["family_id"], p["id"], source="catalog")
        add(code[:max(5, len(code) * 3 // 5)], code, "sku", p["family_id"], p["id"], source="catalog")

    for q, n in sorted(empty.items(), key=lambda kv: (-kv[1], kv[0]))[:N_EMPTY]:
        add(q[:max(4, len(q) * 7 // 10)], q, "no_results", source="log_empty", freq=n)

    variants = Counter(p["family_id"] for p in products)
    names = {p["family_id"]: p["family_name"] for p in products}
    for fam, n in sorted(variants.items(), key=lambda kv: (-kv[1], kv[0]))[:N_DUPES]:
        name = fold(names[fam])
        add(word_prefix(name, 2), name, "license_dupes", fam, None, source="catalog")

    for i, it in enumerate(items):
        it["id"] = f"px-{i + 1:04d}"
    return items, (start, end)


def weighted_pick(rng, pairs):
    total = sum(w for _, w in pairs)
    x = rng.uniform(0, total)
    for v, w in pairs:
        x -= w
        if x <= 0:
            return v
    return pairs[-1][0]


def build_intent_events(seed, log_start, log_days, queries, products, events):
    """Намерения: фокус → показы панели → исход → действия с товаром. Все с intent_id."""
    rng = random.Random(f"{seed}:p3-events")
    by_id = {p["id"]: p for p in products}
    results = {}
    for e in events:
        if e["type"] == "search":
            results.setdefault(fold(e["query"]), e["results_count"])
    pool = [q for q in queries if q["type"] not in ("noise", "non_product") and fold(q["text"]) in results]
    pool.sort(key=lambda q: q["id"])
    weights = [(q, q["freq"]) for q in pool]
    with_product = [(q, w) for q, w in weights if q["judgments"]]
    out = []
    for n in range(N_INTENTS):
        outcome = weighted_pick(rng, OUTCOMES)
        q = weighted_pick(rng, with_product if outcome in ("product", "category", "vendor") else weights)
        text = fold(q["text"])
        nres = results[text]
        best = sorted(q["judgments"].items(), key=lambda kv: (-kv[1], kv[0]))
        pid = best[0][0] if best else None
        sess = f"{SESSION_PREFIX}{rng.getrandbits(48):012x}"
        intent = f"{INTENT_PREFIX}{rng.getrandbits(48):012x}"
        client = rng.choice([None, None, None, "client-a"]) if pid and outcome != "abandon" else None
        t = log_start + timedelta(seconds=rng.randrange(log_days * 86400 - 3600))
        typed = min(len(text), rng.randint(2, 8))
        lines = []

        def emit(kind, dt, product=None, position=None, **payload):
            nonlocal t
            t += timedelta(seconds=dt)
            e = dict(ts=t.strftime("%Y-%m-%dT%H:%M:%SZ"), session_id=sess, client_id=client, type=kind,
                     intent_id=intent)
            if product:
                e["product_id"] = product
            if position is not None:
                e["position"] = position
            if payload:
                e["payload"] = payload
            lines.append(e)

        emit("search_opened", 0, state="empty")
        shown = 0 if nres == 0 else rng.randint(3, 8)
        sections = [] if shown == 0 else ["suggestions", "products"]
        emit("suggest_impression", rng.randint(1, 4), prefix_len=typed, sections=sections, items=shown)
        act = None
        if outcome == "commit":
            emit("search_commit", rng.randint(1, 6), source="enter", query=q["text"], results_count=nres,
                 prefix_len=len(text))
            act = nres > 0
        elif outcome == "suggestion":
            if shown == 0:
                emit("search_commit", rng.randint(1, 6), source="enter", query=q["text"], results_count=nres,
                     prefix_len=len(text))
            else:
                emit("suggestion_select", rng.randint(1, 4), position=rng.randint(1, 3), prefix_len=typed)
                emit("search_commit", 0, source="suggestion", query=q["text"], results_count=nres, prefix_len=typed)
            act = nres > 0
        elif outcome == "product":
            fam = by_id[pid]["family_id"]
            emit("instant_product_click", rng.randint(1, 4), product=pid, position=rng.randint(1, 4), family_id=fam,
                 prefix_len=typed)
            if rng.random() < 0.3:
                emit("add_to_cart", rng.randint(20, 120), product=pid)
        elif outcome in ("category", "vendor"):
            p = by_id[pid]
            key = "category_id" if outcome == "category" else "vendor_id"
            emit(f"{outcome}_select", rng.randint(1, 4), value=p[key], prefix_len=typed)
            if rng.random() < 0.35:
                emit("click", rng.randint(5, 40), product=pid, position=rng.randint(1, 6))
        else:
            emit("search_abandon", rng.randint(2, 15), prefix_len=typed)
        if act and pid and rng.random() < 0.55:
            emit("click", rng.randint(4, 40), product=pid, position=rng.randint(1, 5))
            if rng.random() < 0.4:
                emit("add_to_cart", rng.randint(20, 120), product=pid)
        out += lines
    out.sort(key=lambda e: (e["ts"], e["session_id"]))
    end = log_start + timedelta(days=log_days)
    return [e for e in out if ts(e) < end]


def extend(seed, log_start, log_days, products, queries, events):
    """Контур выдачи: (prefixes, p3_events). events — события контуров поиска и коммерции (не меняются)."""
    prefixes, _ = build_prefixes(events, queries, products, random.Random(f"{seed}:p3-prefixes"))
    p3_events = build_intent_events(seed, log_start, log_days, queries, products, events)
    return prefixes, p3_events
