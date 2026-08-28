# Route-Dependent Billing Ratios

## Scenario: Billing factors that depend on the selected channel

### 1. Scope / Trigger

Use this contract whenever a billing multiplier depends on user group, billing group, selected channel, Auto routing, affinity, or retry. It prevents pre-consume from using one route while settlement or asynchronous polling uses another.

### 2. Signatures

```go
type Channel struct {
    CostRatio *float64 `json:"cost_ratio"`
}

type GroupRatioInfo struct {
    GroupRatio          float64 // effective ratio for settlement compatibility
    BaseGroupRatio      float64
    UserGroupRatio      float64
    ChannelRatio        float64
    IncludeChannelRatio bool
}
```

Persisted option keys:

```text
group_ratio_setting.user_group_ratio
group_ratio_setting.include_channel_ratio
UserGroupRatioMigrationVersion
```

Authenticated group display fields:

```text
ratio_kind: single | range | auto | unavailable
ratio_min: number (single/range only)
ratio_max: number (single/range only)
available: boolean
```

### 3. Contracts

The only effective-ratio formula is:

```text
effective = base group ratio * user-group ratio * (include channel ratio ? selected channel ratio : 1)
```

- Missing channel cost ratio is `1`; channel cost ratio must satisfy `0 < ratio <= 1000`.
- Missing user-group ratio is `1`; a configured zero is preserved.
- Resolve the ratio after every channel/group selection, including retries.
- Before sending the upstream attempt, reserve the recomputed target if it exceeds the existing reservation.
- Settlement uses the last successful route's effective ratio; a cheaper route refunds through normal settlement.
- Asynchronous tasks snapshot all component ratios and the effective ratio. Polling and token recalculation must not read current mutable settings.
- Anonymous group responses may expose the existing base `ratio`, but never cost-derived ranges or raw channel ratios.
- Component ratios belong under `other.admin_info.billing_ratios`; ordinary user log views strip `admin_info`.

### 4. Validation & Error Matrix

| Condition | Required behavior |
|---|---|
| Channel ratio nil | Normalize to `1` |
| Channel ratio non-finite, `<= 0`, or `> 1000` | Reject channel create/update |
| User-group ratio missing | Use `1` |
| User-group ratio negative, non-finite, or above bound | Reject option update |
| Legacy negative absolute override | Ignore it as invalid; never create a credit |
| Legacy positive override with zero base group ratio | Keep legacy runtime semantics, report conflict, do not write migration marker |
| Include-channel flag enabled before migration completes | Reject activation |
| Retry target is more expensive | Reserve higher target before upstream request |
| Historical task lacks `EffectiveRatio` | Treat stored legacy `GroupRatio` as effective |

### 5. Good / Base / Bad Cases

- Good: base `2`, user `0.8`, selected channel `1.5`, include enabled -> effective `2.4`.
- Base: same inputs with include disabled -> effective `1.6`.
- Good retry: initial effective `1.6`, retry effective `2.4` -> reserve `2.4` target before sending retry.
- Bad: compute pre-consume before channel selection and only change the log's ratio after retry.
- Bad: asynchronous polling re-reads today's group/channel settings for a task submitted yesterday.

### 6. Tests Required

- Channel validation: nil normalization, valid fractional value, zero/negative/non-finite/overflow rejection.
- Migration: equivalent conversion, missing group fallback, zero/zero, zero-base conflict, idempotent second run.
- Resolver: base x user x channel and include-disabled behavior.
- Retry reservation: target increases before attempt and final pre-consumed quota is updated.
- Async snapshot: token recalculation uses persisted model/effective ratios after settings change.
- Range API: enabled channels only, comma-separated groups, equal min/max, unavailable, Auto, authenticated vs anonymous.
- Logs: component ratios nested under `admin_info`; non-admin formatting removes the entire object.

### 7. Wrong vs Correct

#### Wrong

```go
// Group special ratio replaces the base, and channel is unknown here.
ratio := ratio_setting.GetGroupRatio(info.UsingGroup)
if special, ok := ratio_setting.GetGroupGroupRatio(info.UserGroup, info.UsingGroup); ok {
    ratio = special
}
preConsume(baseQuota * ratio)
```

#### Correct

```go
// Run after the selected route is in context, and repeat for every retry.
ratioInfo := helper.HandleGroupRatio(ctx, info)
info.PriceData.GroupRatioInfo = ratioInfo
if apiErr := service.PrepareBillingForSelectedRoute(ctx, info); apiErr != nil {
    return apiErr
}
```
