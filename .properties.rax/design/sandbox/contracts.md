# Sandbox v2 对象与版本合同

## 1. 通用版本纪律

所有 Sandbox 正式对象采用以下共同规则：

- `contract_version`：固定合同族和 major 版本；v2 候选统一以 `praxis.sandbox/*/v2` 命名。
- `id`：Owner 分区内稳定标识；不得把 Provider 原始句柄作为全局 ID。
- `revision`：Owner CAS 版本，从 1 开始严格递增。
- `digest`：对严格、规范化、带 domain/version/discriminator 分隔的 JSON 计算；有序集合必须排序且唯一。
- `created_unix_nano`、`updated_unix_nano`、`expires_unix_nano`：使用注入时钟；TTL 不能被回包延长。
- `RefV2`：至少含 ID、Revision、Digest；引用不能只含可复用名称。
- `ExpectedRevision`：所有可变事实写入必须携带；CAS 回包丢失后按精确 ID Inspect。
- `SourceRegistrationID + SourceEpoch + SourceSequence`：所有 Provider/Enforcer Observation 必须来源有序；同序同摘要幂等，同序换内容冲突。
- `OpaqueExtension`：前向扩展只能使用注册 Schema、显式 required 标志和大小上限；严格解码拒绝未知字段、重复键和尾随文档。
- Secret 只允许 `SecretRef`/Credential Lease 引用，禁止明文进入合同、日志、Digest 输入回显或 CLI 输出。

## 2. 核心对象

