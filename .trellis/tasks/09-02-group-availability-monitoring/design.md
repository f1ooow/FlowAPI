# 技术设计

## Architecture

新增 `group_monitoring` 领域，由配置、探测执行、结果持久化、查询 API 和前端页面组成。配置继续使用项目全局 settings；探测结果使用独立数据库表；调度注册到现有 system task runner。

## Configuration

`group_monitoring_setting` 包含：

- `enabled`: 总开关。
- `interval_minutes`: 全局探测间隔，限定 1-1440 分钟。
- `history_retention_days`: 结果保留天数，限定 1-90 天。
- `targets`: JSON 数组，每项包含唯一 `group`、非空 `model`、`endpoint_type` 和 `stream`。`endpoint_type` 与原生渠道测试器一致；旧记录缺失该字段时按 `auto` 读取。

管理 API 按分组返回当前启用能力表中的模型列表，配置页使用联动下拉框而非自由文本。配置保存时校验分组存在、模型属于该分组、分组不重复；仅大小写不一致且可唯一匹配的历史模型名归一化为渠道登记的准确名称。删除分组配置不立即删除历史记录。

## Data Model

`GroupMonitoringResult`：

- `id`
- `group_name`
- `success`
- `latency_ms`
- `channel_id`（仅管理员接口返回）
- `request_id` 和 `route_history`（仅管理员接口返回，用于定位实际失败/接管的渠道）
- `error_code`、`error_message`（仅管理员接口返回）
- `checked_at`

索引：`(group_name, checked_at)` 以及 `checked_at`。查询在 Go 中完成固定数量的时间桶聚合，避免数据库方言时间函数。

## Probe Flow

1. system task runner 根据配置间隔创建 `group_monitoring` 任务。
2. 任务顺序处理启用的目标，使用目标分组和模型调用生产渠道选择逻辑。
3. 使用现有 `buildTestRequest` 按目标的模型、请求格式与流式配置构造最小请求，再由正常分组路由选中渠道并完成响应语义校验。`auto` 对 Claude/Gemini 优先选择原生协议，其余沿用现有模型类别判断。
4. 探测上下文标记为 probe，复用正常 Relay 的消费/错误日志链路（TokenName 为“模型测试”），但跳过额度预扣/结算、用户/渠道用量、性能指标、亲和缓存和自动禁用副作用。
5. 无论成功失败都持久化一条结果；任务结束后清理过期结果。

第一版每个分组每周期只发一次请求，测量的是“用户以该分组请求该模型时能否成功”，而不是逐个渠道巡检。

## API Contracts

- `GET /api/group-monitoring/summary?hours=24`: UserAuth，返回脱敏分组摘要和固定时间桶。
- `GET /api/group-monitoring/admin`: RootAuth，返回配置候选、配置值和内部最后结果。
- `PUT /api/group-monitoring/admin`: RootAuth，保存完整配置。
- `POST /api/group-monitoring/admin/run`: RootAuth，幂等获取或创建一次手动 system task，统一返回 202，并以 `created` 区分是否新建。
- `POST /api/group-monitoring/admin/run-group`: RootAuth，只检测请求指定的一个已配置分组并同步返回结果；同一分组检测中返回冲突。

普通摘要只返回 `group_name`、`availability_rate`、`avg_latency_ms`、`last_checked_at`、`last_success`、`stale`、`testing`、`batch_testing` 和时间桶；时间桶额外返回 `latest_success`，用于让每个色块反映该桶最后一次完成的探测，成功数/总数仍保留给可用率和悬浮信息；忙碌字段仅表达探测状态，不包含内部诊断信息。

## Frontend

- 新增受保护路由 `/group-monitoring`，使用现有认证路由布局。
- 在登录用户主导航中增加“分组监控”入口；不增加匿名公共导航。
- 页面以可扫描的分组卡网格呈现，不显示模型或渠道信息。
- 全量探测从点击开始即将所有目标卡片置为忙碌，随后由摘要中的 `batch_testing` 接管并持续到任务完成；单分组探测只维护该分组的忙碌状态。
- 操作请求跳过全局 Axios 错误提示，由页面统一展示一次后端消息，避免重复通知和裸露状态码。
- 管理员在系统设置中直接获得完整配置表单和最近结果诊断，不经过二级弹窗；分组监控展示页仅在管理员卡片上提供“测试此分组”，不为普通用户增加内部诊断控件。
- 设置表单使用 React Hook Form + Zod、Base UI 组件、Hugeicons 和 React Query。

## Compatibility And Operations

- AutoMigrate 创建新表；不修改已有数据。
- 多实例依靠 system task 数据库租约去重，不要求 Redis。
- 总开关关闭后不再创建新任务，历史仍可查询。
- 回滚时可停止配置并保留结果表；旧版本会忽略该表和设置。

## Risks

- 生产 relay 流程当前与计费、日志耦合较深。实现必须尽量复用请求转换和响应校验，同时把 synthetic 副作用控制放在明确上下文标记上。
- 单次采样反映生产路由选择的结果，不保证周期内覆盖所有渠道；这是分组可用性而非渠道巡检的有意定义。
