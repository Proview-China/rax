package runtimeadapter

import (
	"context"
	"errors"
	"fmt"
	"reflect"
	"slices"
	"time"

	runtimecore "github.com/Proview-China/rax/ExecutionRuntime/runtime/core"
	runtimeports "github.com/Proview-China/rax/ExecutionRuntime/runtime/ports"
	"github.com/Proview-China/rax/ExecutionRuntime/sandbox/contract"
	sandboxports "github.com/Proview-China/rax/ExecutionRuntime/sandbox/ports"
)

const checkpointDispatchExecutionCurrentContractV1 = "praxis.sandbox/checkpoint-dispatch-execution-current/v1"

// CheckpointDispatchExecutionCurrentV1 is an exact association supplied by
// the Runtime dispatch/Provider-attempt Owners. Generic Sandbox current Facts
// remain separate and must match these refs. It grants no Provider authority.
type CheckpointDispatchExecutionCurrentV1 struct {
	ContractVersion     string                                               `json:"contract_version"`
	OperationFact       contract.Ref                                         `json:"operation_fact"`
	AttemptFact         contract.Ref                                         `json:"attempt_fact"`
	Attempt             runtimeports.OperationDispatchAttemptRefV3           `json:"attempt"`
	PrepareFact         *contract.Ref                                        `json:"prepare_fact,omitempty"`
	Prepare             *runtimeports.OperationDispatchEnforcementPhaseRefV4 `json:"prepare,omitempty"`
	PreparedAttemptFact *contract.Ref                                        `json:"prepared_attempt_fact,omitempty"`
	PreparedAttempt     *runtimeports.PreparedProviderAttemptRefV2           `json:"prepared_attempt,omitempty"`
	Current             bool                                                 `json:"current"`
	CheckedUnixNano     int64                                                `json:"checked_unix_nano"`
	ExpiresUnixNano     int64                                                `json:"expires_unix_nano"`
	ProjectionDigest    runtimecore.Digest                                   `json:"projection_digest"`
}

func (p CheckpointDispatchExecutionCurrentV1) Validate(now time.Time) error {
	if p.ContractVersion != checkpointDispatchExecutionCurrentContractV1 || p.OperationFact.ValidateShape("checkpoint Runtime operation fact") != nil || p.AttemptFact.ValidateShape("checkpoint Runtime attempt fact") != nil || p.Attempt.Validate() != nil || !p.Current || p.CheckedUnixNano <= 0 || p.ExpiresUnixNano <= p.CheckedUnixNano || now.IsZero() || now.UnixNano() < p.CheckedUnixNano || !now.Before(time.Unix(0, p.ExpiresUnixNano)) {
		return fmt.Errorf("%w: checkpoint Runtime dispatch exact projection is incomplete or stale", sandboxports.ErrStale)
	}
	preparePresent := p.PrepareFact != nil || p.Prepare != nil || p.PreparedAttemptFact != nil || p.PreparedAttempt != nil
	if preparePresent {
		if p.PrepareFact == nil || p.Prepare == nil || p.PreparedAttemptFact == nil || p.PreparedAttempt == nil || p.PrepareFact.ValidateShape("checkpoint prepare enforcement fact") != nil || p.Prepare.Validate() != nil || p.PreparedAttemptFact.ValidateShape("checkpoint prepared attempt fact") != nil || p.PreparedAttempt.Validate() != nil || p.Prepare.Phase != runtimeports.OperationDispatchEnforcementPrepareV4 || p.Prepare.OperationDigest != p.Attempt.OperationDigest || p.Prepare.EffectID != p.Attempt.EffectID || p.Prepare.AttemptID != p.Attempt.AttemptID || p.PreparedAttempt.OperationDigest != p.Attempt.OperationDigest || p.PreparedAttempt.IntentID != p.Attempt.EffectID || p.PreparedAttempt.IntentRevision != p.Attempt.IntentRevision || p.PreparedAttempt.IntentDigest != p.Attempt.IntentDigest || p.PreparedAttempt.AttemptID != p.Attempt.AttemptID || p.ExpiresUnixNano > p.Prepare.ExpiresUnixNano || p.ExpiresUnixNano > p.PreparedAttempt.ExpiresUnixNano {
			return fmt.Errorf("%w: checkpoint Runtime prepared execution association drifted", sandboxports.ErrConflict)
		}
	}
	digest, err := p.DigestV1()
	if err != nil || digest != p.ProjectionDigest {
		return fmt.Errorf("%w: checkpoint Runtime dispatch projection digest drifted", sandboxports.ErrConflict)
	}
	return nil
}

