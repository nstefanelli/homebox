import { describe, expect, test } from "vitest";
import { fitFontSize, linesRequired, type TextMeasurer } from "./fit";

// Flat-width measurer: every glyph is 0.6em. Deterministic and proportional
// to font size, which is all the fit algorithm assumes about real metrics.
const measure: TextMeasurer = (text, sizePx) => text.length * sizePx * 0.6;

// 5160-ish text column: ~134px next to the QR on a 2.625in label.
const AVAIL = 134;
const BASE = 12;
const FLOOR = BASE * 0.65;

describe("linesRequired", () => {
  test("packs words greedily onto lines", () => {
    // "Tote" = 4*0.6*12 = 28.8px; ~4 words + separators per 134px line.
    expect(linesRequired("Tote", AVAIL, BASE, measure)).toBe(1);
    expect(linesRequired("Tote Tote Tote Tote Tote Tote", AVAIL, BASE, measure)).toBe(2);
  });

  test("slices tokens wider than the line (break-words)", () => {
    // 60 chars, no spaces: 432px at 12px => 4 slices of 134px.
    expect(linesRequired("x".repeat(60), AVAIL, BASE, measure)).toBe(4);
  });

  test("empty text needs no lines", () => {
    expect(linesRequired("", AVAIL, BASE, measure)).toBe(0);
  });
});

describe("fitFontSize", () => {
  test("fits at base: short text keeps the base size", () => {
    expect(fitFontSize("HDX Tote", AVAIL, BASE, measure)).toBe(BASE);
  });

  test("shrinks to fit: a long name steps below base but lands above the floor", () => {
    // Four 10-char words: 72px each at 12px, so one word per 134px line (4
    // lines). Two per line needs 12.6*size <= 134 => fits from 10.5 down.
    const name = "AAAAAAAAAA BBBBBBBBBB CCCCCCCCCC DDDDDDDDDD";
    const size = fitFontSize(name, AVAIL, BASE, measure);
    expect(size).toBeLessThan(BASE);
    expect(size).toBeGreaterThan(FLOOR);
    // The returned size actually fits the 2-line budget...
    expect(linesRequired(name, AVAIL, size, measure)).toBeLessThanOrEqual(2);
    // ...and one step up did not (the largest fitting size was chosen).
    expect(linesRequired(name, AVAIL, size + 0.25, measure)).toBeGreaterThan(2);
  });

  test("floors and clamps: a pathological name stops exactly at 65% of base", () => {
    const monster = "X".repeat(220);
    expect(fitFontSize(monster, AVAIL, BASE, measure)).toBe(FLOOR);
  });

  test("never returns outside [floor, base]", () => {
    for (const text of ["a", "word ".repeat(10), "y".repeat(500)]) {
      const size = fitFontSize(text, AVAIL, BASE, measure);
      expect(size).toBeGreaterThanOrEqual(FLOOR);
      expect(size).toBeLessThanOrEqual(BASE);
    }
  });

  test("degenerate inputs return the base size unchanged", () => {
    expect(fitFontSize("", AVAIL, BASE, measure)).toBe(BASE);
    expect(fitFontSize("name", 0, BASE, measure)).toBe(BASE);
    expect(fitFontSize("name", -5, BASE, measure)).toBe(BASE);
  });

  test("honors a custom floor ratio and line budget", () => {
    const monster = "X".repeat(220);
    expect(fitFontSize(monster, AVAIL, BASE, measure, { floorRatio: 0.5 })).toBe(6);
    // A 1-line budget forces a shrink where the 2-line default fits at base.
    const twoLiner = "Tote Tote Tote Tote Tote Tote";
    expect(fitFontSize(twoLiner, AVAIL, BASE, measure)).toBe(BASE);
    expect(fitFontSize(twoLiner, AVAIL, BASE, measure, { maxLines: 1 })).toBeLessThan(BASE);
  });
});
