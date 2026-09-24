# 技术设计与集成边界

## 架构

产品 → FlowAPI 路由/鉴权/选渠道 → 供应商适配器 → 供应商。异步请求由现有 task 持久化、轮询和结算层管理。KeliAPI 仅作为只读参考，不成为 FlowAPI 的上游依赖。

同步音频/图片沿用现有 Relay 与计费会话；异步视频/媒体沿用 TaskAdaptor 的 EstimateBilling、AdjustBillingOnSubmit、AdjustBillingOnComplete。端点模板和视频协议属于渠道设置，不按域名硬编码分支。

## 所有权与合并次序

| 所有者 | 独占实现面 | 共享修改的处理 |
|---|---|---|
| 父任务 | `constant/` 注册、`router/`、`middleware/distributor.go`、共享 DTO、`relay/relay_adaptor.go`、`relay/relay_task.go`、`controller/relay.go`、`model/task.go`、`service/task_polling.go`、`service/task_billing.go` | 先确定接口，再逐子任务合并；不得多代理同时覆盖 |
| video | `relay/channel/task/{ali,hailuo,doubao,sora,fastaitoken}/` 及新增 Wan 恢复/媒体时长专用文件 | Wan 数据表/路由、任务上下文、共享结算由父任务审入 |
| media | `relay/channel/task/{viapi,volcvod,volccv}/`、`relay/channel/{minimax,volcengine}/` 的音频/上传文件、MiniMax 扩展 controller | 与 images 共享的 volcengine adaptor/constants 交父任务串行整合 |
| images | Seedream 图片转换、`relay/channel/openai/relay_image.go` 的限定分层计费分支及图片回归 | 不触碰普通 GPT/Gemini 价格策略；共享 volcengine 入口走父任务 |
| pricing | `setting/ratio_setting/` 新模型默认值、`web/src/features/{pricing,system-settings,channels}/` 相关配置/展示、必要 i18n、配置与验收工件 | 后端结算修改由父任务统一，不能覆盖视频/音频作者的安全修改 |

实际 dispatch 时明确文件清单和接口，子代理知晓共享工作区且不得撤销他人改动。原生 Codex 上下文注入优先；不可用才加载子任务 manifests。实施前重新核对 dirty 状态，选择隔离工作树承载产品实现。

## 共享任务契约

1. 提交时保存公开/上游任务 ID、原模型/映射模型、原渠道及实际凭据身份；查询与文件/音色复用不切到其他账号。
2. 轮询初始化提供 ChannelId、ChannelType、ChannelOtherSettings；FetchTask 同时得到上游模型和 action。修复当前 FlowAPI 仅传 base/key 与 task_id/action 的缺口。
3. 保存 FlowAPI 已有 BaseGroupRatio、UserGroupRatio、ChannelRatio、IncludeChannelRatio、EffectiveRatio。异步结算读取提交快照，不用当天配置重新计价；历史任务兼容原快照。
4. TaskEndpointOverride 限同源相对路径，fetch 必须有且仅有一个 task_id 占位符；默认空设置保持现有行为。Wan 协议开关在 FlowAPI 的 `relaykit/dto/channel_settings.go` 扩展，不能创建重复 root DTO。
5. FastAI 源线上是 60，两个本地 fork 的 GetTaskAdaptor 均没有 60 分支。目标推荐复用独立 FastAI 适配器形式、配置为专用类型（若采用源 103 则先确认未占用）。只验证 Seedance time/token；Wan 不进入 FastAI 模型表，统一走 Rolldek 协议。
6. Wan 恢复保留 opt-in 协议、用户+令牌隔离、请求摘要冲突检测、发送前持久化和原任务关联。上游可能接受时保留 unknown，禁止重新付费提交；明确失败才退款，重复状态处理幂等。

## 计费与媒体安全

唯一有效倍率：base group × user group ×（启用渠道倍率 ? channel ratio : 1）。计费单位来自供应商协议及目标定价配置，不由显示标签决定。源币种两边相同不意味着售价应相同。

每条路径追踪 validation → estimate → reserve → usage/时长/张数 → settle/refund → log。复用 MaxImageN、MaxTaskDurationSeconds、checked quota helpers 与 AddOtherRatio；同样约束 metadata、multipart、音频头和上游 usage。禁止裸 int 转换和复制旧 fork 的无界算术。

Wan 使用经验证的视频轨实际时长，不用音轨尾长；保留下载大小、超时、重定向、SSRF 和临时文件清理限制。MiniMax 文件 ID 用字符串对客户端输出以保精度，向上游按其协议编码；文件/声音 ID 必须与上传/克隆账号一致。

Seedream 分层仅针对确认的模型和布尔 true 请求；在 model-price 路径依源语义使用实际生成张数 × 0.5，并把倍数作为目标定价审阅项。0、缺失或异常 usage 不可静默标记为已核准实际成本计费；异常路径须有明确错误/记录与账务行为。预扣要覆盖受支持最大输出，结算退款差额。表达式路径与按张路径不能同时乘数量。

## 配置、迁移和回滚

先生成不含密钥的渠道/价格差分草案：源 ID、目标类型、默认地址解析、模型映射、协议设置、分组、优先级、价格依据、启用状态。复用供应商凭据的授权/权限由配置阶段确认；不把凭据写进仓库。

先本地隔离数据验证。后续获准生产配置后，新条目默认禁用；禁用验证只证明不会被路由。真实请求另用临时启用的受限测试分组/令牌，测试后恢复约定状态。测试分组不加入普通 auto 选择集。

数据库新增/迁移需支持 SQLite/MySQL/PostgreSQL；兼容旧任务快照。发布仅更新 FlowAPI 应用，保留 DB/Redis/卷和回滚镜像。回滚先禁用新渠道，允许已接受任务继续安全轮询/结算；不得恢复旧 DB 快照抹掉发布后账单或进行中的任务。若旧版不认识新任务类型，必须先排空或保留兼容轮询能力再回滚应用。
