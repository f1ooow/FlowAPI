# Implement：子项 B 渠道可靠性设置存读一致

子项 A 已完成，不在本清单内。按顺序执行，采用 TDD：先写会失败的回归测试，再改代码。

## 清单

1. [ ] 回归测试先行（使用 testify）：读取来源、中继超时来源、服务端校验、`SetOtherSettings` 回写保留、竞速资格（无粘连可竞速、有粘连不竞速）。确认它们在旧代码上失败，竞速"有粘连不竞速"一项除外，它锁定的是不变行为。
2. [ ] `relaykit/dto/channel_settings.go`：给 `ChannelOtherSettings` 新增 `Reliability`，从 `ChannelSettings` 删除 `Reliability`。
   - 立即验证：`cd relaykit && GOWORK=off go build ./...`
3. [ ] `model/channel.go`：
   - `GetReliabilitySettings()` 改读 `GetOtherSettings()`。
   - `ValidateSettings` 改为校验 `settings.reliability`，删掉旧的 `setting.reliability` 校验。
4. [ ] 中继超时读取改为 `info.ChannelOtherSettings.Reliability`：`relay/helper/stream_scanner.go`（流式静默超时）、`relay/channel/api_request.go`（非流式总超时）。
5. [ ] 更新锁定旧 `setting` 列的测试（见 `design.md`「测试要点」），用 `settings` 构造同样的场景，不删掉原有保护。
6. [ ] 全局搜索 `ChannelSetting.Reliability` 和 `setting.reliability`，确认非测试代码中已无残留。

## 验证命令

- `gofmt -l relaykit model controller relay service`
- `go vet ./model/... ./controller/... ./relay/... ./service/...`
- `go build ./...`
- `cd relaykit && GOWORK=off go build ./...`
- `go test ./model/... ./controller/... ./relay/... ./service/... -count=1`
- `go test -race ./controller/... -run 'Hedge' -count=1`
- `go test -race ./relay/helper/... -run 'Gate|Stream' -count=1`

## 风险点与回滚

- 风险文件：`relaykit/dto/channel_settings.go`（跨模块 DTO）、`model/channel.go`（每个请求都会读取）。
- 不做任何数据迁移，回滚就是回退提交。
- 子项 A 的改动在 `relay/helper/stream_scanner.go` 里尚未提交；改这个文件时只动超时读取那一行，不要碰闸门逻辑。

---

# Implement：子项 C 移除竞速 + 子项 D 静默超时全局默认

按顺序执行，先 C 后 D，都在 A、B 未提交的改动之上进行。行号以 `research/hedge-removal.md` 为准，A、B 的改动会让行号偏移。

## 清单

1. [ ] **C 后端**：按 `research/hedge-removal.md` 删除竞速控制流、计费、日志和 `RelayInfo.Hedge`；从 `controller/relay.go` 移除竞速入口。保留 C7 列出的通用改动。
2. [ ] **C 闸门**：删除 `relay/helper/stream_scanner.go` 里的竞速分支，包括 R5 特例。子项 A 的放行逻辑保持不变，普通尝试相关的测试保留。
3. [ ] **C 设置**：删除 `bill_hedge_losers` 和 `FirstContentTimeoutSeconds`，含校验、默认值与测试。
4. [ ] **C 测试**：删除竞速测试，并按 C8 补上三项更小的测试。
5. [ ] **C 前端**：删除首内容阈值字段、输家计费开关、日志里的竞速展示和竞速专用文案；更新相关前端测试。
6. [ ] **D**：在 `stream_scanner.go` 里加入静默超时的全局回退，先写测试：渠道 0 时用全局值，渠道为正数时用渠道值，放行前可重试，放行后不重放。
7. [ ] **D 前端**：把两项超时的说明文字改成"0 表示使用全局默认"，并同步各语言文案。
8. [ ] **全局搜索**：确认非测试代码里已经没有竞速符号。

## 验证命令

- `gofmt -l relaykit model controller relay service setting common logger`
- `go vet ./...`
- `go build ./...`
- `cd relaykit && GOWORK=off go build ./...`
- `go test ./... -count=1`
- `go test -race ./relay/... ./controller/... -count=1`
- 前端（`web/`）：`bun run typecheck`，`bun run build`，`bun run i18n:sync`，以及受影响的测试

## 风险与回滚

- A、B、C 三者都会改 `stream_scanner.go` 和 `stream_scanner_test.go`，B、C 都会改 `api_request.go` 和 `deadline_body_test.go`。改动时逐处核对，不要回退 A、B 的内容。
- 部署分支 `codex/flowapi-provider-coverage` 在 `channel-form.ts`、`details-dialog.tsx`、`zh.json`、`static-keys.ts` 里与竞速相关的行附近有改动，部署前合并时要留意冲突。
- 回滚：回退 C、D 的提交。不涉及数据迁移。
