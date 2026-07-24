package kernel

import (
	"context"
	"time"

	"github.com/Proview-China/rax/ExecutionRuntime/runtime/control"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	"github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
)

const checkpointEvidenceMaximumQualificationTTL = ports.MaxCheckpointRestoreEvidenceQualificationTTLV1

// CheckpointRestoreEvidenceGatewayV1 is the only application-facing issuer of
// a checkpoint Evidence qualification. The Evidence Owner only accepts the
// derived Owner request produced after two exact currentness reads.
type CheckpointRestoreEvidenceGatewayV1 struct {
	Facts        ports.CheckpointRestoreEvidenceFactPortV1
	Checkpoints  ports.CheckpointEvidenceAttemptCurrentReaderV1
	Inputs       ports.CheckpointAttemptInputsCurrentReaderV2
	Reservations ports.CheckpointParticipantPhaseReservationCurrentReaderV2
	Execution    ports.CheckpointEvidenceExecutionCurrentReaderV1
	Policies     ports.ControlledOperationEvidencePolicyCurrentReaderV2
	Sources      ports.CheckpointEvidenceSourceCurrentReaderV1
	Records      ports.EvidenceSourceRecordReaderV2
	Clock        func() time.Time
}

func (g CheckpointRestoreEvidenceGatewayV1) IssueCheckpointPhaseQualificationV1(ctx context.Context, request ports.IssueCheckpointPhaseQualificationRequestV1) (ports.CheckpointRestoreEvidenceQualificationRefV1, error) {
	for _, dependency := range []struct {
		value any
		name  string
	}{{g.Facts, "checkpoint Evidence Fact Owner"}, {g.Checkpoints, "checkpoint current Reader"}, {g.Inputs, "checkpoint inputs current Reader"}, {g.Reservations, "checkpoint reservation current Reader"}, {g.Execution, "checkpoint Permit and Enforcement current Reader"}, {g.Policies, "checkpoint Evidence Policy current Reader"}, {g.Sources, "checkpoint Evidence source current Reader"}, {g.Clock, "checkpoint Evidence clock"}} {
		if err := requireCheckpointDependencyV2(dependency.value, dependency.name); err != nil {
			return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
		}
	}
	now, err := g.evidenceNowV1(time.Time{})
	if err != nil {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
	}
	if err := request.Validate(now); err != nil {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
	}
	first, err := g.readCheckpointEvidenceQualificationInputsV1(ctx, request, now, 0)
	if err != nil {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
	}
	fresh, err := g.evidenceNowV1(now)
	if err != nil {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
	}
	second, err := g.readCheckpointEvidenceQualificationInputsV1(ctx, request, fresh, 0)
	if err != nil {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
	}
	if first.attemptDigest != second.attemptDigest || first.barrierDigest != second.barrierDigest || first.cutDigest != second.cutDigest || first.inputsDigest != second.inputsDigest || first.reservationDigest != second.reservationDigest || first.prepareDigest != second.prepareDigest || first.executeDigest != second.executeDigest || first.policyDigest != second.policyDigest || first.sourceDigest != second.sourceDigest || first.expiresUnixNano != second.expiresUnixNano {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence qualification inputs changed before Owner create")
	}
	ownerRequest := ports.CreateCheckpointPhaseQualificationOwnerRequestV1{Request: request, DerivedExpiresUnixNano: second.expiresUnixNano}
	created, err := g.Facts.CreateCheckpointPhaseQualificationFactV1(ctx, ownerRequest)
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
		}
		expected, deriveErr := checkpointEvidenceQualificationRefV1(request, second.expiresUnixNano)
		if deriveErr != nil {
			return ports.CheckpointRestoreEvidenceQualificationRefV1{}, deriveErr
		}
		fact, inspectErr := g.Facts.InspectCheckpointPhaseQualificationHistoricalV1(context.WithoutCancel(ctx), expected)
		if inspectErr != nil || fact.Validate() != nil || fact.Ref != expected {
			return ports.CheckpointRestoreEvidenceQualificationRefV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "checkpoint Evidence qualification outcome cannot be inspected")
		}
		return fact.Ref, nil
	}
	expected, err := checkpointEvidenceQualificationRefV1(request, second.expiresUnixNano)
	if err != nil || created != expected {
		if err != nil {
			return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
		}
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence Owner returned another qualification")
	}
	return created, nil
}

