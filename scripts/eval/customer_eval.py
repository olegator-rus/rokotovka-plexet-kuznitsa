#!/usr/bin/env python3
"""Оценка по эталонным запросам заказчика через живой POST /api/search (контур фида).

Эталоны — data/softline/eval/customer-queries.jsonl (10 строк с target_url) и 7 примеров из ТЗ без цели
(только счётчики). Первая цель ответа: поле target, иначе первый товар выдачи — URL семейства,
оффера или страницы вендора. URL сравниваются после нормализации (normalize_url). Запросы идут с
source=compare: в журнал пишутся, в аналитику и стоимость покупателя не входят. Запросы — по одному, с паузой;
на 429 — повтор с ожиданием.

Конфигурации (поле config тела запроса, backend/in/search.go → services.validateSearch):
  live — текущая сборка: витрина, конвейер B3 на последней принятой версии правил (в трассе effective_config=B4);
  B1   — «BM25 без улучшений», ближайшая из имеющихся: multi_match best_fields по мультиполям + нормализация и
         словари версии 1 (раскладка, опечатки, алиасы); без векторов, атрибутов, намерения, порога, бизнес-слоя,
         ослаблений и LLM. Чистого BM25 без нормализатора в API нет;
  B0   — подстрока запроса в названии (реконструкция исходного поиска магазина); B2 — B1 + векторы + RRF;
  B3, B4 — лестница оценки (B4 — с принятыми правками агента).

Запуск из корня репозитория (API по умолчанию http://127.0.0.1:8088, для стенда — API=https://…). Скрипт
печатает версию каталога из /api/meta и из трасс и предупреждает, если каталог mock.

  # точка отсчёта на фиде: текущая сборка и BM25 без улучшений в одном отчёте
  python3 scripts/eval/customer_eval.py --configs live,B1 --out data/softline/eval/results/baseline-feed
  # то же по отдельности
  python3 scripts/eval/customer_eval.py --configs live --out data/softline/eval/results/baseline-feed-live
  python3 scripts/eval/customer_eval.py --configs B1 --out data/softline/eval/results/baseline-feed-b1
  # пробный прогон на mock-каталоге (отчёт уже в data/softline/eval/results/)
  python3 scripts/eval/customer_eval.py --configs live,B1 --out data/softline/eval/results/baseline-mock-2

На live маршрут D при холодном кэше разборов идёт в LLM (секунды); повторный прогон попадает в кэш, и p95 падает.
Для сравнения «до/после» запускать в одинаковом состоянии кэша.

  # юнит-тесты нормализации URL
  cd scripts/eval && python3 -m unittest
"""
import argparse
import json
import math
import os
import re
import sys
import time
import urllib.error
import urllib.request
from collections import Counter
from datetime import datetime
from urllib.parse import parse_qsl, quote, unquote, urlsplit

ROOT = os.path.dirname(os.path.dirname(os.path.dirname(os.path.abspath(__file__))))
API = os.environ.get("API", "http://127.0.0.1:8088").rstrip("/")
ORIGIN = os.environ.get("ORIGIN", "http://localhost:5177")
QUERIES = os.path.join(ROOT, "data", "softline", "eval", "customer-queries.jsonl")

STORE = "https://store.softline.ru"
STORE_HOST = "store.softline.ru"

# Примеры из ТЗ без эталонной цели: по ним только счётчики.
TZ_QUERIES = ["сервер", "антивирь", "офис", "винда", "астра линукс", "мдаемон", "Astra Linux"]

DEFAULT_CONFIGS = "live,B1"
CONFIG_NOTES = {
    "live": "текущая сборка: витрина, конвейер B3 на последней принятой версии правил",
    "B0": "подстрока в названии (реконструкция исходного поиска магазина)",
    "B1": "BM25 без улучшений (ближайшая): multi_match по мультиполям + нормализация и словари v1",
    "B2": "B1 + векторы + RRF",
    "B3": "B2 + атрибуты, намерение, порог, бизнес-слой",
    "B4": "B3 + принятые правки агента",
}

NO_URL = "нет URL"
RETRY_429 = 5
RETRY_OTHER = 2


# --- нормализация URL -------------------------------------------------------------------------------------

_INDEX = re.compile(r"\[\d+\]")
_PATH_SAFE = "/:@!$&'()*+,;=-._~"


