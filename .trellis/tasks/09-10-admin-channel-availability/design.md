# Design: 管理员渠道可用性监控

## 1. 总体数据流

```
每次渠道尝试(attempt) ──打点──> 内存热桶(channel 维度) ──flush──> channel_metrics 表(主库)
                                                                        │
GET /api/channel-monitoring/summary (AdminAuth) ── 按 range 选 step 聚合 ─┘
  └─> DTO ─> 前端管理员页面
```

**为什么不复用 `perf_metrics`**：见 prd.md R1。三条硬伤——GORM 不改已有唯一索引导致老库 MySQL 静默串数据、失败打点在重试循环之外使 failover 中的故障渠道不可见、`/api/perf-metrics` 是准公开链路。

## 2. 存储

### 2.1 新表 `channel_metrics`

`model/channel_metric.go`：

```go
type ChannelMetric struct {
    Id             int   `gorm:"primaryKey"`
    ChannelId      int   `gorm:"uniqueIndex:idx_channel_metric_channel_bucket,priority:1"`
    BucketTs       int64 `gorm:"uniqueIndex:idx_channel_metric_channel_bucket,priority:2;index:idx_channel_metric_bucket_ts"`
    AttemptCount   int64 `gorm:"default:0"`
    SuccessCount   int64 `gorm:"default:0"`
    TotalLatencyMs int64 `gorm:"default:0"`
}
```

- 落**主库 `DB`**，加入 `model/main.go` 的 AutoMigrate 列表（含 `migrateDBFast()` 的并行列表）
- 索引从第一天就是最终形态，不存在老库索引变更问题
- 不带 model / group 维度：页面按渠道逐行展示，行数与模型数/分组数解耦
- upsert 用 `clause.OnConflict` + `gorm.Expr("col + ?")`，照抄 `model/perf_metric.go:29-49` 的三库通用写法
- 清理照抄 `model/perf_metric.go:118-123` `DeletePerfMetricsBefore` 的形状

**基数**：每桶行数 = 有流量的渠道数 ≤ 渠道总数。100 渠道 7d@5min ≤ 20.2 万行（约 12–50 MB），上界确定且运营可控。

### 2.2 保留期（硬性）

复用 `perf_metrics_setting.RetentionDays`（任务 A 已把默认值从 `0` 改为 `7`）。**不得**新建一个默认为 0 的配置——`perf_metrics` 当前永不清理正是因为 `flush.go:70-72` 对 0 直接 return。

## 3. 采集

### 3.1 运行时：复用 `pkg/perf_metrics` 的 flush 生命周期

在 `pkg/perf_metrics` 内新增 channel 维度的热桶（`sync.Map[channelBucketKey]*atomicChannelBucket`），**共用现有的 `flushLoop`**，不新起 goroutine。

理由：两者的 flush 间隔、桶宽、保留期语义完全一致，各自一个定时器只是重复的生命周期管理。热桶 map、表、查询都独立，不构成薄封装。

桶宽同样取 `perf_metrics_setting.GetBucketSeconds()`（任务 A 已默认 5min），分桶在 Go 侧算：`bucketStart(ts) = ts - ts%bucketSeconds`。

### 3.2 打点位置：attempt 级（本方案的核心价值）

**必须挂在单次渠道尝试上**。若沿用 `perf_metrics` 的请求级打点，典型 failover（渠道 A 500 → 重试渠道 B 成功）只会产生一条记在 B 的成功样本，**A 的失败完全不可见**，可用率永远接近 100%。

| 场景 | 挂载点（research 给出，实现时必须复核行号） |
|---|---|
| 文本 relay 成功 | `controller/relay.go` 的 `RouteAttemptSucceeded` 分支，紧邻 `service.RecordAutoBanSuccess(channel.Id)` |
| 文本 relay 失败 | `processChannelError` —— **所有失败 attempt 的唯一汇合点**，文本 relay 和 `RelayTask` 都调用它 |
| 任务类 | `RelayTask` 的重试循环与成功分支、`RelayMidjourney` |

**注意**：任务 A 已经改过 `controller/relay.go`（新增 `recordTaskRelaySample`、删除 `group_monitoring_probe` 消费点、内联 `recordResilientRouteErrorLog`），research 里的行号已失效，以实际代码为准。

### 3.3 两个必须处理的陷阱

1. **nil 指针**：`relayInfo.ChannelMeta` 是嵌入指针，`GenRelayInfo` **不初始化**它。当失败发生在选渠道之前（`getChannel` 返回错误直接 break），`ChannelMeta == nil`，读 `info.ChannelId` 会 panic。

   **策略**：尚未选中渠道的失败**跳过打点**。理由：这类失败（无可用渠道、鉴权失败、模型不存在）不归属任何具体渠道，计入任何一个渠道的可用率都是错误归因。仓库内其他读取处都有显式 nil 检查（`relay/common/override.go:437,444` 等），照此办理。

2. **探测流量排除**：`relayInfo.IsChannelTest` 为 true 时跳过打点，与现有 3 个 perf 打点点一致。（任务 A 已删除 `group_monitoring_probe` 标记，无需再判断它。）

