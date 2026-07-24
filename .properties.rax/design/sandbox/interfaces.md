# Sandbox v2 公共接口、治理矩阵与 SDK/CLI/API 边界

> Checkpoint V2 Runtime公共合同已落live代码：OperationScope V2、Evidence V1、Settlement V5、
> Participant Governance V2与Restore Governance V2均可编译且Runtime Gateway存在。Sandbox已完成
> phase Reservation/current Reader、Provider、DomainResult/Apply、Workspace Snapshot capture与
> fresh Instance Restore；Snapshot terminal purge仍等待外部Owner合同。

## 1. 公共与私有 Port

| Port/接口 | 可见性 | 用途 | Sandbox 规则 |
|---|---|---|---|
| Runtime Binding V2 / `DescriberV2` | 公共 | Manifest、Capability、Owner、Locality、Residual、Conformance | Sandbox runtime adapter 只发布已证明能力；不从 legacy Manifest 猜测 |
| Runtime Operation V3 + Enforcement 4.1 | 公共 | Admission、Permit、Begin、prepare/execute双阶段Enforcement、Observation/Evidence、Operation Settlement | Sandbox实现exact-current Reader与Rust dispatch adapter；Runtime持久化Receipt/Journal，Rust只回Observation/Receipt；DomainResultFact与ApplySettlement仍由Sandbox Owner持有 |
| Application `SandboxLifecyclePortV4` | 公共 | start-or-inspect lifecycle与lost-reply恢复 | Sandbox `applicationadapter`实现；Application Coordinator只见Plan exact ref、Runtime DomainResult ref与Settlement ref，不见backend handle/Provider Receipt |
| Runtime `RunSettlementParticipantPortV2` | 公共 | Cleanup/Residual/Workspace 等 Run Requirement 参与者 Inspect | Sandbox 只提交领域 participant fact；不选 Outcome |
| Runtime legacy `EnvironmentPort` | 公共但窄 | 旧 Allocate/Activate/Inspect/Fence/Release Observation | 只读兼容投影；不得作为 v2 Commit 接口 |
| Runtime `CheckpointParticipantPort` | 公共但不完整 | v1 Barrier 参与者骨架 | 只作为 Delta 输入；不私建授权补丁 |
| `CheckpointRestoreOperationScopeV2` | Runtime live公共合同 | Checkpoint/Restore typed OperationScope与Attempt坐标 | 只给坐标；不授Authority/Permit/执行权 |
| `CheckpointRestoreEvidenceScopeV1` + `CheckpointRestoreEvidenceGovernancePortV1` | Runtime live公共合同/Gateway | 各Phase独立Evidence qualification/handoff/consume/Inspect | Sandbox adapter已实现；不得复用OperationScope Evidence V3 |
| `OperationCheckpointRestoreSettlementSubmissionV5` + `OperationCheckpointRestoreSettlementGovernancePortV5` | Runtime live公共合同/Gateway | 各Phase独立Runtime Settlement提交/治理 | 不得复用Operation Settlement V4；Submission不替代Sandbox ApplySettlement |
| `CheckpointRestoreParticipantGovernancePortV2` | Runtime live公共合同 | Reserve/Inspect/DomainResult current/Apply Settlement | Sandbox实现已闭合；不得由caller mint Phase Fact |
| `RestoreGovernancePortV2` | Runtime live公共合同/Gateway | RestoreAttempt reservation与short-TTL Eligibility | 只到Attempt/Eligibility，不含执行授权；Sandbox另经双Enforcement执行Restore stage |
| Continuity Manifest/RestorePlan V2 | Continuity live公共Port | Manifest/RestorePlan Fact exact Inspect/CAS及Sandbox/Runtime refs | Continuity只拥有Manifest/RestorePlan Fact；不得推进Runtime Checkpoint权威或Sandbox领域Fact |
| `SnapshotArtifactOwnerPortV2` | Sandbox控制域公共Port | Reserve/Commit available与aggregate/entry/current exact Inspect已实现；terminal retention/purge仍外部门 | seal/CAS committer只存在Owner实现包内；不得暴露raw Apply/CAS、caller payload、backend handle；StorageArtifactRef/FactRef type+digest domain必须exact |
| Sandbox Controller Port v2 | 待评审的组件公共 Port | Requirement/Placement/Allocation/Workspace/Inspect/Cleanup 等领域 API | 由 Sandbox 合同拥有，Application 通过 Domain Adapter 调用；不得返回 Backend 原始句柄 |
| Sandbox Backend Provider Port v2 | 组件公共扩展 Port | Host/Container/MicroVM/WASM Adapter 与 Conformance | Provider 只能 Prepare/Execute/Inspect 并产 Observation/Receipt |
| `SandboxCheckpointProviderV2` / `SandboxRestoreProviderV2` | Sandbox私有backend-neutral SPI | Prepare/ExecutePrepared/InspectLocal | 已接线；只返回opaque provider attempt、Observation/Receipt，不拥有Snapshot Fact、Consistency、Eligibility或Settlement |
| Sandbox Fact Store Port v2 | 组件私有 Owner Port | Allocation/Activation/Inspection/Release/Workspace/Cleanup Fact CAS | 不是 Application/Harness API；实现必须持久化和 CAS |
| Sandbox SQLite State Plane v1 | 组件私有生产Driver | 生命周期Fact、Projection history/current、Settlement/DomainResult binding、Application plan/result、SnapshotArtifact history/current | 单节点WAL参考实现；不缓存或伪造Runtime/Retention/Continuity/Review current事实，不声明分布式SLA |
| Sandbox Enforcer Port v2 | Data Plane 私有 | 实际文件/网络/进程/Secret/Capability 门禁 | 只消费 Permit/LeaseRef/Policy current facts；不签发权限 |
| `SandboxCheckpointEnforcerV2` | Data Plane私有SPI | 每次Checkpoint/Restore Provider调用前复读全部exact current facts | 已接线；不生成Runtime Receipt或Domain Fact，stale Lease/Fence/Host水位时零Provider调用 |
| Sandbox `ExactCurrentStore` / Runtime Adapter | 组件私有只读 + 公共Port实现 | 用expected exact ref独立复读Attempt，再独立复读Reservation/Lease/Projection/Requirement/Policy/Placement/Backend/Slot/Generation并生成4.1 current projection | Attempt的ID/Revision/Digest/TTL、双向Reservation绑定及Lease绑定必须逐项一致；只有`runtimeadapter`可依赖Runtime public core/ports；零写、零Receipt、零Provider调用 |
| Harness `ContextPort`/`ModelTurnPort`/`EventCandidatePort` | Harness 私有 | Harness 内部物化、模型 Turn、Event journal | Sandbox 禁止实现、导入或当成 6+1 公共 Port |

