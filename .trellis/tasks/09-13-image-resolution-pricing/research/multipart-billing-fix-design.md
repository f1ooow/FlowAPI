# Design: 让 multipart 图像请求的计费表达式读到 `size` / `n`（最小改动方案）

- **Query**: `/v1/images/edits` 等 multipart 请求下 `param()` 全为 nil，表达式只能落兜底档、张数恒按 1；用户同意改网关代码，要求最优雅的最小方案，且能 cherry-pick 到下游 new-api
- **Scope**: 只设计不写代码。结论基于读源码 + 在 `/tmp` 临时模块里对真实 `relay/helper` / `pkg/billingexpr` / `relaykit/dto` 实跑验证前提（**未修改仓库任何代码**，临时模块已清理）
- **Date**: 2026-09-13
- **基线**: FlowAPI `main@9794c7363`（与上游 merge-base `e468b7391 2026-08-27`）

---

## 结论

**改一个函数、加约 12 行，不碰 `relaykit/`，不改 JSON 路径。** 在 `relay/helper/billing_expr_request.go` 的 `ResolveIncomingBillingExprRequestInput` 里：当入站 body 不是 JSON（`readIncomingBillingExprBody` 返回 nil）且 `info.Request` 是已解析的 `*dto.ImageRequest` 时，把这个 DTO 剥掉重字段后交给**已有的** `BuildBillingExprRequestInputFromRequest` 序列化成 JSON 当表达式 body。预扣时 DTO 已解析完成（时序见 §1），结算复用预扣冻结的同一份 body（现有机制，不动）。改完后 multipart 编辑请求的 `param("size")` / `param("n")` / `param("quality")` / `param("model")` 与 JSON 请求同名同值，6 条表达式回到第 5 版干净形态，无需 `isMultipart` 分支。

实跑验证（真实 multipart 请求 → 真实 `GetAndValidOpenAIImageRequest` → 真实 `ResolveIncomingBillingExprRequestInput`）：现状 body 0 字节 → 表达式落 `1k`；按本方案投影后 body 117 字节 `{"model":"gpt-image-2.5-flare","n":4,"prompt":"","quality":"high","size":"2048x1152","stream":false,"watermark":true}` → 同一条表达式落 `2k` × 4 张。

---

## 1. 时序确认：预扣时 multipart DTO 已经解析完成

```
controller/relay.go:121   request, err := helper.GetAndValidateRequest(c, relayFormat)
  └ relay/helper/valid_request.go:182-287  GetAndValidOpenAIImageRequest
      ├ :186-232  multipart 分支：ParseMultipartFormReusable → imageRequest.Prompt/Model/N/Quality/Size/Stream/Image(文本)/Watermark
      │           :197-203 n 校验 0..128；:222-224 N 为 nil/0 → 1
      └ :234-284  JSON 分支：UnmarshalBodyReusable（原样）
controller/relay.go:132   relayInfo := GenRelayInfo(c, relayFormat, request, ws)
  └ relay/common/relay_info.go:437-441 GenRelayInfoImage → genBaseRelayInfo
      :511  Request: request            ← 解析好的 *dto.ImageRequest 挂到 info.Request
      :532  RequestHeaders: cloneRequestHeaders(c)
controller/relay.go:165   helper.ModelPriceHelper(c, relayInfo, tokens, meta)
  └ relay/helper/price.go:99-101  tiered_expr → modelPriceHelperTiered
      :307  requestInput, err := ResolveIncomingBillingExprRequestInput(c, info)   ← 改动点
      │   └ relay/helper/billing_expr_request.go:13-35
      │       :29  bodyBytes := readIncomingBillingExprBody(c)   → multipart 时 nil（:53-62 只认 application/json）
      │       [新增] 若 bodyBytes 为空且 info.Request 是 *dto.ImageRequest → 投影 DTO 为 JSON
      :312  billingexpr.RunExprWithRequest(exprStr, {P,C,Len}, requestInput)      ← param() 现在读得到
      :351-352  info.TieredBillingSnapshot = snapshot；info.BillingRequestInput = &requestInput   ← 冻结
controller/relay.go:176   service.PreConsumeBilling
relay/image_handler.go:149  service.PostTextConsumeQuota
  └ service/text_quota.go:454  TryTieredSettle(...)
      └ service/tiered_settle.go:208-211  requestInput = *relayInfo.BillingRequestInput   ← 复用冻结 body，不再 Resolve
```

