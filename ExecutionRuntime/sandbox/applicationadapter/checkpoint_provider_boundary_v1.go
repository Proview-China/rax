package applicationadapter

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/dataplaneadapter"
)

// CheckpointProviderPlanV1 describes exactly one Sandbox checkpoint semantic
// phase (prepare, commit, or abort). Each semantic phase owns a distinct
// Runtime Operation/Attempt and crosses both physical enforcement points.
type CheckpointProviderPlanV1 struct {
	Prepare            runtimeports.EnforceCurrentCheckpointRestoreDispatchRequestV1
	Execute            runtimeports.EnforceCurrentCheckpointRestoreDispatchRequestV1
	CheckpointAttempt  runtimeports.CheckpointAttemptRefV2
	Barrier            runtimeports.CheckpointBarrierLeaseRefV2
	EffectCut          runtimeports.EffectCutRefV2
	Reservation        runtimeports.CheckpointParticipantPhaseReservationRefV2
	Phase              runtimeports.CheckpointParticipantPhaseV2
	DeclaredDelegation runtimeports.ExecutionDelegationRefV2

	PrepareRequestID string
	ExecuteRequestID string
	PayloadSchema    string
	PayloadRevision  uint64
	Payload          dataplaneadapter.ProviderPayloadV1
	NotAfter         time.Time

	QualificationID string
	HandoffID       string
	ConsumptionID   string
	EvidenceScope   runtimeports.CheckpointRestoreEvidenceScopeV1
	EvidenceEvent   runtimeports.EvidenceEventCandidateV2
}

type CheckpointProviderResultV1 struct {
	PrepareCurrent runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1
	PrepareRequest dataplaneadapter.DispatchRequestV1
	PrepareResult  dataplaneadapter.DispatchResponseV1
	ExecuteCurrent runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1
	ExecuteRequest dataplaneadapter.DispatchRequestV1
	ExecuteResult  dataplaneadapter.DispatchResponseV1
	Qualification  runtimeports.CheckpointRestoreEvidenceQualificationRefV1
	Handoff        runtimeports.CheckpointRestoreEvidenceProviderHandoffRefV1
	Record         runtimeports.EvidenceLedgerRecordV2
	Consumption    runtimeports.CheckpointRestoreEvidenceConsumptionRefV1
}

// CheckpointProviderBoundaryV1 is the production actual-point sequence for a
// checkpoint phase. It does not write a checkpoint phase Fact or Runtime
// Settlement; those remain later Owner gates.
type CheckpointProviderBoundaryV1 struct {
	enforcement        runtimeports.CheckpointRestoreDispatchEnforcementGovernancePortV1
	checkpointEvidence runtimeports.CheckpointRestoreEvidenceGovernancePortV1
	ledger             runtimeports.EvidenceGovernancePortV2
	dataplane          DataPlanePortV1
	now                func() time.Time
}

func NewCheckpointProviderBoundaryV1(enforcement runtimeports.CheckpointRestoreDispatchEnforcementGovernancePortV1, checkpointEvidence runtimeports.CheckpointRestoreEvidenceGovernancePortV1, ledger runtimeports.EvidenceGovernancePortV2, dataplane DataPlanePortV1, now func() time.Time) (*CheckpointProviderBoundaryV1, error) {
	if nilLike(enforcement) || nilLike(checkpointEvidence) || nilLike(ledger) || nilLike(dataplane) || nilLike(now) {
		return nil, errors.New("checkpoint Provider boundary requires enforcement, checkpoint Evidence, Evidence ledger, data plane, and clock")
	}
	return &CheckpointProviderBoundaryV1{enforcement: enforcement, checkpointEvidence: checkpointEvidence, ledger: ledger, dataplane: dataplane, now: now}, nil
}

