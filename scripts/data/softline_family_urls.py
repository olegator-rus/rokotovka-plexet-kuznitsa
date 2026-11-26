#!/usr/bin/env python3
"""Сборщик URL страниц семейств store.softline.ru по хлебным крошкам SKU.

Только стандартная библиотека. Только GET, один поток, пауза между запросами
не меньше 1 с (или Crawl-delay из robots.txt, если он больше).

Команды:
  collect   — для каждого group_id с доступным оффером открыть один SKU
              и взять URL семейства из хлебных крошек (возобновляемо);
  refs      — сверка собранного с эталонами customer-queries.jsonl
              и known-family-urls.txt (без сети);
  sitemap   — скачать /https-sitemap.xml и sitemap/families.xml, сверить;
  prices    — на N случайных семействах сравнить цену «от» страницы
              с минимальной ценой доступного оффера группы в фиде;
  stats     — покрытие и неудачи по причинам (без сети).
"""

from __future__ import annotations

import argparse
import gzip
import html
import json
import random
import re
import signal
import sys
import time
import urllib.error
import urllib.parse
import urllib.request
import xml.etree.ElementTree as ET
from collections import Counter
from datetime import datetime, timezone
from html.parser import HTMLParser
from pathlib import Path

try:
    import fcntl
except ImportError:  # pragma: no cover
    fcntl = None

ROOT = Path(__file__).resolve().parents[2]
SITE = "https://store.softline.ru"
HOST = "store.softline.ru"
USER_AGENT = "Mozilla/5.0 (compatible; catalog-check/1.0)"
TIMEOUT = 20
MIN_DELAY = 1.0
ERROR_PAUSE = 60
MAX_CONSECUTIVE_ERRORS = 3
MAX_REDIRECTS = 3
MAX_ATTEMPTS_PER_GROUP = 2
MAX_CONSECUTIVE_NO_CRUMBS = 5

DEFAULT_FEED = ROOT / "data/softline/feed/Mindbox.yml.xml"
DEFAULT_OUT = ROOT / "data/softline/family_urls.jsonl"
DEFAULT_FAILED = ROOT / "data/softline/family_urls_failed.jsonl"
PKG_DATA = ROOT / "data/softline/eval"
DEFAULT_QUERIES = PKG_DATA / "customer-queries.jsonl"
DEFAULT_KNOWN = PKG_DATA / "known-family-urls.txt"

BLOCK_MARKERS = (
    "showcaptcha",
    "checkcaptcha",
    "не робот",
    "are you a robot",
    "are you human",
    "ddos-guard",
    "qrator",
    "access denied",
    "доступ ограничен",
    "доступ запрещ",
    "cf-challenge",
    "challenge-platform",
)


class StopCrawl(Exception):
    """Остановка сбора: сайт отказывает, капча или неожиданная разметка."""


def now_iso() -> str:
    return datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ")


def log(msg: str) -> None:
    print(f"{datetime.now().strftime('%Y-%m-%d %H:%M:%S')} {msg}", flush=True)


# ---------------------------------------------------------------- URL utils

def norm_path(url: str) -> str:
    """Путь URL в нижнем регистре хоста, с завершающим слэшем, без query."""
    p = urllib.parse.urlsplit(url.strip())
    path = p.path or "/"
    if not path.endswith("/"):
        path += "/"
    return path


def norm_url(url: str) -> str:
    p = urllib.parse.urlsplit(url.strip())
    host = (p.netloc or HOST).lower()
    if host.startswith("www."):
        host = host[4:]
    scheme = "https"
    q = p.query.replace("filter%5Bcategory%5D%5B0%5D", "filter[category][]")
    q = q.replace("filter[category][0]", "filter[category][]")
    q = q.replace("filter%5Bcategory%5D%5B%5D", "filter[category][]")
    return urllib.parse.urlunsplit((scheme, host, norm_path(url), q, ""))


def abs_url(href: str) -> str:
    return urllib.parse.urljoin(SITE + "/", href)


# ---------------------------------------------------------------- robots.txt

