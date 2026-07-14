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
| `ExecutionRuntime/runtime/foundation` | 组件中立的最小Activate/Run/Checkpoint/Restore/Stop协调器 |
| `ExecutionRuntime/runtime/fakes` | 事实、Execution、Sandbox、Evidence和Checkpoint的确定性内存实现 |
| `ExecutionRuntime/runtime/tests` | 合同、非法状态、并发、容灾、Effect、Port与完整闭环反例 |

当前专项入口：[Binding V2](./binding-v2.md)、[Effect治理V2](./effect-governance-v2.md)、[Review Verdict V2](./review-v2.md)、[Evidence Ledger V2](./evidence-ledger-v2.md)、[Run Settlement V2](./run-settlement-v2.md)、[Operation与Execution治理V3](./operation-governance-v3.md)。

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

## 7. 下一入口

下一入口是完成Application Step Journal、Command Outbox消费与Harness governed bridge组合门禁。只有组合闭环和允许导入/版本矩阵发布后才可解锁6+1；领域组件仍只能依赖`runtime/core`与`runtime/ports`。
