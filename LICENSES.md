# Лицензии сторонних материалов

Правило: только OFL / MIT / ISC / Apache-2.0 / BSD; шрифты и иконки self-host; строка добавляется в момент
подключения.

| Name | Version | Origin | License | Used in |
| --- | --- | --- | --- | --- |
| Fiber | v2.52.x | github.com/gofiber/fiber | MIT | backend |
| fiberzap | v2.1.6 | github.com/gofiber/contrib/fiberzap | MIT | backend (журнал HTTP) |
| tinylib/msgp | v1.2.5 | github.com/tinylib/msgp | MIT | backend (косвенно: лимитер Fiber) |
| philhofer/fwd | v1.1.3 | github.com/philhofer/fwd | MIT | backend (косвенно: msgp) |
| zap | v1.28.0 | go.uber.org/zap | MIT | backend (журнал) |
| oklog/ulid | v2.1.2 | github.com/oklog/ulid | Apache-2.0 | backend (номер запроса) |
| godotenv | v1.5.1 | github.com/joho/godotenv | MIT | backend (локальная разработка) |
| GORM | v1.31.2 | gorm.io/gorm | MIT | backend |
| GORM PostgreSQL driver | v1.6.3 | gorm.io/driver/postgres | MIT | backend |
| pgx | v5.11.0 | github.com/jackc/pgx | MIT | backend (драйвер PostgreSQL) |
| google/uuid | v1.6.0 | github.com/google/uuid | BSD-3-Clause | backend (UUIDv7 ключи) |
| excelize | v2.11.1 | github.com/xuri/excelize | BSD-3-Clause | backend (чтение XLSX в generic-импорте) |
| xuri/efp, xuri/nfp | 2025–2026 | github.com/xuri | BSD-3-Clause | backend (косвенно: excelize — формулы и числовые форматы) |
| richardlehane/mscfb, msoleps | v1.0.x | github.com/richardlehane | Apache-2.0 | backend (косвенно: excelize — старые контейнеры OLE) |
| tiendc/go-deepcopy | v1.7.2 | github.com/tiendc/go-deepcopy | MIT | backend (косвенно: excelize) |
| golang.org/x/text, x/net, x/crypto | v0.43 / v0.62 / v0.58 | go.googlesource.com | BSD-3-Clause | backend (cp1251 в generic-импорте; x/crypto/bcrypt — пароли AUTH_ACCOUNTS при входе по ролям; x/net — косвенно: excelize) |
| golang.org/x/sync | v0.24 | go.googlesource.com | BSD-3-Clause | backend (singleflight: один вызов LLM на одинаковые параллельные разборы) |
| yaml.v3 | v3.0.1 | gopkg.in/yaml.v3 | MIT AND Apache-2.0 | backend (файл сопоставления mapping.yaml) |
| Vue | 3.5.x | vuejs.org | MIT | frontend |
| Vue Router | 5.x | router.vuejs.org | MIT | frontend |
| Pinia | 4.x | pinia.vuejs.org | MIT | frontend |
| reka-ui | 2.11.x | reka-ui.com | MIT | frontend (примитивы: Combobox, Select, Dialog, Toast, Tooltip, ToggleGroup, Switch, Collapsible) |
| Tailwind CSS | 4.x | tailwindcss.com | MIT | frontend |
| Lucide (lucide-vue-next) | 1.x | lucide.dev | ISC | frontend (иконки) |
| Manrope (@fontsource-variable/manrope) | 5.3.x | github.com/sharanda/manrope | SIL OFL 1.1 | frontend (шрифт, self-host) |
| Caddy | 2.x | caddyserver.com | Apache-2.0 | deploy (web) |
| OpenSearch (образ opensearchproject/opensearch) | 3.9.0 | opensearch.org | Apache-2.0 | deploy (поиск: BM25 и k-NN; backend ходит по REST через net/http, без SDK) |
| BAAI/bge-m3 (веса) | ревизия 5617a9f | huggingface.co/BAAI/bge-m3 | MIT | ml (эмбеддинги; качается в кеш HF, в репо не лежит) |
| BAAI/bge-reranker-v2-m3 (веса) | ревизия 953dc6f | huggingface.co/BAAI/bge-reranker-v2-m3 | Apache-2.0 | ml (реранкер; качается в кеш HF) |
| PyTorch | 2.15.0 | github.com/pytorch/pytorch | BSD-3-Clause | ml (инференс на MPS/CPU) |
| Transformers | 5.24.1 | github.com/huggingface/transformers | Apache-2.0 | ml (модели) |
| tokenizers | 0.23.3 | github.com/huggingface/tokenizers | Apache-2.0 | ml (токенизатор, через transformers) |
| huggingface_hub | 2.4.0 | github.com/huggingface/huggingface_hub | Apache-2.0 | ml (скачивание моделей) |
| FastAPI | 0.165.0 | github.com/fastapi/fastapi | MIT | ml (HTTP API) |
| Uvicorn | 0.54.0 | github.com/Kludex/uvicorn | BSD-3-Clause | ml (HTTP-сервер) |
| Прочие транзитивные пакеты ml | см. `ml/requirements.txt` | pypi.org | MIT / BSD / Apache-2.0 / ISC; исключения: tqdm — MPL-2.0 AND MIT, typing_extensions — PSF-2.0, regex — Apache-2.0 AND CNRI-Python | ml (ставятся pip, не распространяются; tqdm тянут transformers и huggingface_hub) |
| Playwright (@playwright/test) | 1.61.1 | playwright.dev | Apache-2.0 | e2e (сквозные тесты и запись видео; Chromium из кеша Playwright) |

## Значки производителей (`frontend/public/vendors/`)

49 значков сайтов производителей из mock-каталога, получены по их доменам (сервис значков Google; для
Kaspersky и D-Link — файл с сайта производителя) и приведены к PNG. Это **товарные знаки их владельцев, а не
материалы под открытой лицензией**; используются только чтобы обозначить производителя товара в концепте для
хакатона. До публичного использования нужно проверить условия правообладателей или убрать значки (без файла
интерфейс показывает общую иконку).

Ещё 142 значка производителей из фида заказчика взяты со страниц производителей на store.softline.ru (магазин
заказчика) и вписаны в квадрат 128×128 скриптом `scripts/data/vendor_logos.py`; соответствие имён —
`frontend/src/lib/vendorLogoFeed.ts`. Оговорка о товарных знаках та же.

## Фото демо-аккаунтов (`frontend/public/avatars/`)

7 портретов, переданы владельцем проекта (экспорт из Figma) и уменьшены до 160 px. Происхождение и права по
файлам не установлены. Персоны вымышленные; нужно подтвердить, что это сгенерированные или лицензированные
изображения, а не фото реальных людей (правило проекта: реальных персональных данных нет).
