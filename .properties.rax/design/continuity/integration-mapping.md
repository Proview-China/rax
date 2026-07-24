# Continuity 公共装配与跨模块映射

状态：**Checkpoint-first纵切及Restore最小公共参考纵切已落地**。Restore已覆盖Application Intent、Runtime Reservation/Eligibility、Admission/Review/Permit/Begin、双重Enforcement、Sandbox Stage、Evidence/Settlement、Context materialization与Activation；production trusted Assembler、跨Owner全量Participant与root仍未闭合。

## 1. 目的

本文只声明Continuity对Runtime/Harness/Application统一装配的贡献和需求，不定义新的公共Slot、Hook或Phase枚举。所有名称必须来自Harness接线线最终冻结的namespaced、版本化公共对象。

## 2. 公共与私有Port

| Port/合同 | 性质 | Continuity使用方式 |
|---|---|---|
| Runtime Binding V2 / Manifest V2 | 公共 | 声明Continuity能力、Locality、Owner、Schema、依赖、Residual与Conformance |
| Runtime Operation Effect V3 | 公共 | 外部/破坏性/远程持久动作治理；不重造Permit链 |
| Runtime Review V2 | 公共 | 绑定需要审核的Rewind/Restore/Retention候选 |
| Runtime `EvidenceSourceRecordReaderV2` | 公共只读 | C-01按Source Key与Reader返回的exact Record Ref做S1/S2；sequence/digest/Trust由Reader派生，不接受caller副本 |
| Runtime Evidence current/readability Reader（R-CTY-06） | additive Delta，待Evidence Owner终审 | 提供source/policy/tombstone/readability及共同TTL上限；不授append权 |
| 领域Owner Fact current Reader | 各Owner公开只读Port，按Fact kind装配 | **仅**`authoritative_fact` S1/S2；其余五类禁止借generic Reader升级，caller不能注入Reader或自报current |
| Artifact Owner Relation Source Reader | consumer-side typed只读Port，按Artifact Owner装配 | C-05只读取Owner密封的Artifact/storage/parent/origin与Related exact历史关系；不授Artifact current或写权 |
| Continuity Content Integrity Audit V1 | 组件公共读/受控Owner写 | bounded Object/Journal双轮读取后形成immutable诊断Fact；无Purge/Retention/Provider写面 |
| Continuity Content Delta V1 | 组件公共读/受控Owner写 | Base/Target exact Object双轮读取后形成immutable结构共享Fact；无patch execute/Compaction/Purge写面 |
| Continuity History Derivation Candidate V1 | 组件公共读/受控Owner写 | exact Event集合+output Object形成candidate-only Fact；无publish-current/Event rewrite/Compaction/Purge写面 |
| Runtime Run Settlement V2/V3 | 公共 | 声明Continuity Run Requirement并提交Owner Participant Fact |
| Runtime legacy Timeline/CheckpointParticipant | 公共但restricted | 只作兼容识别，不承载完整Continuity语义 |
| Runtime `core.CheckpointSet/RestoreRequest`与Foundation协调器 | legacy reference | 只用于识别历史/测试合同；不得补默认Review/Fence、不得包装为Governance V2 |
| `RestoreGovernancePortV2`、typed Restore scope、RestorePlan current Adapter | 公共Restore链 | exact Plan/Seal/Consistency→Attempt/Identity Reservation→short-TTL Eligibility；后续Admission/Stage由Application/Runtime/Sandbox公开Port承担，Continuity不执行 |
| Harness `ports.ContextPort` | Harness私有 | 禁止依赖或实现 |
| Harness `ports.ModelTurnPort` | Harness私有 | 禁止依赖或实现 |
| Harness `ports.EventCandidatePort`/Journal | Harness私有 | 禁止作为Continuity公共Event入口；Harness通过Application/Evidence公共链提供候选 |
| Harness Governed Session/Candidate Fact Ports | Harness Owner私有/公共接线受控 | Continuity只持精确Fact/Evidence ref，不读写Harness Session Fact Store |
| Continuity Domain Fact/Query Ports | 组件公共 | C-01/C-02/C-03/C-05/C-06/C-07/C-08 owner-local代码已实现；旧caller Candidate服务仅为`ReferenceTimeline`，raw repository不是Attach/Audit/Delta/Derivation API；production root只能装配治理Controller；domain/kernel只依赖自身合同 |

