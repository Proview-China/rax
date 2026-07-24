# Knowledge领域合同 v1

本文件冻结实现所需对象、字段和状态机；不是已授权Go API。

## 1. 核心对象

### `KnowledgeSourceV1`

字段：Source ID/Revision/Digest、Owner、Authority、License、Asset/Connector Ref、Source Version、Content Digest、Schema、Sensitivity、Retention、AcquiredAt、ValidFrom/To、State、Current Pointer、Provenance和Policy。

State：`registered`、`acquiring`、`available`、`stale`、`withdrawn`、`purge_pending`、`purged`、`failed`。Source Ref变化或正文变化必须创建新Revision。

### `KnowledgePackageV1`

字段：Package ID/Version/Revision/Digest、Source Set、Record Manifest、Schema、Authority、License Summary、Policy、Coverage、Status、Created/Updated。

Package Version用于业务发布；Revision用于同一Version下的CAS事实水位，两者不可混用。

### `KnowledgeCandidateV1`

字段组：Candidate ID/Revision/Digest/Kind/State、Operation或Run关联、Target Source/Package/Record Revision、Payload Ref/Schema/Digest/Revision、Producer/Source Epoch/Sequence、Asset/Evidence/Provenance、Authority/Scope/Policy/Purpose、License/Sensitivity/Poisoning/Conflict、Created/Expires。

Candidate不可变；Admission、Review、Commit Attempt独立版本化。

### `KnowledgeRecordV1`

字段：Record/Claim ID、Revision、Digest、Package、Content Ref/Digest、Source Supports、Evidence、Scope、Authority、License、Trust State、Conflict Group、Valid/Transaction Time、Correction/Withdraw Links、Status、Policy。

Trust State不是模型评分；至少区分`unverified`、`source_supported`、`conflicted`、`withdrawn`。组织级“权威”只能来自明确Authority/Policy事实。

### `KnowledgeSnapshotV1`

字段：Snapshot ID/Version/Revision/Digest、Source Set、Package Set、Record Watermarks、Projection Refs、Coverage、Authority/Policy、Manifest Digest、State、Built/Published时间和Previous Snapshot。

Snapshot发布后不可变；修复通过新Snapshot，不原地改Manifest。

### `KnowledgeViewV1`

字段：View ID/Revision/Digest、Principal、Authority、Policy、Purpose、Snapshot集合、Scope/Domain/Source Filter、Sensitivity/License约束、Allowed Representations、Projection集合、预算、TTL和Residual Policy。

### `KnowledgeCommitAttemptV1`

字段：Attempt ID、Operation Subject、Candidate/Admission/Review Ref、Intent/Reservation/Admission Receipt/Permit/Begin/Delegation/Prepare/Enforcement Ref、Expected Fact Revision、Idempotency、Conflict Domain、Observation、Inspection、Evidence、DomainResultFact Ref、Runtime Operation Settlement Ref、ApplySettlement Revision、Residual和时间。

状态：`reserved`、`admitted`、`permitted`、`begun`、`prepared`、`executing`、`unknown_outcome`、`observed`、`settled`、`reconciliation_required`。状态不可跳过Begin后恢复规则。

## 2. Admission与版本CAS

`KnowledgeAdmissionFactV1`绑定Candidate、Source/License/Authority、Scope/Sensitivity、Duplicate、Entity Alignment、Conflict、Poisoning、Policy、Target Revision、Decision、Evidence和TTL。

CAS规则：

- Candidate按Producer Source Key exact-idempotent；同键换Payload为Evidence Conflict；
- Source、Package、Record、Snapshot、View各自使用Expected Revision；
- 首次创建使用显式`expect_absent`；
- Publish原子移动Current Snapshot Pointer并保存Previous；
- Withdraw原子写入Withdraw Fact并使新View水位不再包含目标；
- Projection水位变化不修改Record/Snapshot事实；只发布新的Projection Ref或新Snapshot。

## 3. Projection对象

`KnowledgeProjectionV1`字段：Projection ID/Kind、Revision/Digest、Source Snapshot、Record范围、Builder/Parser/Chunker/Model/Graph Schema/Reranker版本、Index Version、Coverage、State、Created/Updated、Residual。

State：`building`、`partial`、`ready`、`stale`、`rebuilding`、`failed`、`retired`。

Vector附加字段：Embedding Model/Revision、Dimension、Chunk Strategy、Record/Revision/Range。Graph附加字段：Node/Edge Schema、Owner、Source、Valid/Transaction Time。Skill Index附加字段：Title、Description、Keywords、UseWhen、DoNotUseWhen、DetailRef。

## 4. Retrieval与Citation

`RetrievalQueryV1`、`RetrievalCandidateV1`、`RetrievalResultV1`和`CitationV1`与Memory共用版本化协议外形，但`Domain=knowledge`时必须额外携带Snapshot、Package、Source、License、Trust、Conflict和Withdraw状态。

Result必须包含：Query/View/Snapshot水位、命中Projection、Record/Claim Revision、Source/Asset范围、Match Reason、Conflict组、Coverage、Dropped Reason、Representation、Citation、Next Cursor、Result/Evidence Digest。

无结果、权限拒绝、Partial Coverage、Source withdrawn和Projection failed必须分开表达。

## 5. Pre-run Evidence合同需求

