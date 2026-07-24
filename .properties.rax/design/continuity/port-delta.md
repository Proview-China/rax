# Continuity Port Delta

状态：**Runtime Checkpoint-first V2、C-01/C-02、C-03 RestorePlan V2、Restore最小公共参考纵切、C-05/C-06/C-07/C-08 owner-local治理、Continuity持久化与只读SDK均已落地**。production trusted Assembler、跨Owner全量Participant、远程Provider与root仍未闭合。

## 1. 使用规则

- 本文件是联合评审输入，不是Runtime/Harness代码变更授权。
- Runtime、Harness、Application公共Delta只能由对应Owner串行合入。
- Continuity不得在自身目录复制Runtime类型、私建兼容接口或把legacy Port包装成完整能力。
- 每项必须经Owner确认version、Schema、Capability和Conformance后才能进入实现计划对应阶段。

## 2. 组件自有公共Port

这些Port由Continuity语义Owner定义。C-02领域合同和reference backend已经实现；Application/Assembler接线与其余跨Owner能力仍须各Owner独立验收。

### C-01 Timeline Projection Governance Port V1

- **用例**：把Runtime Evidence Owner可精确复读的Evidence Record与领域Owner可证明current的Fact关联为可重建Timeline Projection；提供Attempt create/Inspect/CAS、historical Event Inspect、current Projection Inspect、Query/Watch，不建立第二Ledger。
- **语义Owner**：Continuity Timeline/Projection Attempt/Event/Query Owner；Runtime Evidence Ledger Owner仍唯一拥有Evidence Record、source registration、ledger sequence与摘要链；各领域Owner仍唯一拥有自身Fact及其current结论。
- **输入闭集**：production `TimelineProjectionRequestV1`只包含stable Attempt ID/idempotency key、`EvidenceSourceKeyV2`、可选expected `EvidenceRecordRefV2`（仅期望坐标）、仅`authoritative_fact`允许的Owner Fact exact ref、Projection Policy ref、受验Execution Scope与`RequestedNotAfter`。expected Attempt ref/revision只属于CAS方法参数，不进入Request。Request不得携semantic/causal/object refs、payload、Observed/Recorded time、可信bool、sequence、ledger/record/candidate/chain/payload digest、Trust/current或Owner inspection结论。
- **输出**：`TimelineProjectionAttemptFactV1` exact ref与中立历史projection、immutable `TimelineEventRecordV1` exact ref、独立`TimelineProjectionCurrentV1`短时readability/current-binding projection、独立`TimelineProjectionTombstoneFactV1`/Visibility overlay ref、`TimelineCursorV1`、bounded page/watch/Rebuild item result。Request、Attempt历史、Event历史都不携带或暗示production current；current-binding projection也不改变Ledger TrustClass。
- **Attempt状态与CAS**：`proposed -> inspecting -> admitted -> visible`；可恢复状态为`reconcile_required`，失败终态为`rejected | expired | indeterminate`。Attempt只保存Request闭集坐标digest、各Owner exact refs/immutable sealed S1/S2 projections、Reader派生Event digest、共同`checked_at/not_after`、状态与结果Event ref，不保存caller admission、semantic/payload/causal内容或caller sequence/digest/Trust/current副本。Create-once同ID同canonical请求坐标幂等、同ID换任一source/Owner/Policy/Scope/bound为Conflict；所有转移使用包含完整previous Fact digest的expected exact revision CAS。已前进revision重放旧CAS必须Conflict，不能因next内容曾出现而幂等回退或形成ABA。
- **Controller唯一序列**：`Create Attempt -> S1 Record双读 -> R-CTY-06 current/readability/tombstone -> authoritative_fact按需Owner current -> S2 exact -> fresh ValidateCurrent/aggregate TTL -> atomic Event + Attempt visible + Continuity projection current index`。S1先`InspectBySource(SourceKey)`，再以返回exact Ref执行`InspectRecord`并要求完整Record相等；S2使用相同bindings重复全部Reader并逐字段exact比较。fresh只在S2后读取一次now。Event semantic/causal/payload/sequence/digest/Trust全部从S2 Owner结果派生。原子publish全有或全无；不能证明时保持`admitted/reconcile_required`且查询不可见。
- **六类Trust路由**：Runtime `EvidenceTrustClassV2`六值逐项闭合。`observation | late_observation | receipt | attestation | claim`都原样保持Ledger TrustClass进入Timeline observation/projection，禁止调用generic Owner Fact Reader升级为`authoritative_fact`或authoritative current；其中`claim`不等于Run终态，`receipt`不等于领域Fact。只有`authoritative_fact`允许且必须携Runtime Record已绑定的Owner Fact exact ref，并通过按`owner contract/schema/fact kind`路由的`TimelineOwnerFactCurrentReaderV1`。若某领域要对attestation/claim另做current证明，必须发布独立typed Delta/Fact projection并保持原Ledger TrustClass，不能修改Runtime trust语义。
- **Owner Fact S1/S2**：仅`authoritative_fact`执行。S1/S2必须使用同一Owner Reader binding，并Inspect同一immutable sealed projection；逐字段exact比较完整Fact ref、Owner binding、Scope、revision/digest、state/currentness、Authority/Policy/Binding、自然`CheckedUnixNano`、自然`ExpiresUnixNano`与`ProjectionDigest`。Owner Reader不接收`RequestedNotAfter`，该值不得进入Owner sealed projection或ProjectionDigest；Reader也不能在每次读取时用fresh now重封新projection，fresh now只用于`ValidateCurrent(now)`。调用方自报`InspectedByOwner/current/accepted`一律不构成生产证据。
- **TTL/current顺序**：先校验`RequestedNotAfter`：`==0`表示caller不增加上限，`<0`为`invalid_argument`，`>0`只能缩短但暂不传给任何Owner Reader。随后Evidence/Owner Readers各自返回与caller无关的自然sealed Checked/Expires/Digest；Continuity完成S1/S2 exact比较与fresh-now `ValidateCurrent`后，先取Projection Policy、Runtime Evidence source/policy/readability/tombstone-absence、仅`authoritative_fact` Owner Fact及其Authority/Scope/Binding全部自然上限的最小值，最后才按正数`RequestedNotAfter`截短为Attempt共同`NotAfter`。caller bound不得改变任何Owner ProjectionDigest。没有Owner定义上限、无法复读readability/tombstone、`now >= NotAfter`、时钟回拨或任一S1/S2漂移均Fail Closed；禁止用Event时间、Cursor TTL、Retention window或自定默认TTL伪造current。historical Event永久只表示当时投影，不表示当前仍可读或仍权威。
- **Rebuild**：production Rebuild只接受Request闭集或Request refs，并逐项调用同一Controller完整序列；不得接受caller Candidate/Event/Store record，不得直调`PutProjection`/`ReplaceLedgerScope`，不得bulk覆盖Ledger Scope、Tombstone overlay或history。每项独立Attempt/Inspect/Residual，unknown只收敛原Item。
- **历史不可变/Tombstone**：Event create后revision/digest/bytes永久不变。Tombstone必须create独立immutable Fact并在同一Continuity Owner事务CAS Visibility/current index；Query/Watch组合Event history与overlay。禁止`TombstoneProjection`原地改Event的`Visibility/TombstoneRef`，也禁止Rebuild删除overlay后复活Event。
- **不变量**：ledger scope/sequence/record/candidate/chain/payload digest、Trust及Event semantic/causal内容全部从Runtime Reader派生；同Evidence/source同内容幂等、换内容Conflict；authoritative投影必须独立复读Owner Fact；S2/fresh之后才可原子CAS可见；查询受Authority/Policy/current/Tombstone水位过滤；Continuity不成为Model/Tool/其他领域identity Owner。
- **Effect/Recovery**：Projection、Rebuild Item与Tombstone都是Continuity派生Fact CAS，不产生第二Operation Effect。Create/CAS/commit回包丢失只Inspect原Attempt/Tombstone exact ref；`unavailable | indeterminate`不得当NotFound、换ID或盲重试。Projection可从Evidence重建；仅`authoritative_fact`还复读Owner Fact，但每项重建必须重新执行完整Controller序列和current门禁。
- **closed errors**：`invalid_argument | not_found | conflict | precondition_failed | unavailable | indeterminate | unsupported`为C-01唯一公开分类；Runtime Adapter仅做到`core.ErrorCategory`的闭合映射，不复制Runtime reason语义。只有Owner线性化Reader明确返回exact absent才是`not_found`；typed-nil、超时、后端无覆盖或无法判定均为`unavailable/indeterminate`。
- **反例**：见`acceptance.md` CTY-TL-P0-01..03与CTY-TL-01..33；核心包括caller可信字段/semantic/payload注入、expected Ref直接当Record、缺Attempt或漏S1/S2/current/fresh、三对象非原子、lost reply换ID/ABA、Rebuild批量Candidate直写/Replace Scope/覆盖Tombstone、Tombstone原地改Event/digest/bytes、overlay删除复活与tombstoned payload泄漏。
- **兼容影响**：`praxis.continuity.timeline/v1alpha1`旧caller Candidate服务已显式降名为`ReferenceTimeline`；public Store已移除`ReplaceLedgerScope`/`TombstoneProjection`，Tombstone使用immutable Fact+overlay。production形状只允许`TimelineProjectionAdapterV1`的coordinate-only Project/Rebuild，不升级legacy `TimelinePort`、不做V1/V2双写。真实typed Owner Reader与Application root仍须跨Owner装配。

