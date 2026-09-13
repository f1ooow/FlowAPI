# 图像模型按分辨率阶梯计费

## Goal

6 个图像模型从「一刀切单价」改为「按分辨率分档计价」，**通过后台表达式配置实现，零代码改动**。

当前问题实测：`gemini-3-pro-image-preview` 请求 `imageSize=1K` 交付 1024×1024 (330 KB)、`imageSize=4K` 交付 4096×4096 (20.8 MB)，像素差 16 倍、体积差 63 倍，**收费都是 ¥0.20**。

## Constraints

- **以配置为主，允许最小幅度的代码改动**（2026-09-13 用户确认变更）。原约束为「纯配置、不得二开」，理由是下游还有一个 new-api 网关要配同一套规则。后因 multipart 判档缺口影响 88% 的编辑流量、且 workaround 方案都不干净，用户确认下游网关同样由己方部署，允许改代码，要求「用最优雅的方案」。
  - 代码改动必须能直接 cherry-pick 到下游网关，不得依赖 FlowAPI 特有的东西。
  - 表达式部分仍只使用当前基线（merge-base 08-27）与下游**共有的函数子集**；上游 new-api 2026-09-09 新增的 `fixed(amount)` / `image_count` 不使用。
- 站点 ¥:$ = 1:1。上游 tuzi 汇率 $1 = ¥0.7。

## Scope

本次配置 6 个模型：

| 模型 | 1K | 2K | 4K |
|---|---|---|---|
| gpt-image-2 | 0.12 | 0.12 | 0.21 |
| gpt-image-2.5-flare | 0.12 | 0.16 | 0.21 |
| gpt-image-2.5-sunburst | 0.12 | 0.16 | 0.21 |
| gemini-3.1-flash-image-preview | 0.3 | 0.3 | 0.35 |
| gemini-3.1-flash-image | 0.3 | 0.3 | 0.35 |
| gemini-3-pro-image-preview | 0.15 | 0.15 | 0.2 |

单位为人民币/张。`gemini-3.1-flash-image`（无 `-preview`）由用户确认与小香蕉同价，其现有 0.2 单价一并替换。

**不在本次范围**：
- `gpt-image-2.5-sunburst` 掉进 token 计费的历史事故（ModelPrice 缺条目，一张 2880×2880 仅扣 ¥0.00036，与同尺寸 gpt-image-2 的 ¥0.15 相差 417 倍）。用户决定回头与本次改动一起处理，本次只要配上表达式即自然消除。
- `gpt-image-1`（30 天 0 请求、未启用）、`gpt-image-2.5`（已从 abilities 下线，残留 ModelPrice 0.15）。

## 分档规则

**两类模型边界不同，不可共用阈值。** 依据上游 tuzi 账单导出（160 行，每个尺寸单价零方差）：

### gpt-image 系 —— 按像素面积

| 档 | 条件 | 账单实证尺寸 | 上游价 |
|---|---|---|---|
| 1K | `px <= 1,572,864` | 1024x1024(1.05M)、1536x1024(1.57M)、不传 size、auto | $0.04 / $0.1714 |
| 2K | `1,572,864 < px < 4,194,304` | 2048x896(1.84M)、2048x1152(2.36M)、2560x1440(3.69M) | $0.1714 / $0.2286 |
| 4K | `px >= 4,194,304` | 2048x2048(4.19M)、3456x1728(5.97M)、2208x3360(7.42M)、3824x2144(8.20M)、2880x2880(8.29M) | $0.30 |

**禁止使用「最长边」规则**：`2048x2048`（最长边 2048）是上游最高档，`2560x1440`（最长边 2560）却是中档，最长边会判反。独立验证：上游 output image token 与交付像素面积严格线性（1826.60 px/token，5 样本变异系数 0.015%），成本比即面积比。

### gemini 系 —— 按 `imageSize` 字面量

上游严格执行 `generationConfig.imageConfig.imageSize`，取值为干净的 `1K`/`2K`/`4K`，实测零偏移。

**边界与 gpt-image 系不同**：`2048x2048` 在 gemini-3.1-flash 上属**最低档**（$0.409，与 1024x1024 同价），仅 `4096x4096` / 字面量 `4K` 属高档（$0.464）。这与用户「1K/2K 同价」的定价结构一致。

