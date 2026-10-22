import { test, expect, type Page } from "@playwright/test";
import { newObject } from "../src/api";

async function login(page: Page, role = "admin", expand = true) {
  const server = {
    ...newObject("server"),
    id: "s1",
    name: "host-a",
    environment: "Промышленный",
    address: "10.0.0.1",
    version: 1,
  };
  const instance = {
    ...newObject("instance", "s1"),
    id: "i1",
    name: "pg-main",
    environment: "Тестовый",
    version: 1,
  };
  const db = {
    ...newObject("database", "i1"),
    id: "d1",
    name: "payroll",
    environment: "Промышленный",
    attributes: {
      ...newObject("database").attributes,
      serviceOrganization: "Организация",
      supervisor: "Руководитель",
      responsible: "Ответственный",
    },
    version: 1,
  };
  await page.addInitScript(
    ({ server, instance, db, role }) => {
      const user = { id: "u1", login: "admin", role, state: "active" };
      const catalog = {
        servers: [server],
        instances: [instance],
        databases: [db],
      };
      let favorites: (typeof server)[] = [];
      const g = window as unknown as { go: unknown; __saved: unknown };
      g.go = {
        main: {
          App: {
            GetConnection: async () => ({
              connection: {
                host: "localhost",
                port: 5432,
                database: "catalogdb",
                username: "catalogdb",
                schema: "dbcatalog",
                sslMode: "verify-full",
                sslRootCert: "",
              },
              hasPassword: false,
            }),
            Connect: async () => ({
              connected: true,
              schema: "dbcatalog",
              needsAdmin: false,
              user: null,
            }),
            IsSetupMode: async () => false,
            Login: async () => user,
            GetWorkspace: async () => {
              const backend = window.go!.main.App;
              return {
                catalog: await backend.GetCatalog(),
                lookups: await backend.GetLookups(),
                favorites: await backend.ListFavorites(),
                policies: await backend.ListPolicies(),
              };
            },
            GetCatalog: async () => catalog,
            ListFavorites: async () => favorites,
            ListPolicies: async () => [],
            ListFilterPresets: async () => [],
            GetLookups: async () => ({
              status: ["Эксплуатируется", "Архив"],
              environment: ["Тестовый", "Промышленный"],
              dbms: [
                "PostgreSQL",
                "Oracle Database",
                "MySQL/MariaDB",
                "Firebird",
                "Microsoft SQL Server",
              ],
              domain: ["oms.admsurgut.ru", "отсутствует"],
              connectionMethod: ["RDP", "SSH"],
              location: ['МКУ "УИТС г. Сургута"'],
              serviceOrganization: ['МКУ "УИТС г. Сургута"'],
              supervisor: ["Первый руководитель"],
              responsible: ["Первый ответственный"],
              administration: [
                "Полное администрирование",
                "Мониторинг и контроль",
                "Резервное копирование",
              ],
              backupType: [
                "Локальное",
                "Централизованное",
                "Локальное и Централизованное",
              ],
            }),
            SetFavorite: async (kind: string, id: string, enabled: boolean) => {
              if (enabled) {
                const obj = [
                  ...catalog.servers,
                  ...catalog.instances,
                  ...catalog.databases,
                ].find((o) => o.id === id && o.kind === kind);
                if (obj && !favorites.some((f) => f.id === id))
                  favorites.push(obj);
              } else favorites = favorites.filter((f) => f.id !== id);
            },
            SaveObject: async (o: typeof db) => {
              g.__saved = o;
              catalog.databases = [o];
              return { ...o, version: 2 };
            },
            GetHistory: async () => [],
          },
        },
      };
    },
    { server, instance, db, role },
  );
  await page.goto("/");
  await page
    .getByRole("button", { name: "Проверить, обновить схему и подключиться" })
    .click();
  await page.getByLabel("Логин *", { exact: true }).fill("admin");
  await page
    .getByLabel("Пароль *", { exact: true })
    .fill("long-enough-password");
  await page.getByRole("button", { name: "Войти", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Общий каталог", exact: true }),
  ).toBeVisible();
  if (expand)
    await page
      .getByRole("button", { name: "Раскрыть все", exact: true })
      .click();
}
test("right-click adds child with the selected server and personal favorites can be removed", async ({
  page,
}) => {
  await login(page);
  await page
    .locator(".tree-node")
    .filter({ hasText: "host-a" })
    .click({ button: "right" });
  await page.getByRole("menuitem", { name: "Добавить экземпляр СУБД" }).click();
  await expect(
    page.getByRole("combobox", { name: "Сервер *", exact: true }),
  ).toHaveValue("s1");
  await expect(
    page
      .getByRole("combobox", { name: "Тип СУБД *", exact: true })
      .locator("option"),
  ).toHaveCount(5);
  await page.getByRole("button", { name: "Отмена", exact: true }).click();
  await page
    .locator(".tree-node")
    .filter({ hasText: "host-a" })
    .click({ button: "right" });
  await page.getByRole("menuitem", { name: "Добавить в избранное" }).click();
  await page.getByRole("button", { name: "★ Избранное", exact: true }).click();
  await expect(
    page.getByRole("cell", { name: "host-a", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Убрать из избранного", exact: true })
    .click();
  await expect(
    page.getByText("В избранном пока нет объектов.", { exact: false }),
  ).toBeVisible();
});
test("database editor sends a real array for multiple DBA responsibilities and allows new names", async ({
  page,
}) => {
  await login(page);
  await page
    .getByRole("button", { name: /payroll/ })
    .click({ button: "right" });
  await page.getByRole("menuitem", { name: "Изменить", exact: true }).click();
  await page
    .getByRole("checkbox", { name: "Мониторинг и контроль", exact: true })
    .check();
  await page
    .getByRole("checkbox", { name: "Резервное копирование", exact: true })
    .check();
  await page
    .getByLabel("Сопровождение (руководитель)", { exact: true })
    .fill("Новый руководитель");
  await page
    .getByLabel("Ответственное лицо", { exact: true })
    .fill("Новый ответственный");
  await page
    .getByRole("combobox", { name: "Состояние *", exact: true })
    .selectOption("Архив");
  await page.getByRole("button", { name: "Сохранить", exact: true }).click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  const saved = await page.evaluate(
    () =>
      (
        window as unknown as {
          __saved: {
            attributes: {
              administration: string[];
              supervisor: string;
              responsible: string;
            };
            status: string;
          };
        }
      ).__saved,
  );
  expect(saved.attributes.administration).toEqual([
    "Мониторинг и контроль",
    "Резервное копирование",
  ]);
  expect(saved.attributes.supervisor).toBe("Новый руководитель");
  expect(saved.status).toBe("Архив");
});
test("viewer context menu has personal actions but no mutation commands", async ({
  page,
}) => {
  await login(page, "viewer");
  await page
    .locator(".tree-node")
    .filter({ hasText: "pg-main" })
    .click({ button: "right" });
  await expect(
    page.getByRole("menuitem", { name: "Добавить в избранное" }),
  ).toBeVisible();
  await expect(
    page.getByRole("menuitem", { name: "Изменить", exact: true }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("menuitem", { name: "Добавить базу данных" }),
  ).toHaveCount(0);
  await expect(
    page.getByRole("menuitem", { name: "Удалить", exact: true }),
  ).toHaveCount(0);
  await page.keyboard.press("Escape");
  await expect(page.getByRole("menu")).toHaveCount(0);
});
test("instance context menu opens a policy with the requested backup type choices", async ({
  page,
}) => {
  await login(page);
  await page
    .locator(".tree-node")
    .filter({ hasText: "pg-main" })
    .click({ button: "right" });
  await page
    .getByRole("menuitem", {
      name: "Политика резервного копирования",
      exact: true,
    })
    .click();
  await expect(
    page.getByRole("combobox", { name: "Объект назначения *", exact: true }),
  ).toHaveValue("i1");
  const choices = page.getByRole("combobox", { name: "Тип РК", exact: true });
  await expect(choices.locator("option")).toHaveCount(5);
  await choices.selectOption("Локальное и Централизованное");
  await expect(choices).toHaveValue("Локальное и Централизованное");
});

test("Excel requires explicit confirmation for catalog-only import and skipping invalid rows", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    Object.assign(window.go!.main.App, {
      PreviewExcel: async () => ({
        token: "preview",
        sheet: "Новая версия",
        columns: 28,
        total: 2,
        valid: 1,
        ignored: 64235,
        secretsIgnored: 0,
        backupRows: 1,
        rows: [
          {
            row: 2,
            server: "host-a",
            instance: "pg-main",
            database: "one",
            errors: [],
          },
          {
            row: 3,
            server: "",
            instance: "default",
            database: "two",
            errors: ["Не заполнено: Имя сервера"],
          },
        ],
      }),
      ApplyExcel: async (
        _token: string,
        skip: boolean,
        catalogOnly: boolean,
      ) => {
        if (!skip || !catalogOnly) throw new Error("Missing confirmation");
        return {
          serversCreated: 1,
          instancesCreated: 1,
          databasesCreated: 1,
          objectsUpdated: 0,
          skipped: 1,
        };
      },
    });
  });
  await page
    .getByRole("button", { name: "⇄ Экспорт/Импорт", exact: true })
    .click();
  await page.getByLabel("Файл каталога (до 20 МБ)").setInputFiles({
    name: "catalog.xlsx",
    mimeType:
      "application/vnd.openxmlformats-officedocument.spreadsheetml.sheet",
    buffer: Buffer.from("bridge-contract-fixture"),
  });
  const apply = page.getByRole("button", {
    name: "Подтвердить импорт 1 строк",
    exact: true,
  });
  await expect(apply).toBeDisabled();
  await page
    .getByRole("checkbox", {
      name: "Пропустить 1 ошибочных строк, импортировать только проверенные",
    })
    .check();
  await expect(apply).toBeDisabled();
  await page
    .getByRole("checkbox", {
      name: "Подтверждаю импорт только каталога без политик РК и паролей архива",
    })
    .check();
  await expect(apply).toBeEnabled();
  await apply.click();
  await expect(
    page.getByText("Импорт завершён.", { exact: false }),
  ).toBeVisible();
});

test("pipe AND search follows one branch and card flags have distinct colors", async ({
  page,
}) => {
  await login(page);
  const search = page.getByRole("textbox", { name: "Поиск в каталоге" });
  await search.fill("host-a | payroll");
  await expect(page.getByRole("button", { name: /payroll/ })).toBeVisible();
  await expect(
    page.locator(".tree-node").filter({ hasText: "pg-main" }),
  ).toBeVisible();
  await page.getByRole("button", { name: /payroll/ }).click();
  await expect(page.locator(".object-card .value-badge.production")).toHaveText(
    "Промышленный",
  );
  await expect(page.locator(".object-card .value-badge.success")).toHaveText(
    "Эксплуатируется",
  );
  await page
    .locator(".object-card")
    .screenshot({ path: "/tmp/catalogdb-card-040.png" });
  await search.fill("host-a | missing");
  await expect(
    page.getByText("Совпадений не найдено", { exact: true }),
  ).toBeVisible();
  await search.fill("host-a | pg-main | Тестовый");
  await expect(
    page.locator(".tree-node").filter({ hasText: "pg-main" }),
  ).toBeVisible();
});
test("parent deletion displays server-calculated child counts and sends the preview fingerprint", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    Object.assign(window.go!.main.App, {
      PreviewDeletion: async () => ({
        kind: "server",
        name: "host-a",
        servers: 1,
        instances: 1,
        databases: 1,
        policies: 0,
        fingerprint: "branch-snapshot",
      }),
      DeleteObject: async (
        kind: string,
        id: string,
        version: number,
        fingerprint: string,
      ) => {
        (window as unknown as { __deletion: unknown }).__deletion = {
          kind,
          id,
          version,
          fingerprint,
        };
        const c = await window.go!.main.App.GetCatalog();
        c.servers = [];
        c.instances = [];
        c.databases = [];
        return {
          kind: "server",
          name: "host-a",
          servers: 1,
          instances: 1,
          databases: 1,
          policies: 0,
          fingerprint,
        };
      },
    });
  });
  await page
    .locator(".tree-node")
    .filter({ hasText: "host-a" })
    .click({ button: "right" });
  await page.getByRole("menuitem", { name: "Удалить", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "Подтверждение удаления",
    exact: true,
  });
  await expect(
    dialog.getByText("Экземпляры СУБД", { exact: true }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () => (window as unknown as { __deletion?: unknown }).__deletion,
    ),
  ).toBeUndefined();
  await dialog
    .getByRole("button", { name: "Удалить вместе с дочерними" })
    .click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(
    await page.evaluate(
      () =>
        (window as unknown as { __deletion: { fingerprint: string } })
          .__deletion.fingerprint,
    ),
  ).toBe("branch-snapshot");
  await expect(
    page.getByText("Каталог пока пуст. Добавьте первый сервер."),
  ).toBeVisible();
});
test("settings clearing requires two consecutive confirmations and is hidden from viewers", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    Object.assign(window.go!.main.App, {
      PreviewClearCatalog: async () => ({
        kind: "all",
        name: "Весь каталог",
        servers: 1,
        instances: 1,
        databases: 1,
        policies: 0,
        fingerprint: "all-snapshot",
      }),
      ClearCatalog: async (fingerprint: string, confirmation: string) => {
        (window as unknown as { __clear: unknown }).__clear = {
          fingerprint,
          confirmation,
        };
        const c = await window.go!.main.App.GetCatalog();
        c.servers = [];
        c.instances = [];
        c.databases = [];
        return {
          kind: "all",
          name: "Весь каталог",
          servers: 1,
          instances: 1,
          databases: 1,
          policies: 0,
          fingerprint,
        };
      },
    });
  });
  await page.getByRole("button", { name: "⚙ Настройки", exact: true }).click();
  await page
    .getByRole("button", { name: "Очистить все записи каталога" })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Очистка каталога",
    exact: true,
  });
  await expect(dialog.getByText("ПОДТВЕРЖДЕНИЕ 1 ИЗ 2")).toBeVisible();
  await dialog.getByRole("button", { name: "Продолжить", exact: true }).click();
  expect(
    await page.evaluate(
      () => (window as unknown as { __clear?: unknown }).__clear,
    ),
  ).toBeUndefined();
  const final = dialog.getByRole("button", {
    name: "Очистить каталог",
    exact: true,
  });
  await expect(final).toBeDisabled();
  await dialog.getByRole("textbox").fill("УДАЛИТЬ КАТАЛОГ");
  await final.click();
  await expect(page.getByRole("dialog")).toHaveCount(0);
  expect(
    await page.evaluate(
      () =>
        (window as unknown as { __clear: { confirmation: string } }).__clear
          .confirmation,
    ),
  ).toBe("УДАЛИТЬ КАТАЛОГ");
});
test("viewer has no clearing or audit controls in settings", async ({
  page,
}) => {
  await login(page, "viewer");
  await expect(
    page.getByRole("button", { name: "≡ Журнал аудита", exact: true }),
  ).toHaveCount(0);
  await page.getByRole("button", { name: "⚙ Настройки", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Настройки", exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Очистить все записи каталога" }),
  ).toHaveCount(0);
});
test("audit filters send all terms and show details of a deleted object", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    Object.assign(window.go!.main.App, {
      GetAudit: async (filter: unknown) => {
        (window as unknown as { __auditFilter: unknown }).__auditFilter =
          filter;
        return {
          events: [
            {
              id: "9",
              actorId: "u1",
              actor: "admin",
              action: "cascade_delete",
              actionLabel: "Удаление ветви каталога",
              entity: "server",
              objectId: "s1",
              objectName: "deleted-host",
              createdAt: "2026-10-22T08:00:00Z",
              details: { servers: 1, instances: 2, databases: 3, policies: 1 },
            },
          ],
          total: 1,
          page: 0,
          pageSize: 25,
          actors: [{ id: "u1", login: "admin" }],
          actions: { cascade_delete: "Удаление ветви каталога" },
        };
      },
    });
  });
  await page
    .getByRole("button", { name: "≡ Журнал аудита", exact: true })
    .click();
  await expect(
    page.getByRole("cell", { name: "Удаление ветви каталога", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("textbox", { name: "Поиск в аудите" })
    .fill("admin | deleted-host");
  await page.getByRole("button", { name: "Применить фильтры" }).click();
  expect(
    await page.evaluate(
      () =>
        (window as unknown as { __auditFilter: { query: string } })
          .__auditFilter.query,
    ),
  ).toBe("admin | deleted-host");
  await page.getByRole("button", { name: "Подробнее", exact: true }).click();
  await expect(
    page.locator(".audit-detail").getByText("deleted-host", { exact: true }),
  ).toBeVisible();
  await expect(
    page.locator(".audit-detail").getByText("Базы данных", { exact: true }),
  ).toBeVisible();
});

test("catalog starts collapsed and opens each branch by double click or arrow", async ({
  page,
}) => {
  await login(page, "admin", false);
  await expect(page.getByRole("button", { name: /pg-main/ })).toHaveCount(0);
  await page
    .getByRole("button", { name: /host-a/ })
    .filter({ has: page.locator(".node-icon") })
    .dblclick();
  await expect(
    page
      .getByRole("button", { name: /pg-main/ })
      .filter({ has: page.locator(".node-icon") }),
  ).toBeVisible();
  await expect(page.getByRole("button", { name: /payroll/ })).toHaveCount(0);
  await page
    .getByRole("button", { name: "Раскрыть ветку pg-main", exact: true })
    .click();
  await expect(page.getByRole("button", { name: /payroll/ })).toBeVisible();
  await page.getByRole("button", { name: "Свернуть все", exact: true }).click();
  await expect(page.getByRole("button", { name: /payroll/ })).toHaveCount(0);
  await page
    .getByRole("button", { name: "Раскрыть ветку host-a", exact: true })
    .focus();
  await page.keyboard.press("Enter");
  await expect(
    page.getByRole("button", { name: "Свернуть ветку host-a", exact: true }),
  ).toHaveAttribute("aria-expanded", "true");
});
test("audit details are a modal and restore focus after Escape", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() =>
    Object.assign(window.go!.main.App, {
      GetAudit: async () => ({
        events: [
          {
            id: "1",
            actor: "admin",
            actionLabel: "Очистка каталога",
            entity: "catalog",
            createdAt: new Date().toISOString(),
            details: { databases: 4 },
          },
        ],
        total: 1,
        page: 0,
        pageSize: 25,
        actors: [],
        actions: {},
      }),
    }),
  );
  await page
    .getByRole("button", { name: "≡ Журнал аудита", exact: true })
    .click();
  const more = page.getByRole("button", { name: "Подробнее", exact: true });
  await more.click();
  const dialog = page.getByRole("dialog", { name: "Подробности события" });
  await expect(dialog).toBeVisible();
  await expect(dialog.getByText("Базы данных", { exact: true })).toBeVisible();
  await page.keyboard.press("Escape");
  await expect(dialog).toHaveCount(0);
  await expect(more).toBeFocused();
});
test("download history opens files, reveals folders and saves a copy", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    const g = window as unknown as { __downloadActions: string[] };
    g.__downloadActions = [];
    Object.assign(window.go!.main.App, {
      ListDownloads: async () => [
        {
          id: "f1",
          name: "Каталог.xlsx",
          category: "Экспорт каталога",
          createdAt: new Date().toISOString(),
          size: 4096,
          available: true,
        },
      ],
      OpenDownload: async (id: string) =>
        g.__downloadActions.push("open:" + id),
      ShowDownload: async (id: string) =>
        g.__downloadActions.push("show:" + id),
      SaveDownloadAs: async (id: string) => {
        g.__downloadActions.push("save:" + id);
        return true;
      },
    });
  });
  await page.getByRole("button", { name: "↓ Загрузки", exact: true }).click();
  await expect(page.getByText("Каталог.xlsx", { exact: true })).toBeVisible();
  await page.getByRole("button", { name: "Открыть", exact: true }).click();
  await page.getByRole("button", { name: "В папке", exact: true }).click();
  await page
    .getByRole("button", { name: "Сохранить как…", exact: true })
    .click();
  await expect(page.getByRole("status")).toHaveText("Копия файла сохранена");
  expect(
    await page.evaluate(
      () =>
        (window as unknown as { __downloadActions: string[] })
          .__downloadActions,
    ),
  ).toEqual(["open:f1", "show:f1", "save:f1"]);
});
test("an import error can be corrected and revalidated before confirmation", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    const values = Array(21).fill("");
    values[0] = "one";
    values[1] = "pg-main";
    values[5] = "PostgreSQL";
    const preview = {
      token: "before",
      sheet: "Каталог",
      columns: 21,
      total: 1,
      valid: 0,
      ignored: 0,
      secretsIgnored: 0,
      backupRows: 0,
      rows: [
        {
          row: 2,
          server: "",
          instance: "pg-main",
          database: "one",
          errors: ["Не заполнено: Имя сервера"],
          values,
        },
      ],
    };
    Object.assign(window.go!.main.App, {
      PreviewExcel: async () => preview,
      RevalidateExcel: async (
        token: string,
        edits: { row: number; values: string[] }[],
      ) => {
        if (token !== "before" || edits[0].values[4] !== "fixed-host")
          throw new Error("Некорректные исправления");
        return {
          ...preview,
          token: "after",
          valid: 1,
          rows: [
            {
              ...preview.rows[0],
              server: "fixed-host",
              errors: [],
              values: edits[0].values,
            },
          ],
        };
      },
      ApplyExcel: async (token: string) => {
        if (token !== "after") throw new Error("Старый токен");
        return {
          serversCreated: 1,
          instancesCreated: 1,
          databasesCreated: 1,
          objectsUpdated: 0,
          skipped: 0,
        };
      },
    });
  });
  await page
    .getByRole("button", { name: "⇄ Экспорт/Импорт", exact: true })
    .click();
  await page.getByLabel("Файл каталога (до 20 МБ)").setInputFiles({
    name: "bad.xlsx",
    mimeType: "application/octet-stream",
    buffer: Buffer.from("mock"),
  });
  await page.getByRole("button", { name: "Исправить", exact: true }).click();
  const dialog = page.getByRole("dialog", { name: "Исправление строки 2" });
  await dialog.getByLabel("Имя сервера", { exact: true }).fill("fixed-host");
  await dialog
    .getByRole("button", { name: "Проверить исправления", exact: true })
    .click();
  await expect(dialog).toHaveCount(0);
  await expect(
    page.getByRole("cell", { name: "fixed-host", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Подтвердить импорт 1 строк", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText("Импорт завершён");
});
test("notification inbox marks only selected events and navigates to a collapsed object", async ({
  page,
}) => {
  await login(page, "viewer", false);
  await page.evaluate(() => {
    let read = false;
    Object.assign(window.go!.main.App, {
      GetNotifications: async () => ({
        items: [
          {
            id: "n1",
            category: "favorite",
            title: "Изменён избранный объект",
            body: "payroll",
            entity: "database",
            objectId: "d1",
            createdAt: new Date().toISOString(),
            readAt: read ? new Date().toISOString() : null,
          },
        ],
        unread: read ? 0 : 1,
        total: 1,
        page: 0,
      }),
      MarkNotificationsRead: async (ids: string[], all: boolean) => {
        if (all || ids[0] === "n1") read = true;
      },
    });
  });
  await page.getByRole("button", { name: /Уведомления/ }).click();
  await expect(
    page.getByRole("heading", {
      name: "Изменён избранный объект",
      exact: true,
    }),
  ).toBeVisible();
  await page.getByRole("button", { name: "Перейти", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Общий каталог", exact: true }),
  ).toBeVisible();
  await expect(page.locator(".tree-node.selected")).toContainText("payroll");
  await expect(page.locator(".nav-counter")).toHaveCount(0);
});
test("server connection action surfaces a missing-client error", async ({
  page,
}) => {
  await login(page, "viewer");
  await page.evaluate(() =>
    Object.assign(window.go!.main.App, {
      ConnectToServer: async (kind: string, id: string) => {
        if (kind !== "database" || id !== "d1")
          throw new Error("Неверный объект");
        throw new Error(
          "Не найден клиент ssh.exe. Установите соответствующий компонент Windows",
        );
      },
    }),
  );
  await page
    .getByRole("button", { name: /payroll/ })
    .click({ button: "right" });
  await page
    .getByRole("menuitem", { name: "Подключение к серверу", exact: true })
    .click();
  await expect(page.getByRole("alert")).toContainText(
    "Не найден клиент ssh.exe",
  );
});
test("statistics shows PostgreSQL counts and links to catalog search", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() =>
    Object.assign(window.go!.main.App, {
      GetStatistics: async () => ({
        servers: 1,
        uniqueServers: 1,
        serverAddresses: 1,
        instances: 1,
        databases: 1,
        policies: 0,
        enabledPolicies: 0,
        protectedDatabases: 0,
        unprotectedDatabases: 1,
        disabledDatabases: 0,
        dbms: [{ name: "PostgreSQL", instances: 1, databases: 1 }],
        statuses: [],
        environments: [],
        generatedAt: new Date().toISOString(),
      }),
    }),
  );
  await page.getByRole("button", { name: "▥ Статистика", exact: true }).click();
  await expect(
    page.getByRole("heading", { name: "Эффективная политика РК", exact: true }),
  ).toBeVisible();
  await page.getByRole("button", { name: /PostgreSQL/ }).click();
  await expect(
    page.getByRole("textbox", { name: "Поиск в каталоге" }),
  ).toHaveValue("PostgreSQL");
  await expect(page.getByRole("button", { name: /payroll/ })).toBeVisible();
});

