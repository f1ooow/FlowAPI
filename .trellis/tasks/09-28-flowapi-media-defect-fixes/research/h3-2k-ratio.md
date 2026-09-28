# Research: MiniMax-H3 的 2K 倍率（29/19 与 1.25）及 H3 计费口径

- **Query**：
  1. 29/19 和 1.25 各自的来源、先后顺序和改动理由；仓库内 H3 / metaso 2K 定价依据。
  2. 追加问题：
     - FlowAPI 的 H3 预扣与结算怎么算，默认时长是多少；
     - 输入媒体是否计费（对照上游 `c9d24ae47`）；
     - 有哪些分辨率和档位；
     - 是否支持再生成和 Context IR；
     - 参考视频时长从哪里拿；
     - 请求限额；
     - 默认 ModelPrice 在哪里；
     - 按新规则实现时，预扣和结算怎么算。
- **Scope**：mixed
  - FlowAPI、keli 两个仓库的 git 历史（只读）；
  - FlowAPI 的 `upstream/main`；
  - keli-canvas 文档（`.trellis/` 已被 gitignore，没有 git 历史）；
  - 运维台账；
  - Codex 会话记录；
  - 用户在 2026-08-07 粘贴的 MiniMax 官方文档本地副本。
- **Date**：2026-09-28
- 本文只陈述证据，不替用户做决定。凡涉及生产，均未 SSH 核实。

## 结论

- **29/19 的来源**：2026-08-07，用户按当时的 MetaSo 成本价要求"768P 0.19、2K 0.29 元/秒"，keli 据此实现为 ModelPrice 0.19、2K 倍率 29/19。
- **1.25 的来源**：2026-09-05，用户要求改为"768P 0.20、高分辨率 0.25"，keli 代码的倍率随之改成 1.25。但生产数据库的 ModelPrice 一直没改，仍是 0.19。
- **FlowAPI 的现状**：从 keli 代码继承了 1.25，从 keli 生产数据库继承了 0.19。所以 FlowAPI 当前的 768P 0.19、2K 0.2375 元/秒，两次决策的本意都不是这个组合。
- **FlowAPI H3 计费的实际口径**：
  - 按请求时长预扣，客户端不传时默认 5 秒，结算时不做调整；
  - 不对输入媒体收费；
  - 不读取上游返回的 usage。

## 1. 29/19 与 1.25：来源与先后