## 3. Slot/Phase贡献声明

Continuity不定义Slot/Phase常量。以下仅是语义贡献类型，最终Slot/Phase ID、顺序和合并规则由Harness接线设计冻结：

| 贡献类型 | 语义需求 | 输入 | 输出 | 禁止事项 |
|---|---|---|---|---|
| Observer | 在公共治理链已持久化Evidence后请求同TrustClass投影；仅authoritative_fact关联Owner Fact | 仅请求Source/Record坐标、Scope、RequestedNotAfter；authoritative_fact再带Owner Fact exact坐标 | Attempt ref；S1/S2完成后才可返回Event/readability-current projection | 不提交可信sequence/digest/Trust/current；不把receipt/attestation/claim经generic OwnerFact升级；不修改Run/Context、不联网 |
| Gate | Checkpoint Barrier时判断Harness Session是否可贡献Snapshot | Runtime Barrier/Checkpoint attempt ref、Harness current Session ref | prepared/unsupported/partial/unknown Observation | 不自行阻止Runtime Run，不宣布Checkpoint consistent |
| Port | Checkpoint Participant Commit/Inspect reference Port已落地；生产Reserve/Prepare/Abort与phase接线后置 | namespaced versioned checkpoint对象 | Participant Owner Reservation/report/fact | 不用通用Hook写Fact，不越过Owner；Restore Stage后置 |
| Filter | 默认无贡献；只有未来明确的Timeline查询脱敏公共Phase才可声明 | authorized projection | bounded redacted projection | 不改Context，不执行网络/持久Effect |

当前不声明任何可在任意Phase修改Context、联网、写Fact或Dispatch Effect的通用Hook。Checkpoint Gate与reference Participant调用映射已闭合；production Participant phase/Provider及Assembler root仍未闭合，Continuity只引用公共对象，不实施私有替代品。

## 4. Runtime/Harness/Application映射

### 4.1 Event进入Timeline

```text
Harness/组件Owner Observation或领域Fact
  -> Application namespaced workflow
  -> Runtime Evidence Governance append（若尚无Record）
  -> caller只提交Projection Request坐标
  -> Continuity create-once Projection Attempt
  -> Runtime Evidence exact Reader S1（Source定位 + Record复读）
  -> 仅authoritative_fact调用领域Owner Fact current Reader S1
  -> 相同Reader bindings完成Evidence/Owner S2逐字段复读
  -> sealed Evidence projection同时覆盖source/policy/current、readability、Tombstone或absence watermark、subject-current index
  -> 对同一immutable sealed projections exact比较；fresh now只ValidateCurrent
  -> Owner Readers自然sealed输出不接收RequestedNotAfter，S1/S2 exact后fresh now只ValidateCurrent
  -> Continuity计算全部Owner自然上限的共同min-TTL
  -> RequestedNotAfter 0=无caller上限、<0拒绝、>0仅在最后截短；不进入Owner Digest
  -> Continuity CAS admitted
  -> 原子Event + Attempt visible + current index
```

Harness terminal Event仍按现有Run Claim链进入Runtime；Continuity不能截获并选择Outcome。production形状`TimelineProjectionAdapterV1.Project/Rebuild`已经统一走Readers/Attempt/S1/S2/fresh/atomic publish；旧caller Candidate服务显式为`ReferenceTimeline`且不得由production root装配。public `ReplaceLedgerScope`与`TombstoneProjection`已删除，Tombstone使用immutable Fact+overlay；production装配仍等待真实typed Owner Readers与Application root。

Rebuild不是第二条导入捷径。production Rebuild只提交Request闭集或Request ref，并逐Item复用上述同一create-once Attempt、S1/S2、fresh current与原子提交Controller；某Item unknown只Inspect该Item原Attempt，禁止整批caller Candidate/Event import、Scope替换、history/overlay覆盖或current index回退。

Tombstone不改Event。Continuity必须create独立immutable revision 1 Tombstone Fact，并在同一Owner线性化事务中CAS Visibility overlay/current index；Query/Watch组合历史Event与overlay。Tombstone前后exact Event revision/digest/canonical bytes必须一致，删除overlay或Rebuild均不得使已tombstoned Event静默复活。

### 4.2 Checkpoint

