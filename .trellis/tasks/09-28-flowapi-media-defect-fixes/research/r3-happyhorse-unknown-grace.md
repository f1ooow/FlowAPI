# Research: R3 HappyHorse（ali 任务适配器）上游 UNKNOWN 的处理

- **Query**: ali 适配器遇到 UNKNOWN 怎么处理；keli `de16bfc5f` 的宽限逻辑；FlowAPI 缺了哪部分；任务最终挂多久、何时失败退款
- **Scope**: internal（FlowAPI main `10d32a657`，keli `/Users/Zhuyu/Documents/Code/new-api` feature/fulladaptor 工作树，上游 new-api `upstream/main` 只读对照）
- **Date**: 2026-09-28

## 结论

FlowAPI 只移植了 keli 修复的适配器一半，轮询一半（宽限常量、宽限判断、超窗判失败）没有移植。上游一直返回 UNKNOWN 的任务会一直挂着，要等 24 小时超时清扫才判失败并全额退款。结论是缺陷（移植不完整），后果是收敛延迟，不会造成资金损失。

## 1. 现状（FlowAPI）

### 1.1 适配器：UNKNOWN 映射为非终态

- `relay/channel/task/ali/adaptor.go:679-689`：`"UNKNOWN"` 映射为 `model.TaskStatusUnknown`，同时保留上游 message。
- `adaptor.go:680-683` 的注释写的是"由视频轮询按任务年龄决定宽限存活或有界判死（见 service/task_polling.go updateVideoSingleTask 与 model.TaskUnknownStatusGraceSeconds）"。但 **FlowAPI 里没有 `TaskUnknownStatusGraceSeconds`，也没有 `InUnknownStatusGrace`**：全仓 grep 只找到 `model/task.go:41` 的状态常量。
- `git log -S "TaskUnknownStatusGraceSeconds"` 只命中 `8156149df`，即这段注释本身；`git log -S "InUnknownStatusGrace"` 没有结果。

### 1.2 轮询：UNKNOWN 保持原状

`service/task_polling.go:609-612`：

```go
case model.TaskStatusUnknown:
    task.Status = snap.Status
    task.Data = snap.Data
    return nil
```

这个分支不写库、不打日志（只有 `:518` 的 debug 级响应日志），也没有退出条件。它由 `8156149df` 引入：`git show 8156149df -- service/task_polling.go` 中这 4 行就是完整实现。

### 1.3 共用这个分支的其他适配器

以下适配器也会产出 `TaskStatusUnknown`，所以修改轮询分支会同时影响它们：

| 适配器 | 何时返回 UNKNOWN | 位置 |
|---|---|---|
| ali | 上游显式返回 `UNKNOWN` | `ali/adaptor.go:679-689` |
| hailuo H3 | 状态不属于已识别集合（queued / running / processing / in_progress / succeeded / failed…） | `hailuo/adaptor.go:419-421` |
| sora Wan | 未识别状态 | `sora/wan.go:125-126` |
| fastaitoken | 未识别状态 | `fastaitoken/adaptor.go:267-269` |

### 1.4 一直返回 UNKNOWN 的任务的完整生命周期

1. 提交落库时状态为 `NOT_START`、进度 `0%`，`SubmitTime` 为当前时间（`model/task.go:246-248`）。
2. 每 15 秒轮询一次（`controller/system_task_handlers.go:146`），每次都走 1.2 的分支，任务不变。
3. 每轮轮询开始时先执行 `sweepTimedOutTasks`（`service/task_polling.go:148`，函数在 `:74-123`）：
   - 超时阈值 `TASK_TIMEOUT_MINUTES` 默认 1440 分钟（`common/init.go:205`）。
   - 查询条件：`progress != 100%`、状态不是 FAILURE / SUCCESS、`submit_time < cutoff`，每轮最多 100 条（`model/task.go:338-350`）。
   - 命中后经 CAS 置为 FAILURE，原因写"任务超时（1440分钟）"，并按 `task.Quota` 全额退款（`task_polling.go:84, 105-117`，`RefundTaskQuota` 在 `service/task_billing.go:184-224`）。
   - 若 `TASK_TIMEOUT_MINUTES <= 0`，清扫被关闭，任务永久挂起（`task_polling.go:75-77`）。HK 的实际取值未确认。
