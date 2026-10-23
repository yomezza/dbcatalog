import { useState } from "react";
import { api, type User } from "./api";
import { Field } from "./components";
import { Modal } from "./Modal";
import type { AsyncTaskRunner } from "./ui/types";

export function UserPasswordDialog({
  user,
  busy,
  error,
  run,
  onClose,
  onSaved,
}: {
  user: User;
  busy: boolean;
  error: string;
  run: AsyncTaskRunner;
  onClose: () => void;
  onSaved: () => void;
}) {
  const [password, setPassword] = useState(""),
    [confirmation, setConfirmation] = useState("");
  const mismatch = confirmation !== "" && password !== confirmation;
  const valid =
    [...password].length >= 12 &&
    new TextEncoder().encode(password).length <= 1024 &&
    password === confirmation;
  return (
    <Modal
      title={"Сменить пароль: " + user.login}
      busy={busy}
      onClose={onClose}
    >
      <p>
        Новый пароль применяется при следующем входе. Уже открытые сеансы
        продолжают работать. Роль и состояние пользователя сохраняются.
      </p>
      {error && (
        <p className="notice error" role="alert">
          {error}
        </p>
      )}
      <form
        onSubmit={(e) => {
          e.preventDefault();
          if (!valid || busy) return;
          void run(async () => {
            await api().SetUserPassword(user.id, password);
            setPassword("");
            setConfirmation("");
            onSaved();
          });
        }}
      >
        <Field label="Новый пароль">
          <input
            type="password"
            autoComplete="new-password"
            required
            minLength={12}
            maxLength={1024}
            disabled={busy}
            value={password}
            onChange={(e) => setPassword(e.target.value)}
          />
        </Field>
        <Field label="Повторите новый пароль">
          <input
            type="password"
            autoComplete="new-password"
            required
            maxLength={1024}
            disabled={busy}
            value={confirmation}
            onChange={(e) => setConfirmation(e.target.value)}
          />
        </Field>
        <p className="muted">Не менее 12 символов, не более 1024 байт.</p>
        {mismatch && (
          <p className="notice error" role="alert">
            Пароли не совпадают
          </p>
        )}
        <div className="actions">
          <button type="button" disabled={busy} onClick={onClose}>
            Отмена
          </button>
          <button type="submit" className="primary" disabled={busy || !valid}>
            Сохранить пароль
          </button>
        </div>
      </form>
    </Modal>
  );
}
