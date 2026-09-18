# Research: 按渠道统计缓存命中率（对齐 CCH 算法）

- **Query**: 缓存相关量的来源链路 / 缓存命中率定义（对齐 Claude Code Hub）/ 数据源选型（`channel_metrics` 加列 vs `logs` 聚合）/ 在 `channel-monitoring` 页面的接线点 / 累加防溢出
- **Scope**: mixed（本仓库 internal + CCH 源码 external）
- **Date**: 2026-09-11
- **前置阅读**（已读，不重复）: `.trellis/tasks/09-10-admin-channel-availability/prd.md`、`design.md`、`.trellis/tasks/09-10-group-monitor-log-aggregation/research/log-aggregation-feasibility.md`

## 提交前复核（2026-09-18）

本文正文保留 2026-09-11 的研究快照，代码行号、CCH 源码及尚待实现事项不代表当前状态。当前仓库 `model/channel_metric.go` 已有缓存计数列，不能据正文再次添加。本次未重新拉取 CCH、执行迁移或连接生产服务器。

以下更正优先于正文的历史表述：

- §2.1 将缓存项重复加进分母会使命中率偏低，并不会因此变成大于 1；大于 1 需要缓存读取量本身超过归一化总输入等异常。统计侧仍应校验原始计数并限制结果范围。
- §3.2 的 JSON 方言、跨库和查询成本是实现约束，不能证明日志聚合在技术上“不可行”；本研究是在这些约束下推荐独立计数列。§3.3 的加列方案不涉及已有唯一索引变更，但生产锁表、耗时及各库兼容性仍需按数据规模验证，不能直接视为迁移安全保证。
- §1.3 列出的是已检查到的解析路径；未单列的渠道可能复用 OpenAI 等适配器，不能据此断言其缓存量恒为 0。§2.3 的跨 provider 偏差依赖具体 usage 返回行为，源码字段本身不能证明所有 Claude 未命中都会写缓存或 OpenAI 缓存写入免费。
- §5 的超大整数例子来自请求侧 `*uint` 风险，不适用于这里的 `dto.Usage` 有符号 `int` 字段；超出 `int` 范围的 JSON 数值应解析失败。统计仍需防范可解析范围内的异常大值及累计溢出。

---

## 0. 结论速览

| 问题 | 结论 |
|---|---|
| 「缓存读取」与「缓存写入」是否两个独立值 | **是**。读取 = `usage.PromptTokensDetails.CachedTokens`；写入 = `usage.PromptTokensDetails.CacheCreationTokensTotal()`（Claude 还额外拆 5m/1h）|
| Claude 的 `cache_read_input_tokens` / `cache_creation_input_tokens` | **两者都解析且都保留**，见 §1.3 |
| CCH 分母 `input + cache_creation + cache_read` 能否直接套 | **不能直接套**。本项目 `PromptTokens` 的语义**随 usage semantic 变化**：Claude 语义下是 text-only（可加），OpenAI/Gemini 语义下**已含 cached**（再加就是重复计算）。项目内已有现成的正确归一化量，见 §2.1 |
| CCH 前置过滤 `cache_creation>0 OR cache_read>0` 能否实现 | **能**，两个量都在内存里拿得到，见 §2.2 |
| OpenAI 命中率会不会系统性偏高 | **会，而且这是真坑**。见 §2.3 |
| 数据源选型 | **选 A（`channel_metrics` 加列）**，B 被 `other` JSON + ClickHouse 双重阻断，见 §3 |
| 加普通列的迁移风险 | **安全**，与 `perf_metrics` 加 `channel_id` 的唯一索引陷阱**完全不同类**，见 §3.3 |
| 打点位置 | **不能挂在现有 attempt 级打点上**（那里拿不到 usage），必须新增一个成功路径的打点，见 §3.4 |

---

## 1. Q1 — 本项目的缓存量确切清单

### 1.1 承载结构（唯一的规范容器）

`relaykit/dto/openai_response.go:223-244` `dto.Usage`，其中缓存相关的字段：

| Go 字段 | JSON | 语义 | 证据 |
|---|---|---|---|
| `Usage.PromptTokens` | `prompt_tokens` | **语义随上游格式变化**，见 §2.1 | `openai_response.go:224` |
| `Usage.InputTokens` | `input_tokens` | Claude 语义下 = input+read+creation；OpenAI 语义下 = prompt_tokens | `openai_response.go:234`；`service/billing_usage.go:160`、`:107-109` |
| `Usage.PromptTokensDetails.CachedTokens` | `prompt_tokens_details.cached_tokens` | **缓存读取**（唯一的归一化落点） | `openai_response.go:257` |
| `Usage.PromptTokensDetails.CachedCreationTokens` | `cached_creation_tokens` | **缓存写入**（Claude 转换路径填这个） | `openai_response.go:258` |
| `Usage.PromptTokensDetails.CacheWriteTokens` | `cache_write_tokens` | **缓存写入**（OpenAI 原生字段，注释明写「billed at the cache-creation price」） | `openai_response.go:259-263` |
| `Usage.ClaudeCacheCreation5mTokens` | `claude_cache_creation_5_m_tokens` | Claude 5 分钟 TTL 缓存写入 | `openai_response.go:239` |
| `Usage.ClaudeCacheCreation1hTokens` | `claude_cache_creation_1_h_tokens` | Claude 1 小时 TTL 缓存写入 | `openai_response.go:240` |
| `Usage.PromptCacheHitTokens` | `prompt_cache_hit_tokens` | DeepSeek 风格的缓存读取，会被归一化进 `CachedTokens` | `openai_response.go:227`；`relay/channel/openai/usage.go:17-18` |

**唯一的「缓存写入总量」取法**：`InputTokenDetails.CacheCreationTokensTotal()`（`relaykit/dto/openai_response.go:275-284`）

