// Консультант контура консультанта (CONTRACT.md, SCENARIOS.md) на заглушке событий
// фронта (localStorage.consultant_mock = '1', src/lib/consultantMock.ts): окно открывается ТОЛЬКО по нажатию; сценарии
// 2 и 3 текстом; карточка, список с четырьмя позициями и причиной, таблица со стоимостью; «В спецификацию» и счётчик;
// ошибка с повтором и запасной ответ; история при смене страницы; новый разговор не трогает спецификацию; 360 px.
// Помощник контура помощника при выключенном флаге — в p10-assistant.spec.ts. Без звука.
import type { Page } from "@playwright/test";
import { GUEST, expect, test } from "./fixtures";

const win = (page: Page) => page.getByTestId("consultant-window");
const launcher = (page: Page) => page.getByTestId("consultant-launcher");
const input = (page: Page) => page.getByTestId("consultant-input");
const plain = (s: string) => s.replace(/[  ]/g, " ");

async function start(page: Page, path = "/search") {
  await page.addInitScript(() => {
    try {
      window.localStorage.setItem("consultant_mock", "1");
      window.localStorage.removeItem("plexet.spec");
    } catch {
      /* без хранилища */
    }
  });
  await page.goto(path);
}

async function say(page: Page, text: string) {
  await input(page).fill(text);
  await input(page).press("Enter");
  await expect(page.getByTestId("consultant-typing")).toHaveCount(0, { timeout: 15_000 });
}

async function noHorizontalScroll(page: Page) {
  const over = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(over, "горизонтальной прокрутки нет").toBeLessThanOrEqual(0);
}