func (b *CheckpointProviderBoundaryV1) ExecuteCheckpointPhaseV1(ctx context.Context, plan CheckpointProviderPlanV1) (CheckpointProviderResultV1, error) {
	if b == nil || nilLike(ctx) {
		return CheckpointProviderResultV1{}, errors.New("checkpoint Provider boundary or context is nil")
	}
	if err := validateCheckpointProviderPlanV1(plan); err != nil {
		return CheckpointProviderResultV1{}, err
	}
	prepareCurrent, err := b.enforcement.EnforceCurrentCheckpointRestoreDispatchV1(ctx, plan.Prepare)
	if err != nil {
		return CheckpointProviderResultV1{}, err
	}
	if err := validateCheckpointEnforcementCurrentV1(prepareCurrent, plan.Prepare, b.now()); err != nil {
		return CheckpointProviderResultV1{}, err
	}
	prepareRequest, err := dataplaneadapter.NewCheckpointDispatchRequestV1(dataplaneadapter.CheckpointDispatchInputV1{
		RequestID: plan.PrepareRequestID, Current: prepareCurrent, PayloadSchema: plan.PayloadSchema,
		PayloadRevision: plan.PayloadRevision, Payload: plan.Payload, RequestedNotAfter: plan.NotAfter,
	})
	if err != nil {
		return CheckpointProviderResultV1{}, err
	}
	prepareResult, err := b.dispatchOrInspectCheckpointV1(ctx, prepareRequest)
	if err != nil {
		return CheckpointProviderResultV1{}, err
	}
	preparedAttempt, err := checkpointPreparedProviderAttemptV1(plan.DeclaredDelegation, prepareCurrent, prepareResult)
	if err != nil {
		return CheckpointProviderResultV1{}, err
	}
	executeRequest := plan.Execute
	executeRequest.Prepare = &prepareCurrent.Phase
	executeRequest.PreparedAttempt = &preparedAttempt
	executeRequest.ExpectedJournalRevision = prepareCurrent.Journal.Revision
	executeCurrent, err := b.enforcement.EnforceCurrentCheckpointRestoreDispatchV1(ctx, executeRequest)
	if err != nil {
		return CheckpointProviderResultV1{}, err
	}
	if err := validateCheckpointEnforcementCurrentV1(executeCurrent, executeRequest, b.now()); err != nil {
		return CheckpointProviderResultV1{}, err
	}
	executeDispatch, err := dataplaneadapter.NewCheckpointDispatchRequestV1(dataplaneadapter.CheckpointDispatchInputV1{
		RequestID: plan.ExecuteRequestID, Current: executeCurrent, PayloadSchema: plan.PayloadSchema,
		PayloadRevision: plan.PayloadRevision, Payload: plan.Payload, RequestedNotAfter: plan.NotAfter,
	})
	if err != nil {
		return CheckpointProviderResultV1{}, err
	}
	executeResult, err := b.dispatchOrInspectCheckpointV1(ctx, executeDispatch)
	if err != nil {
		return CheckpointProviderResultV1{}, err
	}

	scope, candidate, err := checkpointEvidencePayloadV1(plan, prepareCurrent, executeCurrent, executeDispatch, executeResult)
	if err != nil {
		return CheckpointProviderResultV1{}, err
	}
	qualificationRequest := runtimeports.IssueCheckpointPhaseQualificationRequestV1{
		ID: plan.QualificationID, Attempt: plan.CheckpointAttempt, Barrier: plan.Barrier,
		EffectCut: plan.EffectCut, Reservation: plan.Reservation, Phase: plan.Phase, Scope: scope,
	}
	qualification, err := b.checkpointEvidence.IssueCheckpointPhaseQualificationV1(ctx, qualificationRequest)
	if err != nil {
		// The Runtime gateway owns lost-reply recovery because qualification
		// expiry is Owner-derived and cannot be reconstructed by this caller.
		return CheckpointProviderResultV1{}, err
	}
	if _, err := b.checkpointEvidence.InspectCheckpointPhaseQualificationCurrentV1(ctx, qualification); err != nil {
		return CheckpointProviderResultV1{}, err
	}
	handoffRequest := runtimeports.CreateCheckpointPhaseProviderHandoffRequestV1{ID: plan.HandoffID, Qualification: qualification, Attempt: scope.DispatchAttempt, Phase: plan.Phase, ScopeDigest: qualification.ScopeDigest}
	handoff, err := b.checkpointEvidence.CreateCheckpointPhaseProviderHandoffV1(ctx, handoffRequest)
	if err != nil {
		return CheckpointProviderResultV1{}, err
	}
	if _, err := b.checkpointEvidence.InspectCheckpointPhaseProviderHandoffCurrentV1(ctx, handoff); err != nil {
		return CheckpointProviderResultV1{}, err
	}
	record, err := b.ledger.AppendGoverned(ctx, runtimeports.EvidenceAppendRequestV2{Candidate: candidate, ExpectedSourceRevision: candidate.RegistrationRevision})
	if err != nil {
		recovered, inspectErr := b.ledger.InspectGovernedBySource(context.WithoutCancel(ctx), scope.Source)
		candidateDigest, digestErr := candidate.DigestV2()
		if inspectErr != nil || digestErr != nil || recovered.CandidateDigest != candidateDigest || !reflect.DeepEqual(recovered.Candidate, candidate) {
			return CheckpointProviderResultV1{}, err
		}
		record = recovered
	}
	if record.Validate() != nil || !reflect.DeepEqual(record.Candidate, candidate) {
		return CheckpointProviderResultV1{}, errors.New("checkpoint Evidence ledger returned another immutable record")
	}
	consumeRequest := runtimeports.ConsumeCheckpointPhaseEvidenceRequestV1{ID: plan.ConsumptionID, Qualification: qualification, Handoff: handoff, Record: record.Ref, Source: scope.Source}
	consumption, err := b.checkpointEvidence.ConsumeCheckpointPhaseEvidenceCurrentV1(ctx, consumeRequest)
	if err != nil {
		// The Runtime gateway already inspects the exact immutable consume
		// identity on recoverable Owner outcomes.
		return CheckpointProviderResultV1{}, err
	}
	if _, err := b.checkpointEvidence.InspectCheckpointPhaseEvidenceConsumptionCurrentV1(ctx, consumption); err != nil {
		return CheckpointProviderResultV1{}, err
	}
	return CheckpointProviderResultV1{PrepareCurrent: prepareCurrent, PrepareRequest: prepareRequest, PrepareResult: prepareResult, ExecuteCurrent: executeCurrent, ExecuteRequest: executeDispatch, ExecuteResult: executeResult, Qualification: qualification, Handoff: handoff, Record: record, Consumption: consumption}, nil
}

