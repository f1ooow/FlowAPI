# 流式闸门判定修复 + 可靠性设置存读一致 + 移除竞速 + 超时全局默认

本任务含四个子项，均已实现、通过最终全量检查，提交于 `d4183f2bf`（代码）与 `c59af3457`（任务文档），2026-09-26 00:18 CST 一起部署到 HK。

---

## 子项 A：流式闸门——认不出的帧不再判失败，thinking 快速放行

状态：已实现（trellis-implement）并通过 trellis-check，已提交 `d4183f2bf`。

### Goal

流式预提交闸门不再因为认不出的帧而判上游失败：放行前只有明确的失败信号才换渠道，缓冲攒满上限时直接放行。Claude Code 的长时间思考请求在思考开始后立即放行，客户端马上收到响应头和首批事件；渠道不再因此被误计失败、误封。放行前出现的上游报错仍能透明换渠道。

### Background（已确认事实）

- 闸门由 09-01 故障路由任务引入（`.trellis/tasks/09-01-resilient-routing-model-redirects/design.md:54`）：首个有效内容出现前扣住上游事件、不向客户端写任何字节，以便透明切换渠道；设计要求只保留校验所需的最小前缀。前缀上限为 64 个 JSON 事件 / 10MB（`relay/helper/stream_gate.go:11-12`），数值与 CCH 默认值相同；修复前超过上限即判 `prebuffer_overflow` 并计为渠道失败。
- 放行前客户端收不到任何字节：响应头在放行时才写，放行前 FlowAPI 也不发 ping（`relay/helper/stream_scanner.go:226-228`）。
- Claude Code 默认带 `redact-thinking-2026-02-12` beta：思考期间上游持续发送文本为空的 `thinking_delta`，思考结束时发 `signature_delta`。修复前 Anthropic 判定要求 `delta.thinking` 非空才算内容（`relay/helper/stream_gate.go:97-99`），空 `thinking_delta` 被当作中性帧计数；上游约 1.4 秒一帧，约 90 秒达到 64 帧上限。
- OpenAI Chat 有同类缺口：`delta.reasoning`、`delta.reasoning_details` 不在内容路径里（`relay/helper/stream_gate.go:107-111`），生产仅 2 次（#61 兔子 DeepSeek，约 2 秒内攒满 64 帧，换渠道后成功）。
- 每次耗尽渠道尝试次数累计 1 次自动封禁（实际生效的是默认值：5 次封 30 分钟，见子项 B）。
- 竞速以闸门放行作为赢家判定（`relay/helper/stream_scanner.go:316-324`、`controller/relay_hedge.go:318-319`）；竞速从未运行过（见子项 B）。
- 生产证据（09-02 至 09-25，详见 `research/other-protocols.md`）：
  - `prebuffer_overflow`：`/v1/messages` 173 个请求（119 个最终失败、54 个白等后成功），集中在 #10 ZIVV CCMAX、#47 FAST CC、#11 CCTQ CC；`/v1/responses` 3 个、`/v1/chat/completions` 2 个，均最终成功。
  - 闸门在放行前拦下真实失败并换渠道成功：`/v1/responses` 约 589 个、`/v1/chat/completions` 43 个、`/v1/messages` 14 个。
  - 真实失败发生得晚：Codex 上游错误从尝试开始算 p50 43 秒、p90 133 秒。抽样 504 个被救回的 Codex 请求，若设 10 / 30 / 60 秒时间放行，分别有 34% / 23% / 12% 会在出错前被放行，变成用户可见错误。
  - 用户 hx 09-25 当天 opus-5-5 错误 69 条；请求 `202609250900150805426128268d9d6DAf4EUyJ` 三次尝试分别在 96/94/104 秒溢出，第 4 次客户端取消，总耗时 5m58s。
  - CCH 08-20 至 08-23 同类报错 14 条，错误体 `frames_seen: 65`、`buffered_bytes` 9300–9900，约 140 字节/帧，与空 `thinking_delta` 帧大小一致，远大于 ping 帧（约 37 字节）；请求头含 `redact-thinking-2026-02-12`。
- 按字段识别内容的做法已多次出问题：CCH issue #1394（DeepSeek `reasoning_content`）与本次空 `thinking_delta`。

### Requirements

