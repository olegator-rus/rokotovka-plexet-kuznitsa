// Режим подбора в поиске: контур подбора (поведение) и контур экрана (рабочая область). По умолчанию /api/guide и /api/guide/route — заглушки по контракту экрана (p8-guide-data.ts), поиск —
// живой. E2E_GUIDE_LIVE=1 — подбор на живом API ядра. E2E_GUIDE_SHOTS=<папка> — снимки на 1440, 1280 и 360.
// Демо-состояние не меняется: события и заявки КП перехватываются общей обвязкой.
import type { Locator, Page, Response } from "@playwright/test";
import { GUEST, expect, search, searchBox, test } from "./fixtures";
import { GUIDE_LIVE, stubGuide, type GuideBody } from "./p8-guide-data";

const SHOTS = process.env.E2E_GUIDE_SHOTS ?? "";
const DATE = /\b\d{1,2}\.\d{1,2}\.\d{2,4}\b|\b20\d{2}-\d{2}-\d{2}\b/;

const panel = (page: Page) => page.getByTestId("guide-panel");
const resultsArea = (page: Page) =>
  page.locator(".search-page__results").getByRole("article");
const isGuide = (r: Response) =>
  r.url().endsWith("/api/guide") && r.request().method() === "POST";
/** Пробелы в числах — неразрывные: сравниваем по обычным. */
const plain = (s: string) => s.replace(/[  ]/g, " ");
const text = async (l: Locator) => plain(await l.innerText());

/** Ход подбора: действие и ответ /api/guide. */
async function turn(page: Page, action: () => Promise<unknown>) {
  const [resp] = await Promise.all([
    page.waitForResponse(isGuide, { timeout: 30_000 }),
    action(),
  ]);
  expect(resp.status(), "POST /api/guide").toBe(200);
  return resp;
}

/** Поиск, после которого подбор раскрывается сам (явная просьба): ждём и выдачу, и первый ход. */
async function searchGuide(page: Page, q: string) {
  await turn(page, () => search(page, q));
}

async function noHorizontalScroll(page: Page) {
  const over = await page.evaluate(
    () => document.documentElement.scrollWidth - window.innerWidth,
  );
  expect(over, "горизонтальной прокрутки нет").toBeLessThanOrEqual(0);
}

