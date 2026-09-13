# Research: HK 生产服务器图像模型真实请求日志

- **Query**: 调查 HK 生产库里 5 个图像模型的真实请求日志，确认「按分辨率计费」在数据层面是否可行、当前一刀切计了多少钱
- **Scope**: mixed（HK 生产 PostgreSQL 只读查询 + 本仓库代码验证）
- **Date**: 2026-09-13
- **数据窗口**: `created_at >= now() - 30 days`，即 2026-08-14 ~ 2026-09-13
- **连接方式**: `ssh hk`（野草云-HK，端口 39070）→ `docker exec -i flowapi-postgres psql -U flowapi -d flowapi`
- **操作性质**: 全程只读 SELECT，未改任何配置、未重启服务、未写库

---

## TL;DR（结论先行）

> **配套文档**：`live-probe-results.md` —— 2026-09-13 对生产网关做的 8 次实测请求，验证了 `size` 参数与 usage token 的真实行为。**结论：按 `img_o`（图像输出 token）计价不可行，`param("size")` 是唯一可行信号。** 本文档第 2 节关于「分辨率是否可得」的结论在那里有实测佐证。

1. **「五个模型」确认了**：`gpt-image-2.5` 已在 2026-09-10 拆成 `gpt-image-2.5-flare` 和 `gpt-image-2.5-sunburst` 两个独立模型。裸名 `gpt-image-2.5` 已从 `abilities` 表下线。
2. **分辨率在数据层面可行** — 但**不在 `other` 字段里**，而在 `logs.content` 里，格式为中文串 `大小 2048x1152, 品质 high, 生成数量 1`。来源 `relay/image_handler.go:139`。
3. **计费链路已经有现成挂载点**：`relay/helper/price.go:148-150` 的 `modelPrice * meta.ImagePriceRatio`。目前 `ImagePriceRatio` 只对 `dall-e*` 前缀生效（`relaykit/dto/openai_image.go:137`），gpt-image / gemini 系列恒为 1.0，所以分辨率被无视。
4. **`gpt-image-2.5-sunburst` 当前是计费事故**：它没有配 `ModelPrice`，落到了按 token 计费（`ModelRatio` 0.06），30 次请求一共只计了 **1020 quota ≈ ¥0.002**，平均每张 ¥0.00007。
5. **Gemini 两个模型的分辨率大部分拿不到**：`gemini-3.1-flash-image-preview` 31 次请求里 28 次（90.3%）没有任何 size 信息，因为走的是原生 `/v1beta/...:generateContent` 或 `/v1/chat/completions` 路径，不经过 `image_handler.go`。
6. **当前零真实收入**：全部 564 条图像消费日志的 `billing_source` 都是 `unlimited`，`UnlimitedFunding` 明确「不动用户钱包余额」。所以下面所有金额都是**记账口径的名义值，不是已实现收入**。

---

## 问题 1：实际模型名清单

### 最近 30 天 `logs` 表实际出现的图像模型

`type=2` 是消费日志，`type=5` 是错误日志（错误日志 quota 恒为 0）。

| model_name | type=2 请求数 | type=2 总 quota | type=5 错误数 | 首次出现 | 最后出现 |
|---|---:|---:|---:|---:|---|---|
| `gpt-image-2` | 405 | 21,914,501 | 30 | 2026-08-28 | 2026-09-13 |
| `gpt-image-2.5-flare` | 73 | 3,650,000 | 55 | 2026-09-12 | 2026-09-13 |
| `gemini-3.1-flash-image-preview` | 31 | 2,860,000 | 19 | 2026-09-10 | 2026-09-13 |
| `gpt-image-2.5-sunburst` | 30 | **1,020** | 4 | 2026-09-12 | 2026-09-13 |
| `gemini-3-pro-image-preview` | 15 | 1,430,000 | 15 | 2026-09-10 | 2026-09-13 |
| `gpt-image-2.5`（裸名，已下线） | 10 | 750,000 | 0 | 2026-09-10 | 2026-09-10 |
| `gemini-3.1-flash-image` | 0 | 0 | 2 | 2026-09-13 | 2026-09-13 |

**合计 type=2：564 条**（含已下线的 `gpt-image-2.5`）；目标 5 模型合计 **554 条**。

### `abilities` 表当前启用的图像模型（权威清单）

```
 gemini-3-pro-image-preview     | image,【职刻】Image | 2 channels | enabled
 gemini-3.1-flash-image         | image,【职刻】Image | 1 channel  | enabled
 gemini-3.1-flash-image-preview | image,【职刻】Image | 2 channels | enabled
 gpt-image-2                    | image,【职刻】Image | 6 channels | enabled
 gpt-image-2.5-flare            | image,【职刻】Image | 4 channels | enabled
 gpt-image-2.5-sunburst         | image,【职刻】Image | 4 channels | enabled
```

