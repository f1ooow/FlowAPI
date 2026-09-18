# Research: gpt-image-2 按分辨率计费的可行性

- **Query**: 把 `gpt-image-2` 改成按分辨率计费（1k=$0.1 / 2k=$0.15 / 4k=$0.2，按张），需要做什么；并且用户希望**按实际出图**而非按请求声明的 `size` 计费
- **Scope**: internal（无外部检索工具可用，见「Caveats」）
- **Date**: 2026-09-11
- **纪律**: 只读。未修改任何生产文件。为验证表达式语法，曾在 `pkg/billingexpr/` 临时放一个 `zz_tmp_probe_test.go` 跑 `go test` 后立即删除，工作区无残留。

## 提交前复核（2026-09-18）

下文两轮内容为 2026-09-11 的探索记录，保留当时的方案演进和代码行号，不是当前实施方案或生产状态。本次只复核文档和本地代码，未复跑历史表达式试验、调用供应商或连接生产服务器。

以下更正优先于两轮正文：

- **multipart 限制已修复**：当前 `relay/helper/billing_expr_request.go` 会将已解析的图像 DTO 投影为计费 JSON，`param("size")` / `param("n")` 不再因 multipart 必然返回 nil。后续采用的按请求分辨率方案及 22 条双侧验证记录见 `../../09-13-image-resolution-pricing/research/final-expressions-verification.md` 与 `../../09-13-image-resolution-pricing/research/keli-log-verification.md`。正文的 B/C 推荐及“multipart 全线失效”属于历史结论。
- **尺寸解码不能统一限定为前 64 字节或 O(1)**：PNG 的尺寸字段靠前，但 JPEG 的 SOF 前可能有较长的 APP/EXIF 段；`image.DecodeConfig` 会扫描至所需信息。实现必须采用有界读取，并明确超过上限或无法解析时的处理。正文固定 64 字节的方案不具备通用正确性。
- **测试样本仅证明解析契约**，不能证明真实供应商一定返回 usage。DTO 没有尺寸字段，也不证明上游原始响应一定没有尺寸元数据。
- **token 计价只是有条件的选项**：必须确认供应商真实计价单位、usage 完整性、输入/输出明细和相应费率。不能仅凭代码宣称 token 数与像素尺寸严格对应、总 token 等于实际成本或毛利率恒定。当前配置还需检查 `billing_mode`：已有 `tiered_expr` 时，仅删除固定价并设置倍率不会切换为 token 倍率计费。
- **字段兜底和安全护栏需分开看**：`PromptTokens == 0` 即会被补成 1，即使上游给了有效输出 usage；`img_o` 在上游使用 `output_tokens_details` 时未被该 DTO 解析，但兼容字段 `completion_tokens_details.image_tokens` 可以填入，不能断言其恒为 0。普通倍率路径的 `baseTokens` 下界也不等于所有 usage 字段已校验；额度饱和和审计不能保证上游计费数据真实。
- **失败与价格配置有例外**：第一轮 Q8.2 展示的代码仍调用 `ChargeViolationFeeIfNeeded`，因此“上游错误完全不计费”不适用于违规费等例外。第二轮所谓“给无 usage 渠道单独配置固定价”需要独立模型别名或其他已支持的配置隔离；全局模型价格本身不能按同名模型的不同渠道分别切换计费模式。

---

## TL;DR

| 问题 | 结论 |
|---|---|
| 现在能否**纯配置**做到「按分辨率不同单价」 | **能，但只能按"请求里声明的 `size`"**，且必须用 `tiered_expr` 表达式模式（不是填价格表数字）。走这条路会**丢掉现有的「按实际出图张数计费」能力**（见 Q4 致命副作用） |
| 现有「分辨率 / 质量」计费设施 | 存在但**只对 `dall-e*` 硬编码生效**（`relaykit/dto/openai_image.go:137`），`gpt-image-*` 永远 `sizeRatio=1.0`。`upstream-ratio-sync-helpers.ts` 的 `Resolution*` 是**冲突解决(conflict resolution)**，与图片分辨率无关——这是个假线索 |
| 按**实际出图**计费 | **技术上可做，但需要写代码**。上游响应里**没有任何尺寸元数据**；唯一来源是解码 `b64_json` 图片头。计费时刻后端**确实还持有完整字节**，所以可行 |
| 「逐张不同单价」 | `types.PriceData` 结构上**只支持「单价 × 倍率连乘」**，但可以把「Σ每张档位系数」塞进一个 `OtherRatio` 来精确表达混合分辨率。**不需要改 PriceData 结构** |
| `size` 现有校验 | **完全没有**。`gpt-image-*` 在 `relay/helper/valid_request.go` 没有任何 size 白名单，任意字符串都放行 |

---

## Q1 — `gpt-image-2` 当前的完整计费链路

### 1.1 链路总览（固定价 / per-call 模式）

| 步骤 | 位置 | 做了什么 |
|---|---|---|
| 请求校验 | `relay/helper/valid_request.go:182` `GetAndValidOpenAIImageRequest` | 解析 JSON 或 multipart；`n` 上界 `MaxImageN=128`（:249 / :199）；`size` **对 gpt-image 无任何校验**；`n==nil→1`（:281） |
| 计费元数据 | `relaykit/dto/openai_image.go:133` `GetTokenCountMeta()` | 产出 `ImagePriceRatio = sizeRatio*qualityRatio`（gpt-image 恒为 `1.0`）和 `BillingRatios={"n": 声明的n}`（:168-169），`MaxTokens: 1584` |
| 快路径取 meta | `controller/relay.go:467-469` | 图片请求即使关了 token 计数也强制走 `GetTokenCountMeta()`，注释写明「Pricing for image requests depends on ImagePriceRatio」 |
| 价格解析 | `relay/helper/price.go:93` `ModelPriceHelper` | `ratio_setting.GetModelPrice(OriginModelName,false)`（:94）→ `usePrice`；`HandleGroupRatio`（:96）算分组/渠道倍率 |
| 分辨率倍率注入 | `relay/helper/price.go:148-150` | `if meta.ImagePriceRatio != 0 { modelPrice = modelPrice * meta.ImagePriceRatio }` ← **这就是"分辨率倍率"唯一的注入点** |
| n 倍率注入 | `relay/helper/price.go:189-192` | 仅 `usePrice` 时把 `meta.BillingRatios` 逐个 `AddOtherRatio` |
| 预扣额度 | `relay/helper/price.go:193-199` | `PreConsumeQuotaBeforeGroup = ApplyOtherRatiosToFloat(modelPrice * QuotaPerUnit)`；再 `* GroupRatio` → `QuotaFromFloatStrict` |
| 预扣费 | `controller/relay.go:176` | `service.PreConsumeBilling`；失败/异常时 `defer` 里 `relayInfo.Billing.Refund(c)`（:182-191） |
| 响应处理 | `relay/channel/openai/adaptor.go:645-650` → `relay/channel/openai/relay_image.go:34 / :93` | 非流式 `OpenaiImageHandler`，流式 `OpenaiImageStreamHandler` |
| **实际张数覆盖** | `relay/channel/openai/relay_image.go:25-30, :52` | `updateOpenAIImageCount(info, gjson.GetBytes(responseBody,"data.#").Int())` → `AddOtherRatio("n", 实际张数)`，**覆盖**预扣时的声明 n（`types/price_data.go:52` 是 map 赋值覆盖） |
| 结算 | `relay/image_handler.go:149` → `service/text_quota.go:397` `PostTextConsumeQuota` | → `calculateTextQuotaSummary`（:231） |
| 最终额度 | `service/text_quota.go:368-376` | `quota = ModelPrice × QuotaPerUnit × GroupRatio`，再 `ApplyOtherRatiosToDecimal`（:371，把 `n` 连乘进去），`QuotaFromDecimalChecked` 饱和 |

### 1.2 关键结论

- **`gpt-image-2` 当前是按张（per-call 固定价）计费**，前提是管理员在价格表里给它配了 `model_price`。配置字段就是 `ModelPrice` 选项映射（`setting/ratio_setting/model_ratio.go:360 GetModelPrice` / `modelPriceMap`），前端在 **系统设置 → 模型定价 → 编辑模型 → "Per request" 标签页 → "Fixed price"**（`web/src/features/system-settings/models/model-pricing-sheet.tsx:551,601,609`，提示语 "Cost in USD per request, regardless of tokens used." :631）。
- 若没配 `model_price`，会退化成按 token 的 `model_ratio` 路径（`price.go:115-146`），此时 `n` 和 `ImagePriceRatio` **都不生效**（`price.go:189` 的 `if usePrice` 门禁，以及 `relay_image.go:26` 的 `!info.PriceData.UsePrice` 早退）。
- **`n` 的参与方式**：作为 `PriceData.otherRatios["n"]` 连乘。预扣用声明 n，结算用**上游实际返回张数**。
- `QuotaPerUnit = 500 * 1000.0`（`common/constants.go:22`），所以 `$0.1 → model_price 0.1 → 50000 quota`。

