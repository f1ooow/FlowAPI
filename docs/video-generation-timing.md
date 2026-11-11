# 视频任务历史耗时

视频任务查询在响应**最外层**增加 `metadata.generation_timing`，供调用方估算等待进度。覆盖 `/v1/videos/:task_id`、Wan `/api/v1/tasks/:task_id` 和通用 `{code,data}` 查询格式；原有状态、结果、用量与其他 metadata 保留。提交请求和供应商协议不变。

```json
{
  "metadata": {
    "generation_timing": {
      "average_seconds": 123.5,
      "sample_count": 42
    }
  }
}
```

无有效样本、任务缺少实际渠道/型号身份、统计关闭或统计不可用时，字段为 `null`。统计错误不改变正常任务查询的状态码或业务状态；任务本身不存在、越权或查询失败仍按原有错误处理。对于通用响应，metadata 与 `code`、`data` 同级，不放入 `data`。供应商若已有同名字段，以 FlowAPI 的统计字段覆盖。

## 统计口径

- 先按当前用户读取任务，再取该任务实际 `channel_id`、`properties.upstream_model_name`、`properties.origin_model_name`，精确匹配历史记录。客户端不能指定统计渠道；不同用户只共享匿名聚合值，不返回样本任务、用户信息或凭据。
- 只取 `SUCCESS`、`submit_time > 0`、`finish_time > submit_time`，且完成时间处于统计窗口内的任务。按 `finish_time DESC, id DESC` 选最新的有限样本，在数据库中计算 `AVG(finish_time - submit_time)` 和 `COUNT(*)`。
- 使用原始型号区分共用上游 MiniMax-H3 的视频与 Context IR 操作。旧任务缺少型号身份时不猜测归类。
- 这是**供应商受理后到 FlowAPI 观察到成功**的耗时，包含受理后的排队和轮询观察延迟。`submit_time` 在供应商 POST 接受后登记，因此不含上传、首次 POST 耗时或下游下载/入库耗时；`start_time` 不参与统计，避免漏掉未观察到 running 的快速任务。
- 当前历史记录没有可靠统一的分辨率快照，因此不按分辨率、视频时长或生成模式进一步拆分。同一型号不同配置的耗时可能混合，样本均值不是完成时间保证。
- 此字段只用于 UI。调用方可按 `average_seconds * 1.1` 设置并冻结当前任务的展示估时；不得据此变更任务超时、取消、重试、真实进度、供应商参数或账务终态。

## 窗口和缓存

复用 `perf_metrics_setting` 的已有开关、保留窗口和刷新间隔，新增样本数量配置；均通过现有服务端设置管理，不新增前端设置页。

| 配置 | 默认值 | 用途 |
| --- | --- | --- |
| `perf_metrics_setting.enabled` | `true` | 关闭时返回 null |
| `perf_metrics_setting.retention_days` | `7` | 按完成时间向前查询的天数；非正值不执行统计 |
| `perf_metrics_setting.flush_interval` | `5` | 缓存分钟数；沿用已有最小 1 分钟规则 |
| `perf_metrics_setting.task_timing_sample_limit` | `100` | 最近成功任务的最大样本数；非正值不执行统计 |
| 环境变量 `VIDEO_TASK_TIMING_CACHE_CAP` | `1000` | 未启用 Redis 时的进程内 LRU 容量 |

100 条样本是可调整的查询资源预算，不是供应商约束或任务执行上限。缓存按实际渠道、两个型号身份和影响结果的设置分组，设置变化使用新缓存键；同进程并发冷查询由 singleflight 合并。启用 Redis 时复用已有 HybridCache；否则使用有容量上限的内存缓存。缓存有过期时间，不按任务 ID 重复查询历史。

有值、无样本及统计数据库失败均缓存一个刷新周期，防止每次轮询重复访问历史表。取消的查询不写入失败缓存。缓存服务故障时放弃本次统计并返回 null，不将故障转成密集数据库回源。错误日志只记录统计不可用，不附带 SQL、任务或供应商内容。

## 实现与验证记录（2026-11-10）

- `model/task_generation_timing.go`：数据库有界子查询与聚合；JSON 条件分别使用 SQLite `json_extract`、MySQL `JSON_UNQUOTE(JSON_EXTRACT(...))`、PostgreSQL `->>`，不改变表结构。
- `service/task_generation_timing.go`：身份、窗口、缓存及并发冷查询合并。
- `relay/task_generation_timing.go` 和 `relay/relay_task.go`：鉴权成功后的统一出口追加根 metadata；以 RawMessage 保留供应商数字精度及原有内容。
- `go test ./model ./service ./relay -count=1` 通过。新增回归覆盖成功受理至结束均值、小数均值、样本/时间/身份边界、无样本、设置失效、查询错误不干扰轮询、重复轮询只查一次统计、三个响应格式、越权拒绝、凭据不泄露、既有 metadata 与大整数不变。
- 新增相关测试的 `go test -race` 通过；macOS 链接器输出 `LC_DYSYMTAB` warning，未导致测试失败。
- 隔离 SQLite 的实际查询计划使用 `idx_tasks_channel_id`，对匹配记录临时排序后聚合。时间窗口及 LIMIT 限定统计范围和参与聚合的样本数，不代表底层扫描行数最多 100；生产数据量下的执行计划与延迟尚未验证。
- 未连接生产数据库、真实渠道或供应商，未执行部署；未运行真实 MySQL/PostgreSQL/Redis 联调。不将 SQLite 回归称为生产性能或三数据库验收。
- 本分支仅包含估时元数据，与 H3 分辨率退款分支独立。用户已授权单独提交 PR；未合并或上线。调用方配套分支为 AIGC 的 `feat/model-generation-progress`，没有该字段的旧调用方保持原行为。
- 临时查询计划脚本已删除；SQLite 回归只用内存库，未留下数据库或导出包。任务工作区在 PR 远程提交核对后按分支流程释放，保留本地及远程分支供审查恢复。
