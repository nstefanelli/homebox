import { readFileSync } from "node:fs";
import { fileURLToPath } from "node:url";
import { describe, expect, it, vi } from "vitest";
import { PDFDocument } from "pdf-lib";
import {
  PX_TO_PT,
  pxToPt,
  unitToPt,
  computeCellRect,
  makePdfMeasurer,
  wrapLines,
  sniffImageType,
  generateLabelSheetPdf,
} from "./pdf";
import type { LabelPdfItem, LabelPdfState } from "./pdf";
import { fitFontSize } from "./fit";
import { LABEL_PRESETS } from "./presets";

// ── px→pt conversion ────────────────────────────────────────────────────────

describe("unit conversion", () => {
  it("defines 1px = 0.75pt (96 CSS px per inch, 72pt per inch)", () => {
    expect(PX_TO_PT).toBe(0.75);
    expect(pxToPt(96)).toBe(72);
    expect(pxToPt(16)).toBe(12);
    expect(pxToPt(12)).toBe(9);
  });

  it("converts sheet-measure units to points", () => {
    expect(unitToPt(1, "in")).toBe(72);
    expect(unitToPt(8.5, "in")).toBe(612);
    expect(unitToPt(11, "in")).toBe(792);
    expect(unitToPt(2.54, "cm")).toBeCloseTo(72, 10);
    expect(unitToPt(25.4, "mm")).toBeCloseTo(72, 10);
    // Unknown measure reads as inches, like the page's UNITS_PER_INCH ?? 1.
    expect(unitToPt(1, "bogus")).toBe(72);
  });
});

// ── measurer adapter ────────────────────────────────────────────────────────

describe("makePdfMeasurer", () => {
  // Deterministic fake PDFFont: width = chars × size × 0.5 (in pt).
  const fakeFont = { widthOfTextAtSize: (text: string, size: number) => text.length * size * 0.5 };

  it("returns CSS-px widths from a pt-based font", () => {
    const measure = makePdfMeasurer(fakeFont);
    // 4 chars at 12px → font is asked at 9pt: 4 × 9 × 0.5 = 18pt → 24px.
    expect(measure("abcd", 12)).toBeCloseTo(24, 10);
    // Linearity: the px→pt→px round trip is exact for any size.
    expect(measure("abcd", 16)).toBeCloseTo(fakeFont.widthOfTextAtSize("abcd", 16), 10);
  });

  it("drives fitFontSize identically to a native px measurer", () => {
    const measure = makePdfMeasurer(fakeFont);
    const nativePx = (text: string, size: number) => text.length * size * 0.5;
    for (const text of ["short", "a somewhat longer label name", "x".repeat(80)]) {
      for (const base of [12, 16]) {
        expect(fitFontSize(text, 134.4, base, measure, { floorRatio: 0.65 })).toBe(
          fitFontSize(text, 134.4, base, nativePx, { floorRatio: 0.65 })
        );
      }
    }
  });
});

// ── coordinate math (top-left CSS → bottom-left PDF) ────────────────────────

const preset5160 = LABEL_PRESETS.find(p => p.id === "avery-5160")!;
const preset22806 = LABEL_PRESETS.find(p => p.id === "avery-22806")!;
const preset5163 = LABEL_PRESETS.find(p => p.id === "avery-5163")!;

function geometryFor(preset: typeof preset5160, offset = { x: 0, y: 0 }) {
  return {
    measure: preset.measure,
    gapX: preset.gutterX,
    rowGapY: preset.gutterY,
    card: { width: preset.labelWidth, height: preset.labelHeight },
    page: {
      width: preset.pageWidth,
      height: preset.pageHeight,
      pt: preset.pagePaddingTop,
      pb: preset.pagePaddingBottom,
      pl: preset.pagePaddingLeft,
      pr: preset.pagePaddingRight,
    },
    printOffset: offset,
  };
}

