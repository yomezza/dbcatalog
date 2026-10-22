import { useEffect, useState } from "react";
import {
  api,
  type Catalog,
  type CatalogCriteria,
  type Lookups,
  type FilterPreset,
  type ObjectRef,
  type BulkPatch,
  type BulkPreview,
  type QualityPage,
  type DeletedPage,
  type DeletedRecord,
  type RestorePreview,
  type Kind,
} from "./api";
import { Modal } from "./Modal";
import { emptyCriteria } from "./catalog";
type Run = (task: () => Promise<void>) => Promise<void>;
export function CatalogFilters({
  value,
  onChange,
  catalog,
  lookups,
  busy,
  run,
}: {
  value: CatalogCriteria;
  onChange: (f: CatalogCriteria) => void;
  catalog: Catalog;
  lookups: Lookups;
  busy: boolean;
  run: Run;
}) {
  const [presets, setPresets] = useState<FilterPreset[]>([]),
    [name, setName] = useState("");
  useEffect(() => {
    void run(async () => setPresets(await api().ListFilterPresets()));
  }, []);
  function select(
    key: keyof CatalogCriteria,
    label: string,
    values: string[],
    extras?: { value: string; label: string }[],
  ) {
    return (
      <label className="field">
        <span>{label}</span>
        <select
          disabled={busy}
          value={value[key]}
          onChange={(e) => onChange({ ...value, [key]: e.target.value })}
        >
          <option value="">Все</option>
          {extras?.map((o) => (
            <option key={o.value} value={o.value}>
              {o.label}
            </option>
          ))}
          {[
            ...new Set([
              ...values,
              ...(value[key] && !extras?.some((o) => o.value === value[key])
                ? [value[key]]
                : []),
            ]),
          ]
            .filter(Boolean)
            .sort()
            .map((v) => (
              <option key={v} value={v}>
                {v}
              </option>
            ))}
        </select>
      </label>
    );
  }
  return (
    <details className="workflow-filters">
      <summary>Фильтры баз данных</summary>
      <div className="form-grid">
        {select("dbms", "СУБД", [
          ...catalog.instances.map((i) => i.dbms),
          ...catalog.databases.map((d) => d.dbms),
        ])}
        {select("status", "Состояние", lookups.status ?? [])}
        {select("environment", "Тип эксплуатации", [
          "Промышленный",
          "Тестовый",
        ])}
        {select(
          "responsible",
          "Ответственный",
          catalog.databases.map((d) => d.attributes.responsible),
          [{ value: "__empty__", label: "Не указан" }],
        )}
        <label className="field">
          <span>Политика РК (с учётом наследования)</span>
          <select
            value={value.policy}
            onChange={(e) => onChange({ ...value, policy: e.target.value })}
          >
            <option value="">Любая</option>
            <option value="present">Назначена</option>
            <option value="missing">Отсутствует</option>
            <option value="active">Активна</option>
            <option value="disabled">Отключена</option>
          </select>
        </label>
      </div>
      <div className="actions">
        <button disabled={busy} onClick={() => onChange(emptyCriteria())}>
          Сбросить фильтры и поиск
        </button>
        <input
          aria-label="Название набора фильтров"
          placeholder="Название набора"
          maxLength={100}
          value={name}
          onChange={(e) => setName(e.target.value)}
        />
        <button
          disabled={busy || !name.trim()}
          onClick={() =>
            void run(async () => {
              await api().SaveFilterPreset({
                id: "",
                version: 0,
                name,
                criteria: value,
              });
              setPresets(await api().ListFilterPresets());
              setName("");
            })
          }
        >
          Сохранить набор
        </button>
      </div>
      <div className="actions">
        {presets.map((p) => (
          <span key={p.id}>
            <button disabled={busy} onClick={() => onChange(p.criteria)}>
              {p.name}
            </button>
            <button
              disabled={busy}
              aria-label={"Удалить набор " + p.name}
              onClick={() =>
                void run(async () => {
                  await api().DeleteFilterPreset(p.id, p.version);
                  setPresets(await api().ListFilterPresets());
                })
              }
            >
              ×
            </button>
          </span>
        ))}
      </div>
    </details>
  );
}
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
export function QualityPanel({
  busy,
  run,
  onOpen,
}: {
  busy: boolean;
  run: Run;
  onOpen: (kind: Kind, id: string) => void;
}) {
  const [data, setData] = useState<QualityPage | null>(null),
    [code, setCode] = useState("");
  const labels: Record<string, string> = {
    missing_responsible: "Нет ответственного",
    missing_address: "Нет адреса сервера",
    production_no_policy: "Промышленная БД без политики РК",
    production_disabled_policy: "Политика РК отключена",
    duplicate_server_name: "Совпадающие имена серверов",
    duplicate_server_address: "Совпадающие адреса серверов",
    similar_sid: "Похожие SID внутри родителя",
  };
  async function load(c = code, p = 0) {
    setData(await api().GetQualityReport(c, p));
  }
  useEffect(() => {
    void run(() => load());
  }, []);
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Качество данных</h1>
          <p className="muted">
            Подозрения на дубли требуют проверки. Одинаковый SID на разных
            серверах допустим.
          </p>
        </div>
        <button disabled={busy} onClick={() => void run(() => load())}>
          Обновить
        </button>
      </div>
      <select
        aria-label="Тип проблемы"
        disabled={busy}
        value={code}
        onChange={(e) => {
          setCode(e.target.value);
          void run(() => load(e.target.value));
        }}
      >
        <option value="">Все проблемы</option>
        {Object.entries(labels).map(([k, v]) => (
          <option key={k} value={k}>
            {v} ({data?.counts[k] ?? 0})
          </option>
        ))}
      </select>
      <p>Найдено: {data?.total ?? 0}</p>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Объект</th>
              <th>Проблема</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {data?.items.map((i) => (
              <tr key={i.code + i.id}>
                <td>{i.path}</td>
                <td>{i.detail}</td>
                <td>
                  <button disabled={busy} onClick={() => onOpen(i.kind, i.id)}>
                    Открыть карточку
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
          onPage={(p) => void run(() => load(code, p))}
        />
      )}
    </>
  );
}
function Pages({
  page,
  total,
  busy,
  onPage,
}: {
  page: number;
  total: number;
  busy: boolean;
  onPage: (p: number) => void;
}) {
  return (
    <div className="pagination">
      <button disabled={busy || page === 0} onClick={() => onPage(page - 1)}>
        ←
      </button>
      <span>
        {page + 1} / {Math.max(1, Math.ceil(total / 25))} · Записей: {total}
      </span>
      <button
        disabled={busy || (page + 1) * 25 >= total}
        onClick={() => onPage(page + 1)}
      >
        →
      </button>
    </div>
  );
}
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
