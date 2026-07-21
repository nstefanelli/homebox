// Label-sheet PDF generation (pdf-lib). Renders the SAME state the verified
// HTML sheet renders — preset geometry via the page's `out`, pagination from
// calcPages (skip nulls included), the clamped printOffset, and fit.ts's
// fitFontSize driven by a pdf-lib measurer — as a real PDF with exact point
// geometry. WebKit prints CSS inches at ~1.0667 physical inches and ignores
// @page size, so no Safari HTML print path can be 1:1; a PDF printed at
// Actual Size is, and it also unlocks AirPrint from iPhone/iPad.
//
// Pure module by design: no Vue, no DOM (the QR fetcher is injectable), so it
// is import-testable in node for the raster-measurement harness. The Vue page
// passes a plain state object; nothing here recomputes grid/pagination logic.

import {
  PDFDocument,
  rgb,
  degrees,
  TextRenderingMode,
  pushGraphicsState,
  popGraphicsState,
  moveTo,
  lineTo,
  closePath,
  clip,
  endPath,
  setTextRenderingMode,
  setStrokingColor,
  setLineWidth,
} from "pdf-lib";
import type { PDFFont, PDFImage, PDFPage } from "pdf-lib";
import fontkit from "@pdf-lib/fontkit";
import { fitFontSize } from "./fit";
import type { TextMeasurer } from "./fit";

// ── Unit conversions — the ONE place px/measure-units become points ─────────
// The HTML sheet does its physical math in the sheet's measure (inches for
// every preset) and its text math in CSS px. PDF user space is points, origin
// at the page's BOTTOM-left. Constants: 1in = 72pt = 96 CSS px, therefore
// 1px = 72/96 = 0.75pt. Every conversion in this module funnels through
// PX_TO_PT / unitToPt so the two coordinate systems can never drift.
export const PT_PER_IN = 72;
export const CSS_PX_PER_IN = 96;
export const PX_TO_PT = PT_PER_IN / CSS_PX_PER_IN; // 0.75

/** Mirrors the page's UNITS_PER_INCH (label-generator.vue). */
const UNITS_PER_INCH: Record<string, number> = { in: 1, cm: 2.54, mm: 25.4 };

export function pxToPt(px: number): number {
  return px * PX_TO_PT;
}

/** Sheet-measure units ("in" | "cm" | "mm") → points. Unknown units read as inches, like the page. */
export function unitToPt(value: number, measure: string): number {
  return (value / (UNITS_PER_INCH[measure] ?? 1)) * PT_PER_IN;
}

// ── Style constants mirroring the HTML template (label-generator.vue) ───────
const CELL_BORDER_PX = 2; // border-2: in the border-box even when transparent
const SAFE_INSET_IN = 0.1; // LABEL_SAFE_INSET, absolute inches on purpose
const TEXT_GUTTER_PX = 8; // ml-2 between the QR and the text column
const ASSET_FONT_PX = 16; // asset row inherits the 16px root size…
const ASSET_LINE_PX = 24; // …with the body's unitless 1.5 line-height
const XS_FONT_PX = 12; // Tailwind text-xs
const XS_LINE_PX = 16; // text-xs pins line-height to 1rem — fixed, even when
//                        the name row's inline font-size is auto-fit below 12
const BOLD_SLOT_LINE_RATIO = 1.5; // bold-slot name has no size class, so the
//                                   unitless 1.5 scales with the fitted size
const MAX_CLAMP_LINES = 2; // line-clamp-2 on the asset and name rows
const ELLIPSIS = "…";
// Single bundled face: bold is approximated by stroking the fill outline
// (~3% of the size — visually close to weight 700), italic by the standard
// ~12° oblique skew. Weight 300 ("font-light") renders regular.
const BOLD_STROKE_RATIO = 0.03;
const ITALIC_SKEW_DEG = 12;
const BLACK = rgb(0, 0, 0);

// ── Input surface — plain data handed over by the label-generator page ──────

export interface LabelPdfItem {
  /** authenticated /api/v1/qrcode?data=… URL (same-origin) */
  url: string;
  /** pre-formatted asset id; "" omits the row (never 000-000) */
  assetLine: string;
  /** entity name; takes the bold slot when assetLine is "" */
  nameLine: string;
  /** location / parentPath row */
  locationLine: string;
}

