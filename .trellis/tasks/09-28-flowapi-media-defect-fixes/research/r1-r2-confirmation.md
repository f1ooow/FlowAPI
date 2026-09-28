# Research: R1 火山 ASR `X-Api-Sequence` / R2 Seedream `tools` 丢失

- **Query**: 确认 R1、R2 的代码事实、来源、测试位置与修法约束
- **Scope**: mixed（FlowAPI 源码 + keli 只读对比 + 火山官方公开文档）
- **Date**: 2026-09-28

---

## R1 火山 ASR `X-Api-Sequence`

### 结论

- **代码事实已确认**：submit `relay/channel/volcengine/asr.go:408`、query `:486` 都设置了 `X-Api-Sequence: -1`。仓库内只有这两处（grep 全仓）。同文件注释 `:18-20` 写的是 "no X-Api-Sequence which is streaming-only"。
- **来源已确认**：`8156149df` 新建 asr.go 时就带着这两行（`git show 8156149df -- relay/channel/volcengine/asr.go` 中新增的第 420、498 行）。
- **与 keli 的差异只有这一处**：逐函数 diff keli 与 FlowAPI 的 `asrSubmit` / `asrQueryOnce`，请求头的唯一差别就是这两行。其余差别是 FlowAPI 用 `LimitReader` 限制响应大小，并且状态码判定更严。keli HEAD `9b80cd99c` 的头设置见 keli `asr.go:353-357`（submit）和 `:418-422`（query），两处都没有 Sequence。
- **keli `364b4a068`（2026-05-28）的理由**，commit message 原文：
  > 删 X-Api-Sequence(流式专用, 文件 ASR 带它会被路由到流式鉴权 → 45000000 app key not found)

  这个 commit 同时改了 4 处：
  - Resource-Id 从 `volc.seedasr.auc` 改为 `volc.bigasr.auc`；
  - 鉴权头从 `X-Api-Key` 改为 `X-Api-Access-Key`；
  - 删除 `X-Api-Sequence`；
  - `X-Api-App-Key` 改为无条件设置。

  message 称"实测调通"。
- **反证：“必然失败”未确认**。火山官方《录音文件识别标准版HTTP》（getDocDetail DocumentID=1354868, LibraryID=6561，更新于 2026-06-26）的写法与 keli 的归因矛盾：
  - **submit** 的请求头表：旧版控制台（App-Key + Access-Key）和新版控制台（X-Api-Key）两种都列了 `X-Api-Sequence | 发包序号，固定值，-1`，示例代码里也带 `"X-Api-Sequence": "-1"`；
  - **query** 的请求头表**没有**这一项。

  keli 那次 commit 同时改了 4 处，`45000000` 也可能是 `X-Api-Key → X-Api-Access-Key` 这项改动引起的，不能单独归因于 Sequence。另外，官方错误码表里没有 `45000000`（表中只有 20000000–20000003、45000001/2/131/132/151、550xxxx、55000031）。
- **能确定的只有**：
  - keli 这套头（两处都不带 Sequence）经过端到端实测；
  - FlowAPI 这套（两处都带）从未实测：`release-gate-2026-09-25.md:18` 的 live smoke 只覆盖 Seedream、MiniMax TTS、Volc TTS 和 Wan，不含 ASR。

  按 PRD 验收标准“两处都不带”修改，就是对齐唯一实测过的配置，风险低。但 PRD R1 “已确认缺陷 / 必然失败”的表述缺少直接证据。
- **附带 caveat（不在 R1 范围）**：官方文档写明 `volc.bigasr.auc` = 豆包录音文件识别模型 **1.0**，`volc.seedasr.auc` = 模型 **2.0**。FlowAPI `asr.go:59` 用的是 bigasr，而公开模型名是 `doubao-asr-2.0`（`asr.go:201`）。名称、定价与实际资源版本是否一致，未确认。
- 官方 submit 的“Response body 为空”，任务 ID 就是请求里的 `X-Api-Request-Id`。FlowAPI `asr.go:424-432` 回退到 reqID 的逻辑与此一致。

### 现有测试