冻结方法语义（最终Go签名仍须联合Review）：

```text
CreateTimelineProjectionAttemptV1(request_coordinates, expect_absent)
InspectTimelineProjectionAttemptV1(exact_attempt_ref)
InspectCurrentTimelineProjectionAttemptV1(tenant, scope, attempt_id, owner_binding)
CompareAndSwapTimelineProjectionAttemptV1(expected_exact_ref, next)
AdmitTimelineProjectionV1(expected_attempt_ref)
InspectTimelineEventV1(exact_event_ref)
InspectTimelineProjectionCurrentV1(exact_event_ref, requested_not_after)
RebuildTimelineProjectionsV1(request_refs) -> per-item exact results
CreateTimelineProjectionTombstoneV1(exact_event_ref, policy_basis, idempotency_key)
InspectTimelineProjectionTombstoneV1(exact_tombstone_ref)
QueryTimelineV1(query, cursor)
WatchTimelineV1(query, cursor)
```

### C-02 `CheckpointManifestGovernancePortV2`

- **用例**：持久化可CAS的Checkpoint Manifest Fact，并在引用闭包冻结后create-once immutable Manifest Seal，供Runtime只读复读。
- **语义Owner**：Continuity Manifest/Seal Owner；Runtime只拥有Checkpoint Attempt/Barrier/EffectCut/Consistency/Finalization，不持Continuity Fact Store写口。
- **输入**：Manifest candidate、expected exact revision、受验`TenantID + ExecutionScopeDigest + Continuity Owner Binding`、exact Runtime CheckpointAttempt/Barrier/Effect Cut refs、Timeline Cut、Context Generation/Frame refs、Application/Runtime Attempt refs、opaque Runtime Settlement refs、Memory Watermark/View/Projection refs、Knowledge Snapshot/View/Projection refs、Participant Inspect/Evidence/Diagnostic/Residual Fact refs与frozen-ref-set digest。
- **输出**：collecting/verified_candidate/partial/indeterminate/rejected Manifest Fact、immutable revision 1 Manifest Seal Fact及精确ref/projection；该状态集没有兼容别名。
- **不变量**：所有跨Owner引用绑定`contract/schema + owner binding + TenantID + ID + revision + digest + ScopeDigest`；exact identity key必须是包含完整`OwnerBinding`全部字段的可比较结构，不得以允许`|`等分隔符进入字段的裸字符串拼接，也不得只绑定`Owner.ComponentID`。Manifest递归检查Context/Memory/Knowledge/Attempt/Settlement/Inspection/Participant/Snapshot/Coverage/Evidence/Diagnostic/Residual全部exact refs与Manifest Execution Scope同Tenant同Scope；聚合顶层及Attempt、Participant、任意severity Diagnostic的全部Residual，`verified_candidate`要求聚合结果为空。Seal exact绑定current Manifest、CheckpointAttempt、Barrier、EffectCut、frozen/required/runtime Participant set、每个RuntimeClosure ref及Context/Artifact closure digest；Seal revision固定为1且历史不可覆盖；同Seal ID同canonical内容幂等返回，同ID换任一binding为Conflict；Manifest/Seal任何状态都不等于Runtime consistent。
- **Effect/Recovery**：Manifest使用create-once/CAS；Seal仅create-once/Inspect。Repository在同一事务内复读current Manifest并重验exact revision、`verified_candidate`、Owner及全部Seal binding，不能绕过Controller写入。current/history/seal以结构化`TenantID + ScopeDigest + ID`分区，current Reader还必须携受验Continuity Owner。任一回包丢失按原Manifest或Seal exact ref Inspect；已前进后重放旧CAS必须Conflict，不能按幂等回退或形成ABA。Unavailable/Indeterminate不能当NotFound并改ID。远程Blob动作另走已登记公开Operation合同。
- **反例**：Continuity自行标consistent；delimiter collision；`OwnerBinding`任一字段漂移仍命中同identity；跨Tenant/Scope splice或跨Tenant Seal串读；已有Settlement的Attempt残留Residual、Participant Residual或nonblocking Diagnostic Residual仍进入`verified_candidate`；直接Repository Seal绕过current/Owner/binding检查；只写Manifest未Seal便提交Consistency；Seal revision不是1；同Seal ID换Manifest/Attempt/Barrier/EffectCut/Participant closure；lost reply后创建第二Seal；历史CAS在current继续前进后被误判幂等；通过CAS覆盖历史Seal；裸字符串或Memory/Knowledge正文进入V2。
- **兼容影响**：新增V2组件合同，不改变`core.CheckpointSet`；V1裸字符串Manifest只能作为诊断历史资产，不能默认补值迁移。

