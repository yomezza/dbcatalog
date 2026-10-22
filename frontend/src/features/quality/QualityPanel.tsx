import { useEffect, useState } from "react";
import { api, type QualityPage, type Kind } from "../../api";
import { PageNavigation as Pages } from "../../ui/PageNavigation";
import type { AsyncTaskRunner as Run } from "../../ui/types";
export function QualityPanel({
  busy,
  run,
  onOpen,
}: {
  busy: boolean;
  run: Run;
  onOpen: (kind: Kind, id: string) => void;
}) {
  const [data, setData] = useState<QualityPage | null>(null),
    [code, setCode] = useState("");
  const labels: Record<string, string> = {
    missing_responsible: "Нет ответственного",
    missing_address: "Нет адреса сервера",
    production_no_policy: "Промышленная БД без политики РК",
    production_disabled_policy: "Политика РК отключена",
    duplicate_server_name: "Совпадающие имена серверов",
    duplicate_server_address: "Совпадающие адреса серверов",
    similar_sid: "Похожие SID внутри родителя",
  };
  async function load(c = code, p = 0) {
    setData(await api().GetQualityReport(c, p));
  }
  useEffect(() => {
    void run(() => load());
  }, []);
  return (
    <>
      <div className="page-heading">
        <div>
          <h1>Качество данных</h1>
          <p className="muted">
            Подозрения на дубли требуют проверки. Одинаковый SID на разных
            серверах допустим.
          </p>
        </div>
        <button disabled={busy} onClick={() => void run(() => load())}>
          Обновить
        </button>
      </div>
      <select
        aria-label="Тип проблемы"
        disabled={busy}
        value={code}
        onChange={(e) => {
          setCode(e.target.value);
          void run(() => load(e.target.value));
        }}
      >
        <option value="">Все проблемы</option>
        {Object.entries(labels).map(([k, v]) => (
          <option key={k} value={k}>
            {v} ({data?.counts[k] ?? 0})
          </option>
        ))}
      </select>
      <p>Найдено: {data?.total ?? 0}</p>
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Объект</th>
              <th>Проблема</th>
              <th />
            </tr>
          </thead>
          <tbody>
            {data?.items.map((i) => (
              <tr key={i.code + i.id}>
                <td>{i.path}</td>
                <td>{i.detail}</td>
                <td>
                  <button disabled={busy} onClick={() => onOpen(i.kind, i.id)}>
                    Открыть карточку
                  </button>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {data && (
        <Pages
          page={data.page}
          total={data.total}
          busy={busy}
          onPage={(p) => void run(() => load(code, p))}
        />
      )}
    </>
  );
}
