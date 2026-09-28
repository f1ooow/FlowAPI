# 修复 FlowAPI 媒体渠道缺陷

## Goal

FlowAPI 在 `8156149df`（2026-09-25）移植了 keli-api 的全部媒体渠道，但上线实测只覆盖了 Seedream 出图、MiniMax TTS、火山 TTS 和 Rolldek Wan。本任务修复已发现的 FlowAPI 媒体缺陷，使 FlowAPI 直连供应商时的请求、状态和计费都正确，并且不亏本。

受益方有两个：
- HK 现网 FlowAPI（下称 A）；
- 用户计划另部署的独立运营实例 B。B 与 A 同一套代码：文本和图片经 New API 类型渠道接 A，音视频在 B 里直连供应商。

## Background

- 只改 FlowAPI（`/Users/Zhuyu/Documents/Code/FlowAPI`），不改 keli-api。
- HK 生产当前镜像基于 main `10d32a657`，媒体渠道为 69–83，媒体分组为 `image` / `vedio`。
- 本任务相关的生产渠道：

  | 渠道 | 类型 | 模型 | 路由 |
  |---|---|---|---|
  | 71 Minimax TTS（官方账号） | 35 | speech、music、voice-clone、files-upload、Hailuo | p0 |
  | 80 MetaSo MiniMax H3 | 35 | H3、files-upload | p10 |
  | 76 VolcSeedream | 45 | Seedream | 未开透传 |
  | 78 VolcEngine ASR | 45 | doubao-asr-2.0 | — |
  | 70 / 79 HappyHorse | 17 | HappyHorse | 79 带 Zivv 端点覆盖 |
  | 82 Zivv 即梦 | 60 | Seedream 5.0 Pro | — |
  | 83 Rolldek Wan | 1 | Wan | `video_protocol=wan-rolldek-v1` |

- 调研产物在 `research/` 下：
  - `r1-r2-confirmation.md`
  - `r4-voice-clone.md`
  - `r7-tts-usage-characters.md`
  - `r8-music-placeholder.md`
  - 另有 R3、R5、R6 和 H3 的调研进行中。

## Requirements

### R1 火山 ASR 请求头对齐唯一实测跑通的配置

- **现状**：submit（`relay/channel/volcengine/asr.go:408`）和 query（`:486`）都设置 `X-Api-Sequence: -1`，而同文件头注释（`:18-20`）写明不应带这个头。这是 FlowAPI 与 keli 在 ASR 请求头上的唯一差异，由 `8156149df` 引入。
- **证据冲突**：
  - keli `364b4a068` 删除这个头后端到端实测跑通，commit 把 `45000000 app key not found` 归因于它。但该 commit 同时改了 4 处，不能单独归因。
  - 火山官方文档的 submit 示例带这个头，query 不带。
  - FlowAPI 这套配置从未实测。
- **修法**：两处都去掉，与 keli 实测跑通的配置一致。回归测试需断言 submit 和 query 的请求头里都不存在 `X-Api-Sequence`（判断 header map 中没有这个 key，不能只看 `Get()` 是否为空），并断言 Access-Key、App-Key、Resource-Id、Request-Id 不变。
- **测试接缝**：`asrHTTPClient`（`:78`）、`asrSubmit`（`:377`）、`asrQueryOnce`（`:476`）。不要走 `asrPoll`，否则会真实等待 2 秒。

### R2 Seedream 请求保留客户端的 `tools` 字段（已确认缺陷）

- **现状**：
  - `relaykit/dto/openai_image.go:17-50` 的 `ImageRequest` 没有 `tools` 字段。`MarshalJSON` 不合并 `Extra`（`:95-101`），这是上游为修 #1961 有意为之：非 OpenAI 字段会让严格校验的上游报错。
  - 未开透传时，`relay/image_handler.go:84` 用 DTO 序列化后发出，`tools` 就此丢失，ParamOverride 也救不回来。