### 1.3 名词澄清（务必不要混用）

| 名字 | 含义 | 证据 |
|---|---|---|
| `ModelPrice` / `model_price` | **按次固定价，单位 USD/请求** | `price.go:369` `dModelPrice.Mul(dQuotaPerUnit)`；UI 文案 `model-pricing-sheet.tsx:631` |
| `ModelRatio` | 按 token 计费的倍率 | `price.go:122,141` |
| **`ImageRatio` / `image_ratio`** | **⚠️ 不是分辨率倍率**，是「输入图片 token」的倍率 | `service/text_quota.go:332-336` `dImageTokens.Mul(dImageRatio)`，`dImageTokens` 来自 `usage.PromptTokensDetails.ImageTokens`（:264）。`setting/ratio_setting/model_ratio.go:653 GetImageRatio` |
| `ImagePriceRatio`（`TokenCountMeta`） | **这才是 size×quality 的分辨率倍率**，但只 `dall-e*` 有值 | `relaykit/types/request_meta.go:29`；`relaykit/dto/openai_image.go:137-168` |
| `OtherRatios` | 附加乘数集合（`n`、`seconds`、`size`…），连乘到最终额度 | `types/price_data.go:45-105` |

---

## Q2 — 项目里已有的「分辨率 / 质量」计费设施

### 2.1 线索一：`AGENTS.md` 提到的 "resolution/quality ratios" → **确实存在，但对 gpt-image 无效**

`relaykit/dto/openai_image.go:133-171`：

```go
func (i *ImageRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var sizeRatio = 1.0
	var qualityRatio = 1.0

	if strings.HasPrefix(i.Model, "dall-e") {          // :137  ← 硬编码门禁
		if i.Size == "256x256" { sizeRatio = 0.4 }     // :139-147
		...
		if i.Model == "dall-e-3" && i.Quality == "hd" { qualityRatio = 2.0 }  // :149
	}
	...
	return &types.TokenCountMeta{
		ImagePriceRatio: sizeRatio * qualityRatio,     // :168
		BillingRatios:   map[string]float64{"n": float64(imageN)},  // :169
	}
}
```

**`gpt-image-2` 走不到 `if` 里面，`ImagePriceRatio` 恒为 `1.0`。** 分辨率维度对它完全不存在。

### 2.2 线索二：`upstream-ratio-sync-helpers.ts` 的 `Resolution*` → **假线索**

`web/src/features/system-settings/models/upstream-ratio-sync-helpers.ts:41-59`：

```ts
export type ResolutionsMap = Record<string, Record<string, number | string>>
export type ResolutionSelection = { model, ratioType, value, sourceName }
export type ResolutionRemoval = { model: string; ratioType: RatioType }
```

这里的 "resolution" 是 **"冲突解决"**：上游价格同步页面里，同一个模型在多个上游有不同倍率，管理员选哪个上游的值。`ratioType` 取值见 `:61-76`（`model_ratio` / `completion_ratio` / `cache_ratio` / `image_ratio` / `audio_ratio` / `model_price` / `billing_mode` / `billing_expr`），**没有任何图片分辨率维度**。对应后端是 `setting/ratio_setting/*` + `setting/billing_setting/tiered_billing.go` 的那几张 map，**不能用来配置按分辨率的本地价格**。

### 2.3 线索三：`PriceData.AddOtherRatio` / `OtherRatios` → **是正确的承载位置，但目前没人往里写分辨率**

`types/price_data.go:45-118`。当前全部写入点：

| 位置 | key | 含义 |
|---|---|---|
| `relay/helper/price.go:191` | 来自 `meta.BillingRatios` | 图片路径实际只有 `n` |
| `relay/channel/openai/relay_image.go:29` | `n` | 实际出图张数 |
| `relay/channel/ali/image.go:53,64,337,339` | `prompt_extend` / `n` | 阿里图片 |
| `relay/relay_task.go:130-134` | `seconds` / `size` | **视频任务**的时长/分辨率 |
| `relay/channel/task/sora/adaptor.go:122-128` | `seconds` / `size` | **Sora 的分辨率倍率**：`1792x1024`/`1024x1792` → `1.666667`，其余 `1` |

**`relay/channel/task/sora/adaptor.go:98-130` `EstimateBilling()` 就是项目里现成的「请求 size → 计费倍率」范例**，是改代码方案最该照抄的形状。但它走的是 task（异步任务）计费链路（`relay/relay_task.go:189-206`），同步图片链路没有 `EstimateBilling` 这个 hook。

安全护栏：`isValidOtherRatio`（`types/price_data.go:116-118`）拒绝 `<=0`、`+Inf`；NaN 因 `NaN > 0 == false` 也被拒。

### 2.4 明确回答：**现在能不能不写代码、纯靠后台配置实现「按分辨率不同单价」？**

- **填价格表数字（Fixed price / model_price）：不能。** 一个模型名只有一个固定价，`ImagePriceRatio` 对 gpt-image 恒为 1，没有任何后台字段能按 size 分档。
- **用 `tiered_expr` 表达式：能**（我实测通过，见 Q4），**但只能按"请求里声明的 `size`"**，且会带来一个严重副作用（Q4.4）。
- **配置路径**（表达式方案）：系统设置 → 模型定价（`web/src/features/system-settings/models/model-pricing-sheet.tsx`）→ 找到/新建 `gpt-image-2` → 切到 **Tiered / `tiered_expr`** 标签（`model-pricing-core.ts:42,220-223`）→ 用 **Raw 表达式编辑器**（`tiered-pricing-editor.tsx:875` "Raw expression editor"）粘贴表达式。存储落到 DB `options` 表的 `billing_setting.billing_mode` / `billing_setting.billing_expr`（`setting/billing_setting/tiered_billing.go:19-23`）。保存前会 `SmokeTestExpr`（:73-106）。

---

## Q3 — 分辨率信息从哪来、怎么归一化

### 3.1 请求里的 `size`

- DTO 字段：`relaykit/dto/openai_image.go:21` `Size string \`json:"size,omitempty"\``
- JSON 路径解析：`relay/helper/valid_request.go:235` `common.UnmarshalBodyReusable`
- multipart（`/v1/images/edits` form-data）路径：`relay/helper/valid_request.go:205` `imageRequest.Size = formData.Get("size")`
- 唯一的"清洗"：`:245-247` 拒绝全角乘号 `×`

### 3.2 gpt-image-2 支持哪些 size —— **仓库里查不到**

- `relay/helper/valid_request.go:254-275` 只对 `dall-e-2`/`dall-e`/`dall-e-3` 有白名单；`gpt-image-1` 分支（:271-275）**只补 `quality` 默认值，不校验 size**；`gpt-image-2` 连分支都没有。
- `relay/channel/openai/constant.go:70` 的 OpenAI 模型清单里有 `gpt-image-1`/`gpt-image-1-mini`/`gpt-image-1.5`，**没有 `gpt-image-2`**。
- 项目自带的图片 Playground 固定用 `gpt-image-2`（`web/src/features/image-playground/constants.ts:26`），它给出的 size 选项只有：
  `web/src/features/image-playground/types.ts:22`
  ```ts
  export type ImageSize = 'auto' | '1024x1024' | '1024x1536' | '1536x1024'
  ```
  `constants.ts:30-36` 同样只有这四个，默认 `'auto'`（:58）。

> **⚠️ 未找到：** 仓库里**没有任何 2k / 4k 尺寸的痕迹**。用户想要的 `2048x2048` / `4096x4096` 档位在本代码库中无据可查。是否真的支持，必须去 OpenAI 官方文档确认（本次无外部检索工具，见 Caveats）。

### 3.3 「1k / 2k / 4k」归一化 —— **没有现成函数**

- 后端**没有**任何 `WxH → 档位` 的通用归一化函数。仅有的两处都是 if 硬编码字符串等值比较：
  - `relay/channel/task/sora/adaptor.go:126` `if size == "1792x1024" || size == "1024x1792"`
  - `relay/relay_task.go:132` 同上
  - `relaykit/dto/openai_image.go:139-147` 同上
- 多宽高比归一到同档位：**无现成实现**。要做得自己解析 `WxH`（`strconv.Atoi(strings.Split(size,"x"))`），然后按 `max(w,h)` 或 `w*h` 落档。
- `'auto'` 是合法且默认的 size 取值（`constants.ts:58`），此时请求里**根本没有分辨率信息**。

### 3.4 安全：`size` 校验现状 = **无**

