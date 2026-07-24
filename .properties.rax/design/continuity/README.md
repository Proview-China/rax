# Continuity 可实施设计

> Continuity Owner Manifest/Seal V2、Checkpoint-first跨Owner reference纵切及Restore最小公共参考纵切已实现并通过定向重复/race与相关模块全门；Restore纵切包含Application Intent、Runtime Reservation/Eligibility、Admission/Review/Permit/Begin、双重Enforcement、Sandbox Stage、Evidence/Settlement/ApplySettlement、Context物化与Runtime Activation。仍不解锁production trusted Assembler/root、跨Owner全量Participant、远程Provider或SLA。

> C-01 Timeline Projection Governance、Artifact Relation、C-06/C-07/C-08、Continuity-owned持久化Owner切面及Checkpoint Manifest Seal exact Runtime只读Adapter已实现。Checkpoint-first reference纵切已组合Harness Gate、Application Coordinator、Harness/Sandbox两个测试Participant、Continuity Manifest/Seal和Runtime Consistency；全链`ProviderCalls=0`，不宣称生产Snapshot capture、production root或Restore GO。

> 2026-07-18：Sandbox Owner已实现Workspace Snapshot Artifact的`reserved -> available`原子
> Fact/CAS与Host Local加密Content Store；它尚未接入Runtime Participant phase、Application
> Coordinator或Continuity Manifest，因此Checkpoint production仍NO-GO。Checkpoint capture复用
> 现有Runtime/Application exact ref，无新增Runtime schema。

## 1. 状态、依据与授权边界

- 状态：Wave 1、C-01、Artifact Relation、C-06/C-07/C-08、Continuity Manifest/Seal、Checkpoint-first跨Owner reference纵切、`RestorePlanFactV2` owner-local治理/SQLite持久化、Restore最小公共参考纵切，以及Workspace-only Rewind的Continuity Plan+Sandbox结构化Composition均已落地；Application/Assembler尚未把Rewind Composition接入既有`workspace-commit`治理链，trusted Assembler、跨Owner全量Participant、远程Provider与production root仍是**NO-GO**。
- 最高业务输入：`tmp.document/Continuity.md`。
- 强制治理输入：仓库`AGENTS.md`、`.properties.rax/MAIN.md`、`component-governance/README.md`。
- live合同基线：Runtime Binding V2、Operation Effect V3、Operation Settlement V4、Review V2、Evidence Ledger V2/V3、Run Settlement/Lifecycle V2/V3，以及Harness Governed V2/V3和Assembly/CompiledGraph公开合同。Runtime Checkpoint-first V2与跨Owner reference纵切均已落地。Restore使用typed scope、create-once `RestoreAttemptFactV2`、Runtime原子新Instance/high Epoch/new Lease/Fence Reservation、short-TTL `RestoreEligibilityFactV2`及Inspect/CAS/lost-reply恢复；Application随后按immutable Intent→Admission→Review/Authorization→Permit/Begin→双重Enforcement→Sandbox Stage→Evidence/Settlement→Context→Activation收敛。Continuity `RestorePlanCurrentReaderV2`只读exact submitted Plan、immutable Seal和Runtime Consistency，不写Runtime/Review/Sandbox/Context事实；公共参考实现存在不自动形成production root或任意Provider资格。
- 独占资产范围：`.properties.rax/{design,plan,module,memory}/continuity/**`；实现根为`ExecutionRuntime/continuity/**`。
- live实现根已经包含独立Go module、Wave 1合同/领域逻辑/reference backend，以及approved `CheckpointManifestGovernancePortV2`、Manifest/immutable Seal Repository/Reader与测试；Restore Stage/Activate由各自Owner公共合同与参考组合实现，不位于Continuity，也不构成production Checkpoint capture或任意Provider授权。

本设计把定稿中的“可查询、可Fork、可Rewind、可Restore且不篡改现实因果关系”翻译成可实施合同。治理文档中的目标条款不会被误写成live能力；当前不存在的公共能力只进入`port-delta.md`。

### 1.1 资产索引

