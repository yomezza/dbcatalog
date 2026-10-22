import React from "react";
import type { Connection, Status } from "./api";
import { Field } from "./components";
interface Props {
  status: Status;
  register: boolean;
  login: string;
  userPassword: string;
  busy: boolean;
  setLogin: (v: string) => void;
  setUserPassword: (v: string) => void;
  setRegister: (v: boolean) => void;
  onSubmit: React.FormEventHandler<HTMLFormElement>;
}
export function AuthPanel({
  status,
  register,
  login,
  userPassword,
  busy,
  setLogin,
  setUserPassword,
  setRegister,
  onSubmit,
}: Props) {
  return (
    <section className="auth-card">
      <div className="eyebrow">{status.schema.toUpperCase()}</div>
      <h1>
        {status.needsAdmin
          ? "Первый администратор"
          : register
            ? "Регистрация"
            : "Вход в CatalogDB"}
      </h1>
      <p className="muted">
        {status.needsAdmin
          ? "Создайте администратора новой схемы. Повторное создание будет заблокировано."
          : register
            ? "Новая учётная запись появится после подтверждения администратором."
            : "Используйте учётную запись приложения, а не PostgreSQL."}
      </p>
      <form onSubmit={onSubmit}>
        <fieldset disabled={busy}>
          <Field label="Логин *">
            <input
              required
              minLength={3}
              maxLength={80}
              autoComplete="username"
              value={login}
              onChange={(e) => setLogin(e.target.value)}
            />
          </Field>
          <Field label="Пароль *">
            <input
              required
              type="password"
              minLength={status.needsAdmin || register ? 12 : 1}
              maxLength={1024}
              autoComplete={
                status.needsAdmin || register
                  ? "new-password"
                  : "current-password"
              }
              value={userPassword}
              onChange={(e) => setUserPassword(e.target.value)}
            />
          </Field>
          <button className="primary">
            {status.needsAdmin
              ? "Создать администратора"
              : register
                ? "Отправить заявку"
                : "Войти"}
          </button>
          {!status.needsAdmin && (
            <button
              type="button"
              className="link"
              onClick={() => {
                setRegister(!register);
                setUserPassword("");
              }}
            >
              {register ? "Уже есть учётная запись" : "Зарегистрироваться"}
            </button>
          )}
        </fieldset>
      </form>
    </section>
  );
}
