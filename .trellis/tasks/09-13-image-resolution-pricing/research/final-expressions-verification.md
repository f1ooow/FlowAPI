# Research: 6 条定稿表达式的实跑验证（第 6 版·终版：阈值按上游账单修正）

- **Query**: 在第 5 版（gpt 三行版 + gemini 两路版，`let` 多行）基础上修正面积阈值：4K 边界统一改为 `px > 3686400`；flare/sunburst 的 2K 边界改为 `px > 1048576`（上游 flare 表达式实证），gpt-image-2 / gemini 的 2K 边界保持 `px > 1572864`（gpt-image-2 账单实证 1536x1024 为 1K；1K/2K 同价，只影响档位名）
- **Scope**: internal（对真实 `pkg/billingexpr` 实跑；用 bun 实跑前端保存链路；**未修改仓库任何代码**，临时模块已清理）
- **Date**: 2026-09-13
- **基线**: FlowAPI `main@9794c7363`；`QuotaPerUnit=500000`；`GroupRatio=1`；站点 ¥:$=1:1
- **换算口径**: 每个用例同时跑结算函数 `billingexpr.ComputeTieredQuotaWithRequest`（`pkg/billingexpr/settle.go:21-39`）和预扣公式 `RunExprWithRequest → rawCost/1e6×QuotaPerUnit → QuotaRoundStrict`（`relay/helper/price.go:312-323`），逐用例相等（不等会显示 `MISMATCH`，实际零出现）
- **multipart 编辑请求**：当前基线下 `param()` 读不到表单字段，一律落 1K；根治方案见 `multipart-billing-fix-design.md`（改 `relay/helper/billing_expr_request.go` 约 12 行），修复后本文表达式不需要任何改动即可按 `size`/`n` 判档（第 3 节最后一行已用修复后的投影 body 验证）

---

## 结论

**下面 6 条可以直接粘进生产。** 与第 5 版相比只改了阈值数字（`4194304 → 3686400`；flare/sunburst 的 `1572864 → 1048576`），其余逐字不变。后端编译通过、全矩阵实跑零错误、预扣=结算；多行文本经 bun 实跑前端保存链路后与原文逐字相等，定价页识别出 `4k/2k/1k`。

**前端「Token 估算器」对这 6 条显示 `Unexpected keyword 'let'`（浏览器：`Unexpected strict mode reserved word`）—— 不阻断保存，粘进去直接点保存。** 任何含 `param()` 的表达式在估算器里都必然报错（证据见 `no-let-expressions.md`）。

### gpt 三条（判档只看 `size` 的 `WxH` 像素面积；`auto`/缺失/其它一律 1K；乘张数）

**A. gpt-image-2**（1K ¥0.12 / 2K ¥0.12 / 4K ¥0.21；`px > 3686400` → 4K，`px > 1572864` → 2K）

```
let s = string(param("size") ?? "");
let px = s matches "^[0-9]+x[0-9]+$" ? float(int(split(s, "x")[0])) * float(int(split(s, "x")[1])) : 0.0;
(px > 3686400 ? tier("4k", 210000) : (px > 1572864 ? tier("2k", 120000) : tier("1k", 120000))) * max(float(param("n") ?? 1), 1.0)
```

**B. gpt-image-2.5-flare**（1K ¥0.12 / 2K ¥0.16 / 4K ¥0.21；`px > 3686400` → 4K，`px > 1048576` → 2K）

```
let s = string(param("size") ?? "");
let px = s matches "^[0-9]+x[0-9]+$" ? float(int(split(s, "x")[0])) * float(int(split(s, "x")[1])) : 0.0;
(px > 3686400 ? tier("4k", 210000) : (px > 1048576 ? tier("2k", 160000) : tier("1k", 120000))) * max(float(param("n") ?? 1), 1.0)
```

**C. gpt-image-2.5-sunburst**（1K ¥0.12 / 2K ¥0.16 / 4K ¥0.21）—— 与 B 完全相同

```
let s = string(param("size") ?? "");
let px = s matches "^[0-9]+x[0-9]+$" ? float(int(split(s, "x")[0])) * float(int(split(s, "x")[1])) : 0.0;
(px > 3686400 ? tier("4k", 210000) : (px > 1048576 ? tier("2k", 160000) : tier("1k", 120000))) * max(float(param("n") ?? 1), 1.0)
```