type checkpointEvidenceQualificationInputsV1 struct {
	attemptDigest     core.Digest
	barrierDigest     core.Digest
	cutDigest         core.Digest
	inputsDigest      core.Digest
	reservationDigest core.Digest
	prepareDigest     core.Digest
	executeDigest     core.Digest
	policyDigest      core.Digest
	sourceDigest      core.Digest
	expiresUnixNano   int64
}

func (g CheckpointRestoreEvidenceGatewayV1) readCheckpointEvidenceQualificationInputsV1(ctx context.Context, request ports.IssueCheckpointPhaseQualificationRequestV1, now time.Time, frozenExpiry int64) (checkpointEvidenceQualificationInputsV1, error) {
	checkpoint, err := g.Checkpoints.InspectCheckpointEvidenceAttemptCurrentV1(ctx, request.Attempt, request.Barrier, request.EffectCut)
	if err != nil {
		return checkpointEvidenceQualificationInputsV1{}, err
	}
	if err := checkpoint.Validate(now); err != nil {
		return checkpointEvidenceQualificationInputsV1{}, err
	}
	if checkpoint.Attempt != request.Attempt || checkpoint.Barrier != request.Barrier || checkpoint.EffectCut != request.EffectCut {
		return checkpointEvidenceQualificationInputsV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence qualification does not bind current Attempt, Barrier and Effect Cut")
	}
	inputs, err := g.Inputs.InspectCheckpointAttemptInputsCurrentV2(ctx, request.Attempt)
	if err != nil || inputs.Validate(now) != nil {
		if err != nil {
			return checkpointEvidenceQualificationInputsV1{}, err
		}
		return checkpointEvidenceQualificationInputsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint Evidence Attempt inputs are not current")
	}
	reservation, err := g.Reservations.InspectCheckpointParticipantPhaseReservationCurrentV2(ctx, request.Reservation, request.Phase)
	if err != nil || reservation.Validate(now) != nil {
		if err != nil {
			return checkpointEvidenceQualificationInputsV1{}, err
		}
		return checkpointEvidenceQualificationInputsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonBindingExpired, "checkpoint Evidence reservation is not current")
	}
	execution, err := g.Execution.InspectCheckpointEvidenceExecutionCurrentV1(ctx, request.Scope.Operation, request.Scope.EffectID, request.Scope.DispatchAttempt)
	if err != nil || execution.Validate(now) != nil || !ports.SameOperationSubjectV3(execution.Operation, request.Scope.Operation) || execution.EffectID != request.Scope.EffectID || execution.DispatchAttempt != request.Scope.DispatchAttempt || execution.PrepareEnforcement != request.Scope.PrepareEnforcement || execution.ExecuteEnforcement != request.Scope.ExecuteEnforcement {
		if err != nil {
			return checkpointEvidenceQualificationInputsV1{}, err
		}
		return checkpointEvidenceQualificationInputsV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Permit and Enforcement closure is not exact current")
	}
	policy, err := g.Policies.InspectCurrentControlledOperationEvidencePolicyV2(ctx, request.Scope.EvidencePolicy)
	if err != nil || policy.Validate() != nil || policy.RefV3() != request.Scope.EvidencePolicy || policy.State != ports.OperationScopeEvidencePolicyActiveV3 || policy.OperationKind != request.Scope.Operation.Kind || policy.EffectKind != request.Scope.EffectKind || policy.ExpectedSchema != request.Scope.PayloadSchema || request.Scope.PayloadLength > policy.MaximumPayloadBytes || !checkpointEvidencePolicyAllowsBothPhasesV1(policy) || !now.Before(time.Unix(0, policy.ExpiresUnixNano)) {
		if err != nil {
			return checkpointEvidenceQualificationInputsV1{}, err
		}
		return checkpointEvidenceQualificationInputsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceTrustInvalid, "checkpoint Evidence Policy is not exact current")
	}
	source, err := g.Sources.InspectCheckpointEvidenceSourceCurrentV1(ctx, request.Scope.Source)
	if err != nil || source.Validate(now) != nil || source.Source != request.Scope.Source || source.Policy != request.Scope.EvidencePolicy || source.Schema != request.Scope.PayloadSchema {
		if err != nil {
			return checkpointEvidenceQualificationInputsV1{}, err
		}
		return checkpointEvidenceQualificationInputsV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceSourceStale, "checkpoint Evidence source is not exact current")
	}
	scopeDigest, digestErr := request.Scope.DigestV1()
	if digestErr != nil || reservation.Ref != request.Reservation || reservation.Attempt != request.Attempt || reservation.Barrier != request.Barrier || reservation.EffectCut != request.EffectCut || reservation.Phase != request.Phase || !ports.SameOperationSubjectV3(reservation.Operation, request.Scope.Operation) || reservation.OperationDigest != request.Scope.OperationDigest || reservation.EffectID != request.Scope.EffectID || reservation.EffectKind != request.Scope.EffectKind || reservation.IntentDigest != request.Scope.IntentDigest || reservation.Generation != request.Scope.Assembly || inputs.GenerationBinding != request.Scope.Assembly || inputs.AuthorityRef != request.Scope.Authority || scopeDigest.Validate() != nil || execution.EffectRevision != request.Scope.EffectRevision || execution.PermitID != request.Scope.PermitID || execution.PermitFactRevision != request.Scope.PermitFactRevision || execution.PermitDigest != request.Scope.PermitDigest || execution.AuthorizedAdmissionDigest != request.Scope.AuthorizedAdmissionDigest || execution.Authorization != request.Scope.Authorization || execution.SandboxAttempt != request.Scope.SandboxAttempt || execution.SandboxProjectionDigest != request.Scope.SandboxProjectionDigest || execution.SandboxLease != request.Scope.SandboxLease || execution.FenceEpoch != request.Scope.FenceEpoch || execution.PayloadSchema != request.Scope.PayloadSchema || execution.PayloadDigest != request.Scope.PayloadDigest || execution.PayloadRevision != request.Scope.PayloadRevision {
		return checkpointEvidenceQualificationInputsV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence scope differs from current reservation or Attempt inputs")
	}
	maximumTTL := checkpointEvidenceMaximumQualificationTTL
	if policy.MaximumQualificationTTL < maximumTTL {
		maximumTTL = policy.MaximumQualificationTTL
	}
	expires := now.Add(maximumTTL).UnixNano()
	for _, candidate := range []int64{checkpoint.ExpiresUnixNano, reservation.ExpiresUnixNano, inputs.ExpiresUnixNano, execution.ExpiresUnixNano, policy.ExpiresUnixNano, source.ExpiresUnixNano} {
		if candidate > 0 && candidate < expires {
			expires = candidate
		}
	}
	if frozenExpiry > 0 {
		if !now.Before(time.Unix(0, frozenExpiry)) || frozenExpiry > expires {
			return checkpointEvidenceQualificationInputsV1{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceUnavailable, "checkpoint Evidence frozen expiry exceeds a current Owner bound")
		}
		expires = frozenExpiry
	} else if !now.Before(time.Unix(0, expires)) || (request.ExpiresUnixNano != 0 && request.ExpiresUnixNano != expires) {
		return checkpointEvidenceQualificationInputsV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence expiry is not the exact current minimum")
	}
	return checkpointEvidenceQualificationInputsV1{attemptDigest: checkpoint.ProjectionDigest, barrierDigest: checkpoint.Barrier.Digest, cutDigest: checkpoint.EffectCut.Digest, inputsDigest: inputs.ProjectionDigest, reservationDigest: reservation.ProjectionDigest, prepareDigest: execution.PrepareEnforcement.ReceiptDigest, executeDigest: execution.ExecuteEnforcement.ReceiptDigest, policyDigest: policy.Digest, sourceDigest: source.ProjectionDigest, expiresUnixNano: expires}, nil
}

