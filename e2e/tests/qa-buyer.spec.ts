// QA-обход покупателя: находки по витрине, поиску, странице товара и подбору — проверки на живом API.
// Подбор (контур помощника) — в p10-assistant.spec.ts: каждая карточка помощника со своим действием.
import type { Page } from "@playwright/test";
import {
  GUEST,
  expect,
  openSearch,
  resultCards,
  search,
  test,
} from "./fixtures";

const exactCards = (page: Page) =>
  page.locator('[data-match-group="exact"]').getByRole("article");

test.describe("QA покупателя: поиск", () => {
  test.use({ storageState: GUEST });

  test("«касперский на 10 мест» — цель не обучающая платформа ASAP, первой — Endpoint Security", async ({
    page,
  }) => {
    await openSearch(page);
    const body = (await search(page, "касперский на 10 мест")) as unknown as {
      target: { type: string; title: string } | null;
    };
    expect(body.target?.title ?? "").not.toMatch(/Awareness Platform/);
    await expect(resultCards(page).first()).toContainText("Endpoint Security");
  });

  test("«касперский» — линейка вендора, а не три семейства; голова — лицензия, не носитель", async ({
    page,
  }) => {
    await openSearch(page);
    const body = (await search(page, "касперский")) as unknown as {
      items: { family_id: string; title: string }[];
    };
    expect(
      new Set(body.items.map((i) => i.family_id)).size,
    ).toBeGreaterThanOrEqual(10);
    await expect(resultCards(page).first()).toContainText("Endpoint Security");
    expect(body.items[0]?.title).not.toMatch(/media pack/i);
  });

  test("«drweb» — вендор Доктор Веб, без «нет в каталоге» и без категории «Вебкамеры»", async ({
    page,
  }) => {
    await openSearch(page);
    const body = await search(page, "drweb");
    expect(body.chips.map((c) => c.label)).not.toContain("Вебкамеры");
    expect(body.empty ?? null).toBeNull();
    await expect(
      page.getByRole("heading", { name: /нет в каталоге/ }),
    ).toHaveCount(0);
    await expect(resultCards(page).first()).toContainText("Доктор Веб");
  });

  test("«крипто про csp 5.0» — первой версия 5.0; номер ЕРРП и «5.0» не становятся количеством", async ({
    page,
  }) => {
    await openSearch(page);
    await search(page, "крипто про csp 5.0");
    const first = exactCards(page).first();
    await expect(first).toContainText("5.0");
    await expect(first).not.toContainText("версии 4.0");
    await expect(page.locator("main")).not.toContainText("Количество 515");
    await expect(page.locator("main")).not.toContainText("Количество 4 332");
    await expect(page.locator("main")).not.toContainText("0 серверов");
  });

  test("«windows server» — «для государственных организаций» не лицензия на организацию", async ({
    page,
  }) => {
    await openSearch(page);
    await search(page, "windows server");
    await expect(exactCards(page).first()).toContainText(
      "Windows Server Standard",
    );
    await expect(exactCards(page).first()).not.toContainText("На организацию");
  });

  test("«asdfgh» — «ничего не нашли», а не вопрос к менеджеру", async ({
    page,
  }) => {
    await openSearch(page);
    await search(page, "asdfgh");
    await expect(
      page.getByRole("heading", { name: /«asdfgh» ничего не нашли/ }),
    ).toBeVisible();
    await expect(
      page.getByRole("heading", { name: /с этим вопросом поможет менеджер/ }),
    ).toHaveCount(0);
  });
});

test.describe("QA покупателя: страница товара", () => {
  test.use({ storageState: GUEST });

  test("«На сайте ↗» ведёт в магазин, у цены со ступенью — «за узел», варианты различимы", async ({
    page,
  }) => {
    await openSearch(page);
    await search(page, "продлить касперский на 100 пк на 2 года");
    await page.getByTestId("product-link").first().click();
    await expect(page.getByRole("heading", { level: 1 })).toBeVisible();
    const site = page.getByTestId("product-site");
    await expect(site).toHaveAttribute(
      "href",
      /^https:\/\/store\.softline\.ru\//,
    );
    await expect(site).toHaveAttribute("target", "_blank");
    await expect(page.getByTestId("product-per-place")).toHaveText("за узел");
    // Коммерческий и академический варианты с одинаковыми условиями различаются подписью типа организации.
    await expect(page.locator("main")).toContainText(
      "для: образовательное учреждение",
    );
  });
});
