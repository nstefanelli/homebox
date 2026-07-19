# Homebox Fork End-to-End Remediation Report

- Review date: 2026-07-18
- Baseline: `1675fab0` (`main`)
- Validated application revision: `f1966c76`
- Review scope: backend, frontend, SQLite, PostgreSQL, storage, migrations, security, performance, Docker, Compose, CI/release, documentation, and every material fork feature
- Upstream comparison: `sysadminsmedia/homebox` main at `52eb6f35`; common ancestor `91797ff0`

## 1. Executive summary

### Outcome

The fork is stable and suitable for a self-hosted production release, subject to the operator-specific limitations listed in section 7. The review produced 49 focused application-remediation commits over the supplied baseline. No known critical or high-severity defect remains open.

This conclusion is based on more than compilation: the final code passed full backend and race suites, frontend unit/API tests, linting, type checking, static generation, dependency and source security scans, clean and upgrade migrations on SQLite, clean PostgreSQL migrations and workflows, query-plan tests, authenticated browser workflows on desktop and mobile, and build validation of the regular, rootless, and hardened container variants. The final Docker runtime/Compose and release-artifact rows are recorded in section 6.

### Health by area

| Area | Final health | Summary |
| --- | --- | --- |
| Correctness | Green | CRUD, hierarchy, container, template, background-job, statistics, import/export, and owner/member behavior agree across API, storage, and UI. |
| Security | Green | Confirmed tenant-boundary, secret, upload, SSRF, session, OIDC, rate-limit, CSV, PWA-cache, archive, and Docker-default defects were fixed and regression-tested. |
| Data integrity | Green | Multi-step writes are atomic; hierarchy cycles and invalid container types are rejected; blob deletion happens after commit with reference protection. |
| Migrations | Green | Fresh SQLite, v0.26.2-era SQLite upgrade, and fresh PostgreSQL migrations passed with integrity and constraint checks. |
| Frontend | Green with performance debt | Lint, type check, 100 tests, build, authenticated desktop/mobile workflows, labels, and integrations passed. The 1.57 MB entry chunk remains the main optimization target. |
| Deployment | Green | Compose now requires the API-key pepper, persists `/data`, uses the supported log setting, loads strict YAML, and fails closed for an unsafe production demo password. All three image variants and all nine supported binary targets build and run with exact provenance. |
| Maintainability | Green with normal debt | High-risk behavior has focused tests and toolchains are pinned. Seven non-blocking TODOs remain and are classified in section 7. |

### Major risks discovered and removed

The initial application built and its Go tests passed, but several reachable problems made it unsafe to call production-ready:

- scoped entity and entity-type updates could continue into unscoped side effects or fetches after updating zero rows, creating cross-collection mutation or disclosure paths;
- attachment and collection deletion could remove blobs before the database transaction committed, so a rollback could lose live files;
- entity and tag hierarchies accepted self/descendant parenting and could form cycles;
- multi-step entity, template, tag, CSV, and restore operations could partially commit;
- frontend API clients, stores, and asynchronous AI/catalog work could retain the previous collection after a collection switch;
- authenticated API responses were cached by URL in the service worker without collection or authorization context;
- member-visible integration metadata and URLs could reveal sensitive endpoint information or embedded credentials;
- OIDC discovery, notifier/external URLs, uploads, archive extraction, CSV output, sessions, and rate-limit outcomes had reachable trust-boundary defects;
- backup/restore omitted template default-tag remapping and template-photo blobs;
- SQLite deferred transactions could fail during read-to-write upgrades under concurrent writers;
- in-memory pub/sub topics were closed by transient publishers, breaking later thumbnails and export/import jobs;
- fresh default locations could remain without asset IDs until restart;
- the checked-in Compose file omitted the required pepper and durable volume, used the wrong log variable, and reported weak provenance;
- `/data/config.yml` was passed to the binary but silently ignored, and production demo mode could fall back to the public `demodemo` password;
- the release matrix silently skipped an unsupported FreeBSD/RISC-V target, stamped `buildTime` as `now`, panicked on `--version`, mutated generated sources during release, and lacked output-count enforcement;
- four independent startup cleanup writers raced on SQLite and emitted transient `SQLITE_BUSY` errors immediately after migration;
- the baseline Go and frontend dependency graphs included reachable or production dependency advisories;
- Vue package skew hid genuine type errors behind hundreds of incompatible runtime types.

All of the above were fixed directly. Regression tests were added at the narrowest practical layer and broader suites were rerun afterward.

### Remaining limitations

The remaining items are not release blockers:

- the main frontend entry is still 1,572.51 kB minified (511.90 kB gzip), largely because 43 locale JSON modules are eagerly included;
- a cold development-mode dashboard took about 4.5 seconds to become interactive; this is directional evidence, not a production benchmark;
- Nuxt reports the known module-preload sourcemap and large-chunk warnings, and Astro reports two documentation-only warnings;
- real printer stock alignment and real operator AI/barcode credentials were not available; generated sheets, queue/grid behavior, protocol clients, unconfigured gates, and stubbed paths were validated instead;
- the dependency scanner reports four advisories in required Go modules, but `govulncheck` found no imported or reachable vulnerable symbol;
- the tracked `backend/cosign.key` is an encrypted upstream signing key and is excluded from Docker build context; key provenance/rotation remains an owner governance task.
- GitHub-hosted upload, OIDC-backed SLSA issuance, and the hosted verifier cannot be executed locally; their pinned workflow wiring passed structural validation.

## 2. Baseline and remediation ledger

### Baseline

| Check | Baseline result | Evidence |
| --- | --- | --- |
| Git | Clean at `1675fab0` | Fork was 92 commits ahead and 9 behind then-current upstream. |
| Backend build/tests | Pass | Go 1.26.0 build and `go test ./...` passed; listener tests needed execution outside the restricted sandbox. |
| Backend lint | Fail | One complexity error and one Swagger-formatting error. |
| Backend vulnerability scan | Fail | Nineteen reachable standard-library advisories with Go 1.26.0; cleared with Go 1.26.5. |
| Frontend install | Pass with peer/runtime warnings | pnpm lockfile installed; Node 20 was below the supported runtime, so exact Node 22 was used. |
| Frontend lint | Pass | Existing ESLint run passed. |
| Frontend type check | Fail | 441 lines of errors: Vue 3.5.20/3.5.38 skew plus genuine stale API/model use. |
| Frontend tests | Partial | 54 standalone tests passed; live API files could not run without a server. |
| Frontend build | Pass with warnings | Static generation was approximately 11.71 seconds; main entry about 1.60 MB/519 kB gzip. |
| Frontend audit | Fail | One critical and five high production advisories plus lower-severity findings. |
| Docker | Initially unavailable | Docker Desktop was installed but its daemon was not running; it was started for final validation. |

### Prioritized issue ledger

| Severity | Confirmed finding | Root cause | Final status and evidence |
| --- | --- | --- | --- |
| Critical | Cross-collection entity update side effects | A scoped update was followed by unscoped relationship/field work and fetches. | Fixed in `repo_entities.go`; two-collection authorization and zero-row tests pass. |
| Critical | Cross-collection entity-type/container response | Scoped update followed by unscoped fetch. | Fixed in `repo_entity_types.go`; foreign-group tests pass. |
| Critical | Rollback-time blob loss | Storage deletion happened before the owning DB transaction committed. | DB-first/post-commit cleanup plus reference checks; forced-failure tests pass. |
| Critical | Stale tenant frontend and private PWA cache | Collection identity was captured in singleton clients/stores; Workbox cached authenticated API responses by URL. | Request-time group headers, generation guards, keyed stores, removal/cleanup of `api-cache`; tests and live collection workflows pass. |
| High | Hierarchy cycles and cross-tenant parents | Parent ownership existed without self/descendant checks in every path. | Entity/tag self, descendant, depth, and tenant checks added; focused tests pass. |
| High | Partial entity/template/tag/CSV writes | Related rows were written across independent transactions. | Atomic repository transactions and rollback tests added. |
| High | Unsafe collection/member lifecycle | Final-group leaving, owner deletion, and default reassignment had inconsistent behavior. | Deterministic owner/member rules and atomic default reassignment; service/repository tests pass. |
| High | Backup/restore fidelity and unsafe failure behavior | Nested template tag UUIDs and template-photo blobs were omitted; destructive steps could outlive a failed import. | Full remap/blob archive, clean-group guard, bounds and fail-closed validation; round-trip tests pass. |
| High | Integration metadata/secret exposure | Redaction covered explicit token fields but not endpoint/userinfo metadata or role scope. | Owner-only write/test, member-safe feature summary, URL userinfo redaction, write-only secret sentinels; tests pass. |
| High | Password reset and login throttling gaps | Generic middleware cleared limiter state for uniform successful reset responses. | Outcome-aware limiter semantics, constant user-facing behavior, tests for 429 accumulation. |
| High | OIDC/session trust-boundary weaknesses | Discovery/redirect/issuer and session paths relied on insufficient validation. | Discovery and redirect restrictions, cookie/session tests, email normalization, and timing equalization added. |
| High | SSRF and unsafe external URLs | Notifier, AI, telemetry, and external-link paths did not share strict scheme/host/userinfo rules. | Central validators and guarded clients; private/unsafe target tests pass. |
| High | Upload/archive/MIME weaknesses | UI filtering and generic multipart errors were trusted too far. | Byte-level image validation, 413 preservation, archive limits/path checks, safe headers; tests pass. |
| High | Spreadsheet formula injection | User text was emitted directly to spreadsheet-compatible CSV. | Shared formula neutralization across exports; prefix table tests pass. |
| High | Dependency/toolchain advisories | Floating/old Go, frontend, and release dependencies. | Exact patched toolchain and dependency pins; audits/scans pass. |
| High | Public production demo credential | Production images could seed `demo@example.com` with known `demodemo`. | Non-development mode requires explicit 12+ byte password and fails before startup; tests pass. |
| High | Release matrix and metadata were not truthful | GoReleaser silently skipped unsupported FreeBSD/RISC-V, used a nonexistent ldflag symbol, and `--version` panicked. | Matrix reduced to the nine supported targets with explicit ignores and output-count gates; build time and version CLI verified on native artifacts. |
| Medium | SQLite `SQLITE_BUSY` during read-to-write upgrades | Deferred write transactions raced after reads. | `_txlock=immediate` default while preserving explicit operator override; two-connection tests pass. |
| Medium | Pub/sub jobs failed after first publisher | Transient publishers closed shared topics. | Topics live for service lifetime; repeated image/export/import jobs and idempotent shutdown pass. |
| Medium | Fresh default asset IDs appeared only after restart | Assignment relied on later backfill. | IDs 1–8 assigned during fresh user/group creation; next item receives 9 on SQLite and PostgreSQL. |
| Medium | Statistics mixed inactive/sold values and null arrays | Inconsistent active-inventory definitions and serialization. | Quantity-weighted active valuation aligned; empty dimensions serialize as `[]`; tests pass. |
| Medium | Invalid pagination and quantities | Negative/oversized inputs were not normalized consistently. | Explicit bounds, negative quantity rejection, and route/repository tests. |
| Medium | Label grid/queue defects | Invalid persisted presets, a 1x1 gap loop, duplicate queue entries, and page-flow math. | Validated presets/grid, dedupe, queue mode, and explicit page breaks; unit and browser sheet validation pass. |
| Medium | Compose/config contract was misleading | Missing pepper/volume, wrong env name, ignored positional YAML. | Production-safe Compose, real strict YAML precedence, missing-default behavior, and documentation. |
| Medium | Concurrent startup cleanup produced SQLite lock errors | Four immediate daily cleanup plugins opened independent write transactions in the same second. | Cleanup is one named sequential task; all image, Compose, native, and development-demo fresh/recreate logs contain zero lock errors. |
| Low | Dynamic i18n and dialog accessibility warnings | User/database labels were treated as translation keys; dialog/sheet metadata was absent. | Literal dynamic labels, hidden accessible descriptions/titles, and global i18n scope; clean browser diagnostic apart from the configured experimental compiler warning. |

