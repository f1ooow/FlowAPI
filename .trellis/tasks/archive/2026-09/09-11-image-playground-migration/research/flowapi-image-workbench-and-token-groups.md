# Research: FlowAPI 生图工作台现状 + 令牌可用分组机制

- **Query**: 现有生图工作台页面结构与调用链；令牌「可用分组」前后端下发机制；在生图页加「选分组 → 自动选该分组下令牌」需要什么
- **Scope**: internal（只读代码调研，无外部检索）
- **Date**: 2026-09-11
- **Repo**: `/Users/Zhuyu/Documents/Code/FlowAPI`（branch `main`）

---

## 一、现有「生图工作台」前端页面

### 1.1 路由与入口

| 项 | 位置 |
|---|---|
| 路由文件 | `/Users/Zhuyu/Documents/Code/FlowAPI/web/src/routes/_authenticated/playground/index.tsx`（全文 26 行） |
| 路由定义 | 同上 line 23：`createFileRoute('/_authenticated/playground/')` |
| 页面组件 | 同上 line 21 导入 `ImagePlayground`，line 24 `component: ImagePlayground` |
| 路由生成文件 | `web/src/routeTree.gen.ts`（TanStack Router 自动生成） |
| 侧边栏菜单 | `/Users/Zhuyu/Documents/Code/FlowAPI/web/src/hooks/use-sidebar-data.ts` line 74-77：`title: t('生图工作台')`、`url: '/playground'`、`icon: Image` |

注意：`/playground` 是**唯一**生图入口。`web/src/features/chat/` 与 `web/src/features/playground/` 目录存在但**只剩空目录**（`hooks/`、`lib/`、`components/` 全为空），无任何文件，不是生图实现。

### 1.2 组件/模块清单（路径 + 职责 + 行数）

feature 根目录：`web/src/features/image-playground/`

| 文件（相对 `web/src/features/image-playground/`） | 行数 | 职责 |
|---|---|---|
| `index.tsx` | 19 | barrel，仅 `export { ImagePlayground } from './components/image-playground'` |
| `components/image-playground.tsx` | 768 | 主页面：左侧 composer（prompt/参数/上传/生成按钮）+ 右侧画廊；token 下拉、拖拽/粘贴上传、右键菜单、图片预览、mask 入口 |
| `components/task-card.tsx` | 239 | 画廊里单个任务卡片（导出 `TaskCard`，line 122） |
| `components/task-detail.tsx` | 225 | 任务详情抽屉/弹窗（导出 `TaskDetail`，line 61） |
| `components/mask-editor.tsx` | 363 | 画笔/橡皮 mask 编辑器 |
| `hooks/use-image-playground.ts` | 741 | 全部业务状态：prompt/params/输入图/mask/任务列表/IndexedDB 持久化/submit/cancel/reuse/edit/retry/delete；导出 `useImagePlayground()`（line 309）与类型 `ImagePlaygroundState`（line 741） |
| `lib/api.ts` | 229 | 调 relay 生图/改图接口 + 响应归一化 + token key 解析 |
| `lib/tokens.ts` | 48 | 拉取用户令牌列表（分页聚合） |
| `lib/storage.ts` | 228 | IndexedDB（库名 `flowapi-image-playground`）任务/图片持久化 |
| `lib/mask-preprocess.ts` | 260 | mask 尺寸归一、校验、预览生成 |
| `lib/mask-canvas.ts` | 41 | mask 画布合成 |
| `lib/image-utils.ts` | 76 | 下载、复制到剪贴板、取图片尺寸 |
| `constants.ts` | 67 | 模型名、参数选项、IndexedDB 常量 |
| `types.ts` | 151 | 全部前端类型 |
| 测试 | `components/__tests__/image-playground.test.tsx`(280)、`lib/__tests__/api.test.ts`(176)、`lib/__tests__/storage.test.ts`(38)、`lib/__tests__/mask-canvas.test.ts`(51)、`lib/__tests__/mask-preprocess.test.ts`(79) | |

`index.tsx` 只导出 `ImagePlayground`；`TaskCard` / `TaskDetail` / `MaskEditor` 仅在 feature 内部被引用。

### 1.3 生图接口调用链

**模型常量**：`web/src/features/image-playground/constants.ts` line 26：`export const IMAGE_PLAYGROUND_MODEL = 'gpt-image-2'`（前端硬编码，不走 `/api/user/models`）。