“Sandbox Controller Port v2”是待联合评审的正式组件合同，不是为填补 Runtime 缺口而私建的兼容接口；其 Runtime/Application 映射必须通过结构化 Port Delta 和 Domain Adapter 评审后才可实现。

### 1.1 Rust Data Plane IPC v1

2026-07-17用户已确认Go Control Plane + Rust Data Plane独立进程。首批本地IPC冻结为
Unix-domain stream + 4-byte big-endian长度帧 + canonical JSON body，合同版本
`praxis.sandbox/data-plane-ipc/v1`。单帧上限4 MiB；socket目录权限`0700`、socket权限`0600`，
服务端使用`SO_PEERCRED`拒绝非配置UID。禁止FFI、stdin命令拼接、raw shell transport和未版本化JSON。

每个请求共同字段：

```text
contract_version, request_id, phase=prepare|execute|inspect|fence|release|cleanup
operation_digest, effect_id, intent_revision, intent_digest
attempt_id, tenant_id
provider_binding_exact_ref, sandbox_attempt_exact_ref
runtime_enforcement_phase_exact_ref
requested_not_after_unix_nano, payload_schema, payload_digest, payload_revision
```

Rust Enforcer在Provider `Prepare`与`ExecutePrepared`前分别通过只读current-reader IPC以exact坐标
复读Owner-sealed `OperationDispatchSandboxCurrentProjectionV4`和Runtime
`CurrentOperationDispatchEnforcementV4`。请求不能携`current=true`或caller快照作为授权；调用方附带
的projection只作expected digest，Reader返回值才是本次current事实。任一Read失败、typed-nil、
unknown字段、UID不符、digest/revision/TTL/phase/attempt/provider漂移或`now >= expires`均返回closed
错误且Provider调用为0。