## 3. Custom-feature status

Each feature below was traced from UI action through frontend state/API client, route/controller, validation/service, repository/schema/storage, response refresh, export/import impact, and user-visible errors.

### 3.1 Container-aware entity types and nested semantics

- **Intended behavior:** location-backed entity types can be marked as containers, remain in the location hierarchy, hold items/containers, carry icons/fields, and optionally be movable without inheriting the legacy child-location sync default.
- **Reviewed:** `frontend/pages/collection/index/entity-types.vue`, `frontend/components/Entity/*`, location tree/cards/selectors, `v1_ctrl_entity_types.go`, `repo_entity_types.go`, `repo_entities.go`, Ent schema, migrations, search/tree/path/export behavior, owner permissions, and SQLite/PostgreSQL constraints.
- **Defects fixed:** forged cross-tenant update/fetch; invalid `is_container && !is_location`; cycle/depth gaps; stale selectors; icon/type contract drift; container totals using foreign or inactive rows; incorrect child-location sync behavior.
- **Tests/validation:** cross-tenant repository/API tests; entity-type constraint migration tests on both engines; hierarchy/path/container queries; browser entity-type administration showed `Tote` as container/movable-container; clean and upgrade migrations reject invalid inserts/updates.
- **Remaining concern:** none functionally; drag-and-drop tree reordering remains a separately noted product enhancement.

### 3.2 Container batch creation and print handoff

- **Intended behavior:** create up to 100 numbered containers, with or without a template, place them under a chosen location, and hand the created set to the label queue.
- **Reviewed:** create modal count clamping/defaults, template `batchCreate`, next-number inference, transactional repository work, toast action, label queue and location refresh.
- **Defects fixed:** stale type/template selection, incomplete defaults, non-atomic field writes, cross-group type/location references, count/naming edge cases, duplicate print queue entries, and stale async UI results.
- **Tests/validation:** API/template batch tests and frontend label/store tests; authenticated browser imported the catalog, selected the `Tote` type and HDX 17-gallon template, created a numbered batch under Garage, and rendered 12 queued QR labels with parent rows.
- **Remaining concern:** physical printer calibration is operator/hardware-specific.

### 3.3 Container catalog import

- **Intended behavior:** idempotently create the canonical `Tote` type and standard tote templates with capacity/dimension/color fields.
- **Reviewed:** catalog constants, type normalization, template/field writes, duplicate detection, error feedback, and permissions.
- **Defects fixed:** invalid empty time values, pre-existing but incorrectly flagged type handling, partial import writes, stale model/icon state, and misleading success/error paths.
- **Tests/validation:** catalog unit tests plus browser import: first run reported 12 created, second run 0 created/12 existing; imported templates populated the create workflow.
- **Remaining concern:** catalog maintenance/versioning is manual; future catalog changes should be versioned rather than silently mutating user templates.

### 3.4 Container contents, move, empty, and tree behavior

- **Intended behavior:** show direct containers separately from child locations/items, catalog contents, move containers, empty all direct contents to the parent, and preserve a deduplicated hierarchy.
- **Reviewed:** location detail composables, `getContainers`, move/empty services, scoped counts, tree/path/selector output, `syncChildEntityLocations`, and background refresh.
- **Defects fixed:** duplicated children, foreign-group totals, cycles, non-atomic moves, accidental flattening, unsafe parent choice, and missing empty/move feedback.
- **Tests/validation:** repository container totals/hierarchy tests, sync behavior tests, browser Garage detail with container section and contents, move/empty controls, and nested-location creation.
- **Remaining concern:** very large trees were query-plan reviewed but not benchmarked with a production-sized fixture.

### 3.5 Template-driven creation and template photos

- **Intended behavior:** templates supply entity defaults, tags, location, custom fields, photos, manufacturer/model, and can create one item or a numbered container batch; template photos copy to created entities.
- **Reviewed:** template UI/selector/photo section, API routes, services/repositories, blob storage/refcounts, export/import, delete/rollback behavior, and generated contracts.
- **Defects fixed:** partial field writes; lost default tags; lost template-photo blobs; duplicate UI actions; missing 413 feedback; non-image acceptance; reference/orphan leaks; dropped manufacturer/model; stale default-template state.
- **Tests/validation:** template CRUD/default/tag/location/field API tests, photo MIME and blob lifecycle tests, export/import round trips, final 100-test frontend/API suite, and browser template-default display with capacity/dimensions/color.
- **Remaining concern:** no real high-volume photo benchmark; image processing was exercised repeatedly and under race detection.

### 3.6 AI add-by-photo and barcode lane

- **Intended behavior:** collection-scoped OpenAI-compatible or Anthropic vision analysis prefills an item; barcodes route to product lookup; users review every suggestion and can cancel slow analysis.
- **Reviewed:** group/env precedence, provider adapters, outbound URL guards, request deadlines, upload validation, hints, modal race/cancel state, feature gating, secret redaction, and error mapping.
- **Defects fixed:** stale collection/client/results; invalid provider URLs; metadata/secret exposure; dropped manufacturer/model; Ollama-incompatible response format; timeout cancellation; upload spoofing; unsafe logging; ambiguous unconfigured 404.
- **Tests/validation:** provider/unit/controller/integration-store tests, safe URL and timeout paths, browser owner settings, and explicit `ai not configured` test result. Real third-party credentials were not supplied.
- **Remaining concern:** model quality and provider rate/cost behavior require an operator-specific acceptance test before enabling in production.

### 3.7 Bulk AI tote catalog and contents snapshots

- **Intended behavior:** analyze multiple photos, normalize/deduplicate candidates, let the user edit/retry/skip, batch-create accepted items into a container, and retain container contents-snapshot photos.
- **Reviewed:** dialog/session lifecycle, retry generation, candidate normalization, provider service, entity writes, snapshot upload/idempotency, refresh, close guards, and collection switching.
- **Defects fixed:** stale retries/results; state leakage between containers; invalid type/icon/cache state; duplicate or partial writes; unsafe close; snapshot retry duplication; error feedback.
- **Tests/validation:** `lib/bulk-catalog/session.test.ts`, AI adapter/controller tests, repeated attachment/pubsub workflows, and browser availability/gating from container locations.
- **Remaining concern:** live model output quality was not tested without credentials; deterministic session and failure behavior is covered.

### 3.8 Entity and entity-type icons

- **Intended behavior:** per-entity icons override type defaults and propagate through trees, breadcrumbs, cards, selectors, create/edit, and API summaries.
- **Reviewed:** migration/schema fields, generated models, icon registry/resolver, selector/card/tree/path usage, import/export, and location/container fallbacks.
- **Defects fixed:** stale generated types, invalid icon carry-over after type changes, missing path/summary metadata, selector concatenation, and dynamic i18n misuse.
- **Tests/validation:** icon resolver unit tests, type checking/build, API path/tree workflows, and desktop/mobile browser rendering.
- **Remaining concern:** icon registry changes remain code-driven rather than administrator-configurable.

### 3.9 Collection-scoped integrations

- **Intended behavior:** owners configure/test AI and barcode settings per collection; database values override environment defaults; secrets are write-only; members see only safe capability state.
- **Reviewed:** settings UI/store, group headers, GET/PUT/test routes, service resolution/merge, JSON persistence, redaction, URL validation, role checks, and live refresh.
- **Defects fixed:** ordinary-member metadata disclosure, URL userinfo leakage, stale collection results, sentinel/clear semantics, invalid endpoints, owner-control gaps, and inconsistent failure status.
- **Tests/validation:** controller/service/store owner/member tests, URL/redaction tests, browser settings form and both unconfigured Test buttons, collection-switch guards.
- **Remaining concern:** actual provider connectivity is environment-specific.

### 3.10 Label presets, grid, queue, and multipage printing

