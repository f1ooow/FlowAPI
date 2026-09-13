# 技术设计 — 图像模型按分辨率阶梯计费

## 问题陈述

计费表达式通过 `param(path)` 读取入站请求体来判断分辨率档位。但 `relay/helper/billing_expr_request.go:53-62` 的 `readIncomingBillingExprBody` 对非 `application/json` 的 Content-Type 直接返回 nil body，导致 multipart 请求的 `param()` 全部取不到值。

影响面：`/v1/images/edits` 走 multipart，占 `gpt-image-2` 全部请求的 **75%**（30 天 305/405 次）、`gpt-image-2.5-flare` 的 45%。这些请求无论客户端传了什么 `size`，一律落兜底档。

而同一个 `size` **会被正常转发给上游**（DTO 解析正常、日志也记录了），上游按它分档收费。于是形成单向亏损：

| 模型 | 编辑请求 size | 条数 | 上游收 | 我方落兜底档收 | 盈亏 |
|---|---|---:|---:|---:|---:|
| flare | `2048x1152` / `2560x1440` | 31 | ¥0.16 | ¥0.12 | **−0.04** |
| gpt-image-2 | `2048x1152` | 21 | ¥0.12 | ¥0.12 | 0 |
| gpt-image-2 | `2880x2880` | 4 | ¥0.21 | ¥0.12 | **−0.09** |

数据来源：上游 tuzi 账单导出（`usage-logs-20260913233235.csv`，61 条编辑请求）。

> 当前全部流量的 `billing_source` 都是 `unlimited`，**尚无真实资金损失**；这是切换到真实计费后的敞口。

## 方案选型

| 方案 | 说明 | 结论 |
|---|---|---|
| A. 表达式内兜底档 | 用 `has(header("content-type"), "multipart/form-data")` 识别 multipart，给它固定档位 | 零代码但不精确。兜底 2K 仍有 4/61 亏损；兜底 4K 则 57/61 被多收。**不采用** |
| B. **补齐数据通路** | 让 multipart 请求的表达式读到已解析的 DTO 参数 | **采用**。判档与 JSON 路径完全一致，无近似 |
| C. 整体同步上游 09-09 补丁 | 合入上游 `ResolveImageBillingRequestInput` | 改动面大，且它只在表达式引用 `image_count` 时才投影，覆盖不到本场景。**不采用** |

用户最初要求纯配置（下游网关需同步），后确认下游同为己方部署、允许改代码，要求「用最优雅的方案」。方案 B 是一次性的基础设施修复：**计费规则仍 100% 在表达式配置中**，后续调价、加模型、改阈值均无需再动代码。

## 改动设计

### 注入点

`relay/helper/billing_expr_request.go` 的 `ResolveIncomingBillingExprRequestInput` —— 全仓库唯一的生产调用点。

调用链（证据见 `research/multipart-billing-fix-design.md`）：

```
controller/relay.go:121  GetAndValidateRequest
                         → valid_request.go:186-232 解析 multipart 成 dto.ImageRequest
                           （n 校验 0..128，nil/0 归一为 1）
controller/relay.go:132  GenRelayInfo → relay_info.go:511 挂到 info.Request
controller/relay.go:165  ModelPriceHelper
                         → price.go:307  ResolveIncomingBillingExprRequestInput   ← 注入点
                         → price.go:352  冻结进 info.BillingRequestInput
service/tiered_settle.go:208-211  结算只读冻结值
```

**时序成立**：预扣计费发生时 multipart DTO 已解析完成，`Model` / `Size` / `Quality` / `N` / `Stream` / `Watermark` 全部就绪。因注入点唯一且结果被冻结，预扣与结算自动一致，重试、切分组、渠道测试路径均无需改动。

### 逻辑

在 `readIncomingBillingExprBody` 返回空 body 之后插入：若 `info.Request` 断言为 `*dto.ImageRequest`，则拷贝一份 DTO、置空 `Prompt` / `Image` / `Images` / `Mask` / `Extra`，交给已有的 `BuildBillingExprRequestInputFromRequest`（同文件 :37）序列化为 body。

约 12 行，走 `common.Marshal`，不碰 `relaykit/`，不改 JSON 路径（仅在 body 为空时介入）。

### 必须剥字段

`valid_request.go:213-215` 会把 multipart 的文本 `image` 字段放进 `DTO.Image`，客户端可能塞 base64 data URI。实测：

| | body 大小 |
|---|---:|
| 不剥字段 | **201,163 字节** |
| 剥字段后 | **117 字节** |

剥后的投影 body：

```json
{"model":"gpt-image-2.5-flare","n":4,"prompt":"","quality":"high","size":"2048x1152","stream":false,"watermark":true}
```

键名与客户端 JSON 请求的 tag 完全一致，因此同一条表达式在 multipart 与 JSON 两条路径上行为相同。同一条请求在现状下落 1K 档，改动后落 2K 档 × 4 张。

### 字段差异（均在表达式不读取的位置）

- multipart 投影中 `n` 永远是校验后的 1..128，`prompt` 为空，图片字段已剥离
- `response_format` 等表单字段不出现（multipart 分支本就不解析）
- JSON 原始 body 完全不变

