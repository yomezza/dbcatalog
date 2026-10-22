import { useState } from "react";
import type { ImportRowPreview, Lookups } from "./api";
import { Modal } from "./Modal";
import { Field } from "./components";
const headers = [
  "SID",
  "Экземпляр",
  "Состояние",
  "Тип эксплуатации",
  "Имя сервера",
  "Тип СУБД",
  "Версия СУБД",
  "Подключение к серверу",
  "Доменное членство",
  "IPv4-адрес",
  "Месторасположение",
  "Информационная система",
  "Обслуживающая организация",
  "Сопровождение (руководитель)",
  "Ответственное лицо",
  "Операционная система",
  "Версия системы",
  "Семейство системы",
  "Назначение",
  "Работа АБД",
  "Персональные данные",
];
const categories: Record<number, string> = {
  2: "status",
  3: "environment",
  5: "dbms",
  7: "connectionMethod",
  8: "domain",
  10: "location",
  12: "serviceOrganization",
  13: "supervisor",
  14: "responsible",
  19: "administration",
};
export function ImportRowEditor({
  row,
  lookups,
  busy,
  error,
  onClose,
  onSave,
}: {
  row: ImportRowPreview;
  lookups: Lookups;
  busy: boolean;
  error: string;
  onClose: () => void;
  onSave: (values: string[]) => Promise<void>;
}) {
  const [values, setValues] = useState(() =>
    Array.from({ length: 21 }, (_, i) => row.values?.[i] ?? ""),
  );
  return (
    <Modal
      title={`Исправление строки ${row.row}`}
      onClose={onClose}
      busy={busy}
      className="import-row-editor"
    >
      <p className="info">
        После исправления проверяется весь файл. Общие параметры сервера и
        экземпляра должны совпадать в повторяющихся строках. Данные каталога
        изменятся только после подтверждения импорта.
      </p>
      {row.errors.length > 0 && (
        <ul className="row-errors">
          {row.errors.map((e, i) => (
            <li key={i}>{e}</li>
          ))}
        </ul>
      )}
      {values[5] === "Oracle Database" && (
        <p className="info">
          Oracle Database размещается непосредственно на сервере. Поле
          «Экземпляр» не участвует в проверке и сопоставлении.
        </p>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          void onSave(values);
        }}
      >
        <fieldset disabled={busy}>
          <div className="import-editor-grid">
            {headers.map((label, i) => {
              const options = lookups[categories[i]] ?? [];
              return (
                <Field label={label} key={i}>
                  {i === 19 ? (
                    <textarea
                      value={values[i]}
                      placeholder="Значения через ;"
                      onChange={(e) =>
                        setValues((v) =>
                          v.map((old, c) => (c === i ? e.target.value : old)),
                        )
                      }
                    />
                  ) : (
                    <input
                      value={values[i]}
                      list={options.length ? `import-options-${i}` : undefined}
                      maxLength={20000}
                      onChange={(e) =>
                        setValues((v) =>
                          v.map((old, c) => (c === i ? e.target.value : old)),
                        )
                      }
                    />
                  )}
                  {options.length > 0 && (
                    <datalist id={`import-options-${i}`}>
                      {options.map((v) => (
                        <option key={v} value={v} />
                      ))}
                    </datalist>
                  )}
                </Field>
              );
            })}
          </div>
          <div className="actions">
            <button type="button" onClick={onClose}>
              Отмена
            </button>
            <button className="primary">Проверить исправления</button>
          </div>
        </fieldset>
      </form>
      {error && (
        <div className="notice error" role="alert">
          {error}
        </div>
      )}
    </Modal>
  );
}
