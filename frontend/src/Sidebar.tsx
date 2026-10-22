import type { Status } from "./api";
export type View =
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
  | "statistics";
const sections: { view: View; label: string; admin?: boolean }[] = [
  { view: "catalog", label: "▦ Общий каталог" },
  { view: "policies", label: "◷ Политики РК" },
  { view: "excel", label: "⇄ Экспорт/Импорт" },
  { view: "favorites", label: "★ Избранное" },
  { view: "downloads", label: "↓ Загрузки" },
  { view: "notifications", label: "♧ Уведомления" },
  { view: "statistics", label: "▥ Статистика" },
  { view: "users", label: "♙ Пользователи", admin: true },
  { view: "audit", label: "≡ Журнал аудита", admin: true },
  { view: "settings", label: "⚙ Настройки" },
];
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
  return (
    <aside>
      <div className="workspace-label">РАБОЧАЯ ОБЛАСТЬ</div>
      <nav>
        {sections
          .filter((s) => !s.admin || status.user?.role === "admin")
          .map((s) => (
            <button
              key={s.view}
              disabled={busy || (!status.user && s.view !== "settings")}
              className={
                view === s.view ||
                (view === "connection" && s.view === "settings")
                  ? "active"
                  : ""
              }
              onClick={() => onNavigate(s.view)}
            >
              {s.label}
              {s.view === "notifications" && unread > 0 && (
                <span
                  className="nav-counter"
                  aria-label={`Непрочитанных: ${unread}`}
                >
                  {unread}
                </span>
              )}
            </button>
          ))}
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
            <button disabled={busy} onClick={onLogout}>
              Выйти
            </button>
          </>
        )}
      </div>
    </aside>
  );
}