- `gpt-image-*` 走 `relay/helper/valid_request.go:234-283` 的 `default` 分支，`size` 除了全角乘号检查外**完全放行**。任意字符串（`"999999x999999"`、`"4096"`、`""`）都能进来。
- 今天**无害**，因为 `ImagePriceRatio` 对 gpt-image 恒为 1，`size` 不参与任何乘法。
- **一旦让 size 决定单价，就直接违反 AGENTS.md**「每个成为计费乘数的用户可控量必须在请求校验阶段设界并拒绝越界值」。
- 对比：`n` 是有界的（`dto.MaxImageN=128`，`valid_request.go:199,249`）；`sora` 的 size 是白名单的（`relay/common/relay_utils.go:255-260`，非法 size 直接 400 `invalid_size`）。**`sora` 那段就是 size 白名单该有的样子。**

---

## Q4 — `billingexpr` 能不能表达这个需求

### 4.1 能访问 size 吗 —— **能，通过 `param("size")`**

- `pkg/billingexpr/run.go:94-104`：`param(path)` = `gjson.GetBytes(request.Body, path)`，读的是**入站请求原始 body**。
- body 来源：`relay/helper/billing_expr_request.go:53-62` `readIncomingBillingExprBody`。
- **⚠️ 限制：`:54` 只在 `Content-Type: application/json` 时返回 body**，否则返回 `nil`。所以 **`/v1/images/edits` 的 multipart 请求，`param("size")` / `param("n")` 全是 `nil`**（项目自己的 Playground 编辑图就是 multipart：`web/src/features/image-playground/lib/api.ts:204-221`）。
- 预扣时冻结 `info.BillingRequestInput`（`relay/helper/price.go:352`），结算时复用同一份（`service/tiered_settle.go:208-211`），所以预扣/结算看到的是同一个 body，不会漂移。

### 4.2 阶梯怎么写 + **实测验证**

额度换算：`pkg/billingexpr/settle.go:11` `quota = exprOutput / 1e6 * QuotaPerUnit`，而固定价是 `quota = price * QuotaPerUnit`。
⇒ **`exprOutput = 目标美元价 × 1_000_000`**。所以 `$0.1 → 100000`、`$0.15 → 150000`、`$0.2 → 200000`。

针对本需求的表达式（**已在本机 `go test` 实跑验证**）：

```
has(param("size"), "4096") ? tier("4k", 200000) : (has(param("size"), "2048") ? tier("2k", 150000) : tier("1k", 100000))
```

带张数：

```
(has(param("size"), "4096") ? tier("4k", 200000) : (has(param("size"), "2048") ? tier("2k", 150000) : tier("1k", 100000))) * float(param("n") ?? 1)
```

实测输出（`RunExprWithRequest`，`TokenParams{P:100,C:100,Len:100}`）：

| body | 表达式1 out / tier | 表达式2 out / tier |
|---|---|---|
| `{"size":"4096x4096","n":3}` | `200000` / `4k` | `600000` / `4k` |
| `{"size":"auto"}` | `100000` / `1k` | `100000` / `1k` |
| `nil`（multipart） | `100000` / `1k` | `100000` / `1k` |

编译无错，`param()` 的 `interface{}` 返回值配合 `has()` / `float()` / `??` 都工作正常。`SmokeTestExpr`（`setting/billing_setting/tiered_billing.go:77-106`）的向量都不含 `size`/`n`，常量表达式必然通过。

### 4.3 表达式 vs 固定价的优先级

`relay/helper/price.go:99-101`：

```go
if billing_setting.GetBillingMode(info.OriginModelName) == billing_setting.BillingModeTieredExpr {
	return modelPriceHelperTiered(c, info, promptTokens, meta, groupRatioInfo)
}
```

**`tiered_expr` 优先级最高，直接 return**，`model_price` / `model_ratio` 全部被绕过。

### 4.4 ⚠️ **致命副作用：走 `tiered_expr` 会丢掉「按实际出图张数计费」**

`modelPriceHelperTiered`（`price.go:296-365`）构造的 `PriceData` 只有 4 个字段（:354-359），**`UsePrice` 是零值 `false`，`meta.BillingRatios` / `meta.ImagePriceRatio` 被完全忽略**。后果：

1. `relay/channel/openai/relay_image.go:26` `if ... !info.PriceData.UsePrice { return }` → **`updateOpenAIImageCount` 直接早退，永远不写 `n`**。
2. `service/tiered_settle.go:213` `ComputeTieredQuotaWithRequest` 只用表达式结果，`composeTieredTextQuota`（`service/text_quota.go:203-226`）除了工具调用附加费外**不碰 `OtherRatios`**。

⇒ **tiered 模式下，`n` 只能来自表达式里的 `param("n")`，也就是用户请求里声明的张数。上游实际只出 2 张，用户声明 4 张，照样按 4 张收费。** 这是从现状（按实际出图）的**明确回退**，而且方向对用户不利。

---

## Q6 — 上游响应里能不能拿到实际图片分辨率

### 6.1 响应 DTO

`relaykit/dto/openai_image.go:183-192`：

```go
type ImageResponse struct {
	Data     []ImageData     `json:"data"`
	Created  int64           `json:"created"`
	Metadata json.RawMessage `json:"metadata,omitempty"`
}
type ImageData struct {
	Url           string `json:"url"`
	B64Json       string `json:"b64_json"`
	RevisedPrompt string `json:"revised_prompt"`
}
```

**没有 width / height / size 字段。**

但注意：**OpenAI 图片非流式路径压根不反序列化成 `ImageResponse`**。`OpenaiImageHandler`（`relay/channel/openai/relay_image.go:34-60`）只解 `dto.SimpleResponse{Usage; Error}`（`relaykit/dto/openai_response.go:15-18`），然后 `gjson` 数 `data.#`，原样透传 body（:55）。`openaiImageJSONAsStreamHandler` 也显式注释说不反序列化 data[]（:245-247，理由是 b64 太大）。

### 6.2 `b64_json` 还是 `url`

- 两者都支持，**由上游决定，项目不做转换**：非流式是 `IOCopyBytesGracefully` 原样透传（:55）；JSON→SSE 转换时对 `url` / `revised_prompt` / `b64_json` 三个字段都做 passthrough（:297）。
- 请求侧有 `ResponseFormat string \`json:"response_format,omitempty"\``（`relaykit/dto/openai_image.go:23`），原样转发给上游。其他渠道会读它（`relay/channel/replicate/adaptor.go:250`、`relay/channel/ali/dto.go:112,140`），**OpenAI 渠道不读**。
- 流式帧里两者都可能出现：`relay/helper/stream_gate.go:152` 判定内容帧时检查 `"b64_json", "partial_image_b64", "url", "result"`。
- **未找到**：仓库里没有任何「gpt-image 系列只返回 b64_json」的断言或依据。

### 6.3 **计费时后端是否还持有完整图片字节 —— 持有**

| 路径 | 证据 | 结论 |
|---|---|---|
| 非流式 `OpenaiImageHandler` | `relay_image.go:37` `responseBody, err := io.ReadAll(resp.Body)`；`:52` 数张数；`:55` 才写给客户端 | **`:52` 那一行手里有完整 body（含全部 b64）**，可直接解析尺寸 |
| JSON→SSE `openaiImageJSONAsStreamHandler` | `:240` `io.ReadAll`；`:258-259` 数张数 | 同上 |
| 真流式 `OpenaiImageStreamHandler` | `:116-140` 回调里 `raw := common.StringToByteSlice(data)`，`:133` 识别 `image_generation.completed` / `image_edit.completed` | **每个完成帧的 raw JSON 在回调内可得**，其中含该张图的 `b64_json` |

⚠️ 但 `responseBody` 是**函数局部变量**，没有挂到 `RelayInfo` 上。计费真正发生在 `relay/image_handler.go:149 PostTextConsumeQuota`，那时已经出了 handler。**所以尺寸必须在 handler 内部就解析出来并写进 `info.PriceData`**（和现有 `updateOpenAIImageCount` 完全一样的时机和形状）。

### 6.4 后端有没有解尺寸的能力 —— **有，现成的**

`service/file_service.go:464-492`：

```go
func decodeImageConfig(data []byte) (image.Config, string, error) {
	config, format, err := image.DecodeConfig(bytes.NewReader(data))  // :468  PNG/JPEG
	...
	config, err = webp.DecodeConfig(reader)                            // :474  WebP
	...
	if heifMime := detectHEIF(data); heifMime != "" { ... }            // :480-489  HEIC/HEIF
}
```

导入在 `service/file_service.go:8-11`（`image`, `_ image/jpeg`, `_ image/png`）。`image.DecodeConfig` **只读文件头，不解码像素**，成本 O(1)、几十字节，无网络 IO。已有导出封装 `service.GetImageConfig`（:387）但它接受 `types.FileSource` 会触发下载，**不适合计费路径**；应该导出/复用 `decodeImageConfig(data []byte)`。

