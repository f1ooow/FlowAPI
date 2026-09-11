# Research: perf_metrics 作为分组监控数据源的适配性

- **Query**: perf_metrics 覆盖面（尤其生图）/ 非流式 latency 与 TTFT 口径 / 改 5min 桶的影响面 / 小桶合并大桶可行性 / 多节点当前桶实时性
- **Scope**: internal
- **Date**: 2026-09-10
- **前置调研**: `.trellis/tasks/09-10-group-monitor-log-aggregation/research/current-group-monitor.md`、`log-aggregation-feasibility.md`（不重复其内容）

---

## 0. 结论速览

| 问题 | 结论 |
|---|---|
| Q1 生图能否监控到 | **能**。`/v1/images/generations` 与 gemini image 走 `PostTextConsumeQuota` → `RecordRelaySample` |
| Q1 覆盖盲区 | **任务类（MJ/Suno/视频）完全不在**；**Realtime WSS 只记失败不记成功**（会假性 0% 成功率） |
| Q2 非流式 TTFT | **完全不累加**（`TtftCount` 不增），生图模型的 `avg_ttft_ms` 恒为 0；只能展示 `TotalLatencyMs/RequestCount` |
| Q2 流式 latency | 记的是**整请求总耗时**（含全部生成），不是 TTFT；TTFT 单独在 `ttft_sum_ms/ttft_count` |
| Q3 改 5min 桶 | 数据层安全，**展示层有 3 处会破**（都在模型广场/pricing）；`RetentionDays=0` 下必须同时设保留期 |
| Q3 flush | flush 间隔与桶宽**无耦合约束**，不会漏 flush 也不会重复（`drain()` 是 `Swap(0)`） |
| Q4 合并小桶 | `GetPerfMetricsSummaryBucketsAll` **不带 group、不带 ttft**，**支撑不了**，必须新写查询函数 |
| Q5 多节点当前桶 | 表现为「**只有本节点数据**」或「**该桶整体缺失**」，小样本下极易被判成故障 |

---

## Q1 覆盖面 —— 哪些请求会进 perf_metrics

### 1.1 三个调用点的上游

`RecordRelaySample` 全仓库只有 3 个调用点（`grep -rn "RecordRelaySample" --include="*.go"`）：

| # | 调用点 | 语义 | 条件 |
|---|---|---|---|
| A | `controller/relay.go:379` | **失败** | `newAPIError != nil && !relayInfo.IsChannelTest`（`controller/relay.go:377`） |
| B | `service/quota.go:381` | **成功** | 在 `PostAudioConsumeQuota`（`service/quota.go:279`）末尾，`!relayInfo.IsChannelTest`（`:379`） |
| C | `service/text_quota.go:545` | **成功** | 在 `PostTextConsumeQuota`（`service/text_quota.go:397`）末尾，`!relayInfo.IsChannelTest`（`:543`） |

**A 的覆盖范围 = 整个 `controller.Relay()` 函数**（`controller/relay.go:71`）。打点在选路循环结束之后、函数返回之前（`:370-381`），即 **重试全部耗尽后的终态失败**才打一次，中间的单次 attempt 失败不打点。`Relay()` 被 `router/relay-router.go` 的所有 HTTP 入口调用：

- `relay-router.go:82` Claude、`:87/:90` OpenAI chat/completions、`:95/:98` Responses、`:103` AlphaSearch
- `relay-router.go:108/111/114` **images**（generations / edits / 第三个入口）
- `relay-router.go:119` embeddings、`:124/127/130` audio、`:135` rerank
- `relay-router.go:140/143`、`:196` Gemini、`:72` Realtime WSS

**B/C 的覆盖范围 = 所有调用 `PostTextConsumeQuota` / `PostAudioConsumeQuota` 的成功结算路径**：

```
relay/image_handler.go:149        → PostTextConsumeQuota   （生图）
relay/gemini_handler.go:202,302   → PostTextConsumeQuota   （gemini 文本/图/embedding）
relay/claude_handler.go:157,228   → PostTextConsumeQuota
relay/compatible_handler.go:88/90, 217/219 → PostAudio / PostText （OpenAI chat 主路径）
relay/responses_handler.go:158,166,168    → PostText / PostAudio
relay/audio_handler.go:71,73      → PostAudio / PostText   （transcription / TTS）
relay/embedding_handler.go:91     → PostTextConsumeQuota
relay/rerank_handler.go:103       → PostTextConsumeQuota
relay/alpha_search_handler.go:118 → PostTextConsumeQuota
relay/websocket.go:44             → PostWssConsumeQuota    ← 唯一没有打点的成功路径
```

### 1.2 逐类型判定