已实现公共Port：

```go
	type CheckpointManifestGovernancePortV2 interface {
		CreateCheckpointManifestV2(candidate, expectAbsent)
		InspectCheckpointManifestV2(exactManifestRef)
		InspectCurrentCheckpointManifestV2(tenantID, scopeDigest, manifestID, ownerBinding)
		CompareAndSwapCheckpointManifestV2(expectedExactRef, next)
		CreateCheckpointManifestSealV2(request)
		InspectCheckpointManifestSealV2(exactSealRef)
	}
```

`CreateCheckpointManifestSealV2`请求至少包含稳定Seal ID/idempotency key、expected Manifest revision、exact Manifest ID/revision/digest、Attempt/Barrier/EffectCut refs、frozen-ref-set digest、required Participant set digest与canonical Participant closure refs。返回的`CheckpointManifestSealFactV2`必须是immutable revision 1；`InspectCheckpointManifestSealV2`返回中立projection，Runtime Consistency只保存该Seal ref。

### C-03 Fork/Rewind/Restore Plan Fact Port V2

实现状态：**Restore owner-local + Runtime current Adapter completed；Workspace-only Rewind owner-local completed**。live Restore包含exact shape、closed state、TTL/current、create-once/CAS/history/current、内存与SQLite Repository、只读SDK及`runtimeadapter.RestorePlanCurrentReaderV2`。live Rewind V2包含Checkpoint/Manifest、Sandbox View/keep-drop/planned ChangeSet、Dependency/Review/irreversible Effect/Residual exact refs、closed state、TTL/current、create-once/CAS/history/current、lost-reply Inspect、内存与SQLite schema v9 Repository及只读SDK；两者都不创建Runtime/Sandbox Fact。

- **用例**：创建、Inspect、CAS版本化Plan，并输出提交Application的不可变Plan ref。
- **语义Owner**：Continuity Plan Owner。
- **输入**：Plan candidate、source Runtime immutable `CheckpointConsistencyFactV2`/Continuity `CheckpointManifestSealRefV2`、frozen-ref-set digest、source scope、new Instance/higher Epoch proposal、Required Participant set digest、Context Generation/Frame requirement refs、compatibility/currentness requirement shapes、Review/Authority/Budget/Binding prerequisite shapes、stable Conflict Domain、Residual policy、TTL与expected revision。
- **输出**：Restore为`draft|checkpoint_inspected|compatibility_inspected|admitted|submitted|rejected|expired`；Rewind为`draft|workspace_inspected|dependencies_inspected|admitted|submitted|rejected|expired`。两者的`submitted`都只表示exact Plan ref可交给Application。
- **不变量**：Plan不执行Effect、不创建Instance、不继承父级全部Authority；Restore只接受Runtime immutable `CheckpointConsistencyFact` ref，但该Fact不代表当前Restore资格；Plan携带Review requirement/candidate ref，不在Application Intent前形成或解释current Review Verdict；Continuity不能写`RestoreEligibilityFact`、Review Verdict、Runtime Authorization、Permit、Fence或RestoreAttempt。
- **Effect/Recovery**：Plan Fact只执行create-once/CAS/Inspect；Runtime已实现create-once Attempt/Identity Reservation、short-TTL Eligibility、history/current Inspect与CAS。Plan/Attempt/Eligibility写回包丢失均只Inspect原ID/revision/digest；Eligibility current会复读Plan与prerequisite current。Application/Runtime/Sandbox/Context公开参考组合已补齐Action route、Admission、Authorization、Evidence/Settlement、Stage、Context与Activation；Continuity仍不执行这些Effect。
- **反例**：Approved Rewind直接改文件；submitted Restore Plan直接调用Participant；Plan被解释为current Eligibility、Review Verdict或Runtime运行Port；RestorePlan复活旧Instance；Fork自动继承Tool权限。
- **兼容影响**：新组件合同；SDK/CLI写命令必须经Application namespaced workflow。

候选方法语义：

```text
CreateRestorePlanV2(candidate, expect_absent)
InspectRestorePlanV2(exact_plan_ref)
InspectCurrentRestorePlanV2(tenant_id, scope_digest, plan_id, owner_binding)
CompareAndSwapRestorePlanV2(expected_exact_ref, next_state)

CreateRewindPlanV2(candidate, expect_absent)
InspectRewindPlanV2(exact_plan_ref)
InspectCurrentRewindPlanV2(tenant_id, scope_digest, plan_id, owner_binding)
CompareAndSwapRewindPlanV2(expected_exact_ref, next_state)
```

### C-04 Continuity Content/Journal Port V1

- **用例**：Backend无关地保存内容寻址Object/Chunk与SQLite↔RocksDB跨存储Journal。
- **语义Owner**：Continuity Content Fact Owner。
- **输入**：Object Manifest、Chunk stream/ref、expected journal revision、Retention/Encryption refs。
- **输出**：Object ref、Journal Fact、Inspect结果、integrity report。
- **不变量**：content staged且digest匹配后才提交current ref；RocksDB存在不等于领域事实；无引用闭包证明不得回收。
- **Effect/Recovery**：本地写入由WAL/Journal恢复；远程Put/Delete走Operation V3；Unknown只Inspect同Object/Attempt。
- **反例**：SQLite current ref指向缺失Chunk；压缩后未重算摘要；孤儿内容被Checkpoint引用仍删除。
- **兼容影响**：Backend SPI；SQLite+RocksDB是默认实现，不进入公共业务Schema。

### C-05 Artifact Relation Governance Port V1

