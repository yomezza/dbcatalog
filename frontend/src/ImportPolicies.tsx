import { useState } from "react";
import {
  api,
  newPolicy,
  type ImportPreview,
  type PolicyImportConfig,
  type Policy,
  type ImportPolicyCandidate,
} from "./api";
import { Modal } from "./Modal";
export function ImportPolicies({
  preview,
  busy,
  run,
  onChange,
  onDirty,
}: {
  preview: ImportPreview;
  busy: boolean;
  run: (task: () => Promise<void>) => Promise<void>;
  onChange: (p: ImportPreview) => void;
  onDirty: (v: boolean) => void;
}) {
  const [config, setConfig] = useState<PolicyImportConfig>({
      enabled: false,
      targetKind: "",
      defaultState: "",
      retentionUnit: "",
      defaults: newPolicy("database", ""),
      overrides: [],
    }),
    [dirty, setDirty] = useState(false),
    [editing, setEditing] = useState<ImportPolicyCandidate | null>(null),
    [page, setPage] = useState(0);
  function update(c: PolicyImportConfig) {
    setConfig(c);
    setDirty(true);
    onDirty(true);
  }
  const candidates = preview.policies ?? [],
    last = Math.max(0, Math.ceil(candidates.length / 25) - 1),
    current = Math.min(page, last);
  return (
    <section className="policy-import">
      <h3>Перенос политик РК</h3>
      <p className="muted">
        Первые 21 колонка — каталог; колонки 22–26 — параметры РК. Пароли архива
        не сохраняются. Пустые строки РК оставляют существующие политики без
        изменений. Общие значения ниже применяются только к отсутствующим
        параметрам.
      </p>
      <label className="check">
        <input
          disabled={busy}
          type="checkbox"
          checked={config.enabled}
          onChange={(e) => update({ ...config, enabled: e.target.checked })}
        />
        Переносить политики РК вместе с каталогом
      </label>
      {config.enabled && (
        <fieldset disabled={busy}>
          <div className="form-grid">
            <label className="field">
              <span>Уровень назначения</span>
              <select
                aria-label="Уровень назначения"
                value={config.targetKind}
                onChange={(e) =>
                  update({ ...config, targetKind: e.target.value })
                }
              >
                <option value="">Выберите явно</option>
                <option value="database">База данных</option>
                <option value="instance">
                  Экземпляр (не подходит для Oracle)
                </option>
              </select>
            </label>
            <label className="field">
              <span>Пустой статус СРК</span>
              <select
                aria-label="Пустой статус СРК"
                value={config.defaultState}
                onChange={(e) =>
                  update({ ...config, defaultState: e.target.value })
                }
              >
                <option value="">Считать ошибкой</option>
                <option value="enabled">Включить</option>
                <option value="disabled">Отключить</option>
              </select>
            </label>
            <label className="field">
              <span>Единицы числового срока из Excel</span>
              <select
                aria-label="Единицы числового срока из Excel"
                value={config.retentionUnit}
                onChange={(e) =>
                  update({ ...config, retentionUnit: e.target.value })
                }
              >
                <option value="">Требовать уточнения</option>
                <option value="days">Дни</option>
                <option value="weeks">Недели</option>
              </select>
            </label>
            {(
              [
                "schedule",
                "backupType",
                "scheduler",
                "storage",
                "retentionDays",
              ] as const
            ).map((k) => (
              <label className="field" key={k}>
                <span>
                  {
                    {
                      schedule: "Расписание, если отсутствует",
                      backupType: "Тип РК, если отсутствует",
                      scheduler: "Планировщик, если отсутствует",
                      storage: "Путь РК, если отсутствует",
                      retentionDays:
                        "Срок по умолчанию, дней (если ячейка пуста)",
                    }[k]
                  }
                </span>
                <input
                  type={k === "retentionDays" ? "number" : "text"}
                  min={0}
                  max={k === "retentionDays" ? 36500 : undefined}
                  value={config.defaults[k]}
                  onChange={(e) =>
                    update({
                      ...config,
                      defaults: {
                        ...config.defaults,
                        [k]:
                          k === "retentionDays"
                            ? Number(e.target.value)
                            : e.target.value,
                      },
                    })
                  }
                />
              </label>
            ))}
          </div>
        </fieldset>
      )}
      <button
        disabled={busy || (config.enabled && !config.targetKind)}
        onClick={() =>
          void run(async () => {
            const next = await api().PreviewExcelPolicies(
              preview.token,
              config,
            );
            onChange(next);
            setDirty(false);
            onDirty(false);
          })
        }
      >
        Проверить назначения политик
      </button>
      {dirty && (
        <p className="info">
          Параметры изменены. Повторите проверку до подтверждения импорта.
        </p>
      )}
      {!!candidates.length && (
        <>
          <p>
            Назначений в строках: {candidates.length}. Одинаковые назначения
            объединяются; противоречивые отмечаются ошибкой. Существующая
            политика заменяется параметрами предпросмотра.
          </p>
          <div className="table-scroll">
            <table>
              <thead>
                <tr>
                  <th>Строка / назначение</th>
                  <th>Действие / проверка</th>
                  <th>Было → станет</th>
                  <th />
                </tr>
              </thead>
              <tbody>
                {candidates.slice(current * 25, current * 25 + 25).map((c) => (
                  <tr key={c.row}>
                    <td>
                      {c.row}: {c.target}
                      <small>{c.policy.kind}</small>
                    </td>
                    <td>
                      {c.skipped ? "Пропустить политику" : c.action}
                      <p>{c.errors.join("; ")}</p>
                    </td>
                    <td>
                      {c.before && <PolicyText p={c.before} />}
                      <span>→</span>
                      <PolicyText p={c.policy} />
                    </td>
                    <td>
                      <button
                        disabled={busy || dirty}
                        onClick={() => setEditing(c)}
                      >
                        Исправить политику
                      </button>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
          <div className="pagination">
            <button
              disabled={busy || !current}
              onClick={() => setPage(current - 1)}
            >
              ←
            </button>
            <span>
              {current + 1} / {last + 1}
            </span>
            <button
              disabled={busy || current === last}
              onClick={() => setPage(current + 1)}
            >
              →
            </button>
          </div>
        </>
      )}
      {editing && (
        <PolicyCorrection
          candidate={editing}
          busy={busy}
          onClose={() => setEditing(null)}
          onSave={(policy, skip) => {
            update({
              ...config,
              overrides: [
                ...config.overrides.filter((o) => o.row !== editing.row),
                { row: editing.row, skip, policy },
              ],
            });
            setEditing(null);
          }}
        />
      )}
    </section>
  );
}
function PolicyText({ p }: { p: Policy }) {
  return (
    <small>
      {p.name} · {p.enabled ? "Включена" : "Отключена"} · {p.backupType} ·{" "}
      {p.scheduler} · {p.schedule || "нет расписания"} ·{" "}
      {p.storage || "нет пути"} · {p.retentionDays} дней
    </small>
  );
}
function PolicyCorrection({
  candidate,
  busy,
  onClose,
  onSave,
}: {
  candidate: ImportPolicyCandidate;
  busy: boolean;
  onClose: () => void;
  onSave: (p: Policy, skip: boolean) => void;
}) {
  const [p, setP] = useState(candidate.policy),
    [skip, setSkip] = useState(candidate.skipped);
  return (
    <Modal
      title={"Политика РК: строка " + candidate.row}
      busy={busy}
      onClose={onClose}
    >
      <p>{candidate.target}</p>
      <label className="check">
        <input
          type="checkbox"
          checked={skip}
          onChange={(e) => setSkip(e.target.checked)}
        />
        Не переносить политику этой строки (каталог импортируется)
      </label>
      <label className="field">
        <span>Назначение</span>
        <select
          value={p.kind}
          onChange={(e) =>
            setP({ ...p, kind: e.target.value as Policy["kind"] })
          }
        >
          <option value="database">База данных</option>
          <option value="instance">Экземпляр</option>
        </select>
      </label>
      <label className="check">
        <input
          type="checkbox"
          checked={p.enabled}
          onChange={(e) => setP({ ...p, enabled: e.target.checked })}
        />
        Включена
      </label>
      {(
        [
          "name",
          "backupType",
          "scheduler",
          "schedule",
          "storage",
          "retentionDays",
          "description",
        ] as const
      ).map((k) => (
        <label className="field" key={k}>
          <span>
            {
              {
                name: "Название",
                backupType: "Тип РК",
                scheduler: "Планировщик",
                schedule: "Расписание",
                storage: "Путь РК",
                retentionDays: "Срок хранения в днях",
                description: "Описание",
              }[k]
            }
          </span>
          <input
            value={p[k]}
            type={k === "retentionDays" ? "number" : "text"}
            min={0}
            onChange={(e) =>
              setP({
                ...p,
                [k]:
                  k === "retentionDays"
                    ? Number(e.target.value)
                    : e.target.value,
              })
            }
          />
        </label>
      ))}
      <button className="primary" onClick={() => onSave(p, skip)}>
        Применить к предпросмотру
      </button>
    </Modal>
  );
}