func checkpointEvidencePolicyAllowsBothPhasesV1(policy ports.OperationScopeEvidencePolicyFactV3) bool {
	prepare, execute := false, false
	for _, phase := range policy.AllowedPhases {
		prepare = prepare || phase == ports.OperationDispatchEnforcementPrepareV4
		execute = execute || phase == ports.OperationDispatchEnforcementExecuteV4
	}
	return prepare && execute
}

func checkpointEvidenceQualificationRefV1(request ports.IssueCheckpointPhaseQualificationRequestV1, expires int64) (ports.CheckpointRestoreEvidenceQualificationRefV1, error) {
	scopeDigest, err := request.Scope.DigestV1()
	if err != nil {
		return ports.CheckpointRestoreEvidenceQualificationRefV1{}, err
	}
	ref := ports.CheckpointRestoreEvidenceQualificationRefV1{ID: request.ID, Revision: 1, Attempt: request.Attempt, Barrier: request.Barrier, EffectCut: request.EffectCut, Reservation: request.Reservation, Phase: request.Phase, ScopeDigest: scopeDigest, ExpiresUnixNano: expires}
	ref.Digest, err = ref.DigestV1()
	return ref, err
}

func (g CheckpointRestoreEvidenceGatewayV1) InspectCheckpointPhaseQualificationHistoricalV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceQualificationRefV1) (ports.CheckpointRestoreEvidenceQualificationFactV1, error) {
	if err := ref.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceQualificationFactV1{}, err
	}
	fact, err := g.Facts.InspectCheckpointPhaseQualificationHistoricalV1(ctx, ref)
	if err != nil || fact.Validate() != nil || fact.Ref != ref {
		if err != nil {
			return ports.CheckpointRestoreEvidenceQualificationFactV1{}, err
		}
		return ports.CheckpointRestoreEvidenceQualificationFactV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence historical qualification drifted")
	}
	return fact, nil
}