- **Intended behavior:** print individual or queued item/location/container QR labels using validated stock presets, configurable grid/margins/skip count, location row, and exact page breaks.
- **Reviewed:** queue store/entry points, grid math, preset persistence, labels for items/locations/containers, selection workflows, HTML/CSS print layout, base URL, and server label maker.
- **Defects fixed:** blanked location text, 1x1 infinite/gap behavior, invalid stored presets, queue duplicates, empty-queue fallback, second-page vertical drift, Safari/Chrome page margins, and label text overflow.
- **Tests/validation:** seven focused label tests plus store tests; browser handoff rendered 12 distinct container labels on Avery 5160 layout with Garage parent rows and the expected 3-column grid.
- **Remaining concern:** physical stock/printer alignment was not available for measurement.

### 3.11 Collection switching, roles, and owner/member lifecycle

- **Intended behavior:** every request uses the active collection at request time; only owners perform destructive/configuration actions; members can safely leave; default-group state remains valid.
- **Reviewed:** auth context, request headers, Pinia stores, route tabs/actions, invitations/members, services, repository defaults, user deletion, sessions, and API keys.
- **Defects fixed:** captured stale group IDs; late response overwrite; member-visible owner controls; unsafe last/default group transitions; owner/member deletion inconsistencies; invite authorization gaps.
- **Tests/validation:** request/store tests, two-user/two-group service/repository/API tests, complete authenticated browser session, and owner-only settings display.
- **Remaining concern:** no granular role beyond owner/member exists; this is an ideation item, not a defect.

### 3.12 Backup, restore, CSV, attachments, and data portability

- **Intended behavior:** export/import the complete collection including IDs, relationships, fields, tags, templates/defaults/photos, attachments, and blobs; reject unsafe or partial input.
- **Reviewed:** archive manifest and limits, UUID remap, clean-group guard, SQL transactions, blob staging/commit/cleanup, CSV imports/exports, external links, formula safety, progress pub/sub, and both engines.
- **Defects fixed:** template tag/photo loss; missing blob validation; partial SQL writes; early blob deletion; unsafe paths/sizes; CSV formulas; nullable field/date contract; refcount leaks; transient pub/sub closure.
- **Tests/validation:** service/repository round trips, corrupted/oversized/failure archives, CSV rollback/formula tests, repeated export/import jobs, SQLite upgrade, PostgreSQL workflow, and clean shutdown.
- **Remaining concern:** an operator should still perform a restore drill against their own largest archive and external storage configuration.

## 4. Changes made

### Backend

| Files/symbols | Change |
| --- | --- |
| `backend/internal/data/repo/repo_entities.go` | Assert target ownership before any mutation; atomic hierarchy/field writes; cycle/depth checks; safe container totals and child-sync semantics. |
| `repo_entity_types.go`, `repo_entity_templates.go`, `repo_tags.go`, `repo_group.go` | Scoped fetch/update/delete, atomic related rows, owner/member/default invariants, type constraints, and tag hierarchy isolation. |
| `repo_item_attachments.go`, `service_items_attachments.go` | DB-first mutations, post-commit cleanup, shared-blob reference protection, scoped attachment access, and external-link safety. |
| `repo_csv_import.go`, `service_exports.go` | Atomic imports; complete archive remapping/blobs; clean-group/fail-closed archive contract; reusable background topics. |
| `backend/app/api/handlers/v1/*` | Pagination/input validation, consistent 4xx/413/503 responses, MIME/archive checks, tenant authorization, safe cookies, integration controls, and bounded upload behavior. |
| `backend/app/api/providers/oidc.go`, `middleware.go` | OIDC discovery/redirect hardening and outcome-aware authentication/reset throttling. |
| `backend/pkgs/ai/*`, `backend/pkgs/mailer/*`, `backend/pkgs/utils/pubsub.go` | Guarded provider clients, deadline normalization, provider compatibility, reusable pub/sub topics, and idempotent shutdown. |
| `backend/internal/sys/config/conf.go` | Real positional YAML loading with defaults < YAML < environment < flags precedence, strict unknown-field/single-document validation, and safe missing `/data/config.yml`. |
| `backend/app/api/demo.go`, `main.go` | Fail closed before startup when production/unknown-mode demo lacks an explicit 12+ byte password. |
| `backend/app/api/bgrunner.go`, `recurring.go` | Preserve task identities and serialize startup/daily cleanup so SQLite writers do not race. |

### Frontend

| Files/symbols | Change |
| --- | --- |
| `frontend/lib/requests/requests.ts`, `lib/api/base/base-api.ts`, `composables/use-api.ts` | Resolve collection headers at request time, preserve nullable contracts, and prevent tenant-stale client behavior. |
| `stores/integrations.ts`, `composables/use-auth-context.ts`, collection pages | Collection-keyed state, in-flight generation guards, owner/member gating, and safe reset on collection change. |
| `components/Entity/CreateModal.vue`, `Location/BulkCatalogDialog.vue` | Stable async/template/AI state, batch defaults/count handling, retry isolation, better failures, and correct create payloads. |
| `components/Template/*`, template pages | Photo lifecycle, template defaults, duplicates, 413 feedback, custom fields, and container compatibility. |
| `lib/labels/grid.ts`, `stores/labels.ts`, `pages/reports/label-generator.vue` | Validated presets/grid/queue, deduplication, correct container/location text, and multipage print layout. |
| `lib/utils.ts`, `global/DetailsSection/*`, Markdown/form components | HTTP(S)-only links, literal custom labels, safe rendering, and normalized form validation. |
| `nuxt.config.ts`, `pwa-cache.ts`, `00.pwa-cache-cleanup.client.ts` | Remove private API runtime caching and delete the legacy cache while keeping API navigations out of PWA fallback. |
| `App/CreateModal.vue`, `ui/sidebar/Sidebar.vue`, `Entity/Selector.vue` | Accessible dialog/drawer metadata and clean dynamic label/i18n behavior. |
| `package.json`, `pnpm-lock.yaml`, generated contracts | Align Vue/Nuxt router packages, patch vulnerable Workbox/transitives, and match backend API models. |

### Database and storage

- Added tenant-oriented indexes in `20260718000000_add_tenant_query_indexes.sql` for entity archive/name, parent hierarchy, tags, entity types, templates, members, tag edges, and field foreign keys on SQLite and PostgreSQL.
- Added forward-only `20260718130000_require_container_location.sql`, normalizing existing rows before enforcing the named `is_container => is_location` constraint on both engines.
- Set SQLite `_txlock=immediate` unless explicitly configured otherwise; WAL readers remain enabled.
- Assigned asset IDs during default-location creation rather than relying on restart-time backfill.
- Reordered blob lifecycle so storage deletion follows committed metadata changes and occurs only after the final reference.

### Security

- Closed confirmed tenant IDORs for entities, types, attachments, tags, groups, memberships, integrations, and import/export.
- Hardened sessions/cookies, OIDC, login/reset rate limits, constant-time/timing-equalized credential paths, redirect and email handling.
- Centralized safe outbound URL and notifier validation, redacted secrets/userinfo/logs, and bounded telemetry.
- Added byte-level image validation, multipart 413 mapping, archive traversal/size limits, safe content headers, CSV formula neutralization, and HTTP(S)-only custom links.
- Removed private API responses from Workbox cache; excluded signing material from container context; pinned actions, images, Go, Node, pnpm, release, SBOM, lint, and scanner tooling.
- Required the API-key pepper in Compose and the runtime, and removed the public production demo fallback.

### Performance and reliability

- Added measured query-plan indexes rather than speculative micro-optimizations.
- Removed stale/redundant tenant requests and late async overwrites; retained list pagination and bounded batch counts.
- Eliminated SQLite read-to-write lock upgrades, pub/sub topic reuse failures, shutdown noise, and mailer timeout races.
- Reduced the main entry by about 32.1 kB minified versus baseline; the much larger locale-lazy-loading opportunity remains deliberately deferred.

### Deployment and release

- Corrected architecture conditions and stale embedded assets in all Dockerfiles; excluded signing key material.
- Made Compose fail before resource creation when the pepper is absent, persist `/data` in `homebox-data`, use `HBOX_LOG_LEVEL`, restart unless stopped, and pass explicit build provenance.
- Made `/data/config.yml` functional and strict while retaining the empty-volume default.
- Hardened GitHub/GitLab release, fork, cache, currency-update, and image-cleanup workflows; pinned toolchain/action versions and unique artifact/checksum/SBOM wiring.
- Aligned GoReleaser with the nine Go-supported targets, corrected embedded build time and `--version`, removed release-time source mutation, and added generated-tree plus archive/SBOM/checksum count gates.

### Tests

High-risk additions cover tenant authorization, zero-row updates, forced transaction rollback, cycles/depth, blob references, MIME/archive limits, rate limits, OIDC/sessions, user/group deletion, pagination, CSV safety, import/export fidelity, pub/sub reuse, graceful shutdown, migration constraints/query plans, request-time collection headers, integration-store generations, PWA privacy, label grids/queues, catalog sessions, icons, and custom feature APIs.

### Documentation

- Updated README and install/configuration documentation for the pepper, Compose volume/restart/provenance contract, rootless ownership, YAML file loading and precedence, safe demo mode, and pinned runtime expectations.
- Added this report and a dated `CHANGELOG.md` remediation entry.

## 5. Performance and efficiency results

| Workload | Baseline | Final | Interpretation |
| --- | ---: | ---: | --- |
| Main frontend entry | 1,604.62 kB minified | 1,572.51 kB | 32.11 kB / 2.0% smaller. Final gzip is 511.90 kB. |
| Frontend production build | ~11.71 s observed | 11.30–17.16 s observed across warm/fresh states | No defensible timing improvement; cache state dominates. Final build transformed 4,538 modules and prerendered 23 routes. |
| Locale payload inside entry | Not separately measured | 43 eager JSON modules, 1,426,900 rendered bytes / 379,088 gzip; ~88.9% of entry | Largest confirmed frontend bottleneck. True lazy loading needs an async locale-loader refactor. |
| Cold development dashboard | Not recorded | ~4.5 s blank-to-render | Directional only; consistent with the entry/locale finding, not a production benchmark. |
| Entity default list plan | Archived predicate + temp sort | `idx_entities_group_archived_name` | Tenant composite index removes the dominant temp-sort/full-scan path. |
| Entity hierarchy plan | Weak/general index | `idx_entities_group_parent` | Direct tenant/parent lookup. |
| Tag list/hierarchy | General scan/sort | `idx_tags_group_name`, `idx_tags_group_parent` | Tenant and parent queries use covering/selective paths. |
| Entity types/templates | General group filtering | group/name composite indexes | Common selector/admin lists avoid temp sorting. |
| Membership/tag edges/fields | Partial FK coverage | dedicated group-user, entity-tag, and field FK indexes | Query-plan regression test proves selected indexes. |
| Concurrent SQLite writers | Reproducible deferred upgrade `SQLITE_BUSY` | Two-connection write tests pass with immediate lock | Reliability gain; explicit operator `_txlock` remains honored. |
| Repeated background jobs | Later publish could fail after topic close | Repeated thumbnail/export/import publications pass | Service-lifetime topics remove a latent one-shot failure. |

