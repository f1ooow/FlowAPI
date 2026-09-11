# Research: 分组监控改为「聚合真实调用日志」的可行性

- **Query**: 评估把分组监控数据源从主动探测改为聚合真实调用日志的可行性（日志表结构 / 日志库分离 / 已有聚合设施 / 定时任务机制 / 三库兼容）
- **Scope**: internal
- **Date**: 2026-09-10

---

## 0. 结论速览（先看这个）

| 问题 | 结论 | 证据 |
|---|---|---|
| 在线率能否从 `logs` 表算出来 | **默认不能**。错误请求默认**不写** Log 表（`ERROR_LOG_ENABLED` 默认 `false`） | `common/init.go:196`、`controller/relay.go:524` |
| TTFT 是否已有列 | **`logs` 表没有 TTFT 列**。但 TTFT **已经以毫秒精度写进 `logs.other` JSON 的 `frt` 字段**（仅消费日志，错误日志没有） | `service/log_info_generate.go:104`；`model/log.go:59-81` 无该列 |
| 是否有可复用的聚合设施 | **有，而且几乎是现成的**：`perf_metrics` 表 + `pkg/perf_metrics` 内存分桶 + flush 定时任务，已含 request/success/latency/**ttft**/tokens/generation，按 `(model, group, bucket_ts)` 聚合，且**已支持 5 分钟桶** | `model/perf_metric.go`、`pkg/perf_metrics/*`、`setting/perf_metrics_setting/config.go:28-38` |

> 最重要的一点：**本项目已经存在一套「按 (模型, 分组, 时间桶) 聚合真实调用」的运行时设施**（`perf_metrics`），它记录的正是分组监控想要的 4 个指标：请求数、成功数、总耗时、TTFT。它**不是**从 `logs` 表 SQL 聚合来的，而是在 relay 结束时打点到内存 + Redis，再定期 flush 进主库聚合表。

---

## 1. 日志表结构（`logs`）

### 1.1 完整字段清单

`model/log.go:59-81`：

| 字段 | Go 类型 | 列名 | 索引 | 备注 |
|---|---|---|---|---|
| `Id` | `int` | `id` | `idx_created_at_id`(p2), `idx_user_id_id`(p2) | |
| `UserId` | `int` | `user_id` | `index`, `idx_user_id_id`(p1) | |
| `CreatedAt` | `int64` | `created_at` | `bigint`; `idx_created_at_id`(p1), `idx_created_at_type` | **unix 秒**，不是 `time.Time` |
| `Type` | `int` | `type` | `idx_created_at_type` | 见 1.4 |
| `Content` | `string` | `content` | — | |
| `Username` | `string` | `username` | `index`, `index_username_model_name`(p2) | |
| `TokenName` | `string` | `token_name` | `index` | |
| `ModelName` | `string` | `model_name` | `index`, `index_username_model_name`(p1) | |
| `Quota` | `int` | `quota` | — | |
| `PromptTokens` | `int` | `prompt_tokens` | — | |
| `CompletionTokens` | `int` | `completion_tokens` | — | |
| `UseTime` | `int` | `use_time` | — | **单位是秒**，见 1.3 |
| `IsStream` | `bool` | `is_stream` | — | |
| `ChannelId` | `int` | `channel` (json) / `channel_id` (列) | `index` | |
| `ChannelName` | `string` | — | `gorm:"->"` | **只读虚拟字段**，运行期从 channel 缓存/表 join 填充（`model/log.go:628-659`），DB 里没有这列 |
| `TokenId` | `int` | `token_id` | `index` | |
| `Group` | `string` | `group` | `index` | 见 1.2 |
| `Ip` | `string` | `ip` | `index` | 受用户设置 `RecordIpLog` 控制 |
| `RequestId` | `string` | `request_id` | `idx_logs_request_id` | varchar(64) |
| `UpstreamRequestId` | `string` | `upstream_request_id` | `idx_logs_upstream_request_id` | varchar(128) |
| `Other` | `string` | `other` | — | **JSON 字符串**，装了大量结构化信息 |

