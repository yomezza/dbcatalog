import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  api,
  emptyCatalog,
  newObject,
  type Connection,
  type User,
  type Status,
  type Catalog,
  type CatalogObject,
  type HistoryEntry,
} from "./api";
import { visibleServers, pageOf } from "./catalog";
import "./style.css";
import { Field, Stat, UserRow } from "./components";
import { ConnectionPanel } from "./ConnectionPanel";
import { AuthPanel } from "./AuthPanel";

const labels = {
  server: "Сервер",
  instance: "Экземпляр СУБД",
  database: "База данных",
};
const roleLabels = {
  admin: "Администратор",
  editor: "Редактор",
  viewer: "Наблюдатель",
};
const fieldLabels: Record<string, string> = {
  name: "Название",
  address: "Адрес",
  os: "ОС",
  environment: "Среда",
  status: "Статус",
  description: "Описание",
  dbms: "СУБД",
  dbmsVersion: "Версия СУБД",
  port: "Порт",
  purpose: "Назначение",
  parentId: "Родительский объект",
  version: "Версия записи",
};
const initialConnection: Connection = {
  host: "localhost",
  port: 5432,
  database: "catalogdb",
  username: "catalogdb",
  schema: "dbcatalog",
  sslMode: "verify-full",
  sslRootCert: "",
};
function App() {
  const [view, setView] = useState<"connection" | "auth" | "catalog" | "users">(
    "connection",
  );
  const [connection, setConnection] = useState<Connection>(initialConnection),
    [password, setPassword] = useState(""),
    [hasPassword, setHasPassword] = useState(false),
    [useSaved, setUseSaved] = useState(false),
    [remember, setRemember] = useState(true);
  const [status, setStatus] = useState<Status>({
    connected: false,
    schema: "",
    needsAdmin: false,
    user: null,
  });
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  const [login, setLogin] = useState(""),
    [userPassword, setUserPassword] = useState(""),
    [register, setRegister] = useState(false);
  const [catalog, setCatalog] = useState<Catalog>(emptyCatalog),
    [selected, setSelected] = useState<CatalogObject | null>(null),
    [editor, setEditor] = useState<CatalogObject | null>(null),
    [deleteTarget, setDeleteTarget] = useState<CatalogObject | null>(null);
  const [query, setQuery] = useState(""),
    [page, setPage] = useState(0),
    [users, setUsers] = useState<User[]>([]),
    [history, setHistory] = useState<HistoryEntry[] | null>(null);
  async function run(work: () => Promise<void>) {
    setBusy(true);
    setError("");
    setMessage("");
    try {
      await work();
    } catch (e) {
      setError(String(e instanceof Error ? e.message : e));
    } finally {
      setBusy(false);
    }
  }
  useEffect(() => {
    void run(async () => {
      const saved = await api().GetConnection();
      setConnection(saved.connection);
      setHasPassword(saved.hasPassword);
      setUseSaved(saved.hasPassword);
      if (saved.hasPassword && !(await api().IsSetupMode())) {
        const result = await api().Connect(saved.connection, "", true, false);
        setStatus(result);
        setView("auth");
      }
    });
  }, []);
  async function loadCatalog() {
    const result = await api().GetCatalog();
    setCatalog(result);
    return result;
  }
  function updateConnection<K extends keyof Connection>(
    key: K,
    value: Connection[K],
  ) {
    setConnection((c) => ({ ...c, [key]: value }));
    setUseSaved(false);
    setHasPassword(false);
  }
  const canEdit =
    status.user?.role === "admin" || status.user?.role === "editor";
  const matches = visibleServers(catalog, query);
  const lastPage = Math.max(0, Math.ceil(matches.length / 10) - 1);
  const currentPage = Math.min(page, lastPage);
  function choose(o: CatalogObject) {
    setSelected(o);
    setHistory(null);
  }
  function node(o: CatalogObject) {
    return (
      <button
        className={"tree-node " + (selected?.id === o.id ? "selected" : "")}
        onClick={() => choose(o)}
      >
        <span className={"node-icon " + o.kind}>
          {o.kind === "server" ? "▤" : o.kind === "instance" ? "◈" : "▱"}
        </span>
        <span>
          {o.name}
          <small>
            {o.kind === "server"
              ? o.address
              : o.kind === "instance"
                ? o.dbms
                : o.purpose}
          </small>
        </span>
      </button>
    );
  }
  const connectionSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    void run(async () => {
      const result = await api().Connect(
        connection,
        password,
        useSaved,
        remember,
      );
      setStatus(result);
      setPassword("");
      setUserPassword("");
      setRegister(false);
      setCatalog(emptyCatalog);
      setSelected(null);
      setView("auth");
    });
  };
  const authSubmit = (e: React.FormEvent<HTMLFormElement>) => {
    e.preventDefault();
    void run(async () => {
      if (status.needsAdmin || register) {
        await api().Register(login, userPassword, status.needsAdmin);
        const fresh = await api().GetStatus();
        setStatus(fresh);
        setUserPassword("");
        setRegister(false);
        setMessage(
          "Учётная запись создана. " +
            (status.needsAdmin
              ? "Войдите в приложение."
              : "Ожидайте подтверждения администратора."),
        );
      } else {
        const user = await api().Login(login, userPassword);
        setStatus((s) => ({ ...s, user }));
        setUserPassword("");
        await loadCatalog();
        setView("catalog");
      }
    });
  };
  return (
    <>
      <header className="titlebar">
        <span className="brand">▦</span>
        <strong>CatalogDB</strong>
        <span className="title-muted">Каталог инфраструктуры баз данных</span>
        <div className="window-controls">
          <button
            title="Свернуть"
            onClick={() => window.runtime?.WindowMinimise()}
          >
            ─
          </button>
          <button
            title="Развернуть"
            onClick={() => window.runtime?.WindowToggleMaximise()}
          >
            □
          </button>
          <button title="Закрыть" onClick={() => window.runtime?.Quit()}>
            ×
          </button>
        </div>
      </header>
      <div className="shell">
        <aside>
          <div className="workspace-label">РАБОЧАЯ ОБЛАСТЬ</div>
          <nav>
            <button
              disabled={!status.user || busy}
              className={view === "catalog" ? "active" : ""}
              onClick={() => setView("catalog")}
            >
              ▦ Общий каталог
            </button>
            {status.user?.role === "admin" && (
              <button
                disabled={busy}
                className={view === "users" ? "active" : ""}
                onClick={() =>
                  void run(async () => {
                    setUsers(await api().ListUsers());
                    setView("users");
                  })
                }
              >
                ♙ Пользователи
              </button>
            )}
            <button
              disabled={busy}
              className={view === "connection" ? "active" : ""}
              onClick={() => {
                setPassword("");
                setView("connection");
              }}
            >
              ⚙ Подключение
            </button>
          </nav>
          <div className="sidebar-bottom">
            <span className={"dot " + (status.connected ? "online" : "")} />
            {status.connected ? `Схема: ${status.schema}` : "Нет подключения"}
            {status.user && (
              <>
                <p>
                  {status.user.login}
                  <small>{roleLabels[status.user.role]}</small>
                </p>
                <button
                  disabled={busy}
                  onClick={() =>
                    void run(async () => {
                      await api().Logout();
                      setStatus((s) => ({ ...s, user: null }));
                      setCatalog(emptyCatalog);
                      setSelected(null);
                      setEditor(null);
                      setView("auth");
                    })
                  }
                >
                  Выйти
                </button>
              </>
            )}
          </div>
        </aside>
        <main aria-busy={busy}>
          {error && (
            <div className="notice error" role="alert">
              {error}
            </div>
          )}
          {message && (
            <div className="notice success" role="status">
              {message}
            </div>
          )}
          {busy && (
            <div className="loading" role="status">
              Выполняется операция…
            </div>
          )}
          {view === "connection" && (
            <ConnectionPanel
              connection={connection}
              password={password}
              hasPassword={hasPassword}
              useSaved={useSaved}
              remember={remember}
              busy={busy}
              updateConnection={updateConnection}
              setPassword={setPassword}
              setUseSaved={setUseSaved}
              setRemember={setRemember}
              onSubmit={connectionSubmit}
            />
          )}
          {view === "auth" && (
            <AuthPanel
              status={status}
              register={register}
              login={login}
              userPassword={userPassword}
              busy={busy}
              setLogin={setLogin}
              setUserPassword={setUserPassword}
              setRegister={setRegister}
              onSubmit={authSubmit}
            />
          )}
          {view === "catalog" && (
            <>
              <div className="page-heading">
                <div>
                  <div className="eyebrow">ИНФРАСТРУКТУРА</div>
                  <h1>Общий каталог</h1>
                  <p className="muted">
                    Серверы, экземпляры СУБД и базы данных
                  </p>
                </div>
                <div className="actions">
                  <button
                    disabled={busy}
                    onClick={() =>
                      void run(async () => {
                        const c = await loadCatalog();
                        if (selected)
                          setSelected(
                            [...c.servers, ...c.instances, ...c.databases].find(
                              (o) => o.id === selected.id,
                            ) ?? null,
                          );
                      })
                    }
                  >
                    ↻ Обновить
                  </button>
                  {canEdit && (
                    <button
                      disabled={busy}
                      className="primary"
                      onClick={() => setEditor(newObject("server"))}
                    >
                      + Добавить сервер
                    </button>
                  )}
                </div>
              </div>
              <div className="stats">
                <Stat label="Серверов" value={catalog.servers.length} />
                <Stat
                  label="Экземпляров СУБД"
                  value={catalog.instances.length}
                />
                <Stat label="Баз данных" value={catalog.databases.length} />
              </div>
              <div className="catalog-layout">
                <section className="tree-panel">
                  <div className="tree-tools">
                    <input
                      aria-label="Поиск в каталоге"
                      placeholder="Поиск по имени, IP, назначению…"
                      value={query}
                      onChange={(e) => {
                        setQuery(e.target.value);
                        setPage(0);
                      }}
                    />
                  </div>
                  <div className="tree">
                    {pageOf(matches, currentPage).map((server) => (
                      <details open key={server.id}>
                        <summary>{node(server)}</summary>
                        <div className="branch">
                          {catalog.instances
                            .filter((i) => i.parentId === server.id)
                            .map((instance) => (
                              <details open key={instance.id}>
                                <summary>{node(instance)}</summary>
                                <div className="branch">
                                  {catalog.databases
                                    .filter((d) => d.parentId === instance.id)
                                    .map((db) => (
                                      <div key={db.id}>{node(db)}</div>
                                    ))}
                                </div>
                              </details>
                            ))}
                        </div>
                      </details>
                    ))}
                    {matches.length === 0 && (
                      <div className="empty">
                        {query
                          ? "Совпадений не найдено"
                          : "Каталог пока пуст. Добавьте первый сервер."}
                      </div>
                    )}
                  </div>
                  <div className="pagination">
                    <button
                      disabled={currentPage === 0}
                      onClick={() => setPage(currentPage - 1)}
                    >
                      ←
                    </button>
                    <span>
                      Страница {currentPage + 1} из {lastPage + 1}
                    </span>
                    <button
                      disabled={currentPage === lastPage}
                      onClick={() => setPage(currentPage + 1)}
                    >
                      →
                    </button>
                  </div>
                </section>
                <section className="detail-panel">
                  {selected ? (
                    <>
                      <div className="eyebrow">{labels[selected.kind]}</div>
                      <h2>{selected.name}</h2>
                      <span className="badge">{selected.status}</span>
                      <dl>
                        {Object.entries(fieldLabels)
                          .filter(
                            ([key]) => !["version", "parentId"].includes(key),
                          )
                          .map(([key, label]) => {
                            const value = selected[key as keyof CatalogObject];
                            return value ? (
                              <React.Fragment key={key}>
                                <dt>{label}</dt>
                                <dd>{String(value)}</dd>
                              </React.Fragment>
                            ) : null;
                          })}
                      </dl>
                      <div className="detail-actions">
                        {canEdit && (
                          <>
                            <button
                              disabled={busy}
                              onClick={() => setEditor({ ...selected })}
                            >
                              Изменить
                            </button>
                            {selected.kind !== "database" && (
                              <button
                                disabled={busy}
                                onClick={() =>
                                  setEditor(
                                    newObject(
                                      selected.kind === "server"
                                        ? "instance"
                                        : "database",
                                      selected.id,
                                    ),
                                  )
                                }
                              >
                                +{" "}
                                {selected.kind === "server"
                                  ? "Экземпляр СУБД"
                                  : "База данных"}
                              </button>
                            )}
                          </>
                        )}
                        <button
                          disabled={busy}
                          onClick={() =>
                            void run(async () =>
                              setHistory(
                                await api().GetHistory(
                                  selected.kind,
                                  selected.id,
                                ),
                              ),
                            )
                          }
                        >
                          История изменений
                        </button>
                        {status.user?.role === "admin" && (
                          <button
                            className="danger"
                            disabled={busy}
                            onClick={() => setDeleteTarget(selected)}
                          >
                            Удалить
                          </button>
                        )}
                      </div>
                      {history && (
                        <div className="history">
                          <h3>История изменений</h3>
                          {history.length === 0 && <p>Нет записей.</p>}
                          {history.map((h) => (
                            <article key={h.id}>
                              <strong>
                                {(
                                  {
                                    create: "Создание",
                                    update: "Изменение",
                                    delete: "Удаление",
                                  } as Record<string, string>
                                )[h.action] ?? h.action}
                              </strong>
                              <small>
                                {h.actor} ·{" "}
                                {new Date(h.createdAt).toLocaleString("ru")}
                              </small>
                              {Object.entries(fieldLabels)
                                .filter(
                                  ([k]) =>
                                    h.before?.[k as keyof CatalogObject] !==
                                    h.after?.[k as keyof CatalogObject],
                                )
                                .map(([k, label]) => (
                                  <p key={k}>
                                    <b>{label}: </b>
                                    {String(
                                      h.before?.[k as keyof CatalogObject] ??
                                        "—",
                                    )}{" "}
                                    →{" "}
                                    {String(
                                      h.after?.[k as keyof CatalogObject] ??
                                        "—",
                                    )}
                                  </p>
                                ))}
                            </article>
                          ))}
                        </div>
                      )}
                    </>
                  ) : (
                    <div className="empty detail-empty">
                      <span>◈</span>
                      <h2>Выберите объект</h2>
                      <p>Откройте сервер, экземпляр или базу в дереве слева.</p>
                    </div>
                  )}
                </section>
              </div>
            </>
          )}
          {view === "users" && (
            <>
              <div className="page-heading">
                <div>
                  <div className="eyebrow">АДМИНИСТРИРОВАНИЕ</div>
                  <h1>Пользователи</h1>
                </div>
                <button
                  disabled={busy}
                  onClick={() =>
                    void run(async () => setUsers(await api().ListUsers()))
                  }
                >
                  ↻ Обновить
                </button>
              </div>
              <section className="table-panel">
                <table>
                  <thead>
                    <tr>
                      <th>Логин</th>
                      <th>Роль</th>
                      <th>Состояние</th>
                      <th>Действие</th>
                    </tr>
                  </thead>
                  <tbody>
                    {users.map((u) => (
                      <UserRow
                        key={u.id}
                        user={u}
                        disabled={busy || u.id === status.user?.id}
                        save={(role, state) =>
                          void run(async () => {
                            await api().UpdateUser(u.id, role, state);
                            setUsers(await api().ListUsers());
                            setMessage("Пользователь обновлён");
                          })
                        }
                      />
                    ))}
                  </tbody>
                </table>
              </section>
            </>
          )}
        </main>
      </div>
      {editor && (
        <div className="modal-backdrop">
          <section
            className="modal"
            role="dialog"
            aria-modal="true"
            aria-label="Редактирование объекта"
          >
            <h2>
              {editor.id ? "Изменить" : "Добавить"}:{" "}
              {labels[editor.kind].toLowerCase()}
            </h2>
            <form
              onSubmit={(e) => {
                e.preventDefault();
                void run(async () => {
                  const saved = await api().SaveObject(editor);
                  await loadCatalog();
                  setSelected(saved);
                  setHistory(null);
                  setEditor(null);
                  setMessage("Объект сохранён");
                });
              }}
            >
              <fieldset disabled={busy}>
                <div className="form-grid">
                  <Field label="Название *">
                    <input
                      required
                      maxLength={200}
                      value={editor.name}
                      onChange={(e) =>
                        setEditor({ ...editor, name: e.target.value })
                      }
                    />
                  </Field>
                  <Field label="Статус">
                    <input
                      value={editor.status}
                      onChange={(e) =>
                        setEditor({ ...editor, status: e.target.value })
                      }
                    />
                  </Field>
                  {editor.kind === "server" ? (
                    <>
                      <Field label="IP / сетевой адрес">
                        <input
                          value={editor.address}
                          onChange={(e) =>
                            setEditor({ ...editor, address: e.target.value })
                          }
                        />
                      </Field>
                      <Field label="Операционная система">
                        <input
                          value={editor.os}
                          onChange={(e) =>
                            setEditor({ ...editor, os: e.target.value })
                          }
                        />
                      </Field>
                      <Field label="Среда эксплуатации">
                        <input
                          value={editor.environment}
                          onChange={(e) =>
                            setEditor({
                              ...editor,
                              environment: e.target.value,
                            })
                          }
                        />
                      </Field>
                    </>
                  ) : (
                    <Field
                      label={
                        editor.kind === "instance"
                          ? "Сервер *"
                          : "Экземпляр СУБД *"
                      }
                    >
                      <select
                        required
                        value={editor.parentId}
                        onChange={(e) =>
                          setEditor({ ...editor, parentId: e.target.value })
                        }
                      >
                        <option value="">Выберите объект</option>
                        {(editor.kind === "instance"
                          ? catalog.servers
                          : catalog.instances
                        ).map((o) => (
                          <option key={o.id} value={o.id}>
                            {o.name}
                          </option>
                        ))}
                      </select>
                    </Field>
                  )}
                  {editor.kind === "instance" && (
                    <>
                      <Field label="Тип СУБД *">
                        <input
                          required
                          value={editor.dbms}
                          onChange={(e) =>
                            setEditor({ ...editor, dbms: e.target.value })
                          }
                        />
                      </Field>
                      <Field label="Версия СУБД">
                        <input
                          value={editor.dbmsVersion}
                          onChange={(e) =>
                            setEditor({
                              ...editor,
                              dbmsVersion: e.target.value,
                            })
                          }
                        />
                      </Field>
                      <Field label="Порт">
                        <input
                          type="number"
                          min={1}
                          max={65535}
                          value={editor.port ?? ""}
                          onChange={(e) =>
                            setEditor({
                              ...editor,
                              port: e.target.value
                                ? Number(e.target.value)
                                : null,
                            })
                          }
                        />
                      </Field>
                    </>
                  )}
                  {editor.kind === "database" && (
                    <Field label="Назначение / информационная система">
                      <input
                        value={editor.purpose}
                        onChange={(e) =>
                          setEditor({ ...editor, purpose: e.target.value })
                        }
                      />
                    </Field>
                  )}
                </div>
                <Field label="Описание">
                  <textarea
                    maxLength={20000}
                    rows={4}
                    value={editor.description}
                    onChange={(e) =>
                      setEditor({ ...editor, description: e.target.value })
                    }
                  />
                </Field>
                <div className="actions">
                  <button type="button" onClick={() => setEditor(null)}>
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
      )}
      {deleteTarget && (
        <div className="modal-backdrop">
          <section
            className="modal compact"
            role="dialog"
            aria-modal="true"
            aria-label="Подтверждение удаления"
          >
            <h2>Удалить «{deleteTarget.name}»?</h2>
            <p>
              Удаление необратимо. История сохранится. Если есть дочерние
              объекты или политики, операция будет отклонена.
            </p>
            <div className="actions">
              <button disabled={busy} onClick={() => setDeleteTarget(null)}>
                Отмена
              </button>
              <button
                disabled={busy}
                className="danger"
                onClick={() =>
                  void run(async () => {
                    await api().DeleteObject(
                      deleteTarget.kind,
                      deleteTarget.id,
                      deleteTarget.version,
                    );
                    await loadCatalog();
                    setSelected(null);
                    setDeleteTarget(null);
                    setMessage("Объект удалён");
                  })
                }
              >
                Удалить
              </button>
            </div>
            {error && <p className="error notice">{error}</p>}
          </section>
        </div>
      )}
    </>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