4. 结果：任务至少挂 24 小时，外加最多一个轮询周期；超时任务超过 100 条时顺延到后续轮次。期间预扣额度一直被占用。按 15 秒一次算，每个任务约向上游查询 5760 次。

### 1.5 挂起期间用户看到的状态

- `/v1/videos/{id}`：`ConvertToOpenAIVideo` 读取 `task.Data`（`ali/adaptor.go:697-709`）。`task.Data` 仍是提交响应里的 `PENDING`，所以显示 `queued`（`:730-735`）。
- `/v1/video/generations/{id}`：返回 `TaskModel2Dto`（`relay/relay_task.go:511-515`），显示 `NOT_START / 0%`。
- 清扫之后：显示 failure，原因"任务超时（1440分钟）"。

### 1.6 DashScope 对 UNKNOWN 的语义（外部）

- `keli-canvas/.trellis/spec/important/project-docs/happyhorse-api-reference.md:53-57` 记录：状态流转为 `PENDING -> RUNNING -> SUCCEEDED / FAILED / CANCELED / UNKNOWN`，task_id 有效期 24 小时。
- keli 修复的动机"刚提交、尚不可查的任务也返回 UNKNOWN"出自 `keli-canvas/.trellis/tasks/archive/2026-07/07-10-happyhorse-video-fail-debug/research/01-root-cause.md:127`，该文档没有附上对应的上游日志。**未确认。**

## 2. 与 keli 的差异

### 2.1 keli `de16bfc5f`（2026-07-10）的做法

- 宽限常量与判断（`model/task.go:143-154`）：
  - `TaskUnknownStatusGraceSeconds int64 = 120`；
  - `InUnknownStatusGrace(now)` 的条件是 `now - SubmitTime <= 120`，`SubmitTime` 为 0 时视为已超窗。
- 轮询分支（keli 工作树 `service/task_polling.go:587-601`）：
  - 宽限内：打 warn 日志，还原状态和数据后返回；
  - 超窗：reason 为空时填"upstream task status stayed UNKNOWN for more than 120 seconds after submit, task treated as lost"，然后 `fallthrough` 到 FAILURE 分支，置终态并退款（CAS 保证只退一次）。
- 同样的逻辑在 `controller/task_video.go:244-254` 有一份（无调用方的遗留副本，FlowAPI 没有这个文件）。
- 测试：`model/task_unknown_grace_test.go`（4 个）、`service/task_polling_unknown_test.go`（3 个：宽限内存活、超窗失败并退款、FAILED 立即失败）。
- 同一提交里的"提交响应等任务落库后再返回"已经移植到 FlowAPI（`controller/task_response_buffer.go`，来自 `8156149df`）。

### 2.2 FlowAPI 缺的部分

1. 宽限常量和宽限判断方法；
2. 超窗后 fallthrough 判失败；
3. 上述两组测试。

### 2.3 默认状态映射不同，影响面更大

| 适配器 | keli 的 default 分支 | FlowAPI 的 default 分支 |
|---|---|---|
| H3 | `TaskStatusInProgress`（`hailuo/adaptor.go:377-379`） | UNKNOWN |
| FastAI | `TaskStatusInProgress`（`fastaitoken/adaptor.go:244-246`） | UNKNOWN |
| Wan | UNKNOWN（`sora/wan.go:120`） | UNKNOWN |

因此在 FlowAPI 里，UNKNOWN 分支覆盖的情形比 keli 多。

### 2.4 上游 new-api 的另一种做法

`9df450fe5`（2026-09-03，FlowAPI HEAD 不包含）：

- UNKNOWN 和未识别状态都记为一次轮询失败（`upstream/main:service/task_polling.go:538-540`）；
- 连续失败达到 `TASK_POLL_MAX_FAILURES`（默认 20，见 `upstream/main:common/init.go:205`）后判失败并结算退款（`:773-790`）；
- 连续失败次数持久化在 `PrivateData.PollFailures`。

