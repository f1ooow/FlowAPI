# Research: R8 音乐占位模型名 `minimax-music-generation`

- **Query**: 为何拒绝占位名；占位名在代码、清单、定价、能力表和渠道中的出现位置；FlowAPI 与 keli 的完整行为；兼容与不兼容两种方案的影响
- **Scope**: mixed（FlowAPI/keli/Canvas 源码 + 导入清单 + MiniMax 公开文档）
- **Date**: 2026-09-28

## 1. 为何拒绝

- `relay/channel/minimax/music.go:36-42`：`upstreamModel` 取映射后的 `UpstreamModelName`，为空时回退到 `OriginModelName`。如果它等于占位名，就返回 **400 `configure music model mapping`**（`miniMaxError` 带 SkipRetry，`extended.go:21-23`）。
- 设计意图：占位名只用于路由和计价，必须通过渠道的 model_mapping 映射到真实模型后才能发给上游。
- MiniMax 官方《音乐生成》（`docs/api-reference/music-generation.md`）：`model` 必填，enum 为 `music-3.0`、`music-2.6`、`music-cover`、`music-3.0-free`、`music-2.6-free`、`music-cover-free`。**占位名不在其中。**
- 官方服务调整：
  - 同一文档写明，自 2026-08-20 起付费音乐接口不再面向新用户，历史付费用户可以继续使用；
  - 免费接口（-free 系列）停止服务；
  - 《按量计费》（`pricing-paygo.md`）音乐段中的所有模型都标为“已下线”。

## 2. 占位名出现在哪里

| 位置 | 证据 |
|---|---|
| 代码默认值 | `middleware/distributor.go:435-437`：`/v1/music_generation` 请求不带 model 时，默认使用占位名 |
| 适配器模型列表 | `relay/channel/minimax/constants.go:10` |
| 拒绝逻辑 | `relay/channel/minimax/music.go:40-41` |
| 能力表 | `model/media_billing_units.json:87-90` `{"fixed":"request","task":true}` |
| 定价（导入清单） | `09-22-flowapi-pricing-verification/target-pricing-proposal.json:42` 中 `model_price` 为 1；`:495` 的 row 为 unit request，source_channels [49] |
| 渠道模型列表 | `flowapi-disabled-channels.json:64`：源 49 → FlowAPI 71 的 models 包含占位名 |
| 导入前状态 | `09-22-flowapi-provider-coverage/research/coverage-matrix.json:755` 标注 `absent_from_flow_channel_lists: true`；源 49 的 `model_mapping: null` |
| 测试 | 没有音乐测试。`controller/relay_minimax_extended_test.go` 只测上传免费和克隆价格 |

- 71 在生产上是否配置了 model_mapping：清单中源 49 的 `settings: {}`，也没有 model_mapping。**推断没有配置，生产 DB 未核实。**
- 71 挂的真实音乐模型（来自清单）：music-2.6、music-2.6-free、music-2.5+、music-2.5、music-2.0、music-cover、music-cover-free。
  - 缺少 music-3.0；
  - -free 系列已停服；
  - 2.5 / 2.5+ / 2.0 不在当前官方 enum 中。

## 3. FlowAPI 的完整行为（带占位名或不带 model）

1. `distributor.go:382-388` 从 JSON body 取 model；`:435-437` 在为空时填入占位名，并设置 `relay_mode=Music`。
2. 如果 token 开启了模型限制（`:65-83`），限制列表必须包含占位名。
3. 在分组内选出挂有占位名的渠道（71）。`SetupContextForSelectedChannel` 设置 `original_model` 为占位名（`:501`）。
4. `controller/relay_minimax_extended.go` 的处理：
   - `:47` 检查渠道类型；
   - OriginModelName **不被覆盖**（`:51-56` 只覆盖克隆和上传）；
   - `:57` 调用 `ModelMappedHelper`（`relay/helper/model_mapped.go:18-28`），如果渠道配置了映射，UpstreamModelName 改为映射目标。
