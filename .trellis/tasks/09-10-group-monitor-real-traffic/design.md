# Design: 分组监控改数据源与配置化

## 1. 总体数据流

```
真实用户请求
  └─ relay 各路径 ──打点──> pkg/perf_metrics 内存热桶 ──flush(5min)──> perf_metrics 表(主库)
                                                                            │
分组监控 API ─── 新查询(带 group + ttft 维度) ───────────────────────────────┘
  └─ Go 侧把底层 5min 桶合并成展示桶(5/15/30/60) ─> DTO ─> 前端渲染
```

**关键判断**：不新建表、不新建定时任务、不新建聚合管线。`perf_metrics` 已经是「按 (模型, 分组, 时间桶) 聚合真实调用」的现成设施，本任务主要是**补齐它的采集覆盖面**、**降低桶宽**，然后**新写一个读取查询**。

## 2. 采集层改动

### 2.1 补齐两处打点

现有 3 个调用点：`controller/relay.go:379`（失败）、`service/quota.go:381`、`service/text_quota.go:545`（成功）。

| 盲区 | 位置 | 处理 |
|---|---|---|
| Realtime WSS 成功 | `service/quota.go:150-251` `PostWssConsumeQuota` | 在函数末尾补 `perfmetrics.RecordRelaySample`，对齐 `service/quota.go:379-383` 的写法 |
| 任务类 relay | `controller.RelayTask` / `controller.RelayMidjourney` | 补打点。这两个入口不经过 `Relay()`，需确认能拿到 `RelayInfo` 或等价的 model/group/latency/success |

**注意**：任务类是异步提交-轮询模型，「成功」的语义与同步 relay 不同。design 决策：**只记录提交阶段的成败与耗时**（即 relay 层能直接观测到的部分），不追踪任务最终执行结果。理由是分组监控关心的是「网关到上游这条链路通不通」，任务执行失败属于业务结果而非可用性。实现时在代码注释里写明这个口径。

### 2.2 桶宽与保留期

`setting/perf_metrics_setting/config.go`：

- `BucketTime` 默认 `"hour"` → `"5min"`
- `RetentionDays` 默认 `0` → `7`（`pkg/perf_metrics/flush.go:70-72` 对 `0` 直接 return，是当前不清理的原因）

flush 侧不动：`flushCompletedBuckets` 每次扫全部已关闭桶，`drain()` 是 `Swap(0)`，与桶宽无耦合。

**行数影响**：约 5x 而非 12x（热点模型/分组对是 12x，稀疏对基本不变）。中型场景约 21,480 行/天，配合 7 天保留期可控。

### 2.3 模型广场三处连带修复

桶宽变化后这三处会坏，必须同步修：

| 位置 | 问题 | 修法方向 |
|---|---|---|
| `web/src/features/.../model-details-charts.tsx:32-36` | `formatHourLabel` 把 288 个点全标成 `HH:00`，分类轴 key 重复 | 标签精确到分钟，或按桶宽动态选格式 |
| `pkg/perf_metrics/metrics.go:190` | `recentSuccessRates(..., 3)` 原意「近 3 小时」，5min 桶下变成「近 15 分钟」 | 改为按时长计算桶数，而非硬编码桶个数 |
| `model-details-uptime-sparkline.tsx` | 渲染 288 根条溢出 | 降采样到固定条数 |

## 3. 查询层

### 3.1 现有查询不可用

`GetPerfMetricsSummaryBucketsAll`（`model/perf_metric.go:99`）**用不了**：`GROUP BY model_name, bucket_ts` 把分组维度塌掉了，且 Select 里没有 ttft 两列。

### 3.2 新查询

在 `model/perf_metric.go` 新增按 `(group, model_name, bucket_ts)` 聚合的查询：

- 入参：分组白名单、模型白名单（按分组）、时间范围
- 出参每行含：`group`、`model_name`、`bucket_ts`、`request_count`、`success_count`、`total_latency_ms`、`ttft_sum_ms`、`ttft_count`
- 分组列名用 `commonGroupCol`（`group` 是保留字）
- 只做 `WHERE bucket_ts BETWEEN ?` + `GROUP BY`，**不写 SQL 侧时间分桶**