- **用例**：把Artifact Owner密封的Artifact历史描述与Context Frame、Workspace Change、Review、Tool Result、Effect或Checkpoint exact ref保存为Continuity不可变因果关系。
- **语义Owner**：Artifact正文/revision/current/storage语义仍属于Artifact Owner；Related Fact仍属于对应领域Owner；Continuity只拥有`ArtifactRelationFactV1`及Artifact/Related历史索引。
- **输入闭集**：stable Relation ID/idempotency key、期望Execution Scope、Artifact Fact exact ref、Related Fact exact ref、closed relation kind、Evidence Record ref和可选expected source projection ref。caller不得携storage digest、parent、origin digest、source projection digest、current/Trust/Verdict/Outcome。
- **Owner Reader**：`ArtifactRelationSourceReaderV1`是consumer-side typed seam；生产实现必须路由到Artifact Owner。Reader返回Owner密封`ArtifactRefV1`、Related exact ref、relation kind、Evidence exact digest及source projection exact ref。source projection必须与Artifact Fact共享完整BindingSet/Component/Manifest/Artifact owner identity，仅Capability/Fact kind可按typed projection不同。
- **Controller唯一序列**：按稳定坐标先Inspect既有Relation以收口lost reply；不存在时执行S1 Timeline exact Event+typed source projection→逐字段exact比较→S2重复→S1/S2全量canonical一致→create immutable revision-1 Relation+两个索引。Timeline Event必须exact绑定同Evidence digest/Execution Scope并引用Artifact与Related坐标。
- **不变量**：结构化`TenantID+ScopeDigest+RelationID`隔离；same-ID/same-idempotency exact重放返回原Fact，换内容Conflict；历史Fact无CAS更新、无current语义、无ABA；返回值深拷贝。typed Router缺失、typed-nil、Reader无覆盖或未分类错误时`unsupported|indeterminate`且零Fact。
- **Effect/Recovery**：仅Continuity本地create-once Fact，不产生外部Effect。commit回包丢失只Inspect原Relation ID；不得换ID重建或重读Provider。
- **反例**：见`acceptance.md` CTY-ART-01..06；尤其caller伪造storage/parent、S1/S2漂移、跨Tenant splice、Owner路由错绑、Relation升级为Artifact current或其他Owner事实。
- **兼容影响**：owner-local合同、内存/SQLite repository和只读SDK已经实现；真实Artifact Owner Adapter、Application/Assembler route和production `AttachArtifact`仍需各Owner联合Review，不升级旧ObjectRefs字符串或generic Owner Reader。

### C-06 Content Integrity Audit Governance Port V1

- **用例**：对调用方明确给出的Object/Journal坐标执行可重复、可审计的跨Metadata/Content完整性诊断，形成immutable revision-1 Audit Fact。
- **语义Owner**：Continuity拥有Object Manifest、Write Journal与Audit Fact；Content backend只返回读取Observation。跨Owner引用、Checkpoint/Fork/Review current与删除资格仍属于各自Owner/未来协调链。
- **输入闭集**：stable Audit ID、Idempotency Key、Execution Scope、bounded Subject集合；每个Subject仅含Object ID、Journal ID与可选expected Manifest digest。caller不得提交可信visibility、Journal state、Chunk结果、classification、residual或cleanup结论。
- **输出**：exact `ContentIntegrityAuditRefV1`、不可变Fact、逐Subject/Chunk closed finding和`healthy | attention_required | indeterminate`聚合结论。`healthy`只证明本次明确坐标在两轮检查中的一致性。
- **唯一序列**：先按Audit ID Inspect收口lost reply；不存在时对每个Subject执行S1 Object+Journal+Chunk读取→S2同构重读→canonical exact比较→create-once Fact。missing/corrupt/unknown分别闭合为`dangling_reference | corrupt_content | indeterminate`；任何S1/S2漂移不得降级为healthy。
- **不变量**：结构化Tenant/Scope/Audit ID隔离；same-ID/same-idempotency exact replay；换Subject/expected digest Conflict；Fact/history不可CAS覆盖；Reader深拷贝。Audit Repository不暴露cleanup/purge写口。
- **Effect/Recovery**：仅本地诊断Fact写入；无外部Effect。commit回包丢失只Inspect原Audit；Metadata/Content unavailable记录indeterminate且不触发重写或Provider调用。
- **反例**：有限Subject healthy被解释成全库无孤儿；缺Chunk仍返回正文；corrupt bytes仅信key存在；Audit自动推进Journal/Retention或删除Chunk；lost reply换ID重扫；S1/S2漂移仍封healthy。
- **兼容影响**：owner-local additive合同、内存repository及SQLite（自schema v6引入，当前schema v9）、lost-reply fake与只读SDK已经实现。Content keyspace枚举、orphan reference closure、physical purge、remote archive/provider仍unsupported，不能由该Port扩权。

### C-07 Content Delta Governance Port V1

- **用例**：从两个已可见、完整可读的Content Object派生immutable结构共享关系，明确target recipe中的reused/added及base-only removed Chunk。
- **语义Owner**：Continuity拥有Object Manifest、Chunk内容寻址语义与`ContentDeltaFactV1`；caller不拥有Chunk分类，Compactor/Purge及跨Owner引用资格不在本Port。
- **输入闭集**：stable Delta ID、Idempotency Key、Execution Scope、Base/Target Object ID与各自expected Manifest digest。禁止caller提交Chunk refs、reuse bool、统计、payload或预制Fact。
- **输出**：exact Base/Target Object ref、ordered target recipe、normalized reused/added/removed Chunk refs、共享/新增/移除bytes及immutable Delta ref。
- **唯一序列**：先Inspect原Delta收口lost reply；不存在时S1 Base+Target Manifest/visibility+全Chunk bytes→逐Chunk及完整Object digest校验→S2同构重读→exact compare→create revision-1 Fact。
- **不变量**：只有`schema+digest+length`全等的Chunk可reuse；两Object必须同Scope且visible；结构化Tenant/Scope/Delta ID隔离；same-ID/idempotency exact replay，changed Conflict；历史不可覆盖、Reader深拷贝。
- **Effect/Recovery**：只写Continuity本地关系Fact；不创建/改写Object、不执行Compaction、不删除Chunk。lost reply只Inspect原Delta；读取unknown直接Fail Closed。
- **反例**：caller伪造reuse；仅digest相等但schema/length不同仍复用；缺/坏Chunk仍封Delta；S1/S2漂移；用Delta授权删除base；把Delta当可执行patch或Checkpoint Snapshot。
- **兼容影响**：owner-local additive合同、内存/SQLite（自schema v7引入，当前schema v9）repository、lost-reply fake与只读SDK已经实现；production remote/object Provider、Compactor、Purge与跨Ownerroot仍unsupported。

### C-08 History Derivation Candidate Governance Port V1

