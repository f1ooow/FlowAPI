# 执行计划 — 生图工作台迁移

参照源：`/Users/Zhuyu/Documents/Code/gpt-image/deploy/repo/src/`
目标：`web/src/features/image-playground/`

原则：**先探路再批量搬**。样式层适配是唯一可能塌方的地方，先搬一个最复杂组件验证，再铺开。

---

## 阶段 0：探路（先做，别跳过）

- [ ] 0.1 在 `web/src/features/image-playground/` 下建临时验证分支/目录，搬入 `components/InputBar.tsx`(1947)，跑 `bun run dev`，确认 Tailwind v4 下样式基本成型。
- [ ] 0.2 写 `image-playground.css`：把参照项目 `tailwind.config.js` 的 8 组颜色迁成 `@theme`，加 `@custom-variant dark (@media (prefers-color-scheme: dark))`，覆写 `--color-gray-*` 为 zinc 值。
- [ ] 0.3 把参照 `index.css` 中工作台实际用到的规则**作用域化**到 `.image-playground-root`，删外部字体 `@import`。
- [ ] 0.4 若 0.2 的 gray remap 在 v4 下不成立 → 降级为全局文本替换 `gray-N` → `zinc-N`，记录到 research 产物。

**验证**：`bun run dev` 打开页面，InputBar 视觉正常（间距/圆角/暗色）。
**回滚点**：这一步的产物可整体丢弃，不影响既有代码。

---

## 阶段 1：搬运骨架与 API 层（无 UI 风险）

- [ ] 1.1 `bun add fflate`（唯一新增依赖）。
- [ ] 1.2 搬 `lib/` 中保留项：`imageApiShared.ts`、`serverSentEvents.ts`、`size.ts`、`mask.ts`、`maskPreprocess.ts`、`transparentImage.ts`、`imageCache.ts`、`dataUrl.ts`、`canvasImage.ts`、`downloadImages.ts`、`exportZip.ts`、`exportFileName.ts`、`clipboard.ts`、`dropdown.ts`、`domRect.ts`、`viewport.ts`、`viewportTransform.ts`、`taskState.ts`、`favoriteState.ts`、`persistedState.ts`、`inputDraftState.ts`、`promptImageMentions.ts`、`contentEditableMentions.ts`、`clickSuppression.ts`、`tooltipDismiss.ts`、`paramDisplay.tsx`、`taskPromptDisplay.ts`、`runtimeEnv.ts`、`api.ts`、`db.ts`。
- [ ] 1.3 搬 `hooks/`：`useCloseOnEscape`、`useDragSelect`、`useHintTooltip`、`usePreventBackgroundScroll`、`useTooltip`。
- [ ] 1.4 搬 `store.ts`（zustand），删 agent slice、settings slice。
- [ ] 1.5 搬 `types.ts`，删 Agent / settings / profile 管理相关类型，保留 `ApiProfile`。
- [ ] 1.6 搬 `components/Select.tsx` 等自研 UI 原语。

**验证**：`bun run typecheck` 通过（此时组件还没接上，允许有未使用告警，不允许有类型错误）。

---

## 阶段 2：改造 API 层 key 来源（核心，§5+§6）

- [ ] 2.1 重写 `apiProfiles.ts` → 单函数 `resolveImageProfile(tokenKey): ApiProfile`：`apiKey`=令牌 key，`baseUrl`=站内同源常量，`model`=`IMAGE_PLAYGROUND_MODEL`，`timeout`/`apiMode`=常量。删除多 profile 管理、localStorage 持久化、导入导出。
- [ ] 2.2 删 `falAiImageApi.ts`、`settingsCustomProvider.ts`、`presetConfig.ts`、`urlSettings.ts`、`profileImportUrl.ts`、`customProviderConfigUrl.ts` 及各自测试。
- [ ] 2.3 令牌选择器：基于 FlowAPI 现有令牌列表（`getApiKeys`）+ 保留 `group` 字段，做成受控组件。**复用站内已有实现**，不要重造。
- [ ] 2.4 校验 `openaiCompatibleImageApi.ts` 请求路径落在 `/v1/images/generations` 与 `/v1/images/edits`。

