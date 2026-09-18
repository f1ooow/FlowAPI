# Research: keli × FlowAPI 双侧生产日志核对 —— 22 条验证请求的命中档位与扣费

- **Query**: 核对 2026-09-14 00:26–00:35 CST 两批共 22 条图像验证请求，在链路 **keli → FlowAPI → tuzi** 的两端各自的 `matched_tier` 与 `quota`，判定阶梯计费改动是否验收通过
- **Scope**: 只读查询（两侧均只执行 `SELECT`，未做任何 `UPDATE`/`INSERT`/`DELETE`，未改任何配置）
- **Date**: 2026-09-14
- **记录性质**: 以下为当日查询记录；2026-09-18 提交前仅做文档一致性复核，未重新连接生产库。服务器、配置和部署版本均是历史快照。
- **keli 侧**: `root@8.138.196.139`，容器 `postgres`（`postgres:15-alpine`），库 `new-api`，user `root`。分组 `image` 倍率 = **1.5**
- **FlowAPI 侧**: `ssh hk`（野草云-HK，端口 39070），容器 `flowapi-postgres`（`postgres:15-alpine`），库/user `flowapi`。分组 `【职刻】Image` 倍率 = **1**；应用镜像 `flowapi:hk-multipart-billing-20260914-001321`（即含 multipart 投影修复的那版）
- **换算**: 两侧 `QuotaPerUnit` 均为默认 500000、`USDExchangeRate=1`、站点 CNY → **¥ = quota / 500000**

---

## 结论

**22 / 22 在两侧全部符合预期，零偏差。** 三条关键判据全部通过。两侧的档位判定**逐条一致**（22/22 同档），FlowAPI 扣费恰为 keli 的 1/1.5，即 keli 侧多收的部分完全来自分组倍率而非判档差异。两侧均无任何一条扣费为 0。6 个模型的 `billing_mode` 均为 `tiered_expr`，`billing_expr` 在 **FlowAPI、keli、定稿第 6 版三方逐字节相同**。

---

## 双侧逐条对照表

配对依据：两侧日志时间戳**精确到同一秒**，再叠加模型名、`prompt/completion_tokens`、`content` 三重校验，22 条 1:1 无歧义（00:30:20 有三条并发，靠模型名与 token 数区分开）。

`基准 ¥` = FlowAPI 侧金额（倍率 1，即表达式 `tier()` 值折算后的原价）。`keli ¥` = 基准 × 1.5。

### 第一批（request id 前缀 `20260913162957` / `20260913163059`）

| # | 模型 | 请求参数 | 时间 CST | keli rid / tier / quota / ¥ | FlowAPI rid / tier / quota / ¥ | 预期 keli ¥ | 两侧同档 | 符合 |
|---|---|---|---|---|---|---:|:--:|:--:|
| 1 | gpt-image-2 | `1024x1024` | 00:30:38 | `6uXqwOzq3` `1k` 90000 ¥0.18 | `6wIimFABY` `1k` 60000 ¥0.12 | 0.18 | ✅ | ✅ |
| 2 | gpt-image-2 | `2048x2048` | 00:30:40 | `6m8dsVKh1` `4k` 157500 ¥0.315 | `6Cpzyesb7` `4k` 105000 ¥0.21 | 0.315 | ✅ | ✅ |
| 3 | flare | `1024x1024` | 00:30:19 | `6fpRbZuaH` `1k` 90000 ¥0.18 | `6TK4gUCs0` `1k` 60000 ¥0.12 | 0.18 | ✅ | ✅ |
| 4 | flare | `1536x1024` | 00:30:20 | `6S6Kc1ifd` **`2k`** 120000 ¥0.24 | `6CkeuY8Uf` **`2k`** 80000 ¥0.16 | 0.24 | ✅ | ✅ |
| 5 | flare | `2048x1152` | 00:30:20 | `6mwkZICcz` `2k` 120000 ¥0.24 | `69LLZ0BDK` `2k` 80000 ¥0.16 | 0.24 | ✅ | ✅ |
| 6 | flare | `2880x2880` | 00:30:29 | `6vKbAW4XT` `4k` 157500 ¥0.315 | `6I04Piibr` `4k` 105000 ¥0.21 | 0.315 | ✅ | ✅ |
| 7 | flare **edits multipart** | `2048x1152` | 00:31:28 | `6kE077nSa` **`2k`** 120000 ¥0.24 | `6iYui4qDj` **`2k`** 80000 ¥0.16 | 0.24 | ✅ | ✅ |
| 8 | flare **edits multipart** | `2048x1152` `n=2` | 00:31:31 | `6BxCLauoD` **`2k`×2** 240000 ¥0.48 | `6aMH2SuO0` **`2k`×2** 160000 ¥0.32 | 0.48 | ✅ | ✅ |
| 9 | flare | `auto` | 00:30:25 | `6quFuQFaP` `1k` 90000 ¥0.18 | `6h1TOgkqV` `1k` 60000 ¥0.12 | 0.18 | ✅ | ✅ |
| 10 | gemini-3-pro 原生 | `imageSize=4K` | 00:30:47 | `6TM4Z1Qsb` `4k` 150000 ¥0.30 | `64nOumr6e` `4k` 100000 ¥0.20 | 0.30 | ✅ | ✅ |
| 11 | gemini-3-pro | 不传 size | 00:30:20 | `67OC6Qtfk` `1k` 112500 ¥0.225 | `6wr5p7DpM` `1k` 75000 ¥0.15 | 0.225 | ✅ | ✅ |