- [合同与状态机](contracts-and-state-machines.md)：Checkpoint Manifest V2 exact-ref闭包、Restore Plan/Attempt状态机和治理不变量。
- [公共Port Delta](port-delta.md)：C-01 exact/current Reader与Attempt治理、C-02 Manifest/Seal Owner切面及最小Runtime Seal exact lookup/Participant mapping Delta已实现；Restore Plan current Adapter与Runtime Reservation/Eligibility最小参考链已实现，Checkpoint生产装配及Restore execution仍保持NO-GO。
- [跨模块映射](integration-mapping.md)：Runtime/Harness/Application调用序列、Action Gateway与Context refresh依赖。
- [设计验收](acceptance.md)：NO-GO反例、故障恢复和Owner边界验收。
- [架构图](architecture.drawio)：Control Plane授权、Continuity ref-only Fact与Participant Owner关系。
- [落地计划](../../plan/continuity/README.md)：依赖闭合后的文件级实施顺序和测试矩阵。
- [模块说明](../../module/continuity/README.md)：Wave 1 live包结构、Owner边界和能力上限。
- [进度记忆](../../memory/continuity/20260716-112436-ContinuityWave1LiveState资产收口.md)：本次live-state收口与实际验证证据。
- [独立审计返修记忆](../../memory/continuity/20260716-144250-CheckpointManifestGovernanceV2独立审计返修完成.md)：recursive scope、Residual、Repository、结构化Reader、64路竞争与no-ABA返修证据。
- [结构键复审返修记忆](../../memory/continuity/20260716-145936-ExactFactIdentity结构键复审返修完成.md)：delimiter-safe完整OwnerBinding identity与跨Tenant Seal隔离证据。
- [最终独立代码复审YES记忆](../../memory/continuity/20260716-151911-ManifestSealV2最终独立代码复审YES.md)：Continuity Owner最终裁决与未解锁边界。
- [RestoreAttempt/Eligibility最小Runtime Delta记忆](../../memory/continuity/20260718-120000-RestoreAttemptEligibility最小RuntimeDelta.md)：Plan current Adapter、Runtime Reservation/Eligibility、lost-reply/CAS与测试证据。
- [C-01 Timeline Projection Governance设计冻结记忆](../../memory/continuity/20260716-211100-C01TimelineProjectionGovernance设计冻结.md)：production P0、S1/S2/min-TTL/closed errors、最小Runtime Delta与联合Review门禁。
- [C-01三条live P0最小修复冻结记忆](../../memory/continuity/20260716-213500-C01三条liveP0最小修复冻结.md)：三入口live证据、Request闭集、统一Controller、Rebuild逐项治理与immutable Tombstone overlay。
- [C-01 Continuity Owner治理切面实现记忆](../../memory/continuity/20260717-143000-C01ContinuityOwner治理切面实现.md)：Attempt/S1-S2/atomic publish、typed Owner路由边界及实际测试证据。
- [Timeline Projection Policy Current V1实现记忆](../../memory/continuity/20260717-144500-TimelineProjectionPolicyCurrentV1实现.md)：Policy exact ref/history/current/CAS、自然TTL及并发证据。
- [SQLite+RocksDB生产存储闭环记忆](../../memory/continuity/20260717-152200-SQLiteRocksDB生产存储闭环.md)：driver裁决、schema、跨存储恢复、benchmark及边界。
- [RestorePlan V2与只读SDK记忆](../../memory/continuity/20260717-155000-RestorePlanV2与只读SDK.md)：exact Plan治理、SQLite history/current、SDK边界与测试证据。
- [Artifact因果关系owner-local纵切记忆](../../memory/continuity/20260717-162500-Artifact因果关系owner-local纵切.md)：coordinate-only Request、typed Owner S1/S2、immutable Relation、SQLite索引与门禁。
- [Content Integrity Audit V1 owner-local纵切记忆](../../memory/continuity/20260717-165245-ContentIntegrityAuditV1-owner-local纵切.md)：bounded Subject双轮检查、immutable诊断Fact、SQLite schema v6引入点及全门证据。
- [Content Delta V1 owner-local纵切记忆](../../memory/continuity/20260717-171218-ContentDeltaV1-owner-local纵切.md)：Base/Target exact双轮检查、结构共享recipe、SQLite schema v7及全门证据。
- [History Derivation Candidate V1 owner-local纵切记忆](../../memory/continuity/20260717-172900-HistoryDerivationCandidateV1-owner-local纵切.md)：ordered Event/output Object双轮检查、candidate-only Fact（自SQLite schema v8引入，当前schema v9）及全门证据。
- [Checkpoint Manifest Seal Runtime exact Adapter闭环](../../memory/continuity/20260717-232700-CheckpointManifestSealRuntimeExactAdapter闭环.md)：最小Runtime Delta、只读Adapter、S1/S2与全门证据。
- [Checkpoint-first跨Owner Reference纵切](../../memory/continuity/20260718-010308-CheckpointFirst跨OwnerReference纵切.md)：Harness Gate、Application协调、两个测试Participant、Manifest/Seal、Runtime Consistency、故障恢复与生产门禁。
- [Timeline全维度查询闭环](../../memory/continuity/20260718-011554-Timeline全维度查询闭环.md)：Turn/Step/Action/Artifact/Effect/Review Case/Checkpoint过滤、Cursor防漂移及SQLite reopen证据。
- [CLI/API治理写面范围冻结](../../memory/continuity/20260718-012446-CLIAPI治理写面范围冻结.md)：用户范围裁决、A-CTY-01最小Application公开Gateway Delta及NO-GO边界。
- [首版Participant/Rewind/部署裁决](../../memory/continuity/20260718-171800-首版ParticipantRewind部署裁决.md)：核心五类Required Participant、仅Workspace文件Rewind及本机SQLite+RocksDB无SLA边界。
- [Workspace-only RewindPlan V2 owner-local闭环](../../memory/continuity/20260718-173300-WorkspaceOnlyRewindPlanV2OwnerLocal闭环.md)：exact Plan、CAS/TTL/lost-reply、SQLite schema v9与跨Owner剩余门。
- [A-CTY-01公开Gateway与治理写面Reference闭环](../../memory/continuity/20260718-093800-ACTY01公开Gateway与治理写面Reference闭环.md)：公开DTO/Port、Application reference Gateway、Continuity Adapter/SDK/CLI映射与production NO-GO真值。
- [SDK Timeline Page与CLI读取闭环](../../memory/continuity/20260718-095800-SDKTimelinePage与CLI读取闭环.md)：Reader Page exact复验、输入Cursor前置拒绝、只读CLI映射与全量门禁。
- [Wave1 Conformance exact能力闭集](../../memory/continuity/20260718-100638-Wave1ConformanceExact能力闭集.md)：supported/unsupported闭集、四级拒绝边界与production root NO-GO真值。

### 1.2 Wave 1 live-state

截至2026-07-18，`ExecutionRuntime/continuity`已实现Timeline/C-01、Content/Journal/Retention、Artifact Relation、C-06/C-07/C-08、C-02 Manifest/Seal、Runtime exact Seal只读Adapter、`RestorePlanFactV2` owner-local治理/SQLite持久化、RecoveryCredential纯验证及只读Go SDK。Checkpoint-first reference纵切已用公开Port组合Harness Gate、Application协调、Harness/Sandbox两个测试Participant、Continuity Manifest/Seal与Runtime Consistency；lost reply和partial路径均保持Inspect-only/Fail Closed。Continuity Plan current Adapter可请求Runtime建立Attempt/Identity Reservation与Eligibility，但不创建Runtime Fact；实际Restore Stage、Context与Activation由Application/Runtime/Sandbox/Context各Owner的公共参考组合完成。

这不改变Governance V2门禁：reference测试Participant不等于生产Snapshot capture或任意Provider准入；A-CTY-01公开Gateway、Restore Action Gateway、Continuity Adapter/SDK/CLI映射及Context Restore Adapter已实现，但trusted CompiledGraph/Binding/Consumer current Assembler、跨Owner全量Participant和production root仍未闭合。真实remote blob/purge/archive与生产SLA继续NO-GO。owner-local `RecoveryCredentialV1`不是Runtime eligibility、Permit、Fence或执行凭证。Partial只生成诊断分类并保持Gate受Fence；Restore/Rewind不宣称外部世界回滚。

## 2. 冻结裁决

### 2.1 Event Ledger与Timeline sequence单主

live Runtime已经冻结：`Evidence Ledger V2`是执行证据唯一账本主库，只有Ledger Owner分配`ledger sequence`并形成摘要链；旧`TimelinePort`是legacy restricted，Timeline不得建立第二写入口、第二sequence或V1/V2双写。

因此本组件采用以下精确分工：

1. Runtime Evidence Ledger Owner拥有不可变Evidence Record、source cursor、ledger sequence与摘要链。
2. Continuity拥有Timeline查询模型、因果图、Parent/Child Link、稳定查询Cursor、Projection、Checkpoint Manifest、Fork/Rewind/Restore Plan、Recovery Credential和Retention领域事实。
3. Continuity的`TimelineEventRecord`是对一个精确`EvidenceRecordRefV2`的可重建投影；只有Ledger Trust为`authoritative_fact`时才同时绑定领域Owner Fact Ref，不另分配权威ledger sequence。
4. Timeline Cursor绑定`ledger_scope_digest + after_sequence + query_digest + projection_revision`；它复用Ledger水位，但不转移Ledger所有权。
5. Continuity领域事实成为权威证据时，必须先由Continuity Fact Owner独立Inspect精确Fact，再按Evidence V2的`authoritative_fact`规则落账；“已写入Timeline”本身不升级可信度。

