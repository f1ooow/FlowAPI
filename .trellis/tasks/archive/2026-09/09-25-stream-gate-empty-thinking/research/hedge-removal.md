# Research: hedge racing removal inventory (sub-item C)

- **Query**: Complete removal inventory for the provider hedge racing feature (`b9692d6f4`, deployed 09-18, never fired), with file:line and DELETE / EDIT / KEEP per item; which contracts must survive; prod data compatibility; coordination with sub-item B.
- **Scope**: internal (code, git history, specs). No prod queries were run; prod facts are quoted from `research/hedge-config.md`.
- **Date**: 2026-09-25

## Summary

| Area | DELETE (whole file) | EDIT | KEEP (touched by hedge commit, not hedge-owned) |
|---|---|---|---|
| Backend Go (controller, relay, service, model, common) | 10 | 14 | 1 (`logger/logger.go`) |
| Settings (setting/, relaykit DTO) | 1 | 4 | `setting/config/config.go:291-292` lock (optional) |
| Frontend (`web/`, incl. i18n) | 3 | 22 | `routing-reliability-section.tsx:333` save-failure return |
| Specs / task docs | 1 (`streaming-provider-hedge.md`, after migrating survivors) | 4 specs + task `prd.md`, `design.md`, `implement.jsonl`, `check.jsonl` | archive + journal |
| Database / migrations | 0 | 0 | none exist |

Baseline facts:

- The hedge feature is entirely from `b9692d6f4` (71 files). No later commit on `main` (`98f89cd0a..f3f7a8129`) changed hedge code. The deployed branch `codex/flowapi-provider-coverage` (`8156149df`, now `1f5bb7309`) adds no hedge code (`git diff f3f7a8129 1f5bb7309 | grep -i hedge` only hits the spec index row).
- **Line numbers below are from the working tree on 2026-09-25 about 20:30 CST.** They include uncommitted sub-item A edits (`relay/helper/stream_gate.go`, `stream_scanner.go`, `stream_scanner_test.go`, two spec files) and in-progress sub-item B edits (`relaykit/dto/channel_settings.go`, `model/channel.go`, `relay/channel/api_request.go`, several tests). They will shift when A and B land.
- `git revert b9692d6f4` would also remove things that must stay: per-channel idle and non-stream timeouts (`streamIdleReader`, `deadlineResponseBody`, DTO fields and bounds, their form fields and i18n keys), the `logger` atomic fix, the config lock, and the settings save-failure handling. It would also conflict with the archive move `98f89cd0a` and with A/B. The removal has to be done surgically.

---

## 1. Backend control flow

