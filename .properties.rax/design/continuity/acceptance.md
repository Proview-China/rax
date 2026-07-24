# Continuity 设计验收合同

## 1. 当前结论

Runtime Checkpoint-first V2、C-02 Manifest/Seal、C-01及Restore最小公共参考纵切已落地；参考纵切包含Action Admission、Authorization、Stage、Evidence/Settlement、Context materialization与Activation，但不表示production Restore启用。trusted Assembler、跨Owner全量Participant、远程Provider与production root闭合前不得形成production Capability。

## 2. 设计完整性门禁

- [x] 最高业务输入已逐项覆盖Timeline、SQLite/RocksDB、Checkpoint/Snapshot、Fork/Rewind/Restore、Retention、SDK/CLI/API。
- [x] 明确Runtime、Continuity、Harness、Participant、Review、Sandbox、Context、Tool/MCP、Memory/Knowledge Owner边界。
- [x] 明确Evidence Ledger V2单主，Timeline不分配第二sequence。
- [x] 明确C-01 caller只能提交请求坐标/期望，不能提交可信sequence/digest/Trust/current；production Event字段必须从Runtime Evidence exact Reader派生。
- [x] 明确production Request字段闭集只有stable Attempt/idempotency、EvidenceSourceKey、可选expected RecordRef、authoritative-only OwnerFact ref、Policy、Scope与RequestedNotAfter；禁止caller semantic/causal/object/payload/time及可信bool/sequence/digest/Trust。
- [x] 明确Projection Attempt create-once/Inspect/CAS只保存Owner exact refs/密封S1/S2 projections、共同TTL、状态与结果ref；不保存caller Admission/current副本。
- [x] 明确Evidence S1/S2使用相同Reader binding，并闭合source/policy/current、readability、exact Tombstone或Owner-defined absence watermark及subject-current index；Tombstone/Retention binding/source-policy mutation与index/watermark在Runtime Evidence Owner同一线性化边界原子推进。仅`authoritative_fact`追加Owner Fact S1/S2。两者都复读同一immutable sealed projection，逐字段闭合Record/source/candidate/chain/payload/Owner/Authority/Policy/Scope/Binding/Checked/Expires/ProjectionDigest；fresh now只ValidateCurrent，current已前进时旧CAS Conflict且禁止ABA。
- [x] 明确Runtime六类Trust逐项路由：除`authoritative_fact`外五类原样输出且禁止generic OwnerFact升权；attestation/claim的额外current证明只能走领域独立typed Delta。
- [x] 明确`RequestedNotAfter`的0=caller不加上限、<0非法、>0只能缩短；Owner Reader不得接收该值或据此重封，Continuity只在S1/S2 exact与fresh-now ValidateCurrent后对自然共同TTL最终截短。Event时间、Cursor TTL、Retention window或默认值不得伪造current。
- [x] 明确Projection Attempt状态包含`reconcile_required`，该状态不可查询为visible/current且只Inspect原Attempt。
- [x] 明确Controller唯一序列为Create Attempt→S1 Record双读→R-CTY-06 current→必要Owner current→S2 exact→fresh ValidateCurrent→原子Event+Attempt visible+Continuity current index。
- [x] 明确Rebuild逐项走同一Controller；禁止caller Candidate/Event、`PutProjection`、`ReplaceLedgerScope`、bulk历史覆盖或绕Reader。
- [x] 明确Timeline Event create后revision/digest/bytes永久immutable；Tombstone是独立Fact+Visibility overlay CAS，禁止原地修改Event或通过Rebuild/删除overlay复活。
- [x] 明确C-01 closed errors仅为`invalid_argument|not_found|conflict|precondition_failed|unavailable|indeterminate|unsupported`，lost reply/unknown只Inspect原Attempt。
- [x] 明确SQLite权威元数据与RocksDB内容寻址Value边界及Write-Ahead Journal。
- [x] 明确Provider Receipt/Observation未经Evidence Admission不能直接成为Timeline；即使投影也只保持Observation等级，不能成为领域Fact或Run Outcome。
- [x] 明确Timeline Request先create Attempt，再经Owner Readers S1/S2与领域CAS；Checkpoint外部phase严格遵循Reservation→Admission→Review/Authorization→Permit→Begin→双Enforcement→Execute/Inspect→Observation/Evidence→DomainResultFact→Runtime Settlement V5→Domain ApplySettlement；Restore运行顺序后置，不由本轮预先冻结。
- [x] 明确全部Continuity Effect kind、tenant稳定Conflict Domain、Unknown恢复和Run Requirement候选。
- [x] 明确Runtime Checkpoint-first V2参考纵切已完成独立代码终审YES，包含原子Attempt+Barrier、EffectCut、Checkpoint phase Evidence/Settlement、Consistency/Finalization及ManifestSeal Reader；Checkpoint链`ProviderCalls=0`、`ProductionClaimEligible=false`。Restore最小参考纵切已有受治理Host-Local Stage，但仍不授production资格。
- [x] 明确Harness私有Port禁用、Slot/Phase仅声明Observer/Gate/Port贡献且不发明公共枚举。
- [x] 明确Model Invoker只通过RouteID、routegateway与公开execution union关联；不依赖internal/SDK/Raw事件。
- [x] 明确ContextReference不可物化时Fail Closed或Residual。
- [x] 明确Checkpoint Governance V2只冻结Owner公开exact refs；裸Runtime/Session/Generation字符串不得进入V2。
- [x] 明确Restore Plan/Intent/Attempt分层：Continuity拥有Plan Fact与只读current Adapter；Application拥有immutable Intent与协调；Runtime拥有create-once Attempt/Identity Reservation、short-TTL Eligibility、Enforcement/Settlement/Activation；Sandbox/Context只拥有自身Stage与Generation/Frame事实，Continuity不创建或解释这些事实。
- [x] 明确legacy `CheckpointSet/RestoreRequest/CheckpointParticipantPort/Foundation`不得经Adapter补默认治理字段或扩权。
- [x] 明确G6A既有Action Gateway/跨Owner验收先于G6B、G7和Checkpoint；Restore专用路由/pre-run Evidence/公共Phase后置，不在本轮补齐或重造。
- [x] 明确Go默认、无已证明Rust热点。
- [x] 明确Checkpoint与Restore分开验收：本轮只审Checkpoint第一波；Restore仅Plan/Ref shape，运行链必须另行联合Review。
- [x] 明确Checkpoint唯一接入顺序为G6A→G6B→Harness G7→Runtime原子Attempt+Barrier bundle→EffectCut→Participant Reserve/治理/Inspect/CAS→Manifest CAS→immutable Seal→Runtime Commit Consistency+CloseBarrier或Finalize+CloseBarrier；流程到此停止。
- [x] 明确Manifest递归校验所有层级exact ref的`TenantID + ScopeDigest`；`verified_candidate`与Seal要求顶层、Attempt、Participant和任意severity Diagnostic聚合Residual为空。
- [x] 明确Repository事务内重验current Manifest、exact revision、`verified_candidate`、Owner与全部Seal binding；current/history/seal结构化按Tenant/Scope/ID分区且Reader携受验Scope/Owner。
- [x] 明确64路不同内容CAS/Seal只有一个赢家；progressed lost-reply旧CAS只能Conflict并保留immutable history，禁止ABA。
- [x] 明确exact identity key包含完整`OwnerBinding`的可比较结构；delimiter collision、Owner任一字段漂移和跨Tenant同Seal ID均不得串键或串读。

