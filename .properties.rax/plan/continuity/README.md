# Continuity 落地计划（Wave 1 + Checkpoint/Restore Governance V2 Delta）

> Continuity Owner Manifest/Seal V2、Checkpoint-first跨Owner reference纵切及Restore最小公共参考纵切已实现并通过全量ordinary/race/vet；不解锁生产Checkpoint/Restore root、跨Owner全量Participant、远程Provider或production Backend/SLA。

> C-01、Artifact Relation、C-06 Content Integrity Audit、C-07 Content Delta、C-08 History Derivation Candidate Continuity Owner切面及owner-local持久化已实现：C-08从ordered exact Event集合与已可见output Object双轮读取派生immutable candidate-only Fact并落SQLite（自schema v8引入，当前schema v9）；它不证明语义正确、不发布current、不执行Compaction/Purge。跨Owner typed Owner Reader真实装配与Application root未完成，仍不得宣称端到端production GO、production Attach或Purge。

## 1. 状态与使用方式

- 状态：Wave 1、C-01、Artifact Relation、C-06/C-07/C-08、Continuity Manifest/Seal、RestorePlan V2治理/SQLite持久化、Continuity Runtime current Adapter、Checkpoint-first及Restore最小公共参考纵切已落地；production trusted Assembler、跨Owner全量Participant、远程Provider/root仍未完成。
- 设计依据：`.properties.rax/design/continuity/**`。
- 最高业务依据：`tmp.document/Continuity.md`。
- 计划实现根：`ExecutionRuntime/continuity/**`。
- 当前波次已授权为闭合Restore最小公共纵切所必需的Runtime/Application/Sandbox/Context additive Delta；仍不授权改Harness、Model Invoker、Go workspace、CI、根配置或全局索引，也不允许跨Owner复制事实语义。后续跨Owner缺口继续通过公共Port与联合Conformance闭合。
- Port Delta必须由对应Owner先合入；不得在Continuity内做“暂时兼容”实现。
- 实现完成并通过验收后，本计划保留并标记为陈旧计划，不删除。

### 1.1 设计输入索引

- [设计总览](../../design/continuity/README.md)
- [Checkpoint/Restore合同与状态机](../../design/continuity/contracts-and-state-machines.md)
- [Governance V2 Port Delta](../../design/continuity/port-delta.md)
- [Runtime/Harness/Application映射](../../design/continuity/integration-mapping.md)
- [设计验收与NO-GO矩阵](../../design/continuity/acceptance.md)
- [架构图](../../design/continuity/architecture.drawio)
- [Wave 1模块说明](../../module/continuity/README.md)
- [Wave 1 live-state记忆](../../memory/continuity/20260716-112436-ContinuityWave1LiveState资产收口.md)
- [Checkpoint Manifest Governance V2 Continuity Owner实现记忆](../../memory/continuity/20260716-140547-CheckpointManifestGovernanceV2ContinuityOwner实现.md)
- [Checkpoint Manifest Governance V2独立审计返修记忆](../../memory/continuity/20260716-144250-CheckpointManifestGovernanceV2独立审计返修完成.md)
- [ExactFactIdentity结构键复审返修记忆](../../memory/continuity/20260716-145936-ExactFactIdentity结构键复审返修完成.md)
- [Manifest/Seal V2最终独立代码复审YES记忆](../../memory/continuity/20260716-151911-ManifestSealV2最终独立代码复审YES.md)
- [C-01 Timeline Projection Governance设计冻结记忆](../../memory/continuity/20260716-211100-C01TimelineProjectionGovernance设计冻结.md)
- [C-01三条live P0最小修复冻结记忆](../../memory/continuity/20260716-213500-C01三条liveP0最小修复冻结.md)
- [C-01 Continuity Owner治理切面实现记忆](../../memory/continuity/20260717-143000-C01ContinuityOwner治理切面实现.md)
- [Timeline Projection Policy Current V1实现记忆](../../memory/continuity/20260717-144500-TimelineProjectionPolicyCurrentV1实现.md)
- [SQLite+RocksDB生产存储闭环记忆](../../memory/continuity/20260717-152200-SQLiteRocksDB生产存储闭环.md)
- [RestorePlan V2与只读SDK记忆](../../memory/continuity/20260717-155000-RestorePlanV2与只读SDK.md)
- [Workspace-only RewindPlan V2 owner-local闭环](../../memory/continuity/20260718-173300-WorkspaceOnlyRewindPlanV2OwnerLocal闭环.md)
- [RestoreAttempt/Eligibility最小Runtime Delta记忆](../../memory/continuity/20260718-120000-RestoreAttemptEligibility最小RuntimeDelta.md)
- [Restore最小公共纵切与Enforcement恢复](../../memory/continuity/20260718-165700-Restore最小公共纵切与Enforcement恢复.md)
- [Artifact因果关系owner-local纵切记忆](../../memory/continuity/20260717-162500-Artifact因果关系owner-local纵切.md)
- [Content Integrity Audit V1 owner-local纵切记忆](../../memory/continuity/20260717-165245-ContentIntegrityAuditV1-owner-local纵切.md)
- [Content Delta V1 owner-local纵切记忆](../../memory/continuity/20260717-171218-ContentDeltaV1-owner-local纵切.md)
- [History Derivation Candidate V1 owner-local纵切记忆](../../memory/continuity/20260717-172900-HistoryDerivationCandidateV1-owner-local纵切.md)
- [Continuity文档逐项完成度审计](../../memory/continuity/20260717-173850-Continuity文档逐项完成度审计.md)
- [Checkpoint Manifest Seal Runtime exact Adapter闭环](../../memory/continuity/20260717-232700-CheckpointManifestSealRuntimeExactAdapter闭环.md)
- [Checkpoint-first跨Owner Reference纵切](../../memory/continuity/20260718-010308-CheckpointFirst跨OwnerReference纵切.md)
- [Timeline全维度查询闭环](../../memory/continuity/20260718-011554-Timeline全维度查询闭环.md)
- [CLI/API治理写面范围冻结](../../memory/continuity/20260718-012446-CLIAPI治理写面范围冻结.md)
- [A-CTY-01公开Gateway与治理写面Reference闭环](../../memory/continuity/20260718-093800-ACTY01公开Gateway与治理写面Reference闭环.md)
- [SDK Timeline Page与CLI读取闭环](../../memory/continuity/20260718-095800-SDKTimelinePage与CLI读取闭环.md)
- [Wave1 Conformance exact能力闭集](../../memory/continuity/20260718-100638-Wave1ConformanceExact能力闭集.md)
- [ComponentReleaseV1 Delta](../../design/continuity/component-release-v1.md)：已实现reference-only owner publisher/readiness；不解锁production capture、remote Provider、production Restore root、cleanup或deployment root。

live代码和测试已经落地C-01、Artifact Relation owner-local、Continuity Manifest Governance/Seal V2、Runtime exact Seal只读Adapter、Checkpoint-first跨Owner reference纵切及RestorePlan V2 shape。Continuity Seal本身仍不等于Runtime Consistency或Restore current资格；reference纵切只由Runtime独立双读并提交Consistency。

### 1.2 Wave 1当前落地状态