响应只允许：

```text
accepted=false + closed category/reason
或 provider_attempt_ref + Observation/Receipt + checked/expires + residual hints
```

Rust不得返回`Runtime Receipt`、`DomainResultFact`、`Settlement`、`Lease active`、`cleanup complete`
权威结论。IPC回包丢失后Go侧只能用同一request/attempt exact identity调用Inspect；不能重新Prepare、
切换Provider或生成新Attempt。

### 1.2 Provider SPI v1

Rust包内统一trait：`Describe`、`Probe`、`Prepare`、`InspectPreparedLocal`、`ExecutePrepared`、
`InspectAttemptLocal`、`Fence`、`Release`、`InspectCleanup`。Container与WASM实现同一状态语义，
但payload DTO分型且禁止互填。Provider内部状态键固定为`tenant + operation digest + effect id +
attempt id + phase`；所有create操作幂等，same key换digest为Conflict。

- Container Prepare解析已批准的OCI spec、校验image digest、创建container/snapshot但不启动task；
  ExecutePrepared只启动已经Inspect一致的task。Fence发送kill并Inspect task退出；Release删除task/
  container/snapshot；Cleanup分别报告process、mount、network、secret、background、remote、provider七维。
- WASM Prepare按component digest编译/加载并验证WIT world与imports；默认零filesystem/network/process/
  environment capability。ExecutePrepared只实例化已准备component并调用批准export；fuel、epoch deadline、
  memory/table/instance限制全部由Owner Policy投影给出且只能缩减。Host capability调用仍走独立Gateway，
  不能由Linker闭包直接绕过Review/Effect治理。

## 2. 全部 Governed Effect Kind

所有 Effect Kind 使用 namespaced 候选名；最终注册由 Runtime/Harness/Assembler 联合评审决定。

