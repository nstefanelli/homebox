# Label Identity & Title Auto-Fit — Design

**Date:** 2026-07-21
**Status:** Awaiting approval
**Driver:** Real-world 8160 print test (2026-07-21): a batch of 10 identical totes printed 10 visually identical labels. The distinguishing batch number lives at the end of the name, which the 2-line clamp truncates; containers never print their asset ID at all (queue mode shows asset ID only for `kind === "item"`).

## Goal

One number identifies a physical thing everywhere — on the label, in the UI list, and inside the QR code — and the label always shows it. The full name stays readable by shrinking to fit instead of truncating.

## Decisions (made with Nick, 2026-07-21)

1. **Asset ID is the single identifier.** No parallel batch/tote numbering: two unique numbering systems for the same object is confusion by design.
2. **Title shrinks to fit.** Font steps down to a floor before any ellipsis.
3. **Out of scope:** Safari print-scale calibration (deferred — print from Chrome for now; see homelab docs/homebox.md gotcha), preset geometry changes (presets match Avery's templates exactly and must not be touched).

## Changes

### 1. Label layout: asset ID on every label

`frontend/pages/reports/label-generator.vue` (queue mode `getEntityItem` path and range mode `getItem` path):

- The **formatted asset ID** (existing `fmtAssetID` format, e.g. `000-042`) renders as the bold top line on **every** label — items, containers, locations — replacing the current split (items: asset ID; containers/locations: name).
- The name moves to the auto-fit title row (change 2) on all labels.
- If an entity's `assetId` is 0/absent (legacy rows, `auto_increment_asset_id=false`), the ID row is **omitted** — never print a meaningless `000-000`. The name then takes the bold slot.
- QR content is unchanged (it already encodes the asset-ID URL in range mode and the entity URL in queue mode — verify, don't alter).
- Prerequisite: the print-queue store (`stores/labelSelection.ts`) and every enqueue site (locations tree select mode, location page "Print container labels", items table bulk print, batch-create success toast) must carry `assetId` for non-item kinds. The entities API already returns `assetId`; thread it through.

### 2. Title auto-fit

- The name row fits its full text by stepping the font size down from the current size to a floor of **65% of base** (≈8px at the default label text size), then and only then ellipsizing (existing `line-clamp-2` as backstop).
- Implementation constraint: must be computed in the DOM the print pipeline sees (measurement-based sizing is fine — same rendered DOM prints), must be deterministic in Chromium, and must **never change the label cell's outline geometry** (the cell is a fixed border-box; only the text inside scales).
- Applies to the name row on all labels; the asset-ID row does not shrink (it is the at-a-glance identifier).

### 3. Batch create: names carry the asset ID

Backend batch create (`POST /v1/templates/{id}/batch-create`):

- Each created entity is named `<base name> #<formatted asset id>` using the asset ID assigned to that entity in the same operation — not a per-batch sequence counter.
- The numbering-continues-across-batches counter is no longer used for names. (The asset-ID auto-increment continues to work exactly as today; only the name suffix source changes.)
- Existing entities are untouched — no migration, no rename. (Optional follow-up, separate from this change: one-time rename of the 10 existing HDX totes to their asset IDs.)
- UPC-scan container creation with a count field uses the same naming rule.

## Acceptance criteria

1. Queue-mode labels for an item, a container, and a location each show the formatted asset ID bold when `assetId > 0`; the row is absent when `assetId == 0`.
2. A batch-create of N totes yields names suffixed with each tote's own formatted asset ID; no name contains a bare sequence number.
3. A ~60-character name renders fully (no ellipsis) on a 5160 label via font shrink; a pathological name (200+ chars) still clamps at the floor size without escaping the cell.
4. All existing label geometry tests stay green; the render harness confirms cell outlines are byte-identical to v0.26.2-e2e.2 (content-only change).
5. Backend suite green; new naming behavior covered by tests through the real batch-create path.

## Risks / notes

- Auto-fit needs a measurement pass (no pure-CSS solution); keep it O(labels) and computed before print, not during.
- Asset-ID prominence on a 1in-tall 5160 label is a layout squeeze: ID bold row + auto-fit name + optional location/blank line. The ID row wins prominence by design.
- Container/location asset IDs exist (seeder assigns 1..8; `CreateContainer` assigns sequentially) but some user-created locations may be 0 — the omit rule covers them.
