# Research: 现有「分组监控」实现全貌

- **Query**: 摸清 FlowAPI 现有分组监控（非渠道测试）功能的完整实现：入口/命名、数据产生方式、配置项、对外 API、前端渲染
- **Scope**: internal
- **Date**: 2026-09-10

## 结论速览

1. 功能在代码里叫 **group monitoring / 分组监控**，`SystemTaskType = "group_monitoring"`（`model/system_task.go:24`）。
2. 数据来源 **100% 是主动合成探测（synthetic probe）**：system task 定时对每个配置的「分组 + 探测模型」发一次真实 relay 请求，结果单条落表 `group_monitoring_results`。**当前完全不读 `logs` 表，也不读 `perf_metrics`。**
3. 整个功能是**未提交的新代码**（`git status` 全部为 `??` 未跟踪）：`controller/group_monitoring.go`、`model/group_monitoring.go`、`setting/operation_setting/group_monitoring_setting.go`、`web/src/features/group-monitoring/`。
4. **截图里的「统计窗口（小时）」「图表分桶（分钟）5/15/30/60」「公开分组与模型（对外显示名 / slug / 说明文案 / 排序 / 公开模型 / 供应商类型覆盖）」在本仓库中未找到任何实现**（详见「未找到的部分」）。当前不存在匿名可访问的公开状态页。

---

## 1. 功能入口与命名

### 后端文件

| 文件 | 说明 |
|---|---|
| `controller/group_monitoring.go` (554 行, 未跟踪) | 全部 HTTP handler + 探测执行 + 聚合逻辑 |
| `controller/group_monitoring_test.go` (未跟踪) | 后端测试 |
| `model/group_monitoring.go` (34 行, 未跟踪) | `GroupMonitoringResult` 表模型 + 3 个 DB 函数 |
| `setting/operation_setting/group_monitoring_setting.go` (130 行, 未跟踪) | 配置结构体、默认值、校验 |
| `setting/operation_setting/group_monitoring_setting_test.go` (未跟踪) | 配置校验测试 |
| `controller/system_task_handlers.go:28-50` | `groupMonitoringHandler` 定时任务处理器 |
| `model/system_task.go:24` | `SystemTaskTypeGroupMonitoring = "group_monitoring"` |
| `router/api-router.go:25-36` | 路由注册 |
| `model/main.go:338`, `model/main.go:401` | `&GroupMonitoringResult{}` 加入 AutoMigrate |

探测请求专用的旁路开关分散在这些已有文件里（`c.Set("group_monitoring_probe", true)` 的消费方）：

- `controller/relay.go:530`（`recordResilientRouteErrorLog`：探测请求即使 ErrorLog 关闭也强制记错误日志）
- `controller/relay.go:554-556`（错误日志 `other["group_monitoring_probe"]=true`）
- `service/log_info_generate.go:95-97`（消费日志 `other["group_monitoring_probe"]=true`）
- `service/text_quota.go:400-402`（消费日志 content 追加 `"分组监控探测"`）

### 前端文件

| 文件 | 说明 |
|---|---|
| `web/src/features/group-monitoring/index.tsx` (333 行) | 页面主体 `GroupMonitoring`：搜索、排序、全量/单组触发、汇总统计 |
| `web/src/features/group-monitoring/components/group-status-card.tsx` (162 行) | 单个分组状态卡（可用率 / 色条 / 平均延迟 / 最后检测时间） |
| `web/src/features/group-monitoring/components/monitoring-settings-panel.tsx` (385 行) | 管理端配置表单 + 最近探测结果 |
| `web/src/features/group-monitoring/api.ts` (66 行) | 4 个接口封装 |
| `web/src/features/group-monitoring/types.ts` (101 行) | 全部 DTO 类型 |
| `web/src/features/group-monitoring/lib/status.ts` (21 行) | `getBucketState` 色条状态判定 |
| `web/src/features/group-monitoring/lib/form-schema.ts` (32 行) | 表单 schema |
| `web/src/routes/_authenticated/group-monitoring/index.tsx` | 路由 `/group-monitoring`，挂在 `_authenticated` 下（**必须登录**） |
| `web/src/features/system-settings/models/group-monitoring-settings-section.tsx` | 系统设置里的「Group availability monitoring」区块 |
| `web/src/hooks/use-sidebar-data.ts:89-93` | 侧边栏入口 `Group Monitoring` |

---

## 2. 数据是怎么产生的：主动定时探测

