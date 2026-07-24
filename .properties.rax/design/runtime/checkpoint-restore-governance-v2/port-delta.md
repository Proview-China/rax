# Checkpoint/Restore Governance V2公共Port Delta

状态：**Checkpoint-first V2第三次独立代码终审YES（P0/P1/P2=0），additive public/reference纵切完成**。不授权production composition；Restore只保留shape，不提供运行Port。

协调性已实施Delta：`OperationSettlementCurrentReaderV3` 仅抽取既有
`InspectOperationSettlementV3` 只读方法；`OperationSettlementGovernancePortV3`
改为嵌入该Reader并保留原`SettleOperationEffectV3`，因此既有方法集、对象、
digest与行为均未改变。该协调Delta已随Checkpoint第三次独立终审共同验证。

## 1. Runtime Checkpoint Governance V2

公共类型：

- `CheckpointAttemptRefV2`、`CheckpointBarrierLeaseRefV2`、`CheckpointAttemptBarrierBundleV2`；
- `EffectCutRefV2`、`CheckpointEffectCutBundleV2`；
- `CheckpointConsistencyRefV2`、`CheckpointConsistencyCommitBundleV2`；
- `CheckpointFinalizationCutRefV2`、`CheckpointDiagnosticsFinalizationSealRefV2`、`CheckpointResidualsFinalizationSealRefV2`、`CheckpointFinalizationInputClosureRefV2`、`CheckpointAttemptFinalizationBundleV2`、`CheckpointAttemptTerminalCurrentProjectionV2`；
- `CreateCheckpointAttemptRequestV2`、`InspectCheckpointAttemptRequestV2`、`FreezeCheckpointEffectCutRequestV2`、`PrepareCheckpointFinalizationInputsRequestV2`、`CommitCheckpointConsistencyRequestV2`、`FinalizeCheckpointAttemptRequestV2`；
- historical/current inspection projections。

公共Governance Port候选：

```go
type CheckpointGovernancePortV2 interface {
	CreateCheckpointAttemptV2(
		context.Context,
		CreateCheckpointAttemptRequestV2,
	) (CheckpointAttemptBarrierBundleV2, error)

	InspectCheckpointAttemptV2(
		context.Context,
		InspectCheckpointAttemptRequestV2,
	) (CheckpointAttemptBarrierBundleV2, error)
	// Historical only: existence/immutability, not current terminal visibility.

	InspectCheckpointBarrierHistoricalV2(
		context.Context,
		CheckpointBarrierLeaseRefV2,
	) (CheckpointBarrierLeaseFactV2, error)

	InspectCheckpointBarrierCurrentV2(
		context.Context,
		CheckpointBarrierLeaseRefV2,
	) (CheckpointBarrierCurrentProjectionV2, error)

	FreezeCheckpointEffectCutV2(
		context.Context,
		FreezeCheckpointEffectCutRequestV2,
	) (CheckpointEffectCutBundleV2, error)

	InspectCheckpointEffectCutV2(
		context.Context,
		EffectCutRefV2,
	) (EffectCutFactV2, error)

	PrepareCheckpointFinalizationInputsV2(
		context.Context,
		PrepareCheckpointFinalizationInputsRequestV2,
	) (CheckpointFinalizationInputClosureRefV2, error)

	InspectCheckpointFinalizationInputsV2(
		context.Context,
		CheckpointFinalizationInputClosureRefV2,
	) (CheckpointFinalizationInputClosureFactV2, error)
	// Historical only: exact Closure fact, not current Seal validity.

	InspectCheckpointAttemptTerminalCurrentV2(
		context.Context,
		CheckpointAttemptRefV2,
	) (CheckpointAttemptTerminalCurrentProjectionV2, error)

	CommitCheckpointConsistencyAndCloseBarrierV2(
		context.Context,
		CommitCheckpointConsistencyRequestV2,
	) (CheckpointConsistencyCommitBundleV2, error)

	FinalizeCheckpointAttemptAndCloseBarrierV2(
		context.Context,
		FinalizeCheckpointAttemptRequestV2,
	) (CheckpointAttemptFinalizationBundleV2, error)

	InspectCheckpointConsistencyV2(
		context.Context,
		CheckpointConsistencyRefV2,
	) (CheckpointConsistencyFactV2, error)
}
```

