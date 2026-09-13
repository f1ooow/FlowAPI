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

---

## 2026-09-14 — 图像模型按分辨率阶梯计费（09-13-image-resolution-pricing）

### 需求

6 个图像模型从一刀切单价改为按 1K/2K/4K 分辨率分档：`gpt-image-2`、
`gpt-image-2.5-flare`、`gpt-image-2.5-sunburst`、`gemini-3-pro-image-preview`、
`gemini-3.1-flash-image-preview`、`gemini-3.1-flash-image`。

原始约束是「纯配置、不得二开」，因为下游 keli api 网关也要配同一套规则。后因
multipart 判档缺口影响 88% 的编辑流量、且所有 workaround 方案都不干净，用户
确认下游同为己方部署，改为「以配置为主 + 最小代码改动」。

### 调查结论（57 次生产实测 + 上游账单 160 行）

- **判档只能按请求参数**。`img_o`（输出图 token）路线实测否决：`gpt-image-2`
  请求 1024x1024 得 img_o=1056、请求 4096x4096 得 659，值与分辨率反向；主力
  渠道 55 对 flare 的 usage 覆盖率仅 6.3%、对 gemini-3.1-flash 为 0%；上游
  自标 `"source": "estimated"`。
- **面积阈值**从两个独立来源交叉验证：上游账单每尺寸单价零方差反推，以及
  tuzi 后台日志审计字段暴露的上游表达式原文
  （`imagePixels(param("size")) > 3686400 → 4K`、`> 1048576 → 2K`）。
  不能用「最长边」——`2048x2048` 是最高档而 `2560x1440` 是中档，会判反。
- **gemini 走 OpenAI 兼容路径拿不到高分辨率**。11/11 实测：传
  `size=4096x4096`/`2048x2048`/不传，交付恒为 1024×1024。真正生效的是原生
  `generationConfig.imageConfig.imageSize`。用户拍板按客户端声明计费，接受
  这条路径「收 4K 价交 1K 图」。
- **multipart 是最大的缺口**。`/v1/images/edits` 占 `gpt-image-2` 请求的 75%
  （305/405）、flare 的 45%。`param()` 全为 nil 导致一律落兜底档，而 size
  照样转发给上游并被计费 —— 账单 61 条编辑请求中 flare 的 31 条每张亏 ¥0.04。

### 改动

两个仓库同一处修复：`ResolveIncomingBillingExprRequestInput` 在入站 body 为空
且 `info.Request` 为 `*dto.ImageRequest` 时投影已解析的 DTO，投影前剥掉
`Prompt/Image/Images/Mask/Extra`（实测 200KB → 117 字节）。

keli 额外补齐 `n` 上界（`MaxImageN = 128`）——该仓库 multipart 与 JSON 两个
分支都缺，而 JSON 分支的 `param("n")` 在投影落地前就已经是活的。FlowAPI 两条
分支本就有，无需改动。

### 踩到的坑

- **前后端是两套表达式解析器**。后端 Go 引擎认 `let`，前端「Token 估算器」是
  纯 JS `new Function` 沙箱、环境里没有 `param`，任何读请求参数的表达式都必然
  报错（`Unexpected keyword 'let'` / `param is not defined`）。不阻断保存。
- **`??` 只对 nil 生效，遇空串会卡住链条**。gemini 的 `imageSize` 五段探测链
  必须用显式 `!= ""`，否则 `imageSize:""` + 下划线路径有值时会漏判。
- **`n=0` 会免费放行**。校验层把 `n=0` 归一成 1 正常出图，但 `param("n")` 读到
  的原始值仍是 0，乘下去 quota 为 0 且不报错。表达式必须套 `max(…, 1.0)`。
- **单位写错静默失败**。`tier("1k", 0.12)` 算出 0 quota，图照出、钱不收、日志
  无异常。必须写「人民币售价 × 100 万」。

### 测试

