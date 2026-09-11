# Research: 渠道级可用性数据的来源、扩维成本与替代方案

- **Query**: 新增「仅管理员可见的渠道可用性监控页」需要的渠道级时序数据从哪来；给 `perf_metrics` 加 `channel_id` 的完整成本；7 天范围怎么支持；有无更合适的替代方案；管理员权限的标准做法
- **Scope**: internal
- **Date**: 2026-09-10
- **前序结论（未重复验证）**: `research/current-group-monitor.md`、`research/log-aggregation-feasibility.md`（`logs` 表不可依赖、分组监控现状）

---

## 0. 结论速览

| 问题 | 结论 |
|---|---|
| 现成的渠道级**时序**可用性数据 | **不存在任何一张按渠道分桶的时序表**。只有 3 类「当前值」：`channels.test_time/response_time`（最近一次合成测试）、`channels.other_info.auto_ban_failures/auto_ban_until`（连续失败计数，成功即清零）、`channels.status/used_quota`（累计值） |
| 唯一带 `channel_id` 的聚合表 | `quota_data`（小时桶，`model/usedata.go:13-26`），但**只记成功消费日志、没有失败数**，且受 `DataExportEnabled` 开关控制 → 算不出可用率 |
| `perf_metrics` 加 `channel_id` | 技术上可行，但有 **3 个硬伤**：① 唯一索引变更在 AutoMigrate 下**不会生效**（GORM 只按索引名建，不改已有索引），MySQL 会**静默把不同渠道的数据合并进同一行**；② 失败打点是**每请求一次、只归属最后一个渠道**，failover 场景下故障渠道的失败被隐藏；③ `perf_metrics` 走的是**准公开**接口（`pricing` nav 模块），channel 维度进去有泄露面 |
| 推荐方案 | **新建独立的 `channel_metrics`（channel_id, bucket_ts）桶表**，在 relay 的**每次 attempt**上打点（`controller/relay.go` 的重试循环 + `RelayTask`/`RelayMidjourney`），与 `perf_metrics` 并存、各自打点。行数与模型数/分组数**解耦**，7d@5min 上界 = 渠道数 × 2016 行 |
| 7 天范围 | 项目**没有多级桶（5min/1h/1d）先例**，`BucketTime` 是全局单档配置。7d 用同一张 5min 桶表 + **查询期再分桶**（`(bucket_ts / step) * step`，先例见 `model/usedata_rankings.go:51-56`，PG 用 `FLOOR`） |
| 管理员权限 | 后端 `middleware.AdminAuth()`（`middleware/auth.go:98-102`），前端 TanStack Router `beforeLoad` 判 `auth.user.role < ROLE.ADMIN` → `redirect('/403')`（`web/src/routes/_authenticated/channels/index.tsx:36-44`），侧边栏 `admin` 分组由 `use-sidebar-view.ts:56-58` 按角色过滤 |

---

## 1. 渠道级可用性数据现有来源全排查

### 1.1 `model/channel.go` 上的相关字段

| 字段 | 位置 | 语义 | 更新时机 | 能否做时间线 |
|---|---|---|---|---|
| `TestTime int64` | `model/channel.go:35` | 最近一次**渠道测试**的 unix 秒 | `UpdateResponseTime`（`model/channel.go:869-875`），由 `controller/channel-test.go:889`（手动测试）/ `:959`（定时测试）调用 | ❌ 只有一个最新值，覆盖写 |
| `ResponseTime int` | `model/channel.go:36` | 最近一次测试的整请求毫秒数 | 同上 | ❌ 同上 |
| `Status int` | `model/channel.go:31` | 1=启用 / 2=手动禁用 / 3=自动禁用 | 自动封禁、手动改、自动恢复 | ❌ 状态位，无历史 |
| `UsedQuota int64` | `model/channel.go:44` | 累计消耗额度 | 计费结算累加 | ❌ 单调累计，无时间维 |
| `OtherInfo.auto_ban_failures` | `model/channel.go:373,403-409` | **连续**失败次数（非窗口计数） | 失败 +1（`RecordChannelAutoBanFailure`，`model/channel.go:411-473`），成功清零（`ClearChannelAutoBanFailures`，`:475`） | ❌ 成功即归零，无法反推失败率 |
| `OtherInfo.auto_ban_until` | `model/channel.go:374,384-401` | 自动封禁到期时间 | 达阈值时写入（`:442-449`） | ❌ 点值 |
| `OtherInfo.status_reason/status_time` | `model/channel.go:446-447` | 最近一次禁用原因/时间 | 同上 | ❌ 点值 |

