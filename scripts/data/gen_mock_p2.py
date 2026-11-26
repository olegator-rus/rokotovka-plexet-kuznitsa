"""Контур коммерции поверх выхода контура поиска: оборудование, CAL 2025, правила, шаблоны, набор p2, заказы с аккаунтами.

Вызывается из gen_mock.py после того, как построены и посчитаны данные контура поиска (включая события и популярность).
Данные контура поиска не меняются: позиции, запросы, разметка и строки событий остаются как были. Единственная правка
позиций контура поиска — добавление ключа attributes.version у серверных ОС, CAL и SQL Server.
Новые события дописываются в конец events.jsonl, отдельным отсортированным по времени блоком.
"""

import math
import random
from collections import Counter, defaultdict
from datetime import timedelta

from defs_p2 import (ATTR, ATTR_BY_CATEGORY, CATEGORIES_P2, COMPAT, COMPAT_SOURCE, DELL_URL, HARDWARE, HPE_URL, KINGSTON_URL,
                     LICENSE_RULES, MARGIN_RANGE, MS_CAL_URL, MS_RDS_URL, MS_SQL_URL, ONEC_URL, P1_VERSION, REVIEWED,
                     SERVER_MODELS, SOFTWARE_DESC, SOFTWARE_P2, SOLUTION_TEMPLATES, VENDORS_P2, YADRO_URL)
from defs_queries_p2 import COMPAT as COMPAT_SEL, P2_QUERIES

CATALOG_VERSION = "mock-2"
SYNTHETIC = ["margin", "orders"]
SESSION_PREFIX = "s2-"
ALPHA = 0.01          # односторонний точный тест Фишера, уровень 0,99
MIN_ACCOUNTS = 3
PLANTED_PAIR = ("dell-poweredge-r750", "ippon-innova-rt-ii")
NOISE_ACCOUNT = "acc-007"
NOISE_PAIR = ("mon-hp-e24g5", "sw-dlink-dgs1210-28")


# ------------------------------------------------------------------ статистика
def fisher_greater(a, b, c, d):
    """P(X >= a) для таблицы [[a, b], [c, d]] (гипергеометрическое, односторонний тест)."""
    n1, k, n = a + b, a + c, a + b + c + d
    total = math.comb(n, n1)
    return sum(math.comb(k, x) * math.comb(n - k, n1 - x) for x in range(a, min(n1, k) + 1)) / total


def copurchase_stats(order_lines, fam_of):
    """По заказам с аккаунтом: пары семейств → (a, b, c, d, аккаунты, p)."""
    baskets, acc_of = defaultdict(set), {}
    for e in order_lines:
        if e.get("account_id"):
            baskets[e["order_id"]].add(fam_of[e["product_id"]])
            acc_of[e["order_id"]] = e["account_id"]
    n = len(baskets)
    single, pair, pair_acc = Counter(), Counter(), defaultdict(set)
    for oid, fams in baskets.items():
        for f in fams:
            single[f] += 1
        fs = sorted(fams)
        for i in range(len(fs)):
            for j in range(i + 1, len(fs)):
                pair[(fs[i], fs[j])] += 1
                pair_acc[(fs[i], fs[j])].add(acc_of[oid])
    out = {}
    for (x, y), a in pair.items():
        b, c = single[x] - a, single[y] - a
        d = n - a - b - c
        out[(x, y)] = dict(a=a, b=b, c=c, d=d, accounts=len(pair_acc[(x, y)]), p=fisher_greater(a, b, c, d))
    return out, n


def unexplained_pairs(stats, relations):
    """Значимые пары без объяснения: нет прямой связи и нет общего соседа по связям (комплектующие одного якоря),
    ≥ MIN_ACCOUNTS аккаунтов, p ниже порога Бонферрони по всем парам с ≥ MIN_ACCOUNTS аккаунтами."""
    nb = defaultdict(set)
    for r in relations:
        nb[r["from_family"]].add(r["to_family"])
        nb[r["to_family"]].add(r["from_family"])
    tested = {k: v for k, v in stats.items() if v["accounts"] >= MIN_ACCOUNTS}
    bonf = ALPHA / max(1, len(tested))
    out = {k: v for k, v in tested.items()
           if v["p"] < bonf and k[1] not in nb[k[0]] and not (nb[k[0]] & nb[k[1]])}
    return dict(sorted(out.items(), key=lambda kv: kv[1]["p"])), bonf, len(tested)


# ------------------------------------------------------------------ каталог
def _product(base, rng, vendors_by_id, kind):
    vend = vendors_by_id[base["vendor_id"]]
    mode = base.get("price_mode", "fixed")
    price = base["price"]
    lic = base.get("license") or dict(type=None, metric=None, quantity=None, duration_months=None, edition="")
    lo, hi = MARGIN_RANGE[base["category_id"]]
    return {
        "id": base["id"], "sku": base.get("sku", ""), "family_id": base["family_id"],
        "family_name": base["family_name"], "title": base["title"], "vendor_id": base["vendor_id"],
        "category_id": base["category_id"], "description": base["description"], "attributes": base["attributes"],
        "license": lic, "price_rub": price if mode != "quote_required" else None,
        "price_on_request": mode == "quote_required", "origin": vend["origin"], "in_reestr": vend["origin"] == "ru",
        "popularity": 0.01, "margin": round(rng.uniform(lo, hi), 3), "kind": kind, "mpn": base.get("mpn", ""),
        "specs": base.get("specs", {}), "compatible_with": [], "model_id": base.get("model_id"), "price_mode": mode,
    }


def build_products(rng, vendors_by_id):
    out = []
    for pid, fam, fname, title, sku, edition, metric, qty, price, by_one in SOFTWARE_P2:
        unit = {"user": "пользователю", "device": "устройству"}[metric]
        attrs = {"edition": edition, "version": "2025"}
        if by_one:
            attrs["purchase"] = "поштучно"
        base = dict(id=pid, sku=sku, family_id=fam, family_name=fname, title=title, vendor_id="microsoft",
                    category_id="os-server", description=SOFTWARE_DESC[fam].format(unit=unit), attributes=attrs,
                    license=dict(type="perpetual", metric=metric, quantity=qty, duration_months=None, edition=edition),
                    price=price)
        out.append(_product(base, rng, vendors_by_id, "software"))
    for base in HARDWARE:
        out.append(_product(base, rng, vendors_by_id, "hardware"))
    return out