class Robots:
    """Разбор группы User-agent: * с поддержкой '*' и '$' (самое длинное правило)."""

    def __init__(self, text: str):
        self.rules: list[tuple[str, bool]] = []  # (pattern, allow)
        self.crawl_delay: float | None = None
        groups: list[tuple[list[str], list[tuple[str, str]]]] = []
        agents: list[str] = []
        lines: list[tuple[str, str]] = []
        last_was_agent = False
        for raw in text.splitlines():
            line = raw.split("#", 1)[0].strip()
            if not line or ":" not in line:
                continue
            key, val = line.split(":", 1)
            key, val = key.strip().lower(), val.strip()
            if key == "user-agent":
                if not last_was_agent and agents:
                    groups.append((agents, lines))
                    agents, lines = [], []
                agents.append(val.lower())
                last_was_agent = True
            else:
                lines.append((key, val))
                last_was_agent = False
        if agents:
            groups.append((agents, lines))
        for ag, ls in groups:
            if "*" in ag:
                for key, val in ls:
                    if key in ("allow", "disallow") and val:
                        self.rules.append((val, key == "allow"))
                    elif key == "crawl-delay":
                        try:
                            self.crawl_delay = float(val)
                        except ValueError:
                            pass
                break

    @staticmethod
    def _match(pattern: str, path: str) -> bool:
        anchored = pattern.endswith("$")
        pat = pattern[:-1] if anchored else pattern
        rx = "^" + ".*".join(re.escape(part) for part in pat.split("*"))
        if anchored:
            rx += "$"
        return re.match(rx, path) is not None

    def allowed(self, url: str) -> bool:
        p = urllib.parse.urlsplit(url)
        path = p.path or "/"
        if p.query:
            path += "?" + p.query
        best: tuple[int, bool] | None = None
        for pattern, allow in self.rules:
            if self._match(pattern, path):
                cand = (len(pattern), allow)
                if best is None or cand[0] > best[0] or (cand[0] == best[0] and allow):
                    best = cand
        return True if best is None else best[1]


# ---------------------------------------------------------------- HTTP client

class _NoRedirect(urllib.request.HTTPRedirectHandler):
    def redirect_request(self, req, fp, code, msg, headers, newurl):
        return None