禁止额外`AcquireBarrier`、`CloseBarrier`或caller提交terminal state的mutation。Attempt+Barrier、Finalization Cut+Attempt revision、Input Closure+Attempt binding、Consistency+CloseBarrier、Finalize+CloseBarrier必须分别由同一Runtime Checkpoint Fact Owner原子提交。

Barrier Policy与Owner sealing/current-inspection dependencies：

```go
type CheckpointBarrierPolicyCurrentReaderV2 interface {
	InspectCheckpointBarrierPolicyCurrentV2(
		context.Context,
		CheckpointBarrierPolicyRefV2,
	) (CheckpointBarrierPolicyCurrentProjectionV2, error)
}

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

Create只把exact Barrier Policy ref冻结进Attempt；Owner从current Policy派生并限幅Barrier expiry与`ReconciliationDeadlineUnixNano`，caller无`BarrierTTL`或deadline写权。无法保证deadline时`indeterminate+close Barrier`的Policy在Create前零写拒绝。

`PrepareCheckpointFinalizationInputsV2`先原子冻结Attempt的Finalization Cut，再让Diagnostics Owner与Residuals Owner分别create-once immutable Seal；Seal mutation严格归各语义Owner，Runtime只协调Owner Port且不得代写。每个Seal绑定同一Cut及自身Owner、source epoch/sequence、ledger/root digest、complete-set ref/digest。Runtime随后重读两个Seal current并create-once Closure；Closure只绑定两个typed Seal。Seal后pre-cut publish必须Conflict。Finalize不接受Trigger、Policy或实际集合，只接受Attempt历史exact绑定的Closure；caller可选expected set仅在Prepare阶段与Seal内容exact compare。

`InspectCheckpointAttemptV2`与`InspectCheckpointFinalizationInputsV2`均为historical。唯一terminal current读取面是`InspectCheckpointAttemptTerminalCurrentV2`：它重新读取Closure和两个Owner Seal current，并逐字段核对Owner/root/source epoch/sequence/complete-set/Cut/ref/digest。任一漂移、Owner violation、typed-nil、Unavailable或Indeterminate均不返回Projection；historical terminal可存在但current不可见。该入口只读、零Provider、零mutation。

## 2. Continuity ManifestSeal中立只读面

Runtime `ports`定义中立DTO/Reader；Continuity仍是ManifestFact/SealFact语义Owner：

```go
type CheckpointManifestSealReaderV2 interface {
	InspectCheckpointManifestSealV2(
		context.Context,
		InspectCheckpointManifestSealRequestV2,
	) (CheckpointManifestSealProjectionV2, error)
}
```

Reader request必须携`CheckpointManifestSealRefV2.ExactLookup`中的完整Continuity Owner/Scope coordinate、expected Runtime Participant Set digest及sorted current Participant closures。Reader从Continuity immutable Seal复读并exact校验Attempt、Barrier、EffectCut、Manifest、RuntimeClosure映射、Context/Artifact closure与frozen set；Runtime Gateway在Consistency CAS前完成S1/S2。该2.1.0 reference实现已落地并通过ordinary100/race20/full ordinary/race/vet。Runtime Consistency只保存SealRef；不持Continuity Fact Store写口，也不提供第二个Seal create/CAS入口，不解锁Provider、Restore或production root。

Manifest current状态字典固定为`collecting|verified_candidate|partial|indeterminate|rejected`；旧`verified`、自由`ready`或Provider自报状态均不得产生Seal或Runtime Consistency。

## 3. Participant ReservePhase与current Reader

```go
type CheckpointRestoreParticipantGovernancePortV2 interface {
	ReserveCheckpointPhaseV2(
		context.Context,
		ReserveCheckpointParticipantPhaseRequestV2,
	) (CheckpointParticipantPhaseReservationRefV2, error)

	InspectCheckpointPhaseReservationHistoricalV2(
		context.Context,
		CheckpointParticipantPhaseReservationRefV2,
	) (CheckpointParticipantPhaseReservationFactV2, error)

	InspectCheckpointPhaseV2(
		context.Context,
		CheckpointParticipantPhaseRefV2,
	) (CheckpointParticipantPhaseFactV2, error)

	ReadCheckpointDomainResultCurrentV2(
		context.Context,
		CheckpointParticipantDomainResultRefV2,
	) (CheckpointParticipantDomainResultCurrentProjectionV2, error)

	ApplyCheckpointPhaseSettlementV2(
		context.Context,
		ApplyCheckpointPhaseSettlementRequestV2,
	) (CheckpointParticipantPhaseFactV2, error)
}

