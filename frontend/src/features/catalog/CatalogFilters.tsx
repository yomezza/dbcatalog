import { useEffect, useMemo, useState } from "react";
import {
  api,
  type Catalog,
  type CatalogCriteria,
  type Lookups,
  type FilterPreset,
} from "../../api";
import { emptyCriteria } from "../../catalog";
import type { AsyncTaskRunner as Run } from "../../ui/types";
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
  const options = useMemo(
    () => ({
      dbms: [
        ...new Set([
          ...catalog.instances.map((i) => i.dbms),
          ...catalog.databases.map((d) => d.dbms),
        ]),
      ],
      responsible: [
        ...new Set(catalog.databases.map((d) => d.attributes.responsible)),
      ],
    }),
    [catalog],
  );
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
        {select("dbms", "СУБД", options.dbms)}
        {select("status", "Состояние", lookups.status ?? [])}
        {select("environment", "Тип эксплуатации", [
          "Промышленный",
          "Тестовый",
        ])}
        {select("responsible", "Ответственный", options.responsible, [
          { value: "__empty__", label: "Не указан" },
        ])}
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
