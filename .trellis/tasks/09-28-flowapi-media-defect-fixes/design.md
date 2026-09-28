# Design: 修复 FlowAPI 媒体渠道缺陷

需求和决定以 `prd.md` 为准，证据在 `research/`。本文只写实现边界、数据流和取舍。

## 分组与边界

改动分成三组。三组不改同一个文件，可以并行实现，各自带测试。

| 组 | 内容 | 主要文件 |
|---|---|---|
| A 同步图片 / 音频 | R1 ASR 请求头；R2 Seedream `tools`；R5 Seedream Pro 图层计费 | `relay/channel/volcengine/asr.go`、`relaykit/dto/openai_image.go`、`relay/image_handler.go`、`relay/image_seedream.go`、`relay/channel/openai/relay_image.go` 及对应测试 |
| B MiniMax | R4a–R4e 上传与克隆；R7a TTS 计费字符上界；R8 删除音乐占位名 | `relay/channel/minimax/*`、`middleware/distributor.go`、`middleware/minimax_resource_route.go`、`controller/relay_minimax_extended.go`、`model/minimax_resource.go`、`model/media_billing_units.json` 及对应测试 |
| C 异步任务 | R3 HappyHorse UNKNOWN 宽限；R6 与 Wan 计费；H3 计费 | `service/task_polling.go`、`service/wan_video.go`、`relay/channel/task/{ali,sora,hailuo}/*`、`relay/relay_task.go`、`controller/relay.go`（只改 Wan 输入时长那一处）及对应测试 |

共享约束（AGENTS.md）：
- JSON 一律用 `common.*`，relaykit 内用 `kitutil`；
- 额度换算只用 `common/quota_math.go` 的 `*Checked` 系列，发生钳制时写入 `relayInfo.QuotaClamp` 或任务结算审计；
- 新测试用 testify 的 require / assert；
- 数据库代码兼容 SQLite、MySQL、PostgreSQL；
- relaykit 改动后必须通过 `cd relaykit && GOWORK=off go build ./...`。

## A 组

### R1 ASR

删除 `asr.go:408` 和 `:486` 的 `X-Api-Sequence`，其余请求头不动。

测试直接调用 `asrSubmit` 和 `asrQueryOnce`，并替换 `asrHTTPClient`：
- 用 header map 断言没有 `X-Api-Sequence` 这个 key；
- 断言 Access-Key、App-Key、Resource-Id、Request-Id、Content-Type 不变。

### R2 Seedream `tools`

- **DTO**：`ImageRequest` 加一个字段 `Tools json.RawMessage \`json:"tools,omitempty"\``。`GetJSONFieldNames` 会自动把它排除出 `Extra`。
- **非 Seedream 模型清掉 `tools`**：
  - 位置在 `relay/image_handler.go` 非透传分支里，调用 `ConvertImageRequest` 之前（这样 volcengine、openai、newapi 渠道都能覆盖）；
  - 条件是 `!dto.IsSeedreamModel(info.UpstreamModelName)` 时把 `request.Tools` 置空；
  - 透传分支按「原样发送」的语义处理，不改。
- **Pro 删除 `tools`**：在 `normalizeSeedreamImagePayload` 里对 Pro 执行 `delete(fields, "tools")`，与 `sequential_*` 的处理方式相同。
- **测试**：
  - 在 `relay/channel/volcengine/seedream_test.go` 的样例中加入 `tools`，用 JSONEq 断言保留；
  - 新增断言：非 Seedream 模型不转发 `tools`；
  - 新增断言：Pro 的 normalize 会删除 `tools`。

### R5 Pro 图层计费

修改 `relay/channel/openai/relay_image.go:91-111` 的校验：
- `success` = `data` 中带 `url` 或 `b64_json` 且没有 `error` 的项数。
- `generated` 合法（整数，1..17）时，计费张数 = `min(success, generated)`；不合法或缺失时，计费张数 = `success`。计费张数上限 17。
- `success == 0` 时，保持现有的 502 + skip-retry + 退款。
- `success ≥ 1` 时，原样写回完整响应体，结算倍率设为 `n = 计费张数 × SeedreamLayerImagePriceFactor`。
- 日志写入 `success`、`generated` 和上游 request id。

测试：
- 调整 `seedream_image_test.go:51-69`；
- 覆盖以下场景，并断言精确的 `n`：含 error 项、`success < generated`、`success == 0`、`generated` 缺失、`generated` 越界。

归档任务里的 `billing-contract.md:22-23` 在收尾时更新到规范。

## B 组

### R4a 上传按用途路由

