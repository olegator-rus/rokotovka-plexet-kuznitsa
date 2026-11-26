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
    await openSearch(page)
    const body = await search(page, 'зуп')
    expect(body.config).toBe('live')
    expect(body.trace.rules_version).toBe(meta.rules_version)
    // Принятая правка «зуп» (версия ≥ 2) работает на витрине без переключения конфигурации.
    if (meta.rules_version >= 2) {
      expect(body.total).toBeGreaterThan(0)
      await expect(resultCards(page).first()).toContainText('Зарплата и управление персоналом')
    }
  })

  test('B2: «win» — Windows и подсказка, а не «нет в каталоге»', async ({ page }) => {
    await openSearch(page)
    const body = await search(page, 'win')
    expect(body.total).toBeGreaterThan(0)
    expect(body.empty ?? null).toBeNull()
    await expect(resultCards(page).first()).toContainText('Windows')
    await expect(understanding(page)).toContainText('Возможно, вы искали')
    await expect(page.getByRole('heading', { name: /нет в каталоге/ })).toHaveCount(0)
  })

  test('M3: «касперский на 1000 пк бессрочно» — первой бизнес-линейка', async ({ page }) => {
    await openSearch(page)
    await search(page, 'касперский на 1000 пк бессрочно')
    await expect(resultCards(page).first()).toContainText(/Endpoint Security|Small Office Security/)
  })

  test('M4: «видеоконференции российские на 50 человек» — категория и TrueConf', async ({ page }) => {
    await openSearch(page)
    const body = await search(page, 'видеоконференции российские на 50 человек')
    expect(body.chips.some((c) => c.field === 'category' && c.value === 'videoconf')).toBe(true)
    await expect(resultCards(page).first()).toContainText('TrueConf')
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

  test('M6: шум хвоста отсекается с причиной, разметка — честное «нет»', async ({ page }) => {
    await openSearch(page)
    const win = await search(page, 'винда 11 про')
    expect(win.items.some((i) => /Acrobat|Zoom/.test(i.title))).toBe(false)
    expect(win.trace.rejected?.some((r) => r.reason === 'tail_low_relevance')).toBe(true)

    const noise = await search(page, '<script>alert(1)</script>')
    expect(noise.total).toBe(0)
    await expect(resultCards(page)).toHaveCount(0)
  })
})