| 文件 | 内容 |
|---|---|
| `relay/channel/volcengine/audio_test.go:46-72` | `TestSeedTTSHeaderAndASRResultBounds`，只测 `validateASRResult`（`:55-63`），不涉及 ASR 请求头 |
| `relay/channel/volcengine/seedream_test.go`, `seedream_endpoint_test.go` | Seedream，无关 |
| FlowAPI 无 `asr_test.go` | — |
| keli `relay/channel/volcengine/asr_test.go:237-350` | `TestDoASRRequest_FullPipeline`，用 `hostRewriter`（`:403-413`）替换 `asrHTTPClient`；**mock 不检查请求头** |

可用的测试接缝：
- `asrHTTPClient` 是包级变量（`asr.go:78`）；
- `asrSubmit(ctx, token, appID, url, *asrContext, format)`（`:377`）和 `asrQueryOnce(ctx, token, appID, taskID)`（`:476`）是包内函数，可以直接调用，不需要 VOD 上传。

注意：FlowAPI 的 `asrPoll` 用 `time.NewTimer` 等待（`asr.go:466`），`asrSleepFn`（`:89`）声明了但没有使用（keli 用的是 asrSleepFn）。如果测试走 poll，且首个 query 返回“处理中”，会真实等待 2 秒。直接测 `asrQueryOnce` 可以避开。

### 回归测试应断言（testify require/assert，按 AGENTS.md）

- **submit**：mock 响应头 `X-Api-Status-Code: 20000000`、空 body。断言：
  - `req.URL.String() == asrSubmitEndpoint`；
  - `_, ok := req.Header["X-Api-Sequence"]` 为 false（用 map 判断，不能只看 `Get()==""`）；
  - `X-Api-Access-Key == token`，`X-Api-App-Key == appID`，`X-Api-Resource-Id == "volc.bigasr.auc"`；
  - `X-Api-Request-Id` 非空且等于返回的 taskID；
  - `Content-Type == application/json`。
- **query**：同一组头，其中 `X-Api-Request-Id == taskID`，同样不含 `X-Api-Sequence`。
- 放在新建的 `relay/channel/volcengine/asr_test.go` 或现有的 `audio_test.go`。

---

## R2 Seedream `tools` 字段在非透传路径被丢弃

### 结论（已确认）

- `relaykit/dto/openai_image.go`：
  - `:17-50` 的 `ImageRequest` 没有 `tools` 字段；
  - `:48-49` 定义了 `Extra map[string]json.RawMessage` 并标为 `json:"-"`；
  - `UnmarshalJSON`（`:52-78`）把未知字段收进 Extra；
  - `MarshalJSON`（`:81-104`）中合并 Extra 的代码被注释掉（`:95-101`，注释为“不能合并ExtraFields！！！”）。
- **禁止合并 Extra 的原因**：upstream 在 `00579f2f8` 引入 Extra 透传，`7b5d40678`（CaIon, 2025-10-03，message 引用 #1961）把合并注释掉。GitHub #1961 的标题是 “dall-e-3报错Unknown parameter: 'sync_mode'”：全局合并会把非 OpenAI 字段发给严格校验的上游。
- keli `dto/openai_image.go` 同样没有 tools、同样不合并（keli `:89-91`）。keli 生产靠全局 `pass_through_request_enabled=true` 透传 tools，见 `keli-canvas/.trellis/spec/important/project-docs/seedream-api-reference.md:217-219`。
- **非透传路径丢失 tools 的位置**：
  - `relay/image_handler.go:70-106` → `adaptor.ConvertImageRequest`；
  - 火山渠道：`relay/channel/volcengine/adaptor.go:115-118` → `seedream.go:11-28`，返回的是 dto 值；
  - `image_handler.go:84` 的 `common.Marshal(convertedRequest)` 走 dto `MarshalJSON`，tools 在这一步丢失。
  - `common.DeepCopy`（`common/copy.go:9-19`，copier DeepCopy）会保留 Extra，所以丢失发生在 Marshal，不在 DeepCopy。
- **ParamOverride 救不回来**：`image_handler.go:89-95` 作用在已经丢了 tools 的 JSON 上。override 的条件上下文只有 `original_model` 等字段（`relay/common/override.go:2102-2124`），读不到客户端原始 body，只能静态 set。
- **B→A 链路同样受影响**：New API 类型渠道 `relay/channel/newapi/adaptor.go:88-89` → `relay/channel/openai/adaptor.go:572-573`（default 分支原样返回 dto），同样在 `image_handler.go:84` 丢失。Zivv 82（type 60）同理。
- **透传路径保留 tools**：`image_handler.go:57-69` → `relay/helper/passthrough_body.go:16-43`，只在有模型映射时改写 `model`，其余字段原样保留。
- Canvas 发送 tools 的位置：`keli-canvas/server/lib/imageProvider.ts:1194-1200`（`if (data.webSearch === true) payload.tools = [{type:'web_search'}]`）。
- 响应侧没有问题：`OpenaiImageHandler` 把上游 body 原样写回客户端（`relay/channel/openai/relay_image.go:34-69`，`IOCopyBytesGracefully`），`tools` 和 `usage.tool_usage` 都会返回。