**结论**：用户说的「5 个模型」精确对应
1. `gpt-image-2`
2. `gpt-image-2.5-flare`
3. `gpt-image-2.5-sunburst`
4. `gemini-3.1-flash-image-preview`（小香蕉）
5. `gemini-3-pro-image-preview`（大香蕉）

**flare / sunburst 确实是两个独立的 model name，不是通过参数区分。** 裸名 `gpt-image-2.5` 已不在 `abilities` 中（只在 2026-09-10 当天有 10 条日志，之后被 flare/sunburst 取代）。

**额外发现（不在用户列表里）**：`gemini-3.1-flash-image`（无 `-preview` 后缀）也处于 enabled 状态，挂 1 个渠道，最近 30 天只有 2 条错误日志、0 条成功消费。改价时需要决定是否一并处理，否则它会留在旧价上。

### 承载渠道

| 渠道 ID | 名称 | type | 承载模型 |
|---|---|---:|---|
| 8 | DC GPTimage | 1 (OpenAI) | gpt-image-2 |
| 30 | ZIVV image | 1 | gpt-image-2 |
| 40 | （未在本次查询返回，日志中出现） | — | gpt-image-2, gpt-image-2.5 |
| 41 | SC Adobe | 1 | gpt-image-2 |
| 42 | SC Adobe 备用 | 1 | gpt-image-2 |
| 46 | ZIVV nanobanana | 24 (Gemini) | gemini-3-pro / 3.1-flash |
| 54 | Packy image | 1 | gpt-image-2, flare, sunburst |
| 55 | tuzi image | 60 | 全部 5 个模型 |

> 渠道 40 在 `channels` 表查询中未返回行（可能已删除），但日志里有 10 条 `gpt-image-2.5` 记录指向它。

---

## 问题 2：分辨率信息在日志里可见吗

### 2a. `other` 字段：**没有** size / resolution / quality 键

最近 30 天 564 条图像消费日志里，`other` JSON 的完整键集合（出现次数）：

```
cache_ratio         564    admin_info           564    request_path        564
model_price         564    model_ratio          564    group_ratio         564
completion_ratio    564    request_conversion   564    billing_source      564
cache_tokens        564    frt                  564    route_history       186
image_ratio         163    image_output         163    image               163
upstream_model_name   5    is_model_mapped        5
```

**没有 `size`、`resolution`、`quality`、`width`、`height` 中的任何一个。**

真实样本（已脱敏，无 token/key/用户信息）：

```json
{
    "frt": -1000,
    "admin_info": {
        "route_history": [
            {"attempt": 1, "outcome": "succeeded", "priority": 100,
             "channel_id": 55, "status_code": 200, "channel_name": "tuzi image"}
        ],
        "billing_ratios": {"channel_ratio": 1, "effective_ratio": 1,
            "legacy_override": false, "base_group_ratio": 1,
            "user_group_ratio": 1, "include_channel_ratio": false},
        "usage_billing_path": "upstream"
    },
    "cache_ratio": 0,
    "group_ratio": 1,
    "model_price": 0.15,
    "model_ratio": 0,
    "cache_tokens": 0,
    "request_path": "/v1/images/generations",
    "billing_source": "unlimited",
    "completion_ratio": 0,
    "request_conversion": ["openai_image"]
}
```

`image_output` 这个键**不能**当分辨率代理用。实测 `gpt-image-2` 的 `image_output` 取值从 194 到 14859，长尾散布、不按分辨率量化（例如 2391/2399/2423/2424/2443/2481… 各 1 次）。它来自 `service/text_quota.go:523` 的 `summary.ImageTokens`，而 `service/text_quota.go:301` 把该字段赋为 `usage.PromptTokensDetails.ImageTokens` —— 即**输入侧**图像 token 数（图生图时上传图片的 token），**不是输出图片的 token 数**。与请求 size 无任何映射关系。输出侧图像 token 对应的是表达式变量 `img_o`（`service/tiered_settle.go:41`，取 `usage.CompletionTokenDetails.ImageTokens`），该值未写入 `logs.other`。详见 `live-probe-results.md`。

同名但不同来源的坑：日志里的 `image_ratio` 来自 `service/text_quota.go:522` 的 `summary.ImageRatio`（即 `ratio_setting.GetImageRatio(model)`，按模型名配的静态倍率），**不是** `relaykit/types/request_meta.go:29` 里那个按 size 算的 `ImagePriceRatio`。两者 JSON tag 都叫 `image_ratio`，排查时容易混。

