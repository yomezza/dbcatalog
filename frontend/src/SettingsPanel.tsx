import { version } from "../package.json";
import type { Connection, Status } from "./api";
import { ValueBadge } from "./ValueBadge";
export function SettingsPanel({
  connection,
  status,
  busy,
  onConnection,
  onClear,
}: {
  connection: Connection | null;
  status: Status;
  busy: boolean;
  onConnection: () => void;
  onClear: () => void;
}) {
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">РАБОЧЕЕ МЕСТО</div>
          <h1>Настройки</h1>
          <p className="muted">
            Подключение к центральной базе и административные операции.
          </p>
        </div>
      </div>
      <div className="settings-grid">
        <section className="settings-card">
          <div className="settings-card-heading">
            <h2>Подключение PostgreSQL</h2>
            <ValueBadge
              value={status.connected ? "Подключено" : "Не подключено"}
            />
          </div>
          {connection && (
            <dl className="settings-fields">
              <dt>Сервер</dt>
              <dd>
                {connection.host}:{connection.port}
              </dd>
              <dt>База данных</dt>
              <dd>{connection.database}</dd>
              <dt>Схема</dt>
              <dd>
                <code>{connection.schema || "dbcatalog"}</code>
              </dd>
              <dt>Пользователь PostgreSQL</dt>
              <dd>{connection.username}</dd>
              <dt>Защита соединения</dt>
              <dd>{connection.sslMode}</dd>
            </dl>
          )}
          <button disabled={busy} onClick={onConnection}>
            Настроить подключение
          </button>
        </section>
        {status.user?.role === "admin" && (
          <section className="settings-card danger-zone">
            <div className="eyebrow">АДМИНИСТРИРОВАНИЕ</div>
            <h2>Очистка каталога</h2>
            <p>
              Удалить все серверы, экземпляры СУБД, базы данных и их политики
              резервного копирования. Перед выполнением будут показаны
              количества записей и два последовательных подтверждения.
            </p>
            <p className="muted">
              Учётные записи, справочники, настройки, история изменений и аудит
              останутся.
            </p>
            <button className="danger" disabled={busy} onClick={onClear}>
              Очистить все записи каталога
            </button>
          </section>
        )}
        <p className="app-version">CatalogDB {version}</p>
      </div>
    </>
  );
}
