import { HistoryDialog } from "./HistoryDialog";
import { VersionsDialog } from "./VersionsDialog";
import type { RecordTarget } from "./api";
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
  type Lookups,
  type DeletionPreview,
} from "./api";
import { filterCatalog, pageOf, branchKeys, oracleGroupKey } from "./catalog";
import "./style.css";
import { Stat, UserRow } from "./components";
import { ConnectionPanel } from "./ConnectionPanel";
import { AuthPanel } from "./AuthPanel";
import { PoliciesPanel, PolicyEditor } from "./PoliciesPanel";
import { ExcelPanel } from "./ExcelPanel";
import { ObjectEditor } from "./ObjectEditor";
import { ContextMenu } from "./ContextMenu";
import { FavoritesPanel } from "./FavoritesPanel";
import { ObjectCard } from "./ObjectCard";
import { DeletionDialog } from "./DeletionDialog";
import { SettingsPanel } from "./SettingsPanel";
import { DownloadsPanel } from "./DownloadsPanel";
import { NotificationsPanel } from "./NotificationsPanel";
import { StatisticsPanel } from "./StatisticsPanel";
import { Sidebar, type View } from "./Sidebar";
import { CatalogTree } from "./CatalogTree";
import type { Notification } from "./api";
import { AuditPanel } from "./AuditPanel";

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
  const [view, setView] = useState<View>("connection");
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
    [deletion, setDeletion] = useState<{
      target: CatalogObject | null;
      preview: DeletionPreview;
    } | null>(null);
  const [expanded, setExpanded] = useState<Set<string>>(new Set()),
    [unread, setUnread] = useState(0);
  function toggleBranch(id: string) {
    setExpanded((old) => {
      const next = new Set(old);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  }
  const [recordWindow, setRecordWindow] = useState<{
    mode: "history" | "versions";
    target: RecordTarget;
  } | null>(null);
  const [settingsConnection, setSettingsConnection] =
    useState<Connection | null>(null);
  const [query, setQuery] = useState(""),
    [page, setPage] = useState(0),
    [users, setUsers] = useState<User[]>([]);
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
        : o.kind === "instance" || o.dbms === "Oracle Database"
          ? o.parentId
          : instance?.parentId;
    setExpanded(
      (old) =>
        new Set([
          ...old,
          ...(serverID ? [serverID] : []),
          ...(instance ? [instance.id] : []),
          ...(o.dbms === "Oracle Database" ? [oracleGroupKey(o.parentId)] : []),
        ]),
    );
    setPage(
      Math.max(
        0,
        Math.floor(catalog.servers.findIndex((s) => s.id === serverID) / 10),
      ),
    );
  }
  async function navigateNotification(n: Notification) {
    if (n.entity === "user" && status.user?.role === "admin") {
      setUsers(await api().ListUsers());
      setView("users");
      return;
    }
    if (n.entity === "catalog") {
      await loadCatalog();
      setView("catalog");
      return;
    }
    if (n.entity === "account") {
      setMessage(n.body);
      return;
    }
    const c = await loadCatalog();
    const o = [...c.servers, ...c.instances, ...c.databases].find(
      (o) => o.kind === n.entity && o.id === n.objectId,
    );
    if (!o) {
      setMessage("Связанный объект удалён или недоступен");
      return;
    }
    goTo(o);
  }
  async function toggleFavorite(o: CatalogObject) {
    await api().SetFavorite(o.kind, o.id, !isFavorite(o));
    setFavorites(await api().ListFavorites());
  }
  function openEditor(o: CatalogObject) {
    setError("");
    setEditor(o);
  }
  function requestDelete(o: CatalogObject) {
    void run(async () => {
      const preview = await api().PreviewDeletion(o.kind, o.id, o.version);
      setDeletion({ target: o, preview });
    });
  }
  function requestClear() {
    void run(async () =>
      setDeletion({ target: null, preview: await api().PreviewClearCatalog() }),
    );
  }
  async function performDeletion(confirmation: string) {
    if (!deletion) return;
    await run(async () => {
      const p = deletion.preview;
      if (deletion.target) {
        const o = deletion.target;
        await api().DeleteObject(o.kind, o.id, o.version, p.fingerprint);
      } else {
        await api().ClearCatalog(p.fingerprint, confirmation);
      }
      await loadCatalog();
      setSelected(null);
      setRecordWindow(null);
      setDeletion(null);
      setMessage(
        deletion.target
          ? "Запись и дочерние объекты удалены"
          : "Каталог очищен. Пользователи, настройки, история и аудит сохранены",
      );
    });
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
  useEffect(() => {
    setRecordWindow(null);
    if (!status.user) {
      setUnread(0);
      setExpanded(new Set());
      return;
    }
    let live = true;
    const refresh = async () => {
      try {
        const result = await api().GetNotifications(0, true);
        if (live) setUnread(result.unread);
      } catch {
        /* Operation errors are reported when the inbox is opened. */
      }
    };
    void refresh();
    const timer = window.setInterval(() => void refresh(), 30000);
    return () => {
      live = false;
      window.clearInterval(timer);
    };
  }, [status.user?.id, status.schema]);
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
  const filtered = filterCatalog(catalog, query);
  const matches = filtered.servers;
  const lastPage = Math.max(0, Math.ceil(matches.length / 10) - 1);
  const currentPage = Math.min(page, lastPage);
  function choose(o: CatalogObject) {
    setSelected(o);
    setRecordWindow(null);
  }
  function node(o: CatalogObject) {
    return (
      <button
        className={"tree-node " + (selected?.id === o.id ? "selected" : "")}
        onClick={() => choose(o)}
        onDoubleClick={() => {
          if (o.kind !== "database") toggleBranch(o.id);
        }}
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
      setDeletion(null);
      setContextMenu(null);
      setRecordWindow(null);
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
        <Sidebar
          view={view}
          status={status}
          busy={busy}
          unread={unread}
          onNavigate={(next) => {
            if (
              next === "policies" ||
              next === "favorites" ||
              next === "users" ||
              next === "settings"
            )
              void run(async () => {
                if (next === "policies")
                  setPolicies(await api().ListPolicies());
                if (next === "favorites")
                  setFavorites(await api().ListFavorites());
                if (next === "users") setUsers(await api().ListUsers());
                if (next === "settings")
                  setSettingsConnection(
                    (await api().GetConnection()).connection,
                  );
                setView(next);
              });
            else setView(next);
          }}
          onLogout={() =>
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
              setDeletion(null);
              setContextMenu(null);
              setRecordWindow(null);
              setView("auth");
            })
          }
        />
        <main aria-busy={busy}>
          {error && !recordWindow && (
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
          {view === "settings" && (
            <SettingsPanel
              connection={settingsConnection}
              status={status}
              busy={busy}
              onConnection={() => {
                setPassword("");
                if (settingsConnection) setConnection(settingsConnection);
                setView("connection");
              }}
              onClear={requestClear}
            />
          )}
          {view === "audit" && status.user?.role === "admin" && (
            <AuditPanel
              busy={busy}
              run={run}
              onRecordHistory={(target) =>
                setRecordWindow({ mode: "history", target })
              }
              onRecordVersions={(target) =>
                setRecordWindow({ mode: "versions", target })
              }
            />
          )}
          {view === "downloads" && <DownloadsPanel busy={busy} run={run} />}
          {view === "notifications" && (
            <NotificationsPanel
              busy={busy}
              run={run}
              onUnread={setUnread}
              onNavigate={navigateNotification}
            />
          )}
          {view === "statistics" && (
            <StatisticsPanel
              busy={busy}
              run={run}
              onSearch={(q) => {
                setView("catalog");
                setQuery(q);
                setPage(0);
                const f = filterCatalog(catalog, q);
                setExpanded(new Set(branchKeys(f)));
              }}
            />
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
                      placeholder="Например: сервер | PostgreSQL | база"
                      title="Разделяйте условия символом | — должны совпасть все условия одной цепочки"
                      value={query}
                      onChange={(e) => {
                        setQuery(e.target.value);
                        setPage(0);
                        const f = filterCatalog(catalog, e.target.value);
                        setExpanded(
                          e.target.value ? new Set(branchKeys(f)) : new Set(),
                        );
                      }}
                    />
                  </div>
                  <div className="tree-controls">
                    <button
                      disabled={busy || !matches.length}
                      onClick={() => setExpanded(new Set(branchKeys(filtered)))}
                    >
                      Раскрыть все
                    </button>
                    <button
                      disabled={busy || !expanded.size}
                      onClick={() => setExpanded(new Set())}
                    >
                      Свернуть все
                    </button>
                  </div>
                  <div className="tree">
                    <CatalogTree
                      catalog={filtered}
                      servers={pageOf(matches, currentPage)}
                      expanded={expanded}
                      onToggle={toggleBranch}
                      renderNode={node}
                    />
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
                <ObjectCard
                  object={selected}
                  catalog={catalog}
                  policies={policies}
                  busy={busy}
                  canEdit={!!canEdit}
                  canDelete={status.user?.role === "admin"}
                  favorite={selected ? isFavorite(selected) : false}
                  onEdit={() => selected && openEditor({ ...selected })}
                  onAddOracle={() =>
                    selected &&
                    openEditor({
                      ...newObject("database", selected.id),
                      dbms: "Oracle Database",
                    })
                  }
                  onAdd={() =>
                    selected &&
                    selected.kind !== "database" &&
                    openEditor(
                      newObject(
                        selected.kind === "server" ? "instance" : "database",
                        selected.id,
                      ),
                    )
                  }
                  onFavorite={() =>
                    selected && void run(() => toggleFavorite(selected))
                  }
                  onVersions={() =>
                    selected &&
                    setRecordWindow({ mode: "versions", target: selected })
                  }
                  onHistory={() =>
                    selected &&
                    setRecordWindow({ mode: "history", target: selected })
                  }
                  onDelete={() => selected && requestDelete(selected)}
                  onPolicy={openPolicy}
                />
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
              onHistory={(p) =>
                setRecordWindow({
                  mode: "history",
                  target: { kind: "policy", id: p.id, name: p.name },
                })
              }
              onVersions={(p) =>
                setRecordWindow({
                  mode: "versions",
                  target: { kind: "policy", id: p.id, name: p.name },
                })
              }
            />
          )}
          {view === "excel" && (
            <ExcelPanel
              canEdit={!!canEdit}
              busy={busy}
              run={run}
              onImported={loadCatalog}
              lookups={lookups}
              error={error}
              onDownloads={() => setView("downloads")}
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
              setRecordWindow(null);
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
          onAddOracle={() =>
            openEditor({
              ...newObject("database", contextMenu.object.id),
              dbms: "Oracle Database",
            })
          }
          onAdd={() =>
            openEditor(
              newObject(
                contextMenu.object.kind === "server" ? "instance" : "database",
                contextMenu.object.id,
              ),
            )
          }
          onConnect={() =>
            void run(async () => {
              await api().ConnectToServer(
                contextMenu.object.kind,
                contextMenu.object.id,
              );
              setMessage("Клиент подключения запущен");
            })
          }
          onPolicy={() => openPolicy(contextMenu.object)}
          onHistory={() => {
            goTo(contextMenu.object);
            setRecordWindow({ mode: "history", target: contextMenu.object });
          }}
          onVersions={() => {
            goTo(contextMenu.object);
            setRecordWindow({ mode: "versions", target: contextMenu.object });
          }}
          onFavorite={() => void run(() => toggleFavorite(contextMenu.object))}
          onDelete={() => requestDelete(contextMenu.object)}
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
      {recordWindow?.mode === "history" && (
        <HistoryDialog
          target={recordWindow.target}
          catalog={catalog}
          busy={busy}
          error={error}
          run={run}
          onClose={() => setRecordWindow(null)}
          onVersions={
            status.user?.role === "admin"
              ? () => setRecordWindow({ ...recordWindow, mode: "versions" })
              : undefined
          }
        />
      )}
      {recordWindow?.mode === "versions" && status.user?.role === "admin" && (
        <VersionsDialog
          target={recordWindow.target}
          catalog={catalog}
          busy={busy}
          error={error}
          run={run}
          onClose={() => setRecordWindow(null)}
          onRestored={async () => {
            const c = await loadCatalog();
            const latest = [...c.servers, ...c.instances, ...c.databases].find(
              (o) =>
                o.id === recordWindow.target.id &&
                o.kind === recordWindow.target.kind,
            );
            if (latest) {
              setSelected(latest);
              setRecordWindow((w) => (w ? { ...w, target: latest } : null));
            }
          }}
        />
      )}
      {deletion && (
        <DeletionDialog
          key={deletion.preview.fingerprint}
          preview={deletion.preview}
          busy={busy}
          error={error}
          onClose={() => setDeletion(null)}
          onConfirm={(phrase) => void performDeletion(phrase)}
        />
      )}
    </>
  );
}
createRoot(document.getElementById("root")!).render(<App />);