```text
Application Checkpoint workflow
  -> Runtime原子create-once CheckpointAttempt+Barrier bundle（Runtime Owner参考纵切已实现）
  -> Runtime freeze immutable EffectCut
  -> Harness及组件公共Participant Reserve/治理Port（reference接线已闭合；production phase/Provider待闭合）
  -> Participant各自Prepare/Commit + Owner Inspect/CAS Fact
  -> Context冻结exact Generation/Frame refs
  -> Effect Cut逐项冻结Application/Runtime Attempt与opaque Settlement refs
  -> Memory/Knowledge Owner提供exact Watermark/View/Projection/Snapshot refs
  -> Continuity CAS CheckpointManifestFactV2 ref-only candidate
  -> Continuity create-once immutable CheckpointManifestSealFactV2
  -> Runtime按完整Owner/Scope exact lookup读取Seal
  -> Runtime current Participant Set/closures S1
  -> Continuity Adapter逐项校验RuntimeClosureRef、Context/Artifact/Frozen digests
  -> Runtime current Participant Set/closures S2 + 同一Seal exact复读
  -> success: atomic Consistency+Attempt consistent+Barrier closed
     OR non-success: atomic Attempt Finalization+Barrier closed, NO Consistency
```

Harness已暴露Checkpoint Gate/Guard及Participant公开Adapter，并完成reference组合测试；`ControlCapabilities.Checkpoint`字段本身仍不等于production Loop/phase/Provider能力。

Checkpoint不复制被引用对象的正文或Owner语义。所有跨Ownerref最低包含`contract/schema + owner binding + TenantID + ScopeDigest + ID + revision + digest`；Runtime typed Participant closure只能通过公开`DeriveCheckpointParticipantClosureExactRefV2`映射，不得由Application扫描或拼接。现有裸`RuntimeStateRef/RunSessionRef/ContextGeneration`不能作为V2输入。每个已Begin Attempt缺少exact Settlement时只能形成indeterminate诊断资产。

### 4.3 Restore最小公共参考链

```text
historical CheckpointConsistencyFactV2 + CheckpointManifestSealRefV2
  -> Continuity exact submitted RestorePlanV2 current Reader
  -> Runtime create-once Attempt + fresh Identity Reservation
  -> Runtime Issue/Bind short-TTL Eligibility
  -> Application immutable Intent
  -> Admission -> Review/Authorization -> Permit/Fence -> Begin
  -> Runtime Prepare/Execute actual-point Enforcement
  -> Sandbox Stage/Inspect -> Evidence -> Runtime Settlement(ref only) -> Sandbox ApplySettlement
  -> Context materialize new Generation/Frame
  -> Runtime Activate reserved new Instance/high Epoch/new Lease
```

Continuity不调用Harness kernel、Sandbox实现、Context实现或Model Invoker internal。Continuity拥有Plan/Manifest/Seal并只提供Plan current Adapter；Runtime/Application/Sandbox/Context通过各自公开Port完成后续链。不得用legacy、`activation_attempt`或transport kind替代资格；Host-Local参考Stage不授远程Provider或production root资格。

## 5. 依赖DAG

```text
Agent Assembler / Assembly SDK / CompiledGraph（公共装配Owner）
  -> Binding V2 Manifest/Capability/Schema映射
     -> Application namespaced Workflow与Step Journal
        -> Runtime Operation/Evidence/Review/Run Settlement公共链
           -> Continuity Runtime Adapter
              -> Continuity Domain Contracts
                 -> SQLite Fact Store
                 -> RocksDB Content Store

Harness公共Slot/Phase合并规则
  -> Checkpoint Participant接线
     -> Harness Snapshot贡献
     -> 其他组件Snapshot贡献
        -> Runtime Checkpoint Attempt+Barrier bundle/EffectCut
           -> Continuity Manifest Fact + immutable Seal
              -> Runtime Consistency or Attempt Finalization
```

禁止反向依赖：Continuity storage/domain不得依赖Runtime/Harness/Application实现；Continuity不得被其他组件实现包直接导入。

## 6. 当前公共装配硬阻塞