- **影响**：
  - 客户端传入的 `tools: [{"type":"web_search"}]` 会被静默去掉，例如 Canvas 对 `doubao-seedream-5-0-260128` 开放的联网搜索（`keli-canvas/server/lib/imageProvider.ts:1194-1200`）。
  - B 经 New API 渠道接 A 的图片链路（`relay/channel/newapi/adaptor.go:88-89`）同样受影响。
  - 按火山当前官方文档，DTO 缺的请求字段只有 `tools`。
- **修法**：
  - DTO 增加 `Tools json.RawMessage \`json:"tools,omitempty"\``。
  - 上游模型不是 Seedream 时，发出前清除 `tools`，避免 OpenAI 类上游因为未知参数返回 400。
  - Seedream 5.0 Pro 官方不支持联网搜索，在 `normalizeSeedreamImagePayload` 里对 Pro 删除 `tools`，与现有的 `sequential_*` 处理一致。
- **约束**：`relaykit/` 是独立模块，改动后必须通过 `cd relaykit && GOWORK=off go build ./...`，relaykit 内用 `kitutil` 做 JSON。
- **回归测试**：
  - 在 `relay/channel/volcengine/seedream_test.go:14-41` 的样例里加上 `tools`，用 JSONEq 断言保留；
  - 断言非 Seedream 模型不转发 `tools`；
  - 断言 Pro 删除 `tools`。

### R3 HappyHorse UNKNOWN 宽限（已确认缺陷，移植不完整，见 `research/r3-happyhorse-unknown-grace.md`）

- **现状**：
  - ali 适配器把上游 `UNKNOWN` 映射为非终态（`relay/channel/task/ali/adaptor.go:679-689`），注释说由轮询按任务年龄判定宽限（`:680-683`），但注释里引用的 `TaskUnknownStatusGraceSeconds` 在 FlowAPI 里并不存在。
  - 轮询的 UNKNOWN 分支（`service/task_polling.go:609-612`）原样返回，没有任何退出条件。任务要挂满 24 小时，由超时清扫（`:74-123`）判失败并全额退款。
  - 期间预扣一直占着，用户一直看到「排队中」，每个任务约被查询 5760 次。不会亏钱，但收敛太慢。
- **修法**：只对 ali 适配器补上 keli 的宽限，提交后 120 秒内保持，超过就判失败并退款（CAS 保证只退一次）。
  - 不能照搬 keli 的共用分支。FlowAPI 里 H3、FastAI、Wan 把未识别状态也映射成 UNKNOWN，而 keli 里 H3 和 FastAI 映射的是「进行中」，共用分支会让它们 120 秒后被误判失败。
  - 实现可以让适配器通过可选接口声明宽限，或按平台判断。
- **回归测试**（`service/task_polling_test.go`，夹具复用 `service/task_billing_test.go`）：
  - 宽限内保持；
  - 超窗判失败，且只退款一次；
  - H3、FastAI、Wan 的 UNKNOWN 行为不变。

### R4 MiniMax 上传与声音克隆（已确认 5 处缺陷，见 `research/r4-voice-clone.md`）

- **R4a 上传按用途选渠道**
  - 现状：上传总是落到优先级最高的渠道 80（MetaSo），而克隆只挂在 71，于是克隆时资源校验以 403 拒绝（`middleware/minimax_resource_route.go:127-129`）。distributor 只看 model，不看 purpose。
  - 修法：在 distributor 里给 `/v1/files/upload` 按 purpose 选渠道：`voice_clone` / `prompt_audio` 只在挂了 `minimax-voice-clone` 的渠道里选；`video_generation_input` 只在挂了 `MiniMax-H3` 的渠道里选。上传仍然免费。
  - 回归测试：两个渠道优先级不同时，`voice_clone` 用途的上传落到挂克隆模型的渠道。
  - 另外两种做法都不行：只调优先级，H3 又会坏；拆分上传模型名，需要客户端配合。
- **R4b purpose 白名单改为官方枚举**
  - 现状：`relay/channel/minimax/files.go:82-86` 只收 `video_generation`（非官方值），拒收官方的 `video_generation_input`。Canvas 的 H3 驱动发的正是 `video_generation_input`，上传会 400，H3 的 `mm_file` 引用链路在上传这一步就断了，直接影响 B。
  - 修法：改为官方枚举 `voice_clone`、`prompt_audio`、`t2a_async_input`、`video_understanding`、`video_generation_input`，`video_generation` 保留作兼容。上传大小上限按官方各用途的上限设置（现为统一 20MB，`relay/channel/minimax/extended.go:17`）。