| 请求类型 | 成功是否记 | 失败是否记 | 依据 |
|---|---|---|---|
| **普通文本对话 OpenAI（流/非流）** | ✅ | ✅ | `relay/compatible_handler.go:90,219` → C；`relay-router.go:87` → A |
| **Claude 格式（流/非流）** | ✅ | ✅ | `relay/claude_handler.go:157,228` → C；`relay-router.go:82` → A |
| **Gemini 格式（流/非流）** | ✅ | ✅ | `relay/gemini_handler.go:202,302` → C；`relay-router.go:140,143,196` → A |
| **Responses / Compaction** | ✅ | ✅ | `relay/responses_handler.go:158,168` → C |
| **图像生成 `/v1/images/generations`、`/images/edits`** | ✅ | ✅ | `relay/image_handler.go:149` `service.PostTextConsumeQuota(c, info, usage.(*dto.Usage), logContent)` → C；`relay-router.go:108,111,114` → A |
| **gemini image** | ✅ | ✅ | gemini 出图同样落在 `relay/gemini_handler.go:202` 的 `PostTextConsumeQuota`（图像 token 记在 `summary.ImageTokens`，`service/text_quota.go:486-490`） |
| **音频 transcription / translation** | ✅ | ✅ | `relay/audio_handler.go:71,73` → B 或 C |
| **音频 TTS（`/audio/speech`）** | ✅ | ✅ | 同上；注意 `supportsTransparentFailover` 对 audio 三种 mode 关闭重试（`controller/relay.go:388-392`），失败即终态，A 仍会打点 |
| **Embedding（OpenAI + Gemini）** | ✅ | ✅ | `relay/embedding_handler.go:91`、`relay/gemini_handler.go:302` → C |
| **Rerank** | ✅ | ✅ | `relay/rerank_handler.go:103` → C |
| **Realtime WSS（`/v1/realtime`）** | ❌ **不记** | ✅ 记 | 成功走 `relay/websocket.go:44` → `PostWssConsumeQuota`（`service/quota.go:150-251`），该函数末尾只有 `model.RecordConsumeLog`（`:237-250`），**没有 perfmetrics 调用**；失败仍走 A（`relay-router.go:72` 也是 `controller.Relay`）→ **只有失败进桶，success_rate 会显示 0%** |
| **Midjourney（`/mj/**`）** | ❌ | ❌ | 入口是 `controller.RelayMidjourney`（`controller/relay.go:577`），完全不经过 `Relay()`；结算在 `relay/mjproxy_handler.go:276,641` 直接 `RecordConsumeLog` |
| **Suno / 视频 / Kling / Jimeng（task 类）** | ❌ | ❌ | 入口是 `controller.RelayTask`（`controller/relay.go:659`，路由 `router/relay-router.go:182`、`router/video-router.go:23,30,38,50`），自带一套选路重试循环（`:691-`），结算在 `service/task_billing.go:53` `RecordConsumeLog`，无 perfmetrics |
| **分组监控探测 / 渠道测试** | ❌ | ❌ | A/B/C 三处都有 `!relayInfo.IsChannelTest` 守卫；探测在 `controller/group_monitoring.go:466` 设置 `is_channel_test` |

### 1.3 「不被记录」的原因归纳

不记录的两类走的都是**不含打点的独立结算路径**：

1. **Realtime**：成功结算函数 `PostWssConsumeQuota` 是 `PostTextConsumeQuota` 的兄弟实现，作者加打点时只加在 `PostText` / `PostAudio` 两个函数上，漏了 WSS。
2. **任务类（MJ / Suno / 视频）**：根本不共享 `controller.Relay()` 与 `Post*ConsumeQuota`，是完全并行的一条 relay + billing 链路（`RelayTask` / `RelayMidjourney` → `service/task_billing.go` / `relay/mjproxy_handler.go`）。

### 1.4 失败请求的细节（重要，会影响在线率口径）

- **只记终态失败**：`controller/relay.go:377` 在跨渠道/跨 attempt 循环全部结束后才执行，因此「重试后成功」记 1 次成功（C）+ 0 次失败；「重试耗尽」记 1 次失败（A）。→ **在线率天然是「用户视角的最终成败」，不是「上游单次调用成败」**，这正是分组监控想要的口径。
- **上游 4xx/5xx / 超时**：只要最终 `newAPIError != nil` 就记失败，不区分 error code；`ERROR_LOG_ENABLED` 对 perf_metrics **无影响**（那是 `logs` 表的开关，`controller/relay.go:524`）。
- **客户端侧错误也算失败**：请求体校验失败、`GetAndValidateRequest` 失败（`controller/relay.go:118-127`）、预扣费失败（`:178-182`）都会走到 defer 前的 `newAPIError != nil`……**但注意**：这些早退发生在 `:71-206` 区间，`return` 直接跳过了 `:377` 的打点（打点在选路循环之后）。只有进入选路循环后产生的错误才会打点。**即：用户余额不足、模型不存在这类请求不会污染在线率**。
- `Record()` 里 `sample.Group == "" → "default"`（`pkg/perf_metrics/metrics.go:62-64`）；group 取 `info.UsingGroup`（`:47`），auto 跨分组重试时是**最后实际使用的分组**。

