# Research: 图像生成计费链路 —— 纯表达式配置能否实现按分辨率阶梯计费

- **Query**: 在不二开代码的前提下，只靠 new-api 后台「表达式阶梯计价」能否让 6 个图像模型按 1K/2K/4K 分档计价；给出表达式、单位换算、字符串处理能力、img_o 来源、失败兜底与安全边界
- **Scope**: mixed（内部代码只读 + 上游 `upstream/main` 对比 + 官方文档抓取 + 在 `/tmp` 临时模块里对真实 `pkg/billingexpr` 实跑；**未修改仓库任何代码**）
- **Date**: 2026-09-13
- **基线**: FlowAPI `main@9794c7363`（与上游 new-api 的 merge-base 为 `e468b7391 2026-08-27`）；`go test ./pkg/billingexpr/ ./relay/helper/` 通过
- **实测数据**（分辨率参数落在哪个字段、各模型真实回传的 usage 数值）由 hk-log-research 的 live probe 文档覆盖，本文只写读源码才能回答的部分

---

## 一句话结论

**能。纯后台表达式配置、零代码改动即可实现按分辨率阶梯计价，FlowAPI 当前版本与上游 new-api 均可运行同一份表达式。** 唯一硬性缺口：FlowAPI 当前版本下 multipart 的 `/v1/images/edits`（Playground 编辑图）读不到 `size`，会落到表达式的兜底档；上游 new-api 2026-09-09 之后的版本已原生支持（见 0.8）。

---

## 0. 直接回答最高优先级问题

### 0.1 生死线：图像请求走不走表达式求值 —— **走，与 chat 是同一个入口**

`/v1/images/generations` 与 `/v1/chat/completions` 共用 `controller.Relay`，预扣与结算都进 `tiered_expr`：

| 步骤 | 位置 | 说明 |
|---|---|---|
| 解析请求 | `controller/relay.go:121` → `relay/helper/valid_request.go:182-287` | `size` 进 `dto.ImageRequest.Size`（JSON `:235`，multipart `:205`）|
| 计费入口（与 chat 相同） | `controller/relay.go:165` `helper.ModelPriceHelper(c, relayInfo, tokens, meta)` | 不区分 relay 格式 |
| **tiered 分派** | `relay/helper/price.go:99-101` | `GetBillingMode(model) == "tiered_expr"` → `modelPriceHelperTiered`，**在按次/按 token 判断之前直接 return** |
| 表达式预扣 | `price.go:296-365`：`:307` 取原始请求 body、`:312-316` 运行表达式、`:322-323` 换算 quota、`:351-352` 冻结 snapshot 与 body | 图像请求 `P=prompt 文本 token`，`C=meta.MaxTokens=1584`（`relaykit/dto/openai_image.go:167`）|
| 预扣落账 | `controller/relay.go:176` `service.PreConsumeBilling` | |
| 图像响应处理 | `relay/image_handler.go:113` `adaptor.DoResponse`；`:125-130` 把 0 token 补成 1 保证可结算 | |
| **结算（与 chat 相同函数）** | `relay/image_handler.go:149` `service.PostTextConsumeQuota` → `service/text_quota.go:454` `TryTieredSettle` → `service/tiered_settle.go:202-228` 用冻结 snapshot + 同一份 body 重跑表达式 | |
| 日志 | `text_quota.go:557-559` → `service/log_info_generate.go:352-368` 写 `billing_mode=tiered_expr`、`matched_tier`、`expr_b64` | 前端「阶梯计费 / 命中档位」只认这两个字段（`web/src/features/usage-logs/components/columns/common-logs-columns.tsx:154`）|

旁证：用户现网 `gemini-3-pro-image-preview` 的日志已经显示「阶梯计费 / 命中档位 2K」—— 只有这条链路会写这些字段，说明该模型**已经在 `tiered_expr` 模式下跑图像请求**。上游 new-api 同一入口：`upstream/main:relay/helper/price.go:89-90`、`upstream/main:relay/image_handler.go:195`。

### 0.2 单位语义：`tier("1k", V)` 的 `V` 是什么 —— **`V` 是「$/1M」域的数，按次价必须写成 `美元 × 1,000,000`**

完整链路（file:line）：

1. 求值：`pkg/billingexpr/run.go:123-131` `expr.Run` 的结果必须是 `float64`，原样返回（`tier()` 只是记录档位名后把 `value` 原样返回，`:68-72`）。
2. 预扣换算：`relay/helper/price.go:322` `quotaBeforeGroup := rawCost / 1_000_000 * common.QuotaPerUnit`，`:323` `× GroupRatio` → `billingexpr.QuotaRoundStrict`（`pkg/billingexpr/round.go:17-19` → `common.QuotaRoundStrict`）。
3. 结算换算：`pkg/billingexpr/settle.go:8-13` `quotaConversion = exprOutput / 1_000_000 * snap.QuotaPerUnit`；`:28` `× snap.GroupRatio` → `common.QuotaRoundChecked`（四舍五入 + int32 饱和 + 审计）。
4. `QuotaPerUnit = 500 * 1000.0`（`common/constants.go:22`），即 **$1 = 500,000 quota**；站点实测也是 500000。
5. 写日志：`text_quota.go:563-572` `RecordConsumeLog(Quota: summary.Quota)`。

