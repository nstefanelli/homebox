# Label Sheet PDF Generation — Design

**Date:** 2026-07-21
**Status:** Approved (Nick, 2026-07-21)
**Driver:** WebKit prints CSS inches at 1.0667 physical inches (legacy 1.25 shrink factor in `PrintContext.h`; Blink fixed theirs to 1.333 years ago) and ignores `@page size` — no Safari HTML print path can ever be 1:1, and Safari's "Save as PDF" inherits the same error. Industry standard (Avery, Pirate Ship, Shopify) is a real PDF printed at Actual Size. A client-side PDF also unlocks iPhone/iPad printing via AirPrint.

## Goal

Generate the label sheet as a real PDF with exact point geometry (Letter = 612×792pt), client-side, from the same state the verified HTML sheet renders — making label printing browser-independent and mobile-capable. The Chromium HTML print path stays as-is.

## Design

### New module: `frontend/lib/labels/pdf.ts`

Built on **pdf-lib** + **@pdf-lib/fontkit** with one bundled OFL-licensed TTF (a Noto Sans subset or similar, kept small) — pdf-lib's built-in fonts are WinAnsi-only and would throw on non-Latin names (Homebox ships ~20 locales). No silent fallback to a Latin-1 font: if the font asset fails to load, surface an error, don't emit a PDF that breaks names.

Geometry: preset inches × 72 = points, driven by the SAME modules the HTML path uses — `presets.ts`, `grid.ts`, the `calcPages` output (pages/cells/skip positions), and the clamped `printOffset` (which becomes a plain coordinate translation in PDF space — no shrink-to-fit exists inside a PDF, so no padding-delta trick needed).

Content parity with the HTML sheet, per cell:
- QR: fetch the same authenticated same-origin `/api/v1/qrcode?data=...` URLs (cookie auth carries over), dedupe identical URLs, sniff bytes (JPEG today, don't hard-code), `embedJpg`/`embedPng` at the same size ratio as the HTML `qrSize` computed.
- Asset-ID bold row with the omit-when-empty rule (never `000-000`).
- Auto-fit name row: reuse `fitFontSize` from `fit.ts` verbatim with an injected pdf-lib measurer (`font.widthOfTextAtSize`) — same floor, same stepping, same wrap model; clamp to 2 lines with a manual ellipsis (PDF has no line-clamp).
- HomeBox/blank-write-in line behaviors and the location row, matching the HTML template's conditional logic.
- Bordered mode draws the cell rectangle at the true label edge.
- The 0.1in print-safe inset applies identically.

### UI

A "Download PDF" button on the label-generator page next to Print. For non-Chromium user agents, the page hints that PDF is the accurate path (Safari's engine cannot print HTML at true scale). Filename `labels-<preset>-<yyyy-mm-dd>.pdf`. Include a short printed-guidance line in the UI copy: print the PDF at 100% / Actual Size (macOS: Preview; iOS: AirPrint panel, Paper US Letter, no fit-to-page).

## Acceptance criteria

1. PDF page size is exactly the preset's page dimensions in points (612×792 for the Letter presets), every page.
2. Cell origins and sizes in the generated PDF match the Avery preset math within 0.25pt, all four presets, multi-page, including skip positions and non-zero offsets (verified by rasterizing the PDF and measuring, as the harness does for the HTML path).
3. Auto-fit parity: for the same inputs, the PDF name row uses the same font-size decisions as the HTML path (same `fitFontSize`, measurer-injected), and long names clamp at 2 lines with ellipsis without escaping the cell.
4. QRs embed from the authenticated endpoint and remain scannable after rasterization at 300dpi.
5. A non-Latin (e.g. CJK or accented) item name renders in the PDF without throwing.
6. The HTML print path, all existing label tests (23), and cell geometry are untouched; new unit tests cover the pt-conversion math and the pdf measurer adapter.

## Out of scope

- Server-side generation; `backend/pkgs/labelmaker` (per-label PNG rasterizer for the dedicated-printer feature — wrong foundation, untouched).
- Removing or altering the Chromium HTML print path.
- The `/a/000-000` QR-collision fix for asset-ID-less items (separate task, running in its own worktree).