---

## Q2 非流式请求的 latency 与 TTFT

### 2.1 `RecordRelaySample` 的字段来源（`pkg/perf_metrics/metrics.go:27-55`）

```go
now := time.Now()
hasTtft := info.IsStream && info.HasSendResponse()          // :32
ttftMs := int64(0)
if hasTtft {
    ttftMs = info.FirstResponseTime.Sub(info.StartTime).Milliseconds()  // :35
}
latencyMs := now.Sub(info.StartTime).Milliseconds()          // :37
generationMs := latencyMs                                    // :38
if hasTtft {
    generationMs = now.Sub(info.FirstResponseTime).Milliseconds()  // :40
}
if generationMs <= 0 { generationMs = latencyMs }            // :42-44
```

累加规则在 `pkg/perf_metrics/types.go:89-105`：

```go
b.requestCount.Add(1)                                  // :90 —— 无条件
if sample.Success { b.successCount.Add(1) }            // :91-93
if sample.LatencyMs > 0 { b.totalLatencyMs.Add(...) }  // :94-96  ← 注意 >0 才加
if sample.HasTtft && sample.TtftMs >= 0 {              // :97-100 ← TtftCount 唯一入口
    b.ttftSumMs.Add(sample.TtftMs); b.ttftCount.Add(1)
}
if sample.OutputTokens > 0 && sample.GenerationMs > 0 { ... }  // :101-104
```

### 2.2 非流式是否累加 TTFT —— **确认：完全不累加**

`hasTtft = info.IsStream && info.HasSendResponse()`。非流式请求 `IsStream=false`（`relay/common/relay_info.go:533`），短路为 `false`：

- `HasTtft=false` → `types.go:97` 条件不成立 → **`TtftSumMs` 和 `TtftCount` 都不增**。
- 因此 `frt` 为负这件事**根本不会流进 perf_metrics**（`FirstResponseTime = startTime.Add(-time.Second)`，`relay/common/relay_info.go:536`；`HasSendResponse()` 判定 `FirstResponseTime.After(StartTime)`，`:856-858`）。负 TTFT 只是 `logs.other.frt` 的问题，不是 perf_metrics 的问题。

**推论**：纯非流式模型（典型：`/v1/images/generations` 不带 `stream`）在 `perf_metrics` 中 `ttft_count = 0`。查询侧 `avg(sum, count)` 在 `count<=0` 时返回 0（`pkg/perf_metrics/metrics.go:359-363`），所以 `avg_ttft_ms` 恒为 **0**，不是 null。

**例外**：生图**开了 stream** 时有 TTFT：
- 真 SSE 流式出图（`relay/channel/openai/relay_image.go:93` `OpenaiImageStreamHandler`）经通用扫描器 `relay/helper/stream_scanner.go:346` 调 `SetFirstResponseTime()`；
- 上游返回 JSON 但客户端要 stream 时走 `relay/channel/openai/relay_image.go:237` `openaiImageJSONAsStreamHandler`，在 **整个响应体已读完之后** 才 `info.SetFirstResponseTime()`（`:268-270`）→ 这里的 "TTFT" ≈ 上游全量耗时，**语义失真**，不宜作为生图的展示指标。

### 2.3 UI 该展示什么

| 模型类型 | 可用指标 | 计算式 | 代码 |
|---|---|---|---|
| 非流式（生图 / embedding / rerank / TTS 等） | **平均总延迟** | `TotalLatencyMs / RequestCount` | `pkg/perf_metrics/metrics.go:335`、`:353`（`avg_latency_ms`） |
| 流式文本 | TTFT + 总延迟 | `TtftSumMs/TtftCount`、`TotalLatencyMs/RequestCount` | `:334-335`、`:352-353` |

**建议的展示口径**：分组监控卡片主指标用 `avg_latency_ms`（对所有类型都有意义），TTFT 作为**次要列**，`ttft_count == 0` 时显示「—」而不是 `0 ms`。当前后端 DTO 只暴露 `avg_ttft_ms`（`pkg/perf_metrics/types.go:29,37`），**不暴露 `ttft_count`**，无法在前端区分「0 ms」和「无 TTFT 样本」——新查询函数需要把 `ttft_count` 或一个 `has_ttft` 布尔带出来。

### 2.4 流式 latency 记的是什么 —— **总耗时**

