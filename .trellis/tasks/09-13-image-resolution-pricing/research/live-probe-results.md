# Research: 生产网关实测探针结果（size 参数 + usage token）

- **Query**: 验证 (1) 客户端 `size` 参数的实际取值分布与缺失率；(2) 上游 usage 里有没有可用于阶梯计价的图像 output token
- **Scope**: mixed（生产网关实测 curl + HK 生产库交叉验证 + 代码确认）
- **Date**: 2026-09-13
- **端点**: `https://flowapi.robusta.top/`
- **凭据**: 通过环境变量 `$FLOWAPI_KEY` 传入，**未写入任何文件**
- **实测成本**: 8 次请求，合计约 **$1.15 / ¥1.15**

---

## TL;DR — 两条路线的结论

| 路线 | 可行性 | 原因 |
|---|---|---|
| **按 `img_o`（图像 output token）计价** | **不可行** | 值与分辨率**反向**、大面积缺失、上游自标 `"source": "estimated"` |
| **按 `param("size")` 判档** | **gpt-image 系可行；gemini 系不可行** | size 恒可读，但交付分辨率与请求值脱钩；gemini 上 `size` 完全不控制档位（见 `upstream-cost-probe.md` 文末复测） |

**最关键的一条实测**：同一个 `gpt-image-2`，同一句 prompt，唯一变量是 size —

| 请求 size | 实际交付图片 | `img_o`（output image tokens） | 扣费 |
|---|---|---:|---:|
| `1024x1024` | 1254×1254 PNG | **1056** | ¥0.15 |
| `4096x4096` | 2880×2880 PNG (8.37 MB) | **659** | ¥0.15 |

4K 的图片像素是 1K 的 **5.3 倍**，token 数却只有 **0.62 倍**。按 `img_o` 计价会让**大图比小图更便宜**。这条路直接排除。

---

## 一、实测请求全记录（8 次）

所有请求 prompt 统一为 `"a red cube"`，`n=1`，走 `/v1/images/generations`（除标注外）。图片本体未记录，只记尺寸与字节数。

### 第 1 轮：gpt-image-2 的 token 是否随分辨率变化

**请求 1 — `size: "1024x1024"`**

```json
{
  "created": 1789309169,
  "data": [{ "height": 1254, "width": 1254, "revised_prompt": "a red cube", "url": "<redacted>" }],
  "usage": {
    "input_tokens": 3,
    "output_tokens": 1056,
    "total_tokens": 1059,
    "input_tokens_details":  { "text_tokens": 3, "image_tokens": 0, "cached_tokens": 0 },
    "output_tokens_details": { "text_tokens": 0, "image_tokens": 1056, "reasoning_tokens": 0 }
  }
}
```

**请求 2 — `size: "4096x4096"`**

```json
{
  "background": "opaque",
  "created": 1789309251,
  "data": [{
      "byteSize": 8368271, "contentType": "image/png",
      "height": 2880, "width": 2880,
      "size": "4K", "prompt": "a red cube", "revised_prompt": "a red cube",
      "key": "generated-images/org-<redacted>/2026/09/13/<redacted>.png",
      "url": "<redacted>"
  }],
  "model": "gpt-image-2",
  "output_format": "png",
  "provider": "adobe2banana",
  "quality": "low",
  "size": "2880x2880",
  "usage": {
    "generated_images": 1,
    "input_tokens": 3,
    "input_tokens_details":  { "text_tokens": 3 },
    "output_tokens": 659,
    "output_tokens_details": { "image_tokens": 659 },
    "source": "estimated",
    "total_tokens": 662
  }
}
```

**要点**：
- 请求 4096x4096 → 上游**没给 4096**，给的是 2880×2880，并自己标注 `"size": "4K"`、`"quality": "low"`
- 请求 1024x1024 → 上游给的是 **1254×1254**，也不是 1024
- 两次命中的上游实现不同（请求 2 暴露了 `provider: "adobe2banana"`，请求 1 没有该字段），usage 结构也不同
- 请求 2 的 usage 明确标 **`"source": "estimated"`** —— 上游自己承认这是**估算值**，不是真实计量