keli：新增 353 行（3 个测试文件），`go build ./...` OK、
`go test ./relay/helper/...` ok 3.632s、`gofmt`/`go vet` 干净。
反向验证：stash 掉投影后 multipart `2048x1152 n=4` 预扣 60000/tier 1k，
恢复后 320000/tier 2k。

FlowAPI：新增 323 行（2 个测试文件），`go build ./...` OK、
`go test ./relay/helper/...` ok 2.633s、`gofmt`/`go vet` 干净、未触及 relaykit。

### Git Commits

| 仓库 | Hash | Message |
|------|------|---------|
| keli api | `570aba948` | fix(billing): 让 multipart 图像请求能进入阶梯计费表达式 |
| FlowAPI | `bc3e2edae` | fix(billing): 让 multipart 图像请求能进入阶梯计费表达式 |

### Status

[OK] **代码已提交并推送**（keli → `fork/feature/fulladaptor`，FlowAPI →
`origin/main`）。两边生产部署进行中。表达式配置由用户自行完成，6 条定稿文本
见桌面《图像模型分辨率阶梯计费-配置指南.md》与
`research/final-expressions-verification.md` 第 6 版。

### 部署结果（2026-09-14 00:13）

| | FlowAPI (HK) | keli api (广州) |
|---|---|---|
| 新镜像 | `flowapi:hk-multipart-billing-20260914-001321` | `new-api:image-multipart-billing-20260914-001329` |
| 回滚 tag | `flowapi:rollback-multipart-billing-20260914-001321` | `new-api:rollback-20260914-001329` |
| 备份 | `/opt/flowapi/backups/pre-multipart-billing-20260914-001321/` | `/opt/new-api/docker-compose.yml.bak-20260914-001329` |
| 健康检查 | 7s，外部 200 | 8s，连续 5 次 200，停服约 8s |
| QuotaPerUnit | 500000（编译默认，无覆盖） | 500000（`/api/status` 直读确认） |

### 部署带回的三处事实更正

1. **两边都没有任何图像模型在跑 `tiered_expr`**。此前我据「日志里有『阶梯计费/命中档位』」判断
   `gemini-3-pro-image-preview` 已在表达式路径上 —— 错了，那张截图是**上游 tuzi 后台**的日志，
   不是我方的。实际两边的图像模型全走 ModelPrice/ModelRatio：FlowAPI 的 gemini-3-pro 是
   ModelPrice 0.2；keli 的 gpt-image-2 / gemini-3-pro / flash-preview 是 0.1/0.1/0.08，
   flare/sunburst 走 ModelRatio 0.05/0.06。
   推论：配表达式是**从固定单价切到 tiered_expr**，回滚基线是 ModelPrice 的值；且
   「multipart 落兜底档导致亏损」目前并未实际发生 —— 压根没走表达式路径，那是切换后的敞口。
2. **`gemini-3.1-flash-image` 在 keli 上无任何价格配置**（ModelPrice / ModelRatio 均无条目），
   配表达式时须一并处理，否则会掉进按 token 计费。
3. **keli 的分组倍率记录过时**。生产实际 `default=1.5, image=1.2, video=1.2, Gemini=1.5,
   国产模型=6.8, Codex=0.5`，skill 里记的 `default=1.2 / video=1.1` 已失效，已更正
   `keli-api-ops/SKILL.md`。这 6 个模型在 `image` 分组，倍率 1.2。

另：keli 的 6 个图像模型全挂 channel 73「FLOW API 图片」(type=60)，即 keli → FlowAPI → tuzi
三层链路，各自独立计费。在 keli 上验证时 FlowAPI 侧也会记账。

server skill 的野草云-HK 台账此前滞后 4 次发布（记 `hk-monitoring-cards-20260911-135223`，
实际运行 `hk-all-fixes-20260911-204312`），已由部署 agent 更正并加「查当前镜像一律以
`docker ps` 为准」的告警。

### 剩余

表达式配置与验证由用户执行。核心验证点：图生图（multipart）传 `size=2048x1152`
应命中 `2k` 档（改动前只能落 `1k`）。
