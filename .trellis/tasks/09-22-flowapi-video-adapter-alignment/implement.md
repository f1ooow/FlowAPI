# 视频执行清单

1. [ ] 接收父 P0 接口；建立原生/中转端点、模型映射、计费单位和 payload fixtures。
2. [ ] HappyHorse 1.1 与 61 覆盖路径；保留 FlowAPI 现有 duration clamp。
3. [ ] Seedance 四版本与 Hailuo 档位；H3 接入 media 上传合同后完成账号绑定和查询端点。
4. [ ] FastAI 专用类型/适配器、time/token 账务分支、Wan 别名协议；向父任务提交注册修改需求。
5. [ ] Sora Wan 分支、原生路由/DTO、时长 probing、TaskSubmission 持久恢复。父任务合入共享文件后做断连、重启、隔离和幂等用例。
6. [ ] 交付逐模型协议/价格需求及 focused tests；pricing 复核样例账单；父任务运行联合回归。

计划命令：`go test ./relay/channel/task/... ./relay/... ./service/... ./model/... ./middleware/... ./controller/...`；新增迁移需在 SQLite/MySQL/PostgreSQL 验证创建/重启/旧任务读取。触及 relaykit 的合同由父任务执行 `GOWORK=off go build ./...`。现有 Sora 和旧视频为必选回归；真实调用不在本地测试中夹带。

关闭条件：上述代码/fixture/账务合同通过且共享改动已审入；真实供应商和部署状态仍在父任务矩阵独立记录，不能据此宣称线上已可用。