**没有的列（必须记住）**：`channel_type`（供应商类型）、`status_code`、`ttft` / `first_token_time` / `frt`、`error_code`。这些要么根本没有，要么只存在于 `other` JSON 里。

### 1.2 分组字段：用哪个？

只有一个 `Group` 列。它的值来源是 **`relayInfo.UsingGroup`**，即「本次请求实际使用的分组」（auto 跨分组重试时会变动），不是用户的静态分组：

- `service/text_quota.go:540` — `Group: relayInfo.UsingGroup`
- `service/quota.go:248`、`service/quota.go:376` — 同上
- `relay/common/relay_info.go:89` — `UsingGroup string // 使用的分组，当auto跨分组重试时，会变动`
- `relay/common/relay_info.go:516` — 从 `constant.ContextKeyUsingGroup` 读取

错误日志的 group 来源不同：`controller/relay.go:542` 用的是 `c.GetString("group")`（用户分组），**与消费日志的 `UsingGroup` 语义不完全一致**，跨分组 auto 重试场景下两者会不同。

**公开状态页按分组聚合应使用 `logs.group`（= UsingGroup）**，这也是现有 `perf_metrics.group` 的取值来源（`pkg/perf_metrics/metrics.go:51`）。

查询时列名必须用 `logGroupCol`（`group` 是保留字）：`model/log.go:602,694,754,755`，定义在 `model/main.go:43-50`。

### 1.3 耗时字段

- **总耗时**：`logs.use_time`，**单位秒（整数）**。
  - `service/text_quota.go:235` — `UseTimeSeconds: time.Now().Unix() - relayInfo.StartTime.Unix()`
  - `service/text_quota.go:538` — `UseTimeSeconds: int(summary.UseTimeSeconds)`
  - `controller/relay.go:573` — 错误日志 `useTimeSeconds := int(time.Since(startTime).Seconds())`
  - → **秒级精度对延迟曲线基本不可用**（大量请求会落到 0s / 1s）。

- **TTFT**：`logs` 表**没有专用列**。但**已经存在于 `other` JSON**：
  - `service/log_info_generate.go:104`：
    ```go
    other["frt"] = float64(relayInfo.FirstResponseTime.UnixMilli() - relayInfo.StartTime.UnixMilli())
    ```
  - 该行在 `GenerateTextOtherInfo` 中，被 text / claude / audio / wss 各路径复用（`log_info_generate.go:292,304,321`），所以**所有走文本类 relay 的消费日志都带 `other.frt`（毫秒）**。
  - **注意**：非流式或从未触发首包时 `FirstResponseTime` 会是 `StartTime - 1s`（`relay/common/relay_info.go:536`），此时 `frt` 是**负数（约 -1000）**。判定有效性的官方方式是 `info.HasSendResponse()`（`relay/common/relay_info.go:856-858`）+ `info.IsStream`，见 `pkg/perf_metrics/metrics.go:32`。
  - **错误日志（`LogTypeError`）不写 `frt`** —— `controller/relay.go:536-575` 的 `other` 是手工构造的，没有 frt。

- **首包时间在 relay 层的计算位置**：
  - `relay/common/relay_info.go:849-853` `SetFirstResponseTime()`（幂等，只记第一次）
  - 调用点：`relay/helper/stream_scanner.go:346`（通用 SSE 扫描器）、`relay/channel/openai/relay_image.go:269`、`relay/channel/openai/relay_realtime.go:117`、`relay/channel/aws/relay-aws.go:296`、`relay/channel/cohere/relay-cohere.go:120`、`relay/channel/cloudflare/relay_cloudflare.go:68`

