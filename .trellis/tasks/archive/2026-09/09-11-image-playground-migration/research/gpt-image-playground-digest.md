# gpt_image_playground 调研 — 决策摘要

> 由 `gpt-image-playground.md`（42KB，超出注入上限）压缩而来，保留路径/行数/可删判定与障碍结论；完整版见同目录原文件。

# Research: gpt_image_playground 迁移调研

- **Query**: 调研 `https://github.com/CookSleep/gpt_image_playground`（本地副本优先），产出技术栈、页面/组件结构、settings 功能构成、生图接口调用方式、依赖差集、迁移到 FlowAPI 的障碍
- **Scope**: mixed（外部仓库 + FlowAPI 内部技术栈对照）
- **Date**: 2026-09-11

---

## 0. 结论速览

- 本机存在**两份**本地副本：
  - **`/Users/Zhuyu/Documents/Code/gpt-image/deploy/repo`（v0.7.5，推荐作为参照源）** —— 带 `.git`（但 git status 快照被剥离，只有一条历史）、`gpt-image-config.example.json`、`sponsor-presets.json`、`deploy/nginx.conf` 等部署产物，是一份**已定制部署过**的工作副本。
  - `/Users/Zhuyu/Documents/Codex/2026-06-25/bh/work/gpt_image_playground-v0.6.10` —— 上游 v0.6.10 的干净 clone（commit `774b722 chore: release v0.6.10`），比 v0.7.5 少 Agent 模式的文件。
  - 两处都不是「`Code/gpt_image_playground`」这个名字，所以按目录名找不到；`/Users/Zhuyu/Documents/Code/gpt-image/` 顶层只有 `.claude/.cursor/.trellis/AGENTS.md/deploy`，真正的源码在 `deploy/repo/`。
- 该项目的 settings 是**单一大弹窗**（`SettingsModal.tsx`，1995 行，5 个 tab），状态是 **Zustand persist → localStorage（key=`gpt-image-playground`）**，任务/图片/Agent 对话在 **IndexedDB（`gpt-image-playground` v3）**。
- 它是**纯前端静态站**，没有后端；生图请求由浏览器直接打上游（`/v1/images/generations`、`/v1/images/edits`、`/v1/responses`），`Authorization: Bearer <profile.apiKey>`。API key / baseUrl / model 全部来自 settings 里的 `ApiProfile`，是**唯一的参数来源**。
- FlowAPI 侧 `web/src/features/image-playground/lib/api.ts` 已经在用同样的 `/v1/images/generations` + `/v1/images/edits`（`gpt-image-2`），且 key 已经来自 `fetchTokenKey(tokenId)`（`/api/token/{id}/key`）；后端 `router/relay-router.go:110/113` 已注册这两个路由。

---

## 1. 本地副本定位

| 路径 | 版本 | 说明 |
|---|---|---|
| `/Users/Zhuyu/Documents/Code/gpt-image/deploy/repo` | `package.json` v0.7.5 | **推荐**。含 `.git`（`git log` 只有 `774b722 chore: release v0.6.10`，工作树相对该 commit 有改动，见 `.git/index`）、`node_modules/`、`dist/`（已构建）、`gpt-image-config.example.json`、`sponsor-presets.json`、`deploy/nginx.conf`。源码目录 `src/` 完整。 |
| `/Users/Zhuyu/Documents/Codex/2026-06-25/bh/work/gpt_image_playground-v0.6.10` | `package.json` v0.6.10 | 上游 v0.6.10 干净 clone，`git status` 有 2 个改动文件（`src/lib/apiProfiles.ts`、`src/vite-env.d.ts`）。**没有 Agent 模式的相关文件**（`agentApi.ts`、`agentConversationState.ts`、`inputDraftState.ts`、`presetConfig.ts`、`profileImportUrl.ts`、`favoriteState.ts` 等至少 20 个文件缺失）。 |
| `/Users/Zhuyu/Documents/Code/gpt-image/deploy/notes/plan.md` | — | 用户自己的部署方案笔记，记录了「真 key 由 nginx 注入、前端填任意占位 key 放行」的部署形态，以及 `hasSubmitApiConfig = Boolean(activeProfile.apiKey)`（`src/components/InputBar.tsx:432`）这个关键判断。 |
| `/Users/Zhuyu/Documents/Code/gpt-image/deploy/nginx/` | — | `image.robusta.top.conf` + `htpasswd`（实际部署的 nginx 站点配置）。 |

无需 clone 上游仓库。两份副本的 `src/` 差异集中在 v0.7.5 新增的 Agent / 收藏夹 / 预置配置模块。

规模（v0.7.5 非测试源码）：约 **31,463 行**，50 个 `lib/*.ts` + 1 个 `lib/*.tsx` + 39 个 `components/*` + 7 个 `hooks/*`，另有 33 个 `*.test.ts`（约 13,000 行，不迁移）。

---

### 构建命令