所以 `quota = V / 1e6 × 500000 × groupRatio = V / 2 × groupRatio`。用生产同款 `ComputeTieredQuotaWithRequest` 实跑：

| 表达式 | quota | 折算 | 说明 |
|---|---|---|---|
| `tier("1k", 0.12)` | **0** | **$0.0000，免费** | 0.12/1e6×500000 = 0.06 → 四舍五入为 0；tiered 模式 `ModelRatio=0`，`text_quota.go:417` 的「最低 1 quota」兜底不生效 → **请求免费放行，无任何报错** |
| `tier("1k", 120000)` | 60000 | $0.12 | 正确写法 |
| `tier("1k", 0.12 * 1000000)` | 60000 | $0.12 | 等价写法，可读性更好 |
| `120000`（不包 tier） | 60000 | $0.12 | parser 接受，但 `matched_tier` 为空，日志不显示档位 |

**结论：每张 ¥0.12（站点 ¥:$=1:1）写 `120000` 或 `0.12 * 1000000`；写 `0.12` 会把图片变成免费。** 上游 new-api 新版的 `fixed(amount)` 函数内部就是 `amount * 1_000_000`（`upstream/main:pkg/billingexpr/run.go` `"fixed"` 分支；`expr.md`「`fixed(amount)` returns `amount * 1,000,000` internally」），语义完全一致，只是 FlowAPI 当前版本没有这个函数（0.8）。

### 0.3 常数项合法吗 —— **合法，语法和语义都是**

- 语法：`expr.Compile(body, expr.Env(...), expr.AsFloat64())`（`compile.go:190`）对纯常量表达式正常编译；`tier` 的签名是 `func(string, float64) float64`（`:135`），整数字面量 `120000` 自动转 float64。已实跑 `tier("1k", 120000)`、`tier("1k", 0.12 * 1000000)`、`120000`。
- 语义：不引用任何 token 变量时，`p/c/img_o` 等被忽略，结果就是常量；不会被当成 0（0.2 表）。`extractUsedVars`（`compile.go:231-245`）只影响 `p/c` 的自动排除，与常量无关。
- 保存侧：FlowAPI 后端保存时**没有**编译/冒烟校验（`SmokeTestExpr` 全仓无调用点；`model/option.go:293-301` `validateOptionValue` 不含 `billing_setting.*`），写错要到请求时才报 400（`price.go:317-319`）。配完务必用测试令牌各打一次。

### 0.4 六个模型的表达式（FlowAPI 当前引擎实跑通过）

价格表（¥/张，¥:$=1:1）→ 系数 = 价格 × 1,000,000。分档规则采用**最大边**：`≤1536 → 1K`、`≤2560 → 2K`、`>2560 → 4K`（换成总像素见 0.9）。`auto` / 缺省 / 非 `WxH` 字符串 / multipart 读不到 → 落**基础档**（想改成落最高档，把最后一个 `tier(...)` 换到 `ok ? … : tier("4k", …)` 的 else 位置即可）。张数乘 `float(param("n") ?? 1)`（`n` 已在校验层被 `dto.MaxImageN=128` 封顶）。

**gpt-image-2**（1K=2K=0.12，4K=0.21，只需判「是不是 4K」）：

```
let s = string(param("size") ?? ""); let ok = s matches "^[0-9]+x[0-9]+$"; let m = ok ? max(float(int(split(s, "x")[0])), float(int(split(s, "x")[1]))) : 0.0; (m > 2560 ? tier("4k", 210000) : tier("1k-2k", 120000)) * float(param("n") ?? 1)
```

**gpt-image-2.5-flare / gpt-image-2.5-sunburst**（1K=0.12，2K=0.16，4K=0.21）：

```
let s = string(param("size") ?? ""); let ok = s matches "^[0-9]+x[0-9]+$"; let m = ok ? max(float(int(split(s, "x")[0])), float(int(split(s, "x")[1]))) : 0.0; (m > 2560 ? tier("4k", 210000) : (m > 1536 ? tier("2k", 160000) : tier("1k", 120000))) * float(param("n") ?? 1)
```

**gemini-3-pro-image-preview**（1K=2K=0.15，4K=0.2；有 `img_o` 按实际输出判，否则按请求参数判；官方 Pro 4K=2000 tokens、1K/2K=1120，阈值取中点 1500）：

```
let sz = string(param("size") ?? param("generationConfig.imageConfig.imageSize") ?? param("extra_body.google.image_config.image_size") ?? param("image_config.image_size") ?? ""); let is4k = img_o > 0 ? img_o >= 1500 : (sz == "4K" || sz == "4k" || sz == "4096x4096"); (is4k ? tier("4k", 200000) : tier("1k-2k", 150000)) * float(param("n") ?? 1)
```

**gemini-3.1-flash-image-preview / gemini-3.1-flash-image**（1K=2K=0.3，4K=0.35；官方 Flash 4K=2520、2K=1680、1K=1120、0.5K=747，阈值取 2100）：