| 时间 | 事件 | 证据 |
|---|---|---|
| 2026-08-07 23:08 CST | 用户："价格768P的价格为0.19元/秒；2K的价格为0.29元/秒"。23:47 又说："价格你就先填成本价，就是我刚刚报给你那个价格。" | Codex 会话 `~/.codex/sessions/2026/08/07/rollout-2026-08-07T21-55-14-019fdc81-….jsonl`，用户消息时间戳 15:08:06Z、15:47:39Z |
| 2026-08-07 | keli-canvas 合同文档 §3.5 记录"NewAPI model cost baseline：768P 0.19 / 2K 0.29；实现方式 ModelPrice=0.19，2K resolution multiplier = 29/19"。文档注明"Last verified: 2026-08-07" | `keli-canvas/.trellis/spec/important/project-docs/minimax-h3-api-reference.md:5, 206-225, 364-367`。其中 `:339-343` 专门警告"ModelPrice=0.29 且 2K 倍率 29/19 会重复收费" |
| 2026-08-07 | 生产部署：镜像 `minimax-h3-20260807-235115`，渠道 62 为 MetaSo；台账记录"MiniMax-H3=0.19 元/秒、2K resolution multiplier=29/19（即 0.29 元/秒基准）"，冒烟测试 768P/4s 扣费 0.912 元 | `~/.claude/skills/server/references/servers.md:314` |
| 2026-08-07 至 09-05 | keli 相关代码一直没有提交（keli reflog 显示 `c9c361150` 08-04 与 `a18f30e01` 09-10 之间没有任何提交）。工作树中的写法为：`hailuo/adaptor.go:520 resolutionRatio = 29.0 / 19.0`、`adaptor_test.go:345 wantRatio: 29.0 / 19.0`、前端 `pricingTypeRegistry.js:35 '2K': 29 / 19` | Codex 会话 2026-08-10、08-28、09-05 中的工具输出（例如 `~/.codex/sessions/2026/09/05/rollout-2026-09-05T12-59-51-01a06fef-….jsonl`） |
| 2026-09-05 13:48 CST | 用户："minimaxh3 768p的价格为768p 0.2元 1080p 0.25元 请按照这个逻辑去调整"。助手回复（13:58）："把两条 adaptor 的 H3 高分辨率倍率统一改成 0.25 / 0.20 = 1.25……生产数据库的 ModelPrice 再改为 0.20" | 同一 09-05 会话，时间戳 05:48:24Z、05:58:02Z |
| 2026-09-05 13:59 CST | 补丁：`resolutionRatio = 29.0 / 19.0` 改为 `H3HighResolutionRatio`；新增 `H3HighResolutionRatio = 1.25`，注释"¥0.25/s versus the ¥0.20/s 768P base"；基价注释 0.19 改为 0.20 | 同一会话，时间戳 05:59:54Z（patch 原文） |
| 2026-09-05 约 17:25 CST | keli 以当时的工作树构建镜像 `new-api:fastai-time-20260905-1725` 并部署。按时间推断包含 1.25；未 SSH 核实 | 同一会话 06:06:20Z（"当前工作树里原有的 Keli API 改动会一起进入镜像"）、09:37Z |
| 2026-09-06 17:41 CST | 助手："广州也还有一个遗留值：H3 数据库仍是 0.19，并未完成之前约定的 0.20/0.25 调整" | 同一会话文件，时间戳 2026-09-06T09:41:26Z |
| 2026-09-06 | Canvas 自有网关的定价改为 768P 0.20、2K 0.25 元/输出秒，并注明"旧 API 文档中的历史 NewAPI ModelPrice……不覆盖本文" | `keli-canvas/.trellis/spec/important/project-docs/video-model-pricing.md:3-24, 151`；`minimax-h3-api-reference.md:3` |
| 2026-09-10 | keli 提交 `eac2a27d7`：`H3HighResolutionRatio = 1.25`（`hailuo/constants.go:59-60`），默认 `"MiniMax-H3": 0.2`，注释为"768P per-second base; high resolution is 0.25/s (ratio 1.25)"（keli `setting/ratio_setting/model_ratio.go:312`），前端 `{'768P':1,'1080P':1.25,'2K':1.25}`。提交说明："计价 0.20 元/秒（768P 基准），1080P/2K 倍率 1.25" | `git show eac2a27d7`。`git log --all -S H3HighResolutionRatio` 只有 `eac2a27d7` 和 `02b17a6e8`（FastAI 复用），之后没有再改 |
| 2026-09-22 | FlowAPI 覆盖清点时，keli 线上数据库 H3 为 `ModelPrice: 0.19`，没有 billing_mode 或表达式，也就是数据库里不存 2K 倍率 | FlowAPI `.trellis/tasks/09-22-flowapi-provider-coverage/research/coverage-matrix.json:371-383` |
| 2026-09-25 | FlowAPI `8156149df` 原样复制 `constants.go`（1.25）；`1f5bb7309` 清单写入 `"MiniMax-H3": 0.19`（来源为 keli 零售价） | `relay/channel/task/hailuo/constants.go:59-60`；`.trellis/tasks/09-22-flowapi-pricing-verification/target-pricing-proposal.json:26, 98-114` |

补充说明：

- 字符串 `29/19`、`29.0 / 19`、`29.0/19`、`1.526` 在两个仓库的全部 git 历史中都不存在，因为 29/19 的代码从未被提交。
- 台账（servers.md:314）在 09-05 之后没有更新 H3 的条目。
- **现价推算：**keli 线上与 FlowAPI HK 都是 ModelPrice 0.19 × 2K 倍率 1.25：
  - 分组前：768P 0.19、2K 0.2375 元/秒；
  - 乘以视频分组倍率 1.2 后：768P 0.228、2K 0.285 元/秒。

## 2. 仓库内的 H3 / metaso 2K 定价依据

- **FlowAPI 代码注释**：
  - `hailuo/constants.go:59`："¥0.25/s versus the ¥0.20/s 768P base"；
  - `hailuo/adaptor.go:543-545`："0.20 元/秒基价"。
  - 这两处都是 keli 2026-09-05 那次决策的原文，与 FlowAPI 实际的 0.19 不一致。