test("Oracle is grouped beside regular instances and remains searchable without an instance", async ({
  page,
}) => {
  await login(page);
  const oracle = {
    ...newObject("database", "s1"),
    id: "o1",
    name: "ORCL",
    dbms: "Oracle Database",
    dbmsVersion: "19c",
    version: 1,
  };
  await page.evaluate(async (oracle) => {
    const c = await window.go!.main.App.GetCatalog();
    Object.assign(window.go!.main.App, {
      GetCatalog: async () => ({ ...c, databases: [...c.databases, oracle] }),
    });
  }, oracle);
  await page.getByRole("button", { name: "↻ Обновить", exact: true }).click();
  await expect(
    page.locator(".tree-node").filter({ hasText: "pg-main" }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", {
      name: /Oracle Database.*непосредственно на сервере/,
    }),
  ).toBeVisible();
  await expect(
    page.locator(".tree-node").filter({ hasText: "ORCL" }),
  ).toHaveCount(0);
  await page
    .getByRole("button", {
      name: "Раскрыть ветку Oracle Database",
      exact: true,
    })
    .click();
  await page.locator(".tree-node").filter({ hasText: "ORCL" }).click();
  await expect(page.locator(".object-path")).toHaveText(
    "host-a / Oracle Database / ORCL",
  );
  await expect(
    page
      .locator(".object-card .value-badge")
      .filter({ hasText: "Oracle Database" }),
  ).toBeVisible();
  await page
    .getByRole("textbox", { name: "Поиск в каталоге", exact: true })
    .fill("host-a | Oracle | ORCL");
  await expect(
    page.locator(".tree-node").filter({ hasText: "ORCL" }),
  ).toBeVisible();
  await expect(
    page.locator(".tree-node").filter({ hasText: "pg-main" }),
  ).toHaveCount(0);
  await page.screenshot({
    path: "/tmp/catalogdb-oracle-050.png",
    fullPage: true,
  });
});
test("server context menu creates Oracle directly with server and version fields", async ({
  page,
}) => {
  await login(page);
  await page
    .locator(".tree-node")
    .filter({ hasText: "host-a" })
    .click({ button: "right" });
  await page
    .getByRole("menuitem", { name: "Добавить базу Oracle", exact: true })
    .click();
  const dialog = page.getByRole("dialog", { name: "Редактирование объекта" });
  await expect(
    dialog.getByRole("combobox", { name: "Сервер *", exact: true }),
  ).toHaveValue("s1");
  await expect(
    dialog.getByRole("combobox", { name: "Экземпляр СУБД *", exact: true }),
  ).toHaveCount(0);
  await dialog
    .getByLabel("SID / имя базы данных *", { exact: true })
    .fill("ORCL");
  await dialog
    .getByLabel("Версия Oracle Database", { exact: true })
    .fill("19c");
  await dialog.getByLabel("Порт Oracle", { exact: true }).fill("1521");
  await dialog.getByRole("button", { name: "Сохранить", exact: true }).click();
  const saved = await page.evaluate(
    () =>
      (
        window as unknown as {
          __saved: { dbms: string; parentId: string; dbmsVersion: string };
        }
      ).__saved,
  );
  expect(saved.dbms).toBe("Oracle Database");
  expect(saved.parentId).toBe("s1");
  expect(saved.dbmsVersion).toBe("19c");
  expect((saved as typeof saved & { port: number }).port).toBe(1521);
});
test("branded scrollbar rules cover document, dark navigation, dialogs and textareas", async ({
  page,
}) => {
  await login(page);
  await page
    .locator(".tree-node")
    .filter({ hasText: "host-a" })
    .click({ button: "right" });
  await page.getByRole("menuitem", { name: "Изменить", exact: true }).click();
  const styles = await page.evaluate(() =>
    [
      document.documentElement,
      document.querySelector("aside nav")!,
      document.querySelector(".modal")!,
      document.querySelector("textarea")!,
    ].map((el) => ({
      width: getComputedStyle(el, "::-webkit-scrollbar").width,
      thumb: getComputedStyle(el, "::-webkit-scrollbar-thumb").backgroundColor,
      track: getComputedStyle(el, "::-webkit-scrollbar-track").backgroundColor,
    })),
  );
  expect(styles.every((s) => s.width === "9px")).toBe(true);
  expect(["rgb(138, 168, 183)", "rgb(75, 132, 153)"]).toContain(
    styles[0].thumb,
  );
  expect(["rgb(53, 85, 105)", "rgb(80, 130, 152)"]).toContain(styles[1].thumb);
  expect(styles[2].track).toBe("rgb(233, 240, 244)");
});

