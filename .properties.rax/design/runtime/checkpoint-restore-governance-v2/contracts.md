# Checkpoint/Restore Governance V2合同

状态：**Checkpoint-first V2第三次独立代码终审YES（P0/P1/P2=0），本纵切完成**。本合同仍不授权Restore、真实Provider、production backend/root或SLA。

## 1. 版本、canonical与边界

| 合同 | SemVer | 当前资格 |
|---|---|---|
| `praxis.runtime/checkpoint-governance` | `2.0.0` | Checkpoint第一波候选 |
| `praxis.runtime/checkpoint-participant-reservation` | `2.0.0` | Checkpoint第一波候选 |
| `praxis.runtime/checkpoint-restore-evidence` | `1.0.0` | 仅checkpoint phase候选；restore current unsupported |
| `praxis.runtime/operation-settlement-checkpoint-restore` | `5.0.0` | 仅checkpoint phase候选；restore current unsupported |
| `praxis.runtime/restore-governance` | `2.0.0` | shape only，运行能力后置 |

所有Fact/Ref使用结构化ID、Revision、Digest；current Ref另带真实`ExpiresUnixNano`。Digest以domain/version/discriminator隔离，计算时清空self digest；strict decode拒绝未知治理字段、重复键和尾随文档。无序集合canonical排序并拒绝重复，nil/empty统一；所有公共集合必须有显式cardinality/payload上限。

V2不修改V3/V4/4.1任何字段、Validate、canonical或digest。legacy对象不能经alias、默认字段或sidecar升级为V2。

## 2. Attempt+Barrier原子bundle

### 2.1 refs与bundle

```go
type CheckpointAttemptRefV2 struct {
	TenantID core.TenantID `json:"tenant_id"`
	ID       string        `json:"attempt_id"`
	Revision core.Revision `json:"revision"`
	Digest   core.Digest   `json:"digest"`
}

type CheckpointBarrierLeaseRefV2 struct {
	TenantID        core.TenantID `json:"tenant_id"`
	ID              string        `json:"barrier_id"`
	AttemptID       string        `json:"attempt_id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

type CheckpointAttemptBarrierBundleV2 struct {
	Attempt CheckpointAttemptFactV2      `json:"attempt"`
	Barrier CheckpointBarrierLeaseFactV2 `json:"barrier"`
}
```

`CheckpointAttemptFactV2`最低绑定Tenant、ExecutionScope及digest、Run ID和stable identity digest、Generation、Generation-Binding association、BindingSet、required Participant Set certification、workflow、Barrier exact ref、Policy、Created/Updated和状态。

`CheckpointBarrierLeaseFactV2`最低绑定Tenant、Attempt ID、Scope/Run stable identity、Barrier Policy、冲突Effect分类、acquired dispatch watermark、Acquired/Expires、状态与Close provenance。Barrier只阻止Policy声明冲突的新dispatch；不证明quiesced、Effect已settled或Checkpoint consistent。

### 2.2 create请求与状态

```go
type CreateCheckpointAttemptRequestV2 struct {
	AttemptID                   string                                       `json:"attempt_id"`
	BarrierID                   string                                       `json:"barrier_id"`
	IdempotencyKey              string                                       `json:"idempotency_key"`
	Scope                       core.ExecutionScope                          `json:"scope"`
	ScopeDigest                 core.Digest                                  `json:"scope_digest"`
	RunID                       core.AgentRunID                              `json:"run_id"`
	RunStableIdentityDigest     core.Digest                                  `json:"run_stable_identity_digest"`
	Generation                  GenerationArtifactRefV1                      `json:"generation"`
	GenerationBinding           GenerationBindingAssociationRefV1            `json:"generation_binding"`
	BindingSet                  RunBindingSetRefV2                           `json:"binding_set"`
	ParticipantSetCertification CheckpointParticipantSetCertificationRefV2   `json:"participant_set_certification"`
	Workflow                    CheckpointWorkflowRefV2                       `json:"workflow"`
	BarrierPolicy               CheckpointBarrierPolicyRefV2                  `json:"barrier_policy"`
	ExpectedRunRevision         core.Revision                                 `json:"expected_run_revision"`
	ExpectedBarrierExpiresUnixNano int64                                      `json:"expected_barrier_expires_unix_nano,omitempty"`
}
```

caller不得提交自由`BarrierTTL`。Gateway必须通过`CheckpointBarrierPolicyCurrentReaderV2`复读请求中的exact Policy；Attempt保存该Policy ref作为冻结值，Barrier expiry由Owner用checked arithmetic派生：

```go
type CheckpointBarrierPolicyCurrentProjectionV2 struct {
	Ref                      CheckpointBarrierPolicyRefV2 `json:"ref"`
	MaxBarrierTTLUnixNano    int64                        `json:"max_barrier_ttl_unix_nano"`
	MaxReconciliationTTLUnixNano int64                    `json:"max_reconciliation_ttl_unix_nano"`
	UnknownAtDeadlineMode    CheckpointUnknownAtDeadlineModeV2 `json:"unknown_at_deadline_mode"`
	AbsoluteNotAfterUnixNano int64                        `json:"absolute_not_after_unix_nano"`
	CheckedUnixNano          int64                        `json:"checked_unix_nano"`
	ExpiresUnixNano          int64                        `json:"expires_unix_nano"`
	ProjectionDigest         core.Digest                  `json:"projection_digest"`
}

