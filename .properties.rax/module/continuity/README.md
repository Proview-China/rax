# Continuity模块说明

> Continuity Owner Manifest/Seal V2、Checkpoint-first纵切及Restore最小公共参考纵切已实现；纵切覆盖Application Intent、Runtime Reservation/Eligibility、Admission/Review/Permit/Begin、双重Enforcement、Sandbox Stage、Evidence/Settlement/ApplySettlement、Context物化与Runtime Activation。production trusted Assembler/root、跨Owner全量Participant、远程Provider及SLA仍未解锁。

> Sandbox Workspace Snapshot Artifact `reserved -> available`、Host Local加密Content Store及
> `prepare/capture -> Owner S1 -> commit -> Owner S2` governed Participant顺序已落地；
> Continuity仍只消费最终Participant exact refs，尚无真实Lifecycle与生产capture接线。

> C-01 Continuity Owner切面与owner-local生产持久化已实现：Request/Attempt/CAS、Runtime Evidence S1/S2、六类Trust、typed Owner Router接口、fresh TTL、原子发布、lost-reply Inspect与逐Request Rebuild均已落地；SQLite/RocksDB默认Backend已通过恢复与并发门，旧caller Candidate面为显式`ReferenceTimeline`。真实跨Owner Reader装配与Application root仍NO-GO。

> 2026-07-18 production-proof live核验：SQLite `ProductionSPI`现精确汇总已存在的Metadata/Retention、Timeline Governance/Policy、Checkpoint Manifest、Restore Plan、Artifact Relation、Content Integrity、Content Delta与History Derivation公开Reader/Repository；严格JSON、history/current、lost-reply、TTL、typed-nil及64个Store对象共享DB的单一CAS线性化证据已补齐。`releasecandidate`对11项proof逐项区分`owner-local implemented`与`production satisfied`：前六项只有owner-local事实，仍缺独立current certification；remote blob、Participant capture、Restore execute、cleanup root与deployment root继续缺失。因此候选保持`reference_only`，没有production Publisher/root。

> 2026-07-23 Workspace-only Rewind补齐Sandbox Owner结构化Composition：exact View+keep/drop ChangeSet refs只生成新的staged ChangeSet和immutable revision-1 Composition Fact，不执行文件Effect；Application/Assembler接入既有`workspace-commit`治理链仍未闭合，因此不解锁production Rewind。

状态：`Wave 1 + C-01 + Artifact/Content/History + Manifest/Seal + Checkpoint-first + Restore minimal public reference vertical + read SDK implemented / trusted Assembler + cross-owner all-participant + production root/provider NOT READY`。

当前设计冻结与门禁证据见[C-01 Timeline Projection Governance设计冻结记忆](../../memory/continuity/20260716-211100-C01TimelineProjectionGovernance设计冻结.md)、[C-01三条live P0最小修复冻结记忆](../../memory/continuity/20260716-213500-C01三条liveP0最小修复冻结.md)和[C-01严格复核P2纠正记忆](../../memory/continuity/20260716-214900-C01严格复核P2纠正.md)。

最新实现与测试真值见[C-01 Continuity Owner治理切面实现记忆](../../memory/continuity/20260717-143000-C01ContinuityOwner治理切面实现.md)。

Projection Policy current真值见[Timeline Projection Policy Current V1实现记忆](../../memory/continuity/20260717-144500-TimelineProjectionPolicyCurrentV1实现.md)。

生产存储实现与基准真值见[SQLite+RocksDB生产存储闭环记忆](../../memory/continuity/20260717-152200-SQLiteRocksDB生产存储闭环.md)。

RestorePlan V2与只读SDK真值见[RestorePlan V2与只读SDK记忆](../../memory/continuity/20260717-155000-RestorePlanV2与只读SDK.md)。

Workspace-only RewindPlan V2真值见[owner-local闭环记忆](../../memory/continuity/20260718-173300-WorkspaceOnlyRewindPlanV2OwnerLocal闭环.md)。

Restore Reservation/Eligibility最小链真值见[RestoreAttempt/Eligibility最小Runtime Delta记忆](../../memory/continuity/20260718-120000-RestoreAttemptEligibility最小RuntimeDelta.md)。

