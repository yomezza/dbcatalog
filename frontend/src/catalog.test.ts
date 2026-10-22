import { describe, it, expect } from "vitest";
import { newObject } from "./api";
import { visibleServers, pageOf } from "./catalog";
describe("hierarchy pagination and search", () => {
  const servers = Array.from({ length: 12 }, (_, i) => ({
    ...newObject("server"),
    id: `s${i}`,
    name: `server-${i}`,
  }));
  const instances = [
    { ...newObject("instance", "s11"), id: "i1", name: "Oracle production" },
  ];
  const databases = [
    { ...newObject("database", "i1"), id: "d1", name: "Payroll" },
  ];
  const catalog = { servers, instances, databases };
  it("finds ancestor server through a matching database", () => {
    expect(visibleServers(catalog, "PAYROLL").map((s) => s.id)).toEqual([
      "s11",
    ]);
  });
  it("keeps servers as the unit of pagination", () => {
    expect(pageOf(visibleServers(catalog, ""), 0)).toHaveLength(10);
    expect(pageOf(visibleServers(catalog, ""), 1).map((s) => s.id)).toEqual([
      "s10",
      "s11",
    ]);
  });
  it("finds an instance without databases and ignores surrounding spaces", () => {
    expect(
      visibleServers({ ...catalog, databases: [] }, " oracle ").map(
        (s) => s.id,
      ),
    ).toEqual(["s11"]);
  });
});

import { filterCatalogCriteria, emptyCriteria } from "./catalog";
import { newPolicy } from "./api";
describe("database filters preserve only matching ancestry", () => {
  const s = { ...newObject("server"), id: "s", name: "host" };
  const i = { ...newObject("instance", "s"), id: "i", dbms: "PostgreSQL" };
  const d = {
    ...newObject("database", "i"),
    id: "d",
    name: "payroll",
    environment: "Промышленный",
    attributes: { ...newObject("database").attributes, responsible: "Иван" },
  };
  const oracle = {
    ...newObject("database", "s"),
    id: "o",
    name: "ORCL",
    dbms: "Oracle Database",
  };
  const c = { servers: [s], instances: [i], databases: [d, oracle] };
  it("combines field filters with literal AND search along a single ancestry path", () => {
    expect(
      filterCatalogCriteria(
        c,
        "host | payroll",
        {
          ...emptyCriteria(),
          dbms: "PostgreSQL",
          responsible: "Иван",
          environment: "Промышленный",
        },
        [],
      ).databases.map((d) => d.id),
    ).toEqual(["d"]);
    expect(
      filterCatalogCriteria(
        c,
        "host | ORCL",
        { ...emptyCriteria(), dbms: "PostgreSQL" },
        [],
      ).servers,
    ).toHaveLength(0);
  });
  it("direct disabled policy overrides inherited enabled policy", () => {
    const inherited = { ...newPolicy("instance", "i"), enabled: true },
      direct = { ...newPolicy("database", "d"), enabled: false };
    expect(
      filterCatalogCriteria(c, "", { ...emptyCriteria(), policy: "active" }, [
        inherited,
        direct,
      ]).databases,
    ).toHaveLength(0);
    expect(
      filterCatalogCriteria(c, "", { ...emptyCriteria(), policy: "disabled" }, [
        inherited,
        direct,
      ]).databases.map((d) => d.id),
    ).toEqual(["d"]);
    expect(
      filterCatalogCriteria(c, "", { ...emptyCriteria(), policy: "missing" }, [
        inherited,
        direct,
      ]).databases.map((d) => d.id),
    ).toEqual(["o"]);
  });
  it("keeps direct Oracle databases under their server and filters missing responsible", () => {
    const result = filterCatalogCriteria(
      c,
      "",
      { ...emptyCriteria(), dbms: "Oracle Database", responsible: "__empty__" },
      [],
    );
    expect(result.databases.map((d) => d.id)).toEqual(["o"]);
    expect(result.instances).toHaveLength(0);
    expect(result.servers.map((s) => s.id)).toEqual(["s"]);
  });
  it("an empty filter preserves empty servers and instances", () => {
    expect(
      filterCatalogCriteria({ ...c, databases: [] }, "", emptyCriteria(), [])
        .servers,
    ).toHaveLength(1);
  });
});