这同时满足定稿中的多Source、多Epoch、跨分支查询要求，并消除Timeline与Evidence双主风险。

### 2.1.1 C-01 Timeline Projection Governance冻结

1. production Request字段闭集只有stable Attempt ID/idempotency key、EvidenceSourceKey、可选expected RecordRef、仅authoritative_fact OwnerFact ref、Policy、Scope与RequestedNotAfter；不得携semantic/causal/object/payload/time或可信bool/sequence/digest/Trust/current。Continuity先create-once `TimelineProjectionAttemptFactV1`，Attempt只保存Owner exact refs/密封projections、Reader派生Event digest、共同TTL、状态和结果ref；lost reply只Inspect原Attempt，progressed replay Conflict且禁止ABA。
2. S1/S2都必须通过同一Runtime Evidence Reader binding按Source Key定位、再按Reader返回的exact Record Ref复读；sequence、Record/candidate/chain/payload digest、Trust与scope全部从Reader结果派生。R-CTY-06同时密封source/policy/current、readability、exact Tombstone或Owner-defined absence watermark及subject-current projection index；空Tombstone ref不等于未删除。Tombstone/已接纳Retention binding/source-policy mutation必须与current index/watermark在Runtime Evidence Owner同一线性化边界原子推进。Reader返回immutable sealed projection，S1/S2逐字段exact比较稳定Checked/Expires/ProjectionDigest；fresh now只做`ValidateCurrent(expected ProjectionRef)`，不能每读重封。旧ProjectionRef历史仍可Inspect，但index漂移后即使未过Expires也不得current。
3. Runtime六类Trust闭合路由：`observation | late_observation | receipt | attestation | claim`原样保持Ledger TrustClass，只能形成对应Timeline observation/projection，禁止generic OwnerFact升级；只有`authoritative_fact`允许且必须通过同一领域Owner Reader binding复读Owner Fact immutable sealed current projection。领域若要为attestation/claim另做current证明，必须发布独立typed Delta且不改变Ledger TrustClass。
4. Controller唯一顺序是Create Attempt→S1 `InspectBySource+InspectRecord`双读→R-CTY-06 current/readability/tombstone→仅authoritative_fact Owner current→S2 exact重复全部Readers→fresh ValidateCurrent/TTL→原子Event+Attempt visible+Continuity current index。`reconcile_required`属于明确状态；三对象不能全有时保持不可见并只Inspect原Attempt。
5. `RequestedNotAfter == 0`表示caller不加上限，`<0`非法，`>0`只能缩短；它不传给Owner Reader，也不进入任何Owner ProjectionDigest。Evidence/Owner Readers先返回自然sealed Checked/Expires/Digest，Continuity在S1/S2 exact与fresh-now `ValidateCurrent`通过后，取Projection Policy、Evidence source/policy/readability/tombstone-absence、仅authoritative_fact的Owner Fact/Authority/Scope/Binding所有自然上限的最小值，最后才按正数RequestedNotAfter截短。Tombstone存在时历史metadata仍可审计，但payload不得声明readable/current；任何Owner缺失可证明上限时Fail Closed，Event时间、Cursor TTL或Retention window不能补TTL。
6. Request、Attempt/Event历史与Model/Tool Projection Ref本身都不代表production current。任何current-binding projection都不修改Ledger TrustClass；Continuity不成为Model/Tool identity Owner。
7. C-01 closed errors仅允许`invalid_argument | not_found | conflict | precondition_failed | unavailable | indeterminate | unsupported`；unknown/lost reply只Inspect原identity，progressed replay不得形成ABA。
8. Production Rebuild只接受Request闭集/Request refs，并逐Item走同一Attempt+S1/S2/fresh/atomic Controller；禁止bulk caller Candidate/Event import、`PutProjection`、`ReplaceLedgerScope`、覆盖history/Tombstone overlay或回退current index。
9. Timeline Event create后revision/digest/bytes永久immutable。Tombstone必须create独立immutable `TimelineProjectionTombstoneFactV1`并与Visibility/current overlay index同事务CAS；Query/Watch组合history+overlay。`TombstoneProjection`原地改Event、删除overlay复活Event和Rebuild覆盖Tombstone全部禁止。

C-01当前为**Continuity Owner切面与owner-local持久化实现完成、跨Owner装配未完成**：生产形状`TimelineProjectionAdapterV1.Project/Rebuild`已统一走Attempt/S1/S2/fresh/atomic Controller；同Request完成重放直接Inspect可见结果，Unknown/lost reply只Inspect原Attempt。六类Trust保持Runtime原值，只有`authoritative_fact`调用typed Owner Router；没有真实领域Reader时Fail Closed为`unsupported`。旧caller Candidate路径已显式命名为`ReferenceTimeline`且不得由生产root装配。剩余门是各领域Owner typed current Reader、Application/Assembler binding和联合Conformance；这仍不构成production current GO。

### 2.2 SQLite与RocksDB默认实现

SQLite+RocksDB已按先Conformance/Benchmark后实现完成owner-local生产存储选择，公共合同仍保持Backend无关且不承诺SLA：SQLite采用pure-Go modernc driver并启用WAL/FULL/IMMEDIATE/busy-timeout；RocksDB采用`continuity_rocksdb` build tag下的极窄C API bridge，当前实证基线为系统RocksDB 9.10。

| 存储 | 负责内容 | 不负责内容 |
|---|---|---|
| SQLite | Continuity领域Fact、CAS revision、对象关系、当前Revision、幂等键、Projection水位、Checkpoint/Plan索引、Retention/Legal Hold、跨存储Write Journal | 不保存大型明文Snapshot，不成为Evidence Ledger的第二主库 |
| RocksDB | 内容寻址Chunk、Delta、Fragment、压缩块、可复用Manifest正文、加密Snapshot Blob/大型Evidence的受治理副本 | 不分配领域revision、Authority、Timeline sequence或Runtime状态 |
| 远程对象库 | 经治理允许的加密Blob/Chunk | 不因Provider回包成为Checkpoint事实，不保存长期明文Secret |

RocksDB中的不可变Value可由内容摘要验证，但“内容存在”只是一项可Inspect存储事实；当前对象、Retention、引用计数、Checkpoint资格和Restore资格仍由对应Fact Owner及CAS决定。SQLite与RocksDB之间不假装分布式事务，必须通过Write-Ahead Journal收敛：