**端点**（`web/src/features/image-playground/lib/api.ts`）：
- 生图：line 178 `fetch('/v1/images/generations', { method: 'POST', ... })`，body 见 line 169-177：`{ model, prompt, size, quality, output_format, n, moderation: 'auto' }`，`Content-Type: application/json`
- 改图：line 221 `fetch('/v1/images/edits', { method: 'POST', ... })`，`body` 是 `FormData`（line 204-219）：`model` / `prompt` / `size` / `quality` / `output_format` / `n` / `moderation` / `image`（多图时 `image[]`）/ 可选 `mask`
- 鉴权头：`requestHeaders()` line 144-148 → `Authorization: Bearer sk-xxx`；`normalizeApiKey()` line 140-142 会给不带 `sk-` 的 key 补前缀
- 输出数量上限：`boundedOutputCount()` line 150-153，钳制到 `IMAGE_PLAYGROUND_MAX_OUTPUTS = 4`（`constants.ts` line 27）
- 响应归一化：`normalizeImageResponse()` line 121-138，读 `payload.data[].b64_json | url | revised_prompt`，转 Blob / objectURL
- 前端不含任何 `group` 字段 —— 请求体里**没有** group

**key 来源**（关键）：
```
lib/tokens.ts:19  getApiKeys({ p, size: 100 })   → GET /api/token/?p&size   (循环分页)
   ↓ 映射为 PlaygroundToken { id, name, maskedKey, status }（line 35-41）
components/image-playground.tsx:310-330  useEffect 拉列表，默认选中 items[0].id
components/image-playground.tsx:546-571  <select id='image-playground-token'> 手动选 key
hooks/use-image-playground.ts:565  const apiKey = await resolvePlaygroundToken(tokenId)
lib/api.ts:155-161  resolvePlaygroundToken → fetchTokenKey(tokenId) → POST /api/token/{id}/key → 明文 key
```
即：**先选令牌 → 换取明文 key → 前端拿明文 key 打 `/v1/images/*`**。

### 1.4 与 relay 的对接（后端侧）

- 路由：`router/relay-router.go` line 110-115，`/v1/images/generations` 与 `/v1/images/edits`（还有 `/v1/edits`），均走 `controller.Relay(c, types.RelayFormatOpenAIImage)`
- 中间件链：`relayV1Router`（line 66-70）`RouteTag("relay") → SystemPerformanceCheck() → TokenAuth() → ModelRequestRateLimit()`，随后 `httpRouter.Use(middleware.Distribute())`（line 82）
- 前端 dev 代理：`web/rsbuild.config.ts` line 20 已代理 `['/api', '/mj', '/pg', '/v1']`，所以 `/v1/*` 同源直发没问题

### 1.5 store / hooks / types / i18n

- **store**：生图**没有**用 `web/src/stores/` 下任何 store（`auth-store.ts` / `notification-store.ts` / `system-config-store.ts`）。状态全在 `useImagePlayground()` 内部 `useState`；持久化在 IndexedDB（`lib/storage.ts`，库名 `flowapi-image-playground`，`constants.ts` line 64-67）
- **hooks**：只有 `hooks/use-image-playground.ts`；全局 `web/src/hooks/` 下没有任何生图相关 hook
- **types**：`web/src/features/image-playground/types.ts`，关键类型 `PlaygroundToken`（line 34-39，字段 `id / name / maskedKey / status`，**无 group**）、`ImageRequestParams`（line 27-32）、`PlaygroundTask`（line 63-82）、`StoredTask`（line 124-151）
- **i18n key**：组件里用 `t('…')`，英文源串作 key，例如 `'API key'`、`'Select an API key'`、`'Loading API keys...'`、`'Unable to load API keys'`、`'Select an API key before generating'`、`'Generate image'`、`'Generating'`、`'Size' / 'Quality' / 'Format' / 'Quantity'`、`'Your gallery is empty'` 等（完整清单见 `components/*.tsx` 与 `hooks/use-image-playground.ts` 的 `t()` 调用）。侧边栏标题用中文 key：`web/src/hooks/use-sidebar-data.ts` line 74 `t('生图工作台')`
- **i18n 现状（重要）**：`web/src/i18n/locales/` 下**只有 `zh.json`**（外加 `_extras/`、`_reports/_sync-report.json`）；`web/src/i18n/config.ts` 只注册 `zhCN`，`supportedLngs: ['zhCN']`，`lng/fallbackLng: 'zhCN'`。虽然 `AGENTS.md` 描述多语言，但当前代码是单语言 zh。同步脚本 `web/scripts/sync-i18n.mjs`（`package.json` line 20 `i18n:sync`）