type CheckpointUnknownAtDeadlineModeV2 string

const (
	CheckpointUnknownAtDeadlineIndeterminate CheckpointUnknownAtDeadlineModeV2 = "terminalize_indeterminate"
)
```

```text
Expires = min(
  now + Policy.MaxBarrierTTL,
  Policy.ExpiresUnixNano,
  Run/Workflow/Binding/Authority current NotAfter
)

ReconciliationDeadline = min(
  now + Policy.MaxReconciliationTTL,
  Expires,
  Policy.ExpiresUnixNano,
  Run/Workflow/Binding/Authority current NotAfter
)
```

`MaxBarrierTTL <= 0`、`MaxReconciliationTTL <= 0`、加法overflow、任一上游`NotAfter <= now`、deadline不满足`now < deadline <= Expires`或派生值与非零`ExpectedBarrierExpiresUnixNano`不一致，必须在Attempt/Barrier写入前拒绝。Create只接受冻结Policy明确声明`terminalize_indeterminate` deadline mode；空值、deny或其他模式会产生Barrier expiry+unknown死锁，必须零写拒绝。expected字段只用于exact compare，不能延长Policy。Attempt持久化exact Policy ref、Policy semantic digest与`ReconciliationDeadlineUnixNano`。Freeze新Cut、Reserve新phase与Commit consistent仍复读Policy current并拒绝漂移；非成功Finalize只使用Attempt冻结语义，Policy后续expiry/revoke/drift不能取消强制收口或改变结论。

持久状态不包含孤立`proposed`：

```text
barrier_acquired -> cut_frozen -> collecting -> finalizing_inputs -> consistent
barrier_acquired|cut_frozen|collecting|finalizing_inputs -> incomplete | aborted | indeterminate
```

`CreateCheckpointAttemptV2`由同一Runtime Checkpoint Fact Owner在一个事务中写Attempt+Barrier。回值必须完整Validate并与请求exact；任一半对象存在视为Owner原子合同破坏并fail closed。相同ID同canonical幂等；同Attempt ID换Barrier、Scope、Run、Generation、Binding、Participant set、Workflow或Policy为Conflict。

Barrier不可续同一Fact。由于`ReconciliationDeadline <= Expires`，时间状态只有两段：`now < ReconciliationDeadline`时Barrier必然仍current，unknown只Inspect/Reconcile；`now >= ReconciliationDeadline`时Runtime必须使用已认证Finalization Input Closure原子派生`indeterminate`并关闭Barrier。Barrier expiry一旦到达也必已进入deadline终结段，并禁止Freeze新Cut、Reserve新phase或Commit consistent。该deadline终结是Attempt创建资格的一部分，不允许后续Policy否决。

## 3. EffectCut

`EffectCutFactV2`由Runtime Owner一次freeze后不可变，并与Attempt状态`barrier_acquired -> cut_frozen`同事务提交。请求只携Attempt、Barrier、expected revisions和精确Effect inventory root/current watermark；caller不得提交Disposition或自由entries，Gateway从Fact Owners逐项派生Cut。

每个`EffectCutEntryV2`的外层`EffectID/IntentRevision/IntentDigest`必须与其
`OperationDispatchAttemptRefV3`逐字段exact；禁止A/B splice。Disposition闭表不含
`failed`，任何unknown alias、自由字符串或terminal/disposition混搭均在Owner写入前拒绝。

每个entry至少包含Effect、Intent、Dispatch Attempt、phase exact refs和封闭分类：

- `settled`：exact V3/V4/V5 Runtime Settlement ref；
- `confirmed_not_applied`：Owner inspection ref；
- `unknown`：原Attempt inspection与Residual；
- `excluded_by_policy`：exact Policy Fact ref。

`RuntimeOperationTerminalRefV2`是versioned tagged union，禁止裸字符串、仅digest opaque ref或未知required版本。任何已Begin Effect缺exact settlement/inspection都不能从Cut消失。

Barrier与Begin并发必须只有一个顺序胜出：Begin先胜则Effect进入Cut；Barrier先胜则Begin与actual execution point都拒绝。跨Owner水位在Freeze和最终Commit前重新Inspect，漂移则reconcile，不宣称跨Owner原子事务。

## 4. Participant ReservePhase

```go
type CheckpointParticipantPhaseV2 string

const (
	CheckpointPhasePrepare CheckpointParticipantPhaseV2 = "checkpoint_prepare"
	CheckpointPhaseCommit  CheckpointParticipantPhaseV2 = "checkpoint_commit"
	CheckpointPhaseAbort   CheckpointParticipantPhaseV2 = "checkpoint_abort"
)

type CheckpointParticipantPhaseReservationRefV2 struct {
	ID              string        `json:"id"`
	Revision        core.Revision `json:"revision"`
	Digest          core.Digest   `json:"digest"`
	ExpiresUnixNano int64         `json:"expires_unix_nano"`
}