Restore治理、Stage、Context与Activation最小公共参考纵切真值见[Restore最小公共纵切与Enforcement恢复](../../memory/continuity/20260718-165700-Restore最小公共纵切与Enforcement恢复.md)。

Artifact因果关系真值见[Artifact因果关系owner-local纵切记忆](../../memory/continuity/20260717-162500-Artifact因果关系owner-local纵切.md)。
Content完整性诊断真值见[Content Integrity Audit V1 owner-local纵切记忆](../../memory/continuity/20260717-165245-ContentIntegrityAuditV1-owner-local纵切.md)。
Content结构共享真值见[Content Delta V1 owner-local纵切记忆](../../memory/continuity/20260717-171218-ContentDeltaV1-owner-local纵切.md)。
历史派生候选真值见[History Derivation Candidate V1 owner-local纵切记忆](../../memory/continuity/20260717-172900-HistoryDerivationCandidateV1-owner-local纵切.md)。
文档最终范围与剩余跨Owner缺口见[Continuity文档逐项完成度审计](../../memory/continuity/20260717-173850-Continuity文档逐项完成度审计.md)。

最新Checkpoint组合证据见[Checkpoint-first跨Owner Reference纵切](../../memory/continuity/20260718-010308-CheckpointFirst跨OwnerReference纵切.md)。

Timeline全维度查询与SQLite reopen证据见[Timeline全维度查询闭环](../../memory/continuity/20260718-011554-Timeline全维度查询闭环.md)。

CLI/API V1的A-CTY-01最小公开Submit+Inspect Gateway、Continuity Adapter、SDK与CLI命令映射已实现；详见[CLI/API治理写面范围冻结](../../memory/continuity/20260718-012446-CLIAPI治理写面范围冻结.md)。production trusted Assembler/current route/root仍NO-GO，接口存在不解锁Restore或Provider。

最新实现与测试真值见[A-CTY-01公开Gateway与治理写面Reference闭环](../../memory/continuity/20260718-093800-ACTY01公开Gateway与治理写面Reference闭环.md)。

SDK Timeline Page复验与CLI只读命令真值见[SDK Timeline Page与CLI读取闭环](../../memory/continuity/20260718-095800-SDKTimelinePage与CLI读取闭环.md)。

Wave1 supported/unsupported exact能力闭集与production root NO-GO真值见[Wave1 Conformance exact能力闭集](../../memory/continuity/20260718-100638-Wave1ConformanceExact能力闭集.md)。

Checkpoint Manifest Seal最小Runtime Delta与只读Adapter真值见[Checkpoint Manifest Seal Runtime exact Adapter闭环](../../memory/continuity/20260717-232700-CheckpointManifestSealRuntimeExactAdapter闭环.md)。

## 1. 作用与Owner边界

Continuity拥有Timeline投影与查询、Checkpoint Manifest/Plan领域对象、`CheckpointManifestFactV2`的create-once/CAS/history/current Inspect、diagnostic/residual finalization及immutable revision 1 Manifest Seal。Runtime Evidence Ledger仍是ledger sequence和Evidence Record唯一Owner；Runtime拥有CheckpointAttempt、Barrier、Effect Cut、immutable `CheckpointConsistencyFact`、short-TTL/current `RestoreEligibilityFact`、RestoreAttempt、新Instance/高Epoch/new Lease/Fence和Activation。各Participant只拥有自身Snapshot；Continuity只保存exact refs，不捕获、不解释、不修改其他Owner的Snapshot或事实。

生产语义要求Evidence只有在Runtime exact Reader完成immutable sealed S1/S2后才能投影。该Continuity Adapter链已实现；六类Trust逐项保持：`observation|late_observation|receipt|attestation|claim`都保持原Ledger TrustClass，禁止generic OwnerFact升权；只有`authoritative_fact`必须由领域Owner Fact current Reader完成同一sealed projection S1/S2。领域若需证明attestation/claim current，必须发布独立typed Delta且不改Runtime Trust。Owner Readers只返回自然sealed Checked/Expires/Digest，不接收`RequestedNotAfter`；caller上限仅由Continuity在S1/S2后最终截短。旧caller admission只存在于`ReferenceTimeline`测试形状，production root不得装配。Continuity投影、Provider Receipt或Timeline写入本身都不能把Observation升级为authoritative Fact、Settlement或Run终态。Unknown/lost reply只能Inspect原identity，不创建替代Attempt、Journal、Manifest、Instance或Epoch。

