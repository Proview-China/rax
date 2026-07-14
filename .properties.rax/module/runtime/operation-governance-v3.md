# Runtime Operation与Execution治理V3模块说明

## 作用

该模块为Application和Harness提供不依赖Runtime内部包的外部动作治理面：Intent准入、Permit Issue/Begin、Host→Provider Delegation、Prepare、持久Enforcement、ExecutePrepared、Observation Evidence、Settlement及unknown Inspect恢复。

## 代码入口

- 公共类型与Port：`ports/execution_governance_v2.go`
- Effect准入与Issue/Begin/unknown：`control/operation_admission_gateway_v3.go`、`control/operation_governance_gateway_v3.go`
- Delegation与prepared提交：`control/execution_delegation_governance_v2.go`
- Observation与Settlement治理：`control/operation_observation_gateway_v3.go`、`control/operation_settlement_gateway_v3.go`
- 测试Fact Owner与Provider：`fakes/execution_governance_v2.go`
- Application import门禁：`conformance/application_imports_v3.go`

## Application调用顺序

`Admit/Inspect accepted → Issue → Begin → Declare/Inspect delegation → Provider Prepare → CommitPrepared/Inspect → Harness AttachPrepared → ExecutePrepared/Inspect → Record/Inspect Observation → Harness AttachObserved → Settle/Inspect → Harness ApplySettled`。

任何Begin后的不确定结果进入`MarkOperationDispatchUnknownV3`；远程Inspect本身是新的受治理Operation。Application不得调用raw Fact Store，也不得把Observation解释成Settlement或Outcome。

`InspectPreparedExecutionV2`对exact declared但尚未prepared的事实返回`ErrorNotFound`，用于首次Prepare的只读判定；若Operation、Delegation或Permit不精确则保持`PreconditionFailed`。因此Application只能在权威确认“prepared事实尚不存在”后进入首次Prepare，不能把治理冲突翻译成NotFound。

## 兼容与限制

- 既有Effect V2保持Run Effect兼容；Operation V3以additive合同覆盖activation、run、termination、admin和custom subject，不伪造RunID；
- `GovernedExecutionAttemptRefsV2`供Harness持久状态引用，不把Harness Session、Model Turn或领域状态机塞入Runtime；
- `ApplicationCommandFactPortV2`是Command/Outbox唯一公共面，`control`别名仅供legacy/restricted迁移；
- fake只证明状态机、CAS、丢回包Inspect和race语义，不代表production durability、跨Owner原子事务、RPC或SLA。