class PoliteClient:
    def __init__(self, delay: float = MIN_DELAY):
        self.delay = max(MIN_DELAY, delay)
        self.opener = urllib.request.build_opener(_NoRedirect())
        self.last_done = 0.0
        self.consecutive_errors = 0
        self.requests = 0
        self.robots: Robots | None = None

    def _wait(self) -> None:
        gap = time.monotonic() - self.last_done
        if gap < self.delay:
            time.sleep(self.delay - gap)

    def _raw_get(self, url: str) -> tuple[int, dict, bytes]:
        self._wait()
        req = urllib.request.Request(
            url,
            method="GET",
            headers={
                "User-Agent": USER_AGENT,
                "Accept": "text/html,application/xhtml+xml,application/xml;q=0.9,*/*;q=0.8",
                "Accept-Language": "ru,en;q=0.5",
                "Accept-Encoding": "gzip",
            },
        )
        self.requests += 1
        try:
            with self.opener.open(req, timeout=TIMEOUT) as resp:
                status, headers, body = resp.status, dict(resp.headers), resp.read()
        except urllib.error.HTTPError as e:
            status, headers = e.code, dict(e.headers or {})
            try:
                body = e.read()
            except Exception:
                body = b""
        finally:
            self.last_done = time.monotonic()
        if headers.get("Content-Encoding", headers.get("content-encoding", "")).lower() == "gzip":
            try:
                body = gzip.decompress(body)
            except OSError:
                pass
        return status, headers, body

    def get(self, url: str) -> tuple[int, str, str]:
        """GET с ручными редиректами и правилами ошибок.

        Возвращает (status, final_url, text). 429/403/5xx и сетевые ошибки —
        пауза 60 с и повтор; три подряд — StopCrawl.
        """
        if self.robots is not None and not self.robots.allowed(url):
            return -1, url, ""
        hops = 0
        while True:
            try:
                status, headers, body = self._raw_get(url)
            except (urllib.error.URLError, TimeoutError, OSError) as e:
                status, headers, body = 0, {}, str(e).encode()
            if status in (429, 403) or status >= 500 or status == 0:
                self.consecutive_errors += 1
                detail = body[:200].decode("utf-8", "replace") if status == 0 else ""
                log(f"WARN http={status} {detail} url={url} "
                    f"(подряд {self.consecutive_errors}/{MAX_CONSECUTIVE_ERRORS})")
                if self.consecutive_errors >= MAX_CONSECUTIVE_ERRORS:
                    raise StopCrawl(
                        f"три ошибки подряд (последняя http={status}) на {url}"
                    )
                log(f"пауза {ERROR_PAUSE} с")
                time.sleep(ERROR_PAUSE)
                continue
            self.consecutive_errors = 0
            if status in (301, 302, 303, 307, 308):
                loc = headers.get("Location") or headers.get("location")
                if not loc or hops >= MAX_REDIRECTS:
                    return status, url, ""
                nxt = urllib.parse.urljoin(url, loc)
                if urllib.parse.urlsplit(nxt).netloc.lower() not in (HOST, "www." + HOST):
                    return -2, nxt, ""
                if self.robots is not None and not self.robots.allowed(nxt):
                    return -1, nxt, ""
                url, hops = nxt, hops + 1
                continue
            ctype = headers.get("Content-Type", headers.get("content-type", ""))
            m = re.search(r"charset=([\w-]+)", ctype or "")
            enc = m.group(1) if m else "utf-8"
            return status, url, body.decode(enc, "replace")

    def load_robots(self) -> Robots:
        status, _, text = self.get(SITE + "/robots.txt")
        if status != 200:
            raise StopCrawl(f"robots.txt недоступен: http={status}")
        self.robots = Robots(text)
        if self.robots.crawl_delay and self.robots.crawl_delay > self.delay:
            self.delay = self.robots.crawl_delay
        log(f"robots.txt: правил для *: {len(self.robots.rules)}, "
            f"Crawl-delay: {self.robots.crawl_delay}, пауза: {self.delay} с")
        return self.robots


# ---------------------------------------------------------------- feed

def parse_feed(path: Path) -> tuple[dict[str, str], dict[str, dict]]:
    """Категории и сводка по группам: кандидаты SKU и минимальная цена доступных."""
    t0 = time.monotonic()
    cats: dict[str, str] = {}
    groups: dict[str, dict] = {}
    offers_parent = None
    n_offers = n_nogroup = 0
    for event, el in ET.iterparse(str(path), events=("start", "end")):
        if event == "start":
            if el.tag == "offers":
                offers_parent = el
            continue
        if el.tag == "category":
            cats[el.get("id", "")] = (el.text or "").strip()
            continue
        if el.tag != "offer":
            continue
        n_offers += 1
        gid = el.get("group_id") or (el.findtext("group_id") or "").strip()
        if not gid:
            n_nogroup += 1
        else:
            avail = el.get("available") == "true"
            try:
                price = float((el.findtext("price") or "0").strip() or 0)
            except ValueError:
                price = 0.0
            g = groups.setdefault(gid, {"n": 0, "avail": 0, "min_avail_price": None,
                                        "min_avail_price_offer": None, "cands": []})
            g["n"] += 1
            if avail:
                g["avail"] += 1
                if price > 0 and (g["min_avail_price"] is None or price < g["min_avail_price"]):
                    g["min_avail_price"] = price
                    g["min_avail_price_offer"] = el.get("id")
            rank = 0 if (avail and price > 0) else 1 if avail else 2
            g["cands"].append((rank, el.get("id"), (el.findtext("url") or "").strip(), price))
            if len(g["cands"]) > 12:
                g["cands"].sort(key=lambda c: (c[0], int(c[1]) if c[1].isdigit() else 0))
                del g["cands"][6:]
        el.clear()
        if offers_parent is not None:
            offers_parent.clear()
    for g in groups.values():
        g["cands"].sort(key=lambda c: (c[0], int(c[1]) if c[1].isdigit() else 0))
        del g["cands"][6:]
    log(f"фид: офферов {n_offers}, без group_id {n_nogroup}, групп {len(groups)}, "
        f"с доступным оффером {sum(1 for g in groups.values() if g['avail'])}, "
        f"категорий {len(cats)}, {time.monotonic() - t0:.0f} с")
    return cats, groups