- **用例**：把既有Timeline immutable Event集合与已可见Content Object绑定成`projection | summary | index`候选，供未来受治理整理流程Inspect。
- **语义Owner**：Continuity拥有Candidate关系Fact；Timeline历史Event仍不可变，output正文仍是Content Object；领域Fact/Memory/Knowledge/Review/Run current不归本Port。
- **输入闭集**：stable Candidate ID、Idempotency Key、Execution Scope、closed kind、ordered Evidence Record ref+expected Record/Projection digest、output Object ID+expected Manifest digest。禁止caller提交Event payload、authoritative/current、summary correctness或删除计划。
- **输出**：ordered exact Timeline Event refs、exact Content Object ref、kind、source-set digest与immutable Candidate ref；Authority固定candidate-only。
- **唯一序列**：先Inspect原Candidate收口lost reply；不存在时S1逐Event exact Inspect+output Manifest/Chunk完整读取→S2同构重读→canonical compare→create revision-1 Fact。
- **不变量**：全部source与output同Execution Scope；source顺序稳定且无重复；same-ID/idempotency exact replay，changed Conflict；历史Event和output Object零改写，Reader深拷贝。
- **Effect/Recovery**：只写Continuity本地Candidate Fact；无模型/网络/Compaction/Purge Effect。lost reply只Inspect原Candidate；任一source/output unavailable或漂移时零Fact。
- **反例**：Candidate冒充Timeline current/领域Fact；批量replace Scope；修改Event bytes；source跨Scope；output缺/坏Chunk；lost reply换ID；Candidate直接触发Purge。
- **兼容影响**：owner-local additive合同、Controller、内存/SQLite（自schema v8引入，当前schema v9）repository、lost-reply fake、只读SDK及unit/blackbox/fault/conformance已实现；真正Compactor/Indexer/Consolidator执行、算法评估、预算/Review、production root与Purge仍unsupported。

## 3. Runtime公共Port Delta

### R-CTY-01 Checkpoint phase Scope、Evidence与Settlement第一波live参考合同

- **用例**：只为`checkpoint_prepare | checkpoint_commit | checkpoint_abort`提供Participant phase治理、Evidence V1 current consumption与Settlement V5；Restore branch只允许decode/ValidateShape。
- **语义Owner**：Runtime Evidence与Settlement Owner；Continuity只提供Manifest/Seal Fact和opaque refs。
- **输入**：exact CheckpointAttempt/Barrier/EffectCut、Participant Reservation/phase、Operation/Effect/Intent/Dispatch Attempt、Admission、Authorization、Permit、prepare/execute Enforcement、Assembly route、Generation、Lease/Fence、Evidence Policy、source coordinate、schema/payload与TTL。
- **输出**：Checkpoint phase Qualification/Handoff/Consumption refs、Evidence Record/association及Settlement V5 opaque ref。
- **不变量**：Evidence sequence单主；只有`consumed_current`可进入V5；V5与V3/V4共享`(TenantID, EffectID)`terminal guard；V5不选择Checkpoint Consistency。Restore shape不得进入ValidateCurrent、Issue、Handoff、Consume或Settle。
- **Effect/Recovery**：Checkpoint Provider回包Unknown只Inspect原Participant phase/provider coordinate，不换Attempt/Effect。Restore最小公共链的Unknown同样只Inspect原Enforcement、Participant、Evidence、Settlement、Context或Activation attempt，不换ID、不重派Effect。
- **反例**：把checkpoint phase Evidence用于Restore；在Evidence V3闭表加Restore；Observation consumption进入V5；建立独立V5 terminal store；Restore shape触发Provider。
- **兼容影响**：checkpoint-first V1/V5与Restore专用Stage/Evidence/Settlement/Activation公开合同并存；不得把checkpoint phase合同、legacy或transport kind包装成Restore资格，也不代表production接线完成。

### R-CTY-02 Checkpoint Fact、Barrier与Effect Cut V2

- **用例**：Runtime以一个Owner事务原子create-once Attempt+Barrier bundle，冻结EffectCut，并在成功或非成功终结时原子关闭Barrier。
- **语义Owner**：Runtime Checkpoint Fact Owner；Continuity只提供ManifestFact与immutable ManifestSeal。
- **输入**：Checkpoint workflow、Execution Scope/Run/Generation/Binding、Required Participant certification、Barrier policy/TTL、exact Participant closures、EffectCut、`CheckpointManifestSealRefV2`与expected revisions。
- **输出**：`CheckpointAttemptBarrierBundleV2`、immutable EffectCut；成功返回`CheckpointConsistencyCommitBundleV2`，非成功返回`CheckpointAttemptFinalizationBundleV2`。
- **不变量**：不公开独立`AcquireBarrier`或`CloseBarrier`；Attempt+Barrier全有或全无。成功事务原子写Attempt consistent、Barrier closed与immutable revision 1 Consistency；Consistency结论只能`consistent`。`incomplete | aborted | indeterminate`只属于Attempt Finalization并原子关闭Barrier，不创建Consistency。Runtime Consistency只绑定immutable ManifestSeal，不绑定mutable Manifest/candidate/RestorePlan。
- **Effect/Recovery**：create/commit/finalize回包丢失只Inspect原bundle/Attempt/Barrier/Consistency；Unknown不换Checkpoint ID。Commit与Finalize并发只有一个线性化赢家。
- **反例**：Attempt持久而Barrier缺失；调用独立AcquireBarrier；partial/indeterminate/rejected Consistency；mutable Manifest冒充Seal；两个并发Commit/Finalize都成功。
- **兼容影响**：新增V2 Fact/Governance Port；legacy `core.CheckpointSet`与Foundation参考协调器保持restricted，不原地扩权。

live `CheckpointConsistencyFactV2`最小字段：`contract/schema/version + Runtime owner binding + fact ID/revision/digest=revision1 + exact CheckpointAttempt/Barrier/EffectCut/ManifestSeal refs + Required Participant closure-set digest + frozen-ref-set digest + created_at + conclusion=consistent`。该Fact create-once/immutable；非成功Finalization不生成它，也不允许以CAS补TTL/currentness。

该最小跨Owner Delta已冻结并形成reference实现：Runtime `CheckpointManifestSealContractVersionV2=2.1.0`以`CheckpointExternalExactFactRefV2`携Continuity完整`contract/schema + OwnerBinding + TenantID + ScopeDigest + ID/revision/digest`，`InspectCheckpointManifestSealRequestV2`同时携Runtime current Participant Set digest与sorted exact `CheckpointParticipantClosureRefV2`。唯一Participant映射由`DeriveCheckpointParticipantClosureExactRefV2`生成immutable revision 1 exact ref，完整Provider Owner binding参与比较，禁止Application私有映射、扫描ID或字符串拼接identity。Continuity Seal新增`RuntimeClosureRef`、`RuntimeParticipantSetDigest`、`ContextClosureDigest`与`ArtifactClosureDigest`；Context digest只覆盖exact Generation+normalized Frame refs，Artifact digest只覆盖normalized Memory/Knowledge/Snapshot/Coverage refs，所有SHA-256只允许经公开canonical normalization进入Runtime `core.Digest`。`runtimeadapter.CheckpointManifestSealReaderV2`只读exact Seal并逐字段校验Manifest/Attempt/Barrier/EffectCut/frozen set/Participant closure，Runtime Gateway在Consistency CAS前完成S1/S2复读与current Participant closure比较。该reference链不授Checkpoint capture、Participant执行、Provider、Restore或production root资格。

