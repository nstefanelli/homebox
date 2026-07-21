import { describe, expect, test } from "vitest";
import { parseQuickAddLine, splitQuickAddLines } from "./quick-add";

describe("parseQuickAddLine", () => {
  test("parses a leading '<int>x ' prefix into quantity + stripped name", () => {
    expect(parseQuickAddLine("3x AA batteries")).toEqual({ name: "AA batteries", quantity: 3 });
  });

  test("parses the spaced '<int> x ' variant", () => {
    expect(parseQuickAddLine("3 x AA batteries")).toEqual({ name: "AA batteries", quantity: 3 });
  });

  test("is case-insensitive on the x", () => {
    expect(parseQuickAddLine("4X Baby Hats")).toEqual({ name: "Baby Hats", quantity: 4 });
    expect(parseQuickAddLine("4 X Baby Hats")).toEqual({ name: "Baby Hats", quantity: 4 });
  });

  test("trims surrounding whitespace before parsing", () => {
    expect(parseQuickAddLine("  2x onesies  ")).toEqual({ name: "onesies", quantity: 2 });
  });

  test("clamps quantities to 1..999", () => {
    expect(parseQuickAddLine("0x socks")).toEqual({ name: "socks", quantity: 1 });
    expect(parseQuickAddLine("999x socks")).toEqual({ name: "socks", quantity: 999 });
    expect(parseQuickAddLine("1000x socks")).toEqual({ name: "socks", quantity: 999 });
    expect(parseQuickAddLine("99999999999999999999x socks")).toEqual({ name: "socks", quantity: 999 });
  });

  test("'3x' with no name is a literal name", () => {
    expect(parseQuickAddLine("3x")).toEqual({ name: "3x", quantity: 1 });
  });

  test("prefix followed by only whitespace is a literal name", () => {
    // "3x " trims to "3x"; "3x  " (interior spaces survive an outer trim)
    // matches the grammar but yields an empty name, so it stays literal.
    expect(parseQuickAddLine("3x   ")).toEqual({ name: "3x", quantity: 1 });
  });

  test("non-matching prefixes are literal names", () => {
    expect(parseQuickAddLine("3xfoo")).toEqual({ name: "3xfoo", quantity: 1 });
    expect(parseQuickAddLine("3.5x foo")).toEqual({ name: "3.5x foo", quantity: 1 });
    expect(parseQuickAddLine("-3x foo")).toEqual({ name: "-3x foo", quantity: 1 });
    expect(parseQuickAddLine("x foo")).toEqual({ name: "x foo", quantity: 1 });
    expect(parseQuickAddLine("3  x foo")).toEqual({ name: "3  x foo", quantity: 1 });
  });

  test("extra whitespace after the prefix is stripped from the name", () => {
    expect(parseQuickAddLine("3x  foo")).toEqual({ name: "foo", quantity: 3 });
  });

  test("an x later in the name is not a prefix", () => {
    expect(parseQuickAddLine("box of 3x cables")).toEqual({ name: "box of 3x cables", quantity: 1 });
  });

  test("plain names pass through with quantity 1", () => {
    expect(parseQuickAddLine("AA batteries")).toEqual({ name: "AA batteries", quantity: 1 });
  });
});

describe("splitQuickAddLines", () => {
  test("splits on newlines, trims, and drops empties", () => {
    expect(splitQuickAddLines("a\n  b \n\n   \nc\n")).toEqual(["a", "b", "c"]);
  });

  test("handles CRLF line endings", () => {
    expect(splitQuickAddLines("a\r\nb\r\n\r\nc")).toEqual(["a", "b", "c"]);
  });

  test("whitespace-only input yields no lines", () => {
    expect(splitQuickAddLines("   \n \r\n\t\n")).toEqual([]);
  });

  test("single line without newline is one entry", () => {
    expect(splitQuickAddLines("3x AA batteries")).toEqual(["3x AA batteries"]);
  });
});