# ---------------------------------------------------------------- breadcrumbs

class _CrumbParser(HTMLParser):
    def __init__(self):
        super().__init__(convert_charrefs=True)
        self.items: list[dict] = []
        self._cur: dict | None = None
        self._in_name = False

    def handle_starttag(self, tag, attrs):
        a = dict(attrs)
        if tag == "li" and "itemListElement" in (a.get("itemprop") or ""):
            self._cur = {"href": None, "name": "", "position": None}
            self.items.append(self._cur)
        elif self._cur is not None:
            if tag == "a" and a.get("href") and self._cur["href"] is None:
                self._cur["href"] = a["href"]
                if a.get("title") and not self._cur["name"]:
                    self._cur["title"] = a["title"]
            elif tag == "span" and a.get("itemprop") == "name":
                self._in_name = True
            elif tag == "meta" and a.get("itemprop") == "position":
                self._cur["position"] = a.get("content")
            elif tag == "meta" and a.get("itemprop") == "item" and a.get("content"):
                self._cur["href"] = self._cur["href"] or a["content"]

    def handle_endtag(self, tag):
        if tag == "span":
            self._in_name = False
        elif tag == "li":
            self._cur = None
            self._in_name = False

    def handle_data(self, data):
        if self._in_name and self._cur is not None:
            self._cur["name"] += data


def extract_crumbs(page: str) -> list[dict] | None:
    i = page.find("schema.org/BreadcrumbList")
    if i < 0:
        return None
    start = page.rfind("<", 0, i)
    end = page.find("</ul>", i)
    if end < 0:
        end = page.find("</ol>", i)
    if end < 0:
        end = i + 20000
    p = _CrumbParser()
    p.feed(page[start:end + 5])
    for it in p.items:
        it["name"] = re.sub(r"\s+", " ", it["name"]).strip() or it.get("title", "")
    return p.items


def pick_family(crumbs: list[dict], sku_url: str, offer_id: str) -> tuple[dict | None, str]:
    """Возвращает (крошка семейства, причина неудачи)."""
    linked = [c for c in crumbs if c.get("href")]
    if not linked:
        return None, "no_breadcrumbs"
    sku_path = norm_path(abs_url(sku_url))
    cand_idx = len(linked) - 1

    def is_product(c: dict) -> bool:
        path = norm_path(abs_url(c["href"]))
        if path == sku_path:
            return True
        m = re.search(r"-(\d+)/$", path)
        return bool(m and m.group(1) == offer_id)

    if is_product(linked[cand_idx]):
        cand_idx -= 1
        if cand_idx < 0:
            return None, "only_product_crumb"
    cand = linked[cand_idx]
    cpath = norm_path(abs_url(cand["href"]))
    first = norm_path(abs_url(linked[0]["href"]))
    if cpath == "/":
        return None, "last_crumb_is_root"
    if cand_idx > 0 and cpath.startswith(first):
        return None, "last_crumb_is_category"
    if cand_idx == 0 and len(linked) == 1:
        return None, "single_crumb"
    if len([s for s in cpath.split("/") if s]) != 2:
        return None, "unexpected_crumb_shape"
    return cand, ""


def looks_blocked(page: str) -> bool:
    low = page[:300000].lower()
    return len(page) < 20000 or any(m in low for m in BLOCK_MARKERS)


def name_key(s: str) -> str:
    s = html.unescape(s).casefold().replace("ё", "е")
    s = re.sub(r"[«»\"'“”„]", "", s)
    return re.sub(r"\s+", " ", s).strip()


# ---------------------------------------------------------------- jsonl io

def read_jsonl(path: Path) -> list[dict]:
    if not path.exists():
        return []
    out = []
    with path.open(encoding="utf-8") as f:
        for line in f:
            line = line.strip()
            if not line:
                continue
            try:
                out.append(json.loads(line))
            except json.JSONDecodeError:
                continue
    return out


