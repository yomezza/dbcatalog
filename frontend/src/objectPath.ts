import type { Catalog, CatalogObject } from "./api";
export function objectPath(o: CatalogObject, catalog: Catalog): string {
  if (o.kind === "server") return o.name + (o.address ? " • " + o.address : "");
  if (o.kind === "instance") {
    const parent = catalog.servers.find((s) => s.id === o.parentId);
    return (parent ? parent.name + " / " : "") + o.name + " [" + o.dbms + "]";
  }
  if (o.dbms === "Oracle Database") {
    const server = catalog.servers.find((s) => s.id === o.parentId);
    return (server ? server.name + " / " : "") + "Oracle Database / " + o.name;
  }
  const instance = catalog.instances.find((i) => i.id === o.parentId);
  return (instance ? objectPath(instance, catalog) + " / " : "") + o.name;
}
