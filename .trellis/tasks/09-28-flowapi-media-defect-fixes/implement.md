# Implement: 修复 FlowAPI 媒体渠道缺陷

A、B、C 三组并行实现。每组各派一个 trellis-implement，全部完成后派一个 trellis-check 做全量检查。

## A 组（同步图片 / 音频）

- [x] R1：删除 `relay/channel/volcengine/asr.go:408` 和 `:486` 的 `X-Api-Sequence`，新增 `asr_test.go`，断言请求头。
- [x] R2：
  - DTO 增加 `Tools`；
  - `image_handler.go` 非透传分支对非 Seedream 模型清空 `Tools`；
  - `normalizeSeedreamImagePayload` 对 Pro 删除 `tools`；
  - 补测试。
- [x] R5：`relay_image.go:91-111` 按成功项计费，补测试。
- [x] 验证：
  - `go test ./relay/channel/volcengine/... ./relay/channel/openai/... ./relay/... ./service/...`，只跑与 Seedream、ASR 相关的测试；
  - `cd relaykit && GOWORK=off go build ./... && GOWORK=off go test ./...`。

## B 组（MiniMax）

- [x] R4a：distributor 对 `/v1/files/upload` 按 purpose 选路由模型，补路由测试。
- [x] R4b：更新 purpose 白名单和按用途的大小上限，补表驱动测试。
- [x] R4c：`file_id` 同时接受数字和字符串，补测试。
- [x] R4d：克隆字段对齐官方，`clone_prompt.prompt_audio` 做资源归属校验，更新 `media_test.go`。
- [x] R4e：上游明确拒绝时释放 pending 的 voice_id，补测试。
- [x] R7a：TTS 用本地加权计数做上界并饱和，预扣也按加权计数估算，补测试。
- [x] R8：去掉音乐接口的占位名默认值，从模型列表和计费单位表中删除，补测试。
- [x] 验证：`go test ./relay/channel/minimax/... ./middleware/... ./controller/... ./model/...`，只跑 MiniMax、media、distributor 相关的测试。

## C 组（异步任务）

- [x] R3：新增 `TaskUnknownGraceAdaptor`，ali 实现；修改轮询的 UNKNOWN 分支，补测试。
- [x] Wan：
  - 参考视频按 URL 去重、每段向上取整；
  - 预扣加上参考视频秒数；
  - 结算用「成片 + 参考」，缺时长时按请求时长；
  - 新任务写 `wan_input_billed` 标记，旧任务按旧公式结算；
  - 补测试。
- [x] H3：
  - 2K 倍率改为 5/3，额度精确；
  - 带参考视频时预扣加 15 秒；
  - 实现 `TaskBillingCheckedAdaptor` 读 `task.usage` 结算；
  - 旧任务不调整；
  - 补测试。
- [x] 验证：`go test ./service/... ./relay/channel/task/... ./relay/...`，只跑与任务、Wan、H3、ali 相关的测试。

## 全部完成后

- [x] `go build ./...`，`go vet` 覆盖受影响的包。
- [x] 受影响包全量 `go test`（另跑 -race 通过）。
- [x] `cd relaykit && GOWORK=off go build ./...`。
- [x] `git diff --check`。
- [x] 更新 `.trellis/spec/backend/` 中与本任务相关的规范（`media-channel-deployment.md`「Media settlement contracts」）：
  - Seedream 图层计费合同（取代归档任务里的 `billing-contract.md:22-23`）；
  - H3 和 Wan 的计费口径；
  - UNKNOWN 宽限。
- [ ] 在 PRD 验收清单上打勾，写入上线清单。

## 风险点

- `service/task_polling.go` 只由 C 组修改。A、B 两组不得改这个文件。
- `middleware/distributor.go` 只由 B 组修改。
- 测试不得依赖真实网络，也不得依赖 sleep 计时；时间用可注入的 now 或 SubmitTime。
