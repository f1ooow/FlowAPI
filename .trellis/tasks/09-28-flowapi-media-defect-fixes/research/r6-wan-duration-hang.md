# Research: R6 Wan 上游成功但输出时长无法校验

- **Query**：`service/task_polling.go:540-546` 附近的 Wan 任务状态怎么流转；会不会永久挂起、有没有兜底；用户看到什么；计费怎么走；keli 工作树怎么做；FlowAPI 这样做是不是有意为之
- **Scope**：internal（FlowAPI、keli 工作树，并参照上游 new-api `upstream/main`）
- **Date**：2026-09-28

## 结论

"不判失败、保留预扣、继续轮询"是 FlowAPI 的有意设计，代码注释和专门的测试都能证明。但这个设计没有上限，唯一的兜底是 24 小时超时清扫。结果是：

- 如果时长一直无法核验（例如 >30 秒、URL 被 SSRF 拦截、文件不是 MP4），任务一定会挂满 24 小时，然后判失败、全额退款；
- 上游其实已经出片并扣费，用户却始终拿不到视频。

keli 工作树的做法是：提交后 120 秒内暂缓，超过就判失败并退款。由于视频生成通常超过 120 秒，这实际上等于第一次核验失败就判死。

## 1. 现状（FlowAPI）

### 1.1 核验逻辑

位置：`service/task_polling.go:538-549`。

```go
if bc := task.PrivateData.BillingContext; bc != nil && bc.BillingUnit == "output_seconds" && taskResult.Status == model.TaskStatusSuccess {
    seconds := taskResult.OutputVideoDuration
    if seconds <= 0 { seconds, err = ProbeWanMediaDuration(ctx, taskResult.Url, true) }
    if err != nil || math.IsNaN(seconds) || math.IsInf(seconds, 0) || seconds <= 0 || seconds+bc.InputVideoDuration > 30 {
        // A successful provider task is not a failed job merely because its
        // media is temporarily unavailable. Keep the reservation and retry polling.
        return fmt.Errorf("Wan output duration verification pending")
    }
    bc.OutputVideoDuration = seconds
}
```

- 时长的第一来源是上游的 `usage.output_video_duration`。值 <0 或 >30 时会被清零（`relay/channel/task/sora/wan.go:109-112`），转而下载成片探测。
- 探测函数 `ProbeWanMediaDuration`（`service/wan_video.go:23-89`）：
  - 只接受 HTTP(S)，并做 SSRF 校验（`:25-31`）；
  - 超时 45 秒；
  - 下载上限为 `MAX_FILE_DOWNLOAD_MB`（默认 64，见 `common/init.go:184`，封顶 100 MB，代码在 `:52-57`）；
  - 用 MP4 视频轨时长作为结果，≤0 或 >3600 视为失败（`:91-130`）。
- 函数 `return` 发生在 `task.Data` 赋值（`:551`）之前，所以**什么都不落库**：状态、进度、ResultURL 都保持上一次的值。
- 这个错误只由 `updateVideoTasks` 打一条 `LogError`（`:448-450`），每 15 秒一次。每一轮都会重新下载成片，单次最多 64 MB。

### 1.2 两类原因

| 类型 | 具体原因 | 后果 |
|---|---|---|
| 确定性（重试也不会变） | 输出 + 输入 > 30 秒；URL 被 SSRF 或域名规则拦截；非 HTTP(S) 地址；不是 MP4 或没有视频轨；超过下载上限 | 必然挂满 24 小时 |
| 暂时性 | 网络错误、5xx、超时 | 下一轮可能恢复 |

### 1.3 超时兜底

- `sweepTimedOutTasks`（`service/task_polling.go:74-123`）在每轮轮询开始时执行（`:148`），阈值默认 1440 分钟（`common/init.go:205`）。
- 到期后经 CAS 置为 FAILURE，原因写"任务超时（1440分钟）"，并通过 `RefundTaskQuota` 全额退还预扣（`:105-117`）。
- 如果 `TASK_TIMEOUT_MINUTES <= 0`，清扫关闭，任务永久挂起（`:75-77`）。

