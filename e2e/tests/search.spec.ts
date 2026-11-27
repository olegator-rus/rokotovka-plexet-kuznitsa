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
} from "./fixtures";

test.describe("Поиск", () => {
  test("F1: подсказки после двух символов, выбор подсказки ведёт к точному товару", async ({
    page,
  }) => {
    await openSearch(page);
    await searchBox(page).pressSequentially("win", { delay: 60 });

    const list = page.getByRole("listbox");
    await expect(list).toBeVisible();
    // Сервер контура коммерции — группа «Запросы», панель контура выдачи (§11.2) — «Подсказки».
    await expect(list.getByText(/^(Запросы|Подсказки)$/).first()).toBeVisible();
    const options = list.getByRole("option");
    await expect(options.first()).toBeVisible();
    // Панель контура выдачи (§11.2): ≤5 подсказок, ≤2 категории, ≤2 производителя, ≤6 семейств товаров;
    // сервер контура коммерции — не больше 8 строк в группах.
    const sugg = list
      .getByRole("group", { name: "Подсказки" })
      .getByRole("option");
    if (await sugg.count()) {
      expect(await sugg.count()).toBeLessThanOrEqual(5);
      expect(await options.count()).toBeLessThanOrEqual(5 + 2 + 2 + 6);
    } else {
      expect(await options.count()).toBeLessThanOrEqual(8);
    }

    // Фид store.softline.ru: товар из панели «Товары» — поиск по его названию, он же первым.
    const product = list
      .getByRole("option", { name: /Windows 11 Pro/ })
      .first();
    await expect(product).toBeVisible();
    const body = await searchResponse(page, () => product.click());
    expect(body.total).toBeGreaterThan(0);
    // Контур коммерции — «Найдено N», страница результатов контура выдачи — «По запросу … — N продуктов · M вариантов».
    await expect(
      page
        .getByRole("status")
        .filter({ hasText: /Найдено|\d+ продукт/ })
        .first(),
    ).toBeVisible();
    // Карточка «цели» скрыта по решению владельца — сама позиция стоит среди точных.
    await expect(
      page
        .locator('[data-match-group="exact"]')
        .getByRole("article")
        .filter({ hasText: "Windows 11 Pro" })
        .first(),
    ).toBeVisible();
  });

  test("F2: опечатка исправляется с плашкой, «Искать как введено» ищет исходный текст", async ({
    page,
  }) => {
    await openSearch(page);
    const fixed = await search(page, "касперкий");
    expect(fixed.correction?.applied).toBe(true);

    const bar = understanding(page);
    await expect(bar).toContainText("Показаны результаты по запросу");
    await expect(bar.getByText("касперский", { exact: true })).toBeVisible();
    await expect(resultCards(page).first()).toContainText("Kaspersky");

    const asTyped = await searchResponse(page, () =>
      bar.getByRole("button", { name: "касперкий", exact: true }).click(),
    );
    expect(asTyped.query).toBe("касперкий");
    await expect(page).toHaveURL(/as_typed=1/);
    await expect(bar).toContainText("как введено, без исправления");
  });

  test("F2: раскладка и сленг — «rfcgthcrbq» → касперский, «винда 11 про» без плашки", async ({
    page,
  }) => {
    await openSearch(page);
    const layout = await search(page, "rfcgthcrbq");
    expect(layout.total).toBeGreaterThan(0);
    await expect(
      understanding(page).getByText("касперский", { exact: true }),
    ).toBeVisible();
    await expect(resultCards(page).first()).toContainText("Kaspersky");

    const slang = await search(page, "винда 11 про");
    expect(slang.correction?.applied ?? false).toBe(false);
    await expect(page.getByText("Показаны результаты по запросу")).toHaveCount(
      0,
    );
    await expect(resultCards(page).first()).toContainText("Windows 11 Pro");
  });

  test("F3: «аналог фотошопа на 20 пользователей» → чипы, снятие и возврат чипа; аналоги — графические редакторы других вендоров, без «Популярного»", async ({
    page,
  }) => {
    await openSearch(page);
    const body = await search(page, "аналог фотошопа на 20 пользователей");
    expect(body.trace.route).toBe("D");

    const bar = understanding(page);
    await expect(bar).toContainText("Мы поняли запрос так");
    await expect(
      bar.getByRole("button", {
        name: "Снять условие «Аналог Adobe Photoshop»",
      }),
    ).toBeVisible();
    await expect(
      bar.getByRole("button", { name: "Снять условие «20 пользователей»" }),
    ).toBeVisible();

    // Связей аналогов на фиде нет: аналоги — тот же класс у других вендоров, «Популярное» не нужно.
    expect(body.target?.title).toBe("Графические редакторы");
    expect(body.items.length).toBeGreaterThanOrEqual(4);
    expect(
      body.items.every(
        (i: { vendor?: string }) => !/adobe/i.test(i.vendor ?? ""),
      ),
    ).toBe(true);
    await expect(page.locator('[data-match-group="fallback"]')).toHaveCount(0);

    await searchResponse(page, () =>
      bar
        .getByRole("button", { name: "Снять условие «20 пользователей»" })
        .click(),
    );
    await expect(page).toHaveURL(/rm=license_quantity/);
    await expect(bar).toContainText("Снято условий: 1");
    await expect(resultCards(page).first()).toBeVisible();

    await searchResponse(page, () =>
      bar.getByRole("button", { name: "вернуть" }).click(),
    );
    await expect(bar).not.toContainText("Снято условий");
    await expect(
      bar.getByRole("button", { name: "Снять условие «20 пользователей»" }),
    ).toBeVisible();
  });

  // Маршрут D: снятые чипы применяются к итогу разбора LLM, в ключ кэша не входят — пересчёт из кэша за 0 ₽.
  test("F3: снятый чип исчезает из строки понимания, пересчёт без вызова LLM", async ({
    page,
  }) => {
    await openSearch(page);
    await search(page, "аналог фотошопа на 20 пользователей");
    const bar = understanding(page);
    const after = await searchResponse(page, () =>
      bar
        .getByRole("button", { name: "Снять условие «20 пользователей»" })
        .click(),
    );
    expect(after.chips.some((c) => c.field === "license_quantity")).toBe(false);
    expect(after.trace.parse_source, "снятие чипа не вызывает LLM").not.toBe(
      "llm",
    );
    await expect(
      bar.getByRole("button", { name: "Снять условие «20 пользователей»" }),
    ).toHaveCount(0);
  });

  // «notion» на фиде находит посторонние позиции по словам описания — берём товар, которого в фиде нет.
  test("F4: «1password» → честное «нет в каталоге» и «Спросить менеджера»", async ({
    page,
  }) => {
    await openSearch(page);
    const body = await search(page, "1password");
    expect(body.total).toBe(0);

    await expect(
      page.getByRole("heading", { name: "Товара «1password» нет в каталоге" }),
    ).toBeVisible();
    await page
      .getByRole("button", { name: "Спросить менеджера" })
      .first()
      .click();
    const dialog = page.getByRole("dialog", { name: "Спросить менеджера" });
    await expect(dialog).toContainText("Запрос передан");
    await dialog.getByRole("button", { name: "Понятно" }).click();
    await expect(dialog).toBeHidden();
  });

  // Переключатель «Глазами клиента» убран из шапки: сотрудник смотрит витрину как гость.
  // Своя выдача клиента осталась у покупателя — её проверяет roles.spec.ts («покупатель client-a»).
  test("F5: у сотрудника нет выбора клиента — выдача гостевая, client в адресе не держится", async ({
    page,
  }) => {
    await openSearch(page, "?client=client-a");
    await expect(page.getByRole("combobox", { name: "Клиент" })).toHaveCount(0);
    await expect(page).not.toHaveURL(/client=/);
    const body = await search(page, "продлить антивирус");
    expect(body.items.some((i) => i.purchased_before)).toBe(false);
    await expect(page.getByText("Вы покупали этот продукт")).toHaveCount(0);
  });

  test("F8: повтор сложного запроса — разбор из кэша, 0 ₽", async ({
    page,
  }) => {
    const q = "аналог фотошопа на 20 пользователей";
    await openSearch(page, "?trace=1");
    await search(page, q);
    // Повтор тем же адресом: страница заново шлёт тот же запрос.
    const again = await searchResponse(page, () => page.reload());
    expect(again.trace.parse_source).toBe("cache");
    expect(again.trace.cost_rub).toBe(0);

    const panel = tracePanel(page);
    // Подпись источника разбора — только по-русски (QA r2 №15).
    await expect(
      panel.getByRole("definition").filter({ hasText: /^\s*кэш(\s|$)/ }),
    ).toBeVisible();
    await expect(
      panel.getByRole("definition").filter({ hasText: /^0\s₽$/ }),
    ).toBeVisible();
  });
});
