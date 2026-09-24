# 媒体与音频设计

新增 Viapi/VolcVOD/VolcCV provider package，编号候选为源 100/101/102，注册前检查目标冲突。按动作表分配计费单位，不把所有动作统一按次。签名算法/动作参数保留 provider 包内，固定时间/参数 fixture 可验证签名，无需真实账号。

MiniMax 扩展路由沿用现有 token auth/distributor/relay 生命周期。注意源 controller 虽获取 usage 却按次结算：目标先确定克隆/音乐的合同，不因 `usage_characters` 字段存在就自动改成字符价。文件上传须零价且不能走 token 兜底收费。

文件/音色是账号资源。先核对当前 affinity/任务凭据快照能否满足：上传选定账号须能在后续消费 ID 时解析并限制同账号；无法仅靠 token affinity 证明时，在相应资源记录/请求关联中保存最小归属信息。接口保持现有 file_id / voice_id 形式，禁止随机切换账号后继续发同一 ID。具体持久化与校验落父任务评审，三数据库兼容。

火山 VOD helper 供真正依赖的 ASR/CV/媒体任务复用；不把云凭据塞进前端、任务文档或日志。source helpers 迁移前检查包依赖、临时文件和上传大小/超时/SSRF；保留现有媒体限制。

本子任务拥有新 provider 包、minimax/volcengine 音频上传专用文件和 MiniMax extended controller；通用 router/distributor、共享 volcengine adaptor/constants 与模型设置由父任务/指定所有者整合。
