# Runtime模块说明

## 1. 作用

Runtime是Praxis全局执行治理基座。它把不可变`ResolvedAgentPlan`对应的实例执行转化为可控制、可Fence、可观察且失败结果真实的生命周期，不拥有模型、Context、Tool、Review、Memory或Sandbox的领域算法。

## 2. 当前组成

| 实现位置 | 作用 |
|---|---|
| `ExecutionRuntime/runtime/core` | 公共对象、三维状态、TypedError、epoch、Fence、Effect和恢复约束 |
| `ExecutionRuntime/runtime/control` | 命令Envelope、安全支配和持久化Run Fact合同 |
| `ExecutionRuntime/runtime/admission` | Activation Journal、Recovery Planner和Identity Lease原子激活合同 |
| `ExecutionRuntime/runtime/kernel` | Instance Aggregate、持久化Run Journal、Completion Claim证据摄取、revision线性化、迟到Observation隔离和显式Policy监督决策 |
| `ExecutionRuntime/runtime/ports` | Harness及所有相邻组件的版本化接入合同 |
| `ExecutionRuntime/runtime/releasecandidate` | reference-only Release/Readiness/Conformance与descriptor-only Factory；详见[说明](./component-release-v1.md) |
| `ExecutionRuntime/runtime/foundation` | 组件中立的最小Activate/Run/Checkpoint/Restore/Stop协调器 |
| `ExecutionRuntime/runtime/fakes` | 事实、Execution、Sandbox、Evidence和Checkpoint的确定性内存实现 |
| `ExecutionRuntime/runtime/tests` | 合同、非法状态、并发、容灾、Effect、Port与完整闭环反例 |

当前专项入口：[Binding V2](./binding-v2.md)、[Generation-Binding Association V1](./generation-binding-association-v1.md)、[Effect治理V2](./effect-governance-v2.md)、[Review Verdict V2](./review-v2.md)、[Evidence Ledger V2](./evidence-ledger-v2.md)、[Evidence Subject Current V1](./evidence-subject-current-v1.md)、[Run Settlement V2](./run-settlement-v2.md)、[Operation与Execution治理V3](./operation-governance-v3.md)、[Operation Settlement V4](./operation-settlement-v4.md)、[G6A Action Matrix/Router V1](./g6a-action-matrix-router-v1.md)、[Controlled Operation Provider V2](./controlled-operation-provider-v2.md)、[Checkpoint-first Governance V2](./checkpoint-first-governance-v2.md)、[Model Pre-Dispatch Assembly Current V1](./model-predispatch-assembly-current-v1.md)。

## 3. 输入与输出

当前切面接收ResolvedAgentPlan、版本化执行引用、EffectIntent、Fence、Checkpoint和Port描述；输出Ready/Run/Checkpoint/Restore/Termination结果、确定性RecoveryDecision、稳定TypedError和聚合快照。

当前只通过fake Port运行完整生命周期，不请求真实模型、不分配真实Sandbox，也不提交真实Memory或Artifact。

## 4. 依赖与被依赖

- 当前实现只依赖Go标准库；
- 不直接依赖现有`model-invoker`；
- 未来Runtime Host、Application Facade和组件Adapter依赖本模块公共合同；
- Context/Cache、Tool/MCP、审批链和Harness内部实现继续保持独立语义所有权。

## 5. 构建与验证

```bash
cd ExecutionRuntime/runtime
go test ./...
go test -race ./...
go vet ./...
```

2026-07-15公共合同、Activation容灾、Binding/Effect/Review/Evidence V2、Run Settlement V2、Operation/Execution治理V3及Application公共Port边界已实际通过普通测试、shuffle Race与Vet。

## 6. 当前限制