- **若真要给 `logs` 加 TTFT 列**，需要动的地方：
  1. `model/log.go:59-81` 结构体加字段
  2. `model/log.go:397-427`（`RecordErrorLog`）与 `model/log.go:461-486`（`RecordConsumeLog`）赋值
  3. `model/log.go:429-442` `RecordConsumeLogParams` 加参数
  4. 所有 `RecordConsumeLog` 调用点（9 处，见 `grep RecordConsumeLog(`：`service/quota.go:237,365`、`service/text_quota.go:529`、`service/task_billing.go:53`、`service/violation_fee.go:152`、`relay/mjproxy_handler.go:276,641`、`controller/channel-test.go:501`）
  5. **`model/main.go:486-513` 的 ClickHouse 建表 DDL 是手写的**，必须同步加列，且已有 CH 表不会自动 migrate（`migrateClickHouseLogDB` 只 `CREATE TABLE IF NOT EXISTS`）
  6. `docs/openapi/api.json` + 前端日志表格

  → **成本不低，且 ClickHouse 存量表无迁移路径。而 TTFT 其实已经在 `other.frt` 和 `perf_metrics.ttft_sum_ms` 两处有了。**

### 1.4 成功/失败标记与「在线率」

日志类型常量（`model/log.go:84-93`，注释明确 "don't use iota, avoid change log type value"）：

```go
LogTypeUnknown = 0
LogTypeTopup   = 1
LogTypeConsume = 2   // 成功计费请求
LogTypeManage  = 3
LogTypeSystem  = 4
LogTypeError   = 5   // 失败请求
LogTypeRefund  = 6
LogTypeLogin   = 7
```

- 区分成功/失败的唯一列级手段是 `type`：`2` = 成功消费，`5` = 错误。
- **状态码**：没有列。错误日志把它塞进 `other`（`controller/relay.go:550` `other["status_code"]`），同时还有 `other["error_type"]`、`other["error_code"]`、`other["channel_type"]`、`other["channel_name"]`。
- `is_stream` 有列，可用于区分流式。

**关键风险 —— 错误请求默认不入库：**

```go
// controller/relay.go:524
if constant.ErrorLogEnabled && types.IsRecordErrorLog(err) && !c.GetBool("resilient_route_started") {
    recordErrorLog(c, err)
}
```

```go
// common/init.go:196
constant.ErrorLogEnabled = GetEnvOrDefaultBool("ERROR_LOG_ENABLED", false)
```

- **`ERROR_LOG_ENABLED` 默认 `false`** → 默认部署下 `logs` 表里**只有成功请求**，分母缺失，**在线率无法计算**。
- 即使打开，还有额外过滤：
  - `types.IsRecordErrorLog(err)`（`relaykit/types/error.go:408`）默认 true，但一批计费/额度错误显式关闭了（`service/billing_session.go:202,226,230,264,295,386,392` 用 `ErrOptionWithNoRecordErrorLog()`）——这些是「用户余额不足」类，本来也不该算上游故障，某种意义上是对的。
  - `resilient_route_started` 时走 `recordResilientRouteErrorLog`（`controller/relay.go:529-534`），只在链路彻底耗尽后落一条终态错误日志（`controller/relay.go:537` 注释）。
- 另外 `common.LogConsumeEnabled` 默认 `true`（`common/constants.go:89`），但**可被关闭**；关闭后 `RecordConsumeLog` 直接 return（`model/log.go:445-447`），分子也没了。

**对比：`perf_metrics` 不受这两个开关影响**，失败在 `controller/relay.go:377-381` 打点、成功在 `service/quota.go:379-383` 与 `service/text_quota.go:543-547` 打点，只受 `perf_metrics_setting.Enabled`（默认 `true`）控制。

### 1.5 探测流量的标记

现有主动探测请求会写日志，并带标记：
- `controller/group_monitoring.go:447` — `c.Set("group_monitoring_probe", true)`
- `service/log_info_generate.go:95-97` — 消费日志写 `other["group_monitoring_probe"] = true`
- `controller/relay.go:554-556` — 错误日志同样标记
- `controller/group_monitoring.go:466` — 同时 `c.Set("is_channel_test", true)`，因此 **探测流量被 perf_metrics 排除**（`relayInfo.IsChannelTest` 判断）