```
npm install / npm run dev / npm run build / npm test / npm run test:watch
npm run mock:api   # 本地假生图接口，scripts/mock-image-api.mjs
npm run deploy:cf  # wrangler
```

### 构建期注入（`vite.config.ts:37-90` + `src/vite-env.d.ts`）

---

## 3. 页面 / 组件结构

### 3.2 Gallery 模式的组件清单（文件:行数:职责）

| 文件 | 行数 | 职责 |
|---|---|---|
| `src/App.tsx` | 163 | 根组件；模式切换、浮层挂载、启动引导 |
| `src/components/Header.tsx` | 348 | 顶部栏：品牌名 + `NEW` 角标、画廊/Agent 切换、下载图标、帮助（`HelpModal`）、历史（`HistoryModal`）、PWA 安装、**设置齿轮（`setShowSettings`，:298）**、滚动方向感知 |
| `src/components/InputBar.tsx` | **1947** | 输入工作台核心：contenteditable 提示词框、`@` 引用图片、拖拽上传、参考图缩略条、遮罩入口、参数行（桌面/移动两套布局）、提交/停止按钮、批量选择条、ZIP 批量下载；`hasSubmitApiConfig = Boolean(activeProfile.apiKey)`（:432）决定提交是否放行 |
| `src/components/input/inputParamsPanel.tsx` | 306 | 底部参数行本体：尺寸 / 质量 / 格式 / 压缩 / 审核 / 数量(n) / 透明背景，全部由 props 传入（纯展示组件） |
| `src/components/input/inputBatchBars.tsx` | 206 | 批量选择时的浮动操作条 |
| `src/components/input/dragUploadOverlay.tsx` | 44 | 拖拽上传遮罩 |
| `src/components/input/buttonTooltip.tsx` | 14 | 按钮 tooltip 包装 |
| `src/components/TaskGrid.tsx` | 330 | 结果画廊网格；框选（drag select）、搜索/状态/收藏过滤、排序 |
| `src/components/TaskCard.tsx` | 740 | 单任务卡片；侧滑、封面比例标签、参数横向滚动条（API Name / Model / Mask / 透明背景 / 参数 diff）、重试按钮、**读 `settings.alwaysShowRetryButton`（:641）** |
| `src/components/DetailModal.tsx` | 1213 | 任务详情：左图右信息（参考图、参数 diff、耗时）、下载、**读 `settings.codexCli` / `settings.zipDownloadRoutes`（:256-419）** |
| `src/components/Lightbox.tsx` | 800 | 全屏图片查看/缩放/切换 |
| `src/components/SearchBar.tsx` | 210 | 搜索框、状态过滤、收藏过滤、收藏夹管理入口 |
| `src/components/SizePickerModal.tsx` | 442 | 尺寸预设选择器（比例/档位网格，支持自定义尺寸，含 Codex CLI 尺寸规范化） |
| `src/components/MaskEditorModal.tsx` | 1048 | 遮罩涂抹编辑器（画笔/橡皮、缩放平移、与主图尺寸对齐校验） |
| `src/components/ImageContextMenu.tsx` | 257 | 图片右键菜单（复制/下载/作参考图/加收藏…） |
| `src/components/ConfirmDialog.tsx` | 184 | 通用确认弹窗（store 里的 `confirmDialog` 驱动） |
| `src/components/Toast.tsx` | 45 | 轻提示 |
| `src/components/Select.tsx` | 502 | 自研下拉（可拖拽排序、带 actions、视口自适应高度） |
| `src/components/ViewportTooltip.tsx` / `TooltipButton.tsx` | 139 / 56 | portal tooltip |
| `src/components/Checkbox.tsx` | 37 | 勾选框 |
| `src/components/icons.tsx` | 235 | 全部 SVG 图标 |
| `src/components/HistoryModal.tsx` | 311 | 历史（从 Header 打开） |
| `src/components/HelpModal.tsx` | 188 | 帮助（从 Header 打开） |
| `src/components/SupportPromptModal.tsx` | 96 | 赞助/打赏提示（可在 settings 里关闭） |
| `src/components/MarkdownRenderer.tsx` | 224 | Markdown + KaTeX 渲染（Agent 回复、配置说明用） |
| `src/components/FavoriteCollections.tsx` | 4 | 再导出 barrel |
| `src/components/favorites/*` | 36–458 ×5 | 收藏夹视图 / 概览卡 / 选择器弹窗 / 管理弹窗 / 标题 hook |
| `src/components/SettingsModal.tsx` | **1995** | 设置大弹窗（5 tab）——**本次要删除** |
| `src/components/settings/GeneralSettingsTab.tsx` | 220 | 通用设置 tab ——**删** |
| `src/components/settings/AgentSettingsTab.tsx` | 158 | Agent 设置 tab ——**删** |
| `src/components/settings/CustomProviderModal.tsx` | 160 | 自定义服务商编辑弹窗 ——**删** |
| `src/components/settings/ProfileImportUrlModal.tsx` | 94 | 复制「导入配置 URL」弹窗 ——**删** |
| `src/components/settings/ZipDownloadRouteModal.tsx` | 103 | ZIP 下载途径管理弹窗 ——**删** |
| `src/components/AgentWorkspace.tsx` | 1053 | Agent 模式工作区（PRD R6 要求删） |

