import { describe, expect, test } from "vitest";
import { unicodeLength } from "./utils";

describe("unicodeLength", () => {
  test("matches backend rune counting for ASCII and non-ASCII text", () => {
    expect(unicodeLength("homebox")).toBe(7);
    expect(unicodeLength("café")).toBe(4);
    expect(unicodeLength("📦")).toBe(1);
  });
});
