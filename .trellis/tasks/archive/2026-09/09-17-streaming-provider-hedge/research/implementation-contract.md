# Implementation Contract

User approved implementation on 2026-09-17. This file coordinates backend and frontend ownership, not new product scope.

## Channel JSON

Under the existing `settings.reliability` object:

- `first_content_timeout_seconds`: optional integer pointer; absent or zero disables hedge; positive 1-180.
- `streaming_idle_timeout_seconds`: optional integer pointer; absent or zero disables this channel's scanner timer; positive 60-600.
- `non_streaming_timeout_seconds`: optional integer pointer; absent or zero disables this added per-attempt timer; positive 60-1800. Independent transport/client safety limits may still apply.

Latest user instruction: all three fields display/save zero by default; no blank/inheritance UI. Only manually configured positive durations enable the timers.

Backend Go field names: `FirstContentTimeoutSeconds`, `StreamingIdleTimeoutSeconds`, `NonStreamingTimeoutSeconds`, all `*int` with omitempty. Preserve explicit zero and existing reliability defaults/unknown settings round-trip.

## Ownership

- Backend implementer owns runtime relay/coordinator/billing/DTO/model changes and focused Go regression tests.
- Frontend implementer owns `web/src/features/channels`, `web/src/features/usage-logs`, new translations/static keys, frontend tests and verification. Backend supplies the usage-log metadata contract before log UI integration.
- Root coordinates contracts, reviews integration, verification, documentation and final artifact updates. Existing unrelated worktree changes remain untouched.

## Global Loser Billing

Latest user instruction adds `general_setting.bill_hedge_losers` (boolean, default true), shown in routing reliability system settings. Backend field: `GeneralSetting.BillHedgeLosers`. Capture it per request. Enabled: only losers with an already valid protocol response prefix when the winner is selected can drain and be charged. Headers/heartbeats alone do not qualify. Disabled: cancel/refund all losers, charge only winner. Preserve explicit false through load/save. This policy supersedes earlier unconditional loser retention.

## Accounting

Every billable racing attempt preconsumes independently and settles once. Effective group ratio comes from existing `HandleGroupRatio`; channel ratio participates only when that attempt's resolved group enables it. Loser usage collection is bounded and never writes to the client. Unknown interrupted loser usage is not fabricated into a charge. Cost/token totals include charged losers while logical user-request count and delivered-response metrics remain request-level.

## Usage Log Metadata

Public-safe `other.hedge` metadata from backend:

- `attempt_id`: stable string.
- `attempt`: numeric ordinal.
- `role`: `winner` or `loser`.
- `usage_source`: `upstream`, `estimated`, or `unknown`; estimated is normal winner fallback only.
- `metering_status`: `complete`, `partial`, or `unmetered`.
- `settlement_status`: `settled`, `failed`, `unmetered`, or `not_billed`. `not_billed` means policy excludes this loser and its reservation was released without charge; preserve received metering evidence independently. Billing breakdowns are shown only for `settled`.

Rows share the original client request_id; attempt channel internals remain admin-only. Zero-quota unmetered audit rows must not be presented as confirmed zero provider cost or successful delivered responses.

## Verified Locale Runtime

Live code at `web/src/i18n/config.ts:22` imports only `zh.json` and supports only `zhCN`. The generic seven-language AGENTS/skill catalog is stale for this fork. Preserve the actual shipped runtime: add translation keys via the mandated script and sync for the existing locale; do not reintroduce removed language support in this task.
