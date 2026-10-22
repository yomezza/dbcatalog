import { useState } from "react";
import {
  api,
  type ImportPreview,
  type ImportResult,
  type ImportRowPreview,
  type Lookups,
} from "./api";
import { ImportRowEditor } from "./ImportRowEditor";
interface Props {
  lookups: Lookups;
  error: string;
  onDownloads: () => void;
  canEdit: boolean;
  busy: boolean;
  run: (task: () => Promise<void>) => Promise<void>;
  onImported: () => Promise<unknown>;
}
export function ExcelPanel({
  canEdit,
  busy,
  run,
  onImported,
  lookups,
  error,
  onDownloads,
}: Props) {
  const [preview, setPreview] = useState<ImportPreview | null>(null),
    [result, setResult] = useState<ImportResult | null>(null),
    [skipInvalid, setSkipInvalid] = useState(false),
    [catalogOnly, setCatalogOnly] = useState(false),
    [onlyErrors, setOnlyErrors] = useState(false),
    [page, setPage] = useState(0),
    [editing, setEditing] = useState<ImportRowPreview | null>(null),
    [downloadNotice, setDownloadNotice] = useState("");
  async function fileSelected(file: File) {
    setPreview(null);
    setResult(null);
    setPage(0);
    setSkipInvalid(false);
    setCatalogOnly(false);
    setOnlyErrors(false);
    if (file.size > 20 * 1024 * 1024)
      throw new Error("Максимальный размер файла — 20 МБ");
    const encoded = await new Promise<string>((resolve, reject) => {
      const reader = new FileReader();
      reader.onerror = () => reject(new Error("Не удалось прочитать файл"));
      reader.onload = () => resolve(String(reader.result).split(",")[1]);
      reader.readAsDataURL(file);
    });
    setPreview(await api().PreviewExcel(encoded));
  }
  async function exportFile() {
    const file = await api().ExportToDownloads();
    setDownloadNotice(`Файл «${file.name}» сохранён в загрузки.`);
    await api().SaveDownloadAs(file.id);
  }
  const filtered =
    preview?.rows.filter((r) => !onlyErrors || r.errors.length > 0) ?? [];
  const lastPage = Math.max(0, Math.ceil(filtered.length / 50) - 1);
  const currentPage = Math.min(page, lastPage);
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">ОБМЕН ДАННЫМИ</div>
          <h1>Экспорт/Импорт</h1>
          <p className="muted">
            Утверждённый каталог на 21 или 28 колонок. Одна строка — одна база
            данных.
          </p>
        </div>
        <button disabled={busy} onClick={() => void run(exportFile)}>
          Экспорт каталога XLSX
        </button>
      </div>
      {downloadNotice && (
        <div className="notice success" role="status">
          {downloadNotice}{" "}
          <button onClick={onDownloads}>Открыть загрузки</button>
        </div>
      )}
      {canEdit && (
        <section className="excel-panel">
          <h2>Импорт XLS / XLSX</h2>
          <p className="muted">
            Выберите файл для проверки. До подтверждения данные не записываются.
            Новые и существующие объекты сопоставляются по имени сервера,
            экземпляру и SID; конфликтующие данные показываются как ошибки.
          </p>
          <label className="field">
            <span>Файл каталога (до 20 МБ)</span>
            <input
              type="file"
              accept=".xls,.xlsx"
              disabled={busy}
              onChange={(e) => {
                const file = e.target.files?.[0];
                if (file) void run(() => fileSelected(file));
                e.target.value = "";
              }}
            />
          </label>
          {preview && (
            <>
              <div className="import-summary">
                <span>
                  Лист: <b>{preview.sheet}</b>
                </span>
                <span>
                  Формат: <b>{preview.columns} колонок</b>
                </span>
                <span>
                  Строк: <b>{preview.total}</b>
                </span>
                <span>
                  Корректных: <b>{preview.valid}</b>
                </span>
                <span>
                  Ошибочных: <b>{preview.total - preview.valid}</b>
                </span>
                <span>
                  Пропущено строк шаблона: <b>{preview.ignored}</b>
                </span>
              </div>
              {preview.columns === 28 && (
                <div className="info">
                  В этой версии импорт переносит данные каталога из первых 21
                  колонки. Сведения о РК присутствуют в {preview.backupRows}{" "}
                  строках и не переносятся в политики автоматически: в шаблоне
                  нет расписания, необходимого для активной политики. Пароли
                  архива исключаются из импорта, предпросмотра и истории. Строк
                  с заполненными секретами: {preview.secretsIgnored}.
                </div>
              )}
              <label className="check">
                <input
                  type="checkbox"
                  checked={onlyErrors}
                  onChange={(e) => {
                    setOnlyErrors(e.target.checked);
                    setPage(0);
                  }}
                />
                Показывать только ошибки
              </label>
              <div className="import-table">
                <table>
                  <thead>
                    <tr>
                      <th>Строка</th>
                      <th>Сервер</th>
                      <th>Экземпляр</th>
                      <th>SID</th>
                      <th>Проверка</th>
                      <th>Исправления</th>
                    </tr>
                  </thead>
                  <tbody>
                    {filtered
                      .slice(currentPage * 50, currentPage * 50 + 50)
                      .map((r) => (
                        <tr key={r.row}>
                          <td>{r.row}</td>
                          <td>{r.server || "—"}</td>
                          <td>{r.instance || "—"}</td>
                          <td>{r.database || "—"}</td>
                          <td>
                            {r.errors.length ? r.errors.join("; ") : "Готово"}
                          </td>
                          <td>
                            <button
                              disabled={busy}
                              onClick={() => setEditing(r)}
                            >
                              {r.errors.length ? "Исправить" : "Изменить"}
                            </button>
                          </td>
                        </tr>
                      ))}
                  </tbody>
                </table>
              </div>
              <div className="pagination">
                <button
                  disabled={currentPage === 0}
                  onClick={() => setPage(currentPage - 1)}
                >
                  ←
                </button>
                <span>
                  Страница {currentPage + 1} из {lastPage + 1}
                </span>
                <button
                  disabled={currentPage === lastPage}
                  onClick={() => setPage(currentPage + 1)}
                >
                  →
                </button>
              </div>
              {preview.valid !== preview.total && (
                <label className="check">
                  <input
                    type="checkbox"
                    checked={skipInvalid}
                    onChange={(e) => setSkipInvalid(e.target.checked)}
                  />
                  Пропустить {preview.total - preview.valid} ошибочных строк,
                  импортировать только проверенные
                </label>
              )}
              {preview.columns === 28 && (
                <label className="check">
                  <input
                    type="checkbox"
                    checked={catalogOnly}
                    onChange={(e) => setCatalogOnly(e.target.checked)}
                  />
                  Подтверждаю импорт только каталога без политик РК и паролей
                  архива
                </label>
              )}
              <div className="actions">
                <button
                  disabled={busy}
                  onClick={() =>
                    void run(async () => {
                      const file = await api().ExportImportPreview(
                        preview.token,
                      );
                      setDownloadNotice(
                        `Файл «${file.name}» сохранён в загрузки.`,
                      );
                    })
                  }
                >
                  Скачать исправленную копию (21 колонка)
                </button>
              </div>
              <button
                className="primary"
                disabled={
                  busy ||
                  !preview.valid ||
                  (preview.valid !== preview.total && !skipInvalid) ||
                  (preview.columns === 28 && !catalogOnly)
                }
                onClick={() =>
                  void run(async () => {
                    const report = await api().ApplyExcel(
                      preview.token,
                      skipInvalid,
                      catalogOnly,
                    );
                    setResult(report);
                    setPreview(null);
                    await onImported();
                  })
                }
              >
                Подтвердить импорт {preview.valid} строк
              </button>
            </>
          )}
          {result && (
            <div className="notice success" role="status">
              Импорт завершён. Создано серверов: {result.serversCreated},
              экземпляров: {result.instancesCreated}, БД:{" "}
              {result.databasesCreated}. Обновлено объектов:{" "}
              {result.objectsUpdated}. Пропущено ошибочных строк:{" "}
              {result.skipped}.
            </div>
          )}
        </section>
      )}
      {editing && preview && (
        <ImportRowEditor
          row={editing}
          lookups={lookups}
          busy={busy}
          error={error}
          onClose={() => setEditing(null)}
          onSave={async (values) => {
            await run(async () => {
              const next = await api().RevalidateExcel(preview.token, [
                { row: editing.row, values },
              ]);
              setPreview(next);
              setEditing(null);
              setSkipInvalid(false);
              setCatalogOnly(false);
              setPage(0);
            });
          }}
        />
      )}
      {!canEdit && (
        <div className="info">
          Для импорта требуется роль редактора или администратора. Экспорт
          доступен для просмотра каталога.
        </div>
      )}
    </>
  );
}