- **预扣时机 DTO 可得**：`GetAndValidateRequest`（`:121`）先于 `ModelPriceHelper`（`:165`），`info.Request` 在 `GenRelayInfo`（`:132`）就已挂上。实跑确认 multipart 解析后 `Model/Size/Quality/N/Stream/Watermark` 全部就绪。
- **结算无需改**：`TryTieredSettle` 只读 `relayInfo.BillingRequestInput`，是预扣时冻结的那份；改动只影响冻结前的构造，预扣/结算天然一致。
- **重试/切组无需改**：`PrepareBillingForSelectedRoute`（`tiered_settle.go:164-196`）只刷新分组倍率，不重新 Resolve；`ResolveIncomingBillingExprRequestInput` 第一行就检查 `info.BillingRequestInput != nil` 直接返回克隆（`:14-22`）。
- **渠道测试路径无需改**：`controller/channel-test.go:518-528` 直接调 `BuildBillingExprRequestInputFromRequest`，不经过 `ResolveIncoming…`。
- **注入点唯一正确位置就是 `ResolveIncomingBillingExprRequestInput`**：它是生产 relay 路径上表达式 body 的唯一构造者（全仓只有 `price.go:307` 调用），改这里预扣与结算自动一致；改 `readIncomingBillingExprBody` 也可以但它拿不到 `info`，改 `modelPriceHelperTiered` 则把 DTO 知识泄漏进定价逻辑。

## 2. 改动点清单

| # | 文件 | 位置 | 改什么 | 行数 |
|---|---|---|---|---|
| 1 | `relay/helper/billing_expr_request.go` | `ResolveIncomingBillingExprRequestInput` `:29-34` 之间 | `readIncomingBillingExprBody` 返回空时，若 `info.Request` 是 `*dto.ImageRequest`，构造剥掉重字段的副本，调用已有 `BuildBillingExprRequestInputFromRequest(&projected, info.RequestHeaders)`，用其 `Body` 作为 `input.Body` | ≈12 |
| 2 | `relay/helper/billing_expr_request_test.go` | 追加 | 表驱动回归测试（§6） | ≈70 |
| 3 | `relay/helper/price_test.go` | 追加 | multipart 端到端：`modelPriceHelperTiered` 用 `param("size")`/`param("n")` 表达式预扣正确（§6） | ≈50 |
| 4 | `.trellis/tasks/.../final-expressions-verification.md` | — | 表达式回到第 5 版形态（本身不含 `isMultipart`，无需改） | 0 |

改动 #1 的形状（示意，非最终代码）：

```go
bodyBytes, err := readIncomingBillingExprBody(c)
if err != nil {
    return billingexpr.RequestInput{}, err
}
if len(bodyBytes) == 0 && info != nil {
    if imageRequest, ok := info.Request.(*dto.ImageRequest); ok {
        // multipart / form 请求没有 JSON body：用已解析并通过校验的 DTO 投影出计费用 body。
        // 只保留标量字段，图片与提示词不进表达式 body（image 文本字段可能是几百 KB 的 base64）。
        projected := *imageRequest
        projected.Prompt = ""
        projected.Image = nil
        projected.Images = nil
        projected.Mask = nil
        projected.Extra = nil
        built, err := BuildBillingExprRequestInputFromRequest(&projected, info.RequestHeaders)
        if err != nil {
            return billingexpr.RequestInput{}, err
        }
        bodyBytes = built.Body
    }
}
input.Body = bodyBytes
```

- 复用 `BuildBillingExprRequestInputFromRequest`（`:37-51`，内部 `common.Marshal` → `ImageRequest.MarshalJSON` `relaykit/dto/openai_image.go:74-97`），不新写序列化。
- 只在 `bodyBytes` 为空时介入：JSON 路径（`application/json` 有 body）零改动。
- 类型断言只认 `*dto.ImageRequest`：音频等其它 multipart DTO 行为不变（仍为 nil body）。

## 3. 字段一致性（DTO 投影 vs 客户端 JSON）

`dto.ImageRequest` 的 json tag（`relaykit/dto/openai_image.go:17-43`）：`model, prompt, n, size, quality, response_format, style, user, extra_fields, background, moderation, output_format, output_compression, partial_images, stream, images, mask, input_fidelity, watermark, watermark_enabled, user_id, image`；`Extra` 是 `json:"-"` 且 `MarshalJSON` 明确不合并（`:88-94`）。投影用的是同一套 tag，**键名与客户端 JSON 完全一致**。

实跑投影结果（multipart 带 model/prompt/size/n/quality/stream/watermark/response_format/image 文本 + 文件）：

```
{"model":"gpt-image-2.5-flare","n":4,"prompt":"","quality":"high","size":"2048x1152","stream":false,"watermark":true}
```

与 JSON 路径的差异（都在表达式不读或读不到的地方，对 6 条表达式无影响）：

