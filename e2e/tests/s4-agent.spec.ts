// S4 контура автономии «Агент работает сам»: кадр 1 — «Запуски» с режимом, триггерами
// и итогом; кадр 2 — память в ленте и вкладка «Память»; плашка «Очереди»; кадр 3 — «Разобрать» из трассы на живом
// API и модели (медленный: только E2E_LIVE_AGENT=1, создаёт настоящий запуск и, возможно, предложение в очереди).
// Кадры 1–2 только читают. E2E_SCREENS=1 — снимки 1440/360 в artifacts/screens/p5/.
import type { Page } from '@playwright/test'
import { expect, openSearch, search, test, tracePanel } from './fixtures'

const OUT = '../artifacts/screens/p5'
const SHOTS = process.env.E2E_SCREENS === '1'
const LIVE = process.env.E2E_LIVE_AGENT === '1'
const QUERY = process.env.E2E_AGENT_QUERY ?? 'чертилка сапр'
const TRIGGER_LABELS = ['расписание', 'всплеск', 'каталог', 'вручную']

async function shot(page: Page, name: string) {
  if (!SHOTS) return
  for (const w of [1440, 360]) {
    await page.setViewportSize({ width: w, height: w === 360 ? 780 : 900 })
    await page.waitForTimeout(300)
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    )
    expect(overflow, `${name} ${w}: горизонтальная прокрутка`).toBeLessThanOrEqual(0)
    await page.screenshot({ path: `${OUT}/${name}-${w}.png`, fullPage: true })
  }
  await page.setViewportSize({ width: 1440, height: 900 })
}

interface RunLite {
  id: string
  trigger: string
  mode?: string
  plan?: unknown
  budget?: unknown
}

/** Запуск контура автономии, в ленте которого есть шаг нужного вида (новые сверху, не больше 20 запусков). */
async function findRunWithStep(page: Page, kinds: string[]): Promise<string | null> {
  const list = (await (await page.request.get('/api/agent/runs')).json()) as { items: RunLite[] }
  // Запуски пробелов подбора (mode=gaps) — без модели и плана: для кадра «память» не годятся.
  for (const r of list.items
    .filter((x) => (x.plan || x.budget) && x.mode !== 'gaps')
    .slice(0, 20)) {
    const d = (await (await page.request.get(`/api/agent/runs/${r.id}`)).json()) as {
      steps: { kind: string }[]
    }
    if (d.steps.some((s) => kinds.includes(s.kind))) return r.id
  }
  return null
}

