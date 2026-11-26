// Сценарий видео одним дублем (≈2:40).
// Запись 1920×1080 без звука — в artifacts/video/run/ (в git не идёт).
//
// Режимы:
//   VIDEO_PACE=1 (по умолчанию) — паузы для чтения; VIDEO_PACE=0 — быстрая проверка сценария (make e2e).
//   без VIDEO_ACCEPT          — диалог «Принять правку?» открывается и закрывается «Отменой»; шаг
//                               «акад находит AutoCAD» пропускается. Демо не меняется.
//   VIDEO_ACCEPT=1            — принятие «акад» на ПОДМЕНЁННЫХ ответах API (approve, очередь, мета, поиск
//                               «акад» в B4): база не меняется, сценарий проходит целиком.
//   VIDEO_ACCEPT=1 VIDEO_LIVE=1 — живой дубль: настоящее принятие «акад» → версия правил N+1. Только
//                               человек/оркестратор в конце, один раз; после него «акад» в очереди уже принят.
import type { Page, Route } from '@playwright/test'
import {
  MAIN_RUN_ID,
  PACE,
  expect,
  openSearch,
  pause,
  resultCards,
  search,
  searchResponse,
  test,
  tracePanel,
  understanding,
} from './fixtures'

const ACCEPT = process.env.VIDEO_ACCEPT === '1'
const LIVE = ACCEPT && process.env.VIDEO_LIVE === '1'
const TYPING = PACE > 0 ? 90 : 0
// Паузы в сценарии подобраны под ≈2:40 при VIDEO_PACE=1 (замер на подменах).
const read = (page: Page, ms: number) => pause(page, ms * 1.55)
const ORIGIN = { Origin: process.env.E2E_BASE_URL ?? 'http://localhost:5177' }

test.use({ allowDestructive: LIVE })

/** Подмена принятия «акад»: approve → 200, дальше очередь, мета и поиск видят новую версию правил. */
async function mockAccept(page: Page) {
  let accepted: { id: string; version: number; at: string } | null = null
  const meta = (await (await page.request.get('/api/meta', { headers: ORIGIN })).json()) as { rules_version: number }
  const nextVersion = meta.rules_version + 1

  const patchProposal = (p: Record<string, unknown>) =>
    accepted && p.id === accepted.id
      ? { ...p, status: 'accepted', decided_at: accepted.at, config_version: accepted.version }
      : p

  await page.route(/\/api\/proposals\/[^/]+\/approve$/, async (route: Route) => {
    const id = new URL(route.request().url()).pathname.split('/')[3]
    const list = (await (await page.request.get('/api/proposals', { headers: ORIGIN })).json()) as {
      items: Record<string, unknown>[]
    }
    const p = list.items.find((x) => x.id === id)
    if (!p) return route.fulfill({ status: 404, json: { error: { message: 'нет предложения' } } })
    accepted = { id, version: nextVersion, at: new Date().toISOString() }
    return route.fulfill({ json: { proposal: patchProposal(p), config_version: nextVersion } })
  })

  await page.route(/\/api\/proposals(\?.*)?$/, async (route: Route) => {
    if (route.request().method() !== 'GET' || !accepted) return route.fallback()
    const resp = await route.fetch()
    const body = (await resp.json()) as { items: Record<string, unknown>[] }
    const url = new URL(route.request().url())
    const status = url.searchParams.get('status')
    let items = body.items.map(patchProposal)
    if (status) items = items.filter((p) => p.status === status)
    return route.fulfill({ response: resp, json: { ...body, items } })
  })

  await page.route(/\/api\/meta$/, async (route: Route) => {
    if (!accepted) return route.fallback()
    const resp = await route.fetch()
    const body = (await resp.json()) as Record<string, unknown>
    return route.fulfill({ response: resp, json: { ...body, rules_version: accepted.version } })
  })

  // Поиск «акад» в B4 после принятия: реальный ответ, AutoCAD поднят первым (как обещал прогон 5/6).
  await page.route(/\/api\/search$/, async (route: Route) => {
    const req = route.request()
    const body = req.postDataJSON() as { query?: string; config?: string }
    if (!accepted || body.query?.trim() !== 'акад' || body.config !== 'B4') return route.fallback()
    const resp = await route.fetch({ postData: JSON.stringify({ ...body, source: 'compare' }) })
    const data = (await resp.json()) as {
      items: { title: string; group: string }[]
      trace: Record<string, unknown>
    }
    const i = data.items.findIndex((x) => /^Autodesk AutoCAD,/.test(x.title))
    if (i > 0) {
      const [hit] = data.items.splice(i, 1)
      data.items.unshift({ ...hit, group: 'exact' })
    }
    data.trace = { ...data.trace, rules_version: accepted.version }
    return route.fulfill({ response: resp, json: data })
  })
}

