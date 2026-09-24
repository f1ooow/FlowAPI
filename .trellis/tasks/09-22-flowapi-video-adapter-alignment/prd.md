# 视频模型、协议与恢复机制

## 目标与范围

落实父任务 R1/R2：渠道 42/48/61/62/64/75，以及 49 的 Hailuo 视频。覆盖四种 Seedance、HappyHorse 1.0/1.1、Hailuo/H3、FastAI Seedance、Rolldek Wan/prime。Wan 仅接入 Rolldek，FastAI Wan 排除；FastAI 两个 time 路由因独立秒价未确认保持禁用，原生 Seedance 覆盖公开模型。逐模型目标范围以已审阅部署工件为准，coverage-matrix 同时保留源差异证据。

## 依赖

- 前置父任务 P0：渠道协议/端点 DTO、注册方式、轮询元数据、倍率快照接口。
- H3 文件输入端到端验收依赖 media 子任务的 MiniMax 上传和账号一致性；H3 请求/响应 fixtures 可先写。
- FastAI/Wan 的最终精确账单与价格页依赖 pricing；本子任务先提供 usage/时间/分辨率字段和数量范围。
- 父任务集成 Wan 新表、路由与共享 controller/settlement，避免并行覆盖。

## 验收

- [ ] 原生与中转各自明确 submit/fetch 路径；Zivv 61 的覆盖设置和默认空设置均测试。
- [ ] 每个协议有提交、查询、success/failure/UNKNOWN、provider error、媒体 URL 和原账号/模型绑定的确定性测试。
- [ ] Seedance token 与输入视频倍率、Hailuo 档位、H3 秒数/分辨率、FastAI time/token 不重复计量；数量在所有 payload 入口有界；FastAI 明确拒绝 Wan。
- [ ] Rolldek Wan 原生接口/协议开关、输入输出媒体、实际视频轨时长、恢复头与查询端点兼容 Keli 现有客户端合同。
- [ ] 同键重放、409 冲突、跨令牌隔离、断连/重启/未知接受和重复退款均有回归；未知状态不会触发第二次付费 POST。
- [ ] 与现有 Sora、Ali、Doubao、Hailuo 模型及旧任务快照兼容。

代码和本地契约测试已完成；配置/源码审查不是供应商实测。2026-09-25 用户授权 HK FlowAPI 上线，实际部署及供应商验收由父任务记录；KeliAPI 保持不变。