def append_jsonl(fh, rec: dict) -> None:
    fh.write(json.dumps(rec, ensure_ascii=False) + "\n")
    fh.flush()


# ---------------------------------------------------------------- commands

_stop_requested = False


def _on_signal(signum, frame):
    global _stop_requested
    _stop_requested = True
    log(f"получен сигнал {signum}: завершаю после текущей группы")


def cmd_collect(args) -> int:
    signal.signal(signal.SIGTERM, _on_signal)
    signal.signal(signal.SIGINT, _on_signal)
    out_path, failed_path = Path(args.out), Path(args.failed)
    out_path.parent.mkdir(parents=True, exist_ok=True)
    out_fh = out_path.open("a", encoding="utf-8")
    if fcntl is not None:
        try:
            fcntl.flock(out_fh, fcntl.LOCK_EX | fcntl.LOCK_NB)
        except BlockingIOError:
            log(f"STOP: {out_path} уже занят другим запуском")
            return 4
    failed_fh = failed_path.open("a", encoding="utf-8")

    done = {r["group_id"] for r in read_jsonl(out_path) if r.get("family_url")}
    failed_before = {r["group_id"] for r in read_jsonl(failed_path)}
    cats, groups = parse_feed(Path(args.feed))
    targets = sorted((g for g, v in groups.items() if v["avail"] > 0), key=lambda x: int(x))
    if args.groups:
        wanted = [g.strip() for g in args.groups.split(",") if g.strip()]
        missing = [g for g in wanted if g not in groups]
        if missing:
            log(f"нет в фиде: {missing}")
        targets = [g for g in wanted if g in groups]
    todo = [g for g in targets if g not in done
            and (args.retry_failed or g not in failed_before)]
    if args.limit:
        todo = todo[: args.limit]
    log(f"цель: {len(targets)} групп; уже собрано {len(done & set(targets))}; "
        f"пропуск прошлых неудач {0 if args.retry_failed else len((failed_before - done) & set(targets))}; "
        f"к обходу {len(todo)}")

    client = PoliteClient(args.delay)
    try:
        client.load_robots()
    except StopCrawl as e:
        log(f"STOP: {e}")
        return 2

    t0 = time.monotonic()
    ok = fail = 0
    no_crumbs_streak = 0
    reasons: Counter = Counter()
    try:
        for k, gid in enumerate(todo, 1):
            if _stop_requested:
                log("STOP: по сигналу, прогресс сохранён")
                return 5
            g = groups[gid]
            attempts = 0
            last_reason, last_info = "", {}
            for rank, offer_id, sku_url, _price in g["cands"]:
                if attempts >= MAX_ATTEMPTS_PER_GROUP:
                    break
                if not sku_url:
                    continue
                if client.robots and not client.robots.allowed(sku_url):
                    last_reason, last_info = "robots_disallow", {"sku_url": sku_url}
                    continue
                attempts += 1
                status, final_url, page = client.get(sku_url)
                info = {"sku_url": sku_url, "offer_id": offer_id, "http": status}
                if final_url != sku_url:
                    info["final_url"] = final_url
                if status == -1:
                    last_reason, last_info = "robots_disallow", info
                    continue
                if status == -2:
                    last_reason, last_info = "redirect_offsite", info
                    continue
                if status != 200:
                    last_reason, last_info = f"http_{status}", info
                    continue
                crumbs = extract_crumbs(page)
                if crumbs is None:
                    no_crumbs_streak += 1
                    if looks_blocked(page):
                        raise StopCrawl(f"похоже на капчу или блокировку: {sku_url} "
                                        f"(длина {len(page)})")
                    if no_crumbs_streak >= MAX_CONSECUTIVE_NO_CRUMBS:
                        raise StopCrawl(f"{no_crumbs_streak} страниц подряд без хлебных "
                                        f"крошек, последняя {sku_url}")
                    last_reason, last_info = "no_breadcrumbs", info
                    continue
                no_crumbs_streak = 0
                fam, reason = pick_family(crumbs, final_url, offer_id)
                if fam is None:
                    info["crumbs"] = [c.get("href") for c in crumbs]
                    last_reason, last_info = reason, info
                    continue
                fam_url = norm_url(abs_url(fam["href"]))
                rec = {
                    "group_id": gid,
                    "family_url": fam_url,
                    "source": "breadcrumbs",
                    "sku_url": sku_url,
                    "offer_id": offer_id,
                    "fetched_at": now_iso(),
                    "crumb_name": fam["name"],
                    "name_match": name_key(fam["name"]) == name_key(cats.get(gid, "")),
                }
                if final_url != sku_url:
                    rec["final_url"] = final_url
                append_jsonl(out_fh, rec)
                ok += 1
                last_reason = ""
                break
            if last_reason or attempts == 0:
                reason = last_reason or "no_candidate_sku"
                rec = {"group_id": gid, "reason": reason, "attempts": attempts,
                       "fetched_at": now_iso(), "feed_name": cats.get(gid, "")}
                rec.update(last_info)
                append_jsonl(failed_fh, rec)
                fail += 1
                reasons[reason] += 1
                log(f"FAIL group={gid} reason={reason} {last_info.get('sku_url', '')}")
            if k % 25 == 0 or k == len(todo):
                el = time.monotonic() - t0
                eta = el / k * (len(todo) - k)
                log(f"прогресс {k}/{len(todo)}: ok={ok} fail={fail} запросов={client.requests} "
                    f"прошло {el / 60:.1f} мин, осталось ~{eta / 60:.1f} мин")
    except StopCrawl as e:
        log(f"STOP: {e}. Собрано за запуск: {ok}, неудач: {fail}. "
            f"Повторный запуск продолжит с места остановки.")
        return 2
    finally:
        out_fh.close()
        failed_fh.close()
    log(f"DONE: собрано {ok}, неудач {fail} {dict(reasons)}, запросов {client.requests}, "
        f"{(time.monotonic() - t0) / 60:.1f} мин")
    return 0