def build_compat(products):
    by_id = {p["id"]: p for p in products}
    rules = []
    for i, (pid, model, state, cond) in enumerate(COMPAT, 1):
        src, url_vendor = COMPAT_SOURCE.get(by_id[pid]["vendor_id"], ("expert", None))
        url = ""
        if url_vendor in ("dell", "hpe") and model.startswith(url_vendor):
            url = (DELL_URL if url_vendor == "dell" else HPE_URL).format(model=model.split("-", 1)[1])
        elif url_vendor == "yadro":
            url = YADRO_URL
        elif url_vendor == "kingston":
            url = KINGSTON_URL
        elif url_vendor in ("dell", "hpe"):
            src = "expert"  # компонент одного вендора в сервере другого: вывод эксперта, не документ
        rules.append({"id": f"cr-{i:04d}", "product_id": pid, "target_model": model, "state": state,
                      "condition": cond, "source": src, "source_url": url, "rule_version": 1, "date": REVIEWED})
    order = {m: i for i, m in enumerate(SERVER_MODELS)}
    for p in products:
        if "kind" not in p:
            continue  # позиции контура поиска не трогаем
        ok = [r["target_model"] for r in rules if r["product_id"] == p["id"] and r["state"] in ("verified", "conditional")]
        p["compatible_with"] = sorted(set(ok), key=lambda m: order.get(m, 99))
    return rules


def compat_state(rules, pid, model):
    for r in rules:
        if r["product_id"] == pid and r["target_model"] == model:
            return r["state"]
    return "unknown"


# ------------------------------------------------------------------ связи
def _rel(rid, frm, to, typ, note, requirement=None, compatibility=None, conditions=None, quantity_strategy=None,
         source="catalog_rule", source_url="", evidence_level="E2", review_status="approved", reviewed_at=REVIEWED,
         from_product=None, to_product=None, upgrade_adds=None):
    r = {"id": rid, "from_family": frm, "to_family": to, "from_product": from_product, "to_product": to_product,
         "type": typ, "note": note, "requirement": requirement or "optional", "compatibility": compatibility or "unknown",
         "conditions": conditions or [], "quantity_strategy": quantity_strategy or "none", "source": source,
         "source_url": source_url, "evidence_level": evidence_level, "review_status": review_status,
         "reviewed_at": reviewed_at}
    if typ == "upgrade":
        r["upgrade_adds"] = upgrade_adds or []
    return r


ACCESS = ["access_license_required", "no_existing_equivalent_license"]

# Уточнения для связей контура поиска: (from, to, type) → поля. from/to/type/note не меняются.
P1_ENRICH = {
    ("windows-server-2022", "windows-server-cal", "complement"): dict(
        requirement="conditional_required", compatibility="verified", conditions=ACCESS, quantity_strategy="per_user",
        source="vendor_documentation", source_url=MS_CAL_URL, evidence_level="E2"),
    ("windows-server-cal", "windows-server-2022", "compatibility"): dict(
        requirement="optional", compatibility="verified", source="vendor_documentation", source_url=MS_CAL_URL,
        evidence_level="E2"),
    ("sql-server-2022", "windows-server-2022", "complement"): dict(
        requirement="recommended", compatibility="verified", quantity_strategy="per_server",
        source="vendor_documentation", source_url=MS_SQL_URL, evidence_level="E2"),
    ("1c-client-license", "1c-server", "complement"): dict(
        requirement="recommended", compatibility="verified", quantity_strategy="one", source="vendor_documentation",
        source_url=ONEC_URL, evidence_level="E2"),
    ("1c-complex-automation", "1c-server", "complement"): dict(
        requirement="recommended", compatibility="verified", quantity_strategy="one", evidence_level="E3",
        source="expert"),
    ("1c-server", "postgres-pro", "complement"): dict(
        requirement="recommended", compatibility="verified", quantity_strategy="one", evidence_level="E3",
        source="expert"),
    ("office-2024", "windows-11-pro", "complement"): dict(
        requirement="optional", compatibility="verified", quantity_strategy="per_device", evidence_level="E3"),
    ("kaspersky-endpoint-security", "astra-linux-se", "compatibility"): dict(
        compatibility="verified", source="vendor_documentation", evidence_level="E2"),
    ("kaspersky-endpoint-security", "red-os", "compatibility"): dict(
        compatibility="verified", source="vendor_documentation", evidence_level="E2"),
    ("myoffice-standard", "astra-linux-se", "compatibility"): dict(
        compatibility="verified", source="vendor_documentation", evidence_level="E2"),
    ("r7-office-professional", "alt-os", "compatibility"): dict(
        compatibility="verified", source="vendor_documentation", evidence_level="E2"),
    ("secret-net-studio", "windows-11-pro", "compatibility"): dict(
        compatibility="verified", source="vendor_documentation", evidence_level="E2"),
}

LAPTOP_FAMS = ["lenovo-thinkpad-e14-gen5", "lenovo-thinkpad-t14-gen4", "hp-probook-450-g10", "dell-latitude-5540",
               "huawei-matebook-d16", "aquarius-cmp-ns685"]
MONITOR_FAMS = ["dell-p2425h", "dell-p2725h", "dell-u2724d", "hp-e24-g5", "lenovo-l27i-40",
                "samsung-odyssey-g5", "huawei-mateview-se"]
SERVER_PARTS = {  # модель → семейства памяти, дисков, сервиса, которые к ней относятся
    "dell-poweredge-r740": ["dell-rdimm-ddr4", "dell-server-drives", "dell-prosupport"],
    "dell-poweredge-r750": ["dell-rdimm-ddr4", "dell-server-drives", "dell-prosupport"],
    "hpe-proliant-dl380-gen10": ["hpe-smart-memory-ddr4", "hpe-server-drives", "hpe-tech-care"],
    "hpe-proliant-dl380-gen11": ["hpe-smart-memory-ddr5", "hpe-server-drives", "hpe-tech-care"],
    "yadro-vegman-r220-g2": ["yadro-memory", "yadro-drives", "yadro-support"],
}