func (p CheckpointDispatchExecutionCurrentV1) DigestV1() (runtimecore.Digest, error) {
	copy := p
	copy.ProjectionDigest = ""
	return runtimecore.CanonicalJSONDigest("praxis.sandbox.checkpoint-dispatch-execution-current", "1.0.0", "CheckpointDispatchExecutionCurrentV1", copy)
}

func SealCheckpointDispatchExecutionCurrentV1(p CheckpointDispatchExecutionCurrentV1, now time.Time) (CheckpointDispatchExecutionCurrentV1, error) {
	p.ContractVersion = checkpointDispatchExecutionCurrentContractV1
	p.ProjectionDigest = ""
	digest, err := p.DigestV1()
	if err != nil {
		return CheckpointDispatchExecutionCurrentV1{}, err
	}
	p.ProjectionDigest = digest
	return p, p.Validate(now)
}

type CheckpointDispatchExecutionCurrentReaderV1 interface {
	InspectCheckpointDispatchExecutionCurrentV1(context.Context, runtimeports.OperationSubjectV3, runtimecore.EffectIntentID, string) (CheckpointDispatchExecutionCurrentV1, error)
}

type CheckpointRestoreDispatchCurrentReaderV1 struct {
	store        sandboxports.CheckpointPhaseStore
	source       sandboxports.CheckpointCurrentSource
	ownerCurrent sandboxports.CheckpointParticipantCurrentReader
	reservations runtimeports.CheckpointParticipantPhaseReservationCurrentReaderV2
	execution    CheckpointDispatchExecutionCurrentReaderV1
	clock        func() time.Time
}

func NewCheckpointRestoreDispatchCurrentReaderV1(store sandboxports.CheckpointPhaseStore, source sandboxports.CheckpointCurrentSource, ownerCurrent sandboxports.CheckpointParticipantCurrentReader, reservations runtimeports.CheckpointParticipantPhaseReservationCurrentReaderV2, execution CheckpointDispatchExecutionCurrentReaderV1, clock func() time.Time) (*CheckpointRestoreDispatchCurrentReaderV1, error) {
	if checkpointAdapterNilV1(store) || checkpointAdapterNilV1(source) || checkpointAdapterNilV1(ownerCurrent) || checkpointAdapterNilV1(reservations) || checkpointAdapterNilV1(execution) || clock == nil {
		return nil, errors.New("checkpoint Store, current sources, Runtime readers, and clock are required")
	}
	return &CheckpointRestoreDispatchCurrentReaderV1{store: store, source: source, ownerCurrent: ownerCurrent, reservations: reservations, execution: execution, clock: clock}, nil
}

