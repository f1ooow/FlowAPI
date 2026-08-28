# Implementation Plan

## Implementation Status (2026-08-28)

Implementation is complete across persistence, migration, runtime billing, retry reservation, asynchronous snapshots, authenticated ratio ranges, administrator UI, API Key group display, i18n, and upstream ratio-sync removal.

Verified in this checkout:

- Focused and full Go test suites, billing expression tests, and channel-cache race tests pass.
- Frontend typecheck, touched-file lint, focused Vitest suites, i18n sync, and production build pass.
- A fresh SQLite installation and real browser flow verified channel cost-ratio configuration, group include toggles, authenticated final-ratio range display, unavailable groups, anonymous non-disclosure, mobile layout, and removed/preserved endpoint boundaries.
- MySQL and PostgreSQL live migrations were not available; the change uses GORM `AutoMigrate`, nullable scalar storage, and dialect-neutral option transactions.

Independent `trellis-check` is complete; evidence is in `check-report.md`. Pending workflow steps are optional spec capture, explicit staging/commit authorization, and archive.

## Phase 0 - Safety Baseline

- [ ] Re-read `prd.md`, `design.md`, task research, relevant Trellis specs, root `AGENTS.md`, and `web/AGENTS.md` before editing.
- [ ] Run `git status --short` and record the task-owned file allowlist; preserve all unrelated dirty-worktree changes.
- [ ] Load the project `i18n-translate` skill before adding or changing frontend text.
- [ ] Run the existing focused ratio, billing, channel, task billing, and API Key group selector tests as a baseline where the current dirty checkout permits.

## Phase 1 - Persisted Configuration And Migration

- [ ] Add nullable channel cost ratio storage, `GetCostRatio()` normalization, a shared maximum bound, and validation without a GORM business-default tag.
- [ ] Include the new channel field in create/update DTOs, frontend schemas, form defaults, payload transforms, root-only authorization classification, and management audit output.
- [ ] Extend group ratio configuration with `user_group_ratio` and `include_channel_ratio` maps plus safe default accessors.
- [ ] Implement legacy `GroupGroupRatio` migration with atomic persistence, version marker, idempotence, missing-group fallback, zero/zero handling, and structured zero-base conflict reporting.
- [ ] Preserve legacy semantics until migration completes; never partially activate new semantics.
- [ ] Add deterministic migration and validation tests using `require`/`assert`.

## Phase 2 - Unified Runtime Multiplier

- [ ] Introduce a named runtime billing ratio state containing base group, user group, channel, include flag, and effective ratio.
- [ ] Centralize multiplier resolution; remove direct “special ratio replaces group ratio” behavior.
- [ ] Propagate channel cost ratio through selected-channel context and `ChannelMeta` for every relay format.
- [ ] Update ordinary ratio, fixed-price, tiered expression, per-call, Midjourney, audio/WSS, and task paths to consume the shared effective ratio.
- [ ] Preserve safe quota conversion and saturation audit markers at every calculation boundary.
- [ ] Extend admin-only billing log data with component and effective ratios; ensure user log sanitization still strips these fields.

## Phase 3 - Retry, Reservation, And Async Consistency

- [ ] Replace tiered-only route refresh with a unified prepare-selected-route billing step.
- [ ] On every retry, recompute target pre-consume after group/channel selection and reserve any positive delta before sending upstream.
- [ ] Verify cheaper retries refund through final settlement and more expensive retries do not send before reservation is updated.
- [ ] Extend `TaskBillingContext` with all route-dependent multiplier components and effective ratio.
- [ ] Make asynchronous token recalculation, polling settlement, refund, remix/origin-task flows, and historical task fallback use the persisted snapshot rather than current settings.
- [ ] Add regression tests for specified channel, affinity channel, auto-group switch, cross-channel retry, fixed price, model ratio, tiered expression, per-call task, and old task snapshots.

## Phase 4 - Group Range API

- [ ] Build enabled-channel group ratio summaries within the channel cache lifecycle, supporting comma-separated channel groups and nil-as-one normalization.
- [ ] Return structured single/range/auto/unavailable data from the authenticated self-group endpoint using the current user's user-group multiplier.
- [ ] Keep anonymous group responses free of range and raw cost data.
- [ ] Add backend tests for disabled channels, multiple groups, equal min/max, no channels, user-group-specific multipliers, auto, cache rebuilds, and anonymous non-disclosure.

