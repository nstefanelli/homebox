/**
 * QR payload composition for the asset-range label generator (pure — the page
 * wraps the returned payload in the /qrcode endpoint URL).
 */

/** "42" / 42 -> "000-042"; mirrors the backend AssetID.String() format. */
export function fmtAssetID(aid: number | string) {
  aid = aid.toString();

  let aidStr = aid.toString().padStart(6, "0");
  aidStr = aidStr.slice(0, 3) + "-" + aidStr.slice(3);
  return aidStr;
}

export interface RangeLabelItem {
  id: string;
  /** pre-formatted by the API ("000-042"); "" when the entity has none */
  assetId: string;
}

function normalizeOrigin(origin: string): string {
  origin = origin.trim();
  return origin.endsWith("/") ? origin.slice(0, -1) : origin;
}

/**
 * QR payload for an item queued from a list/table (print-queue entries carry
 * this as their absolute deep-link URL). Items with an asset ID keep the
 * asset URL built from the API's pre-formatted assetId; items without one
 * deep-link their entity page — `/a/` with an empty id is a dead link.
 */
export function itemQrPayload(origin: string, item: RangeLabelItem): string {
  origin = normalizeOrigin(origin);
  return item.assetId ? `${origin}/a/${item.assetId}` : `${origin}/item/${item.id}`;
}

/**
 * QR payload for one range-mode label slot.
 *
 * A real item whose assetId is "" (auto-increment off / never assigned) must
 * NOT go through the asset-URL form: fmtAssetID("") is "000-000", so every
 * such item would share the identical /a/000-000 payload — resolving to asset
 * ID 0, not the item. Those items deep-link their entity page instead, the
 * same URL form the print queue uses for containers/locations.
 *
 * Slots past the inventory (item == null) are the blank, pre-printable labels
 * and keep the range-derived /a/ URL. Items WITH an asset ID keep the exact
 * historical encoding, formatter quirks included.
 */
export function rangeQrPayload(origin: string, n: number, item: RangeLabelItem | null): string {
  origin = normalizeOrigin(origin);

  if (item && !item.assetId) {
    return `${origin}/item/${item.id}`;
  }

  return `${origin}/a/${fmtAssetID(item?.assetId ?? n + 1)}`;
}
