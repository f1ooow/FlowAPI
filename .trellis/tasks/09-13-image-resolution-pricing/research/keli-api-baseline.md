# Research: keli api 代码基线是否具备表达式计费系统

> **结论（一句话）：keli api 有完整的表达式计费系统，且与 FlowAPI 是同一代实现；FlowAPI 定稿的 6 条表达式在 keli api 的引擎上原样可用 —— 已用 keli 的依赖版本（expr-lang v1.17.8 + gjson v1.18.0）和逐字复制的编译/运行环境实跑 28 个用例，0 失败，档位与金额与 FlowAPI 侧验证结果逐个相等。可以直接在 keli api 上验证，无需移植。**

- **Query**: 确认 keli api（`f1ooow/keli-api`，本地副本 `/Users/Zhuyu/Documents/Code/new-api`，分支 `feature/fulladaptor`）有没有 `tiered_expr` 表达式计费；与 FlowAPI 的差距、分叉程度、图像链路现状
- **Scope**: internal（双仓库只读逐文件 diff + 在 `/tmp` 隔离模块内实跑表达式；**未修改 keli api 仓库任何文件，未执行任何 git 写操作，未连生产服务器/数据库**，临时模块已清理）
- **Date**: 2026-09-13
- **基线**: keli api `feature/fulladaptor@ba25f9bd6`（2026-09-10）；FlowAPI `main@9794c7363`
- **只读证明**: 调查前后 `git status --porcelain` 均只有 `?? .ace-tool/`、`?? web/classic/bun.lock` 两条既有未跟踪项，`HEAD` 仍为 `ba25f9bd6`

---

## A. 表达式计费系统是否存在 —— 全部存在

| # | 要素 | keli api | 证据 |
|---|---|---|---|
| 1 | `pkg/billingexpr/` 目录 | **在** | `compile.go` / `run.go` / `settle.go` / `types.go` / `round.go` / `expr.md` / `billingexpr_test.go` |
| 1b | `expr.md` | **在** | `pkg/billingexpr/expr.md`（12,604 字节；FlowAPI 14,999 字节） |
| 2 | `setting/billing_setting/tiered_billing.go`、`tiered_expr` 常量 | **在** | `setting/billing_setting/tiered_billing.go:13` `BillingModeTieredExpr = "tiered_expr"`；该文件与 FlowAPI **字节级完全相同** |
| 3 | `relay/helper/billing_expr_request.go` | **在** | `ResolveIncomingBillingExprRequestInput`:13、`BuildBillingExprRequestInputFromRequest`:37、`readIncomingBillingExprBody`:53 —— 三个函数全在，行号与 FlowAPI 一致 |
| 4 | `service/tiered_settle.go`、`BuildTieredTokenParams`、`TryTieredSettle` | **在** | `service/tiered_settle.go:21` `BuildTieredTokenParams`、`:95` `TryTieredSettle` |
| 5 | `relay/helper/price.go` 的 `modelPriceHelperTiered` 分支 | **在** | `relay/helper/price.go:73-74` 分流、`:241` 函数定义 |

补充：结算侧也已完整接线 —— `service/quota.go:161`、`service/quota.go:286`、`service/text_quota.go:341` 三处调用 `TryTieredSettle`；`service/log_info_generate.go:271` `InjectTieredBillingInfo` 写日志；渠道测试路径 `controller/channel-test.go:525,533,537` 也走表达式。

keli api 上表达式计费的落地时间早于分叉：`git log -- pkg/billingexpr/` 最早一笔为 `91ed4e196`（2026-03-16 `feat: implement tiered billing expression evaluation and related functionality`），最新一笔 `8d87d5fd8`（2026-06-17）。

### 前端配套也在

`web/default/src/features/system-settings/models/tiered-pricing-editor.tsx`（1878 行）与 FlowAPI 的 `web/src/features/system-settings/models/tiered-pricing-editor.tsx`（1878 行）**除一行注释外逐字相同**：

```diff
-    // shape. Mirrors the classic editor's UX for adding tiers.
+    // shape with an immediately useful fallback.
```