| Effect kind 候选 | Operation scope | Run binding requirement | Conflict Domain | Review | Fence/Scope | Domain Result / Settlement |
|---|---|---|---|---|---|---|
| `praxis.sandbox/backend-discovery`（资产候选，live合同未实现） | 目标映射为`activation_attempt`，当前NO-GO | Run禁止伪造；没有live EffectKind/领域Fact/Provider入口 | 候选`praxis.sandbox/backend-discovery` | 仅保留未来风险设计，不授执行 | 命名映射YES不等于合同能力；当前零Provider调用 | 未来Backend Observation/Conformance Candidate；不得伪造为已实现Fact |
| `praxis.sandbox/allocate` | `activation_attempt` | Run 不要求；可绑定已批准的未来 RunPlanRef | `praxis.sandbox/environment-lifecycle` | 按资源、位置、数据域风险 | Runtime 预分配 LeaseRef；activation Fence；不得由 Sandbox发 Lease | `AllocationFactV2` Settlement |
| `praxis.sandbox/activate` | `activation_attempt` | Run 不要求 | `praxis.sandbox/environment-lifecycle` | 按 Secret/Network/Resource 风险 | 当前 LeaseRef、Policy、Fence epoch、Scope | `ActivationFactV2` Settlement |
| `praxis.sandbox/open` | `activation_attempt` | Run 不要求；不等于 Run start | `praxis.sandbox/environment-lifecycle` | 通常按 policy；暴露远程入口时 review | 当前 LeaseRef/Fence/Binding | `OpenFactV2` Settlement |
| `praxis.sandbox/checkpoint` | typed checkpoint scope | 精确当前Run；prepare/commit/abort各自独立Operation/Attempt | `praxis.sandbox/checkpoint` | Snapshot含敏感数据/远端保留时需要 | 当前Run/Lease/Fence、Barrier/Effect Cut；两次Enforcement | Participant/Snapshot/Evidence/Settlement/Apply |
| `praxis.sandbox/restore` | typed restore scope | fresh Instance目标；旧Run/Instance/Lease仅作历史Ref | `praxis.sandbox/checkpoint` | 基于restore policy重新Review | 新Instance/高epoch/新Lease/Fence与两次Enforcement | Restore Stage Fact/Evidence/Settlement/Apply |
| `praxis.sandbox/cancel` | run | 精确 active/stopping Run 必须存在 | `praxis.sandbox/execution-control` | policy_not_required 或治理策略 | 精确 Run/Attempt/Lease/Fence；只缩减当前执行 | `CancelFactV2`; 不 Close/Release |
| `praxis.sandbox/close` | termination | active Run 禁止；必须已 stopping/terminal 或不存在 | `praxis.sandbox/environment-lifecycle` | policy 决定 | 必须证明无 active Run 或 Run 已 stopping/terminal | `CloseFactV2`; 不隐式 Cancel/Cleanup |
| `praxis.sandbox/fence` | termination 或 admin | Run 可选；若存在必须精确绑定，Emergency 仍不扩大 Scope | `praxis.sandbox/environment-lifecycle` | Emergency Safety 可走专门 Runtime policy；普通 Fence 仍绑定 Review projection | Runtime 唯一拥有 Fence；Sandbox只执行当前 fence attempt | `FenceExecutionFactV2`; 不改 Fence epoch |
| `praxis.sandbox/release` | termination | active Run 禁止 | `praxis.sandbox/environment-lifecycle` | 按资源/远端残留策略 | 当前 LeaseRef/Fence；只释放 Provider 资源 | `ReleaseFactV2`; 不宣称 Cleanup complete |
| `praxis.sandbox/inspect` | 与原 Operation 对应；例行可 admin/run/termination | 继承原 Attempt 的 Run 绑定；例行 Inspect 可无 Run | 未知恢复沿用原 Conflict Domain；例行用 `praxis.sandbox/inspection` | 远端 Inspect 是独立 Effect；本地 State Plane inspect 可 policy_not_required | 精确 original effect/revision/attempt/provider ref | `InspectionFactV2`; 不能重派原 Effect |
| `praxis.sandbox/cleanup` | termination 或 admin recovery | Run 可作历史 Ref，不要求 current Run | 原 Effect Conflict Domain | fresh cleanup/compensation authority；不可沿用过期 Verdict | 当前 Runtime Lease/Fence/Scope 或显式 recovery policy | `CleanupFactV2`/Residual Settlement |
| `praxis.sandbox/workspace-commit` | run | 精确当前 Run 必须存在 | `praxis.sandbox/workspace-commit` | 绑定 exact ChangeSet；策略或人工 Review | 当前 Run/Lease/Policy/Fence/Base Revision | 新 Workspace Revision；ChangeSet CAS committed |

所有 Conflict Domain 使用 Runtime V2 要求的 tenant-stable scope digest；不能缩窄为 Run/Instance 以逃避跨 Restore 冲突。更细并发由 domain subject/idempotency 和 Owner CAS 控制。

OperationScope Evidence V3的lifecycle行只识别`activation_attempt + praxis.runtime/activation-evidence + praxis.sandbox/{allocate,activate,open}`；独立远端Inspect另有唯一窄行`activation_attempt + praxis.runtime/activation-inspection-evidence + praxis.sandbox/inspect`，不得将inspection profile用于原动作或其他Effect。Sandbox live合同没有rollback kind，且`praxis.sandbox/cancel`必须保持Run-scoped，禁止用cancel实现pre-run rollback；`praxis.sandbox/close`与`praxis.sandbox/release`继续属于termination scope，不得放入activation矩阵。

Checkpoint prepare/commit/abort与restore均不属于上述activation slice。live实现只使用typed
Checkpoint/Restore scope、专用Evidence/Settlement与双Enforcement；不得使用transport kind本身获得
授权，也不得冒用activation scope、Evidence V3或Settlement V4。

## 3. Effect/Review/Fence/Unknown 矩阵

