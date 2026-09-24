# 图片执行清单

1. [ ] 固定四模型的 endpoint/字段/usage fixtures，追踪生成和支持的编辑/多参考图路径。
2. [ ] 补模型注册和参数转换，Pro 不支持字段由模型协议规则省略，保留其他模型合法参数。
3. [ ] 增加限定分层数量/预扣/结算合同；完成 n、actual images 与 expression 优先级样例。
4. [ ] 普通/分层/异常 usage 回归及现有 GPT/Gemini JSON/multipart 回归。
5. [ ] 向 pricing 交付每模型单位、样例账单、显示字段；父任务整合共享文件与配置草案。

检查使用 `go test ./relay/channel/openai/... ./relay/channel/volcengine/... ./relay/helper/... ./relay/... ./service/...` 中实际受影响路径。DTO 改动涉及 relaykit 时执行独立模块构建。前端由 pricing 统一验证，本任务不重复改七套 locale。

代码完成、真实出图、生产启用为三个独立状态；缺真实结果不能报告模型已线上可用。
