# Local feature commit

Implementation, focused verification and final independent review passed.

`feat(routing): add provider hedge racing and loser billing`

One coherent feature commit includes the resumed backend, configuration/UI, billing logs, regression tests, task research and reusable contracts.

## Included paths

- `.trellis/spec/backend/billing-ratio-routing.md`
- `.trellis/spec/backend/index.md`
- `.trellis/spec/backend/streaming-provider-hedge.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/check.jsonl`
- `.trellis/tasks/09-17-streaming-provider-hedge/design.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/implement.jsonl`
- `.trellis/tasks/09-17-streaming-provider-hedge/implement.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/prd.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/research/cch-timeout-routing.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/research/commit-plan.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/research/configuration-surface.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/research/flowapi-backend.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/research/hedge-user-billing.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/research/implementation-contract.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/research/resume-review.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/research/validation.md`
- `.trellis/tasks/09-17-streaming-provider-hedge/task.json`
- `common/body_storage.go`
- `common/body_storage_fork_test.go`
- `controller/relay.go`
- `controller/relay_hedge.go`
- `controller/relay_hedge_policy_test.go`
- `controller/relay_hedge_test.go`
- `logger/logger.go`
- `model/clickhouse_log_test.go`
- `model/log.go`
- `model/log_hedge_stats_test.go`
- `relay/channel/api_request.go`
- `relay/channel/deadline_body.go`
- `relay/channel/deadline_body_test.go`
- `relay/common/hedge.go`
- `relay/common/relay_info.go`
- `relay/helper/hedge_usage.go`
- `relay/helper/hedge_usage_test.go`
- `relay/helper/stream_scanner.go`
- `relay/helper/stream_scanner_test.go`
- `relaykit/dto/channel_reliability.go`
- `relaykit/dto/channel_reliability_test.go`
- `service/channel_select.go`
- `service/hedge_billing.go`
- `service/hedge_billing_test.go`
- `service/quota.go`
- `service/text_quota.go`
- `setting/config/config.go`
- `setting/operation_setting/general_setting.go`
- `setting/operation_setting/hedge_setting_test.go`
- `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx`
- `web/src/features/channels/components/drawers/sections/__tests__/channel-reliability-fields.test.tsx`
- `web/src/features/channels/components/drawers/sections/channel-reliability-fields.tsx`
- `web/src/features/channels/lib/__tests__/channel-reliability-form.test.ts`
- `web/src/features/channels/lib/channel-form-errors.ts`
- `web/src/features/channels/lib/channel-form.ts`
- `web/src/features/channels/types.ts`
- `web/src/features/models/components/drawers/model-mutate-drawer.tsx`
- `web/src/features/system-settings/models/__tests__/hedge-loser-billing.test.tsx`
- `web/src/features/system-settings/models/index.tsx`
- `web/src/features/system-settings/models/routing-reliability-section.tsx`
- `web/src/features/system-settings/models/section-registry.tsx`
- `web/src/features/system-settings/types.ts`
- `web/src/features/usage-logs/components/__tests__/cost-display.test.tsx`
- `web/src/features/usage-logs/components/__tests__/hedge-billing-details.test.tsx`
- `web/src/features/usage-logs/components/__tests__/route-history-timeline.test.tsx`
- `web/src/features/usage-logs/components/columns/common-logs-columns.tsx`
- `web/src/features/usage-logs/components/dialogs/details-dialog.tsx`
- `web/src/features/usage-logs/components/hedge-attempt-status.tsx`
- `web/src/features/usage-logs/components/log-cost-display.tsx`
- `web/src/features/usage-logs/components/route-history-timeline.tsx`
- `web/src/features/usage-logs/components/usage-logs-mobile-card.tsx`
- `web/src/features/usage-logs/types.ts`
- `web/src/i18n/locales/zh.json`
- `web/src/i18n/static-keys.ts`

## Excluded existing changes

- `.trellis/tasks/09-10-admin-channel-availability/research/channel-cache-rate.md`
- `.trellis/tasks/09-10-group-monitor-real-traffic/research/image-resolution-billing.md`
- `.trellis/tasks/09-13-image-resolution-pricing/research/keli-log-verification.md`
- `router/web-router.go`
- `router/web_router_cache_test.go`
- `web/src/lib/http-client.ts`