结论：**`channels` 表上没有任何可用于「24 小时时间线」的历史数据**，`response_time` 还只是合成测试的墙钟耗时（和真实流量无关）。

### 1.2 渠道级熔断/健康度设施

- **自动封禁（auto-ban）**：`service/channel.go:17-44`（`RecordAutoBanFailure` / `RecordAutoBanSuccess`）→ `model/channel.go:411-473`。阈值/时长来自 `channel.GetReliabilitySettings()`（`model/channel.go:1325-1331`）。**只保存「当前连续失败数」**，不保存历史。
- **禁用判定**：`service.ShouldDisableChannel`（`service/channel.go:77-97`，按状态码/关键词），`DisableChannel` / `EnableChannel`（`:51-75`）。
- **重试 / failover**：`controller/relay.go:223-369` 的 resilient route 循环。每次 attempt 的结果被记录进 **`service.RouteState`**（`service/route_state.go:14-64`）：
  ```go
  type RouteAttempt struct {
      ChannelID int; ChannelName string; Attempt int
      Outcome   RouteAttemptOutcome // succeeded / retrying_channel / channel_exhausted / stopped
      Reason    string; StatusCode int; Priority int64; Weight int
  }
  ```
  这是**全仓唯一「每个渠道每次尝试成功/失败」的完整信息**，但它只活在内存里，最终被 JSON 化写进日志 `other.route_history`（`controller/relay.go:398-409`、`:566-568`）。单请求最多 20 个不同渠道（`service/route_state.go:3` `MaxDistinctChannelsPerRequest = 20`）。
- **渠道选择**：`service/channel_select.go:84 CacheGetRandomSatisfiedChannel` / `:168 cacheGetResilientChannel`；`middleware/distributor.go` 里的渠道禁用/亲和逻辑在 `IsChannelTest` 时旁路（见 `current-group-monitor.md` §2.3）。**没有内存/Redis 里的滑动窗口失败率或熔断器。**

### 1.3 按渠道维度的时序/桶表

全仓表清单（`grep "func (.*) TableName"` + `model/main.go:310-344` AutoMigrate 列表）中，带时间桶的只有 3 张：

| 表 | 桶键 | 有 channel_id? | 有失败数? | 位置 |
|---|---|---|---|---|
| `perf_metrics` | (model_name, group, bucket_ts) | ❌ | ✅ `request_count`/`success_count` | `model/perf_metric.go:11-23` |
| `quota_data` | (user_id, username, model_name, created_at 小时, use_group, token_id, **channel_id**, node_name) | ✅ | ❌ 只有成功消费的 `count/quota/token_used` | `model/usedata.go:13-26,78-98` |
| `group_monitoring_results` | 明细行（非桶），带 `ChannelID` | ✅ | ✅ 但只是探测结果 | `model/group_monitoring.go:3-16` |

- `quota_data` 的写入点只有 `model/log.go:492` 和 `:555`（都在 `RecordConsumeLog` 成功路径内），且整体受 `common.DataExportEnabled` 控制（`model/usedata.go:41-49`）。**失败请求永远不进这张表 → 无法计算可用率。** 它的键还含 `user_id/username/token_id`，基数远高于监控所需。
- `group_monitoring_results.ChannelID` 是「本次探测最终落到哪个渠道」，一个探测周期每分组只有 1 行，**不能覆盖全部渠道**。

**结论：渠道级可用性没有现成数据源，必须新增打点。**

---

## 2. 给 `perf_metrics` 加 `channel_id` 的完整成本

### 2.1 表结构与唯一索引：AutoMigrate 在三库上**不安全**

现状（`model/perf_metric.go:11-23`）：