describe("computeCellRect", () => {
  it("places 5160 cell (0,0) from the preset math", () => {
    // x = 0.1875in × 72 = 13.5pt; CSS top = 0.5in = 36pt from the page top,
    // so PDF y = 792 − 36 − 72 = 684pt; cell 2.625in × 1in = 189 × 72pt.
    const rect = computeCellRect(geometryFor(preset5160), 0, 0);
    expect(rect.x).toBeCloseTo(13.5, 6);
    expect(rect.y).toBeCloseTo(684, 6);
    expect(rect.width).toBeCloseTo(189, 6);
    expect(rect.height).toBeCloseTo(72, 6);
  });

  it("advances 5160 columns by card + gutter", () => {
    // pitch = (2.625 + 0.125)in = 198pt.
    const g = geometryFor(preset5160);
    expect(computeCellRect(g, 0, 1).x).toBeCloseTo(13.5 + 198, 6);
    expect(computeCellRect(g, 0, 2).x).toBeCloseTo(13.5 + 396, 6);
  });

  it("places the last 5160 cell (row 9, col 2) at the bottom margin", () => {
    // CSS top = 0.5 + 9×1 = 9.5in = 684pt → PDF y = 792 − 684 − 72 = 36pt,
    // which equals the 0.5in bottom margin: 36pt. Margins close the page.
    const rect = computeCellRect(geometryFor(preset5160), 9, 2);
    expect(rect.x).toBeCloseTo(409.5, 6);
    expect(rect.y).toBeCloseTo(36, 6);
  });

  it("applies the print offset as a translation (+y moves content DOWN the page)", () => {
    const rect = computeCellRect(geometryFor(preset5160, { x: 0.05, y: -0.03 }), 0, 0);
    expect(rect.x).toBeCloseTo(13.5 + 0.05 * 72, 6); // 17.1
    // CSS top = (0.5 − 0.03)in → PDF y = 792 − 33.84 − 72 = 686.16.
    expect(rect.y).toBeCloseTo(686.16, 6);
  });

  it("includes the 22806 row gutter (7/12 in) in the row pitch", () => {
    // Row 1 CSS top = 0.625 + 2 + 7/12 in = 231pt → y = 792 − 231 − 144 = 417.
    const g = geometryFor(preset22806);
    const rect = computeCellRect(g, 1, 0);
    expect(rect.x).toBeCloseTo(45, 6); // 0.625in left margin
    expect(rect.y).toBeCloseTo(417, 6);
    expect(rect.width).toBeCloseTo(144, 6);
    expect(rect.height).toBeCloseTo(144, 6);
  });

  it("matches 5163 interior cell (1,1)", () => {
    // x = (0.15625 + 4 + 0.1875)in = 312.75pt;
    // CSS top = (0.5 + 2)in = 180pt → y = 792 − 180 − 144 = 468pt.
    const rect = computeCellRect(geometryFor(preset5163), 1, 1);
    expect(rect.x).toBeCloseTo(312.75, 6);
    expect(rect.y).toBeCloseTo(468, 6);
  });
});

// ── line wrapping ───────────────────────────────────────────────────────────

describe("wrapLines", () => {
  // 10px-per-char fake measurer at size 10 (width = chars × size).
  const measure = (text: string, size: number) => text.length * size;

  it("packs words greedily", () => {
    // "aa bb cc" is 80px wide; at 70px the third word starts line 2.
    expect(wrapLines("aa bb cc", 70, 10, measure)).toEqual(["aa bb", "cc"]);
  });

  it("slices an overlong token at the line edge", () => {
    expect(wrapLines("abcdefghij", 50, 10, measure)).toEqual(["abcde", "fghij"]);
  });

  it("returns no lines for empty text (empty div has no line box)", () => {
    expect(wrapLines("", 100, 10, measure)).toEqual([]);
    expect(wrapLines("   ", 100, 10, measure)).toEqual([]);
  });
});

// ── image sniffing ──────────────────────────────────────────────────────────