### 第 2 轮：Gemini 图像模型

**请求 A — `gemini-3.1-flash-image-preview`，`/v1/images/generations`，`size: "4096x4096"`**

```json
{
  "created": 1789309307,
  "data": [{ "b64_json": "", "revised_prompt": "", "url": "<redacted>" }]
}
```

**完全没有 `usage` 字段。**

**请求 B — `gemini-3-pro-image-preview`，`/v1/chat/completions`**

```json
{
  "id": "chatcmpl-<redacted>",
  "model": "gemini-3-pro-image-preview",
  "object": "chat.completion",
  "created": 1789309328,
  "choices": [{ "index": 0, "finish_reason": "stop",
    "message": { "role": "assistant", "content": "![image](<redacted png url>)" } }],
  "usage": {
    "prompt_tokens": 3,
    "completion_tokens": 1270,
    "total_tokens": 1273,
    "usage_source": "gemini_response_result",
    "output_image_count": 1,
    "output_image_tokens_included_in_completion": true,
    "output_image_tokens_source": "upstream_modality_details",
    "prompt_tokens_details":     { "cached_tokens": 0, "text_tokens": 3, "audio_tokens": 0, "image_tokens": 0 },
    "completion_tokens_details": { "text_tokens": 150, "audio_tokens": 0, "image_tokens": 1120, "reasoning_tokens": 0 }
  }
}
```

**要点**：Gemini **chat 路径**是唯一给出可信 image token 的（`image_tokens: 1120`，`output_image_tokens_source: "upstream_modality_details"`，非 estimated）。但 chat 路径**没有 `size` 参数概念**，日志 `content` 为空。

**请求 D — `gemini-3-pro-image-preview`，`/v1/images/generations`，`size: "4096x4096"`**

```json
{
  "created": 1789309417,
  "data": [{ "b64_json": "", "revised_prompt": "", "url": "<redacted>" }]
}
```

实测拉取返回的图片：**PNG 1024×1024，195,292 字节**。

**要点**：请求 4K，**实际交付 1024×1024**，且无 usage。

> **后续复测纠正**：该结论当时基于单次采样。`upstream-cost-probe.md` 文末的 12 次跨渠道复测证明：`size` 并非「完全忽略」，它会影响**宽高比**，只是从不改变**分辨率档位**；且不同模型默认档不同（`gemini-3.1-flash-image` 默认就是 2K 而非 1K）。结论方向不变——gemini 不能用 `size` 判档。

### 第 3 轮：不传 size 的默认行为

**请求 C — `gpt-image-2`，不带 `size` 参数**

```json
{
  "created": 1789309370,
  "data": [{ "height": 1254, "width": 1254, "revised_prompt": "a red cube",
             "url": "<remote PNG 1254x1254, Content-Length=1004707>" }],
  "usage": {
    "input_tokens": 3, "output_tokens": 1056, "total_tokens": 1059,
    "input_tokens_details":  { "text_tokens": 3, "image_tokens": 0, "cached_tokens": 0 },
    "output_tokens_details": { "text_tokens": 0, "image_tokens": 1056, "reasoning_tokens": 0 }
  }
}
```

**要点**：不传 size 的结果与传 `1024x1024` **完全一致**（1254×1254、1056 tokens）。→ **默认档设为 1K 与上游实际行为一致。**

### 第 4 轮：flare / sunburst 的 usage 结构

**请求 E — `gpt-image-2.5-flare`，`size: "4096x4096"`**

```json
{
  "background": "auto", "created": 1789309468,
  "data": [{ "b64_json": "", "url": "<remote PNG 2880x2880, Content-Length=3873524>" }],
  "model": "gpt-image-2.5-flare", "output_format": "png",
  "quality": "auto", "size": "2880x2880"
}
```

**没有 `usage` 字段。**

**请求 F — `gpt-image-2.5-sunburst`，`size: "4096x4096"`**

