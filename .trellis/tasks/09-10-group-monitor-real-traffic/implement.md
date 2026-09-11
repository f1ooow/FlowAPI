# Implement: 分组监控改数据源与配置化

执行顺序按依赖排列。每个阶段结束后跑该阶段的验证命令，通过再进下一阶段。

## 阶段 0 — 前置确认

- [ ] 复核 design 里引用的所有行号（工作区有其他 session 的未提交改动，行号可能已漂移）
- [ ] 确认 `controller/group_monitoring.go` 等文件仍为未跟踪状态（`git status`），本任务改动与其他 session 的 scope 不重叠
- [ ] 阅读 `.trellis/spec/backend/index.md`、`api-patterns.md`，以及 `AGENTS.md` 的数据库/JSON/测试约束

## 阶段 1 — 采集层：补齐打点

**回滚点**：本阶段独立，可单独 revert。

- [ ] `service/quota.go` `PostWssConsumeQuota`（约 :150-251）末尾补 `perfmetrics.RecordRelaySample`，写法对齐 `service/quota.go:379-383`
- [ ] `controller.RelayTask` / `controller.RelayMidjourney` 补打点；确认能拿到 model / group / latency / success
- [ ] 任务类打点口径写进代码注释：**只记提交阶段的成败与耗时**，不追踪任务最终执行结果
- [ ] 确认图像生成路径无需改动（`relay/image_handler.go:149` → `PostTextConsumeQuota` → `text_quota.go:543-546` 已打点）

验证：
```bash
go build ./...
go test ./pkg/perf_metrics/... ./service/... -run 'Perf|Metric' -count=1
```

## 阶段 2 — 采集层：桶宽与保留期

**回滚点**：改回两个默认值即可。

- [ ] `setting/perf_metrics_setting/config.go`：`BucketTime` 默认 `"hour"` → `"5min"`，`RetentionDays` 默认 `0` → `7`
- [ ] `pkg/perf_metrics/metrics.go:190` `recentSuccessRates(..., 3)` 改为按**时长**换算桶数，不再硬编码桶个数
- [ ] `model-details-charts.tsx:32-36` `formatHourLabel` 改为按桶宽动态选格式（精确到分钟）
- [ ] `model-details-uptime-sparkline.tsx` 降采样到固定条数，避免 288 根条溢出
- [ ] 确认 flush 侧无需改动（`flushCompletedBuckets` 扫全部已关闭桶，`drain()` 是 `Swap(0)`）

验证：
```bash
go build ./... && go test ./pkg/perf_metrics/... -count=1
cd web && bun run build
```
人工：模型广场页面在 5min 桶下时间轴标签不重复、sparkline 不溢出、成功率窗口语义正确。

## 阶段 3 — 查询层

- [ ] `model/perf_metric.go` 新增按 `(group, model_name, bucket_ts)` 聚合的查询，返回含 `request_count / success_count / total_latency_ms / ttft_sum_ms / ttft_count`
- [ ] 分组列名用 `commonGroupCol`；只用 `WHERE bucket_ts BETWEEN ?` + `GROUP BY`，**不写 SQL 侧时间分桶**
- [ ] Go 侧展示桶合并：`displayTs = ts - ts%displaySeconds`，**先累加计数器再算率**
- [ ] `ttft_count == 0` 时 TTFT 返回 null 而非 0
- [ ] 展示桶宽不得小于 `GetBucketSeconds()`，否则降级到可用最细粒度
- [ ] 丢弃 `displayTs + displaySeconds > now` 的桶；`displaySeconds < FlushInterval*60` 时多丢一个
- [ ] 最小样本门槛：`request_count` 低于阈值按 no-data
- [ ] 后端算好 bucket `state`（healthy/degraded/down/no-data）

测试（表驱动，`require`/`assert`）：
- [ ] 桶合并：5min 底层桶 → 15/30/60 展示桶，计数正确
- [ ] 率计算：多桶合并后 availability = Σsuccess/Σrequest，**不是 rate 平均**
- [ ] `ttft_count=0` → TTFT null；`ttft_count>0` → 正确均值
- [ ] 尾桶丢弃边界
- [ ] 最小样本门槛边界

验证：
```bash
go build ./... && go test ./model/... -run 'PerfMetric' -count=1
```

## 阶段 4 — 配置层

- [ ] `setting/operation_setting/group_monitoring_setting.go` 替换结构体为 `GroupMonitoringSetting{Enabled, BucketMinutes, Groups[]{Group, Description, Models[]}}`
- [ ] 删除 `GroupMonitoringTarget`、`IntervalMinutes`、`HistoryRetentionDays`、`EndpointType`/`Stream` 相关的白名单与校验函数
- [ ] option key：保留 `.enabled`，新增 `.bucket_minutes` / `.groups`，废弃三个旧 key（不读不迁移）
- [ ] 校验：`bucket_minutes ∈ {5,15,30,60}`；group 存在于 `ratio_setting.GetGroupRatioCopy()`；model 在 `model.GetGroupEnabledModels(group)` 内
- [ ] 更新 `group_monitoring_setting_test.go`，删除探测相关用例，为新校验补用例

