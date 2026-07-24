# Runtime Shared Engine Component Release V1实施计划

状态：P4 owner-local候选已实现；production保持NO-GO。

## 已完成

- [x] 核验Binding/Command/Admission/Activation/Run/Effect/Evidence/Settlement/Checkpoint公共Fact/Gateway；
- [x] 核验SQLite实际表面仅覆盖Binding、EvidenceSubject与Review current子集；
- [x] 在`runtime/ports`冻结唯一公共`RuntimeSharedEngineComponentIDV1`；
- [x] 新增Release Builder、Readiness、Conformance和lost-reply exact Publisher；
- [x] 发布Manifest、Module、Capability、Port、三Owner、Artifact、Certification、Evidence、TTL与descriptor-only Factory；
- [x] 固定`reference_only`并拒绝production自提升；
- [x] 覆盖public identity外部导入、alias canonical漂移、64并发、typed-nil、TTL/clock/drift；
- [x] 不导入Application、Host或6+1实现。

## Production残余

- [ ] 完整durable facts与current indexes；
- [ ] Scheduler/Supervision生产协调；
- [ ] Activation/Run/Cleanup/Reconciliation Gateway真实接线；
- [ ] Checkpoint/Restore生产Provider和持久闭包；
- [ ] 可执行Factory、deployment attestation与production composition root。

本计划不授权修改Application、Harness、6+1或Host。