### 2b. `logs.content` 字段：**有完整分辨率**

分辨率写在 `logs.content`，格式是中文可读串：

```
大小 2048x1152, 品质 high, 生成数量 1
大小 1024x1024, 品质 medium, 生成数量 1
大小 4096x4096, 品质 medium, 生成数量 1
```

写入点 `relay/image_handler.go:137-147`：

```go
var logContent []string
if len(request.Size) > 0 {
    logContent = append(logContent, fmt.Sprintf("大小 %s", request.Size))
}
if len(quality) > 0 {
    logContent = append(logContent, fmt.Sprintf("品质 %s", quality))
}
if imageN > 0 {
    logContent = append(logContent, fmt.Sprintf("生成数量 %d", imageN))
}
service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), logContent)
```

**覆盖率（按模型 × 请求路径）**：

| model_name | request_path | content 为空 | 含「大小」 | 请求数 |
|---|---|:-:|:-:|---:|
| gpt-image-2 | `/v1/images/edits` | 否 | **是** | 305 |
| gpt-image-2 | `/v1/images/generations` | 否 | **是** | 78 |
| gpt-image-2 | `/v1/images/edits` | 否 | 否 | 22 |
| gpt-image-2.5-flare | `/v1/images/generations` | 否 | **是** | 40 |
| gpt-image-2.5-flare | `/v1/images/edits` | 否 | **是** | 33 |
| gpt-image-2.5-sunburst | `/v1/images/generations` | 否 | **是** | 29 |
| gpt-image-2.5-sunburst | `/v1/images/edits` | 否 | **是** | 1 |
| gemini-3.1-flash-image-preview | `/v1beta/...:generateContent` | **是** | 否 | 19 |
| gemini-3.1-flash-image-preview | `/v1/images/generations` | 否 | 否 | 9 |
| gemini-3.1-flash-image-preview | `/v1/images/generations` | 否 | **是** | 3 |
| gemini-3-pro-image-preview | `/v1beta/...:generateContent` | **是** | 否 | 5 |
| gemini-3-pro-image-preview | `/v1/images/generations` | 否 | 否 | 5 |
| gemini-3-pro-image-preview | `/v1/images/generations` | 否 | **是** | 3 |
| gemini-3-pro-image-preview | `/v1/chat/completions` | **是** | 否 | 2 |

**分辨率可见性汇总**：

| 模型 | 有 size 的请求 | 占比 |
|---|---:|---:|
| `gpt-image-2` | 383 / 405 | 94.6% |
| `gpt-image-2.5-flare` | 73 / 73 | 100% |
| `gpt-image-2.5-sunburst` | 30 / 30 | 100% |
| `gemini-3.1-flash-image-preview` | 3 / 31 | **9.7%** |
| `gemini-3-pro-image-preview` | 3 / 15 | **20.0%** |

### 2c. 计费时到底知不知道分辨率

**gpt-image 系列：知道。** 走 `/v1/images/{generations,edits}` → `relaykit/dto/openai_image.go` 的 `ImageRequest.Size` 字段已解析，`GetTokenCountMeta()` 在计费前被调用，`ImagePriceRatio` 已经流到 `PriceData`，`relay/helper/price.go:147-151`：

```go
} else {
    if meta.ImagePriceRatio != 0 {
        modelPrice = modelPrice * meta.ImagePriceRatio
    }
}
```

挡住它的是 `relaykit/dto/openai_image.go:133-155` 的模型前缀判断：

```go
func (i *ImageRequest) GetTokenCountMeta() *types.TokenCountMeta {
	var sizeRatio = 1.0
	var qualityRatio = 1.0

	if strings.HasPrefix(i.Model, "dall-e") {
		// ... 只有 dall-e 才根据 Size / Quality 调整 sizeRatio / qualityRatio
	}
	...
	return &types.TokenCountMeta{
		CombineText:     i.Prompt,
		MaxTokens:       1584,
		ImagePriceRatio: sizeRatio * qualityRatio,
		BillingRatios:   map[string]float64{"n": float64(imageN)},
	}
}
```

对 `gpt-image-*` / `gemini-*`，`sizeRatio = qualityRatio = 1.0`，所以 `ImagePriceRatio = 1.0`，固定价原样生效 —— **一刀切的根因就在这一行前缀判断**。

**gemini 原生路径：当前不知道。** `/v1beta/models/...:generateContent` 的 `generationConfig.imageConfig` 在 `relaykit/dto/gemini.go:350` 被声明为 `json.RawMessage` 透传，未解析出 `imageSize`。`/v1/chat/completions` 路径同理没有 size 概念。这 24 条请求（占两个 gemini 模型 46 条中的 52%）在现有代码下拿不到分辨率。

