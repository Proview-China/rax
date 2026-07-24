# Continuity Wave 1 + Checkpoint Manifest Governance V2

> Continuity Owner Manifest/Seal V2、Checkpoint-first纵切、A-CTY-01治理写面及Restore最小公共参考纵切已实现；Restore纵切包含Application immutable Intent、Runtime Reservation/Eligibility、Admission/Review/Permit/Begin、双重Enforcement、Sandbox Workspace Stage、Evidence/Settlement/ApplySettlement、Context新Generation/Frame与新Instance/高Epoch/new Lease Activation。这不解锁production trusted Assembler/root、跨Owner全量Participant、远程Provider或SLA。

本模块实现Praxis Continuity领域闭环，Go版本为1.25。领域包保持组件自包含；`runtimeadapter`仅依赖Runtime公开`core`与`ports`，不导入Runtime kernel/control/foundation/fakes/internal，也不导入Harness、Application、Model Invoker实现包。

## 已实现

- Evidence Ledger单主之上的`TimelineEventRecord`投影；Continuity复用ledger sequence，不分配第二sequence。
- 已由Evidence Ledger准入并可Inspect的Observation可进入Timeline，但保持`observation` TrustClass，不升级为领域Fact、Settlement或Run终态。
- Evidence ref、source epoch/sequence及ledger scope/sequence的幂等、重复和冲突判断。
- Timeline Inspect与过滤查询；`TimelineQuery`逐项支持Identity/Lineage/Instance/Run/Turn/Step/Action/Artifact/Effect/Review Case/Checkpoint/Time/Parent/Causation/Correlation，全部过滤字段进入canonical Query digest，稳定Cursor拒绝Authority/Policy/Query漂移并检测Watch gap。SDK在返回调用方前再次校验每条Event、过滤命中、严格sequence顺序、PageLimit和Cursor exact watermark，拒绝Reader漂移。
- Cursor同时校验摘要与strict canonical wire bytes；非canonical重编码Fail Closed并有fuzz覆盖。
- reference Projection重建逐项append/idempotent，不批量替换或删除历史。
- Timeline Tombstone使用immutable revision-1 Fact与visibility overlay；historical Event Inspect的revision/bytes不改写，公共Store不再暴露`ReplaceLedgerScope`/`TombstoneProjection`。
- C-01 `TimelineProjectionRequestV1`、create-once Attempt/history/current/CAS、Runtime Evidence Record双读与R-CTY current S1/S2、Policy/typed Owner路由、fresh TTL、六类Trust、原子Event+Attempt+current index、lost-reply Inspect及逐Request Rebuild；旧caller Candidate路径显式为`ReferenceTimeline`测试形状。
- Continuity-owned Projection Policy exact ref、create-once/history/current/CAS、closed current state、stable digest与自然TTL reference repository；不承载未确认的业务Policy枚举。
- `ArtifactRefV1`与immutable revision-1 `ArtifactRelationFactV1`：production Request只携stable坐标；Controller两次精确读取Timeline Event与typed Artifact Owner source projection，S1/S2一致后才create-once写入Artifact/Related索引。真实typed Owner Router/Application root未装配时保持reference-only。
- `CheckpointManifest`、`ForkPlan`和legacy Rewind/Restore Plan纯验证；`RestorePlanFactV2`与Workspace-only `RewindPlanFactV2`进一步实现exact refs、闭状态机、TTL/current、create-once/CAS/history/current、lost-reply Inspect及SQLite持久化。Rewind V2只绑定Checkpoint/Manifest、Sandbox View/keep-drop/planned ChangeSet、Dependency/Review/irreversible Effect/Residual exact refs，不执行文件或创建Sandbox/Runtime事实。
- Sandbox Owner现已提供`WorkspaceRewindCompositionPortV1`：从exact Workspace View与keep/drop ChangeSet refs结构化生成新的staged ChangeSet，并以SQLite revision-1 immutable Composition Fact封存；same-request幂等、changed Conflict、lost-reply Inspect、64路并发单赢家及历史过期后可读均已覆盖。它不提交文件Effect；Application/Assembler仍须把该结果接入既有`workspace-commit`治理链。
- `runtimeadapter.RestorePlanCurrentReaderV2`只读exact submitted Plan、immutable Manifest Seal与Runtime Consistency，并向Runtime公开稳定natural-TTL current projection；Runtime `RestoreGovernancePortV2`随后create-once提交Attempt/new Instance/high Epoch/new Lease/Fence Reservation并Issue/Bind短TTL Eligibility。Application公开参考组合继续按Intent→Admission→Review/Authorization→Permit/Begin→双重Enforcement→Stage→Evidence/Settlement→Context→Activation收敛；Continuity不写这些Runtime或其他Owner事实，也不执行Stage/Activate。
- `RecoveryCredentialV1`短期、secret-free、可撤销的Restore Plan exact-binding纯验证；只允许`inspect/stage`形状，不授Runtime eligibility、Permit、Fence、Dispatch或Provider能力。
- approved `CheckpointManifestGovernancePortV2`：V2 exact refs、Manifest create-once/CAS、Owner current与history Inspect、diagnostic/residual finalization；全部层级exact refs递归绑定Manifest Tenant/Execution Scope，`verified_candidate`要求聚合Residual为空。Exact identity使用包含完整`OwnerBinding`的可比较结构键，不使用允许分隔符碰撞的字符串拼接。
- immutable revision 1 `CheckpointManifestSealFactV2`：Repository事务内复读current `verified_candidate`并exact绑定Manifest revision/digest/Owner、Attempt/Barrier/EffectCut、frozen-ref-set与required Participant closures；same-ID exact幂等、changed Conflict、lost reply只Inspect原Seal。
- 并发安全的Manifest/Seal内存reference repository与lost-reply fault fake；current/history按结构化Tenant/Scope/ID隔离，返回值均深拷贝，不暴露可变alias；progressed旧CAS重放Conflict且不形成ABA。
- Backend-neutral metadata/content/retention SPI；`storage/sqlite`已提供pure-Go WAL/FULL SQLite schema v9实现，覆盖Journal/Object/Retention、Timeline、Artifact Relation、Content Integrity Audit、Content Delta、History Derivation Candidate、Checkpoint Manifest/Seal及Restore/Rewind Plan history/current/CAS；`storage/rocksdb`在`continuity_rocksdb` build tag下提供窄C API生产Chunk实现。
- C-06 `ContentIntegrityAuditFactV1`：对调用方明确列出的Object/Journal坐标执行两轮Manifest/Journal/Chunk exact检查，封存immutable revision-1诊断Fact；missing/corrupt/unknown闭合为dangling/corrupt/indeterminate。它不枚举全库、不证明无孤儿、不回收、不删除、不调用Provider。
- C-07 `ContentDeltaFactV1`：从两个已可见且完整可读Object的Manifest/Chunk双轮检查派生ordered target recipe及reused/added/removed集合；只记录结构共享关系，不创建Target、不执行patch/Compaction、不删除base/Chunk。
- C-08 `HistoryDerivationCandidateFactV1`：从同Execution Scope的ordered exact Timeline Event集合与已可见output Content Object双轮检查派生immutable revision-1 candidate-only Fact；不证明summary/index正确，不发布current，不改写Event，不执行Compaction/Purge。
- `applicationadapter.GovernedWorkflowAdapterV1`只依赖Application公开`contract/ports`，要求Continuity-owned exact Domain Request Ref；`sdk`新增唯一受治理写入口`Submit/InspectGovernedWorkflow`，七类写请求只交给Application Gateway，不持Fact Store、不构造raw Bundle、不调用Provider。
- `cli`实现Continuity自有命令描述与strict JSON参数映射：只读`timeline show/watch`、`checkpoint inspect`，治理写`timeline project`、`checkpoint create`、`fork`、`rewind plan`、`restore`、`artifact attach`、`retention resolve`及`workflow inspect`；根CLI注册、endpoint、credential与production root不在本模块。
- `releasecandidate`通过Agent Assembler公共合同发布`reference_only` ComponentRelease候选；publisher遇到Ensure未知回包只按exact ref Inspect，Host只得到Factory descriptor。Wave1 Conformance的supported/unsupported声明为exact闭集，未知、缺失、重复能力均Fail Closed；只读Adapter已存在不等于production Runtime/Application root已解锁。
- 内容寻址Chunk、Manifest、跨存储Journal、精确Inspect恢复和完整性校验。
- Retention、Tombstone、Legal Hold元数据状态机；Create/CAS未知或丢回包按原Object/revision/content exact Inspect，changed winner Conflict；Physical Purge明确unsupported。
- `DomainResultFact -> Runtime Operation Settlement(ref only) -> ApplySettlement`分层；Runtime引用只保存identity/digest绑定，拒绝复制Outcome/Disposition等Runtime语义，ApplySettlement不解释领域Outcome。
- 并发安全的内存reference backend，仅用于测试与合同验证，不声明生产持久性或SLA。