def build_relations(p1_relations):
    out = []
    for i, r in enumerate(p1_relations, 1):
        key = (r["from_family"], r["to_family"], r["type"])
        extra = dict(review_status="approved", reviewed_at="2026-04-02", evidence_level="E3")
        extra.update(P1_ENRICH.get(key, {}))
        out.append(_rel(f"rel-{i:04d}", r["from_family"], r["to_family"], r["type"], r["note"], **extra))
    n = len(out)

    def add(*a, **kw):
        out.append(_rel(f"rel-{len(out) + 1:04d}", *a, **kw))

    # сценарий 1: Windows Server → лицензии доступа
    add("windows-server-2025", "windows-server-cal-2025", "complement", "Для подключения пользователей нужны CAL 2025",
        requirement="conditional_required", compatibility="verified", conditions=ACCESS, quantity_strategy="per_user",
        source="vendor_documentation", source_url=MS_CAL_URL)
    add("windows-server-2025", "windows-server-cal", "complement", "Для подключения пользователей нужны CAL",
        requirement="conditional_required", compatibility="verified", conditions=ACCESS, quantity_strategy="per_user",
        source="catalog_rule", evidence_level="E1", reviewed_at="2025-09-11")                       # P2-01: сломана
    add("windows-server-cal-2025", "windows-server-2025", "compatibility", "CAL 2025 подходят для Windows Server 2025",
        compatibility="verified", source="vendor_documentation", source_url=MS_CAL_URL)
    add("windows-server-cal-2025", "windows-server-2022", "compatibility",
        "CAL 2025 дают доступ и к серверам 2022", compatibility="verified", source="vendor_documentation",
        source_url=MS_CAL_URL)
    add("rds-cal-2025", "windows-server-2025", "compatibility", "RDS CAL 2025 для удалённого рабочего стола",
        compatibility="verified", source="vendor_documentation", source_url=MS_RDS_URL)
    # P2-02: правило lr-ws25-rds есть, связи windows-server-2025 → rds-cal-2025 нет (намеренно)
    add("sql-server-2022", "sql-server-2022", "complement", "Модель Server + CAL: SQL CAL на каждого пользователя",
        from_product="sql22-stdsrv", to_product="sql22-cal", requirement="conditional_required",
        compatibility="verified", conditions=ACCESS, quantity_strategy="per_user", source="vendor_documentation",
        source_url=MS_SQL_URL)
    add("sql-server-2022", "windows-server-2025", "complement", "Серверная ОС под СУБД", requirement="recommended",
        compatibility="verified", quantity_strategy="per_server", source="vendor_documentation", source_url=MS_SQL_URL)
    add("1c-server", "1c-client-license", "complement", "Клиентские лицензии на подключения к серверу 1С",
        requirement="conditional_required", compatibility="verified", conditions=ACCESS, quantity_strategy="per_user",
        source="vendor_documentation", source_url=ONEC_URL)
    # старшие редакции
    add("windows-server-2025", "windows-server-2025", "upgrade", "Datacenter вместо Standard",
        from_product="ws25-std16", to_product="ws25-dc16", compatibility="verified", quantity_strategy="per_core",
        source="vendor_documentation", upgrade_adds=["Неограниченное число виртуальных машин Windows Server",
                                                     "Storage Spaces Direct и программно-определяемые сети"])
    add("windows-server-2022", "windows-server-2022", "upgrade", "Datacenter вместо Standard",
        from_product="ws22-std16", to_product="ws22-dc16", compatibility="verified", quantity_strategy="per_core",
        source="vendor_documentation", upgrade_adds=["Неограниченное число виртуальных машин Windows Server",
                                                     "Storage Spaces Direct и программно-определяемые сети"])
    add("microsoft-365-business", "microsoft-365-business", "upgrade", "Business Premium вместо Business Standard",
        from_product="m365b-bs", to_product="m365b-bp", compatibility="verified", quantity_strategy="per_user",
        source="vendor_documentation", upgrade_adds=["Microsoft Intune для управления устройствами",
                                                     "Microsoft Defender для бизнеса"])
    add("office-2024", "office-2024", "upgrade", "Для дома и бизнеса вместо версии для дома",
        from_product="off24-home", to_product="off24-hb", compatibility="verified", quantity_strategy="per_device",
        source="vendor_documentation", upgrade_adds=["Outlook", "Право коммерческого использования"])
    add("kaspersky-endpoint-security", "kaspersky-endpoint-security", "upgrade", "Расширенный вместо Стандартного",
        from_product="kes-std10", to_product="kes-ext10", compatibility="verified", quantity_strategy="per_device",
        source="vendor_documentation", upgrade_adds=["Шифрование дисков и файлов", "Управление исправлениями ПО"])
    # сценарий 2: сервер → совместимые комплектующие и гарантия (точная совместимость — compatibility_rules.json)
    for model, (mem, disk, svc) in SERVER_PARTS.items():
        add(model, mem, "complement", "Модули памяти из списка совместимости", requirement="recommended",
            compatibility="conditional" if model == "dell-poweredge-r740" else "verified",
            conditions=["check_compatibility_rule"], quantity_strategy="none", source="vendor_documentation")
        add(model, disk, "complement", "Диски из списка совместимости", requirement="recommended",
            compatibility="verified", conditions=["check_compatibility_rule"], quantity_strategy="none",
            source="vendor_documentation")
        add(model, svc, "complement", "Расширенная гарантия на сервер", requirement="recommended",
            compatibility="verified", quantity_strategy="per_server", source="vendor_documentation")
    # P2-05: связь-замена ведёт на снятую позицию
    add("dell-poweredge-r740", "hpe-proliant-dl380-gen10", "substitute", "Сервер 2U того же класса",
        to_product="srv-dl380g10-4210", compatibility="unknown", source="expert", evidence_level="E1",
        reviewed_at="2024-12-02")
    add("dell-poweredge-r740", "dell-poweredge-r750", "substitute", "Следующее поколение PowerEdge",
        compatibility="unknown", source="expert", evidence_level="E3")
    add("hpe-proliant-dl380-gen10", "hpe-proliant-dl380-gen11", "substitute", "Следующее поколение ProLiant",
        compatibility="unknown", source="expert", evidence_level="E3")
    add("eltex-mes2300", "ippon-smart-power-pro-ii", "complement", "ИБП для коммутатора PoE", requirement="optional",
        compatibility="verified", quantity_strategy="one", source="expert", evidence_level="E3")
    add("eltex-mes2300", "huawei-s5735", "substitute", "Коммутатор доступа того же класса", source="expert",
        evidence_level="E3")
    # сценарий 3: рабочее место → ноутбук, офис, защита
    for fam in LAPTOP_FAMS:
        add(fam, "windows-11-pro", "complement", "ОС для ноутбука без предустановленной системы",
            requirement="conditional_required", compatibility="verified",
            conditions=["no_preinstalled_os", "no_existing_equivalent_license"], quantity_strategy="per_device",
            source="catalog_rule", evidence_level="E2")
        add(fam, "microsoft-365-business", "complement", "Офисный пакет для рабочего места", requirement="optional",
            compatibility="verified", quantity_strategy="per_user", source="catalog_rule", evidence_level="E3")
        add(fam, "office-2024", "complement", "Офисный пакет для рабочего места", requirement="optional",
            compatibility="verified", quantity_strategy="per_device", source="catalog_rule", evidence_level="E3")
        add(fam, "kaspersky-endpoint-security", "complement", "Защита рабочего места", requirement="optional",
            compatibility="verified", quantity_strategy="per_device", source="catalog_rule", evidence_level="E3")
        for mon in ("dell-p2425h", "dell-p2725h"):
            add(fam, mon, "complement", "Второй экран для рабочего места", requirement="optional",
                compatibility="verified", quantity_strategy="per_device", source="catalog_rule", evidence_level="E3")
    add("aquarius-cmp-ns685", "astra-linux-se", "complement", "Отечественная ОС для ноутбука без ОС",
        requirement="optional", compatibility="verified", quantity_strategy="per_device",
        source="vendor_documentation", evidence_level="E2")
    add("lenovo-thinkpad-e14-gen5", "hp-probook-450-g10", "substitute", "Офисный ноутбук того же класса",
        source="expert", evidence_level="E3")
    add("lenovo-thinkpad-t14-gen4", "dell-latitude-5540", "substitute", "Бизнес-ноутбук того же класса",
        source="expert", evidence_level="E3")
    add("microsoft-365-business", "kaspersky-endpoint-security", "complement", "Часто берут вместе",
        requirement="optional", compatibility="verified", quantity_strategy="per_user", source="co_purchase",
        evidence_level="E4")
    add("office-2024", "kaspersky-endpoint-security", "complement", "Часто берут вместе", requirement="optional",
        compatibility="verified", quantity_strategy="per_device", source="co_purchase", evidence_level="E4")
    add("windows-11-pro", "kaspersky-endpoint-security", "complement", "Часто берут вместе", requirement="optional",
        compatibility="verified", quantity_strategy="per_device", source="co_purchase", evidence_level="E4")
    add("windows-11-pro", "microsoft-365-business", "complement", "Часто берут вместе", requirement="optional",
        compatibility="verified", quantity_strategy="per_user", source="co_purchase", evidence_level="E4")
    for mon in MONITOR_FAMS[2:]:
        add("dell-p2425h", mon, "substitute", "Офисный монитор", source="expert", evidence_level="E3")
    # F17: «Дополните решение» к серверной ОС — резервное копирование и защита сервера (в конце: id прежних не сдвигаются)
    for ws in ("windows-server-2025", "windows-server-2022"):
        add(ws, "cyber-backup", "complement", "Резервное копирование сервера", requirement="recommended",
            compatibility="verified", quantity_strategy="per_server", source="catalog_rule", evidence_level="E3")
        add(ws, "kaspersky-endpoint-security", "complement", "Защита сервера от вредоносного ПО",
            requirement="optional", compatibility="verified", quantity_strategy="per_server", source="catalog_rule",
            evidence_level="E3")
    return out, n