> 另注：`relay/channel/gemini/adaptor.go:63-66` 的 `ConvertImageRequest` 只接受 `imagen` 前缀模型，所以 gemini 图像模型走 `/v1/images/generations` 时并不走这个 adaptor 的 size→aspectRatio 转换，而是被当作 OpenAI 兼容请求转发（日志 `request_conversion: ["openai_image"]` 佐证）。

---

## 问题 3：当前一刀切单价

### 站点换算基准

`options` 表中**没有** `QuotaPerUnit` 行，即使用代码默认值 **500,000 quota = $1**。实测反推全部吻合：
- `model_price` 0.15 → quota 75,000 ✓
- `model_price` 0.10 → quota 50,000 ✓
- `model_price` 0.20 → quota 100,000 ✓

站点 ¥:$ = 1:1，所以 `¥金额 = quota / 500000`。

### 当前生效配置（`options` 表实测）

```
ModelPrice | gpt-image-2                    | 0.15
ModelPrice | gpt-image-2.5                  | 0.15    ← 模型已下线，配置残留
ModelPrice | gpt-image-2.5-flare            | 0.1
ModelPrice | gemini-3-pro-image-preview     | 0.2
ModelPrice | gemini-3-pro-image             | 0.2
ModelPrice | gemini-3.1-flash-image-preview | 0.2
ModelPrice | gemini-3.1-flash-image         | 0.2
ModelPrice | gemini-3.1-flash-lite-image    | 0.2
ModelRatio | gpt-image-2.5-sunburst         | 0.06    ← 只有 ratio，没有 price
```

**`gpt-image-2.5-sunburst` 在 `ModelPrice` 里不存在**，所以 `usePrice = false`，走按 token 计费：

```
quota = (prompt_tokens + completion_tokens × completion_ratio) × model_ratio × group_ratio
      = (50 + 2018 × 2) × 0.06 × 1
      = 4086 × 0.06 = 245.16 → 245
```

实测样本完全吻合（`model_price: -1`, `model_ratio: 0.06`, `completion_ratio: 2`, quota 245）。

### 实际每次请求扣了多少（最近 30 天实测）

| model_name | model_price | model_ratio | 请求数 | quota/次 | ¥/次 | 总 quota | 总 ¥ |
|---|---:|---:|---:|---:|---:|---:|---:|
| `gpt-image-2` | 0.15 | — | 189 | 75,000 | 0.15 | 14,175,000 | 28.35 |
| `gpt-image-2` | 0.07 | — | 203 | 35,000 | 0.07 | 7,105,000 | 14.21 |
| `gpt-image-2` | -1（旧 token 计费） | 2.5 | 13 | 18,499~145,041 | 0.04~0.29 | 634,501 | 1.27 |
| `gpt-image-2.5-flare` | 0.1 | — | 73 | 50,000 | 0.10 | 3,650,000 | 7.30 |
| `gpt-image-2.5-sunburst` | **-1** | **0.06** | 30 | **1~245** | **~0.00007** | **1,020** | **0.002** |
| `gemini-3.1-flash-image-preview` | 0.2 | — | 28 | 100,000 | 0.20 | 2,800,000 | 5.60 |
| `gemini-3.1-flash-image-preview` | 0.04 | — | 3 | 20,000 | 0.04 | 60,000 | 0.12 |
| `gemini-3-pro-image-preview` | 0.2 | — | 14 | 100,000 | 0.20 | 1,400,000 | 2.80 |
| `gemini-3-pro-image-preview` | 0.06 | — | 1 | 30,000 | 0.06 | 30,000 | 0.06 |
| `gpt-image-2.5`（已下线） | 0.15 | — | 10 | 75,000 | 0.15 | 750,000 | 1.50 |

**30 天目标 5 模型名义计费合计：¥59.71**（含已下线 `gpt-image-2.5` 则 ¥61.21）。

> `gpt-image-2` 窗口内价格变过（0.07 → 0.15），`gemini-*` 也出现过 0.04 / 0.06 的历史值。按**当前生效价**重算同一批 554 条请求，名义金额应为 **¥77.25**（sunburst 按其当前 token 计费≈0 计入）。

### 关键：这些都不是真实收入

全部 554 条请求的 `billing_source` 都是 `unlimited`：