### 1.4 用户看到的状态

来源是 `GET /api/v1/tasks/:task_id`（`relay/relay_task.go:455-482`）。

- 挂起期间：
  - 若库里状态是 IN_PROGRESS，显示 `RUNNING`，否则显示 `PENDING`；
  - 没有 `video_url`，也没有 `usage`。
- 24 小时后：显示 `FAILED`，message 为"任务超时（1440分钟）"。
- 上游的成片地址从未写入 ResultURL，因此用户在任何阶段都拿不到视频。

### 1.5 计费

- **预扣**：`ModelPrice × 分组倍率 × 请求秒数 × 分辨率倍率`。
  - 秒数与分辨率倍率来自 `sora/adaptor.go:107-110` 和 `relay/relay_task.go:218-237`；
  - 请求秒数范围 2–30（`dto/wan.go:62-67`）；
  - 分辨率倍率：480P 0.5、720P 1、1080P 2（`dto/wan.go:108-117`）；
  - 清单中 `wan3.0-video` 为 0.6、`wan3.0-video-prime` 为 0.9（`target-pricing-proposal.json:58-59`）。
- **挂起期间**：预扣一直占用，不结算。
- **24 小时后**：全额退款。上游（Rolldek）已经报告成功，推定已计费（未确认），这笔成本由运营方承担。
- **正常路径**：按输出秒数结算，见 `service/task_polling.go:704-708` 和 `service/wan_video.go:134-160`。

### 1.6 测试把"挂起"固化为合同

`service/wan_video_test.go:145-170` `TestWanUnverifiedDurationPreservesReservation`，对"缺失输出时长"和"时长 31 秒"两种情况断言：

- 返回 `verification pending` 错误；
- 状态仍为 IN_PROGRESS；
- `task.Quota` 不变；
- 用户和令牌额度不变。

## 2. 与 keli 工作树的差异（未提交改动）

- keli `service/task_polling.go:515-528`：

  ```go
  if err != nil || seconds <= 0 || seconds+bc.InputVideoDuration > 30 {
      if task.InUnknownStatusGrace(time.Now().Unix()) {
          return fmt.Errorf("Wan output duration verification pending")
      }
      taskResult = relaycommon.FailTaskInfo("unable to verify output video duration")
  }
  ```

  宽限函数复用了 R3 的 `InUnknownStatusGrace`：提交后 ≤120 秒视为宽限期（keli `model/task.go:149-154`）。超过宽限就走 FAILURE 分支并退款。

- **宽限的起算点是提交时间，而不是"首次发现上游已成功"的时间。** 只要任务从提交到上游成功的耗时超过 120 秒，第一次核验失败就会立刻判失败，暂时性错误也不会重试。Wan 的实际耗时未确认。
- keli 还多了一个早返回：已经是终态的 Wan 任务直接跳过（keli `task_polling.go:471-473`）。
- keli 的 `wanActualQuota` 用的是裸 `int(math.Round(...))`，没有饱和审计。FlowAPI 已经改成 `wanActualQuotaChecked`（`service/wan_video.go:134-160`）。
- keli 的 `service/wan_video_test.go` 没有"时长无法核验"的测试。

### 参照：上游 new-api 的 DashScope Wan3 插件

`upstream/main:plugins/tasks/alibaba/plugin.js:1115-1135`：

- usage 缺少任一侧时，**保留预留额度**，即按提交时的秒数结算成功（注释原文："Partial Wan3 statistics cannot establish the total billable duration. Keep the reservation when either side is missing."）；
- 计费秒数是 `input_video_duration + output_video_duration`（测试见 `upstream/main:plugins/alibaba_responses_test.go:664-666`）。

## 3. 是否有意设计

**有意的部分：不判失败、保留预扣。**

