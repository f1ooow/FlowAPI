# Seedream 图片与图层拆分

## 目标与范围

落实父任务 R4：原生 `doubao-seedream-4-0-250828`、`doubao-seedream-4-5-251128`、`doubao-seedream-5-0-260128`，以及 Zivv NewAPI 兼容 `doubao-seedream-5-0-pro-260628`。覆盖生成、多参考图/编辑中供应商实际支持的端点、输出图片/usage、Pro 图层拆分。

## 依赖

- 父 P0 的路由/配置合同；与 media 共享 volcengine adaptor/constants 的修改由父任务串行整合。
- pricing 提供目标零售价/图层系数及价格页合同；本任务提供数量/usage/模式优先级样例。
- 现有 `09-13-image-resolution-pricing` 只作为 GPT/Gemini 回归边界，不覆盖其规则或任务状态。

## 验收

- [ ] 四个 ID 均有模型注册、原生/兼容协议、参数透传与响应 fixture；供应商支持面明确。
- [ ] Pro 不发送供应商不支持的 sequential 参数；未传、显式 false/0 的可选字段保持各自语义。
- [ ] 分层限定在受支持模型与布尔 true；实际张数 1/17 的计费样例明确，缺失、零、异常大 usage 有可审计处理；预扣覆盖允许最大输出、结算退差。
- [ ] 普通生图仍沿原正常数量合同，layer=false/缺失/错误类型不误触分层；不把层数、n 与表达式再叠加扣费。
- [ ] 现有 GPT/Gemini 图像 JSON/multipart 价格与常规 Seedream 无回归；结果 URL/base64 与 usage 解析正确。

不改 KeliAPI、不切产品流量；真实出图、商业价格和上线属于父任务后续门槛。