```go
func (d InputTokenDetails) CacheCreationTokensTotal() int {
	total := d.CachedCreationTokens
	if d.CacheWriteTokens > total {
		total = d.CacheWriteTokens
	}
	if total < 0 {
		return 0
	}
	return total
}
```

取 max 而非相加，注释明写「when both are present the larger value wins so the same tokens are never double-counted」，且负值 clamp 到 0。**统计侧必须用这个方法，不要自己读裸字段。**

### 1.2 计费侧的规范快照 `textQuotaSummary`

`service/text_quota.go:44-48` + `:257-265`：

```go
summary.PromptTokens          = usage.PromptTokens                                  // :257
summary.CacheTokens           = usage.PromptTokensDetails.CachedTokens              // :260  ← 缓存读取
summary.CacheCreationTokens   = usage.PromptTokensDetails.CacheCreationTokensTotal()// :261  ← 缓存写入
summary.CacheCreationTokens5m = usage.ClaudeCacheCreation5mTokens                   // :262
summary.CacheCreationTokens1h = usage.ClaudeCacheCreation1hTokens                   // :263
summary.UsageSemantic         = usageSemanticFromUsage(relayInfo, usage)            // :245
summary.IsClaudeUsageSemantic = summary.UsageSemantic == "anthropic"                // :247
```

`cacheWriteTokensTotal(summary)`（`service/text_quota.go:79-88`）是**另一个**归一化函数，只用于**日志展示**：当 5m/1h 拆分值存在且其和 < `CacheCreationTokens` 时取 `CacheCreationTokens`，否则取和。它与 `CacheCreationTokensTotal()` 的区别是前者处理「Claude 拆分字段与总量不一致」，后者处理「OpenAI vs Claude 字段位置不同」。写入 `other["cache_write_tokens"]`（`service/text_quota.go:506-512`）。

### 1.3 各 provider 的解析链路（**是否归一化到同一字段**）

| Provider | 上游字段 | 解析位置 | 落到 |
|---|---|---|---|
| **Claude 原生 relay** | `cache_read_input_tokens` | `relay/channel/claude/relay-claude.go:239` | `PromptTokensDetails.CachedTokens` |
| | `cache_creation_input_tokens` | `relay/channel/claude/relay-claude.go:240` | `PromptTokensDetails.CachedCreationTokens` |
| | `cache_creation.ephemeral_5m/1h_input_tokens` | `relay/channel/claude/relay-claude.go:241-242`（经 `relaykit/dto/claude.go:574-586`） | `ClaudeCacheCreation5m/1hTokens` |
| **Claude → BillingUsage 重映射** | 同上 | `service/billing_usage.go:168-169` | 同上 |
| **Claude → OpenAI 转换** | 同上 | `relaykit/relayconvert/internal/claude_messages/to_oai_chat_resp.go:181`、`:348`、`:371` | `PromptTokensDetails.CachedTokens` |
| **OpenAI Chat** | `prompt_tokens_details.cached_tokens` | 直接 unmarshal 到 `openai_response.go:257` | 同字段 |
| **OpenAI Responses** | `input_tokens_details.cached_tokens` | `relay/channel/openai/relay_responses.go:47`、`:112`；`relay_responses_compact.go:39` | `PromptTokensDetails.CachedTokens` |
| **OpenAI Image** | `input_tokens_details.cached_tokens` | `relay/channel/openai/relay_image.go:81` | 同字段 |
| **OpenAI Realtime** | `input_token_details.cached_tokens` | `relay/channel/openai/relay_realtime.go:132`、`:234` | 累加进 `RealtimeUsage.InputTokenDetails.CachedTokens`（**独立结构，不是 `dto.Usage`**） |
| **OpenAI 原生 cache-write** | `prompt_tokens_details.cache_write_tokens` / `input_tokens_details.cache_write_tokens` | 直接 unmarshal | `PromptTokensDetails.CacheWriteTokens` |
| **Gemini** | `usageMetadata.cachedContentTokenCount` | `service/billing_usage.go:185`；`relaykit/relayconvert/internal/gemini_chat/to_oai_chat_resp.go:37` | `PromptTokensDetails.CachedTokens` |
| **DeepSeek** | `prompt_cache_hit_tokens` | `relay/channel/openai/usage.go:16-19` | `PromptTokensDetails.CachedTokens` |
| **智谱 v4** | `prompt_tokens_details.cached_tokens` / body 回捞 | `relay/channel/openai/usage.go:20-30` + `:53-82` | 同字段 |
| **Moonshot** | `choices[].usage.cached_tokens`（非标准位置） | `relay/channel/openai/usage.go:31-43` + `:86-111` | 同字段 |
| **llama.cpp（OpenAI 类型渠道）** | `timings.cache_n` | `relay/channel/openai/usage.go:44-49` + `:114-133` | 同字段 |
| **OpenRouter（Claude 计费）** | 无缓存写入字段，从 `cost` **反推** | `service/quota.go:257-281` `CalcOpenRouterCacheCreateTokens`，调用点 `service/text_quota.go:274-279` | `summary.CacheCreationTokens` |

**结论：缓存读取已经统一归一化到 `PromptTokensDetails.CachedTokens` 一个字段**（Realtime 除外，它走独立的 `dto.RealtimeUsage`）。**缓存写入分散在三个字段**，必须经 `CacheCreationTokensTotal()` 汇总。

**返回缓存信息的 provider（有实解析代码的）**：Claude / OpenAI(Chat, Responses, Image, Realtime) / Gemini / DeepSeek / 智谱v4 / Moonshot / llama.cpp / OpenRouter。其余 30+ 渠道无缓存解析代码 → 两个量恒为 0。

### 1.4 `logs.other` 里的缓存字段分别是什么、在哪写