### `prepareSeedreamImageRequest`（`relay/image_seedream.go:21-54`）

透传和非透传两条路径，只要 `dto.IsSeedreamModel(info.UpstreamModelName)` 成立，都会在 body 构造完成后调用它（`image_handler.go:109-114`）。它依次做以下事：

1. `:22-24`：只允许 `/v1/images/generations` + `application/json`。
2. `:25-32`：读取 body 后调用 `normalizeSeedreamImagePayload`（`:56-98`），其中：
   - 先按 `map[string]json.RawMessage` 解析；
   - 只对 Pro 删除 `sequential_image_generation(_options)` 并用 map 重新编码（`:67-75`），其余字段原样保留（注释 `:57-58`："Preserve arbitrary passthrough fields… Never roundtrip the full body through DTO"）；
   - 再反序列化成 dto，**仅用于校验**：n≤128、max_images≤128、layer 仅限 Pro、layer 时 n=1、layer 不支持流式（`:80-96`）。
3. `:33-35`：设置 `seedream_layer_decomposition` 和 `info.IsStream`。
4. `:36-52`：UsePrice 且没有 tiered 快照时，按最终 payload 重算 `n` 倍率，并调用 `PrepareBillingForSelectedRoute` 重新预扣。
5. `:53`：返回 map 级别的 raw，不经过 DTO 往返。

两条路径的差别：
- **透传**：输入就是客户端原始 body，tools 保留，本函数不校验 tools。
- **非透传**：输入是 DTO 序列化后的 body，tools 已经丢失，本函数无法恢复。另外 ParamOverride 在它之前生效，所以 override 开启 layer 的情况也会被校验到（测试 `relay/image_seedream_test.go:123-128`）。
- 现有测试 `relay/image_seedream_test.go:63-83` 已断言未知字段 `"unknown"` 在 normalize 后保留，说明 Seedream 的设计本来就接受任意字段。

### 官方请求字段 vs DTO

依据：火山方舟《图片生成 API》，getDocDetail DocumentID=1541523（LibraryID=82379），更新于 2026-09-22，覆盖 5.0 pro/flash/lite、4.5、4.0。

| 官方字段 | DTO | 备注 |
|---|---|---|
| model | ✓ `Model` | |
| prompt | ✓ `Prompt`（无 omitempty） | 图层拆分场景下官方为可选 |
| image | ✓ `Image` RawMessage | |
| size | ✓ `Size` | |
| sequential_image_generation | ✓ `*string` | |
| sequential_image_generation_options.max_images | ✓ 类型化 struct，只含 max_images（`seedream_image.go:13-15`） | 官方目前也只有这一个子字段 |
| **tools[].type** | **✗ 缺失** | 官方：仅 5.0 lite 支持；5.0 pro/flash “暂不支持组图生成、联网搜索、流式输出” |
| stream | ✓ `*bool` | |
| response_format | ✓ | |
| output_format | ✓ RawMessage | |
| watermark | ✓ `*bool` | |
| optimize_prompt_options(.mode) | ✓ RawMessage | |
| background | ✓ RawMessage | 5.0 pro/flash；与 OpenAI 字段同名 |
| layer_decomposition | ✓ `*bool` | |

- **结论**：按当前官方文档，DTO 缺的请求字段只有 `tools`。DTO 中的 `seed` / `guidance_scale` 不在 4.0–5.0 文档里；keli-canvas 文档 `:58`、`:173` 也注明本系列不支持 guidance_scale。
- `GetAndValidOpenAIImageRequest` 在 n 为空时会补 `n=1`（`relay/helper/valid_request.go:281-283`），所以非透传请求总是带 `"n":1`。官方文档里没有 n 字段，但 Seedream smoke 已通过，说明上游能容忍。
- **计费未确认**：官方说开启联网搜索后，实际搜索次数见 `usage.tool_usage.web_search`。Seedream 联网是否另外收费，《模型价格》文档（DocumentID=1544106）中未能确认：`web_search 0.02 元/次` 出现在 Managed Agents 段落。FlowAPI 只按图片张数计费。

