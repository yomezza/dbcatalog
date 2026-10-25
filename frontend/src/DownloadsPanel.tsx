import { useEffect, useState } from "react";
import { api, type DownloadEntry } from "./api";
import { matchesSearch } from "./search";
import { isAndroid } from "./android";
export function formatSize(n: number) {
  return n < 1024
    ? `${n} Б`
    : n < 1024 * 1024
      ? `${(n / 1024).toFixed(1)} КБ`
      : `${(n / 1024 / 1024).toFixed(1)} МБ`;
}
export function DownloadsPanel({
  busy,
  run,
}: {
  busy: boolean;
  run: (fn: () => Promise<void>) => Promise<void>;
}) {
  const [items, setItems] = useState<DownloadEntry[]>([]),
    [query, setQuery] = useState(""),
    [page, setPage] = useState(0),
    [notice, setNotice] = useState("");
  const load = () => run(async () => setItems(await api().ListDownloads()));
  useEffect(() => {
    void load();
  }, []);
  const filtered = items.filter((e) =>
      matchesSearch([e.name, e.category], query),
    ),
    last = Math.max(0, Math.ceil(filtered.length / 25) - 1),
    current = Math.min(last, page);
  const action = (e: DownloadEntry, type: "open" | "show" | "save") =>
    run(async () => {
      setNotice("");
      if (type === "open") await api().OpenDownload(e.id);
      else if (type === "show") await api().ShowDownload(e.id);
      else if (await api().SaveDownloadAs(e.id))
        setNotice("Копия файла сохранена");
    });
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">ФАЙЛЫ НА ЭТОМ КОМПЬЮТЕРЕ</div>
          <h1>Загрузки</h1>
          <p className="muted">
            Экспорты, копии вложений, инструкции и исправленные файлы импорта
            текущего подключения и пользователя.
          </p>
        </div>
        <button disabled={busy} onClick={() => void load()}>
          ↻ Обновить
        </button>
      </div>
      <p className="info">
        Файлы сохраняются в каталоге приложения и остаются доступны после
        перезапуска. «Сохранить как…» создаёт копию в выбранной папке.
      </p>
      <input
        aria-label="Поиск загрузок"
        placeholder="Название | категория"
        value={query}
        onChange={(e) => {
          setQuery(e.target.value);
          setPage(0);
        }}
      />
      {notice && (
        <p role="status" className="notice success">
          {notice}
        </p>
      )}
      <section className="table-panel downloads-table">
        <table>
          <thead>
            <tr>
              <th>Файл</th>
              <th>Дата</th>
              <th>Размер</th>
              <th>Действия</th>
            </tr>
          </thead>
          <tbody>
            {filtered.slice(current * 25, current * 25 + 25).map((e) => (
              <tr key={e.id}>
                <td>
                  <strong>{e.name}</strong>
                  <small className="table-caption">
                    {e.category}
                    {!e.available && " · Файл недоступен"}
                  </small>
                </td>
                <td>{new Date(e.createdAt).toLocaleString("ru")}</td>
                <td>{formatSize(e.size)}</td>
                <td>
                  <div className="actions">
                    <button
                      disabled={
                        busy || !e.available || /\.(ps1|sql)$/i.test(e.name)
                      }
                      title={
                        /\.(ps1|sql)$/i.test(e.name)
                          ? "Сохраните скрипт и откройте в выбранном редакторе"
                          : undefined
                      }
                      onClick={() => void action(e, "open")}
                    >
                      Открыть
                    </button>
                    <button
                      disabled={busy || !e.available}
                      onClick={() => void action(e, "show")}
                    >
                      {isAndroid() ? "Поделиться" : "В папке"}
                    </button>
                    <button
                      disabled={busy || !e.available}
                      onClick={() => void action(e, "save")}
                    >
                      Сохранить как…
                    </button>
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
        {!filtered.length && (
          <div className="empty">
            Загрузок пока нет. Создайте экспорт в разделе «Экспорт/Импорт».
          </div>
        )}
        <div className="pagination">
          <button disabled={!current} onClick={() => setPage(current - 1)}>
            ←
          </button>
          <span>
            Файлов: {filtered.length} · Страница {current + 1} из {last + 1}
          </span>
          <button
            disabled={current === last}
            onClick={() => setPage(current + 1)}
          >
            →
          </button>
        </div>
      </section>
    </>
  );
}
