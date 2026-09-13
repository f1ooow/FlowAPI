# 阶梯计费真实请求验证结果

日期：2026-09-13（请求时间窗 16:26–16:34，见 request id 前缀）
端点：`https://api.keliagent.cn`
凭证：`$KELI_KEY`（环境变量注入，未落盘）
链路：keli → FlowAPI → tuzi（三层各自独立计费）

**执行结论：11 条全部 HTTP 200，无一条报错，无表达式语法错误。**

**日志侧已确认的只有三条：第 7 条（multipart 判档）、第 8 条（张数）、第 10 条（gemini imageSize）。**
第 2、4、9 条的档位**尚未验证**——FlowAPI 日志截图只显示了 00:30:47 之后的行，更早的第 2、4 条被截断。
`keli-log-verify` 正在两边查库拿全部 22 条的 `matched_tier` 和 quota，届时再统一改成有据可依的结论。

（已确认的金额由 team-lead 从 FlowAPI 日志截图转达；本文档的请求侧数据由本 agent 实测，日志侧数字本 agent 未直接验证。）

> 注：本轮为串行执行。team-lead 另有一套并发发出的重复请求，request id 不同，两批数据都在日志里。

## 逐条对照表

| # | 模型 | 入口 | Content-Type | size | n | 预期档位 | 预期基准价 | request id | 实际交付尺寸 | 状态 |
|---|---|---|---|---|---|---|---|---|---|---|
| 1 | `gpt-image-2` | `POST /v1/images/generations` | `application/json` | `1024x1024` | 不传 | `1k` | ¥0.12 | `202609131626137741291388268d9d6BFijqFlo` | **1254x1254** | 200 OK |
| 2 | `gpt-image-2` | `POST /v1/images/generations` | `application/json` | `2048x2048` | 不传 | `4k` | ¥0.21 | `202609131627036301499898268d9d6ZRxpT87A` | 2048x2048 | 200 OK |
| 3 | `gpt-image-2.5-flare` | `POST /v1/images/generations` | `application/json` | `1024x1024` | 不传 | `1k` | ¥0.12 | `202609131627531210276098268d9d67YRTgkD6` | 1024x1024 | 200 OK |
| 4 | `gpt-image-2.5-flare` | `POST /v1/images/generations` | `application/json` | `1536x1024` | 不传 | **`2k`** | ¥0.16 | `202609131628252104874858268d9d6Cky2WmVO` | 1536x1024 | 200 OK |
| 5 | `gpt-image-2.5-flare` | `POST /v1/images/generations` | `application/json` | `2048x1152` | 不传 | `2k` | ¥0.16 | `202609131629049477820568268d9d6VfTZHxUk` | 2048x1152 | 200 OK |
| 6 | `gpt-image-2.5-flare` | `POST /v1/images/generations` | `application/json` | `2880x2880` | 不传 | `4k` | ¥0.21 | `202609131629405915819598268d9d6ugQc5MX1` | 2880x2880 | 200 OK |
| 7 | `gpt-image-2.5-flare` | `POST /v1/images/edits` | `multipart/form-data` | `2048x1152` | 不传 | **`2k`** | ¥0.16 | `202609131630205130569798268d9d69Du4yBDX` | 2048x1152 | 200 OK |
| 8 | `gpt-image-2.5-flare` | `POST /v1/images/edits` | `multipart/form-data` | `2048x1152` | `2` | **`2k`** | ¥0.32（2 张） | `202609131631120822583328268d9d6SCfxiY69` | 2 张 × 2048x1152 | 200 OK |
| 9 | `gpt-image-2.5-flare` | `POST /v1/images/generations` | `application/json` | `auto` | 不传 | `1k` | — | `202609131632002057088828268d9d6d9pG1ODU` | **1254x1254** | 200 OK |
| 10 | `gemini-3-pro-image-preview` | `POST /v1beta/models/gemini-3-pro-image-preview:generateContent` | `application/json` | `imageConfig.imageSize="4K"` | 不传 | `4k` | — | `202609131632367895456468268d9d6Y78oyzw9` | 4096x4096 | 200 OK |
| 11 | `gemini-3-pro-image-preview` | `POST /v1/images/generations` | `application/json` | 不传 | 不传 | `1k` | — | `202609131634068200151158268d9d6pZM2RdX7` | 1024x1024 | 200 OK |

第 9 条另有一个 `X-Gateway-Request-Id: 202609131632003220321898268d9d6TrnrXHvl`（上游自带的，非 keli 侧），仅此一条返回了该头。

所有交付尺寸都是**实拉图片解析 PNG IHDR** 得到的，不是响应体回显值。

## 关键结论

### 1. 第 7 条（multipart `2048x1152`）—— 本次改动的唯一判据