→ 若改为日志聚合，可用这个标记把探测流量从「真实调用」里剔除，反之亦然。

---

## 2. 日志库分离与新表落库位置

### 2.1 日志库可以是独立库，甚至是 ClickHouse

- `model/main.go:215-254` `InitLogDB()`：`LOG_SQL_DSN` 为空时 `LOG_DB = DB`（同库）；非空时开一个独立连接。
- **日志库支持 4 种类型**，比主库多一个 ClickHouse：`common/database.go:5-10`（`mysql` / `sqlite` / `postgres` / `clickhouse`）。
- 主库**明确禁止** ClickHouse：`model/main.go:131-133`。
- 独立列名变量：主库用 `commonGroupCol`/`commonKeyCol`，日志库用 `logGroupCol`/`logKeyCol`（`model/main.go:22-51`）。
- ClickHouse 分支还改变了排序、LIKE 转义、删除方式：`model/log.go:34-47`、`model/log.go:106-108`、`model/log.go:814-833`。

**→ 跨库风险**：任何「JOIN logs 与主库表」的写法都不可行（`model/log.go:648` 已经用了单独查 `DB.Table("channels")` 再在 Go 里 map 回去的做法来规避）。而 ClickHouse 上还多一层方言差异。

### 2.2 新的聚合表应该落主库

现有先例全部落**主库 `DB`**：

| 表 | 位置 | 落库 | 证据 |
|---|---|---|---|
| `perf_metrics` | `model/perf_metric.go` | 主库 `DB` | `model/perf_metric.go:33,53,83,101,122`；`model/main.go:337` 在 `migrateDB()` 的 AutoMigrate 列表里 |
| `group_monitoring_results` | `model/group_monitoring.go` | 主库 `DB` | `model/group_monitoring.go:19,27,33`；`model/main.go:338` |
| `quota_data`（数据看板） | `model/usedata.go` | 主库 `DB` | `model/usedata.go:110,120,128,144`；`model/main.go:324` |
| `logs` | `model/log.go` | **日志库 `LOG_DB`** | `model/log.go:103`、`model/main.go:448-453` |

**结论：分组监控聚合结果表应落主库 `DB`，加入 `model/main.go:310-344` 的 `AutoMigrate` 列表（以及 `migrateDBFast()` 的并行列表，见 `model/main.go:366+`）。** 这样也避免了 ClickHouse 方言问题。

---

## 3. 已有聚合设施（**复用重点**）

### 3.1 `perf_metrics` —— 现成的「按 (模型, 分组, 时间桶) 聚合」表

`model/perf_metric.go:11-27`：

```go
type PerfMetric struct {
    Id             int    `gorm:"primaryKey"`
    ModelName      string `gorm:"size:128;uniqueIndex:idx_perf_model_group_bucket,priority:1"`
    Group          string `gorm:"column:group;size:64;uniqueIndex:idx_perf_model_group_bucket,priority:2"`
    BucketTs       int64  `gorm:"uniqueIndex:idx_perf_model_group_bucket,priority:3;index:idx_perf_bucket_ts"`
    RequestCount   int64  `gorm:"default:0"`
    SuccessCount   int64  `gorm:"default:0"`
    TotalLatencyMs int64  `gorm:"default:0"`
    TtftSumMs      int64  `gorm:"default:0"`   // ← TTFT 已有
    TtftCount      int64  `gorm:"default:0"`
    OutputTokens   int64  `gorm:"default:0"`
    GenerationMs   int64  `gorm:"default:0"`
}
```

