# Runtime执行底座P0实施清单

## 状态

用户已批准按“公共合同先行→事实Port/状态机→协调器→Foundation适配→Conformance testkit→全量闭环”实施。P0.1至P0.6及Runtime/Application/Harness组合闭环已经完成；6+1与后续自定义组件可开始领域实现和Port适配。

## 阶段

- [x] P0.1 Binding Manifest V2、自定义组件扩展面、BindingFact/BindingSet、Conformance首切面。
- [x] P0.2 Effect Fact Journal、Governance Dispatch Gateway、Budget Binding V2、CurrentScope/Run一致投影、Gateway Begin最终门禁、绑定Enforcement二次验真、稳定冲突域与正交Settlement/Residual/Cleanup收口。
- [x] P0.3 Review Verdict V2、Condition Satisfaction、Review Governance Gateway、自动Reviewer Effect约束与执行点三次复读。
- [x] P0.4 Evidence Ledger V2、独立Source Policy、Timeline单主与V2 Run Claim精确持久关联。
- [x] P0.5 Run Settlement/CompleteRun、分段Run Effect、Closure Attempt、持久Foundation准备边界与Conformance。
- [x] P0.6 Runtime公共入口：Operation V3、Execution Delegation、certified pending Run/RunLifecycle V3、Run Claim ingest V3、Application Command/Outbox ports seam。
- [x] P0.6 Application Coordinator、Step Journal、Command Outbox Dispatcher与Harness governed bridge组合闭环。
- [x] Runtime全量普通、shuffle Race、Vet与说明资产收口。
- [x] Runtime/Application/Harness最终组合门禁与6+1开发解锁裁决。
- [x] P0.6 Run Settlement Plan Admission/Certification：宿主可信Assembler复读current BindingSet、全member Fact/grant、baseline与组件声明，签发create-once proof；组件和普通Application不能certify/create。

## 兼容与禁止项

- 保留现有Activation、RunClaim、Supervision和`v1alpha1`组件合同。
- 破坏性变化只新增version/type/adapter，不原地提升Observation权威性。
- 不修改Harness、6+1组件、model-invoker、根Workspace、CI、MAIN或全局索引。
- 不选择生产数据库、RPC、进程拓扑、Scheduler或SLA。
- Runtime不导入组件实现；Application只通过公共Port编排。

## P0.5 live caveat

- Foundation V2已持久准备Run+Plan+Effect Index，并移除Settlement caller Outcome；但直接Close/Fence/Release仍为restricted compatibility。
- P0.6必须以Operation Effect、Step Journal和Prepare→持久Enforcement→ExecutePrepared替代真实start/cancel/close/release，才可宣称外部动作闭环。
- P0.5不声明生产Backend、跨Owner原子快照、RPC、Scheduler、进程拓扑或SLA。

## P0.6 Runtime公共面

- `OperationEffectAdmissionPortV3`只接收受绑定Intent Owner提交的不可变Operation Intent，并提供accepted事实只读恢复；Application不得直接写Effect Fact Store。
- `OperationGovernancePortV3`负责Issue、Begin、unknown与Inspect；授权Envelope携完整Permit与Fence，Begin以后不确定只能Inspect原attempt。
- `ExecutionDelegationGovernancePortV2`负责Host relay到Data Provider的declare、prepared commit和Inspect；Provider执行前仍须复读current治理事实及持久Enforcement。
- `OperationObservationGovernancePortV3`只摄取精确Evidence绑定的Provider Observation；`OperationSettlementGovernancePortV3`由Settlement Owner提交权威Disposition与受治理Domain Result。Observation不直接驱动Harness终态。
- `RunLifecycleGovernancePortV3`提供certified pending Run创建、BeginStop、Settlement、Termination reconcile/inspect；`RunClaimIngestGovernancePortV3`在历史认证preflight后提供Claim Ledger+Association精确摄取，Claim仍不选择Outcome。
- `RunStartGovernancePortV3`返回持久`RunStartConfirmationEnvelopeV3`，Run与exact start Operation/Attempt/Settlement proof同Owner原子提交；裸running/StartedAt不能作为恢复证明。
- `ApplicationCommandFactPortV2`是Application唯一Command/Desired State/Outbox事实面。`runtime/control`中的同名别名仅供legacy/restricted迁移；Application导入规则只允许`runtime/core`和`runtime/ports`。
- 以上合同与内存fake不代表生产数据库、跨Owner事务、RPC、Scheduler、进程拓扑或SLA；当前只解锁6+1领域实现、Port适配与Conformance开发，不构成生产部署认证。
- `RunSettlementPlanAdmissionPortV3`与`TrustedRunAssemblerPortV3`仅由宿主可信装配持有；组件只能提交BindingSet绑定的声明，不能自行删Requirement、换Policy、签发Certification或创建Run。
- 所有公开Inspect/Begin/unknown/Observation/Settlement/Plan Admission入口均先验证完整Envelope、ID/revision和依赖，再读取Fact/Evidence/backend；非法输入保证零后端读取。