| model_name | billing_source | 分组 | 独立用户 | 独立 token | 请求数 |
|---|---|---|---:|---:|---:|
| gpt-image-2 | unlimited | Image | 4 | 5 | 205 |
| gpt-image-2 | unlimited | 【职刻】Image | 2 | 2 | 179 |
| gpt-image-2 | unlimited | image | 1 | 1 | 21 |
| gpt-image-2.5-flare | unlimited | 【职刻】Image | 1 | 1 | 70 |
| gpt-image-2.5-flare | unlimited | image | 1 | 1 | 3 |
| gpt-image-2.5-sunburst | unlimited | 【职刻】Image | 1 | 1 | 27 |
| gpt-image-2.5-sunburst | unlimited | image | 1 | 1 | 3 |
| gemini-3.1-flash-image-preview | unlimited | 【职刻】Image | 1 | 1 | 20 |
| gemini-3.1-flash-image-preview | unlimited | image | 1 | 1 | 8 |
| gemini-3.1-flash-image-preview | unlimited | Image | 1 | 1 | 3 |
| gemini-3-pro-image-preview | unlimited | image | 1 | 1 | 10 |
| gemini-3-pro-image-preview | unlimited | 【职刻】Image | 1 | 1 | 4 |
| gemini-3-pro-image-preview | unlimited | Image | 1 | 1 | 1 |

`service/funding_source.go:26-33`：

```go
// UnlimitedFunding records usage without touching the user's wallet balance.
// Token-level quota is still reserved and settled by BillingSession.
type UnlimitedFunding struct{}

func (u *UnlimitedFunding) Source() string       { return BillingSourceUnlimited }
func (u *UnlimitedFunding) PreConsume(int) error { return nil }
func (u *UnlimitedFunding) Settle(int) error     { return nil }
func (u *UnlimitedFunding) Refund() error        { return nil }
```

即 `quota` 列只是**记账数字**，没有任何钱包余额被扣。最近 30 天参与的独立用户最多 4 个，集中在 `Image` / `【职刻】Image` / `image` 三个分组。

---

## 问题 4：真实分辨率分布

### 4a. 逐模型精确 size 值（最近 30 天，type=2）

**`gpt-image-2`**（405 条）

| size | quality | 请求数 |
|---|---|---:|
| 2048x1152 | high | 168 |
| 2048x896 | high | 115 |
| （无 size） | standard | 22 |
| auto | medium | 19 |
| 1024x1024 | medium | 16 |
| 2560x1440 | medium | 15 |
| auto | auto | 11 |
| 1024x1024 | standard | 4 |
| 1024x1024 | high | 4 |
| auto | auto (n=4) | 4 |
| 4096x4096 | medium | 3 |
| 1536x1024 | high | 3 |
| 2048x2048 | medium | 3 |
| 2048x2048 | standard | 2 |
| 2160x3840 | medium | 2 |
| 3840x2160 | standard | 2 |
| 其余 11 种各 1 条 | — | 11 |

其余单条尺寸：`1024x1536`、`1672x941`、`1254x1254`、`2288x1824`、`1664x944`(×2, 不同 quality)、`3001x3001`、`3344x1882`、`1248x1248`、`1573x1573`、`2001x2001`、`2000x2000`

**`gpt-image-2.5-flare`**（73 条）

| size | quality | 请求数 |
|---|---|---:|
| 2048x1152 | high | 33 |
| 1024x1024 | medium | 12 |
| 2048x2048 | medium | 7 |
| 2560x1440 | medium | 5 |
| 1672x941 | medium | 2 |
| 1024x1536 | medium | 2 |
| 1200x1200 | medium | 2 |
| 1254x1254 | medium | 2 |
| 4096x4096 | medium | 2 |
| 1152x2048 / 2048x2048(std) / 1100x1400 / auto(high) / auto(low) / 1536x1024 | — | 各 1 |

**`gpt-image-2.5-sunburst`**（30 条）

| size | quality | 请求数 |
|---|---|---:|
| 1024x1024 | medium | 10 |
| 2048x2048 | medium | 7 |
| 2560x1440 | medium | 3 |
| 2880x2880 | high | 3 |
| 3456x1728 | high | 3 |
| 4096x4096 | medium | 2 |
| 2208x3360 | high | 1 |
| 1024x1024 | standard | 1 |

**`gemini-3.1-flash-image-preview`**（31 条）

| size | quality | 请求数 |
|---|---|---:|
| （无） | （无） | 19 |
| （无） | standard | 9 |
| 1024x1024 | standard | 2 |
| 2048x2048 | standard | 1 |

**`gemini-3-pro-image-preview`**（15 条）

| size | quality | 请求数 |
|---|---|---:|
| （无） | （无） | 7 |
| （无） | standard | 5 |
| 1024x1024 | standard | 1 |
| 4096x4096 | standard | 1 |
| **4K**（字面量，非 WxH） | standard | 1 |