前端等价物：`web/src/features/image-playground/lib/mask-preprocess.ts:97-109` `readPngDimensions`（纯 PNG 头 offset 16/20 读 uint32）。

成本评估：`b64_json` 是 base64 字符串，只需 decode **前 ~64 字节**（PNG 的 IHDR 在前 24 字节，JPEG SOF 稍靠后）就能拿宽高，不必 decode 整张图。

### 6.5 `url` 分支 —— **确认走不通**

响应只有 URL 字符串，后端计费路径上没有图片字节。要拿尺寸只能发 HTTP 下载：
- 引入网络 IO + 超时 + SSRF 面（项目有 `common/ssrf_protection.go`、`service/protected_fetch_client.go` 说明这是被严肃对待的攻击面）
- 引入新的失败模式：下载失败时该按哪档收费？没有正确答案
- **结论：不可接受。** `url` 分支只能退回到「按请求 `size` 判档」或「按最低档兜底」。

### 6.6 上游元数据里有没有尺寸 —— **仓库内未找到任何证据**

- 全仓搜索：没有任何 Go 代码从图片响应里读 `size` / `width` / `height` / `output_format`（`grep "\"size\"" relay/ service/` 命中的全是**请求侧**或 task 侧）。
- 测试固件里的完成帧只有 `{"type":"image_generation.completed","b64_json":"first"}` 和 `usage`（`relay/channel/openai/image_stream_test.go:120-129`）。
- `dto.Usage` / `InputTokensDetails`（`relay/channel/openai/relay_image.go:80-87` 映射的那些字段）里**没有尺寸信息**。
- **未找到** = 项目从未消费过任何尺寸元数据。上游真实响应是否回显 `size` 字段**必须查 OpenAI 官方文档确认**（本次无法外部检索）。如果确实回显，那是**最理想的来源**（一个 gjson 读取就够，无需解码图片）。

---

## Q7 — 流式出图的计费点与数据可得性

三条流式路径：

| 路径 | 触发条件 | 计费点 | 那时能否知道尺寸 |
|---|---|---|---|
| 真 SSE | `Content-Type: text/event-stream`（`relay_image.go:103`） | `:159-169`：流结束后按 `completedImages` 调 `updateOpenAIImageCount` | **能**——`:133` 识别完成帧时，回调参数 `data`/`raw` 里就有该帧的 `b64_json`（`stream_gate.go:152` 证实完成帧带 `b64_json`/`url`） |
| 上游返 JSON 但客户端要流 | `:104` `openaiImageJSONAsStreamHandler` | `:258-259` | **能**——`:240` 已 `io.ReadAll` 全量 body |
| 上游返非 200 | `:100-102` 回落到 `OpenaiImageHandler` | — | 错误路径，不计费 |

**流式的计费时机反而更适合逐张取尺寸**：每张图在 `:133` 被单独识别一次，天然可以逐张解析尺寸并累加档位系数。

流式的防滥用逻辑（`:152-169`）值得注意：只有上游正常结束（`Done`/`EOF`）或 `completedImages > requestedN` 时才用实际计数，否则保留声明 n——防止客户端在第一张完成后立即断开来少付钱。**按实际出图做分辨率计费时必须沿用同样的 abort 守卫**，否则客户端断连可以把 4k 的账单压成 1k。

---

## Q8 — 部分成功 / 失败的计费语义

### 8.1 `n=4` 上游只返 2 张 → **按 2 张收费**

`relay/channel/openai/relay_image.go:52` 用 `gjson.GetBytes(responseBody,"data.#")`（实际数组长度）调 `updateOpenAIImageCount`，`types/price_data.go:52` 是 map 覆盖赋值，直接把预扣时的 `n=4` 覆盖成 `2`。流式同理（:159-169，带 abort 守卫）。

**所以「按实际出图张数计费」是现状，已经实现了。** 用户的诉求本质上是把这个原则从「张数」扩展到「分辨率」。

### 8.2 上游错误 → **完全不计费，预扣全额退回**

`controller/relay.go:182-191`：

```go
defer func() {
	if newAPIError != nil {
		newAPIError = service.NormalizeViolationFeeError(newAPIError)
		if relayInfo.Billing != nil { relayInfo.Billing.Refund(c) }
		service.ChargeViolationFeeIfNeeded(c, relayInfo, newAPIError)
	}
}()
```

另外 `service/text_quota.go:443-449`：`!summary.hasBillableUsage()` 时不更新用量，`SettleBilling(..., summary.Quota)` 照常执行差额结算。

### 8.3 **「逐张不同单价」现有结构能不能表达 —— 能，但要用聚合系数**

结构本身是**均一模型**：
```
quota = ModelPrice × QuotaPerUnit × GroupRatio × Π(otherRatios)     // service/text_quota.go:369-371
```
`PostTextConsumeQuota(ctx, relayInfo, usage, extraContent)`（`service/text_quota.go:397`）的入参里**没有任何逐张结构**；`extraContent []string` 纯日志文本。

**但不需要改结构**。把 `ModelPrice` 设为基准档（1k = $0.1），然后：

```
otherRatios["n"]          = 1           // 中和掉张数维度
otherRatios["resolution"] = Σ_i (price_i / 0.1)
```

例：一次返回 1 张 1k + 2 张 4k → `resolution = 1 + 2 + 2 = 5` → `0.1 × 5 = $0.5`。**精确，无降级。**

约束核查：`isValidOtherRatio`（`types/price_data.go:116-118`）要求 `> 0` 且非 `+Inf` —— 聚合系数天然 ≥ 1，满足。上界由 `MaxImageN=128` × 最高档系数（2）= 256 封顶，远离 int32 边界。

⇒ **不需要「取最大档」或「按第一张」这种降级方案。** 但代价是账单可读性下降（日志里得额外写清每张的档位，`relay/image_handler.go:137-147` 的 `logContent` 是现成的挂点）。

> 反过来说：**`tiered_expr` 表达式路线做不到这一点**——`ComputeTieredQuotaWithRequest` 只吃 `TokenParams` + 冻结的请求 body，够不着响应（Q4.4）。

---

## Q9 — 两种计费口径的安全性对比

### 9.1 按请求 `size` 计费（用户可控输入）

| 风险 | 现状 | 必须补的防护 |
|---|---|---|
| 伪造超大 size 骗低价/高价 | `size` **零校验**（Q3.4） | **白名单**，照抄 `relay/common/relay_utils.go:255-260` 的 sora 形状：非白名单 → 400 |
| `size` 缺省 / `'auto'` | `'auto'` 是默认值（`constants.ts:58`） | 必须显式定义 `auto` 落哪档（保守应落**最高档**，否则用户一律填 auto 来拿 1k 价） |
| 全角乘号 | 已有（`valid_request.go:245`） | — |
| multipart 绕过 | `valid_request.go:205` 从 form 取 size，**同样零校验**；且 `param()` 在 multipart 下返回 `nil`（`billing_expr_request.go:54`） | AGENTS.md 点名的「验证绕过路径」，multipart 分支必须施加**同一套**白名单 |
| 额度换算溢出 | `QuotaFromDecimalChecked`（`text_quota.go:365,373`）已饱和 + 审计 | — |

**用户可以少付钱**：声明 `size:"1024x1024"` 但上游因 `quality`/模型默认实际出了更大的图 —— 这正是用户提出「按实际出图」的动机。

### 9.2 按实际出图计费（上游可控输入）

AGENTS.md 原文：「上游返回的数值同样不可信（举例 Kling 的 `FinalUnitDeduction`）」。解码出来的 `image.Config{Width,Height}` 来自**攻击者可构造的图片头**（恶意/被入侵的上游渠道可以返回一个声明 `width=2^31` 的 4 字节 PNG 头）。

必须的防护：

1. **档位映射必须是"落桶"而非"按像素线性计算"**：`max(w,h) <= 1536 → 1k`、`<= 2560 → 2k`、`<= 4608 → 4k`、`> 4608 → 落最高档并告警`。**绝不能写 `ratio = w*h / (1024*1024)` 这种线性式**——那等于把上游数字直接变成乘数。
2. **解码失败 / 非图片 / `url` 分支 → 落到请求声明的 `size` 档；再不行落最低档**，并 `logger.LogWarn` 留痕。绝不能因为解不出尺寸就跳过计费。
3. **聚合系数的上界**：`Σ 系数 ≤ MaxImageN × 最高档系数`，用 `common.QuotaFromDecimalChecked` / `QuotaFromFloatChecked` 的 `*Checked` 变体承接，把 `*common.QuotaClamp` 挂到 `relayInfo.QuotaClamp`（`service/text_quota.go:194-201 noteQuotaClamp`），由 `attachQuotaSaturation`（`service/log_info_generate.go`）写进 `other.admin_info.quota_saturation`。
4. **沿用流式 abort 守卫**（`relay_image.go:152-169`）：客户端提前断开时不许把已产生的高档账单降级。
5. **解码字节上界**：只 base64-decode 前 N 字节（如 64B）再喂 `image.DecodeConfig`，避免 128 张 4k 图全量解码造成 CPU/内存放大。

