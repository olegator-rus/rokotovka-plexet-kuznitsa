#!/usr/bin/env python3
"""Копия поискового индекса между двумя OpenSearch: настройки, разметка (с _meta), документы и алиас.

    python3 -I scripts/deploy/copy_index.py --src http://127.0.0.1:9201 --dst http://127.0.0.1:19200 [--alias products]

Нужна, чтобы стенд получил тот же индекс, что и локальная сборка (те же векторы и версия каталога), без
повторного импорта и пересчёта эмбеддингов. Адрес стенда — через SSH-туннель; наружу OpenSearch не открыт.
"""
import argparse
import json
import sys
import urllib.request

KEEP_SETTINGS = ("analysis", "number_of_shards", "number_of_replicas", "knn", "knn.algo_param.ef_search", "refresh_interval", "max_ngram_diff", "mapping")


def call(method, url, body=None, ndjson=False):
    data = None
    headers = {}
    if body is not None:
        data = body if isinstance(body, bytes) else json.dumps(body).encode()
        headers["Content-Type"] = "application/x-ndjson" if ndjson else "application/json"
    req = urllib.request.Request(url, data=data, method=method, headers=headers)
    with urllib.request.urlopen(req, timeout=300) as r:
        return json.load(r)


def main() -> int:
    ap = argparse.ArgumentParser()
    ap.add_argument("--src", required=True)
    ap.add_argument("--dst", required=True)
    ap.add_argument("--alias", default="products")
    ap.add_argument("--batch", type=int, default=1000)
    a = ap.parse_args()

    aliases = call("GET", f"{a.src}/_alias/{a.alias}")
    index = next(iter(aliases))
    info = call("GET", f"{a.src}/{index}")[index]
    settings = {k: v for k, v in info["settings"]["index"].items() if k in KEEP_SETTINGS}
    settings["number_of_replicas"] = "0"
    body = {"settings": {"index": settings}, "mappings": info["mappings"]}
    try:
        call("DELETE", f"{a.dst}/{index}")
    except Exception:
        pass
    call("PUT", f"{a.dst}/{index}", body)
    total = call("GET", f"{a.src}/{index}/_count")["count"]
    print(f"индекс {index}: {total} документов", flush=True)

    page = call("POST", f"{a.src}/{index}/_search?scroll=5m", {"size": a.batch, "sort": ["_doc"], "query": {"match_all": {}}})
    done = 0
    while page["hits"]["hits"]:
        lines = []
        for h in page["hits"]["hits"]:
            lines.append(json.dumps({"index": {"_index": index, "_id": h["_id"]}}))
            lines.append(json.dumps(h["_source"], ensure_ascii=False))
        res = call("POST", f"{a.dst}/_bulk", ("\n".join(lines) + "\n").encode(), ndjson=True)
        if res.get("errors"):
            bad = next(i for i in res["items"] if i["index"].get("error"))
            print("ошибка записи:", json.dumps(bad, ensure_ascii=False)[:400], file=sys.stderr)
            return 1
        done += len(page["hits"]["hits"])
        print(f"  {done}/{total}", flush=True)
        page = call("POST", f"{a.src}/_search/scroll", {"scroll": "5m", "scroll_id": page["_scroll_id"]})
    call("POST", f"{a.dst}/{index}/_refresh")
    call("POST", f"{a.dst}/_aliases", {"actions": [{"add": {"index": index, "alias": a.alias}}]})
    got = call("GET", f"{a.dst}/{index}/_count")["count"]
    print(f"готово: на стенде {got} из {total}, алиас {a.alias} → {index}")
    return 0 if got == total else 1


if __name__ == "__main__":
    sys.exit(main())
