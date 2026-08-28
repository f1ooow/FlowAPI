# Internal Flow API simplification - technical design

## Status

本设计已获用户批准并进入施工；实现以同目录最新 `prd.md` 和用户最新明确反馈为产品合同，并持续部署到 RC 验收。

## Architecture boundaries

本任务分成六条边界清晰但需要一起验收的变更链：

1. 前端壳层收敛：固定主题、布局、方向和简体中文，删除用户选择入口及旧偏好影响。
2. 产品表面删减：精简概览，移除签到、聊天、Playground、订阅和空 FAQ 面板的 UI、路由、设置及专用后端入口。
3. 默认公开首页与品牌：用单屏 Flow API 入口替换默认营销页，并统一用户可见品牌资产。
4. 无限账户额度：在用户、缓存、鉴权上下文、计费、异步任务和管理 UI 之间增加显式布尔合同。
5. 兼容与数据保留：保留标准 Relay 协议、管理员自定义首页、历史签到表、旧前端配置归一化和非展示层技术标识。
6. 账号认证收敛：只保留邮箱、用户名与密码路径，删除 OAuth、Passkey、2FA 和偏好类账号功能的所有运行时入口。

不得把这些边界扩大为 Relay 协议重写、数据库清理或 Go module 重命名。代码中的 AGPL 许可头不是用户可见 rebrand 目标，必须保留。

## 1. Fixed frontend shell

### 1.1 Appearance contract

固定结果为：仅浅色、`default` 颜色预设、Auto 字体、Auto 圆角、默认密度、`sidebar` 侧栏、`icon` 折叠方式、默认页面布局、`centered` 内容宽度和 `ltr` 方向。

实现不是只隐藏 `ConfigDrawer`：

- 从应用顶栏、公开页、错误页、Setup 和个人设置中删除 `ConfigDrawer`、`ThemeSwitch`、`LanguageSwitcher` 及其可见入口。
- 根应用不再从 `theme_preset`、`theme_font`、`theme_radius`、`theme_scale`、`theme_content_layout`、`layout_variant`、`layout_collapsible`、`dir` 和 `vite-ui-theme` 读取用户选择。
- 初始化时一次性清除这些历史 Cookie，并移除遗留 `data-theme-*` 属性，防止升级用户继续得到不同外观。
- `ThemeProvider` 固定提供浅色状态，不监听 `prefers-color-scheme`，也不暴露可持久化的手动模式选择。
- `LayoutProvider` 和 `DirectionProvider` 如仍被共享组件需要，应改为只提供固定值；删除仅服务于配置抽屉的 setter/reset API 和未使用分支。
- `ThemeCustomizationProvider`、主题选项注册表和只服务于用户自定义的代码在引用清空后删除。默认浅色 CSS token 和通用圆角工具继续保留。

这样可以保证新浏览器、已有 Cookie 的浏览器、桌面端和移动端使用同一浅色产品形态，同时不破坏响应式侧栏。

### 1.2 Simplified Chinese runtime

保留 React 组件中的 `useTranslation()` / `t()` 调用，避免把本次产品收敛变成全仓文案重写；运行时改为只注册简体中文资源：

- `web/src/i18n/index.ts` 不再使用浏览器语言检测器，也不读取或写入用户语言偏好。
- 删除语言菜单、个人资料语言设置和 Setup 语言入口。
- 移除非简体中文 locale 的运行时导入；确认 i18n 工具脚本与依赖后，删除不再使用的 detector 和 locale 文件。
- 明确设置 `lng` 与 `fallbackLng` 为简体中文，并保持技术值、模型名、代码片段和协议字段原文。

后台错误国际化和外部 API 合同不属于本轮单语言前端的删除范围。

## 2. Product surface removal

### 2.1 Dashboard overview

在 `overview-dashboard.tsx` 删除 `startSteps`、`quickActions`、FAQ 面板及对应渲染。剩余概览按现有数据源重排为无嵌套卡片的工作台：第一行保留关键用量/余额或健康摘要，后续保留实际日志和趋势内容。不得用介绍文案、教程、推荐卡片或空 FAQ 卡片填补空位。FAQ 配置页同时删除，避免保留无消费者设置。

无限账户的概览余额应显示 `∞` 与累计已用量，不能显示 `$0` 或把无限状态当作零余额。

### 2.2 Check-in

删除签到控制器、路由、运行时设置、状态字段、前端 API/type/UI 和相关测试。`model/main.go` 不再为新部署 AutoMigrate `Checkin`，但不新增 `DROP TABLE`、数据迁移或启动清理逻辑；已有 `checkins` 表自然休眠。

如果签到配置键仍存在于升级数据库，后端忽略它；系统设置接口和 UI 不再暴露该键。路由验收以 `/api/user/checkin` 相关端点返回不存在为准，而不是返回“功能关闭”。

