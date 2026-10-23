import { useEffect, useState } from "react";
import { api, type Statistics, type Distribution } from "./api";
import { Stat } from "./components";
export function StatisticsPanel({
  revision = 0,
  busy,
  run,
  onSearch,
}: {
  revision?: number;
  busy: boolean;
  run: (fn: () => Promise<void>) => Promise<void>;
  onSearch: (query: string) => void;
}) {
  const [data, setData] = useState<Statistics | null>(null);
  const load = () => run(async () => setData(await api().GetStatistics()));
  useEffect(() => {
    void load();
  }, [revision]);
  function chart(title: string, rows: Distribution[], instances = false) {
    const max = Math.max(1, ...rows.map((r) => r.databases));
    return (
      <section className="settings-card distribution-card">
        <h2>{title}</h2>
        {!rows.length && <p className="muted">Нет данных</p>}
        {rows.map((r) => (
          <button
            className="distribution-row"
            key={r.name}
            onClick={() => onSearch(r.name)}
            disabled={busy || !r.name}
          >
            <span>
              {r.name || "Не указано"}
              <small>
                {instances ? `${r.instances} экз. · ` : ""}
                {r.databases} БД
              </small>
            </span>
            <span className="distribution-track">
              <span style={{ width: `${(r.databases / max) * 100}%` }} />
            </span>
          </button>
        ))}
      </section>
    );
  }
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">ОБЗОР КАТАЛОГА</div>
          <h1>Статистика</h1>
          <p className="muted">
            Показатели рассчитаны PostgreSQL
            {data
              ? ` · ${new Date(data.generatedAt).toLocaleString("ru")}`
              : ""}
            .
          </p>
        </div>
        <button disabled={busy} onClick={() => void load()}>
          ↻ Обновить
        </button>
      </div>
      {data && (
        <>
          <div className="stats">
            <Stat label="Записей серверов" value={data.servers} />
            <Stat label="Уникальных имён серверов" value={data.uniqueServers} />
            <Stat label="Пар сервер / адрес" value={data.serverAddresses} />
          </div>
          <div className="stats">
            <Stat label="Экземпляров СУБД" value={data.instances} />
            <Stat label="Баз данных" value={data.databases} />
            <Stat label="Политик РК / активных" value={data.policies} />
          </div>
          <p className="muted">
            Уникальные имена сравниваются без регистра и крайних пробелов. Пара
            «сервер / адрес» учитывает оба значения. Активных политик:{" "}
            {data.enabledPolicies}.
          </p>
          <div className="statistics-grid">
            {chart("Типы СУБД", data.dbms, true)}
            {chart("Состояния баз данных", data.statuses)}
            {chart("Среда эксплуатации БД", data.environments)}
            <section className="settings-card">
              <h2>Эффективная политика РК</h2>
              <dl className="settings-fields">
                <dt>Активная политика</dt>
                <dd>{data.protectedDatabases} БД</dd>
                <dt>Политика отключена</dt>
                <dd>{data.disabledDatabases} БД</dd>
                <dt>Политика отсутствует</dt>
                <dd>{data.unprotectedDatabases} БД</dd>
              </dl>
              <p className="muted">
                Политика БД имеет приоритет над политикой экземпляра, в том
                числе при отключении. Показатели отражают настройки каталога, а
                не результат выполнения РК.
              </p>
            </section>
          </div>
        </>
      )}
    </>
  );
}