| 动作组 | Review 漂移 | Fence/Lease 漂移 | Begin 后 Unknown | NotFound/进程死亡 | Residual/Cleanup |
|---|---|---|---|---|---|
| backend-discovery资产候选 / live allocate | 重审/重新 Admission，原 Attempt 不继续；候选discovery不授执行 | fail closed；Sandbox 不修补 Runtime Lease | allocate只Inspect原provider attempt；discovery当前无live attempt | 不自动重派；可经独立证据结算 not_applied | 资源可能已分配，进入 allocation residual |
| activate/open | 原 Permit 失效 | fail closed | Inspect 原 attempt；Environment 不 Ready | 不能推导未执行或可重开 | Secret/network/process 残留独立报告 |
| checkpoint/restore | Snapshot/Policy任一变化重审 | 旧Lease/Instance直接拒绝；缺任一current绑定即零Provider | unknown只Inspect/Reconcile原Attempt，未确认则indeterminate | failed/not_applied/unknown均不创建commit或abort | Snapshot/远端保留占conflict domain；Abort不暗示补偿或已清理 |
| cancel/close/fence | 普通路径重审；Emergency Safety 仅缩权 | Fence 由 Runtime current fact决定 | 只 Inspect；不重复控制命令 | 不能推导安全关闭或已 Fence | Close/Fence 后仍需 Cleanup |
| release/cleanup | fresh cleanup authority | 旧 Lease 只能作为历史 subject | cleanup indeterminate | 不等于 clean | 七维全确认前不 complete |
| workspace_commit | ChangeSet/Base/Scope变化重审 | fail closed | Inspect 原 commit 与目标 revision | 不重放 Diff | 部分文件/锁/临时对象显式 residual |

Inspect Effect 自身未知时，不允许递归无限生成 Inspect 链；进入 blocked indeterminate，等待外部权威事实变化或人工处置。

## 4. Run Settlement Requirements

Sandbox 对每个 Run 只声明由 Plan/Assembler 选择的精确 requirement，不自行修改 Run Plan：

| Requirement ID 候选 | Phase | Subject | Owner | 允许 disposition |
|---|---|---|---|---|
| `praxis.sandbox/execution-quiesced` | run_completion | Run+LeaseRef+Open/Cancel attempts；证明Run内执行已停、全部attempt已分类且禁止新dispatch | Sandbox Settlement Owner | confirmed_satisfied/failed/unknown/not_applied |
| `praxis.sandbox/workspace-settled` | run_completion | Run 内 ChangeSet 集合摘要 | Sandbox Workspace Settlement Owner | satisfied/failed/unknown/operation_not_required |
| `praxis.sandbox/checkpoint-participation` | run_completion | 本 Run 要求的 Barrier/Participant facts | Sandbox Checkpoint Participant Owner | satisfied/incomplete/confirmed_not_applied/indeterminate/not_required |
| `praxis.sandbox/environment-closed` | termination_report | LeaseRef+Close settlement | Sandbox Settlement Owner | satisfied/failed/unknown/not_applied |
| `praxis.sandbox/cleanup-settled` | termination_report | LeaseRef+七维 Cleanup subject | Sandbox Cleanup Owner | satisfied/failed/unknown |
| `praxis.sandbox/residual-accounted` | termination_report | Residual set + conflict domains | Sandbox Cleanup Owner | satisfied/failed/unknown/not_required |

`operation_not_required` 必须有精确 Runtime policy fact；“本次没有文件修改/Checkpoint”不能靠空回包猜测。

## 5. Runtime/Application/Harness 映射

