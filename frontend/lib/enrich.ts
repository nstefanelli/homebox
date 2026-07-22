import type { BarcodeProduct } from "./api/types/data-contracts";

/**
 * Pure merge-plan logic for the "Enrich from lookup" flow (item page ->
 * product lookup -> per-field merge dialog). Kept free of Vue/API imports so
 * it is unit-testable independently of the dialog that renders it.
 */

/**
 * Candidate chosen in the product lookup dialog: the product plus whether it
 * came from the AI fallback (and must therefore be badged as an AI guess).
 */
export interface ProductLookupPick {
  product: BarcodeProduct;
  aiGuess: boolean;
}

export type EnrichTextField = "name" | "manufacturer" | "modelNumber" | "description";

export const ENRICH_TEXT_FIELDS: readonly EnrichTextField[] = ["name", "manufacturer", "modelNumber", "description"];

export interface EnrichFields {
  name: string;
  manufacturer: string;
  modelNumber: string;
  description: string;
}

export interface EnrichCurrent extends EnrichFields {
  /** Whether the item already has at least one photo attachment. */
  hasPhoto: boolean;
}

export interface EnrichProposed extends EnrichFields {
  /** Whether the candidate carries a fetchable product image. */
  hasPhoto: boolean;
}

export type MergeRow =
  | {
      kind: "text";
      field: EnrichTextField;
      current: string;
      proposed: string;
      /** Default selection: pre-checked only when the current value is empty. */
      checked: boolean;
    }
  | {
      kind: "photo";
      field: "photo";
      /** Whether the item already has a photo (display only). */
      hasCurrent: boolean;
      /** Default selection: pre-checked only when the item has no photo yet. */
      checked: boolean;
      /** Attach as the primary photo — only when the item has none. */
      primary: boolean;
    };

/**
 * Maps a BarcodeProduct candidate onto the flat proposed-field shape, using
 * the same field precedence the create-modal prefill uses
 * (applyProductPrefill in components/Entity/CreateModal.vue): top-level
 * manufacturer/modelNumber win over the nested item copy, and the photo only
 * exists when imageURL is set (imageBase64 is the fetched payload for it).
 */
export function proposedFromProduct(product: BarcodeProduct): EnrichProposed {
  return {
    name: product.item.name ?? "",
    description: product.item.description ?? "",
    manufacturer: product.manufacturer || product.item.manufacturer || "",
    modelNumber: product.modelNumber || product.item.modelNumber || "",
    hasPhoto: !!product.imageURL && !!product.imageBase64,
  };
}

/**
 * Builds the per-field merge rows shown by the enrichment dialog.
 *
 * Defaults:
 * - text field with no proposed value          -> row omitted
 * - text field where proposed equals current   -> row omitted (nothing to change)
 * - empty current, proposed present            -> row present, pre-checked
 * - non-empty current, differing proposed      -> row present, unchecked (overwrite is opt-in)
 * - photo proposed, item has none              -> row present, pre-checked, attach as primary
 * - photo proposed, item already has photos    -> row present, unchecked, attach as non-primary
 * - no photo proposed                          -> photo row omitted
 *
 * Values are compared and returned trimmed; row order is name, manufacturer,
 * modelNumber, description, photo.
 */
export function computeMergePlan(current: EnrichCurrent, proposed: EnrichProposed): MergeRow[] {
  const rows: MergeRow[] = [];

  for (const field of ENRICH_TEXT_FIELDS) {
    const proposedValue = (proposed[field] ?? "").trim();
    if (!proposedValue) {
      continue;
    }

    const currentValue = (current[field] ?? "").trim();
    if (proposedValue === currentValue) {
      continue;
    }

    rows.push({
      kind: "text",
      field,
      current: currentValue,
      proposed: proposedValue,
      checked: currentValue === "",
    });
  }

  if (proposed.hasPhoto) {
    rows.push({
      kind: "photo",
      field: "photo",
      hasCurrent: current.hasPhoto,
      checked: !current.hasPhoto,
      primary: !current.hasPhoto,
    });
  }

  return rows;
}
