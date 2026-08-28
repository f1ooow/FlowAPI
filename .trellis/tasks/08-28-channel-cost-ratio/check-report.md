# Check Report

## Outcome

Independent Trellis review passed for the channel cost-ratio task. Implementation, privacy review, and release checks are complete; the task is ready for commit, HK deployment, and archive.

## Acceptance Review

- AC1: `/api/ratio_sync/*` backend and frontend removed; `/api/ratio_config` and `/api/channel/upstream_updates/*` preserved.
- AC2: nullable channel `cost_ratio` defaults to `1`, validates `0 < ratio <= 1000`, is root-only, audited, and covered by backend/frontend tests.
- AC3: group pricing persists `include_channel_ratio`; existing groups default false.
- AC4-AC5: user-group multipliers are independent factors and the shared resolver computes group x user x optional channel.
- AC6: synchronous retries refresh the route multiplier and reserve a higher target before the upstream attempt.
- AC7: asynchronous tasks persist effective and component ratio snapshots; token recalculation uses the snapshot.
- AC8-AC9: authenticated API Key group selection supports single/range/auto/unavailable and shows only final values; anonymous groups expose no range fields.
- AC10: legacy absolute ratios migrate atomically and idempotently; zero-base conflicts block activation and keep legacy semantics.
- AC11: billing components are nested under `other.admin_info.billing_ratios`; existing non-admin log formatting strips `admin_info`.
- AC12: SQLite real migration passed. No live MySQL/PostgreSQL DSNs were available; the implementation uses GORM nullable scalar migration and dialect-neutral option transactions.
- AC13: User-side privacy was tightened after review: historical factor keys are scrubbed from user logs, anonymous groups omit base ratios, and pricing catalog values are composed final display values rather than pre-channel bases.

## Checks Passed

- `go test -count=1 ./...`
- `go test -race -count=1 ./model`
- `cd relaykit && GOWORK=off go build ./...`
- Frontend `bun run typecheck`
- Task-focused Vitest: 4 files, 17 tests
- Task-touched `oxlint`: zero findings
- Task-touched `oxfmt --check`: passed
- `bun run i18n:sync`: current `zh` locale reports missing=0, extras=0, untranslated=0
- `bun run build`
- `jq` validation for OpenAPI and locale JSON
- `git diff --check`
- Trellis context validation: 7 implement entries and 6 check entries

## Browser Verification

Used a separate temporary SQLite database and local servers, without touching the user's configured database.

- Completed first-run setup and root login.
- Created enabled premium channels with cost ratios `0.8` and `1.5`.
- Verified the channel editor exposes a root-editable cost-ratio control with correct value and layout.
- Verified the group pricing table saves the include-channel-ratio checkbox.
- Verified authenticated premium display changes between single `1.6x` and range `1.28x-2.4x` when the checkbox is toggled.
- Verified API Key selection shows final range, Auto, and unavailable states on desktop and 390x844 mobile layouts without overlap.
- Verified anonymous premium response contains only `desc`; no ratio or internal factor is exposed.
- Verified removed ratio-sync route returns 404, ratio-config remains registered (403 while exposure is disabled), and upstream model-update POST remains registered.

## Residual Baseline

- Full frontend Vitest: 30/32 files and 143/151 tests pass. The remaining eight failures predate this task: broken test `localStorage` wiring affects API Key and redemption drawers, plus one existing Auto-order button-state assertion.
- Full copyright check still reports five pre-existing files outside the task allowlist; both new task TypeScript files have valid headers.
- Browser console reports an existing nested `div` inside `FormDescription` in the channel credential section. It is unrelated to the cost-ratio control.
- No paid upstream provider request was made.
