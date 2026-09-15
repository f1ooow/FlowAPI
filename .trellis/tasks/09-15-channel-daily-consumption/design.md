# Design: 渠道今日消耗统计

## 1. 数据流

```text
GET /api/channel 或 /api/channel/search
  -> 主库取当前页渠道
  -> 提取渠道 ID
  -> LOG_DB 按北京当天时间范围 + channel_id 聚合 quota
  -> 在 Go 中将结果回填渠道 DTO
  -> 前端表格 / 卡片 / 标签聚合行
```

主库与 `LOG_DB` 可能是两个数据库，因此使用两次查询和 Go map 回填，不使用 JOIN。

## 2. API 合同

在 `model.Channel` 增加不持久字段：

```go
TodayUsedQuota *int64 `json:"today_used_quota" gorm:"-"`
```

- 指针区分「成功查询后的 0」与「统计不可用」。
- 字段不写入 `channels` 表，也不参与渠道更新。
- `GET /api/channel` 和 `GET /api/channel/search` 保持现有响应外形，只在 item 上增加字段。

## 3. 时间口径

后端显式使用 UTC+8 固定时区计算当天起点，不依赖 `time.Local`：

```text
now -> UTC+8 -> 当天 00:00:00 -> Unix 秒
end = now.Unix()
```

中国标准时间没有夏令时，固定 `+08:00` 可确定处理日界。测试注入固定 `now`，覆盖 UTC 与北京时间日期不同的时刻。

## 4. 聚合查询

在 `model/log.go` 提供按渠道批量聚合的函数，使用 `LOG_DB`：

```text
SELECT channel_id, SUM(quota)
FROM logs
WHERE channel_id IN (...)
  AND created_at >= start
  AND created_at <= end
  AND type = LogTypeConsume
GROUP BY channel_id
```

- 查询只包含当前响应需要的渠道 ID，必要时分批以避免 SQLite 参数数量上限。
- 查询形状只使用各支持数据库共有的 `WHERE` / `SUM` / `GROUP BY`。
- 只汇总正向消费日志；退款日志不参与计算，与首页今日用量口径保持一致。
- 返回 `map[int]int64`；查询成功但 map 中没有某 ID 时，回填显式的 `0` 指针。

## 5. 失败降级

- `common.LogConsumeEnabled == false` 时不执行聚合，所有值保持 `nil`。因为当天可能只有部分日志，返回数字会带来错误精确感。
- `LOG_DB` 查询失败时记录后端错误，但渠道列表仍返回成功，该字段保持 `null`。
- 不使用过期值或将错误替换成 `0`。

## 6. 前端

- `Channel` schema 增加 `today_used_quota: z.number().nullable().default(null)`。
- 新列紧跟「已使用 / 剩余」，使用现有 `formatQuotaWithCurrency`、紧凑数字与 tooltip 精确值规则。
- 敏感信息关闭时使用与 `BalanceCell` 一致的遮罩。
- 标签聚合保留三态：全部可用时求和，任一子项不可用时为 `null`，空数组不会产生伪零值。
- 移动卡片将它作为财务指标放在累计用量附近，不破坏右侧操作指标网格。
- 本轮不增加服务端排序参数。

## 7. 兼容与风险

| 风险 | 处理 |
| --- | --- |
| 消费日志被关闭或清理 | 显示 `--`，不伪造 0 |
| 日志库与主库分离 | 两次查询 + Go map，不 JOIN |
| 标签模式包含大量渠道 | 分批查询 channel ID，汇总返回 map |
| 跨日退款导致净值歧义 | 统计正向消费，退款不冲减 |
| 额外聚合影响渠道页延迟 | 单次批量聚合，禁止 N+1；用聚焦测试与本地查询计划核对 |

## 8. 本地限制

所有实现、数据库 fixture、页面验收和构建均在本地进行。不使用 SSH、生产 URL、生产数据库或部署命令。
