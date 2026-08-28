# Current Ratio And Billing Boundaries

## Scope

This research records repository evidence gathered before planning the channel cost ratio feature. It is descriptive of the current checkout and does not authorize implementation.

## Confirmed Current Behavior

- `router/api-router.go` registers public `/api/ratio_config` separately from root-only `/api/ratio_sync/channels` and `/api/ratio_sync/fetch`.
- `controller/ratio_sync.go` fetches remote `/api/pricing` or `/api/ratio_config` data and normalizes model pricing fields. This is the feature requested for removal.
- Channel upstream model update endpoints live under `/api/channel/upstream_updates/*`; they synchronize model availability, not pricing, and remain in scope to preserve.
- `model.Channel` has no cost ratio column. The channel cache loads complete channel rows and can carry a new nullable field after cache refresh.
- `setting/ratio_setting/group_ratio.go` stores base group ratios and `GroupGroupRatio`. `GetGroupGroupRatio` returns the configured special value directly.
- `relay/helper/price.go:HandleGroupRatio` currently replaces the base group ratio with the special value. The special value is therefore an absolute effective ratio under current behavior, not a multiplier.
- `controller/group.go:GetUserGroups` returns one `ratio` field per selectable group. `web/src/features/keys/components/api-key-group-combobox.tsx` and the API key list consume that single value.
- `controller/relay.go` computes the initial price and pre-consumes quota before entering the channel retry loop. The initial selected channel is already available in Gin context because distributor middleware selected it before the controller.
- On retry, `controller/relay.go:getChannel` refreshes group ratio and calls `SetupContextForSelectedChannel`, but only tiered billing currently has a dedicated reservation refresh step.
- `relay/helper/price.go` applies the current effective group ratio in ordinary ratio billing, fixed-price billing, per-call task billing, and tiered expression pre-consume.
- `service/text_quota.go`, `service/quota.go`, and `pkg/billingexpr/settle.go` use the stored effective group ratio during settlement.
- `model.TaskBillingContext` snapshots model price, group ratio, model ratio, and other ratios, but not the channel ratio. `service/task_billing.go:RecalculateTaskQuotaByTokens` re-reads current ratio settings, so it must be changed to use the submission snapshot.
- `model.InitChannelCache` loads all channels and rebuilds enabled group/model indexes. Channel CRUD and status flows already refresh this cache, providing an invalidation boundary for group ratio ranges.
- The root option system currently exposes legacy `GroupGroupRatio`; the new multiplier needs an idempotent migration and an unambiguous persisted name.

## Planning Consequences

1. Store channel cost ratio as a nullable channel column and normalize nil to `1` in one getter.
2. Preserve base group ratio, user group ratio, channel ratio, application flag, and effective ratio separately in runtime billing state.
3. Refresh route-dependent billing state after each selected channel, including retries, before the upstream request is sent.
4. Persist all factors needed by asynchronous settlement instead of re-reading mutable global settings.
5. Derive group display ranges from enabled cached channels and return structured min/max data only from authenticated self-service APIs.
6. Remove only ratio synchronization; keep ratio exposure and upstream model-list synchronization.

## Known Compatibility Edge

The old absolute special ratio cannot be converted to a multiplier when the target base group ratio is zero and the old special ratio is positive. Planning therefore requires explicit conflict detection and no silent migration completion.
