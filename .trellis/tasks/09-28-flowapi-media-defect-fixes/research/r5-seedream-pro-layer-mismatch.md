# Research: R5 Seedream 5.0 Pro 图层拆分 generated_images 与 data 条数不一致

- **Query**：FlowAPI 在什么情况下会判定"不一致"；不一致时是否已被上游扣费、用户能否拿到结果、预扣是否退还；keli 怎么处理
- **Scope**：mixed（FlowAPI、keli、keli-canvas 的代码与文档；火山方舟文档仅有本地副本，未联网核实）
- **Date**：2026-09-28

## 结论

FlowAPI 发现 `usage.generated_images` 与 `data` 数组长度不一致时，会在写回客户端之前返回 502，并且不重试、全额退还预扣。这是有文档记载的有意设计，不是代码写错了。但业务后果是：上游已经出图并扣费，用户拿不到任何图，这笔上游费用由运营方承担。

最可能触发不一致的场景是部分图片生成失败：此时 `data` 中会出现 `{error:{...}}` 项。火山文档记载了组图场景下会出现这种失败项，但图层拆分是否同样如此，未确认。

keli 不做一致性校验，直接把响应写给客户端，并按 `generated_images × 0.5` 计费。

## 1. 现状（FlowAPI）

### 1.1 请求阶段

- 判定为图层拆分请求的条件：模型为 `doubao-seedream-5-0-pro-260628`、`layer_decomposition=true`、`n≤1`、非流式。见 `relay/image_seedream.go:85-95` 和 `relaykit/dto/seedream_image.go:25-27`。
- 预扣：`n` 按最大值 `17 × 0.5 = 8.5` 预留（`relay/image_seedream.go:43-49`）。常量定义在 `relaykit/dto/seedream_image.go:8-10`：`MaxSeedreamLayerImages=17`，`SeedreamLayerImagePriceFactor=0.5`。

### 1.2 响应阶段：校验逻辑

位置：`relay/channel/openai/relay_image.go:91-111`。

- `count` 取 `gjson "data.#"`（`:92`），会把 data 中的所有元素计入，包括 error 项。
- 判定条件（`:102`）：

  ```go
  if err != nil || generated == nil || *generated < 1 || *generated > 17 || count != *generated {
  ```

- 不满足时：
  - 只记一条 `LogWarn`（`:103`），日志里不含两个计数值，也不含上游 request id；
  - 返回 502 + `ErrorCodeBadResponseBody` + `ErrOptionWithSkipRetry`（`:104`）。
- 满足时（`:106-109`）：按 `n = generated × 0.5` 结算（替换预扣时的 8.5），前提是 `UsePrice` 为真且没有表达式快照。

### 1.3 响应阶段：校验在写回之前

- 非流式：先校验（`:58-60`），后写回（`:63`）。
- JSON 当流式处理：同样先校验（`:325-328`）。
- 上游若返回 SSE 流，直接判 502（`:155-158`）。

### 1.4 错误出口

- 控制器在请求出错时执行 `relayInfo.Billing.Refund(c)`，全额退还预扣（`controller/relay.go:182-191`）。
- skip-retry 错误会停止渠道遍历，不重试（`controller/relay.go:323-338`）。
- skip-retry 错误不会触发渠道自动禁用（`service/channel.go:87-89`）。

### 1.5 测试把 502 固化为合同

- `relay/channel/openai/seedream_image_test.go:51-69` `TestSeedreamLayerRejectsUntrustworthyUsageBeforeWriting`。其中 `generated_images:2` 且 data 只有 1 项的用例（`:52`）断言：
  - 返回 502；
  - skip-retry；
  - 响应体为空；
  - `n` 保持 8.5。
- `:18-49` 覆盖 1 张与 17 张两种正常情况。

### 1.6 设计文档

- `.trellis/tasks/09-22-flowapi-seedream-images/billing-contract.md:22-23`：
  - 合法的图层 usage 必须满足：`generated_images` 为 1..17 的整数，且等于返回的 data 数组长度；
  - 缺失、为 0、为负、为小数、为字符串、溢出、越界或与 data 不一致，一律在转发图片之前返回 502，不重试；
  - 交由现有控制器记日志并退款，"绝不作为零价成功"。
- `.trellis/tasks/09-22-flowapi-seedream-images/design.md`：要求"不能照搬（keli）'上游数量天然安全'的注释作为验证"。

## 2. 什么情况会不一致（证据）

1. **部分图片失败，data 中出现 error 项（可能性最大，图层场景未确认）**
   - `keli-canvas/.trellis/spec/important/project-docs/seedream-api-reference.md:197-204` 记载了方舟官方文档（2026-05-23 由用户粘贴）的组图失败处理：审核不通过时继续生成下一张，内部 500 时停止后续，失败项形式为 `data: [{ error: { code, message } }]`。
   - keli-canvas 客户端专门防御图层结果中的 error 项：`server/lib/seedreamLayers.ts:23` 遇到 `item.error` 即抛出"Provider returned an incomplete image result"，测试 `server/lib/seedreamLayers.test.ts:67-73` 包含 `{ error: { code: 'rejected' } }` 用例。
   - 如果 `generated_images` 只统计成功的图（字段名如此暗示，未确认），那么 `data.#` 会大于 `generated_images`，FlowAPI 返回 502。
2. **中转站改写响应**：82 号渠道 Zivv 是 type 60（New API）。如果它重新序列化响应、过滤 error 项、增删 usage，两个数字就可能对不上。未确认。反方向的证据是：keli 2026-09-14 经 Zivv 的实测样本里 usage 原样保留（见 3.1）。
3. **底图是否计入**：实测样本为 1 张底图 + 8 个图层，`generated_images = 9`，说明底图计入。两边口径一致，这一项不会造成不一致。
4. 超过 17 张、缺少 usage、字段类型不对等情况，同样走 502 路径（严格说不属于"不一致"）。
5. **实际触发频率**：未确认，没有查 HK 生产日志。而且 `:103` 的日志不记录两个数值，事后很难统计。

