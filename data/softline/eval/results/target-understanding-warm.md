# Оценка по цели — набор понимания (настройка) (understanding)

Каталог: фид заказчика, 136 040 офферов · правила v5 · коммит 77028d3+dirty · хэш набора `47264b9685c0` · 64 запросов · прогон …b3a36f · режим: тёплый (второй проход: кэши разборов, векторов и реранкера заполнены первым)

Команда: `go run . eval --set etalon,tz,neighbors,understanding --configs live --report ../data/softline/eval/results`

Success@1 — первая цель ответа (поле `target`, иначе первая карточка: URL семейства → оффера → вендора) совпала с ожидаемой; URL сравниваются после нормализации (регистр хоста, схема, `www`, завершающий слэш, URL-кодирование, `filter[category][]=` ≡ `[0]=`, порядок параметров). «В первых 4» — она же или карточка ожидаемой цели среди первых четырёх; MRR@10 — по месту первой такой карточки. «Все условия» — Success@1 (у набора понимания — проверка первых N карточек) и условия строки: нет исключённых семейств, есть «Можно купить сейчас», нет нарушений жёстких условий. ≥4 семейств и SKU — доля ответов (все карточки, включая «Популярное в наличии»). Нарушения — запросы с жёсткими условиями (продление, число мест, тип организации, срок), где карточка в первых 10 их нарушает. D: метка / модель / кэш — поисков с меткой маршрута D, с вызовом модели и с разбором из кэша; ₽/запрос — llm_usage прогона на запрос.

| конфигурация | Success@1 | в первых 4 | MRR@10 | все условия | ≥4 семейств | ≥4 SKU | нарушения | p50, мс | p95, мс | ₽/запрос | D: метка / модель / кэш |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Полный продукт (live) | — | — | — | 64/64 (100 %) | 64/64 (100 %) | 64/64 (100 %) | — | 55 | 127 | 0,0000 | 0 / 0 / 0 |


## Все запросы — Полный продукт (live)