| # | File:line | Item | Action | Notes |
|---|---|---|---|---|
| 1.1 | `controller/relay_hedge.go:1-590` | `hedgeChannelAllowed` :37, `hedgeEligible` :51, `hedgeReplaySafe` :84, `hedgeWorker` :129, `relayWithHedge` :160, `detachHedgeLoser` :484, `cloneHedgeContext` :504, `hedgeResponseWriter` :526-590, `hedgeDrainTimeout`/`hedgeDrainSlots` :33-35 | DELETE file | The only identifiers used outside the file are `hedgeEligible` and `relayWithHedge` (`controller/relay.go:164-165`) |
| 1.2 | `controller/relay_hedge_test.go` (4 tests) | | DELETE file | B edited :73 (`Setting` to `OtherSettings`); that edit becomes moot |
| 1.3 | `controller/relay_hedge_policy_test.go` (11 tests plus fixture `hedgePolicyTransport`/`hedgePolicyFixture`) | | DELETE file | B edited :153. No fixture type is used by other tests. The shared helper `setupModelListControllerTestDB` lives in `controller/model_list_test.go` (KEEP) |
| 1.4 | `controller/relay.go:164-167` | `if initial, _ := model.CacheGetChannel(...); hedgeEligible(...) { newAPIError = relayWithHedge(...); return }` | EDIT (delete 4 lines) | `model`, `common`, `constant`, `tokens` and `meta` are still used right after (:169, :219) |
| 1.5 | `relay/common/hedge.go:1-52` | `HedgeAttempt`, `ObservePrefix`, `LogInfo` | DELETE file | |
| 1.6 | `relay/common/relay_info.go:177` | `Hedge *HedgeAttempt` | EDIT (delete line) | Its only users are the items in this document |
| 1.7 | `relay/helper/hedge_usage.go:1-175` | `validHedgePrefix`, `hasReportedHedgeUsage`, `observeHedgeUsage` | DELETE file | |
| 1.8 | `relay/helper/hedge_usage_test.go` (7 tests) | | DELETE file | |
| 1.9 | `relay/helper/stream_scanner.go:111-113` | `toolCollector` and its comment | EDIT delete | Read only at :385 and :530 |
| 1.10 | `stream_scanner.go:149-151` | `if info.Hedge != nil { scanner.Buffer(..., 1<<20) }` (1 MB cap) | EDIT delete | |
| 1.11 | `stream_scanner.go:317-325` | `info.Hedge.Commit()` offer inside `commitBufferedStream` | EDIT delete | |
| 1.12 | `stream_scanner.go:367-401` | Hedge block: `ObservePrefix` :370/:373, `observeHedgeUsage` :375, Responses function-call counting :376-387, `Detached` handling incl. `invalid_losing_stream` :388-400 | EDIT delete | |
| 1.13 | `stream_scanner.go:444-451` | Sub-item A R5 special case: `capReached && info.Hedge != nil` gives `prebuffer_overflow` | EDIT delete | `capReached` (:443) is still used at :462. After this, every attempt commits at the cap (PRD C4). The string `prebuffer_overflow` then no longer appears in production code (only in the deleted test at `stream_scanner_test.go:617`) |
| 1.14 | `stream_scanner.go:477-479` | Skip dispatch when `Detached` | EDIT delete | |
| 1.15 | `stream_scanner.go:529-531` | `info.Hedge.ReportedToolUsage = ...` | EDIT delete | |
| 1.16 | `stream_scanner.go:17` | import `relaykit/dto` | EDIT delete | Its only use is :384 (hedge block). `common` (:170, :296) and `strings` (:363, :476) stay in use |
| 1.17 | `relay/helper/stream_gate.go:10-12` | A's comment "only a hedge candidate ... fails instead" | EDIT | |
| 1.18 | `stream_gate.go:194-200` | A's comment "(or fail a hedge candidate)" in `isStreamKeepAlive` doc | EDIT | Every function in `stream_gate.go` keeps a caller in `stream_scanner.go` |
| 1.19 | `relay/helper/stream_scanner_test.go:500`, `:513` | `runHeldGatedStream(..., hedge *relaycommon.HedgeAttempt, ...)` and `Hedge: hedge` | EDIT | |
| 1.20 | `stream_scanner_test.go:532-536`, `:570-581` | Comment "serial attempt and a hedge candidate alike"; subtest `"hedge candidate"` | EDIT (delete subtest) | |
| 1.21 | `stream_scanner_test.go:564`, `:594` | `nil` hedge argument | EDIT | |
| 1.22 | `stream_scanner_test.go:603-621` | `TestStreamScannerHandlerWithGate_HedgeCandidateFailsAtPrefixCap` | EDIT (delete test) | `sync/atomic` is still used (:98, :113 and more) |
| 1.23 | `relay/channel/api_request.go:605-607` | `info.Hedge.Dispatched()` | EDIT delete | |
| 1.24 | `relay/channel/api_request.go:625-627` | Wrap `resp.Body` in `hedgeResponseBody` | EDIT delete | The non-stream deadline (:549-561, :609-624) stays |
| 1.25 | `relay/channel/deadline_body.go:15-32`, imports `fmt` :5 and `relaycommon` :6 | `maxHedgeDrainBytes`, `hedgeResponseBody` | EDIT | `deadlineResponseBody` :10-13 and :34-37 stay |
| 1.26 | `relay/channel/deadline_body_test.go:48-58`, import `assert` :10 | `TestHedgeDrainByteLimitAppliesOnlyAfterDetachment` | EDIT | `assert` is used only at :54. The two non-stream deadline tests (:20, :72) stay |
| 1.27 | `relay/channel/<provider>/**` | | none | No adaptor subpackage references `Hedge` |
| 1.28 | `common/body_storage.go:32-68` | `ForkBodyStorage`, `bodyStorageView` | EDIT delete | Only caller is `relay_hedge.go:191`. `fmt` and `os` are still used elsewhere in the file |
| 1.29 | `common/body_storage_fork_test.go` | `TestForkBodyStorageOwnsCursorAndSurvivesParentClose` | DELETE file | Contains no "hedge" string; it tests the hedge-only helper |
| 1.30 | `service/channel_select.go:20`, `:188-190` | `RetryParam.CandidateAllowed` (added by the hedge commit) and its closure | EDIT: remove the field; closure back to `return channel.Status == common.ChannelStatusEnabled` | Only setters are `relay_hedge.go:383` and `:458`. `model.ChannelSelectionOptions.CandidateAllowed` (`model/channel_resilient_select.go:15`, from `845a85998`) stays |

## 2. Billing, logging and metrics

