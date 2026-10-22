import { useEffect, useId, useRef, type ReactNode } from "react";
export function Modal({
  title,
  onClose,
  children,
  className = "",
  busy = false,
}: {
  title: string;
  onClose: () => void;
  children: ReactNode;
  className?: string;
  busy?: boolean;
}) {
  const ref = useRef<HTMLElement>(null),
    id = useId();
  const close = useRef(onClose);
  close.current = onClose;
  const locked = useRef(busy);
  locked.current = busy;
  useEffect(() => {
    const previous = document.activeElement as HTMLElement | null;
    ref.current?.focus();
    const key = (e: KeyboardEvent) => {
      if (e.key === "Escape" && !locked.current) {
        e.preventDefault();
        close.current();
      }
      if (e.key === "Tab") {
        const items = Array.from(
          ref.current?.querySelectorAll<HTMLElement>(
            'button:not(:disabled), input:not(:disabled), select:not(:disabled), textarea:not(:disabled), [tabindex="0"]',
          ) ?? [],
        );
        const first = items[0],
          last = items[items.length - 1];
        if (!first) {
          e.preventDefault();
          return;
        }
        if (
          e.shiftKey &&
          (document.activeElement === first ||
            document.activeElement === ref.current)
        ) {
          e.preventDefault();
          last.focus();
        } else if (
          !e.shiftKey &&
          (document.activeElement === last ||
            document.activeElement === ref.current)
        ) {
          e.preventDefault();
          first.focus();
        }
      }
    };
    document.addEventListener("keydown", key);
    return () => {
      document.removeEventListener("keydown", key);
      previous?.focus();
    };
  }, []);
  return (
    <div
      className="modal-backdrop"
      onMouseDown={(e) => {
        if (e.target === e.currentTarget && !busy) onClose();
      }}
    >
      <section
        ref={ref}
        tabIndex={-1}
        className={"modal " + className}
        role="dialog"
        aria-modal="true"
        aria-labelledby={id}
      >
        <div className="modal-heading">
          <h2 id={id}>{title}</h2>
          <button
            type="button"
            aria-label="Закрыть окно"
            disabled={busy}
            onClick={onClose}
          >
            ×
          </button>
        </div>
        {children}
      </section>
    </div>
  );
}
