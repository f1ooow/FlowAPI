# Research: 生产只读查证（H3 usage、Wan 时长与上游模型、HappyHorse UNKNOWN、HK 环境变量）

- **Query**：R3 / R6 / H3 调研里标为"未确认"的生产事实，外加 Wan 上游模型名与渠道 83 配置（主线追加）
- **Scope**：生产只读——keli 广州 `ssh root@8.138.196.139`（`docker exec -i postgres psql -U root -d new-api`）、FlowAPI HK `ssh hk`（`flowapi-postgres`），另对照少量本地代码
- **Date**：2026-09-28

## 方法与约束

- 每段 SQL 都包在 `BEGIN READ ONLY; ... ROLLBACK;` 里，只用 SELECT。没有写库、改配置、重启服务，也没有发任何 API 请求。
- 没有查询 `channels.key` 和 `tokens.key`。HK 任务的 `private_data` 里有一个 `key` 路径，只列出了路径名，从未读取它的值。渠道 `settings` 按路径展开时，路径名含 auth、key、token、secret、password 的叶子值一律打码。
- `tasks.data` 里的成片 URL 都带签名查询串，本文只写域名。
- 应用日志只统计匹配行数，或者输出脱敏后的单行。

## 结论速览

1. **H3**：keli 渠道 62 共 119 条任务（不是约 80 条），全部成功。119 条都带 `data.task.usage`，四个字段齐全。`input_seconds` 全部为 0，`output_seconds` 与请求时长 119/119 一致，`total_seconds = input + output` 也是 119/119 成立。
2. **Wan**：keli 渠道 75 的 4 条任务和 HK 渠道 83 的 1 条冒烟任务，Rolldek 查询响应里都**没有** `usage`，也没有任何时长字段。已计费的输出秒数只可能来自下载成片探测，与请求秒数完全一致。上游模型名依次为 `wan3.0-video-prime-480p`、`wan3.0-video-480p`、`wan3.0-video-720p` ×2，HK 冒烟为 `wan3.0-video-480p`。HK 渠道 83 的 `model_mapping` 是 JSON `null`，`settings` 只有 `{"video_protocol":"wan-rolldek-v1"}`。
3. **HappyHorse**：`stayed UNKNOWN` 从未出现，渠道 48、61 为 0 条，整张 tasks 表也是 0 条。宽限期内遇到 UNKNOWN 的 warn 日志同样为 0，但日志只覆盖 2026-09-04 之后的 4 条任务。状态统计：渠道 48 成功 4、失败 2，渠道 61 成功 51。
4. **HK 环境变量**：`TASK_TIMEOUT_MINUTES` 和 `MAX_FILE_DOWNLOAD_MB` 都没有设置，按代码默认值生效，分别是 1440 分钟和 64 MB。

## 1. H3（MetaSo）查询响应里的 usage（keli 渠道 62）

### 1.1 结构

- 119 条的 `data` 都是 object，顶层只有一个键 `task`。**顶层没有 `usage`**，usage 在 `data.task.usage`。
- `data.task` 的键（括号内为出现条数）：`content`（119，内部只有 `url`）、`created_at`、`duration`、`estimated_remaining_seconds`、`id`、`modality`、`model`、`ratio`、`resolution`、`status`、`task_type`、`updated_at`、`usage`（以上均为 119）、`title`（95）。
- `data.task.usage` 的键：`input_image_count`、`input_seconds`、`output_seconds`、`total_seconds`，119 条都有，类型都是 number。
- 上游字段取值一致：`status=succeeded`、`task_type=generation`、`modality=video`、`model=MiniMax-H3`。
- `data` 是原始查询响应，只剥掉了 `response.bytesBase64Encoded`（keli `service/task_polling.go:529` 的 `redactVideoResponseBody`，函数定义在 `:653-677`）。由此可以确认 MetaSo 原样透传了 `task.usage`。
- 成片域名：`task.content.url` 与 `private_data.result_url` 都是 `files.metaso.cn`，都带签名查询串。

### 1.2 取值分布（119 条）

| 字段 | 分布 |
|---|---|
| `input_seconds` | 0：119 条。**大于 0 的：0 条** |
| `output_seconds` | 4：9，5：7，6：1，8：5，10：29，13：3，14：4，15：61 |
| `total_seconds` | 与 `output_seconds` 逐值相同；`total = input + output` 119/119 成立 |
| `input_image_count` | 0：12，1：50，2：37，3：20。大于 0 的 107 条，大于 5 的 0 条 |
| `task.duration` | 与 `output_seconds` 119/119 相等 |