### 9.3 推荐

**推荐「按实际出图」，以「请求 `size` 白名单」作为 fallback 和第二道闸。** 理由：

- 用户诉求明确；
- 项目在张数维度**已经是**按实际返回计费（Q8.1），分辨率跟上才一致；
- `'auto'` 是默认 size，按请求口径根本判不了档（Q3.2）；
- 数据可得性已验证（Q6.3/6.4），成本是读 64 字节文件头。

**退路**（若 OpenAI 的 gpt-image-2 实际走 `url` 返回，或解码率低）：退回按请求 `size` 判档 + 强制白名单 + `auto` 落最高档。

---

## Q5 — 方案对比与推荐

### 方案 A：纯配置 —— `tiered_expr` 表达式（按请求声明的 `size`）

- **改动面**：0 行代码。后台粘一个表达式。
- **配置**：系统设置 → 模型定价 → `gpt-image-2` → Tiered 模式 → Raw 编辑器：
  ```
  (has(param("size"), "4096") ? tier("4k", 200000) : (has(param("size"), "2048") ? tier("2k", 150000) : tier("1k", 100000))) * float(param("n") ?? 1)
  ```
- **触及计费核心**：否（表达式是既有机制）。
- **风险（高）**：
  1. **丢掉按实际出图张数计费**（Q4.4）——上游少出图照样按声明 n 收；
  2. **`size` 无白名单**，`"4096"` 这个子串匹配可被 `"14096x1"` 之类字符串骗；
  3. **multipart（`/v1/images/edits`）全线失效** —— `param()` 返回 nil，一律按 1k×1 收费（Q4.1）。项目自带 Playground 的编辑图就走这条路；
  4. `'auto'` 落 1k 档；
  5. 完全不按实际出图。
- **是否需要新增校验**：**需要**。没有白名单就是在违反 AGENTS.md。而且加白名单本身就要改 `valid_request.go` —— 所以「纯配置」的前提并不成立。

### 方案 B：改代码 —— 按**请求 `size`** 加分辨率倍率（最小改动）

三处改动：

1. `relay/helper/valid_request.go` —— 给 `gpt-image-*` 加 `size` 白名单（JSON 分支 :234-283 + multipart 分支 :204-205 两处都要），照抄 `relay/common/relay_utils.go:255-260` 的形状，非法 → 400。
2. `relaykit/dto/openai_image.go:133-171` —— 在 `GetTokenCountMeta()` 里给 gpt-image 分支填 `ImagePriceRatio`：1k→1.0、2k→1.5、4k→2.0。
3. 后台把 `gpt-image-2` 的 Fixed price 配成 **0.1**（基准 = 1k 单价）。

- **改动面**：~40 行 + 表驱动。
- **触及计费核心**：否。完全复用既有的 `ImagePriceRatio` 通道（`price.go:148-150`）和 `n` 通道，**保留按实际出图张数计费**。
- **风险**：低。`'auto'` 需要显式决策（建议落最高档）。仍是按声明 size 而非实际出图。
- **需要新增校验**：**是**（第 1 条就是）。

### 方案 C：改代码 —— 按**实际出图**的分辨率（用户的真实诉求）

在方案 B 基础上追加：

4. 在 `service/` 导出 `decodeImageConfig`（`service/file_service.go:465`）或加一个 `service.ImageDimensionsFromBytes(data []byte) (w, h int, ok bool)`。
5. `relay/channel/openai/relay_image.go` —— 把 `updateOpenAIImageCount`（:25-30）升级成 `updateOpenAIImageBilling(info, responseBody)`：遍历 `data.#`，对每张 base64-decode 前 64 字节 → 落桶 → 累加系数 → `AddOtherRatio("n", 1)` + `AddOtherRatio("resolution", Σ系数)`。三个入口都要调：`:52`、`:167`（流式，逐帧累加）、`:259`。
6. 保留请求 `size` 档位作为 **fallback**（url 分支 / 解码失败 / passthrough 模式）。
7. `relay/image_handler.go:137-147` 的 `logContent` 里补每张实际尺寸，让账单可溯源。
8. 饱和与审计按 Q9.2 走 `*Checked` + `noteQuotaClamp`。

- **改动面**：~120 行 + 测试。
- **触及计费核心**：**部分触及** —— 新增一个 `OtherRatio` 维度，但**不改 `PriceData` 结构、不改 `text_quota.go` 的计算式**，量级与 sora 的 `EstimateBilling` 相当。
- **风险**：中。新增上游可控输入路径，需 Q9.2 的全套防护。流式逐帧累加需要仔细处理 abort 守卫。
- **需要新增校验**：**是**（请求侧白名单 + 响应侧落桶上界，两道）。

### 推荐

**做 B，然后做 C（C 依赖 B 的白名单和档位表）。**

理由：
- **方案 A 不推荐**。它看起来「零改动」，但为了合规仍然必须改 `valid_request.go` 加白名单，所以省不下改动；而代价是丢掉按实际出图张数计费 + multipart 全线失效，是明确的功能倒退。
- **B 是安全的地基**，本身就能上线（用户拿到按分辨率差价），且保留现有一切行为。
- **C 才是用户真正要的**，且已验证可行（字节在手、解码器现成、`OtherRatio` 聚合系数能精确表达逐张不同单价）。
- 唯一的硬卡点是 **Q3.2：仓库里查不到 gpt-image-2 到底支不支持 2k/4k**。**动手前必须先确认真实支持的 size 取值集合**，否则白名单和档位表都是空中楼阁。

---

## Caveats / Not Found

1. **无外部检索工具** —— 本 session 的可用工具只有 Read / Write / Bash / Skill，`mcp__exa__*` / WebSearch / context7 均不可调用。因此**未能核实** OpenAI 官方对 `gpt-image-2` 的：(a) 支持的 `size` 取值集合；(b) 响应体是否回显 `size` / `output_format` 等元数据；(c) 是否返回 `url`。这三项直接决定方案 C 的最优实现路径，**必须在动手前由人工或联网 agent 确认**。
2. **仓库内无 2k / 4k 痕迹** —— `gpt-image-2` 在项目里唯一的"权威"尺寸定义是前端 Playground 的 `'auto' | '1024x1024' | '1024x1536' | '1536x1024'`（`web/src/features/image-playground/types.ts:22`）。用户想要的 2k/4k 档位在代码库中无据可查。
3. **`gpt-image-2` 无任何后端特判** —— 不在 `relay/channel/openai/constant.go:59-76` 的模型清单里，不在任何默认价格表里，`relay/helper/valid_request.go` 没有它的分支。它完全依赖管理员在后台配置价格。
4. **未查证**：`PassThroughRequestEnabled` / `ChannelSetting.PassThroughBodyEnabled` 开启时（`relay/image_handler.go:49`）请求体绕过 DTO 转换，但 `GetAndValidateRequest` 仍然先跑过，所以 `GetTokenCountMeta()` 的 size 解析应该仍有效；**此路径未做端到端验证**。
5. **未查证**：非 OpenAI 渠道（xai `relay/channel/xai/adaptor.go:117` 复用 `OpenaiImageHandler`；ali / zhipu / replicate / minimax 有各自的 image handler）在分辨率计费下的行为。本次只覆盖 OpenAI 渠道路径。

---
---

# 第二轮追加：用「出图 token 数」计费的可行性

- **Query**: 上一轮结论「上游响应没有尺寸元数据」查的是 `ImageData`（`data[]` 元素），**响应顶层 `usage` 未查**。本轮验证能否用 output_tokens 代替解图片头
- **Scope**: internal（仍无联网工具，不报告任何 OpenAI 官方文档内容，只报告仓库内可证实的）
- **Date**: 2026-09-11
- **纪律**: 只读。未修改任何生产文件。未执行任何 git 写操作。未触碰 `web/src/features/group-monitoring/**`、`channel-monitoring/**`、`router/`

## TL;DR（本轮推翻上一轮的关键结论）