```text
proposed
  -> metadata_pending
  -> content_staged
  -> reference_committed
  -> visible
  -> reclaimable（仅在引用证明和Retention允许后）
```

任一步回包丢失或进程崩溃都只Inspect精确Journal/Object Ref；禁止以新身份重写或盲目重派。

### 2.3 Checkpoint、Snapshot、Restore与Rewind

- Runtime Checkpoint Fact Owner拥有原子create-once的`CheckpointAttemptFactV2 + CheckpointBarrierLeaseFactV2` bundle、immutable Effect Cut、成功时唯一的immutable `CheckpointConsistencyFactV2`及非成功Attempt Finalization；不存在独立`AcquireBarrier` mutation。Runtime Restore Owner已实现create-once `RestoreAttempt + new Instance/high Epoch/new Lease/Fence Reservation`、short-TTL Eligibility、Stage actual-point Enforcement/Settlement及Activation；它不拥有Continuity Plan、Review Verdict、Sandbox Snapshot/Stage或Context语义。live legacy `core.CheckpointSet/RestoreRequest`不因名称相同获得V2权威性。
- 各Participant拥有自身Snapshot语义、捕获、覆盖范围、Receipt、Inspect/CAS与Cleanup。
- Continuity拥有Checkpoint Manifest内容、Snapshot Ref关系、恢复覆盖/缺口图、Fork/Rewind/Restore Plan和Recovery Credential。
- 首个Profile的Required Participant集合已经管理线冻结为Runtime、Harness、Context、Sandbox、Memory-Knowledge五类；Review与Tool只冻结其Owner已形成的exact ref，不作为首版Participant，Model Invoker不成为Checkpoint Participant。
- Snapshot只是Participant状态材料；Continuity必须先对Manifest Fact CAS并create-once immutable ManifestSeal。只有Runtime独立复读Seal、Effect/Settlement水位和Required Participant后，才能原子提交只允许`consistent`的immutable `CheckpointConsistencyFactV2`并关闭Barrier。`partial | indeterminate | rejected`不得生成Consistency，只能由Runtime Finalize原Attempt并关闭Barrier。
- Partial、Unsupported、Unknown只能进入诊断Manifest，不能自动Restore。
- Restore必须创建新Instance ID、更高Instance Epoch和新SandboxLease；旧Instance不能被复活。
- Rewind首先是计划。首版只允许受治理的Workspace文件ChangeSet；Continuity形成exact Plan，Sandbox拥有实际View/ChangeSet并通过既有`praxis.sandbox/workspace-commit`治理链执行新的文件Effect。Tool、远程对象或外部系统补偿不进入首版；邮件、交易、网络请求等不可逆Effect只继承为历史事实、Settlement、Residual或后续补偿候选。
- Checkpoint Governance V2只冻结Owner公开的exact Fact Ref；最低包含`contract/schema + owner binding + ID + revision + digest`。现有`RuntimeStateRef`、`RunSessionRef`、`ContextGeneration`等裸字符串不能迁移为V2权威引用。
- V2 Manifest必须精确冻结Context Generation/Frame、Application/Runtime Attempt、opaque Runtime Settlement、Memory Watermark/View/Projection与Knowledge Snapshot/View/Projection引用；不得内联Context正文、Memory/Knowledge正文、Provider Session、Runtime Outcome或执行句柄。
- Continuity只拥有Manifest/Plan Fact与其Inspect/CAS；Checkpoint协调和资格、RestoreAttempt、Review Authorization、Permit/Fence、新Instance/高Epoch及最终Activation始终属于Runtime或对应领域Owner。
- Restore Plan admitted/submitted只表示Continuity保存了可审计Plan并可作为Runtime current Reader输入；Plan中的Identity仍只是proposal。Runtime create-once提交后才形成权威Attempt/Reservation，并在复读Plan与requirements current后Issue/Bind短TTL Eligibility。Eligibility只保存Review target/requirement/policy basis及Authority/Scope/Budget/Binding/Context requirement exact refs，不包含accepted Verdict、Review Authorization或Permit。Application之后独立形成Intent、Admission、Review/Authorization、Permit/Begin，并经Runtime actual-point Enforcement调用Sandbox Stage；Evidence/Settlement、Context物化全部成功后Runtime才激活reserved新Instance。不得用`activation_attempt`、transport kind或legacy RestoreRequest替代这条链。

### 2.4 Checkpoint/Restore Governance V2两阶段冻结

Checkpoint与Restore必须拆成两个可独立验收、不可跨越的阶段：

| 阶段 | 唯一Owner与职责 | Continuity产物 | 阶段完成事实 | 本阶段禁止 |
|---|---|---|---|---|
| 第一阶段：Checkpoint资格 | Runtime原子创建Attempt+Barrier bundle，拥有immutable Effect Cut、Participant协调、成功Consistency与非成功Finalization；各Participant拥有Snapshot/coverage/Inspect/CAS Fact | `CheckpointManifestFactV2` CAS、immutable revision 1 `CheckpointManifestSealFactV2`、诊断分类 | 成功：Runtime原子`Consistency + Attempt consistent + Barrier closed`；非成功：Runtime原子`Attempt incomplete/aborted/indeterminate + Barrier closed`且不生成Consistency | 独立AcquireBarrier；Continuity设置consistent；Receipt直接当Fact；Partial/Unknown生成Consistency；覆盖历史Seal |
| 第二阶段A：Restore Reservation/Eligibility | Continuity拥有`RestorePlanV2`；Runtime拥有Attempt、Identity Reservation与Eligibility | exact submitted Plan current projection、create-once Attempt/Reservation、short-TTL Eligibility、history/current Inspect与CAS | Eligibility current；lost reply只Inspect原Attempt/Eligibility | Continuity创建Runtime Fact；accepted Authorization进入Eligibility |
| 第二阶段B：Restore最小公共参考执行 | Application/Runtime/Review/Context/Sandbox各自Owner | Continuity仅保存Plan和最终exact refs | Intent→Admission→Authorization→Permit/Begin→Enforcement→Sandbox Stage→Evidence/Settlement→Context→Activation | Continuity执行；旧Instance复活；外部世界回滚；绕过trusted Assembler自升production |

第一波唯一接入顺序固定为：G6A Action Gateway/跨Owner验收→G6B Context Refresh→Harness G7 Checkpoint门→Runtime原子Attempt+Barrier bundle→immutable Effect Cut→Participant Reserve/治理/Inspect/CAS→Continuity Manifest CAS→immutable ManifestSeal→Runtime复读→成功时原子Consistency+CloseBarrier，或非成功时原子Finalize+CloseBarrier。流程到此停止；Restore只允许Plan/Ref shape验证与历史Inspect，后续运行链必须等待独立联合Review。