### R-CTY-03 Restore Reservation/Eligibility最小Runtime Delta（reference implemented）

- **用例**：从Continuity exact submitted Plan与Runtime immutable Consistency创建唯一RestoreAttempt，并在Runtime同一线性化边界保留fresh Instance/high Epoch/new Lease/Fence；随后Issue/Bind短TTL Eligibility。
- **语义Owner**：Continuity拥有RestorePlan；Runtime拥有Attempt、Identity Reservation、Eligibility及其history/current/CAS。Plan中的Identity仅为proposal，Runtime create成功前不具权威性。
- **公开输入输出**：`CheckpointRestoreOperationScopeV2`、`RestoreAttemptFactV2/RefV2`、`RestoreEligibilityFactV2/RefV2`、`RestorePlanCurrentReaderV2`、`RestoreEligibilityInputsCurrentReaderV2`与`RestoreGovernancePortV2`。Eligibility只绑定Review target/requirement/policy basis及各类requirement exact refs，不含accepted Verdict/Authorization/Permit。
- **S1/S2与恢复**：Attempt create前Plan S1/S2；Eligibility bind前Plan+inputs S1/S2；current Inspect复读Plan、reserved Attempt history和inputs，并在返回前复读Attempt/Eligibility current。changed-content、target Instance/Lease重复、stale CAS与ABA均Conflict；lost reply只Inspect原Attempt/Eligibility，不换identity。
- **当前输出上限**：create-once Attempt/Reservation、short-TTL Eligibility、history/current Inspect/CAS；不产生Admission、Authorization、Permit/Begin、Stage、Evidence/Settlement Restore结果或Activation。
- **硬门禁**：Restore Action Gateway、Evidence/Settlement、Context materialization、Stage/Activate公共参考实现已存在；production trusted Assembler/current Reader、跨Owner全量Participant、远程Provider与root仍未实现。任何绕过exact Owner链的调用必须unsupported/fail closed。
- **反例**：历史Consistency/ManifestSeal直接授current资格；Plan触发Issue/Bind或Provider；checkpoint phase V1/V5用于restore-stage；legacy `RestoreRequest/RestoreCheckpoint`包装成V2；使用`activation_attempt`冒充typed Restore scope。
- **兼容影响**：最小公开shape、字段currentness与reference实现已冻结；Application/Runtime/Sandbox/Context参考纵切已实现，后续production扩展只能走additive联合Review，不能从Continuity Plan Delta推导执行能力。

### R-CTY-04 Checkpoint Participant V2公共合同

- **用例**：Checkpoint第一波以统一版本化对象请求组件Reserve/Prepare/Inspect/Commit/Abort，并保留coverage与Residual；Restore Stage后置。
- **语义Owner**：Checkpoint协调状态属于Runtime；每个Participant拥有自身Reservation、Snapshot/phase Fact、DomainResult与ApplySettlement。
- **输入**：Checkpoint Attempt/Barrier/EffectCut、Scope/Run/Binding水位、required policy、Participant Reservation、current Admission/Authorization/Permit/Begin/双Enforcement、expected participant revision。
- **输出**：Participant Fact ref；closed状态`prepared|unsupported|partial|unknown|committed|aborted`；Snapshot/coverage/evidence/residual refs。
- **不变量**：Receipt不是Fact；所有状态由Participant Owner Inspect/CAS；Commit/Abort幂等；Unknown只Inspect；Participant不能修改Runtime资格。
- **Effect/Recovery**：checkpoint capture/远程存储按对应已登记公开Operation合同执行；回包丢失Inspect exact Participant Reservation/phase attempt。Restore Stage按专用公开Action/Enforcement/Evidence/Settlement链执行，Unknown只Inspect原attempt。
- **反例**：无coverage仍Prepared；同Barrier换Snapshot digest；Participant自报Checkpoint consistent。
- **兼容影响**：新增V2 Port，不改变legacy `CheckpointParticipantPort`；legacy `RestoreCheckpoint`没有治理字段，禁止Adapter补默认值或宣称V2；Capability必须按版本明确声明。

### R-CTY-05 Continuity RunSettlementRequirement声明映射

- **用例**：让Trusted Run Assembler把Continuity声明的Timeline durability、Checkpoint classification、Content residual要求纳入Run Settlement Plan。
- **语义Owner**：RunSettlementRequirement Participant Fact由Continuity Owner提供；最终Plan/Certification由Runtime Trusted Assembler拥有。
- **输入**：BindingSet member、Component Manifest、Requirement declaration、schema/policy refs、subject digest构造规则。
- **输出**：可复读声明与Runtime Plan requirement；组件Participant Inspect Port。
- **不变量**：组件不能删Requirement、换Policy、签发Certification或CompleteRun；Evidence只允许attestation/authoritative fact。
- **Effect/Recovery**：Participant Fact写回不产生Outcome；回包丢失Inspect exact fact；unknown按Plan Policy阻止或Residual化。
- **反例**：Continuity自行把Requirement标not-required；使用Observation当Completion证据；Timeline持久即推导Run成功。
- **兼容影响**：优先复用live Run Settlement声明/Admission能力；仅缺少组件声明映射时由Runtime/Assembler Owner补映射，不新增第二Settlement框架。

### R-CTY-06 Timeline Evidence current/readability最小Delta

