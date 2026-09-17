# Resumed implementation review

Resumed 2026-09-17 from Codex task `研究 CCH 超时路由策略`
(`01a0af4d-a22a-7cf0-ae60-8aba13a79457`). The existing task was already
approved for implementation. Latest user additions: all three timeout fields
default to zero; add a global default-on loser-billing switch matching the
provided screenshot.

## Review findings to resolve

- Gemini nested response modalities and provider-owned cached content must be
  excluded from speculative replay.
- Empty usage objects are not reported zero usage and cannot justify constant
  expression charges on interrupted losers.
- Queued threshold events must not launch a backup after client cancellation.
- Late loser charges must not create new logical RPM samples; quota/token
  totals remain additive.
- Cancellation/refund by policy must preserve known metering evidence and be
  distinguishable from unknown usage in logs.
- Consume logs need the applicable coordinator history before insertion;
  adding it only to the parent context afterward loses the audit trail.

The independent checker fixed logical RPM: explicit loser rows are excluded
even when they settle outside the winner's time window or are viewed through a
channel/group filter. Fees/tokens stay additive and legacy rows keep their
original counting behavior. Focused model/ClickHouse-pagination tests passed
with SQLite fixtures; live MySQL/PostgreSQL/ClickHouse were not exercised.

All reported feature findings were fixed and checked, including protocol
structure, empty usage, Gemini aliases/wrappers, tool surcharge evidence,
policy refunds and history. Final independent full-scope review passed with no
remaining feature blockers. See `validation.md` for passing checks and the two
independently reproduced baseline race-test failures.

## Browser verification

Parent executed `/private/tmp/flowapi-hedge-browser-static-qa.mjs` against
locally built assets with all API calls mocked. Passed:

- Existing 60/120/300-second settings loaded correctly.
- Clearing each timeout saves explicit zero; reopening shows zero.
- Timeout controls remain inside a 390-pixel viewport.
- Missing global setting defaults to ON.
- Saving OFF persists across reload, then keyboard toggling ON saves true.
- Global settings layout inspected at desktop and mobile widths.
- No page errors or console errors from the production-build browser run.

Screenshots: `/private/tmp/flowapi-hedge-qa/{timeouts,loser-billing}-{desktop,mobile}.png`.
The development-server run also passed timeout round-trip but emitted an
existing `ChannelAuthSection` nested `div`/`p` warning outside this task's
changed files; no unrelated authentication UI edits were made.

## Scope exclusions

Existing changes to `router/web-router.go`, `router/web_router_cache_test.go`,
`web/src/lib/http-client.ts`, the image-resolution-pricing research, and older
monitoring research directories are unrelated and must remain outside this
task's commit.