`latencyMs = now.Sub(info.StartTime)`（`metrics.go:37`），`now` 是 `RecordRelaySample` 被调用的时刻，也就是 `PostTextConsumeQuota` 结算时（响应已完整写完）。**流式和非流式一样，`latencyMs` 都是整请求墙钟耗时**。流式的「生成阶段耗时」单独记在 `generationMs`（`metrics.go:40`），只用于算 TPS（`metrics.go:373-378`）。

**已知小偏差**：`types.go:94` 只在 `LatencyMs > 0` 时累加，但 `requestCount` 无条件 +1。亚毫秒请求（几乎不可能出现在真实 relay）会让平均延迟偏低。可忽略。

---

## Q3 把 BucketTime 改成 5min 的影响面

### 3.1 谁在消费 perf_metrics

**唯一读取入口**：`pkg/perf_metrics` 的 `Query`（`metrics.go:79`）与 `QuerySummaryAll`（`metrics.go:125`），底层分别调 `model.GetPerfMetrics`（`model/perf_metric.go:51`）和 `model.GetPerfMetricsSummaryBucketsAll`（`:99`）。
`GetPerfMetricsSummaryAll`（`:81`）**全仓无调用点**（dead）。

**HTTP 出口**：`controller/perf_metrics.go:14` `GetPerfMetricsSummary`、`:38` `GetPerfMetrics`；路由 `router/api-router.go:45-50`（`/api/perf-metrics`，`HeaderNavModulePublicOrUserAuth("pricing")`）。

**前端消费方（4 处）**：

| 前端位置 | 用的接口 | 用到桶的方式 |
|---|---|---|
| `web/src/features/pricing/components/model-card-grid.tsx:25` | `/summary` | 只用聚合值 + `recent_success_rates` |
| `web/src/features/dashboard/components/models/performance-overview.tsx:26` | `/summary` | 同上 |
| `web/src/features/dashboard/components/overview/performance-health-panel.tsx:26` | `/summary` | 同上 |
| `web/src/features/pricing/components/model-details-performance.tsx:29` + `model-details-charts.tsx` | `/api/perf-metrics`（带 series） | **逐桶渲染** |

### 3.2 改桶宽会破坏的展示（3 处，都在 pricing / 模型广场）

1. **X 轴标签全部塌成 `HH:00`** —— `web/src/features/pricing/components/model-details-charts.tsx:32-36`：
   ```ts
   function formatHourLabel(iso: string): string {
     const date = new Date(iso)
     return `${String(date.getHours()).padStart(2, '0')}:00`
   }
   ```
   `LatencyTrendChart` 用它做 `xField: 'time'` 的**分类轴**（`:107,114`）。24h × 5min = 288 个点，每小时 12 个点标签完全相同 → 分类轴出现 12 组重复 key，折线图会错乱/合并。**必须改成按桶宽输出 `HH:mm`。**

2. **`recent_success_rates` 语义从「最近 3 小时」变成「最近 15 分钟」** —— `pkg/perf_metrics/metrics.go:190` `recentSuccessRates(modelBuckets[name], 3)`（实现在 `:234-253`，取最后 3 个桶）。前端 `web/src/features/pricing/components/model-perf-badge.tsx:64-72` 把它渲染成 3 根状态柱。桶变窄 12 倍后，这 3 根柱子只代表 15 分钟，样本量骤减 → 模型广场徽标会频繁闪红。**应把 `3` 改成按 `GetBucketSeconds()` 折算的桶数（如固定覆盖 3 小时 = 36 个 5min 桶，或改为聚合而非取最后 3 个）。**

3. **每分组 uptime sparkline 从 24 根条变成 288 根条** —— `model-details-performance.tsx:141-151` `toGroupUptimeSeries` 一个桶一根条，`model-details-uptime-sparkline.tsx:93-126` 无采样地全部渲染，`md` 尺寸每根 `w-1` + `gap-[2px]` ≈ 6px → 288 根 ≈ 1730px，表格单元格内会溢出。**必须在前端做降采样。**

**数据层不会破**：所有查询都是 `WHERE bucket_ts BETWEEN ?` + `GROUP BY`（`model/perf_metric.go:103,111`），与桶宽无关；`bucketStart()` 只影响写入侧（`pkg/perf_metrics/metrics.go:266-272`）。**桶宽切换后新旧桶混存**（老的 hour 桶 `bucket_ts` 是整点，新的 5min 桶落在同一时间窗），查询会把它们当成不同的桶各自返回 → 切换当天的图表会出现「一根很高的小时桶 + 一堆 5min 桶」并存的过渡态。这不是错误数据，但过渡期视觉上很怪。

### 3.3 行数增长量化