| `other` key | 对应量 | 写入位置 |
|---|---|---|
| `cache_tokens` | **缓存读取** = `summary.CacheTokens` = `PromptTokensDetails.CachedTokens` | `service/log_info_generate.go:98`（形参 `cacheTokens`），调用方 `service/text_quota.go:470`（Claude 路径）、`:477`（通用路径） |
| `cache_ratio` | 缓存读取的**计费倍率**（不是命中率！）= `relayInfo.PriceData.CacheRatio` | `service/log_info_generate.go:99`；来源 `service/text_quota.go:237` |
| `cache_creation_tokens` | 缓存写入总量 | `service/log_info_generate.go:324`（Claude 专用生成器）、`service/text_quota.go:495` |
| `cache_creation_ratio` | 缓存写入计费倍率 | `service/log_info_generate.go:325`、`service/text_quota.go:496` |
| `cache_creation_tokens_5m` / `_1h` | Claude 拆分 TTL 写入量，**仅当 >0 时写** | `service/log_info_generate.go:327`、`:331`；`service/text_quota.go:499`、`:503` |
| `cache_write_tokens` | 归一化后的写入总量（展示用），**仅当 >0 时写** | `service/text_quota.go:506-512` |

注意 `GenerateWssOtherInfo`（`service/log_info_generate.go:293`）与 `GenerateAudioOtherInfo`（`:305`）传的 `cacheTokens=0, cacheRatio=0.0` —— **realtime / audio 路径的日志里缓存量恒为 0**，即使 realtime 上游确实返回了 `cached_tokens`（`relay/channel/openai/relay_realtime.go:132`）。这是一个已存在的口径缺口。

---

## 2. Q2 — 对齐 CCH 的命中率定义（不下最终结论，只给可行性与必须的修正）

### 2.0 CCH 的确切算法（从源码核实，与协调者给的一致）

克隆 `ding113/claude-code-hub` 读到两处实现：

**排行榜版**（`src/repository/leaderboard.ts:819-850`，函数 `findProviderCacheHitRateLeaderboardWithTimezone`）:

```
totalInputTokens = COALESCE(input_tokens,0) + COALESCE(cache_creation_input_tokens,0) + COALESCE(cache_read_input_tokens,0)
WHERE cache_creation_input_tokens > 0 OR cache_read_input_tokens > 0      -- 前置过滤
cacheHitRate = COALESCE(sum(cache_read_input_tokens) / NULLIF(sum(totalInputTokens),0), 0)   -- 先累加再除
-> clampRatio01
GROUP BY usage_ledger.final_provider_id                                  -- 最终实际使用的供应商
```

**告警版**（`src/repository/cache-hit-rate-alert.ts:250-292`）额外给出三个指标，值得注意：

- `hitRateTokens` = 与排行榜同一公式（`cache-hit-rate-alert.ts:266-269`）
- `engagementRate` = `cacheSignalRequests / totalRequests`（`:271-274`）—— 即「有缓存信号的请求占比」，把「前置过滤掉了多少」显式暴露出来，**这一项本项目也算得出来，建议一起做**，因为它正好回答「不支持缓存 vs 支持但未命中」
- `hitRateTokensEligible` = 只统计「同 session、非首请求、与上一请求间隔 ≤ TTL」的请求（`:235-259`、`:276-291`）—— **本项目算不出来**，见 §2.4

告警版还有一个 2xx 过滤（`cache-hit-rate-alert.ts:299-300`）。

### 2.1 **最大的坑：`input_tokens` 映射（这里最容易错）**

CCH 的 `input_tokens` 是 Claude 原生语义的 **uncached input**。本项目的 `PromptTokens` **语义不统一**，权威文档明写：

`pkg/billingexpr/expr.md:220-221`：
```
- **OpenAI/GPT**: `prompt_tokens` = total (text + cache + image + audio)
- **Claude**: `input_tokens` = text only (cache reported separately)
```

代码侧的三处独立证据：

1. `service/text_quota.go:308-311` —— 只有**非** Claude 语义才从计费基数里**减掉** cached，说明非 Claude 语义下 cached 本来就含在 `PromptTokens` 里：
   ```go
   if !dCacheTokens.IsZero() {
       if !summary.IsClaudeUsageSemantic && !legacyClaudeDerived {
           baseTokens = baseTokens.Sub(dCacheTokens)
       }
       cachedTokensWithRatio = dCacheTokens.Mul(dCacheRatio)
   }
   ```
2. `service/billing_usage.go:157` —— Claude 语义下 `PromptTokens: claudeUsage.InputTokens`（uncached only），而 `:160` `InputTokens: input + CacheRead + CacheCreation`
3. `service/tiered_settle.go:44-50` —— 项目**已有**一个正好等于 CCH 分母的归一化量：
   ```go
   // len = total input context length for tier condition evaluation.
   // Non-Claude: prompt_tokens already includes everything.
   // Claude: input_tokens is text-only, so add cache read + cache creation.
   inputLen := p
   if isClaudeUsageSemantic {
       inputLen = p + cr + cc5m + cc1h
   }
   ```
   （`pkg/billingexpr/expr.md:235` 同样定义了 `len`）

> **所以：直接把 CCH 的 `p + cr + cc` 套到本项目上，会让 OpenAI / Gemini / DeepSeek / Moonshot / 智谱 等所有非 Claude 语义渠道的分母偏大、命中率被系统性拉低。**
>
> 正确做法是复用 §2.1 第 3 点的归一化逻辑（`tiered_settle.go:47-50` 的 `inputLen`）：分母 = Claude 语义时 `PromptTokens + CachedTokens + CacheCreation(5m+1h)`，其余语义时就是 `PromptTokens`。这在数值上与 CCH 的 `input + creation + read` 完全等价，只是各自的 `PromptTokens` 基准不同。

还有第三种语义：`isLegacyClaudeDerivedOpenAIUsage`（`service/text_quota.go:90-101`）—— 没有 `UsageSource`/`UsageSemantic` 标签但带 Claude 5m/1h 字段的历史 usage，计费上按 Claude 语义处理。统计侧若忽略它，这批请求的分母会偏小、命中率偏高。

