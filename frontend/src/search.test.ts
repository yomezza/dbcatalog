import { describe, it, expect } from "vitest";
import { matchesSearch, searchTerms } from "./search";
import { filterCatalog } from "./catalog";
import { newObject } from "./api";
describe("literal AND search", () => {
  it("requires all terms, ignores empty segments and case", () => {
    expect(searchTerms(" TEST1 | | test2 | TEST1 ")).toEqual([
      "test1",
      "test2",
    ]);
    expect(matchesSearch(["test1", "value TEST2"], "TEST1 | TEST2")).toBe(true);
    expect(matchesSearch(["test1"], "TEST1 | TEST2")).toBe(false);
    expect(matchesSearch(["host[1]"], "[1] | host")).toBe(true);
    expect(matchesSearch([], " | ")).toBe(true);
  });
  it("matches one ancestor path and retains the corresponding tree", () => {
    const s = { ...newObject("server"), id: "s", name: "TEST1" };
    const i = {
      ...newObject("instance", "s"),
      id: "i",
      name: "Oracle",
      dbms: "Oracle Database",
    };
    const a = { ...newObject("database", "i"), id: "a", name: "TEST2" };
    const b = { ...newObject("database", "i"), id: "b", name: "OTHER" };
    const c = { servers: [s], instances: [i], databases: [a, b] };
    expect(
      filterCatalog(c, "TEST1 | TEST2").databases.map((d) => d.id),
    ).toEqual(["a"]);
    expect(filterCatalog(c, "TEST2 | OTHER").servers).toHaveLength(0);
    expect(filterCatalog(c, "TEST1 | Oracle").databases).toHaveLength(2);
  });
});