- 按分辨率：
  - 768P 共 99 条（16:9 85、3:4 6、adaptive 5、9:16 2、4:3 1），输出秒数 4：9、5：7、6：1、8：5、10：28、13：3、14：4、15：42；
  - 2K 共 20 条（16:9 19、3:4 1），输出秒数 10：1、15：19。
- **两类秒数都是 0 或 ≥4 的整数，没有出现小数。**

### 1.3 `output_seconds` 与请求时长

- 请求体没有落库：`properties` 只有 `input`、`origin_model_name`、`upstream_model_name` 三个键，而 `properties.input` 在 119 条里长度都是 0。
- 因此用计费快照 `private_data.billing_context.other_ratios["hailuo-h3-duration"]` 作为请求时长。keli 取的是请求里的 `duration`，缺省时取 5（keli `relay/channel/task/hailuo/adaptor.go:510-523`）。
- 结果：快照秒数与 `usage.output_seconds` **119/119 相等，不一致 0 条**。请求时长的分布与 1.2 中的输出秒数完全相同。
- 旁证：计费快照都是 `model_price=0.19`、`group_ratio=1.2`、`per_call_billing=true`、`billing_source=wallet`。2K 的分辨率倍率是 `1.5263157894736843`（即 29/19），共 20 条，时间在 2026-08-08 至 08-28；768P 的倍率是 1，共 99 条，时间在 08-07 至 09-24。`quota = round(0.19 × 1.2 × 秒数 × 分辨率倍率 × 500000)` 119/119 成立。**keli 的任务里没有出现过 1.25 倍率**：最后一条 2K 任务在 2026-08-28，早于 1.25 那次改动。

### 1.4 样本（最近一条 2K，id、title 和 URL 已脱敏）

```json
{"task": {"id": "<redacted>", "model": "MiniMax-H3", "ratio": "16:9", "title": "<redacted>",
  "usage": {"input_seconds": 0, "total_seconds": 15, "output_seconds": 15, "input_image_count": 1},
  "status": "succeeded", "content": {"url": "https://files.metaso.cn/<redacted>"},
  "duration": 15, "modality": "video", "task_type": "generation",
  "created_at": 1787888829, "resolution": "2K", "updated_at": 1787889041, "estimated_remaining_seconds": 0}}
```

最近一条 768P 的结构相同：usage 为 `{0, 5, 5, 0}`，`duration` 为 5。

### 1.5 查询依据

| 查询摘要 | 结果行数 |
|---|---|
| `SELECT channel_id, platform, count(*) FROM tasks WHERE channel_id IN (48,61,62,75) GROUP BY 1,2` | 4 行（62 → 119 条） |
| 渠道 62 按 `status, action` 分组 | 1 行（SUCCESS / generate 119） |
| `jsonb_object_keys(data)`、`jsonb_object_keys(data->'task')`、`jsonb_object_keys(data->'task'->'usage')` 键频次 | 1 / 14 / 4 行 |
| 四个 usage 字段与 `task.duration` 的取值分布（UNION ALL） | 29 行 |
| 一致性计数（`input>0`、`total=in+out`、`out=duration`、`img>0`、`img>5`） | 1 行 |
| 快照 `hailuo-h3-duration` × `output_seconds` 交叉分组，以及相等计数 | 8 行 + 1 行 |
| 分辨率 × 快照倍率 × 价格分组 | 2 行 |
| URL 域名分组（只截取 host） | 1 行 |
| 应用日志 `grep -c "reference_video"`、`grep -c "mm_file://"` | 均为 0 |

### 1.6 未确认

- **这 119 条里有没有带参考视频：未确认。** 请求体没有落库，应用日志也没有请求体（日志只覆盖 2026-09-04 之后，其中 H3 任务 17 条）。所以 `input_seconds` 全为 0 有两种解释：一是没人用过参考视频，二是 MetaSo 不回报输入秒数。现有数据无法区分。
- HK FlowAPI 的 tasks 表没有 H3 任务，整张表只有 1 条 Wan 冒烟，所以 H3 的证据只来自 keli。

## 2. Wan（Rolldek）查询响应里的输出时长，以及上游模型名

### 2.1 逐条任务（keli 渠道 75 的 4 条 + HK 渠道 83 的 1 条冒烟）

