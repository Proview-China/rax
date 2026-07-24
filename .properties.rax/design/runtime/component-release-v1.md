# Runtime Shared Engine Component Release V1

## 1. 裁决

Runtime shared-engine可以发布Agent Assembler可消费的完整声明式`ComponentReleaseV1`，但当前固定为`reference_only`。

live Runtime已有广泛的Binding、Command、Admission、Activation、Run、Effect、Evidence、Settlement和Checkpoint公共Fact/Gateway；SQLite State Plane也真实覆盖Binding、EvidenceSubject与Review Binding/Evidence/Governance current。其余主体仍由foundation、内存fake、reference Owner或Conformance candidate支撑，因此部分SQLite不能解释为完整production Runtime。

## 2. 唯一公共身份

`runtime/ports.RuntimeSharedEngineComponentIDV1`固定为`components/runtime`，是Runtime Release与Application-facing Binding唯一可导入的公共ComponentID。Application只需依赖`runtime/ports`，不得复制字符串，也不需要导入`runtime/releasecandidate`；Runtime不反向导入Application。

## 3. Release闭包

| 对象 | 固定值 | 边界 |
|---|---|---|
| Component | `ports.RuntimeSharedEngineComponentIDV1` | `components/runtime` |
| Kind | `praxis/runtime` | shared-engine |
| Capability | `praxis.runtime/execution-governance` | reference-only |
| Module | `module/runtime-shared-engine` | 声明式描述 |
| Port | `port/runtime/execution-governance` | unknown/stale/drift Fail Closed |
| Factory | `factory/runtime-shared-engine` | 仅descriptor，无可执行Factory |
| Conformance | `restricted_controlled` | Residual=`inspectable` |

`ConformanceV1`只承认public facts、public gateways与partial SQLite已存在；固定否认complete durability、Scheduler/Supervision、Activation/Run production root、Checkpoint Restore、可执行Factory、Cleanup root和SLA。

## 4. Publisher恢复

Publisher只使用Agent Assembler公共Release Publisher/Reader。Ensure返回indeterminate时，用`context.WithoutCancel`按完整Release Ref执行一次exact Inspect，不重试mutation；返回Release、TTL或时钟漂移一律拒绝。

## 5. Production P0

1. Command/Desired State/Outbox完整持久Owner；
2. Identity Lease、Activation Journal/Commit与恢复事实生产后端；
3. Run/Run Effect/Run Settlement及Operation Effect/Admission/Authorization/Enforcement/Settlement完整持久后端；
4. Evidence Ledger/current与Checkpoint/Restore完整持久闭包，不只partial SQLite/reference store；
5. 生产Scheduler、Supervision reconcile和进程存活观测；
6. Activation/Run Gateway、Cleanup/Reconciliation真实接线；
7. 可执行Factory、deployment attestation和production composition root。

上述proof未全部闭合前，Release不得进入`standalone`或`production`。