### 3.4 lib 模块（请求/数据链路的骨干）

| 文件 | 行数 | 职责 |
|---|---|---|
| `lib/openaiCompatibleImageApi.ts` | **1055** | OpenAI 兼容请求实现：Images API（生成/编辑）、Responses API、SSE 流式解析、自定义异步服务商轮询 |
| `lib/api.ts` | 14 | 薄分发：`provider === 'fal'` → fal，否则 → OpenAI 兼容 |
| `lib/apiProfiles.ts` | **1205** | `ApiProfile`/`AppSettings` 规范化、默认值、预置配置合并、导入导出 |
| `lib/devProxy.ts` | 106 | `normalizeBaseUrl`（自动补 `/v1`）、`buildApiUrl`、代理开关判定 |
| `lib/imageApiShared.ts` | 219 | `CallApiOptions`/`CallApiResult`、体积上限（遮罩 50MiB / 载荷 512MiB）、防改写提示词前缀 |
| `lib/presetConfig.ts` | 143 | 部署期预置配置（锁定/只读/禁删） |
| `lib/urlSettings.ts` | 337 | URL 查询参数注入配置 |
| `lib/customProviderConfigUrl.ts` / `lib/defaultApiUrl.ts` / `lib/profileImportUrl.ts` | 70 / 67 / 42 | 预置配置来源解析 |
| `lib/db.ts` | 348 | IndexedDB 封装（tasks/images/thumbnails/agentConversations） |
| `lib/persistedState.ts` | 235 | zustand persist 的 `partialize`/`merge`/迁移 |
| `lib/imageCache.ts` | 225 | 内存 dataURL 缓存 + 缩略图回填 |
| `lib/canvasImage.ts` / `lib/maskPreprocess.ts` / `lib/mask.ts` / `lib/transparentImage.ts` | 102 / 112 / 35 / 401 | 图片编解码、遮罩预处理、透明背景抠图 |
| `lib/size.ts` | 290 | 尺寸规范化（16 的倍数、1K/2K/4K 档位、比例预设、Codex CLI 尺寸提示词注入） |
| `lib/paramCompatibility.ts` | 53 | 按 provider 归一化参数；输出图上限（OpenAI 10 / fal 4） |
| `lib/serverSentEvents.ts` | 97 | SSE 解析 |
| `lib/agentApi.ts` | 920 | Agent 模式的 Responses API 调用 ——**删** |
| `lib/agentConversationState.ts` / `agentResponseState.ts` / `agentInputBuilder.ts` / `agentAssistantBlocks.ts` / `agentImageReferences.ts` / `agentWebSearch.ts` | 276 / 444 / 196 / 251 / 83 / 74 | Agent 模式 ——**删** |
| `lib/exportZip.ts` / `dataOperations.ts` / `downloadImages.ts` / `exportFileName.ts` | 325 / 6 / 134 / 12 | ZIP 导出与下载 |
| `lib/favoriteState.ts` / `inputDraftState.ts` / `taskState.ts` / `taskPromptDisplay.ts` | 175 / 227 / 100 / 6 | 收藏、草稿、任务状态 |
| `lib/clipboard.ts` / `browserNotification.ts` / `dropdown.ts` / `domRect.ts` / `viewport.ts` / `viewportTransform.ts` / `clickSuppression.ts` / `tooltipDismiss.ts` / `dataUrl.ts` / `runtimeEnv.ts` / `paramDisplay.tsx` | 小 | 杂项 |
| `lib/falAiImageApi.ts` | 227 | fal.ai 队列 API —— provider 只有 openai 时**删** |
| `lib/settingsCustomProvider.ts` | 137 | 自定义服务商 LLM 提示词模板 ——**删** |

---

## 4. 设置（settings）功能详解

### 4.1 形态

- **单一大弹窗**（`src/components/SettingsModal.tsx`，1995 行，`createPortal` 到 body），由 store 的 `showSettings: boolean` + `settingsTabRequest: SettingsTab | null` 控制（`src/store.ts:387-389, 980-988`）。
- `SettingsTab = 'general' | 'agent' | 'api' | 'data' | 'about'`（`src/store.ts:113`）。tab 导航在 `SettingsModal.tsx:1143/1152/1161/1172/1181`，内容区在 `:1195/1205/1220/1729/1858`。
- 入口只有两处：`Header.tsx:298` 的齿轮按钮；`InputBar.tsx:1793/1900` 当 `!hasSubmitApiConfig` 时点提交会 `setShowSettings(true)`；另有 store 内部 4 处（`store.ts:540/553/565/1670/2296/2446`）在 Agent 配置不满足时弹设置。

