import { describe, expect, test } from "vitest";
import { resolveEntityIconName, CONTAINER_DEFAULT_ICON, LOCATION_DEFAULT_ICON } from "./entity-icon";

const known = ["basket-outline", "safe", "map-marker-outline", "package-variant"];

describe("resolveEntityIconName", () => {
  test("entity override wins", () => {
    expect(resolveEntityIconName({ icon: "safe", typeIcon: "basket-outline", isContainer: true }, known)).toBe("safe");
  });

  test("type icon when no override", () => {
    expect(resolveEntityIconName({ icon: "", typeIcon: "basket-outline", isContainer: true }, known)).toBe(
      "basket-outline"
    );
  });

  test("container flavor default", () => {
    expect(resolveEntityIconName({ isContainer: true, isLocation: true }, known)).toBe(CONTAINER_DEFAULT_ICON);
  });

  test("location flavor default", () => {
    expect(resolveEntityIconName({ isLocation: true }, known)).toBe(LOCATION_DEFAULT_ICON);
  });

  test("unknown names fall through to flavor default", () => {
    expect(resolveEntityIconName({ icon: "bogus-icon", isLocation: true }, known)).toBe(LOCATION_DEFAULT_ICON);
  });
});