test.describe("рабочая область подбора", () => {
  test.use({ storageState: GUEST });

  test.describe("на заглушках", () => {
    test.skip(GUIDE_LIVE, "E2E_GUIDE_LIVE=1 — заглушки выключены");

    test("сценарий 1: «подбери антивирь» — справа от выдачи, вопрос о количестве, варианты, функция", async ({
      page,
    }) => {
      const calls = await stubGuide(page);
      await page.goto("/");
      await searchGuide(page, "подбери антивирь");
      const p = panel(page);

      // Справа от выдачи, 480–560 px; выдача слева видна и кликабельна.
      await expect(p).toBeVisible();
      const pb = (await p.boundingBox())!;
      const rb = (await resultsArea(page).first().boundingBox())!;
      expect(pb.x).toBeGreaterThan(rb.x + rb.width);
      expect(pb.width).toBeGreaterThanOrEqual(479);
      expect(pb.width).toBeLessThanOrEqual(561);
      await expect(page.getByTestId("guide-hint")).toHaveCount(0);

      // Задача понята, без «что вы имели в виду»; вопрос о количестве с быстрыми значениями.
      await expect(p.getByTestId("guide-task")).toHaveText(
        "Антивирус для рабочих мест",
      );
      await expect(p.getByTestId("guide-question")).toContainText(
        "Сколько компьютеров нужно защитить?",
      );
      await expect(p.getByTestId("guide-quick")).toHaveText([
        "10",
        "20",
        "30",
        "50",
        "100",
      ]);
      await expect(p.locator('[data-cond="operation"]')).toContainText(
        "по умолчанию",
      );
      await expect(p.getByTestId("guide-spec")).toHaveCount(0);

      await turn(page, () =>
        p.getByTestId("guide-quick").filter({ hasText: /^30$/ }).click(),
      );
      expect(calls.at(-1)).toMatchObject({
        chip: { key: "count:workstation", value: "30" },
        state: { selection_id: "e2e-guide" },
      });
      // Поле количества, несколько вендоров: полная стоимость задачи, без победителя и без главной кнопки.
      await expect(p.getByTestId("guide-count")).toHaveValue("30");
      const cost = p.getByTestId("guide-task-cost");
      await expect(cost.getByTestId("guide-cost-row")).toHaveCount(3);
      await expect(cost).toContainText(
        "Состав функций и условия лицензирования различаются",
      );
      await expect(p.getByTestId("guide-choose")).toHaveText(
        "Выберите вариант, чтобы добавить в спецификацию",
      );
      await expect(p.getByTestId("guide-spec")).toHaveCount(0);
      // Выбор строки — появляется итог этой позиции и одна кнопка.
      await cost.getByTestId("guide-cost-row").nth(1).click();
      expect(await text(p.getByTestId("guide-sum"))).toBe("84 402 ₽");
      await expect(p.getByTestId("guide-spec")).toHaveCount(1);

      await turn(page, () =>
        p
          .getByTestId("guide-option")
          .filter({ hasText: "Нужно шифрование данных" })
          .click(),
      );
      const res = p.getByTestId("guide-result");
      await expect(res.getByTestId("guide-pick-status")).toHaveText(
        "Подходит под ваши условия",
      );
      await expect(res.getByTestId("guide-result-title")).toHaveText(
        "Kaspersky Endpoint Security — Расширенный",
      );
      await expect(res).toContainText("Новая лицензия · 30 компьютеров · 1 год");
      await expect(p.getByTestId("guide-feature")).toBeChecked();
      await expect(p.getByTestId("guide-other")).toHaveCount(2);
      await expect(p.getByTestId("guide-other").first()).toContainText(
        "Нет шифрования данных",
      );
      expect(await text(p.getByTestId("guide-sum"))).toBe("122 445 ₽");
      expect(await text(p.getByTestId("guide-formula"))).toBe(
        "30 × 4 081,50 ₽",
      );
      // Ни ленты сообщений, ни дат.
      expect(await p.innerText()).not.toMatch(DATE);
      await expect(p.getByText("Вы", { exact: true })).toHaveCount(0);
      // Выдача слева кликабельна при открытом подборе.
      await resultsArea(page).first().getByRole("button").first().click({ trial: true });
    });

    test("сценарий 2: сравнение редакций, «Выбрано», уточнение строкой поиска, спецификация", async ({
      page,
    }) => {
      const calls = await stubGuide(page);
      await page.goto("/");
      await searchGuide(
        page,
        "защита для компьютеров в офисе на 30 ПК, чем расширенный отличается от стандартного",
      );
      const p = panel(page);
      await expect(p.getByTestId("guide-task")).toHaveText(
        "Антивирус для организации · Kaspersky",
      );
      const table = p.getByTestId("guide-table");
      await expect(table.getByTestId("guide-row")).toHaveCount(4);
      await expect(table.locator('[data-row="encryption"] td').nth(0)).toContainText("Нет");
      await expect(table.locator('[data-row="encryption"] td').nth(1)).toContainText("Есть");
      await expect(table.getByTestId("guide-legend")).toContainText("Есть");
      await table.getByTestId("guide-more-rows").click();
      await expect(table.getByTestId("guide-row")).toHaveCount(5);
      await expect(table.getByTestId("guide-legend")).toContainText("С условиями");
      await expect(table).toContainText("Ещё 2 характеристики без сведений у вендора");
      expect(await text(table.getByTestId("guide-difference"))).toBe(
        "Меньшая стоимость закупки: Стандартный, на 38 043 ₽ дешевле.",
      );
      // Без назначенного победителя карточки нет; сумма — один раз, в итоговой строке.
      await expect(p.getByTestId("guide-result")).toHaveCount(0);
      expect(await text(p.getByTestId("guide-sum"))).toBe("84 402 ₽");
      expect((await text(p)).split("84 402 ₽").length - 1).toBe(1);
      // «Выбрано» меняет итоговую строку.
      await table.getByTestId("guide-selected").nth(1).check();
      expect(await text(p.getByTestId("guide-sum"))).toBe("122 445 ₽");
      expect(await text(p.getByTestId("guide-formula"))).toBe("30 × 4 081,50 ₽");

      // Реплика в строке поиска уточняет подбор: выдача не перезапрашивается, строка возвращается к запросу выдачи.
      let searched = 0;
      page.on("request", (r) => {
        if (r.url().endsWith("/api/search") && r.method() === "POST") searched++;
      });
      const box = searchBox(page);
      await box.fill("компьютеров будет 50, и шифрование нужно");
      await turn(page, () => box.press("Enter"));
      expect(calls.at(-1)).toMatchObject({
        message: "компьютеров будет 50, и шифрование нужно",
        state: { selection_id: "e2e-guide" },
      });
      expect(searched).toBe(0);
      await expect(box).toHaveValue(/чем расширенный отличается/);

      await expect(p.getByTestId("guide-changes")).toContainText(
        "Изменено: 30 → 50 компьютеров, шифрование обязательно",
      );
      expect(await text(p.getByTestId("guide-price-change"))).toContain(
        "25–49 компьютеров: 4 081,50 ₽ → 50–99 компьютеров: 3 940,20 ₽",
      );
      const res = p.getByTestId("guide-result");
      await expect(res.getByTestId("guide-result-title")).toHaveText(
        "Kaspersky Endpoint Security — Расширенный",
      );
      await expect(p.getByTestId("guide-other")).toContainText(
        "Не подходит: нет шифрования данных",
      );
      // Источник: текстовая ссылка, фрагмент и документ вендора.
      await res.getByTestId("guide-source").click();
      const pop = page.getByTestId("guide-source-pop");
      await expect(pop).toContainText("Шифрование файлов и дисков");
      await expect(pop.getByRole("link", { name: /Открыть документ вендора/ })).toHaveAttribute(
        "href",
        /kaspersky\.ru/,
      );
      await page.keyboard.press("Escape");
      // Сведения о лицензии — в раскрытии.
      await res.getByRole("button", { name: /Сведения о лицензии/ }).click();
      await expect(res.getByTestId("guide-details")).toContainText("Артикул");

      expect(await text(p.getByTestId("guide-sum"))).toBe("197 010 ₽");
      await p.getByTestId("guide-spec").click();
      const added = p.getByTestId("guide-added");
      await expect(added).toContainText("Добавлено в спецификацию");
      expect(await text(added)).toContain("197 010 ₽");
      await expect(added.getByRole("button", { name: "Открыть спецификацию" })).toBeVisible();
      // Условия не сброшены.
      await expect(p.getByTestId("guide-count")).toHaveValue("50");
      await expect(p.getByTestId("guide-feature")).toBeChecked();
      await expect(page.getByRole("button", { name: /^Спецификация, позиций: 1/ })).toBeVisible();
    });

    test("поле уточнения в рабочей области: тот же ход, «Вы: …» одной строкой, без ленты", async ({
      page,
    }) => {
      const calls = await stubGuide(page);
      await page.goto("/");
      await searchGuide(
        page,
        "защита для компьютеров в офисе на 30 ПК, чем расширенный отличается от стандартного",
      );
      const p = panel(page);
      const field = p.getByTestId("guide-text");
      await expect(field).toHaveAttribute("placeholder", /Уточните: например, 50 компьютеров/);
      await field.fill("компьютеров будет 50, и шифрование нужно");
      await turn(page, () => field.press("Enter"));
      expect(calls.at(-1)).toMatchObject({
        message: "компьютеров будет 50, и шифрование нужно",
        state: { selection_id: "e2e-guide" },
      });
      await expect(field).toHaveValue("");
      await expect(p.getByTestId("guide-said")).toHaveText(
        "Вы: компьютеров будет 50, и шифрование нужно",
      );
      await expect(p.getByTestId("guide-changes")).toContainText("Изменено: 30 → 50");
      await expect(p.getByTestId("guide-said")).toHaveCount(1);
      // Поле уточнения и итоговая строка видны без прокрутки области.
      await expect(field).toBeInViewport();
      await expect(p.getByTestId("guide-total")).toBeInViewport();
    });

    test("сценарий 3: серверная КриптоПро — подходящая, дешёвые на рабочее место с причиной", async ({
      page,
    }) => {
      await stubGuide(page);
      await page.goto("/");
      await searchGuide(page, "самый дешёвый криптопро csp для сервера");
      const p = panel(page);
      await expect(p.getByTestId("guide-task")).toHaveText("КриптоПро CSP для серверной ОС");
      const res = p.getByTestId("guide-result");
      await expect(res.getByTestId("guide-pick-status")).toHaveText("Подходящая лицензия");
      await expect(res.getByTestId("guide-result-title")).toHaveText("КриптоПро CSP 5.0");
      await expect(res).toContainText("Серверная лицензия · бессрочная");
      await expect(res.getByTestId("guide-source")).toHaveText("Правила лицензирования КриптоПро");
      const others = p.getByTestId("guide-other");
      await expect(others).toHaveCount(3);
      await expect(others.nth(0)).toContainText("Не подходит для серверной ОС");
      expect(await text(others.nth(0))).toContain("1 850 ₽");
      await expect(others.nth(2)).toContainText("Отдельная лицензия для TLS-сервера");
      expect(await text(p.getByTestId("guide-sum"))).toBe("70 000 ₽");
      expect(await text(p.getByTestId("guide-formula"))).toBe("1 × 70 000 ₽");
      await expect(p.getByTestId("guide-spec")).toHaveCount(1);
    });

    test("ожидание, пересчёт, ошибка с повтором, свернуть и вернуть без нового хода", async ({
      page,
    }) => {
      const calls = await stubGuide(page, { fail: 1, delayMs: 600 });
      await page.goto("/");
      await search(page, "подбери антивирь");
      const p = panel(page);
      // Ожидание: скелетон на месте результата, выдача уже на экране.
      await expect(p.getByTestId("guide-loading")).toContainText(
        "Проверяем условия лицензирования",
      );
      await expect(resultsArea(page).first()).toBeVisible();
      // Ошибка: условия и выдача целы, повтор.
      await expect(p.getByTestId("guide-error")).toContainText(
        "Не удалось завершить подбор. Результаты поиска доступны, ваши условия сохранены.",
      );
      await expect(resultsArea(page).first()).toBeVisible();
      await turn(page, () => p.getByRole("button", { name: "Повторить подбор" }).click());
      await expect(p.getByTestId("guide-question")).toBeVisible();

      await turn(page, () => p.getByTestId("guide-quick").filter({ hasText: /^30$/ }).click());
      // Пересчёт: прежний результат с пометкой, кнопка неактивна.
      await p.getByTestId("guide-cost-row").first().click();
      await expect(p.getByTestId("guide-spec")).toBeEnabled();
      const pending = page.waitForResponse(isGuide);
      await p.getByTestId("guide-op-renewal").click();
      await expect(p.getByTestId("guide-updating")).toHaveText("Обновляем подбор");
      await expect(p.getByTestId("guide-task-cost")).toBeVisible();
      await pending;
      expect(calls.at(-1)).toMatchObject({ condition: { key: "operation", value: "renewal" } });
      await expect(p.getByTestId("guide-updating")).toHaveCount(0);

      // Степпер: несколько нажатий — один ход.
      const before = calls.length;
      await p.getByTestId("guide-count-plus").click();
      await p.getByTestId("guide-count-plus").click();
      await turn(page, () => p.getByTestId("guide-count-plus").click());
      expect(calls.length).toBe(before + 1);
      expect((calls.at(-1) as GuideBody).condition).toEqual({
        key: "count:workstation",
        value: "33",
      });

      // Свернуть: строка-приглашение возвращает подбор с теми же условиями без нового хода.
      const n = calls.length;
      await p.getByTestId("guide-close").click();
      await expect(p).toHaveCount(0);
      const hint = page.getByTestId("guide-hint");
      await expect(hint).toContainText("условия сохранены");
      await hint.getByTestId("guide-start").click();
      await expect(panel(page).getByTestId("guide-count")).toHaveValue("30");
      expect(calls.length).toBe(n);
    });

    test("1280 px: фильтры выдачи — кнопкой, подбор 480–560 px", async ({ page }) => {
      await page.setViewportSize({ width: 1280, height: 800 });
      await stubGuide(page);
      await page.goto("/");
      await searchGuide(page, "самый дешёвый криптопро csp для сервера");
      const p = panel(page);
      const box = (await p.boundingBox())!;
      expect(box.width).toBeGreaterThanOrEqual(479);
      expect(box.width).toBeLessThanOrEqual(561);
      const toggle = page.getByTestId("filters-toggle");
      await expect(toggle).toBeVisible();
      await expect(page.locator(".search-page__facets")).toBeHidden();
      await toggle.click();
      await expect(page.locator(".search-page__facets")).toBeVisible();
      await expect(p.getByTestId("guide-total")).toBeInViewport();
      await noHorizontalScroll(page);
    });

    test("360 px: блок над выдачей, условия строкой, таблица строками, сворачивание", async ({
      page,
    }) => {
      await page.setViewportSize({ width: 360, height: 760 });
      await stubGuide(page);
      await page.goto("/");
      await searchGuide(
        page,
        "защита для компьютеров в офисе на 30 ПК, чем расширенный отличается от стандартного",
      );
      const p = panel(page);
      const pb = (await p.boundingBox())!;
      const rb = (await resultsArea(page).first().boundingBox())!;
      expect(pb.y).toBeLessThan(rb.y);
      expect(await text(p.getByTestId("guide-fields-line"))).toBe("30 ПК · Новая · 1 год");
      await expect(p.getByTestId("guide-count")).toHaveCount(0);
      await p.getByRole("button", { name: "Изменить условия" }).click();
      await expect(p.getByTestId("guide-count")).toHaveValue("30");
      await expect(p.getByTestId("guide-table").locator("table")).toHaveCount(0);
      await expect(p.getByTestId("guide-row").first()).toContainText("Стандартный");
      await noHorizontalScroll(page);
      await p.getByTestId("guide-collapse").click();
      await expect(p.getByTestId("guide-collapsed-title")).toHaveText(
        "Kaspersky Endpoint Security — Стандартный",
      );
      await expect(p.getByTestId("guide-total")).toHaveCount(0);
      await p.getByTestId("guide-collapse").click();
      await expect(p.getByTestId("guide-table")).toBeVisible();
      await noHorizontalScroll(page);
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
      await expect
        .poll(() => new URL(page.url()).searchParams.has("guide"))
        .toBe(false);
      await expect(resultsArea(page).first()).toBeVisible();
    });

    test("общий запрос — строка-приглашение, открытие по кнопке", async ({ page }) => {
      const calls = await stubGuide(page);
      await page.goto("/");
      await search(page, "антивирус");
      const hint = page.getByTestId("guide-hint");
      await expect(hint).toContainText("Не знаете, какую лицензию выбрать?");
      await expect(hint).toContainText("Сравните редакции и рассчитайте стоимость");
      const h = (await hint.boundingBox())!.height;
      expect(h).toBeGreaterThanOrEqual(71);
      expect(h).toBeLessThanOrEqual(97);
      expect(calls).toHaveLength(0);
      await turn(page, () => hint.getByTestId("guide-start").click());
      await expect(panel(page).getByTestId("guide-question")).toBeVisible();
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
    test.skip(!SHOTS || GUIDE_LIVE, "E2E_GUIDE_SHOTS=<папка> — снимки на заглушках");
    for (const width of [1440, 1280, 360]) {
      test(`сценарии 2 и 3 на ${width}`, async ({ page }) => {
        await page.setViewportSize({ width, height: width < 600 ? 760 : 900 });
        await stubGuide(page);
        await page.goto("/");
        await searchGuide(
          page,
          "защита для компьютеров в офисе на 30 ПК, чем расширенный отличается от стандартного",
        );
        await page.screenshot({ path: `${SHOTS}/p9-s2-compare-${width}.png` });
        const box = searchBox(page);
        await box.fill("компьютеров будет 50, и шифрование нужно");
        await turn(page, () => box.press("Enter"));
        await page.screenshot({ path: `${SHOTS}/p9-s2-fifty-${width}.png` });
        await searchGuide(page, "самый дешёвый криптопро csp для сервера");
        await page.screenshot({ path: `${SHOTS}/p9-s3-${width}.png` });
      });
    }
  });

  test.describe("живой API", () => {
    test.skip(!GUIDE_LIVE, "E2E_GUIDE_LIVE=1 — подбор на живом API");
    test.describe.configure({ timeout: 180_000 });

    test("сценарий 2 вживую: сравнение, 30 → 50, спецификация", async ({ page }) => {
      await page.goto("/");
      await searchGuide(
        page,
        "защита для компьютеров в офисе на 30 ПК, чем расширенный отличается от стандартного",
      );
      const p = panel(page);
      const table = p.getByTestId("guide-table");
      await expect(table).toContainText("Стандартный");
      const rows = await table.getByTestId("guide-row").count();
      expect.soft(rows).toBeGreaterThanOrEqual(2);
      expect.soft(rows).toBeLessThanOrEqual(6);
      expect.soft(await text(p.getByTestId("guide-sum"))).toBe("84 402 ₽");
      const box = searchBox(page);
      await box.fill("компьютеров будет 50, и шифрование нужно");
      await turn(page, () => box.press("Enter"));
      await expect.soft(p.getByTestId("guide-changes")).toContainText("30 → 50");
      expect.soft(await text(p.getByTestId("guide-sum"))).toBe("197 010 ₽");
      await expect.soft(p.getByTestId("guide-result")).toContainText("Расширенный");
      expect(await p.innerText()).not.toMatch(DATE);
      await p.getByTestId("guide-spec").click();
      await expect(p.getByTestId("guide-added")).toContainText("Добавлено в спецификацию");
    });

    test("сценарий 3 вживую: серверная первой, рабочее место — с причиной", async ({ page }) => {
      await page.goto("/");
      await searchGuide(page, "самый дешёвый криптопро csp для сервера");
      const p = panel(page);
      await expect.soft(p.getByTestId("guide-pick-status")).toHaveText("Подходящая лицензия");
      expect.soft(await text(p.getByTestId("guide-sum"))).toBe("70 000 ₽");
      await expect.soft(p.getByTestId("guide-other").first()).toContainText(
        "Не подходит для серверной ОС",
      );
      expect(await p.innerText()).not.toMatch(DATE);
    });

    test("сценарий 1 вживую: вопрос о количестве", async ({ page }) => {
      await page.goto("/");
      await searchGuide(page, "подбери антивирь");
      const p = panel(page);
      await expect(p.getByTestId("guide-question")).toContainText("Сколько компьютеров");
      await turn(page, () => p.getByTestId("guide-quick").filter({ hasText: /^30$/ }).click());
      await expect(p.getByTestId("guide-count")).toHaveValue("30");
    });
  });
});
