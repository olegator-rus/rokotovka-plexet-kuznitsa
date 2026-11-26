# Оценка по цели — соседи эталонов (настройка) (neighbors)

Каталог: фид заказчика, 136 040 офферов · правила v5 · коммит 77028d3+dirty · хэш набора `e42f489a8153` · 114 запросов · прогон …8043df · режим: тёплый (второй проход: кэши разборов, векторов и реранкера заполнены первым)

Команда: `go run . eval --set etalon,tz,neighbors,understanding --configs live --report ../data/softline/eval/results`

Success@1 — первая цель ответа (поле `target`, иначе первая карточка: URL семейства → оффера → вендора) совпала с ожидаемой; URL сравниваются после нормализации (регистр хоста, схема, `www`, завершающий слэш, URL-кодирование, `filter[category][]=` ≡ `[0]=`, порядок параметров). «В первых 4» — она же или карточка ожидаемой цели среди первых четырёх; MRR@10 — по месту первой такой карточки. «Все условия» — Success@1 (у набора понимания — проверка первых N карточек) и условия строки: нет исключённых семейств, есть «Можно купить сейчас», нет нарушений жёстких условий. ≥4 семейств и SKU — доля ответов (все карточки, включая «Популярное в наличии»). Нарушения — запросы с жёсткими условиями (продление, число мест, тип организации, срок), где карточка в первых 10 их нарушает. D: метка / модель / кэш — поисков с меткой маршрута D, с вызовом модели и с разбором из кэша; ₽/запрос — llm_usage прогона на запрос.

| конфигурация | Success@1 | в первых 4 | MRR@10 | все условия | ≥4 семейств | ≥4 SKU | нарушения | p50, мс | p95, мс | ₽/запрос | D: метка / модель / кэш |
|---|---|---|---|---|---|---|---|---|---|---|---|
| Полный продукт (live) | 114/114 (100 %) | 114/114 (100 %) | 1,000 | 114/114 (100 %) | 114/114 (100 %) | 114/114 (100 %) | 0 из 2 | 59 | 119 | 0,0000 | 0 / 0 / 0 |


## По виду искажения

| По виду искажения: Success@1 (в первых 4) | live |
|---|---|
| атрибут | 11/11 (11) |
| название | 49/49 (49) |
| написание | 23/23 (23) |
| опечатка | 4/4 (4) |
| раскладка | 3/3 (3) |
| склонение | 9/9 (9) |
| транслит | 15/15 (15) |

## По вендорам

| По вендорам: Success@1 (в первых 4) | live |
|---|---|
| anydesk | 6/6 (6) |
| astralinux | 18/18 (18) |
| cyberprotect | 13/13 (13) |
| kaspersky | 19/19 (19) |
| kripto-pro | 15/15 (15) |
| microsoft | 18/18 (18) |
| npo-echelon | 8/8 (8) |
| positive-technologies | 8/8 (8) |
| webinar | 9/9 (9) |

## Все запросы — Полный продукт (live)

