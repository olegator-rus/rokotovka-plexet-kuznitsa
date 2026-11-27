<!-- Контур помощника: оценка помощника по выбору. -->
> **Контур помощника, помощник по выбору.** Тот же набор, что видит экран «На данных заказчика»: **52 диалога** — 48 диалогов
> контура подбора (36 по вендорам эталонов и 12 отрицательных) и 4 диалога группы `practical`, итого 40 по вендорам и
> 12 отрицательных. Прежние документы называют 48 — это набор до добавления `practical`. Каждый ход идёт через ядро
> `POST /api/guide`, которое собирает сообщение чата (`chat`); к автопроверкам добавлены две проверки
> сообщения помощника (строки «Сообщение помощника…» и «Суммы на карточках…» ниже). Ходы — те же вызовы, что шлёт панель: первая
> реплика — `query` без состояния, следующие — `message` с состоянием и исходным запросом, кнопки — `chip`.

# Оценка режима подбора

Каталог: фид заказчика · коммит b85703f+dirty · прогон …69bef9 · диалогов 52 (по вендорам 40, отрицательных 12) · прогонов 3.

Команда: `go run . guide-eval -runs 3 -report ../data/softline/eval/results/REPORT-guide.md`

## Гейт — **пройден**

| условие | полный подбор | на чём проверено |
|---|---|---|
| выдуманные идентификаторы офферов и фактов | 0 | 0 из 574 |
| выдуманные или неверные цены и суммы | 0 | 0 из 426 |
| нарушения правил лицензий у подходящих карточек | 0 | 0 из 252 |
| утверждения о свойстве без факта | 0 из 109 | 0 из 109 |

## Задачи

| вариант | решено 3 из 3 | по вендорам | отрицательные | решено по прогонам |
|---|---|---|---|---|
| поиск без подбора | 15 из 52 (один прогон) | 12 из 40 | 3 из 12 | 15 из 52 |
| модель с поиском без проверок (эмуляция) | 51 из 52 | 39 из 40 | 12 из 12 | 154 из 156 |
| полный подбор | 52 из 52 | 40 из 40 | 12 из 12 | 156 из 156 |

Задача решена — во всех прогонах выполнены ожидания каждого хода (тип ответа, вопрос, подходящее семейство, блок «Не подходит», факты, «по запросу», изменения) и ни одна автопроверка не нарушена.

## Автопроверки на ответе

| проверка | поиск без подбора | без проверок (эмуляция) | полный подбор |
|---|---|---|---|
| Сообщение помощника: до трёх фраз, без слов из списка «не писать», у карточки действие | — | 0 из 228 | 0 из 228 |
| Суммы на карточках и в таблице помощника — из карточек каталога | — | 0 из 228 | 0 из 228 |
| У каждого утверждения о свойстве есть факт | — | 0 из 109 | 0 из 109 |
| Нет фактов ниже порога показа | — | 0 из 268 | 0 из 268 |
| Нет запрещённых вызовов | — | 0 из 684 | 0 из 684 |
| Подходящие карточки — из семейств задачи | — | 0 из 252 | 0 из 252 |
| Идентификаторы офферов и фактов существуют | 0 из 252 | 0 из 574 | 0 из 574 |
| Во фразе нет чисел, которых нет в карточках и условиях | — | 0 из 195 | 0 из 195 |
| Во фразе нет запрещённых слов | — | 0 из 195 | 0 из 195 |
| Правила лицензий не нарушены | 5 из 63 | 0 из 252 | 0 из 252 |
| Цена и сумма в карточке совпадают с каталогом | 0 из 252 | 0 из 426 | 0 из 426 |
| Вопросов в диалоге не больше допустимого | — | 0 из 156 | 0 из 156 |
| Вызовов модели и времени хода не больше предела | — | 0 из 228 | 0 из 228 |

Число — нарушений из проверенного (карточек, фактов, ходов, диалогов). У поиска проверены первые четыре карточки и правила лицензий у первой карточки при требованиях, которые разобрал подбор; вопросов, фраз и утверждений у поиска нет.

Сняли проверки до показа (по классам, все прогоны): lead — 2.

- Эмуляция: тот же ход подбора, к показанному добавлено то, что сняли проверки до показа (guide_turns.removed). Карточки, цены и артикулы и в этом варианте строит код: выдуманной цены модель дать не может, поэтому «цены» здесь не отличаются от полного подбора.

## Время и расход полного подбора

| момент | p50, мс | p95, мс |
|---|---|---|
| первый вывод (офферы собраны) | 2 | 218 |
| проверенные строки | 1 | 287 |
| весь ход | 59 | 598 |
| весь ход с вызовом модели (9 ходов) | 1249 | 1888 |