- 表名 `perf_metrics`（`model/perf_metric.go:25-27`）
- **原子累加 upsert**：`model/perf_metric.go:29-49` 用 `clause.OnConflict{Columns: [model_name, group, bucket_ts], DoUpdates: clause.Assignments(gorm.Expr("col + ?"))}` —— 这是 GORM 层写法，三库通用。
- 查询：`GetPerfMetrics`（`:51`）、`GetPerfMetricsSummaryAll`（`:81`）、`GetPerfMetricsSummaryBucketsAll`（`:99`）；分组过滤统一用 `commonGroupCol`（`:56,90,108`）。
- 清理：`DeletePerfMetricsBefore`（`:118`）。

**注意：它是按「模型 × 分组」聚合的，没有 channel_id 维度。**如果分组监控要下钻到渠道，需要新增维度或新表。

### 3.2 打点 → 内存桶 → flush 的完整链路

| 环节 | 位置 |
|---|---|
| 采样入口（从 RelayInfo 提取 latency/ttft/tps） | `pkg/perf_metrics/metrics.go:27-55` `RecordRelaySample` |
| 内存热桶 `sync.Map[bucketKey]*atomicBucket` | `pkg/perf_metrics/metrics.go:17`、`pkg/perf_metrics/types.go:79-153` |
| 时间分桶（**在 Go 里算，不在 SQL**） | `pkg/perf_metrics/metrics.go:266-272` `bucketStart(ts) = ts - ts%bucketSeconds` |
| Redis 旁路累加（多节点） | `pkg/perf_metrics/metrics.go:380-406` `recordRedis`（HIncrBy，key `perf:{model}:{group}:{bucketTs}`，TTL 1h） |
| 定时 flush + 过期清理 | `pkg/perf_metrics/flush.go:13-78` `flushLoop` / `flushCompletedBuckets` / `cleanupExpiredMetrics` |
| 启动 | `main.go:338` `perfmetrics.Init()` → `pkg/perf_metrics/metrics.go:23-25` `go flushLoop()` |
| 失败回滚（flush 出错把 counters 加回内存桶） | `pkg/perf_metrics/flush.go:53-57` |
| 打点调用点（成功） | `service/quota.go:379-383`、`service/text_quota.go:543-547` |
| 打点调用点（失败） | `controller/relay.go:377-381` |
| HTTP 出口 | `controller/perf_metrics.go`；路由 `router/api-router.go:45-50`（`/api/perf-metrics`，`HeaderNavModulePublicOrUserAuth("pricing")` → **可公开**） |

### 3.3 桶宽配置：**已支持 5 分钟**

`setting/perf_metrics_setting/config.go:27-38`：

```go
func GetBucketSeconds() int64 {
    switch perfMetricsSetting.BucketTime {
    case "minute": return 60
    case "5min":   return 300
    case "hour":   return 3600
    default:       return 3600
    }
}
```

默认值（`config.go:12-17`）：`Enabled=true, FlushInterval=5(分钟), BucketTime="hour", RetentionDays=0`（0 = 不清理，见 `flush.go:70-72`）。

### 3.4 `quota_data` —— 另一个「小时桶」先例

`model/usedata.go`：

- 表结构 `model/usedata.go:13-26`，主键维度 = `(user_id, username, model_name, created_at, use_group, token_id, channel_id, node_name)`
- **分桶同样在 Go 里做**：`model/usedata.go:80` `createdAt := params.CreatedAt - (params.CreatedAt % 3600)  // 只精确到小时`
- 内存缓存 + 定时保存：`model/usedata.go:41-49` `UpdateQuotaData()` 是一个 `for { ...; time.Sleep(DataExportInterval 分钟) }` goroutine
- 落库：先 `First` 查存在性，存在则 `Updates(gorm.Expr("count + ?"))`，否则 `Create`（`model/usedata.go:100-139`）—— 比 `perf_metrics` 的 OnConflict 写法旧
- 数据由 `RecordConsumeLog` 顺带触发（`model/log.go:491-504`），受 `common.DataExportEnabled`（默认 `true`，`common/constants.go:28`）控制