| 实例 / id | 提交（UTC） | 提交到完成（秒） | 公开模型 | **上游模型名** | 分辨率 | 请求秒数 | 计费输出秒数 | 预扣 quota |
|---|---|---|---|---|---|---|---|---|
| keli 339 | 2026-09-16 10:43:35 | 329 | wan3.0-video-prime | `wan3.0-video-prime-480p` | 480P | 2 | 2 | 540000 |
| keli 340 | 2026-09-17 03:26:38 | 314 | wan3.0-video | `wan3.0-video-480p` | 480P | 2 | 2 | 360000 |
| keli 341 | 2026-09-17 06:01:36 | 298 | wan3.0-video | `wan3.0-video-720p` | 720P | 7 | 7 | 2520000 |
| keli 342 | 2026-09-18 03:34:10 | 249 | wan3.0-video | `wan3.0-video-720p` | 720P | 5 | 5 | 1800000 |
| HK 1 | 2026-09-24 17:08:28 | 130 | wan3.0-video | `wan3.0-video-480p` | 480P | 2 | 2 | 360000 |

- 5 条都是 `SUCCESS / 100%`，上游 `status=completed`、`progress=100`、`object=video`。按上游 `completed_at - created_at` 计的耗时依次为 326、247、287、210、104 秒。
- **上游模型名的来源**：`properties.upstream_model_name` 与 `data.model` 在 5 条里逐条相同。它的格式是"公开模型名 + 小写分辨率"，与代码一致：FlowAPI `relay/channel/task/sora/wan.go:70-71`，keli 同名文件 `:71`。拼接发生在 `BuildRequestBody`（`relay/relay_task.go:240`）里，晚于渠道模型映射（`:173`），所以即使配置了映射也会被覆盖。
- **分辨率 / 秒数的来源**：计费快照 `billing_context` 的 `resolution`、`other_ratios.seconds`、`other_ratios.resolution`（480P 为 0.5，720P 为 1）、`output_video_duration`。价格：prime 为 0.9，其余为 0.6。分组倍率都是 1.2，HK 另有 `effective_ratio=1.2`。

### 2.2 查询响应里有没有时长

- 5 条的 `data` 键路径完全相同：`completed_at`、`created_at`、`id`、`metadata.url`、`model`、`object`、`progress`、`status`。
- **没有 `usage`，也没有 `output_video_duration`、`seconds`、`duration` 或其他任何时长字段。**
- `data` 是去掉 base64 后的原始查询响应（FlowAPI `service/task_polling.go:551`，keli `:529`），所以可以认定 Rolldek 的查询响应不带时长。
- 成片域名：`metadata.url` 与 `private_data.result_url` 都是 `dashscope-a717.oss-accelerate.aliyuncs.com`，都带签名查询串。
- **推论（依据代码）**：
  - 没有 usage 时，`OutputVideoDuration` 为 0（FlowAPI `sora/wan.go:96-112`），于是轮询会下载成片，按 MP4 视频轨时长探测（FlowAPI `service/task_polling.go:539-541`，keli `:515-519`）。
  - 计费秒数只在一个地方赋值：FlowAPI `service/task_polling.go:548`，keli `:526`。
  - 因此快照里的 2 / 2 / 7 / 5 / 2 都是探测结果，与请求秒数 5/5 一致。
- **首次核验都成功了**：
  - keli 日志目录 `/opt/new-api/logs` 覆盖 2026-09-04 22:05 CST 至今，包含这 4 条任务。其中 `Wan output duration verification pending` 为 0 次，`unable to verify output video duration` 为 0 次。
  - HK 的 `flowapi_flowapi_logs` 卷有 57 个日志文件（2026-08-27 至 2026-09-26）。`verification pending` 为 0 次；冒烟所在文件 `oneapi-20260925010530.log` 中 `Failed to update video task` 为 0 次。
  - keli 提交这 4 条时运行的镜像是 `new-api:wan3-20260916`。对镜像快照里的二进制做只读 grep，含 `Wan output duration verification pending` 字符串，说明这段核验代码在运行版本里。

### 2.3 参考视频 / 参考图 / 参考音频

- 请求体没有落库：5 条的 `properties.input` 长度都是 0。
- **参考视频：按代码推断没有。**
  - 5 条的 `billing_context` 都没有 `input_video_duration` 键。这个字段带 `omitempty`（FlowAPI `model/task.go:123`，keli `:116`），没有键就说明值为 0。
  - 这个值等于提交时各段 `reference_video` 探测秒数之和，每段必须在 1–15 秒之间，否则直接 400（FlowAPI `sora/wan.go:43-63`）。写入位置在 FlowAPI `controller/relay.go:784` 和 `relay/relay_task.go:253`。所以只要带了参考视频，这个值就一定 ≥ 1。
  - 前提是运行版本里有这段代码：keli `wan3-20260916` 的二进制含 `wan_input_video_seconds`；HK 冒烟所用的 `8156149df` 版本中，上述文件已包含这段代码（`git show 8156149df:<file>` 可以查到）。
