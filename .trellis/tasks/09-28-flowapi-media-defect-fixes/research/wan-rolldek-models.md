# Research: Wan 的 Rolldek 协议——模型名映射、计费口径、接口文档核对

- **Query**：`wan-rolldek-v1` 发给 Rolldek 的 model 是什么、每种输入组合落到哪个 Rolldek 模型；FlowAPI 现行预扣和结算；与 Rolldek 价目对比是否亏本；改成「成片 + 参考视频」计费要改哪里；对照 Rolldek WAN 3.0 接口文档逐条核对；参考视频秒数是否按 URL 去重；探测是否要求公网 HTTPS
- **Scope**：internal（FlowAPI main `10d32a657`，只读），附上游 new-api `upstream/main` 作参照。从发布提交 `8156149df` 到 HEAD，本文涉及的 Wan 相关文件没有改动（只有 `controller/relay.go` 删掉了与 Wan 无关的 hedge 分支），所以生产行为与本文描述一致
- **Date**：2026-09-28
- **前提**：币种已由用户确认为人民币。FlowAPI 价格也按人民币显示（`target-pricing-proposal.json:3`：`"currency": "CNY display / USDExchangeRate=1"`），`QuotaPerUnit = 500000` 对应 1 元（`common/constants.go:22`）。Wan 改为按「成片时长 + 参考视频时长」计费，用户已确定。
- **Rolldek 文档来源**：用户贴出、由主线转述的要点（本文第 5 节）。原文未见，与原文有出入的地方以原文为准。

## 结论速览

1. **模型名**：`model` = 公开模型名 + `-` + 小写分辨率（`wan.go:71`），与输入素材无关。所以只会落到 `wan3.0-video-{480p,720p,1080p}` 或 `wan3.0-video-prime-{…}`，**从不发送 `wan3.0-image-*`**。
2. **现行计费**：预扣 = ModelPrice × 分组倍率 × 请求秒数 × 分辨率倍率；结算 = 同一公式，只把请求秒数换成实际成片秒数。参考视频时长在提交时测出，存入 `BillingContext.InputVideoDuration`，但**不参与计价**，只用于检查「成片 + 参考 ≤ 30 秒」。
3. **与 Rolldek 的差异**：没有参考视频时，分组倍率为 1 或 1.2 的所有档位都赚钱，480P 标准版利润最薄，每秒 0.03 元。带参考视频时，Rolldek 按「成片 + 参考」收费，FlowAPI 只收成片秒数，**6 个档位都可能亏**。最坏情况是成片 2 秒、参考 15 秒，单次亏 2.48 至 4.66 元。改成按「成片 + 参考」计费后，分组倍率 ≥ 0.9 时所有档位都不亏。
4. **修改落点**：预扣改 `EstimateBilling`（`adaptor.go:108-111`），结算改 `wanActualQuotaChecked`（`wan_video.go:134-160`）。参考视频时长可直接复用提交时的测量结果，预扣读 `c.GetFloat64("wan_input_video_seconds")`，结算读 `bc.InputVideoDuration`。FlowAPI 只发 video 系列，而且 Rolldek 的 image 系列不允许参考视频，所以一个公式对所有请求都适用，不需要按系列分支。
5. **接口文档核对**：端点、鉴权、`seconds` 字段、模型后缀、状态值和 `metadata.url` 一致。不一致的有 7 项：计费不含参考视频；参考视频不按 URL 去重；允许 `http://` 和 `data:image/`；允许 4:3 和 3:4；可能发出空 prompt；多发了 `watermark` 字段；超时 24 小时（Rolldek 建议 10–15 分钟）。
6. **追加问题**：参考视频秒数**不按 URL 去重**（`wan.go:40-58`）。`ProbeWanMediaDuration` **不要求 HTTPS**，`http` 也放行（`wan_video.go:26`）；「必须是公网地址」只在默认开启 SSRF 防护时成立。

---

## 1. 模型名映射与上游请求

### 1.1 调用链（按执行顺序）