```
let sz = string(param("size") ?? param("generationConfig.imageConfig.imageSize") ?? param("extra_body.google.image_config.image_size") ?? param("image_config.image_size") ?? ""); let is4k = img_o > 0 ? img_o >= 2100 : (sz == "4K" || sz == "4k" || sz == "4096x4096"); (is4k ? tier("4k", 350000) : tier("1k-2k", 300000)) * float(param("n") ?? 1)
```

实跑结果（`ComputeTieredQuotaWithRequest`，`QuotaPerUnit=500000`，`groupRatio=1`）：

| 模型 | 输入 | quota | 折算 | 档位 |
|---|---|---|---|---|
| gpt-image-2 | `size=2048x1152` / `2048x896` / `1024x1024` / `2560x1440` / `1672x941` | 60000 | $0.12 | 1k-2k |
| gpt-image-2 | `size=3456x1728` / `2880x2880` / `3840x2160` / `4096x4096` | 105000 | $0.21 | 4k |
| gpt-image-2 | `size=auto` / 无 size / 空 body（multipart） | 60000 | $0.12 | 1k-2k |
| gpt-image-2 | `size=3840x2160, n=2`（`n` 为数字或字符串 `"2"` 都行） | 210000 | $0.42 | 4k |
| gpt-image-2.5-* | `1024x1024` / `1536x1024` / `auto` | 60000 | $0.12 | 1k |
| gpt-image-2.5-* | `1573x1573` / `2048x1152` / `2560x1440` | 80000 | $0.16 | 2k |
| gpt-image-2.5-* | `3001x3001` | 105000 | $0.21 | 4k |
| gpt-image-2.5-* | `1024x1536, n=3` | 180000 | $0.36 | 1k |
| gemini-3-pro | 预扣（`img_o=0`）+ `size=4K` | 100000 | $0.20 | 4k |
| gemini-3-pro | 结算 `img_o=2000` | 100000 | $0.20 | 4k |
| gemini-3-pro | 结算 `img_o=1120`（请求写 4K 但上游实际出 2K） | 75000 | $0.15 | 1k-2k（按实际，退差额）|
| gemini-3-pro | 原生 `generationConfig.imageConfig.imageSize=4K`、无 usage | 100000 | $0.20 | 4k |
| gemini-3-pro | chat `extra_body.google.image_config.image_size=4K` + `img_o=2000` | 100000 | $0.20 | 4k |
| gemini-3-pro | 什么都没有 / `img_o=1120` | 75000 | $0.15 | 1k-2k |
| gemini-flash | `img_o=2520` | 175000 | $0.35 | 4k |
| gemini-flash | `img_o=1680` | 150000 | $0.30 | 1k-2k |
| gemini-flash | `size=4K`、无 usage | 175000 | $0.35 | 4k |

Gemini 表达式的 `param()` 探针顺序（`size` → 原生 `imageSize` → chat `extra_body` → 顶层 `image_config`）覆盖了 B2.1 列出的全部入口；**实际生产用的是哪个字段以 live probe 为准，确认后可以删掉多余探针**。`img_o` 优先级高于请求参数：只要上游回了输出图 token 就按实际出图收，请求参数只在无 usage 时兜底。

### 0.5 字符串处理能力 —— **远超 UI 提示，能从 `"2048x1152"` 精确取宽高，不需要枚举**

