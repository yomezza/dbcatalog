import type { CatalogObject, HistoryEntry } from "./api";
import { fieldLabels, objectValue } from "./objectFields";
import { Modal } from "./Modal";
export function RestoreDialog({
  object,
  entry,
  side,
  busy,
  error,
  onClose,
  onConfirm,
}: {
  object: CatalogObject;
  entry: HistoryEntry;
  side: "before" | "after";
  busy: boolean;
  error: string;
  onClose: () => void;
  onConfirm: () => void;
}) {
  const target = entry[side],
    changes = Object.entries(fieldLabels).filter(
      ([key]) =>
        key !== "version" &&
        objectValue(object, key) !== objectValue(target, key),
    );
  return (
    <Modal
      title={`Восстановить состояние «${object.name}»?`}
      onClose={onClose}
      busy={busy}
      className="restore-dialog"
    >
      <p className="info">
        Состояние «{side === "before" ? "Было" : "Стало"}» из записи{" "}
        {new Date(entry.createdAt).toLocaleString("ru")} будет записано новой
        версией. Более поздняя история сохранится. Восстанавливаются поля
        существующего объекта; удалённые дочерние записи и политики не
        восстанавливаются.
      </p>
      <div className="restore-table">
        <table>
          <thead>
            <tr>
              <th>Поле</th>
              <th>Сейчас</th>
              <th>Будет</th>
            </tr>
          </thead>
          <tbody>
            {changes.map(([key, label]) => (
              <tr key={key}>
                <td>{label}</td>
                <td>{objectValue(object, key) || "—"}</td>
                <td>{objectValue(target, key) || "—"}</td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {!changes.length && <p>Поля уже соответствуют выбранному состоянию.</p>}
      <div className="actions">
        <button disabled={busy} onClick={onClose}>
          Отмена
        </button>
        <button
          className="primary"
          disabled={busy || !changes.length}
          onClick={onConfirm}
        >
          Подтвердить восстановление
        </button>
      </div>
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}
    </Modal>
  );
}