> 注意 `大小 4K` 这条：客户端直接传了 Gemini 风格的 `"4K"` 字符串而不是 `WxH`。分档规则必须同时认 `1K`/`2K`/`4K` 字面量和 `WxH` 数值，否则会误落到 1K 档。

### 4b. 分档占比（两种分档规则，结果差异显著）

`auto` 和「无 size」在下表中记作 `1K*`（按默认档兜底，属**假设**，非实测）。

**规则 A — 按最长边**：`≤1024 → 1K`，`≤2048 → 2K`，`>2048 → 4K`

| model_name | 1K | 1K*(auto/无) | 2K | 4K | 合计 |
|---|---:|---:|---:|---:|---:|
| gpt-image-2 | 24 (5.9%) | 56 (13.8%) | 300 (74.1%) | 25 (6.2%) | 405 |
| gpt-image-2.5-flare | 12 (16.4%) | 2 (2.7%) | 52 (71.2%) | 7 (9.6%) | 73 |
| gpt-image-2.5-sunburst | 11 (36.7%) | 0 | 7 (23.3%) | 12 (40.0%) | 30 |
| gemini-3.1-flash-image-preview | 2 (6.5%) | 28 (90.3%) | 1 (3.2%) | 0 | 31 |
| gemini-3-pro-image-preview | 1 (6.7%) | 12 (80.0%) | 0 | 2 (13.3%) | 15 |

**规则 B — 按像素面积**：`≤1.5M px → 1K`，`≤6M px → 2K`，`>6M px → 4K`

| model_name | 1K | 1K*(auto/无) | 2K | 4K | 合计 |
|---|---:|---:|---:|---:|---:|
| gpt-image-2 | 24 | 56 | 316 (78.0%) | 9 (2.2%) | 405 |
| gpt-image-2.5-flare | 14 | 2 | 55 (75.3%) | 2 (2.7%) | 73 |
| gpt-image-2.5-sunburst | 11 | 0 | 13 (43.3%) | 6 (20.0%) | 30 |
| gemini-3.1-flash-image-preview | 2 | 28 | 1 | 0 | 31 |
| gemini-3-pro-image-preview | 1 | 12 | 0 | 2 | 15 |

**两种规则的分歧点**：`2560x1440`（3.69M px）、`3456x1728`（5.97M px）、`2288x1824`（4.17M px）在规则 A 下算 4K、在规则 B 下算 2K。合计 4K 档请求数从 46（规则 A）降到 19（规则 B）。**分档规则的选择比价格表本身更影响结果**，需要先定下来。

### 4c. 流量时间分布（关键：窗口极度不均衡）

| 日期 | gpt-image-2 | flare | sunburst | 3.1-flash | 3-pro |
|---|---:|---:|---:|---:|---:|
| 2026-09-13 | 26 | 43 | 6 | 28 | 14 |
| 2026-09-12 | 43 | 30 | 24 | — | — |
| 2026-09-11 | 131 | — | — | — | — |
| 2026-09-10 | 168 | — | — | 3 | 1 |
| 2026-09-09 | 2 | — | — | — | — |
| 2026-09-02 | 2 | — | — | — | — |
| 2026-09-01 | 13 | — | — | — | — |
| 2026-08-31 | 9 | — | — | — | — |
| 2026-08-29 | 10 | — | — | — | — |
| 2026-08-28 | 1 | — | — | — | — |

- `flare` / `sunburst` 只有 **2 天**数据（09-12 起）
- `gemini-3.1-flash` / `3-pro` 实质只有 09-13 一天有量
- `gpt-image-2` 的 405 条里 342 条（84%）集中在 09-10 ~ 09-12 三天

**这意味着「30 天请求量」不是稳态基线。** 用它外推月度收入不可靠 —— 下面的估算只是「把这 554 条按新价重算」，不是月度预测。

---

## 问题 5：改价影响面估算

目标价（¥/张，站点 ¥:$ = 1:1，quota = 价格 × 500000）：

| 模型 | 1K | 2K | 4K |
|---|---:|---:|---:|
| gpt-image-2 | 0.12 | 0.12 | 0.21 |
| gpt-image-2.5-flare | 0.12 | 0.16 | 0.21 |
| gpt-image-2.5-sunburst | 0.12 | 0.16 | 0.21 |
| gemini-3.1-flash-image-preview | 0.30 | 0.30 | 0.35 |
| gemini-3-pro-image-preview | 0.15 | 0.15 | 0.20 |

> 已核对：`flare` 与 `sunburst` 共用同一档价格表，且两者确为独立 model name，可直接分别配置。

### 三个基线对比（同一批 554 条请求）