func (b *CheckpointProviderBoundaryV1) dispatchOrInspectCheckpointV1(ctx context.Context, request dataplaneadapter.DispatchRequestV1) (dataplaneadapter.DispatchResponseV1, error) {
	dispatched, dispatchErr := b.dataplane.Dispatch(ctx, request)
	if dispatchErr != nil {
		inspected, inspectErr := b.dataplane.Inspect(context.WithoutCancel(ctx), request)
		if inspectErr != nil {
			return dataplaneadapter.DispatchResponseV1{}, fmt.Errorf("checkpoint Provider outcome is unknown after dispatch error (%v): %w", dispatchErr, inspectErr)
		}
		if err := validateCheckpointProviderInspectionV1(request, inspected, b.now()); err != nil {
			return dataplaneadapter.DispatchResponseV1{}, err
		}
		return inspected, nil
	}
	if err := validateCheckpointProviderInspectionV1(request, dispatched, b.now()); err != nil {
		return dataplaneadapter.DispatchResponseV1{}, err
	}
	// A successful Provider response is still only an Observation. Sandbox
	// independently inspects the exact attempt before it can form Evidence.
	inspected, err := b.dataplane.Inspect(context.WithoutCancel(ctx), request)
	if err != nil {
		return dataplaneadapter.DispatchResponseV1{}, fmt.Errorf("checkpoint Provider response is not independently inspectable: %w", err)
	}
	if err := validateCheckpointProviderInspectionV1(request, inspected, b.now()); err != nil {
		return dataplaneadapter.DispatchResponseV1{}, err
	}
	if !reflect.DeepEqual(dispatched, inspected) {
		return dataplaneadapter.DispatchResponseV1{}, errors.New("checkpoint Provider dispatch and independent Inspect disagree")
	}
	return inspected, nil
}

