# Quick Add for Container/Location Contents — Design

**Date:** 2026-07-21
**Status:** Approved (Nick, 2026-07-21)
**Driver:** No fast path for entering many *different* simple items while unpacking a tote. Existing paths are heavyweight (full Create modal), AI-driven (Catalog contents), or same-item-N-times (template batch create).

## Behavior

An inline "Quick add" row above the items list on the location page (`frontend/pages/location/[id]/index/index.vue` — containers and plain locations both use it). Keyboard-first: a single text input plus an Add button.

1. **Enter-per-item loop:** typed name → Enter → item created immediately with defaults: entity type = default Item type, parent = this location/container, auto asset ID, quantity 1 (unless parsed). Input clears, focus retained. Whitespace-only input no-ops.
2. **Quantity prefix parsing:** a leading `<int>x ` or `<int> x ` (case-insensitive) sets quantity and is stripped from the name — `3x AA batteries` → qty 3, name "AA batteries". Bounds: clamp to 1..999. Any non-matching text is literally the name. Parsing lives in a pure, unit-tested function.
3. **Paste multi-line:** pasting text containing newlines splits per line, trims, drops empties, parses each line, creates sequentially with a compact progress indicator ("7/15…"). Per-line failure handling: failed lines are restored into the input (only the failures) so typed content is never lost; successes proceed.
4. **Optimistic list + undo:** each created item appears in the contents list immediately with a transient "created / Undo" affordance; Undo deletes the item (real DELETE).
5. **Label wrap-up:** a session counter tracks items added via quick add; a toast (after idle, or a small "Print labels (N)" affordance in the row) enqueues them into the existing label print queue with assetId threaded — same pattern and store as batch create.
6. **Concurrency:** creations are sequential (orderly asset IDs); Enter during an in-flight paste run queues the new line rather than interleaving.

## Non-goals

Photos, custom fields, template hookup, duplicate-name warnings (two identical names = two items, correct for inventory), asset-ID name suffixing (that convention stays exclusive to batch create's identical-copies case), backend changes (per-item `POST /v1/entities` like the UPC count path).

## Acceptance criteria

1. On a container page: type name + Enter → item exists under that container with defaults, input refocused, list shows it.
2. `3x AA batteries` creates quantity-3 "AA batteries"; `3x` alone is a literal name; clamping at 999 works.
3. Pasting 15 lines (some empty, some with qty prefixes) creates the non-empty ones sequentially; simulated mid-run failure leaves only failed lines in the input.
4. Undo removes the created item from backend and list.
5. "Print labels (N)" enqueues exactly the session's quick-added items with asset IDs into the print queue.
6. Parser covered by unit tests; typecheck + eslint clean; full frontend label/composable suites stay green (56); no backend diff.