- **FlowAPI 测试**：`hailuo/adaptor_test.go:366` 用 0.2 / 0.25 断言倍率关系。
- **FlowAPI 清单**：`target-pricing-proposal.json:98-114` 为 `source_retail.model_price 0.19`、`supplier_cost_verified: false`、`target_price: null`。没有任何 2K 倍率字段。
- **FlowAPI 的 `.trellis/spec/`**：没有 H3 定价内容。
- **成本依据**：
  - 仓库内唯一的成本口径是 2026-08-07 用户所说的"成本价 0.19 / 0.29"，见 1 节；
  - 用户 2026-09-28 提供的 metaso 价目见 4.3 节。
  - 两者差异的原因：未确认。

## 3. FlowAPI 当前的 H3 计费口径（追加问题 1）

- **预扣**：
  - 公式为 `ModelPrice × QuotaPerUnit × 分组倍率 × 时长 × 分辨率倍率`（`relay/helper/price.go:240-242`、`relay/relay_task.go:218-237`）；
  - 倍率来自 `EstimateBilling` 的 `hailuo-h3-duration` 与 `hailuo-h3-resolution`，768P 为 1，2K 为 1.25（`hailuo/adaptor.go:551-565`）。
- **结算：不做调整。**
  - H3 使用 ModelPrice，所以 `PerCallBilling = UsePrice = true`（`model/task.go:152`），轮询结算在 `service/task_polling.go:716-720` 直接跳过；
  - hailuo 没有实现 `TaskBillingCheckedAdaptor`，`BaseBilling.AdjustBillingOnComplete` 返回 0（`taskcommon/helpers.go:95-97`）；
  - H3 查询结构体不含 `usage` 字段（`hailuo/models.go:91-100`），解析时也不读 usage（`adaptor.go:383-424`）。
  - 所以最终扣费 = 按请求时长算出的预扣额。
- **客户端没传时长时**：取 `H3DefaultDuration = 5`（`hailuo/constants.go:58`，`hailuo/models.go:204-214`），同时接受字符串形式的 `seconds`。
- **上游默认时长：没有。**
  - MiniMax 官方 V2 创建接口把 `duration` 列为必选，取值为 4–15 的整数（官方文档本地副本 `~/.codex/attachments/6a547350-…/pasted-text.txt:159-163, 270-285`）；
  - FlowAPI 总是显式下发 `duration`（`hailuo/models.go:61`，字段没有 omitempty）；
  - 上游 plugin 和 keli-canvas 的本地默认值也是 5（`upstream/main:plugins/tasks/hailuo/plugin.js:192, 207-209`；keli-canvas `server/lib/drivers/video/minimax-h3.ts:215`）。

## 4. 输入媒体计费（追加问题 2）

### 4.1 FlowAPI

不收费。预扣只包含输出时长和分辨率两个倍率（`hailuo/adaptor.go:551-565`），结算也不读 usage（见 3 节）。keli 与 FlowAPI 相同。

### 4.2 上游 new-api `c9d24ae47`（2026-09-03，合入 main 为 `73afad588`，#7171）

**为什么收费：**

- 上游 plugin 头注释写明"only MiniMax-H3 charges for input media"（`upstream/main:plugins/tasks/hailuo/plugin.js:1-2`，由 `a8ed7f7c5` 在 2026-09-17 加入）。也就是说，供应商对 H3 的输入媒体收费，所以网关必须采集输入量。

**采集什么：**新增两个 usage fact。

- `input_images`：按张计；
- `input_video_seconds`：按秒计（`plugin.js:18-29`）。

**提交时怎么估：**

- 数 `image_url` 项得到图片数；
- 只要存在 `video_url`，参考视频秒数就按"文档记载的总时长上限" `H3_MAX_INPUT_VIDEO_SECONDS = 15` 预留（`plugin.js:197, 505-519`）。

**完成时怎么算：**

- 用 `task.usage` 的实际值替换估算：`output_seconds` 取值 4–15，`input_image_count` 为 0–9 的整数，`input_seconds` 取值 0–15；
- 越界或缺失的值会被丢弃，保留提交时的估算（`plugin.js:610-630`）。
- 对应测试见 `plugins/hailuo_responses_test.go`，出现在 `git show c9d24ae47`。

**单价：代码里没有。**