- R1（thinking 快速放行）：Anthropic 协议下，`type` 为 `content_block_delta` 且 `delta.type` 为 `thinking_delta` 的帧，无论 `delta.thinking` 是否为空，都判为有效内容，闸门在该帧放行。
- R2（按上限兜底）：放行前，认不出的帧不再导致失败。事件数或字节数达到现有上限（64 个 / 10MB）时直接放行，不再返回 `prebuffer_overflow`，也不计渠道失败。不设时间放行。
- R3（补发顺序）：放行时，之前扣住的事件按上游原顺序补发给客户端，之后正常转发。
- R4（失败信号不变）：放行前出现的上游错误帧、损坏帧、空流（未出现内容就结束）、静默超时，仍按现有逻辑失败并可切换渠道。
- R5（竞速不变）：R2 的按上限放行不适用于竞速候选，竞速候选达到上限仍判失败，避免未见内容就占用赢家位置。R1 的 thinking 判定对竞速候选同样生效（空 `thinking_delta` 是真实生成进度）。
- R6：OpenAI Chat、OpenAI Responses、Gemini、图片协议不新增内容判定规则，只通过 R2 受益。

### Acceptance Criteria

- [x] 回归测试（R1、R3）：`message_start` + 空 thinking `content_block_start` + 超过 64 个空 `thinking_delta` + `signature_delta` + 文本 → 在第一个空 `thinking_delta` 处放行；不出现 `prebuffer_overflow`；客户端收到的事件及顺序与上游一致。
- [x] 回归测试（R2、R3）：超过 64 个认不出的中性 JSON 帧后才出现内容 → 在达到上限处放行，请求成功，不计渠道失败，事件顺序与上游一致。
- [x] 回归测试（R4）：放行前出现上游错误帧 / 损坏帧 / 未出现内容就结束 → 仍返回预提交失败，响应未提交，可切换渠道。
- [x] 回归测试（R5）：竞速候选达到上限仍判失败；竞速相关现有测试全部通过。
- [x] `relay/helper`、`controller` 现有测试全部通过；`go build ./...` 通过；未改动 `relaykit/`。

### Out of Scope

- 为其他协议新增推理 / 进度帧的快速放行规则（如 OpenAI Chat 的 `delta.reasoning`），由 R2 兜底。
- 时间放行；上限做成可配置；增加闸门开关。
- 把 thinking 的 `content_block_start` 也作为放行点。
- 非 JSON 的 `data: ping` 在放行补发时会中止 Claude / Gemini 处理器（`relay/channel/claude/relay-claude.go:88-91`、`relay/channel/gemini/relay-gemini.go:154-157`），现有问题，生产未出现。
- 客户端断开后被记为 `empty_stream` 的日志标签问题。
- 向 CCH 上游提 issue。

### Accepted Trade-offs

- 思考开始后（R1 放行后）上游才出错的请求，不能再透明切换渠道，由客户端自行重试。
- 认不出的慢节奏进度帧，客户端最多要等到攒满上限才收到第一个字节（按 1.4 秒一帧约 90 秒）。
- 攒满上限后才出错的请求不能再透明切换渠道（非 Claude 协议三周仅 5 次超限，均最终成功）。

---

## 子项 B：渠道可靠性设置存读一致

状态：已实现（trellis-implement）并通过 trellis-check，已提交 `d4183f2bf`。技术方案见 `design.md`，执行清单见 `implement.md`。

### Goal

后台渠道表单里保存的可靠性设置（最大尝试次数、自动封禁阈值与时长、流式首内容阈值、流式静默超时、非流式总超时）真正生效，并在服务端校验。

### Background（已确认事实，详见 `research/hedge-config.md`）

- 前端把可靠性设置写入 `channels.settings`（`web/src/features/channels/lib/channel-form.ts:837-848`，载荷 `:939`），也从 `settings` 读回表单（`:565-598`）；前端类型把 `reliability` 放在 `ChannelOtherSettings`（`web/src/features/channels/types.ts:95-115`）；竞速规范 §2 约定的也是 `settings.reliability`（`.trellis/spec/backend/streaming-provider-hedge.md:11`）。
- 后端只从 `channels.setting` 读取（`model/channel.go:1313-1332`，`relaykit/dto/channel_settings.go:26`）；`ChannelOtherSettings` 没有 `Reliability` 字段（`relaykit/dto/channel_settings.go:69-89`）；服务端校验也只看 `setting`（`model/channel.go:1259-1276`），表单填的值从未被服务端校验。
- 读取点：每渠道最大尝试 `controller/relay.go:245`、`controller/relay_hedge.go:436`；自动封禁阈值与时长 `model/channel.go:432-449`；竞速阈值 `controller/relay_hedge.go:65-67`、`:275-277`；流式静默超时 `relay/helper/stream_scanner.go:124-125`；非流式超时 `relay/channel/api_request.go:550`。后两处读 `info.ChannelSetting.Reliability`，来自 `channel.GetSetting()`（`middleware/distributor.go:446`、`relay/common/relay_info.go:225-228`）。
- 错位自 `845a85998`（09-11）起。生产 `setting` 中 0 个渠道有 `reliability`，`settings` 中 35 个有。#26、#67 保存了 `max_attempts: 3`，09-18 起分别有 86、18 次在第 2 次尝试后放弃；#10、#58 保存了首内容阈值 60 秒，竞速从未触发。
- 竞速对 Claude Code 请求另有两处拦截：会话粘连规则 `claude cli trace` 的"失败不重试"标记（`controller/relay_hedge.go:56`，普通换渠道刻意忽略该标记，`service/relay_retry.go:36-38`）；粘连规则的请求头透传模板被当作参数覆盖（`controller/relay_hedge.go:62-64`，备用启动处 `:205-209` 同样拒绝）。#10 约 99% 的请求命中这两处。

