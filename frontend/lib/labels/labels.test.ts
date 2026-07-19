import { describe, expect, test } from "vitest";
import { LABEL_PRESETS, CUSTOM_PRESET_ID, type LabelPreset } from "./presets";
import { calculateGrid } from "./grid";

describe("label presets", () => {
  test("contains the four Avery presets plus custom marker", () => {
    const ids = LABEL_PRESETS.map(p => p.id);
    expect(ids).toContain("avery-5160");
    expect(ids).toContain("avery-8160");
    expect(ids).toContain("avery-5163");
    expect(ids).toContain("avery-22806");
    expect(ids).not.toContain(CUSTOM_PRESET_ID); // custom is a sentinel, not a preset entry
  });

  test("8160 shares 5160 geometry (inkjet twin)", () => {
    const p5160 = LABEL_PRESETS.find(p => p.id === "avery-5160");
    const p8160 = LABEL_PRESETS.find(p => p.id === "avery-8160");
    expect(p8160).toBeDefined();
    const { id: _a, nameKey: _b, ...geom5160 } = p5160 as LabelPreset;
    const { id: _c, nameKey: _d, ...geom8160 } = p8160 as LabelPreset;
    expect(geom8160).toEqual(geom5160);
  });

  test("every preset fits its page", () => {
    for (const p of LABEL_PRESETS) {
      expect(p.labelWidth).toBeLessThanOrEqual(p.pageWidth);
      expect(p.labelHeight).toBeLessThanOrEqual(p.pageHeight);
    }
  });
});

describe("calculateGrid", () => {
  const gridFromPreset = (id: string) => {
    const p = LABEL_PRESETS.find(x => x.id === id) as LabelPreset;
    return calculateGrid({
      pageWidth: p.pageWidth,
      pageHeight: p.pageHeight,
      cardWidth: p.labelWidth,
      cardHeight: p.labelHeight,
      pagePaddingTop: p.pagePaddingTop,
      pagePaddingBottom: p.pagePaddingBottom,
      pagePaddingLeft: p.pagePaddingLeft,
      pagePaddingRight: p.pagePaddingRight,
      gutterX: p.gutterX,
      gutterY: p.gutterY,
    });
  };

  test("Avery 5160 yields 3 columns x 10 rows = 30 per page", () => {
    const grid = gridFromPreset("avery-5160");
    expect(grid.cols).toBe(3);
    expect(grid.rows).toBe(10);
    expect(grid.perPage).toBe(30);
  });

  test("Avery 5163 yields 2 columns x 5 rows = 10 per page", () => {
    const grid = gridFromPreset("avery-5163");
    expect(grid.cols).toBe(2);
    expect(grid.rows).toBe(5);
    expect(grid.perPage).toBe(10);
  });

  test("Avery 22806 yields 3 columns x 4 rows = 12 per page (gutters on both axes)", () => {
    const grid = gridFromPreset("avery-22806");
    expect(grid.cols).toBe(3);
    expect(grid.rows).toBe(4);
    expect(grid.perPage).toBe(12);
  });

  test("custom one-label grids use finite zero gaps", () => {
    const grid = calculateGrid({
      pageWidth: 4,
      pageHeight: 4,
      cardWidth: 3,
      cardHeight: 3,
      pagePaddingTop: 0,
      pagePaddingBottom: 0,
      pagePaddingLeft: 0,
      pagePaddingRight: 0,
    });

    expect(grid).toMatchObject({ cols: 1, rows: 1, perPage: 1, gapX: 0, gapY: 0 });
    expect(Number.isFinite(grid.gapX)).toBe(true);
    expect(Number.isFinite(grid.gapY)).toBe(true);
  });
});
