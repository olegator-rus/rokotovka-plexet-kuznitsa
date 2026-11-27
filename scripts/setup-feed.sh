#!/usr/bin/env bash
# Локальный стенд на фиде заказчика одной командой.
#
#   scripts/setup-feed.sh <путь к Mindbox.yml.gz.xml> [<дамп базы .sql.gz>]
#
# Что делает:
#   1. кладёт фид в data/softline/feed/ (папка в .gitignore) и конвертирует его (scripts/data/softline_feed.py);
#   2. создаёт backend/.env через `make env`, если его нет, и дописывает DATA_SOURCE/DATA_MAPPING, если их нет;
#   3. поднимает Postgres и OpenSearch (`make db`) и ML-сервис (ml/run.sh start);
#   4. если передан дамп — восстанавливает базу (история агента, журнал поиска, оценки);
#   5. импортирует каталог в базу и индекс (векторы — на ML-сервисе, ~10 мин при первом импорте) и строит подсказки.
#
# Потом: `make dev` (API 127.0.0.1:8088 + витрина localhost:5177).
set -euo pipefail

FEED="${1:-}"
DUMP="${2:-}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"

if [[ -z "$FEED" || ! -f "$FEED" ]]; then
  echo "Укажите файл фида: scripts/setup-feed.sh <путь к Mindbox.yml.gz.xml> [<дамп .sql.gz>]" >&2
  exit 1
fi

echo "== 1. Фид"
mkdir -p data/softline/feed
if [[ "$FEED" == *.gz && "$(file -b "$FEED")" == gzip* ]]; then
  gunzip -c "$FEED" > data/softline/feed/Mindbox.yml.xml
else
  cp "$FEED" data/softline/feed/Mindbox.yml.xml
fi
python3 scripts/data/softline_feed.py

echo "== 2. backend/.env"
[[ -f backend/.env ]] || make env
for kv in "DATA_SOURCE=generic" "DATA_MAPPING=../data/softline/mapping.yaml"; do
  key="${kv%%=*}"
  if ! grep -q "^${key}=" backend/.env; then
    echo "$kv" >> backend/.env
    echo "   добавлено: $key"
  fi
done

echo "== 3. Postgres, OpenSearch, ML"
make db
ml/run.sh start

if [[ -n "$DUMP" ]]; then
  echo "== 4. Восстановление базы из дампа"
  [[ -f "$DUMP" ]] || { echo "нет файла дампа: $DUMP" >&2; exit 1; }
  DB_CONTAINER="$(docker ps --format '{{.Names}}' | grep -E 'kuznica.*db' | head -1)"
  [[ -n "$DB_CONTAINER" ]] || { echo "не найден контейнер базы" >&2; exit 1; }
  docker exec "$DB_CONTAINER" sh -c 'psql -U "$POSTGRES_USER" -d "$POSTGRES_DB" -c "drop schema public cascade; create schema public;"' >/dev/null
  gunzip -c "$DUMP" | docker exec -i "$DB_CONTAINER" sh -c 'psql -q -U "$POSTGRES_USER" -d "$POSTGRES_DB"' >/dev/null
  echo "   база восстановлена"
fi

echo "== 5. Импорт каталога и подсказки"
cd backend
DATA_SOURCE=generic DATA_MAPPING=../data/softline/mapping.yaml go run . import || \
  echo "   (если упал только «импорт логов» в конце — это не страшно: каталог к тому моменту импортирован)"
DATA_SOURCE=generic DATA_MAPPING=../data/softline/mapping.yaml go run . suggest-build

echo
echo "Готово. Запуск: make dev  → витрина http://localhost:5177"
