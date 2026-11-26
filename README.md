# Магазин: шаблон к хакатону «AI Кузница», кейс поиска

Каркас интернет-магазина: Go API с проверкой здоровья и витрина на Vue без входа. Кейс хакатона
ещё не известен; шаблон даёт рабочую раскладку, сборку, проверки и деплой, чтобы в день старта
сразу писать поиск.

## Что уже есть
- `GET /health` и `GET /api/health` — `{"status":"ok","db":"ok|down","time":"…"}`. Всегда 200, раз
  процесс ответил; лежащая база видна в поле `db`.
- Витрина на `/`: поиск, фильтр по категориям, сетка товаров, пустой результат с кнопкой сброса,
  статус связи с API в подвале. Запрос и категория хранятся в адресе (`/?q=плед&cat=home`).
- Товары — демо-набор во фронте (`frontend/src/mocks/catalog.ts`), поиск по ним — временный
  подстрочный (`frontend/src/lib/catalog.ts`). Настоящий каталог и поиск переедут в API.

## Стек
| Часть | Что |
| --- | --- |
| API | Go 1.25, Fiber v2, GORM + pgx, zap |
| База | PostgreSQL 16 |
| Фронт | Vue 3.5, TypeScript, Vite, PrimeVue 4.5 (тема Aura), Tailwind CSS 4, Pinia, Vue Router, Lucide |
| Деплой | Docker Compose: Postgres, API, Caddy (SPA + прокси `/api`, TLS) |
| Проверки | go vet, go test, ESLint, Prettier, vue-tsc, node --test, gitleaks |

## Запуск
Нужны Go (тулчейн 1.25 докачается сам), Node 22 с pnpm, Docker.

```bash
make env       # deploy/.env.dev и backend/.env со случайным паролем dev-базы
make dev       # Postgres :5441 + API 127.0.0.1:8088 + Vite http://localhost:5177
```

Без Docker: `make run` и `make web` в двух терминалах — API стартует без базы, health покажет
`db: down`.

| Команда | Что делает |
| --- | --- |
| `make test` | go vet + go test, node --test фронта |
| `make lint` | gofmt, go vet, eslint, prettier, vue-tsc |
| `make check` | lint + test + сборка фронта + поиск секретов — перед коммитом |
| `make build` | образы `deploy/` (нужен корневой `.env` из `.env.example`) |

## Порты
| Что | Адрес |
| --- | --- |
| Vite (dev) | http://localhost:5177, `/api` проксируется на API |
| API (dev) | 127.0.0.1:8088 |
| Postgres (dev) | 127.0.0.1:5441, контейнер `kuznica-dev-db-1` |
| API в контейнере | :8080 за Caddy |

Лицензии зависимостей — `LICENSES.md`.
