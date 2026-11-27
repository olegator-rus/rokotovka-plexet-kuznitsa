#!/usr/bin/env python3
"""Снимок SearchOps для маркетолога (требование 10): числа отчёта и состояния агента, только чтение.

Вход — демо-вход сотрудника, как в e2e-фикстурах: POST /api/auth/demo-login {"persona": "staff-ecom"} → cookie
plexet_auth. Дальше только GET:
  /api/agent/report     — разделы отчёта и их размер, слои тестового потока, итоги, очередь без разбора, пробелы подбора;
  /api/agent/runs       — запуски агента, их стоимость и токены;
  /api/costs/summary    — сводка стоимости (число запусков и средняя стоимость по журналу вызовов модели);
  /api/proposals        — предложения агента по статусам и видам, задачи владельцам по причинам;
  /api/configs          — версии правил поиска, утверждённые правки агента;
  /api/relations/versions — версии справочника связей.
Запуск агента, решения по предложениям и откаты скрипт не вызывает.

Запуск из корня репозитория:
  python3 scripts/eval/searchops_snapshot.py --out data/softline/eval/results/searchops-snapshot
  API=http://127.0.0.1:8093 python3 scripts/eval/searchops_snapshot.py --persona staff-head
"""

import argparse
import http.cookiejar
import json
import os
import re
import sys
import time
import urllib.error
import urllib.request
from collections import Counter

API = os.environ.get("API", "http://127.0.0.1:8088").rstrip("/")
ORIGIN = "http://localhost:5177"


class Client:
    def __init__(self):
        self.jar = http.cookiejar.CookieJar()
        self.op = urllib.request.build_opener(urllib.request.HTTPCookieProcessor(self.jar))

    def call(self, method, path, body=None, retries=6):
        data = json.dumps(body).encode("utf-8") if body is not None else None
        for attempt in range(retries):
            req = urllib.request.Request(API + path, data=data, method=method,
                                         headers={"Origin": ORIGIN, "Content-Type": "application/json"})
            try:
                with self.op.open(req, timeout=60) as r:
                    return json.load(r)
            except urllib.error.HTTPError as e:
                if e.code == 429 and attempt < retries - 1:
                    time.sleep(2 * (attempt + 1))
                    continue
                raise

    def get(self, path):
        return self.call("GET", path)


def cat_name(v):
    """Версия каталога без даты в имени (в отчётах даты не пишем): softline-ГГГГ-ММ-ДД → softline."""
    return re.sub(r"-\d{4}-\d{2}-\d{2}$", "", v) if isinstance(v, str) else v


def count(xs):
    return dict(Counter(xs).most_common())