type CheckpointParticipantPhaseReservationCurrentProjectionV2 struct {
	ContractVersion string                                           `json:"contract_version"`
	Ref             CheckpointParticipantPhaseReservationRefV2       `json:"ref"`
	Participant     CheckpointParticipantRefV2                       `json:"participant"`
	OwnerBinding    ProviderBindingRefV2                              `json:"owner_binding"`
	Phase           CheckpointParticipantPhaseV2                     `json:"phase"`
	Attempt         CheckpointAttemptRefV2                            `json:"attempt"`
	Barrier         CheckpointBarrierLeaseRefV2                       `json:"barrier"`
	EffectCut       EffectCutRefV2                                   `json:"effect_cut"`
	Operation       OperationSubjectV3                               `json:"operation"`
	OperationDigest core.Digest                                      `json:"operation_digest"`
	EffectID        core.EffectIntentID                              `json:"effect_id"`
	EffectKind      EffectKindV2                                     `json:"effect_kind"`
	IntentDigest    core.Digest                                      `json:"intent_digest"`
	PreviousPhase   *CheckpointParticipantPhaseClosureRefV2           `json:"previous_phase,omitempty"`
	Domain          CheckpointParticipantDomainReservationRefV2      `json:"domain"`
	Generation      GenerationBindingAssociationRefV1                `json:"generation"`
	CheckedUnixNano int64                                            `json:"checked_unix_nano"`
	ExpiresUnixNano int64                                            `json:"expires_unix_nano"`
	ProjectionDigest core.Digest                                     `json:"projection_digest"`
}

type CheckpointParticipantPhaseClosureRefV2 struct {
	Phase          CheckpointParticipantPhaseV2                `json:"phase"`
	Reservation    CheckpointParticipantPhaseReservationRefV2  `json:"reservation"`
	PhaseFact      CheckpointParticipantPhaseRefV2             `json:"phase_fact"`
	DomainResult   CheckpointParticipantDomainResultRefV2      `json:"domain_result"`
	Evidence       CheckpointRestoreEvidenceConsumptionRefV1    `json:"evidence"`
	Settlement     OperationCheckpointRestoreSettlementRefV5    `json:"settlement"`
	ApplySettlement CheckpointParticipantApplySettlementRefV2   `json:"apply_settlement"`
	Digest         core.Digest                                 `json:"digest"`
}

type CheckpointParticipantBranchGuardRefV2 struct {
	TenantID      core.TenantID                   `json:"tenant_id"`
	AttemptID     string                          `json:"attempt_id"`
	ParticipantID string                          `json:"participant_id"`
	SelectedPhase CheckpointParticipantPhaseV2    `json:"selected_phase"`
	Revision      core.Revision                   `json:"revision"`
	Digest        core.Digest                     `json:"digest"`
}
```

Reservation Fact由Participant Owner create-once；Runtime只持中立Ref/Reader，不写Sandbox或其他Participant事实。Reservation本身不镜像Admission/Review/Permit状态，也不授Provider权。

每phase固定顺序：

```text
ReservePhase -> InspectCurrent -> Admission -> Review/Auth -> Permit -> Begin
-> prepare Enforcement -> execute Enforcement -> Provider Observation
-> Evidence V1 -> DomainResult -> Settlement V5 -> ApplySettlement
```

prepare/commit/abort分别使用独立Reservation、Operation、Effect、Dispatch Attempt、Permit和双Enforcement；不得跨phase复用。Reservation过期、revoke、owner/binding/attempt/barrier/cut漂移时零Admission、零Provider。

### 4.1 Participant phase状态机与branch guard

每个Participant的prepare历史先闭合，commit与abort再竞争同一个create-once branch guard：

```text
prepare: reserved -> executing -> prepared | failed | not_applied | unknown

after exact prepared closure:
  branch_guard absent -> commit selected -> executing -> committed | failed | not_applied | unknown
  branch_guard absent -> abort  selected -> executing -> aborted   | failed | not_applied | unknown

failed | not_applied: no successor phase; feed incomplete | confirmed_not_applied finalization input
unknown: no successor phase; Inspect/Reconcile original phase; deadline -> indeterminate
```

- prepare的`PreviousPhase`必须为nil；只有`prepared`可创建commit或abort，且二者必须携exact prepare Reservation、Phase Fact、DomainResult、Evidence consumption、Settlement V5与ApplySettlement闭包；
- commit和abort共享`(TenantID, AttemptID, ParticipantID)` branch guard，只能一个分支线性化，另一分支即使使用不同Operation/Effect/Reservation ID也必须Conflict；
- prepare为`failed|not_applied|unknown`时禁止创建任何后继Reservation、Operation或Effect：failed进入`incomplete`，not_applied进入`confirmed_not_applied`，unknown只Inspect/Reconcile原phase并在deadline进入`indeterminate`；
- 选择commit或abort后不得切换，任何历史ref/revision/digest不得覆盖；后继phase的`unknown`同样只Inspect原phase/attempt，不能改走兄弟分支；
- `consistent`要求每个required Participant均有exact prepared closure及committed closure，且没有abort/unknown；
- 非成功closure中，`aborted`要求所有required Participant均有exact aborted closure或冻结Policy允许的confirmed-not-applied证明；已知failed/missing形成`incomplete`；任一unknown/不可判定形成reconcile或`indeterminate`，不得伪造成功closure。

## 5. Continuity ManifestFact与ManifestSeal

Continuity通过其独立ManifestSeal Owner Port拥有可CAS的`CheckpointManifestFactV2`和immutable revision 1的`CheckpointManifestSealFactV2`。ManifestFact可处于`collecting|verified_candidate|partial|indeterminate|rejected`，但任何状态都不等于Runtime consistent；不再使用歧义状态`verified`。该Owner Port及lost-reply Inspect是Checkpoint第一波的联合前置；Runtime只持下列中立Reader，不实现第二个Manifest/Seal Store。

Runtime `ports`只定义中立Seal ref/projection/reader。`CheckpointManifestSealContractVersionV2=2.1.0`是additive exact lookup版本；`CheckpointExternalExactFactRefV2`保留Continuity完整OwnerBinding、Tenant/Scope、ID/revision与raw digest，不复制Continuity Fact payload：

```go
type CheckpointManifestSealRefV2 struct {
	ExactLookup        CheckpointExternalExactFactRefV2 `json:"exact_lookup"`
	ID                 string                      `json:"seal_id"`
	Revision           core.Revision               `json:"revision"`
	Digest             core.Digest                 `json:"digest"`
	ManifestID         string                      `json:"manifest_id"`
	ManifestRevision   core.Revision               `json:"manifest_revision"`
	ManifestDigest     core.Digest                 `json:"manifest_digest"`
	Attempt            CheckpointAttemptRefV2      `json:"attempt"`
	Barrier            CheckpointBarrierLeaseRefV2 `json:"barrier"`
	EffectCut          EffectCutRefV2              `json:"effect_cut"`
	FrozenRefSetDigest core.Digest                 `json:"frozen_ref_set_digest"`
}