### 3.5 `group_monitoring_results` —— 当前主动探测的存储

`model/group_monitoring.go:3-16`：`GroupName / Success / LatencyMs / ChannelID / RequestID / RouteHistory(text) / ErrorCode / ErrorMessage / CheckedAt`，索引 `idx_group_monitoring_group_time (group_name, checked_at)`。

- **明细行存储，不是聚合表**：每次探测插一行（`controller/group_monitoring.go:495`）。
- 分桶完全在**读取时的 Go 代码**里做：`controller/group_monitoring.go:92-145` `buildGroupMonitoringSummaries`，固定 32 个桶，`bucketWidth = (end-start)/32`。
- 在线率：`AvailabilityRate = successCount / len(rows) * 100`（`:138`）；平均延迟 `totalLatency / len(rows)`（`:139`）；`Stale` 判定 `end - LastCheckedAt > intervalMinutes*120`（`:140`）。
- 保留期清理在探测任务末尾做：`controller/group_monitoring.go:528-531`。
- 公开出口：`router/api-router.go:24-27` `/api/group-monitoring/summary`，**注意它挂了 `middleware.UserAuth()`，当前不是完全匿名公开**（对比 perf-metrics 用的是 `HeaderNavModulePublicOrUserAuth`）。

---

## 4. 定时任务机制

项目里有**三种**周期任务写法，新功能应该用第一种。

### 4.1 标准做法：`SystemTask` 框架（推荐模板）

- 框架：`service/system_task.go`
  - 接口 `SystemTaskHandler`（`:34-37`）与 `ScheduledSystemTaskHandler`（`Enabled() / Interval() / NewPayload()`，`:42-47`）
  - 注册 `RegisterSystemTaskHandler`（`:57-64`）
  - 启动 `StartSystemTaskRunner()`（`:123-166`），15s ticker + wakeup channel
  - 调度器 `runSystemTaskScheduler()`（`:263-303`）：按 `Interval()` 与上次 `UpdatedAt` 判断是否到期
  - 认领 `runSystemTaskClaimPass()`（`:225-257`），每类型一个 goroutine
  - 租约心跳 `runWithLeaseHeartbeat()`（`:308-336`），TTL 60s，心跳失败 cancel ctx
  - 进度上报 `NewSystemTaskProgressReporter()`（`:478-509`）
- DB 层：`model/system_task.go`
  - `SystemTask`（`:29-42`）+ `SystemTaskLock`（`:44-50`）
  - 类型常量 `:19-24`，已含 `SystemTaskTypeGroupMonitoring = "group_monitoring"`
  - **多 master 去重**：`ActiveKey *string gorm:"uniqueIndex"`（`:34`）+ `acquireSystemTaskLock`（`:269-308`）
- 注册入口：`controller/system_task_handlers.go:20-26` `RegisterScheduledSystemTasks()`
- **最贴切的现成模板** —— 分组监控自己的 handler：`controller/system_task_handlers.go:28-50`
  ```go
  type groupMonitoringHandler struct{}
  func (groupMonitoringHandler) Type() string { return model.SystemTaskTypeGroupMonitoring }
  func (groupMonitoringHandler) Enabled() bool {
      s := operation_setting.GetGroupMonitoringSetting(); return s.Enabled && len(s.Targets) > 0
  }
  func (groupMonitoringHandler) Interval() time.Duration {
      return time.Duration(operation_setting.GetGroupMonitoringSetting().IntervalMinutes) * time.Minute
  }
  func (groupMonitoringHandler) NewPayload() any { return nil }
  func (groupMonitoringHandler) Run(ctx context.Context, task *model.SystemTask, runnerID string) { ... }
  ```
- **只在 master 节点跑**：`service/system_task.go:125-127` `if !common.IsMasterNode { return }`

### 4.2 裸 goroutine + Sleep（旧写法，仍在用）