def normalize_url(url):
    """Каноническая форма URL магазина для сравнения с эталоном.

    Одно и то же: относительный и абсолютный URL (схема и хост https://store.softline.ru), регистр хоста,
    http и https у магазина, завершающий слэш, URL-кодирование (%5B%5D = []), filter[x][]= и filter[x][0]=,
    порядок параметров. Фрагмент отбрасывается. Пустое — пустая строка.
    """
    if url is None:
        return ""
    u = str(url).strip()
    if not u:
        return ""
    if u.startswith("//"):
        u = "https:" + u
    elif not re.match(r"^[a-zA-Z][a-zA-Z0-9+.-]*://", u):
        if u.lower().startswith(STORE_HOST):
            u = "https://" + u
        else:
            u = STORE + ("" if u.startswith("/") else "/") + u
    parts = urlsplit(u)
    scheme = parts.scheme.lower()
    host = (parts.hostname or "").rstrip(".")
    port = parts.port
    if host == STORE_HOST or host == "www." + STORE_HOST:
        scheme, host, port = "https", STORE_HOST, None
    if port and not ((scheme == "https" and port == 443) or (scheme == "http" and port == 80)):
        host = f"{host}:{port}"
    path = quote(unquote(parts.path), safe=_PATH_SAFE).rstrip("/") or "/"
    params = []
    for k, v in parse_qsl(parts.query, keep_blank_values=True):
        params.append((_INDEX.sub("[]", k), v))
    params.sort()
    query = "&".join(f"{quote(k, safe='[]')}={quote(v, safe='')}" for k, v in params)
    return f"{scheme}://{host}{path}" + (f"?{query}" if query else "")


def same_url(a, b):
    na, nb = normalize_url(a), normalize_url(b)
    return bool(na) and na == nb


# --- первая цель ответа ---------------------------------------------------------------------------------

def _first(d, *keys):
    for k in keys:
        v = d.get(k) if isinstance(d, dict) else None
        if isinstance(v, str) and v.strip():
            return v.strip()
    return ""


def first_target(resp):
    """Первая цель ответа: (url, тип, источник). url пустой — URL в ответе нет.

    1) поле target; 2) первый товар: URL семейства → URL оффера → страница вендора.
    """
    t = resp.get("target")
    if isinstance(t, dict) and (t.get("url") or t.get("type")):
        return _first(t, "url"), t.get("type") or "", "target"
    items = resp.get("items") or []
    if not items:
        return "", "", "пусто"
    it = items[0]
    url = _first(it, "family_url")
    if url:
        return url, "family", "items[0].family_url"
    url = _first(it, "offer_url", "product_url", "url")
    if url:
        return url, "sku", "items[0].offer_url"
    url = _first(it, "vendor_url")
    if url:
        return url, "vendor", "items[0].vendor_url"
    return "", "", "items[0] без URL"


def _sku(it):
    return it.get("offer_id") or it.get("product_id") or ""


def counts(resp):
    """Счётчики ответа: карточки, разные семейства, разные SKU (карточки и их варианты лицензии), match_type."""
    items = resp.get("items") or []
    fams, skus = set(), set()
    mt, groups = Counter(), Counter()
    for it in items:
        if it.get("family_id"):
            fams.add(it["family_id"])
        if _sku(it):
            skus.add(_sku(it))
        for v in it.get("variants") or []:
            if _sku(v):
                skus.add(_sku(v))
        if it.get("match_type"):
            mt[it["match_type"]] += 1
        if it.get("group"):
            groups[it["group"]] += 1
    return {"cards": len(items), "families": len(fams), "skus": len(skus),
            "match_type": dict(mt), "group": dict(groups), "server_counts": resp.get("counts")}


def miss_reason(matched, etalon, resp):
    """Причина промаха для гейта ссылка (нужное семейство первое, URL не тот или нет), ранг N, цель."""
    if matched:
        return ""
    gid = etalon.get("group_id")
    if gid:
        for i, it in enumerate(resp.get("items") or [], 1):
            if str(it.get("family_id")) == str(gid):
                return "ссылка" if i == 1 else f"ранг {i}"
    return "цель"


# --- HTTP ------------------------------------------------------------------------------------------------

def _post(path, body, timeout=40):
    req = urllib.request.Request(API + path, data=json.dumps(body).encode(), method="POST",
                                 headers={"Content-Type": "application/json", "Origin": ORIGIN})
    with urllib.request.urlopen(req, timeout=timeout) as r:
        return json.load(r)