func validateCheckpointProviderInspectionV1(request dataplaneadapter.DispatchRequestV1, result dataplaneadapter.DispatchResponseV1, now time.Time) error {
	if result.Validate(request) != nil || !result.Accepted || result.ProviderObservation == nil || result.ProviderReceipt == nil || now.IsZero() || !now.Before(time.Unix(0, result.ExpiresUnixNano)) {
		return errors.New("checkpoint Data Plane returned an invalid observation")
	}
	return nil
}

func checkpointEvidencePayloadV1(plan CheckpointProviderPlanV1, prepare, execute runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1, dispatch dataplaneadapter.DispatchRequestV1, result dataplaneadapter.DispatchResponseV1) (runtimeports.CheckpointRestoreEvidenceScopeV1, runtimeports.EvidenceEventCandidateV2, error) {
	payload, err := json.Marshal(result.ProviderObservation)
	if err != nil || len(payload) == 0 || result.ObservationDigest == nil {
		return runtimeports.CheckpointRestoreEvidenceScopeV1{}, runtimeports.EvidenceEventCandidateV2{}, errors.New("checkpoint Provider observation cannot form Evidence")
	}
	scope := plan.EvidenceScope
	scope.PrepareEnforcement = prepare.Phase
	scope.ExecuteEnforcement = execute.Phase
	scope.PayloadDigest = runtimecore.Digest(*result.ObservationDigest)
	scope.PayloadLength = uint64(len(payload))
	if err := scope.Validate(); err != nil {
		return runtimeports.CheckpointRestoreEvidenceScopeV1{}, runtimeports.EvidenceEventCandidateV2{}, err
	}
	candidate := plan.EvidenceEvent
	candidate.LedgerScope = runtimeports.EvidenceLedgerScopeV2{Partition: runtimeports.EvidencePartitionInstance, TenantID: scope.Operation.ExecutionScope.Identity.TenantID, IdentityID: scope.Operation.ExecutionScope.Identity.ID, LineageID: scope.Operation.ExecutionScope.Lineage.ID, InstanceID: scope.Operation.ExecutionScope.Instance.ID}
	candidate.RegistrationID = scope.Source.RegistrationID
	candidate.SourceEpoch = scope.Source.SourceEpoch
	candidate.SourceSequence = scope.Source.SourceSequence
	candidate.TrustClass = runtimeports.EvidenceTrustObservation
	candidate.ExecutionScope = scope.Operation.ExecutionScope
	candidate.Payload = runtimeports.EvidencePayloadRefV2{Schema: scope.PayloadSchema, ContentDigest: scope.PayloadDigest, Revision: scope.PayloadRevision, Length: scope.PayloadLength, Ref: fmt.Sprintf("praxis.sandbox.dataplane/checkpoint/%s", dispatch.Digest)}
	candidate.Authority = scope.Authority
	candidate.ObservedUnixNano = result.ProviderObservation.ObservedUnixNano
	if candidate.Causation == nil {
		candidate.Causation = []runtimeports.EvidenceCausationRefV2{}
	}
	if err := candidate.Validate(); err != nil {
		return runtimeports.CheckpointRestoreEvidenceScopeV1{}, runtimeports.EvidenceEventCandidateV2{}, err
	}
	return scope, candidate, nil
}

func checkpointPreparedProviderAttemptV1(delegation runtimeports.ExecutionDelegationRefV2, current runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1, result dataplaneadapter.DispatchResponseV1) (runtimeports.PreparedProviderAttemptRefV2, error) {
	if result.ProviderAttempt == nil || result.ProviderObservation == nil {
		return runtimeports.PreparedProviderAttemptRefV2{}, errors.New("checkpoint prepare lacks Provider attempt")
	}
	legacy := current.Dispatch.Record.Permit.LegacyPermit
	legacyDigest, err := legacy.DigestV3()
	if err != nil {
		return runtimeports.PreparedProviderAttemptRefV2{}, err
	}
	id, err := runtimeports.DerivePreparedProviderAttemptIDV2(delegation.ID, legacy.ID, legacy.AttemptID)
	if err != nil {
		return runtimeports.PreparedProviderAttemptRefV2{}, err
	}
	return runtimeports.SealPreparedProviderAttemptRefV2(runtimeports.PreparedProviderAttemptRefV2{
		ID: id, Revision: runtimecore.Revision(result.ProviderAttempt.Revision), DeclaredDelegation: delegation,
		OperationDigest: current.Sandbox.OperationDigest, IntentID: legacy.IntentID, IntentRevision: legacy.IntentRevision,
		IntentDigest: legacy.IntentDigest, PermitID: legacy.ID, PermitRevision: legacy.Revision, PermitDigest: legacyDigest,
		AttemptID: legacy.AttemptID, Provider: legacy.Provider, PayloadSchema: legacy.PayloadSchema,
		PayloadDigest: legacy.PayloadDigest, PayloadRevision: legacy.PayloadRevision,
		PreparedUnixNano: result.ProviderObservation.ObservedUnixNano,
		ExpiresUnixNano:  minimumInt64(result.ExpiresUnixNano, current.ExpiresUnixNano),
	})
}

