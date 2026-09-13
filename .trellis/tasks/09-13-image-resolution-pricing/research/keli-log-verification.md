# Research: keli 生产日志核对 —— 22 条验证请求的实际命中档位与扣费

- **Query**: 在 keli api 广州生产实例（`8.138.196.139`，容器 `postgres:15-alpine`，库 `new-api`）核对 2026-09-14 00:26–00:35 CST 两批共 22 条图像验证请求的 `matched_tier` 与 `quota`，判定阶梯计费改动是否验收通过
- **Scope**: 只读查询（`SELECT` only，未执行任何 `UPDATE`/`INSERT`/`DELETE`）
- **Date**: 2026-09-14
- **换算口径**: `QuotaPerUnit=500000`，站点 `quota_display_type=CNY`、`usd_exchange_rate=1` → **¥ = quota / 500000**
- **分组倍率**: 生产 `options.GroupRatio` 实测 `image = 1.5`（全部 22 条日志 `other.group_ratio` 均为 `1.5`），故 **实扣 = 表达式基准价 × 1.5**

---

## 结论

**22 / 22 全部符合预期，零偏差。** 三条关键判据全部通过；无任何一条扣费为 0；两批同参数请求的档位与扣费逐条一致；6 个模型（实际 8 个模型名条目）的 `billing_mode` 全为 `tiered_expr`，`billing_expr` 与 `final-expressions-verification.md` 第 6 版定稿**逐字节相同**。

---

## 逐条对照表

`¥` 列 = `quota / 500000`。`基准` 列 = 实扣 ÷ 1.5，即表达式 `tier()` 值折算后的价格。

### 第一批

| # | 模型 | 请求参数 | request id 尾段 | 实际档位 | 实扣 quota | 实扣 ¥ | 基准 ¥ | 预期 ¥ | 符合 |
|---|---|---|---|---|---:|---:|---:|---:|:--:|
| 1 | gpt-image-2 | `1024x1024` | `6uXqwOzq3` | `1k` | 90000 | 0.18 | 0.12 | 0.18 | ✅ |
| 2 | gpt-image-2 | `2048x2048` | `6m8dsVKh1` | `4k` | 157500 | 0.315 | 0.21 | 0.315 | ✅ |
| 3 | flare | `1024x1024` | `6fpRbZuaH` | `1k` | 90000 | 0.18 | 0.12 | 0.18 | ✅ |
| 4 | flare | `1536x1024` | `6S6Kc1ifd` | **`2k`** | 120000 | 0.24 | 0.16 | 0.24 | ✅ |
| 5 | flare | `2048x1152` | `6mwkZICcz` | `2k` | 120000 | 0.24 | 0.16 | 0.24 | ✅ |
| 6 | flare | `2880x2880` | `6vKbAW4XT` | `4k` | 157500 | 0.315 | 0.21 | 0.315 | ✅ |
| 7 | flare **edits multipart** | `2048x1152` | `6kE077nSa` | **`2k`** | 120000 | 0.24 | 0.16 | 0.24 | ✅ |
| 8 | flare **edits multipart** | `2048x1152` `n=2` | `6BxCLauoD` | **`2k` ×2** | 240000 | 0.48 | 0.32 | 0.48 | ✅ |
| 9 | flare | `auto` | `6quFuQFaP` | `1k` | 90000 | 0.18 | 0.12 | 0.18 | ✅ |
| 10 | gemini-3-pro 原生 | `imageSize=4K` | `6TM4Z1Qsb` | `4k` | 150000 | 0.30 | 0.20 | 0.30 | ✅ |
| 11 | gemini-3-pro | 不传 size | `67OC6Qtfk` | `1k` | 112500 | 0.225 | 0.15 | 0.225 | ✅ |

### 第二批