| # | File:line | Item | Action | Notes |
|---|---|---|---|---|
| 2.1 | `service/hedge_billing.go:1-51` | `FinalizeHedgeAttempt` | DELETE file | Only caller is `relay_hedge.go:330` |
| 2.2 | `service/hedge_billing_test.go` | | DELETE file | |
| 2.3 | `service/quota.go:284-289` | `PostAudioConsumeQuota` usage capture and early return | EDIT delete | |
| 2.4 | `service/quota.go:357-361` | Loser uses `UpdateUserUsedQuota` (no request count) | EDIT: back to `model.UpdateUserUsedQuotaAndRequestCount(relayInfo.UserId, quota)` | `model.UpdateUserUsedQuota` predates hedge (used by task billing, midjourney). KEEP the function |
| 2.5 | `service/quota.go:367-369` | `Hedge.SettlementStatus = "failed"` | EDIT delete | |
| 2.6 | `service/quota.go:382-384` | `other["hedge"] = relayInfo.Hedge.LogInfo()` | EDIT delete | |
| 2.7 | `service/quota.go:385-393`, `:409-411` | `recordMetrics` closure (winner-only success sample, synchronous for hedge) | EDIT: pre-hedge form is `gopool.Go(func(){ perfmetrics.RecordRelaySample(relayInfo, true, ...) })` after `RecordConsumeLog` | Same behavior as today when `Hedge == nil` |
| 2.8 | `service/text_quota.go:435-439` | `PostTextConsumeQuota` capture and early return | EDIT delete | |
| 2.9 | `service/text_quota.go:445` | Affinity cache observation limited to the winner | EDIT: `if originUsage != nil {` | |
| 2.10 | `service/text_quota.go:451-458` | `billableAttempt` and zeroing of quota and tool surcharges | EDIT delete | |
| 2.11 | `service/text_quota.go:462` | `&& billableAttempt` | EDIT | |
| 2.12 | `service/text_quota.go:495-497` | Winner with no usage gets request count +1 at 0 quota | EDIT delete | |
| 2.13 | `service/text_quota.go:500-504` | Loser without request count | EDIT: back to `UpdateUserUsedQuotaAndRequestCount` | |
| 2.14 | `service/text_quota.go:510-514` | `SettlementStatus` `failed` / `not_billed` | EDIT delete (keep the `LogError`) | |
| 2.15 | `service/text_quota.go:587-589` | `other["hedge"]` | EDIT delete | |
| 2.16 | `service/text_quota.go:598-610`, `:626-628` | Hedge gating of `recordCacheSample`; `recordMetrics` closure | EDIT: back to the pre-hedge async block after `RecordConsumeLog` (`recordCacheSample := originUsage != nil`) | Same behavior as today when `Hedge == nil` |
| 2.17 | `controller/relay_hedge.go:296, 325, 350, 354, 358, 372, 385, 390` | Route-history outcomes `hedge_launched`, `hedge_loser`, `hedge_winner`, `hedge_draining`, `hedge_cancelled`, `hedge_threshold`, `hedge_skipped`. Reasons `first_content`, `loser_billing_policy`, `first_content_timeout`, `no_eligible_backup`, `backup_admission_failed` | Removed with 1.1 | String literals only. `service/route_state.go:5-11` has no hedge constants (KEEP) |
| 2.18 | `controller/relay_hedge.go:413-416`, `:424`, `:446` | Per-attempt `perfmetrics.RecordChannelAttempt`, `RecordAutoBanSuccess/Failure` for hedge workers | Removed with 1.1 | `pkg/perf_metrics` has no hedge code. Loser exclusion lived only at the call sites in 2.7, 2.16 and 1.1 |
| 2.19 | `model/log.go:775-779`, `:826-855` | `SumUsedQuota` RPM excludes rows whose `other.hedge.role == "loser"` (`hedgeMetadataPattern`, `CASE WHEN`, Go-side row scan) | EDIT: back to `count(*) rpm` (pre-hedge) | Prod has 0 `other` rows containing `"hedge"`, so results are identical. The `gorm` import is still used (:19, :620, :718) |
| 2.20 | `model/log.go:148-152` | `clickHouseLogOrder` tie-breakers `channel_id desc, upstream_request_id desc, other desc` and the hedge comment | **Decision**: EDIT (revert to `created_at desc, request_id desc`) or KEEP (update the comment) | Callers :288, :660, :753 (ClickHouse only). The tie-breakers also stabilize paging of non-hedge rows that share a timestamp and request ID |
| 2.21 | `model/log_hedge_stats_test.go` | `setupHedgeLogDB` and 3 `SumUsedQuota` tests | DELETE file (see decision in Caveats) | These are the only `SumUsedQuota` tests in the repo. `TestSumUsedQuotaHedgeAttempts` :85-95 also covers the user, channel, model, token, group and time-window filters |
| 2.22 | `model/clickhouse_log_test.go:99-121` | `TestClickHouseLogOrder` calls `setupHedgeLogDB` and inserts `other.hedge` rows | EDIT (compile dependency on 2.21) | The pre-hedge version is two `assert.Equal` string checks (`git show b9692d6f4^:model/clickhouse_log_test.go`). `require` stays in use |
| 2.23 | `logger/logger.go:11, 30, 32, 45, 113-114` | `atomic.Int64` log count, `atomic.Bool` setup flag (hedge commit) | KEEP | Generic data-race fix with no hedge semantics |

## 3. Settings