| 对象 | 语义与关键字段 | Owner/权威边界 |
|---|---|---|
| `ExecutionRequirementV2` | OS/arch/toolchain、文件/网络/进程/Secret/资源、持久化、IsolationProfile、Checkpoint、RiskClass、允许后端与显式降级、版本/TTL | Assembler 产生不可变需求；不是权限或 Placement |
| `SandboxPolicyV2` | RequirementRef、Identity/Authority/Scope/CapabilityGrant 当前绑定、组织策略、Review/Budget 策略引用、文件/网络/进程/Secret 边界、PolicyDigest/TTL | Sandbox 编译并拥有 Policy Fact；不拥有源 Authority/Review/Budget |
| `BackendDescriptorV2` | BackendKind、Locality、Artifact/Contract、可强制能力、Inspect/Fence/Cleanup/Checkpoint、RawBypass、ResidualClass、Conformance、Evidence/TTL | Provider 自报是 Candidate；Sandbox Probe/Inspect 后才形成 admitted descriptor fact |
| `PlacementCandidateV2` | RequirementRef、PolicyRef、BackendDescriptorRef、SlotCandidateRef、MatchEvidence、Cost/Resource Observation、降级项 | Scheduler/Provider 可生产 Candidate；不授予执行权 |
| `AllocationFactV2` | CandidateDigest、Requirement/Policy/Backend/Slot 精确引用、选择或拒绝原因、AllowedDowngrade、AdmissionEvidence、RuntimeLeaseRef、Settlement | Sandbox allocation 领域 Fact；不签发或推进 Lease，不授予执行权 |
| `RuntimeSandboxLeaseBindingV2` | Runtime LeaseRef、InstanceRef/Epoch、LeaseID/Epoch、FenceEpoch、ScopeDigest、ObservedRevision/Expiry | Runtime 唯一权威对象的只读绑定投影；Sandbox 只能校验和引用 |
| `ActivationFactV2` / `OpenFactV2` | 原 Operation/Attempt、Allocation/Policy/Backend/Workspace、RuntimeLeaseRef、Inspection provenance、领域状态与 Settlement | Sandbox 对 activate/open 的领域 Fact；不能推进 Runtime Lease/Run |
| `CancelFactV2` / `CloseFactV2` | 精确 Run/Attempt、RuntimeLeaseRef、停止接受执行或取消的 Inspect provenance、领域状态与 Settlement | Sandbox 只证明实际执行面结果；Cancel 与 Close 不得互相替代 |
| `ReleaseFactV2` / `FenceExecutionFactV2` / `CleanupFactV2` | 原独立 Effect、Provider/Enforcer Observation、Inspect、Residual、Settlement | Sandbox 对实际 release/fence/cleanup 的领域 Fact；Runtime 独立推进 Lease/Fence |
| `WorkspaceViewV2` | BaseArtifactRevision、只读挂载、写 Overlay、Temp、Secret mount ref、隐藏路径、FileScopeDigest、WorkspacePolicyRef | Sandbox 拥有可见性投影；不等于真实文件系统事实 |
| `WorkspaceChangeSetV2` | ViewRef、BaseRevision、DiffRef/Digest、变更清单、ArtifactRevision、Operation/Run、Review/Effect/Settlement、CommitState | Sandbox 拥有 staging/commit 领域事实；真实 Workspace 由对应 Effect Owner 落地 |
| `EffectEnforcementRequestV2` | SandboxOperationRef、LeaseRef、OperationSubjectDigest、Permit/Intent/Fence 摘要、ActionScope、Policy/Workspace 当前水位 | Data Plane 第二门禁输入；不是新的 Runtime Permit |
| `ProviderObservationV2` | PreparedAttempt、ProviderOperationRef、ProviderState、PayloadDigest、Source 坐标、ReceiptRef、时间 | 仅 Provider 声明；不能推进 Runtime Lease、Sandbox Fact、ChangeSet 或 Cleanup |
| `ExecutionReceiptV2` | exact Permit/Attempt/Lease/Policy/Fence/Enforcer、ValidatedAt、Attestation/Evidence candidate | 实际执行点的验证回执；仍不是领域 Settlement |
| `SandboxInspectFactV2` | InspectKind、覆盖维度、Lease/Operation/Provider 精确坐标、ObservedState、Residual、Evidence、TTL | Sandbox Owner 独立 Inspect 结果；CAS 前不能成为当前事实 |
| `SandboxDomainResultFactV2` | OriginalOperation/Attempt、Disposition、Observation/Inspect provenance、DomainResult、Evidence、Owner | Sandbox对领域结果的权威CAS；Runtime Operation Settlement Ref由后续ApplySettlement精确关联，名称不得冒充Runtime Settlement |
| `CleanupReportV2` | Process/FileMount/Network/Secret/BackgroundTask/RemoteContinuation/ProviderRetention 七维状态、覆盖率、Evidence | Sandbox Cleanup Owner；全维 confirmed clean 才 complete |
| `ResidualReportV2` | ResidualKind、Scope、ConflictDomain、Owner、Inspectable/Compensatable、Evidence、ResolutionState | Sandbox 记录；未解决时占用 tenant-stable conflict domain |
| `CheckpointCompatibilityV2` | Barrier/Checkpoint、Lease/Policy/Workspace、Backend、Snapshot schema、effect watermark、restore constraints、Residual refs | Sandbox参与者事实；Runtime独立拥有Checkpoint资格、consistent Fact与restore eligibility；Continuity只拥有Manifest/RestorePlan Fact及其Inspect/CAS |
| `CheckpointParticipantFactV2` / `WorkspaceRestoreStageFactV1` | Barrier/Checkpoint/Participant、原 Attempt、Snapshot/Workspace refs、Compatibility、Inspect/Residual/Settlement；Restore目标绑定新 Instance/epoch/LeaseRef | live提交Sandbox领域事实，但不裁定 consistent 或创建新 Runtime Instance/Lease |
| `SnapshotArtifactReservationV2` / `SnapshotArtifactFactV2` / `SnapshotArtifactAggregateCurrentProjectionV2` | 四层canonical；StorageArtifactRef/FactRef独立exact type/digest domain；reserved→available live；terminal HoldIndex/Carry/CurrentIndex/Tombstone仍为外部Delta | Capture Owner已实现；外部Retention/Runtime purge/Management terminal仍待，Controller/Provider/Enforcer不得获得backend handle或raw CAS |
| `WorkspaceCommitFactV2` | exact ChangeSet/Base/Target、原 commit Attempt、真实 Workspace 新 Revision Inspect、Settlement | Sandbox 证明 ChangeSet 领域提交结果；真实 Workspace 最终事实仍由对应 Effect Owner |
| `BackendConformanceReportV2` | Backend/Artifact、Capability、正反例结果、Fence/Inspect/Cleanup/Unknown 证据、有效期 | Sandbox Conformance Owner；测试成功不自动成为生产认证 |

## 3. ExecutionRequirementV2

最低字段组：

```text
Identity: ID, Revision, Digest, ContractVersion
Platform: OS family/version range, architecture, toolchain/artifact refs
Workspace: read scopes, write scopes, overlay mode, persistence, artifact refs
Network: deny_all | allow_list；目标与协议范围摘要
Process: executable/tool scope, subprocess, syscall/device requirements
Secret: SecretRef classes, injection mode, maximum TTL；无明文
Resources: CPU/memory/storage/PID/time bounds；均为有限值或明确 not-required
Isolation: required properties + minimum conformance + prohibited residuals
Checkpoint: unsupported | metadata_only | workspace_snapshot | environment_snapshot
Risk: namespaced risk class
Backend: allowed surface classes + ordered explicit downgrade policy
```

