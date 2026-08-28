# Current-state research

## Scope

本文件记录 2026-08-26 对当前工作树的只读调研结果，用于支撑 PRD 和后续技术设计。它不是实施批准。

## Fixed UI and Classic frontend

- `controller/option.go` 已拒绝 `theme.frontend != default`。
- `model/frontend_option_migration.go` 会把退役的前端选项归一化为 `default`。
- `web/src/components/config-drawer.tsx` 承载主题、颜色预设、字体、圆角、密度、侧栏、布局、内容宽度和方向的可见配置。
- `web/src/components/layout/components/app-header.tsx` 直接渲染 `ConfigDrawer` 与 `LanguageSwitcher`。
- `web/src/lib/theme-customization.ts` 当前默认 `contentLayout` 是 `full`，与截图中选中的 `centered` 不同。
- `web/src/context/layout-provider.tsx` 当前侧栏 variant 默认是 `inset`，与截图中选中的“侧边栏”不同。
- `web/src/context/theme-provider.tsx` 当前颜色模式默认跟随系统，与截图一致。

## Dashboard onboarding

- `web/src/features/dashboard/components/overview/overview-dashboard.tsx` 同时定义并渲染 `startSteps` 与 `quickActions`。
- 引导步骤包含创建 API 密钥、添加额度和通过 Playground 发送请求；右侧包含 API 密钥、渠道、使用日志和定价等推荐操作。

## Language

- `web/src/components/language-switcher.tsx` 提供多语言菜单。
- `web/src/i18n/index.ts` 初始化语言检测器和多个 locale。
- `web/src/features/profile/index.tsx` 渲染语言偏好卡片。
- `useTranslation()` 在前端被广泛使用；保留翻译抽象并固定 `zh` 比逐个重写所有组件更利于控制改动和后续合并。

## Homepage

- `web/src/features/home/index.tsx` 既支持自定义首页内容，也包含默认多区块营销首页。
- 默认首页由 `Hero`、`Stats`、`Features`、`HowItWorks`、`CTA` 和 `Footer` 六段组成，明显超过用户要求的一屏长度。
- 当前 Hero 包含渐变背景、网格、模型宣传、支持客户端、定价/文档操作和模拟终端；这些均与用户要求的单一网关说明冲突。
- `PublicHeader` 当前默认包含动态导航、语言、主题、通知和登录操作；单屏首页需要通过现有 props 收敛到品牌与唯一登录/控制台操作。
- 管理员可通过 `HomePageContent` 提供 Markdown、HTML 或外部 URL 覆盖默认首页。用户没有要求删除该兼容能力，因此计划仅替换空配置时的默认首页。
- `/sign-in` 使用独立 `AuthLayout` 和 `UserAuthForm`，并处理注册开关、redirect、OAuth、Passkey、验证码和条款。直接把表单复制到首页会形成认证入口重复；若要内嵌，应复用同一认证组件而不是复制逻辑。
- 用户最终确认首页不内嵌认证表单，只提供按钮。为避免顶栏和 Hero 重复同一操作，默认首页由 Hero 提供全页唯一 CTA：未登录进入 `/sign-in`，已登录进入 `/dashboard`；顶栏只保留品牌。
- 首页视觉技术栈已经具备 Tailwind v4、Base UI、Public Sans、语义主题变量和 reduced-motion 动画，无需新增 UI 系统或动画依赖。
- 设计定位为 `DESIGN_VARIANCE: 6`、`MOTION_INTENSITY: 3`、`VISUAL_DENSITY: 2`：适度不对称、仅状态与进入动效、低密度首屏。
- 用户明确允许自绘 SVG Logo。该许可适用于 Flow API 品牌标志，不改变常规操作图标继续使用现有 HugeIcons 的规则。

## Check-in

