# Implementation Plan

Status: implementation authorized on 2026-09-17.

## Work Order

- [x] Pin CCH source and research the three timeout semantics, race lifecycle, cost and concurrency behavior.
- [x] Inspect existing FlowAPI routing, precommit gate, billing ownership and channel configuration surface.
- [x] Persist proposed PRD and technical design.
- [x] Obtain scope review, resolve changes and curate implementation/check context.
- [x] Activate the existing approved task; implementation in progress.
- [x] Add compatible channel timeout settings and backend validation; cover omitted/zero/bounds and settings round-trip.
- [x] Implement isolated attempt ownership, a bounded two-attempt scheduler and winner arbitration at the shared stream gate.
- [x] Integrate additive per-attempt reservations, winner/loser settlement, bounded background usage collection, cancellation outcomes, affinity, health and route audit.
- [x] Add global default-on loser billing switch, request-local policy snapshot, prefix admission, disabled cancellation/refund behavior and regressions.
- [x] Add correlated per-attempt billing details to user/admin logs, including late loser charges and incomplete metering; preserve aggregate cost attribution without double-counting summary rows.
- [x] Separate logical request/success/RPM metrics from billable-attempt cost and token totals; review correlated-row pagination.
- [x] Add per-attempt idle/nonstream hard deadlines without changing the soft hedge trigger or post-commit replay boundary.
- [x] Add channel drawer controls, all-zero defaults, form serialization and current shipped zhCN translations following frontend skills' script workflow; live i18n runtime has only zhCN.
- [x] Run focused backend lifecycle/billing tests, race detection, frontend tests/typecheck/lint/build and browser checks for the changed drawer.
- [x] Final independent Trellis review passed after all fixes; reusable contracts and verification results are recorded.

## Delegation Boundaries

Backend attempt orchestration and billing changes belong to one implementation owner because they share mutable lifecycle contracts. Channel UI/i18n can run independently only after JSON field/default/bounds contracts are fixed. Review owns verification and documented findings. Preserve all existing unrelated worktree changes.

## Planned Verification

Choose exact test names after implementation; baseline commands are:

```sh
go test ./controller ./relay/... ./service ./model ./middleware ./dto
go test -race ./controller ./relay/helper ./relay/common ./service ./middleware
```

Run impacted frontend tests with the existing package script and then, from `web/`:

```sh
bun run typecheck
bun run build
```

Use the repository lint/format scripts on changed files and i18n validation according to the skill. If `relaykit/` changes, also run from `relaykit/`:

```sh
GOWORK=off go build ./...
```

## Required Regression Outcomes

- Exactly one attempt can commit client output; each billable winner/loser attempt settles once with no mixed stream bytes or duplicate charges.
- Losing attempts drain within finite resource/deadline limits; cancellation reaches upstream when those limits are reached. Pre-winner disconnect cancels every attempt; detached post-winner collectors remain bounded.
- Existing channel priority/eligibility/model redirects, failover caps and explicit no-retry policy continue to work.
- Billing uses each attempt's own route ratio, expression snapshot and original billing identity; every backup reserves independently before dispatch, even when cheaper. No cross-attempt or duplicate refund.
- Verify group channel-ratio inclusion both off and on with different A/B channel ratios, Auto-group changes, zero user factors and immutable snapshots during delayed loser settlement. Reuse HandleGroupRatio; never unconditionally multiply channel ratios or apply them twice.
- Correlated request total equals the sum of per-attempt charges; user-visible details identify extra racing charges while provider-private fields remain admin-only. Incomplete/no usage never fabricates a charge or known-zero provider cost.
- Startup/heartbeat bytes do not win, legitimate empty completion does, and committed streams never switch channels.
- All three timers have distinct semantics; missing and zero values disable them, and only manually configured positive values enable them.

## Review And Rollback Points

Review request isolation and billing ownership before connecting concurrent dispatch. Review configuration defaults before frontend wiring. Inspect race-test output and request-cancellation behavior before declaring the backend complete. Per-channel zero hedge threshold disables the feature; no deployment or live configuration change is authorized by this implementation task.