- **R4c 入站 `file_id` 兼容数字和字符串**
  - 现状：`voice_clone.go:20` 用 `json:"file_id,string"`，按 MiniMax 官方格式传数字的客户端会得到 400。
  - 修法：同时接受数字和字符串，拒绝非整数、≤0 和超出 int64 的值；发给上游仍用数字。
- **R4d 克隆字段对齐官方**
  - `need_noise_reduce` 改为 `need_noise_reduction`，入站兼容旧名；
  - `accuracy` 改为 [0,1] 小数；
  - 转发 `clone_prompt`、`text_validation`、`language_boost`，其中 `clone_prompt.prompt_audio` 要做资源归属校验；
  - 试听 `text` 上限改为 1000；
  - `voice_id` 末位不能是 `-` 或 `_`。
- **R4e 上游明确拒绝克隆时释放 voice_id**
  - 现状：`voice_clone.go:66` 写入的 pending 绑定一直不释放，该 voice_id 以后就用不了。
  - 修法：上游明确拒绝时释放；结果不确定时保留，防止重复付费。

### R5 Seedream 5.0 Pro 图层数与返回条数不一致（用户 2026-09-28 决定：交付成功的图，按成功张数计费）

- **决定**：
  - 成功项指 `data` 中带 `url` 或 `b64_json`、且不含 `error` 的项。
  - 计费张数 = min(成功项数, `generated_images`)，上限 17，再乘 0.5 的图层价格系数。
  - `generated_images` 缺失或非法时，按成功项数计费，上限不变。
  - 成功项数 ≥ 1 时，按原样写回完整响应体（含 error 项）。
  - 成功项数为 0 时，维持 502，不重试，并退还预扣。
  - 日志要记录成功项数、`generated_images` 和上游 request id。
  - 这一决定取代 `.trellis/tasks/09-22-flowapi-seedream-images/billing-contract.md:22-23` 里「不一致即 502」的合同，收尾时同步更新相关规范。
- **已知限制，不在本任务范围**：Canvas 只要结果里有 error 项就整单判失败（`keli-canvas/server/lib/seedreamLayers.ts:23`），所以 Canvas 用户可能出现已付费、界面却显示失败的情况，需要 Canvas 另行处理。
- **回归测试**：
  - 调整 `relay/channel/openai/seedream_image_test.go:51-69` 的断言；
  - 覆盖以下场景：含 error 项、成功项少于 `generated_images`、成功项为 0、`generated_images` 缺失，并断言每种场景的精确计费。

以下为决定前的现状和依据：

- **现状**：`generated_images` 与 `data` 条数（含 error 项）不一致时，在写回之前返回 502，不重试，并全额退还预扣（`relay/channel/openai/relay_image.go:91-111`）。
  - 这是 `.trellis/tasks/09-22-flowapi-seedream-images/billing-contract.md:22-23` 写明的合同，测试 `relay/channel/openai/seedream_image_test.go:51-69` 把它固化了下来。
- **后果**：上游已经出图扣费，用户拿不到任何图，这笔成本由我们承担。
  - 最可能的触发场景是部分图片失败、`data` 中出现 `{error:{...}}` 项。火山组图文档有这种格式，图层拆分是否同样如此，未确认。
- **keli 的做法**：不校验，直接写回，按 `generated_images × 0.5` 计费。

### R6 Wan 输出时长无法校验时一直挂起（用户 2026-09-28 决定：直接按请求时长结算）

- **决定**：
  - 上游返回的成片时长可信时，仍按实际时长结算。
  - 上游没有返回时长，或返回值越界时，不再下载成片测量，直接按提交时的请求时长结算为成功：把视频地址交给用户，在日志和审计里标记「时长未核验」。
  - 输出加输入超过 30 秒的违约成片，同样按请求时长结算。
  - 这样就不会再挂 24 小时，也不会每 15 秒重复下载成片。
  - 提交时对参考视频的探测（`relay/channel/task/sora/wan.go:40-58`）保持不变。
