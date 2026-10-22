import { describe, it, expect } from "vitest";
import { newObject, newPolicy } from "./api";
import { effectivePolicy } from "./PoliciesPanel";
describe("backup policy display", () => {
  it("uses the disabled database policy instead of an inherited enabled policy", () => {
    const db = { ...newObject("database", "i"), id: "d" };
    const inherited = {
      ...newPolicy("instance", "i"),
      id: "pi",
      enabled: true,
    };
    const direct = { ...newPolicy("database", "d"), id: "pd", enabled: false };
    expect(effectivePolicy(db, [inherited, direct])).toEqual({
      policy: direct,
      inherited: false,
    });
    expect(effectivePolicy(db, [inherited])).toEqual({
      policy: inherited,
      inherited: true,
    });
  });
});
