# 实施与验证

1. 获得最终方案批准，加载 trellis-before-dev、前后端规范和 i18n skill；记录 dirty worktree 基线，配置子代理上下文后才 task.py start。
2. 实现后端 timing 合同与请求内尝试计时；覆盖控制器结果汇合、耗尽节点、成功日志提前快照及错误日志。不得挪动计费写入时序。
3. 实现共享时间线、可访问时间提示和七语言文案，保留两个入口及既有样式。
4. 定向测试：Go 使用固定时间输入验证相对时间、耗时、零值、重试重置、完成幂等和提前成功快照；控制器回归覆盖同渠道重试/跨渠道失败后成功/终止/同步图片。验证非管理员日志脱敏。
5. 前端 Vitest 验证两个入口、新旧数据、真实零值、非法数据、秒格式与提示。运行 go test ./service ./controller ./model；在 web 运行 bun run test -- src/features/usage-logs/components/__tests__/route-history-timeline.test.tsx 与受影响详情测试、bun run typecheck、bun run lint。测试失败区分既有基线与本次增量，不反复跑已通过的同一检查。
6. 使用 Playwright 或 Chrome DevTools 检查管理员两个入口、窄屏换行和普通用户不可见。无可用运行环境时明确记录未验证，不宣称通过。
7. trellis-check 子代理复核整体增量、成功日志时序、权限与既有改动保护；更新必要合同，汇报已完成/已验证/未验证。不自动推送或部署。

## 工作分工
按后端 -> 前端 -> 审查顺序委派，避免交叉修改已有脏文件。实现代理不得覆盖他人改动。主会话负责协调、任务状态与最终验收。