无Run管理Operation需要`OperationEvidenceScopeV3`：

- 绑定`OperationSubjectV3`的kind、custom/admin operation ID、subject revision和current projection；
- 保留Tenant/Identity/Lineage层级，不要求伪造Run ID；
- Source Registration绑定Operation、Producer、Authority、Policy、Allowed Trust/Kind和TTL；
- authoritative evidence必须引用Knowledge Owner Fact的ID/Revision/Digest/Payload；
- source epoch/sequence、causation/correlation和append-only ledger规则保持不变；
- Runtime Run路径仍使用现有Evidence v2，不被放宽。

## 6. Unknown与错误

Begin前拒绝可证明Not Applied；Begin后超时必须`unknown_outcome`。Inspect必须绑定原Operation、Intent Revision、Permit、Attempt和Provider。远程Inspect本身是独立Operation Effect并通过非递归Inspection Settlement关联原Attempt。

稳定错误至少包括：`source_unavailable`、`license_denied`、`scope_denied`、`candidate_rejected`、`conflict_pending`、`review_stale`、`authority_stale`、`fence_stale`、`budget_unavailable`、`revision_conflict`、`snapshot_partial`、`projection_stale`、`source_withdrawn`、`context_reference_unmaterializable`、`commit_unknown`、`inspection_incomplete`、`purge_residual`。

## 7. 大小、排序与严格解码

- 所有集合有显式上限、稳定排序和唯一键；
- 大型Payload只用Ref，公共Envelope有长度和Schema；
- Canonical Digest采用domain/version/discriminator隔离；
- 严格解码拒绝重复键、未知字段和尾随文档；
- 可扩展字段进入版本化Opaque Extension，不允许“忽略未知治理字段”。

## 8. DomainResult、ApplySettlement与Residual合同

Knowledge Owner先形成DomainResultFact，至少包含Operation/Attempt、Owner Fact Ref、CAS Before/After、Inspection、Evidence Ref、Projection/Connector Coverage、Cleanup状态、Residual集合和Digest。Runtime Settlement Owner再提交封闭Effect Disposition的Operation Settlement；Knowledge通过ApplySettlement CAS形成领域settled/result_ready投影。Knowledge不写Runtime Outcome、Binding、Policy、Trust或其他Owner事实。

Residual至少区分：Provider状态未知、Source不可回读、Projection partial/stale、ContextReference不可物化、Withdraw传播未完成、Purge副本残留和Evidence追加待恢复。Residual不可由“Record/Snapshot存在”自动清除。

## 9. per-turn Context Source合同需求

本节的owner-current只读合同由[KnowledgeContextSourceCurrentReaderV1设计Delta](./context-source-port-v1.md)冻结，只允许本地`InspectAttempt/InspectForTurn/ReadContentExact`；Reader、Knowledge Adapter、Application三阶段Port、Context `knowledge_reference`与nonzero reference fixture均已YES。production G6B/root仍未装配。`Retrieve`、远程Resolver与正文读取属于[Knowledge Retrieval Domain Gateway Delta](./retrieval-domain-gateway-v1.md)，当前继续unsupported且Provider/Resolver=0；Checkpoint/Restore使用另行合同。

### `KnowledgeRetrievalObservationV1`

字段：Observation ID/Revision/Digest、Owner、Retrieval Attempt Ref、Query/View/Watermark/Published Snapshot exact refs、Result Ref、Result/Evidence Digest、Coverage、Next Cursor、稳定有序Hit refs、Observed/Expires和Residual。它只证明一次检索Observation，不证明Frame已物化或模型已看到。

### `KnowledgeContributionObservationV1`

字段：Contribution ID/Revision/Digest、Owner=`knowledge`、Run/Turn/Execution Scope Digest、Retrieval Observation/Result、Query/View/Watermark/Published Snapshot、Authority/Policy/Purpose、Observed/Expires、Coverage、预算摘要、Residual和Items。

Item字段：Rank、Score、Record/Content exact refs、DomainResult exact ref、DomainResultAssociation、SettlementApplication exact ref、Package/Snapshot/Source/Evidence/Projection refs、Citation Digest、License/Trust/Conflict、Token Estimate/Estimator Digest，以及Record/Source/Projection有效期。

不变量：

1. Owner重算DomainResult canonical digest，精确验证Association和SettlementApplication，并验证Owner/Subject Ref/CASAfter；
2. Published Snapshot pointer、Snapshot、Package、Record、Source和Projection必须在Inspect与Frame freeze时均为current；
3. License、Authority、Policy、Purpose、Scope和Sensitivity任一缩小即拒绝；
4. exact content bytes必须匹配Length/MediaType/Digest；
5. `now >= min(Observation, View, Snapshot, Record, Source, Projection, Frame deadline)`即过期；
6. 排序固定为Score降序、Record ID升序、Revision降序、Digest升序；排序、Citation、Coverage或NextCursor篡改均使canonical验证失败；
7. effectful Retrieval/Resolver/远程正文的Unknown只调用Gateway的`InspectOriginalAttempt`并携带原Operation/Attempt/Permit/双Enforcement坐标；Current Reader的`InspectAttempt`只检查本地Owner Journal；
8. Knowledge Contribution只能由Context Owner物化。缺少公共Knowledge fragment kind或Route exact-reference能力时Fail Closed。
