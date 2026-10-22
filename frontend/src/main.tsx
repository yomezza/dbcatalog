import React, { useEffect, useState } from "react";
import { createRoot } from "react-dom/client";
import {
  api,
  emptyCatalog,
  newObject,
  newPolicy,
  type Policy,
  type Connection,
  type User,
  type Status,
  type Catalog,
  type CatalogObject,
  type HistoryEntry,
  type Lookups,
} from "./api";
import { visibleServers, pageOf } from "./catalog";
import "./style.css";
import { Stat, UserRow } from "./components";
import { ConnectionPanel } from "./ConnectionPanel";
import { AuthPanel } from "./AuthPanel";
import { BackupCard, PoliciesPanel, PolicyEditor } from "./PoliciesPanel";
import { ExcelPanel } from "./ExcelPanel";
import { ObjectEditor } from "./ObjectEditor";
import { ContextMenu } from "./ContextMenu";
import { FavoritesPanel } from "./FavoritesPanel";
import { fieldLabels, objectValue } from "./objectFields";

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
  const [view, setView] = useState<
    | "connection"
    | "auth"
    | "catalog"
    | "users"
    | "favorites"
    | "excel"
    | "policies"
  >("connection");
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
  const [policies, setPolicies] = useState<Policy[]>([]),
    [policyEditor, setPolicyEditor] = useState<Policy | null>(null),
    [policyDelete, setPolicyDelete] = useState<Policy | null>(null);
  function openPolicy(o: CatalogObject) {
    if (o.kind === "server") return;
    setError("");
    setPolicyEditor(
      policies.find((p) => p.kind === o.kind && p.objectId === o.id) ??
        newPolicy(o.kind, o.id),
    );
  }
  const [lookups, setLookups] = useState<Lookups>({});
  const [favorites, setFavorites] = useState<CatalogObject[]>([]);
  const [contextMenu, setContextMenu] = useState<{
    object: CatalogObject;
    x: number;
    y: number;
  } | null>(null);
  const isFavorite = (o: CatalogObject) =>
    favorites.some((f) => f.kind === o.kind && f.id === o.id);
  function openContext(e: React.MouseEvent, o: CatalogObject) {
    e.preventDefault();
    if (busy) return;
    choose(o);
    setContextMenu({ object: o, x: e.clientX, y: e.clientY });
  }
  function goTo(o: CatalogObject) {
    choose(o);
    setView("catalog");
    setQuery("");
    const instance =
      o.kind === "database"
        ? catalog.instances.find((i) => i.id === o.parentId)
        : null;
    const serverID =
      o.kind === "server"
        ? o.id
        : o.kind === "instance"
          ? o.parentId
          : instance?.parentId;
    setPage(
      Math.max(
        0,
        Math.floor(catalog.servers.findIndex((s) => s.id === serverID) / 10),
      ),
    );
  }
  async function toggleFavorite(o: CatalogObject) {
    await api().SetFavorite(o.kind, o.id, !isFavorite(o));
    setFavorites(await api().ListFavorites());
  }
  function openEditor(o: CatalogObject) {
    setError("");
    setEditor(o);
  }
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
    const [result, refs, favs, ps] = await Promise.all([
      api().GetCatalog(),
      api().GetLookups(),
      api().ListFavorites(),
      api().ListPolicies(),
    ]);
    setPolicies(ps);
    setLookups(refs);
    setFavorites(favs);
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
        onContextMenu={(e) => openContext(e, o)}
        onKeyDown={(e) => {
          if (e.key === "ContextMenu" || (e.shiftKey && e.key === "F10")) {
            e.preventDefault();
            const r = e.currentTarget.getBoundingClientRect();
            choose(o);
            setContextMenu({ object: o, x: r.left + 20, y: r.bottom });
          }
        }}
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
      setPolicyEditor(null);
      setPolicyDelete(null);
      setDeleteTarget(null);
      setContextMenu(null);
      setHistory(null);
      setFavorites([]);
      setLookups({});
      setPolicies([]);
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
            <button
              disabled={!status.user || busy}
              className={view === "policies" ? "active" : ""}
              onClick={() =>
                void run(async () => {
                  setPolicies(await api().ListPolicies());
                  setView("policies");
                })
              }
            >
              ◷ Политики РК
            </button>
            <button
              disabled={!status.user || busy}
              className={view === "excel" ? "active" : ""}
              onClick={() => setView("excel")}
            >
              ⇄ Excel
            </button>
            <button
              disabled={!status.user || busy}
              className={view === "favorites" ? "active" : ""}
              onClick={() =>
                void run(async () => {
                  setFavorites(await api().ListFavorites());
                  setView("favorites");
                })
              }
            >
              ★ Избранное
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
                      setFavorites([]);
                      setLookups({});
                      setPolicies([]);
                      setSelected(null);
                      setEditor(null);
                      setPolicyEditor(null);
                      setPolicyDelete(null);
                      setDeleteTarget(null);
                      setContextMenu(null);
                      setHistory(null);
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
                      onClick={() => openEditor(newObject("server"))}
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
                            const value = objectValue(selected, key);
                            return value ? (
                              <React.Fragment key={key}>
                                <dt>{label}</dt>
                                <dd>{String(value)}</dd>
                              </React.Fragment>
                            ) : null;
                          })}
                      </dl>
                      {selected.kind !== "server" && (
                        <BackupCard
                          object={selected}
                          policies={policies}
                          canEdit={!!canEdit}
                          onEdit={openPolicy}
                        />
                      )}
                      <div className="detail-actions">
                        <button
                          disabled={busy}
                          onClick={() =>
                            void run(() => toggleFavorite(selected))
                          }
                        >
                          {isFavorite(selected)
                            ? "★ В избранном"
                            : "☆ В избранное"}
                        </button>
                        {canEdit && (
                          <>
                            <button
                              disabled={busy}
                              onClick={() => openEditor({ ...selected })}
                            >
                              Изменить
                            </button>
                            {selected.kind !== "database" && (
                              <button
                                disabled={busy}
                                onClick={() =>
                                  openEditor(
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
                                    objectValue(h.before, k) !==
                                    objectValue(h.after, k),
                                )
                                .map(([k, label]) => (
                                  <p key={k}>
                                    <b>{label}: </b>
                                    {String(
                                      objectValue(h.before, k) || "—",
                                    )} →{" "}
                                    {String(objectValue(h.after, k) || "—")}
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
          {view === "policies" && (
            <PoliciesPanel
              catalog={catalog}
              policies={policies}
              busy={busy}
              canEdit={!!canEdit}
              canDelete={status.user?.role === "admin"}
              onEdit={(p) => {
                setError("");
                setPolicyEditor(p);
              }}
              onDelete={(p) => {
                setError("");
                setPolicyDelete(p);
              }}
              onCreate={() => {
                setError("");
                setPolicyEditor(newPolicy());
              }}
              run={run}
            />
          )}
          {view === "excel" && (
            <ExcelPanel
              canEdit={!!canEdit}
              busy={busy}
              run={run}
              onImported={loadCatalog}
            />
          )}
          {view === "favorites" && (
            <FavoritesPanel
              objects={favorites}
              busy={busy}
              onOpen={goTo}
              onRemove={(o) => void run(() => toggleFavorite(o))}
              onContext={openContext}
            />
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
        <ObjectEditor
          editor={editor}
          catalog={catalog}
          lookups={lookups}
          busy={busy}
          error={error}
          onChange={setEditor}
          onClose={() => setEditor(null)}
          onSave={() =>
            void run(async () => {
              const saved = await api().SaveObject(editor);
              await loadCatalog();
              setSelected(saved);
              setHistory(null);
              setEditor(null);
              setMessage("Объект сохранён");
            })
          }
        />
      )}
      {contextMenu && (
        <ContextMenu
          {...contextMenu}
          canEdit={!!canEdit}
          canDelete={status.user?.role === "admin"}
          favorite={isFavorite(contextMenu.object)}
          onClose={() => setContextMenu(null)}
          onOpen={() => goTo(contextMenu.object)}
          onEdit={() => openEditor({ ...contextMenu.object })}
          onAdd={() =>
            openEditor(
              newObject(
                contextMenu.object.kind === "server" ? "instance" : "database",
                contextMenu.object.id,
              ),
            )
          }
          onPolicy={() => openPolicy(contextMenu.object)}
          onHistory={() => {
            goTo(contextMenu.object);
            void run(async () =>
              setHistory(
                await api().GetHistory(
                  contextMenu.object.kind,
                  contextMenu.object.id,
                ),
              ),
            );
          }}
          onFavorite={() => void run(() => toggleFavorite(contextMenu.object))}
          onDelete={() => {
            setError("");
            setDeleteTarget(contextMenu.object);
          }}
        />
      )}
      {policyEditor && (
        <PolicyEditor
          policy={policyEditor}
          catalog={catalog}
          lookups={lookups}
          busy={busy}
          error={error}
          onChange={setPolicyEditor}
          onClose={() => setPolicyEditor(null)}
          onSave={() =>
            void run(async () => {
              await api().SavePolicy(policyEditor);
              setPolicies(await api().ListPolicies());
              setPolicyEditor(null);
              setMessage("Политика сохранена");
            })
          }
        />
      )}
      {policyDelete && (
        <div className="modal-backdrop">
          <section
            className="modal compact"
            role="dialog"
            aria-modal="true"
            aria-label="Подтверждение удаления политики"
          >
            <h2>Удалить политику «{policyDelete.name}»?</h2>
            <p>
              После удаления политики БД может снова применяться политика
              экземпляра. История изменений сохранится.
            </p>
            <div className="actions">
              <button disabled={busy} onClick={() => setPolicyDelete(null)}>
                Отмена
              </button>
              <button
                className="danger"
                disabled={busy}
                onClick={() =>
                  void run(async () => {
                    await api().DeletePolicy(
                      policyDelete.id,
                      policyDelete.version,
                    );
                    setPolicies(await api().ListPolicies());
                    setPolicyDelete(null);
                    setMessage("Политика удалена");
                  })
                }
              >
                Удалить
              </button>
            </div>
            {error && <div className="notice error">{error}</div>}
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