## 3. Owner与可信度验收

| 场景 | 必须通过 | 必须拒绝 |
|---|---|---|
| Event入Timeline | Request闭集；Controller完整S1/S2/fresh；仅authoritative_fact追加Owner current；Event+Attempt visible+Continuity index原子可见 | caller Candidate/semantic/payload/可信字段、绕Attempt/Reader、三对象半提交、旧Project进入生产 |
| Model/Tool事件 | Action Candidate或Observation只保留Evidence关系 | 直接成为Tool Result/Verdict/Timeline/Outcome |
| Snapshot | Participant Owner Inspect Fact | Provider Receipt直接Committed |
| Checkpoint | Runtime原子Attempt+Barrier；exact Generation/Frame/Attempt/Settlement/Memory-Knowledge refs；Continuity immutable Seal；成功Consistency与Barrier关闭原子 | 独立AcquireBarrier、裸字符串、mutable Manifest入Consistency、非成功生成Consistency、Continuity宣布consistent |
| Restore | exact Plan current→Intent→Attempt/Reservation→Eligibility→Admission→Authorization→Permit/Begin→Enforcement→Stage→Evidence/Settlement→Context→Activation；lost reply只Inspect | historical Consistency/Seal冒充Eligibility；Authorization进入Eligibility；绕门直调Stage/Activate/Provider；外部世界回滚 |
| Rewind | Continuity Workspace-only Plan，Sandbox Workspace Owner执行new ChangeSet Effect | Continuity或Tool直接改文件；历史记录直接改现实世界 |
| Retention | Policy/Hold/引用闭包+受治理Purge | 无证据删除、改写原Event |
| Run Settlement | Continuity只提交自身Participant Fact | 写Runtime Outcome或自签Plan |

## 4. 合同反例矩阵