No large synthetic inventory benchmark was invented for this report. The database work is supported by explicit before/after query plans and concurrency tests; a production-sized load test is recommended when representative user data is available.

## 6. Final validation matrix

| Check | Command or method | Result | Notes |
| --- | --- | --- | --- |
| Backend formatting | `gofmt` check over backend Go files | Pass | No formatting diff. |
| Backend modules/build | `go mod tidy`, `go mod verify`, `go build ./...` | Pass | All modules verified with Go 1.26.5. |
| Backend unit/integration | `go test ./... -count=1` | Pass | Final head; listener tests run with local-network permission. |
| Backend race | `go test -race ./... -count=1` | Pass | Repository/services/background/shutdown paths included. |
| Backend vet | `go vet ./...` | Pass | Final head. |
| Backend lint | `golangci-lint run ./...` | Pass | `0 issues`. |
| Backend static security | strict `gosec -exclude-generated -track-suppressions ./...` | Pass | 162 hand-authored files, 39,789 lines, 27 justified suppressions, 0 findings. Generated Ent code is separately regenerated, tidied, diff-checked, compiled, and tested. |
| Backend vulnerabilities | `govulncheck ./...` with official DB | Pass | No symbol/imported/reachable vulnerabilities; four module-only advisories have no call path. |
| Frontend frozen install | pnpm 10.28.0 with `--frozen-lockfile` on Node 22 | Pass | Clean lockfile install. |
| Frontend dependency audit | full and production `pnpm audit --audit-level=low` | Pass | No known vulnerabilities. |
| Frontend lint | `pnpm lint` | Pass | Final UI remediation included. |
| Frontend type check | `pnpm typecheck` | Pass | Vue/Nuxt package alignment and generated contracts are clean. |
| Frontend tests | Vitest against final local API | Pass | 25 files / 100 tests: 29 API integration + 71 frontend-only. A sandbox-only `EPERM` local-connect attempt was rerun with permission and passed. |
| Frontend build | `pnpm build` / Nuxt generate | Pass | 4,538 modules, 23 routes; only documented sourcemap/large-chunk warnings. |
| PWA privacy | generated `sw.js` plus `pwa-cache` tests | Pass | No `api-cache` or NetworkFirst API route; `/api/` stays in navigation denylist. |
| Documentation | frozen install and Astro build | Pass | 212 pages; deprecated markdown config and unsupported `caddy` highlighter warnings only. |
| Fresh SQLite | startup, migrations, register/login/CRUD/search/custom fields | Pass | Default asset IDs 1–8; first new entity 9. |
| SQLite upgrade | v0.26.2 fixture through migration `20260718130000` | Pass | 19 table counts/IDs/relationships preserved; FK/integrity clean; source SHA-256 unchanged. |
| PostgreSQL 17 | all 30 migrations plus auth/CRUD/search/custom fields/restart | Pass | Named constraint and 10 new indexes present; invalid writes rejected; asset IDs 1–9 correct. |
| Query plans | `TestSQLiteTenantQueryIndexesImproveDominantPlans` | Pass | Explicit before/after plans use the intended composite indexes. |
| Authorization boundary | two users/two collections across entities/types/tags/attachments/groups/integrations/export | Pass | Foreign reads/writes and zero-row side effects rejected. |
| Transaction rollback | forced constraint/storage/relationship failures | Pass | Entity/template/tag/CSV/restore operations leave no partial state. |
| Import/export/blob fidelity | service/repository round trips and repeated jobs | Pass | Template tags/photos and attachments preserved; corrupt/oversized archives fail closed. |
| Background jobs/shutdown | repeated thumbnail/export/import plus Ctrl-C | Pass | Topics reusable; SQLite/PostgreSQL shutdown clean and idempotent. |
| Authenticated browser | desktop CRUD/search/table/location/template/catalog/container/label/integration flows | Pass | Created item, nested location, 12 catalog templates, numbered containers, 12-label sheet; no application error log. |
| Mobile browser | 390 x 844 items view and navigation drawer | Pass | Core controls and collection administration remain accessible. |
| Browser diagnostics | warnings/errors collected after final retest | Pass with informational warning | No dynamic-key or dialog accessibility warnings; only the intentionally configured vue-i18n custom-compiler experimental warning. |
| Regular Docker image | exact-head build/runtime/health/volume checks | Pass | Image `df8aadcb0483…`, 48,357,202 bytes, root/0:0 data; exact SHA/time, auth, restart, recreation, persistence, and zero lock errors. |
| Rootless Docker image | exact-head build/runtime UID/ownership/health | Pass | Image `b30ce7c0a9f9…`, 48,359,249 bytes, UID/data 65532:65532; exact SHA/time, auth/persistence, and zero lock errors. |
| Hardened Docker image | exact-head build/runtime UID/health | Pass | Image `10fb2390bbf6…`, 35,907,263 bytes, UID/data 65532:65532; exact SHA/time, auth/persistence, and zero lock errors. |
| Compose | missing-secret gate, build/start/health/recreate/persistence | Pass | Missing pepper rejected before creation; exact image healthy in 22.68 s with named volume and `unless-stopped`; auth persisted through restart/force-recreate; zero lock errors. |
| Config file | positional YAML precedence, unknown keys/docs, missing default | Pass | Focused/full config tests and container proof; invalid/multi-document files fail closed. |
| Production demo | missing/short/strong password and startup behavior | Pass | Public default is development-only; production/unknown modes fail closed. |
| GoReleaser/workflows | four configs, actionlint, release/SBOM generation | Pass | All four real snapshots passed: nine supported archives, nine valid SPDX-2.3 SBOMs, 18 verified checksums, exact target binaries, clean generation diff, correct version/time; all 13 workflows structurally clean. |

## 7. Remaining issues and limitations

| Severity | Item | User impact | Why it remains | Recommended next action |
| --- | --- | --- | --- | --- |
| Medium | 1.57 MB main entry and eager locales | Slower cold/mobile startup and more transferred/parsed JavaScript. | A correct fix changes runtime locale loading and SSR/static-generation behavior; it deserves its own measured refactor. | Introduce async per-locale imports, retain English fallback, measure route/cold-load/Web Vitals before and after. |
| Low | Nuxt sourcemap and >500 kB chunk warnings | Build diagnostics/noise; sourcemap precision may be lower. | Emitted by upstream Nuxt module-preload transform and confirmed large bundle. | Recheck after locale split and Nuxt updates; do not simply raise the warning threshold. |
| Low | Astro markdown/caddy warnings | Documentation build noise; one code fence may not highlight. | Existing Astro configuration/highlighter support. | Migrate deprecated markdown config and register/replace the `caddy` lexer. |
| Low, environment-dependent | Physical label alignment | Printer/stock margins may differ even though generated geometry is correct. | No physical printer/Avery stock in the environment. | Print calibration page at 100%, margins off, duplex off; save per-printer offset if needed. |
| Low, environment-dependent | Real AI/barcode provider acceptance | Provider-specific model quality, cost, rate limits, or credentials may differ. | No real secrets were supplied, appropriately. | Run owner-only Test and one reviewed photo/barcode session against the intended provider before enablement. |
| Informational | Four Go module advisories without call path | No known executable exposure. | Required modules contain affected code, but the application does not import/call vulnerable symbols. | Keep dependency updates automated and rerun `govulncheck` on every release. |
| Low, tooling traceability | Syft creator string is `syft-[not provided]` in locally generated SBOMs | SBOM consumers do not see the tool version in `creationInfo`, although each document is valid. | The pinned `go install` binary exposes module provenance for v1.44.0 but lacks injected display metadata. | Prefer a checksum-pinned official Syft binary/action in a separate workflow-hardening change; retain the current exact pin until then. |
| Environment-dependent | Hosted release publication and SLSA OIDC attestation | Local validation cannot prove GitHub token permissions, hosted release upload, OIDC issuance, or verifier service behavior. | These operations exist only on a tagged GitHub Actions run. | Run a release-candidate tag in the intended repository and require upload, provenance, and verifier jobs before promoting the final tag. |
| Governance | Encrypted `backend/cosign.key` remains tracked | Key provenance/rotation is an owner concern; it is not in images. | It is inherited upstream and encrypted; deleting history is a separate trust decision. | Confirm ownership and rotation policy; prefer keyless release signing if practical. |
| Low maintainability | Seven TODOs/commented test | No current functional failure; future UX/reactivity/docs/test debt. | Items depend on Reka UI changes or product choices, or are superseded by newer tests. | Triage profile notice, tag cursor, draggable tree, multi-tab warning, docs API link, formatter reactivity, and obsolete group-stat test in a maintenance issue. |

There are no unresolved critical or high findings. No important path is labeled “passed” solely because it compiled.

## 8. Upstream divergence

At validated application revision `f1966c76`, the fork is **141 commits ahead and 9 commits behind** upstream main. The common ancestor is `91797ff0`; upstream main used for comparison is `52eb6f35`. The application remediation changes 283 files relative to the supplied baseline (14,921 insertions, 5,291 deletions); the full fork differs from the common ancestor across 356 files (27,590 insertions, 5,377 deletions). The later report/changelog handoff commit is documentation-only and is intentionally excluded from these application statistics.

### Highest-conflict areas

