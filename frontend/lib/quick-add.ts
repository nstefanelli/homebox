/**
 * Pure parsing helpers for the quick-add row on the location page
 * (components/Entity/QuickAddRow.vue). Kept free of Vue/API imports so the
 * quantity-prefix grammar is unit-testable in isolation.
 */

export interface QuickAddLine {
  name: string;
  quantity: number;
}

/** Quantity prefix grammar: a leading "<int>x " or "<int> x ", case-insensitive. */
const QUANTITY_PREFIX = /^(\d+) ?x (.+)$/i;

export const QUICK_ADD_MIN_QUANTITY = 1;
export const QUICK_ADD_MAX_QUANTITY = 999;

/**
 * Parses one quick-add line into a name + quantity. A leading "<int>x " /
 * "<int> x " (case-insensitive) sets the quantity (clamped to 1..999) and is
 * stripped from the name; anything that doesn't match the prefix grammar --
 * "3x" with no name, "3.5x foo", "-3x foo", "x foo" -- is literally the name
 * with quantity 1.
 */
export function parseQuickAddLine(line: string): QuickAddLine {
  const trimmed = line.trim();

  const match = QUANTITY_PREFIX.exec(trimmed);
  if (match) {
    const name = match[2]!.trim();
    if (name) {
      const quantity = Math.min(QUICK_ADD_MAX_QUANTITY, Math.max(QUICK_ADD_MIN_QUANTITY, parseInt(match[1]!, 10)));
      return { name, quantity };
    }
  }

  return { name: trimmed, quantity: 1 };
}

/**
 * Splits pasted multi-line text into the lines quick add should process:
 * per-line, trimmed, empties dropped. CRLF-tolerant since clipboard content
 * routinely arrives from Windows-authored lists.
 */
export function splitQuickAddLines(text: string): string[] {
  return text
    .split(/\r?\n/)
    .map(line => line.trim())
    .filter(line => line.length > 0);
}