test("admin selects an explicit version, reviews it and returns as a new version", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    const app = window.go!.main.App,
      get = app.GetCatalog;
    let current = 3;
    Object.assign(app, {
      GetVersions: async () => ({
        items: Array.from({ length: current }, (_, i) => ({
          version: current - i,
          action: current - i === 1 ? "create" : "update",
          actor: "admin",
          createdAt: new Date().toISOString(),
        })),
        total: current,
        page: 0,
        currentVersion: current,
        exists: true,
      }),
      GetRecordVersion: async (_kind: string, _id: string, version: number) => {
        const c = await get();
        return {
          ...c.databases[0],
          name: version === 1 || version === 4 ? "old-payroll" : "payroll",
          version,
        };
      },
      RestoreVersion: async (
        kind: string,
        id: string,
        from: number,
        to: number,
      ) => {
        if (kind !== "database" || id !== "d1" || from !== 3 || to !== 1)
          throw new Error("Неверное подтверждение");
        current = 4;
      },
      GetCatalog: async () => {
        const c = await get();
        return {
          ...c,
          databases: c.databases.map((d) => ({
            ...d,
            name: current === 4 ? "old-payroll" : d.name,
            version: current,
          })),
        };
      },
    });
  });
  await page.locator(".tree-node").filter({ hasText: "payroll" }).click();
  await page
    .getByRole("button", { name: "Версии записи", exact: true })
    .click();
  const dialog = page.locator(".versions-dialog");
  await expect(dialog).toBeVisible();
  await dialog
    .getByRole("button", { name: "Просмотреть v1", exact: true })
    .click();
  await expect(
    dialog.getByRole("cell", { name: "old-payroll", exact: true }),
  ).toBeVisible();
  await dialog
    .getByRole("button", { name: "Вернуться к v1", exact: true })
    .click();
  await expect(
    dialog.getByRole("button", {
      name: "Подтвердить возврат к v1",
      exact: true,
    }),
  ).toBeVisible();
  await dialog
    .getByRole("button", { name: "Подтвердить возврат к v1", exact: true })
    .click();
  await expect(dialog.getByRole("status")).toContainText(
    "Создана новая версия",
  );
  await expect(
    dialog.getByText("Текущая версия: v4", { exact: true }),
  ).toBeVisible();
  await dialog.screenshot({ path: "/tmp/catalogdb-versions-060.png" });
  await dialog
    .getByRole("button", { name: "Закрыть окно", exact: true })
    .click();
  await expect(page.locator(".object-card h2")).toHaveText("old-payroll");
});
test("import rows can be excluded, returned and bulk excluded before applying", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    const rows = [
      {
        row: 2,
        server: "host-a",
        instance: "pg-main",
        database: "valid",
        errors: [],
        values: Array(21).fill(""),
      },
      {
        row: 3,
        server: "",
        instance: "default",
        database: "",
        errors: ["Не заполнено: SID"],
        values: Array(21).fill(""),
      },
      {
        row: 4,
        server: "",
        instance: "default",
        database: "bad",
        errors: ["Не заполнено: Имя сервера"],
        values: Array(21).fill(""),
      },
    ];
    const excluded = new Set<number>();
    let revision = 0;
    const preview = () => ({
      token: `r${revision}`,
      sheet: "Каталог",
      columns: 21,
      originalTotal: 3,
      total: 3 - excluded.size,
      valid: rows.filter((r) => !excluded.has(r.row) && !r.errors.length)
        .length,
      excluded: excluded.size,
      excludedRows: rows.filter((r) => excluded.has(r.row)),
      ignored: 0,
      secretsIgnored: 0,
      backupRows: 0,
      rows: rows.filter((r) => !excluded.has(r.row)),
    });
    Object.assign(window.go!.main.App, {
      PreviewExcel: async () => preview(),
      SetImportRowsExcluded: async (
        token: string,
        ids: number[],
        exclude: boolean,
      ) => {
        if (token !== `r${revision}`) throw new Error("Старый токен");
        for (const id of ids) {
          if (exclude) excluded.add(id);
          else excluded.delete(id);
        }
        revision++;
        return preview();
      },
      ApplyExcel: async (token: string) => {
        if (token !== `r${revision}` || excluded.size !== 2)
          throw new Error("Неверный состав импорта");
        return {
          serversCreated: 1,
          instancesCreated: 1,
          databasesCreated: 1,
          objectsUpdated: 0,
          skipped: 0,
          excluded: 2,
        };
      },
    });
  });
  await page
    .getByRole("button", { name: "⇄ Экспорт/Импорт", exact: true })
    .click();
  await page.getByLabel("Файл каталога (до 20 МБ)").setInputFiles({
    name: "rows.xlsx",
    mimeType: "application/octet-stream",
    buffer: Buffer.from("mock"),
  });
  await page
    .getByRole("button", { name: "Исключить строку 3", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Исключить строку 3", exact: true }),
  ).toHaveCount(0);
  await page
    .getByRole("button", {
      name: "Исключить все ошибочные строки (1)",
      exact: true,
    })
    .click();
  await expect(
    page.getByRole("button", {
      name: "Подтвердить импорт 1 строк",
      exact: true,
    }),
  ).toBeEnabled();
  await page.getByText("Исключённые строки (2)", { exact: true }).click();
  await page
    .getByRole("button", { name: "Вернуть строку 3", exact: true })
    .click();
  await expect(
    page.getByRole("button", {
      name: "Подтвердить импорт 1 строк",
      exact: true,
    }),
  ).toBeDisabled();
  await page
    .getByRole("button", { name: "Исключить строку 3", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Подтвердить импорт 1 строк", exact: true })
    .click();
  await expect(page.getByRole("status")).toContainText(
    "Исключено перед импортом: 2",
  );
});
for (const role of ["editor", "viewer"]) {
  test(`${role} can open history in a window but cannot access version controls`, async ({
    page,
  }) => {
    await login(page, role);
    await page.evaluate(() =>
      Object.assign(window.go!.main.App, {
        GetHistoryPage: async () => ({
          items: [
            {
              id: 1,
              action: "update",
              actor: "admin",
              createdAt: new Date().toISOString(),
              before: {
                id: "d1",
                kind: "database",
                name: "before",
                version: 1,
                attributes: { administration: [] },
              },
              after: {
                id: "d1",
                kind: "database",
                name: "payroll",
                version: 2,
                attributes: { administration: [] },
              },
            },
          ],
          total: 1,
          page: 0,
        }),
      }),
    );
    await page.locator(".tree-node").filter({ hasText: "payroll" }).click();
    await expect(
      page.getByRole("button", { name: "Версии записи", exact: true }),
    ).toHaveCount(0);
    await page
      .getByRole("button", { name: "История изменений", exact: true })
      .click();
    const dialog = page.getByRole("dialog", {
      name: "История изменений: payroll",
      exact: true,
    });
    await expect(dialog).toBeVisible();
    await expect(
      dialog.getByRole("columnheader", { name: "Было", exact: true }),
    ).toBeVisible();
    await expect(
      dialog.getByRole("cell", { name: "before", exact: true }),
    ).toBeVisible();
    await expect(
      dialog.getByRole("button", { name: "Версии записи", exact: true }),
    ).toHaveCount(0);
    await page.keyboard.press("Escape");
    await expect(dialog).toHaveCount(0);
    await page
      .locator(".tree-node")
      .filter({ hasText: "payroll" })
      .click({ button: "right" });
    await expect(
      page.getByRole("menuitem", { name: "Версии записи", exact: true }),
    ).toHaveCount(0);
  });
}
test("policy history is paginated in a separate window and has administrator version access", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    const policy = {
      id: "p1",
      kind: "database",
      objectId: "d1",
      name: "Daily",
      enabled: false,
      backupType: "Локальное",
      scheduler: "cron",
      schedule: "daily",
      storage: "/backup",
      retentionDays: 7,
      description: "",
      version: 26,
    };
    Object.assign(window.go!.main.App, {
      ListPolicies: async () => [policy],
      GetHistoryPage: async (kind: string, id: string, page: number) => {
        if (kind !== "policy" || id !== "p1")
          throw new Error("Неверная запись");
        return {
          items: Array.from({ length: page ? 1 : 25 }, (_, i) => ({
            id: 26 - page * 25 - i,
            action: "update",
            actor: "admin",
            createdAt: new Date().toISOString(),
            before: {
              ...policy,
              retentionDays: 1,
              version: 25 - page * 25 - i,
            },
            after: { ...policy, version: 26 - page * 25 - i },
          })),
          total: 26,
          page,
        };
      },
    });
  });
  await page
    .getByRole("button", { name: "◷ Политики РК", exact: true })
    .click();
  await page.getByRole("button", { name: "История", exact: true }).click();
  const dialog = page.getByRole("dialog", {
    name: "История изменений: Daily",
    exact: true,
  });
  await expect(dialog).toBeVisible();
  await expect(
    dialog.getByRole("button", { name: "Версии записи", exact: true }),
  ).toBeVisible();
  await dialog.getByRole("button", { name: "→", exact: true }).click();
  await expect(
    dialog.getByText("Событий: 26 · Страница 2 из 2", { exact: true }),
  ).toBeVisible();
});

