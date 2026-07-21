# Quick Add for Container/Location Contents — Design

**Date:** 2026-07-21 (revised same day: added the Contents manifest field per Nick)
**Status:** Approved (Nick, 2026-07-21)
**Driver:** No fast path for entering many *different* simple items while unpacking a tote. Existing paths are heavyweight (full Create modal), AI-driven (Catalog contents), or same-item-N-times (template batch create). Additionally (Nick, with a real example — a baby-clothes tote whose list was crammed into the description as one run-on paragraph): sometimes contents don't warrant real item records at all — a line-per-line free-text **manifest** on the container is enough, but it must display as entered and be searchable.

## Contents manifest field (new)

- New nullable TEXT column `contents` on entities (additive goose migration, sqlite3 + postgres — old binary tolerant, consistent with fork migration discipline).
- Exposed as `contents` on EntityOut; settable via EntityCreate/EntityUpdate.
- **Search:** the `q` predicate extends to match `contents` (wherever it currently matches name/description), so a manifest line like "Baby Hats" finds the tote itself.
- **Display:** a "Contents" card on the location page, rendered line-per-line exactly as stored (`whitespace-pre-line` or line-split rendering); hidden when empty. Editable as a textarea on the location edit page.
- Related one-line fix, included: description display preserves entered line breaks (`whitespace-pre-line`) instead of collapsing to a run-on paragraph.
- Plain text lines only — no per-line check-off, no promotion-to-item (both possible later).

## Behavior

An inline "Quick add" row above the items list on the location page (`frontend/pages/location/[id]/index/index.vue` — containers and plain locations both use it). Keyboard-first: a single text input plus an Add button, and a two-state mode toggle:

- **Items mode (default, always):** each entry creates a real item (behavior below). The default is fixed — no persistence of the toggle.
- **List mode:** each entry appends one line to the entity's `contents` manifest (server round-trip via entity update; optimistic UI; Undo removes the line). Quantity-prefix parsing does NOT apply in List mode — lines are stored literally ("3x AA batteries" stays "3x AA batteries"). Paste multi-line appends all non-empty trimmed lines. Single-user app: last-write-wins on the contents field is acceptable, no optimistic-concurrency machinery.

1. **Enter-per-item loop:** typed name → Enter → item created immediately with defaults: entity type = default Item type, parent = this location/container, auto asset ID, quantity 1 (unless parsed). Input clears, focus retained. Whitespace-only input no-ops.
2. **Quantity prefix parsing:** a leading `<int>x ` or `<int> x ` (case-insensitive) sets quantity and is stripped from the name — `3x AA batteries` → qty 3, name "AA batteries". Bounds: clamp to 1..999. Any non-matching text is literally the name. Parsing lives in a pure, unit-tested function.
3. **Paste multi-line:** pasting text containing newlines splits per line, trims, drops empties, parses each line, creates sequentially with a compact progress indicator ("7/15…"). Per-line failure handling: failed lines are restored into the input (only the failures) so typed content is never lost; successes proceed.
4. **Optimistic list + undo:** each created item appears in the contents list immediately with a transient "created / Undo" affordance; Undo deletes the item (real DELETE).
5. **Label wrap-up:** a session counter tracks items added via quick add; a toast (after idle, or a small "Print labels (N)" affordance in the row) enqueues them into the existing label print queue with assetId threaded — same pattern and store as batch create.
6. **Concurrency:** creations are sequential (orderly asset IDs); Enter during an in-flight paste run queues the new line rather than interleaving.

## Non-goals

Photos, custom fields, template hookup, duplicate-name warnings (two identical names = two items, correct for inventory), asset-ID name suffixing (that convention stays exclusive to batch create's identical-copies case), per-line check-off, promote-line-to-item. Items mode needs no backend changes (per-item `POST /v1/entities` like the UPC count path); the only backend surface is the `contents` field + search predicate.

## Acceptance criteria

1. On a container page: type name + Enter → item exists under that container with defaults, input refocused, list shows it.
2. `3x AA batteries` creates quantity-3 "AA batteries"; `3x` alone is a literal name; clamping at 999 works.
3. Pasting 15 lines (some empty, some with qty prefixes) creates the non-empty ones sequentially; simulated mid-run failure leaves only failed lines in the input.
4. Undo removes the created item from backend and list.
5. "Print labels (N)" enqueues exactly the session's quick-added items with asset IDs into the print queue.
6. Parser covered by unit tests; typecheck + eslint clean; full frontend label/composable suites stay green (56).
7. Contents: List-mode Enter appends a line; the Contents card shows lines exactly as entered; search `q` matching a contents line returns the container (through the real handler path, regression test included); description newlines display preserved; migration applies cleanly to a copy of a seeded DB and the old-binary tolerance rule holds (nullable additive column only).
8. Full backend suite green.