### 3.3 展示桶合并（Go 侧）

底层 5min 桶 → 展示桶按 `displayTs = ts - ts%displaySeconds` 归并，**累加计数器后再算率**：

```
availability = Σsuccess_count / Σrequest_count
avg_ttft     = Σttft_sum_ms / Σttft_count      // ttft_count 为 0 时标记无样本，不返回 0
avg_latency  = Σtotal_latency_ms / Σrequest_count
```

**禁止对多个桶的 rate 求平均**（`model-details-performance.tsx:127-131` 是错误示范）。

展示桶宽从配置读，且不得小于 `GetBucketSeconds()`——管理员若把全局桶宽调回 `hour`，5/15/30 档物理上无法提供，此时降级为可用的最细粒度并在 UI 提示。

### 3.4 尾桶与最小样本

- 丢弃 `displayTs + displaySeconds > now` 的桶；若 `displaySeconds < FlushInterval*60`，多丢一个尾桶
- `request_count` 低于最小样本阈值的桶按 no-data 处理（对所有桶生效）
- 卡片头部在线率用整个 24h 累加值，不受最新桶影响

## 4. 配置层

### 4.1 结构体替换（破坏性，不留兼容层）

```go
// 旧（删除）
type GroupMonitoringTarget struct { Group, Model, EndpointType string; Stream bool }
type GroupMonitoringSetting struct { Enabled bool; IntervalMinutes, HistoryRetentionDays int; Targets []GroupMonitoringTarget }

// 新
type GroupMonitoringGroup struct {
    Group       string   `json:"group"`
    Description string   `json:"description"`
    Models      []string `json:"models"`
}
type GroupMonitoringSetting struct {
    Enabled       bool                   `json:"enabled"`
    BucketMinutes int                    `json:"bucket_minutes"` // 5/15/30/60
    Groups        []GroupMonitoringGroup `json:"groups"`
}
```

option key：保留 `group_monitoring_setting.enabled`，新增 `.bucket_minutes` 与 `.groups`，废弃 `.interval_minutes` / `.history_retention_days` / `.targets`（不读、不迁移；options 表残留行无害）。

校验：`bucket_minutes` 必须是 5/15/30/60 之一；group 必须存在于 `ratio_setting.GetGroupRatioCopy()`；model 必须在 `model.GetGroupEnabledModels(group)` 内（沿用 `controller/group_monitoring.go:223-235` 的既有校验思路）。

## 5. API 契约

### `GET /api/group-monitoring/summary`（登录用户，入口不变）

窗口固定 24h，不再接受 `hours` 参数。

```jsonc
{
  "bucket_minutes": 5,
  "groups": [{
    "group_name": "image",
    "description": "如果显示无数据，是因为24小时内没有相关模型的调用",
    "models": [{
      "model_name": "gpt-image-2",
      "has_data": true,
      "availability_rate": 97.53,      // has_data=false 时不参考
      "avg_latency_ms": 117260,
      "avg_ttft_ms": null,             // 无 TTFT 样本时为 null，不是 0
      "ttft_sample_count": 0,
      "request_count": 140,
      "buckets": [{"ts": 0, "request_count": 0, "success_count": 0, "state": "no-data"}]
    }]
  }]
}
```

`state` 由后端算好（`healthy` / `degraded` / `down` / `no-data`），避免前端重复实现判定规则。

### `GET|PUT /api/group-monitoring/admin`（root）

- GET 返回 `{setting, available_groups, available_models_by_group}`，**不再返回** `summaries` 与 `latest_results`
- PUT 保存新结构
- **删除** `POST /run` 与 `POST /run-group`

## 6. 前端改动

UI 结构从「一个分组一张卡」改为「分组作为 section，section 内每个模型一张卡」（对齐用户参考的形态）：