## 2. Wave 1 live组成

| 实现位置 | 已实现内容 |
|---|---|
| `ExecutionRuntime/continuity/contract` | Wave 1、Timeline Governance V1、V2 Manifest/Seal、RestorePlan V2与Workspace-only RewindPlan V2 exact shape/状态机 |
| `ExecutionRuntime/continuity/ports` | Backend-neutral SPI、Timeline、Checkpoint Manifest与RestorePlan Repository/Reader；不暴露Runtime raw Fact Port |
| `ExecutionRuntime/continuity/domain` | Timeline、Content/Retention/Settlement/Manifest与RestorePlan治理；旧caller Candidate服务显式命名`ReferenceTimeline` |
| `ExecutionRuntime/continuity/runtimeadapter` | Timeline public Readers S1/S2，以及Checkpoint完整Owner/Scope exact Seal lookup、RuntimeClosure映射、Context/Artifact digest只读投影；只依赖Runtime core/ports，不写Runtime Fact |
| `ExecutionRuntime/continuity/storage/memory` | 线程安全reference backend；Manifest/Seal闭环；Timeline Tombstone为immutable revision-1 Fact+visibility overlay，historical Event Inspect不变；public Store不再含bulk Replace或Event mutation写口 |
| `ExecutionRuntime/continuity/storage/sqlite` | pure-Go SQLite schema v9/additive migration、WAL/FULL/IMMEDIATE、Journal/Object/Retention、Timeline、Artifact Relation、Content Integrity Audit、Content Delta、History Derivation Candidate、Checkpoint Manifest/Seal及Restore/Rewind Plan history/current/CAS生产repository |
| `ExecutionRuntime/continuity/applicationadapter` | 只依赖Application公开contract/ports的治理写面Adapter；要求Continuity-owned exact Domain Request Ref，不构造raw Bundle或调用Provider |
| `ExecutionRuntime/continuity/sdk` | Timeline/Artifact Relation/Content Integrity Audit/Content Delta/Checkpoint/RestorePlan/Retention Inspect、Query/Watch、Timeline Page/Cursor exact边界复验、Plan/Credential Validate及唯一Application治理`Submit/InspectGovernedWorkflow`；无Fact Store直写 |
| `ExecutionRuntime/continuity/cli` | 只读`timeline show/watch`、`checkpoint inspect`，七类治理写命令与`workflow inspect`的strict JSON参数/输出映射；不负责根CLI注册、endpoint或credential |
| `ExecutionRuntime/continuity/storage/rocksdb` | `continuity_rocksdb` build tag隔离的RocksDB 9.10窄C API ContentStore；sync WAL、checksum、Snappy、metrics与Chunk integrity |
| `ExecutionRuntime/continuity/conformance` | restricted/reference-only supported/unsupported exact能力闭集；未知、缺失、重复与production root overclaim均拒绝 |
| `ExecutionRuntime/continuity/releasecandidate` | 公共`ComponentReleaseV1` reference-only builder、readiness与lost-reply exact publisher；Factory仅descriptor |
| `ExecutionRuntime/continuity/fakes` | 测试专用create/CAS/Seal durable-write lost-reply wrapper；拒绝nil/typed-nil delegate，不是生产Backend |
| `ExecutionRuntime/continuity/tests` | 单元、白盒、黑盒、故障注入、并发/race、Conformance和Checkpoint/Restore/Settlement NO-GO反例 |

## 3. 已实现能力

