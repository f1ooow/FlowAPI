# Internal Flow API simplification - implementation plan

## Start gate

- [ ] 用户已审阅最终 `prd.md`、`design.md` 与本计划，并在规划摘要之后的下一条消息明确批准实施。
- [ ] 执行 `python3 .trellis/scripts/task.py start 08-26-internal-flow-api-simplification`；在此之前不得修改产品代码。
- [ ] 通过 `trellis-before-dev` 读取本任务涉及的 frontend/backend/database spec，并把实际读取的 spec/research 记录到任务上下文。
- [ ] 重新检查 `git status --short`，仅处理本任务文件；不覆盖用户当前的 `AGENTS.md`、`.agents/`、`.codex/`、`.serena/` 或其他平行改动。
- [ ] 记录基线：相关 Go 测试、`cd web && bun run test`、`bun run typecheck` 与 `bun run build` 的当前结果。

本任务采用当前会话的 inline 单代理工作流；`implement.jsonl` / `check.jsonl` 的 seed 行不作为施工上下文。若后续切换为 sub-agent dispatch，必须先替换 seed 并为两个 manifest 写入真实 spec/research 条目。

## Phase 1 - Unlimited quota foundation

### 1.1 Database model and cache propagation

- [ ] 在 `model.User` 增加 `UnlimitedQuota bool`，不添加数据库特定 SQL 或布尔默认 tag。
- [ ] 将字段加入 `User.ToBaseUser`、`UserBase`、Redis hash Lua 写入与读取测试。
- [ ] 将 `userCacheSchemaVersion` 从 `2` 升到 `3`，验证旧 hash 自动回源并重建。
- [ ] 新增 `ContextKeyUserUnlimitedQuota`，由 `UserBase.WriteContext` 写入，由 `RelayInfo` 读取。
- [ ] 将字段加入管理员用户响应与 `buildSelfUserData`，检查登录态本地存储清理逻辑不会泄露敏感字段。
- [ ] 验证 GORM AutoMigrate 在 SQLite 增列；检查生成 SQL/模型 tag 对 MySQL、PostgreSQL 兼容。

### 1.2 Administrator mutation contract

- [ ] 扩展 `ManageRequest`：动作 `set_unlimited_quota`，`enabled` 使用可区分缺失与 false 的字段。
- [ ] 在 `ManageUser` 中复用角色边界，定向更新单列并同步发布用户缓存。
- [ ] 记录 `user.unlimited_quota` 管理审计，包含旧值与新值。
- [ ] 增加 true、false、缺失 enabled、越权、目标不存在、缓存同步的控制器/模型测试。
- [ ] 确认普通 `UpdateUser`、注册和自助资料接口无法设置该字段。

### 1.3 Unified billing source

- [ ] 在 `service/billing.go` 增加 `BillingSourceUnlimited`。
- [ ] 在 `service/funding_source.go` 增加 no-op `UnlimitedFunding`。
- [ ] `NewBillingSession` 优先选择 unlimited source，不读取/扣减钱包或订阅。
- [ ] 扩展 `reserveFunding`、`rollbackFundingReserve`、`shouldTrust`、`needsRefundLocked`、`syncRelayInfo` 和低余额通知分支。
- [ ] 保持 Token 预扣、补扣、退款及统计调用不变；不能把实际 quota 或 `preConsumedQuota` 伪装为零。
- [ ] 增加有限钱包、有限订阅、无限账户 + 有限 Token、无限账户 + 无限 Token 的预扣/结算/退款/Reserve 矩阵测试。

### 1.4 Realtime and asynchronous paths

- [ ] `service/quota.go` 的 Realtime/WSS 预扣和后结算识别 unlimited，跳过账户余额变更但保留 Token 与用量统计。
- [ ] `relay/mjproxy_handler.go` 的 legacy 用户余额检查对 unlimited 放行。
- [ ] 通用 `TaskPrivateData.BillingSource` 支持 `unlimited`，结算/退款账户部分为 no-op，旧空值继续 wallet fallback。
- [ ] `model.Midjourney` 增加兼容性的 `billing_source` 字段，并在 prepare/settle/refund 全链路持久化；历史空值视为 wallet。
- [ ] 验证失败任务不会给无限用户增加钱包余额，成功任务仍增长 `used_quota`、request count 与日志费用。
- [ ] 扩展 `service/task_billing_test.go`、Realtime 和 Midjourney 回归测试，覆盖重复退款/CAS 幂等边界。