test("bulk edit requires a fresh preview after every field change and displays server errors in the dialog", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    const backend = window.go!.main.App;
    Object.assign(backend, {
      PreviewBulkEdit: async (
        refs: { id: string; version: number }[],
        patch: { responsible: string },
      ) => {
        if (refs.length !== 1 || refs[0].id !== "d1" || refs[0].version !== 1)
          throw new Error("Неверный выбор");
        const before = (await backend.GetCatalog()).databases[0];
        return {
          fingerprint: "confirmed",
          selected: 1,
          changed: 1,
          items: [
            {
              before,
              after: {
                ...before,
                attributes: {
                  ...before.attributes,
                  responsible: patch.responsible,
                },
              },
            },
          ],
        };
      },
      ApplyBulkEdit: async () => {
        throw new Error("Выбранная БД изменена другим пользователем");
      },
    });
  });
  await page
    .getByRole("checkbox", { name: "Выбрать БД payroll", exact: true })
    .check();
  await page
    .getByRole("button", { name: "Массовое редактирование", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Массовое редактирование: 1 БД",
  });
  await dialog
    .getByRole("checkbox", { name: "Ответственный", exact: true })
    .check();
  await dialog.getByLabel("Новый ответственный", { exact: true }).fill("Новый");
  await expect(
    dialog.getByRole("button", { name: "Подтвердить изменения", exact: true }),
  ).toHaveCount(0);
  await dialog
    .getByRole("button", { name: "Предпросмотр изменений", exact: true })
    .click();
  await expect(
    dialog.getByRole("cell", { name: "Новый", exact: true }),
  ).toBeVisible();
  await dialog
    .getByLabel("Новый ответственный", { exact: true })
    .fill("Другой");
  await expect(
    dialog.getByRole("button", { name: "Подтвердить изменения", exact: true }),
  ).toHaveCount(0);
  await dialog
    .getByRole("button", { name: "Предпросмотр изменений", exact: true })
    .click();
  await dialog
    .getByRole("button", { name: "Подтвердить изменения", exact: true })
    .click();
  await expect(dialog.getByRole("alert")).toHaveText(
    "Выбранная БД изменена другим пользователем",
  );
});

