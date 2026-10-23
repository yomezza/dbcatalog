import { useEffect, useState } from "react";
import { api, type AuditEvent, type AuditFilter, type AuditPage } from "./api";
import type { RecordTarget, RecordKind } from "./api";
import { Modal } from "./Modal";
import { Field } from "./components";
const entityLabels: Record<string, string> = {
  server: "Сервер",
  instance: "Экземпляр СУБД",
  database: "База данных",
  policy: "Политика РК",
  user: "Пользователь",
  catalog: "Каталог",
  import: "Импорт",
  export: "Экспорт",
};
const detailLabels: Record<string, string> = {
  objectName: "Объект",
  selected: "Выбрано БД",
  changed: "Изменено БД",
  servers: "Серверы",
  instances: "Экземпляры СУБД",
  databases: "Базы данных",
  policies: "Политики РК",
  sourceVersion: "Выбранная версия",
  previousVersion: "Предыдущая версия",
  resultVersion: "Созданная версия",
  reason: "Причина",
  from: "Прежний путь",
  to: "Новый путь",
  attachmentId: "Идентификатор вложения",
  fileName: "Имя файла",
  size: "Размер в байтах",
  name: "Название",
  version: "Версия",
};
const initial: AuditFilter = {
  query: "",
  actorId: "",
  action: "",
  entity: "",
  from: "",
  to: "",
  page: 0,
  pageSize: 25,
};
export function dateBoundary(date: string, exclusiveEnd = false): string {
  if (!date) return "";
  const [y, m, d] = date.split("-").map(Number);
  const value = new Date(y, m - 1, d);
  if (exclusiveEnd) value.setDate(value.getDate() + 1);
  return value.toISOString();
}
export function AuditPanel({
  busy,
  run,
  onRecordHistory,
  onRecordVersions,
}: {
  busy: boolean;
  run: (work: () => Promise<void>) => Promise<void>;
  onRecordHistory: (target: RecordTarget) => void;
  onRecordVersions: (target: RecordTarget) => void;
}) {
  const [query, setQuery] = useState(""),
    [actor, setActor] = useState(""),
    [action, setAction] = useState(""),
    [entity, setEntity] = useState(""),
    [from, setFrom] = useState(""),
    [to, setTo] = useState(""),
    [data, setData] = useState<AuditPage | null>(null),
    [applied, setApplied] = useState(initial),
    [selected, setSelected] = useState<AuditEvent | null>(null);
  const load = (f: AuditFilter) =>
    run(async () => {
      const next = await api().GetAudit(f);
      setData(next);
      setApplied({ ...f, page: next.page });
      setSelected(null);
    });
  useEffect(() => {
    void load(initial);
  }, []);
  const pages = Math.max(
    1,
    Math.ceil((data?.total ?? 0) / (data?.pageSize ?? 25)),
  );
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">АДМИНИСТРИРОВАНИЕ</div>
          <h1>Журнал аудита</h1>
          <p className="muted">
            Действия пользователей, изменения каталога и административные
            операции.
          </p>
        </div>
        <button disabled={busy} onClick={() => void load(applied)}>
          ↻ Обновить
        </button>
      </div>
      <form
        className="audit-filters"
        onSubmit={(e) => {
          e.preventDefault();
          void load({
            query,
            actorId: actor,
            action,
            entity,
            from: dateBoundary(from),
            to: dateBoundary(to, true),
            page: 0,
            pageSize: 25,
          });
        }}
      >
        <fieldset disabled={busy}>
          <div className="audit-filter-grid">
            <Field label="Поиск в аудите">
              <input
                value={query}
                onChange={(e) => setQuery(e.target.value)}
                placeholder="Имя | действие | объект"
              />
            </Field>
            <Field label="Пользователь">
              <select value={actor} onChange={(e) => setActor(e.target.value)}>
                <option value="">Все пользователи</option>
                {data?.actors.map((a) => (
                  <option key={a.id} value={a.id}>
                    {a.login}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Действие">
              <select
                value={action}
                onChange={(e) => setAction(e.target.value)}
              >
                <option value="">Все действия</option>
                {Object.entries(data?.actions ?? {}).map(([key, label]) => (
                  <option key={key} value={key}>
                    {label}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Тип объекта">
              <select
                value={entity}
                onChange={(e) => setEntity(e.target.value)}
              >
                <option value="">Все типы</option>
                {Object.entries(entityLabels).map(([key, label]) => (
                  <option key={key} value={key}>
                    {label}
                  </option>
                ))}
              </select>
            </Field>
            <Field label="Дата с">
              <input
                type="date"
                value={from}
                onChange={(e) => setFrom(e.target.value)}
              />
            </Field>
            <Field label="Дата по включительно">
              <input
                type="date"
                value={to}
                min={from}
                onChange={(e) => setTo(e.target.value)}
              />
            </Field>
          </div>
          <div className="actions">
            <button
              type="button"
              onClick={() => {
                setQuery("");
                setActor("");
                setAction("");
                setEntity("");
                setFrom("");
                setTo("");
                void load(initial);
              }}
            >
              Сбросить
            </button>
            <button className="primary">Применить фильтры</button>
          </div>
        </fieldset>
      </form>
      <section className="table-panel audit-table">
        <table>
          <thead>
            <tr>
              <th>Дата и время</th>
              <th>Пользователь</th>
              <th>Действие</th>
              <th>Объект</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {data?.events.map((e) => (
              <tr key={e.id}>
                <td>{new Date(e.createdAt).toLocaleString("ru")}</td>
                <td>{e.actor}</td>
                <td>{e.actionLabel}</td>
                <td>
                  {entityLabels[e.entity] ?? e.entity}
                  {e.objectName && <> · {e.objectName}</>}
                </td>
                <td>
                  <button onClick={() => setSelected(e)}>Подробнее</button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {data?.total === 0 && <div className="empty">События не найдены.</div>}
        <div className="pagination">
          <button
            disabled={busy || !data || data.page === 0}
            onClick={() =>
              void load({ ...applied, page: (data?.page ?? 0) - 1 })
            }
          >
            ←
          </button>
          <span>
            Событий: {data?.total ?? 0} · Страница {(data?.page ?? 0) + 1} из{" "}
            {pages}
          </span>
          <button
            disabled={busy || !data || data.page >= pages - 1}
            onClick={() =>
              void load({ ...applied, page: (data?.page ?? 0) + 1 })
            }
          >
            →
          </button>
        </div>
      </section>
      {selected && (
        <Modal
          title="Подробности события"
          onClose={() => setSelected(null)}
          className="compact audit-detail"
        >
          <div className="eyebrow">СОБЫТИЕ № {selected.id}</div>
          <h2>{selected.actionLabel}</h2>
          <dl className="settings-fields">
            <dt>Пользователь</dt>
            <dd>{selected.actor}</dd>
            <dt>Время</dt>
            <dd>{new Date(selected.createdAt).toLocaleString("ru")}</dd>
            <dt>Объект</dt>
            <dd>
              {selected.objectName || entityLabels[selected.entity] || "—"}
            </dd>
            {selected.objectId && (
              <>
                <dt>Идентификатор объекта</dt>
                <dd>
                  <code>{selected.objectId}</code>
                </dd>
              </>
            )}
            {Object.entries(selected.details).map(([key, value]) => (
              <div className="audit-detail-row" key={key}>
                <dt>{detailLabels[key] ?? key}</dt>
                <dd>{String(value)}</dd>
              </div>
            ))}
          </dl>
          {selected.objectId &&
            ["server", "instance", "database", "policy"].includes(
              selected.entity,
            ) && (
              <div className="actions">
                <button
                  onClick={() => {
                    const target = {
                      kind: selected.entity as RecordKind,
                      id: selected.objectId,
                      name:
                        selected.objectName || entityLabels[selected.entity],
                    };
                    setSelected(null);
                    onRecordHistory(target);
                  }}
                >
                  История объекта
                </button>
                <button
                  onClick={() => {
                    const target = {
                      kind: selected.entity as RecordKind,
                      id: selected.objectId,
                      name:
                        selected.objectName || entityLabels[selected.entity],
                    };
                    setSelected(null);
                    onRecordVersions(target);
                  }}
                >
                  Версии объекта
                </button>
              </div>
            )}
        </Modal>
      )}
    </>
  );
}