| 问题 | 结论 |
|---|---|
| 图像响应顶层有没有 `usage` | **有，而且早就在解析、早就在用**。`relay/channel/openai/relay_image.go:42-43` 用 `dto.SimpleResponse` 解顶层 `usage`；`:57` `normalizeOpenAIUsage` 把 `output_tokens` 映射到 `CompletionTokens` |
| 能不能拿到 `output_tokens` | **能。非流式、JSON→SSE、真 SSE 三条路径全都能拿**，且**与 `url` / `b64_json` / multipart 无关**——它在响应顶层，不在 `data[]` 里 |
| 上一轮「无尺寸元数据」是否仍成立 | 成立，但**已不重要**：不需要尺寸，token 数就是分辨率×质量的函数 |
| 现有计费链路能不能直接吃 | **能，而且是零代码**。`gpt-image-1` 在默认价格表里**本来就是按 token 计费**（`setting/ratio_setting/model_ratio.go:59,332,639`） |
| 推荐 | **方案 C（直接按 token 计价）**。零代码、全路径覆盖、与上游成本严格对应。B（token 落桶）比 A 好但有 `n` 归一化的硬伤 |

---

## Q1 — 图像响应顶层 `usage`：**存在且已被消费**

### 1.1 DTO 定义分两层，上一轮查错了层

| 结构 | 位置 | 有没有 usage |
|---|---|---|
| `dto.ImageResponse` | `relaykit/dto/openai_image.go:183-187` | **没有**（只有 `Data` / `Created` / `Metadata`） |
| `dto.ImageData` | `relaykit/dto/openai_image.go:188-192` | 没有（上一轮查的就是这个） |
| **`dto.SimpleResponse`** | `relaykit/dto/openai_response.go:15-18` | **有** —— `Usage \`json:"usage"\`` 内嵌 + `Error any` |

**关键：OpenAI 图片路径根本不用 `dto.ImageResponse`。** 它只用 `dto.SimpleResponse`：

```go
// relay/channel/openai/relay_image.go:42-46
var usageResp dto.SimpleResponse
err = common.Unmarshal(responseBody, &usageResp)
```

`dto.ImageResponse` 的全部使用者都是**其他渠道在造 OpenAI 兼容响应**（gemini `relay-gemini.go:457`、minimax `image.go:157`、jimeng `image.go:33`、replicate `adaptor.go:252`、ali `image.go:272`），OpenAI 渠道从不用它。所以「`ImageResponse` 没有 usage 字段」对 OpenAI 计费路径**无影响**。

### 1.2 `dto.Usage` 覆盖了哪些图像 usage 子字段

`relaykit/dto/openai_response.go:223-244`：

```go
type Usage struct {
	PromptTokens           int                `json:"prompt_tokens"`
	CompletionTokens       int                `json:"completion_tokens"`
	TotalTokens            int                `json:"total_tokens"`
	PromptTokensDetails    InputTokenDetails  `json:"prompt_tokens_details"`
	CompletionTokenDetails OutputTokenDetails `json:"completion_tokens_details"`
	InputTokens            int                `json:"input_tokens"`      // :234
	OutputTokens           int                `json:"output_tokens"`     // :235
	InputTokensDetails     *InputTokenDetails `json:"input_tokens_details"` // :236
	...
}
```

`InputTokenDetails`（:256-267）含 `cached_tokens` / `text_tokens` / `audio_tokens` / `image_tokens`。

> **⚠️ 缺口：没有 `output_tokens_details`。** `CompletionTokenDetails` 的 tag 是 `completion_tokens_details`（:233），不是 `output_tokens_details`。所以若上游在图像响应里回 `usage.output_tokens_details.image_tokens`，**当前解不出来**，`img_o` 表达式变量恒为 0（见 Q3.4）。用 `c`（= `output_tokens`）不受影响。

### 1.3 归一化：`output_tokens → CompletionTokens`

`relay/channel/openai/relay_image.go:70-91` `normalizeOpenAIUsage`：

```go
if usage.InputTokens != 0  { usage.PromptTokens = usage.InputTokens }        // :74-76
if usage.OutputTokens != 0 { usage.CompletionTokens = usage.OutputTokens }   // :77-79
if usage.InputTokensDetails != nil { ... ImageTokens / TextTokens / AudioTokens ... } // :80-87
if usage.TotalTokens == 0 { usage.TotalTokens = Prompt + Completion }        // :88-90
```

函数头注释（:62-69）明确写着这是**专为 OpenAI Images 路径**（generations/edits，流式与非流式）设计的。

### 1.4 仓库内的响应测试样本 —— **证明项目可解析该 usage 形状**

| 形态 | 样本 | 位置 |
|---|---|---|
| 非流式 JSON body | `{"created":1710000000,"data":[{"b64_json":"first",...},{"b64_json":"second"}],"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"image_tokens":2,"text_tokens":1}}}` | `relay/channel/openai/image_stream_test.go:292` |
| SSE 末帧（usage-only） | `data: {"usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7,"input_tokens_details":{"image_tokens":2,"text_tokens":1}}}` | `relay/channel/openai/image_stream_test.go:86` |
| SSE 完成帧内嵌 usage | `data: {"type":"image_edit.completed","b64_json":"second","usage":{"input_tokens":3,"output_tokens":4,"total_tokens":7}}` | `relay/channel/openai/image_stream_test.go:125` |

断言直接检查 `usage.CompletionTokens == 4`（:99、:301）。这些固件是项目自己维护的图像响应形状契约。

> **注意口径**：这是「仓库里能证实项目**按此形状解析**」，不是「OpenAI 官方保证一定返回」。上游是否总回 usage 仍需实测（见 Q5 退路）。

### 1.5 「原始 body 还在手里能不能 gjson 读 `usage.output_tokens`」

**能，但没必要。** `relay_image.go:37` 已 `io.ReadAll` 全量 body，`:43` 已经 `common.Unmarshal` 成结构体了——`gjson` 是多余的第二次解析。上一轮说的「只用 gjson 数 `data.#`」只描述了**张数**那一条线（:52），usage 走的是另一条线（:42-59），两条并存。

---

## Q2 — 流式出图的 usage：**三条流式路径全部能拿**

### 2.1 真 SSE：`OpenaiImageStreamHandler`

`relay/channel/openai/relay_image.go:116-140` 回调里，**每一帧都尝试解 usage**：

```go
var chunk struct {
	Type  string    `json:"type"`
	Usage dto.Usage `json:"usage"`
}
if err := common.Unmarshal(raw, &chunk); err == nil {
	normalizeOpenAIUsage(&chunk.Usage)
	if service.ValidUsage(&chunk.Usage) {   // :130
		usage = &chunk.Usage                 // 后到的有效 usage 覆盖先到的
	}
	if chunk.Type == "image_generation.completed" || chunk.Type == "image_edit.completed" {
		completedImages++                    // :134
	}
}
```

- `service.ValidUsage`（`service/usage_helpr.go:31-33`）= `PromptTokens != 0 || CompletionTokens != 0`。
- **usage 帧不必是完成帧**：`:86` 的固件就是一个只有 `usage` 没有 `type` 的裸帧，照样被收下（测试 :98-102 断言通过）。
- **完成帧里内嵌 usage 也收**：`:125` 固件。
- 覆盖语义是「最后一个有效 usage 获胜」，符合 OpenAI 在流末给 usage 的习惯。

### 2.2 stream gate 不会吞掉 usage 帧

`relay/helper/stream_gate.go:147-154` 的 `StreamProtocolOpenAIImage` 分支只把带 `b64_json`/`partial_image_b64`/`url`/`result` 的 `image_*.` 帧判为 `streamFrameContent`；usage-only 帧落到 `streamFrameNeutral`（:156）。**gate 是 precommit 缓冲判定，不是过滤器**——所有帧最终都会进 `relay_image.go:116` 的回调。`image_stream_test.go:73-109` 这个测试就是证据：流里只有一个 partial_image 内容帧 + 一个 usage 裸帧，usage 仍被正确提取。

### 2.3 JSON→SSE 伪流：`openaiImageJSONAsStreamHandler`

`relay/channel/openai/relay_image.go:248-256`：同样 `dto.SimpleResponse` 解 + `normalizeOpenAIUsage`。而且 `:288-293` 还会把 usage **塞进每一个伪造的 completed 帧**转发给客户端。测试 `image_stream_test.go:287-314` 断言 `usage.CompletionTokens == 4`。

### 2.4 非 200 回落

`relay_image.go:100-102` 回落到 `OpenaiImageHandler` —— 错误路径，不计费（上一轮 Q8.2 已证）。

### 2.5 覆盖度总表