| 字段 | JSON 原始 body | multipart 投影 | 影响 |
|---|---|---|---|
| `size` / `quality` / `model` | 客户端原文 | 校验后 DTO 值（`size` 原样；`quality` 仅 gpt-image-1 缺省补 `standard`，`valid_request.go:217-221`） | 一致 |
| `n` | 客户端原文（可为 0） | 校验后 0..128，nil/0 归一为 1（`:197-203,222-224`） | multipart 下 `param("n")` 永不为 0；表达式的 `max(…,1.0)` 保留用于 JSON |
| `prompt` | 原文 | 空串（有意剥掉） | 无表达式读它 |
| `image` / `images` / `mask` | 原文（JSON edits 可含 base64） | 剥掉 | 必须剥：multipart 的 `image` 文本字段可能是 base64 data URI，实跑不剥时 body 201 KB、剥后 117 字节 |
| `response_format` / `background` / `output_format` 等 | 原文 | **不出现**（multipart 分支 `:186-232` 根本不解析这些表单字段） | 无表达式读它 |
| 未知字段（`extra_body` 等） | 原文可读 | 不出现（`Extra` 不序列化） | Gemini 的 `extra_body` 只在 chat JSON 路径出现，与 multipart 无关 |

## 4. 不破坏现有行为

| 要求 | 保证方式 |
|---|---|
| JSON 请求路径完全不变 | 只在 `len(bodyBytes) == 0` 时介入；`application/json`（含 `; charset=utf-8`）都有 body |
| 已配置 `tiered_expr` 的其它模型（含生产中的 `gemini-3-pro-image-preview`） | 原生 `generateContent` 与 chat 都是 JSON → 不介入；Gemini 若走 multipart edits，`info.Request` 是 `*dto.ImageRequest` → 投影生效，`param("size")` 从 nil 变成真实值 —— 这是本改动的目的，且 gemini 表达式对 `size` 判档已在第 5 版验证 |
| 预扣与结算同一份 body | 投影发生在 `price.go:307` 的 Resolve 里、`:352` 冻结之前；结算 `tiered_settle.go:208-211` 只读冻结值 |
| `relaykit/` 独立性 | 不触及；`dto.ImageRequest` 只是被读取和值拷贝 |
| JSON 包规范 | 走 `BuildBillingExprRequestInputFromRequest` 内部的 `common.Marshal` |
| passthrough 开启的渠道 | `GetAndValidateRequest` 仍先跑，DTO 照样解析（`relay/image_handler.go:49-54` 只影响发上游的 body） |
| `x-www-form-urlencoded` 的 images 请求 | 现状 `param()` 为 nil；改后同样走投影（DTO 经 `parseFormData` 解析），行为与 multipart 对齐 |
| 音频 multipart（`/v1/audio/transcriptions` 等） | `info.Request` 是 `*dto.AudioRequest`，断言失败 → 仍为 nil body，与现状一致；这些模型若配了 `tiered_expr`，`param()` 行为不变 |

## 5. 风险与规避

| 风险 | 规避 |
|---|---|
| 图片二进制进表达式 body | 文件部分在 `form.File`，从不进 DTO；但 `valid_request.go:213-215` 会把**文本** `image` 字段（可能是 base64 data URI）塞进 `DTO.Image`，所以投影必须置空 `Image/Images/Mask`。实跑：不剥 201,163 字节，剥后 117 字节 |
| 提示词进表达式 body / 冻结在 `RelayInfo` 里 | 置空 `Prompt`（JSON 路径本来就含 prompt，这里只是不额外复制一份） |
| `n` 语义变化 | multipart 下 `param("n")` 从 nil 变为校验后的 1..128：张数开始按声明收，这是目标；上界由 `dto.MaxImageN`（`openai_image.go:15`）在校验层保证，符合 AGENTS.md「用户可控乘数在校验层设界」 |
| 老配置依赖「multipart 必落兜底」 | 第 5 版 6 条没有 `isMultipart` 分支，无依赖；若有人配了 `has(header("content-type"), "multipart/form-data")` 之类的判别式，改后仍成立（header 不变），只是 `param()` 也有值了 |
| 与上游 09-09 补丁的关系 | 上游 `ResolveImageBillingRequestInput`（`upstream/main:relay/helper/billing_expr_request.go:40-66`）只在表达式引用 `image_count` 时才用 `{"model","n","size","quality","parameters"}` 覆盖 body，不引用时 multipart 仍为 nil —— 即上游也没有覆盖本场景，本方案不是重复造轮子。两者可共存：本方案先填 body，上游逻辑在用到 `image_count` 时再覆盖，键集是本方案的子集 |
| 下游 cherry-pick | 改动只在 `ResolveIncomingBillingExprRequestInput` 的 `readIncomingBillingExprBody` 之后插入一段；上游版本该函数除 `maps.Copy` 外与本仓库逐行相同，上下文行一致，`git cherry-pick` 大概率干净应用；即使下游已含 09-09 补丁也不冲突（补丁改的是同文件的另一处新增函数与 `price.go:379` 的调用） |
| 不依赖 FlowAPI 特有代码 | 用到的 `dto.ImageRequest`、`BuildBillingExprRequestInputFromRequest`、`info.Request` 都是上游同名同签名 |