```go
ModelName string `gorm:"size:128;uniqueIndex:idx_perf_model_group_bucket,priority:1"`
Group     string `gorm:"column:group;size:64;uniqueIndex:idx_perf_model_group_bucket,priority:2"`
BucketTs  int64  `gorm:"uniqueIndex:idx_perf_model_group_bucket,priority:3;index:idx_perf_bucket_ts"`
```

Upsert 依赖这个唯一索引（`model/perf_metric.go:33-48`，`clause.OnConflict{Columns: model_name, group, bucket_ts}`）。

**关键事实**：GORM v1.25.2 的 `AutoMigrate` 对索引只做「按名字判断存在与否，不存在才建」，**永远不会修改/重建同名索引**：

```
~/go/pkg/mod/gorm.io/gorm@v1.25.2/migrator/migrator.go:171-177
for _, idx := range parseIndexes {
    if !queryTx.Migrator().HasIndex(value, idx.Name) {
        if err := execTx.Migrator().CreateIndex(value, idx.Name); err != nil { ... }
    }
}
```

本仓库**没有任何 DropIndex/RenameIndex 的迁移先例**（`grep DropIndex/CreateIndex model/` 只命中 `HasTable`/`HasColumn`，`model/main.go:554,639,643,694,699`）。

所以如果直接把 `channel_id` 加进 `idx_perf_model_group_bucket`：

| 数据库 | 老库（索引已存在）上的实际行为 |
|---|---|
| **MySQL** | 驱动把 `clause.OnConflict` 编译成 `ON DUPLICATE KEY UPDATE`，**完全忽略 `Columns` 列表**（`gorm.io/driver/mysql@v1.4.3/mysql.go:185-192`）。旧的 3 列唯一键仍在 → 不同 `channel_id`、相同 (model, group, bucket) 的插入会命中旧唯一键，**把别的渠道的计数累加到同一行**。静默数据错误，最难发现的一种 |
| **PostgreSQL** | `ON CONFLICT (model_name,"group",bucket_ts,channel_id)` 找不到匹配的唯一索引 → 运行时报错 `42P10 there is no unique or exclusion constraint matching the ON CONFLICT specification`，每次 flush 都失败 |
| **SQLite** | 同 PG，`ON CONFLICT` 的冲突目标必须匹配一个唯一索引，否则报错 |
| 新库（全新部署） | 正常，因为索引是按新定义首次创建的 |

要正确落地，必须显式写迁移：新索引换名（例如 `idx_perf_model_group_channel_bucket`）+ 显式 `DB.Migrator().DropIndex(&PerfMetric{}, "idx_perf_model_group_bucket")`，并处理「老数据没有 channel_id（0 值）与新数据混存」的语义。三库都要验证（PG 的 `uniqueIndex` tag 生成的是 unique index 而非 constraint，`DROP INDEX` 可行）。**这是本方案最大的实施风险，不是「加个列」那么轻。**

### 2.2 打点侧：channel_id 拿得到，但有 nil 指针与归属两个问题

`RecordRelaySample` 全仓 3 个调用点：

| 位置 | 场景 | 是否有 channel |
|---|---|---|
| `controller/relay.go:377-381` | **失败**（整个 resilient route 走完仍失败） | 可能没有 |
| `service/quota.go:379-383` | 成功（非流式 / 通用结算） | 有 |
| `service/text_quota.go:543-547` | 成功（文本流式结算） | 有 |

- 取值来源：`relayInfo.ChannelId`，来自嵌入指针 `*ChannelMeta`（`relay/common/relay_info.go:186`、字段定义 `:58-60`），由 `InitChannelMeta` 赋值（`:190-238`）。同一个值在两个成功路径已经被用于写日志：`ChannelId: relayInfo.ChannelId`（`service/quota.go:366`、`service/text_quota.go:530`）。
- **⚠️ nil 指针**：`ChannelMeta` 只在各 relay handler 里初始化（`relay/*_handler.go` 共 14 处，如 `relay/compatible_handler.go:26`），`GenRelayInfo` **不初始化**。当失败发生在选渠道之前（`getChannel` 返回错误，`controller/relay.go:226-233` 直接 break），`relayInfo.ChannelMeta == nil`。当前 `RecordRelaySample` 只读 `OriginModelName`/`UsingGroup`（`pkg/perf_metrics/metrics.go:45-54`）所以安全；**一旦改成读 `info.ChannelId` 就会 panic**。仓库里其他读取处都有显式 nil 检查（`relay/common/override.go:437,444`、`relay/common/relay_info.go:750,759,766`、`relay/helper/model_mapped.go:12-13`）。
- **⚠️ 归属语义**：失败打点是**每请求一次**（`controller/relay.go:377`，在整个重试循环之外），只带最后一个渠道。典型 failover：渠道 A 500 → 重试渠道 B 成功 → 只产生**一条成功样本记在 B**，A 的失败**完全不可见**。用这套数据画「渠道可用率」会系统性高估故障渠道的可用率（除非该渠道是最后一个被尝试的）。
- **⚠️ 覆盖面**：任务类 relay（`RelayTask` 在 `controller/relay.go:659-785`、`RelayMidjourney` 在 `:577`）有自己的重试循环，既没有 `RouteState` 也没有任何 perf 打点 → MJ/Suno/视频渠道在监控页上永远是「无数据」。

