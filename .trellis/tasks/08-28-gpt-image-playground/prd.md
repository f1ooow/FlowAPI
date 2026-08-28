# GPT Image Playground

## Goal

在 FlowAPI 用户端提供一个与现有产品整合的 GPT Image Playground，让已登录用户选择自己已经创建的 Token，直接通过当前网关完成图片生成和图片编辑，而不需要离开 FlowAPI 或手工拼装 API 请求。

该功能参考 `CookSleep/gpt_image_playground` 的工作流和交互思路，但不复制其实现、视觉或信息架构；最终体验必须延续 FlowAPI 现有设计系统和用户流程。

## Confirmed Facts

- 用户明确要求只使用 Image API：
  - `POST /v1/images/generations` 用于从提示词生成图片。
  - `POST /v1/images/edits` 用于基于输入图片进行编辑。
- 不使用 Responses API，也不设计基于 Responses 的多轮会话。
- Playground 中由用户选择其已经创建的 Token。
- 不预检查所选 Token 是否包含或允许 GPT Image 模型；用户选择后直接使用该 Token 发起调用，并展示网关返回结果。
- 不照搬 New API 原有 Playground，也不照搬参考仓库。
- OpenAI 官方文档确认 Image API 的 Generations 与 Edits 正是单次生图和单次改图的两个端点。
- 入口路径已确定为 `/playground`；它代表新的 GPT Image Playground，不恢复旧聊天 Playground 或 `/pg/chat/completions`。
- 用户已确认按参考项目保留浏览器本地历史画廊：任务图片和非敏感元数据在当前浏览器刷新后可恢复，但不新增服务端历史表，也不持久化真实 Token。

## Requirements

- Playground 必须是 FlowAPI 用户端的一项正式功能，并使用现有导航、页面布局、组件、主题、响应式和 i18n 约定。
- 用户必须能够从自己已有的 Token 中选择一个作为本次调用凭据。
- 生成模式必须能够提交 GPT Image generations 请求并展示返回图片。
- 编辑模式必须能够上传输入图片、提交 GPT Image edits 请求并展示返回图片。
- Token 模型权限、可用模型或渠道能力不在提交前校验；上游或网关拒绝时展示可操作的错误信息。
- 参数面板保持克制，只暴露真实用户生成/编辑任务需要的选项；不得仅因为参考项目存在某项设置就机械复制。
- 页面不得重复展示已经由选择状态、缩略图、按钮状态或结果本身清楚表达的信息。
- 图片请求、错误和结果必须沿用网关已有鉴权、计费、日志和 relay 行为，不建立绕开 FlowAPI 的第二套 OpenAI 代理。
- 首版保留参考项目的生成/编辑、结果画廊、详情预览、下载、删除和再次编辑闭环；使用固定的 GPT Image 模型和少量必要输出选项，不提供复杂设置页。
- 编辑模式至少支持上传一张本地输入图片；生成结果可作为下一次编辑的输入。
- 生成与编辑任务及图片在当前浏览器 IndexedDB 中持久化，刷新后恢复；正在执行的请求不要求跨刷新恢复。

## Acceptance Criteria

- [ ] 已登录用户可以从自己的 Token 列表中选择一个 Token，并用它发起 Playground 请求。
- [ ] 已登录用户从侧边栏进入 `/playground`，看到的是新的 GPT Image 工作台，而不是旧聊天 Playground。
- [ ] 文生图请求只调用 `/v1/images/generations`，成功后可以查看返回的全部图片。
- [ ] 图片编辑请求只调用 `/v1/images/edits`，至少支持上传一张输入图片并成功展示结果。
- [ ] Playground 不调用 `/v1/responses`，也不依赖 Responses API 会话状态。
- [ ] Playground 不会因为 Token 未声明 GPT Image 模型而阻止提交。
- [ ] 生成/编辑成功后任务和图片写入浏览器本地历史；刷新 `/playground` 后仍可打开详情、下载、删除和再次编辑。
- [ ] 再次编辑会以新任务保存，不修改原任务；清除站点数据后历史消失但不影响网关服务端日志。
- [ ] 网关返回的验证、鉴权、余额、渠道、限流、内容安全及服务端错误能以用户可理解的方式呈现。
- [ ] 桌面和窄屏布局都能完成 Token 选择、模式切换、输入、提交、等待、查看结果和失败重试。
- [ ] 新增的用户可见文案覆盖项目当前启用的前端 locale；当前运行时由精简任务固定为简体中文。
- [ ] 实现通过相关后端测试、前端 lint/type-check/build 和真实浏览器主流程验证。

## Out of Scope

- Responses API 及其多轮图片会话能力。
- 提交前探测 Token 的 GPT Image 模型权限或渠道可用性。
- 复制参考项目的源代码、品牌或页面视觉。
- Agent 模式、流式中间图片、多供应商配置、复杂收藏夹/批量 ZIP 管理。
- 遮罩绘制器、多轮 Responses 会话、Web 搜索、供应商自定义 HTTP 配置和服务端图片历史同步。

## Technical Notes From Repository Research

- 当前旧 Playground 正由 `.trellis/tasks/08-26-internal-flow-api-simplification` 任务删除；该任务同时删除 `/playground` 路由、旧 feature 目录和 `/pg/chat/completions` 专用入口，但保留标准 `/v1/images/generations` 与 `/v1/images/edits` Relay。新功能必须作为独立实现与该清理任务衔接，不能恢复旧 `/pg` 协议。
- 前端已有 `getApiKeys` 分页查询本人 Token、`fetchTokenKey` 按 Token ID 取真实 Key 的受保护接口；列表响应只返回掩码 Key。选择 Token 时可以按需解析真实 Key，真实 Key 只保留在当前页面内存中，不写入 URL 或持久化设置。
- Image Relay 已支持 JSON generations 与 multipart edits，包含模型映射、渠道选择、计费、日志和错误转换；Playground 不需要新增后端代理或旁路计费接口。
- 前端使用 TanStack Router、React Query、Base UI/shadcn Base 组件、Tailwind v4、Public Sans 与 Hugeicons；页面应复用 `SectionPageLayout`、`Combobox`、`Select`、`Textarea`、`Input`、`Button`、`Skeleton`、`Dialog/Drawer` 等现有组件。
- 参考项目的有效信息架构是“结果画廊 + 底部/侧边请求编排区 + 结果详情/再次编辑”，但 Flow API 版本应以现有控制台浅色语义 token 和工作台布局重新表达，不照搬其黑色三列卡片样式。
- 入口路径已确定为 `/playground`；它代表新的 GPT Image Playground，不恢复旧聊天 Playground 或 `/pg/chat/completions`。
- 首版固定使用 `gpt-image-2` 作为请求模型；不做模型下拉框或提交前能力探测。

## Open Questions

- 无。

## Notes

- 本任务属于复杂跨层功能，`design.md` 与 `implement.md` 已完成；必须在用户批准最终计划后再执行 `task.py start`。
