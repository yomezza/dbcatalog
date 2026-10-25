import { useEffect, useState, type ReactNode } from "react";
import type { Status } from "./api";
export type View =
  | "diagnostics"
  | "connection"
  | "auth"
  | "catalog"
  | "users"
  | "favorites"
  | "excel"
  | "policies"
  | "settings"
  | "audit"
  | "downloads"
  | "notifications"
  | "statistics"
  | "quality"
  | "deleted";
type NavigationView = Exclude<View, "connection" | "auth">;
const sections: { view: NavigationView; label: string; admin?: boolean }[] = [
  { view: "catalog", label: "Общий каталог" },
  { view: "policies", label: "Политики РК" },
  { view: "excel", label: "Экспорт/Импорт" },
  { view: "favorites", label: "Избранное" },
  { view: "downloads", label: "Загрузки" },
  { view: "notifications", label: "Уведомления" },
  { view: "quality", label: "Качество данных" },
  { view: "deleted", label: "Удалённые", admin: true },
  { view: "statistics", label: "Статистика" },
  { view: "users", label: "Пользователи", admin: true },
  { view: "audit", label: "Журнал аудита", admin: true },
  { view: "diagnostics", label: "Диагностика" },
  { view: "settings", label: "Настройки" },
];

const shapes = {
  catalog: (
    <>
      <rect x="3" y="3" width="18" height="18" rx="2" />
      <path d="M3 9h18M9 9v12" />
    </>
  ),
  policies: (
    <>
      <circle cx="12" cy="12" r="9" />
      <path d="M12 7v5l3 2" />
    </>
  ),
  excel: (
    <>
      <path d="M4 7h16l-4-4M20 17H4l4 4M20 7l-4 4M4 17l4-4" />
    </>
  ),
  favorites: (
    <path d="m12 3 2.8 5.7 6.3.9-4.6 4.5 1.1 6.3-5.6-3-5.6 3 1.1-6.3L2.9 9.6l6.3-.9Z" />
  ),
  downloads: (
    <>
      <path d="M12 3v12m-5-5 5 5 5-5M4 16v4a1 1 0 0 0 1 1h14a1 1 0 0 0 1-1v-4" />
    </>
  ),
  notifications: (
    <>
      <path d="M18 8a6 6 0 0 0-12 0c0 7-3 7-3 9h18c0-2-3-2-3-9M10 21h4" />
    </>
  ),
  quality: (
    <>
      <path d="m12 2 10 10-10 10L2 12Z" />
      <path d="m8 12 3 3 5-6" />
    </>
  ),
  deleted: (
    <>
      <path d="M3 11a9 9 0 1 1 2.6 7M3 4v7h7M12 7v5l3 2" />
    </>
  ),
  statistics: (
    <>
      <path d="M4 3v18h17M9 16v-5m5 5V7m5 9V4" />
    </>
  ),
  users: (
    <>
      <circle cx="9" cy="7" r="4" />
      <path d="M2 21v-3a7 7 0 0 1 14 0v3M17 3a4 4 0 0 1 0 8m3 3a7 7 0 0 1 2 5v2" />
    </>
  ),
  audit: (
    <>
      <rect x="5" y="4" width="15" height="18" rx="2" />
      <path d="M9 2h7v4H9zM9 11h7m-7 4h7m-7 4h4" />
    </>
  ),
  diagnostics: (
    <>
      <circle cx="12" cy="12" r="10" />
      <path d="M3 12h5l2-5 4 10 2-5h5" />
    </>
  ),
  settings: (
    <>
      <path d="m10 2-.7 3-2 .9-2.8-1-2 3.4 2.2 2.1v2.3l-2.2 2.1 2 3.5 2.8-1 2 .9.7 2.8h4l.7-2.8 2-.9 2.8 1 2-3.5-2.2-2.1v-2.3l2.2-2.1-2-3.4-2.8 1-2-.9L14 2Z" />
      <circle cx="12" cy="11.5" r="3" />
    </>
  ),
  logout: (
    <>
      <path d="M9 3H4a1 1 0 0 0-1 1v16a1 1 0 0 0 1 1h5M9 12h12m-5-5 5 5-5 5" />
    </>
  ),
  toggle: <path d="m14 6-6 6 6 6" />,
} satisfies Record<NavigationView | "logout" | "toggle", ReactNode>;