| 请求形态 | 响应形态 | 走哪个 handler | 拿得到 output_tokens |
|---|---|---|---|
| JSON generations | JSON | `OpenaiImageHandler` :34 | ✅ :43 |
| JSON generations, `stream:true` | SSE | `OpenaiImageStreamHandler` :93 | ✅ :128 |
| JSON generations, `stream:true` | 上游回 JSON | `openaiImageJSONAsStreamHandler` :237 | ✅ :249 |
| **multipart `/v1/images/edits`** | 任意 | 同上（`adaptor.go:645-650` 按 `RelayMode` 分派，`RelayModeImagesEdits` 与 generations 共用 handler） | ✅ **请求编码不影响响应解析** |
| 上游回 `url` 而非 `b64_json` | — | 同上 | ✅ **usage 在顶层，与 data[] 内容无关** |

**这正是 token 方案相对解图片头方案的决定性优势**：上一轮 Q6.5 判定「`url` 分支走不通」、Q4.1 判定「multipart 下 `param()` 返回 nil 全线失效」，**这两个硬伤在 token 方案下都不存在**。

---

## Q3 — 现有计费链路能否直接吃 token

### 3.1 图像路径当前传给 `PostTextConsumeQuota` 的 usage 是**真实的**

`relay/image_handler.go:113` `usage, newAPIError := adaptor.DoResponse(...)` → 上面那个真实 usage；`:149` `service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), logContent)`。

中间只有两处**兜底下限**（`relay/image_handler.go:125-130`）：

```go
if usage.(*dto.Usage).TotalTokens == 0  { usage.(*dto.Usage).TotalTokens = 1 }
if usage.(*dto.Usage).PromptTokens == 0 { usage.(*dto.Usage).PromptTokens = 1 }
```

**`CompletionTokens` 不被改写**。所以真实 `output_tokens` 原封不动进入结算。这是「伪造」的唯一成分，且只在上游完全没回 usage 时触发（见 Q5 退路）。

### 3.2 按 token 计费**今天就能跑，零代码** —— `gpt-image-1` 就是现成样板

`setting/ratio_setting/model_ratio.go`：

| 配置项 | 行 | 值 | 含义（`1 === $0.002/1K tokens`，:23） |
|---|---|---|---|
| `defaultModelRatio["gpt-image-1"]` | :59 | `2.5` | `$5 / 1M` 输入 token（注释就这么写的） |
| `defaultCompletionRatio["gpt-image-1"]` | :332 | `8` | 输出 = 输入×8 → **`$40 / 1M` 输出 token** |
| `defaultImageRatio["gpt-image-1"]` | :639 | `2` | 输入**图片** token → `$10 / 1M` |
| `defaultModelPrice`（:272-297） | — | **无 `gpt-image-1` 条目** | ⇒ `usePrice=false` ⇒ 走 token 路径 |

换算公式（`common/constants.go:22` `QuotaPerUnit = 500*1000 // $0.002/1K`；`model_ratio.go:14` `USD = 500`）：

```
USD per 1M tokens = model_ratio × 2
```

所以目标「输出 $30/M」⇒ `model_ratio × completion_ratio = 15`。例如 `model_ratio = 2.5`（输入 $5/M）、`completion_ratio = 6`（输出 $30/M）。

**结论：给 `gpt-image-2` 在后台配 model_ratio / completion_ratio / image_ratio，且不配 Fixed price，就完成了按 token 计费。改动 0 行代码。**

### 3.3 token 路径的完整计算式

`service/text_quota.go:304-367`（`!UsePrice` 分支）：

```
promptQuota     = (p - cached - cacheCreate - imageTokens) + cached*cacheRatio
                  + imageTokens*imageRatio + cacheCreate*cacheCreationRatio   // :355
completionQuota = CompletionTokens × completionRatio                          // :356
quota = (promptQuota + completionQuota) × modelRatio × groupRatio             // :357,300
quota = ApplyOtherRatiosToDecimal(quota)                                      // :359
quota = QuotaFromDecimalChecked(quota)                                        // :365  ← 饱和+审计
```

- `CompletionTokens` 就是 `output_tokens`（Q1.3）。
- `imageTokens` 来自 `usage.PromptTokensDetails.ImageTokens` ← `input_tokens_details.image_tokens`（`relay_image.go:84`）。**输入参考图自动按 image_ratio 单独计价**，这是 `/v1/images/edits` 的正确行为。
- **`n` 不参与**：`updateOpenAIImageCount`（`relay_image.go:25-30`）在 `!info.PriceData.UsePrice` 时早退。**但这不是损失**——`output_tokens` 本身就随张数线性增长，再乘 `n` 会双重计费。

### 3.4 token **落桶**再收固定价 —— 表达式路线可行，但有硬伤

`billingexpr` 支持按 `c` 做条件分支，仓库里有现成先例：

```
// web/src/features/system-settings/models/tiered-pricing-editor.tsx:167 (GLM-4.5 Air 预设)
len < 32000 && c < 200 ? tier("short_output", ...) : len < 32000 && c >= 200 ? tier("long_output", ...) : ...
```

变量定义 `pkg/billingexpr/expr.md:45-47`：`c` = 输出 token 数、`img_o` = 图片输出 token 数、`ao` = 音频输出 token 数。
映射实现 `service/tiered_settle.go:27-96`：`c = usage.CompletionTokens`（:29）、`imgO = usage.CompletionTokenDetails.ImageTokens`（:41）。

**并且仓库已有 gpt-image 的 token 表达式预设**（`tiered-pricing-editor.tsx:180-183`）：
```
gpt-image-1-mini → tier("base", p * 2 + c * 8 + img * 2.5)
```
——再次确证「gpt-image 系列按 token 计价」是本项目的既定做法。

⚠️ **三个硬伤**：

1. **`img_o` 在 OpenAI 图像路径恒为 0**。`normalizeOpenAIUsage`（`relay_image.go:70-91`）**不填 `CompletionTokenDetails`**（全仓 grep `CompletionTokenDetails.ImageTokens` 在 `relay/channel/openai/` 下**无命中**），而 `Usage.CompletionTokenDetails` 的 JSON tag 是 `completion_tokens_details` 不是 `output_tokens_details`（:233）。所以表达式里只能用 `c`，不能用 `img_o`。
2. **落桶必须先除以张数，而张数拿不到**。`c` 是 n 张图的**总** token。要判「这张是 1k 还是 4k」得算 `c / n`，而 `n` 只能来自 `param("n")`——上一轮 Q4.1 已证 **multipart 下 `param()` 返回 `nil`**（`relay/helper/billing_expr_request.go:54` 只在 `application/json` 时返回 body）。`/v1/images/edits` 直接判错档。
3. **token→1k/2k/4k 的阈值，仓库里推不出来**。全仓唯一相关数字是 `relaykit/dto/openai_image.go:167` `MaxTokens: 1584`（无注释，仅用作预扣估算）；Gemini 侧的 `1400`（`relay-gemini.go:55`）和 `258`（:482）是 Gemini 自己的常量，不可挪用。**2k/4k 的 token 数必须实测采样**（发三档各一张，读日志里的 `output_tokens`）。

### 3.5 tiered 模式下图像预扣的取数

`relay/helper/price.go:302-316`：预扣时 `C = meta.MaxTokens`，图像请求即 **1584**（`relaykit/dto/openai_image.go:167`）。结算时 `TryTieredSettle`（`service/tiered_settle.go:202-227`）用真实 usage 重算并差额结算。`controller/relay.go:467-469` 保证图像请求一定会调 `GetTokenCountMeta()` 拿到这个 1584。

---

## Q4 — 用 token 计费的安全防护

### 4.1 现状：项目对**上游上报的 token 数不设上界**，只在额度换算处饱和

全仓核查结果：

- **没有**任何对 `usage.CompletionTokens` / `OutputTokens` 的上界校验。grep `usage.CompletionTokens =` 命中的 20+ 处全是各渠道的**赋值**（`relay/channel/openai/relay_responses.go:44`、`audio.go:49`、`cohere/relay-cohere.go:191` …），无一处 clamp。
- 只有**下界**钳位：`service/tiered_settle.go:78-83`（`p<0→0`、`c<0→0`）、`service/text_quota.go:351-353`（`baseTokens` 负数→0）。
- `relay/helper/valid_request.go:122` 的 `maxTokensLimit = math.MaxInt32/2` 管的是**请求侧** `max_tokens` 家族字段，不是响应侧 usage。
- 真正的护栏在额度换算：`common/quota_math.go:14` `MaxQuota = math.MaxInt32`，`saturateQuotaBounded`（:86-100）溢出即饱和 + `SysError` 日志 + 返回 `*QuotaClamp`；`QuotaFromDecimalChecked`（:159）/ `QuotaRoundChecked`（:140）。

### 4.2 审计链路（现成，无需新造）

