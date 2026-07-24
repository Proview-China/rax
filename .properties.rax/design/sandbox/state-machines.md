# Sandbox v2 状态机与失败恢复

## 1. Owner 修正

Runtime 唯一拥有 `SandboxLease`、Fence 和 Instance epoch。Sandbox 不定义或提交第二套 Lease 状态机；它只维护绑定 Runtime LeaseRef 的领域 Fact：Allocation、Activation、Open、Checkpoint Participant、Restore、Cancel、Close、Fence Execution、Release、Inspection、Cleanup、Workspace ChangeSet 及各自 Settlement。

Runtime Lease/Fence的任何迁移必须由Runtime Owner根据Sandbox DomainResultFact、Runtime Operation Settlement、Sandbox ApplySettlement和其他当前事实完成。Sandbox Provider的`active/fenced/released`字符串永远不是Runtime Lease Fact。

## 2. 通用 Governed Operation 状态机

每个外部动作都是独立 Operation/Effect，不允许合并：

```text
domain_candidate
  -> domain_reserved       （Application 先持久化精确领域 Reservation）
  -> owner_current_read    （Owner独立复读Reservation与全部current事实）
  -> effect_admitted       （Runtime Operation Intent Admission）
  -> review_authorized     （Review Verdict + Authority current）
  -> permit_issued
  -> dispatch_begun        （Begin只写前置事实，不授Provider执行权）
  -> prepare_current_read  （以expected Attempt exact ref独立复读Attempt及全部current facts）
  -> prepare_enforcement_persisted
  -> provider_prepared
  -> execute_current_read  （再次独立复读Attempt及全部current facts）
  -> execute_enforcement_persisted
  -> execute_prepared
  -> provider_observed -----> domain_inspected -> domain_result_fact
                                            -> runtime_operation_settlement_ref
                                            -> domain_apply_settlement
              |
              `-> unknown -> inspect_original_attempt_only
                                |-> domain_inspected -> domain_settled
                                `-> blocked_indeterminate
```

### 不变量

- Domain Reservation、Owner current Inspect、Admission、Review/Auth、Permit、Begin必须严格有序且早于Provider contact；缺任一项时Provider调用为0。
- Provider Prepare 前必须先持久化prepare Enforcement Receipt；ExecutePrepared前必须再次复读current facts并持久化独立execute Enforcement Receipt，prepare ref不能冒充execute ref。
- Sandbox exact-current Reader只生成Runtime 4.1 current projection：请求携带Attempt的exact `ID/Revision/Digest/ExpiresUnixNano`；Reader先独立复读Attempt Fact，再复读Reservation/Lease/Projection/Requirement/Policy/Placement/Backend/Slot/Generation。Attempt ref纳入projection digest与最短TTL；任一Attempt revision/digest/TTL漂移均在Provider前fail closed。
- Reader不写Runtime Receipt、不调用Provider、不推进Runtime Lease/Fence/Outcome；prepare/execute Enforcement Receipt与Journal继续由Runtime Owner持久化并绑定同一Attempt exact ref。
- pre-Run allocate/activate/open的Provider Observation使用已落地的OperationScope-aware
  Evidence V3；缺匹配profile时真实Provider路径保持零调用。
- `Begin` 之后丢回包只 Inspect 原 Attempt；Provider `NotFound`、进程死亡或 timeout 不恢复自动重派权。
- 只有独立 Inspect 能证明 `confirmed_not_applied` 后，原 Attempt 才可结算；是否新建新 Intent/Attempt 由上层显式策略重新 Admission，不能在恢复函数中自动发生。Checkpoint prepare另受第6.1节P14约束：failed/not_applied均无Phase后继，不能借此通用规则创建commit/abort。
- Remote Inspect 是新 Operation Effect，必须用 relation 绑定原 Effect；本地 InspectPrepared/InspectLocalAttempt 不联系远端。
- Observation/Evidence、Inspection、DomainResultFact、Runtime Operation Settlement、Domain ApplySettlement、Runtime Lease/Fence transition彼此分离。

## 3. 独立 Effect 状态与禁止偷步

