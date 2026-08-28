# Implementation File Allowlist

This task intentionally touched the following product areas. The checkout also contains unrelated user and parallel work; do not stage by directory or with broad Git commands.

## Backend

- `constant/billing.go`, `constant/context_key.go`
- `model/channel.go`, `model/channel_cache.go`, `model/option.go`, `model/task.go`
- `setting/ratio_setting/group_ratio.go`, `types/price_data.go`
- `middleware/distributor.go`, `relay/common/relay_info.go`, `relay/helper/price.go`, `relay/relay_task.go`
- `service/group.go`, `service/log_info_generate.go`, `service/quota.go`, `service/task_billing.go`, `service/tiered_settle.go`
- `controller/channel.go`, `controller/channel_authz.go`, `controller/group.go`, `controller/pricing.go`, `controller/relay.go`
- `router/api-router.go`, `docs/openapi/api.json`

## Backend tests

- `setting/ratio_setting/group_ratio_test.go`
- `model/channel_cost_ratio_test.go`, `model/user_group_ratio_migration_test.go`
- `relay/helper/channel_ratio_test.go`, `service/route_billing_test.go`
- `service/billing_ratio_log_test.go`
- `controller/group_ratio_display_test.go`, `controller/channel_authz_test.go`
- `model/log_format_test.go` covers historical billing-factor scrubbing for user logs.
- `service/task_billing_test.go` contains a concurrently added snapshot regression that aligns with this task.

## Frontend

- Channel schema, form transforms, drawer, and channel cost-ratio tests under `web/src/features/channels/`.
- Group-pricing defaults, types, forms, visual editor, and section registry under `web/src/features/system-settings/`.
- Structured ratio API contract in `web/src/lib/api.ts` and display behavior under `web/src/features/keys/`.
- User usage-log views under `web/src/features/usage-logs/` show only the final composed ratio.
- `web/src/i18n/locales/zh.json` and its generated sync report. The current checkout supports only `zhCN`; deleted locale files from parallel work were not restored.

## Removed upstream ratio-sync files

- `controller/ratio_sync.go`
- `relaykit/dto/ratio_sync.go`
- `web/src/features/system-settings/models/{upstream-ratio-sync.tsx,upstream-ratio-sync-table.tsx,upstream-ratio-sync-columns.tsx,upstream-ratio-sync-helpers.ts,channel-selector-dialog.tsx,constants.ts}`
