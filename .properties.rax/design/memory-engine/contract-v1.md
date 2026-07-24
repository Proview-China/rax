# Memory领域合同 v1

本文件冻结实现所需对象、版本字段和不变量；字段名为设计合同，不是已授权Go API。

## 1. 共同Envelope

每个写入命令、事实与结果都必须携带或可精确解析：

- `ContractVersion`、`SchemaRef`、`ID`、`Revision`、`Digest`；
- `TenantID`、`IdentityID/Epoch`、`LineageID/PlanDigest`；
- 来源于Run时的`InstanceID/Epoch`、`SandboxLeaseID/Epoch`、`RunID`；
- `AuthorityRef/Epoch`、`PolicyRef/Revision/Digest`、`Purpose`、`ActionScopeDigest`；
- `Created/Updated/ExpiresUnixNano`；
- `Causation`、`CorrelationID`和有界Provenance集合。

空集合与`nil`的Canonical语义必须冻结；集合必须有上限、排序且唯一；正文只通过`ContentRef + ContentDigest + Length + MediaType`寻址。

## 2. 核心对象

### `MemoryCandidateV1`

| 字段组 | 必需内容 |
|---|---|
| 身份 | Candidate ID、Revision、Canonical Digest、Kind、状态 |
| 作用域 | 完整来源Execution Scope、Run ID（如适用）、目标Memory Scope、Subject、Purpose |
| 内容 | Content Ref、Schema、Digest、Revision、长度、候选用途 |
| 来源 | Producer Binding、Source Epoch/Sequence、Evidence Ref、Continuity范围、模型/规则版本（如使用） |
| 治理 | Sensitivity、Confidence表达、Retention/Expiry建议、Policy、Authority、Poisoning评估引用 |
| 变更 | Target Record ID/Expected Revision、Correction/Merge/Forget关系 |
| 时间 | Created、Updated、Expires |

Candidate内容不可变；Admission状态变化放入独立`MemoryAdmissionFactV1`，避免修改原Candidate后让Review和Effect摘要漂移。

### `MemoryAdmissionFactV1`

包含Admission ID、Candidate Ref/Digest/Revision、Owner Binding、Schema/Source/Scope/Sensitivity/Duplicate/Poisoning/Policy检查结果、Decision、Merge Target、Review Requirement、Revision、Evidence与TTL。

Decision闭集：`rejected`、`merged`、`review_required`、`commit_ready`。Admission不是Review Verdict和Commit授权。

### `MemoryRecordV1`

至少包含：

`MemoryID, Revision, Kind, Scope, Subject, ContentRef, ContentDigest, SourceRefs, Confidence, Subjectivity, PolicyBinding, Sensitivity, ValidTime, TransactionTime, CreatedAt, ExpiresAt, Status, CorrectionLinks, ConflictGroupRef, Retention, LegalHoldRef, Digest`。

不变量：

- `MemoryID + Revision`唯一，Revision严格递增；
- Content变化必须改变ContentDigest和Record Digest；
- Current Pointer通过Expected Revision CAS移动；
- 历史Revision不可原地覆盖；
- SourceRefs非空；只有领域Owner Inspect后的Fact才能作为authoritative evidence；
- Tombstone不得携带被忘记的正文。

### `MemoryRefV1`

包含Memory ID、Revision、Record Digest、Scope Digest、可见性级别和Owner Binding；不携带超出View权限的正文。

### `MemoryViewV1`

包含View ID/Revision/Digest、Principal、Authority、Policy、Purpose、Scope Selectors、Subject Filter、Sensitivity上限、Allowed Representations、Snapshot Watermark、Projection Refs、Result Budget、TTL。

### `MemoryCommitAttemptV1`

