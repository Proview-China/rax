# Application Coordinator设计入口

## 定位

Application层是用户/API、Runtime Control Plane、Harness与6+1组件之间的可恢复编排层。它只拥有工作流提交、Outbox交接和Step Journal，不拥有Runtime状态、组件领域结论、Effect授权或Provider结果。

## 已冻结边界

- 用户提交先形成不可变`CommandPayloadFactV2 + WorkflowPlanV2`，再由Runtime `CommandFactPort.AcceptCommand`原子接纳Command、Desired State和Outbox；
- Outbox的`dispatched=true`只表示已经交给持久`WorkflowJournalV2`，不表示Provider调用、领域提交或任务成功；
- Step Kind必须是namespaced值，通过`StepCatalogV2`发现；Coordinator不对continuity/tool/memory/sandbox/review/context/Harness做switch；
- governed Step Descriptor必须绑定唯一namespaced Provider Capability，并以revision/digest/TTL进入Workflow Plan；同一个自定义Kind不能在计划交接时换绑另一能力、版本或过期描述符；
- 未知required Step fail closed；只有Catalog权威返回`unknown capability`时，optional Step才保留原Opaque Payload并记为显式`skipped`。Catalog超时/不可用保持可恢复，不能伪装成组件不存在；
- 显式`skipped`的optional Step按no-op满足依赖；它不产生领域成功、Effect或Settlement，但也不会让后续required Step永久等待；
- 外部动作先由唯一Domain Owner原子提交exact Domain Reservation，再统一进入Runtime Operation Effect→Governance Gateway→Delegation→Provider Prepare→持久Enforcement→ExecutePrepared→Inspect/Settlement；Application禁止裸调Fact Store的Begin；
- Domain Reservation绑定rev1 Attempt、Intent、Subject、Session与Candidate；历史过期Fact可Inspect用于恢复，但任何Runtime mutation前必须复读当前Reservation和Binding currentness，过期或漂移关闭；
- `UnknownOutcome`只进入Inspect，不能重派；每个跨Owner步骤先写Journal，再调用，再按exact Fact Inspect恢复；
- 当前V2保留`indeterminate/blocked`读模型但禁止无Policy Fact的新跃迁；未知结果持续留在`waiting_inspect`，直到对应Owner提交精确Settlement；
- Application只读取并编排Runtime/领域Fact，不直接写Kernel事实，也不把Receipt/Observation升级成权威结果。
- governed Step完成时必须保留原write-ahead Effect ref并附Settlement；不能用终态CAS洗掉执行历史。
- Submission、Journal、恢复列表与Worker Claim均按完整`ExecutionScope`分区；Claim是显式Policy和TTL约束的调度租约，不是Effect或Provider执行权。

## 自定义组件扩展

自定义组件注册namespaced Step Kind、版本合同、Schema和执行类别。计划中的Provider Binding、Payload Schema、Authority、Operation Subject和后续Permit必须逐层精确绑定。新增第八、第九组件不修改Application状态机；未知required能力仍关闭执行。

## 当前实现切面

- `ExecutionRuntime/application/contract`：Submission、Workflow DAG、Step Journal公共合同；
- `ports`：Submission/Journal Fact Port、Step Catalog与有界恢复列表/Worker Claim Port；
- `fakes`：线程安全CAS测试Fact Store，不声明生产持久性或SLA；
- `FacadeV2`：Submission→Command acceptance，回包丢失只做exact Inspect；
- `OutboxDispatcherV2`：Journal先于Outbox dispatched，支持重启与并发幂等恢复；
- `WorkflowJournalRecoveryPortV2`：完整Scope分区、有界列举、CAS认领、过期接管与丢回包Inspect；
- `conformance`：自定义Step Kind/Schema/Capability的公共testkit；报告永远不授予Binding、dispatch或Commit资格；
- `GovernedOperationCoordinatorV3`：Attempt、Domain Reservation和Journal write-ahead后，依次编排Runtime Admission、Permit、Begin、Delegation、Prepare/Enforcement、Execute/Inspect、Observation/Unknown与Settlement；
- `OperationDomainRouterV3`：按精确Step Kind、Descriptor和Domain Adapter Binding解析唯一领域Owner，不内置6+1或自定义模块分支；
- `RuntimeBindingCurrentnessAdapterV3`：只读验证Runtime Provider Binding投影并缩短为最多30秒的Application授权；不绑定、不续租、不提升权限；
- `OperationDomainStatePortV3`：领域Owner持久吸收prepared→observed|unknown→settled，回包不确定只Inspect；Observation与Provider Receipt都不能直接完成Step或领域Commit；
- `CheckOperationDomainStatePortV3`：复用同一黑盒合同验证幂等、并发线性化、exact Inspect和换链拒绝，报告不授予生产、Binding、dispatch或Commit资格；
- `RunCoordinatorV3`：持久保存Application Run恢复水位，经`TrustedRunAssemblerPortV3`、Runtime Lifecycle/Start/certified Claim V3公共Port编排pending Run、exact Plan Certification/Start Confirmation、Claim Evidence Association、Settlement与Termination Reconcile；
- Create、Lifecycle、Start与Claim必须保留同一Plan Certification Association；CAS成功回值和恢复Inspect只能接受exact值或严格append-only successor，并发Sibling不得被误判为回退；
- Run Claim required/optional由不可变Runtime Settlement Plan和唯一Settlement Owner判定；Application只摄取精确Association，不解释ClaimKind、不生成Outcome；
- terminal Run仍可有未闭合Cleanup；unknown Participant保持`terminal_cleanup`，只有显式Inspect/Reconcile得到新的Runtime权威Envelope后才能进入`termination_closed`；
- 可执行import-boundary门禁：Application生产代码只可依赖Runtime `core/ports`，不得导入Runtime Owner、Kernel、Foundation、fake或Harness内部包。