### gemini 三条（先 `imageSize` 五段探测链，再 `size` 字面量，再 `size` 面积，兜底 1K；乘张数）

**D. gemini-3-pro-image-preview**（1K ¥0.15 / 2K ¥0.15 / 4K ¥0.20）

```
let zNativeSnakeSnake = upper(trim(string(param("generationConfig.image_config.image_size") ?? "")));
let zNativeSnakeCamel = upper(trim(string(param("generationConfig.image_config.imageSize") ?? "")));
let zNativeCamelCamel = upper(trim(string(param("generationConfig.imageConfig.imageSize") ?? "")));
let zNativeCamelSnake = upper(trim(string(param("generationConfig.imageConfig.image_size") ?? "")));
let zExtraBody = upper(trim(string(param("extra_body.google.image_config.image_size") ?? "")));
let zImageSize = zNativeSnakeSnake != "" ? zNativeSnakeSnake : (zNativeSnakeCamel != "" ? zNativeSnakeCamel : (zNativeCamelCamel != "" ? zNativeCamelCamel : (zNativeCamelSnake != "" ? zNativeCamelSnake : zExtraBody)));
let s = string(param("size") ?? "");
let zSizeLiteral = upper(trim(s));
let px = s matches "^[0-9]+x[0-9]+$" ? float(int(split(s, "x")[0])) * float(int(split(s, "x")[1])) : 0.0;
let t = zImageSize in ["4K", "2K", "1K"] ? zImageSize : (zSizeLiteral in ["4K", "2K", "1K"] ? zSizeLiteral : (px > 3686400 ? "4K" : (px > 1572864 ? "2K" : "1K")));
(t == "4K" ? tier("4k", 200000) : (t == "2K" ? tier("2k", 150000) : tier("1k", 150000))) * max(float(param("n") ?? 1), 1.0)
```

**E. gemini-3.1-flash-image-preview**（1K ¥0.30 / 2K ¥0.30 / 4K ¥0.35）

```
let zNativeSnakeSnake = upper(trim(string(param("generationConfig.image_config.image_size") ?? "")));
let zNativeSnakeCamel = upper(trim(string(param("generationConfig.image_config.imageSize") ?? "")));
let zNativeCamelCamel = upper(trim(string(param("generationConfig.imageConfig.imageSize") ?? "")));
let zNativeCamelSnake = upper(trim(string(param("generationConfig.imageConfig.image_size") ?? "")));
let zExtraBody = upper(trim(string(param("extra_body.google.image_config.image_size") ?? "")));
let zImageSize = zNativeSnakeSnake != "" ? zNativeSnakeSnake : (zNativeSnakeCamel != "" ? zNativeSnakeCamel : (zNativeCamelCamel != "" ? zNativeCamelCamel : (zNativeCamelSnake != "" ? zNativeCamelSnake : zExtraBody)));
let s = string(param("size") ?? "");
let zSizeLiteral = upper(trim(s));
let px = s matches "^[0-9]+x[0-9]+$" ? float(int(split(s, "x")[0])) * float(int(split(s, "x")[1])) : 0.0;
let t = zImageSize in ["4K", "2K", "1K"] ? zImageSize : (zSizeLiteral in ["4K", "2K", "1K"] ? zSizeLiteral : (px > 3686400 ? "4K" : (px > 1572864 ? "2K" : "1K")));
(t == "4K" ? tier("4k", 350000) : (t == "2K" ? tier("2k", 300000) : tier("1k", 300000))) * max(float(param("n") ?? 1), 1.0)
```

**F. gemini-3.1-flash-image**（1K ¥0.30 / 2K ¥0.30 / 4K ¥0.35）—— 与 E 完全相同

```
let zNativeSnakeSnake = upper(trim(string(param("generationConfig.image_config.image_size") ?? "")));
let zNativeSnakeCamel = upper(trim(string(param("generationConfig.image_config.imageSize") ?? "")));
let zNativeCamelCamel = upper(trim(string(param("generationConfig.imageConfig.imageSize") ?? "")));
let zNativeCamelSnake = upper(trim(string(param("generationConfig.imageConfig.image_size") ?? "")));
let zExtraBody = upper(trim(string(param("extra_body.google.image_config.image_size") ?? "")));
let zImageSize = zNativeSnakeSnake != "" ? zNativeSnakeSnake : (zNativeSnakeCamel != "" ? zNativeSnakeCamel : (zNativeCamelCamel != "" ? zNativeCamelCamel : (zNativeCamelSnake != "" ? zNativeCamelSnake : zExtraBody)));
let s = string(param("size") ?? "");
let zSizeLiteral = upper(trim(s));
let px = s matches "^[0-9]+x[0-9]+$" ? float(int(split(s, "x")[0])) * float(int(split(s, "x")[1])) : 0.0;
let t = zImageSize in ["4K", "2K", "1K"] ? zImageSize : (zSizeLiteral in ["4K", "2K", "1K"] ? zSizeLiteral : (px > 3686400 ? "4K" : (px > 1572864 ? "2K" : "1K")));
(t == "4K" ? tier("4k", 350000) : (t == "2K" ? tier("2k", 300000) : tier("1k", 300000))) * max(float(param("n") ?? 1), 1.0)
```