### 2.3 基数爆炸评估

**推算依据（可核对的事实）**：

1. 一个桶内的行数上界 = 该桶内**有流量**的键组合数。
2. 现在的键是 `(model, group)`；加维后是 `(model, group, channel)`。
3. `(group, model, channel_id)` 三元组的全集就是 `abilities` 表的主键（`model/ability.go:18-26`：`Group`/`Model`/`ChannelId` 三列组成复合主键）。所以**加维后每桶行数 ≤ `SELECT COUNT(*) FROM abilities WHERE enabled = true`**，同时 ≤ 该桶内的请求数。
4. 放大倍数 `A = 每桶内平均「服务同一 (model, group) 的不同渠道数」`，`1 ≤ A ≤ 该 (model,group) 的候选渠道数 c`，且 `A ≤ 该桶内该 (model,group) 的请求数 n`。均匀选择时 `E[A] = c·(1-(1-1/c)^n)`；本项目是**优先级优先**的选择（`service/channel_select.go:84,168`，priority/weight 排序），流量集中在最高优先级渠道，实际 A 会明显小于 c。
5. 桶大小默认是 **1 小时**（`setting/perf_metrics_setting/config.go:15` `BucketTime: "hour"`，`:27-38` 只支持 `minute`/`5min`/`hour` 三档）。24h@1h=24 桶，24h@5min=288 桶，7d@5min=**2016** 桶，7d@1h=168 桶。

**算式**：`行数 = 桶数 × 每桶活跃键数`。

**场景化估算**（假设值已标注，真实值需用下方 SQL 核对）：

| 假设（需核对） | 记号 | 取值 |
|---|---|---|
| 每桶有流量的 (model, group) 对 | K | 100 |
| 平均放大倍数 | A | 2（候选渠道 c≈3，优先级集中） |
| `abilities` 启用行数（绝对上界） | N_ab | 2000 |

| 窗口 / 桶 | 现在（K×桶数） | 加 channel（K×A×桶数） | 绝对上界（N_ab×桶数） |
|---|---|---|---|
| 24h @ 1h（当前默认） | 2.4k | 4.8k | 48k |
| 24h @ 5min | 28.8k | 57.6k | 576k |
| 7d @ 5min | 201.6k | **403k** | **4.03M** |
| 7d @ 1h | 16.8k | 33.6k | 336k |

行宽约 7 个 int64 + 两个 varchar + 索引 ≈ 100–150 B/行（`model/perf_metric.go:11-23`），7d@5min 加维后约 **40–90 MB**，绝对上界情形 **0.4–0.6 GB**。

**核对用 SQL（只读，交给运维在生产上跑）**：

```sql
-- 绝对上界的基数
SELECT COUNT(*) FROM abilities WHERE enabled = true;
-- 当前每桶实际活跃键数（把 <ts> 换成最近一个完整桶）
SELECT COUNT(*) FROM perf_metrics WHERE bucket_ts = <ts>;
-- 现表总量与时间跨度
SELECT COUNT(*), MIN(bucket_ts), MAX(bucket_ts) FROM perf_metrics;
-- 渠道数（替代方案的上界）
SELECT COUNT(*) FROM channels WHERE status = 1;
```

**另外两处同样被放大的地方（容易漏）**：