| 步骤 | 位置 | 与 Wan 相关的行为 |
|---|---|---|
| 路由 | `router/video-router.go:15` | `POST /api/v1/services/aigc/video-generation/video-synthesis` 依次经过 TaskSubmission、Distribute、RelayTask |
| 选渠道 | `middleware/distributor.go:318-330` | 用请求体里的 `model` 选渠道 |
| 适配器 | `relay/relay_adaptor.go:140-146,169-170` | 渠道类型 1（OpenAI）使用 `tasksora.TaskAdaptor` |
| 协议开关 | `relay/channel/task/sora/adaptor.go:77` | `wanProtocol = VideoProtocol == "wan-rolldek-v1"` |
| 校验和测时长 | `adaptor.go:94-96` → `wan.go:20-66` | 解析 `dto.WanRequest`，探测参考视频和参考音频的时长 |
| 模型映射 | `relay/relay_task.go:171-175` | 执行 `ModelMappedHelper`，但其结果随后会被第 8 步覆盖 |
| 定价 | `relay_task.go:188-195` | 按公开模型名（`OriginModelName`）取 ModelPrice；Wan 必须配置 ModelPrice |
| 预扣 | `relay_task.go:201-237` | EstimateBilling，然后乘各项倍率，然后预扣 |
| 构造请求体 | `relay_task.go:240` → `wan.go:68-78` | **在这里改写 `info.UpstreamModelName`** |
| 发送 | `relay_task.go:275` → `relay/channel/api_request.go:643-671` | 使用 `BuildRequestURL` 和 `BuildRequestHeader` |
| 不重试 | `controller/relay.go:826` | Wan 路径一律不重试 |

### 1.2 model 字段规则

```go
// relay/channel/task/sora/wan.go:70-72
// Model aliases come exclusively from the public model + validated resolution, never caller metadata.
info.UpstreamModelName = req.Model + "-" + strings.ToLower(req.Parameters.Resolution)
body := map[string]any{"model": info.UpstreamModelName, "prompt": ..., "seconds": strconv.Itoa(*req.Parameters.Duration), "size": req.Parameters.Resolution, "aspect_ratio": req.Parameters.Ratio, "watermark": false}
```

- `req.Model` 只能是 `wan3.0-video` 或 `wan3.0-video-prime`（`dto/wan.go:14,40-42`）。
- 分辨率只能是 `480P`、`720P`、`1080P`，缺省为 `1080P`（`dto/wan.go:50-55`）。
- 构造请求体发生在模型映射之后（`relay_task.go:173` 早于 `:240`），而且用的是 `req.Model`，因此**渠道即使配置了 model_mapping，也不会影响发给上游的模型名**。
- 单元测试把这条规则固定了下来：`wan_test.go:20-50`，用首尾帧输入，断言 `model == name+"-"+小写分辨率`（`:39`）。
- 发给上游的模型名会写入 `Properties.UpstreamModelName`（`model/task.go:226-227`）。结算日志把它记为 `upstream_model_name`（`service/task_billing.go:154-158`），可以据此核对每个任务实际用了哪个 Rolldek 模型。

### 1.3 输入组合与 Rolldek 模型的对应

FlowAPI 允许的输入类型和数量见 `dto/wan.go:72`：首帧 1、尾帧 1、参考图 10、参考视频 5、参考音频 5。另有以下约束：

- 首尾帧模式与参考模式互斥（`dto/wan.go:95-98`）；
- 有尾帧时必须有首帧（`:92-94`）；
- `file`、`link` 类型和 `oss:` 地址在 Rolldek 渠道直接拒绝（`wan.go:41-43`）。

下表中 `{v}` 表示 `video` 或 `video-prime`，由公开模型名决定；`{res}` 为 `480p`、`720p` 或 `1080p`。

| 输入组合 | FlowAPI 能否通过 | 发送的 model | Rolldek 计费口径 |
|---|---|---|---|
| 纯文本 | 能 | `wan3.0-{v}-{res}` | 成片秒数 |
| 首帧 | 能 | `wan3.0-{v}-{res}` | 成片秒数 |
| 首帧 + 尾帧 | 能 | `wan3.0-{v}-{res}` | 成片秒数 |
| 参考图 1–10 张 | 能 | `wan3.0-{v}-{res}` | 成片秒数 |
| 参考图 + 参考音频 | 能 | `wan3.0-{v}-{res}` | 成片秒数（参考音频不计秒） |
| 只有参考音频 | 能 | `wan3.0-{v}-{res}` | 成片秒数 |
| 参考视频 1–5 段（可再加参考图或参考音频） | 能 | `wan3.0-{v}-{res}` | **成片秒数 + 全部参考视频秒数** |
| 首帧或尾帧与任一参考素材混用 | 400 | — | — |
| 只有尾帧 | 400 | — | — |
| file、link 或 oss 地址 | 400 | — | — |

- 用户价目表中 image 系列与 video 系列的单价逐档相同（标准版 0.27/0.34/0.36 元，高速版 0.30/0.38/0.40 元）。
- 因此，只用图片或音频的请求即使走 video 系列，成本也与走 image 系列相同。只有带参考视频时，成本才多出参考视频的秒数。
- Rolldek 是否接受首尾帧请求走 video 系列：**未确认**。发布冒烟只记录了「2 秒成片」，没有记录输入组合（`release-gate-2026-09-25.md:18,27-28`）。

### 1.4 完整 URL、请求头与请求体

