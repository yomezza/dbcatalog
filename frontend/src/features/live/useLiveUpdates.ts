import { useEffect, useRef } from "react";
export interface LiveChange {
  schema: string;
  type: string;
}
export function useLiveUpdates(
  userId: string | undefined,
  schema: string,
  busy: boolean,
  refresh: (types: Set<string>, isCurrent: () => boolean) => Promise<void>,
) {
  const latest = useRef({ busy, refresh });
  latest.current = { busy, refresh };
  useEffect(() => {
    if (!userId) return;
    let stopped = false,
      running = false,
      timer: ReturnType<typeof setTimeout> | undefined;
    let retryDelay = 250;
    const pending = new Set<string>();
    const schedule = (delay = retryDelay) => {
      if (!timer && !stopped) timer = setTimeout(() => void flush(), delay);
    };
    async function flush() {
      timer = undefined;
      if (stopped || running || !pending.size) return;
      if (document.hidden) return;
      if (latest.current.busy) {
        schedule(1000);
        return;
      }
      running = true;
      const types = new Set(pending);
      pending.clear();
      try {
        await latest.current.refresh(types, () => !stopped);
        retryDelay = 250;
      } catch {
        pending.add("resync");
        retryDelay = Math.min(30000, Math.max(1000, retryDelay * 2));
      } finally {
        running = false;
        if (pending.size) schedule();
      }
    }
    const enqueue = (type: string) => {
      pending.add(type);
      schedule();
    };
    const off = window.runtime?.EventsOn?.(
      "catalogdb:change",
      (change: LiveChange) => {
        if (
          change.schema === schema &&
          [
            "catalog_changed",
            "users_changed",
            "notifications_changed",
            "resync",
          ].includes(change.type)
        )
          enqueue(change.type);
      },
    );
    const visible = () => {
      if (!document.hidden) enqueue("resync");
    };
    document.addEventListener("visibilitychange", visible);
    // Recovery for dropped events, session expiry and an unavailable native listener.
    const poll = setInterval(() => enqueue("resync"), 60000);
    return () => {
      stopped = true;
      clearInterval(poll);
      if (timer) clearTimeout(timer);
      off?.();
      document.removeEventListener("visibilitychange", visible);
    };
  }, [userId, schema]);
}