**推算依据**：行数不是笛卡尔积，而是 **每个桶内「实际有调用」的 (model, group) 对数之和**。
`flushCompletedBuckets` 对 `requestCount == 0` 的桶直接跳过 upsert（`pkg/perf_metrics/flush.go:36-39`），且 `UpsertPerfMetric` 对 `RequestCount == 0` 直接 return（`model/perf_metric.go:30-32`）。

设：
- `P_hot` = 全天几乎每个桶都有流量的热点 (model, group) 对数
- `P_cold` = 稀疏对数，各自每天只在 `k` 个桶里有流量

```
rows/day(hour) = P_hot × 24  + P_cold × min(k, 24)
rows/day(5min) = P_hot × 288 + P_cold × min(k, 288)
```

**场景 B（中型网关：40 个模型 × 5 个分组 = 200 对，其中 60 对全天热，140 对稀疏、每天散落约 30 次调用）**：

| 桶宽 | rows/day | rows/年 | 磁盘/年（250 B/行） |
|---|---|---|---|
| `hour` | 60×24 + 140×20 = **4,240** | 1.55 M | ~390 MB |
| `5min` | 60×288 + 140×30 = **21,480** | 7.84 M | **~1.96 GB** |

**场景 A（小型：10 模型 × 3 分组，30 对全热）**：
- `hour`: 720 rows/day → 263 K/年
- `5min`: 8,640 rows/day → 3.15 M/年 → ~790 MB/年

行宽推算：`id int4` + `model_name varchar(128)`（实际均值约 30 B）+ `group varchar(64)`（约 12 B）+ `bucket_ts` 与 7 个计数列共 8 × `int8` = 64 B ≈ **110 B 净数据**；加 PK + `idx_perf_model_group_bucket`（复合唯一）+ `idx_perf_bucket_ts` 三套索引，按 InnoDB 常见 2.2–2.5x 膨胀取 **~250 B/行**。

**增长倍数不是 12x 而是约 5x**：热点对按 12x 增长，稀疏对基本不变（一天调 30 次，无论 hour 还是 5min 都只产生 ≤30 行）。

**是否必须同时设保留期 —— 是。**
`RetentionDays` 默认 `0`（`setting/perf_metrics_setting/config.go:16`），`cleanupExpiredMetrics` 在 `retentionDays <= 0` 时直接 return（`pkg/perf_metrics/flush.go:70-72`），**永不清理**。5min 桶 + 永不清理 = 场景 B 三年后近 6 GB，SQLite 部署尤其吃不消。

- 分组监控窗口固定 24h，`perfmetrics.Query` 的 `hours` 上限是 30 天（`pkg/perf_metrics/metrics.go:83-85`）。
- **建议 `RetentionDays` 默认改为 `7`**（覆盖分组监控 24h 窗口且留足余量，场景 B 仅 150 K 行 ≈ 38 MB）；若要保住 API 的 30 天最大窗口则设 `30`（644 K 行 ≈ 161 MB）。
- **切换时的一次性风险**：如果先跑了很久 `RetentionDays=0` 再打开保留期，第一次 `DeletePerfMetricsBefore`（`model/perf_metric.go:118-123`）是一条无 LIMIT 的 `DELETE`，可能一次删掉数百万行。SQLite/MySQL 上会长时间持锁。建议首次清理前手动分批删。

### 3.4 FlushInterval 与桶宽的关系

`flushLoop`（`pkg/perf_metrics/flush.go:13-24`）：`Sleep(FlushInterval 分钟)` → `flushCompletedBuckets()`。
`flushCompletedBuckets`（`:26-62`）：遍历**全部** hot bucket，跳过 `bucketTs >= currentBucket`（`:30-32`），其余 `drain()` 后 upsert。

- **不要求 flush 间隔 ≤ 桶宽**。`Range` 每次扫的是所有已关闭的桶，即使一次 sleep 期间关闭了 12 个桶，也会在同一次 flush 里全部写出去。**不会漏 flush**。
- **不会重复**：`drain()` 用 `Swap(0)` 原子取走并清零（`pkg/perf_metrics/types.go:119-129`）；upsert 失败时用 `addCounters` 加回内存桶（`flush.go:53-57`），语义正确。
- **唯一影响是可见性延迟**：一个桶关闭后，最坏要等 `FlushInterval` 才落库。默认 `FlushInterval=5` + `5min` 桶 → 已关闭的桶最迟 5 分钟后落库。
- **副作用：hotBuckets 常驻 24 小时**。`deleteOldEmptyBucket`（`flush.go:64-68`）只在 `bucketTs < bucketStart(now-24h)` 时才从 `sync.Map` 删除。桶宽 12x 变细 → 常驻 entry 数 12x。场景 B 下从约 1,800 涨到约 21,500 个 entry（内存约 3.4 MB，可忽略），但 **每次 `/api/perf-metrics` 和 `/api/perf-metrics/summary` 请求都会做一次全量 `hotBuckets.Range`**（`metrics.go:110`、`:155`），扫描量同步 12x。绝对值仍很小，记录备查。