```json
{
  "background": "opaque", "created": 1789309496,
  "data": [{
      "byteSize": 822343, "contentType": "image/png",
      "height": 2880, "width": 2880, "size": "4K",
      "prompt": "a red cube", "revised_prompt": "a red cube",
      "key": "generated-images/org-<redacted>/2026/09/13/<redacted>.png",
      "url": "<remote PNG 2880x2880, Content-Length=822343>"
  }],
  "model": "gpt-image-2.5-sunburst", "output_format": "png",
  "provider": "zf-banana", "quality": "medium", "size": "2880x2880",
  "usage": {
    "generated_images": 1,
    "input_tokens": 3, "input_tokens_details": { "text_tokens": 3 },
    "output_tokens": 1483, "output_tokens_details": { "image_tokens": 1483 },
    "source": "estimated", "total_tokens": 1486
  }
}
```

**要点**：flare 和 sunburst 的 usage 结构**不一致** —— flare 完全没有 usage，sunburst 有但标 `"source": "estimated"`。同一个 family 无法复用同一套 token 判档逻辑。

**更致命的对比**：同样是 **2880×2880** 的输出图片，
- `gpt-image-2` 报 `image_tokens: 659`
- `gpt-image-2.5-sunburst` 报 `image_tokens: 1483`

差 2.25 倍。token 数由上游各自估算，跨模型、跨渠道完全不可比。

---

## 二、交叉验证：网关自己在计费时看到了什么

实测后立即从 HK 生产库 `logs` 表捞出对应行（时间窗 22:20–22:25 CST）：

| 时间 (CST) | model_name | ch | `content` 记录的 size | pt | ct | quota | ¥ | model_price | path |
|---|---|---:|---|---:|---:|---:|---:|---:|---|
| 22:20:05 | gpt-image-2 | 55 | `大小 1024x1024, 品质 standard, 生成数量 1` | 3 | 1056 | 75000 | 0.15 | 0.15 | images/generations |
| 22:20:52 | gpt-image-2 | 55 | `大小 4096x4096, 品质 standard, 生成数量 1` | 3 | **659** | 75000 | 0.15 | 0.15 | images/generations |
| 22:21:47 | gemini-3.1-flash-image-preview | 55 | `大小 4096x4096, 品质 standard, 生成数量 1` | 1 | **0** | 100000 | 0.20 | 0.2 | images/generations |
| 22:22:09 | gemini-3-pro-image-preview | 55 | **（空）** | 3 | 1270 | 100000 | 0.20 | 0.2 | chat/completions |
| 22:23:14 | gpt-image-2 | 55 | `品质 standard, 生成数量 1`（**无「大小」段**） | 3 | 1056 | 75000 | 0.15 | 0.15 | images/generations |
| 22:23:37 | gemini-3-pro-image-preview | 55 | `大小 4096x4096, 品质 standard, 生成数量 1` | 1 | **0** | 100000 | 0.20 | 0.2 | images/generations |
| 22:24:29 | gpt-image-2.5-flare | 55 | `大小 4096x4096, 品质 standard, 生成数量 1` | 1 | **0** | 50000 | 0.10 | 0.1 | images/generations |
| 22:24:56 | gpt-image-2.5-sunburst | 55 | `大小 4096x4096, 品质 standard, 生成数量 1` | 3 | 1483 | **178** | **0.00036** | -1 (ratio 0.06) | images/generations |

**这张表直接回答了「网关计费时看到的是什么」**：

1. **`content` 记录的是客户端请求的 size，不是实际交付的 size。** 请求 4096x4096、实际交付 2880x2880（甚至 1024x1024），日志里一律记 `大小 4096x4096`。
2. **一刀切确认无误**：8 次请求中 7 次扣费完全只由 `model_price` 决定，与 size、与 token 数都无关。
3. **不传 size 时 `content` 里就没有「大小」段**（22:23:14 那行），`param("size")` 会取不到值 → 必须有默认档兜底。
4. **sunburst 的计费事故实测复现**：一张 2880×2880 的图只扣了 **178 quota = ¥0.00036**，而同尺寸的 gpt-image-2 扣 ¥0.15。相差 **417 倍**。
5. **chat 路径的 `content` 为空**（22:22:09 那行）→ Gemini 走 chat completions 时，日志里没有任何 size 信息。

