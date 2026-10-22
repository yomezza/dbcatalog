import { useEffect, useMemo, useRef, useState } from "react";
import type { CatalogObject } from "../../api";
import type { CatalogBrowser } from "./useCatalogBrowser";
import { CatalogNode } from "./CatalogNode";
interface Props {
  browser: CatalogBrowser;
  selectedId?: string;
  busy: boolean;
  onSelect: (o: CatalogObject) => void;
  onContext: (o: CatalogObject, x: number, y: number) => void;
}
type Row = { key: string; depth: number } & (
  | { type: "object"; object: CatalogObject }
  | { type: "oracle"; server: string; count: number }
  | {
      type: "load";
      kind: "server" | "instance" | "oracle";
      id: string;
      error: string;
    }
  | {
      type: "pager";
      kind: "server" | "instance" | "oracle";
      id: string;
      page: number;
      total: number;
    }
  | { type: "empty" }
);
const ROW_HEIGHT = 52;
export function RemoteCatalogTree({
  browser: b,
  selectedId,
  busy,
  onSelect,
  onContext,
}: Props) {
  const ref = useRef<HTMLDivElement>(null),
    [scroll, setScroll] = useState(0),
    [height, setHeight] = useState(560),
    focused = useRef(-1);
  const rows = useMemo(() => {
    const result: Row[] = [];
    const counts = { ...b.root.children };
    for (const branch of Object.values(b.branches))
      Object.assign(counts, branch.data?.children ?? {});
    function children(
      kind: "server" | "instance" | "oracle",
      id: string,
      depth: number,
      total: number,
    ) {
      const key = kind === "oracle" ? "oracle:" + id : id,
        branch = b.branches[key];
      if (total === 0) {
        if (kind !== "server" || !counts[id]?.oracle)
          result.push({ key: key + ":empty", type: "empty", depth });
        return;
      }
      if (!branch?.data) {
        result.push({
          key: key + ":load",
          type: "load",
          kind,
          id,
          depth,
          error: branch?.error ?? "",
        });
        return;
      }
      for (const o of branch.data.items) object(o, depth);
      if (branch.data.total > branch.data.pageSize)
        result.push({
          key: key + ":pager",
          type: "pager",
          kind,
          id,
          depth,
          page: branch.data.page,
          total: branch.data.total,
        });
    }
    function object(o: CatalogObject, depth: number) {
      result.push({
        key: o.kind + ":" + o.id,
        type: "object",
        object: o,
        depth,
      });
      if (o.kind === "database" || !b.isExpanded(o.id)) return;
      if (o.kind === "server") {
        children("server", o.id, depth + 1, counts[o.id]?.instances ?? -1);
        const count = counts[o.id]?.oracle ?? 0;
        if (count) {
          const group = "oracle:" + o.id;
          result.push({
            key: group,
            type: "oracle",
            server: o.id,
            count,
            depth: depth + 1,
          });
          if (b.isExpanded(group)) children("oracle", o.id, depth + 2, count);
        }
      } else
        children("instance", o.id, depth + 1, counts[o.id]?.databases ?? -1);
    }
    for (const o of b.root.items) object(o, 0);
    return result;
  }, [b.root, b.branches, b.expanded, b.isExpanded]);
  const start = Math.max(0, Math.floor(scroll / ROW_HEIGHT) - 6),
    end = Math.min(rows.length, Math.ceil((scroll + height) / ROW_HEIGHT) + 6),
    visible = rows.slice(start, end);
  useEffect(() => {
    const el = ref.current;
    if (!el) return;
    const resize = new ResizeObserver(() => setHeight(el.clientHeight));
    resize.observe(el);
    return () => resize.disconnect();
  }, []);
  useEffect(() => {
    const el = ref.current;
    if (el) {
      el.scrollTop = 0;
      setScroll(0);
    }
  }, [b.loadEpoch]);
  useEffect(() => {
    for (const row of visible)
      if (row.type === "load" && !row.error)
        void b.ensureBranch(row.kind, row.id);
  }, [visible, b.ensureBranch]);
  useEffect(() => {
    if (!b.focusRevision || focused.current === b.focusRevision || !selectedId)
      return;
    const index = rows.findIndex(
      (r) => r.type === "object" && r.object.id === selectedId,
    );
    if (index >= 0 && ref.current) {
      ref.current.scrollTop = Math.max(0, index * ROW_HEIGHT - height / 3);
      setScroll(ref.current.scrollTop);
      focused.current = b.focusRevision;
    }
  }, [rows, b.focusRevision, selectedId, height]);
  function changeBranchPage(
    row: Extract<Row, { type: "pager" }>,
    page: number,
  ) {
    const parent = rows.findIndex((r) =>
      row.kind === "oracle"
        ? r.key === "oracle:" + row.id
        : r.type === "object" && r.object.id === row.id,
    );
    if (parent >= 0 && ref.current) {
      ref.current.scrollTop = parent * ROW_HEIGHT;
      setScroll(ref.current.scrollTop);
    }
    void b.ensureBranch(row.kind, row.id, page);
  }
  return (
    <div
      className="virtual-tree"
      ref={ref}
      onScroll={(e) => setScroll(e.currentTarget.scrollTop)}
      aria-label="Дерево каталога"
      data-visible-rows={visible.length}
      data-loaded-rows={rows.length}
    >
      <div style={{ height: rows.length * ROW_HEIGHT, position: "relative" }}>
        {visible.map((row, offset) => (
          <div
            key={row.key}
            className="virtual-tree-row"
            style={{
              top: (start + offset) * ROW_HEIGHT,
              height: ROW_HEIGHT,
              paddingLeft: row.depth * 18,
            }}
          >
            {row.type === "object" && (
              <>
                <span className="tree-toggle-slot">
                  {row.object.kind !== "database" ? (
                    <button
                      className="tree-toggle"
                      aria-label={
                        (b.isExpanded(row.object.id)
                          ? "Свернуть"
                          : "Раскрыть") +
                        " ветку " +
                        row.object.name
                      }
                      aria-expanded={b.isExpanded(row.object.id)}
                      onClick={() => b.toggleBranch(row.object.id)}
                    >
                      {b.isExpanded(row.object.id) ? "⌄" : "›"}
                    </button>
                  ) : null}
                </span>
                <CatalogNode
                  object={row.object}
                  selected={selectedId === row.object.id}
                  checked={b.checked.has(row.object.id)}
                  busy={busy}
                  onSelect={onSelect}
                  onContext={onContext}
                  onToggle={b.toggleBranch}
                  onCheck={(o, value) =>
                    b.setChecked((old) => {
                      const next = new Map(old);
                      if (value)
                        next.set(o.id, { id: o.id, version: o.version });
                      else next.delete(o.id);
                      return next;
                    })
                  }
                />
              </>
            )}
            {row.type === "oracle" && (
              <>
                <button
                  className="tree-toggle"
                  aria-label={
                    (b.isExpanded(row.key) ? "Свернуть" : "Раскрыть") +
                    " ветку Oracle Database"
                  }
                  aria-expanded={b.isExpanded(row.key)}
                  onClick={() => b.toggleBranch(row.key)}
                >
                  {b.isExpanded(row.key) ? "⌄" : "›"}
                </button>
                <button
                  className="tree-node oracle-group-label"
                  onDoubleClick={() => b.toggleBranch(row.key)}
                  onKeyDown={(e) => {
                    if (e.key === "Enter" || e.key === " ") {
                      e.preventDefault();
                      b.toggleBranch(row.key);
                    }
                  }}
                >
                  <span className="node-icon oracle">◈</span>
                  <span>
                    Oracle Database
                    <small>{row.count} БД · непосредственно на сервере</small>
                  </span>
                </button>
              </>
            )}
            {row.type === "load" &&
              (row.error ? (
                <>
                  <small role="alert">{row.error}</small>
                  <button
                    disabled={busy}
                    onClick={() =>
                      void b.ensureBranch(row.kind, row.id, undefined, true)
                    }
                  >
                    Повторить загрузку
                  </button>
                </>
              ) : (
                <small className="muted">Загрузка ветви…</small>
              ))}
            {row.type === "empty" && (
              <small className="muted">Дочерних записей нет</small>
            )}
            {row.type === "pager" && (
              <div className="branch-pages">
                <button
                  disabled={busy || row.page === 0}
                  onClick={() => changeBranchPage(row, row.page - 1)}
                >
                  ←
                </button>
                <small>
                  Ветка: {row.page + 1} / {Math.ceil(row.total / 100)} ·{" "}
                  {row.total}
                </small>
                <button
                  disabled={busy || (row.page + 1) * 100 >= row.total}
                  onClick={() => changeBranchPage(row, row.page + 1)}
                >
                  →
                </button>
              </div>
            )}
          </div>
        ))}
      </div>
    </div>
  );
}