### 1.6 本地是否有 gpt_image_playground 副本

未在 `/Users/Zhuyu/Documents/Code` 下找到 `gpt_image_playground` / `gpt-image-playground` 目录。找到的最接近项：
- `/Users/Zhuyu/Documents/Code/gpt-image/` — 内容是 `.claude/`、`.cursor/`、`.trellis/`、`deploy/`、`AGENTS.md`，**不是**该前端项目
- `/Users/Zhuyu/Documents/Code/tmp-repos/` — `huobao-drama`
- 仅有历史 agent-browser 配置残留：`/Users/Zhuyu/.agent-browser/image-playground-settings-20260811.config` 等

结论：**PRD 中「用户已在本地持有 gpt_image_playground 代码副本」这一条在本次调研范围内无法证实**，需要向用户确认路径。

---

## 二、令牌「可用分组」机制

### 2.1 概念区分（三者不同）

1. **用户自己的 group** — `model.User.Group`（`model/user.go`；读取函数 `model.GetUserGroup(id int, fromDB bool)`，`model/user.go` line 1212）
2. **令牌上的 group** — `model.Token.Group`，`model/token.go` line 29：`Group string \`json:"group" gorm:"default:''"\``；空串代表「跟随用户分组」
3. **令牌可选分组列表** — 由「用户分组」推导出的 `GetUserUsableGroups(userGroup)`，即 `/api/user/self/groups` 返回的集合

### 2.2 后端计算与下发

**可用分组计算**
- `service/group.go` line 14-41 `GetUserUsableGroups(userGroup string) map[string]string`：起点 `setting.GetUserUsableGroupsCopy()`，再按 `ratio_setting.GetGroupRatioSetting().GroupSpecialUsableGroup.Get(userGroup)` 做 `-:` 删除 / `+:` 添加 / 直接添加；若 userGroup 本身不在集合里，则补进 `userGroup -> "用户分组"`
- `service/group.go` line 43-46 `GroupInUserUsableGroups(userGroup, groupName) bool`
- `service/group.go` line 48-53 `IsUserSelectableGroup(userGroup, groupName) bool`：排除 `""` 与 `auto`，且要求 `ratio_setting.ContainsGroupRatio(groupName)`
- `service/group.go` line 56-70 `GetUserAutoGroup(userGroup)` 用户级 auto 顺序
- `service/group.go` line 74-92 `FilterUserTokenAutoGroups(userGroup, groups)` 令牌级 auto 快照（受 `setting.GetMaxTokenAutoGroups()` 限制）
- `service/group.go` line 97-107 `GetRequestAutoGroups(c, userGroup)` 供 distributor 解析
- `service/group.go` line 127-136 `GetUserGroupRatio(userGroup, group) float64` 倍率
- 设置源：`setting/user_usable_group.go` line 16 `GetUserUsableGroupsCopy()`；`setting/ratio_setting/group_ratio.go` line 42 `GroupSpecialUsableGroup`

**接口 `/api/user/self/groups`**
- 路由：`router/api-router.go` line 84 `selfRoute.GET("/self/groups", controller.GetUserGroups)`（同文件 line 83 另有匿名版 `userRoute.GET("/groups", controller.GetUserGroups)`）
- 控制器：`controller/group.go` line 26-84 `GetUserGroups`
  - line 32 取用户分组：`model.GetUserGroup(userId, false)`
  - line 34 计算可用集合：`service.GetUserUsableGroups(userGroup)`
  - 逐分组组装：`desc`（line 38）；登录用户额外带 `ratio_kind`（`single` / `range` / `auto` / `unavailable`）、`ratio_min`、`ratio_max`、`available`（line 39-62）
  - line 67-78 单独处理 `auto`：`{ratio_kind: 'auto', available: true, desc: setting.GetUsableGroupDescription('auto')}`
  - line 79-83 响应：`{success, message, data: { <groupName>: {...} }}`

**令牌接口**
- 路由：`router/api-router.go` line 157-168 `tokenRoute`（`middleware.UserAuth()`）
  - line 159 `GET /api/token/` → `GetAllTokens`
  - line 160 `GET /api/token/search` → `SearchTokens`
  - line 161 `GET /api/token/auto-groups` → `GetTokenAutoGroups`
  - line 162 `GET /api/token/:id` → `GetToken`
  - line 163 `POST /api/token/:id/key` → `GetTokenKey`（返回明文 key）
  - line 168 `POST /api/token/batch/keys` → `GetTokenKeysBatch`
