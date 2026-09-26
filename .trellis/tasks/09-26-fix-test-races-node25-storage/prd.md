# 修复测试竞态与 Node 25 前端测试环境

## Goal

让现有后端 race 测试和前端 Vitest 在当前工具链下稳定通过，同时保留原有测试保护的行为。

## Background

- `go test -race ./relay/...` 报 Gin 全局 mode 的并发读写，测试中的 `gin.SetMode` 是写入方。
- `go test -race ./controller -count=1` 报任务提交测试 cleanup 恢复 `common.RedisEnabled`，与后台性能统计读取冲突；cleanup 还可能在统计任务结束前关闭测试数据库。
- `go test -race ./service -count=1` 报视频轮询测试在 `Eventually` 中读取 `*model.Task`，同时轮询协程通过 GORM 更新同一对象。
- Node 25 的 `globalThis.localStorage` 覆盖 jsdom Storage，默认 `bun run test` 有 8 个失败；提供有效 `--localstorage-file` 后 289 个测试全部通过，原先额外断言失败是环境污染的连带结果。

## Requirements

- 后端修复仅调整测试及其隔离/同步，不改变 relay、任务提交、视频轮询和性能统计的生产行为。
- 保留并行测试和视频轮询按渠道并发的测试目标，不以关闭并发来消除 race 报错。
- 前端使用 jsdom 的 Storage 作为测试环境的 `localStorage`，不要求开发者手动传入 Node 参数，不改组件实现。
- 保留已有测试断言和覆盖的用户/API 行为。

## Acceptance Criteria

- [x] `go test -race ./relay/...` 不再报告测试调用 `gin.SetMode` 导致的竞态。
- [x] `go test -race ./controller -count=1` 不再报告任务提交测试 cleanup 与异步统计任务的竞态。
- [x] `go test -race ./service -count=1` 不再报告轮询测试共享 `Task` 指针的竞态。
- [x] 在 Node 25 下直接运行 `cd web && bun run test`，全部既有测试通过，包括 API key 的 Restore global Auto 断言。
- [x] 所有改动通过适用的格式、类型和 lint 检查；生产代码保持不变。

## Out of Scope

- 重构生产环境的 Gin 模式、Redis 开关、性能统计调度或视频轮询机制。
- 扩展与这些失败无关的测试覆盖或修复其他独立失败。