def load_results(out: Path) -> dict[str, dict]:
    res = {}
    for r in read_jsonl(out):
        if r.get("family_url"):
            res[r["group_id"]] = r
    return res


def cmd_refs(args) -> int:
    res = load_results(Path(args.out))
    queries = read_jsonl(Path(args.queries))
    fam_q = [q for q in queries if q.get("target_type") == "family"]
    hit = 0
    print(f"Эталоны family из {args.queries}: {len(fam_q)}")
    for q in fam_q:
        r = res.get(q["group_id"])
        got = r["family_url"] if r else None
        ok = got is not None and norm_url(got) == norm_url(abs_url(q["target_url"]))
        hit += ok
        print(f"  {'OK ' if ok else 'BAD'} group={q['group_id']:>7} "
              f"цель={q['target_url']} собрано={got}")
    print(f"Совпало: {hit} из {len(fam_q)}")
    known = [l.strip() for l in Path(args.known).read_text(encoding="utf-8").splitlines()
             if l.strip()]
    collected = {norm_url(r["family_url"]) for r in res.values()}
    found = [u for u in known if norm_url(abs_url(u)) in collected]
    print(f"known-family-urls: {len(found)} из {len(known)} есть среди собранных")
    for u in known:
        if norm_url(abs_url(u)) not in collected:
            print(f"  нет: {u}")
    return 0 if hit == len(fam_q) else 1


