// Title auto-fit for label cells: step the font size down from the row's base
// until the text fits its line budget, stopping at a floor ratio of the base
// (the CSS line-clamp stays on as the ellipsis backstop below the floor).
// Pure and measurement-injected so it is unit-testable without a DOM; the
// component supplies a canvas measurer (createCanvasMeasurer) whose font
// string matches the rendered row, which makes the result deterministic in
// Chromium — canvas and DOM layout share the same shaper and font faces, and
// the previewed DOM is the DOM the print pipeline rasterizes.

/** Returns the rendered width in CSS px of `text` at `fontSizePx`. */
export type TextMeasurer = (text: string, fontSizePx: number) => number;

export interface FitOptions {
  /** Lowest allowed size as a fraction of the base size. */
  floorRatio?: number;
  /** Wrap budget — mirrors the row's line-clamp. */
  maxLines?: number;
  /** Size decrement per attempt, px. */
  stepPx?: number;
}

const DEFAULT_FLOOR_RATIO = 0.65;
const DEFAULT_MAX_LINES = 2;
const DEFAULT_STEP_PX = 0.25;

/**
 * Greedy simulation of the browser's line breaker: words pack left to right,
 * a word that no longer fits starts a fresh line, and a word wider than the
 * whole line (the row has break-words) is sliced at the line edge. Greedy
 * matches Chromium's normal-text breaking; the slice count for overlong
 * tokens is a linear approximation — close enough for a fit decision, and
 * the line-clamp backstop covers any residual error.
 */
export function linesRequired(text: string, availWidthPx: number, fontSizePx: number, measure: TextMeasurer): number {
  const words = text.split(/\s+/).filter(Boolean);
  if (words.length === 0) return 0;
  if (availWidthPx <= 0) return Infinity;

  const spaceWidth = measure(" ", fontSizePx);
  let lines = 1;
  let used = 0;

  for (const word of words) {
    const wordWidth = measure(word, fontSizePx);

    if (used === 0 && wordWidth <= availWidthPx) {
      used = wordWidth;
      continue;
    }
    if (used > 0 && used + spaceWidth + wordWidth <= availWidthPx) {
      used += spaceWidth + wordWidth;
      continue;
    }
    if (wordWidth <= availWidthPx) {
      lines += 1;
      used = wordWidth;
      continue;
    }
    // Overlong token: break-word moves it to a fresh line first, then slices.
    if (used > 0) lines += 1;
    const slices = Math.ceil(wordWidth / availWidthPx);
    lines += slices - 1;
    used = wordWidth - (slices - 1) * availWidthPx;
  }

  return lines;
}

/**
 * Largest font size in [base * floorRatio, base] at which `text` fits
 * `maxLines` lines of `availWidthPx`, stepping down by `stepPx`; returns the
 * floor when nothing fits. Empty text and degenerate widths return the base —
 * shrinking cannot help there, and the base keeps the row's look unchanged.
 */
export function fitFontSize(
  text: string,
  availWidthPx: number,
  baseSizePx: number,
  measure: TextMeasurer,
  opts: FitOptions = {}
): number {
  const floorRatio = opts.floorRatio ?? DEFAULT_FLOOR_RATIO;
  const maxLines = opts.maxLines ?? DEFAULT_MAX_LINES;
  const stepPx = opts.stepPx ?? DEFAULT_STEP_PX;
  const floor = baseSizePx * floorRatio;

  if (!text || availWidthPx <= 0 || baseSizePx <= 0) return baseSizePx;

  for (let size = baseSizePx; size > floor; size -= stepPx) {
    if (linesRequired(text, availWidthPx, size, measure) <= maxLines) return size;
  }
  return floor;
}

/**
 * Canvas-backed measurer bound to one font weight/family; the caller passes
 * the family the label text actually renders with so measurement and layout
 * agree. Falls back to a flat 0.6em-per-glyph estimate when no 2D context
 * exists (non-DOM test runs) — never used in the browser.
 */
export function createCanvasMeasurer(fontWeight: number | string, fontFamily: string): TextMeasurer {
  const ctx = typeof document !== "undefined" ? document.createElement("canvas").getContext("2d") : null;
  if (!ctx) {
    return (text, fontSizePx) => text.length * fontSizePx * 0.6;
  }
  return (text, fontSizePx) => {
    ctx.font = `${fontWeight} ${fontSizePx}px ${fontFamily}`;
    return ctx.measureText(text).width;
  };
}