- **回归测试**：
  - 替换 `service/wan_video_test.go:145-170` 中固化「挂起」行为的断言；
  - 覆盖上游缺时长和上游时长越界两种情况：都按请求时长结算成功、写回视频地址、只结算一次。

- **现状**：上游已成功，但输出时长核验不过时（`service/task_polling.go:538-549`），轮询只返回 pending，任何状态都不落库，也不结算。唯一的兜底是 24 小时超时清扫，到时全额退款。
  - 测试 `service/wan_video_test.go:145-170` 把这一行为固化了下来。
- **核验失败的两类原因**：
  - 确定性原因：输出加输入超过 30 秒、URL 被 SSRF 拦截、不是 MP4、超过下载上限。这类任务一定挂满 24 小时。
  - 暂时性原因：网络错误、5xx、超时。
- **后果**：
  - 期间用户只看到 RUNNING 或 PENDING，拿不到视频；
  - 每 15 秒重新下载一次成片，单次最多 64 MB；
  - 24 小时后退款，上游成本由我们承担。
- **其他做法**：
  - keli 按提交时间给 120 秒宽限，超过就判失败并退款，实际上第一次核验失败就判失败；
  - 上游 new-api 的 Wan3 插件是按预扣结算成功，并把视频地址交给用户。

### R7 MiniMax TTS 计费

- **口径保留，不改**：按上游 `usage_characters` 计费（`relay/channel/minimax/tts.go:252-256`）。
  - MiniMax 官方规定 1 个汉字计 2 个字符，175/100 正好是 MiniMax 每计费字符的刊例价（hd 3.5 元/万、turbo 2.0 元/万），所以收费 = 供应商成本 × 分组倍率，不会亏。
  - keli 按本地字数计，中文只收成本的一半，分组倍率低于 2 时会亏，不采用。
  - 这一条依据用户「至少不亏」的原则确定。
- **R7a 已确认缺陷：上下限冲突，上游成功后仍返回 502**
  - 现状：输入允许 10000 个字（`relay/channel/minimax/adaptor.go:61`），响应却要求计费字符不超过 10000（`tts.go:190-192`）。文本超过约 5000 个汉字时，上游已经成功，FlowAPI 仍返回 502，不给音频。
  - 修法：响应的计费字符按「本地加权计数」（汉字 ×2，其余 ×1）的上界饱和处理，不再因为计数本身返回 502；上游计数缺失或越界时，改按本地加权计数计费，并写审计。预扣也按本地加权计数估算，与结算口径一致。

### R8 下掉音乐模型（用户 2026-09-28 决定）

- **现状**：
  - 占位名 `minimax-music-generation` 没有模型映射时，在请求上游之前就返回 400（`relay/channel/minimax/music.go:36-42`），预扣会退回，不扣费。71 没有配置映射，所以现在调用必然 400。
  - 71 挂的音乐模型与 MiniMax 现状对不上：缺 `music-3.0`，却列着已停服的 `-free` 和已下线的旧型号。
  - MiniMax 自 2026-08-20 起付费音乐接口不再对新用户开放。
  - 音乐近期没人用，也没接进 Canvas。
- **决定**：直接删，不做实测。
  - 代码：去掉 `/v1/music_generation` 不传 model 时的默认占位名（`middleware/distributor.go:435-437`），改为返回「model 必填」；从 `relay/channel/minimax/constants.go:10` 的模型列表和 `model/media_billing_units.json:87-90` 删掉占位名。
  - 生产配置（随上线执行，需授权）：删掉 71 上全部音乐模型和占位名，同时删掉它们的价格。
  - 保留音乐转发代码，包括 `/v1/music_generation` 路由和 `RelayMiniMaxMusic`（用户 2026-09-28 确认），以后需要时只配渠道和价格即可重新启用。

## Decisions