长度：A/B/C 272 字符、3 行；D/E/F 1184 字符、11 行。

### 阈值依据（本版修正点）

| 模型 | 2K 边界 | 4K 边界 | 依据 |
|---|---|---|---|
| gpt-image-2.5-flare / -sunburst | `px > 1,048,576`（1024×1024 之上即 2K） | `px > 3,686,400`（2560×1440 之上即 4K） | 上游 flare 表达式实证 `imagePixels(size) > 1048576 → 2K`、`> 3686400 → 4K`。旧阈值 `> 1572864` 会把 `1536x1024`、`1254x1254`、`1024x1536`、`1200x1200` 等判 1K，而上游按 2K 收，每张亏 ¥0.04 |
| gpt-image-2 | `px > 1,572,864`（仅影响档位名，1K/2K 同价） | `px > 3,686,400` | 账单：`1536x1024` 收 $0.04（1K），`2560x1440` 收 $0.1714（2K），`2048x2048` 收 $0.30（4K） |
| gemini 三条 | `px > 1,572,864`（仅影响档位名） | `px > 3,686,400` | 与 gpt-image-2 统一；`imageSize` 字面量优先，`size` 面积只在 OpenAI 兼容路径出现 |

修正后生产尺寸的落档变化（相对第 5 版）：flare/sunburst 的 `1536x1024`、`1024x1536`、`1254x1254`、`1248x1248`、`1100x1400`、`1200x1200`、`1664x944` 从 1k 升为 2k；六条的 `2001x2001`、`2000x2000`、`2288x1824`（4.0M–4.17M）从 2k 升为 4k。

### 表达式怎么读

| 行 | 作用 |
|---|---|
| `let s = string(param("size") ?? "")` | 客户端原始 `size`，缺失/null 归一为空串 |
| `let px = s matches "^[0-9]+x[0-9]+$" ? … : 0.0` | 只有严格 `WxH` 才算面积；`auto`、宽高比、空串、带空格、大写 `X` 都算 0 → 落 1K。`matches` 为假时右侧 `int(split(...))` 不会执行（expr-lang 短路，实跑确认） |
| `zNative*`、`zExtraBody` | Gemini `imageSize` 的五种写法，`?? ""` 把缺失/null 归一为空串再 `upper(trim())`；顺序镜像 DTO（`generationConfig` 下 `image_config` 覆盖 `imageConfig`，`relaykit/dto/gemini.go:432-433`），空串会继续向后取 |
| `let t = zImageSize in [...] ? zImageSize : (zSizeLiteral in [...] ? zSizeLiteral : (面积))` | 优先级：`imageSize` 枚举 → `size` 字面量 `4K/2K/1K` → `size` 面积 → 1K。`imageSize` 为非枚举值（`512px`、数字）时视同未声明，继续看 `size` |
| `tier("4k", …)` / `tier("2k", …)` / `tier("1k", …)` | 档位名小写，原样进用量日志 `matched_tier`（`service/log_info_generate.go:363`），定价页与日志详情按同名匹配（`web/src/features/usage-logs/lib/format.ts:291-303`）。1k 与 2k 同价的模型也分开命名，便于审计真实分布 |
| `* max(float(param("n") ?? 1), 1.0)` | 张数按声明值收；`n=0` 在校验层被归一成 1 正常出图但 `param("n")` 读到的仍是 0，`max(…,1.0)` 防免费；`n` 已被校验层封顶 128 |

### 换算核对

`tier(…, 120000)` → `120000 / 1e6 × 500000 = 60000 quota = ¥0.12`；`160000 → 80000`；`210000 → 105000`；`150000 → 75000`；`200000 → 100000`；`300000 → 150000`；`350000 → 175000`。**写成 `0.12` 会折算为 0 quota、免费放行且不报错。**

