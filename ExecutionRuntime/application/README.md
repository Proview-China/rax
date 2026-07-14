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

Application不拥有Runtime/领域事实，不产生dispatch资格。外部动作必须经过Runtime Operation Governance Gateway和实际Provider二次校验；Runtime Settlement形成后仍由精确Domain Adapter唯一吸收。当前fake只用于合同测试，不代表生产数据库、消息队列、RPC、Scheduler、可信Plan Assembler或SLA；公共基座解锁组件开发不等于完成生产部署。

验证：

```bash
go test -count=1 -shuffle=on ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
```