模型定价页、用量日志的 `tiered_expr` 展示同样齐备（`web/default/src/features/usage-logs/lib/format.ts:256`、`.../model-pricing-snapshots.ts:79`）。另有一套旧前端 `web/classic`（`pages/Setting/Ratio/components/ModelPricingEditor.jsx:417` 的 `<Radio value='tiered_expr'>表达式/阶梯计费`），两套都会构建（`Dockerfile:39-40`）。

**B2（移植成本评估）不适用** —— 系统已存在，无需移植。

---

## B. 两仓库表达式系统的逐项 diff

### B1.1 6 条定稿表达式在 keli 引擎上是否全部可用 —— 是，已实跑

**依赖版本是决定性因素，两仓一致**：

| 依赖 | keli api | FlowAPI |
|---|---|---|
| `github.com/expr-lang/expr` | **v1.17.8** | **v1.17.8** |
| `github.com/tidwall/gjson` | v1.18.0 | v1.19.0 |
| `go` directive | 1.25.1 | 1.25.1 |

6 条表达式用到的 `let`、`matches`、`split`、`int`、`float`、`string`、`upper`、`trim`、`??`、`in` 全部是 expr-lang 内置语法/函数（`let` 需 ≥1.16），两仓同为 v1.17.8 → 全部可用。`tier`、`param`、`max` 来自 env，两仓定义相同（见 B1.2）。

**变量与函数集逐项对比**（keli `pkg/billingexpr/compile.go:124-150` vs FlowAPI `compile.go:123-160`）：

两仓 env 原型完全一致：`p, c, len, cr, cc, cc1h, img, img_o, ai, ao, tier, header, param, has, hour, minute, weekday, month, day, max, min, abs, ceil, floor`。FlowAPI 多出的只有两个**内部保留标识符** `_trace` / `_trace_int`（请求规则追踪用，且 `extractUsedVars` 会把它们排除）。运行期 env（keli `run.go:55-101` vs FlowAPI `run.go:56-120`）同理：`param()` 的实现（`gjson.GetBytes` → `result.Exists()` → `result.Value()`）、`tier()` 的实现、`max = math.Max` 全部逐字相同。

> FlowAPI 的 `requestRulePatcher` 只改写「条件里含 `param`/`header`/时间函数、且两个分支都是数字字面量、else 分支恰为 `1`」的三元表达式。6 条表达式的三元分支都是 `tier(...)` 调用或字符串，**不触发 patch**，因此两仓行为必然一致。

**实跑验证**（`/tmp` 隔离模块，逐字复制 keli 的 `compileEnvPrototypeV1` 与 `runProgram` env，锁定 keli 的依赖版本；quota 换算用 keli `settle.go:9` + `round.go` 的 `int(math.Round(...))`）：

```
expr-lang v1.17.8 + gjson v1.18.0 (keli-api go.mod versions)
QuotaPerUnit=500000  GroupRatio=1

CASE                                                       | TIER   |        RAW |    QUOTA | CNY(1:1)
------------------------------------------------------------------------------------------------------
A gpt-image-2 size=1024x1024 n=1                           | 1k     |     120000 |    60000 | 0.1200
A gpt-image-2 size=1536x1024 n=1                           | 1k     |     120000 |    60000 | 0.1200
A gpt-image-2 size=2560x1440 n=1                           | 2k     |     120000 |    60000 | 0.1200
A gpt-image-2 size=2048x2048 n=1                           | 4k     |     210000 |   105000 | 0.2100
A gpt-image-2 size=auto                                    | 1k     |     120000 |    60000 | 0.1200
A gpt-image-2 no size no n                                 | 1k     |     120000 |    60000 | 0.1200
A gpt-image-2 size=2048x1152 n=4 (multipart projection)    | 2k     |     480000 |   240000 | 0.4800
A gpt-image-2 n=0 (guard)                                  | 1k     |     120000 |    60000 | 0.1200
A gpt-image-2 size=null                                    | 1k     |     120000 |    60000 | 0.1200
A EMPTY BODY (multipart today)                             | 1k     |     120000 |    60000 | 0.1200
B flare size=1024x1024                                     | 1k     |     120000 |    60000 | 0.1200
B flare size=1536x1024 (2K per upstream)                   | 2k     |     160000 |    80000 | 0.1600
B flare size=2048x1152 n=4                                 | 2k     |     640000 |   320000 | 0.6400
B flare size=2880x2880                                     | 4k     |     210000 |   105000 | 0.2100
D gemini native snake.snake 4K                             | 4k     |     200000 |   100000 | 0.2000
D gemini native snake.camel 2K                             | 2k     |     150000 |    75000 | 0.1500
D gemini native camel.camel 4k lowercase                   | 4k     |     200000 |   100000 | 0.2000
D gemini native camel.snake 1K                             | 1k     |     150000 |    75000 | 0.1500
D gemini extra_body 4K                                     | 4k     |     200000 |   100000 | 0.2000
D gemini size literal 2K                                   | 2k     |     150000 |    75000 | 0.1500
D gemini size area 2048x2048 -> 4K                         | 4k     |     200000 |   100000 | 0.2000
D gemini size area 1536x1024 -> 1K                         | 1k     |     150000 |    75000 | 0.1500
D gemini imageSize=512px (non-enum) + size=2560x1440       | 2k     |     150000 |    75000 | 0.1500
D gemini nothing -> 1K                                     | 1k     |     150000 |    75000 | 0.1500
D gemini n=3 with 4K                                       | 4k     |     600000 |   300000 | 0.6000
D gemini EMPTY BODY                                        | 1k     |     150000 |    75000 | 0.1500
E gemini31 4K n=2                                          | 4k     |     700000 |   350000 | 0.7000
E gemini31 default                                         | 1k     |     300000 |   150000 | 0.3000

compiled expressions: 4, failures: 0
```

