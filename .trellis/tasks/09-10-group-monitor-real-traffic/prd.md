# 分组监控改数据源与配置化

## Goal

把「分组监控」的数据来源从**主动合成探测**改为**真实调用聚合**（复用 `perf_metrics`），并把当前硬编码的展示维度改为可配置：监控哪些分组、每个分组监控哪些模型、分组备注说明、图表分桶粒度。

页面仍然是**登录用户可见的内部页面**，入口保持不变。不做匿名公开状态页。

## Background

当前实现（见 `research/current-group-monitor.md`）是 100% 主动合成探测：system task 每 5 分钟在 master 节点串行跑一轮，用进程内 `httptest` 组一条真实的 `Distribute → Relay` 链路，靠 `is_channel_test` 标记旁路计费与自动封禁。虽然不扣内部额度，但**上游是真实调用、真实产生费用**，且探测出的可用率/延迟是合成流量的表现，不反映真实用户请求。

`perf_metrics`（见 `research/log-aggregation-feasibility.md`）已经在聚合真实调用：维度 `(model_name, group, bucket_ts)`，指标含 `request_count / success_count / total_latency_ms / ttft_sum_ms / ttft_count`，5 分钟桶已支持，打点在 relay 层且成功失败都打，`group` 取 `relayInfo.UsingGroup`（与分组监控同源），探测流量按 `IsChannelTest` 自动排除。

**不要走聚合 `logs` 表这条路**：`ERROR_LOG_ENABLED` 默认 `false` 导致失败请求不入库（在线率分母缺失），TTFT 仅存在于 `logs.other` 的 JSON 文本 `frt` 字段，且日志库可能是独立库甚至 ClickHouse。

## Requirements

### R1 数据源切换

- 分组监控展示的在线率、延迟、TTFT、时间线，全部改为从 `perf_metrics` 读取真实调用数据。
- 在线率 = `success_count / request_count`；TTFT = `ttft_sum_ms / ttft_count`；延迟 = `total_latency_ms / request_count`。
- 某分组+模型在窗口内无调用时，展示「无数据」而非 0%，与「可用率 0%」明确区分。

### R2 删除主动探测

- 探测执行体、探测调度（system task handler）、`group_monitoring_results` 表及其 AutoMigrate 注册、探测相关配置项，整体删除，不保留开关、不保留兼容层。
- **约束（硬性）**：`controller/channel-test.go` 的渠道测试功能必须原样保留、行为不变。分组监控当前复用了它的 `buildTestRequest` 与 `resolveChannelTestUserID`，删除探测时只断开复用关系，不得改动或删除这两个函数本身，也不得改动渠道测试的任何行为。
- 两类标记要分清，处理方式不同：
  - `is_channel_test`（15 处旁路计费/封禁逻辑）是**渠道测试共用**的，**保留不动**。
  - `group_monitoring_probe` 是**分组监控探测专属**的，其 4 个消费点必须一并清理：`controller/relay.go:530`、`controller/relay.go:554-556`、`service/log_info_generate.go:95-97`、`service/text_quota.go:400-402`。

### R3 监控范围配置

- 可勾选「要监控哪些分组」。
- 每个被监控的分组下，**各自独立选择要监控的模型列表**（不是全局共用一份模型列表）。
- 每个被监控的分组可填写**备注说明文案**，展示在该分组卡片区域。
- 未被勾选的分组不出现在监控页面。

### R4 分桶配置

- 图表时间线的分桶粒度可配置：**5 / 15 / 30 / 60 分钟**四档。
- 当前前端硬编码 32 桶（`grid-cols-[repeat(32,...)]`）与后端 `bucketWidth = window/32` 必须改为按配置的分桶宽度动态计算桶数。
- `perf_metrics` 底层桶宽独立于本配置；本配置大于底层桶宽时，由查询侧把多个底层桶合并成一个展示桶。

### R5 统计窗口

- 统计窗口**固定 24 小时**，不做成配置项。

### R6 补齐打点覆盖面