| 阻塞 | Continuity影响 | 本设计处理 |
|---|---|---|
| Agent Assembler最终Profile输出与Required Participant policy未冻结 | 无法冻结首个Required/Optional集合、Run Requirement与Capability DAG | 只声明Manifest/Requirement候选，等待Assembler/管理线给出Profile事实 |
| Assembly SDK / CompiledGraph公共合同已存在，但Checkpoint运行时映射缺失 | 编译图可表达贡献，尚不能证明Participant seam被运行时调用 | 不私建CompiledGraph；由Harness/Assembler Owner补Checkpoint PortSpec和调用映射 |
| 公共Slot/Phase对象已存在，但Checkpoint Gate/Observer未接执行器 | 不能仅凭catalog声明启用Checkpoint | Continuity只声明Observer/Gate/Port贡献，不定义新枚举或执行顺序 |
| Binding V2已有live基线，Checkpoint/Restore exact binding映射未冻结 | Manifest/Attempt无法证明绑定到同一编译Profile和Execution Scope | 复用live Binding V2；只请求Checkpoint/Restore映射Delta |
| Harness G7 Checkpoint门与reference运行时映射已闭合，production phase/Provider未闭合 | reference纵切可接通两个Participant，但不能据此启用生产Snapshot capture | 保留公开Port与Conformance；production root不得装配测试fixture |
| G6A Action Gateway跨Owner验收未闭合 | Checkpoint Participant外部phase不可执行；Restore仍后置 | Checkpoint只复用既有Gateway；不得直调Participant或复用legacy Port |
| Runtime Checkpoint-first V2、C-02及跨Owner reference接线已实现 | Harness Gate→Runtime→两个测试Participant→Continuity→Runtime Consistency→Gate release可组合验证；production仍不可实施 | Provider零调用；由各Owner另行完成真实phase、Snapshot Provider、production root与联合Conformance后再启用 |
| C-01跨Owner typed Owner routing与production root未闭合 | Continuity Adapter已复用R-CTY-06窄Reader并统一Project/Rebuild；旧路径为`ReferenceTimeline` | Assembler只为authoritative_fact绑定真实typed Owner Reader并证明production root不装配Reference/raw Store |
| Artifact Relation typed Owner route与Application root未闭合 | owner-local S1/S2 Controller、immutable Fact与SQLite索引可独立验证，但不能证明真实Artifact Owner source projection | 由Artifact/Application Owner提供版本化typed Reader与route Conformance；缺失时Capability reference-only且SDK无Attach写面 |
| Restore production装配后置 | 历史Consistency/ManifestSeal可能被误当current恢复资格或执行Permit | exact Plan current、Eligibility、Admission/Authorization、Enforcement、Stage Settlement、Context与Activation逐门复读；trusted Assembler/root缺失时Capability仍unsupported |
| per-turn Context refresh接线未统一 | 目标Instance无法证明exact Generation/Frame已物化 | Restore Fail Closed/Residual；Checkpoint只保存历史exact refs，不宣称普遍恢复 |

这些阻塞不否定已经实现的Continuity C-02、Checkpoint-first reference纵切及Restore Reservation/Eligibility最小参考链，但阻止生产Checkpoint与Restore执行面启用。

### 6.1 G6A/G6B之后的接入顺序

公共接入只允许以下单一顺序；G6A中的Action Gateway是既有基座，后续不得重新实现：

```text
G6A Action Gateway / cross-owner acceptance
  -> G6B Context Refresh
  -> Harness G7 Checkpoint gate
  -> Runtime atomic Attempt+Barrier bundle
  -> immutable EffectCut
  -> Participant Reserve/Admission/Review/Permit/Begin/Enforcement
  -> Harness/other Participant Fact closures
  -> Continuity Manifest CAS + immutable ManifestSeal
  -> Runtime fresh reread
  -> atomic Consistency+CloseBarrier OR Finalize+CloseBarrier
  -> STOP; Restore execution remains disabled
```

这是Checkpoint第一波唯一Capability接入顺序，不把G6A/G6B实现复制进Continuity，不允许Continuity修改Runtime/Harness/Context事实。Restore已有Reservation/Eligibility最小参考链；Restore专用Action/Stage路由尚不属于本波公共装配。

## 7. Effect/Review/Fence/Unknown映射