### 4.5 删掉 settings 后，哪些能整块移除、哪些被别处依赖

**能整块删除（无其他引用）**

1. `src/components/SettingsModal.tsx`（1995 行）
2. `src/components/settings/` 整个目录（5 文件，735 行）
3. `src/lib/settingsCustomProvider.ts`（137 行，只被 SettingsModal 引用）
4. `src/lib/presetConfig.ts`（143 行，只被 App.tsx / SettingsModal / apiProfiles 引用；若一并放弃「部署期预置配置」）
5. `src/lib/urlSettings.ts`（337 行）、`lib/profileImportUrl.ts`、`lib/customProviderConfigUrl.ts`、`lib/defaultApiUrl.ts`（这套是「URL 参数/JSON 预置配置」链路，与设置弹窗同属一个 feature）
6. `src/lib/exportZip.ts` + `lib/dataOperations.ts`（仅数据 tab 的导入导出用）
7. store 中的 `showSettings`、`settingsTabRequest`、`setShowSettings`、`SettingsTab` 类型（`store.ts:113, 387-389, 980-988`）以及 6 处 `setShowSettings` 调用点
8. `AppSettings` 中纯 UI 开关：`enterSubmit`、`clearInputAfterSubmit`、`persistInputOnRestart`、`reuseTaskApiProfileTemporarily`、`alwaysShowRetryButton`、`allowPromptRewrite`、`taskCompletionNotification`、`zipDownloadRoutes`、Agent 全部字段、`customProviders`、`providerOrder`

**被别处依赖、不能直接删的（关键判断）**

| 被依赖项 | 消费点 | 删掉 settings 后的处置 |
|---|---|---|
| `ApiProfile.apiKey` | `openaiCompatibleImageApi.ts:87-91` `createRequestHeaders()`；`InputBar.tsx:432` `hasSubmitApiConfig`；`agentApi.ts:80` | 改为 FlowAPI 令牌：`fetchTokenKey(tokenId)`（`/api/token/{id}/key`，见 `web/src/features/keys/api.ts:112-117`） |
| `ApiProfile.baseUrl` | `buildApiUrl(profile.baseUrl, path, …)`（`openaiCompatibleImageApi.ts:550/587/808/843/1019`） | 站内同源，等价于 FlowAPI 现状的 `fetch('/v1/images/generations')`（`web/src/features/image-playground/lib/api.ts:178/221`），**baseUrl 变空串、路径直接用 `/v1/...`** |
| `ApiProfile.model` | 请求体 `model` 字段（`openaiCompatibleImageApi.ts:496/558/1009`） | PRD R8：底部参数行加「模型」下拉，当前只有 `gpt-image-2`；值注入 profile 对象即可，请求代码不用动 |
| `ApiProfile.timeout` | `openaiCompatibleImageApi.ts:489/893/996` `setTimeout(… profile.timeout * 1000)` | 常量（默认 600） |
| `ApiProfile.apiMode` | `openaiCompatibleImageApi.ts:438` 分支（`responses` vs `images`）；`paramCompatibility.ts` | 固定 `'images'`（PRD 只保留画廊 + Images API） |
| `ApiProfile.apiProxy` | `shouldUseApiProxy(profile.apiProxy, …)` + `readClientDevProxyConfig()`（`devProxy.ts:96-106`） | 直接用 Vite/`dev-proxy.config.json` 那套，或整段删掉，改为同源相对路径 |
| `ApiProfile.streamImages` / `streamPartialImages` | `openaiCompatibleImageApi.ts:143/445/517/582/604/1015/1035`；`InputBar`、`paramCompatibility` 提示文案 | 常量 `false`（FlowAPI relay 是否支持 `partial_images` 见 §5 注） |
| `ApiProfile.responseFormatB64Json` | `openaiCompatibleImageApi.ts:514/579/790/801` | 常量（建议 `true`，与 FlowAPI 现有 `normalizeImageResponse` 处理 `b64_json`+`url` 一致） |
| `ApiProfile.codexCli` | `openaiCompatibleImageApi.ts` 十余处；`DetailModal.tsx:256-257`；`paramCompatibility.ts:31`；`size.ts` | 常量 `false`，但**相关分支代码仍在** |
| `settings.alwaysShowRetryButton` | `TaskCard.tsx:641` | 常量 `false` 或删条件 |
| `settings.zipDownloadRoutes` | `InputBar.tsx:213/239`、`DetailModal.tsx:398/419` | 二选一：删 ZIP 批量下载，或改常量 |
| `settings.enterSubmit` | `InputBar.tsx:870` | 常量 |
| `settings.allowPromptRewrite` | `openaiCompatibleImageApi.ts:478/672/1010` | 常量 |
| `settings.persistInputOnRestart` / `clearInputAfterSubmit` | `store.ts`、`persistedState.ts:96-116`、`InputBar` | 常量 |
| `settings.reuseTaskApiProfileTemporarily` | `InputBar.tsx:419`、`store.ts submitTask` | 删该功能分支 |
| `settings.profiles` / `activeProfileId` | `getActiveApiProfile(settings)` 被 `TaskCard.tsx:80`（间接）、`DetailModal.tsx:28`、`InputBar.tsx`、`api.ts`、`paramCompatibility.ts` 使用 | **建议保留 `ApiProfile` 类型结构**，由 FlowAPI 侧合成一个 profile 对象注入 `settings.profiles/activeProfileId`，请求代码零改动地跑通 |

