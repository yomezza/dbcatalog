import { useEffect, useState } from "react";
import { api, type Notification, type NotificationPage } from "./api";
export function NotificationsPanel({
  revision = 0,
  busy,
  run,
  onUnread,
  onNavigate,
}: {
  revision?: number;
  busy: boolean;
  run: (fn: () => Promise<void>) => Promise<void>;
  onUnread: (count: number) => void;
  onNavigate: (n: Notification) => Promise<void>;
}) {
  const [data, setData] = useState<NotificationPage | null>(null),
    [onlyUnread, setOnlyUnread] = useState(false);
  const load = (page = 0, only = onlyUnread) =>
    run(async () => {
      const next = await api().GetNotifications(page, only);
      setData(next);
      onUnread(next.unread);
    });
  useEffect(() => {
    void load(data?.page ?? 0);
  }, [revision]);
  async function mark(ids: string[], all = false) {
    await api().MarkNotificationsRead(ids, all);
    const next = await api().GetNotifications(data?.page ?? 0, onlyUnread);
    setData(next);
    onUnread(next.unread);
  }
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">СОБЫТИЯ</div>
          <h1>Уведомления</h1>
          <p className="muted">
            Заявки, изменения прав и избранных объектов, административные
            события.
          </p>
        </div>
        <div className="actions">
          <button disabled={busy} onClick={() => void load(data?.page)}>
            ↻ Обновить
          </button>
          <button
            disabled={busy || !data?.unread}
            onClick={() => void run(() => mark([], true))}
          >
            Прочитать все
          </button>
        </div>
      </div>
      <label className="check">
        <input
          type="checkbox"
          checked={onlyUnread}
          disabled={busy}
          onChange={(e) => {
            setOnlyUnread(e.target.checked);
            void load(0, e.target.checked);
          }}
        />
        Только непрочитанные ({data?.unread ?? 0})
      </label>
      <div className="notification-list">
        {data?.items.map((n) => (
          <article
            className={"notification-item " + (!n.readAt ? "unread" : "")}
            key={n.id}
          >
            <div className="notification-copy">
              <small>
                {new Date(n.createdAt).toLocaleString("ru")}
                {!n.readAt && " · Новое"}
              </small>
              <h2>{n.title}</h2>
              <p>{n.body}</p>
            </div>
            <div className="actions">
              {n.entity && (
                <button
                  disabled={busy}
                  onClick={() =>
                    void run(async () => {
                      await mark([n.id]);
                      await onNavigate(n);
                    })
                  }
                >
                  Перейти
                </button>
              )}
              {!n.readAt && (
                <button
                  disabled={busy}
                  onClick={() => void run(() => mark([n.id]))}
                >
                  Прочитано
                </button>
              )}
            </div>
          </article>
        ))}
      </div>
      {data && !data.items.length && (
        <div className="empty">
          {onlyUnread
            ? "Непрочитанных уведомлений нет."
            : "Уведомлений пока нет."}
        </div>
      )}
      <div className="pagination">
        <button
          disabled={busy || !data?.page}
          onClick={() => void load((data?.page ?? 0) - 1)}
        >
          ←
        </button>
        <span>
          Уведомлений: {data?.total ?? 0} · Страница {(data?.page ?? 0) + 1} из{" "}
          {Math.max(1, Math.ceil((data?.total ?? 0) / 25))}
        </span>
        <button
          disabled={busy || !data || (data.page + 1) * 25 >= data.total}
          onClick={() => void load((data?.page ?? 0) + 1)}
        >
          →
        </button>
      </div>
    </>
  );
}