func (g CheckpointRestoreEvidenceGatewayV1) InspectCheckpointPhaseQualificationCurrentV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceQualificationRefV1) (ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1, error) {
	if err := g.requireEvidenceCurrentDependenciesV1(); err != nil {
		return ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{}, err
	}
	now, err := g.evidenceNowV1(time.Time{})
	if err != nil {
		return ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{}, err
	}
	fact, err := g.InspectCheckpointPhaseQualificationHistoricalV1(ctx, ref)
	if err != nil {
		return ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{}, err
	}
	first, err := g.readCheckpointEvidenceQualificationInputsV1(ctx, fact.Request, now, ref.ExpiresUnixNano)
	if err != nil {
		return ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{}, err
	}
	fresh, err := g.evidenceNowV1(now)
	if err != nil {
		return ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{}, err
	}
	second, err := g.readCheckpointEvidenceQualificationInputsV1(ctx, fact.Request, fresh, ref.ExpiresUnixNano)
	if err != nil || !sameCheckpointEvidenceInputsV1(first, second) {
		if err != nil {
			return ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{}, err
		}
		return ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence qualification current inputs drifted")
	}
	projection, err := g.Facts.InspectCheckpointPhaseQualificationCurrentV1(ctx, ref)
	projectionScopeDigest, projectionScopeErr := projection.Scope.DigestV1()
	if err != nil || projection.Validate(fresh) != nil || projection.Ref != ref || projectionScopeErr != nil || projectionScopeDigest != mustCheckpointEvidenceScopeDigestV1(fact.Request.Scope) {
		if err != nil {
			return ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{}, err
		}
		return ports.CheckpointRestoreEvidenceQualificationCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence current qualification drifted")
	}
	return projection, nil
}

