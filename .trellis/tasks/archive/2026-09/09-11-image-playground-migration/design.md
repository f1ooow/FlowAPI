# 技术设计 — 生图工作台迁移至 gpt_image_playground

## 1. 参照源与目标

- **参照源**：`/Users/Zhuyu/Documents/Code/gpt-image/deploy/repo`（v0.7.5，本地定制副本）。源码在 `deploy/repo/src/`，顶层 `gpt-image/` 只有 `.claude/.cursor/.trellis/deploy`。
- **备选**：`/Users/Zhuyu/Documents/Codex/2026-06-25/bh/work/gpt_image_playground-v0.6.10`，缺 Agent 模式 20+ 文件，**不用**。
- **目标**：整体替换 `web/src/features/image-playground/`（现有 4079 行），不保留旧实现、不做兼容层。
- **策略**：文件级搬运 + 定点改造。能整文件搬的整文件搬（`lib/` 下 API 层、遮罩、尺寸、SSE 等），不能搬的（`Header`/`Settings`/`Agent`）直接删，剩下的按 FlowAPI 技术栈适配。

## 2. 技术栈差异与适配面

| 维度 | 参照项目 | FlowAPI `web/` | 处置 |
|---|---|---|---|
| 构建 | Vite 6 | Rsbuild 2.1 | 改 import 路径与 env（`import.meta.env.DEV` 兼容），`vite.config.ts` 的 `base`/`server.proxy`/构建期 JSON 内嵌全部丢弃 |
| 样式 | Tailwind 3.4 + `tailwind.config.js` | Tailwind 4.3（CSS-first） | **唯一实质风险点**，见 §3 |
| 状态 | zustand 5 | zustand 5 | 原样搬 |
| UI 原语 | 自研 `Select.tsx`(502) 等 | Base UI 1.6 | 保留参照项目自研组件，不强行换 Base UI（换的成本高于收益） |
| i18n | 无，中文硬编码 | i18next + 单 zh.json | 文案抽 key，见 §5 |
| 包管理 | npm/wrangler | Bun | `bun` |

### 依赖增删

- **新增**：`fflate`（`exportZip.ts` 用，FlowAPI 没有）。
- **不引入**：`streamdown` / `react-markdown` / `remark-gfm` / `katex` / `@streamdown/math`（全部只服务 Agent 模式的 markdown 渲染）、`@fal-ai/client`（fal.ai 渠道，站内不走）、`wrangler` / `core-js`。
- `react` / `react-dom` / `zustand` 版本已兼容，不动。

## 3. Tailwind v3 → v4 适配（主要风险）

参照项目用了三处 v4 里不成立的东西：

1. `tailwind.config.js` 的 `theme.extend.colors`（`background`/`border`/`gray: zinc`/`muted`/`primary`/`sidebar` 等 8 组，值走 `hsl(var(--x) / <alpha-value>)`）→ 迁成 CSS-first 的 `@theme` 块。
2. `darkMode: 'media'` → v4 用 `@custom-variant dark (@media (prefers-color-scheme: dark))`。
3. `gray → zinc` remap → 在 `@theme` 里覆写 `--color-gray-*` 指向 zinc 值。若这条成本失控，**降级方案**是批量把 `gray-*` class 改写成 `zinc-*`（纯文本替换，机械但确定），不改语义。
4. `@tailwind base/components/utilities` 三行 → `@import "tailwindcss"`。
5. 参照项目 `index.css` 488 行的全局样式（滚动条、`user-select`、`.saveable-image` 等）**不全量并入 FlowAPI 全局**，只把工作台实际用到的部分**作用域化**（包在 `.image-playground-root` 下），避免污染站内其他页面。
6. 外部字体 `@import url(fontsapi.zeoseven.com)` / `jsdelivr` → **删除**，用 FlowAPI 现有字体链。

**预检验证**：先搬 1 个最复杂的组件（`InputBar.tsx`）跑通样式，再批量搬其余。这是唯一的"先探路"动作，避免 20+ 文件搬完才发现样式层塌方。

## 4. 删除清单

### 4.1 Settings（R2）

- `src/components/SettingsModal.tsx` (1995)
- `src/components/settings/` 全部 5 文件 (735)：`GeneralSettingsTab` / `AgentSettingsTab` / `ZipDownloadRouteModal` 等
- `src/lib/settingsCustomProvider.ts`、`presetConfig.ts`、`urlSettings.ts` 及其 `.test.ts`
- `src/hooks/useDockerApiUrlMigrationNotice.ts`、`useVersionCheck.ts`
- store 中 `showSettings` / `SettingsTab` 状态及其 6 处调用点

### 4.2 顶栏（R6）

- `src/components/Header.tsx` (348) **整文件删除**：品牌名 + `NEW` 角标、`画廊/Agent` 切换、下载图标、帮助图标、设置齿轮全在里面。
- `src/components/HelpModal.tsx` 一并删除。
- 页面顶部不再渲染任何自带 bar；FlowAPI 站内导航保留（R7）。

### 4.3 Agent 模式（R6）

- `src/components/AgentWorkspace.tsx` (1053)
- `src/lib/agentApi.ts`、`agentAssistantBlocks.ts`、`agentConversationState.ts`、`agentImageReferences.ts`、`agentInputBuilder.ts`、`agentResponseState.ts`、`agentWebSearch.ts` 及各自 `.test.ts`
- `src/lib/responsesOutputState.ts`、`browserNotification.ts`（仅 Agent 用，搬运时确认）
- `src/types.ts` 里 Agent 相关类型、store 里 agent 相关 slice、`paramCompatibility` 里 agent 分支

