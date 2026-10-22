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
