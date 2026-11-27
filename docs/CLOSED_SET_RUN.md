# Оценка по цели — отложенный набор (закрытая часть) (closed)

Каталог: фид заказчика, 136 040 офферов · правила v5 · коммит 33979de+dirty · хэш набора `6aba3cd02208` · 83 запросов · прогон …901dc2 · режим: холодный (кэш разборов, векторов и реранкера не читается)

Команда: `go run . eval --set closed --file ../data/softline/eval/closed.jsonl --cold --configs BM25,B1,B2,live --report ../docs`

Success@1 по целевой странице — первая целевая страница ответа (поле `target`, иначе первая карточка: URL семейства → оффера → вендора) совпала с ожидаемой; URL сравниваются после нормализации (регистр хоста, схема, `www`, завершающий слэш, URL-кодирование, `filter[category][]=` ≡ `[0]=`, порядок параметров). Hit@4 (нужное семейство в первых четырёх) — первая целевая страница или карточка ожидаемой целевой страницы среди первых четырёх; MRR@10 — по месту первой такой карточки. «Все условия» — Success@1 (у набора понимания — проверка первых N карточек) и условия строки: нет исключённых семейств, есть «Можно купить сейчас», нет нарушений жёстких условий. ≥4 семейств и SKU — доля ответов (все карточки, включая «Популярное в наличии»). Нарушения — запросы с жёсткими условиями (продление, число мест, тип организации, срок), где карточка в первых 10 их нарушает. D: метка / модель / кэш — поисков с меткой маршрута D, с вызовом модели и с разбором из кэша; ₽/запрос — llm_usage прогона на запрос.

| конфигурация | Success@1 по целевой странице | Hit@4 | MRR@10 | все условия | ≥4 семейств | ≥4 SKU | нарушения | p50, мс | p95, мс | ₽/запрос | D: метка / модель / кэш |
|---|---|---|---|---|---|---|---|---|---|---|---|
| BM25 (BM25) | 37/83 (45 %) | 46/83 (55 %) | 0,527 | 29/83 (35 %) | 37/83 (45 %) | 64/83 (77 %) | 3 из 6 | 4 | 10 | 0,0000 | 0 / 0 / 0 |
| BM25 + нормализатор (B1) | 47/83 (57 %) | 66/83 (80 %) | 0,722 | 39/83 (47 %) | 32/83 (39 %) | 76/83 (92 %) | 2 из 6 | 11 | 25 | 0,0000 | 0 / 0 / 0 |
| Гибрид (B2) | 46/83 (55 %) | 73/83 (88 %) | 0,750 | 38/83 (46 %) | 83/83 (100 %) | 83/83 (100 %) | 4 из 6 | 38 | 55 | 0,0000 | 0 / 0 / 0 |
| Полная конфигурация (поиск на витрине) (live) | 66/83 (80 %) | 77/83 (93 %) | 0,882 | 61/83 (73 %) | 83/83 (100 %) | 83/83 (100 %) | 1 из 6 | 144 | 1994 | 0,0227 | 14 / 12 / 0 |

- B1 против BM25: хуже 1, лучше 22.
- B2 против B1: хуже 7, лучше 9.
- live против B2: хуже 5, лучше 31.

## По частям набора

| По частям набора: Success@1 по целевой странице (Hit@4) | BM25 | B1 | B2 | live |
|---|---|---|---|---|
| catalog | 22/39 (27) | 21/39 (28) | 22/39 (31) | 27/39 (33) |
| neighbor | 15/44 (19) | 26/44 (38) | 24/44 (42) | 39/44 (44) |

## По виду искажения

| По виду искажения: Success@1 по целевой странице (Hit@4) | BM25 | B1 | B2 | live |
|---|---|---|---|---|
| ТЗ в словоформе | 0/7 (2) | 0/7 (4) | 2/7 (5) | 6/7 (7) |
| атрибут | 6/9 (8) | 7/9 (8) | 7/9 (9) | 7/9 (9) |
| мусорный | 7/7 (7) | 6/7 (6) | 6/7 (6) | 6/7 (6) |
| опечатка | 2/9 (3) | 4/9 (8) | 3/9 (9) | 9/9 (9) |
| по запросу → альтернатива | 6/8 (7) | 6/8 (7) | 5/8 (8) | 5/8 (5) |
| раскладка | 0/8 (0) | 4/8 (6) | 4/8 (8) | 6/8 (8) |
| сервер | 2/10 (4) | 2/10 (4) | 2/10 (5) | 4/10 (9) |
| сервисный | 7/7 (7) | 7/7 (7) | 7/7 (7) | 6/7 (6) |
| склонение | 6/9 (7) | 7/9 (9) | 7/9 (9) | 9/9 (9) |
| транслит | 1/9 (1) | 4/9 (7) | 3/9 (7) | 8/9 (9) |