func (g CheckpointRestoreEvidenceGatewayV1) CreateCheckpointPhaseProviderHandoffV1(ctx context.Context, request ports.CreateCheckpointPhaseProviderHandoffRequestV1) (ports.CheckpointRestoreEvidenceProviderHandoffRefV1, error) {
	if err := request.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	qualification, err := g.InspectCheckpointPhaseQualificationCurrentV1(ctx, request.Qualification)
	if err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	if qualification.Scope.DispatchAttempt != request.Attempt {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence handoff changed the qualified Operation Attempt")
	}
	expected, err := checkpointEvidenceHandoffRefV1(request)
	if err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	created, err := g.Facts.CreateCheckpointPhaseProviderHandoffV1(ctx, request)
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
		}
		stored, inspectErr := g.InspectCheckpointPhaseProviderHandoffHistoricalV1(context.WithoutCancel(ctx), expected)
		if inspectErr != nil || stored != expected {
			return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "checkpoint Evidence handoff outcome cannot be inspected")
		}
		return expected, nil
	}
	if created != expected {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence Owner returned another handoff")
	}
	return created, nil
}

func (g CheckpointRestoreEvidenceGatewayV1) InspectCheckpointPhaseProviderHandoffHistoricalV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceProviderHandoffRefV1) (ports.CheckpointRestoreEvidenceProviderHandoffRefV1, error) {
	if err := ref.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Evidence Fact Owner"); err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	stored, err := g.Facts.InspectCheckpointPhaseProviderHandoffHistoricalV1(ctx, ref)
	if err != nil {
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
	}
	if err := stored.Validate(); err != nil || stored != ref {
		if err != nil {
			return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, err
		}
		return ports.CheckpointRestoreEvidenceProviderHandoffRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence historical handoff drifted")
	}
	return stored, nil
}

func (g CheckpointRestoreEvidenceGatewayV1) InspectCheckpointPhaseProviderHandoffCurrentV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceProviderHandoffRefV1) (ports.CheckpointRestoreEvidenceHandoffCurrentProjectionV1, error) {
	if err := ref.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceHandoffCurrentProjectionV1{}, err
	}
	if _, err := g.InspectCheckpointPhaseQualificationCurrentV1(ctx, ref.Qualification); err != nil {
		return ports.CheckpointRestoreEvidenceHandoffCurrentProjectionV1{}, err
	}
	if _, err := g.InspectCheckpointPhaseProviderHandoffHistoricalV1(ctx, ref); err != nil {
		return ports.CheckpointRestoreEvidenceHandoffCurrentProjectionV1{}, err
	}
	now, err := g.evidenceNowV1(time.Time{})
	if err != nil {
		return ports.CheckpointRestoreEvidenceHandoffCurrentProjectionV1{}, err
	}
	projection, err := g.Facts.InspectCheckpointPhaseProviderHandoffCurrentV1(ctx, ref)
	if err != nil || projection.Validate(now) != nil || projection.Ref != ref {
		if err != nil {
			return ports.CheckpointRestoreEvidenceHandoffCurrentProjectionV1{}, err
		}
		return ports.CheckpointRestoreEvidenceHandoffCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence handoff current projection drifted")
	}
	return projection, nil
}

func (g CheckpointRestoreEvidenceGatewayV1) ConsumeCheckpointPhaseEvidenceCurrentV1(ctx context.Context, request ports.ConsumeCheckpointPhaseEvidenceRequestV1) (ports.CheckpointRestoreEvidenceConsumptionRefV1, error) {
	return g.consumeCheckpointEvidenceV1(ctx, request, ports.CheckpointEvidenceConsumedCurrentV1)
}

