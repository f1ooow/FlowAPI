# 管理员渠道可用性监控

## Goal

提供一个**仅管理员可见**的「渠道可用性监控」页面：用紧凑的响应式渠道卡片展示可用率时间线、平均延迟、尝试数和缓存命中率，顶部展示整体汇总，并支持多档时间范围切换。

数据来自**真实调用**（不是合成探测）。

第二轮将列表收整为运维人员可快速扫读的响应式卡片网格，删除主界面上的数据口径长文和重复脚注。

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
- 第一轮已在 relay attempt 完成点、任务重试循环和 Midjourney 提交门控处完成打点；未挂到也被合成渠道测试调用的 `processChannelError`。
- **任务类 relay（Midjourney / Suno / 视频）当前完全没有 perf 打点**，本任务需要覆盖到，否则这些渠道永远无数据。

### R3 页面与权限

- 后端路由挂 `middleware.AdminAuth()`；前端路由 `beforeLoad` 判 `role < ROLE.ADMIN` 时 `redirect('/403')`，侧边栏入口归入 `admin` 组。
- 页面内容：
  - 顶部汇总：整体可用性、平均延迟、错误率、缓存命中率
  - 主体：仅展示当前范围内有流量的渠道卡片，每张卡包含可用率时间线与核心指标
  - API 保留 `has_data: false` 的无数据语义；页面始终过滤无流量渠道，不把它们显示为 0% 可用率
- 时间范围切换：15 分钟 / 1 小时 / 6 小时 / 24 小时 / 7 天。

### R4 保留策略（硬性）

- `channel_metrics` **必须有保留期清理**，不得复制 `perf_metrics` 的现状。
- `channel_metrics` 复用当前 `perf_metrics_setting.RetentionDays`；现行默认值为 7 天，清理路径已有测试保护。

### R5 可读性改造（第二轮）

- 无流量渠道始终不展示，删除「隐藏无流量渠道」开关。
- 搜索仅在当前时间范围内有流量的渠道中过滤，保留按渠道名和 ID 搜索。
- 删除页面中「数据来自真实调用」和「缓存命中率跨供应商不可比」的两段可见长文。口径约束保留在代码、测试与任务文档中。
- 删除每个渠道下方「N 次尝试中成功 M 次」与「每根色条 N 分钟」脚注。
- 顶部汇总卡仅保留指标名和数值，删除对指标定义的可见说明句。
- 渠道列表改为响应式卡片网格：手机单列、平板两列、宽屏三列。
- 每张卡片保留渠道名、ID、健康状态、可用率、平均延迟、尝试数、缓存命中率与微型时间线。
- 卡片内建立清晰的主次层级：可用率为主指标，其他三项为辅助指标；不在卡片内再嵌套装饰性子卡片。
- 时间线仍保留每个 bucket 的时间与成败详情 tooltip，但不在主界面重复解释分桶口径。
- 保持现有时间范围切换、自动刷新、加载、错误、空状态和管理员权限。
- 本轮只修改本地代码与本地验证，不连接或部署生产环境。

## Non-Goals

- **端点健康（endpoint health）功能不做** —— 只做可用性监控。
- 不做主动探测。渠道的「活跃探测 x/y」「负载」这类依赖探针的指标不做。
- 不修改 `perf_metrics` 的表结构。
- 不修 `perf_metrics` 既有的 dead path 与无保留期问题（记录在案，另行处理）。
- 不改动 `controller/channel-test.go` 的渠道测试功能。

## Constraints

- 三库兼容（SQLite / MySQL / PostgreSQL），时间分桶在 Go 侧计算成 `bucket_ts` 列，SQL 只做 `WHERE bucket_ts BETWEEN ?` + `GROUP BY bucket_ts`。项目内无 SQL 侧时间分桶先例，不得引入。
- `group` 等保留字列名走 `commonGroupCol`；避免 `gorm:"default:true"` 布尔 tag。
- 7 天范围下 5 分钟桶数据量大，展示时在查询期做降采样；整数分桶沿用 `model/usedata_rankings.go:51-56` 的方言分支，MySQL 使用 `FLOOR`。
- JSON 序列化走 `common/json.go` 封装。
- 前端文案走 i18n。
- 工作区存在其他 session 的大量未提交改动：commit 显式列文件名，禁止 `git add -A/.`，禁止 `git restore/checkout/clean`。

## Acceptance Criteria

- [x] `channel_metrics` 表在三库上均能正确 AutoMigrate 与 upsert 累加 —— **SQLite 实跑；MySQL/PG 为代码走查**。索引自建表起即为最终形态，不存在 GORM 不改已有索引的隐患
- [x] 打点在 attempt 级：构造一次 failover（渠道 A 失败、渠道 B 成功），渠道 A 的失败被记录，可用率下降 —— 单测 `TestRecordChannelAttemptAttributesEachFailoverAttempt` 重放「11 失败 → 11 再失败 → 22 成功」，断言 11 为 2/0、22 为 1/1
- [x] 任务类 relay（MJ / Suno / 视频）的渠道尝试也被记录 —— **代码层面接入；未在真实任务流量下验证**
- [x] 页面仅管理员可访问，非管理员访问后端返回鉴权失败、前端跳 403 —— **代码与单测层面；未在真实环境用非管理员账号点过**
- [x] 按渠道展示可用率时间线，支持 15m / 1h / 6h / 24h / 7d 切换
- [x] API 对无请求渠道返回 `has_data: false`，不会把它计算成 0% 可用率
- [x] `channel_metrics` 保留期清理生效，超期数据被删除 —— 单测验证；复用 `perf_metrics_setting.RetentionDays`，**同样受老库已持久化 `retention_days=0` 影响**，部署后需手动改配置
- [x] 后端构建通过；若触及 `relaykit/`，额外通过 `cd relaykit && GOWORK=off go build ./...`
- [x] 前端 `bun run build` 与类型检查通过
- [x] 无流量渠道不出现，且页面不再提供显示它们的开关
- [x] 主页面不再显示两段口径长文、成功尝试脚注、每根色条时长脚注或汇总卡说明句
- [x] 渠道以手机 1 列 / 平板 2 列 / 宽屏 3 列卡片展示，八项保留信息可扫读且无截断、重叠和横向溢出
- [x] 时间线 bucket tooltip、时间范围切换、搜索、自动刷新及各异步状态保持可用
- [x] 更新相关前端行为测试，并通过受影响测试、typecheck、定向 lint 与 build
- [x] 仅在本地开发服务器上完成桌面 / 平板 / 手机视觉验收，未连接或修改生产环境

## Notes

- 打点位置、选渠道前失败的 nil 处理和数据库分桶方言已在第一轮实现与测试中解决；第二轮不改这些后端合同。
- 本轮产品口径已收敛，无阻塞实现的开放问题。
- 本轮仅做本地代码、测试和视觉验收，不使用 research 中的生产核对 SQL。