### 3.5 替代方案对比 —— 推荐

| 方案 | 做法 | 代价 | 风险 |
|---|---|---|---|
| **① 改全局 `BucketTime` 为 `5min`** | 只动 `setting/perf_metrics_setting/config.go:15` 默认值 | 必须修 3.2 的 3 个 pricing 展示点；必须设 `RetentionDays`；行数 ~5x | **`bucket_time` 是管理员可改选项**（`web/src/features/system-settings/integrations/monitoring-settings-section.tsx:68,262-268` 提供 `minute/5min/hour` 下拉）。管理员改回 `hour`，分组监控的 5/15/30 分钟档就**物理上无法实现**（不能把小时桶拆开），页面会静默降级或撒谎 |
| **② 分组监控独立打点到自己的桶表** | 新表 + 新 flush loop + 在同样 3 个调用点再打一次 | 维度 `(model, group, bucket_ts)` 与 `perf_metrics` **完全相同**，样本也完全相同，等于把整条管线复制一份；还要再写一份保留期清理；与本任务 Non-Goals「不修改 perf_metrics 打点逻辑」冲突（仍要动那 3 个调用点） | 双份数据长期漂移；违反 `.trellis/spec/guides/code-reuse-thinking-guide.md` 的复用判断 |
| **③（推荐）单表单管线 + 把桶宽从「管理员开关」降级为「系统下限」** | 默认 `BucketTime` 改 `5min`；同时让分组监控读 `perf_metrics_setting.GetBucketSeconds()`，**只提供 ≥ 底层桶宽且是其整数倍的展示档位**（底层 5min → 提供 5/15/30/60；底层 hour → 只提供 60） | 修 3.2 的 3 个展示点 + `RetentionDays` 默认值 + 分组监控档位做成派生 | 无静默撒谎：底层桶宽变粗时分组监控明确减少可选档位 |

**推荐 ③**。理由：

1. `perf_metrics` 的维度 `(model_name, group, bucket_ts)`（`model/perf_metric.go:13-15`）与分组监控要的「分组 + 模型 + 时间线」**逐字对应**，另建表纯属复制，没有任何新增信息。
2. 方案 ② 并不能避开「改 3 个打点调用点」这件事，反而多一条 flush/清理管线，长期维护面翻倍。
3. 方案 ① 的真实风险不是行数（可用保留期解决），而是**桶宽是一个可被管理员反向修改的开关**。把展示档位做成从 `GetBucketSeconds()` 派生（③），这个风险就变成可见的功能降级而不是数据错误。
4. 行数 ~5x + 7 天保留期后绝对量只有 15 万行级别，不构成方案取舍的决定因素。

---

## Q4 「合并小桶成大桶」是否可行

### 4.1 `GetPerfMetricsSummaryBucketsAll` 的签名与返回

`model/perf_metric.go:99-116`：

```go
func GetPerfMetricsSummaryBucketsAll(startTs int64, endTs int64, groups []string) ([]PerfMetricSummaryBucket, error) {
    query := DB.Model(&PerfMetric{}).
        Select("model_name, bucket_ts, SUM(request_count) as request_count, SUM(success_count) as success_count, SUM(total_latency_ms) as total_latency_ms, SUM(output_tokens) as output_tokens, SUM(generation_ms) as generation_ms").
        Where("bucket_ts >= ? AND bucket_ts <= ?", startTs, endTs)
    if groups != nil {
        if len(groups) == 0 { return summaries, nil }
        query = query.Where(commonGroupCol+" IN ?", groups)
    }
    err := query.Group("model_name, bucket_ts").Having("SUM(request_count) > 0").Order("bucket_ts ASC").Find(&summaries).Error
```

返回结构 `model/perf_metric.go:71-79`：

```go
type PerfMetricSummaryBucket struct {
    ModelName, BucketTs, RequestCount, SuccessCount, TotalLatencyMs, OutputTokens, GenerationMs
}
```

### 4.2 结论 —— **支撑不了，必须新写查询函数**

三个硬伤：

1. **没有 group 维度**。`groups` 参数只做 `WHERE ... IN` 过滤，`GROUP BY model_name, bucket_ts`（`:111`）把所有分组**塌成一行**。分组监控的核心就是「按分组区分」，这个函数从结构上给不出来。
2. **没有 TTFT**。`Select` 里没有 `ttft_sum_ms` / `ttft_count`（对比 `PerfMetric` 结构体 `model/perf_metric.go:19-20` 是有列的）。R1 明确要 TTFT。
3. **没有分组维度就无法做「无数据」判定**。R1 要求「某分组+模型窗口内无调用时显示无数据」，需要 per-(group, model) 的 requestCount。