- 4 条唯一表达式（A / B=C / D / E=F）全部编译通过，28 个用例全部运行成功。
- 档位与金额与 `final-expressions-verification.md` 第 6 版逐个相等（例：flare `2048x1152 n=4` → 2k ¥0.64；gpt-image-2 `2048x2048` → 4k ¥0.21；gemini `imageSize=512px` 非枚举时回落看 `size` 面积 → 2k）。
- gjson v1.18.0 对 `generationConfig.image_config.image_size`、`extra_body.google.image_config.image_size` 这类点号深路径解析正常，五段探测链全部命中 —— gjson 版本差异不构成风险。
- 空 body（今天 multipart 的现状）在两仓同样落 1K，行为一致。

### B1.2 计费换算公式是否一致 —— 完全一致

| 环节 | keli api | FlowAPI | 结论 |
|---|---|---|---|
| 版本换算 | `pkg/billingexpr/settle.go:9` `exprOutput / 1_000_000 * snap.QuotaPerUnit` | `settle.go:11` 同一行 | **相同** |
| `QuotaPerUnit` 默认值 | `common/constants.go:62` `500 * 1000.0` | `common/constants.go:22` `500 * 1000.0` | **相同（500000）** |
| 分组倍率 | `settle.go:26` `QuotaRound(quotaBeforeGroup * snap.GroupRatio)` | `settle.go:28` `common.QuotaRoundChecked(...)` | 数值相同，FlowAPI 多一个饱和标记 |
| 预扣公式 | `relay/helper/price.go:267-268` `rawCost / 1_000_000 * common.QuotaPerUnit` → `billingexpr.QuotaRound(× GroupRatio)` | `price.go` 同口径，改用 `QuotaRoundStrict` | 数值相同 |
| 结算取冻结 body | `service/tiered_settle.go:101-104` 读 `relayInfo.BillingRequestInput` | 同 | **相同** |

即 FlowAPI 的 `quota = 表达式输出 / 1e6 × 500000 × 分组倍率` 在 keli api 上**逐字成立**，`tier(…, 120000)` → 60000 quota 的换算不变。

### B1.3 `usd_exchange_rate=1` + `quota_display_type=CNY` 会不会让同一条表达式算出不同金额 —— 不会

这两个设置**只影响展示字符串，不进入计费算术**：

- `logger/logger.go:122-147` `LogQuota` / `logger.go:150-175` `FormatQuota`：拿到已经算好的 `quota int`，再按 `QuotaDisplayType` 格式化；CNY 分支是 `quota / QuotaPerUnit * USDExchangeRate`。
- `USDExchangeRate` 只有一处定义（`setting/operation_setting/payment_setting_old.go:18`，默认 7.3），其余引用集中在 `controller/billing.go`、`controller/topup*.go`、`controller/misc.go:95`、`controller/subscription_payment_creem.go` —— 全是展示与充值换算。
- 表达式链路（`price.go:267-268` → `settle.go:9` → `QuotaRound`）不引用任何展示设置。

