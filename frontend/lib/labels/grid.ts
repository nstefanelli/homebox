export interface GridInput {
  pageWidth: number;
  pageHeight: number;
  cardWidth: number;
  cardHeight: number;
  pagePaddingTop: number;
  pagePaddingBottom: number;
  pagePaddingLeft: number;
  pagePaddingRight: number;
  /** physical gap between columns/rows (Avery gutters); when set, gaps are FIXED, not derived */
  gutterX?: number;
  gutterY?: number;
}

export interface GridData {
  cols: number;
  rows: number;
  perPage: number;
  gapX: number;
  gapY: number;
}

export function calculateGrid(input: GridInput): GridData {
  const usable = {
    w: input.pageWidth - input.pagePaddingLeft - input.pagePaddingRight,
    h: input.pageHeight - input.pagePaddingTop - input.pagePaddingBottom,
  };
  if (input.gutterX !== undefined || input.gutterY !== undefined) {
    // Explicit-gutter mode (presets): fixed gaps, count = how many pitches fit.
    // Add one gutter to the usable span because N labels need only N-1 gutters.
    const gX = input.gutterX ?? 0;
    const gY = input.gutterY ?? 0;
    const EPS = 1e-6; // float-noise guard: real sheets sum exactly to the page
    const cols = Math.max(1, Math.floor((usable.w + gX + EPS) / (input.cardWidth + gX)));
    const rows = Math.max(1, Math.floor((usable.h + gY + EPS) / (input.cardHeight + gY)));
    return { cols, rows, perPage: cols * rows, gapX: gX, gapY: gY };
  }
  // Derived mode (Custom dimensions): preserve the EXISTING upstream behavior —
  // moved verbatim from label-generator.vue calculateGridData() (lines 105-108),
  // with displayProperties.*/page.* references replaced by input.* fields.
  // Note the asymmetry is intentional/preserved: gapX derives from the *usable*
  // width, but gapY derives from the *full* page height (matches prior behavior).
  const cols = Math.max(1, Math.floor(usable.w / input.cardWidth));
  const rows = Math.max(1, Math.floor(usable.h / input.cardHeight));
  const gapX = cols > 1 ? (usable.w - cols * input.cardWidth) / (cols - 1) : 0;
  const gapY = rows > 1 ? (input.pageHeight - rows * input.cardHeight) / (rows - 1) : 0;
  return { cols, rows, perPage: cols * rows, gapX, gapY };
}
