#!/usr/bin/env bash
# Фид заказчика на чистом клоне одной командой.
#
#   scripts/data/fetch_feed.sh                 # скачать архив из релиза приватной репы (gh release download)
#   scripts/data/fetch_feed.sh <файл.xml.gz>   # взять архив, присланный напарником (общая папка, мессенджер)
#
# Что делает:
#   1. берёт Mindbox.yml.xml.gz (из релиза FEED_RELEASE или из аргумента);
#   2. сверяет sha256 распакованного XML с data/softline/feed.sha256 (файл в git);
#   3. кладёт XML в data/softline/feed/ (папка в .gitignore) и пересобирает производные *.jsonl
#      конвертером scripts/data/softline_feed.py.
# База, индекс и ML здесь не трогаются: дальше scripts/setup-feed.sh или шаги из README.
#
# Переменные: FEED_REPO (репозиторий с релизом фида, обязателен для скачивания), FEED_RELEASE (по умолчанию feed),
# FEED_ASSET (по умолчанию Mindbox.yml.xml.gz), PYTHON (по умолчанию python3.13, иначе python3).
#
# Подготовить архив и сумму (у того, у кого фид уже есть):
#   gzip -9 -c data/softline/feed/Mindbox.yml.xml > Mindbox.yml.xml.gz
#   shasum -a 256 data/softline/feed/Mindbox.yml.xml | awk '{print $1}' > data/softline/feed.sha256
set -euo pipefail

ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"

FEED_REPO="${FEED_REPO:-}"
FEED_RELEASE="${FEED_RELEASE:-feed}"
FEED_ASSET="${FEED_ASSET:-Mindbox.yml.xml.gz}"
SUM_FILE="data/softline/feed.sha256"
OUT_DIR="data/softline/feed"
PY="${PYTHON:-$(command -v python3.13 || command -v python3)}"

TMP="$(mktemp -d)"
trap 'rm -rf "$TMP"' EXIT

SRC="${1:-}"
if [[ -z "$SRC" ]]; then
  [[ -n "$FEED_REPO" ]] || { echo "укажи FEED_REPO или путь к архиву аргументом" >&2; exit 1; }; command -v gh >/dev/null || { echo "нужен gh (GitHub CLI) или путь к архиву аргументом" >&2; exit 1; }
  echo "== Скачиваю $FEED_ASSET из релиза $FEED_RELEASE ($FEED_REPO)"
  gh release download "$FEED_RELEASE" --repo "$FEED_REPO" --pattern "$FEED_ASSET" --dir "$TMP"
  SRC="$TMP/$FEED_ASSET"
fi
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
