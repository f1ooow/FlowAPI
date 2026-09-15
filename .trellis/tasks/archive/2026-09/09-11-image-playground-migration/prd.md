# 生图工作台迁移至 gpt_image_playground

## Goal

把 FlowAPI 现有的「生图工作台」整体替换为 [gpt_image_playground](https://github.com/CookSleep/gpt_image_playground) 的界面与交互，去掉其中的设置（settings）功能与 Agent 模式，并把生图所用凭据改为「用户选择自己的令牌」—— 令牌自带分组，选中令牌即确定分组，用该令牌的 key 发起站内生图请求。

用户价值：得到一套更完整的生图工作台交互（参数面板、结果画廊、历史、遮罩编辑等），同时不需要用户手填/粘贴 API key，直接复用站内令牌与分组授权体系。

## 背景与已确认事实（2026-09-11）

- 参照代码在本地已具备，路径为 `/Users/Zhuyu/Documents/Code/gpt-image/deploy/repo`（v0.7.5，源码在 `deploy/repo/src/`，所以按项目名搜不到）。另有 `~/Documents/Codex/2026-06-25/bh/work/gpt_image_playground-v0.6.10`，缺 Agent 模式 20+ 文件，不采用。无需 clone 上游。
- 现有工作台 `web/src/features/image-playground/` 共 4079 行，**整体替换**，不保留旧界面或兼容层。
- 站内 `/v1/images/*` relay 不读请求中的 `group` 字段，生效分组只来自令牌行自身的 `Token.Group`（`middleware/auth.go:459-477`）。唯一的分组覆盖分支（`middleware/distributor.go:88-104`）挂在 `/pg/chat/completions`，该路由未在 `router/` 注册，是死分支。
- 令牌列表接口本就返回 `group` 字段（`model.Token.Group`），现有工作台在 `web/src/features/playground/lib/tokens.ts:35-41` 与 `types.ts:34-39` 将其丢弃。
- 参照项目的 `apiKey` / `baseUrl` / `model` 同出于 `getActiveApiProfile(settings)`（`lib/apiProfiles.ts:785`）。

## 需求

- R1：FlowAPI 前端生图工作台整体换成 gpt_image_playground 的实现，走 FlowAPI 既有前端技术栈与路由。
- R2：移除 gpt_image_playground 的 settings 功能，包括相关 UI 入口、状态、持久化与类型；不保留死代码。原 settings 承载的参数（API key、base URL、模型、代理等）一律由站内决定，用户不可设置。
- R3：生图请求的 key 来自用户自己的令牌：用户只能选择自己的令牌，key 本身不出现在 UI，也不允许手填。
- R4：界面文案走 i18n（`web/src/i18n/locales/zh.json`），沿用项目现有约定。
- R5：生图请求走站内 relay/既定接口，纳入既有计费与日志链路。
- R6：删除原项目顶部栏的全部内容：品牌名 + `NEW` 角标、`画廊 / Agent` 模式切换、下载图标、帮助（?）图标、设置齿轮。只保留画廊（Gallery）模式，不提供 Agent 模式、不做模式切换；Agent 模式相关代码、状态、类型一并删除。
- R7：FlowAPI 本站自己的导航/页头不受影响，生图工作台作为站内一个普通页面嵌入。
- R8：在底部参数行新增「模型」选择项（排在最前，尺寸/质量/格式/透明背景/审核/数量之前）。当前只提供 `gpt-image-2` 一个选项，即下拉存在但只有单项可选；其余模型留待后续放开，本任务不接。
- R9：底部参数行的取值必须收敛为站内模型实际支持的枚举，数量 `n` 受上界约束，未知取值在请求构造处拒绝而非静默降级（依据 `AGENTS.md` billing invariants，详见 design.md §8）。

## 验收标准

- [ ] 生图工作台页面可打开，界面为 gpt_image_playground 的布局与交互，无旧工作台残留组件。
- [ ] 页面内不存在 settings 相关入口、弹窗、localStorage/后端配置项；用户无法设置 API key、base URL、模型或代理。
- [ ] 页面内不存在品牌名/`NEW` 角标、`画廊/Agent` 切换、下载图标、帮助图标；无 Agent 模式代码路径。
- [ ] 令牌选择器只列出当前用户自己的令牌；选中后生图请求使用该令牌，不再要求用户输入 key。
- [ ] 底部参数行最左侧有「模型」下拉，当前唯一可选值为 `gpt-image-2`；选择结果进入生图请求的 model 字段。
- [ ] 尺寸/质量/格式/透明背景/审核均为枚举选择，数量受上界约束；构造请求时对未知取值报错。
- [ ] 生图成功返回并在界面展示结果；失败时有可溯源的错误提示。
- [ ] 前端 `lint` / `typecheck` / `test` / `build:check` 通过。

## 范围外

- 不保留旧生图工作台界面或兼容层。
- 不修改后端 relay、`middleware/`、`model/`，不新增按分组解析令牌的接口。
- 不引入 gpt_image_playground 的服务端组件、Cloudflare/Wrangler 部署产物或 fal.ai 渠道。
- 不引入 `streamdown` / `react-markdown` / `remark-gfm` / `katex`（均只服务 Agent 模式的 markdown 渲染）。
- 不开放 `gpt-image-2` 之外的模型选择。

## 关联产物

- `design.md` — 技术设计、技术栈差异与适配面、删除清单、凭据来源改造、计费边界。
- `implement.md` — 分阶段执行计划与验证命令。
- `research/gpt-image-playground.md`（完整）与 `research/gpt-image-playground-digest.md`（注入用摘要）— 参照项目调研。
- `research/flowapi-image-workbench-and-token-groups.md` — 站内现状与令牌分组机制调研。
