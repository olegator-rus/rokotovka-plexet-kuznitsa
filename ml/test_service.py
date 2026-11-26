"""Проверка запущенного сервиса (ml/run.sh start). Только стандартная библиотека.

    python ml/test_service.py          # свой раннер
    python -m pytest ml/test_service.py  # если установлен pytest

Адрес — ML_URL (по умолчанию http://127.0.0.1:8089).
"""

import json
import math
import os
import sys
import urllib.error
import urllib.request

URL = os.environ.get("ML_URL", "http://127.0.0.1:8089").rstrip("/")

AUTOCAD = "Autodesk AutoCAD LT, подписка на 1 год. САПР для проектирования и выпуска документации в 2D."
KASPERSKY = "Kaspersky Internet Security, 2 устройства, 1 год. Защита компьютера от вирусов и фишинга."
OFFICE = "Microsoft 365 бизнес стандарт: Word, Excel, PowerPoint и почта Outlook для офиса."
ONE_C = "1С:Бухгалтерия 8 ПРОФ. Учёт, зарплата и отчётность для малого бизнеса."


def call(method: str, path: str, body=None) -> tuple[int, object]:
    data = None if body is None else json.dumps(body).encode()
    req = urllib.request.Request(URL + path, data=data, method=method, headers={"Content-Type": "application/json"})
    try:
        with urllib.request.urlopen(req, timeout=60) as resp:
            raw = resp.read()
            return resp.status, json.loads(raw) if raw else None
    except urllib.error.HTTPError as e:
        raw = e.read()
        return e.code, json.loads(raw) if raw else None


def embed(texts):
    status, vecs = call("POST", "/embed", {"inputs": texts, "normalize": True})
    assert status == 200, (status, vecs)
    return vecs


def cos(a, b) -> float:
    return sum(x * y for x, y in zip(a, b))


def test_health():
    status, _ = call("GET", "/health")
    assert status == 200


def test_embed_normalized_1024():
    vecs = embed([AUTOCAD, KASPERSKY, "винда 11 про"])
    assert len(vecs) == 3
    for v in vecs:
        assert len(v) == 1024
        assert abs(math.sqrt(sum(x * x for x in v)) - 1.0) < 1e-3


def test_embed_single_string():
    vecs = embed("касперский на 10 пк")
    assert len(vecs) == 1 and len(vecs[0]) == 1024


def test_embed_deterministic_across_batches():
    alone = embed([OFFICE])[0]
    in_batch = embed([AUTOCAD, "короткий", OFFICE, KASPERSKY * 5])[2]
    assert cos(alone, in_batch) > 0.999


def test_semantic_drawing_closer_to_autocad():
    q, autocad, kaspersky = embed(["программа для чертежей", AUTOCAD, KASPERSKY])
    assert cos(q, autocad) > cos(q, kaspersky), (cos(q, autocad), cos(q, kaspersky))


def test_semantic_antivirus_closer_to_kaspersky():
    q, autocad, kaspersky = embed(["антивирус для офиса", AUTOCAD, KASPERSKY])
    assert cos(q, kaspersky) > cos(q, autocad), (cos(q, kaspersky), cos(q, autocad))


def test_rerank_puts_relevant_first():
    texts = [KASPERSKY, OFFICE, AUTOCAD, ONE_C]
    status, ranked = call("POST", "/rerank", {"query": "программа для чертежей", "texts": texts, "raw_scores": False})
    assert status == 200, ranked
    assert sorted(r["index"] for r in ranked) == [0, 1, 2, 3]
    assert ranked[0]["index"] == 2, ranked
    scores = [r["score"] for r in ranked]
    assert scores == sorted(scores, reverse=True)
    assert all(0.0 <= s <= 1.0 for s in scores)


def test_rerank_raw_scores_same_order():
    texts = [KASPERSKY, OFFICE, AUTOCAD, ONE_C]
    _, prob = call("POST", "/rerank", {"query": "бухгалтерия для ип", "texts": texts})
    status, raw = call("POST", "/rerank", {"query": "бухгалтерия для ип", "texts": texts, "raw_scores": True})
    assert status == 200
    assert [r["index"] for r in raw] == [r["index"] for r in prob]
    assert raw[0]["index"] == 3, raw


def test_long_input_is_truncated():
    vecs = embed(["лицензия " * 3000])
    assert len(vecs[0]) == 1024


def test_errors_tei_shape():
    status, body = call("POST", "/embed", {"inputs": []})
    assert status == 400 and body["error_type"] == "Empty", (status, body)
    status, body = call("POST", "/embed", {"inputs": ["x"] * 65})
    assert status == 413 and body["error_type"] == "Validation", (status, body)
    status, body = call("POST", "/rerank", {"query": "x"})
    assert status == 422 and "error" in body, (status, body)


if __name__ == "__main__":
    tests = [(name, fn) for name, fn in list(globals().items()) if name.startswith("test_") and callable(fn)]
    failed = 0
    for name, fn in tests:
        try:
            fn()
            print(f"PASS {name}")
        except Exception as e:  # noqa: BLE001
            failed += 1
            print(f"FAIL {name}: {e!r}")
    print(f"{len(tests) - failed}/{len(tests)} passed")
    sys.exit(1 if failed else 0)
