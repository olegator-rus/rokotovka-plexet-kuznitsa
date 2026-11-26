// Контур фида (кадры 1–2): первый экран SearchOps — отчёт для маркетолога на фиде заказчика.
// Четыре раздела, подпись потока «тестовый поток: запросы заказчика, прогоны сайта, синтетика из каталога», частоты
// модельные, рублей и дат нет; строка раскрывается в ленту шагов агента. Контракт — подменённым ответом (не зависит
// от журнала), живой отчёт — на текущей базе. Демо-состояние не меняется.
// E2E_SOPS_SHOTS=<каталог> — снимки первого экрана 360 и 1440 в этот каталог.
import type { Page } from '@playwright/test'
import { expect, test } from './fixtures'

const SHOTS = process.env.E2E_SOPS_SHOTS ?? ''
const SECTIONS = [
  'Спрос без предложения',
  'Товар есть, но не найден',
  'Слова клиента, которых нет в каталоге',
  'Сервисные запросы',
]

async function noOverflow(page: Page, name: string) {
  const overflow = await page.evaluate(
    () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
  )
  expect(overflow, `${name}: горизонтальная прокрутка`).toBeLessThanOrEqual(0)
}

const GUIDE = 'Вопросы, на которые подбор не ответил'
const RUN = '01b00000-0000-7000-8000-000000000001'
const CLUSTER = '01b00000-0000-7000-8000-0000000000c1'

const REPORT = {
  catalog_version: 'softline-feed',
  rules_version: 4,
  days: 7,
  stream: {
    label: 'тестовый поток: запросы заказчика, прогоны сайта, синтетика из каталога',
    freq_label: 'частоты модельные',
    layers: [
      {
        code: 'customer',
        label: 'запросы заказчика',
        queries: 17,
        searches: 122,
      },
      { code: 'site', label: 'прогоны сайта', queries: 31, searches: 124 },
      {
        code: 'synthetic',
        label: 'синтетика из каталога',
        queries: 103,
        searches: 286,
      },
    ],
  },
  totals: {
    searches: 532,
    failed_searches: 140,
    queries: 151,
    failed_queries: 40,
  },
  sections: [
    {
      code: 'no_offer',
      title: SECTIONS[0],
      hint: 'Ищут то, что нельзя купить.',
      searches: 3,
      failures: 3,
      rows: [
        {
          query: 'microsoft visual studio',
          freq: 3,
          failures: 3,
          empty: 0,
          no_click: 3,
          reformulated: 0,
          layers: ['synthetic'],
          origin: '«Microsoft Visual Studio»: в фиде только «по запросу»',
          kind: 'on_request',
          kind_label: 'найдено, но только по запросу',
          evidence: 'первым — «Microsoft Visual Studio»: купить нельзя, только по запросу',
          found: {
            family_id: '102110',
            title: 'Microsoft Visual Studio',
            price_state: 'on_request',
          },
        },
      ],
    },
    {
      code: 'not_found',
      title: SECTIONS[1],
      hint: 'Товар продаётся, а поиск его не показал.',
      searches: 3,
      failures: 3,
      rows: [
        {
          query: 'кибер бэкап расширенаня редакция',
          freq: 3,
          failures: 3,
          empty: 0,
          no_click: 3,
          reformulated: 0,
          layers: ['synthetic'],
          kind: 'not_understood',
          kind_label: 'поиск не понял слово',
          evidence: 'агент: опечатка в названии редакции',
          words: ['расширенаня'],
          agent: {
            code: 'proposal',
            label: 'есть предложение агента',
            diagnosis: 'TYPO',
            run_id: RUN,
            cluster_id: CLUSTER,
            cluster_label: 'расширенаня',
          },
          proposal: {
            id: 'p-1',
            status: 'pending',
            action: 'rules_patch',
            diagnosis: 'TYPO',
            run_id: RUN,
            cluster_id: CLUSTER,
          },
        },
      ],
    },
    {
      code: 'unknown_words',
      title: SECTIONS[2],
      hint: 'Слова из неудачных запросов.',
      searches: 3,
      failures: 3,
      rows: [
        {
          query: 'расширенаня',
          freq: 3,
          failures: 3,
          empty: 0,
          no_click: 3,
          reformulated: 0,
          layers: ['synthetic'],
          kind: 'unknown_word',
          kind_label: 'слова нет в каталоге',
          evidence: 'в 1 неудачном запросе: «кибер бэкап расширенаня редакция»',
          examples: ['кибер бэкап расширенаня редакция'],
        },
      ],
    },
    {
      code: 'service',
      title: SECTIONS[3],
      hint: 'Вопросы о заказе.',
      searches: 4,
      failures: 4,
      rows: [
        {
          query: 'доставка',
          freq: 4,
          failures: 4,
          empty: 4,
          no_click: 0,
          reformulated: 0,
          layers: ['site'],
          origin: 'прогон сайта №29, у сайта: провал',
          kind: 'service',
          kind_label: 'вопрос не о товаре',
          evidence: 'поиск отвечает справкой: вопрос не о товаре',
        },
      ],
    },
  ],
  pending: [],
  checked_at: '2025-07-03T00:00:00Z',
  guide: {
    title: GUIDE,
    hint: 'Покупатель спросил подбор, а ответа не нашлось.',
    cause: {
      code: 'kb_missing',
      label: 'Нет факта в базе знаний',
      owner: 'product',
      owner_label: 'Категорийный и лицензионный менеджер',
    },
    total: 5,
    rows: [
      {
        code: 'missing_fact',
        label: 'Нет факта в базе знаний',
        question: 'кибер бэкап стандартная поддерживает ленточные библиотеки?',
        count: 3,
        families: [{ family_id: '173614', title: 'Кибер Бэкап Стандартная редакция' }],
        kb_missing: true,
      },
      {
        code: 'no_matching_offer',
        label: 'Нет оффера под условия',
        question: 'мтс линк для вебинара на 150 человек',
        count: 2,
        families: [{ family_id: '447242', title: 'МТС Линк Стандарт' }],
        kb_missing: false,
      },
    ],
  },
}

