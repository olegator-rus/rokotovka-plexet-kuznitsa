// Флоу жюри F6–F7 и бэк-офиса F9–F10. Агент не запускается, решения по предложениям не принимаются:
// страж в fixtures.ts обрывает такие запросы и валит тест.
import {
  MAIN_RUN_ID,
  expect,
  openSearch,
  resultCards,
  search,
  searchResponse,
  test,
  tracePanel,
} from './fixtures'

test.describe('Жюри', () => {
  test('F6: трасса — сводка, шаги, разложение скора по клику «почему здесь»', async ({ page }) => {
    await openSearch(page)
    await search(page, 'аналог фотошопа на 20 пользователей')
    await expect(tracePanel(page)).toHaveCount(0)

    await page.getByRole('switch', { name: 'Трасса' }).click()
    const panel = tracePanel(page)
    await expect(panel).toBeVisible()
    await expect(page).toHaveURL(/trace=1/)
    for (const term of ['Маршрут', 'Время', 'Стоимость', 'Разбор']) {
      await expect(panel.getByRole('term').filter({ hasText: term })).toBeVisible()
    }
    await expect(panel.getByRole('heading', { name: 'Шаги' })).toBeVisible()
    await expect(panel.getByRole('heading', { name: 'Разложение скора' })).toBeVisible()
    const bar = panel.getByRole('img', { name: /Релевантность .*Популярность .*Маржа .*Контекст/ })
    await expect(bar).toBeVisible()

    // «Почему здесь» на другой карточке переключает разложение на неё.
    const cards = resultCards(page)
    if ((await cards.count()) > 1) {
      const second = cards.nth(1)
      const title = (await second.getAttribute('aria-label')) ?? ''
      await second.getByRole('button', { name: /^Почему здесь/ }).click()
      await expect(second.getByRole('button', { name: /^Почему здесь/ })).toHaveAttribute(
        'aria-pressed',
        'true',
      )
      await expect(panel).toContainText(title)
    } else {
      await cards.first().getByRole('button', { name: /^Почему здесь/ }).click()
      await expect(panel).toContainText((await cards.first().getAttribute('aria-label')) ?? '')
    }

    await panel.getByRole('button', { name: 'Закрыть трассу' }).click()
    await expect(tracePanel(page)).toHaveCount(0)
  })

  test('F7: «До / После» — таблица метрик B0–B4 и строка воспроизводимости', async ({ page }) => {
    await page.goto('/proofs')
    await expect(page.getByRole('heading', { level: 1, name: 'Доказательства' })).toBeVisible()
    await expect(page.getByRole('tab', { name: 'До / После' })).toHaveAttribute('aria-selected', 'true')
    await expect(page.getByRole('tab', { name: 'Метрики' })).toHaveAttribute('aria-selected', 'true')

    const table = page.getByRole('region', { name: 'Таблица метрик' })
    await expect(table).toBeVisible()
    for (const cfg of ['B0', 'B1', 'B2', 'B3', 'B4']) {
      await expect(table.getByRole('columnheader', { name: new RegExp(`^${cfg}`) })).toBeVisible()
    }
    await expect(table.getByRole('rowheader', { name: /NDCG/ }).first()).toBeVisible()
    await expect(page.getByText(/заморожен — поиск B0–B3 на нём не настраивался/)).toBeVisible()
    await expect(page.getByText(/Воспроизводимость: хэш набора/)).toBeVisible()

    // Ячейка открывает «Как посчитано» — число не голое.
    await table.getByRole('button', { name: /NDCG.*B4.*Как посчитано/ }).first().click()
    const dialog = page.getByRole('dialog')
    await expect(dialog).toBeVisible()
    await page.keyboard.press('Escape')
    await expect(dialog).toBeHidden()
  })
})