| # | 模型 | 请求参数 | request id 尾段 | 实际档位 | 实扣 quota | 实扣 ¥ | 基准 ¥ | 预期 ¥ | 符合 |
|---|---|---|---|---|---:|---:|---:|---:|:--:|
| 1 | gpt-image-2 | `1024x1024` | `6BFijqFlo` | `1k` | 90000 | 0.18 | 0.12 | 0.18 | ✅ |
| 2 | gpt-image-2 | `2048x2048` | `6ZRxpT87A` | `4k` | 157500 | 0.315 | 0.21 | 0.315 | ✅ |
| 3 | flare | `1024x1024` | `67YRTgkD6` | `1k` | 90000 | 0.18 | 0.12 | 0.18 | ✅ |
| 4 | flare | `1536x1024` | `6Cky2WmVO` | **`2k`** | 120000 | 0.24 | 0.16 | 0.24 | ✅ |
| 5 | flare | `2048x1152` | `6VfTZHxUk` | `2k` | 120000 | 0.24 | 0.16 | 0.24 | ✅ |
| 6 | flare | `2880x2880` | `6ugQc5MX1` | `4k` | 157500 | 0.315 | 0.21 | 0.315 | ✅ |
| 7 | flare **edits multipart** | `2048x1152` | `69Du4yBDX` | **`2k`** | 120000 | 0.24 | 0.16 | 0.24 | ✅ |
| 8 | flare **edits multipart** | `2048x1152` `n=2` | `6SCfxiY69` | **`2k` ×2** | 240000 | 0.48 | 0.32 | 0.48 | ✅ |
| 9 | flare | `auto` | `6d9pG1ODU` | `1k` | 90000 | 0.18 | 0.12 | 0.18 | ✅ |
| 10 | gemini-3-pro 原生 | `imageSize=4K` | `6Y78oyzw9` | `4k` | 150000 | 0.30 | 0.20 | 0.30 | ✅ |
| 11 | gemini-3-pro | 不传 size | `6pZM2RdX7` | `1k` | 112500 | 0.225 | 0.15 | 0.225 | ✅ |

日志 id 区间 `576485`–`576510`，`created_at` 区间 `1789316805`–`1789317270`（CST `09-14 00:26:45` – `00:34:30`）。全部 `type=2`（消费日志）、`group=image`、`channel 73`、`billing_source=wallet`。

---

## 三条关键判据

### 判据 1 —— 第 7 条 multipart `2048x1152` 命中什么档？**`2k` ¥0.24，改动生效** ✅

两批的 multipart edits 请求（`6kE077nSa` / `69Du4yBDX`）：

- `other.matched_tier` = **`2k`**
- `other.request_path` = `/v1/images/edits`（确认走的是 multipart 编辑入口，不是 generations）
- `quota` = 120000 = **¥0.24**
- `content` = `大小 2048x1152, 品质 standard, 生成数量 1`（表单里的 `size` 已被日志层读到）

**本次代码改动的唯一判据成立**：multipart 表单字段已成功投影进表达式的 `param()` 命名空间，`param("size")` 读到 `2048x1152`，面积 2,359,296 > 1,048,576 → 命中 `2k`。若投影未生效会是 `1k` ¥0.18，实际不是。

### 判据 2 —— 第 4 条 `1536x1024` 命中什么档？**`2k`，边界填对了** ✅

两批（`6S6Kc1ifd` / `6Cky2WmVO`）均为 `matched_tier=2k`、quota 120000 = ¥0.24。

生产 `gpt-image-2.5-flare` 表达式确认为 `px > 1048576 ? tier("2k", 160000)`，**不是** gpt-image-2 的 `1572864`。1536×1024 = 1,572,864，恰好等于 gpt-image-2 的边界值——若 flare 误填 `1572864`，`>` 为严格大于会判成 `1k` ¥0.18。实际命中 `2k`，说明边界是 `1048576`，**无需修改**。

### 判据 3 —— 第 8 条 n=2 扣了几张的钱？**2 张** ✅

两批（`6BxCLauoD` / `6SCfxiY69`）均为 `quota` = 240000 = **¥0.48**，恰为第 7 条（同参数 n=1）120000 的 **2 倍**；`content` 记录 `生成数量 2`。

`param("n")` 在 multipart 路径下被正确读到，表达式尾部 `* max(float(param("n") ?? 1), 1.0)` 按 2 生效。若读不到会退化为 ×1 → ¥0.24。

---

## 另外必查项

### 有没有任何一条扣费为 0？**没有** ✅