**结论 —— 删掉设置后，API key / base URL / 模型从哪里来：**

- 原项目里这三者是**同一个来源**：`settings.profiles[activeProfileId]`（`getActiveApiProfile`，`apiProfiles.ts:785-806`），而 `settings` 又从**部署期预置配置**（`VITE_DEFAULT_API_URL` / EmbeddedDefaultConfig / URL 查询参数）与**用户在设置弹窗里的编辑**合并而来。
- FlowAPI 里的替代来源：
  - `apiKey` → 用户自己的令牌（`/api/token/{id}/key`，`web/src/features/keys/api.ts:112`）；`model.Token` 有 `group`、`auto_groups`、`cross_group_retry` 字段（`model/token.go:29-31`），`ApiKeyFormData` 也有 `group`/`auto_groups`（`web/src/features/keys/types.ts:85-101`）——**「用户选令牌 → 分组随之确定」在数据上成立**。
  - `baseUrl` → 站内同源，`'/v1/images/generations'`、`'/v1/images/edits'`（后端 `router/relay-router.go:110/113` 已注册，`types.RelayFormatOpenAIImage`）。
  - `model` → PRD R8 的新下拉，初值 `gpt-image-2`（与 `web/src/features/image-playground/constants.ts:24` 一致）。
- 保留 `ApiProfile` 结构、只把它的**填充来源**从「设置弹窗 + 预置配置」换成「FlowAPI 令牌 + 常量」，是改动面最小的路径：`openaiCompatibleImageApi.ts` / `imageApiShared.ts` / `serverSentEvents.ts` / `size.ts` / `paramCompatibility.ts` 这些请求层可以基本原样移植。

---

## 5. 生图接口调用方式

### 5.1 分发与路径

`lib/api.ts:11-13`：

```
callImageApi(opts)
  └─ profile.provider === 'fal' ? callFalAiImageApi
     : callOpenAICompatibleImageApi(opts, profile, getCustomProviderDefinition(...))
        ├─ customProvider 存在 → callCustomHttpImageApi（自定义/异步轮询）
        └─ profile.apiMode === 'responses' ? callResponsesImageApi : callImagesApi
```

路径常量在 `openaiCompatibleImageApi.ts:38-43`：

```ts
{ generationPath: 'images/generations', editPath: 'images/edits' }
```

URL 由 `buildApiUrl(profile.baseUrl, path, proxyConfig, useApiProxy)`（`devProxy.ts:59-82`）拼接：

- `useApiProxy === true` → `/api-proxy/{path}`（同源，由 nginx 注入真 key）
- `baseUrl` 以 `/` 结尾 → 直接拼接，**不补 `/v1`**
- 否则 → `normalizeBaseUrl` 自动补 `/v1`（`devProxy.ts:13-37`），最终 `{baseUrl}/v1/images/generations`

### 5.2 文生图请求（`callImagesApiSingle`，`openaiCompatibleImageApi.ts:557-597`）

```
POST {baseUrl}/v1/images/generations
Authorization: Bearer {profile.apiKey}
Content-Type: application/json

{
  model: profile.model,
  prompt,
  output_format: 'png' | 'jpeg' | 'webp',
  moderation: 'auto' | 'low',
  size,                       // codexCli 时不发
  quality,                    // codexCli 时不发
  output_compression,         // 仅 output_format !== 'png' 且非 null
  n,                          // 仅 n > 1
  response_format: 'b64_json',// 仅 profile.responseFormatB64Json
  stream: true,               // 仅 profile.streamImages
  partial_images: 0..3        // 仅 profile.streamImages
}
```

### 5.3 图生图 / 遮罩编辑（`openaiCompatibleImageApi.ts:494-556`）

```
POST {baseUrl}/v1/images/edits
Authorization: Bearer {profile.apiKey}
Content-Type: multipart/form-data

model / prompt / size / output_format / moderation / quality
output_compression（条件） / n（>1）/ response_format（条件）
stream + partial_images（条件）
image[]            // 每个输入图一个字段，文件名 input-{i}.{ext}；单图时也发 image[]（FlowAPI 现有实现单图发 image，多图发 image[]）
mask               // mask.png，仅在有遮罩时；第一个输入图会先转 PNG 再作为主图
```

