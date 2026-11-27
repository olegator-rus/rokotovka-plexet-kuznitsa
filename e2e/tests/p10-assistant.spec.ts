// Помощник по выбору (контур помощника, SCENARIOS.md): панель открывается ТОЛЬКО по
// нажатию; справа 440 px рядом с рабочей выдачей; сообщение блоками — таблица, карточки с полной стоимостью, группы,
// подсказки, строка «Учитываю: …»; «В спецификацию» — системная строка и счётчик. По умолчанию /api/guide — заглушка
// ответами живого ядра (p10-assistant-data.ts), поиск — живой. E2E_GUIDE_LIVE=1 — помощник на живом API.
// E2E_GUIDE_SHOTS=<папка> — снимки сценариев 2 и 3 на 1280 и 360. Без звука.
import type { Locator, Page, Request } from "@playwright/test";
import { GUEST, expect, search, test } from "./fixtures";
import { REPLIES } from "./p10-assistant-data";

const LIVE = process.env.E2E_GUIDE_LIVE === "1";
const SHOTS = process.env.E2E_GUIDE_SHOTS ?? "";

const panel = (page: Page) => page.getByTestId("assistant-panel");
const entry = (page: Page) => page.getByTestId("assistant-entry");
const input = (page: Page) => page.getByTestId("assistant-input");
const lastMessage = (page: Page) =>
  page.getByTestId("assistant-message").last();
const plain = (s: string) => s.replace(/[  ]/g, " ");
const text = async (l: Locator) => plain(await l.innerText());

/** Заглушка /api/guide по тексту хода; журнал запросов — для проверки протокола. */
async function stubAssistant(page: Page): Promise<Record<string, unknown>[]> {
  const calls: Record<string, unknown>[] = [];
  if (LIVE) return calls;
  await page.route("**/api/guide/spec", (r) =>
    r.fulfill({
      status: 404,
      contentType: "application/json",
      body: JSON.stringify({ error: { code: "NOT_FOUND", message: "нет" } }),
    }),
  );
  await page.route("**/api/guide", (r) => {
    const body = (r.request() as Request).postDataJSON() as Record<string, unknown>;
    calls.push(body);
    const t = `${body.query ?? ""} ${body.message ?? ""}`.toLowerCase();
    const reply = t.includes("будет 50")
      ? REPLIES.s2b
      : t.includes("расширенный")
        ? REPLIES.s2a
        : t.includes("криптопро")
          ? REPLIES.s3
          : t.includes("оплат")
            ? REPLIES.pay
            : REPLIES.s1;
    return r.fulfill({
      status: 200,
      contentType: "application/json",
      body: JSON.stringify(reply),
    });
  });
  return calls;
}

async function say(page: Page, msg: string) {
  const resp = page.waitForResponse(
    (r) => r.url().endsWith("/api/guide") && r.request().method() === "POST",
    { timeout: 30_000 },
  );
  await input(page).fill(msg);
  await input(page).press("Enter");
  expect((await resp).status(), "POST /api/guide").toBe(200);
  await expect(page.getByTestId("assistant-wait")).toHaveCount(0);
}

async function noHorizontalScroll(page: Page) {
  const over = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(over, "горизонтальной прокрутки нет").toBeLessThanOrEqual(0);
}