### 2.3 Chat and Playground

产品表面删除包括：

- 侧边栏聊天分组、外部客户端列表和个人侧栏配置中的相应模块。
- `/playground`、`/chat/$chatId`、`/chat2link` 路由及只为这些页面服务的 feature 代码。
- Playground 系统设置、状态字段、`controller/playground.go` 与 `/pg/chat/completions` 专用路由。
- Dashboard、首页、错误页或其他文案中对 Playground 的入口和引导。

TanStack Router 的生成路由树由现有构建流程重新生成，不手工伪造残留声明。

协议保留边界是标准 `/v1/chat/completions` 及其渠道 adaptor、模型能力与计费链。删除检查必须按“产品专用 `/pg` 不存在，标准 `/v1` 仍注册”成对验收，避免误删 Chat Completions Relay。

### 2.4 Subscriptions

删除订阅计划作为可用产品能力，但保留历史数据：

- 删除 `/subscriptions` 路由、侧栏项、钱包订阅卡、用户行“管理订阅”操作、系统设置订阅开关及整个订阅 feature。
- 后端不注册 `/api/subscription/...` 和仅用于创建/列出订阅商品的 option 路由，不启动订阅额度重置后台任务。
- `NewBillingSession` 不再读取订阅偏好或选择 `SubscriptionFunding`；无限用户选择 `UnlimitedFunding`，其余用户统一选择 `WalletFunding`。
- 现有订阅表、订单和日志保留，不新增 `DROP TABLE` 或启动清理；历史日志中的订阅资金来源可继续只读展示，以免破坏审计记录。

### 2.5 Email/password-only account model

认证收敛按“路由不可达 + 界面不可见 + 历史数据不破坏”实施：

- 登录页删除 OAuth provider、WeChat/Telegram、Passkey 和 2FA 后续步骤，只提交用户名/邮箱与密码。
- 后端停止注册 OAuth、Passkey、2FA、登录会话管理和自定义 OAuth provider 管理路由；保留邮箱验证码、密码登录、注册、找回密码和邮箱绑定。
- 邮箱绑定迁移到 `/api/user/email/bind`，不继续使用 `/api/oauth/email/bind`。
- `Login` 不再读取历史 `TwoFA` 状态或创建 2FA 登录 flow；旧账号成功验证密码后直接建立标准登录 session。
- 个人资料改成无 Tab 的邮箱与密码页面；删除通知偏好、非邮箱绑定、登录会话、访问令牌、Passkey 和 2FA 卡片。
- 认证系统设置只保留基础认证与机器人防护；删除 OAuth、Custom OAuth 和 Passkey section。
- 相关 controller/model/service 可以因历史数据兼容继续存在为未注册代码，但不得有运行时路由、后台任务或用户可见配置入口；不在本任务内执行表删除。

## 3. Default homepage and Flow API identity

### 3.1 Rendering contract

`web/src/features/home/index.tsx` 继续先处理管理员 `HomePageContent`：URL 仍渲染受限 iframe，HTML/Markdown 仍走当前 `RichContent`。只有内容为空时才进入新的默认首页，因此已有实例的自定义首页行为不变。

默认首页由一个轻量页面组件承担，不继续拼装旧 `Hero`、`Stats`、`Features`、`HowItWorks`、`CTA` 和长 `Footer`：

- 默认营销首页不渲染顶栏，直接显示首屏主体；自定义首页分支保持既有容器行为。
- Hero 是全页主要区域，包含 Flow API、一个不超过两行的网关标题、一段短说明和全页唯一 CTA。
- 未登录 CTA 为“登录”并链接 `/sign-in`；已登录 CTA 为“进入控制台”并链接 `/dashboard`。
- 不内嵌或复制 `UserAuthForm`，认证、OAuth、Passkey、验证码、条款与 redirect 继续由现有 `/sign-in` 负责。
- 不渲染模型、协议、功能、客户端、统计、价格、排行榜、假终端、假 Dashboard、Logo 墙或二级营销 CTA。

### 3.2 Visual direction

设计参数：`DESIGN_VARIANCE: 6`、`MOTION_INTENSITY: 3`、`VISUAL_DENSITY: 2`。

- 视觉气质是克制、精确、成熟的开发者工具，不使用 AI 产品常见的紫色渐变大字、发光光球或卡片堆叠。
- 保留现有 Public Sans 与语义 token；页面以中性背景/墨色为主，品牌使用 cobalt 与 teal 两个实色强调，主要按钮使用现有 primary 语义色。
- 首屏使用稳定的 `min-height`、最大宽度和上下栏尺寸约束；常用桌面与移动视口都露出完整标题、说明和 CTA，不依赖按视口宽度缩放字体。
- 只允许轻微进入动效和品牌路径状态动效；尊重 `prefers-reduced-motion`，不能让动画改变布局尺寸。
- 焦点态、对比度、按钮点击区、SVG `title`/替代文本和键盘导航必须可用。