C-01反例索引为`CTY-TL-P0-01..03`与连续`CTY-TL-01..33`；不得遗漏28..33的Rebuild history/overlay与Tombstone immutable/readability反例。

| ID | 反例 | 必须结果 |
|---|---|---|
| CTY-LEDGER-01 | Continuity为Evidence-backed Event另分配sequence | 拒绝，保留Evidence Ledger sequence单主 |
| CTY-LEDGER-02 | 同source epoch/sequence换Payload digest | Conflict，原Record不变 |
| CTY-LEDGER-03 | Provider时间戳用于跨Source重排 | 拒绝；仅作时间证据 |
| CTY-TL-P0-01 | caller Candidate路径把`EvidenceAdmission`中的sequence/digest/Trust和布尔值直接写入Projection | 已降名`ReferenceTimeline`且production装配必须拒绝；production只使用coordinate-only Adapter |
| CTY-TL-P0-02 | 历史live `Rebuild`批量接受caller Candidate并直接`ReplaceLedgerScope` | bulk Replace已删除；当前仍须把caller Candidate逐项Put收拢为Request+Controller，反例继续防回归 |
| CTY-TL-P0-03 | 历史live `TombstoneProjection`原地修改Event的`Visibility/TombstoneRef` | 已改immutable revision-1 Fact+overlay且historical Event零改写；反例继续防回归 |
| CTY-TL-01 | caller请求携`AdmittedByLedger/InspectedByOwner/current=true`并要求直接可见 | 拒绝；caller只可提交坐标/期望，全部可信值从Owner Readers派生 |
| CTY-TL-02 | Evidence S1/S2漏比Record/source/candidate/chain/payload digest或Trust任一字段 | `conflict/precondition_failed`；零Event、零current |
| CTY-TL-03 | authoritative投影Owner S1/S2漏比Fact revision/digest、Owner Binding、Authority/Policy/Scope/Binding或not-after | Fail Closed；Attempt不可visible |
| CTY-TL-04 | `RequestedNotAfter < 0`、正数延长Owner TTL，或Cursor TTL/RecordedAt/Retention window被用于补TTL | `<0` invalid；0不加caller上限；正数只截短；共同not-after取全部真实上限最小值 |
| CTY-TL-05 | Projection create/CAS回包丢失后换Attempt ID重试 | 拒绝；只Inspect原Attempt exact ref |
| CTY-TL-06 | current已经前进后重放旧expected revision，因next内容曾出现而返回幂等成功 | Conflict；历史不变、禁止ABA |
| CTY-TL-07 | Evidence/Owner S1与S2使用不同Reader binding | Conflict；不得admitted/visible |
| CTY-TL-08 | typed-nil、timeout或Reader无覆盖被映射成NotFound | 拒绝；分类为`unavailable/indeterminate`，只Inspect原Attempt |
| CTY-TL-09 | Observation Event/Projection Ref被解释为Model/Tool领域Fact或production current | 拒绝；保持原TrustClass，领域identity/current仍由对应Owner拥有 |
| CTY-TL-10 | `observation|late_observation|receipt|attestation|claim`经generic OwnerFact Reader升级为`authoritative_fact`或authoritative current | 拒绝；五类逐项保持原Ledger TrustClass |
| CTY-TL-11 | 领域要证明attestation/claim current，直接修改Runtime Trust或复用generic OwnerFact | 拒绝；必须独立typed Delta/Fact projection，原Trust不变 |
| CTY-TL-12 | Owner S1/S2每次按fresh now重封，导致Checked/Expires/ProjectionDigest变化 | 拒绝；复读同一immutable sealed projection，fresh now只ValidateCurrent |
| CTY-TL-13 | Attempt处于`reconcile_required`仍被Query/Watch当visible/current | 拒绝；只Inspect原Attempt/Event并收敛到admitted/visible/indeterminate |
| CTY-TL-14 | 空Tombstone ref被当成“未删除”，或S1 absence watermark在S2漂移 | 拒绝；必须有Owner-defined current absence proof且S1/S2 exact一致 |
| CTY-TL-15 | Tombstone已存在仍返回payload-readable/current | 拒绝payload；只允许保留受Policy约束的历史审计metadata |
| CTY-TL-16 | Runtime Evidence或领域Owner Reader接收RequestedNotAfter，导致S1/S2的Expires/ProjectionDigest随caller变化 | 拒绝；Owner自然sealed projection必须稳定，caller bound只影响Continuity aggregate最终NotAfter |
| CTY-TL-17 | Tombstone/Retention binding/source-policy mutation成功，但subject-current projection index或absence watermark仍指旧值 | 原子合同破坏，Fail Closed；mutation/index/watermark必须全有或全无 |
| CTY-TL-18 | 旧ProjectionRef尚未Expires且历史Inspect成功，因此ValidateCurrent也成功 | 拒绝；current index已推进时必须`precondition_failed`，历史可读不等于current |
| CTY-TL-19 | subject-current index回到旧ProjectionRef/revision/digest | Conflict；revision单调、禁止ABA |
| CTY-TL-20 | Request携semantic/custom class、Parent/Causation/Correlation/Object refs、payload或Observed/Recorded time | `invalid_argument`；这些字段只能由Reader结果派生 |
| CTY-TL-21 | caller expected RecordRef被直接当作可信Record，不做`InspectBySource -> InspectRecord`双读 | 拒绝；expected Ref只作比较，S1双读必须完整相等 |
| CTY-TL-22 | `Project`未create Attempt便读取/写入，或Attempt跳过`inspecting/admitted/reconcile_required` CAS | 拒绝；Repository写口不能绕Controller状态机 |
| CTY-TL-23 | S1只读Record不读R-CTY-06，或authoritative_fact不读Owner current | Fail Closed；零Event、零current index |
| CTY-TL-24 | S2漏重复任一Reader、使用不同binding，或fresh now在S2前/复用旧时间 | Conflict/precondition_failed；不得publish |
| CTY-TL-25 | Event成功但Attempt未visible，或Attempt visible但Continuity current index/Event缺失 | `indeterminate/reconcile_required`；三对象必须同事务全有或全无 |
| CTY-TL-26 | Rebuild把caller Candidate转换成Event后调用`PutProjection/ReplaceLedgerScope` | 拒绝；每项只接Request闭集并完整调用Controller |
| CTY-TL-27 | Rebuild某Item unknown后换Attempt ID或用整批重跑覆盖history | 拒绝；只Inspect原Item Attempt，其他Item保持独立exact结果 |
| CTY-TL-28 | Rebuild清空Scope、覆盖旧Event/Tombstone overlay或让晚到旧Record回退current index | Conflict；历史与overlay不可替换，index单调 |
| CTY-TL-29 | Tombstone操作修改既有Event bytes/revision/digest或在Event内写Visibility/TombstoneRef | 拒绝；create独立immutable Tombstone Fact并CAS overlay |
| CTY-TL-30 | Tombstone create成功但Visibility/current index未推进，或反向半提交 | `indeterminate/reconcile_required`；Tombstone+index同事务全有或全无 |
| CTY-TL-31 | 删除Tombstone overlay、重建Scope或重放旧Projection使Event重新visible | 拒绝；禁止静默复活，任何retraction须未来独立治理Fact |
| CTY-TL-32 | Tombstone前后exact Inspect同一Event得到不同digest/bytes | 数据损坏/Conflict；历史Event必须bit-stable |
| CTY-TL-33 | Tombstone后Query/Watch仍返回payload-readable，因为Event历史仍有PayloadRef | 拒绝payload；可见性/readability必须合并current overlay与Policy |
| CTY-TL-34 | Turn/Step/Action/Artifact/Effect/Review Case/Checkpoint过滤字段未进入Query digest | 拒绝；所有类型化维度都必须封入canonical digest，Cursor语义不可漂移 |
| CTY-TL-35 | Cursor签发后替换任一类型化ObjectRef仍继续分页/Watch | `cursor_invalidated`；不得沿用旧Cursor读取另一查询集合 |
| CTY-TL-36 | 多个类型化维度使用OR或忽略其中一个条件 | 拒绝；所有非空维度按exact ObjectRef AND匹配，非法Ref在Store读取前失败 |
| CTY-MODEL-01 | Provider completed事件直接成为Timeline终态 | 拒绝；等待领域Owner Fact/Settlement |
| CTY-MODEL-02 | Tool Call被记录为Tool Result | 拒绝；只允许Action Candidate |
| CTY-STORAGE-01 | SQLite current ref存在但RocksDB Chunk缺失 | `cross_store_indeterminate`，Fail Closed |
| CTY-STORAGE-02 | RocksDB Object存在但Journal未提交ref | orphan；不可见，可在引用证明后回收 |
| CTY-STORAGE-03 | Compaction崩溃后旧新块均存在 | Inspect Journal，选择唯一current ref，不盲删 |
| CTY-STORAGE-04 | 敏感Snapshot无encryption envelope | Rejected conformance |
| CTY-STORAGE-05 | caller提交healthy/Chunk状态，或有限Subject healthy被解释成全库无孤儿 | 拒绝；状态必须由Owner双轮读取派生，结论只覆盖请求坐标 |
| CTY-STORAGE-06 | key存在但Chunk bytes长度或digest错误 | `corrupt_content`；不得返回正文或形成healthy |
| CTY-STORAGE-07 | visible Object缺Chunk、Journal/Object digest错绑或expected Manifest漂移 | `dangling_reference/indeterminate`；Fail Closed |
| CTY-STORAGE-08 | S1/S2期间Manifest、visibility、Journal revision/state或Chunk内容变化 | `indeterminate`；不得封存healthy Fact |
| CTY-STORAGE-09 | Audit create成功但回包丢失后换Audit ID重扫 | 拒绝；只Inspect原Tenant/Scope/Audit ID，same request返回原Fact |
| CTY-STORAGE-10 | Audit直接推进Journal/Retention/Tombstone、删除Chunk或调用remote Provider | 拒绝；诊断Port零Cleanup/Effect能力 |
| CTY-STORAGE-11 | 同Audit ID/idempotency换Subject、expected digest或Scope | Conflict；原revision-1 Fact/history不变 |
| CTY-DELTA-01 | caller提交Chunk列表、reuse bool、bytes统计或预制Fact | 拒绝；只接受Base/Target exact坐标，关系由Owner读取派生 |
| CTY-DELTA-02 | Base/Target任一不可见、缺Chunk、坏Chunk或完整Object digest不匹配 | Fail Closed；零Delta Fact |
| CTY-DELTA-03 | Chunk digest相同但schema或length不同仍标reuse | 拒绝；Chunk exact identity必须三字段全等 |
| CTY-DELTA-04 | S1/S2期间任一Manifest、visibility或Chunk bytes漂移 | `indeterminate`；零Delta Fact |
| CTY-DELTA-05 | 同Delta ID/idempotency换Base/Target/expected digest | Conflict；原revision-1 Fact不变 |
| CTY-DELTA-06 | create成功丢回包后换Delta ID重算 | 拒绝；只Inspect原Tenant/Scope/Delta ID |
| CTY-DELTA-07 | 将Delta Fact解释为可执行patch、Compaction完成或base删除资格 | 拒绝；只保存结构共享关系 |
| CTY-DELTA-08 | 跨Tenant/Scope Object拼接为Delta | invalid/conflict；不写入 |
| CTY-DERIVE-01 | caller提交Event payload、summary正确性、authoritative/current或删除计划 | 拒绝；Request只携exact坐标与candidate kind |
| CTY-DERIVE-02 | source Event跨Scope、重复、digest/projection drift或历史bytes被改写 | Fail Closed；零Candidate Fact |
| CTY-DERIVE-03 | output Object不可见、Manifest漂移、缺Chunk或坏Chunk | Fail Closed；零Candidate Fact |
| CTY-DERIVE-04 | S1/S2期间任一Event或output变化 | `indeterminate/conflict`；零Candidate Fact |
| CTY-DERIVE-05 | 同Candidate ID/idempotency换source/order/kind/output | Conflict；原revision-1 Fact不变 |
| CTY-DERIVE-06 | create成功丢回包后换ID重算 | 拒绝；只Inspect原Candidate ID |
| CTY-DERIVE-07 | Candidate被发布为Timeline current、领域Fact、Memory/Knowledge或Run终态 | 拒绝；Authority固定candidate-only |
| CTY-DERIVE-08 | Candidate触发bulk replace、Event mutation、Compaction或Purge | 拒绝；Port无这些方法 |
| CTY-ART-01 | caller直接携带storage/parent/source digest并创建Artifact Relation Fact | 拒绝；Request只能携带稳定坐标，权威描述必须由typed Owner Reader派生 |
| CTY-ART-02 | S1/S2期间Artifact、Related Ref、Evidence、Storage或Parent任一字段漂移 | Conflict/indeterminate；零Relation Fact、零索引 |
| CTY-ART-03 | 同Relation ID或Idempotency Key换内容 | Conflict；原revision 1 Fact不变，不形成ABA |
| CTY-ART-04 | create成功但回包丢失后换Relation ID重建 | 拒绝；只Inspect原exact ref |
| CTY-ART-05 | Continuity将Artifact Relation解释为Artifact current、Review Verdict、Tool Result或Effect Outcome | 拒绝；关系Fact只保存Owner exact refs与因果kind |
| CTY-ART-06 | 跨Tenant关系、Parent换Owner/Artifact ID/schema或parent revision不低于当前revision | invalid/conflict；不写入 |
| CTY-CKPT-01 | Required Harness unsupported | Partial/Rejected；不可自动Restore |
| CTY-CKPT-02 | Effect begun未settled且未入Effect Cut | 不得consistent |
| CTY-CKPT-03 | Provider Snapshot回包成功但Owner Inspect失败 | Unknown/Partial |
| CTY-CKPT-04 | `RuntimeStateRef`、`RunSessionRef`或`ContextGeneration`裸字符串进入V2 | 拒绝；要求Owner公开ID/revision/digest exact ref |
| CTY-CKPT-05 | Context Generation与Frame不属于同一Owner闭包 | Context Inspect失败；Diagnostic Indeterminate |
| CTY-CKPT-06 | 已Begin Attempt无exact Settlement仍标verified | 拒绝；unknown/inspection/residual必须入Manifest |
| CTY-CKPT-07 | Manifest内联Memory/Knowledge正文或Runtime Outcome | 拒绝；只允许opaque exact refs |
| CTY-CKPT-11 | Runtime先写Attempt再独立AcquireBarrier | 拒绝；Attempt+Barrier必须同一Owner事务全有或全无 |
| CTY-CKPT-12 | Attempt-only或Barrier-only半对象可被Inspect为有效 | 原子合同破坏，Fail Closed；不得继续Freeze/Participant |
| CTY-CKPT-13 | `partial|indeterminate|rejected`写入CheckpointConsistencyFact | 拒绝；非成功只Finalize Attempt+CloseBarrier且不生成Consistency |
| CTY-SEAL-01 / CKP2-C04 | ManifestFact处于`verified_candidate`但Seal缺失，或mutable Manifest Fact/candidate直接进入Runtime Consistency | 拒绝；必须绑定immutable revision 1 ManifestSeal；状态枚举不接受兼容别名 |
| CTY-SEAL-02 | 同Seal ID换Manifest revision/digest、Attempt、Barrier、EffectCut或Participant closure | Conflict；原Seal不变 |
| CTY-SEAL-03 | Seal create回包丢失后创建新ID | 拒绝；Inspect原Seal ID/ref |
| CTY-SEAL-04 | CAS覆盖、续写或删除重建历史Seal | 拒绝；Seal revision 1且永久immutable |
| CTY-SEAL-05 | Runtime只给Seal ID或窄ref，缺Continuity完整Owner/Scope exact lookup | invalid/fail closed；不得扫描Manifest或由Application补默认Owner |
| CTY-SEAL-06 | Runtime Participant closure与Seal内`RuntimeClosureRef`缺失、重复、Participant ID/Owner任一字段或digest漂移 | Conflict；不得提交Consistency |
| CTY-SEAL-07 | Context Generation/Frame或Memory/Knowledge/Snapshot/Coverage换包但Context/Artifact Closure digest沿用旧值 | canonical/binding Conflict；不得提交Consistency |
| CTY-SEAL-08 | Runtime Gateway的Seal/Participant closure S1与S2不同，或exact Inspect回包丢失后换Seal ID | Conflict/indeterminate；只复读同一exact Seal与原Participant closure坐标 |
| CTY-SEAL-09 | raw external digest使用大写、非SHA-256、临时拼接算法或丢失Owner原始spelling | invalid digest；只允许公开canonical normalizer且exact lookup保留raw digest |
| CTY-RESTORE-01 | RestorePlan/historical Consistency直接触发RestoreAttempt或Provider | unsupported；零Fact、零Provider |
| CTY-RESTORE-02 | checkpoint phase Evidence V1/Settlement V5用于restore-stage | 拒绝；Restore current applicability后置 |
| CTY-RESTORE-03 | legacy RestoreRequest/RestoreCheckpoint包装成V2 | 拒绝；shape不足且不得补默认治理字段 |
| CTY-RESTORE-04 | 用`activation_attempt`或transport kind授Restore资格 | 拒绝；shape coordinate不授current资格 |
| CTY-RESTORE-05 | Restore/Rewind宣称撤销邮件、交易或远程动作 | 拒绝；外部世界不回滚 |
| CTY-REWIND-01 | Rewind试图取消已发送邮件 | 保留不可逆Effect，拒绝虚假回滚 |
| CTY-REWIND-02 | 选择保留两个文件但依赖不闭合 | Plan conflicts，不能提交执行 |
| CTY-REWIND-03 | caller携文件payload、accepted Verdict、Permit/Fence或可信current创建Plan | invalid/forbidden；只接受coordinate并由Owner复读 |
| CTY-REWIND-04 | Rewind Plan直接写文件或创建Sandbox ChangeSet/Settlement | Owner mismatch；Continuity只保存Plan和exact refs |
| CTY-REWIND-05 | Begin后丢回包便换Plan/ChangeSet/Operation ID重派 | 拒绝；只Inspect原Sandbox Workspace Commit Attempt |
| CTY-REWIND-06 | 首版尝试Tool、邮件、交易、远程DB或remote blob补偿 | unsupported；Provider=0且保留历史/Residual/候选 |
| CTY-REWIND-07 | source Workspace revision、ChangeSet digest、依赖closure或Review current在S1/S2间漂移 | conflict/precondition failed；零文件Effect |
| CTY-REWIND-08 | Rewind覆盖旧Workspace/Event/Checkpoint历史或宣称外部世界已回滚 | 拒绝；新ChangeSet/new Effect，历史immutable |
| CTY-UNKNOWN-01 | Begin后timeout后换Attempt重派 | 拒绝，只Inspect原Attempt |
| CTY-RETENTION-01 | Legal Hold对象到期 | 禁止Purge |
| CTY-CURSOR-01 | Authority缩小后继续使用旧Cursor | Cursor invalidated |
| CTY-SLOT-01 | Continuity私建checkpoint phase enum | 拒绝，等待Harness公共对象 |
| CTY-OWNER-01 | Continuity写Review Verdict/Run Outcome/Binding | 拒绝，Owner mismatch |
| CTY-CKPT-08 | 原子Attempt+Barrier bundle或Manifest/Seal持久成功但回包丢失后创建新ID | 拒绝；Inspect原bundle/Manifest/Seal exact ref |
| CTY-CKPT-09 | Partial Manifest被标记为restore eligible | 拒绝；Partial只保留诊断价值 |
| CTY-CKPT-10 | 通过CAS给immutable CheckpointConsistencyFact补TTL/currentness | 拒绝；该Fact只读历史一致性，回包丢失只Inspect exact ref |
| CTY-RESTORE-11 | immutable Consistency或ManifestSeal被直接当作current Restore资格 | 拒绝；必须经exact submitted Plan、Runtime create-once Attempt/Reservation与Eligibility current复读 |
| CTY-RESTORE-12 | 仅凭shape字段调用Issue/Bind、Admission、Begin、Stage或Activate | unsupported/fail closed；Provider=0 |
| CTY-RESTORE-13 | Continuity私建`RestoreGovernancePortV2`或第二Gateway | 拒绝；只能调用Runtime公开最小Port；Application Action route与Stage仍等待各Owner联合Review |
| CTY-FORK-01 | Fork新Lineage后释放旧unknown Conflict Domain或继承全部Authority | 拒绝；旧unknown继续占域，并重新验证全部current治理 |
| CTY-API-01 | CLI/API直接调用Continuity/Application Fact Store或`application.FacadeV2` | 拒绝；只能调用Application公开Submission+Inspect Gateway |
| CTY-API-02 | caller提交raw Submission Bundle、Provider binding、Permit/Fence或accepted Verdict | `invalid_argument/forbidden`；这些值只能由trusted Application/Assembler/Owner派生 |
| CTY-API-03 | Submit回包丢失后换Request/Command ID重交 | 拒绝；按原Scope+identity Inspect Submission/Command/Outbox/Journal |
| CTY-API-04 | 写命令已注册但CompiledGraph/Binding/Capability/current缺失 | `unsupported/precondition_failed`；零Provider调用、零领域Fact猜测 |
| CTY-API-05 | Restore或Purge命令存在即被解释为production执行能力 | 拒绝；接口可见性不授trusted Assembler、remote Provider或production root资格 |
| CTY-API-06 | API超时后客户端以同义新payload重派 | 拒绝；Idempotency identity与canonical payload drift为Conflict，只Inspect原长任务 |
| CTY-API-07 | trusted Assembler返回换Request digest、Scope、Idempotency、canonical payload、root kind或root step | Gateway在Facade/backend前拒绝 |
| CTY-API-08 | Submit/Submission回包丢失，或64路相同Request并发 | 只Inspect原Request/Command；exact重放同结果，同ID换内容Conflict |
| CTY-API-09 | CLI输入未知字段、command-kind不匹配或直接携`permit` | strict decode/闭kind校验拒绝，零Gateway调用且零输出 |
| CTY-API-10 | Timeline Reader返回无效Event、过滤不命中、sequence乱序、超PageLimit或Cursor query/watermark漂移 | SDK在返回调用方前Fail Closed；不泄漏Reader返回的漂移页 |
| CTY-API-11 | 调用方提交伪造、过期或不匹配的Timeline输入Cursor | 在Reader调用前拒绝；Reader调用次数为零 |
| CTY-API-12 | `timeline show/watch`或`checkpoint inspect`缺失Reader，或只读JSON含未知可信字段 | `unsupported/invalid_argument`且零输出；不得回退到Fact Store或治理写Gateway |