**另有 OpenAI cache-write 的重叠问题**：`service/text_quota.go:347-349` 与 `service/tiered_settle.go:76-77` 的注释明写
> "OpenAI cache-write usage reports unadjusted prefix counts, so cached_tokens + cache_write_tokens can exceed prompt_tokens and the remainder can go negative. Clamp at zero."

即 OpenAI 语义下 `cached + cache_write` 可能 > `prompt_tokens`，三者的包含关系**不干净**。因此非 Claude 语义下分母**只能**用 `PromptTokens`，不能加任何缓存项，否则会出现「命中率 > 1」（CCH 的 `clampRatio01` 会把它硬夹到 1，掩盖掉数据问题）。

### 2.2 前置过滤 `cache_creation>0 OR cache_read>0` —— **可实现**

两个量在打点处都拿得到（见 §3.4），过滤条件即：
```
cacheRead := usage.PromptTokensDetails.CachedTokens
cacheWrite := usage.PromptTokensDetails.CacheCreationTokensTotal()   // Claude 时还要考虑 5m/1h
hasCacheSignal := cacheRead > 0 || cacheWrite > 0
```
这需要把「有缓存信号的请求」单独计一个计数器（CCH 的 `cacheSignalRequests`），并且**分母 token 只累加有信号的请求**。即需要两组计数器，见 §3.5。

**「不支持缓存」与「支持但未命中」能否区分？—— 部分能，必须写清楚：**

| 情形 | `cache_read` | `cache_write` | 前置过滤后 | 能否区分 |
|---|---|---|---|---|
| 不支持缓存的模型/渠道 | 0 | 0 | 被排除 ✓ | **与下一行无法区分** |
| 支持缓存、本次完全未命中且未写入（客户端没打 `cache_control`） | 0 | 0 | 被排除 ✓ | **无法区分** |
| Claude：只写不读（首次建缓存 = 未命中） | 0 | >0 | **保留**，贡献分母不贡献分子 → 正确拉低命中率 ✓ | 能 |
| Claude：读到了 | >0 | ≥0 | 保留 ✓ | 能 |
| OpenAI：读到了 | >0 | 0（多数情况） | 保留 ✓ | 能 |
| OpenAI：未命中 | 0 | 0（多数情况） | **被排除** ✗ | **不能** —— 这是 §2.3 的偏高来源 |

**明确结论**：本项目（和 CCH 一样）**无法在代码层面区分「渠道/模型不支持缓存」与「支持但两个量都为 0」**。CCH 的前置过滤把这两类合并排除，代价是 §2.3 的偏差。`engagementRate` = 有信号请求数 / 总请求数是唯一能把「被排除了多少」摆到台面上的指标。

### 2.3 OpenAI 命中率是否系统性偏高 —— **会，且偏差不小**

原因链：
1. Claude 每次未命中的缓存请求都会返回 `cache_creation_input_tokens > 0`（写入即计费，`relaykit/dto/claude.go:558`），所以 **Claude 的「未命中」样本会留在分母里**。
2. OpenAI Chat Completions 的标准 prompt caching 是**自动且免费**的，未命中时 `prompt_tokens_details` 里既没有 `cached_tokens` 也没有 `cache_write_tokens` → 两个量都是 0 → **被前置过滤直接排除**。
3. 于是 OpenAI 渠道的分母里**只剩命中过的请求**，命中率 = `sum(cached) / sum(prompt_tokens)`，在「命中了但只命中了一部分前缀」时才 < 1，**结构上不可能被「完全未命中」的请求拉低**。
4. 反观 Claude，一个只写不读的请求贡献 `0 / (input + creation)` 的分母，会真实拉低命中率。

**量级示意**：同样的流量模式（一半请求命中 90% 前缀、一半完全未命中），Claude 算出约 45%，OpenAI 算出约 90%。**跨 provider 的数字不可比。**

OpenAI 原生 `cache_write_tokens` 字段（`relaykit/dto/openai_response.go:259-263`）的存在缓解了一部分，但它只在部分 OpenAI 模型/计费模式下才返回，不能假设普遍存在。

→ **建议在 UI 上不做跨渠道排名，或至少标注 provider 类型**；否则这个指标会让运维得出「OpenAI 渠道缓存比 Claude 渠道好」的错误结论。

### 2.4 CCH 有而本项目没有的量（阻断点清单）

| CCH 字段 | 用途 | 本项目 |
|---|---|---|
| `messageRequest.sessionId` | `hitRateTokensEligible` 的会话分组 | **无**。relay 请求没有 session 概念。`middleware/auth.go:202` 的 `session_id` 是登录会话，与请求链无关；`model/auth_flow.go:46` 是 OAuth 流程 |
| `messageRequest.requestSequence` | 「非首请求」判定 | **无** |
| `prev.createdAt` / `gapToPrevSeconds` | 与上一请求间隔 ≤ TTL | **无**（需要 sessionId 才能定义「上一请求」）|
| `messageRequest.cacheTtlApplied` | TTL 推断 | **无**该字段；只能从 `ClaudeCacheCreation5m/1hTokens` 间接推 5m/1h |
| `messageRequest.swapCacheTtlApplied` | 计费口径 5m/1h 翻转还原 | **无**此概念 |
| `usageLedger.finalProviderId` | 分组键 | **有对应**：`relayInfo.ChannelId` / `channel_metrics.channel_id`；且本项目的 attempt 级打点比 CCH 的「final provider」口径更细 |
| `statusCode` 2xx 过滤 | 只统计成功请求 | **天然满足**：usage 只在成功结算路径存在（`service/text_quota.go:397`）|
| `cacheCoefficientBp` | CCH 排行榜首要排序键，来自 `src/repository/provider-cache-effectiveness.ts:166`、`:218` | **无**，本轮也不做 |

