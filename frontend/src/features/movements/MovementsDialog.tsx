import { useEffect, useState } from "react";
import {
  api,
  type CatalogObject,
  type MovePreview,
  type MoveRequest,
  type ObjectOption,
  type MovementPage,
} from "../../api";
import { Modal } from "../../Modal";
import { ObjectPicker } from "../../ui/ObjectPicker";
import { PageNavigation } from "../../ui/PageNavigation";
import type { AsyncTaskRunner } from "../../ui/types";
export function MoveDialog({
  object,
  busy,
  error,
  run,
  onClose,
  onMoved,
}: {
  object: CatalogObject;
  busy: boolean;
  error: string;
  run: AsyncTaskRunner;
  onClose: () => void;
  onMoved: (o: CatalogObject) => Promise<void>;
}) {
  const [parent, setParent] = useState<ObjectOption | null>(null),
    [reason, setReason] = useState(""),
    [preview, setPreview] = useState<MovePreview | null>(null);
  const kind =
    object.kind === "instance" || object.dbms === "Oracle Database"
      ? "server"
      : "instance";
  const request: MoveRequest = {
    kind: object.kind,
    id: object.id,
    version: object.version,
    parentId: parent?.object.id ?? "",
    parentVersion: parent?.object.version ?? 0,
    reason,
  };
  return (
    <Modal
      title={"Переместить: " + object.name}
      busy={busy}
      onClose={onClose}
      className="wide-modal"
    >
      <p>
        Меняется размещение записи в каталоге. Физические данные СУБД не
        переносятся. Вложения остаются связаны с объектом.
      </p>
      {error && (
        <p className="notice error" role="alert">
          {error}
        </p>
      )}
      <ObjectPicker
        kind={kind}
        value={parent?.object.id ?? ""}
        label={kind === "server" ? "Новый сервер" : "Новый экземпляр"}
        disabled={busy}
        onChange={(p) => {
          setParent(p);
          setPreview(null);
        }}
      />
      <label className="field">
        <span>Причина перемещения</span>
        <textarea
          disabled={busy}
          maxLength={2000}
          aria-label="Причина перемещения"
          value={reason}
          onChange={(e) => {
            setReason(e.target.value);
            setPreview(null);
          }}
        />
      </label>
      <button
        disabled={busy || !parent || !reason.trim()}
        onClick={() =>
          void run(async () => setPreview(await api().PreviewMove(request)))
        }
      >
        Предпросмотр перемещения
      </button>
      {preview && (
        <>
          <dl className="settings-fields">
            <dt>Откуда</dt>
            <dd>{preview.fromPath}</dd>
            <dt>Куда</dt>
            <dd>{preview.toPath}</dd>
            <dt>СУБД до → после</dt>
            <dd>
              {preview.fromDBMS} → {preview.toDBMS}
            </dd>
            <dt>Дочерних БД</dt>
            <dd>{preview.databases}</dd>
            <dt>Вложений</dt>
            <dd>{preview.attachments}</dd>
            <dt>Политика РК до</dt>
            <dd>{policyText(preview.fromPolicy)}</dd>
            <dt>Политика РК после</dt>
            <dd>{policyText(preview.toPolicy)}</dd>
          </dl>
          {preview.conflicts.map((c) => (
            <p className="notice error" key={c}>
              {c}
            </p>
          ))}
          <button
            disabled={busy || !!preview.conflicts.length}
            className="primary"
            onClick={() =>
              void run(async () => {
                const saved = await api().ApplyMove(
                  request,
                  preview.fingerprint,
                );
                await onMoved(saved);
                onClose();
              })
            }
          >
            Подтвердить перемещение
          </button>
        </>
      )}
    </Modal>
  );
}
function policyText(p: MovePreview["fromPolicy"]) {
  return p
    ? p.name +
        " · " +
        (p.enabled ? "Включена" : "Отключена") +
        " · " +
        p.schedule +
        " · " +
        p.retentionDays +
        " дней"
    : "Не назначена";
}
export function MovementsDialog({
  object,
  busy,
  error,
  run,
  onClose,
}: {
  object: CatalogObject;
  busy: boolean;
  error: string;
  run: AsyncTaskRunner;
  onClose: () => void;
}) {
  const [data, setData] = useState<MovementPage | null>(null);
  async function load(page = 0) {
    setData(await api().GetMovements(object.kind, object.id, page));
  }
  useEffect(() => {
    void run(() => load());
  }, [object.kind, object.id]);
  return (
    <Modal
      title={"История перемещений: " + object.name}
      className="wide-modal"
      busy={busy}
      onClose={onClose}
    >
      {error && (
        <p className="notice error" role="alert">
          {error}
        </p>
      )}
      <p>
        Сохраняются пути и политики на момент изменения, в том числе при
        редактировании карточки и возврате к версии.
      </p>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Дата / автор</th>
              <th>Откуда → куда</th>
              <th>Причина</th>
              <th>Политика РК до → после</th>
            </tr>
          </thead>
          <tbody>
            {data?.items.map((m) => (
              <tr key={m.id}>
                <td>
                  {new Date(m.createdAt).toLocaleString("ru")}
                  <small>
                    {m.actor} · v{m.version}
                  </small>
                </td>
                <td>
                  {m.fromPath}
                  <small>→ {m.toPath}</small>
                </td>
                <td>{m.reason}</td>
                <td>
                  {policyText(m.fromPolicy)}
                  <small>→ {policyText(m.toPolicy)}</small>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {data && !data.total && <p className="empty">Перемещений пока нет.</p>}
      {data && (
        <PageNavigation
          page={data.page}
          total={data.total}
          busy={busy}
          onPage={(p) => void run(() => load(p))}
        />
      )}
    </Modal>
  );
}
