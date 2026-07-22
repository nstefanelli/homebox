# Product Lookup by Keyword + Item Enrichment — Design

**Date:** 2026-07-21
**Status:** Approved (Nick, 2026-07-21)
**Driver:** Completes the intake trio (barcode scan, AI photo, and now keyword) and adds an enrichment path for existing items (e.g. AI-cataloged tote contents that have names but no manufacturer/model/description).

## Decisions (Nick)

1. **Provider first, AI fallback:** upcitemdb keyword search is the primary source; a one-click "Ask AI instead" hands the keyword to the configured LLM, badged as an AI guess.
2. **Full prefill including photo** on the create path.
3. **Per-field merge preview** on the enrichment path — blanks pre-checked, filled fields unchecked (overwrite is explicit opt-in).

## Backend

- **Keyword search endpoint** alongside the existing barcode lookup (study `backend/app/api/handlers/v1/v1_ctrl_product_search.go` and mirror its conventions/route shape): query param keyword → upcitemdb `https://api.upcitemdb.com/prod/trial/search?s=<kw>` → mapped through the existing `repo.BarcodeProduct` shape with the same bounded-body/timeout hardening, `SearchEngineName` provenance set, results capped (~10). Provider chain: upcitemdb only for now (Barcodespider/OpenFacts have no comparable keyword search) — structure so more providers can chain later.
- **AI fallback action**: an action endpoint taking a keyword and returning ONE `BarcodeProduct`-shaped candidate plus an `aiGuess: true` flag. Gated exactly like analyze-photo: 503 when AI is unconfigured; reuse `backend/pkgs/ai` provider adapters with a structured-output prompt (name, manufacturer, model, description; instruct the model to leave model-number empty when unsure rather than invent).
- **Product image** reuse: the barcode path's hardened image fetch (size cap, redirect cap, resolver guards) is the only way a product image enters the system — the keyword path must flow through the same code, not a new fetcher.

## Frontend

- **Create modal**: a lookup button beside the name field (joining barcode + AI-photo), seeded with the typed name. Dialog: keyword input, search, candidate cards (name/brand/thumbnail/provenance). Picking prefills name/manufacturer/model/description and attaches the product photo via the existing product-image path. Prefill clears an active template (same semantics as barcode prefill). "Ask AI instead" appears when the provider returns zero results (and after any search, as a secondary action) only when AI is configured; AI results carry the existing "AI guess — please verify" badge.
- **Item enrichment**: an "Enrich from lookup" action on the item page (item-kind entities only), seeded with the item's name → same picker dialog → **merge dialog**: one row per field (name, manufacturer, model, description, photo) showing current → proposed with a checkbox per row; empty-current rows pre-checked, non-empty unchecked; photo row adds the image as an item photo (primary only when the item has none). Apply commits checked fields in a single entity update (+ photo attach), toast summarizes applied fields. AI-fallback candidates flow through the same dialog with the badge.
- **Merge logic is a pure function** (`computeMergePlan(current, proposed) → rows with defaults`) in lib/, unit-tested independently of the dialog.

## Acceptance criteria

1. Keyword search returns mapped candidates with provenance; provider HTTP mocked in handler tests following the existing product-search test patterns; hardening limits enforced (oversized body rejected).
2. AI action: 503 when unconfigured; with a fake AI provider, returns one flagged candidate; prompt instructs no invented model numbers.
3. Create modal: pick → all four text fields + photo prefilled; template cleared; AI path badged.
4. Enrichment: merge dialog defaults (blanks checked, filled unchecked) proven by `computeMergePlan` unit tests; apply updates only checked fields (unchecked fields byte-identical after update); photo primary only when none existed.
5. Suites: full backend green; frontend 71 stay green plus new tests; typecheck + eslint clean.

## Out of scope

Paid provider keys (existing config patterns cover it later), result caching, enrichment for locations/containers, bulk enrichment.