**唯一真正的阻断点是 `hitRateTokensEligible`** —— 需要 session + 请求序列 + 时间间隔三件套，本项目一件都没有。**排行榜版公式（`hitRateTokens`）与 `engagementRate` 完全可实现。**

最接近 session 的现成设施是渠道亲和性的 key fingerprint（`setting/operation_setting/channel_affinity_setting.go:124` 用 gjson 取 `prompt_cache_key`），但它只在配置了亲和性规则时存在，不能作为通用 session 键。

### 2.5 项目内已有的「缓存命中率」先例（口径对照）

`service/channel_affinity.go` 已经有一套上游缓存命中统计，口径与 CCH **不同**，注意不要混淆：

- 判定：`usageCacheSignals`（`service/channel_affinity.go:913-929`）—— `cached > 0 || promptCacheHitTokens > 0` 即算「命中」，**请求级布尔判定，不是 token 加权**
- 累加：`observeChannelAffinityUsageCache`（`:830-877`），`next.Hit++` / `next.Total++` / `next.CachedTokens +=` / `next.PromptTokens +=`
- 已经识别了 provider 语义差异，用 `cachedTokenRateMode` 标注（`service/channel_affinity.go:67-69`、`:892-901`）：
  ```go
  cacheTokenRateModeCachedOverPrompt           = "cached_over_prompt"            // OpenAI 系
  cacheTokenRateModeCachedOverPromptPlusCached = "cached_over_prompt_plus_cached"// Claude
  cacheTokenRateModeMixed                      = "mixed"
  ```
  → **这正是 §2.1 那个分母问题的项目内既有答案**，新指标应沿用同一套模式判定，而不是另发明一套。
- 前端展示：`web/src/features/system-settings/general/channel-affinity/cache-stats-dialog.tsx:107-111` 显示 `hit/total`，`:144-146` 的文案「Hit criteria: If cached tokens exist in usage, it counts as a hit.」
- 存储：内存 cache + TTL，**不落库、无历史**（`:841-842`、`:876`），所以不能直接复用给监控页

---

## 3. Q3 — 数据源选型：**推荐 A（`channel_metrics` 加列）**

### 3.1 路线 A：给 `channel_metrics` 加缓存计数列

现表 `model/channel_metric.go:22-29`：
```go
type ChannelMetric struct {
	Id             int   `json:"id" gorm:"primaryKey"`
	ChannelId      int   `json:"channel_id" gorm:"uniqueIndex:idx_channel_metric_channel_bucket,priority:1"`
	BucketTs       int64 `json:"bucket_ts" gorm:"uniqueIndex:idx_channel_metric_channel_bucket,priority:2;index:idx_channel_metric_bucket_ts"`
	AttemptCount   int64 `json:"-" gorm:"default:0"`
	SuccessCount   int64 `json:"-" gorm:"default:0"`
	TotalLatencyMs int64 `json:"-" gorm:"default:0"`
}
```

已具备的条件：
- `(channel_id, bucket_ts)` 唯一索引 + `clause.OnConflict` 原子累加（`model/channel_metric.go:35-50`）
- 保留期清理已接线（`pkg/perf_metrics/flush.go:24` → `channel_metrics.go:144-152` → `model/channel_metric.go:108-113`）
- 查询层已按 range 选 step 在 SQL 侧 rollup（`model/channel_metric.go:68-92`）
- 前端页面与 AdminAuth 已就位（`router/api-router.go:37-41`）

需要新增的列（见 §3.5）+ 在 `UpsertChannelMetric` 的 `DoUpdates` 里加对应 `gorm.Expr("col + ?")` + 在 `GetChannelMetricBuckets` 的 `Select` 里加 `SUM(...)`。

### 3.2 路线 B：从 `logs` 聚合 —— **不可行，双重阻断**

1. **CCH 公式需要的三个量，两个在 `other` JSON 文本列里**：
   - `logs.prompt_tokens` 是真列（`model/log.go` 结构体字段），但它的值是 `summary.PromptTokens`（`service/text_quota.go:528`），而 Claude 语义下这已经是 uncached-only，非 Claude 语义下含 cached → **列本身语义不统一，且 `logs` 里没有记录 usage semantic 的真列**（只有 `other["usage_semantic"]`，`service/text_quota.go:475`，而且**只有 Claude 路径才写**）。
   - `cache_tokens`（读取）在 `other` JSON（`service/log_info_generate.go:98`）
   - `cache_creation_tokens` / `cache_write_tokens`（写入）在 `other` JSON，且**只在 >0 时才写**（`service/text_quota.go:494`、`:506`）
   - → 要在 SQL 里聚合就必须 `JSON_EXTRACT` / `::jsonb->>` / `json_extract` / `JSONExtract*` 四套方言写法，**AGENTS.md 明确禁止无 fallback 的 DB 专有 JSON 用法**。
2. **日志库可能是独立库甚至 ClickHouse**：`common/database.go:5-10` 允许 `clickhouse`；`model/main.go:215-254` `InitLogDB()`；聚合结果要落主库 → 跨库读写，且 CH 上再多一层方言差异。
3. 额外：`logs` 只有成功消费日志（错误日志默认不入库，`common/init.go:196` `ERROR_LOG_ENABLED=false`）—— 这一点对缓存命中率**恰好无害**（CCH 也只统计 2xx），但仍然要靠 `LogConsumeEnabled`（`common/constants.go:89`，可被关闭）。
4. 全表扫 `other` 拉回 Go 解析 = 全表扫描，7d 窗口不可接受。

**→ B 确认不可行。**

### 3.3 加普通列的迁移风险 —— **安全，与 `perf_metrics` 索引陷阱不同类**