## 3. Owner与非Owner矩阵

| 对象/动作 | 唯一Owner | Continuity职责 | Continuity禁止事项 |
|---|---|---|---|
| Identity/Lineage/Instance/Lease/Fence | Runtime Control Plane | 保存精确引用与历史关系 | 创建、续租、撤销或修改 |
| Run Fact/ExecutionOutcome/Cleanup | Runtime Kernel/Settlement Owner | 投影Evidence与Settlement关系 | 从Harness Claim推导Outcome |
| Evidence ledger sequence/record chain | Runtime Evidence Ledger Owner | 读取、索引、投影、查询 | 第二sequence、双写或改写Record |
| Timeline Attempt/Event/因果图/查询Cursor | Continuity | create/Inspect/CAS、S1/S2后形成可见派生Fact与短时current projection | 把caller candidate、历史Event或Projection Ref当业务权威/current事实 |
| Artifact正文/revision/current/storage语义 | Artifact Owner | 只消费typed source projection并保存exact ref | 从路径、caller字段或generic Reader推导Artifact事实 |
| immutable Artifact Relation/历史索引 | Continuity | Timeline+typed Artifact Owner S1/S2后create-once，按Artifact/Related exact ref查询 | 升级为Artifact current、Review Verdict、Tool Result或Effect Outcome；提供production Attach直写 |
| immutable `CheckpointConsistencyFact` | Runtime | 保存exact ref，作为Restore Plan历史输入 | 自行宣布consistent；把它解释为current eligibility |
| `RestoreEligibilityFactV2` | Runtime Restore Owner | Continuity只提交exact Plan current Projection并保存返回Ref | 创建、续期、current解释、把Eligibility当Permit或声称Stage能力存在 |
| Checkpoint Manifest/Seal | Continuity | Manifest形成/Inspect/CAS；Seal revision 1 create-once/Inspect/Retention | 修改Participant Snapshot语义、覆盖历史Seal或宣布Runtime consistent |
| Snapshot | 各Participant | 保存Ref/Digest/Owner/Coverage | 捕获或解释其他组件内部状态 |
| Harness Session | Harness | 保存精确Session snapshot ref（未来P1） | 读取Harness kernel/internal或声称跨Instance恢复 |
| Workspace ChangeSet | Sandbox | 绑定Timeline、Checkpoint与Rewind Plan | 实际改文件或选择性Commit |
| Context Frame/Generation | Context Engine | 保存引用和代际关系 | 组装或渲染Context |
| Review Verdict | Review Owner | 关联Case/Verdict/Evidence | 决定审核结果 |
| Tool/MCP Effect与Settlement | Tool/MCP/Settlement Owner | 保存Intent→Permit→Attempt→Settlement关系 | Dispatch、Inspect或结算工具效果 |
| Memory/Knowledge正式事实 | Memory/Knowledge Owner | 保存候选和来源关系 | 晋升、合并、撤回正式事实 |
| Retention/Legal Hold | Continuity Policy Owner；隐私/合规策略为外部权威输入 | 执行领域CAS与受治理删除计划 | 绕过Hold或无证据Physical Purge |

## 4. 部署与依赖方向

```text
SDK / CLI / API
  -> Application Coordinator（namespaced workflow）
     -> Runtime Command / Operation Governance / Evidence Governance
     -> Continuity Runtime Adapter
        -> Continuity Domain Controller
           -> SQLite Fact Store
           -> RocksDB Content Store
           -> governed Remote Object Store（可选）

Runtime Checkpoint Governance V2（atomic Attempt+Barrier bundle）
  -> Checkpoint Participant Reservation/Governance Ports
     -> Harness / Sandbox / Context / Memory / Tool 等Owner
  -> Continuity CheckpointManifestGovernancePortV2
     -> ManifestFact CAS + immutable ManifestSeal
  -> Runtime CommitConsistencyAndCloseBarrier | FinalizeAttemptAndCloseBarrier
```

依赖规则：

- Continuity domain/kernel只依赖自身contract和自身ports。
- Continuity runtime adapter只可依赖`runtime/core`与`runtime/ports`。
- 禁止依赖`runtime/foundation`、`runtime/fakes`、`runtime/kernel`内部实现、Harness kernel/fakes/internal。
- Harness私有`ContextPort`、`ModelTurnPort`、`EventCandidatePort`不是Continuity公共接入面。
- 跨组件只由Application Coordinator和版本化Port关联；不得导入其他组件实现包。

## 5. 领域对象与版本字段

所有可持久对象必须使用canonical serialization并包含以下共同字段：

- `contract_version`、`schema_ref`；
- 稳定`id`、`revision`、`digest`；
- `tenant_id`、Identity+Epoch、Lineage+Plan Digest、Instance+Epoch、适用Run/Effect/Lease、Authority Epoch；
- `owner_binding`：BindingSet ID/revision、Component ID、Manifest/Artifact Digest、Capability；
- `created_unix_nano`、`updated_unix_nano`、必要TTL；
- `idempotency_key`与稳定作用域摘要；
- 精确Evidence/Fact/Parent/Causation/Correlation引用；
- `residual_refs`与数据分类/Retention引用。

核心对象：