- 代码注释：`task_polling.go:544-545`；
- 测试：`service/wan_video_test.go:145-170`；
- `.trellis/tasks/09-22-flowapi-video-adapter-alignment/design.md:9`："输出时长验证失败的 source 行为和账务后果在 fixture 明确，不假装拿到了 authoritative usage"；
- `.trellis/tasks/09-22-flowapi-provider-coverage/research/contract-review.md:11`："异常时长/下载失败不能标成已核验 usage"；
- `.trellis/tasks/09-22-flowapi-provider-coverage/design.md:28`："明确失败才退款"。

**没有记录的部分：**

- 任何文档都没有讨论"无上限等待 → 24 小时后退款 → 运营方承担上游成本、用户拿不到视频"这一最终结果；
- 也没有区分确定性原因和暂时性原因；
- `.trellis/spec/` 里没有 Wan 时长核验或 UNKNOWN 的相关约定：`backend/media-channel-deployment.md` 只规定了 Wan 只允许配置在 Rolldek 渠道。

**判断：**行为本身是有意的，不属于实现错误。但对确定性原因而言，"挂满 24 小时 + 运营方承担成本 + 用户拿不到结果"是一个未经权衡的业务缺陷。是否处理由用户决定。

## 4. 可选修复方案

| 方案 | 行为 | 风险 |
|---|---|---|
| A. 照搬 keli：提交后 120 秒内暂缓，之后判失败并退款 | 快速收敛 | 按提交时间计时，实际上"首次失败即判死"；暂时性下载失败也会丢片；上游成本仍由运营方承担 |
| B. 从"首次观察到上游成功"开始计时或计次：在 PrivateData 持久化首次时间或尝试次数，达到上限后二选一——B1 判失败并退款，B2 按预留结算成功并写回 URL | 暂时性错误有重试余地，且有上限 | 挂起路径目前什么都不落库，需要新增一次持久化写入；B1 的成本问题与 A 相同 |
| C. 与上游插件一致：无法核验时按预留结算成功，写回 URL，并审计（记录 admin_info 标记和日志） | 用户拿到视频，运营方不承担损失 | 实际输出短于请求时长时用户多付，但因请求 ≤30 秒，上界是确定的；"输出 + 输入 > 30"本属违约输出，也会按预留收费 |
| D. 按错误类型分流：确定性原因立即决断（走 A 或 C 的处理），暂时性原因有界重试 | 精细 | 需要给 `ProbeWanMediaDuration` 的错误分类（目前只返回泛化的错误文本） |
| E. 维持现状，在 PRD 记录 24 小时后退款 | 无代码改动 | 保留 1.3 至 1.5 所述的全部后果，外加每 15 秒重复下载一次 |

## 5. 建议的回归测试位置（现有文件）

- `service/wan_video_test.go:145-170` `TestWanUnverifiedDurationPreservesReservation`：固化了当前行为，改方案时需要同步修改。
- `service/wan_video_test.go:106-143` `TestWanBillingSnapshotAndRepeatedTerminal`：覆盖成功、失败和重复终态不重复计费。
- `service/task_polling_test.go:460` `TestSweepTimedOutTasksHonorsRefundRolloutBoundary`：24 小时兜底。
- `relay/channel/task/sora/wan_test.go`：解析时对 >30 秒的钳制（`wan.go:109-112`）。

## Caveats / 未确认

- Rolldek 成片 URL 的有效期、是否一定返回 `usage.output_video_duration`、以及对未交付的成片是否仍然计费：未确认。2026-09-25 的发布冒烟里有一次 2 秒成片核验通过（`09-22-flowapi-provider-coverage/release-gate-2026-09-25.md:18,27`）。
- 旁支观察（不在 R6 范围内）：FlowAPI 的 Wan 只按输出秒计费（`wan_video.go:134-160`），输入视频秒数只参与 30 秒上限校验；上游 DashScope Wan3 插件则按输入 + 输出计费。Rolldek 是否对输入视频计费，未确认。
- HK 生产的 `TASK_TIMEOUT_MINUTES` 与 `MAX_FILE_DOWNLOAD_MB`：未确认。