- 令牌列表响应结构：`controller/token.go` line 65-77 `buildMaskedTokenResponse`（`Key` 换成 `GetMaskedKey()`，附 `auto_groups`）；`tokenResponse` 定义在 line 35-38（内嵌 `*model.Token` + `auto_groups`）。因为内嵌的是 `model.Token`，**`group` 字段在列表/详情响应里是有的**（`json:"group"`），`auto_groups` 在 `model.Token` 上是 `json:"-"`，由 `tokenResponse` 单独补出
- `GetTokenAutoGroups` line 176-186 返回 `{groups: service.GetUserAutoGroup(userGroup), max_count: setting.GetMaxTokenAutoGroups()}`
- 令牌写入时校验分组：`controller/token.go` line 90-125 `setTokenAutoGroups`，逐项 `service.IsUserSelectableGroup(userGroup, group)`，非法则 `i18n.MsgTokenAutoGroupsInvalid`；上限 `i18n.MsgTokenAutoGroupsTooMany`；`getTokenRequestUserGroup` 见 line 79-87
- **没有**「按 group 过滤令牌」的服务端参数：`model.GetAllUserTokens(userId, startIdx, num)`（`model/token.go` line 106-111）只按 `user_id` 排序分页；`SearchUserTokens`（`model/token.go` line 159+）只支持 keyword / token 关键词

**请求时分组如何生效（relay 侧）**
- `middleware/auth.go` line 459-476 `TokenAuth()`：`tokenGroup := token.Group`；非空时先校验在 `service.GetUserUsableGroups(userGroup)` 内，再校验 `ratio_setting.ContainsGroupRatio(tokenGroup)`（`auto` 例外），通过则 **`userGroup = tokenGroup`**；line 477 `SetContextKey(… ContextKeyUsingGroup, userGroup)`
- `middleware/auth.go` line 504 `SetContextKey(… ContextKeyTokenGroup, token.Group)`；line 508-518 若 `token.AutoGroups != ""` 则写入 `ContextKeyTokenAutoGroups`
- `middleware/distributor.go` line 110-134：`usingGroup == "auto"` 时用 `service.GetRequestAutoGroups(c, userGroup)` 依次尝试
- `middleware/distributor.go` line 88-104：**唯一**的「请求里带 group 覆盖」路径，且仅对 `/pg/chat/completions` 生效 —— 解析 `dto.PlayGroundRequest.Group`，用 `service.GroupInUserUsableGroups` 校验后覆盖 `ContextKeyUsingGroup`；line 405-406 也只在该路径下 `modelRequest.Group = req.Group`
  - 注意：`/pg/chat/completions` 这个路由在 `router/` 下**当前未注册**（全仓库 grep 无匹配），只剩 distributor 里的历史分支

结论：对 `/v1/images/generations`、`/v1/images/edits`，**请求体里的 `group` 字段不会被读取**；实际使用的分组完全由 `Authorization` 头里那枚令牌自身的 `Token.Group`（空则回落用户分组）决定。

### 2.3 前端已有用法

| 位置 | 调用/形状 |
|---|---|
| `web/src/lib/api.ts` line 57-75 | `getUserGroups()` → `api.get('/api/user/self/groups')`，类型 `UserGroupDisplay`（line 57-66）= `{desc, ratio_kind: 'single'│'range', ratio_min, ratio_max, available: true}` 或 `{desc, ratio_kind:'auto', available:true}` 或 `{desc, ratio_kind:'unavailable', available:false}` |
| `web/src/features/keys/components/api-keys-mutate-drawer.tsx` line 126-134 | `useQuery({ queryKey: ['user-groups'], queryFn: getUserGroups, enabled: open })` |
| 同上 line 157-170 | 映射为 `ApiKeyGroupOption[]`：`{value: key, label: key, desc: info.desc, ratio: groupRatioFromApi(info), disabled: info.available === false}` |
| 同上 line 422-444 | 表单里的 `<ApiKeyGroupCombobox>` 分组下拉（令牌的 `group` 字段） |
| 同上 line 456+ / `auto-group-order-editor.tsx` line 231 | auto 分组顺序编辑器，复用同一个 combobox |
| 同上 line 140-151 | `useQuery({ queryKey: ['token-auto-groups'], queryFn: getTokenAutoGroups })` → `GET /api/token/auto-groups` |
| `web/src/features/keys/components/api-keys-columns.tsx` line 58-62 | 表格单元格也调 `getUserGroups` 显示倍率徽章 |
| `web/src/features/keys/api.ts` line 36-42 / 59-62 / 65-70 / 112-117 / 120-127 | `getApiKeys` / `getApiKey` / `getTokenAutoGroups` / `fetchTokenKey` / `fetchTokenKeysBatch` |
| `web/src/features/keys/types.ts` line 24-52 | `apiKeySchema`：`group: z.string().nullish().default('')`、`auto_groups: z.array(z.string()).nullish().default(null)`、`status: 1\|2\|3\|4` |
| `web/src/features/keys/components/__tests__/api-keys-mutate-drawer.test.tsx` line 52-80 | `/api/user/self/groups` 的 mock 数据形状（`auto` / `default` / `vip`，含 `ratio_range`） |