func (g CheckpointRestoreEvidenceGatewayV1) ConsumeCheckpointPhaseEvidenceObservationV1(ctx context.Context, request ports.ConsumeCheckpointPhaseEvidenceRequestV1) (ports.CheckpointRestoreEvidenceConsumptionRefV1, error) {
	return g.consumeCheckpointEvidenceV1(ctx, request, ports.CheckpointEvidenceConsumedObservationV1)
}

func (g CheckpointRestoreEvidenceGatewayV1) InspectCheckpointPhaseEvidenceConsumptionCurrentV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceConsumptionRefV1) (ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1, error) {
	if err := ref.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1{}, err
	}
	if err := requireCheckpointDependencyV2(g.Records, "Evidence ledger record Reader"); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1{}, err
	}
	qualification, err := g.InspectCheckpointPhaseQualificationCurrentV1(ctx, ref.Qualification)
	if err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1{}, err
	}
	if _, err := g.InspectCheckpointPhaseProviderHandoffCurrentV1(ctx, ref.Handoff); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1{}, err
	}
	if _, err := g.inspectCheckpointEvidenceLedgerRecordV1(ctx, ref.Record, ref.Source, qualification.Scope, ref.State); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1{}, err
	}
	if _, err := g.InspectCheckpointPhaseEvidenceConsumptionHistoricalV1(ctx, ref); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1{}, err
	}
	projection, err := g.Facts.InspectCheckpointPhaseEvidenceConsumptionCurrentV1(ctx, ref)
	if err != nil || projection.Validate() != nil || projection.Ref != ref {
		if err != nil {
			return ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1{}, err
		}
		return ports.CheckpointRestoreEvidenceConsumptionCurrentProjectionV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence consumption current projection drifted")
	}
	return projection, nil
}

func (g CheckpointRestoreEvidenceGatewayV1) InspectCheckpointPhaseEvidenceConsumptionHistoricalV1(ctx context.Context, ref ports.CheckpointRestoreEvidenceConsumptionRefV1) (ports.CheckpointRestoreEvidenceConsumptionRefV1, error) {
	if err := ref.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	if err := requireCheckpointDependencyV2(g.Facts, "checkpoint Evidence Fact Owner"); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	stored, err := g.Facts.InspectCheckpointPhaseEvidenceConsumptionHistoricalV1(ctx, ref)
	if err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	if err := stored.Validate(); err != nil || stored != ref {
		if err != nil {
			return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
		}
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence historical consumption drifted")
	}
	return stored, nil
}

func (g CheckpointRestoreEvidenceGatewayV1) consumeCheckpointEvidenceV1(ctx context.Context, request ports.ConsumeCheckpointPhaseEvidenceRequestV1, state ports.CheckpointRestoreEvidenceConsumptionStateV1) (ports.CheckpointRestoreEvidenceConsumptionRefV1, error) {
	if err := request.Validate(); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	if err := requireCheckpointDependencyV2(g.Records, "Evidence ledger record Reader"); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	fact, err := g.InspectCheckpointPhaseQualificationHistoricalV1(ctx, request.Qualification)
	if err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	if fact.Request.Scope.Source != request.Source {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceSourceStale, "checkpoint Evidence consumption changed the qualified source coordinates")
	}
	firstQualification, err := g.InspectCheckpointPhaseQualificationCurrentV1(ctx, request.Qualification)
	if err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	if _, err := g.InspectCheckpointPhaseProviderHandoffCurrentV1(ctx, request.Handoff); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	firstRecordDigest, err := g.inspectCheckpointEvidenceLedgerRecordV1(ctx, request.Record, request.Source, firstQualification.Scope, state)
	if err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	secondQualification, err := g.InspectCheckpointPhaseQualificationCurrentV1(ctx, request.Qualification)
	if err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	if _, err := g.InspectCheckpointPhaseProviderHandoffCurrentV1(ctx, request.Handoff); err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	secondRecordDigest, err := g.inspectCheckpointEvidenceLedgerRecordV1(ctx, request.Record, request.Source, secondQualification.Scope, state)
	if err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	firstScopeDigest, firstScopeErr := firstQualification.Scope.DigestV1()
	secondScopeDigest, secondScopeErr := secondQualification.Scope.DigestV1()
	if firstScopeErr != nil || secondScopeErr != nil || firstScopeDigest != secondScopeDigest || firstRecordDigest != secondRecordDigest {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence ledger or qualification changed before consumption")
	}
	expected, err := checkpointEvidenceConsumptionRefV1(request, state)
	if err != nil {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
	}
	var created ports.CheckpointRestoreEvidenceConsumptionRefV1
	if state == ports.CheckpointEvidenceConsumedCurrentV1 {
		created, err = g.Facts.ConsumeCheckpointPhaseEvidenceCurrentV1(ctx, request)
	} else {
		created, err = g.Facts.ConsumeCheckpointPhaseEvidenceObservationV1(ctx, request)
	}
	if err != nil {
		if !checkpointRecoverableV2(err) {
			return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, err
		}
		stored, inspectErr := g.InspectCheckpointPhaseEvidenceConsumptionHistoricalV1(context.WithoutCancel(ctx), expected)
		if inspectErr != nil || stored != expected {
			return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, core.NewError(core.ErrorIndeterminate, core.ReasonEvidenceUnavailable, "checkpoint Evidence consume outcome cannot be inspected")
		}
		return expected, nil
	}
	if created != expected {
		return ports.CheckpointRestoreEvidenceConsumptionRefV1{}, core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "checkpoint Evidence Owner returned another consumption")
	}
	return created, nil
}