- `pkg/perf_metrics/flush.go:13-24` `flushLoop()`：`for { time.Sleep(interval分钟); flushCompletedBuckets(); cleanupExpiredMetrics() }`，由 `main.go:338` 启动
- `model/usedata.go:41-49` `UpdateQuotaData()`：同样模式
- **这两个没有 master 节点判断，也没有跨节点锁** —— 多实例部署时 `perf_metrics` 靠 upsert 累加保证正确性，各节点各自 flush 自己的内存桶。

### 4.3 手动触发

`service.EnqueueSystemTask(taskType, payload)`（`service/system_task.go:201-220`）→ 复用同一套 handler。调用例：`controller/group_monitoring.go:255`、`controller/channel-test.go:1129`、`controller/channel_upstream_update.go:1114`。

---

## 5. 三库（+ClickHouse）兼容风险

### 5.1 「按 5 分钟分桶 GROUP BY」的方言差异 —— **本项目的答案是：不要写这种 SQL**

如果真在 SQL 里对 `logs.created_at`（unix 秒）做 5 分钟分桶：

| DB | 写法 |
|---|---|
| SQLite | `created_at - (created_at % 300)`（`%` 可用），或 `CAST(created_at/300 AS INTEGER)*300` |
| MySQL | `created_at - (created_at MOD 300)`；`/` 是浮点除法，必须 `DIV` 或 `FLOOR` |
| PostgreSQL | `created_at - MOD(created_at, 300)`；`%` 对 `bigint` 可用，但 `/` 是整数除法（与 MySQL 语义相反） |
| ClickHouse | `intDiv(created_at, 300)*300`，或 `toStartOfFiveMinute(toDateTime(created_at))` |

`/` 与 `%` 在四个引擎上语义不一致（MySQL 的 `/` 返回 DECIMAL、PG 的 `/` 是整除、CH 又是另一套），**这是真实的踩坑点**。

**实测：仓库里没有任何一处 SQL 侧时间分桶。** 全项目 grep `strftime|DATE_FORMAT|date_trunc|FROM_UNIXTIME|to_timestamp|% 3600` 在 `model/ controller/ service/` 下只命中两处：
- `model/db_time.go:14` — `SELECT strftime('%s','now')`，SQLite 专用分支（取服务器时间，与分桶无关）
- `model/usedata.go:80` — **Go 里** `createdAt - (createdAt % 3600)`

**项目约定（强）：时间分桶在 Go 代码里算好，写成 `bucket_ts int64` 列，SQL 只做 `WHERE bucket_ts BETWEEN ?` + `GROUP BY bucket_ts`。** 两个先例：
- `pkg/perf_metrics/metrics.go:266-272` `bucketStart()`
- `model/usedata.go:80`

这样 `GROUP BY model_name, bucket_ts`（`model/perf_metric.go:111`）在四个引擎上完全一致。

### 5.2 保留字与方言 helper 用法

- **`group` 是保留字**，必须用变量拼接，不能硬编码：
  - 主库表 → `commonGroupCol`（`model/main.go:33,38`；用例 `model/perf_metric.go:56,90,108`、`model/ability.go:46`）
  - 日志库表 → `logGroupCol`（`model/main.go:45,48`；用例 `model/log.go:602,694,754,755`）
  - **两者不可混用**：主库是 PG 而日志库是 MySQL（或反之）时值不同，见 `model/main.go:30-51`。