def cmd_stats(args) -> int:
    res = load_results(Path(args.out))
    failed = read_jsonl(Path(args.failed))
    cats, groups = parse_feed(Path(args.feed))
    targets = {g for g, v in groups.items() if v["avail"] > 0}
    got = set(res) & targets
    last_fail: dict[str, dict] = {}
    for r in failed:
        last_fail[r["group_id"]] = r
    still = {g: r for g, r in last_fail.items() if g not in res}
    urls = Counter(r["family_url"] for r in res.values())
    dup = {u: c for u, c in urls.items() if c > 1}
    summary = {
        "targets": len(targets),
        "collected": len(got),
        "collected_outside_targets": len(set(res) - targets),
        "failed": len(still),
        "failed_reasons": Counter(r["reason"] for r in still.values()),
        "not_attempted": len(targets - got - set(still)),
        "unique_family_urls": len(urls),
        "family_urls_shared_by_groups": len(dup),
        "name_match": sum(1 for r in res.values() if r.get("name_match")),
        "redirected": sum(1 for r in res.values() if r.get("final_url")),
        "first_fetch": min((r["fetched_at"] for r in res.values()), default=None),
        "last_fetch": max((r["fetched_at"] for r in res.values()), default=None),
    }
    print(json.dumps(summary, ensure_ascii=False, indent=2, default=dict))
    if dup:
        print("URL, общий для нескольких групп:")
        for u, c in sorted(dup.items(), key=lambda x: -x[1])[:30]:
            gids = [g for g, r in res.items() if r["family_url"] == u]
            print(f"  {c} {u} {gids}")
    mism = [r for r in res.values() if not r.get("name_match")]
    if mism and args.verbose:
        print("Крошка не совпала с названием категории фида:")
        for r in mism[:60]:
            print(f"  {r['group_id']}: «{r.get('crumb_name')}» / «{cats.get(r['group_id'], '')}»"
                  f" {r['family_url']}")
    if still:
        print("Неудачи:")
        for g, r in sorted(still.items(), key=lambda x: int(x[0])):
            print(f"  {g} {r['reason']} {r.get('sku_url', '')} «{r.get('feed_name', '')}»")
    return 0


def _locs(xml_text: str) -> list[str]:
    return [html.unescape(m.strip()) for m in re.findall(r"<loc>\s*([^<]+?)\s*</loc>", xml_text)]


def cmd_sitemap(args) -> int:
    save_dir = Path(args.save_dir)
    save_dir.mkdir(parents=True, exist_ok=True)
    client = PoliteClient(args.delay)
    try:
        client.load_robots()
        status, _, idx = client.get(SITE + "/https-sitemap.xml")
        if status != 200:
            print(f"https-sitemap.xml: http={status}")
            return 2
        (save_dir / "https-sitemap.xml").write_text(idx, encoding="utf-8")
        locs = _locs(idx)
        fam_maps = [u for u in locs if "families" in u]
        print(f"индекс sitemap: {len(locs)} ссылок; семейства: {fam_maps}")
        if not fam_maps:
            return 2
        fam_urls: list[str] = []
        for u in fam_maps:
            status, _, text = client.get(u)
            if status != 200:
                print(f"{u}: http={status}")
                return 2
            name = urllib.parse.urlsplit(u).path.strip("/").replace("/", "_")
            (save_dir / name).write_text(text, encoding="utf-8")
            fam_urls += _locs(text)
    except StopCrawl as e:
        print(f"STOP: {e}")
        return 2
    sm = {norm_url(u) for u in fam_urls}
    res = load_results(Path(args.out))
    coll = {norm_url(r["family_url"]) for r in res.values()}
    inter = coll & sm
    print(json.dumps({
        "sitemap_family_urls": len(fam_urls),
        "sitemap_unique": len(sm),
        "collected_unique": len(coll),
        "collected_in_sitemap": len(inter),
        "collected_in_sitemap_share": round(len(inter) / len(coll), 4) if coll else None,
        "collected_not_in_sitemap": len(coll - sm),
        "sitemap_not_collected": len(sm - coll),
    }, ensure_ascii=False, indent=2))
    extra = sorted(coll - sm)
    if extra:
        print("Собрано, но нет в sitemap:")
        for u in extra[:50]:
            print(f"  {u}")
    (save_dir / "sitemap_not_collected.txt").write_text(
        "\n".join(sorted(sm - coll)) + "\n", encoding="utf-8")
    (save_dir / "collected_not_in_sitemap.txt").write_text(
        "\n".join(extra) + "\n", encoding="utf-8")
    return 0