func validateCheckpointEnforcementCurrentV1(current runtimeports.CurrentCheckpointRestoreDispatchEnforcementV1, request runtimeports.EnforceCurrentCheckpointRestoreDispatchRequestV1, now time.Time) error {
	if err := current.Validate(now); err != nil {
		return err
	}
	operationDigest, err := request.Operation.DigestV3()
	if err != nil || current.Phase.OperationDigest != operationDigest || current.Phase.EffectID != request.EffectID || current.Phase.PermitID != request.PermitID || current.Phase.AttemptID != request.AttemptID || current.Phase.Phase != request.Phase || current.Sandbox.Reservation.Ref != request.Reservation || current.Sandbox.ProjectionDigest != request.SandboxProjectionDigest {
		return errors.New("checkpoint Runtime enforcement returned another exact phase")
	}
	return nil
}

func validateCheckpointProviderPlanV1(plan CheckpointProviderPlanV1) error {
	if plan.Prepare.Validate() != nil || plan.CheckpointAttempt.Validate() != nil || plan.Barrier.Validate() != nil || plan.EffectCut.Validate() != nil || plan.Reservation.Validate() != nil || plan.DeclaredDelegation.Validate() != nil {
		return errors.New("checkpoint Provider plan exact references are invalid")
	}
	phaseValid := plan.Phase == runtimeports.CheckpointPhasePrepareV2 || plan.Phase == runtimeports.CheckpointPhaseCommitV2 || plan.Phase == runtimeports.CheckpointPhaseAbortV2
	if !phaseValid || plan.Prepare.Phase != runtimeports.OperationDispatchEnforcementPrepareV4 || plan.Execute.Phase != runtimeports.OperationDispatchEnforcementExecuteV4 || plan.Execute.ExpectedJournalRevision != 0 || plan.Execute.Prepare != nil || plan.Execute.PreparedAttempt != nil || plan.PrepareRequestID == "" || plan.ExecuteRequestID == "" || plan.PrepareRequestID == plan.ExecuteRequestID || plan.PayloadSchema == "" || plan.PayloadRevision == 0 || plan.NotAfter.IsZero() || plan.QualificationID == "" || plan.HandoffID == "" || plan.ConsumptionID == "" || !runtimeports.SameOperationSubjectV3(plan.Prepare.Operation, plan.Execute.Operation) || plan.Prepare.EffectID != plan.Execute.EffectID || plan.Prepare.PermitID != plan.Execute.PermitID || plan.Prepare.AttemptID != plan.Execute.AttemptID || plan.Prepare.Reservation != plan.Reservation || plan.Execute.Reservation != plan.Reservation || plan.CheckpointAttempt.ID != plan.Barrier.AttemptID || plan.EffectCut.Attempt != plan.CheckpointAttempt {
		return errors.New("checkpoint Provider plan mixes phase identities")
	}
	if plan.EvidenceEvent.ContractVersion != runtimeports.EvidenceContractVersionV2 || plan.EvidenceScope.Operation.Validate() != nil || !runtimeports.SameOperationSubjectV3(plan.EvidenceScope.Operation, plan.Prepare.Operation) || plan.EvidenceScope.EffectID != plan.Prepare.EffectID || plan.EvidenceScope.DispatchAttempt.AttemptID != plan.Prepare.AttemptID || plan.EvidenceScope.Source.Validate() != nil {
		return errors.New("checkpoint Provider Evidence plan is incomplete")
	}
	return nil
}
