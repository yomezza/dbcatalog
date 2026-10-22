import { useEffect, useRef } from "react";
import type { CatalogObject } from "./api";
interface Props {
  object: CatalogObject;
  x: number;
  y: number;
  canEdit: boolean;
  canDelete: boolean;
  favorite: boolean;
  onClose: () => void;
  onOpen: () => void;
  onConnect: () => void;
  onEdit: () => void;
  onAdd: () => void;
  onAddOracle: () => void;
  onHistory: () => void;
  onPolicy: () => void;
  onFavorite: () => void;
  onDelete: () => void;
}
export function ContextMenu(p: Props) {
  const ref = useRef<HTMLDivElement>(null);
  useEffect(() => {
    ref.current?.querySelector<HTMLButtonElement>("button")?.focus();
    const close = (e: PointerEvent) => {
      if (!ref.current?.contains(e.target as Node)) p.onClose();
    };
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape") p.onClose();
    };
    window.addEventListener("pointerdown", close);
    window.addEventListener("keydown", key);
    window.addEventListener("resize", p.onClose);
    return () => {
      window.removeEventListener("pointerdown", close);
      window.removeEventListener("keydown", key);
      window.removeEventListener("resize", p.onClose);
    };
  }, [p.onClose]);
  const action = (fn: () => void) => () => {
    p.onClose();
    fn();
  };
  return (
    <div
      ref={ref}
      className="context-menu"
      role="menu"
      aria-label="Действия с объектом"
      style={{
        left: Math.max(8, Math.min(p.x, window.innerWidth - 270)),
        top: Math.max(40, Math.min(p.y, window.innerHeight - 430)),
      }}
      onKeyDown={(e) => {
        if (e.key === "ArrowDown" || e.key === "ArrowUp") {
          e.preventDefault();
          const buttons = Array.from(
            ref.current?.querySelectorAll("button") ?? [],
          );
          const i = buttons.indexOf(
            document.activeElement as HTMLButtonElement,
          );
          buttons[
            (i + (e.key === "ArrowDown" ? 1 : -1) + buttons.length) %
              buttons.length
          ]?.focus();
        }
      }}
    >
      <button role="menuitem" onClick={action(p.onOpen)}>
        Открыть карточку
      </button>
      <button role="menuitem" onClick={action(p.onConnect)}>
        Подключение к серверу
      </button>
      {p.canEdit && (
        <>
          <button role="menuitem" onClick={action(p.onEdit)}>
            Изменить
          </button>
          {p.object.kind === "server" && (
            <button role="menuitem" onClick={action(p.onAddOracle)}>
              Добавить базу Oracle
            </button>
          )}
          {p.object.kind !== "database" && (
            <button role="menuitem" onClick={action(p.onAdd)}>
              {p.object.kind === "server"
                ? "Добавить экземпляр СУБД"
                : "Добавить базу данных"}
            </button>
          )}
        </>
      )}
      {p.canEdit && p.object.kind !== "server" && (
        <button role="menuitem" onClick={action(p.onPolicy)}>
          Политика резервного копирования
        </button>
      )}
      <button role="menuitem" onClick={action(p.onHistory)}>
        История изменений
      </button>
      <button role="menuitem" onClick={action(p.onFavorite)}>
        {p.favorite ? "Убрать из избранного" : "Добавить в избранное"}
      </button>
      {p.canDelete && (
        <button role="menuitem" className="danger" onClick={action(p.onDelete)}>
          Удалить
        </button>
      )}
    </div>
  );
}