- **Base URL**：`https://rolldek.com`，类型 1，分组 `vedio`，`video_protocol=wan-rolldek-v1`，无 model_mapping。
  - 来源是导入清单 `09-22-flowapi-pricing-verification/flowapi-disabled-channels.json:283-300`，当时的源渠道号为 75。生产渠道 83 就是从这份清单导入的（`release-gate-2026-09-25.md:26`）。
  - 生产库里渠道 83 的实际值未直接查看：**未确认**。
- **提交**：`POST {base}/v1/videos`（`adaptor.go:145-150`；Wan 的 action 为 generate，见 `wan.go:64`，不会进入 remix 分支）。
- **查询**：`GET {base}/v1/videos/{upstream_task_id}`（`adaptor.go:285`），每 15 秒一轮（`controller/system_task_handlers.go:146`）。
- **成片内容**：
  - FlowAPI 的 `GET /v1/videos/{public_task_id}/content`（`router/video-router.go:22`）会代理到 `{base}/v1/videos/{upstream_id}/content`，携带 `Bearer channel.Key`（`controller/video_proxy.go:114-116`）。
  - DashScope 风格的 `GET /api/v1/tasks/{id}` 直接返回存下的签名直链作为 `video_url`（`relay/relay_task.go:465`）。
- **请求头**（`adaptor.go:153-157`）：
  - `Authorization: Bearer {key}`；
  - `Content-Type` 照抄客户端的值。客户端必须发 `application/json`，否则请求体不会被解析，校验会以 400 失败（`common/gin.go:136-145`）。
  - 客户端必须带的 `X-DashScope-Async: enable`（`wan.go:24-26`）和可选的 `X-Keli-Submission-Key`（`middleware/task_submission.go:25`）都不会转发给上游。
- **请求体**（`wan.go:72-75`）：

| 字段 | 取值 |
|---|---|
| `model` | `wan3.0-video[-prime]-{480p,720p,1080p}` |
| `prompt` | 客户端的 `input.prompt`，超过 20000 字符截断（`dto/wan.go:46-48`）。**有媒体时可以为空字符串**（`dto/wan.go:43-45`） |
| `seconds` | 字符串形式的整数，范围 2–30，缺省 `"5"`（`dto/wan.go:62-68`） |
| `size` | `480P`、`720P` 或 `1080P`（大写 P） |
| `aspect_ratio` | `adaptive`（缺省）、`16:9`、`9:16`、`1:1`、`4:3`、`3:4`（`dto/wan.go:56-61`） |
| `watermark` | 固定为 `false` |
| `input.media` | 有媒体时原样转发 `[{type,url}]`（`wan.go:73-75`；`dto/wan.go:16-19`） |

- `reference_images`、`reference_videos`、`reference_audios` 这三个字段都不会发送。
- 以下非默认参数一律以 400 拒绝（`wan.go:35-38`）：`audio=false`、非 `-1` 的 seed、`prompt_extend=false`、`watermark=true`。

---

## 2. FlowAPI 现行计费

### 2.1 预扣（提交时，按请求秒数）

```
Quota_pre = trunc( ModelPrice × 500000 × G × O_req × r )
```

| 符号 | 含义与来源 |
|---|---|
| ModelPrice | 按公开模型名取价：`wan3.0-video` 为 0.6，`wan3.0-video-prime` 为 0.9，单位元/秒，以 720P 为基准（`relay/helper/price.go:213,242`；清单见 `target-pricing-proposal.json:58-59`）。缺价时拒绝请求（`relay_task.go:193-195`） |
| G | 有效分组倍率 = 分组倍率 × 用户分组倍率 × 渠道倍率（仅当开启「计入渠道倍率」时才乘渠道倍率），见 `relay/helper/price.go:88` 和规范 `.trellis/spec/backend/billing-ratio-routing.md:44-55` |
| O_req | 请求秒数，`EstimateBilling` 给出 `{"seconds": O_req, "resolution": r}`（`adaptor.go:108-111`） |
| r | 分辨率倍率：480P 为 0.5，720P 为 1，1080P 为 2（`dto/wan.go:108-117`） |

- 计算过程：先乘各项倍率（`relay_task.go:218-225`；`types/price_data.go:84-96`），再用 `QuotaFromFloatChecked` 截断。
- 预扣额度在提交成功后立即结算为实扣（`controller/relay.go:767`），然后写入 `task.Quota`（`:786`）。
- 发布冒烟记录的 Wan 提交额度是 360000，即 0.72 元，成片 2 秒（`release-gate-2026-09-25.md:27`）。这与「标准版、480P、G=1.2」吻合（0.6 × 1.2 × 2 × 0.5 = 0.72），但也有其他组合能得到同样结果，所以 `vedio` 分组的实际倍率**未确认**。

### 2.2 结算（完成时，按实际成片秒数）

```
Quota_actual = round( ModelPrice × G_snapshot × r × O_act × 500000 )    // service/wan_video.go:134-160
delta = Quota_actual − task.Quota   → 多退少补                           // service/task_billing.go:232-299
```