- **用例**：在不复制Ledger写权的前提下，为C-01 S1/S2证明一个exact Evidence Record与Source Key仍绑定同一Record，且其source registration、policy、Execution Scope、tombstone/readability在短时窗口内current。
- **语义Owner**：Runtime Evidence Owner；Continuity只消费密封current projection，不拥有Evidence identity、Trust、source lifecycle、tombstone或Retention结论。
- **复用**：现有`EvidenceSourceRecordReaderV2.InspectRecord/InspectBySource`继续负责immutable Record exact读取，不新增第二Record DTO或弱ID查询。
- **最小新增输入**：完整`EvidenceRecordRefV2`、`EvidenceSourceKeyV2`、expected Execution Scope digest、expected source registration/policy exact refs与reader binding/capability ref。**不得输入`RequestedNotAfter`或fresh now来改变sealed内容。**
- **最小新增输出**：immutable sealed `EvidenceTimelineProjectionCurrentV1`及exact `EvidenceTimelineProjectionRefV1`，包含subject key digest、projection ID/revision/digest、exact Record/Source、source registration ID/revision/configuration digest、source policy ref/revision/digest、Execution/Ledger Scope digest、六值trust class、payload schema/ref/revision/digest、closed readability state、存在时的exact Tombstone ref/revision/digest、缺失时的Owner-defined tombstone absence watermark/current revision、Retention/readability policy current ref、subject-current projection index revision/digest、Authority/Scope/Binding水位、自然且稳定的`CheckedUnixNano/ExpiresUnixNano/ProjectionDigest`；不返回append/renew/tombstone写权。
- **Owner线性化边界**：Runtime Evidence Owner必须让source registration/policy、Tombstone、以及其接纳的Retention/readability binding mutation与对应subject-current projection index和tombstone absence watermark在同一Owner线性化边界原子推进。mutation成功则新current projection/index/watermark全有，失败则全无；lost reply只Inspect原mutation及subject-current index。禁止先提交Tombstone/Policy后仍让旧projection current，也禁止用异步派生窗口宣称payload-readable。
- **历史/current分离**：旧`EvidenceTimelineProjectionRefV1`及其sealed projection永久可按exact ref Inspect用于审计；但`ValidateCurrent(subject, expected_projection_ref, now)`必须同时比较subject-current index revision/digest和absence watermark。任一mutation推进后，旧ref即使`now < ExpiresUnixNano`也必须`precondition_failed`，不得仅凭自然Expires继续current；current index revision单调且禁止ABA。
- **不变量**：S1/S2必须由同一Runtime Evidence Reader binding线性化复读同一sealed projection，并对全部字段exact比较；Record Ref与Source Key必须exact指向同一Record。空Tombstone ref不是absence证明，必须返回Owner定义的current absence watermark；tombstone存在时历史Event可保留审计元数据，但不得返回payload-readable/current权限。Owner Reader重复读取时Checked/Expires/Digest保持自然sealed值；fresh now只由Continuity传给`ValidateCurrent`，不得导致Reader重封。Runtime不得读取或吸收caller `RequestedNotAfter`；Continuity在S1/S2之后才聚合Owner自然上限并最终截短。任一字段或current index漂移Fail Closed。
- **Effect/Recovery**：纯Reader、零Evidence mutation。回包丢失/timeout只重读同一exact坐标；typed-nil、存储无覆盖或无法区分absent/unknown返回`unavailable | indeterminate`，不得降为NotFound或改source/record identity。
- **反例**：仅凭immutable Record存在宣称current；空Tombstone ref被当作“未删除”；Tombstone/Retention/source-policy mutation已提交但subject-current index或absence watermark未原子推进；旧ProjectionRef尚未Expires便被ValidateCurrent继续接受；current index回到旧revision/digest形成ABA；tombstone存在仍返回payload-readable；S1 absence watermark在S2已漂移仍提交；`InspectBySource`与`InspectRecord`不同内容仍通过；Cursor/RecordedAt/Retention默认值冒充TTL；Continuity直接读Runtime internal store；reader回包丢失后Append第二Record。
- **兼容影响**：public `EvidenceSourceRecordReaderV2`与`EvidenceSubjectCurrentReaderV1`软件合同已经存在且由Continuity Adapter消费；不扩`EvidenceGovernancePortV2`写面、不触发pre-run Evidence、不修改Evidence V2/V3 closed schema。production root仍需跨Owner Conformance。

候选只读方法（名称/version由Runtime Evidence Owner终审）：

```text
InspectTimelineEvidenceProjectionCurrentV1(
  exact_record_ref,
  source_key,
  expected_execution_scope,
  expected_source_policy
) -> immutable sealed EvidenceTimelineProjectionCurrentV1

InspectTimelineEvidenceProjectionV1(exact_projection_ref)
  -> immutable historical sealed projection

ValidateTimelineEvidenceProjectionCurrentV1(
  subject_key,
  expected_projection_ref,
  now
) -> same sealed projection or precondition_failed
```

这些方法必须在同一Owner current读取闭包内覆盖source registration、policy、tombstone/readability、subject-current index与absence watermark；不能要求Continuity分别读raw Fact Port后自行拼接current。`now`只出现在ValidateCurrent，不参与Owner seal或ProjectionDigest。

## 4. Harness公共Port Delta

### H-CTY-01 Harness Checkpoint Contribution V2

- **用例**：Harness在Runtime原子Attempt+Barrier bundle下贡献可Inspect的Run Session snapshot/coverage；production Restore所需Harness公共Phase仍作为后置装配，不由Continuity私建。
- **语义Owner**：Harness拥有Session/Snapshot Fact；Runtime拥有Barrier/Checkpoint资格；Continuity只引用。
- **输入**：namespaced versioned public Checkpoint Attempt/Barrier对象、exact Run/Session ref、Binding/Scope水位、required coverage policy、Operation authorization（如有外部动作）。
- **输出**：Harness Participant Fact ref、Session snapshot ref/digest、event/source watermark、coverage、`prepared|unsupported|partial|unknown|aborted`、Residual。
- **不变量**：不得复用Harness私有ContextPort/ModelTurnPort/EventCandidatePort；`ControlCapabilities.Checkpoint=true`不等于实现；Snapshot不拥有Runtime Run/Outcome；Restore不能复活Provider native session。
- **Effect/Recovery**：Harness Fact CAS回包丢失Inspect exact Session/Barrier；外部provider session检查走Operation V3；Unknown不重派模型或工具调用。
- **反例**：普通`contract.Snapshot`直接当Checkpoint；Provider native session ref替代Harness Session；terminal Claim当Restore成功。
- **兼容影响**：由Harness Owner新增公共namespaced V2 seam并声明Slot/Phase贡献；未合入前Harness Capability明确unsupported。

## 5. Application/Assembler接线依赖

### A-CTY-01 Continuity namespaced Workflow映射

