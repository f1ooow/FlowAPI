# Design：子项 B 渠道可靠性设置存读一致

子项 A（流式闸门）为轻量改动，只有 PRD；本文件只覆盖子项 B。事实与行号来自 `research/hedge-config.md`。

## 根因

- 写入与约定：前端表单把 `reliability` 写入 `channels.settings`，并从 `settings` 读回（`web/src/features/channels/lib/channel-form.ts:837-848`、`:565-598`）；前端类型、前端测试（`channel-reliability-form.test.ts:60`）和竞速规范 §2 都约定 `settings.reliability`。
- 读取：后端 `Channel.GetReliabilitySettings()` 从 `channels.setting` 读取（`model/channel.go:1313-1332`，`relaykit/dto/channel_settings.go:26`）；中继超时读 `info.ChannelSetting.Reliability`（`relay/helper/stream_scanner.go:124-125`、`relay/channel/api_request.go:550-552`）。
- `dto.ChannelOtherSettings` 没有 `Reliability` 字段（`relaykit/dto/channel_settings.go:69-89`）。任何经结构体重新序列化 `settings` 的写入方（`model/channel.go:1356-1363` `SetOtherSettings`，调用方 `controller/channel.go:1305`、`controller/channel_upstream_update.go:500-509`）都会把 `reliability` 丢掉。#27、#28 已因此丢失 `max_attempts=3`（推断）。

## 决策

1. **唯一来源为 `channels.settings`**：在 `dto.ChannelOtherSettings` 上新增 `Reliability *ChannelReliabilitySettings \`json:"reliability,omitempty"\``。理由：前端、前端测试、规范都已按此约定；生产有 35 行在 `settings`、0 行在 `setting`，无需数据迁移。
2. **删除 `dto.ChannelSettings.Reliability`**，不保留兼容读取（生产无数据；按用户偏好优先破坏性变更）。
3. **读取**：`Channel.GetReliabilitySettings()` 改为读 `GetOtherSettings().Reliability`，再 `WithDefaults()`。调用方签名不变：`controller/relay.go:245`、`controller/relay_hedge.go:65,275,436`、`model/channel.go:432`。
4. **中继超时**：流式静默超时与非流式总超时改读 `info.ChannelOtherSettings.Reliability`。`RelayInfo.ChannelMeta.ChannelOtherSettings` 已存在（`relay/common/relay_info.go:72`），由 `channel.GetOtherSettings()` 填充（`middleware/distributor.go:447`），竞速每次尝试也会重新填充（`controller/relay_hedge.go:223`）。
5. **校验**：`Channel.ValidateSettings`（`model/channel.go:1258-1311`）改为对 `settings.reliability` 调用 `dto.ChannelReliabilitySettings.Validate()`（`relaykit/dto/channel_reliability.go:45-70`），移除对 `setting.reliability` 的校验。新增、更新、高级配置保存、复制都经过这里（`controller/channel.go:518`、`:1495`）。
6. **回写保留**：`reliability` 成为有类型字段后，`SetOtherSettings` 的各调用方会原样保留它，不再丢失。
7. **竞速资格不变**（PRD B5）：`hedgeEligible` 中会话粘连的两处拦截（`controller/relay_hedge.go:56`、`:62-64`）保持现状。

## 兼容与风险

- **解析失败会清空设置**：`GetOtherSettings()` 在 JSON 解析失败时把整个 `settings` 重置为 `{}` 并保存（`model/channel.go:1343-1354`），且每个请求都会调用。`Reliability` 有类型后，类型不对的值（如字符串）会触发这次重置。控制手段：
  - 生产现有 141 个值全是范围内的整数，已核验。
  - 决策 5 让服务端保存时就拒绝非法值。
  - 本任务不改动重置行为本身，已记入 Out of Scope。
- **部署即生效**：#10、#58 首内容阈值 60 秒，#26、#67 最大尝试 3 次。#26 的上游报错在尝试开始后 p50 43 秒才出现，每个失败请求在切换前要多等一轮。
- **relaykit 独立性**：DTO 改动位于 `relaykit/`，必须通过 `cd relaykit && GOWORK=off go build ./...`，且不得引入对根模块的依赖。
- **回滚**：回退提交即可。本次不迁移数据，`settings` 内容不变。

## 测试要点