| # | File:line | Item | Action | Notes |
|---|---|---|---|---|
| 3.1 | `setting/operation_setting/general_setting.go:14`, `:28` | `BillHedgeLosers` field and default `true` | EDIT delete | |
| 3.2 | `setting/operation_setting/general_setting.go:46-51` | `SnapshotGeneralSetting()` | EDIT delete | Callers: `relay_hedge.go:161`, `relay_hedge_policy_test.go:125`, `hedge_setting_test.go` |
| 3.3 | `setting/config/config.go:41-48` | `ConfigManager.Snapshot` | EDIT delete | Its only caller is 3.2. `reflect` is still used (25 occurrences) |
| 3.4 | `setting/config/config.go:291-292` | `UpdateConfigFromMap` now takes `GlobalConfig.mutex` (hedge commit) | KEEP (optional revert) | Generic serialization with `LoadFromDB`, `Get` and `ExportAllConfigs`. Callers: `model/option.go:447`, `:714` |
| 3.5 | `setting/operation_setting/hedge_setting_test.go` | `TestBillHedgeLosersDefaultsAndSnapshot` | DELETE file | |
| 3.6 | Load and save of `general_setting.bill_hedge_losers` | Generic only: `config.GlobalConfig.Register` (`general_setting.go:39`), `LoadFromDB`, `ExportAllConfigs`, `PUT /api/option/` via `model/option.go:700-714` | none | No key-specific backend code. After removal an old option row is ignored (`updateConfigFromMap` iterates struct fields, `setting/config/config.go:200-205`) |
| 3.7 | `relaykit/dto/channel_reliability.go:15` | `FirstContentTimeoutSeconds *int` | EDIT delete | Requires `cd relaykit && GOWORK=off go build ./...` |
| 3.8 | `relaykit/dto/channel_reliability.go:51` | Bound `{"first_content_timeout_seconds", ..., 1, 180}` | EDIT delete | The loop keeps two entries |
| 3.9 | `relaykit/dto/channel_reliability.go:23-43` | `DefaultChannelReliabilitySettings` / `WithDefaults` | none | They never set a first-content default |
| 3.10 | `relaykit/dto/channel_reliability_test.go:34-39` | First-content bound cases | EDIT delete | |
| 3.11 | Backend readers of `FirstContentTimeoutSeconds` | `controller/relay_hedge.go:65-66` (eligibility), `:275-280` (threshold timer) | Removed with 1.1 | No other reader in Go (grep) |
| 3.12 | Server validation call site | `model/channel.go:1290-1294` (B: `settings.reliability` then `Validate()`) | none | Bounds change comes from 3.8 |

### 3.13 `streaming_idle_timeout_seconds` / `non_streaming_timeout_seconds` are independent of racing (confirmed)

| Timeout | Backend path (no `Hedge` reference) | Tests that stay | Frontend (stays) |
|---|---|---|---|
| Streaming idle | `relay/helper/stream_scanner.go:91-103` `streamIdleReader`; `:124-139` timer from `info.ChannelOtherSettings.Reliability` (B, :125); `:514-519` `idle_timeout` precommit failure / `StreamEndReasonTimeout` | `stream_scanner_test.go:951` `TestStreamScannerHandler_StreamStatus_Timeout`; `relaykit/dto/channel_reliability_test.go:20-33`, `:40-42` | `channel-reliability-fields.tsx:82-88`; `channel-form.ts:299-306`, `:491`, `:562`, `:595-596`, `:668`, `:846`; drawer `:339`, `:737-739`, `:1010`; errors `:61`; types `:122` |
| Non-stream total | `relay/channel/api_request.go:549-561`, `:609-624`; `deadline_body.go:10-13`, `:34-37` `deadlineResponseBody` | `deadline_body_test.go:20`, `:72`; `channel_reliability_test.go:43-48` | `channel-reliability-fields.tsx:89-95`; `channel-form.ts:307-314`, `:492`, `:563`, `:597-598`, `:669`, `:847`; drawer `:340`, `:740-742`, `:1011`; errors `:62`; types `:123` |

Facts to keep in mind (no change proposed here):

- Both timeouts arrived in the same commit as racing (`b9692d6f4`). That commit also replaced the scanner's global idle timer. Before, it used `constant.StreamingTimeout` (env `STREAMING_TIMEOUT`, default 300 s, `common/init.go:182`), reset on every line (`git show b9692d6f4^:relay/helper/stream_scanner.go:105-110,307`). Now it uses only the per-channel value, and missing or 0 means no idle timer (`stream_scanner.go:124-134`).
- `constant.StreamingTimeout` is now read by no production code. Only Gemini tests still set it (`relay/channel/gemini/relay_responses_test.go:92-94`, `relay_gemini_usage_test.go:75-78`).
- Prod has `streaming_idle_timeout_seconds = 0` on all 12 channels that carry the key (`hedge-config.md` §6.1), and `RELAY_TIMEOUT` defaults to 0 (`common/init.go:111`). So a stalled stream currently has no FlowAPI idle bound unless a channel sets 60–600 s. Removing racing does not change this.

## 4. Frontend (`web/src`)

### 4.1 Channel form: first-content threshold

| File:line | Item | Action |
|---|---|---|
| `features/channels/components/drawers/sections/channel-reliability-fields.tsx:75-81` | `TIMEOUT_FIELDS[0]` (`first_content_timeout_seconds`, label and description) | EDIT delete |
| `features/channels/lib/channel-form.ts:293-298` | zod rule (message `Enter 0 or a whole number from 1 to 180.`) | EDIT delete |
| `features/channels/lib/channel-form.ts:490` | Default `first_content_timeout_seconds: 0` | EDIT delete |
| `features/channels/lib/channel-form.ts:561`, `:593-594`, `:667` | Load from `settings.reliability` into form values | EDIT delete |
| `features/channels/lib/channel-form.ts:845` | Write into `settingsObj.reliability` | EDIT delete (stale-key decision in §7) |
| `features/channels/lib/channel-form-errors.ts:60` | Advanced-settings error field | EDIT delete |
| `features/channels/types.ts:121` | `ChannelReliabilitySettings.first_content_timeout_seconds?` | EDIT delete |
| `features/channels/components/drawers/channel-mutate-drawer.tsx:338`, `:734-736`, `:1009` | "Advanced settings configured" checks and `form.watch` | EDIT delete |
| `features/channels/components/drawers/sections/__tests__/channel-reliability-fields.test.tsx:60-62` | Asserts the first-content label | EDIT |
| `features/channels/lib/__tests__/channel-reliability-form.test.ts:49`, `:62`, `:84`, `:92-95`, `:140` | First-content in load/preserve, zero-persist, bounds and serialize tests | EDIT |