const STEPS = [
  {
    kind: 'memory_read',
    cluster_id: null,
    title: 'Читаю память: решений 0, «не предлагать» — 0, эпизодов — 0',
  },
  { kind: 'plan', cluster_id: null, title: 'План: 1 кластер' },
  {
    kind: 'tool_call',
    cluster_id: CLUSTER,
    tool: 'search_catalog',
    title: 'Поиск по каталогу: «расширенаня» → Кибер Бэкап Расширенная редакция',
  },
  {
    kind: 'diagnosis',
    cluster_id: CLUSTER,
    title: 'Диагноз: TYPO — опечатка, которую поиск не исправил',
  },
  {
    kind: 'tool_call',
    cluster_id: 'другой',
    tool: 'get_search_trace',
    title: 'чужой кластер',
  },
].map((s, i) => ({
  id: `s${i}`,
  run_id: RUN,
  seq: i + 1,
  ts: '2025-07-03T00:00:00Z',
  tool: null,
  args: null,
  result: null,
  ok: true,
  hypothesis_no: null,
  rejected_reason: null,
  ms: 10,
  ...s,
}))

test.describe('Контур фида: отчёт SearchOps для маркетолога', () => {
  test('контракт: первый экран, четыре раздела, подпись потока, раскрытие строки в ленту агента', async ({
    page,
  }) => {
    await page.route('**/api/agent/report?**', (r) => r.fulfill({ json: REPORT }))
    await page.route(`**/api/agent/runs/${RUN}`, (r) =>
      r.fulfill({
        json: {
          run: {
            id: RUN,
            status: 'done',
            trigger: 'manual',
            started_at: '2025-07-03T00:00:00Z',
            finished_at: '2025-07-03T00:01:00Z',
            clusters_total: 1,
            clusters_done: 1,
            proposals_created: 1,
            tool_calls: 2,
            input_tokens: 1,
            output_tokens: 1,
            cost_rub: 1,
            stop_reason: null,
            summary: null,
            current_cluster: null,
          },
          steps: STEPS,
        },
      }),
    )
    await page.goto('/searchops')
    await expect(page.getByRole('tab', { name: 'Отчёт' })).toHaveAttribute('data-state', 'active')
    await expect(
      page.getByText('Тестовый поток: запросы заказчика, прогоны сайта, синтетика из каталога'),
    ).toBeVisible()
    await expect(page.getByText(/Частоты модельные/)).toBeVisible()
    await expect(page.getByRole('heading', { level: 3 })).toHaveText([...SECTIONS, GUIDE])
    const panel = page.locator('.mrep')
    await expect(panel).not.toContainText('₽')
    await expect(panel).not.toContainText(/\d{2}\.\d{2}\.\d{4}/)

    const onReq = page.locator('[data-section="no_offer"]')
    await expect(onReq).toContainText('microsoft visual studio')
    await expect(onReq).toContainText('найдено, но только по запросу')
    await expect(onReq).toContainText('синтетика')

    const notFound = page.locator('[data-section="not_found"]')
    const row = notFound.getByRole('button', {
      name: /кибер бэкап расширенаня редакция/,
    })
    await expect(row).toContainText('правка ждёт утверждения')
    await row.click()
    await expect(row).toHaveAttribute('aria-expanded', 'true')
    const steps = notFound.getByRole('list', {
      name: 'Шаги агента по запросу',
    })
    await expect(steps.getByRole('listitem')).toHaveCount(3)
    await expect(steps).toContainText('Читаю память')
    await expect(steps).toContainText('Диагноз: TYPO')
    await expect(steps).not.toContainText('чужой кластер')
    await expect(notFound.getByRole('link', { name: 'Открыть предложение' })).toHaveAttribute(
      'href',
      /tab=queue.*p=p-1/,
    )
    await expect(notFound.getByRole('link', { name: 'Весь запуск' })).toHaveAttribute(
      'href',
      `/searchops/runs/${RUN}?cluster=${CLUSTER}`,
    )

    // вопросы, на которые подбор не ответил — вопрос, сколько раз, продукт, чего не хватило; задача — только
    // для «нет факта в базе знаний» (запуск агента mode=gaps, без модели).
    const guide = page.locator('[data-section="guide"]')
    await expect(guide).toContainText(
      '«кибер бэкап стандартная поддерживает ленточные библиотеки?»',
    )
    await expect(guide).toContainText('3 раза')
    await expect(guide).toContainText('Кибер Бэкап Стандартная редакция')
    await expect(guide).toContainText('Не хватило: Нет факта в базе знаний')
    await expect(guide).toContainText('владелец: Категорийный и лицензионный менеджер')
    await expect(guide.getByRole('button', { name: 'Создать задачу каталогу' })).toHaveCount(1)
    let body: unknown = null
    await page.route('**/api/agent/runs', async (r) => {
      if (r.request().method() !== 'POST') return r.fallback()
      body = r.request().postDataJSON()
      await r.fulfill({ status: 202, json: { run_id: 'gaps-run' } })
    })
    await page.route('**/api/agent/runs/gaps-run', (r) =>
      r.fulfill({
        json: {
          run: { id: 'gaps-run', status: 'done', summary: 'Разобрано пробелов подбора: 1' },
          steps: [],
        },
      }),
    )
    await guide.getByRole('button', { name: 'Создать задачу каталогу' }).click()
    await expect(page.getByText('Задачи по пробелам подбора созданы').first()).toBeVisible()
    expect(body).toEqual({ mode: 'gaps', trigger: 'manual', days: 7, max_clusters: 1 })
  })

  test('живой отчёт: разделы на месте, без горизонтальной прокрутки на 360 и 1440', async ({
    page,
  }) => {
    for (const [w, h] of [
      [1440, 900],
      [360, 780],
    ] as const) {
      await page.setViewportSize({ width: w, height: h })
      await page.goto('/searchops')
      const loaded = page.getByRole('heading', { level: 3, name: SECTIONS[0] })
      const failed = page.getByText('Отчёт не загрузился')
      await expect(loaded.or(failed).first()).toBeVisible({ timeout: 30_000 })
      await expect(failed).toBeHidden()
      await expect(page.getByRole('heading', { level: 3 })).toHaveText(
        SECTIONS.concat(
          (await page.getByRole('heading', { level: 3, name: GUIDE }).count()) ? [GUIDE] : [],
          (await page.getByRole('heading', { level: 3, name: 'Ещё не разобраны' }).count())
            ? ['Ещё не разобраны']
            : [],
        ),
      )
      await expect(page.locator('.mrep')).not.toContainText('₽')
      await noOverflow(page, `отчёт ${w}`)
      if (SHOTS) {
        await page.screenshot({
          path: `${SHOTS}/report-${w}.png`,
          fullPage: w === 1440,
        })
        await page.screenshot({ path: `${SHOTS}/report-${w}-screen.png` })
        const guide = page.locator('[data-section="guide"]')
        if (await guide.count()) await guide.screenshot({ path: `${SHOTS}/report-guide-${w}.png` })
      }
    }
  })

  test('живой отчёт: строка «товар есть, но не найден» раскрывается в ленту настоящего запуска', async ({
    page,
  }) => {
    await page.setViewportSize({ width: 1440, height: 900 })
    await page.goto('/searchops')
    const sec = page.locator('[data-section="not_found"]')
    await expect(sec.getByRole('heading', { level: 3 })).toBeVisible({
      timeout: 30_000,
    })
    const rows = sec
      .getByRole('button', { expanded: false })
      .filter({ hasText: /ждёт|принят|утвержд|агент/ })
    test.skip((await rows.count()) === 0, 'в разделе нет строк, которые агент уже разобрал')
    const row = rows.first()
    await row.click()
    const steps = sec.getByRole('list', { name: 'Шаги агента по запросу' })
    await expect(steps).toBeVisible()
    await expect(steps).toContainText('Читаю память')
    await expect(sec.getByRole('link', { name: 'Весь запуск' })).toBeVisible()
    if (SHOTS) {
      await sec.screenshot({ path: `${SHOTS}/report-1440-row-feed.png` })
    }
  })
})