test.describe('SearchOps', () => {
  test('F9: главный запуск открывается из истории — гипотезы зачёркнуты с причиной', async ({
    page,
  }) => {
    await page.goto('/searchops?tab=runs')
    await expect(page.getByRole('heading', { level: 1, name: 'SearchOps' })).toBeVisible()
    // Пока идёт чужой запуск, кнопка называется «Открыть идущий запуск».
    await expect(
      page.getByRole('button', { name: /^(Запустить разбор|Открыть идущий запуск)$/ }),
    ).toBeVisible()

    const runs = page.getByRole('region', { name: 'Запуски агента' })
    await expect(runs).toBeVisible()
    const link = runs.locator(`a[href*="${MAIN_RUN_ID}"]`)
    await expect(link).toHaveCount(1)
    await link.click()

    await expect(page).toHaveURL(new RegExp(`/searchops/runs/${MAIN_RUN_ID}`))
    await expect(
      page.getByRole('heading', { level: 1, name: 'Разбор неудачных запросов' }),
    ).toBeVisible()
    await expect(page.getByText(MAIN_RUN_ID)).toBeVisible()
    await expect(page.getByRole('button', { name: 'Остановить' })).toHaveCount(0)

    const feed = page.getByRole('list', { name: 'Шаги агента' })
    await expect(feed).toBeVisible()
    // Сегмент кластера ищем по содержимому: заголовок сегмента может показывать id, а не подпись.
    const akad = feed
      .getByRole('listitem')
      .filter({ has: page.getByRole('heading', { name: /^Кластер / }) })
      .filter({ hasText: 'алиас «акад» → «AutoCAD»' })
      .first()
    await expect(akad).toBeVisible()
    await expect(akad).toContainText('Гипотеза 3 прошла прогон')
    // Отклонённые гипотезы остаются в ленте зачёркнутыми, с причиной (по логу — 9 из 11).
    const reasons = feed.getByText('Гипотеза отклонена:')
    await expect(reasons.first()).toBeVisible()
    expect(await reasons.count()).toBeGreaterThanOrEqual(5)
    await expect(akad.getByText('Гипотеза отклонена:').first()).toBeVisible()
    expect(await feed.getByRole('deletion').count()).toBe(0) // зачёркивание — <s>, не <del>
    expect(await akad.locator('s').count()).toBeGreaterThanOrEqual(2)

    await expect(page.getByText(/Стоимость запуска:/).first()).toContainText('30,00')
  })

  test('F10: карточка «Правка поиска» — причина и владелец крупно, запросы, разница правил, прогон; диалог без утверждения', async ({
    page,
  }) => {
    // Предложение берётся из очереди, а не по имени: после переимпорта каталога предложения прежнего каталога
    // становятся stale (новый каталог начинает цепочку правил заново), и «акад» контура поиска в очереди уже нет.
    const res = await page.request.get('/api/proposals?status=pending')
    expect(res.ok()).toBeTruthy()
    const body = (await res.json()) as {
      items: { id: string; action: string; run_id?: string; cluster?: { label?: string; queries?: { text: string }[] } }[]
    }
    const p = body.items.find((x) => x.action === 'rules_patch' && x.cluster?.label)
    test.skip(!p, 'В очереди нет ждущих предложений по правилам поиска (после переимпорта каталога они stale)')
    if (!p) return

    await page.goto('/searchops')
    await page.getByRole('tab', { name: /Очередь/ }).click()
    await expect(page).toHaveURL(/tab=queue/)

    const card = page.locator(`#proposal-${p.id}`)
    await expect(card).toBeVisible()
    await expect(card.getByText('Ждёт решения', { exact: false })).toBeVisible()
    await expect(card.getByTestId('kind')).toHaveText('Правка поиска')
    await expect(card.getByTestId('cause')).not.toBeEmpty()
    await expect(card.getByTestId('owner')).toContainText('Владелец:')
    await expect(card).toContainText(`кластер «${p.cluster!.label}»`)
    const firstQuery = p.cluster!.queries?.[0]?.text
    if (firstQuery) {
      await expect(card.getByRole('region', { name: 'Запросы кластера' })).toContainText(firstQuery)
    }
    await expect(card.getByRole('region', { name: 'Что предлагается' })).toBeVisible()
    await expect(card.getByRole('region', { name: 'Результат прогона' })).toBeVisible()

    // Диалог «Утвердить» открывается и закрывается «Отменой» — без запроса к API.
    await card.getByRole('button', { name: 'Утвердить', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Утвердить правку поиска?' })
    await expect(dialog).toContainText('Правило начнёт действовать сразу')
    await expect(dialog).toContainText(/версия \d+|новая версия правил/)
    await dialog.getByRole('button', { name: 'Отмена' }).click()
    await expect(dialog).toBeHidden()
    await expect(card.getByRole('button', { name: 'Утвердить', exact: true })).toBeEnabled()

    // «Показать ход расследования» ведёт на запуск, который нашёл правку.
    if (p.run_id) {
      await card.getByRole('link', { name: /Показать ход расследования/ }).click()
      await expect(page).toHaveURL(new RegExp(`/searchops/runs/${p.run_id}\\?cluster=`))
      await expect(page.getByRole('list', { name: 'Шаги агента' })).toBeVisible()
    }
  })
})

// Единственный тест, которому разрешено менять демо: запускается только при E2E_DESTRUCTIVE=1.
test.describe('SearchOps — принятие', () => {
  test.use({ allowDestructive: process.env.E2E_DESTRUCTIVE === '1' })

  test('F10-accept: принятие «акад» → версия правил → поиск находит AutoCAD @destructive', async ({
    page,
    allowDestructive,
  }) => {
    test.skip(!allowDestructive, 'меняет демо-состояние: E2E_DESTRUCTIVE=1 и allowDestructive')
    await page.goto('/searchops?tab=queue')
    const card = page.locator('article').filter({ hasText: 'кластер «акад»' }).first()
    await card.getByRole('button', { name: 'Утвердить', exact: true }).click()
    const dialog = page.getByRole('dialog', { name: 'Утвердить правку поиска?' })
    await dialog.getByRole('button', { name: 'Утвердить', exact: true }).click()
    await expect(page.getByText(/Правка утверждена · версия правил \d+/)).toBeVisible()

    // После решения карточка уходит из фильтра «Ждёт решения»: проверяем через кнопку в уведомлении.
    const body = await searchResponse(page, () =>
      page.getByRole('button', { name: 'Проверить в поиске' }).click(),
    )
    expect(body.config).toBe('B4')
    await expect(resultCards(page).first()).toContainText('AutoCAD')
  })
})
