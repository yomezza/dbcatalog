import { useEffect, useState } from "react";
import { api, type Catalog, type HistoryPage, type RecordTarget } from "./api";
import { Modal } from "./Modal";
import { SnapshotTable } from "./SnapshotTable";
import { actionLabels } from "./recordFields";
export function HistoryDialog({
  target,
  catalog,
  busy,
  error,
  run,
  onClose,
  onVersions,
}: {
  target: RecordTarget;
  catalog: Catalog;
  busy: boolean;
  error: string;
  run: (fn: () => Promise<void>) => Promise<void>;
  onClose: () => void;
  onVersions?: () => void;
}) {
  const [data, setData] = useState<HistoryPage | null>(null);
  const load = (page = 0) =>
    run(async () =>
      setData(await api().GetHistoryPage(target.kind, target.id, page)),
    );
  useEffect(() => {
    void load();
  }, [target.kind, target.id]);
  return (
    <Modal
      title={`История изменений: ${target.name}`}
      onClose={onClose}
      busy={busy}
      className="record-history-dialog"
    >
      <div className="actions">
        <button disabled={busy} onClick={() => void load(data?.page)}>
          ↻ Обновить историю
        </button>
        {onVersions && (
          <button disabled={busy} onClick={onVersions}>
            Версии записи
          </button>
        )}
      </div>
      <div className="history-entries">
        {data?.items.map((h) => (
          <article key={h.id}>
            <div className="history-event-heading">
              <strong>{actionLabels[h.action] ?? h.action}</strong>
              <span>
                {h.before?.version ? `v${h.before.version}` : "—"} →{" "}
                {h.after?.version ? `v${h.after.version}` : "—"}
              </span>
            </div>
            <p className="muted">
              {h.actor} · {new Date(h.createdAt).toLocaleString("ru")}
            </p>
            <SnapshotTable
              before={h.before}
              after={h.after}
              kind={target.kind}
              catalog={catalog}
              onlyChanges
            />
          </article>
        ))}
      </div>
      {data && !data.items.length && (
        <p className="empty">Нет записей истории.</p>
      )}
      <div className="pagination">
        <button
          disabled={busy || !data?.page}
          onClick={() => void load((data?.page ?? 0) - 1)}
        >
          ←
        </button>
        <span>
          Событий: {data?.total ?? 0} · Страница {(data?.page ?? 0) + 1} из{" "}
          {Math.max(1, Math.ceil((data?.total ?? 0) / 25))}
        </span>
        <button
          disabled={busy || !data || (data.page + 1) * 25 >= data.total}
          onClick={() => void load((data?.page ?? 0) + 1)}
        >
          →
        </button>
      </div>
      {error && (
        <div role="alert" className="notice error">
          {error}
        </div>
      )}
    </Modal>
  );
}
