#!/usr/bin/env bash
# Фид заказчика на чистом клоне одной командой.
#
#   scripts/data/fetch_feed.sh <файл.xml.gz>   # архив фида заказчика
#
# Что делает:
#   1. берёт архив из аргумента;
#   2. сверяет sha256 распакованного XML с data/softline/feed.sha256, если он есть;
#   3. кладёт XML в data/softline/feed/ (папка в .gitignore) и пересобирает производные *.jsonl
#      конвертером scripts/data/softline_feed.py.
# База, индекс и ML здесь не трогаются: дальше scripts/setup-feed.sh или шаги из README.
#
# Обновить архив и сумму (у того, у кого новый фид):
#   gzip -9 -c data/softline/feed/Mindbox.yml.xml > Mindbox.yml.xml.gz
#   shasum -a 256 data/softline/feed/Mindbox.yml.xml | awk '{print $1}' > data/softline/feed.sha256
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

SUM_FILE="data/softline/feed.sha256"
OUT_DIR="data/softline/feed"
PY="${PYTHON:-$(command -v python3.13 || command -v python3)}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

SRC="${1:-}"; [[ -n "$SRC" ]] || { echo "укажи архив фида: scripts/data/fetch_feed.sh <Mindbox.yml.xml.gz>" >&2; exit 1; }
[[ -f "$SRC" ]] || { echo "нет файла: $SRC" >&2; exit 1; }

echo "== Распаковываю"
if [[ "$(file -b "$SRC")" == gzip* ]]; then
  gunzip -c "$SRC" > "$TMP/Mindbox.yml.xml"
else
  cp "$SRC" "$TMP/Mindbox.yml.xml"
fi

echo "== Сверяю sha256"
GOT="$(shasum -a 256 "$TMP/Mindbox.yml.xml" | awk '{print $1}')"
if [[ -f "$SUM_FILE" ]]; then
  WANT="$(tr -d '[:space:]' < "$SUM_FILE")"
  if [[ "$GOT" != "$WANT" ]]; then
    echo "sha256 не совпадает: получено $GOT, ожидалось $WANT ($SUM_FILE)" >&2
    exit 1
  fi
  echo "   совпадает: $GOT"
else
  echo "   $SUM_FILE нет в репозитории, сверять не с чем; получено $GOT" >&2
fi

mkdir -p "$OUT_DIR"
mv "$TMP/Mindbox.yml.xml" "$OUT_DIR/Mindbox.yml.xml"

echo "== Конвертирую в *.jsonl"
"$PY" scripts/data/softline_feed.py

echo
echo "Готово: $OUT_DIR. Дальше: scripts/setup-feed.sh $OUT_DIR/Mindbox.yml.xml или шаги из README (make db, make import)."
