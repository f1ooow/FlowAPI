# 实施计划

1. 增加配置结构、校验、结果模型、迁移和聚合查询测试。
2. 增加 probe 上下文和分组探测执行路径，复用正常 Relay 的分组路由/重试及“模型测试”消费、错误日志，同时验证无额度、用量、性能指标、亲和缓存和渠道启停副作用。
3. 注册 system task handler，增加手动触发和定时去重测试。
4. 增加 UserAuth 摘要 API 与 RootAuth 管理 API，覆盖权限和脱敏契约。
5. 增加前端 API/types、独立受保护路由、主导航入口和分组状态卡。
6. 增加管理员配置与立即检测交互，补充 i18n。
7. 运行后端定向测试、Go build、前端测试、typecheck、涉及文件 lint 和生产 build。
8. 增量审计全部新改动，确认未覆盖工作区中既有用户修改。
9. 修复生产配置允许保存不可路由模型的问题：管理 API 返回分组模型映射，前端改为联动选择，保存与探测执行统一校验并兼容历史大小写差异。
10. 按用户澄清修正探测契约：每个分组增加请求格式与流式配置，复用原生渠道测试请求构造；Claude/Gemini 自动使用原生协议；系统设置改为直接配置面。
11. 修复全量手动检测的运行态契约：启动接口幂等返回 202，摘要暴露批任务状态，全量时所有卡片持续显示测试中；保留单分组独立忙碌状态，并统一错误消息出口。

## Risky Files

- `controller/channel-test.go`: 已有复杂请求转换与用户改动，采用小范围结构化修改。
- `controller/system_task_handlers.go`: 与现有调度共用注册点。
- `router/api-router.go`: 当前工作区已有用户修改，必须合并而非覆盖。
- `web/src/components/layout/*`: 导航影响全局布局，需做响应式回归检查。
- `web/src/features/system-settings/*`: 当前工作区已有用户修改，只增加独立 section/配置项。

## Validation

- `go test` 仅运行新增领域和直接受影响包的确定性测试。
- `go build ./...`
- `cd web && bun run test -- <affected tests>`
- `cd web && bun run typecheck`
- `cd web && bun run lint -- <affected files>`（按项目脚本实际参数调整）
- `cd web && bun run build`