- `backend/internal/data/repo/repo_entities.go` and adjacent repositories: tenant scoping, hierarchy, containers, fields, and storage transactions all touch upstream core persistence.
- API handlers, services, generated contracts, and `frontend/lib/api/types/data-contracts.ts`: schema changes cascade across generated and handwritten layers.
- entity types/templates/attachments/export: container flags, default templates, photos, blobs, and backup remapping are tightly coupled.
- frontend create/location/template/collection pages and state stores: custom UI flows overlay upstream navigation, selection, and settings.
- migrations: forward-only custom migrations must stay ordered after upstream schema changes on both SQLite and PostgreSQL.
- release/Docker workflows: fork-safe registry rules and extra hardened/rootless artifacts can conflict with upstream workflow rewrites.

### Good upstream candidates

The following are generic fixes rather than fork policy and could be proposed upstream in small, independently tested changes:

- scoped-update/zero-row authorization patterns and post-commit blob deletion;
- hierarchy cycle protection and pagination bounds;
- CSV formula neutralization and upload/MIME/413 normalization;
- forgot-password limiter outcome handling, OIDC/session hardening, and URL userinfo redaction;
- SQLite immediate-write transaction option, query indexes, pub/sub topic ownership, and clean shutdown;
- PWA authenticated-API cache removal;
- strict positional YAML loading, production-safe Compose secret/volume defaults, and production demo fail-closed behavior;
- label grid/page-break fixes that apply to upstream’s generator.

### Recommended maintenance strategy

1. Fetch upstream at least monthly and before every fork release; do not accumulate another large release-boundary merge.
2. Merge or rebase in a dedicated integration branch and run tenant, migration, import/export, and generated-contract gates before feature work resumes.
3. Keep each upstreamable fix narrow, with its regression test and without container-specific behavior where possible.
4. Keep Go model/OpenAPI/generated TypeScript changes in the same commit so contracts cannot drift.
5. Never rewrite deployed migration history; add corrective forward migrations for both engines.
6. Treat `repo_entities.go`, export/import, templates, collection state, and Docker workflows as mandatory manual-review hotspots.
7. Maintain a small compatibility document mapping upstream concepts to fork additions (`is_container`, template photos/defaults, group integrations, label queue, AI/catalog jobs).

## 9. Static hygiene disposition

The final search covered TODO/FIXME/HACK/XXX markers, debugging statements, swallowed errors, hard-coded secrets/URLs, feature flags, routes/components, loops, query bounds, generated-source drift, and release outputs. No new hard-coded secret, dead security flag, unbounded custom batch, or swallowed production failure was found. Remaining TODOs are listed in section 7. Debug-level non-fatal formatter logging and intentional public/documentation URLs were validated as such. The production demo default, ignored config path, cached private API, signing-key build-context exposure, silent release-target skip, stale build timestamp, broken version CLI, and concurrent startup cleanup found during this search were remediated.

## 10. Feature ideation and prioritization

These are design proposals only. None were implemented during remediation. Ideation began only after exact revision `f1966c76` passed the complete application, container, Compose, release, and SBOM matrix. The proposals assume the remediated architecture: a collection-scoped Go API with service/repository boundaries, SQLite and PostgreSQL support, a Nuxt/Pinia PWA, transactional import/export, asset-ID and barcode workflows, container-aware entity types, templates, maintenance entries, attachments, and write-only integration secrets.

Score legend: **User value**, **architectural fit**, and **differentiation** use 5 as best/highest. **Implementation effort**, **maintenance burden**, and **risk** use 5 as hardest/highest. Effort bands: **S** = one focused cycle, **M** = one to two cycles, **L** = multiple cycles, **XL** = cross-cutting program.

### 10.1 Quick wins

#### 1. Inventory Health Inbox

- **Problem:** Missing locations, photos, values, serial numbers, warranty documents, or overdue maintenance are scattered across the collection. **Target user:** collection owners and members maintaining inventory. **Proposed behavior:** calculate deterministic, prioritized findings and link each finding to a filtered correction flow; allow snooze or dismissal where appropriate. **UX:** a dashboard card and dedicated inbox grouped by severity, location, type, and finding, with safe bulk actions.
- **Backend impact:** add a collection-scoped health-query service that reuses existing active-inventory, attachment, maintenance, and hierarchy semantics. **Frontend impact:** add an inbox route, summary card, filters, deep links, and mobile-friendly fix flows. **Database impact:** findings can remain query-derived; a small `health_finding_state` table would store dismissals and snoozes by collection, entity, and stable rule identifier.
- **Security:** preserve collection scoping on every aggregate and deep link; restrict destructive bulk fixes to owners or a future explicit permission. **Migration:** one additive table and indexes, with no existing-data rewrite. **Dependencies:** existing statistics semantics, attachment counts, maintenance dates, and entity search filters.
- **Effort:** S. **Value:** high because it turns existing data into an actionable daily workflow. **Architectural risk:** low; the first version can be read-only apart from dismissal state. **Fork vs upstream:** broadly useful and a strong upstream-first candidate; fork-only container checks can be optional rules.
- **Scores:** User value **5**; implementation effort **2**; maintenance burden **2**; architectural fit **5**; differentiation **3**; risk **1**.

#### 2. Saved Views

- **Problem:** Users repeatedly reconstruct the same location, entity-type, tag, archive, sort, and column filters. **Target user:** collectors, administrators, and power users with large inventories. **Proposed behavior:** save a validated canonical query definition as private or collection-shared, with an optional default view. **UX:** “Save view” beside search, a sidebar/menu picker, rename/duplicate controls, and owner-managed shared views.
- **Backend impact:** add CRUD endpoints that validate filter fields against the existing search contract rather than accepting arbitrary query text. **Frontend impact:** serialize current filters and column state, restore them predictably, and keep views keyed to the active collection. **Database impact:** add `saved_views` with owner user, collection, name, visibility, and bounded JSON configuration.
- **Security:** never store SQL or executable expressions; scope private views to their creator and shared views to the collection; limit shared-view writes to owners. **Migration:** additive table and unique-name indexes. **Dependencies:** stable search/filter parameter names and request-time collection headers.
- **Effort:** S. **Value:** medium-high through reduced navigation friction. **Architectural risk:** low because it wraps existing searches. **Fork vs upstream:** suitable for upstream core with fork-specific filter keys ignored when unavailable.
- **Scores:** User value **4**; implementation effort **2**; maintenance burden **2**; architectural fit **5**; differentiation **2**; risk **1**.

### 10.2 Workflow improvements

#### 3. Scan-to-Tote Session

- **Problem:** Assigning many existing items to a container is slow and error-prone when each item is edited separately. **Target user:** anyone packing, reorganizing, or inventorying storage. **Proposed behavior:** start a session against a target container, scan asset IDs or product barcodes, review duplicates and current placement, then commit accepted moves in bounded transactional batches with session-level undo. **UX:** a full-screen mobile workflow with target summary, live count, vibration/sound feedback, exception queue, and final review.
- **Backend impact:** add scoped scan resolution, idempotent session commands, and batch relocation that honors hierarchy, container-type, and child-location synchronization rules. **Frontend impact:** add camera/keyboard scanner handling and resilient session state. **Database impact:** add short-lived scan sessions and action rows sufficient for audit and conflict-safe undo.
- **Security:** require authentication and collection membership for every resolution; avoid exposing foreign asset-ID existence; rate-limit scans and cap batch size. **Migration:** additive session tables with retention cleanup. **Dependencies:** asset IDs, barcode lookup, container move services, and optimistic concurrency.
- **Effort:** M. **Value:** very high for the fork’s container workflow. **Architectural risk:** low-medium if the existing move service remains the single mutation path. **Fork vs upstream:** implement fork-first, then propose the generic scan-and-relocate core upstream.
- **Scores:** User value **5**; implementation effort **3**; maintenance burden **2**; architectural fit **5**; differentiation **4**; risk **2**.

#### 4. Relocation and Move Plans

- **Problem:** Household moves and room reorganizations span many containers and cannot be safely completed as one untracked batch. **Target user:** families, collectors, and administrators coordinating multi-step moves. **Proposed behavior:** create a plan with origin, destination, assigned items/containers, checklist, and staged status; validate the full hierarchy before finalization and retain an event history. **UX:** a plan board with search-based assignment, printable/QR handoff, progress counts, exception review, and explicit finalize/cancel actions.
- **Backend impact:** add a move-plan state machine and reuse transactional entity-move validation for each bounded finalize step. **Frontend impact:** add plan pages, selection tools, progress state, and mobile scan handoff. **Database impact:** add plans, plan entries, and immutable plan events.
- **Security:** scope plans and referenced entities to one collection; prevent self/descendant moves; record actor identity; require elevated permission for cross-location mass moves. **Migration:** additive tables only. **Dependencies:** scan sessions, hierarchy validation, label generation, and activity events.
- **Effort:** L. **Value:** high for major reorganizations. **Architectural risk:** medium because long-lived plans can conflict with intervening edits. **Fork vs upstream:** fork-first around containers, with a later upstream proposal for generic relocation plans.
- **Scores:** User value **4**; implementation effort **4**; maintenance burden **3**; architectural fit **4**; differentiation **4**; risk **3**.

### 10.3 Power-user features

#### 5. Type-Specific Custom-Field Schemas

- **Problem:** Templates supply custom fields, but collections cannot consistently enforce required fields, ranges, options, units, or uniqueness by entity type. **Target user:** administrators managing structured collections. **Proposed behavior:** define versioned field schemas per entity type, optionally inherited by templates, and enforce them across UI, API, batch creation, CSV import, and restore. **UX:** a schema builder with field preview, validation examples, existing-data impact report, and staged enforcement.
- **Backend impact:** add a reusable schema validator invoked by all entity mutation and import paths. **Frontend impact:** render controls from schema metadata and display the same validation messages before submission. **Database impact:** extend field-definition metadata or add versioned schema records; index values only for explicitly searchable or unique fields.
- **Security:** use bounded declarative validation only; compile patterns with Go’s safe regular-expression engine; cap pattern and option sizes; keep schema changes owner-only. **Migration:** existing values begin grandfathered, followed by an audit-and-fix phase before enforcement. **Dependencies:** Inventory Health Inbox, template defaults, and import validation.
- **Effort:** L. **Value:** very high for reliable structured inventory. **Architectural risk:** medium because validation must remain consistent across every write path. **Fork vs upstream:** propose the base schema contract upstream; retain container/catalog-specific presets in the fork.
- **Scores:** User value **5**; implementation effort **4**; maintenance burden **3**; architectural fit **5**; differentiation **4**; risk **3**.

