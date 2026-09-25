# Research: hedge racing config (where it is stored, prod state, why it never fired)

- **Query**: The user says racing is enabled (confirmed: #10 ZIVV CCMAX). Earlier checks found 0/52 channels with `reliability`/`first_content_timeout` in `channels.setting` and no hedge launch in the app logs. Which is true, and why has racing never fired?
- **Scope**: mixed. Code trace plus read-only prod checks on `ssh hk` (psql on `flowapi-postgres`, logs in container `flowapi`, plus two backup dumps streamed to stdout; no temp files written)
- **Date**: 2026-09-25 (prod queried about 18:40–18:50 CST)

## Verdict

**Case: saved but never read.**

- #10 ZIVV CCMAX has `first_content_timeout_seconds = 60`. It is saved in **`channels.settings`** (Go `Channel.OtherSettings`, JSON key `settings`), at `reliability.first_content_timeout_seconds`. #58 Rolldek image2.5 also has 60. Every other channel is 0 or has no value.
- The backend reads reliability only from **`channels.setting`** (singular; Go `Channel.Setting`), through `GetSetting()` into `dto.ChannelSettings.Reliability`. That column has no `reliability` key on any of the 52 prod channels. So `GetReliabilitySettings()` returns defaults, `FirstContentTimeoutSeconds` is `nil`, and `hedgeEligible` returns false at `controller/relay_hedge.go:65-67` for every request on every channel.
- The earlier 0/52 check was correct for `setting`. It never looked at `settings`. The admin form showing 60 is correct too: it loads the value from `settings`.
- Racing never fired. Since the 09-18 deploy there are 0 `hedge attempt … launched` lines in the app logs and 0 `other.hedge` or `hedge_*` markers in `logs`.
- #10's Claude Code traffic has **two more independent blockers**, both caused by the default channel-affinity rule `claude cli trace`: `ShouldSkipRetryAfterChannelAffinityFailure` (`controller/relay_hedge.go:56`) and the affinity param-override template (`:62-64`).

## 1. Where the setting lives: UI → API → model → reader

### Write path (frontend puts reliability in `settings`)

| Step | Location | What happens |
|---|---|---|
| Form field | `web/src/features/channels/components/drawers/sections/channel-reliability-fields.tsx:74-80` | `first_content_timeout_seconds`, label "Streaming first-content threshold (seconds)" (zh `流式首内容阈值（秒）`, under `Request timeouts`/`请求超时`) |
| Validation (UI only) | `web/src/features/channels/lib/channel-form.ts:293-298` | zod integer from 0 to 180 |
| Load into form | `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx:662-665` (`GET /api/channel/:id`, `web/src/features/channels/api.ts:103-106`), then `:1235-1238` calls `transformChannelToFormDefaults(channelData.data)` | Loads from the backend, not from a local draft |
| Parse | `channel-form.ts:561` (default 0), `:565-598` | `JSON.parse(channel.settings)`, then `parsed.reliability.first_content_timeout_seconds ?? 0` (`:592-594`) |
| Frontend types | `web/src/features/channels/types.ts:84-93` (`ChannelSettings`, i.e. the `setting` string, has **no** `reliability`), `:95-115` (`ChannelOtherSettings`, i.e. the `settings` string, has `reliability` at `:114`) | Frontend models reliability as part of `settings` |
| Build `setting` | `channel-form.ts:677-701` `buildSettingJSON` | force_format / thinking_to_content / proxy / pass_through_body_enabled / system_prompt / system_prompt_override / http_* only. **No reliability** |
| Build `settings` | `channel-form.ts:706-851` `buildSettingsJSON`; reliability block at `:837-848` | `settingsObj.reliability = {max_attempts, auto_ban_threshold, auto_ban_duration_seconds, first_content_timeout_seconds, streaming_idle_timeout_seconds, non_streaming_timeout_seconds}` |
| Payload | `channel-form.ts:915-967` (`setting: buildSettingJSON` `:936`, `settings: buildSettingsJSON` `:939`); called from `web/src/features/channels/hooks/use-channel-mutate-form.ts:95-98` | `PUT /api/channel/` (`api.ts:141-151`) |
| Controller | `controller/channel.go:988` `UpdateChannel`: `common.Unmarshal(rawBody, &channel)` `:995`, `validateChannel` `:1011`, `channel.Update()` `:1133` | JSON `settings` goes to `Channel.OtherSettings`, JSON `setting` to `Channel.Setting`. Both are saved as sent |
| Server validation | `controller/channel.go:509-520` → `model/channel.go:1258-1311` `ValidateSettings` | Validates `Reliability` **only from `channel.Setting`** (`:1259-1276`). `settings` is parsed into `dto.ChannelOtherSettings` (`:1288-1294`), which has no `Reliability` field. The `settings.reliability` block is silently ignored, so the server never checks UI-entered bounds |
| Columns | `model/channel.go:53` `Setting *string json:"setting"` (column `setting`); `:60` `OtherSettings string json:"settings" gorm:"column:settings"` | |

### Read path (backend reads reliability from `setting`)

| Reader | Location |
|---|---|
| `GetSetting()` unmarshals `channel.Setting` into `dto.ChannelSettings` | `model/channel.go:1313-1324` |
| `GetReliabilitySettings()`: if `Reliability == nil`, returns `DefaultChannelReliabilitySettings()` | `model/channel.go:1326-1332` |
| `dto.ChannelSettings.Reliability` (json `reliability`) | `relaykit/dto/channel_settings.go:26` (struct `:13-27`) |
| `dto.ChannelOtherSettings` has **no** `Reliability` field | `relaykit/dto/channel_settings.go:69-89` (HEAD); `:76-97` in the deployed `8156149df` |
| Defaults: MaxAttempts 2, AutoBanThreshold 5, AutoBanDurationSecs 1800. **No timeout is set** (`FirstContentTimeoutSeconds` stays `nil`) | `relaykit/dto/channel_reliability.go:23-29`, `WithDefaults` `:31-43` |
| Hedge gate | `controller/relay_hedge.go:65-67` |
| Hedge threshold timer (would also be `nil`) | `controller/relay_hedge.go:275-277` |
| Other readers of the same empty field: per-channel max attempts `controller/relay.go:245` and `controller/relay_hedge.go:436`; auto-ban threshold/duration `model/channel.go:432-449`; streaming idle timeout `relay/helper/stream_scanner.go:124-125` (HEAD); non-streaming timeout `relay/channel/api_request.go:550`. The last two read `info.ChannelSetting.Reliability`, which is filled from `channel.GetSetting()` (`middleware/distributor.go:446`, `relay/common/relay_info.go:225-228`) | |

**History (git):** The mismatch started in `845a85998` (2026-09-11). That commit added `GetReliabilitySettings()` reading `setting` and, in the same commit, a frontend `settingsObj.reliability` block in `buildSettingsJSON`, writing to `settings`. `b9692d6f4` (hedge, 09-17) added the three timeout fields to that same frontend block. The archived hedge task's `research/configuration-surface.md` records "`ChannelOtherSettings` … its `reliability` property" as the existing contract. The deployed `8156149df` does not change either side (`git diff f3f7a8129 8156149df` touches neither the reliability read/write paths nor `controller/relay_hedge.go`).

### Global options

- The only hedge-related global option in code is `general_setting.bill_hedge_losers` (`setting/operation_setting/general_setting.go:14,28`). It is read per request at `controller/relay_hedge.go:161` and only decides loser billing. It is **not persisted** in prod `options`, so the default `true` applies. UI: `web/src/features/system-settings/models/routing-reliability-section.tsx:78`.
- **No** global enable switch or global default threshold exists. `FirstContentTimeoutSeconds` is only referenced in `controller/relay_hedge.go` and `relaykit/dto/channel_reliability.go`.
- The 09-18 deploy note mentions "three global switches". I could not match that to code; there is one global hedge option. It may mean the three per-channel timeout fields (inferred).

## 2. Prod state (read-only)

### Column scan, all 52 channels

| Column | Contains `reliability` | Contains `first_content_timeout` |
|---|---|---|
| `setting` (read by backend) | **0** | **0** |
| `settings` (written by UI) | 35 | 12 |
| `other_info` / `other` / `param_override` / `header_override` | 0 | 0 |

### The 12 channels with the `first_content_timeout_seconds` key (all in `settings`)

| id | name | type | status | value |
|---|---|---|---|---|
| **10** | **ZIVV CCMAX** | 14 (Anthropic) | 1 | **60** |
| **58** | **Rolldek image2.5** | 60 (New API) | 1 | **60** |
| 6 | ZIVV DeepSeek | 60 | 1 | 0 |
| 11 | CCTQ CC | 14 | 1 | 0 |
| 26 | FAST PLUS | 1 | 1 | 0 (max_attempts 3) |
| 45 | SC CODEX 3 | 1 | 1 | 0 |
| 47 | FAST CC | 14 | 1 | 0 |
| 48 | CCTQ codex | 1 | 1 | 0 |
| 64 | SC CODEX 2 | 1 | 1 | 0 |
| 66 | Rolldek image2.5 专线满血 | 60 | 1 | 0 |
| 67 | FAST PRO 订阅 | 1 | 2 | 0 (max_attempts 3) |
| 68 | ZIVV CCMAXfable | 14 | 1 | 0 |

#10's current row:

- `setting` = `{"force_format":false,"thinking_to_content":false,"proxy":"","pass_through_body_enabled":true,"system_prompt":"","system_prompt_override":false}`
- `settings.reliability` = `{"max_attempts":2,"auto_ban_threshold":5,"auto_ban_duration_seconds":1800,"first_content_timeout_seconds":60,"streaming_idle_timeout_seconds":0,"non_streaming_timeout_seconds":0}`
- `param_override` is empty and `header_override` is `{"*": true}`. Base URL is `https://api2.zivv.pro` and the groups are `【职刻】Claude 官转,Claude 官转`.

### When the 60 was saved (backup dumps, `pg_restore -a -t channels -f -`, keys not printed)

- `pre-passthrough-20260918-154921` (2026-09-18 23:49 CST): #10 `settings.reliability` has only max_attempts and auto-ban. **No timeout keys.**
- `pre-provider-coverage-20260925` (2026-09-25 01:00 CST): #10 **already has `first_content_timeout_seconds: 60`**. #58 is the same.
- Audit log (`logs.type=3`) records full saves of #10 in that window at 09-23 01:48:23, 09-23 16:37:32, 09-23 16:37:50, 09-25 00:19:03, 00:28:18, 00:29:03 and 00:34:52 CST. The only later events are status toggles at 13:37 and 16:29. The audit `changed_fields` list (`controller/channel.go:1143-1161`) does not track `setting` or `settings`, so the exact save cannot be pinned down.
- So the value was saved before the coordinator's 17:00–17:30 check. That check only covered `setting`.

### `options` table

- No hedge, racing, reliability or first-content keys.
- Present: `RetryTimes=10`, `channel_affinity_setting.keep_on_channel_disabled=false`. Affinity `rules` and `enabled` are not persisted, so the code defaults apply, including `claude cli trace`.
- Also present: `global.pass_through_request_enabled=true` and `global.pass_through_headers_enabled=true`.

### Prod behaviour confirms that `settings.reliability` is ignored

#26 and #67 save `max_attempts: 3` in `settings`. Route history in the app logs since 09-18 shows `"attempt":2,"outcome":"channel_exhausted","reason":"attempts_exhausted"` 86 times for #26 and 18 times for #67, and **never an attempt 3**. The deployed binary therefore uses the default 2 (from `setting`), not the saved 3.

## 3. Did racing ever fire? No.

### Strings the hedge path emits (code)

- `logger.LogInfo`: `"hedge attempt %d launched on channel %d"` (`controller/relay_hedge.go:297`). This is logged for **every** launch, including the initial attempt (`launch(initial, …)` at `:335`). A zero count therefore means `relayWithHedge` was never entered.
- `logger.LogWarn`: `"hedge backup not admitted: …"` (`:392`)
- Route-history outcomes (logged via `logRouteHistory` as `route history: […]`, `controller/relay.go:425-436`, and stored under `other.admin_info.route_history`): `hedge_launched` `:296`, `hedge_winner`/`first_content` `:350`, `hedge_draining` `:354`, `hedge_cancelled`/`loser_billing_policy` `:358`, `hedge_threshold`/`first_content_timeout` `:372`, `hedge_skipped`/`no_eligible_backup` `:385`, `hedge_skipped`/`backup_admission_failed` `:390`, outcome `hedge_loser` `:325`
- Consume-log field `other.hedge` = `{attempt_id, attempt, role, usage_source, metering_status, settlement_status}` (`relay/common/hedge.go:45-52`, written at `service/quota.go:383` and `service/text_quota.go:588`)

### App logs (`/app/logs` in container `flowapi`, timestamps CST)

| File (start → end) | image | `hedge attempt` | `hedge backup not admitted` | `hedge_` | `route history:` lines (with ch 10) |
|---|---|---|---|---|---|
| `oneapi-20260918004621.log` (09-18 00:46 → 23:51) | hk-hedge-b9692d6f4 | 0 | 0 | 0 | 4889 (526) |
| `oneapi-20260918235138.log` (09-18 23:51 → 09-25 01:05) | (passthrough f3f7a8129) | 0 | 0 | 0 | 28527 (5952) |
| `oneapi-20260925010530.log` (09-25 01:05 → now) | hk-media-8156149df | 0 | 0 | 0 | 3420 (2281) |

The only case-insensitive `hedge` matches are the 2 startup banners (`New API hk-hedge-b9692d6f4-20260918 started`). Info-level logging is active, as the `route history:` lines show.

### DB `logs` since 2026-09-18 00:00 CST (`created_at >= 1789660800`)

| type | rows | `other` contains `"hedge"` | `other` contains `hedge_` | channel 10 |
|---|---|---|---|---|
| 2 consume | 36565 | 0 | 0 | 8602 |
| 3 manage | 197 | 0 | 0 | 0 |
| 5 error | 343 | 0 | 0 | 118 |
| 7 | 9 | 0 | 0 | 0 |

## 4. Why #10 never races: checks in `hedgeEligible` (`controller/relay_hedge.go:51-82`)

#10 traffic since 09-18 (type=2): about 8,350–8,370 stream and 258 non-stream consume rows, all `claude-*` models. Counts drift slightly between live queries. Breakdown query for stream rows: 8067 matched the affinity rule and used the pinned channel (`selected_group` present), 237 matched the rule without the pin, and 51 did not match the rule (sum 8355).

| Check | Code | Result for #10 | Status |
|---|---|---|---|
| Model name contains `audio` / `-search` | `:52-55` | Passes (claude models) | Confirmed |
| `!info.IsStream` | `:56` | Blocks the 258 non-stream requests | Confirmed |
| `hedgeChannelAllowed(#10)` | `:37-49`, `:56` | Passes: status 1; type 14 maps to `APITypeAnthropic` (`common/api_type.go:10-11`); `param_override` column empty | Confirmed (code + DB) |
| `ShouldSkipRetryAfterChannelAffinityFailure(c)` | `:56`; `service/channel_affinity.go:626-642` | **Blocks** Claude Code requests. The default rule `claude cli trace` (`setting/operation_setting/channel_affinity_setting.go:134-148`: `^claude-.*$`, `/v1/messages`, key `metadata.user_id`, `SkipRetryOnFailure: true`) is matched in the distributor (`middleware/distributor.go:105`, which stores meta at `service/channel_affinity.go:596-610`). On a pinned hit, `MarkChannelAffinityUsed` sets gin key `channel_affinity_skip_retry_on_failure=true` (`:684`). Otherwise it falls back to `meta.SkipRetry=true` (`:637-641`). The only thing that resets it to false is `ClearCurrentChannelAffinityCache` (`:659`, distributor `:131` when the pinned channel is unusable). This is the **first** check to fail for about 8300 of about 8355 stream requests | Confirmed code; per-request value inferred from log markers |
| `specific_channel_id` | `:59`; set only at `middleware/auth.go:516-518` (admin token with `-<channelId>` suffix) | Not expected for normal traffic | Inferred |
| Context `ContextKeyChannelParamOverride` non-empty | `:62-64` | **Blocks** every affinity-matched request. `SetupContextForSelectedChannel` merges the rule's template into the param override (`middleware/distributor.go:448-453` → `service/channel_affinity.go:533-548`, `mergeChannelOverride` `:448-475`). The template is `{"operations":[{"mode":"pass_headers",…,"keep_origin":true}]}`, one key (`channel_affinity_setting.go:84-96`). Prod: 8541 of 8604 #10 consume rows carry `admin_info.channel_affinity.override_template = {"applied":true,"param_override_keys":1,"rule_name":"claude cli trace"}` | Confirmed (code + prod logs) |
| `FirstContentTimeoutSeconds == nil \|\| <= 0` | `:65-67` | **Blocks every request on every channel**: the value is read from `setting` (empty); the saved 60 sits in `settings` | Confirmed (code + DB + #26/#67 behaviour) |
| Relay mode / format | `:69-75` | Passes (RelayFormatClaude) | Confirmed |
| `hedgeReplaySafe(body)` | `:76-127` | Standard Claude custom tools (`name` + `input_schema`, no `type`) pass at `:118`. Any tool with a non-`function` `type` (e.g. server tools such as `web_search_*`) would fail at `:124` | Not verified: request bodies are not stored |
| Threshold elapsed? | Timer armed in `Dispatched` `:269-294` | Never armed: `relayWithHedge` is never entered. Even inside it, `:275-277` reads the same empty field. Had it been armed, 60 s would have passed on some requests: 42 successful #10 stream requests had `frt >= 60000` ms (first upstream event, `relay/helper/stream_scanner.go:453` HEAD); 521 had `use_time >= 60 s`; 55 #10 error rows (type 5) carry `prebuffer_overflow` (each attempt about 80–90 s, e.g. log ids 81964, 81965) | Inferred (lower bounds) |

Related facts:

- **Serial vs hedge asymmetry.** The serial path deliberately ignores the affinity skip-retry flag for cross-channel failover (`service/relay_retry.go:36-38`); `hedgeEligible` honours it (`:56`).
- **Backup admission.** Removing `:56` and `:65` alone would not be enough for affinity-matched requests. `launch` re-runs `SetupContextForSelectedChannel` on a cloned context that still holds the affinity meta (`cloneHedgeContext` `:504-522`). It then rejects the backup with `"parameter overrides require serial routing"` (`:205-209`), recorded as `hedge_skipped`/`backup_admission_failed` (`:389-392`). Inferred from code.
- **#58 Rolldek image2.5 (the other 60 s channel)** could not race even if the column were right. Type 60 maps to `APITypeNewAPI` (`common/api_type.go:82-83`), which `hedgeChannelAllowed` rejects (`:41-47`). All 1977 of its consume rows since 09-18 are non-stream image generation (`gpt-image-2.5-*`). Confirmed.

## 5. Body pass-through on #10 (`pass_through_body_enabled: true`)

- **`hedgeEligible` / `hedgeChannelAllowed` / `hedgeReplaySafe`** never read `PassThroughBodyEnabled`. The replay check parses the client body from `BodyStorage` (`controller/relay_hedge.go:76-81`), whether or not pass-through is on. No effect. Confirmed.
- **Stream gate.** Type 14 goes through `relay/channel/claude/adaptor.go:124-131` → `ClaudeStreamHandler` → `helper.StreamScannerHandlerWithGate(..., StreamProtocolAnthropic, ...)` (`relay/channel/claude/relay-claude.go:194-208`). The gate is on whenever the protocol is non-empty (`relay/helper/stream_scanner.go:109-120`, HEAD), whether or not pass-through is on. No effect. Confirmed.
- **What pass-through does change** (Claude path):
  - The body is forwarded from `BodyStorage` via `helper.NewPassthroughBody` (`relay/claude_handler.go:55,163-175`; `relay/helper/passthrough_body.go:16-43`, which only rewrites `model` when it is mapped).
  - The param override, including the affinity `pass_headers` template, is applied only in the non-pass-through branch (`relay/claude_handler.go:193-199`). The same holds for Responses (`relay/responses_handler.go:80-116`).
  - The `message_delta` usage patch is skipped (`relay/channel/claude/relay-claude.go:68-76`, used at `:113`).
  - So on #10 the affinity template that blocks hedging at `:62` has no effect on the outbound request body (code reading).
- **Global pass-through is on** in prod (`global.pass_through_request_enabled=true`, `relay/claude_handler.go:55`). Every channel takes the pass-through branch, and #10's own flag changes nothing.

## Files Found

| File Path | Description |
|---|---|
| `controller/relay_hedge.go` | `hedgeChannelAllowed` :37-49, `hedgeEligible` :51-82, `hedgeReplaySafe` :84-127, `relayWithHedge` :160-482 (launch log :297, backup admission check :205-209, threshold :269-294/:368-393) |
| `controller/relay.go` | hedge entry :164-167, serial max attempts :245, `logRouteHistory` :425-436 |
| `model/channel.go` | columns :53/:60, `ValidateSettings` :1258-1311, `GetSetting` :1313-1324, `GetReliabilitySettings` :1326-1332 |
| `relaykit/dto/channel_settings.go` | `ChannelSettings.Reliability` :26; `ChannelOtherSettings` without reliability :69-89 |
| `relaykit/dto/channel_reliability.go` | reliability DTO, defaults, bounds |
| `web/src/features/channels/lib/channel-form.ts` | form schema, load :561-598, `buildSettingJSON` :677-701, `buildSettingsJSON` reliability :837-848, update payload :915-967 |
| `web/src/features/channels/types.ts` | `ChannelOtherSettings.reliability` :114 |
| `web/src/features/channels/components/drawers/sections/channel-reliability-fields.tsx` | timeout fields :74-96 |
| `web/src/features/channels/components/drawers/channel-mutate-drawer.tsx` | detail query :662-665, form load :1235-1238 |
| `controller/channel.go` | `GetChannel` :450-470, `validateChannel` :509-520, `UpdateChannel` :988-1180 |
| `middleware/distributor.go` | affinity selection :105-133, `SetupContextForSelectedChannel` :437-453 |
| `service/channel_affinity.go` | template merge :448-475/:533-548, meta :596-610, skip-retry :626-642, clear :644-666, mark used :676-699 |
| `setting/operation_setting/channel_affinity_setting.go` | default `claude cli trace` rule :134-148 |
| `service/relay_retry.go` | serial retry ignores affinity skip-retry :36-38 |
| `setting/operation_setting/general_setting.go` | `bill_hedge_losers` :14/:28 |
| `relay/claude_handler.go`, `relay/helper/passthrough_body.go`, `relay/channel/claude/relay-claude.go`, `relay/channel/claude/adaptor.go` | pass-through and Claude stream gate path |

## Related Specs

- `.trellis/spec/backend/streaming-provider-hedge.md`. §2 says "Channel `settings.reliability` stores optional integer seconds", i.e. the UI's storage location. The backend implementation reads `setting.reliability`.
- `.trellis/tasks/archive/2026-09/09-17-streaming-provider-hedge/research/configuration-surface.md`: records the frontend contract (`ChannelOtherSettings.reliability`).

## Caveats / Not Found

- Prod runs image `flowapi:hk-media-8156149df-20260925` (branch `codex/flowapi-provider-coverage`). I did not inspect the binary. Code parity comes from `git diff f3f7a8129 8156149df`, and the #26/#67 attempt counts confirm the behaviour.
- The working tree has uncommitted edits by another agent in `relay/helper/stream_gate.go`, `stream_scanner.go` and `stream_scanner_test.go`. Line numbers for `stream_scanner.go` above come from `git show HEAD:`. `relay/helper` is identical between HEAD and the deployed commit.
- The exact save time of #10's 60 cannot be determined (see §2 window). No write path other than `PUT /api/channel/` was investigated.
- Replay safety of real Claude Code bodies was not verified.
- Seen in code only, and not relevant to #10: the advanced-custom config endpoint re-marshals `settings` through `dto.ChannelOtherSettings` (`controller/channel.go:1291-1305`). That struct has no `reliability` field, so this path would drop a `reliability` block.
- Latency counts (42 / 521 / 55) are lower bounds. `frt` measures the first upstream event, not the first content.

## 6. Values that take effect after the fix

Follow-up added 2026-09-25. Prod queried read-only around 19:30 CST; backup dumps were streamed to stdout and keys were never printed.

### 6.1 Prod `channels.settings` → `reliability` survey

- Of 52 channels, **35** have `settings.reliability`, always a JSON object. **17** have none. **0** have `setting.reliability`, so switching the reader to `settings` loses nothing.
- Keys present are only the six known ones. There are no legacy keys (`failure_threshold` / `open_duration_seconds`, which the UI still reads at `channel-form.ts:601-609`) and no unknown keys.
- All 141 values are plain JSON integer literals: no strings, decimals, exponents or signs.
- Ranges seen:
  - `max_attempts`: 2–3 (35 rows)
  - `auto_ban_threshold`: 5 (35)
  - `auto_ban_duration_seconds`: 1800 (35)
  - `first_content_timeout_seconds`: 0–60 (12)
  - `streaming_idle_timeout_seconds`: 0 (12)
  - `non_streaming_timeout_seconds`: 0 (12)
- Every value is within `relaykit/dto/channel_reliability.go:45-70` and spec §2. **No malformed or out-of-range values.**

**Channels with non-default values** (defaults: max_attempts 2, auto_ban_threshold 5, auto_ban_duration_seconds 1800, timeouts 0 or absent):

| id | name | type | status | non-default fields |
|---|---|---|---|---|
| 10 | ZIVV CCMAX | 14 (Anthropic) | 1 enabled | `first_content_timeout_seconds=60` |
| 26 | FAST PLUS | 1 (OpenAI) | 1 enabled | `max_attempts=3` |
| 58 | Rolldek image2.5 | 60 (New API) | 1 enabled | `first_content_timeout_seconds=60` |
| 67 | FAST PRO 订阅 | 1 (OpenAI) | 2 manually disabled | `max_attempts=3` |

- `reliability` present with all defaults: **31** channels.
- Effect, per code:
  - #26: serial per-channel attempts go from 2 to 3 (`controller/relay.go:245`).
  - #67: no traffic effect while disabled.
  - #10: racing becomes possible only for requests that also pass `controller/relay_hedge.go:56` and `:62`. The affinity blockers in §4 still apply to its Claude Code traffic.
  - #58: still cannot race. Its API type is rejected at `:41-47` and all its traffic is non-stream.
- **Values already lost (not in `settings` any more): #27 FAST PLUS 稳定 and #28 FAST PRO.**
  - Both had `reliability={max_attempts:3, auto_ban_threshold:5, auto_ban_duration_seconds:1800}` in the 09-11, 09-14 and 09-18 backups.
  - In the 09-25 01:00 backup and in the current row, `reliability` is gone and `upstream_model_update_check_enabled=true` (last check 2026-09-25 19:28:52 CST).
  - Their last UI saves were 09-23 10:41–10:42 CST, and the UI always writes the reliability block (`channel-form.ts:837-848`).
  - Inferred cause: the upstream-model-update writer in §6.3 re-marshals `settings` through `dto.ChannelOtherSettings`. After the fix these two channels read defaults unless the values are entered again.

### 6.2 Readers that must switch from `setting` to `settings`

| Reader | Location | Reads today |
|---|---|---|
| `Channel.GetReliabilitySettings()` | `model/channel.go:1326-1332` (via `GetSetting()` `:1313-1324`) | `channels.setting` → `dto.ChannelSettings.Reliability` |
| ↳ hedge gate | `controller/relay_hedge.go:65` | |
| ↳ hedge threshold timer | `controller/relay_hedge.go:275` | |
| ↳ hedge per-channel retry cap | `controller/relay_hedge.go:436` | |
| ↳ serial per-channel max attempts | `controller/relay.go:245` | |
| ↳ auto-ban threshold and duration | `model/channel.go:432` (used `:441-445`) | |
| Streaming idle timeout | `relay/helper/stream_scanner.go:124-125` at HEAD (`:125-126` in the working tree, which another agent is editing) | `info.ChannelSetting.Reliability.StreamingIdleTimeoutSeconds` |
| Non-streaming total timeout | `relay/channel/api_request.go:550-552` | `info.ChannelSetting.Reliability.NonStreamingTimeoutSeconds` |

- No other non-test Go code references `MaxAttempts`, `AutoBanThreshold`, `AutoBanDurationSecs` or the three `*TimeoutSeconds` fields, apart from the DTO itself.
- The frontend already reads `settings` (`channel-form.ts:592-611`).
- **`RelayInfo` already carries the parsed other-settings:**
  - Field: `ChannelMeta.ChannelOtherSettings dto.ChannelOtherSettings` (`relay/common/relay_info.go:72`, next to `ChannelSetting` at `:71`). It is reachable as `info.ChannelOtherSettings` because `*ChannelMeta` is embedded.
  - Filled in `InitChannelMeta` (`relay/common/relay_info.go:230-233`) from `ContextKeyChannelOtherSetting`. That key is set by `SetupContextForSelectedChannel` from `channel.GetOtherSettings()` (`middleware/distributor.go:447`).
  - `InitChannelMeta` runs once per relay handler (e.g. `relay/claude_handler.go:26`, `relay/compatible_handler.go:26`, `relay/responses_handler.go:23`, `relay/gemini_handler.go:55,207`) and once per hedge attempt (`controller/relay_hedge.go:223`).
  - `dto.ChannelOtherSettings` currently has **no `Reliability` field** (`relaykit/dto/channel_settings.go:69-89` at HEAD). It lives in the independent `relaykit` module; AGENTS.md requires `cd relaykit && GOWORK=off go build ./...` for changes there.
  - Two non-relay places also build `ChannelMeta.ChannelOtherSettings`: `controller/channel-billing.go:382` and `controller/channel_upstream_update.go:480`.

### 6.3 Backend writers and validators of `reliability`

**Validators**

- `model/channel.go:1258-1311` `ValidateSettings`:
  - validates **`setting.reliability`** only (`:1259-1276`, `Reliability.Validate()`);
  - parses `settings` into `dto.ChannelOtherSettings` (`:1288-1294`), so `settings.reliability` is not validated;
  - callers: `validateChannel` at `controller/channel.go:518`, which is used by AddChannel (`:663`), UpdateChannel (`:1011`) and advanced-custom save (`:1323`); and channel copy (`controller/channel.go:1495`).
- `relaykit/dto/channel_reliability.go:45-70` `Validate()`, with `WithDefaults()` at `:31-43`.
- Frontend-only bounds: `web/src/features/channels/lib/channel-form.ts:290-314`.

**Writers of `setting`** (carry `setting.reliability` only if it is present; prod has none)

- `model/channel.go:1334-1341` `SetSetting`. Its only non-test caller is `controller/channel.go:1317-1321` (advanced-custom proxy), which round-trips `dto.ChannelSettings`, a struct that does include `Reliability`.
- `model/channel.go:1313-1324` `GetSetting`: on an unmarshal error it sets `Setting=nil` and calls `channel.Save()`.
- AddChannel, UpdateChannel and copy persist the `setting` string as received (`controller/channel.go:741`, `:995`/`:1133`, `:1484-1502`).

**Writers of `settings`** (where `reliability` lives)

- UI create and update persist `settings` as received (`controller/channel.go:741`, `:995`/`:1133`). The frontend merges into the existing reliability object and keeps unknown sub-keys (`channel-form.ts:837-848`).
- Channel copy keeps `settings` unchanged (`controller/channel.go:1484-1502`).
- `model/channel.go:1356-1363` `SetOtherSettings` re-marshals `dto.ChannelOtherSettings`. **Today this drops `reliability` completely.** With the field added, it would keep the six known keys and drop unknown ones. Callers:
  - `controller/channel.go:1291-1305` (advanced-custom config endpoint)
  - `controller/channel_upstream_update.go:500-509` `updateChannelUpstreamModelSettings`, called at:
    - `:529` and `:549` inside `checkAndPersistChannelUpstreamModelUpdates`. That function is used by the periodic `runChannelUpstreamModelUpdateTaskOnce` (`:689-754`, for channels with `upstream_model_update_check_enabled`) and by manual detect (`:933`).
    - `:994` inside `applyChannelUpstreamModelUpdates` (manual apply).
  - This is the likely path that removed #27/#28's values (§6.1).
- `model/channel.go:1343-1354` `GetOtherSettings`: on an unmarshal error it resets all of `settings` to `{}` and calls `channel.Save()`. It runs on every request (`middleware/distributor.go:447`). Once a typed `Reliability` field exists, a type-invalid reliability value would take this path. Prod values are all plain integers today.

**Tests that write `setting.reliability` / `ChannelSetting.Reliability`** (they lock the current column)

- `controller/relay_hedge_policy_test.go:152-153`: `Setting: &settings`, with `{"reliability":{"first_content_timeout_seconds":1,"max_attempts":1}}`
- `controller/relay_hedge_test.go:72-73`: the same
- `model/channel_auto_ban_test.go:27-33` and `service/channel_auto_ban_test.go:45-51`: `channel.SetSetting(dto.ChannelSettings{Reliability: ...})`
- `relay/channel/deadline_body_test.go:33`, `:83-85` and `relay/helper/stream_scanner_test.go:964`: `ChannelMeta{ChannelSetting: dto.ChannelSettings{Reliability: ...}}`
- `relaykit/dto/channel_reliability_test.go`: DTO-only, independent of the column
- Frontend tests lock **`settings.reliability`**: `web/src/features/channels/lib/__tests__/channel-reliability-form.test.ts:31-72`, `:73-90`, `:122-144`, `:158-183`, including preservation of the unknown `future_setting` at `:60`.
- Spec: `.trellis/spec/backend/streaming-provider-hedge.md:11` documents `settings.reliability`. That file is currently modified in the working tree by another agent.