- **`param()` 返回类型**：JSON 字符串 → Go `string`；数字 → `float64`；缺失 → `nil`（`run.go:94-104` `gjson.Result.Value()`）。实跑 `type(param("size")) == "string"`、`type(param("n")) == "float"`、缺失 `== nil` 均成立。
- **求值器实际可用的函数 = new-api 注入的 + expr-lang v1.17.8 全部内置**。`compile.go:190` 用 `expr.Env(map)` 编译，**没有** `DisableBuiltin` / `DisableAllBuiltins`；上游 new-api 同样（`upstream/main:pkg/billingexpr/compile.go:212`）。UI 只列了 new-api 注入的那几个。expr-lang 内置（`$GOMODCACHE/github.com/expr-lang/expr@v1.17.8/builtin/builtin.go`）：`type, int, float, string, round, trim, trimPrefix, trimSuffix, upper, lower, split, splitAfter, replace, repeat, join, indexOf, lastIndexOf, hasPrefix, hasSuffix, first, last, get, take, keys, values, sort, uniq, concat, flatten, all/any/none/one/filter/map/find/count/sum/mean/median, toJSON/fromJSON, toBase64/fromBase64, now/duration/date/timezone …`；运算符：`? :`、`??`、`in`、`matches`（正则）、`contains`、`startsWith`、`endsWith`、`let x = …;`、`if … {} else {}`、下标 `[i]`。
- **实跑确认可用**：`split(s,"x")[1]`、`int()`、`float()`、`string()`、`trim()`、`lower()`、`round()`、`s matches "^[0-9]+x[0-9]+$"`、`s contains "2048"`、`startsWith/endsWith`、`x in ["a","b"]`、`if … else`、`?? ` 多级链、`param("extra_body.google.image_config.image_size")` 深路径。
- **两个坑**：(1) `len` 被 token 变量 `len` 遮蔽（`compile.go:127`），`len(split(...))` 编译失败 `float64 is not callable` —— 判格式用 `matches` 正则；(2) 顶层结果必须是 float64（`compile.go:190` `AsFloat64`、`run.go:127-130`），字符串管道只能放在 `let` 里。
- **gjson 路径本身做不了字符串拆分**：`param("size.0")` 对字符串值返回 `nil`；gjson 修饰符（`@tostr` 等）不能切分 `"2048x1152"`。不需要它，expr-lang 的 `split/int` 已足够。
- **纯 `has()` 枚举写法的覆盖率**（万一某个下游版本连 expr-lang 内置都不可用——本仓库与上游都不是这种情况）：对生产尺寸清单，gpt-image-2 只需枚举 4K 边长子串 `3840 / 4096 / 3456 / 2880 / 3001 / 3344`（6 个 `has()`）即可 100% 覆盖清单里的 4K 尺寸，其余全落 1k-2k；已实跑：`(has(param("size"),"3840") || has(param("size"),"4096") || … ? tier("4k", 210000) : tier("1k-2k", 120000)) * float(param("n") ?? 1)`。子串匹配可被 `"13840x1"` 这类串误判，所以正式版用 `matches + split`。
- **表达式长度/嵌套上限**：expr-lang 无硬上限；`options.value` 是 GORM 无长度 `string`（`model/option.go:19-22`），MySQL 驱动未设 `DefaultStringSize` 时映射为 `longtext`，PostgreSQL/SQLite 为 `text`（驱动默认行为，未在生产库核对）；全站所有模型的 `billing_expr` 是一整个 JSON map 存在一行里，现有预设最长约 300 字符，上面最长的表达式约 420 字符，无压力。
- **有没有更好的分档依据**：图像请求 DTO 字段见 `relaykit/dto/openai_image.go:17-43`（`model, prompt, n, size, quality, response_format, style, user, background, moderation, output_format, output_compression, partial_images, stream, images, mask, input_fidelity, watermark …`），且 `param()` 读的是**原始 body**，客户端多发的任何字段都读得到。`quality`（gpt-image-2：low/medium/high/auto；2.5：low/medium/high/xhigh/max/auto）与分辨率**正交**（OpenAI 文档：size 与 quality 各自独立影响 token 数），不能当 1K/2K/4K 的代理。**`size` 就是唯一正确的分档依据**；Gemini 的 `imageSize` 是枚举，直接等值比较。

### 0.6 `img_o` 从哪来、缺失时是什么

- 绑定：`service/tiered_settle.go:41` `imgO := float64(usage.CompletionTokenDetails.ImageTokens)` → `run.go:65` `"img_o": params.ImgO`。字段 tag `completion_tokens_details.image_tokens`（`relaykit/dto/openai_response.go:233,289`）。
- 填充来源：Gemini 原生/转换路径 `candidatesTokensDetails[modality=IMAGE]` → `relaykit/relayconvert/internal/gemini_chat/to_oai_chat_resp.go:57-60`、`service/billing_usage.go:196`；OpenAI 兼容 chat 响应直接反序列化；`usageFromOpenAIBillingUsage`（`service/billing_usage.go:99-100`）是整结构体拷贝，细节保留。OpenAI `/v1/images/*` 路径 `normalizeOpenAIUsage`（`relay/channel/openai/relay_image.go:70-91`）不碰 `CompletionTokenDetails`，上游若不回 `completion_tokens_details` 则为 0。
- **缺失 = 0，不是缺省、不报错**。因此：`img_o * 120` 这种「按 token 单价」写法在 usage 缺失时**费用为 0，图片免费**（严重事故）；0.4 里 `img_o` 只做**条件**、叶子全是常量，`img_o=0` 时走参数分支或基础档，**永远不会算成 0**。这就是 0.4 用 `img_o > 0 ? … : …` 的原因。
- 预扣时 `img_o` 恒为 0（`price.go:312-316` 只传 `P/C/Len`），结算时才有真实值 → 天然「先估后补」（0.7）。

### 0.7 `tiered_expr` 与按次计费能否共存、失败兜底

- **互斥，表达式完全替换**：`price.go:99-101` 命中 `tiered_expr` 就 return，`ModelPrice` / `ModelRatio` / `ImagePriceRatio` / `n` 的 `OtherRatios` 全部不再参与（`price.go:147-200` 是 else 分支）。旧价格字段仍存在 DB 里，切回「按次」模式即恢复（前端 `model-pricing-sheet.tsx:441-464` 提交时两套字段都带上）。
- **没有「表达式失败回退原价」**：
  - 模型标了 `tiered_expr` 但无表达式 → 预扣报错 400（`price.go:297-300`），请求被拒。
  - 表达式编译/运行错误（预扣）→ 400 `ErrorCodeModelPriceError`（`price.go:317-319`、`controller/relay.go:165-169`），**不会免费放行**。
  - 表达式在结算时运行错误 → 按预扣额（或估算额）收（`tiered_settle.go:214-219`），不免费。
  - **表达式算出 0**（单位写错、条件全不命中且叶子是 0）→ 预扣 0、结算 0、无报错、**免费放行**。这是唯一的静默失败模式，靠配置纪律（0.2）和上线前用测试令牌验证 `matched_tier` 与扣费额来防。