- **位置**：distributor 在给 `/v1/files/upload` 解析 model 时，按 multipart 里的 `purpose` 选定一个只用于选渠道的「路由模型」：
  - `voice_clone`、`prompt_audio` → `minimax-voice-clone`；
  - `video_generation_input`、`video_generation` → `MiniMax-H3`；
  - 其他 purpose 保持 `minimax-files-upload`。
- **只影响选渠道**：
  - `OriginModelName` 和计价仍按 `minimax-files-upload`，上传免费（controller 现在已经强制这一点）；
  - 令牌的模型限制仍按 `minimax-files-upload` 检查，不能因为改了路由模型就绕过或多出限制。
- **测试**：两个 type 35 渠道优先级不同，`voice_clone` 用途的上传落到挂克隆模型的渠道，`video_generation_input` 的上传落到挂 H3 的渠道。

### R4b purpose 白名单与大小上限

- 白名单：`voice_clone`、`prompt_audio`、`t2a_async_input`、`video_understanding`、`video_generation_input`，外加兼容值 `video_generation`。
- 大小上限按官方用途设置：
  - `video_generation_input`、`video_generation`：按文件的 MIME 类型区分，视频 50MB、图片 30MB、音频 15MB；
  - 其余用途保持 20MB。
  - 不超过全局 `MAX_REQUEST_BODY_MB`。
- 测试：表驱动，覆盖每种 purpose 和各自的大小边界。

### R4c `file_id`

- 入站改用自定义 `UnmarshalJSON`，同时接受数字和字符串。拒绝：非整数（例如 1.5）、≤0、超出 int64 的值。
- 发给上游仍用数字。
- 测试：数字、字符串、1.5、负数、超出 int64。

### R4d 克隆字段

- 出站字段改为 `need_noise_reduction`，入站两个名字都接受。
- `accuracy` 改为 `*float64`，取值 [0,1]，越界返回 400。
- 转发 `clone_prompt`、`text_validation`、`language_boost`。其中 `clone_prompt.prompt_audio` 是账号资源，要走与 file_id 相同的资源归属校验。
- 试听 `text` 上限 1000。
- `voice_id` 正则的最后一位不能是 `-` 或 `_`。
- 更新 `media_test.go` 里固化旧字段名的断言。

### R4e 释放 voice_id

- 上游明确拒绝时（HTTP 4xx，或 `base_resp.status_code != 0`），删除这次克隆写入的 pending 绑定。
- 网络错误、响应读不出来等结果不确定的情况，保留 pending。
- 如果 model 层没有删除函数，就新增一个，按 user、kind、id 和 state=pending 条件删除，三种数据库都兼容。
- 测试：上游拒绝后，同一个 voice_id 可以再次克隆。

### R7a TTS 计费字符

- **本地加权计数**：Han 字符算 2，其余 rune 算 1。只用于 MiniMax speech 模型。
- **结算**：
  - 上游 `usage_characters` 满足 `0 < v ≤ 加权计数 + 余量` 时用上游值，余量取 16，覆盖停顿标记这类计数差异；
  - 缺失、≤0 或超过上界时，改用加权计数，写 `common.SysError`，并把钳制写入 `relayInfo.QuotaClamp`。
  - 不再因为计数本身返回 502，只要上游成功就返回音频。
- **预扣**：MiniMax speech 按加权计数估算，与结算口径一致。
- **测试**：
  - 5001 个汉字：返回音频，按上游计数结算；
  - 上游计数缺失：按加权计数；
  - 上游计数异常偏大：钳制到加权计数，并留审计。

### R8 音乐占位名

- `/v1/music_generation` 没带 model 时返回 400「model is required」，不再默认填占位名（`middleware/distributor.go:435-437`）。
- 从 `relay/channel/minimax/constants.go` 的模型列表和 `model/media_billing_units.json` 中删掉占位名。
- 音乐转发代码保留。
- 生产上删除 71 号渠道的音乐模型和价格，列入上线清单。

## C 组

### R3 HappyHorse UNKNOWN 宽限

- **可选接口**：新增 `TaskUnknownGraceAdaptor { UnknownStatusGraceSeconds() int64 }`，只由 ali 适配器实现，返回 120。
- **轮询 UNKNOWN 分支**：
  - 适配器实现了该接口，且 `now - SubmitTime > grace`（`SubmitTime` 为 0 视为已超窗）：填写失败原因，按 FAILURE 处理并退款，沿用现有 CAS，保证只退一次；
  - 宽限内，或适配器没实现该接口：保持现状。
- **测试**：
  - 宽限内保持；
  - 超窗判失败，且只退款一次；
  - H3、FastAI、Wan 的 UNKNOWN 行为不变。

