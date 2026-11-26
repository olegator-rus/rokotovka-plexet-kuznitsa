# Оценка по цели — эталоны заказчика (etalon)

Каталог: фид заказчика, 136 040 офферов · правила v4 · коммит 77028d3+dirty · хэш набора `b861595a92cf` · 10 запросов · прогон …e12212 · режим: холодный (кэш разборов, векторов и реранкера не читается)

Команда: `go run . eval --set etalon,tz,neighbors,understanding --cold --configs BM25,B1,B2,live --report ../data/softline/eval/results`

Success@1 — первая цель ответа (поле `target`, иначе первая карточка: URL семейства → оффера → вендора) совпала с ожидаемой; URL сравниваются после нормализации (регистр хоста, схема, `www`, завершающий слэш, URL-кодирование, `filter[category][]=` ≡ `[0]=`, порядок параметров). «В первых 4» — она же или карточка ожидаемой цели среди первых четырёх; MRR@10 — по месту первой такой карточки. «Все условия» — Success@1 (у набора понимания — проверка первых N карточек) и условия строки: нет исключённых семейств, есть «Можно купить сейчас», нет нарушений жёстких условий. ≥4 семейств и SKU — доля ответов (все карточки, включая «Популярное в наличии»). Нарушения — запросы с жёсткими условиями (продление, число мест, тип организации, срок), где карточка в первых 10 их нарушает. D: метка / модель / кэш — поисков с меткой маршрута D, с вызовом модели и с разбором из кэша; ₽/запрос — llm_usage прогона на запрос.

| конфигурация | Success@1 | в первых 4 | MRR@10 | все условия | ≥4 семейств | ≥4 SKU | нарушения | p50, мс | p95, мс | ₽/запрос | D: метка / модель / кэш |
|---|---|---|---|---|---|---|---|---|---|---|---|
| BM25 (BM25) | 5/10 (50 %) | 8/10 (80 %) | 0,750 | 5/10 (50 %) | 3/10 (30 %) | 9/10 (90 %) | — | 6 | 11 | 0,0000 | 0 / 0 / 0 |
| BM25 + нормализатор (B1) | 7/10 (70 %) | 9/10 (90 %) | 0,900 | 7/10 (70 %) | 2/10 (20 %) | 9/10 (90 %) | — | 9 | 12 | 0,0000 | 0 / 0 / 0 |
| Гибрид (B2) | 6/10 (60 %) | 9/10 (90 %) | 0,867 | 6/10 (60 %) | 10/10 (100 %) | 10/10 (100 %) | — | 26 | 38 | 0,0000 | 0 / 0 / 0 |
| Полный продукт (live) | 10/10 (100 %) | 10/10 (100 %) | 1,000 | 10/10 (100 %) | 10/10 (100 %) | 10/10 (100 %) | — | 92 | 142 | 0,0000 | 0 / 0 / 0 |

- B1 против BM25: хуже 0, лучше 2.
- B2 против B1: хуже 1, лучше 0.
- live против B2: хуже 0, лучше 4.

## Все запросы — Полный продукт (live)

| id | запрос | ожидаем | Success@1 | в 4 | место | первая цель | условия | маршрут | сем. / SKU |
|---|---|---|---|---|---|---|---|---|---|
| e01 | astra linux | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | A | 5 / 827 |
| e02 | Microsoft Office для дома и бизнеса | family 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | — | B | 15 / 517 |
| e03 | kaspersky small office security | family 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | — | B | 15 / 1022 |
| e04 | крипто про csp 5.0 | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 12 / 629 |
| e05 | anydesk | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| e06 | кибер бэкап | vendor_category cyberprotect+1033 | да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 5 / 478 |
| e07 | сканер-вс | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | A | 8 / 481 |
| e08 | мтс | vendor webinar | да | да | 1 | vendor https://store.softline.ru/webinar | — | A | 4 / 10 |
| e09 | max patrol | family 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | A | 4 / 3156 |
| e10 | 365 | family 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | — | B | 8 / 298 |