def search(query, config, log=print):
    """POST /api/search с повтором: 429 — до RETRY_429 раз с растущей паузой, сбой связи и 5xx — до RETRY_OTHER."""
    body = {"query": query, "source": "compare", "config": config}
    tries_429 = tries_other = 0
    while True:
        t = time.monotonic()
        try:
            d = _post("/api/search", body)
            d["_ms"] = round((time.monotonic() - t) * 1000)
            d["_http"] = 200
            return d
        except urllib.error.HTTPError as e:
            ms = round((time.monotonic() - t) * 1000)
            text = e.read().decode(errors="replace")[:300]
            if e.code == 429 and tries_429 < RETRY_429:
                tries_429 += 1
                wait = _retry_after(e) or 5 * 2 ** (tries_429 - 1)
                log(f"  429 на «{query}» — жду {wait} с (повтор {tries_429}/{RETRY_429})")
                time.sleep(wait)
                continue
            if e.code >= 500 and tries_other < RETRY_OTHER:
                tries_other += 1
                time.sleep(3)
                continue
            return {"_http": e.code, "_error": text, "_ms": ms}
        except Exception as e:  # noqa: BLE001 — сбой связи пишется в отчёт, прогон идёт дальше
            ms = round((time.monotonic() - t) * 1000)
            if tries_other < RETRY_OTHER:
                tries_other += 1
                time.sleep(3)
                continue
            return {"_http": 0, "_error": str(e)[:300], "_ms": ms}


def _retry_after(e):
    try:
        return max(1, int(e.headers.get("Retry-After", "")))
    except (TypeError, ValueError):
        return 0


def meta():
    req = urllib.request.Request(API + "/api/meta", headers={"Origin": ORIGIN})
    try:
        with urllib.request.urlopen(req, timeout=10) as r:
            return json.load(r)
    except Exception:  # noqa: BLE001
        return {}


# --- прогон -----------------------------------------------------------------------------------------------

