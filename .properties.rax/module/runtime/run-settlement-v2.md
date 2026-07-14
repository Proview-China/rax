# Runtime Run Settlement V2模块说明

## 作用

该模块提供持久Run创建、Run Effect原子登记与冻结、独立Execution/Claim/Effect/Participant复读、Closure Attempt恢复、Outcome派生、Decision+Run原子提交，以及终态后的Termination Progress/Report。调用方不能传Outcome，组件不能改Runtime终态。

## 代码组成

- 公共Plan、Policy、Participant与Execution Inspector合同：`ports/run_settlement_v2.go`
- Run/Plan、分段Effect Index、Closure/Decision/Progress/Report事实：`control/run_settlement_fact_v2.go`
- Outcome派生与恢复入口：`kernel/run_settlement_gateway_v2.go`
- 测试Fact Owner与输入投影：`fakes/run_settlement_store_v2.go`、`fakes/run_effect_store_v2.go`、`fakes/run_settlement_inputs_v2.go`
- 自定义Participant与Backend testkit：`conformance/run_settlement_v2.go`
- Application-facing Run生命周期与pending→running入口：`ports/run_lifecycle_v3.go`、`kernel/run_lifecycle_gateway_v3.go`、`control/run_start_governance_v3.go`
- Plan声明聚合、认证与Binding currentness只读投影：`ports/run_plan_admission_v3.go`、`control/run_plan_admission_v3.go`、`control/provider_binding_currentness_v2.go`
- Foundation restricted兼容与公共生命周期恢复适配：`foundation/run_v2.go`

## 公共接入面

组件runtime-adapter只允许导入`runtime/core`和`runtime/ports`：实现`RunSettlementParticipantPortV2`，返回Plan预先绑定的exact namespaced Requirement、Owner、Schema、Subject、Evidence和Disposition。注册、返回成功或通过Conformance都不授予Binding、Policy、Dispatch、Commit或Outcome资格。

Application通过versioned Port访问Run bundle、分区Run Effect、Claim Association、Evidence Reader、Execution Inspector和Participant Reader；不得导入fake、Foundation或Harness内部类型。跨组件编排留在Application/Governance Gateway，组件之间不建立实现依赖。

## 可预期行为

- Run与Settlement Plan在dispatch前原子创建；
- Effect write-ahead与分段索引登记在一个Effect Fact Owner内原子完成；
- stopping是创建/dispatch冻结边界，冻结集合可确定分页枚举；
- Claim只提供Evidence关联，不选择Outcome；
- Execution Evidence精确绑定Inspection ID/revision/truth/causation；Observation、自报成功或领域失败都不会直接完成Run；
- unknown、failed、not_applied分别由显式Policy决定block或NeedsReconciliation；
- Closure attempt可在TTL续租后重建，旧attempt不能Commit；
- Decision与terminal Run原子提交且不可改写；
- pending→running与不可变Start Confirmation同Owner原子提交；公共Inspect返回完整Operation/Attempt/Settlement proof，不以裸running或StartedAt恢复；
- Plan Admission复读所有member Fact/grant、唯一宿主certifier与baseline，Certification精确聚合Runtime reserved及namespaced custom Requirement；
- pending Run原子持久Run、Plan与Certification Association，后续Start、Claim V3、Stop/Settlement/Reconcile均先做历史认证preflight且失败零副作用；
- Fact Owner从Closure重新验证Decision/Outcome并从Progress重建Report，raw Port不能伪造终态；
- Termination Progress是可交付interim snapshot，永久unknown不假增长，Report持久且只在完全闭合后产生。

## Live capability caveat

1. 当前fake只证明单Owner状态机、线性化、分页和故障恢复，不代表生产持久Backend、分布式事务或SLA；
2. Binding、Authority、Policy、Evidence、Participant与Run Fact属于不同Owner，Gateway逐次精确复读但不宣称跨Store原子快照；
3. pending Run、start confirmation、Settlement与terminal cleanup恢复已有公共Port；legacy Foundation Close/Fence/Release仍仅为restricted compatibility；
4. Operation V3公共治理面已完成，仍需Application Step Journal与Host/Data Provider governed bridge完成最终组合接线，才能宣称外部动作端到端可恢复；
5. 当前未选择生产Scheduler、RPC、签名、存储或进程拓扑。
6. `TrustedRunAssemblerPortV3`与Plan Admission仍只允许由宿主可信装配注入；组件只能发布声明，不能certify或create Run。认证事实与内存fake不代表生产认证Backend或availability。