### Phase 1 validation

```bash
go test ./model ./controller ./service ./relay/common ./relay/...
go test ./service -run 'Billing|Quota|Task|Midjourney|Realtime'
go build ./...
```

Rollback point：如果任一有限用户现有测试回归，停止后续 UI/删除工作，仅回滚本阶段代码；新增数据库列保留，不做降级删列。

## Phase 2 - Unlimited quota administration UI

- [ ] 在 `web/src/features/users/types.ts`、API payload 和 auth/self types 增加 `unlimited_quota`。
- [ ] 在 `UserQuotaDialog` 增加现有 Switch 风格的无限开关；开启时禁用金额模式，关闭不修改余额；确认后把提交值回传给编辑抽屉即时渲染。
- [ ] 调用 `set_unlimited_quota` 后刷新用户列表、当前用户和相关 query cache。
- [ ] `UserQuotaCell` 无限时显示 `∞` 与累计已用量，有限时保持当前进度条。
- [ ] 概览、钱包和个人信息中识别无限状态；用户编辑抽屉开启无限后显示 `∞` 和中文说明，隐藏 `0.000000`、`$0` 与有限余额输入。
- [ ] 添加组件/API 测试：开/关 payload、禁用金额控件、无限/有限渲染、累计用量格式。

### Phase 2 validation

```bash
cd web
bun run test -- users
bun run typecheck
bun run lint
```

浏览器检查管理员将测试用户设为无限、零余额请求前状态、切回有限后的余额恢复显示。该阶段不发起付费上游请求；后端测试覆盖计费行为。

## Phase 3 - Freeze light appearance and Simplified Chinese

### 3.1 Remove customization

- [ ] 搜索所有 `ConfigDrawer`、`ThemeSwitch`、`LanguageSwitcher`、`useThemeCustomization`、`useLayout` setter 与 `useDirection` setter 引用，先处理消费者再删除实现。
- [ ] 从 app/public/error/setup/profile header 移除主题与语言控件，不留下空分隔线或移动端汉堡菜单。
- [ ] 固定 light theme、default preset/font/radius/scale、centered content、sidebar variant/icon collapse 与 LTR。
- [ ] 清理已知外观 Cookie 和 `data-theme-*` 属性；添加带旧 Cookie 的初始化回归测试。
- [ ] 保留 `resolvedTheme`、基础 CSS token 和实际仍被图表/骨架使用的固定圆角工具。
- [ ] 删除无引用配置抽屉、主题注册表、可选主题 CSS 与相关测试；运行 `knip` 确认死代码。

### 3.2 Single-language runtime

- [ ] i18n 初始化只注册简体中文，不启用浏览器 detector，不持久化语言选择。
- [ ] 删除个人语言偏好、Setup 切换器、公开页切换器及相关 API/local storage 逻辑。
- [ ] 删除不再加载的 locale 与 detector 依赖，更新 Bun lockfile 和 i18n 脚本预期。
- [ ] 保留 `t()` 抽象，补齐因删除页面或新 Flow API UI 引入的简体中文 key。
- [ ] 在 `navigator.language=en-US`、旧语言存储/Cookie 的测试环境中验证仍显示简体中文。

### Phase 3 validation

```bash
cd web
bun run test
bun run typecheck
bun run lint
bun run knip
bun run build
```

Rollback point：若固定 provider 影响图表或响应式侧栏，先恢复 provider 兼容壳但继续强制常量值，不能恢复用户选择入口作为临时修复。

## Phase 4 - Remove check-in, Chat, and Playground surfaces

### 4.1 Check-in removal

- [ ] 删除签到 router/controller/model/settings/status字段。
- [ ] 从 AutoMigrate 列表移除 `Checkin`，不添加 DROP TABLE。
- [ ] 删除前端签到卡片、日历、API、types、设置 section/registry 与文案。
- [ ] 删除只验证已移除功能实现细节的测试，补充 API 路由不存在和升级数据不被清理的回归。

### 4.2 Chat and Playground removal

