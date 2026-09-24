# 媒体与音频执行清单

1. [ ] 确定三类新渠道常量、动作表、端点和权限；父任务合入注册点。
2. [ ] MiniMax upload + 账号归属/ID 精度；供 H3 子任务联调。接着 clone/music/TTS 字幕及账务闭环。
3. [ ] VOD credentials/upload/sign helper；先独立验证签名和错误，再接 VOD/CV/ASR。
4. [ ] Viapi 同步/异步动作、VolcVOD/CV 查询、火山 TTS/ASR；填写每动作单位/使用量/结果合同。
5. [ ] 把渠道配置字段及前端展示需求交 pricing；父任务合入共享轮询/路由并验证所有 API 表面。
6. [ ] focused tests、负面边界、同账号和免费上传账单样例通过后交父任务统一验收。

运行变更包相关 `go test ./relay/channel/... ./controller/... ./middleware/... ./service/... ./model/...`；provider request/response 和签名 tests 使用确定性样例。新增持久化先做 SQLite/MySQL/PostgreSQL 兼容验证。真实供应商只能在父 P4 获准后做，不因上传免费就隐式开通/调用其他收费路径。

交付包括各端点请求/响应 fixture、计费参数表、模型/渠道草案输入和失败处理。价格、真实调用和生产状态由父任务验收矩阵继续追踪。
