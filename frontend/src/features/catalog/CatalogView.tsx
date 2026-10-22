import type { ReactNode } from "react";
import { api, type Catalog, type Lookups, type CatalogObject } from "../../api";
import { branchKeys, pageOf } from "../../catalog";
import { Stat } from "../../components";
import { CatalogTree } from "../../CatalogTree";
import { CatalogFilters } from "./CatalogFilters";
import { BulkDialog } from "./BulkDialog";
import { CatalogNode } from "./CatalogNode";
import type { CatalogBrowser } from "./useCatalogBrowser";
import type { AsyncTaskRunner } from "../../ui/types";
interface Props {
  catalog: Catalog;
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
  catalog,
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
    expanded,
    setExpanded,
    toggleBranch,
    checked,
    setChecked,
    bulkOpen,
    setBulkOpen,
    filtered,
    currentPage,
    lastPage,
    hiddenCount,
    changeQuery,
  } = browser;
  const matches = filtered.servers;
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
        <Stat label="Серверов" value={catalog.servers.length} />
        <Stat label="Экземпляров СУБД" value={catalog.instances.length} />
        <Stat label="Баз данных" value={catalog.databases.length} />
      </div>
      <CatalogFilters
        value={{ ...criteria, query }}
        onChange={(f) => {
          setCriteria(f);
          setQuery(f.query);
          setPage(0);
        }}
        catalog={catalog}
        lookups={lookups}
        busy={busy}
        run={run}
      />
      <div className="selection-toolbar">
        <span>
          БД в результате: {filtered.databases.length} · Выбрано: {checked.size}{" "}
          (вне результата: {hiddenCount})
        </span>
        <button
          disabled={busy || !filtered.databases.length}
          onClick={() =>
            setChecked(
              new Map(
                filtered.databases.map((d) => [
                  d.id,
                  { id: d.id, version: d.version },
                ]),
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
          disabled={busy || !filtered.databases.length}
          onClick={() =>
            void run(async () => {
              const f = await api().ExportSelection(
                filtered.databases.map((d) => d.id),
              );
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
              disabled={busy || !matches.length}
              onClick={() => setExpanded(new Set(branchKeys(filtered)))}
            >
              Раскрыть все
            </button>
            <button
              disabled={busy || !expanded.size}
              onClick={() => setExpanded(new Set())}
            >
              Свернуть все
            </button>
          </div>
          <div className="tree">
            <CatalogTree
              catalog={filtered}
              servers={pageOf(matches, currentPage)}
              expanded={expanded}
              onToggle={toggleBranch}
              renderNode={(o) => (
                <CatalogNode
                  object={o}
                  selected={selectedId === o.id}
                  busy={busy}
                  checked={checked.has(o.id)}
                  onSelect={onSelect}
                  onToggle={toggleBranch}
                  onContext={onContext}
                  onCheck={(o, value) =>
                    setChecked((old) => {
                      const next = new Map(old);
                      if (value)
                        next.set(o.id, { id: o.id, version: o.version });
                      else next.delete(o.id);
                      return next;
                    })
                  }
                />
              )}
            />
            {matches.length === 0 && (
              <div className="empty">
                {query
                  ? "Совпадений не найдено"
                  : "Каталог пока пуст. Добавьте первый сервер."}
              </div>
            )}
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
        </section>
        {children}
      </div>
    </>
  );
}
