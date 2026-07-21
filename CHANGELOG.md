# Changelog

All notable changes to this fork are documented in this file. Format loosely follows [Keep a Changelog](https://keepachangelog.com/en/1.1.0/). Upstream is [sysadminsmedia/homebox](https://github.com/sysadminsmedia/homebox); this file only covers fork-specific work on top of v0.26.2.

## v0.26.2-e2e.5 - 2026-07-21

### Added

- Quick add on container/location pages: an inline, keyboard-first entry
  row — type a name, press Enter, the item is created in place with
  defaults (parent, default type, auto asset ID). A leading `3x ` prefix
  sets quantity; pasting a multi-line list creates items sequentially
  with per-line failure recovery; created items are undoable and a
  "Print labels (N)" affordance enqueues the session's additions.
- Contents manifests: a List mode on the same quick-add row appends
  free-text lines to a new `contents` field on containers/locations —
  for totes whose contents don't warrant real item records. Rendered
  line-per-line in a Contents card, editable on the location edit page,
  and included in search (a manifest line finds its tote). Additive
  nullable migration (20260721000000, both dialects).
- Import readiness now treats a seeded location carrying a contents
  manifest as user data (blocks import wipe).

### Fixed

- Description text preserves entered line breaks when displayed
  (markdown soft-break rendering opt-in on description surfaces).

## v0.26.2-e2e.4 - 2026-07-21

### Added

- Label sheets can now be downloaded as a real PDF (Download PDF on the
  label generator): exact point geometry (Letter = 612×792pt) generated
  client-side with pdf-lib from the same preset/grid/fit/offset state as
  the print view. This bypasses the browser print engine entirely —
  WebKit/Safari prints CSS inches 6.7% oversize (legacy 1.25 shrink
  factor) and can never print the HTML sheet at true scale — and enables
  printing from macOS Preview and iOS/AirPrint at 100% / Actual Size.
  Non-Chromium browsers see a hint steering them to the PDF. Text uses a
  bundled Noto Sans subset (Latin/Greek/Cyrillic; CJK/Thai/Arabic names
  render as placeholder glyphs — known limitation).

- Search now covers locations and containers, not just items: a kind
  filter (All / Items / Containers / Locations) on the search page,
  kind badges/icons on results, and kind-aware navigation. The
  `/v1/entities` endpoint gains an opt-in `includeAllKinds` param — its
  items-only default is unchanged for all existing callers.

### Fixed

- Seeded default entity types ("Item", "Location") displayed their raw
  i18n keys (`global.item`, `global.location`) in the type dropdown,
  create-modal labels/toasts, and the entity-types page — a regression
  from the warning-cleanup in 8174c1ac. Names that are known translation
  keys are translated again; user-created literal names pass through
  untouched.
- Items without an asset ID no longer share a single colliding QR
  payload (`/a/000-000`); their labels now encode the entity URL.

## v0.26.2-e2e.3 - 2026-07-21

### Changed

- Labels now show the formatted asset ID as the bold identifier on every
  label — items, containers, and locations alike — omitted only when an
  entity has no asset ID (a meaningless `000-000` is never printed). The
  asset ID is the single findable number across the UI list, the label,
  and the QR code.
- Label names auto-fit: the name row steps its font size down (to a 65%
  floor) until the full name fits, ellipsizing only past the floor, so
  batch numbers at the end of long names stay visible.
- Batch create (template batch and UPC-scan count) names each created
  entity `<base> #<asset-id>` from the asset ID assigned in the same
  operation, replacing the per-batch sequence counter — one identity
  number per entity instead of two. When no asset ID is assigned
  (auto-increment disabled), the legacy sequence suffix is kept so names
  stay distinguishable.

### Added

- Design spec `docs/superpowers/specs/2026-07-21-homebox-label-identity-design.md`;
  unit tests for the auto-fit sizing function and asset-ID batch naming
  through the real batch-create path.

## v0.26.2-e2e.2 - 2026-07-21

### Fixed

- Restored the label-print fixes dropped by the `main` rebuild for PR #2
  (originally phase 3.4/3.5 on `feature/containers`): the 0.1in print-safe
  inset inside each label cell, the Print Offset X/Y calibration inputs
  (localStorage-persisted), and line-clamped label text.
- Corrected the Avery 22806 preset's vertical geometry to the official
  template (0.625in top/bottom margins, 7/12in row gutter — was 0.6/0.6/0.6),
  so rows register on the physical die-cut.
- Reimplemented the print offset as clamped padding deltas on the sheet
  section instead of a CSS translate: a positive X offset used to push the
  sheet past the `@page` box and trigger Chromium's print shrink-to-fit,
  compressing column pitch by roughly the offset being calibrated (latent in
  the original phase 3.5 implementation).
- Sized label QR codes to the cell content box (`min(height × 0.9,
  content width × 0.6)`): the previous height-only formula overflowed the
  square 22806 cell, clipping the QR and collapsing the text column.
- Contained long label text: unbreakable tokens no longer print across
  neighboring labels (cell `overflow-hidden`, `min-w-0` + `break-words` text
  column) and wrapped lines clamp cleanly instead of clipping mid-glyph.
- Made the label generator's asset range end-inclusive (start 1 / end 30 now
  prints 30 labels) and set the default range to fill exactly three 30-up
  sheets.
- Fixed collection import always returning 409 for freshly registered
  collections: `isSeedLocation` required `quantity == 0 && asset_id == 0`,
  but the registration seeder creates default locations with the schema
  default quantity (1) and sequential asset IDs 1..8. Seed rows are now
  matched per-row on emptiness plus a collective numeric-shape check
  (legacy all-zero or the seeder's contiguous asset-ID range, duplicate-free)
  so a user-recreated, seed-named location still correctly blocks import
  instead of being silently wiped.

### Added

- Label geometry regression tests pinning every preset to the official Avery
  dimensions plus a page-closure invariant, and import-readiness tests
  covering the real registration seeder, recreated seed-named locations
  (out-of-range / zero / duplicate / boundary asset IDs), and backfilled
  legacy seeds.

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