- **H3 计费改为按总秒数计**（用户 2026-09-28 确定）：
  - 计费秒数 = 输出时长 + 参考视频时长，按输出分辨率的单价计；图片、音频不另收。
  - 原因：metaso 对参考视频也按输出分辨率的秒价收费，而 FlowAPI 原先只按输出时长收费，参考视频长时会亏。
- **H3 单价**（用户 2026-09-28 确定）：
  - 768P 基准价从 0.19 改为 0.15 元/秒；
  - 2K 单价定为 0.25 元/秒，即 2K 倍率从 1.25（`relay/channel/task/hailuo/constants.go:60`）改为 5/3；
  - 分组倍率为 1 时，两档都是成本的约 1.67 倍；
  - 0.15 × 5/3 不能因浮点截断少算，2K 每秒必须精确等于 0.25 元（QuotaPerUnit=500000 时为 125000 额度）；
  - 768P 基准价属于 HK 生产配置 `options.ModelPrice`，随本任务上线一起改，需要上线授权。
- **H3 上游成本结构**（metaso，用户提供）：
  - 输出：2K 0.15 元/秒，768P 0.09 元/秒；
  - 输入素材：音频免费；图片前 5 张免费，超出每张 0.05 元；参考视频按输出分辨率的秒价收费；
  - 视频再生成（768P→2K）0.06 元/秒，其输入视频 0.06 元/秒、图片超出 5 张后每张 0.04 元；
  - Context IR 0.05 元/次；
  - 用户要求：FlowAPI 不照搬这套复杂规则，只要不亏。
- **H3 现状**（见 `research/h3-2k-ratio.md`）：
  - FlowAPI 只支持生成和查询两个接口，不支持视频再生成和 Context IR，所以这两项价格与我们无关。
  - 分辨率只有 768P 和 2K。
  - 计费方式：按请求时长预扣，结算时不调整。H3 按次计价，轮询结算被跳过（`service/task_polling.go:716-720`）。没传时长时默认 5 秒。
  - 参考视频时长在提交时拿不到：请求里没有这个字段，`mm_file://` 上传也不记录时长。完成时上游查询结果里有 `usage.input_seconds`，但 FlowAPI 目前不读。
  - 官方限额：参考视频最多 3 段、单段 2–15 秒、总计不超过 15 秒；参考图最多 9 张。
  - 29/19 和 1.25 的来源：29/19 来自 2026-08-07 用户给的成本价（768P 0.19、2K 0.29）；1.25 来自 2026-09-05 的调价（768P 0.20、高分辨率 0.25），只改了代码，没改库。两者都已被本次决定取代。
  - FlowAPI 代码里没有 H3 默认价，改价只需改生产配置和导入清单 `target-pricing-proposal.json`。

- **Seedream 联网搜索不另外收费**（用户 2026-09-28 确认）：R2 修复后联网搜索会真正生效，但计费仍按张计，不按搜索次数加收。

## Open Questions

- **Wan 计费改为按总秒数计**（用户 2026-09-28 确定）：
  - 计费秒数 = 成片时长 + 参考视频总时长，按分辨率单价计；参考图片和音频不计。
  - 与 RollDek 的规则保持一致：同一个参考视频 URL 只计一次。
  - 成片时长的取值沿用 R6：上游时长可信时用实际值，否则用请求时长。
  - 参考视频时长复用提交时已经测过的值（`relay/channel/task/sora/wan.go:40-58`）。
  - 只接 RollDek 的 `wan3.0-video-*` 和 `wan3.0-video-prime-*`，不接 `wan3.0-image-*`：
    - video 系列本身就支持首尾帧、参考图和参考音频；
    - 只传图片时，它只按成片计费，与 image 系列同价。
  - 币种为人民币（1:1，用户确认）。
- **Wan 总时长上限是阿里官方规则**（《万相3.0-视频生成API参考》）：
  - 没有视频输入时，duration 取 2–30 的整数；
  - 有视频输入时，输入视频总时长 + 输出视频时长不超过 30 秒；
  - 参考视频单段 1–15 秒、总计不超过 15 秒，最多 5 段。
  - FlowAPI 提交时已经校验这些规则，本任务不改。