包含Attempt ID、Operation Subject、Candidate/Admission/Review精确Ref、Intent/Reservation/Operation Admission Receipt/Permit/Begin/Delegation/Prepare/Enforcement/Fence Ref、Expected Record Revision、Idempotency、Conflict Domain、Owner集合、Observation、Evidence、Receipt、Inspection、Settlement、Residual和时间。

状态闭集：`reserved`、`admitted`、`permitted`、`begun`、`prepared`、`executing`、`unknown_outcome`、`observed`、`confirmed_applied`、`confirmed_not_applied`、`settled`、`failed`、`reconciliation_required`。不得跳过Begin后的原Attempt恢复规则。

### `MemoryCommitReceiptV1`

包含Attempt、Record Ref、Applied Revision、CAS Before/After水位、Provider Receipt（可选）、Owner Inspection Ref、Evidence Refs、Disposition、Residual/Cleanup和Digest。Receipt本身不是Runtime Outcome。

## 3. Correction与Forget对象

- `MemoryCorrectionCandidateV1`：精确绑定Base Record Revision、修正正文或关系、原因、来源和Policy；走完整Candidate/Review/Commit链。
- `MemoryForgetRequestV1`：绑定目标Revision集合、Scope、Requester Authority、Reason、Retention/Legal Hold Policy和幂等键。
- `MemoryTombstoneFactV1`：由Owner CAS创建，阻止新View返回正文，保留最小审计Ref。
- `MemoryPurgeAttemptV1`：独立Operation Effect，记录每个Backend/Projection的删除、Unknown、Residual和Legal Hold阻塞。

Forget不等于Purge。API必须分别返回逻辑不可见、物理清除进度与不可清除原因。

## 4. Projection与检索对象

### `MemoryProjectionV1`

字段：Projection ID/Kind、Owner Domain、Source Snapshot/Record范围、Builder Version、Schema、Index Version、Embedding Model/Dimension（适用时）、Content Digest、Coverage、State、Revision、Digest、Created/Updated。

Kind闭集首版：`skill_index`、`lexical`、`vector`、`graph`。State：`building`、`partial`、`ready`、`stale`、`rebuilding`、`failed`、`retired`。

### `RetrievalQueryV1`

包含Query ID/Revision/Digest、Memory View Ref、Purpose、Intent、Lexical/Entity/Time约束、Required Evidence、Conflict策略、Representation、Result/Byte/Token/Latency Budget、Cursor、RequestedAt和TTL。

### `RetrievalCandidateV1`

包含Record Ref、命中Projection、Match Reason、Score Components、Provenance、Conflict Group、Representation Availability和Coverage。Score只用于排序，不代表事实置信度。

### `RetrievalResultV1`

包含Query Ref、View/Projection水位、候选集合、Citation集合、Conflict摘要、Coverage、Dropped Reasons、Next Cursor、Result Digest、Evidence Digest和ObservedAt。它是Context候选，不是Context Frame。

### `CitationV1`

绑定Record ID/Revision/Digest、Source/Evidence Ref、内容范围、当前性、可见Representation和可验证摘要；禁止只返回一个无来源Embedding命中。

## 5. CAS与幂等

- Create Candidate：按`Producer + SourceEpoch + SourceSequence` exact-idempotent；同键换内容为Evidence Conflict；
- Admission：按Candidate Digest和Expected Admission Revision CAS；
- Record Commit：按Record ID和Expected Current Revision CAS；首次创建使用显式`expect_absent`；
- Correction/Supersede：旧Revision与新Revision、Link写入在同一领域事务中线性化；
- Commit回包丢失：只按Attempt/Intent Inspect；
- Query为只读，但Cursor绑定View、Projection水位和Query Digest，水位漂移后旧Cursor失效。

## 6. 错误分类

至少稳定区分：`candidate_rejected`、`source_unverified`、`scope_denied`、`sensitivity_denied`、`poisoning_suspected`、`duplicate_conflict`、`review_missing_or_stale`、`authority_stale`、`fence_stale`、`budget_unavailable`、`revision_conflict`、`commit_unknown`、`inspection_incomplete`、`partial_coverage`、`record_withdrawn`、`forget_blocked_by_legal_hold`、`purge_residual`。