| id | запрос | ожидаем | Success@1 | в 4 | место | первая цель | условия | маршрут | сем. / SKU |
|---|---|---|---|---|---|---|---|---|---|
| u01 | astra linux | family 121231 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | A | 5 / 827 |
| u02 | Microsoft Office для дома и бизнеса | family 391338 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | — | B | 15 / 517 |
| u03 | kaspersky small office security | family 3761 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | — | B | 15 / 1022 |
| u04 | крипто про csp 5.0 | family 22268 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 12 / 629 |
| u05 | anydesk | family 306539 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| u06 | kaspersky endpoint security | family 3935,3953 (первые 3) | по карточкам: да | да | 1 | vendor_category https://store.softline.ru/kaspersky?filter%5Bcategory%5D[]=650 | — | B | 19 / 1271 |
| u07 | windows 11 pro | family 286676 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/microsoft/microsoft-windows-professional-11 | — | A | 4 / 11 |
| u08 | secret net studio | family 42240 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/kod-bezopasnosti/kod-bezopasnosti-secret-net-studio | — | A | 4 / 312 |
| u09 | termidesk | family 299975 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/astralinux/termidesk | — | A | 4 / 357 |
| u10 | workspad | family 300796 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/astralinux/workspad | — | A | 4 / 287 |
| u11 | компас-3d | family 11571 (первые 3) | по карточкам: да | да | 1 | family https://store.softline.ru/ascon/kompas-3d | — | A | 4 / 55 |
| u12 | nanocad | family 268540 (первые 3) | по карточкам: да | да | 1 | family https://store.softline.ru/nanosoft/platforma-nanocad | — | A | 20 / 1080 |
| u13 | альт сервер | family 408335 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/basealt/alt-server-11 | — | B | 8 / 229 |
| u14 | postgres pro | vendor postgrespro (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/postgrespro | — | A | 11 / 134 |
| u15 | яндекс 360 | vendor yandex (первые 1) | по карточкам: да | да | 1 | vendor_category https://store.softline.ru/yandex?filter%5Bcategory%5D[]=222323 | — | B | 10 / 138 |
| u16 | usergate | vendor entensys (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/entensys | — | B | 4 / 6 |
| u17 | кибер бэкап | vendor cyberprotect (первые 1) | по карточкам: да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 5 / 478 |
| u18 | dr.web | vendor drweb (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/drweb | — | B | 11 / 3767 |
| u19 | мтс линк | vendor webinar (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/webinar | — | B | 4 / 10 |
| u20 | 365 | family 165034 (первые 3) | по карточкам: да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | — | B | 8 / 298 |
| u21 | сканер-вс | family 46421 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | A | 8 / 481 |
| u22 | сканер вс | family 46421 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | B | 9 / 482 |
| u23 | max patrol | family 437421 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | A | 4 / 3156 |
| u24 | maxpatrol | family 437421 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | A | 4 / 3156 |
| u25 | антивирь | category 650,3775 (первые 4) | по карточкам: да | да | 1 | category «Антивирусы для организаций» | — | B | 20 / 4713 |
| u26 | винда | family 286676,310176 (первые 3) | по карточкам: да | да | 1 | category «Windows 11: лицензии Home и Pro» | — | B | 17 / 150 |
| u27 | офис | category 1145,222323 (первые 4) | по карточкам: да | да | 1 | category «Офисные пакеты программ Microsoft Office» | — | B | 20 / 173 |
| u28 | касперыч | vendor kaspersky (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/kaspersky | — | B | 4 / 470 |
| u29 | ворд | family 162037,162743 (первые 3) | по карточкам: да | да | 1 | sku https://store.softline.ru/writebrothers/1wm1122-49080 | — | B | 10 / 85 |
| u30 | астра линукс | family 121231 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 9 / 849 |
| u31 | астралинукс | family 121231 (первые 3) | по карточкам: да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 8 / 844 |
| u32 | мдаемон | vendor alt-n-technologies-ltd (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/alt-n-technologies-ltd | — | B | 5 / 495 |
| u33 | мдемон | vendor alt-n-technologies-ltd (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/alt-n-technologies-ltd | — | B | 5 / 495 |
| u34 | доктор веб | vendor drweb (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/drweb | — | B | 11 / 3767 |
| u35 | др веб | vendor drweb (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/drweb | — | B | 11 / 3767 |
| u36 | касперский | vendor kaspersky (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/kaspersky | — | B | 4 / 480 |
| u37 | виндовс 11 | family 286676,310176 (первые 2) | по карточкам: да | да | 1 | family https://store.softline.ru/microsoft/microsoft-windows-professional-11 | — | B | 15 / 100 |
| u38 | ред ос | family 165644 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/redsoft/red-os-1 | — | B | 5 / 160 |
| u39 | альт линукс | vendor basealt (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/basealt | — | B | 8 / 229 |
| u40 | секрет нет | family 42240,263917 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/kod-bezopasnosti/kod-bezopasnosti-secret-net-lsp | — | B | 4 / 366 |
| u41 | термидеск | family 299975 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/astralinux/termidesk | — | B | 4 / 129 |
| u42 | rfcgthcrbq | vendor kaspersky (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/kaspersky | — | B | 4 / 480 |
| u43 | rhbgnjghj | vendor kripto-pro (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/kripto-pro | — | B | 13 / 631 |
| u44 | kaspersky totl security | family 3971 (первые 3) | по карточкам: да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-total-security-dlya-biznesa | — | B | 14 / 1008 |
| u45 | астра линкус | family 121231 (первые 3) | по карточкам: да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 9 / 849 |
| u46 | eset | vendor pro32 (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/pro32 | — | B | 6 / 3533 |
| u47 | nod32 | vendor pro32 (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/pro32 | — | B | 6 / 3533 |
| u48 | abbyy finereader | vendor contentai (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/contentai/abbyy-lingvo-x6-multiligual | — | B | 9 / 42 |
| u49 | acronis | vendor cyberprotect (первые 1) | по карточкам: да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 5 / 478 |
| u50 | сервер | category 86563 (первые 4) | по карточкам: да | да | 1 | category «Rack-серверы» | — | C | 14 / 19 |
| u51 | почтовый сервер | category 369,1267,7413 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/mbklab/pochtovyy-server-tegu | — | C | 9 / 311 |
| u52 | антивирус для сервера | category 650 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/drweb/drweb-server-security-suite-1 | — | B | 20 / 4863 |
| u53 | серверная память | category 76963 (первые 1) | по карточкам: да | да | 1 | sku https://store.softline.ru/kingston/ksm64r52bd4-64md-490542 | — | C | 20 / 20 |
| u54 | антивирус | category 650,3775 (первые 4) | по карточкам: да | да | 1 | category «Антивирусы для организаций» | — | C | 20 / 4655 |
| u55 | оперативная память | category 76963 (первые 4) | по карточкам: да | да | 1 | category «Оперативная память» | — | C | 5 / 5 |
| u56 | ноутбук | category 70824 (первые 4) | по карточкам: да | да | 1 | category «Ноутбуки» | — | C | 16 / 20 |
| u57 | сканер | category 94641 (первые 1) | по карточкам: да | да | 1 | category «Сканеры» | — | C | 19 / 20 |
| u58 | монитор | category 80843,91767 (первые 1) | по карточкам: да | да | 1 | category «Мониторы» | — | C | 19 / 20 |
| u59 | kaspersky | vendor kaspersky (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/kaspersky | — | B | 4 / 78 |
| u60 | видеоконференции | category 7388,157991 (первые 1) | по карточкам: да | да | 1 | sku https://store.softline.ru/yealink/m900-0010-475274 | — | C | 15 / 42 |
| u61 | 1с на 5 пользователей | vendor 1s-bitriks (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/1s-bitriks/1s-bitriks24-1 | — | B | 4 / 273 |
| u62 | электронная подпись | family 37267,413744,426903,443262,37278 (первые 1) | по карточкам: да | да | 1 | vendor_category https://store.softline.ru/aktiv?filter%5Bcategory%5D[]=687 | — | B | 9 / 434 |
| u63 | windows server 2025 | family 161664,281974,294539,161826 (первые 1) | по карточкам: да | да | 1 | family https://store.softline.ru/microsoft/microsoft-windows-server-essentials-2022 | — | A | 15 / 96 |
| u64 | продлить касперский на 100 пк на 2 года | vendor kaspersky (первые 1) | по карточкам: да | да | 1 | vendor https://store.softline.ru/kaspersky | — | B | 17 / 444 |