func (r *CheckpointRestoreDispatchCurrentReaderV1) InspectCheckpointRestoreDispatchSandboxCurrentV1(ctx context.Context, operation runtimeports.OperationSubjectV3, effectID runtimecore.EffectIntentID, expected runtimeports.CheckpointParticipantPhaseReservationRefV2) (runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1, error) {
	now := r.clock()
	if now.IsZero() || operation.Validate() != nil || effectID == "" || expected.Validate() != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, fmt.Errorf("%w: checkpoint dispatch exact request is invalid", sandboxports.ErrConflict)
	}
	localExpected := contract.Ref{ID: expected.ID, Revision: uint64(expected.Revision), Digest: trimRuntimeDigestV1(expected.Digest)}
	reservation, err := r.store.InspectCheckpointPhaseReservation(ctx, localExpected)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	if err := reservation.ValidateCurrent(now); err != nil || !contract.SameRef(reservation.Meta.Ref(), localExpected) || reservation.OperationID != exactOperationID(operation) || reservation.EffectID != string(effectID) {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, fmt.Errorf("%w: checkpoint Reservation is not exact current for the operation", sandboxports.ErrStale)
	}
	participant, err := r.store.InspectCheckpointParticipantCurrent(ctx, reservation.ParticipantRef.ID)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	if err := participant.ValidateCurrent(now); err != nil || participant.ActiveReservation.Ref == nil || !contract.SameRef(*participant.ActiveReservation.Ref, reservation.Meta.Ref()) || participant.Meta.ID != reservation.ParticipantRef.ID || participant.Meta.Revision != reservation.ParticipantRef.Revision+1 {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, fmt.Errorf("%w: checkpoint Participant current moved from the Reservation", sandboxports.ErrStale)
	}
	runtimePhase, err := checkpointRuntimePhaseV1(reservation.Phase)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	runtimeReservation, err := r.reservations.InspectCheckpointParticipantPhaseReservationCurrentV2(ctx, expected, runtimePhase)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	if err := runtimeReservation.Validate(now); err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	if err := validateCheckpointReservationMappingV1(reservation, runtimeReservation, operation, effectID, participant); err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	execution, err := r.execution.InspectCheckpointDispatchExecutionCurrentV1(ctx, operation, effectID, reservation.ExpectedRuntimeAttemptRef.ID)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	if err := execution.Validate(now); err != nil || execution.Attempt.OperationDigest != mustRuntimeOperationDigestV1(operation) || execution.Attempt.EffectID != effectID || execution.Attempt.AttemptID != reservation.ExpectedRuntimeAttemptRef.ID || !contract.SameRef(execution.AttemptFact, reservation.ExpectedRuntimeAttemptRef) || runtimeReservation.IntentDigest != execution.Attempt.IntentDigest {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, fmt.Errorf("%w: checkpoint Runtime dispatch Attempt is not exact current", sandboxports.ErrStale)
	}
	stage, expectedRefs, firstRead, err := r.readExpectedCheckpointCurrentV1(ctx, reservation, execution, now)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	request := checkpointCurrentRequestV1(reservation, participant, stage, expectedRefs)
	ownerProjection, err := r.ownerCurrent.ReadCheckpointParticipantCurrent(ctx, &request)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	fresh := r.clock()
	if err := ownerProjection.ValidateCurrent(fresh); err != nil || !contract.SameRef(ownerProjection.ReservationRef, reservation.Meta.Ref()) || !contract.SameRef(ownerProjection.ParticipantRef, participant.Meta.Ref()) {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, fmt.Errorf("%w: checkpoint Owner S2 projection drifted", sandboxports.ErrStale)
	}
	if err := validateCheckpointS1S2V1(firstRead, ownerProjection.Current); err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	return mapCheckpointDispatchProjectionV1(operation, effectID, reservation, participant, runtimeReservation, execution, ownerProjection, fresh)
}

