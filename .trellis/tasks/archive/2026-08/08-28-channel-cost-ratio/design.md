# Technical Design

## 1. Design Objective

Introduce channel cost ratio as a route-dependent billing factor without creating separate formulas for different billing modes. The system must resolve one auditable multiplier state for the current user group, billing group, and selected channel, then use that state consistently for pre-consume, retry reservation, settlement, logs, asynchronous tasks, and user-facing range summaries.

## 2. Persisted Data

### 2.1 Channel cost ratio

Add a nullable channel field:

```go
CostRatio *float64 `json:"cost_ratio"`
```

- The database column has no GORM business-default tag.
- `GetCostRatio()` returns `1` when the pointer is nil.
- Create/update validation accepts only finite values in the documented safe range `0 < ratio <= MaxChannelCostRatio`.
- A shared constant owns the upper bound; validation is reused by API, model normalization, and tests.
- The field is returned only by existing administrator channel APIs.
- Changing the field through the general channel update API requires root authority because it changes billing policy; delegated channel operators may still edit their existing allowed fields.
- Channel audit records include the field name and old/new numeric values, never credentials.

### 2.2 Group settings

Extend group ratio settings with unambiguous fields:

```text
group_ratio_setting.user_group_ratio
group_ratio_setting.include_channel_ratio
```

- `user_group_ratio` is a nested map: user group -> billing group -> multiplier.
- `include_channel_ratio` is a map: billing group -> boolean.
- Missing user multiplier returns `1`.
- Missing include flag returns `false`.
- New writes use only the new keys; legacy `GroupGroupRatio` is retained only as a migration input/compatibility read until migration is complete.

### 2.3 Migration version

Store a versioned marker in options, for example `UserGroupRatioMigrationVersion`.

Migration runs after options are loaded and follows this state machine:

1. If the current version is already present, do nothing.
2. If the new user-ratio key is already populated but the marker is absent, validate it and require an explicit deterministic reconciliation path; never overwrite it from legacy data.
3. Convert each legacy absolute special ratio using the target base group ratio.
4. Persist the new map and migration marker atomically with `UpdateOptionsBulk`.
5. If a zero-base/nonzero-special conflict exists, do not write the marker or partially convert. Keep the legacy runtime interpretation active, log the exact conflicting user-group/billing-group pairs, and return structured conflict status to the root settings UI.
6. Re-running after conflicts are resolved performs the conversion once.

Conversion rules:

```text
base > 0:  new user ratio = old absolute ratio / base
base absent: use existing runtime fallback base 1
base = 0 and old = 0: new user ratio = 1
base = 0 and old > 0: conflict
```

## 3. Runtime Billing State

Evolve the existing group ratio runtime state rather than passing anonymous multipliers through `OtherRatios`.

Recommended shape:

```go
type BillingRatioInfo struct {
    GroupRatio          float64
    UserGroupRatio      float64
    ChannelRatio        float64
    IncludeChannelRatio bool
    EffectiveRatio      float64
}
```

During a compatibility transition, `PriceData.GroupRatioInfo.GroupRatio` may continue to carry the effective ratio for existing settlement callers, but new code must also populate the named components. The final implementation should avoid having two independently calculated “effective” values.

Resolver inputs:

- user group from `RelayInfo.UserGroup`
- selected billing group from `RelayInfo.UsingGroup`, including resolved `auto_group`
- selected channel cost ratio from channel context/meta
- include flag from group settings

Resolver output:

```text
effective = group * user * (include ? channel : 1)
```

The resolver validates every factor and uses the existing quota saturation path for the resulting charge.

## 4. Channel Selection And Retry

### 4.1 Context propagation

`SetupContextForSelectedChannel` writes the selected channel cost ratio into a typed context key. `RelayInfo.InitChannelMeta` copies it into channel metadata so every relay format sees the same value.

### 4.2 Initial attempt

Distributor middleware has already selected the initial channel before controller pricing. Initial pre-consume therefore uses the initial channel ratio read from context.

### 4.3 Retry attempts

Replace the tiered-only refresh concept with a unified route-dependent billing preparation step:

```text
select channel
-> update selected group and channel context
-> resolve BillingRatioInfo
-> recompute target pre-consume for current billing mode
-> reserve any positive delta
-> send upstream request
```

This step must cover:

- ordinary model-ratio billing
- fixed model-price billing
- tiered expression billing
- per-call task billing
- locked/origin task channels
- specified channel requests

If a retry target is cheaper, the existing reservation remains; final settlement refunds the difference. If it is more expensive, the reservation is increased before upstream work starts.

### 4.4 Final settlement

Settlement reads the last successfully attempted route's ratio state. Logs, user used quota, channel used quota, billing session settlement, and tiered snapshots must all use the same effective ratio.

## 5. Billing Mode Integration

### 5.1 Token/model-ratio billing

Replace `modelRatio * groupRatio` with `modelRatio * effectiveRatio` in pre-consume and actual token settlement.

### 5.2 Fixed model-price billing

Replace `modelPrice * QuotaPerUnit * groupRatio` with `modelPrice * QuotaPerUnit * effectiveRatio`, preserving request-specific `OtherRatios` and one-time rounding.

