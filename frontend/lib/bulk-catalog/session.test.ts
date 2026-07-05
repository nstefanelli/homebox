import { describe, expect, test } from "vitest";
import { appendCandidates, toEntityCreate, type ReviewCandidate } from "./session";
import type { BulkItemCandidate } from "~~/lib/api/types/data-contracts";

function cand(name: string, extra: Partial<BulkItemCandidate> = {}): BulkItemCandidate {
  return {
    name,
    description: "",
    manufacturer: "",
    modelNumber: "",
    quantity: 1,
    categoryHints: [],
    confidence: 0.8,
    ...extra,
  };
}

describe("appendCandidates", () => {
  test("wraps incoming with defaults", () => {
    const out = appendCandidates([], [cand("Stove")], 0);
    expect(out).toHaveLength(1);
    expect(out[0]).toMatchObject({
      checked: true,
      status: "pending",
      possibleDuplicate: false,
      photoIndex: 0,
      key: "p0-0",
    });
  });

  test("flags cross-photo duplicates only", () => {
    const first = appendCandidates([], [cand("Rope"), cand("rope")], 0);
    // same-photo lookalikes are NOT flagged
    expect(first[1]!.possibleDuplicate).toBe(false);

    const second = appendCandidates(first, [cand(" ROPE ")], 1);
    expect(second[2]!.possibleDuplicate).toBe(true);
  });

  test("appends without mutating earlier entries", () => {
    const first = appendCandidates([], [cand("Tarp")], 0);
    const second = appendCandidates(first, [cand("Stakes")], 1);
    expect(second).toHaveLength(2);
    expect(second[0]!.key).toBe("p0-0");
    expect(second[1]!.key).toBe("p1-0");
  });
});

describe("toEntityCreate", () => {
  test("maps all fields", () => {
    const rc = appendCandidates([], [cand("Stove", { manufacturer: "Coleman", quantity: 2 })], 0)[0] as ReviewCandidate;
    const out = toEntityCreate(rc, "parent-1", "type-1", ["tag-1"]);
    expect(out).toMatchObject({
      name: "Stove",
      manufacturer: "Coleman",
      quantity: 2,
      parentId: "parent-1",
      entityTypeId: "type-1",
      tagIds: ["tag-1"],
      modelNumber: "",
      description: "",
      icon: "",
    });
  });
});