- **O_act 的来源**：优先取上游 `usage.output_video_duration`，但超出 (0, 30] 的值会被清零（`wan.go:96-98,109-112`）；清零或缺失时下载成片探测视频轨时长（`task_polling.go:539-542`）。
- **核验条件**：要求 `O_act + InputVideoDuration ≤ 30`，否则挂起（`task_polling.go:543-547`）。R6 已决定这种情况改为按请求时长结算。
- **G 取提交时的快照**（`wan_video.go:139-142`）；r 取快照里的 `OtherRatios["resolution"]`（`:143-146`）。快照中的 `OtherRatios["seconds"]` 不参与结算。
- **分支顺序**：`BillingUnit=="output_seconds"` 的判断排在 PerCallBilling 之前（`task_polling.go:704-708`）；Wan 快照的 `PerCallBilling` 被显式置为 false（`relay_task.go:251`；`controller/relay.go:782`）。
- **跳过调整**：`wanActualQuotaChecked` 返回 0 时（O_act 缺失或越界），`RecalculateTaskQuota` 直接返回（`task_billing.go:233-235`），保持预扣额度。

### 2.3 参考视频时长：怎么测、存在哪、有没有用

| 环节 | 位置 | 说明 |
|---|---|---|
| 测量 | `wan.go:39-58` | 对每个 `reference_video` 调 `ProbeWanMediaDuration(..., true)`，取 MP4 视频轨时长（mdhd 时长除以 timescale，见 `service/wan_video.go:87-130`）。单段须在 1–15 秒（`wan.go:49-51`），总计 ≤ 15 秒，且总计加 O_req ≤ 30 秒（`:59-61`）。参考音频另行累加，不计费 |
| 暂存 | `wan.go:63` | `c.Set("wan_input_video_seconds", videoSeconds)`，值为浮点数，例如 4.97 |
| 持久化 | `relay_task.go:253`（提交派发快照）；`controller/relay.go:784`（任务行） | 写入 `TaskBillingContext.InputVideoDuration`（`model/task.go:123`，`json:"input_video_duration,omitempty"`） |
| 计价 | `wan_video.go:134-160` | **没有使用** |
| 其他用途 | `task_polling.go:543`；`task_billing.go:130`；`relay_task.go:475` | 30 秒上限核验；计费日志的 `other.input_video_duration`；客户端响应的 `usage.input_video_duration` |

---

## 3. 与 Rolldek 口径的差异（人民币，元）

符号：O 为成片秒数，V 为参考视频秒数。FlowAPI 每秒价格 = ModelPrice × r × G。

### 3.1 每秒单价对比（无参考视频）

| 档位 | FlowAPI（G=1） | FlowAPI（G=1.2） | Rolldek | 每秒利润（G=1 / G=1.2） | 保本分组倍率 |
|---|---|---|---|---|---|
| 标准版 480P | 0.30 | 0.36 | 0.27 | +0.03（毛利率 10%）/ +0.09 | 0.90 |
| 标准版 720P | 0.60 | 0.72 | 0.34 | +0.26 / +0.38 | 0.567 |
| 标准版 1080P | 1.20 | 1.44 | 0.36 | +0.84 / +1.08 | 0.30 |
| 高速版 480P | 0.45 | 0.54 | 0.30 | +0.15 / +0.24 | 0.667 |
| 高速版 720P | 0.90 | 1.08 | 0.38 | +0.52 / +0.70 | 0.422 |
| 高速版 1080P | 1.80 | 2.16 | 0.40 | +1.40 / +1.76 | 0.222 |

没有参考视频时，G=1 和 G=1.2 下所有档位都不亏。

### 3.2 带参考视频（现行 FlowAPI 只收 O，Rolldek 收 O+V）

- 单次利润 = O × p_F × G − (O + V) × p_R。
- 当 **V/O > p_F·G/p_R − 1** 时亏本。
- FlowAPI 的边界：O 为 2–30，单段参考视频 ≥ 1 秒，V ≤ 15，O + V ≤ 30，所以 V/O 最大为 7.5（O=2、V=15）。**6 个档位都可能亏**。

