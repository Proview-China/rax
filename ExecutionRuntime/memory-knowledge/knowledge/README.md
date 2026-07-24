# Knowledge Owner backend-neutral framework

本包是Knowledge领域Owner的Go参考实现，只拥有Knowledge事实，不拥有Runtime Outcome、Binding、Policy、Trust、Context Frame或其他组件事实。

当前已闭合：

- `Source`、`Package`、`Candidate`、版本化`Admission`、`Record`、`Projection`、不可变`Snapshot`与独立current pointer、`View`；
- Correction、Withdraw与Tombstone；
- `ProducerID + SourceEpoch + SourceSequence`的tenant内exact-idempotency；
- 显式`ExpectAbsent`/`ExpectRevision` CAS、TTL/currentness、Authority/Policy精确水位；
- 本地确定性词法检索、Source/Package/Snapshot Citation、Coverage与Dropped Reason；
- `DomainResultFact -> RuntimeSettlementRef + DomainResultAssociation -> ApplySettlement`。Runtime Settlement仅作为opaque Ref消费，association必须精确绑定DomainResult的ID/revision/canonical digest，领域Result不会被自动改成settled；
- Begin后丢回包通过`InspectCommit`检查原Operation/Attempt，不创建新Attempt。
- Source Refresh/Deprecate/Withdraw、Correction/Conflict、Export/Watch/Reindex、metadata-only Purge Intent；
- Acquire/Parse/Normalize/Validate/Index/Snapshot/Publish阶段Journal与Settlement-gated两阶段Sync；
- Source Connector只定义受治理Request/Observation/Inspect原Attempt合同，不实现网络或Provider；
- Skill/Lexical/Vector/Graph Projection、Hybrid Query、Citation/Coverage/Cursor及Owner-local V1/V2 Context Current Reader。

边界：

- `Access`只核验已经治理完成的Tenant/Authority/Policy精确引用；本包不签发、解释或扩大这些事实。
- `ContentReader`只允许本地只读物化。本包不实现真实Connector、生产Vector/Graph、Remote Index、无Run管理动作或Runtime/Context/Review/Assembly Adapter。
- 内存Store与Reference Store仅用于Wave 1参考和测试，不代表生产Backend、持久性或SLA。
- 所有返回集合均深拷贝；Snapshot发布后不原地修改，current pointer单独CAS。

包内验证：

```text
go test ./knowledge
go test -run 'StateMachine|CAS|Fault|BlackBox|Conformance' ./knowledge
go test -race ./knowledge
```