---

## 三、回答团队 lead 的两个补充问题

### 问题 1：客户端到底传没传 `size`

数据来自最近 30 天 `logs.content`（`大小 …` 段由 `relay/image_handler.go:139` 写入，取值即客户端请求体里的 `size`）。

| 模型 | 显式传了 size | 未传 size | 传了但是 `auto` | size 可判档率 |
|---|---:|---:|---:|---:|
| `gpt-image-2` | 383 / 405 | 22 | 34 | **86.2%**（349/405） |
| `gpt-image-2.5-flare` | 73 / 73 | 0 | 2 | **97.3%**（71/73） |
| `gpt-image-2.5-sunburst` | 30 / 30 | 0 | 0 | **100%** |
| `gemini-3.1-flash-image-preview` | 3 / 31 | 28 | 0 | **9.7%** |
| `gemini-3-pro-image-preview` | 3 / 15 | 12 | 0 | **20.0%** |

**实际取值分布**（完整明细见 `hk-production-logs.md` 第 4a 节）：

- **`gpt-image-2` 最高频的两个尺寸是非方形横版**：`2048x1152`（168 次）和 `2048x896`（115 次），合计占 70%
- 方形 `1024x1024` 共 24 次，`2048x2048` 5 次，`4096x4096` 3 次
- 出现大量**非标准尺寸**：`1254x1254`、`1672x941`、`1664x944`、`2288x1824`、`3001x3001`、`1573x1573`、`3344x1882`、`2001x2001`、`3456x1728`、`2880x2880`、`2208x3360`
- 竖版也有：`1024x1536`、`2160x3840`、`1152x2048`、`1100x1400`

> 注意 `1254x1254` 和 `2880x2880` —— 这正是上游**实际交付**的尺寸（见第一轮实测）。说明有客户端把上一张图的实际尺寸回填进下一次请求。
>
> 还有 1 条 `gemini-3-pro-image-preview` 直接传了字面量 **`大小 4K`**（非 `WxH`）。

**结论**：
- gpt-image 三个模型：`param("size")` 判档**可用**（86%~100%）
- gemini 两个模型：`param("size")` 判档**基本不可用**（9.7% / 20%），且实测证明**上游根本不看这个参数**
- 判档表达式必须能处理：`WxH` 数值、字面量 `1K`/`2K`/`4K`、`auto`、以及字段缺失四种情况
- **默认档应为 1K** —— 实测证明不传 size 与传 1024x1024 的上游行为完全一致

### 问题 2：usage 里有没有可用的图像 output token

`img_o` 的取值来源是 `service/tiered_settle.go:41`：

```go
imgO := float64(usage.CompletionTokenDetails.ImageTokens)
```

即上游 usage 的 `completion_tokens_details.image_tokens` / `output_tokens_details.image_tokens`。

**实测到的 `img_o` 值**：

| 模型 | 路径 | 请求 size | 实际交付 | `img_o` | usage 可信度 |
|---|---|---|---|---:|---|
| gpt-image-2 | images/generations | 1024x1024 | 1254×1254 | 1056 | 无 source 标注 |
| gpt-image-2 | images/generations | 4096x4096 | 2880×2880 | **659** | **`"source": "estimated"`** |
| gpt-image-2 | images/generations | （不传） | 1254×1254 | 1056 | 无 source 标注 |
| gpt-image-2.5-flare | images/generations | 4096x4096 | 2880×2880 | **无 usage** | — |
| gpt-image-2.5-sunburst | images/generations | 4096x4096 | 2880×2880 | 1483 | **`"source": "estimated"`** |
| gemini-3.1-flash-image-preview | images/generations | 4096x4096 | （未取尺寸） | **无 usage** | — |
| gemini-3-pro-image-preview | images/generations | 4096x4096 | 1024×1024 | **无 usage** | — |
| gemini-3-pro-image-preview | chat/completions | （无此参数） | — | 1120 | `upstream_modality_details`（可信） |