| 对象 | 关键字段/不变量 |
|---|---|
| `TimelineProjectionRequestV1` | 只允许stable Attempt/idempotency、EvidenceSourceKey、optional expected RecordRef、authoritative-only OwnerFact ref、Policy、Scope、RequestedNotAfter；无caller semantic/payload/可信值 |
| `TimelineProjectionAttemptFactV1` | Continuity Owner exact identity/revision/digest、Request闭集digest、Evidence与按需Owner Fact密封S1/S2、Reader派生Event digest、共同checked-at/not-after、`reconcile_required`状态、Event ref |
| `TimelineEventRecordV1` | 全部内容由S2 Readers派生；create后revision/digest/bytes永久immutable，不含可变Visibility/TombstoneRef |
| `TimelineProjectionTombstoneFactV1` | immutable revision 1，exact Event/policy/reason；与Visibility/current overlay index同事务create/CAS，不修改Event |
| `TimelineProjectionCurrentV1` | exact Attempt/Event、Reader binding/S1/S2/Policy projection digests、checked-at/not-after；只在全部Owner current上限最小窗口内有效 |
| `TimelineProjectionPolicyCurrentV1` | Continuity Owner opaque Policy exact ref/revision/scope/digest、closed current state与自然Checked/Expires；history/current/CAS，caller bound不参与密封 |
| `TimelineCursorV1` | ledger scope digest、after sequence、query digest、projection revision、expiry；不同query不能复用 |
| `ObjectManifestV1` | content digest、ordered chunks、compression/encryption envelope、owner、classification、retention、length |
| `CheckpointManifestFactV2`（Delta） | exact CheckpointAttempt/Barrier/EffectCut/TimelineCut refs；Generation/Frame、Attempt/Settlement、Memory/Knowledge与Participant Fact ref闭包；只给诊断状态，不给consistent/restore eligibility |
| `CheckpointManifestSealFactV2`（Delta） | immutable revision 1；exact绑定Manifest、Attempt/Barrier/EffectCut、frozen/required/runtime Participant set、RuntimeClosure refs及Context/Artifact closure digests；same-ID exact幂等、changed Conflict、lost reply只Inspect |
| `SnapshotBindingV1` | Participant binding、snapshot ref/revision/digest、coverage schema、storage/encryption ref、Inspect evidence |
| `ForkPlanV1` | source node/checkpoint、new lineage/session intent、parent relation、Authority/Profile/Binding重验要求 |
| `RewindPlanFactV2` | exact target Checkpoint/Manifest、source View、keep/drop/planned ChangeSet、dependency inspection、irreversible effects、Review requirements、residual、TTL/current |
| `CheckpointConsistencyFactV2`（Runtime live） | Runtime Owner参考实现已落地；immutable revision 1且结论只能`consistent`，绑定CheckpointAttempt/Barrier/EffectCut/ManifestSeal/Participant closures；不携current Restore资格；非成功不创建 |
| `RestoreEligibilityFactV2`（reference implemented） | Runtime create-once Attempt/Identity Reservation后原子Issue/Bind的short-TTL current Fact；exact绑定Plan/Attempt/Instance/Lease/Fence及Review/Authority/Scope/Budget/Binding/Context requirements，不包含accepted Authorization/Verdict/Permit，也不授Stage/Provider资格 |
| `RestorePlanV2`（owner-local + runtime current Adapter implemented） | 已实现Runtime immutable CheckpointConsistencyFact、Continuity ManifestSeal、frozen-ref-set digest、fresh Instance/high Epoch/new Lease/Fence proposal、Context与prerequisite exact refs、stable tenant Conflict Domain、TTL、create-once/CAS/history/current；Adapter只从exact submitted Plan、Seal、Consistency和prerequisite current派生稳定Projection，Runtime再建立authoritative Reservation/Eligibility，随后STOP |
| `RecoveryCredentialV1` | checkpoint/manifest/plan digest、subject scope、Authority/Policy、allowed actions、TTL；只保存凭证引用，不保存Secret正文 |
| `RetentionPolicyFactV1` | classification、hot/warm/cold策略、legal hold、tombstone/purge要求、policy revision/digest |
| `ContinuityWriteJournalV1` | operation ID、exact object/content refs、phase、attempt/revision、last receipt/inspection、next safe action |

`ObservedAt`是来源声明时间，`RecordedAt/IngestedAt`是账本接受时间；二者均为证据，不能决定跨Source因果或覆盖ledger sequence。

## 6. Event语义与Timeline查询

Event kind使用namespaced值并保留语义类别：Command、Intent、Candidate、Observation、Decision、Permit、Effect、Settlement、Claim、Control、Checkpoint、Cleanup，以及组件扩展kind。可信度只来自Evidence V2封闭TrustClass；仅`authoritative_fact`追加Owner Fact复读，其他五类禁止generic升权，kind字符串本身不授Trust。

live `TimelineQuery`已支持Identity、Lineage、Instance、Run、Turn、Step、Action、Artifact、Effect、Review Case、Checkpoint、Time Range、Parent、Causation与Correlation过滤；Turn至Checkpoint使用Event内已冻结的exact ObjectRef，多个维度按AND组合。所有过滤字段进入canonical Query digest，Cursor复用时任一维度漂移均Fail Closed。当前有效Revision由Evidence/Continuity current index与immutable历史关系决定。查询结果默认是受权、脱敏Projection；原始Payload按独立权限和Retention读取。

稳定Cursor规则：

1. 绑定Ledger scope、Query digest、Policy/Authority水位和Projection revision；
2. page内按Evidence ledger sequence稳定排序；
3. 新Projection可推进但不得让旧Cursor悄悄改变query语义；
4. Tombstone只改变可见性/正文可用性，不重排ledger sequence；
5. Watch发生gap、过期或权限漂移时返回显式错误并要求重新建Cursor。

## 7. Candidate、Admission、Review、Effect与Settlement链

Timeline Projection Request只提交坐标，随后经过Attempt create/CAS、Evidence sealed S1/S2，且仅`authoritative_fact`追加Owner Fact sealed S1/S2；它不是外部Effect，也不获得Dispatch权。live caller-driven Candidate只是reference fixture，不能进入production current。

任何外部、破坏性、远程持久或可能产生费用的动作，都必须达到Runtime公开治理链的同等门禁强度，不得在Continuity中重造或改序。现有通用Effect可复用已适用的Operation合同；Checkpoint使用V2/V1/V5 checkpoint phase合同，不能据此扩legacy闭表；Restore使用已落地的专用Stage/Enforcement/Evidence/Settlement/Activation合同：

```text
领域Intent/Reservation
  -> Admission
  -> Review/Authorization
  -> Permit
  -> Begin
  -> Delegation/Prepare
  -> Enforcement
  -> Execute/Inspect
  -> Observation/Evidence
  -> Participant DomainResultFact
  -> Runtime Operation Settlement
  -> Participant ApplySettlement
```

其中Review、Fence、Authority、Scope、Budget、Binding与Credential必须在Policy规定的Admission/Permit/Begin门禁中满足，实际执行点在Enforcement再次复读；Review Verdict仍由Review Owner持有。Begin后若丢失回包，只能Inspect原Attempt并沿同一冲突域完成Evidence、Participant DomainResultFact、Runtime Operation Settlement与Participant ApplySettlement，禁止换ID重派。

分流规则：

- Evidence Ledger append使用专用`EvidenceGovernancePortV2`，不再套一层重复Operation Effect。
- Timeline Projection可从Evidence重建；仅`authoritative_fact`还需Owner Fact current Reader，属于幂等派生写且不获得Dispatch权。
- Checkpoint Manifest、Fork/Rewind/Restore Plan是Continuity领域Fact，只有Owner CAS可形成当前revision；计划存在不等于已经执行。
- 远程Blob Put/Delete、Physical Purge、选择性Rewind ChangeSet、可能外发或计费的Compaction/Archive按其已登记的公开Operation合同执行；Restore Stage/Activate已使用专用公开合同、Evidence/Settlement与actual-point Enforcement，禁止借用`activation_attempt`、checkpoint phase合同或legacy闭表。远程Provider和production root仍须独立准入。
- Review是否必需由current Policy决定；高风险Rewind、Restore漂移、Privacy Purge、Legal Hold例外和远程披露不能由调用方自选跳过。

