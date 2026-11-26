// QA-обход: B1, B2, M3–M6 на живом API.
// Только поиск: демо-состояние (правила, предложения, запуски агента) не меняется.
import { expect, openSearch, resultCards, search, test, understanding } from './fixtures'

test.describe('QA: поиск', () => {
  test('B1: витрина по умолчанию — действующие правила (live), трасса показывает их версию', async ({
    page,
  }) => {
    const meta = (await (await page.request.get('/api/meta')).json()) as {
      default_config: string
      rules_version: number
    }
    expect(meta.default_config).toBe('live')
    // Правка «зуп» — из действующей цепочки правил текущего каталога: после переимпорта каталога прежние версии
    // архивируются, и алиаса может не быть — тогда проверяется только, что витрина на действующей версии.
    const configs = (await (await page.request.get('/api/configs')).json()) as {
      items: { archived?: boolean; diff?: { add?: { kind?: string; alias?: string }[] } }[]
    }
    const zupAccepted = configs.items.some(
      (v) => !v.archived && (v.diff?.add ?? []).some((a) => a.kind === 'alias' && a.alias === 'зуп'),
    )
    await openSearch(page)
    const body = await search(page, 'зуп')
    expect(body.config).toBe('live')
    expect(body.trace.rules_version).toBe(meta.rules_version)
    // Принятая правка «зуп» работает на витрине без переключения конфигурации.
    if (zupAccepted) {
      expect(body.total).toBeGreaterThan(0)
      await expect(resultCards(page).first()).toContainText('Зарплата и управление персоналом')
    }
  })

  test('B2: «win» — товары и без «нет в каталоге»', async ({ page }) => {
    // Фид: «win» — короткое слово; в выдаче позиции с WIN в названии, а не честное «нет».
    await openSearch(page)
    const body = await search(page, 'win')
    expect(body.total).toBeGreaterThan(0)
    expect(body.empty ?? null).toBeNull()
    await expect(resultCards(page).first()).toContainText(/win/i)
    await expect(page.getByRole('heading', { name: /нет в каталоге/ })).toHaveCount(0)
  })

  test('M3: «касперский на 1000 пк бессрочно» — первой бизнес-линейка, условия в строке разбора', async ({
    page,
  }) => {
    await openSearch(page)
    const body = await search(page, 'касперский на 1000 пк бессрочно')
    for (const [field, value] of [
      ['vendor', 'kaspersky'],
      ['license_quantity', '1000'],
      ['license_type', 'perpetual'],
    ])
      expect(body.chips.some((c) => c.field === field && c.value === value), field).toBe(true)
    await expect(resultCards(page).first()).toContainText(/Endpoint Security/)
  })

  test('M4: «видеоконференции российские на 50 человек» — условия поняты, честное «нет» с подбором', async ({
    page,
  }) => {
    // На фиде нет ПО видеосвязи, проходящего все условия (отечественное и на 50 пользователей), — честное «нет»
    // с предложением поставки и блоком «Популярное в наличии», а не пустая страница.
    await openSearch(page)
    const body = (await search(page, 'видеоконференции российские на 50 человек')) as Awaited<
      ReturnType<typeof search>
    > & { empty?: { supply?: boolean } }
    expect(body.chips.some((c) => c.field === 'license_quantity' && c.value === '50')).toBe(true)
    expect(body.chips.some((c) => c.field === 'origin' && c.value === 'ru')).toBe(true)
    expect(body.total).toBe(0)
    expect(body.empty?.supply).toBe(true)
    await expect(page.locator('[data-match-group="fallback"]').getByRole('article').first()).toBeVisible()
  })

  test('M6: шум хвоста отсекается с причиной, разметка — честное «нет»', async ({ page }) => {
    await openSearch(page)
    const win = await search(page, 'винда 11 про')
    expect(win.items.some((i) => /Acrobat|Zoom/.test(i.title))).toBe(false)
    // Хвост чужих категорий и вендоров в выдаче не остаётся: всё найденное — категория первой позиции или её
    // вендор (карточки «Популярное в наличии» — не найденное, их нет при цели-категории); отсечённое — в трассе.
    const top = win.items[0]
    expect(top?.title).toMatch(/Windows 11/)
    const found = (win.items as { category?: string; vendor?: string; match_type?: string }[]).filter(
      (i) => i.match_type !== 'fallback',
    )
    expect(found.every((i) => i.category === top.category || i.vendor === top.vendor)).toBe(true)
    expect((win.trace.rejected ?? []).length).toBeGreaterThan(0)
    expect(win.trace.rejected?.every((r) => r.reason.length > 0)).toBe(true)

    // Разметка: найденного нет (total 0), ниже — только «Популярное в наличии» (fallback).
    const noise = await search(page, '<script>alert(1)</script>')
    expect(noise.total).toBe(0)
    await expect(page.locator('[data-match-group="exact"]')).toHaveCount(0)
    await expect(page.locator('[data-match-group]:not([data-match-group="fallback"])')).toHaveCount(0)
  })

  test('M5: «акад лт» — AutoCAD LT как найденный товар, без «нет в каталоге» и «Аналог»', async ({
    page,
  }) => {
    await openSearch(page)
    const body = await search(page, 'акад лт')
    expect(body.empty ?? null).toBeNull()
    const first = resultCards(page).first()
    await expect(first).toContainText('AutoCAD LT')
    await expect(first.getByText('Аналог', { exact: true })).toHaveCount(0)
  })
})
