import { indexCatalog, type CatalogIndex } from "./indexes";
import type { Catalog, CatalogObject } from "../api";
import { createSearchMatcher, searchTerms } from "../search";
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
export function filterCatalog(
  c: Catalog,
  query: string,
  index: CatalogIndex = indexCatalog(c),
): Catalog {
  if (!searchTerms(query).length) return c;
  const match = createSearchMatcher(query);
  const { byServer, byInstance, oracleByServer: direct } = index;
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

export function filterCatalogCriteria(
  c: Catalog,
  query: string,
  f: import("../api").CatalogCriteria,
  policies: import("../api").Policy[],
  index: CatalogIndex = indexCatalog(c),
): Catalog {
  const base = filterCatalog(c, query, index);
  if (
    !f.dbms &&
    !f.status &&
    !f.environment &&
    !f.responsible &&
    (!f.policy || f.policy === "any")
  )
    return base;
  const instances = index.instances,
    direct = new Map(
      policies.filter((p) => p.kind === "database").map((p) => [p.objectId, p]),
    ),
    inherited = new Map(
      policies.filter((p) => p.kind === "instance").map((p) => [p.objectId, p]),
    );
  const databases = base.databases.filter((d) => {
    const policy = direct.get(d.id) ?? inherited.get(d.parentId);
    return (
      (!f.dbms || (d.dbms || instances.get(d.parentId)?.dbms) === f.dbms) &&
      (!f.status || d.status === f.status) &&
      (!f.environment || d.environment === f.environment) &&
      (!f.responsible ||
        (f.responsible === "__empty__"
          ? !d.attributes.responsible.trim()
          : d.attributes.responsible === f.responsible)) &&
      (!f.policy ||
        f.policy === "any" ||
        (f.policy === "present" && !!policy) ||
        (f.policy === "missing" && !policy) ||
        (f.policy === "active" && policy?.enabled) ||
        (f.policy === "disabled" && policy?.enabled === false))
    );
  });
  const parents = new Set(databases.map((d) => d.parentId));
  const keptInstances = base.instances.filter((i) => parents.has(i.id));
  const servers = new Set([
    ...keptInstances.map((i) => i.parentId),
    ...databases
      .filter((d) => d.dbms === "Oracle Database")
      .map((d) => d.parentId),
  ]);
  return {
    databases,
    instances: keptInstances,
    servers: base.servers.filter((s) => servers.has(s.id)),
  };
}
export const emptyCriteria = (): import("../api").CatalogCriteria => ({
  query: "",
  dbms: "",
  status: "",
  environment: "",
  responsible: "",
  policy: "",
});