| 能力 | live状态 | 本计划裁决 |
|---|---|---|
| Timeline/Event Projection | Wave 1查询面与C-01 Request/Attempt/CAS、Runtime Reader S1/S2、六类Trust、fresh TTL、原子Event/current、lost-reply Inspect、逐Request Rebuild、immutable Tombstone overlay及SQLite持久化均已实现 | Continuity Owner切面与持久化完成；真实typed Owner Readers和Application装配未完成，端到端production仍NO-GO |
| Content/Journal/Retention | 已实现Backend-neutral SPI、内容寻址Chunk/Object、CAS Journal恢复、Retention/Tombstone/Legal Hold、SQLite元数据与RocksDB Chunk production backend | remote blob、physical purge、archive、跨Ownerroot与生产SLA仍NO-GO |
| Artifact Relation | 已实现coordinate-only Request、Timeline+typed Artifact Owner S1/S2、immutable revision-1 Fact、Artifact/Related indexes、lost-reply Inspect、内存repository及SQLite（自schema v5引入，当前schema v9）与只读SDK | 真实typed Artifact Owner Router、Application root和production `AttachArtifact`仍NO-GO；不拥有Artifact current/正文 |
| Content Integrity Audit | 已实现bounded Object/Journal coordinate Request、Manifest/Journal/Chunk双轮检查、immutable revision-1 diagnostic Fact、lost-reply fake、64路单赢家、内存repository及SQLite（自schema v6引入，当前schema v9）与只读SDK | keyspace枚举、全库无孤儿证明、跨Owner引用闭包、自动回收/physical purge/remote Provider仍NO-GO |
| Content Delta | 已实现Base/Target exact Object双轮完整读取、ordered target recipe、reused/added/removed集合、immutable Fact、lost-reply fake、64路单赢家、内存/SQLite（自schema v7引入，当前schema v9）与只读SDK | Target创建、binary patch、Compactor/Purge/remote Provider不在本切片 |
| History Derivation Candidate | 已实现ordered exact Event集合+已可见output Object S1/S2、immutable candidate-only Fact、lost-reply fake、64路单赢家、内存/SQLite（自schema v8引入，当前schema v9）与只读SDK | 不证明summary/index正确，不发布current、不改写Event；真正Compactor/Indexer/Consolidator执行仍NO-GO |
| Checkpoint/Plan | 已实现`SnapshotBinding`、`CheckpointManifest`、Fork/legacy Rewind/Restore Plan纯验证、RestorePlan V2及Workspace-only RewindPlan V2 exact shape/create-once/CAS/history/current/TTL/lost-reply；Restore Plan Adapter可请求Runtime Reservation/Eligibility；Sandbox Owner已实现exact View+keep/drop→new staged ChangeSet及immutable Composition Fact | Continuity不创建Runtime/Sandbox Fact、不执行Restore/Rewind；Rewind Apply仍缺Application/Assembler把Composition exact ref接入既有`workspace-commit`治理链 |
| Checkpoint Manifest Governance V2 | 已实现recursive Tenant/Scope exact-ref、聚合Residual、Manifest create-once/CAS、immutable revision 1 Seal、Runtime完整Owner/Scope exact lookup、typed Participant closure唯一映射、Context/Artifact canonical digest及只读Adapter | Runtime Gateway可S1/S2复读Seal；Continuity不协调Barrier、不捕获Snapshot、不调用Participant/Provider，production root仍NO-GO |
| Runtime Checkpoint-first V2 | Runtime Owner参考纵切第三次独立代码终审YES；Attempt+Barrier/EffectCut/Evidence V1/Settlement V5/Consistency/Finalization已落地 | 不等于production Checkpoint；Checkpoint仍`ProviderCalls=0`、`ProductionClaimEligible=false` |
| Settlement | 已实现`DomainResultFact -> opaque Runtime Settlement ref -> ApplySettlement`绑定校验 | 不复制或解释Runtime Outcome/Disposition |
| Governance V2跨Owner接线 | **completed / reference**：Checkpoint Harness Gate→两个测试Participant→Manifest/Seal→Runtime Consistency；Restore Intent→治理→Sandbox Stage→Evidence/Settlement→Context→Activation | production Snapshot capture、trusted Assembler、跨Owner全量Participant、remote Provider与CLI/root保持unsupported |
| Workspace Snapshot owner-local | Sandbox Artifact `reserved -> available`、Host Local加密Content Store、Workspace complete Coverage/Participant Owner Fact、Snapshot aggregate exact-current复读、Application Owner-current映射及`prepare/capture -> Owner S1 -> commit -> Owner S2`顺序已实现 | Evidence/Settlement Lifecycle与production root尚未闭合；不构成production Checkpoint或Restore |

## 2. 最终可预见产物

经设计与计划批准、公共Delta闭合并完成实现后，预期产生：

1. Provider无关、版本化的Continuity合同与验证器；
2. Evidence Ledger单主之上的Timeline Projection、因果图、Query/Watch/Cursor；
3. SQLite Continuity Fact/CAS/关系/Journal默认实现；
4. RocksDB内容寻址Chunk/Delta/Fragment/加密Blob默认实现；
5. SQLite↔RocksDB崩溃可恢复Write-Ahead闭环；
6. Checkpoint Manifest、Snapshot Binding、Effect Cut与诊断资产；
7. Fork/Rewind/Restore Plan与Recovery Credential；
8. Runtime/Application适配层，不重造Operation/Evidence/Review/Settlement链；
9. Backend/Participant/Policy/Clock/Key Provider窄Port及测试Fake；
10. Go SDK、Transport-neutral API合同和CLI命令映射；
11. Conformance、单元、白盒、黑盒、故障注入、race、vet、集成与系统测试资产；
12. 性能基准与容量报告；无证据时不引入Rust。

Checkpoint-first公共reference Delta已完成；当前可交付上限包含可重复验证的跨Owner reference纵切，但不能宣称production Checkpoint或Restore可用。

## 3. 范围与不做事项

### 3.1 实现范围

- Continuity领域合同、Fact、状态机、存储与查询；
- Runtime V2/V3公共合同Adapter；
- SQLite+RocksDB默认Backend与可替换SPI；
- 加密Snapshot/Object metadata与Key Provider引用边界；
- 只读Projection SDK/API；
- Application namespaced workflow Adapter（公共装配完成后）；
- Component Manifest V2、Capability、Schema和Run Requirement声明候选；
- 全套自动化验证。

### 3.2 明确不做

- 不实现Runtime Checkpoint/Restore/Outcome/Binding/Policy/Trust事实；
- 不实现Harness Session或私有Port；
- 不实现Sandbox ChangeSet、Tool/MCP执行、Review Verdict、Context渲染、Memory/Knowledge晋升；
- 不调用Model Invoker internal、厂商SDK、Raw/Native event；
- 不选择RPC/进程拓扑、生产对象库、生产KMS或SLA；
- 不宣称外部世界回滚或Exactly Once；
- 不让Timeline成为Evidence第二主库；
- Restore Stage只经已实现的typed scope、Eligibility、Admission/Review/Permit/Begin、双重Enforcement、Evidence/Settlement公共链执行；不以`activation_attempt`或transport kind替代资格，不扩legacy闭表；
- 不为“高性能”泛化使用Rust。

## 4. 计划代码与文件级落点

以下文件树同时包含live落点与后续候选，不表示每一项已存在；精确live文件见[模块说明](../../module/continuity/README.md)。C-02与C-03分别落在`checkpoint_v2.go`和`restore_plan_v2.go`系列文件；跨Owner与Restore执行候选仍不得创建：

