import { ObjectPicker } from "./ui/ObjectPicker";
import { PageNavigation } from "./ui/PageNavigation";
import { useEffect, useState } from "react";
import {
  api,
  type PolicyPage,
  type Catalog,
  type CatalogObject,
  type Policy,
  type Lookups,
} from "./api";
import { Field } from "./components";
import { Choice, SuggestedInput } from "./ObjectEditor";
export function effectivePolicy(object: CatalogObject, policies: Policy[]) {
  const direct = policies.find(
    (p) => p.kind === object.kind && p.objectId === object.id,
  );
  const inherited =
    !direct && object.kind === "database" && object.dbms !== "Oracle Database"
      ? policies.find(
          (p) => p.kind === "instance" && p.objectId === object.parentId,
        )
      : undefined;
  return { policy: direct ?? inherited, inherited: !!inherited };
}
export function BackupCard({
  object,
  policies,
  canEdit,
  onEdit,
}: {
  object: CatalogObject;
  policies: Policy[];
  canEdit: boolean;
  onEdit: (o: CatalogObject) => void;
}) {
  const { policy, inherited } = effectivePolicy(object, policies);
  return (
    <section className="backup-card">
      <h3>Политика резервного копирования</h3>
      {policy ? (
        <>
          <span className="badge">
            {policy.enabled ? "Включен" : "Отключен"}
          </span>
          <p>
            <b>{policy.name}</b>
            {inherited
              ? " · Унаследована от экземпляра"
              : " · Назначена непосредственно"}
          </p>
          <dl>
            <dt>Тип РК</dt>
            <dd>{policy.backupType}</dd>
            <dt>Планировщик</dt>
            <dd>{policy.scheduler}</dd>
            <dt>Расписание</dt>
            <dd>{policy.schedule || "—"}</dd>
            <dt>Хранилище</dt>
            <dd>{policy.storage || "—"}</dd>
            <dt>Срок хранения</dt>
            <dd>{policy.retentionDays} дней</dd>
          </dl>
          {object.kind === "database" &&
            object.dbms !== "Oracle Database" &&
            !inherited &&
            !policy.enabled && (
              <p className="muted">
                Отключённая политика БД подавляет наследование политики
                экземпляра.
              </p>
            )}
        </>
      ) : (
        <p className="muted">Политика не назначена.</p>
      )}
      {canEdit && (
        <button onClick={() => onEdit(object)}>
          {inherited
            ? "Назначить свою политику"
            : policy
              ? "Изменить политику"
              : "Добавить политику РК"}
        </button>
      )}
    </section>
  );
}
export function PoliciesPanel({
  revision,
  busy,
  canEdit,
  canDelete,
  onEdit,
  onDelete,
  onCreate,
  onHistory,
  onVersions,
}: {
  revision: number;
  busy: boolean;
  canEdit: boolean;
  canDelete: boolean;
  onEdit: (p: Policy) => void;
  onDelete: (p: Policy) => void;
  onCreate: () => void;
  onHistory: (p: Policy) => void;
  onVersions: (p: Policy) => void;
}) {
  const [query, setQuery] = useState(""),
    [page, setPage] = useState(0),
    [data, setData] = useState<PolicyPage | null>(null),
    [loadError, setLoadError] = useState(""),
    [loading, setLoading] = useState(false);
  useEffect(() => {
    let live = true;
    setLoading(true);
    const timer = setTimeout(() => {
      void api()
        .GetPolicyPage(query, page)
        .then((result) => {
          if (live) {
            setData(result);
            setLoadError("");
          }
        })
        .catch((e) => {
          if (live) setLoadError(String(e));
        })
        .finally(() => {
          if (live) setLoading(false);
        });
    }, 200);
    return () => {
      live = false;
      clearTimeout(timer);
    };
  }, [query, page, revision]);
  const filtered = data?.items ?? [];
  const target = (p: Policy) => data?.paths[p.id] ?? p.objectId;
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">СОПРОВОЖДЕНИЕ</div>
          <h1>Политики резервного копирования</h1>
          <p className="muted">
            Учёт настроек резервного копирования экземпляров и отдельных БД.
          </p>
        </div>
        {canEdit && (
          <button disabled={busy} className="primary" onClick={onCreate}>
            + Добавить политику
          </button>
        )}
      </div>
      <div className="info">
        Политика БД имеет приоритет над политикой экземпляра, включая
        отключённую политику. CatalogDB учитывает настройки и не запускает
        резервное копирование. Для Oracle политика назначается непосредственно
        базе.
      </div>
      <div className="tree-tools">
        <input
          aria-label="Поиск политик"
          placeholder="Поиск по названию или объекту"
          value={query}
          onChange={(e) => {
            setQuery(e.target.value);
            setPage(0);
          }}
        />
      </div>
      {loadError && (
        <p className="notice error" role="alert">
          {loadError}
        </p>
      )}
      {loading && <p role="status">Поиск политик по всему каталогу…</p>}
      <section className="table-panel">
        {filtered.length === 0 ? (
          <div className="empty">Политик пока нет.</div>
        ) : (
          <table>
            <thead>
              <tr>
                <th>Политика</th>
                <th>Объект</th>
                <th>Тип РК</th>
                <th>Статус</th>
                <th>Действия</th>
              </tr>
            </thead>
            <tbody>
              {filtered.map((p) => (
                <tr key={p.id}>
                  <td>{p.name}</td>
                  <td>
                    {p.kind === "instance" ? "Экземпляр" : "БД"}: {target(p)}
                  </td>
                  <td>{p.backupType}</td>
                  <td>{p.enabled ? "Включен" : "Отключен"}</td>
                  <td>
                    <div className="actions">
                      {canEdit && (
                        <button
                          disabled={busy}
                          onClick={() => onEdit({ ...p })}
                        >
                          Изменить
                        </button>
                      )}
                      <button disabled={busy} onClick={() => onHistory(p)}>
                        История
                      </button>
                      {canDelete && (
                        <button disabled={busy} onClick={() => onVersions(p)}>
                          Версии записи
                        </button>
                      )}
                      {canDelete && (
                        <button
                          disabled={busy}
                          className="danger"
                          onClick={() => onDelete(p)}
                        >
                          Удалить
                        </button>
                      )}
                    </div>
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        )}
      </section>
      {data && (
        <PageNavigation
          page={data.page}
          total={data.total}
          busy={busy || loading}
          onPage={setPage}
        />
      )}
    </>
  );
}
export function PolicyEditor({
  policy: p,
  lookups,
  busy,
  error,
  onChange,
  onClose,
  onSave,
}: {
  policy: Policy;
  catalog: Catalog;
  lookups: Lookups;
  busy: boolean;
  error: string;
  onChange: (p: Policy) => void;
  onClose: () => void;
  onSave: () => void;
}) {
  const set = <K extends keyof Policy>(key: K, value: Policy[K]) =>
    onChange({ ...p, [key]: value });
  return (
    <div className="modal-backdrop">
      <section
        className="modal"
        role="dialog"
        aria-modal="true"
        aria-label="Политика резервного копирования"
      >
        <h2>{p.id ? "Изменить" : "Добавить"} политику РК</h2>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            onSave();
          }}
        >
          <fieldset disabled={busy}>
            <div className="form-grid">
              <Field label="Название *">
                <input
                  required
                  maxLength={200}
                  value={p.name}
                  onChange={(e) => set("name", e.target.value)}
                />
              </Field>
              <Field label="Статус">
                <select
                  value={String(p.enabled)}
                  onChange={(e) => set("enabled", e.target.value === "true")}
                >
                  <option value="false">Отключен</option>
                  <option value="true">Включен</option>
                </select>
              </Field>
              <Field label="Уровень назначения">
                <select
                  value={p.kind}
                  onChange={(e) =>
                    onChange({
                      ...p,
                      kind: e.target.value as Policy["kind"],
                      objectId: "",
                    })
                  }
                >
                  <option value="instance">Экземпляр СУБД</option>
                  <option value="database">База данных</option>
                </select>
              </Field>
              <Field label="Объект назначения *">
                <ObjectPicker
                  kind={p.kind}
                  value={p.objectId}
                  label="Объект назначения *"
                  disabled={busy}
                  onChange={(option) => set("objectId", option.object.id)}
                />
              </Field>
              <Field label="Тип РК">
                <Choice
                  value={p.backupType}
                  options={["Не указано", ...(lookups.backupType ?? [])]}
                  onChange={(v) => set("backupType", v)}
                />
              </Field>
              <Field label="Служба планировщика заданий">
                <SuggestedInput
                  value={p.scheduler}
                  options={[
                    "Не указано",
                    "cron",
                    "Windows Task Scheduler",
                    "SQL Server Agent",
                    "pgAgent",
                  ]}
                  onChange={(v) => set("scheduler", v)}
                />
              </Field>
              <Field label="Расписание">
                <input
                  value={p.schedule}
                  onChange={(e) => set("schedule", e.target.value)}
                  placeholder="Например: ежедневно в 02:00"
                  required={p.enabled}
                />
              </Field>
              <Field label="Путь / место хранения РК">
                <input
                  value={p.storage}
                  onChange={(e) => set("storage", e.target.value)}
                  required={p.enabled}
                />
              </Field>
              <Field label="Срок хранения РК, дней">
                <input
                  type="number"
                  min={p.enabled ? 1 : 0}
                  max={36500}
                  required
                  value={p.retentionDays}
                  onChange={(e) => set("retentionDays", Number(e.target.value))}
                />
              </Field>
            </div>
            <Field label="Примечания">
              <textarea
                maxLength={20000}
                value={p.description}
                onChange={(e) => set("description", e.target.value)}
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
          <div className="notice error" role="alert">
            {error}
          </div>
        )}
      </section>
    </div>
  );
}