- section 头显示分组名 + 配置的备注说明 + 模型数 + 异常数
- 模型卡显示：模型名、状态徽章、在线率(24H)、延迟或 TTFT(24H)、色条时间线

改动点：

| 文件 | 改动 |
|---|---|
| `features/group-monitoring/types.ts` | 按新 DTO 重写 |
| `features/group-monitoring/api.ts` | 删除 run / run-group 两个接口 |
| `features/group-monitoring/index.tsx` | 改为分组 section + 模型卡布局；删除全量触发按钮；轮询固定 60s（探测中状态已不存在） |
| `components/group-status-card.tsx` | 改造为模型卡；**删除硬编码 `grid-cols-[repeat(32,...)]`**，桶数由数据长度驱动 |
| `lib/status.ts` | 状态判定移到后端后，这里只做颜色映射 |
| `components/monitoring-settings-panel.tsx` | 删除探测字段（Probe interval / History retention / Probe model / Endpoint Type / Stream Mode / Latest probe results）；改为分组勾选 + 每组模型多选 + 备注输入 + 分桶下拉 |
| `system-settings/models/group-monitoring-settings-section.tsx` | 同步字段变更 |

文案走 i18n，key 用英文原文。

## 7. 删除清单

| 类别 | 内容 |
|---|---|
| 探测执行体 | `controller/group_monitoring.go:407-533`（httptest 链路、串行循环、保留期清理） |
| 调度 | `controller/system_task_handlers.go:28-50` `groupMonitoringHandler` 及其注册；`model/system_task.go:24` 的类型常量 |
| 存储 | `model/group_monitoring.go` 全文；`model/main.go:338,401` 的 AutoMigrate 注册 |
| 内存聚合 | `controller/group_monitoring.go:92-145` 32 桶聚合、`AvailabilityRate=-1` 哨兵、`Stale` 判定 |
| 探测标记消费点 | `controller/relay.go:530`、`controller/relay.go:554-556`、`service/log_info_generate.go:95-97`、`service/text_quota.go:400-402` |
| 去重锁 | `controller/group_monitoring.go:28-31` `sync.Map` + `beginGroupMonitoringRun` |
| 测试 | `controller/group_monitoring_test.go`、`setting/operation_setting/group_monitoring_setting_test.go` 中针对探测的用例 |

**表 `group_monitoring_results`**：从 AutoMigrate 移除后，已有部署的库里会残留空表。不写 DROP 迁移（三库 DDL 差异 + 误删风险），在 release notes 里说明可手动清理。

## 8. 不能碰的东西

- `controller/channel-test.go` 全文，特别是 `buildTestRequest`（:698）与 `resolveChannelTestUserID`（:55）—— 只断开分组监控对它们的调用（`group_monitoring.go:330,384,286,503`），函数本体和渠道测试行为不变。
- `is_channel_test` 的 15 处旁路逻辑 —— 渠道测试共用。
- `perf_metrics` 表结构 —— 不加 `channel_id`。

## 9. 与兄弟任务 B 的协调

`09-10-admin-channel-availability` 也会改 `controller/relay.go`（attempt 级打点）。两者改动区域相邻，**建议 A 先落地、B 后跟进**，避免同一文件并行冲突。B 依赖 A 建立的打点范式。

## 10. 风险

| 风险 | 影响 | 缓解 |
|---|---|---|
| 多节点最新桶不完整 | 尾桶假故障 | 丢尾桶 + 最小样本门槛（R8）。根因 `mergeRedisActiveBuckets` dead path 不在本任务修 |
| 桶宽改动波及模型广场 | pricing 页面显示错乱 | 三处同步修复并纳入验收 |
| 任务类打点口径 | 「成功」语义与同步 relay 不同 | 明确只记提交阶段，代码注释写明 |
| 冷门模型长期无数据 | 页面大片「—」 | 方案固有代价，用户已确认接受（不做兜底探测）；用配置的备注说明向用户解释 |
| 三库兼容 | 新查询在某库失败 | 分桶在 Go 侧、`commonGroupCol`、只用 GORM 方法 |
