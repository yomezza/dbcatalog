import { useState } from "react";
import { api, type CatalogObject, type PDFOptions } from "./api";
import { Modal } from "./Modal";
import type { AsyncTaskRunner } from "./ui/types";

export type PrintTarget = { object: CatalogObject } | { ids: string[] };

export function PrintPDFDialog({
  target,
  busy,
  error,
  run,
  onClose,
  onCreated,
}: {
  target: PrintTarget;
  busy: boolean;
  error: string;
  run: AsyncTaskRunner;
  onClose: () => void;
  onCreated: (message: string) => void;
}) {
  const [options, setOptions] = useState<PDFOptions>({
    layout: "detailed",
    includePolicies: true,
  });
  const batch = "ids" in target;
  return (
    <Modal title="Печать в PDF" busy={busy} onClose={onClose}>
      {error && (
        <p className="notice error" role="alert">
          {error}
        </p>
      )}
      <p>
        {batch
          ? `Выбрано БД: ${target.ids.length}. Паспорта будут объединены в один PDF.`
          : `Паспорт объекта «${target.object.name}».`}
      </p>
      <fieldset disabled={busy} className="print-pdf-options">
        <legend>Вид паспорта</legend>
        <label className="check">
          <input
            type="radio"
            name="pdf-layout"
            value="detailed"
            checked={options.layout === "detailed"}
            onChange={() => setOptions({ ...options, layout: "detailed" })}
          />
          Подробный — как в полном паспорте
        </label>
        <p className="muted">
          Крупные блоки и свободное оформление. Длинные сведения переносятся на
          следующие страницы.
        </p>
        <label className="check">
          <input
            type="radio"
            name="pdf-layout"
            value="compact"
            checked={options.layout === "compact"}
            onChange={() => setOptions({ ...options, layout: "compact" })}
          />
          Краткий — один объект на странице
        </label>
        <p className="muted">
          Все сведения в компактном макете A4. Если текст не помещается при
          читаемом размере шрифта, выберите подробный вид — данные не
          обрезаются.
        </p>
        <label className="check">
          <input
            type="checkbox"
            checked={options.includePolicies}
            onChange={(e) =>
              setOptions({ ...options, includePolicies: e.target.checked })
            }
          />
          Включать политики резервного копирования
        </label>
      </fieldset>
      {batch && (
        <p className="muted">
          До 500 БД в одном файле. Учитываются все отмеченные БД, включая
          выбранные вне текущего поиска или страницы.
        </p>
      )}
      <div className="actions">
        <button disabled={busy} onClick={onClose}>
          Отмена
        </button>
        <button
          className="primary"
          disabled={busy || (batch && target.ids.length > 500)}
          onClick={() =>
            void run(async () => {
              const file = batch
                ? await api().ExportSelectionPDF(target.ids, options)
                : await api().ExportObjectPDFWithOptions(
                    target.object.kind,
                    target.object.id,
                    options,
                  );
              onCreated("PDF-паспорт сохранён в «Загрузки»");
              onClose();
              await api().SaveDownloadAs(file.id);
            })
          }
        >
          {busy ? "Формирование PDF…" : "Сохранить PDF"}
        </button>
      </div>
    </Modal>
  );
}