## 3. 与 keli 的差异

### 3.1 keli `9b80cd99c`（2026-09-14）的做法

- 先写回客户端，再算倍率：keli `relay/channel/openai/relay_image.go:24-48`（写回在 `:43`，计费在 `:47`）。
- `applyLayerDecompositionRatio`（`:100-112`）：
  - `GeneratedImages > 0`、`UsePrice` 为真、且入站 `Extra["layer_decomposition"]==true` 时，设 `n = generated_images × 0.5`；
  - 不校验 data 长度，也不设上限（注释称"火山侧天然有界"）；
  - `GeneratedImages <= 0` 时不设倍率，`image_handler` 回落到请求的 n（通常为 1），按一张全价收费（keli `relay/image_handler.go:128-133`）。
- 实测样本（keli `relay/channel/openai/layer_decomposition_billing_test.go:15-26`）：2026-09-14，调用链为画布 → keliapi → zivv → 方舟；1 张底图 + 8 个图层，`generated_images = 9`。fixture 中的 data 被裁剪到 2 项，注释写明"只保留计费用得到的字段"。
- 提交说明写明"上游按返回张数收费"，并有实测"同一张输入图两次分别拆出 9 张和 8 张"。

### 3.2 同一不一致场景下的对比

| | FlowAPI | keli |
|---|---|---|
| 返回给用户 | 502 错误，没有图 | 完整响应体，包括 error 项 |
| 向用户收费 | 0（预扣全额退还） | `generated_images × 0.5 ×` 单价 × 分组倍率 |
| 上游费用 | 已产生，由运营方承担 | 已产生，由用户付费覆盖 |
| 重试 | 不重试 | 不涉及 |

补充：keli 路径下，keli-canvas 客户端遇到 error 项会整单判失败（`seedreamLayers.ts:23`），此时用户已付费却拿不到可用结果。

## 4. 预扣、扣费与结果

- **上游是否已扣费**：上游返回了 HTTP 200 且带有图片，说明生成已经完成。按 keli 提交说明，上游按返回张数收费。Zivv 的具体计费口径未确认。
- **用户是否拿不到结果**：是。响应体在写回前就被拦下（测试断言 recorder 为空），用户只收到 502。重新提交会让上游再扣一次。
- **FlowAPI 是否退预扣**：是，全额退还。预扣额度为 ModelPrice（清单中为 0.25，见 `.trellis/tasks/09-22-flowapi-pricing-verification/target-pricing-proposal.json:33`）× 8.5 × 分组倍率，由 `controller/relay.go:186-188` 退还。

## 5. 是否构成缺陷

**判断：代码按设计运行（`billing-contract.md:23`），不是实现缺陷。**

但"上游已扣费、用户拿不到图、运营方承担上游成本"这一后果，设计文档没有权衡过，只写了"不当作零价成功"。是否接受这一后果属于产品决策。

触发条件（data 中是否真的会出现 error 项）目前没有 FlowAPI 侧证据，只有方舟组图文档和 keli-canvas 的防御代码作为间接依据。

## 6. 可选修复方案

| 方案 | 行为 | 风险 |
|---|---|---|
| A. 维持现状（可补日志，把两个计数和 request id 写进 `:103`） | 严格；从不错收用户 | 部分失败时用户拿不到图，运营方承担上游费用 |
| B. 信任 `generated_images`（仍限定 1..17），忽略 data 长度，照常写回（接近 keli，但保留上界） | 用户拿到部分结果，按上游报告的成功张数付费 | 上游若多报，用户多付；像 Canvas 这样遇到 error 项就判整单失败的客户端，会出现"已付费但结果不可用" |
| C. 只统计 data 中有 `url`/`b64_json` 且没有 `error` 的项，要求它等于 `generated_images`，否则仍返回 502 | 容忍 error 项，其余情况保持严格 | 依赖 error 项的格式（图层场景未确认）；其他类型的不一致仍会 502 |
| D. 按 min(成功项数, generated_images) 计费并写回，同时记录两个数值 | 不会多收用户 | 若上游按 `generated_images` 计费而 data 丢了项，会少收 |
| E. 按 max(...) 计费，不超过 17 | 对运营方最保守 | data 含 error 项且上游不对失败项计费时，用户多付 |

B、C、D 都会改变 `billing-contract.md:23` 的合同，需要同步修改该文档和 1.5 中的断言。

## 7. 建议的回归测试位置（现有文件）

- `relay/channel/openai/seedream_image_test.go`：
  - `:51-69` `TestSeedreamLayerRejectsUntrustworthyUsageBeforeWriting`，其中 `generated_images:2` 的用例需要随所选方案调整；
  - `:18-49` `TestSeedreamLayerResponseUsage`，可新增"data 含 error 项"的用例；
  - `:138-150` `TestSeedreamLayerMalformedResponseCannotBeRetried`。
- `service/seedream_image_billing_test.go:19` `TestSeedreamLayerPriceReservationAndSettlement`：覆盖预扣、结算、退款的精确金额。
- `relay/image_seedream_test.go:85` `TestSeedreamLayerReservationAndModePriority`：覆盖 8.5 的预扣。

## Caveats / 未确认

- 方舟 Seedream 5.0 Pro 图层拆分在部分失败时的响应格式，以及 `generated_images` 是否只统计成功项：只有组图文档作为间接依据，没有联网核实。
- Zivv 是否原样转发 usage 和 data，以及它的计费口径：未确认。
- 生产环境是否已经出现过此类 502：未查。
