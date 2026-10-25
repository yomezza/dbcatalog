import React, { useState } from "react";
import type { Connection } from "./api";
import { Field } from "./components";
import { isAndroid, pickRootCertificate } from "./android";
interface Props {
  connection: Connection;
  password: string;
  hasPassword: boolean;
  useSaved: boolean;
  remember: boolean;
  busy: boolean;
  updateConnection: <K extends keyof Connection>(
    key: K,
    value: Connection[K],
  ) => void;
  setPassword: (v: string) => void;
  setUseSaved: (v: boolean) => void;
  setRemember: (v: boolean) => void;
  onSubmit: React.FormEventHandler<HTMLFormElement>;
}
export function ConnectionPanel({
  connection,
  password,
  hasPassword,
  useSaved,
  remember,
  busy,
  updateConnection,
  setPassword,
  setUseSaved,
  setRemember,
  onSubmit,
}: Props) {
  const android = isAndroid();
  const [pickingCertificate, setPickingCertificate] = useState(false);
  const [certificateError, setCertificateError] = useState("");
  async function chooseCertificate() {
    setCertificateError("");
    setPickingCertificate(true);
    try {
      const path = await pickRootCertificate();
      if (path) updateConnection("sslRootCert", path);
    } catch (error) {
      setCertificateError(
        error instanceof Error
          ? error.message
          : "Не удалось выбрать сертификат",
      );
    } finally {
      setPickingCertificate(false);
    }
  }
  return (
    <section className="setup-card">
      <div className="eyebrow">НАСТРОЙКА РАБОЧЕГО МЕСТА</div>
      <h1>Подключение к PostgreSQL</h1>
      <p className="muted">
        Выберите центральную базу и схему каталога. Отсутствующая схема будет
        создана, доступные миграции — применены автоматически.
      </p>
      <form onSubmit={onSubmit}>
        <fieldset disabled={busy}>
          <div className="form-grid">
            <Field label="Сервер *">
              <input
                required
                value={connection.host}
                onChange={(e) => updateConnection("host", e.target.value)}
              />
            </Field>
            <Field label="Порт *">
              <input
                type="number"
                required
                min="1"
                max="65535"
                value={connection.port}
                onChange={(e) =>
                  updateConnection("port", Number(e.target.value))
                }
              />
            </Field>
            <Field label="База данных *">
              <input
                required
                value={connection.database}
                onChange={(e) => updateConnection("database", e.target.value)}
              />
            </Field>
            <Field label="Схема БД">
              <input
                placeholder="dbcatalog"
                maxLength={63}
                pattern="[a-z_][a-z0-9_]*"
                value={connection.schema}
                onChange={(e) => updateConnection("schema", e.target.value)}
              />
            </Field>
            <Field label="Пользователь PostgreSQL *">
              <input
                required
                autoComplete="username"
                value={connection.username}
                onChange={(e) => updateConnection("username", e.target.value)}
              />
            </Field>
            <Field label="Пароль PostgreSQL">
              <input
                type="password"
                autoComplete="current-password"
                disabled={useSaved}
                value={password}
                onChange={(e) => setPassword(e.target.value)}
              />
            </Field>
            <Field label="Режим TLS">
              <select
                value={connection.sslMode}
                onChange={(e) => updateConnection("sslMode", e.target.value)}
              >
                <option value="verify-full">
                  verify-full — сертификат и имя сервера
                </option>
                <option value="verify-ca">verify-ca — сертификат</option>
                <option value="require">require — шифрование</option>
                <option value="disable">
                  disable — только доверенная тестовая сеть
                </option>
              </select>
            </Field>
            <Field label="Корневой сертификат (путь)">
              <input
                value={connection.sslRootCert}
                readOnly={android}
                onChange={(e) =>
                  updateConnection("sslRootCert", e.target.value)
                }
                placeholder="При необходимости"
              />
              {android && (
                <div className="certificate-actions">
                  <button
                    type="button"
                    disabled={busy || pickingCertificate}
                    onClick={() => void chooseCertificate()}
                  >
                    {pickingCertificate
                      ? "Выбор сертификата…"
                      : "Выбрать сертификат…"}
                  </button>
                  {connection.sslRootCert && (
                    <button
                      type="button"
                      disabled={busy || pickingCertificate}
                      onClick={() => updateConnection("sslRootCert", "")}
                    >
                      Сбросить
                    </button>
                  )}
                </div>
              )}
              {certificateError && (
                <small role="alert" className="certificate-error">
                  {certificateError}
                </small>
              )}
            </Field>
          </div>
          {hasPassword && (
            <label className="check">
              <input
                type="checkbox"
                checked={useSaved}
                onChange={(e) => setUseSaved(e.target.checked)}
              />
              Использовать пароль из системного хранилища
            </label>
          )}
          <label className="check">
            <input
              type="checkbox"
              checked={remember}
              onChange={(e) => setRemember(e.target.checked)}
            />
            Сохранить пароль в системном хранилище учётных данных
          </label>
          <div className="info">
            Пустое имя схемы означает <code>dbcatalog</code>. Схемы изолированы:
            смена имени открывает другой каталог и не переносит данные.
            Первичная настройка требует прав на создание схемы и таблиц;
            существующая база данных должна быть создана администратором.
          </div>
          <button className="primary" type="submit">
            Проверить, обновить схему и подключиться →
          </button>
        </fieldset>
      </form>
    </section>
  );
}
