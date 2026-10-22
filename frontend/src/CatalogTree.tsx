import { oracleGroupKey } from "./catalog";
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
    byInstance = new Map<string, CatalogObject[]>(),
    oracleByServer = new Map<string, CatalogObject[]>();
  for (const i of catalog.instances) {
    const a = byServer.get(i.parentId) ?? [];
    a.push(i);
    byServer.set(i.parentId, a);
  }
  for (const d of catalog.databases) {
    const map = d.dbms === "Oracle Database" ? oracleByServer : byInstance;
    const a = map.get(d.parentId) ?? [];
    a.push(d);
    map.set(d.parentId, a);
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
            {children.map(branch)}
            {o.kind === "server" &&
              (oracleByServer.get(o.id)?.length ?? 0) > 0 &&
              oracleGroup(o.id)}
            {!children.length &&
              !(o.kind === "server" && oracleByServer.has(o.id)) && (
                <p className="empty-branch">Дочерних записей нет</p>
              )}
          </div>
        )}
      </div>
    );
  }
  function oracleGroup(serverId: string) {
    const id = oracleGroupKey(serverId),
      open = expanded.has(id);
    return (
      <div className="oracle-group">
        <div className="tree-row">
          <button
            className="tree-toggle"
            aria-label={`${open ? "Свернуть" : "Раскрыть"} ветку Oracle Database`}
            aria-expanded={open}
            aria-controls={`children-${id}`}
            onClick={() => onToggle(id)}
          >
            {open ? "⌄" : "›"}
          </button>
          <button
            className="tree-node oracle-group-label"
            onDoubleClick={() => onToggle(id)}
            onKeyDown={(e) => {
              if (e.key === "Enter" || e.key === " ") {
                e.preventDefault();
                onToggle(id);
              }
            }}
          >
            <span className="node-icon oracle">◈</span>
            <span>
              Oracle Database
              <small>
                {oracleByServer.get(serverId)?.length} БД · непосредственно на
                сервере
              </small>
            </span>
          </button>
        </div>
        {open && (
          <div className="branch" id={`children-${id}`}>
            {oracleByServer.get(serverId)?.map(branch)}
          </div>
        )}
      </div>
    );
  }
  return <>{servers.map(branch)}</>;
}
