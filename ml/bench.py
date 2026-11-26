"""Замер запущенного сервиса: загрузка, память, задержки /embed (1 и 64) и /rerank (query + 20).

    python ml/bench.py                 # против ML_URL (по умолчанию http://127.0.0.1:8089), PID из ml/.pid
    python ml/bench.py --json          # то же, машиночитаемо
    ml/.venv/bin/python ml/bench.py --precision   # fp16 против fp32 в одном процессе (грузит модели сам)

Только стандартная библиотека, кроме режима --precision.
"""

import argparse
import json
import os
import re
import statistics
import subprocess
import sys
import time
import urllib.request

HERE = os.path.dirname(os.path.abspath(__file__))
URL = os.environ.get("ML_URL", "http://127.0.0.1:8089").rstrip("/")

PRODUCTS = [
    ("Kaspersky Internet Security", "Антивирус и защита платежей для домашних компьютеров. Блокирует фишинговые сайты, проверяет загрузки и защищает веб-камеру от слежки."),
    ("Kaspersky Endpoint Security для бизнеса", "Защита рабочих станций и файловых серверов компании с централизованным управлением. Контроль приложений, устройств и веб-доступа."),
    ("Dr.Web Security Space", "Комплексная защита от вирусов, шифровальщиков и спама. Родительский контроль и защита от эксплойтов для Windows и macOS."),
    ("Autodesk AutoCAD", "Система автоматизированного проектирования для двухмерного и трёхмерного черчения. Выпуск рабочей документации, библиотеки блоков и аннотаций."),
    ("КОМПАС-3D", "Российская САПР для машиностроительного и строительного проектирования. Трёхмерное моделирование деталей и сборок, оформление чертежей по ЕСКД."),
    ("nanoCAD Plus", "Отечественная платформа для проектирования и выпуска документации в формате DWG. Поддержка СПДС и командной работы."),
    ("Microsoft Office для дома и бизнеса", "Классические приложения Word, Excel, PowerPoint и Outlook. Работа с документами, таблицами и презентациями на одном компьютере."),
    ("МойОфис Стандартный", "Российский офисный пакет: текстовый редактор, табличный редактор и презентации. Совместим с форматами Microsoft Office, входит в реестр."),
    ("Р7-Офис Профессиональный", "Офисный пакет для совместного редактирования документов, таблиц и презентаций. Подходит для госзаказчиков, работает на Linux."),
    ("1С:Бухгалтерия 8 ПРОФ", "Учёт хозяйственной деятельности, расчёт налогов и подготовка регламентированной отчётности. Для организаций и индивидуальных предпринимателей."),
    ("1С:Зарплата и управление персоналом", "Расчёт заработной платы, кадровый учёт и отчётность в фонды. Учёт отпусков, больничных и штатного расписания."),
    ("Windows 11 Pro", "Операционная система для бизнеса с шифрованием BitLocker, удалённым рабочим столом и управлением групповыми политиками."),
    ("Astra Linux Special Edition", "Российская защищённая операционная система для серверов и рабочих станций. Сертифицирована ФСТЭК, входит в реестр отечественного ПО."),
    ("Adobe Photoshop", "Растровый графический редактор для обработки фотографий, ретуши и создания макетов. Слои, маски и инструменты на основе ИИ."),
    ("Veeam Backup & Replication", "Резервное копирование и восстановление виртуальных машин, физических серверов и облачных нагрузок. Репликация и мгновенное восстановление."),
    ("Bitrix24 Энтерпрайз", "Корпоративный портал с CRM, задачами, документами и чатами. Коробочная версия для установки на собственный сервер компании."),
]
LICENSES = ["подписка на 1 год, 1 устройство", "подписка на 1 год, 5 устройств", "бессрочная лицензия, 1 пользователь", "продление на 2 года, 10 устройств"]

CARDS = [f"{title}, {lic}. {desc}" for title, desc in PRODUCTS for lic in LICENSES]
QUERY = "программа для чертежей на 5 рабочих мест"


def post(path: str, body: dict):
    req = urllib.request.Request(URL + path, data=json.dumps(body).encode(), method="POST", headers={"Content-Type": "application/json"})
    with urllib.request.urlopen(req, timeout=120) as resp:
        return json.loads(resp.read())


def get(path: str):
    with urllib.request.urlopen(URL + path, timeout=10) as resp:
        raw = resp.read()
        return resp.status, json.loads(raw) if raw else None


def timed(fn, runs: int, warmup: int = 2) -> dict:
    for _ in range(warmup):
        fn()
    samples = []
    for _ in range(runs):
        t0 = time.perf_counter()
        fn()
        samples.append((time.perf_counter() - t0) * 1000)
    samples.sort()
    return {
        "runs": runs,
        "p50_ms": round(statistics.median(samples), 1),
        "p95_ms": round(samples[min(len(samples) - 1, int(round(0.95 * (len(samples) - 1))))], 1),
        "mean_ms": round(statistics.fmean(samples), 1),
        "min_ms": round(samples[0], 1),
    }


def memory(pid: int | None) -> dict:
    if not pid:
        return {}
    out = {}
    rss = subprocess.run(["ps", "-o", "rss=", "-p", str(pid)], capture_output=True, text=True).stdout.strip()
    if rss:
        out["rss_mb"] = round(int(rss) / 1024)
    try:
        fp = subprocess.run(["footprint", "-p", str(pid)], capture_output=True, text=True, timeout=30).stdout
        m = re.search(r"phys_footprint:\s*([\d.]+)\s*([KMG])B", fp)
        if m:
            out["phys_footprint_mb"] = round(float(m.group(1)) * {"K": 1 / 1024, "M": 1, "G": 1024}[m.group(2)])
    except (OSError, subprocess.SubprocessError):
        pass
    return out


