// Контур подбора режим подбора в поиске (DIALOGS.md D1 и D2).
// По умолчанию /api/guide — заглушка по контракту (p8-guide-data.ts), поиск — живой. E2E_GUIDE_LIVE=1 — подбор
// на живом API ядра. E2E_GUIDE_SHOTS=<папка> — снимки на 360, 768 и 1440. Демо-состояние не меняется: события и
// заявки КП перехватываются общей обвязкой.
import type { Locator, Page, Response } from "@playwright/test";
import { GUEST, expect, search, test } from "./fixtures";
import { GUIDE_LIVE, stubGuide, type GuideBody } from "./p8-guide-data";

const SHOTS = process.env.E2E_GUIDE_SHOTS ?? "";
const DATE = /\b\d{1,2}\.\d{1,2}\.\d{2,4}\b|\b20\d{2}-\d{2}-\d{2}\b/;

const panel = (page: Page) => page.getByTestId("guide-panel");
const resultsArea = (page: Page) =>
  page.locator(".search-page__results").getByRole("article");
const isGuide = (r: Response) =>
  r.url().endsWith("/api/guide") && r.request().method() === "POST";

/** Ход подбора: действие и ответ /api/guide. */
async function turn(page: Page, action: () => Promise<unknown>) {
  const [resp] = await Promise.all([
    page.waitForResponse(isGuide, { timeout: 30_000 }),
    action(),
  ]);
  expect(resp.status(), "POST /api/guide").toBe(200);
  return resp;
}

async function say(page: Page, text: string) {
  const box = panel(page).getByTestId("guide-text");
  await box.fill(text);
  return turn(page, () => box.press("Enter"));
}

const cond = (p: Locator, key: string) => p.locator(`[data-cond="${key}"]`);

