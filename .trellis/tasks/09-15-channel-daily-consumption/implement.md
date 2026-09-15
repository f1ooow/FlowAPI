# Implement: 渠道今日消耗统计

## 0. 前置

- [x] 用户确认统计正向消费总额，退款不冲减
- [x] 完成 PRD 收敛，提交最终计划供用户审核
- [x] 读取 `trellis-before-dev` 与相关 backend / frontend / shared 规范
- [x] 确认工作区已有变更，仅编辑本任务文件

## 1. 后端统计

- [x] 为 `model.Channel` 增加不持久、可空的 `TodayUsedQuota`
- [x] 在 `model/log.go` 增加按渠道 ID 批量聚合指定时间范围 quota 的查询
- [x] 只聚合 `LogTypeConsume`，退款日志不参与计算
- [x] 对渠道 ID 去重并按安全批量查询，不产生 N+1
- [x] 在 `controller/channel.go` 为 `GetAllChannels` 和 `SearchChannels` 回填统计
- [x] 明确处理日志关闭与查询失败的 `null` 降级

聚焦验证：

```bash
go test ./model ./controller -count=1
go build ./...
```

## 2. 前端数据与聚合

- [x] `web/src/features/channels/types.ts` 接入可空字段
- [x] `aggregateChannelsByTag` 对可用值求和，对不可用值传播 `null`
- [x] 补充保护标签聚合三态语义的确定性测试

## 3. 表格与移动卡片

- [x] 新增「Today's consumption」表格列，位于「Used / Remaining」之后
- [x] 复用 quota / currency 格式化、紧凑显示与精确值 tooltip
- [x] 接入敏感数据遮罩，区分 `0` 和 `null`
- [x] 移动端渠道卡片增加对应指标并保持稳定布局
- [x] 按 `i18n-translate` 技能复用当前语言包已有的 `Consumed today` 键

聚焦验证：

```bash
cd web
bun run test -- <affected channel tests>
bun run typecheck
bun run lint
```

## 4. 本地界面验收

- [x] 启动本地开发服务器，不使用任何生产地址
- [x] 在桌面与移动视口验证表格 / 卡片布局、遮罩、零值和不可用值
- [x] 验证标签模式的聚合值
- [x] 检查没有文字截断、重叠或横向溢出

## 5. 质量门

```bash
go test ./model ./controller -count=1
go build ./...
cd web && bun run typecheck && bun run lint && bun run build
```

- [x] 使用 `trellis-check` 进行规范、数据流、测试和一致性检查
- [x] 确认未连接或修改生产环境
- [x] 仅显式处理本任务文件，保留其他未提交变更

最终验证：后端聚焦测试、`go build ./...`、`go vet ./model ./controller`、前端 5 个相关测试文件 26 项、`bun run typecheck`、定向 lint / format、`bun run build` 与 `git diff --check` 均通过。全仓前端 lint 仍受本任务外既有错误阻塞。界面在 1920、1440、900 和 390 像素宽度完成本地验收；未访问生产环境。

## 6. 回滚形状

本功能无迁移。回滚时移除 API item 的临时字段、聚合查询与前端列即可，不需要恢复数据。

## 7. 待部署

- [ ] 等待用户明确下令后再部署到 HK，当前阶段不连接或修改生产环境
- [ ] 部署前备份并核对 `/opt/flowapi/.env` 与 Compose 配置
- [ ] 在生产 `.env` 设置 `MAX_REQUEST_BODY_MB=256`
- [ ] 随本次发布重建应用容器，并验证环境变量生效、容器健康及公网接口正常