Ходов 228; по журналу `guide_turns` — 228 строк: вызовов модели 9, токенов на входе 6 327, на выходе 481, 0,7289 ₽. По `llm_usage` (request_id хода): вызовов 9, 0,7289 ₽. Стоимость диалога в среднем — **0,0047 ₽** (156 диалогов-прогонов); диалога, где модель вызывалась (9), — 0,0810 ₽.

## Ошибки

поиск без подбора:

- `astralinux_clarify`, прогон 1, ход 1 — нет в поиске: вопрос, сравнение, факты, «не подходит» или запрос КП
- `astralinux_change`, прогон 1, ход 2 — чипа и правки условия в поиске нет
- `microsoft_clarify`, прогон 1, ход 2 — первая карточка поиска — семейство offer-428243
- `microsoft_change`, прогон 1, ход 2 — чипа и правки условия в поиске нет
- `microsoft_trap`, прогон 1, ход 1 — первая карточка поиска: тип организации government ≠ commercial
- `kaspersky_clarify`, прогон 1, ход 1 — нет в поиске: вопрос, сравнение, факты, «не подходит» или запрос КП

## Диалоги полного подбора

| диалог | тип | прогоны | типы ответов (прогон 1) | вызовов модели | причина |
|---|---|---|---|---|---|
| astralinux_narrow | narrow | 3 из 3 | selection | 0 |  |
| astralinux_clarify | clarify | 3 из 3 | clarification → comparison → selection | 0 |  |
| astralinux_change | change | 3 из 3 | selection → selection | 0 |  |
| astralinux_trap | trap | 3 из 3 | clarification | 0 |  |
| microsoft_narrow | narrow | 3 из 3 | selection | 0 |  |
| microsoft_clarify | clarify | 3 из 3 | selection → selection | 0 |  |
| microsoft_change | change | 3 из 3 | selection → selection | 0 |  |
| microsoft_trap | trap | 3 из 3 | selection → selection | 0 |  |
| kaspersky_narrow | narrow | 3 из 3 | selection | 0 |  |
| kaspersky_clarify | clarify | 3 из 3 | clarification → selection → comparison → selection | 3 |  |
| kaspersky_change | change | 3 из 3 | selection → selection | 0 |  |
| kaspersky_trap | trap | 3 из 3 | selection | 0 |  |
| kripto-pro_narrow | narrow | 3 из 3 | selection | 0 |  |
| kripto-pro_clarify | clarify | 3 из 3 | clarification → selection | 0 |  |
| kripto-pro_change | change | 3 из 3 | selection → selection | 0 |  |
| kripto-pro_trap | trap | 3 из 3 | selection → selection | 0 |  |
| anydesk_narrow | narrow | 3 из 3 | clarification → selection | 0 |  |
| anydesk_clarify | clarify | 3 из 3 | clarification → selection | 0 |  |
| anydesk_change | change | 3 из 3 | selection → selection → selection | 0 |  |
| anydesk_trap | trap | 3 из 3 | clarification | 0 |  |
| cyberprotect_narrow | narrow | 3 из 3 | selection | 0 |  |
| cyberprotect_clarify | clarify | 3 из 3 | clarification → selection | 0 |  |
| cyberprotect_change | change | 3 из 3 | selection → selection | 3 |  |
| cyberprotect_trap | trap | 3 из 3 | selection | 0 |  |
| npo-echelon_narrow | narrow | 3 из 3 | selection | 0 |  |
| npo-echelon_clarify | clarify | 3 из 3 | selection → selection | 0 |  |
| npo-echelon_change | change | 3 из 3 | selection → selection | 3 |  |
| npo-echelon_trap | trap | 3 из 3 | selection | 0 |  |
| webinar_narrow | narrow | 3 из 3 | selection | 0 |  |
| webinar_clarify | clarify | 3 из 3 | no_data | 0 |  |
| webinar_change | change | 3 из 3 | no_data → no_data | 0 |  |
| webinar_trap | trap | 3 из 3 | selection | 0 |  |
| positive-technologies_narrow | narrow | 3 из 3 | selection | 0 |  |
| positive-technologies_clarify | clarify | 3 из 3 | no_data | 0 |  |
| positive-technologies_change | change | 3 из 3 | selection → selection | 0 |  |
| positive-technologies_trap | trap | 3 из 3 | selection | 0 |  |
| practical_antivirus_vague | practical | 3 из 3 | clarification → selection | 0 |  |
| practical_antivirus_office | practical | 3 из 3 | selection | 0 |  |
| practical_drweb_pack | practical | 3 из 3 | selection | 0 |  |
| practical_ksos_limit | practical | 3 из 3 | selection | 0 |  |
| negative_01 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_02 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_03 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_04 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_05 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_06 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_07 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_08 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_09 | negative | 3 из 3 | out_of_scope | 0 |  |
| negative_10 | negative | 3 из 3 | clarification | 0 |  |
| negative_11 | negative | 3 из 3 | clarification | 0 |  |
| negative_12 | negative | 3 из 3 | no_data | 0 |  |
