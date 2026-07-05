import type { BulkItemCandidate, EntityCreate } from "~~/lib/api/types/data-contracts";

export type ReviewCandidate = BulkItemCandidate & {
  key: string;
  photoIndex: number;
  checked: boolean;
  status: "pending" | "creating" | "created" | "failed";
  possibleDuplicate: boolean;
};

/**
 * Wraps a photo's candidates for review and appends them. A candidate is
 * flagged as a possible duplicate when its trimmed, case-insensitive name
 * matches any candidate from an EARLIER photo — same-photo lookalikes are
 * legitimately distinct objects and are never flagged.
 */
export function appendCandidates(
  existing: ReviewCandidate[],
  incoming: BulkItemCandidate[],
  photoIndex: number
): ReviewCandidate[] {
  const earlierNames = new Set(existing.filter(c => c.photoIndex < photoIndex).map(c => c.name.trim().toLowerCase()));
  const wrapped = incoming.map((c, i) => ({
    ...c,
    key: `p${photoIndex}-${i}`,
    photoIndex,
    checked: true,
    status: "pending" as const,
    possibleDuplicate: earlierNames.has(c.name.trim().toLowerCase()),
  }));
  return [...existing, ...wrapped];
}

export function toEntityCreate(
  c: ReviewCandidate,
  parentId: string,
  entityTypeId: string,
  tagIds: string[]
): EntityCreate {
  return {
    name: c.name,
    description: c.description,
    manufacturer: c.manufacturer,
    modelNumber: c.modelNumber,
    quantity: c.quantity,
    icon: "",
    parentId,
    entityTypeId,
    tagIds,
  };
}
