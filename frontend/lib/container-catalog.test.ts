import { describe, expect, test } from "vitest";
import { CONTAINER_CATALOG, catalogFields } from "./container-catalog";

describe("container catalog", () => {
  test("has at least 10 entries with unique names", () => {
    expect(CONTAINER_CATALOG.length).toBeGreaterThanOrEqual(10);
    const names = new Set(CONTAINER_CATALOG.map(e => e.name));
    expect(names.size).toBe(CONTAINER_CATALOG.length);
  });

  test("catalogFields produces the three size fields", () => {
    const fields = catalogFields(CONTAINER_CATALOG[0]);
    expect(fields.map(f => f.name)).toEqual(["Capacity", "Dimensions", "Color"]);
    expect(fields.every(f => f.textValue.length > 0)).toBe(true);
  });
});