### 修法评估

**方案 A：DTO 增加 `Tools json.RawMessage \`json:"tools,omitempty"\``（改 relaykit）**
- 改动最小。`GetJSONFieldNames`（`openai_image.go:106-129`）会自动把 tools 从 Extra 中排除。
- 影响面：所有直接序列化 DTO 的图片渠道（openai、newapi、volcengine 等），在客户端带 tools 时都会转发 tools。OpenAI Images API 没有 tools 参数，以前被静默丢弃的请求可能变成 400，与 #1961 同类。可以在非 Seedream 路径清掉 tools，或只在 `dto.IsSeedreamModel(UpstreamModelName)` 时保留。
- Pro 不支持联网，转发 tools 是否会 400，未确认。可以参照 sequential_* 的处理，在 normalize 中对 Pro 删除 tools。

**方案 B：对 Seedream 保留全部未知字段**
- 实现方式：在 relay 层让 Seedream 非透传也使用“原始 body + normalize”，或让 `ConvertSeedreamImageRequest` 返回合并了 Extra 的 map。
- 优点：
  - 与 `normalizeSeedreamImagePayload` 的现有设计意图一致；
  - 以后官方新增字段不必再改 DTO；
  - 若在 `relay/` 层实现，可以不碰 relaykit。
- 风险：未知字段绕过 DTO 校验，AGENTS.md 计费不变量提醒过“passthrough 字段可能携带计费乘数”。当前 Seedream 已知的乘数（n、max_images、layer_decomposition）都已在 normalize 中对最终 payload 校验，不受影响；但未知的、会影响成本的参数（如联网）不计费。另外会扩大未经审查的上游输入面。
- 需要注意的等价点：非透传路径会应用 ParamOverride（`image_handler.go:89-95`）和 `ConvertSeedreamImageRequest` 的校验（Pro 不能走原生火山渠道等，`seedream.go:12-26`），透传路径两者都不做。方案 B 需要保留这两项。

**对比**：只修 tools 最小、可验证，但以后可能再漏字段；保留全部未知字段更贴合 Seedream 现有设计，代价是输入面扩大。由主线决定。无论选哪个，都应在 Pro 上剔除 tools，或先实测确认不会报错。

### relaykit 约束

- `relaykit/go.mod` 是独立 module（`module github.com/QuantumNous/new-api/relaykit`，go 1.25.1）。根 `go.mod:167,170` 用 `replace … => ./relaykit` 引用，仓库没有 go.work。
- AGENTS.md 要求：relaykit 不得 import 根模块；任何改动都必须通过 `cd relaykit && GOWORK=off go build ./...`（根模块构建通过不算）。relaykit 内 JSON 用 `kitutil`（`openai_image.go:9,55,85`），不用 `common.*`。
- `.trellis/spec/backend/channel-reliability.md:132-133` 有同样要求。
- 基线：2026-09-28 本次执行 `cd relaykit && GOWORK=off go build ./...`，通过。

### 现有测试位置

| 文件 | 覆盖 |
|---|---|
| `relay/channel/volcengine/seedream_test.go:14-41` | Unmarshal → DeepCopy → ConvertSeedreamImageRequest → Marshal，用 `JSONEq` 比较。**最适合加 tools 回归**：在 raw 里加 `"tools":[{"type":"web_search"}]`，现在会 red，修复后 green |
| `relay/image_seedream_test.go` | normalize / prepare / 预扣 / Pro 字段删除 / 未知字段保留（`:63-83`） |
| `relay/channel/openai/seedream_image_test.go` | 响应侧、Pro 兼容端点 |
| `service/seedream_image_billing_test.go` | 图层计费 |
| `relay/passthrough_model_test.go` | 透传时的 model 重写 |
| `relaykit/dto/*_test.go` | **没有** ImageRequest 测试 |
| ImageHelper 级别的非透传 body 测试 | 不存在 |

## Caveats / Not Found

- 生产 76 “未开透传”、全局 `PassThroughRequestEnabled` 取值：来自 PRD，未直接核实（未 SSH）。
- Seedream 联网搜索是否另外计费、Pro 带 tools 是否报错：未确认。
- R1 的 `45000000` 与 `X-Api-Sequence` 之间的因果关系：未确认（证据冲突，见上文）。
