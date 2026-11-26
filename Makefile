# Makefile — обвязка. Цели описываются через `## `: help собирается из этих строк.
SHELL := /bin/bash

# go.mod требует Go 1.25; на машине может стоять старее — тулчейн докачается сам.
export GOTOOLCHAIN := auto

COMPOSE := docker compose -f deploy/docker-compose.yml --env-file .env
DEV_COMPOSE := docker compose -f deploy/docker-compose.dev.yml --env-file deploy/.env.dev

.DEFAULT_GOAL := help
.PHONY: help env db db-down ml import install dev run web test test-live lint check check-secrets build deploy

help: ## показать список целей
	@grep -hE '^[a-zA-Z0-9_-]+:.*?## ' $(MAKEFILE_LIST) \
		| awk 'BEGIN {FS = ":.*?## "} {printf "  \033[36m%-14s\033[0m %s\n", $$1, $$2}'

env: ## локальные секреты: случайный пароль dev-базы и backend/.env (существующие не трогает)
	@if [ -f deploy/.env.dev ]; then echo "deploy/.env.dev уже есть — не трогаю"; else \
	umask 077 && printf 'DEV_DB_PASSWORD=%s\n' "$$(openssl rand -hex 16)" > deploy/.env.dev && \
	echo "создан deploy/.env.dev"; fi
	@if [ -f backend/.env ]; then echo "backend/.env уже есть — не трогаю"; else \
	pw=$$(sed -n 's/^DEV_DB_PASSWORD=//p' deploy/.env.dev) && \
	sed -e "s#^DATABASE_URL=.*#DATABASE_URL=postgres://kuznica:$$pw@127.0.0.1:5441/kuznica?sslmode=disable#" \
	    backend/.env.example > backend/.env && chmod 600 backend/.env && echo "создан backend/.env"; fi

db: ## поднять локальные Postgres (127.0.0.1:5441) и OpenSearch (127.0.0.1:9201)
	$(DEV_COMPOSE) up -d --wait db opensearch

db-down: ## остановить локальные Postgres и OpenSearch (данные остаются в томах)
	$(DEV_COMPOSE) down

ml: ## эмбеддер и реранкер на 127.0.0.1:8089 (нативно, MPS)
	./ml/run.sh

import: db ## импорт data/mock в Postgres и индекс OpenSearch (векторы — если make ml запущен)
	cd backend && go run . import

install: ## зависимости фронта (pnpm)
	cd frontend && pnpm install

dev: db ## Postgres + OpenSearch + API на 127.0.0.1:8088 + Vite на localhost:5177 (Ctrl+C гасит API и Vite)
	@test -d frontend/node_modules || (cd frontend && pnpm install)
	@trap 'kill 0' EXIT; (cd backend && go run .) & (cd frontend && pnpm dev) & wait

run: ## только API из исходников (без базы и OpenSearch — health отдаёт db/search: down)
	cd backend && go run .

web: ## только dev-сервер фронта
	cd frontend && pnpm dev

test: ## go vet + go test бэкенда, node --test фронта
	cd backend && go vet ./... && go test ./... -race -count=1
	cd frontend && pnpm test

test-live: ## живой тест поиска на локальных OpenSearch и эмбеддере (свой индекс itest_products)
	cd backend && OPENSEARCH_URL=http://127.0.0.1:9201 EMBEDDER_URL=http://127.0.0.1:8089 \
		go test -tags integration ./services/ -run Live -count=1 -v

lint: ## gofmt + go vet, eslint + prettier + vue-tsc
	@out=$$(cd backend && gofmt -l .); if [ -n "$$out" ]; then echo "gofmt: $$out"; exit 1; fi
	cd backend && go vet ./...
	cd frontend && pnpm lint && pnpm format:check && pnpm typecheck

check: ## всё перед коммитом: lint, test, сборка фронта, секреты
	$(MAKE) lint
	$(MAKE) test
	cd frontend && pnpm build
	$(MAKE) check-secrets

check-secrets: ## gitleaks по истории и staged + поиск ключей в собранном бандле
	@if command -v gitleaks >/dev/null; then gitleaks git --no-banner --redact . && \
	gitleaks git --pre-commit --staged --no-banner --redact . ; \
	else echo "gitleaks не установлен (brew install gitleaks) — проверка истории пропущена"; fi
	@if [ -d frontend/dist ]; then \
	if grep -rEl 'AKIA|sk-[A-Za-z0-9]{20,}|AQVN' frontend/dist; then echo "ключ в бандле"; exit 1; \
	else echo "bundle clean"; fi; else echo "frontend/dist нет — проверка бандла пропущена"; fi

build: ## сборка образов deploy/ (нужен корневой .env)
	$(COMPOSE) build

deploy: ## на сервере: сборка и compose up
	$(COMPOSE) build
	$(COMPOSE) up -d
