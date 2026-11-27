// Поиск по ТЗ (контур ТЗ): скрепка и зона сброса, полоса
// разбора с настоящими этапами, отмена без позднего перехода, страница /search?tz=<хеш>, статусы позиций, «Ошибка
// поиска» с повтором одной позиции, не больше четырёх поисков одновременно, спецификация «из ТЗ», возврат из полного
// поиска, недоступная ссылка, телефон 360 px. По умолчанию /api/tz и поиск по позициям (source "tz") — заглушки
// ответами по контракту TZ-CORE.md §2; E2E_TZ_LIVE=1 — разбор тестового файла на живом API.
import type { Page, Route } from "@playwright/test";
import { fileURLToPath } from "node:url";
import { GUEST, expect, test } from "./fixtures";
import { TZ_DOC, TZ_HASH } from "./p12-tz-data";

const LIVE = process.env.E2E_TZ_LIVE === "1";
const PDF = fileURLToPath(
  new URL("../../data/softline/eval/TZ_Severny_veter.pdf", import.meta.url),
);
const DOCX = fileURLToPath(
  new URL("../../data/softline/eval/TZ_Severny_veter.docx", import.meta.url),
);
const plain = (s: string) => s.replace(/[  ]/g, " ");

type Item = Record<string, unknown>;
const item = (id: string, over: Item = {}): Item => ({
  product_id: id,
  family_id: `f-${id}`,
  title: id,
  vendor: "Вендор",
  category: "ПО",
  license_line: "",
  price_rub: 1000,
  price_on_request: false,
  group: "match",
  match_type: "related",
  matches_query: true,
  purchased_before: false,
  final: 0.5,
  variants: [],
  ...over,
});

/** Ответ поиска по позиции: по номеру позиции из текста запроса. */
function searchItems(query: string): Item[] {
  const n = TZ_DOC.positions.find((p) => p.search_query === query)?.number;
  switch (n) {
    case 1:
      return [
        item("kes-std", {
          title: "Kaspersky Endpoint Security для бизнеса Стандартный",
          match_type: "exact",
          group: "exact",
          price_rub: 2715.3,
          tier_min: 50,
          tier_max: 99,
          attrs: { license_metric: "device", duration_months: 12, license_type: "new" },
        }),
        item("kes-ext", { match_type: "exact", group: "exact" }),
        item("kes-plus", { match_type: "exact", group: "exact" }),
        item("kes-cloud", { match_type: "exact", group: "exact" }),
      ];
    case 2:
      return [item("astra", { match_type: "exact", group: "exact", price_rub: 6150 })];
    case 3:
      return [item("cyber-1"), item("cyber-2"), item("cyber-3")];
    case 4:
      return [item("csp-server", { match_type: "exact", group: "exact", price_rub: 70000 })];
    case 5:
      return [item("o1"), item("o2"), item("o3"), item("o4"), item("o5")];
    case 6:
      return [item("ngfw-1"), item("ngfw-2")];
    case 7:
      return [item("server-1")];
    case 8:
      return [item("random-service-goods")];
    default:
      return [];
  }
}

interface Stub {
  peak: number;
  searches: string[];
  deletes: number;
  polls: number;
}

/**
 * Заглушки /api/tz (операция: два опроса «reading», затем completed) и поиска с source "tz". failOnce — номер позиции,
 * чей первый поиск отвечает 500.
 */
async function stubTz(
  page: Page,
  opts: { failOnce?: number; stages?: string[]; never?: boolean } = {},
): Promise<Stub> {
  const st: Stub = { peak: 0, searches: [], deletes: 0, polls: 0 };
  if (LIVE) return st;
  const stages = opts.stages ?? ["reading", "reading"];
  let inflight = 0;
  let failed = false;
  await page.route("**/api/tz", (r) => {
    st.polls = 0;
    return r.fulfill({ json: { operation_id: "op-1" } });
  });
  await page.route("**/api/tz/operations/*", (r) => {
    if (r.request().method() === "DELETE") {
      st.deletes++;
      return r.fulfill({ status: 204 });
    }
    const i = st.polls++;
    if (opts.never || i < stages.length)
      return r.fulfill({ json: { status: "processing", stage: stages[Math.min(i, stages.length - 1)] } });
    return r.fulfill({ json: { status: "completed", hash: TZ_HASH } });
  });
  await page.route(`**/api/tz/${TZ_HASH}`, (r) => r.fulfill({ json: TZ_DOC }));
  await page.route("**/api/tz/expired-hash", (r) =>
    r.fulfill({ status: 404, json: { error: { code: "NOT_FOUND", message: "нет" } } }),
  );
  await page.route("**/api/search", async (r: Route) => {
    const body = r.request().postDataJSON() as { query: string; source?: string };
    if (body.source !== "tz") return r.fallback();
    st.searches.push(body.query);
    inflight++;
    st.peak = Math.max(st.peak, inflight);
    await new Promise((res) => setTimeout(res, 150));
    inflight--;
    const n = TZ_DOC.positions.find((p) => p.search_query === body.query)?.number;
    if (n === opts.failOnce && !failed) {
      failed = true;
      return r.fulfill({ status: 500, json: { error: { code: "INTERNAL", message: "сбой" } } });
    }
    return r.fulfill({
      json: {
        request_id: `rid-${n}`,
        query: body.query,
        normalized: body.query,
        config: "B3",
        chips: [],
        total: 1,
        items: searchItems(body.query),
      },
    });
  });
  return st;
}

