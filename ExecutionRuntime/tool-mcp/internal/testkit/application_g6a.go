package testkit

import (
	"time"

	applicationcontract "github.com/Proview-China/rax/ExecutionRuntime/application/contract"
	modelinvoker "github.com/Proview-China/rax/ExecutionRuntime/model-invoker"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	toolcontract "github.com/Proview-China/rax/ExecutionRuntime/tool-mcp/contract"
)

type ApplicationG6AFixtureV1 struct {
	Request     applicationcontract.SingleCallToolActionRequestV1
	Projection  modelinvoker.ToolCallCandidateObservationProjectionV1
	ToolResult  toolcontract.ToolResultV2
	Inspection  runtimeports.OperationInspectionSettlementRefV4
	Association runtimeports.OperationSettlementEvidenceAssociationV4
	Provider    runtimeports.ProviderBindingRefV2
}

func ApplicationG6AFixture(now time.Time) ApplicationG6AFixtureV1 {
	projection := ModelProjection(1)
	boundary := BoundaryFixture(now)
	prepare := boundary.Enforcement
	prepare.Phase = runtimeports.OperationDispatchEnforcementPrepareV4
	prepare.JournalRevision = 1
	prepare.ReceiptDigest = Digest("prepare-receipt-app")
	prepare.PrepareReceiptDigest = ""
	prepare.PreparedAttemptDigest = ""
	if err := prepare.Validate(); err != nil {
		panic(err)
	}
	provider := ProviderBinding()
	schema := Schema("application-result")
	runtimeDomain := runtimeports.OperationSettlementDomainResultFactRefV4{Owner: provider, Kind: "praxis.tool/domain-result", ID: "tool-domain-result-app", Revision: 1, Digest: Digest("tool-domain-result-app"), TenantID: boundary.Operation.ExecutionScope.Identity.TenantID, EffectID: boundary.Attempt.EffectID, EffectRevision: boundary.Attempt.IntentRevision, Operation: boundary.Operation, OperationDigest: boundary.Attempt.OperationDigest, Attempt: boundary.Attempt, Schema: schema, PayloadDigest: Digest("tool-payload-app"), PayloadRevision: 1, AuthoritativeTime: now.Add(-time.Second).UnixNano()}
	makeBinding := func(phase runtimeports.OperationDispatchEnforcementPhaseV4, phaseRef runtimeports.OperationDispatchEnforcementPhaseRefV4, sequence uint64) runtimeports.OperationSettlementEvidenceBindingV4 {
		label := string(phase)
		record := runtimeports.OperationScopeEvidenceRecordRefV3{LedgerScopeDigest: Digest("ledger-app-" + label), Sequence: sequence, RecordDigest: Digest("record-app-" + label)}
		issued := runtimeports.OperationScopeEvidenceQualificationRefV3{ID: "qualification-app-" + label, Revision: 1, Digest: Digest("qualification-issued-app-" + label), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}
		final := issued
		final.Revision = 2
		final.Digest = Digest("qualification-final-app-" + label)
		return runtimeports.OperationSettlementEvidenceBindingV4{Phase: phase, Consumption: runtimeports.OperationScopeEvidenceConsumptionRefV3{ID: "consumption-app-" + label, Revision: 1, Digest: Digest("consumption-app-" + label), Record: record}, IssuedQualification: issued, FinalQualification: final, Record: record, CandidateDigest: Digest("candidate-app-" + label), Handoff: runtimeports.OperationScopeEvidenceProviderHandoffRefV3{ID: "handoff-app-" + label, Revision: 1, Digest: Digest("handoff-app-" + label), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}, Attempt: boundary.Attempt, EnforcementPhase: phaseRef, OperationScopeDigest: Digest("scope-app-" + label)}
	}
	evidence := []runtimeports.OperationSettlementEvidenceBindingV4{makeBinding(runtimeports.OperationDispatchEnforcementPrepareV4, prepare, 1), makeBinding(runtimeports.OperationDispatchEnforcementExecuteV4, boundary.Enforcement, 2)}
	scopeSet, err := runtimeports.DigestOperationSettlementScopeSetV4(evidence)
	if err != nil {
		panic(err)
	}
	owner := SettlementOwner()
	submission, err := runtimeports.SealOperationSettlementSubmissionV4(runtimeports.OperationSettlementSubmissionV4{ID: "runtime-settlement-app", TenantID: runtimeDomain.TenantID, Operation: boundary.Operation, OperationDigest: boundary.Attempt.OperationDigest, OperationScopeDigest: scopeSet, EffectID: boundary.Attempt.EffectID, ExpectedEffectRevision: 3, Owner: owner, DomainResult: runtimeDomain, Evidence: evidence, IdempotencyKey: "settlement-app", ConflictDomain: Digest("conflict-app"), SettledUnixNano: now.Add(-time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	fact, err := runtimeports.SealOperationSettlementFactV4(runtimeports.OperationSettlementFactV4{Submission: submission})
	if err != nil {
		panic(err)
	}
	settlement := fact.RefV4()
	association, err := runtimeports.SealOperationSettlementEvidenceAssociationV4(runtimeports.OperationSettlementEvidenceAssociationV4{ID: "association-app", Settlement: settlement, Prepare: evidence[0], Execute: evidence[1]})
	if err != nil {
		panic(err)
	}
	guard, err := runtimeports.SealOperationSettlementTerminalGuardV4(runtimeports.OperationSettlementTerminalGuardV4{ID: "guard-app", TenantID: runtimeDomain.TenantID, OperationDigest: runtimeDomain.OperationDigest, EffectID: runtimeDomain.EffectID, Settlement: settlement})
	if err != nil {
		panic(err)
	}
	terminal, err := runtimeports.SealOperationSettlementTerminalProjectionV4(runtimeports.OperationSettlementTerminalProjectionV4{ID: "terminal-app", TenantID: runtimeDomain.TenantID, OperationDigest: runtimeDomain.OperationDigest, EffectID: runtimeDomain.EffectID, Settlement: settlement, Association: association.RefV4(), Guard: guard.RefV4(), DomainResult: runtimeDomain})
	if err != nil {
		panic(err)
	}
	inspection, err := runtimeports.SealOperationInspectionSettlementRefV4(runtimeports.OperationInspectionSettlementRefV4{Settlement: settlement, Association: association.RefV4(), Guard: guard.RefV4(), Projection: terminal.RefV4(), DomainResult: runtimeDomain, EffectFactRevision: 4, Owner: owner, CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: now.Add(8 * time.Second).UnixNano()}, now)
	if err != nil {
		panic(err)
	}
	request := applicationRequestV1(now, projection, boundary.Operation, provider, schema)
	actionRef := toolcontract.ObjectRef{ID: "action-app", Revision: 1, Digest: Digest("action-app")}
	reservationRef := toolcontract.ObjectRef{ID: "reservation-app", Revision: 1, Digest: Digest("reservation-app")}
	domainRef := toolcontract.ObjectRef{ID: runtimeDomain.ID, Revision: runtimeDomain.Revision, Digest: runtimeDomain.Digest}
	applyRef := toolcontract.ObjectRef{ID: "apply-app", Revision: 1, Digest: Digest("apply-app")}
	toolResult, err := toolcontract.SealToolResultV2(toolcontract.ToolResultV2{ID: "tool-result-app", Action: actionRef, Reservation: reservationRef, DomainResult: domainRef, Apply: applyRef, Inspection: inspection, Outcome: toolcontract.ToolOutcomeSucceededV2, Disposition: toolcontract.ToolDispositionConfirmedAppliedV2, Schema: schema, PayloadDigest: runtimeDomain.PayloadDigest, PayloadRevision: runtimeDomain.PayloadRevision, FinalizedUnixNano: now.Add(-time.Nanosecond).UnixNano()})
	if err != nil {
		panic(err)
	}
	return ApplicationG6AFixtureV1{request, projection, toolResult, inspection, association, provider}
}

func applicationRequestV1(now time.Time, projection modelinvoker.ToolCallCandidateObservationProjectionV1, operation runtimeports.OperationSubjectV3, provider runtimeports.ProviderBindingRefV2, schema runtimeports.SchemaRefV2) applicationcontract.SingleCallToolActionRequestV1 {
	d := Digest
	canonicalArgumentsDigest := core.DigestBytes(projection.Observation.Calls[0].CanonicalArguments)
	sessionDigest := d("session-source-app")
	turnDigest := d("turn-source-app")
	request, err := applicationcontract.SealSingleCallToolActionRequestV1(applicationcontract.SingleCallToolActionRequestV1{Workflow: applicationcontract.SingleCallWorkflowCoordinateV1{WorkflowContractVersion: applicationcontract.WorkflowContractVersionV2, PlanID: "plan-app", PlanRevision: 1, PlanDigest: d("plan-app"), JournalID: "journal-app", JournalRevision: 1, JournalDigest: d("journal-app"), StepID: "step-app", StepKind: applicationcontract.SingleCallToolActionStepKindV1, StepDescriptor: applicationcontract.StepDescriptorRefV2{Kind: applicationcontract.SingleCallToolActionStepKindV1, Revision: 1, Digest: d("step-app"), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}, WorkflowAttempt: 1}, ExecutionScope: operation.ExecutionScope, Run: applicationcontract.SingleCallRunCoordinateV1{RunID: operation.RunID, Revision: 1, Digest: d("run-app")}, Session: applicationcontract.SingleCallSessionCoordinateV1{ID: "session-app", Revision: 1, Digest: d("session-app"), Phase: applicationcontract.SingleCallSessionWaitingActionV1, CheckedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}, SessionApplicabilitySource: applicationcontract.SingleCallSessionApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallSessionSourceKindV1, ID: "session:" + string(sessionDigest), Revision: 1, Digest: sessionDigest}, Turn: applicationcontract.SingleCallTurnCoordinateV1{ID: "turn-app", Ordinal: 1, Revision: 1, Digest: d("turn-app")}, TurnApplicabilitySource: applicationcontract.SingleCallTurnApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallTurnSourceKindV1, ID: "turn:" + string(turnDigest), Revision: 1, Digest: turnDigest}, PendingAction: applicationcontract.SingleCallPendingActionCoordinateV1{ActionRef: "pending-app", RequestDigest: d("pending-app"), Capability: "praxis.tool/execute", PayloadSchema: schema, PayloadDigest: canonicalArgumentsDigest, SourceCandidateID: "source-app", SourceCandidateRevision: 1, SourceCandidateDigest: d("source-app"), ProjectionDigest: d("pending-projection-app")}, Observation: applicationcontract.SingleCallObservationCoordinateV1{ProjectionContractVersion: projection.ContractVersion, ProjectionID: projection.Ref.ID, ProjectionRevision: projection.Ref.Revision, ProjectionDigest: projection.Ref.Digest, InvocationID: projection.Ref.InvocationID, InvocationDigest: projection.Ref.InvocationDigest, ObservationDigest: projection.Ref.ObservationDigest, SourceResponseID: projection.Ref.Source.ResponseID, SourceSequence: projection.Ref.Source.SourceSequence, Evidence: runtimeports.EvidenceRecordRefV2{LedgerScopeDigest: d("model-ledger-app"), Sequence: 1, RecordDigest: d("model-record-app")}, CallCount: 1}, Assembly: applicationcontract.SingleCallAssemblyCoordinateV1{GenerationID: "generation-app", GenerationRevision: 1, GenerationDigest: d("generation-app"), BindingAssociation: runtimeports.GenerationBindingAssociationRefV1{ID: "generation-binding-app", Revision: 1, Digest: d("generation-binding-app")}, ToolProvider: provider}, Authority: runtimeports.AuthorityBindingRefV2{Ref: "authority-app", Revision: 1, Digest: d("authority-app"), Epoch: operation.ExecutionScope.AuthorityEpoch}, ParentFrame: applicationcontract.SingleCallParentFrameCoordinateV1{FrameID: "frame-app", FrameRevision: 1, FrameDigest: d("frame-app"), GenerationID: "generation-app", GenerationRevision: 1, GenerationDigest: d("generation-app"), ExpiresUnixNano: now.Add(20 * time.Second).UnixNano()}, ParentFrameApplicabilitySource: applicationcontract.SingleCallParentFrameApplicabilitySourceCoordinateV1{Kind: applicationcontract.SingleCallParentFrameSourceKindV1, ID: "frame-app", Revision: 1, Digest: d("frame-source-app")}, CreatedUnixNano: now.Add(-time.Second).UnixNano(), ExpiresUnixNano: now.Add(7 * time.Second).UnixNano()})
	if err != nil {
		panic(err)
	}
	return request
}

func DigestString(label string) core.Digest { return Digest(label) }