- fact 的描述只有"Input image unit price / Input video unit price"；
- 定价由管理员配置的阶梯表达式决定（`aece11d2f` 提交说明："2K/duration pricing is expressed through the model's tiered billing expression over those facts"）。
- FlowAPI 的任务计费不支持表达式（`model/media_billing_unit.go:28-31`，并且 `relay_task.go:189` 走的是 `ModelPriceHelperPerCall`），所以无法直接照搬上游的做法。

### 4.3 metaso H3 价目（用户 2026-09-28 提供，照录）

**生成：**

| 项 | 价格 |
|---|---|
| 输出 2K | 0.15 元/秒 |
| 输出 768P | 0.09 元/秒 |
| 输入音频 | 免费 |
| 输入图片 | 前 5 张免费，超出每张 0.05 元 |
| 参考视频 | 按输出分辨率的秒价收费（2K 0.15，768P 0.09） |

**视频再生成（768P→2K）：**

| 项 | 价格 |
|---|---|
| 再生成 | 0.06 元/秒 |
| 输入音频 | 免费 |
| 输入图片 | 沿用原 768P 任务的前 5 张免费，超出每张 0.04 元 |
| 视频素材 | 0.06 元/秒 |

**H3 Context IR：**0.05 元/次。

### 4.4 FlowAPI 现行口径少收的部分，以及未覆盖的供应商成本

参考视频总时长上限按官方的 15 秒计（见 8 节）。FlowAPI 本身不校验这个上限。

| | 768P | 2K |
|---|---|---|
| 按现价，参考视频每秒应收未收（分组 1.2） | 0.228 元 | 0.285 元 |
| 单任务最多少收（15 秒） | 3.42 元 | 4.275 元 |
| 参考视频的供应商成本（15 秒） | 1.35 元 | 2.25 元 |
| 超出 5 张的图片成本（最多 9 张） | ≤ 0.20 元 | ≤ 0.20 元 |

最坏情况举例（现价，输出 4 秒，参考视频 15 秒，9 张图片）：

| 分辨率 / 分组倍率 | 收入 | 成本 | 亏损 |
|---|---|---|---|
| 2K / 1 | 0.95 元 | 3.05 元 | 2.10 元 |
| 2K / 1.2 | 1.14 元 | 3.05 元 | 1.91 元 |
| 768P / 1 | 0.76 元 | 1.91 元 | 1.15 元 |
| 768P / 1.2 | 0.912 元 | 1.91 元 | 1.00 元 |

## 5. 分辨率与档位（追加问题 3）

- **FlowAPI**：只接受 768P 和 2K；不传时默认 768P；其他值返回 400（`hailuo/models.go:172-181`）。倍率 768P 为 1.0，2K 为 1.25（`adaptor.go:558-561`）。时长 4–15 秒逐秒计费，没有分档表（`HailuoTierRatios` 不包含 H3，见 `constants.go:74-92`）。
- **MiniMax 官方文档副本**：`resolution` 的可用值只有 `768P` 和 `2K`（`pasted-text.txt:264-269`）。
- **上游 plugin**：枚举为 `["768P","2K"]`（`plugin.js:11-16`）。
- **MetaSo 实测报错**：2026-09-05 用 1080P 提交，MetaSo 返回 400，文本为"resolution 仅支持 480p、512p、768P 或 2K (2013)"（09-05 Codex 会话，时间戳 06:01:37Z）。报错文本里提到 480p 和 512p，但官方文档和 metaso 价目都没有这两档。FlowAPI 不放行这两档。它们是否可用、价格多少：未确认。
- **FastAI**：曾以 1080p 作为 H3 的高分辨率标签（实测输出 1920×1088），与 2K 不是同一档。FastAI 的 H3 渠道已于 2026-09-05 从 keli 生产删除（同一会话，05:57Z、06:03Z）。FlowAPI 的 fastaitoken 没有 H3。

## 6. 视频再生成（768P→2K）与 Context IR：FlowAPI 不支持

