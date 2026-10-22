import type { ReactNode } from "react";
import { api, type Lookups, type CatalogObject } from "../../api";
import { Stat } from "../../components";
import { RemoteCatalogTree } from "./RemoteCatalogTree";
import { CatalogFilters } from "./CatalogFilters";
import { BulkDialog } from "./BulkDialog";
import type { CatalogBrowser } from "./useCatalogBrowser";
import type { AsyncTaskRunner } from "../../ui/types";
interface Props {
  lookups: Lookups;
  browser: CatalogBrowser;
  busy: boolean;
  error: string;
  canEdit: boolean;
  selectedId?: string;
  onSelect: (object: CatalogObject) => void;
  onContext: (object: CatalogObject, x: number, y: number) => void;
  onRefresh: () => Promise<void>;
  onAddServer: () => void;
  onMessage: (message: string) => void;
  run: AsyncTaskRunner;
  children: ReactNode;
}
export function CatalogView({
  lookups,
  browser,
  busy,
  error,
  canEdit,
  selectedId,
  onSelect,
  onContext,
  onRefresh,
  onAddServer,
  onMessage,
  run,
  children,
}: Props) {
  const {
    query,
    criteria,
    setCriteria,
    setQuery,
    setPage,

    checked,
    setChecked,
    bulkOpen,
    setBulkOpen,
    root,
    currentPage,
    lastPage,
    hiddenCount,
    changeQuery,
  } = browser;
  const counts = root.counts;
  return (
    <>
      <div className="page-heading">
        <div>
          <div className="eyebrow">ИНФРАСТРУКТУРА</div>
          <h1>Общий каталог</h1>
          <p className="muted">Серверы, экземпляры СУБД и базы данных</p>
        </div>
        <div className="actions">
          <button disabled={busy} onClick={() => void run(onRefresh)}>
            ↻ Обновить
          </button>
          {canEdit && (
            <button disabled={busy} className="primary" onClick={onAddServer}>
              + Добавить сервер
            </button>
          )}
        </div>
      </div>
      <div className="stats">
        <Stat label="Серверов" value={counts.servers} />
        <Stat label="Экземпляров СУБД" value={counts.instances} />
        <Stat label="Баз данных" value={counts.databases} />
      </div>
      <CatalogFilters
        value={{ ...criteria, query }}
        onChange={(f) => {
          setCriteria(f);
          setQuery(f.query);
          setPage(0);
        }}
        lookups={lookups}
        busy={busy}
        run={run}
      />
      <div className="selection-toolbar">
        <span>
          БД в результате: {counts.databases} · Выбрано: {checked.size} (вне
          результата: {hiddenCount})
        </span>
        <button
          disabled={busy || browser.loading || !counts.databases}
          onClick={() =>
            void run(async () =>
              setChecked(
                new Map(
                  (await api().SelectCatalogResult(browser.scope)).map(
                    (ref) => [ref.id, ref],
                  ),
                ),
              ),
            )
          }
        >
          Выбрать все БД результата
        </button>
        <button
          disabled={busy || !checked.size}
          onClick={() => setChecked(new Map())}
        >
          Снять выбор
        </button>
        {canEdit && (
          <button
            disabled={busy || !checked.size}
            onClick={() => setBulkOpen(true)}
          >
            Массовое редактирование
          </button>
        )}
        <button
          disabled={busy || browser.loading || !counts.databases}
          onClick={() =>
            void run(async () => {
              const f = await api().ExportCatalogResult(browser.scope);
              onMessage("Результат сохранён в Загрузки");
              await api().SaveDownloadAs(f.id);
            })
          }
        >
          Экспорт результата
        </button>
        <button
          disabled={busy || !checked.size}
          onClick={() =>
            void run(async () => {
              const f = await api().ExportSelection([...checked.keys()]);
              onMessage("Выбор сохранён в Загрузки");
              await api().SaveDownloadAs(f.id);
            })
          }
        >
          Экспорт выбранного
        </button>
      </div>
      {bulkOpen && (
        <BulkDialog
          error={error}
          refs={[...checked.values()]}
          lookups={lookups}
          busy={busy}
          run={run}
          onClose={() => setBulkOpen(false)}
          onSaved={onRefresh}
        />
      )}
      <div className="catalog-layout">
        <section className="tree-panel">
          <div className="tree-tools">
            <input
              aria-label="Поиск в каталоге"
              placeholder="Например: сервер | PostgreSQL | база"
              title="Разделяйте условия символом | — должны совпасть все условия одной цепочки"
              value={query}
              onChange={(e) => changeQuery(e.target.value)}
            />
          </div>
          <div className="tree-controls">
            <button
              disabled={busy || !root.items.length}
              onClick={browser.expandAll}
            >
              Раскрыть все
            </button>
            <button disabled={busy} onClick={browser.collapseAll}>
              Свернуть все
            </button>
          </div>
          {browser.error && (
            <p className="notice error" role="alert">
              {browser.error}
            </p>
          )}
          {browser.loading ? (
            <p className="empty" role="status">
              Поиск по всему каталогу…
            </p>
          ) : (
            <RemoteCatalogTree
              browser={browser}
              selectedId={selectedId}
              busy={busy}
              onSelect={onSelect}
              onContext={onContext}
            />
          )}
          {!browser.loading && !root.total && (
            <div className="empty">
              {!browser.scope.query &&
              !browser.scope.dbms &&
              !browser.scope.status &&
              !browser.scope.environment &&
              !browser.scope.responsible &&
              (!browser.scope.policy || browser.scope.policy === "any")
                ? "Каталог пока пуст. Добавьте первый сервер."
                : "Совпадений не найдено"}
            </div>
          )}
          <div className="pagination">
            <button
              disabled={busy || browser.loading || currentPage === 0}
              onClick={() => setPage(currentPage - 1)}
            >
              ←
            </button>
            <span>
              Страница {currentPage + 1} из {lastPage + 1}
            </span>
            <button
              disabled={busy || browser.loading || currentPage === lastPage}
              onClick={() => setPage(currentPage + 1)}
            >
              →
            </button>
          </div>
        </section>
        {children}
      </div>
    </>
  );
}