- `ChannelMetric` 在 `DB.AutoMigrate` 列表里：`model/main.go:338`（顺序迁移）与 `model/main.go:401`（`migrateDBFast` 并行列表）。GORM `AutoMigrate` 对已存在的表会**补齐缺失列**（`ALTER TABLE ... ADD COLUMN`），三库都支持。
- 项目内的反例（危险操作）是 **修改已有索引**：`prd.md:27` 与 `model/channel_metric.go:16-21` 的注释都记录了 GORM v1.25.2 只按索引名建索引、**永不修改已有索引**，所以给 `perf_metrics` 的唯一索引加 `channel_id` 在老库上会 MySQL 静默串数据 / PG-SQLite 报 `42P10`。
- **本次是「加普通 int64 列」，不触碰 `idx_channel_metric_channel_bucket` 唯一索引，也不加新索引** → 不落入那个陷阱。
- 需要注意的两点：
  1. 新列必须用 `gorm:"default:0"` 数值默认值，**不要用 `default:true` 类布尔 tag**（AGENTS.md；现表全部是 `default:0`，照抄即可）。老行的新列值为 0，正好是正确的「无数据」语义。
  2. SQLite 的 `ALTER TABLE ... ADD COLUMN` 支持（`model/main.go:621` 已有这个模式的先例，用于 `subscription_plans`），不涉及 `ALTER COLUMN`。
- 项目里不需要手写迁移：`logs.upstream_request_id` 等后加字段都只是结构体字段 + AutoMigrate。

### 3.4 **打点位置：现有 attempt 级打点拿不到 usage**

这是路线 A 唯一需要设计的地方。

现有 attempt 级打点：
- `controller/relay.go:290` `perfmetrics.RecordChannelAttempt(relayInfo, channel.Id, attemptStart, newAPIError == nil)`
- `controller/relay.go:593`（Midjourney 提交）、`controller/relay.go:720`（Task 提交）
- 签名 `pkg/perf_metrics/channel_metrics.go:86`：`(info *relaycommon.RelayInfo, channelId int, attemptStart time.Time, success bool)` —— **没有 usage 参数**

**`relayInfo` 不携带最终 usage**（已 grep 确认：`relay/common/relay_info.go` 无 `Usage` / `FinalUsage` / `LastUsage` 字段；结算路径 `PostTextConsumeQuota` 在 relay helper 内部被调用，如 `relay/compatible_handler.go:90`，usage 只作为函数参数流动，不回写 relayInfo）。

所以 `controller/relay.go:290` 处**无法**拿到 `cached_tokens`。

**正确挂点 = 成功结算路径**（usage + channelId 同时可得）：

| 路径 | 位置 | 可用量 |
|---|---|---|
| **文本/Claude/Gemini/Responses/Embedding/Rerank/Image** | `service/text_quota.go:397` `PostTextConsumeQuota`，最自然的位置是 `:540-542` 现有 `gopool.Go(func(){ perfmetrics.RecordRelaySample(...) })` 旁边 | `summary.CacheTokens`、`summary.CacheCreationTokens`、`summary.CacheCreationTokens5m/1h`、`summary.PromptTokens`、`summary.IsClaudeUsageSemantic`、`relayInfo.ChannelId` |
| **Realtime WSS** | `service/quota.go:252-254` | `usage.InputTokens/OutputTokens`；**缓存量未归一化进 `dto.Usage`**（realtime 用独立 `dto.RealtimeUsage`，`relay/channel/openai/relay_realtime.go:132`），本轮建议跳过 |
| **Audio** | `service/quota.go:383-385` `PostAudioConsumeQuota` | 缓存量恒为 0（`service/log_info_generate.go:305` 传 `cacheTokens=0`），跳过 |
| **Task / MJ** | `service/task_billing.go`、`relay/mjproxy_handler.go` | 无 usage 缓存概念，跳过 |

> **口径差异必须写进注释**：可用率是 **attempt 级**（`model/channel_metric.go:13-17`），而缓存命中率只能是 **成功请求级**（失败的 attempt 没有 usage）。同一张表里两组计数器口径不同 —— `attempt_count` 与新的 `cache_request_count` 不可互相当分母。**这是 A 路线最需要在代码和 UI 上说清楚的一点。**

另外 `relayInfo.IsChannelTest` 的排除逻辑（`pkg/perf_metrics/channel_metrics.go:87`）也要在新打点里照做，否则渠道测试流量会污染命中率。

### 3.5 需要加的字段（CCH 公式所需的最小集）

为了算出 `hitRateTokens` + `engagementRate` 两个指标，需要 **5 个新列**：

| 列 | 语义 | 累加条件 |
|---|---|---|
| `cache_request_count` | 成功结算的请求数（缓存统计的总分母，对应 CCH `totalRequests`） | 每个成功结算请求 +1 |
| `cache_signal_count` | 有缓存信号的请求数（CCH `cacheSignalRequests`） | `cacheRead>0 \|\| cacheWrite>0` 时 +1 |
| `cache_read_tokens` | Σ 缓存读取（CCH 分子） | **仅有信号的请求**累加 `CachedTokens` |
| `cache_write_tokens` | Σ 缓存写入（诊断用，也是 Claude 分母的一部分） | 仅有信号的请求累加 `CacheCreationTokensTotal()`（Claude 时用 5m+1h 归一化） |
| `cache_input_tokens` | Σ 归一化后的总输入（CCH 分母，按 §2.1 的 `inputLen` 口径算好再累加） | 仅有信号的请求累加 |

派生（后端算，不存）：
```
cache_hit_rate  = clamp01(cache_read_tokens / cache_input_tokens)     // = CCH hitRateTokens
engagement_rate = cache_signal_count / cache_request_count            // = CCH engagementRate
```

可选第 6 列 `cache_semantic_mixed`（或者存一个模式枚举）用来标记该桶是否混了 Claude 与 OpenAI 语义 —— 混了就说明 §2.3 的偏差在这个渠道上同时存在两种口径，数字更不可信。参考 `service/channel_affinity.go:856-863` 的 `mixed` 处理方式。