### 4.2 Loser-billing toggle

| File:line | Item | Action |
|---|---|---|
| `features/system-settings/models/routing-reliability-section.tsx:80` | zod `general_setting: z.object({ bill_hedge_losers })` | EDIT delete |
| `routing-reliability-section.tsx:140`, `:160` | Prop and normalized types | EDIT delete |
| `routing-reliability-section.tsx:184-186`, `:212-213`, `:241` | `buildFormDefaults`, `normalizeDefaults`, `normalizeFormValues` | EDIT delete |
| `routing-reliability-section.tsx:351-372`, `:374` | Switch `FormField` and the `<Separator />` the hedge commit added after it | EDIT delete (other `Switch`/`Separator`/`SettingsSwitch*` uses remain, so imports stay) |
| `routing-reliability-section.tsx:333` | `if (!result.success) return` (hedge commit, generic) | KEEP |
| `features/system-settings/models/section-registry.tsx:77-78` | Pass-through of the option | EDIT delete |
| `features/system-settings/models/index.tsx:69` | Default `true` | EDIT delete |
| `features/system-settings/types.ts:197` | `ModelSettings` key | EDIT delete |
| `features/models/components/drawers/model-mutate-drawer.tsx:331` | `ModelSettings` literal key | EDIT delete (required once 4.2 types.ts is removed) |
| `features/system-settings/models/__tests__/hedge-loser-billing.test.tsx` | 2 tests | DELETE file. Its second test is the only test of the :333 rejected-save retry |

### 4.3 Usage-log UI for `other.hedge` and `hedge_*`

| File:line | Item | Action |
|---|---|---|
| `features/usage-logs/components/hedge-attempt-status.tsx` | `HedgeAttemptStatus`, `HedgeMeteringStatus` | DELETE file |
| `features/usage-logs/components/columns/common-logs-columns.tsx:61`, `:280`, `:287-297` | Import; hedge badge instead of type badge | EDIT (restore the unconditional `StatusBadge`; `parseLogOther` is still used at :324+) |
| `features/usage-logs/components/usage-logs-mobile-card.tsx:49`, `:50`, `:160`, `:164`, `:177-196`, `:347` | `LogOtherData` import (hedge only), component import, `hedge` prop, badge branch | EDIT |
| `features/usage-logs/components/log-cost-display.tsx:35`, `:122`, `:125-138`, `:144`, `:158` | Import, `t` (used only by the hedge branch in `LogCostDisplay`), non-settled branch, metering labels | EDIT. `:151` `flex-wrap` was also added by the hedge commit (cosmetic, optional) |
| `features/usage-logs/components/dialogs/details-dialog.tsx:86-90` | Imports `HedgeAttemptStatus`, `HedgeMeteringStatus`, `LogCostDisplay` (all hedge only) | EDIT delete |
| `details-dialog.tsx:396` | `Attempt charge` vs `Total Cost` | EDIT (always `Total Cost`) |
| `details-dialog.tsx:497-503`, `:1131` | `showBillingBreakdown` | EDIT (pre-hedge: `isTieredBilling = isConsume && !isViolation && ...`; `{isConsume && other && !isViolation && (`) |
| `details-dialog.tsx:634-643`, `:659-683` | Title badge branch; "Provider racing" section | EDIT delete |
| `features/usage-logs/components/route-history-timeline.tsx:41-79`, `:127-129`, `:200-209` | 6 `hedge_*` cases, 3 reason labels, hedge colour branches | EDIT delete. After removal an unknown outcome renders its raw string via `default` (:101-107) |
| `features/usage-logs/types.ts:128-135` | `LogOtherData.hedge` | EDIT delete |
| `features/usage-logs/components/__tests__/hedge-billing-details.test.tsx` | | DELETE file |
| `features/usage-logs/components/__tests__/cost-display.test.tsx:26-27`, `:41-134` | Hedge imports, `losingAttempt` and 5 tests | EDIT (keep :135-178; `formatLogQuota`/`normalizedText` are still used) |
| `features/usage-logs/components/__tests__/route-history-timeline.test.tsx:24-60` | Racing-states test | EDIT delete |

### 4.4 i18n

Only `web/src/i18n/locales/zh.json` exists (plus the empty `_extras/` and `_reports/`). All locations below were checked by grepping `web/src` for each literal key outside the hedge-owned locations.

