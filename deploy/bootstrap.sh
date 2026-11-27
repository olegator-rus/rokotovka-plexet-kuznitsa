#!/usr/bin/env bash
# Развёртывание стенда с нуля из этого репозитория: фид → импорт → индекс → запуск.
#
#   DOMAIN=search.example.ru deploy/bootstrap.sh <путь к фиду Mindbox.yml.xml или .xml.gz> [--demo]
#
# Нужны только Docker с Compose и доступ в интернет (образы, веса моделей с Hugging Face, сертификат).
# Что делает:
#   1. создаёт .env в корне, если его нет: случайные пароль базы и токен бэк-офиса, DOMAIN из окружения;
#   2. кладёт фид в data/softline/feed/ и конвертирует его (scripts/data/softline_feed.py, в контейнере python);
#   3. собирает образы, поднимает PostgreSQL, OpenSearch и сервис моделей, ждёт готовности моделей;
#   4. импортирует каталог в базу и индекс (с векторами) и строит словарь подсказок;
#   5. поднимает API и веб-сервер и проверяет /api/health;
#   6. с ключом --demo: проигрывает тестовый трафик SearchOps и прогоняет наборы оценки (экраны бэк-офиса и
#      «Доказательств» наполняются числами). Часть этих шагов вызывает языковую модель — нужен LLM_API_KEY в .env.
# Повторный запуск безопасен: .env не перезаписывается, импорт пересобирает каталог и индекс заново.
set -euo pipefail

FEED="${1:-}"
DEMO="${2:-}"
ROOT="$(cd "$(dirname "$0")/.." && pwd)"
cd "$ROOT"
DC=(docker compose --env-file .env -f deploy/docker-compose.yml)

if [[ -z "$FEED" || ! -f "$FEED" ]]; then
  echo "Укажите файл фида: DOMAIN=<домен> deploy/bootstrap.sh <путь к Mindbox.yml.xml|.xml.gz> [--demo]" >&2
  exit 1
fi

echo "== 1. Настройки (.env)"
if [[ ! -f .env ]]; then
  : "${DOMAIN:?задайте DOMAIN — домен стенда; без своего домена подойдёт <IP через дефисы>.sslip.io}"
  ( umask 077; cat > .env <<ENV
DOMAIN=${DOMAIN}
ACME_EMAIL=${ACME_EMAIL:-admin@${DOMAIN}}
HTTP_PORT=${HTTP_PORT:-}
HTTPS_PORT=${HTTPS_PORT:-}
POSTGRES_DB=kuznica
POSTGRES_USER=kuznica
POSTGRES_PASSWORD=$(openssl rand -hex 24)
BACKOFFICE_TOKEN=$(openssl rand -hex 32)
TRUSTED_PROXIES=172.16.0.0/12
AUTH_REQUIRED=on
DEMO_LOGIN_BUTTONS=on
# Языковая модель (OpenAI-совместимый API). Без ключа поиск работает на правилах, словаре и векторах;
# трудные запросы, ИИ-консультант, разбор ТЗ и агент SearchOps модель требуют.
LLM_PROVIDER=${LLM_PROVIDER:-}
LLM_BASE_URL=${LLM_BASE_URL:-}
LLM_MODEL=${LLM_MODEL:-}
LLM_FOLDER_ID=${LLM_FOLDER_ID:-}
LLM_API_KEY=${LLM_API_KEY:-}
ENV
  )
  echo "создан .env (пароль базы и токен бэк-офиса сгенерированы)"
else
  echo ".env уже есть — не трогаю"
fi

echo "== 2. Фид"
mkdir -p data/softline/feed
if [[ "$FEED" == *.gz ]]; then
  gunzip -c "$FEED" > data/softline/feed/Mindbox.yml.xml
elif [[ "$(cd "$(dirname "$FEED")" && pwd)/$(basename "$FEED")" != "$ROOT/data/softline/feed/Mindbox.yml.xml" ]]; then
  cp "$FEED" data/softline/feed/Mindbox.yml.xml
fi
docker run --rm --user "$(id -u):$(id -g)" -v "$ROOT":/w -w /w python:3.13-slim python scripts/data/softline_feed.py
# API в контейнере работает от непривилегированного пользователя: каталог данных должен читаться всеми
chmod -R a+rX data 2>/dev/null || true
ls -lh data/softline/feed | sed 1d

echo "== 3. Образы, база, индекс, модели"
"${DC[@]}" build
"${DC[@]}" up -d db opensearch ml
echo "жду сервис моделей (первый запуск скачивает около 4,5 ГБ весов)…"
ML_ID="$("${DC[@]}" ps -q ml)"
for _ in $(seq 1 360); do
  [[ "$(docker inspect -f '{{.State.Health.Status}}' "$ML_ID")" == "healthy" ]] && break
  sleep 10
done
[[ "$(docker inspect -f '{{.State.Health.Status}}' "$ML_ID")" == "healthy" ]] || { echo "сервис моделей не поднялся: docker logs $ML_ID" >&2; exit 1; }

echo "== 4. Импорт каталога и подсказки"
"${DC[@]}" run --rm api import
"${DC[@]}" run --rm api suggest-build

echo "== 5. API и веб-сервер"
"${DC[@]}" up -d api web
API_ID="$("${DC[@]}" ps -q api)"
for _ in $(seq 1 60); do
  [[ "$(docker inspect -f '{{.State.Health.Status}}' "$API_ID")" == "healthy" ]] && break
  sleep 5
done
"${DC[@]}" exec -T api wget -qO- http://localhost:8080/api/health; echo

if [[ "$DEMO" == "--demo" ]]; then
  echo "== 6. Демо-данные: тестовый трафик и оценки"
  "${DC[@]}" run --rm -e TRAFFIC_API_URL=http://api:8080 api traffic --flow /data/softline/searchops_flow.jsonl --duration 15m --panel 0
  "${DC[@]}" run --rm api eval --set etalon,tz,neighbors,understanding --cold --configs BM25,B1,B2,live
  "${DC[@]}" run --rm api eval --set closed --file /data/softline/eval/closed.jsonl --cold --configs BM25,B1,B2,live
fi

DOMAIN_NOW="$(sed -n 's/^DOMAIN=//p' .env)"
echo "Готово: https://${DOMAIN_NOW}  (вход в бэк-офис — демо-кнопки на /login)"