- [ ] 删除侧边栏 chat module、默认 sidebar config、个人模块设置和外部聊天链接。
- [ ] 删除 `/playground`、`/chat/$chatId`、`/chat2link` route files 与专用 feature 目录；由 Router 插件重建 route tree。
- [ ] 删除 chat/playground settings、status字段、controller 与 `/pg/chat/completions` 路由。
- [ ] 清理 Dashboard/首页/错误页/翻译中的入口引用。
- [ ] 定向检查 `/v1/chat/completions` router、relay mode、channel adaptors 和协议 tests 未被删除。

### Phase 4 validation

```bash
rg -n "checkin|Playground|chat2link|/pg/chat/completions" router controller model setting web/src
go test ./router ./controller ./model ./service ./relay/...
go build ./...
cd web
bun run test
bun run typecheck
bun run lint
bun run build
```

`rg` 的剩余命中需要逐项分类：协议级 Chat Completions、历史迁移注释或测试 fixture 可以保留；产品入口和专用 `/pg` 不可保留。

## Phase 5 - Simplify dashboard overview

- [ ] 删除 `startSteps`、`quickActions`、对应 icons/callbacks/imports 与 Playground 文案。
- [ ] 删除 Dashboard FAQ 面板及其系统设置配置入口，不保留空 FAQ 卡片。
- [ ] 用现有数据模块重排概览，避免嵌套卡片和介绍性填充。
- [ ] 在 1440x900、1280x720、390x844 检查标题、统计、日志、健康信息和侧栏不重叠。
- [ ] 验证管理员、普通有限用户、普通无限用户、空数据和加载/错误状态。
- [ ] 更新 overview 组件测试，断言引导/推荐不存在且核心模块仍可操作。

## Phase 6 - Flow API brand and one-screen homepage

### 6.1 Brand asset

- [ ] 沿用项目原有 Logo 资产和几何，将产品文字改为 Flow API，不再使用本轮试制的自绘 Logo。
- [ ] 以同一资产作为默认 auth Logo 和 favicon；验证 16px、28px 与移动端尺寸。
- [ ] 将默认系统名、HTML title、默认 Logo/fallback、系统设置预览改为 Flow API，继续尊重管理员自定义名称/Logo。
- [ ] 定向审查所有用户可见 `New API` 命中；保留上游渠道类型、协议描述、源码许可、import path 和 storage key 等技术语义。

### 6.2 Default homepage rewrite

- [ ] 保留 `HomePageContent` URL/HTML/Markdown 的现有优先分支与隔离行为。
- [ ] 删除默认 `Stats`、`Features`、`HowItWorks`、`CTA`、长 Footer 和旧营销 Hero 依赖。
- [ ] 默认营销首页不渲染顶栏；Hero 包含产品名、短标题、短说明和唯一按钮。
- [ ] 匿名按钮“登录”链接 `/sign-in`；已登录按钮“进入控制台”链接 `/dashboard`；不内嵌认证表单。
- [ ] 使用现有语义 token、Public Sans、固定浅色和 reduced-motion；不新增 UI/动画依赖。
- [ ] 添加 Home 组件测试：自定义内容优先、匿名 CTA、登录 CTA、禁用营销区块、唯一主要按钮。

### 6.3 Visual browser gate

- [ ] 启动本地 dev server，使用真实应用路由验证 `/`、`/sign-in`、`/dashboard`、`/users`。
- [ ] Playwright/浏览器分别截图 1440x900、1280x720、390x844，覆盖固定浅色。
- [ ] 检查首屏高度、横向溢出、文本换行、焦点态、SVG 是否裁切、按钮是否重复、旧 Cookie 是否生效。
- [ ] 用 DOM 查询确认默认首页只有一个主要 CTA，不存在隐藏的营销区块或移动端重复操作。
- [ ] 用图片像素/可见性检查确认 SVG 非空且 favicon 可加载。

## Phase 7 - Remove subscriptions and finish unlimited quota UI

- [ ] `UserQuotaDialog` 的成功回调携带最新 `unlimited` 值，用户编辑抽屉用本地状态即时切换无限/有限展示。
- [ ] 为无限额度弹窗、抽屉状态和提示补齐简体中文词条并添加用户可见行为回归测试。
- [ ] 删除 `/subscriptions` 路由、订阅 feature、侧栏/钱包/用户行/系统设置中的订阅入口，并重新生成路由树。
- [ ] 删除 `/api/subscription/...` 与订阅商品 option 路由，停止订阅额度重置任务。
- [ ] 新计费会话只在 `UnlimitedFunding` 和 `WalletFunding` 中选择，不再使用订阅余额；保留历史订阅表和审计展示，不执行破坏性清理。