### 第二批（`20260913162613` ~ `20260913163406`）

| # | 模型 | 请求参数 | 时间 CST | keli rid / tier / quota / ¥ | FlowAPI rid / tier / quota / ¥ | 预期 keli ¥ | 两侧同档 | 符合 |
|---|---|---|---|---|---|---:|:--:|:--:|
| 1 | gpt-image-2 | `1024x1024` | 00:26:45 | `6BFijqFlo` `1k` 90000 ¥0.18 | `6dP8HkBzR` `1k` 60000 ¥0.12 | 0.18 | ✅ | ✅ |
| 2 | gpt-image-2 | `2048x2048` | 00:27:43 | `6ZRxpT87A` `4k` 157500 ¥0.315 | `6aXR3qEY0` `4k` 105000 ¥0.21 | 0.315 | ✅ | ✅ |
| 3 | flare | `1024x1024` | 00:28:12 | `67YRTgkD6` `1k` 90000 ¥0.18 | `6T9UPhCkK` `1k` 60000 ¥0.12 | 0.18 | ✅ | ✅ |
| 4 | flare | `1536x1024` | 00:28:49 | `6Cky2WmVO` **`2k`** 120000 ¥0.24 | `6MT0WC1ZF` **`2k`** 80000 ¥0.16 | 0.24 | ✅ | ✅ |
| 5 | flare | `2048x1152` | 00:29:28 | `6VfTZHxUk` `2k` 120000 ¥0.24 | `6GY0sIDCF` `2k` 80000 ¥0.16 | 0.24 | ✅ | ✅ |
| 6 | flare | `2880x2880` | 00:30:09 | `6ugQc5MX1` `4k` 157500 ¥0.315 | `6X3pmViaQ` `4k` 105000 ¥0.21 | 0.315 | ✅ | ✅ |
| 7 | flare **edits multipart** | `2048x1152` | 00:30:57 | `69Du4yBDX` **`2k`** 120000 ¥0.24 | `6aCYuhLyc` **`2k`** 80000 ¥0.16 | 0.24 | ✅ | ✅ |
| 8 | flare **edits multipart** | `2048x1152` `n=2` | 00:31:45 | `6SCfxiY69` **`2k`×2** 240000 ¥0.48 | `6MtSIPwB1` **`2k`×2** 160000 ¥0.32 | 0.48 | ✅ | ✅ |
| 9 | flare | `auto` | 00:32:15 | `6d9pG1ODU` `1k` 90000 ¥0.18 | `6par6lB0t` `1k` 60000 ¥0.12 | 0.18 | ✅ | ✅ |
| 10 | gemini-3-pro 原生 | `imageSize=4K` | 00:33:41 | `6Y78oyzw9` `4k` 150000 ¥0.30 | `6ptU1meEh` `4k` 100000 ¥0.20 | 0.30 | ✅ | ✅ |
| 11 | gemini-3-pro | 不传 size | 00:34:30 | `6pZM2RdX7` `1k` 112500 ¥0.225 | `67rk823KM` `1k` 75000 ¥0.15 | 0.225 | ✅ | ✅ |