# ------------------------------------------------------------------ справочники
def build_attr_defs():
    return [{"category_id": c, "attrs": [dict(ATTR[k]) for k in keys]} for c, keys in ATTR_BY_CATEGORY.items()]


# ------------------------------------------------------------------ запросы
def judge(entries, products, rules, expect):
    out = {}
    for e in entries:
        if e[0] == COMPAT_SEL:
            _, category, cond, vendor, target = (list(e) + [None, None, None])[:5]
            target = target or expect.get("compat_target")
            for p in products:
                if p["category_id"] != category or (vendor and p["vendor_id"] != vendor):
                    continue
                if cond and not _cond_ok(p, cond):
                    continue
                st = compat_state(rules, p["id"], target)
                g = {"verified": 3, "conditional": 2}.get(st, 0)
                if g:
                    out[p["id"]] = max(out.get(p["id"], 0), g)
            continue
        sel, g = e
        for p in products:
            if (callable(sel) and sel(p)) or sel == p["id"] or (isinstance(sel, str) and sel == f"fam:{p['family_id']}"):
                out[p["id"]] = max(out.get(p["id"], 0), g)
    unavailable = {p["id"] for p in products if p.get("price_mode") == "unavailable"}
    out = {k: (min(v, 1) if k in unavailable else v) for k, v in out.items()}
    return dict(sorted(out.items(), key=lambda kv: (-kv[1], kv[0])))


def _cond_ok(p, cond):
    for k, v in cond.items():
        if k.startswith("a_"):
            if str(p["attributes"].get(k[2:])) != str(v):
                return False
        elif p.get("specs", {}).get(k) != v:
            return False
    return True


def build_queries(products, rules, first_id):
    out = []
    for i, (typ, text, freq, entries, expect) in enumerate(P2_QUERIES):
        expect = dict(expect)
        extra = expect.pop("notes_extra", "")
        j = judge(entries, products, rules, expect)
        notes = {"p2_license": "лицензионное правило и количество компаньона",
                 "p2_mpn": "точный парт-номер или модель, без исправления опечаток",
                 "p2_specs": "число с единицей → фильтр",
                 "p2_compat": "совместимость с целевой моделью: verified 3, conditional 2, unknown/incompatible 0",
                 "p2_bundle": "комплексная закупка, шаблон решения",
                 "p2_negative": "отрицательный или пограничный: блок и фильтры не должны срабатывать ошибочно"}[typ]
        out.append({"id": f"q-{first_id + i:04d}", "text": text, "type": typ, "set": "p2", "freq": freq,
                    "client_id": None, "judgments": j, "absent": bool(expect.get("absent")),
                    "notes": notes + (f"; {extra}" if extra else ""), "cluster": f"p2-{i + 1:03d}", "terms": [],
                    "expect": expect})
    return out


