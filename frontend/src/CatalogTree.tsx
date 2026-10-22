import type { ReactNode } from "react";
import type { Catalog, CatalogObject } from "./api";
export function CatalogTree({
  catalog,
  servers,
  expanded,
  onToggle,
  renderNode,
}: {
  catalog: Catalog;
  servers: CatalogObject[];
  expanded: Set<string>;
  onToggle: (id: string) => void;
  renderNode: (o: CatalogObject) => ReactNode;
}) {
  const byServer = new Map<string, CatalogObject[]>(),
    byInstance = new Map<string, CatalogObject[]>();
  for (const i of catalog.instances) {
    const a = byServer.get(i.parentId) ?? [];
    a.push(i);
    byServer.set(i.parentId, a);
  }
  for (const d of catalog.databases) {
    const a = byInstance.get(d.parentId) ?? [];
    a.push(d);
    byInstance.set(d.parentId, a);
  }
  function branch(o: CatalogObject) {
    const children =
      o.kind === "server"
        ? (byServer.get(o.id) ?? [])
        : o.kind === "instance"
          ? (byInstance.get(o.id) ?? [])
          : [];
    const open = expanded.has(o.id);
    return (
      <div className="catalog-branch" key={o.id}>
        <div className="tree-row">
          {o.kind !== "database" ? (
            <button
              className="tree-toggle"
              aria-label={`${open ? "Свернуть" : "Раскрыть"} ветку ${o.name}`}
              aria-expanded={open}
              aria-controls={`children-${o.id}`}
              onClick={() => onToggle(o.id)}
            >
              {open ? "⌄" : "›"}
            </button>
          ) : (
            <span className="tree-toggle-spacer" />
          )}
          {renderNode(o)}
        </div>
        {o.kind !== "database" && open && (
          <div className="branch" id={`children-${o.id}`}>
            {children.length ? (
              children.map(branch)
            ) : (
              <p className="empty-branch">Дочерних записей нет</p>
            )}
          </div>
        )}
      </div>
    );
  }
  return <>{servers.map(branch)}</>;
}