| 档位 | 亏本阈值 V/O（G=1 / G=1.2） | O=5、V=1 | O=5、V=5 | O=10、V=5 | O=5、V=15 | O=2、V=15（最坏） |
|---|---|---|---|---|---|---|
| 标准版 480P | 0.111 / 0.333 | −0.12 / +0.18 | −1.20 / −0.90 | −1.05 / −0.45 | −3.90 / −3.60 | −3.99 / −3.87 |
| 标准版 720P | 0.765 / 1.118 | +0.96 / +1.56 | −0.40 / +0.20 | +0.90 / +2.10 | −3.80 / −3.20 | −4.58 / −4.34 |
| 标准版 1080P | 2.333 / 3.0 | +3.84 / +5.04 | +2.40 / +3.60 | +6.60 / +9.00 | −1.20 / 0.00 | −3.72 / −3.24 |
| 高速版 480P | 0.5 / 0.8 | +0.45 / +0.90 | −0.75 / −0.30 | 0.00 / +0.90 | −3.75 / −3.30 | −4.20 / −4.02 |
| 高速版 720P | 1.368 / 1.842 | +2.22 / +3.12 | +0.70 / +1.60 | +3.30 / +5.10 | −3.10 / −2.20 | −4.66 / −4.30 |
| 高速版 1080P | 3.5 / 4.4 | +6.60 / +8.40 | +5.00 / +6.80 | +12.00 / +15.60 | +1.00 / +2.80 | −3.20 / −2.48 |

各示例列的值为单次利润，格式为「G=1 / G=1.2」。

- 以 O=5、V=5 为例，FlowAPI 少收的是 V 秒 × Rolldek 秒价，例如标准版 720P 少收 5 × 0.34 = 1.70 元。
- **480P**：
  - 标准版 G=1 时，只要 V/O > 0.111 就亏，例如成片 10 秒配 2 秒参考视频；G=1.2 时阈值为 0.333。
  - 高速版的阈值为 0.5（G=1）和 0.8（G=1.2）。
- **720P**：
  - 标准版的阈值为 0.765（G=1）和 1.118（G=1.2）；
  - 高速版的阈值为 1.368（G=1）和 1.842（G=1.2）。
  - 参考视频与成片等长或更长时，720P 就开始亏。

### 3.3 改为按 O+V 计费之后

- 单次利润 = (O + V) × (p_F·G − p_R)。G=1 或 G=1.2 时 6 个档位都为正，保本分组倍率同 3.1 表。
- 最薄的是标准版 480P、G=1：每秒 0.03 元，毛利率 10%。例如 O=2、V=15 时收 5.10 元，成本 4.59 元，利润 0.51 元。
- 这 10% 的余量是否足以覆盖 Rolldek 与 FlowAPI 在测量和取整上的差异，**未确认**，见文末注意事项。

---

## 4. 修改落点：计费秒数 = 成片 + 参考视频

### 4.1 适用范围

- FlowAPI 只发 video 系列（`wan.go:71`），而 Rolldek 的 video 系列恰好按 O+V 计费。
- 没有参考视频的请求 V=0，结果与现在相同。
- Rolldek 的 image 系列不允许参考视频，所以将来即使接入 image 系列，也有 V=0。
- 因此**一个公式适用于所有 Wan 请求，不需要按系列分支**。
- 参考音频不计入 V（`wan.go:52-56` 已分开累加），与 Rolldek「参考音频不计秒数」一致。

### 4.2 预扣

- **改动位置**：`relay/channel/task/sora/adaptor.go:108-111`，把 `seconds` 改为 `float64(*Duration) + c.GetFloat64("wan_input_video_seconds")`。
- **时序可行**：`ValidateRequestAndSetAction` 在 `relay_task.go:160` 已写入该值，`EstimateBilling` 在 `:201` 才调用，两者共用同一个 gin context，**可以直接复用，不需要重新下载**。
- **公式**：Quota_pre = trunc(ModelPrice × 500000 × G × (O_req + V) × r)。
- **倍率只能相乘**（`types/price_data.go:84-96`），加法项只能并进 `seconds`，不能另开一个倍率键。
- **有界**：`wan.go:59` 已保证 V ≤ 15 且 O_req + V ≤ 30，满足 AGENTS.md「计费乘数必须有界」的要求。
- **连带变化**：以下三处的 `seconds` 会同步变为 O_req + V：
  - 快照 `OtherRatios.seconds`（`model/task.go:150`）；
  - 提交日志的「计算参数」（`service/task_billing.go:27-37`）；
  - 响应头 `X-New-Api-Other-Ratios`（`relay_task.go:295-300`）。

### 4.3 结算

- **改动位置**：`service/wan_video.go:134-160`，把 `bc.OutputVideoDuration` 换成 `bc.OutputVideoDuration + bc.InputVideoDuration`：`value = ModelPrice × ratio × resolution × (O_act + V) × QuotaPerUnit`。
- **V 读自任务快照**（`controller/relay.go:784`），符合规范「异步任务只读提交快照」（`.trellis/spec/backend/billing-ratio-routing.md:55`）。
- **防御校验**：
  - 现有的非法因子检查（`:150-156`）和 O 的边界检查（`:136`）都不覆盖 V；
  - AGENTS.md 要求媒体元数据时长也要做饱和或拒绝处理，V 同样属于这类时长；
  - 快照里的 V 出现 NaN、负数或越界时，应与其他因子一样走 `QuotaRoundChecked(NaN)` 的可审计钳制。
