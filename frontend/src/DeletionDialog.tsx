import { useState } from "react";
import type { DeletionPreview } from "./api";
export function DeletionDialog({
  preview: p,
  busy,
  error,
  onClose,
  onConfirm,
}: {
  preview: DeletionPreview;
  busy: boolean;
  error: string;
  onClose: () => void;
  onConfirm: (confirmation: string) => void;
}) {
  const [step, setStep] = useState(0),
    [phrase, setPhrase] = useState("");
  const all = p.kind === "all";
  const total = p.servers + p.instances + p.databases + p.policies;
  return (
    <div className="modal-backdrop">
      <section
        className="modal compact deletion-dialog"
        role="dialog"
        aria-modal="true"
        aria-label={all ? "Очистка каталога" : "Подтверждение удаления"}
      >
        <div className="eyebrow">
          {all ? `ПОДТВЕРЖДЕНИЕ ${step + 1} ИЗ 2` : "УДАЛЕНИЕ ВЕТВИ"}
        </div>
        <h2>{all ? "Очистить весь каталог?" : `Удалить «${p.name}»?`}</h2>
        <p>
          Будут удалены выбранные записи, их дочерние объекты и связанные
          политики резервного копирования. Ссылки в избранном удалятся у всех
          пользователей.
        </p>
        <dl className="deletion-counts">
          <dt>Серверы</dt>
          <dd>{p.servers}</dd>
          <dt>Экземпляры СУБД</dt>
          <dd>{p.instances}</dd>
          <dt>Базы данных</dt>
          <dd>{p.databases}</dd>
          <dt>Политики РК</dt>
          <dd>{p.policies}</dd>
        </dl>
        <p className="info">
          Пользователи, справочники, настройки подключения, история изменений и
          журнал аудита сохранятся. Операция выполняется целиком в одной
          транзакции.
        </p>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            if (!total || busy) return;
            if (all && step === 0) {
              setStep(1);
              return;
            }
            if (all && phrase !== "УДАЛИТЬ КАТАЛОГ") return;
            onConfirm(phrase);
          }}
        >
          {all && step === 1 && (
            <label className="field">
              <span>
                Введите УДАЛИТЬ КАТАЛОГ для окончательного подтверждения
              </span>
              <input
                autoFocus
                autoComplete="off"
                value={phrase}
                onChange={(e) => setPhrase(e.target.value)}
                disabled={busy}
              />
            </label>
          )}
          <p className="muted">
            Вложения сохраняются в PostgreSQL и будут доступны после
            восстановления объекта.
          </p>
          {!total && <p>Каталог уже пуст.</p>}
          <div className="actions">
            <button type="button" disabled={busy} onClick={onClose}>
              Отмена
            </button>
            <button
              className="danger"
              disabled={
                busy ||
                !total ||
                (all && step === 1 && phrase !== "УДАЛИТЬ КАТАЛОГ")
              }
            >
              {all && step === 0
                ? "Продолжить"
                : all
                  ? "Очистить каталог"
                  : "Удалить вместе с дочерними"}
            </button>
          </div>
        </form>
        {error && (
          <div className="notice error" role="alert">
            {error}
          </div>
        )}
      </section>
    </div>
  );
}