### 3.4 口径

- `AttemptCount` 记的是**尝试次数**不是请求数：一个请求 failover 三次会产生 3 条 attempt 记录，分属不同渠道。这正是想要的语义。
- `TotalLatencyMs` 记单次 attempt 的耗时。
- 任务类同任务 A 的口径：**只记提交阶段**，不追踪异步执行结果。

## 4. 查询层

### 4.1 按 range 选 step，SQL 侧整数分桶

7d @ 5min × 100 渠道 = 20 万行，不能全量取回内存再聚合（这点与任务 A 不同，A 的窗口固定 24h 且维度小）。

| range | step |
|---|---|
| 15m | 5min（原始桶） |
| 1h | 5min |
| 6h | 15min |
| 24h | 30min |
| 7d | 2h |

SQL：`GROUP BY channel_id, (bucket_ts / step) * step`，方言分支照抄 `model/usedata_rankings.go:51-56`：

```go
if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
    return fmt.Sprintf("FLOOR(bucket_ts / %d) * %d", step, step)
}
return fmt.Sprintf("(bucket_ts / %d) * %d", step, step)
```

**与任务 A 的「禁止 SQL 侧时间分桶」不矛盾**：那条禁令针对的是对原始时间戳用 `strftime`/`DATE_FORMAT`/`date_trunc` 等**时间函数**分桶（四方言语义不一致）。这里 `bucket_ts` 已经是整数秒，做整数除法分桶是项目已有先例，只需处理 PG 整除的类型差异。

### 4.2 派生指标

同样**先累加计数器再算率**：

```
availability = Σsuccess_count / Σattempt_count
avg_latency  = Σtotal_latency_ms / Σattempt_count
error_rate   = 1 - availability
```

无 attempt 的渠道 → `has_data: false`，不返回 0%。

`state`（healthy / degraded / down / no-data）由后端算好，与任务 A 保持一致的阈值口径。

### 4.3 渠道名

`channel_metrics` 不存渠道名。查询后单独查 `channels` 表补名（`model/log.go:628-659` 已有「单独查 channels 再在 Go 里 map 回去」的先例，避免跨库 JOIN）。已删除的渠道保留 id 展示。

## 5. API

```
GET /api/channel-monitoring/summary?range=24h
```

- 挂 `middleware.AdminAuth()`（渠道相关接口的统一口径，见 `router/channel-router.go:21`）。**不用** `UserAuth`，也不引入任何公开入口。
- `range` 白名单：`15m` / `1h` / `6h` / `24h` / `7d`，非法值返回 400。

```jsonc
{
  "range": "24h",
  "step_minutes": 30,
  "overall": { "availability_rate": 97.46, "avg_latency_ms": 68940, "error_rate": 2.54 },
  "channels": [{
    "channel_id": 12,
    "channel_name": "FAST CODEX",
    "has_data": true,
    "state": "healthy",
    "availability_rate": 100.0,
    "attempt_count": 90,
    "buckets": [{"ts": 0, "attempt_count": 0, "success_count": 0, "state": "no-data"}]
  }]
}
```

## 6. 前端

新页面 `web/src/features/channel-monitoring/`，路由 `web/src/routes/_authenticated/channel-monitoring/`。

- **路由守卫**：`beforeLoad` 判 `!auth.user || auth.user.role < ROLE.ADMIN` → `throw redirect({to: '/403'})`，照抄 `web/src/routes/_authenticated/channels/index.tsx:36-44`
- **侧边栏**：放进 `id: 'admin'` 的 nav group（`use-sidebar-data.ts`），由 `use-sidebar-view.ts:53-63` 按角色过滤
- 顶部三张汇总卡：整体可用性、平均延迟、错误率
- 主体：按渠道逐行的时间线 + 行尾可用率与请求数；无数据渠道显示「暂无数据 / 无请求」
- range 切换：15m / 1h / 6h / 24h / 7d
- **不做**「端点健康」tab，**不做**「活跃探测 / 负载」这类依赖探针的卡片
- 文案走 i18n

## 7. 不能碰的东西

- `controller/channel-test.go` 与渠道测试行为
- `perf_metrics` 的表结构（不加 `channel_id`）
- 任务 A 已落地的分组监控代码（除非发现真实冲突）

## 8. 与任务 A 的关系

A 已完成并改动了 `controller/relay.go`。B 在同一文件继续加 attempt 级打点，**必须先复核 A 改动后的实际代码结构**，不要照搬 research 里的旧行号。

## 9. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| `ChannelMeta` nil panic | 选渠道前失败时崩溃 | 显式 nil 检查，跳过打点（§3.3） |
| attempt 级打点遗漏某条失败路径 | 可用率虚高 | 优先挂在 `processChannelError` 这个唯一汇合点，而非各分支散落 |
| 7d 查询数据量 | 慢查询 | SQL 侧按 step 聚合，不全量取回；`bucket_ts` 有索引 |
| 渠道数增长 | 页面行数过多 | 行数 = 渠道数，运营可控；必要时前端分页/筛选 |
| PG 整除类型差异 | 分桶结果错误 | 用 `FLOOR` 分支，照抄既有先例 |