另一个已有函数 `GetPerfMetrics(modelName, group, startTs, endTs)`（`:51-60`）返回完整 `PerfMetric` 行（含 ttft），但**一次只能查一个 model**，K 个监控模型要 K 次查询。可用但不优雅。

### 4.3 需要新写的查询

```go
// model/perf_metric.go 新增
type PerfMetricGroupBucket struct {
    ModelName      string
    Group          string   // gorm 需要 column:group，读取侧用 commonGroupCol 拼 Select
    BucketTs       int64
    RequestCount, SuccessCount, TotalLatencyMs, TtftSumMs, TtftCount int64
}

func GetPerfMetricsGroupBuckets(models []string, groups []string, startTs, endTs int64) ([]PerfMetricGroupBucket, error)
// Select: model_name, <commonGroupCol> as "group", bucket_ts, SUM(request_count), SUM(success_count),
//         SUM(total_latency_ms), SUM(ttft_sum_ms), SUM(ttft_count)
// Where:  bucket_ts BETWEEN ? AND ?  AND model_name IN ?  AND <commonGroupCol> IN ?
// Group:  model_name, <commonGroupCol>, bucket_ts
// Having: SUM(request_count) > 0
```

注意点：
- `group` 是保留字，**必须用 `commonGroupCol`**（`model/main.go:33,38`），已有先例 `model/perf_metric.go:56,90,108`。
- 底层其实每个 `(model, group, bucket_ts)` 只有一行（唯一索引 `idx_perf_model_group_bucket`，`:13-15`），`SUM(...)` 只是形式上的；也可以直接 `Find(&[]PerfMetric{})` + `WHERE model_name IN ? AND group IN ?` 拿原始行，更简单。
- **索引**：`WHERE bucket_ts BETWEEN` 走 `idx_perf_bucket_ts`（`:15`），24h 窗口在 5min 桶下约 2 万行，可接受。

### 4.4 合并到展示桶（在 Go 侧）

```go
displayTs := bucketTs - bucketTs % displaySeconds   // displaySeconds ∈ {300, 900, 1800, 3600}
```

与项目既有约定一致（`pkg/perf_metrics/metrics.go:266-272`、`model/usedata.go:80`），**不写 SQL 侧时间分桶**（跨 SQLite/MySQL/PG 的 `/` `%` 语义不一致，见 `log-aggregation-feasibility.md` §5.1）。

**合并必须累加计数器再算率，不能平均率**：

```
success_rate = Σsuccess_count / Σrequest_count      （不是 avg(rate_i)）
avg_latency  = Σtotal_latency_ms / Σrequest_count
avg_ttft     = Σttft_sum_ms / Σttft_count           （Σttft_count == 0 → 无数据）
```

现有 `mergeCounters`（`metrics.go:274-287`）/ `mergeModelBucket`（`:216-232`）已经是「先累加 counters 再算率」的正确范式，可直接照抄。
**反例（不要抄）**：前端 `model-details-performance.tsx:127-131` 是对多个分组的 `success_rate` 做算术平均，这在样本量不等时是错的。

**约束**：`displaySeconds` 必须是 `GetBucketSeconds()` 的整数倍，否则展示桶边界会切开底层桶。5/15/30/60 分钟对 300s 底层桶都成立（1/3/6/12 倍）；对 3600s 底层桶只有 60 分钟成立。

---

## Q5 多节点当前桶的实时性

### 5.1 事实确认

- `recordRedis`（`pkg/perf_metrics/metrics.go:380-406`）在每次采样时 `HIncrBy` 写 `perf:{model}:{group}:{bucketTs}`，TTL 1 小时（`:404`）。
- `mergeRedisActiveBuckets`（`:408-424`）是**唯一的读取方，全仓库没有任何调用点**（`grep -rn "mergeRedisActiveBuckets"` 只命中定义处）。`redisCounters`（`flush.go:80`）同理只被这个 dead 函数用。
- 因此 **Redis 里的计数从写入到过期，全程无人读取**。
- `flushCompletedBuckets` 明确跳过当前桶（`flush.go:30-32`），当前桶**永远不在 DB 里**。
- `perfmetrics.Init()` 在 `main.go:338` 无条件调用，**不区分 master/slave**，每个节点都跑自己的 `flushLoop`，各自 flush 自己进程内的 `hotBuckets`，靠 `UpsertPerfMetric` 的 `OnConflict ... col + ?`（`model/perf_metric.go:33-48`）累加汇总。历史桶的多节点合并是正确的。