| 场景 | Effect链 | Review | Fence/Scope | Unknown |
|---|---|---|---|---|
| Timeline Projection | Evidence专用治理+Continuity Attempt CAS；无第二Operation Effect | 通常不需要；由Projection Policy决定敏感读取 | Evidence sealed S1/S2；仅authoritative_fact追加Owner sealed S1/S2；共同min-TTL | Inspect原Attempt/Event；reconcile_required不可见 |
| Artifact Relation | Continuity create-once本地Fact；无外部Effect | 无执行Review；读取仍受Projection/Retention Policy约束 | Timeline exact Event + typed Artifact Owner source projection S1/S2 | lost reply只Inspect原Relation；typed Router缺失则unsupported且零Fact |
| Content Integrity Audit | Continuity create-once本地诊断Fact；无外部Effect | 无执行Review；不形成Cleanup资格 | expected Scope/Manifest + Object/Journal/Chunk S1/S2 | lost reply只Inspect原Audit；漂移/不可用为indeterminate，零删除 |
| Content Delta | Continuity create-once本地关系Fact；无外部Effect | 无执行Review；不形成Compaction/Purge资格 | Base/Target expected Manifest + visibility + Chunk bytes S1/S2 | lost reply只Inspect原Delta；读取unknown零Fact |
| History Derivation Candidate | Continuity create-once本地候选Fact；无外部Effect | 无执行Review；不形成publish/execute资格 | Timeline Event exact refs + output Object/Chunk S1/S2 | lost reply只Inspect原Candidate；任一漂移零Fact |
| Remote Blob Put | Operation V3完整链 | 披露/区域Policy | Host Permit+provider执行点双验 | Inspect原provider operation |
| Checkpoint Prepare/Commit/Abort | Runtime Attempt+Barrier bundle、Participant Reservation、Checkpoint Evidence V1/Settlement V5 | Policy决定 | exact Attempt/Barrier/EffectCut/Scope/Binding/current gates | Participant Inspect原phase；Unknown不重派 |
| Restore Workspace Stage | Application/Runtime/Sandbox专用公开链 | Policy决定Review requirement；Authorization独立绑定exact Attempt/Eligibility | typed scope、Permit/Fence与Prepare/Execute actual-point Enforcement双验 | Inspect原Enforcement/Workspace attempt；unknown不重派Effect |
| Rewind Apply（首版） | Sandbox Workspace Owner既有`praxis.sandbox/workspace-commit`治理链 | exact submitted Rewind Plan + source View + planned ChangeSet + current风险Policy | new Operation subject、稳定tenant/Workspace Conflict Domain与actual-point Fence | Owner Inspect原Effect；Tool/远程补偿不装配 |
| Retention Purge | Continuity Operation V3 | Hold/Privacy Policy | tenant稳定域+Object set digest | Inspect原delete/purge |

## 8. Settlement、Cleanup与Residual

- Continuity只提交自身Effect的Settlement和自身Run Requirement Participant Fact。
- Snapshot、ChangeSet、Tool、Context、Harness Session等由各自领域Owner形成DomainResultFact并执行ApplySettlement；每次Effect的Operation Settlement仍唯一归Runtime。
- Runtime聚齐所有Requirement后才能CompleteRun；Continuity不能从自身`timeline-durability`推导Run Outcome。
- Content orphan、remote retention unknown、key unavailable、projection gap、checkpoint partial分别形成精确Residual；不得折叠成单一Cleanup状态。
- Cleanup完成必须有对象引用闭包、Provider Inspect、Retention Policy与Evidence；Provider“删除成功”Receipt不是Cleanup事实。

## 9. 冲突检查