- 结构体侧则用 tag：`model/perf_metric.go:14` `gorm:"column:group;..."`，`clause.Column{Name: "group"}`（`model/perf_metric.go:36`）由 GORM 负责 quote。
- `commonTrueVal`/`commonFalseVal`（`model/main.go:35-36,40-41`）：PG 用 `true/false`，MySQL/SQLite 用 `1/0`。只在裸 SQL 里需要；GORM 传参（`Where("enabled = ?", true)`）不需要。
- `common.UsingMainDatabase(...)` / `common.UsingLogDatabase(...)`（`common/database.go:36-42`）用于必须分支的场景，例：`model/log.go:34`（ClickHouse LIKE 转义）、`model/log.go:814`（ClickHouse DELETE mutation）、`model/main.go:354`（SQLite 建表）。
- **禁止 `default:true` 布尔 tag**（AGENTS.md）：`perf_metrics` 全部用 `default:0` 数值，`GroupMonitoringResult.Success` 干脆不带 default（`model/group_monitoring.go:6`）—— 可直接照抄。
- 行锁用 `lockForUpdate(tx)`（`model/locking.go`）—— 聚合 upsert 用 `clause.OnConflict` 就不需要显式锁。

### 5.3 如果坚持从 `logs` 表 SQL 聚合，额外要处理的

1. `logs` 可能在**独立库甚至 ClickHouse**上 → 聚合结果要写主库，就变成「跨库读 + 跨库写」，无法放进一个事务，也无法 JOIN `channels` 拿渠道类型（`model/log.go:648` 已经是这个规避写法）。
2. `other` 是 JSON **字符串列**（不是 JSON 类型），要取 `frt` / `status_code` 就得 JSON 函数：MySQL `JSON_EXTRACT`、PG `::jsonb ->>`、SQLite `json_extract`（依赖 JSON1 扩展）、CH `JSONExtract*` —— **四套写法，AGENTS.md 明确禁止无 fallback 的 DB 专有 JSON 用法**。实践上只能把整列拉回 Go 里 `common.UnmarshalJsonStr` 解析，而这就是全表扫描。
3. `logs.use_time` 只有秒精度，延迟图会失真。
4. 错误请求默认不入库（见 1.4）。

---

## 6. 相关 spec 文档

| 路径 | 说明 |
|---|---|
| `.trellis/spec/backend/index.md` / `api-patterns.md` / `error-handling.md` / `quality.md` | 后端约定 |
| `.trellis/spec/guides/code-reuse-thinking-guide.md` | 复用判断（本任务高度相关：`perf_metrics` 是否可复用） |
| `.trellis/spec/guides/cross-layer-thinking-guide.md` | 跨层数据流 |
| `AGENTS.md`（仓库根） | 数据库三库兼容硬约束、JSON wrapper 约束、测试质量约束 |

---

## 7. Caveats / 未查证

- 未查证前端 `web/src/features/group-monitoring/` 与 `web/src/features/performance-metrics/` 的具体数据契约细节（只确认了文件位置：`web/src/features/group-monitoring/{api.ts,index.tsx}`、`web/src/features/performance-metrics/api.ts`、`web/src/routes/_authenticated/group-monitoring/index.tsx`）。
- 未查证任务类 relay（Midjourney / Suno / 视频）是否走 `perfmetrics.RecordRelaySample` —— 从 grep 看**没有**，`RecordRelaySample` 只有 3 个调用点（`controller/relay.go:379`、`service/quota.go:381`、`service/text_quota.go:545`）。任务类请求因此不在 `perf_metrics` 里。
- 未查证 `perf_metrics` 在多 master 节点下 `flushLoop` 无锁是否会造成重复计数 —— 从代码看每个节点只 flush**自己进程内**的 `hotBuckets`，OnConflict 累加，逻辑上不会重复；但 `recordRedis` 写的 Redis 计数**从未被 flush 消费**（`mergeRedisActiveBuckets` 在 `metrics.go:408` 定义，但全仓库没有调用点），属于 dead path，需要留意。
- `GetPerfMetricsSummaryAll`（`model/perf_metric.go:81`）在仓库中没有找到调用点（只有 `GetPerfMetricsSummaryBucketsAll` 被 `metrics.go:136` 使用），未深挖。
- 未实际运行任何 SQL 验证方言行为，5.1 表中的方言写法基于通用 SQL 知识，非本仓库实测。
