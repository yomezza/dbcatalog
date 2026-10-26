import { test, expect, type Page } from "@playwright/test";
import { newObject } from "../src/api";

type NativeRequest = { id: string; method: string; args: unknown[] };
declare global {
  interface Window {
    __androidRequests: NativeRequest[];
    __androidHold: string[];
    __androidErrors: Record<string, string>;
    __androidPicks: string[];
    __androidResults: unknown[];
    __androidEvents: string[];
  }
}

test.use({ hasTouch: true });

async function host(page: Page, catalog = false) {
  await page.addInitScript(
    ({ catalog, server }) => {
      const connection = {
        host: "catalog.example.org",
        port: 5432,
        database: "catalogdb",
        username: "catalogdb",
        schema: "dbcatalog",
        sslMode: "verify-full",
        sslRootCert: "",
      };
      const user = {
        id: "u1",
        login: "demo_admin",
        role: "admin",
        state: "active",
      };
      const status = {
        connected: true,
        schema: "dbcatalog",
        needsAdmin: false,
        user,
      };
      const root = {
        items: [server],
        total: 1,
        page: 0,
        pageSize: 10,
        counts: { servers: 1, instances: 0, databases: 0 },
        children: { s1: { instances: 0, databases: 0, oracle: 0 } },
      };
      window.__androidRequests = [];
      window.__androidHold = [];
      window.__androidErrors = {};
      window.__androidPicks = [];
      window.__androidResults = [];
      window.__androidEvents = [];
      window.CatalogDBAndroid = {
        postMessage(raw: string) {
          const request = JSON.parse(raw) as NativeRequest;
          window.__androidRequests.push(request);
          if (window.__androidHold.includes(request.method)) return;
          const error = window.__androidErrors[request.method];
          let result: unknown;
          switch (request.method) {
            case "GetConnection":
              result = { connection, hasPassword: catalog };
              break;
            case "IsSetupMode":
              result = false;
              break;
            case "Connect":
              result = { ...status, user: null };
              break;
            case "ResumeSession":
            case "GetStatus":
              result = status;
              break;
            case "GetCatalogWorkspace":
              result = {
                root,
                lookups: {
                  status: ["Эксплуатируется"],
                  environment: ["Тестовый"],
                },
                favorites: [],
              };
              break;
            case "BrowseCatalog":
              result = root;
              break;
            case "GetObjectContext":
              result = {
                object: server,
                catalog: { servers: [server], instances: [], databases: [] },
                policies: [],
                serverPage: 0,
                branchPages: {},
                path: server.name,
              };
              break;
            case "GetNotifications":
              result = { items: [], total: 0, unread: 0, page: 0 };
              break;
            case "ListFilterPresets":
              result = [];
              break;
            case "GetLookups":
              result = { status: ["Эксплуатируется"] };
              break;
            case "PickRootCertificate":
              result = window.__androidPicks.shift() ?? "";
              break;
            case "ListDownloads":
              result = [
                {
                  id: "f1",
                  name: "catalog.xlsx",
                  category: "Экспорт каталога",
                  createdAt: "2026-10-24T07:00:00Z",
                  size: 1024,
                  sha256: "demo",
                  available: true,
                },
              ];
              break;
            case "ExportObjectPDF":
              result = { id: "pdf-1", name: "Паспорт.pdf", category: "PDF-паспорт объекта", size: 1000, sha256: "demo", available: true, createdAt: "2026-10-26T07:00:00Z" };
              break;
            case "OpenDownload":
            case "ShowDownload":
            case "SaveDownloadAs":
              result = true;
              break;
            default:
              result = null;
          }
          queueMicrotask(() =>
            window.__catalogdbAndroidReceive?.({
              id: request.id,
              ...(error ? { error } : { result }),
            }),
          );
        },
      };
    },
    {
      catalog,
      server: {
        ...newObject("server"),
        id: "s1",
        name: "APP-TABLET-01",
        address: "192.0.2.10",
        version: 1,
      },
    },
  );
  await page.goto("/");
  await expect(page.locator("html")).toHaveAttribute(
    "data-platform",
    "android",
  );
  await expect(
    page.getByRole("heading", {
      name: catalog ? "Общий каталог" : "Подключение к PostgreSQL",
      exact: true,
    }),
  ).toBeVisible();
}