func (r *CheckpointRestoreDispatchCurrentReaderV1) readExpectedCheckpointCurrentV1(ctx context.Context, reservation contract.CheckpointPhaseReservation, execution CheckpointDispatchExecutionCurrentV1, now time.Time) (contract.CheckpointReadStage, []contract.CheckpointExpectedCurrentRef, []contract.CheckpointCurrentCoordinate, error) {
	query := contract.CheckpointCurrentQuery{TenantID: reservation.TenantID, ParticipantID: reservation.ParticipantRef.ID, CheckpointAttemptRef: reservation.Base.CheckpointAttempt, Phase: reservation.Phase, OperationID: reservation.OperationID, EffectID: reservation.EffectID, AttemptID: reservation.AttemptID, ExpectedRuntimeAttemptRef: reservation.ExpectedRuntimeAttemptRef}
	values := make(map[contract.CheckpointCurrentKind]contract.CheckpointCurrentCoordinate, len(contract.AllCheckpointCurrentKinds()))
	absent := make(map[contract.CheckpointCurrentKind]bool)
	for _, kind := range contract.AllCheckpointCurrentKinds() {
		query.Kind = kind
		value, err := r.source.InspectCheckpointCurrent(ctx, query)
		if errors.Is(err, sandboxports.ErrNotFound) {
			absent[kind] = true
			continue
		}
		if err != nil {
			return "", nil, nil, err
		}
		if err := value.ValidateCurrent(now); err != nil {
			return "", nil, nil, err
		}
		values[kind] = value
	}
	prepareAbsent := absent[contract.CheckpointCurrentPrepareEnforcement]
	preparedAbsent := absent[contract.CheckpointCurrentPreparedAttempt]
	stage := contract.CheckpointReadPrePrepare
	if !prepareAbsent || !preparedAbsent {
		if prepareAbsent || preparedAbsent || execution.Prepare == nil || execution.PreparedAttempt == nil || execution.PrepareFact == nil || execution.PreparedAttemptFact == nil {
			return "", nil, nil, fmt.Errorf("%w: checkpoint prepared current has partial presence", sandboxports.ErrConflict)
		}
		stage = contract.CheckpointReadPreExecute
		if !contract.SameRef(values[contract.CheckpointCurrentPrepareEnforcement].Meta.Ref(), *execution.PrepareFact) || !contract.SameRef(values[contract.CheckpointCurrentPreparedAttempt].Meta.Ref(), *execution.PreparedAttemptFact) {
			return "", nil, nil, fmt.Errorf("%w: checkpoint prepared typed refs differ from Sandbox current Facts", sandboxports.ErrConflict)
		}
	} else if execution.Prepare != nil || execution.PreparedAttempt != nil || execution.PrepareFact != nil || execution.PreparedAttemptFact != nil {
		return "", nil, nil, fmt.Errorf("%w: checkpoint pre-prepare source carries typed prepared state", sandboxports.ErrConflict)
	}
	expected := make([]contract.CheckpointExpectedCurrentRef, 0, len(contract.AllCheckpointCurrentKinds()))
	first := make([]contract.CheckpointCurrentCoordinate, 0, len(values))
	for _, kind := range contract.AllCheckpointCurrentKinds() {
		presence := contract.CheckpointExpectedPresenceFor(stage, kind, reservation.ChangeSet.Presence)
		value, present := values[kind]
		if presence == contract.CheckpointPresent {
			if !present {
				return "", nil, nil, fmt.Errorf("%w: required checkpoint current %s is absent", sandboxports.ErrNotFound, kind)
			}
			ref := value.Meta.Ref()
			expected = append(expected, contract.CheckpointExpectedCurrentRef{Kind: kind, Presence: presence, Ref: &ref})
			first = append(first, value)
		} else {
			if present {
				return "", nil, nil, fmt.Errorf("%w: checkpoint current %s must be absent", sandboxports.ErrConflict, kind)
			}
			expected = append(expected, contract.CheckpointExpectedCurrentRef{Kind: kind, Presence: presence})
		}
	}
	return stage, expected, first, nil
}

func checkpointCurrentRequestV1(reservation contract.CheckpointPhaseReservation, participant contract.CheckpointParticipantFact, stage contract.CheckpointReadStage, expected []contract.CheckpointExpectedCurrentRef) contract.CheckpointCurrentReadRequest {
	return contract.CheckpointCurrentReadRequest{
		TenantID: reservation.TenantID, ParticipantRef: participant.Meta.Ref(), CheckpointAttemptRef: reservation.Base.CheckpointAttempt,
		Phase: reservation.Phase, PreviousPresence: reservation.PreviousPresence, Stage: stage, ExpectedReservationRef: reservation.Meta.Ref(), ExpectedPreviousPhase: reservation.PreviousPhase,
		OperationID: reservation.OperationID, EffectID: reservation.EffectID, AttemptID: reservation.AttemptID, ExpectedRuntimeAttempt: reservation.ExpectedRuntimeAttemptRef,
		Runtime: reservation.Runtime, ChangeSet: reservation.ChangeSet, Watermarks: slices.Clone(reservation.Watermarks), ExpectedCurrentRefs: expected,
	}
}

func validateCheckpointS1S2V1(first, second []contract.CheckpointCurrentCoordinate) error {
	if len(first) != len(second) {
		return fmt.Errorf("%w: checkpoint Owner current presence changed between S1 and S2", sandboxports.ErrStale)
	}
	for index := range first {
		if first[index].Kind != second[index].Kind || !contract.SameRef(first[index].Meta.Ref(), second[index].Meta.Ref()) {
			return fmt.Errorf("%w: checkpoint Owner current ref changed between S1 and S2", sandboxports.ErrStale)
		}
	}
	return nil
}