可复用的现成件：
- 组件 `web/src/features/keys/components/api-key-group-combobox.tsx`（`ApiKeyGroupCombobox`，line 63；props `{options: ApiKeyGroupOption[], value, onValueChange, placeholder, disabled}`，`ApiKeyGroupOption` 定义 line 47-53）
- 数据转换 `web/src/features/keys/lib/group-ratio-display.ts`：`groupRatioFromApi(info: UserGroupDisplay): GroupRatio`、`formatGroupRatio(ratio, auto, unavailable)`
- 徽章 `web/src/components/group-badge.tsx`（`GroupBadge`）、`web/src/features/keys/components/auto-group-visuals.tsx`（`GroupRatioBadge` / `AutoGroupFlowBorder`）
- 但注意：以上都是 `features/keys/**` 内部文件，生图页复用需要跨 feature import（或提升共享位置）

### 2.4 现有生图页怎么选 token / key

- 现有的「从令牌里选」先例**就在生图页里**：`components/image-playground.tsx` line 310-330 拉 `getPlaygroundTokens()`，line 546-571 原生 `<select>`（`id='image-playground-token'`）展示 `{token.name} · {token.maskedKey}`，line 316 默认选第一个
- key 明文获取走 `lib/api.ts` line 155-161 `resolvePlaygroundToken(tokenId)` → `POST /api/token/{id}/key`
- `PlaygroundToken`（`types.ts` line 34-39）**没有 group 字段**，`lib/tokens.ts` line 35-41 映射时只挑了 `id / name / key / status` —— 也就是说 API 返回里其实带着 `group`，只是被丢掉了
- 其他 feature 里没有「按分组自动挑令牌」的先例：`features/chat/`、`features/playground/` 是空目录；`features/keys/components/dialogs/cc-switch-dialog.tsx` 只是把已有明文 key 拼成第三方客户端 URL
- 生图页选中的 key 会写进任务记录（`PlaygroundTask.tokenId` / `tokenName`，`types.ts` line 70-71；落库见 `hooks/use-image-playground.ts` line 516-533、`taskToStored` line 220-247），所以加入分组维度后任务模型也需要相应字段

---

## 三、要加「选择分组 → 自动用该分组下的可用令牌」需要什么

### 3.1 后端：已存在、可直接用的接口

| 能力 | 接口 | 位置 |
|---|---|---|
| 当前用户可用分组 + 倍率 | `GET /api/user/self/groups` | `router/api-router.go:84` → `controller/group.go:26` |
| 用户令牌列表（含 `group`、`auto_groups`、`status`、`key` 掩码） | `GET /api/token/?p=&size=` | `router/api-router.go:159` → `controller/token.go:127`（`buildMaskedTokenResponse` line 65） |
| 单令牌详情 | `GET /api/token/:id` | `router/api-router.go:162` → `controller/token.go:161` |
| 令牌明文 key | `POST /api/token/:id/key` | `router/api-router.go:163` → `controller/token.go:188` |
| 批量明文 key | `POST /api/token/batch/keys` | `router/api-router.go:168` → `controller/token.go` |
| 用户级 auto 分组顺序 | `GET /api/token/auto-groups` | `router/api-router.go:161` → `controller/token.go:176` |
| 令牌搜索（keyword / token） | `GET /api/token/search` | `router/api-router.go:160` → `controller/token.go`（`model.SearchUserTokens`） |

### 3.2 后端：当前缺失