def snapshot(c):
    rep = c.get("/api/agent/report")
    runs = c.get("/api/agent/runs").get("items") or []
    costs = (c.get("/api/costs/summary").get("summary")) or {}
    props = c.get("/api/proposals").get("items") or []
    cfg = c.get("/api/configs")
    rel = c.get("/api/relations/versions")

    current_cat = rep.get("catalog_version")
    ver_cat = {v["version"]: v.get("catalog_version") for v in cfg.get("items") or []}
    cur = lambda v: ver_cat.get(v) == current_cat

    def agent_codes(rows):
        return count((r.get("agent") or {}).get("label") or (r.get("agent") or {}).get("code") or "нет разбора агента" for r in rows)

    sections = [{"code": s["code"], "title": s["title"], "rows": len(s.get("rows") or []),
                 "searches": s.get("searches"), "failures": s.get("failures"),
                 "agent": agent_codes(s.get("rows") or [])} for s in rep.get("sections") or []]
    guide = rep.get("guide") or {}

    def run_stats(rs):
        return {"runs": len(rs), "by_status": count(r["status"] for r in rs), "by_mode": count(r.get("mode") for r in rs),
                "cost_rub": round(sum(r.get("cost_rub") or 0 for r in rs), 2),
                "avg_cost_rub": round(sum(r.get("cost_rub") or 0 for r in rs) / len(rs), 2) if rs else None,
                "max_cost_rub": round(max((r.get("cost_rub") or 0 for r in rs), default=0), 2),
                "tokens": sum((r.get("input_tokens") or 0) + (r.get("output_tokens") or 0) for r in rs),
                "tool_calls": sum(r.get("tool_calls") or 0 for r in rs),
                "proposals_created": sum(r.get("proposals_created") or 0 for r in rs),
                "zero_cost_runs": sum(1 for r in rs if not r.get("cost_rub"))}

    def prop_stats(ps):
        tasks = [p for p in ps if p.get("task_to_owner")]
        return {"proposals": len(ps), "by_status": count(p["status"] for p in ps),
                "by_action": count(p["action"] for p in ps),
                "by_diagnosis": count(p.get("diagnosis") for p in ps),
                "tasks": len(tasks),
                "tasks_by_cause": count(p.get("cause_label") or p.get("cause") or "—" for p in tasks),
                "tasks_by_owner": count(p.get("owner_label") or p.get("owner") or "—" for p in tasks),
                "tasks_by_status": count(p["status"] for p in tasks)}

    agent_cfg = [v for v in cfg.get("items") or [] if v.get("source") == "agent"]
    res = {
        "catalog_version": current_cat, "rules_version": rep.get("rules_version"), "days": rep.get("days"),
        "stream": {"label": (rep.get("stream") or {}).get("label"), "freq_label": (rep.get("stream") or {}).get("freq_label"),
                   "layers": [{k: l.get(k) for k in ("code", "label", "queries", "searches")}
                              for l in (rep.get("stream") or {}).get("layers") or []]},
        "totals": rep.get("totals"),
        "sections": sections,
        "pending": {"rows": len(rep.get("pending") or []), "agent": agent_codes(rep.get("pending") or [])},
        "guide": {"title": guide.get("title"), "total": guide.get("total"), "rows": len(guide.get("rows") or []),
                  "by_code": count(r.get("label") or r.get("code") for r in guide.get("rows") or [])},
        "runs_all": run_stats(runs),
        "runs_current_catalog": run_stats([r for r in runs if cur(r.get("rules_version"))]),
        "costs_summary": {k: costs.get(k) for k in ("agent_runs", "agent_run_avg_cost_rub", "llm_calls", "llm_cost_rub")},
        "proposals_all": prop_stats(props),
        "proposals_current_catalog": prop_stats([p for p in props if cur(p.get("base_version"))]),
        "rules_versions": {"active": cfg.get("active"),
                           "items": [{k: v.get(k) for k in ("version", "source", "author", "catalog_version", "archived",
                                                             "aliases", "description")} for v in cfg.get("items") or []]},
        "approved_agent_rules": [{"version": v["version"], "catalog_version": v.get("catalog_version"),
                                  "archived": v.get("archived"), "description": v.get("description")} for v in agent_cfg],
        "approved_proposals": count(p["status"] for p in props if p["status"] in ("accepted", "rolled_back")),
        "relations_versions": {"active": rel.get("active"),
                               "items": [{k: v.get(k) for k in ("version", "source", "author", "catalog_version", "archived")}
                                         for v in rel.get("items") or []]},
    }
    res["catalog_version"] = cat_name(res["catalog_version"])
    for v in res["rules_versions"]["items"] + res["approved_agent_rules"] + res["relations_versions"]["items"]:
        v["catalog_version"] = cat_name(v.get("catalog_version"))
    return res


def kv(d):
    return ", ".join(f"{k} — {v}" for k, v in (d or {}).items()) or "—"