export interface LabelPdfPage {
  /** calcPages cells in row-major order; null = skipped label position */
  items: Array<LabelPdfItem | null>;
}

export interface LabelPdfState {
  /** sheet measure, "in" for every preset */
  measure: string;
  cols: number;
  /** column gap between cells, measure units (out.gapX) */
  gapX: number;
  /** EFFECTIVE row gap: the page renders `currentPreset ? out.gapY : 0` */
  rowGapY: number;
  card: { width: number; height: number };
  page: { width: number; height: number; pt: number; pb: number; pl: number; pr: number };
  /** already clamped by the page's printOffset computed, measure units */
  printOffset: { x: number; y: number };
  bordered: boolean;
  printLocationRow: boolean;
  /** QR edge, measure units (page's qrSize computed) */
  qrSize: number;
  /** text column width in CSS px (page's textColumnPx computed) */
  textColumnPx: number;
  /** page's auto-fit constants: floor 0.65, bases 12px / 16px (bold slot) */
  nameFit: { floorRatio: number; baseSizePx: number; boldBaseSizePx: number };
  /** the "_______________" write-in sentinel */
  labelBlankLine: string;
  /** the page's getHomeBoxLineText: falsy result omits the row */
  homeBoxLine: (item: LabelPdfItem) => string | null;
  pages: LabelPdfPage[];
}

export type FetchImage = (url: string) => Promise<Uint8Array>;

export interface LabelPdfDeps {
  /** bundled OFL TTF bytes (assets/fonts/NotoSans-Regular-subset.ttf) */
  fontBytes: Uint8Array | ArrayBuffer;
  /** QR byte fetcher; defaults to same-origin fetch (session cookie rides along) */
  fetchImage?: FetchImage;
}

// ── Geometry ────────────────────────────────────────────────────────────────

export interface CellRectPt {
  /** PDF coords: x/y is the cell's BOTTOM-left corner, points */
  x: number;
  y: number;
  width: number;
  height: number;
}

type GeometryState = Pick<LabelPdfState, "measure" | "gapX" | "rowGapY" | "card" | "page" | "printOffset">;

/**
 * Cell border-box rectangle in PDF points. The CSS sheet positions cell
 * (row, col) top-left at (pl + offsetX + col·(cardW + gapX),
 * pt + offsetY + row·(cardH + rowGapY)) from the page's TOP-left; PDF measures
 * from the BOTTOM-left, so y = pageH − topOffset − cardH. The print offset is
 * a plain translation here — a PDF never shrink-to-fits, so the page's
 * padding-delta trick isn't needed, but the clamped values are identical.
 */
export function computeCellRect(state: GeometryState, row: number, col: number): CellRectPt {
  const m = state.measure;
  const width = unitToPt(state.card.width, m);
  const height = unitToPt(state.card.height, m);
  const x = unitToPt(state.page.pl + state.printOffset.x + col * (state.card.width + state.gapX), m);
  const topFromPageTop = unitToPt(state.page.pt + state.printOffset.y + row * (state.card.height + state.rowGapY), m);
  const y = unitToPt(state.page.height, m) - topFromPageTop - height;
  return { x, y, width, height };
}

// ── Text measurement & layout ───────────────────────────────────────────────

/**
 * fit.ts measurer backed by the embedded PDF font, so fitFontSize makes its
 * size decisions from the exact glyph widths the PDF renders with. fit.ts
 * speaks CSS px; widthOfTextAtSize speaks pt. width is linear in size, so
 * measuring at (px · PX_TO_PT) pt and dividing the result by PX_TO_PT returns
 * a width in px — the explicit round-trip keeps every px↔pt crossing on the
 * PX_TO_PT constant above.
 */
export function makePdfMeasurer(font: Pick<PDFFont, "widthOfTextAtSize">): TextMeasurer {
  return (text, fontSizePx) => font.widthOfTextAtSize(text, fontSizePx * PX_TO_PT) / PX_TO_PT;
}

