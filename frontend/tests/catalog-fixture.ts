import { filterCatalogCriteria, emptyCriteria } from "../src/catalog";
import {
  indexCatalog,
  objectLocation,
  indexedObject,
} from "../src/catalog/indexes";
import { objectPath } from "../src/objectPath";
import { createSearchMatcher } from "../src/search";
import type {
  Catalog,
  CatalogBrowseRequest,
  CatalogCriteria,
  Policy,
  Kind,
} from "../src/api";
// An in-memory server fixture for UI tests. Production browsing is implemented in PostgreSQL.
export function fixtureCatalogOperation(
  op: string,
  request: any,
  c: Catalog,
  policies: Policy[],
) {
  const f: CatalogCriteria = {
    ...emptyCriteria(),
    ...(request.criteria ?? {}),
  };
  const filtered = filterCatalogCriteria(c, f.query, f, policies);
  if (op === "browse") {
    const r = request as CatalogBrowseRequest;
    let items = filtered.servers,
      pageSize = 10;
    if (r.parentKind === "server") {
      items = filtered.instances.filter((o) => o.parentId === r.parentId);
      pageSize = 100;
    }
    if (r.parentKind === "instance") {
      items = filtered.databases.filter(
        (o) => o.parentId === r.parentId && o.dbms !== "Oracle Database",
      );
      pageSize = 100;
    }
    if (r.parentKind === "oracle") {
      items = filtered.databases.filter(
        (o) => o.parentId === r.parentId && o.dbms === "Oracle Database",
      );
      pageSize = 100;
    }
    const total = items.length,
      page = Math.max(
        0,
        Math.min(r.page, Math.max(0, Math.ceil(total / pageSize) - 1)),
      ),
      children: Record<
        string,
        { instances: number; databases: number; oracle: number }
      > = {};
    for (const s of filtered.servers)
      children[s.id] = {
        instances: filtered.instances.filter((i) => i.parentId === s.id).length,
        databases: 0,
        oracle: filtered.databases.filter(
          (d) => d.parentId === s.id && d.dbms === "Oracle Database",
        ).length,
      };
    for (const i of filtered.instances)
      children[i.id] = {
        instances: 0,
        databases: filtered.databases.filter((d) => d.parentId === i.id).length,
        oracle: 0,
      };
    return {
      items: items.slice(page * pageSize, page * pageSize + pageSize),
      total,
      page,
      pageSize,
      counts: {
        servers: filtered.servers.length,
        instances: filtered.instances.length,
        databases: filtered.databases.length,
      },
      children,
    };
  }
  if (op === "context") {
    const index = indexCatalog(c),
      object = indexedObject(index, request.kind, request.id);
    if (!object) throw new Error("Объект не найден");
    const location = objectLocation(index, object),
      instance =
        object.kind === "instance"
          ? object
          : object.kind === "database" && object.dbms !== "Oracle Database"
            ? index.instances.get(object.parentId)
            : undefined;
    const server =
      object.kind === "server"
        ? object
        : index.servers.get(instance?.parentId ?? object.parentId);
    const catalog = {
        servers: server ? [server] : [],
        instances: instance ? [instance] : [],
        databases: object.kind === "database" ? [object] : [],
      },
      branchPages: Record<string, number> = {};
    if (instance && server)
      branchPages[server.id] = Math.floor(
        c.instances
          .filter((i) => i.parentId === server.id)
          .findIndex((i) => i.id === instance.id) / 100,
      );
    if (object.kind === "database")
      branchPages[
        object.dbms === "Oracle Database"
          ? "oracle:" + object.parentId
          : object.parentId
      ] = Math.floor(
        c.databases
          .filter((d) => d.parentId === object.parentId)
          .findIndex((d) => d.id === object.id) / 100,
      );
    return {
      object,
      catalog,
      policies: policies.filter(
        (p) =>
          p.objectId === object.id ||
          (p.kind === "instance" && p.objectId === instance?.id),
      ),
      path: objectPath(object, c),
      serverPage: location.page,
      branchPages,
    };
  }
  if (op === "select")
    return filtered.databases.map((d) => ({ id: d.id, version: d.version }));
  if (op === "outside") {
    const visible = new Set(filtered.databases.map((d) => d.id));
    return request.ids.filter((id: string) => !visible.has(id)).length;
  }
  if (op === "options") {
    const all =
      request.kind === "server"
        ? c.servers
        : request.kind === "instance"
          ? c.instances
          : c.databases;
    const match = createSearchMatcher(request.query);
    const items = all
      .filter((o) =>
        match([
          objectPath(o, c),
          o.name,
          o.address,
          o.dbms,
          ...Object.values(o.attributes ?? {}).flat(),
        ]),
      )
      .map((object) => ({ object, path: objectPath(object, c) }));
    return {
      items: items.slice(request.page * 25, request.page * 25 + 25),
      page: request.page,
      total: items.length,
    };
  }
  if (op === "policies") {
    const match = createSearchMatcher(request.query),
      index = indexCatalog(c),
      paths: Record<string, string> = {};
    const items = policies.filter((p) => {
      const o = indexedObject(index, p.kind as Kind, p.objectId);
      paths[p.id] = o ? objectPath(o, c) : p.objectId;
      return match([
        p.name,
        paths[p.id],
        p.backupType,
        p.scheduler,
        p.schedule,
        p.storage,
        p.enabled ? "Включен" : "Отключен",
      ]);
    });
    return {
      items: items.slice(request.page * 25, request.page * 25 + 25),
      total: items.length,
      page: request.page,
      paths,
    };
  }
  throw new Error("Unknown catalog fixture operation");
}