### 2.1 调度位置：system task 框架（不是 cron，不是裸 goroutine）

`controller/system_task_handlers.go:28-50`：

```go
type groupMonitoringHandler struct{}
func (groupMonitoringHandler) Type() string { return model.SystemTaskTypeGroupMonitoring }
func (groupMonitoringHandler) Enabled() bool {
	setting := operation_setting.GetGroupMonitoringSetting()
	return setting.Enabled && len(setting.Targets) > 0
}
func (groupMonitoringHandler) Interval() time.Duration {
	return time.Duration(operation_setting.GetGroupMonitoringSetting().IntervalMinutes) * time.Minute
}
```

注册于 `controller/system_task_handlers.go:25`（`RegisterScheduledSystemTasks`），由 `service.StartSystemTaskRunner()`（`service/system_task.go:123`）驱动；runner 只在 `common.IsMasterNode` 上跑（`service/system_task.go:126-128`），并通过 DB 租约去重多实例。

手动触发：
- 全量：`RunGroupMonitoringNow` → `service.EnqueueSystemTask(model.SystemTaskTypeGroupMonitoring, nil)`（`controller/group_monitoring.go:254-261`），已有活跃任务时幂等返回、`created=false`。
- 单组：`RunSingleGroupMonitoring` → 直接同步执行 `runGroupMonitoringTarget`（`controller/group_monitoring.go:267-301`），不走 system task。

### 2.2 探测链路：复用真实 relay（内存 httptest，不出网到自己）

核心在 `controller/group_monitoring.go:407-499` `runGroupMonitoringTarget`：

- 构造一个**进程内的 gin engine**（`gin.New()`，`:440`），用 `httptest.NewRecorder()` + `httptest.NewRequest()` 发请求（`:437-438`），路由链是
  `预置上下文 handler → middleware.Distribute() → Relay(c, relayFormat)`（`:442-483`）。
  所以走的是**真实 relay 链路**：真实渠道选择、真实重试/failover（`route_history`）、真实上游调用。
- 使用的 user/key：`resolveChannelTestUserID(c)`（`controller/channel-test.go:55-70`）——优先当前请求的登录用户 id，定时任务时（`c == nil`）取 **role = root 的用户**。Token 是一个**内存里伪造的 `model.Token`**（不落库）：
  ```go
  middleware.SetupContextForToken(c, &model.Token{
      UserId: testUserID, Name: "模型测试", UnlimitedQuota: true, Group: target.Group,
  })
  ```
  （`controller/group_monitoring.go:448-453`）
- 关键上下文标记：
  - `c.Set("is_channel_test", true)`（`:466`）→ `controller/relay.go:134` 置 `relayInfo.IsChannelTest = true`
  - `c.Set("group_monitoring_probe", true)`（`:447`）
  - `ContextKeyUserUnlimitedQuota = true`（`:462`）

### 2.3 是否真实扣费

**不扣费、不计用量、不改渠道状态**，靠 `IsChannelTest` 全链路旁路：

- `service/quota.go:88`、`:218`、`:346`、`:379`、`:414` — 预扣费 / 结算 / 用户与渠道用量累计全部跳过
- `service/billing.go:53`、`service/tiered_settle.go:128,176`、`service/violation_fee.go:107` — 计费、阶梯结算、违规费跳过
- `service/text_quota.go:406,449,543` — 渠道亲和用量、用量累计跳过
- `middleware/distributor.go:119,129,134,171` — 渠道禁用/亲和逻辑跳过
- `controller/relay.go:301,334,351,377` — 自动封禁（auto ban）跳过

**会产生的副作用（有意为之）**：写「模型测试」消费日志 / 错误日志到 `logs` 表，`other.group_monitoring_probe = true`，content 含 `分组监控探测`。

### 2.4 频率、并发、超时

| 项 | 值 | 位置 |
|---|---|---|
| 探测频率 | `IntervalMinutes`，默认 5，范围 1–1440 | `setting/operation_setting/group_monitoring_setting.go:12,95-97`；`controller/system_task_handlers.go:36-38` |
| 单次探测超时 | 硬编码 `60 * time.Second` | `controller/group_monitoring.go:36` `groupMonitoringProbeTimeout` |
| 并发 | **无并发，串行** for 循环逐个 target 探测 | `controller/group_monitoring.go:509-527` |
| 去重锁 | `sync.Map` 按 group 名互斥（`beginGroupMonitoringRun`），重复触发返回 `errGroupMonitoringAlreadyRunning` | `controller/group_monitoring.go:28-31,303-311,408-411` |
| 目标上限 | 50 | `setting/.../group_monitoring_setting.go:14` |
| 保留天数 | `HistoryRetentionDays`，默认 7，范围 1–90，每次任务结束清理 | `:13,98-100`；清理在 `controller/group_monitoring.go:528-531` |