function SidebarIcon({ name }: { name: keyof typeof shapes }) {
  return (
    <svg
      className="sidebar-icon"
      viewBox="0 0 24 24"
      fill="none"
      stroke="currentColor"
      strokeWidth="1.65"
      strokeLinecap="round"
      strokeLinejoin="round"
      aria-hidden="true"
      focusable="false"
    >
      {shapes[name]}
    </svg>
  );
}
const roleLabels = {
  admin: "Администратор",
  editor: "Редактор",
  viewer: "Наблюдатель",
};
export function Sidebar({
  view,
  status,
  busy,
  unread,
  onNavigate,
  onLogout,
}: {
  view: View;
  status: Status;
  busy: boolean;
  unread: number;
  onNavigate: (view: View) => void;
  onLogout: () => void;
}) {
  const [collapsed, setCollapsed] = useState(() => {
    try {
      const saved = localStorage.getItem("catalogdb.sidebar.collapsed");
      return saved === null
        ? document.documentElement.dataset.platform === "android"
        : saved === "true";
    } catch {
      return document.documentElement.dataset.platform === "android";
    }
  });
  useEffect(() => {
    try {
      localStorage.setItem("catalogdb.sidebar.collapsed", String(collapsed));
    } catch {
      // Navigation still works when the WebView disallows local persistence.
    }
  }, [collapsed]);
  const connectionLabel = status.connected
    ? `Схема: ${status.schema}`
    : "Нет подключения";
  const toggleLabel = collapsed
    ? "Развернуть рабочую область"
    : "Свернуть рабочую область";
  return (
    <aside
      className={`sidebar${collapsed ? " is-collapsed" : ""}`}
      aria-label="Рабочая область"
    >
      <div className="workspace-heading">
        <div className="workspace-label">РАБОЧАЯ ОБЛАСТЬ</div>
        <button
          type="button"
          className="sidebar-toggle"
          title={toggleLabel}
          aria-label={toggleLabel}
          aria-expanded={!collapsed}
          aria-controls="workspace-navigation"
          onClick={() => setCollapsed((value) => !value)}
        >
          <SidebarIcon name="toggle" />
        </button>
      </div>
      <nav id="workspace-navigation" aria-label="Разделы приложения">
        {sections
          .filter((s) => !s.admin || status.user?.role === "admin")
          .map((s) => (
            <button
              key={s.view}
              title={s.label}
              aria-label={s.label}
              aria-current={
                view === s.view ||
                (view === "connection" && s.view === "settings")
                  ? "page"
                  : undefined
              }
              disabled={
                busy ||
                (!status.user &&
                  s.view !== "settings" &&
                  s.view !== "diagnostics")
              }
              className={
                view === s.view ||
                (view === "connection" && s.view === "settings")
                  ? "active"
                  : ""
              }
              onClick={() => onNavigate(s.view)}
            >
              <SidebarIcon name={s.view} />
              <span className="nav-label">{s.label}</span>
              {s.view === "notifications" && unread > 0 && (
                <span
                  className="nav-counter"
                  aria-label={`Непрочитанных: ${unread}`}
                >
                  {unread > 99 ? "99+" : unread}
                </span>
              )}
            </button>
          ))}
      </nav>
      <div className="sidebar-bottom">
        <div
          className="sidebar-status"
          title={connectionLabel}
          role="status"
          aria-label={connectionLabel}
        >
          <span className={"dot " + (status.connected ? "online" : "")} />
          <span className="sidebar-connection">{connectionLabel}</span>
        </div>
        {status.user && (
          <>
            <div
              className="sidebar-user"
              role="group"
              aria-label={`Пользователь: ${status.user.login} · ${roleLabels[status.user.role]}`}
              title={`${status.user.login} · ${roleLabels[status.user.role]}`}
            >
              <span className="sidebar-user-icon">
                <SidebarIcon name="users" />
              </span>
              <div className="sidebar-user-details">
                <strong>{status.user.login}</strong>
                <small>{roleLabels[status.user.role]}</small>
              </div>
            </div>
            <button
              className="sidebar-logout"
              title="Выйти"
              aria-label="Выйти"
              disabled={busy}
              onClick={onLogout}
            >
              <SidebarIcon name="logout" />
              <span className="nav-label">Выйти</span>
            </button>
          </>
        )}
      </div>
    </aside>
  );
}
