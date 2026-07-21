import { describe, expect, it } from "vitest";
import { translateEntityTypeName } from "./use-entity-type-name";

describe("translateEntityTypeName", () => {
  const messages: Record<string, string> = {
    "global.item": "Item",
    "global.location": "Location",
  };
  const t = (key: string) => messages[key] ?? key;
  const te = (key: string) => key in messages;

  it("translates seeded default type names stored as i18n keys", () => {
    expect(translateEntityTypeName("global.item", t, te)).toBe("Item");
    expect(translateEntityTypeName("global.location", t, te)).toBe("Location");
  });

  it("passes literal user-created names through untouched", () => {
    expect(translateEntityTypeName("Tote", t, te)).toBe("Tote");
    expect(translateEntityTypeName("Movable Container", t, te)).toBe("Movable Container");
  });

  it("never calls t() for names that are not known keys (no missing-key warnings)", () => {
    let called = false;
    const trackingT = (key: string) => {
      called = true;
      return t(key);
    };
    expect(translateEntityTypeName("Tote", trackingT, te)).toBe("Tote");
    expect(called).toBe(false);
  });

  it("falls back to the en locale check when the active locale lacks the key", () => {
    // Active locale (e.g. "de") has no entry; en does. te(name) is false but
    // te(name, "en") is true, so the name still translates via fallback.
    const teActiveEmpty = (key: string, locale?: string) => locale === "en" && key in messages;
    expect(translateEntityTypeName("global.item", t, teActiveEmpty)).toBe("Item");
    expect(translateEntityTypeName("Tote", t, teActiveEmpty)).toBe("Tote");
  });

  it("returns an empty string for null/undefined/empty names", () => {
    expect(translateEntityTypeName(null, t, te)).toBe("");
    expect(translateEntityTypeName(undefined, t, te)).toBe("");
    expect(translateEntityTypeName("", t, te)).toBe("");
  });
});