### R6 与 Wan 计费

- **预扣**（`relay/channel/task/sora/adaptor.go:108-111`）：秒数 = 请求秒数 + 参考视频秒数。
- **参考视频秒数**（在 `relay/channel/task/sora/wan.go:40-58` 里计算）：
  - 按 URL 去重，每个 URL 只探测一次；
  - 每段时长向上取整到整秒再求和；
  - 15 秒和 30 秒上限都按去重后的值判断；
  - 结果写入 context 和 `BillingContext.InputVideoDuration`。
  - `controller/relay.go:784` 写入的是去重、取整后的值。
- **结算**（`service/task_polling.go:538-549`、`service/wan_video.go:134-160`）：
  - 成片秒数：上游 `output_video_duration` 在 (0, 30] 内时用上游值；否则直接用请求秒数，不再下载探测，写审计标记「时长未核验」，按成功结算并写回视频地址。
  - 计费秒数 = 成片秒数 + `InputVideoDuration`。
  - 旧任务判断：快照 `OtherRatios.seconds` 等于请求秒数、而 `InputVideoDuration > 0`，说明是上线前提交的，参考视频不计费，只按成片结算。实现时统一用一个判断：新任务在快照里写入标记 `wan_input_billed=true`，没有这个标记的按旧公式结算。
  - 替换 `service/wan_video_test.go:145-170` 中固化「挂起」行为的测试。
- **测试**：
  - 不带参考视频时，收费与原来一致；
  - 带参考视频时，预扣和结算都包含参考秒数；
  - 同一 URL 只计一次、每段向上取整；
  - 上游缺时长时按请求时长结算成功，并写回 URL；
  - 旧快照不补扣；
  - 重复终态不重复结算。

### H3 计费

- **倍率**：`relay/channel/task/hailuo/constants.go:60` 的 2K 倍率改为 5/3，同步 `adaptor.go:544` 和 `:551-565` 的注释。
  - 额度必须精确：0.15 × 5/3 × QuotaPerUnit 要等于 125000，不能因浮点截断得到 124999。用 decimal 计算，或把倍率与基准价的乘积先四舍五入到安全精度，例如 `QuotaFromDecimal`。测试断言 2K 1 秒 = 125000、768P 1 秒 = 75000。
- **预扣**：
  - 请求带参考视频时，`hailuo-h3-duration` 取输出秒数 + 15。15 是官方的参考视频总时长上限。
  - 不带参考视频时，只取输出秒数。
  - 图片和音频不计。
- **结算**：hailuo 适配器实现 `TaskBillingCheckedAdaptor`（参照 `relay/channel/task/volcvod/billing.go`），从 `task.Data` 读 `task.usage`：
  - `output_seconds` 在 4..15 内，且 `input_seconds` 在 0..15 内时，计费秒数 = 两者之和，按快照里的单价、分辨率倍率和分组倍率重算，退还多预扣的部分。
  - 请求带了参考视频、上游却报 `input_seconds == 0` 时，这个 0 不可信：参考视频按 15 秒计，也就是保留预扣。
  - usage 缺失或越界时，保留预扣。
  - 不补扣超过预扣的部分：预扣已经是上界，而 `RecalculateTaskQuota` 补扣时不检查余额。
- **旧任务**：快照里没有 H3 新标记的任务，保持原来「按次、结算不调整」的行为。
- **测试**：
  - 倍率精确；
  - 带参考视频预扣 +15；
  - usage 可信时按实际秒数结算并退差额；
  - `input_seconds == 0` 且请求带参考视频时保留预扣；
  - usage 缺失时保留预扣；
  - 旧任务不调整。

## 上线清单（本任务只列出，不执行）

1. HK `options.ModelPrice["MiniMax-H3"]` 从 0.19 改为 0.15。导入清单 `target-pricing-proposal.json` 同步，B 复用这份清单。
2. HK 71 号渠道删除全部音乐模型，并删除这些模型和占位名的价格。
3. 部署前备份：数据库 dump、compose 和镜像回滚 tag，按 servers.md 的 FlowAPI 部署惯例执行。
4. 上线后实测：ASR、声音克隆（上传→克隆）、H3 带 `mm_file` 参考素材的请求、Wan 带参考视频的请求。

## 回滚

三组都是纯代码改动，没有数据库结构变更。R4e 如果新增删除函数，也不涉及表结构。回滚方式是切回上一版镜像。

H3 和 Wan 的新旧任务通过快照标记区分。回滚后，新标记任务仍按旧逻辑处理，只会少退差额，不会多扣。