5. `:61` 调用 `miniMaxExtendedPrice`（`:25-41`）：占位名必须显式配置价格（清单为 1），否则返回 400 `model_price_error`。
6. `:68-74` 预扣 1 × QuotaPerUnit × GroupRatio。
7. 进入 `RelayMiniMaxMusic`：
   - 先校验 prompt ≤2000、lyrics ≤10000 rune、duration（`:22-35`）；官方 lyrics 上限是 3500，官方也没有 duration 参数；
   - 没有映射时返回 400，**不请求上游**。
8. controller 在 `:94-96` 返回 400，defer（`:75-79`）退回预扣，**不扣费，也不写消费日志**。

其他情况：
- 71 若配置了 `{"minimax-music-generation":"music-2.6"}`：上游收到 `model=music-2.6`，按占位名的价格 1 计费。`callMiniMaxExtended` 会检查 HTTP 状态和 `base_resp`，失败时退款（`extended.go:58-69`）。
- 客户端直接传真实模型名（如 music-2.6）：按该名路由和计价（清单中为 1），正常转发。

## 4. keli 的处理

- keli `relay/channel/minimax/music.go:37-102` **原样转发请求 body**（`:46`、`:61`）：
  - 不改 model，也不应用映射；
  - 不检查 HTTP 状态和 `base_resp`，直接 `c.Data(resp.StatusCode, …)`（`:100`），并一律返回 usage（`:102`）；
  - keli controller 按 OriginModelName 按次预扣并结算（keli `controller/relay_minimax_extended.go:36`、`:47`、`:85`）。
- 所以 keli 会把占位名**原样发给 MiniMax**。如果 body 本身没有 model，发给 MiniMax 的请求也没有 model（distributor 的默认值只用于路由，keli `middleware/distributor.go:413`）。
- MiniMax 是否接受：`model` 是必填的 enum，占位名不在其中，预计返回参数错误（如 2013）。**未实测，未确认**（按约束未发真实请求）。
- 由此推断：如果上游返回 HTTP 200 + `base_resp` 错误，keli 仍会按次扣费。这是 keli 的缺陷，不在本任务范围内。keli 的“兼容”并不能作为可用的基线。
- 没有发现依赖占位名的已知客户端：
  - Canvas 不调用音乐接口，`keli-canvas/server/lib/agent/keliModelRegistry.ts:105-119` 只是把 `music-`、`minimax-music` 从文本模型列表中排除；
  - FlowAPI `web/src` 中也没有调用。

## 5. 方案与影响

| 方案 | 做法 | 影响 |
|---|---|---|
| A 兼容（代码默认映射） | 占位名没有映射时，在代码里默认映射到某个真实模型（music-2.6 或 music-3.0） | 不带 model 的客户端可以用；按占位名的价格 1 计费（music-2.6/3.0 刊例价也是 1 元/首）；默认选哪个模型是产品决策；代码里硬编码了供应商模型名，下线时要改代码；能否成功取决于账号是否属于历史付费用户（未确认） |
| B 兼容（只改配置） | 给 71 配置 `{"minimax-music-generation":"music-2.6"}` | 不改代码，与现有报错提示的设计一致；可以通过运维调整；其余影响同 A |
| C 不兼容 + 清理 | 保持 400，同时把 distributor 的默认值改为报 “model is required”，并从 `constants.go` 的 ModelList、71 的模型列表、定价、`media_billing_units.json` 中移除占位名 | 客户端必须传真实模型；消除“定价页可见、调用必然 400”的状态；现在已经不扣费，没有计费风险 |
| 维持现状 | 不改 | 占位名在 71 的模型列表和价格表中可见，调用 100% 返回 400，不扣费 |

无论选哪种方案，还有一个共同问题：71 的真实音乐模型列表与官方现状不一致（缺 music-3.0，列着已停服的 -free 和已下线的旧型号）。建议另行确认账号是否仍能使用音乐接口。

## Caveats / Not Found

- MiniMax 对占位名的实际报错、账号是否仍有付费音乐权限：未确认。
- 71 的生产 model_mapping：未直接核实。