按 `created_at BETWEEN 1789316700 AND 1789317400` 全窗口扫描（36 行），结果：

- 22 条目标图像请求 quota 分别为 90000 / 112500 / 120000 / 150000 / 157500 / 240000，**无一为 0**
- 窗口内 quota=0 的 10 行全部是 `type=3`（管理端设置变更审计日志，id 576475–576484，内容为 `Updated system setting ModelPrice` / `billing_setting.billing_mode` / `billing_setting.billing_expr` 等），是用户本次配置表达式留下的记录，**不是消费日志**
- 其余 4 行（`gpt-5.6-terra`、`claude-sonnet-5` ×3）是同窗口的无关业务流量，正常计费

「上游未返回 usage → 回退 token 计费 → 算成 0」这条最危险的失败模式**未发生**。佐证：全部 22 条 `other` 中 `model_price=0`、`model_ratio=0`、`completion_ratio=0`，`billing_mode=tiered_expr`，说明走的是表达式路径而非 ratio 路径；第 3–8、11 条的 `prompt_tokens=1 / completion_tokens=0`（上游确实没给 usage），但扣费仍然正确。

### 第 9 条 `size=auto` 命中什么档？**`1k`，判档取的是请求参数，符合设计** ✅

两批（`6quFuQFaP` / `6d9pG1ODU`）均为 `matched_tier=1k`、quota 90000 = ¥0.18，`content` = `大小 auto`。

`auto` 不匹配 `^[0-9]+x[0-9]+$` → `px = 0.0` → 落 `1k`。**判档基于客户端声明的请求参数，与上游实际交付的 1254×1254 无关**，与 `final-expressions-verification.md` 的计费口径声明（「按客户端请求中声明的分辨率计费，与实际交付尺寸、与路由到哪个渠道都无关」）一致。

### 两批同参数请求的档位与扣费是否一致？**逐条完全一致，无随机性** ✅

11 组同参数配对的 `matched_tier`、`quota`、`request_path` 全部相同。另外对每条日志的 `other.expr_b64` 取 md5，同模型跨两批哈希完全相同：

| 模型 | expr_b64 md5 | 出现条数 |
|---|---|---:|
| gpt-image-2 | `2820c02ddc0ce77b97e18ca32412d4bc` | 4 |
| gpt-image-2.5-flare | `3b9f9148da9a6c261b721503f9bfdb80` | 14 |
| gemini-3-pro-image-preview | `711fcdc2f0daa964e261b5d723d53ffe` | 4 |

说明两批请求命中的是同一份表达式快照，期间没有配置漂移。

### `billing_mode` 是否都是 `tiered_expr`？**是** ✅

`options` 表 key `billing_setting.billing_mode`（长度 1496）中，本次涉及的图像模型条目：

| 模型名 | billing_mode |
|---|---|
| `gpt-image-2` | `tiered_expr` |
| `gpt-image-2.5-flare` | `tiered_expr` |
| `gpt-image-2.5-sunburst` | `tiered_expr` |
| `gemini-3-pro-image` | `tiered_expr` |
| `gemini-3-pro-image-preview` | `tiered_expr` |
| `gemini-3.1-flash-image` | `tiered_expr` |
| `gemini-3.1-flash-image-preview` | `tiered_expr` |
| `google/gemini-3.1-flash-image-preview` | `tiered_expr` |

定稿文档的 6 条表达式实际落到了 **8 个模型名条目**（`gemini-3-pro-image` 与 `gemini-3.1-flash-image-preview` 的 `google/` 前缀别名各多挂了一份），属于合理扩展，非配置错误。

### `billing_expr` 文本是否与第 6 版定稿一致？**8 条全部逐字节相同** ✅

拉取 `options` 表 key `billing_setting.billing_expr`（长度 11876），与 `research/final-expressions-verification.md` 第 6 版 A–F 六条围栏代码块逐字节比对：