# ------------------------------------------------------------------ пары «якорь → кандидат»
def build_pairs(products, rules, relations, lrules, rng):
    by_id = {p["id"]: p for p in products}
    fam_products = defaultdict(list)
    for p in products:
        fam_products[p["family_id"]].append(p["id"])
    cand = defaultdict(list)
    # required — по правилам лицензирования, с проверкой версии
    for anchor in ["ws25-std16", "ws25-dc16", "ws22-std16", "ws22-dc16"]:
        for c in ["wscal25-u1", "wscal25-d1", "wscal25-u5", "wscal25-d5", "rdscal25-u1", "rdscal25-d1"]:
            cand["required"].append((anchor, c, "windows server"))
        for c in CAL22_ALL:
            if anchor.startswith("ws22"):
                cand["required"].append((anchor, c, "windows server 2022"))
            else:
                cand["incompatible"].append((anchor, c, "windows server 2025"))
    cand["required"] += [("sql22-stdsrv", "sql22-cal", "sql server 2022 server cal")]
    for a in ["1csrv-prof64", "1csrv-corp64"]:
        for c in fam_products["1c-client-license"]:
            cand["required"].append((a, c, "сервер 1с"))
    cand["irrelevant"] += [("sql22-std2c", "sql22-cal", "sql server 2022 по ядрам")]
    # совместимость комплектующих
    servers = [p for p in products if p["category_id"] == "server-systems" and p["price_mode"] != "unavailable"]
    parts = [p for p in products if p["category_id"] in ("server-memory", "server-storage", "server-services")]
    for s in servers:
        for c in parts:
            st = compat_state(rules, c["id"], s["model_id"])
            cls = {"verified": "compatible", "conditional": "conditional", "incompatible": "incompatible"}.get(st)
            if cls is None:
                if c["category_id"] == "server-services":
                    continue
                cls = "unknown"
            cand[cls].append((s["id"], c["id"], s["family_name"].lower()))
    # дополнения рабочего места и ИБП
    laptops = [p for p in products if p["category_id"] == "laptops"]
    for lp in laptops:
        for c in ["m365b-bs", "off24-hb", "kes-std10", "mon-dell-p2425h", "mon-dell-p2725h"]:
            cand["complement"].append((lp["id"], c, lp["family_name"].lower()))
        if lp["attributes"]["os"] == "без ОС":
            cand["required"].append((lp["id"], "w11p-oem", lp["family_name"].lower()))
    cand["complement"] += [("sw-eltex-mes2324p", "ups-ippon-spp-2200", "коммутатор poe"),
                           ("sql22-stdsrv", "ws25-std16", "sql server 2022")]
    # старшие редакции
    for r in relations:
        if r["type"] == "upgrade":
            cand["upgrade"].append((r["from_product"], r["to_product"], by_id[r["from_product"]]["family_name"].lower()))
            for x in fam_products[r["from_family"]]:
                if x not in (r["from_product"], r["to_product"]) and by_id[x]["license"]["type"] != "renewal":
                    if by_id[x]["license"]["edition"] == by_id[r["from_product"]]["license"]["edition"]:
                        cand["upgrade"].append((x, r["to_product"], by_id[x]["family_name"].lower()))
    # несвязанные
    pool = [("ws25-std16", "ps-t1y"), ("srv-r750-s4314", "acro-pro1y"), ("nb-e14g5-i5-16-512", "wscal25-u1"),
            ("mem-dell-32g-d4", "off24-hb"), ("mon-dell-p2425h", "sql22-cal"), ("sw-eltex-mes2324b", "kplus-3dev-1y"),
            ("ups-ippon-rt-3000", "m365b-bs"), ("wscal25-u1", "acad-1y"), ("srv-dl380g11-4410y", "fm-std"),
            ("nb-t14g4-i5-16-512", "1cits-prof12"), ("kes-std25", "mem-hpe-32g-d5"), ("off24-hb", "hdd-dell-2400-sas"),
            ("m365b-bp", "svc-dell-ps-r740-3y"), ("srv-vegman-r220-base", "zoom-pro1y"), ("rdscal25-u1", "mon-samsung-g5-27"),
            ("ws22-std16", "nb-pb450g10-i5-8-256"), ("ssd-hpe-960-sata", "cdr-perp"), ("1csrv-prof64", "mon-hp-e24g5"),
            ("mem-samsung-64g-d4", "srv-r740-s4210"), ("ssd-samsung-pm893-960", "srv-dl380g11-4410y"),
            ("sql22-ent2c", "sql22-cal"), ("nb-mbd16-i5-16-512", "ws25-dc16")]
    for a, c in pool:
        cls = "unknown" if c.startswith("srv-") else "irrelevant"
        if cls == "unknown":
            cand[cls].append((c, a, by_id[c]["family_name"].lower()))
        else:
            cand[cls].append((a, c, None))
    target = {"required": 20, "compatible": 25, "conditional": 15, "incompatible": 20, "unknown": 20,
              "complement": 20, "upgrade": 10, "irrelevant": 20}
    out, seen = [], set()
    for cls in ["required", "compatible", "conditional", "incompatible", "unknown", "complement", "upgrade",
                "irrelevant"]:
        items = []
        for x in cand[cls]:
            if (x[0], x[1]) not in seen:
                seen.add((x[0], x[1]))
                items.append(x)
        rng.shuffle(items)
        chosen = items[:target[cls]]
        for a, c, q in sorted(chosen):
            row = {"anchor_product": a, "candidate_product": c, "class": cls}
            if q:
                row["query"] = q
            out.append(row)
    return out


CAL22_ALL = ["wscal-u1", "wscal-d1", "wscal-u5", "wscal-d5", "wscal-rds"]


# ------------------------------------------------------------------ события
ACCOUNTS = [f"acc-{i:03d}" for i in range(1, 41)]
KNOWN = ["client-a", "client-c", "client-d", "client-f"]