### 4.1 两阶段场景验收

| 场景 | 第一波 Checkpoint | Restore最小公共参考执行 |
|---|---|---|
| Happy | G6A→G6B→G7→atomic Attempt+Barrier→EffectCut→Participant Reserve/治理/Inspect/CAS→Manifest CAS→Seal→Runtime Consistency+CloseBarrier | Plan current→Intent→Attempt/Reservation→Eligibility→Admission→Authorization→Permit/Begin→Enforcement→Stage→Evidence/Settlement→Context→Activation |
| Crash | 只Inspect原Attempt+Barrier bundle、Participant、Manifest、Seal；成功/非成功终结分别原子 | 在当前Owner门停止；只Inspect原Plan/Intent/Attempt/Enforcement/Participant/Evidence/Settlement/Context/Activation，不跨门跳转 |
| Lost reply | 不独立AcquireBarrier、不重Prepare/Commit、不创建第二Checkpoint/Seal ID | 只Inspect原stable identity；不创建第二Target Instance/Lease，不重派Effect |
| Stale | Scope/Binding/Generation/Frame/Settlement/Seal换包则Finalize incomplete/indeterminate，不生成Consistency | Plan、Consistency、Seal、prerequisite或Eligibility任一过期/漂移即拒绝current；不得用重发绕过 |
| Partial/Unknown | Manifest只诊断；Runtime Finalize+CloseBarrier且无Consistency | required Stage/Context出现Residual或Unknown时不得Activate/Ready；外部世界不回滚 |
| Fork/Rewind | 仅冻结历史和依赖引用 | 后续新Lineage/Instance语义仍需独立Review；不得宣称外部世界回滚 |