```
text_quota.go:365 QuotaFromDecimalChecked  →  noteQuotaClamp (text_quota.go:194-201)
tiered: billingexpr/settle.go:28 QuotaRoundChecked → TieredResult.Clamp → noteQuotaClamp (tiered_settle.go:225)
                                    ↓
                        relayInfo.QuotaClamp
                                    ↓
service/quota.go:233 / :368  attachQuotaSaturation (service/log_info_generate.go:60)
                                    ↓
              log.other.admin_info.quota_saturation  （非管理员视图自动剥离）
```

### 4.3 用 token 计费需要额外做什么 —— **基本不需要**

| 风险 | 是否需要新防护 |
|---|---|
| 上游谎报天价 `output_tokens` | **不需要新造**。与文本模型（Claude/GPT 全部走同一条路）完全同构；已有 int32 饱和 + `SysError` + `admin_info.quota_saturation` 审计 |
| 预扣不足 | 已有差额结算；`price.go:302-305` 用 `MaxTokens=1584` 估算，结算补差 |
| 负数 token | 已钳位（`tiered_settle.go:78-83`、`text_quota.go:351-353`） |
| 请求侧 `size` 零校验（上一轮 Q3.4 的痛点） | **token 方案下不再是计费乘数，风险自动消失**。`size` 不参与任何乘法 |
| `n` 越界 | `dto.MaxImageN = 128`（`relaykit/dto/openai_image.go:15`），且 token 路径根本不乘 `n` |

> ⚠️ **反过来说，方案 A（解图片头）和方案 B（token 落桶）都要新造防护**：A 要防伪造 PNG 头（上一轮 Q9.2），B 要防 `size`/`n` 绕过 + 落桶阈值上界。**C 是唯一不新增攻击面的方案。**

---

## Q5 — 三条路对比与推荐

| 维度 | **A：解图片字节 → 落桶 → 固定价** | **B：usage.output_tokens → 落桶 → 固定价** | **C：usage.output_tokens → 直接按 token 计价** |
|---|---|---|---|
| **可行性** | 可行但有阻断：`url` 返回形态拿不到字节（上一轮 Q6.5，下载=SSRF 面，不可接受） | 可行，但落桶阈值**仓库内推不出来**，需实测采样 | **完全可行，零阻断** |
| **改动面** | ~120 行 + 白名单 + 测试（上一轮方案 C） | 0 行代码（后台粘表达式），但需先实测三档 token 数 | **0 行代码**（后台配 model_ratio / completion_ratio / image_ratio） |
| 非流式 | ✅ `relay_image.go:37` 有全量 body | ✅ `:43` | ✅ |
| 真 SSE | ✅ 逐帧 `:117` raw 里有 b64 | ✅ `:128` | ✅ |
| JSON→SSE | ✅ `:240` | ✅ `:249` | ✅ |
| **multipart edits** | ✅（响应侧无关请求编码） | ⚠️ **落桶要 `n`，`param()` 在 multipart 下为 nil**（`billing_expr_request.go:54`）→ 判错档 | ✅ **token 天然线性，不需要 n** |
| **上游返 `url`** | ❌ **走不通** | ✅ | ✅ |
| 混合分辨率（一次多张不同尺寸） | ✅ 逐张精确（聚合 OtherRatio，上一轮 Q8.3） | ❌ 只有总 token，无法拆分 | ✅ **天然精确**（总 token 就是总成本） |
| **安全风险** | 中 —— 新增上游可控输入（伪造图片头），要落桶+上界+解码字节上限 | 中 —— `size`/`n` 绕过 + 阈值上界 | **低 —— 与现有文本 token 计费完全同构，不新增攻击面** |
| 与上游成本对应 | 近似（档位化） | 近似（档位化） | **严格一致** |
| 用户是否需要自定档位 | 要 | 要 | **不需要** |
| 是否丢失「按实际出图张数计费」 | 否（保留 `n`） | ⚠️ tiered 模式下 `n` 失效（上一轮 Q4.4），但 `c` 已含张数，等价 | 否（`c` 已含张数） |

### 推荐：**C**

理由（全部有据）：

1. **零代码**。`gpt-image-1` 已是这套配置（`model_ratio.go:59,332,639` + `defaultModelPrice` 无条目），`gpt-image-1-mini` 的表达式预设也是 token 式（`tiered-pricing-editor.tsx:182`）。给 `gpt-image-2` 照配即可。
2. **全路径覆盖**。usage 在响应顶层，与 `data[]` 内容、请求编码（JSON / multipart）、返回形态（`url` / `b64_json`）全部解耦。A 和 B 各自的硬伤（url 走不通 / multipart 拿不到 n）都不存在。
3. **不新增攻击面**。复用现成的 int32 饱和 + `admin_info.quota_saturation` 审计（Q4.2），符合 AGENTS.md 计费安全条款且无需新写守卫。
4. **与上游成本严格对应**。OpenAI 按 token 收，我们按 token 收，毛利率恒定；用户不用去猜 1k/2k/4k 的分档边界，也不会在「用户实际出了 2k 但我按 1k 收」时亏钱。
5. **混合分辨率天然正确**。上一轮为了表达「一次返回 1 张 1k + 2 张 4k」要设计聚合系数（Q8.3），token 方案下这个问题根本不存在。

**如果用户坚持要 $0.1 / $0.15 / $0.2 的固定档位**（比如为了对外报价可读），退而选 **B**，但必须先：
- 实测采样三档各一张的 `output_tokens`（无 `n`，即 n=1），确定阈值；
- 接受 multipart `/v1/images/edits` 多张时判错档（或在表达式里对 edits 场景另开分支）；
- 表达式形如 `c <= T1 ? tier("1k", 100000) : (c <= T2 ? tier("2k", 150000) : tier("4k", 200000))`（`exprOutput = 美元 × 1e6`，`pkg/billingexpr/settle.go:11`）。

**A 不再推荐**：相对 B/C 唯一的优势是「混合分辨率逐张精确」，而 C 本来就精确；代价却是 `url` 分支走不通 + 新增伪造图片头攻击面 + ~120 行代码。

### 上游不返回 usage 时的退路 —— **这是 C 唯一的真实风险，必须明确**

若上游（尤其是第三方中转渠道）不回 `usage`：

- `usageResp.Usage` 全零 → `relay/image_handler.go:125-130` 把 `TotalTokens`/`PromptTokens` 兜底为 **1**，`CompletionTokens` **保持 0**。
- `hasBillableUsage()`（`service/text_quota.go:75-77`）= `TotalTokens > 0` → **true**，不会跳过计费。
- 结算：`quota = (1 + 0×completionRatio) × modelRatio × groupRatio` ≈ 极小值，再被 `:380-382` 兜底成 **1 quota**（≈ $0.000002）。
- ⇒ **几乎白送一张图。** tiered 模式同理（`C=0` 进表达式）。

退路（按优先级）：

1. **先实测**：拿真实渠道打一发 `gpt-image-2`，看日志/ `admin_info.usage_billing_path`（`service/billing_usage.go:27-55`，`upstream` vs `local`）确认上游是否回 usage。这是上线前的**前置条件**，不是可选项。
2. **若某些渠道不回 usage**：对那些渠道单独配 **Fixed price**（`model_price`）——固定价不依赖 usage（`text_quota.go:369` 只用 `ModelPrice × QuotaPerUnit × GroupRatio`），对「无 usage」完全免疫。这是 A/B/C 之外最稳的兜底，代价是退回按张收费、不区分分辨率。
3. **若要在 token 模式下防白送**：可考虑给表达式加 `max(c, 下限)`（`billingexpr` 有 `max` 函数，`expr.md:87`），把无 usage 的请求按最低档兜底。**注意这需要走 tiered_expr 模式**，纯 model_ratio 模式没有这个表达能力。

---

## 本轮 Caveats / Not Found

1. **未找到 `output_tokens_details` 的解析**。`dto.Usage` 只有 `completion_tokens_details`（`relaykit/dto/openai_response.go:233`），若上游按 `output_tokens_details` 返回图片输出 token 明细，当前**解不出**，`img_o` 恒 0。用 `c` 规避。
2. **未找到 1k/2k/4k 与 token 数的对应关系**。仓库内唯一数字是 `MaxTokens: 1584`（`relaykit/dto/openai_image.go:167`，无注释、仅用于预扣估算）。**必须实测采样**。
3. **未验证上游是否总回 usage**。仓库内证据只证明「项目按这个形状解析，且测试固件长这样」，不证明 OpenAI 或第三方渠道一定返回。无联网工具，无法查官方文档。
4. **未验证非 OpenAI 渠道**。xai 复用 `OpenaiImageHandler`（`relay/channel/xai/adaptor.go:117`），其余（ali / zhipu / minimax / jimeng / replicate / gemini）各有 handler，usage 行为未逐一核查。
5. **未实跑**。本轮全部结论来自代码与测试固件的静态阅读，未执行 `go test`（上一轮跑过表达式验证，本轮无新表达式需要验证）。