| File | Hedge-only keys (DELETE) | Keys that stay |
|---|---|---|
| `web/src/i18n/locales/zh.json` | 25 keys at lines 108 (`0: racing disabled. ...`), 510 (`Attempt`), 512 (`Attempt charge`), 513 (`Attempt ID`), 620 (`Background metering`), 679 (`Bill provider racing losers`), 859 (`Charges with the same request ID ...`), 1778 (`Enter 0 or a whole number from 1 to 180.`), 1878 (`Estimated usage`), 1936 (`Extra racing attempt`), 2173 (`First content received`), 2175 (`First-content threshold reached`), 2774 (`Loser billing policy`), 2869 (`Metered usage`), 3306 (`Not billed`), 3308 (`Not charged`), 3532 (`Partial usage`), 3863 (`Provider racing`), 3923-3925 (`Racing attempt cancelled` / `started` / `Racing winner`), 4531 (`Settlement failed`), 4685 (`Streaming first-content threshold (seconds)`), 5324 (`Usage unknown`), 5587 (`When enabled, only losing attempts ...`) | `Request timeouts`, `Streaming idle timeout (seconds)`, `Non-streaming total timeout (seconds)`, the two `0: ... timer disabled ...` descriptions, `Enter 0 or a whole number from 60 to 600.` / `... 60 to 1800.`, `Usage` (also used by `common-logs-stats.tsx`, `dashboard/lib/charts.ts`) |
| `web/src/i18n/static-keys.ts` | :595-603 and :608 (10 entries) | :604-607, :609-610 (idle/non-stream label, descriptions, messages) |

## 5. Database and migrations

None. The hedge commit added no table, column, index or migration: it did not touch `model/main.go` or any ClickHouse DDL. The only `model/log.go` changes are query-side (2.19, 2.20). Hedge state lived only in JSON:

- `logs.other.hedge`
- `logs.other.admin_info.route_history[].outcome = hedge_*`
- `channels.settings.reliability.first_content_timeout_seconds`
- the option key `general_setting.bill_hedge_losers`

## 6. Specs and docs

### 6.1 `.trellis/spec/backend/streaming-provider-hedge.md`: contract map

| Lines | Contract | Hedge only? | Surviving home today |
|---|---|---|---|
| 5-7 | Scope trigger "channel timeouts" | Partly | Needs a new home (no spec covers channel reliability settings; grep for `max_attempts\|auto_ban\|reliability` in `.trellis/spec/` hits only these two files) |
| 11-17 | `settings.reliability` stores optional integer seconds; bounds table | Row `first_content_timeout_seconds` (1-180) only | **Must survive**: storage location, `streaming_idle_timeout_seconds` 0 or 60–600, `non_streaming_timeout_seconds` 0 or 60–1800 |
| 19-21 | `*int` + `omitempty`; missing and 0 disable; UI shows and saves 0; transport limits independent | No | **Must survive** |
| 23-25 | `general_setting.bill_hedge_losers` | Yes | DELETE |
| 27-32 | Public `other.hedge` fields, `not_billed`, `settled` display | Yes | DELETE |
| 36-45 | Threshold launch, single committer, prefix vs winner, loser drain | Yes | "A committed answer never switches providers" is already in `stream-precommit-gate.md:33-36` |
| 46-48 | Startup frames by protocol structure; Gemini `response` wrapper | Mostly (prefix admission, usage observation) | Gate classifier also unwraps Gemini `response` (`stream_gate.go:137-140`); not stated in `stream-precommit-gate.md` (optional carry-over) |
| 49-73 | Attempt-owned state, additive reserve, per-attempt pricing, exactly-once finalization, unmetered losers, usage evidence, loser stats, cancellation | Yes | Client disconnect "not a channel failure" is already in `stream-precommit-gate.md:71` |
| 74-75 | Non-stream total timeout covers body reads; idle timeout ends a committed stream without replay | No | **Must survive** (the idle precommit row is already `stream-precommit-gate.md:69`) |
| 81-82 | Missing/zero timeout disabled; negative, fractional or out-of-range rejected on channel update | No | **Must survive** (B enforces this on `settings.reliability`) |
| 83-92 | Backup/loser rows, incl. A's row 88 | Yes | DELETE |
| 95 | "Base: all three timeouts are zero; no speculative backup launches" | Partly | Survives as "timeouts zero, no channel timers" |
| 96-122 | Good/bad cases, tests required, wrong vs correct | Yes | `relaykit` build rule duplicates AGENTS.md |

Contracts from sub-item B (B1, B2, B6) that also need a home:

- `settings.reliability` is the only source.
- The server validates it on save.
- It survives `SetOtherSettings`.

`max_attempts` (1–10, default 2), `auto_ban_threshold` (1–100, default 5) and `auto_ban_duration_seconds` (1–86400, default 1800) are defined only in `relaykit/dto/channel_reliability.go:5-12` and are not in any spec.

### 6.2 Other spec and doc edits