## 影响与风险

| 项 | 评估 |
|---|---|
| JSON 请求路径 | 完全不变（仅在 body 为空时介入） |
| 已在生产跑 `tiered_expr` 的模型 | `gemini-3-pro-image-preview` 不受影响（走 JSON 路径） |
| 其他 multipart 请求（音频转写等） | DTO 断言失败 → 仍为 nil body，行为不变 |
| `x-www-form-urlencoded` 的图像请求 | 也会被投影，与 multipart 行为对齐（合理） |
| passthrough 渠道 | 不受影响 |
| `relaykit` 模块独立性 | 不涉及 |
| 与上游 09-09 补丁 | 键集是本方案子集，两者可共存 |

**顺带修复**：multipart 表单里的 `n` 此前表达式读不到，恒按 1 张收费；改动后按真实张数计费。

## 测试方案

遵循 `AGENTS.md` 后端测试要求，表驱动 + `testify/require`。

`relay/helper/billing_expr_request_test.go`：
- multipart 投影字段正确、图片字段被剥离
- `n` 缺省与 `n=0` 的归一结果
- JSON 路径 body 不变
- 音频类 multipart 仍为 nil body
- 冻结值被复用（不重复构建）
- `x-www-form-urlencoded` 行为

`relay/helper/price_test.go`（参照既有 `TestModelPriceHelperTieredUsesPreloadedRequestInput`）：
- multipart `size=2048x1152` + `n=4` → `QuotaToPreConsume = 4×80000`、`EstimatedTier = 2k`
- 同一请求的 JSON 与 multipart 形式结果一致

预估：生产代码 12 行，测试 120 行。

## 表达式配置（不属于代码改动）

6 条表达式见 `research/final-expressions-verification.md` 第 6 版。判档阈值：

| 边界 | 阈值 | 依据 |
|---|---|---|
| 2K → 4K（六条统一） | `px > 3686400` | 上游 flare 表达式原文 `imagePixels(param("size")) > 3686400`，且与 gpt-image-2 账单吻合（`2560x1440` 判 2K、`2048x2048` 判 4K） |
| 1K → 2K（flare / sunburst） | `px > 1048576` | 上游 flare 表达式原文 |
| 1K → 2K（gpt-image-2 / gemini） | `px > 1572864` | 仅影响日志档位名，1K 与 2K 同价 |

## 部署顺序

**FlowAPI 线上有活跃用户，不先动。** 先在下游 keli api（`/Users/Zhuyu/Documents/Code/new-api`，分支 `feature/fulladaptor`，广州 `8.138.196.139`）实施并验证，确认无误后再同步回 FlowAPI。

前置条件已确认（`research/keli-api-baseline.md`）：

| 项 | 结论 |
|---|---|
| `tiered_expr` 系统 | **具备且与 FlowAPI 同代**。`setting/billing_setting/tiered_billing.go` 字节级相同；`billing_expr_request.go` 三个函数齐全；`modelPriceHelperTiered` 在 `relay/helper/price.go:241` |
| 表达式语法 | expr-lang 两仓同为 v1.17.8，6 条定稿表达式在 keli 引擎上实跑 28 用例、档位与金额与第 6 版逐个相等 |
| 换算口径 | 完全一致（`exprOutput/1e6 × QuotaPerUnit(500000) × GroupRatio`，同一行代码同一默认值） |
| 展示设置差异 | `usd_exchange_rate` / `quota_display_type` 仅影响展示格式化（`logger.go:122-175`），不进计费算术 |
| 改动可移植性 | `billing_expr_request.go` 两仓 91 行对 91 行相同，仅 import 路径不同（`dto` vs `relaykit/dto`）；依赖符号全部同名同签名 |

分叉程度：keli 与上游 merge-base 在 2026-07-01，FlowAPI 在 2026-08-27，相差约 2 个月。keli 无 `relaykit/`，前端为 `web/default` + `web/classic` 双套（生产运行 classic）。

### 移植时必须一并处理的安全问题

**keli 的 multipart 分支没有 `n` 上界校验**（`valid_request.go:161`；FlowAPI 有 `dto.MaxImageN=128`）。

当前无害 —— 表达式读不到 `n`。但本改动落地后 `param("n")` 会成为**无上界的用户可控计费乘数**，客户端传 `n=999999` 将被直接乘进账单，并可能使 quota 计算逼近溢出边界。这违反 `AGENTS.md` 的计费安全不变量（「每个成为计费乘数的用户可控量必须在到达 quota 计算前被限制」）。

因此 keli 侧的改动必须包含：multipart 分支补齐 `n` 的 0..128 上界校验，与 JSON 分支及 FlowAPI 对齐。

### 模型名差异

`gpt-image-2` / `-2.5-flare` / `-2.5-sunburst` 在两仓代码里都不存在，是 DB 配置的自定义模型名，代码层面不构成阻碍。gemini 侧 keli 只有 `-preview` 别名，缺 `gemini-3.1-flash-image`（无后缀）；若 keli 需要该模型，需另行在 DB 配置。