test("Android bridge starts before React and matches asynchronous responses by unique ID", async ({
  page,
}) => {
  await host(page);
  await expect(page.locator(".window-controls")).toBeHidden();
  await expect(page.getByRole("complementary")).toHaveCSS("width", "72px");
  await page.evaluate(() => {
    window.__androidHold = ["GetLookups", "GetStatistics"];
    void window.go!.main.App.GetLookups().then((result) => {
      window.__androidResults[0] = result;
    });
    void window.go!.main.App.GetStatistics().then((result) => {
      window.__androidResults[1] = result;
    });
  });
  const requests = await page.evaluate(() =>
    window.__androidRequests.filter((r) =>
      window.__androidHold.includes(r.method),
    ),
  );
  expect(requests).toHaveLength(2);
  expect(requests[0].id).not.toBe(requests[1].id);
  expect(requests[0].id).toMatch(/^[a-f\d-]+$/);
  await page.evaluate(([first, second]) => {
    window.__catalogdbAndroidReceive?.(null);
    window.__catalogdbAndroidReceive?.({ id: "unknown-id", result: "ignored" });
    window.__catalogdbAndroidReceive?.({
      id: second.id,
      result: { marker: "second" },
    });
    window.__catalogdbAndroidReceive?.({
      id: first.id,
      result: { marker: "first" },
    });
    window.__catalogdbAndroidReceive?.({
      id: first.id,
      result: { marker: "duplicate" },
    });
  }, requests);
  await expect
    .poll(() => page.evaluate(() => window.__androidResults))
    .toEqual([{ marker: "first" }, { marker: "second" }]);
  const error = await page.evaluate(async () => {
    window.__androidHold = [];
    window.__androidErrors.GetStatistics = "Тестовый отказ PostgreSQL";
    try {
      await window.go!.main.App.GetStatistics();
      return "resolved";
    } catch (reason) {
      return reason instanceof Error ? reason.message : String(reason);
    }
  });
  expect(error).toBe("Тестовый отказ PostgreSQL");
});

test("Android object menu creates a PDF and opens the native save action", async ({ page }) => {
  await page.setViewportSize({ width: 1024, height: 768 });
  await host(page, true);
  await page.getByRole("button", { name: "Действия: APP-TABLET-01", exact: true }).tap();
  await page.getByRole("menuitem", { name: "Печать в PDF", exact: true }).tap();
  await expect.poll(() => page.evaluate(() => window.__androidRequests.filter((r) => ["ExportObjectPDF", "SaveDownloadAs"].includes(r.method)).map((r) => ({ method: r.method, args: r.args })))).toEqual([
    { method: "ExportObjectPDF", args: ["server", "s1"] },
    { method: "SaveDownloadAs", args: ["pdf-1"] },
  ]);
  await expect(page.locator(".notice[role=status]")).toContainText("PDF-паспорт сохранён");
});

test("Android live events validate payloads and remove only the unsubscribed listener", async ({
  page,
}) => {
  await host(page);
  const events = await page.evaluate(() => {
    const off = window.runtime!.EventsOn!("catalogdb:change", (change) =>
      window.__androidEvents.push("first:" + change.type),
    );
    window.runtime!.EventsOn!("catalogdb:change", (change) =>
      window.__androidEvents.push("second:" + change.type),
    );
    window.__catalogdbAndroidEvent?.(null);
    window.__catalogdbAndroidEvent?.({
      name: "unknown",
      data: { schema: "dbcatalog", type: "resync" },
    });
    window.__catalogdbAndroidEvent?.({
      name: "catalogdb:change",
      data: { schema: "dbcatalog", type: "unexpected" },
    });
    window.__catalogdbAndroidEvent?.({
      name: "catalogdb:change",
      data: { schema: "dbcatalog", type: "catalog_changed" },
    });
    off();
    window.__catalogdbAndroidEvent?.({
      name: "catalogdb:change",
      data: { schema: "dbcatalog", type: "resync" },
    });
    return window.__androidEvents;
  });
  expect(events).toEqual([
    "first:catalog_changed",
    "second:catalog_changed",
    "second:resync",
  ]);
});