| File:line | Item | Action |
|---|---|---|
| `.trellis/spec/backend/stream-precommit-gate.md:37-38` | "for serial attempts also the moment a prefix cap is reached" | EDIT (the cap applies to every attempt) |
| `stream-precommit-gate.md:46` | "never fail a serial attempt" | EDIT (wording) |
| `stream-precommit-gate.md:49-52` | Hedge candidate `prebuffer_overflow` bullet | EDIT delete |
| `stream-precommit-gate.md:60-71` | "Hedge candidate" column; `:65` hedge cell | EDIT (drop column) |
| `stream-precommit-gate.md:94-97` | "(serial and hedge) ... hedge `Commit` is called exactly once" | EDIT |
| `stream-precommit-gate.md:98` | "(serial)" qualifier | EDIT |
| `stream-precommit-gate.md:100-101` | "Hedge candidate fails at the prefix cap" test | EDIT delete |
| `stream-precommit-gate.md:114`, `:124-134` | "serial attempt" comment; `if info.Hedge != nil` in the Correct snippet | EDIT |
| `.trellis/spec/backend/index.md:13` | Hedge spec row | EDIT delete (add a row if a new reliability spec is created) |
| `.trellis/spec/backend/billing-ratio-routing.md:54-57` | Hedge bullet (whole bullet added by `b9692d6f4`) | EDIT delete |
| `.trellis/spec/backend/global-passthrough.md:23`, `:50` | "retries and hedged attempts"; "Retry or hedge chooses another channel" (from `c99ea5f97`) | EDIT wording |
| `.trellis/tasks/09-25-stream-gate-empty-thinking/implement.jsonl:2`, `check.jsonl:3` | Point at the hedge spec | EDIT after the spec is deleted |
| Same task `prd.md:22`, `:37` (R5), `:45` (R5 acceptance), `:71`, `:99` ("三项超时"); `design.md:15`, `:16`, `:19`, `:37` | References to racing, `relay_hedge.go` lines and three timeouts | EDIT (planner) |
| `.trellis/tasks/archive/2026-09/09-17-streaming-provider-hedge/`, `.trellis/workspace/zhuyu/index.md` | History | KEEP |

No other active task mentions hedge (grep over `.trellis/tasks/` excluding archive and this task).

## 7. Prod data compatibility after removal