Requirement 只描述“需要什么”。它不携带已批准 Review Verdict、当前 Budget 或 Provider Permit。

## 4. SandboxPolicyV2

Policy 编译必须绑定：

- RequirementRef 与完整 RequirementDigest；
- Identity、Lineage/Plan、Instance、适用 Run/Operation Scope；
- AuthorityRef/Epoch、CurrentScopeRef、CapabilityGrantDigest；
- ReviewPolicyRef、BudgetPolicyRef，而非未来具体 Verdict；
- FileScope、NetworkScope、ProcessScope、SecretScope、ResourceScope；
- IsolationProfile、MinimumConformance、AllowedResiduals；
- LeaseTTL/renewal bounds、FencePolicy、OfflinePolicy；
- Policy Revision/Digest/TTL。

具体高风险 Operation 仍必须绑定 Runtime `OperationEffectIntentV3` 的当前 Review、Budget、Authority、Policy 与 Permit；SandboxPolicy 不能替代 Runtime Governance。

## 5. BackendDescriptorV2

`BackendKind` 冻结为：

- `host_workspace`：受控宿主执行与真实项目 Workspace；
- `container`：共享或强化内核边界的 Linux 工作环境；
- `microvm`：独立内核级执行面；
- `wasm_capability`：能力级 Tool/Skill/Policy/Data Transform 运行时。

`Locality` 独立为 `host`、`instance_data_plane`、`remote_provider`。Remote 不是第五种隔离保证。

Descriptor 必须逐项声明 `enforced | observed_only | unsupported`：文件、网络、进程、Secret、资源、设备/syscall、Fence、Prepared Attempt local inspect、Provider operation inspect、Cleanup、Checkpoint、Workspace overlay、Residual。无法证明的能力不得对外暴露。

## 6. Runtime Lease 绑定与 Sandbox 领域 Fact

Runtime 唯一拥有 Tenant、Identity/Instance epoch、LeaseID/LeaseEpoch、FenceEpoch、Lease phase/revision/expiry。Sandbox 只保存 `RuntimeSandboxLeaseBindingV2` 的精确只读引用，并在每次 Admission、实际执行点和 CAS 前校验当前性：

- Sandbox Fact 必须绑定 Runtime LeaseRef、InstanceRef/Epoch、LeaseID/Epoch、FenceEpoch、ScopeDigest；
- Allocation/Activation/Open/Inspect/Release/FenceExecution/Cleanup 各有独立 Operation、Attempt、Observation、Inspect 和 Settlement；
- Sandbox Fact 的 revision/CAS 只推进该领域事实，不得顺带推进 Runtime Lease/Fence/Instance；
- Runtime Lease 变化后，旧绑定上的 Provider 回包只能归档为迟到 Observation，不能提交当前 Fact；
- Provider slot 使用不透明 Ref，不暴露给 Harness/普通 SDK；
- 同一 Instance 的 Lease 独占由 Runtime 线性化；Sandbox 仍以未终结 Allocation/Residual 的 conflict domain 防止资源槽位被错误复用。

## 7. WorkspaceChangeSetV2

ChangeSet 必须绑定：

- WorkspaceViewRef 与 BaseArtifactRevision；
- canonical path set、add/modify/delete/rename、mode/symlink/submodule 变化；
- Diff/Blob ArtifactRef 与摘要，禁止把大文件内联到治理对象；
- AgentInstance、适用 Run/Operation、Lease/Policy Revision；
- review candidate/verdict/condition、governed effect/attempt/settlement；
- base drift 与 merge conflict 结果；
- commit 后独立 Inspect 得到的真实 Artifact Revision。

Overlay 中存在文件不等于 ChangeSet 已提交；Provider 表示“write success”也不等于真实 Workspace 当前版本已改变。

## 8. Cleanup 与 Residual

Cleanup 的七个独立维度不得折叠为一个 Provider boolean：

1. 进程/子进程；
2. 文件挂载、Overlay、临时目录；
3. 网络连接、代理令牌、端口；
4. Secret 注入路径与 Credential Lease；
5. 后台任务/定时器；
6. 远端继续执行；
7. Provider 保留数据/快照。

任一维度已确认残留为 `residual`；覆盖不足为 `indeterminate`。只有全部 required 维度 confirmed clean、无 unresolved residual，槽位才可复用。

## 9. 动作 Fact 的共同字段

`Allocation/Activation/Open/CheckpointParticipant/Restore/Cancel/Close/FenceExecution/Release/Inspection/Cleanup/WorkspaceCommit` Fact 均必须包含：