日志 id 区间：keli `576485`–`576510`；FlowAPI `26034`–`26060`。两侧全部 `type=2`（消费日志）、`billing_mode=tiered_expr`。keli 侧 `group=image`、channel 73；FlowAPI 侧 `username=keli`、`group=【职刻】Image`、channel 55（【职刻】Image）。

---

## 三条关键判据

### 判据 1 —— 第 7 条 multipart `2048x1152` 命中什么档？**两侧均 `2k`，改动生效** ✅

| 侧 | 第一批 | 第二批 | matched_tier | request_path | content |
|---|---|---|---|---|---|
| keli | `6kE077nSa` 120000 ¥0.24 | `69Du4yBDX` 120000 ¥0.24 | `2k` | `/v1/images/edits` | `大小 2048x1152 … 生成数量 1` |
| FlowAPI | `6iYui4qDj` 80000 ¥0.16 | `6aCYuhLyc` 80000 ¥0.16 | `2k` | `/v1/images/edits` | `大小 2048x1152 … 生成数量 1` |

**本次代码改动的唯一判据成立。** multipart 表单字段已成功投影进表达式的 `param()` 命名空间：`param("size")` 读到 `2048x1152`，面积 2,359,296 > 1,048,576 → 命中 `2k`。投影未生效时会是 `1k`（keli ¥0.18 / FlowAPI ¥0.12），实际不是。两侧独立部署的同一份修复都生效了。

### 判据 2 —— 第 4 条 `1536x1024` 命中什么档？**两侧均 `2k`，边界填对了** ✅

keli `6S6Kc1ifd` / `6Cky2WmVO` = `2k` ¥0.24；FlowAPI `6CkeuY8Uf` / `6MT0WC1ZF` = `2k` ¥0.16。

两侧 `gpt-image-2.5-flare` 表达式均确认为 `px > 1048576 ? tier("2k", 160000)`，**不是** gpt-image-2 的 `1572864`。1536×1024 = 1,572,864 恰好**等于** gpt-image-2 的边界值 —— 若 flare 误填该值，`>` 为严格大于会判成 `1k`。实际命中 `2k`，**边界无需修改**。

### 判据 3 —— 第 8 条 n=2 扣了几张的钱？**两侧均 2 张** ✅

keli `6BxCLauoD` / `6SCfxiY69` = 240000 = ¥0.48；FlowAPI `6aMH2SuO0` / `6MtSIPwB1` = 160000 = ¥0.32。均恰为同参数 n=1 那条的 **2 倍**，`content` 记录 `生成数量 2`。

`param("n")` 在 multipart 路径下被正确读到，表达式尾部 `* max(float(param("n") ?? 1), 1.0)` 按 2 生效。

---

## FlowAPI 侧追加确认项

### ① 两条 ¥0.32 确实是「2k 档 × 2 张」，不是某个 ¥0.32 单档 ✅

三重证据：

1. `other.matched_tier` = **`2k`**（不是某个更高档位名）
2. `content` = `大小 2048x1152, 品质 standard, 生成数量 2` —— 张数明确记为 2
3. **flare 表达式里根本不存在 ¥0.32 的单档**。三个 `tier()` 值只有 `120000`/`160000`/`210000`，折算 ¥0.12/¥0.16/¥0.21。160000 quota 只能由 `2k` 的 80000 × 2 得到，无第二种组合

对照同参数 n=1 的 `6iYui4qDj`/`6aCYuhLyc` = 80000，倍数关系闭合。

### ② ¥0.12 那条（00:32:15，tokens 3/515）对应哪次请求？**第二批第 9 条，`size=auto`** ✅

FlowAPI `6par6lB0t`（id 26058） ↔ keli `6d9pG1ODU`（id 576508），两侧同为 `00:32:15`、`pt=3 ct=515`、`content = 大小 auto`、`matched_tier=1k`。

**它不是落到了不同上游后端 —— 它就是 `auto` 那条，¥0.12 是 `1k` 档的正常价。** 有输出 token 的并非只有它一条，全窗口 22 条里有 6 条带 usage：

