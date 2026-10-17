# Implementation Plan

## Ordered checklist

1. Read backend/frontend pre-implementation and cross-layer guidelines before
   editing product code.
2. Add a user-scoped log aggregation result type and batch query in
   `model/log.go`, reusing `ChannelCostRatioBucket.CostQuota` and the existing
   log database/dialect conventions.
3. Add response-only cost enrichment to `GetAllUsers` and `SearchUsers` using
   the page's user IDs. Preserve existing pagination, filtering, and user
   fields.
4. Add deterministic backend tests for:
   - multiple users and multiple historical ratios;
   - deleted channel IDs and channel renames not affecting totals;
   - estimated/missing snapshots setting `cost_incomplete` without adding to
     exact cost;
   - no per-user query path (one aggregate call at the model boundary).
   - list availability and null cost on a log query failure or disabled logging;
   - ordinary user responses remain free of administrator cost fields.
5. Extend the frontend user type and add the cost column/tooltip. Add locale
   keys for the column and incomplete-history explanation.
6. Add/update frontend column tests for currency formatting, zero cost, and the
   incomplete marker, unavailable state, and mobile rendering.
7. Run focused Go model/controller tests, frontend user tests, and the relevant
   type/build checks.
8. Run the Trellis quality check, review only the task allowlist, then prepare
   the scoped commit.

## Validation commands

```bash
go test ./model ./controller
cd web && bun run test -- src/features/users/__tests__
cd web && bun run typecheck
cd web && bun run build
go test ./...
```

For any `relaykit/` change (not expected), also run:

```bash
cd relaykit && GOWORK=off go build ./...
```

## Risk and rollback points

- Backend risk: an aggregate query that accidentally joins current channels or
  groups by only `channel_id` would reprice history. Regression tests must use
  deleted IDs and multiple ratio snapshots.
- Backend risk: adding fields directly to a broadly reused model response could
  leak admin-only data. Keep cost fields on the admin list response path or
  explicitly verify all affected serializers.
- Frontend risk: treating quota units as already-currency would display a
  500,000x error. Reuse `formatQuota` and assert the rendered result.
- Rollback before commit: revert only the allowlisted model/controller/frontend
  files; do not touch existing channel-board code or unrelated work.

## Workspace

- Worktree: `/Users/Zhuyu/.codex/worktrees/flowapi-admin-user-cost/FlowAPI`
- Branch: `feat/admin-user-cost`
- Fetched baseline: `origin/main` at `d139fc73162bfcb2f043ebf80c53587e6df6f2a8`.
- The original checkout's unpushed retry-badge commit remains untouched.
- Implementation does not authorize commit, push, merge, or deployment.