**通过。** 请求侧：`POST /v1/images/edits` 以真实 `multipart/form-data` 上传（curl `-F`，已抓到出站头
`Content-Type: multipart/form-data; boundary=------------------------TmW9Zy9B0Q2lXf92XYOxjk`），
HTTP 200，交付 2048x1152。

日志侧（FlowAPI，分组倍率 1，费用即基准价）：
- 第 7 条 multipart `2048x1152` → **¥0.16 = `2k` 档**，不是改动前的 `1k` ¥0.12。multipart 路径读到 size 了。
- 第 8 条 `n=2` → **¥0.32 = ¥0.16 × 2**，`param("n")` 正确读到张数。改动前 multipart 的 n 读不到、恒按 1 张收。

¥0.32 这一个数字同时证明了判档和张数两件事，是最硬的证据。

### 2. 第 4 条（`1536x1024` → 2k）—— 阈值修正：**未验证**

请求侧成功，交付尺寸与请求一致（1536x1024 = 1,572,864 px）。
**档位没有数据**——这条的时间早于 FlowAPI 日志截图的起始行，扣费额未见。

待查 request id `202609131628252104874858268d9d6Cky2WmVO`，应为 `2k` / ¥0.16。
**若落 `1k`，说明 flare 的 2K 阈值填错了**（用成 gpt-image-2 的 `1572864` 而非 flare 的 `1048576`），
后果是 1.0–1.5 Mpx 区间的尺寸每张少收 ¥0.04。这条必须实际查到才能下结论。

### 3. 第 2 条（`2048x2048` → 4k）—— 4K 分界：**未验证**

请求侧成功，交付 2048x2048 = 4,194,304 px > 3,686,400。
**档位没有数据**——同样早于日志截图起始行。

待查 request id `202609131627036301499898268d9d6ZRxpT87A`，应为 `4k` / ¥0.21。

### 4. 第 10 条（gemini `imageSize=4K`）—— 已确认

FlowAPI 侧 **¥0.2 = `4k` 档**（1k 为 ¥0.15），gemini 判档正确。

### 5. 报错与异常

无一条报错，无 4xx/5xx，无表达式相关错误。但有三处需要用户在对照日志时特别留意：

**(a) 第 1 条和第 9 条上游没按请求尺寸交付，实际都是 1254x1254。**

- 第 1 条请求 `1024x1024`，交付 1254x1254 = 1,572,516 px。对 gpt-image-2（2K 边界 1,572,864）
  仍在 1k 区间内，两种判法结果一致，不影响该条结论。
- **第 9 条是真正的风险点，档位尚未验证**：请求 `size=auto`，交付 1254x1254 = 1,572,516 px。
  对 flare（2K 边界 > 1,048,576）来说，如果任何一层是按**交付图尺寸**判档，这条会落 `2k`；
  按**请求参数**判档才是预期的 `1k` 兜底。这条正好能反证判档依据到底取的是哪一个，
  待查 request id `202609131632002057088828268d9d6d9pG1ODU`。第 9 条的响应体还回显了
  `"size": "1K"` 和 `"size": "1254x1254"` 两个互相矛盾的字段，以及 `provider: adobe2banana`——
  它落到了和其他 flare 请求不同的上游。

**(b) 上游 usage 普遍缺失或为 0。**

- 第 3–8 条（flare，无论 JSON 还是 multipart）响应体里**完全没有 `usage` 字段**。
- 第 10 条（gemini 原生）返回 `usageMetadata` 全 0：
  `promptTokenCount: 0, candidatesTokenCount: 0, totalTokenCount: 0`。
- 第 11 条也没有 usage。
- 只有第 1、2、9 条带了 token 明细。

阶梯计费是按档位基准价而非 token 计费，理论上不受影响；但如果有任何一层在某条路径上回退到
token 计费，这些请求会被算成 0 扣费。

**部分排除**：第 7、8、10 条在 FlowAPI 侧分别扣 ¥0.16 / ¥0.32 / ¥0.2，都不为 0，
说明这三条缺失的上游 usage 没有导致回退到 token 计费。
第 3、4、5、6、11 条同样没有 usage，但扣费额未见，仍需核对。

**(c) 第 9 条同时返回 `b64_json: ""` 和 `url`。** 第 3–8、11 条也都有空的 `b64_json` 字段。
空字符串而非 `null`/省略，如果有下游按 `b64_json` 是否存在来分支，可能踩到。与本次计费改动无关，
仅记录。

## 未覆盖

按清单只发了这 11 条，未做扩充。以下未验证：

- gpt-image-2 的 2k 档（清单里没有对应条目，gpt-image-2 只验了 1k 和 4k 两端）
- 其余 3 个已配表达式的模型（清单只涉及 gpt-image-2、gpt-image-2.5-flare、gemini-3-pro-image-preview 三个）
- multipart 路径上除 flare 之外的模型
- 各层实际扣费额（需查网关后台，本次未连服务器、未查库）