| id | запрос | ожидаем | Success@1 | в 4 | место | первая цель | условия | маршрут | сем. / SKU |
|---|---|---|---|---|---|---|---|---|---|
| n001 | астра линукс | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 9 / 849 |
| n002 | астры линукс | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 14 / 856 |
| n003 | для астра линукс | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 10 / 898 |
| n004 | astra linux special edition | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | A | 5 / 797 |
| n005 | astralinux | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 5 / 827 |
| n006 | астра линукс се | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 6 / 862 |
| n007 | фыекф дштгч | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 5 / 827 |
| n008 | астра линкус | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 9 / 849 |
| n009 | astra linux воронеж | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | A | 5 / 827 |
| n010 | астра линукс смоленск | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 6 / 876 |
| n011 | техподдержка астра линукс | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 9 / 874 |
| n012 | астра линукс для сервера | family 121231 | да | да | 1 | family https://store.softline.ru/astralinux/astra-linux-special-edition | — | B | 14 / 1098 |
| n013 | астра | vendor astralinux | да | да | 1 | vendor https://store.softline.ru/astralinux | — | B | 5 / 394 |
| n014 | алд про | family 287015 | да | да | 1 | family https://store.softline.ru/astralinux/ald-pro | — | B | 4 / 155 |
| n015 | ald pro | family 287015 | да | да | 1 | family https://store.softline.ru/astralinux/ald-pro | — | A | 4 / 155 |
| n016 | термидеск | family 299975 | да | да | 1 | family https://store.softline.ru/astralinux/termidesk | — | B | 4 / 129 |
| n017 | рубэкап | family 406074 | да | да | 1 | family https://store.softline.ru/astralinux/rubackup-oneclick | — | B | 5 / 334 |
| n018 | тантор | family 342351 | да | да | 1 | family https://store.softline.ru/astralinux/tantor | — | C | 4 / 97 |
| n019 | офис для дома и бизнеса | family 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | — | B | 20 / 466 |
| n020 | майкрософт офис для дома и бизнеса | family 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | — | B | 20 / 291 |
| n021 | office home and business 2024 | family 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | — | A | 9 / 18 |
| n022 | ms office для дома и бизнеса 2024 | family 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | — | B | 15 / 435 |
| n023 | офиса для дома и бизнеса | family 391338 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-biznesa-2024 | — | B | 20 / 500 |
| n024 | офис для дома и учебы | family 391335 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-ucheby-2024 | — | B | 20 / 161 |
| n025 | office home and student | family 391335 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-office-dlya-doma-i-ucheby-2024 | — | C | 4 / 4 |
| n026 | microsoft 365 | family 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | — | B | 13 / 586 |
| n027 | майкрософт 365 | family 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | — | B | 14 / 599 |
| n028 | офис 365 | family 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | — | B | 19 / 709 |
| n029 | office 365 | family 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | — | B | 20 / 736 |
| n030 | 365 для семьи | family 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | — | A | 7 / 246 |
| n031 | м365 | family 165034 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-for-family | — | B | 15 / 646 |
| n032 | 365 персональный | family 283494 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-365-personal | — | A | 11 / 360 |
| n033 | visio | family 391350,391347 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-visio-standard-2024 | — | A | 7 / 74 |
| n034 | ms project | family 391344,391341 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-project-standard-2024 | — | B | 6 / 310 |
| n035 | windows 11 | family 286676,310176 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-windows-professional-11 | — | A | 4 / 11 |
| n036 | виндовс 11 про | family 286676 | да | да | 1 | family https://store.softline.ru/microsoft/microsoft-windows-professional-11 | — | B | 15 / 141 |
| n037 | касперский small office security | family 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | — | B | 16 / 1020 |
| n038 | касперского смолл офис секьюрити | family 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | — | B | 8 / 650 |
| n039 | kaspersky small office | family 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | — | B | 4 / 99 |
| n040 | ksos | family 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | — | B | 12 / 944 |
| n041 | kaspersky smal ofice security | family 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | — | B | 15 / 1022 |
| n042 | продление kaspersky small office security | family 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | — | B | 4 / 51 |
| n043 | касперский смол офис на 10 пк | family 3761 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-small-office-security | — | B | 4 / 213 |
| n044 | касперский | vendor kaspersky | да | да | 1 | vendor https://store.softline.ru/kaspersky | — | B | 4 / 480 |
| n045 | kaspersky | vendor kaspersky | да | да | 1 | vendor https://store.softline.ru/kaspersky | — | B | 4 / 78 |
| n046 | касперского | vendor kaspersky | да | да | 1 | vendor https://store.softline.ru/kaspersky | — | B | 4 / 480 |
| n047 | каспер | vendor kaspersky | да | да | 1 | vendor https://store.softline.ru/kaspersky | — | B | 4 / 480 |
| n048 | лаборатория касперского | vendor kaspersky | да | да | 1 | vendor https://store.softline.ru/kaspersky | — | B | 4 / 480 |
| n049 | kaspersky endpoint security | vendor_category kaspersky+650 | да | да | 1 | vendor_category https://store.softline.ru/kaspersky?filter%5Bcategory%5D[]=650 | — | B | 19 / 1271 |
| n050 | касперский эндпоинт стандартный | family 3953 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-endpoint-security-dlya-biznesa-standartnyiy | — | B | 4 / 279 |
| n051 | kes расширенный | family 3935 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-endpoint-security-dlya-biznesa-rasshirennyiy | — | B | 14 / 996 |
| n052 | kaspersky total security для бизнеса | family 3971 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-total-security-dlya-biznesa | — | B | 20 / 1302 |
| n053 | касперский для почтовых серверов | family 3707 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-security-dlya-pochtovyih-serverov | — | B | 20 / 1227 |
| n054 | kaspersky endpoint security cloud | family 4052 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-endpoint-security-dlya-biznesa-cloud | — | B | 17 / 1251 |
| n055 | kaspersky standard | family 341252 | да | да | 1 | family https://store.softline.ru/kaspersky/kaspersky-standard | — | B | 4 / 452 |
| n056 | криптопро csp | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 12 / 622 |
| n057 | крипто про цсп | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 10 / 486 |
| n058 | cryptopro csp 5.0 | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 13 / 631 |
| n059 | криптопро csp 5 | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 11 / 620 |
| n060 | крипто-про csp | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 12 / 622 |
| n061 | rhbgnjghj csp | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 12 / 622 |
| n062 | лицензия криптопро csp | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 13 / 631 |
| n063 | крипто про сsp 5.0 | family 22268 | да | да | 1 | family https://store.softline.ru/kripto-pro/cryptopro-csp | — | B | 12 / 629 |
| n064 | криптопро | vendor kripto-pro | да | да | 1 | vendor https://store.softline.ru/kripto-pro | — | B | 13 / 631 |
| n065 | крипто про | vendor kripto-pro | да | да | 1 | vendor https://store.softline.ru/kripto-pro | — | B | 13 / 631 |
| n066 | cryptopro | vendor kripto-pro | да | да | 1 | vendor https://store.softline.ru/kripto-pro | — | B | 13 / 631 |
| n067 | криптопро jcp | family 22211 | да | да | 1 | family https://store.softline.ru/kripto-pro/kripto-pro-jcp | — | B | 12 / 622 |
| n068 | криптопро ngate | family 167189 | да | да | 1 | family https://store.softline.ru/kripto-pro/kriptopro-ngate | — | B | 4 / 481 |
| n069 | криптопро tsp | family 22181 | да | да | 1 | family https://store.softline.ru/kripto-pro/kripto-pro-tsp | — | B | 8 / 421 |
| n070 | криптопро csp tls | family 385280 | да | да | 1 | family https://store.softline.ru/kripto-pro/kriptopro-csp-tls | — | B | 11 / 488 |
| n071 | энидеск | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| n072 | эни деск | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| n073 | any desk | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| n074 | anydesk лицензия | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| n075 | энидекс | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| n076 | фтнвуыл | family 306539 | да | да | 1 | family https://store.softline.ru/anydesk/anydesk | — | B | 4 / 30 |
| n077 | кибербэкап | vendor_category cyberprotect+1033 | да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 5 / 478 |
| n078 | кибер бекап | vendor_category cyberprotect+1033 | да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 5 / 478 |
| n079 | cyber backup | vendor_category cyberprotect+1033 | да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 5 / 478 |
| n080 | киберпротект бэкап | vendor_category cyberprotect+1033 | да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 5 / 478 |
| n081 | кибер бэкапа | vendor_category cyberprotect+1033 | да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 4 / 328 |
| n082 | кибер бэкап расширенная редакция | family 173615 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-bekap-rasshirennaya | — | B | 5 / 478 |
| n083 | кибер бэкап стандартная | family 173614 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-bekap | — | B | 5 / 478 |
| n084 | кибер бэкап старт | family 444870 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-bekap-start | — | B | 4 / 328 |
| n085 | кибер бэкап облачный | family 442025 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-bekap-oblacnyi | — | B | 5 / 478 |
| n086 | кибер хранилище | family 412674 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-khranilishche | — | A | 20 / 169 |
| n087 | кибер файлы | family 394620 | да | да | 1 | family https://store.softline.ru/cyberprotect/kiber-fayly | — | A | 7 / 662 |
| n088 | киберпротект | vendor cyberprotect | да | да | 1 | vendor https://store.softline.ru/cyberprotect | — | B | 7 / 662 |
| n089 | акронис | vendor_category cyberprotect+1033 | да | да | 1 | vendor_category https://store.softline.ru/cyberprotect?filter%5Bcategory%5D[]=1033 | — | B | 4 / 328 |
| n090 | сканер вс | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | B | 9 / 482 |
| n091 | сканервс | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | B | 9 / 482 |
| n092 | skaner vs | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | B | 9 / 487 |
| n093 | эшелон сканер-вс | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | A | 12 / 531 |
| n094 | сканера вс | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | B | 9 / 482 |
| n095 | сканер-вс 6 | family 46421 | да | да | 1 | family https://store.softline.ru/npo-echelon/eshelon-skaner-vs | — | A | 8 / 481 |
| n096 | эшелон | vendor npo-echelon | да | да | 1 | vendor https://store.softline.ru/npo-echelon | — | A | 6 / 485 |
| n097 | ак-вс 3 | family 411254 | да | да | 1 | family https://store.softline.ru/npo-echelon/pk-ak-vs-3 | — | A | 4 / 143 |
| n098 | мтс линк | vendor webinar | да | да | 1 | vendor https://store.softline.ru/webinar | — | B | 4 / 10 |
| n099 | mts link | vendor webinar | да | да | 1 | vendor https://store.softline.ru/webinar | — | B | 4 / 10 |
| n100 | мтслинк | vendor webinar | да | да | 1 | vendor https://store.softline.ru/webinar | — | B | 4 / 10 |
| n101 | вебинар | vendor webinar | да | да | 1 | vendor https://store.softline.ru/webinar | — | B | 4 / 10 |
| n102 | webinar | vendor webinar | да | да | 1 | vendor https://store.softline.ru/webinar | — | B | 4 / 10 |
| n103 | мтс линка | vendor webinar | да | да | 1 | vendor https://store.softline.ru/webinar | — | B | 4 / 10 |
| n104 | мтс вебинары | vendor webinar | да | да | 1 | vendor https://store.softline.ru/webinar | — | B | 4 / 10 |
| n105 | мтс линк про | family 447243 | да | да | 1 | family https://store.softline.ru/webinar/mts-link-pro | — | B | 4 / 10 |
| n106 | мтс линк стандарт | family 447242 | да | да | 1 | family https://store.softline.ru/webinar/mts-link-standart | — | B | 4 / 10 |
| n107 | макс патрол | family 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | B | 4 / 3156 |
| n108 | maxpatrol | family 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | A | 4 / 3156 |
| n109 | макспатрол | family 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | B | 4 / 3156 |
| n110 | max patrol epp | family 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | A | 4 / 3156 |
| n111 | maxpatrol endpoint protection | family 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | A | 7 / 4044 |
| n112 | макс патрола | family 437421 | да | да | 1 | family https://store.softline.ru/positive-technologies/maxpatrol-endpoint-protection-platform | — | B | 4 / 3156 |
| n113 | xspider | family 28574,428013 | да | да | 1 | family https://store.softline.ru/positive-technologies/positive-technologies-xspider | — | A | 4 / 146 |
| n114 | позитив технолоджис | vendor positive-technologies | да | да | 1 | vendor https://store.softline.ru/positive-technologies | — | B | 4 / 138 |