1. 已有Command/Desired State/Outbox、Identity Lease和Activation抽象事实合同及内存fake，但尚无生产持久后端；
2. Foundation已经跑通最小Binding与独立Ready检查，Kernel已有无默认SLA、Digest防漂移的监督Policy与纯决策；当前尚无生产调度器和Supervision事实后端，完整持续Reconcile仍按Runtime后续切面推进；
3. `RunLifecycleGovernancePortV3`已提供certified pending Run、独立Settlement、原子CompleteRun和终态Termination恢复；`RunClaimIngestGovernancePortV3`在历史认证preflight后完成Claim Evidence与Association精确持久关联，Claim仍不完成Run；
4. Review V2与Evidence V2已有Runtime侧Fact/Port/Gateway/Conformance，但Review领域组件、生产Verdict/Ledger后端仍未实现；当前Evidence各分区保守要求active Run，跨Fact Owner复读与Ledger append也不宣称生产原子事务；
5. `OperationGovernancePortV3`、Execution Delegation/Observation/Settlement公共入口已经冻结，但Application Step Journal与Harness Provider governed bridge仍需完成最终组合接线；
6. `ApplicationCommandFactPortV2`位于`runtime/ports`，Application只允许导入`runtime/core`和`runtime/ports`；`control`别名仅为legacy/restricted迁移；
7. 尚未与现有Model Invoker形成生产接线，也未决定数据库、RPC、进程拓扑、真实Sandbox、生产Checkpoint后端和SLA。
8. Operation Settlement V4已闭合Evidence V3 prepare/execute强类型关联、V3/V4共享terminal guard、历史四对象闭包与lost-reply/staged failure恢复；reference store与Conformance不声明生产持久化或SLA。
9. Checkpoint-first V2已提供Runtime-owned Attempt+Barrier、EffectCut、Finalization Closure、Consistency/Finalize、历史/当前终态Inspect、Evidence V1与Settlement V5 reference纵切；Restore、真实Participant/Continuity/Sandbox Adapter及生产后端仍保持unsupported。

## 7. G6A隔离切面

G6A Action matrix/router已完成Runtime最小实现与隔离fixture验证：只接受唯一Run内Tool Action矩阵，五维Owner-current Router无损路由，Provider前复读execute Enforcement、Evidence Handoff与Tool Owner Boundary current proof。当前仍无production composition root/backend/SLA，不启用Capability、Context Refresh、Continuation或Turn推进。

## 8. Controlled Operation Provider V2

Controlled Operation Provider V2已完成Runtime纵向切面与第三轮独立代码审计。它在真实Provider不可逆入口前复读Route、Prepared、execute Enforcement、Evidence、Boundary与七个Binding current闭包；以稳定Entry key线性化单次逻辑admission，并在lost reply或并发progressed state下只Inspect原Entry。当前仍只有reference store、fake transport与Conformance，无production composition root/backend、持久性、availability、SLA或物理exactly-once声明。

## 9. Checkpoint-first Governance V2

Checkpoint-first V2已通过第三次独立代码终审（P0/P1/P2=0）。Runtime唯一拥有Attempt/Barrier、EffectCut与终态Commit线性化；成功路径原子提交Consistency+closed Barrier，非成功路径原子提交Runtime派生终态+closed Barrier。Diagnostics/Residuals/ManifestSeal/Participant DomainResult仍归各语义Owner，Runtime只持typed ref并复读current。Evidence V1 Issue不推进cursor，Consumption复读真实Ledger Record，只有`consumed_current`可进入Settlement V5；EffectCut V2对缺exact Dispatch Attempt证明的V5 terminal永久fail closed。当前只提供reference fake与public Conformance，Provider调用数为0，Restore与production root/backend/SLA均未实现。

## 10. Generation-Binding Association只读面

`GenerationBindingAssociationCurrentReaderV1`已从既有Governance Port加法抽取并通过最终独立代码短审（P0/P1/P2=0）。只读消费者仅能Inspect Runtime权威Association Fact；Governance通过兼容嵌入保留原有Associate+Inspect方法集，既有对象、digest、Store和Gateway均未改变。该Reader不授Binding、Activation、Permit、Provider或production资格。

`OperationSettlementCurrentReaderV5`已完成最小Go实现与Owner门禁，并通过第二次独立代码短审YES（P0/P1/P2=0/0/0），详见[模块说明](./operation-settlement-current-reader-v5.md)。它只暴露current Inspect，Governance兼容嵌入后仍保持原六方法；Gateway对request Operation/Effect与returned Bundle执行exact交叉。该Reader不授Settle、Fact Owner或production backend/root资格。

## 12. Model Pre-Dispatch Assembly Current V1

Runtime neutral DTO、Registry exact Reader、Assembly Current Reader、Validate/canonical/digest、public Conformance与import-boundary测试已完成，并通过双独立代码审计YES（P0/P1/P2=0/0/0）及全门。Runtime只拥有Go nominal与只读合同；Harness仍唯一拥有publisher/CAS/current index和Assembly语义，Model/Harness/Tool适配与system/production composition root尚未解锁。详见[模块说明](./model-predispatch-assembly-current-v1.md)。