```text
ExecutionRuntime/continuity/
|-- go.mod
|-- doc.go
|-- README.md
|-- contract/
|   |-- version.go
|   |-- common.go
|   |-- timeline.go
|   |-- content.go
|   |-- artifact_relation_v1.go
|   |-- checkpoint.go
|   |-- checkpoint_governance_v2.go
|   |-- plans.go
|   |-- restore_plan_v2.go
|   |-- retention.go
|   `-- errors.go
|-- ports/
|   |-- timeline.go
|   |-- facts.go
|   |-- content.go
|   |-- artifact_relation_v1.go
|   |-- checkpoint.go
|   |-- checkpoint_facts_v2.go
|   |-- plans.go
|   |-- restore_plan_v2.go
|   |-- policy.go
|   |-- clock.go
|   `-- keys.go
|-- domain/
|   |-- projection.go
|   |-- content_journal.go
|   |-- artifact_relation_v1.go
|   |-- checkpoint_manifest.go
|   |-- checkpoint_manifest_v2.go
|   |-- fork_plan.go
|   |-- rewind_plan.go
|   |-- restore_plan.go
|   |-- restore_plan_v2.go
|   `-- retention.go
|-- storage/
|   |-- sqlite/
|   |   |-- schema.go
|   |   |-- migrate.go
|   |   |-- facts.go
|   |   |-- relations.go
|   |   |-- projections.go
|   |   |-- journal.go
|   |   |-- artifact_relation_v1.go
|   |   `-- restore_plan_v2.go
|   |-- rocksdb/
|   |   |-- keyspace.go
|   |   |-- chunks.go
|   |   |-- objects.go
|   |   `-- integrity.go
|   `-- coordinator/
|       |-- writer.go
|       |-- recovery.go
|       `-- compaction.go
|-- runtimeadapter/
|   |-- manifest_v2.go
|   |-- evidence_projection_v2.go
|   |-- operation_v3.go
|   |-- review_v2.go
|   |-- run_settlement_v2.go
|   `-- checkpoint_v2.go
|-- applicationadapter/
|   |-- timeline.go
|   |-- checkpoint.go
|   |-- plans.go
|   `-- retention.go
|-- sdk/
|   `-- client.go
|-- cli/
|   |-- commands.go
|   `-- render.go
|-- conformance/
|   |-- manifest.go
|   |-- timeline.go
|   |-- content.go
|   |-- participant.go
|   `-- backend.go
|-- fakes/
|   |-- clock.go
|   |-- fact_store.go
|   |-- content_store.go
|   |-- participant.go
|   `-- policy.go
`-- tests/
    |-- contract/
    |-- domain/
    |-- storage/
    |-- blackbox/
    |-- fault/
    |-- conformance/
    |-- integration/
    `-- system/
