# Journal - zhuyu (Part 1)

> AI development session journal
> Started: 2026-08-26

---



## Session 1: HK 渠道成本倍率部署与任务归档

**Date**: 2026-08-28
**Task**: HK 渠道成本倍率部署与任务归档
**Branch**: `main`

### Summary

完成渠道成本倍率、用户分组最终倍率区间和用户侧隐私收敛；提交 70bb3614，部署到 HK flowapi.robusta.top，备份 PostgreSQL/compose，验证健康与 ratio_sync 404，并更新 HK/RC 服务器台账。

### Main Changes

- 新增渠道成本倍率与分组计入开关，统一同步/异步计费快照和最终倍率展示
- 用户日志与公开分组接口不暴露渠道成本及内部拆分因子
- FlowAPI 生产部署目标更正为 HK，RC 标记为旧副节点并保留回滚

### Git Commits

| Hash | Message |
|------|---------|
| `70bb3614` | (see git log) |

### Testing

- [OK] go test ./...、relaykit 独立构建、frontend typecheck/build、倍率相关 Vitest
- [OK] HK app/PostgreSQL/Redis healthy，公网 /api/status 成功，/api/ratio_sync/channels 返回 404

### Status

[OK] **Completed**

### Next Steps

- 确认 fallback.robusta.top 的旧 RC 入口下线窗口后再单独停用 RC 实例


## Session 2: Integrate and deploy GPT Image Playground

**Date**: 2026-08-28
**Task**: Integrate and deploy GPT Image Playground
**Branch**: `main`

### Summary

Implemented the authenticated Flow API GPT Image Playground at /playground with user-token selection, generations/edits Image API calls, IndexedDB history, responsive gallery, detail/re-edit/download/delete flows, tests, and HK deployment as flowapi:hk-gpt-image-playground-941298f6. Archived task 08-28-gpt-image-playground.

### Git Commits

| Hash | Message |
|------|---------|
| `9f131ab7` | (see git log) |

### Status

[OK] **Completed**


## Session 3: Global channel passthrough completed

**Date**: 2026-09-11
**Task**: Global channel passthrough completed
**Branch**: `main`

### Summary

Implemented independent global body/header passthrough settings and inherited channel UI. Backend tests, six frontend tests, typecheck, scoped lint, build and desktop/mobile browser checks passed. Committed only this feature; unrelated work preserved. No deployment.

### Git Commits

| Hash | Message |
|------|---------|
| `0e391d3e0` | (see git log) |

### Status

[OK] **Completed**


## Session 4: Real-traffic availability monitoring + pending-work review

**Date**: 2026-09-11
**Task**: 09-10-group-monitor-log-aggregation (parent) + two children
**Branch**: `main`

### Summary

Replaced the synthetic-probe group monitor with real-traffic aggregation over
`perf_metrics`, added an admin-only channel availability view backed by a new
attempt-level `channel_metrics` table, and — at the user's request — reviewed and
fixed the entire uncommitted working tree (four other sessions' worth) before
committing everything as one buildable change.

Probe removal restored `controller/channel-test.go` byte-identical to HEAD: the
probe had grown `groupOverride`/`recordLog` parameters that no longer had any
non-default caller. Cleaning 17 unreachable `IsChannelTest` guards also reduced
`service/billing.go`, `tiered_settle.go`, `violation_fee.go` and `text_quota.go`
to a zero diff against HEAD.

### Blockers found in the pending work and fixed

- Non-gated streaming channels (aws/baidu/xai/dify/openai-audio/cohere/zhipu/
  ollama/...) had already written bytes but were treated as uncommitted, so a
  retry appended a second complete SSE stream to the same response body.
- Legitimate empty completions (`finish_reason` with no content, usage-only tail
  frames, proxy heartbeats) were classified `empty_stream` and replayed across
  every channel before failing.
- `doRequest` set SSE headers unconditionally, so a pre-commit 502 came back as
  `text/event-stream` and the ping keepalive flushed a 200 that swallowed the
  error body entirely.
- `route_history` sat at `other` top level, outside `formatUserLogs` redaction —
  any user could read channel ids/names/priorities/weights off their own logs.
- Global header passthrough shipped without 15 blacklist entries. The upstream
  account group is a writable control plane, not merely a leak: override runs
  after adapter setup, so a client could overwrite `OpenAI-Organization`.

### Corrections to my own planning

- design.md put the integer-division `FLOOR` branch on PostgreSQL. It belongs on
  MySQL, whose `/` yields DECIMAL and would produce no rollup at all. Caught by
  the implementing agent against the repo's own `usedata_rankings.go` precedent.
- I specified `processChannelError` as the attempt sampling point; it is also
  called from `channel-test.go:945`, which would have recorded synthetic channel
  tests against real availability.
- `ratio_sync` initially looked like abandoned residue. It is the deliberate
  restoration required by task 08-26 after `70bb36145` removed it by mistake.

### Tests

Backend: 33 packages, 0 failures. Frontend: 55 files / 218 passed; the 8 failures
are the pre-existing `storage.setItem` cases in keys/redemption-codes. copyright
and format checks clean; lint holds at the pre-existing 268 baseline.

### Git Commits

| Hash | Message |
|------|---------|
| `845a85998` | feat: real-traffic availability monitoring and relay hardening |

### Status

[OK] **Code committed** — deployment to HK pending in this session.
