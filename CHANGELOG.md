# Changelog

All notable changes to this fork are documented in this file. Format loosely follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Upstream is [sysadminsmedia/homebox](https://github.com/sysadminsmedia/homebox); this file only covers fork-specific work on top of v0.26.2.

## Unreleased - 2026-07-18

### Security

- Enforced collection ownership before entity, entity-type, attachment, tag,
  hierarchy, integration, group-member, and import/export side effects; added
  cross-collection regression coverage for reads, writes, and error responses.
- Hardened OIDC discovery, outbound URLs, redirect handling, password-reset and
  login throttling, sessions, multipart/archive parsing, MIME validation, CSV
  formula output, custom-field links, telemetry, and secret-bearing logs.
- Removed authenticated API responses from the service-worker runtime cache and
  added one-time cleanup of the legacy `api-cache`, whose URL-only cache keys
  could cross user or collection boundaries.
- Updated the Go and frontend dependency graphs to patched releases; strict
  `gosec`, `golangci-lint`, production dependency audit, and reachable
  vulnerability scans are clean.

### Fixed

- Made multi-step entity, template, tag, CSV, attachment, and restore mutations
  atomic, with database commit preceding best-effort blob deletion and shared
  blob references retained until their final owner is removed.
- Rejected hierarchy cycles, foreign-collection relationships, invalid custom
  fields, negative quantities, and unsafe pagination; restored the explicit
  sync-child location contract without enabling it by default for containers.
- Preserved template default tags, template photos, attachment blobs, IDs, and
  relationships through export/import, including fail-closed archive and
  rollback behavior.
- Corrected container totals and active-inventory valuation, template and batch
  defaults, AI/bulk-catalog race handling, integration redaction, label grid and
  queue behavior, tenant switching, and owner/member workflows.
- Made in-memory background-job topics reusable across repeated attachment,
  thumbnail, export, and import publications, and made worker shutdown clean and
  idempotent.
- Serialized startup and daily cleanup writes so fresh SQLite launches no
  longer emit transient lock errors, and retained background-task identities
  for diagnostics.
- Corrected dynamic-label translation handling and supplied accessible dialog
  and drawer metadata, eliminating the confirmed browser console warnings.

### Data and migrations

- Added tenant-oriented indexes for dominant entity, hierarchy, tag, template,
  field, and membership queries on SQLite and PostgreSQL.
- Enforced the invariant that a container type is also a location type, with a
  forward migration that normalizes legacy rows and named database constraints
  that reject future invalid writes.
- Assigned asset IDs immediately to fresh-install default locations instead of
  relying on a later process restart to backfill them.
- Configured SQLite write transactions to reserve the writer at `BEGIN`,
  preventing deferred read-to-write upgrade races while retaining WAL readers.

### Build, release, and operations

- Pinned reproducible Go, Node, pnpm, lint, release, SBOM, action, and container
  toolchains; corrected multi-architecture image logic and excluded signing-key
  material from container build contexts.
- Made GitLab merge-request pipelines fork-safe and prevented registry login,
  push, or shared-cache mutation outside trusted release contexts.
- Hardened currency and GHCR maintenance workflows with immutable sources,
  bounded downloads, schema validation, atomic file replacement, and fork-aware
  ownership.
- Made Compose require the API-key pepper, persist `/data`, use the supported
  log setting, retain exact build provenance, and restart unless stopped.
- Implemented strict positional YAML loading with documented precedence and
  made production/unknown demo mode require an explicit 12+ byte password.
- Aligned binary releases to the nine Go-supported targets; added archive,
  SBOM, checksum, and generated-tree gates; fixed embedded build timestamps and
  clean `--version` behavior; and removed release-time source mutation.

### Validation

- Added regression coverage for authorization boundaries, transactions,
  storage lifecycle, migrations, query plans, background jobs, tenant-aware
  frontend state, custom workflows, and PWA cache privacy.
- Validated clean and v0.26.2-era SQLite upgrades, clean PostgreSQL migrations,
  backend race/static-security suites, frontend lint/type/unit/build checks,
  and regular, rootless, and hardened container paths. See
  `REMEDIATION_REPORT.md` for the final matrix and environment-dependent limits.

## v0.26.2-phase3.3 - 2026-07-06

### Fixed
- Multi-page label sheets printed page 2+ shifted ~0.25in up, misaligning the labels against the physical Avery stock. Sections flowed continuously so a full sheet plus the next section's top padding straddled the page boundary. Now forces `break-before: page` on every sheet after the first so each starts at the exact same physical position.

## v0.26.2-phase3.2 - 2026-07-06

### Fixed
- Label printing clipped the outer label columns left and right in Safari (and shrank-to-fit in Chrome). The label sheet renders at the full physical page width with the Avery margins baked into its own padding; without an explicit `@page` rule the browser reserved its default print margins, pushing the full-width sheet past the printable area. Now emits a reactive `@page { size; margin: 0 }` sized to the current sheet so it prints 1:1, edge to edge.

## v0.26.2-phase3.1 - 2026-07-05

### Bulk tote cataloging

- New `POST /v1/actions/analyze-photo-bulk` endpoint with shared upload preamble, backing multi-photo catalog sessions where users photograph multiple items in a container, review AI-generated candidates per item (edit hints, mark duplicates, uncheck unwanted candidates, or retry a card with a new photo), and batch-create entities into the target container.
- Catalog-contents entry point on location and container pages — shows a dialog to upload photos, review candidates, and commit the batch.
- Contents-snapshot photos on containers (separate from the container thumbnail) — uploaded during the final commit and stored with the same blob handling as entity attachments.
- Bulk vision method `AnalyzeContents` on both AI adapters (OpenAI-compatible and Anthropic messages).

### Integrations settings UI

- New group-scoped integrations settings page (accessed via collection settings), with database-backed storage for AI provider, base URL, model, API key, barcodespider token, and OpenFoodFacts contact email.
- Environment variables (`HBOX_AI_*`, `HBOX_BARCODESPIDER_*`, `HBOX_OPENFOODFACTS_*`) now serve as per-field fallback defaults; UI edits are persisted to the database and live-applied without restart.
- Owner-only editing of settings; all secrets are write-only (never echoed back in GET responses).
- Test Connection buttons for AI and barcodespider endpoints.
- New `GET/PUT /v1/groups/integrations` endpoints with redaction and owner-only gating, plus runtime resolution of effective (DB-over-env) config. Goose migration `20260705130000` adds `groups.integrations` column.

### Behavior changes

- `/v1/actions/analyze-photo` and `/v1/actions/analyze-photo-bulk` now return `503 Service Unavailable` when AI is unconfigured, rather than `404`. Client can gate the UI accordingly.
- Environment misconfiguration (missing required fields) now emits a warning at startup instead of aborting the server.
- Shared `Dialog` component gained an optional `beforeClose` guard: a callback that can prevent dialog closure if it returns a falsy value.

### Fixes

- Bulk catalog analysis no longer sends `json_object` response format to the AI adapter — was guaranteed to 502 on Ollama with qwen3-vl.
- Test-connection endpoints extend the request deadline past the global write timeout (they can be slow on cold model loads).
- Bumped test AI fixture image from 1×1 to 64×64 pixels — the 1px image crashed qwen3-vl on Ollama.
- Env hint field shows the raw environment defaults, not the effective (DB-over-env) config.
- Close guard in bulk review dialog now covers the X button; snapshot uploads are idempotent across retries.

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