**历史日志佐证 usage 缺失是常态，且按渠道分化**（最近 30 天，`completion_tokens > 0` 的比例）：

| 模型 | 渠道 55 (tuzi) | 渠道 54 (Packy) | 渠道 46 (ZIVV nanobanana) | 其他渠道 |
|---|---:|---:|---:|---:|
| `gpt-image-2` | 98.6% (68/69) | 100% (13/13) | — | 100%（8/30/40/41/42 共 319 条） |
| `gpt-image-2.5-flare` | **6.3%** (4/64) | 100% (9/9) | — | — |
| `gpt-image-2.5-sunburst` | **9.5%** (2/21) | 100% (9/9) | — | — |
| `gemini-3.1-flash-image-preview` | **0%** (0/27) | — | 100% (4/4) | — |
| `gemini-3-pro-image-preview` | **14.3%** (2/14) | — | 100% (1/1) | — |

渠道 55（tuzi image）承载了 flare/sunburst/gemini 的绝大部分流量，而它对这些模型**基本不回传 usage**。

**历史日志里 token 与分辨率的关系也自相矛盾**（`gpt-image-2`，同模型不同尺寸的 `completion_tokens` 实测区间）：

| size + quality | 样本数 | completion_tokens |
|---|---:|---|
| `1024x1024` + high | 4 | 恒为 **4160** |
| `1536x1024` + high | 3 | 恒为 **6240** |
| `1024x1024` + medium | 16 | 1056 ~ 4096 |
| `2560x1440` + medium | 15 | 1842 ~ **14400** |
| `2160x3840` + medium | 2 | 3336 ~ **32400** |
| `4096x4096` + medium | 3 | 659 ~ 4541 |
| `2048x1152` + high | 168 | 1413 ~ 13824 |

- `4160`（1024x1024 high）和 `6240`（1536x1024 high）精确等于 OpenAI 官方 gpt-image-1 的 output image token 表，说明**部分渠道**确实按官方规则计量
- `14400 = 2560×1440/256`、`32400 = 2160×3840/256`，说明**另一些渠道**按「像素数 ÷ 256」自行估算
- 但 `4096x4096` 却出现 **659**（远低于 1024x1024 的 1056），说明**还有渠道乱报**

**结论：`img_o` 不能作为判档依据。** 四个独立的失败原因，任何一个单独都足以否决：

1. **值与分辨率反向** —— 实测 4K→659 < 1K→1056
2. **大面积缺失** —— 主力渠道 55 对 flare/gemini 的 usage 覆盖率 0%~14.3%，`img_o` 会是 0
3. **上游自标 estimated** —— `"source": "estimated"` 意味着上游自己都不保证准确
4. **跨模型不可比** —— 同为 2880×2880，gpt-image-2 报 659、sunburst 报 1483

唯一的例外是 **Gemini 走 `/v1/chat/completions`** 时（`img_o = 1120`，来源 `upstream_modality_details`），但该路径没有 size 参数、日志 `content` 为空，且只覆盖两个 gemini 模型的一小部分流量。

---

## 四、`gpt-image-1` / `gpt-image-2.5` 影响面（本次不改，供判断）

| 模型 | 最近 30 天请求数 | 计费总额 | 当前 `ModelPrice` | 在 `abilities` 中 |
|---|---:|---:|---:|---|
| `gpt-image-1` | **0** | ¥0 | 未配 `ModelPrice`；配了 `ModelRatio` 2.5 + `CompletionRatio` 8 + `defaultImageRatio` 2 | **否**（未启用） |
| `gpt-image-2.5` | 10（全部在 2026-09-10） | ¥1.50 | 0.15 | **否**（已下线） |