## 8. Checkpoint与恢复协议

### 8.1 Checkpoint

```text
Runtime atomic CreateCheckpointAttemptV2
  -> Attempt+Barrier bundle同时create-once；不存在独立AcquireBarrier
  -> Runtime freeze immutable per-attempt EffectCut
  -> Participant Owner ReservePhase
  -> Admission -> Review/Authorization -> Permit -> Begin
  -> prepare/execute Enforcement -> Provider -> Evidence V1
  -> Participant DomainResult -> Settlement V5 -> ApplySettlement
  -> Participant Owner Inspect/CAS exact Snapshot/Session/Generation/Frame/Memory/Knowledge facts
  -> Continuity CAS CheckpointManifestFactV2
  -> Continuity create-once immutable CheckpointManifestSealFactV2
  -> Runtime re-reads exact EffectCut/Participant closures/ManifestSeal
  -> success: atomic Consistency + Attempt consistent + Barrier closed
     OR failure: atomic Attempt incomplete|aborted|indeterminate + Barrier closed, NO Consistency
```

`CheckpointConsistencyFactV2`是immutable revision 1且只允许`consistent`。它至少要求全部Required Participant满足Policy、Effect Cut无未解释缺口、ManifestSeal可复读、Snapshot coverage满足要求及冻结水位一致。Partial/Unknown/Unsupported只进入Manifest诊断和Runtime Attempt Finalization，绝不生成Consistency。

Manifest中的所有跨Owner对象都必须使用Owner公开exact ref，不复制Owner状态枚举或结论。Generation必须由Context Owner证明包含对应Frame；每个已Begin Attempt必须关联exact Runtime Settlement，否则Checkpoint只能`diagnostic_indeterminate`。历史ref过期不删除审计价值，但Restore前必须重新做currentness/compatibility检查。

### 8.2 Restore Reservation/Eligibility最小链

```text
immutable CheckpointConsistencyFactV2 + CheckpointManifestSealRefV2
  -> Continuity exact submitted RestorePlanV2 current Reader
  -> Runtime create-once RestoreAttempt + fresh Instance/high Epoch/new Lease/Fence Reservation
  -> Runtime Issue/Bind short-TTL RestoreEligibilityFactV2
  -> Application immutable Intent
  -> Action Admission -> Review/Authorization -> Permit/Fence -> Begin
  -> Runtime Prepare/Execute actual-point Enforcement
  -> Sandbox Stage/Inspect -> Evidence -> Runtime Settlement(ref only) -> Sandbox ApplySettlement
  -> Context materialize new Generation/Frame
  -> Runtime Activate reserved new Instance/high Epoch/new Lease
```

Continuity Adapter复读exact Plan、immutable ManifestSeal与Runtime Consistency；Plan自然`Updated/Expires`形成稳定current projection，fresh now只Validate，不按每次读取重封。Runtime Gateway在Attempt create和Eligibility bind前分别执行S1/S2；回包丢失只Inspect原identity，Instance/Lease在Runtime原子create后才成为Reservation。Eligibility prerequisite漂移或TTL到界即current失败。旧Provider Session只可作为历史线索，未知Effect继续占用稳定tenant冲突域。

公共参考链只允许上述exact Owner序列；任一TTL、Authority、Scope、Fence、Review、Evidence、Settlement、Context或Residual门失败都必须Fail Closed。legacy `core.RestoreRequest`、`CheckpointParticipantPort.RestoreCheckpoint`、Runtime Foundation参考协调器或`activation_attempt`不得被包装成Stage/Activate能力。Host-Local Sandbox Stage不授远程Provider、跨Owner全量Participant或production root资格。

### 8.3 Rewind

Workspace-only `RewindPlanFactV2`已实现目标Checkpoint/Manifest、source Workspace View、expected revision/file scope、keep/drop/planned ChangeSet、依赖Inspection、Review Requirement、不可逆Effect与Residual exact refs，及create-once/history/current/CAS/TTL/lost-reply Inspect和SQLite schema v9持久化。执行权仍属于Runtime/Application协调及Sandbox Workspace Owner；Continuity只保存计划、关系与结果引用。Sandbox `WorkspaceRewindCompositionPortV1`现已按exact View与keep/drop refs组合新的staged ChangeSet并封存immutable Composition；Application/Assembler尚未将该exact结果接入既有`workspace-commit`治理链，因此跨Owner Rewind Apply继续NO-GO。

## 9. UnknownOutcome与恢复纪律

- 任一外部Effect在Begin后回包丢失：标记`unknown_outcome`，只Inspect原Attempt。
- Inspect本身是受治理Operation Effect，并用`relation`绑定原Effect；禁止递归无限Inspect链。
- Provider Receipt、Enforcement Receipt或Snapshot Report都不是正式Fact。
- SQLite/RocksDB跨存储未知：按`ContinuityWriteJournalV1`精确Inspect metadata/content/current ref，确定唯一下一安全动作。
- 如果无法证明内容是否写入，不生成新Object ID；如果无法证明引用是否提交，不Physical Purge。
- Projection损坏可以从Evidence与Manifest重建；原始Evidence、已提交Continuity Fact与内容寻址对象不能只依赖Projection。

## 10. Retention、Compaction与审计

- Hot/Warm/Cold是Policy而非固定容量/SLA。
- Compactor/Indexer/Consolidator只能生成新Projection、Summary或Candidate，不得无痕重写Event。
- Tombstone、Privacy Erasure、Retention Expiry与Physical Purge是不同状态。
- Legal Hold、Checkpoint/Fork/Review/审计引用优先于普通回收；没有引用闭包证明不得删除Chunk。
- 敏感Snapshot必须加密；Manifest只保存加密Envelope、Key reference和算法/版本元数据，不保存长期明文Secret。
- Key不可用、远程Retention未知或删除不可Inspect时形成Residual并阻止虚假Cleanup完成。

## 11. SDK、CLI与API边界

### Go SDK