### 3.3 Brand asset

本轮默认沿用项目原有 Logo 资产和几何，将相邻产品文字改为 Flow API；不继续使用试制的自绘标志。认证页默认 Logo 和 favicon 使用同一资产，系统自定义 Logo 存在时继续优先使用管理员资产。验收覆盖浅色背景、16px favicon、28px 登录页和移动端，不允许裁切或视觉错位。
- 默认 `SystemBrand` / `DEFAULT_SYSTEM_NAME` / HTML title 和 favicon 回退统一为 Flow API。
- 只替换实际产品展示层的 New API 文案。上游渠道类型名、兼容协议描述、源码许可证、import path、存储 key 等技术语义不因 rebrand 被机械替换。

## 4. Unlimited user quota

### 4.1 Persistent contract and API

在 `model.User` 增加 `UnlimitedQuota bool`，JSON 为 `unlimited_quota`，数据库列为 `unlimited_quota`。零值 `false` 即业务默认，不添加会触发跨数据库重复迁移的 GORM 布尔默认 tag。

SQLite、MySQL 与 PostgreSQL 都通过现有 GORM AutoMigrate 增列。旧行读取为 false；不修改现有 `quota`，开关打开或关闭都保留账户原余额，便于关闭后恢复有限账户行为。

管理员沿用 `POST /api/user/manage`，新增明确动作：

```json
{
  "id": 2,
  "action": "set_unlimited_quota",
  "enabled": true
}
```

- `enabled` 使用 `*bool` 或等价的存在性校验，确保显式 `false` 不被当作缺失。
- 沿用当前 `canManageTargetRole` 与 admin route 鉴权，普通用户更新 DTO 不接收该字段。
- 数据库更新成功后同步发布用户缓存；失败时返回错误，不能出现 UI 成功但 Relay 仍读旧状态。
- 管理审计记录目标用户、旧值、新值和操作者上下文，不记录敏感凭据。

### 4.2 Cache and request context

字段传播链固定为：

```text
users.unlimited_quota
  -> User.ToBaseUser / Redis user hash
  -> UserBase.WriteContext
  -> ContextKeyUserUnlimitedQuota
  -> RelayInfo.UserUnlimitedQuota
  -> billing source selection
```

- `UserBase` 与 Redis Lua hash 写入新增字段，`userCacheSchemaVersion` 从 2 升到 3，使旧缓存强制回源重建。
- 无 Redis 时数据库路径返回相同字段；有 Redis 时管理更新不得只失效 quota 数值而遗漏布尔状态。
- `buildSelfUserData`、管理员列表、认证用户类型和钱包/概览查询需要返回该字段，供 UI 一致展示。

### 4.3 Unified billing source

新增 `BillingSourceUnlimited = "unlimited"` 与 `UnlimitedFunding`：

- `Source()` 返回 `unlimited`。
- `PreConsume`、`Settle`、`Refund` 都是无账户余额变更的成功操作。
- `NewBillingSession` 在读取钱包/订阅偏好前检查 `RelayInfo.UserUnlimitedQuota`，直接选择 `UnlimitedFunding`。
- Token 预扣、Token 补扣、Token 退款继续由 `BillingSession` 原流程执行；账户无限不改变 `TokenUnlimited`。
- `Reserve`、回滚、`NeedsRefund` 和低余额通知对 unlimited source 分支必须显式正确：账户资金无操作，有限 Token 仍可预扣/退还，不能落入 unsupported source。
- `relayInfo.BillingSource` 与日志 `other.billing_source` 记录 `unlimited`；费用估算、实际 quota、`used_quota`、request count 和渠道统计继续使用真实数值。

这比把 `preConsumedQuota` 强行改成 0 更安全：后者会破坏实际费用统计、Token 限额和异步任务对账。

### 4.4 Legacy and asynchronous billing paths

必须覆盖未完全走统一 `BillingSession` 的路径：

- Realtime/WSS：`PreWssConsumeQuota` 与后结算在 unlimited 时跳过账户余额校验/扣减，但保留 Token 与用量统计。
- legacy Midjourney handler：两处直接 `GetUserQuota` 的不足判断对 unlimited 放行，仍设置真实任务 quota 并更新统计。
- 通用异步 Task：`TaskPrivateData.BillingSource` 允许 `unlimited`；补扣和退款把它视为账户无操作。空值/未知历史值仍按现有 wallet 回退，避免旧任务升级后无法退款。
- legacy Midjourney model：新增 `billing_source` 持久化列并在准备、结算、退款中使用。历史空值继续视为 wallet；新 unlimited 任务失败时不得错误增加用户钱包余额。
- 钱包低余额邮件/通知仅对 wallet source 触发，unlimited 不发送。