验证：
```bash
go build ./... && go test ./setting/... -count=1
```

## 阶段 5 — 删除探测

**这是破坏性阶段，建议单独 commit。**

- [ ] 删除 `controller/group_monitoring.go` 的探测执行体（约 :407-533）、32 桶内存聚合（约 :92-145）、`sync.Map` 去重锁（约 :28-31）
- [ ] 删除 `controller/system_task_handlers.go:28-50` `groupMonitoringHandler` 及注册；`model/system_task.go:24` 类型常量
- [ ] 删除 `model/group_monitoring.go` 全文 + `model/main.go:338,401` 的 AutoMigrate 注册
- [ ] 删除 `group_monitoring_probe` 的 4 个消费点：`controller/relay.go:530`、`controller/relay.go:554-556`、`service/log_info_generate.go:95-97`、`service/text_quota.go:400-402`
- [ ] 断开对 `buildTestRequest`（`channel-test.go:698`）与 `resolveChannelTestUserID`（`channel-test.go:55`）的调用（`group_monitoring.go:330,384,286,503`）—— **只断调用，不动函数本体**
- [ ] 删除 `controller/group_monitoring_test.go` 中的探测用例

**红线检查**：
```bash
rg -n 'group_monitoring_probe|GroupMonitoringResult|groupMonitoringHandler|SystemTaskTypeGroupMonitoring' --glob '!.trellis/**'   # 应为空
rg -n 'is_channel_test' | wc -l    # 数量应与改动前一致，未被误删
go build ./...
go test ./controller/... -run 'ChannelTest' -count=1   # 渠道测试行为不变
```

## 阶段 6 — API 层

- [ ] `GET /api/group-monitoring/summary` 改为新 DTO（见 design §5），窗口固定 24h，不再接受 `hours` 参数
- [ ] `GET|PUT /api/group-monitoring/admin` 改为新结构；GET 不再返回 `summaries` / `latest_results`
- [ ] 删除 `POST /run` 与 `POST /run-group` 路由与 handler（`router/api-router.go:25-36`）
- [ ] 保持 `middleware.UserAuth()` / `middleware.RootAuth()` 不变——**不引入匿名入口**
- [ ] 同步 `docs/openapi/api.json`

验证：
```bash
go build ./... && go test ./controller/... -run 'GroupMonitoring' -count=1
```

## 阶段 7 — 前端

- [ ] `types.ts` 按新 DTO 重写
- [ ] `api.ts` 删除 run / run-group
- [ ] `index.tsx` 改为「分组 section + 模型卡」布局；删除全量触发按钮；轮询固定 60s
- [ ] `group-status-card.tsx` 改造为模型卡；**删除 `grid-cols-[repeat(32,...)]`**，桶数由数据长度驱动
- [ ] `lib/status.ts` 精简为颜色映射（判定已在后端）
- [ ] `monitoring-settings-panel.tsx` 改为：分组勾选 + 每组模型多选 + 备注输入 + 分桶下拉；删除全部探测字段与 Latest probe results
- [ ] `system-settings/models/group-monitoring-settings-section.tsx` 同步
- [ ] 新增文案进 i18n（`web/src/i18n/locales/*.json`，扁平 JSON，key 用英文原文）

验证：
```bash
cd web && bun run build && bun run lint
```

## 阶段 8 — 全量验证

```bash
go build ./...
go vet ./...
go test ./... -count=1
cd web && bun run build
```

若触及 `relaykit/`：
```bash
cd relaykit && GOWORK=off go build ./...
```

三库兼容：新查询至少在 SQLite 实跑；MySQL/PG 通过代码走查确认无方言依赖（无 SQL 侧时间函数、`group` 走 `commonGroupCol`、无 `default:true` 布尔 tag）。

## 阶段 9 — 收口

- [ ] 对照 `prd.md` 验收清单逐条打勾
- [ ] 更新 `task.json.notes`：子项状态 + commit hash + session 编号 + 测试数
- [ ] 写 journal（真实改动与测试结果，不留 placeholder）
- [ ] release notes 说明：`group_monitoring_results` 残留空表可手动清理；`perf_metrics` 桶宽与保留期默认值变更

## Commit 纪律

工作区有 150+ 个其他 session 的未提交改动：

- **禁止** `git add -A` / `git add .`，每次 commit 显式列出本任务改动的文件名
- **禁止** `git restore` / `git checkout` / `git clean`
- scope 外的测试或类型检查失败**原样汇报**，不顺手修

建议 commit 切分：阶段 1-2（采集层）/ 阶段 3-4（查询与配置）/ 阶段 5（删除探测）/ 阶段 6-7（API 与前端）。
