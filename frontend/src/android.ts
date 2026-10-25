import type { api } from "./api";

type Backend = ReturnType<typeof api>;
type Change = { schema: string; type: string };
type Pending = {
  resolve: (value: unknown) => void;
  reject: (reason: Error) => void;
};

declare global {
  interface Window {
    CatalogDBAndroid?: { postMessage(message: string): void };
    __catalogdbAndroidReceive?: (message: unknown) => void;
    __catalogdbAndroidEvent?: (message: unknown) => void;
  }
}

let request:
  | ((method: string, args: unknown[]) => Promise<unknown>)
  | undefined;

function record(value: unknown): value is Record<string, unknown> {
  return value !== null && typeof value === "object" && !Array.isArray(value);
}

export function isAndroid() {
  return document.documentElement.dataset.platform === "android";
}

export async function pickRootCertificate(): Promise<string> {
  if (!request)
    throw new Error("Выбор сертификата доступен в Android-приложении");
  const result = await request("PickRootCertificate", []);
  if (typeof result !== "string") {
    throw new Error("Android не вернул выбранный сертификат");
  }
  return result;
}

// Installed before React starts. Native code only attaches this transport to
// trusted bundled assets; authorization remains in the shared Go service.
export function initializeAndroid() {
  const transport = window.CatalogDBAndroid;
  if (!transport || typeof transport.postMessage !== "function" || request) {
    return;
  }
  document.documentElement.dataset.platform = "android";
  const prefix = Array.from(crypto.getRandomValues(new Uint32Array(4)), (n) =>
    n.toString(16),
  ).join("-");
  let sequence = 0;
  let closed = false;
  const pending = new Map<string, Pending>();
  const listeners = new Set<(change: Change) => void>();
  const functions = new Map<string, (...args: unknown[]) => Promise<unknown>>();

  request = (method, args) =>
    new Promise((resolve, reject) => {
      if (closed) {
        reject(new Error("Окно Android-приложения закрыто"));
        return;
      }
      if (pending.size >= 256) {
        reject(new Error("Слишком много ожидающих операций Android"));
        return;
      }
      const id = `${prefix}-${++sequence}`;
      let message: string;
      try {
        message = JSON.stringify({ id, method, args });
        // A 20 MiB upload needs about 28 MiB of base64. Native and Go also
        // enforce operation-specific size and method limits independently.
        if (message.length > 32 * 1024 * 1024) {
          reject(new Error("Слишком большой запрос Android"));
          return;
        }
      } catch {
        reject(new Error("Не удалось подготовить запрос Android"));
        return;
      }
      pending.set(id, { resolve, reject });
      try {
        transport.postMessage(message);
      } catch {
        pending.delete(id);
        reject(new Error("Не удалось передать запрос Android"));
      }
    });

  window.__catalogdbAndroidReceive = (message) => {
    if (!record(message) || typeof message.id !== "string") return;
    const operation = pending.get(message.id);
    if (!operation) return;
    pending.delete(message.id);
    if (typeof message.error === "string" && message.error) {
      operation.reject(new Error(message.error));
    } else if (message.error != null && message.error !== "") {
      operation.reject(new Error("Некорректный ответ Android"));
    } else {
      operation.resolve(message.result);
    }
  };
  window.__catalogdbAndroidEvent = (event) => {
    if (
      !record(event) ||
      event.name !== "catalogdb:change" ||
      !record(event.data) ||
      typeof event.data.schema !== "string" ||
      !/^[a-z_][a-z0-9_]{0,62}$/.test(event.data.schema) ||
      typeof event.data.type !== "string" ||
      ![
        "catalog_changed",
        "users_changed",
        "notifications_changed",
        "resync",
      ].includes(event.data.type)
    )
      return;
    const change = { schema: event.data.schema, type: event.data.type };
    for (const listener of [...listeners]) {
      try {
        listener(change);
      } catch {
        // One stale listener must not prevent other mounted views refreshing.
      }
    }
  };
  const backend = new Proxy(Object.create(null) as Backend, {
    get(_target, method) {
      if (
        typeof method !== "string" ||
        !/^[A-Z][A-Za-z0-9]{0,79}$/.test(method)
      ) {
        return undefined;
      }
      let fn = functions.get(method);
      if (!fn) {
        fn = (...args) => request!(method, args);
        functions.set(method, fn);
      }
      return fn;
    },
  });
  window.go = { main: { App: backend } };
  window.runtime = {
    WindowMinimise() {},
    WindowToggleMaximise() {},
    Quit() {
      void request!("Quit", []).catch(() => {});
    },
    EventsOn(name, callback) {
      if (name !== "catalogdb:change") return () => {};
      listeners.add(callback);
      return () => {
        listeners.delete(callback);
      };
    },
  };
  window.addEventListener("pagehide", (event) => {
    if (event.persisted) return;
    closed = true;
    for (const operation of pending.values()) {
      operation.reject(new Error("Окно Android-приложения закрыто"));
    }
    pending.clear();
    listeners.clear();
  });
}
