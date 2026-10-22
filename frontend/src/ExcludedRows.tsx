import { useState } from "react";
import type { ImportRowPreview } from "./api";
export function ExcludedRows({
  rows,
  busy,
  onRestore,
}: {
  rows: ImportRowPreview[];
  busy: boolean;
  onRestore: (ids: number[]) => void;
}) {
  const [page, setPage] = useState(0),
    last = Math.max(0, Math.ceil(rows.length / 25) - 1),
    current = Math.min(last, page);
  if (!rows.length) return null;
  return (
    <details className="excluded-rows">
      <summary>Исключённые строки ({rows.length})</summary>
      <p className="muted">
        Не участвуют в проверке, импорте и исправленной копии. Исходный файл не
        изменён.
      </p>
      <button disabled={busy} onClick={() => onRestore(rows.map((r) => r.row))}>
        Вернуть все исключённые строки
      </button>
      <div className="import-table">
        <table>
          <thead>
            <tr>
              <th>Строка</th>
              <th>Сервер</th>
              <th>Экземпляр</th>
              <th>SID</th>
              <th>Действие</th>
            </tr>
          </thead>
          <tbody>
            {rows.slice(current * 25, current * 25 + 25).map((r) => (
              <tr key={r.row}>
                <td>{r.row}</td>
                <td>{r.server || "—"}</td>
                <td>{r.instance || "—"}</td>
                <td>{r.database || "—"}</td>
                <td>
                  <button disabled={busy} onClick={() => onRestore([r.row])}>
                    Вернуть строку {r.row}
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      <div className="pagination">
        <button
          disabled={!current || busy}
          onClick={() => setPage(current - 1)}
        >
          ←
        </button>
        <span>
          Страница {current + 1} из {last + 1}
        </span>
        <button
          disabled={current === last || busy}
          onClick={() => setPage(current + 1)}
        >
          →
        </button>
      </div>
    </details>
  );
}