只剩画廊模式后，"模式"这个概念本身消失，不做 `mode === 'gallery'` 之类的判断——直接把画廊作为唯一路径。

### 4.4 频道/供应商收敛

- `src/lib/falAiImageApi.ts` + `.test.ts` 删除（站内无 fal 渠道）。
- 自定义供应商配置导入/导出相关（`profileImportUrl.ts`、`customProviderConfigUrl.ts` 及其测试）删除。

## 5. key / baseUrl / model 的来源改造（R2+R3，核心）

参照项目里这三者都来自 `getActiveApiProfile(settings)`（`src/lib/apiProfiles.ts:785`）返回的 `ApiProfile`，被 `openaiCompatibleImageApi.ts` 直接读取 11 个字段。

**方案：保留 `ApiProfile` 类型结构，只换填充来源。**

| 字段 | 改造后来源 |
|---|---|
| `apiKey` | 用户选中的令牌 key（`fetchTokenKey`，见 §6） |
| `baseUrl` | 站内同源，常量（空/`window.location.origin`），请求打 `/v1/images/...` |
| `model` | 常量 `gpt-image-2`（复用 `constants.ts:31` 的 `IMAGE_PLAYGROUND_MODEL`） |
| `timeout` / `apiMode` / 其余字段 | 常量或站内默认值 |

**为什么不删 `ApiProfile` 类型**：删掉后 `openaiCompatibleImageApi.ts`(1055) / `imageApiShared.ts` / `serverSentEvents.ts` / `size.ts` 的函数签名要全改，工作量远大于保留。

**因此**：把 `apiProfiles.ts` 从"多 profile 管理 + 持久化 + 导入导出"重写为单函数 `resolveImageProfile(tokenKey): ApiProfile`，删除 profile 增删改查、`localStorage` 持久化、导入导出逻辑。

## 6. 令牌选择（R3）

**已查实**：`/v1/images/*` 的 relay **不读请求里的 group 字段**，生效分组只来自令牌行自身的 `Token.Group`（`middleware/auth.go:459-477`）。唯一能覆盖分组的 `middleware/distributor.go:88-104` 挂在 `/pg/chat/completions` 上，而该路由在 `router/` 未注册，是死分支。

→ **"选分组 → 后端按分组解析令牌"不可行，无需新增后端接口。** 形态定为：**前端列出用户自己的令牌（各自带 group）→ 用户选中 → 用该令牌的 key 发请求**。选中令牌即确定分组，不单独做分组下拉。

**实现要点**：
- 令牌列表接口已返回 `group` 字段（`model.Token.Group`，`json:"group"`），但现有工作台在 `web/src/features/playground/lib/tokens.ts:35-41` 与 `types.ts:34-39` 把它丢掉了 → 新实现保留该字段。
- 选择器**复用 FlowAPI 现有的令牌选择组件**（playground 侧已有先例，`getApiKeys` 客户端分页拉全量），不重造。
- key 本身不出现在 UI 任何位置；用户不可编辑 baseUrl / model / 代理。

## 7. 模型下拉（R8）

- 位置：底部参数行最左，尺寸之前。
- 选项：`[{ value: 'gpt-image-2', label: 'gpt-image-2' }]`，走 `Select` 组件（不硬编码为静态文本），便于后续放开。
- 值落入请求体 `model` 字段。
- 复用现有 `IMAGE_PLAYGROUND_MODEL` 常量，不新造字符串。

## 8. 计费安全边界（必须做，不是可选项）

`AGENTS.md` 的 billing invariants 要求：**任何用户可控的数量/倍率在进入计费前必须有上界**。底部参数行开放后，这些字段直接决定 quota：

- **数量 `n`** → 必须受 `dto.MaxImageN` 约束，前端下拉上限沿用现有 `IMAGE_PLAYGROUND_MAX_OUTPUTS = 4`，并在请求构造处再校验一次。
- **尺寸 / 质量 / 格式 / 透明背景 / 审核** → 参照项目的 `paramCompatibility.ts` 允许自由组合。**移植时必须按站内模型实际支持的取值收敛为枚举**，不接受任意字符串，否则未知参数会绕过倍率表。
- 后端若已有对应校验则复用，不新增 ad hoc limit；若发现参照项目传来的取值站内无对应倍率，**在请求构造处拒绝**而不是静默降级。

这一节的验证写进 implement.md 的验收项。

## 9. 数据流

```
用户选令牌 ──> tokenKey
                 │
参数行(模型/尺寸/质量/格式/透明/审核/数量) ──> 校验上界
                 │
                 v
       resolveImageProfile(tokenKey) ──> ApiProfile{apiKey, baseUrl, model, ...}
                 │
                 v
   openaiCompatibleImageApi ──> POST /v1/images/generations | /v1/images/edits
                 │                            （站内同源，带令牌 key）
                 v
            站内 relay（既有计费 + 日志链路，不改）
```

## 10. 不做的事

- 不改 `/v1/images/*` relay 行为，不加按分组解析令牌的后端接口。
- 不保留旧 `image-playground` 任何组件、类型、i18n key 的向后兼容。
- 不引入参照项目的服务端/部署组件（`wrangler`/Cloudflare）。
- 不把参照项目的 488 行全局 CSS 灌进 FlowAPI 全局样式。

## 11. 回滚

改动集中在 `web/src/features/image-playground/` 一个目录 + `zh.json` 增 key + `package.json` 加 `fflate`。回滚 = `git revert` 该次提交；无数据库迁移、无后端变更、无部署副作用。