体积守卫：`assertMaskEditFileSize`（50 MiB）、`assertImageInputPayloadSize`（512 MiB），见 `imageApiShared.ts:8-9`。

### 5.6 流式

- `image_generation.partial_image` / `image_edit.partial_image` → `b64_json` + `partial_image_index`，作为中间步骤图
- `object === 'image.generation.result' | 'image.edit.result'` → 最终结果载荷
- `image_generation.completed` / `image_edit.completed` → 最终单图项

Responses 流式事件：`response.image_generation_call.partial_image`（`partial_image_b64`）与 `response.output_item.done`（`:380-400`）。

### 5.8 支持的参数

| 参数 | 取值 | 说明 |
|---|---|---|
| `size` | `auto` / `1024x1024` / `1024x1536` / `1536x1024` / 任意 `WxH` | `lib/size.ts` 规范化（16 的倍数、1K/2K/4K 档、比例预设、最大边约束） |
| `quality` | `auto` / `low` / `medium` / `high` | Codex CLI 模式强制 `auto` |
| `output_format` | `png` / `jpeg` / `webp` | |
| `output_compression` | `number \| null` | 仅非 png |
| `moderation` | `auto` / `low` | |
| `n` | 1–10（OpenAI）/ 1–4（fal） | `MAX_OPENAI_OUTPUT_IMAGES = 10`（`paramCompatibility.ts:5`）；流式且 n>1 或 codexCli 时会拆成并发单图请求（`:445-471`） |
| `transparent_output` | boolean | 前端提示词工程 + Canvas 抠背景后处理（`lib/transparentImage.ts`，绿/洋红键控色） |
| 参考图 | 最多 16 张 | `API_MAX_IMAGES = 16`（`InputBar.tsx:41`） |
| `mask` | PNG | 遮罩编辑，仅作用于第一张输入图；全图遮罩会弹二次确认（`store.ts:1683-1700`） |
| `response_format` | `b64_json` | 可选 |
| `stream` + `partial_images` | 0–3 | 可选 |

---

## 6. 依赖清单（package.json）

原项目 dependencies / devDependencies 全量：

```
dependencies:  @fal-ai/client, @streamdown/math, core-js, fflate, katex,
               react, react-dom, react-markdown, remark-gfm, streamdown, zustand
devDependencies: @types/react, @types/react-dom, @vitejs/plugin-react,
                 autoprefixer, jsdom, postcss, tailwindcss, typescript, vite, vitest, wrangler
```

### 反向：FlowAPI 有、原项目没有的（可用于替换）

---

## 7. 移植到 FlowAPI 的主要障碍与需适配点

> 以下只陈述事实性差异与需要决策的落点，不含改进建议。

### 7.1 构建与工具链

| 项 | 原项目 | FlowAPI `web/` | 影响面 |
|---|---|---|---|
| 构建器 | Vite 6 | **Rsbuild 2**（`rsbuild.config.ts`） | `vite.config.ts` 里的 `define`（`__APP_VERSION__`、`__DEV_PROXY_CONFIG__`）、`server.proxy`、`embedDefaultConfig`（构建期读 JSON 内嵌）、`base: './'` 全部没有对应物，需整套删除或改写 |
| 环境变量 | `import.meta.env.VITE_*`（`src/vite-env.d.ts` 声明 8 个） | Rsbuild 用 `import.meta.env.PUBLIC_*` 约定（需核对 `rsbuild.config.ts` 的 `source.define`） | `lib/runtimeEnv.ts`（3 行）、`lib/devProxy.ts:96-106`、`lib/apiProfiles.ts:18-27`、`lib/presetConfig.ts:4-6` 都读这些变量 |
| Tailwind | **v3.4**，`tailwind.config.js` 里 remap `gray → zinc`，扩展 `background/border/foreground/muted/primary/sidebar` CSS 变量色板；`darkMode: 'media'` | **Tailwind v4**（CSS-first 配置） | 39 个组件里的 `dark:` 变体、`gray-*`（zinc 语义）、`rounded-xl` 等类名需要按 v4 与新主题重排；`darkMode: 'media'` 与 FlowAPI 的主题切换机制可能不一致 |
| 全局 CSS | `src/index.css` 488 行：`@tailwind base/components/utilities`、`:root` CSS 变量、`@import` 两个外部字体 CDN（`fontsapi.zeoseven.com`、`jsdelivr` 的 Harmony Sans）、`-webkit-user-select` 全局禁用、自定义滚动条、`safe-area-*` | FlowAPI 有自己的全局样式与主题 | 需挑选而非整体搬运 |
| 类型检查 | `tsc -b` | `tsgo -b`（`@typescript/native-preview`） | 语法兼容性风险（`verbatimModuleSyntax`、`allowImportingTsExtensions` 等 flag 需对齐） |
| 测试 | Vitest + jsdom，33 个 `*.test.ts` | Vitest + jsdom + @testing-library | 原项目测试 import 的是 `./apiProfiles` 等相对路径与内部实现，**迁移后大概率整批丢弃**（PRD 也未要求搬运） |
| 包管理器 | npm | **Bun** | 只是安装/脚本执行差异 |