test.describe('S4 контура автономии: агент работает сам', () => {
  test('кадр 1: «Запуски» — режим, расписание и пороги; триггер, план, итог, стоимость; фильтр по триггеру', async ({
    page,
  }) => {
    await page.goto('/searchops?tab=runs')
    const mode = page.getByRole('region', { name: 'Режим агента' })
    await expect(mode.getByTestId('agent-mode')).toHaveText(/Предлагает правки|Только чтение/)
    await expect(mode.getByTestId('agent-line')).not.toBeEmpty()

    const runs = page.getByRole('region', { name: 'Запуски агента' })
    await expect(runs).toBeVisible()
    for (const h of ['Время', 'Триггер', 'План', 'Итог', 'Стоимость', 'Длительность'])
      await expect(runs.getByRole('columnheader', { name: h })).toBeVisible()
    const triggers = (await runs.getByTestId('run-trigger').allTextContents()).map((t) => t.trim())
    expect(triggers.length).toBeGreaterThan(0)
    expect(triggers.every((t) => TRIGGER_LABELS.includes(t))).toBe(true)
    const outcomes = await runs.getByTestId('run-outcome').allTextContents()
    expect(outcomes.every((o) => /\d+ предложени/.test(o))).toBe(true)
    test.info().annotations.push({
      type: 'триггеры на стенде',
      description: [...new Set(triggers)].join(', '),
    })
    await shot(page, 's4-1-runs')

    // Фильтр по триггеру: остаются строки одного вида.
    await page.getByRole('combobox', { name: 'Триггер' }).click()
    await page.getByRole('option', { name: 'вручную' }).click()
    const shown = await runs.getByTestId('run-trigger').allTextContents()
    expect(shown.every((t) => t.trim() === 'вручную')).toBe(true)
  })

  test('кадр 2: память в ленте выделена и подписана; «Память» — три вкладки со ссылками на запуск', async ({
    page,
  }) => {
    const strong = await findRunWithStep(page, ['memory_skip', 'effect_check'])
    const id = strong ?? (await findRunWithStep(page, ['memory_read']))
    test.info().annotations.push({
      type: 'кадр 2',
      description: strong
        ? `запуск ${strong} с пропуском по памяти или проверкой эффекта`
        : 'на стенде ещё нет запуска с пропуском по памяти / проверкой эффекта — проверено чтение памяти',
    })
    if (id) {
      await page.goto(`/searchops/runs/${id}`)
      const feed = page.getByRole('list', { name: 'Шаги агента' })
      await expect(feed).toBeVisible()
      const tag = feed.getByTestId('step-tag').filter({
        hasText: strong ? /память · пропуск|проверка эффекта/ : /память/,
      })
      await expect(tag.first()).toBeVisible()
      await expect(page.getByRole('region', { name: 'Бюджет запуска' })).toContainText(' из ')
      await tag.first().scrollIntoViewIfNeeded()
      await shot(page, 's4-2-feed-memory')
    }

    await page.goto('/searchops?tab=memory')
    const layers = page.getByRole('tablist', { name: 'Слои памяти агента' })
    await expect(layers.getByRole('tab')).toHaveText([/^Решения/, /^Не предлагать/, /^Эпизоды/])
    const decisions = page.getByRole('region', { name: 'Решения' })
    const empty = page.getByText('Решений пока нет')
    await expect(decisions.or(empty).first()).toBeVisible()
    if (await decisions.isVisible()) {
      for (const h of [
        'Предложение',
        'Причина агента',
        'Решение человека',
        'Причина человека',
        'Запуск',
      ])
        await expect(decisions.getByRole('columnheader', { name: h })).toBeVisible()
      await expect(decisions.locator('a[href^="/searchops/runs/"]').first()).toBeVisible()
    }
    await shot(page, 's4-2-memory-decisions')
    await layers.getByRole('tab', { name: /^Не предлагать/ }).click()
    await expect(page).toHaveURL(/mem=suppress/)
    await expect(
      page
        .getByRole('region', { name: 'Не предлагать' })
        .or(page.getByText('Отклонённого человеком пока нет'))
        .first(),
    ).toBeVisible()
    await shot(page, 's4-2-memory-suppress')
    await layers.getByRole('tab', { name: /^Эпизоды/ }).click()
    await expect(
      page
        .getByRole('region', { name: 'Эпизоды' })
        .or(page.getByText(/Эпизодов пока нет/))
        .first(),
    ).toBeVisible()
    await shot(page, 's4-2-memory-episodes')
  })

  test('«Очередь»: плашка «N предложений от …» и у карточки — откуда она', async ({ page }) => {
    const res = await page.request.get('/api/proposals?status=pending')
    const items = (
      (await res.json()) as { items: { id: string; trigger?: string; run_id?: string }[] }
    ).items
    const withOrigin = items.filter((p) => p.trigger && p.run_id)
    test.skip(!withOrigin.length, 'нет ждущих предложений с триггером запуска')
    await page.goto('/searchops?tab=queue')
    await expect(page.getByTestId('queue-banner')).toContainText(/\d+ предложени\S* от /)
    const card = page.locator(`#proposal-${withOrigin[0]!.id}`)
    await expect(card.getByTestId('origin')).toContainText('запуск')
    await expect(card.getByTestId('cause')).not.toBeEmpty()
    await expect(card.getByTestId('owner')).toContainText('Владелец:')
    await shot(page, 's4-queue')
  })
})

test.describe('S4 контура автономии, кадр 3 — живой запуск на модели (медленный)', () => {
  test.use({ allowDestructive: LIVE })

  test('кадр 3: запрос → трасса → «Разобрать» → план, шаги, итог за 90 с', async ({ page }) => {
    test.skip(!LIVE, 'живой запуск агента на модели: E2E_LIVE_AGENT=1')
    test.slow()
    const started = Date.now()
    await openSearch(page)
    await search(page, QUERY)
    await page.getByRole('switch', { name: 'Трасса' }).click()
    const panel = tracePanel(page)
    await expect(panel).toBeVisible()
    await panel.getByRole('button', { name: 'Разобрать' }).click()

    await expect(page).toHaveURL(/\/searchops\/runs\//, { timeout: 15_000 })
    await expect(
      page.getByRole('heading', { level: 1, name: `Разбор запроса «${QUERY}»` }),
    ).toBeVisible()
    await expect(page.getByText('триггер: вручную')).toBeVisible()
    await shot(page, 's4-3-live-running')

    // Запуск закончился: статус «Завершён» и итог — в пределах 90 с от начала кадра.
    const left = Math.max(5_000, 90_000 - (Date.now() - started))
    await expect(page.locator('.run__actions .badge')).toHaveText(/Завершён|Остановлен|Ошибка/, {
      timeout: left,
    })
    const seconds = Math.round((Date.now() - started) / 1000)
    test.info().annotations.push({ type: 'кадр 3', description: `${seconds} с` })
    expect(seconds).toBeLessThanOrEqual(90)
    await expect(page.getByTestId('run-outcome')).toContainText(/предложени/)
    await expect(page.getByRole('region', { name: 'Бюджет запуска' })).toContainText(' из ')
    // Либо поиск уже отвечает верно (без модели), либо план от модели и шаги.
    const solved = page.getByText(/поиск уже отвечает верно/)
    if (!(await solved.isVisible())) {
      await expect(page.getByRole('region', { name: 'План запуска' })).toBeVisible()
    }
    await shot(page, 's4-3-live-done')
  })
})
