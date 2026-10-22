import React, { useState, useEffect } from "react";
import type { User } from "./api";
export function Field({
  label,
  children,
}: {
  label: string;
  children: React.ReactNode;
}) {
  return (
    <label className="field">
      <span>{label}</span>
      {children}
    </label>
  );
}
export function Stat({ label, value }: { label: string; value: number }) {
  return (
    <div className="stat">
      <span>{label}</span>
      <strong>{value}</strong>
    </div>
  );
}
export function UserRow({
  user,
  disabled,
  save,
}: {
  user: User;
  disabled: boolean;
  save: (role: string, state: string) => void;
}) {
  const [role, setRole] = useState(user.role),
    [state, setState] = useState(user.state);
  useEffect(() => {
    setRole(user.role);
    setState(user.state);
  }, [user]);
  return (
    <tr>
      <td>{user.login}</td>
      <td>
        <select
          disabled={disabled}
          value={role}
          onChange={(e) => setRole(e.target.value as User["role"])}
        >
          <option value="admin">Администратор</option>
          <option value="editor">Редактор</option>
          <option value="viewer">Наблюдатель</option>
        </select>
      </td>
      <td>
        <select
          disabled={disabled}
          value={state}
          onChange={(e) => setState(e.target.value)}
        >
          <option value="pending">Ожидает подтверждения</option>
          <option value="active">Активна</option>
          <option value="blocked">Заблокирована</option>
          <option value="disabled">Отключена</option>
        </select>
      </td>
      <td>
        <button
          disabled={disabled || (role === user.role && state === user.state)}
          onClick={() => save(role, state)}
        >
          Сохранить
        </button>
      </td>
    </tr>
  );
}
