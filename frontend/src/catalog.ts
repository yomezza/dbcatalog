import type { Catalog, CatalogObject } from "./api";
import { createSearchMatcher, searchTerms } from "./search";
function values(o: CatalogObject): unknown[] {
  return [
    o.name,
    o.address,
    o.dbms,
    o.dbmsVersion,
    o.port,
    o.description,
    o.purpose,
    o.os,
    o.environment,
    o.status,
    ...Object.values(o.attributes ?? {}).flat(),
  ];
}
// Match each complete ancestor path, never combine unrelated sibling databases.
export function filterCatalog(c: Catalog, query: string): Catalog {
  if (!searchTerms(query).length) return c;
  const match = createSearchMatcher(query);
  const byServer = new Map<string, CatalogObject[]>(),
    byInstance = new Map<string, CatalogObject[]>();
  for (const o of c.instances) {
    const list = byServer.get(o.parentId) ?? [];
    list.push(o);
    byServer.set(o.parentId, list);
  }
  for (const o of c.databases) {
    const list = byInstance.get(o.parentId) ?? [];
    list.push(o);
    byInstance.set(o.parentId, list);
  }
  const direct = new Map<string, CatalogObject[]>();
  for (const d of c.databases.filter((d) => d.dbms === "Oracle Database")) {
    const a = direct.get(d.parentId) ?? [];
    a.push(d);
    direct.set(d.parentId, a);
  }
  const servers: CatalogObject[] = [],
    instances: CatalogObject[] = [],
    databases: CatalogObject[] = [];
  for (const server of c.servers) {
    const sv = values(server),
      serverMatches = match(sv);
    let serverKept = serverMatches;
    for (const instance of byServer.get(server.id) ?? []) {
      const iv = [...sv, ...values(instance)],
        instanceMatches = serverMatches || match(iv);
      let instanceKept = instanceMatches;
      for (const db of byInstance.get(instance.id) ?? []) {
        if (instanceMatches || match([...iv, ...values(db)])) {
          databases.push(db);
          instanceKept = true;
        }
      }
      if (instanceKept) {
        instances.push(instance);
        serverKept = true;
      }
    }
    for (const db of direct.get(server.id) ?? []) {
      if (serverMatches || match([...sv, ...values(db)])) {
        databases.push(db);
        serverKept = true;
      }
    }
    if (serverKept) servers.push(server);
  }
  return { servers, instances, databases };
}
export function visibleServers(c: Catalog, query: string): CatalogObject[] {
  return filterCatalog(c, query).servers;
}
export function pageOf<T>(items: T[], page: number, size = 10): T[] {
  return items.slice(Math.max(0, page) * size, (Math.max(0, page) + 1) * size);
}

export function oracleGroupKey(serverId: string) {
  return `oracle:${serverId}`;
}
export function branchKeys(c: Catalog) {
  return [
    ...c.servers.map((o) => o.id),
    ...c.instances.map((o) => o.id),
    ...new Set(
      c.databases
        .filter((o) => o.dbms === "Oracle Database")
        .map((o) => oracleGroupKey(o.parentId)),
    ),
  ];
}
