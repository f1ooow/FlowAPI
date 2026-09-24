# 跨任务契约与验收矩阵

本文件吸收只读复核结果，并由主会话纠正来源路径混淆：100–103/FastAI/Wan 原生路由属于 Keli，当前 FlowAPI 并未具备。代码路径以 coverage-baseline.md 为准。

| 合同 | 必测行为 | 所有者 |
|---|---|---|
| 提交与查询一致 | 两个渠道/同模型，修改优先级或价格后仍用原账号、原上游 ID/模型/协议；旧任务可读 | 父任务 + video/media |
| 倍率快照 | base=2、user=.8、channel=1.5，包含渠道时 2.4，否则 1.6；提交后改配置不改结算；退款用同一快照 | 父任务 + pricing |
| 端点覆盖 | 空覆盖保持默认；Zivv 61 精确路径；拒绝外域、路径穿越、无效占位符 | 父任务 + video |
| Wan 恢复 | opt-in、同键同 payload 返回原关联、改 payload 409、跨用户/令牌不可查；发送前 reserve；未知接受不重发；重启后继续恢复 | video + 父任务 |
| Wan 实际秒数 | 区分 requested 与 output 秒数；视频轨而非音轨尾长；异常时长/下载失败不能标成已核验 usage；失败与补扣/退款幂等 | video + pricing |
| FastAI 映射 | OriginModelName 定客户价，UpstreamModelName 定协议；Seedance time 与 token 分别结算，不能同时乘秒数又按完整 usage 扣费；Wan 不进入 FastAI 映射，统一走 Rolldek | video + pricing |
| 文件与声音账号 | 大 int64 file_id 输出字符串；上传→克隆→合成/视频沿同账号；多 key 轮转/换渠道时不给另一账号 ID；上传零扣费不落 token 默认价 | media + video |
| 音频/上传安全 | multipart/长文本/音频头的数量上界；返回音频字节和内容类型、字幕、错误结构符合客户端合同；签名用固定时间样例 | media |
| 图片 | 正常生图、编辑/多参考图按支持端点测试；Pro 不发送未支持字段；JSON/multipart 在实际支持路径保留显式 0/false、使用现有图片数量上界 | images |
| 图层拆分 | 仅确认模型的布尔 true 生效，按真实张数 × 0.5 的目标定价方案；1/17 张边界、缺失/0/过大 usage 有确定行为；不改普通图片，不与表达式重复计量 | images + pricing |
| 账务边界 | wallet/subscription、预扣不足、超界返回 400、非有限值/溢出、饱和标记、重试前增额预留；重复 poll 不重扣不重退 | 父任务 + 各子任务 |
| 渠道可用性 | 禁用不参与模型/组普通路由；测试启用组确实选目标渠道；普通组无泄漏；管理员成本字段不向普通用户暴露 | pricing + 父任务 |
| 结果与界面 | 结果 URL/代理实际取回媒体；价格表、配置页、用量日志的单位与扣费一致；七语言及渠道类型 UI | pricing |
| 数据兼容 | SQLite/MySQL/PostgreSQL 新表和字段迁移可执行、重启幂等；旧 task 快照兼容；relaykit 无 root-module 依赖 | 父任务 |

数据型测试使用明确输入和精确输出，不为增加覆盖率造压力循环。后端新增/大改测试使用 testify require/assert；不无谓复制所有上游测试。供应商真实样例、mock fixtures 和浏览器验证各有单独证据列。