判档字段优先级：`generationConfig.imageConfig.imageSize` → `extra_body.google.image_config.image_size` → `size`。**不得单独按 `size` 判档**：实测请求 `size=4096x4096` 上游交付 1024×1024，该字段上游不认。

### 兜底

`auto`、空值、字段缺失、非法串、multipart 编辑图一律落 **1K 档**。依据：实测不传 size 与传 `1024x1024` 上游行为完全一致（同为 1254×1254、1056 tokens），且账单中 `auto` 与不传 size 均落最低档。取少收不多收。

## Acceptance Criteria

- [ ] 6 个模型在后台「模型定价 → 表达式编辑器」配置完成，`billingMode` 为 `tiered_expr`
- [ ] 配置前已导出 `gemini-3-pro-image-preview` 的现有表达式作为回滚基线（该模型已在 tiered_expr 模式运行）
- [ ] 每个模型用测试令牌实打 1K / 2K / 4K / auto 各一次，核对用量日志的 `matched_tier` 与扣费额与价格表一致
- [ ] gemini 系额外验证：原生 `generateContent` + `imageConfig.imageSize=4K` 命中 4K 档
- [ ] 同一份表达式在下游 new-api 网关粘贴生效，无语法错误
- [ ] 确认无「静默免费」：任一档位扣费额不为 0

## 已知限制（配置前须接受）

1. **按「请求的尺寸」计费，非「实际交付的尺寸」**。实测上游常不按请求尺寸交付：`gpt-image-2` 请求 4096x4096 实际给 2880×2880（上游自标 "4K"）。表达式只能读请求体，读不到响应。
2. **multipart `/v1/images/edits` 判不了档**。Playground 编辑图走 multipart，`param()` 全部取不到值，固定落 1K 档。FlowAPI 当前基线不支持 multipart 读 size（上游 09-09 已修，未合入）。
3. **张数按声明的 `n` 收，不按实际出图数**。`tiered_expr` 模式下 `updateOpenAIImageCount` 因 `UsePrice=false` 早退。`n` 已被 `dto.MaxImageN=128` 封顶，无溢出风险。
4. **保存表达式时无任何校验**。语法错误要到真实请求才返回 400；单位写错（如写 `0.12` 而非 `120000`）会算出 0 quota —— **图片正常交付、一分钱不收、日志无异常**。故验收必须实打。
5. **渠道 55 背后有两个上游后端**，同模型随机落到不同实现。后端 B 会把 4096x4096 降级交付 2880x2880 且 usage 标 `"source": "estimated"`。分档按请求参数做，不受此影响。
6. **`img_o`（输出图 token）路线已排除**。实测 `gpt-image-2` 请求 1024x1024 得 img_o=1056、请求 4096x4096 得 img_o=659 —— 值与分辨率反向；且主力渠道 55 对 flare 的 usage 覆盖率 6.3%、对 gemini-3.1-flash 为 0%；上游自标 estimated；同为 2880×2880 输出，gpt-image-2 报 659、sunburst 报 1483。
7. **前端定价页显示异常（仅展示）**。`parseTiersFromExpr` 的正则只认 `p|c|len` 条件，`param()` 条件的表达式会显示出档位但单价为 0。计费与用量日志的 `matched_tier` 均正确。

## 待确认

- gemini 原生 `generateContent` 流量来自用户自有的广州 urun 服务器（`8.138.x.x`）。该客户端是否固定传 `imageSize` 决定 gemini 的判档覆盖率 —— 网关不留存请求体，无法回溯查证，需用户从客户端侧确认或改造。

## References

- `research/hk-production-logs.md` — 生产日志分布、模型名确认、计费现状
- `research/live-probe-results.md` — 8 次实测，img_o 路线否决、默认档确定
- `research/upstream-cost-probe.md` — 14 次实测，面积线性关系、gemini imageSize 行为
- `research/billing-chain.md` — 表达式引擎能力、单位语义、6 条表达式实跑验证
- `research/final-expressions-verification.md` — 定稿表达式逐用例验证