- 自身 `contract_version/id/revision/digest/created/updated/expiry`；
- 精确`OperationSubjectV3`/Intent/Attempt/Permit/Delegation/Runtime Operation Settlement refs与摘要；
- Runtime Instance/Lease/Fence/Scope 的只读值绑定；
- Requirement/Policy/Backend/Workspace/Checkpoint 适用对象的 Ref+Revision+Digest；
- Provider/Enforcer source ordering、Observation/Receipt、independent Inspect provenance；
- `Disposition = confirmed_applied | confirmed_not_applied | failed | unknown | residual`；
- ExpectedRevision CAS、Evidence candidate/ref、Residual/Cleanup refs。

各动作可增加自己的结果字段，但不能用同一个“lifecycle state”对象把多个 Effect 合并提交。

## 10. Schema 与兼容

- major 不兼容：对象语义、Owner、状态含义或必填字段变化；通过新 Schema/Contract major 迁移。
- minor/additive：仅允许 registered optional extension；required extension 未识别时 fail closed。
- 旧 `EnvironmentPort` Observation 可投影成只读 legacy view，但不得反向构造 v2 Fact。
- 迁移时先双读/比对，后由 Sandbox Owner CAS 切换自身领域 Fact；Runtime Lease 只能由 Runtime Owner 迁移。禁止以 Provider 现状扫描直接生成或推进 `active` Lease。
- v1/legacy 没有 Policy/Placement/Revision/Digest/Settlement 的记录只能进入 `contained_observe_only` 或显式迁移队列，不能获得生产 Dispatch 资格。

## 11. Checkpoint/Restore V2 live字段组

Sandbox Owner侧Checkpoint participant phase Reservation、exact-current Reader、执行Port、
Evidence/Settlement、Provider SPI与Restore运行能力已经落地；下列字段是live兼容边界。

`SandboxCheckpointPhaseReservationV2`最低字段：

```text
Identity: ContractVersion, ID, Revision, Digest, Created/Updated/Expires
Phase: prepare | commit | abort
RuntimeCheckpoint: CheckpointAttemptRef, BarrierRef, EffectCutRef
SandboxParticipant: ParticipantRef, ExpectedParticipantRevision
PreviousPhase: explicit absent discriminator for prepare; exact PreviousPhaseRef + PreviousPhaseClosureRef(ID/Revision/Digest/State=prepared/ExpiresUnixNano) for commit|abort
OperationCoordinate: stable Operation/Effect key and expected Runtime Attempt exact ref when the Runtime Owner has published it
Runtime: Instance ID/Epoch, SandboxLease ID/Epoch, FenceEpoch, RuntimeLeaseBindingRef
Domain: RequirementRef, PolicyRef, WorkspaceView/ChangeSet refs, Backend/Placement/Slot/Generation refs
Watermarks: source-ordered Effect watermarks
CAS: ExpectedRevision, create-once phase key, PreviousPhase closure key
```

`SandboxCheckpointParticipantCurrentReaderV2` live请求/投影：

```text
Request: Tenant/Participant/CheckpointAttempt/Phase exact coordinates,
         ReadStage(pre_admission | pre_prepare | pre_execute),
         ExpectedReservationRef, ExpectedPreviousPhaseClosureRef,
         ExpectedOperation/Attempt refs（按读取阶段显式presence）
Projection: exact ReservationRef, Phase, PreviousPhaseClosureRef,
            current CheckpointAttempt/Barrier/EffectCut refs,
            current Operation/Attempt/Authority/Review/Budget/Scope/Permit refs,
            current Instance/Lease/Fence/RuntimeLeaseBinding refs,
            current Requirement/Policy/Workspace/Backend/Placement/Slot/Generation refs,
            ProjectionRevision, ProjectionDigest, ExpiresUnixNano, OwnerComputedCurrent
```

Reader请求只能给exact坐标与expected refs，不接受caller提供的事实快照或`current=true`。Reader先复读Reservation与PreviousPhase closure，再逐Owner复读全部current事实；commit/abort的PreviousPhase closure必须精确证明`prepare.apply_settled(prepared)`，failed/not_applied/unknown/indeterminate closure一律返回零projection。输出TTL取Reservation、PreviousPhase closure以及全部上游事实的最早到期时间。Projection只证明“可继续进入下一治理门的current输入一致”，不授Admission、Review/Auth、Permit、Begin、Enforcement或Provider执行权。