### 2.5 结果落表

`model/group_monitoring.go:3-16`，表名 `group_monitoring_results`：

```go
type GroupMonitoringResult struct {
	ID           int64  `gorm:"primaryKey"`
	GroupName    string `gorm:"size:64;index:idx_group_monitoring_group_time,priority:1"`
	Success      bool
	LatencyMs    int64
	ChannelID    int
	RequestID    string `gorm:"size:64;index"`
	RouteHistory string `gorm:"type:text"`   // JSON，仅管理员接口暴露
	ErrorCode    string `gorm:"size:64"`
	ErrorMessage string `gorm:"type:text"`
	CheckedAt    int64  `gorm:"index:idx_group_monitoring_group_time,priority:2;index"`
}
```

DB 操作只有三个：`CreateGroupMonitoringResult`（:18）、`GetGroupMonitoringResults(groups, start, end)`（:22，**一次性 Find 全量明细行，无聚合 SQL**）、`DeleteGroupMonitoringResultsBefore(cutoff)`（:32）。

注意：`LatencyMs` 是**整个探测请求的墙钟耗时**（`time.Since(started).Milliseconds()`，`controller/group_monitoring.go:486`），**不是 TTFT**。

---

## 3. 配置项：实际存在的只有 4 个

`setting/operation_setting/group_monitoring_setting.go:18-36`：

```go
type GroupMonitoringTarget struct {
	Group        string `json:"group"`
	Model        string `json:"model"`
	EndpointType string `json:"endpoint_type"`  // auto / openai / anthropic / gemini / ... 
	Stream       bool   `json:"stream"`
}

type GroupMonitoringSetting struct {
	Enabled              bool
	IntervalMinutes      int   // 默认 5
	HistoryRetentionDays int   // 默认 7
	Targets              []GroupMonitoringTarget
}
```

- 注册：`config.GlobalConfig.Register("group_monitoring_setting", &groupMonitoringSetting)`（`:38-40`）
- 持久化：写入 `options` 表，4 个 key（`controller/group_monitoring.go:241-246`）：
  `group_monitoring_setting.enabled` / `.interval_minutes` / `.history_retention_days` / `.targets`（targets 为 JSON 字符串）
- 校验：`ValidateGroupMonitoringSetting`（`:94-130`）+ 控制器侧再校验 group 必须存在于 `ratio_setting.GetGroupRatioCopy()`、model 必须在 `model.GetGroupEnabledModels(group)` 内（`controller/group_monitoring.go:223-235`）
- 端点类型白名单/流式支持：`IsGroupMonitoringEndpointTypeSupported`（:65）、`GroupMonitoringEndpointSupportsStream`（:82）

### 「统计窗口 / 分桶」当前的真实语义

**当前没有任何可配置的窗口/分桶设置**，全部是硬编码：

- 窗口：由前端 query 参数 `hours` 决定，默认 24，范围 1–168（`controller/group_monitoring.go:148-151`）；前端固定传 24（`web/src/features/group-monitoring/index.tsx:61-62`）。
- 分桶：**固定 32 桶**，`bucketWidth = (end-start)/32`（`controller/group_monitoring.go:93-98`）。也就是说 24h 窗口下每桶 45 分钟，是窗口除出来的，**不是「5/15/30/60 分钟」的可选值**。
- 前端 CSS 也硬编码 32 列：`grid-cols-[repeat(32,minmax(3px,1fr))]`（`group-status-card.tsx:119`）。
- 「聚合」只发生在**内存里**：`buildGroupMonitoringSummaries` 把 `group_monitoring_results` 的明细行按桶累加（`controller/group_monitoring.go:92-145`）。**没有预聚合表、没有聚合 SQL。**

所以：**当前的统计窗口/分桶作用在「探测结果表」上，不是别的数据源。**

---

## 4. 对外 API

`router/api-router.go:25-36`：