/**
 * Renderer-side line breaker: same greedy model as fit.ts's linesRequired
 * (words pack left to right, an overlong token starts a fresh line and is
 * sliced at the line edge) but producing the actual line strings. The FIT
 * decision still comes solely from fitFontSize; this only materializes lines,
 * and clampLines' ellipsis backstops any hairline divergence (linesRequired
 * approximates overlong-token slices linearly, this slices per character).
 */
export function wrapLines(text: string, availWidthPx: number, fontSizePx: number, measure: TextMeasurer): string[] {
  const words = text.split(/\s+/).filter(Boolean);
  if (words.length === 0 || availWidthPx <= 0) return [];

  const fits = (s: string) => measure(s, fontSizePx) <= availWidthPx;
  const lines: string[] = [];
  let cur = "";

  for (const word of words) {
    if (!cur && fits(word)) {
      cur = word;
      continue;
    }
    if (cur && fits(`${cur} ${word}`)) {
      cur = `${cur} ${word}`;
      continue;
    }
    if (fits(word)) {
      lines.push(cur);
      cur = word;
      continue;
    }
    // Overlong token: break-words moves it to a fresh line, then slices it.
    if (cur) lines.push(cur);
    let rest = word;
    while (!fits(rest)) {
      let i = 1;
      while (i < rest.length && fits(rest.slice(0, i + 1))) i++;
      lines.push(rest.slice(0, i));
      rest = rest.slice(i);
    }
    cur = rest;
  }
  if (cur) lines.push(cur);
  return lines;
}

/** Trims `text` until `text + …` fits, then appends the ellipsis. */
function ellipsize(text: string, availWidthPx: number, fontSizePx: number, measure: TextMeasurer): string {
  let t = text.trimEnd();
  while (t.length > 0 && measure(t + ELLIPSIS, fontSizePx) > availWidthPx) {
    t = t.slice(0, -1).trimEnd();
  }
  return t + ELLIPSIS;
}

/** CSS `truncate`: single line, ellipsis on overflow. */
function truncateLine(text: string, availWidthPx: number, fontSizePx: number, measure: TextMeasurer): string {
  if (!text || measure(text, fontSizePx) <= availWidthPx) return text;
  return ellipsize(text, availWidthPx, fontSizePx, measure);
}

/** CSS `line-clamp-2`: cap at maxLines, ellipsizing the last visible line. */
function clampLines(lines: string[], availWidthPx: number, fontSizePx: number, measure: TextMeasurer): string[] {
  if (lines.length <= MAX_CLAMP_LINES) return lines;
  const kept = lines.slice(0, MAX_CLAMP_LINES);
  kept[MAX_CLAMP_LINES - 1] = ellipsize(kept[MAX_CLAMP_LINES - 1] ?? "", availWidthPx, fontSizePx, measure);
  return kept;
}

interface TextRow {
  lines: string[];
  fontPx: number;
  linePx: number;
  bold: boolean;
  italic: boolean;
}

/**
 * The text column's row stack, mirroring the template's conditionals row for
 * row: bold asset id (omitted when ""), HomeBox line (omitted when the
 * behavior computed returns falsy; italic unless it's the blank write-in
 * sentinel), auto-fit name (bold slot when no asset id), location row (when
 * enabled — an empty locationLine yields no line box, height 0, like an empty
 * div). Write-in sentinels are literal underscore runs and render as text.
 */