结论：同一条表达式在 keli api 与 FlowAPI 上**扣减的 quota 数值完全相同**。`usd_exchange_rate=1` + `CNY` 的效果是把 `quota/500000` 直接当成 ¥ 显示，与 FlowAPI「站点 ¥:$=1:1」的口径一致，所以 `tier(…,120000)` 在两边都显示 ¥0.12。

> 需要分清的一点：`usd_exchange_rate` 会影响**充值时每元换多少 quota**（`controller/topup.go`），不影响**每次请求扣多少 quota**。若两站的充值汇率不同，同样的 quota 对用户的实际人民币成本不同 —— 这是定价口径问题，不是表达式问题。

### B1.4 FlowAPI 有而 keli api 没有的部分（不影响本次验证）

| FlowAPI 独有 | 位置 | 对本次验证的影响 |
|---|---|---|
| 请求规则追踪 `_trace` / `_trace_int`、`RequestRuleTrace` | `compile.go:19-20,34-100`、`run.go:67-84`、`types.go:31-36` | 无。6 条表达式不触发 patch（分支不是数字字面量） |
| quota 饱和体系 `common/quota_math.go`（`QuotaRoundChecked`/`QuotaRoundStrict`/`QuotaClamp`） | keli **无此文件**；keli `round.go:9` 仍是 `int(math.Round(f))` | 无。本次金额量级远低于 int32 边界 |
| `TieredResult.Clamp` + `attachQuotaSaturation` 审计 | `types.go:70`、`service/log_info_generate.go` | 无 |
| 切组重扣 `PrepareBillingForSelectedRoute` / `PrepareTieredBillingForSelectedGroup` | `service/tiered_settle.go:99-175` | 无（单组验证场景） |
| `CacheCreationTokensTotal()` 语义 | `service/tiered_settle.go:31` | 无（图像请求无 cache token） |
| `types.PriceData.BillingRatios` / `ApplyOtherRatiosToFloat` | `relay/helper/price.go`、`relaykit/dto/openai_image.go:157-171` | 无（`tiered_expr` 走 `modelPriceHelperTiered`，不读 `BillingRatios`；见 D2 关于 `n` 不会重复计费的论证） |
| `dto.MaxImageN = 128` | `relaykit/dto/openai_image.go:15` | **有影响，见 D2 / 风险项** |
| `pkg/billingexpr/settle_clamp_test.go` | keli 无 | 无 |

keli api `types/price_data.go:30-38` 的 `AddOtherRatio` 只挡 `ratio <= 0`，不挡 NaN/+Inf（FlowAPI 已加固）。与本次表达式验证无关，记录备查。

---

## C. 两仓库的分叉程度

### C1. 共同祖先

