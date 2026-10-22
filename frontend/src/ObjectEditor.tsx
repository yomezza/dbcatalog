import { objectPath } from "./objectPath";
import React, { useId } from "react";
import {
  type Attributes,
  type Catalog,
  type CatalogObject,
  type Lookups,
  emptyAttributes,
} from "./api";
import { Field } from "./components";

export function Choice({
  value,
  options,
  onChange,
  required = false,
}: {
  value: string;
  options: string[];
  onChange: (v: string) => void;
  required?: boolean;
}) {
  const values =
    value && !options.includes(value) ? [value, ...options] : options;
  return (
    <select
      value={value}
      required={required}
      onChange={(e) => onChange(e.target.value)}
    >
      <option value="">Не указано</option>
      {values.map((v) => (
        <option key={v} value={v}>
          {v}
        </option>
      ))}
    </select>
  );
}
export function SuggestedInput({
  value,
  options,
  onChange,
}: {
  value: string;
  options: string[];
  onChange: (v: string) => void;
}) {
  const id = useId();
  return (
    <>
      <input
        value={value}
        list={id}
        maxLength={1000}
        onChange={(e) => onChange(e.target.value)}
        placeholder="Выберите или введите своё значение"
      />
      <datalist id={id}>
        {options.map((v) => (
          <option key={v} value={v} />
        ))}
      </datalist>
    </>
  );
}
interface Props {
  editor: CatalogObject;
  catalog: Catalog;
  lookups: Lookups;
  busy: boolean;
  error: string;
  onChange: (o: CatalogObject) => void;
  onClose: () => void;
  onSave: () => void;
}
export function ObjectEditor({
  editor: o,
  catalog,
  lookups,
  busy,
  error,
  onChange,
  onClose,
  onSave,
}: Props) {
  const a = { ...emptyAttributes(), ...o.attributes };
  const oracle = o.kind === "database" && o.dbms === "Oracle Database";
  const field = (key: keyof CatalogObject, value: unknown) =>
    onChange({ ...o, [key]: value });
  const attr = <K extends keyof Attributes>(key: K, value: Attributes[K]) =>
    onChange({ ...o, attributes: { ...a, [key]: value } });
  const choice = (
    key: keyof CatalogObject,
    category: string,
    required = false,
  ) => (
    <Choice
      value={String(o[key] ?? "")}
      options={lookups[category] ?? []}
      onChange={(v) => field(key, v)}
      required={required}
    />
  );
  const suggested = (
    key: "location" | "serviceOrganization" | "supervisor" | "responsible",
  ) => (
    <SuggestedInput
      value={a[key]}
      options={lookups[key] ?? []}
      onChange={(v) => attr(key, v)}
    />
  );
  return (
    <div className="modal-backdrop">
      <section
        className="modal object-editor"
        role="dialog"
        aria-modal="true"
        aria-label="Редактирование объекта"
      >
        <h2>
          {o.id ? "Изменить" : "Добавить"}:{" "}
          {
            {
              server: "сервер",
              instance: "экземпляр СУБД",
              database: "базу данных",
            }[o.kind]
          }
        </h2>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            onSave();
          }}
        >
          <fieldset disabled={busy}>
            <div className="form-grid">
              <Field
                label={
                  o.kind === "database"
                    ? "SID / имя базы данных *"
                    : "Название *"
                }
              >
                <input
                  required
                  maxLength={200}
                  value={o.name}
                  onChange={(e) => field("name", e.target.value)}
                />
              </Field>
              <Field label="Состояние *">
                {choice("status", "status", true)}
              </Field>
              <Field label="Тип эксплуатации">
                {choice("environment", "environment")}
              </Field>
              {o.kind === "database" && (
                <Field label="Размещение БД">
                  <select
                    value={oracle ? "oracle" : "instance"}
                    onChange={(e) =>
                      onChange({
                        ...o,
                        dbms:
                          e.target.value === "oracle" ? "Oracle Database" : "",
                        dbmsVersion: "",
                        port: null,
                        parentId: "",
                      })
                    }
                  >
                    <option value="instance">В экземпляре СУБД</option>
                    <option value="oracle">Oracle Database — на сервере</option>
                  </select>
                </Field>
              )}
              {o.kind !== "server" && (
                <Field
                  label={
                    o.kind === "instance" || oracle
                      ? "Сервер *"
                      : "Экземпляр СУБД *"
                  }
                >
                  <select
                    required
                    value={o.parentId}
                    onChange={(e) => field("parentId", e.target.value)}
                  >
                    <option value="">Выберите объект</option>
                    {(o.kind === "instance" || oracle
                      ? catalog.servers
                      : catalog.instances
                    ).map((p) => (
                      <option key={p.id} value={p.id}>
                        {objectPath(p, catalog)}
                      </option>
                    ))}
                  </select>
                </Field>
              )}
              {o.kind === "server" && (
                <>
                  <Field label="IPv4 / сетевой адрес">
                    <input
                      value={o.address}
                      onChange={(e) => field("address", e.target.value)}
                    />
                  </Field>
                  <Field label="Подключение к серверу">
                    <Choice
                      value={a.connectionMethod}
                      options={lookups.connectionMethod ?? []}
                      onChange={(v) => attr("connectionMethod", v)}
                    />
                  </Field>
                  <Field label="Доменное членство">
                    <Choice
                      value={a.domain}
                      options={lookups.domain ?? []}
                      onChange={(v) => attr("domain", v)}
                    />
                  </Field>
                  <Field label="Месторасположение">
                    {suggested("location")}
                  </Field>
                  <Field label="Операционная система">
                    <input
                      value={o.os}
                      onChange={(e) => field("os", e.target.value)}
                    />
                  </Field>
                  <Field label="Версия системы">
                    <input
                      value={a.osVersion}
                      onChange={(e) => attr("osVersion", e.target.value)}
                    />
                  </Field>
                  <Field label="Семейство системы">
                    <input
                      value={a.osFamily}
                      onChange={(e) => attr("osFamily", e.target.value)}
                    />
                  </Field>
                </>
              )}
              {o.kind === "instance" && (
                <>
                  <Field label="Тип СУБД *">
                    <Choice
                      value={o.dbms}
                      options={(lookups.dbms ?? []).filter(
                        (v) => v !== "Oracle Database",
                      )}
                      onChange={(v) => field("dbms", v)}
                      required
                    />
                  </Field>
                  <Field label="Версия СУБД">
                    <input
                      value={o.dbmsVersion}
                      onChange={(e) => field("dbmsVersion", e.target.value)}
                    />
                  </Field>
                  <Field label="Порт">
                    <input
                      type="number"
                      min={1}
                      max={65535}
                      value={o.port ?? ""}
                      onChange={(e) =>
                        field(
                          "port",
                          e.target.value ? Number(e.target.value) : null,
                        )
                      }
                    />
                  </Field>
                </>
              )}
              {o.kind === "database" && (
                <>
                  {oracle && (
                    <Field label="Порт Oracle">
                      <input
                        type="number"
                        min={1}
                        max={65535}
                        value={o.port ?? ""}
                        onChange={(e) =>
                          field(
                            "port",
                            e.target.value ? Number(e.target.value) : null,
                          )
                        }
                      />
                    </Field>
                  )}
                  {oracle && (
                    <Field label="Версия Oracle Database">
                      <input
                        value={o.dbmsVersion}
                        onChange={(e) => field("dbmsVersion", e.target.value)}
                      />
                    </Field>
                  )}
                  <Field label="Информационная система">
                    <input
                      value={a.informationSystem}
                      maxLength={1000}
                      onChange={(e) =>
                        attr("informationSystem", e.target.value)
                      }
                    />
                  </Field>
                  <Field label="Назначение">
                    <input
                      value={o.purpose}
                      onChange={(e) => field("purpose", e.target.value)}
                    />
                  </Field>
                  <Field label="Персональные данные">
                    <SuggestedInput
                      value={a.personalData}
                      options={["Да", "Нет"]}
                      onChange={(v) => attr("personalData", v)}
                    />
                  </Field>
                </>
              )}
              <Field label="Обслуживающая организация">
                {suggested("serviceOrganization")}
              </Field>
              <Field label="Сопровождение (руководитель)">
                {suggested("supervisor")}
              </Field>
              <Field label="Ответственное лицо">
                {suggested("responsible")}
              </Field>
            </div>
            {o.kind === "database" && (
              <fieldset className="multi-select">
                <legend>Работа АБД — можно выбрать несколько</legend>
                {(lookups.administration ?? []).map((v) => (
                  <label key={v} className="check">
                    <input
                      type="checkbox"
                      checked={a.administration.includes(v)}
                      onChange={(e) =>
                        attr(
                          "administration",
                          e.target.checked
                            ? [...a.administration, v]
                            : a.administration.filter((x) => x !== v),
                        )
                      }
                    />
                    {v}
                  </label>
                ))}
              </fieldset>
            )}
            <Field label="Описание">
              <textarea
                maxLength={20000}
                rows={3}
                value={o.description}
                onChange={(e) => field("description", e.target.value)}
              />
            </Field>
            <div className="actions">
              <button type="button" onClick={onClose}>
                Отмена
              </button>
              <button className="primary">Сохранить</button>
            </div>
          </fieldset>
        </form>
        {error && (
          <p className="error notice" role="alert">
            {error}
          </p>
        )}
      </section>
    </div>
  );
}