Stored `channels.settings.reliability.first_content_timeout_seconds` values (#10 = 60, #58 = 60, and #6, #11, #26, #45, #47, #48, #64, #66, #67, #68 = 0; `hedge-config.md` §2):

| Path | Behavior after C | Evidence |
|---|---|---|
| Read (`GetOtherSettings`, per request) | **Ignored on unmarshal.** `common.UnmarshalJsonStr` is plain `encoding/json`, so unknown keys are dropped silently. It cannot trigger the reset-to-`{}` path at `model/channel.go:1343-1354`, whatever its JSON type. (With B alone, a non-integer value would; prod values are integers) | `common/json.go:13-15` |
| Validation (`ValidateSettings`) | Ignored; never rejected | `model/channel.go:1283-1289` parses into `dto.ChannelOtherSettings` |
| Backend re-marshal (`SetOtherSettings`) | **Dropped on the next write.** Writers: advanced-custom config (`controller/channel.go:1305`); upstream model update (`controller/channel_upstream_update.go:501`, called by the periodic check, manual detect and apply) | `model/channel.go:1356-1363` |
| Admin UI save (`PUT /api/channel/`) | **Kept verbatim** unless the frontend deletes it. `buildSettingsJSON` spreads the existing `reliability` object before writing known keys (`channel-form.ts:837-841`; the unknown-key preservation is locked by `channel-reliability-form.test.ts:31-72`). The controller stores `settings` as received | `channel-form.ts:706-717`, `:640`; `hedge-config.md` §6.3 |
| Channel copy | Kept | `controller/channel.go:1484-1502` |

- **Cleanup is not needed for correctness.** Nothing reads the key after C.
- Whether it disappears depends on the path: `SetOtherSettings` drops it; UI saves keep it. Which of the 12 channels run periodic upstream model checks (and would lose the key organically) was not verified.
- Optional: an explicit delete in `buildSettingsJSON`, or a one-off JSON edit (decision; PRD C6 only requires "no effect on read and save").

Other stored data:

- `logs.other.hedge` and `hedge_*` route outcomes: **0 rows** since 09-18 (`hedge-config.md` §3; not re-queried). If any existed:
  - `SumUsedQuota` would count them in RPM like ordinary rows.
  - The UI would show the normal type badge and billing breakdown.
  - `hedge_*` outcomes would render with the timeline's default branch.
- No cleanup needed.
- Option `general_setting.bill_hedge_losers`: not persisted in prod (`hedge-config.md` §1). An existing row would be ignored by `updateConfigFromMap`.

## 8. Coordination with sub-item B

| B touch point (working tree) | B change | What C does there |
|---|---|---|
| `relaykit/dto/channel_settings.go:88-89` | `ChannelOtherSettings.Reliability` added; `ChannelSettings.Reliability` removed | Nothing |
| `relaykit/dto/channel_reliability.go:15`, `:51`; `channel_reliability_test.go:34-39` | Untouched by B (PRD B5: field kept for now) | **C deletes** (3.7, 3.8, 3.10) |
| `model/channel.go:1290-1294` `ValidateSettings`; `:1326-1332` `GetReliabilitySettings` | Read and validate `settings.reliability` | Nothing (bounds change flows from the DTO) |
| `controller/relay.go:245`, `model/channel.go:432` | Unchanged callers | Nothing |
| `controller/relay_hedge.go:65`, `:275`, `:436` | Unchanged callers | **C deletes the file** |
| `relay/helper/stream_scanner.go:125` | Idle timeout reads `ChannelOtherSettings` | **C edits the same file** (1.9-1.16) |
| `relay/channel/api_request.go:550` | Non-stream timeout reads `ChannelOtherSettings` | **C edits the same file** (:605-607, :625-627) |
| `relay/helper/stream_scanner_test.go:964` | Timeout test uses `ChannelOtherSettings` | **C edits the same file** (:497-621) |
| `relay/channel/deadline_body_test.go:33`, `:83`, `:93` | `ChannelOtherSettings` | **C deletes :48-58 and the `assert` import** |
| `controller/relay_hedge_test.go:73`, `relay_hedge_policy_test.go:153` | `Setting` to `OtherSettings` | **C deletes both files** |
| `model/channel_settings_test.go` (new tests), `controller/channel_upstream_update_test.go` (new test), `controller/relay_test.go` (new), `model/channel_auto_ban_test.go`, `service/channel_auto_ban_test.go` | None contains `first_content_timeout_seconds` | Nothing required. These are the natural place for a C6 case: stored `first_content_timeout_seconds` passes validation and is ignored or dropped |
| Task docs: PRD B goal (:71) and acceptance (:99, "三项超时"); `design.md:15-19`, `:37` | Mention first-content / racing | **C updates** |

Sequencing facts:

- A, B and C all edit `relay/helper/stream_scanner.go` and `stream_scanner_test.go`. B and C both edit `relay/channel/api_request.go` and `deadline_body_test.go`. A and B are uncommitted.
- PRD states B and C deploy together (`prd.md:111`). If B were deployed without C, #10's 60 s threshold would become live. Most #10 Claude Code traffic is still blocked by affinity, but the roughly 51 of 8355 stream requests that do not match the affinity rule would become racing-eligible, with loser billing defaulting to `true` (`hedge-config.md` §4, §6.1).

## Shared code the hedge path uses that must stay

- `RelayInfo.ForcePreConsume` (`relay_info.go:125`; async tasks `relay/relay_task.go:210`) and `DisablePing` (`:105`; Gemini `relay/channel/gemini/adaptor.go:170`)
- `model.UpdateUserUsedQuota`
- `service.RouteState` and outcomes (`service/route_state.go`)
- `model.ChannelSelectionOptions.CandidateAllowed`
- `CacheGetRandomSatisfiedChannel`, `ShouldSkipRetryAfterChannelAffinityFailure`, `ClearCurrentChannelAffinityCache`, `RecordAutoBanSuccess/Failure`, `perfmetrics.RecordChannelAttempt`, `logRouteHistory`, `supportsTransparentFailover`, `middleware.SetupContextForSelectedChannel`: all have serial or distributor callers
- All of `stream_gate.go`; `CountBillableToolCall`; `relayconvert.UsageFromGeminiMetadata` (Gemini adaptor)

## Verification

- `go build ./...`
- `cd relaykit && GOWORK=off go build ./...`
- `go vet` and `go test ./controller/... ./relay/... ./service/... ./model/... ./setting/... ./common/... -count=1`
- `go test -race ./relay/helper/... ./controller/... -count=1`
- `grep -rn -i hedge --include='*.go' .` should return nothing
- `web/`: `bun run typecheck`, `bun run lint`, `bun run test` (vitest; see `web/AGENTS.md:78,166`)
- `grep -rn -i "hedge\|racing\|first_content" web/src` should return nothing

## Caveats / Not Found / Open decisions

- **Tests with generic value that go away with hedge files** (AGENTS.md: preserve meaningful regression coverage):
  - `TestSumUsedQuotaHedgeAttempts` (only `SumUsedQuota` filter test)
  - `TestClickHouseLogOrder` paging (needs a new fixture or the pre-hedge form)
  - `hedge-loser-billing.test.tsx` second test (only test of rejected-save retry, `routing-reliability-section.tsx:333`)
- **`clickHouseLogOrder` tie-breakers**: revert or keep (2.20).
- **Stale `first_content_timeout_seconds` key**: leave it (inert; kept by UI saves, dropped by `SetOtherSettings`), or delete it explicitly in `buildSettingsJSON` or with a one-off edit.
- **Spec home**: no existing spec covers channel reliability settings. The surviving contracts (§6.1) and B's storage and validation contracts need a destination, for example `stream-precommit-gate.md` for the idle timeout or a new spec file.
- **Idle-timeout default**: since `b9692d6f4`, `STREAMING_TIMEOUT` is dead config and missing or 0 per-channel idle timeout means no idle timer. All prod channels are 0 or missing. This is out of C's stated scope but affects PRD C background claim `prd.md:127`.
- **Merge overlap with the deployed branch** `codex/flowapi-provider-coverage` (not merged into `main`):
  - It changes `controller/relay.go` (hunks at 210, 478, 664+; not at 164), `relay/common/relay_info.go` (hunks at 70, 199; not at 177), `relaykit/dto/channel_settings.go` and `model/channel.go` (B's files).
  - Frontend: `channel-form.ts` (hunks at 252, 315, 454, 551, 577, 639, 797, near the first-content lines), `channels/types.ts:93`, `details-dialog.tsx` (61, 237), `usage-logs/types.ts:200`, `static-keys.ts` (19, 310) and `zh.json` (+20).
  - Backend hedge files (`relay/helper`, `api_request.go`, `deadline_body.go`, `quota.go`, `text_quota.go`, `model/log.go`, `setting/`, `relay_hedge.go`) are not touched by that branch.
- Prod counts (0 hedge rows, option not persisted, 12 channels with the key) come from `hedge-config.md` (queried 18:40–19:30 CST) and were not re-queried.
- Line numbers are a working-tree snapshot and will move as A and B land.