#### 6. Bulk Edit with Preview and Undo

- **Problem:** Correcting many items is repetitive, while direct mass mutation risks silently damaging good records. **Target user:** power users, import operators, and collection administrators. **Proposed behavior:** select a search result set, define changes, preview exact per-record diffs and failures, apply in bounded transactions, and allow one-time undo while target versions remain unchanged. **UX:** a multi-select toolbar, dry-run summary, downloadable exception list, progress display, and guarded undo.
- **Backend impact:** add dry-run and apply services, optimistic version checks, operation IDs, and reusable batch validation. **Frontend impact:** add selection persistence, diff review, conflict handling, and operation history. **Database impact:** add batch-operation metadata and retained before-state snapshots with strict size and expiry limits.
- **Security:** enforce per-field permissions and collection scope; cap affected rows and retained snapshots; redact secrets and unsafe attachment metadata. **Migration:** additive operation tables and optional entity version columns. **Dependencies:** idempotency/concurrency support, schema validation, and audit logging.
- **Effort:** XL. **Value:** very high once collections grow. **Architectural risk:** high because undo spans heterogeneous relationships and blob references. **Fork vs upstream:** upstream the generic batch contract; keep fork-specific container and custom-field operations as extensions.
- **Scores:** User value **5**; implementation effort **5**; maintenance burden **4**; architectural fit **4**; differentiation **4**; risk **4**.

### 10.4 Mobile improvements

#### 7. Offline Capture Queue

- **Problem:** Basements, garages, and storage units often have poor connectivity, making mobile capture unreliable. **Target user:** mobile inventory operators. **Proposed behavior:** store bounded create/update drafts and photo blobs in IndexedDB, synchronize on reconnect using idempotency and version checks, and surface conflicts for review. **UX:** a connectivity banner, pending count, per-record status, retry/cancel controls, and a clear conflict-resolution screen.
- **Backend impact:** support idempotency keys, conditional writes, resumable or retry-safe uploads, and deterministic conflict responses. **Frontend impact:** add a collection- and user-bound offline queue without restoring private API response caching. **Database impact:** add short-lived idempotency receipts and resource versions; queued drafts remain client-side.
- **Security:** bind queued data to user and collection, clear it on logout or collection removal, enforce browser-storage quotas, and never persist bearer tokens in queue records. **Migration:** additive server receipt/version support; no migration of PWA cache data. **Dependencies:** proposal 21, upload limits, and a carefully versioned client queue format.
- **Effort:** XL. **Value:** very high for the physical environment where Homebox is used. **Architectural risk:** high due to conflict resolution and sensitive local storage. **Fork vs upstream:** best pursued as an upstream PWA initiative with fork workflows layered on top.
- **Scores:** User value **5**; implementation effort **5**; maintenance burden **5**; architectural fit **4**; differentiation **5**; risk **4**.

#### 8. Continuous Mobile Scan Mode

- **Problem:** Reopening the scanner for every item makes audits and relocation sessions unnecessarily slow. **Target user:** mobile users scanning shelves, rooms, or containers. **Proposed behavior:** keep the camera scanner active and offer explicit modes such as locate, count, assign, or move; recognize asset IDs and barcodes and suppress accidental duplicate scans. **UX:** large mode controls, flashlight, haptic feedback, recent-scan strip, clear mutation confirmation, and keyboard-scanner fallback.
- **Backend impact:** add a compact scoped resolution endpoint and bounded batch commands, or efficiently reuse existing endpoints with rate-limit semantics tuned for scan sessions. **Frontend impact:** add continuous camera lifecycle management and accessible non-camera input. **Database impact:** none for read-only locate mode; optional session logs can reuse proposal 3.
- **Security:** process camera frames locally, request permission explicitly, prevent foreign-ID enumeration, and require confirmation before mutation modes. **Migration:** none for the MVP. **Dependencies:** asset-ID uniqueness, barcode integration, and browser camera support.
- **Effort:** M. **Value:** very high for routine inventory work. **Architectural risk:** low-medium. **Fork vs upstream:** upstream the generic scanning shell; keep container assignment modes fork-specific.
- **Scores:** User value **5**; implementation effort **3**; maintenance burden **2**; architectural fit **5**; differentiation **4**; risk **2**.

### 10.5 Reporting and analytics

#### 9. Coverage and Risk Dashboard

- **Problem:** Owners cannot quickly see uninsured value, missing documentation, warranty exposure, or value concentration by location and type. **Target user:** household owners, insurance preparation users, and administrators. **Proposed behavior:** show active-value coverage, insured/uninsured splits, document completeness, expiring warranties, and concentration with drill-down to source records. **UX:** configurable dashboard cards, location/type filters, accessible charts, and formula-safe CSV export.
- **Backend impact:** add aggregate queries aligned with the remediated active-versus-sold statistics contract and existing tenant indexes. **Frontend impact:** add responsive charts and tabular fallbacks. **Database impact:** current-state reporting needs no new tables; optional daily snapshot rows enable trends without replaying mutable history.
- **Security:** collection-scope every aggregate, suppress small-group cross-tenant inference, and exclude secret integration metadata. **Migration:** optional additive snapshot and threshold-setting tables. **Dependencies:** consistent insured, purchase-price, attachment, warranty, and archive semantics.
- **Effort:** M. **Value:** high for insurance and collection stewardship. **Architectural risk:** low-medium. **Fork vs upstream:** suitable for upstream, with container/type breakdowns enabled when those fields exist.
- **Scores:** User value **4**; implementation effort **3**; maintenance burden **3**; architectural fit **5**; differentiation **3**; risk **2**.

#### 10. Lifecycle Cost and Maintenance Forecast

- **Problem:** Existing purchase, warranty, scheduled-maintenance, completed-maintenance, and cost data are useful individually but do not support planning. **Target user:** owners of appliances, tools, vehicles, electronics, or other maintained assets. **Proposed behavior:** calculate upcoming maintenance and warranty events, annual maintenance spend, ownership cost, and deterministic replacement scenarios without opaque AI predictions. **UX:** calendar and timeline views, cost charts, due-soon filters, and item-level explanation of every calculation.
- **Backend impact:** add collection-scoped rollups over existing entity and maintenance-entry data plus a recurrence calculator for explicitly configured schedules. **Frontend impact:** add forecast cards, calendar/table views, and editable assumptions. **Database impact:** add recurrence and reminder metadata only where existing scheduled dates are insufficient.
- **Security:** preserve normal entity visibility; avoid exposing free-form maintenance notes in aggregate exports unintentionally. **Migration:** additive nullable recurrence fields with existing entries unchanged. **Dependencies:** maintenance entries, warranty dates, notification policies, and stable value semantics.
- **Effort:** M. **Value:** high for maintained assets. **Architectural risk:** low-medium if calculations remain deterministic and explainable. **Fork vs upstream:** broadly upstreamable.
- **Scores:** User value **4**; implementation effort **3**; maintenance burden **3**; architectural fit **5**; differentiation **3**; risk **2**.

### 10.6 Automation and integrations

#### 11. Collection-Scoped Rules and Signed Webhooks

- **Problem:** Repetitive tagging, notifications, and external synchronization require manual work or brittle polling. **Target user:** advanced self-hosters and administrators. **Proposed behavior:** define declarative triggers, conditions, and bounded actions for entity changes, moves, maintenance/warranty dates, imports, and container events; deliver retryable HMAC-signed webhooks. **UX:** a rule builder with event samples, dry run, enable/disable, execution history, and redacted delivery diagnostics.
- **Backend impact:** introduce explicit domain events, a persistent transactional outbox, worker leases, delivery retries, and a declarative evaluator; do not rely on the in-memory pub/sub bus for durable delivery. **Frontend impact:** add rule management and run-history pages. **Database impact:** add rules, outbox events, action runs, webhook destinations, and delivery attempts.
- **Security:** reuse hardened outbound-URL validation; block private targets unless explicitly permitted by deployment policy; store secrets write-only; sign payloads; cap retries and payloads; disallow scripts and SQL. **Migration:** additive tables with no rules enabled by default. **Dependencies:** audit events, persistent scheduling, notifier patterns, and secret-management policy.
- **Effort:** XL. **Value:** very high for integrations. **Architectural risk:** high because it introduces durable asynchronous behavior. **Fork vs upstream:** seek an upstream event/outbox contract before cementing fork-only semantics; expose custom container events as namespaced extensions.
- **Scores:** User value **5**; implementation effort **5**; maintenance burden **4**; architectural fit **4**; differentiation **5**; risk **4**.

#### 12. Calendar Feeds and Reminder Policies

- **Problem:** Scheduled maintenance and warranty expiration are easy to overlook outside Homebox. **Target user:** any owner using a calendar or email workflow. **Proposed behavior:** provide revocable per-user WebCal feeds and configurable reminder policies for maintenance, warranty, and loan due dates, with deduplicated notifier delivery. **UX:** reminder settings, feed copy/revoke controls, event preview, and delivery history.
- **Backend impact:** add signed calendar generation, due-event selection, and persistent deduplication for scheduled notifications. **Frontend impact:** add calendar/reminder settings and due-date affordances. **Database impact:** add reminder policies, hashed feed tokens, and delivery receipts.
- **Security:** make tokens revocable and high entropy; avoid logging tokenized URLs; require authentication for management; include only records the user can access. **Migration:** additive tables. **Dependencies:** maintenance/warranty dates, notifier service, and custody due dates if proposal 18 is implemented.
- **Effort:** M. **Value:** medium-high with low workflow disruption. **Architectural risk:** low-medium. **Fork vs upstream:** a strong standalone upstream candidate that avoids provider-specific OAuth initially.
- **Scores:** User value **4**; implementation effort **3**; maintenance burden **3**; architectural fit **5**; differentiation **3**; risk **2**.

