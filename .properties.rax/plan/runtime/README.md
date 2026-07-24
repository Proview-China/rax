# Runtime Plan入口

## 当前状态

- 当前版本：通过独立文件复审并正在分切面落地的Runtime V1 Plan；
- 状态：OperationScope Evidence V3与Operation Settlement V4已完成实现、中央复验和最终Review YES；Generation-Binding Association additive只读Reader与Model Pre-Dispatch Assembly Current V1 neutral ports均已完成独立代码审计YES；其余切面按各自最新专项资产推进，未解锁production root或真实Provider；
- 旧版本：2026-07-14早期Plan草稿及两次文件复审修正前候选已由本候选完全替换，不再有效；
- 设计事实源：[Runtime设计入口](../../design/runtime/README.md)。

## 当前候选

- [Runtime Shared Engine Component Release V1（P4 owner-local完成；production NO-GO）](./component-release-v1.md)
- [Runtime V1执行治理核心计划](./runtime-v1.md)
- [OperationScope Evidence V3实施计划](./operation-scope-evidence-v3.md)
- [Operation Settlement V4实施计划](./operation-settlement-v4.md)
- [Operation Settlement V5窄Current Reader实施计划（第二次独立代码短审YES）](./operation-settlement-current-reader-v5.md)
- [Model Pre-Dispatch Assembly Current V1中立Port实施计划（Runtime实现与双独立代码审计YES；相邻Owner适配NO-GO）](./model-predispatch-assembly-current-v1.md)
- [Evidence Subject Current V1实施计划候选（R-CTY-06，asset-only二审YES）](./evidence-subject-current-v1.md)
- [G6A Action Matrix/Router实施计划](./g6a-action-matrix-router-v1.md)
- [Generation-Binding Association V1只读Port Delta（已实施）](../../design/runtime/generation-binding-association-v1/port-delta.md)

## 边界

已完成切面采用Go module，落点为`ExecutionRuntime/runtime`。数据库、协议、进程拓扑、生产Sandbox、SLA以及相邻组件真实实现仍未决定，未经后续确认不得引入。