test.describe("режим подбора в поиске", () => {
  test.use({ storageState: GUEST });

  test.describe("на заглушках", () => {
    test.skip(GUIDE_LIVE, "E2E_GUIDE_LIVE=1 — заглушки выключены");

    test("D1: от запроса до спецификации", async ({ page }) => {
      const calls = await stubGuide(page, { delayMs: 300 });
      await page.goto("/");
      await search(page, "защита для компьютеров в офисе");

      // Ноль вопросов до первых карточек: выдача на экране, подбор не вызывался, над выдачей — предложение.
      await expect(resultsArea(page).first()).toBeVisible();
      await expect(page.getByTestId("guide-hint")).toContainText(
        "Могу помочь выбрать редакцию и посчитать лицензии",
      );
      expect(calls).toHaveLength(0);

      await turn(page, () => page.getByTestId("guide-start").click());
      const p = panel(page);
      await expect(p.getByTestId("guide-question")).toContainText(
        "Сколько компьютеров",
      );
      await expect(page.getByTestId("guide-hint")).toHaveCount(0);
      expect(calls[0]).toMatchObject({
        query: "защита для компьютеров в офисе",
      });
      expect(calls[0]!.session_id).toBeTruthy();

      // Чип «30»: условия по умолчанию видны и помечены.
      await turn(page, () =>
        p.getByRole("button", { name: "30", exact: true }).click(),
      );
      expect(calls[1]).toMatchObject({
        chip: { key: "count:workstation", value: "30" },
      });
      await expect(cond(p, "count:workstation")).toContainText("30 ПК");
      await expect(cond(p, "operation")).toContainText("по умолчанию");
      await expect(cond(p, "term_months")).toContainText("1 год");
      await expect(cond(p, "term_months")).toHaveAttribute(
        "data-origin",
        "default",
      );
      await expect(p.getByTestId("guide-offer")).toHaveCount(2);
      // Выдача под блоком подбора остаётся.
      await expect(resultsArea(page).first()).toBeVisible();

      // Сравнение: таблица, четыре вида ячейки, источник с фрагментом и без даты.
      await say(page, "чем расширенный касперский отличается от стандартного");
      expect(calls[2]).toMatchObject({
        message: "чем расширенный касперский отличается от стандартного",
        state: { turn: 2 },
      });
      const table = p.getByTestId("guide-table");
      await expect(table).toBeVisible();
      await expect(
        table.getByRole("columnheader", { name: "Расширенный" }),
      ).toBeVisible();
      // По умолчанию — только различия: одинаковые строки (консоль, сертификаты) скрыты, цена — одной строкой «Итого».
      const features = table.locator('tr[data-kind="feature"]');
      await expect(features).toHaveCount(2);
      await expect(table.locator('tr[data-row="console"]')).toHaveCount(0);
      const enc = table.locator('tr[data-row="encryption"]');
      await expect(enc.locator("td").nth(0)).toHaveText(
        "нет по данным вендора",
      );
      await expect(enc.locator("td").nth(1)).toHaveText("есть");
      const total = table.locator('tr[data-row="total"]');
      await expect(total.getByTestId("guide-cell-total").nth(1)).toHaveText(
        "122 445 ₽",
      );
      await expect(total.locator("td").nth(1)).toContainText("4 081,50 ₽");
      await expect(
        table.locator('tr[data-row="price_per_seat_year"]'),
      ).toHaveCount(0);
      await enc.getByTestId("guide-source").click();
      const pop = page.getByTestId("guide-source-pop");
      await expect(pop).toContainText("Шифрование файлов и дисков");
      await expect(
        pop.getByRole("link", { name: /Открыть страницу/ }),
      ).toHaveAttribute("href", /kaspersky\.ru/);
      expect(await pop.innerText()).not.toMatch(DATE);
      await page.keyboard.press("Escape");

      // Все строки: одинаковые возвращаются, «нет сведений» подписано честно.
      await table.getByRole("switch").click();
      await expect(features).toHaveCount(4);
      await expect(table.locator('tr[data-row="certs"] td').first()).toHaveText(
        "нет сведений",
      );

      // 30 → 50 и шифрование: Расширенный, ступень 50–99, итог; «При изменении условий».
      await say(page, "на самом деле компьютеров будет 50, и шифрование нужно");
      expect(calls[3]!.state).toMatchObject({ turn: 3 });
      await expect(cond(p, "count:workstation")).toContainText("50 ПК");
      await expect(cond(p, "required_features")).toContainText(
        "нужно шифрование",
      );
      const offer = p.getByTestId("guide-offer");
      await expect(offer).toHaveCount(1);
      await expect(offer.getByTestId("guide-offer-sku")).toHaveText(
        "KL4867RAQFS",
      );
      await expect(offer).toContainText("50–99");
      await expect(offer.getByTestId("guide-offer-unit")).toContainText(
        "3 940,20 ₽",
      );
      await expect(offer.getByTestId("guide-offer-total")).toContainText(
        "197 010 ₽",
      );
      await expect(p.getByTestId("guide-changes")).toContainText(
        "Стандартный — 2 715,30 ₽",
      );
      await expect(p).not.toContainText(/\b0 ₽/);

      // «Как подобрано»: свёрнуто, раскрывается.
      const trace = p.getByTestId("guide-trace");
      await expect(trace.locator("li").first()).toBeHidden();
      await trace.locator("summary").click();
      await expect(trace.locator("li")).toHaveCount(3);
      await expect(trace).toContainText("Подобрана ступень 50–99");

      // В спецификацию: одна строка, 50 шт., сумма; повторный клик строку не дублирует.
      await p.getByTestId("guide-spec").click();
      await expect(p.getByTestId("guide-spec")).toHaveText("В спецификации");
      await p.getByTestId("guide-spec").click();
      const dialog = page.getByRole("dialog", { name: "Спецификация" });
      await expect(dialog).toBeVisible();
      await expect(dialog).toContainText("KL4867RAQFS");
      await expect(dialog).toContainText("197 010 ₽");
      await expect(
        dialog.getByRole("button", { name: "Запросить КП" }),
      ).toBeVisible();
      await page.keyboard.press("Escape");
      await expect(
        page.getByRole("button", { name: /^Спецификация, позиций: 1/ }),
      ).toBeVisible();

      // Свернуть и закрыть: выдача и спецификация на месте.
      await p.getByTestId("guide-collapse").click();
      await expect(p.getByTestId("guide-text")).toBeHidden();
      await expect(p).toContainText("50 ПК");
      await p.getByTestId("guide-close").click();
      await expect(panel(page)).toHaveCount(0);
      await expect(resultsArea(page).first()).toBeVisible();
      await expect(
        page.getByRole("button", { name: /^Спецификация, позиций: 1/ }),
      ).toBeVisible();
      await expect(page.getByTestId("guide-hint")).toBeVisible();
    });

    test("D2: серверная лицензия КриптоПро первой, рабочее место — «не подходит»", async ({
      page,
    }) => {
      const calls = await stubGuide(page);
      await page.goto("/");
      // Вопрос в строке поиска — сразу подбор; выдача тоже на экране.
      await Promise.all([
        page.waitForResponse(isGuide),
        search(page, "самый дешёвый криптопро csp для сервера"),
      ]);
      const p = panel(page);
      await expect(p.getByTestId("guide-lead")).toHaveText(
        "Лицензия на рабочее место в серверной ОС не действует — нужна серверная.",
      );
      const first = p.getByTestId("guide-offer").first();
      await expect(first).toContainText("серверная лицензия");
      await expect(first.getByTestId("guide-offer-unit")).toContainText(
        "70 000 ₽",
      );
      // «Не подходит»: лицензии на рабочее место — отдельным блоком с причиной, без кнопок, ниже серверной.
      const rejected = p.getByTestId("guide-rejected");
      await expect(rejected).toContainText("Не подходит");
      await expect(rejected).toContainText("1 850 ₽ за рабочее место");
      await expect(rejected).toContainText("для сервера не подходит");
      await expect(rejected.getByRole("button")).toHaveCount(0);
      await expect(p.getByTestId("guide-offer")).toHaveCount(1);
      await expect(p.getByTestId("guide-trace")).toContainText(
        "не подходят по праву использования: 2",
      );
      await expect(p.getByTestId("guide-quote")).toBeVisible();
      await expect(resultsArea(page).first()).toBeVisible();
      expect(calls[0]).toMatchObject({
        query: "самый дешёвый криптопро csp для сервера",
      });

      await say(page, "а для tls-сервера?");
      await expect(p.getByTestId("guide-lead")).toHaveText(
        "Для TLS-сервера у КриптоПро отдельная лицензия.",
      );
      await expect(p.getByTestId("guide-offer").first()).toContainText("TLS");

      // «Запросить КП»: позиция с условиями — в спецификацию, панель КП открыта.
      await p.getByTestId("guide-quote").click();
      const dialog = page.getByRole("dialog", { name: "Спецификация" });
      await expect(dialog).toContainText("КриптоПро CSP TLS");
      await expect(
        dialog.getByRole("button", { name: "Запросить КП" }),
      ).toBeVisible();
    });

    test("ошибка хода — повтор; правка и снятие условия", async ({ page }) => {
      const calls = await stubGuide(page, { fail: 1 });
      await page.goto("/");
      await search(page, "защита для компьютеров в офисе");
      await page.getByTestId("guide-start").click();
      const p = panel(page);
      await expect(p.getByTestId("guide-error")).toContainText(
        "Подобрать не получилось",
      );
      await expect(resultsArea(page).first()).toBeVisible();
      await turn(page, () =>
        p.getByRole("button", { name: "Повторить" }).click(),
      );
      await expect(p.getByTestId("guide-question")).toBeVisible();
      expect(calls).toHaveLength(2);

      await turn(page, () =>
        p.getByRole("button", { name: "30", exact: true }).click(),
      );
      // Снять «1 год»: уходит condition с value null и прежнее состояние.
      await turn(page, () =>
        cond(p, "term_months")
          .getByRole("button", { name: /Убрать условие/ })
          .click(),
      );
      expect(calls.at(-1)).toMatchObject({
        condition: { key: "term_months", value: null },
        message: "без условия «1 год»",
        state: { turn: 2 },
      });
      // Изменить «30 ПК» → 40.
      await cond(p, "count:workstation")
        .getByRole("button", { name: /Изменить условие/ })
        .click();
      const input = cond(p, "count:workstation").getByRole("textbox");
      await input.fill("40");
      await turn(page, () => input.press("Enter"));
      expect((calls.at(-1) as GuideBody).condition).toEqual({
        key: "count:workstation",
        value: "40",
      });
    });

    test("вход со страницы семейства: «Помочь выбрать лицензию» и чип-вопрос", async ({
      page,
    }) => {
      const calls = await stubGuide(page);
      await page.goto("/product/197958");
      const box = page.getByTestId("product-guide");
      await expect(box).toBeVisible();
      await turn(page, () =>
        box.getByRole("button", { name: "Чем отличаются редакции?" }).click(),
      );
      await expect(page).toHaveURL(/\/search\?/);
      await expect(panel(page)).toBeVisible();
      expect(calls[0]!.message).toBe("Чем отличаются редакции?");
      expect(calls[0]!.query.length).toBeGreaterThan(0);
      // Признак входа снимается с адреса: перезагрузка не открывает подбор второй раз.
      await expect
        .poll(() => new URL(page.url()).searchParams.has("guide"))
        .toBe(false);
      await expect(resultsArea(page).first()).toBeVisible();
    });

    test("режим строки от ядра: /api/guide/route главнее правила фронта", async ({
      page,
    }) => {
      await stubGuide(page, {
        route: { mode: "search_offer", offer: "Помогу подобрать лицензию." },
      });
      await page.goto("/");
      await search(page, "криптопро csp 5.0");
      await expect(page.getByTestId("guide-hint")).toContainText(
        "Помогу подобрать лицензию.",
      );
    });

    test("точный запрос — обычный поиск без предложения", async ({ page }) => {
      const calls = await stubGuide(page);
      await page.goto("/");
      await search(page, "криптопро csp 5.0");
      await expect(resultsArea(page).first()).toBeVisible();
      await expect(page.getByTestId("guide-hint")).toHaveCount(0);
      await expect(panel(page)).toHaveCount(0);
      expect(calls).toHaveLength(0);
    });
  });

  test.describe("снимки", () => {
    test.skip(
      !SHOTS || GUIDE_LIVE,
      "E2E_GUIDE_SHOTS=<папка> — снимки на заглушках",
    );

    for (const width of [360, 768, 1440]) {
      test(`D1 на ${width}`, async ({ page }) => {
        await page.setViewportSize({
          width,
          height: width === 360 ? 780 : 900,
        });
        await stubGuide(page);
        await page.goto("/");
        await search(page, "защита для компьютеров в офисе");
        await expect(page.getByTestId("guide-hint")).toBeVisible();
        await page.screenshot({ path: `${SHOTS}/${width}-1-hint.png` });
        await turn(page, () => page.getByTestId("guide-start").click());
        await turn(page, () =>
          panel(page).getByRole("button", { name: "30", exact: true }).click(),
        );
        await page.screenshot({ path: `${SHOTS}/${width}-2-conditions.png` });
        await say(
          page,
          "чем расширенный касперский отличается от стандартного",
        );
        await panel(page)
          .locator('tr[data-row="encryption"]')
          .getByTestId("guide-source")
          .click();
        await expect(page.getByTestId("guide-source-pop")).toBeVisible();
        await panel(page).screenshot({ path: `${SHOTS}/${width}-3-table.png` });
        await page.keyboard.press("Escape");
        await say(
          page,
          "на самом деле компьютеров будет 50, и шифрование нужно",
        );
        await panel(page).getByTestId("guide-trace").locator("summary").click();
        await panel(page).screenshot({
          path: `${SHOTS}/${width}-4-selection.png`,
        });
        await page.screenshot({ path: `${SHOTS}/${width}-5-page.png` });
        // Страница не прокручивается вбок.
        const overflow = await page.evaluate(
          () =>
            document.documentElement.scrollWidth -
            document.documentElement.clientWidth,
        );
        expect(overflow).toBeLessThanOrEqual(0);
      });
    }
  });

  test.describe("живой API", () => {
    test.skip(!GUIDE_LIVE, "E2E_GUIDE_LIVE=1 — на ответах ядра");
    // Мягкие проверки ждут каждая до своего таймаута: тесту нужен запас, чтобы дойти до спецификации.
    test.describe.configure({ timeout: 180_000 });
    // Проверки по DIALOGS.md. Что зависит от данных и правил ядра — expect.soft: прогон доходит до спецификации
    // и показывает все расхождения сразу, а не первое. Условия не ослаблены — мягкая проверка тоже валит тест.

    test("D1 на живом API: от запроса до спецификации", async ({ page }) => {
      await page.goto("/");
      await search(page, "защита для компьютеров в офисе");
      await expect(resultsArea(page).first()).toBeVisible();
      const top4 = (await resultsArea(page).allInnerTexts())
        .slice(0, 4)
        .join("\n");
      expect
        .soft(top4, "в первых карточках выдачи — защита рабочих мест Kaspersky")
        .toMatch(/Kaspersky/i);
      await expect(page.getByTestId("guide-hint")).toBeVisible();

      await turn(page, () => page.getByTestId("guide-start").click());
      const p = panel(page);
      await expect(p.getByTestId("guide-question")).toContainText(
        "Сколько компьютеров",
      );
      await turn(page, () =>
        p
          .getByTestId("guide-question")
          .getByRole("button", { name: "30", exact: true })
          .click(),
      );
      await expect(p.locator('[data-cond^="count"]')).toContainText("30 ПК");
      await expect(cond(p, "operation")).toContainText("по умолчанию");
      await expect(cond(p, "term_months")).toContainText("по умолчанию");
      const titles30 = await p
        .getByTestId("guide-offer")
        .locator("h4")
        .allInnerTexts();
      expect(titles30.length).toBeGreaterThan(0);
      for (const t of titles30)
        expect
          .soft(t, "на 30 ПК — только защита рабочих мест")
          .not.toMatch(/для мобильных устройств|интернет-шлюз|Office 365/i);

      await say(page, "чем расширенный касперский отличается от стандартного");
      const table = p.getByTestId("guide-table");
      await expect(table).toBeVisible();
      await expect
        .soft(table.getByRole("columnheader", { name: "Стандартный" }))
        .toBeVisible();
      await expect
        .soft(table.getByRole("columnheader", { name: "Расширенный" }))
        .toBeVisible();
      const enc = table.locator('tr[data-row="encryption"]');
      await expect
        .soft(enc, "строка «Шифрование» из базы знаний")
        .toHaveCount(1);
      if (await enc.count()) {
        await expect
          .soft(enc.locator("td").nth(0))
          .toHaveText("нет по данным вендора");
        await expect.soft(enc.locator("td").nth(1)).toHaveText("есть");
        await expect.soft(enc.getByTestId("guide-source")).toBeVisible();
      }
      await expect(table).not.toContainText(/\b0 ₽/);

      await say(page, "на самом деле компьютеров будет 50, и шифрование нужно");
      await expect(p.locator('[data-cond^="count"]')).toContainText("50 ПК");
      const offers = p.getByTestId("guide-offer");
      await expect(offers.first()).toBeVisible();
      // Подходит один Расширенный; Стандартный — не среди подходящих (альтернатива с причиной или «При изменении»).
      await expect
        .soft(
          p.locator('[data-testid="guide-offer"][data-role="match"]'),
          "подходит один Расширенный",
        )
        .toHaveCount(1);
      await expect
        .soft(p.locator('[data-testid="guide-offer"][data-role="match"]'))
        .not.toContainText("Стандартный");
      const first = offers.first();
      await expect
        .soft(first.getByTestId("guide-offer-sku"))
        .toHaveText("KL4867RAQFS");
      await expect
        .soft(first.getByTestId("guide-offer-unit"))
        .toContainText("3 940,20 ₽");
      await expect
        .soft(first.getByTestId("guide-offer-total"))
        .toContainText("197 010 ₽");
      await expect
        .soft(p.getByTestId("guide-changes"))
        .toContainText(/Стандартн/);
      await expect.soft(p.getByTestId("guide-preliminary")).toHaveCount(0);
      await expect(p).not.toContainText(/\b0 ₽/);
      const sku = (
        await first.getByTestId("guide-offer-sku").innerText()
      ).trim();

      await p.getByTestId("guide-trace").locator("summary").click();
      await expect(
        p.getByTestId("guide-trace").locator("li").first(),
      ).toBeVisible();

      await p.getByTestId("guide-spec").click();
      await expect(
        page.getByRole("button", { name: /^Спецификация, позиций: 1/ }),
      ).toBeVisible();
      await page
        .getByRole("button", { name: /^Спецификация, позиций: 1/ })
        .click();
      const dialog = page.getByRole("dialog", { name: "Спецификация" });
      await expect(dialog).toContainText(sku);
      await expect(
        dialog.getByRole("button", { name: "Запросить КП" }),
      ).toBeVisible();
    });

    test("D2 на живом API: серверная первой, рабочее место — «Не подходит»", async ({
      page,
    }) => {
      await page.goto("/");
      await Promise.all([
        page.waitForResponse(isGuide),
        search(page, "самый дешёвый криптопро csp для сервера"),
      ]);
      const p = panel(page);
      const first = p.getByTestId("guide-offer").first();
      await expect(first).toBeVisible();
      await expect
        .soft(first, "первой — серверная лицензия")
        .toContainText(/серверн/i);
      await expect
        .soft(first.getByTestId("guide-offer-unit"))
        .toContainText("70 000 ₽");
      const rejected = p.getByTestId("guide-rejected");
      await expect(rejected).toBeVisible();
      await expect(rejected).toContainText(/рабоч\S* мест/i);
      await expect(rejected).toContainText(/сервер/i);
      await expect.soft(rejected).toContainText("1 850 ₽");

      await say(page, "а для tls-сервера?");
      await expect
        .soft(
          p.getByTestId("guide-offer").first(),
          "семейство КриптоПро CSP TLS",
        )
        .toContainText("TLS");
    });
  });

  test.describe("живой API: практичные сценарии", () => {
    test.skip(!GUIDE_LIVE, "E2E_GUIDE_LIVE=1 — на ответах ядра");
    test.describe.configure({ timeout: 120_000 });

    test("«подбери антивирус для офиса на 20 компьютеров»: рекомендация крупно, понятая задача чипом", async ({
      page,
    }) => {
      await page.goto("/");
      await Promise.all([
        page.waitForResponse(isGuide),
        search(page, "подбери антивирус для офиса на 20 компьютеров"),
      ]);
      const p = panel(page);
      await expect.soft(cond(p, "direction")).toContainText("Понял:");
      await expect(p.locator('[data-cond^="count"]')).toContainText("20 ПК");
      const first = p.getByTestId("guide-offer").first();
      await expect(first).toBeVisible();
      await expect
        .soft(p.getByTestId("guide-pick-status"))
        .toHaveText("Рекомендуем");
      await expect.soft(p.getByTestId("guide-reason")).toContainText("₽");
      await expect(first.getByTestId("guide-offer-total")).toContainText("₽");
      await expect(p.getByTestId("guide-spec")).toHaveCount(1);
      await expect(p).not.toContainText(/\b0 ₽/);
      // Сравнение вариантов — по кнопке, на всю ширину блока.
      const toggle = p.getByTestId("guide-compare-toggle");
      if (await toggle.count()) {
        await toggle.click();
        await expect(p.getByTestId("guide-table")).toBeVisible();
      }
    });

    test("«подбери антивирь»: вопрос о числе мест, задача и тип покупателя видны", async ({
      page,
    }) => {
      await page.goto("/");
      await Promise.all([
        page.waitForResponse(isGuide),
        search(page, "подбери антивирь"),
      ]);
      const p = panel(page);
      await expect(p.getByTestId("guide-question")).toContainText("Сколько");
      await expect.soft(cond(p, "direction")).toContainText("Понял:");
      await expect.soft(cond(p, "org_type")).toBeVisible();
    });
  });

  test.describe("снимки на живом API", () => {
    test.skip(
      !GUIDE_LIVE || !SHOTS,
      "E2E_GUIDE_LIVE=1 и E2E_GUIDE_SHOTS=<папка>",
    );

    for (const width of [360, 1440]) {
      test(`D1 и D2 вживую на ${width}`, async ({ page }) => {
        await page.setViewportSize({
          width,
          height: width === 360 ? 780 : 900,
        });
        await page.goto("/");
        await search(page, "защита для компьютеров в офисе");
        await page.screenshot({ path: `${SHOTS}/live-${width}-1-hint.png` });
        await turn(page, () => page.getByTestId("guide-start").click());
        await turn(page, () =>
          panel(page).getByRole("button", { name: "30", exact: true }).click(),
        );
        await panel(page).screenshot({
          path: `${SHOTS}/live-${width}-2-thirty.png`,
        });
        await say(
          page,
          "чем расширенный касперский отличается от стандартного",
        );
        await panel(page).screenshot({
          path: `${SHOTS}/live-${width}-3-compare.png`,
        });
        await say(
          page,
          "на самом деле компьютеров будет 50, и шифрование нужно",
        );
        await panel(page).getByTestId("guide-trace").locator("summary").click();
        await panel(page).screenshot({
          path: `${SHOTS}/live-${width}-4-fifty.png`,
        });
        await Promise.all([
          page.waitForResponse(isGuide),
          search(page, "самый дешёвый криптопро csp для сервера"),
        ]);
        await expect(
          panel(page).getByTestId("guide-offer").first(),
        ).toBeVisible();
        await panel(page).screenshot({
          path: `${SHOTS}/live-${width}-5-d2.png`,
        });
        const overflow = await page.evaluate(
          () =>
            document.documentElement.scrollWidth -
            document.documentElement.clientWidth,
        );
        expect(overflow).toBeLessThanOrEqual(0);
      });
    }
  });
});