```go
groupMonitoringRoute := apiRouter.Group("/group-monitoring")
groupMonitoringRoute.Use(middleware.UserAuth())          // ← 必须登录，非匿名
{
    groupMonitoringRoute.GET("/summary", controller.GetGroupMonitoringSummary)
    groupMonitoringAdminRoute := groupMonitoringRoute.Group("/admin")
    groupMonitoringAdminRoute.Use(middleware.RootAuth())  // ← root 才能配置/触发
    {
        GET  ""            → GetGroupMonitoringAdmin
        PUT  ""            → UpdateGroupMonitoringAdmin
        POST "/run"        → RunGroupMonitoringNow
        POST "/run-group"  → RunSingleGroupMonitoring
    }
}
```

### `GET /api/group-monitoring/summary?hours=24`（普通用户，脱敏）

响应 `data: []groupMonitoringSummary`（`controller/group_monitoring.go:45-55`）：

| 字段 | 类型 | 含义 |
|---|---|---|
| `group_name` | string | 分组名 |
| `availability_rate` | float64 | 窗口内成功率百分比；**无数据时为 `-1`**（`:107`） |
| `average_latency_ms` | int64 | 窗口内平均整请求耗时 |
| `last_checked_at` | int64 | 最后一次探测 unix 秒 |
| `last_success` | bool | 最后一次是否成功 |
| `stale` | bool | `end - last_checked_at > intervalMinutes*120`（即 2 个周期，`:140`） |
| `testing` | bool | 该组正在探测（`sync.Map` 内存状态） |
| `batch_testing` | bool | 存在活跃的 group_monitoring system task |
| `buckets` | 32 项 | `{timestamp, success_count, total_count, latest_success?}` |

**无缓存**：每次请求直接查明细行 + 查活跃 system task（`:159-168`），在内存里重算聚合。

### `GET /api/group-monitoring/admin`（root）

返回 `{setting, available_groups, available_models, summaries, latest_results}`（`controller/group_monitoring.go:201-205`）。`latest_results` 是每组最后一条明细 + 解析出的 `route_history`（`:543-554`，`toGroupMonitoringAdminResult` `:65-71`）。窗口固定 24h（`:191`）。

---

## 5. 前端渲染字段映射

`web/src/features/group-monitoring/components/group-status-card.tsx`：

| UI 元素 | 数据字段 | 代码位置 |
|---|---|---|
| 右上大号「在线率」百分比 | `availability_rate`（`>= 0` 才算有数据） | `group-status-card.tsx:59-61,110` |
| 状态圆点颜色（绿/红/灰） | `testing` → 琥珀脉冲；`hasData && !stale` → `last_success` 绿/红；否则灰 | `:63-75` |
| 色条时间线（32 格） | `buckets[]`，颜色由 `getBucketState(bucket)` 决定 | `:118-130`，`lib/status.ts:12-20` |
| 色条状态规则 | `total_count===0` → no-data(灰)；有 `latest_success` → healthy/down；否则按 rate：≥0.99 healthy、≥0.8 degraded(琥珀)、else down | `lib/status.ts:13-19` |
| 「xx ms average」 | `average_latency_ms`（**不是 TTFT**，是整请求耗时） | `:76-80,131-135` |
| 右下时间 | `last_checked_at`（HH:mm） | `:29-35,136-139` |
| 「Test this group」按钮 | 仅 `canTest`（root）显示 → `POST /admin/run-group` | `:141-158`，`index.tsx:218-219` |

页头统计（`index.tsx:161-174`）：
- online 数 = `availability_rate >= 0 && !stale && last_success`
- 平均可用率 = 所有 `availability_rate >= 0` 的算术平均

轮询：`refetchInterval` — 批量测试中 1s，否则 60s（`index.tsx:63-67`）。

**前端没有 TTFT 字段**。TTFT 在本项目里属于另一套系统（见下）。

---

## 6. 与「渠道测试」和其他相邻功能的边界

### 6.1 vs 渠道测试（`controller/channel-test.go`）— 有代码复用，但目标不同

| | 渠道测试 | 分组监控 |
|---|---|---|
| 测试对象 | **指定单个渠道**（`testChannel(ctx, channel, ...)`，`channel-test.go:72`） | **一个分组**，由 `middleware.Distribute()` 正常选渠道 |
| 是否走 relay 全链路 | 直接调 adaptor，不走 Distribute | 走 `Distribute() → Relay()`，含重试/failover |
| 结果去处 | 更新 `channels.test_time/response_time`，可自动禁用渠道 | 写 `group_monitoring_results`，**绝不改渠道状态** |
| system task type | `channel_test`（`system_task_handlers.go:57`） | `group_monitoring`（`:31`） |