func (g CheckpointRestoreEvidenceGatewayV1) inspectCheckpointEvidenceLedgerRecordV1(ctx context.Context, ref ports.EvidenceRecordRefV2, source ports.EvidenceSourceKeyV2, scope ports.CheckpointRestoreEvidenceScopeV1, state ports.CheckpointRestoreEvidenceConsumptionStateV1) (core.Digest, error) {
	byRef, err := g.Records.InspectRecord(ctx, ref)
	if err != nil {
		return "", err
	}
	bySource, err := g.Records.InspectBySource(ctx, source)
	if err != nil {
		return "", err
	}
	if err := control.ValidateEvidenceLedgerRecordV2(byRef); err != nil {
		return "", err
	}
	if err := control.ValidateEvidenceLedgerRecordV2(bySource); err != nil {
		return "", err
	}
	if byRef.Ref != ref || bySource.Ref != ref || byRef.Ref != bySource.Ref || byRef.CandidateDigest != bySource.CandidateDigest || byRef.Ref.RecordDigest != bySource.Ref.RecordDigest {
		return "", core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence record ref and source key do not resolve to one immutable ledger record")
	}
	candidate := byRef.Candidate
	executionScope := scope.Operation.ExecutionScope
	expectedLedgerScope := ports.EvidenceLedgerScopeV2{Partition: ports.EvidencePartitionInstance, TenantID: executionScope.Identity.TenantID, IdentityID: executionScope.Identity.ID, LineageID: executionScope.Lineage.ID, InstanceID: executionScope.Instance.ID}
	if candidate.RegistrationID != source.RegistrationID || candidate.SourceEpoch != source.SourceEpoch || candidate.SourceSequence != source.SourceSequence || candidate.LedgerScope != expectedLedgerScope || candidate.Payload.Schema != scope.PayloadSchema || candidate.Payload.ContentDigest != scope.PayloadDigest || candidate.Payload.Revision != scope.PayloadRevision || candidate.Payload.Length != scope.PayloadLength || !ports.SameExecutionScopeV2(candidate.ExecutionScope, executionScope) || candidate.Authority != scope.Authority {
		return "", core.NewError(core.ErrorConflict, core.ReasonEvidenceConflict, "Evidence ledger record does not bind the qualified source, payload, authority and execution scope")
	}
	if byRef.Ref.Sequence > 1 {
		previousRef := ports.EvidenceRecordRefV2{LedgerScopeDigest: byRef.Ref.LedgerScopeDigest, Sequence: byRef.Ref.Sequence - 1, RecordDigest: byRef.PreviousRecordDigest}
		previous, inspectErr := g.Records.InspectRecord(ctx, previousRef)
		if inspectErr != nil {
			return "", inspectErr
		}
		if validateErr := control.ValidateEvidenceLedgerRecordV2(previous); validateErr != nil {
			return "", validateErr
		}
		if previous.Ref != previousRef {
			return "", core.NewError(core.ErrorConflict, core.ReasonEvidenceChainConflict, "Evidence ledger predecessor does not close the consumed record chain")
		}
	}
	if state == ports.CheckpointEvidenceConsumedCurrentV1 && candidate.TrustClass == ports.EvidenceTrustLateObservation {
		return "", core.NewError(core.ErrorPreconditionFailed, core.ReasonEvidenceTrustInvalid, "late Evidence cannot become consumed_current")
	}
	return byRef.Ref.RecordDigest, nil
}