func validateCheckpointReservationMappingV1(local contract.CheckpointPhaseReservation, source runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2, operation runtimeports.OperationSubjectV3, effectID runtimecore.EffectIntentID, participant contract.CheckpointParticipantFact) error {
	if source.Ref.ID != local.Meta.ID || uint64(source.Ref.Revision) != local.Meta.Revision || trimRuntimeDigestV1(source.Ref.Digest) != local.Meta.Digest || source.Ref.ExpiresUnixNano != local.Meta.ExpiresUnixNano || source.Participant.ID != local.ParticipantRef.ID || trimRuntimeDigestV1(source.Participant.Digest) != local.ParticipantRef.Digest || source.Attempt.ID != local.Base.CheckpointAttempt.ID || uint64(source.Attempt.Revision) != local.Base.CheckpointAttempt.Revision || trimRuntimeDigestV1(source.Attempt.Digest) != local.Base.CheckpointAttempt.Digest || source.Barrier.ID != local.Base.Barrier.ID || uint64(source.Barrier.Revision) != local.Base.Barrier.Revision || trimRuntimeDigestV1(source.Barrier.Digest) != local.Base.Barrier.Digest || source.EffectCut.ID != local.Base.EffectCut.ID || uint64(source.EffectCut.Revision) != local.Base.EffectCut.Revision || trimRuntimeDigestV1(source.EffectCut.Digest) != local.Base.EffectCut.Digest || !runtimeports.SameOperationSubjectV3(source.Operation, operation) || source.EffectID != effectID || participant.CheckpointAttemptRef.ID != source.Attempt.ID {
		return fmt.Errorf("%w: Runtime and Sandbox checkpoint Reservation exact refs differ", sandboxports.ErrConflict)
	}
	return nil
}

func mapCheckpointDispatchProjectionV1(operation runtimeports.OperationSubjectV3, effectID runtimecore.EffectIntentID, reservation contract.CheckpointPhaseReservation, participant contract.CheckpointParticipantFact, runtimeReservation runtimeports.CheckpointParticipantPhaseReservationCurrentProjectionV2, execution CheckpointDispatchExecutionCurrentV1, local contract.CheckpointParticipantCurrentProjection, now time.Time) (runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1, error) {
	byKind := make(map[contract.CheckpointCurrentKind]contract.CheckpointCurrentCoordinate, len(local.Current))
	for _, value := range local.Current {
		byKind[value.Kind] = value
	}
	require := func(kind contract.CheckpointCurrentKind) (contract.CheckpointCurrentCoordinate, error) {
		value, ok := byKind[kind]
		if !ok {
			return contract.CheckpointCurrentCoordinate{}, fmt.Errorf("%w: checkpoint mapped current %s is absent", sandboxports.ErrNotFound, kind)
		}
		return value, nil
	}
	requirement, err := require(contract.CheckpointCurrentRequirement)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	policy, err := require(contract.CheckpointCurrentPolicy)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	workspace, err := require(contract.CheckpointCurrentWorkspace)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	placement, err := require(contract.CheckpointCurrentPlacement)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	backend, err := require(contract.CheckpointCurrentBackend)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	slot, err := require(contract.CheckpointCurrentSlot)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	runtimeLease, err := require(contract.CheckpointCurrentRuntimeLease)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	operationCurrent, err := require(contract.CheckpointCurrentOperation)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	attemptCurrent, err := require(contract.CheckpointCurrentAttempt)
	if err != nil {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, err
	}
	if !contract.SameRef(operationCurrent.Meta.Ref(), execution.OperationFact) || !contract.SameRef(attemptCurrent.Meta.Ref(), execution.AttemptFact) {
		return runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{}, fmt.Errorf("%w: checkpoint Runtime typed execution refs differ from Owner current", sandboxports.ErrConflict)
	}
	var changeSet *runtimeports.OperationDispatchSandboxFactRefV4
	if value, ok := byKind[contract.CheckpointCurrentChangeSet]; ok {
		ref := runtimeFactRef(value.Meta)
		changeSet = &ref
	}
	stage := runtimeports.CheckpointRestoreDispatchSandboxPrePrepareV1
	if local.Stage == contract.CheckpointReadPreExecute {
		stage = runtimeports.CheckpointRestoreDispatchSandboxPreExecuteV1
	}
	expires := minimumExpiry(local.ExpiresUnixNano, runtimeReservation.ExpiresUnixNano, execution.ExpiresUnixNano)
	value := runtimeports.CheckpointRestoreDispatchSandboxCurrentProjectionV1{
		Operation: operation, OperationDigest: mustRuntimeOperationDigestV1(operation), EffectID: effectID,
		IntentRevision: execution.Attempt.IntentRevision, IntentDigest: execution.Attempt.IntentDigest,
		Reservation: runtimeReservation, SandboxReservation: runtimeFactRef(reservation.Meta), Participant: runtimeFactRef(participant.Meta), DispatchAttempt: runtimeFactRefFromLocalV1(execution.AttemptFact, minimumExpiry(execution.ExpiresUnixNano, attemptCurrent.Meta.ExpiresUnixNano)),
		RuntimeLease: runtimeports.OperationDispatchRuntimeLeaseBindingV4{Ref: runtimeFactRef(runtimeLease.Meta), Lease: runtimecore.SandboxLeaseRef{ID: runtimecore.SandboxLeaseID(reservation.Runtime.LeaseID), Epoch: runtimecore.Epoch(reservation.Runtime.LeaseEpoch)}, Instance: runtimecore.InstanceRef{ID: runtimecore.AgentInstanceID(reservation.Runtime.InstanceID), Epoch: runtimecore.Epoch(reservation.Runtime.InstanceEpoch)}, FenceEpoch: runtimecore.Epoch(reservation.Runtime.FenceEpoch), ScopeDigest: operation.ExecutionScopeDigest, ObservedRevision: runtimecore.Revision(runtimeLease.Meta.Revision)},
		Requirement:  runtimeFactRef(requirement.Meta), Policy: runtimeFactRef(policy.Meta), Workspace: runtimeFactRef(workspace.Meta), ChangeSet: changeSet,
		Placement: runtimeFactRef(placement.Meta), Backend: runtimeFactRef(backend.Meta), Slot: runtimeFactRef(slot.Meta), Generation: runtimeReservation.Generation, Verifier: runtimeReservation.OwnerBinding,
		Stage: stage, PrepareEnforcement: execution.Prepare, PreparedAttempt: execution.PreparedAttempt,
		Watermarks: make([]runtimeports.CheckpointRestoreDispatchWatermarkV1, len(local.Watermarks)), Current: true, ProjectionRevision: runtimecore.Revision(local.ProjectionRevision), CheckedUnixNano: now.UnixNano(), ExpiresUnixNano: expires,
	}
	for index, watermark := range local.Watermarks {
		value.Watermarks[index] = runtimeports.CheckpointRestoreDispatchWatermarkV1{SourceID: watermark.SourceID, SourceEpoch: runtimecore.Epoch(watermark.SourceEpoch), Sequence: watermark.Sequence}
	}
	return runtimeports.SealCheckpointRestoreDispatchSandboxCurrentProjectionV1(value, now)
}