**验证**：选中一个真实令牌，请求能打到站内 relay 并在 usage log 里留下记录。
**回滚点**：此阶段结束前后端契约已固定，之后只在 UI 层推进。

---

## 阶段 3：删 settings / 顶栏 / Agent

- [ ] 3.1 删 `components/SettingsModal.tsx` + `components/settings/` 全部。
- [ ] 3.2 删 `components/Header.tsx`(348) + `components/HelpModal.tsx`；页面不再渲染自带 bar。
- [ ] 3.3 删 `components/AgentWorkspace.tsx`(1053) 及 `lib/agent*.ts` 一族 + 测试。
- [ ] 3.4 清 store / types / `paramCompatibility` 中 agent 与 settings 残留分支。
- [ ] 3.5 删 `useDockerApiUrlMigrationNotice.ts`、`useVersionCheck.ts`、`browserNotification.ts`（确认无引用后）。

**验证**：全局搜 `settings` / `Agent` / `apiProfile` 无残留引用；`bun run typecheck` + `bun run lint` 通过。

---

## 阶段 4：画廊 UI 搬运与接线

- [ ] 4.1 搬 `components/InputBar.tsx`、`TaskCard.tsx`、`TaskGrid.tsx`、`DetailModal.tsx`、`Lightbox.tsx`、`MaskEditorModal.tsx`、`SizePickerModal.tsx`、`HistoryModal.tsx`、`ImageContextMenu.tsx`、`icons.tsx`。
- [ ] 4.2 底部参数行最左加**模型下拉**（§7）：选项 `gpt-image-2` 单条，值进请求体 `model`。
- [ ] 4.3 接令牌选择器到 InputBar 附近（位置需在实现时定，倾向参数行内或输入区顶部）。
- [ ] 4.4 落地计费安全边界（§8）：`n` 上限校验、尺寸/质量/格式/透明/审核收敛为枚举、未知取值拒绝而非降级。
- [ ] 4.5 `index.tsx` 导出替换为新工作台；清理旧 `components/image-playground.tsx` 等 4079 行旧实现及测试。

**验证**：`bun run dev` 全流程走一遍——选令牌 → 填提示词 → 生成 → 结果展示 → 单图下载。

---

## 阶段 5：i18n 与收尾

- [ ] 5.1 抽文案 key，写入 `web/src/i18n/locales/zh.json`。**注意：该文件当前被另一 session 修改中（`git diff` 有 3 处改动），只追加不改动已有行。**
- [ ] 5.2 跑 `bun run i18n:sync`。
- [ ] 5.3 全量验证：`bun run lint` / `bun run typecheck` / `bun run test` / `bun run build:check`。
- [ ] 5.4 浏览器实测（`Skill(agent-browser)`）：截图确认顶栏五元素消失、settings 入口消失、模型下拉存在且唯一选项为 `gpt-image-2`、令牌选择器只列自己的令牌。

---

## 验证命令

```bash
cd web
bun run lint
bun run typecheck
bun run test
bun run build:check
```

## 风险与回滚点

| 风险 | 位置 | 处置 |
|---|---|---|
| Tailwind v4 样式塌方 | 阶段 0 | 探路先行；降级方案是 gray→zinc 文本替换 |
| 参照项目全局 CSS 污染站内页面 | 阶段 0.3 | 强制作用域化到 `.image-playground-root` |
| 删除 `ApiProfile` 会牵连 1000+ 行签名 | 阶段 2.1 | 保留类型，只换来源 |
| 计费参数未收敛导致异常倍率 | 阶段 4.4 | 枚举收敛 + 上界校验，验收必查 |
| zh.json 被其他 session 并发修改 | 阶段 5.1 | 只追加，不重排不格式 |

---

## 明确不做

- 不动后端 relay、`middleware/`、`model/`。
- 不新增按分组解析令牌的接口（已查实不可行且不必要）。
- 不保留旧工作台兼容层。
- 不引 `streamdown` / `react-markdown` / `katex` / `@fal-ai/client` / `wrangler`。
