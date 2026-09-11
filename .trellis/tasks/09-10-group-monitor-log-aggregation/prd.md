# 监控体系改造：从合成探测转向真实流量

## Goal

把可用性监控的数据来源从**主动合成探测**整体转向**真实调用聚合**，并补齐管理员视角的渠道级可用性视图。

父任务只负责：源需求集、任务map、跨子任务验收、最终集成 review。实现落在子任务里。

## 源需求（用户原话要点）

1. 分组监控改成「一定时段内某个分组的模型调用实际情况」来分析，而不是发起请求 —— 更经济，且能反映真实请求情况和首字延迟。
2. 数据源和配置都要做。配置包括：选择要监控的分组、备注说明、要监控的模型、分桶。
3. 现有的探测整体删除。
4. **不要动原本的渠道测试功能。**
5. 分组监控不需要具体到某个渠道，只需要分组 + 模型。
6. 额外增加一个**只有管理员可见**的渠道健康可用性监控。**不需要端点监控功能，只需要可用性监控。**
7. **不需要公开状态页**，用户能看到就好，并且还是原本的入口。

补充确认（本轮问答）：
- 「要监控的模型」= 每个分组各自选模型（不是全局一份列表）。
- 统计窗口固定 24 小时，不做成配置项（只有分桶可配）。

## 任务map

| 子任务 | 交付物 | 数据源 |
|---|---|---|
| `09-10-group-monitor-real-traffic` | 分组监控改数据源 + 配置化（监控分组 / 每组模型 / 备注 / 分桶），删除主动探测 | 复用 `perf_metrics` |
| `09-10-admin-channel-availability` | 新增仅管理员可见的渠道可用性监控页面 | 新建 `channel_metrics` 表 + attempt 级打点 |

两者数据源不同、页面不同、可独立验证，无强制先后顺序。若并行推进，注意二者都会触碰 `controller/relay.go` 的打点/日志区域，需协调改动避免冲突。

## 跨子任务约束

1. **渠道测试功能（`controller/channel-test.go`）必须原样保留**，两个子任务都不得改动其行为。分组监控当前复用了它的 `buildTestRequest` 与 `resolveChannelTestUserID`，删除探测时只断开复用关系。
2. **不得聚合 `logs` 表**：`ERROR_LOG_ENABLED` 默认 `false` 导致失败请求不入库（在线率分母缺失）；TTFT 仅在 `logs.other` 的 JSON 文本里；日志库可能是独立库甚至 ClickHouse。
3. **不得给 `perf_metrics` 加 `channel_id`**：GORM v1.25.2 不会修改已有唯一索引，老库上 MySQL 会静默把不同渠道计数累加进同一行，PG/SQLite 每次 flush 失败。
4. 时间分桶一律在 Go 侧算成 `bucket_ts` 列，不写 SQL 侧时间分桶（项目无此先例，四方言语义不一致）。
5. 三库兼容（SQLite / MySQL / PostgreSQL）为硬约束，遵循 `AGENTS.md`。
6. 工作区有其他 session 的大量未提交改动：commit 显式列文件名，禁止 `git add -A/.`，禁止 `git restore/checkout/clean`，scope 外的失败原样汇报不顺手改。

## 明确不做

- 匿名公开状态页、对外显示名 / slug / 排序 / 供应商类型覆盖。
- 端点健康（endpoint health）。
- 任何形式的主动探测（包括「无数据时兜底探测」）。

## 已记录但不在本次修复范围的既有问题

这些是调研过程中发现的既有缺陷，**不在本次任务修**，另行处理：

1. `pkg/perf_metrics/metrics.go:408` `mergeRedisActiveBuckets` 从未被调用 → `recordRedis` 写进 Redis 的计数从来没被读取过，多节点部署时当前桶的实时数据实际不合并。
2. `model/perf_metric.go:81` `GetPerfMetricsSummaryAll` 无调用点。
3. `perf_metrics` 的 `RetentionDays` 默认 `0`，而 `flush.go:71-73` 对 `0` 直接 `return` 不清理 —— 这张表目前根本不会被清理。

## 集成验收

- [x] 两个子任务各自的验收标准全部通过
- [x] 渠道测试功能在两个子任务合并后行为仍完全不变
- [x] 仓库内无残留的分组监控探测路径（探测执行体、调度、`group_monitoring_results` 表、`group_monitoring_probe` 标记的 4 个消费点）
- [x] 分组监控入口保持原样，仍为登录用户可见，未引入匿名公开入口
- [x] 后端构建与前端 `bun run build` 均通过