- **与 R6 的衔接**：
  - R6 决定无法核验时「按请求时长结算」。改动后，这里的请求时长应为 O_req + V。
  - 如果 R6 采用保留预扣的做法，预扣已含 V，自然成立。
  - 如果 R6 把 O_act 设为 O_req 再走 `wanActualQuotaChecked`，也会得到 O_req + V。
  - Rolldek 文档没说完成响应里是否带时长字段。如果确实不带，R6 之后大多数任务的结算额会等于预扣额。

### 4.4 其他影响与测试

- **上线时仍在跑的旧任务**：
  - 自 `8156149df` 起，旧任务的快照里已有 `InputVideoDuration`，但预扣只含 O；
  - 新的结算公式会对这些任务按 O+V 补扣差额（`task_billing.go:253-262`，delta > 0 时扣减用户额度）；
  - 旧快照的 `OtherRatios.seconds` 等于 O_req，新快照等于 O_req + V，可以据此区分新旧任务。
  - 是否对旧任务补扣，需要决定。
- **客户端响应**：`relay_task.go:475` 中的 `usage.duration` 现在等于 O。上游插件把 `usage.duration` 当作成片时长的后备值（`upstream/main:plugins/tasks/alibaba/plugin.js:1123`），所以可以不改。
- **测试位置**：
  - `relay/channel/task/sora/wan_test.go:46`：V=0 时 EstimateBilling 结果不变，需新增 V>0 的用例；
  - `service/wan_video_test.go:58-90`：结算公式、钳制，需新增 V 为 NaN、负数、越界的用例；
  - `service/wan_video_test.go:106-143`：需新增 V>0 时端到端扣费精确的用例；
  - 仓库里目前没有任何测试引用 `wan_input_video_seconds` 或 `InputVideoDuration`。
- **上游参照**：上游插件提交时不知道 V，所以有参考视频就按 30 秒预扣（`plugin.js:1087-1092`），完成后按 `usage.input_video_duration + output_video_duration` 结算（`:1121-1131`）。FlowAPI 提交时已测得 V，可以精确预扣。

---

## 5. 对照 Rolldek WAN 3.0 接口文档逐条核对

标记：✓ 一致；✗ 不一致；— 不适用或无影响。