- **官方接口**：`POST /v2/video_regeneration`（`~/.codex/attachments/6f62bf02-…/pasted-text.txt:38-54`），`POST /v2/h3_context_ir`（`~/.codex/attachments/31b67aa4-…/pasted-text.txt:39-44`）。
- **FlowAPI H3** 只有两个端点：`/v2/video_generation` 与 `/v2/query/video_generation/{task_id}`（`hailuo/constants.go:21-24`，`adaptor.go:58-66, 239-251`）。
- **MiniMax 扩展路由**只有 `/files/upload`、`/voice_clone`、`/music_generation`（`router/relay-router.go:151-153`）。
- `/v1/videos/:id/remix` 是通用的 remix 路由（`router/video-router.go:31`），hailuo 适配器没有针对 remix 的分支。
- 上游 plugin 同样只有 generation 与 query 两个接口（`plugin.js:457, 528`）。keli-canvas 合同明确"Explicitly deferred: H3 Context IR and video regeneration"（`minimax-h3-api-reference.md:29`）。
- 结论：这两项的价格与 FlowAPI 当前的计费无关。

## 7. 参考视频时长从哪里拿

### 7.1 提交时

- **请求本身**：没有时长字段。`H3Metadata.ReferenceVideos` 是 `[]string`，只装 URL 或 `mm_file://`（`hailuo/models.go:33-42`）。`TaskSubmitReq` 的 `Duration` 和 `Seconds` 指的是输出时长。
- **现成的探测工具**：`service.ProbeWanMediaDuration(ctx, url, true)`（`service/wan_video.go:23-89`），逻辑是下载后读 MP4 视频轨时长。
  - Wan 已经在提交时用它探测参考视频，并要求单段 1–15 秒、总计 ≤15 秒、输入加输出 ≤30 秒（`relay/channel/task/sora/wan.go:40-58`）；
  - 但它只接受 HTTP(S)（`wan_video.go:25-27`），**不能用于 `mm_file://`**；
  - 另有下载上限 `MAX_FILE_DOWNLOAD_MB`（默认 64）、45 秒超时和 SSRF 限制。
- **`mm_file://` 上传：不记录时长。**
  - 上传处理 `RelayMiniMaxFilesUpload`（`relay/channel/minimax/files.go:63-129`）手里有文件字节（`:88-104`），但没有解析媒体；
  - 绑定表 `MiniMaxResourceBinding` 没有时长列（`model/minimax_resource.go:13-21`）；
  - MiniMax 上传响应和 `files/retrieve` 返回的也只有 file_id、bytes、filename、purpose、download_url（`files.go:19-27`，`hailuo/models.go:355-367`）。
  - 另外，FlowAPI 的上传上限是 20 MB（`minimax/extended.go:17`），小于官方的视频单文件上限 50 MB。

### 7.2 完成时

- MiniMax 官方查询成功示例中，`task.usage` 为 `{"total_seconds":5,"input_seconds":0,"output_seconds":5,"input_image_count":0}`，同时还有 `resolution`、`duration`、`task_type: "generation"`（`pasted-text.txt:108-137`）。
- keli-canvas 把这组字段描述为"observed"（`platform/adapter/minimax.go:360-366`，fixture 在 `direct_adapters_test.go:204-224`）。
- FlowAPI 的 H3 结构体没有 `usage`（`hailuo/models.go:91-100`），不读这组字段。
- 不过，查询响应在结算前已经存进了 `task.Data`（`service/task_polling.go:551`，结算在 `:654-656`）。所以结算时可以从 `task.Data` 读出这组字段。
- MetaSo 是否原样透传 `usage`：未确认。可以用 HK 生产 `tasks.data` 只读核实，本次没做。

### 7.3 上游 `c9d24ae47` 的做法

提交时不探测，有参考视频就预留 15 秒；完成时用 `usage.input_seconds` 替换；越界或缺失时保留预留值（见 4.2 节）。

## 8. 请求限额（以 FlowAPI 代码校验为准）