type InspectCheckpointManifestSealRequestV2 struct {
	Ref                          CheckpointManifestSealRefV2          `json:"ref"`
	ExpectedParticipantSetDigest core.Digest                         `json:"expected_participant_set_digest"`
	ExpectedParticipantClosures  []CheckpointParticipantClosureRefV2 `json:"expected_participant_closures"`
}

type CheckpointManifestSealProjectionV2 struct {
	Ref                   CheckpointManifestSealRefV2       `json:"ref"`
	ParticipantSetDigest  core.Digest                       `json:"participant_set_digest"`
	ParticipantClosures   []CheckpointParticipantClosureRefV2 `json:"participant_closures"`
	ContextClosureDigest  core.Digest                       `json:"context_closure_digest"`
	ArtifactClosureDigest core.Digest                       `json:"artifact_closure_digest"`
	SealDigest            core.Digest                       `json:"seal_digest"`
}
```

Runtime typed Participant closure到Continuity exact ref只能调用`DeriveCheckpointParticipantClosureExactRefV2`：完整Provider Owner binding、Tenant、Scope、closure ID、immutable revision 1和digest全部参与；禁止字符串拼接identity、Application私有映射或扫描ID。external SHA-256只经`NormalizeCheckpointExternalSHA256DigestV2`进入`core.Digest`，raw exact spelling继续保留用于Owner lookup。Gateway先读current Participant Set/closures再Inspect Seal，CAS前按相同coordinate完成S2；Reader逐项校验Runtime closure、Context/Artifact closure digest与frozen set。Runtime Consistency中唯一Continuity-owned字段仍是`CheckpointManifestSealRefV2`。mutable ManifestFact ref、Manifest candidate、RestorePlan、Continuity自报`consistent`或current eligibility均禁止进入Consistency。

## 6. Consistency成功终结与非成功Finalize

`CheckpointConsistencyFactV2`是immutable revision 1，只允许结论`consistent`。它绑定Attempt、Barrier、EffectCut、ManifestSeal、required Participant closure set digest、frozen-ref-set digest与创建时Runtime current watermarks；不携TTL或Restore current资格。

```go
type CommitCheckpointConsistencyRequestV2 struct {
	Attempt                  CheckpointAttemptRefV2             `json:"attempt"`
	Barrier                  CheckpointBarrierLeaseRefV2        `json:"barrier"`
	ExpectedAttemptRevision  core.Revision                      `json:"expected_attempt_revision"`
	ExpectedBarrierRevision  core.Revision                      `json:"expected_barrier_revision"`
	EffectCut                EffectCutRefV2                     `json:"effect_cut"`
	ManifestSeal             CheckpointManifestSealRefV2        `json:"manifest_seal"`
	ParticipantClosures      []CheckpointParticipantClosureRefV2 `json:"participant_closures"`
	IdempotencyKey           string                             `json:"idempotency_key"`
}

type CheckpointConsistencyCommitBundleV2 struct {
	Attempt     CheckpointAttemptFactV2     `json:"attempt"`
	Barrier     CheckpointBarrierLeaseFactV2 `json:"barrier"`
	Consistency CheckpointConsistencyFactV2  `json:"consistency"`
}
```

Runtime在一个Owner事务中原子：Attempt→consistent、Barrier→closed、create Consistency。三对象全有或全无。

非成功路径：

```go
type FinalizeCheckpointAttemptRequestV2 struct {
	Attempt                 CheckpointAttemptRefV2       `json:"attempt"`
	Barrier                 CheckpointBarrierLeaseRefV2  `json:"barrier"`
	ExpectedAttemptRevision core.Revision                `json:"expected_attempt_revision"`
	ExpectedBarrierRevision core.Revision                `json:"expected_barrier_revision"`
	Inputs                  CheckpointFinalizationInputClosureRefV2 `json:"inputs"`
	IdempotencyKey          string                       `json:"idempotency_key"`
}

