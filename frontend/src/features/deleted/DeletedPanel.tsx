import { useEffect, useState } from "react";
import {
  api,
  type DeletedPage,
  type DeletedRecord,
  type RestorePreview,
} from "../../api";
import { Modal } from "../../Modal";
import { PageNavigation as Pages } from "../../ui/PageNavigation";
import type { AsyncTaskRunner as Run } from "../../ui/types";
export function DeletedPanel({
  busy,
  run,
  onRestored,
  error,
}: {
  error: string;
  busy: boolean;
  run: Run;
  onRestored: () => Promise<unknown>;
}) {
  const [data, setData] = useState<DeletedPage | null>(null),
    [target, setTarget] = useState<DeletedRecord | null>(null),
    [branch, setBranch] = useState(true),
    [preview, setPreview] = useState<RestorePreview | null>(null);
  async function load(p = 0) {
    setData(await api().ListDeleted(p));
  }
  useEffect(() => {
    void run(() => load());
  }, []);
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Удалённые</h1>
          <p className="muted">
            Ветвь включает дочерние записи из той же операции удаления. Сначала
            восстановите отсутствующего родителя. Избранное потребуется отметить
            заново.
          </p>
        </div>
        <button disabled={busy} onClick={() => void run(() => load())}>
          Обновить
        </button>
      </div>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Объект</th>
              <th>Тип</th>
              <th>Кто / когда удалил</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {data?.items.map((r) => (
              <tr key={r.kind + r.id}>
                <td>{r.name}</td>
                <td>{r.kind}</td>
                <td>
                  {r.actor} · {new Date(r.deletedAt).toLocaleString("ru")}
                </td>
                <td>
                  <button
                    disabled={busy}
                    onClick={() => {
                      setTarget(r);
                      setPreview(null);
                      setBranch(true);
                    }}
                  >
                    Восстановить…
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {data && (
        <Pages
          page={data.page}
          total={data.total}
          busy={busy}
          onPage={(p) => void run(() => load(p))}
        />
      )}
      {target && (
        <Modal
          title={"Восстановление: " + target.name}
          busy={busy}
          onClose={() => setTarget(null)}
        >
          {error && (
            <p role="alert" className="notice error">
              {error}
            </p>
          )}
          <label className="check">
            <input
              type="checkbox"
              checked={branch}
              disabled={busy}
              onChange={(e) => {
                setBranch(e.target.checked);
                setPreview(null);
              }}
            />
            Восстановить всю удалённую ветвь
          </label>
          <button
            disabled={busy}
            onClick={() =>
              void run(async () =>
                setPreview(
                  await api().PreviewDeletedRestore(
                    target.kind,
                    target.id,
                    branch,
                  ),
                ),
              )
            }
          >
            Проверить восстановление
          </button>
          {preview && (
            <>
              <p>Будет восстановлено: {preview.items.length}</p>
              <ul>
                {preview.items.map((r) => (
                  <li key={r.kind + r.id}>
                    {r.kind}: {r.name}
                  </li>
                ))}
              </ul>
              {preview.conflicts.map((c) => (
                <p className="notice error" key={c}>
                  {c}
                </p>
              ))}
              <button
                className="primary"
                disabled={busy || !!preview.conflicts.length}
                onClick={() =>
                  void run(async () => {
                    await api().RestoreDeleted(
                      target.kind,
                      target.id,
                      branch,
                      preview.fingerprint,
                    );
                    await load();
                    await onRestored();
                    setTarget(null);
                  })
                }
              >
                Подтвердить восстановление
              </button>
            </>
          )}
        </Modal>
      )}
    </>
  );
}