所有路径都必须保持 `UpdateUserUsedQuotaAndRequestCount` 或等价统计调用，不以“没有扣余额”为由跳过日志。

### 4.5 Administration UI

在用户额度对话框增加二元 `Switch`：

- 开启时明确显示“无限额度”，禁用加/减/覆盖金额控件，提交 `set_unlimited_quota`。
- 确认成功后用提交值立即更新当前编辑抽屉，不等待旧 `currentRow` 对象刷新；抽屉显示 `∞`、“无限额度”和用量统计说明，隐藏有限额度输入与 `$0`。
- 关闭时保留当前 `quota` 数值并恢复现有原子调整控件；关闭动作不隐式充值或清零。
- 用户表格的额度单元格在无限时显示 `∞` 和“已用 …”，不显示有限额度进度条。
- 用户自助概览/钱包同样显示无限状态与累计用量；不能把 `quota=0` 格式化成欠费。

交互使用现有 Switch、Dialog、Badge/文本和 HugeIcons，不为普通操作手绘 SVG。

## 5. Compatibility and migration

### Preserved contracts

- `/v1/chat/completions`、其他标准 Relay 端点和 provider adaptor。
- 管理员自定义 `HomePageContent` 的 URL/HTML/Markdown覆盖。
- `theme.frontend` 非 `default` 值归一化到 `default` 的后端兼容逻辑。
- 历史 `checkins` 表、现有用户 quota 数值、旧任务 wallet fallback。
- Go module/import path、后端 package、数据库表名、Docker 镜像和源码许可头。

### Removed contracts

- 前端主题/布局/语言选择。
- 签到 HTTP API 与运行时设置。
- `/pg/chat/completions` 和 Chat/Playground 页面路由。
- 订阅页面、购买/管理 API、订阅额度重置任务和请求资金来源选择。
- Dashboard FAQ 面板及无消费者的 FAQ 配置页。
- 默认多段营销首页与 New API 默认展示品牌。

客户端若仍访问被删除的签到、Chat 或 Playground 路由，应得到标准 404/路由不存在，不提供长期 redirect 或隐藏兼容页。

## 6. Rollout and rollback

### Rollout order

1. 先落地数据字段、缓存 schema 和计费分支，并用测试证明有限用户不回归。
2. 再接入管理 API/UI 和自助显示，保证开关可观察、可关闭。
3. 再删除产品表面和设置，避免先删 UI 后无法验证底层无限账户。
4. 最后重写默认首页、品牌资产和固定外观，统一做浏览器视觉验收。

### Rollback points

- `unlimited_quota` 与 legacy `billing_source` 增列是向后兼容的 additive migration；回滚旧二进制时多余列可保留，不执行降级删列。
- 若无限计费出现异常，先通过管理员动作关闭用户开关，再回滚应用版本；原 quota 从未被覆盖，可立即恢复有限行为。
- 删除的路由和 UI 可通过代码回滚恢复；历史签到表和自定义首页数据未被删除。
- 品牌和首页回滚只影响静态前端产物，不改变 API 或数据库。

## 7. Main risks and mitigations

| Risk | Mitigation |
| --- | --- |
| 只改统一 BillingSession，legacy/异步路径仍拒绝或错误退款 | 为 Realtime、Task、legacy Midjourney 建独立回归测试，并搜索所有直接用户 quota 读写 |
| 无限用户绕过 Token 限额 | 资金源只旁路账户余额；Token 预扣/结算/退款测试分别覆盖有限与无限 Token |
| Redis 旧缓存导致开关延迟生效 | cache schema 升级、Lua hash 写字段、管理动作同步 publish，并测试 stale cache 回源 |
| 关闭无限后余额被清空或污染 | 开关不修改 `quota`；测试 true -> false 后恢复原余额校验 |
| 删除 Chat 误删标准 Chat Completions | `/pg` 缺失与 `/v1/chat/completions` 路由存在成对验证 |
| 删除签到破坏升级数据库 | 不执行 DROP；只移除运行时模型迁移与访问 |
| 主题按钮消失但旧 Cookie 仍改变 UI | 初始化清理全部已知 Cookie/data attributes，使用带旧 Cookie 的浏览器回归 |
| Rebrand 误改协议/上游类型/许可证 | 仅按用户可见位置逐项替换；禁止全仓机械替换 new-api/QuantumNous |
| 单屏页面在窄屏溢出或按钮重复 | 固定响应约束，唯一 Hero CTA，Playwright 桌面/移动截图与溢出检查 |
