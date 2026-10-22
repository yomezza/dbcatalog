import { useEffect, useState } from "react";
import {
  api,
  type CatalogObject,
  type AttachmentPage,
  type Attachment,
  type Instruction,
} from "../../api";
import { Modal } from "../../Modal";
import { PageNavigation } from "../../ui/PageNavigation";
import type { AsyncTaskRunner } from "../../ui/types";
const instructionLabels = {
  title: "Название инструкции",
  purpose: "Назначение",
  prerequisites: "Предварительные условия",
  steps: "Порядок выполнения",
  verification: "Проверка результата",
  rollback: "Откат и восстановление",
  notes: "Примечания",
};
export function AttachmentsDialog({
  object,
  busy,
  error,
  canEdit,
  canDelete,
  run,
  onClose,
}: {
  object: CatalogObject;
  busy: boolean;
  error: string;
  canEdit: boolean;
  canDelete: boolean;
  run: AsyncTaskRunner;
  onClose: () => void;
}) {
  const [data, setData] = useState<AttachmentPage | null>(null),
    [name, setName] = useState(""),
    [description, setDescription] = useState(""),
    [file, setFile] = useState<File | null>(null),
    [instruction, setInstruction] = useState<{
      id: string;
      version: number;
      value: Instruction;
    } | null>(null),
    [deleting, setDeleting] = useState<Attachment | null>(null),
    [notice, setNotice] = useState("");
  async function load(page = data?.page ?? 0) {
    setData(await api().ListAttachments(object.kind, object.id, page));
  }
  useEffect(() => {
    void run(() => load(0));
  }, [object.kind, object.id]);
  async function download(a: Attachment) {
    const f = await api().DownloadAttachment(a.id);
    setNotice("Файл сохранён в Загрузки.");
    await api().SaveDownloadAs(f.id);
  }
  return (
    <Modal
      title={"Вложения и инструкции: " + object.name}
      className="wide-modal document-dialog"
      busy={busy}
      onClose={onClose}
    >
      <p>
        Файлы хранятся в общей PostgreSQL. При переносе и восстановлении объекта
        связь сохраняется. Максимальный размер файла — 20 МБ.
      </p>
      {error && (
        <p role="alert" className="notice error">
          {error}
        </p>
      )}
      {notice && (
        <p role="status" className="notice success">
          {notice}
        </p>
      )}
      {canEdit && (
        <>
          <details>
            <summary>Добавить файл</summary>
            <div className="form-grid">
              <label className="field">
                <span>Файл</span>
                <input
                  type="file"
                  disabled={busy}
                  accept=".pdf,.docx,.xlsx,.txt,.md,.csv,.png,.jpg,.jpeg,.sql,.ps1,.json,.zip"
                  onChange={(e) => {
                    const f = e.target.files?.[0] ?? null;
                    setFile(f);
                    if (f) setName(f.name);
                  }}
                />
              </label>
              <label className="field">
                <span>Название вложения</span>
                <input
                  maxLength={200}
                  value={name}
                  disabled={busy}
                  onChange={(e) => setName(e.target.value)}
                />
              </label>
              <label className="field">
                <span>Описание вложения</span>
                <textarea
                  aria-label="Описание вложения"
                  value={description}
                  maxLength={20000}
                  disabled={busy}
                  onChange={(e) => setDescription(e.target.value)}
                />
              </label>
            </div>
            <button
              disabled={busy || !file || !name.trim()}
              onClick={() =>
                void run(async () => {
                  if (!file) return;
                  if (file.size > 20 * 1024 * 1024)
                    throw new Error("Максимальный размер файла — 20 МБ");
                  const encoded = await new Promise<string>(
                    (resolve, reject) => {
                      const reader = new FileReader();
                      reader.onload = () =>
                        resolve(String(reader.result).split(",")[1]);
                      reader.onerror = () =>
                        reject(new Error("Не удалось прочитать файл"));
                      reader.readAsDataURL(file);
                    },
                  );
                  await api().UploadAttachment({
                    kind: object.kind,
                    objectId: object.id,
                    name,
                    description,
                    fileName: file.name,
                    data: encoded,
                  });
                  setFile(null);
                  setName("");
                  setDescription("");
                  await load(0);
                })
              }
            >
              Загрузить вложение
            </button>
          </details>
          <h3>Сформировать инструкцию</h3>
          <p className="muted">
            Выберите шаблон, дополните порядок работ и сохраните документ Word.
            Команды для конкретной СУБД необходимо указать перед использованием.
          </p>
          <div className="actions">
            {[
              ["connection", "Подключение"],
              ["backup", "Резервное копирование"],
              ["restore", "Восстановление"],
              ["maintenance", "Регламент"],
            ].map(([type, label]) => (
              <button
                disabled={busy}
                key={type}
                onClick={() =>
                  void run(async () =>
                    setInstruction({
                      id: "",
                      version: 0,
                      value: await api().InstructionTemplate(
                        object.kind,
                        object.id,
                        type,
                      ),
                    }),
                  )
                }
              >
                {label}
              </button>
            ))}
          </div>
        </>
      )}
      <div className="table-scroll">
        <table>
          <thead>
            <tr>
              <th>Документ</th>
              <th>Размер / автор</th>
              <th>Действия</th>
            </tr>
          </thead>
          <tbody>
            {data?.items.map((a) => (
              <tr key={a.id}>
                <td>
                  <b>{a.name}</b>
                  <small>
                    {a.fileName} · v{a.version}
                  </small>
                  <p>{a.description}</p>
                </td>
                <td>
                  {(a.size / 1024).toFixed(1)} КБ
                  <small>
                    {a.actor} · {new Date(a.updatedAt).toLocaleString("ru")}
                  </small>
                </td>
                <td>
                  <div className="actions">
                    <button
                      disabled={busy}
                      onClick={() => void run(() => download(a))}
                    >
                      Скачать
                    </button>
                    {canEdit && a.instruction && (
                      <button
                        disabled={busy}
                        onClick={() =>
                          setInstruction({
                            id: a.id,
                            version: a.version,
                            value: a.instruction!,
                          })
                        }
                      >
                        Редактировать инструкцию
                      </button>
                    )}
                    {canDelete && (
                      <button
                        disabled={busy}
                        className="danger"
                        onClick={() => setDeleting(a)}
                      >
                        Удалить вложение
                      </button>
                    )}
                  </div>
                </td>
              </tr>
            ))}
          </tbody>
        </table>
      </div>
      {data && !data.total && <p className="empty">Вложений пока нет.</p>}
      {data && (
        <PageNavigation
          page={data.page}
          total={data.total}
          busy={busy}
          onPage={(p) => void run(() => load(p))}
        />
      )}
      {instruction && (
        <Modal
          title={
            instruction.id ? "Редактирование инструкции" : "Новая инструкция"
          }
          busy={busy}
          onClose={() => setInstruction(null)}
          className="wide-modal document-dialog"
        >
          {error && (
            <p role="alert" className="notice error">
              {error}
            </p>
          )}
          {(
            [
              "title",
              "purpose",
              "prerequisites",
              "steps",
              "verification",
              "rollback",
              "notes",
            ] as const
          ).map((key) => (
            <label key={key} className="field">
              <span>{instructionLabels[key]}</span>
              {key === "title" ? (
                <input
                  disabled={busy}
                  aria-label={instructionLabels[key]}
                  value={instruction.value[key]}
                  maxLength={200}
                  onChange={(e) =>
                    setInstruction({
                      ...instruction,
                      value: { ...instruction.value, [key]: e.target.value },
                    })
                  }
                />
              ) : (
                <textarea
                  disabled={busy}
                  rows={key === "steps" ? 8 : 3}
                  aria-label={instructionLabels[key]}
                  value={instruction.value[key]}
                  maxLength={100000}
                  onChange={(e) =>
                    setInstruction({
                      ...instruction,
                      value: { ...instruction.value, [key]: e.target.value },
                    })
                  }
                />
              )}
            </label>
          ))}
          <p className="muted">
            В Word также попадут путь объекта, его версия, адрес сервера, автор
            и дата формирования. Пароли в шаблон не включаются.
          </p>
          <button
            disabled={
              busy ||
              !instruction.value.title.trim() ||
              !instruction.value.steps.trim()
            }
            className="primary"
            onClick={() =>
              void run(async () => {
                const saved = await api().SaveInstruction(
                  object.kind,
                  object.id,
                  instruction.id,
                  instruction.version,
                  instruction.value,
                );
                setInstruction(null);
                await load(0);
                setNotice(
                  "Инструкция «" +
                    saved.name +
                    "» сохранена. Её можно скачать в Word.",
                );
              })
            }
          >
            Сохранить инструкцию Word
          </button>
        </Modal>
      )}
      {deleting && (
        <Modal
          title="Удаление вложения"
          busy={busy}
          onClose={() => setDeleting(null)}
        >
          <p>Удалить «{deleting.name}» без возможности восстановления файла?</p>
          {error && (
            <p role="alert" className="notice error">
              {error}
            </p>
          )}
          <button
            disabled={busy}
            className="danger"
            onClick={() =>
              void run(async () => {
                await api().DeleteAttachment(deleting.id, deleting.version);
                setDeleting(null);
                await load();
              })
            }
          >
            Удалить файл окончательно
          </button>
        </Modal>
      )}
    </Modal>
  );
}