function layoutRows(state: LabelPdfState, item: LabelPdfItem, measure: TextMeasurer): TextRow[] {
  const avail = state.textColumnPx;
  const rows: TextRow[] = [];

  if (item.assetLine) {
    rows.push({
      lines: clampLines(wrapLines(item.assetLine, avail, ASSET_FONT_PX, measure), avail, ASSET_FONT_PX, measure),
      fontPx: ASSET_FONT_PX,
      linePx: ASSET_LINE_PX,
      bold: true,
      italic: false,
    });
  }

  const homeBoxText = state.homeBoxLine(item);
  if (homeBoxText) {
    rows.push({
      lines: [truncateLine(homeBoxText, avail, XS_FONT_PX, measure)],
      fontPx: XS_FONT_PX,
      linePx: XS_LINE_PX,
      bold: false,
      italic: homeBoxText !== state.labelBlankLine,
    });
  }

  const boldSlot = !item.assetLine;
  const base = boldSlot ? state.nameFit.boldBaseSizePx : state.nameFit.baseSizePx;
  const namePx = fitFontSize(item.nameLine, avail, base, measure, { floorRatio: state.nameFit.floorRatio });
  rows.push({
    lines: clampLines(wrapLines(item.nameLine, avail, namePx, measure), avail, namePx, measure),
    fontPx: namePx,
    linePx: boldSlot ? namePx * BOLD_SLOT_LINE_RATIO : XS_LINE_PX,
    bold: boldSlot,
    italic: false,
  });

  // Guard on non-empty like the assetLine/homeBox rows: the HTML sheet's
  // v-if row collapses to zero height for an empty interpolation, so an
  // unguarded push here would center the PDF's text stack one phantom
  // line-box higher than the verified HTML render.
  if (state.printLocationRow && item.locationLine) {
    rows.push({
      lines: [truncateLine(item.locationLine, avail, XS_FONT_PX, measure)],
      fontPx: XS_FONT_PX,
      linePx: XS_LINE_PX,
      bold: false,
      italic: false,
    });
  }

  return rows;
}

// ── QR images ───────────────────────────────────────────────────────────────

/** Magic-byte sniff — the endpoint serves JPEG today, but don't hard-code it. */
export function sniffImageType(bytes: Uint8Array): "jpg" | "png" {
  if (bytes.length >= 3 && bytes[0] === 0xff && bytes[1] === 0xd8 && bytes[2] === 0xff) return "jpg";
  if (
    bytes.length >= 8 &&
    bytes[0] === 0x89 &&
    bytes[1] === 0x50 &&
    bytes[2] === 0x4e &&
    bytes[3] === 0x47 &&
    bytes[4] === 0x0d &&
    bytes[5] === 0x0a &&
    bytes[6] === 0x1a &&
    bytes[7] === 0x0a
  )
    return "png";
  throw new Error("QR endpoint returned neither JPEG nor PNG");
}

const defaultFetchImage: FetchImage = async url => {
  // Same-origin fetch: the session cookie is sent by default, matching the
  // authenticated <img> loads the HTML sheet does.
  const res = await fetch(url);
  if (!res.ok) throw new Error(`QR fetch failed (${res.status}) for ${url}`);
  return new Uint8Array(await res.arrayBuffer());
};

/** Fetch + embed each DISTINCT QR URL once (range sheets repeat none, but queue sheets can). */
async function embedQrImages(
  doc: PDFDocument,
  pages: LabelPdfPage[],
  fetchImage: FetchImage
): Promise<Map<string, PDFImage>> {
  const urls = new Set<string>();
  for (const page of pages) {
    for (const item of page.items) {
      if (item) urls.add(item.url);
    }
  }
  const entries = await Promise.all(
    [...urls].map(async url => {
      const bytes = await fetchImage(url);
      const image = sniffImageType(bytes) === "jpg" ? await doc.embedJpg(bytes) : await doc.embedPng(bytes);
      return [url, image] as const;
    })
  );
  return new Map(entries);
}

// ── Drawing ─────────────────────────────────────────────────────────────────

