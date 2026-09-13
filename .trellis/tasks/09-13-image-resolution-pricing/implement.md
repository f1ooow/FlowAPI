# 执行计划 — 图像模型按分辨率阶梯计费

两个仓库、四个阶段。**keli api 先行验证，FlowAPI 后同步。**

---

## 阶段 1 — keli api 代码改动

仓库：`/Users/Zhuyu/Documents/Code/new-api`，分支 `feature/fulladaptor`

> ⚠️ 该仓库 remote 规则：**只 `git push fork`，绝不 push origin**（origin 是上游 `QuantumNous/new-api`）。

- [x] **1.1 补齐 multipart 的 `n` 上界校验**（安全前置，必须先做）
  - `relay/helper/valid_request.go:161` 附近，multipart 分支补 `n` 的 0..128 校验，与 JSON 分支对齐
  - 参照 FlowAPI 的 `dto.MaxImageN=128`；keli 若无该常量则新增，不要写裸字面量
  - 超界返回 400
- [x] **1.2 multipart 参数投影**
  - `relay/helper/billing_expr_request.go` 的 `ResolveIncomingBillingExprRequestInput`
  - 在 `readIncomingBillingExprBody` 返回空 body 后插入：`info.Request` 断言 `*dto.ImageRequest` 成功时，拷贝 DTO、置空 `Prompt`/`Image`/`Images`/`Mask`/`Extra`，交 `BuildBillingExprRequestInputFromRequest` 序列化
  - 走 `common.Marshal`；JSON 路径不得受影响（仅 body 为空时介入）
  - 约 12 行
- [x] **1.3 测试**
  - `billing_expr_request_test.go`：投影字段正确 / 图片字段被剥离 / `n` 缺省与 `n=0` 归一 / JSON 路径不变 / 音频类 multipart 仍为 nil body / 冻结值复用 / `x-www-form-urlencoded`
  - `price_test.go`：multipart `size=2048x1152` + `n=4` → `QuotaToPreConsume = 4×80000`、`EstimatedTier=2k`；同请求 JSON 与 multipart 结果一致
  - `n` 上界的边界用例：`n=128` 通过、`n=129` 400
  - 表驱动 + `testify/require`
- [x] **1.4 验证**
  - `go build ./...`
  - `go test ./relay/helper/...`
  - 剥字段生效核对：投影 body 应为百字节量级而非百 KB

**验收**：构建通过、测试全绿、JSON 路径行为零变化。

---

## 阶段 2 — keli api 部署与配置

> 部署 SOP 见 `keli-api-ops` skill 的 `references/deploy-sop.md`；DB 操作见 `references/db-ops.md`。**动手前先读**。

- [x] **2.1 记录 baseline**
  - 服务器 `docker ps` 取当前 image tag，`grep image: /opt/new-api/docker-compose.yml` 验证
  - `docker tag <当前> new-api:rollback-<ts>`
  - baseline 写进 task notes
- [x] **2.2 本地 build 后部署**（不在服务器 build）
  - 前端未改动，但 `dist` 是 `go:embed`，仍需重 build 整个 Go binary
- [x] **2.3 健康检查**
  - `curl -fsS http://localhost:3000/api/status` 60s 内必须 200，失败立即回滚
- [x] **2.4 核实 `QuotaPerUnit`**（配表达式前必做）
  - 基线调查只核实了代码，**未连生产库**。`QuotaPerUnit` 若不是默认 500000，6 条表达式的金额会整体偏移，需按比例调整常数
  - 一并确认这 6 个模型在 keli 上是否已配置、当前计费方式是什么
  - 确认方式：`docker exec` 查 options 表，或后台 UI 查看

  > 分组倍率由用户自行管理，本任务只负责基准价格，验证时不计入倍率。
