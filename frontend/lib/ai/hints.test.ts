import { describe, expect, test } from "vitest";
import { matchHintToTag } from "./hints";

const tags = [
  { id: "t1", name: "Power Tool" },
  { id: "t2", name: "kitchen" },
];

describe("matchHintToTag", () => {
  test("case-insensitive match", () => {
    expect(matchHintToTag("power tool", tags)?.id).toBe("t1");
    expect(matchHintToTag("KITCHEN", tags)?.id).toBe("t2");
  });

  test("trims whitespace", () => {
    expect(matchHintToTag("  power tool ", tags)?.id).toBe("t1");
  });

  test("no match returns null", () => {
    expect(matchHintToTag("garden", tags)).toBeNull();
  });

  test("empty hint returns null", () => {
    expect(matchHintToTag("   ", tags)).toBeNull();
  });
});
