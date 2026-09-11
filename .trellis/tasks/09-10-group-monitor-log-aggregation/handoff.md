# 夜间收口清单（2026-09-11）

用户已睡，授权：做完 A+B → 审查含其他 session 的全部改动 → 全部 commit（工作区要干净）→ 部署 HK 生产。

## 已完成

- **A 任务**（`09-10-group-monitor-real-traffic`）：分组监控改读 `perf_metrics` 真实流量、删除主动探测、配置化（分组/每组模型/备注/分桶）、前端重构为「分组 section + 模型卡」。全仓 `go test` 通过。
- **A 收口审计**：修了 2 个 P0（`channel-test.go` 被探测污染已还原为与 HEAD 逐字相同；`relay.go` 残留探测专属计费旁路已删）；补了 openapi。
- **死代码清理**：清掉 17 处不可达 `IsChannelTest` guard。清完后 `service/billing.go`、`tiered_settle.go`、`violation_fee.go`、`text_quota.go` 的 diff 归零（自动回到 HEAD）。HEAD 原有的 7 处完好。
- **尾桶加固**：序列上界改为 `align(now - flushSeconds, displaySeconds)`，消除多节点未 flush 桶造成的假红条。
- **前端门禁修复**：copyright 补齐 4 个文件；i18n 补 75 个 key（纯新增零删除）；`safeNumberFieldProps` 修 NaN 输入；checkbox 加可访问名；删除 `web/scripts/add-missing-keys.mjs`；custom 同步端点修了两个 bug（sentinel + 输入框每键 remount）。
- **format**：`model-status-card.tsx` 已 `oxfmt` 格式化，`group-monitoring/` 目录 format 干净。
- **lint 基线确认**：268 个错误全部既有（集中在 `src/assets/brand-icons/*` 的 `no-import-type-side-effects`、`scripts/sync-i18n.mjs` 等），本次改动零新增。

- **后端 Blocker 修复**（全仓 `go test` + relaykit 独立构建通过）：
  - B1 流式重放：`StreamStatus` 加不可变 `gateEnabled` 标志，非 gated 通道（aws/baidu/xai/dify/openai-audio/cohere/zhipu/ollama/tencent/coze/palm/cloudflare/xunfei）回落 `c.Writer.Written()`
  - B2 合法空补全：新增 `streamFrameCompleted` 判据（openai `finish_reason` / anthropic `message_delta+stop_reason` / responses `response.completed` / gemini `finishReason`）；裸 `[DONE]` 只在零 neutral 帧时才判失败；非 JSON `data:` 归为 neutral
  - route_history 挪进 `other.admin_info.route_history`，前后端同步，补了 `TestFormatUserLogsHidesRouteHistory`
  - `skip_retry_on_failure` 恢复（保留 `AutoBanEligible`，因为错误本身是真实上游失败、只有遍历被 opt-out），删除死代码 `shouldRetry`
  - `doRequest` 移除无条件 `SetEventStreamHeaders`、pinger 改为 gate 感知 → 预提交 502 现在是 `application/json` 且错误体不再被吞
  - TTFT 口径统一：缓冲帧补回 `SetFirstResponseTime()` 与 `ReceivedResponseCount++`，gated 与非 gated 保持「首个上游事件」语义

- **透传黑名单补齐**：15 项全进（IP/代理链 9 + Referer/Origin 2 + 上游账号作用域 3 + `mj-api-secret` 1），`global-passthrough.md` 新开 Limits 段，测试加了 must-not-forward 断言与「显式 admin override 仍可设置被屏蔽头」的契约锁定。额外发现 `relay/channel/task/vertex/adaptor.go:121,279` 也设 `x-goog-user-project`。
- **心跳事件预算**：`len(buffered)` 拆成 `buffered`（回放用，含心跳）与 `bufferedEvents`（只计 JSON 帧，驱动 64 事件上限）。字节上限仍计心跳，无限 ping 依然有界。避免了 10s 心跳的代理在长 reasoning 请求约 11 分钟后撞上限 → 502 → 跨渠道重放。
- **B 任务后端**（全仓测试通过）：`channel_metrics (channel_id, bucket_ts)` 新表、attempt 级打点 3 处、range→step 查询、`AdminAuth` 的 `GET /api/channel-monitoring/summary`。
  - **纠正了 design 的一处错误**：`FLOOR` 分支应给 **MySQL** 而非 PostgreSQL（MySQL 的 `/` 返回 DECIMAL，`(bucket_ts/1800)*1800` 会算回原值导致零 rollup；PG/SQLite 才是整数截断）。按仓库先例 `model/usedata_rankings.go:51-56` 修正并加 `TestChannelMetricBucketExprPerDialect` 锁定。
  - **拒绝了指定的打点位置并给出更好的**：没挂 `processChannelError`（因 `controller/channel-test.go:945` 也调它，会把合成渠道测试失败记进真实可用率），改挂 relay switch 之后一行，一处覆盖成功与失败。

