import { describe, expect, test } from "vitest";
import { computeMergePlan, proposedFromProduct, type EnrichCurrent, type EnrichProposed } from "./enrich";
import type { BarcodeProduct } from "./api/types/data-contracts";

function current(overrides: Partial<EnrichCurrent> = {}): EnrichCurrent {
  return {
    name: "",
    manufacturer: "",
    modelNumber: "",
    description: "",
    hasPhoto: false,
    ...overrides,
  };
}

function proposed(overrides: Partial<EnrichProposed> = {}): EnrichProposed {
  return {
    name: "Sony WH-1000XM5",
    manufacturer: "Sony",
    modelNumber: "WH-1000XM5",
    description: "Wireless noise-cancelling headphones",
    hasPhoto: true,
    ...overrides,
  };
}

describe("computeMergePlan", () => {
  test("empty current: all text rows present and pre-checked", () => {
    const rows = computeMergePlan(current(), proposed({ hasPhoto: false }));

    expect(rows.map(r => r.field)).toEqual(["name", "manufacturer", "modelNumber", "description"]);
    expect(rows.every(r => r.checked)).toBe(true);
  });

  test("filled current with differing proposals: rows present but unchecked", () => {
    const rows = computeMergePlan(
      current({
        name: "Headphones",
        manufacturer: "Somy",
        modelNumber: "WH-1000",
        description: "old description",
      }),
      proposed({ hasPhoto: false })
    );

    expect(rows.map(r => r.field)).toEqual(["name", "manufacturer", "modelNumber", "description"]);
    expect(rows.every(r => !r.checked)).toBe(true);
  });

  test("partial current: empty fields checked, filled fields unchecked", () => {
    const rows = computeMergePlan(
      current({ name: "Headphones", description: "already described" }),
      proposed({ hasPhoto: false })
    );

    const byField = Object.fromEntries(rows.map(r => [r.field, r]));
    expect(byField.name!.checked).toBe(false);
    expect(byField.manufacturer!.checked).toBe(true);
    expect(byField.modelNumber!.checked).toBe(true);
    expect(byField.description!.checked).toBe(false);
  });

  test("fields with no proposed value are omitted", () => {
    const rows = computeMergePlan(
      current({ name: "Headphones" }),
      proposed({ manufacturer: "", modelNumber: "   ", hasPhoto: false })
    );

    expect(rows.map(r => r.field)).toEqual(["name", "description"]);
  });

  test("fields where proposed equals current (after trim) are omitted", () => {
    const rows = computeMergePlan(
      current({ name: "Sony WH-1000XM5 ", manufacturer: "Sony" }),
      proposed({ hasPhoto: false })
    );

    expect(rows.map(r => r.field)).toEqual(["modelNumber", "description"]);
  });

  test("row values are trimmed", () => {
    const rows = computeMergePlan(
      current({ manufacturer: "  Somy  " }),
      proposed({ name: "", manufacturer: "  Sony  ", modelNumber: "", description: "", hasPhoto: false })
    );

    expect(rows).toEqual([{ kind: "text", field: "manufacturer", current: "Somy", proposed: "Sony", checked: false }]);
  });

  test("no rows at all when nothing is proposed", () => {
    const rows = computeMergePlan(
      current(),
      proposed({ name: "", manufacturer: "", modelNumber: "", description: "", hasPhoto: false })
    );

    expect(rows).toEqual([]);
  });

  test("photo row: pre-checked and primary when the item has no photo", () => {
    const rows = computeMergePlan(current(), proposed());
    const photo = rows.find(r => r.kind === "photo");

    expect(photo).toEqual({ kind: "photo", field: "photo", hasCurrent: false, checked: true, primary: true });
  });

  test("photo row: unchecked and non-primary when the item already has photos", () => {
    const rows = computeMergePlan(current({ hasPhoto: true }), proposed());
    const photo = rows.find(r => r.kind === "photo");

    expect(photo).toEqual({ kind: "photo", field: "photo", hasCurrent: true, checked: false, primary: false });
  });

  test("photo row omitted when the candidate has no image", () => {
    const rows = computeMergePlan(current(), proposed({ hasPhoto: false }));

    expect(rows.some(r => r.kind === "photo")).toBe(false);
  });

  test("photo row is last, after the text rows", () => {
    const rows = computeMergePlan(current(), proposed());

    expect(rows.at(-1)?.field).toBe("photo");
  });
});

describe("proposedFromProduct", () => {
  function product(overrides: Partial<BarcodeProduct> = {}): BarcodeProduct {
    return {
      barcode: "",
      imageBase64: "",
      imageURL: "",
      manufacturer: "",
      modelNumber: "",
      notes: "",
      search_engine_name: "upcitemdb.com",
      item: {
        name: "Sony WH-1000XM5",
        description: "Wireless headphones",
        manufacturer: "Sony (nested)",
        modelNumber: "WH-nested",
        quantity: 1,
        entityTypeId: "",
        icon: "",
        parentId: null,
        tagIds: [],
      },
      ...overrides,
    } as BarcodeProduct;
  }

  test("maps name/description from the nested item", () => {
    const p = proposedFromProduct(product());
    expect(p.name).toBe("Sony WH-1000XM5");
    expect(p.description).toBe("Wireless headphones");
  });

  test("top-level manufacturer/modelNumber win over the nested item copy", () => {
    const p = proposedFromProduct(product({ manufacturer: "Sony", modelNumber: "WH-1000XM5" }));
    expect(p.manufacturer).toBe("Sony");
    expect(p.modelNumber).toBe("WH-1000XM5");
  });

  test("falls back to the nested item manufacturer/modelNumber", () => {
    const p = proposedFromProduct(product());
    expect(p.manufacturer).toBe("Sony (nested)");
    expect(p.modelNumber).toBe("WH-nested");
  });

  test("hasPhoto requires both imageURL and imageBase64 (same gate as the barcode prefill)", () => {
    expect(proposedFromProduct(product()).hasPhoto).toBe(false);
    expect(proposedFromProduct(product({ imageURL: "https://x/img.jpg" })).hasPhoto).toBe(false);
    expect(
      proposedFromProduct(product({ imageURL: "https://x/img.jpg", imageBase64: "data:image/jpeg;base64,AAAA" }))
        .hasPhoto
    ).toBe(true);
  });
});