- 内存热桶 `hotBuckets sync.Map`，键是 `bucketKey{model, group, bucketTs}`（`pkg/perf_metrics/types.go:63-67`，`metrics.go:17,69-75`）。加 channel 后**进程内 key 数同比例放大**，且已 flush 的空桶要等 24 小时才删（`pkg/perf_metrics/flush.go:64-68` `deleteOldEmptyBucket`）→ 内存驻留 key ≈ 24h 内出现过的全部键组合。
- Redis 每个 bucketKey 一个 hash（`pkg/perf_metrics/metrics.go:380-406,426-428`，`perf:<model>:<group>:<ts>`，TTL 1h）。加 channel 后 Redis key 数同比例放大。注意：写它的 `recordRedis` 有调用，但**读它的 `mergeRedisActiveBuckets`（`metrics.go:408-424`）全仓没有任何调用方** —— 目前这部分 Redis 写入实际是死数据。

### 2.4 清理 / 保留策略

- 清理函数：`pkg/perf_metrics/flush.go:70-78 cleanupExpiredMetrics` → `model.DeletePerfMetricsBefore`（`model/perf_metric.go:118-123`）。
- 触发：只在 `flushLoop` 里，每个 `FlushInterval`（默认 5 分钟，`config.go:14`）跑一次（`flush.go:13-24`）。`perfmetrics.Init()` 在 `main.go:338` 无条件启动，**不限于 master 节点**。
- **默认 `RetentionDays: 0`（`setting/perf_metrics_setting/config.go:16`），而 `cleanupExpiredMetrics` 在 `retentionDays <= 0` 时直接 return（`flush.go:71-73`）→ 默认配置下 `perf_metrics` 永不清理，无限增长。**
- 含义：加维等于在一张**本来就没有保留策略**的表上乘一个放大系数。加维就必须同时给 `RetentionDays` 设非零默认值（这是行为变更，会删掉老数据，需要显式决策）。

---

## 3. 7 天范围怎么支持

- **行数**：5 分钟桶 7 天 = `7×24×12 = 2016` 桶。行数见 §2.3 表。
- **有没有多级桶先例**：**没有**。`BucketTime` 是**全局单一配置**（`setting/perf_metrics_setting/config.go:8,27-38`），`bucketStart()` 是唯一的分桶函数（`pkg/perf_metrics/metrics.go:266-272`），内存里也只有一张 `hotBuckets`。全仓没有 rollup / 降采样 / 多分辨率表。
- **现成的「查询期再分桶」先例**（推荐用于 7d）：`model/usedata_rankings.go:34-56`，对原始行做整数除法分桶并按方言分支：

  ```go
  func rankingBucketExpr(bucketSize int64) string {
      if common.UsingMainDatabase(common.DatabaseTypePostgreSQL) {
          return fmt.Sprintf("FLOOR(created_at / %d) * %d", bucketSize, bucketSize)
      }
      return fmt.Sprintf("(created_at / %d) * %d", bucketSize, bucketSize)
  }
  ```
  （PG 的整数除法是取整但类型不同，所以显式 `FLOOR`；MySQL/SQLite 直接整除。）

- **另一个先例（纯内存再分桶）**：分组监控把明细行在应用层聚合成固定 32 桶（`controller/group_monitoring.go:92-145`），前端也是固定 32 列（`group-status-card.tsx:119`）。UI 上一条 24h 时间线的点数是固定的（几十个），所以**存储桶粒度和展示桶粒度可以解耦**：存 5min，展示时按 `窗口/点数` 再聚合。
- 所以 7 天的可行做法（不需要新增多级桶表）：
  1. 存储统一 5min 桶；
  2. 查询按 `range` 选 `step`（15m→5min、24h→30min、7d→2h 之类），SQL 里 `GROUP BY channel_id, (bucket_ts/step)*step`，用上面的方言分支；
  3. 或者行数确实大时改为「7d 走 1h 桶查询」——但要求存储桶 ≤ 1h 且能整除。

---

## 4. 替代方案对比

### 方案 A：给 `perf_metrics` 加 `channel_id`

