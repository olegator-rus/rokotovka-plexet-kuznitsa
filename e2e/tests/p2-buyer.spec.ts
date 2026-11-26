// Витрина контура коммерции («Правила блока», «События», /rules).
// Пока бэкенд-треки не отдают поля — на фикстурах контракта (p2-fixtures.ts); P2_LIVE=1 — на живом API.
import { mkdirSync } from 'node:fs'
import {
  expect,
  openSearch,
  resultCards,
  search,
  test,
  tracePanel,
  understanding,
} from './fixtures'
import { MEM_QUERY, P2_LIVE, WS_QUERY, mockP2, ofType } from './p2-fixtures'

const SCREENS = '../artifacts/screens/p2'
mkdirSync(SCREENS, { recursive: true })

const specButton = (page: import('@playwright/test').Page) =>
  page.getByRole('button', { name: /^Спецификация, позиций/ })
const specSheet = (page: import('@playwright/test').Page) =>
  page.getByRole('dialog', { name: 'Спецификация' })
const block = (page: import('@playwright/test').Page) =>
  page.getByRole('region', { name: /^Дополните решение к/ })

test.describe('Витрина контура коммерции', () => {
  test.beforeEach(async ({ page }) => {
    // Спецификация живёт в localStorage — каждый тест начинает с пустой.
    await page.addInitScript(() => {
      try {
        if (!sessionStorage.getItem('p2-e2e-init')) {
          localStorage.removeItem('plexet.spec')
          sessionStorage.setItem('p2-e2e-init', '1')
        }
      } catch {
        /* без хранилища */
      }
    })
  })

  test('F16: строка «Проверьте необходимость» — раскрытие, «уже есть», RDS, количество из запроса', async ({
    page,
  }) => {
    const cap = await mockP2(page)
    await openSearch(page)
    await search(page, WS_QUERY)

    const first = resultCards(page).first()
    const line = first.getByRole('button', {
      name: /Проверьте необходимость: .*CAL.* × 50/,
    })
    await expect(line).toBeVisible()
    await line.click()
    await expect(first.getByText('Нужны для доступа пользователей к серверу')).toBeVisible()
    await expect(first.getByRole('link', { name: /Основание/ })).toHaveAttribute('href', /^https:/)
    await expect(first.getByRole('spinbutton', { name: /Количество: .*CAL/ })).toHaveValue('50')

    // RDS добавляет вторую позицию.
    await expect(first.getByText('Windows Server 2025 RDS CAL', { exact: false })).toHaveCount(0)
    await first.getByRole('switch', { name: /RDS/ }).click()
    await expect(
      first.getByText('Windows Server 2025 RDS CAL, лицензия на пользователя', {
        exact: true,
      }),
    ).toBeVisible()

    // «Уже есть» сворачивает строку и пишет companion_dismiss.
    await first.getByRole('switch', { name: 'Уже есть подходящие лицензии' }).click()
    await expect(first.getByRole('button', { name: 'Не требуется, по вашим данным' })).toBeVisible()
    await expect.poll(() => ofType(cap, 'companion_dismiss').length).toBe(1)
    const dismiss = ofType(cap, 'companion_dismiss')[0]!
    expect(dismiss.product_id).toBe('ws-cal-2025-user')
    expect((dismiss.payload as Record<string, unknown>).condition).toBeTruthy()

    // Вернуть и добавить в спецификацию с количеством 50.
    await first.getByRole('button', { name: 'Не требуется, по вашим данным' }).click()
    await first.getByRole('switch', { name: 'Уже есть подходящие лицензии' }).click()
    await first.getByRole('button', { name: 'Добавить в спецификацию' }).first().click()
    await expect(specButton(page)).toHaveAccessibleName('Спецификация, позиций: 1')
    await expect.poll(() => ofType(cap, 'companion_add').length).toBe(1)
    const add = ofType(cap, 'companion_add')
    expect((add[0]!.payload as Record<string, unknown>).quantity).toBe(50)
  })

  test('F17: «Дополните решение» под топом, ничего не выбрано, спецификация и запрос КП', async ({
    page,
  }) => {
    const cap = await mockP2(page, { synthetic: ['orders', 'margin'] })
    await openSearch(page)
    await search(page, WS_QUERY)

    const b = block(page)
    await expect(b).toBeVisible()
    await expect(b.getByRole('heading', { level: 2 })).toHaveText(
      'Дополните решение к Microsoft Windows Server 2025 Standard, 16 ядер',
    )
    // 3 позиции (несовместимое отброшено), подписи классов, ничего не выбрано, старого блока нет.
    await expect(b.getByRole('group')).toHaveCount(3)
    await expect(b.getByText('Несовместимое')).toHaveCount(0)
    await expect(b.getByText('Дополняет решение — резервное копирование')).toBeVisible()
    await expect(b.getByText('Продление и поддержка').first()).toBeVisible()
    await expect(b.getByText('В спецификации')).toHaveCount(0)
    await expect(page.getByText('Старый блок С этим покупают')).toHaveCount(0)
    await expect(page.getByText(/Рекомендуем/)).toHaveCount(0)
    // «Часто берут вместе» на придуманных заказах — с плашкой.
    await expect(b.getByText('Часто берут вместе')).toBeVisible()
    await expect(b.getByText('Синтетические данные')).toBeVisible()

    // Блок стоит после третьего результата, до четвёртого.
    const order = await page.evaluate(() => {
      const blockEl = document.querySelector('#cmp-title')!
      const cards = [...document.querySelectorAll('article')]
      const pos = (el: Element) =>
        blockEl.compareDocumentPosition(el) & Node.DOCUMENT_POSITION_FOLLOWING ? 'after' : 'before'
      return cards.map(pos)
    })
    expect(order.slice(0, 3)).toEqual(['before', 'before', 'before'])
    expect(order[3]).toBe('after')

    // Показ блока — одно событие на поиск, переключение трассы его не повторяет.
    await expect
      .poll(
        () =>
          ofType(cap, 'companion_impression').filter(
            (e) => (e.payload as { place: string }).place === 'block',
          ).length,
      )
      .toBe(1)
    await page.getByRole('switch', { name: 'Трасса' }).click()
    await page.getByRole('switch', { name: 'Трасса' }).click()
    await page.waitForTimeout(300)
    expect(
      ofType(cap, 'companion_impression').filter(
        (e) => (e.payload as { place: string }).place === 'block',
      ),
    ).toHaveLength(1)

    // Основной товар + две позиции блока.
    await resultCards(page)
      .first()
      .getByRole('button', { name: /^В спецификацию:/ })
      .click()
    await b.getByRole('button', { name: /^В спецификацию: Veeam/ }).click()
    await b.getByRole('button', { name: /^В спецификацию: Внедрение/ }).click()
    await expect(specButton(page)).toHaveAccessibleName('Спецификация, позиций: 3')
    await expect.poll(() => ofType(cap, 'companion_add').length).toBe(2)

    await specButton(page).click()
    const sheet = specSheet(page)
    await expect(sheet).toBeVisible()
    const rows = sheet.getByRole('list', { name: 'Позиции спецификации' }).getByRole('listitem')
    await expect(rows).toHaveCount(3)
    await expect(rows.nth(0)).toContainText('из поиска')
    await expect(rows.nth(1)).toContainText('дополнение')
    // Серверная лицензия на ядра: «50 пользователей» не переносится в её количество.
    await expect(rows.nth(0).getByRole('spinbutton')).toHaveValue('1')
    await expect(sheet.getByRole('status')).toContainText(
      '3 позиции · у 1 цена по запросу · известная часть от',
    )
    await page.waitForTimeout(400) // панель доезжает
    await page.screenshot({ path: `${SCREENS}/buyer-spec-1440.png` })

    // Количество и удаление.
    await rows.nth(0).getByRole('spinbutton').fill('2')
    await rows.nth(0).getByRole('spinbutton').press('Tab')
    await expect(rows.nth(0)).toContainText('240 000')
    await sheet.getByRole('button', { name: /^Удалить: Veeam/ }).click()
    await expect(rows).toHaveCount(2)
    await expect.poll(() => ofType(cap, 'spec_remove').length).toBe(1)

    await sheet.getByRole('button', { name: 'Скопировать спецификацию' }).isVisible()
    await sheet.getByRole('button', { name: 'Запросить КП' }).click()
    await expect(sheet.getByText('Запрос КП отправлен')).toBeVisible()
    await expect(sheet.getByText('Q-2026-0042')).toBeVisible()
    expect(cap.quotes).toHaveLength(1)
    const q = cap.quotes[0] as {
      items: { product_id: string; quantity: number; origin: string }[]
    }
    expect(q.items.map((i) => [i.product_id, i.quantity, i.origin])).toEqual([
      ['ws-2025-std', 2, 'search'],
      ['impl-ws', 1, 'companion'],
    ])
    await expect.poll(() => ofType(cap, 'quote_request').length).toBe(1)
  })

  test('Спецификация: пусто — «Добавьте позиции из поиска»; ошибка КП — позиции сохранены, «Повторить»', async ({
    page,
  }) => {
    await mockP2(page, { quote: 'error' })
    await openSearch(page)
    await specButton(page).click()
    await expect(specSheet(page).getByText('Добавьте позиции из поиска')).toBeVisible()
    await specSheet(page).getByRole('button', { name: 'К поиску' }).click()

    await search(page, WS_QUERY)
    await block(page)
      .getByRole('button', { name: /^В спецификацию: Software Assurance/ })
      .click()
    await specButton(page).click()
    const sheet = specSheet(page)
    await sheet.getByRole('button', { name: 'Запросить КП' }).click()
    await expect(sheet.getByRole('alert')).toContainText('Запрос КП не отправлен')
    await expect(sheet.getByRole('alert')).toContainText('Позиции сохранены')
    await expect(sheet.getByRole('button', { name: 'Повторить запрос КП' })).toBeVisible()
    await expect(
      sheet.getByRole('list', { name: 'Позиции спецификации' }).getByRole('listitem'),
    ).toHaveCount(1)

    // Перезагрузка — позиции на месте (localStorage).
    await page.keyboard.press('Escape')
    await page.reload()
    await expect(specButton(page)).toHaveAccessibleName('Спецификация, позиций: 1')
  })

  test('F20–F21: чипы с единицами, значки совместимости, пометка условных, фасеты', async ({
    page,
  }) => {
    const cap = await mockP2(page)
    await openSearch(page)
    await search(page, MEM_QUERY)

    await expect(understanding(page).getByText('Память 32 ГБ')).toBeVisible()
    await expect(understanding(page).getByText('Совместимо с Dell PowerEdge R740')).toBeVisible()
    const cards = resultCards(page)
    await expect(cards.nth(0)).toContainText('Подтверждено для Dell PowerEdge R740')
    await expect(cards.nth(0)).toContainText('Парт-номер SNS-R740-32G')
    await expect(cards.nth(1)).toContainText('При условии: BIOS не ниже 2.12')
    await expect(cards.nth(2)).toContainText('Совместимость с Dell PowerEdge R740 не подтверждена')
    await expect(cards.nth(2)).toContainText('от 41 000')
    await expect(page.getByText('Для точной проверки укажите конфигурацию сервера')).toBeVisible()
    // Блока нет — компаньонов нет, старый «С этим покупают» пуст.
    await expect(block(page)).toHaveCount(0)

    const facets = page.getByRole('region', { name: 'Фильтры' })
    await expect(facets.getByRole('slider', { name: 'Объём: от' })).toBeVisible()
    await page.screenshot({ path: `${SCREENS}/buyer-hardware-1440.png` })

    await facets.getByRole('button', { name: /DDR4/ }).click()
    await expect(page).toHaveURL(/f=memory_type%3Aeq%3ADDR4|f=memory_type:eq:DDR4/)
    await expect
      .poll(() => cap.searches.at(-1)?.filters)
      .toEqual([{ key: 'memory_type', op: 'eq', value: 'DDR4' }])
    await expect.poll(() => ofType(cap, 'facet_apply').length).toBe(1)

    // Ползунок с клавиатуры: нижний бегунок вправо → gte.
    const lo = facets.getByRole('slider', { name: 'Объём: от' })
    await lo.focus()
    await lo.press('PageUp')
    await expect
      .poll(() => JSON.stringify(cap.searches.at(-1)?.filters))
      .toContain('"key":"ram_gb"')

    await facets.getByRole('button', { name: /Сбросить/ }).click()
    await expect(page).not.toHaveURL(/f=/)
  })

  test('F24: трасса — шаги «Оборудование» и «Компаньоны» с якорем и отклонёнными', async ({
    page,
  }) => {
    await mockP2(page)
    await openSearch(page)
    await search(page, WS_QUERY)
    await page.getByRole('switch', { name: 'Трасса' }).click()
    const panel = tracePanel(page)
    const step = panel.getByRole('button', { name: /Компаньоны/ })
    await expect(step).toContainText('якорь найден · 2 кандидата · отклонено 1')
    await step.click()
    await expect(
      panel.getByText('Якорь: Microsoft Windows Server 2025 Standard, 16 ядер'),
    ).toBeVisible()
    await expect(
      panel.getByRole('list', { name: 'Отклонённые кандидаты' }).getByRole('listitem'),
    ).toContainText('несовместимо по версии')
    await expect(panel.getByText(/Документация вендора/)).toBeVisible()

    await search(page, MEM_QUERY)
    const hw = panel.getByRole('button', { name: /Оборудование/ })
    await expect(hw).toContainText('1 единица · совместимость с dell-poweredge-r740')
    await hw.click()
    await expect(panel.getByText('ram_gb = 32 ГБ')).toBeVisible()
  })

  test('Флаг companions выключен — ни блока, ни строки; поиск работает', async ({ page }) => {
    const cap = await mockP2(page, {
      features: {
        hardware: true,
        companions: false,
        margin_policy: true,
        solutions: false,
      },
    })
    await openSearch(page)
    await search(page, WS_QUERY)
    await expect(resultCards(page).first()).toBeVisible()
    await expect(block(page)).toHaveCount(0)
    await expect(page.getByText(/Проверьте необходимость/)).toHaveCount(0)
    await page.waitForTimeout(300)
    expect(ofType(cap, 'companion_impression')).toHaveLength(0)
  })

  test('/rules: страница правил и строка в подвале', async ({ page }) => {
    await openSearch(page)
    const footer = page.getByRole('contentinfo')
    await expect(footer).toContainText('На сайте применяются рекомендательные технологии')
    await footer.getByRole('link', { name: 'Правила рекомендаций' }).click()
    await expect(page).toHaveURL(/\/rules$/)
    await expect(
      page.getByRole('heading', {
        name: 'Правила применения рекомендательных технологий',
      }),
    ).toBeVisible()
    for (const h of [
      'Какие данные используются',
      'Как формируется блок дополнений',
      'Как на это повлиять',
      'Обращения и сведения о владельце',
    ])
      await expect(page.getByRole('heading', { name: h })).toBeVisible()
    await page.screenshot({
      path: `${SCREENS}/buyer-rules-1440.png`,
      fullPage: true,
    })
  })

  test('360 px: выдача с блоком, строкой и спецификацией без горизонтальной прокрутки', async ({
    page,
  }) => {
    test.skip(P2_LIVE, 'скриншоты на фикстурах; живые — отдельным прогоном')
    await page.setViewportSize({ width: 360, height: 780 })
    await mockP2(page, { synthetic: ['orders'] })
    await openSearch(page)
    await search(page, WS_QUERY)
    const first = resultCards(page).first()
    await first.getByRole('button', { name: /Проверьте необходимость/ }).click()
    const overflow = () =>
      page.evaluate(
        () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
      )
    expect(await overflow()).toBeLessThanOrEqual(0)
    await page.screenshot({
      path: `${SCREENS}/buyer-search-360.png`,
      fullPage: true,
    })

    await block(page)
      .getByRole('button', { name: /^В спецификацию: Veeam/ })
      .click()
    await first.getByRole('button', { name: 'Добавить в спецификацию' }).first().click()
    await specButton(page).click()
    await expect(specSheet(page)).toBeVisible()
    expect(await overflow()).toBeLessThanOrEqual(0)
    await page.waitForTimeout(400)
    await page.screenshot({ path: `${SCREENS}/buyer-spec-360.png` })
    await page.keyboard.press('Escape')

    await search(page, MEM_QUERY)
    expect(await overflow()).toBeLessThanOrEqual(0)
    await page.screenshot({
      path: `${SCREENS}/buyer-hardware-360.png`,
      fullPage: true,
    })

    await page.goto('/rules')
    expect(await overflow()).toBeLessThanOrEqual(0)
    await page.screenshot({
      path: `${SCREENS}/buyer-rules-360.png`,
      fullPage: true,
    })
  })

  test('1440: выдача с условно обязательным и блоком — скриншот', async ({ page }) => {
    test.skip(P2_LIVE, 'скриншоты на фикстурах')
    await mockP2(page, { synthetic: ['orders'] })
    await openSearch(page)
    await search(page, WS_QUERY)
    await resultCards(page)
      .first()
      .getByRole('button', { name: /Проверьте необходимость/ })
      .click()
    await page.screenshot({
      path: `${SCREENS}/buyer-search-1440.png`,
      fullPage: true,
    })
  })
})