### Requirements

- B1：后端以 `settings.reliability` 为唯一来源读取渠道可靠性设置，所有读取点一致。
- B2：保存渠道时，服务端按竞速规范 §2 与 `relaykit/dto/channel_reliability.go` 的范围校验 `settings.reliability`，越界拒绝保存。
- B3：移除 `setting` 中的 `reliability` 字段（生产无数据）。
- B6：`reliability` 成为 `settings` 的有类型字段，所有经 `SetOtherSettings` 回写 `settings` 的路径都保留它。
- B4：部署后，已保存的值立即生效（清单见 `research/hedge-config.md`「Values that take effect after the fix」）。
- B5：（已被子项 C 取代）竞速将整体移除，B 不为竞速新增任何行为或测试；`first_content_timeout_seconds` 暂留在 DTO 中，由 C 删除。

### Out of Scope

- 竞速相关的一切改动（由子项 C 处理）。
- 前端表单改动（前端已按 `settings.reliability` 读写）。
- 恢复 #27、#28 已丢失的 `max_attempts=3`（需在后台重新填写）。
- `GetOtherSettings()` 解析失败时整份重置 `settings` 的现有行为（`model/channel.go:1343-1354`）。后续事项：自动封禁事务持有渠道行锁时调用 `GetReliabilitySettings()`（`model/channel.go:424-432`），若 `settings` 解析失败会经全局 `DB` 执行 `channel.Save()`（`:1350`），在 MySQL/PostgreSQL 上会等待自身事务的行锁；B 之前 `GetSetting()` 已有同样模式，仅在旧的或手工编辑的坏数据上触发，建议另开任务处理。

### Acceptance Criteria

- [x] `settings.reliability` 中的最大尝试次数、自动封禁阈值与时长、三项超时在对应读取点生效；缺省时使用默认值。
- [x] 保存渠道时，越界的可靠性设置被服务端拒绝；合法值正常保存。
- [x] 经 `SetOtherSettings` 回写（如上游模型更新）后，`reliability` 不丢失。
- [x] 锁定旧 `setting` 列的测试改为 `settings` 后仍覆盖原场景；`go build ./...`、`cd relaykit && GOWORK=off go build ./...`、受影响包测试与 `-race` 全部通过。

### 部署后生效的值（生产核验，全部为范围内的整数）

| 渠道 | 生效值 |
|---|---|
| #26 FAST PLUS | 最大尝试 3 次 |
| #67 FAST PRO 订阅 | 最大尝试 3 次（当前手动禁用） |

#10 ZIVV CCMAX、#58 Rolldek image2.5 保存的首内容阈值 60 秒随子项 C 一起失效（竞速移除，B 与 C 同批部署）。其余 31 个带 `reliability` 的渠道全为默认值，17 个没有该字段。

---

## 子项 C：移除竞速功能（含首内容阈值）

状态：已实现（trellis-implement）并通过最终全量检查（trellis-check），已提交 `d4183f2bf`。移除清单见 `research/hedge-removal.md`，方案见 `design.md`，执行清单见 `implement.md`。

### Goal

删除从未运行过的竞速功能（代码、设置、前端、规范），生产行为不变；保留预提交闸门、换渠道与其余可靠性设置。

### Background（已确认事实）