### 7.2 i18n

- 原项目**全部中文硬编码在 JSX 里**（如 `SettingsModal.tsx`、`inputParamsPanel.tsx` 的 "尺寸"/"质量"/"格式"、`store.ts` 的 toast 文案 "任务已提交"/"请输入提示词"）。没有 i18n 框架。
- FlowAPI 要求 `useTranslation()` + `t('English key')` + `web/src/i18n/locales/{lang}.json` 7 语言文件。
- **不存在可复用的 key**：所有文案都要新抽 key 并翻译。以 `InputBar.tsx`（1947 行）、`DetailModal.tsx`（1213 行）、`SizePickerModal.tsx`（442 行）、`TaskCard.tsx`（740 行）为文案密集区。

### 7.3 UI 原语与基础设施

| 原项目 | 依赖 | FlowAPI 对应物 | 备注 |
|---|---|---|---|
| `components/Select.tsx`（502 行，自研下拉，含拖拽排序、actions、视口自适应） | 无 | `@base-ui/react` / `cmdk` / `components/ui/*` | 自研 Select 的交互（拖拽排序、`getDropdownMaxHeight`）在 Base UI 里没有直接对等物 |
| `components/Toast.tsx` + `showToast`（`store.ts`） | 自研 | `sonner`（`web/package.json`） | |
| `components/ConfirmDialog.tsx` + `confirmDialog` state | 自研 | 待确认 FlowAPI 是否有统一 confirm | 原项目通过 `useStore.setConfirmDialog({title, message, confirmText, action})` 实现「确认后回调」模式，迁移时要有等价能力 |
| `components/ViewportTooltip.tsx` / `TooltipButton.tsx` / `useTooltip` / `lib/tooltipDismiss` | 自研 portal tooltip | 待确认 | 3 个 hook + 2 个组件构成一套 tooltip 体系 |
| `components/icons.tsx`（235 行自绘 SVG） | 无图标库 | `lucide-react` / `@hugeicons/react` | |
| `components/MarkdownRenderer.tsx` | `react-markdown`+`streamdown` | `web/src/components/ui/markdown.tsx`（`marked`+`katex`+`dompurify`） | 仅 Agent 模式与配置说明使用；两者都删则不需要 |
| `lib/viewport.ts` `installMobileViewportGuards()` | 自研移动端视口守卫 | 待确认 | `main.tsx:8-10` 调用 |
| CSS `safe-area-*`、移动端手势（侧滑、`useDragSelect` 框选） | 自研 | 待确认 | 与 FlowAPI 运维面板的移动端行为可能冲突 |

### 7.4 状态与持久化的根本差异

- 原项目整个 store（`store.ts`，4573 行 + 5224 行测试）是**单文件全局 Zustand store**，包含 gallery + agent + settings + favorites + UI 浮层，并配 `persist`（localStorage）+ IndexedDB 双通道。
- 迁移要按 PRD 拆掉：settings 分支、agent 分支（**`store.ts` 里 agent 相关引用 548 处**、`types.ts` 38 处）、`preset`/`urlSettings`/`customProvider` 分支。
- 现存的 FlowAPI 生图工作台是另一套形态：`web/src/features/image-playground/hooks/use-image-playground.ts`（741 行）+ `lib/storage.ts`（IndexedDB `flowapi-image-playground` v1）+ `components/{image-playground,mask-editor,task-card,task-detail}.tsx`。PRD 要求整体替换，两套 store/DB 命名与 schema 不同（原项目 DB `gpt-image-playground` v3、4 个 object store；FlowAPI 的 DB `flowapi-image-playground` v1、`tasks` + `images` 两个 store，见 `web/src/features/image-playground/constants.ts:57-61`）。
- 原项目 `persistedState.ts` 里 `partialize` 会把 `inputImages` 的 `dataUrl` 清空只留 `id`（`:96-116`），图片实体在 IndexedDB，这是「提示词草稿 + 图片引用」的分离设计，迁移时要保持一致性。

### 7.5 请求层与 FlowAPI 后端的对齐

已核对的事实：

