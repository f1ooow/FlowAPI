# Implement: 管理员渠道可用性监控

## 阶段 0 — 前置

- [ ] 任务 A（`09-10-group-monitor-real-traffic`）已完成并改动过 `controller/relay.go`。**先读实际代码**，不要照搬 research/design 里的旧行号。
- [ ] 确认 A 已把 `perf_metrics_setting.RetentionDays` 默认值改为 `7`、`BucketTime` 改为 `"5min"`
- [ ] 确认 `group_monitoring_probe` 标记已被 A 删除（B 只需判 `IsChannelTest`）
- [ ] 阅读 `.trellis/spec/backend/index.md`、`AGENTS.md`

## 阶段 1 — 存储层

- [ ] 新建 `model/channel_metric.go`：`ChannelMetric{Id, ChannelId, BucketTs, AttemptCount, SuccessCount, TotalLatencyMs}`，唯一索引 `(channel_id, bucket_ts)` + `bucket_ts` 普通索引
- [ ] upsert 用 `clause.OnConflict` + `gorm.Expr("col + ?")`，照抄 `model/perf_metric.go:29-49`
- [ ] 清理函数照抄 `model/perf_metric.go:118-123` 的形状
- [ ] 加入 `model/main.go` 的 AutoMigrate 列表（**含 `migrateDBFast()` 的并行列表**，两处都要）
- [ ] 不用 `gorm:"default:true"`；计数列用 `default:0`

验证：`go build ./... && go test ./model/... -count=1`

## 阶段 2 — 采集层

- [ ] 在 `pkg/perf_metrics` 内新增 channel 维度热桶（`sync.Map[channelBucketKey]*atomicChannelBucket`），**共用现有 `flushLoop`**，不新起 goroutine
- [ ] 桶宽取 `perf_metrics_setting.GetBucketSeconds()`，分桶在 Go 侧 `ts - ts%bucketSeconds`
- [ ] flush 写入 `channel_metrics`；失败回滚照抄 `pkg/perf_metrics/flush.go:53-57` 把 counters 加回内存桶
- [ ] 清理复用 `RetentionDays`

**打点（核心）** —— 挂在 attempt 级，不是请求级：
- [ ] 文本 relay 成功：`RouteAttemptSucceeded` 分支，紧邻 `service.RecordAutoBanSuccess(channel.Id)`
- [ ] 文本 relay 失败：`processChannelError` —— 所有失败 attempt 的唯一汇合点，优先挂这里而非各分支散落
- [ ] 任务类：`RelayTask` 重试循环与成功分支、`RelayMidjourney`
- [ ] **nil 检查**：`relayInfo.ChannelMeta` 为 nil（选渠道前就失败）时**跳过打点**，不要读 `info.ChannelId`，否则 panic
- [ ] `IsChannelTest` 为 true 时跳过打点
- [ ] 口径注释写明：`AttemptCount` 是尝试次数不是请求数；任务类只记提交阶段

测试：
- [ ] failover 场景：渠道 A 失败 → 渠道 B 成功，断言 A 记失败、B 记成功（**这是本任务最重要的测试**）
- [ ] 选渠道前失败不 panic 且不产生打点
- [ ] `IsChannelTest` 流量不被记录
- [ ] 桶键归并正确

验证：`go build ./... && go test ./pkg/perf_metrics/... ./controller/... -count=1`

## 阶段 3 — 查询层

- [ ] 新增按 `(channel_id, step 分桶)` 聚合的查询
- [ ] range → step 映射：`15m→5min`、`1h→5min`、`6h→15min`、`24h→30min`、`7d→2h`
- [ ] SQL 分桶用整数除法 + PG 分支：
  ```go
  if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
      return fmt.Sprintf("FLOOR(bucket_ts / %d) * %d", step, step)
  }
  return fmt.Sprintf("(bucket_ts / %d) * %d", step, step)
  ```
  照抄 `model/usedata_rankings.go:51-56` 的先例
- [ ] 派生指标**先累加计数器再算率**：`availability = Σsuccess/Σattempt`，`error_rate = 1 - availability`
- [ ] 无 attempt 的渠道 `has_data: false`，不返回 0%
- [ ] `state`（healthy/degraded/down/no-data）后端算好，阈值与任务 A 一致
- [ ] 渠道名单独查 `channels` 表在 Go 里 map 回去（**不要跨库 JOIN**），已删除渠道保留 id 展示

测试：range→step 映射、率为累加而非平均、无数据语义、PG 分支表达式正确性。

验证：`go build ./... && go test ./model/... -count=1`

## 阶段 4 — API

- [ ] `GET /api/channel-monitoring/summary?range=24h`，挂 `middleware.AdminAuth()`
- [ ] `range` 白名单校验，非法值 400
- [ ] DTO 按 design §5
- [ ] **不引入任何公开/匿名入口**

验证：`go build ./... && go test ./controller/... ./router/... -count=1`

## 阶段 5 — 前端

- [ ] 新建 `web/src/features/channel-monitoring/`
- [ ] 路由 `web/src/routes/_authenticated/channel-monitoring/`，`beforeLoad` 判 `role < ROLE.ADMIN` → `redirect('/403')`，照抄 `routes/_authenticated/channels/index.tsx:36-44`
- [ ] 侧边栏入口放进 `id: 'admin'` nav group（`use-sidebar-data.ts`）
- [ ] 顶部三张汇总卡：整体可用性、平均延迟、错误率
- [ ] 主体：按渠道逐行时间线 + 行尾可用率与请求数；无数据渠道显示「暂无数据」
- [ ] range 切换 15m / 1h / 6h / 24h / 7d
- [ ] **不做**端点健康 tab、**不做**活跃探测/负载卡片
- [ ] 文案进 i18n（扁平 JSON，key 用英文原文）

验证：
```bash
cd web && bun run typecheck && bun run lint && bun run build
```

## 阶段 6 — 全量验证

```bash
go build ./... && go vet ./... && go test ./... -count=1
cd web && bun run typecheck && bun run build
```
若触及 `relaykit/`：`cd relaykit && GOWORK=off go build ./...`

三库：新查询至少在 SQLite 实跑；PG 的 `FLOOR` 分支与 MySQL 整除走查确认。

- [ ] 非管理员访问：后端鉴权失败、前端跳 403
- [ ] 逐条对照 prd.md 验收清单

## 阶段 7 — 收口

- [ ] 更新 `task.json.notes`、写 journal
- [ ] 更新父任务 `09-10-group-monitor-log-aggregation` 的集成验收清单

## Commit 纪律

- **禁止** `git add -A` / `git add .`，显式列文件名
- **禁止** `git restore` / `git checkout` / `git clean`
- scope 外失败原样汇报