| FlowAPI id | 对应用例 | pt / ct |
|---|---|---|
| 26034 | 二批 #1 gpt-image-2 `1024x1024` | 3 / 1056 |
| 26036 | 二批 #2 gpt-image-2 `2048x2048` | 17 / 2296 |
| 26046 | 一批 #5 flare `2048x1152` | 3 / 367 |
| 26050 | 一批 #1 gpt-image-2 `1024x1024` | 3 / 1056 |
| 26051 | 一批 #2 gpt-image-2 `2048x2048` | 3 / 397 |
| 26058 | 二批 #9 flare `auto` | 3 / 515 |

值得注意：**两批的 `auto` 请求 usage 表现不同** —— 一批 `6h1TOgkqV` 是 `pt=1 ct=0`，二批 `6par6lB0t` 是 `pt=3 ct=515`，参数完全相同。这是上游是否回传 usage 的抖动，**但两条扣费完全一样（均 `1k`，FlowAPI ¥0.12 / keli ¥0.18）**。这与本批仅依赖请求参数的分辨率表达式一致：这些表达式不引用 token 变量，因此两种 usage 记录得到相同扣费；不能推广为所有 `tiered_expr` 表达式都不看 token。

### ③ gemini 的 ¥0.2 是 `4k` 档，不是巧合 ✅

FlowAPI `64nOumr6e` / `6ptU1meEh`：`matched_tier=4k`，quota 100000 = ¥0.20，`request_path=/v1beta/models/gemini-3-pro-image-preview:generateContent`（原生路径）。

同批次另有两条 gemini 落 `1k`：`6wr5p7DpM` / `67rk823KM`，quota 75000 = **¥0.15**，走 `/v1/images/generations`。

两个档位在同一批日志里被**分别观测到且金额不同**（¥0.20 vs ¥0.15），对应表达式的 `tier("4k", 200000)` 与 `tier("1k", 150000)`，档位与金额一一对上，排除巧合。

### ④ FlowAPI 侧目标图像消费日志有没有扣费为 0 的记录？**没有** ✅

`created_at BETWEEN 1789316700 AND 1789317500` 全窗口扫描共 27 行：

- 22 条图像消费日志 quota 分别为 60000 / 75000 / 80000 / 100000 / 105000 / 160000，**无一为 0**
- 唯一 quota=0 的是 id `26057`（`type=3`，`username=flowadmin`，内容 `Updated user tang (ID: 13)`）—— 管理端**用户资料**变更审计日志，与计费配置无关，不是消费日志
- 其余 4 行（`gpt-5.6-terra` ×1、`claude-sonnet-5` ×3）是同窗口无关业务流量，正常计费

keli 侧同样扫描（36 行）：22 条图像日志无零扣费；10 行 quota=0 全为 `type=3` 设置变更审计（id 576475–576484，内容为 `Updated system setting billing_setting.billing_expr` 等，是用户本次配置表达式留下的）。

**「上游未返回 usage → 回退 token 计费 → 算成 0」这条最危险的失败模式在两侧均未发生。** 佐证：22 条中有 16 条 `ct=0`，扣费依然全部正确。仅凭日志中的零输出 token 不能断言上游完全没有返回 usage；确认这一点需要对应的原始响应。

### ⑤ 两边档位判定是否一致？**22/22 完全一致，无一处分歧** ✅

每一对配对请求的 `matched_tier` 与 `request_path` 都相同，且 `keli quota = FlowAPI quota × 1.5` 逐条精确成立：

| 档位 | FlowAPI quota | FlowAPI ¥ | keli quota | keli ¥ | 出现次数 |
|---|---:|---:|---:|---:|---:|
| flare/gpt-image-2 `1k` | 60000 | 0.12 | 90000 | 0.18 | 6 |
| flare `2k` | 80000 | 0.16 | 120000 | 0.24 | 6 |
| flare `2k` ×2 | 160000 | 0.32 | 240000 | 0.48 | 2 |
| flare/gpt-image-2 `4k` | 105000 | 0.21 | 157500 | 0.315 | 4 |
| gemini `1k` | 75000 | 0.15 | 112500 | 0.225 | 2 |
| gemini `4k` | 100000 | 0.20 | 150000 | 0.30 | 2 |

keli 侧多出的部分**完全来自分组倍率 1.5**，没有任何一分钱来自判档差异。六种档位组合在两侧都被实际观测到，换算逐条闭合。

