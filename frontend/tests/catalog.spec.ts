import { test, expect, type Page } from "@playwright/test";
import { newObject } from "../src/api";

async function login(page: Page, role = "admin") {
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
            GetCatalog: async () => catalog,
            ListFavorites: async () => favorites,
            ListPolicies: async () => [],
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
}
test("right-click adds child with the selected server and personal favorites can be removed", async ({
  page,
}) => {
  await login(page);
  await page.getByRole("button", { name: /host-a/ }).click({ button: "right" });
  await page.getByRole("menuitem", { name: "Добавить экземпляр СУБД" }).click();
  await expect(
    page.getByRole("combobox", { name: "Сервер *", exact: true }),
  ).toHaveValue("s1");
  await expect(
    page
      .getByRole("combobox", { name: "Тип СУБД *", exact: true })
      .locator("option"),
  ).toHaveCount(6);
  await page.getByRole("button", { name: "Отмена", exact: true }).click();
  await page.getByRole("button", { name: /host-a/ }).click({ button: "right" });
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
    .getByRole("button", { name: /pg-main/ })
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
    .getByRole("button", { name: /pg-main/ })
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
  await expect(page.getByRole("button", { name: /pg-main/ })).toBeVisible();
  await page.getByRole("button", { name: /payroll/ }).click();
  await expect(page.locator(".object-card .value-badge.production")).toHaveText(
    "Промышленный",
  );
  await expect(page.locator(".object-card .value-badge.success")).toHaveText(
    "Эксплуатируется",
  );
  await page
    .locator(".object-card")
    .screenshot({ path: "/tmp/catalogdb-card-030.png" });
  await search.fill("host-a | missing");
  await expect(
    page.getByText("Совпадений не найдено", { exact: true }),
  ).toBeVisible();
  await search.fill("host-a | pg-main | Тестовый");
  await expect(page.getByRole("button", { name: /pg-main/ })).toBeVisible();
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
  await page.getByRole("button", { name: /host-a/ }).click({ button: "right" });
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