- 预扣/结算差额：`service/billing.go:52-77` `SettleBilling` 按 `actual - preConsumed` 双向补扣/退还；上游非 2xx 不结算并退预扣（`image_handler.go:100-109`、`controller/relay.go:182-191`）。
- 分组重试：`service/tiered_settle.go:99-159` 只刷新 group 字段，表达式与 body 冻结不变。

### 0.8 上游 new-api 新版差异（对「下游网关也要配」至关重要）

FlowAPI 与上游的 merge-base 是 `e468b7391 2026-08-27`。上游在 **2026-09-09** 两个提交里扩展了同一套引擎（`git log upstream/main -S'"fixed"' -- pkg/billingexpr/compile.go`）：

| 提交 | 内容 | FlowAPI 当前 |
|---|---|---|
| `f362c7c51 feat(billingexpr): support image cache and quantity variables` | 新变量 `image_count`（1–128，来自 `request.ImageCount`）、`img_cr`；`image_count` 结算按实际张数、SSE 可减、断连不减 | 无 |
| `064ed943e feat(billing): support fixed per-request expression pricing` | 新函数 `fixed(amount)` = `amount * 1_000_000`，官方按次写法 `tier("image", fixed(0.12)) * image_count`；`u("field")` 读 usage 事实；新增 `POST /api/option/model_pricing/convert` 把旧按次价自动转成表达式 | 无 |
| 同期 `relay/helper/billing_expr_request.go` `ResolveImageBillingRequestInput` | 表达式引用 `image_count` 时，把 `param()` 的 body 替换成 `{"model","n","size","quality","parameters"}` 标量（**multipart 也有**），不含 prompt/图片 | FlowAPI 只读原始 JSON body，multipart 为 nil |

**可移植性结论**：
- 0.4 的表达式只用了 `param / has / tier / max` + expr-lang 内置 + 数字常量，**在 FlowAPI 与上游任何带 `param()` 的版本上语义相同**（上游「Existing expressions without `fixed()` retain their original grammar and behavior」）。两边可以粘同一份。
- 若下游网关是 2026-09-09 之后的上游版本，可改用原生写法：`(cond ? tier("4k", fixed(0.21)) : tier("1k-2k", fixed(0.12))) * image_count` —— 好处是 multipart 也能读 `size`、张数按实际结算、日志多 `billing_unit=request` / `fixed_price` 字段。注意上游对 `fixed()` 表达式有额外校验（叶子只能是 `fixed(字面量)`，不能与 token 项相加、不能乘 token）。
- FlowAPI 若要拿到 `fixed()` / `image_count` / multipart 支持，走「同步上游」（merge `upstream/main`）而不是二开；这不在本任务范围，但它是路线 B 之外唯一不产生本地私有代码的补缺口方式。

### 0.9 分档规则：生产尺寸清单落桶（PRD 拍板用）

规则候选与生产尺寸（team-lead 提供的 25 种）的归档：

| 尺寸（次数） | 最大边规则（≤1536/≤2560） | 总像素规则（≤1,572,864/≤4,194,304） |
|---|---|---|
| 2048x1152 (201) | 2K | 2K（2,359,296）|
| 2048x896 (115) | 2K | 2K（1,835,008）|
| 1024x1024 (24) | 1K | 1K |
| 2560x1440 (15) | 2K | 2K（3,686,400）|
| 2048x2048 (5) | 2K | 2K（=4,194,304，取 ≤）|
| 1536x1024 (3) | 1K | 1K（=1,572,864）|
| 3456x1728 / 2880x2880 / 3840x2160 / 2160x3840 / 4096x4096 / 3001x3001 / 3344x1882 | 4K | 4K |
| 1672x941 | 2K | 2K（1,573,352，刚超）|
| 1254x1254 | 1K | 1K（1,572,516，刚不超）|
| 2288x1824 / 1573x1573 / 2001x2001 / 2000x2000 / 1152x2048 | 2K | 2K |
| **1664x944** | **2K**（1664>1536） | **1K**（1,570,816）|
| 1248x1248 / 1100x1400 / 1200x1200 | 1K | 1K |
| auto | 兜底档 | 兜底档 |

两种规则只在 `1664x944` 上不一致；对 gpt-image-2 / Gemini（只分「是否 4K」）两种规则结果完全相同。按次数估算流量占比（最大边规则）：2K ≈ 88%、1K ≈ 8%、4K ≈ 4%。总像素版表达式已实跑（把 `let m = …` 换成 `let px = ok ? float(int(split(s,"x")[0])) * float(int(split(s,"x")[1])) : 0.0`，阈值 `px > 4194304` / `px > 1572864`）。