`perf_metrics` 当前有两个盲区，本任务一并补齐：

- **Realtime WSS 误报（真 bug）**：成功走 `service/quota.go:150-251` `PostWssConsumeQuota` **没有打点**，失败却走 `controller/relay.go:379` 会打点 → 这类模型会显示 **0% 成功率**。必须给成功路径补上打点。
- **任务类 relay 完全无数据**：Midjourney / Suno / 视频的入口是 `controller.RelayTask` / `RelayMidjourney`，不经过 `Relay()`，一条都不进 `perf_metrics`。必须补上打点。

已确认**无需**处理的路径：图像生成（`relay/image_handler.go:149` → `service.PostTextConsumeQuota` → `service/text_quota.go:543-546`）成功与失败均已打点。

### R7 桶宽与保留期

- 全局 `BucketTime` 默认从 `hour` 降为 `5min`，分组监控的 5/15/30/60 档由 `GetBucketSeconds()` 派生（管理员若把桶宽调回 `hour`，细档物理上无法提供，UI 需相应处理）。
- `RetentionDays` 默认从 `0` 改为 `7`。注意 `pkg/perf_metrics/flush.go:70-72` 对 `0` 直接 `return`，即**当前这张表根本不会被清理**。
- 桶宽变更会波及模型广场，三个展示点必须同步修复：
  - `model-details-charts.tsx:32-36` `formatHourLabel` 会把 288 个点全标成 `HH:00`（分类轴 key 重复）
  - `pkg/perf_metrics/metrics.go:190` `recentSuccessRates(..., 3)` 语义从「近 3 小时」变成「近 15 分钟」
  - `model-details-uptime-sparkline.tsx` 会渲染 288 根条导致溢出
- flush 侧**无需改动**：`flushCompletedBuckets` 每次扫全部已关闭桶，`drain()` 是 `Swap(0)`，flush 间隔与桶宽无耦合。

### R8 规避最新桶假故障

多节点部署下，当前未 flush 的桶只存在于处理该请求那个节点的进程内存里（`mergeRedisActiveBuckets` 是 dead path，Redis 计数纯写不读）。该桶要么只含 1/N 流量，要么整桶从 series 中消失。叠加现有色条规则「2 个请求挂 1 个 = 50% = 红」，极易显示成假故障，且单节点开发环境完全复现不出来。

- 时间线丢弃 `displayTs + displaySeconds > now` 的桶，或渲染成「采集中」灰态；若 `displaySeconds < FlushInterval*60`，需多丢一个尾桶。
- 加最小样本门槛：`total_count` 低于阈值一律按 no-data 处理（对所有桶生效，不只尾桶）。
- 卡片头部的在线率用整个 24h 累加值计算，不受最新桶影响。

### R9 指标口径

- **非流式请求不累加 TTFT**：`pkg/perf_metrics/metrics.go:32` 的 `hasTtft := info.IsStream && info.HasSendResponse()` 对非流式短路为 false，生图等模型 `ttft_count = 0`。
- `avg(sum, count)` 在 `count <= 0` 时返回 **0 而非 null**（`metrics.go:359-363`），因此查询必须把 `ttft_count` 带出来，UI 才能区分「0 ms」与「无样本」。现有 DTO 未暴露该字段，需新增。
- 无 TTFT 样本的模型，展示 `TotalLatencyMs / RequestCount`（总延迟），标签需与 TTFT 区分，不得把总延迟标成首字延迟。
- **计算率值时必须先累加计数器再算率**，不得对多个桶的 rate 求平均。`model-details-performance.tsx:127-131` 是错误示范，不要照抄。

## Non-Goals