### 5.3 Tiered expression billing

Treat `EffectiveRatio` as the snapshot's route-dependent post-expression multiplier. Refresh it before every attempt and before settlement. Preserve expression text, request input, tier trace, and quota saturation behavior.

### 5.4 Per-call and asynchronous task billing

Per-call task price calculation includes the current effective ratio. `TaskBillingContext` snapshots at least:

```text
base group ratio
user group ratio
channel ratio
include-channel flag
effective ratio
model price/model ratio
other ratios
origin model
```

Polling settlement, token recalculation, and refunds use the snapshot. They must not re-read mutable group/channel settings. Historical task payloads without the new fields use their stored legacy `GroupRatio` as the effective ratio.

## 6. Group Range Service And API

### 6.1 Range source

Build a group-to-channel-ratio summary from enabled channels in the existing channel cache. A channel assigned to multiple comma-separated groups contributes to each group. Disabled channels do not contribute. `InitChannelCache`/channel updates invalidate or rebuild the summary with existing channel cache lifecycle.

The range is intentionally group-wide and model-independent, matching the user requirement and avoiding a model selector dependency in API Key group selection.

### 6.2 API contract

The authenticated self-group endpoint returns structured display data, for example:

```json
{
  "premium": {
    "desc": "Premium",
    "ratio_kind": "range",
    "ratio_min": 1.2,
    "ratio_max": 1.8,
    "available": true
  }
}
```

For a single value, `ratio_kind` is `single` and min/max may be equal. `auto` uses `ratio_kind: "auto"`. A group with no enabled channels uses `available: false` and no numeric range.

The unauthenticated `/api/user/groups` response remains compatible and does not include range fields. If it currently shares a controller, split public and authenticated response construction so user identity and cost-derived data cannot leak through the anonymous route.

### 6.3 Formatting

The backend returns numeric values; the frontend formats them with bounded precision and trims trailing zeros. It displays only `1.2x` or `1.2x-1.8x`, never a factor breakdown.

## 7. Admin And User UI

### 7.1 Channel editor

Add one numeric “Channel cost ratio” control to the existing channel routing/pricing-relevant section. Default is `1`; include validation and concise help explaining that only opted-in groups apply it to user billing.

### 7.2 Group pricing editor

Extend each existing group row with a checkbox/switch “Include channel ratio”. Rename “Special ratio rules” to “User group ratios” and update descriptions and sentence-style rule summaries. Do not add a separate decorative card or duplicate internal-state badges.

### 7.3 User API Key group selection

Update group option types and the shared ratio badge to support structured `single`, `range`, `auto`, and unavailable states. The trigger, dropdown options, API Key table cells, search text, and tests must remain consistent.

## 8. Pricing Sync Boundary

Remove:

- channel-list or channel-editor row actions that fetch an upstream ratio for one channel

Keep:

- root-only `/api/ratio_sync/*` routes and `controller/ratio_sync.go`
- sync DTOs, tests, OpenAPI entries, and the new-frontend upstream price-sync tab
- official/public pricing presets and selected-upstream `/api/pricing` ingestion
- `/api/ratio_config`
- `ExposeRatioEnabled`
- channel `/upstream_updates/*` model-list detection
- local model ratio and model price configuration

The global model-price synchronization workflow may read pricing from selected channels, but it does not read or write the channel `cost_ratio` used by route billing.

## 9. Security And Auditing

- Root-only mutation for channel cost ratio aligns with root-only global pricing changes.
- Channel list/detail responses remain admin-only and may contain the ratio.
- Authenticated users receive only final display values, never raw factor fields.
- Logs store component ratios under `other.admin_info.billing_ratios` or an equivalent admin-only nested structure; user-visible log sanitization continues stripping `admin_info`.
- Migration conflict logs contain group names and ratios but no credentials or user personal data.

## 10. Compatibility And Rollback

- Nullable channel ratio plus disabled group flags make the database migration behavior-neutral.
- User-ratio conversion is atomic and versioned.
- Old async task snapshots retain legacy effective-ratio interpretation.
- Rollback before enabling any include flags is behavior-neutral.
- After include flags are enabled, rollback requires restoring the previous binary and retaining the added nullable column/options; do not destructively drop data.
- `/api/ratio_sync/*` remains a root-only administrator contract; channel cost-ratio rollout must not remove it.

## 11. User-Side Privacy Boundary

`other.admin_info.billing_ratios` is an administrator-only audit payload. The user log formatter must remove the entire `admin_info` object and also scrub legacy top-level billing-factor keys before serializing a response. `group_ratio` remains only as the already-composed final ratio for compatibility with existing user log rendering.

The anonymous `/api/user/groups` response contains descriptions only. The pricing catalog may keep its existing response field for client compatibility, but its values must be composed final display multipliers rather than base group ratios. For a channel-inclusive group, use the final lower-bound display value; never return the pre-channel value because dividing the authenticated final range by it would reveal channel cost bounds. No user-facing payload may contain raw channel cost fields or the admin billing-ratio map.
