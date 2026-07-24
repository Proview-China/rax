# Praxis Runtime

本目录是Praxis全局执行治理基座，不是`model-invoker/execution.Runtime`的重命名或迁移。

## 当前交付

当前已经完成公共合同与最小可运行基座：

- Runtime公共版本号与独立Go module；
- Identity、Lineage、Instance、Lease、Run、Effect与唯一所有者引用；
- `LifecyclePhase + ExecutionCertainty + CleanupStatus`三维状态及迁移验证；
- epoch/revision前置条件、TypedError和迟到Observation隔离；
- ExecutionFence、显式离线撤销策略、ReplacementPermit与受限恢复Effect白名单；
- Command Envelope和安全命令支配关系；
- Command、Desired State与Outbox的线性化事实合同和原子内存fake；
- IdentityExecutionLease单持有者、epoch、reserved/active/renew/revoke/release/expiry事实合同和内存fake；一般执行权只由Activation原子提交授予；
- 持久化Activation Journal、确定性Recovery Planner和`ActivationFactPort.CommitActivation`原子合同；
- 线程安全、revision线性化的纯Runtime Instance Aggregate；
- Runtime所有的持久化`RunFactPort`与`RunJournal`：单活跃Run原子门禁、revision CAS、回包丢失后Inspect恢复及冲突终态拒绝；
- `RunClaimGateway`：按source epoch/sequence幂等落账Harness completion Observation并关联不可变Claim，但不自动生成Runtime `ExecutionOutcome`；
- 无隐式生产SLA且Digest固定的显式SupervisionPolicy：定期Inspect、Identity Lease续租窗口、有界退避、Quarantine和Fence升级；
- Timeline、Checkpoint、Restore/Rewind和单活跃Run公共合同；
- Profile/Assembly、Harness/Model Invoker、Context/Cache、Tool/MCP、Memory/Knowledge/Asset、Organization/Review/Management、Sandbox、Budget、Evidence、Timeline和Checkpoint的版本化Port；
- `ComponentRegistry + ResolvedAgentPlan`装配门禁：版本、Digest、能力TTL、依赖DAG、Required/Optional和Residual；
- 组件中立Foundation Coordinator：Activate→Ready→Run→Checkpoint→Restore→Stop→Cleanup；
- 两种Execution Conformance共用闭环，以及回包丢失、Ready冲突、Partial Checkpoint和不完整Cleanup故障注入；
- Checkpoint-first V2 reference纵切：原子Attempt+Barrier、EffectCut、Participant closure、Continuity immutable Manifest Seal双读、唯一Consistency/Finalization及终态Attempt；Participant closure始终绑定collection Attempt，终态Inspect由Consistency回指该历史Attempt，禁止跨Attempt拼接和ABA；
- 普通、Race与Vet验证。

## 目录

| 目录 | 职责 |
|---|---|
| `core` | 公共对象、摘要、错误、三维状态、Fence、Effect和恢复合同 |
| `control` | Transport-neutral命令、前置条件和安全支配 |
| `admission` | 持久化Activation阶段、原子提交合同和确定性恢复决策 |
| `kernel` | Instance聚合、revision线性化、Observation分类与纯函数监督决策 |
| `ports` | 相邻组件唯一允许的Runtime-facing接入缝隙 |
| `releasecandidate` | 固定`reference_only`的声明式Component Release、Readiness、Conformance与descriptor-only Factory |
| `foundation` | 只依赖公共Port的最小生命周期协调器与恢复入口 |
| `fakes` | 事实、Execution、Environment、Evidence和Checkpoint的确定性内存实现 |
| `tests` | 对象、状态、命令、并发、epoch、Effect、Port、容灾与完整闭环反例 |

## 与现有Model Invoker的边界

`ExecutionRuntime/model-invoker`继续独立拥有模型Route、协议映射、Provider、调用语义和已有单次执行并集。当前Runtime不导入、不修改它，也不复制其类型。

后续联合接线时，通过`ports.ExecutionPort`或经用户确认后的窄Adapter，把Model Invoker的稳定能力、Manifest、调用Observation和Effect关联翻译到全局Runtime合同。Model Invoker不能反向拥有全局Identity、Instance生命周期、Fence epoch、Command Log或Task/Goal结果。

## 当前明确没有实现

- Harness内部组成与首个Harness；
- Context Engine和Cache算法；
- Tool/MCP链条；
- Review、Management和Organization审批链；
- Memory、Knowledge和Asset内部逻辑；
- Sandbox、Budget、Evidence或权威事实存储生产后端；
- RPC、数据库、进程拓扑、Remote API、SDK和生产SLA。

这些能力目前只有Port合同，后续必须由用户逐项指导设计后再实现。

## 验证

```bash
cd ExecutionRuntime/runtime
go test ./...
go test -race ./...
go vet ./...
```

## 后续接入方式

后续组件必须先实现自己的窄Port和Descriptor，再注册进`ComponentRegistry`；每接入一个组件都重复执行同一套完整闭环、故障注入、Race和Vet验收。当前基座不据此授权任何真实组件、生产后端、Transport或SLA。
