import { useEffect, useState } from "react";
import {
  api,
  type Catalog,
  type RecordSnapshot,
  type RecordTarget,
  type RecordVersion,
  type VersionPage,
} from "./api";
import { Modal } from "./Modal";
import { SnapshotTable } from "./SnapshotTable";
import { actionLabels } from "./recordFields";
export function VersionsDialog({
  target,
  catalog,
  busy,
  error,
  run,
  onClose,
  onRestored,
}: {
  target: RecordTarget;
  catalog: Catalog;
  busy: boolean;
  error: string;
  run: (fn: () => Promise<void>) => Promise<void>;
  onClose: () => void;
  onRestored: () => Promise<void>;
}) {
  const [data, setData] = useState<VersionPage | null>(null),
    [selected, setSelected] = useState<{
      meta: RecordVersion;
      snapshot: RecordSnapshot;
      current: RecordSnapshot | null;
    } | null>(null),
    [confirm, setConfirm] = useState(false),
    [notice, setNotice] = useState("");
  async function refresh(page = 0) {
    const next = await api().GetVersions(target.kind, target.id, page);
    setData(next);
    setSelected(null);
    setConfirm(false);
  }
  useEffect(() => {
    void run(() => refresh());
  }, [target.kind, target.id]);
  const choose = (meta: RecordVersion) =>
    run(async () => {
      if (!data) return;
      setNotice("");
      const [snapshot, current] = await Promise.all([
        api().GetRecordVersion(target.kind, target.id, meta.version),
        data.exists && data.currentVersion !== meta.version
          ? api().GetRecordVersion(target.kind, target.id, data.currentVersion)
          : Promise.resolve(null),
      ]);
      setSelected({
        meta,
        snapshot,
        current: meta.version === data.currentVersion ? snapshot : current,
      });
      setConfirm(false);
    });
  return (
    <Modal
      title={`Версии записи: ${target.name}`}
      onClose={onClose}
      busy={busy}
      className="versions-dialog"
    >
      <p className="info">
        Возврат сохраняет выбранное состояние новой версией. Все промежуточные
        версии и история останутся. Удалённые дочерние записи автоматически не
        восстанавливаются.
      </p>
      {!confirm && (
        <>
          <div className="actions">
            <span className="muted">
              {data?.exists
                ? `Текущая версия: v${data.currentVersion}`
                : "Запись отсутствует: только просмотр сохранённых версий"}
            </span>
            <button
              disabled={busy}
              onClick={() => void run(() => refresh(data?.page))}
            >
              ↻ Обновить версии
            </button>
          </div>
          <div className="version-list">
            <table>
              <thead>
                <tr>
                  <th scope="col">Версия</th>
                  <th scope="col">Дата</th>
                  <th scope="col">Пользователь</th>
                  <th scope="col">Действие</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {data?.items.map((v) => (
                  <tr
                    key={v.version}
                    className={
                      selected?.meta.version === v.version
                        ? "selected-version"
                        : ""
                    }
                  >
                    <td>
                      <strong>v{v.version}</strong>
                      {v.version === data.currentVersion && (
                        <small className="table-caption">Текущая</small>
                      )}
                    </td>
                    <td>{new Date(v.createdAt).toLocaleString("ru")}</td>
                    <td>{v.actor}</td>
                    <td>{actionLabels[v.action] ?? v.action}</td>
                    <td>
                      <button disabled={busy} onClick={() => void choose(v)}>
                        Просмотреть v{v.version}
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          {data && !data.items.length && (
            <p className="empty">Сохранённых версий нет.</p>
          )}
          <div className="pagination">
            <button
              disabled={busy || !data?.page}
              onClick={() => void run(() => refresh((data?.page ?? 0) - 1))}
            >
              ←
            </button>
            <span>
              Версий: {data?.total ?? 0} · Страница {(data?.page ?? 0) + 1} из{" "}
              {Math.max(1, Math.ceil((data?.total ?? 0) / 25))}
            </span>
            <button
              disabled={busy || !data || (data.page + 1) * 25 >= data.total}
              onClick={() => void run(() => refresh((data?.page ?? 0) + 1))}
            >
              →
            </button>
          </div>
        </>
      )}
      {selected && (
        <section className="version-preview">
          <h3>
            {confirm
              ? `Подтверждение возврата к v${selected.meta.version}`
              : `Состояние версии v${selected.meta.version}`}
          </h3>
          <SnapshotTable
            before={selected.current}
            after={selected.snapshot}
            kind={target.kind}
            catalog={catalog}
            onlyChanges={confirm}
            labels={["Сейчас", `Версия v${selected.meta.version}`]}
          />
          {data?.exists && selected.meta.version !== data.currentVersion && (
            <div className="actions">
              {confirm ? (
                <>
                  <button disabled={busy} onClick={() => setConfirm(false)}>
                    Назад к версиям
                  </button>
                  <button
                    className="primary"
                    disabled={busy}
                    onClick={() =>
                      void run(async () => {
                        await api().RestoreVersion(
                          target.kind,
                          target.id,
                          data.currentVersion,
                          selected.meta.version,
                        );
                        await onRestored();
                        await refresh(data.page);
                        setNotice(
                          "Возврат выполнен. Создана новая версия записи.",
                        );
                      })
                    }
                  >
                    Подтвердить возврат к v{selected.meta.version}
                  </button>
                </>
              ) : (
                <button
                  className="primary"
                  disabled={busy}
                  onClick={() => setConfirm(true)}
                >
                  Вернуться к v{selected.meta.version}
                </button>
              )}
            </div>
          )}
        </section>
      )}
      {notice && (
        <p className="notice success" role="status">
          {notice}
        </p>
      )}
      {error && (
        <div role="alert" className="notice error">
          {error}
        </div>
      )}
    </Modal>
  );
}
