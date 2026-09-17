# 后端开发规范

> API 模式、错误处理、代码质量的项目规范。

## 规范索引

| 指南 | 说明 | 状态 |
|------|------|------|
| [api-patterns.md](./api-patterns.md) | API 设计模式、路由约定 | 参考模板 |
| [error-handling.md](./error-handling.md) | 错误类型、处理策略 | 参考模板 |
| [quality.md](./quality.md) | 后端代码质量标准 | 参考模板 |
| [billing-ratio-routing.md](./billing-ratio-routing.md) | 渠道选择相关倍率、预扣和异步快照合同 | 项目合同 |
| [streaming-provider-hedge.md](./streaming-provider-hedge.md) | 超时竞速、输家计费开关、请求隔离和统计合同 | 项目合同 |
| [global-passthrough.md](./global-passthrough.md) | 全局请求体/请求头透传、覆盖优先级与渠道继承 | 项目合同 |
| [channel-list-derived-metrics.md](./channel-list-derived-metrics.md) | 渠道列表从独立日志库补充派生指标的合同 | 项目合同 |

## 如何填写

同前端规范，bootstrap 任务会从代码中提取，也可手动更新。