## 5. 设计与实现一致性验收

未来实现评审必须逐项证明：

1. 生产包导入边界符合`domain -> own contract/ports`、`runtimeadapter -> runtime/core+ports`；
2. 未导入其他组件实现、Runtime foundation/kernel/fakes或Harness internal/private ports；
3. 所有public object都有canonical digest、bounds、version、Validate与clone/immutability策略；
4. 所有状态迁移集中验证，非法状态在读取backend前失败；
5. SQLite/RocksDB每个崩溃点都有Inspect/收敛测试；
6. 远程/破坏性Effect走对应Runtime公开治理合同并在执行点二次验真；Checkpoint phase使用V2/V1/V5且不扩legacy闭表；Restore最小公共链按Intent→Eligibility→Admission→Authorization→Permit/Begin→双重Enforcement→Stage→Evidence/Settlement→Context→Activation执行；
7. Run Requirement由Trusted Assembler认证，组件不自签；
8. Fakes只用于测试，文档不宣称生产Backend/SLA；
9. 全部单元、白盒、黑盒、故障注入、Conformance、race、vet通过；
10. 联合集成和系统测试必须由相应Owner批准并记录真实命令/结果。
11. Checkpoint V2必须覆盖Attempt+Barrier原子半写、Manifest/Seal CAS、Seal lost reply/changed conflict/history overwrite、Commit-vs-Finalize并发与Participant/Settlement exact Inspect反例。
12. CLI/API V1包含受治理写面，但所有写请求必须通过Application公开Submission+Inspect Gateway；任何直接Fact Store、Application root实现、raw Bundle或Provider路径均为NO-GO。
13. Restore Plan current Adapter与`RestoreGovernancePortV2`只形成Reservation/Eligibility，不自行授Action Admission、Review Authorization、Permit、Stage或Activation；这些门由各Owner专用公共链独立形成。任何对象都不授外部世界回滚，也不得以第二Gateway、legacy、`activation_attempt`或transport kind绕过。

## 6. 评审输入与未决门禁

联合评审必须包含：

- Runtime Owner：R-CTY-01～05；
- Runtime Evidence Owner：`R-CTY-06` exact current/readability/tombstone/subject-current Reader（`P1-1`）；
- Application/Assembler与各领域Fact Owner：仅`authoritative_fact`使用的typed Owner Fact current Reader routing（`P1-2`）；
- Harness Owner：H-CTY-01和公共Slot/Phase对象；
- Application/Assembler Owner：A-CTY-01、CompiledGraph、Binding/Requirement映射；
- Model Invoker Owner：公开Route/execution union引用边界；
- Sandbox/Context/Tool/Review/Memory Owner：Snapshot/ChangeSet/Context物化/Effect/Verdict/正式Fact引用；
- 管理线：Participant policy、Retention、远程存储/密钥与容量目标；首版API包含受治理写面已经用户裁决。

任一Owner未确认不表示整个设计不可读，但对应Capability必须在实现计划中保持blocked/unsupported，不能由Continuity自行补齐。