另外对 keli 侧每条日志的 `other.expr_b64` 取 md5，同模型跨两批哈希相同（gpt-image-2 `2820c02d…`、flare `3b9f9148…`、gemini `711fcdc2…`），说明两批命中的是同一份表达式快照，期间无配置漂移。

### ⑥ FlowAPI 的 `billing_mode` / `billing_expr` 配置 ✅

三方（FlowAPI / keli / 定稿第 6 版）逐字节比对：

| 模型 | FlowAPI mode | FlowAPI = keli | FlowAPI = 定稿 | 长度 |
|---|---|:--:|:--:|---:|
| `gpt-image-2` | `tiered_expr` | ✅ | ✅ | 272 |
| `gpt-image-2.5-flare` | `tiered_expr` | ✅ | ✅ | 272 |
| `gpt-image-2.5-sunburst` | `tiered_expr` | ✅ | ✅ | 272 |
| `gemini-3-pro-image-preview` | `tiered_expr` | ✅ | ✅ | 1184 |
| `gemini-3.1-flash-image-preview` | `tiered_expr` | ✅ | ✅ | 1184 |
| `gemini-3.1-flash-image` | `tiered_expr` | ✅ | ✅ | 1184 |

长度与定稿声明的「gpt 三条 272 字符 3 行；gemini 三条 1184 字符 11 行」完全吻合，多行文本经两侧前端保存链路后均未被改写。

**一处两侧配置差异（已核实为无害）**：keli 侧额外为 `gemini-3-pro-image` 与 `google/gemini-3.1-flash-image-preview` 两个别名各配了一份相同表达式，FlowAPI 侧没有。查 `abilities` 表确认：**这两个模型名在 keli 和 FlowAPI 都没有任何渠道挂载（各 0 行）**，即两侧都不对外提供该模型名，属于防御性的冗余配置，不构成 FlowAPI 侧的配置缺口。若将来真要上线这两个名字，需在 FlowAPI 侧补配。

---

## 观察项（非缺陷，供决策）

1. **`gpt-image-2` 的 `2k` 与 `1k` 同价**（均 `tier(…, 120000)` = ¥0.12），且 2K 边界为 `px > 1572864`，与 flare 的 `1048576` 不同。这是定稿的有意设计（账单实证 `1536x1024` 按 1K 收，1K/2K 同价，边界只影响档位名），**不需要改**。本次 22 条中 gpt-image-2 只覆盖了 `1k` 和 `4k`，其 `2k` 分支未被触发。

2. **`size=auto` 的上游账单口径尚未用真实账单交叉验证**。两侧均按请求参数判 `1k`（FlowAPI ¥0.12 成本价）；上游 flare 交付 1254×1254 = 1,572,516 px，若上游改按**交付像素**而非请求 `size` 计价，会落在其自身 2K 边界（`> 1,048,576`）之上，形成每张约 ¥0.04 的价差。定稿文档 `入口对照` 表已把该行判为「一致」（即上游同样按请求 `size` 归 1K），但这是本批 22 条里唯一一处我方判档与交付像素不同向的情形。FlowAPI 侧 flare 各档均为**平价**（成本 = 售价），一旦口径不符会直接变成亏损而非毛利收窄。建议下期 tuzi 账单到账后单独核对这两条 `auto` 请求的实际计费档。**当前不构成验收阻塞。**

3. `gemini-3-pro-image-preview` 的两条入口在两侧都正常工作：原生 `/v1beta/models/…:generateContent`（`imageSize` 五段探测链）判 `4k`，OpenAI 兼容 `/v1/images/generations`（无 size 兜底）判 `1k`。

---

## Caveats

- 两侧全部为只读 `SELECT`，未对任何生产库做写操作，未修改任何配置。
- 两侧配对依据为「同秒时间戳 + 模型名 + token 数 + content」四重匹配，22 条全部唯一确定；未使用 `upstream_request_id` 关联（keli 侧该字段在本批日志中未填充）。
- gemini 原生路径的两条日志 `content` 为空，档位判定取自 `other.matched_tier` —— 该字段由结算时写入（`service/log_info_generate.go`），是实际命中的档位名，可信度高于 `content`。
- 上游成本数字引自 `final-expressions-verification.md` 的 tuzi 账单口径，本次未重新核对账单，仅用于观察项 2 的量级估计。