- 改动点：`model/perf_metric.go`（列 + 唯一索引 + upsert + 所有 Select/Group）、`pkg/perf_metrics/{types,metrics,flush}.go`（bucketKey / Sample / Redis key / 三个查询函数）、3 个打点调用点、显式索引迁移。
- 风险：§2.1 的三库唯一索引问题（MySQL 静默串数据）；§2.2 的 nil 指针 + failover 归属失真 + 任务类无覆盖；§2.3 的内存/Redis 同比例放大；§2.4 默认无保留策略。
- 额外泄露面：`/api/perf-metrics` 和 `/api/perf-metrics/summary` 挂的是 `middleware.HeaderNavModulePublicOrUserAuth("pricing")`（`router/api-router.go:45-49`），即**可能对未登录访客开放**，服务于模型广场/定价页（`web/src/features/pricing/*`）。`PerfMetric` 的计数字段虽然是 `json:"-"`，且 `seriesSchema` 注释明确写了「不要在做隐私收敛时改它」（`pkg/perf_metrics/metrics.go:19-21`），但把渠道维度塞进这条链路，任何一次聚合口径调整都可能把渠道信息带到公开响应里。
- 好处：复用现成的内存累加 + flush + 清理框架，代码量最小。

### 方案 B（推荐）：新建独立的 `channel_metrics` 桶表，与 `perf_metrics` 并存、各自打点

- 表：`(channel_id, bucket_ts)` 唯一索引 + `bucket_ts` 普通索引，计数列 `request_count / success_count / total_latency_ms`（可选 `ttft_*`）。**不带 model / group** —— 页面按渠道逐行展示，不需要模型维度。
- 基数：每桶行数 = **有流量的渠道数 ≤ 渠道总数**，与模型数、分组数**完全解耦**。

| 窗口 / 桶 | 100 个渠道 | 300 个渠道 |
|---|---|---|
| 24h @ 5min（288 桶） | ≤ 28.8k 行 | ≤ 86.4k 行 |
| 7d @ 5min（2016 桶） | ≤ **201.6k 行** | ≤ 604.8k 行 |

  行宽更窄（无 varchar 键，≈ 60–80 B），7d 上界约 **12–50 MB**。对比方案 A 的 7d 上界 4M 行，量级差一个数量级以上，且上界是**确定的**（渠道数是运营可控的小数字），不像 `abilities` 会随模型接入线性膨胀。
- 打点位置（这是方案 B 真正的价值）：hook 在**每次 attempt**上，而不是每请求一次：
  - 成功：`controller/relay.go:290-306`（`RouteAttemptSucceeded` 分支，紧邻 `service.RecordAutoBanSuccess(channel.Id)`）；
  - 失败：`controller/relay.go:308-333`（每次 attempt 失败都会走 `processChannelError`，`controller/relay.go:514-527` 是所有失败 attempt 的**唯一汇合点**，文本 relay 和 `RelayTask` 都调用它）；
  - 任务类：`controller/relay.go:724-739`（`RelayTask` 的重试循环）、`:748`（成功分支）、`RelayMidjourney`（`:577`）。
  - 这样 failover 中被跳过的故障渠道**会被正确记为失败**，可用率才是真的「这个渠道的可用率」。
- 沿用现有约定：`relayInfo.IsChannelTest` 为 true 时跳过打点（现有 3 个 perf 打点点都这么做：`controller/relay.go:377`、`service/quota.go:379`、`service/text_quota.go:543`），分组监控探测同理（`c.GetBool("group_monitoring_probe")`）。
- 可复用现有范式：内存累加 + 定时 flush upsert（`pkg/perf_metrics/flush.go:26-62`）、`clause.OnConflict` + `gorm.Expr("col + ?")` 增量 upsert（`model/perf_metric.go:29-49`）、保留期清理（`model/perf_metric.go:118-123`）。新表**索引从第一天就是最终形态**，不存在 §2.1 的老库索引问题。
- 代价：多一套 flush goroutine / 多一张表（约 150–250 行新代码）；与 `perf_metrics` 有少量概念重复（但键、语义、消费方都不同，不是薄封装）。

### 方案 C：从 `logs` 表 SQL 聚合

已由 `research/log-aggregation-feasibility.md` 否掉：`ERROR_LOG_ENABLED` 默认 false（失败不入库）、日志库可独立甚至 ClickHouse。