---

## 计费口径说明（可独立阅读）

**按客户端请求中声明的分辨率计费，与实际交付尺寸、与路由到哪个渠道都无关。** 判档规则是对用户的产品承诺，必须稳定可解释、不随渠道变；上游成本差异通过定价毛利和换渠道时的成本核对来管理。

### gpt-image 三条

判档只有一条路：`size` 的 `WxH` 像素面积（阈值见上表）。`auto`、不传、宽高比、非法串一律 1K。依据：30 天生产日志（`hk-production-logs.md` §4a，gpt 三个模型 508 条）里 `size` 只有 `WxH`、`auto`、不传三种形态，零字面量、零 `imageSize`，`quality` 全是常规值。`auto` 落 1K 的三方证据：实测 gpt-image-2 不传 `size` 或 `size=auto`（含 prompt 明确要求 4K 的文生图与图生图编辑）交付的都是 1K；上游账单 flare 传 `auto` 收 $0.171428（其 1K 档价）、gpt-image-2 无 `size` 收 $0.04（1K 档）；上游表达式 `imageSizeTier` 对 `auto` 不匹配、落 default。

### gemini 三条

1. `imageSize` 字面量（原生 `generationConfig.imageConfig.imageSize` 四种写法 + chat 的 `extra_body.google.image_config.image_size`）→ `4K` / `2K` / `1K`
2. `size`：先认字面量 `4K` / `2K` / `1K`（30 天日志里 gemini-3-pro 有 1 条 `size=4K`；上游账单对该条收 $0.257 即 4K 档价），再按 `WxH` 面积
3. 兜底 1K：`auto`、宽高比、空、缺失、非法串

大小写与首尾空格不敏感；`imageSize` 与 `size` 同时出现时 `imageSize` 优先（真实流量不存在）。

### 上游成本、我方售价与毛利（¥；成本 = tuzi 账单 $ × 0.7）

| 模型 | 1K | 2K | 4K |
|---|---|---|---|
| gpt-image-2 | 成本 ¥0.028 / 售价 ¥0.12 / 毛利 +¥0.092 | ¥0.120 / ¥0.12 / ±0 | ¥0.210 / ¥0.21 / ±0 |
| gpt-image-2.5-flare / -sunburst | ¥0.120（$0.1714）/ ¥0.12 / ±0 | ¥0.160（$0.2286）/ ¥0.16 / ±0 | ¥0.210 / ¥0.21 / ±0 |
| gemini-3-pro-image-preview | ¥0.120 / ¥0.15 / +¥0.030 | ¥0.150 / ¥0.15 / ±0 | ¥0.180 / ¥0.20 / +¥0.020 |
| gemini-3.1-flash-image(-preview) | ¥0.286 / ¥0.30 / +¥0.014 | ¥0.286 / ¥0.30 / +¥0.014 | ¥0.325 / ¥0.35 / +¥0.025 |

（flare/sunburst 的三档成本按 team-lead 提供的 flare 账单口径 $0.1714 / $0.2286 / $0.30；flash 2K 成本账单未单列，按上游 1K/2K 同价。）

### 入口对照：客户端声明 → 我方判档 → 用户实际拿到的分辨率

| 入口 | 客户端写法 | 我方判档 | 用户实际拿到 | 备注 |
|---|---|---|---|---|
| gpt OpenAI images | `size` = `WxH` | 面积分档 | ≈ 请求尺寸（16 对齐；`4096x4096` 有后端降到 2880²） | 一致 |
| gpt OpenAI images | `size` = `auto` / 缺省 | 1K | 1K（实测 1254×1254） | 一致 |
| gpt `/v1/images/edits` multipart | 任何 `size` | **修复前** 1K（`param()` 读不到表单）；**修复后** 按 `size` 面积 | 上游按表单 `size` 分档收我方 | 修复方案见 `multipart-billing-fix-design.md` |
| Gemini 原生 `generateContent` | `imageConfig.imageSize` = 1K / 2K / 4K（四种写法） | 同值 | 同值（实测 pro 1024² / 2048² / 4096²；flash 1408×768 / 5632×3072） | 一致 |
| Gemini 原生 | 只传 `aspectRatio` / 无 `imageConfig` / `imageSize=512px` | 1K | 1K | 一致 |
| gemini OpenAI images | `size` = `WxH`（如 `4096x4096`、`2048x2048`） | 面积分档 → 4K | pro 交付 1024×1024；flash-image 交付 2048×2048 | **用户拿到的比声明小**，用户已接受 |
| gemini OpenAI images | `size` = `4K` 字面量 | 4K | 实测 pro 交付 1024×1024 | 同上；上游账单对该写法收 4K 档价 |
| gemini OpenAI images | `size` = `auto` / 缺省 / `1:1` / `4:3` | 1K | 1K | 一致 |
| gemini OpenAI chat | `extra_body.google.image_config.image_size` = 4K | 4K | 未实测 | — |
| gemini OpenAI chat | 顶层 `image_config`（不在 `extra_body`） | 1K | 1K（我方 DTO 丢弃该字段） | 一致 |

