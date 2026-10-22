import type { Catalog, CatalogObject } from "./api";
export function visibleServers(c: Catalog, query: string): CatalogObject[] {
  const q = query.trim().toLocaleLowerCase("ru");
  if (!q) return c.servers;
  const match = (o: CatalogObject) =>
    [o.name, o.address, o.dbms, o.description, o.purpose]
      .join(" ")
      .toLocaleLowerCase("ru")
      .includes(q);
  const matchingInstances = new Set(
    c.instances
      .filter(
        (i) =>
          match(i) || c.databases.some((d) => d.parentId === i.id && match(d)),
      )
      .map((i) => i.parentId),
  );
  return c.servers.filter((s) => match(s) || matchingInstances.has(s.id));
}
export function pageOf<T>(items: T[], page: number, size = 10): T[] {
  return items.slice(Math.max(0, page) * size, (Math.max(0, page) + 1) * size);
}