### 推荐

**方案 B**。理由按重要性排序：
1. 语义正确性 —— 只有 attempt 级打点能反映 failover 中被绕过的故障渠道，方案 A 结构性做不到；
2. 基数可控 —— 行数只和渠道数相关，7d 上界是十万级而不是百万级；
3. 迁移安全 —— 新表没有老库唯一索引变更的三库陷阱（方案 A 的 MySQL 静默合并是数据正确性事故）；
4. 隔离 —— 不动准公开的模型广场链路，管理员数据留在管理员接口里。

---

## 5. 「仅管理员可见」的标准做法

### 后端

`middleware/auth.go:92-108`：

```go
func UserAuth()  { authHelper(c, common.RoleCommonUser) }
func AdminAuth() { authHelper(c, common.RoleAdminUser) }
func RootAuth()  { authHelper(c, common.RoleRootUser) }
```

现有例子（整组挂载）：

```go
// router/channel-router.go:20-21
channelRoute := router.Group("/api/channel")
channelRoute.Use(middleware.AdminAuth())
```

单条路由挂载：`router/api-router.go:185-189`（`logRoute.GET("/", middleware.AdminAuth(), controller.GetAllLogs)`）、`:210-213`（`dataRoute`）。分组监控现在用的是 `UserAuth()` + 子组 `RootAuth()`（`router/api-router.go:25-36`），**渠道监控页应该用 `AdminAuth()`**（渠道相关接口的统一口径，见 `router/channel-router.go:21`）。

### 前端

1. **路由守卫**（`web/src/routes/_authenticated/channels/index.tsx:36-44`）：

```tsx
export const Route = createFileRoute('/_authenticated/channels/')({
  beforeLoad: () => {
    const { auth } = useAuthStore.getState()
    if (!auth.user || auth.user.role < ROLE.ADMIN) {
      throw redirect({ to: '/403' })
    }
  },
  validateSearch: channelsSearchSchema,
  component: Channels,
})
```

角色常量在 `web/src/lib/roles.ts:21-26`（`USER=1 / ADMIN=10 / SUPER_ADMIN=100`）。

2. **侧边栏**：放进 `id: 'admin'` 的 nav group（`web/src/hooks/use-sidebar-data.ts:112-115`），过滤逻辑在 `web/src/hooks/use-sidebar-view.ts:53-63`：

```ts
const isAdmin = role >= ROLE.ADMIN
return configFilteredRoot
  .filter((group) => (group.id === 'admin' ? isAdmin : true))
  .map((group) => {
    const items = group.items.filter(
      (item) => item.requiredRole === undefined || role >= item.requiredRole
    )
    ...
  })
```

单项还可用 `requiredRole`（`web/src/components/layout/types.ts:36`，用例 `use-sidebar-data.ts:129,155` 是 `ROLE.SUPER_ADMIN`）。

3. 现有的 Group Monitoring 入口挂在**非 admin 组**（`use-sidebar-data.ts:89-93`，`monitoring` 相关分组），渠道监控页若要仅管理员可见，应改挂 `admin` 组或加 `requiredRole: ROLE.ADMIN`。

---

## Caveats / 未覆盖

- **无法读取生产数据库**，§2.3 的 K/A/N_ab 全是标注过的假设值；表里已给出核对用的只读 SQL。真实倍数必须用 `abilities` / `perf_metrics` 的实际计数替换后再定结论。
- `E[A] = c·(1-(1-1/c)^n)` 是均匀随机选择下的期望值；本项目是优先级+权重选择（`service/channel_select.go:84,168`），实际 A 更小。该公式只用于给出「不会超过多少」的直觉，不是精确模型。
- 未逐行阅读 `middleware/distributor.go`（532 行）与 `service/channel_select.go` 的完整选择算法，只确认了「不存在滑动窗口失败率/熔断器」这一否定结论（grep `cooldown|circuit|failure|Redis` 无命中）。
- 未评估 ClickHouse 日志库场景下新表的落库位置（新表应落主库 `DB`，与 `perf_metrics` 一致，不涉及 `LOG_DB`）。
- 工作区有大量其他 session 的未提交改动；本次调研全程只读，未执行任何 git 写操作。