```text
Assembler/CompiledGraph（当前公共阻塞）
  -> Binding V2 注册 Sandbox Controller/Enforcer/Provider capability
  -> Runtime 预分配 Instance/Lease/Fence 与 OperationSubjectV3
  -> Runtime Activation/Reconcile或Termination/Reconcile解析生命周期Adapter；Application只解析Run内业务Domain Adapter
  -> ReservePhase / Sandbox 原子 Domain Reservation
  -> Sandbox exact-current Reader（独立复读Reservation及全部current facts）
  -> Runtime Operation Admission
  -> Review/Auth current
  -> Runtime Permit
  -> Begin（不授执行）
  -> Sandbox exact-current Reader（prepare执行点；expected Attempt exact ref）
  -> Runtime prepare enforcement persisted（Receipt绑定同一Attempt exact ref）
  -> Evidence qualification/handoff（prepare）
  -> Rust actual-point复读current -> Provider Prepare
  -> Evidence record+consume（prepare）
  -> Sandbox exact-current Reader（execute执行点再次复读，Attempt revision/digest/TTL漂移即拒绝）
  -> Runtime execute enforcement persisted（继续绑定同一Attempt exact ref）
  -> Evidence qualification/handoff（execute）
  -> Rust actual-point复读current -> ExecutePrepared -> Provider Observation/Receipt
  -> Evidence record+consume（execute）
  -> 独立praxis.sandbox/inspect Operation/Effect/Attempt/Permit
  -> Inspect prepare/execute双Enforcement + inspection Evidence profile
  -> Rust Inspect original ProviderAttempt（不重放原动作）
  -> Inspect DomainResultFact -> Runtime Settlement -> Sandbox ApplySettlement
  -> 原动作Sandbox DomainResultFact
  -> Runtime Operation Settlement
  -> Sandbox ApplySettlement
  -> Sandbox CAS settled领域投影
  -> Run Settlement Participant Inspect
  -> Runtime 聚合 Outcome/Cleanup
```

Harness只消费已装配Endpoint/ExecutionScope/Run Plan；不会直接调用Sandbox Controller。CompleteRun只等待`execution-quiesced`等run_completion requirement，不等待Close。Close、Release、Cleanup在Runtime进入stopping/terminal后按termination_report继续收口。Harness的Session、Event、Claim保持自身Owner边界。

## 6. SDK 边界

### Controller SDK

面向可信宿主/Coordinator，暴露 transport-neutral 客户端：Describe、Match Requirement、Plan Placement Candidate、Reserve Domain Operation、Inspect Domain Fact、Watch、Prepare Workspace ChangeSet、Inspect Cleanup。Allocate/Activate/Open/... 的真正 Dispatch 必须交给 Application+Runtime Operation V3，不提供 `backend.ExecuteRaw`。

### Workspace File Driver

Sandbox Owner注入`WorkspaceView exact ref -> BaseRoot/OverlayRoot/BlobRoot`的本地opaque binding；
binding不进入公共DTO。Driver对Base与Overlay做两次树摘要复读，拒绝symlink/special file、隐藏路径、
只读scope漂移、base revision漂移和捕获期间host drift；只把write scope内差异转换为
`WorkspaceChangeSet(staged)`，blob采用content-addressed create-once保存。该Driver不调用Provider、
不产生Review/Settlement、不能把staged标为committed。真实commit仍必须作为独立
`praxis.sandbox/workspace-commit` Operation经过双Enforcement/Evidence/Inspect/Settlement/Apply。

### Provider SDK

面向 Backend 开发者，实现 Describe/Probe、Prepare、InspectPreparedLocal、ExecutePrepared、InspectAttemptLocal、domain Inspect、Cleanup Inspect、Checkpoint capability 和 Conformance suite。SDK 不暴露 Sandbox/Runtime Fact Store CAS，也不能签发 Permit/Lease/Fence。

## 7. CLI 边界

候选命令：

```text
praxis sandbox backends
praxis sandbox inspect
praxis sandbox fence
praxis sandbox diff
praxis sandbox commit
praxis sandbox residuals
praxis sandbox cleanup
```

- CLI 只能调用 Controller SDK/API 并提交意图；不能直连 Provider socket、container daemon 或 VM monitor。
- `fence/commit/cleanup` 默认显示候选和治理状态；是否进入 dispatch 由 Runtime/Application 处理。
- 默认输出隐藏 Secret、Credential、宿主绝对路径和 Provider 原始句柄。

## 8. API 边界

- 异步 Operation、稳定 idempotency key、ExpectedRevision CAS、exact Inspect、Watch cursor、取消、Lease TTL、Checkpoint Barrier、ArtifactRef、长 Cleanup；
- transport 未选型；HTTP/gRPC/本地 socket 都不是本设计冻结项；
- API 错误按稳定 category/reason，不返回 Secret/raw provider payload；
- Watch 只观察领域 Fact，不授予权限；
- API 不能把多个独立 Effect 折叠成“一键 provision-and-run”原子假象。