| 冲突 | 必须结果 |
|---|---|
| Evidence Ledger与Timeline都分配sequence | 拒绝；Timeline只能复用Evidence Record sequence |
| `Project`把caller Candidate/admission直接交给`PutProjection` | 拒绝；production只接受Request闭集，并按Attempt→S1→current→S2→fresh→atomic commit执行 |
| `Rebuild`批量caller Candidate/Event并调用`ReplaceLedgerScope` | 拒绝；逐Item走同一Controller，禁止bulk import、Scope替换、history/overlay覆盖 |
| `TombstoneProjection`原地修改Event的Visibility/TombstoneRef | 拒绝；另建immutable Tombstone Fact并CAS overlay/current index，历史Event零改写 |
| legacy `PutProjection/ReplaceLedgerScope/TombstoneProjection`被production Adapter装配 | 拒绝；这些写口只可reference/internal，生产面只能装配C-01 Governance Controller |
| Harness EventCandidatePort被当公共Port | 拒绝；改走Application/Evidence公共链 |
| 组件私建Checkpoint Slot/Phase | 拒绝；等待Harness公共namespaced对象 |
| Runtime公开独立AcquireBarrier或出现Attempt-only/Barrier-only半对象 | 拒绝；Attempt+Barrier必须由Runtime Owner原子create-once并以bundle Inspect |
| partial/indeterminate/rejected被写成CheckpointConsistencyFact | 拒绝；Consistency只允许consistent；其余由Attempt Finalization+CloseBarrier收口且不生成Consistency |
| mutable Manifest/candidate被Runtime Consistency直接绑定 | 拒绝；必须是Continuity immutable revision 1 ManifestSeal exact ref |
| 同Seal ID换Manifest revision/digest、Attempt/Barrier/EffectCut或Participant closure | Conflict；原Seal不可覆盖，lost reply只Inspect原Seal |
| immutable CheckpointConsistencyFact或ManifestSeal被直接解释为current Restore资格 | 拒绝；只能经exact submitted Plan与Runtime prerequisite复读形成short-TTL Eligibility，且Eligibility仍不授执行资格 |
| V2 Manifest继续使用裸Runtime/Session/Generation字符串 | 拒绝；改为Owner公开ID/revision/digest exact ref |
| Generation与Frame来自不同Context闭包 | 拒绝；Context Owner Inspect失败，Checkpoint降级诊断 |
| 已Begin Attempt没有同Attempt Settlement仍标verified | 拒绝；indeterminate并保留inspection/residual |
| Provider Snapshot Receipt直接进入Manifest committed | 拒绝；Participant Owner必须Inspect/CAS |
| Model Invoker Raw/Native Event绕过Evidence Ledger直接进入Timeline | 拒绝；公开union Observation须先成为精确Evidence Record后才能按Observation投影，权威结论另需Owner Fact/Settlement |
| receipt/attestation/claim携generic OwnerFact要求升级authoritative | 拒绝；原样保持Ledger TrustClass；额外领域current证明必须走独立typed Delta |
| claim Timeline投影直接成为Run终态 | 拒绝；Claim Trust不等于Runtime Outcome/Settlement |
| S1/S2每次使用fresh now重封Owner projection | 拒绝；必须复读同一immutable sealed Checked/Expires/ProjectionDigest，fresh now只ValidateCurrent |
| `RequestedNotAfter < 0`或正数值延长Owner TTL | `invalid_argument`或Fail Closed；0无caller上限，正数只能截短 |
| Owner Reader接收RequestedNotAfter并据此重封Expires/ProjectionDigest | 拒绝；Owner只返回自然sealed projection，caller bound由Continuity在S1/S2后最终截短 |
| 空Tombstone ref被当作absence证明，或Tombstone存在仍返回payload-readable | 拒绝；必须复读Owner-defined absence watermark/current revision，历史metadata与payload readability分离 |
| Tombstone/Retention binding/source-policy mutation与subject-current index/absence watermark分步推进 | 拒绝；Runtime Evidence Owner同一线性化边界全有或全无 |
| 旧Evidence ProjectionRef尚未Expires便继续ValidateCurrent | 若subject-current index已推进则`precondition_failed`；旧ref仅保留historical Inspect |
| ContextReference无法物化仍声称可恢复 | Fail Closed或Residual |
| Restore换Instance绕过旧unknown Conflict Domain | 拒绝；Conflict Domain保持tenant稳定 |
| Continuity拿admitted/submitted Plan直接Stage/Activate | unsupported；Plan只支持shape Validate/Inspect |
| historical Consistency/Seal绕过Plan current触发RestoreAttempt，或Eligibility绕过Admission门禁 | 拒绝；只有exact submitted Plan可进入Runtime create-once Attempt/Reservation，Eligibility之后仍须独立Admission/Authorization/Permit/Begin |
| checkpoint phase Evidence V1/Settlement V5用于restore-stage | 拒绝；Restore Stage使用专用Evidence/Settlement公开合同，不扩checkpoint closed union |
| 使用`activation_attempt`承载Restore或扩V3/V4闭表 | 拒绝；typed restore shape不授运行资格 |
| legacy RestoreRequest/RestoreCheckpoint被包装成V2 | 拒绝；缺失治理字段不能由Adapter补默认值 |
| Context refresh未完成仍复用旧Frame进入新Instance | 拒绝；Fail Closed或Residual，不得Ready |
| Continuity写Review Verdict/Run Outcome/Binding Policy | 拒绝；越过Owner边界 |
| SQLite/RocksDB双写无Journal | 拒绝；无法证明恢复与引用一致性 |