| Effect | Sandbox 领域 Fact | 成功后可提交 | 明确禁止 |
|---|---|---|---|
| `allocate` | `AllocationFactV2` | 已分配执行槽、Backend/Workspace 绑定的领域结果 | 直接创建/激活 Runtime Lease；顺带 activate/open |
| `activate` | `ActivationFactV2` | Provider 环境已按 Policy 激活的 Settlement | 顺带 open Harness/Run |
| `open` | `OpenFactV2` | 可供已治理执行入口连接的领域结果 | 推导 Run running；持有 Harness Session |
| `checkpoint` | `CheckpointParticipantFactV2`、Snapshot/Coverage facts | 已结算的Participant与Artifact | 把Reader current当执行权或宣称全局 consistent |
| `restore` | `WorkspaceRestoreStageFactV1` | fresh Instance workspace stage结果 | 复用旧 Instance/Lease或宣称外部世界回滚 |
| `cancel` | `CancelFactV2` | 精确 Run/Operation 的取消执行结果 | 释放 Lease、Close 环境 |
| `close` | `CloseFactV2` | 已停止接受新执行/连接的结果 | 顺带 Cancel active Run；顺带 Cleanup complete |
| `fence` | `FenceExecutionFactV2` | Provider/Enforcer 的实际隔离 Settlement | Sandbox 修改 Runtime Fence epoch；扩大权限 |
| `release` | `ReleaseFactV2` | Provider 释放资源的领域结果 | 宣称 Cleanup complete 或槽位可复用 |
| `inspect` | `InspectionFactV2` | 精确 Operation/Provider Ref 的观察覆盖 | 自动重派原 Effect |
| `cleanup` | `CleanupFactV2` | 七维 Cleanup/Residual Settlement | 根据进程死亡推导 complete |
| `workspace_commit` | `WorkspaceCommitFactV2` | 真实 Workspace 新 Revision 的独立 Inspect 结果 | 将 Overlay 存在当作已提交 |

`close` 前置条件必须由 Coordinator/Runtime 证明目标 Run 已停止或无 active Run；Sandbox 不通过 Close 隐式 Cancel。

## 4. Allocation 领域状态

```text
candidate -> reserved -> prepared -> observed -> inspected -> settled
                           |           |
                           +-----------+-> unknown -> inspect_only -> settled|indeterminate
candidate -> rejected | expired
```

- AllocationFact 绑定 Runtime 预分配的 SandboxLeaseRef，但 Sandbox 不拥有该 Lease。
- 同一 AgentInstance 的单活 Lease约束由 Runtime Owner 线性化；Sandbox 还必须拒绝与其领域 Store 中未终结 Allocation/Residual 冲突的候选。
- Allocation settled 只证明领域资源分配；Runtime 是否把 Lease 推进到 reserved/active 由 Runtime Owner 决定。

## 5. WorkspaceChangeSet 状态

```text
draft -> staged -> candidate -> review_pending -> governed
  -> committing -> inspected -> committed
                     |-> rejected
                     `-> indeterminate -> inspect_original_commit_only
```

- `staged` 只在 Overlay；`committed` 必须有真实 Workspace 新 Revision 和 Evidence。
- Base Revision 漂移进入 `rejected` 或新的显式 merge/rebase Operation，不在原 Commit Attempt 中偷偷改基线。
- Review/Policy drift 必须创建新 Candidate/Revision 并重审。

### 5.1 Snapshot Artifact aggregate候选（NO-GO）

当前仅冻结`SnapshotArtifactOwnerV2`位于Sandbox控制域且独立于Controller/Provider/Enforcer；
以下状态机是`./snapshot-artifact-owner-v2.md`的联合审查候选，不是已批准实现合同：

```text
DeletionAttemptState = reserved | unknown | failed | confirmed | indeterminate
ArtifactAggregateState = reserved | available | deletion_in_progress | deleted | indeterminate

absent -> reserved -> available
                        |-> retention revision(s) -> available
                        `-> deletion_in_progress(one active Attempt)
                              |-> unknown -> Inspect same Attempt
                              |-> failed -> close Attempt -> available
                              |-> confirmed -> deleted + terminal tombstone/index
                              `-> indeterminate -> terminal tombstone/index