- **状态**：**reference implementation completed / production root NO-GO**。Application公开DTO、`ContinuityWorkflowSubmissionGatewayV1`、trusted Assembler Port与reference Gateway，以及Continuity Adapter/SDK/CLI mapping已经实现；production CompiledGraph/Binding/Consumer current Assembler、各kind真实route与联合Conformance仍未闭合。Continuity只依赖Application公开`contract/ports`，不依赖`application.FacadeV2`或私建同形接口。
- **用例**：SDK/CLI/API把Timeline Projection、Checkpoint Create、Fork、Rewind Plan、Restore、Attach Artifact与Resolve Retention请求提交给Application Step Journal，由其调用Runtime与Continuity公共Port。
- **语义Owner**：Application/Harness接线与Agent Assembler Owner。
- **最小公开Port**：由Application Owner在`application/ports`提供版本化`ContinuityWorkflowSubmissionGatewayV1`，至少含`SubmitContinuityWorkflowV1`与`InspectContinuityWorkflowV1`；请求/结果DTO必须位于Application公开`contract/ports`，不得要求组件导入Application root实现包。
- **输入**：`contract/version + request ID + idempotency key + closed workflow kind + exact target scope + domain request/plan exact ref + expected revision（如有）+ requested-at/not-after + CompiledGraph/Binding/Consumer坐标`。closed kind候选为`timeline-project|checkpoint-create|fork|rewind-plan|restore|artifact-attach|retention-resolve`；最终namespaced值由Application/Assembler联合冻结。
- **禁止输入**：caller不得携可信sequence/digest/Trust/current、accepted Review Verdict/Authorization、Permit/Fence、Provider binding/Receipt、Runtime Outcome/Settlement语义副本或自造`SubmissionBundleV2`。
- **输出**：exact Application Command/Plan/Outbox坐标；Journal创建后返回exact Journal ref、状态、Step refs及领域Attempt/Result refs。输出只引用Owner Fact，不复制Runtime/Review/Participant结论。
- **不变量**：Application必须从trusted CompiledGraph/Binding/Descriptor装配immutable Submission Bundle；不扩legacy closed Command enum；不私建Slot/Phase/Hook；Step绑定exact Component/Capability/Schema；Action Gateway只接收Runtime current Authorization；Context refresh返回Context Owner exact Generation/Frame Fact。API/CLI只能使用Gateway，不能触及Submission/Journal Fact Store。
- **Effect/Recovery**：Submit回包丢失按同Scope+Request/Command identity Inspect；Journal未出现时只Inspect原Submission/Command/Outbox，不换ID重交。Checkpoint遵循已冻结第一波顺序；production Restore root/remote purge等Capability未闭合时返回`unsupported`。Begin后Unknown只Inspect原Attempt。
- **反例**：CLI直接调用Fact Store或`application.FacadeV2`；caller提交raw Application Bundle/Provider；超时后换Request ID；CompiledGraph/Binding/current缺失仍Dispatch；Restore接口存在即调用Provider；Observation冒充长期任务完成Fact。
- **兼容影响**：additive public Port已落地；现有`FacadeV2.SubmitWorkflowV2`仍为Application实现细节，只由Application Gateway复用且不成为组件依赖。仍等待Agent Assembler production Profile及各kind Descriptor/Schema/Capability/current映射；Continuity不提供替代品，Gateway存在不表示production可装配。

## 6. Delta优先级与阻塞关系

| Delta | 优先级 | 阻塞能力 |
|---|---|---|
| C-01 | Continuity owner-local完成 / cross-owner P1 | Attempt/S1-S2/atomic Project+Rebuild、legacy降权及owner-local persistence已实现；typed Owner Readers与Application root未闭合前禁止端到端生产装配 |
| C-04 | Continuity owner-local完成 | SQLite/RocksDB默认Backend与跨存储Journal recovery已实现；remote对象库、跨Ownerroot及SLA仍NO-GO |
| C-03 | Continuity+Sandbox owner-local完成 | RestorePlan V2与Workspace-only RewindPlan V2 exact shape/history/current/CAS/TTL/lost-reply及只读SDK已实现；Restore最小参考纵切已闭合。Sandbox `WorkspaceRewindCompositionPortV1`已完成keep/drop→new staged ChangeSet与immutable Composition；Rewind仍缺Application/Assembler接入既有`workspace-commit`治理链，production root/provider仍NO-GO |
| R-CTY-02 | Runtime已落地 | Checkpoint-first V2 Runtime Owner参考纵切已终审YES；不等于跨Owner/production Checkpoint |
| R-CTY-04 | P1联合 | 多Participant Checkpoint资格闭环 |
| R-CTY-06 | Runtime Evidence软件完成 | Continuity Adapter已接入exact current/readability、S1/S2与共同TTL；production root仍NO-GO |
| H-CTY-01 | P1 Harness | Harness作为Required Participant |
| R-CTY-01 | Runtime已落地 | Checkpoint phase Evidence V1/Settlement V5及Restore Stage专用Evidence/Settlement参考实现已落地；production source/root仍unsupported |
| R-CTY-03 | reference已实现 | Attempt/Identity Reservation、Eligibility、Inspect/CAS/lost-reply及Stage/Activation公共参考链；production root仍unsupported |
| R-CTY-05 | Assembler映射 | Run Settlement闭环 |
| A-CTY-01 | 公共装配 | SDK/CLI/API写路径和端到端系统验收 |

Checkpoint第一波实施顺序固定为：`G6A Action Gateway/跨Owner验收 → G6B Context Refresh → Harness G7 Checkpoint门 → Runtime原子Attempt+Barrier bundle → immutable EffectCut → Participant Reserve/治理/Inspect/CAS → H-CTY-01 Harness Participant + C-02 Manifest CAS → Create immutable ManifestSeal → Runtime fresh reread → Commit Consistency+CloseBarrier 或 Finalize Attempt+CloseBarrier`。Checkpoint流程到此停止。Restore最小参考链另按`Plan current → Intent → Attempt/Reservation → Eligibility → Admission → Authorization → Permit/Begin → Enforcement → Stage → Evidence/Settlement → Context → Activation`闭合；不授production root或任意Provider资格。

### 6.1 两阶段Delta归属

| 阶段 | 必需公共Delta | Continuity可交付上限 | 阶段外动作 |
|---|---|---|---|
| 第一阶段 Checkpoint | Runtime R-CTY-01/02参考纵切与C-02 Owner切面已实现；跨Owner仍依赖G6A/G6B/G7、R-CTY-04与H-CTY-01接线 | Manifest V2 create-once/Inspect/CAS、immutable Seal、diagnostic分类、recursive exact-ref/frozen-set/Residual校验 | C-02只生成Manifest/Seal；Runtime参考实现可生成唯一Consistency或Finalize，但未完成跨Owner装配/production root |
| 第二阶段 Restore A：Reservation/Eligibility reference | C-03、`RestoreGovernancePortV2`最小合同 | Plan Fact/current Adapter、create-once Attempt/Identity Reservation、short-TTL Eligibility、Inspect/CAS/lost-reply recovery | 已实现reference链；不授Admission/Authorization/Permit/Begin/Stage/Provider |
| 第二阶段 Restore B：reference implemented | Restore Evidence/Settlement、Action route、Context materialization、Stage/Activate公共合同 | 最小公共参考纵切 | trusted Assembler、跨Owner全量Participant、remote Provider、production root仍待联合验收 |

第一阶段只接受Owner exact refs和immutable ManifestSeal；Runtime Consistency结论只能`consistent`。第二阶段A只能由exact submitted Plan、Runtime immutable Consistency、ManifestSeal及prerequisite current复读形成Runtime short-TTL Eligibility；Eligibility不是执行Permit。任何legacy `core.CheckpointSet/RestoreRequest`、`CheckpointParticipantPort`或Foundation对象都不能经Adapter补默认字段冒充V2。