### 3.6 改动面估算

| 文件 | 改动 |
|---|---|
| `model/channel_metric.go` | 结构体加 5 列（`:22-29`）；`UpsertChannelMetric` 的 `DoUpdates` 加 5 个 `gorm.Expr`（`:40-49`）；`UpsertChannelMetric` 的空样本早退条件要改（`:36` 现在只看 `AttemptCount == 0`，缓存打点不带 attempt）；`ChannelMetricBucket` 加 5 字段（`:54-60`）；`GetChannelMetricBuckets` 的 `Select` 加 5 个 `SUM`（`:86`） |
| `pkg/perf_metrics/channel_metrics.go` | `atomicChannelBucket` / `channelCounters` 加 5 个计数器（`:27-37`）；`add` / `drain` / `addCounters` 同步（`:39-67`）；新增 `RecordChannelCacheUsage(...)` 打点函数；`flushCompletedChannelBuckets` 的 `drained.attemptCount == 0` 早退判断要改（`:115`，否则「只有缓存样本、没有 attempt 样本」的桶会被丢弃） |
| `service/text_quota.go` | `:540-542` 的 `gopool.Go` 里加一次缓存打点调用 |
| `controller/channel_monitoring.go` | `channelMonitoringCounters`（`:90-94`）与 `plus`（`:96-101`）加 5 个字段；新增 `channelMonitoringCacheHitRate` 派生函数（照 `:106-112` 的「先累加再相除」形状）；`channelMonitoringChannelSummary`（`:60-71`）与 `channelMonitoringOverall`（`:73-79`）加 DTO 字段；`buildChannelMonitoringChannels`（`:151-234`）的 row→counters 映射加 5 项 |
| `model/main.go` | **无需改动**（`ChannelMetric` 已在 `:338` 与 `:401`） |
| 前端 | 见 §4 |

无新表、无新索引、无新定时任务、无新 API 端点。**后端约 4 个文件，前端约 3 个文件。**

---

## 4. Q4 — 在 `web/src/features/channel-monitoring/` 的接线点

后端 DTO 与路由都已就位，只需加字段：

| 位置 | 改动 |
|---|---|
| `controller/channel_monitoring.go:60-71` `channelMonitoringChannelSummary` | 加 `CacheHitRate float64 \`json:"cache_hit_rate"\``、`CacheHasData bool \`json:"cache_has_data"\``（或 `EngagementRate`、`CacheSignalCount`） |
| `controller/channel_monitoring.go:73-79` `channelMonitoringOverall` | 同上（顶部汇总卡） |
| `controller/channel_monitoring.go:53-58` `channelMonitoringBucket` | **建议不加**。时间线格子现在按可用率着色（`:192-201`），再塞缓存率会让一个格子承载两个语义 |
| `web/src/features/channel-monitoring/types.ts:37-49` | `ChannelMonitoringChannelSummary` 加同名字段 |
| `web/src/features/channel-monitoring/types.ts:51-58` | `ChannelMonitoringOverall` 加同名字段 |
| `web/src/features/channel-monitoring/components/channel-availability-row.tsx:104-137` | 行尾现在是三段：Availability / Avg latency / Attempts（各自一个 `<div className='text-right'>`）。**加第四段「Cache hit」**，`cache_has_data === false` 时显示 `NO_VALUE`（`= '--'`，`:36`），与现有 `hasData ? ... : NO_VALUE` 完全同形 |
| `web/src/features/channel-monitoring/components/monitoring-overview-cards.tsx:76` | 顶部是 `grid gap-3 sm:grid-cols-3` 三张卡。加第四张 → 改成 `sm:grid-cols-2 lg:grid-cols-4`，复用同文件 `:29-54` 的 `OverviewCard` 组件 |
| i18n | 新文案键（英文原文作 key）加进 `web/src/i18n/locales/{lang}.json`；`hint` 里必须写明口径（「成功请求、且有缓存信号的请求」），因为它与同页其它指标的 attempt 级口径不同 |
| `web/src/features/channel-monitoring/api.ts` | **无需改动**（只是多返几个字段） |
| `router/api-router.go:37-41` | **无需改动**（`AdminAuth()` 已挂，权限工作量为 0） |
| 测试 | `controller/channel_monitoring_test.go`、`web/src/features/channel-monitoring/components/__tests__/*` 已有表驱动骨架，照加 |

**排序**：`buildChannelMonitoringChannels` 现在按可用率升序（最差在前，`:224-232`）。缓存命中率**不要**参与默认排序 —— 可用性页面的阅读目的是找坏渠道。若要按命中率排，应由前端做可切换的列排序。

---

## 5. Q5 — 统计侧的溢出与污染防护

缓存 token 数完全来自上游响应，属不可信输入。这里只是统计不是计费，但仍需：