type PrepareCheckpointFinalizationInputsRequestV2 struct {
	Attempt                 CheckpointAttemptRefV2        `json:"attempt"`
	Barrier                 CheckpointBarrierLeaseRefV2   `json:"barrier"`
	EffectCut               EffectCutRefV2                `json:"effect_cut"`
	ExpectedAttemptRevision core.Revision                 `json:"expected_attempt_revision"`
	ExpectedBarrierRevision core.Revision                 `json:"expected_barrier_revision"`
	ExpectedDiagnostics     *CheckpointDiagnosticSetRefV2 `json:"expected_diagnostics,omitempty"`
	ExpectedResiduals       *CheckpointResidualSetRefV2   `json:"expected_residuals,omitempty"`
	IdempotencyKey          string                        `json:"idempotency_key"`
}

type CheckpointDiagnosticSetRefV2 struct {
	AttemptID  string        `json:"attempt_id"`
	Revision   core.Revision `json:"revision"`
	Count      uint32        `json:"count"`
	SetDigest  core.Digest   `json:"set_digest"`
}

type CheckpointResidualSetRefV2 struct {
	AttemptID  string        `json:"attempt_id"`
	Revision   core.Revision `json:"revision"`
	Count      uint32        `json:"count"`
	SetDigest  core.Digest   `json:"set_digest"`
}

type CheckpointFinalizationCutRefV2 struct {
	ID            string                 `json:"cut_id"`
	Revision      core.Revision          `json:"revision"`
	Attempt       CheckpointAttemptRefV2 `json:"attempt"`
	EffectCut     EffectCutRefV2         `json:"effect_cut"`
	CutUnixNano   int64                  `json:"cut_unix_nano"`
	Digest        core.Digest            `json:"digest"`
}

type CheckpointFinalizationInputClosureRefV2 struct {
	ID                       string                       `json:"closure_id"`
	Revision                 core.Revision                `json:"revision"`
	Attempt                  CheckpointAttemptRefV2       `json:"attempt"`
	Barrier                  CheckpointBarrierLeaseRefV2  `json:"barrier"`
	EffectCut                EffectCutRefV2               `json:"effect_cut"`
	FinalizationCut          CheckpointFinalizationCutRefV2 `json:"finalization_cut"`
	DiagnosticsSeal          CheckpointDiagnosticsFinalizationSealRefV2 `json:"diagnostics_seal"`
	ResidualsSeal            CheckpointResidualsFinalizationSealRefV2 `json:"residuals_seal"`
	Digest                   core.Digest                  `json:"digest"`
}

type CheckpointAttemptTerminalCurrentProjectionV2 struct {
	ContractVersion string                                      `json:"contract_version"`
	Attempt         CheckpointAttemptRefV2                      `json:"attempt"`
	Barrier         CheckpointBarrierLeaseRefV2                 `json:"barrier"`
	TerminalState   CheckpointAttemptStateV2                    `json:"terminal_state"`
	Consistency     *CheckpointConsistencyRefV2                 `json:"consistency,omitempty"`
	Inputs          *CheckpointFinalizationInputClosureRefV2    `json:"inputs,omitempty"`
	DiagnosticsSeal *CheckpointDiagnosticsFinalizationSealRefV2 `json:"diagnostics_seal,omitempty"`
	ResidualsSeal   *CheckpointResidualsFinalizationSealRefV2   `json:"residuals_seal,omitempty"`
	CheckedUnixNano int64                                       `json:"checked_unix_nano"`
	ProjectionDigest core.Digest                                `json:"projection_digest"`
}

type CheckpointDiagnosticsFinalizationSealRefV2 struct {
	ID                string                          `json:"seal_id"`
	Revision          core.Revision                   `json:"revision"`
	Attempt           CheckpointAttemptRefV2          `json:"attempt"`
	FinalizationCut   CheckpointFinalizationCutRefV2  `json:"finalization_cut"`
	Owner             ProviderBindingRefV2            `json:"owner"`
	SourceEpoch       uint64                          `json:"source_epoch"`
	SourceSequence    uint64                          `json:"source_sequence"`
	LedgerRootDigest  core.Digest                     `json:"ledger_root_digest"`
	CompleteSet       CheckpointDiagnosticSetRefV2    `json:"complete_set"`
	CompleteSetDigest core.Digest                     `json:"complete_set_digest"`
	Digest            core.Digest                     `json:"digest"`
}