| 项 | FlowAPI 代码 | MiniMax 官方文档副本 | 上游 plugin |
|---|---|---|---|
| 输出时长 | 整数 4–15，默认 5（`models.go:204-217`） | 必选，整数 4–15（`pasted-text.txt:270-285`） | 4–15，默认 5（`plugin.js:190-192`） |
| 参考图 | ≤9（`models.go:227`）；首帧、尾帧各 ≤1，且帧与参考不能混用（`:237-243`） | 首帧 ≤1、尾帧 ≤1、参考图 ≤9，单张 ≤30 MB（`:221`） | 参考图 ≤9，全部图片合计 ≤9（`plugin.js:303-304`） |
| 参考视频数量 | ≤3（`models.go:230`） | ≤3（`:237`） | ≤3（`plugin.js:305`） |
| 参考视频单段 / 总时长 | **不校验（未确认）** | 单段 2–15 秒，总计 ≤15 秒，单个 ≤50 MB（`:235-239`） | 只把总计 15 秒作为预留和完成校验的上限（`plugin.js:197, 620`） |
| 参考音频 | ≤3（`models.go:233`），时长不校验 | ≤3，单段 2–15 秒，总计 ≤15 秒，单个 ≤15 MB（`:257-261`） | ≤3 |
| 提示词 | ≤7000 字符（`models.go:200-202`） | ≤7000（`:357-358`） | — |
| 请求体 | — | 总计 ≤64 MB（`:203`） | — |

## 9. 默认 ModelPrice 的位置，以及改价要动哪些地方

- **FlowAPI 代码里没有 H3 的默认价。**
  - `setting/ratio_setting/*.go` 中没有 MiniMax 或 Hailuo 的条目（已 grep）；
  - `ModelPriceHelperPerCall` 在未配置价格时会查 `GetDefaultModelPriceMap`，里面也没有 H3（`relay/helper/price.go:213-233`）。
  - keli 的默认值 0.2 只存在于 keli 仓库（keli `setting/ratio_setting/model_ratio.go:312`）。
- **实际生效的价格**只有 HK 生产的 `options.ModelPrice["MiniMax-H3"]`（清单导入值 0.19，另见 PRD:61）。
- **出现这个数字的其他位置**：

  | 位置 | 内容 | 是否影响计费 |
  |---|---|---|
  | `target-pricing-proposal.json:26` | `deployment_base_pricing.model_price` 0.19，是部署使用的基准价，实例 B 复用时会读这里 | 不直接影响，由导入写入生产 |
  | `target-pricing-proposal.json:104-111` | `rows[].source_retail` 0.19（keli 来源记录），`target_price: null` | 否 |
  | `coverage-matrix.json:371-383` | keli 线上快照，属于历史证据 | 否 |
  | `hailuo/constants.go:59`、`adaptor.go:544` | 0.20 / 0.25 的注释 | 否 |
  | `hailuo/adaptor_test.go:366` | 0.2 / 0.25 的算式断言 | 否 |

- `model/media_billing_units.json:2-5` 只记录单位 `seconds`，不含价格。
- **改价要动的地方**：
  - 改 768P 基准价：只改生产 option；清单与注释按需同步。
  - 改 2K 倍率：需要改代码 `hailuo/constants.go:60`，同时改测试 `adaptor_test.go:344-370`。

## 10. 按用户新规则实现时的预扣与结算

用户新规则：计费秒数 = 输出时长 + 参考视频时长，按输出分辨率的单价计算；图片和音频不收费；768P 基准价 0.15。

### 10.1 现有代码的约束

1. **OtherRatios 是连乘关系**（`relay_task.go:218-221`，`ApplyOtherRatiosToFloat`）。"输出 + 参考"是加法，必须合并成一个秒数倍率，例如 `hailuo-h3-duration = 输出秒 + 参考秒`，不能各设一个倍率。
2. **H3 当前被视为按次计费**，结算会被跳过（`model/task.go:152`，`task_polling.go:716-720`）。如果要按实际结算，可以实现 `TaskBillingCheckedAdaptor`（接口定义在 `task_polling.go:38-43`），它在 `:709-715` 调用，位于 PerCallBilling 跳过逻辑之前。现有先例是 `relay/channel/task/volcvod/billing.go:12-46`：使用不可变快照，入参无效时返回 `handled=false`，从而保留预扣。
3. **`TaskInfo` 没有输入秒字段**，只有 `OutputVideoDuration`（`relay/common/relay_info.go:960-971`）。实际 usage 可以从 `task.Data` 解析，或者给 `TaskInfo` 新增字段。
4. **`RecalculateTaskQuota` 的两个行为**（`service/task_billing.go:232-262`）：
   - `actual <= 0` 时忽略；
   - 补扣时 `DecreaseUserQuota` 不检查余额，余额可能被扣成负数（`model/user.go:1324-1347`）。
   - 所以预扣越接近上界，结算时需要补扣的就越少。

### 10.2 公式