- 数据与逻辑：`model/checkin.go`、`controller/checkin.go`。
- API：`router/api-router.go` 中的签到状态和签到操作路由。
- 前端：个人页签到卡片、签到日历、请求封装、类型及系统设置中的签到配置区。
- `model/main.go` 参与签到模型迁移；升级路径不宜直接删除已有表。

## Chat and Playground

- `web/src/hooks/use-sidebar-data.ts` 与 `web/src/hooks/use-sidebar-config.ts` 生成聊天分组、Playground 和外部聊天客户端入口。
- 前端存在 `/playground`、`/chat/$chatId` 和 `/chat2link` 等路由。
- `router/relay-router.go` 与 `controller/playground.go` 提供专用 `/pg/chat/completions` Playground 入口。
- `setting/chat.go`、状态接口和用户默认侧栏配置仍包含 Chat 配置。
- 标准 `/v1/chat/completions` 走正常 Relay 链路，必须与 Playground 产品功能区分处理。

## Unlimited quota

- API Token 已有 `model.Token.UnlimitedQuota`，Token 管理界面也已有对应语义。
- 用户账户 `model.User` 当前只有 `Quota` 与 `UsedQuota`；用户缓存基础对象也没有账户无限字段。
- `service/billing_session.go` 的钱包资金源在预扣阶段读取用户额度并执行余额不足判断，结算阶段再扣减/退款。
- 实际用量日志与 `UsedQuota` 更新和钱包余额扣减并非同一字段，因此可以设计独立的无限账户资金源或显式旁路：不扣余额，但保留统计。
- 新字段需要进入数据库迁移、用户缓存 schema、管理员编辑 DTO、前端类型和表格展示。
- 账户无限与 Token 无限应保持两层独立控制，避免一个无限账户意外放开所有泄漏的有限 Token。
- `service/funding_source.go` 已把钱包与订阅抽象成 `FundingSource`；新增无余额变更的 `UnlimitedFunding` 比在每个正常计费分支散布条件判断更能保持预扣、结算与退款的一致性。
- `service/quota.go` 的 Realtime/WSS 计费、`relay/mjproxy_handler.go` 的 legacy Midjourney 检查，以及异步任务退款链路仍有绕过统一 `BillingSession` 的余额读写，需要显式纳入无限账户设计。
- 通用异步任务已经在 `TaskPrivateData.BillingSource` 持久化钱包/订阅来源；应新增 `unlimited` 并在结算、退款中作为无余额变更来源处理。旧记录的空值继续回退为钱包，保持历史兼容。
- legacy `Midjourney` 记录目前没有持久化账户资金来源。若无限用户的失败任务仍按钱包退款，会错误增加余额，因此该模型也需要新增可回退的 `billing_source` 字段。
- `model/user_cache.go` 当前 cache schema 是 `2`，Redis hash 写入由 `model/user_auth_cache.go` 的 Lua 脚本维护。无限字段必须进入 `UserBase`、上下文与 RelayInfo，同时升级 schema，防止旧缓存把无限账户当作有限账户。
- 管理员现有 `POST /api/user/manage` 已处理角色边界、审计和原子额度调整，适合新增独立 `set_unlimited_quota` 动作；不能依赖普通用户更新接口接收该权限字段。

## Branding governance

- 用户已于 2026-08-26 从根 `AGENTS.md` 删除 new-api 与 QuantumNous 品牌保护条款。
- 对根目录及子目录 `AGENTS.md` 的复查没有发现其他同等的禁止 rebrand 规则，Flow API 前端 rebrand 当前可以进入设计范围。
- 用户同时删除了原 `Project Governance` 下的 Pull Request 作者检查与模板要求；本任务不擅自恢复这些规则。
- 为控制风险和上游合并成本，品牌改造应限定在用户可见的前端名称与资产，不主动重命名 Go module、import path、数据库表或后端 package。
- 用户要求未来开源时重新加入 new-api 上游标识；这应作为独立的发布门禁记录，而不是当前内部 RC 的显示要求。