- 读取：`settings.reliability.max_attempts=3` 时 `GetReliabilitySettings()` 返回 3；缺省时返回默认值；`setting` 中的旧键不再有任何作用（字段已删除）。
- 中继：流式静默超时与非流式总超时取自 `ChannelOtherSettings.Reliability`。
- 校验：越界值（首内容阈值 181、最大尝试 11、静默超时 30）保存被拒；合法值通过。
- 回写：经 `SetOtherSettings` 回写（上游模型更新路径）后 `reliability` 仍在。
- 竞速：`settings.reliability.first_content_timeout_seconds=60` 且无粘连的请求可以竞速；命中粘连时仍不竞速。
- 更新锁定旧 `setting` 列的测试：`controller/relay_hedge_policy_test.go:152-153`、`controller/relay_hedge_test.go:72-73`、`model/channel_auto_ban_test.go:27-33`、`service/channel_auto_ban_test.go:45-51`、`relay/channel/deadline_body_test.go:33,83-85`、`relay/helper/stream_scanner_test.go:964`。

---

# Design：子项 C 移除竞速（含首内容阈值）

移除清单逐项列在 `research/hedge-removal.md`；本节只记录决策。

## 决策

1. **逐项删除，不回退提交**：`b9692d6f4` 同时带入了静默超时、非流式超时和通用修复，`git revert` 会误删这些，还会和归档提交以及 A、B 冲突。按清单的 DELETE / EDIT / KEEP 执行。
2. **闸门**：删除 `info.Hedge` 相关的全部分支，包括 1MB 扫描缓冲、`ObservePrefix`、`observeHedgeUsage`、`Detached`、`Commit`，以及 A 的 R5 特例。此后任何尝试攒满上限都走放行路径。`StreamPrecommitFailure{Reason: "prebuffer_overflow"}` 若已无调用方就一并删除。
3. **设置**：删除 `general_setting.bill_hedge_losers` 及其配置快照项，并从 `dto.ChannelReliabilitySettings` 中删除 `FirstContentTimeoutSeconds` 及其校验、默认值。
4. **前端**：删除首内容阈值字段、输家计费开关、日志详情里的竞速展示，以及 `zh.json` / `static-keys.ts` 里只给竞速用的文案。保留 `routing-reliability-section.tsx:333` 的保存失败返回。
5. **保留的通用改动**：`logger/logger.go` 的数据竞争修复、`setting/config/config.go:291-292` 的配置锁、`model/log.go:148-152` 的 ClickHouse 排序补充键。补充键对非竞速行也有用，保留。
6. **测试**：删除竞速测试。其中覆盖通用行为的三项改成更小的直接测试：`SumUsedQuota` 过滤（原 `TestSumUsedQuotaHedgeAttempts`）、ClickHouse 日志排序（原 `TestClickHouseLogOrder` 依赖的 `setupHedgeLogDB` 改成通用夹具），以及前端设置保存被拒后的重试（原 `hedge-loser-billing.test.tsx`）。
7. **规范**：新建 `.trellis/spec/backend/channel-reliability.md`，放入以下合同，然后删除 `streaming-provider-hedge.md`：
   - `settings.reliability` 是唯一来源，保存时校验，经 `SetOtherSettings` 回写时保留；
   - 六个字段的取值范围，包括 `max_attempts` 1–10、自动封禁阈值 1–100、时长 1–86400；
   - 静默超时与非流式超时的语义；
   - 子项 D 的"填 0 用全局默认"。

   同步更新 `stream-precommit-gate.md`（去掉竞速相关内容，攒满上限一律放行）、`index.md`、`billing-ratio-routing.md` 和 `global-passthrough.md` 里的引用。
8. **生产数据**：不做迁移。已存的 `first_content_timeout_seconds` 键读取和校验时会被忽略；后台表单保存会原样保留它，后端经 `SetOtherSettings` 回写时会丢弃，两种情况都无害。

# Design：子项 D 流式静默超时填 0 使用全局默认

## 决策

1. `relay/helper/stream_scanner.go` 的静默超时取值：
   - 渠道 `streaming_idle_timeout_seconds` 大于 0 时用渠道值；
   - 否则用 `constant.StreamingTimeout`，也就是 `STREAMING_TIMEOUT`，默认 300 秒（`common/init.go:182`）；
   - 两者都不大于 0 时关闭。

   计时和原版一致：从收到响应头开始，每读到一次数据就重置。
2. 放行前触发时，仍返回可重试的 `idle_timeout` 预提交失败，透明换渠道；放行后触发时，结束这条流，不重放。这两点都是现有行为，不需要改。
3. 非流式总超时不改代码。渠道值为 0 时已经沿用全局 `RELAY_TIMEOUT`（`service/http_client.go:106-108`），只修改前端说明文字。
4. 前端说明文字：两项超时都改为"0 表示使用全局默认"，并同步各语言文案。
5. 风险：300 秒在 09-18 之前一直在生效，其间从未触发过，误伤风险低。