test.describe("П11: консультант на заглушке", () => {
  test.use({ storageState: GUEST });

  test("открывается только по нажатию; помощника контура помощника нет; приветствие и четыре подсказки", async ({
    page,
  }) => {
    await start(page);
    await expect(launcher(page)).toBeVisible();
    await expect(page.getByTestId("assistant-entry")).toHaveCount(0);
    await page.waitForTimeout(1000);
    await expect(win(page)).toBeHidden();
    await launcher(page).click();
    await expect(win(page)).toBeVisible();
    await expect(win(page)).toContainText("Консультант");
    await expect(win(page)).toContainText("Виртуальный");
    await expect(win(page)).toContainText("Здравствуйте! Помогу подобрать ПО");
    await expect(page.getByTestId("consultant-starters").getByRole("button")).toHaveCount(4);
    await expect(input(page)).toBeFocused();
    await launcher(page).click();
    await expect(win(page)).toBeHidden();
  });

  test("сценарий 2 текстом: таблица 84 402 / 122 445 ₽ → 50 и шифрование → 197 010 ₽ → в спецификацию", async ({
    page,
  }) => {
    await start(page);
    await launcher(page).click();
    const count = page.getByTestId("consultant-spec-count");
    await expect(count).toHaveText("0");
    await say(page, "Чем расширенный касперский отличается от стандартного, нам на 30 компьютеров");
    await expect(page.getByTestId("consultant-starters")).toHaveCount(0);
    const table = page.getByTestId("consultant-comparison");
    await expect(table).toBeVisible();
    const t = plain(await table.innerText());
    expect(t).toContain("84 402 ₽");
    expect(t).toContain("122 445 ₽");
    expect(t).toContain("Шифрование");
    // Текст ответа стоит перед вложениями; полужирные цены в тексте.
    const texts = page.getByTestId("consultant-text");
    await expect(texts.filter({ hasText: "На 30 компьютеров" }).locator("strong")).toHaveCount(2);
    await expect(page.getByTestId("consultant-quick-reply")).toHaveCount(2);

    await say(page, "Компьютеров будет 50, и шифрование нужно");
    await expect(page.getByTestId("consultant-quick-reply")).toHaveCount(0);
    const card = page.getByTestId("consultant-card").last();
    expect(plain(await card.getByTestId("consultant-card-price").innerText())).toBe("197 010 ₽");
    expect(plain(await card.innerText())).toContain("50 × 3 940,20 ₽");
    await card.getByTestId("consultant-card-add").click();
    await expect(page.getByTestId("consultant-spec-event").last()).toContainText(
      "Добавлено в спецификацию",
    );
    await expect(count).toHaveText("1");
    await expect(card.getByTestId("consultant-card-add")).toHaveText(/В спецификации/);
    await expect(card.getByTestId("consultant-card-add")).toBeDisabled();
    // Добавление текстом той же позиции не создаёт дубль.
    await say(page, "добавь его");
    await expect(page.getByTestId("consultant-spec-event").last()).toContainText("Добавлено");
    await expect(count).toHaveText("1");
    // Удаление из ответа отражается в счётчике.
    await say(page, "убери антивирус");
    await expect(page.getByTestId("consultant-spec-event").last()).toContainText("Удалено");
    await expect(count).toHaveText("0");
  });

  test("сценарий 3 текстом: четыре варианта без раскрытия, неподходящие с причиной, источник", async ({
    page,
  }) => {
    await start(page);
    await launcher(page).click();
    await say(page, "самый дешёвый криптопро csp для сервера");
    const list = page.getByTestId("consultant-list");
    await expect(list.getByTestId("consultant-list-row")).toHaveCount(4);
    const unfit = list.locator('[data-testid="consultant-list-row"][data-kind="unfit"]');
    await expect(unfit).toHaveCount(2);
    await expect(unfit.first()).toContainText("Не подходит для серверной ОС");
    const lt = plain(await list.innerText());
    expect(lt.indexOf("70 000 ₽")).toBeLessThan(lt.indexOf("1 850 ₽"));
    await expect(page.getByTestId("consultant-source")).toContainText("Правила лицензирования");
    await expect(page.getByTestId("consultant-quick-reply")).toHaveCount(3);
    expect(lt).not.toMatch(/(^|[^\d ])0 ₽/);
  });

  test("ошибка модели: запасной ответ со списком; без ответа — «Попробовать ещё раз» без дубля реплики", async ({
    page,
  }) => {
    await start(page);
    await launcher(page).click();
    await say(page, "ошибка");
    await expect(win(page)).toContainText("Не получилось сформулировать ответ, но вот что нашлось в каталоге");
    await expect(page.getByTestId("consultant-list")).toBeVisible();
    await say(page, "сбой");
    await expect(page.getByTestId("consultant-retry")).toBeVisible();
    const users = page.locator('[data-testid="consultant-message"][data-role="user"]');
    await expect(users).toHaveCount(2);
    await page.getByTestId("consultant-retry").click();
    await expect(page.getByTestId("consultant-typing")).toHaveCount(0, { timeout: 15_000 });
    await expect(users).toHaveCount(2);
  });

  test("история и черновик переживают закрытие и смену страницы; новый разговор не трогает спецификацию", async ({
    page,
  }) => {
    await start(page);
    await launcher(page).click();
    await say(page, "Компьютеров будет 50, и шифрование нужно");
    await page.getByTestId("consultant-card-add").click();
    await expect(page.getByTestId("consultant-spec-count")).toHaveText("1");
    await input(page).fill("черновик");
    await page.getByTestId("consultant-close").click();
    await expect(win(page)).toBeHidden();
    await page.getByRole("link", { name: "Доказательства" }).first().click();
    await expect(page).toHaveURL(/\/proofs/);
    await launcher(page).click();
    await expect(win(page)).toContainText("197");
    await expect(input(page)).toHaveValue("черновик");

    await page.getByTestId("consultant-menu").click();
    await page.getByTestId("consultant-new").click();
    await expect(page.getByTestId("consultant-new-dialog")).toContainText(
      "Товары в спецификации останутся",
    );
    await page.getByTestId("consultant-new-confirm").click();
    await expect(page.locator('[data-testid="consultant-message"][data-role="user"]')).toHaveCount(0);
    await expect(page.getByTestId("consultant-starters")).toBeVisible();
    await expect(page.getByTestId("consultant-spec-count")).toHaveText("1");
  });

  test("развёрнутое окно 640 × 760; телефон 360 px — полный экран без горизонтальной прокрутки", async ({
    page,
  }) => {
    await start(page);
    await launcher(page).click();
    await page.getByTestId("consultant-expand").click();
    const box = (await win(page).boundingBox())!;
    expect(Math.round(box.width)).toBe(640);
    expect(Math.round(box.height)).toBeLessThanOrEqual(760);
    await page.getByTestId("consultant-expand").click();

    await page.setViewportSize({ width: 360, height: 740 });
    await say(page, "Чем расширенный касперский отличается от стандартного, нам на 30 компьютеров");
    const b = (await win(page).boundingBox())!;
    expect(Math.round(b.width)).toBe(360);
    expect(Math.round(b.x)).toBe(0);
    await noHorizontalScroll(page);
    const feedOver = await page
      .getByTestId("consultant-feed")
      .evaluate((el) => el.scrollWidth - el.clientWidth);
    expect(feedOver).toBeLessThanOrEqual(0);
    await say(page, "самый дешёвый криптопро csp для сервера");
    await noHorizontalScroll(page);
    await expect(input(page)).toBeInViewport();
  });
});