官方尺寸定义（本轮抓取核实）：OpenAI `gpt-image-2` / `2.5-*` 接受任意 `WxH`（16 的倍数、长短边比 ≤3:1、最大边 ≤3840、像素 655,360–8,294,400），文档「Popular sizes」：`1024x1024 / 1536x1024 / 1024x1536 / 2048x2048(2K) / 2048x1152(2K) / 3840x2160(4K) / 2160x3840(4K) / auto`；**4K 的标准写法是 3840x2160，4096 已超上限**（生产里出现的 `4096x4096` 说明 tuzi 上游放宽了）。Gemini 是枚举 `"1K"/"2K"/"4K"`（Flash 另有 512px），像素随 aspectRatio 变化，缺省 1K。

### 0.10 仍需拍板 / 由实测覆盖

| 事项 | 归属 |
|---|---|
| 最大边 vs 总像素；`auto`/缺省/multipart 落基础档还是最高档 | PRD |
| Gemini 请求侧分辨率参数实际落在哪个字段；各模型真实回传的 `img_o` 数值（决定阈值 1500/2100 是否合适、`/v1/images/generations` 入口是否有 usage） | live probe（hk-log-research）|
| 现网 6 个模型当前的 `billing_mode` / `billing_expr`（Gemini Pro 已是 tiered，表达式内容未知） | 后台导出 |
| 下游网关的 new-api 版本（决定能否用 `fixed()`/`image_count`） | 用户 |
| Gemini 走 chat 时 `param("n")` 是 chat 的 choices 数，不是张数；若担心误乘，可把因子改成 `(param("messages") != nil ? 1.0 : float(param("n") ?? 1))` | PRD |
| Flash 的 512px（747 tokens）没有价目，当前落 1k-2k | PRD |

---

## A. 现有能力盘点（细节）

### A1. `pkg/billingexpr` 表达式系统

#### A1.1 可用变量（v1 环境）

`pkg/billingexpr/compile.go:124-151`（编译期类型原型）与 `run.go:57-121`（运行期注入）完全一致：

| 类别 | 名字 | 来源（结算时） |
|---|---|---|
| token 变量 | `p` `c` `len` `cr` `cc` `cc1h` `img` `img_o` `ai` `ao` | `service/tiered_settle.go:27-97` `BuildTieredTokenParams`；`img_o = usage.CompletionTokenDetails.ImageTokens`（`:41`） |
| 请求探针 | `param(path)`、`header(key)` | `run.go:94-104` gjson 读 `request.Body`；`:91-93` header 小写匹配 |
| 时间探针 | `hour/minute/weekday/month/day(tz)` | `run.go:111-115` |
| 辅助 | `tier(name, value)`、`has(src, substr)`、`max/min/abs/ceil/floor` | `run.go:68-72,105-120` |
| expr-lang 内置 | 见 0.5 | 未禁用 |

没有名为 `size/width/height/resolution/quality` 的一等变量，但不需要：`param()` 读原始 body。

#### A1.2 `param()` 的数据来源与时机

`relay/helper/billing_expr_request.go:13-35`：首次调用读 `common.GetBodyStorage(c)` 的**原始入站字节**（`:53-62`，仅 `application/json`），之后复用 `info.BillingRequestInput`（`price.go:352` 冻结）。所以 `param()` 看到的是**客户端原文**，不是转换后发往上游的 body；不受 `ConvertImageRequest`、`ParamOverride`、passthrough 影响；预扣与结算读同一份。同文件 `:37-51` `BuildBillingExprRequestInputFromRequest` 把 DTO Marshal 成 JSON 当 body，目前只被渠道测试用（`controller/channel-test.go:518-528`）。

#### A1.3 已知限制

1. `len` 遮蔽内置 `len()`（0.5）。
2. multipart 请求 `param()` 为 nil（`billing_expr_request.go:53-62`）；Playground 编辑图走 multipart（`web/src/features/image-playground/lib/api.ts:204-221`），生成图走 JSON（`:172-184`）。
3. 保存无校验（0.3）。
4. tiered 模式下 `n` 不按实际出图修正：`modelPriceHelperTiered` 的 `PriceData` 没有 `UsePrice`（`price.go:354-359`），`updateOpenAIImageCount` 早退（`relay/channel/openai/relay_image.go:25-30`），`composeTieredTextQuota` 不乘 `OtherRatios`（`text_quota.go:240-263`）。张数只能来自 `param("n")`。
5. 定价页解析器只认 `p|c|len` 条件与 `变量*系数`（`web/src/features/pricing/lib/billing-expr.ts:262-313`），常量档位在定价页显示单价 0 —— 仅展示问题；编辑器对无法可视化的表达式自动进 Raw 模式且不丢内容（`tiered-pricing-editor.tsx:1654-1672`）；`splitBillingExprAndRequestRules`（`billing-expr.ts:561-591`）识别不出规则因子时原样保留。

### A2. 现有图像倍率机制

| 机制 | 位置 | 语义 | 与本需求关系 |
|---|---|---|---|
| `TokenCountMeta.ImagePriceRatio` | 赋值 `relaykit/dto/openai_image.go:133-171`（只对 `dall-e*`）；唯一读取 `price.go:147-151`（固定价分支） | 固定价 × 请求 size/quality 倍率，硬编码 | tiered 模式完全绕过；是路线 B/C 的模板 |
| `PriceData.AddOtherRatio` | `types/price_data.go:45-53`（覆盖非累加）；`:116-118` 拒非正/Inf/NaN | 通用乘数 | 固定价模式下 `n` 走这里 |
| `image_ratio` | `price.go:138` → `text_quota.go:369-373` 乘输入图 token | **输入**参考图 token 倍率 | 同名陷阱，与输出分辨率无关 |