## G6A：单Call Tool Action协调合同

N=1 Action Gateway的V1 Application owner-local协调实现与测试已完成；该V1链不含Identity V2，也不接入Harness P3；Tool P4/P5/system链仍未完成，不能计为系统G6A或production GO：

- [SingleCallToolActionPortV1设计](./single-call-tool-action-v1.md)
- [G6A测试矩阵](./single-call-tool-action-v1-test-matrix.md)
- [调用与依赖图](./single-call-tool-action-v1.drawio)
- [实施计划](../../plan/application/single-call-tool-action-v1.md)

该增量只冻结Application自有窄Port、中立DTO、协调Journal与测试组合边界。Session、Turn与ParentFrame CTX-D10 applicability source均使用Application自有distinct nominal四元坐标；ParentFrame metadata不能替代CTX-D10 source，公共Runtime ref只能由后续Router无损投影并经对应Owner Reader复读。它不导入或复制Harness、Tool、Context类型，不预载Runtime Applicability Ref、不持有Tool Provider Boundary proof、不创建生产composition root，也不把G6B Context Refresh、Continuation或Turn推进带入G6A。

### G6A Identity additive V2

状态：联合设计终审`YES`（P0/P1/P2=0），design/plan已冻结；Application owner-local P2代码第四独立终审`YES（P0/P1/P2=0）`。V2不修改V1摘要，Binding Base直接复用V1 `SingleCallPendingActionCoordinateV1`，不复制同形V2 nominal；V2 Proof不携带旧V1 Session/Turn Applicability source。P2 FactPort只验证Coordination结构与CAS，不独立证明Tool Owner结果真实性或以caller时间签发current；该权威门属于P4/system。Harness P3 Assembler/InputCurrent Reader已实现，并在live合同漂移与并发P1返修后独立复审 YES（P0/P1/P2=0）；Tool P4与P5跨模块fixture尚未实现，系统G6A和production composition root继续`NO-GO`：

- [SingleCallToolAction V2 Identity设计](./single-call-tool-action-v2.md)
- [V2测试矩阵](./single-call-tool-action-v2-test-matrix.md)
- [V2调用图](./single-call-tool-action-v2.drawio)
- [V2实施计划](../../plan/application/single-call-tool-action-v2.md)
- [跨Owner Identity事实源](../harness/assembly/model-tool-call-pending-action-identity-v1.md)

## 明确非产物

当前不选择数据库、消息队列、RPC、Scheduler、进程拓扑、可信Plan Assembler或默认SLA；fake/conformance通过不产生生产认证、Binding或dispatch资格。公共合同已经允许6+1和后续自定义组件开发，但不代表生产部署完成。

## Shared Engine Component Release

[Component Release V1](component-release-v1.md)把Command/Workflow、Run V3、Governed Operation、G6A、Context Refresh和Checkpoint作为六项独立协调Capability交给Agent Assembler。当前最多形成standalone候选；durable stores、worker/recovery、Runtime gateways、cleanup、deployment attestation、独立Certification与production root未闭合，production保持NO-GO。

## Agent Activation V2 候选

HostV2参考纵切当前使用V1 fake闭合Activation结果，不能接真实Runtime/Sandbox/Harness八步事实。additive [Agent Activation V2](agent-activation-v2.md)与[实施计划](../../plan/application/agent-activation-v2.md)已完成独立设计复审（P0/P1=0），当前待用户设计审核。在用户审核、Owner窄Reader与Host Service V3接线完成前，production Activation保持NO-GO。
