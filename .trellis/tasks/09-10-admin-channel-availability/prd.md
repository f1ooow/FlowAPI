# 管理员渠道可用性监控

## Goal

新增一个**仅管理员可见**的「渠道可用性监控」页面：按渠道逐行展示可用率时间线、可用率百分比与请求数，顶部有整体可用性 / 平均延迟 / 错误率汇总，支持多档时间范围切换。

数据来自**真实调用**（不是合成探测）。

## Background

见 `research/channel-availability-source.md`（在兄弟目录 `09-10-group-monitor-log-aggregation/research/` 下）。

**当前没有任何现成的渠道级可用性数据源**：
- `channels` 表只有点值，无历史：`test_time` / `response_time` 是最近一次合成测试的墙钟耗时（覆盖写），`other_info.auto_ban_failures` 是连续失败计数（成功即清零）。
- 唯一带 `channel_id` 的桶表 `quota_data` 只写成功消费日志且受 `DataExportEnabled` 控制，算不出可用率。
- 全仓唯一「每渠道每次尝试成败」的完整信息是内存里的 `service.RouteState`，只被 JSON 化写进日志的 `other.route_history`，无法聚合。

所以必须新增打点与存储。

## Requirements

### R1 新建 `channel_metrics` 桶表（不给 `perf_metrics` 扩维）

- 维度：`(channel_id, bucket_ts)`，唯一索引 + `clause.OnConflict` 原子累加，落**主库 `DB`**。
- 指标至少包含：`attempt_count`、`success_count`、`total_latency_ms`。
- **禁止给 `perf_metrics` 加 `channel_id`**，原因（硬性，不可推翻）：
  - GORM v1.25.2 只按索引名建索引、永不修改已有索引，老库上唯一索引不会更新。MySQL 会退化成忽略列名的 `ON DUPLICATE KEY UPDATE`，**把不同渠道的计数静默累加进同一行**（脏数据且无告警）；PG/SQLite 每次 flush 报 `42P10` 失败。仓库内无 DropIndex 先例。
  - `/api/perf-metrics` 挂 `HeaderNavModulePublicOrUserAuth("pricing")`，是准公开链路，渠道信息不应经由它暴露。

### R2 attempt 级打点

- 打点必须挂在**单次渠道尝试**层面，不是单次用户请求层面。理由：失败打点若放在重试循环之外，failover 成功时故障渠道的失败会完全不可见，可用率永远接近 100%，页面失去意义。
- 打点位置（研究给出的汇合点，实现时复核）：成功 `controller/relay.go:290-306`，失败 `processChannelError`（`controller/relay.go:514-527`），任务类 `RelayTask`（`controller/relay.go:724-748`）。
- **任务类 relay（Midjourney / Suno / 视频）当前完全没有 perf 打点**，本任务需要覆盖到，否则这些渠道永远无数据。

### R3 页面与权限

- 后端路由挂 `middleware.AdminAuth()`；前端路由 `beforeLoad` 判 `role < ROLE.ADMIN` 时 `redirect('/403')`，侧边栏入口归入 `admin` 组。
- 页面内容：
  - 顶部汇总：整体可用性、平均延迟、错误率
  - 主体：按渠道逐行的可用率时间线（各时间桶按健康度着色）+ 行尾可用率百分比与请求数
  - 无数据的渠道明确显示「暂无数据 / 无请求」，不显示为 0% 可用率
- 时间范围切换：15 分钟 / 1 小时 / 6 小时 / 24 小时 / 7 天。

### R4 保留策略（硬性）

- `channel_metrics` **必须有保留期清理**，不得复制 `perf_metrics` 的现状。
- 注意：`perf_metrics` 的 `RetentionDays` 默认 `0`，而 `flush.go` 里 `0` 直接 `return` 不清理，**这张表目前根本不会被清理**。这是既有隐患，本任务不修，但新表不得重蹈覆辙。

## Non-Goals

- **端点健康（endpoint health）功能不做** —— 只做可用性监控。
- 不做主动探测。渠道的「活跃探测 x/y」「负载」这类依赖探针的指标不做。
- 不修改 `perf_metrics` 的表结构。
- 不修 `perf_metrics` 既有的 dead path 与无保留期问题（记录在案，另行处理）。
- 不改动 `controller/channel-test.go` 的渠道测试功能。

## Constraints

- 三库兼容（SQLite / MySQL / PostgreSQL），时间分桶在 Go 侧计算成 `bucket_ts` 列，SQL 只做 `WHERE bucket_ts BETWEEN ?` + `GROUP BY bucket_ts`。项目内无 SQL 侧时间分桶先例，不得引入。
- `group` 等保留字列名走 `commonGroupCol`；避免 `gorm:"default:true"` 布尔 tag。
- 7 天范围下 5 分钟桶数据量大，展示时需要在查询期做降采样（项目无多级桶先例，方言分支可参考 `model/usedata_rankings.go:51-56`，PG 用 `FLOOR`）。
- JSON 序列化走 `common/json.go` 封装。
- 前端文案走 i18n。
- 工作区存在其他 session 的大量未提交改动：commit 显式列文件名，禁止 `git add -A/.`，禁止 `git restore/checkout/clean`。

## Acceptance Criteria

- [x] `channel_metrics` 表在三库上均能正确 AutoMigrate 与 upsert 累加 —— **SQLite 实跑；MySQL/PG 为代码走查**。索引自建表起即为最终形态，不存在 GORM 不改已有索引的隐患
- [x] 打点在 attempt 级：构造一次 failover（渠道 A 失败、渠道 B 成功），渠道 A 的失败被记录，可用率下降 —— 单测 `TestRecordChannelAttemptAttributesEachFailoverAttempt` 重放「11 失败 → 11 再失败 → 22 成功」，断言 11 为 2/0、22 为 1/1
- [x] 任务类 relay（MJ / Suno / 视频）的渠道尝试也被记录 —— **代码层面接入；未在真实任务流量下验证**
- [x] 页面仅管理员可访问，非管理员访问后端返回鉴权失败、前端跳 403 —— **代码与单测层面；未在真实环境用非管理员账号点过**
- [x] 按渠道展示可用率时间线，支持 15m / 1h / 6h / 24h / 7d 切换
- [x] 无请求的渠道显示「暂无数据」而非 0%
- [x] `channel_metrics` 保留期清理生效，超期数据被删除 —— 单测验证；复用 `perf_metrics_setting.RetentionDays`，**同样受老库已持久化 `retention_days=0` 影响**，部署后需手动改配置
- [x] 后端构建通过；若触及 `relaykit/`，额外通过 `cd relaykit && GOWORK=off go build ./...`
- [x] 前端 `bun run build` 与类型检查通过

## Open Questions

- 打点位置的具体行号需在实现时复核（研究基于当前工作区代码，且工作区有其他 session 的未提交改动）。
- `relayInfo.ChannelMeta` 是嵌入指针且 `GenRelayInfo` 不初始化，**选渠道之前就失败的请求读 `info.ChannelId` 会 panic** —— 打点实现必须处理这种「尚未选中渠道」的情形，具体策略待 design 确定（跳过打点 or 归入特殊 channel_id）。
- 行数估算中的关键参数（活跃渠道数等）是研究给出的假设值，research 文件里附了可直接跑的只读核对 SQL，落地前建议用真实数据核对。
