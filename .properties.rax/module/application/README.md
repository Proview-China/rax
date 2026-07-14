# Application模块说明

Application模块把Runtime的线性化事实与Harness/组件Port组织成可恢复工作流。它的核心资产是不可变Submission和持久Step Journal，而不是业务算法。

当前代码包含：

- `FacadeV2`：持久Submission后接纳Command，任一未知回包只Inspect；
- `OutboxDispatcherV2`：验证accepted Command与Outbox exact payload，创建Journal，再标记交接；
- `WorkflowPlanV2`：组件中立namespaced DAG；
- `WorkflowJournalV2`：单Step单CAS推进，依赖完成后才Ready，禁止假revision；
- `StepCatalogV2`：用户自定义组件的发现面，不授予执行权；Descriptor的revision/digest/TTL随Plan冻结并在Journal交接时复读，只有权威unknown允许跳过optional，Catalog故障保持可恢复；
- `WorkflowJournalRecoveryPortV2`：完整ExecutionScope分区的有界恢复发现与CAS Worker Claim；Claim要求调用方显式提供Policy Digest和租期，不内置默认SLA，也不产生执行权；
- `conformance`：可供第八、第九及后续自定义模块复用的Step Catalog检查；它只验证Kind、执行类别、Capability与Schema精确匹配，报告固定不可用于Binding、dispatch和领域Commit；
- `GovernedOperationCoordinatorV3`：先持久Attempt、Domain Reservation和Journal，再编排Runtime Admission→Permit→Begin→Delegation→Prepare/Enforcement→Execute或Inspect→Observation/Unknown→Settlement；
- Operation unknown因果门禁：pre-prepared分支保持Begin revision；post-prepared分支只能以exact Prepared/Enforcement及其`RecordedRevision`证明Permit Fact单调`+1`，不能仅凭错误回包或Application本地推断推进；
- `OperationDomainRouterV3`：以namespaced Step Kind、冻结Descriptor和Domain Adapter Binding选择唯一领域Port，新增组件无需修改Application状态机；
- `RuntimeBindingCurrentnessAdapterV3`：消费Runtime只读Binding当前性Port，验证exact Provider Binding并生成最多30秒的Application本地投影；它不授予Binding或执行权；
- `OperationDomainReservationRefV3`：Domain Owner在Runtime mutation前原子保留rev1 Attempt、Intent、Subject、Session和Candidate。过期Fact仍可Inspect用于恢复，但不得继续Admission/Issue/Begin/Declare/Prepare/Execute；
- `OperationDomainStatePortV3`：领域Owner以create-once/CAS方式保存prepared、observed、unknown和settled状态；Provider回包、Observation或Application摘要都不能替代领域事实；
- `CheckOperationDomainStatePortV3`：验证正常/unknown分支、幂等回放、64路并发线性化、exact Inspect与换链拒绝；报告的生产、Binding、dispatch、Commit资格固定为false；
- `RunCoordinatorV3`：以独立Application Fact记录Run恢复水位；可信Assembler提供带Plan Certification Association的不可变Runtime Plan，Runtime Lifecycle/Start/certified Claim V3 Owner分别创建pending Run、验证exact settled start、摄取Claim及派生终态；
- Run状态链：`create_planned→pending→start_planned→running→claim_planned→claim_associated→stop_planned→stopping→terminal_cleanup→termination_closed`。所有写入回包丢失先Inspect，Claim只关联Evidence，unknown Cleanup不伪造完成；
- import-boundary测试：Application生产包只依赖Runtime `core/ports`，禁止导入Runtime Owner/kernel/foundation/fakes及Harness内部实现；
- 跨模块黑盒：两个自定义namespaced模块通过同一Coordinator；真实Harness Model Turn Domain Adapter完成observed→settled和pre-prepared unknown→failed两条闭环；
- 测试Fact Store：确定性、深拷贝、线程安全、lost-reply注入；CAS成功和恢复结果必须Validate，且只能接受exact值或严格append-only successor。
- public request门禁：Runtime Binding currentness、Governed Completion、Recovery List/Acquire/Release先集中校验Scope、ID、Owner、Policy、revision、epoch、TTL与游标，失败时不调用任何backend。

生产接线必须经过Runtime Operation Governance Gateway与实际执行点二次校验；本模块本身不拥有Permit、Fence、Review、Budget、ExecutionOutcome或领域Commit。fake、Conformance、测试Assembler和跨模块测试替身不代表生产Backend、认证、可信Plan装配或SLA；开发接口解锁不等于生产部署完成。