| 生产模型名 | 对应定稿条目 | 结果 | 生产长度 | 定稿长度 |
|---|---|---|---:|---:|
| `gpt-image-2` | A | IDENTICAL | 272 | 272 |
| `gpt-image-2.5-flare` | B | IDENTICAL | 272 | 272 |
| `gpt-image-2.5-sunburst` | C | IDENTICAL | 272 | 272 |
| `gemini-3-pro-image-preview` | D | IDENTICAL | 1184 | 1184 |
| `gemini-3-pro-image` | D | IDENTICAL | 1184 | 1184 |
| `gemini-3.1-flash-image-preview` | E | IDENTICAL | 1184 | 1184 |
| `gemini-3.1-flash-image` | F | IDENTICAL | 1184 | 1184 |
| `google/gemini-3.1-flash-image-preview` | E | IDENTICAL | 1184 | 1184 |

长度与定稿声明的「A/B/C 272 字符、3 行；D/E/F 1184 字符、11 行」完全吻合，多行文本经前端保存链路后未被改写。

---

## 换算验证（表达式 `tier()` 值 → 实扣）

`tier(name, v)` → `v / 1e6 × QuotaPerUnit(500000) × groupRatio(1.5)`：

| tier 值 | 基准 quota | 基准 ¥ | ×1.5 后 quota | 实扣 ¥ | 日志实测 |
|---:|---:|---:|---:|---:|---|
| 120000 | 60000 | 0.12 | 90000 | 0.18 | ✅ 1k(gpt/flare) |
| 160000 | 80000 | 0.16 | 120000 | 0.24 | ✅ 2k(flare) |
| 210000 | 105000 | 0.21 | 157500 | 0.315 | ✅ 4k(gpt/flare) |
| 150000 | 75000 | 0.15 | 112500 | 0.225 | ✅ 1k(gemini-pro) |
| 200000 | 100000 | 0.20 | 150000 | 0.30 | ✅ 4k(gemini-pro) |
| 160000×2 | 160000 | 0.32 | 240000 | 0.48 | ✅ 2k×2(flare n=2) |

六种档位组合全部在生产日志中被实际观测到，换算逐条闭合。

---

## 观察项（非缺陷，供决策）

1. **`gpt-image-2` 的 `2k` 与 `1k` 同价（均 `tier(…, 120000)` = ¥0.12），且 2K 边界为 `px > 1572864`**，与 flare 的 `1048576` 不同。这是第 6 版定稿的有意设计（`阈值依据` 表：gpt-image-2 账单实证 `1536x1024` 按 1K 收，1K/2K 同价，边界只影响档位名），**不需要改**。本次 22 条中 gpt-image-2 只覆盖了 1k 和 4k，`2k` 分支未被实际触发。

2. **`size=auto` 的上游账单口径尚未用真实账单交叉验证**。我方按请求参数判 `1k` ¥0.12；上游 flare 交付 1254×1254 = 1,572,516 px，若上游改为按**交付像素**而非请求 `size` 计价，则会落在其自身 2K 边界（`> 1,048,576`）之上，形成每张约 ¥0.04 的价差。定稿文档 `入口对照` 表已把该行标为「一致」（即判定上游同样按请求 `size` 归 1K），但这是本批 22 条里唯一一处我方判档与交付像素不同向的情形。建议下一期 tuzi 账单到账后单独核对这两条 `auto` 请求的实际计费档，确认口径后即可关闭该观察项。**当前不构成验收阻塞。**

3. `gemini-3-pro-image-preview` 的第 10 条走原生路径 `/v1beta/models/gemini-3-pro-image-preview:generateContent`（`request_conversion=["Google Gemini"]`），第 11 条走 OpenAI 兼容路径 `/v1/images/generations`（`request_conversion=["openai_image"]`）。两条入口都正确命中各自预期档位，说明 `imageSize` 五段探测链与 `size` 面积兜底两条判档路径在生产均正常工作。

---

## Caveats

- 本次核对全部为只读 `SELECT`，未对生产库做任何写操作，未修改任何配置。
- `content` 字段中 gemini 原生路径两条（`6TM4Z1Qsb` / `6Y78oyzw9`）为空，档位判定依据取自 `other.matched_tier`，该字段由 `service/log_info_generate.go` 写入，是结算时实际命中的档位名，可信度高于 `content`。
- 上游成本数字引自 `final-expressions-verification.md` 的 tuzi 账单口径，本次未重新核对账单，仅用于观察项 2 的量级估计。
