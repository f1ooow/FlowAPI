# Align GPT Image Playground with reference workflow

## Goal

把已部署的 Flow API 生图工作台从“简化表单”重做为接近 `CookSleep/gpt_image_playground` 核心工作流的统一生图画廊，同时把公开首页恢复为 New API 原生营销页的既有语境，只删除多余内容并收敛到首屏，不另行设计一套新首页。

## Confirmed Product Decisions

- 生图工作台入口和页面标题统一使用中文“生图工作台”，不显示英文 `GPT Image Playground` 作为入口或标题。
- 用户不需要选择“生成模式/编辑模式”。是否调用 generations 或 edits 由是否存在参考图片自动决定：无参考图走 generations，有参考图走 edits。
- 生成请求提交后立即在画廊新增一张“生成中”任务卡；参数区和画廊同时可见，不能让结果区空等到请求结束。
- 生成完成后只保留任务卡片，不自动打开详情弹层；用户点击图片或任务卡片后再查看详情。
- 任务卡片需要覆盖成功、生成中、失败三种状态，并展示图片、提示词、参数标签、耗时/时间和操作栏。
- 点击任务卡或图片进入详情后，用户可查看提示词、实际参数、复用配置、再次编辑、删除和下载；详情不是请求完成后的自动动作。
- 参考图片可以通过上传、拖拽和剪贴板粘贴添加；生成结果可以作为下一次编辑素材。
- 页面继续使用左侧输入/参数区、右侧结果画廊的左右布局；只借鉴参考项目的浅色视觉层次和信息密度，不复制其底部悬浮输入条或整体页面结构。
- 桌面端图片右键菜单至少支持复制图片、下载和编辑/加入参考图；复制后的图片可以直接粘贴回生图输入区。
- 参数区的参考图片缩略图可预览；预览中提供“编辑图片”，进入独立遮罩编辑器。
- 遮罩编辑器按官方图片编辑约束自动准备主图：必要时转换 PNG、限制最大边长并将尺寸规整为 16 的倍数；保存后以主图 + mask 调用 edits。
- 生图功能继续使用用户已创建的 Token；不提交前探测该 Token 是否拥有 GPT Image 模型，也不静默换 Key。
- 继续只使用 Image API 的 `/v1/images/generations` 与 `/v1/images/edits`，不使用 Responses API。
- 本地任务和图片历史继续保存在浏览器 IndexedDB；真实 API Key 不进入 URL、localStorage、IndexedDB 或任务详情。
- 首页恢复 New API 原生营销页的既有 Hero/Terminal 视觉和文案语境；只保留一屏内的核心内容，删除 Stats、Features、HowItWorks、CTA、长 Footer、额外产品说明和重复导航，不新增自定义视觉概念。
- 管理员配置的自定义首页 URL/HTML/Markdown 覆盖能力保持不变；仅空配置时使用精简后的原生默认首页。

## Requirements

- 生图工作台必须复用现有 Flow API 控制台布局、浅色主题、Public Sans、Hugeicons/Base UI 和 i18n 约定。
- 任务状态在请求发起时就写入 React 状态并渲染卡片；取消、失败和成功均有稳定可见反馈。
- 请求编排区保持可用，不因为生成中的任务卡或详情状态而被替换或隐藏。
- 编辑请求的参考图顺序、mask 主图和 multipart 字段必须稳定，支持官方 edits relay 的现有契约。
- 任务卡操作不能因卡片外层点击而误触发；嵌套按钮必须阻止事件冒泡并具备可访问名称。
- 复制图片使用浏览器 Clipboard API，失败时给出明确反馈；在不支持图片剪贴板的环境中不伪造成功。
- IndexedDB 不可用时降级到当前会话内存状态，且成功的上游结果不能因为本地保存失败而丢失。
- 首页默认内容不渲染新的“内部团队”宣传语，不改变 New API 原生营销页的视觉语境；只做区块删除和首屏布局收敛。

## Acceptance Criteria