**复用的东西（明确边界）**：
- `buildTestRequest(model, endpointType, channel, isStream)`（`channel-test.go:698`）—— 分组监控在 `controller/group_monitoring.go:330,384` 复用它构造探测请求体，`channel` 传 `nil`。
- `resolveChannelTestUserID(c)`（`channel-test.go:55`）—— 分组监控在 `:286,503` 复用它拿 root user id。
- 共用 `is_channel_test` 上下文标记来旁路计费/封禁（分组监控在 `:466` 主动设置）。

除此之外两者的调度、存储、API、前端完全独立。

### 6.2 vs `perf_metrics`（真实流量聚合）— 完全独立，但是「日志聚合」最接近的现成范式

`model/perf_metric.go` + `pkg/perf_metrics/` + `setting/perf_metrics_setting/config.go`：

- `PerfMetric` 是**预聚合桶表**（`model_name + group + bucket_ts` 唯一索引，`perf_metric.go:11-23`），字段含 `RequestCount / SuccessCount / TotalLatencyMs / TtftSumMs / TtftCount / OutputTokens / GenerationMs`。
- 数据来自**真实 relay 流量**在内存累加，按 `FlushInterval`（默认 5 分钟）刷盘 upsert（`pkg/perf_metrics/flush.go:12-60`）。
- 分桶配置是 `BucketTime: "minute" | "5min" | "hour"`（`perf_metrics_setting/config.go:27-36`），**不是 5/15/30/60 分钟**。
- 已有聚合 SQL：`GetPerfMetricsSummaryAll`（`perf_metric.go:81`）、`GetPerfMetricsSummaryBucketsAll`（`:99`），带 `commonGroupCol` 跨库列名处理。
- 09-02 PRD 明确要求分组探测**不得写入 `perf_metrics`**（`.trellis/tasks/09-02-group-availability-monitoring/prd.md` Technical Notes）。

### 6.3 vs Uptime Kuma 集成 — 无关

`controller/uptime_kuma.go`、`setting/console_setting/validation.go:243-305`、`GET /api/uptime/status`（`router/api-router.go:22`）是 upstream new-api 自带的「拉取外部 Uptime Kuma 状态页」功能。那里的 `slug` / `categoryName` / `description` 是 **Uptime Kuma 状态页的 slug**，与分组监控无任何代码关系。zh.json:4484 的 `"Status Page Slug"` 也属于这里。

### 6.4 日志表（若改为日志聚合，数据源在这里）

`model/log.go:59-82` `Log` 表可用字段：`CreatedAt`（unix 秒，`idx_created_at_type` 索引）、`Type`（`LogTypeConsume=2` / `LogTypeError=5`，`:87,90`）、`ModelName`（索引）、`Group`（索引）、`ChannelId`（索引）、`UseTime`、`IsStream`、`RequestId`、`Other`（JSON 文本，含 `frt`（TTFT）、`error_code`、`status_code`、`route_history`、`group_monitoring_probe`）。

- TTFT 在 `other.frt`：`service/log_info_generate.go` 中 `other["frt"] = relayInfo.FirstResponseTime - relayInfo.StartTime`（`log_info_generate.go` 的 `GenerateTextOtherInfo`，约 `:105`）。**`Other` 是 JSON 字符串列，跨库无法直接 GROUP BY，取 TTFT 需要应用层解析或改用 perf_metrics 的数值列。**
- 现有日志聚合类查询范式：`SumUsedQuota`（`model/log.go:721`）、`SumUsedToken`（`:777`），可参考其跨库写法。

---

## 7. 「改成日志聚合」需要改动的最小面（仅事实清单，不含方案建议）

以下是**当前实现中与「探测」强耦合、任何日志聚合改造都必然会触及**的点：