| 口径 | 名义金额 |
|---|---:|
| **历史实际计费**（含窗口内已变过的旧价、sunburst 的 token 计费） | **¥59.71** |
| **当前生效价一刀切**（gpt-image-2 @0.15、flare @0.10、gemini @0.20、sunburst 按 token≈0） | **¥77.25** |
| **目标阶梯价 · 规则 A（最长边）** | **¥78.93** |
| **目标阶梯价 · 规则 B（像素面积）** | **¥76.86** |

### 逐模型明细（规则 A vs 当前生效价一刀切）

| model_name | 请求数 | 一刀切 ¥ | 阶梯 ¥ | 差额 | 变化 |
|---|---:|---:|---:|---:|---:|
| gpt-image-2 | 405 | 60.75 | 50.85 | **-9.90** | -16.3% |
| gpt-image-2.5-flare | 73 | 7.30 | 11.47 | **+4.17** | +57.1% |
| gpt-image-2.5-sunburst | 30 | ~0.00 | 4.96 | **+4.96** | — |
| gemini-3.1-flash-image-preview | 31 | 6.20 | 9.30 | **+3.10** | +50.0% |
| gemini-3-pro-image-preview | 15 | 3.00 | 2.35 | **-0.65** | -21.7% |
| **合计** | **554** | **77.25** | **78.93** | **+1.68** | **+2.2%** |

### 逐模型 × 分档明细（规则 A，对比历史实际计费）

| model_name | 档位 | 请求数 | 现计 ¥ | 新计 ¥ | 差额 ¥ |
|---|---|---:|---:|---:|---:|
| gemini-3-pro-image-preview | 1K | 1 | 0.20 | 0.15 | -0.05 |
| gemini-3-pro-image-preview | 1K*（无 size） | 12 | 2.26 | 1.80 | -0.46 |
| gemini-3-pro-image-preview | 4K | 2 | 0.40 | 0.40 | 0.00 |
| **gemini-3-pro-image-preview 小计** | | **15** | **2.86** | **2.35** | **-0.51** |
| gemini-3.1-flash-image-preview | 1K | 2 | 0.40 | 0.60 | +0.20 |
| gemini-3.1-flash-image-preview | 1K*（无 size） | 28 | 5.12 | 8.40 | +3.28 |
| gemini-3.1-flash-image-preview | 2K | 1 | 0.20 | 0.30 | +0.10 |
| **gemini-3.1-flash-image-preview 小计** | | **31** | **5.72** | **9.30** | **+3.58** |
| gpt-image-2 | 1K | 24 | 3.44 | 2.88 | -0.56 |
| gpt-image-2 | 1K*（auto/无） | 56 | 5.08 | 6.72 | +1.64 |
| gpt-image-2 | 2K | 300 | 31.56 | 36.00 | +4.44 |
| gpt-image-2 | 4K | 25 | 3.75 | 5.25 | +1.50 |
| **gpt-image-2 小计** | | **405** | **43.83** | **50.85** | **+7.02** |
| gpt-image-2.5-flare | 1K | 12 | 1.20 | 1.44 | +0.24 |
| gpt-image-2.5-flare | 1K*（auto） | 2 | 0.20 | 0.24 | +0.04 |
| gpt-image-2.5-flare | 2K | 52 | 5.20 | 8.32 | +3.12 |
| gpt-image-2.5-flare | 4K | 7 | 0.70 | 1.47 | +0.77 |
| **gpt-image-2.5-flare 小计** | | **73** | **7.30** | **11.47** | **+4.17** |
| gpt-image-2.5-sunburst | 1K | 11 | 0.00 | 1.32 | +1.32 |
| gpt-image-2.5-sunburst | 2K | 7 | 0.00 | 1.12 | +1.12 |
| gpt-image-2.5-sunburst | 4K | 12 | 0.00 | 2.52 | +2.52 |
| **gpt-image-2.5-sunburst 小计** | | **30** | **0.00** | **4.96** | **+4.96** |
| **总计** | | **554** | **59.71** | **78.93** | **+19.22** |

### 对估算结果影响最大的三个因素

1. **分档规则**（规则 A vs B）：总额 ¥78.93 vs ¥76.86，但 `gpt-image-2` 的 4K 请求数从 25 变成 9，`sunburst` 从 12 变成 6。
2. **`auto` / 无 size 的兜底档**：98 条请求（占 17.7%）落在这里，全部按 1K 兜底。若改按 2K 兜底，总额会显著上升 —— 其中 28 条 `gemini-3.1-flash`（¥0.30→¥0.30 无差别）、56 条 `gpt-image-2`（¥0.12→¥0.12 无差别）、12 条 `gemini-3-pro`（¥0.15→¥0.15 无差别）。**按用户给的价格表，1K 和 2K 同价的模型占了兜底请求的绝大多数，所以兜底档选择对总额影响其实很小**；只有 flare/sunburst 的 2 条 auto 会受影响（¥0.12 vs ¥0.16）。
3. **窗口不代表稳态**：flare/sunburst/gemini 只有 1~2 天数据（见 4c）。