type CheckpointResidualsFinalizationSealRefV2 struct {
	ID                string                          `json:"seal_id"`
	Revision          core.Revision                   `json:"revision"`
	Attempt           CheckpointAttemptRefV2          `json:"attempt"`
	FinalizationCut   CheckpointFinalizationCutRefV2  `json:"finalization_cut"`
	Owner             ProviderBindingRefV2            `json:"owner"`
	SourceEpoch       uint64                          `json:"source_epoch"`
	SourceSequence    uint64                          `json:"source_sequence"`
	LedgerRootDigest  core.Digest                     `json:"ledger_root_digest"`
	CompleteSet       CheckpointResidualSetRefV2      `json:"complete_set"`
	CompleteSetDigest core.Digest                     `json:"complete_set_digest"`
	Digest            core.Digest                     `json:"digest"`
}
```

caller不提交Trigger、Policy、实际diagnostics/residuals或terminal state。Runtime先以fresh clock和expected revisions将Attempt原子推进到`finalizing_inputs`并冻结`CheckpointFinalizationCutRefV2`；新phase/Provider从该点起全部拒绝。Diagnostics Owner与Residuals Owner必须各自以create-once事务生成独立immutable Seal，两个Seal都精确绑定同一Attempt/Cut、各自Owner identity、source epoch/sequence、ledger/root digest、complete-set ref/digest。

Gateway顺序为`Create/Inspect Diagnostics Seal A → Create/Inspect Residuals Seal B → Inspect A current → Inspect B current`。任一Seal create lost reply只Inspect原ID；同ID换Owner/epoch/sequence/root/set/Cut为Conflict。A/B exact稳定后，Runtime Checkpoint Fact Owner create-once `CheckpointFinalizationInputClosureRefV2`并将其绑定到Attempt history；Closure只绑定两个typed Owner Seal，不复制自由set、水位或caller事实。expected set若存在只能与Owner Seal内complete set exact compare，省略expected set不省略Owner封印。

Finalization Cut定义共同冻结点：两个Owner Seal必须证明包含所有causally-at-or-before Cut的事实。Seal发布后，该Owner不得再接受任何pre-cut fact；尝试发布必须Conflict且Ledger/Seal不变。Cut之后发生的Observation只进入post-cut审计分区，不得重写Seal、Closure或historical terminal。

最终CAS前Runtime必须再次Inspect两个Seal current并与Closure exact比较。若B Seal之后、Finalize之前出现pre-cut unknown：要么其发布先胜并被Seal纳入，要么Seal先胜且发布Conflict；禁止两者都成功。若Owner错误接受Seal后的pre-cut fact、Reader报告root/sequence/set漂移或Owner不可判定，则current terminal projection必须Fail Closed不可见，Finalize零写；historical Fact仍仅供审计。Finalize后晚到pre-cut unknown也必须在Owner入口Conflict；若Owner违反该约束，后续current Inspect不得继续暴露terminal。Finalize只接受Attempt history中exact绑定的Closure ref。

Runtime派生规则：完整Closure证明已知required缺口/failed且无unknown→`incomplete`；只有prepare已`prepared`并经显式abort分支闭合，或prepare为`not_applied`且有confirmed-not-applied证明，才可参与非成功结论；任何unknown、遗漏、Reader unavailable、集合不完整或结论无法证明在deadline前只reconcile。`now >= ReconciliationDeadlineUnixNano`时，冻结Policy已预先授权唯一基线结论`indeterminate`，Runtime必须用exact Closure原子终结Attempt并关闭Barrier。伪造safe-cancel、换Policy、隐藏unknown或省略Residual均零写。Attempt终结与Barrier closed同事务，不创建Consistency。

Owner sealing/current-inspection dependencies：

```go
type CheckpointAttemptDiagnosticsFinalizationOwnerPortV2 interface {
	SealCheckpointDiagnosticsForFinalizationV2(
		context.Context,
		CheckpointAttemptRefV2,
		EffectCutRefV2,
		CheckpointFinalizationCutRefV2,
	) (CheckpointDiagnosticsFinalizationSealRefV2, error)

	InspectCheckpointDiagnosticsFinalizationSealCurrentV2(
		context.Context,
		CheckpointDiagnosticsFinalizationSealRefV2,
	) (CheckpointDiagnosticsFinalizationSealProjectionV2, error)
}