## Phase 5 - Administrator UI

- [ ] Add the channel cost ratio input to the existing channel editor without adding a standalone page or duplicate display.
- [ ] Add “Include channel ratio” to each row in the existing group pricing editor and serialize it to the new setting.
- [ ] Rename special ratio rules to user group ratios and update rule summaries to reflect multiplier semantics.
- [ ] Surface migration conflicts to root users in the group settings workflow and prevent misleading saves until conflicts are resolved.
- [ ] Add/update component tests for defaults, validation, payloads, permissions, migration conflict state, and group setting serialization.

## Phase 6 - User Group Selection UI

- [ ] Update the self-group API type to a discriminated structured display contract.
- [ ] Extend the API Key group combobox, ratio badge, search, selected trigger, auto-order display, and API Key table cell for single/range/auto/unavailable states.
- [ ] Show only final values; do not add factor labels, breakdown tooltips, or duplicate explanatory UI.
- [ ] Format ranges with bounded precision and no layout shift or overflow on mobile/desktop.
- [ ] Add tests for single value, range, equal bounds, unavailable, auto, user-specific value, and absence of component labels.
- [ ] Complete all supported locale translations and run `bun run i18n:sync` according to the project skill.

## Phase 7 - Remove Upstream Ratio Sync

- [ ] Remove `/api/ratio_sync/*` registration, controller implementation, dedicated tests, sync-only DTOs, and unused helpers after reference checks.
- [ ] Remove the frontend upstream-sync tab and all sync-only components, APIs, helpers, constants, and types.
- [ ] Remove matching OpenAPI paths and stale documentation while preserving `/api/ratio_config`.
- [ ] Verify channel upstream model update endpoints and UI remain available.
- [ ] Run reference searches proving no stale `ratio_sync`, `FetchUpstreamRatios`, or `UpstreamRatioSync` symbols remain outside intentional release notes/history.

## Phase 8 - Verification

- [ ] Run `gofmt` on task-owned Go files.
- [ ] Run focused Go tests for `setting/ratio_setting`, `model`, `controller`, `relay/helper`, `service`, `middleware`, and billing expression integration.
- [ ] Run broader `go test ./...` if the dirty checkout allows; otherwise report unrelated blockers precisely.
- [ ] Run `go test -race` on the channel cache/range packages where practical.
- [ ] Run SQLite real `AutoMigrate` and option migration tests.
- [ ] Verify MySQL/PostgreSQL compatibility through available CI/dialect fixtures; record whether live instances were unavailable.
- [ ] From `web/`, run focused Vitest suites, `bun run typecheck`, touched-file lint/format checks, `bun run i18n:sync`, and `bun run build`.
- [ ] Start the local application and validate with real browser paths: channel edit, group settings, VIP/default API Key group selection, range update after channel status/ratio change, and anonymous non-disclosure.
- [ ] Check browser console/network errors and responsive layouts at desktop and mobile widths.
- [ ] Run `git diff --check` and review only the explicit task-owned diff.

## Phase 9 - Review And Closeout

- [ ] Run `trellis-check` against PRD/design acceptance criteria and resolve all material findings.
- [ ] Review whether the new billing/migration invariants should be captured through `trellis-update-spec`.
- [ ] Prepare release notes for the removed root API and migration behavior.
- [ ] Stage only the task-owned allowlist. Commit and archive require separate user approval under the Trellis workflow.

## Primary Risk And Rollback Points

- **Migration:** never write the version marker if conversion conflicts or persistence fails.
- **Retry billing:** reservation must be updated before each upstream attempt; this is the highest financial-risk boundary.
- **Async tasks:** snapshot compatibility must be in place before enabling the new multiplier in production.
- **Privacy:** range data must remain authenticated and must not expose raw channel ratios.
- **Dirty worktree:** no broad staging, deletion, or cleanup commands.

## Planned Validation Commands

```bash
go test ./setting/ratio_setting ./model ./controller ./relay/helper ./service ./middleware
go test ./pkg/billingexpr/...
go test ./...
git diff --check
```

```bash
cd web
bun run test -- <focused test files>
bun run typecheck
bun run lint
bun run i18n:sync
bun run build
```