- **参考图（含首尾帧）、参考音频：未确认。** 没有任何字段记录它们。keli 的消费日志 `other` 只有 `billing_unit`、`group_ratio`、`is_task`、`model_price`、`request_path`、`resolution`、`seconds`，内容形如"Wan 输出秒数预扣：2s，480P"。HK 的冒烟日志 `other` 只有 `admin_info.billing_ratios.*`、`group_ratio`、`is_task`、`model_price`、`request_path`。

### 2.4 HK 渠道 83 的 `model_mapping` 和 `settings`（只选这两列）

- `model_mapping`：不是 SQL NULL，而是文本 `null`（JSON null），即**没有模型映射**。
- `settings`：按路径展开后只有一个叶子，内容是 `{"video_protocol": "wan-rolldek-v1"}`，没有其他配置，也没有凭据类字段。
- 对照 keli 渠道 75：`model_mapping` 是 SQL NULL，`settings` 同样只有 `video_protocol=wan-rolldek-v1`。
- HK 任务的 `private_data.channel_settings` 快照里也只有 `video_protocol=wan-rolldek-v1`。

### 2.5 查询依据

| 查询摘要 | 结果行数 |
|---|---|
| keli 渠道 75：状态、进度、时间、`data` 长度 | 4 行 |
| keli 渠道 75：`data`、`private_data`、`properties` 递归键路径（只看结构） | 9 行 + 18 行 |
| keli 渠道 75：上游 status / model / 耗时 / URL 域名 | 4 行 |
| keli 渠道 75：计费快照与 `upstream_model_name` | 4 行 + 4 行 |
| keli `logs WHERE model_name LIKE 'wan3.0%'`：`other` 键与内容 | 7 行 + 4 行 |
| HK tasks 全表概览 | 1 行（只有渠道 83 的 1 条） |
| HK 渠道 83 任务：递归键路径（`private_data.key` 只列路径、不读值） | 36 行 |
| HK 渠道 83 任务：上游字段、计费快照、上游模型名 | 1 行 + 1 行 |
| HK `logs WHERE model_name LIKE 'wan3.0%'` | 1 行 |
| HK `SELECT model_mapping FROM channels WHERE id=83`；`settings` 路径展开（敏感路径打码） | 1 行；2 行 |
| 应用日志 `grep -c`（keli 15 个文件，HK 57 个文件） | 计数均为 0 |

## 3. HappyHorse 的 UNKNOWN 宽限实际触发过没有（keli）

### 3.1 结论

**没有触发过判死。** 渠道 48、61 的失败任务中，`fail_reason` 含 `stayed UNKNOWN` 的有 0 条；把范围放宽到"含 `unknown`，不区分大小写"也是 0 条。整张 tasks 表（345 条）里含 `stayed UNKNOWN` 的同样是 0 条。因此不存在"判死之后上游又出片"的问题，这一项不适用。

### 3.2 状态统计（ali 平台 17）

