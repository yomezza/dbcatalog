import { useState } from "react";
import {
  api,
  type ObjectRef,
  type Lookups,
  type BulkPatch,
  type BulkPreview,
} from "../../api";
import { Modal } from "../../Modal";
import type { AsyncTaskRunner as Run } from "../../ui/types";
export function BulkDialog({
  refs,
  lookups,
  busy,
  run,
  onClose,
  onSaved,
  error,
}: {
  error: string;
  refs: ObjectRef[];
  lookups: Lookups;
  busy: boolean;
  run: Run;
  onClose: () => void;
  onSaved: () => Promise<unknown>;
}) {
  const [patch, setPatch] = useState<BulkPatch>({
      setResponsible: false,
      responsible: "",
      setOrganization: false,
      organization: "",
      setStatus: false,
      status: "",
      setAdministration: false,
      administration: [],
    }),
    [preview, setPreview] = useState<BulkPreview | null>(null);
  function update(p: Partial<BulkPatch>) {
    setPatch({ ...patch, ...p });
    setPreview(null);
  }
  return (
    <Modal
      title={"Массовое редактирование: " + refs.length + " БД"}
      busy={busy}
      onClose={onClose}
      className="wide-modal"
    >
      {error && (
        <p role="alert" className="notice error">
          {error}
        </p>
      )}
      <p>
        Меняются только отмеченные поля. Пустое значение очищает поле. Выбор
        работы АБД полностью заменяет список.
      </p>
      <fieldset disabled={busy}>
        <label className="check">
          <input
            type="checkbox"
            checked={patch.setResponsible}
            onChange={(e) => update({ setResponsible: e.target.checked })}
          />
          Ответственный
        </label>
        <input
          aria-label="Новый ответственный"
          value={patch.responsible}
          onChange={(e) => update({ responsible: e.target.value })}
          list="bulk-responsible"
        />
        <datalist id="bulk-responsible">
          {(lookups.responsible ?? []).map((v) => (
            <option key={v}>{v}</option>
          ))}
        </datalist>
        <label className="check">
          <input
            type="checkbox"
            checked={patch.setOrganization}
            onChange={(e) => update({ setOrganization: e.target.checked })}
          />
          Обслуживающая организация
        </label>
        <input
          aria-label="Новая организация"
          value={patch.organization}
          onChange={(e) => update({ organization: e.target.value })}
        />
        <label className="check">
          <input
            type="checkbox"
            checked={patch.setStatus}
            onChange={(e) => update({ setStatus: e.target.checked })}
          />
          Состояние
        </label>
        <select
          aria-label="Новое состояние"
          value={patch.status}
          onChange={(e) => update({ status: e.target.value })}
        >
          <option value="">Выберите состояние</option>
          {(lookups.status ?? []).map((v) => (
            <option key={v}>{v}</option>
          ))}
        </select>
        <label className="check">
          <input
            type="checkbox"
            checked={patch.setAdministration}
            onChange={(e) => update({ setAdministration: e.target.checked })}
          />
          Работа АБД
        </label>
        <div className="choice-grid">
          {(lookups.administration ?? []).map((v) => (
            <label className="check" key={v}>
              <input
                type="checkbox"
                checked={patch.administration.includes(v)}
                onChange={(e) =>
                  update({
                    administration: e.target.checked
                      ? [...patch.administration, v]
                      : patch.administration.filter((x) => x !== v),
                  })
                }
              />
              {v}
            </label>
          ))}
        </div>
      </fieldset>
      <button
        disabled={busy}
        onClick={() =>
          void run(async () =>
            setPreview(await api().PreviewBulkEdit(refs, patch)),
          )
        }
      >
        Предпросмотр изменений
      </button>
      {preview && (
        <>
          <p>
            Выбрано {preview.selected}; изменится {preview.changed}. Сохранение
            всех изменений выполняется одной операцией.
          </p>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>База / ID</th>
                  <th>Поле</th>
                  <th>Было</th>
                  <th>Станет</th>
                </tr>
              </thead>
              <tbody>
                {preview.items.flatMap(({ before: b, after: a }) =>
                  [
                    [
                      "Ответственный",
                      b.attributes.responsible,
                      a.attributes.responsible,
                    ],
                    [
                      "Организация",
                      b.attributes.serviceOrganization,
                      a.attributes.serviceOrganization,
                    ],
                    ["Состояние", b.status, a.status],
                    [
                      "Работа АБД",
                      b.attributes.administration.join("; "),
                      a.attributes.administration.join("; "),
                    ],
                  ]
                    .filter(([, x, y]) => x !== y)
                    .map(([field, x, y]) => (
                      <tr key={b.id + field}>
                        <td>
                          {b.name}
                          <small>{b.id}</small>
                        </td>
                        <td>{field}</td>
                        <td>{x || "—"}</td>
                        <td>{y || "—"}</td>
                      </tr>
                    )),
                )}
              </tbody>
            </table>
          </div>
          <button
            className="primary"
            disabled={busy || !preview.changed}
            onClick={() =>
              void run(async () => {
                await api().ApplyBulkEdit(refs, patch, preview.fingerprint);
                await onSaved();
                onClose();
              })
            }
          >
            Подтвердить изменения
          </button>
        </>
      )}
    </Modal>
  );
}
