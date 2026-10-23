import { test, expect } from "@playwright/test";

// Browser contract tests simulate only the Wails bridge. PostgreSQL and Windows
// integration are exercised separately; this is not a replacement for Windows QA.
test.beforeEach(async ({ page }) => {
  await page.addInitScript(() => {
    const g = window as unknown as { go: unknown; __connection: unknown };
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
          IsSetupMode: async () => false,
          Connect: async (connection: unknown) => {
            g.__connection = connection;
            return {
              connected: true,
              schema: (connection as { schema: string }).schema,
              needsAdmin: true,
              user: null,
            };
          },
        },
      },
    };
  });
  await page.goto("/");
});
test("default schema can be changed and reaches the Go bridge", async ({
  page,
}) => {
  const schema = page.getByLabel("Схема БД");
  await expect(schema).toHaveValue("dbcatalog");
  await schema.fill("team_catalog");
  await page
    .getByRole("button", { name: "Проверить, обновить схему и подключиться" })
    .click();
  await expect(
    page.getByRole("heading", { name: "Первый администратор" }),
  ).toBeVisible();
  expect(
    await page.evaluate(
      () =>
        (window as unknown as { __connection: { schema: string } }).__connection
          .schema,
    ),
  ).toBe("team_catalog");
  await expect(page.getByText("Схема: team_catalog")).toBeVisible();
});
test("invalid schema is rejected before connecting", async ({ page }) => {
  await page.getByLabel("Схема БД").fill("x;drop schema public");
  await page
    .getByRole("button", { name: "Проверить, обновить схему и подключиться" })
    .click();
  expect(
    await page
      .getByLabel("Схема БД")
      .evaluate((input: HTMLInputElement) => input.validity.patternMismatch),
  ).toBe(true);
  expect(
    await page.evaluate(
      () => (window as unknown as { __connection?: unknown }).__connection,
    ),
  ).toBeUndefined();
});

test("workspace collapses to accessible icons, persists and expands from the keyboard", async ({
  page,
}) => {
  await page.setViewportSize({ width: 960, height: 640 });
  const sidebar = page.getByRole("complementary", { name: "Рабочая область" });
  const content = page.locator("main");
  await expect(sidebar).toHaveCSS("width", "190px");
  const expandedWidth = (await sidebar.boundingBox())!.width;
  const contentWidth = (await content.boundingBox())!.width;
  await page.getByRole("button", { name: "Свернуть рабочую область" }).click();
  await expect(sidebar).toHaveCSS("width", "72px");
  expect((await content.boundingBox())!.width).toBeGreaterThan(
    contentWidth + 100,
  );
  const settings = page.getByRole("button", { name: "Настройки", exact: true });
  await expect(settings).toHaveAttribute("title", "Настройки");
  await expect(settings.locator("svg")).toBeVisible();
  await expect(settings.locator(".nav-label")).toBeHidden();
  await settings.click();
  await expect(
    page.getByRole("heading", { name: "Настройки", exact: true }),
  ).toBeVisible();
  await page.reload();
  await expect(sidebar).toHaveCSS("width", "72px");
  const toggle = page.getByRole("button", {
    name: "Развернуть рабочую область",
  });
  await expect(toggle).toHaveAttribute("aria-expanded", "false");
  await toggle.focus();
  await page.keyboard.press("Enter");
  await expect(sidebar).toHaveCSS("width", `${expandedWidth}px`);
  await expect(
    page.getByRole("button", { name: "Свернуть рабочую область" }),
  ).toHaveAttribute("aria-expanded", "true");
});

test("workspace remains usable when the WebView blocks local storage", async ({
  page,
}) => {
  await page.addInitScript(() => {
    Object.defineProperty(window, "localStorage", {
      get() {
        throw new DOMException("Storage disabled", "SecurityError");
      },
    });
  });
  await page.reload();
  await page.getByRole("button", { name: "Свернуть рабочую область" }).click();
  await expect(page.getByRole("complementary")).toHaveCSS("width", "72px");
  await expect(
    page.getByRole("heading", { name: "Подключение к PostgreSQL" }),
  ).toBeVisible();
  await page
    .getByRole("button", { name: "Развернуть рабочую область" })
    .click();
  await expect(
    page.getByRole("button", { name: "Свернуть рабочую область" }),
  ).toBeVisible();
});