test.describe("П10: помощник по выбору", () => {
  test.use({ storageState: GUEST });

  test("панель не открывается сама: ни запрос-вопрос, ни загрузка, ни «ничего не найдено», ни перезагрузка", async ({
    page,
  }) => {
    let guideCalls = 0;
    page.on("request", (r) => {
      if (new URL(r.url()).pathname.startsWith("/api/guide") && r.method() === "POST") guideCalls++;
    });
    await page.goto("/");
    await expect(panel(page)).toHaveCount(0);
    for (const q of [
      "чем расширенный касперский отличается от стандартного",
      "самый дешёвый криптопро csp для сервера",
      "подбери антивирь",
      "зщщщфывапролдж",
    ]) {
      await search(page, q);
      await page.waitForTimeout(800);
      await expect(panel(page), `«${q}»`).toHaveCount(0);
    }
    await page.goto("/search?q=антивирус");
    await page.waitForTimeout(800);
    await expect(panel(page)).toHaveCount(0);
    expect(guideCalls, "без нажатия к помощнику не ходим").toBe(0);
  });

  test("сценарий 2: кнопка → сравнение на 30 → 50 и шифрование → в спецификацию", async ({
    page,
  }) => {
    const calls = await stubAssistant(page);
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.goto("/");
    await search(page, "антивирус");
    await expect(panel(page)).toHaveCount(0);
    await entry(page).click();
    const p = panel(page);
    await expect(p).toBeVisible();
    // Справа, 400–480 px; выдача слева видна.
    const pb = (await p.boundingBox())!;
    expect(pb.width).toBeGreaterThanOrEqual(399);
    expect(pb.width).toBeLessThanOrEqual(481);
    const rb = (await page
      .locator(".search-page__main")
      .first()
      .boundingBox())!;
    expect(rb.x + rb.width).toBeLessThanOrEqual(pb.x + 1);
    // Короткий запрос — приветствие и подсказки по теме, ничего не отправлено.
    await expect(p.getByTestId("assistant-welcome")).toContainText(
      "Подберём подходящую лицензию",
    );
    await expect(p.getByTestId("assistant-starters").getByRole("button")).toHaveCount(4);
    expect(calls).toHaveLength(0);

    await say(
      page,
      "чем расширенный касперский отличается от стандартного, нам на 30 компьютеров",
    );
    if (!LIVE) expect(calls.at(-1)).toEqual({
      session_id: expect.any(String),
      query: "чем расширенный касперский отличается от стандартного, нам на 30 компьютеров",
    });
    const m1 = page.getByTestId("assistant-message").nth(0);
    await expect(m1.getByTestId("assistant-user")).toHaveCount(0);
    const rows = m1.getByTestId("assistant-table-row");
    const n = await rows.count();
    expect(n).toBeGreaterThanOrEqual(3);
    expect(n).toBeLessThanOrEqual(5);
    expect(await text(rows.last())).toMatch(/Итого\s+84 402 ₽\s+122 445 ₽/);
    expect(await text(rows.nth(n - 2))).toMatch(/За место\s+2 813,40 ₽\s+4 081,50 ₽/);
    const cards = m1.getByTestId("assistant-card");
    expect(await cards.count()).toBeGreaterThanOrEqual(4);
    await expect(m1.locator('[data-offer="4985"] [data-testid=assistant-price]')).toHaveText(/84\s402\s₽/);
    await expect(m1.locator('[data-offer="4985"] [data-testid=assistant-calc]')).toHaveText(
      /30 × 2\s813,40 ₽/,
    );
    await expect(page.getByTestId("assistant-conditions")).toContainText(
      "Учитываю: 30 ПК · 1 год · новая",
    );
    await expect(m1.getByTestId("assistant-suggestions").getByRole("button")).toHaveCount(3);

    await say(page, "компьютеров будет 50, и шифрование нужно");
    if (!LIVE) expect(calls.at(-1)).toMatchObject({
      query: "чем расширенный касперский отличается от стандартного, нам на 30 компьютеров",
      message: "компьютеров будет 50, и шифрование нужно",
      state: { selection_id: expect.any(String) },
    });
    const m2 = lastMessage(page);
    await expect(m2).toContainText("На 50 компьютеров действует другая ценовая ступень.");
    const adv = m2.locator('[data-offer="4857"]');
    await expect(adv.getByTestId("assistant-badge")).toHaveText("Есть нужное шифрование");
    await expect(adv.getByTestId("assistant-price")).toHaveText(/197\s010\s₽/);
    await expect(adv.getByTestId("assistant-calc")).toHaveText(/50 × 3\s940,20 ₽/);
    await expect(adv).toContainText("Цена за место ниже: 3 940,20 ₽ вместо 4 081,50 ₽");
    const std = m2.locator('[data-offer="4986"]');
    await expect(std.getByTestId("assistant-reason")).toHaveText(
      "Шифрования данных в этой редакции нет",
    );
    await expect(std.getByTestId("assistant-action")).toHaveCount(0);
    await expect(std.getByTestId("assistant-store")).toHaveCount(1);
    await expect(page.getByTestId("assistant-conditions")).toContainText(
      "50 ПК · 1 год · новая · шифрование",
    );
    // Подсказки — только под последним ответом.
    await expect(m1.getByTestId("assistant-suggestions")).toHaveCount(0);

    const before = Number(await page.getByTestId("assistant-spec-count").innerText());
    await adv.getByTestId("assistant-action").click();
    await expect(page.getByTestId("assistant-system").last()).toContainText(
      "Добавлено в спецификацию",
    );
    await expect(adv.getByTestId("assistant-in-spec")).toHaveText(/В спецификации/);
    await expect(page.getByTestId("assistant-spec-count")).toHaveText(String(before + 1));
    if (SHOTS) await page.screenshot({ path: `${SHOTS}/s2-1280.png` });

    // Закрытие сохраняет историю; перезагрузка панель не открывает, кнопка возвращает разговор.
    await page.getByTestId("assistant-close").click();
    await expect(panel(page)).toHaveCount(0);
    await page.reload();
    await page.waitForTimeout(500);
    await expect(panel(page)).toHaveCount(0);
    await entry(page).click();
    await expect(page.getByTestId("assistant-message")).toHaveCount(2);
  });

  test("сценарий 3: «Сбросить» и самая дешёвая КриптоПро для сервера — неподходящее с причиной", async ({
    page,
  }) => {
    const calls = await stubAssistant(page);
    await page.setViewportSize({ width: 1280, height: 720 });
    await page.goto("/");
    await entry(page).click();
    await say(page, "нужен антивирус в офис на 20 компьютеров");
    await expect(page.getByTestId("assistant-conditions")).toBeVisible();
    await page.getByTestId("assistant-reset").click();
    await expect(page.getByTestId("assistant-conditions")).toHaveCount(0);
    await say(page, "самый дешёвый криптопро csp для сервера");
    if (!LIVE) expect(calls.at(-1)).toEqual({
      session_id: expect.any(String),
      query: "самый дешёвый криптопро csp для сервера",
    });
    const m = lastMessage(page);
    await expect(m).toContainText("Серверная лицензия стоит 70 000 ₽.");
    const server = m.locator('[data-offer="197958"]');
    await expect(server.getByTestId("assistant-badge")).toHaveText("Для серверной ОС");
    await expect(server.getByTestId("assistant-price")).toHaveText(/70\s000\s₽/);
    const rej = m.locator('[data-group="rejected"]');
    await expect(rej).toContainText("Не подходят для серверной ОС");
    const cheap = rej.locator('[data-offer="411620"]');
    await expect(cheap.getByTestId("assistant-badge")).toHaveText("Не подходит для сервера");
    await expect(cheap.getByTestId("assistant-price")).toHaveText(/1\s850\s₽/);
    await expect(cheap.getByTestId("assistant-reason")).toHaveText("Лицензия для рабочей станции");
    await expect(cheap.getByTestId("assistant-action")).toHaveCount(0);
    expect(await m.getByTestId("assistant-card").count()).toBeGreaterThanOrEqual(4);
    await expect(m.getByTestId("assistant-sources")).toContainText("Условия лицензирования КриптоПро");
    if (SHOTS) await page.screenshot({ path: `${SHOTS}/s3-1280.png` });
  });

  test("не его тема: короткая переадресация без карточек", async ({ page }) => {
    await stubAssistant(page);
    await page.goto("/");
    await entry(page).click();
    await say(page, "как оплатить по счёту?");
    const m = lastMessage(page);
    await expect(m).toHaveAttribute("data-status", "out_of_scope");
    await expect(m).toContainText("Помощник подбирает товары и лицензии.");
    await expect(m.getByTestId("assistant-card")).toHaveCount(0);
  });

  test("360 px: на весь экран, таблица без горизонтальной прокрутки, «назад» к выдаче", async ({
    page,
  }) => {
    await stubAssistant(page);
    await page.setViewportSize({ width: 360, height: 740 });
    await page.goto("/");
    await search(page, "антивирус");
    await entry(page).click();
    const p = panel(page);
    const box = (await p.boundingBox())!;
    expect(box.width).toBe(360);
    await say(
      page,
      "чем расширенный касперский отличается от стандартного, нам на 30 компьютеров",
    );
    await expect(lastMessage(page).getByTestId("assistant-table")).toBeVisible();
    await noHorizontalScroll(page);
    const feedOver = await page
      .getByTestId("assistant-feed")
      .evaluate((el) => el.scrollWidth - el.clientWidth);
    expect(feedOver, "лента без горизонтальной прокрутки").toBeLessThanOrEqual(0);
    if (SHOTS) await page.screenshot({ path: `${SHOTS}/s2-360.png` });
    await page.getByTestId("assistant-back").click();
    await expect(panel(page)).toHaveCount(0);
    await entry(page).click();
    await expect(page.getByTestId("assistant-message")).toHaveCount(1);
  });
});
