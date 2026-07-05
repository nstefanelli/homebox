# Changelog

All notable changes to this fork are documented in this file. Format loosely follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Upstream is [sysadminsmedia/homebox](https://github.com/sysadminsmedia/homebox); this file only covers fork-specific work on top of v0.26.2.

## v0.26.2-ai-icons.1 - 2026-07-05

### AI add-by-photo

- New `POST /v1/actions/analyze-photo` endpoint backed by a pluggable vision-LLM adapter (OpenAI-compatible, including local Ollama, and Anthropic messages API), configured via `HBOX_AI_PROVIDER` / `HBOX_AI_BASE_URL` / `HBOX_AI_API_KEY` / `HBOX_AI_MODEL` / `HBOX_AI_TIMEOUT_SECONDS`. The endpoint and UI are conditionally registered behind an `aiPhotoAnalysis` status flag and are fully hidden when unconfigured.
- Two-lane add-by-photo flow on the create form: a barcode found in the photo routes to the existing UPC lookup, otherwise the vision model returns name/description/manufacturer/model/tag hints for the user to review and accept, with staged loading states, cancel support, and an "AI guess" verification badge.
- Clickable AI category-hint chips that attach an existing tag or create a new one.
- `EntityCreate` now accepts `manufacturer` and `modelNumber` at creation time — also fixes the barcode lookup lane, which was silently dropping those two fields.
- Per-request deadline extension on `analyze-photo` so a cold local-model load isn't killed by the global write timeout.

### Entity icons

- New `entities.icon` column (goose migration `20260705120000_add_entity_icon`, sqlite3 + postgres) for a per-entity icon override, independent of entity type.
- Tree and entity-path (breadcrumb) APIs now carry `icon`, `typeIcon`, and `isContainer`, backed by an icon resolver that falls back to a type-level default when no override is set.
- 16 new icons added to the location/container icon registry.
- New icon picker/selector wired into the create modal, location edit page, tree view, cards, other entity selectors, and breadcrumbs.

## v0.26.2-containers.1 - 2026-07-05

- New `Container` entity type (`is_container` flag on entity types) with a dedicated filter, distinct styling in the entity-type selector, and a container/tote-aware create toggle.
- Nested contents: containers show their held items on the location page, and container/location tree, card, and selector views respect the container hierarchy.
- Move and empty-container quick actions on location/container pages, including bulk-move of everything out of a container.
- Batch container creation from the create modal (numbered entities from a template) with print handoff, plus a general batch-create API endpoint with group-ownership validation.
- Template photos: storage, upload/serve/delete endpoints, UI for attaching a template photo, and automatic copy of the template photo to entities created from it, with the same hardening (nosniff/CSP/safe disposition, blob refcounting, 404 on missing blob) as the regular attachment handler.
- Container catalog import support.
- QR label generator: preset definitions, print-queue mode with a selection store, and print-queue entry points from the tree, location page, and items table.
- Assorted correctness fixes surfaced by e2e coverage: entity-type selector label/badge text concatenation, empty print-queue falling back to the all-assets view, container catalog import 500s on an empty `timeValue`, containers not defaulting `sync_child_entity_locations` (which would otherwise flatten contents on move), batch `startNumber` inference, and client-side batch count clamping (1-100).