## 3. 是否构成缺陷

**判断：构成缺陷（移植不完整）。**

- 适配器注释说明了预期行为，但所引用的符号在 FlowAPI 不存在（`ali/adaptor.go:680-683`）。
- FlowAPI 的任务记录里没有"有意去掉宽限"的说明。`09-22-flowapi-video-adapter-alignment/verification.md:7` 只写了"UNKNOWN nonterminal"。
- `09-22-flowapi-provider-coverage/design.md:28` 的"上游可能接受时保留 unknown……明确失败才退款"说的是**提交阶段**的不确定（submission unknown），不是轮询得到的 UNKNOWN。

**影响：**

- 真正丢失的任务要 24 小时才收敛；
- 期间额度被占用，用户一直看到"排队中"，上游被反复查询；
- 24 小时后全额退款，没有资金损失。

**反面：**如果"刚提交时短暂返回 UNKNOWN"确实存在，现状对这种短暂 UNKNOWN 是安全的，不会误杀。

## 4. 可选修复方案

| 方案 | 行为 | 风险 |
|---|---|---|
| A. 照搬 keli：共用分支，按提交年龄宽限 120 秒 | UNKNOWN 超过 120 秒即失败并退款 | 1.3 表中 H3 / FastAI / Wan 的未识别状态也会在 120 秒后被判死。上游其实还在跑、还在计费，用户拿不到结果。keli 里 H3 / FastAI 走 InProgress，不受此影响。 |
| B. A，再把 H3 / FastAI 的 default 改回 InProgress（与 keli 一致） | 影响面与 keli 相同：只剩 ali 的显式 UNKNOWN 和 Wan 的未识别状态会进入宽限 | H3 / FastAI 如果出现新的失败状态字符串，任务会挂到 24 小时 |
| C. 宽限只对 ali 生效（按平台判断，或由适配器以可选接口声明） | 其他适配器保持现状 | 需要新增接口或平台判断 |
| D. 上游式连续失败计数（`PollFailures`，默认 20 次，约 5 分钟） | 同时兜住传输错误和未识别响应 | 改动最大：要持久化计数、区分错误类型；和 FlowAPI 现有 UNKNOWN 语义也需要统一 |
| E. 维持现状，在 PRD 记录"UNKNOWN 最长 24 小时后退款" | 无代码改动 | 1.4 所述占额度、刷查询 |

方案 A、B、C 共有的风险：宽限按**提交**时间计算。如果 DashScope 对运行中的任务也会短暂返回 UNKNOWN（未确认），超过 120 秒就会被误杀并退款，而上游可能继续出片并计费。

## 5. 建议的回归测试位置（现有文件）

- `relay/channel/task/ali/adaptor_test.go:327-345`：已有"UNKNOWN 非终态"和"保留 message"两个用例。
- `service/task_polling_test.go`：新增三个用例——宽限内保持、超窗判失败且只退一次、FAILED 立即失败。夹具可复用 `service/task_billing_test.go:64-282` 中的 `truncate`、`seedUser`、`seedToken`、`seedChannel`、`makeTask`、`getUserQuota`。
- `model/task_cas_test.go`：如果在 model 层新增宽限方法，放这里。
- 如果改 default 映射，以下断言会变：`relay/channel/task/hailuo/adaptor_test.go:317`、`relay/channel/task/fastaitoken/adaptor_test.go:138`、`relay/channel/task/sora/wan_test.go:69`（目前都断言 UNKNOWN）。
- 24 小时兜底的现有测试：`service/task_polling_test.go:460` `TestSweepTimedOutTasksHonorsRefundRolloutBoundary`。

## Caveats / 未确认

- DashScope 是否会对刚提交的任务返回 UNKNOWN：本次没有联网查官方文档，也没有查生产日志。
- Zivv 端点覆盖（渠道 79，`ali/protocol_test.go:22-56`）在任务丢失时返回什么：未确认。
- HK 生产的 `TASK_TIMEOUT_MINUTES`：未确认（代码默认 1440）。