### 10.7 Smart-home integrations

#### 13. Read-Only Home Assistant Registry Bridge

- **Problem:** Smart devices are represented separately in Home Assistant and Homebox, so model, location, documentation, and physical-inventory data drift. **Target user:** self-hosters with Home Assistant. **Proposed behavior:** connect a collection to Home Assistant, search its device/entity registry, associate a Homebox entity with a device, display safe live metadata and a deep link, and refresh mappings without controlling devices. **UX:** owner integration setup, item-level association, connection test, stale-link warning, and explicit unlink.
- **Backend impact:** add a guarded Home Assistant client, write-only token handling, registry synchronization, and mapping service. **Frontend impact:** add integration settings and an item summary panel. **Database impact:** add collection/entity-to-external-device mappings and last-seen metadata; reuse integration configuration storage where appropriate.
- **Security:** owner-only configuration; reuse SSRF, scheme, host, and URL-userinfo protections; never return tokens; keep the first version read-only and bounded. **Migration:** additive mapping table. **Dependencies:** collection-scoped integration resolution and secret-redaction patterns.
- **Effort:** L. **Value:** high for the smart-home segment but narrower overall. **Architectural risk:** medium. **Fork vs upstream:** best as an optional integration package or fork feature until upstream accepts a provider interface.
- **Scores:** User value **4**; implementation effort **4**; maintenance burden **3**; architectural fit **4**; differentiation **5**; risk **3**.

#### 14. NFC Location Checkpoints

- **Problem:** QR scanning can be awkward in dark or tightly packed storage areas, and users need a fast physical entry point into location/container workflows. **Target user:** mobile users and smart-home enthusiasts. **Proposed behavior:** issue a revocable opaque checkpoint for a location or container; tapping a programmed NFC tag opens the authenticated PWA at that target and can start an explicit scan or move session. **UX:** “Program NFC tag” where Web NFC is available, printable QR fallback, tag status, and a confirmation screen before mutations.
- **Backend impact:** add checkpoint creation, token resolution, revocation, and optional rule-event emission. **Frontend impact:** add tag programming, fallback QR, and deep-link routing. **Database impact:** add checkpoint records with collection, target, hashed token, creation actor, and revoked timestamp.
- **Security:** the token identifies a target but never bypasses authentication or authorization; use high-entropy revocable tokens, rate-limit resolution, and prevent replay from directly mutating data. **Migration:** additive table. **Dependencies:** mobile deep links, scan sessions, and browser-specific Web NFC capability detection.
- **Effort:** M. **Value:** medium-high for physical workflows. **Architectural risk:** low-medium. **Fork vs upstream:** upstream the safe checkpoint/deep-link model; retain container-specific actions in the fork.
- **Scores:** User value **4**; implementation effort **3**; maintenance burden **2**; architectural fit **4**; differentiation **4**; risk **2**.

### 10.8 Data-quality features

#### 15. Duplicate Detection and Transactional Merge

- **Problem:** Manual entry, CSV imports, AI suggestions, and catalog workflows can create near-duplicate entities with different attachments or metadata. **Target user:** owners and import operators. **Proposed behavior:** generate explainable duplicate candidates using normalized name, manufacturer, model, serial, barcode, and location evidence; merge a selected pair while preserving chosen fields, tags, maintenance, attachments, and references. **UX:** side-by-side comparison, match reasons, per-field selection, merge preview, and conflict-safe undo.
- **Backend impact:** add normalized matching and a transactional merge service that preserves blob reference counts, asset IDs, hierarchy, and collection scope. **Frontend impact:** add a duplicate-review queue and merge editor. **Database impact:** add optional normalized fingerprints/indexes and retained merge-operation metadata.
- **Security:** never compare across collections; restrict merge permission; cap candidate generation; sanitize compared text; redact attachment URLs where required. **Migration:** additive indexes/operation rows with no automatic merging. **Dependencies:** robust transactions, search normalization, audit logging, and optimistic versions.
- **Effort:** L. **Value:** very high for long-lived collections. **Architectural risk:** medium because merges touch many relationships. **Fork vs upstream:** strongly upstreamable, with fork-specific type/container rules supplied as validation hooks.
- **Scores:** User value **5**; implementation effort **4**; maintenance burden **3**; architectural fit **5**; differentiation **4**; risk **3**.

#### 16. Declarative Data-Quality Policies

- **Problem:** Different collections have different completeness expectations, and hard-coded findings alone cannot express them. **Target user:** collection owners and data stewards. **Proposed behavior:** configure bounded policies such as “insured items require a receipt,” “serial numbers are unique within type,” “containers require capacity,” or “items in this location need a photo”; evaluate existing data and preview safe repairs. **UX:** policy templates, a simple condition builder, affected-record preview, severity, and enable/disable controls integrated with the Health Inbox.
- **Backend impact:** add a non-executable policy evaluator shared by audits and mutation-time warnings. **Frontend impact:** add policy management and finding filters. **Database impact:** add policy definitions and optional materialized finding state for expensive rules.
- **Security:** no user-authored code, SQL, or network calls; validate field references and cap rule complexity; owner-only configuration. **Migration:** additive tables with a small set of disabled example policies. **Dependencies:** proposal 1 and, for typed-field policies, proposal 5.
- **Effort:** M. **Value:** high for disciplined collections. **Architectural risk:** low-medium if the language stays declarative. **Fork vs upstream:** upstream the policy framework and keep catalog/container templates optional.
- **Scores:** User value **4**; implementation effort **3**; maintenance burden **3**; architectural fit **5**; differentiation **3**; risk **2**.

### 10.9 Collaboration features

#### 17. Activity, Comments, Mentions, and Tasks

- **Problem:** Questions and handoffs about an item or location happen outside the inventory record. **Target user:** households, teams, and volunteers sharing a collection. **Proposed behavior:** provide user-facing activity, comments, mentions, checklists, and resolvable tasks on entities, locations, move plans, and maintenance work. **UX:** an Activity tab, collection inbox, @member picker, task status, and notification preferences.
- **Backend impact:** add collaboration events, comments/tasks services, mention resolution, and durable notification delivery. **Frontend impact:** add activity streams, composer controls, and inbox surfaces. **Database impact:** add activity entries, comments, tasks, mentions, and notification receipts.
- **Security:** collection-scope all references; sanitize rendered content; rate-limit comments; define edit/delete windows; prevent mentions of users outside the collection. **Migration:** additive tables. **Dependencies:** a durable event/outbox foundation and member lifecycle hooks.
- **Effort:** L. **Value:** high for multi-user installations. **Architectural risk:** medium because notification and retention semantics become permanent product behavior. **Fork vs upstream:** broadly useful upstream, with custom workflow events added by the fork.
- **Scores:** User value **4**; implementation effort **4**; maintenance burden **4**; architectural fit **4**; differentiation **4**; risk **3**.

#### 18. Custody and Loans

- **Problem:** Borrowed or lent items are often represented only in notes, so current custodian, due date, and return history are unclear. **Target user:** families, workshops, clubs, and shared households. **Proposed behavior:** check an item out to a collection member or minimal free-form contact, record due date and condition, send reminders, and retain return history while keeping ownership/value semantics explicit. **UX:** Check out/Return actions, item badge, overdue view, condition/photos, and borrower history.
- **Backend impact:** add an atomic custody service and define how active, available, archived, and sold statistics treat checked-out items. **Frontend impact:** add loan forms, badges, filters, and dashboard card. **Database impact:** add loans and loan events, with optional condition-attachment references.
- **Security:** minimize contact data, restrict borrower history appropriately, keep public sharing out of the first version, and preserve collection isolation. **Migration:** additive tables only. **Dependencies:** reminder policies, attachments, and audit/activity events.
- **Effort:** M. **Value:** high for shared assets. **Architectural risk:** low-medium. **Fork vs upstream:** a strong upstream feature with little fork-specific coupling.
- **Scores:** User value **4**; implementation effort **3**; maintenance burden **3**; architectural fit **5**; differentiation **4**; risk **2**.

### 10.10 Administrative features

#### 19. Durable Security Audit Log

- **Problem:** Security-sensitive changes lack a single durable, searchable record of actor, target, outcome, and redacted change. **Target user:** collection owners and operators of shared deployments. **Proposed behavior:** record authentication, membership, API-key, integration, import/export, destructive, and bulk-operation events with request IDs and bounded redacted diffs. **UX:** owner-only audit page with filters, event detail, retention settings, and safe export.
- **Backend impact:** add an audit service invoked at transaction boundaries and middleware for request outcome context. **Frontend impact:** add administration views and actor/target navigation. **Database impact:** add append-only audit events with collection/time/actor/action indexes and optional hash chaining.
- **Security:** redact secrets, URLs with userinfo, credentials, and sensitive request bodies; make audit reads owner-only; prevent normal repositories from updating events. **Migration:** forward-only additive table; history begins at deployment. **Dependencies:** centralized authorization and request IDs.
- **Effort:** L. **Value:** very high for accountability and future permissions. **Architectural risk:** medium because incomplete coverage creates false confidence. **Fork vs upstream:** upstream-first foundation, with fork actions registered through stable event names.
- **Scores:** User value **5**; implementation effort **4**; maintenance burden **4**; architectural fit **5**; differentiation **3**; risk **3**.

#### 20. Granular Collection Roles

- **Problem:** Owner/member is too coarse: ordinary editors need not manage integrations, members, backups, or bulk deletion. **Target user:** shared households, clubs, and organizational deployments. **Proposed behavior:** begin with fixed roles—viewer, editor, inventory manager, integration administrator, and owner—then align API-key scopes to the same permission vocabulary. **UX:** a clear role matrix, member assignment, permission explanations, and last-owner protection.
- **Backend impact:** replace scattered owner checks with a centralized default-deny authorization policy used by routes, services, background jobs, and generated API documentation. **Frontend impact:** consume capability flags rather than duplicating role-name logic. **Database impact:** add role/permission membership data and API-key scopes.
- **Security:** preserve last-owner invariants, test every route against all roles, deny unknown permissions, and avoid per-location ACLs in the initial version. **Migration:** owners map to owner; members map to editor, preserving current behavior. **Dependencies:** proposal 19, comprehensive authorization tests, and invitation lifecycle updates.
- **Effort:** XL. **Value:** very high for shared deployments. **Architectural risk:** high because authorization is cross-cutting. **Fork vs upstream:** requires an upstream RFC to avoid a permanently divergent permission model.
- **Scores:** User value **5**; implementation effort **5**; maintenance burden **5**; architectural fit **4**; differentiation **4**; risk **4**.