type CheckpointParticipantPhaseReservationCurrentReaderV2 interface {
	InspectCheckpointParticipantPhaseReservationCurrentV2(
		context.Context,
		CheckpointParticipantPhaseReservationRefV2,
		CheckpointParticipantPhaseV2,
	) (CheckpointParticipantPhaseReservationCurrentProjectionV2, error)
}
```

`ReserveCheckpointPhaseV2`是Participant Owner mutation；Runtime、Application与Continuity不得持其Fact Store。Reservation成功后仍必须依次取得Admission、Review Authorization、Permit、Begin和双Enforcement，才能接触Provider。

prepare Reservation的`PreviousPhase=nil`。只有`prepared`可创建commit或abort，并须携exact prepare Reservation/DomainResult/Evidence/Settlement V5/ApplySettlement闭包；Participant Owner使用共享`(TenantID, AttemptID, ParticipantID)` branch guard线性化二者，仅一个可胜。prepare为`failed|not_applied`时不创建后继，分别进入`incomplete|confirmed_not_applied`输入；`unknown`只Inspect/Reconcile原phase并在deadline进入`indeterminate`。已选分支、历史refs和terminal phase均不可覆盖。

legacy `CheckpointParticipantPort`保持restricted，不得通过Adapter补默认字段实现上述接口。

## 4. Evidence V1 additive Port

```go
type CheckpointRestoreEvidenceGovernancePortV1 interface {
	IssueCheckpointPhaseQualificationV1(
		context.Context,
		IssueCheckpointPhaseQualificationRequestV1,
	) (CheckpointRestoreEvidenceQualificationRefV1, error)

	InspectCheckpointPhaseQualificationHistoricalV1(
		context.Context,
		CheckpointRestoreEvidenceQualificationRefV1,
	) (CheckpointRestoreEvidenceQualificationFactV1, error)

	InspectCheckpointPhaseQualificationCurrentV1(
		context.Context,
		CheckpointRestoreEvidenceQualificationRefV1,
	) (CheckpointRestoreEvidenceQualificationCurrentProjectionV1, error)

	CreateCheckpointPhaseProviderHandoffV1(
		context.Context,
		CreateCheckpointPhaseProviderHandoffRequestV1,
	) (CheckpointRestoreEvidenceProviderHandoffRefV1, error)

	ConsumeCheckpointPhaseEvidenceCurrentV1(
		context.Context,
		ConsumeCheckpointPhaseEvidenceRequestV1,
	) (CheckpointRestoreEvidenceConsumptionRefV1, error)

	ConsumeCheckpointPhaseEvidenceObservationV1(
		context.Context,
		ConsumeCheckpointPhaseEvidenceRequestV1,
	) (CheckpointRestoreEvidenceConsumptionRefV1, error)
}
```

第一波只注册checkpoint prepare/commit/abort；restore phase调用全部unsupported。Issue不推进source cursor；只有current consumption可进入Settlement V5。

## 5. Operation Settlement V5 additive Port

```go
type OperationCheckpointRestoreSettlementGovernancePortV5 interface {
	SettleCheckpointPhaseV5(
		context.Context,
		OperationCheckpointRestoreSettlementSubmissionV5,
	) (OperationCheckpointRestoreSettlementRefV5, error)

	InspectCheckpointPhaseSettlementHistoricalV5(
		context.Context,
		InspectOperationCheckpointRestoreSettlementRequestV5,
	) (OperationCheckpointRestoreSettlementCommitBundleV5, error)

	InspectCheckpointPhaseSettlementCurrentV5(
		context.Context,
		InspectCurrentOperationCheckpointRestoreSettlementRequestV5,
	) (OperationCheckpointRestoreSettlementInspectionV5, error)

	InspectCheckpointPhaseSettlementAssociationV5(
		context.Context,
		OperationSubjectV3,
		OperationCheckpointRestoreSettlementAssociationRefV5,
	) (OperationCheckpointRestoreSettlementAssociationV5, error)

	InspectCheckpointPhaseTerminalGuardV5(
		context.Context,
		OperationSubjectV3,
		OperationCheckpointRestoreTerminalGuardRefV5,
	) (OperationCheckpointRestoreTerminalGuardV5, error)

	InspectCheckpointPhaseTerminalProjectionV5(
		context.Context,
		OperationSubjectV3,
		OperationCheckpointRestoreTerminalProjectionRefV5,
	) (OperationCheckpointRestoreTerminalProjectionV5, error)
}
```

Fact Owner实现必须扩展现有OperationEffectStore同一实例/同一锁，共享`(TenantID, EffectID)` terminal guard；不得新建独立V5 store或sidecar到已terminal V3/V4 Effect。

## 6. Checkpoint Action Gateway Delta

```go
type CheckpointActionGovernancePortV1 interface {
	AdmitCheckpointPhaseV1(
		context.Context,
		AdmitCheckpointPhaseRequestV1,
	) (CheckpointPhaseAdmissionRefV1, error)

	StartCheckpointPhaseV1(
		context.Context,
		StartCheckpointPhaseRequestV1,
	) (CheckpointParticipantPhaseRefV2, error)

	InspectCheckpointPhaseAttemptV1(
		context.Context,
		CheckpointParticipantPhaseRefV2,
	) (CheckpointPhaseInspectionV1, error)

	ReconcileCheckpointPhaseV1(
		context.Context,
		ReconcileCheckpointPhaseRequestV1,
	) (CheckpointPhaseInspectionV1, error)
}
```

`AdmitCheckpointPhaseV1`必须携exact current Reservation、Attempt、Barrier、EffectCut、route和Operation/Effect stable coordinates；`StartCheckpointPhaseV1`另携current Authorization、Permit、Begin、双Enforcement、Evidence qualification/handoff与expected Participant revision。Gateway不得让Application取得Participant Fact Store、Runtime Effect Store或raw Provider。

## 7. Assembly Delta

候选只读/认证对象：

- `CheckpointParticipantDeclarationV2`：participant kind、owner binding、contract/schema、checkpoint phase support、EffectKind、DomainResultKind、required/optional、coverage与Policy；
- `CheckpointParticipantSetCertificationV2`：绑定Generation/Graph/Input、Generation-Binding、BindingSet、ExecutionScope与canonical declaration set；
- `CheckpointParticipantCompiledRouteV2`及current Reader。

Runtime不从Go registry、自报Descriptor或Provider回包发现Participant。Attempt创建后Participant set冻结；hot plug、Binding/Declaration漂移只允许Fail Closed或新Attempt。

## 8. Restore后置shape

可在`ports`保留经联合Review确认的Restore Ref/shape定义，但本计划不提供`RestoreGovernancePortV2`、Restore Action Gateway或current Reader运行实现。任何Restore Issue/Bind/Admission/Begin/Evidence/Settlement/Activate调用都必须明确unsupported且Provider=0。

## 9. 兼容策略

- V3/V4/4.1字段、Validate、canonical和digest完全不变；
- Evidence V3 activation/action matrix不增加checkpoint/restore行；
- V5不能伪装V3/V4，也不能绕过shared terminal guard；
- legacy CheckpointSet/RestoreRequest/Foundation只可restricted test或历史Inspect；
- V2 public code、Conformance及联合Review YES前，所有真实Checkpoint/Restore Provider路径保持unsupported；
- 不选择production backend、Scheduler、RPC、SLA或进程拓扑。

## 10. 依赖与恢复约束

- 所有Gateway入口在backend读取前拒绝nil和typed-nil Fact Store、Reader、Clock/transaction dependency；不得panic或半写；
- lost reply只接受同immutable request、revision单调且位于同一状态分支的合法progressed successor；revision回退、same revision换digest、commit/abort分支切换、历史ref覆盖或terminal重写一律Conflict；
- Continuity ManifestSeal Owner Port与Sandbox `Reservation → Provider Observation → Evidence consumed_current → DomainResultFact → Runtime Settlement V5 → ApplySettlement`链是联合前置，任一尚未冻结时Provider=0；
- Restore仍后置，不得借Checkpoint public Port或legacy接口获得运行资格。

### 10.1 返修后的Evidence Owner分层

`CheckpointRestoreEvidenceGovernancePortV1`仍是调用方面；新增Owner-only `CheckpointRestoreEvidenceFactPortV1.CreateCheckpointPhaseQualificationFactV1`只接受Runtime Gateway构造的`CreateCheckpointPhaseQualificationOwnerRequestV1`。Gateway依赖`CheckpointEvidenceAttemptCurrentReaderV1`、`CheckpointAttemptInputsCurrentReaderV2`与`CheckpointParticipantPhaseReservationCurrentReaderV2`，执行S1/S2 current复读并派生bounded expiry；Fact Store不作为Governance Port暴露。

`CheckpointRestoreEvidenceQualificationRefV1`直接携带exact `Barrier`、`EffectCut`、`Reservation`，其self digest与Fact Validate覆盖全部坐标。historical Inspect证明immutable存在；current Inspect仍不等于Permit、Enforcement、DomainResult或Settlement。

返修增加只读current依赖`CheckpointRunCurrentReaderV2`、
`CheckpointEvidenceExecutionCurrentReaderV1`与
`CheckpointEvidenceSourceCurrentReaderV1`；前者消费Create请求的
`ExpectedRunRevision`，后二者与Evidence Policy current Reader共同闭合
Permit/Admission/Authorization、双Enforcement、Lease/Fence、source epoch/sequence、
schema/payload和TTL。Evidence Governance/Fact ports同时增加Handoff与Consumption
historical exact Inspect；既有current Inspect继续执行Owner-current reread。

Consumption复用既有只读`EvidenceSourceRecordReaderV2`，不新增Ledger Owner或Fake
自证接口。Gateway在Consume写入前执行两轮`InspectRecord(exact Ref)`与
`InspectBySource(exact SourceKey)`，要求两路返回同一通过Owner链摘要校验的Record，
并与Qualification冻结的instance scope、source epoch/sequence、schema/payload、
ExecutionScope和Authority逐项exact；非genesis记录还必须能exact读取其直接前驱。
Reader缺失/typed-nil/Unavailable、S1/S2映射漂移或任一record坐标替换均零写。

`CheckpointFactPortV2.InspectCheckpointAttemptLineageV2`只为create lost-reply
验证逐revision合法successor服务；不能授予写权限或跳过transition校验。

### 10.2 已裁决：EffectCut V2对V5 terminal fail closed

live `OperationSettlementRefV4.DomainResult.Attempt`可让EffectCut V4分支逐字段绑定
`OperationDispatchAttemptRefV3`。但live `OperationCheckpointRestoreSettlementRefV5`
只有Checkpoint Attempt、EffectID与OperationDigest，不能证明IntentDigest、Permit和
Dispatch Attempt ID。独立设计短审已裁决：本轮不新增第二份typed proof，也不原地改
V5 ref/digest；`EffectCutEntryV2.Validate`对V5 terminal一律fail closed/unsupported，
Freeze必须零写。V3/V4保持可用。

后续若需要纳入V5，必须另立设计，从既有V5 CommitBundle由Settlement Owner派生
historical/current terminal projection，并引入新的versioned EffectCut entry及完整版本链；
未获联合设计YES前不得实现该类型。
