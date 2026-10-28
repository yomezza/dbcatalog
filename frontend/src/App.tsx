import { CatalogView } from "./features/catalog/CatalogView";
import { PrintPDFDialog, type PrintTarget } from "./PrintPDFDialog";
import { useCatalogBrowser } from "./features/catalog/useCatalogBrowser";
import { QualityPanel } from "./features/quality/QualityPanel";
import { DeletedPanel } from "./features/deleted/DeletedPanel";
import { AttachmentsDialog } from "./features/documents/AttachmentsDialog";
import {
  MoveDialog,
  MovementsDialog,
} from "./features/movements/MovementsDialog";
import type { CatalogPage, Kind } from "./api";
import { emptyCriteria } from "./catalog";
import { HistoryDialog } from "./HistoryDialog";
import { VersionsDialog } from "./VersionsDialog";
import type { RecordTarget } from "./api";
import React, { useEffect, useRef, useState } from "react";
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
import { UserRow } from "./components";
import { useLiveUpdates } from "./features/live/useLiveUpdates";
import { UserPasswordDialog } from "./UserPasswordDialog";
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
import type { Notification } from "./api";
import { DiagnosticsPanel } from "./DiagnosticsPanel";
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
export function App() {
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
  const [passwordUser, setPasswordUser] = useState<User | null>(null);
  const [printTarget, setPrintTarget] = useState<PrintTarget | null>(null);
  useEffect(
    () => setPrintTarget(null),
    [status.connected, status.schema, status.user?.id],
  );
  const [busy, setBusy] = useState(false),
    [error, setError] = useState(""),
    [message, setMessage] = useState("");
  function printObject(object: CatalogObject) {
    setError("");
    setPrintTarget({ object });
  }
  const [sessionHours, setSessionHours] = useState(0);
  const [liveDirty, setLiveDirty] = useState(false);
  const [liveRevision, setLiveRevision] = useState(0);
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
  const [unread, setUnread] = useState(0);
  const [recordWindow, setRecordWindow] = useState<{
    mode: "history" | "versions";
    target: RecordTarget;
  } | null>(null);
  const [settingsConnection, setSettingsConnection] =
    useState<Connection | null>(null);
  const [users, setUsers] = useState<User[]>([]);
  const [policies, setPolicies] = useState<Policy[]>([]),
    [policyEditor, setPolicyEditor] = useState<Policy | null>(null),
    [policyDelete, setPolicyDelete] = useState<Policy | null>(null);
  const [rootSeed, setRootSeed] = useState<CatalogPage | null>(null),
    [catalogRevision, setCatalogRevision] = useState(0);
  const browser = useCatalogBrowser(rootSeed, catalogRevision);
  const selectionRequest = useRef(0);
  const [attachmentObject, setAttachmentObject] =
      useState<CatalogObject | null>(null),
    [moveObject, setMoveObject] = useState<CatalogObject | null>(null),
    [movementObject, setMovementObject] = useState<CatalogObject | null>(null);
  function openPolicy(o: CatalogObject) {
    if (o.kind === "server") return;
    const kind = o.kind;
    void run(async () => {
      const context = await api().GetObjectContext(o.kind, o.id);
      setPolicies(context.policies);
      setCatalog(context.catalog);
      setPolicyEditor(
        context.policies.find(
          (p) => p.kind === o.kind && p.objectId === o.id,
        ) ?? newPolicy(kind, o.id),
      );
    });
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
  function showContext(o: CatalogObject, x: number, y: number) {
    if (busy) return;
    choose(o);
    setContextMenu({ object: o, x, y });
  }
  function openContext(e: React.MouseEvent, o: CatalogObject) {
    e.preventDefault();
    showContext(o, e.clientX, e.clientY);
  }

  async function showObject(
    kind: Kind,
    id: string,
    reveal: boolean,
    isCurrent = () => true,
  ) {
    const request = ++selectionRequest.current;
    const context = await api().GetObjectContext(kind, id);
    if (request !== selectionRequest.current || !isCurrent()) return;
    setSelected(context.object);
    setCatalog(context.catalog);
    setPolicies(context.policies);
    if (reveal) {
      setView("catalog");
      browser.reveal(context);
    }
  }
  function goTo(o: CatalogObject) {
    setRecordWindow(null);
    void run(() => showObject(o.kind, o.id, true));
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
    if (
      n.entity !== "server" &&
      n.entity !== "instance" &&
      n.entity !== "database"
    ) {
      setMessage("Связанный объект недоступен");
      return;
    }
    await loadCatalog();
    try {
      await showObject(n.entity, n.objectId, true);
    } catch {
      setMessage("Связанный объект удалён или недоступен");
    }
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
        const resumed = result.needsAdmin
          ? result
          : await api().ResumeSession();
        setStatus(resumed);
        if (resumed.user) {
          await loadCatalog();
          setView("catalog");
        } else setView("auth");
      }
    });
  }, []);
  useEffect(() => {
    setRecordWindow(null);
    setPasswordUser(null);
    setAttachmentObject(null);
    setMoveObject(null);
    setMovementObject(null);
    if (!status.user) {
      setUnread(0);
      setRootSeed(null);
      setLiveDirty(false);
      browser.reset();
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
    return () => {
      live = false;
    };
  }, [status.user?.id, status.schema]);

  async function loadCatalog(isCurrent = () => true) {
    const workspace = await api().GetCatalogWorkspace();
    if (!isCurrent()) return emptyCatalog;
    setRootSeed(workspace.root);
    setLookups(workspace.lookups);
    setFavorites(workspace.favorites);
    setCatalogRevision((v) => v + 1);
    browser.clearSelection();
    if (selected) {
      try {
        await showObject(selected.kind, selected.id, false, isCurrent);
      } catch {
        if (!isCurrent()) return emptyCatalog;
        setSelected(null);
        setPolicies([]);
      }
    } else
      setCatalog({
        servers: workspace.root.items,
        instances: [],
        databases: [],
      });
    return {
      servers: workspace.root.items,
      instances: [],
      databases: [],
    } as Catalog;
  }
  async function refreshCatalog() {
    await loadCatalog();
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

  function choose(o: CatalogObject) {
    setSelected(o);
    setRecordWindow(null);
    const request = ++selectionRequest.current;
    void api()
      .GetObjectContext(o.kind, o.id)
      .then((c) => {
        if (request === selectionRequest.current) {
          setSelected(c.object);
          setCatalog(c.catalog);
          setPolicies(c.policies);
        }
      })
      .catch((e) => {
        if (request === selectionRequest.current) setError(String(e));
      });
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
      const resumed = result.needsAdmin ? result : await api().ResumeSession();
      setStatus(resumed);
      if (resumed.user) {
        await loadCatalog();
        setView("catalog");
      } else setView("auth");
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
        const user = await api().LoginWithSession(
          login,
          userPassword,
          sessionHours,
        );
        setStatus((s) => ({ ...s, user }));
        setUserPassword("");
        await loadCatalog();
        setView("catalog");
      }
    });
  };
  useLiveUpdates(
    status.user?.id,
    status.schema,
    busy,
    async (types, isCurrent) => {
      if (types.has("users_changed") || types.has("resync")) {
        const fresh = await api().GetStatus();
        if (!isCurrent()) return;
        setStatus(fresh);
        if (!fresh.user) {
          setSelected(null);
          setEditor(null);
          setPolicyEditor(null);
          setCatalog(emptyCatalog);
          setView("auth");
          setMessage("Сессия завершена. Войдите повторно.");
          return;
        }
        if (fresh.user.role !== status.user?.role) {
          setRecordWindow(null);
          setDeletion(null);
          setView("catalog");
        }
        if (view === "users" && fresh.user.role === "admin") {
          const users = await api().ListUsers();
          if (!isCurrent()) return;
          setUsers(users);
        }
      }
      const notifications = await api().GetNotifications(0, true);
      if (!isCurrent()) return;
      setUnread(notifications.unread);
      if (types.has("catalog_changed") || types.has("resync")) {
        if (editor || policyEditor || browser.bulkOpen) {
          setLiveDirty(true);
          setMessage(
            "Каталог изменился. Введённые данные сохранены; при сохранении будет проверена актуальная версия.",
          );
        } else {
          await loadCatalog(isCurrent);
          if (!isCurrent()) return;
          setLiveDirty(false);
          setLiveRevision((v) => v + 1);
        }
      } else if (types.has("notifications_changed"))
        setLiveRevision((v) => v + 1);
    },
  );
  useEffect(() => {
    if (
      liveDirty &&
      !busy &&
      !editor &&
      !policyEditor &&
      !browser.bulkOpen &&
      status.user
    ) {
      void loadCatalog()
        .then(() => {
          setLiveDirty(false);
          setLiveRevision((v) => v + 1);
        })
        .catch(() => {});
    }
  }, [
    liveDirty,
    busy,
    editor,
    policyEditor,
    browser.bulkOpen,
    status.user?.id,
  ]);
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
                if (next === "policies") await loadCatalog();
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
          {view === "diagnostics" && <DiagnosticsPanel busy={busy} run={run} />}
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
          {view === "quality" && (
            <QualityPanel
              busy={busy}
              run={run}
              onOpen={(kind, id) => {
                void run(() => showObject(kind, id, true));
              }}
            />
          )}
          {view === "deleted" && status.user?.role === "admin" && (
            <DeletedPanel
              error={error}
              busy={busy}
              run={run}
              onRestored={loadCatalog}
            />
          )}
          {view === "downloads" && <DownloadsPanel busy={busy} run={run} />}
          {view === "notifications" && (
            <NotificationsPanel
              revision={liveRevision}
              busy={busy}
              run={run}
              onUnread={setUnread}
              onNavigate={navigateNotification}
            />
          )}
          {view === "statistics" && (
            <StatisticsPanel
              revision={liveRevision}
              busy={busy}
              run={run}
              onSearch={(q) => {
                browser.setCriteria(emptyCriteria());
                browser.changeQuery(q);
                setView("catalog");
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
              sessionHours={sessionHours}
              setSessionHours={setSessionHours}
              busy={busy}
              setLogin={setLogin}
              setUserPassword={setUserPassword}
              setRegister={setRegister}
              onSubmit={authSubmit}
            />
          )}
          {view === "catalog" && (
            <CatalogView
              lookups={lookups}
              browser={browser}
              busy={busy}
              error={error}
              canEdit={!!canEdit}
              selectedId={selected?.id}
              onSelect={choose}
              onContext={showContext}
              onRefresh={refreshCatalog}
              onAddServer={() => openEditor(newObject("server"))}
              onMessage={setMessage}
              onPrintSelection={(ids) => {
                setError("");
                setPrintTarget({ ids });
              }}
              run={run}
            >
              <ObjectCard
                object={selected}
                catalog={catalog}
                policies={policies}
                busy={busy}
                canEdit={!!canEdit}
                canDelete={status.user?.role === "admin"}
                favorite={selected ? isFavorite(selected) : false}
                onAttachments={() => selected && setAttachmentObject(selected)}
                onPrint={() => selected && printObject(selected)}
                onMove={() => selected && setMoveObject(selected)}
                onMovements={() => selected && setMovementObject(selected)}
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
            </CatalogView>
          )}
          {view === "policies" && (
            <PoliciesPanel
              revision={catalogRevision + liveRevision}
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
                        passwordDisabled={busy}
                        onPassword={() => {
                          setError("");
                          setPasswordUser(u);
                        }}
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
      {passwordUser && (
        <UserPasswordDialog
          user={passwordUser}
          busy={busy}
          error={error}
          run={run}
          onClose={() => setPasswordUser(null)}
          onSaved={() => {
            setPasswordUser(null);
            setMessage("Пароль пользователя изменён");
          }}
        />
      )}
      {attachmentObject && (
        <AttachmentsDialog
          object={attachmentObject}
          busy={busy}
          error={error}
          canEdit={!!canEdit}
          canDelete={status.user?.role === "admin"}
          run={run}
          onClose={() => setAttachmentObject(null)}
        />
      )}
      {moveObject && (
        <MoveDialog
          object={moveObject}
          busy={busy}
          error={error}
          run={run}
          onClose={() => setMoveObject(null)}
          onMoved={async (o) => {
            await loadCatalog();
            await showObject(o.kind, o.id, true);
            setMessage(
              "Объект перемещён. Физические данные СУБД не переносились.",
            );
          }}
        />
      )}
      {movementObject && (
        <MovementsDialog
          object={movementObject}
          busy={busy}
          error={error}
          run={run}
          onClose={() => setMovementObject(null)}
        />
      )}
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
              await showObject(saved.kind, saved.id, true);
              setRecordWindow(null);
              setEditor(null);
              setMessage("Объект сохранён");
            })
          }
        />
      )}
      {printTarget && status.user && (
        <PrintPDFDialog
          target={printTarget}
          busy={busy}
          error={error}
          run={run}
          onClose={() => setPrintTarget(null)}
          onCreated={setMessage}
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
          onAttachments={() => setAttachmentObject(contextMenu.object)}
          onPrint={() => printObject(contextMenu.object)}
          onMove={() => setMoveObject(contextMenu.object)}
          onMovements={() => setMovementObject(contextMenu.object)}
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
              await loadCatalog();
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
                    await loadCatalog();
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
            await loadCatalog();
            if (recordWindow.target.kind !== "policy") {
              const context = await api().GetObjectContext(
                recordWindow.target.kind,
                recordWindow.target.id,
              );
              setSelected(context.object);
              setCatalog(context.catalog);
              setPolicies(context.policies);
              setRecordWindow((w) =>
                w ? { ...w, target: context.object } : null,
              );
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