deleted / indeterminate: terminal, no resurrection
```

Artifact aggregate按`Tenant+AggregateID+purpose=deletion`只允许一个active Attempt；Attempt stable
key另含Operation/Effect/Attempt。unknown仍占active key；failed关闭后才能以fresh治理和全新stable
key重试。Retention/Reservation/历史Fact到期不改变AggregateState；CurrentIndex按state-active TTL
闭集续期。deleted/indeterminate tombstone只可同state增revision，不能因旧Fact过期回到absent。

Deletion confirmed不能以`LegalHoldRef=nil`证明无hold；S1/S2 proof必须绑定同一subject与exact
coverage（jurisdiction、hold-kind、selector），以及穷尽HoldIndex generation/set digest/count/query
watermark。每份watermark generation必须等于HoldIndex exact ref generation并绑定index revision/
digest；S1→S2按`(generation,sequence)`词典序。跨generation时必须携连续、端点衔接且coverage
exact的carry proof refs；同generation explicit absent。S2仍NoActive，不要求Projection revision/
digest相等。Unknown/Unavailable/NotFound/expired、carry缺失、coverage漂移或watermark回退均拒绝。

`deleted|indeterminate`只接受完整TerminalTombstone exact DTO与CurrentIndex terminal presence组合；
两者canonical body覆盖全部presence/TTL并排除own Ref/Digest。Tombstone state/aggregate/subject/cause
lineage不匹配、active Attempt与tombstone同时present、terminal续期清空tombstone均Conflict。

NotFound必须区分Owner Reservation replay、CAS winner lost reply与Provider Unknown：只有Owner
linearizable absent允许同request create-once；CAS丢回包Inspect expected successor/current；Provider
NotFound永远只Inspect原Attempt。冻结前不能创建Go枚举、Store、跨包committer或caller-mint入口。

## 6. Checkpoint Participant 状态

状态：`implemented`。Sandbox-owned Checkpoint phase Reservation/current Reader、Provider、
Evidence/DomainResult/Settlement/Apply与fresh Instance Restore均已按下述状态机落地。

```text
每个 prepare | commit | abort Phase：
candidate -> phase_reserved -> current_inspected -> admitted
  -> review_authorized -> permitted -> begun
  -> prepare_current_inspected -> prepare_enforcement_persisted -> provider_prepared
  -> execute_current_inspected -> execute_enforcement_persisted -> provider_observed
  -> evidence_consumed -> inspected -> domain_result_fact
  -> runtime_settlement_ref_bound -> apply_settled

任一可能已调用 Provider 的未知
  -> phase_unknown -> inspect/reconcile_original_phase_attempt_only
       |-> confirmed -> domain_result_fact -> runtime_settlement_ref_bound -> apply_settled
       `-> unresolved -> participant_indeterminate
```

- 目标完整链严格为`ReservePhase -> InspectCurrent -> Admission -> Review/Auth -> Permit -> Begin -> prepare Enforcement -> Provider Prepare -> execute Enforcement -> Provider ExecutePrepared -> Evidence -> DomainResult -> Settlement -> Apply`；两个Enforcement前都必须再次InspectCurrent。缺任一门、任一门过期/漂移或Begin之后未形成对应Enforcement时，Provider调用必须为0。
- `SandboxCheckpointPhaseReservationV2`、Participant exact-current Reader、Evidence、
  DomainResult、Settlement、Apply与Provider SPI均为live实现。
- Participant聚合投影只能由各Phase已ApplySettlement的精确Fact推导：`unprepared | prepared | committed | aborted | incomplete | confirmed_not_applied | indeterminate`。`failed`是prepare Phase结果，只能聚合为`incomplete`；`not_applied`只能聚合为`confirmed_not_applied`。`aborted`不表示Snapshot已删除、外部Effect已回滚或Residual已清理；这些必须由独立Operation结算。
- Sandbox只提交Participant Phase Fact和Compatibility；Runtime唯一拥有CheckpointAttempt、Barrier、Effect Cut、Checkpoint资格、consistent Fact、restore eligibility以及Instance/Lease/Fence迁移。Continuity只拥有Manifest/RestorePlan Fact及其Inspect/CAS，不能推进上述Runtime权威事实。
- `partial`/`unknown`仅诊断，不能自动Commit、Abort、Restore或创建替代Attempt。

### 6.1 Runtime P14一致的唯一fail-closed Phase状态机

```text
prepare.apply_settled(prepared)
  -> publish PreviousPhaseClosureRef
  -> CAS choose exactly one successor
       |-> commit.phase_reserved -> ... -> commit.apply_settled
       `-> abort.phase_reserved  -> ... -> abort.apply_settled

prepare.apply_settled(failed)
  -> participant.incomplete                  （terminal；无后继）

prepare.apply_settled(not_applied)
  -> participant.confirmed_not_applied       （terminal；无后继）

prepare.unknown
  -> Inspect/Reconcile original prepare Attempt only
       |-> resolved prepared     -> 回到prepared分支守卫
       |-> resolved failed       -> participant.incomplete
       |-> resolved not_applied  -> participant.confirmed_not_applied
       `-> unresolved            -> participant.indeterminate（terminal；无后继）
```