func checkpointRuntimePhaseV1(phase contract.CheckpointPhase) (runtimeports.CheckpointParticipantPhaseV2, error) {
	switch phase {
	case contract.CheckpointPhasePrepare:
		return runtimeports.CheckpointPhasePrepareV2, nil
	case contract.CheckpointPhaseCommit:
		return runtimeports.CheckpointPhaseCommitV2, nil
	case contract.CheckpointPhaseAbort:
		return runtimeports.CheckpointPhaseAbortV2, nil
	default:
		return "", fmt.Errorf("%w: checkpoint phase is invalid", sandboxports.ErrConflict)
	}
}

func runtimeFactRefFromLocalV1(ref contract.Ref, expires int64) runtimeports.OperationDispatchSandboxFactRefV4 {
	return runtimeports.OperationDispatchSandboxFactRefV4{ID: ref.ID, Revision: runtimecore.Revision(ref.Revision), Digest: runtimeDigest(ref.Digest), ExpiresUnixNano: expires}
}

func trimRuntimeDigestV1(value runtimecore.Digest) string {
	text := string(value)
	if len(text) > len("sha256:") && text[:len("sha256:")] == "sha256:" {
		return text[len("sha256:"):]
	}
	return text
}

func mustRuntimeOperationDigestV1(operation runtimeports.OperationSubjectV3) runtimecore.Digest {
	digest, _ := operation.DigestV3()
	return digest
}

func checkpointAdapterNilV1(value any) bool {
	if value == nil {
		return true
	}
	rv := reflect.ValueOf(value)
	switch rv.Kind() {
	case reflect.Chan, reflect.Func, reflect.Interface, reflect.Map, reflect.Pointer, reflect.Slice:
		return rv.IsNil()
	default:
		return false
	}
}

var _ runtimeports.CheckpointRestoreDispatchSandboxCurrentReaderV1 = (*CheckpointRestoreDispatchCurrentReaderV1)(nil)