| 层 | 位置 | 耦合内容 |
|---|---|---|
| 数据产生 | `controller/group_monitoring.go:407-499` | 整个 httptest 探测执行体 |
| 调度 | `controller/system_task_handlers.go:28-50` + `controller/group_monitoring.go:501-533` | 定时任务、串行探测循环、保留期清理 |
| 存储 | `model/group_monitoring.go` 全文；`model/main.go:338,401` | `group_monitoring_results` 表与 AutoMigrate |
| 聚合 | `controller/group_monitoring.go:92-145` | 32 桶硬编码内存聚合，`AvailabilityRate=-1` 哨兵、`Stale = interval*120` |
| 窗口 | `controller/group_monitoring.go:148-151`（1–168h，默认 24）；`:191`（admin 固定 24h） | 窗口来源 |
| 配置 | `setting/operation_setting/group_monitoring_setting.go` 全文 + `controller/group_monitoring.go:236-251`（4 个 option key） | `Enabled/IntervalMinutes/HistoryRetentionDays/Targets{group,model,endpoint_type,stream}` |
| 探测旁路标记 | `controller/relay.go:530,554`；`service/log_info_generate.go:95`；`service/text_quota.go:400` | `group_monitoring_probe` 的 4 个消费点 |
| 对外 DTO | `controller/group_monitoring.go:38-63`（bucket/summary/admin result） | 契约 |
| 前端类型 | `web/src/features/group-monitoring/types.ts` 全文 | 与 DTO 一一对应 |
| 前端渲染 | `group-status-card.tsx:119`（`repeat(32,...)`）、`lib/status.ts:12-20` | 32 桶硬编码、色条状态规则 |
| 前端配置表单 | `monitoring-settings-panel.tsx`（`Probe interval (minutes)` / `History retention (days)` / `Monitoring targets` / `Probe model` / `Endpoint Type` / `Stream Mode` / `Latest probe results`） | 现有全部为探测语义字段 |
| 复用点（改造时需保留或解耦） | `buildTestRequest`（`channel-test.go:698`）、`resolveChannelTestUserID`（`channel-test.go:55`） | 分组监控是这两个函数的额外调用方 |

现成可参考的「按桶预聚合」实现：`model/perf_metric.go`（唯一索引 upsert + SUM GROUP BY）、`pkg/perf_metrics/flush.go`（内存累加定时刷盘）、`setting/perf_metrics_setting/config.go`（分桶配置）。

---

## 未找到的部分（明确声明：以下功能在本仓库中不存在）

对照用户描述的截图逐项核查，以下**均未找到任何实现，也未在 git 历史中出现**（`git log --all -S "bucket_minutes"` / `-S "public_groups"` 均无结果）：

1. **「统计窗口（小时）」作为可配置项** — 不存在。窗口只是 `GET /summary` 的 `hours` query 参数，前端硬编码 24，无 UI、无持久化配置字段。
2. **「图表分桶（分钟）5 / 15 / 30 / 60」** — 不存在。当前是硬编码 32 桶（`controller/group_monitoring.go:93`）。最接近的 `perf_metrics_setting.BucketTime` 只有 `minute/5min/hour` 三档，且服务于模型广场而非分组监控。
3. **「公开分组与模型」配置块**，包括：
   - 对外显示名（display name）— 未找到
   - slug — 仅在 Uptime Kuma 集成（`console_setting/validation.go:268`）和自定义 OAuth provider（`model/custom_oauth_provider.go:43`）中存在，与分组监控无关
   - 说明文案（description）— 未找到
   - 排序（sort order）— 未找到（前端排序是客户端 `SortMode`，不持久化，`index.tsx:39,148-158`）
   - 公开模型（public models）— 未找到
   - 供应商类型覆盖（provider type override）— 未找到；`GroupMonitoringTarget.EndpointType` 是**协议/端点类型**（openai/anthropic/gemini/...），语义是请求格式，不是供应商类型
4. **匿名可访问的公开状态页** — 不存在。`/api/group-monitoring/*` 全部挂 `middleware.UserAuth()`（`router/api-router.go:26`），前端路由挂 `_authenticated`。09-02 PRD 的 Out Of Scope 第一条明确写「匿名公开状态页」不做。
5. **摘要接口缓存** — 不存在，每次请求实时查明细行重算。

结论：截图内容与本仓库当前 `main` + 工作区代码不符，属于**设计稿 / 目标态 / 其他项目（09-02 PRD 提到的参考对象 CCH 状态页、EF-new-api）**，不是已实现功能。若主 agent 需要确认截图来源，需要用户提供上下文。

## Caveats

- 本功能所有核心文件在 `git status` 中均为 **未跟踪（`??`）**，即尚未提交。工作区另有 150+ 个其他 session 的改动，本次调研全程只读。
- `controller/group_monitoring.go:475-478` 在探测失败且非 resilient route 时会**主动补写一条错误日志**，这意味着探测结果已经在 `logs` 表里留有痕迹（`other.group_monitoring_probe = true`），这是探测数据与日志数据当前唯一的交点。
- 未阅读 `monitoring-settings-panel.tsx` 全文（仅提取了字段清单）与 `controller/group_monitoring_test.go`、`setting/operation_setting/group_monitoring_setting_test.go` 的测试细节。