## 6. 测试清单（`testify/require`，表驱动）

`relay/helper/billing_expr_request_test.go`：

| 用例 | 输入 | 断言 |
|---|---|---|
| multipart edits 投影 | `mime/multipart` 构造 model/prompt/size/n/quality + 文件 + 文本 `image`（base64） → 真实 `GetAndValidOpenAIImageRequest` → `RelayInfo{Request: dto, RequestHeaders}` → `ResolveIncomingBillingExprRequestInput` | `gjson` 读 `size`/`n`/`quality`/`model` 等于表单值；`prompt`/`image` 不存在或为空；`len(Body) < 512`；`Headers["Content-Type"]` 含 `multipart/form-data` |
| multipart `n` 缺省 / `n=0` | 同上，不传 `n` / 传 `0` | `param("n")`（`gjson`）为 `1` |
| JSON 路径不变 | `application/json` body | `Body` 与原始字节逐字相等（含 `n:0`、未知字段保留） |
| 非图像 multipart | `RelayInfo{Request: &dto.AudioRequest{}}` + multipart 头 | `Body` 为 nil |
| 冻结复用 | 先 Resolve 一次并赋给 `info.BillingRequestInput`，再 Resolve | 第二次返回的 `Body` 与第一次相同，且是克隆（修改副本不影响原件） |
| `x-www-form-urlencoded` images | `parseFormData` 路径 | 同 multipart 投影 |

`relay/helper/price_test.go`（照 `TestModelPriceHelperTieredUsesPreloadedRequestInput` `:19-65` 的写法，用 `config` 注入 `billing_setting.billing_mode/billing_expr`）：

| 用例 | 表达式 | 断言 |
|---|---|---|
| multipart 预扣按 size 判档 | 第 5 版 B/C 表达式 | multipart `size=2048x1152, n=4` → `QuotaToPreConsume == 4 × 80000`，`TieredBillingSnapshot.EstimatedTier == "2k"`，`info.BillingRequestInput.Body` 含 `"size":"2048x1152"` |
| multipart 与 JSON 同请求同结果 | 同上 | 同一组 size/n 用 JSON 与 multipart 各发一次，`QuotaToPreConsume` 相等 |

`pkg/billingexpr` 无需新增（`param()` 行为未变）。

## 7. 预估

- 生产代码：≈12 行（1 个函数内新增一段，无新符号或新文件）。
- 测试：≈120 行（两个已有测试文件各追加一组表驱动用例）。
- 不需要迁移、不需要前端改动、不需要 `relaykit` 构建验证（未触及）。
- 上线后 6 条表达式无需改；multipart 编辑请求开始按 `size` 判档、按 `n` 收张数。

## 8. 上线后行为变化（需要告知用户）

- gpt-image 系 multipart 编辑：`size=2048x1152` 从 1K 档（¥0.12）变为 2K 档（flare ¥0.16 / gpt-image-2 ¥0.12）；`2880x2880` 从 1K 变为 4K（¥0.21）；`auto`/不传仍 1K。按上游账单 61 条编辑请求计算，现状 −¥1.14 → 改后与 JSON 路径同口径（flare 2K 尺寸平价、gpt-image-2 2880² 平价、小图 +0.092）。
- multipart 表单里 `n>1` 开始按张数收费（原来恒按 1 张）。
- Gemini 若有 multipart 编辑流量，同样按 `size` 判档。

## 附：本设计不选的方案

| 方案 | 不选原因 |
|---|---|
| 表达式里加 `isMultipart ? tier("2k"…)` 兜底 | 治标：所有编辑请求一刀切一个档，`2880x2880` 仍亏、小图多收、`n` 仍读不到；6 条表达式各带一个分支 |
| 让 `readIncomingBillingExprBody` 把 multipart 表单值通用转成 JSON | 拿不到 `info`，只能盲转全部表单字段（含 `image` 文本 base64），且绕过了校验层的归一化（`n` 0/缺省 → 1、上界 128） |
| 直接同步上游 09-09 两个提交 | 引入 `fixed()`/`image_count`/`img_cr`/`u()` 与迁移接口，改动面大；且上游只在表达式用 `image_count` 时才投影 multipart，不覆盖本场景 |
| 在 `GenRelayInfoImage` 里预先构造 `BillingRequestInput` | 把计费 body 的构造分散到两处，且对非 tiered 模型白做一次序列化 |