test('Видео: поиск задачей → трасса → агент → очередь → «акад» → До / После', async ({
  page,
}, testInfo) => {
  testInfo.annotations.push({
    type: 'mode',
    description: LIVE ? 'живое принятие' : ACCEPT ? 'принятие на подменах' : 'без принятия',
  })
  if (ACCEPT && !LIVE) await mockAccept(page)
  const started = Date.now()

  // ── F3: задача своими словами → чипы, аналог ──
  await openSearch(page)
  await read(page, 2000)
  await search(page, 'аналог фотошопа на 20 пользователей', TYPING)
  const bar = understanding(page)
  await expect(bar.getByText('Аналог Adobe Photoshop')).toBeVisible()
  await expect(bar.getByText('20 пользователей')).toBeVisible()
  await expect(resultCards(page).first()).toContainText('CorelDRAW')
  await read(page, 6000)
  await bar.getByText('20 пользователей').hover()
  await read(page, 3000)

  // ── F6: трасса и разложение скора ──
  await page.getByRole('switch', { name: 'Трасса' }).click()
  const panel = tracePanel(page)
  await expect(panel.getByRole('heading', { name: 'Шаги' })).toBeVisible()
  await read(page, 7000)
  await resultCards(page).first().getByRole('button', { name: /^Почему здесь/ }).click()
  const breakdown = panel.getByRole('heading', { name: 'Разложение скора' })
  await breakdown.scrollIntoViewIfNeeded()
  await expect(panel.getByRole('img', { name: /Релевантность/ })).toBeVisible()
  await read(page, 7000)

  // ── F9: главный запуск агента из истории ──
  await page.getByRole('navigation', { name: 'Разделы' }).getByRole('link', { name: /^SearchOps/ }).click()
  await expect(page.getByRole('heading', { level: 1, name: 'SearchOps' })).toBeVisible()
  await read(page, 3000)
  await page.getByRole('region', { name: 'Запуски агента' }).locator(`a[href*="${MAIN_RUN_ID}"]`).click()
  const feed = page.getByRole('list', { name: 'Шаги агента' })
  await expect(feed).toBeVisible()
  await read(page, 5000)
  const akad = feed
    .getByRole('listitem')
    .filter({ has: page.getByRole('heading', { name: /^Кластер / }) })
    .filter({ hasText: 'алиас «акад» → «AutoCAD»' })
    .first()
  await akad.getByText('Гипотеза отклонена:').first().scrollIntoViewIfNeeded()
  await expect(akad.getByText('Гипотеза отклонена:').first()).toBeVisible()
  await read(page, 9000)
  await akad.getByText('Гипотеза 3 прошла прогон').scrollIntoViewIfNeeded()
  await read(page, 6000)
  const outcome = page.getByText(/Стоимость запуска:/).first()
  await outcome.scrollIntoViewIfNeeded()
  await expect(outcome).toBeVisible()
  await read(page, 6000)

  // ── F10: очередь, карточка «акад», решение человека ──
  await page.getByRole('link', { name: 'Открыть очередь' }).click()
  const card = page.getByRole('article', { name: 'Кластер «акад»' }).first()
  await expect(card).toBeVisible()
  await card.scrollIntoViewIfNeeded()
  await read(page, 4000)
  const run = card.getByRole('region', { name: 'Результат прогона' })
  await run.scrollIntoViewIfNeeded()
  await expect(run).toContainText('Условия допуска выполнены')
  await read(page, 8000)

  await card.getByRole('button', { name: 'Принять', exact: true }).click()
  const dialog = page.getByRole('dialog', { name: 'Принять правку?' })
  await expect(dialog).toContainText('Правило начнёт действовать сразу')
  await read(page, 4000)

  if (ACCEPT) {
    await dialog.getByRole('button', { name: 'Принять', exact: true }).click()
    await expect(page.getByText(/Правка принята · версия правил \d+/)).toBeVisible()
    await read(page, 3500)

    // ── «акад» в поиске теперь находит AutoCAD ──
    // После решения карточка уходит из фильтра «Ждёт решения»: проверяем через кнопку в уведомлении.
    const body = await searchResponse(page, () =>
      page.getByRole('button', { name: 'Проверить в поиске' }).click(),
    )
    expect(body.config).toBe('B4')
    await expect(resultCards(page).first()).toContainText('AutoCAD')
    await read(page, 7000)
  } else {
    await dialog.getByRole('button', { name: 'Отмена' }).click()
    await expect(dialog).toBeHidden()
    await read(page, 1500)
  }

  // ── F7: «До / После» ──
  await page.getByRole('navigation', { name: 'Разделы' }).getByRole('link', { name: 'До / После' }).click()
  const table = page.getByRole('region', { name: 'Таблица метрик' })
  await expect(table.getByRole('columnheader', { name: /^B4/ })).toBeVisible()
  await read(page, 9000)
  await table.getByRole('button', { name: /NDCG.*B4.*Как посчитано/ }).first().click()
  await expect(page.getByRole('dialog')).toBeVisible()
  await read(page, 7000)
  await page.keyboard.press('Escape')
  await read(page, 3000)

  const seconds = Math.round((Date.now() - started) / 1000)
  testInfo.annotations.push({ type: 'duration', description: `${seconds} с` })
  console.log(`[video] сценарий: ${seconds} с (${LIVE ? 'живой' : ACCEPT ? 'подмены' : 'без принятия'})`)
  if (PACE >= 1) expect(seconds, 'видео — до 3 минут').toBeLessThan(180)
})
