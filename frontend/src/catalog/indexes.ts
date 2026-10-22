import type { Catalog, CatalogObject } from "../api";
function append(
  groups: Map<string, CatalogObject[]>,
  parent: string,
  object: CatalogObject,
) {
  const group = groups.get(parent);
  if (group) group.push(object);
  else groups.set(parent, [object]);
}
// Rebuilt only when the catalog snapshot changes, never when a row is selected.
export function indexCatalog(catalog: Catalog) {
  const servers = new Map(catalog.servers.map((o) => [o.id, o]));
  const instances = new Map(catalog.instances.map((o) => [o.id, o]));
  const databases = new Map(catalog.databases.map((o) => [o.id, o]));
  const byServer = new Map<string, CatalogObject[]>(),
    byInstance = new Map<string, CatalogObject[]>(),
    oracleByServer = new Map<string, CatalogObject[]>();
  for (const o of catalog.instances) append(byServer, o.parentId, o);
  for (const o of catalog.databases)
    append(
      o.dbms === "Oracle Database" ? oracleByServer : byInstance,
      o.parentId,
      o,
    );
  const serverPages = new Map(
    catalog.servers.map((o, i) => [o.id, Math.floor(i / 10)]),
  );
  return {
    servers,
    instances,
    databases,
    byServer,
    byInstance,
    oracleByServer,
    serverPages,
  };
}
export type CatalogIndex = ReturnType<typeof indexCatalog>;
export function indexedObject(index: CatalogIndex, kind: string, id: string) {
  switch (kind) {
    case "server":
      return index.servers.get(id);
    case "instance":
      return index.instances.get(id);
    case "database":
      return index.databases.get(id);
    default:
      return undefined;
  }
}
export function objectLocation(index: CatalogIndex, object: CatalogObject) {
  const instance =
    object.kind === "database" && object.dbms !== "Oracle Database"
      ? index.instances.get(object.parentId)
      : undefined;
  const serverId =
    object.kind === "server"
      ? object.id
      : object.kind === "instance" || object.dbms === "Oracle Database"
        ? object.parentId
        : instance?.parentId;
  return {
    page: serverId ? (index.serverPages.get(serverId) ?? 0) : 0,
    branches: [
      ...(serverId ? [serverId] : []),
      ...(instance ? [instance.id] : []),
      ...(object.dbms === "Oracle Database"
        ? ["oracle:" + object.parentId]
        : []),
    ],
  };
}
export function hiddenSelectionCount(
  selected: ReadonlyMap<string, unknown>,
  visible: ReadonlySet<string>,
) {
  let hidden = 0;
  for (const id of selected.keys()) if (!visible.has(id)) hidden++;
  return hidden;
}
