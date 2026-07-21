import { describe, expect, test } from "vitest";
import { fmtAssetID, itemQrPayload, rangeQrPayload } from "./qr";

const ORIGIN = "https://homebox.example.com";

describe("fmtAssetID", () => {
  test("formats numeric ids with padding and dash", () => {
    expect(fmtAssetID(1)).toBe("000-001");
    expect(fmtAssetID(42)).toBe("000-042");
    expect(fmtAssetID(123456)).toBe("123-456");
  });
});

describe("rangeQrPayload", () => {
  test("blank slots past the inventory use the range-derived asset URL", () => {
    expect(rangeQrPayload(ORIGIN, 0, null)).toBe(`${ORIGIN}/a/000-001`);
    expect(rangeQrPayload(ORIGIN, 89, null)).toBe(`${ORIGIN}/a/000-090`);
  });

  test("items with an asset id keep the historical /a/ encoding", () => {
    const item = { id: "3f6c9a0e-item-uuid", assetId: "000-042" };
    expect(rangeQrPayload(ORIGIN, 0, item)).toBe(`${ORIGIN}/a/${fmtAssetID("000-042")}`);
  });

  // Regression (2026-07-21 label-identity review): a real item whose assetId
  // is "" (auto_increment_asset_id=false) must not be routed through
  // fmtAssetID — that yields "000-000", so EVERY id-less item shared the
  // identical /a/000-000 payload, resolving to asset ID 0 instead of the
  // item. Id-less items deep-link their entity page instead.
  test("items without an asset id deep-link /item/{id}, never /a/000-000", () => {
    const first = { id: "aaaa-1111", assetId: "" };
    const second = { id: "bbbb-2222", assetId: "" };

    const firstPayload = rangeQrPayload(ORIGIN, 0, first);
    const secondPayload = rangeQrPayload(ORIGIN, 1, second);

    expect(firstPayload).toBe(`${ORIGIN}/item/aaaa-1111`);
    expect(secondPayload).toBe(`${ORIGIN}/item/bbbb-2222`);
    // The two payloads are unique per item — the old behavior collapsed both
    // onto the same URL.
    expect(firstPayload).not.toBe(secondPayload);
    expect(firstPayload).not.toContain("000-000");
    expect(secondPayload).not.toContain("000-000");
  });

  test("origin is trimmed and stripped of a trailing slash", () => {
    expect(rangeQrPayload(`  ${ORIGIN}/  `, 0, null)).toBe(`${ORIGIN}/a/000-001`);
    expect(rangeQrPayload(`${ORIGIN}/`, 0, { id: "cccc-3333", assetId: "" })).toBe(`${ORIGIN}/item/cccc-3333`);
  });
});

describe("itemQrPayload", () => {
  test("items with an asset id use the raw pre-formatted asset URL", () => {
    expect(itemQrPayload(ORIGIN, { id: "aaaa-1111", assetId: "000-042" })).toBe(`${ORIGIN}/a/000-042`);
  });

  // Regression (queue-mode variant of the /a/000-000 bug): enqueueing an item
  // whose assetId is "" built `{origin}/a/` — a dead link. Id-less items
  // deep-link their entity page instead.
  test("items without an asset id deep-link /item/{id}, never a bare /a/", () => {
    const payload = itemQrPayload(ORIGIN, { id: "bbbb-2222", assetId: "" });
    expect(payload).toBe(`${ORIGIN}/item/bbbb-2222`);
    expect(payload).not.toContain("/a/");
  });

  test("origin is trimmed and stripped of a trailing slash", () => {
    expect(itemQrPayload(`${ORIGIN}/`, { id: "cccc-3333", assetId: "" })).toBe(`${ORIGIN}/item/cccc-3333`);
    expect(itemQrPayload(` ${ORIGIN} `, { id: "dddd-4444", assetId: "000-007" })).toBe(`${ORIGIN}/a/000-007`);
  });
});