- 竞速 09-18 上线（`b9692d6f4`），至今从未触发：日志无任何 `hedge attempt … launched`，`logs.other` 无 hedge 标记（`research/hedge-config.md` §3）；`general_setting.bill_hedge_losers` 在生产从未保存过；没有竞速专用的表、列或迁移。
- 即使修好字段错位，Claude Code 请求全部命中会话粘连规则而被竞速资格检查拒绝（`research/hedge-config.md` §4）；要让它生效需对粘连会话竞速，每次触发都要在备用渠道全量写一遍提示缓存（约 50 万 token 上下文约 ¥2.9），输家计费开启时可能双份收费。
- `b9692d6f4` 同时加入了流式静默超时、非流式总超时和几处通用修复，不能整体回退；移除范围按 `research/hedge-removal.md` 逐项列出（后端删 10 个文件、改 14 个，前端删 3 个、改 22 个，设置删 1 个、改 4 个）。
- 用户 2026-09-25 决定放弃竞速；首内容阈值不改作"超时即换渠道"，随竞速一起删除。上游迟迟不开始时，由上游报错（未回 200 或放行前流内报错，均透明换渠道）或流式静默超时（子项 D）兜底。

### Requirements

- C1：删除竞速的控制流、计费与日志：尝试调度、输家计费与读取、`other.hedge`、`hedge_*` 路由结果。
- C2：删除全局设置 `general_setting.bill_hedge_losers` 及其前端开关。
- C3：删除渠道设置 `first_content_timeout_seconds`（DTO、校验、默认值、前端表单、文案）。
- C4：闸门删除竞速分支，包括子项 A 的 R5 特例；此后所有尝试攒满上限都放行。
- C5：保留预提交闸门、换渠道、`settings.reliability` 的其余字段；竞速规范中仍有效的合同（`settings.reliability` 存储与取值范围、校验、静默超时与非流式超时语义）迁移到新规范 `.trellis/spec/backend/channel-reliability.md` 后删除竞速规范。
- C6：生产中已保存的 `first_content_timeout_seconds` 键不影响读取与保存，不做数据清理。
- C7：保留竞速提交中与竞速无关的改动：`logger/logger.go` 的数据竞争修复、`setting/config/config.go` 的配置锁、`model/log.go` ClickHouse 日志排序的补充排序键、`routing-reliability-section.tsx` 的保存失败返回。
- C8：随竞速文件一起消失、但覆盖通用行为的测试，用更小的测试直接保留其合同：`SumUsedQuota` 过滤、ClickHouse 日志排序、前端设置保存被拒后的重试。

### Acceptance Criteria

- [x] 非测试代码中不再有竞速相关符号（`Hedge`、`hedge_`、`bill_hedge_losers`、`FirstContentTimeoutSeconds`、`first_content_timeout_seconds`）；前端不再有竞速相关组件与文案。
- [x] 闸门：普通尝试攒满上限放行的测试保留，竞速候选相关分支与测试已删除。
- [x] C8 的三项合同各有测试覆盖。
- [x] 新规范 `channel-reliability.md` 覆盖 C5 列出的合同，`stream-precommit-gate.md` 等引用已更新，竞速规范已删除。
- [x] `go build ./...`、`cd relaykit && GOWORK=off go build ./...`、受影响包测试与 `-race`、前端类型检查与构建全部通过。

### Out of Scope

- 表单保存时主动删除已存的 `first_content_timeout_seconds` 键（后端忽略即可）。
- 跨渠道重试的总时长预算。

---

## 子项 D：渠道流式静默超时填 0 使用全局默认

状态：已实现（trellis-implement）并通过最终全量检查（trellis-check），已提交 `d4183f2bf`。

### Goal

渠道的流式静默超时填 0 时，沿用原版 New API 的全局默认（`STREAMING_TIMEOUT`，默认 300 秒），让挂住的上游被切走：放行前透明换渠道，放行后结束该流。

### Background（已确认事实，详见 `research/other-protocols.md` §8）