- Timeline生产形状从Runtime Evidence exact Record与R-CTY current Reader派生scope/sequence/digest/Trust，不另分配第二sequence；S1/S2/fresh、六类Trust、Attempt/CAS与原子current均已实现。旧caller数据仅存在于显式`ReferenceTimeline`测试形状。
- Projection Policy由Continuity Owner以opaque policy ID+scope的exact ref、history/current/CAS、closed state和自然TTL密封；它不解释或新增业务Policy结论，caller `RequestedNotAfter`不进入其digest。
- Timeline支持Identity/Lineage/Instance/Run/Turn/Step/Action/Artifact/Effect/Review Case/Checkpoint/Time/Parent/Causation/Correlation全维度Query、稳定Cursor、Authority/Policy/Query漂移拒绝、Watch gap、逐Request governed Rebuild、因果环拒绝和immutable Tombstone overlay；多个类型化ObjectRef按AND匹配，Rebuild不bulk替换，Tombstone不改历史Event。
- canonical digest覆盖Evidence mirror、Scope、Owner binding、Plan、Manifest及有序Chunk；同identity换内容、摘要篡改或非canonical cursor均Fail Closed。
- Content以内容摘要切Chunk，按`proposed -> metadata_pending -> content_staged -> reference_committed -> visible -> closed`推进Journal；lost reply/崩溃后Inspect原Journal并恢复原Object。
- Retention/Tombstone/Legal Hold为revision CAS元数据状态机；Physical Purge明确返回unsupported。
- Retention Manager已补齐Create/CAS durable-write lost-reply exact Inspect、changed same-ID Conflict、typed-nil依赖拒绝与SDK Reader结果复验；不因此获得Physical Purge或Policy Owner权力。
- Artifact Relation owner-local纵切以coordinate-only Request启动；只有Timeline exact Event和typed Artifact Owner source projection两轮复读全量一致才生成immutable revision-1 Relation。Artifact/Related结构索引、lost-reply Inspect、跨Tenant隔离、内存repository及SQLite（自schema v5引入，当前schema v9）与只读SDK已落地；它不拥有Artifact正文、current或其他Owner事实。
- C-06 Content Integrity Audit以coordinate-only bounded Subject启动；Object Manifest/visibility、Write Journal及每个Chunk的存在性/bytes length/digest完成两轮一致检查后，才形成immutable revision-1诊断Fact。`healthy`只覆盖本次坐标与时点，不表示全库无孤儿；无任何Cleanup、Purge或Provider能力。
- C-07 Content Delta以coordinate-only Base/Target Object启动；两个Object均visible且Manifest/Chunk bytes完成S1/S2 exact检查后，派生ordered recipe与reused/added/removed集合并形成immutable revision-1 Fact。它不创建Target、不执行patch/Compaction、不授base删除资格。
- C-08 History Derivation Candidate以ordered Evidence Record ref+expected Record/Projection digest与output Object坐标启动；Event和Object/Chunk双轮exact一致后形成immutable revision-1 candidate-only Fact。它不证明派生语义正确、不成为任何current、不改写Event、不执行Compaction/Purge。
- `DomainResultFact -> OperationSettlementRef -> AppliedSettlement`只校验exact identity/digest绑定；Runtime ref不含Outcome、Disposition、Status等语义副本。
- `CheckpointManifest`把required Participant的partial/unsupported判为诊断partial，把unknown、Effect Cut未接受或Context未物化判为诊断indeterminate；它不形成Runtime consistency或restore eligibility。
- V2 Manifest跨Owner输入全部使用`contract/schema + Owner Binding + TenantID + ID + revision + digest + ScopeDigest` exact ref；identity key是包含完整`OwnerBinding`的可比较结构，delimiter-bearing字段或Owner任一字段漂移不会碰撞。Context/Memory/Knowledge/Attempt/Participant/Evidence/Diagnostic/Residual递归与Manifest Execution Scope同Tenant同Scope，required participant set和frozen ref set均从canonical closure重算，换包Fail Closed。
- Manifest聚合顶层、已有Settlement或未Settlement的Attempt、Participant及任意severity Diagnostic Residual；`verified_candidate`和Seal要求聚合Residual为空。
- Manifest revision 1 create-once，后续只允许CAS推进；每个revision永久保留，current/history按结构化Tenant/Scope/ID隔离，current reader额外验证Continuity Owner。terminal finalization不可再CAS；已前进的旧CAS重放Conflict，不形成ABA。
- 已Begin Attempt缺opaque Settlement时，必须携exact Inspection与Residual且只能进入diagnostic finalization；Unknown不猜测、不重派。
- Seal只接受current `verified_candidate` Manifest，Repository事务内重验exact revision/Owner和全部binding，固定revision 1；same ID/content幂等，换Manifest/Attempt/Barrier/EffectCut/Participant closure Conflict，lost reply只Inspect原Seal。
- Runtime `CheckpointManifestSealContractVersionV2=2.1.0`已携完整Continuity exact lookup；Participant typed closure只经公开结构化映射进入Seal。Continuity Adapter逐项校验Runtime Participant Set/closure、Attempt/Barrier/EffectCut、frozen set及Context/Artifact closure digest，Runtime Gateway在Consistency CAS前执行S1/S2；该链只有Reader能力。
- Workspace Snapshot capture的Sandbox owner-local闭包现已形成complete Coverage/Participant Fact，
  并以Snapshot Artifact Fact + exact aggregate current通过Application Reader映射到上述Participant
  typed closure输入；Residual非空仍只形成诊断，不进入Manifest Seal/Consistency。