def _ts(rng, start, days, weighted):
    day_w = [(d, 1.0 if (start + timedelta(days=d)).weekday() < 5 else 0.3) for d in range(days)]
    hour_w = [(h, w) for h, w in zip(range(5, 18), (1, 4, 7, 9, 9, 6, 7, 9, 8, 7, 5, 3, 1))]
    return start + timedelta(days=weighted(rng, day_w), hours=weighted(rng, hour_w), seconds=rng.randrange(3600))


def _sid(rng):
    return SESSION_PREFIX + "%012x" % rng.getrandbits(48)


def _fmt(t):
    return t.strftime("%Y-%m-%dT%H:%M:%SZ")


def build_orders(rng, products, rules):
    by_id = {p["id"]: p for p in products}
    fam = defaultdict(list)
    for p in products:
        fam[p["family_id"]].append(p)

    def acct():
        return rng.choice(ACCOUNTS + KNOWN)

    def compatible(model, category):
        return [p["id"] for p in products if p["category_id"] == category
                and compat_state(rules, p["id"], model) in ("verified", "conditional")]

    orders = []  # (account, [(pid, qty)])
    # сценарий 1: Windows Server + CAL
    for i in range(28):
        users = rng.choice([5, 10, 15, 20, 25, 30, 50])
        lines = [("ws25-std16", 1)]
        if rng.random() < 0.3:
            lines.append(("ws25-std2", rng.choice([2, 4, 8])))
        if i < 3:
            lines.append(("wscal-u1", users))           # к серверу 2025 купили CAL 2022 — след сломанной связи
        elif users % 5 == 0 and rng.random() < 0.5:
            lines.append(("wscal25-u5", users // 5))
        else:
            lines.append((rng.choice(["wscal25-u1", "wscal25-d1"]), users))
        if rng.random() < 0.3:
            lines.append(("rdscal25-u1", max(1, users // 2)))
        orders.append((acct(), lines))
    for _ in range(6):
        users = rng.choice([5, 10, 20])
        orders.append((acct(), [("ws22-std16", 1), (rng.choice(["wscal-u1", "wscal25-u1"]), users)]))
    for _ in range(5):
        users = rng.choice([10, 15, 25])
        orders.append((acct(), [("sql22-stdsrv", 1), ("sql22-cal", users)]))
    # сценарий 2: сервер + комплектующие (+ ИБП к R750 — устойчивая пара без связи, P2-03)
    planted_accounts = ["acc-002", "acc-011", "acc-019", "acc-024", "acc-031", "client-c", "acc-036"]
    srv_plan = ([("dell-poweredge-r750", a) for a in planted_accounts] + [("dell-poweredge-r750", "acc-011"),
                                                                          ("dell-poweredge-r750", "acc-024")]
                + [("dell-poweredge-r750", acct()) for _ in range(3)]
                + [(m, acct()) for m in ["dell-poweredge-r740"] * 6 + ["hpe-proliant-dl380-gen10"] * 4
                   + ["hpe-proliant-dl380-gen11"] * 7 + ["yadro-vegman-r220-g2"] * 3])
    for k, (model, account) in enumerate(srv_plan):
        servers = [p["id"] for p in fam[model] if p["price_mode"] != "unavailable"]
        lines = [(rng.choice(servers), rng.choice([1, 1, 2]))]
        mem = compatible(model, "server-memory")
        lines.append((rng.choice(mem), rng.choice([2, 4, 8])))
        if rng.random() < 0.8:
            lines.append((rng.choice(compatible(model, "server-storage")), rng.choice([2, 4])))
        if rng.random() < 0.6:
            lines.append((compatible(model, "server-services")[0], 1))
        if model == "dell-poweredge-r750" and k < len(planted_accounts) + 2:
            lines.append((rng.choice(["ups-ippon-rt-3000", "ups-ippon-rt-6000"]), 1))
        orders.append((account, lines))
    # сценарий 3: рабочее место
    laptops = [p for p in products if p["category_id"] == "laptops"]
    for _ in range(50):
        lp = rng.choice(laptops)
        n = rng.choice([1, 2, 3, 5, 5, 10, 10, 15, 20, 25])
        lines = [(lp["id"], n)]
        r = rng.random()
        if r < 0.45:
            lines.append((rng.choice(["m365b-bs", "m365b-bb", "m365b-bp"]), n))
        elif r < 0.75:
            lines.append(("off24-hb", n))
        if rng.random() < 0.5:
            lines.append((rng.choice(["kes-std10", "kes-std25", "kes-std50"]), 1))
        if lp["attributes"]["os"] == "без ОС" and rng.random() < 0.6:
            lines.append(("w11p-oem", n))
        if rng.random() < 0.2:
            lines.append((rng.choice(["mon-dell-p2425h", "mon-dell-p2725h"]), n))
        orders.append((acct(), lines))
    # сеть и питание
    for _ in range(6):
        orders.append((acct(), [("sw-eltex-mes2324p", 1), ("ups-ippon-spp-2200", 1)]))
    # шум: одна пара, повторяемая одним аккаунтом (не проходит порог ≥3 аккаунтов)
    for _ in range(4):
        orders.append((NOISE_ACCOUNT, [(NOISE_PAIR[0], 2), (NOISE_PAIR[1], 1)]))
    # шум: заменители в одном заказе (сравнение ноутбуков) — пара не дополнение
    for a in ["acc-004", "acc-015", "acc-028"]:
        orders.append((a, [("nb-e14g5-i5-16-512", 1), ("nb-pb450g10-i5-8-256", 1)]))
    # одиночные и случайные пары
    hw = [p["id"] for p in products if p.get("kind") == "hardware" and p["price_mode"] not in ("unavailable",)
          and p["category_id"] not in ("server-services",)]
    sw = ["kes-std10", "m365b-bs", "off24-hb", "wscal25-u1", "ws25-std16", "w11p-esd", "veeam-vul10-1y"]
    for _ in range(45):
        lines = [(rng.choice(hw + sw), rng.choice([1, 1, 2, 3, 5]))]
        if rng.random() < 0.2:
            x = rng.choice(hw + sw)
            if x != lines[0][0]:
                lines.append((x, 1))
        orders.append((acct(), lines))
    return orders


def build_events(rng, start, days, weighted, queries, products, legacy, rules):
    by_id = {p["id"]: p for p in products}
    events = []

    def ev(t, sid, client, typ, **kw):
        e = {"ts": _fmt(t), "session_id": sid, "client_id": client, "type": typ}
        e.update(kw)
        events.append(e)

    # поиски по набору p2: ровно freq поисков на запрос
    for q in queries:
        res = legacy.search(q["text"])
        pos = {pid: i + 1 for i, pid in enumerate(res)}
        good = [(pid, pos[pid]) for pid, g in q["judgments"].items() if g >= 2 and pid in pos]
        for _ in range(q["freq"]):
            t, sid = _ts(rng, start, days, weighted), _sid(rng)
            ev(t, sid, None, "search", query=q["text"], results_count=len(res))
            if good and rng.random() < 0.55:
                pid, ps = rng.choice(sorted(good))
                t += timedelta(seconds=rng.randint(4, 40))
                ev(t, sid, None, "click", product_id=pid, position=ps)
                if by_id[pid]["price_on_request"]:
                    if rng.random() < 0.4:
                        t += timedelta(seconds=rng.randint(20, 200))
                        ev(t, sid, None, "price_request", product_id=pid)
                elif rng.random() < 0.3:
                    t += timedelta(seconds=rng.randint(15, 180))
                    ev(t, sid, None, "add_to_cart", product_id=pid)
    # заказы с аккаунтами
    specs = []
    for account, lines in build_orders(rng, products, rules):
        specs.append((_ts(rng, start, days, weighted), account, lines, _sid(rng)))
    specs.sort(key=lambda x: (x[0], x[3]))
    for n, (t, account, lines, sid) in enumerate(specs, 1):
        client = account if account.startswith("client-") else None
        for pid, qty in lines:
            t += timedelta(seconds=rng.randint(10, 90))
            if by_id[pid]["price_on_request"]:
                ev(t, sid, client, "price_request", product_id=pid)
            ev(t, sid, client, "add_to_cart", product_id=pid)
        t += timedelta(seconds=rng.randint(60, 600))
        for pid, qty in lines:
            ev(t, sid, client, "order", product_id=pid, quantity=qty, order_id=f"o-{n:05d}", account_id=account)
    events.sort(key=lambda e: (e["ts"], e["session_id"]))
    return events


def apply_popularity(new_products, p1_events, p2_events):
    weight = {"order": 5, "add_to_cart": 2, "price_request": 2, "click": 1}
    top = Counter()
    for e in p1_events:
        if e["type"] in weight:
            top[e["product_id"]] += weight[e["type"]]
    top_score = max(top.values())
    score = Counter()
    for e in p2_events:
        if e["type"] in weight:
            score[e["product_id"]] += weight[e["type"]]
    for p in new_products:
        s = score.get(p["id"], 0)
        p["popularity"] = min(1.0, round(0.02 + 0.98 * math.log1p(s) / math.log1p(top_score), 3)) if s else 0.01


# ------------------------------------------------------------------ PLANTED.md
def planted_md(stats, n_orders, relations, lrules, products, rules):
    rel_by = {(r["from_family"], r["to_family"], r["type"]): r for r in relations}
    broken = rel_by[("windows-server-2025", "windows-server-cal", "complement")]
    sub_unavail = rel_by[("dell-poweredge-r740", "hpe-proliant-dl380-gen10", "substitute")]
    pp = stats[tuple(sorted(PLANTED_PAIR))]
    unexpl, bonf, n_tested = unexplained_pairs(stats, relations)
    derived = [k for k in unexpl if k != tuple(sorted(PLANTED_PAIR))]
    nz = stats.get(tuple(sorted(("hp-e24-g5", "dlink-dgs-1210"))), {})
    sb = stats.get(tuple(sorted(("lenovo-thinkpad-e14-gen5", "hp-probook-450-g10"))), {})
    by_id = {p["id"]: p for p in products}
    lines = [
        "# Заложено для агента SearchOps Commerce (контур коммерции)",
        "",
        "Файл генерирует `scripts/data/gen_mock_p2.py`; числа ниже посчитаны на текущих данных. Заказы и маржа",
        "синтетические (`catalog.json` → `synthetic`).",
        "",
        "| id | Диагноз | Что заложено | Где | Ожидаемое действие агента |",
        "| --- | --- | --- | --- | --- |",
        f"| P2-01 | `BROKEN_RELATION` | Связь `{broken['id']}`: windows-server-2025 → windows-server-cal (CAL 2022), "
        f"complement, `conditional_required`, review_status approved, evidence E1. Правило `lr-ws25-cal` требует "
        f"`version >= 2025`, у всех позиций windows-server-cal `attributes.version = \"2022\"` | relations.json, "
        f"license_rules.json | `relation_patch`: remove `{broken['id']}` (верная связь на windows-server-cal-2025 уже "
        f"есть); след в заказах — 3 заказа «WS 2025 + CAL 2022» |",
        "| P2-02 | `MISSING_REQUIRED` | Правило `lr-ws25-rds` (RDS CAL 2025 к windows-server-2025) есть, связи "
        "windows-server-2025 → rds-cal-2025 нет | license_rules.json, relations.json | `relation_patch`: add "
        "complement `conditional_required`, conditions `remote_desktop_used`, per_user, source vendor_documentation |",
        f"| P2-03 | `MISSING_RELATION` | Пара семейств {PLANTED_PAIR[0]} ↔ {PLANTED_PAIR[1]} (сервер ↔ ИБП, "
        f"категории server-systems × ups): {pp['a']} совместных заказов из {pp['a'] + pp['b']} заказов R750 и "
        f"{pp['a'] + pp['c']} заказов ИБП, {pp['accounts']} разных аккаунтов, p = {pp['p']:.2e} (односторонний "
        f"точный тест Фишера, {n_orders} заказов с аккаунтом); проходит и поправку Бонферрони на {n_tested} пар "
        f"с ≥{MIN_ACCOUNTS} аккаунтами (порог {bonf:.1e}). Связи нет ни в одну сторону, общих соседей по связям нет, "
        f"не заменители | "
        f"events.jsonl (`order` с `order_id`, `account_id`) | Предложение связи complement, source co_purchase, "
        f"evidence E4, на подтверждение человеку |",
        f"| P2-04 | `BROKEN_RELATION` (снятый товар) | Связь `{sub_unavail['id']}`: dell-poweredge-r740 → "
        f"hpe-proliant-dl380-gen10, substitute, `to_product = srv-dl380g10-4210`; у позиции `price_mode = "
        f"unavailable` (снята с производства) | relations.json, catalog.json | Убрать `to_product` или заменить на "
        f"srv-dl380g10-5218 |",
        "",
        "## Шум, который агент должен отклонить",
        "",
        "| id | Что | Числа | Почему не связь |",
        "| --- | --- | --- | --- |",
        f"| N-01 | hp-e24-g5 ↔ dlink-dgs-1210 | {nz.get('a', 0)} совместных заказа, аккаунтов {nz.get('accounts', 0)} "
        f"({NOISE_ACCOUNT}), p = {nz.get('p', 1):.2e} | Меньше {MIN_ACCOUNTS} аккаунтов: повтор одного клиента |",
        f"| N-02 | lenovo-thinkpad-e14-gen5 ↔ hp-probook-450-g10 | {sb.get('a', 0)} совместных заказа, аккаунтов "
        f"{sb.get('accounts', 0)}, p = {sb.get('p', 1):.2e} | Заменители (связь substitute есть): сравнение, не "
        f"дополнение |",
        "| N-03 | Случайные пары из одиночных заказов | 1–2 совместных заказа | Не значимы по тесту Фишера |",
        "| N-04 | Комплектующие одного сервера (память × диски × гарантия) | значимы | Объяснены общей связью с "
        "сервером-якорем: это «соседи», а не новая связь |",
        f"| N-05 | Производные от P2-03: {', '.join(' ↔ '.join(k) for k in derived)} | значимы, те же заказы R750 | "
        f"Сворачиваются к паре с системой-якорем (P2-03): ИБП покупают к серверу, а не к его памяти |",
        "",
        "## Совместимость: примеры всех состояний",
        "",
        "| Состояние | Пример | Где |",
        "| --- | --- | --- |",
        "| verified | mem-hpe-32g-d4 → hpe-proliant-dl380-gen10 | compatibility_rules.json |",
        "| conditional | mem-dell-32g-d4 → dell-poweredge-r740 («работает на 2933 МТ/с…») | compatibility_rules.json |",
        "| incompatible | mem-hpe-32g-d5 → dell-poweredge-r740 (DDR5 в слоты DDR4) | compatibility_rules.json |",
        "| unknown | ssd-samsung-pm893-960 → любой сервер; mem-kingston-32g-d4 → hpe-proliant-dl380-gen10 | правила нет |",
        "",
        "## Прочее",
        "",
        "- Модель `price_mode`: fixed, from (сервер R750, VEGMAN — цена базовой сборки), quote_required "
        f"(позиций: {sum(p.get('price_mode') == 'quote_required' for p in products)}), unavailable (srv-dl380g10-4210).",
        "- Отрицательные запросы набора p2 (`p2_negative`): голое число, категорийные запросы, отсутствующие товары, "
        "SQL Server по ядрам (CAL не нужны), «windows server 2025 и cal 2022».",
    ]
    return "\n".join(lines) + "\n"


# Разметка контура поиска для новых позиций (пулинг): CAL 2025 и RDS CAL 2025 — тот же ответ, что их аналоги 2022 в запросах
# контура поиска, иначе замороженный test штрафует поиск за верную позицию, которой не было при разметке. Явный год 2022 в
# запросе — не выше 2 (подходящая альтернатива); артикул — не выше 1. Остальные новые позиции к запросам контура поиска
# не относятся (0 по умолчанию). Хэш заморозки считается без этих оценок (validate.py, POOLED_P2).
POOLED_P2 = {"wscal25-u1": "wscal-u1", "wscal25-d1": "wscal-d1", "wscal25-u5": "wscal-u5", "wscal25-d5": "wscal-d5",
             "rdscal25-u1": "wscal-rds", "rdscal25-d1": "wscal-rds"}


def pool_p1_judgments(queries):
    for q in queries:
        j = q["judgments"]
        for new, old in POOLED_P2.items():
            if old not in j or new in j:
                continue
            g = j[old]
            if q["type"] == "sku":
                g = min(g, 1)
            elif "2022" in q["text"]:
                g = min(g, 2)
            j[new] = g


# ------------------------------------------------------------------ точка входа
def extend(seed, start, days, weighted, products, vendors, cats_list, relations, clients, queries, events, aliases,
           legacy_cls):
    """Дописывает контур коммерции. Возвращает (catalog_meta, relations, extra_files: {имя: данные}, p2_events, planted_md)."""
    rng = random.Random(f"{seed}:p2")
    for p in products:
        if p["family_id"] in P1_VERSION:
            p["attributes"]["version"] = P1_VERSION[p["family_id"]]
    vendors += [dict(id=i, name=n, origin=o) for i, n, o in VENDORS_P2]
    cats_list += [dict(id=i, name=n, parent_id=pid) for i, n, pid in CATEGORIES_P2]
    vendors_by_id = {v["id"]: v for v in vendors}
    new = build_products(random.Random(f"{seed}:p2-margin"), vendors_by_id)
    products += new
    rules = build_compat(products)
    rels, _ = build_relations(relations)
    pool_p1_judgments(queries)
    p2q = build_queries(products, rules, len(queries) + 1)
    queries += p2q
    cats = {c["id"]: c for c in cats_list}
    legacy = legacy_cls(products, vendors, cats, aliases)
    p2_events = build_events(random.Random(f"{seed}:p2-events"), start, days, weighted, p2q, products, legacy, rules)
    apply_popularity(new, events, p2_events)
    fam_of = {p["id"]: p["family_id"] for p in products}
    stats, n_orders = copurchase_stats([e for e in p2_events if e["type"] == "order"], fam_of)
    pairs = build_pairs(products, rules, rels, LICENSE_RULES, random.Random(f"{seed}:p2-pairs"))
    extra = {
        "license_rules.json": LICENSE_RULES,
        "compatibility_rules.json": rules,
        "attribute_definitions.json": build_attr_defs(),
        "solution_templates.json": SOLUTION_TEMPLATES,
        "companion_pairs.json": pairs,
    }
    md = planted_md(stats, n_orders, rels, LICENSE_RULES, products, rules)
    return {"version": CATALOG_VERSION, "synthetic": SYNTHETIC}, rels, extra, p2_events, md
