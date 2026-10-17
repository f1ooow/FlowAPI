# Admin User Cost

## 1. Scope / Trigger

Use when exposing upstream channel costs in administrator user lists. Cost is
not the user's `used_quota`, and must not be recalculated from mutable channels.

## 2. Signatures

Only items returned by `GET /api/user`, `GET /api/user/`, and
`GET /api/user/search` gain these response fields:

```ts
cost_quota: number | null
cost_incomplete: boolean
```

The database inputs are retained consume logs' `user_id`, `base_quota`,
`channel_cost_ratio`, and `channel_cost_ratio_estimated`. No migration or
persisted user counter is required.

## 3. Contracts

- Batch positive user IDs from the current page against `LOG_DB`; never join
  the main `users` or `channels` tables to the log database.
- Group by user and recorded ratio, summing base quota before applying
  `ChannelCostRatioBucket.CostQuota`. This reuses the monitoring board's
  fixed-point ratio and quota rounding, but excludes estimated rows.
- Exclude missing/estimated ratio snapshots and unavailable base amounts from
  exact cost; mark the aggregate incomplete. Never substitute current channel
  settings, even after rename, deletion, or recreation.
- A zero-base, zero-charge row is also incomplete: `consumeLogBaseQuota`
  returns this sentinel for free user groups, so zero charge is not proof of
  zero upstream cost. A user with no consume logs still has an empty total of zero.
- Use aggregate-safe conversion for totals, not the single-request int32 cap.
- Keep fields on an admin-list DTO, not `model.User` or ordinary user responses.
- Preserve existing quota, billing, pagination, filtering, and sorting behavior.
- Cost covers retained logs only. Purged or never-written history cannot be
  recovered; `cost_incomplete: false` does not certify a lifetime ledger.
- The frontend uses `formatQuota`, never treats quota units as currency, and
  does not offer cost sorting without a server-wide sorting implementation.

## 4. Validation & Error Matrix

| Condition | Result |
| --- | --- |
| Successful query, no consume logs | `cost_quota: 0`, complete |
| Valid observed snapshot and positive base quota | Include bucket cost |
| Missing/estimated ratio or unusable base amount | Exclude row, incomplete |
| Non-consume logs | Ignore |
| Empty page | Skip log query; keep empty list |
| Logging disabled or log query fails | Successful user list, `null` cost, incomplete |
| Channel renamed, repriced, deleted, or added | Historical cost unchanged |

## 5. Good / Base / Bad Cases

- Good: a user's two requests with base quotas `1000` and ratios `0.5` and `2`
  cost `2500`, even if their channel now has ratio `7` or no longer exists.
- Base: a new user displays a formatted zero; unavailable log data displays a
  dash with an explanation, never a formatted zero.
- Bad: `used_quota * current_channel.cost_ratio`, including inferred backfills
  as exact snapshots, or presenting query failure as complete zero cost.

## 6. Tests Required

- Multiple users and historical ratios, bounded batch query, separate log DB.
- Historical stability after current channel changes/deletion/addition.
- Missing, estimated, and unusable snapshots excluded with incomplete marker.
- List and search include the fields; ordinary user responses do not.
- Empty history, empty pages, logging disabled, and log query failure.
- Frontend currency conversion, zero versus null, incomplete tooltip, and
  desktop/mobile visibility using the real table/formatter paths.

## 7. Wrong vs Correct

Wrong: enumerate today's channels and multiply user charges by their current
ratios. That erases deleted channels and reprices historical requests.

Correct: aggregate the page's user IDs directly from retained consume logs and
apply only the observed ratio stored on each historical bucket, exposing any
excluded rows through `cost_incomplete`.