- [ ] 侧边栏入口和页面标题均显示“生图工作台”，不显示 `GPT Image Playground`。
- [ ] 用户不需要在页面上选择生成/编辑模式；无参考图提交 generations，有参考图提交 edits。
- [ ] 点击提交后，画廊立即出现生成中任务卡，参数区保持可用；请求完成后同一卡片更新为成功或失败。
- [ ] 成功结果不会自动弹出详情；点击卡片或图片才打开详情。
- [ ] 任务卡能展示提示词、参数标签、输出数量、耗时/时间，并提供复用配置、再次编辑、删除和下载操作。
- [ ] 任务详情能查看全部输出图片和实际参数；多图请求不会只显示第一张。
- [ ] 参考图片支持上传、拖拽和粘贴；生成结果可以通过编辑操作回填为参考图片。
- [ ] 图片右键菜单支持复制、下载和编辑；复制的图片可粘贴回输入区。
- [ ] 点击参数区参考图预览可以进入遮罩编辑器；编辑器保存时对主图执行官方尺寸准备并提交 mask。
- [ ] 遮罩主图和 mask 尺寸一致、输出为 PNG、最大边长和 16 倍数约束得到单元测试保护。
- [ ] 生图请求只访问 `/v1/images/generations` 或 `/v1/images/edits`，不会访问 `/v1/responses`。
- [ ] 所选 Token 通过现有用户归属接口解析并只在内存中使用；历史记录和 URL 中不存在真实 Key。
- [ ] 桌面和窄屏布局都能完成输入、粘贴/上传、生成中观察、详情、再次编辑和删除，且无横向溢出。
- [ ] 默认首页恢复 New API 原生 Hero/Terminal 语境，只保留首屏内容；Stats、Features、HowItWorks、CTA、长 Footer 和“内部团队”文案不渲染。
- [ ] 管理员自定义首页内容的 URL/HTML/Markdown 分支行为不变。
- [ ] 新增/修改的用户可见文案覆盖当前运行时启用的简体中文 locale。
- [ ] 通过受影响的 Vitest、前端 typecheck/lint/build、Go 图像 relay 测试和真实浏览器桌面/移动流程验证。

## Out of Scope

- Agent 模式、Responses API、多供应商配置、Web 搜索、复杂收藏夹/批量 ZIP、服务端图片历史同步。
- 除遮罩编辑必要的官方尺寸准备外，不扩展高级画笔、图层或复杂图像处理套件。
- 不重写 New API 首页视觉，不制作新的品牌插画、渐变背景或产品设计系统。
- 不恢复旧聊天 Playground、`/pg/chat/completions` 或旧 Chat 导航。

## Technical Notes From Repository Research

- 当前实现位于 `web/src/features/image-playground/`，内部仍有 `mode: generate|edit`、成功后自动 `selectedTaskId`、只展示第一张图、仅支持 file input 和一个简化详情弹层；本修订要改的是交互模型和状态生命周期，不是恢复旧聊天 feature。
- 参考项目的任务卡在请求开始时创建 `running` 记录，`TaskCard` 通过 `status` 渲染 loading/error/done；`DetailModal`、`ImageContextMenu`、`MaskEditorModal` 和 `maskPreprocess` 分别覆盖详情操作、右键复制/下载/编辑、遮罩绘制和官方尺寸准备。用户提供的浅色截图是本次 UI 适配的视觉基准：可以沿用其画廊卡片、标签、图标操作、底部悬浮输入区和浅色层次，不应因为 README 中存在 OLED 深色截图而改成深色主题。
- 现有 Flow API Image Relay 已支持 generations JSON、edits multipart、`dto.ImageRequest`、mask 和图像计费，不需要新增后端接口；编辑请求必须保持现有 `/v1/images/edits` 路由和 multipart 解析契约。
- 现有前端运行时已由精简任务固定使用简体中文 `zh.json`；新增文案只需通过项目 i18n 脚本写入当前启用 locale。
- 当前默认首页在 `web/src/features/home/index.tsx` 中是自定义“Internal AI API infrastructure”单屏；上游/历史版本的原生营销页由 `Hero` + `HeroTerminalDemo` 等组件组成。本次只恢复原生首屏语境并删除多余 section，不恢复全部长营销页。

## Open Questions

- 无。首版推荐保留一个主参考图编辑入口和最多 16 张参考图上限，复用当前 Image Relay/前端图片处理约束；不把遮罩编辑做成独立产品模式。
