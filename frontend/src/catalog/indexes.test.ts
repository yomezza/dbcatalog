import { describe, it, expect } from "vitest";
import { newObject } from "../api";
import {
  indexCatalog,
  indexedObject,
  objectLocation,
  hiddenSelectionCount,
} from "./indexes";
describe("catalog index and navigation", () => {
  const servers = Array.from({ length: 12 }, (_, i) => ({
    ...newObject("server"),
    id: "s" + i,
  }));
  const instance = { ...newObject("instance", "s11"), id: "i" };
  const db = { ...newObject("database", "i"), id: "d" };
  const oracle = {
    ...newObject("database", "s11"),
    id: "o",
    dbms: "Oracle Database",
  };
  const index = indexCatalog({
    servers,
    instances: [instance],
    databases: [db, oracle],
  });
  it("separates Oracle siblings from instance databases", () => {
    expect(index.byServer.get("s11")).toEqual([instance]);
    expect(index.byInstance.get("i")).toEqual([db]);
    expect(index.oracleByServer.get("s11")).toEqual([oracle]);
  });
  it("reveals the right page and ancestors for each database model", () => {
    expect(objectLocation(index, db)).toEqual({
      page: 1,
      branches: ["s11", "i"],
    });
    expect(objectLocation(index, oracle)).toEqual({
      page: 1,
      branches: ["s11", "oracle:s11"],
    });
    expect(objectLocation(index, servers[0])).toEqual({
      page: 0,
      branches: ["s0"],
    });
  });
  it("keeps selected IDs independent from visibility and checks entity kinds", () => {
    const selected = new Map([
      ["d", 1],
      ["o", 2],
      ["missing", 3],
    ]);
    expect(hiddenSelectionCount(selected, new Set(["d"]))).toBe(2);
    expect(selected.size).toBe(3);
    expect(indexedObject(index, "database", "d")).toBe(db);
    expect(indexedObject(index, "server", "d")).toBeUndefined();
    expect(indexedObject(index, "policy", "d")).toBeUndefined();
  });
});
