package control

import (
	"context"
	"strings"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

// OperationObservationGovernanceGatewayV3 records a provider Observation and
// only then advances the Effect to dispatched. It cannot settle the Effect.
type OperationObservationGovernanceGatewayV3 struct {
	Effects      OperationEffectFactPortV3
	Observations ports.ProviderAttemptObservationFactPortV2
	Delegations  ports.ExecutionDelegationFactPortV2
	Current      ports.OperationGovernanceCurrentReaderV3
	Dispatch     ports.OperationDispatchCurrentReaderV3
	Evidence     ports.EvidenceSourceRecordReaderV2
	Clock        func() time.Time
}

func (g OperationObservationGovernanceGatewayV3) RecordGovernedProviderObservationV3(ctx context.Context, request ports.RecordGovernedProviderObservationRequestV2) (ports.ProviderAttemptObservationRefV2, error) {
	if g.Effects == nil || g.Observations == nil || g.Delegations == nil || g.Current == nil || g.Dispatch == nil || g.Evidence == nil || g.Clock == nil {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "observation gateway requires Effect, Observation, Delegation, current readers and clock")
	}
	if err := request.Intent.Validate(); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if err := request.Permit.Validate(); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if err := request.Fence.Validate(); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if err := request.Attempt.ValidatePrepared(); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if err := request.Observation.ValidateAgainstPrepared(request.Attempt.Prepared); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if request.Observation.Delegation != request.Attempt.Delegation {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider Observation binds another delegation")
	}
	execute := ports.ExecutePreparedRequestV2{Delegation: request.Attempt.Delegation, Prepared: request.Attempt.Prepared, Enforcement: request.Attempt.Enforcement, Intent: request.Intent, Permit: request.Permit, Fence: request.Fence}
	if err := execute.Validate(); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	now := g.Clock()
	if now.IsZero() || request.Observation.ObservedUnixNano > now.UnixNano() || request.Observation.ObservedUnixNano < request.Attempt.Prepared.PreparedUnixNano {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "provider Observation clock is zero, future or regressed")
	}
	record, err := g.Evidence.InspectBySource(ctx, ports.EvidenceSourceKeyV2{RegistrationID: request.Observation.SourceRegistrationID, SourceEpoch: request.Observation.SourceEpoch, SourceSequence: request.Observation.SourceSequence})
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if err := ValidateEvidenceLedgerRecordV2(record); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	producer := ports.EvidenceProducerBindingRefV2(request.Attempt.Prepared.Provider)
	if record.Ref != request.Observation.Evidence || record.Candidate.RegistrationID != request.Observation.SourceRegistrationID || record.Candidate.SourceEpoch != request.Observation.SourceEpoch || record.Candidate.SourceSequence != request.Observation.SourceSequence || record.Candidate.EventID != request.Observation.ProviderOperationRef || record.Candidate.CorrelationID != request.Attempt.Prepared.ID || !ports.SameExecutionScopeV2(record.Candidate.ExecutionScope, request.Intent.Operation.ExecutionScope) || record.Candidate.Producer != producer || record.Candidate.Payload.Schema.Key() != request.Observation.Payload.Schema.Key() || record.Candidate.Payload.ContentDigest != request.Observation.Payload.ContentDigest || record.Candidate.Payload.Revision != request.Observation.PayloadRevision || len(record.Candidate.Causation) != 1 || record.Candidate.Causation[0].LedgerScopeDigest != record.Ref.LedgerScopeDigest || record.Candidate.Causation[0].EventID != request.Observation.Delegation.ID {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider Observation Evidence does not bind exact source, payload, operation and causation")
	}
	byRef, err := g.Evidence.InspectRecord(ctx, request.Observation.Evidence)
	if err != nil || byRef.Ref != record.Ref {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "provider Observation source key and record ref resolve differently")
	}
	delegation, err := g.Delegations.InspectExecutionDelegationV2(ctx, request.Attempt.Delegation.ID)
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if err := execute.ValidateAgainstDelegation(delegation, now); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	dispatch, err := g.Dispatch.InspectOperationDispatch(ctx, request.Intent.Operation, request.Permit.ID, request.Attempt.Delegation.ID)
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	current, err := g.Current.InspectOperationGovernance(ctx, request.Intent.Operation)
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if err := dispatch.ValidateForExecute(execute, current, now); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	stored, err := g.Observations.CreateProviderAttemptObservationV2(ctx, request.Observation)
	if err != nil {
		if !recoverableOperationWriteErrorV3(err) {
			return ports.ProviderAttemptObservationRefV2{}, err
		}
		stored, err = g.Observations.InspectProviderAttemptObservationV2(context.WithoutCancel(ctx), request.Observation.Delegation, request.Observation.Prepared.ID)
		if err != nil {
			return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "cannot prove provider Observation write")
		}
	}
	observationRef, err := stored.RefV2()
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	effect, err := g.Effects.InspectOperationEffectV3(ctx, request.Intent.Operation, request.Intent.ID)
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if effect.State == OperationEffectDispatchedV3 && effect.DispatchReceipt != nil && effect.DispatchReceipt.Observation == observationRef {
		return observationRef, nil
	}
	if effect.State != OperationEffectDispatchIntentV3 || effect.IntentDigest != request.Attempt.Admission.IntentDigest || effect.Revision != request.Attempt.Admission.FactRevision+1 {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorConflict, core.ReasonEffectStateConflict, "provider Observation cannot advance the current Effect")
	}
	receipt := OperationProviderDispatchReceiptV3{
		PermitID: request.Permit.ID, PermitRevision: request.Permit.Revision, PermitDigest: request.Attempt.PermitDigest, AttemptID: request.Permit.AttemptID,
		IntentID: request.Intent.ID, IntentRevision: request.Intent.Revision, IntentDigest: request.Attempt.Admission.IntentDigest,
		OperationDigest: request.Attempt.Admission.OperationDigest, Provider: request.Permit.Provider,
		PayloadSchema: request.Intent.Payload.Schema, PayloadDigest: request.Intent.Payload.ContentDigest, PayloadRevision: request.Intent.PayloadRevision,
		Delegation: request.Attempt.Delegation, Prepared: request.Attempt.Prepared, Enforcement: request.Attempt.Enforcement,
		Observation: observationRef, ProviderOperationRef: observationRef.ProviderOperationRef, ObservationDigest: observationRef.Digest, ObservedUnixNano: observationRef.ObservedUnixNano,
	}
	if err := receipt.Validate(); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	next := effect
	next.State = OperationEffectDispatchedV3
	next.Revision++
	next.DispatchReceipt = &receipt
	next.UpdatedUnixNano = now.UnixNano()
	committed, err := g.Effects.CompareAndSwapOperationEffectV3(ctx, request.Intent.Operation, OperationEffectCASRequestV3{ExpectedRevision: effect.Revision, Next: next})
	if err != nil {
		if !recoverableOperationWriteErrorV3(err) {
			return ports.ProviderAttemptObservationRefV2{}, err
		}
		committed, err = g.Effects.InspectOperationEffectV3(context.WithoutCancel(ctx), request.Intent.Operation, request.Intent.ID)
		if err != nil || committed.State != OperationEffectDispatchedV3 || committed.DispatchReceipt == nil || committed.DispatchReceipt.Observation != observationRef {
			return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "cannot prove provider Observation association")
		}
	}
	return observationRef, nil
}

func (g OperationObservationGovernanceGatewayV3) InspectGovernedProviderObservationV3(ctx context.Context, delegation ports.ExecutionDelegationRefV2, preparedID string) (ports.ProviderAttemptObservationRefV2, error) {
	if g.Observations == nil {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonComponentMissing, "Observation owner is required")
	}
	if err := delegation.Validate(); err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	if strings.TrimSpace(preparedID) == "" {
		return ports.ProviderAttemptObservationRefV2{}, core.NewError(core.ErrorInvalidArgument, core.ReasonInvalidReference, "provider Observation inspection requires a prepared attempt ID")
	}
	observation, err := g.Observations.InspectProviderAttemptObservationV2(ctx, delegation, preparedID)
	if err != nil {
		return ports.ProviderAttemptObservationRefV2{}, err
	}
	return observation.RefV2()
}