- **RollDek 接口事实**（用户提供的 RollDek WAN 3.0 文档）：
  - 端点：`POST /v1/videos`、`GET /v1/videos/{task_id}`、`GET /v1/videos/{task_id}/content`。
  - 请求是扁平的 OpenAI 风格；模型名必须带分辨率后缀，后缀会覆盖请求里的分辨率。
  - 状态值：queued、in_progress、completed、failed。
  - 完成响应的 `metadata.url` 是带有效期的签名直链。
  - 文档没说完成响应里是否带时长字段，待 `research/prod-evidence.md` 核实。
- **Wan 实现核对结论**（`research/wan-rolldek-models.md`）：
  1. **模型名**：发给上游的是「公开模型名-小写分辨率」（`relay/channel/task/sora/wan.go:71`），所以永远落到 `wan3.0-video-*` 或 `wan3.0-video-prime-*`，与只接 video 系列的决定一致，不改。
  2. **现行计费**：
     - 预扣 = ModelPrice × QuotaPerUnit × 分组倍率 × 请求秒数 × 分辨率倍率（`relay/relay_task.go:218-225`）；
     - 结算把请求秒数换成实际成片秒数（`service/wan_video.go:134-160`）；
     - 参考视频时长在提交时测出，存入 `BillingContext.InputVideoDuration`（`controller/relay.go:784`），但只用于 30 秒上限校验。
  3. **修改落点**：
     - 预扣：秒数改为「请求秒数 + 参考视频秒数」（`relay/channel/task/sora/adaptor.go:108-111`）；
     - 结算：秒数加上 `bc.InputVideoDuration`，并纳入现有的非法值钳制（`service/wan_video.go:158`）。
  4. **主线按「不亏、不多收」定下的细节**：
     - 参考视频按 URL 去重。现在 `wan.go:40-58` 对同一个 URL 会重复下载、重复计入，15 秒和 30 秒上限也会因此误拒请求。
     - 去重后每段时长向上取整到整秒再合计，以免 RollDek 按整秒计费时标准版 480P（毛利约 10%）出现亏损。
     - 上线时仍在运行的旧任务，按原来的公式结算，不补扣参考视频的钱。旧快照的 `OtherRatios.seconds` 不含参考秒数，可以据此区分新旧任务。
  5. **与 RollDek 文档不一致、但本任务不改的 5 项**：
     - 放行 `http://` 和 `data:image/` 地址；
     - 放行 4:3 和 3:4 比例；
     - 有媒体时可能发出空 prompt；
     - 多发 `watermark:false`；
     - 超时是 24 小时，RollDek 建议 10–15 分钟。

     RollDek 是否真的拒绝这些都没有实测，本地收紧反而可能拦掉现在能跑通的请求；上游拒绝时会退款，不会亏。

以下是做出上述决定时的依据：

- **Wan 是否也按「成片 + 参考视频」计费**：
  - Rolldek 价目（用户 2026-09-28 提供，单位每秒，按人民币理解）：

    | 模型 | 480p | 720p | 1080p | 计费口径 |
    |---|---|---|---|---|
    | `wan3.0-video-*`（标准） | 0.27 | 0.34 | 0.36 | 参考视频 + 成片的总秒数 |
    | `wan3.0-video-prime-*`（高速） | 0.30 | 0.38 | 0.40 | 同上 |
    | `wan3.0-image-*`（图生，图片 + 音频参考） | 与标准同价 | | | 只按成片秒数 |
    | `wan3.0-image-prime-*`（图生高速） | 与高速同价 | | | 同上 |

  - FlowAPI 现行做法：只按成片秒数收费，参考视频时长只用于 30 秒上限校验（`service/wan_video.go:134-160`）。
  - FlowAPI 现价（分组倍率 1）：标准 480P / 720P / 1080P 为 0.30 / 0.60 / 1.20，高速为 0.45 / 0.90 / 1.80。
  - 亏损风险：参考视频长的请求会亏，尤其是 480P。
  - 待确认：FlowAPI 实际发给 Rolldek 的模型名，调研见 `research/wan-rolldek-models.md`；计费规则待用户决定。

## Acceptance Criteria