type CheckpointAttemptResidualsFinalizationOwnerPortV2 interface {
	SealCheckpointResidualsForFinalizationV2(
		context.Context,
		CheckpointAttemptRefV2,
		EffectCutRefV2,
		CheckpointFinalizationCutRefV2,
	) (CheckpointResidualsFinalizationSealRefV2, error)

	InspectCheckpointResidualsFinalizationSealCurrentV2(
		context.Context,
		CheckpointResidualsFinalizationSealRefV2,
	) (CheckpointResidualsFinalizationSealProjectionV2, error)
}
```

两个Seal mutation分别由Diagnostics语义Owner和Residuals语义Owner执行；Runtime Gateway只协调其public Owner Port，不得代写Seal Fact、ledger/root、source sequence或complete set。两个Seal Projection必须重读immutable Seal Fact，并证明Owner、source epoch/sequence、ledger/root、complete-set、Finalization Cut、Checked/current status与self digest exact；不得只返回caller给出的ID列表。Runtime Closure是跨Owner共同冻结认证，不把两个Owner Store合并为Runtime Store。

### 6.1 historical与terminal current读取面

`InspectCheckpointAttemptV2`、`InspectCheckpointFinalizationInputsV2`以及各Finalization Fact Inspect均明确是historical：它们证明历史对象存在且immutable，但不授current terminal可见性。

```go
type CheckpointAttemptTerminalCurrentReaderV2 interface {
	InspectCheckpointAttemptTerminalCurrentV2(
		context.Context,
		CheckpointAttemptRefV2,
	) (CheckpointAttemptTerminalCurrentProjectionV2, error)
}
```

current Inspect先Validate terminal Attempt、closed Barrier与terminal分支。`consistent`分支只允许exact Consistency ref且Finalization sidecars全nil；`incomplete|aborted|indeterminate`分支必须读取Attempt历史绑定的Closure，要求Projection中的两个Seal refs等于Closure，再分别调用两个Owner current Inspect。

`CheckpointAttemptTerminalCurrentProjectionV2.Validate`要求ContractVersion固定、Attempt/Barrier同Tenant与Attempt identity、TerminalState为封闭terminal enum、`CheckedUnixNano > 0`且self digest精确。consistent与非成功两种sidecar组合互斥；nil/empty、部分Closure、单Seal、换Seal ref或重Seal后字段漂移均拒绝。

任一Seal Owner/root/source epoch/source sequence/complete-set/Cut/ref/digest漂移，Owner violation，Reader unavailable/typed-nil，或current结果不可判定，都必须返回Conflict/Unavailable/Indeterminate并且不返回Projection；即“historical terminal存在但current不可见”。只有两个Seal current均exact且Projection重新Seal成功，调用方才可消费terminal current。该Reader零mutation、零Provider，不能把historical Inspect包装成current成功。

## 7. 专用Evidence V1与Settlement V5

Evidence V1不修改OperationScope Evidence V3闭表。Checkpoint第一波current phase只允许`checkpoint_prepare|checkpoint_commit|checkpoint_abort`；restore shape可以decode/ValidateShape，但`ValidateCurrent`、Issue/Handoff/Consume均unsupported。

Checkpoint Evidence Scope必须绑定Attempt、Barrier、EffectCut、Participant Reservation/phase、Operation/Effect/Intent/Dispatch Attempt、Admission、Authorization、Permit、prepare/execute Enforcement、Assembly route、Generation、Lease/Fence、Evidence Policy、source epoch/sequence、schema/payload和TTL。Gateway注入`CheckpointEvidenceExecutionCurrentReaderV1`、Evidence Policy current Reader与`CheckpointEvidenceSourceCurrentReaderV1`，S1/S2复读全部坐标；TTL=`min(30s cap, policy maximum TTL, Attempt/Barrier, inputs/Authority, Reservation, Permit/Enforcement/Sandbox, Policy, Source)`。Fact Validate同时限制30秒绝对上限，caller嵌入expiry只可exact匹配。Issue不推进cursor；只有`consumed_current`可进入V5；late/observation不能升级Settlement。

Handoff与Consumption ref均重算self digest。Governance Port提供historical exact Inspect和current Inspect；current路径必须重新验证Qualification及其Owner current闭包。Consume另注入既有`EvidenceSourceRecordReaderV2`，在Fact Owner写入前以S1/S2分别按RecordRef和SourceKey复读同一真实Ledger Record，验证record/candidate/chain self digest、instance ledger scope、source epoch/sequence、schema/payload、ExecutionScope与Authority exact；Reader nil、typed-nil、Unavailable、两次映射漂移或任一坐标splice均零Consumption/零cursor。mutation回包丢失使用`context.WithoutCancel` historical exact Inspect，不得盲重写或重调Provider。

Settlement V5精确绑定Effect/Operation、Attempt、Participant phase Fact、typed DomainResult Fact、Evidence V1 consumed_current/record/association、Handoff、Dispatch Attempt、4.1 phase、Assembly route与Owner。Runtime Settlement Owner必须在现有OperationEffectStore同一Owner实例与同一锁下，使用共享key`(TenantID, EffectID)`原子写Settlement、Association、Terminal Guard、Projection并CAS Effect terminal。

V3/V4/V5任一版本先胜，其他版本即使Settlement ID或OperationDigest不同也必须Conflict且零sidecar；跨Tenant相同EffectID独立。V5不选择Run Outcome、Checkpoint Consistency或Restore Eligibility。

## 8. Restore shape后置

可保留`RestoreAttemptRefV2`、`RestoreEligibilityRefV2`、Instance/Epoch/Lease/Fence Reservation refs、`CheckpointRestoreOperationScopeV2` restore branch和Continuity `RestorePlanV2`引用形状，但本轮不提供运行Port或current资格。

后续独立Review前：Restore Issue/Bind Eligibility、Admission、Authorization、Permit、Begin、Participant stage、Evidence current、Settlement current、Activate/Abort与Provider全部unsupported。历史Consistency/ManifestSeal不授Restore资格；legacy RestoreRequest不能升权。

## 9. CAS、lost reply、TTL与时钟

- mutation先Inspect create-once identity；只在明确NotFound时写；Unavailable/Indeterminate后用`context.WithoutCancel` exact Inspect；
- 相同ID同canonical幂等；同ID换内容Conflict；
- Attempt+Barrier、FreezeCut+Attempt revision、Finalization Cut+Attempt revision、Input Closure+Attempt binding、Consistency+CloseBarrier、Finalize+CloseBarrier各自单Owner原子；跨Owner不宣称事务；
- 第一次current check与最终CAS前取fresh clock，非零且不得回拨；`now >= expires`拒绝任何新Cut/phase/consistent，但允许使用创建时冻结deadline与exact Finalization Input Closure执行强制`indeterminate+close Barrier`；
- historical Fact过期后仍可Inspect，但不能作为新phase或执行资格；
- Commit与Finalize、Barrier与Begin、V3/V4/V5 terminal CAS并发均只允许一个线性化赢家；
- Unknown只Inspect原Attempt/phase/provider coordinate，不换ID、不重Provider、不递归远程Inspect。

### 9.1 legal progressed successor与no-ABA

lost-reply恢复必须先Validate历史Fact，再重派生immutable identity/request digest。仅当stored revision不小于expected，且stored state是本次目标或同一分支的合法传递后继，才可作为inspect-only恢复：

- create bundle：`barrier_acquired`可推进到`cut_frozen|collecting|finalizing_inputs|consistent|incomplete|aborted|indeterminate`，但Attempt/Barrier/Policy/ReconciliationDeadline/Scope/Run/Generation/Participant set历史必须exact；
- freeze Cut：可接受携同一immutable Cut ref的`cut_frozen|collecting|finalizing_inputs`及任一terminal后继；不得接受另一个Cut；
- prepare finalization inputs：只接受携同一Finalization Cut、Diagnostics Seal、Residuals Seal的`finalizing_inputs`或绑定同一Closure的terminal后继；任一跨Cut、跨Owner或混合Seal闭包均非合法后继；
- participant prepare：只接受prepare分支合法后继；commit/abort分别只能接受自己已选择的分支，不得把兄弟分支当幂等；
- consistency commit：只接受携同一Consistency ref和closed Barrier的`consistent`；
- non-success finalize：只接受同一派生结论、同一Closure/两个Owner Seal及closed Barrier的exact terminal。

same revision换digest、revision回退、低于expected、immutable字段改变、历史ref被覆盖、terminal改写或分支切换均为Conflict/Owner合同破坏。revision严格单调、terminal immutable和branch guard create-once共同禁止ABA；恢复路径不得重新调用Provider或重新Consume Evidence。

### 9.2 nil与typed-nil依赖

Gateway在任何Store/Reader调用前必须验证Fact Owner、Barrier Policy Reader、ManifestSeal Reader、Participant Reservation Reader、Diagnostics/Residuals Finalization Owner Ports、Evidence/Settlement Readers均非nil且非typed-nil。nil或typed-nil统一返回`component_missing`/InvalidArgument类fail-closed错误，零backend read、零mutation、零Provider。Store收到nil/typed-nil clock或transaction dependency同样不得panic或部分发布。

### 9.3 Sandbox Participant联合前置

Sandbox checkpoint链必须保持：Reservation →双Enforcement→Provider Observation/Receipt→Evidence Owner `consumed_current`→Sandbox Owner Inspect/CAS DomainResultFact→Runtime Settlement V5→Sandbox ApplySettlement。Provider Observation、Receipt、DomainResult、Runtime Settlement与ApplySettlement彼此不能替代；Sandbox Owner Port/Reader未冻结或缺失时，本设计保持真实路径unsupported且Provider调用数为0。

## 10. legacy边界

live `core.CheckpointSet`、`core.RestoreRequest`、`CheckpointParticipantPort`、`CheckpointParticipantReport`与Foundation Coordinator仅允许restricted test/legacy Inspect。其BarrierID/Epoch/Snapshot字符串、Observation或Receipt不得构造AttemptRef、BarrierRef、ReservationRef、ManifestSealRef、Evidence V1、Settlement V5或Consistency Fact。V2 public code、Conformance及联合Review YES前Provider调用数必须为0。

## 11. 独立审计返修合同

1. `RuntimeOperationTerminalRefV2`是封闭tagged union，结构上识别V3 Settlement、V4 Settlement、Checkpoint Settlement V5、Dispatch unknown V3与Checkpoint Policy exclusion五类typed ref；每种terminal只允许一个sidecar。EffectCut V2仅授权V3、V4、Dispatch unknown与Policy exclusion，V5因旧Ref不能证明exact Dispatch Attempt而必须fail closed/unsupported。
2. Consistency S1/S2必须从`CheckpointEffectInventoryCurrentReaderV2`重新读取完整current projection；完整Attempt/Barrier refs、root、watermark、count和每个规范化entry都必须等于immutable EffectCut。historical Cut存在不代替current inventory。正常CAS回值及lost-reply恢复拼出的Attempt+Barrier+Consistency必须与Gateway expected canonical bundle exact。
3. Attempt持久化`FrozenUnknownAtDeadlineMode`与`FrozenAllowConfirmedNotAppliedAbort`。非成功Finalize不读取current Policy；unknown在冻结deadline前只Inspect，deadline到达后只能派生indeterminate+closed Barrier。confirmed-not-applied是否可形成aborted只取冻结字段。
4. Qualification ref及Fact必须exact绑定Attempt、Barrier、EffectCut、Reservation、phase、full Scope digest与Owner派生expiry。Gateway在Owner create前对Checkpoint closure、Attempt Inputs、Reservation、Permit/双Enforcement、Evidence Policy及source分别S1/S2复读；expiry采用全部current上界、Policy maximum与30秒cap的最小值，caller不能延长。
5. Create前通过`CheckpointRunCurrentReaderV2`消费`ExpectedRunRevision`，并精确验证running Run、stable identity及scope。Create unknown恢复只接受同immutable identity且拥有从expected create revision到current revision逐步合法Attempt/Barrier transition lineage的successor；缺口、非法跳转、ABA或immutable字段变化为Conflict。
6. terminal-current必须从两个typed classification Seal重新派生non-success state并与terminal Fact exact；错误终态只保留historical审计，不返回current Projection。
7. terminal-current在首个Fact/backend read前preflight Facts、Clock、Manifest、Participant set/closure/branch、Diagnostics、Residuals完整依赖集合；任何nil/typed-nil返回component_missing且backend read计数为0。