## 明确不支持

Checkpoint-first reference纵切已闭合Harness Gate、Application协调、Harness/Sandbox两个测试Participant、Continuity Manifest/Seal、Runtime Consistency及Gate release；Checkpoint组合仍保持`ProviderCalls=0`。Restore最小公共参考纵切现已闭合Plan current、Runtime Reservation/Eligibility、Application治理链、Sandbox Host-Local workspace Stage、Evidence/Settlement、Context物化与Activation，并覆盖exact Inspect、CAS、lost reply和重放不重复Effect。Workspace-only Rewind的Continuity Plan与Sandbox结构化Composition已经闭合，但Application/Assembler尚未把Composition exact ref接入既有`workspace-commit`治理链。trusted Assembler/current Reader生产装配、Context与Memory-Knowledge等全量Participant、Run Settlement Participant create/CAS公共写口、root/credential/deployment attestation仍未闭合。真实remote blob/purge/archive及外部世界回滚仍未实现；`conformance.Wave1Manifest`继续把production执行能力声明为unsupported。

因此`releasecandidate`即使闭合Manifest/Module/Capability/Port/Factory/owners/artifact/candidate-certification/evidence/TTL，也固定为`reference_only`；强改production必须失败关闭。

## 包结构

```text
contract/          版本化对象、纯验证、状态机和稳定错误
ports/             Backend-neutral SPI
domain/            Timeline、Content Journal、Retention、Settlement及Manifest Governance
runtimeadapter/     仅消费Runtime public core/ports的Timeline治理适配
applicationadapter/ 仅消费Application public contract/ports的治理写面适配
storage/memory/    并发安全reference backend与Manifest/Seal history/current repository
storage/sqlite/    pure-Go SQLite schema/migration、WAL/FULL、Fact history/current/CAS与Journal
storage/rocksdb/   build-tag隔离的RocksDB 9.10窄C API ContentStore
sdk/               transport-neutral查询、Page/Cursor边界复验、Inspect、Plan Validate与Application治理写请求客户端
cli/               Continuity只读/治理写命令描述、strict JSON参数与结果映射；不注册根CLI
conformance/       Wave 1能力声明与越界检查
releasecandidate/   reference-only ComponentRelease builder与lost-reply exact publisher
fakes/             测试专用lost-reply fault wrapper，无生产能力
tests/             黑盒、故障注入与Conformance测试
```

## 验证

在本目录运行：

```bash
go test -count=1 -shuffle=on ./...
go test -count=100 ./contract ./domain ./fakes ./storage/memory ./tests/blackbox ./tests/fault ./tests/conformance
go test -race -count=20 ./domain ./fakes ./storage/memory ./tests/fault ./tests/conformance
go test -race ./...
go vet ./...
go test -tags continuity_rocksdb ./...
go test -race -tags continuity_rocksdb ./...
go vet -tags continuity_rocksdb ./...
```
