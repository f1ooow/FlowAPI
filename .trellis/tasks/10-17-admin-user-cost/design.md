# Technical Design

## Boundaries

- Reuse the existing consume-log cost source used by the channel availability board.
- Add a user-dimension aggregation in `model/log.go`; do not join `channels` or read current channel configuration.
- Enrich only the administrator user-list/search response with cost fields. Existing quota fields, billing, and ordinary user responses remain unchanged.
- Add one frontend column using the existing currency-aware quota formatter.

## Historical cost contract

For each consume log, the exact upstream-cost contribution is:

```text
base_quota * channel_cost_ratio / channel_cost_ratio_scale
```

The ratio must be the log's write-time snapshot (`channel_cost_ratio`) with
`channel_cost_ratio_estimated = false`. A row with an estimated ratio, missing
ratio, or missing positive `base_quota` contributes no exact cost and marks the
user's aggregate as incomplete. It must never be repriced from `channels`.

Rows with a zero charge and zero base quota do not add cost and still set the
incomplete marker. `consumeLogBaseQuota` returns the zero sentinel for free
user groups too, so a zero user charge does not prove zero upstream cost.
This implementation-time correction avoids confusing user charges with cost.

The aggregation groups by `user_id`, snapshot ratio, estimated flag, and
`base_quota` sentinel. This preserves the existing board's ratio-bucket
rounding while allowing incomplete rows to be identified without per-log
queries.

## Backend flow

1. `GetAllUsers` and `SearchUsers` fetch their normal page as today.
2. Collect positive user IDs from that page.
3. Call one `LOG_DB` aggregate for the page IDs, with `type = LogTypeConsume`.
4. Fold exact buckets through `ChannelCostRatioBucket.CostQuota`; mark
   `CostIncomplete` for estimated/missing-snapshot buckets.
5. Attach `cost_quota` and `cost_incomplete` to an admin-list-only response DTO.
6. If consume logging is disabled or the log query fails, preserve the user
   list and return `cost_quota: null` with `cost_incomplete: true`; do not
   disguise unavailable data as a zero cost.

The query is batched using the existing log aggregation batch limit and works
against the shared main database, a separate SQL log database, and ClickHouse
without consulting the channel table. Deleted or renamed channels therefore
remain in the result.

## API contract

Each admin list/search item gains:

- `cost_quota`: observed historical upstream cost in internal quota units, or
  `null` when the metric is unavailable. Successful empty history returns `0`.
- `cost_incomplete`: `true` when one or more consume logs for the user could
  not be priced from an observed request-time snapshot.

The existing `quota` and `used_quota` fields retain their current meaning.
The metric covers retained consume logs, not an independent lifetime ledger;
deleted logs cannot be reconstructed by this read-only aggregation.

## Frontend behavior

- Add a `Cost` column beside the existing quota column.
- Format `cost_quota` through `formatQuota`, which applies the configured
  currency and quota-per-unit conversion.
- Show a small indicator/tooltip when `cost_incomplete` is true, explaining
  that the displayed value excludes historical rows without a request-time
  cost snapshot.
- Add all user-facing strings through the project i18n locale workflow.
- Render unavailable data distinctly from zero, with an explanatory tooltip.

## Compatibility and risks

- No schema migration is needed; `base_quota`, `channel_cost_ratio`, and
  `channel_cost_ratio_estimated` already exist and are populated by the board
  rollout/backfill.
- The 16 HK estimated rows remain visible only through the incomplete marker,
  not silently included in exact user costs. The 2,177 zero-base rows contribute
  nothing numerically under board accounting, but the new user view marks
  them incomplete because the stored base does not prove zero upstream cost.
- Current channel edits, renames, and deletes cannot change the result because
  neither the aggregation nor the response enrichment reads `channels`.
- Page-level aggregation adds one bounded log query per list request rather
  than an N+1 query per user.

## Rollback

Remove the response enrichment and frontend column; the existing log columns
and board aggregation remain untouched. No data rollback is required.