test("catalog filters save personal presets and export the filtered result as a viewer", async ({
  page,
}) => {
  await login(page, "viewer");
  await page.evaluate(() => {
    let presets: unknown[] = [];
    Object.assign(window.go!.main.App, {
      SaveFilterPreset: async (f: unknown) => {
        presets = [f];
        return f;
      },
      ListFilterPresets: async () => presets,
      ExportSelection: async (ids: string[]) => {
        if (ids.join() !== "d1") throw new Error("Выбраны лишние БД");
        return { id: "file", name: "result.xlsx" };
      },
      SaveDownloadAs: async () => {},
    });
  });
  await page.getByText("Фильтры баз данных", { exact: true }).click();
  await page
    .getByRole("combobox", { name: "Тип эксплуатации", exact: true })
    .selectOption("Тестовый");
  await expect(page.locator(".tree-node")).toHaveCount(0);
  await page
    .getByRole("combobox", { name: "Тип эксплуатации", exact: true })
    .selectOption("Промышленный");
  await page
    .getByRole("combobox", {
      name: "Политика РК (с учётом наследования)",
      exact: true,
    })
    .selectOption("missing");
  await page.getByLabel("Название набора фильтров").fill("Без РК");
  await page
    .getByRole("button", { name: "Сохранить набор", exact: true })
    .click();
  await expect(
    page.getByRole("button", { name: "Без РК", exact: true }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Сбросить фильтры и поиск", exact: true })
    .click();
  await page.getByRole("button", { name: "Без РК", exact: true }).click();
  await expect(
    page.getByRole("combobox", {
      name: "Политика РК (с учётом наследования)",
      exact: true,
    }),
  ).toHaveValue("missing");
  await page
    .getByRole("button", { name: "Экспорт результата", exact: true })
    .click();
  await expect(
    page.getByText("Результат сохранён в Загрузки", { exact: true }),
  ).toBeVisible();
  await expect(
    page.getByRole("button", { name: "Массовое редактирование", exact: true }),
  ).toHaveCount(0);
});

test("quality report opens the affected record and deleted branch conflicts prevent restoration", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    const object = {
      id: "s1",
      kind: "server",
      name: "host-a",
      batchId: "b",
      deletedAt: "2026-10-22T10:00:00Z",
      actor: "admin",
      snapshot: {},
    };
    Object.assign(window.go!.main.App, {
      GetQualityReport: async () => ({
        page: 0,
        total: 1,
        counts: { missing_address: 1 },
        items: [
          {
            code: "missing_address",
            severity: "warning",
            kind: "server",
            id: "s1",
            name: "host-a",
            path: "host-a",
            detail: "Не указан сетевой адрес сервера",
          },
        ],
      }),
      ListDeleted: async () => ({ page: 0, total: 1, items: [object] }),
      PreviewDeletedRestore: async () => ({
        fingerprint: "preview",
        items: [object],
        conflicts: ["Имя сервера уже занято"],
      }),
      RestoreDeleted: async () => {
        throw new Error("Восстановление не должно вызываться");
      },
    });
  });
  await page
    .getByRole("button", { name: "◇ Качество данных", exact: true })
    .click();
  await page
    .getByRole("button", { name: "Открыть карточку", exact: true })
    .click();
  await expect(page.locator(".object-card")).toContainText("host-a");
  await page.getByRole("button", { name: "♲ Удалённые", exact: true }).click();
  await page
    .getByRole("button", { name: "Восстановить…", exact: true })
    .click();
  const dialog = page.getByRole("dialog", {
    name: "Восстановление: host-a",
    exact: true,
  });
  await dialog
    .getByRole("button", { name: "Проверить восстановление", exact: true })
    .click();
  await expect(dialog).toContainText("Имя сервера уже занято");
  await expect(
    dialog.getByRole("button", {
      name: "Подтвердить восстановление",
      exact: true,
    }),
  ).toBeDisabled();
  await dialog
    .getByRole("checkbox", {
      name: "Восстановить всю удалённую ветвь",
      exact: true,
    })
    .uncheck();
  await expect(
    dialog.getByRole("button", {
      name: "Подтвердить восстановление",
      exact: true,
    }),
  ).toHaveCount(0);
});