test("Android certificate chooser retains selection on cancellation and can clear it", async ({
  page,
}) => {
  await host(page);
  const certificate = page.getByLabel("Корневой сертификат (путь)");
  await expect(certificate).toHaveAttribute("readonly", "");
  await page.evaluate(() => {
    window.__androidPicks = [
      "/data/user/0/org.catalogdb/files/certificates/root-ca.pem",
      "",
    ];
  });
  await page
    .getByRole("button", { name: "Выбрать сертификат…", exact: true })
    .tap();
  await expect(certificate).toHaveValue(
    "/data/user/0/org.catalogdb/files/certificates/root-ca.pem",
  );
  await page
    .getByRole("button", { name: "Выбрать сертификат…", exact: true })
    .tap();
  await expect(certificate).toHaveValue(
    "/data/user/0/org.catalogdb/files/certificates/root-ca.pem",
  );
  await page.getByRole("button", { name: "Сбросить", exact: true }).tap();
  await expect(certificate).toHaveValue("");
});

for (const viewport of [
  { width: 768, height: 1024 },
  { width: 1280, height: 800 },
]) {
  test(`Android tablet ${viewport.width}×${viewport.height} exposes touch actions and bounded dialogs`, async ({
    page,
  }) => {
    await page.setViewportSize(viewport);
    await host(page, true);
    const columns = await page
      .locator(".catalog-layout")
      .evaluate(
        (element) =>
          getComputedStyle(element).gridTemplateColumns.split(" ").length,
      );
    expect(columns).toBe(viewport.width < 1000 ? 1 : 2);
    expect(
      await page.evaluate(
        () => document.documentElement.scrollWidth <= window.innerWidth,
      ),
    ).toBe(true);
    const actions = page.getByRole("button", {
      name: "Действия: APP-TABLET-01",
      exact: true,
    });
    await expect(actions).toBeVisible();
    const target = await actions.boundingBox();
    expect(target!.width).toBeGreaterThanOrEqual(44);
    expect(target!.height).toBeGreaterThanOrEqual(44);
    await actions.tap();
    const menu = page.getByRole("menu", { name: "Действия с объектом" });
    await expect(menu).toBeVisible();
    const bounds = await menu.boundingBox();
    expect(bounds!.x).toBeGreaterThanOrEqual(0);
    expect(bounds!.x + bounds!.width).toBeLessThanOrEqual(viewport.width);
    expect(bounds!.y + bounds!.height).toBeLessThanOrEqual(viewport.height);
    await menu.getByRole("menuitem", { name: "Изменить", exact: true }).tap();
    const dialog = page.getByRole("dialog", { name: "Редактирование объекта" });
    await expect(dialog).toBeVisible();
    const box = await dialog.boundingBox();
    expect(box!.x).toBeGreaterThanOrEqual(0);
    expect(box!.width).toBeLessThanOrEqual(viewport.width - 20);
  });
}

test("Android downloads share through the native method instead of exposing a filesystem browser", async ({
  page,
}) => {
  await host(page, true);
  await page.getByRole("button", { name: "Загрузки", exact: true }).tap();
  await page.getByRole("button", { name: "Поделиться", exact: true }).tap();
  await expect
    .poll(() =>
      page.evaluate(() =>
        window.__androidRequests
          .filter((r) => r.method === "ShowDownload")
          .map((r) => r.args),
      ),
    )
    .toEqual([["f1"]]);
  await expect(
    page.getByRole("button", { name: "В папке", exact: true }),
  ).toHaveCount(0);
});