function drawCell(
  page: PDFPage,
  state: LabelPdfState,
  item: LabelPdfItem,
  rect: CellRectPt,
  font: PDFFont,
  measure: TextMeasurer,
  qrImage: PDFImage
): void {
  // Bordered mode: 1pt rectangle at the true label edge (die-cut alignment).
  if (state.bordered) {
    page.drawRectangle({
      x: rect.x,
      y: rect.y,
      width: rect.width,
      height: rect.height,
      borderColor: BLACK,
      borderWidth: 1,
    });
  }

  // overflow-hidden: clip everything to the padding box (border box minus the
  // 2px border) exactly like the HTML cell.
  const borderPt = pxToPt(CELL_BORDER_PX);
  const clipX0 = rect.x + borderPt;
  const clipX1 = rect.x + rect.width - borderPt;
  const clipY0 = rect.y + borderPt;
  const clipY1 = rect.y + rect.height - borderPt;
  page.pushOperators(
    pushGraphicsState(),
    moveTo(clipX0, clipY0),
    lineTo(clipX1, clipY0),
    lineTo(clipX1, clipY1),
    lineTo(clipX0, clipY1),
    closePath(),
    clip(),
    endPath()
  );

  // QR: left edge after the border + 0.1in safe inset, vertically centered
  // (the flex row stretches, items-center centers the square image).
  const insetPt = SAFE_INSET_IN * PT_PER_IN;
  const qrPt = unitToPt(state.qrSize, state.measure);
  const qrX = rect.x + borderPt + insetPt;
  const qrY = rect.y + (rect.height - qrPt) / 2;
  page.drawImage(qrImage, { x: qrX, y: qrY, width: qrPt, height: qrPt });

  // Text column: after the QR plus the ml-2 gutter; the flex-col is
  // justify-center, so the row stack centers in the cell's content height.
  const textX = qrX + qrPt + pxToPt(TEXT_GUTTER_PX);
  const rows = layoutRows(state, item, measure);
  const stackPt = pxToPt(rows.reduce((sum, r) => sum + r.lines.length * r.linePx, 0));
  const contentHPt = rect.height - 2 * borderPt;
  let cursorTop = rect.y + rect.height - borderPt - (contentHPt - stackPt) / 2;

  for (const row of rows) {
    const sizePt = pxToPt(row.fontPx);
    const linePt = pxToPt(row.linePx);
    // Baseline: center the font's real glyph box (ascent+descent) inside the
    // CSS line box, mirroring how browsers center the em box in line-height.
    const glyphHPt = font.heightAtSize(sizePt);
    const ascentPt = font.heightAtSize(sizePt, { descender: false });
    if (row.bold) {
      page.pushOperators(
        setTextRenderingMode(TextRenderingMode.FillAndOutline),
        setStrokingColor(BLACK),
        setLineWidth(sizePt * BOLD_STROKE_RATIO)
      );
    }
    for (const line of row.lines) {
      const baselineY = cursorTop - (linePt - glyphHPt) / 2 - ascentPt;
      if (line) {
        page.drawText(line, {
          x: textX,
          y: baselineY,
          size: sizePt,
          font,
          color: BLACK,
          ...(row.italic ? { ySkew: degrees(ITALIC_SKEW_DEG) } : {}),
        });
      }
      cursorTop -= linePt;
    }
    if (row.bold) {
      page.pushOperators(setTextRenderingMode(TextRenderingMode.Fill));
    }
  }

  page.pushOperators(popGraphicsState());
}

// ── Entry point ─────────────────────────────────────────────────────────────

/**
 * Generate the label sheet as PDF bytes from the page's plain state object.
 * Throws (never falls back to a Latin-1 built-in) when the bundled font fails
 * to embed, when a QR fetch fails, or when there is nothing to render — the
 * caller surfaces the error as a toast.
 */
export async function generateLabelSheetPdf(state: LabelPdfState, deps: LabelPdfDeps): Promise<Uint8Array> {
  if (state.pages.length === 0) throw new Error("no label pages to render");

  const doc = await PDFDocument.create();
  doc.registerFontkit(fontkit);

  let font: PDFFont;
  try {
    font = await doc.embedFont(deps.fontBytes, { subset: true });
  } catch (err) {
    throw new Error(`label font failed to embed: ${err instanceof Error ? err.message : err}`, { cause: err });
  }
  const measure = makePdfMeasurer(font);

  const images = await embedQrImages(doc, state.pages, deps.fetchImage ?? defaultFetchImage);

  const pageWidthPt = unitToPt(state.page.width, state.measure);
  const pageHeightPt = unitToPt(state.page.height, state.measure);

  for (const sheet of state.pages) {
    const page = doc.addPage([pageWidthPt, pageHeightPt]);
    sheet.items.forEach((item, idx) => {
      if (!item) return; // skipped label position
      const rect = computeCellRect(state, Math.floor(idx / state.cols), idx % state.cols);
      const qrImage = images.get(item.url);
      if (!qrImage) throw new Error(`QR image missing for ${item.url}`);
      drawCell(page, state, item, rect, font, measure, qrImage);
    });
  }

  return doc.save();
}