## 待办（按顺序）

### 1. ~~补透传黑名单~~ 已完成

commit `0e391d3e0` 把逐渠道配置改成了全局开关，但黑名单没跟着补。审查确认 15 个安全项**全部缺失**。

在 `relay/channel/api_request.go` 的 `passthroughSkipHeaderNamesLower`（约 :73-107）追加：

```go
// Client IP / proxy chain
"x-forwarded-for": {}, "x-real-ip": {}, "x-forwarded-host": {},
"x-forwarded-proto": {}, "x-forwarded-port": {}, "cf-connecting-ip": {},
"true-client-ip": {}, "forwarded": {}, "via": {},
// Caller-site disclosure
"referer": {}, "origin": {},
// Upstream account / billing scope — 客户端可覆盖渠道配置，是可写入控制面不只是泄露
"openai-organization": {}, "openai-project": {}, "x-goog-user-project": {},
// Gateway's own user token (middleware/auth.go:397)
"mj-api-secret": {},
```

**为什么是控制面而非单纯泄露**：`processHeaderOverride` 在 `SetupRequestHeader` **之后**执行，客户端同名头会静默覆盖 `openai/adaptor.go:190` 的 `OpenAI-Organization` 与 `vertex/adaptor.go:225` 的 `x-goog-user-project` → 403 触发渠道自动禁用，或 key 跨 org 有效时计费错位。

配套：把这些头加进 `relay/channel/global_header_passthrough_test.go:70-82` 的 must-not-forward 列表；在 `.trellis/spec/backend/global-passthrough.md` 新开 Limits 段。

### 2. B 任务（`09-10-admin-channel-availability`）

prd/design/implement 已就绪。核心：新建 `channel_metrics (channel_id, bucket_ts)` 表，**attempt 级打点**（请求级会让 failover 中被绕过的故障渠道永远记不到失败），仅 `AdminAuth()` 可见。必须等 `controller/relay.go` 释放。

### 3. 全量验证 → commit → 部署

## 部署注意（HK）

- `ssh hk`（端口 39070，公钥登录）；compose 在 `/opt/flowapi`
- 流程：构建 `linux/amd64` 镜像 `flowapi:hk-<feature>-<timestamp>` → 备份 PG dump + compose + .env + SHA256SUMS 到 `/opt/flowapi/backups/pre-<feature>-<ts>/` → 打回滚镜像 tag → 只重建 `app`（PG/Redis/CCH 不动）→ 验证 healthy + 公网 `/api/status`
- **部署后手动改两个配置**：桶宽 5 分钟、保留期 7 天。代码默认值已改，但 `options` 表已持久化 `bucket_time=hour` / `retention_days=0`，代码默认值不覆盖已持久化行。不改的话 5/15/30 分钟档全部降级成小时粒度，`perf_metrics` 也继续不清理。
- **`RetentionDays` 0→7 的首次清理是无界 DELETE**，长期积累的库上可能长时间持锁，考虑先手动分批删。
- **验证时别猛刷页面**：`/api/user/auth/refresh` 与 `/api/user/login` 共用 20 次/20 分钟/IP 的 Redis 固定窗口，access token 只存内存 TTL 15 分钟，每次整页加载必打一次 refresh，刷 21 次就把登录额度吃光、之后 login 一直 429。
- 切桶宽当天，24h 窗口内会同时存在旧小时桶和新 5min 桶，图表短暂出现一根高桶挨着一堆细桶，属过渡态。

## 要告诉用户的事（不自行决定）

1. **全局请求头透传开关请保持关闭**，直到黑名单补完并验证。默认值是 false，部署本身安全。另外它没有渠道级 opt-out，是全有或全无——如果渠道池里有不可信第三方中转，即使补完黑名单也不建议开。
2. `web/src/lib/time.ts` 的 UTC+8 是用户有意为之，**已确认不改**，审查报告里的相关建议一律驳回。见 memory `dashboard-today-is-intentionally-utc8`。
3. 归档任务 `08-28-channel-cost-ratio` 的 prd/design/check-report 被本次工作区改动改写过，使归档文档与当初实际发布内容相反。这是记账口径问题不是缺陷，但需要用户确认是否有意。
4. 弹性路由还有 6 个 Major 未修（多 Key 轮询索引被缓存替换冲掉、选渠道退化成全渠道扫描+每渠道正则编译、空 model mapping 存成字面量 `"null"`、`maxAttempts<=0` 时 panic、遍历无时间预算最多 40 次上游调用、立即禁用被静默降级为计数禁用）。本轮只修了 2 个 Blocker + 2 个 Major。
