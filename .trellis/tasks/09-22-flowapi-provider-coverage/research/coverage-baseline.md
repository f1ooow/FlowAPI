# 已核对的基线与局限

## 代码与线上观察

- FlowAPI 本地 `main` / `f3f7a8129`；只读观察线上 HK 使用 `flowapi:hk-passthrough-f3f7a8129-20260918`。Keli 本地 `/Users/Zhuyu/Documents/Code/new-api` 的 `feature/fulladaptor` / `9b80cd99c`，有既存未提交/未跟踪的 Wan 与其他修改；广州观察镜像 `new-api:frontend-fix-20260918`。工作树不等同于完整生产源码证明，后续按文件合同移植。
- 2026-09-22 15:28（UTC+8）读取两库的 channels/models 和允许的价格选项；随后从 Keli `settings` 列补查 video_protocol / task_endpoint_override。均为只读，无密钥输出到工件。
- Keli：20 渠道均启用、92 唯一模型 ID；Flow：37 渠道/34 启用、53 唯一 ID。缺 48 媒体 ID + 3 历史文本别名。模型/映射/源零售价详见 coverage-matrix.json，完整阅读表见 model-inventory.md。

## 源配置与目标差异

1. Flow `constant/channel.go:60` 已有 NewAPI=60，但 `relay/relay_adaptor.go:144` 的任务分派没有 60 或 Viapi/VOD/CV/FastAI。源 Keli 才有 100/101/102/103 四种额外类型。线上 50/51/52 为 100/101/102；FastAI 64 为 **60**，并无 103 渠道。不要把“仓库有适配器”写成“线上正在用”。
2. Keli `relay/relay_task.go:147` 取 platform→GetTaskAdaptor；本地标准任务路径未见将 60 重写为 103 的逻辑。仅靠清单不能证明线上 FastAI 走通；目标必须显式接专用任务协议，验证其实际端点。源专用包固定 `/v1/videos/generations` 与同族查询端点，模型列表只有四种 Seedance 上游 ID，未列 Wan。
3. HappyHorse Zivv 渠道 61：submit 覆盖 `/v1/services/aigc/video-generation/video-synthesis`；fetch `/api/v1/tasks/{task_id}`。缺端点设置适配会造成通用模型列表正确但请求失败。
4. Rolldek 75 为 OpenAI type=1，`settings.video_protocol=wan-rolldek-v1`。源路径在 Sora adaptor 加 Wan 分支；Flow `router/video-router.go`、Sora 包、task billing context 尚无该原生 Wan 能力。不可写成 Flow 已有 WanCreatePath。
5. Flow `service/task_polling.go:413` 只初始化 base/key，`:462` FetchTask 只传 task_id/action；源轮询还传 channel ID/settings/model。H3 按模型决定查询端点，Wan 按协议开关决定解析，VOD 需要渠道元数据；这是父任务的共享依赖。
6. Flow `model/task.go:116` 保留包括 EffectiveRatio 的完整倍率快照，Keli 对应结构不同。`service/task_billing.go`、`service/task_polling.go`、`controller/relay.go` 必须逐段整合，不能整体覆盖。
7. Flow 图片源码/渠道配置不等于已有 Seedream 新模型生产覆盖。53 原生 4.0/4.5/5.0 与 74 Pro 分别检查火山和 NewAPI/OpenAI 图像路径，保留正常图片与分层返回 usage 的不同语义。

## 价格事实（不是最终定价）

- 两边 `USDExchangeRate=1`，`general_setting.quota_display_type=CNY`。options 均未单列 QuotaPerUnit；代码默认 500000，发布前还须核对运行环境覆盖。此前“一边美元另一边人民币”的判断不成立。
- Keli `video=1.2`、`image=1`；Flow 没有 video 分组，且有渠道成本倍率/用户分组倍率。目标不能整表复制 GroupRatio，也不能用 Keli 零售价格充当已验证供应商成本。
- HappyHorse 1.1 同时有 ModelPrice=0.5、tiered_expr 模式和零表达式；仅证明源配置冲突风险，不能据此断言真实任务零扣费。需查任务价格分支与 mode 优先级并给目标明确方案。
- FastAI `wan-3.0`/`wan-3.0-time` 没有显式 ModelPrice/ModelRatio/表达式记录；本次目标配置明确排除这两个 FastAI Wan 别名，Wan 只通过 Rolldek 75 接入。
- MiniMax 扩展控制器源 `controller/relay_minimax_extended.go:60` 丢弃返回 usage，随后按次结算；voice_clone 代码收集 usage_characters 不等于实际按字符结算。TTS、克隆、音乐分别审计。
- ASR 渠道的映射含非本 ASR 模型条目；按模型白名单构建目标配置，不复制未知/无关映射。

## 检查边界

已做代码/配置阅读，未发真实供应商付费请求，未验证库存中所有模型当前可用；未修改 Keli、Flow 生产或产品代码。参考旧 skill/历史任务仅用于定位，实时事实以上述快照为准。`.trellis/spec/backend/quality.md`、frontend quality 中包含与当前 Go/Bun 项目不符的模板，不放入本任务上下文；使用根 AGENTS、web/AGENTS 和实际脚本。