### 5.2 多节点部署下「当前未 flush 的桶」在页面上的表现

处理 API 请求的那个节点，会把 DB 行与**自己进程内**的 `hotBuckets` 合并（`Query`: `metrics.go:110-120`；`QuerySummaryAll`: `:155-172`）。所以当前桶的表现取决于该节点自己收到了多少流量：

| 情形 | 表现 |
|---|---|
| N 个节点负载均衡，本节点收到 1/N 流量 | 当前桶**只有本节点数据**，`request_count` 约为真实值的 1/N。成功率是本节点样本的成功率 —— 小样本噪声大 |
| 本节点在当前桶内该 (model, group) 零请求 | `requestCount == 0` → `mergeModelBucket`（`:216-219`）与 `buildQueryResult`（`:291-293`）都直接跳过 → **该桶整体不出现在 series 里**（缺失，不是 0） |
| 单节点部署 | 当前桶完整、实时。**这是最容易掩盖问题的场景**：开发/测试环境一切正常，生产多节点才出问题 |
| 已关闭但尚未 flush 的桶（其他节点的部分） | 同样只有本节点数据，最长滞后 `FlushInterval`（默认 5 分钟） |

### 5.3 会不会让最新的一两个桶看起来像「故障」—— 会

三重放大：

1. **桶本身未完成**：5 分钟桶在 t+30s 被查询时，只累积了 1/10 的样本。
2. **多节点只见本节点**：样本再除以 N。
3. **现有着色规则对小样本极敏感**：`web/src/features/group-monitoring/lib/status.ts:12-20`
   ```ts
   if (bucket.total_count === 0) return 'no-data'
   const rate = bucket.success_count / bucket.total_count
   if (rate >= 0.99) return 'healthy'
   if (rate >= 0.8)  return 'degraded'
   return 'down'
   ```
   → **2 个请求里失败 1 个 = 50% = `down`（红）**。真实在线率可能是 99.5%。

即：**最新 1–2 个桶会周期性地闪红**，尤其是低流量的 (分组 + 模型) 组合。

### 5.4 规避建议

按性价比排序：

1. **丢弃/降级最后一个未完成的展示桶（最低成本，强烈建议）**
   时间线不渲染 `displayTs + displaySeconds > now` 的桶，或渲染为独立的「采集中」灰色态（不参与红/黄/绿判定，也不计入卡片汇总在线率）。
   若 `displaySeconds < FlushInterval*60`（例如展示 5 分钟桶 + flush 间隔 5 分钟），多节点下**倒数第二个桶也可能不完整**，此时应丢弃 `ceil(FlushInterval*60 / displaySeconds)` 个尾桶，或把 `FlushInterval` 默认改为 1 分钟。

2. **最小样本量门槛（对低流量分组必需）**
   在 `getBucketState` 前加一层：`total_count < N`（如 N=5）时返回 `no-data`/`low-sample`，不着红。这条对**所有**桶都有效，不只是最后一个。

3. **卡片主指标用整个 24h 窗口，不用最新桶**
   在线率大数字取窗口累加值（`Σsuccess / Σrequest`），最新桶的抖动只影响色条最右一两格，不影响头部数字。

4.（可选，成本较高）**复活 `mergeRedisActiveBuckets`**
   需要注意两点：(a) 本节点的样本同时存在于 `hotBuckets` 和 Redis，直接 merge 会**双计**，必须二选一；(b) Redis TTL 只有 1 小时（`metrics.go:404`），只能覆盖最近的桶；(c) `Redis` 未启用时无回退。本任务 Non-Goals 已明确不修这个 dead path，建议走方案 1+2。

---

## Caveats / 未查证

- 行数与磁盘估算基于「典型部署」假设的 (model, group) 活跃对数，**未查询任何真实数据库**；推算式与假设已在 §3.3 写明，主 agent 可用真实 `SELECT COUNT(*) FROM perf_metrics` 校准。
- 250 B/行 的行宽是按 InnoDB + 3 套索引的常见膨胀系数估的，未实测；PostgreSQL 因 24 B 元组头会更大，SQLite 更小。
- §3.2 的 3 个前端破坏点是通过读代码判定的，**未实际把 `bucket_time` 改成 `5min` 跑前端验证**。
- 未验证 VChart 分类轴在重复 x 值下的确切行为（重复 key 是合并、覆盖还是报错），只确认了 `formatHourLabel` 必然产生重复 key。
- 未逐一核对 `relay/channel/**` 下每个 adaptor 的 `DoResponse` 是否都会走到 `Post*ConsumeQuota`；Q1 的判定基于 `relay/*_handler.go` 这一层的调用点。
- 工作区有大量其他 session 未提交改动，本次调研**全程只读**，未执行任何 git 写操作。