Rejected表示未执行；Indeterminate表示可能已经发生Effect；Partial Coverage是查询覆盖声明，不得混同失败或空结果。

## 7. OperationScope-aware Evidence需求

无Run的管理员Correction、Forget/Purge或组织Scope变更需要`OperationEvidenceScopeV3`：

- 绑定`OperationSubjectV3`的kind、admin/custom operation ID、subject revision与current projection；
- 保留Tenant/Identity/Lineage层级，不要求伪造Run ID；
- authoritative evidence只引用Memory Owner CAS后Fact的ID/Revision/Digest/Payload；
- source epoch/sequence、causation/correlation和append-only ledger规则不变；
- 时点严格为Owner CAS形成DomainResultFact后、Runtime Operation Settlement前；Run路径继续使用既有Evidence合同。

## 8. 大小、排序与严格解码

- 所有集合有显式上限、稳定排序和唯一键；
- Canonical Digest使用domain/version/discriminator隔离；
- 严格解码拒绝重复键、未知字段和尾随文档；
- 大型正文只用Ref；Opaque Extension必须版本化，不能忽略未知治理字段。

## 9. per-turn Context Source合同需求

本节的owner-current只读合同由[MemoryContextSourceCurrentReaderV1设计Delta](./context-source-port-v1.md)冻结，只允许本地`InspectAttempt/InspectForTurn/ReadContentExact`；Reader、Memory Adapter、Application三阶段Port与Context nonzero reference fixture均已YES。production G6B/root仍未装配。`Retrieve`与远程正文读取属于[Memory Retrieval Domain Gateway Delta](./retrieval-domain-gateway-v1.md)，当前继续unsupported且Provider=0；Checkpoint/Restore使用另行合同。

### `MemoryRetrievalObservationV1`

字段：Observation ID/Revision/Digest、Owner、Retrieval Attempt Ref、Query/View/Watermark exact refs、Result Ref、Result/Evidence Digest、Coverage、Next Cursor、按稳定顺序保存的Hit refs、Observed/Expires和Residual。它是可Inspect Observation，不是Memory Record、Context Candidate或Frame。

### `MemoryContributionObservationV1`

字段：Contribution ID/Revision/Digest、Owner=`memory`、Run/Turn/Execution Scope Digest、Retrieval Observation/Result、Query/View/Watermark、Authority/Policy/Purpose、Observed/Expires、Coverage、预算摘要、Residual和Items。

Item字段：Rank、Score、Record exact ref、Content Ref/Digest/Length/MediaType、DomainResult exact ref、DomainResultAssociation、SettlementApplication exact ref、Source/Evidence/Projection refs、Citation Digest、Token Estimate/Estimator Digest和Record Expires。

不变量：

1. Owner按当前State Plane重算DomainResult canonical digest，并验证Association精确绑定相同ID/revision/digest；
2. DomainResult Owner、Subject Ref、CASAfter及SettlementApplication必须与当前Record revision一致；
3. current查询要求Watermark和Record current pointer未漂移，Record仍为active且未Correction/Tombstone/Withdraw；
4. Context物化时复读exact content bytes并重算Length/MediaType/Digest；任何不一致拒绝；
5. 所有TTL取最小值；`now >= Expires`拒绝；
6. 排序固定为Score降序、Record ID升序、Revision降序、Digest升序；任何换序都改变canonical digest；
7. effectful Retrieval或远程正文丢回包只调用Gateway的`InspectOriginalAttempt`并携带原Operation/Attempt/Permit/双Enforcement坐标；Current Reader的`InspectAttempt`只检查本地Owner Journal；
8. Contribution只能被Context Owner转为Candidate并冻结Frame，Memory不得声明最终注入成功。