---

## Caveats / 查不到的部分

### 已明确查到的边界

- **`other` 字段里没有任何分辨率键** —— 已用 `jsonb_object_keys` 全量枚举 564 条日志确认，不是抽样结论。
- **gemini 原生 `generateContent` 路径的分辨率查不到** —— 已确认 `logs.content` 为空、`other` 无 size 键、`relaykit/dto/gemini.go:350` 的 `ImageConfig` 是 `json.RawMessage` 透传未解析。这 24 条请求（两个 gemini 模型 46 条里的 52%）在现有代码和现有数据下都无法还原分辨率。
- **渠道 40 的信息查不到** —— `channels` 表中无 id=40 的行（可能已删除），但日志里有 10 条 `gpt-image-2.5` 指向它。未进一步追查。

### 尝试过但没有结果的路径

- **用 `image_output` 反推分辨率：失败。** 取值长尾散布（194~14859），不按分辨率量化，与 `logs.content` 里的 size 无稳定对应。后经实测确认该字段是**输入侧**图像 token，本来就与输出分辨率无关。已放弃作为代理指标。
- **用 `prompt_tokens` / `completion_tokens` / `use_time` 反推：未做。** 观察到 `gpt-image-2.5-flare` 有 `prompt_tokens=1, completion_tokens=0` 的记录（固定价路径下 token 被置为占位值，见 `relay/image_handler.go:125-130`），token 字段不可靠，不具备反推价值。

### 明确标注为推测的部分

- 把 `auto` 和「无 size」归入 1K 档，是**为了能出数而做的假设**，不是实测。实际这 98 条请求的真实出图分辨率由上游决定，日志里无记录。
- 规则 A / 规则 B 两套分档阈值都是我为了给出区间而自定的，**项目里目前不存在任何既定的 1K/2K/4K 分档定义**。真正的阈值需要产品侧拍板。
- 「上游成本」无法估算：所有日志的 `billing_ratios.include_channel_ratio` 都是 `false`、`channel_ratio` 都是 1，渠道侧没有配成本倍率，库里没有上游实际花费数据。用户问的「收入/成本大概怎么变」只能回答收入侧（且是名义收入）。

### 两个需要决策的既有问题（只陈述事实，不做建议）

1. `gpt-image-2.5-sunburst` 缺 `ModelPrice` 配置，当前按 `ModelRatio` 0.06 的 token 计费，30 次请求共计 1020 quota（¥0.002）。
2. `options.ModelPrice` 里仍残留已下线模型 `gpt-image-2.5` 的 0.15 配置；`gemini-3.1-flash-image`（无 `-preview`）在 `abilities` 中 enabled 且配了 0.2，但不在用户列出的 5 个模型内。

---

## 附：本次用到的代码位置

| 文件:行 | 作用 |
|---|---|
| `relay/image_handler.go:137-149` | 把 `大小/品质/生成数量` 写进 `logs.content` |
| `relay/image_handler.go:125-130` | 固定价路径下把 token 置为占位值 1 |
| `relaykit/dto/openai_image.go:133-171` | `GetTokenCountMeta()`，`ImagePriceRatio` 的唯一产出点，`dall-e` 前缀判断在 137 行 |
| `relay/helper/price.go:147-151` | `modelPrice *= meta.ImagePriceRatio`，固定价模式下的分辨率倍率挂载点 |
| `relay/helper/price.go:138` | `imageRatio, _ = ratio_setting.GetImageRatio(...)`，按模型名的静态倍率（与上者不同） |
| `relaykit/types/request_meta.go:29` | `ImagePriceRatio float64 \`json:"image_ratio,omitempty"\`` |
| `setting/ratio_setting/model_ratio.go:638-659` | `defaultImageRatio` / `GetImageRatio`，默认只有 `gpt-image-1: 2` |
| `service/text_quota.go:301,522-523` | 把 `image_ratio` / `image_output` 写进 `logs.other`；`image_output` 实为**输入侧**图像 token |
| `service/funding_source.go:26-33` | `UnlimitedFunding`，不动钱包余额 |
| `service/billing.go:13-17` | `BillingSourceWallet/Subscription/Unlimited` 常量 |
| `relaykit/dto/gemini.go:350` | `ImageConfig json.RawMessage` 透传，未解析 `imageSize` |
| `relay/channel/gemini/adaptor.go:63-127` | `ConvertImageRequest`，只接受 `imagen` 前缀 |
