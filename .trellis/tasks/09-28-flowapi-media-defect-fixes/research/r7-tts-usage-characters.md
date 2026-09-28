# Research: R7 MiniMax TTS 按 `usage_characters` 计费的口径

- **Query**: usage_characters 的官方定义；与 keli 本地 rune 计数、ModelRatio 175/100 的口径对比；是否中文多收一倍；修法选项
- **Scope**: mixed（FlowAPI/keli 源码 + MiniMax 公开文档 platform.minimaxi.com/docs）
- **Date**: 2026-09-28

## 1. 代码现状

| 环节 | FlowAPI | keli |
|---|---|---|
| 输入上限 | 按 rune 计 ≤10000（`relay/channel/minimax/adaptor.go:61`，`MaxSpeechCharacters`，`extended.go:18`） | — |
| 预扣估算 | rune 数：`AudioRequest` 的 TokenType 为 `TokenTypeTextNumber`（`relaykit/dto/audio.go:32-41`），计数在 `service/token_counter.go:225-226`；音频的 RelayFormat 是 `RelayFormatOpenAIAudio`（`relay/common/relay_info.go:393`），不加 OpenAI 的 +3 | 相同（keli `service/token_counter.go:220-221`） |
| 响应校验 | `usage_characters ≤ 0` 或 `> 10000` → 502（`tts.go:190-192`） | 无 |
| usage | `PromptTokens = TotalTokens = usage_characters`（`tts.go:252-256`） | `PromptTokens = GetEstimatePromptTokens()`（rune），`TotalTokens = usage_characters`（keli `tts.go:245-249`） |
| 结算取值 | `summary.PromptTokens = usage.PromptTokens`（`service/text_quota.go:294-296`）；quota = PromptTokens × ModelRatio × GroupRatio（`:337`、`:392-394`） | 同一公式；TotalTokens 会被重算，不参与计费（keli `service/text_quota.go:185-187`）→ **keli 实际按 rune 计费** |

`8156149df` 把 FlowAPI 的 PromptTokens 从 `info.GetEstimatePromptTokens()` 改成了 usage_characters（该 commit 中 `tts.go` diff 的第 250-254 行）。

## 2. MiniMax 官方定义

- 《同步语音合成 HTTP》（`docs/api-reference/speech-t2a-http.md`）对 `extra_info` 两个字段的定义：
  - `usage_characters`：“计费字符数”；
  - `word_count`：“已发音的字数统计，包含汉字、数字、字母，不包含标点符号”（这是另一个指标）。
- 《按量计费》（`docs/guides/pricing-paygo.md`，语音合成 Info 块）原文：
  > 计费单位为字符数，以 10,000 个字符（输入）为单位。**1 个汉字算 2 个字符**；英文字母、希腊字母、标点符号、特殊符号、空格、回车等各算 1 个字符。
- 单价：speech-2.8-hd / 2.6-hd / 02-hd 为 **3.50 元/万字符**；对应的 turbo 为 **2.00 元/万字符**。
- 《音色快速复刻》说明 `usage_characters` 是“本次试听合成的计费字符数，可用于和账单进行对账”。
- **未确认**：
  - 停顿标记 `<#x#>`、语气词标签 `(laughs)`、行内注音 `(chu3)`、`pronunciation_dict` 是否计入、各算几个字符，官方没有说明。按通用规则，它们都是字母和符号，若按原文计数则每个字符算 1。
  - 日文假名、韩文是否按 ×2 计：未确认。
  - 文档示例里的数值（26/52、18/18）与示例文本对不上，是占位值，不能用来反推规则。

## 3. 口径对比

### ModelRatio 175/100 的来源

- `/Users/Zhuyu/.claude/skills/keli-api-ops/references/pricing-system.md`：
  - `:12-22`：QuotaPerUnit 500000，usd_exchange_rate=1 → ModelRatio 1 = 2 元/1M tokens；
  - `:76`：“TTS 按字符 → ModelRatio = 期望(元/1K 字符) / 0.002（如 0.35 → 175）”；
  - `:45-46`：期望价 0.385 / 0.22 元每千字符，分别等于 175×0.002×1.1 和 100×0.002×1.1。