def _family_low_price(page: str) -> tuple[float | None, int | None]:
    """lowPrice и offerCount из AggregateOffer в JSON-LD страницы семейства."""
    for m in re.finditer(r'<script[^>]*application/ld\+json[^>]*>(.*?)</script>', page, re.S):
        try:
            data = json.loads(m.group(1))
        except json.JSONDecodeError:
            continue
        stack = [data]
        while stack:
            x = stack.pop()
            if isinstance(x, dict):
                off = x.get("offers")
                if x.get("@type") == "Product" and isinstance(off, dict) \
                        and off.get("@type") == "AggregateOffer":
                    try:
                        lp = float(off.get("lowPrice")) if off.get("lowPrice") not in (None, "") else None
                    except (TypeError, ValueError):
                        lp = None
                    oc = off.get("offerCount")
                    return lp, int(oc) if str(oc).isdigit() else None
                stack.extend(x.values())
            elif isinstance(x, list):
                stack.extend(x)
    return None, None


def cmd_prices(args) -> int:
    res = load_results(Path(args.out))
    cats, groups = parse_feed(Path(args.feed))
    pool = sorted(g for g in res if g in groups and groups[g]["avail"] > 0)
    rnd = random.Random(args.seed)
    sample = rnd.sample(pool, min(args.n, len(pool)))
    client = PoliteClient(args.delay)
    rows = []
    try:
        client.load_robots()
        for gid in sample:
            url = res[gid]["family_url"]
            status, final, page = client.get(url)
            low, cnt = _family_low_price(page) if status == 200 else (None, None)
            feed_min = groups[gid]["min_avail_price"]
            match = low is not None and feed_min is not None and abs(low - feed_min) < 0.51
            rows.append({
                "group_id": gid, "family_url": url, "http": status,
                "page_low_price": low, "page_offer_count": cnt,
                "feed_min_avail_price": feed_min,
                "feed_min_offer": groups[gid]["min_avail_price_offer"],
                "feed_avail": groups[gid]["avail"], "feed_n": groups[gid]["n"],
                "match": match, "name": cats.get(gid, ""),
            })
            r = rows[-1]
            print(f"{'OK ' if match else 'DIFF'} {gid:>7} страница={low} фид={feed_min} "
                  f"офферов стр/фид={cnt}/{r['feed_avail']}({r['feed_n']}) {url}", flush=True)
    except StopCrawl as e:
        print(f"STOP: {e}")
    if args.save:
        Path(args.save).write_text(json.dumps(rows, ensure_ascii=False, indent=2), encoding="utf-8")
    print(f"совпало {sum(r['match'] for r in rows)} из {len(rows)}")
    return 0


def main() -> int:
    ap = argparse.ArgumentParser(description=__doc__, formatter_class=argparse.RawTextHelpFormatter)
    ap.add_argument("--feed", default=str(DEFAULT_FEED))
    ap.add_argument("--out", default=str(DEFAULT_OUT))
    ap.add_argument("--failed", default=str(DEFAULT_FAILED))
    ap.add_argument("--delay", type=float, default=1.1,
                    help="пауза между запросами, с (не меньше 1.0)")
    sub = ap.add_subparsers(dest="cmd", required=True)

    c = sub.add_parser("collect")
    c.add_argument("--groups", help="только эти group_id через запятую")
    c.add_argument("--limit", type=int, default=0)
    c.add_argument("--retry-failed", action="store_true",
                   help="повторить группы из файла неудач")

    r = sub.add_parser("refs")
    r.add_argument("--queries", default=str(DEFAULT_QUERIES))
    r.add_argument("--known", default=str(DEFAULT_KNOWN))

    s = sub.add_parser("stats")
    s.add_argument("-v", "--verbose", action="store_true")

    m = sub.add_parser("sitemap")
    m.add_argument("--save-dir", required=True)

    p = sub.add_parser("prices")
    p.add_argument("-n", type=int, default=15)
    p.add_argument("--seed", type=int, default=0x1352906)
    p.add_argument("--save")

    args = ap.parse_args()
    if args.delay < MIN_DELAY:
        ap.error("--delay не может быть меньше 1.0")
    return {"collect": cmd_collect, "refs": cmd_refs, "stats": cmd_stats,
            "sitemap": cmd_sitemap, "prices": cmd_prices}[args.cmd](args)


if __name__ == "__main__":
    sys.exit(main())