const PNG_1PX = Uint8Array.from(
  atob("iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVR42mP8z8BQDwAEhQGAhKmMIQAAAABJRU5ErkJggg=="),
  c => c.charCodeAt(0)
);

describe("sniffImageType", () => {
  it("detects JPEG magic bytes", () => {
    expect(sniffImageType(Uint8Array.from([0xff, 0xd8, 0xff, 0xe0, 0x00]))).toBe("jpg");
  });

  it("detects the PNG signature", () => {
    expect(sniffImageType(PNG_1PX)).toBe("png");
  });

  it("throws on anything else", () => {
    expect(() => sniffImageType(Uint8Array.from([0x47, 0x49, 0x46, 0x38]))).toThrow();
  });
});

// ── end-to-end: real font, fake QR bytes, pdf-lib re-parse ──────────────────

const fontBytes = readFileSync(
  fileURLToPath(new URL("../../assets/fonts/NotoSans-Regular-subset.ttf", import.meta.url))
);

function makeItem(n: number, name: string): LabelPdfItem {
  return {
    url: `https://box.example.com/api/v1/qrcode?data=${n}`,
    assetLine: `000-00${n % 10}`,
    nameLine: name,
    locationLine: "Garage / Shelf B",
  };
}

function makeState(pages: LabelPdfState["pages"]): LabelPdfState {
  const p = preset5160;
  return {
    ...geometryFor(p),
    cols: 3,
    bordered: true,
    printLocationRow: true,
    qrSize: 0.9,
    textColumnPx: 134.4,
    nameFit: { floorRatio: 0.65, baseSizePx: 12, boldBaseSizePx: 16 },
    labelBlankLine: "_______________",
    homeBoxLine: () => "HomeBox",
    pages,
  };
}

describe("generateLabelSheetPdf", () => {
  it("emits pages at the preset's exact point size and dedupes QR fetches", async () => {
    const items = Array.from({ length: 31 }, (_, i) => makeItem(i, `Item ${i}`));
    // Duplicate URLs on purpose: dedupe must fetch each distinct URL once.
    items[30] = { ...makeItem(30, "Item 30"), url: items[0]!.url };
    const pages = [{ items: [null, ...items.slice(0, 29)] }, { items: items.slice(29) }];
    const fetchImage = vi.fn(async () => PNG_1PX);

    const bytes = await generateLabelSheetPdf(makeState(pages), { fontBytes, fetchImage });
    expect(fetchImage).toHaveBeenCalledTimes(30); // 31 cells, 30 distinct URLs

    const doc = await PDFDocument.load(bytes);
    expect(doc.getPageCount()).toBe(2);
    for (const page of doc.getPages()) {
      expect(page.getWidth()).toBeCloseTo(612, 6);
      expect(page.getHeight()).toBeCloseTo(792, 6);
    }
  });

  it("renders non-Latin and accented names without throwing", async () => {
    const pages = [
      {
        items: [
          { ...makeItem(1, "Café Décor Überprüfung"), assetLine: "" }, // bold-slot name
          makeItem(2, "工具箱 ボックス"), // outside the subset → .notdef, must not throw
        ],
      },
    ];
    const bytes = await generateLabelSheetPdf(makeState(pages), { fontBytes, fetchImage: async () => PNG_1PX });
    const doc = await PDFDocument.load(bytes);
    expect(doc.getPageCount()).toBe(1);
  });

  it("throws when the font cannot be embedded (no silent Latin-1 fallback)", async () => {
    await expect(
      generateLabelSheetPdf(makeState([{ items: [makeItem(1, "x")] }]), {
        fontBytes: Uint8Array.from([1, 2, 3]),
        fetchImage: async () => PNG_1PX,
      })
    ).rejects.toThrow(/font/i);
  });

  it("throws when there are no pages", async () => {
    await expect(
      generateLabelSheetPdf(makeState([]), { fontBytes, fetchImage: async () => PNG_1PX })
    ).rejects.toThrow();
  });
});