### A3. `setting/` 定价配置结构

`price.go:93-101`：`tiered_expr`（最高优先级）→ `ModelPrice` 按次 → `ModelRatio` 按 token。存储 `setting/billing_setting/tiered_billing.go:19-31`（`billing_setting.billing_mode` / `billing_setting.billing_expr` 两张 `map[model]string`）。6 个模型在代码里都没有默认价格条目（`setting/ratio_setting/model_ratio.go` 无命中；`relay/channel/openai/constant.go` 无 `gpt-image-2`）。前端 `model-pricing-sheet.tsx:549-555` 三个 Tab，Expression Tab 提交 `billingMode=tiered_expr` + `billingExpr`（`:456-459`）；编辑器预设含 `Gemini 3 Pro Image: tier("base", p * 2 + c * 12 + img_o * 120)`（`tiered-pricing-editor.tsx:190-193`）—— 这是 Google 官方按 token 计价的写法，但 usage 缺失时会免费（0.6）。

---

## B. 分辨率信息的可得性（细节）

### B1. OpenAI 图像路径

调用链见 0.1。`size` 对 gpt-image-* **零校验**（`valid_request.go:245-247` 只拒全角 `×`；`:254-275` 白名单只覆盖 dall-e-2/3 与 gpt-image-1 的 quality 缺省）。响应侧 `dto.ImageResponse.Data[]` 只有 `url/b64_json/revised_prompt`（`openai_image.go:183-192`），`dto.Usage` 无尺寸字段；上游（tuzi）实测响应形状不稳定、2.5 系列无 usage、gpt-image-2 usage 带 `estimated`（旧调查 `gateway-curl-findings.md` §1）→ OpenAI 路径只能按请求 `size` 分档。

### B2. Gemini 图像路径

#### B2.1 请求侧三种入口

| 入口 | 分辨率参数 | 代码 | `param()` 路径 |
|---|---|---|---|
| 原生 `/v1beta/models/{m}:generateContent` | `generationConfig.imageConfig.imageSize`（`"1K"/"2K"/"4K"`，大写 K）| `relaykit/dto/gemini.go:350`（`:374,432-433` 接受 snake `image_config`）| `param("generationConfig.imageConfig.imageSize")` |
| OpenAI chat → Gemini 原生（Gemini 类型渠道） | `extra_body.google.image_config.image_size` | `relaykit/relayconvert/internal/oai_chat/to_gemini_chat_req.go:119-146`（驼峰 `imageSize` 被 400 拒绝）| `param("extra_body.google.image_config.image_size")` |
| `/v1/images/generations` | OpenAI 类型渠道：`size` 原样透传（`relay/channel/openai/adaptor.go:443-448`）；Gemini 类型渠道只接受 `imagen*`（`relay/channel/gemini/adaptor.go:63-66`），且只把 `quality` 映射到 `1K/2K`（`:110-124`）| `param("size")` |

⚠️ `GeneralOpenAIRequest`（`relaykit/dto/openai_request.go`）没有顶层 `image_config` 字段也不保留未知字段，chat 请求顶层 `image_config` 发往 OpenAI 类型上游时会被丢掉（`extra_body` 有字段 `:81`，会保留）。**按请求参数分档的前提是参数真被上游执行**，否则会出现按 4K 收费、实际出 1K —— 这正是 0.4 让 `img_o` 优先的原因。

#### B2.2 响应侧 `img_o` 与官方 token 表

来源见 0.6。Google 官方（2026-09-13 抓取 https://ai.google.dev/gemini-api/docs/image-generation 与 /pricing）：

| 模型 | 0.5K | 1K | 2K | 4K | 输出图单价 |
|---|---|---|---|---|---|
| Gemini 3.1 Flash Image | 747 | 1120 | 1680 | 2520 | $60/1M（0.5K $0.045、1K $0.067、2K $0.101、4K $0.151）|
| Gemini 3 / 3.1 Pro Image | — | 1120 | 1120 | 2000 | $120/1M（1K/2K $0.134、4K $0.24）|

用户价目「1K 与 2K 同价、只区分 4K」与 Pro 的 token 表一致。tuzi 经 OpenAI 兼容 chat 路径实测回过 `completion_tokens_details.image_tokens: 1120`（旧调查 §2.2）；`/v1/images/generations` 路径无 usage。

---

## C. 计费安全边界