### 10.11 Developer and API improvements

#### 21. Idempotent Mutations and Optimistic Concurrency

- **Problem:** Mobile retries, batch workflows, and external clients can duplicate creates or overwrite newer edits. **Target user:** all users indirectly, plus integration developers. **Proposed behavior:** accept `Idempotency-Key` on create/import/batch mutations and expose entity versions or ETags for `If-Match` conflict detection. **UX:** transparent during normal use, with precise conflict dialogs showing that data changed rather than generic failures.
- **Backend impact:** add idempotency middleware, request-hash validation, bounded response receipts, and repository version checks inside transactions on SQLite and PostgreSQL. **Frontend impact:** send keys for retryable operations and preserve ETags/version values. **Database impact:** add scoped idempotency receipts with expiry and version columns on mutable resources.
- **Security:** scope keys to principal, collection, route, and request hash; never replay a key with different input; cap retention/body size and exclude secret-bearing responses. **Migration:** additive receipts and version defaults. **Dependencies:** stable error contracts and cleanup scheduling.
- **Effort:** L. **Value:** foundationally high for offline, automation, and bulk workflows. **Architectural risk:** medium. **Fork vs upstream:** pursue upstream first because client and API semantics should remain compatible.
- **Scores:** User value **5**; implementation effort **4**; maintenance burden **3**; architectural fit **5**; differentiation **3**; risk **3**.

#### 22. Versioned SDK and Event Test Console

- **Problem:** Integration authors must infer behavior from generated contracts and cannot safely inspect scoped API or event payloads. **Target user:** self-hosters, plugin authors, and contributors. **Proposed behavior:** publish a versioned OpenAPI contract, generated TypeScript and Python clients, contract tests, and an owner-only console for redacted request/event samples and webhook simulations. **UX:** developer settings with one-time API-key display, scope selection, copyable examples, and side-effect-free simulation.
- **Backend impact:** tighten OpenAPI/error schemas, add API-key scopes and event simulation endpoints, and validate SDKs in CI. **Frontend impact:** add the developer console. **Database impact:** extend API keys with scopes and optionally retain short-lived redacted samples.
- **Security:** never proxy arbitrary URLs; show keys once; rate-limit the console; redact secrets and personal fields; simulation must not execute real mutations. **Migration:** additive API-key scope fields with existing keys mapped conservatively. **Dependencies:** stable authorization vocabulary, proposal 11’s event schema, and audit logging.
- **Effort:** M. **Value:** high for ecosystem growth. **Architectural risk:** low-medium if generated artifacts remain CI-controlled. **Fork vs upstream:** upstream generator and console foundations; namespace fork-only endpoints and event types.
- **Scores:** User value **4**; implementation effort **3**; maintenance burden **3**; architectural fit **4**; differentiation **4**; risk **2**.

### 10.12 Reliability and backup features

#### 23. Encrypted Scheduled Backups with Restore Drills

- **Problem:** Manual exports do not prove that recent, complete backups exist or that they can actually be restored. **Target user:** every production operator. **Proposed behavior:** schedule complete collection archives to local, S3-compatible, or WebDAV storage; apply envelope encryption, retention, checksums, and periodic non-destructive restore validation in an isolated scratch database/storage namespace. **UX:** backup-health page with last success, next run, checksum, destination status, restore-drill result, and explicit recovery-key warnings.
- **Backend impact:** add durable scheduling/leases, streaming export delivery, destination adapters, encryption, retention cleanup, and scratch restore validation using existing export/import rules. **Frontend impact:** add owner-only backup configuration and run history. **Database impact:** add schedules, runs, destination credential references, checksums, and validation results.
- **Security:** keep the master/recovery key outside the database; treat destination credentials as write-only; reuse SSRF/path protections; never offer automatic destructive live restore. **Migration:** additive metadata tables. **Dependencies:** persistent jobs, external secret policy, storage-capacity checks, and the remediated archive service.
- **Effort:** XL. **Value:** critical for production resilience. **Architectural risk:** high due to encryption, remote storage, and scratch-database lifecycle. **Fork vs upstream:** highly suitable for upstream, with deployment-specific adapters remaining optional.
- **Scores:** User value **5**; implementation effort **5**; maintenance burden **5**; architectural fit **5**; differentiation **4**; risk **4**.

#### 24. Soft Delete and Change Journal

- **Problem:** Accidental deletion currently requires a full backup restore, which is disproportionate and can overwrite unrelated newer work. **Target user:** all users, especially shared collections. **Proposed behavior:** tombstone entities, locations, templates, tags, and attachments into a time-limited trash view; retain blobs until final purge; restore valid graphs with conflict review; keep a bounded change journal for supported reversals. **UX:** impact preview, Trash page, restore/conflict flow, retention countdown, and owner-only purge.
- **Backend impact:** make delete semantics repository-wide, filter tombstones consistently, implement hierarchy-aware restore and blob garbage collection, and record reversible state. **Frontend impact:** add trash filters, restore dialogs, and hidden-record handling. **Database impact:** add deletion actor/time/version fields, indexes, journal snapshots, and retention metadata.
- **Security:** tombstoned records remain fully authorization-protected; purge requires elevated permission and revalidation; restore cannot recreate cross-collection or cyclic references. **Migration:** additive fields defaulting to active, followed by query-by-query adoption. **Dependencies:** audit logging, optimistic concurrency, and robust blob reference accounting.
- **Effort:** XL. **Value:** very high. **Architectural risk:** very high because deletion behavior touches nearly every query and relationship. **Fork vs upstream:** requires an upstream design proposal or a deliberately isolated fork implementation to avoid long-term merge pain.
- **Scores:** User value **5**; implementation effort **5**; maintenance burden **5**; architectural fit **4**; differentiation **4**; risk **5**.

### Top five quick wins

1. **Inventory Health Inbox (#1):** highest immediate value with a read-mostly, collection-scoped first slice.
2. **Saved Views (#2):** small implementation that improves every repeated inventory workflow.
3. **Continuous Mobile Scan Mode (#8):** leverages asset IDs and existing barcode behavior without requiring offline synchronization.
4. **Calendar Feeds and Reminder Policies (#12):** exposes existing maintenance and warranty value through a familiar external workflow.
5. **Declarative Data-Quality Policies (#16):** start with a few bounded templates after the Health Inbox establishes the finding model.

### Top five strategic features

1. **Idempotent Mutations and Optimistic Concurrency (#21):** foundation for safe offline, retry, batch, and integration work.
2. **Durable Security Audit Log (#19):** foundation for permissions, automation accountability, and operational investigation.
3. **Encrypted Scheduled Backups with Restore Drills (#23):** strongest production-reliability improvement.
4. **Offline Capture Queue (#7):** a differentiated mobile capability that directly matches real storage environments.
5. **Collection-Scoped Rules and Signed Webhooks (#11):** the main ecosystem and automation multiplier once durable events exist.

### Features not to build

These are guardrails, not additional proposals:

- Do not build passive always-on BLE room presence. Accuracy, battery use, spoofing, and household privacy costs exceed its likely value; prefer explicit NFC or QR checkpoints.
- Do not allow arbitrary SQL, shell commands, JavaScript, or templates with unrestricted network access in automation rules.
- Do not let AI, barcode feeds, or scheduled imports commit inventory changes without preview, bounded validation, and an accountable actor.
- Do not begin Home Assistant support with bidirectional device control or automatic location mutation; prove a read-only registry bridge first.
- Do not add arbitrary per-location ACLs in the first granular-role release; the combinatorial authorization surface would be disproportionate.
- Do not provide unauthenticated public attachment or inventory shares until a separately reviewed capability-token and redaction design exists.
- Do not expose one-click destructive restore-in-place. Restore should be verified in isolation and require a deliberate recovery procedure.

### Recommended implementation order

1. Establish product value quickly with **Inventory Health Inbox (#1)** and **Saved Views (#2)**.
2. Add mutation/accountability foundations: **Idempotency and Concurrency (#21)**, then **Audit Log (#19)**.
3. Improve physical workflows: **Continuous Scan (#8)**, **Scan-to-Tote (#3)**, **Custody and Loans (#18)**, then **Move Plans (#4)**.
4. Build the offline queue **(#7)** only after scan commands, idempotency, and conflict semantics are stable.
5. Improve data discipline: **Data-Quality Policies (#16)**, **Field Schemas (#5)**, **Duplicate Merge (#15)**, then **Bulk Edit (#6)**.
6. Add resilience: **Scheduled Backup/Restore Drills (#23)** before the more invasive **Soft Delete/Change Journal (#24)**.
7. Expand governance and collaboration: **Granular Roles (#20)**, then **Activity/Comments/Tasks (#17)**.
8. Establish integration surfaces: **Rules/Webhooks (#11)**, **Calendar Feeds (#12)**, **Home Assistant (#13)**, **NFC Checkpoints (#14)**, and **SDK/Test Console (#22)**.
9. Add decision support once source data is healthier: **Coverage Dashboard (#9)** and **Lifecycle Forecast (#10)**.

### Suggested first feature for a separate cycle

Build **Inventory Health Inbox (#1)** first.

The first slice should remain intentionally bounded: fixed collection-scoped checks, read-only drill-downs, no background job, no automatic repair, and no configurable policy language. Start with missing location, missing photo, missing value for insured items, overdue maintenance, expiring warranty, and duplicate non-empty serial numbers. Validate identical results on SQLite and PostgreSQL, confirm owner/member behavior, and ensure every result links to the exact filtered records that produced it. This creates immediate user value while establishing the finding contract later needed by data-quality policies, reporting, and safer bulk remediation.
