// Покупательские флоу F1–F5 и экономика F8. Опора — роли и подписи, не классы.
import {
  expect,
  openSearch,
  resultCards,
  search,
  searchBox,
  searchResponse,
  test,
  tracePanel,
  understanding,
} from './fixtures'

test.describe('Поиск', () => {
  test('F1: подсказки после двух символов, выбор подсказки ведёт к точному товару', async ({
    page,
  }) => {
    await openSearch(page)
    await searchBox(page).pressSequentially('win', { delay: 60 })

    const list = page.getByRole('listbox')
    await expect(list).toBeVisible()
    // Сервер контура коммерции — группа «Запросы», панель контура выдачи (§11.2) — «Подсказки».
    await expect(list.getByText(/^(Запросы|Подсказки)$/).first()).toBeVisible()
    const options = list.getByRole('option')
    await expect(options.first()).toBeVisible()
    // Панель контура выдачи (§11.2): ≤5 подсказок, ≤2 категории, ≤2 производителя, ≤6 семейств товаров;
    // сервер контура коммерции — не больше 8 строк в группах.
    const sugg = list.getByRole('group', { name: 'Подсказки' }).getByRole('option')
    if (await sugg.count()) {
      expect(await sugg.count()).toBeLessThanOrEqual(5)
      expect(await options.count()).toBeLessThanOrEqual(5 + 2 + 2 + 6)
    } else {
      expect(await options.count()).toBeLessThanOrEqual(8)
    }

    const body = await searchResponse(page, () =>
      list.getByRole('option', { name: 'windows 11 pro', exact: true }).click(),
    )
    expect(body.total).toBeGreaterThan(0)
    // Контур коммерции — «Найдено N», страница результатов контура выдачи — «По запросу … — N продуктов · M вариантов».
    await expect(
      page
        .getByRole('status')
        .filter({ hasText: /Найдено|\d+ продукт/ })
        .first(),
    ).toBeVisible()
    await expect(resultCards(page).first()).toContainText('Windows 11 Pro')
  })

  test('F2: опечатка исправляется с плашкой, «Искать как введено» ищет исходный текст', async ({
    page,
  }) => {
    await openSearch(page)
    const fixed = await search(page, 'касперкий')
    expect(fixed.correction?.applied).toBe(true)

    const bar = understanding(page)
    await expect(bar).toContainText('Показаны результаты по запросу')
    await expect(bar.getByText('касперский', { exact: true })).toBeVisible()
    await expect(resultCards(page).first()).toContainText('Kaspersky')

    const asTyped = await searchResponse(page, () =>
      bar.getByRole('button', { name: 'касперкий', exact: true }).click(),
    )
    expect(asTyped.query).toBe('касперкий')
    await expect(page).toHaveURL(/as_typed=1/)
    await expect(bar).toContainText('как введено, без исправления')
  })

  test('F2: раскладка и сленг — «rfcgthcrbq» → касперский, «винда 11 про» без плашки', async ({
    page,
  }) => {
    await openSearch(page)
    const layout = await search(page, 'rfcgthcrbq')
    expect(layout.total).toBeGreaterThan(0)
    await expect(understanding(page).getByText('касперский', { exact: true })).toBeVisible()
    await expect(resultCards(page).first()).toContainText('Kaspersky')

    const slang = await search(page, 'винда 11 про')
    expect(slang.correction?.applied ?? false).toBe(false)
    await expect(page.getByText('Показаны результаты по запросу')).toHaveCount(0)
    await expect(resultCards(page).first()).toContainText('Windows 11 Pro')
  })

  test('F3: «аналог фотошопа на 20 пользователей» → чипы, аналог, снятие и возврат чипа', async ({
    page,
  }) => {
    await openSearch(page)
    const body = await search(page, 'аналог фотошопа на 20 пользователей')
    expect(body.trace.route).toBe('D')

    const bar = understanding(page)
    await expect(bar).toContainText('Мы поняли запрос так')
    await expect(bar.getByText('Аналог Adobe Photoshop')).toBeVisible()
    await expect(bar.getByText('20 пользователей')).toBeVisible()

    const first = resultCards(page).first()
    await expect(first).toContainText('CorelDRAW')
    await expect(first.getByText('Аналог', { exact: true })).toBeVisible()

    await searchResponse(page, () =>
      bar.getByRole('button', { name: 'Снять условие «20 пользователей»' }).click(),
    )
    await expect(page).toHaveURL(/rm=license_quantity/)
    await expect(bar).toContainText('Снято условий: 1')
    await expect(resultCards(page).first()).toBeVisible()

    await searchResponse(page, () => bar.getByRole('button', { name: 'вернуть' }).click())
    await expect(bar).not.toContainText('Снято условий')
    await expect(bar.getByText('20 пользователей')).toBeVisible()
  })

  // Маршрут D: снятые чипы применяются к итогу разбора LLM, в ключ кэша не входят — пересчёт из кэша за 0 ₽.
  test('F3: снятый чип исчезает из строки понимания, пересчёт без вызова LLM', async ({ page }) => {
    await openSearch(page)
    await search(page, 'аналог фотошопа на 20 пользователей')
    const bar = understanding(page)
    const after = await searchResponse(page, () =>
      bar.getByRole('button', { name: 'Снять условие «20 пользователей»' }).click(),
    )
    expect(after.chips.some((c) => c.field === 'license_quantity')).toBe(false)
    expect(after.trace.parse_source, 'снятие чипа не вызывает LLM').not.toBe('llm')
    await expect(bar.getByRole('button', { name: 'Снять условие «20 пользователей»' })).toHaveCount(
      0,
    )
  })

  test('F4: «notion» → честное «нет в каталоге» и «Спросить менеджера»', async ({ page }) => {
    await openSearch(page)
    const body = await search(page, 'notion')
    expect(body.total).toBe(0)

    await expect(
      page.getByRole('heading', { name: 'Товара «notion» нет в каталоге' }),
    ).toBeVisible()
    await page.getByRole('button', { name: 'Спросить менеджера' }).click()
    const dialog = page.getByRole('dialog', { name: 'Спросить менеджера' })
    await expect(dialog).toContainText('Запрос передан')
    await dialog.getByRole('button', { name: 'Понятно' }).click()
    await expect(dialog).toBeHidden()
  })

  test('F5: «продлить антивирус» — у клиента А его продукт первым, у клиента Б общая выдача', async ({
    page,
  }) => {
    await openSearch(page)
    const pick = async (name: RegExp) => {
      await page.getByRole('combobox', { name: 'Клиент' }).click()
      await page.getByRole('option', { name }).click()
    }

    await pick(/^Клиент А/)
    const a = await search(page, 'продлить антивирус')
    expect(a.items[0]?.purchased_before).toBe(true)
    const firstA = resultCards(page).first()
    await expect(firstA).toContainText('Kaspersky Small Office Security')
    await expect(firstA.getByText('Вы покупали этот продукт')).toBeVisible()

    const b = await searchResponse(page, () => pick(/^Клиент Б/))
    expect(b.items.some((i) => i.purchased_before)).toBe(false)
    await expect(page.getByText('Вы покупали этот продукт')).toHaveCount(0)
    expect(b.items[0]?.title).not.toBe(a.items[0]?.title)
  })

  test('F8: повтор сложного запроса — разбор из кэша, 0 ₽', async ({ page }) => {
    const q = 'аналог фотошопа на 20 пользователей'
    await openSearch(page, '?trace=1')
    await search(page, q)
    // Повтор тем же адресом: страница заново шлёт тот же запрос.
    const again = await searchResponse(page, () => page.reload())
    expect(again.trace.parse_source).toBe('cache')
    expect(again.trace.cost_rub).toBe(0)

    const panel = tracePanel(page)
    await expect(panel.getByRole('definition').filter({ hasText: 'cache' })).toBeVisible()
    await expect(panel.getByRole('definition').filter({ hasText: /^0\s₽$/ })).toBeVisible()
  })
})