- RestorePlan V2已实现exact CheckpointConsistency/ManifestSeal/Context/prerequisite refs、fresh Instance/higher Epoch/new Lease/Fence proposal、closed state、TTL/current、create-once/CAS/history/current及SQLite持久化；Fork/Rewind/Restore均不执行现实副作用。
- Workspace-only RewindPlan V2已实现exact Checkpoint/Manifest、Sandbox View/keep-drop/planned ChangeSet、Dependency/Review/irreversible Effect/Residual refs、closed state、TTL/current、create-once/CAS/history/current、lost-reply Inspect、64路CAS/no-ABA、跨Tenant隔离、SQLite schema v9与只读SDK。Sandbox Owner的`WorkspaceRewindCompositionPortV1`已完成keep/drop结构化组合、new staged ChangeSet、immutable Composition、lost-reply Inspect、64路单赢家和历史读取；Continuity仍不创建Sandbox Fact或执行文件，Application/Assembler治理装配继续NO-GO。
- `runtimeadapter.RestorePlanCurrentReaderV2`已把exact submitted Plan、immutable ManifestSeal与Runtime Consistency映射为稳定natural-TTL Runtime current projection；Runtime `RestoreGovernancePortV2`已实现create-once Attempt/Identity Reservation、short-TTL Eligibility、S1/S2、lost-reply Inspect、history/current及CAS/no-ABA。后续公共参考链由Application/Runtime/Sandbox/Context各Owner闭合，Continuity只关联exact refs。
- `RecoveryCredentialV1`已实现secret-free字段闭集、`inspect/stage` closed action、canonical digest、TTL边界、revocation、clone/no-alias及Restore Plan exact binding；它只作为Plan requirement输入，不能单独形成Runtime current Eligibility、Permit、Fence、Dispatch或Provider调用能力。

## 4. 未实现与硬性NO-GO