- 本状态机与Runtime P14最终联合审计裁决一致：只有exact `prepare.apply_settled(prepared)`可以创建后继；`failed`、`not_applied`、`unknown`、`indeterminate`都不能创建commit或abort。Sandbox不得采纳任何允许这些状态进入“补偿性abort”的历史表述。
- `commit`或`abort`只能从同一`prepare.apply_settled(prepared)`开始，必须绑定exact `PreviousPhaseRef`与`PreviousPhaseClosureRef(ID/Revision/Digest/State=prepared/ExpiresUnixNano)`；未闭合、failed、not_applied、unknown、indeterminate、过期或摘要漂移的prepare不能产生后继。
- `commit`与`abort`对同一PreviousPhase closure互斥。后继选择使用create-once/CAS closure key；一支Reservation存在或已推进后，另一支永久冲突，不能因重试、超时、TTL刷新或进程重启切换分支。
- Reserve/CAS回包丢失时按closure key Inspect：若精确后继已经存在或已推进，返回并继续Inspect该后继；不得重开prepare、建立同类第二后继或建立兄弟分支。PreviousPhase revision/digest/closure及终态单调不可复活，禁止`prepared -> committed -> prepared`式ABA。
- `unknown`期间只允许Inspect/Reconcile原prepare Operation/Attempt及其Evidence/Settlement/Apply binding；禁止Reserve后继、创建新Attempt、换Provider或把unknown当failed/not_applied。Reconcile仍无法获得Owner确认时，唯一收口是`indeterminate`。
- current Reader必须先独立复读Reservation，再复读PreviousPhase closure、Runtime Attempt/Barrier/Effect Cut、Operation/Authority/Review/Budget/Scope/Permit、Lease/Fence、Policy/Workspace/Backend/Generation及全部TTL；请求只携带exact expected refs，不接受caller snapshot或`current=true`。
- 可选ref使用显式discriminator。接口或扩展字段携带typed-nil pointer、nil/empty混淆、unknown required variant时`ValidateShape`直接拒绝；不能把typed-nil解释为缺省或跳过门禁。
- `now >= ExpiresUnixNano`即不current；后继expiry必须小于等于PreviousPhase closure与全部current上游TTL的最小值。过期的committed/aborted等历史终态仍可`ValidateShape`和Inspect，但不能创建新Phase或触发Provider。

Compatibility是不可变的派生领域Fact：

```text
candidate -> exact_current_read -> compatible | incompatible
                              \-> indeterminate
compatible --任一source/target revision、digest、host watermark或TTL漂移--> stale
```

历史`compatible/incompatible/stale`均可Inspect；只有仍在最短TTL内且全部source/target
exact-current一致的`compatible`可进入Restore Admission。

Restore是独立Effect：

```text
restore_candidate
  -> exact_snapshot_and_eligibility_current
  -> fresh_instance_binding
  -> admitted/reviewed/permitted
  -> prepare_enforced/provider_prepared
  -> execute_enforced/provider_staged
  -> inspected/evidenced/domain_result
  -> runtime_settlement
  -> sandbox_apply_settled
```

Runtime独立拥有consistent Checkpoint、restore eligibility、新Instance/更高epoch/新Lease/Fence；
Continuity只拥有Manifest/RestorePlan。transport kind不授Authority/Permit，旧Instance/Lease只能
作source provenance；禁止映射activation scope、Evidence V3或Settlement V4。

## 7. Cleanup/Residual 状态

```text
pending -> inspecting -> complete
                    |-> residual
                    `-> indeterminate
```

- `complete` 需要进程、文件挂载、网络、Secret、后台任务、远端继续执行、Provider 保留七维全部 confirmed clean。
- `residual` 表示已确认残留；`indeterminate` 表示覆盖不足或状态未知，不能互相替代。
- 两者均占用原 tenant-stable Conflict Domain。后续清理必须创建新的、关联原资源和原Attempt的`CleanupAttempt`，重新经过完整治理链；旧`residual/indeterminate`终态保持不可变，不能复活回`pending`。
- Release、Close、进程退出、容器停止、VM 终止均不能单独推出 Cleanup complete。

## 8. Epoch、Revision 与迟到回包

- Instance epoch、Lease ID/Epoch、Fence epoch 由 Runtime 拥有；Sandbox Fact 只引用当前水位。
- 每次实际执行点校验 OperationSubject、Scope、Lease 值、Authority、Review、Budget、Policy、Provider Binding、Permit、Fence、TTL。
- 相等 Scope 必须使用 Runtime `SameExecutionScopeV2`/`SameOperationSubjectV3` 的值语义；不得比较 SandboxLease 指针地址。
- 旧 Epoch/Revision/Permit/Provider Binding 回包只能进入迟到 Evidence 分区，不能 CAS 当前 Sandbox Fact。
- 同源同序同内容幂等；同序换内容为 EvidenceConflict。