func checkpointEvidenceHandoffRefV1(request ports.CreateCheckpointPhaseProviderHandoffRequestV1) (ports.CheckpointRestoreEvidenceProviderHandoffRefV1, error) {
	ref := ports.CheckpointRestoreEvidenceProviderHandoffRefV1{ID: request.ID, Revision: 1, Qualification: request.Qualification, Attempt: request.Attempt, Phase: request.Phase, ScopeDigest: request.ScopeDigest}
	var err error
	ref.Digest, err = ref.DigestV1()
	return ref, err
}

func checkpointEvidenceConsumptionRefV1(request ports.ConsumeCheckpointPhaseEvidenceRequestV1, state ports.CheckpointRestoreEvidenceConsumptionStateV1) (ports.CheckpointRestoreEvidenceConsumptionRefV1, error) {
	ref := ports.CheckpointRestoreEvidenceConsumptionRefV1{ID: request.ID, Revision: 1, Qualification: request.Qualification, Handoff: request.Handoff, Record: request.Record, Attempt: request.Qualification.Attempt, Phase: request.Qualification.Phase, State: state, ScopeDigest: request.Qualification.ScopeDigest, Source: request.Source}
	var err error
	ref.Digest, err = ref.DigestV1()
	return ref, err
}

func sameCheckpointEvidenceInputsV1(left, right checkpointEvidenceQualificationInputsV1) bool {
	return left == right
}

func mustCheckpointEvidenceScopeDigestV1(scope ports.CheckpointRestoreEvidenceScopeV1) core.Digest {
	digest, _ := scope.DigestV1()
	return digest
}

func (g CheckpointRestoreEvidenceGatewayV1) requireEvidenceCurrentDependenciesV1() error {
	for _, dependency := range []struct {
		value any
		name  string
	}{{g.Facts, "checkpoint Evidence Fact Owner"}, {g.Checkpoints, "checkpoint current Reader"}, {g.Inputs, "checkpoint inputs current Reader"}, {g.Reservations, "checkpoint reservation current Reader"}, {g.Execution, "checkpoint Permit and Enforcement current Reader"}, {g.Policies, "checkpoint Evidence Policy current Reader"}, {g.Sources, "checkpoint Evidence source current Reader"}, {g.Clock, "checkpoint Evidence clock"}} {
		if err := requireCheckpointDependencyV2(dependency.value, dependency.name); err != nil {
			return err
		}
	}
	return nil
}

func (g CheckpointRestoreEvidenceGatewayV1) evidenceNowV1(previous time.Time) (time.Time, error) {
	now := g.Clock()
	if now.IsZero() {
		return time.Time{}, core.NewError(core.ErrorInvalidArgument, core.ReasonClockRegression, "checkpoint Evidence clock returned zero")
	}
	if !previous.IsZero() && now.Before(previous) {
		return time.Time{}, core.NewError(core.ErrorPreconditionFailed, core.ReasonClockRegression, "checkpoint Evidence clock regressed")
	}
	return now, nil
}