const position = (page: Page, n: number) =>
  page.locator(`[data-testid="tz-position"][data-number="${n}"]`);

async function uploadFromSearch(page: Page, file = PDF) {
  await page.goto("/search");
  await page.getByTestId("tz-file-input").setInputFiles(file);
}

async function noHorizontalScroll(page: Page) {
  const over = await page.evaluate(() => document.documentElement.scrollWidth - window.innerWidth);
  expect(over, "горизонтальной прокрутки нет").toBeLessThanOrEqual(0);
}

test.describe("П12: поиск по ТЗ", () => {
  test.use({ storageState: GUEST });

  test.beforeEach(async ({ page }) => {
    await page.addInitScript(() => {
      if (!sessionStorage.getItem("p12-init")) {
        localStorage.removeItem("plexet.spec");
        sessionStorage.setItem("p12-init", "1");
      }
    });
  });

  test("скрепка → полоса с настоящими этапами → страница ТЗ; статусы, «Не проверено», до четырёх «Похожих»", async ({
    page,
  }) => {
    const st = await stubTz(page);
    await page.goto("/search");
    const attach = page.getByRole("button", { name: "Загрузить техническое задание" });
    await expect(attach).toBeVisible();
    // Скрепка между крестиком и «Найти»: левее кнопки поиска.
    const a = await attach.boundingBox();
    const f = await page.locator(".sp__submit").boundingBox();
    expect(a!.x + a!.width).toBeLessThanOrEqual(f!.x);

    await page.getByTestId("tz-file-input").setInputFiles(PDF);
    const bar = page.getByTestId("tz-processing");
    await expect(bar).toBeVisible();
    await expect(page.getByTestId("tz-processing-file")).toHaveText("TZ_Severny_veter.pdf");
    // Для текстового документа этапа распознавания нет; процентов нет.
    await expect(page.getByTestId("tz-step-ocr")).toHaveCount(0);
    await expect(page.getByTestId("tz-step-read")).toHaveAttribute("data-state", "active");
    expect(await bar.innerText()).not.toMatch(/%/);
    // Строка поиска рабочая во время разбора.
    await expect(page.getByRole("combobox", { name: "Поиск по каталогу" })).toBeEditable();

    await page.waitForURL(new RegExp(`tz=${TZ_HASH}`));
    if (LIVE) return;
    await expect(page.getByTestId("tz-file-chip")).toHaveText("TZ_Severny_veter.pdf");
    await expect(page.locator(".sp__submit")).toBeDisabled();
    // Позиции сразу в порядке документа.
    await expect(page.getByTestId("tz-position")).toHaveCount(8);
    const order = await page
      .getByTestId("tz-position")
      .evaluateAll((els) => els.map((e) => Number((e as HTMLElement).dataset.number)));
    expect(order).toEqual([1, 2, 3, 4, 5, 6, 7, 8]);
    await expect(page.getByTestId("tz-title")).toHaveText(
      "По ТЗ «TZ_Severny_veter.pdf» — 8 позиций · найдено 7",
    );
    expect(st.peak, "не больше четырёх поисков одновременно").toBeLessThanOrEqual(4);
    expect(st.searches).toHaveLength(8);

    const status = async (n: number) => position(page, n).getAttribute("data-status");
    expect(await status(1)).toBe("found");
    expect(await status(3)).toBe("similar");
    // Работа без точного совпадения — «Нет в каталоге», без случайных товаров.
    expect(await status(8)).toBe("not_found");
    await expect(position(page, 8).locator(".fcard")).toHaveCount(0);
    await expect(position(page, 8).getByTestId("tz-manager")).toBeVisible();
    // Статус — значок и слово.
    await expect(position(page, 1).getByTestId("tz-position-status")).toHaveText("Найдено");
    await expect(position(page, 1).getByTestId("tz-position-status").locator("svg")).toHaveCount(1);

    // «Не проверено» видно без раскрытия, перед карточками.
    await expect(position(page, 3).getByTestId("tz-unverified")).toHaveText(
      "Не проверено: включение в Единый реестр российских программ",
    );
    // «Похожие» — не больше четырёх карточек, без «ещё».
    await expect(position(page, 5).locator(".fcard")).toHaveCount(4);
    await expect(position(page, 5).getByTestId("tz-more")).toHaveCount(0);
    // «Найдено» — одна карточка и «Ещё 3 варианта», раскрываются на месте.
    await expect(position(page, 1).locator(".fcard")).toHaveCount(1);
    await position(page, 1).getByTestId("tz-more").click();
    await expect(position(page, 1).locator(".fcard")).toHaveCount(4);
    await expect(position(page, 1).getByTestId("tz-more")).toHaveText("Свернуть варианты");
    // Цитата — дословный текст документа.
    await position(page, 1).getByTestId("tz-quote-toggle").click();
    await expect(position(page, 1).getByTestId("tz-quote")).toContainText(
      TZ_DOC.positions[0]!.source_quote,
    );
    // Фильтров, сортировки и уточнений нет; слева — список позиций.
    await expect(page.getByTestId("tz-nav")).toBeVisible();
    await expect(page.getByTestId("tz-nav-item")).toHaveCount(8);
    await expect(page.getByText("Сортировка")).toHaveCount(0);
    await expect(page.getByText("Фильтры", { exact: true })).toHaveCount(0);
  });

  test("спецификация: кнопка на карточке, количество из ТЗ, «из ТЗ», «по ТЗ, п. 1», отметка ставится и снимается", async ({
    page,
  }) => {
    test.skip(LIVE, "на заглушке");
    await stubTz(page);
    await page.goto(`/search?tz=${TZ_HASH}`);
    await expect(page.getByTestId("tz-title")).toContainText("найдено");
    // Нет чекбоксов и массового добавления.
    await expect(page.locator('[data-testid="tz-results"] input[type="checkbox"]')).toHaveCount(0);
    await expect(page.getByRole("button", { name: /добавить вс/i })).toHaveCount(0);

    const card = position(page, 1).locator(".fcard").first();
    await expect(card.getByTestId("card-price")).toHaveText(/135\s765\s₽/);
    await expect(card.getByTestId("compact-calc")).toHaveText(/2\s715,30\s₽ × 50/);
    await card.getByTestId("compact-spec").click();
    await expect(page.getByTestId("tz-nav-added")).toHaveCount(1);
    await expect(page.getByTestId("tz-progress")).toHaveText("1 из 8 в спецификации");

    await page.getByRole("button", { name: /^Спецификация, позиций/ }).click();
    const sheet = page.getByRole("dialog", { name: "Спецификация" });
    await expect(sheet.getByText("из ТЗ", { exact: true })).toBeVisible();
    await expect(sheet.getByTestId("spec-purchase")).toContainText("по ТЗ, п. 1");
    await expect(sheet.getByRole("spinbutton").first()).toHaveValue("50");
    await sheet.getByRole("button", { name: /^Удалить:/ }).click();
    await page.keyboard.press("Escape");
    await expect(page.getByTestId("tz-nav-added")).toHaveCount(0);
  });

  test("ошибка одной позиции — «Ошибка поиска» с повтором только её; остальные готовы", async ({
    page,
  }) => {
    test.skip(LIVE, "на заглушке");
    const st = await stubTz(page, { failOnce: 6 });
    await page.goto(`/search?tz=${TZ_HASH}`);
    await expect(position(page, 6)).toHaveAttribute("data-state", "error");
    await expect(position(page, 6).getByTestId("tz-position-status")).toHaveText("Ошибка поиска");
    await expect(position(page, 6)).not.toHaveAttribute("data-status", "not_found");
    await expect(position(page, 1)).toHaveAttribute("data-status", "found");
    const before = st.searches.length;
    await position(page, 6).getByTestId("tz-position-retry").click();
    await expect(position(page, 6)).toHaveAttribute("data-status", "similar");
    expect(st.searches.length - before, "повтор — только для этой позиции").toBe(1);
  });

  test("отмена и обычный поиск во время разбора не переводят на страницу ТЗ", async ({ page }) => {
    test.skip(LIVE, "на заглушке");
    const st = await stubTz(page, { stages: ["reading", "ocr", "ocr"] });
    await uploadFromSearch(page, DOCX);
    await expect(page.getByTestId("tz-step-ocr")).toHaveAttribute("data-state", "active");
    await expect(page.getByTestId("tz-long-ocr")).toBeVisible();
    await page.getByTestId("tz-cancel").click();
    await expect(page.getByTestId("tz-processing")).toHaveCount(0);
    await page.waitForTimeout(4500);
    expect(page.url()).not.toContain("tz=");
    expect(st.deletes).toBe(1);

    await page.getByTestId("tz-file-input").setInputFiles(DOCX);
    await expect(page.getByTestId("tz-processing")).toBeVisible();
    const box = page.getByRole("combobox", { name: "Поиск по каталогу" });
    await box.fill("астра линукс");
    await box.press("Enter");
    await page.waitForTimeout(4500);
    expect(page.url()).toContain("q=");
    expect(page.url()).not.toContain("tz=");
  });

  test("ошибки загрузки под строкой поиска не исчезают сами: формат, несколько файлов, ответ сервера", async ({
    page,
  }) => {
    await page.goto("/search");
    await page
      .getByTestId("tz-file-input")
      .setInputFiles({ name: "tz.txt", mimeType: "text/plain", buffer: Buffer.from("x") });
    const err = page.getByTestId("tz-upload-error");
    await expect(err).toContainText("Поддерживаются только PDF и DOCX");
    await page.waitForTimeout(3000);
    await expect(err).toBeVisible();
    // Несколько файлов перетаскиванием — не отправляется ни один.
    let posts = 0;
    page.on("request", (r) => {
      if (r.method() === "POST" && new URL(r.url()).pathname === "/api/tz") posts++;
    });
    await page.evaluate(() => {
      const dt = new DataTransfer();
      dt.items.add(new File(["%PDF"], "a.pdf", { type: "application/pdf" }));
      dt.items.add(new File(["%PDF"], "b.pdf", { type: "application/pdf" }));
      document.body.dispatchEvent(new DragEvent("dragenter", { dataTransfer: dt, bubbles: true, cancelable: true }));
      document.body.dispatchEvent(new DragEvent("drop", { dataTransfer: dt, bubbles: true, cancelable: true }));
    });
    await expect(err).toContainText("Загрузите один файл технического задания.");
    expect(posts).toBe(0);
    if (LIVE) return;
    // Ответ сервера «защищён паролем» — в полосе разбора.
    await page.route("**/api/tz", (r) => r.fulfill({ json: { operation_id: "op-2" } }));
    await page.route("**/api/tz/operations/*", (r) =>
      r.fulfill({ json: { status: "error", reason: "password" } }),
    );
    await page.getByTestId("tz-file-input").setInputFiles(PDF);
    await expect(err).toContainText("Документ защищён паролем. Загрузите файл без защиты.");
    await expect(err).toHaveAttribute("data-place", "bar");
  });

  test("зона сброса — на любой странице; файл с «Доказательств» ведёт на страницу ТЗ", async ({
    page,
  }) => {
    test.skip(LIVE, "на заглушке");
    await stubTz(page);
    await page.goto("/proofs");
    await expect(page.getByTestId("tz-file-input")).toBeAttached();
    await page.evaluate(() => {
      const dt = new DataTransfer();
      dt.items.add(new File(["%PDF-1.4"], "TZ_drop.pdf", { type: "application/pdf" }));
      document.body.dispatchEvent(new DragEvent("dragenter", { dataTransfer: dt, bubbles: true, cancelable: true }));
      (window as unknown as { __dt: DataTransfer }).__dt = dt;
    });
    await expect(page.getByTestId("tz-dropzone")).toContainText("Перетащите ТЗ сюда");
    // Перетаскивание текста внутри страницы зону не вызывает.
    await page.evaluate(() => {
      window.dispatchEvent(new DragEvent("dragleave", { bubbles: true }));
    });
    await expect(page.getByTestId("tz-dropzone")).toHaveCount(0);
    await page.evaluate(() => {
      const dt = new DataTransfer();
      dt.setData("text/plain", "текст");
      document.body.dispatchEvent(new DragEvent("dragenter", { dataTransfer: dt, bubbles: true, cancelable: true }));
    });
    await expect(page.getByTestId("tz-dropzone")).toHaveCount(0);
    await page.evaluate(() => {
      const dt = (window as unknown as { __dt: DataTransfer }).__dt;
      document.body.dispatchEvent(new DragEvent("dragenter", { dataTransfer: dt, bubbles: true, cancelable: true }));
      document.body.dispatchEvent(new DragEvent("drop", { dataTransfer: dt, bubbles: true, cancelable: true }));
    });
    // Прежняя страница доступна до извлечения позиций.
    await expect(page.getByTestId("tz-processing")).toBeVisible();
    expect(new URL(page.url()).pathname).toBe("/proofs");
    await page.waitForURL(new RegExp(`tz=${TZ_HASH}`));
  });

  test("недоступная ссылка — рабочая страница поиска с «Загрузить ТЗ»; повторное открытие не запускает разбор", async ({
    page,
  }) => {
    test.skip(LIVE, "на заглушке");
    const st = await stubTz(page);
    let uploads = 0;
    page.on("request", (r) => {
      if (r.method() === "POST" && new URL(r.url()).pathname === "/api/tz") uploads++;
    });
    await page.goto("/search?tz=expired-hash");
    await expect(page.getByTestId("tz-unavailable")).toContainText("Результаты по ТЗ недоступны");
    await expect(page.getByTestId("tz-upload-again")).toBeVisible();
    const box = page.getByRole("combobox", { name: "Поиск по каталогу" });
    await expect(box).toBeEditable();
    await page.goto(`/search?tz=${TZ_HASH}`);
    await expect(page.getByTestId("tz-title")).toContainText("найдено");
    expect(uploads, "открытие по хешу не отправляет файл").toBe(0);
    expect(st.polls).toBe(0);
  });

  test("заголовок позиции открывает полный поиск, «Вернуться к ТЗ» восстанавливает прокрутку и раскрытое", async ({
    page,
  }) => {
    test.skip(LIVE, "на заглушке");
    const st = await stubTz(page);
    await page.goto(`/search?tz=${TZ_HASH}`);
    await expect(page.getByTestId("tz-title")).toContainText("найдено");
    await position(page, 1).getByTestId("tz-quote-toggle").click();
    await page.getByTestId("tz-nav-item").nth(4).click();
    await page.waitForTimeout(800);
    const y = await page.evaluate(() => Math.round(window.scrollY));
    expect(y).toBeGreaterThan(200);
    await expect(page.locator(".tzr__nav-item.is-current")).toContainText("Облачный офисный пакет");
    const searches = st.searches.length;
    await position(page, 5).locator(".tzp__title-link").click();
    await page.waitForURL(/fromTz=/);
    expect(page.url()).toContain("tzItem=5");
    await expect(page.getByTestId("tz-back")).toHaveText(
      "Вернуться к ТЗ «TZ_Severny_veter.pdf» · позиция 5 из 8",
    );
    await page.getByTestId("tz-back").click();
    await page.waitForURL(new RegExp(`tz=${TZ_HASH}`));
    await expect(position(page, 1).getByTestId("tz-quote")).toBeVisible();
    await expect
      .poll(() => page.evaluate(() => Math.round(window.scrollY)))
      .toBeGreaterThan(y - 40);
    expect(st.searches.length, "возврат без новых поисков по позициям").toBe(searches);
  });

  test("телефон 360: селектор позиций, вертикальная карточка, без горизонтальной прокрутки, 44 px", async ({
    page,
  }) => {
    test.skip(LIVE, "на заглушке");
    await page.setViewportSize({ width: 360, height: 780 });
    await stubTz(page);
    await page.goto(`/search?tz=${TZ_HASH}`);
    await expect(page.getByTestId("tz-title")).toContainText("найдено");
    await expect(page.getByTestId("tz-nav")).toHaveCount(0);
    await expect(page.getByTestId("tz-mobile-nav")).toBeVisible();
    await noHorizontalScroll(page);
    for (const id of ["tz-attach", "search-clear"]) {
      const b = await page.getByTestId(id).boundingBox();
      expect(b!.width, id).toBeGreaterThanOrEqual(44);
      expect(b!.height, id).toBeGreaterThanOrEqual(44);
    }
    const cta = await position(page, 1).getByTestId("compact-spec").boundingBox();
    expect(cta!.height).toBeGreaterThanOrEqual(44);
    const price = await position(page, 1).getByTestId("card-price").boundingBox();
    const title = await position(page, 1).locator(".fcard__title").boundingBox();
    expect(price!.y, "цена под названием").toBeGreaterThan(title!.y);
    await page.locator(".tzr__select").click();
    await page.getByRole("option").nth(6).click();
    await expect(page.locator(".tzr__select")).toContainText("Позиция 7 из 8");
    await noHorizontalScroll(page);
    expect(plain(await page.getByTestId("tz-title").innerText())).toContain("8 позиций");
  });
});
