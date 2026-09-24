# 定价与配置设计

以父矩阵为清单真相，每行增加目标：channel type/base/protocol、模型与上游映射、计价模式/单位、基价、数量来源、group/user/channel 比率、预期价格和证据。配置写入以 diff 合并，保护既有 channels/abilities/options；包含可回退值与启用状态。

当前两边 USDExchangeRate=1、展示 CNY，不做伪造汇率迁移。代码默认 quota unit 为 500000，但最终写入前验证实际环境。Keli video 组 1.2 只是源零售策略；Flow 有额外用户组/渠道 cost ratio，不应再把相同倍率乘两次。实际成本须按供应商价和充值折扣独立核实。

优先沿现有计费系统表达单位：按次/数量沿 ModelPrice，token/字符沿确认的 usage，阶梯使用当前 billingexpr 文档与版本，不引入第二表达式引擎。必要新增模式必须由实际 API 合同证明需要。非 token 业务不能把 token 模式的空 usage 解释成免费。

前端位于 `web/src/features/pricing`、`system-settings/models`、`channels`；Keli Classic 注册表仅作展示参考，不复制 JSX 目录。价格、管理员设置、消费日志单位共用项目既有数据源；raw channel cost 仅管理员可见。

管理表单包括新类型 100–102、所选 FastAI 类型、凭据字段说明、协议/端点覆盖、分组和映射。新 UI 文案使用英文源 flat keys，i18n skill 和 web/AGENTS 应用，根 AGENTS 冲突时优先。不要把端点/签名技术细节放到普通用户调用流程。

配置测试先本地；真实烟测若用生产，须经批准建立启用的隔离组/令牌，不混入普通 auto。禁用渠道只能验证不可被选，不能宣称通过正常网关调用。