test("policy import requires explicit missing parameters and a separate confirmation that resets on edits", async ({
  page,
}) => {
  await login(page);
  await page.evaluate(() => {
    let revision = 0;
    const policy = {
      id: "",
      version: 0,
      kind: "database",
      objectId: "pending",
      name: "РК",
      enabled: true,
      backupType: "Локальное",
      scheduler: "cron",
      schedule: "Ежедневно 02:00",
      storage: "/backup",
      retentionDays: 7,
      description: "",
    };
    const preview = (withPolicies = false) => ({
      token: "token-" + revision,
      sheet: "Каталог",
      columns: 28,
      originalTotal: 1,
      total: 1,
      valid: 1,
      excluded: 0,
      excludedRows: [],
      ignored: 0,
      secretsIgnored: 0,
      backupRows: 1,
      withPolicies,
      policies: withPolicies
        ? [
            {
              row: 2,
              target: "host-a / pg-main / payroll",
              action: "создать",
              policy,
              before: null,
              errors: [],
              skipped: false,
            },
          ]
        : [],
      rows: [
        {
          row: 2,
          server: "host-a",
          instance: "pg-main",
          database: "payroll",
          values: Array(21).fill(""),
          errors: [],
        },
      ],
    });
    Object.assign(window.go!.main.App, {
      PreviewExcel: async () => preview(),
      PreviewExcelPolicies: async (
        token: string,
        c: {
          enabled: boolean;
          targetKind: string;
          retentionUnit: string;
          defaults: { schedule: string };
        },
      ) => {
        if (token !== "token-" + revision) throw new Error("Старый токен");
        if (
          c.enabled &&
          (c.targetKind !== "database" ||
            c.retentionUnit !== "days" ||
            !c.defaults.schedule)
        )
          throw new Error("Не указаны параметры");
        revision++;
        return preview(c.enabled);
      },
      ApplyExcel: async (
        token: string,
        skipInvalid: boolean,
        catalogOnly: boolean,
      ) => {
        if (catalogOnly || token !== "token-" + revision)
          throw new Error("Неверное подтверждение");
        return {
          serversCreated: 1,
          instancesCreated: 1,
          databasesCreated: 1,
          objectsUpdated: 0,
          policiesCreated: 1,
          policiesUpdated: 0,
          skipped: 0,
          excluded: 0,
        };
      },
    });
  });
  await page
    .getByRole("button", { name: "⇄ Экспорт/Импорт", exact: true })
    .click();
  await page.getByLabel("Файл каталога (до 20 МБ)").setInputFiles({
    name: "policies.xlsx",
    mimeType: "application/octet-stream",
    buffer: Buffer.from("mock"),
  });
  await page
    .getByRole("checkbox", {
      name: "Переносить политики РК вместе с каталогом",
      exact: true,
    })
    .check();
  await page
    .getByLabel("Уровень назначения", { exact: true })
    .selectOption("database");
  await page
    .getByLabel("Единицы числового срока из Excel", { exact: true })
    .selectOption("days");
  await page
    .getByLabel("Расписание, если отсутствует", { exact: true })
    .fill("Ежедневно 02:00");
  const confirm = page.getByRole("button", {
    name: "Подтвердить импорт 1 строк",
    exact: true,
  });
  await expect(confirm).toBeDisabled();
  await page
    .getByRole("button", { name: "Проверить назначения политик", exact: true })
    .click();
  await expect(confirm).toBeDisabled();
  await page
    .getByRole("checkbox", {
      name: "Подтверждаю назначения и замену существующих политик из предпросмотра",
      exact: true,
    })
    .check();
  await expect(confirm).toBeEnabled();
  await page
    .getByLabel("Расписание, если отсутствует", { exact: true })
    .fill("Ежедневно 03:00");
  await expect(confirm).toBeDisabled();
  await page
    .getByRole("button", { name: "Проверить назначения политик", exact: true })
    .click();
  await page
    .getByRole("checkbox", {
      name: "Подтверждаю назначения и замену существующих политик из предпросмотра",
      exact: true,
    })
    .check();
  await confirm.click();
  await expect(page.getByRole("status")).toContainText("Политик создано: 1");
});