def load_queries(path):
    rows = []
    with open(path, encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if line:
                rows.append({**json.loads(line), "kind": "эталон"})
    for q in TZ_QUERIES:
        rows.append({"query": q, "kind": "ТЗ"})
    return rows


def run_one(row, config, log=print):
    d = search(row["query"], config, log)
    tr = d.get("trace") or {}
    top0 = (d.get("items") or [{}])[0]
    url, ttype, src = first_target(d) if d.get("_http") == 200 else ("", "", "ошибка")
    matched = bool(row.get("target_url")) and same_url(url, row["target_url"])
    res = {
        "query": row["query"], "kind": row["kind"],
        "target_type": row.get("target_type"), "target_url": row.get("target_url"), "group_id": row.get("group_id"),
        "http": d.get("_http"), "error": d.get("_error"), "ms": d.get("_ms"),
        "first_url": url, "first_url_norm": normalize_url(url), "first_type": ttype, "first_source": src,
        "first_title": top0.get("title") or "", "first_family_id": top0.get("family_id") or "",
        "route": tr.get("route"), "effective_config": tr.get("effective_config"),
        "server_ms": tr.get("total_ms"), "cost_rub": tr.get("cost_rub"),
        "catalog_version": d.get("catalog_version") or tr.get("catalog_version"),
        "config": d.get("config"),
        "top": [{"product_id": it.get("product_id"), "offer_id": it.get("offer_id"), "family_id": it.get("family_id"),
                 "title": (it.get("title") or "")[:90], "family_url": it.get("family_url"),
                 "offer_url": it.get("offer_url"), "match_type": it.get("match_type"), "group": it.get("group")}
                for it in (d.get("items") or [])[:5]],
        **counts(d),
    }
    if row["kind"] == "эталон":
        res["matched"] = matched
        res["miss_reason"] = miss_reason(matched, row, d) if d.get("_http") == 200 else "ошибка HTTP"
    return res


def percentile(values, p):
    """Перцентиль ближайшего ранга."""
    if not values:
        return None
    s = sorted(values)
    return s[max(0, math.ceil(p / 100 * len(s)) - 1)]


def summarize(results):
    et = [r for r in results if r["kind"] == "эталон"]
    tz = [r for r in results if r["kind"] == "ТЗ"]
    ok = [r for r in results if r["http"] == 200]
    ms = [r["ms"] for r in ok if r["ms"] is not None]

    def share(rows, key):
        return {"n": sum(1 for r in rows if r["http"] == 200 and r[key] >= 4), "of": len(rows)}

    versions = sorted({r["catalog_version"] for r in ok if r["catalog_version"]})
    return {
        "matched": sum(1 for r in et if r.get("matched")), "etalons": len(et),
        "families_ge4": share(results, "families"), "skus_ge4": share(results, "skus"),
        "cards_ge4": share(results, "cards"),
        "families_ge4_etalon": share(et, "families"), "families_ge4_tz": share(tz, "families"),
        "skus_ge4_etalon": share(et, "skus"), "skus_ge4_tz": share(tz, "skus"),
        "p50_ms": percentile(ms, 50), "p95_ms": percentile(ms, 95),
        "http_errors": len(results) - len(ok), "responses_ok": len(ok),
        "with_url": sum(1 for r in ok if r["first_url"]),
        "catalog_versions": versions,
        "routes": dict(Counter(r["route"] or "—" for r in ok)),
        "cost_rub": round(sum(r["cost_rub"] or 0 for r in ok), 4),
        "miss_reasons": dict(Counter(r["miss_reason"] for r in et if r.get("miss_reason"))),
    }


# --- вывод ------------------------------------------------------------------------------------------------

def _cut(s, n):
    s = s or ""
    return s if len(s) <= n else s[: n - 1] + "…"


def _dist(r):
    if r["match_type"]:
        return " ".join(f"{k}:{v}" for k, v in sorted(r["match_type"].items()))
    if r["group"]:
        return "group " + " ".join(f"{k}:{v}" for k, v in sorted(r["group"].items()))
    return "—"


def _verdict(r):
    if r["kind"] != "эталон":
        return ""
    if r["http"] != 200:
        return "ошибка"
    if r.get("matched"):
        return "да"
    return f"нет ({r['miss_reason']})"


def _row_cells(r):
    types = f"{r['target_type'] or '—'}/{r['first_type'] or '—'}"
    first = r["first_url_norm"] or (NO_URL if r["http"] == 200 else f"HTTP {r['http']}")
    return [r["query"], first, _cut(r["first_title"], 44), types, _verdict(r),
            str(r["cards"]), str(r["families"]), str(r["skus"]), _dist(r), r["route"] or "—", str(r["ms"])]


HEAD = ["запрос", "первый URL", "первый товар", "тип эталон/ответ", "совпало", "карт.", "сем.", "SKU",
        "match_type", "маршрут", "мс"]


def _fmt_share(s):
    return f"{s['n']}/{s['of']} ({s['n'] / s['of']:.0%})" if s["of"] else "—"


def summary_lines(s):
    return [
        f"эталоны совпали: {s['matched']} из {s['etalons']}"
        + (f" (причины промахов: {', '.join(f'{k} — {v}' for k, v in sorted(s['miss_reasons'].items()))})"
           if s["miss_reasons"] else ""),
        f"ответов с ≥4 семействами: {_fmt_share(s['families_ge4'])} — эталоны {_fmt_share(s['families_ge4_etalon'])},"
        f" ТЗ {_fmt_share(s['families_ge4_tz'])}",
        f"ответов с ≥4 SKU: {_fmt_share(s['skus_ge4'])} — эталоны {_fmt_share(s['skus_ge4_etalon'])},"
        f" ТЗ {_fmt_share(s['skus_ge4_tz'])}",
        f"ответов с ≥4 карточками: {_fmt_share(s['cards_ge4'])}",
        f"задержка p50 {s['p50_ms']} мс, p95 {s['p95_ms']} мс (у клиента, включая сеть)",
        f"первый URL есть: {s['with_url']} из {s['responses_ok']} ответов",
        f"маршруты: {', '.join(f'{k} — {v}' for k, v in sorted(s['routes'].items()))}; "
        f"стоимость LLM по трассам {s['cost_rub']} ₽; ошибок HTTP {s['http_errors']}",
        f"версия каталога: {', '.join(s['catalog_versions']) or 'не пришла в ответе'}",
    ]


def print_table(cfg, results, s):
    rows = [_row_cells(r) for r in results]
    widths = [max(len(HEAD[i]), *(len(c[i]) for c in rows)) for i in range(len(HEAD))]
    print(f"\n== {cfg} — {CONFIG_NOTES.get(cfg, '')}")
    print("  ".join(h.ljust(w) for h, w in zip(HEAD, widths)))
    for c in rows:
        print("  ".join(v.ljust(w) for v, w in zip(c, widths)))
    print()
    for line in summary_lines(s):
        print("  " + line)


def _md_cell(v):
    return str(v).replace("|", "\\|")


def markdown(run):
    out = [f"# Оценка по эталонам заказчика — {run['started']}", "",
           f"API: `{run['api']}` · каталог: **{run['catalog_version'] or 'неизвестен'}**"
           + (" (mock — не фид заказчика)" if run["mock"] else "")
           + f" · версия правил: {run['meta_rules_version'] or '—'}", "",
           f"Команда: `{run['command']}`", ""]
    if run["mock"]:
        out += ["**Прогон на mock-каталоге.** В нём `family_id` — слаги, а не `group_id` фида, и URL семейств нет: "
                "совпадений с эталонами и причин «ссылка» / «ранг» здесь не может быть по построению. "
                "Это проверка скрипта и счётчиков, не точка отсчёта на данных задачи.", ""]
    out += ["Первая цель — поле `target`, иначе первый товар выдачи (URL семейства → оффера → вендора). "
            "URL сравниваются после нормализации: регистр хоста, схема и хост магазина, завершающий слэш, "
            "URL-кодирование, `[]` = `[0]`, порядок параметров. Семейства — разные `family_id` карточек; "
            "SKU — разные `offer_id`/`product_id` карточек и их вариантов лицензии. "
            "Причина промаха: «ссылка» — нужный `group_id` первым, но URL не тот или нет; «ранг N» — "
            "нужный `group_id` на позиции N; «цель» — нужного `group_id` в выдаче нет. "
            "Колонка match_type: если поля нет, показано распределение `group` (exact/match/analog). "
            "В B0–B2 кандидатов BM25 не больше 50, поэтому SKU = 50 — потолок, а не полнота.", ""]
    for cfg in run["configs"]:
        block = run["runs"][cfg]
        out += [f"## {cfg} — {CONFIG_NOTES.get(cfg, '')}", ""]
        out += ["| " + " | ".join(HEAD) + " |", "|" + "---|" * len(HEAD)]
        for r in block["results"]:
            out.append("| " + " | ".join(_md_cell(c) for c in _row_cells(r)) + " |")
        out += [""] + [f"- {line}" for line in summary_lines(block["summary"])] + [""]
    return "\n".join(out) + "\n"


def out_paths(base):
    base = re.sub(r"\.(json|md)$", "", base)
    if not os.path.isabs(base):
        base = os.path.join(os.getcwd(), base)
    return base + ".json", base + ".md"


def main(argv=None):
    ap = argparse.ArgumentParser(description="Оценка по эталонам заказчика через POST /api/search")
    ap.add_argument("--configs", default=DEFAULT_CONFIGS,
                    help=f"конфигурации через запятую: live, B0–B4 (по умолчанию {DEFAULT_CONFIGS})")
    ap.add_argument("--queries", default=QUERIES, help="файл эталонов JSONL")
    ap.add_argument("--pause", type=float, default=0.7, help="пауза между запросами, с")
    ap.add_argument("--out", help="путь без расширения: пишет <out>.json и <out>.md")
    a = ap.parse_args(argv)

    configs = ["live" if c.strip().lower() == "live" else c.strip().upper() for c in a.configs.split(",") if c.strip()]
    rows = load_queries(a.queries)
    m = meta()
    run = {"started": datetime.now().astimezone().isoformat(timespec="seconds"), "api": API,
           "command": " ".join(["python3", os.path.relpath(os.path.abspath(__file__), os.getcwd()), *(argv or [])]),
           "meta_catalog_version": m.get("catalog_version"), "meta_rules_version": m.get("rules_version"),
           "configs": configs, "runs": {}}
    print(f"API {API}; каталог по /api/meta: {m.get('catalog_version') or 'нет ответа'}; "
          f"запросов {len(rows)} × конфигураций {len(configs)}")
    first = True
    for cfg in configs:
        results = []
        for row in rows:
            if not first:
                time.sleep(a.pause)
            first = False
            results.append(run_one(row, cfg))
        s = summarize(results)
        run["runs"][cfg] = {"note": CONFIG_NOTES.get(cfg, ""), "summary": s, "results": results}
        print_table(cfg, results, s)
    versions = sorted({v for b in run["runs"].values() for v in b["summary"]["catalog_versions"]})
    run["catalog_version"] = ", ".join(versions) or run["meta_catalog_version"]
    run["mock"] = bool(run["catalog_version"]) and run["catalog_version"].lower().startswith("mock")
    if run["mock"]:
        print(f"\nВНИМАНИЕ: прогон на mock-каталоге ({run['catalog_version']}), не на фиде заказчика.")
    if a.out:
        jp, mp = out_paths(a.out)
        os.makedirs(os.path.dirname(jp), exist_ok=True)
        with open(jp, "w", encoding="utf-8") as f:
            json.dump(run, f, ensure_ascii=False, indent=1)
        with open(mp, "w", encoding="utf-8") as f:
            f.write(markdown(run))
        print(f"\nотчёт: {jp}\n       {mp}")
    return run


if __name__ == "__main__":
    main(sys.argv[1:])