- 匿名公开状态页、对外显示名 / slug / 排序 / 供应商类型覆盖 —— 全部不做。
- 渠道维度下钻 —— 分组监控只到「分组 + 模型」。管理员的渠道可用性监控是**兄弟任务** `09-10-admin-channel-availability`，不在本任务范围。
- 给 `perf_metrics` 增加 `channel_id` 维度 —— **禁止**。GORM v1.25.2 不会修改已有唯一索引，老库上 MySQL 会静默把不同渠道计数累加进同一行，PG/SQLite 每次 flush 失败。渠道维度由兄弟任务 `09-10-admin-channel-availability` 的独立表承担。
- 修复 `mergeRedisActiveBuckets` dead path 本身 —— 不修，改为在展示侧规避（见 R8）。`GetPerfMetricsSummaryAll` 无调用点也不在本任务处理。
- 为分组监控另建一张独立桶表 —— 不做。维度与样本和 `perf_metrics` 逐字相同，属重复建设，且照样要改那 3 个打点调用点。

## Constraints

- 数据库改动必须同时兼容 SQLite / MySQL / PostgreSQL，遵循 `AGENTS.md` 的多库约定。
- 时间分桶在 Go 侧计算（`ts - ts%bucketSeconds`），不使用 SQL 侧时间函数分桶——项目无此先例且跨方言不安全。
- JSON 序列化一律走 `common/json.go` 的封装函数。
- 前端文案走 i18n，locale 文件为扁平 JSON，key 用英文原文。
- 工作区存在其他 session 的大量未提交改动：commit 必须显式列文件名，禁止 `git add -A/.`，禁止 `git restore/checkout/clean`。

## Acceptance Criteria

- [x] 分组监控页面展示的数据来自 `perf_metrics` 真实调用，与手动构造的调用记录一致 —— **代码与单测层面验证；真实流量比对待生产观察**
- [x] 无调用的分组+模型显示「无数据」，不显示为 0% 可用率
- [x] 主动探测代码、调度、结果表全部移除，仓库内无残留探测路径 —— grep 为空
- [x] `controller/channel-test.go` 的渠道测试功能行为完全不变，手动测试渠道仍正常工作 —— 该文件与 HEAD 逐字相同；**「手动测试渠道」未在真实环境点过**
- [x] 可勾选监控分组；每个分组可独立配置监控模型列表与备注说明；配置持久化后重启生效 —— **重启生效未实测**，依赖 `config.GlobalConfig` 既有的 slice JSON 反序列化机制
- [x] 分桶可在 5/15/30/60 分钟间切换，前后端桶数随配置动态变化，不再有硬编码 32
- [x] Realtime WSS 模型的成功请求被记录，不再显示 0% 成功率
- [x] 任务类 relay（MJ / Suno / 视频）的请求进入 `perf_metrics`，页面可见数据
- [x] 图像生成模型有数据，且延迟展示的是总延迟而非 TTFT，标签与 TTFT 明确区分
- [x] `ttft_count = 0` 的模型显示「无 TTFT 样本」，不显示为 0 ms
- [x] 全局桶宽降为 5min 后，模型广场三个展示点（时间标签、成功率窗口、sparkline）显示正常
- [x] `RetentionDays` 默认 7 生效，超期桶被清理 —— ⚠️ **仅对全新部署生效**。老库 `options` 表已持久化 `retention_days=0` / `bucket_time=hour`，代码默认值不覆盖已持久化行，部署后需手动改配置
- [x] 未完成的尾桶不被渲染成故障；低于最小样本阈值的桶按 no-data 处理
- [x] 率值计算为「先累加计数器再算率」，不是对多桶 rate 求平均
- [x] 后端构建通过；若触及 `relaykit/`，额外通过 `cd relaykit && GOWORK=off go build ./...`
- [x] 前端 `bun run build` 与类型检查通过
- [x] 三库兼容性：新增/变更的查询在 SQLite/MySQL/PostgreSQL 上均可执行 —— **SQLite 实跑；MySQL/PG 为代码走查**（无 SQL 侧时间函数、`group` 走 `commonGroupCol`、无 `default:true`）

## Open Questions

- 分组监控页面的「延迟」指标当前展示的是什么口径（探测总耗时 vs 首字延迟），切换数据源后要对齐成哪个口径 —— 待 design 阶段确认。