1. **按 group 过滤令牌的接口/参数**。`GET /api/token/` 无 `group` 参数（`model/token.go:106-111`）；`GET /api/token/search` 只认 keyword / token（`model/token.go:159+`）。→ 要么前端拉全量后本地筛选（`lib/tokens.ts` 已经在做 100/页循环聚合，扩展成本最低），要么后端新增过滤参数
2. **「某分组下有哪些可用令牌」的服务端语义**。「可用」需要定义：`status == 1`（`common.TokenStatusEnabled`）且 `group == 该分组`（或 `group == ''` 视为跟随用户分组？）。当前没有任何后端函数表达这个语义
3. **请求级分组覆盖**。relay 侧只认令牌自身的 `Token.Group`；`group` 入参覆盖逻辑只存在于 `/pg/chat/completions`（`middleware/distributor.go:88-104`、`405-406`），而该路由未注册。若希望「一个令牌 + 前端指定分组」而不是「按分组选不同令牌」，则缺后端支持。按 PRD 的措辞（「用该分组下可用的令牌」）走前者即可，**不需要**新增 relay 侧能力
4. **令牌级 auto 语义与本需求的边界**：`Token.Group == 'auto'` 时实际分组由 `service.GetRequestAutoGroups` 决定（`middleware/distributor.go:110-134`），前端无法静态判定「这个 auto 令牌属于哪个分组」

### 3.3 前端：可复用 / 需要新增

可复用：
- 数据：`getApiKeys()`（`web/src/features/keys/api.ts:36`）已返回含 `group` 的令牌；`getUserGroups()`（`web/src/lib/api.ts:68`）已返回分组字典与倍率
- UI：`ApiKeyGroupCombobox`（`web/src/features/keys/components/api-key-group-combobox.tsx:63`）+ `groupRatioFromApi`（`web/src/features/keys/lib/group-ratio-display.ts`）+ `GroupBadge`（`web/src/components/group-badge.tsx`）
- 明文 key：`fetchTokenKey`（`web/src/features/keys/api.ts:112`）已有，`lib/api.ts:155 resolvePlaygroundToken` 已封装
- 查询缓存：`@tanstack/react-query` 已是既有依赖，`queryKey: ['user-groups']` / `['api-keys']` 可与 keys feature 共享缓存

需要新增/改造（描述现状缺口，不含实现建议）：
- `web/src/features/image-playground/types.ts` 的 `PlaygroundToken`（line 34-39）目前丢弃了 `group`；`lib/tokens.ts` line 35-41 的映射同样丢弃
- `PlaygroundTask` / `StoredTask`（`types.ts` line 63-82、124-151）只有 `tokenId` / `tokenName`，无分组维度
- `hooks/use-image-playground.ts` 的 `submit(tokenId, tokenName)`（line 486-487）签名只接受令牌维度
- `components/image-playground.tsx` 的 token 选择 UI（line 546-571）是原生 `<select>`，与 `ApiKeyGroupCombobox` 风格不一致
- 新增 i18n key 需要同步 `web/src/i18n/locales/zh.json`（当前唯一 locale），走 `web/scripts/sync-i18n.mjs`

### 3.4 其它相关约束（调研中发现）

- `web/src/features/image-playground/` 与 `web/src/features/keys/` 是平级 feature，跨 feature import 在本仓库已有先例（如 `image-playground/lib/api.ts:19` 直接 import `@/features/keys/api`），所以复用 keys 的分组组件在形式上可行
- 令牌 `status` 取值 1=启用 / 2=禁用 / 3=过期 / 4=耗尽（`web/src/features/keys/types.ts:31` 注释、`common.TokenStatus*`），筛选「可用令牌」时前端可用该字段
- `web/rsbuild.config.ts:20` 代理已含 `/api` 与 `/v1`，新增分组接口调用无需改代理

---

## 四、Caveats / 未找到

1. **本地未见 `gpt_image_playground` 源码副本** —— 详见 1.6，需用户提供路径
2. `/pg/chat/completions` 在 Go 侧只有 distributor 的解析分支，**没有注册路由**（`router/` 全量 grep 无匹配），属于历史残留；不要假设它可用
3. 本仓库当前 i18n 只有 `zh.json` 单一语言（`web/src/i18n/config.ts`），与根 `AGENTS.md` 描述的多语言不一致；新增文案只需改 `zh.json`
4. `web/src/features/chat/`、`web/src/features/playground/` 是空目录，不含实现
5. `session: 09-11-image-playground-migration` 的 `prd.md` 在调研开始时的 `Goal/Requirements` 为 TBD，任务描述已由调用方在消息中给出；本文件按调用方消息中的三个研究目标组织
