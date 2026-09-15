# Channel List Derived Metrics

## 1. Scope / Trigger

Use this contract when an administrator channel-list field is derived from request logs rather than the `channels` table. The main database and `LOG_DB` may be separate databases, and `LOG_DB` may be ClickHouse, so list enrichment must not rely on a cross-database join.

## 2. Signatures

The current daily-consumption implementation uses:

```go
func SumUsedQuotaByChannelIds(
    ctx context.Context,
    channelIds []int,
    startTimestamp int64,
    endTimestamp int64,
) (map[int]int64, error)

type Channel struct {
    TodayUsedQuota *int64 `json:"today_used_quota" gorm:"-"`
}
```

The field is returned on items from both `GET /api/channel` and `GET /api/channel/search`. It is response-only and must remain excluded from persistence and channel update payloads.

## 3. Contracts

- `today_used_quota` is a quota-unit integer or `null`.
- `0` means the log query succeeded and the channel had no matching consumption.
- `null` means the metric was unavailable, including disabled consume logging or a failed log query.
- A day is the UTC+8 calendar day from `00:00:00` through the injected/current time. It must not depend on `time.Local`.
- Daily consumption sums only `LogTypeConsume`. Refund entries do not reduce the value.
- Aggregate only the channel IDs in the current response page, in bounded batches, then merge the result into the channel DTOs in Go.
- Derived log metrics do not participate in main-database sorting unless a separate, consistent sorting design is implemented.

## 4. Validation & Error Matrix

| Condition | Required behavior |
| --- | --- |
| Empty channel page | Skip the log query and return the normal empty list |
| Duplicate, zero, or negative channel IDs | Deduplicate positive IDs and ignore invalid IDs |
| Consume logging disabled | Skip the log query and return `today_used_quota: null` |
| `LOG_DB` query fails | Log the failure, keep the channel-list response successful, and return `null` |
| Query succeeds with no row for a channel | Return an explicit pointer to `0` |
| Matching consume rows exist | Return their `SUM(quota)` for that channel |
| Refund, other channel, or out-of-range row | Exclude it from the sum |

## 5. Good / Base / Bad Cases

- Good: fetch the current page from `DB`, run one grouped query against `LOG_DB`, and merge a `map[channelID]quota` into the response.
- Base: a valid channel with no consumption today receives `0`, allowing the UI to distinguish it from unavailable data.
- Bad: join `channels` to `logs`, issue one log query per row, treat query failure as zero, or subtract refund rows.

## 6. Tests Required

- Model test: multiple consume rows sum by channel; duplicates and invalid IDs are ignored; refunds and range boundaries are covered.
- Controller test: the UTC+8 day boundary is deterministic even when the process timezone differs.
- Endpoint test: list and search both return zero after a successful empty aggregate and `null` when logging is disabled or the query fails.
- Frontend test: schema parsing preserves `null` versus `0`; tag rows sum only fully available child values; table/card rendering and sensitive-value masking cover both states.

## 7. Wrong vs Correct

Wrong:

```go
// Couples the main list to LOG_DB and cannot work when the databases differ.
DB.Table("channels").Joins("LEFT JOIN logs ON logs.channel_id = channels.id")
```

Correct:

```go
quotaByChannel, err := model.SumUsedQuotaByChannelIds(ctx, channelIDs, start, end)
if err == nil {
    for _, channel := range channels {
        channel.TodayUsedQuota = common.GetPointer(quotaByChannel[channel.Id])
    }
}
```