- FlowAPI 后端已注册 `POST /v1/images/generations`、`POST /v1/images/edits`（`router/relay-router.go:110/113`，`types.RelayFormatOpenAIImage`），另有 `POST /v1/edits`。
- FlowAPI 前端已有同源请求实现：`web/src/features/image-playground/lib/api.ts:178`（generations）、`:221`（edits），body 字段为 `model/prompt/size/quality/output_format/n/moderation`（**无** `output_compression`、`response_format`、`stream`、`partial_images`），key 来自 `resolvePlaygroundToken(tokenId)` → `fetchTokenKey`（`/api/token/{id}/key`）。
- 因此原项目 `openaiCompatibleImageApi.ts` 里这些字段是否能被 FlowAPI relay 接受/转发，需要单独确认（后端证据：`relay/channel/openai/image_edit_test.go:32/67` 覆盖了 `partial_images` 透传；其余 `moderation` / `transparent_output` / `response_format` 在本仓库 grep 未见显式处理）。
- 认证差异：原项目用 `Authorization: Bearer {用户手填的 key}` 直接打上游；FlowAPI 下同一 header 会是**站内令牌 key**，请求打到同源 `/v1/...`，由 relay 转发并计费。`InputBar.tsx:432` 的 `hasSubmitApiConfig = Boolean(activeProfile.apiKey)` 提交门禁需要重写成「已选令牌」。
- `lib/devProxy.ts` 的 `/api-proxy/` 同源代理与 `deploy/nginx.conf:23-46` 的 nginx 代理是「隐藏真 key」方案；在 FlowAPI 内这套完全不适用（站内已同源、key 是用户自己的令牌）。
- `lib/falAiImageApi.ts`（227 行）+ `@fal-ai/client` 依赖、`lib/settingsCustomProvider.ts` 的自定义服务商模板、`lib/agentApi.ts`（920 行）在 PRD 范围内都是删除对象。

### 7.6 其它需要在实现前敲定的落点

1. **`ApiProfile` 类型是否保留**：保留则请求层（`openaiCompatibleImageApi.ts` 1055 行）几乎可原样复用，只需在 FlowAPI 侧合成 profile；不保留则 `openaiCompatibleImageApi.ts`、`paramCompatibility.ts`、`size.ts`、`TaskCard.tsx`、`DetailModal.tsx` 全部要改签名。
2. **任务记录 schema 冲突**：原项目 `TaskRecord`（`types.ts`）含 `apiProfileId/apiProfileName/apiProvider/apiModel/apiMode/codexCli` 等字段，删掉 settings 后这些字段的来源需重定义；FlowAPI 现有 `PlaygroundTask`（`features/image-playground/types.ts`）用 `tokenId/tokenName/model`。
3. **顶部栏（PRD R6）**：`Header.tsx`（348 行）整块删除后，`HelpModal`、`HistoryModal`、PWA 安装、`useVersionCheck`（打 GitHub Release API）一并失去入口；`App.tsx:139` 的 `<Header />` 也要移除。
4. **`isFalProvider` / `isFalTextToImage` / `MAX_FAL_OUTPUT_IMAGES` 等 fal 分支**散布在 `InputBar.tsx`、`inputParamsPanel.tsx`、`paramCompatibility.ts`、`size.ts` 中，只保留 OpenAI 时需要清理这些判断。
5. **PWA**：`public/manifest.webmanifest`、`public/sw.js`、`main.tsx:11-22` 的 Service Worker 注册与 FlowAPI 站点是否兼容未验证。
6. **外部字体 CDN**：`src/index.css:1-2` 依赖 `fontsapi.zeoseven.com` 与 `cdn.jsdelivr.net`，与 FlowAPI 现有字体方案（`@fontsource-variable/public-sans`）不同。

---

## 8. Caveats / 未查实

- `/Users/Zhuyu/Documents/Code/gpt-image/deploy/repo` 的 `.git` 只有一条 commit（`774b722` v0.6.10），但 `package.json` 是 v0.7.5 且工作树有大量改动——**这是本地定制过的副本，不能当作上游某个确定 tag 的等价物**。若需要与上游精确对齐，需另行 clone 上游 `main` 比对。
- 未核对上游 GitHub 仓库当前 HEAD 与本地 v0.7.5 的差异（本机副本已足够，按任务要求未 clone）。
- 未验证 FlowAPI relay 对 `moderation` / `output_compression` / `transparent_output` / `response_format` 这 4 个字段的实际处理路径（仅确认了路由注册、`partial_images` 透传测试、以及 `stream_gate.go` 对图片事件的识别）。
- 未验证 Rsbuild + Tailwind v4 环境下原项目类名的具体失效范围（只确认版本与配置形态不同）。
- 未统计将全部中文文案抽成 i18n key 的数量。
- 「用户选令牌 → 分组」这条链路的后端细节只查到 `model/token.go:29-31`（`group` / `auto_groups` / `cross_group_retry`）与 `controller/token.go:80-90`（`getTokenRequestUserGroup` / `setTokenAutoGroups`），未展开 relay 侧的分组解析（`controller/model.go:175-190`），如 PRD 的阻塞性问题需要精确答案，应再做一轮专项调研。


## 附：被压缩掉的章节

以下章节仅存在于完整版 `gpt-image-playground.md`（同目录），实现时按需查阅，不在注入上下文内：
技术栈明细、settings「API」tab 逐字段编号、settings 其他 4 个 tab、settings 存储位置、
Responses API 调用（本任务不用）、OpenAI Images 兼容性论证、图片返回格式、依赖差集明细、
根组件与模式分发、hooks 逐文件清单。