## 9. Checkpoint/Restore SPI调用边界

```text
ReservePhase（create-once Sandbox phase Reservation）
  -> InspectCurrent（独立复读Reservation、PreviousPhase closure及全部Owner current facts）
  -> Runtime Admission
  -> Review/Auth current
  -> Permit current
  -> Begin（只持久化dispatch前事实，不授执行）
  -> InspectCurrent（prepare执行点再次复读）
  -> Runtime持久化phase-specific prepare Enforcement exact ref
  -> Provider Prepare
  -> InspectCurrent（execute执行点再次复读）
  -> Runtime持久化phase-specific execute Enforcement exact ref
  -> Provider ExecutePrepared
  -> Provider Observation/Receipt
  -> formal Evidence
  -> Sandbox independent Inspect
  -> Sandbox DomainResultFact CAS
  -> Runtime Settlement exact ref
  -> Sandbox ApplySettlement CAS
```

- 上述顺序不可交换或折叠；Reservation、InspectCurrent、Admission、Review/Auth、Permit、Begin、对应阶段Enforcement任一缺失/过期/漂移，Provider调用计数必须为0。Begin本身不授Provider Prepare或Execute权。
- Provider Prepare不授Execute权；PreparedAttempt ref不能代替execute阶段Permit/Enforcement。
- Enforcer请求必须携带exact Operation/Effect/Attempt、Authority/Review/Budget/Scope、Runtime Lease/Fence、Policy/Workspace、Snapshot/Compatibility、Generation/Provider Binding/Credential及expected TTL水位。
- Provider/Enforcer回包不含`consistent=true`、`restore_eligible=true`、Runtime Lease迁移或Sandbox settled状态。
- 与Runtime P14最终裁决一致，只有ApplySettlement为`prepared`并发布exact `PreviousPhaseClosureRef(State=prepared)`的prepare可Reserve后继；后继严格为`commit XOR abort`。`failed`直接参与`incomplete`，`not_applied`直接参与`confirmed_not_applied`，二者均不得创建后继。
- `unknown`只Inspect/Reconcile原prepare Attempt；确认成prepared后才可进入分支守卫，确认成failed/not_applied分别进入上述终态，始终无法确认则收口为`indeterminate`。unknown期间不得创建commit或abort。
- `commit`与`abort`对同一closure互斥，任一分支一旦create-once或已推进，另一分支永久拒绝。每个后继Reservation必须绑定`PreviousPhaseRef + PreviousPhaseClosureRef(ID/Revision/Digest/State=prepared/TTL)`并将其纳入canonical digest。
- lost reply、timeout、NotFound、进程或宿主死亡均进入原Attempt Inspect；若同一prepared closure下后继已创建或已推进，恢复必须返回并Inspect该精确后继，不能重开prepare、换分支、换Provider/Attempt/Snapshot ID或自动重派。Revision/Digest/closure单调且终态不可复活，禁止ABA。
- Shape/Reader输入中的可选引用必须使用显式discriminator；接口值中携带typed-nil pointer、nil/empty混淆或unknown required variant必须拒绝，不能被当作“字段不存在”。`now >= ExpiresUnixNano`即不可用于新Admission/Enforcement；后继TTL不得晚于PreviousPhase closure及全部current上游的最早到期时间，过期历史终态只允许结构/审计Inspect。
- Provider/Enforcer SPI不选择具体产品、transport、进程拓扑或SLA。Sandbox Owner metadata已冻结
  SQLite单节点参考Driver，但公共Port保持backend-neutral；后续分布式Driver必须通过同一CAS、
  append-only、lost-reply与no-ABA Conformance，不能改变Owner合同。

当前Checkpoint Provider SPI、Restore current Reader与完整阶段闭包均已实现；任何缺门、过期、
drift或Unknown恢复仍保持零新Provider调用。

P14反例：prepare Settlement为failed/not_applied后Reserve abort；prepare unknown时以“只缩权”为由抢占abort branch guard；将unknown本地映射为not_applied；或在Inspect/Reconcile仍无权威结论时创建任一后继。以上全部fail closed、零后继Reservation、零Provider调用。
