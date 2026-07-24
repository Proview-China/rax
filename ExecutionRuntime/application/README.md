# Praxis Application Coordinator

本模块是Runtime与Harness/组件之间的持久编排层。当前已实现：

- 不可变Command Payload与组件中立Workflow DAG；
- Submission先于Runtime Command acceptance；
- Outbox先交给持久Step Journal，再标记dispatched；
- namespaced自定义Step Catalog；
- unknown required fail closed、权威unknown optional显式skipped；Catalog暂时不可用仍保持可恢复，不伪装成组件不存在；
- skipped optional作为显式no-op满足依赖，后续required Step不会被未安装的可选模块卡死；
- `GovernedOperationCoordinatorV3`把一个Step按持久Attempt推进为Intent→Domain Reservation→Admission→Permit→Begin→Delegation→Prepare/Enforcement→Execute/Inspect→Observation/Unknown→Settlement；
- post-prepared unknown只接受由exact Prepared与持久Enforcement证明的Permit Fact单调`+1`；Enforcement的Permit/attempt/operation/effect链、RecordedRevision或增量任一漂移均关闭，pre-prepared unknown不得无来源推进Permit revision；
- `OperationDomainRouterV3 + OperationDomainStatePortV3`按精确namespaced Step Kind、Descriptor和Domain Adapter Binding路由领域Owner，不对内置6+1或未来自定义组件写switch；
- Domain Owner必须在任何Runtime mutation前原子保留exact rev1 Attempt/Intent/Subject/Session/Candidate；回包丢失只Inspect，历史过期Reservation仍可恢复，但过期后所有后续Runtime mutation均关闭；
- `RuntimeBindingCurrentnessAdapterV3`只把Runtime已验证的Binding当前性投影缩短为最多30秒的Application只读授权，不绑定、不续租、不提升权限；Router在注册和每次解析时都复读；
- 领域状态只允许prepared→observed|unknown→settled，所有不确定回包都按exact Fact Inspect，Observation不能直接完成Step或领域Commit；
- 可复用Domain Adapter Conformance覆盖幂等、并发线性化、精确Inspect和伪造换链拒绝，但永远不授予Binding、生产、dispatch或Commit资格；
- `RunCoordinatorV3`把Application恢复水位持久化为create_planned→pending→start_planned→running→claim_planned→claim_associated→stop_planned→stopping→terminal_cleanup→termination_closed；
- Run创建、Lifecycle、Start Confirmation与Claim V3始终携带同一个Runtime Plan Certification Association；Run只能由Runtime对exact settled start Attempt形成Start Confirmation后进入running；Harness terminal Claim只形成经认证的Evidence Association，不选择Outcome；unknown Participant停留在terminal_cleanup并显式Reconcile；
- CAS成功回值和lost-reply Inspect结果都必须是exact请求或从同一水位可证明的严格append-only successor；并发Sibling可继续外层状态机，等revision伪造、换Certification和换Plan均拒绝；
- Runtime Binding currentness、Governed Journal Completion和Recovery List/Acquire/Release的public request均在任何backend调用前集中Validate；非法Scope、ID、Owner、Policy、revision、epoch、TTL或游标不会触发读写；
- 生产包只允许导入`runtime/core`与`runtime/ports`，可执行import-boundary测试禁止依赖Runtime Owner/kernel/foundation/fakes或Harness内部包；
- lost reply exact Inspect、CAS、并发、深拷贝隔离，以及自定义第八/第十一模块和真实Harness Domain Adapter跨模块黑盒测试。
- `SingleCallToolActionCoordinatorV1`闭合N=1 G6A隔离切片：严格中立Request、Session/Turn/ParentFrame三种distinct applicability source、完整Model Projection坐标、Application write-ahead Fact、唯一`StartClaimID`先CAS为`waiting_inspect`后才允许一次同canonical Tool start-or-inspect；该状态下NotFound/Unavailable/Indeterminate均只Inspect、永不重派；S1/S2、Runtime V4 current Inspection与public Association逐边界使用fresh clock复读，时钟回拨或途中TTL跨越Fail Closed；完成后硬停，不包含Context Refresh、Continuation、Turn推进、Capability启用或Tool Provider Boundary proof；
- `SingleCallToolActionInputCurrentReaderV1`与`SingleCallOperationSettlementCurrentReaderV1`仅为Application窄只读依赖。测试以neutral fixture注入，未实现Harness/Model/Context/Tool Adapter，也不宣称跨Owner集成、生产root、Backend或SLA。
- `ReviewWaitingCoordinatorV1`闭合REV-D8 Application-owned持久等待：Inline/Detached仅为正交Delivery字段；Application先以SQLite WAL或同合同FactPort写入`waiting_review`，唯一`StartClaimID`成功CAS为`waiting_inspect`的caller才可调用一次Review `StartOrInspect`，CAS幂等回值、冲突、lost reply和重启恢复caller全部永久Inspect同一Case；Review/输入current按S1/S2、fresh clock和最短TTL重验，Target漂移只把Application协调Fact追加为`superseded`并要求新canonical request重新Review。该切片只导出neutral coordinate/Port/PhaseReceipt，不写Review/Harness/Runtime/Model事实，不提供Review adapter或production composition root。
- `SandboxLifecycleCoordinatorV4`通过公开`SandboxLifecyclePortV4`协调start-or-inspect；请求只携Plan exact ref、Operation/Effect/Attempt，结果只暴露Runtime DomainResult/Settlement exact refs。调用回包丢失只走公开Inspect，返回另一request/plan/closure即拒绝；Application不导入Sandbox实现或接触Provider handle/Receipt。Sandbox-owned production composition由Sandbox模块实现并注入此Port。
- `CheckpointCoordinatorV1`已闭合Checkpoint-first reference纵切：Harness Gate→Runtime原子Attempt/Barrier与EffectCut→两个Participant exact commit/Inspect→Continuity Manifest/Seal→Runtime Consistency→Gate release。它只经公开Port编排，lost reply只Inspect原identity；测试链`ProviderCalls=0`，不构成生产Checkpoint root、真实Snapshot capture或Restore能力。
- A-CTY-01最小公开Delta已落地：`application/contract`定义coordinate-only的七类Continuity Workflow Request/Inspection，`application/ports.ContinuityWorkflowSubmissionGatewayV1`公开`Submit+Inspect`，root Gateway只接收trusted Assembler产出的exact Bundle并复用既有Facade。caller不能提交raw Bundle、Permit、Review、Provider、可信current/sequence或Runtime Outcome；lost reply只按原Request/Command Inspect。当前没有production Assembler/root，也不解锁Restore或Provider。
- Agent State Plane V1新增单节点SQLite WAL后端：同一数据库以schema version/digest、history/current外键、payload row digest和严格JSON decode持久化`AgentLifecycleFactV1`与`AgentActivationCoordinationFactV1`，提供create/Inspect/revision+digest CAS；Expected失配（含same-next replay）恒Conflict，只有本次exact invocation CAS正常回值才取得Step Start资格，提交后丢回复及恢复到invocation/unknown均Inspect-only。该后端只承诺本机crash durability，不声明HA、远程副本或SLA；production Agent coordinator仍缺八个真实step adapters与composition root。

Application不拥有Runtime/领域事实，不产生dispatch资格。外部动作必须经过Runtime Operation Governance Gateway和实际Provider二次校验；Runtime Settlement形成后仍由精确Domain Adapter唯一吸收。当前fake只用于合同测试，不代表生产数据库、消息队列、RPC、Scheduler、可信Plan Assembler或SLA；公共基座解锁组件开发不等于完成生产部署。

验证：

```bash
go test -count=1 -shuffle=on ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
```