两者都没有活跃流量，不改不影响任何东西。`gpt-image-2.5` 的 `ModelPrice: 0.15` 是模型下线后的**残留配置**。

---

## 五、结论与需要决策的点

### 已经确定的事实

1. **`img_o` 路线不可行**（四个独立否决理由，见上）
2. **`param("size")` 是唯一可行信号**，对 gpt-image 三个模型可判档率 86%~100%
3. **Gemini 模型在 OpenAI 兼容路径上没有可行的分辨率信号** —— `size` 覆盖率仅 9.7%/20%，且 12 次跨渠道复测证明该参数**不控制分辨率档位**（`1024x1024` 与 `4096x4096` 交付面积相同）。真正的档位控制在原生路径的 `imageConfig.imageSize`
4. **默认档 = 1K 与上游实际行为一致**（实测：不传 size ≡ 传 1024x1024）
5. **判档表达式必须处理 4 种输入形态**：`WxH` 数值、字面量 `1K`/`2K`/`4K`、`auto`、字段缺失

### 需要产品侧决策的点（只陈述，不做建议）

1. **计费语义偏差**：`param("size")` 计的是「用户**要求**的尺寸」，而非「实际**拿到**的尺寸」。实测 `4096x4096` 实际只交付 2880×2880（gpt-image-2）或 1024×1024（gemini-3-pro）。按 4K 档收费但交付 1K 图，是否可接受需要产品拍板。**注**：gemini 侧该问题已由文末复测定论——应直接删掉 size 判档；本条仅对 gpt-image 系仍然适用。
2. **Gemini 两个模型怎么办**：90% 的请求没有 size 信息，且上游不认 size。可选项是维持一刀切，或用别的信号，或接受绝大多数请求落默认档。
3. **非方形尺寸如何归档**：最高频的 `2048x1152` / `2048x896` 都是横版，按最长边算是 2K，按像素面积算也是 2K，两种规则在这里一致；但 `2560x1440` / `3456x1728` / `2288x1824` 两种规则会分歧（详见 `hk-production-logs.md` 第 4b 节）。
4. **`gpt-image-2.5-sunburst` 的计费事故要一并修**：实测一张 2880×2880 只收 ¥0.00036，与同尺寸 gpt-image-2 的 ¥0.15 相差 417 倍。根因是它在 `ModelPrice` 里不存在，落到了 `ModelRatio` 0.06 的按 token 计费。

### 本次实测的成本与足迹

- 8 次生产请求，合计约 **$1.15 / ¥1.15**（`billing_source` 均为 `unlimited`，未扣真实钱包余额）
- 全部为只读性质的正常 API 调用，**未修改任何生产配置、未重启服务、未写数据库**
- 数据库侧只执行 SELECT

---

## 附：本次新增确认的代码位置

| 文件:行 | 作用 |
|---|---|
| `service/tiered_settle.go:41` | `imgO := float64(usage.CompletionTokenDetails.ImageTokens)` —— `img_o` 的唯一来源 |
| `service/tiered_settle.go:39` | `img := float64(usage.PromptTokensDetails.ImageTokens)` —— `img`（输入侧）来源 |
| `service/text_quota.go:301` | `summary.ImageTokens = usage.PromptTokensDetails.ImageTokens` —— 日志 `other.image_output` 记的是**输入**图像 token，不是输出 |
| `setting/billing_setting/tiered_billing.go:12-14` | `BillingModeRatio` / `BillingModeTieredExpr` / `BillingModeField` 常量 |
| `service/tiered_settle.go:104,204` | `snap.BillingMode != "tiered_expr"` 判断入口 |
| `model/pricing.go:403` | 定价接口暴露 `billing_mode` |
| `pkg/billingexpr/expr.md:46` | `img_o` = 图片输出 token 数 |
| `pkg/billingexpr/expr.md:79-81` | `param(path)` 走 gjson 读请求体；`has(source, substr)` 子串判断 |
| `pkg/billingexpr/expr.md:183` | 构建 `RequestInput`（headers + body）供 `param()` / `header()` 使用 |