- [x] **2.5 配置表达式**
  - 后台 定价 → 模型定价 → 表达式编辑器，粘贴 `research/final-expressions-verification.md` 第 6 版的 6 条
  - **配置前先导出各模型现有配置存档**作为回滚基线
  - 估算器会报 `Unexpected keyword 'let'` —— 预期现象，不阻断保存
  - 直改 options 后需 `docker restart new-api`（cache TTL 1 分钟）
  - keli 缺 `gemini-3.1-flash-image` / `gemini-3-pro-image`（无 `-preview` 后缀的两个名字），如需验证另行在 DB 配置

**验收**：服务健康、表达式保存成功、模型定价页可见档位名。

---

## 阶段 3 — 用户验证（由用户执行）

用户借 keli api 实例自行验证表达式是否按预期工作。

建议核对项：

- [x] 文生图：`size=1024x1024` → 1k 档；`2048x1152` → 2k；`2048x2048` → 4k
- [x] `size=auto` / 不传 → 1k 档
- [x] **图生图（multipart）**：传 `size=2048x1152` → **2k 档**（改动前是 1k，这是本次核心验证点）
- [x] 图生图传 `n=4` → 按 4 张计费（改动前恒按 1 张）
- [x] gemini 原生 `generateContent` + `imageConfig.imageSize=4K` → 4k 档
- [x] 每档扣费额与价格表一致，**且均不为 0**（为 0 说明单位写错，立即回滚）

**验收红线**：任一档位扣费为 0，或 multipart 仍落 1k 档 → 回滚重查。

---

## 阶段 4 — 同步回 FlowAPI

用户确认阶段 3 通过后执行。

- [x] **4.1 移植代码改动**
  - `billing_expr_request.go` 两仓 91 行对 91 行相同，仅 import 路径不同（keli `dto` → FlowAPI `relaykit/dto`）
  - FlowAPI 已有 `dto.MaxImageN=128`，阶段 1.1 的校验改动可能无需移植 —— 移植前先核对 FlowAPI 的 multipart 分支是否已有该校验
- [x] **4.2 测试**
  - 同阶段 1.3，按 FlowAPI 的包路径调整
  - `go build ./...`、`go test ./relay/helper/...`
  - **`relaykit` 独立性**：若改动触及 `relaykit/`，必须 `cd relaykit && GOWORK=off go build ./...`
- [x] **4.3 配置表达式**
  - 同阶段 2.4，6 条表达式逐字相同
  - **先导出 `gemini-3-pro-image-preview` 现有表达式存档** —— 它已在生产 `tiered_expr` 模式运行
- [x] **4.4 灰度验证**
  - FlowAPI 有活跃用户，建议先配一个低流量模型观察，再铺开
  - 用测试令牌打完阶段 3 的核对项
- [x] **4.5 更新桌面配置指南**
  - `/Users/Zhuyu/Desktop/图像模型分辨率阶梯计费-配置指南.md`
  - 第一节口径需同步为第 6 版（gpt 只看面积；gemini `imageSize` → `size` 字面量 → 面积；无 `quality` 判档；用 `let`）
  - 第三节填入 6 条表达式
  - 第六节限制 3「multipart 是利润来源」的结论**已被推翻**，需改写为「已通过代码修复」

---

## 回滚点

| 阶段 | 回滚方式 |
|---|---|
| 1 | `git checkout` 撤销改动（注意：该仓库可能有其他 session 的改动，**commit 时显式列文件名，禁用 `git add -A/.`**）|
| 2 | `docker tag new-api:rollback-<ts>` 换回，重启；表达式改回导出的存档配置 |
| 3 | 同阶段 2 |
| 4 | FlowAPI 侧 revert commit；表达式改回存档 |

---

## 不在本次范围

- `gpt-image-2.5-sunburst` 的历史计费事故（配上表达式后自然消除）
- `gpt-image-1`、`gpt-image-2.5`（均无活跃流量）
- 合入上游 09-09 的 `ResolveImageBillingRequestInput`（键集是本方案子集，无需）