| 渠道 | 状态 | 条数 | 上游 `output.task_status` | 提交时间范围 | 提交到完成（秒）min / p50 / max | 其中超过 120 秒 |
|---|---|---|---|---|---|---|
| 48 Happy horse | SUCCESS | 4 | SUCCEEDED | 2026-05-19 ~ 06-05 | 115 / 116 / 172 | 2 |
| 48 | FAILURE | 2 | FAILED | 2026-05-19 | 6 / 6 / 9 | — |
| 61 Happy horse (ZIV） | SUCCESS | 51 | SUCCEEDED | 2026-08-04 ~ 09-15 | 71 / 259 / 579 | 48 |
| 55（渠道已删，不在 48/61 范围内，仅作参照） | SUCCESS | 44 | SUCCEEDED | 2026-06-05 ~ 07-23 | 97 / 186 / 475 | 40 |

- 渠道 48 的两条失败原因都是上游明确返回的 `FAILED`：
  - `task failed, code: DataInspectionFailed , message: Green net check failed for image input`
  - `task failed, code: InvalidParameter , message: Field required: input.media`
- 这两条与 UNKNOWN 无关。
- 上游模型分布：
  - 48：`happyhorse-1.0-r2v` 6 条；
  - 61：`happyhorse-1.1-r2v` 42、`1.1-t2v` 5、`1.1-i2v` 4；
  - 55：`1.0-r2v` 28、`1.1-r2v` 15、`1.0-t2v` 1。
- 101 条 ali 任务的 `data`（最后一次查询响应）里都没有 `UNKNOWN` 字样（`data::text ILIKE '%UNKNOWN%'` 为 0 条）。

### 3.3 宽限内的 UNKNOWN 是否出现过（辅助证据，覆盖面有限）

- **运行版本确实含宽限代码。**
  - keli 当前运行的二进制与 `wan3-20260916` 镜像二进制，都含有 `stayed UNKNOWN for more than` 和 `UNKNOWN within grace period` 两个字符串（只读 grep 镜像快照里的二进制，以及 `docker exec new-api grep -c`）。
  - keli 库中 `de16bfc5f` 是 `7b8e13f1c`（HappyHorse 1.1 定价，07-20）和 `eac2a27d7`（H3 接入，09-10）的祖先。
  - 但宽限代码最早是哪个镜像上线的：**未确认**。keli-canvas 任务 `07-10-happyhorse-video-fail-debug/prd.md:39` 记录的是"未 push 未部署"。
- **日志：没有出现。**
  - keli 应用日志只覆盖 2026-09-04 22:05 CST 以后，窗口内只有 4 条 HappyHorse 任务：id 317、321、328、333，都是渠道 61，都成功，耗时 71 / 81 / 188 / 81 秒。
  - 窗口内 `UNKNOWN within grace period` 为 0 次，`UNKNOWN` 字样为 0 行。
  - 宽限 warn 走 `LogWarn`，它不受 debug 开关限制（keli `logger/logger.go:80-82`），因此确实会写进这些日志文件。
- 窗口之外的 97 条 ali 任务期间，有没有出现过短暂 UNKNOWN：**未确认**，因为没有日志。已知的只有一点：这些任务最终都是 SUCCEEDED 或 FAILED，没有一条以 UNKNOWN 收场。

### 3.4 查询依据

| 查询摘要 | 结果行数 |
|---|---|
| `SELECT channel_id, status, count(*) ... WHERE channel_id IN (48,61) GROUP BY 1,2` | 3 行 |
| `count(*) WHERE channel_id IN (48,61) AND fail_reason ILIKE '%stayed UNKNOWN%'`；`'%unknown%'` | 均为 0 |
| 全表 `fail_reason ILIKE '%stayed UNKNOWN%'` | 0 |
| 渠道 48/61 失败原因分组 | 2 行 |
| `platform='17'` 按渠道、状态、上游 task_status 分组 | 4 行 |
| `platform='17' AND data::text ILIKE '%UNKNOWN%'` | 0 |
| 耗时分布（按渠道） | 3 行 |
| `channels` 只选 `id, name, type, status, group, models`，id 取 48/55/61/62/75 | 4 行（55 不存在） |
| keli 日志 `grep -c`：`UNKNOWN within grace period` / `stayed UNKNOWN` / `UNKNOWN` | 均为 0 |

## 4. HK FlowAPI 的两个环境变量

- 命令 `ssh hk "docker exec flowapi printenv TASK_TIMEOUT_MINUTES MAX_FILE_DOWNLOAD_MB"`：没有任何输出，退出码 1。
- 逐个变量执行 `printenv` 并丢弃输出，退出码都是 1，即**两个变量都没有设置**。
- 容器是 `flowapi`，镜像 `flowapi:hk-channel-ui-10d32a657-20260926`，状态 healthy。
- 应用会从工作目录读取 `.env`（`main.go:283`），而工作目录 `/data` 下没有 `.env`，这一途径也排除。
- 所以按代码默认值生效（`common/env.go:9-19`，未设置或为空时返回默认值）：
  - `TASK_TIMEOUT_MINUTES = 1440`（`common/init.go:205`）：超时清扫开启，任务提交 24 小时后判失败并退款；
  - `MAX_FILE_DOWNLOAD_MB = 64`（`common/init.go:184`）：Wan 成片与参考媒体的探测下载上限为 64 MB（`service/wan_video.go:51`）。

## Caveats / 未确认

- H3：这 119 条有没有带参考视频无法确认，所以 `input_seconds` 全为 0 不能证明 MetaSo 会（或不会）回报参考视频秒数。
- Wan：
  - Rolldek 查询响应不带任何时长字段，这一点来自 5 条样本，全是 480P/720P、2–7 秒、无参考视频的任务，没有覆盖 1080P、带参考视频或长时长的情形；
  - 参考图和参考音频的使用情况：未确认。
- HappyHorse：
  - 2026-09-04 之前的 97 条 ali 任务期间是否出现过短暂 UNKNOWN：未确认；
  - keli 宽限代码最早的上线时间：未确认。
- 任务说明里估计渠道 62 约 80 条，实际是 119 条（2026-08-07 ~ 09-24）。