- C-01跨Owner生产装配：各authoritative领域真实typed Owner current Reader、Application/Assembler Consumer/Policy/Owner route及联合Conformance；owner-local Controller/Adapter/持久化已完成；
- `ReferenceTimeline`与raw `PutProjection`只可测试/reference装配，禁止进入production root；
- `P1-1 / R-CTY-06`已完成Continuity Adapter接入；`P1-2 / typed Owner routing`只有接口与fake测试，真实Owner Readers和Assembler route仍待各Owner闭合；
- Artifact Relation真实typed Artifact Owner Router和production Application/Assembler route尚未闭合；SDK可提交`artifact-attach`治理Workflow，但没有route时Fail Closed，不能直写Relation Fact；
- Content Integrity Audit没有Content keyspace枚举、Checkpoint/Fork/Review跨Owner引用闭包与删除资格；自动orphan reclaim、physical purge、remote archive/provider仍unsupported；
- Content Delta没有Compactor/Indexer/Consolidator执行器、Target materialization或Purge能力；Fact只描述已存在Object的结构共享关系；
- History Derivation Candidate没有Compactor/Indexer/Consolidator调度、执行、算法证明或current发布能力；Fact只描述immutable Event集合与既有Object的候选关系；
- Continuity不实现且不拥有Runtime CheckpointAttempt/Barrier/Effect Cut/Consistency/Finalization；Checkpoint-first跨Owner reference纵切已经组合Harness Gate、Application Coordinator、Continuity Manifest/Seal和Runtime Consistency。Sandbox Workspace Snapshot的owner-local Artifact/Coverage/Participant与current映射已实现；production Checkpoint Participant/root仍未闭合；
- Restore最小公共参考纵切已闭合Application immutable Intent、Action Admission、Review Authorization绑定exact Eligibility/Attempt、Permit/Begin、Restore Stage/Inspect/Settlement/Context materialization/Activate；仍缺trusted Assembler/current Reader生产装配、跨Owner全量Participant、root credential/deployment attestation与远程Provider；
- Workspace-only Rewind已闭合Continuity Plan与Sandbox Owner Composition，但缺Application/Assembler把Composition exact ref转换为既有`workspace-commit`治理请求的公共装配；不能绕过Admission/Review/Fence直接提交staged ChangeSet；
- Runtime Run Settlement公开合同当前只有Participant exact Inspect，缺组件Owner create/CAS公共写口与trusted Assembler Requirement映射；Continuity不得私建第二Settlement链；
- 真实remote blob、purge、archive、KMS、跨Owner生产root、进程拓扑和SLA；
- Runtime/Harness/Tool/Model Invoker真实跨Owner current Adapter、production trusted Assembler、根CLI/API与production root；A-CTY-01 reference Gateway/SDK/CLI mapping、Restore Action Gateway与Context Restore Adapter已实现；
- 未经production root装配的远程Provider或高风险外部副作用；Sandbox Host-Local workspace Stage只在受治理参考组合与测试中执行，不构成任意Provider准入；
- 外部世界回滚。Rewind/Restore只能形成计划和新Instance/高Epoch语义；Partial Checkpoint只供诊断，legacy Port不得包装升权。

Runtime Checkpoint-first V2、Continuity Owner Manifest/Seal V2及Restore最小公共参考纵切已通过当前全门；Component release仍固定`ProductionClaimEligible=false`。production Checkpoint与Restore root继续`NOT READY`；不得把Seal解释为current Restore资格，不得把Host-Local参考Stage解释为跨Owner或远程Provider准入，也不得扩legacy Checkpoint/Restore合同闭表。

Component release 当前只到`reference_only assembly-candidate`：SQLite/RocksDB owner-local durable store与Restore最小参考执行链不等于remote Provider、跨Owner全量Participant、cleanup root或deployment attestation，不能自签production。

2026-07-18管理线已冻结下一切面：首版Required Checkpoint Participant为Runtime、Harness、Context、Sandbox、Memory-Knowledge；Review/Tool只冻结exact refs。Rewind只允许新的Workspace文件ChangeSet Effect并复用Sandbox既有workspace-commit治理链，不执行Tool或外部系统补偿。默认部署为本机SQLite+RocksDB、provider-neutral SPI、按真实benchmark报告且无SLA；remote provider/KMS/root继续NO-GO。

## 5. 验证证据

2026-07-17在`ExecutionRuntime/continuity`实际运行：

```bash
go test -count=1 -shuffle=on ./...
go test -count=100 ./contract ./domain ./fakes ./storage/memory ./tests/blackbox ./tests/fault ./tests/conformance
go test -race -count=20 ./domain ./fakes ./storage/memory ./tests/fault ./tests/conformance
go test -race ./...
go vet ./...
gofmt -l .
go list -deps ./...
```

结果：既有Manifest/Seal、C-01、Artifact Relation、Content Integrity Audit、Content Delta、History Derivation Candidate、RestorePlan V2与只读SDK门均PASS；Checkpoint-first reference纵切及Restore最小公共参考纵切的lost reply、partial、CAS/no-ABA、scope/fence/review漂移和重放反例均已通过。本轮Runtime/Application/Sandbox/Context Engine/Continuity五模块full ordinary/race/vet均PASS。证据不覆盖Compactor、全库orphan closure、Purge、真实typed Owner Readers、production Checkpoint/Restore root或跨Owner全量Participant。