1. **负值 clamp**：`CacheCreationTokensTotal()` 已经 clamp（`relaykit/dto/openai_response.go:280-283`），但 `PromptTokensDetails.CachedTokens` 与 `PromptTokens` **没有**。打点函数必须自己 clamp 到 `>= 0`，否则一个负的 `cached_tokens` 会让累加列变小甚至转负。现有 `add()` 对延迟已经有这个保护：`pkg/perf_metrics/channel_metrics.go:44-46` `if latencyMs > 0 { ... }` —— 照抄这个形状。
2. **单样本上界**：项目对用户提交的 token 类字段用 `maxTokensLimit = math.MaxInt32 / 2`（`relay/helper/valid_request.go:119-122`）。上游返回的 usage token **没有任何上界检查**（全项目 grep 未找到）。累加列是 `int64`，`MaxInt32/2` 级别的单样本要约 86 亿次才溢出 int64，实践上安全；但**建议对单样本做 `math.MaxInt32` 级别的 clamp**，以防一次 `18446744073686646784` 之类的异常值把整个桶的命中率毁掉（AGENTS.md 明确提到 `*uint` 字段接受超大正数、`>= 0` 检查不充分）。
3. **分母为 0**：派生函数必须先判 `cache_input_tokens <= 0` 再除，照抄 `controller/channel_monitoring.go:106-112` 的形状（`if counters.attemptCount <= 0 { return 0 }`）。
4. **结果 clamp 到 [0,1]**：对齐 CCH 的 `clampRatio01`。本项目内的等价物是 `service/channel_affinity.go` 没有做这一步，但 `common.QuotaFromFloat` 家族（`common/quota_math.go:112-166`）是计费侧的 clamp 设施 —— **统计侧不要用 quota 家族**（它们的边界是 `MaxQuota = math.MaxInt32`，语义是配额不是比率），自己写一个 `clamp01`。
5. **不要走 `common/quota_math.go`**：这些是配额转换 helper，统计计数器与配额无关，混用会让 `QuotaClamp` 审计事件被误报成计费异常。
6. **合成流量排除**：`relayInfo.IsChannelTest` 必须跳过（`pkg/perf_metrics/channel_metrics.go:87` 已有先例）。
7. **flush 失败回滚**：新计数器必须同时进 `addCounters`（`pkg/perf_metrics/channel_metrics.go:57-67`），否则 flush 失败时缓存计数会被静默丢弃而 attempt 计数被保留，两组数字从此对不上。

---

## 6. 相关文件索引

| 路径 | 说明 |
|---|---|
| `relaykit/dto/openai_response.go:223-284` | `dto.Usage` / `InputTokenDetails` / `CacheCreationTokensTotal()` |
| `relaykit/dto/claude.go:556-596` | `ClaudeUsage` 与 5m/1h getter |
| `service/billing_usage.go:99-213` | 三家 provider 的 BillingUsage → `dto.Usage` 归一化 |
| `service/text_quota.go:44-101`、`:228-395`、`:397-543` | `textQuotaSummary`、`cacheWriteTokensTotal`、`isLegacyClaudeDerivedOpenAIUsage`、`calculateTextQuotaSummary`、`PostTextConsumeQuota` |
| `service/tiered_settle.go:18-80` | `BuildTieredTokenParams` 的 `inputLen` —— CCH 分母的项目内等价物 |
| `pkg/billingexpr/expr.md:220-237` | **权威**：OpenAI vs Claude 的 prompt_tokens 语义差异 + `len` 定义 |
| `service/log_info_generate.go:92-163`、`:292-335` | `other` JSON 里所有缓存字段的写入点 |
| `relay/channel/openai/usage.go` | DeepSeek / 智谱 / Moonshot / llama.cpp 的缓存量回捞 |
| `relay/channel/claude/relay-claude.go:225-243` | Claude 原生 relay 的 usage 解析 |
| `service/channel_affinity.go:67-69`、`:742-929` | 项目内已有的缓存命中统计（请求级布尔口径）+ `cachedTokenRateMode` |
| `model/channel_metric.go` | `channel_metrics` 表 / upsert / rollup 查询 / 清理 |
| `pkg/perf_metrics/channel_metrics.go` | attempt 级热桶与打点 |
| `pkg/perf_metrics/flush.go:13-26` | 共用 flush 循环 |
| `controller/channel_monitoring.go` | 监控 API 与 DTO |
| `web/src/features/channel-monitoring/` | 前端页面（`types.ts` / `api.ts` / `components/*`）|
| `router/api-router.go:35-41` | `/api/channel-monitoring` + `AdminAuth()` |
| `common/quota_math.go:10-17` | 配额 clamp 边界（**统计侧不要用**） |
| CCH `src/repository/leaderboard.ts:819-850` | 排行榜版命中率算法 |
| CCH `src/repository/cache-hit-rate-alert.ts:107-320` | 告警版（含 `engagementRate` 与 `hitRateTokensEligible`） |
| CCH `src/repository/provider-cache-effectiveness.ts:43`、`:166`、`:218` | `cacheCoefficientBp`（本轮不做） |

CCH 源码读取方式：`git clone --depth 1 https://github.com/ding113/claude-code-hub` 到 `/tmp/cch-src`（只读，**未触碰 HK 服务器上的 CCH 容器或数据库**）。

---

## 7. Caveats / 未找到

- **未找到**任何对上游 usage token 数的上界校验（grep `maxTokensLimit` 只命中 `relay/helper/valid_request.go`，作用于用户请求的 `max_tokens` 系列，不作用于响应 usage）。
- **未找到** relay 请求级的 session / 请求序列标识 → CCH 的 `hitRateTokensEligible` 无法实现（§2.4）。
- **未找到** `logs` 表里记录 usage semantic 的真列；`other["usage_semantic"]` 只在 Claude 路径写（`service/text_quota.go:475`）。
- **未验证**：Realtime（`dto.RealtimeUsage`）路径的 `cached_tokens` 是否有归一化进 `dto.Usage` 的意图；本轮建议直接跳过 realtime。
- **未验证**：OpenAI 原生 `cache_write_tokens` 具体在哪些模型/计费模式下会返回（只确认了字段被解析、注释声明了它按 cache-creation 价计费）。§2.3 的偏高分析基于「多数 OpenAI Chat 请求不返回 cache_write_tokens」这一假设。
- **未实跑**任何 SQL 或迁移；§3.3 的 AutoMigrate 加列安全性基于 GORM 通用行为 + `model/main.go:338,401` 的注册位置，未在 MySQL/PG 上实测。
- **未估算**新增 5 列对 `channel_metrics` 行宽与 7d 数据量的实际影响（`design.md:37` 的原估算是 100 渠道 7d@5min ≈ 20.2 万行 / 12–50 MB；加 5 个 int64 约 +40 B/行，即 +8 MB 量级）。
- §2.3 的「45% vs 90%」是**口径示意**，不是实测数据。