## Phase 8 - Full verification and review

### Backend

```bash
gofmt -w <touched-go-files>
go vet ./...
go test ./...
go build ./...
```

若未修改 `relaykit/`，无需额外独立模块构建；若施工中触及其代码或公开 API，必须追加：

```bash
cd relaykit
GOWORK=off go build ./...
```

### Frontend

```bash
cd web
bun run format:check
bun run copyright:check
bun run lint
bun run typecheck
bun run test
bun run knip
bun run build
```

### Repository and contract audit

```bash
git diff --check
git status --short
rg -n "ConfigDrawer|LanguageSwitcher|ThemeSwitch|/pg/chat/completions|checkin" web/src router controller model setting
rg -n "New API|new-api|QuantumNous" web/src web/public web/index.html
```

- [ ] 对剩余命中逐条说明保留原因，不用全仓替换掩盖协议/许可边界。
- [ ] 使用 SQLite 完成真实迁移和开关回归；MySQL/PostgreSQL 以 GORM 跨方言测试/CI 为最低门槛，若无可用实例必须明确标注未做 live migration。
- [ ] 不把付费上游模型请求作为必需验证；标准 `/v1/chat/completions` 可用 router/handler integration test 或本地 mock upstream 验证。
- [ ] 运行 `trellis-check`，逐项对照 R1-R10 与 acceptance criteria，修复后再复查。
- [ ] 在最终交付中分别列出已验证、未验证、跳过和依赖外部服务的项目。

## Explicitly deferred

- 域名、证书、管理员凭据和服务器同步脚本停用属于独立运维变更；RC 部署已获用户明确授权，仍须遵守 server skill 的资产确认流程。
- 开源发布时恢复 new-api 上游归属是独立发布门禁，不在内部 Flow API 构建中提前展示。
- 不删除历史签到表或其他生产数据。
- 不删除历史订阅表、订单或其他订阅数据。
- 不重命名技术层 module/import/package/image/storage key。
# Latest acceptance additions

- [x] Remove OAuth, custom OAuth, Passkey and 2FA from sign-in, profile, system settings and admin user actions.
- [x] Register only the email binding endpoint under `/api/user/email/bind`; do not register OAuth, Passkey, 2FA, login-session management or custom OAuth provider routes.
- [x] Make password login independent of historical 2FA state so existing accounts cannot be locked out after the UI removal.
- [x] Reduce profile settings to email binding and password change; remove settings/preferences, notifications, access tokens and third-party binding modules.
- [x] Preserve historical authentication tables and records; do not add destructive migrations.

## RC execution record - 2026-08-27

- Deployed `flowapi:rc-auth-simplify-zh-20260826-2345` to the RC experiment environment at `https://fallback.robusta.top`; app, PostgreSQL and Redis are healthy and the app restart count is zero.
- Preserved PostgreSQL, Redis and application volumes. Pre-deployment PostgreSQL backup: `/opt/flowapi/backups/pre-auth-simplify-20260826-233311.dump`; compose backup: `/opt/flowapi/backups/docker-compose.yml.pre-auth-simplify-20260826-233311`.
- Verified the public homepage at 1440x900 and 390x844, authenticated dashboard, profile and users pages in the real RC browser path. The test user renders `infinite` quota and the quota dialog is Chinese with the infinite switch enabled.
- Verified `/playground`, `/subscriptions` and `/oauth/github` render the not-found page; `/console/faq` redirects to the dashboard. Subscription, OAuth, Passkey, 2FA, check-in and Playground handlers are no longer registered.
- Passed `go test ./...`, `go vet ./...`, `go build ./...`, frontend type-check, 148 frontend tests, production frontend build, targeted changed-file lint and `git diff --check`.
- Full-repository lint, copyright and knip retain pre-existing baseline findings outside this task. No paid upstream model request was made, and no live MySQL migration was performed; SQLite tests and the RC PostgreSQL migration path passed.
- The task remains active for the user's RC visual review; do not archive it before that feedback is incorporated.
