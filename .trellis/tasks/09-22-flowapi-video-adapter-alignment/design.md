# 视频设计

复用现有 `relay/channel/task/{ali,hailuo,doubao,sora}`；在 Sora 增加显式 Wan 协议分支和专用 Wan DTO/服务。仅在 FastAI 的协议不匹配已有任务适配器时采用源专用 package 的结构；当前证据支持专用 task 类型，不能给全部 NewAPI=60 强行加视频默认行为。

FastAI 源模型映射：Seedance 2.5/mini 为 token，2.0/fast 为 time；`wan-3.0` 映射到 `wan-3.0-time`。源 FastAI package 模型列表没有 Wan，因此要补真实协议合同，不把它当成现成实现。模型发现、映射、客户价格查找和上游名称必须各归其位。

Wan 请求接受前持久记录 user/token、请求键/摘要、选中渠道与凭据身份，成功后原子关联 task。上游已接受但 ID 未落库保持 unknown，通过已有可验证恢复协议查询；不推测 ID，不透明重试 paid POST。只复用必要的 Keli 恢复设计，适配 FlowAPI 的锁/JSON/三数据库规范。

TaskBillingContext 扩展 Wan 所需计费单位、时长和单价快照；保留 FlowAPI 的 effective ratio 及所有组件字段。输出时长验证失败的 source 行为和账务后果在 fixture 明确，不假装拿到了 authoritative usage。

所有权：本子任务写 video provider 包与新增 Wan 专用文件/测试；注册、路由、共享 DTO、Task/Relay controller 和 polling/billing 层由父任务串行整合。Keli 未提交 Wan 文件仅作只读参考，并记录取样摘要。