def render_md(s):
    t = s["totals"] or {}
    ra, rc = s["runs_all"], s["runs_current_catalog"]
    pa, pc = s["proposals_all"], s["proposals_current_catalog"]
    out = ["# SearchOps: снимок отчёта маркетолога", "",
           "Требование 10. Скрипт `scripts/eval/searchops_snapshot.py`, вход `staff-ecom`, только GET. "
           f"Каталог `{s['catalog_version']}`, активная версия правил {s['rules_version']}, окно отчёта {s['days']} дн.", "",
           f"Поток: {s['stream']['label']} ({s['stream']['freq_label']}). Логов поиска заказчика нет.", "",
           "## Слои потока", "", "| Слой | Запросов | Поисков |", "| --- | --- | --- |"]
    for l in s["stream"]["layers"]:
        out.append(f"| {l['label']} | {l['queries']} | {l['searches']} |")
    out += ["", f"Итого: поисков {t.get('searches')}, неудачных {t.get('failed_searches')}; запросов {t.get('queries')}, "
            f"неудачных {t.get('failed_queries')}. Сумма по слоям: поисков "
            f"{sum(l['searches'] or 0 for l in s['stream']['layers'])}, запросов "
            f"{sum(l['queries'] or 0 for l in s['stream']['layers'])} (запрос может входить в несколько слоёв).", "",
            "## Разделы отчёта", "", "| Раздел | Строк | Поисков | Неудачных | Состояние разбора агентом |",
            "| --- | --- | --- | --- | --- |"]
    for sec in s["sections"]:
        out.append(f"| {sec['title']} | {sec['rows']} | {sec['searches']} | {sec['failures']} | {kv(sec['agent'])} |")
    out += ["", f"Очередь «агент ещё не разбирал»: {s['pending']['rows']} строк ({kv(s['pending']['agent'])}).",
            f"{s['guide']['title']}: всего {s['guide']['total']}, строк {s['guide']['rows']} ({kv(s['guide']['by_code'])}).", "",
            "## Запуски агента", "", "| | Все каталоги | Текущий каталог |", "| --- | --- | --- |",
            f"| Запусков | {ra['runs']} | {rc['runs']} |",
            f"| По режимам | {kv(ra['by_mode'])} | {kv(rc['by_mode'])} |",
            f"| По статусам | {kv(ra['by_status'])} | {kv(rc['by_status'])} |",
            f"| Стоимость, ₽ | {ra['cost_rub']} | {rc['cost_rub']} |",
            f"| Средняя / максимум за запуск, ₽ | {ra['avg_cost_rub']} / {ra['max_cost_rub']} | "
            f"{rc['avg_cost_rub']} / {rc['max_cost_rub']} |",
            f"| Запусков с нулевой стоимостью | {ra['zero_cost_runs']} | {rc['zero_cost_runs']} |",
            f"| Токенов | {ra['tokens']} | {rc['tokens']} |",
            f"| Вызовов инструментов | {ra['tool_calls']} | {rc['tool_calls']} |",
            f"| Предложений создано | {ra['proposals_created']} | {rc['proposals_created']} |", "",
            f"Сводка стоимости (`/api/costs/summary`, журнал вызовов модели): запусков агента "
            f"{s['costs_summary']['agent_runs']}, средняя стоимость {s['costs_summary']['agent_run_avg_cost_rub']} ₽, то есть "
            f"{round((s['costs_summary']['agent_runs'] or 0) * (s['costs_summary']['agent_run_avg_cost_rub'] or 0), 2)} ₽ "
            f"на запусках с вызовами модели. В таблице запусков с ненулевой стоимостью {ra['runs'] - ra['zero_cost_runs']}, "
            f"сумма {ra['cost_rub']} ₽: журнал вызовов считает запуски по agent_run_id, и числа не совпадают.", "",
            "## Предложения и задачи", "", "| | Все каталоги | Текущий каталог |", "| --- | --- | --- |",
            f"| Предложений | {pa['proposals']} | {pc['proposals']} |",
            f"| По статусам | {kv(pa['by_status'])} | {kv(pc['by_status'])} |",
            f"| По видам | {kv(pa['by_action'])} | {kv(pc['by_action'])} |",
            f"| Задач владельцам | {pa['tasks']} | {pc['tasks']} |",
            f"| Задачи по причинам | {kv(pa['tasks_by_cause'])} | {kv(pc['tasks_by_cause'])} |",
            f"| Задачи по владельцам | {kv(pa['tasks_by_owner'])} | {kv(pc['tasks_by_owner'])} |",
            f"| Задачи по статусам | {kv(pa['tasks_by_status'])} | {kv(pc['tasks_by_status'])} |", "",
            "## Утверждённые правки и версии правил", "",
            f"Предложения, утверждённые человеком: {kv(s['approved_proposals'])}.", "",
            "| Версия правил | Источник | Автор | Каталог | В архиве | Алиасов | Описание |",
            "| --- | --- | --- | --- | --- | --- | --- |"]
    for v in s["rules_versions"]["items"]:
        out.append(f"| {v['version']}{' (активная)' if v['version'] == s['rules_versions']['active'] else ''} | "
                   f"{v['source']} | {v['author']} | {v['catalog_version']} | {'да' if v['archived'] else 'нет'} | "
                   f"{v['aliases']} | {v['description']} |")
    rv = s["relations_versions"]
    out += ["", f"Версии справочника связей: {len(rv['items'])}, активная — {rv['active']} "
            f"({kv(Counter(str(v['catalog_version']) for v in rv['items']))}). На фиде блок допродажи собирается из "
            "файлов `data/softline/upsell_*.csv` и `compat_docs.csv`, а не из справочника связей.", "",
            "Качество разбора агента не размечено: снимок показывает объём и состояние, а не точность диагнозов.", ""]
    return "\n".join(out)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("--out", help="путь без расширения: пишутся .json и .md")
    ap.add_argument("--persona", default="staff-ecom")
    args = ap.parse_args()
    c = Client()
    c.call("POST", "/api/auth/demo-login", {"persona": args.persona})
    s = snapshot(c)
    s["persona"] = args.persona
    print(json.dumps({k: s[k] for k in ("totals", "runs_current_catalog", "proposals_current_catalog")},
                     ensure_ascii=False, indent=1))
    if args.out:
        with open(args.out + ".json", "w", encoding="utf-8") as f:
            json.dump(s, f, ensure_ascii=False, indent=1)
        with open(args.out + ".md", "w", encoding="utf-8") as f:
            f.write(render_md(s))
    return 0


if __name__ == "__main__":
    sys.exit(main())
