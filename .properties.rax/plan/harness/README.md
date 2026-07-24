# Harness Plan入口

## 当前状态

- Harness公共合同与组件中立最小骨架已经实现并完成验收；
- 多租户/多Run分区、Event丢回包Inspect、模型Unknown安全停留、显式Binding V2身份和关闭后Cleanup观察已补齐；
- Application V3 governed Domain桥已闭合；legacy Intent/Fence路径仍只允许作为兼容测试基座，不可解读为生产dispatch；
- Harness Assembly P1/P2/P3a与Generation-Binding已有live资产；PendingAction Reader与Route第八独立短审均已过审。G6A Identity第二独立设计短审、Owner-current Delta及对应V3/V4实现、Harness P3 Assembler/InputCurrent Reader最终独立代码审计均为`YES(P0/P1/P2=0)`；Tool Consumer/P4与system fixture未实现，G6A/G6B/production root均保持`NO-GO`，`N>1`仍冻结；
- 单节点SQLite Session/Event、Assembly Publication、M2/A2 Current、Route/Owner Artifact和ACK durable backend已落地；它们不构成production root或SLA，M2独立Handoff current exact Reader/backend仍是明确残余；
- 首切面不选择具体生产Harness、进程协议、账号或真实模型；
- 设计事实源：[Harness设计入口](../../design/harness/README.md)。

## 当前计划

- [Harness Component Release V1（P4 owner-local候选完成；production NO-GO）](./component-release-v1.md)
- [Harness公共合同与最小可运行骨架v1（已完成）](./harness-v1.md)
- [Harness Governed V2接线（执行中）](./harness-governed-v2.md)
- [Harness Assembly公用接线V1（含单Call Action Gateway 4.1冻结线）](./harness-assembly-v1.md)
- [Controlled Operation Provider强类型Route V2](./controlled-operation-provider-route-v2.md)
- [Model PreDispatch Surface Commit Gate V1（M2与Harness concrete Gate Owner-local实现/测试YES；actual-point no-bypass与production root未闭合）](./model-predispatch-surface-commit-gate-v1.md)
- [单Call Action Gateway实施门与独占路径](./harness-assembly-v1.md#p7action-gatewaycheckpoint与per-turn-refresh)
- [单Call Action Gateway V1设计](../../design/harness/assembly/action-gateway-v1.md)
- [单Call Action Gateway V1测试矩阵](../../design/harness/assembly/action-gateway-v1-test-matrix.md)
- [G6B Context Turn Refresh公共合同漂移Port Delta（candidate-p0）](../../design/harness/port-deltas/context-turn-refresh-g6b-v1.md)
- [G6A Model Tool Call → PendingAction Identity V1（第二独立设计短审YES；Current Reader Port Delta开放）](./model-tool-call-pending-action-identity-v1.md)
- [Action Gateway前置：OperationScope Evidence V3计划](../runtime/operation-scope-evidence-v3.md)
- [Harness Route绑定与公共引擎接线v1（历史草案，已被Assembly V1取代）](./harness-route-engine-v1.md)