| 约束（`AGENTS.md`） | 现状 | 本方案触点 |
|---|---|---|
| 用户可控乘数在校验层设界 | `n`：JSON `valid_request.go:249-251`、multipart `:199-201`，上界 `dto.MaxImageN=128`（`relaykit/dto/openai_image.go:15`） | `float(param("n") ?? 1)` 已被 128 封顶；最坏 `350000/1e6 × 500000 × 128 = 22.4M quota` ≪ `MaxQuota=MaxInt32`（`common/quota_math.go:14`） |
| `size` 作为计费输入 | 零校验 | 表达式里 `size` 只用来**选常量档位**、不是乘数，`int()` 溢出也只是落最高档（实跑 `999999999999x1` → 4k），不会产生负数或 overflow。多发怪串最多让自己落到兜底档 |
| quota 转换走 `common/quota_math.go` | 预扣 `QuotaRoundStrict`（`price.go:323`）、结算 `QuotaRoundChecked` + `noteQuotaClamp`（`settle.go:28`、`tiered_settle.go:225`）、审计 `attachQuotaSaturation`（`text_quota.go:561`） | 不新增转换点 |
| `AddOtherRatio` 守卫 | `types/price_data.go:116-118` | 不写 `OtherRatios` |
| 负数 / 免费 | 常量叶子为正；`QuotaRoundChecked` 对负值钳到 `MinQuota` | 唯一风险是 0.7 的「算出 0 → 免费」 |
| 验证绕过（multipart / passthrough） | passthrough 只影响发上游的 body（`image_handler.go:49-54`），DTO 校验已先跑 | multipart 下 `param()` 为 nil → 兜底档（拿不到值，不是被绕过成更便宜） |

---

## D. 路线结论

### 路线 A（纯配置，**推荐，已验证可行**）

- 改动：0 行代码。后台「定价 → 模型定价 → 编辑 → 计费模式：表达式编辑器 → Raw」粘 0.4 的表达式；FlowAPI 与下游网关粘同一份。
- 缺口（PRD 拍板接受）：FlowAPI 当前版本 multipart 编辑图落兜底档；张数按声明 `n`；定价页常量档位显示单价 0；保存无校验。
- 上线检查：每个模型用测试令牌各打 1K/2K/4K（及 `auto`）一次，看日志 `matched_tier` 与扣费额；Gemini 另看有 usage 与无 usage 两种入口。

### 路线 B（小改代码，**备选，仅当 A 的 multipart 缺口不可接受**）

1. `relay/helper/billing_expr_request.go:24-34`：body 为 nil 且 `info.Request` 是 `*dto.ImageRequest` 时，用同文件 `:37-51` 的 `BuildBillingExprRequestInputFromRequest(info.Request, info.RequestHeaders)`（`ImageRequest.MarshalJSON` `openai_image.go:74-97` 只输出已知字段）。≈10 行 + `billing_expr_request_test.go` 一个用例。上游已用更严格的形状实现（`ResolveImageBillingRequestInput` 只放 `model/n/size/quality/parameters`），照抄上游形状更稳。
2. （可选）`relay/helper/valid_request.go` 给 gpt-image-* 的 `size` 加 `^\d+x\d+$` + 边 ≤3840 校验（JSON `:249-251` 之后、multipart `:205` 之后），≈15 行 + `openai_image_request_test.go` 表驱动用例。
- 只改 FlowAPI，下游网关不受益；若下游是新版上游则它本来就不需要。

### 路线 C（`ImagePriceRatio` 档位化 + 配置 + 前端 UI，~200 行）

上一版被判过度设计的方案，且只对固定价模式生效、与 `tiered_expr` 互斥、Gemini 的 `img_o` 分档做不了。**不建议。** 按实际出图字节判档同样不建议（需改三条响应路径、`url` 形态无字节、上游按请求档位而非实际像素定价）。

---

## Related Specs

- `pkg/billingexpr/expr.md` — 表达式语言与架构（AGENTS.md 强制前置）。其「保存时 Compile + Smoke test」描述与当前代码不符（0.3）。上游版本已扩展 `fixed()` / `image_count` / `u()` 章节（0.8）。
- `.trellis/spec/backend/billing-ratio-routing.md` — 分组/渠道倍率在预扣、重试、结算间的一致性；tiered snapshot 的 group 刷新（`service/tiered_settle.go:99-159`）是其实现。
- `AGENTS.md` "Billing safety invariants" — C 节逐条对照。

## Caveats / Not Found

1. 现网 6 个模型当前的 `billing_mode` / `billing_expr` 值在生产 `options` 表，仓库内查不到；Gemini Pro 已是 tiered 仅由日志形态推断。
2. Gemini 请求侧分辨率参数的真实字段、各模型真实 `img_o` 数值、`/v1/images/generations` 入口是否有 usage —— 由 live probe 覆盖，本文 0.4 的探针顺序与阈值需据此收敛。
3. 站点货币展示模式：team-lead 已确认 ¥:$=1:1，本文按此写系数；若为 CNY 模式（`setting/operation_setting/general_setting.go:76-91` 按 `USDExchangeRate=7.3`）则系数要除以汇率。
4. `options.value` 的实际列类型未在生产库核对，按 GORM 驱动默认推断为 `longtext`/`text`。
5. 本轮未起服务实跑 `/v1/images/*` 端到端；链路结论来自静态阅读 + 既有测试，表达式行为来自对真实 `pkg/billingexpr` 的实跑（`ComputeTieredQuotaWithRequest`，与结算同一函数）。
6. 下游网关的 new-api 版本未知；0.8 的可移植性结论基于「两端都有 `param()`」这一前提。