- [x] R1：ASR 的 submit 和 query 请求都不含 `X-Api-Sequence`（有回归测试），其余鉴权头不变。
- [x] R2：
  - 未开透传时，Seedream 请求保留客户端的 `tools`；
  - 非 Seedream 模型不转发 `tools`，Pro 删除 `tools`；
  - 以上都有回归测试；
  - `cd relaykit && GOWORK=off go build ./...` 通过。
- [x] R4a–R4e 各有回归测试：
  - `voice_clone` 用途的上传落到挂克隆模型的渠道；
  - `video_generation_input` 可以上传；
  - 数字和字符串格式的 `file_id` 都能解析；
  - 克隆字段名和取值范围与官方一致；
  - 上游明确拒绝后，同一个 voice_id 可以再次使用。
- [x] R7a：超过 5000 个汉字的文本，只要上游成功就返回音频，并按计费字符结算；计数缺失或越界时按本地加权计数饱和计费并写审计（有回归测试）。
- [x] R3、R5、R6、R8：每项都有核实结论或用户决定；确认的缺陷有修复和回归测试。
- [x] H3 计费有回归测试覆盖：
  - 计费秒数 = 输出时长 + 参考视频时长；
  - 分组倍率为 1、ModelPrice 为 0.15 时，768P 每秒计 0.15 元，2K 每秒计 0.25 元，额度精确、不因浮点截断少算；
  - 图片和音频不产生费用；
  - 预扣和结算都遵守计费安全不变量：不出现负数，饱和钳制可审计。
- [x] 受影响包的 `go test` 通过，`git diff --check` 通过。
- [x] Wan 计费有回归测试覆盖：
  - 预扣和结算的计费秒数 = 成片 + 按 URL 去重、每段向上取整的参考视频秒数；
  - 不带参考视频时，收费与原来一致；
  - 旧快照（`OtherRatios.seconds` 不含参考秒数）按原公式结算，不补扣；
  - 同一个 URL 重复传入时，只探测一次、只计一次，15 秒和 30 秒上限也按去重后的时长判断。
- [x] 列出上线时要执行的生产配置变更，例如 HK `options.ModelPrice` 中 MiniMax-H3 改为 0.15。本任务不执行这些变更。上线清单：
  1. HK `options.ModelPrice["MiniMax-H3"]` 0.19 → 0.15，导入清单 `target-pricing-proposal.json` 同步，B 复用。
  2. HK 71 号渠道删除全部音乐模型，并删除这些模型和 `minimax-music-generation` 的价格。
  3. 部署前备份数据库、compose 并打回滚镜像 tag。镜像在本机用 buildx 构建，不在 HK 构建。
  4. 上线后实测：ASR、声音克隆（上传→克隆）、H3 带 `mm_file` 参考素材、Wan 带参考视频。

## Out of Scope

- keli-api 的任何修改。
- FlowAPI B 的部署与渠道配置。
- 让 FlowAPI 以 FlowAPI 为上游转发音视频的开发（New API 类型的音频 / 任务适配器）。
- FastAI 的 2.0 / 2.0-fast 按秒路由和 `wan-3.0`：这是导入时有意不放开的，见 `.trellis/tasks/09-22-flowapi-provider-coverage/release-config-review.md`。
- `doubao-asr-2.0` 实际调用的是火山录音文件识别模型 1.0（`volc.bigasr.auc`，`asr.go:59`）。名称和定价是否要改，另行决定。

## 实施记录

- 2026-09-28 代码提交 `a9a333bd6`（本地 main，未 push、未部署）。A、B、C 三组全部完成，验收清单全部打勾。
  - 终检另修 2 处：按分钟计费的异步任务预扣少 1 额度（decimal 改动引入，已加 `Round(6)`）；H3 消费日志计费单位显示为「按次」（前端改为秒）。
  - 验证：受影响的 32 个 Go 包 `go test -count=1` 全部 ok；`go test -race` 通过；`cd relaykit && GOWORK=off go build ./... && go test ./...` 通过；前端 `tsc --noEmit` 通过；`git diff --check` 通过。
- 待办：HK 上线，需用户另行授权，按上面的上线清单执行。