live `sdk.Client`已提供Transport-neutral的Inspect Event、Query/Watch Timeline、Inspect/List Artifact Relation、Inspect Content Integrity Audit/Content Delta、Inspect Manifest/Seal/RestorePlan/Retention及Fork/Rewind/Restore Plan、Recovery Credential纯Validate；Query/Watch在Reader返回后复验每条Event、全部过滤条件、严格sequence、PageLimit、Cursor query/currentness与exact page watermark，伪造输入Cursor在Reader调用前拒绝。SDK不持有Fact Store或Runtime内部接口。唯一写面是`Submit/InspectGovernedWorkflow`，它经`applicationadapter.GovernedWorkflowAdapterV1`调用Application公开Gateway，不能直达Continuity Fact Store、Application Facade或Provider。真实写能力仍取决于production trusted Assembler/current Readers/root；缺失时Fail Closed。

### CLI

V1范围已经用户裁决为**包含受治理写面**。Continuity自有映射已实现`praxis timeline show/watch`、`praxis checkpoint inspect`，以及`praxis timeline project`、`praxis checkpoint create`、`praxis fork`、`praxis rewind plan`、`praxis restore`、`praxis artifact attach`、`praxis retention resolve`和`praxis workflow inspect`。只读命令经SDK exact Reader并strict decode；全部写命令只提交Application namespaced workflow，不直接调用Continuity Fact Port、Runtime legacy Command enum或Provider。根CLI注册、endpoint、credential与production redaction policy仍由外部Owner提供。

写命令存在不等于底层Capability已启用：`restore`、生产Checkpoint Participant、remote purge/archive等在相应公共Port/current治理链未闭合时必须返回`unsupported`且Provider调用为零。CLI不得接受调用者自报的Permit、Review Verdict、Authority current、Fence、Provider binding、Runtime Outcome或Application内部`SubmissionBundleV2`。

### API

V1同时冻结只读资源与受治理写请求：Timeline Projection、Checkpoint Create、Fork、Rewind Plan、Restore、Attach Artifact、Resolve Retention。写请求只携stable Request/Idempotency、目标Execution Scope、领域coordinate/plan exact ref、expected revision及caller not-after；可信Binding/Authority/Review/Budget/Fence/Provider/current全部由Application/Runtime/领域Owner复读。API不预选HTTP/gRPC/RPC拓扑。所有长任务返回Application Command/Plan/Journal或领域Attempt exact ref，客户端只Inspect，不以超时触发重派。

## 12. Conformance与能力声明

| 级别 | Continuity要求 |
|---|---|
| `fully_controlled` | 单主Evidence sequence、精确Fact Inspect/CAS、可恢复跨存储Journal、双重门禁、加密Snapshot、Retention引用证明、Unknown只Inspect、全部Required测试通过 |
| `restricted_controlled` | 能明确列出残留/能力限制并Fail Closed；不宣称缺失的Checkpoint/Restore/远程删除能力 |
| `contained_observe_only` | 只提供受权Timeline投影/查询，不创建恢复资格或持久外部Effect |
| `rejected` | 第二sequence、无Inspect的持久Effect、敏感明文Snapshot、不可Fence网络/删除、Provider自报即正式Fact |

SQLite/RocksDB默认实现、远程Provider和任何Fake都必须分别通过Conformance；Fake只能证明测试语义，不能声明生产Backend、SLA或持久性。

## 13. 技术选择

- 核心、合同、状态机、存储协调与Adapter默认使用Go。
- SQLite/RocksDB具体Go driver、CGO策略、版本和编译分发方式在实现评审时选择，不在设计中锁死第三方包。
- 当前没有已证明的计算稠密热点，不规划Rust。
- 只有Go基准、CPU/内存profile证明某个纯计算内核主导成本，且常规Go优化无法满足经用户批准的目标后，才允许另立Rust设计；必须同时定义Go边界、FFI或进程隔离、panic/崩溃/超时/取消/内存所有权和回退语义。

## 14. 兼容与迁移

1. 旧`TimelinePort`、`core.CheckpointSet/RestoreRequest`、`CheckpointParticipantPort`与Foundation参考协调器保持legacy restricted，不原地扩字段、不补默认Review/Fence，也不提升权威性。
2. 新合同使用新version/type/Port并与V1并存；没有字段完备的显式迁移工具，不做V1→V2默认补值。
3. 禁止Evidence V2与Continuity Event Ledger双写；迁移时只从Evidence V2重建Timeline Projection。
4. SQLite schema与RocksDB keyspace都使用显式schema version；升级先写兼容读取和校验，再切current writer。
5. 迁移必须可停在旧reader、新reader双读验证阶段；不得双主写入。
6. Snapshot/Checkpoint迁移若无法证明coverage、digest、owner与encryption metadata，降级为诊断资产，不能自动Restore。

## 15. 仍需联合裁决

以下问题不阻止本设计进入联合评审，但在实现对应阶段前必须有管理线/Owner明确决定：

1. **已裁决（2026-07-18）**：首个Profile的Required Participant为Runtime、Harness、Context、Sandbox、Memory-Knowledge；Review/Tool只冻结exact refs，Model Invoker不是Participant。Optional扩展与更细coverage policy仍由相应Owner后续版本化。
2. Harness是否实现Checkpoint Session seam，以及可恢复到Turn/Step的最小粒度；
3. Runtime P1 CheckpointFact/Barrier/RestoreAttempt合同及Application namespaced workflow接线；
4. **已裁决（2026-07-18）**：首批Rewind只允许Workspace文件ChangeSet；Review仍由current Policy决定；不可逆外部Effect只继承事实/Settlement/Residual或补偿候选，不执行外部世界回滚。
5. Retention/Legal Hold/Privacy Erasure的权威Policy Owner和首批数据分类；
6. **已裁决（2026-07-18）**：远程对象库、远程Purge/Archive、KMS与跨区域披露不进入本机首版，继续走provider-neutral SPI并保持unsupported。
7. **已裁决（2026-07-18）**：本机默认使用现有SQLite+RocksDB实现，不承诺SLA；driver与build-tag边界按live实现和真实benchmark维护，部署不预选远程拓扑。
8. 容量、延迟与保留成本只记录实测benchmark，不形成首版SLA；企业千Agent目标须由后续部署Profile独立裁决。
9. **已裁决（2026-07-18）**：CLI/API V1包含受治理写面；任何写命令必须经Application公开Submission+Inspect Gateway，缺少CompiledGraph/Binding/Capability/current门时Fail Closed，不得直达Fact Store。

## 16. 设计产物索引

- 本文件：总体边界、架构与冻结裁决；
- `contracts-and-state-machines.md`：对象、状态机、失败与接口语义；
- `integration-mapping.md`：公共/私有Port、Slot/Phase贡献、依赖DAG与Runtime/Harness/Application映射；
- `port-delta.md`：公共合同缺口；
- `acceptance.md`：设计验收、反例和评审门禁；
- `architecture.drawio`：部署区、Owner、存储和调用关系图。