def read_pid() -> int | None:
    try:
        with open(os.path.join(HERE, ".pid")) as f:
            return int(f.read().strip())
    except (OSError, ValueError):
        return None


def bench_service(args) -> dict:
    status, _ = get("/health")
    assert status == 200, status
    _, info = get("/info")
    pid = args.pid or read_pid()
    report = {"info": info, "memory_before": memory(pid)}

    one = post("/embed", {"inputs": [QUERY]})
    batch = post("/embed", {"inputs": CARDS})
    report["dim"] = len(one[0])
    report["batch_vectors"] = len(batch)
    report["norm"] = round(sum(x * x for x in batch[0]) ** 0.5, 4)

    report["embed_1"] = timed(lambda: post("/embed", {"inputs": [QUERY]}), runs=args.runs)
    report["embed_64"] = timed(lambda: post("/embed", {"inputs": CARDS}), runs=max(5, args.runs // 3))
    texts20 = CARDS[1::3][:20]
    report["rerank_20"] = timed(lambda: post("/rerank", {"query": QUERY, "texts": texts20}), runs=max(5, args.runs // 2))
    top = post("/rerank", {"query": QUERY, "texts": texts20})[:3]
    report["rerank_top3"] = [{"score": round(r["score"], 4), "text": texts20[r["index"]][:60]} for r in top]
    report["memory_after"] = memory(pid)
    return report


def print_report(r: dict) -> None:
    info = r["info"] or {}
    print(f"device={info.get('device')} dtype={info.get('model_dtype')} micro_batch={info.get('micro_batch')} dim={r['dim']} norm={r['norm']}")
    print(f"load_seconds={info.get('load_seconds')}  mps_driver_mb={info.get('mps_driver_mb')}")
    print(f"memory before={r['memory_before']} after={r['memory_after']}")
    for key, label in (("embed_1", "/embed x1"), ("embed_64", "/embed x64 cards"), ("rerank_20", "/rerank q+20")):
        s = r[key]
        print(f"{label:18} p50 {s['p50_ms']:7.1f} ms  p95 {s['p95_ms']:7.1f} ms  mean {s['mean_ms']:7.1f} ms  (n={s['runs']})")
    print("rerank top3:", r["rerank_top3"])


def bench_precision() -> None:
    sys.path.insert(0, HERE)
    os.environ.setdefault("HF_HUB_OFFLINE", "1")
    import torch

    from embedder.engine import Engine

    queries = ["программа для чертежей", "антивирус на 10 пк", "офисный пакет отечественный", "бухгалтерия для ип"]
    results = {}
    for dtype in ("fp32", "fp16"):
        t0 = time.perf_counter()
        eng = Engine(device="auto", dtype=dtype)
        load = time.perf_counter() - t0
        vecs = eng.embed(queries + CARDS)
        scores = [eng.rerank(q, CARDS[::3][:20]) for q in queries]
        t0 = time.perf_counter()
        for _ in range(5):
            eng.embed(CARDS)
        e64 = (time.perf_counter() - t0) / 5 * 1000
        results[dtype] = {"vecs": vecs, "scores": scores, "load_s": round(load, 1), "embed64_ms": round(e64, 1), "device": eng.device}
        del eng
        if torch.backends.mps.is_available():
            torch.mps.empty_cache()

    a, b = results["fp32"]["vecs"], results["fp16"]["vecs"]
    cos = (a * b).sum(dim=1)
    nq = len(queries)
    sim32 = a[:nq] @ a[nq:].T
    sim16 = b[:nq] @ b[nq:].T
    top10_same = [len(set(sim32[i].topk(10).indices.tolist()) & set(sim16[i].topk(10).indices.tolist())) for i in range(nq)]
    max_score_diff = max(abs(x - y) for s32, s16 in zip(results["fp32"]["scores"], results["fp16"]["scores"]) for x, y in zip(s32, s16))
    rr_order_same = [
        sorted(range(20), key=lambda i: -s32[i])[:5] == sorted(range(20), key=lambda i: -s16[i])[:5]
        for s32, s16 in zip(results["fp32"]["scores"], results["fp16"]["scores"])
    ]
    print(f"device={results['fp16']['device']}")
    print(f"embed cos(fp16, fp32): min {cos.min():.5f} mean {cos.mean():.5f}")
    print(f"dense top-10 overlap per query (of 10): {top10_same}")
    print(f"rerank max |score16 - score32|: {max_score_diff:.4f}; top-5 order identical: {rr_order_same}")
    for dtype in ("fp32", "fp16"):
        r = results[dtype]
        print(f"{dtype}: load {r['load_s']} s, embed x64 {r['embed64_ms']} ms (in-process)")


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--json", action="store_true")
    ap.add_argument("--runs", type=int, default=30)
    ap.add_argument("--pid", type=int)
    ap.add_argument("--precision", action="store_true")
    args = ap.parse_args()
    if args.precision:
        bench_precision()
        return 0
    report = bench_service(args)
    if args.json:
        print(json.dumps(report, ensure_ascii=False, indent=2))
    else:
        print_report(report)
    return 0


if __name__ == "__main__":
    sys.exit(main())