| 仓库 | 与上游 `QuantumNous/new-api` 的 merge-base | 时间 | 领先提交数 |
|---|---|---|---|
| keli api `feature/fulladaptor` | `52858ad1e` *feat: support Wan2.7 i2v media mapping (#4984)* | **2026-07-01** | 44 |
| FlowAPI `main` | `e468b7391` *docs: update PR template and remove PR Check workflow (#7053)* | **2026-08-27** | 23 |

两者共同祖先即 keli 的 merge-base `52858ad1e`（2026-07-01，因为 FlowAPI 的 base 更新）。**上游基线相差约 2 个月**：keli 的 base 之后上游已有 324 个提交，FlowAPI 已包含其中大部分。

### C2. 目录差异

| 目录 | keli api | FlowAPI | 说明 |
|---|---|---|---|
| `relaykit/` | **不存在** | 存在（独立 Go module） | FlowAPI 把 dto/types/转换层抽成了独立模块，这是最大的结构性分叉 |
| `relay/helper/` | 11 个 `.go` | 15 个 `.go` | FlowAPI 多 `stream_gate.go` + 3 个测试；`price.go` 306 vs 365 行 |
| `relay/common/` | 11 个 | 15 个 | FlowAPI 多 `tool_usage.go` 等 |
| `service/` | 59 个 | 82 个 | FlowAPI 多 auth session/token、http transport policy、codex models 等 |
| `common/` | 无 `quota_math.go` | 有 | 见 B1.4 |
| `web/` | `web/default` + `web/classic` 双前端 | 只有 `web/src` | FlowAPI 已删除 classic；`web/default` ≙ FlowAPI `web/src` |

`relay/helper/price.go` 的差异最能说明分叉方向：FlowAPI 把 `types` 改成了 `relaykit/types` + `hosttypes`，重写了 `HandleGroupRatio`（引入 `BaseGroupRatio`/`UserGroupRatio`/`ChannelRatio`/`IncludeChannelRatio`），并加了 `PreConsumeQuotaBeforeGroup`、`BillingRatios` → `AddOtherRatio` 的图像倍率通道。

### C3. multipart 改动能不能 cherry-pick —— **能，几乎逐字，只需改 1 行 import**

这是本次调查最有利的一点。改动目标文件 `relay/helper/billing_expr_request.go` 在两仓**除 import 路径外逐字相同**（两仓均 91 行，函数体、行号全部对齐）：

```diff
--- new-api/relay/helper/billing_expr_request.go
+++ FlowAPI/relay/helper/billing_expr_request.go
@@ -4,9 +4,9 @@
 	"strings"
 
 	"github.com/QuantumNous/new-api/common"
-	"github.com/QuantumNous/new-api/dto"
 	"github.com/QuantumNous/new-api/pkg/billingexpr"
 	relaycommon "github.com/QuantumNous/new-api/relay/common"
+	"github.com/QuantumNous/new-api/relaykit/dto"
 	"github.com/gin-gonic/gin"
 )
```

`multipart-billing-fix-design.md` §2 的那约 12 行插桩点（`ResolveIncomingBillingExprRequestInput` 里 `readIncomingBillingExprBody` 之后）在 keli 是 `billing_expr_request.go:29-34`，上下文行完全一致。它依赖的三个符号 keli 全部具备且签名相同：

| 依赖符号 | keli 位置 | 是否同名同签名 |
|---|---|---|
| `info.Request dto.Request` | `relay/common/relay_info.go:171`（赋值在 `:466` `GenRelayInfoImage`） | 是 |
| `info.RequestHeaders` | `relay/common/relay_info.go:485` `cloneRequestHeaders(c)` | 是 |
| `info.BillingRequestInput` | `relay/common/relay_info.go:169` | 是 |
| `dto.ImageRequest`（含 `Prompt`/`Image`/`Images`/`Mask`/`Extra`） | `dto/openai_image.go:14` | 是（字段集与 FlowAPI `relaykit/dto/openai_image.go` 相同） |
| `BuildBillingExprRequestInputFromRequest` | `relay/helper/billing_expr_request.go:37` | 是 |
| 调用方 `modelPriceHelperTiered` → `ResolveIncoming…` | `relay/helper/price.go:252` | 是（FlowAPI 为 `price.go:307`，仅行号不同） |
| 结算复用冻结 body | `service/tiered_settle.go:101-104` | 是 |

**唯一的写法调整**：在 keli 里 import 写 `"github.com/QuantumNous/new-api/dto"`（该文件已经 import 了，无需新增）。也就是说，把 FlowAPI 的 patch 拿到 keli 上，`git cherry-pick`/`git apply` 大概率零冲突；即使有冲突也只在 import 块。

反向（keli 验证通过后同步回 FlowAPI）同理，只需把 `dto` 换成 `relaykit/dto`。

---

## D. 图像模型现状

### D1. 代码默认配置里有哪些图像模型

| 模型 | keli api | FlowAPI | 证据 |
|---|---|---|---|
| `gpt-image-1` / `-1-mini` / `-1.5` / `chatgpt-image-latest` | **有** | 有 | keli `relay/channel/openai/constant.go:68-69`；倍率 `setting/ratio_setting/model_ratio.go:67,341,664` |
| `gpt-image-2` | **无** | **无** | 两仓代码默认配置里都查不到（grep `gpt-image-2` 在 `setting/`、`constant/`、`relay/`、`relaykit/` 全为空） |
| `gpt-image-2.5-flare` / `-sunburst` | **无** | **无** | 同上（grep `flare`/`sunburst` 全为空） |
| `gemini-3-pro-image-preview` | **有** | 有 | keli `setting/model_setting/gemini.go:30`、`relay/channel/gemini/constant.go:16` |
| `gemini-2.5-flash-image` | **有** | 有 | keli `setting/model_setting/gemini.go:31`、`relay/channel/gemini/constant.go:13` |
| `gemini-3.1-flash-image-preview` | **有** | 有 | keli `setting/model_setting/gemini.go:32`、`relay/channel/gemini/constant.go:17` |
| `gemini-3-pro-image`（非 preview） | **无** | 有 | FlowAPI `setting/model_setting/gemini.go:45` |
| `gemini-3.1-flash-image`（非 preview） | **无** | 有 | FlowAPI `setting/model_setting/gemini.go:47` |

要点：`gpt-image-2` 系三个模型在**两个仓库的代码里都不存在**，它们是通过数据库/渠道配置接入的自定义模型名。因此 keli api 能不能跑这三个模型，取决于广州实例上配了什么渠道，代码层面无阻碍（`tiered_expr` 按模型名配置，不要求模型在代码常量表里）。gemini 侧 keli 少两个非 preview 别名，如果要验证表达式 D/F 对应的模型名，需确认生产上用的是 `-preview` 还是非 preview 名。

### D2. 图像链路是否与 FlowAPI 同构 —— 同构

| 环节 | keli api | FlowAPI |
|---|---|---|
| 路由 | `router/relay-router.go:112` `/images/generations`、`:115` `/images/edits`、`:160` `/images/variations` → `RelayNotImplemented` | 同 |
| 请求解析 + 校验 | `relay/helper/valid_request.go:143` `GetAndValidOpenAIImageRequest`；multipart 分支 `:148-192` | `relay/helper/valid_request.go:182`，multipart 分支 `:186-232` |
| multipart 解析 | `:149` `common.ParseMultipartFormReusable(c)`，读 `prompt/model/n/quality/size/stream/image/watermark` | 同一套字段 |
| DTO 挂载 | `relay/common/relay_info.go:423` `GenRelayInfoImage` → `:466` `Request: request` | 同 |
| 定价 | `relay/helper/price.go:73-74` → `:241` `modelPriceHelperTiered` → `:252` `ResolveIncomingBillingExprRequestInput` → `:257` `RunExprWithRequest` → `:293-294` 冻结 snapshot + `BillingRequestInput` | 同（FlowAPI 行号 `:307/:312/:351-352`） |
| 转发 | `relay/image_handler.go:23` `ImageHelper`（162 行） | 同名同结构 |
| 结算 | `relay/image_handler.go:160` `service.PostTextConsumeQuota` → `service/text_quota.go:341` `TryTieredSettle` | 同 |

**关键确认：`n` 不会重复计费。** keli `relay/image_handler.go:130` 的 `n` 倍率注入被 `if info.PriceData.UsePrice` 包住；而 `modelPriceHelperTiered`（`price.go:296-300`）构造的 `PriceData` 只填 `FreeModel`/`GroupRatioInfo`/`QuotaToPreConsume`，`UsePrice` 为零值 `false`。所以 `tiered_expr` 模型不会走 `AddOtherRatio("n", …)`，张数只能由表达式里的 `* max(float(param("n") ?? 1), 1.0)` 提供 —— 与 FlowAPI 的设计假设一致，无双计。

**现状与 FlowAPI 相同的缺陷**：keli `readIncomingBillingExprBody`（`billing_expr_request.go:53-62`）同样只认 `application/json`，multipart 请求下 `param()` 全为 nil。上面实跑表里 `A EMPTY BODY` / `D gemini EMPTY BODY` 两行就是这个现状 —— 一律落 1K、张数恒按 1。这正是待验证的改动要解决的问题。

---

## 风险与差异提示

1. **keli 的 multipart `n` 没有上界**（唯一一处功能性差异，且与本改动强相关）。
   - keli `relay/helper/valid_request.go:161`：`imageRequest.N = common.GetPointer(uint(common.String2Int(formData.Get("n"))))`，随后 `:176-178` 只把 nil/0 归一为 1，**没有上界检查**。`common.String2Int`（`common/str.go:84-90`）在解析失败时返回 0，但 `"99999999999"` 这种合法大整数会原样通过。
   - FlowAPI `valid_request.go:197-203` 已加 `n < 0 || n > dto.MaxImageN`（`MaxImageN = 128`，`relaykit/dto/openai_image.go:15`）并返回 400。
   - 今天这个缺口对 `tiered_expr` 无害（`param("n")` 读不到）。**但一旦把 multipart 投影改动移到 keli 上，`param("n")` 就活了，表达式的 `* max(float(param("n") ?? 1), 1.0)` 会把一个无上界的用户可控值变成计费乘数。** 按 `AGENTS.md`「用户可控乘数必须在校验层设界」，移植时应把 `MaxImageN` 校验一并带过去（keli 的 `dto/openai_image.go` 目前没有 `MaxImageN` 常量，需要新增）。
2. **gemini 模型名**：keli 只有 `-preview` 后缀的别名，缺 `gemini-3-pro-image` / `gemini-3.1-flash-image`。若要在 keli 上验证表达式 D/F，用生产实际配置的模型名即可（`tiered_expr` 按模型名字符串匹配，不校验模型是否在代码常量表内）。
3. **双前端**：keli 同时构建 `web/default`（`makefile:1` `FRONTEND_DIR = ./web/default`）与 `web/classic`。表达式编辑器两套都有，但 `web/classic` 是旧版 JSX 实现，与 FlowAPI 的 `web/src` 不同源。验证时注意实际访问的是哪套前端。
4. **keli 缺 quota 饱和/审计体系**（无 `common/quota_math.go`、`QuotaRoundStrict`、`QuotaClamp`）。本次金额量级用不到，但若验证期间构造极端 `n`/尺寸，keli 不会像 FlowAPI 那样在日志里留下 `quota_saturation` 标记。
5. **前端「Token 估算器」报错**：`no-let-expressions.md` 记录的「含 `param()` 的表达式在估算器里必然报错、但不阻断保存」的结论同样适用于 keli —— 编辑器组件两仓逐字相同。后端也没有保存前的强制校验（`SmokeTestExpr` 在两仓都**无任何调用方**，是导出但未接线的死代码）。

## Caveats / 查不到

- **没有连生产实例，也没有查数据库**。广州实例 `api.keliagent.cn` 上实际配了哪些模型、`billing_setting.billing_mode` / `billing_expr` 里已有哪些条目、`usd_exchange_rate` 与 `quota_display_type` 的实际取值，本文均未核实 —— 本文只给「代码基线支持什么」，不给「线上现在配了什么」。B1.3 的结论是「这两个设置不进计费算术」，与它们的具体取值无关，所以该结论不受此限制影响。
- **没有在 keli 仓库内跑 `go build` / `go test`**（避免任何写操作，包括 `go.sum` 变更与构建缓存写入）。表达式验证是在 `/tmp` 隔离模块内、用逐字复制的 keli 引擎代码 + keli 的依赖版本完成的，已覆盖编译与运行两个阶段；但「keli 主仓整体编译通过」未单独验证（其 HEAD 是已部署的生产分支，可认为本来就通过）。
- **`relay/helper/price.go` 的完整 diff 未逐行列出**（306 vs 365 行，差异集中在 `relaykit` 拆分与分组倍率重写）。本文只对比了与表达式计费相关的 `modelPriceHelperTiered` 全文与 `ModelPriceHelper` 的分流点。
- 实跑用例覆盖了 A/B/D/E 四条唯一表达式；C 与 B、F 与 E 在定稿文档中声明为逐字相同，未重复跑。

## Related

- `/Users/Zhuyu/Documents/Code/FlowAPI/.trellis/tasks/09-13-image-resolution-pricing/research/final-expressions-verification.md` — 6 条定稿表达式与 FlowAPI 侧实跑基准（本文 diff 基准）
- `/Users/Zhuyu/Documents/Code/FlowAPI/.trellis/tasks/09-13-image-resolution-pricing/research/multipart-billing-fix-design.md` — 待移植的约 12 行改动设计
- `/Users/Zhuyu/Documents/Code/FlowAPI/.trellis/tasks/09-13-image-resolution-pricing/research/billing-chain.md` — FlowAPI 侧完整调用链
- `/Users/Zhuyu/Documents/Code/FlowAPI/.trellis/tasks/09-13-image-resolution-pricing/research/no-let-expressions.md` — 前端估算器报错但不阻断保存的证据