---

## 逐用例实跑结果（第 6 版定稿）

表格单元格格式：`matched_tier / quota (¥折算)`。`ERR` 表示表达式运行报错（预扣阶段即 400 拒绝请求，`relay/helper/price.go:317-319` → `controller/relay.go:165-169`，**不是免费放行**）。gpt 列在 `imageSize`/字面量用例下全部 1K 是设计。
#### 0. 编译检查与长度

- A gpt-image-2: compile err=<nil>，长度 272 字符，3 行
- B/C gpt-image-2.5-*: compile err=<nil>，长度 272 字符，3 行
- D gemini-3-pro: compile err=<nil>，长度 1184 字符，11 行
- E/F gemini-3.1-flash*: compile err=<nil>，长度 1184 字符，11 行

#### 1. 真实生产尺寸（body = {"size":"…"}）—— 阈值修正后

| 输入 body | A gpt-image-2 | B/C gpt-image-2.5-* | D gemini-3-pro | E/F gemini-3.1-flash* |
|---|---|---|---|---|
| `{"size":"1024x1024"}` (px=1048576) | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1536x1024"}` (px=1572864) | 1k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1024x1536"}` (px=1572864) | 1k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1254x1254"}` (px=1572516) | 1k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1248x1248"}` (px=1557504) | 1k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1100x1400"}` (px=1540000) | 1k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1200x1200"}` (px=1440000) | 1k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1664x944"}` (px=1570816) | 1k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1672x941"}` (px=1573352) | 2k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 2k / 75000 (¥0.15) | 2k / 150000 (¥0.30) |
| `{"size":"1573x1573"}` (px=2474329) | 2k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 2k / 75000 (¥0.15) | 2k / 150000 (¥0.30) |
| `{"size":"2048x896"}` (px=1835008) | 2k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 2k / 75000 (¥0.15) | 2k / 150000 (¥0.30) |
| `{"size":"2048x1152"}` (px=2359296) | 2k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 2k / 75000 (¥0.15) | 2k / 150000 (¥0.30) |
| `{"size":"1152x2048"}` (px=2359296) | 2k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 2k / 75000 (¥0.15) | 2k / 150000 (¥0.30) |
| `{"size":"2560x1440"}` (px=3686400) | 2k / 60000 (¥0.12) | 2k / 80000 (¥0.16) | 2k / 75000 (¥0.15) | 2k / 150000 (¥0.30) |
| `{"size":"2001x2001"}` (px=4004001) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"2000x2000"}` (px=4000000) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"2288x1824"}` (px=4173312) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"2048x2048"}` (px=4194304) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"2880x2880"}` (px=8294400) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"3456x1728"}` (px=5971968) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"3824x2144"}` (px=8198656) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"3840x2160"}` (px=8294400) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"2160x3840"}` (px=8294400) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"2208x3360"}` (px=7418880) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"4096x4096"}` (px=16777216) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"3001x3001"}` (px=9006001) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"3344x1882"}` (px=6293408) | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |

#### 2. size 字面量 / 宽高比 / auto / 缺失 / 异常输入（gpt 不认字面量，gemini 认）

| 输入 body | A gpt-image-2 | B/C gpt-image-2.5-* | D gemini-3-pro | E/F gemini-3.1-flash* |
|---|---|---|---|---|
| `{"size":"4K"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"4k"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":" 4K "}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"2K"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 2k / 75000 (¥0.15) | 2k / 150000 (¥0.30) |
| `{"size":"1K"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"4K UHD"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1024"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"512px"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"0.5K"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1:1"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"4:3"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"16:9"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"auto"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":""}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":null}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"prompt":"x"}（无 size）` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `not json` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `<nil>（multipart，代码修复前）` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":1024}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"999999999999x1"}` | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `{"size":"99999999999999999999x1"}` | ERR: expr run error: invalid operation: int(99999999999999999999) (2:46) | ERR: expr run error: invalid operation: int(99999999999999999999) (2:46) | ERR: expr run error: invalid operation: int(99999999999999999999) (9:46) | ERR: expr run error: invalid operation: int(99999999999999999999) (9:46) |
| `{"size":"0x0"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1024×1024"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":" 1024x1024"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"2048X1152"}（大写 X）` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |

#### 3. 张数 n（size=1024x1024；带 † 的 body 在校验层已被 400 拒绝，到不了表达式）

| 输入 body | A gpt-image-2 | B/C gpt-image-2.5-* | D gemini-3-pro | E/F gemini-3.1-flash* |
|---|---|---|---|---|
| `{"size":"1024x1024","n":1}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1024x1024","n":3}` | 1k / 180000 (¥0.36) | 1k / 180000 (¥0.36) | 1k / 225000 (¥0.45) | 1k / 450000 (¥0.90) |
| `{"size":"1024x1024","n":128}` | 1k / 7680000 (¥15.36) | 1k / 7680000 (¥15.36) | 1k / 9600000 (¥19.20) | 1k / 19200000 (¥38.40) |
| `{"size":"1024x1024"}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1024x1024","n":null}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1024x1024","n":0}` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1024x1024","n":"3"} †` | 1k / 180000 (¥0.36) | 1k / 180000 (¥0.36) | 1k / 225000 (¥0.45) | 1k / 450000 (¥0.90) |
| `{"size":"1024x1024","n":129} †` | 1k / 7740000 (¥15.48) | 1k / 7740000 (¥15.48) | 1k / 9675000 (¥19.35) | 1k / 19350000 (¥38.70) |
| `{"size":"1024x1024","n":-1} †` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `{"size":"1024x1024","n":2.5} †` | 1k / 150000 (¥0.30) | 1k / 150000 (¥0.30) | 1k / 187500 (¥0.38) | 1k / 375000 (¥0.75) |
| `{"size":"4096x4096","n":3}` | 4k / 315000 (¥0.63) | 4k / 315000 (¥0.63) | 4k / 300000 (¥0.60) | 4k / 525000 (¥1.05) |
| `{"size":"2048x1152","n":2}` | 2k / 120000 (¥0.24) | 2k / 160000 (¥0.32) | 2k / 150000 (¥0.30) | 2k / 300000 (¥0.60) |
| `{"size":"4K","n":2}` | 1k / 120000 (¥0.24) | 1k / 120000 (¥0.24) | 4k / 200000 (¥0.40) | 4k / 350000 (¥0.70) |
| `{"size":"2048x1152","n":4}（multipart 编辑修复后投影 body）` | 2k / 240000 (¥0.48) | 2k / 320000 (¥0.64) | 2k / 300000 (¥0.60) | 2k / 600000 (¥1.20) |

#### 4. gemini 的 imageSize 五种写法 / 字面量 / 探测链边界 / 与 size 的优先级（gpt 列不认 imageSize，仅供对照）

| 输入 body | A gpt-image-2 | B/C gpt-image-2.5-* | D gemini-3-pro | E/F gemini-3.1-flash* |
|---|---|---|---|---|
| `原生 imageSize=4K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `原生 imageSize=2K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 2k / 75000 (¥0.15) | 2k / 150000 (¥0.30) |
| `原生 imageSize=1K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `原生 imageSize=4k（小写）` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `原生 imageSize=" 4K "` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `原生 只有下划线 image_config.image_size=4K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `混写 image_config.imageSize=4K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `混写 imageConfig.image_size=4K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `extra_body=4K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `extra_body=2K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 2k / 75000 (¥0.15) | 2k / 150000 (¥0.30) |
| `两者都有：驼峰 4K + 下划线 1K（DTO 取下划线）→ 1K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `两者都有：驼峰 1K + 下划线 4K → 4K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `驼峰 "" + 下划线 4K（空串卡链）→ 4K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `驼峰 null + 下划线 4K → 4K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `驼峰 "" + extra_body 4K → 4K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `原生 imageSize 是数字 4` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `原生 imageSize 是对象` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `原生 imageSize=512px` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `原生 imageSize=0.5K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `原生 只传 aspectRatio` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `原生 imageConfig 空对象` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `原生 无 imageConfig` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `顶层 generation_config（DTO 丢弃）` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `chat 顶层 image_config（DTO 丢弃）` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `imageSize=1K + size=4K（imageSize 优先）→ 1K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `imageSize=4K + size=1K → 4K` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `imageSize=2K + size=4096x4096 → 2K` | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 2k / 75000 (¥0.15) | 2k / 150000 (¥0.30) |
| `imageSize=512px + size=4096x4096（imageSize 非枚举 → 看 size）→ 4K` | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |
| `quality=4K（六条都不读 quality）` | 1k / 60000 (¥0.12) | 1k / 60000 (¥0.12) | 1k / 75000 (¥0.15) | 1k / 150000 (¥0.30) |
| `quality=1K + size=4096x4096 → 面积 4K` | 4k / 105000 (¥0.21) | 4k / 105000 (¥0.21) | 4k / 100000 (¥0.20) | 4k / 175000 (¥0.35) |

#### 5. 单行 / CRLF 与多行结果一致

- A gpt-image-2: 多行 1k / 120000 (¥0.24) | 单行 1k / 120000 (¥0.24) | CRLF 1k / 120000 (¥0.24)
- B/C gpt-image-2.5-*: 多行 2k / 160000 (¥0.32) | 单行 2k / 160000 (¥0.32) | CRLF 2k / 160000 (¥0.32)
- D gemini-3-pro: 多行 1k / 150000 (¥0.30) | 单行 1k / 150000 (¥0.30) | CRLF 1k / 150000 (¥0.30)
- E/F gemini-3.1-flash*: 多行 1k / 300000 (¥0.60) | 单行 1k / 300000 (¥0.60) | CRLF 1k / 300000 (¥0.60)


---

## 前端

- **保存不改写**：bun 实跑 `splitBillingExprAndRequestRules → combineBillingExpr`，6 条多行文本保存后与原文逐字相等、行数不变，`requestRuleExpr` 为空；`parseTiersFromExpr` 识别出 `4k/2k/1k`（定价页显示档位但单价栏为 0，展示问题）。
- **档位名**：`tier("4k", …)` 的名字原样写入日志 `other.matched_tier`（`service/log_info_generate.go:352-368`）；用量日志按同名匹配显示（`web/src/features/usage-logs/lib/format.ts:291-303`、`common-logs-columns.tsx:154-165`、`details-dialog.tsx:1101-1107`）。
- **估算器**：对 6 条都显示 `Unexpected keyword 'let'`；估算器是纯 JS `new Function`（`tier-expr.ts:305-308`），任何含 `param()` 的表达式都会报错。**不阻断保存**：提交链 `model-pricing-sheet.tsx:413-439,466-476` 只校验 per-token 字段；`model-ratio-visual-editor.tsx:405-413,591-599` 原样写 `billing_setting.billing_expr`；后端 `model/option.go:293-301` 无表达式校验。详见 `no-let-expressions.md`。

## 校验层：哪些 body 根本到不了表达式

用真实的 `relay/helper.GetAndValidOpenAIImageRequest(c, RelayModeImagesGenerations)`（JSON `/v1/images/generations`）实跑：

| body | 校验结果 | 进入表达式时的状态 |
|---|---|---|
| `…,"size":"1024x1024","n":0}` | 通过 | DTO.N 归一为 1，但 `param("n")` 读原文仍是 0 → `max(…,1.0)` |
| `…,"n":"3"}` | **400**：`json: cannot unmarshal string into Go struct field Alias.n of type uint` | — |
| `…,"n":129}` | **400**：`n must be an integer between 1 and 128`（`valid_request.go:249-251`，`dto.MaxImageN`） | — |
| `…,"n":-1}` / `…,"n":2.5}` | **400**：`cannot unmarshal number … type uint` | — |
| `…,"n":null}` | 通过 | DTO.N=1；`param("n")` 为 nil → `?? 1` |
| `…,"size":1024}`（数字） | **400**：`cannot unmarshal number into … size of type string` | — |
| `…,"size":"1024×1024"}`（全角） | **400**：`valid_request.go:245-247` | — |
| `size` 任意字符串 | 通过 | 无白名单，原样进表达式 |

multipart `/v1/images/edits` 走 `valid_request.go:186-232`：`n` 校验 0..128（`:197-203`）、nil/0 归一为 1（`:222-224`）；修复后投影 body 里 `n` 永远是 1..128。

## 其他已确认的行为

| 场景 | 行为 | 判定 |
|---|---|---|
| `size` 超 int64（`99999999999999999999x1`） | 六条都 `int()` 运行错误 → 预扣阶段 400 拒绝 | 拒绝，不免费 |
| `size` 超大但可解析（`999999999999x1`） | 4K | 上游拒绝无效尺寸 → 非 2xx 不结算并退预扣（`relay/image_handler.go:100-109`、`controller/relay.go:182-191`） |
| `2048X1152`（大写 X）、前导空格、`0x0`、全角 | 面积正则不匹配 → 1K | 保守 |
| gemini `imageSize` 为数字、对象、`512px`、`0.5K`、空、null | 非枚举 → 看 `size` → 通常 1K | `512px` 上游会静默回落 1K |
| gpt 收到 `size:"4K"` 字面量、`generationConfig.*`、`extra_body.*`、`quality:"4K"` | 一律按面积/兜底 → 1K | gpt 请求里不存在这些写法（30 天日志零样本） |
| `quality` | 六条都不读 | 30 天日志里 `quality` 全是常规值 |
| `img_o` / `p` / `c` | 未引用 | 预扣 = 结算 |
| `request_rules` 追踪 | 为空 | 日志只有 `matched_tier` |
| multipart `/v1/images/edits`（修复前） | `param()` 为 nil → 1K、张数按 1 | `multipart-billing-fix-design.md` |

## 上线检查清单

1. 6 个模型分别在「计费模式：表达式编辑器 → Raw」粘贴对应文本（多行原样），忽略估算器红框，保存。后端保存时不做编译校验，写错要到请求时才 400。
2. gpt 三条用测试令牌各打一次：`1024x1024`（1k）、`1536x1024`（gpt-image-2 1k / flare 2k）、`2048x1152`（2k）、`2560x1440`（2k）、`2048x2048`（4k）、`auto`（1k）、`n=2`、`n=0`（应扣 1 张）。
3. gemini 三条走原生 `generateContent` 各打一次：`imageConfig.imageSize` = `4K` / `2K` / `1K` / 缺省；再用 OpenAI images 打 `size:"2048x2048"`（4k）、`size:"4K"`（4k）、`size:"auto"`（1k）。看日志「命中档位」与扣费额是否与本文表格一致。
4. **验收红线**：任一档位扣费额为 0 即表示单位写错，立即回滚。
5. multipart 代码修复上线后：用 multipart 编辑打 `size=2048x1152, n=2`（flare 应 2k × 2 = ¥0.32），确认投影生效；表达式不需要改。
6. 下游网关粘同一份即可（只用了 `param/tier/max` + expr-lang 内置，两端引擎相同）。

## 附录 A：曾评估、未采用的判档来源

- **`quality` 字面量**：上游 tuzi 表达式里有，但 30 天日志里 `quality` 全是 `standard/medium/high/low/auto`，那是 tuzi 内部兼容处理；第 4 版曾加进统一模板，按用户要求砍掉。
- **gpt 三条上的 `imageSize` 五段链与 `size` 字面量**：gpt 请求里不存在这些写法，只按面积。
- **`isMultipart` 兜底档**（`has(header("content-type"), "multipart/form-data")`）：判别式本身可用且零误判（真实 gin 上下文实跑），但只是把所有编辑请求一刀切到一个档，`n` 仍读不到；已被代码修复方案取代。
- **无 `let` 单行写法**：估算器对它同样报错（`Invalid character: '#'`），没有收益且不可读。

## Caveats

- flare/sunburst 的三档成本与 2K 边界来自 team-lead 转述的上游 flare 表达式与账单；sunburst 未单独见到，按与 flare 同口径。
- gpt-image-2 与 gemini 的 1K/2K 边界 `1,572,864` 只影响档位名（同价）；gemini 走 OpenAI 兼容路径的 2K/4K 边界沿用 gpt-image-2 的 `3,686,400`，gemini 自身在该路径上的上游边界未见账单（bill 里 flash `2048x2048` 收低价，本口径按客户端声明判 4K，属用户已接受的多收方向）。
- 未起服务做 HTTP 端到端；表达式行为来自对真实 `pkg/billingexpr` 的实跑，前端行为来自对真实 `billing-expr.ts` / `tier-expr.ts` 的实跑。