test("opening a favorite clears a filter that hides its branch", async ({
  page,
}) => {
  await login(page);
  await page
    .locator(".tree-node")
    .filter({ hasText: "payroll" })
    .click({ button: "right" });
  await page
    .getByRole("menuitem", { name: "Добавить в избранное", exact: true })
    .click();
  await page.getByText("Фильтры баз данных", { exact: true }).click();
  await page
    .getByRole("combobox", { name: "Тип эксплуатации", exact: true })
    .selectOption("Тестовый");
  await expect(page.locator(".tree-node")).toHaveCount(0);
  await page.getByRole("button", { name: "★ Избранное", exact: true }).click();
  await page.getByRole("button", { name: "payroll", exact: true }).click();
  await expect(page.locator(".tree-node.selected")).toContainText("payroll");
  await page.getByText("Фильтры баз данных", { exact: true }).click();
  await expect(
    page.getByRole("combobox", { name: "Тип эксплуатации", exact: true }),
  ).toHaveValue("");
});

test("notification navigation uses the refreshed catalog to reveal a new server on the second page", async ({
  page,
}) => {
  await login(page, "viewer", false);
  await page.evaluate(() => {
    const api = window.go!.main.App;
    const original = api.GetCatalog;
    Object.assign(api, {
      GetCatalog: async () => {
        const initial = await original();
        const template = initial.servers[0];
        const servers = Array.from({ length: 12 }, (_, i) => ({
          ...template,
          id: "new-s" + i,
          name: "server-" + i,
        }));
        return {
          servers,
          instances: [],
          databases: [
            {
              ...initial.databases[0],
              id: "new-db",
              name: "fresh-oracle",
              dbms: "Oracle Database",
              parentId: "new-s11",
            },
          ],
        };
      },
      GetNotifications: async () => ({
        page: 0,
        total: 1,
        unread: 1,
        items: [
          {
            id: "n-new",
            category: "catalog",
            title: "Новая запись",
            body: "Открыть",
            entity: "database",
            objectId: "new-db",
            createdAt: "2026-10-22T10:00:00Z",
            readAt: null,
          },
        ],
      }),
      MarkNotificationsRead: async () => {},
    });
  });
  await page.getByRole("button", { name: /Уведомления/ }).click();
  await page.getByRole("button", { name: "Перейти", exact: true }).click();
  await expect(page.locator(".tree-node.selected")).toContainText(
    "fresh-oracle",
  );
  await expect(page.locator(".pagination")).toContainText("Страница 2 из 2");
});

test("workspace refresh updates the selected card and invalidates selected database versions", async ({
  page,
}) => {
  await login(page);
  await page.locator(".tree-node").filter({ hasText: "payroll" }).click();
  await page
    .getByRole("checkbox", { name: "Выбрать БД payroll", exact: true })
    .check();
  await page.evaluate(() => {
    const original = window.go!.main.App.GetCatalog;
    Object.assign(window.go!.main.App, {
      GetCatalog: async () => {
        const c = await original();
        return {
          ...c,
          databases: c.databases.map((d) => ({
            ...d,
            name: "payroll-updated",
            version: 2,
          })),
        };
      },
    });
  });
  await page.getByRole("button", { name: "↻ Обновить", exact: true }).click();
  await expect(page.locator(".object-card h2")).toHaveText("payroll-updated");
  await expect(
    page.getByRole("checkbox", {
      name: "Выбрать БД payroll-updated",
      exact: true,
    }),
  ).not.toBeChecked();
  await expect(
    page.getByRole("button", { name: "Массовое редактирование", exact: true }),
  ).toBeDisabled();
});
