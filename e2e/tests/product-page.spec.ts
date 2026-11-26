// Страница товара: гость на витрине. Клик по названию карточки
// пишет click с payload.action = product_page и открывает /product/:id; «← К результатам» возвращает к той же выдаче;
// действия на странице пишут события с номером поиска из адреса; 404, ошибка с повтором и ширина 360 px.
// События не уходят в аналитику: фикстура отвечает на POST /api/events 204, тело читается из запроса.
import type { Page, Request } from '@playwright/test'
import { GUEST, type SearchBody, expect, openSearch, results, search, test } from './fixtures'

interface EventBody {
  type: string
  request_id: string | null
  product_id: string | null
  position: number | null
  payload: Record<string, unknown>
}

/** Тела POST /api/events, отправленные страницей. */
function collectEvents(page: Page): EventBody[] {
  const events: EventBody[] = []
  page.on('request', (req: Request) => {
    if (req.method() === 'POST' && new URL(req.url()).pathname === '/api/events') {
      events.push(req.postDataJSON() as EventBody)
    }
  })
  return events
}

test.describe('страница товара — гость', () => {
  test.use({ storageState: GUEST })

  test('клик по названию → страница товара, одно событие click; назад к той же выдаче; действие с номером поиска', async ({
    page,
  }) => {
    const events = collectEvents(page)
    await openSearch(page)
    const body = (await search(page, 'касперский')) as SearchBody & { request_id: string }
    const first = body.items[0]
    expect(first, 'выдача по «касперский» пуста').toBeTruthy()

    const link = results(page).getByTestId('product-link').first()
    await expect(link).toHaveText(first.title)
    await link.click()

    await expect(page).toHaveURL(
      (u) =>
        u.pathname === `/product/${encodeURIComponent(first.product_id)}` &&
        u.searchParams.get('rid') === body.request_id &&
        u.searchParams.get('pos') === '1',
    )
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(first.title)
    await expect(page.getByTestId('product-demo-note')).toBeVisible()
    await expect(
      page.getByRole('heading', { name: 'Другие варианты этого продукта' }),
    ).toBeVisible()

    await expect.poll(() => events.filter((e) => e.type === 'click').length).toBe(1)
    const click = events.find((e) => e.type === 'click')!
    expect(click.request_id).toBe(body.request_id)
    expect(click.product_id).toBe(first.product_id)
    expect(click.position).toBe(1)
    expect(click.payload).toEqual({ action: 'product_page' })

    // Действие на странице — тот же request_id и позиция из адреса, тост демо-стенда.
    const buy = page.getByRole('button', { name: 'В корзину' })
    const ask = page.getByRole('button', { name: 'Запросить цену' })
    const action = (await buy.count()) ? buy : ask
    const type = (await buy.count()) ? 'add_to_cart' : 'price_request'
    await action.click()
    await expect.poll(() => events.filter((e) => e.type === type).length).toBe(1)
    const ev = events.find((e) => e.type === type)!
    expect(ev.request_id).toBe(body.request_id)
    expect(ev.position).toBe(1)
    expect(ev.product_id).toBe(first.product_id)
    await expect(
      page
        .getByText('Демо-стенд: действие записано событием для аналитики', { exact: false })
        .first(),
    ).toBeVisible()

    // «← К результатам» — та же выдача и запрос.
    await page.getByTestId('product-back').click()
    await expect(page).toHaveURL(
      (u) => u.pathname === '/search' && u.searchParams.get('q') === 'касперский',
    )
    await expect(results(page).getByTestId('product-link').first()).toHaveText(first.title)
  })

  test('прямой заход: «К результатам» — ссылка на /search; неизвестный товар — «Товар не найден в каталоге»', async ({
    page,
  }) => {
    await page.goto('/product/net-takogo')
    await expect(page.getByText('Товар не найден в каталоге')).toBeVisible()
    await expect(page.getByTestId('product-back')).toHaveAttribute('href', '/search')
    await page.getByRole('link', { name: 'К поиску', exact: true }).click()
    await expect(page).toHaveURL(/\/search$/)
  })

  test('ошибка сети — сообщение и «Повторить»; после снятия перехвата страница загружается', async ({
    page,
  }) => {
    // id позиции берём из выдачи, чтобы не зависеть от каталога.
    await openSearch(page)
    const body = await search(page, 'касперский')
    const id = body.items[0].product_id

    await page.route('**/api/products/**', (route) => route.abort('failed'))
    await page.goto(`/product/${encodeURIComponent(id)}`)
    const retry = page.getByRole('button', { name: 'Повторить' })
    await expect(retry).toBeVisible()
    await expect(page.getByText('Страница товара не загрузилась')).toBeVisible()

    await page.unroute('**/api/products/**')
    await retry.click()
    await expect(page.getByRole('heading', { level: 1 })).toHaveText(body.items[0].title)
  })

  test('360 px: нет горизонтальной прокрутки', async ({ page }) => {
    await page.setViewportSize({ width: 360, height: 780 })
    await openSearch(page)
    const body = await search(page, 'касперский')
    await page.goto(`/product/${encodeURIComponent(body.items[0].product_id)}`)
    await expect(page.getByRole('heading', { level: 1 })).toBeVisible()
    const overflow = await page.evaluate(
      () => document.documentElement.scrollWidth - document.documentElement.clientWidth,
    )
    expect(overflow).toBeLessThanOrEqual(0)
  })
})