## По вендорам

| По вендорам: Success@1 по целевой странице (Hit@4) | BM25 | B1 | B2 | live |
|---|---|---|---|---|
| anydesk | 1/4 (1) | 4/4 (4) | 4/4 (4) | 4/4 (4) |
| astralinux | 0/5 (0) | 0/5 (4) | 0/5 (5) | 4/5 (5) |
| cyberprotect | 2/5 (2) | 3/5 (5) | 3/5 (5) | 5/5 (5) |
| kaspersky | 2/5 (3) | 5/5 (5) | 5/5 (5) | 5/5 (5) |
| kripto-pro | 3/5 (4) | 3/5 (4) | 1/5 (5) | 3/5 (5) |
| microsoft | 2/5 (2) | 2/5 (3) | 2/5 (4) | 4/5 (5) |
| npo-echelon | 2/5 (2) | 3/5 (3) | 3/5 (4) | 5/5 (5) |
| positive-technologies | 2/5 (2) | 4/5 (5) | 4/5 (5) | 5/5 (5) |
| webinar | 1/5 (3) | 2/5 (5) | 2/5 (5) | 4/5 (5) |

## Все запросы — Полная конфигурация (поиск на витрине) (live)

| id | запрос | ожидаем | Success@1 | Hit@4 | место | первая целевая страница | условия | маршрут | сем. / SKU |
|---|---|---|---|---|---|---|---|---|---|
| c01 | астру линукс | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 10 / 851 |
| c02 | астра линукс спешл эдишн | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | D | 4 / 773 |
| c03 | астра лиункс | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 9 / 849 |
| c04 | fcnhf kbyerc | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 9 / 849 |
| c05 | astra linux для образовательных учреждений | family 121231 | нет (ранг 4) | да | 4 | family https://store.softline.ru/linux-center/linux-edumandriva | target, hard (2 карт.) | B | 13 / 386 |
| c06 | виндовс 11 домашней | family 310176 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-windows-11-home | — | B | 4 / 60 |
| c07 | майкрософт визио | family 391350,391347 | нет (ссылка) | да | 1 | family https://store.softline.ru/microsoft/microsoft-exchange-online | — | B | 9 / 425 |
| c08 | microsft project | family 391344,391341 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-project-standard-2024 | — | B | 7 / 348 |
| c09 | ьшскщыщае 365 | family 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | — | B | 13 / 586 |
| c10 | windows 11 pro электронный ключ | family 286676 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-windows-professional-11 | — | A | 4 / 11 |
| c11 | касперского для виртуальных сред | family 3872 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-security-for-virtualization | — | B | 8 / 635 |
| c12 | касперский тотал секьюрити | family 3971 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-total-security-dlya-biznesa | — | B | 20 / 1255 |
| c13 | kaspersky endpiont security cloud | family 4052 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-endpoint-security-dlya-biznesa-cloud | — | B | 17 / 1251 |
| c14 | лфызукылн увк | family 420044 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-edr-dlya-biznesa-optimalnyy | — | B | 4 / 277 |
| c15 | продление kes расширенный на 50 узлов | family 3935 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-endpoint-security-dlya-biznesa-rasshirennyiy | — | D | 8 / 298 |
| c16 | программы криптопро csp 5.0 | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 13 / 631 |
| c17 | kriptopro csp | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 12 / 622 |
| c18 | криптрпро csp | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 12 / 622 |
| c19 | rhbgnjghj rk.x | family 375232 | нет (ранг 2) | да | 2 | family https://store.softline.ru/kripto-pro/kriptopro-javacsp | — | B | 10 / 567 |
| c20 | криптопро csp на сервер | family 22268 | нет (ранг 2) | да | 2 | family https://store.softline.ru/kripto-pro/kriptopro-csp-tls | — | B | 13 / 631 |
| c21 | эни деска | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| c22 | анидеск | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| c23 | anydsek | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| c24 | anydesk advanced на год | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 79 |
| c25 | кибер файлов | family 394620 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-fayly | — | A | 7 / 576 |
| c26 | kiber bekap | vendor_category cyberprotect+1033 | да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 5 / 478 |
| c27 | кибер бэкп | vendor_category cyberprotect+1033 | да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 5 / 478 |
| c28 | rb,th [hfybkbot | family 412674 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-khranilishche | — | B | 4 / 170 |
| c29 | кибер бэкап старт на 1 пк | family 444870 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-bekap-start | — | B | 4 / 306 |
| c30 | сканеру-вс | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | B | 4 / 531 |
| c31 | scaner-vs | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | D | 4 / 534 |
| c32 | сканр-вс | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | D | 4 / 531 |
| c33 | crfyth-dc | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | B | 4 / 531 |
| c34 | продление сканер-вс | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | B | 4 / 168 |
| c35 | в мтс линке | vendor webinar | да | да | 1 | vendor https://store.softline.ru/webinar | — | B | 4 / 10 |
| c36 | mts link pro | family 447243 | да | да | 1 | family https://store.softline.ru/webinar/mts-link-pro | — | B | 4 / 10 |
| c37 | мтс линг | vendor webinar | да | да | 1 | vendor https://store.softline.ru/webinar | — | B | 4 / 10 |
| c38 | vnc kbyr | vendor webinar | нет (ранг 3) | да | 3 | family https://store.softline.ru/smartcode-solutions/smartcode-vnc-manager | — | B | 5 / 24 |
| c39 | мтс линк стандарт на 1 год | family 447242 | да | да | 1 | family https://store.softline.ru/webinar/mts-link-standart | — | B | 4 / 9 |
| c40 | сканера xspider | family 28574,428013 | да | да | 1 | family https://store.softline.ru/positive-technologies/positive-technologies-xspider-pro | — | A | 11 / 138 |
| c41 | икс спайдер | family 28574,428013 | да | да | 1 | family https://store.softline.ru/positive-technologies/positive-technologies-xspider | — | B | 4 / 130 |
| c42 | maxpatorl | family 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | B | 4 / 3156 |
| c43 | ьфчзфекщд | family 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | B | 4 / 3156 |
| c44 | xspider на 16 хостов | family 28574,428013 | да | да | 1 | family https://store.softline.ru/positive-technologies/positive-technologies-xspider | — | A | 11 / 217 |
| k01 | сервера | category 86563 | да | да | 1 | category «Rack-серверы» | — | C | 14 / 19 |
| k02 | антивирусов | category 650,3775 | да | да | 1 | category «Антивирусы для организаций» | — | C | 20 / 4788 |
| k03 | антивирусом | category 650,3775 | да | да | 1 | category «Антивирусы для организаций» | — | C | 20 / 4547 |
| k04 | антивиря | category 650,3775 | да | да | 1 | category «Антивирусы для организаций» | — | B | 20 / 4713 |
| k05 | офисный пакет | category 1145,222323 | нет (ссылка) | да | 1 | family https://store.softline.ru/microsoft/microsoft-visio-professional | — | A | 20 / 659 |
| k06 | винде | category 445047,2050 \| family 286676,310176 | да | да | 1 | category «Windows 11: лицензии Home и Pro» | — | B | 17 / 150 |
| k07 | мдаемона | vendor alt-n-technologies-ltd \| vendor_category alt-n-technologies-ltd | да | да | 1 | vendor https://store.softline.ru/alt-n-technologies-ltd | — | B | 5 / 495 |
| k08 | стоечный сервер | category 86563 | нет (ссылка) | да | 1 | sku https://store.softline.ru/depo/3470e2a-6-467607 | target, exclude | A | 15 / 20 |
| k09 | купить сервер | category 86563 | да | да | 1 | category «Rack-серверы» | — | C | 14 / 19 |
| k10 | двухпроцессорный сервер | category 86563 | нет (ранг 4) | да | 4 | sku https://store.softline.ru/supermicro/as-2125hs-tnr-bundle3-539251 | — | D | 8 / 8 |
| k11 | серверная стойка | category 87317 \| sku 531626 \| family offer-531626 | да | да | 1 | category «Стойки серверные» | — | A | 4 / 4 |
| k12 | микросервер | category 87268 \| sku 531625 \| family offer-531625 | да | да | 1 | category «Микросерверы» | — | C | 4 / 4 |
| k13 | блок питания для сервера | category 87956 | нет (цель) | нет | — | category «Блоки питания» | — | B | 20 / 20 |
| k14 | серверное оборудование | category 86563,87268,87317,87956 | нет (ссылка) | да | 1 | sku https://store.softline.ru/depo/1901-64212-1p-2p-4-v-r-531626 | — | C | 4 / 4 |
| k15 | серверное программное обеспечение | category 369 | нет (ранг 4) | да | 4 | family https://store.softline.ru/syncplify/syncplifyme-server | — | C | 7 / 276 |
| k16 | сервер inferit | vendor_category inferit+86563 | да | да | 1 | vendor_category https://store.softline.ru/inferit?filter%5Bcategory%5D[]=86563 | — | B | 7 / 7 |
| k17 | серверы supermicro | vendor_category supermicro+86563 | нет (ссылка) | да | 1 | sku https://store.softline.ru/supermicro/sys-621p-trt-482054 | — | B | 4 / 4 |
| k18 | виндовс сервер стандарт | family 161664 | нет (цель) | нет | — | family https://store.softline.ru/astralinux/astra-linux-special-edition | target, alternatives | B | 14 / 794 |
| k19 | windows 10 pro | family 295882 | нет (цель) | нет | — | family https://store.softline.ru/microsoft/microsoft-windows-professional-11 | target, alternatives | A | 4 / 11 |
| k20 | autocad | family 189942 | нет (ранг 7) | нет | 7 | family https://store.softline.ru/ippekshev/modplus-autocad | target, alternatives | A | 16 / 278 |
| k21 | мойофис стандартный | family 344604 | да | да | 1 | family https://store.softline.ru/ncloudtech/moyofis-standartnyy-2 | alternatives | B | 5 / 75 |
| k22 | р7 офис | vendor_category novcommtech+222323 | да | да | 1 | family https://store.softline.ru/novcommtech/ao-r7-dlya-biznesa | alternatives | B | 20 / 307 |
| k23 | microsoft sql server standard | family 1870 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-sql-server-standard-edition | alternatives | B | 8 / 149 |
| k24 | adobe acrobat pro | family 459 | да | да | 1 | family https://store.softline.ru/adobe/adobe-acrobat-pro | alternatives | B | 20 / 545 |
| k25 | vmware vsphere | family 12822 | да | да | 1 | family https://store.softline.ru/vmware/vmware-vsphere | alternatives | B | 20 / 226 |
| k26 | как оплатить заказ | none | да | да | 1 | service https://store.softline.ru/help | — | D | 4 / 80 |
| k27 | доставка электронных лицензий | none | да | да | 1 | service https://store.softline.ru/help | — | D | 4 / 80 |
| k28 | как вернуть лицензию | none | да | да | 1 | family https://store.softline.ru/autosoft/avtopredpriyatie | — | B | 4 / 80 |
| k29 | связаться с менеджером | none | да | да | 1 | service https://store.softline.ru/help | — | C | 4 / 80 |
| k30 | реквизиты для оплаты по счету | none | да | да | 1 | service https://store.softline.ru/help | — | D | 4 / 80 |
| k31 | сколько ждать ключ после оплаты | none | да | да | 1 | service «Помощь менеджера» | — | D | 4 / 80 |
| k32 | режим работы поддержки | none | нет (цель) | нет | — | family https://store.softline.ru/retrospect/retrospect-support-and-maintenance | — | A | 23 / 955 |
| k33 | ывапролдж | none | да | да | 1 | family https://store.softline.ru/autosoft/avtopredpriyatie | — | D | 4 / 80 |
| k34 | zxcvbnm | none | да | да | 1 | family https://store.softline.ru/autosoft/avtopredpriyatie | — | D | 4 / 80 |
| k35 | 1234567890 | none | да | да | 1 | family https://store.softline.ru/autosoft/avtopredpriyatie | — | D | 4 / 80 |
| k36 | аааааааа | none | да | да | 1 | family https://store.softline.ru/autosoft/avtopredpriyatie | — | D | 4 / 80 |
| k37 | лорем ипсум | none | да | да | 1 | family https://store.softline.ru/autosoft/avtopredpriyatie | — | D | 4 / 80 |
| k38 | ??? !!! | none | да | да | 1 | family https://store.softline.ru/autosoft/avtopredpriyatie | — | B | 4 / 80 |
| k39 | кукуруза в банке | none | нет (цель) | нет | — | family https://store.softline.ru/tehnologii-upravleniya-i-razvitiya/kommerceskii-bank | — | C | 4 / 4 |