| # | Rolldek 文档 | FlowAPI 实现 | 结论 |
|---|---|---|---|
| 1 | `POST https://rolldek.com/v1/videos` | `{base}/v1/videos`（`adaptor.go:145-150`），base 见 1.4 | ✓ |
| 2 | `GET /v1/videos/{task_id}` | `{base}/v1/videos/{PathEscape(id)}`（`adaptor.go:285`） | ✓ |
| 3 | `GET /v1/videos/{task_id}/content` | 视频代理对类型 1 渠道转发到该端点（`video_proxy.go:114-116`）；但 `/api/v1/tasks` 返回的是签名直链（`relay_task.go:465`） | ✓（两条取片路径并存） |
| 4 | 鉴权 `Authorization: Bearer` | 提交和查询都是 Bearer（`adaptor.go:154,292`） | ✓ |
| 5 | 扁平的 OpenAI 风格请求体 | 扁平 map（`wan.go:72`）；客户端侧是 DashScope 格式，由 FlowAPI 转换 | ✓ |
| 6 | 模型名必须带分辨率后缀，后缀优先于 size 和 resolution | 总是带后缀（`wan.go:71`）；`size` 与后缀同档，不会冲突 | ✓ |
| 7 | 四个模型系列，其中 `wan3.0-image-*` 用于图生 | 只发 video 和 video-prime 系列，从不发 image 系列（`wan.go:71`；`dto/wan.go:14`） | —（两系列单价相同，见 1.3） |
| 8 | `prompt` 必填 | 有媒体时 prompt 可为空（`dto/wan.go:43-45`），会发出 `"prompt":""`（`wan.go:72`） | ✗（Rolldek 如何处理空 prompt：未确认） |
| 9 | `seconds` 为字符串，也接受 `duration` 别名 | `strconv.Itoa` 字符串，范围 2–30（`wan.go:72`；`dto/wan.go:62-68`） | ✓（Rolldek 的秒数范围：未确认） |
| 10 | `size` / `resolution` 会被后缀覆盖 | 发送 `size:"720P"` 这类值 | ✓（无影响） |
| 11 | `aspect_ratio` / `ratio` 只有 16:9、9:16、1:1、adaptive | 字段名为 `aspect_ratio`，但还放行 `4:3`、`3:4`（`dto/wan.go:59`） | ✗（Rolldek 对这两个值的处理：未确认） |
| 12 | `reference_images`：对象数组，最多 10 张，带 role | 不发这个字段；图片走 `input.media`，参考图最多 10 张，首帧和尾帧各 1 张（`dto/wan.go:72`） | ✓（数量在范围内） |
| 13 | `reference_videos`：服务端自动读取时长并计费 | 不发这个字段；参考视频走 `input.media`，最多 5 段（`dto/wan.go:72`） | ✓（Rolldek 是否同样读取 `input.media` 里的视频时长并计费：文档说按「所有参考视频」计费，未经实测，按计费处理） |
| 14 | `reference_audios`：不计秒数 | 走 `input.media`，最多 5 段，仅 MP3/WAV（`wan_video.go:80-83`），不计费 | ✓ |
| 15 | `input.media` 高级媒体数组，可与 reference_* 同时使用 | 只用 `input.media`，每项为 `{type,url}`，type 沿用 DashScope 取值（`wan.go:73-75`；`dto/wan.go:16-19`） | ✓（Rolldek 的 `input.media` 是否接受同一套 type 取值：未确认） |
| 16 | 素材 URL 必须是公网 HTTPS | 放行 `http`、`https`，图片还放行 `data:image/`（`dto/wan.go:83-90`）。图片地址既不探测，也不做 SSRF 校验；视频和音频虽做 SSRF 校验，但同样放行 http（见 6.2） | ✗ |
| 17 | image 系列不允许参考视频 | 不发 image 系列 | — |
| 18 | video 系列按「成片 + 全部参考视频」计费，相同 URL 只计一次 | 只按成片计费（`wan_video.go:158`）；参考视频不去重（`wan.go:40-58`） | ✗（计费差异见第 3 节；去重见 6.1） |
| 19 | image 系列只按成片计费 | — | — |
| 20 | 创建任务时按后缀对应分辨率的单价预扣 | 按同一档分辨率倍率预扣（`adaptor.go:110`），但不含参考视频 | ✗（与 #18 同源） |
| 21 | 创建响应 `{id, task_id, object, status:"queued", progress:0}` | 先读 `id`，缺失时回退到 `task_id`；`queued` 映射为 Queued（`wan.go:109,113-118,147-158`）；给客户端返回 PENDING（`:156`） | ✓ |
| 22 | 查询状态为 queued、in_progress、completed、failed | 全部覆盖，且映射集合更大（`wan.go:116-127`） | ✓ |
| 23 | 进度 `progress` | 不读取（`wan.go:80-99` 没有该字段），使用 FlowAPI 自己的阶段进度（`task_polling.go:584-596`） | —（无计费影响） |
| 24 | 完成后 `metadata.url` 为签名直链，有有效期，不做本地转存 | 优先读 `metadata.url`（`wan.go:132-137`），存为 ResultURL（`task_polling.go:601-603`），同样不转存 | ✓（直链过期后，`/api/v1/tasks` 返回的是失效链接；`/content` 代理过期后是否仍可用：未确认） |
| 25 | 文档未说明完成响应是否带时长 | 首选 `usage.output_video_duration`（`wan.go:96-98`），缺失时下载成片探测（`task_polling.go:539-542`） | ✗ 或未确认：如果 Rolldek 不返回时长，每个成功任务都要下载一次成片；R6 之后改为按请求时长结算。OpenAI 风格响应里的 `seconds` 字段 FlowAPI 不读，Rolldek 是否返回也未确认 |
| 26 | 建议 15 秒轮询一次 | 15 秒（`controller/system_task_handlers.go:146`） | ✓ |
| 27 | 建议 10–15 分钟超时 | 默认 1440 分钟（`common/init.go:205`） | ✗（属于差异，不是错误） |
| 28 | 文档未列 `watermark` | 固定发送 `watermark:false`（`wan.go:72`；测试 `wan_test.go:42`） | ✗（多发一个字段，是否被忽略：未确认） |

---

## 6. 追加问题

### 6.1 参考视频秒数是否按 URL 去重：**不去重**

- `wan.go:40-58` 逐项遍历 `input.media`。每遇到一个 `reference_video` 就探测一次（同一 URL 会被下载多次），并累加到 `videoSeconds`，其间没有任何已见 URL 的记录。`dto/wan.go:74-82` 同样按项计数。
- **后果**：同一 URL 出现两次时，FlowAPI 的 V 大于 Rolldek 的计费秒数。
  - 如果按 O+V 计费，用户会多付，但不会亏本；
  - 15 秒总量和 30 秒上限（`wan.go:59`）也按未去重的总和判断，所以会拒绝一些 Rolldek 本可以接受的请求。
- **修改点**：如需与 Rolldek 一致，去重只需改 `wan.go:40-58` 一处，因为预扣和结算都来自这一个 `videoSeconds`。
- Rolldek 判断「相同 URL」是按原始字符串还是规范化后的 URL：**未确认**。

### 6.2 `ProbeWanMediaDuration` 是否要求公网 HTTPS：**不要求 HTTPS，公网与否取决于抓取设置**