- 原版 New API（`upstream/main` `c2b7a9a9e`）：`STREAMING_TIMEOUT` 默认 300 秒且始终开启，在收到响应头后开始计时，每读到一行重置；`RELAY_TIMEOUT` 默认 0（不限，覆盖整个请求含流读取）；`RELAY_RESPONSE_HEADER_TIMEOUT` 默认 1800 秒（原版 `b518d0033`，2026-08-30，晚于 FlowAPI 分叉）；没有首字超时；上游 504/524 直接转给客户端不重试。
- FlowAPI：`STREAMING_TIMEOUT` 仍会解析（`common/init.go:182`），但自 `b9692d6f4`（竞速，09-17）起流式处理只读渠道的 `streaming_idle_timeout_seconds`、没有全局回退（`relay/helper/stream_scanner.go:123-133`）；`RELAY_TIMEOUT` 仍生效（`service/http_client.go:106-108`），生产未设置即不限；渠道非流式总超时在 `relay/channel/api_request.go:549-554`；没有等响应头超时；客户端断开会取消上游等待（`relay/channel/api_request.go:546-548`）；上游 504/524 视为 `upstream_timeout`，换下一个渠道并计入自动封禁（`service/relay_retry.go:62-63`）。
- 生产：FlowAPI 自身的静默超时从未触发过，包括 09-18 之前 300 秒默认值仍在生效时；上游 524 共 663 次、504 共 25 次；3 个请求被 nginx 900 秒（`proxy_read_timeout`）切断；ping 已开启，间隔 60 秒。

### Requirements

- D1：渠道 `streaming_idle_timeout_seconds` 为 0 或缺省时，使用全局 `STREAMING_TIMEOUT`（默认 300 秒，环境变量可改）；大于 0 时按渠道值（60–600 秒）生效。放行前触发仍为可重试的预提交失败（透明换渠道），放行后触发结束该流、不重放。
- D2：渠道 `non_streaming_timeout_seconds` 为 0 或缺省时沿用全局 `RELAY_TIMEOUT`（现有行为，无需改代码）。
- D3：前端两项超时的说明文字改为"0 表示使用全局默认"，各语言文案同步。

### Acceptance Criteria

- [x] 渠道静默超时为 0 时，全局 `STREAMING_TIMEOUT` 生效（测试中用较小的全局值验证）；渠道设为正数时按渠道值生效。
- [x] 放行前静默超时返回可重试的预提交失败；放行后静默超时结束流且不重放。
- [x] 前端说明文字与各语言文案已更新，`bun run i18n:sync` 无遗漏。
- [x] 受影响包测试、`go build ./...` 与前端类型检查、构建通过。

### Out of Scope

- 等响应头超时（原版 1800 秒大于前置 nginx 的 900 秒，实际不会生效）。
- 跨渠道重试的总时长预算（3 个请求因多次 524 累计到 nginx 900 秒）。
- 首字超时（随子项 C 删除首内容阈值，不另设）。

---

## 验证记录（最终全量检查，基于 `1f5bb7309`）

- `gofmt`、`go vet ./...`、`go build ./...`、`cd relaykit && GOWORK=off go build ./...` 全部通过；`go test ./... -count=1`：42 个包通过，0 失败。
- 竞态检测：`relay/helper`、`model` 通过；`controller` 只剩任务提交测试（`controller/task_submission_test.go`）的竞态。
- 前端：类型检查、构建、`bun run i18n:sync`（缺失 / 多余 / 未翻译均为 0）通过；受影响测试全部通过；全量 289 个测试中 281 个通过。
- 历史遗留失败：以下各项在未改动的 `1f5bb7309` 上都能复现，本任务不修，另开任务处理：
  - `relay/channel`、`relay/channel/gemini`、`relay/channel/minimax`：并行测试里调用 `gin.SetMode`；
  - `service` 视频任务轮询测试；
  - `controller` 任务提交测试：收尾时写 `common.RedisEnabled`，与后台统计协程同时读写；
  - 前端 8 个密钥 / 兑换码抽屉测试：Node v25 下 `localStorage.clear is not a function`，其中还有一个断言失败。

## 部署记录（2026-09-26 00:18 CST，HK）

- 部署前，本地 main 已快进到生产分支 `codex/flowapi-provider-coverage` 的 `1f5bb7309`（生产 `8156149df` 加一个部署记录提交），再叠加本任务的提交。
- 镜像：`flowapi:hk-stream-gate-c59af3457-20260926`（`sha256:71911a01b68c…`）；回滚镜像：`flowapi:rollback-stream-gate-20260925-161046`，指向 `hk-media-8156149df-20260925`。
- 备份：`/opt/flowapi/backups/pre-stream-gate-20260925-161046/`，包含 PostgreSQL custom dump（391 条 TOC）、Compose、`.env`、容器与镜像元数据、`SHA256SUMS`，校验全部通过。
- 只重建 app：20 秒后 healthy，重启 0 次；PostgreSQL / Redis 未变。
- 验证：
  - 版本正确；公网 `/api/status`、`/pricing`、`/playground` 均为 200，未鉴权的 `/v1/messages` 为 401；启动日志无 panic / fatal。
  - 切换后 4 分钟真实请求 14 个，全部成功，0 错误，无上限放行告警。
