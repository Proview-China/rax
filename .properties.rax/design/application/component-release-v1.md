# Application Shared Engine Component Release V1

Application通过一个Host Control Plane组件发布六项共享协调能力：Command/Outbox Workflow V2、Run Coordination V3、Governed Operation V3、G6A V2、Context Refresh V1和Checkpoint Coordination V1。每项拥有独立Capability、Port及Factory descriptor；Application只编排公共Port，不接管Runtime或领域事实。

Manifest只把`Cleanup`分配给Application；`Effect`与`Settlement`精确归属Runtime公共`RuntimeSharedEngineComponentIDV1`。Release显式声明对`praxis.runtime/execution-governance`的Required Capability、Runtime Component Dependency及fail-closed Assembly Dependency，禁止Application自授Effect/Settlement Owner。

| 模式 | exact证明 | 当前结论 |
|---|---|---|
| reference_only | 完整Manifest/Module/六Capability/Port/Factory | 可发布 |
| standalone | 六项owner-local协调切面均有exact current | 可发布候选 |
| production | 七类durable store、Outbox/Recovery worker、Runtime Governance/Run Settlement/Execution Gateway、Cleanup、production root、Deployment Attestation、独立Certification | NO-GO |

Local与Production Readiness都使用exact Ref、canonical digest、共同TTL和S1/S2复读。任何Reader不可用、漂移、TTL crossing或clock rollback都Fail Closed，不能降级。Catalog写回包丢失只Inspect原Release Ref。

Factory只描述构造合同，不含Coordinator实例、backend句柄、Runtime gateway或生产root。`fakes/**`、owner-local测试、单个Coordinator与跨模块fixture均不构成production证明。

Application Port本身是中立协调命令面，不把领域Effect/Settlement语义复制进Assembly Port；真正的外部Effect始终经过Runtime Governance Gateway和领域Owner。