- **预扣**：`0.15 × 分辨率倍率 × 分组倍率 × (输出秒 + 参考视频预留秒) × QuotaPerUnit`。没有参考视频时，预留秒为 0。
- **结算**：用 `usage.output_seconds + usage.input_seconds` 替换预扣中的秒数，或直接使用官方示例中等于二者之和的 `total_seconds`。
  - 取值应按官方上限约束：输出 4–15 秒，输入 0–15 秒；
  - 缺失或越界时，保留预扣（上游做法），或者改用探测结果。

### 10.3 提交时拿不到参考视频时长：预扣上界的可选取法

| 取法 | 行为 | 风险 |
|---|---|---|
| a. 有参考视频就固定预留 15 秒（官方"总时长 ≤ 15 s"；上游 plugin 同样取值） | 实现最简单，也能覆盖 `mm_file://` | 结算拿不到 usage 且保留预扣时，用户按 15 秒多付；需要依赖 MetaSo 确实执行这个上限（FlowAPI 自己不校验） |
| b. 对 HTTP(S) 参考视频在提交时探测（复用 `ProbeWanMediaDuration`，与 Wan 的 `sora/wan.go:40-58` 一致），`mm_file://` 仍按 a | 对 URL 预扣精确，并可在提交阶段拒绝超过 15 秒的视频 | 每次提交都要下载参考视频（最长 45 秒、最大 64 MB）；网络失败时需要决定是拒绝请求还是按 a 处理 |
| c. 在 `minimax-files-upload` 上传时探测视频轨时长并入库，提交时查表 | `mm_file://` 也能精确 | 需要给绑定表加列（三种数据库都要迁移），并在上传处理里解析 MP4；非 MP4（例如 MOV）需要另行处理 |

### 10.4 新规则下的毛利（基准 0.15；供应商成本见 4.3 节；图片成本最多 0.20 元）

每计费秒的毛利：

| 分辨率与倍率 | 分组倍率 1 | 分组倍率 1.2 |
|---|---|---|
| 768P（0.15 对成本 0.09） | 0.06 | 0.09 |
| 2K @1.25（0.1875 对成本 0.15） | 0.0375 | 0.075 |
| 2K @5/3（0.25 对成本 0.15） | 0.10 | 0.15 |

- 计费秒数至少 4 秒。
- 只有一种组合会亏：**2K @1.25、分组倍率 1、9 张图片、计费秒数 4 或 5**。
  - 4 秒：毛利 0.15，图片成本 0.20，亏 0.05；
  - 5 秒：毛利 0.1875，亏 0.0125。
- 8 张图片、4 秒时恰好打平。其余组合都不亏。
- 参考视频按输出分辨率的秒价计费，和 metaso 的收费方式一致，所以不会产生额外亏损。

## 11. 建议的回归测试位置（现有文件）

- `relay/channel/task/hailuo/adaptor_test.go`：
  - `:344-370` `TestH3EstimateBilling`：秒数与倍率；如果把参考视频预留秒合并进时长，改这里；
  - `:81-219` `TestBuildH3Request`：限额校验；
  - `:308-342` `TestParseH3TaskResult`：如需解析 usage，在这里加用例。
- `service/task_billing_test.go:1166-1253` 一带的 `TestSettle_PerCallBilling_*`、`TestSettle_NonPerCallBilling_AppliesAdaptorAdjustment`：结算分支合同；新增 checked 结算时参照这里。
- `service/task_polling_account_test.go:39`：已经使用 `UpstreamModelName: "MiniMax-H3"` 的轮询夹具。
- `relay/channel/task/hailuo/account_test.go:14`：`mm_file://` 归属校验。

## Caveats / 未确认

- 2026-08-07 的"成本价 0.19 / 0.29"与 2026-09-28 的 metaso 价目"0.09 / 0.15"为什么不同：未确认。可能是供应商调价，也可能口径不同。
- keli 线上当前生效的是 1.25：由部署时间推断，未 SSH 核实镜像。
- MetaSo 是否透传 `task.usage`、是否执行"参考视频总计 ≤ 15 秒"、480p / 512p 是否可用及价格：未确认。
- 官方文档副本是 2026-08-07 的粘贴件（位于 `~/.codex/attachments/`，不在任何仓库里），之后官方可能有更新。
- 本文没有联网查询任何官方页面，也没有发起任何 API 请求。
