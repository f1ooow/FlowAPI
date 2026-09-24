# Seedream 设计

在 FlowAPI 现有 volcengine/image 与 OpenAI/NewAPI image 响应路径扩展，不拷贝 Keli Classic UI。原生三版本和 Pro 中转分开给出请求合同；不假定所有参数在四个模型上通用。

Keli 分层分支位于 `relay/channel/openai/relay_image.go`，以 GeneratedImages × 0.5 替换 model-price 数量因子。移植时限定模型/模式，并保留 FlowAPI `AddOtherRatio`、数量上界和 checked quota 工具；不能照搬“上游数量天然安全”的注释作为验证。

按张系数、实际返回数量和预扣上限进入账务样例；mode 为表达式时使用现有表达式来源，不能在另一分支再乘一次 n。缺失/无效 usage 的处理必须与明确错误/可审计账务结果关联，不能把不明数量当 0 价成功。0.5 是源参考系数，目标正式价格审阅时同时确认。

拥有本次图片专用转换和分层计费修改，共享 volcengine 入口交父任务整合。现有 GPT/Gemini 分辨率表达式和按实际 output-token 的其他讨论不纳入本次新实现。