- FlowAPI 导入清单 `09-22-flowapi-pricing-verification/target-pricing-proposal.json`：`currency: "CNY display / USDExchangeRate=1"`，speech-*-hd 为 175、turbo 为 100，`unit: "characters"`，`supplier_cost_verified: false`。
- 换算：175 × 0.002 = 0.35 元/千 = **3.5 元/万**，正好是 MiniMax hd 刊例价；100 → **2.0 元/万**，正好是 turbo 刊例价。
- 推断：175/100 就是按 MiniMax“计费字符”（汉字 = 2）的刊例价换算来的。依据是数值精确吻合；来源文档只写了“元/1K 字符”，没有写明是计费字符。

### 算例（hd，分组倍率 g）

| 文本 | MiniMax 计费字符 | 供应商成本 | FlowAPI（usage_characters） | keli（rune） |
|---|---|---|---|---|
| 1000 个汉字，无标点 | 2000 | 0.70 元 | 2000×175 = 350000 quota = **0.70·g 元** | 1000×175 = **0.35·g 元**（g=1.1 时 0.385，低于成本） |
| 1000 个英文字符 | 1000 | 0.35 元 | 0.35·g | 0.35·g |
| 中英混排 | — | — | 为 keli 的 1–2 倍 | — |

### 结论

- 相对供应商成本，按 usage_characters 计费**不会多收**。它和 MiniMax 自己的计费字符一致，FlowAPI 的收费 = 成本 × 分组倍率。
- 相对 keli 口径，纯中文多收 **1 倍**，混排时为 1–2 倍。反过来看，keli 的 rune 口径让中文只按成本的一半 × g 收费，g < 2 时低于成本。
- 因此，“中文多收一倍”只有在把 175/100 理解为“每个可见字符”时才成立；按其来源，应理解为“每个计费字符”。
- 需要用户拍板的是产品口径：对外定价中的“字符”指的是哪一种。

## 4. 附带发现：上限与 ×2 规则冲突

- 输入允许 ≤10000 rune（`adaptor.go:61`），响应却要求 usage_characters ≤10000（`tts.go:190`）。
- 文本超过约 5000 个汉字时，计费字符 >10000，FlowAPI 会在上游**已经成功之后**返回 502：不给音频，并退回预扣。测试 `media_test.go:149-160` 用 10001 这一档固化了这个上限。
- MiniMax 的“长度限制小于 10000 字符”（`speech-t2a-http.md`）指的是计费字符还是字数：**未确认**。两种情况的后果：
  - 若按字数：上游成功并扣费，FlowAPI 丢掉结果、自己承担成本；
  - 若按计费字符：上游先拒绝，没有成本，但 FlowAPI 的输入校验放行了一个必然失败的请求。
- 预扣按 rune、结算按计费字符，中文的结算额约为预扣的 2 倍。这只影响预扣是否足额，不影响最终金额。

## 5. 修法选项

| 选项 | 做法 | 影响 |
|---|---|---|
| 1 保留 usage_characters（现状）+ 修上限 | 响应上限改为 2×MaxSpeechCharacters，或与本地加权计数比较；定价页写明“计费字符，汉字按 2 计” | 与供应商账单一致；依赖上游数字，需按 AGENTS.md 保留饱和上界（已有粗上界） |
| 2 改用本地 rune（keli 口径） | PromptTokens 用预估值 | 中文收费减半，g=1.1 时低于成本。若把倍率翻倍到 350/200 让中文回本，英文又会多收 2 倍，无法两全 |
| 3 本地加权计数 | 汉字 ×2，其余 ×1，在本地计算 | 预扣与结算一致，可复算；规则可能与上游漂移（标记、其他文字），与供应商账单出现偏差 |
| 4 取 min(上游, 本地加权) | 两者取小 | 防止上游异常多报；本地规则少算时会少收 |
| 5 保留上游计数、倍率减半（87.5/50） | 只改价格 | 中文价格回到 keli 水平，但英文和标点降到成本的一半，会亏本；不推荐 |

补充：可以增加合理性校验，例如 `usage_characters ≤ 2×rune + 少量余量`，超出时按本地值计费并写审计。这对应 AGENTS.md 中“上游返回的数字要饱和处理”的要求。

## Caveats / Not Found

- MiniMax 对标记、SSML、其他文字的计数规则，以及 10000 上限的计量单位：未确认，按约束未发真实请求。
- FlowAPI 生产的 ModelRatio 和分组倍率以导入清单为准，生产 DB 未直接核实。
- release gate 的 TTS smoke quota 为 2160（`release-gate-2026-09-25.md:27`），测试文本未记录，无法用来反推口径。