```

CLI目标命令名虽为`praxis ...`，Continuity独占目录只实现命令描述、参数合同与Adapter；根CLI注册由其Owner在联合接线任务中完成。

## 5. 技术选型计划

### 5.1 Go

Go负责所有首版合同、领域状态机、SQLite/RocksDB协调、Adapter、SDK、CLI映射与测试。实现前冻结Go版本与Runtime当前module兼容范围，但不得修改根workspace。

### 5.2 SQLite

driver选择前执行只读/原型评估：纯Go与CGO方案的WAL、事务、备份、取消、busy handling、cross compilation和维护状态。选择结果必须有决策记录；不因方便锁死公共Port。

### 5.3 RocksDB

评估Go binding的RocksDB版本兼容、CGO/动态库分发、Column Family、WriteBatch、Snapshot、Iterator、Compaction Filter、校验和、备份与崩溃语义。RocksDB是默认内容Backend，不是Timeline/Evidence Fact Owner。

### 5.4 Rust门禁

首版不规划Rust。只有同时满足以下条件才另立设计：

1. 可重复Go benchmark和CPU/heap profile定位到纯计算热点；
2. 热点不是数据库I/O、锁、CGO或配置问题；
3. Go算法/内存布局/批处理优化后仍未满足管理线批准的目标；
4. Rust方案明确Go API、FFI或独立进程边界、数据所有权、取消/超时、panic/崩溃、版本协商和Go回退；
5. Rust与Go基线在相同语料和故障条件下验证收益与风险。

当前没有上述证据，因此任何Rust代码均越界。

## 6. 依赖与阻塞

### 6.1 已闭合可复用

- Runtime P0.1–P0.6；
- Binding/Manifest V2；
- Operation Effect/Settlement V3执行链、Application Coordinator、Delegation/Prepare/Enforcement/Observation；
- Review V2、Evidence Ledger V2、Run Settlement/Lifecycle V2/V3；
- Harness Governed Session/Candidate V2/V3基础链。
- Runtime Checkpoint-first V2参考纵切：Attempt+Barrier、EffectCut、Checkpoint Evidence V1、Settlement V5、Consistency/Finalization及ManifestSeal Reader已完成第三次独立代码终审YES；`ProviderCalls=0`、`ProductionClaimEligible=false`；
- Runtime Operation Settlement V4公开合同与current Inspection基线已存在，但其closed applicability不包含Checkpoint/Restore；本计划不宣称可直接复用，也不扩表；
- Harness Assembly/CompiledGraph与公共Slot/Phase目录基线，但不包含Checkpoint运行时Participant接线；
- G6A Action Gateway及各Owner隔离切片基线；必须先完成跨Owner验收，后续只复用该Gateway，不重新实现。

Continuity只通过公开合同适配，不重造这些链。

### 6.2 必须先联合闭合

| 依赖 | Owner | 阻塞阶段 |
|---|---|---|
| Runtime Checkpoint-first V2到Harness/Application/Participant/Continuity的跨Owner装配与Conformance | Runtime+Harness+Application+各Participant | 阶段5端到端Checkpoint；Runtime Owner参考纵切本身已完成 |
| R-CTY-06 Evidence current/readability/tombstone Reader | Runtime Evidence | 阶段2 C-01 production S1/S2、subject-current index/absence watermark、共同TTL与current projection |
| typed Owner Fact current Reader routing | Application/Assembler+各Fact Owner | 阶段2 authoritative Timeline Projection |
| `RestoreGovernancePortV2`（R-CTY-03） | Runtime | Reservation/Eligibility已实现；Admission/Authorization/Permit/Stage/Activate最小公共参考链也已接通，production装配待独立验收 |
| R-CTY-04 Participant V2 | Runtime+各Participant | 阶段5/6 |
| Restore typed Evidence/Settlement公开合同 | Runtime Evidence/Settlement | Sandbox DomainResult后发布Evidence并形成opaque Runtime Settlement；production source registration/root待独立装配 |
| H-CTY-01 Harness Checkpoint seam | Harness | Harness Required Participant |
| Agent Assembler最终Profile/Required Participant policy | Assembler/管理线 | 阶段5 Participant集合与阶段7公共装配 |
| Checkpoint Participant PortSpec与Slot/Phase运行时映射 | Harness接线 | H-CTY-01与G7 Capability接入 |
| G6A Action Gateway跨Owner验收 | Runtime/Application/Harness/Tool | 所有后续G6B/G7与Continuity接入的第一道门 |
| 既有G6A Action Gateway的Restore专用路由 | Runtime/Application及Sandbox Participant | 已实现最小公共参考链；production trusted Assembler/current route待验收 |
| G6B Context物化、Generation CAS/new Frame | Application/Context | Restore目标Scope物化已实现；真实Owner Reader/root待装配 |
| short-TTL/current `RestoreEligibilityFact`运行合同与接线 | Runtime | 阶段6每次Restore当前资格；不得与已落地immutable `CheckpointConsistencyFact`合并 |
| `CheckpointRestoreOperationScopeV2` + transport kind `praxis.runtime/restore-attempt` | Runtime | 阶段6 coordinate exact RestoreAttempt；transport kind不授资格，不得使用`activation_attempt`或扩V3/V4闭表 |
| Restore Stage专用Settlement + Checkpoint Settlement V5 | Runtime Settlement | 两者均保持opaque ref-only；Restore Stage已接Sandbox ApplySettlement，跨Owner全量Participant待装配 |

### 6.3 其他组件DAG

- Sandbox：Workspace ChangeSet、Snapshot、Lease/Residual/Cleanup；
- Context Engine：Context Frame/Generation/Reference materialization；
- Tool/MCP：Action Candidate、Effect/Settlement、MCP Session引用；
- Review：Rewind/Restore/Purge Candidate与Verdict；
- Memory/Knowledge：Snapshot/正式Fact ref；
- Model Invoker：仅RouteID+routegateway+公开execution union的Fact/Evidence ref。

## 7. 分阶段实施

### 阶段0：联合评审与合同冻结

- [ ] Runtime确认R-CTY-01～05的Owner、version、Schema、Capability与优先级；
- [ ] Harness确认H-CTY-01与Slot/Phase贡献；
- **partial / scope frozen** — 用户已确认A-CTY-01覆盖CLI/API受治理写面；Application公开Submission+Inspect Gateway、CompiledGraph和Binding/Requirement映射仍待Application/Assembler Owner实现与联合Review；
- [ ] 各Participant确认Snapshot/Coverage/Inspect语义；
- **partial / decided** — 首版Required Participant已冻结为Runtime/Harness/Context/Sandbox/Memory-Knowledge，Review/Tool仅exact refs；Workspace-only Rewind、本机SQLite+RocksDB无SLA和API治理写面已裁决。Retention细分类与企业部署容量Profile仍待后续管理线版本化；
- [ ] 冻结设计反例与实现允许范围。

完成条件：没有未授权私有兼容接口；每项blocked Capability明确标为unsupported。

### 阶段1：合同、Canonical与状态机

- **completed** — 已创建独立Go module及`contract`、`domain`、`ports`、`storage`、`conformance`包边界；
- **partial** — Wave 1共同Ref、bounds、canonical digest、version、Validate与Clone已实现；Checkpoint Manifest V2 exact refs与Seal、Restore Plan current Adapter及Runtime Attempt/Identity Reservation/Eligibility exact refs已实现；
- **partial** — Timeline、Content、Retention、reference-only Plan、Checkpoint Manifest Governance V2及Restore最小公共运行对象已实现；production trusted Assembler、跨Owner全量Participant和root仍未实现；
- **partial** — Wave 1及Manifest V2 create/CAS/finalization/immutable Seal、Restore Intent/Stage/Settlement/Context/Activation状态机已实现；production Checkpoint capture、跨Owner全量Restore Participant和root仍unsupported；
- **partial** — Wave 1与Manifest V2 stable typed errors已实现；Restore运行reason set仍待后续公共合同；
- **partial** — 已覆盖schema round-trip、nil/empty canonical、invalid state、V2 exact binding/typed-nil、Cursor canonical fuzz及History Derivation canonical-tamper fuzz；Restore运行schema未进入实现，其他合同fuzz仍待逐项扩展。

完成条件：非法输入在backend读取前失败；相同语义输入生成相同digest。

### 阶段2：Timeline Projection闭环

- **completed / owner-local** — coordinate-only `TimelineProjectionRequestV1`、create-once Attempt、history/current Inspect、exact revision CAS、same-ID exact幂等/changed Conflict、no-ABA与`reconcile_required`已实现；
- **completed / owner-local** — Runtime public Record双读与R-CTY subject-current S1/S2、exact Record/current/index绑定和fresh Validate已实现；Adapter只依赖Runtime `core`/`ports`，不依赖raw Fact Port或kernel/control/fakes/internal；
- **completed / owner-local** — 六类Runtime Trust闭路由已实现；五类Observation形态不调用Owner Reader，只有`authoritative_fact`要求typed Owner Router并在缺失时Fail Closed；
- **completed / owner-local** — Projection Policy与按需Owner自然sealed TTL取共同最小值，`RequestedNotAfter`只在最后缩短；Event+visible Attempt+Continuity current index原子提交；
- **completed / owner-local** — Continuity-owned Projection Policy exact ref、create-once/history/current/CAS、active/revoked/expired闭状态、stable digest与fresh natural TTL已实现；它只证明opaque policy current，不发明业务判定；
- **completed / owner-local** — 同Request完成重放只Inspect既有结果；lost publish reply保留已提交Event/Attempt并只允许exact Inspect；64路CAS/no-ABA等Repository反例已覆盖；
- **completed / owner-local** — `TimelineProjectionAdapterV1.Rebuild`逐Request复用同一Project Controller；没有bulk caller Event import或scope替换；
- **completed / reference demotion** — 旧caller Candidate路径显式改名`ReferenceTimeline`，不再与生产形状同名；public `ReplaceLedgerScope`/`TombstoneProjection`已删除，Tombstone为immutable Fact+overlay，historical Event零改写；
- **completed** — Event/Parent/Causation/Correlation/Object关系、Query、Watch、Cursor及重建冲突/环检测已实现；Query逐项覆盖Identity/Lineage/Instance/Run/Turn/Step/Action/Artifact/Effect/Review Case/Checkpoint/Time，类型化ObjectRef按AND匹配并全部进入Cursor Query digest；
- **pending / cross-owner** — 各authoritative领域提供真实typed Owner current Reader，Application/Assembler绑定Consumer/Policy/Owner route且不得装配`ReferenceTimeline`或raw Store；
- **completed / owner-local** — SQLite production repository已覆盖Event/Attempt/Policy/current原子事务与durable reopen；RocksDB内容Backend及跨存储Journal recovery已实现；`timeline-durability`跨Owner Requirement Participant Fact仍unsupported。

完成条件：owner-local代码条件已满足；生产完成仍要求typed Owner Readers、Application/Assembler binding与production persistence root联合Conformance YES，并证明`ReferenceTimeline`/raw Store不可装配。

### 阶段3：SQLite+RocksDB默认Backend

- **completed** — 以Conformance与5轮Benchmark冻结driver/build策略：SQLite选pure-Go modernc；RocksDB使用`continuity_rocksdb` build tag与窄C API bridge，不把CGO类型暴露给领域层；
- **completed** — SQLite schema version 3/additive migration、WAL/FULL/IMMEDIATE/busy-timeout、Fact history/current/CAS、对象关系与Journal已实现；
- **completed / current scope** — RocksDB `chunk/v1/<sha256>` keyspace、sync WAL、checksum read、Snappy compression、Chunk integrity与metrics已实现；未使用Column Family，因为当前只有单一Chunk kind，不预造拓扑；
- **completed** — SQLite metadata↔RocksDB content writer/recovery已通过四个durable Journal cut关闭/重开黑盒；
- **completed / metadata boundary** — content addressing、chunking与encryption envelope ref已实现；实际KMS/加解密Provider仍unsupported；
- **completed / diagnostic-only** — dangling/missing/corrupt读取Fail Closed、Journal Inspect、bounded Subject双轮Inspect、immutable `ContentIntegrityAuditFactV1`、lost-reply exact Reader、内存repository及SQLite（自schema v6引入，当前schema v9）均已实现。该诊断不证明全库无孤儿；自动orphan reclaim/跨Owner引用闭包仍待阶段4；
- **partial** — 本地数据库reopen恢复语义已实现；在线backup/remote archive/灾备与SLA仍unsupported。

完成条件：所有持久阶段注入崩溃后可Inspect并唯一收敛；无声悬挂引用为零。

### 阶段4：Retention与后台整理

- **partial** — Retention/Legal Hold/Tombstone元数据CAS、Create/CAS lost-reply exact Inspect、changed winner Conflict、typed-nil及SDK fail-closed已实现；physical purge明确unsupported，未形成远程Purge Fact执行链；
- **unsupported** — Compactor/Indexer/Consolidator派生对象与后台执行未实现；
- **partial** — 本地对象引用与Legal Hold保护已实现；跨Owner Checkpoint/Fork/Review引用闭包及current reader未接入；
- **unsupported** — Remote Put/Delete/Archive/Purge Operation V3 Adapter未实现；
- **partial** — 本地Content Journal与Retention Create/CAS Unknown/lost-reply可Inspect原记录并恢复；远程Operation Attempt/Provider路径未实现；
- **unsupported** — content residual Run Requirement及Runtime/Application装配Adapter尚未实现。

- **completed / diagnostic-only** — C-06 Content Integrity Audit已落在`contract/content_integrity_audit_v1.go`、`ports/content_integrity_audit_v1.go`、`domain/content_integrity_audit_v1.go`、memory/SQLite repository、lost-reply fake、只读SDK及unit/blackbox/fault/conformance测试；不枚举全库、不回收、不删除、不调用Provider。
- **completed / relation-only** — C-07 Content Delta已落在`contract/content_delta_v1.go`、`ports/content_delta_v1.go`、`domain/content_delta_v1.go`、memory/SQLite（自schema v7引入，当前schema v9）repository、lost-reply fake、只读SDK与unit/blackbox/fault/conformance测试；不创建Target、不执行Compaction、不删除base或Chunk。
- **completed / candidate-only** — C-08 History Derivation Candidate已落在`contract/history_derivation_candidate_v1.go`、`ports/history_derivation_candidate_v1.go`、`domain/history_derivation_candidate_v1.go`、memory/SQLite（自schema v8引入，当前schema v9）repository、lost-reply fake、只读SDK与unit/blackbox/fault/conformance测试；不运行Compactor、不发布current、不改写Event。

- **completed / owner-local reference** — `ArtifactRefV1` + immutable `ArtifactRelationFactV1`因果索引纵切；coordinate-only Request、Timeline exact Event与typed Artifact Owner source projection S1/S2、same-ID/idempotency create-once、lost-reply Inspect、Artifact/Related indexes、内存repository及SQLite（自schema v5引入，当前schema v9）、只读SDK、64路并发/黑盒/故障/Conformance均已实现。真实typed Owner Router/Application root仍unsupported，不能宣称production `AttachArtifact`。

完成条件：Legal Hold不可绕过；Unknown remote delete不谎报Cleanup完成。

### 阶段5：第一阶段——Checkpoint资格与Manifest聚合

前置：G6A Action Gateway/跨Owner验收→G6B Context Refresh→Harness G7 Checkpoint门已经按序通过；随后才接R-CTY-02/04，Harness作为Participant还需H-CTY-01。既有Action Gateway只复用不重造；Settlement V4虽live，但不宣称已适用于Checkpoint/Restore，也不扩其闭表。

- **completed** — `CheckpointManifestFactV2` exact-ref schema已实现；跨Owner引用携带contract/schema、Owner Binding、TenantID、ID、revision、digest与ScopeDigest；IdentityKey使用完整OwnerBinding可比较结构，拒绝delimiter collision、Owner字段漂移碰撞与跨Tenant串键，并递归拒绝跨Tenant/Scope splice；
- **completed** — V2 Manifest不再使用裸`RuntimeStateRef/RunSessionRef/ContextGeneration`，且不提供legacy默认补值迁移；
- **partial** — Snapshot/coverage、Manifest与Effect Cut exact引用闭包已实现；两个测试Participant已走公开Adapter，真实Snapshot capture与生产Participant执行未实现；
- **completed（Seal reference Adapter）** — Context Generation/Frame、Attempt、opaque Settlement、Memory、Knowledge、Participant/Evidence/Diagnostic/Residual引用闭包已纳入canonical frozen-set；Runtime exact Seal lookup、RuntimeClosure mapping、Runtime Participant Set及Context/Artifact closure digest Adapter已接入并在Gateway CAS前S1/S2复读；
- **completed（Runtime Owner reference纵切）** — Runtime CheckpointAttempt+Barrier/EffectCut/Consistency/Finalization公共参考实现已完成第三次独立代码终审YES；Continuity不实现或复制这些对象；
- **completed / reference** — Participant Commit/Inspect通过Harness/Sandbox公开Adapter进入组合测试；这些Fact仍由Participant Owner形成，Continuity不实现Participant，Provider调用保持为0；
- **completed** — 每个已Begin Attempt缺Settlement时强制exact Inspection+Residual，且不得进入`verified_candidate`；
- **completed** — 顶层、所有Attempt（含已有Settlement）、Participant与任意severity Diagnostic Residual均聚合；`verified_candidate`与Seal要求聚合Residual为空；
- **completed** — `diagnostic_partial/diagnostic_indeterminate/rejected` finalization及terminal状态机已实现；
- **completed** — Manifest revision create-once/CAS、immutable history、按Tenant/Scope/ID结构化隔离的Owner current/exact historical Inspect、same-request幂等、changed Conflict及progressed replay no-ABA已实现；
- **completed** — immutable revision 1 Manifest Seal Repository/Reader已实现，事务内重验current `verified_candidate`、Owner及Manifest/Attempt/Barrier/EffectCut/frozen/required/runtime Participant set、Runtime closure refs、Context/Artifact closure digests；直接Repository调用不可绕过，lost reply只Inspect原Seal；
- **completed（Runtime Owner reference纵切）** — Runtime复读后提交immutable `CheckpointConsistencyFact`及其repository/current inspection已在Runtime Owner参考实现落地；Continuity本切面不写该Fact；
- **completed（跨Owner reference纵切）** — Runtime public exact Seal Reader、Continuity `runtimeadapter.CheckpointManifestSealReaderV2`、Harness Gate、Application Coordinator及Harness/Sandbox两个测试Participant已组合闭合；Provider=0，reference实现不构成生产Snapshot capture、production root或SLA资格；
- **unsupported** — Checkpoint attempts classified Run Requirement及Assembler接线尚未实现。

完成条件：Required缺失、unknown Effect、Attempt无Settlement、Generation/Frame换包、coverage不足、Context不可物化均不得consistent；Continuity从不产生restore eligibility；`CheckpointConsistencyFact`不得被解释为当前可Restore。

### 阶段6：第二阶段——Restore Plan、RestoreAttempt与新实例恢复

前置：阶段5已形成Runtime immutable `CheckpointConsistencyFact`；G6A Action Gateway、G6B Context Restore materialization与Runtime Restore公共合同已存在。`RestoreGovernancePortV2` Reservation/Eligibility及阶段6最小公共参考纵切已落地；production trusted Assembler、跨Owner全量Participant与deployment root仍须独立验收，不扩legacy闭表。

- **completed / owner-local shape** — Fork/legacy Rewind/Restore Plan纯验证、`RecoveryCredentialV1`、`RestorePlanFactV2`及Workspace-only `RewindPlanFactV2`已实现。Rewind V2包含exact Checkpoint/Manifest、Sandbox View/keep-drop/planned ChangeSet、Dependency/Review/irreversible Effect/Residual refs、stable tenant Workspace Conflict Domain、TTL/current、create-once/CAS/history/current、lost-reply、64路CAS/no-ABA、内存/SQLite schema v9与只读SDK；不授Runtime eligibility、Review Authorization、Permit/Fence、Sandbox ChangeSet创建或Provider能力；
- **completed / public reference** — Application先create-once封存immutable Restore Intent，丢回包只Inspect；Intent持久失败时Runtime Attempt保持不存在。transport kind只coordinate，Continuity不创建Runtime Intent或资格；
- [x] Runtime `RestoreGovernancePortV2` create-once保留同一`RestoreAttempt + new Instance/high Epoch/new Lease/Fence Reservation`；Plan Identity仅proposal，Reservation回包丢失只Inspect原Attempt/history；
- [x] Runtime Issue/Bind exact Plan/Attempt/Instance/Epoch/Lease/Fence Reservation的short-TTL/current `RestoreEligibilityFact`；过期、Plan或prerequisite漂移Fail Closed；
- [x] Eligibility exact schema、TTL、结构化CAS/Inspect key及prerequisite-reader DAG已实现：Review字段只含target/requirement/policy basis，Authority/Scope/Budget/Binding/Context均为requirement exact ref，不含accepted Verdict、Review Authorization或Permit；
- [x] Eligibility成功Issue/Bind后才进入Action Admission；Review Owner随后独立形成current Verdict/Authorization；该Authorization绑定资格Fact/Attempt及typed Restore payload exact refs，再依次Permit/Fence→Begin；
- [x] Begin后通过Restore专用Action Gateway及Runtime actual-point Enforcement进入Sandbox Participant；DomainResult后发布Evidence并由Runtime Settlement ref-only结算；
- **partial / two-owner local completed** — Rewind首版仅提交Sandbox Workspace Owner既有`praxis.sandbox/workspace-commit`受治理Effect；Continuity exact Plan/current/history/CAS/TTL/lost-reply已经实现。Sandbox `WorkspaceRewindCompositionPortV1`已从exact View与keep/drop refs生成新的staged ChangeSet和immutable Composition Fact，并覆盖lost-reply、64路并发及历史读取；Application/Assembler尚未把Composition exact ref接入既有治理链，Tool/远程补偿保持unsupported；
- [x] Runtime Restore Owner使用Reservation中已保留的新Instance/高Epoch/new Lease/Fence；Continuity只Inspect/关联opaque Attempt/Reservation refs；
- **partial / single Sandbox reference** — Sandbox Workspace Stage→Inspect→Validate、Context物化与Runtime Activate已形成最小纵切；跨Owner全量Participant的all-or-nothing Stage集合仍未装配；
- [x] Sandbox Participant形成DomainResultFact→Runtime Settlement(ref)→Participant ApplySettlement，不由Continuity代结算；
- [x] Context Restore在目标Scope物化并返回Context Owner exact Generation/Frame Fact，存在Residual或source ref不可读时Fail Closed；
- [x] Restore typed scope、Evidence/Settlement applicability与actual-point Enforcement已接入；禁止`activation_attempt`或transport kind替代资格，禁止扩legacy闭表；
- [x] legacy `RestoreRequest/RestoreCheckpoint/Foundation`保持restricted，未写兼容Adapter；
- [ ] production trusted Assembler/current Reader、跨Owner全量Participant、root credential/deployment attestation闭合前Capability保持unsupported。

完成条件：参考纵切已满足无半恢复Ready、Continuity无执行入口、Unknown/lost reply只Inspect原Attempt、Eligibility与Authorization分离、TTL/Authority/Scope/Fence漂移零Participant接触、Context Residual阻断Activation及重放不重复Effect。production完成仍要求trusted Assembler/current Reader、跨Owner全量Participant、root credential/deployment attestation与独立Conformance；旧unknown Conflict Domain不能通过Restore绕过。

### 阶段5/6固定实施顺序

1. G6A Action Gateway/跨Owner验收；
2. G6B Context Refresh；
3. Harness G7 Checkpoint门；
4. R-CTY-02/R-CTY-04 Runtime CheckpointAttempt/Barrier/Effect Cut/Participant V2；
5. H-CTY-01 Harness Participant与C-02 Continuity Manifest聚合；
6. Runtime复读并形成immutable `CheckpointConsistencyFact`；
7. C-03 Restore Plan；
8. Application提交绑定`CheckpointRestoreOperationScopeV2`与transport kind的Restore Intent；
9. `RestoreGovernancePortV2` create-once保留`RestoreAttempt + new Instance/high Epoch/new Lease/Fence Reservation`；
10. Runtime Issue/Bind short-TTL/current `RestoreEligibilityFact`，Review字段只含target/requirement/policy basis；
11. Action Admission；
12. 独立current Review/Authorization绑定exact Eligibility/Attempt→Permit/Fence→Begin；
13. 既有G6A Action Gateway Restore专用路由、`CheckpointRestoreEvidenceGovernancePortV1`与Harness公共Phase；
14. Participant Stage→Inspect→DomainResultFact→`OperationCheckpointRestoreSettlementSubmissionV5`/`OperationCheckpointRestoreSettlementGovernancePortV5`(ref only)→Participant ApplySettlement；
15. Runtime全量复读后Activate reserved Instance/Epoch/Lease或Abort。

C-02/C-03 exact-ref schema、canonical、migration/diagnostic规则属于第5步和第7步内部工作；A-CTY-01已以additive Application公开Gateway落地，不扩legacy Command enum，也不改变上述顺序。production trusted Assembler/current route/root仍是后续门。

任一步未闭合，后续步骤保持blocked/unsupported；禁止通过Continuity私有Port、legacy Adapter或测试Fake跳过。

### 阶段7：Manifest、公共装配、SDK/CLI/API

前置：Assembler/CompiledGraph/Slot/Phase/Binding映射/A-CTY-01。

- **completed / reference-only** — ComponentRelease候选已包含Component Manifest V2、Capability、Schema、Owner、Residual、Conformance、Factory descriptor与exact certification；缺全部production proofs，不能自升production；
- **completed / no contribution** — 当前Release显式提交空Slot/Hook/Phase贡献，不私建公共枚举；Checkpoint Gate仍由Harness Owner提供；
- **completed / reference, production root blocked** — Application namespaced workflow Adapter只依赖公开contract/ports并已落地；production trusted Assembler/current route/root缺失时Fail Closed；
- **completed / reference, production root blocked** — Go SDK query/watch/inspect/plan API与唯一`Submit/InspectGovernedWorkflow`治理写面已实现；Application公开Submission+Inspect Gateway、Continuity Adapter及七类closed kind已闭合。Timeline Page在SDK边界复验Event/过滤/sequence/PageLimit/Cursor exact watermark；production trusted Assembler/各kind current route未闭合时仍Fail Closed；
- **completed / owner-local mapping** — CLI只读`timeline show/watch`、`checkpoint inspect`，七类治理写命令、`workflow inspect`及strict JSON参数/输出/错误映射已实现；不注册根CLI，不选择endpoint/credential；
- **partial** — transport-neutral分页、Cursor与长任务exact Inspect已实现；production授权脱敏策略及其root装配仍由外部Policy/Application Owner提供，Continuity不发明redaction字段；
- [ ] 根CLI/API注册只输出集成增量，由Owner串行合入。

完成条件：CLI写命令不直达Fact Store；CompiledGraph/Binding不完整时Fail Closed。

### 阶段8：Conformance、性能与安全收口

- **completed / current Wave1** — `restricted_controlled + reference_only + no SLA`为唯一可接受声明；`fully_controlled`、`contained_observe_only`、`rejected`冒充当前Wave1均拒绝，supported/unsupported未知、缺失、重复项全部Fail Closed；
- [ ] backend/participant/provider conformance；
- **partial** — Cursor与History Derivation canonical fuzz、Artifact Relation/Content Integrity Audit/Content Delta/History Derivation/RestorePlan/Manifest/Timeline定向100轮、race20、full race/vet及SQLite/RocksDB benchmark已完成；当前全模块statement coverage实测59.5%，尚未冻结覆盖率目标，跨Owner系统benchmark仍未完成；
- [ ] 敏感Snapshot、Key不可用、Retention未知、Provider残留测试；
- [ ] 记录真实命令、结果、覆盖和未覆盖风险。

完成条件：所有声明Capability与真实测试一致；Fake不进入生产认证。

### 阶段9：联合集成与系统测试

由主协调和各Owner决定执行时机：

- [ ] Runtime+Application+Continuity Event/Requirement闭环；
- [ ] Harness/Sandbox/Context等多Participant Checkpoint；
- [ ] Restore pre-run Evidence/new Instance/Ready闭环；
- [ ] Model Invoker public Route/evidence关联，不使用internal；
- [ ] 个人设备单Agent长Session；
- [ ] 企业多租户、多Identity、千Agent模拟负载；
- [ ] 长周期Retention/Compaction/Legal Hold/备份恢复。

完成条件：系统测试记录真实拓扑和限制；测试Backend不冒充生产SLA。

## 8. Effect、Conflict Domain、Review、Fence与Unknown执行计划

| Effect kind | 计划实现阶段 | Conflict Domain | Review/Budget | Fence/执行点 | Unknown |
|---|---:|---|---|---|---|
| `continuity/remote-content-put` | 4 | `continuity/content-object`+tenant | 披露/成本Policy | Host Gateway+remote adapter双验 | Inspect原object operation |
| `continuity/remote-content-delete` | 4 | `continuity/content-object`+tenant | Retention/Hold/Privacy | 双验+exact object set | Inspect原delete |
| `continuity/remote-archive` | 4 | `continuity/archive`+tenant | 区域/Retention/成本 | 双验 | Inspect原archive |
| `continuity/retention-purge` | 4 | `continuity/retention`+tenant | Policy决定Review | 双验+Hold复读 | Inspect原purge |
| `continuity/restore-stage` | 6 | `continuity/restore`+tenant | Restore Review/Budget | `CheckpointRestoreOperationScopeV2`+Permit+Participant执行点；实现未获联合Review YES时零Provider调用 | Inspect原stage |
| `continuity/restore-activate` | 6 | `continuity/restore`+tenant | Runtime全量current复读 | Runtime/Participant双验 | Inspect RestoreAttempt，不重派 |
| `continuity/rewind-apply` | 6 | `continuity/rewind`+tenant | ChangeSet Review/Budget | Sandbox/Tool执行点 | Owner Inspect原Effect |
| `continuity/inspect` | 4/6 | 与原Effect一致 | Inspect Policy/Budget | 独立Permit/Fence | 非递归Settlement链 |

每项外部动作顺序固定为：领域Intent/Reservation→Admission→Review/Authorization→Permit→Begin→Delegation/Prepare→Enforcement→Execute/Inspect→Observation/Evidence→领域Owner DomainResultFact→Runtime Operation Settlement→领域Owner ApplySettlement。

Restore冻结特化序列为Plan→Application Intent→Runtime create-once Attempt/Instance/Epoch/Lease/Fence Reservation→Issue/Bind short-TTL Eligibility→Action Admission→独立current Review/Authorization绑定前述两项exact refs→Permit/Fence→Begin→Stage。资格Fact不得后置。

## 9. RunSettlementRequirement实施计划

| RunSettlementRequirement | Phase | 实现 | 证据 | 失败处理 |
|---|---|---|---|---|
| `continuity/timeline-durability` | completion | 阶段2 | Continuity Owner Fact+Evidence ref | unknown阻止或按可信Plan Policy处理 |
| `continuity/checkpoint-attempts-classified` | completion | 阶段5 | Attempt/Manifest分类Fact | 未分类不得CompleteRun |
| `continuity/content-residuals` | termination_report | 阶段4 | Journal/remote retention/cleanup Fact | 显式Residual，不谎报closed |

组件只提交声明和Participant Fact；Trusted Assembler决定最终Plan与not-required Policy。

## 10. 测试矩阵

### 10.1 合同/单元

| 范围 | 必测内容 |
|---|---|
| Canonical | nil/empty、排序、bounds、schema/version、digest漂移 |
| Timeline | source重放、换内容Conflict、parent/cycle、cursor/query/authority漂移 |
| Artifact Relation | coordinate-only字段闭集、typed Owner S1/S2、Artifact/Related/Evidence/Storage/Parent漂移、跨Tenant/Owner splice、immutable digest |
| Content Integrity Audit | coordinate-only Subject、Manifest/Journal/Chunk S1/S2、missing/corrupt/unavailable、same-ID drift、lost reply、typed-nil、deep clone、跨Tenant隔离、零Purge |
| Content Delta | coordinate-only Base/Target、Manifest/visibility/Chunk S1/S2、exact schema+digest+length reuse、missing/corrupt/scope splice、same-ID drift、lost reply、deep clone、零patch/Purge |
| State machine | 每条合法边、每条非法跳转、terminal不可复活、CAS revision |
| Checkpoint | required/optional、coverage、Effect Cut洞、partial/unknown、immutable Consistency与current Eligibility分离 |
| Checkpoint V2 refs | 裸字符串拒绝、Generation/Frame闭包、Attempt/Settlement同源、Memory/Knowledge exact ref、frozen-ref-set digest |
| Plans | new Instance/high Epoch、Authority ceiling、Review requirement/candidate（非Verdict）、stable Conflict Domain、TTL、不可逆Effect继承 |
| Restore governance | create-once Attempt/Instance/Epoch/Lease/Fence Reservation、Eligibility先Issue/Bind再Action Admission、独立Authorization绑定前述资格Fact/Attempt exact refs、Continuity无执行能力、legacy不得扩权、V3/V4闭表不得扩展 |
| Retention | Hold、Tombstone、Purge、引用闭包 |
| RunSettlementRequirement | subject digest、Owner、Evidence trust、unknown policy |

### 10.2 白盒

- SQLite transaction/WAL/busy/cancel/migration；
- RocksDB WriteBatch/Snapshot/Iterator/Compaction/校验和；
- cross-store Journal每阶段恢复；
- Projection rebuild与当前revision CAS；
- Artifact Relation same-ID/idempotency create-once、64路不同内容单赢家、两个exact索引与深拷贝；
- Content Integrity Audit same-ID/idempotency create-once、64路不同Finding单赢家、历史exact Reader与深拷贝；
- Content Delta same-ID/idempotency create-once、64路不同Fact单赢家、历史exact Reader与深拷贝；
- 引用计数/闭包/Chunk去重；
- 锁顺序、bounded queue、backpressure与goroutine退出；
- Adapter导入边界和非法输入零backend读取。

### 10.3 黑盒

- Append→Inspect→Query→Watch完整Timeline；
- 多Source/多Epoch/迟到历史/分支查询；
- Artifact Relation经SQLite创建、关闭重开后exact Inspect并保持Timeline origin可读；
- Content Integrity Audit经SQLite（自schema v6引入，当前schema v9）创建、关闭重开后exact Inspect，诊断Fact保持immutable；
- Content Delta经SQLite schema v7创建、关闭重开后exact Inspect，结构共享Fact保持immutable；
- History Derivation Candidate经SQLite（自schema v8引入，当前schema v9）创建、关闭重开后exact Inspect，ordered source refs与candidate-only Fact保持immutable，source Event零改写；
- 大对象Chunk/去重/压缩/读取校验；
- Checkpoint diagnostic、immutable consistency与per-Restore current eligibility三者分离；
- Rewind Plan不改现实；Restore Plan不创建Instance；
- SDK/CLI分页、Cursor、脱敏、typed error；
- observe-only能力不暴露写入口。

### 10.4 故障注入

| 注入点 | 期望 |
|---|---|
| Evidence append持久成功但回包丢失 | Inspect source key，Projection只一次 |
| Artifact Relation commit成功但回包丢失 | 按原Tenant/Scope/Relation ID Inspect；不重读Owner、不换ID |
| Content Integrity Audit commit成功但回包丢失 | 按原Tenant/Scope/Audit ID Inspect；不重扫Content、不换ID |
| Content Delta commit成功但回包丢失 | 按原Tenant/Scope/Delta ID Inspect；不重读Base/Target、不换ID |
| Artifact/Related/Evidence/Storage/Parent任一S1/S2漂移 | `conflict|indeterminate`；零Relation Fact、零索引 |
| SQLite metadata pending后崩溃 | Journal恢复，不产生可见dangling ref |
| RocksDB content staged后崩溃 | orphan可Inspect，不提前可见 |
| SQLite ref committed回包丢失 | Inspect exact revision，不重写Object |
| Chunk损坏/缺失 | integrity失败，Checkpoint/读取Fail Closed |
| Compaction中断 | 旧current仍有效，新对象不越权切换 |
| Remote Put/Delete timeout | Operation unknown，只Inspect原attempt |
| Participant Prepare回包丢失 | Inspect participant fact，不重Prepare |
| Manifest CAS回包丢失 | Inspect原Manifest ID/revision/digest，不创建替代Manifest |
| Runtime Barrier回包丢失 | Inspect原CheckpointAttempt/Barrier，不生成新Checkpoint ID |
| Runtime Restore Reservation回包丢失 | 只Inspect原RestoreAttempt/Instance/Epoch/Lease/Fence Reservation，不创建第二组身份 |
| RestoreEligibilityFact过期/换包 | Begin前Fail Closed，Provider零调用；不得用CheckpointConsistencyFact替代 |
| Eligibility尚未Issue/Bind便进入Action Admission | 拒绝；不产生Authorization/Permit，不调用Provider |
| Eligibility包含accepted Verdict/Review Authorization | Schema拒绝；只允许Review target/requirement/policy basis |
| Authorization未绑定exact Eligibility/Attempt或未位于Action Admission后 | Fail Closed；不得Permit/Fence、Begin或Stage |
| V2/V1/V5设计候选已冻结但公共代码合同/联合Review YES缺失 | Capability unsupported、Provider零调用；不得用`activation_attempt`、transport kind或扩V3/V4闭表替代 |
| Runtime Permit/Begin回包丢失 | Inspect原Dispatch Record/Attempt，不签发新Permit |
| Restore Stage部分成功 | Abort/Residual，不得Ready |
| Restore Stage回包丢失/unknown | Inspect原Participant attempt与Settlement，不换Attempt、不Activate |
| ContextReference不可物化 | Fail Closed或Residual |
| Context refresh未接线或返回旧Frame | Capability unsupported/拒绝，不得复用旧Generation进入新Instance |
| Authority/Binding/Review/Budget/Fence在Begin前漂移 | 零Provider接触 |
| Begin后漂移/timeout | Inspect并Settlement，不重派 |
| Fork后旧unknown仍存在 | 新Lineage/Instance不得释放原tenant稳定Conflict Domain，不继承全部Authority |
| Restore/Rewind请求回滚外部动作 | 仅记录历史Effect/Residual并产生新补偿候选，不宣称外部世界回滚 |

### 10.5 Conformance

- Manifest/Capability/Locality/Owner/TTL/Residual完整；
- fully/restricted/observe-only/rejected四级；
- 第二sequence、敏感明文、无Inspect Effect、不可Fence删除必须Rejected；
- Fake标识测试用途，不能通过生产Backend认证；
- Remote Provider必须声明Residual/Inspect/Retention/Encryption能力。

### 10.6 Race/Vet/Fuzz计划命令

实现后在`ExecutionRuntime/continuity`运行并记录真实结果：

```bash
go test -count=1 -shuffle=on ./...
go test -count=20 ./...
go test -count=1 -race -shuffle=on ./...
go vet ./...
go test -run=^$ -fuzz=Fuzz -fuzztime=30s ./contract/...
go test -coverpkg=./contract,./ports,./domain,./storage/...,./runtimeadapter,./applicationadapter -coverprofile=/tmp/praxis-continuity.cover ./...
```

具体Fuzz入口按package拆分，不能用一条不存在的命令冒充已运行。

### 10.7 集成测试

- Runtime Evidence V2→Continuity Projection；
- Runtime Run Settlement→Continuity Participant Fact；
- Runtime Checkpoint/Participant/Manifest（Delta完成后）；
- Harness Snapshot contribution（H-CTY-01完成后）；
- Sandbox ChangeSet Rewind；
- Context generation/materialization；
- Review V2条件审核；
- Model Invoker只用公开Route/execution union。

### 10.8 系统测试

- 单机断电/kill -9/磁盘满/只读文件系统/损坏恢复；
- SQLite备份+RocksDB checkpoint一致恢复；
- 多租户同ID隔离、旧epoch迟到、长时间Watch；
- 千Agent模拟、长Session、Retention/Compaction并发；
- 远程对象库网络分区、删除未知、Key失效、区域策略；
- Restore后旧Instance/unknown Effect冲突隔离。

系统阈值必须由管理线批准，不在计划中伪造生产SLA。

## 11. 性能与容量验证

建立以下Go benchmark和profile，不预设通过数字：

- 单/多Source Evidence projection append；
- Query/Watch不同过滤维度与page size；
- SQLite CAS/WAL批量；
- RocksDB Chunk大小、压缩、WriteBatch、读取校验；
- 相同内容去重与Fork/Checkpoint结构共享；
- cross-store recovery扫描；
- Retention引用闭包与Compaction；
- Manifest/Effect Cut规模增长；
- 千Agent并发下锁、队列、内存、磁盘放大和尾延迟。

必须输出CPU/heap/block/mutex profile、写放大、压缩比、WAL/Compaction指标和测试硬件/数据集。只有管理线基于报告设置容量/SLA目标。

## 12. 迁移、兼容与回退

1. legacy Timeline、`core.CheckpointSet/RestoreRequest`、`CheckpointParticipantPort`与Foundation协调器保持restricted；Governance V2并行发布，不原地升权、不补默认Review/Fence。
2. 不做Evidence V2与Continuity Event双写；Timeline Projection随时可从Evidence重建。
3. SQLite migration使用版本表、向前校验、备份和只读回退；失败不切writer。
4. RocksDB keyspace按schema version隔离；新reader先双读验证，writer始终单主。
5. feature/capability未通过Conformance时不在Manifest声明bound。
6. production Restore root、production Harness Checkpoint、Remote Purge等可独立保持unsupported，不影响已实现的只读Timeline和Restore最小参考纵切。
7. 回退不删除用户数据；停止新writer后保留Fact/Journal供Inspect，由后续授权任务决定清理。

## 13. 风险与缓解

| 风险 | 缓解 |
|---|---|
| Evidence/Timeline双主 | Projection只复用Evidence Record sequence，禁止第二append |
| SQLite/RocksDB跨存储不一致 | Write-Ahead Journal+精确Inspect+故障注入 |
| RocksDB CGO/分发复杂 | driver spike、构建矩阵、Backend SPI；不影响公共合同 |
| Snapshot明文/Key丢失 | encryption envelope、Key ref、Fail Closed/Residual |
| Checkpoint覆盖虚假完整 | Participant Owner Inspect+Runtime资格Owner+coverage policy |
| Restore半激活 | Stage all/Validate/Activate together，Runtime Attempt CAS |
| Restore参考实现被误当production GO | ComponentRelease保持`reference_only`；trusted Assembler、跨Owner全量Participant、remote Provider和deployment attestation未闭合前production Capability unsupported |
| immutable Consistency被误当current Eligibility | Runtime分别Owner `CheckpointConsistencyFact`与short-TTL `RestoreEligibilityFact`；Eligibility在Action Admission前独立Issue/Bind，过期Fail Closed |
| Reservation丢回包后身份分叉 | 只Inspect原RestoreAttempt/Instance/Epoch/Lease/Fence Reservation，禁止创建第二组对象 |
| Slot/Phase私有分叉 | 只声明Observer/Gate/Port贡献，等待公共namespaced对象 |
| ContextReference不普适 | Fail Closed或Residual，不宣称普遍恢复 |
| 高频历史拖垮热路径 | bounded queue、batch、Content Addressing、冷热分层、benchmark |
| Rust过度引入 | 严格profile/目标/回退门禁 |

## 14. 管理线必须裁决的问题

1. 首个Checkpoint Required/Optional Participant及coverage policy；
2. Harness Checkpoint最小恢复粒度；
3. production trusted Assembler、首批跨Owner Restore Participant集合与deployment root何时完成联合Conformance并启用production Capability；
4. 首批Rewind ChangeSet、Review与补偿策略；
5. Retention/Legal Hold/Privacy Erasure Owner和数据分类；
6. 远程对象库/KMS/区域/删除Inspect首版范围；
7. SQLite/RocksDB driver、CGO、打包、备份和运维方案；
8. 个人/企业负载目标与成本上限；
9. **已裁决**：首版CLI/API包含受治理写路径；必须经Application公开Submission+Inspect Gateway，不能直达Fact Store或把接口存在解释为Provider已启用；
10. Run Requirement的Required/not-required Policy；
11. Agent Assembler最终Profile/Required Participant policy，以及Checkpoint Participant PortSpec、Slot/Phase运行时映射的发布时间。
12. `RestoreEligibilityFact`的TTL上限、current readers、CAS/Inspect key与过期后重建策略；

## 15. 计划完成条件

本计划进入“已审核、可实现”必须同时满足：

- design/plan联合评审通过；
- 所有实现首阶段所需Port Delta已由Owner冻结；
- 管理线明确首版Capability与unsupported列表；
- 文件级产物、测试矩阵、技术选型评估和回退获得确认；
- 主协调明确说可以开始实现。

当前Checkpoint/Restore最小公共参考实现已经落地；在上述production门禁通过前，ComponentRelease继续保持`reference_only`，不新增remote Provider、跨Owner全量Participant或production root路径。