- **协议**：`wan_video.go:26` 放行 `https` 和 `http`，拒绝带用户信息的 URL。
- **公网校验**：`wan_video.go:29-32` 调用 `ValidateURLWithFetchSetting`：
  - 默认配置（`setting/system_setting/fetch_setting.go:17-24`）开启 SSRF 防护、禁止私网 IP、只放行 80/443/8080/8443 端口，并对域名解析结果做 IP 过滤，效果接近「只接受公网地址」；
  - 重定向逐跳校验（`service/http_client.go:43-52`）；
  - 管理员一旦关闭 SSRF 防护，就完全不做公网校验（`common/ssrf_protection.go:381-384`；`service/http_client.go:137-139`）。
  - HK 生产的抓取设置：**未确认**。
- **与 Rolldek 的差异**：
  - `http://` 的参考视频和参考音频能通过 FlowAPI 的探测并被转发，而 Rolldek 要求 HTTPS；
  - 图片（首帧、尾帧、参考图）完全不探测，`http://` 和 `data:image/` 都会原样转发（`dto/wan.go:85`）。
- **被 Rolldek 拒绝时的计费**：
  - 同步返回 4xx 时，FlowAPI 退回预扣（`relay_task.go:279-283`；`controller/relay.go:682-683`）；
  - 受理后再失败的，由轮询按失败处理并退款（`task_polling.go:613-625,657-659`）；
  - Rolldek 具体走哪种方式：**未确认**。
- **反向约束**：凡是 FlowAPI 接受的参考视频，一定已经测出时长；测不出的在提交时就返回 400（`wan.go:45-48`）。因此改成 O+V 计费时，不存在 V 未知的情况。

---

## Files Found

| 文件 | 作用 |
|---|---|
| `relay/channel/task/sora/wan.go` | Rolldek Wan 的校验、参考素材探测、请求体构造、结果解析 |
| `relay/channel/task/sora/adaptor.go` | 协议开关、EstimateBilling、URL、请求头、查询 |
| `dto/wan.go` | WanRequest 的校验、分辨率倍率、客户端响应结构 |
| `service/wan_video.go` | 媒体时长探测、Wan 结算额计算 |
| `service/task_polling.go` | 完成时长核验、结算入口 |
| `service/task_billing.go` | 差额结算、退款、计费日志 |
| `relay/relay_task.go` | 提交主流程：映射、定价、预扣、派发快照、DashScope 风格查询响应 |
| `controller/relay.go` | 提交成功后结算、写入任务快照、不重试规则 |
| `controller/video_proxy.go` | `/v1/videos/{id}/content` 代理 |
| `relay/helper/price.go`、`types/price_data.go` | ModelPrice、有效分组倍率、倍率连乘 |
| `setting/system_setting/fetch_setting.go`、`common/ssrf_protection.go`、`service/http_client.go` | SSRF 与抓取设置 |
| `.trellis/tasks/09-22-flowapi-pricing-verification/target-pricing-proposal.json`、`flowapi-disabled-channels.json` | Wan 基准价、币种、渠道清单 |
| `.trellis/tasks/09-22-flowapi-provider-coverage/release-gate-2026-09-25.md` | 渠道 83 与冒烟记录 |

## Related Specs

- `.trellis/spec/backend/billing-ratio-routing.md:44-55`：有效倍率公式；异步任务只读提交时的快照。
- `.trellis/spec/backend/media-channel-deployment.md:25,37`：Wan 只允许配置在 Rolldek 的类型 1 渠道上。
- AGENTS.md 的计费安全不变量：计费乘数必须有界；媒体元数据时长必须做饱和处理；新计费路径使用 `*Checked` 并写审计。

## Caveats / 未确认

- 本文依据的 Rolldek 文档是主线转述的要点，没有看到原文。
- 以下项目未确认：
  - Rolldek 对 `input.media` 各 type 的识别；
  - 空 prompt、4:3 和 3:4、`watermark` 字段的处理方式；
  - 秒数范围与各类素材的数量上限。
- Rolldek 如何测量参考视频时长也未确认：用容器时长还是视频轨时长，是否按整秒向上取整。FlowAPI 用视频轨时长的浮点值。如果 Rolldek 按整秒向上取整，标准版 480P、G=1 的 10% 毛利可能被吃掉。
- Rolldek 的成片按请求秒数还是实际秒数结算，以及完成响应是否带 `usage` 或 `seconds`：未确认。
- 渠道 83 的实际 base_url、model_mapping 和渠道倍率，`vedio` 分组的倍率，HK 的抓取设置与 `TASK_TIMEOUT_MINUTES`：未查看生产库，均未确认。
- 第 3 节只计 Rolldek 的秒价，没有计入通道费、退款失败等其他成本。