`ReadStage`不得由caller用来声称门禁已通过，它只选择Owner必须复读的字段集合：`pre_admission`要求Reservation/PreviousPhase/Checkpoint/Lease/Fence/Domain refs且Admission/Review/Permit必须显式absent；`pre_prepare`额外要求Admission、Review/Auth、Permit、Begin与Attempt exact refs；`pre_execute`再要求prepare Enforcement与PreparedAttempt exact refs。阶段所需字段缺失、提前出现、过期或漂移均返回零current projection。

`CheckpointCompatibilityV2`最低字段：source Checkpoint/Snapshot exact refs、old Instance/Lease历史refs、target Requirement/Policy/Workspace/Backend/Generation、Snapshot schema/platform/architecture、required capability、Effect watermarks、host artifact/contract/conformance水位、prohibited residuals、restore constraints、revision/digest/TTL。Compatibility只是Sandbox参与者Fact；Runtime独立提交Checkpoint资格、consistent Fact和restore eligibility，Continuity只对Manifest/RestorePlan Fact执行Inspect/CAS。

Checkpoint prepare/commit/abort执行链分别引用独立Operation/Attempt/Permit、prepare
Enforcement、execute Enforcement、专用Evidence、DomainResult、Runtime Settlement与Sandbox
ApplySettlement坐标，不得引用Evidence V3或Settlement V4。Reader返回current仍只进入下一治理门。

`WorkspaceRestoreStageRequestV1`绑定consistent Checkpoint/RestoreEligibility/SnapshotArtifact/
Compatibility、新Instance/epoch/Lease/Fence及独立RestoreAttempt。`RestoreGovernancePortV2`与
transport kind本身均不授权；旧Instance/Lease只能作为source provenance。

`SnapshotArtifactFactV2`最低字段：ReservationFactRef、stable ArtifactSubjectRef、`SnapshotStorageArtifactRefV2`、content/schema/length、tenant/data domain、encryption/residency refs、Provider Observation/Receipt、formal Evidence、Sandbox `SnapshotArtifactOwnerV2` Inspect provenance、state、RequestedNotAfter与Fact TTL。Storage exact DTO固定type URL/version/revision/digest algorithm/domain及canonical body，FactRef使用独立type/digest domain；二者不得互换。PayloadBodyDigest排除自身Ref/Digest，FactRef形成后才能进入Entry/Envelope。首切片公共Port只有Reserve/Inspect；包内seal/CAS committer不可导出。No-hold Projection的watermark generation必须等于HoldIndex exact ref generation；S1→S2按generation/sequence词典序，跨代需连续coverage carry exact refs。CurrentIndex/Tombstone exact DTO canonical覆盖全部presence、present exact ref TTL与自身TTL，并排除own Ref/Digest；terminal不可回退。未来执行投影仍需完整exact refs/readers和S1/S2复读。

所有Checkpoint/Restore/Snapshot对象必须分离`ValidateShape`与`ValidateCurrent(now)`：Restore postponed shape当前只有前者；过期历史终态仍可Inspect和恢复审计，但不得参与新的Phase、Restore或Provider调用。canonical digest必须覆盖Phase、PreviousPhaseRef、PreviousPhaseClosureRef、分支选择、exact refs、TTL、watermarks、Compatibility constraints与Residual集合；nil/empty、排序、重复键和unknown required extension规则沿用本合同第1节。

- Runtime P14一致性：prepare Reservation必须显式声明PreviousPhase缺失；只有ApplySettlement结果为prepared的closure能作为commit/abort PreviousPhase。commit/abort携带同一prepared closure并以create-once key互斥，任一后继存在或已推进即拒绝兄弟分支。
- prepare `failed`不创建后继并直接形成Participant `incomplete`输入；`not_applied`不创建后继并直接形成`confirmed_not_applied`输入。二者的terminal closure不能被Reader投影为successor-capable。
- prepare `unknown`不创建后继，只能Inspect/Reconcile原Operation/Attempt。确认prepared后才可重新读取prepared closure；确认failed/not_applied进入对应终态；持续无权威结论最终为`indeterminate`。
- lost reply后按PreviousPhase closure key Inspect；已推进后继必须原样返回，不得回退prepare revision、替换digest或以新Reservation制造ABA。
- DTO/扩展接口中携带typed-nil pointer、存在位与值矛盾、nil/empty混淆均为shape错误，不能当成可选字段缺失后继续。
- `now >= ExpiresUnixNano`即不current；后继expiry必须小于等于Reservation、PreviousPhase closure及所有治理/Lease/Fence/Policy/Workspace/Backend上游TTL的最小值。等于最早上游边界可构造，但到达该边界立即失效；任何更晚值拒绝且零写、零Provider调用。
